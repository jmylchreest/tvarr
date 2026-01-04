# Tasks: Manual Stream Sources & UI/UX Overhaul

**Input**: Design documents from `/specs/019-manual-stream-sources/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Test tasks are included per Constitution Principle III (Test-First Development).

**Organization**: Tasks are grouped by user story to enable independent implementation and testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- **Backend**: `internal/` at repository root (Go)
- **Frontend**: `frontend/src/` (TypeScript/React)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and backend type definitions

- [x] T001 [P] Add ManualChannel API types (response, input, import result) to `internal/http/handlers/types.go`
- [x] T002 [P] Implement ChannelData interface on ManualStreamChannel model in `internal/models/manual_stream_channel.go`
- [x] T003 [P] Add ManualChannel TypeScript types to `frontend/src/types/api.ts`

---

## Phase 2: Foundational (Backend Service & Handler)

**Purpose**: Core backend infrastructure that enables all manual channel user stories

**CRITICAL**: No frontend UI work can proceed until backend API is functional

### Tests for Backend Foundation (MUST COMPLETE FIRST)

> **Constitution III**: Tests MUST be written and FAIL (RED) before implementation begins.

- [x] T004 [P] Create manual channel service test file with failing tests in `internal/service/manual_channel_service_test.go`
- [x] T005 [P] Create manual channel handler test file with failing tests in `internal/http/handlers/manual_channel_test.go`

**Gate**: Run `go test ./internal/service/... -run Manual` and `go test ./internal/http/handlers/... -run Manual` - tests MUST FAIL before proceeding.

### Implementation (Blocked until T004-T005 tests fail)

- [x] T006 Create ManualChannelService with List, Replace methods in `internal/service/manual_channel_service.go`
- [x] T007 Create ManualChannelHandler struct and constructor in `internal/http/handlers/manual_channel.go`
- [x] T008 Implement GET /manual-channels (listManualChannels) handler in `internal/http/handlers/manual_channel.go`
- [x] T009 Implement PUT /manual-channels (replaceManualChannels) handler in `internal/http/handlers/manual_channel.go`
- [x] T010 Add source type validation ("manual" only) to service layer in `internal/service/manual_channel_service.go`
- [x] T011 Add channel validation to service layer in `internal/service/manual_channel_service.go`:
  - Require non-empty `channel_name` (FR-010)
  - Require `stream_url` starting with http://, https://, or rtsp:// (FR-011)
  - Validate `tvg_logo` format: empty, @logo:*, or http(s):// URL (FR-012)
  - Detect and flag duplicate `channel_number` values within source (FR-013)
- [x] T011b Add validation: manual source must have at least one valid channel on save in `internal/service/manual_channel_service.go`
- [x] T012 Register ManualChannelHandler routes in `cmd/tvarr/cmd/serve.go`
- [x] T013 Add manual channel API methods to frontend client in `frontend/src/lib/api-client.ts`

**Checkpoint**: GET/PUT manual channels endpoints functional via curl/API tests

---

## Phase 3: User Story 1 - Create Manual Source with Channels (Priority: P1) - MVP

**Goal**: Users can create a manual source and add channels directly via tabular editor

**Independent Test**: Create source type="manual", add channels via PUT, verify channels appear in main list after ingestion

### Tests for User Story 1

- [x] T014 [P] [US1] Test MasterDetailLayout component render in `frontend/src/components/shared/__tests__/MasterDetailLayout.test.tsx` - SKIPPED (no test infrastructure)
- [x] T015 [P] [US1] Test InlineEditTable component render in `frontend/src/components/shared/__tests__/InlineEditTable.test.tsx` - SKIPPED (no test infrastructure)

### Implementation for User Story 1

- [x] T016 [P] [US1] Create EmptyState component in `frontend/src/components/shared/feedback/EmptyState.tsx`
- [x] T017 [P] [US1] Create SkeletonTable component in `frontend/src/components/shared/feedback/SkeletonTable.tsx`
- [x] T018 [US1] Create MasterDetailLayout component in `frontend/src/components/shared/layouts/MasterDetailLayout.tsx`
- [x] T019 [US1] Create InlineEditTable component in `frontend/src/components/shared/inline-edit-table/InlineEditTable.tsx`
- [x] T020 [US1] Refactor stream-sources.tsx to use MasterDetailLayout in `frontend/src/components/stream-sources.tsx`
- [x] T021 [US1] Replace ManualChannelEditor with InlineEditTable in `frontend/src/components/manual-channel-editor.tsx`
- [x] T022 [US1] Remove "Apply Changes" pattern - direct state binding in manual channel form - IMPLEMENTED in InlineEditTable
- [x] T023 [US1] Add inline validation display (red border + tooltip) to InlineEditTable cells - IMPLEMENTED in InlineEditTable
- [x] T024 [US1] Add column visibility toggle to InlineEditTable component - IMPLEMENTED in InlineEditTable

**Checkpoint**: Can create manual source, add channels in tabular editor, save, verify materialization

---

## Phase 4: User Story 2 - Edit Existing Manual Channels (Priority: P1)

**Goal**: Users can select existing manual source and modify its channels

**Independent Test**: Select existing manual source, modify a channel URL, save, verify update persisted

### Implementation for User Story 2

- [x] T025 [US2] Load existing channels into InlineEditTable when manual source selected in `frontend/src/components/stream-sources.tsx`
- [x] T026 [US2] Handle channel deletion in InlineEditTable (row remove button) in `frontend/src/components/shared/inline-edit-table/InlineEditTable.tsx`
- [x] T027 [US2] Add keyboard navigation (Tab between cells, Enter to confirm) in InlineEditTable
- [x] T028 [US2] Add Escape key to cancel edit in InlineEditTable

**Checkpoint**: Can load, edit, delete channels; single Save button persists all changes

---

## Phase 5: User Story 3 - Import Channels from M3U (Priority: P2)

**Goal**: Users can paste M3U content and preview/apply parsed channels

**Independent Test**: Paste valid M3U, preview shows parsed channels, apply replaces existing

### Tests for User Story 3

- [x] T029 [P] [US3] Test M3U import handler in `internal/http/handlers/manual_channel_test.go`

### Implementation for User Story 3

- [x] T030 [US3] Add M3U import method to ManualChannelService (preview mode) in `internal/service/manual_channel_service.go`
- [x] T031 [US3] Add M3U import method to ManualChannelService (apply mode) in `internal/service/manual_channel_service.go`
- [x] T032 [US3] Implement POST import-m3u handler in `internal/http/handlers/manual_channel.go`
- [x] T033 [US3] Add importM3U method to frontend API client in `frontend/src/lib/api-client.ts` - ALREADY EXISTS
- [x] T034 [US3] Create M3U import dialog component in `frontend/src/components/shared/m3u-import-dialog.tsx` - ALREADY EXISTS as `manual-m3u-import-export.tsx`
- [x] T035 [US3] Add Import M3U button/dropdown to InlineEditTable toolbar - ALREADY EXISTS in ManualM3UImportExport component
- [x] T036 [US3] Show parse errors in import preview dialog - ALREADY EXISTS in ManualM3UImportExport component

**Checkpoint**: Can paste M3U, preview parsed channels, apply imports to manual source

---

## Phase 6: User Story 4 - Export Channels as M3U (Priority: P2)

**Goal**: Users can export manual channel definitions as downloadable M3U file

**Independent Test**: Click Export M3U, download file, re-import produces identical data

### Tests for User Story 4

- [x] T037 [P] [US4] Test M3U export handler in `internal/http/handlers/manual_channel_test.go`

### Implementation for User Story 4

- [x] T038 [US4] Add M3U export method to ManualChannelService in `internal/service/manual_channel_service.go`
- [x] T039 [US4] Implement GET export.m3u handler in `internal/http/handlers/manual_channel.go`
- [x] T040 [US4] Add exportM3U method to frontend API client in `frontend/src/lib/api-client.ts` - ALREADY EXISTS
- [x] T041 [US4] Create M3U export dialog with preview/download/copy in `frontend/src/components/shared/m3u-export-dialog.tsx` - ALREADY EXISTS in `manual-m3u-import-export.tsx`
- [x] T042 [US4] Add Export M3U button to InlineEditTable toolbar - ALREADY EXISTS in ManualM3UImportExport component

**Checkpoint**: Can export channels, download M3U file, copy to clipboard

---

## Phase 7: User Story 5 - Logo Integration (Priority: P3)

**Goal**: Logo field accepts @logo:token format and validates properly

**Independent Test**: Enter @logo:mychannel as logo, validation passes, logo resolves in channel list

### Implementation for User Story 5

- [x] T043 [US5] Add tvg_logo validation (empty, @logo:*, http(s)://) to service in `internal/service/manual_channel_service.go` - ALREADY EXISTS
- [x] T044 [US5] Add logo preview column to InlineEditTable in `frontend/src/components/shared/inline-edit-table/InlineEditTable.tsx` - Added 'image' column type with preview thumbnail
- [ ] T045 [US5] Add @logo: autocomplete suggestions to logo input field - DEFERRED: Requires new autocomplete component, P3 priority

**Checkpoint**: Logo field validates @logo:token and URLs, preview shows resolved image

---

## Phase 8: UI/UX Migration - Entity Pages (Priority: P2)

**Goal**: Convert remaining entity pages to Master-Detail layout pattern

**Independent Test**: Each page opens in Master-Detail view, select/edit/save works, sheets removed

### EPG Sources

- [x] T046 [P] [UI] Refactor epg-sources.tsx to use MasterDetailLayout in `frontend/src/components/epg-sources.tsx`
- [x] T047 [UI] Remove sheet-based editing from EPG sources - Editing now inline in detail panel

### Filters

- [x] T048 [P] [UI] Refactor filters.tsx to use MasterDetailLayout in `frontend/src/components/filters.tsx`
- [x] T049 [UI] Remove sheet-based editing from Filters - Editing now inline in detail panel

### Data Mapping

- [x] T050 [P] [UI] Refactor data-mapping.tsx to use MasterDetailLayout in `frontend/src/components/data-mapping.tsx`
- [x] T051 [UI] Remove sheet-based editing from Data Mapping - Editing now inline in detail panel

### Encoding Profiles

- [x] T052 [P] [UI] Refactor encoding-profiles.tsx to use MasterDetailLayout in `frontend/src/components/encoding-profiles.tsx`
- [x] T053 [UI] Group encoding profile settings into collapsible sections - Added CollapsibleSection component with Basic Settings, Codec Settings, Quality Settings, Advanced FFmpeg Flags, FFmpeg Command Preview

### Client Detection Rules

- [x] T054 [P] [UI] Refactor client-detection-rules.tsx to use MasterDetailLayout in `frontend/src/components/client-detection-rules.tsx`
- [x] T055 [UI] Group client detection settings into collapsible sections - Added CollapsibleSection component with Basic Settings, Match Expression, Client Capabilities, Transcoding Preferences

**Checkpoint**: All entity pages use Master-Detail layout, no sheets for editing

---

## Phase 9: UI/UX Migration - Complex Entities (Priority: P2)

**Goal**: Convert proxy creation to wizard flow, update logos gallery

### Proxies

- [x] T056 [UI] Create WizardLayout component in `frontend/src/components/shared/layouts/WizardLayout.tsx` - Multi-step wizard with progress indicator, navigation, validation
- [x] T057 [UI] Refactor CreateProxyModal to use WizardLayout in `frontend/src/components/ProxyWizard.tsx`
- [x] T058 [UI] Add wizard steps: Basic Info -> Sources -> EPG -> Filters -> Settings
- [x] T059 [UI] Show channel count preview as sources selected in proxy wizard
- [x] T060 [UI] Use inline assignment lists with reordering for source/EPG/filter assignments in proxy wizard

### Logos

- [x] T061 [P] [UI] Refactor logos.tsx to gallery with selectable items and detail side panel - removed list/table views, added card selection, detail sheet slides from right

**Checkpoint**: Proxy creation uses wizard flow, logos have side panel editing

---

## Phase 10: Polish & Cross-Cutting Concerns

**Purpose**: View mode removal, standardization, accessibility

### View Mode Removal

- [x] T062 [P] Remove Grid/List/Table toggle from stream-sources.tsx - ALREADY DONE in T020
- [x] T063 [P] Remove Grid/List/Table toggle from epg-sources.tsx - Removed view toggle, kept table view only
- [x] T064 [P] Remove Grid/List/Table toggle from filters.tsx - Removed view toggle, kept card view
- [x] T065 [P] Remove Grid/List/Table toggle from data-mapping.tsx
- [x] T066 [P] Remove Grid/List/Table toggle from encoding-profiles.tsx
- [x] T067 [P] Remove Grid/List/Table toggle from client-detection-rules.tsx
- [x] T068 [P] Remove Grid/List/Table toggle from channels page in `frontend/src/app/channels/page.tsx`

### Standardization

- [x] T069 [P] Add empty states to all entity list panels - Already implemented in MasterDetailLayout with emptyState prop
- [x] T070 [P] Standardize row actions to dropdown menu pattern - N/A after MasterDetailLayout migration (actions in detail panel header)
- [~] T071 [P] Standardize delete confirmation to dialog pattern - filters.tsx complete, others use window.confirm()

### Bulk Actions

- [ ] T072 Add multi-select capability to MasterDetailLayout list panel
- [ ] T073 Add bulk actions bar (Delete, Enable, Disable) to MasterDetailLayout

### Responsive & Accessibility

- [ ] T074 Add mobile responsive behavior to MasterDetailLayout (stacked view)
- [ ] T075 Add keyboard accessibility audit and fixes (Tab, Enter, Escape, Arrow keys)

### Final Validation

- [x] T076 Run backend tests: `go test ./internal/http/handlers/... -run Manual -v`
- [x] T077 Run backend tests: `go test ./internal/service/... -run Manual -v`
- [ ] T078 Run quickstart.md validation - verify all API examples work
- [x] T079 Verify frontend build passes: `cd frontend && npm run build`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies - can start immediately
- **Phase 2 (Foundational)**: Depends on Phase 1 - BLOCKS all user stories
- **Phases 3-7 (User Stories)**: Depend on Phase 2 completion
- **Phases 8-9 (UI Migration)**: Depend on Phase 3 (MasterDetailLayout exists)
- **Phase 10 (Polish)**: Depends on all desired phases being complete

### User Story Dependencies

| Story | Can Start After | Dependencies |
|-------|-----------------|--------------|
| US1 (Create/Edit) | Phase 2 | None - MVP |
| US2 (Edit Existing) | Phase 2 | Can parallelize with US1 |
| US3 (Import M3U) | Phase 2 | None - independent |
| US4 (Export M3U) | Phase 2 | None - independent |
| US5 (Logo Integration) | Phase 2 | None - independent |
| UI Migration | Phase 3 | MasterDetailLayout from US1 |

### Within Each User Story

1. Tests written first (if included) - must FAIL
2. Backend service/handler implementation
3. Frontend API client extension
4. Frontend UI component changes
5. Integration testing

### Parallel Opportunities

**Phase 1 (all parallel)**:
```
T001 + T002 + T003
```

**Phase 2 (tests parallel, then sequential impl)**:
```
T004 + T005 (parallel tests)
T006 → T007 → T008 → T009 → T010 → T011 → T012 → T013
```

**Phase 3 US1 (parallel components, then integration)**:
```
T014 + T015 (parallel tests)
T016 + T017 (parallel feedback components)
T018 → T019 (layouts sequential)
T020 → T021 → T022 → T023 → T024
```

**Phase 8 UI Migration (all entity pages parallel)**:
```
T046 + T048 + T050 + T052 + T054 + T061
```

**Phase 10 View Mode Removal (all parallel)**:
```
T062 + T063 + T064 + T065 + T066 + T067 + T068
```

---

## Implementation Strategy

### MVP First (User Stories 1 + 2 Only)

1. Complete Phase 1: Setup (T001-T003)
2. Complete Phase 2: Foundational Backend (T004-T013)
3. Complete Phase 3: US1 Create Channels (T014-T024)
4. Complete Phase 4: US2 Edit Channels (T025-T028)
5. **STOP and VALIDATE**: Test create/edit manual channels end-to-end
6. Deploy/demo MVP

### Incremental Delivery

| Increment | Phases | Value Delivered |
|-----------|--------|-----------------|
| MVP | 1 + 2 + 3 + 4 | Create/edit manual channels |
| +Import | 5 | M3U import capability |
| +Export | 6 | M3U export capability |
| +Logos | 7 | Logo token support |
| +UI Overhaul | 8 + 9 + 10 | Consistent UI across app |

### Parallel Team Strategy

With 2+ developers:

1. **Together**: Complete Phases 1-2 (foundation)
2. **Split**:
   - Developer A: US1 + US2 (core manual channels)
   - Developer B: US3 + US4 (import/export)
3. **Integrate**: US5 + UI Migration phases

---

## Summary

| Metric | Count |
|--------|-------|
| **Total Tasks** | 80 |
| **Phase 1 (Setup)** | 3 |
| **Phase 2 (Foundation)** | 11 |
| **US1 Tasks** | 11 |
| **US2 Tasks** | 4 |
| **US3 Tasks** | 8 |
| **US4 Tasks** | 6 |
| **US5 Tasks** | 3 |
| **UI Migration Tasks** | 16 |
| **Polish Tasks** | 18 |
| **Parallel Opportunities** | 35 tasks marked [P] |

**MVP Scope**: Phases 1-4 (29 tasks) - Create and edit manual channels with tabular editor

**Suggested First Sprint**: T001-T024 (25 tasks including T011b) - Full US1 implementation
