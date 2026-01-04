# Tasks: Frontend Theme Polish

**Input**: Design documents from `/specs/011-frontend-theme-polish/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Tests are NOT explicitly requested in the specification. Tasks focus on implementation.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3, US4)
- Include exact file paths in descriptions

## Path Conventions

- **Backend**: `internal/` (Go)
- **Frontend**: `frontend/src/` (Next.js/TypeScript)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and structural preparation

- [ ] T001 Create backend themes directory structure at internal/assets/themes/
- [ ] T002 [P] Define Theme types and constants in internal/models/theme.go
- [ ] T003 [P] Create theme service interface in internal/service/interfaces.go (add theme service interface)

---

## Phase 2: Foundational (Backend Theme Service)

**Purpose**: Core backend infrastructure that enables all user stories

**CRITICAL**: No user story work can begin until this phase is complete

- [ ] T004 Implement ThemeService in internal/service/theme_service.go with methods: ListThemes, GetThemeCSS, ValidateTheme
- [ ] T005 [P] Add CSS variable validation regex patterns in internal/service/theme_service.go
- [ ] T006 [P] Add color extraction logic for theme previews in internal/service/theme_service.go
- [ ] T007 Create theme HTTP handlers in internal/http/handlers/theme.go with endpoints: GET /api/v1/themes, GET /api/v1/themes/{id}.css
- [ ] T008 Register theme routes in internal/http/server.go (add theme handler registration)
- [ ] T009 [P] Add $DATA/themes/ directory creation on startup in cmd/tvarr/cmd/serve.go

**Checkpoint**: Backend theme API ready - frontend work can now begin

---

## Phase 3: User Story 1 - Seamless Dark Mode Navigation (Priority: P1)

**Goal**: Fix the white "flashbang" effect during page navigation in dark mode by ensuring theme is applied before first paint

**Independent Test**: Navigate between any two pages in dark mode - no white flash should be visible

### Implementation for User Story 1

- [ ] T010 [US1] Refactor enhanced-theme-script.ts to be fully synchronous in frontend/src/lib/enhanced-theme-script.ts
- [ ] T011 [US1] Update layout.tsx to load theme CSS link in head before script in frontend/src/app/layout.tsx
- [ ] T012 [US1] Add inline background-color style on html element in frontend/src/app/layout.tsx
- [ ] T013 [US1] Update enhanced-theme-provider.tsx to not duplicate theme loading in frontend/src/components/enhanced-theme-provider.tsx
- [ ] T014 [US1] Ensure all page components use bg-background class (audit frontend/src/app/*/page.tsx files)
- [ ] T015 [US1] Test dark mode navigation across Dashboard, Channels, Sources, Proxies pages

**Checkpoint**: User Story 1 complete - dark mode navigation should have no white flashes

---

## Phase 4: User Story 2 - Custom Theme File Support (Priority: P2)

**Goal**: Enable users to add custom CSS theme files to $DATA/themes/ directory and have them appear in the theme selector

**Independent Test**: Place a valid .css theme file in $DATA/themes/ and verify it appears in theme selector

### Implementation for User Story 2

- [ ] T016 [US2] Update ThemeService.ListThemes to scan $DATA/themes/ directory in internal/service/theme_service.go
- [ ] T017 [US2] Implement theme file validation (--background, --foreground, --primary required) in internal/service/theme_service.go
- [ ] T018 [P] [US2] Add caching headers (ETag, Last-Modified, Cache-Control) to theme CSS endpoint in internal/http/handlers/theme.go
- [ ] T019 [US2] Update enhanced-theme-provider.tsx to fetch themes from /api/v1/themes in frontend/src/components/enhanced-theme-provider.tsx
- [ ] T020 [US2] Add fallback to static /themes/themes.json if API unavailable in frontend/src/components/enhanced-theme-provider.tsx
- [ ] T021 [US2] Handle missing/deleted custom theme gracefully (fallback to default) in frontend/src/components/enhanced-theme-provider.tsx
- [ ] T022 [US2] Test custom theme loading: add test-theme.css to $DATA/themes/, verify in UI

**Checkpoint**: User Story 2 complete - custom themes from $DATA/themes/ appear in theme selector

---

## Phase 5: User Story 3 - Consistent Component Styling (Priority: P3)

**Goal**: Ensure all UI components (buttons, inputs, cards, dialogs) have consistent styling across all pages

**Independent Test**: Visual audit of component styles across pages - all should match

### Implementation for User Story 3

- [ ] T023 [US3] Audit button usage across all pages, standardize to consistent variants in frontend/src/app/*/page.tsx
- [ ] T024 [P] [US3] Audit input field usage, ensure all use shadcn Input component in frontend/src/app/*/page.tsx
- [ ] T025 [P] [US3] Audit Card component usage, standardize padding and styling in frontend/src/app/*/page.tsx
- [ ] T026 [P] [US3] Audit Dialog/Sheet components, ensure consistent backdrop and padding in frontend/src/components/*.tsx
- [ ] T027 [US3] Remove any custom inline styles that override shadcn defaults in frontend/src/components/*.tsx
- [ ] T028 [US3] Verify all components use theme CSS variables (--background, --foreground, etc.) in frontend/src/components/ui/*.tsx

**Checkpoint**: User Story 3 complete - all components should have consistent styling

---

## Phase 6: User Story 4 - Intuitive Theme Management UI (Priority: P4)

**Goal**: Enhance theme selector with visual color previews and custom theme distinction

**Independent Test**: Open theme selector and verify color swatches are visible for each theme

### Implementation for User Story 4

- [ ] T029 [US4] Enhance ColorPreview component to show 4 color swatches in frontend/src/components/enhanced-theme-selector.tsx
- [ ] T030 [US4] Add "Custom" badge/label for custom themes in theme selector in frontend/src/components/enhanced-theme-selector.tsx
- [ ] T031 [US4] Group themes by source (Built-in / Custom sections) in frontend/src/components/enhanced-theme-selector.tsx
- [ ] T032 [US4] Ensure theme previews load within 500ms (verify API response time) in frontend/src/components/enhanced-theme-provider.tsx

**Checkpoint**: User Story 4 complete - theme selector shows color previews and distinguishes custom themes

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Final improvements affecting multiple user stories

- [ ] T033 [P] Add structured logging for theme operations in internal/service/theme_service.go
- [ ] T034 [P] Add error handling for theme file read failures in internal/http/handlers/theme.go
- [ ] T035 Run frontend build to verify no TypeScript errors (pnpm --prefix frontend run build)
- [ ] T036 Run backend lint to verify no Go errors (task lint:go)
- [ ] T037 Test edge cases: invalid theme file, duplicate names, missing directory
- [ ] T038 Run quickstart.md validation scenarios

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Story 1 (Phase 3)**: Depends on Foundational (backend ready)
- **User Story 2 (Phase 4)**: Depends on Foundational (backend ready)
- **User Story 3 (Phase 5)**: No backend dependencies - can start after Setup
- **User Story 4 (Phase 6)**: Depends on User Story 2 (needs custom themes to distinguish)
- **Polish (Phase 7)**: Depends on all user stories being complete

### User Story Dependencies

```
Foundational (Phase 2)
    │
    ├──▶ US1: Dark Mode Navigation (P1) - MVP
    │
    ├──▶ US2: Custom Theme Support (P2)
    │         │
    │         └──▶ US4: Theme Selector UI (P4)
    │
    └──▶ US3: Component Consistency (P3) - can run in parallel
