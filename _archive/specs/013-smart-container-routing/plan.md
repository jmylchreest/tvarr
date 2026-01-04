# Implementation Plan: Smart Container Routing

**Branch**: `013-smart-container-routing` | **Date**: 2025-12-10 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/013-smart-container-routing/spec.md`

## Summary

Implement intelligent stream format routing that uses gohlslib Muxer for pure HLS repackaging (avoiding FFmpeg overhead when no transcoding needed), adds X-Tvarr-Player headers to frontend players (mpegts.js, hls.js) for better client detection, and routes streams based on source format × client preference × codec compatibility. Only invoke FFmpeg pipeline when actual transcoding is required.

## Technical Context

**Language/Version**: Go 1.25.x (backend), TypeScript/React (frontend)
**Primary Dependencies**: gohlslib v2 (HLS Client + Muxer), Huma v2.34+ (API), mpegts.js (frontend player), hls.js (future HLS playback)
**Storage**: N/A (stateless routing decisions)
**Testing**: testify + gomock (Go), existing frontend test patterns
**Target Platform**: Linux server (backend), modern browsers (frontend)
**Project Type**: Web application (Go backend + Next.js frontend)
**Performance Goals**: <10% CPU for passthrough vs FFmpeg, <2s stream start time, 3x concurrent capacity for passthrough
**Constraints**: Must maintain backwards compatibility with existing relay profiles
**Scale/Scope**: Support existing channel load (100k+ channels), optimize for HLS-to-HLS majority case

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Memory-First Design | PASS | Passthrough reuses source segments without buffering; repackaging streams data |
| II. Modular Pipeline Architecture | PASS | Format router is a composable stage; handlers implement common interface |
| III. Test-First Development | PASS | Tests required for routing logic, format detection, header injection |
| IV. Clean Architecture (SOLID) | PASS | FormatRouter + OutputHandler interfaces already exist |
| V. Idiomatic Go | PASS | Context propagation, error handling via returns |
| VI. Observable and Debuggable | PASS | FR-009 requires logging routing decisions |
| VII. Security by Default | PASS | No new security surface (header injection is client-side) |
| VIII. No Magic Strings | PASS | Use constants for header names, format values |
| IX. Resilient HTTP Clients | PASS | Existing circuit breaker infrastructure |
| X. Human-Readable Duration | N/A | No new duration configs |
| XI. Human-Readable Byte Size | N/A | No new byte size configs |
| XII. Production-Grade CI/CD | PASS | Existing pipeline covers backend + frontend |
| XIII. Test Data Standards | PASS | Use fictional channel names in tests |

**Gate Status**: PASS - No violations requiring justification

## Project Structure

### Documentation (this feature)

```text
specs/013-smart-container-routing/
├── plan.md              # This file
├── relay-decision-flow.md # Routing decision flowchart (already created)
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
# Backend (Go)
internal/
├── relay/
│   ├── format_router.go       # MODIFY: Add X-Tvarr-Player detection, detection_mode handling
│   ├── client_detector.go     # NEW: Client capability detection from headers
│   ├── routing_decision.go    # NEW: RoutingDecision type and logic
│   ├── hls_muxer.go          # NEW: gohlslib Muxer wrapper for HLS output
│   ├── hls_collapser.go      # EXISTING: gohlslib Client (already uses v2)
│   └── constants.go          # MODIFY: Add X-Tvarr-Player header constant
├── models/
│   └── relay_profile.go      # MODIFY: Add detection_mode field if not present

# Frontend (TypeScript/React)
frontend/
├── src/
│   ├── player/
│   │   ├── MpegTsAdapter.ts  # MODIFY: Add X-Tvarr-Player header injection
│   │   └── HlsAdapter.ts     # NEW: HLS.js adapter with header injection (future)
│   ├── lib/
│   │   └── player-headers.ts # NEW: Constants and utilities for player headers
│   └── components/
│       └── video-player-modal.tsx # MODIFY: Pass headers to player

# Tests
internal/relay/
├── format_router_test.go      # MODIFY: Add tests for new detection logic
├── client_detector_test.go    # NEW: Client detection tests
├── routing_decision_test.go   # NEW: Routing decision tests
└── hls_muxer_test.go         # NEW: HLS Muxer output tests

frontend/src/player/
└── MpegTsAdapter.test.ts      # MODIFY: Header injection tests
```

**Structure Decision**: Follows existing web application pattern with Go backend in `internal/` and Next.js frontend in `frontend/`. New files follow existing naming conventions.

## Complexity Tracking

> No violations requiring justification - design follows existing patterns.

| Aspect | Decision | Rationale |
|--------|----------|-----------|
| Client detection | Single detector module | Consolidates header parsing logic |
| Routing decision | Enum + struct | Matches existing StreamMode pattern |
| HLS Muxer | Wrapper around gohlslib | gohlslib already in use for Client |

