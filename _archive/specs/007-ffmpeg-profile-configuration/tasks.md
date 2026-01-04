# Tasks: FFmpeg Profile Configuration

**Input**: Design documents from `/specs/007-ffmpeg-profile-configuration/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/openapi.yaml, quickstart.md

**Tests**: Not explicitly requested in the feature specification. Test tasks are omitted.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US0, US1, US2)
- Include exact file paths in descriptions

## Path Conventions

- **Backend**: `internal/` at repository root (Go)
- **Frontend**: `frontend/src/` (Next.js/TypeScript)
- **Migrations**: `internal/database/migrations/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and feature branch setup

- [x] T001 Create and checkout feature branch `007-ffmpeg-profile-configuration`
- [x] T002 [P] Create bitstream filter service file in internal/ffmpeg/bitstream_filters.go
- [x] T003 [P] Create flag validator service file in internal/ffmpeg/validator.go
- [x] T004 [P] Create hardware detector service file in internal/services/hardware_detector.go

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**CRITICAL**: No user story work can begin until this phase is complete

- [x] T005 Extend CommandBuilder with custom options methods in internal/ffmpeg/wrapper.go
- [x] T006 Add database migration for new RelayProfile fields in internal/database/migrations/
- [x] T007 Extend RelayProfile model with new fields in internal/models/relay_profile.go (hw_accel_output_format, hw_accel_decoder_codec, hw_accel_extra_options, gpu_index, custom_flags_validated, custom_flags_warnings, success_count, failure_count, last_used_at, last_error_at, last_error_msg)
- [x] T008 Register new migration in internal/database/migrations/registry.go
- [x] T009 Update existing system profiles with sensible default input_options in internal/repository/relay_profile_repo.go or seeder

**Checkpoint**: Foundation ready - user story implementation can now begin

---

## Phase 3: User Story 0 - Fix H.264 Stream Corruption (Priority: P0 CRITICAL)

**Goal**: Fix H.264 stream corruption causing "non-existing PPS" and "Packet corrupt" errors

**Independent Test**: Start a relay with any H.264 HLS source, play with mpv/ffplay, verify no decoding errors

### Implementation for User Story 0

- [x] T010 [US0] Implement codec-to-BSF mapping function in internal/ffmpeg/bitstream_filters.go (h264_mp4toannexb, hevc_mp4toannexb, vp9_superframe mappings)
- [x] T011 [US0] Implement GetBitstreamFilterForCodec function supporting H.264, H.265, VP9, AV1 in internal/ffmpeg/bitstream_filters.go
- [x] T012 [US0] Implement output format detection (MPEG-TS, HLS, FLV, MP4) in internal/ffmpeg/bitstream_filters.go
- [x] T013 [US0] Add hardware encoder codec family mapping (h264_nvenc->h264, hevc_qsv->hevc, etc.) in internal/ffmpeg/bitstream_filters.go
- [x] T014 [US0] Fix FFmpeg input flags: move -fflags +genpts+discardcorrupt to INPUT side in internal/relay/session.go runFFmpegPipeline()
- [x] T015 [US0] Remove -mpegts_copyts and replace with -avoid_negative_ts make_zero in internal/relay/session.go
- [x] T016 [US0] Add -flush_packets 1 and -muxdelay 0 to output flags in internal/relay/session.go
- [x] T017 [US0] Add -pat_period 0.1 for frequent PAT/PMT insertion in internal/relay/session.go
- [x] T018 [US0] Wire GetBitstreamFilter call into runFFmpegPipeline to apply -bsf:v based on codec and output format in internal/relay/session.go
- [x] T019 [US0] Add codec detection using ffprobe for copy mode profiles in internal/relay/session.go
- [x] T020 [US0] Add logging for applied bitstream filters in internal/relay/session.go

**Checkpoint**: Relay streams should now play without "non-existing PPS" or corruption errors

---

## Phase 4: User Story 1 - Add Custom FFmpeg Flags to Profiles (Priority: P1)

**Goal**: Enable administrators to add custom FFmpeg input/output flags to relay profiles

**Independent Test**: Create profile with custom flags, start relay, verify flags appear in FFmpeg command

### Implementation for User Story 1

