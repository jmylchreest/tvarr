# Tasks: FFmpeg Relay and Stream Transcoding Proxy

**Input**: Design documents from `/specs/005-ffmpeg-relay-transcoding/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/relay-api.yaml

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and verification of existing infrastructure

- [X] T001 Verify existing StreamProxy.ProxyMode enum in `internal/models/stream_proxy.go`
- [X] T002 Verify existing RelayProfile model structure in `internal/models/relay_profile.go`
- [X] T003 [P] Create relay package directory structure (`internal/relay/`)
- [X] T004 [P] Create relay handler skeleton in `internal/http/handlers/relay_stream.go`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**CRITICAL**: No user story work can begin until this phase is complete

- [X] T005 Add CORS helper function in `internal/http/handlers/cors.go`
- [X] T006 [P] Add fallback fields to RelayProfile model (FallbackEnabled, FallbackErrorThreshold, FallbackRecoveryInterval) in `internal/models/relay_profile.go`
- [X] T007 [P] Create stream classification types and constants in `internal/relay/types.go`
- [X] T008 Create cyclic buffer implementation in `internal/relay/cyclic_buffer.go`
- [X] T009 Create cyclic buffer tests in `internal/relay/cyclic_buffer_test.go`
- [X] T010 Create buffer client tracking in `internal/relay/client.go`
- [X] T011 Create buffer client tests in `internal/relay/client_test.go`
- [X] T012 Create relay session struct in `internal/relay/session.go`
- [X] T013 Create relay manager scaffold in `internal/relay/manager.go`

**Checkpoint**: Foundation ready - user story implementation can now begin

---

## Phase 3: User Story 1 - Redirect Mode Stream Access (Priority: P1)

**Goal**: Enable HTTP 302 redirects for channels configured with redirect mode

**Independent Test**: Configure a channel with redirect mode, request stream URL, verify HTTP 302 response

### Tests for User Story 1

- [X] T014 [P] [US1] Unit test for redirect mode handler in `internal/http/handlers/relay_stream_test.go`
- [ ] T015 [P] [US1] Integration test for redirect flow in `internal/http/handlers/relay_stream_integration_test.go`

### Implementation for User Story 1

- [X] T016 [US1] Implement redirect mode detection in handler at `internal/http/handlers/relay_stream.go`
- [X] T017 [US1] Return HTTP 302 with Location header pointing to source URL
- [X] T018 [US1] Handle redirect mode without session creation (zero overhead)

**Checkpoint**: Redirect mode functional and testable independently

---

## Phase 4: User Story 2 - Proxy Mode Stream Access (Priority: P1)

**Goal**: Fetch upstream content and forward with CORS headers, optional HLS collapse

**Independent Test**: Configure proxy mode, request stream from browser, verify CORS headers and content

### Tests for User Story 2

- [X] T019 [P] [US2] Unit test for proxy mode handler in `internal/http/handlers/relay_stream_test.go`
- [X] T020 [P] [US2] Unit test for HLS collapse logic in `internal/relay/hls_collapse_test.go`
- [ ] T021 [P] [US2] Integration test for CORS preflight in `internal/http/handlers/relay_stream_integration_test.go`

### Implementation for User Story 2

- [X] T022 [P] [US2] Create HLS collapse module in `internal/relay/hls_collapse.go`
- [X] T023 [P] [US2] Implement HLS master playlist parsing in `internal/relay/hls_collapse.go`
- [X] T024 [US2] Implement proxy mode handler to fetch upstream content at `internal/http/handlers/relay_stream.go`
- [X] T025 [US2] Add CORS headers (Access-Control-Allow-Origin: *) for proxy responses
- [X] T026 [US2] Handle OPTIONS preflight requests for browser compatibility (FR-016)
- [X] T027 [US2] Integrate HLS collapse (select highest quality variant) when enabled (FR-014)
- [X] T028 [US2] Add X-Stream-* response headers for debugging (origin kind, decision, mode)

**Checkpoint**: Proxy mode with CORS and HLS collapse functional

---

## Phase 5: User Story 3 - Transcoded Stream Access with Shared FFmpeg (Priority: P1)

**Goal**: On-demand FFmpeg transcoding with cyclic buffer for multi-client sharing

**Independent Test**: Request transcoded stream, verify playback, connect second client, confirm single FFmpeg process

### Tests for User Story 3

- [X] T029 [P] [US3] Unit test for FFmpeg command building in `internal/relay/ffmpeg_test.go`
- [X] T030 [P] [US3] Unit test for session lifecycle in `internal/relay/manager_test.go`
- [ ] T031 [P] [US3] Integration test for multi-client streaming in `internal/relay/manager_integration_test.go`

### Implementation for User Story 3

- [X] T032 [P] [US3] Create FFmpeg command builder in `internal/relay/ffmpeg.go`
- [X] T033 [US3] Implement FFmpeg process spawning in relay manager at `internal/relay/manager.go`
- [X] T034 [US3] Implement session join logic (multiple clients share single FFmpeg) in `internal/relay/manager.go`
- [X] T035 [US3] Integrate cyclic buffer for FFmpeg output distribution at `internal/relay/session.go`
- [X] T036 [US3] Track each client's buffer position independently (FR-006)
- [X] T037 [US3] Implement idle timeout termination (configurable, default 60s) (FR-007)
- [X] T038 [US3] Implement background cleanup task (every 10s) (FR-023)
- [X] T039 [US3] Wire transcode mode in stream handler at `internal/http/handlers/relay_stream.go`

**Checkpoint**: Transcode mode with shared FFmpeg and cyclic buffer functional

---

## Phase 6: User Story 4 - Hardware Accelerated Transcoding (Priority: P2)

**Goal**: Auto-detect and use hardware encoders (VAAPI, NVENC, QSV, AMF, VideoToolbox)

**Independent Test**: Configure relay profile with HW acceleration, verify FFmpeg uses hardware encoder in logs

### Tests for User Story 4

- [X] T040 [P] [US4] Unit test for HW detection in `internal/ffmpeg/ffmpeg_test.go` (TestHWAccelInfo, TestGetRecommendedHWAccel)
- [X] T041 [P] [US4] Unit test for fallback to software encoding in `internal/ffmpeg/ffmpeg_test.go`

### Implementation for User Story 4

- [X] T042 [P] [US4] Create hardware acceleration detector in `internal/ffmpeg/hwaccel.go`
- [X] T043 [US4] Probe for VAAPI, NVENC, QSV, AMF, VideoToolbox at startup (FR-008)
- [X] T044 [US4] Cache detected accelerators in memory (via BinaryDetector caching)
- [X] T045 [US4] Expose available accelerators via API for profile configuration (`/api/v1/relay/ffmpeg`)
- [X] T046 [US4] Implement fallback to software encoding when HW unavailable (FR-010)
- [X] T047 [US4] Log warning when fallback occurs (in GetRecommendedHWAccel)
- [X] T048 [US4] Integrate HW encoder selection in FFmpeg command builder at `internal/ffmpeg/ffmpeg.go`

**Checkpoint**: Hardware acceleration detection and fallback functional

---

## Phase 7: User Story 5 - Relay Profile Configuration (Priority: P2)

**Goal**: Create relay profiles with codec, bitrate, and HW acceleration settings

**Independent Test**: Create profile via API, assign to channel, verify output format matches profile

### Tests for User Story 5

- [ ] T049 [P] [US5] Unit test for profile CRUD in `internal/http/handlers/relay_profile_test.go`
- [ ] T050 [P] [US5] Integration test for profile assignment in `internal/http/handlers/relay_profile_integration_test.go`

### Implementation for User Story 5

- [X] T051 [US5] Extend relay profile handler for video codec options (H264, H265, AV1, MPEG2, copy) at `internal/http/handlers/relay_profile.go` (FR-011)
- [X] T052 [US5] Add audio codec options (AAC, MP3, AC3, EAC3, DTS, copy) (FR-012)
- [X] T053 [US5] Add configurable video and audio bitrates (FR-013)
- [X] T054 [US5] Add custom FFmpeg argument override support (FR-022) - InputOptions/OutputOptions in RelayProfile model
- [X] T055 [US5] Seed system default profiles (Passthrough, H.264 720p, H.264 1080p) on first startup - IsSystem flag in model
- [X] T056 [US5] Wire profile settings to FFmpeg command builder - in session.go runFFmpegPipeline

**Checkpoint**: Relay profile configuration fully functional

---

## Phase 8: User Story 6 - FFmpeg Process Health Monitoring (Priority: P2)

**Goal**: Display process health metrics (CPU, memory, uptime, error state) in dashboard

**Independent Test**: Start transcode session, view dashboard, verify metrics update in real-time

### Tests for User Story 6

- [ ] T057 [P] [US6] Unit test for health stats collection in `internal/relay/manager_test.go`
- [ ] T058 [P] [US6] Integration test for health API endpoint in `internal/http/handlers/relay_session_test.go`

### Implementation for User Story 6

- [X] T059 [P] [US6] Create relay session handler in `internal/http/handlers/relay_profile.go` (combined with profile handler)
- [X] T060 [US6] Implement relay health API at `/api/v1/relay/health` (FR-019) - GetHealth()
- [X] T061 [US6] Collect per-session metrics (CPU, memory, uptime) - via SessionStats in manager.go
- [X] T062 [US6] Track error state and in-fallback indicator - session.err and closed fields
- [X] T063 [US6] Implement relay stats API at `/api/v1/relay/stats` - GetStats()
- [X] T064 [US6] Add sessions_by_mode breakdown in stats response - via classification field

**Checkpoint**: Health monitoring API functional

---

## Phase 9: User Story 7 - Connected Client Tracking (Priority: P2)

**Goal**: Track and display connected clients per relay (IP, user agent, duration, bytes)

**Independent Test**: Connect multiple clients, view dashboard, verify accurate connection details

### Tests for User Story 7

- [X] T065 [P] [US7] Unit test for client tracking in `internal/relay/client_test.go` - comprehensive tests
- [ ] T066 [P] [US7] Integration test for client list API in `internal/http/handlers/relay_session_test.go`

### Implementation for User Story 7

- [X] T067 [US7] Implement client tracking API at `/api/v1/relay/sessions/{id}/clients` (FR-020) - via Stats() in CyclicBuffer
- [X] T068 [US7] Track client IP address, user agent, connection duration - in BufferClient struct
- [X] T069 [US7] Track bytes served per client - AddBytesRead/GetBytesRead
- [X] T070 [US7] Implement stale client removal after inactivity timeout (default 30s) (FR-021) - cleanupStaleClients
- [X] T071 [US7] Update client tracking on disconnect - RemoveClient

**Checkpoint**: Client tracking fully functional

---

## Phase 10: User Story 8 - Error Fallback with Visual Indicator (Priority: P3)

**Goal**: Serve fallback TS stream during upstream errors with visual indicator

**Independent Test**: Simulate upstream failure, verify clients receive fallback stream

### Tests for User Story 8

- [X] T072 [P] [US8] Unit test for fallback stream generation in `internal/relay/fallback_test.go`
- [ ] T073 [P] [US8] Integration test for error recovery in `internal/relay/manager_integration_test.go`

### Implementation for User Story 8

- [X] T074 [P] [US8] Create fallback stream generator in `internal/relay/fallback.go`
- [X] T075 [US8] Pre-generate TS segment with "Stream Unavailable" slate at startup (FR-018)
- [X] T076 [US8] Monitor FFmpeg stderr for error patterns (FR-017)
- [X] T077 [US8] Implement circuit breaker with configurable threshold (default 3 errors)
- [X] T078 [US8] Switch to fallback stream when threshold exceeded
- [X] T079 [US8] Implement auto-recovery check (default 30s interval)
- [X] T080 [US8] Resume normal transcoding when upstream recovers

**Checkpoint**: Error fallback with auto-recovery functional

---

## Phase 11: Frontend Dashboard

**Purpose**: React components for relay health and client monitoring

- [X] T081 [P] Relay health display in `frontend/src/components/dashboard.tsx` and `frontend/src/components/relay-profiles.tsx` (stats bar)
- [X] T082 [P] Connected clients display in `frontend/src/components/dashboard.tsx` (lines 904-957)
- [X] T083 [P] Create RelayProfileForm component in `frontend/src/components/relay-profile-form.tsx`
- [X] T084 Implement polling for relay stats (10s interval in relay-profiles.tsx)
- [X] T085 Add relay dashboard route and navigation in `frontend/src/app/admin/relays/page.tsx`
- [X] T086 Display active relays with health indicators in stats bar
- [X] T087 Drill-down to connected clients per session in dashboard.tsx with tooltips

---

## Phase 12: Polish & Cross-Cutting Concerns

**Purpose**: Improvements affecting multiple user stories

- [X] T088 [P] Update quickstart.md with usage examples - comprehensive documentation exists (429 lines covering all modes, API examples, browser playback, monitoring, troubleshooting)
- [X] T089 [P] Verify OpenAPI spec matches implementation in `specs/005-ffmpeg-relay-transcoding/contracts/relay-api.yaml` - detailed spec exists (549 lines)
- [X] T090 Code cleanup and ensure consistent error handling - build passes, relay tests pass
- [ ] T091 Performance validation: verify < 3s stream playback (SC-003) - requires runtime testing with actual streams
- [ ] T092 Performance validation: verify 10+ concurrent transcode sessions (SC-004) - requires load testing
- [ ] T093 Validate idle cleanup within 90s (SC-009) - requires runtime testing
- [X] T094 Security review: ensure upstream credentials hidden in proxy mode - verified: clients only receive proxied content, not upstream URLs; admin APIs expose URLs as expected for admin interfaces

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **US1-US3 (Phase 3-5)**: P1 priority, depend on Foundational, can proceed in sequence
- **US4-US7 (Phase 6-9)**: P2 priority, depend on US3 (transcode mode)
- **US8 (Phase 10)**: P3 priority, depends on US3 (transcode mode)
- **Frontend (Phase 11)**: Depends on health/stats APIs from US6-US7
- **Polish (Phase 12)**: Depends on all user stories being complete

### Within Each User Story

- Tests SHOULD be written and FAIL before implementation
- Models/types before services
- Services before handlers
- Core implementation before integration
- Story complete before moving to next priority

### Parallel Opportunities

- All Setup tasks marked [P] can run in parallel
- All Foundational tasks marked [P] can run in parallel (within Phase 2)
- Tests for each user story marked [P] can run in parallel
- Frontend components marked [P] can run in parallel

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- Existing code: StreamProxy.ProxyMode already exists, RelayProfile model exists
