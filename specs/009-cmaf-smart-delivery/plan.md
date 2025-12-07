# Plan: CMAF + Smart Delivery Architecture

**Feature ID**: 009-cmaf-smart-delivery
**Status**: Draft
**Dependencies**: 008-multi-format-streaming (completed phases 1-6)

## Overview

This plan unifies several improvements into a cohesive architecture:

1. **CMAF Support** - Single fMP4 segment format serves both HLS v7+ and DASH
2. **Smart Delivery** - Simplified 2-mode proxy system (Direct/Smart)
3. **Profile Refactor** - Remove OutputFormat, add ContainerFormat with codec-aware defaults
4. **Reduced System Profiles** - Ship 2-3 universal profiles instead of many

## Goals

- Simplify user experience (fewer choices, smarter defaults)
- Support modern codecs (VP9, AV1, Opus) via fMP4
- Single encode serves all clients (HLS and DASH)
- Reduce CPU waste through intelligent passthrough decisions

---

## Part 1: Data Model Changes

### 1.1 StreamProxy Mode Simplification

**Before:**
```go
type StreamProxyMode string
const (
    StreamProxyModeRedirect StreamProxyMode = "redirect"
    StreamProxyModeProxy    StreamProxyMode = "proxy"
    StreamProxyModeRelay    StreamProxyMode = "relay"
)
```

**After:**
```go
type StreamProxyMode string
const (
    StreamProxyModeDirect StreamProxyMode = "direct"  // HTTP 302 redirect
    StreamProxyModeSmart  StreamProxyMode = "smart"   // Intelligent delivery
)
```

**Migration:**
- `redirect` → `direct` (1:1 mapping)
- `proxy` → `smart` (passthrough behavior preserved)
- `relay` → `smart` (transcoding triggered by profile)

### 1.2 RelayProfile Refactor

**Remove:**
- `OutputFormat` field (was: mpegts/hls/dash)

**Add:**
- `ContainerFormat` field (auto/fmp4/mpegts)

**Before:**
```go
type RelayProfile struct {
    // ...
    OutputFormat OutputFormat  // "mpegts", "hls", "dash"
}
```

**After:**
```go
type RelayProfile struct {
    // ...
    ContainerFormat ContainerFormat  // "auto", "fmp4", "mpegts"
}

type ContainerFormat string
const (
    ContainerFormatAuto   ContainerFormat = "auto"   // System decides based on codec
    ContainerFormatFMP4   ContainerFormat = "fmp4"   // Force fMP4 (CMAF)
    ContainerFormatMPEGTS ContainerFormat = "mpegts" // Force MPEG-TS (legacy)
)
```

### 1.3 Codec → Container Constraints

```
┌─────────────────────────────────────────────────────────────────┐
│ Video Codec          │ MPEG-TS  │ fMP4     │ Auto Default       │
├──────────────────────┼──────────┼──────────┼────────────────────┤
│ copy                 │ ✅       │ ✅       │ Match source       │
│ libx264 (H.264)      │ ✅       │ ✅       │ fMP4 (modern)      │
│ libx265 (HEVC)       │ ✅       │ ✅       │ fMP4 (modern)      │
│ libvpx-vp9 (VP9)     │ ❌       │ ✅       │ fMP4 (required)    │
│ libaom-av1 (AV1)     │ ❌       │ ✅       │ fMP4 (required)    │
│ av1_nvenc            │ ❌       │ ✅       │ fMP4 (required)    │
│ av1_qsv              │ ❌       │ ✅       │ fMP4 (required)    │
├──────────────────────┼──────────┼──────────┼────────────────────┤
│ Audio Codec          │ MPEG-TS  │ fMP4     │ Auto Default       │
├──────────────────────┼──────────┼──────────┼────────────────────┤
│ copy                 │ ✅       │ ✅       │ Match source       │
│ aac                  │ ✅       │ ✅       │ fMP4 (modern)      │
│ libopus (Opus)       │ ❌       │ ✅       │ fMP4 (required)    │
└─────────────────────────────────────────────────────────────────┘
```

### 1.4 Validation Rules

```go
func (p *RelayProfile) BeforeSave(tx *gorm.DB) error {
    // VP9/AV1 require fMP4
    if p.RequiresFMP4() && p.ContainerFormat == ContainerFormatMPEGTS {
        return ErrCodecRequiresFMP4Container
    }
    return nil
}

func (p *RelayProfile) RequiresFMP4() bool {
    return isVP9OrAV1(p.VideoCodec) || isOpus(p.AudioCodec)
}
```

---

## Part 2: Smart Delivery Logic

### 2.1 Decision Flow

