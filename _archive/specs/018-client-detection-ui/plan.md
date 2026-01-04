# Implementation Plan: Client Detection UI Improvements

**Branch**: `018-client-detection-ui` | **Date**: 2025-12-15 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/018-client-detection-ui/spec.md`

## Summary

This feature addresses multiple UI and backend improvements:
1. **Export/Import Fix (P1)**: Fix 405 error caused by URL mismatch between frontend API client and backend routes
2. **Expression Editor Enhancement (P2)**: Add intellisense, autocomplete, and validation badges to client detection rule editor
3. **Copyable Expressions (P2)**: Enable copy-to-clipboard for expressions in list view
4. **Smart Remuxing (P2)**: Improve relay logic to remux instead of transcode when codecs are container-compatible
5. **Fuzzy Search (P2)**: Add fuzzy/partial matching to channel and EPG browsers
6. **Default System Rules (P3)**: Add pre-configured rules for VLC, MPV, Kodi, Plex, Jellyfin, Emby

## Technical Context

**Language/Version**: Go 1.25.x (backend), TypeScript/Next.js 16.x (frontend)
**Primary Dependencies**:
- Backend: Huma v2.34+ (Chi router), GORM v2, FFmpeg (external binary)
- Frontend: React 19, shadcn/ui, Tailwind CSS v4
**Storage**: SQLite (default), PostgreSQL/MySQL (configurable via GORM)
**Testing**: Go testify + gomock (backend), Jest/Vitest (frontend)
**Target Platform**: Linux server (primary), Docker containers
**Project Type**: Web application (backend + frontend)
**Performance Goals**:
- Search results within 500ms
- Autocomplete suggestions within 500ms
- Validation badges update within 1s
**Constraints**:
- Memory < 500MB for operations
- API response < 200ms for lists
**Scale/Scope**: 100k+ channels, 1M+ EPG programs

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Memory-First Design | ✅ Pass | Fuzzy search uses server-side pagination, no unbounded collections |
| II. Modular Pipeline Architecture | ✅ Pass | Expression editor components are composable, relay logic uses staged pipeline |
| III. Test-First Development | ✅ Pass | Will write tests before implementation for all changes |
| IV. Clean Architecture (SOLID) | ✅ Pass | Reusing existing expression editor infrastructure, extending via interfaces |
| V. Idiomatic Go | ✅ Pass | Following existing patterns in codebase |
| VI. Observable and Debuggable | ✅ Pass | Structured logging with slog, no emojis |
| VII. Security by Default | ✅ Pass | Input validation at API boundaries, no path traversal concerns |
| VIII. No Magic Strings | ✅ Pass | Will use constants for URLs, codec names |
| IX. Resilient HTTP Clients | ✅ Pass | Using existing httpclient infrastructure |
| X. Human-Readable Duration | N/A | No duration config in this feature |
| XI. Human-Readable Byte Size | N/A | No byte size config in this feature |
| XII. Production-Grade CI/CD | ✅ Pass | Using existing CI/CD pipeline |
| XIII. Test Data Standards | ✅ Pass | Will use fictional broadcaster names in tests |

**Gate Status**: ✅ PASSED - No violations requiring justification

## Project Structure

### Documentation (this feature)

```text
specs/018-client-detection-ui/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
└── tasks.md             # Phase 2 output (via /speckit.tasks)
```

### Source Code (repository root)

```text
backend/
├── internal/
│   ├── http/handlers/
│   │   └── export.go           # Export/import handlers (URL fix not needed - routes correct)
│   ├── relay/
│   │   ├── routing_decision.go # Smart remux logic enhancement
│   │   └── ffmpeg_transcoder.go
│   ├── database/migrations/
│   │   └── migration_013_*.go  # New migration for system rules
│   └── models/
│       └── client_detection_rule.go
└── tests/

frontend/
├── src/
│   ├── components/
│   │   ├── client-detection-rules.tsx      # Main target for enhancement
│   │   ├── client-detection-expression-editor.tsx  # NEW: specialized editor
│   │   ├── expression-editor.tsx           # Base component (reuse)
│   │   ├── autocomplete-popup.tsx          # Reuse
│   │   └── expression-validation-badges.tsx # Reuse
│   ├── hooks/
│   │   └── useHelperAutocomplete.ts        # Reuse with @dynamic() helper
│   ├── lib/
│   │   ├── api-client.ts                   # Fix export/import URLs
│   │   └── expression-constants.ts         # Add client detection helpers
│   └── app/
│       ├── channels/page.tsx               # Add fuzzy search
│       └── epg/page.tsx                    # Add fuzzy search
└── tests/
```

**Structure Decision**: Web application with separate backend (Go) and frontend (Next.js/React). Following existing patterns in the codebase.

## Complexity Tracking

> No constitution violations requiring justification.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| N/A | N/A | N/A |
