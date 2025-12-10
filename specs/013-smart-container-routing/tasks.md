# Tasks: Smart Container Routing

**Input**: Design documents from `/specs/013-smart-container-routing/`
**Prerequisites**: plan.md (required), spec.md (required for user stories), research.md, data-model.md

**Tests**: Not explicitly requested in spec - implementation tasks only.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- **Backend**: `internal/` for Go code
- **Frontend**: `frontend/src/` for TypeScript/React
- **Models**: `internal/models/`
- **Relay**: `internal/relay/`
- **Migrations**: `internal/database/migrations/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and schema changes

- [ ] T001 Create feature branch `013-smart-container-routing` from current development branch
- [ ] T002 Add DetectionMode type constant and field to RelayProfile model in internal/models/relay_profile.go
- [ ] T003 Create database migration for detection_mode column in internal/database/migrations/xxx_add_detection_mode.go
- [ ] T004 [P] Add HeaderXTvarrPlayer constant to internal/relay/constants.go
- [ ] T005 [P] Add FormatValueHLSFMP4 and FormatValueHLSTS constants to internal/relay/constants.go

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core types and interfaces that MUST be complete before ANY user story can be implemented

**CRITICAL**: No user story work can begin until this phase is complete

- [ ] T006 Create RoutingDecision enum type and RoutingResult struct in internal/relay/routing_decision.go
- [ ] T007 [P] Create ClientCapabilities struct in internal/relay/client_detector.go
- [ ] T008 [P] Create ClientDetector interface in internal/relay/client_detector.go
- [ ] T009 Extend OutputRequest struct with XTvarrPlayer and FormatOverride fields in internal/relay/format_router.go
- [ ] T010 Create RoutingDecider interface in internal/relay/routing_decision.go

**Checkpoint**: Foundation ready - user story implementation can now begin in parallel

---

## Phase 3: User Story 5 - Profile Detection Mode Behavior (Priority: P1)

**Goal**: When `detection_mode = "auto"`, use client detection. When `detection_mode != "auto"`, use profile as-is without client detection.

**Independent Test**: Configure profile with explicit detection_mode (e.g., "hls"), request stream with X-Tvarr-Player header, verify header is ignored and profile settings are used.

### Implementation for User Story 5

- [ ] T011 [US5] Add DetectionMode field to RelayProfile struct in internal/models/relay_profile.go
- [ ] T012 [US5] Add IsAutoDetection() method to RelayProfile in internal/models/relay_profile.go
- [ ] T013 [US5] Create DefaultRoutingDecider implementation in internal/relay/routing_decision.go
- [ ] T014 [US5] Implement Decide() method checking detection_mode first in internal/relay/routing_decision.go
- [ ] T015 [US5] Add routing decision logging per FR-009 in internal/relay/routing_decision.go
- [ ] T016 [US5] Update API profile handlers to expose detection_mode field in internal/handlers/relay_profile_handler.go

**Checkpoint**: Profile detection_mode behavior complete - profiles with explicit modes bypass client detection

---

## Phase 4: User Story 1 - Efficient HLS-to-HLS Passthrough (Priority: P1) MVP

**Goal**: HLS source to HLS client with passthrough profile uses lightweight repackaging without FFmpeg

**Independent Test**: Stream HLS source to HLS player, verify no FFmpeg process spawned while playback works correctly

### Implementation for User Story 1

- [ ] T017 [US1] Create HLSMuxer wrapper for gohlslib Muxer in internal/relay/hls_muxer.go
- [ ] T018 [US1] Implement HLSMuxer.Start() method accepting gohlslib track callbacks in internal/relay/hls_muxer.go
- [ ] T019 [US1] Implement HLSMuxer.ServePlaylist() for m3u8 generation in internal/relay/hls_muxer.go
- [ ] T020 [US1] Implement HLSMuxer.ServeSegment() for segment delivery in internal/relay/hls_muxer.go
- [ ] T021 [US1] Implement segment caching/buffer in HLSMuxer for multi-client sharing in internal/relay/hls_muxer.go
- [ ] T021a [US1] Implement subscriber reference counting for shared upstream connections per FR-008 in internal/relay/manager.go
- [ ] T022 [US1] Add HLSMuxer as OutputHandler in FormatRouter registration in internal/relay/format_router.go
- [ ] T023 [US1] Integrate RoutingDecider in session creation to choose Passthrough vs Transcode in internal/relay/session.go
- [ ] T024 [US1] Add passthrough metrics tracking (CPU usage comparison) in internal/relay/session.go

**Checkpoint**: HLS-to-HLS passthrough works without FFmpeg for compatible streams

---

## Phase 5: User Story 2 - Player Identity Headers (Priority: P2)

**Goal**: Frontend players send X-Tvarr-Player header for better client detection

**Independent Test**: Observe network requests from frontend player, verify X-Tvarr-Player header is present with correct player identification

### Implementation for User Story 2

- [ ] T025 [P] [US2] Create player-headers.ts constants file in frontend/src/lib/player-headers.ts
- [ ] T026 [P] [US2] Define PLAYER_HEADER_NAME and buildPlayerHeader() utility in frontend/src/lib/player-headers.ts
- [ ] T027 [US2] Add X-Tvarr-Player header to MpegTsAdapter mediaDataSource config in frontend/src/player/MpegTsAdapter.ts
- [ ] T028 [US2] Create HlsAdapter.ts with xhrSetup header injection in frontend/src/player/HlsAdapter.ts
- [ ] T029 [US2] Update video-player-modal.tsx to pass headers configuration to player adapters in frontend/src/components/video-player-modal.tsx

**Checkpoint**: All frontend player requests include X-Tvarr-Player header

---

## Phase 6: User Story 3 - Format Negotiation Decision Matrix (Priority: P2)

**Goal**: System evaluates source format x client preference x codec requirements to choose optimal path

**Independent Test**: Make requests with different format preferences and verify correct delivery path is selected

### Implementation for User Story 3

- [ ] T030 [US3] Implement DefaultClientDetector with header parsing logic in internal/relay/client_detector.go
- [ ] T031 [US3] Add X-Tvarr-Player header parsing for player name/version extraction in internal/relay/client_detector.go
- [ ] T032 [US3] Add Accept header parsing for format preference detection in internal/relay/client_detector.go
- [ ] T033 [US3] Add User-Agent parsing for fallback player detection in internal/relay/client_detector.go
- [ ] T034 [US3] Implement detection priority: FormatOverride > XTvarrPlayer > Accept > UserAgent in internal/relay/client_detector.go
- [ ] T035 [US3] Implement codec compatibility check in RoutingDecider in internal/relay/routing_decision.go
- [ ] T036 [US3] Update FormatRouter.ResolveFormat() to use ClientDetector when detection_mode=auto in internal/relay/format_router.go
- [ ] T037 [US3] Add format routing decision to session logging in internal/relay/session.go

**Checkpoint**: Format negotiation selects optimal path based on source + client + codecs

---

## Phase 7: User Story 4 - Container Format Override (Priority: P3)

**Goal**: Users can append ?format=mpegts or ?format=fmp4 to override container format

**Independent Test**: Request stream with ?format=mpegts query parameter, verify output is MPEG-TS container

### Implementation for User Story 4

- [ ] T038 [US4] Parse ?format= query parameter in stream handler and populate OutputRequest.FormatOverride in internal/handlers/stream_handler.go
- [ ] T039 [US4] Update RoutingDecider to check FormatOverride first when detection_mode=auto in internal/relay/routing_decision.go
- [ ] T040 [US4] Add validation for format parameter (mpegts, fmp4, hls, dash) in internal/handlers/stream_handler.go
- [ ] T041 [US4] Return 400 error for invalid format parameter values in internal/handlers/stream_handler.go

**Checkpoint**: URL ?format= parameter overrides container format selection

---

## Phase 7a: User Story 7 - Dynamic Header Expression Field (Priority: P2)

**Goal**: Allow expressions to access arbitrary HTTP request headers via `@req_header:<name>` syntax without hardcoding specific headers

**Why this priority**: Makes client detection expressions flexible and future-proof - users can match on any header (X-Tvarr-Player, User-Agent, Accept, custom headers) without requiring code changes.

**Naming Convention**: Using `@req_header:` prefix to distinguish from potential future `@res_header:` for response header manipulation.

**Independent Test**: Create expression `@req_header:X-Custom-Player ~ "MyPlayer"`, send request with that header, verify expression matches.

**Example Usage**:
```
# Match specific player header
@req_header:X-Tvarr-Player ~ "hls.js"