```
Client Request → StreamProxy.Mode
                      │
         ┌───────────┴───────────┐
         │                       │
      direct                   smart
         │                       │
    HTTP 302              ┌──────┴──────┐
    to source             │             │
                    Has Profile?    No Profile
                          │             │
                    ┌─────┴─────┐       │
                    │           │       │
              Needs Transcode?  │       │
                    │           │       │
              ┌─────┴─────┐     │       │
              Yes         No   Copy     │
              │           │     │       │
         FFmpeg      Smart Passthrough  │
         Pipeline    (match src→client) │
                          │             │
                    ┌─────┴─────────────┘
                    │
              Source Classification
                    │
         ┌──────────┼──────────┬──────────┐
         │          │          │          │
       HLS       DASH         TS       Other
         │          │          │          │
    Client wants?   │     Client wants?   │
         │          │          │          │
    ┌────┴────┐   ┌─┴──┐   ┌───┴───┐     │
   HLS  DASH  TS  DASH HLS  HLS DASH TS  │
    │    │    │    │    │    │   │   │   │
 Pass  Pkg  Pkg  Pass Pkg  Pkg Pkg Pass FFmpeg
```

### 2.2 Delivery Decisions

```go
type DeliveryDecision int
const (
    DeliveryPassthrough      DeliveryDecision = iota // Same format, no processing
    DeliveryRepackage                                // Different manifest, same segments
    DeliveryTranscode                                // Full FFmpeg pipeline
)

func SelectDelivery(
    source ClassificationResult,
    clientFormat string,
    profile *RelayProfile,
) DeliveryDecision {

    // 1. Profile requires transcoding?
    if profile != nil && profile.NeedsTranscode() {
        return DeliveryTranscode
    }

    // 2. Source format matches client format?
    if sourceMatchesClient(source, clientFormat) {
        return DeliveryPassthrough
    }

    // 3. Can repackage without transcoding?
    if canRepackage(source, clientFormat) {
        return DeliveryRepackage
    }

    // 4. Must transcode
    return DeliveryTranscode
}
```

### 2.3 CMAF Repackaging

When source is TS and client wants HLS/DASH:

```
Source TS → Segment Extractor → fMP4 Muxer → CMAF Segments
                                                   │
                                    ┌──────────────┼──────────────┐
                                    │              │              │
                              HLS v7 Manifest  DASH MPD    Raw Segments
                              (.m3u8 + .m4s)   (.mpd)      (for caching)
```

Single segment storage, multiple manifest views.

---

## Part 3: System Profiles

### 3.1 Reduced Profile Set

**Ship only 2-3 profiles:**

```yaml
# Profile 1: Universal (default for most users)
- name: "Universal"
  description: "Best compatibility - works on all devices"
  video_codec: "libx264"
  audio_codec: "aac"
  container_format: "auto"  # fMP4 for modern, TS for legacy
  video_bitrate: 0          # Copy source bitrate
  preset: "fast"

# Profile 2: Passthrough (zero processing)
- name: "Passthrough"
  description: "No transcoding - fastest, lowest CPU"
  video_codec: "copy"
  audio_codec: "copy"
  container_format: "auto"

# Profile 3: Efficiency (optional, for power users)
# Uses HEVC (libx265) - NOT adaptive. AV1 requires custom profile.
- name: "Efficiency"
  description: "HEVC encoding for smaller files (requires Apple 10+, Chrome, smart TVs 2018+)"
  video_codec: "libx265"
  audio_codec: "aac"
  container_format: "fmp4"
  video_bitrate: 0
  preset: "medium"
```

### 3.2 Profile Selection UI

```
┌─────────────────────────────────────────────────────────────┐
│ Stream Processing                                           │
│                                                             │
│ ○ Direct Link                                               │
│   Send players directly to source URL                       │
│   Best for: Trusted players, zero server load               │
│                                                             │
│ ● Smart Delivery                                            │
│   Automatically optimize stream for each client             │
│                                                             │
│   Quality Profile: [Universal ▼]                            │
│                                                             │
│   ┌─────────────────────────────────────────────────────┐   │
│   │ Universal                                           │   │
│   │ H.264 + AAC - Works on all devices                  │   │
│   ├─────────────────────────────────────────────────────┤   │
│   │ Passthrough                                         │   │
│   │ No transcoding - fastest performance                │   │
│   ├─────────────────────────────────────────────────────┤   │
│   │ Efficiency                                          │   │
│   │ HEVC - Smaller files, modern devices only           │   │
│   ├─────────────────────────────────────────────────────┤   │
│   │ + Create Custom Profile...                          │   │
│   └─────────────────────────────────────────────────────┘   │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## Part 4: FFmpeg Pipeline Changes

### 4.1 Output Format Selection

Update `runFFmpegPipeline()` in session.go to use container format:

```go
// In RelaySession.runFFmpegPipeline() - update output format section
func (s *RelaySession) runFFmpegPipeline() error {
    // ... existing input configuration ...

    // Determine container format (NEW)
    container := s.Profile.DetermineContainer()

    switch container {
    case ContainerFormatFMP4:
        // fMP4 output for CMAF
        builder.OutputFormat("mp4").
            OutputArgs("-movflags", "frag_keyframe+empty_moov+default_base_moof").
            OutputArgs("-frag_duration", "2000000")  // 2 second fragments

    case ContainerFormatMPEGTS:
        // MPEG-TS output (existing behavior)
        builder.MpegtsArgs().
            FlushPackets().
            MuxDelay("0")
    }

    // ... rest of existing pipeline ...
}
```

### 4.2 Segment Generation

```go
// UnifiedBuffer changes for CMAF
type UnifiedBuffer struct {
    // ... existing fields ...
    containerFormat ContainerFormat
    cmafMuxer       *CMAFMuxer  // New: handles fMP4 segmentation
}

