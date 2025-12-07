# Tasks: Multi-Format Streaming Support

**Input**: Design documents from `/specs/008-multi-format-streaming/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

Based on plan.md structure:
- **Backend**: `internal/` (Go packages)
- **Frontend**: `frontend/src/` (Next.js)
- **Tests**: `internal/*/` (Go tests alongside code)

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and core types

- [ ] T001 Add format/codec constants and content types in internal/relay/constants.go
- [ ] T002 [P] Add VP9, AV1, Opus codec constants to internal/models/relay_profile.go
- [ ] T003 [P] Add DASH output format constant to internal/models/relay_profile.go
- [ ] T004 Add database migration for segment_duration and playlist_size fields in internal/database/migrations/

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**CRITICAL**: No user story work can begin until this phase is complete

- [ ] T005 Create Segment type in internal/relay/segment.go
- [ ] T006 Create SegmentBuffer with ring buffer implementation in internal/relay/segment_buffer.go
- [ ] T007 [P] Create SegmentClient tracking in internal/relay/segment_buffer.go
- [ ] T008 Create OutputHandler interface in internal/relay/output_handler.go
- [ ] T009 Create FormatRouter with format detection logic in internal/relay/format_router.go
- [ ] T010 Add ValidateCodecFormat method to RelayProfile in internal/models/relay_profile.go
- [ ] T011 Add ?format= query parameter parsing to internal/http/handlers/relay_stream.go
- [ ] T012 [P] Unit tests for SegmentBuffer in internal/relay/segment_buffer_test.go

**Checkpoint**: Foundation ready - user story implementation can now begin

---

## Phase 3: User Story 1 - HLS Output for Device Compatibility (Priority: P1) MVP

**Goal**: Serve streams via HLS with .m3u8 playlist and .ts segment delivery

**Independent Test**: Configure relay with HLS output, request ?format=hls, verify valid playlist and segment delivery

### Implementation for User Story 1

- [ ] T013 [US1] Create HLSHandler implementing OutputHandler in internal/relay/hls_handler.go
- [ ] T014 [US1] Implement GeneratePlaylist method in HLSHandler (EXT-X-VERSION 3+, TARGETDURATION, MEDIA-SEQUENCE)
- [ ] T015 [US1] Implement ServePlaylist in HLSHandler with Content-Type: application/vnd.apple.mpegurl
- [ ] T016 [US1] Implement ServeSegment in HLSHandler with Content-Type: video/MP2T
- [ ] T017 [US1] Add segment extraction from MPEG-TS stream in internal/relay/segment_extractor.go
- [ ] T018 [US1] Integrate SegmentBuffer into RelaySession in internal/relay/session.go
- [ ] T019 [US1] Add HLS format routing in FormatRouter for format=hls requests
- [ ] T020 [US1] Handle ?seg= query parameter for segment requests in internal/http/handlers/relay_stream.go
- [ ] T021 [P] [US1] Unit tests for HLSHandler playlist generation in internal/relay/hls_handler_test.go
- [ ] T022 [P] [US1] Integration test for HLS streaming in internal/relay/hls_integration_test.go

**Checkpoint**: HLS output fully functional - test with Safari/iOS/VLC

---

## Phase 4: User Story 2 - DASH Output for Cross-Platform Streaming (Priority: P2)

**Goal**: Serve streams via DASH with .mpd manifest and .m4s segment delivery

**Independent Test**: Configure relay with DASH output, request ?format=dash, verify valid MPD manifest and segment delivery

### Implementation for User Story 2

- [ ] T023 [US2] Create DASHHandler implementing OutputHandler in internal/relay/dash_handler.go
- [ ] T024 [US2] Implement GenerateManifest method in DASHHandler (DASH-IF compliant, SegmentTemplate)
- [ ] T025 [US2] Implement ServePlaylist in DASHHandler with Content-Type: application/dash+xml
- [ ] T026 [US2] Implement ServeInitSegment for video/audio init segments with Content-Type: video/mp4
- [ ] T027 [US2] Implement ServeSegment in DASHHandler with Content-Type: video/iso.segment
- [ ] T028 [US2] Add fMP4 segment creation for DASH in internal/relay/fmp4_muxer.go
- [ ] T029 [US2] Handle ?init= query parameter for initialization segments in internal/http/handlers/relay_stream.go
- [ ] T030 [US2] Add DASH format routing in FormatRouter for format=dash requests
- [ ] T031 [P] [US2] Unit tests for DASHHandler manifest generation in internal/relay/dash_handler_test.go
- [ ] T032 [P] [US2] Integration test for DASH streaming in internal/relay/dash_integration_test.go

**Checkpoint**: DASH output fully functional - test with dash.js/Shaka Player

---

## Phase 5: User Story 3 - Passthrough Proxy Mode Format Preservation (Priority: P1)

**Goal**: Proxy HLS/DASH sources with caching and URL rewriting, no transcoding

**Independent Test**: Configure passthrough for HLS source, connect multiple clients, verify single upstream connection

### Implementation for User Story 3

- [ ] T033 [US3] Create HLS passthrough handler with playlist URL rewriting in internal/relay/hls_passthrough.go
- [ ] T034 [US3] Create DASH passthrough handler with manifest URL rewriting in internal/relay/dash_passthrough.go
- [ ] T035 [US3] Implement segment caching for passthrough mode in internal/relay/segment_cache.go
- [ ] T036 [US3] Add client multiplexing for cached segments in internal/relay/segment_cache.go
- [ ] T037 [US3] Integrate passthrough detection in session.go based on source stream type
- [ ] T038 [US3] Add upstream connection pooling for passthrough in internal/relay/connection_pool.go
- [ ] T039 [P] [US3] Unit tests for playlist URL rewriting in internal/relay/hls_passthrough_test.go
- [ ] T040 [P] [US3] Integration test for passthrough proxy in internal/relay/passthrough_integration_test.go

**Checkpoint**: Passthrough proxy functional - verify reduced upstream connections

---

## Phase 6: User Story 4 - Format Auto-Selection Based on Client (Priority: P3)

**Goal**: Automatically detect and serve optimal format based on User-Agent/Accept headers

**Independent Test**: Request ?format=auto from Safari (expect HLS), Chrome (expect MPEG-TS)

### Implementation for User Story 4

- [ ] T041 [US4] Implement DetectOptimalFormat in FormatRouter with User-Agent detection
- [ ] T042 [US4] Add Accept header parsing for DASH preference detection in FormatRouter
- [ ] T043 [US4] Add default format configuration to proxy settings in internal/models/proxy.go
- [ ] T044 [US4] Handle format=auto in stream handler routing in internal/http/handlers/relay_stream.go
- [ ] T045 [P] [US4] Unit tests for format detection logic in internal/relay/format_router_test.go

**Checkpoint**: Auto-detection functional - test with different clients

---

## Phase 7: Container-Aware Codec Selection (Cross-Cutting)

**Goal**: UI displays only valid codecs for selected output format, validation prevents invalid combinations

### Backend Implementation

- [ ] T046 Implement GET /relay/codecs endpoint with format filtering in internal/http/handlers/relay_profile.go
- [ ] T047 Add codec/format validation in RelayService.CreateProfile in internal/service/relay_service.go
- [ ] T048 Add codec/format validation in RelayService.UpdateProfile in internal/service/relay_service.go
- [ ] T049 Return detailed error messages for codec/format mismatches

### Frontend Implementation

- [ ] T050 [P] Create useCodecsByFormat hook in frontend/src/hooks/useCodecsByFormat.ts
- [ ] T051 [P] Update video codec dropdown to filter by output_format in frontend/src/components/relay-profile/
- [ ] T052 [P] Update audio codec dropdown to filter by output_format in frontend/src/components/relay-profile/
- [ ] T053 Add warning message when output format change makes codec invalid
- [ ] T054 [P] Add VP9, AV1, Opus codec labels to frontend codec lists

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [ ] T055 Add segment buffer stats to session statistics endpoint in internal/http/handlers/relay_stream.go
- [ ] T056 [P] Add structured logging for segment operations in internal/relay/segment_buffer.go
- [ ] T057 [P] Add structured logging for format routing decisions in internal/relay/format_router.go
- [ ] T058 Add memory usage metrics for segment buffers
- [ ] T059 Update API documentation with format query parameter
- [ ] T060 Run quickstart.md validation scenarios
- [ ] T061 [P] Add error handling for segment expiration (404 responses)
- [ ] T062 Add EXT-X-DISCONTINUITY handling for stream interruptions in HLSHandler

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phase 3-6)**: All depend on Foundational phase completion
  - US1 (HLS) and US3 (Passthrough) are P1 - implement first
  - US2 (DASH) is P2 - implement after core HLS works
  - US4 (Auto-Selection) is P3 - implement after formats work
- **Codec Selection (Phase 7)**: Can proceed in parallel with user stories
- **Polish (Phase 8)**: Depends on user stories being complete

### User Story Dependencies

- **User Story 1 (HLS)**: Can start after Foundational - No dependencies on other stories
- **User Story 2 (DASH)**: Can start after Foundational - Shares SegmentBuffer with US1 but independent
- **User Story 3 (Passthrough)**: Can start after Foundational - Completely independent path
- **User Story 4 (Auto-Selection)**: Depends on US1/US2 format handlers existing

### Within Each User Story

- Core handler before specialized features
- Segment handling before playlist generation
- Backend before frontend integration
- Unit tests alongside implementation

### Parallel Opportunities

**Phase 2 (Foundational)**:
```
Parallel Group 1: T007 (SegmentClient) + T012 (SegmentBuffer tests)
```

**Phase 3 (US1 - HLS)**:
```
Parallel Group 2: T021 (HLS unit tests) + T022 (HLS integration tests)
```

**Phase 4 (US2 - DASH)**:
```
Parallel Group 3: T031 (DASH unit tests) + T032 (DASH integration tests)
```

**Phase 7 (Codec Selection)**:
```
Parallel Group 4: T050 + T051 + T052 + T054 (all frontend tasks)
```

---

## Parallel Example: Phase 2 Foundation

```bash
# Sequential core tasks:
Task: "Create Segment type in internal/relay/segment.go"
Task: "Create SegmentBuffer in internal/relay/segment_buffer.go"

# Then parallel:
Task: "Create SegmentClient tracking" [P]
Task: "Unit tests for SegmentBuffer" [P]
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL - blocks all stories)
3. Complete Phase 3: User Story 1 (HLS)
4. **STOP and VALIDATE**: Test HLS with Safari, VLC, iOS
5. Deploy/demo if ready

### Suggested Order

1. Setup + Foundational → Core infrastructure ready
2. US1 (HLS) → Most widely needed format, iOS/Safari support
3. US3 (Passthrough) → Quick win, reuses existing HLS sources
4. US2 (DASH) → Cross-platform coverage
5. Phase 7 (Codec Selection) → UI polish
6. US4 (Auto-Selection) → User convenience
7. Phase 8 (Polish) → Final touches

### MVP Scope

For minimal viable product, implement:
- Phase 1: Setup
- Phase 2: Foundational
- Phase 3: User Story 1 (HLS)

This delivers HLS streaming to maximize device compatibility (iOS, Safari, smart TVs).

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- WebRTC (User Story 5) is deferred to future phase per spec
- All tests use fictional channel/proxy IDs per constitution XIII
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
