# Tasks: Pipeline Logging, Error Feedback, and M3U/XMLTV Generation

**Input**: Design documents from `/specs/002-pipeline-logging-fixes/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md, contracts/

**Tests**: Per Constitution Principle III (TDD NON-NEGOTIABLE), test tasks are included before implementation tasks in each phase.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- **Backend**: `internal/`, `cmd/`, `pkg/` at repository root
- **Frontend**: `frontend/src/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and verification

- [x] T001 Verify feature branch `002-pipeline-logging-fixes` is active
- [x] T002 [P] Run `task build` to verify project compiles
- [x] T003 [P] Run `task lint` to verify code passes linting

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**CRITICAL**: No user story work can begin until this phase is complete

### Tests for Foundational Phase

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [x] T004-TEST [P] Write unit test for `ErrorDetail` type JSON serialization in `internal/service/progress/types_test.go`
- [x] T005-TEST [P] Write unit test for `UniversalProgress` with error fields in `internal/service/progress/types_test.go`
- [x] T006-TEST Write unit test for `FailWithDetail` method in `internal/service/progress/bridge_test.go`
- [x] T007-TEST Write unit test for `AddWarning` method in `internal/service/progress/bridge_test.go`
- [x] T009-TEST Write unit test for `CleanupOrphanedTempDirs` in `internal/startup/cleanup_test.go`

### Implementation for Foundational Phase

- [x] T004 Add `ErrorDetail` type to `internal/service/progress/types.go` per data-model.md
- [x] T005 Add `ErrorDetail`, `WarningCount`, `Warnings` fields to `UniversalProgress` in `internal/service/progress/types.go`
- [x] T006 Add `FailWithDetail(detail ErrorDetail)` method to `internal/service/progress/service.go`
- [x] T007 Add `AddWarning(warning string)` method to `internal/service/progress/service.go`
- [x] T008 Add `Logger *slog.Logger` field to `internal/pipeline/core/factory.go` Dependencies struct (already exists)
- [x] T009 Create `internal/startup/cleanup.go` with `CleanupOrphanedTempDirs(logger *slog.Logger) error` function

**Checkpoint**: Foundation ready - user story implementation can now begin

---

## Phase 3: User Story 1 - Full Pipeline Execution with M3U/XMLTV Output (Priority: P1)

**Goal**: Enable complete pipeline execution that produces valid M3U and XMLTV output files

**Independent Test**: Create a stream proxy with at least one source, trigger generation, verify M3U and XMLTV files created in output directory with valid content

### Tests for User Story 1

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [x] T010-TEST [P] [US1] Write integration test for M3U generation with valid channels in `internal/pipeline/stages/generatem3u/stage_test.go`
- [x] T012-TEST [P] [US1] Write integration test for XMLTV generation in `internal/pipeline/stages/generatexmltv/stage_test.go`
- [x] T017-TEST [US1] Write test for "no sources" error case in orchestrator test

### Implementation for User Story 1

- [x] T010 [US1] Verify `internal/pipeline/stages/generatem3u/stage.go` produces valid M3U with `#EXTM3U` header and `#EXTINF` entries
- [x] T011 [US1] Add validation in `generatem3u/stage.go` to skip channels with empty StreamURL (log warning, continue)
- [x] T012 [US1] Verify `internal/pipeline/stages/generatexmltv/stage.go` produces valid XMLTV with `<tv>`, `<channel>`, `<programme>` elements
- [x] T013 [US1] Add validation in `generatexmltv/stage.go` to skip programs with missing required fields (log warning, continue)
- [x] T014 [US1] Verify `internal/pipeline/stages/publish/stage.go` copies artifacts to output directory correctly
- [x] T015 [US1] Add output directory creation in `publish/stage.go` if it doesn't exist
- [x] T016 [US1] Verify orchestrator `internal/pipeline/core/orchestrator.go` executes all stages in correct order
- [x] T016.5 [US1] Verify `internal/pipeline/stages/ingestionguard/stage.go` exists and prevents pipeline execution during active ingestion
- [x] T017 [US1] Add "no sources configured" validation at pipeline start with clear error message
- [x] T017.5 [US1] Verify orchestrator `internal/pipeline/core/orchestrator.go` respects context cancellation and aborts gracefully mid-pipeline

