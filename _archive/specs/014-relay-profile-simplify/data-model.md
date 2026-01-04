# Data Model: Relay Profile Simplification

**Feature Branch**: `014-relay-profile-simplify`
**Date**: 2025-12-12

## Overview

This document defines the data model changes for simplifying relay profiles. The key changes are:

1. **NEW**: `EncodingProfile` - Simplified profile with ~6 fields
2. **REMOVE**: `RelayProfileMapping` - Replaced by built-in client detection rules
3. **MODIFY**: `StreamProxy` - Update foreign key to reference EncodingProfile
4. **MODIFY**: `EpgSource` - Add timezone detection fields
5. **MODIFY**: Built-in client detection rules - Add explicit codec header rules

---

## Entity: EncodingProfile (NEW)

Replaces the complex RelayProfile model with a simplified encoding profile.

### Fields

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `id` | ULID | PK, NOT NULL | Unique identifier |
| `name` | string(100) | UNIQUE, NOT NULL | Human-readable name |
| `description` | string(500) | - | What this profile does |
| `target_video_codec` | VideoCodec | DEFAULT 'h264' | h264, h265, vp9, av1 |
| `target_audio_codec` | AudioCodec | DEFAULT 'aac' | aac, opus, ac3, eac3, mp3 |
| `quality_preset` | QualityPreset | DEFAULT 'medium' | low, medium, high, ultra |
| `hw_accel` | HWAccelType | DEFAULT 'auto' | auto, none, cuda, vaapi, qsv, videotoolbox |
| `is_default` | bool | DEFAULT false | Is this the default profile |
| `is_system` | bool | DEFAULT false | System profiles cannot be deleted |
| `enabled` | bool | DEFAULT true | Can this profile be used |
| `created_at` | timestamp | NOT NULL | Creation time |
| `updated_at` | timestamp | NOT NULL | Last update time |

### Validation Rules

1. `name` must be unique and non-empty
2. `target_video_codec` must be one of: h264, h265, vp9, av1
3. `target_audio_codec` must be one of: aac, opus, ac3, eac3, mp3
4. `quality_preset` must be one of: low, medium, high, ultra
5. `hw_accel` must be one of: auto, none, cuda, vaapi, qsv, videotoolbox
6. VP9/AV1 codecs automatically use fMP4 container
7. Opus codec automatically uses fMP4 container

### SQL Schema

```sql
CREATE TABLE encoding_profiles (
    id VARCHAR(26) PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    description VARCHAR(500),
    target_video_codec VARCHAR(20) NOT NULL DEFAULT 'h264',
    target_audio_codec VARCHAR(20) NOT NULL DEFAULT 'aac',
    quality_preset VARCHAR(20) NOT NULL DEFAULT 'medium',
    hw_accel VARCHAR(20) NOT NULL DEFAULT 'auto',
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    is_system BOOLEAN NOT NULL DEFAULT FALSE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_encoding_profiles_name ON encoding_profiles(name);
CREATE INDEX idx_encoding_profiles_is_default ON encoding_profiles(is_default);
```

### Go Model

```go
// QualityPreset defines encoding quality levels
type QualityPreset string

const (
    QualityPresetLow    QualityPreset = "low"
    QualityPresetMedium QualityPreset = "medium"
    QualityPresetHigh   QualityPreset = "high"
    QualityPresetUltra  QualityPreset = "ultra"
)

// EncodingProfile defines a simplified transcoding profile
type EncodingProfile struct {
    BaseModel

    Name             string        `gorm:"uniqueIndex;not null;size:100" json:"name"`
    Description      string        `gorm:"size:500" json:"description,omitempty"`
    TargetVideoCodec VideoCodec    `gorm:"size:20;default:'h264'" json:"target_video_codec"`
    TargetAudioCodec AudioCodec    `gorm:"size:20;default:'aac'" json:"target_audio_codec"`
    QualityPreset    QualityPreset `gorm:"size:20;default:'medium'" json:"quality_preset"`
    HWAccel          HWAccelType   `gorm:"size:20;default:'auto'" json:"hw_accel"`
    IsDefault        bool          `gorm:"default:false" json:"is_default"`
    IsSystem         bool          `gorm:"default:false" json:"is_system"`
    Enabled          bool          `gorm:"default:true" json:"enabled"`
}

func (EncodingProfile) TableName() string {
    return "encoding_profiles"
}
```

---

## Entity: StreamProxy (MODIFY)

Update foreign key from `relay_profile_id` to `encoding_profile_id`.

### Changed Fields

| Old Field | New Field | Notes |
|-----------|-----------|-------|
| `relay_profile_id` | `encoding_profile_id` | Same type (ULID), renamed |

### Migration

```sql
-- Add new column
ALTER TABLE stream_proxies ADD COLUMN encoding_profile_id VARCHAR(26);

-- Migrate data (after encoding_profiles populated)
UPDATE stream_proxies
SET encoding_profile_id = (
    SELECT ep.id FROM encoding_profiles ep
    JOIN relay_profiles rp ON rp.name = ep.name
    WHERE rp.id = stream_proxies.relay_profile_id
)
WHERE relay_profile_id IS NOT NULL;

-- Drop old column
ALTER TABLE stream_proxies DROP COLUMN relay_profile_id;

-- Add foreign key
ALTER TABLE stream_proxies
ADD CONSTRAINT fk_stream_proxies_encoding_profile
FOREIGN KEY (encoding_profile_id) REFERENCES encoding_profiles(id);
```

---

## Entity: EpgSource (MODIFY)

Add timezone detection fields.

### New Fields

| Field | Type | Constraints | Description |
|-------|------|-------------|-------------|
| `source_timezone` | string(50) | - | Detected timezone (e.g., "+0000", "Europe/London") |
| `timezone_offset_minutes` | int | DEFAULT 0 | UTC offset in minutes |

