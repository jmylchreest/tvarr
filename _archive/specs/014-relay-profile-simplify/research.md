# Research: Relay Profile Simplification

**Feature Branch**: `014-relay-profile-simplify`
**Date**: 2025-12-12

## Overview

This document captures research findings to resolve technical questions and inform design decisions for the relay profile simplification feature.

---

## Research 1: Quality Preset to FFmpeg Parameter Mapping

### Question
How should quality presets (low, medium, high, ultra) map to FFmpeg encoding parameters?

### Findings

Based on common industry practices and FFmpeg documentation:

| Preset | Video Bitrate | Video CRF | Audio Bitrate | Use Case |
|--------|--------------|-----------|---------------|----------|
| low | 1-2 Mbps | 28-32 | 96-128 kbps | Mobile/bandwidth-constrained |
| medium | 3-5 Mbps | 23-26 | 128-192 kbps | General streaming |
| high | 6-10 Mbps | 18-22 | 192-256 kbps | High-quality home viewing |
| ultra | CRF-based | 15-18 | 256-320 kbps | Maximum quality |

For H.264/H.265 encoding:
- CRF (Constant Rate Factor) provides better quality per bit than fixed bitrate
- Recommended preset: `medium` for software, `p4` for NVENC
- Profile: `main` or `high` for H.264, `main` for H.265

### Decision
Use CRF-based encoding with bitrate caps. The `ultra` preset uses pure CRF without bitrate limits. Lower presets use CRF with maxrate/bufsize constraints for consistent bitrate.

```go
type QualityPreset string

const (
    QualityPresetLow    QualityPreset = "low"
    QualityPresetMedium QualityPreset = "medium"
    QualityPresetHigh   QualityPreset = "high"
    QualityPresetUltra  QualityPreset = "ultra"
)

// GetEncodingParams returns FFmpeg parameters for the preset
func (p QualityPreset) GetEncodingParams() EncodingParams {
    switch p {
    case QualityPresetLow:
        return EncodingParams{CRF: 28, Maxrate: "2M", Bufsize: "4M", AudioBitrate: "128k"}
    case QualityPresetMedium:
        return EncodingParams{CRF: 23, Maxrate: "5M", Bufsize: "10M", AudioBitrate: "192k"}
    case QualityPresetHigh:
        return EncodingParams{CRF: 20, Maxrate: "10M", Bufsize: "20M", AudioBitrate: "256k"}
    case QualityPresetUltra:
        return EncodingParams{CRF: 16, Maxrate: "", Bufsize: "", AudioBitrate: "320k"}
    default:
        return EncodingParams{CRF: 23, Maxrate: "5M", Bufsize: "10M", AudioBitrate: "192k"}
    }
}
```

### Alternatives Considered
1. **Fixed bitrate only**: Rejected because CRF provides better quality-to-size ratio
2. **User-configurable CRF**: Rejected to maintain simplicity; users needing fine control can use custom FFmpeg flags

---

## Research 2: Database Migration Strategy for RelayProfileMapping Removal

### Question
How should we migrate from RelayProfileMapping to built-in client detection rules without data loss?

### Findings

Current RelayProfileMapping table structure:
- `id`, `name`, `description`, `priority`, `expression`
- `accepted_video_codecs`, `accepted_audio_codecs`, `accepted_containers`
- `preferred_video_codec`, `preferred_audio_codec`, `preferred_container`
- `is_enabled`, `is_system`

Migration approach:
1. **System rules**: Already have equivalent built-in rules; can be dropped
2. **User-created rules**: Should be logged as INFO during migration showing what was removed
3. **Expression format**: Same expression engine, patterns transfer directly

### Decision
Immediate removal with migration that:
1. Logs each removed user rule (name, expression, preferred codecs) as INFO
2. Creates backup JSON file in data directory with all RelayProfileMapping rows
3. Drops the `relay_profile_mappings` table
4. No automatic conversion (built-in detection is functionally equivalent or better)