func (b *UnifiedBuffer) WriteChunk(data []byte) error {
    if b.containerFormat == ContainerFormatFMP4 {
        // Parse fMP4 and extract segments
        return b.cmafMuxer.ProcessFragment(data)
    }
    // Existing TS segmentation logic
    return b.writeTSChunk(data)
}
```

---

## Part 5: Implementation Tasks

### Phase A: Data Model Migration

- [ ] A1: Add ContainerFormat to RelayProfile model
- [ ] A2: Add migration to convert OutputFormat → ContainerFormat
- [ ] A3: Update RelayProfile validation (codec↔container)
- [ ] A4: Add StreamProxyMode direct/smart constants
- [ ] A5: Add migration for proxy mode conversion
- [ ] A6: Update seed data with new system profiles

### Phase B: Smart Delivery Logic

- [ ] B1: Create DeliveryDecision type and SelectDelivery function
- [ ] B2: Refactor handleRawStream to use smart dispatch
- [ ] B3: Remove handleRawProxyMode (merged into smart)
- [ ] B4: Remove handleRawRelayMode (merged into smart)
- [ ] B5: Add source↔client format matching logic
- [ ] B6: Update session.go runNormalPipeline for new modes

### Phase C: CMAF Implementation

- [ ] C1: Create CMAFMuxer for fMP4 segment creation
- [ ] C2: Update UnifiedBuffer to support fMP4 segments
- [ ] C3: Update HLSHandler for v7 fMP4 playlists (#EXT-X-MAP)
- [ ] C4: Verify DASHHandler works with CMAF segments
- [ ] C5: Add fMP4 output mode to FFmpeg pipeline
- [ ] C6: Integration tests for CMAF output

### Phase D: Frontend Updates

- [ ] D1: Update proxy mode dropdown (2 options)
- [ ] D2: Update profile selector with descriptions
- [ ] D3: Remove OutputFormat from profile form
- [ ] D4: Add ContainerFormat to profile form (advanced)
- [ ] D5: Update codec dropdowns to show container compatibility
- [ ] D6: Add "why fMP4?" help text for modern codecs

### Phase E: Testing & Documentation

- [ ] E1: E2E test for smart delivery mode switching
- [ ] E2: E2E test for CMAF HLS+DASH from same source
- [ ] E3: Update API documentation
- [ ] E4: Update user documentation
- [ ] E5: Migration guide for existing users

---

## Part 6: Migration Strategy

### 6.1 Database Migration

```sql
-- Step 1: Add new columns
ALTER TABLE relay_profiles ADD COLUMN container_format VARCHAR(20) DEFAULT 'auto';

-- Step 2: Migrate OutputFormat to ContainerFormat
UPDATE relay_profiles SET container_format = 'mpegts' WHERE output_format = 'mpegts';
UPDATE relay_profiles SET container_format = 'fmp4' WHERE output_format IN ('hls', 'dash');

-- Step 3: Update proxy modes
UPDATE stream_proxies SET proxy_mode = 'direct' WHERE proxy_mode = 'redirect';
UPDATE stream_proxies SET proxy_mode = 'smart' WHERE proxy_mode IN ('proxy', 'relay');

-- Step 4: Drop old column (after verification)
ALTER TABLE relay_profiles DROP COLUMN output_format;
```

### 6.2 Backwards Compatibility

- API accepts old mode values, internally converts
- Old profiles continue to work (OutputFormat mapped to ContainerFormat)
- Deprecation warnings in logs for old API usage

---

## Part 7: Success Criteria

1. **User Simplicity**: 2 proxy modes instead of 3
2. **Profile Reduction**: 2-3 system profiles cover 95% of use cases
3. **CMAF Working**: Single encode serves HLS and DASH clients
4. **VP9/AV1 Support**: Modern codecs work via fMP4
5. **Zero Regression**: Existing MPEG-TS workflows unchanged
6. **Performance**: Smart passthrough reduces unnecessary transcoding

---

## Appendix: File Changes Summary

| File | Changes |
|------|---------|
| `internal/models/stream_proxy.go` | Mode constants |
| `internal/models/relay_profile.go` | ContainerFormat, remove OutputFormat |
| `internal/database/migrations/` | New migration |
| `internal/database/seed/` | Updated system profiles |
| `internal/http/handlers/relay_stream.go` | Smart delivery dispatch |
| `internal/relay/session.go` | Pipeline selection |
| `internal/relay/cmaf_muxer.go` | New: CMAF segmentation |
| `internal/relay/unified_buffer.go` | fMP4 support |
| `internal/relay/hls_handler.go` | v7 playlist support |
| `internal/ffmpeg/command_builder.go` | fMP4 output args |
| `frontend/src/components/proxy/` | Mode dropdown |
| `frontend/src/components/relay-profile/` | Profile form |