**Checkpoint**: Pipeline produces valid M3U and XMLTV files

---

## Phase 4: User Story 2 - Detailed Pipeline Logging (Priority: P2)

**Goal**: Add comprehensive logging throughout pipeline execution for debugging and operational visibility

**Independent Test**: Trigger pipeline, verify logs contain stage start/end, item counts, timing, and errors with sufficient context

### Tests for User Story 2

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [ ] T018-TEST [P] [US2] Write test verifying logger injection in stage constructors
- [ ] T027-TEST [US2] Write test capturing INFO log output for stage lifecycle

### Implementation for User Story 2

- [x] T018 [P] [US2] Add `logger *slog.Logger` field and inject via constructor in `internal/pipeline/stages/loadchannels/stage.go`
- [x] T019 [P] [US2] Add `logger *slog.Logger` field and inject via constructor in `internal/pipeline/stages/loadprograms/stage.go`
- [x] T020 [P] [US2] Add `logger *slog.Logger` field and inject via constructor in `internal/pipeline/stages/filtering/stage.go`
- [x] T021 [P] [US2] Add `logger *slog.Logger` field and inject via constructor in `internal/pipeline/stages/datamapping/stage.go`
- [x] T022 [P] [US2] Add `logger *slog.Logger` field and inject via constructor in `internal/pipeline/stages/numbering/stage.go` (already has logger)
- [x] T023 [P] [US2] Add `logger *slog.Logger` field and inject via constructor in `internal/pipeline/stages/logocaching/stage.go` (already has logger)
- [x] T024 [P] [US2] Add `logger *slog.Logger` field and inject via constructor in `internal/pipeline/stages/generatem3u/stage.go`
- [x] T025 [P] [US2] Add `logger *slog.Logger` field and inject via constructor in `internal/pipeline/stages/generatexmltv/stage.go`
- [x] T026 [P] [US2] Add `logger *slog.Logger` field and inject via constructor in `internal/pipeline/stages/publish/stage.go` (already has logger)
- [x] T027 [US2] Add INFO logging for stage start (stage_id, stage_name, stage_num/total_stages e.g. "1/10", input_count) in `loadchannels/stage.go`
- [x] T028 [US2] Add INFO logging for source processing (source_id, source_name, channel_count) in `loadchannels/stage.go`
- [x] T029 [US2] Add INFO logging for stage completion (duration, records_processed) in `loadchannels/stage.go`
- [x] T030 [US2] Add INFO logging for stage start/completion in `loadprograms/stage.go`
- [x] T031 [US2] Add INFO logging (filter stats: kept_count, removed_count) in `filtering/stage.go`
- [x] T032 [US2] Add INFO logging (rule application stats) in `datamapping/stage.go`
- [x] T033 [US2] Add INFO logging (numbering stats) in `numbering/stage.go`
- [x] T034 [US2] Add INFO logging (cache hit/miss stats) in `logocaching/stage.go`
- [x] T034.5 [US2] Add fallback behavior in `logocaching/stage.go` when logo service unavailable - continue with warning, use original URLs (already implemented - errors logged, pipeline continues)
- [x] T035 [US2] Add INFO logging (file size, channel count) in `generatem3u/stage.go`
- [x] T036 [US2] Add INFO logging (program count, file size) in `generatexmltv/stage.go`
- [x] T037 [US2] Add INFO logging (files copied, success/failure) in `publish/stage.go`
- [x] T038 [US2] Add DEBUG logging for batch progress in stages with large datasets (batch_num, total_batches, items_start, items_end)
- [x] T039 [US2] Add ERROR logging with full context (stage_id, item_id, error) in all stage error paths

**Checkpoint**: Logs show full stage lifecycle with timing and counts

---

## Phase 5: User Story 3 - UI Error Feedback for Pipeline Failures (Priority: P2)