```go
// Migration pseudo-code
func MigrateRelayProfileMappings(db *gorm.DB, logger *slog.Logger) error {
    // Backup user rules
    var mappings []RelayProfileMapping
    db.Where("is_system = ?", false).Find(&mappings)

    if len(mappings) > 0 {
        // Write backup file
        backupPath := filepath.Join(dataDir, "relay_profile_mappings_backup.json")
        backupJSON, _ := json.MarshalIndent(mappings, "", "  ")
        os.WriteFile(backupPath, backupJSON, 0644)

        for _, m := range mappings {
            logger.Info("Migrating relay profile mapping to built-in detection",
                "name", m.Name,
                "expression", m.Expression,
                "preferred_video", m.PreferredVideoCodec)
        }
    }

    // Drop table
    return db.Migrator().DropTable(&RelayProfileMapping{})
}
```

### Alternatives Considered
1. **Deprecation period**: Rejected per user decision (option A - immediate removal)
2. **Automatic conversion to custom rules**: Rejected; built-in detection covers all use cases

---

## Research 3: RelayProfile to EncodingProfile Migration

### Question
How do we migrate existing RelayProfile configurations to the new EncodingProfile model?

### Findings

Current RelayProfile has 40+ fields. Mapping strategy:

| RelayProfile Field | EncodingProfile Field | Migration Logic |
|--------------------|----------------------|-----------------|
| `name` | `name` | Direct copy |
| `description` | `description` | Direct copy |
| `video_codec` | `target_video_codec` | Direct copy (if not auto/copy) |
| `audio_codec` | `target_audio_codec` | Direct copy (if not auto/copy) |
| `hw_accel` | `hw_accel` | Direct copy |
| `video_bitrate`, `video_crf` | `quality_preset` | Infer from bitrate/CRF values |
| Other fields | N/A | Dropped (use defaults) |

Quality preset inference:
```go
func InferQualityPreset(videoBitrate int, videoCRF int) QualityPreset {
    if videoCRF > 0 {
        if videoCRF <= 18 { return QualityPresetUltra }
        if videoCRF <= 22 { return QualityPresetHigh }
        if videoCRF <= 26 { return QualityPresetMedium }
        return QualityPresetLow
    }
    if videoBitrate > 0 {
        if videoBitrate >= 8000 { return QualityPresetUltra }
        if videoBitrate >= 5000 { return QualityPresetHigh }
        if videoBitrate >= 2000 { return QualityPresetMedium }
        return QualityPresetLow
    }
    return QualityPresetMedium // Default
}
```

### Decision
Create `encoding_profiles` table alongside existing `relay_profiles`, migrate data, then update `stream_proxies.relay_profile_id` to point to new table, and finally drop `relay_profiles`.

### Alternatives Considered
1. **In-place modification**: Rejected; cleaner to create new table and migrate
2. **Keep both models**: Rejected; unnecessary complexity

---

## Research 4: Built-in Client Detection Rules for Explicit Codec Headers

### Question
What built-in rules should be added for `X-Video-Codec` and `X-Audio-Codec` header support?

### Findings

The expression engine already supports `@header_req:<name>` dynamic field resolver. New rules only need to be added to the migrations:

```go
// Priority 1-4: Video codec explicit headers
{Name: "Explicit H.265 Video Request", Priority: 1,
 Expression: `@header_req:X-Video-Codec == "h265"`,
 PreferredVideoCodec: "h265"},
{Name: "Explicit H.264 Video Request", Priority: 2,
 Expression: `@header_req:X-Video-Codec == "h264"`,
 PreferredVideoCodec: "h264"},
{Name: "Explicit VP9 Video Request", Priority: 3,
 Expression: `@header_req:X-Video-Codec == "vp9"`,
 PreferredVideoCodec: "vp9"},
{Name: "Explicit AV1 Video Request", Priority: 4,
 Expression: `@header_req:X-Video-Codec == "av1"`,
 PreferredVideoCodec: "av1"},

// Priority 5-8: Audio codec explicit headers
{Name: "Explicit AAC Audio Request", Priority: 5,
 Expression: `@header_req:X-Audio-Codec == "aac"`,
 PreferredAudioCodec: "aac"},
{Name: "Explicit Opus Audio Request", Priority: 6,
 Expression: `@header_req:X-Audio-Codec == "opus"`,
 PreferredAudioCodec: "opus"},
{Name: "Explicit AC3 Audio Request", Priority: 7,
 Expression: `@header_req:X-Audio-Codec == "ac3"`,
 PreferredAudioCodec: "ac3"},
{Name: "Explicit EAC3 Audio Request", Priority: 8,
 Expression: `@header_req:X-Audio-Codec == "eac3"`,
 PreferredAudioCodec: "eac3"},
```

