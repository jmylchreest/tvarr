# Implementation Plan: FFmpeg Relay and Stream Transcoding Proxy

**Branch**: `005-ffmpeg-relay-transcoding` | **Date**: 2025-12-05 | **Spec**: [spec.md](spec.md)

## Summary

Implement three stream delivery modes via proxy endpoints:
- **Redirect**: HTTP 302 to original source URL
- **Proxy**: Fetch upstream, add CORS headers, optional HLS collapse
- **Transcode**: On-demand FFmpeg with cyclic buffer for multi-client sharing

Key components: FFmpeg process management, cyclic buffer, hardware acceleration detection, relay profiles, client tracking, and error fallback streams.

## Technical Context

**Language/Version**: Go 1.25.x
**Primary Dependencies**: Huma v2.34+ (Chi router), GORM v2, FFmpeg (external binary)
**Storage**: SQLite/PostgreSQL/MySQL (configurable via GORM)
**Testing**: testify + gomock, TDD mandatory
**Target Platform**: Linux server (primary), macOS, Docker
**Performance Goals**: Stream playback < 3s, 10+ concurrent transcode sessions
**Constraints**: FFmpeg required in PATH for transcode mode

## Constitution Check

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Memory-First Design | PASS | Cyclic buffer with configurable size limits per session |
| II. Modular Pipeline | PASS | Relay manager orchestrates FFmpeg processes independently |
| III. Test-First Development | PASS | TDD mandatory per constitution |
| IV. Clean Architecture | PASS | Handler → Service → Manager layers |
| V. Idiomatic Go | PASS | Context cancellation for FFmpeg lifecycle |
| VI. Observable | PASS | Health metrics, client tracking, structured logging |
| VII. Security | PASS | Upstream credentials hidden in proxy mode |
| VIII. No Magic Strings | PASS | Constants for codecs, modes, headers |

## Project Structure

### Documentation

```text
specs/005-ffmpeg-relay-transcoding/
├── plan.md              # This file
├── research.md          # Phase 0 research
├── data-model.md        # Entity definitions
├── quickstart.md        # User guide
├── contracts/
│   └── relay-api.yaml   # OpenAPI spec
└── tasks.md             # Implementation tasks (Phase 2)
```

### Source Code

```text
internal/
├── models/
│   ├── relay_profile.go     # Extend with transcoding settings
│   └── stream_proxy.go      # Mode field (redirect/proxy/transcode)
├── relay/
│   ├── manager.go           # FFmpeg process lifecycle
│   ├── session.go           # Active relay session state
│   ├── cyclic_buffer.go     # Ring buffer for multi-client
│   ├── client.go            # Connected client tracking
│   ├── hw_detect.go         # Hardware acceleration detection
│   ├── hls_collapse.go      # HLS variant selection
│   └── fallback.go          # Error stream generator
├── http/handlers/
│   ├── relay_stream.go      # Stream endpoint (mode dispatch)
│   ├── relay_profile.go     # Profile CRUD
│   └── relay_session.go     # Session/client info API
└── services/
    └── relay_service.go     # Relay orchestration

frontend/src/components/relay/
├── RelayHealth.tsx          # Process health display
├── RelayClients.tsx         # Connected client list
└── RelayProfileForm.tsx     # Profile configuration
```

## Implementation Phases

### Phase A: Stream Delivery Modes (FR-001 to FR-003)

**Redirect Mode**:
- Handler returns HTTP 302 with Location header
- No session creation, zero overhead

**Proxy Mode**:
- Fetch upstream via `pkg/httpclient`
- Add CORS headers (`Access-Control-Allow-Origin: *`)
- Handle OPTIONS preflight (FR-016)
- Stream response to client

**Transcode Mode**:
- Dispatch to FFmpeg relay manager
- Create/join session via cyclic buffer

### Phase B: FFmpeg Process Management (FR-004 to FR-007)