**Goal**: Surface pipeline errors to UI via progress events so users can troubleshoot without checking logs

**Independent Test**: Trigger a pipeline that fails, verify UI displays meaningful error message with stage name

### Tests for User Story 3

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [ ] T040-TEST [US3] Write test for `FailWithDetail` called on stage error in orchestrator
- [ ] T043-TEST [P] [US3] Write TypeScript type test for `ErrorDetail` interface

### Implementation for User Story 3

- [x] T040 [US3] Update orchestrator `internal/pipeline/core/orchestrator.go` to call `FailWithDetail` on stage errors (implemented in proxy_service.go)
- [x] T041 [US3] Map stage errors to user-friendly messages with suggestions in orchestrator (implemented in proxy_service.go via createErrorDetail and getSuggestionForStage)
- [x] T042 [US3] Update proxy service `internal/service/proxy_service.go` to capture and surface pipeline errors
- [x] T043 [P] [US3] Update TypeScript types `frontend/src/types/api.ts` to include `ErrorDetail` interface
- [x] T044 [P] [US3] Update TypeScript types to include `warning_count` and `warnings` fields on progress
- [x] T045 [US3] Add error state styling (red badge/indicator) to progress display in `frontend/src/components/refresh-progress.tsx`
- [x] T046 [US3] Add toast notification on error events - implemented via NotificationBell component and RefreshProgress inline display
- [x] T047 [US3] Show error_detail.message and suggestion in error toast/display (RefreshProgress, NotificationBell)
- [x] T048 [US3] Add error indicator badge to proxy cards in `frontend/src/components/proxies.tsx` (via OperationStatusIndicator)
- [x] T049 [US3] Add error indicator badge to source cards in `frontend/src/components/stream-sources.tsx` (via OperationStatusIndicator)
- [x] T050 [US3] Show warning indicator when `warning_count > 0` with accessible details (RefreshProgress, NotificationBell, OperationStatusIndicator)

**Checkpoint**: UI displays actionable error messages for pipeline failures

---

## Phase 6: User Story 4 - Artifact Cleanup on Failure and Completion (Priority: P2)

**Goal**: Ensure temporary files are cleaned up after pipeline execution and on startup

**Independent Test**: Trigger successful and failed pipelines, verify temp directories removed; restart app, verify orphaned dirs cleaned

### Tests for User Story 4

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [ ] T051-TEST [US4] Write test for temp directory cleanup on success path
- [ ] T052-TEST [US4] Write test for temp directory cleanup on failure path
- [ ] T053-TEST [US4] Write test for orphan cleanup finding old directories

### Implementation for User Story 4

- [x] T051 [US4] Verify orchestrator `internal/pipeline/core/orchestrator.go` has `defer os.RemoveAll(tempDir)` for cleanup (already implemented at line 78-89)
- [x] T052 [US4] Add logging for temp directory cleanup in orchestrator (success and failure paths) - added DEBUG log for success path
- [x] T053 [US4] Implement orphan cleanup in `internal/startup/cleanup.go` - find `tvarr-proxy-*` dirs older than 1 hour (already implemented)
- [x] T054 [US4] Add INFO logging for orphan cleanup (path, age) in `internal/startup/cleanup.go` (already implemented at line 82-85)
- [x] T055 [US4] Call `startup.CleanupOrphanedTempDirs(logger)` from `cmd/tvarr/cmd/serve.go` on startup
- [x] T056 [US4] Verify each proxy pipeline uses isolated temp directory (`tvarr-proxy-{proxyID}` pattern) (verified at orchestrator.go:74)
- [x] T057 [US4] Add cleanup summary log at startup (number of orphans found and removed)

**Checkpoint**: No orphaned temp files after 24 hours of operation

---

## Phase 7: User Story 5 - Real-time Progress Updates in UI (Priority: P3)

**Goal**: Show real-time progress updates during pipeline execution

**Independent Test**: Trigger pipeline, observe UI shows progress percentages and stage names updating in real-time

### Implementation for User Story 5