The existing priority scheme:
- 1-10: Explicit codec headers (NEW)
- 11-20: Format override query parameters
- 21-30: X-Tvarr-Player header
- 31-40: Accept header
- 50+: User-Agent patterns
- 999: Default fallback

### Decision
Add these rules to the migration registry. No code changes to expression engine required - just new data.

### Alternatives Considered
1. **New field resolver for codec headers**: Rejected; `@header_req:` already handles this
2. **Single rule with extraction**: Rejected; discrete rules are simpler and more debuggable

---

## Research 5: Default Fallback Rule Expression Fix

### Question
How should we fix the "Default (Universal)" rule expression issue where `true` becomes `1` in SQLite?

### Findings

The issue: SQLite stores boolean `true` as integer `1`. When read back and passed to the expression parser, it becomes the string `"1"`, which the parser rejects ("expected field name but got 1").

The expression parser DOES support `true` as a keyword:
```go
// parser.go line 182-187
if p.current.Type == TokenTrue {
    p.advance()
    return NewCondition("", OpContains, ""), nil
}
```

But this only works if the string "true" is stored, not "1".

### Decision
Change the migration to use `user_agent contains ""` which:
1. Always matches (empty string is contained in any string)
2. Is unambiguous and doesn't rely on boolean-to-string conversion
3. Is already the recommended tautology pattern in the codebase

Add a data migration to fix existing databases:
```go
// Fix corrupted expressions
db.Model(&RelayProfileMapping{}).
    Where("expression IN (?, ?, ?)", "1", "true", "TRUE").
    Update("expression", `user_agent contains ""`)
```

### Alternatives Considered
1. **Fix SQLite driver to preserve strings**: Too invasive, affects entire system
2. **Change parser to handle "1"**: Violates parser semantics (1 is not a boolean in expression language)

---

## Research 6: EPG Timezone Detection

### Question
How should EPG source timezone be detected and stored?

### Findings

**XMLTV format**: Programme start/stop times include timezone offset:
```xml
<programme start="20251212140000 +0000" stop="20251212150000 +0000">
```
The format is `YYYYMMDDHHmmss ±HHMM`.

**Xtream API format**: Times are typically in UTC or local server time without explicit timezone. Detection requires:
1. Checking if times are reasonable for UTC
2. Looking for timezone hints in API metadata
3. Defaulting to UTC if unknown

### Decision
Add fields to EPG source model:
```go
type EpgSource struct {
    // ... existing fields ...

    // Detected timezone from source (e.g., "+0000", "+0100", "Europe/London")
    SourceTimezone string `json:"source_timezone,omitempty"`

    // UTC offset in minutes (e.g., 0 for UTC, 60 for +0100)
    TimezoneOffsetMinutes int `json:"timezone_offset_minutes"`
}
```

Log timezone detection on creation/update:
```go
logger.Info("EPG source timezone detected",
    "source", source.Name,
    "timezone", source.SourceTimezone,
    "offset_minutes", source.TimezoneOffsetMinutes)
```

### Alternatives Considered
1. **Always assume UTC**: Rejected; some sources provide local time
2. **User-configurable timezone**: Could be added later if auto-detection insufficient

---

## Research 7: E2E Runner Client Detection Test Mode