# Check User-Agent
@req_header:User-Agent ~ "VLC"

# Check Accept header for format preference
@req_header:Accept ~ "application/vnd.apple.mpegurl"

# Combine with other conditions
@req_header:X-Tvarr-Player ~ "mpegts.js" AND source_format = "hls"
```

### Implementation for User Story 7

- [ ] T068 [US7] Create DynamicFieldResolver interface for parameterized field extraction in internal/expression/dynamic_field.go
- [ ] T069 [US7] Implement RequestHeaderFieldResolver that extracts @req_header:<name> from HTTP request context in internal/expression/req_header_field.go
- [ ] T070 [US7] Register RequestHeaderFieldResolver in expression evaluator to handle @req_header: prefix in internal/expression/eval_context.go
- [ ] T071 [US7] Add http.Header to EvalContext for relay request expressions in internal/expression/eval_context.go
- [ ] T072 [US7] Propagate request headers to expression evaluation in stream handler in internal/handlers/stream_handler.go
- [ ] T073 [P] [US7] Add unit tests for @req_header:<name> field extraction in internal/expression/req_header_field_test.go
- [ ] T074 [P] [US7] Add integration test for header-based routing expression in internal/relay/client_detector_test.go

**Checkpoint**: Expressions can use @req_header:<name> to match any HTTP request header dynamically

---

## Phase 7.5: User Story 6 - Relay Flow Visualization Dashboard (Priority: P3)

**Goal**: Visualize active relay sessions as an animated network flow diagram in the dashboard's active relay processes section, showing data path from origin through processor (FFmpeg or gohlslib) to connected clients, with real-time metrics

**Independent Test**: Open dashboard while streams are active, verify flow diagram displays all session components with animated data flow and live metrics

**Library**: React Flow (@xyflow/react) with shadcn-compatible styling

### Backend: Session Tracking for All Relay Types

- [ ] T050 [US6] Create RelaySessionInfo struct with RouteType, ClientCount, BytesIn/Out, Codecs, CPUPercent, MemoryBytes (nullable for non-FFmpeg) in internal/relay/session_info.go
- [ ] T051 [US6] Add gohlslib client tracking to session manager (passthrough/repackage sessions without FFmpeg) in internal/relay/manager.go
- [ ] T052 [US6] Track RouteType (Passthrough/Repackage/Transcode) per session in internal/relay/session.go
- [ ] T053 [US6] Add bytes transferred counters (in/out) to session stats in internal/relay/session.go
- [ ] T054 [US6] Create GET /api/v1/relay/sessions endpoint returning active session flow data in internal/handlers/relay_session_handler.go

### Backend: Session Flow Data Model

- [ ] T055 [US6] Define RelayFlowNode struct (Origin, Processor, Client) with position hints in internal/relay/flow_types.go
- [ ] T056 [US6] Define RelayFlowEdge struct with bandwidth, codec info, animated flag in internal/relay/flow_types.go
- [ ] T057 [US6] Implement BuildFlowGraph() converting sessions to nodes+edges in internal/relay/flow_builder.go

### Frontend: React Flow Setup

- [ ] T058 [P] [US6] Install @xyflow/react dependency: pnpm --prefix frontend add @xyflow/react
- [ ] T059 [P] [US6] Create RelayFlowDiagram.tsx wrapper component in frontend/src/components/relay/relay-flow-diagram.tsx
- [ ] T060 [US6] Create custom OriginNode component (source icon, URL, codec info, bytes/s ingress) in frontend/src/components/relay/nodes/origin-node.tsx
- [ ] T061 [US6] Create custom ProcessorNode component (RouteType badge, codec transform, optional CPU/memory sparklines when available) in frontend/src/components/relay/nodes/processor-node.tsx
- [ ] T062 [US6] Create custom ClientNode component (player type, IP, bytes received) in frontend/src/components/relay/nodes/client-node.tsx
- [ ] T063 [US6] Create AnimatedEdge component with data flow animation in frontend/src/components/relay/edges/animated-edge.tsx

### Frontend: Integration

- [ ] T064 [US6] Add useRelayFlowData hook fetching /api/v1/relay/sessions with polling in frontend/src/hooks/use-relay-flow-data.ts
- [ ] T065 [US6] Add RelayFlowDiagram to dashboard active relay processes section in frontend/src/app/dashboard/page.tsx
- [ ] T066 [US6] Add session detail tooltip/popover on node hover in frontend/src/components/relay/relay-flow-diagram.tsx
- [ ] T067 [US6] Style nodes with shadcn Card component for consistent look in frontend/src/components/relay/nodes/*.tsx

**Checkpoint**: Dashboard shows animated flow diagram of all active relay sessions with real-time metrics (CPU/memory shown only for FFmpeg sessions)

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [ ] T042 [P] Add routing decision tests in internal/relay/routing_decision_test.go
- [ ] T043 [P] Add client detector tests in internal/relay/client_detector_test.go
- [ ] T044 [P] Add HLS muxer tests in internal/relay/hls_muxer_test.go
- [ ] T045 [P] Add MpegTsAdapter header injection tests in frontend/src/player/MpegTsAdapter.test.ts
- [ ] T046 Run existing relay tests to verify no regressions: go test ./internal/relay/...
- [ ] T047 Run frontend build and lint: pnpm --prefix frontend run build && pnpm --prefix frontend run lint
- [ ] T048 Update API documentation for detection_mode field in relay profiles
- [ ] T049 Add observability: ensure routing decision logged per FR-009

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phase 3-7)**: All depend on Foundational phase completion
  - US5 (Profile Detection Mode) should complete first as other stories depend on routing decider
  - US1 (HLS-to-HLS Passthrough) depends on US5
  - US2 (Player Identity Headers) can proceed in parallel - frontend only
  - US3 (Format Negotiation) depends on US2 for header detection
  - US4 (Container Override) depends on US3
- **Polish (Phase 8)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 5 (P1)**: Can start after Foundational (Phase 2) - Core detection_mode logic
- **User Story 1 (P1)**: Depends on US5 - Uses routing decider for passthrough decision
- **User Story 2 (P2)**: Can start after Foundational (Phase 2) - Frontend only, no backend dependencies
- **User Story 3 (P2)**: Depends on US2 for X-Tvarr-Player header, depends on US5 for routing
- **User Story 4 (P3)**: Depends on US3 - Extends format detection
- **User Story 6 (P3)**: Depends on US5 for RouteType tracking - Can proceed in parallel with US1-US4 once session tracking is in place

### Within Each User Story

- Models/types before implementations
- Implementations before integrations
- Core functionality before observability/logging
- Story complete before moving to next priority

### Parallel Opportunities

- All Setup tasks marked [P] can run in parallel (T004, T005)
- All Foundational tasks marked [P] can run in parallel (T007, T008)
- US2 frontend tasks (T025, T026) can run in parallel
- All Polish phase tests (T042-T045) can run in parallel

---

## Parallel Example: Phase 2 Foundational

```bash
# Launch all foundational types together:
Task: "Create ClientCapabilities struct in internal/relay/client_detector.go"
Task: "Create ClientDetector interface in internal/relay/client_detector.go"

