# Tasks: Configuration Settings & Debug UI Consolidation

**Input**: Design documents from `/specs/006-config-settings-ui/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/openapi.yaml

**Tests**: Constitution mandates TDD (Test-First Development). Tests included per Principle III.

**Organization**: Tasks grouped by user story for independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story (US1-US7)
- Exact file paths included

## Path Conventions

- **Backend**: `internal/`, `pkg/`, `cmd/`
- **Frontend**: `frontend/src/`
- **Tests**: `*_test.go` (Go), `*.test.tsx` (React)

---

## Phase 1: Setup

**Purpose**: Verify prerequisites and prepare for implementation

- [x] T001 Verify branch `006-config-settings-ui` is current and up to date
- [x] T002 [P] Review existing handlers in `internal/http/handlers/settings.go`, `feature.go`, `circuit_breaker.go`
- [x] T003 [P] Review existing circuit breaker implementation in `pkg/httpclient/client.go`
- [x] T004 [P] Review frontend settings component in `frontend/src/components/settings.tsx`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before user stories

**⚠️ CRITICAL**: Circuit breaker stats enhancement and health endpoints are needed by multiple stories

### Backend Foundation

- [x] T005 Create enhanced stats types in `pkg/httpclient/stats.go` (ErrorCategoryCount, StateDurationSummary, StateTransition)
- [x] T006 Add error categorization tracking to `pkg/httpclient/client.go` (track 2xx/4xx/5xx/timeout/network)
- [x] T007 Add state transition history to `pkg/httpclient/client.go` (circular buffer, 50 max)
- [x] T008 Add state duration tracking to `pkg/httpclient/client.go` (StateEnteredAt, cumulative durations)
- [x] T009 Update `pkg/httpclient/manager.go` to expose enhanced stats via GetAllEnhancedStats()
- [x] T010 Write tests for enhanced circuit breaker stats in `pkg/httpclient/stats_test.go`

### Health Endpoints Foundation

- [x] T011 Add `/livez` endpoint to `internal/http/handlers/health.go` (lightweight liveness check)
- [x] T012 Add `/readyz` endpoint to `internal/http/handlers/health.go` (checks DB, scheduler)
- [x] T013 Add LivezResponse and ReadyzResponse types to `internal/http/handlers/health.go`
- [x] T014 Register `/livez` and `/readyz` routes in `internal/http/handlers/health.go`
- [x] T015 Write tests for livez/readyz endpoints in `internal/http/handlers/health_test.go`

**Checkpoint**: Foundation ready - circuit breaker stats enhanced, health endpoints available

---

## Phase 3: User Story 1 - View and Modify Runtime Settings (Priority: P1) 🎯 MVP

**Goal**: Administrators can view/modify runtime settings via unified API without restart

**Independent Test**: Change log level from "info" to "debug" via API, verify logs show debug messages

### Tests for User Story 1

- [x] T016 [P] [US1] Write test for GET /api/v1/config in `internal/http/handlers/config_test.go`
- [x] T017 [P] [US1] Write test for PUT /api/v1/config in `internal/http/handlers/config_test.go`

### Implementation for User Story 1

- [x] T018 [P] [US1] Create UnifiedConfig, RuntimeConfig types in `internal/http/handlers/config_types.go`
- [x] T019 [P] [US1] Create ConfigMeta type in `internal/http/handlers/config_types.go`
- [x] T020 [US1] Create ConfigHandler struct in `internal/http/handlers/config.go`
- [x] T021 [US1] Implement GET /api/v1/config handler in `internal/http/handlers/config.go`
- [x] T022 [US1] Implement PUT /api/v1/config handler in `internal/http/handlers/config.go`
- [x] T023 [US1] Wire ConfigHandler to observability.SetLogLevel() and observability.SetRequestLogging()
- [x] T024 [US1] Register ConfigHandler in `internal/http/handlers/config.go`
- [ ] T025 [US1] Add deprecation warning headers to old endpoints in `internal/http/handlers/settings.go`

**Checkpoint**: User Story 1 complete - runtime settings viewable/modifiable via unified API

---

## Phase 4: User Story 2 - View System Health Metrics on Debug Page (Priority: P1)

**Goal**: Debug page displays CPU/memory/circuit breaker metrics without errors

**Independent Test**: Open debug page, verify CPU load %, memory bars, circuit breaker states display without crashes

### Tests for User Story 2

- [ ] T026 [P] [US2] Write test for error-resilient debug component rendering in `frontend/src/components/debug.test.tsx`

### Implementation for User Story 2

- [x] T027 [US2] Add optional chaining for all health data access in `frontend/src/components/debug.tsx`
- [x] T028 [US2] Add "Not available" fallback displays in `frontend/src/components/debug.tsx`
- [x] T029 [US2] Fix sandbox_manager undefined access in `frontend/src/components/debug.tsx`
- [x] T030 [US2] Update health data hook to handle partial responses (N/A - debug.tsx handles directly)

**Checkpoint**: User Story 2 complete - debug page renders without errors

---

## Phase 5: User Story 3 - Consolidated Configuration API (Priority: P1)

**Goal**: Single API endpoint returns all configuration data

**Independent Test**: Call GET /api/v1/config, receive runtime + startup + circuit breaker config in one response

### Tests for User Story 3

- [x] T031 [P] [US3] Write test for unified response structure in `internal/http/handlers/config_test.go`
- [x] T032 [P] [US3] Write test for atomic updates in `internal/http/handlers/config_test.go`

### Implementation for User Story 3

- [x] T033 [US3] Add StartupConfig assembly from Viper in `internal/http/handlers/config.go`
- [x] T034 [US3] Integrate circuit breaker config from manager in `internal/http/handlers/config.go`
- [x] T035 [US3] Integrate feature flags in unified response in `internal/http/handlers/config.go`
- [x] T036 [US3] Implement atomic update logic (settings + features + CB config) in `internal/http/handlers/config.go`
- [x] T037 [US3] Update frontend settings page to use unified endpoint in `frontend/src/components/settings.tsx`
- [x] T038 [US3] Update frontend API types (inline in settings.tsx)
- [x] T039 [US3] Migrate frontend connectivity provider from /live to /livez in `frontend/src/providers/backend-connectivity-provider.tsx`

**Checkpoint**: User Story 3 complete - single API call loads all config

---

## Phase 6: User Story 4 - Rich Circuit Breaker Visualization (Priority: P2)

**Goal**: Circuit breaker cards show segmented bar, state duration, transition history

**Independent Test**: View CB card, verify segmented bar with error breakdown, state duration, transition list

### Tests for User Story 4

- [ ] T040 [P] [US4] Write test for SegmentedProgressBar component in `frontend/src/components/circuit-breaker/SegmentedProgressBar.test.tsx`
- [ ] T041 [P] [US4] Write test for StateTimeline component in `frontend/src/components/circuit-breaker/StateTimeline.test.tsx`

### Implementation for User Story 4

- [x] T042 [P] [US4] Create CircuitBreakerCard component in `frontend/src/components/circuit-breaker/CircuitBreakerCard.tsx`
- [x] T043 [P] [US4] Create SegmentedProgressBar component in `frontend/src/components/circuit-breaker/SegmentedProgressBar.tsx`
- [x] T044 [P] [US4] Create StateIndicator component in `frontend/src/components/circuit-breaker/StateIndicator.tsx`
- [x] T045 [P] [US4] Create StateTimeline component in `frontend/src/components/circuit-breaker/StateTimeline.tsx`
- [x] T046 [P] [US4] Create StateDurationSummary component in `frontend/src/components/circuit-breaker/StateDurationSummary.tsx`
- [x] T047 [US4] Create circuit-breaker component index in `frontend/src/components/circuit-breaker/index.ts`
- [x] T048 [US4] Add TypeScript types for enhanced CB stats in `frontend/src/types/circuit-breaker.ts`
- [x] T049 [US4] Integrate CircuitBreakerCard into debug page in `frontend/src/components/debug.tsx`
- [x] T050 [US4] Add GET /api/v1/circuit-breakers/stats endpoint in `internal/http/handlers/circuit_breaker.go`

**Checkpoint**: User Story 4 complete - rich CB visualization on debug page

---

## Phase 7: User Story 5 - View Startup Configuration (Priority: P2)

**Goal**: Settings page shows read-only startup config grouped by category

**Independent Test**: View Startup Configuration section, verify all categories displayed as read-only

### Tests for User Story 5

- [ ] T051 [P] [US5] Write test for startup config section in `frontend/src/components/settings.test.tsx`

### Implementation for User Story 5

- [x] T052 [US5] Startup config section integrated directly in `frontend/src/components/settings.tsx` (no separate component needed)
- [x] T053 [US5] Display startup config grouped by category (Server, Database, Storage, etc.) in `frontend/src/components/settings.tsx`
- [x] T054 [US5] Add read-only styling with restart-required indicator in `frontend/src/components/settings.tsx`
- [x] T055 [US5] Add config source indicator (file/env/default) display in `frontend/src/components/settings.tsx`

**Checkpoint**: User Story 5 complete - startup config visible, clearly read-only

---

## Phase 8: User Story 6 - Persist Configuration to File (Priority: P2)

**Goal**: Save button writes current config to YAML file

**Independent Test**: Change setting, click "Save to Config", restart service, verify setting persists

### Tests for User Story 6

- [ ] T056 [P] [US6] Write test for POST /api/v1/config/persist in `internal/http/handlers/config_test.go`
- [ ] T057 [P] [US6] Write test for permission checking in `internal/service/config/persistence_test.go`

### Implementation for User Story 6

- [x] T058 [US6] Persist logic implemented directly in `internal/http/handlers/config.go` (no separate service needed)
- [x] T059 [US6] CanPersist() permission check in ConfigMeta via `internal/http/handlers/config.go`
- [x] T060 [US6] Persist() method using Viper WriteConfig in `internal/http/handlers/config.go`
- [x] T061 [US6] POST /api/v1/config/persist handler in `internal/http/handlers/config.go`
- [x] T062 [US6] Add "Save to Config File" button in `frontend/src/components/settings.tsx`
- [x] T063 [US6] Add success/error feedback for persist operation in `frontend/src/components/settings.tsx`

**Checkpoint**: User Story 6 complete - config changes can be persisted

---

## Phase 9: User Story 7 - Modify Circuit Breaker Configuration (Priority: P3)

**Goal**: Administrators can adjust CB thresholds and reset breakers at runtime

**Independent Test**: Change failure_threshold from 3 to 5, verify new threshold is applied

### Tests for User Story 7

- [ ] T064 [P] [US7] Write test for CB config update via unified API in `internal/http/handlers/config_test.go`
- [ ] T065 [P] [US7] Write test for CB reset preserving config in `pkg/httpclient/manager_test.go`

### Implementation for User Story 7

- [x] T066 [US7] Circuit breaker config editing UI in `frontend/src/components/settings.tsx` (already existed)
- [x] T067 [US7] Per-service profile override UI in `frontend/src/components/settings.tsx` (already existed)
- [x] T068 [US7] Add Reset button per circuit breaker in `frontend/src/components/circuit-breaker/CircuitBreakerCard.tsx`
- [x] T069 [US7] Wire Reset button to POST /api/v1/circuit-breakers/{name}/reset
- [x] T070 [US7] Add confirmation dialog for reset action

**Checkpoint**: User Story 7 complete - CB config modifiable, breakers resettable

---

## Phase 10: Polish & Cross-Cutting Concerns

**Purpose**: Final improvements across all stories

- [ ] T071 [P] Remove deprecated endpoint code paths (if transition period elapsed)
- [x] T072 [P] Add comprehensive error messages for config validation failures
- [x] T073 [P] Add loading states for all async operations in settings page
- [x] T074 [P] Ensure consistent color theming for CB visualization (dark/light mode)
- [x] T075 Run frontend lint: `pnpm run lint` in `frontend/`
- [x] T076 Run backend build: `go build ./...`
- [x] T077 Run backend handler tests: `go test ./internal/http/handlers/...`
- [ ] T078 Validate quickstart.md scenarios work end-to-end

---

## Dependencies & Execution Order

### Phase Dependencies

```
Phase 1 (Setup)
    ↓
