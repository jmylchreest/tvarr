# Data Model: Client Detection UI Improvements

**Feature**: 018-client-detection-ui
**Date**: 2025-12-15

## Existing Entities (No Changes)

### ClientDetectionRule

The existing `ClientDetectionRule` model already supports all required fields. No schema changes needed.

```go
// internal/models/client_detection_rule.go
type ClientDetectionRule struct {
    ID          ulid.ULID `json:"id" gorm:"type:char(26);primaryKey"`
    Name        string    `json:"name" gorm:"size:255;not null"`
    Description string    `json:"description" gorm:"size:1024"`
    Expression  string    `json:"expression" gorm:"type:text;not null"`
    Priority    int       `json:"priority" gorm:"default:0"`
    IsEnabled   *bool     `json:"is_enabled" gorm:"default:true"`
    IsSystem    bool      `json:"is_system" gorm:"default:false"`

    // Codec/format configuration
    VideoCodecs     []string `json:"video_codecs" gorm:"serializer:json"`
    AudioCodecs     []string `json:"audio_codecs" gorm:"serializer:json"`
    OutputFormat    string   `json:"output_format" gorm:"size:50"`

    // Timestamps
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

**Key Fields for This Feature**:
- `IsSystem` - marks system rules that cannot be deleted
- `Expression` - supports `@dynamic()` syntax

### Channel

Existing model, searchable fields:

```go
// internal/models/channel.go
type Channel struct {
    ID            ulid.ULID `json:"id" gorm:"type:char(26);primaryKey"`
    SourceID      ulid.ULID `json:"source_id"`
    ExtID         string    `json:"ext_id" gorm:"index"`           // Searchable
    TvgID         string    `json:"tvg_id" gorm:"index"`           // Searchable
    TvgName       string    `json:"tvg_name" gorm:"size:512"`      // Searchable
    GroupTitle    string    `json:"group_title" gorm:"index"`      // Searchable
    ChannelName   string    `json:"channel_name" gorm:"size:512"`  // Searchable
    ChannelNumber string    `json:"channel_number"`                // Searchable (tvg_chno)
    // ... other fields
}
```

### EpgProgram

Existing model, searchable fields:

```go
// internal/models/epg_program.go
type EpgProgram struct {
    ID          ulid.ULID `json:"id" gorm:"type:char(26);primaryKey"`
    SourceID    ulid.ULID `json:"source_id"`
    ChannelID   string    `json:"channel_id" gorm:"index"`     // Searchable
    Title       string    `json:"title" gorm:"size:512"`       // Searchable
    SubTitle    string    `json:"sub_title"`                   // Searchable
    Description string    `json:"description" gorm:"type:text"` // Searchable
    Category    string    `json:"category" gorm:"index"`       // Searchable
    // ... other fields
}
```

## New Entities

### ContainerCodecCompatibility (Runtime Configuration)

Not a database entity - defined as Go constants for relay routing decisions.

```go
// internal/relay/codec_compatibility.go
package relay

// ContainerFormat represents output container formats
type ContainerFormat string

const (
    ContainerMPEGTS ContainerFormat = "mpegts"
    ContainerHLSTS  ContainerFormat = "hls-ts"
    ContainerHLSFMP4 ContainerFormat = "hls-fmp4"
    ContainerDASH   ContainerFormat = "dash"
)

// CodecCompatibility maps containers to supported codecs
var CodecCompatibility = map[ContainerFormat]map[string]bool{
    ContainerMPEGTS: {
        // Video
        "h264": true, "avc": true, "avc1": true,
        "h265": true, "hevc": true, "hvc1": true, "hev1": true,
        // Audio
        "aac": true, "mp4a": true,
        "mp3": true,
        "ac3": true, "ec3": true, "eac3": true,
    },
    ContainerHLSTS: {
        // Same as MPEG-TS (HLS-TS uses MPEG-TS segments)
        "h264": true, "avc": true, "avc1": true,
        "h265": true, "hevc": true, "hvc1": true, "hev1": true,
        "aac": true, "mp4a": true,
        "mp3": true,
        "ac3": true, "ec3": true, "eac3": true,
    },
    ContainerHLSFMP4: {
        // fMP4 supports all codecs
        "h264": true, "avc": true, "avc1": true,
        "h265": true, "hevc": true, "hvc1": true, "hev1": true,
        "vp9": true, "av1": true,
        "aac": true, "mp4a": true,
        "mp3": true,
        "ac3": true, "ec3": true, "eac3": true,
        "opus": true,
    },
    ContainerDASH: {
        // Same as HLS-fMP4
        "h264": true, "avc": true, "avc1": true,
        "h265": true, "hevc": true, "hvc1": true, "hev1": true,
        "vp9": true, "av1": true,
        "aac": true, "mp4a": true,
        "mp3": true,
        "ac3": true, "ec3": true, "eac3": true,
        "opus": true,
    },
}