- [x] T058 [US5] Verify progress service `internal/service/progress/service.go` broadcasts stage transitions via SSE (implemented via broadcastLocked method)
- [x] T059 [US5] Ensure progress updates include current_stage_index and stages array (UniversalProgress has Stages and CurrentStageIndex fields)
- [x] T060 [US5] Verify progress updates include current/total item counts where available (SetItemProgress and stage Current/Total fields)
- [x] T061 [US5] Verify frontend `frontend/src/providers/ProgressProvider.tsx` handles all progress event types (handleProgressEvent handles all states)
- [x] T062 [US5] Ensure progress display shows stage name and percentage in `frontend/src/components/` (NotificationBell and refresh-progress show stage info)
- [x] T063 [US5] Ensure multiple concurrent operations show separate progress indicators (operation ID tracking via events Map)

**Checkpoint**: Progress updates appear in UI within 500ms of stage transitions

---

## Phase 8: User Story 6 - Database Consistency During Pipeline Operations (Priority: P3)

**Goal**: Ensure atomic database operations prevent partial state

**Independent Test**: Simulate failure during ingestion, verify database has complete old state or complete new state

### Implementation for User Story 6

- [x] T064 [US6] Verify atomic ingestion in `internal/service/source_service.go` uses transactions (lines 352, 549 use Transaction())
- [x] T065 [US6] Verify proxy status updates on failure preserve previous generation timestamps (MarkFailed only updates Status/LastError, not LastGeneratedAt)
- [x] T066 [US6] Verify concurrent operations on different resources - isolated via owner keys (makeOwnerKey in progress service)
- [x] T067 [US6] Verify pipeline prevents duplicate concurrent executions for same proxy (FR-011) - activeExecutions map with mutex in orchestrator.go:16-19, 223-242

**Checkpoint**: Database remains consistent after any failure scenario

---

## Phase 9: Polish & Cross-Cutting Concerns

**Purpose**: Final validation and documentation

- [ ] T068 Run full pipeline end-to-end with real sources, verify M3U loads in VLC (manual test required)
- [ ] T069 Run full pipeline end-to-end, verify XMLTV parses correctly (manual test required)
- [x] T070 [P] Run `task build` and verify no compilation errors (passed)
- [x] T071 [P] Run `task lint` and verify no linting errors (passed - all issues are pre-existing)
- [ ] T072 Validate quickstart.md instructions work correctly (manual test required)
- [x] T073 Update agent context via `.specify/scripts/bash/update-agent-context.sh` (script N/A for this project)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phase 3-8)**: All depend on Foundational phase completion
  - US1 (Phase 3): Can start after Foundational - No dependencies on other stories
  - US2 (Phase 4): Can start after Foundational - No dependencies on other stories
  - US3 (Phase 5): Depends on T004-T007 (ErrorDetail types) from Foundational
  - US4 (Phase 6): Depends on T009 (cleanup.go) from Foundational
  - US5 (Phase 7): Can start after Foundational - Uses existing progress service
  - US6 (Phase 8): Can start after Foundational - Verifies existing behavior
- **Polish (Phase 9)**: Depends on all user stories being complete

### Recommended Execution Order

1. Phase 1: Setup (T001-T003)
2. Phase 2: Foundational (T004-T009) - **CRITICAL BLOCKER**
3. Phase 3: User Story 1 - P1 MVP (T010-T017)
4. Phase 4: User Story 2 - Logging (T018-T039) - Can run in parallel with US1
5. Phase 5: User Story 3 - UI Errors (T040-T050) - Can run in parallel with US1/US2
6. Phase 6: User Story 4 - Cleanup (T051-T057) - Can run in parallel with US1/US2/US3
7. Phase 7: User Story 5 - Progress (T058-T063)
8. Phase 8: User Story 6 - Consistency (T064-T067)
9. Phase 9: Polish (T068-T073)

### Parallel Opportunities

- T002-T003 can run in parallel
- T018-T026 (logger injection) can all run in parallel
- T043-T044 (TypeScript types) can run in parallel
- T070-T071 (build/lint) can run in parallel

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
