# Tasks: End-to-End Pipeline Validation

**Input**: Design documents from `/specs/004-e2e-pipeline-validation/`
**Prerequisites**: plan.md (required), spec.md (required)

**Note**: This is a **testing/validation effort** - tasks are validation and bug-fix tasks, not new feature implementation.

**Tests**: Not applicable - this feature IS the testing effort.

**Organization**: Tasks are grouped by user story to enable independent validation of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different systems, no dependencies)
- **[Story]**: Which user story this task validates (e.g., US1, US2, US3)

---

## Phase 1: Setup (Test Environment)

**Purpose**: Prepare clean test environment for validation

- [ ] T001 Start backend server with clean database (`rm -f tvarr.db && task run:dev`)
- [ ] T002 [P] Start frontend development server (`cd frontend && npm run dev`)
- [ ] T003 Verify both servers are running and accessible (backend: :8080, frontend: :5173)

---

## Phase 2: Foundational (API Connectivity)

**Purpose**: Verify basic API operations before testing user stories

- [ ] T004 Verify health check endpoint returns 200 (`GET /api/v1/health`)
- [ ] T005 [P] Verify OpenAPI documentation is accessible (`GET /api/v1/docs`)
- [ ] T006 [P] Verify SSE progress endpoint accepts connections (`GET /api/v1/progress/events`)

**Checkpoint**: Basic API infrastructure is operational

---

## Phase 3: User Story 1 - Complete Stream Source Ingestion (Priority: P1)

**Goal**: Validate M3U/Xtream source ingestion stores all channel metadata correctly

**Independent Test**: Add stream source via UI, trigger ingestion, verify channels in list

### Validation Tasks for User Story 1

- [ ] T007 [US1] Create M3U stream source via UI with test M3U URL
- [ ] T008 [US1] Trigger ingestion for the M3U source
- [ ] T009 [US1] Monitor SSE progress events during ingestion (verify stage progression)
- [ ] T010 [US1] Verify ingestion completes with "completed" state
- [ ] T011 [US1] Verify channels appear in channel list with correct metadata (name, logo, group)
- [ ] T012 [US1] Verify channel count matches expected from source M3U
- [ ] T013 [US1] Document any bugs found during US1 validation in spec.md

**Checkpoint**: Stream source ingestion works end-to-end

---

## Phase 4: User Story 2 - Complete EPG Source Ingestion (Priority: P1)

**Goal**: Validate XMLTV source ingestion stores program data correctly

**Independent Test**: Add EPG source via UI, trigger ingestion, verify programs stored

### Validation Tasks for User Story 2

- [ ] T014 [US2] Create XMLTV EPG source via UI with test XMLTV URL
- [ ] T015 [US2] Trigger ingestion for the EPG source
- [ ] T016 [US2] Monitor SSE progress events during EPG ingestion
- [ ] T017 [US2] Verify ingestion completes with "completed" state
- [ ] T018 [US2] Verify programs are stored in database (check via API or direct DB query)
- [ ] T019 [US2] Verify program times are correctly parsed with timezone handling
- [ ] T020 [US2] Document any bugs found during US2 validation in spec.md

**Checkpoint**: EPG source ingestion works end-to-end

---

## Phase 5: User Story 3 - Stream Proxy Configuration and Generation (Priority: P1)

**Goal**: Validate proxy creation with source/EPG associations and pipeline generation

**Independent Test**: Create proxy with sources, generate output, verify M3U/XMLTV files

### Validation Tasks for User Story 3

- [ ] T021 [US3] Create stream proxy via UI with name and slug
- [ ] T022 [US3] Associate stream source(s) with the proxy
- [ ] T023 [US3] Associate EPG source(s) with the proxy
- [ ] T024 [US3] Save proxy and verify associations persist (edit proxy to confirm)
- [ ] T025 [US3] Trigger proxy generation
- [ ] T026 [US3] Monitor SSE progress events during generation (verify all 8 stages)
- [ ] T027 [US3] Verify generation completes with "completed" state at 100%
- [ ] T028 [US3] Document any bugs found during US3 validation in spec.md

**Checkpoint**: Proxy configuration and generation pipeline works end-to-end

---

## Phase 6: User Story 4 - Serving Generated M3U Output (Priority: P2)

**Goal**: Validate M3U endpoint returns valid, playable playlist

**Independent Test**: Request M3U URL, verify valid M3U content with channels

### Validation Tasks for User Story 4

