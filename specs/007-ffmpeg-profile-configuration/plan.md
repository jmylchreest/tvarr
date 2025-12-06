# Implementation Plan: FFmpeg Profile Configuration

**Branch**: `007-ffmpeg-profile-configuration` | **Date**: 2025-12-06 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/007-ffmpeg-profile-configuration/spec.md`

## Summary

Enhance FFmpeg relay profiles with custom flag support, hardware acceleration configuration, profile management (CRUD + clone), profile testing capability, and command preview. The goal is to reduce relay errors and improve feed quality by giving administrators fine-grained control over FFmpeg parameters.

## Technical Context

**Language/Version**: Go 1.25.x (latest stable)
**Primary Dependencies**: Huma v2.34+ (Chi router), GORM v2, FFmpeg (external binary)
**Storage**: SQLite/PostgreSQL/MySQL (configurable via GORM)
**Testing**: testify + gomock, table-driven tests
**Target Platform**: Linux server (primary), macOS, Windows
**Project Type**: Web application (Go backend + Next.js frontend)
**Performance Goals**: Profile test feedback within 30 seconds, hardware detection within 10 seconds
**Constraints**: Custom flags must not introduce command injection vulnerabilities
**Scale/Scope**: Support 100+ relay profiles, multiple hardware acceleration types

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Memory-First Design | PASS | Profile management operates on single entities, not bulk collections |
| II. Modular Pipeline Architecture | PASS | FFmpeg command building is already modular via CommandBuilder |
| III. Test-First Development | PASS | Tests required for profile validation, command building, API handlers |
| IV. Clean Architecture (SOLID) | PASS | Repository pattern for profiles, service layer for validation/testing |
| V. Idiomatic Go | PASS | Using existing patterns from codebase |
| VI. Observable and Debuggable | PASS | Command preview enables debugging, structured logging for errors |
| VII. Security by Default | PASS | Input validation for custom flags to prevent injection |
| VIII. No Magic Strings | PASS | Use constants for flag patterns, error messages |
| IX. Resilient HTTP Clients | N/A | No new external HTTP calls required |
| X. Human-Readable Duration | PASS | Existing profile timeouts use duration format |
| XI. Human-Readable Byte Size | N/A | No new byte size configs |
| XII. Production-Grade CI/CD | PASS | Existing pipeline handles new code |
| XIII. Test Data Standards | PASS | Use fictional channel names in tests |

**Gate Status**: PASS - All applicable principles satisfied

## Project Structure

### Documentation (this feature)

```text
specs/007-ffmpeg-profile-configuration/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   └── openapi.yaml     # API contract additions
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
# Backend
internal/
├── models/
│   └── relay_profile.go    # EXISTING - extend with new fields
├── repository/
│   └── relay_profile_repo.go  # EXISTING - add profile testing queries
├── service/
│   └── relay_profile_service.go  # NEW - profile validation, testing service
├── ffmpeg/
│   ├── wrapper.go          # EXISTING - extend command builder
│   ├── hwaccel.go          # EXISTING - hardware detection
│   └── profile_builder.go  # NEW - profile-to-command translation
├── http/handlers/
│   └── relay_profile.go    # EXISTING - extend with test/preview endpoints

# Frontend
frontend/src/
├── components/
│   └── relay-profiles/     # NEW - profile management components
│       ├── ProfileForm.tsx
│       ├── ProfileTestDialog.tsx
│       ├── CommandPreview.tsx
│       └── HardwareAccelConfig.tsx
├── app/
│   └── relay-profiles/     # NEW - profile management page
│       └── page.tsx
└── types/
    └── relay-profile.ts    # EXISTING - extend types
```

**Structure Decision**: Web application pattern - extends existing backend and frontend structure

## Key Design Decisions

### D1: Custom Flags Field Structure

The RelayProfile model already has `InputOptions`, `OutputOptions`, and `FilterComplex` string fields. These will be:
- **InputOptions**: Raw FFmpeg input flags (whitespace-separated)
- **OutputOptions**: Raw FFmpeg output flags (whitespace-separated)
- **FilterComplex**: Complex filter graph string (-filter_complex)

These are NOT currently wired into the FFmpeg command builder. This feature will connect them.

### D2: Flag Validation Strategy

Custom flags will be validated for:
1. Balanced quotes (no unclosed quotes)
2. No shell metacharacters that could enable injection (`;`, `|`, `&`, `$()`, backticks)
3. Valid FFmpeg flag patterns (starts with `-` or `--`)

Invalid flags generate warnings but allow saving (for advanced users with edge cases).

### D3: Hardware Acceleration Configuration

Extend existing `HWAccel` and `HWAccelDevice` fields with additional decoder-specific options:
- `hwaccel_decoder_options` - decoder-specific flags (e.g., `-c:v h264_cuvid`)
- `hwaccel_output_format` - output format for hwaccel (e.g., `cuda`, `vaapi`)

### D4: Profile Testing Architecture

Profile testing will:
1. Run a short FFmpeg test (30s timeout) against a sample stream
2. Capture FFmpeg stderr for error detection
3. Verify hardware acceleration is active (check for GPU device in output)
4. Return structured test results with suggestions for common fixes

### D5: Command Preview Implementation

Command preview will use the existing `CommandBuilder.Build().String()` method to generate the full command line, allowing users to copy and test manually.

## Complexity Tracking

No constitution violations requiring justification.

## Phases Overview

| Phase | Focus | Artifacts |
|-------|-------|-----------|
| 0 | Research | research.md |
| 1 | Design | data-model.md, contracts/openapi.yaml, quickstart.md |
| 2 | Tasks | tasks.md (via /speckit.tasks) |

## Dependencies

### External Dependencies (no changes)
- FFmpeg binary (external)
- GORM v2
- Huma v2.34+

### Internal Dependencies
- `internal/ffmpeg` package - extend CommandBuilder
- `internal/models.RelayProfile` - extend fields
- `internal/repository.RelayProfileRepository` - existing
- `pkg/httpclient` - for stream testing

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Command injection via custom flags | Medium | High | Strict input validation, no shell execution |
| Hardware acceleration detection timeout | Low | Medium | Timeout with fallback to software |
| Profile test hangs indefinitely | Medium | Medium | 30s hard timeout on FFmpeg process |
| Invalid custom flags crash FFmpeg | Medium | Low | Validate syntax, graceful error handling |