Phase 2 (Foundational) ← BLOCKS ALL USER STORIES
    ↓
┌───────────────────────────────────────────────────────┐
│  P1 Stories (can run in parallel after Phase 2):     │
│  - Phase 3: US1 - Runtime Settings                   │
│  - Phase 4: US2 - Debug Page Health                  │
│  - Phase 5: US3 - Consolidated API                   │
└───────────────────────────────────────────────────────┘
    ↓ (P1 complete)
┌───────────────────────────────────────────────────────┐
│  P2 Stories (can run in parallel):                   │
│  - Phase 6: US4 - CB Visualization (needs Phase 2)   │
│  - Phase 7: US5 - Startup Config (needs Phase 5)     │
│  - Phase 8: US6 - Config Persistence (needs Phase 3) │
└───────────────────────────────────────────────────────┘
    ↓ (P2 complete)
Phase 9: US7 - CB Configuration (P3)
    ↓
Phase 10: Polish
```

### User Story Dependencies

| Story | Depends On | Can Start After |
|-------|------------|-----------------|
| US1 | Phase 2 | Foundational complete |
| US2 | Phase 2 | Foundational complete |
| US3 | Phase 2, US1 | US1 backend complete |
| US4 | Phase 2 | Foundational complete |
| US5 | US3 | Unified API available |
| US6 | US3 | Unified API available |
| US7 | US4, US6 | CB viz and persist ready |

### Parallel Opportunities

Within Phase 2 (Foundational):
- T005, T006, T007, T008 can run in parallel (different aspects of CB stats)
- T011, T012 can run in parallel (different endpoints)

Within User Stories:
- All test tasks marked [P] can run in parallel
- All frontend component tasks marked [P] can run in parallel

---

## Parallel Example: User Story 4 (Circuit Breaker Visualization)

```bash
# Launch all component creation in parallel:
Task: "Create CircuitBreakerCard component in frontend/src/components/circuit-breaker/CircuitBreakerCard.tsx"
Task: "Create SegmentedProgressBar component in frontend/src/components/circuit-breaker/SegmentedProgressBar.tsx"
Task: "Create StateIndicator component in frontend/src/components/circuit-breaker/StateIndicator.tsx"
Task: "Create StateTimeline component in frontend/src/components/circuit-breaker/StateTimeline.tsx"
Task: "Create StateDurationSummary component in frontend/src/components/circuit-breaker/StateDurationSummary.tsx"

