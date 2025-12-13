# Quickstart: Relay Profile Simplification

**Feature Branch**: `014-relay-profile-simplify`
**Date**: 2025-12-12

## Overview

This guide helps developers quickly understand and implement the relay profile simplification feature.

## Key Changes Summary

| Component | Change | Impact |
|-----------|--------|--------|
| RelayProfile | Replaced by EncodingProfile | 40+ fields → 6 fields |
| RelayProfileMapping | Removed | Built-in detection rules instead |
| StreamProxy | relay_profile_id → encoding_profile_id | Foreign key rename |
| Default Mode | Changed to "smart" | Better out-of-box experience |
| Smart Defaults | Pre-select sources/filters | Fewer clicks to create proxy |

## New Model: EncodingProfile

```go
// internal/models/encoding_profile.go
type EncodingProfile struct {
    BaseModel

    Name             string        `json:"name"`
    Description      string        `json:"description,omitempty"`
    TargetVideoCodec VideoCodec    `json:"target_video_codec"` // h264, h265, vp9, av1
    TargetAudioCodec AudioCodec    `json:"target_audio_codec"` // aac, opus, ac3, eac3, mp3
    QualityPreset    QualityPreset `json:"quality_preset"`     // low, medium, high, ultra
    HWAccel          HWAccelType   `json:"hw_accel"`           // auto, none, cuda, vaapi, qsv
    IsDefault        bool          `json:"is_default"`
    IsSystem         bool          `json:"is_system"`
    Enabled          bool          `json:"enabled"`
}
```

## Quality Presets

| Preset | CRF | Max Bitrate | Audio | Use Case |
|--------|-----|-------------|-------|----------|
| low | 28 | 2 Mbps | 128k | Mobile, low bandwidth |
| medium | 23 | 5 Mbps | 192k | General streaming |
| high | 20 | 10 Mbps | 256k | High-quality viewing |
| ultra | 16 | Unlimited | 320k | Maximum quality |

## Client Detection Priority

1. **Explicit headers** (priority 1-8): `X-Video-Codec`, `X-Audio-Codec`
2. **Format override** (11-20): Query parameters
3. **X-Tvarr-Player** (21-30): Custom player header
4. **Accept header** (31-40): MIME type preferences
5. **User-Agent** (50+): Browser/player detection
6. **Default** (999): H.264/AAC in MPEG-TS

## API Examples

### Create Encoding Profile

```bash
curl -X POST http://localhost:8080/api/v1/encoding-profiles \
  -H "Content-Type: application/json" \
  -d '{
    "name": "High Quality H.265",
    "description": "For smart TVs and modern devices",
    "target_video_codec": "h265",
    "target_audio_codec": "aac",
    "quality_preset": "high",
    "hw_accel": "auto"
  }'
```

### Create Proxy with Profile

```bash
curl -X POST http://localhost:8080/api/v1/stream-proxies \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My Proxy",
    "proxy_mode": "smart",
    "encoding_profile_id": "01ABC123..."
  }'
```

### Request Explicit Codec (Client)

```bash
# Request H.265 video
curl -H "X-Video-Codec: h265" http://localhost:8080/proxy/channel/123.m3u8

# Request H.264 + AAC
curl -H "X-Video-Codec: h264" -H "X-Audio-Codec: aac" \
  http://localhost:8080/proxy/channel/123.m3u8
```

## Migration Guide

### For Existing Installations

1. **Automatic migration**: Database migrations handle schema changes
2. **Backup created**: `relay_profile_mappings_backup.json` in data directory
3. **Check logs**: INFO messages show migrated profiles and removed mappings

### For Custom Integrations

| Old API | New API |
|---------|---------|
| `GET /relay-profiles` | `GET /encoding-profiles` |
| `relay_profile_id` in proxy | `encoding_profile_id` |
| `POST /relay-profile-mappings` | Use `X-Video-Codec` header instead |

## Testing

### Unit Tests

```bash
# Run encoding profile tests
go test ./internal/models/encoding_profile_test.go -v

# Run client detection tests
go test ./internal/expression/dynamic_field_test.go -v
```

### E2E Client Detection Tests

```bash
# Build and run e2e-runner with client detection mode
./cmd/e2e-runner/e2e-runner --test-mode client-detection
```

### Manual Testing

1. Create proxy with default settings (smart mode, no profile)
2. Test with different clients:
   - Chrome → Should get HLS-fMP4
   - VLC → Should get MPEG-TS
   - Safari → Should avoid VP9/Opus
3. Test explicit headers:
   - Add `X-Video-Codec: h265` → Should get H.265
4. Test with encoding profile:
   - Create H.264 profile, assign to proxy → All clients get H.264

## Troubleshooting

### Expression Parse Error

**Error**: `parse error at line 1, column 1: expected field name but got 1`

**Cause**: SQLite converted `true` to `1` in expression

**Fix**: Migration changes expression to `user_agent contains ""`

### Profile Not Applied

**Symptoms**: Clients still getting auto-detected codecs despite profile

**Check**:
1. Is `proxy_mode` set to `smart`? (not `direct`)
2. Is `encoding_profile_id` correctly set?
3. Is the profile `enabled`?

### Missing Client Detection

**Symptoms**: All clients getting same format

**Check**:
1. Verify client detection rules exist: `GET /api/v1/relay-profile-mappings`
2. Check rule priorities (lower = higher priority)
3. Verify User-Agent is being sent correctly

## Files to Modify

### Backend

```
internal/models/encoding_profile.go          # NEW
internal/models/relay_profile.go             # REMOVE after migration
internal/models/relay_profile_mapping.go     # REMOVE
internal/repository/encoding_profile_repo.go # NEW
internal/service/encoding_profile_service.go # NEW
internal/http/handlers/encoding_profile.go   # NEW
internal/database/migrations/migrations.go   # MODIFY
internal/relay/format_router.go              # MODIFY
cmd/e2e-runner/main.go                       # MODIFY
```

### Frontend

```
frontend/src/components/encoding-profile-form.tsx  # NEW
frontend/src/components/proxies.tsx                # MODIFY
frontend/src/components/CreateProxyModal.tsx       # MODIFY
frontend/src/app/admin/encoding-profiles/page.tsx  # NEW
frontend/src/lib/api/encoding-profiles.ts          # NEW
```

## Next Steps

1. Run `/speckit.tasks` to generate implementation tasks
2. Implement tests first (TDD per constitution)
3. Implement backend models and migration
4. Implement API endpoints
5. Update frontend components
6. Update E2E runner
