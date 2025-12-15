# Tasks: Fix EPG Timezone Normalization and Canvas Layout

**Input**: Design documents from `/specs/017-fix-epg-timezone-canvas/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: Added per Constitution Principle III (Test-First Development - NON-NEGOTIABLE).

**Organization**: Tasks grouped by user story to enable independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- **Backend**: `internal/` at repository root
- **Frontend**: `frontend/src/` for source, `frontend/tests/` for tests

---

## Phase 1: Setup

**Purpose**: Create foundational utilities needed across multiple user stories

- [x] T001 [P] Create timezone parsing utility in `internal/ingestor/timezone.go` with `ParseTimezoneOffset()` and `NormalizeProgramTime()` functions
- [x] T002 [P] Create useCanvasMetrics hook in `frontend/src/hooks/useCanvasMetrics.ts` with dynamic pixels-per-hour calculation, scroll position preservation, and program bounds calculation

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before user story implementation

**CRITICAL**: No user story work can begin until this phase is complete

- [x] T003 Add timezone parsing unit tests in `internal/ingestor/timezone_test.go` covering all offset formats (+HHMM, -HHMM, +HH:MM, Z, empty, invalid)
- [x] T003a [P] Add timezone normalization integration tests in `internal/ingestor/timezone_test.go` for NormalizeProgramTime with various offset+shift combinations
- [~] T003b [P] Add useCanvasMetrics hook tests in `frontend/tests/epg/canvas-metrics.test.ts` - SKIPPED: No frontend test infrastructure (Jest/Vitest not configured)
- [x] T004 Verify existing `EpgSource` model has `DetectedTimezone` and `EpgShift` fields in `internal/models/epg_source.go` - no changes needed if present

**Checkpoint**: Foundation ready - tests written and failing, user story implementation can now begin

---

## Phase 3: User Story 1 - Accurate Current Program Display (Priority: P1)

**Goal**: Fix timezone normalization so "now" indicator accurately shows currently-airing programs

**Independent Test**: Load EPG with +1 timezone source, verify red "now" indicator aligns with actual current programs

### Implementation for User Story 1

- [x] T005 [US1] Modify `internal/ingestor/xmltv_handler.go` to use `ParseTimezoneOffset()` and `NormalizeProgramTime()` for time parsing, applying inverse offset then timeshift
- [x] T006 [US1] Modify `internal/ingestor/xtream_epg_handler.go` to use `ParseTimezoneOffset()` and `NormalizeProgramTime()` for time parsing, applying inverse offset then timeshift
- [x] T007 [US1] Update XMLTV handler to set `source.DetectedTimezone` from parsed offset string in `internal/ingestor/xmltv_handler.go`
- [x] T008 [US1] Update Xtream handler to set `source.DetectedTimezone` from parsed offset in `internal/ingestor/xtream_epg_handler.go`
- [x] T009 [US1] Add structured logging for timezone detection using slog in both handlers (no emojis)
- [x] T010 [US1] Verify "now" indicator calculation uses current UTC time in `frontend/src/components/epg/CanvasEPG.tsx`

**Checkpoint**: User Story 1 complete - timezone normalization working, programs stored in UTC, "now" indicator accurate

---

## Phase 4: User Story 2 - Responsive Canvas with Accurate Time-Column Mapping (Priority: P1)

**Goal**: Canvas adapts to viewport resize while maintaining accurate time-to-pixel mapping

**Independent Test**: View 14:00-14:30 program, resize window, verify it spans exactly half the hour column

### Implementation for User Story 2

- [x] T011 [US2] Replace hardcoded `PIXELS_PER_HOUR = 200` constant with dynamic calculation in `frontend/src/components/epg/CanvasEPG.tsx`
- [x] T012 [US2] Integrate useCanvasMetrics hook into CanvasEPG component in `frontend/src/components/epg/CanvasEPG.tsx`
- [x] T013 [US2] Update program position calculation to use `calculateProgramBounds()` from useCanvasMetrics in `frontend/src/components/epg/CanvasEPG.tsx`
- [x] T014 [US2] Update hour marker rendering to use dynamic `pixelsPerHour` from useCanvasMetrics in `frontend/src/components/epg/CanvasEPG.tsx`
- [x] T015 [US2] Implement scroll position preservation using time-based storage in `frontend/src/components/epg/CanvasEPG.tsx`
- [x] T016 [US2] Add ResizeObserver with 100ms debounce for container dimensions in `frontend/src/components/epg/CanvasEPG.tsx`
- [x] T017 [US2] Enforce minimum 50px/hour threshold and enable horizontal scrolling when below in `frontend/src/components/epg/CanvasEPG.tsx`
- [x] T018 [US2] Enforce minimum 25px program width for very short programs in `frontend/src/components/epg/CanvasEPG.tsx`
- [x] T019 [US2] Update "now" indicator to recalculate position using dynamic pixelsPerHour in `frontend/src/components/epg/CanvasEPG.tsx`
- [x] T019a [US2] Implement 30-second auto-refresh interval for "now" indicator position in `frontend/src/components/epg/CanvasEPG.tsx` using setInterval with cleanup (FR-009)

**Checkpoint**: User Story 2 complete - canvas resizes correctly, programs maintain accurate time positions

---

## Phase 5: User Story 3 - Canvas Viewport Boundaries (Priority: P1)

**Goal**: Canvas stays within designated area without overlapping navigation sidebar

**Independent Test**: Open EPG with sidebar visible, verify canvas never overlaps sidebar at any viewport size

### Implementation for User Story 3

- [x] T020 [US3] Update canvas container to use ResizeObserver on container element (not window) in `frontend/src/components/epg/CanvasEPG.tsx`
- [x] T021 [US3] Modify canvas width calculation to use `containerWidth` from ResizeObserver, not viewport width, in `frontend/src/components/epg/CanvasEPG.tsx`
- [x] T022 [US3] Verify CSS layout uses flexbox/grid to prevent sidebar overlap in `frontend/src/app/epg/page.tsx`
- [x] T023 [US3] Test canvas rendering at narrow viewport (768px) to verify no overlap in `frontend/src/components/epg/CanvasEPG.tsx`

**Checkpoint**: User Story 3 complete - canvas never overlaps sidebar

---

## Phase 6: User Story 4 - Infinite Scroll / Lazy Loading EPG Data (Priority: P2)

**Goal**: Automatically load more EPG data when scrolling toward end of loaded range

**Independent Test**: Scroll EPG right toward end of 12-hour range, verify additional data loads seamlessly

### Implementation for User Story 4

- [x] T024 [US4] Create LazyLoadState interface and useLazyLoad hook in `frontend/src/components/epg/CanvasEPG.tsx` - Added props and refs for lazy loading state
- [x] T025 [US4] Implement scroll position monitoring to detect within 2-hour threshold of loaded boundary in `frontend/src/components/epg/CanvasEPG.tsx`
- [x] T026 [US4] Implement fetchMorePrograms function using `/api/v1/epg/guide` endpoint with time-range params in `frontend/src/app/epg/page.tsx`
- [x] T027 [US4] Implement mergePrograms helper to append new data without disrupting existing in `frontend/src/app/epg/page.tsx`
- [x] T028 [US4] Track `loadedEndTime` and `hasReachedEnd` to prevent redundant fetches - uses guideLoadedEndTimeRef and hasMoreGuideData state
- [x] T029 [US4] Add throttle/debounce on fetch requests to prevent API flooding during rapid scroll in `frontend/src/components/epg/CanvasEPG.tsx`
- [x] T030 [US4] Add loading indicator for lazy load in progress in `frontend/src/components/epg/CanvasEPG.tsx`

**Checkpoint**: User Story 4 complete - infinite scroll working, can view 48+ hours of EPG data

---

## Phase 7: User Story 5 - Comprehensive EPG Search (Priority: P2)

**Goal**: Search finds matches across channel names, tvg_id, program titles, and descriptions

**Independent Test**: Search for term only in program description, verify channel with matching program appears

### Implementation for User Story 5

- [x] T031 [US5] Create useEpgSearch hook in `frontend/src/components/epg/CanvasEPG.tsx` with multi-field search logic - Already implemented inline in filteredChannels useMemo
- [x] T032 [US5] Implement case-insensitive search across channel.name in `frontend/src/components/epg/CanvasEPG.tsx`
- [x] T033 [US5] Implement case-insensitive search across channel.tvg_id in `frontend/src/components/epg/CanvasEPG.tsx`
- [x] T034 [US5] Implement case-insensitive search across program.title in `frontend/src/components/epg/CanvasEPG.tsx`
- [x] T035 [US5] Implement case-insensitive search across program.description in `frontend/src/components/epg/CanvasEPG.tsx`
- [x] T036 [US5] Update search input in filter bar to use useEpgSearch hook in `frontend/src/app/epg/page.tsx` - Uses channelFilter prop passed to CanvasEPG
- [x] T037 [US5] Display "no results" message when search has no matches in `frontend/src/app/epg/page.tsx` - Added Card with Clear Search button
- [x] T038 [US5] Sanitize search input to escape special characters - React auto-escapes HTML; string matching (not regex) used

**Checkpoint**: User Story 5 complete - comprehensive search working across all fields

---

## Phase 8: User Story 6 - Manual Timeshift Adjustment (Priority: P3)

**Goal**: User-configurable timeshift applied after UTC normalization

**Independent Test**: Set +1 hour timeshift, re-ingest, verify programs shifted 1 hour later

### Implementation for User Story 6

- [x] T039 [US6] Verify `EpgShift` field is applied in `NormalizeProgramTime()` after UTC normalization in `internal/ingestor/timezone.go`
- [x] T040 [US6] Verify EPG source edit UI allows configuring timeshift (-12 to +12 hours) - check existing UI in `frontend/src/` - Added min/max constraints
- [x] T041 [US6] Add validation for timeshift range (-12 to +12) in EPG source handler in `internal/http/handlers/types.go` - Added Huma validation tags

**Checkpoint**: User Story 6 complete - manual timeshift works correctly

---

## Phase 9: User Story 7 - Simplified UI (Priority: P3)

**Goal**: Remove unnecessary timezone selector dropdown from filter bar

**Independent Test**: Open EPG, verify timezone dropdown is not present in filter bar

### Implementation for User Story 7

- [x] T042 [US7] Remove timezone selector dropdown from EPG filter bar in `frontend/src/app/epg/page.tsx`
- [x] T043 [US7] Remove timezone state/handler code from EPG page in `frontend/src/app/epg/page.tsx`
- [x] T044 [US7] Verify times display in user's local browser timezone automatically in `frontend/src/components/epg/CanvasEPG.tsx`

**Checkpoint**: User Story 7 complete - UI simplified, no timezone dropdown

---

## Phase 10: Polish & Cross-Cutting Concerns

**Purpose**: Final cleanup and verification

- [x] T045 [P] Verify all error cases handled: network errors in lazy load (try/catch in fetchMoreGuideData), invalid timezone formats (ParseTimezoneOffset handles), empty search (returns all)
- [x] T046 [P] Verify edge cases: very short programs (MIN_PROGRAM_WIDTH=25px), very long programs (spans columns), programs spanning midnight (UTC comparisons)
- [x] T047 [P] Verify performance: 100ms resize debounce, RAF canvas rendering, 300ms search debounce
- [ ] T048 Run manual validation per quickstart.md scenarios - Requires user testing
- [ ] T049 Verify no console errors or warnings in browser dev tools - Requires user testing

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phase 3-9)**: All depend on Foundational phase completion
- **Polish (Phase 10)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Phase 2 - No dependencies on other stories
- **User Story 2 (P1)**: Can start after Phase 2 - Independent of US1
- **User Story 3 (P1)**: Can start after Phase 2 - May benefit from US2 work but independent
- **User Story 4 (P2)**: Can start after Phase 2 - Benefits from US2/US3 canvas work
- **User Story 5 (P2)**: Can start after Phase 2 - Independent
- **User Story 6 (P3)**: Can start after Phase 2 - Depends on US1 timezone work
- **User Story 7 (P3)**: Can start after Phase 2 - Independent

### Within Each User Story

- Backend changes before frontend changes (for US1)
- Core canvas changes before auxiliary features (for US2-US5)
- UI removal is independent (for US7)

### Parallel Opportunities

**Phase 1 (Setup)**:
- T001 and T002 can run in parallel (different tech stacks)

**Phase 2 (Foundational)**:
- T003a and T003b can run in parallel (different tech stacks: Go vs TypeScript)
- T003 must complete before T003a (same file)
- T004 independent of test tasks

**P1 User Stories (US1, US2, US3)**:
- US1 (backend timezone) and US2/US3 (frontend canvas) can run in parallel
- US2 and US3 overlap significantly - likely sequential

**P2 User Stories (US4, US5)**:
- US4 and US5 can run in parallel (lazy load vs search)

**P3 User Stories (US6, US7)**:
- US6 and US7 can run in parallel (timeshift vs UI removal)

---

## Parallel Example: Setup Phase

```bash
# Launch both setup tasks together:
Task: "Create timezone parsing utility in internal/ingestion/timezone.go"
Task: "Create useCanvasMetrics hook in frontend/src/hooks/useCanvasMetrics.ts"
```

## Parallel Example: P1 User Stories

```bash
# Backend (US1) and Frontend (US2/US3) can run in parallel:
Task: "[US1] Modify internal/ingestion/xmltv_handler.go..."
Task: "[US2] Replace hardcoded PIXELS_PER_HOUR constant..."
```

---

## Implementation Strategy

### MVP First (User Stories 1, 2, 3 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL - blocks all stories)
3. Complete Phase 3: User Story 1 (timezone normalization)
4. Complete Phase 4: User Story 2 (responsive canvas)
5. Complete Phase 5: User Story 3 (viewport boundaries)
6. **STOP and VALIDATE**: Test all P1 stories independently
7. Deploy/demo if ready

### Incremental Delivery

1. Complete Setup + Foundational -> Foundation ready
2. Add User Story 1 -> Test independently -> "Now" indicator fixed
3. Add User Stories 2 + 3 -> Test independently -> Canvas layout fixed
4. Add User Story 4 -> Test independently -> Infinite scroll working
5. Add User Story 5 -> Test independently -> Comprehensive search working
6. Add User Stories 6 + 7 -> Test independently -> Final polish
7. Each story adds value without breaking previous stories

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together
2. Once Foundational is done:
   - Developer A: User Stories 1 + 6 (backend timezone work)
   - Developer B: User Stories 2 + 3 (canvas layout)
   - Developer C: User Stories 4 + 5 (lazy load + search)
   - Developer D: User Story 7 (UI cleanup)
3. Stories complete and integrate independently

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- Avoid: vague tasks, same file conflicts, cross-story dependencies that break independence
- No database migrations required - behavior changes only
- Re-ingestion required for existing EPG sources to apply new timezone normalization