- [ ] T029 [US4] Request M3U endpoint for generated proxy (`GET /api/v1/proxy/{slug}/m3u`)
- [ ] T030 [US4] Verify response has correct Content-Type header (audio/x-mpegurl or application/x-mpegurl)
- [ ] T031 [US4] Verify M3U starts with #EXTM3U header
- [ ] T032 [US4] Verify M3U contains expected channel entries (#EXTINF lines)
- [ ] T033 [US4] Verify channel stream URLs are present and correctly formatted
- [ ] T034 [US4] Verify logo URLs are present (if logo caching enabled, should be cached URLs)
- [ ] T035 [US4] Document any bugs found during US4 validation in spec.md

**Checkpoint**: M3U output is valid and contains expected content

---

## Phase 7: User Story 5 - Serving Generated XMLTV Output (Priority: P2)

**Goal**: Validate XMLTV endpoint returns valid EPG guide

**Independent Test**: Request XMLTV URL, verify valid XML with program data

### Validation Tasks for User Story 5

- [ ] T036 [US5] Request XMLTV endpoint for generated proxy (`GET /api/v1/proxy/{slug}/xmltv`)
- [ ] T037 [US5] Verify response has correct Content-Type header (application/xml or text/xml)
- [ ] T038 [US5] Verify XMLTV is valid XML (parse without errors)
- [ ] T039 [US5] Verify XMLTV contains <tv> root element
- [ ] T040 [US5] Verify XMLTV contains <channel> elements matching M3U channels
- [ ] T041 [US5] Verify XMLTV contains <programme> elements with schedule data
- [ ] T042 [US5] Verify programme start/stop times are correctly formatted
- [ ] T043 [US5] Document any bugs found during US5 validation in spec.md

**Checkpoint**: XMLTV output is valid and contains expected content

---

## Phase 8: User Story 6 - Pipeline Stage Progression (Priority: P3)

**Goal**: Validate all 8 pipeline stages execute and report progress via SSE

**Independent Test**: Monitor SSE during generation, verify all stage events

### Validation Tasks for User Story 6

- [ ] T044 [US6] Start SSE listener before triggering generation
- [ ] T045 [US6] Trigger proxy generation and capture all SSE events
- [ ] T046 [US6] Verify "load_programs" stage events are received
- [ ] T047 [US6] Verify "filtering" stage events are received
- [ ] T048 [US6] Verify "data_mapping" stage events are received
- [ ] T049 [US6] Verify "numbering" stage events are received
- [ ] T050 [US6] Verify "logo_caching" stage events are received
- [ ] T051 [US6] Verify "generate_m3u" stage events are received
- [ ] T052 [US6] Verify "generate_xmltv" stage events are received
- [ ] T053 [US6] Verify "publish" stage events are received
- [ ] T054 [US6] Verify final "completed" event with 100% progress
- [ ] T055 [US6] Document any bugs found during US6 validation in spec.md

**Checkpoint**: All pipeline stages execute and report progress correctly

---

## Phase 9: Polish & Bug Fixes

**Purpose**: Address any issues discovered during validation

- [ ] T056 Review all documented bugs from validation phases
- [ ] T057 Fix critical bugs blocking end-to-end flow (if any)
- [ ] T058 Fix non-critical bugs (if any)
- [ ] T059 Re-validate affected user stories after fixes
- [ ] T060 Update spec.md Known Issues section with final status
- [ ] T061 Run full end-to-end validation to confirm all stories pass

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion
- **User Story 1 (Phase 3)**: Depends on Foundational - validates ingestion
- **User Story 2 (Phase 4)**: Depends on Foundational - can run parallel to US1
- **User Story 3 (Phase 5)**: Depends on US1 and US2 (needs ingested data)
- **User Story 4 (Phase 6)**: Depends on US3 (needs generated proxy)
- **User Story 5 (Phase 7)**: Depends on US3 (needs generated proxy) - can run parallel to US4
- **User Story 6 (Phase 8)**: Depends on US3 (needs proxy generation to monitor)
- **Polish (Phase 9)**: Depends on all validation phases

### Parallel Opportunities

- US1 and US2 can run in parallel (both test ingestion independently)
- US4 and US5 can run in parallel (both test output serving)
- Within each phase, tasks marked [P] can run in parallel

---

## Parallel Example: Output Validation

```bash
# Launch M3U and XMLTV validation together:
Task: "Request M3U endpoint for generated proxy"
Task: "Request XMLTV endpoint for generated proxy"
```

---

## Implementation Strategy

### MVP First (US1 + US3 + US4)

1. Complete Phase 1-2: Setup + Foundational
2. Complete Phase 3: US1 (Stream Ingestion)
3. Complete Phase 5: US3 (Proxy Generation) - skip US2 EPG for MVP
4. Complete Phase 6: US4 (M3U Output)
5. **STOP and VALIDATE**: Working M3U playlist from source to output

### Full Validation

1. Complete all phases in order
2. Document all bugs found
3. Fix critical issues
4. Re-validate after fixes

---

## Notes

- This is a **validation effort** - no new code unless bugs found
- Document ALL issues in spec.md Known Issues section
- Take screenshots/logs of any failures for debugging
- Mark tasks complete only when validation passes
- If validation fails, create bug fix task before marking complete

---

## Phase 10: Pipeline Architecture Improvements

**Purpose**: Address architectural issues discovered during pipeline comparison analysis

### Ingestion Guard Enhancement

- [ ] T062 Add configurable grace period timer to ingestion guard stage
  - **Rationale**: When job scheduler is reintroduced, ingestion jobs may start in quick succession
  - **Requirement**: Wait for a configurable grace period (e.g., 5-30 seconds) after last ingestion completes before proceeding
  - **Configuration**: `ingestion_guard_grace_period` setting with sensible default

### Stage Order Correction

- [ ] T063 Reorder stages: Data Mapping MUST run BEFORE Filtering
  - **Current order**: Load Channels → Load Programs → **Filtering → Data Mapping** → ...
  - **Correct order**: Load Channels → Load Programs → **Data Mapping → Filtering** → ...
  - **Rationale**: Data mapping rules may alter fields that filter expressions operate on
  - **Example**: A mapping rule might set `channel.group = "Sports"`, which a filter then matches against
  - **Impact**: Filtering on unmapped data produces incorrect results

### Program Loading Correction

- [ ] T064 Load ALL programs regardless of channel membership
  - **Current behavior**: LoadPrograms filters programs to only those matching loaded channels by TvgID
  - **Required behavior**: Load ALL programs from EPG sources associated with the proxy
  - **Rationale**: Data mapping rules may alter TvgID or other fields used for channel-program matching
  - **Matching**: Program→Channel matching should happen AFTER data mapping, not during load

### Channel Numbering Simplification

- [ ] T065 Remove sequential and group numbering modes, keep only preserve mode
  - **Current**: Three modes (sequential, preserve, group)
  - **Required**: Only preserve mode (two-pass algorithm)
  - **Behavior**:
    - Pass 1: Channels with explicit `ChannelNumber > 0` claim their number (increment on conflict)
    - Pass 2: Channels without number (`ChannelNumber == 0`) fill gaps from `StartingChannelNumber`
  - **Remove**: UI options for sequential/group modes, simplify configuration

### Memory Efficiency Research

- [x] T066 [RESEARCH] Analyze viability of transparent disk-backed slice overflow
  - **COMPLETED AND REVERTED**: Implemented a DiskBackedSlice[T] package but complexity outweighed benefits:
    - No rebalancing from disk to memory after deletions
    - cgroups/container memory limits would cause OOM kill regardless
    - Significant code complexity for marginal benefit
    - Gob encoding overhead for serialization
  - **Decision**: Removed disk-backed slice implementation. Use simple Go slices.
    For very large datasets, rely on container orchestration (cgroups) and streaming/batching.

---

## Corrected Pipeline Stage Order

After T063 and T064, the correct stage order will be:

| # | Stage | Purpose |
|---|-------|---------|
| 1 | **Ingestion Guard** | Wait for active ingestions + grace period |
| 2 | **Load Channels** | Load ALL channels from associated stream sources |
| 3 | **Load Programs** | Load ALL programs from associated EPG sources (unfiltered) |
| 4 | **Data Mapping** | Apply transformation rules to channels AND programs |
| 5 | **Filtering** | Apply filters to mapped data (channels and programs) |
| 6 | **Logo Caching** | Download and cache logos |
| 7 | **Numbering** | Assign channel numbers (preserve mode only) |
| 8 | **Generate M3U** | Write M3U playlist |
| 9 | **Generate XMLTV** | Write XMLTV guide |
| 10 | **Publish** | Atomically publish files |

**Key changes from current**:
- Data Mapping moved BEFORE Filtering
- Load Programs loads ALL programs (matching happens in filtering)
- Ingestion Guard has grace period
- Numbering simplified to preserve-only
