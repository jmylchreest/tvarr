# Specification: CMAF + Smart Delivery Architecture

**Feature ID**: 009-cmaf-smart-delivery
**Priority**: P1
**Status**: Draft

## Problem Statement

The current relay system has complexity that confuses users and limits codec support:

1. **3 Proxy Modes**: Users must understand redirect/proxy/relay distinctions
2. **OutputFormat in Profile**: Conflates container (TS/fMP4) with manifest (HLS/DASH)
3. **Many System Profiles**: Overwhelming choices for common use cases
4. **No VP9/AV1 Support**: Modern codecs require fMP4 containers

## Solution

Simplify to a 2-mode system with intelligent defaults:

| Mode | User Intent | System Behavior |
|------|-------------|-----------------|
| **Direct** | "Don't process, just redirect" | HTTP 302 to source |
| **Smart** | "Deliver optimally" | Auto-select passthrough/repackage/transcode |

Enable CMAF (Common Media Application Format) for:
- Single fMP4 segment serves both HLS v7+ and DASH
- VP9/AV1/Opus codec support
- Better seeking and caching

## User Stories

### US1: Simplified Proxy Configuration
**As a** tvarr user
**I want** to choose between "Direct" and "Smart" delivery
**So that** I don't need to understand technical details of proxy/relay modes

**Acceptance Criteria:**
- Proxy mode dropdown shows 2 options with clear descriptions
- "Direct" sends 302 redirect (zero processing)
- "Smart" automatically optimizes based on source and client

### US2: Modern Codec Support
**As a** user with modern devices
**I want** to use VP9/AV1/Opus codecs
**So that** I get better quality at lower bitrates

**Acceptance Criteria:**
- VP9, AV1 video codecs available in profile
- Opus audio codec available in profile
- System automatically uses fMP4 container for these codecs
- Validation prevents invalid codec/container combinations

### US3: Universal Profile
**As a** new user
**I want** a default profile that works everywhere
**So that** I don't need to configure anything to get started

**Acceptance Criteria:**
- "Universal" profile ships as default (H.264/AAC)
- Works on all devices (iOS, Android, browsers, TVs)
- Container auto-selection logic:
  - fMP4 for: DASH requests, HLS requests from modern browsers (Chrome, Firefox, Edge, Safari 10+)
  - MPEG-TS for: explicit `?format=mpegts`, legacy User-Agents, or when client Accept header requests `video/MP2T`

### US4: CMAF for Efficient Delivery
**As a** server operator
**I want** single encoded segments to serve HLS and DASH clients
**So that** I reduce storage/CPU overhead

**Acceptance Criteria:**
- fMP4 segments generated once
- HLS v7 playlist references fMP4 segments
- DASH MPD references same fMP4 segments
- Both formats served from single buffer

## Technical Requirements

### Container Format Logic

```
Profile.ContainerFormat = "auto" (default)
    │
    ├─ Codec requires fMP4? (VP9/AV1/Opus)
    │   └─ Yes → fMP4
    │
    ├─ Client explicitly requests MPEG-TS? (?format=mpegts)
    │   └─ Yes → MPEG-TS
    │
    ├─ Client format preference?
    │   ├─ DASH request → fMP4
    │   ├─ HLS request → fMP4 (modern default, EXT-X-VERSION:7)
    │   └─ Raw TS request → MPEG-TS
    │
    └─ Default → fMP4 (modern default)
```

**Note**: HLS v7+ capability is not reliably detectable via HTTP headers. The system defaults to fMP4 for all HLS requests (modern players handle this). Users with legacy-only devices should explicitly request `?format=mpegts`.

### Smart Delivery Decision Tree

```
Smart Mode Request
    │
    ├─ Profile set?
    │   ├─ Yes: Needs transcode? (codec != copy)
    │   │   ├─ Yes → FFmpeg pipeline
    │   │   └─ No → Smart passthrough
    │   └─ No: Smart passthrough
    │
    └─ Smart Passthrough:
        ├─ Source HLS, Client HLS → Passthrough (proxy segments)
        ├─ Source DASH, Client DASH → Passthrough (proxy segments)
        ├─ Source HLS, Client DASH → Repackage (same segments, different manifest)
        ├─ Source DASH, Client HLS → Repackage (same segments, different manifest)
        ├─ Source TS, Client HLS → FFmpeg pipeline (no segments to reuse)
        ├─ Source TS, Client DASH → FFmpeg pipeline (no segments to reuse)
        └─ Source TS, Client TS → Passthrough (direct proxy)
```

**Note**: "Repackage" only works when source already has segments (HLS/DASH). Raw TS streams require FFmpeg to create segments, even with `copy` codecs.

### System Profiles

| Profile | Video | Audio | Container | Use Case |
|---------|-------|-------|-----------|----------|
| Universal | H.264 (libx264) | AAC | auto | Default, all devices |
| Passthrough | copy | copy | auto | Zero CPU, source unchanged |
| Efficiency | HEVC (libx265) | AAC | fMP4 | Modern devices (Apple 10+, Chrome, smart TVs 2018+), smaller files |

## API Changes

### StreamProxy Mode Values

```json
// Before
{ "proxy_mode": "redirect" | "proxy" | "relay" }

// After
{ "proxy_mode": "direct" | "smart" }
```

### RelayProfile Schema

```json
// Before
{
  "video_codec": "libx264",
  "audio_codec": "aac",
  "output_format": "hls"  // REMOVED
}

// After
{
  "video_codec": "libx264",
  "audio_codec": "aac",
  "container_format": "auto"  // NEW: "auto" | "fmp4" | "mpegts"
}
```

### Backwards Compatibility

- `proxy_mode: "redirect"` → maps to `"direct"`
- `proxy_mode: "proxy"` → maps to `"smart"`
- `proxy_mode: "relay"` → maps to `"smart"`
- `output_format` → ignored (container derived from codec)

## Out of Scope

- ABR (Adaptive Bitrate) with multiple quality renditions
- Per-request codec negotiation (codecs set at profile level)
- WebRTC output format
- DRM/encryption

## Success Metrics

1. User configuration reduced from 3 modes to 2
2. System profiles reduced from N to 3
3. VP9/AV1 codecs functional
4. CMAF serving both HLS and DASH from single encode
5. No regression in MPEG-TS compatibility