### Question
What test scenarios should the E2E runner support for client detection?

### Findings

Based on ER-001 to ER-006 requirements:

1. **Header injection tests**:
   - `X-Video-Codec: h264`, `X-Video-Codec: h265`, `X-Video-Codec: vp9`, `X-Video-Codec: av1`
   - `X-Audio-Codec: aac`, `X-Audio-Codec: opus`, `X-Audio-Codec: ac3`
   - Combined headers: `X-Video-Codec: h264` + `X-Audio-Codec: aac`

2. **User-Agent detection tests**:
   - VLC: `VLC/3.0.18 LibVLC/3.0.18`
   - Chrome: `Mozilla/5.0 ... Chrome/120.0.0.0`
   - Safari: `Mozilla/5.0 ... Safari/605.1.15`
   - Firefox: `Mozilla/5.0 ... Firefox/121.0`
   - Kodi: `Kodi/20.0`
   - IPTV client: `GSE IPTV/7.3`

3. **Priority ordering tests**:
   - Header > User-Agent: Send both, verify header wins
   - Invalid header fallthrough: `X-Video-Codec: invalid`, verify User-Agent used

4. **Profile override tests**:
   - Assign encoding profile, send different header, verify profile wins

### Decision
Add `--test-mode client-detection` flag to e2e-runner:
```go
type ClientDetectionTest struct {
    Name           string
    Headers        map[string]string
    UserAgent      string
    ExpectedCodec  string
    ExpectedFormat string
}

func RunClientDetectionTests(baseURL string) []TestResult {
    tests := []ClientDetectionTest{
        {Name: "Explicit H.265 header", Headers: map[string]string{"X-Video-Codec": "h265"}, ExpectedCodec: "h265"},
        {Name: "VLC User-Agent", UserAgent: "VLC/3.0.18", ExpectedFormat: "mpegts"},
        // ... more tests
    }
    // Execute and verify
}
```

### Alternatives Considered
1. **Separate binary**: Rejected; e2e-runner already handles similar testing
2. **Unit tests only**: Insufficient; need real HTTP round-trips

---

## Research 8: StreamProxy Smart Defaults for Pre-selection

### Question
How should the proxy creation form pre-select sources and filters by default?

### Findings

Current behavior: User must manually select sources and filters.
Desired behavior: All available sources and system filters selected by default.

Frontend implementation (CreateProxyModal.tsx):
```typescript
// On form open, pre-populate with all sources
useEffect(() => {
    if (isOpen && sources) {
        setSelectedSources(sources.map(s => s.id));
    }
}, [isOpen, sources]);

// Pre-select all system filters
useEffect(() => {
    if (isOpen && filters) {
        const systemFilters = filters.filter(f => f.is_system);
        setSelectedFilters(systemFilters.map(f => f.id));
    }
}, [isOpen, filters]);
```

Backend: No changes needed; frontend controls selection.

### Decision
Modify frontend only:
1. `CreateProxyModal.tsx`: Pre-select all sources and system filters when form opens
2. Set `proxy_mode` default to `smart` (already in model defaults)
3. No encoding profile by default (auto-detection handles most cases)

### Alternatives Considered
1. **Backend returns defaults**: Unnecessary; frontend has all info needed
2. **Separate "quick create" button**: Could be added later; basic form with pre-selection covers most cases

---

## Summary of Decisions

| Topic | Decision |
|-------|----------|
| Quality Presets | CRF-based with bitrate caps; 4 presets (low/medium/high/ultra) |
| RelayProfileMapping | Immediate removal with backup JSON file |
| RelayProfile Migration | Create new EncodingProfile table, migrate, drop old |
| Codec Header Rules | Add to migration registry, priorities 1-8 |
| Default Rule Fix | Use `user_agent contains ""` instead of `true` |
| EPG Timezone | Auto-detect from XMLTV format, log on creation |
| E2E Testing | Add `--test-mode client-detection` flag |
| Smart Defaults | Frontend pre-selects sources/filters on form open |