### SQL Schema Addition

```sql
ALTER TABLE epg_sources ADD COLUMN source_timezone VARCHAR(50);
ALTER TABLE epg_sources ADD COLUMN timezone_offset_minutes INTEGER DEFAULT 0;
```

### Go Model Addition

```go
type EpgSource struct {
    // ... existing fields ...

    // Detected timezone from source data
    SourceTimezone string `gorm:"size:50" json:"source_timezone,omitempty"`

    // UTC offset in minutes (0 = UTC, 60 = +0100, -300 = -0500)
    TimezoneOffsetMinutes int `gorm:"default:0" json:"timezone_offset_minutes"`
}
```

---

## Entity: RelayProfile (REMOVE)

The existing RelayProfile model will be dropped after migration to EncodingProfile.

### Migration Strategy

1. Create `encoding_profiles` table
2. Migrate data from `relay_profiles` to `encoding_profiles`:
   - Direct copy: name, description
   - Map: video_codec → target_video_codec (excluding auto/copy)
   - Map: audio_codec → target_audio_codec (excluding auto/copy)
   - Infer: quality_preset from video_bitrate/video_crf
   - Direct copy: hw_accel, is_default, is_system, enabled
3. Update `stream_proxies.relay_profile_id` → `encoding_profile_id`
4. Drop `relay_profiles` table

---

## Entity: RelayProfileMapping (REMOVE)

This table is immediately removed. Built-in client detection rules replace it.

### Migration Strategy

1. Backup all user-created rules (is_system = false) to JSON file
2. Log each removed rule as INFO
3. Drop `relay_profile_mappings` table

### Backup File Format

```json
{
  "backup_date": "2025-12-12T12:00:00Z",
  "rules": [
    {
      "id": "01ABC...",
      "name": "Custom Rule",
      "expression": "user_agent contains \"CustomPlayer\"",
      "preferred_video_codec": "h264",
      "preferred_audio_codec": "aac"
    }
  ]
}
```

---

## Built-in Client Detection Rules (MODIFY)

Add new high-priority rules for explicit codec headers.

### New Rules

| Priority | Name | Expression | Preferred Video | Preferred Audio |
|----------|------|------------|-----------------|-----------------|
| 1 | Explicit H.265 Video Request | `@header_req:X-Video-Codec == "h265"` | h265 | - |
| 2 | Explicit H.264 Video Request | `@header_req:X-Video-Codec == "h264"` | h264 | - |
| 3 | Explicit VP9 Video Request | `@header_req:X-Video-Codec == "vp9"` | vp9 | - |
| 4 | Explicit AV1 Video Request | `@header_req:X-Video-Codec == "av1"` | av1 | - |
| 5 | Explicit AAC Audio Request | `@header_req:X-Audio-Codec == "aac"` | - | aac |
| 6 | Explicit Opus Audio Request | `@header_req:X-Audio-Codec == "opus"` | - | opus |
| 7 | Explicit AC3 Audio Request | `@header_req:X-Audio-Codec == "ac3"` | - | ac3 |
| 8 | Explicit EAC3 Audio Request | `@header_req:X-Audio-Codec == "eac3"` | - | eac3 |

### Modified Rule: Default (Universal)

| Field | Old Value | New Value |
|-------|-----------|-----------|
| Expression | `true` | `user_agent contains ""` |

This change prevents SQLite boolean-to-integer conversion issues.

---

## Relationships

```
┌─────────────────┐         ┌──────────────────┐
│  StreamProxy    │ ──────> │ EncodingProfile  │
│                 │  0..1   │                  │
│ encoding_       │         │ id               │
│ profile_id (FK) │         │ name             │
│                 │         │ target_video_... │
└─────────────────┘         └──────────────────┘

┌─────────────────┐
│  EpgSource      │
│                 │
│ source_timezone │  (NEW)
│ timezone_offset │  (NEW)
│ ...             │
└─────────────────┘

┌─────────────────────────────┐
│  relay_profile_mappings     │  (REMOVED)
│  (client detection rules)   │
│                             │
│  Replaced by built-in rules │
│  in migrations registry     │
└─────────────────────────────┘
```

---

## State Transitions

### EncodingProfile

```
[Created] --> [Enabled] <--> [Disabled]
     |             |
     v             v
[Assigned to StreamProxy]
     |
     v
[Cannot delete while assigned]
```

### StreamProxy with EncodingProfile

```
[No Profile (Auto)] --> [Profile Assigned]
         ^                    |
         |                    v
         +---- [Profile Unassigned]

Auto Mode: Uses built-in client detection
Profile Mode: Forces specific codec regardless of client
```

---

## Indexes

### EncodingProfile
- `idx_encoding_profiles_name` on `name` (unique)
- `idx_encoding_profiles_is_default` on `is_default`

### StreamProxy (modified)
- Add index on `encoding_profile_id` for foreign key lookups

### EpgSource (no new indexes needed)
- Timezone fields are not searchable, only display

---

## Migration Order

1. Create `encoding_profiles` table
2. Seed system encoding profiles
3. Migrate data from `relay_profiles` to `encoding_profiles`
4. Add `encoding_profile_id` column to `stream_proxies`
5. Migrate `relay_profile_id` data to `encoding_profile_id`
6. Drop `relay_profile_id` from `stream_proxies`
7. Add timezone fields to `epg_sources`
8. Backup and log `relay_profile_mappings` user rules
9. Drop `relay_profile_mappings` table
10. Drop `relay_profiles` table
11. Add new built-in client detection rules (priorities 1-8)
12. Fix Default (Universal) rule expression
