# Implementation Plan: Fix EPG Timezone Normalization and Canvas Layout

**Branch**: `017-fix-epg-timezone-canvas` | **Date**: 2025-12-14 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/017-fix-epg-timezone-canvas/spec.md`

## Summary

Fix EPG timezone handling to properly normalize program times to UTC during ingestion, and refactor the canvas rendering to use dynamic time-to-pixel mapping that responds correctly to viewport resize. Additional improvements include removing the unnecessary timezone dropdown, implementing lazy loading for infinite scroll, and comprehensive search across all EPG fields.

**Technical Approach**: Improve the existing Canvas implementation by replacing the hardcoded `PIXELS_PER_HOUR` constant with a dynamically calculated value, storing scroll position as time rather than pixels, and adding proper resize handling. Backend changes focus on applying the inverse timezone offset during ingestion to normalize all times to UTC.

## Technical Context

**Language/Version**: Go 1.25.x (backend), TypeScript/Next.js (frontend)
**Primary Dependencies**: Huma v2.34+ (API), GORM v2 (ORM), React 19, HTML5 Canvas
**Storage**: SQLite/PostgreSQL/MySQL (configurable via GORM)
**Testing**: testify + gomock (backend), Jest/React Testing Library (frontend)
**Target Platform**: Web browsers (Chrome, Firefox, Safari, Edge), viewport 768px-2560px
**Project Type**: Web application (Go backend + Next.js frontend)
**Performance Goals**: 500ms resize/search response, 60fps canvas scroll, Canvas performance already proven good
**Constraints**: Minimum 50px/hour for readability, sidebar width must not overlap canvas
**Scale/Scope**: Thousands of programs, hundreds of channels, 48+ hours of EPG data via lazy loading

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Memory-First Design | PASS | Lazy loading prevents unbounded EPG data in memory |
| II. Modular Pipeline Architecture | PASS | Canvas metrics calculation as separate hook/module |
| III. Test-First Development | PASS | Unit tests for timezone normalization, integration tests for resize behavior |
| IV. Clean Architecture with SOLID | PASS | Service layer handles normalization, handler layer for API |
| V. Idiomatic Go | PASS | Standard Go patterns for backend changes |
| VI. Observable and Debuggable | PASS | Structured logging for timezone detection, no emojis |
| VII. Security by Default | PASS | No security concerns (EPG is read-only display data) |
| VIII. No Magic Strings or Literals | PASS | Replace hardcoded 200px with calculated value |
| IX. Resilient HTTP Clients | PASS | Existing HTTP client for EPG fetching |
| X. Human-Readable Duration | N/A | No new duration configuration |
| XI. Human-Readable Byte Size | N/A | No byte size configuration |
| XII. Production-Grade CI/CD | PASS | Existing CI pipeline |
| XIII. Test Data Standards | PASS | Use fictional channel names in tests |

**Gate Status**: PASS - No violations requiring justification.

## Project Structure

### Documentation (this feature)

```text
specs/017-fix-epg-timezone-canvas/
├── plan.md              # This file
├── spec.md              # Feature specification
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (OpenAPI)
│   └── openapi.yaml
└── tasks.md             # Phase 2 output (created by /speckit.tasks)
```

### Source Code (repository root)

```text
# Backend (Go)
internal/
├── service/
│   ├── epg_service.go           # Timezone normalization logic
│   └── epg_service_test.go      # Normalization tests
├── ingestion/
│   ├── xmltv_handler.go         # XMLTV timezone detection & normalization
│   ├── xtream_epg_handler.go    # Xtream timezone detection & normalization
│   └── *_test.go                # Handler tests
├── http/handlers/
│   └── epg_handler.go           # EPG API endpoints (lazy loading support)
└── models/
    └── epg_source.go            # EPG source model (detected_timezone, epg_shift)

# Frontend (TypeScript/React)
frontend/
├── src/
│   ├── components/
│   │   └── epg/
│   │       └── CanvasEPG.tsx    # Canvas component with dynamic metrics
│   ├── hooks/
│   │   └── useCanvasMetrics.ts  # New: Dynamic time-to-pixel calculation
│   ├── app/
│   │   └── epg/
│   │       └── page.tsx         # EPG page (remove timezone dropdown)
│   └── lib/
│       └── api-client.ts        # API client for lazy loading
└── tests/
    └── epg/
        └── canvas-metrics.test.ts  # Canvas metric calculation tests
```

**Structure Decision**: Web application with Go backend + Next.js frontend. Changes span both backend (timezone normalization during ingestion) and frontend (canvas rendering, lazy loading, search).

## Complexity Tracking

> No violations requiring justification. All changes align with existing architecture.