// IsCodecCompatible checks if a codec is compatible with a container
func IsCodecCompatible(container ContainerFormat, codec string) bool {
    if compat, ok := CodecCompatibility[container]; ok {
        return compat[strings.ToLower(codec)]
    }
    return false
}

// AreCodecsCompatible checks if all codecs are compatible with a container
func AreCodecsCompatible(container ContainerFormat, codecs []string) bool {
    for _, codec := range codecs {
        if !IsCodecCompatible(container, codec) {
            return false
        }
    }
    return true
}
```

## Frontend Types

### Helper Configuration (TypeScript)

```typescript
// frontend/src/lib/expression-constants.ts

// @dynamic() helper for client detection expressions
export const CLIENT_DETECTION_HELPERS: Helper[] = [
  {
    name: 'dynamic',
    prefix: '@dynamic(',
    description: 'Access request data dynamically',
    example: '@dynamic(request.headers, user-agent)',
    completion: {
      type: 'static',
      options: [
        {
          label: 'request.headers',
          value: 'request.headers',
          description: 'HTTP request headers'
        },
        {
          label: 'request.query',
          value: 'request.query',
          description: 'URL query parameters'
        },
        {
          label: 'request.path',
          value: 'request.path',
          description: 'URL path segments'
        }
      ]
    }
  }
];

// Sub-completions for @dynamic(request.headers, ...)
export const HEADER_COMPLETIONS = [
  { label: 'user-agent', value: 'user-agent', description: 'Client User-Agent string' },
  { label: 'accept', value: 'accept', description: 'Accepted content types' },
  { label: 'accept-language', value: 'accept-language', description: 'Preferred languages' },
  { label: 'x-forwarded-for', value: 'x-forwarded-for', description: 'Client IP address' },
  { label: 'x-real-ip', value: 'x-real-ip', description: 'Real client IP' },
  { label: 'host', value: 'host', description: 'Request host header' },
  { label: 'referer', value: 'referer', description: 'Referring URL' }
];
```

### Fuzzy Search Result (TypeScript)

```typescript
// frontend/src/lib/fuzzy-search.ts

export interface FuzzySearchResult<T> {
  item: T;
  score: number;       // 0 = perfect match, 1 = no match
  matches?: Array<{
    key: string;       // Field that matched
    value: string;     // Matched value
    indices: number[][]; // Character positions of matches
  }>;
}

export interface FuzzySearchOptions {
  keys: Array<{
    name: string;
    weight: number;
  }>;
  threshold?: number;  // 0-1, default 0.4
  distance?: number;   // Max characters to search, default 100
  minMatchCharLength?: number; // Min chars to match, default 2
}
```

## Database Migration

### Migration 013: System Client Detection Rules

```go
// internal/database/migrations/migration_013_system_client_detection_rules.go

func migration013SystemClientDetectionRules() Migration {
    return Migration{
        Version:     "013",
        Description: "Add system client detection rules for popular media players",
        Up: func(tx *gorm.DB) error {
            return createSystemClientDetectionRules(tx)
        },
        Down: func(tx *gorm.DB) error {
            return tx.Where("is_system = ?", true).Delete(&models.ClientDetectionRule{}).Error
        },
    }
}