- [x] T021 [P] [US1] Implement dangerous pattern detection in internal/ffmpeg/validator.go (shell injection, command substitution patterns)
- [x] T022 [P] [US1] Implement blocked flags check in internal/ffmpeg/validator.go (-i, -y, -filter_script, etc.)
- [x] T023 [US1] Implement quote balancing validation in internal/ffmpeg/validator.go
- [x] T024 [US1] Implement ValidateCustomFlags function returning FlagValidationResult in internal/ffmpeg/validator.go
- [x] T025 [US1] Add ApplyInputOptions method to CommandBuilder in internal/ffmpeg/wrapper.go
- [x] T026 [US1] Add ApplyOutputOptions method to CommandBuilder in internal/ffmpeg/wrapper.go
- [x] T027 [US1] Add ApplyFilterComplex method to CommandBuilder in internal/ffmpeg/wrapper.go
- [x] T028 [US1] Wire InputOptions from profile into command builder (after structured settings, before -i) in internal/relay/session.go
- [x] T029 [US1] Wire OutputOptions from profile into command builder (after codec settings, as override) in internal/relay/session.go
- [x] T030 [US1] Wire FilterComplex from profile into command builder in internal/relay/session.go
- [x] T031 [US1] Add validate-flags endpoint handler POST /api/v1/relay-profiles/validate-flags in internal/http/handlers/relay_profile.go
- [x] T032 [US1] Call ValidateCustomFlags on profile save and store warnings in internal/http/handlers/relay_profile.go

**Checkpoint**: Custom flags can be configured and validated, appearing in FFmpeg command

---

## Phase 5: User Story 2 - Configure Hardware Acceleration Parameters (Priority: P1)

**Goal**: Enable detailed hardware acceleration configuration (device selection, decoder options)

**Independent Test**: Configure NVIDIA/VAAPI profile with specific device, verify GPU utilization during relay

### Implementation for User Story 2

- [x] T033 [P] [US2] Implement FFmpeg hwaccels detection (parse ffmpeg -hwaccels output) in internal/services/hardware_detector.go
- [x] T034 [P] [US2] Implement encoder/decoder detection (parse ffmpeg -encoders/-decoders for hw variants) in internal/services/hardware_detector.go
- [x] T035 [US2] Implement GPU device detection (/dev/dri/renderD*, nvidia-smi, etc.) in internal/services/hardware_detector.go
- [x] T036 [US2] Create HardwareCapability struct and cache mechanism in internal/services/hardware_detector.go
- [x] T037 [US2] Add DetectHardwareCapabilities function called at startup in internal/services/hardware_detector.go
- [x] T038 [US2] Wire HWAccelOutputFormat into command builder (-hwaccel_output_format) in internal/relay/session.go
- [x] T039 [US2] Wire HWAccelDecoderCodec into command builder in internal/relay/session.go
- [x] T040 [US2] Wire GpuIndex into command builder (-hwaccel_device) in internal/relay/session.go
- [x] T041 [US2] Wire HWAccelExtraOptions into command builder in internal/relay/session.go
- [x] T042 [US2] Add hardware capabilities endpoint GET /api/v1/hardware-capabilities in internal/http/handlers/relay_profile.go
- [x] T043 [US2] Add hardware refresh endpoint POST /api/v1/hardware-capabilities/refresh in internal/http/handlers/relay_profile.go
- [x] T044 [US2] Implement graceful fallback to software encoding on hardware failure in internal/relay/session.go (leverages existing FallbackController)

**Checkpoint**: Hardware acceleration fully configurable with detected capabilities exposed via API

---

## Phase 6: User Story 3 - Create and Manage Custom Profiles (Priority: P2)

**Goal**: Enable profile cloning, editing, and management with dependency warnings

**Independent Test**: Clone system profile, modify clone, assign to channel, delete with warning

### Implementation for User Story 3

- [x] T045 [US3] Implement Clone method on RelayProfile model in internal/models/relay_profile.go
- [x] T046 [US3] Add clone endpoint handler POST /api/v1/relay-profiles/{id}/clone in internal/http/handlers/relay_profile.go
- [x] T047 [US3] Enforce system profile restrictions (only toggle enabled) in update handler in internal/http/handlers/relay_profile.go
- [x] T048 [US3] Add dependency check before delete (query proxies using profile) in internal/http/handlers/relay_profile.go
- [x] T049 [US3] Return 409 Conflict with affected channels if profile in use on delete in internal/http/handlers/relay_profile.go
- [ ] T050 [US3] Update success_count/failure_count after relay sessions in internal/relay/session.go (requires callback architecture)
- [ ] T051 [US3] Update last_used_at, last_error_at, last_error_msg on relay completion in internal/relay/session.go (requires callback architecture)

