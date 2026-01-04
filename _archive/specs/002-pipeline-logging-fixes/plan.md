# Implementation Plan: Pipeline Logging, Error Feedback, and M3U/XMLTV Generation

**Branch**: `002-pipeline-logging-fixes` | **Date**: 2025-12-02 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/002-pipeline-logging-fixes/spec.md`

## Summary

Enhance the existing pipeline orchestration with comprehensive logging at all stages, implement UI error feedback via the existing SSE progress system, ensure artifact cleanup on both success and failure, and validate that the pipeline produces valid M3U and XMLTV output files. This feature builds on the existing pipeline infrastructure in `internal/pipeline/` and progress service in `internal/service/progress/`.

## Technical Context

**Language/Version**: Go 1.25.x (latest stable per constitution)
**Primary Dependencies**: Huma v2.34+ (HTTP), Chi (router), GORM v2 (ORM), slog (logging)
**Storage**: SQLite with GORM (existing)
**Testing**: testify + gomock (existing test infrastructure)
**Target Platform**: Linux server (primary), cross-platform support
**Project Type**: web (Go backend + React/TypeScript frontend)
**Performance Goals**:
- Pipeline execution < 2 min for 50k channels (per constitution)
- Progress updates within 500ms of stage transitions
- Memory < 500 MB during generation
**Constraints**:
- Memory-bounded processing with streaming patterns
- Atomic database transactions for consistency
- Cleanup must handle crashes (startup cleanup)
**Scale/Scope**: 100k+ channels, millions of EPG entries

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Memory-First Design | PASS | Pipeline already uses streaming, adding cleanup |
| II. Modular Pipeline Architecture | PASS | Using existing stage interface, adding logging |
| III. Test-First Development | PENDING | Tests must be written before implementation |
| IV. Clean Architecture (SOLID) | PASS | Repository pattern exists, services isolated |
| V. Idiomatic Go | PASS | Using slog, context, error handling patterns |
| VI. Observable and Debuggable | ENHANCING | This feature improves observability |
| VII. Security by Default | PASS | File ops already sandboxed via storage.Sandbox |
| VIII. No Magic Strings | CHECK | Ensure new log messages use constants |
| IX. Resilient HTTP Clients | N/A | No new external HTTP clients needed |
| X. Human-Readable Duration | N/A | No new duration configs |
| XI. Human-Readable Byte Size | N/A | No new size configs |
| XII. Production-Grade CI/CD | PASS | Existing CI handles tests/lint |
| XIII. Test Data Standards | PASS | Fictional data generators exist |

**Constitution Gate: PASS (conditional on Test-First adherence)**

## Project Structure

### Documentation (this feature)

```text
specs/002-pipeline-logging-fixes/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (API changes)
└── tasks.md             # Phase 2 output
```

### Source Code (repository root)

```text
# Backend (Go)
internal/
├── pipeline/
│   ├── core/
│   │   ├── orchestrator.go      # MODIFY: enhanced logging, cleanup
│   │   ├── interfaces.go        # EXISTING: Stage interface
│   │   └── artifact.go          # EXISTING: artifact management
│   ├── shared/
│   │   └── progress.go          # MODIFY: enhanced progress reporting
│   └── stages/
│       ├── generatem3u/stage.go    # MODIFY: add logging, validation
│       ├── generatexmltv/stage.go  # MODIFY: add logging, validation
│       └── publish/stage.go        # MODIFY: artifact cleanup
├── service/
│   ├── source_service.go        # EXISTING: atomic ingestion (done)
│   ├── proxy_service.go         # MODIFY: error surfacing
│   └── progress/
│       ├── types.go             # MODIFY: add error details
│       └── service.go           # EXISTING: SSE broadcast
├── observability/
│   └── logger.go                # EXISTING: structured logging utilities
└── startup/
    └── cleanup.go               # NEW: orphaned temp cleanup

pkg/
├── m3u/                         # EXISTING: M3U writer
└── xmltv/                       # EXISTING: XMLTV writer

# Frontend (React/TypeScript)
frontend/
├── src/
│   ├── components/
│   │   ├── stream-sources.tsx       # MODIFY: error indicators
│   │   ├── stream-proxies.tsx       # MODIFY: error indicators
│   │   └── progress/                # MODIFY: error notifications and styling
│   │       └── (progress components)
│   └── providers/
│       └── ProgressProvider.tsx # EXISTING: SSE handling
└── tests/
```

**Structure Decision**: Web application with Go backend and React frontend. Changes are primarily modifications to existing components rather than new packages.

## Complexity Tracking

> No constitution violations requiring justification.

| Pattern | Justification |
|---------|---------------|
| Startup cleanup service | Required by FR-012 for crash recovery |
| Enhanced progress events | Required by FR-015 for UI error feedback |

## Phases Overview

### Phase 0: Research
- Review m3u-proxy reference codebase logging patterns
- Analyze existing pipeline stage logging gaps
- Document cleanup strategies for temp directories

### Phase 1: Design
- Define enhanced progress event structure for errors
- Define startup cleanup interface
- Document logging standards per stage

### Phase 2: Tasks (via /speckit.tasks)
- Task breakdown with dependencies
- Test-first task ordering
