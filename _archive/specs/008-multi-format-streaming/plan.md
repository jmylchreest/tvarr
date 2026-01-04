# Implementation Plan: Multi-Format Streaming Support

**Branch**: `008-multi-format-streaming` | **Date**: 2025-12-07 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/008-multi-format-streaming/spec.md`

## Summary

Extend relay mode to support HLS and DASH output formats in addition to MPEG-TS, with query-parameter-driven format selection (`?format=hls|dash|mpegts|auto`). Segments are served from an in-memory ring buffer, with playlists/manifests generated dynamically. Container-aware codec selection enables VP9/AV1/Opus for DASH output. Passthrough proxy mode caches and rebroadcasts upstream HLS/DASH sources with URL rewriting.

## Technical Context

**Language/Version**: Go 1.25.x (latest stable)
**Primary Dependencies**: Huma v2.34+ (Chi router), GORM v2, FFmpeg (external binary), gohlslib v2, go-astits
**Storage**: SQLite/PostgreSQL/MySQL (configurable via GORM) - existing relay_profiles table
**Testing**: testify + table-driven tests, integration tests with FFmpeg
**Target Platform**: Linux server (primary), macOS (secondary)
**Project Type**: Web application (Go backend + Next.js frontend)
**Performance Goals**: <500ms segment delivery latency, <100MB memory per stream with default settings
**Constraints**: Segment storage memory-only, no disk I/O for live segments, backwards-compatible MPEG-TS default
**Scale/Scope**: 10+ concurrent clients per stream, single-quality live streaming (no ABR)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

### Pre-Design Check (Phase 0)

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Memory-First Design | PASS | Ring buffer with configurable capacity, oldest-segment eviction |
| II. Modular Pipeline Architecture | PASS | Format handlers as composable pipeline stages |
| III. Test-First Development | MUST COMPLY | Integration tests for HLS/DASH output, segment buffer tests |
| IV. Clean Architecture (SOLID) | PASS | Interface-based format handlers, service layer separation |
| V. Idiomatic Go | PASS | Context propagation, error returns, defer cleanup |
| VI. Observable and Debuggable | PASS | Structured logging (slog), segment delivery metrics |
| VII. Security by Default | PASS | URL validation, path traversal prevention in segment URLs |
| VIII. No Magic Strings | PASS | Format constants, content-type constants |
| IX. Resilient HTTP Clients | PASS | Use existing pkg/httpclient for upstream HLS/DASH |
| X. Human-Readable Duration Config | PASS | Segment duration configuration via pkg/duration |
| XI. Human-Readable Byte Size Config | PASS | Buffer size configuration via pkg/bytesize |
| XII. Production-Grade CI/CD | PASS | Existing pipeline supports new tests |
| XIII. Test Data Standards | PASS | Fictional channel names in tests |

**Pre-Design Gate Result**: PASS

### Post-Design Check (Phase 1)

| Principle | Status | Verification |
|-----------|--------|--------------|
| I. Memory-First Design | PASS | SegmentBuffer uses ring buffer pattern with configurable MaxSegments, MaxBufferSize; data-model.md defines size limits |
| II. Modular Pipeline Architecture | PASS | OutputHandler interface enables pluggable format handlers; FormatRouter separates routing from handling |
| III. Test-First Development | PASS | Test files defined in project structure; unit tests for SegmentBuffer, integration tests for HLS/DASH |
| IV. Clean Architecture (SOLID) | PASS | OutputHandler interface (ISP), format-specific handlers (SRP), service layer validation (DI) |
| V. Idiomatic Go | PASS | Context in all handlers, error returns, atomic operations in SegmentBuffer |
| VI. Observable and Debuggable | PASS | SessionStats includes SegmentBufferStats; API contract defines /relay/sessions/{id}/stats |
| VII. Security by Default | PASS | Segment sequence validation prevents path traversal; input validation on format param |
| VIII. No Magic Strings | PASS | Constants defined: ContentTypeHLSPlaylist, QueryParamFormat, FormatValueHLS, etc. |
| IX. Resilient HTTP Clients | PASS | Passthrough proxy reuses existing pkg/httpclient with circuit breakers |
| X. Human-Readable Duration Config | PASS | SegmentDuration field uses integer seconds (consistent with existing config pattern) |
| XI. Human-Readable Byte Size Config | PASS | MaxBufferSize uses int64 bytes (consistent with existing buffer config) |
| XII. Production-Grade CI/CD | PASS | No new build requirements; tests integrate with existing pipeline |
| XIII. Test Data Standards | PASS | All examples use fictional proxyId/channelId placeholders |

**Post-Design Gate Result**: PASS - All constitution requirements verified in design artifacts.

## Project Structure

### Documentation (this feature)

```text
specs/008-multi-format-streaming/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
# Web application structure (existing)
internal/
├── models/
│   └── relay_profile.go     # Existing - add OutputFormat type, codec validation
├── relay/
│   ├── session.go           # Existing - add format-aware output routing
│   ├── cyclic_buffer.go     # Existing - already suitable for segments
│   ├── hls_collapser.go     # Existing - HLS input (not output)
│   ├── segment_buffer.go    # NEW - segment-aware ring buffer with metadata
│   ├── hls_muxer.go         # NEW - HLS playlist generator + segment server
│   ├── dash_muxer.go        # NEW - DASH manifest generator + segment server
│   └── format_router.go     # NEW - format selection logic (query param + auto)
├── ffmpeg/
│   └── wrapper.go           # Existing - add HLS/DASH segment output flags
├── http/handlers/
│   └── relay_stream.go      # Existing - add ?format= param handling
└── service/
    └── relay_service.go     # Existing - codec/format validation

frontend/
└── src/
    └── components/
        └── relay-profile/   # Existing - add container-aware codec dropdowns

tests/
├── integration/
│   └── relay_hls_test.go    # NEW - HLS output integration tests
│   └── relay_dash_test.go   # NEW - DASH output integration tests
└── unit/
    └── segment_buffer_test.go  # NEW - segment buffer unit tests
```

**Structure Decision**: Extends existing web application structure. New files added to internal/relay/ for format-specific muxers. Frontend components updated for container-aware codec selection.

## Complexity Tracking

> No violations to justify - design follows existing patterns.