**Checkpoint**: Profile management complete with cloning and statistics tracking

---

## Phase 7: User Story 4 - Test Profile Before Deployment (Priority: P2)

**Goal**: Allow testing a profile against a sample stream before assigning to channels

**Independent Test**: Use Test Profile feature with sample URL, verify results show codec detection and status

### Implementation for User Story 4

- [ ] T052 [US4] Create ProfileTestResult struct in internal/models/profile_test_result.go (or in handler)
- [ ] T053 [US4] Implement runProfileTest function: spawn FFmpeg with 5-30s timeout, output to /dev/null in internal/relay/profile_tester.go (new file)
- [ ] T054 [US4] Parse FFmpeg stderr for frame count, FPS, codec detection in internal/relay/profile_tester.go
- [ ] T055 [US4] Parse FFmpeg stderr for hardware acceleration active verification in internal/relay/profile_tester.go
- [ ] T056 [US4] Implement error pattern matching and suggestion generation in internal/relay/profile_tester.go
- [ ] T057 [US4] Add test endpoint handler POST /api/v1/relay-profiles/{id}/test with 30s timeout in internal/http/handlers/relay_profile_handler.go
- [ ] T058 [US4] Return structured ProfileTestResult with suggestions in internal/http/handlers/relay_profile_handler.go

**Checkpoint**: Profiles can be tested with detailed diagnostic results

---

## Phase 8: User Story 5 - View FFmpeg Command Preview (Priority: P3)

**Goal**: Show the exact FFmpeg command that will be generated for debugging

**Independent Test**: View profile, see generated command, copy to clipboard for manual testing

### Implementation for User Story 5

- [ ] T059 [US5] Create CommandPreview struct in internal/models/command_preview.go (or in handler)
- [ ] T060 [US5] Implement generateCommandPreview function using CommandBuilder.Build() in internal/relay/session.go or new file
- [ ] T061 [US5] Add preview endpoint handler GET /api/v1/relay-profiles/{id}/preview in internal/http/handlers/relay_profile_handler.go
- [ ] T062 [US5] Return command as array of arguments and as single string in preview response
- [ ] T063 [US5] Log full FFmpeg command on relay start for debugging in internal/relay/session.go

**Checkpoint**: Command preview available for any profile

---

## Phase 9: Frontend UI (Priority: P2-P3)

**Goal**: Expose all new functionality in the web UI

**Independent Test**: Navigate to Settings > Relay Profiles, verify all new fields and actions available

### Implementation for Frontend

- [ ] T064 [P] Update RelayProfile TypeScript types with new fields in frontend/src/types/relay-profile.ts
- [ ] T065 [P] Add ProfileTestResult TypeScript type in frontend/src/types/relay-profile.ts
- [ ] T066 [P] Add CommandPreview TypeScript type in frontend/src/types/relay-profile.ts
- [ ] T067 [P] Add HardwareCapability TypeScript type in frontend/src/types/relay-profile.ts
- [ ] T068 [P] Add FlagValidationResult TypeScript type in frontend/src/types/relay-profile.ts
- [ ] T069 Extend profile form with Custom Input Flags textarea in frontend/src/components/relay-profiles/profile-form.tsx (or existing form component)
- [ ] T070 Extend profile form with Custom Output Flags textarea
- [ ] T071 Extend profile form with Filter Complex textarea
- [ ] T072 Extend profile form with Hardware Acceleration Device field
- [ ] T073 Extend profile form with HW Accel Output Format dropdown
- [ ] T074 Extend profile form with GPU Index number input
- [ ] T075 Add real-time flag validation with warning display to form
- [ ] T076 Create Profile Test Dialog component in frontend/src/components/relay-profiles/profile-test-dialog.tsx
- [ ] T077 Add stream URL input and duration selector to test dialog
- [ ] T078 Add test results display with suggestions to test dialog
- [ ] T079 Create Command Preview Modal component in frontend/src/components/relay-profiles/command-preview-modal.tsx
- [ ] T080 Add syntax highlighting to command display in preview modal
- [ ] T081 Add copy to clipboard button to preview modal
- [ ] T082 Add Clone button to profile list/detail view
- [ ] T083 Add Test Profile button triggering test dialog
- [ ] T084 Add Preview Command button triggering preview modal
- [ ] T085 Show system profile lock icon (non-editable) in profile list
- [ ] T086 Display profile statistics (success_count, failure_count, last_used_at) in profile detail
- [ ] T087 Add Hardware Capabilities section to settings page (display detected hardware)

