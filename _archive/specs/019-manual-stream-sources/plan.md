# Implementation Plan: Manual Stream Sources & UI/UX Overhaul

**Branch**: `019-manual-stream-sources` | **Date**: 2025-01-10 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/019-manual-stream-sources/spec.md`

## Summary

This feature completes the manual stream sources implementation (backend API) and introduces a comprehensive UI/UX overhaul across the application. The backend work provides REST API endpoints for managing manual channel definitions within manual-type stream sources, including M3U import/export. The frontend work establishes consistent design patterns (Master-Detail layouts, inline data tables, wizard flows) to replace the current sheet-based editing approach.

## Technical Context

**Language/Version**: Go 1.25.x (backend), TypeScript/Next.js 16.x (frontend)
**Primary Dependencies**:
- Backend: Huma v2.34+ (API), Chi (router), GORM v2 (ORM)
- Frontend: React 19, shadcn/ui, Tailwind CSS v4, @tanstack/react-table
**Storage**: SQLite/PostgreSQL/MySQL (configurable via GORM) - existing `manual_stream_channels` table
**Testing**:
- Backend: Go testing + testify + gomock
- Frontend: Jest + React Testing Library
**Target Platform**: Linux/macOS/Windows server (Docker primary), Web browser (Chrome, Firefox, Safari)
**Project Type**: Web application (Go backend + Next.js frontend)
**Performance Goals**:
- API response < 200ms for list operations
- CRUD operations < 500ms for up to 100 channels per source
- Frontend rendering < 100ms for 100-row tables
**Constraints**:
- Memory-efficient M3U parsing (streaming for large files)
- No breaking changes to existing API contracts
- Maintain backwards compatibility with existing manual source data
**Scale/Scope**:
- Up to 100 channels per manual source (typical)
- Up to 1000 channels per manual source (maximum supported)
- 10+ entity types affected by UI overhaul

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Memory-First Design | PASS | M3U parsing will use streaming; manual channels limited per source |
| II. Modular Pipeline Architecture | PASS | Manual handler already exists as pipeline stage |
| III. Test-First Development | MUST ENFORCE | Tests required before implementation |
| IV. Clean Architecture with SOLID | PASS | Repository pattern exists; new service layer follows pattern |
| V. Idiomatic Go | PASS | Standard Go patterns will be used |
| VI. Observable and Debuggable | PASS | slog logging, no emojis |
| VII. Security by Default | PASS | Input validation at API boundary; path traversal N/A |
| VIII. No Magic Strings | PASS | Constants for content types, error messages |
| IX. Resilient HTTP Clients | N/A | No external HTTP calls for manual channels |
| X. Human-Readable Duration | N/A | No duration configs in this feature |
| XI. Human-Readable Byte Size | N/A | No byte size configs in this feature |
| XII. Production-Grade CI/CD | PASS | Existing pipeline handles new code |
| XIII. Test Data Standards | PASS | Fictional channel names in tests |

**Gate Status**: PASS - No violations. Proceed to Phase 0.

## Project Structure

### Documentation (this feature)

```text
specs/019-manual-stream-sources/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (OpenAPI specs)
└── tasks.md             # Phase 2 output (created by /speckit.tasks)
```

### Source Code (repository root)

```text
# Backend (Go)
internal/
├── http/handlers/
│   ├── manual_channel.go        # NEW: Manual channel API endpoints
│   └── types.go                 # Extend with manual channel types
├── service/
│   └── manual_channel_service.go # NEW: Manual channel business logic
├── m3u/
│   ├── parser.go                # NEW: M3U parser
│   └── generator.go             # NEW: M3U generator
├── models/
│   └── manual_stream_channel.go # EXISTS: Model already defined
├── repository/
│   └── manual_channel_repo.go   # EXISTS: Repository already defined
└── ingestor/
    └── manual_handler.go        # EXISTS: Materializes to channels table

# Frontend (TypeScript/React)
frontend/src/
├── components/
│   ├── shared/                  # NEW: Shared UI patterns
│   │   ├── data-table/
│   │   │   ├── DataTable.tsx
│   │   │   ├── columns.tsx
│   │   │   └── pagination.tsx
│   │   ├── inline-edit-table/
│   │   │   └── InlineEditTable.tsx
│   │   ├── layouts/
│   │   │   ├── MasterDetailLayout.tsx
│   │   │   └── WizardLayout.tsx
│   │   └── feedback/
│   │       ├── EmptyState.tsx
│   │       └── SkeletonTable.tsx
│   ├── manual-channel-editor.tsx    # REFACTOR: Replace with InlineEditTable
│   ├── stream-sources.tsx           # REFACTOR: Use MasterDetailLayout
│   ├── epg-sources.tsx              # REFACTOR: Use MasterDetailLayout
│   ├── filters.tsx                  # REFACTOR: Use MasterDetailLayout
│   ├── data-mapping.tsx             # REFACTOR: Use MasterDetailLayout
│   ├── encoding-profiles.tsx        # REFACTOR: Use MasterDetailLayout
│   ├── client-detection-rules.tsx   # REFACTOR: Use MasterDetailLayout
│   ├── CreateProxyModal.tsx         # REFACTOR: Use WizardLayout
│   └── logos.tsx                    # REFACTOR: Gallery with side panel
└── lib/
    └── api-client.ts                # EXTEND: Manual channel API methods

# Tests
internal/
├── http/handlers/
│   └── manual_channel_test.go   # NEW
├── service/
│   └── manual_channel_service_test.go # NEW
└── m3u/
    ├── parser_test.go           # NEW
    └── generator_test.go        # NEW

frontend/src/
├── components/shared/__tests__/ # NEW: Shared component tests
└── components/__tests__/        # UPDATE: Component tests
```

**Structure Decision**: Web application with Go backend and Next.js frontend. Backend follows existing Clean Architecture patterns with repository/service/handler layers. Frontend introduces new shared component library for consistent UI patterns.

## Complexity Tracking

> No constitution violations requiring justification.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| N/A | N/A | N/A |
