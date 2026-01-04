# Tasks: Codebase Cleanup and Refactoring

**Input**: Design documents from `/specs/015-codebase-cleanup/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md

**Tests**: Constitution mandates TDD. Tests will be added for masq integration.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1, US2, US3, US4)
- Include exact file paths in descriptions

## Path Conventions

- **Go backend**: `cmd/`, `internal/`, `pkg/` at repository root
- **Frontend**: `frontend/` (not modified in this feature)

---

## Phase 1: Setup

**Purpose**: Verify current state and prepare for changes

- [ ] T001 Run `staticcheck ./...` and capture baseline issues count
- [ ] T002 [P] Verify `go build ./...` succeeds with current local replace directive
- [ ] T003 [P] Count current lines in large files for comparison baseline

**Checkpoint**: Baseline established - ready for implementation

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Add dependencies required by user stories

**⚠️ CRITICAL**: User Story 1 depends on masq being available

- [ ] T004 Add `github.com/m-mizutani/masq` dependency via `go get github.com/m-mizutani/masq`
- [ ] T005 Add `golang.org/x/text/cases` dependency for strings.Title replacement via `go get golang.org/x/text/cases`
- [ ] T006 Run `go mod tidy` to clean up dependencies

**Checkpoint**: Dependencies ready - user story implementation can begin

---

## Phase 3: User Story 1 - Sensitive Data Protection in Logs (Priority: P1) 🎯 MVP

**Goal**: Passwords and sensitive data automatically redacted in all log output

**Independent Test**: Trigger source ingestion with Xtream credentials, verify log output shows redacted values instead of actual passwords

### Tests for User Story 1

- [ ] T007 [US1] Create test for masq redaction in `internal/observability/logger_test.go` - verify password fields are redacted

### Implementation for User Story 1

- [ ] T008 [US1] Import masq and configure redactor in `internal/observability/logger.go` with WithFieldName for: password, Password, secret, Secret, token, Token, apikey, ApiKey, api_key, credential, Credential
- [ ] T009 [US1] Update NewLogger function to use masq.New() as ReplaceAttr in slog.HandlerOptions in `internal/observability/logger.go`
- [ ] T010 [US1] Run test T007 to verify redaction works
- [ ] T011 [US1] Manual verification: start server with DEBUG logging, create Xtream source, verify password not visible in logs

**Checkpoint**: User Story 1 complete - sensitive data redacted in all logs (SC-001)

---

## Phase 4: User Story 2 - Upstream Dependency Migration (Priority: P2)

**Goal**: Project builds without local filesystem dependencies, CI/CD compatible

**Independent Test**: Remove local replace directive, run `go mod tidy`, verify `go build ./...` succeeds

### Implementation for User Story 2

- [ ] T012 [US2] Verify `github.com/jmylchreest/mediacommon` feat/eac3-support branch exists and is accessible on GitHub
- [ ] T013 [US2] Determine pseudo-version for feat/eac3-support branch commit using `go list -m github.com/jmylchreest/mediacommon/v2@feat/eac3-support`
- [ ] T014 [US2] Update `go.mod` to replace local path with GitHub fork reference: `replace github.com/bluenviron/mediacommon/v2 => github.com/jmylchreest/mediacommon/v2 <pseudo-version>`
- [ ] T015 [US2] Run `go mod tidy` to verify dependency resolution
- [ ] T016 [US2] Run `go build ./...` to verify build succeeds
- [ ] T017 [US2] Verify `go.mod` contains no local filesystem replace directives with `grep -E "replace.*=>.*(\.\.|\./)" go.mod`

**Checkpoint**: User Story 2 complete - build works from fresh clone (SC-002, SC-007)

---

## Phase 5: User Story 3 - Dead Code Elimination (Priority: P3)

**Goal**: All unused code removed, staticcheck passes with zero issues

**Independent Test**: Run `staticcheck ./...` and verify zero issues reported

### Implementation for User Story 3

#### Remove Unused Functions

- [ ] T018 [P] [US3] Remove unused `waitForSSECompletion` function from `cmd/e2e-runner/main.go:945`
- [ ] T019 [P] [US3] Remove unused `runTest` function from `cmd/e2e-runner/main.go:1840`
- [ ] T020 [P] [US3] Remove unused `contains`, `containsLower`, `matchLower` functions from `internal/http/handlers/relay_stream.go:674-687`
- [ ] T021 [P] [US3] Remove unused `extractNALUnitsLengthPrefixed` function from `internal/relay/fmp4_adapter.go:252`
- [ ] T022 [P] [US3] Remove unused `runPassthroughBySourceFormat` function from `internal/relay/session.go:320`
- [ ] T023 [P] [US3] Remove unused `cleanupIdleTranscoders` function from `internal/relay/session.go:1150`
- [ ] T024 [P] [US3] Remove unused `demuxerWriter` type and its Write method from `internal/relay/session.go:2344-2350`
- [ ] T025 [P] [US3] Remove unused `streamTypeFromCodec` function from `internal/relay/ts_muxer.go:140`
- [ ] T026 [P] [US3] Remove unused `ruleToResult` and `defaultResult` functions from `internal/service/client_detection_service.go:561-577`

#### Remove Unused Test Mocks

- [ ] T027 [P] [US3] Remove unused `mockEpgHandler` type from `internal/service/epg_service_test.go:259-281`
- [ ] T028 [P] [US3] Remove unused `mockScheduler` type from `internal/service/job_service_test.go:440`
- [ ] T029 [P] [US3] Remove unused `mockPipelineFactory` type from `internal/service/proxy_service_test.go:212-218`

#### Fix Deprecated API Usage

- [ ] T030 [P] [US3] Replace `mpeg4audio.Config` with `AudioSpecificConfig` in `internal/relay/cmaf_muxer.go`
- [ ] T031 [P] [US3] Replace `mpeg4audio.Config` with `AudioSpecificConfig` in `internal/relay/fmp4_adapter.go`
- [ ] T032 [P] [US3] Replace `mpeg4audio.Config` with `AudioSpecificConfig` in `internal/relay/ts_muxer.go`
- [ ] T033 [P] [US3] Replace `mpeg4audio.Config` with `AudioSpecificConfig` in `internal/relay/cmaf_muxer_test.go`
- [ ] T034 [P] [US3] Replace `mpeg4audio.Config` with `AudioSpecificConfig` in `internal/relay/fmp4_adapter_test.go`
- [ ] T035 [P] [US3] Replace `mpeg4audio.Config` with `AudioSpecificConfig` in `internal/relay/ts_mediacommon_test.go`
- [ ] T036 [US3] Replace `strings.Title` with `cases.Title` from `golang.org/x/text/cases` in `internal/service/theme_service.go:304`

#### Fix Style Issues

- [ ] T037 [P] [US3] Fix capitalized error strings at `cmd/e2e-runner/main.go:2030` and `cmd/e2e-runner/main.go:2033`
- [ ] T038 [P] [US3] Remove redundant nil check before len() at `cmd/e2e-runner/main.go:1619`
- [ ] T039 [P] [US3] Fix unused variable assignment at `cmd/e2e-runner/testdata.go:119`
- [ ] T040 [P] [US3] Use type conversion instead of struct literal at `internal/http/handlers/relay_stream.go:344` and `:479`

#### Verification

- [ ] T041 [US3] Run `staticcheck ./...` and verify zero issues (SC-003, SC-004)
- [ ] T042 [US3] Run `go build ./...` to ensure no compilation errors
- [ ] T043 [US3] Run `go test ./...` to ensure tests still pass

**Checkpoint**: User Story 3 complete - staticcheck passes with zero issues

---

## Phase 6: User Story 4 - Code Structure and Organization (Priority: P4)

**Goal**: Codebase follows idiomatic Go patterns, no files exceed 1000 lines

**Independent Test**: Review package structure, check file sizes, verify no duplicate logic

### Implementation for User Story 4

- [ ] T044 [US4] Review `internal/http/handlers/types.go` (1007 lines) - determine if types should be split by domain
- [ ] T045 [US4] Review `internal/relay/session.go` (2360 lines) - identify if session state types can be extracted
- [ ] T046 [US4] Review `internal/http/handlers/relay_stream.go` (1555 lines) - check if handlers can be logically grouped
- [ ] T047 [US4] Document decision for each file: split or accept with justification
- [ ] T048 [US4] Execute any approved file splits (if determined necessary from T044-T047)
- [ ] T049 [US4] Run file size check: `find . -name "*.go" -not -name "*_test.go" -exec wc -l {} + | awk '$1 > 1000'` and document results

**Checkpoint**: User Story 4 complete - code structure reviewed and documented

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Final verification and cleanup

- [ ] T050 Run full verification suite per quickstart.md
- [ ] T051 [P] Update CLAUDE.md if any new patterns established
- [ ] T052 Run `go mod tidy` final pass
- [ ] T053 Run `go build ./...` final verification
- [ ] T054 Run `go test ./...` final test pass
- [ ] T055 Run `staticcheck ./...` final check - must be zero issues
- [ ] T056 Verify SC-001: No plaintext passwords in logs (manual test)
- [ ] T057 Verify SC-002: Build succeeds from clean state
- [ ] T058 Verify SC-007: No local replace directives in go.mod

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - establishes baseline
- **Foundational (Phase 2)**: Depends on Setup - adds masq dependency
- **User Story 1 (Phase 3)**: Depends on Foundational - uses masq
- **User Story 2 (Phase 4)**: Depends on Setup only - independent of US1
- **User Story 3 (Phase 5)**: Depends on Setup only - can run parallel to US1/US2
- **User Story 4 (Phase 6)**: Depends on US3 completion - needs dead code removed first
- **Polish (Phase 7)**: Depends on all user stories complete

### User Story Dependencies

- **User Story 1 (P1)**: Depends on Foundational (masq dependency)
- **User Story 2 (P2)**: Independent - can run in parallel with US1
- **User Story 3 (P3)**: Independent - can run in parallel with US1/US2
- **User Story 4 (P4)**: Should run after US3 (dead code removed simplifies file analysis)

### Parallel Opportunities

Within **User Story 3** (Dead Code Elimination):
- T018-T029: All unused function/mock removals can run in parallel
- T030-T035: All deprecated API fixes can run in parallel
- T037-T040: All style fixes can run in parallel
- T041-T043: Verification must run sequentially after all fixes

---

## Parallel Example: User Story 3 Dead Code Removal

```bash
# Launch all unused function removals in parallel:
Task: "Remove waitForSSECompletion from cmd/e2e-runner/main.go"
Task: "Remove runTest from cmd/e2e-runner/main.go"
Task: "Remove contains/containsLower/matchLower from relay_stream.go"
Task: "Remove extractNALUnitsLengthPrefixed from fmp4_adapter.go"
# ... etc (all T018-T029)