**Checkpoint**: Full frontend functionality for all user stories

---

## Phase 10: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [ ] T088 Verify all new endpoints registered in router in internal/http/router.go
- [ ] T089 Add startup logging for detected hardware capabilities
- [ ] T090 Run quickstart.md validation scenarios manually
- [ ] T091 Code cleanup: ensure consistent error handling across new handlers
- [ ] T092 Verify database migration runs cleanly on fresh database
- [ ] T093 Verify existing relay profiles continue working (backward compatibility)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Story 0 (Phase 3)**: P0 CRITICAL - Must complete first after Foundational
- **User Stories 1-5 (Phase 4-8)**: All depend on User Story 0 completion
  - US1 and US2 can proceed in parallel
  - US3 can proceed after US1/US2 or in parallel
  - US4 depends on US0-US2 (uses command builder and hardware detection)
  - US5 can proceed after US0
- **Frontend (Phase 9)**: Depends on corresponding backend endpoints existing
- **Polish (Phase 10)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 0 (P0)**: Start after Foundational - CRITICAL, must complete first
- **User Story 1 (P1)**: Depends on US0 (fixed command builder) - Can start after US0
- **User Story 2 (P1)**: Depends on US0 (fixed command builder) - Can run parallel with US1
- **User Story 3 (P2)**: No strict dependencies on US1/US2 - Can start after US0
- **User Story 4 (P2)**: Depends on US0-US2 (uses full command builder) - Start after US2
- **User Story 5 (P3)**: Depends on US0 (uses command builder) - Can start after US0

### Within Each User Story

- Models/structs before services
- Services before handlers
- Handlers before frontend components
- Core implementation before integration

### Parallel Opportunities

- T002, T003, T004 (Setup): Different files, no dependencies
- T021, T022 (US1): Different validation functions
- T033, T034 (US2): Different FFmpeg parsing
- T064-T068 (Frontend): All TypeScript types can be added in parallel
- Different user stories can be worked on in parallel by different developers after US0 completes

---

## Parallel Example: Foundational Phase

```bash
# These can run in parallel:
Task: "Create bitstream filter service file in internal/ffmpeg/bitstream_filters.go"
Task: "Create flag validator service file in internal/ffmpeg/validator.go"
Task: "Create hardware detector service file in internal/services/hardware_detector.go"
```

## Parallel Example: User Story 1 + 2

```bash
# After US0 completes, these can run in parallel:

# Developer A - User Story 1:
Task: "Implement dangerous pattern detection in internal/ffmpeg/validator.go"
Task: "Implement blocked flags check in internal/ffmpeg/validator.go"

# Developer B - User Story 2:
Task: "Implement FFmpeg hwaccels detection in internal/services/hardware_detector.go"
Task: "Implement encoder/decoder detection in internal/services/hardware_detector.go"
```

---

## Implementation Strategy

### MVP First (User Story 0 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational
3. Complete Phase 3: User Story 0 (P0 CRITICAL)
4. **STOP and VALIDATE**: Test relay streams with mpv - verify no corruption errors
5. This alone fixes the blocking bug reported by users

### Incremental Delivery

1. Complete Setup + Foundational + US0 -> Stream corruption fixed (MVP!)
2. Add US1 (Custom Flags) -> Test independently -> Deploy
3. Add US2 (Hardware Accel) -> Test independently -> Deploy
4. Add US3 (Profile Management) -> Test independently -> Deploy
5. Add US4 (Profile Testing) -> Test independently -> Deploy
6. Add US5 (Command Preview) -> Test independently -> Deploy
7. Add Frontend -> Complete feature -> Deploy

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational + US0 together (P0 CRITICAL)
2. Once US0 is done:
   - Developer A: User Story 1 (Custom Flags)
   - Developer B: User Story 2 (Hardware Accel)
   - Developer C: User Story 3 (Profile Management)
3. US4 and US5 can follow as capacity allows
4. Frontend work can proceed in parallel once backend endpoints exist

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- User Story 0 (P0 CRITICAL) MUST be completed before any other work
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- The key files to modify are: internal/relay/session.go (critical), internal/ffmpeg/wrapper.go, internal/ffmpeg/bitstream_filters.go (new), internal/http/handlers/relay_profile_handler.go
