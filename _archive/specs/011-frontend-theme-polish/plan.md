# Implementation Plan: Frontend Theme Polish

**Branch**: `011-frontend-theme-polish` | **Date**: 2025-12-08 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/011-frontend-theme-polish/spec.md`

## Summary

This feature addresses three main areas: (1) fixing the white "flashbang" effect during dark mode page navigation by ensuring theme CSS is applied before first paint, (2) enabling custom theme file support from the `$DATA/themes/` directory with validation and caching, and (3) auditing and standardizing component styling consistency across the application.

The technical approach involves:
- Synchronous inline script execution for immediate theme application before React hydration
- Backend API endpoint to serve custom themes with proper validation and caching headers
- Frontend theme provider enhancement to load custom themes dynamically
- Component style audit and standardization using shadcn/ui variables

## Technical Context

**Language/Version**: Go 1.25.x (backend), TypeScript/Next.js 16.x (frontend)
**Primary Dependencies**: Huma v2.34+ (API), React 19, shadcn/ui, Tailwind CSS v4
**Storage**: File system for custom themes (`$DATA/themes/`), localStorage for user preferences
**Testing**: Go test (backend), visual regression testing (frontend)
**Target Platform**: Web browser (Chrome, Firefox, Safari, Edge)
**Project Type**: Web application (Go backend + Next.js frontend)
**Performance Goals**: Theme selector loads <500ms, zero white flashes during navigation
**Constraints**: Must work with existing shadcn/ui components, CSP-compatible inline scripts
**Scale/Scope**: ~15 pages, ~50 UI components, unlimited custom themes

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Memory-First Design | PASS | Theme CSS files are small (<10KB each), loaded on-demand |
| II. Modular Pipeline Architecture | N/A | Frontend feature, no pipeline involvement |
| III. Test-First Development | PASS | Visual regression tests + unit tests for validation |
| IV. Clean Architecture (SOLID) | PASS | Theme service separated from handlers |
| V. Idiomatic Go | PASS | Backend follows Go conventions |
| VI. Observable and Debuggable | PASS | Structured logging for theme loading |
| VII. Security by Default | PASS | File access sandboxed to $DATA/themes/ |
| VIII. No Magic Strings | PASS | CSS variable names as constants |
| IX. Resilient HTTP Clients | N/A | No external HTTP calls for themes |
| X. Human-Readable Duration | N/A | No duration configuration |
| XI. Human-Readable Byte Size | N/A | No byte size configuration |
| XII. Production-Grade CI/CD | PASS | Frontend builds in existing pipeline |
| XIII. Test Data Standards | N/A | No test data with channel names |

## Project Structure

### Documentation (this feature)

```text
specs/011-frontend-theme-polish/
├── spec.md              # Feature specification
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (API contracts)
└── tasks.md             # Phase 2 output
```

### Source Code (repository root)

```text
# Backend (Go)
internal/
├── http/handlers/
│   └── theme.go                    # NEW: Theme API handlers
├── service/
│   └── theme_service.go            # NEW: Theme validation & loading
└── assets/
    └── themes/                     # Built-in themes (embedded)

# Frontend (Next.js)
frontend/
├── src/
│   ├── app/
│   │   └── layout.tsx              # MODIFY: Enhanced theme script
│   ├── components/
│   │   ├── enhanced-theme-provider.tsx  # MODIFY: Custom theme support
│   │   ├── enhanced-theme-selector.tsx  # MODIFY: Custom theme display
│   │   └── ui/                          # AUDIT: Component consistency
│   ├── lib/
│   │   └── enhanced-theme-script.ts     # MODIFY: FOUC prevention
│   └── providers/
│       └── PageLoadingProvider.tsx      # MODIFY: Skeleton placeholders
└── public/
    └── themes/                     # Built-in themes (static)
```

**Structure Decision**: This is a web application with Go backend and Next.js frontend. The feature touches both layers - backend for custom theme serving and frontend for theme application and UI consistency.

## Complexity Tracking

No constitution violations requiring justification.
