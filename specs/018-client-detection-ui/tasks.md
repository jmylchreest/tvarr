# Tasks: Client Detection UI Improvements

**Input**: Design documents from `/specs/018-client-detection-ui/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- **Backend**: `internal/` at repository root (Go)
- **Frontend**: `frontend/src/` (TypeScript/React)

---

## Phase 1: Setup

**Purpose**: Project initialization and dependency setup

- [x] T001 Checkout feature branch `018-client-detection-ui`
- [x] T002 [P] Install Fuse.js dependency in frontend (`cd frontend && pnpm add fuse.js`)
- [x] T003 [P] Verify existing expression editor components are available in frontend/src/components/

---

## Phase 2: Foundational

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T004 Review existing expression editor infrastructure in frontend/src/components/expression-editor.tsx
- [x] T005 [P] Review existing validation badges in frontend/src/components/expression-validation-badges.tsx
- [x] T006 [P] Review useHelperAutocomplete hook in frontend/src/hooks/useHelperAutocomplete.ts
- [x] T007 Review routing decision logic in internal/relay/routing_decision.go

**Checkpoint**: Foundation ready - user story implementation can now begin

---

## Phase 3: User Story 1 - Fix Export/Import Functionality (Priority: P1) 🎯 MVP

**Goal**: Fix 405 error on export/import by correcting frontend API client URLs

**Independent Test**: Click Export button on filters/data-mapping/client-detection pages, verify JSON downloads without error

### Implementation for User Story 1

- [x] T008 [US1] Fix exportFilters URL in frontend/src/lib/api-client.ts (change to `/api/v1/export/filters`)
- [x] T009 [US1] Fix exportDataMappingRules URL in frontend/src/lib/api-client.ts (change to `/api/v1/export/data-mapping-rules`)
- [x] T010 [US1] Fix exportClientDetectionRules URL in frontend/src/lib/api-client.ts (change to `/api/v1/export/client-detection-rules`)
- [x] T011 [US1] Fix exportEncodingProfiles URL in frontend/src/lib/api-client.ts (change to `/api/v1/export/encoding-profiles`)
- [x] T012 [US1] Fix importFiltersPreview URL in frontend/src/lib/api-client.ts (change to `/api/v1/import/filters/preview`)
- [x] T013 [US1] Fix importFilters URL in frontend/src/lib/api-client.ts (change to `/api/v1/import/filters`)
- [x] T014 [US1] Fix importDataMappingRulesPreview URL in frontend/src/lib/api-client.ts (change to `/api/v1/import/data-mapping-rules/preview`)
- [x] T015 [US1] Fix importDataMappingRules URL in frontend/src/lib/api-client.ts (change to `/api/v1/import/data-mapping-rules`)
- [x] T016 [US1] Fix importClientDetectionRulesPreview URL in frontend/src/lib/api-client.ts (change to `/api/v1/import/client-detection-rules/preview`)
- [x] T017 [US1] Fix importClientDetectionRules URL in frontend/src/lib/api-client.ts (change to `/api/v1/import/client-detection-rules`)
- [x] T018 [US1] Fix importEncodingProfilesPreview URL in frontend/src/lib/api-client.ts (change to `/api/v1/import/encoding-profiles/preview`)
- [x] T019 [US1] Fix importEncodingProfiles URL in frontend/src/lib/api-client.ts (change to `/api/v1/import/encoding-profiles`)

**Checkpoint**: Export/Import works for all config types

---

## Phase 4: User Story 2 - Copyable Expressions in List View (Priority: P2)

**Goal**: Enable click-to-copy for expressions in client detection rules list

**Independent Test**: Click on an expression in the list, verify it copies to clipboard with visual feedback

### Implementation for User Story 2

- [x] T020 [US2] Create CopyableExpression component in frontend/src/components/client-detection-rules.tsx
- [x] T021 [US2] Add click handler with navigator.clipboard.writeText() in CopyableExpression
- [x] T022 [US2] Add visual feedback (Copy/Check icons) with useState for copied state
- [x] T023 [US2] Replace expression display in list view with CopyableExpression component
- [x] T024 [US2] Add "Click to copy" tooltip to expression elements

**Checkpoint**: Expressions can be copied from list view

---

## Phase 5: User Story 3 - Intellisense/Autocomplete in Expression Editor (Priority: P2)

**Goal**: Add @dynamic() helper autocomplete to client detection expression editor

**Independent Test**: Type "@" in expression editor, verify autocomplete popup appears with @dynamic() option

### Implementation for User Story 3

- [x] T025 [P] [US3] Add CLIENT_DETECTION_HELPERS constant in frontend/src/lib/expression-constants.ts
- [x] T026 [P] [US3] Add HEADER_COMPLETIONS constant for request.headers sub-completions in frontend/src/lib/expression-constants.ts
- [x] T027 [US3] Create client-detection-expression-editor.tsx in frontend/src/components/
- [x] T028 [US3] Integrate ExpressionEditor base component in client-detection-expression-editor.tsx
- [x] T029 [US3] Integrate useHelperAutocomplete hook with CLIENT_DETECTION_HELPERS
- [x] T030 [US3] Add nested completion logic for @dynamic(request.headers, ...) sub-completions
- [x] T031 [US3] Replace Textarea in client-detection-rules.tsx with ClientDetectionExpressionEditor

**Checkpoint**: Autocomplete works when typing @ in expression editor

---

## Phase 6: User Story 4 - Validation Badges in Expression Editor (Priority: P2)

**Goal**: Display validation badges in client detection expression editor matching filter/data-mapping editors

**Independent Test**: Enter valid/invalid expressions, verify badges show success/error states with tooltips

### Implementation for User Story 4

- [x] T032 [US4] Import ValidationBadges component into client-detection-expression-editor.tsx
- [x] T033 [US4] Connect validation state from ExpressionEditor to ValidationBadges
- [x] T034 [US4] Add onValidationChange callback handler in client-detection-expression-editor.tsx
- [x] T035 [US4] Style ValidationBadges container to match filter/data-mapping editors

**Checkpoint**: Validation badges show correct states for expressions

---

## Phase 7: User Story 5 - Default System Rules for Popular Media Players (Priority: P3)

**Goal**: Add pre-configured system rules for VLC, MPV, Kodi, Plex, Jellyfin, Emby

**Independent Test**: Run migration, verify 6 system rules exist in database with is_system=true

### Implementation for User Story 5

- [x] T036 [US5] Create migration_013_system_client_detection_rules.go in internal/database/migrations/
- [x] T037 [US5] Implement VLC system rule with h264/h265 video and aac/ac3/eac3/mp3 audio
- [x] T038 [US5] Implement MPV system rule with h264/h265/av1/vp9 video and aac/ac3/eac3/opus/mp3 audio
- [x] T039 [US5] Implement Kodi system rule with h264/h265/av1/vp9 video and aac/ac3/eac3/dts/mp3 audio
- [x] T040 [US5] Implement Plex system rule with passthrough configuration (empty codec arrays)
- [x] T041 [US5] Implement Jellyfin system rule with passthrough configuration
- [x] T042 [US5] Implement Emby system rule with passthrough configuration
- [x] T043 [US5] Add migration013 to AllMigrations() in internal/database/migrations/registry.go
- [x] T044 [US5] Add "System" badge styling for is_system=true rules in frontend/src/components/client-detection-rules.tsx
- [x] T045 [US5] Disable delete button for system rules in client-detection-rules.tsx

**Checkpoint**: System rules exist after migration and display correctly in UI

---

## Phase 8: User Story 6 - Smart Remuxing vs Transcoding (Priority: P2)

**Goal**: Relay remuxes instead of transcoding when source codecs are compatible with target container

**Independent Test**: Play MPEG-TS HEVC/EAC3 stream with HLS output, verify FFmpeg uses -c:v copy -c:a copy

### Implementation for User Story 6

- [x] T046 [US6] Create codec_compatibility.go in internal/relay/
- [x] T047 [US6] Define ContainerFormat type and constants (mpegts, hls-ts, hls-fmp4, dash)
- [x] T048 [US6] Define CodecCompatibility map for each container format
- [x] T049 [US6] Implement IsCodecCompatible() function
- [x] T050 [US6] Implement AreCodecsCompatible() function for checking all codecs
- [x] T051 [US6] Modify Decide() in routing_decision.go to check codec compatibility before RouteTranscode
- [x] T052 [US6] Add RouteRepackage route when source codecs are compatible with target container
- [x] T053 [US6] Add structured logging for routing decisions in routing_decision.go

**Checkpoint**: HEVC/EAC3 streams remux to HLS without transcoding

---

## Phase 9: User Story 7 - Fuzzy Search in Channel and EPG Browsers (Priority: P2)

**Goal**: Add fuzzy/partial matching to channel and EPG search with typo tolerance

**Independent Test**: Search for "Desicovery" (typo), verify "Discovery" channels appear in results

### Implementation for User Story 7

- [x] T054 [P] [US7] Create fuzzy-search.ts utility in frontend/src/lib/
- [x] T055 [P] [US7] Define FuzzySearchResult and FuzzySearchOptions types in fuzzy-search.ts
- [x] T056 [US7] Implement createChannelFuse() function with weighted keys (channel_name, tvg_name, tvg_id, group_title, channel_number, ext_id)
- [x] T057 [US7] Implement createEpgFuse() function with weighted keys (title, sub_title, description, channel_id, category)
- [x] T058 [US7] Implement highlightMatches() utility for showing matched fields
- [x] T059 [US7] Integrate Fuse.js in frontend/src/app/channels/page.tsx
- [x] T060 [US7] Add fuzzy filtering to channel search results in channels/page.tsx
- [x] T061 [US7] Display match indicators showing which field matched in channel list
- [x] T062 [US7] Integrate Fuse.js in frontend/src/app/epg/page.tsx
- [x] T063 [US7] Add fuzzy filtering to EPG search results in epg/page.tsx
- [x] T064 [US7] Display match indicators showing which field matched in EPG list
- [x] T065 [US7] Add minimum search length validation (2 characters) with user feedback

**Checkpoint**: Fuzzy search works in both channel and EPG browsers

---

## Phase 10: Polish & Cross-Cutting Concerns

**Purpose**: Final cleanup and validation

- [x] T066 [P] Verify all export/import operations work via UI testing
- [x] T067 [P] Verify expression editor autocomplete and validation across all rule types
- [x] T068 [P] Verify fuzzy search performance with large datasets
- [x] T069 Run quickstart.md validation checklist
- [x] T070 Code cleanup: remove any console.log statements
- [x] T071 Verify no TypeScript errors (`npx tsc --noEmit` in frontend/)
- [x] T072 Verify Go build succeeds (`task build`)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion
- **User Stories (Phase 3-9)**: All depend on Foundational phase completion
- **Polish (Phase 10)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational - No dependencies on other stories
- **User Story 2 (P2)**: Can start after Foundational - No dependencies on other stories
- **User Story 3 (P2)**: Can start after Foundational - No dependencies on other stories
- **User Story 4 (P2)**: Depends on User Story 3 (US3 creates the expression editor component)
- **User Story 5 (P3)**: Can start after Foundational - No dependencies on other stories
- **User Story 6 (P2)**: Can start after Foundational - No dependencies on other stories
- **User Story 7 (P2)**: Can start after Foundational - No dependencies on other stories

### Parallel Opportunities

**After Foundational completes, these can run in parallel:**
- User Story 1 (Export/Import fix) - frontend only
- User Story 2 (Copyable expressions) - frontend only
- User Story 3 (Intellisense) - frontend only
- User Story 5 (System rules) - backend migration + frontend styling
- User Story 6 (Smart remuxing) - backend only
- User Story 7 (Fuzzy search) - frontend only

**Sequential:**
- User Story 4 must follow User Story 3

---

## Parallel Example: Frontend Stories

```bash
# These can all run in parallel after Foundational:
Task: T008-T019 [US1] Fix export/import URLs in api-client.ts
Task: T020-T024 [US2] Copyable expressions in client-detection-rules.tsx
Task: T025-T031 [US3] Expression editor with autocomplete
Task: T054-T065 [US7] Fuzzy search in channel/EPG pages
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational
3. Complete Phase 3: User Story 1 (Export/Import Fix)
4. **STOP and VALIDATE**: Test export/import on all config pages
5. Deploy if needed - critical bug fix complete

### Incremental Delivery

1. Complete Setup + Foundational → Foundation ready
2. Add User Story 1 → Test independently → Export/Import working
3. Add User Stories 2-4 together → Full expression editor experience
4. Add User Story 5 → System rules available
5. Add User Story 6 → Smart remuxing working
6. Add User Story 7 → Fuzzy search enabled
7. Polish → Production ready

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- User Story 1 is the MVP - fixes critical broken functionality