# Then launch all deprecated API fixes in parallel:
Task: "Replace mpeg4audio.Config in cmaf_muxer.go"
Task: "Replace mpeg4audio.Config in fmp4_adapter.go"
# ... etc (all T030-T036)
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001-T003)
2. Complete Phase 2: Foundational (T004-T006)
3. Complete Phase 3: User Story 1 (T007-T011)
4. **STOP and VALIDATE**: Verify passwords are redacted in logs
5. This delivers security improvement immediately

### Incremental Delivery

1. MVP: User Story 1 → Passwords redacted (security)
2. Add: User Story 2 → CI/CD enabled (build reproducibility)
3. Add: User Story 3 → Clean codebase (maintainability)
4. Add: User Story 4 → Documented structure (onboarding)

### Parallel Team Strategy

With multiple developers:

1. Setup + Foundational together
2. Once Foundational done:
   - Developer A: User Story 1 (masq integration)
   - Developer B: User Story 2 (dependency migration)
   - Developer C: User Story 3 (dead code removal)
3. User Story 4 after US3 completes

---

## Success Criteria Mapping

| Success Criteria | Tasks |
|------------------|-------|
| SC-001: Zero passwords in logs | T007-T011, T056 |
| SC-002: Build from fresh clone | T012-T017, T057 |
| SC-003: staticcheck zero issues | T018-T043, T055 |
| SC-004: No unused exported symbols | T018-T029, T041 |
| SC-005: No files over 1000 lines | T044-T049 |
| SC-007: No local replace directives | T014, T017, T058 |

---

## Notes

- [P] tasks = different files, no dependencies, safe to parallelize
- [US#] label maps task to specific user story
- T030-T035 require careful attention - ensure AudioSpecificConfig is imported correctly
- T036 requires adding golang.org/x/text/cases import
- After dead code removal (US3), file line counts will decrease significantly
- User Story 4 file reviews may result in "no action needed" - that's acceptable