func createSystemClientDetectionRules(tx *gorm.DB) error {
    rules := []models.ClientDetectionRule{
        // Direct players with specific codec support
        {
            Name:        "VLC Media Player",
            Description: "VLC player on desktop and mobile platforms",
            Expression:  `@dynamic(request.headers, user-agent) contains "VLC" OR @dynamic(request.headers, user-agent) contains "LibVLC"`,
            Priority:    100,
            IsEnabled:   models.BoolPtr(true),
            IsSystem:    true,
            VideoCodecs: []string{"h264", "h265"},
            AudioCodecs: []string{"aac", "ac3", "eac3", "mp3"},
            OutputFormat: "mpegts",
        },
        {
            Name:        "MPV Media Player",
            Description: "MPV player with wide codec support",
            Expression:  `@dynamic(request.headers, user-agent) contains "mpv" OR @dynamic(request.headers, user-agent) contains "libmpv"`,
            Priority:    100,
            IsEnabled:   models.BoolPtr(true),
            IsSystem:    true,
            VideoCodecs: []string{"h264", "h265", "av1", "vp9"},
            AudioCodecs: []string{"aac", "ac3", "eac3", "opus", "mp3"},
            OutputFormat: "mpegts",
        },
        {
            Name:        "Kodi Media Center",
            Description: "Kodi/XBMC media center",
            Expression:  `@dynamic(request.headers, user-agent) contains "Kodi" OR @dynamic(request.headers, user-agent) contains "XBMC"`,
            Priority:    100,
            IsEnabled:   models.BoolPtr(true),
            IsSystem:    true,
            VideoCodecs: []string{"h264", "h265", "av1", "vp9"},
            AudioCodecs: []string{"aac", "ac3", "eac3", "dts", "mp3"},
            OutputFormat: "mpegts",
        },
        // Media servers - passthrough configuration
        {
            Name:        "Plex Media Server",
            Description: "Plex server - passthrough for server-side transcoding",
            Expression:  `@dynamic(request.headers, user-agent) contains "Plex"`,
            Priority:    90,
            IsEnabled:   models.BoolPtr(true),
            IsSystem:    true,
            VideoCodecs: []string{}, // Empty = passthrough
            AudioCodecs: []string{}, // Empty = passthrough
            OutputFormat: "",        // Empty = source format
        },
        {
            Name:        "Jellyfin Media Server",
            Description: "Jellyfin server - passthrough for server-side transcoding",
            Expression:  `@dynamic(request.headers, user-agent) contains "Jellyfin"`,
            Priority:    90,
            IsEnabled:   models.BoolPtr(true),
            IsSystem:    true,
            VideoCodecs: []string{},
            AudioCodecs: []string{},
            OutputFormat: "",
        },
        {
            Name:        "Emby Media Server",
            Description: "Emby server - passthrough for server-side transcoding",
            Expression:  `@dynamic(request.headers, user-agent) contains "Emby"`,
            Priority:    90,
            IsEnabled:   models.BoolPtr(true),
            IsSystem:    true,
            VideoCodecs: []string{},
            AudioCodecs: []string{},
            OutputFormat: "",
        },
    }

    for _, rule := range rules {
        // Check if rule already exists (by name)
        var existing models.ClientDetectionRule
        if err := tx.Where("name = ? AND is_system = ?", rule.Name, true).First(&existing).Error; err == nil {
            // Already exists, skip
            continue
        }
        if err := tx.Create(&rule).Error; err != nil {
            return err
        }
    }
    return nil
}
```

## Entity Relationships

```
┌─────────────────────┐
│ ClientDetectionRule │
├─────────────────────┤
│ id                  │
│ name                │
│ expression          │◄── Uses @dynamic() helper
│ is_system           │◄── True for default rules
│ video_codecs        │
│ audio_codecs        │
│ output_format       │
└─────────────────────┘
         │
         │ Evaluated at runtime
         ▼
┌─────────────────────┐
│ HTTP Request        │
├─────────────────────┤
│ headers.user-agent  │◄── Matched by expression
│ headers.accept      │
│ query.*             │
│ path.*              │
└─────────────────────┘
         │
         │ Determines routing
         ▼
┌─────────────────────┐
│ Relay Routing       │
├─────────────────────┤
│ ContainerFormat     │◄── From OutputFormat or source
│ Video/Audio Codecs  │◄── Checked for compatibility
│ Route Decision      │◄── passthrough/remux/transcode
└─────────────────────┘
```

## State Transitions

### Client Detection Rule States

```
┌──────────┐    create     ┌─────────┐
│          │──────────────►│ Enabled │
│  (none)  │               │ (new)   │
│          │               └────┬────┘
└──────────┘                    │
                                │ disable
                                ▼
                           ┌─────────┐
                           │Disabled │
                           │         │
                           └────┬────┘
                                │ enable
                                ▼
                           ┌─────────┐
                           │ Enabled │
                           │(toggled)│
                           └─────────┘

System rules: Cannot be deleted, only disabled
```

### Relay Routing Decision Flow

```
┌─────────────┐
│ Source      │
│ Stream      │
└──────┬──────┘
       │
       ▼
┌─────────────────────────┐
│ Check Profile           │
│ NeedsTranscode()?       │
└──────┬──────────────────┘
       │
       ├──► Yes ──► RouteTranscode
       │
       ▼ No
┌─────────────────────────┐
│ Check Format Match      │
│ source == client?       │
└──────┬──────────────────┘
       │
       ├──► Yes ──► RoutePassthrough
       │
       ▼ No
┌─────────────────────────┐     NEW
│ Check Codec Compat      │◄────────
│ with target container?  │
└──────┬──────────────────┘
       │
       ├──► Yes ──► RouteRepackage
       │
       ▼ No
┌─────────────────────────┐
│ RouteTranscode          │
│ (fallback)              │
└─────────────────────────┘
```

## Validation Rules

### ClientDetectionRule
- `name`: required, max 255 chars, unique for non-system rules
- `expression`: required, must pass expression validation
- `priority`: 0-1000, higher = evaluated first
- `is_system`: if true, cannot be deleted via API

### Fuzzy Search
- Minimum query length: 2 characters
- Maximum results per page: 500 (channels), 200 (EPG)
- Threshold: 0.4 (40% match required)