# Then integrate (sequential):
Task: "Create circuit-breaker component index"
Task: "Integrate CircuitBreakerCard into debug page"
```

---

## Implementation Strategy

### MVP First (User Stories 1-3 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL)
3. Complete Phase 3: User Story 1 - Runtime Settings
4. Complete Phase 4: User Story 2 - Debug Page Health
5. Complete Phase 5: User Story 3 - Consolidated API
6. **STOP and VALIDATE**: Test all P1 stories independently
7. Deploy/demo MVP

### Incremental Delivery

After MVP:
1. Add User Story 4 (CB Visualization) → Rich debug page
2. Add User Story 5 (Startup Config) → Complete settings visibility
3. Add User Story 6 (Config Persistence) → Changes survive restart
4. Add User Story 7 (CB Configuration) → Full CB control
5. Polish phase → Production ready

### Suggested MVP Scope

**Minimum Viable Product**: User Stories 1, 2, 3 (all P1 priority)
- Unified config API working
- Debug page not crashing
- Runtime settings modifiable

---

## Notes

- [P] tasks = different files, no dependencies on incomplete tasks in same story
- [Story] label tracks which user story for traceability
- Each user story independently testable after completion
- Commit after each task or logical group
- TDD required per Constitution Principle III
- No emojis in log output per CLAUDE.md