**Session Lifecycle**:
- Spawn FFmpeg on first client request
- Multiple clients join existing session
- Track client count per session
- Terminate after idle timeout (configurable, default 60s)
- Background cleanup task (FR-023, every 10s)

**Cyclic Buffer**:
- Ring buffer storing TS segments with sequence numbers
- Each client tracks independent read position
- Configurable buffer size (default 10MB)
- Handle slow clients (skip to latest if too far behind)

### Phase C: Hardware Acceleration (FR-008 to FR-010)

**Detection at Startup**:
- Probe for VAAPI, NVENC, QSV, AMF, VideoToolbox
- Store available accelerators in memory
- Expose via API for profile configuration

**Fallback Logic**:
- If requested HW unavailable, fall back to software encoding
- Log warning when fallback occurs

### Phase D: Relay Profiles (FR-011 to FR-013, FR-022)

**Video Codecs**: H264, H265, AV1, MPEG2, copy (passthrough)
**Audio Codecs**: AAC, MP3, AC3, EAC3, DTS, copy
**Settings**: Video bitrate, audio bitrate, resolution
**Advanced**: Custom FFmpeg argument override

**Profile CRUD**:
- Create, read, update, delete profiles
- Assign profiles to proxies/channels
- System default profiles (passthrough, 720p, 1080p)

### Phase E: HLS Collapsing (FR-014)

**For Proxy Mode**:
- Parse master playlist to find variants
- Select highest bandwidth variant
- Rewrite playlist to single variant
- Pass through to client

**For Transcode Mode**:
- FFmpeg handles input selection
- Profile controls output quality

### Phase F: CORS Handling (FR-015, FR-016)

**Headers for Proxy/Transcode**:
```
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, OPTIONS
Access-Control-Allow-Headers: Content-Type, Accept, Range
Access-Control-Expose-Headers: Content-Length, Content-Range
```

**Preflight**:
- Handle OPTIONS requests
- Return 204 with CORS headers

### Phase G: Error Handling & Fallback (FR-017, FR-018)

**Error Detection**:
- Monitor FFmpeg stderr for error patterns
- Circuit breaker with configurable threshold
- Activate fallback after N consecutive errors

**Fallback Stream**:
- Pre-generate TS segment with "Stream Unavailable" slate
- Loop fallback while in error state
- Auto-recover when upstream returns

### Phase H: Monitoring & Client Tracking (FR-019 to FR-021)

**Relay Health API**:
- Active session count
- Per-session: CPU, memory, uptime, error state
- In-fallback indicator

**Client Tracking**:
- IP address, user agent
- Connection duration, bytes served
- Buffer position
- Stale client cleanup (30s timeout)

**Dashboard**:
- List active relays with health indicators
- Drill-down to connected clients
- Profile assignment UI

## Key Technical Decisions

| Decision | Rationale |
|----------|-----------|
| Mode on StreamProxy | Cleaner than profile-level mode; profile = transcoding settings only |
| Pre-generated fallback TS | Avoid FFmpeg spawn during error conditions |
| Cyclic buffer in memory | Low latency, clients share single FFmpeg output |
| HW detection at startup | Avoid repeated probing; cache results |
| CORS per-endpoint | Only streaming endpoints need CORS, not admin API |

## Success Criteria Mapping

| Criteria | Implementation |
|----------|---------------|
| SC-001: Mode switching | StreamProxy.ProxyMode field, handler dispatch |
| SC-002: Shared FFmpeg | Cyclic buffer, session join logic |
| SC-003: < 3s playback | FFmpeg pre-warming, buffer priming |
| SC-004: 10 concurrent | Session manager with connection limits |
| SC-005: 50% HW reduction | Hardware encoder selection in profile |
| SC-006: < 1s join latency | Read from buffer immediately |
| SC-007: < 5s dashboard | Polling API, React Query refresh |
| SC-008: 30s recovery | Circuit breaker reset, health check |
| SC-009: 90s idle cleanup | Background goroutine, configurable |
| SC-010: Immediate profile | New connections use updated profile |