```

### Parallel Opportunities

**Within Phase 2 (Foundational)**:
- T005 (validation patterns) and T006 (color extraction) can run in parallel

**Within Phase 5 (US3)**:
- T024 (inputs), T025 (cards), T026 (dialogs) can all run in parallel

**Across Phases**:
- US1 and US3 can be worked on in parallel after Foundational completes
- US2 and US3 can be worked on in parallel

---

## Parallel Example: User Story 3 Component Audit

```bash
# These tasks touch different files and can run in parallel:
Task: "T024 [P] [US3] Audit input field usage..."
Task: "T025 [P] [US3] Audit Card component usage..."
Task: "T026 [P] [US3] Audit Dialog/Sheet components..."
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational
3. Complete Phase 3: User Story 1 (Dark Mode Navigation)
4. **STOP and VALIDATE**: Test navigation - no white flashes
5. Deploy/demo if ready - this is the primary pain point fix

### Incremental Delivery

1. Complete Setup + Foundational → Backend theme API ready
2. Add User Story 1 → Test dark mode navigation → Deploy (MVP - fixes flashbang!)
3. Add User Story 2 → Test custom themes → Deploy (custom theming enabled)
4. Add User Story 3 → Visual audit → Deploy (consistent UI)
5. Add User Story 4 → Test selector UI → Deploy (enhanced UX)

### Recommended Order for Single Developer

1. Phase 1 + Phase 2 (backend foundation)
2. Phase 3 (US1 - MVP, most impactful fix)
3. Phase 4 (US2 - enables custom themes)
4. Phase 6 (US4 - depends on US2)
5. Phase 5 (US3 - can be done anytime, independent audit)
6. Phase 7 (polish)

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- US1 is the MVP - it fixes the most disruptive issue (flashbang)
- US3 (component audit) is independent and can be parallelized with other stories
- US4 depends on US2 for custom theme distinction feature
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