# After those complete, create implementations:
Task: "Extend OutputRequest struct with XTvarrPlayer and FormatOverride in internal/relay/format_router.go"
```

---

## Parallel Example: User Story 2 (Frontend)

```bash
# Launch frontend header tasks together:
Task: "Create player-headers.ts constants file in frontend/src/lib/player-headers.ts"
Task: "Define PLAYER_HEADER_NAME and buildPlayerHeader() utility in frontend/src/lib/player-headers.ts"

# After those complete:
Task: "Add X-Tvarr-Player header to MpegTsAdapter in frontend/src/player/MpegTsAdapter.ts"
Task: "Create HlsAdapter.ts with xhrSetup header injection in frontend/src/player/HlsAdapter.ts"
```

---

## Implementation Strategy

### MVP First (User Story 5 + User Story 1)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL - blocks all stories)
3. Complete Phase 3: User Story 5 (detection_mode behavior)
4. Complete Phase 4: User Story 1 (HLS-to-HLS Passthrough)
5. **STOP and VALIDATE**: Test passthrough streams work without FFmpeg
6. Deploy/demo if ready

### Incremental Delivery

1. Complete Setup + Foundational -> Foundation ready
2. Add User Story 5 -> Test detection_mode behavior
3. Add User Story 1 -> Test HLS passthrough -> Deploy/Demo (MVP!)
4. Add User Story 2 -> Test header injection -> Deploy/Demo
5. Add User Story 3 -> Test format negotiation -> Deploy/Demo
6. Add User Story 4 -> Test format override -> Deploy/Demo
7. Each story adds value without breaking previous stories

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together
2. Once Foundational is done:
   - Developer A: User Story 5 -> User Story 1 (backend routing + passthrough)
   - Developer B: User Story 2 (frontend headers)
3. After US5 complete:
   - Developer A: User Story 3 -> User Story 4 (format detection chain)
   - Developer B: Help with tests
4. Stories complete and integrate independently

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- detection_mode logic is foundational - complete US5 before other stories

---

## Summary

| Metric | Value |
|--------|-------|
| Total Tasks | 75 |
| Phase 1 (Setup) | 5 tasks |
| Phase 2 (Foundational) | 5 tasks |
| Phase 3 (US5 - Detection Mode) | 6 tasks |
| Phase 4 (US1 - HLS Passthrough) | 9 tasks |
| Phase 5 (US2 - Player Headers) | 5 tasks |
| Phase 6 (US3 - Format Negotiation) | 8 tasks |
| Phase 7 (US4 - Container Override) | 4 tasks |
| Phase 7a (US7 - Dynamic Header Expression) | 7 tasks |
| Phase 7.5 (US6 - Relay Flow Visualization) | 18 tasks |
| Phase 8 (Polish) | 8 tasks |
| Parallel Opportunities | 14 tasks marked [P] |
| MVP Scope | US5 + US1 (19 tasks through Phase 4) |
