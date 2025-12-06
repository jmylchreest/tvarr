# Feature Specification: FFmpeg Profile Configuration

**Feature Branch**: `007-ffmpeg-profile-configuration`
**Created**: 2025-12-06
**Status**: Draft
**Input**: User description: "currently ffmpeg relays are generating a lot of errors and poor quality feeds. the profiles should support manual configurable flags as well as defining the accelerated params etc ourselves. I want this to work reliably, with manual configuration/additional profiles"

## User Scenarios & Testing *(mandatory)*

### User Story 0 - Fix H.264 Stream Corruption in Relay Mode (Priority: P0 - CRITICAL)

As an administrator, I want relay streams to play correctly without H.264 decoding errors so that clients receive uncorrupted video without "non-existing PPS" errors, corrupt packets, or timestamp issues.

**Why this priority**: This is a blocking bug. Current relay implementation produces corrupted MPEG-TS output with missing SPS/PPS NAL units, causing widespread playback failures. The errors observed include:
- `h264: non-existing PPS 0 referenced` - SPS/PPS not transmitted
- `mpegts: Packet corrupt` - MPEG-TS muxing issues
- `DTS out of order` - Timestamp discontinuities
- `Invalid video timestamp` - PTS/DTS corruption

**Root Cause Analysis**: The FFmpeg command is missing critical bitstream filters and flags:
1. Missing `-bsf:v h264_mp4toannexb` - Required to convert H.264 from AVCC format (length-prefixed NALUs) to Annex B format (start codes) for MPEG-TS
2. Missing `-bsf:v dump_extra` - Required to re-insert SPS/PPS at keyframes, not just at stream start
3. `-fflags +genpts` applied to output instead of input
4. Missing `-flush_packets 1` for immediate packet output
5. Missing `-muxdelay 0` for reduced live streaming latency

**Independent Test**: Can be fully tested by:
1. Starting a relay with any H.264 HLS source
2. Playing the output with mpv/ffplay
3. Verifying no "non-existing PPS" or "Packet corrupt" errors in player output
4. Confirming video plays smoothly without artifacts

**Acceptance Scenarios**:

1. **Given** an H.264 HLS stream relayed through tvarr, **When** played with mpv, **Then** no "non-existing PPS" errors occur
2. **Given** an H.264 stream with AVCC NAL format, **When** relayed to MPEG-TS, **Then** the output uses Annex B format with proper start codes
3. **Given** a live relay session, **When** a client joins mid-stream, **Then** they receive SPS/PPS at the next keyframe and can decode video immediately
4. **Given** any relay profile (copy or transcode), **When** outputting MPEG-TS, **Then** packets are not marked as corrupt and DTS values are in order
5. **Given** a relay with copy mode, **When** streaming to MPEG-TS, **Then** the h264_mp4toannexb bitstream filter is applied automatically

---

### User Story 1 - Add Custom FFmpeg Flags to Profiles (Priority: P1)

As an administrator, I want to add custom FFmpeg command-line flags to relay profiles so that I can fine-tune transcoding behavior for specific use cases that aren't covered by the standard profile fields.

**Why this priority**: This is the core request - users need escape hatches when the structured profile fields don't provide enough control. Without custom flags, advanced users are blocked from solving quality and compatibility issues.

**Independent Test**: Can be fully tested by creating a profile with custom input/output flags, starting a relay session, and verifying the FFmpeg command includes the custom flags.

**Acceptance Scenarios**:

1. **Given** a relay profile with custom input flags `-fflags +genpts`, **When** a relay session starts, **Then** the FFmpeg command includes `-fflags +genpts` before the input
2. **Given** a relay profile with custom output flags `-movflags +faststart`, **When** a relay session starts, **Then** the FFmpeg command includes `-movflags +faststart` in the output arguments
3. **Given** a profile with invalid custom flags `--invalid-flag`, **When** the profile is saved, **Then** the system validates the flags and displays a warning (but allows saving for advanced use cases)
4. **Given** a profile with both custom flags and structured settings, **When** a relay starts, **Then** custom flags are appended after structured settings (allowing override behavior)

---

### User Story 2 - Configure Hardware Acceleration Parameters (Priority: P1)

As an administrator, I want to configure detailed hardware acceleration parameters (device selection, decoder options, encoder presets) so that I can optimize transcoding performance for my specific hardware.

**Why this priority**: Hardware acceleration is critical for performance but current options are limited. Users with NVIDIA, Intel, or AMD hardware need fine-grained control to avoid errors and maximize throughput.

**Independent Test**: Can be fully tested by configuring a hardware-accelerated profile with specific device and decoder settings, running a relay, and verifying GPU utilization and successful transcoding.

**Acceptance Scenarios**:

1. **Given** an NVIDIA system with multiple GPUs, **When** I configure a profile with `hwaccel_device: "0"`, **Then** FFmpeg uses the specified GPU
2. **Given** a VAAPI-enabled system, **When** I configure a profile with `vaapi_device: "/dev/dri/renderD128"`, **Then** FFmpeg uses the specified render device
3. **Given** a hardware-accelerated profile, **When** I configure decoder-specific options (e.g., `nvdec_surfaces: 25`), **Then** these options are passed to FFmpeg
4. **Given** invalid hardware acceleration settings, **When** the relay starts, **Then** the system gracefully falls back to software encoding with a logged warning

---

### User Story 3 - Create and Manage Custom Profiles (Priority: P2)

As an administrator, I want to create, clone, edit, and delete custom relay profiles so that I can maintain a library of configurations optimized for different stream types and quality requirements.

**Why this priority**: Users need multiple profiles for different scenarios (4K streams, mobile-friendly, bandwidth-constrained). Profile management enables experimentation without affecting production.

**Independent Test**: Can be fully tested by creating a custom profile, cloning it, modifying the clone, and assigning different profiles to different channels.

**Acceptance Scenarios**:

1. **Given** the profile management UI, **When** I create a new profile with a unique name, **Then** the profile is saved and available for assignment
2. **Given** a system profile, **When** I click "Clone", **Then** a user-editable copy is created with "(Copy)" appended to the name
3. **Given** a custom profile assigned to channels, **When** I attempt to delete it, **Then** the system warns about affected channels and requires confirmation
4. **Given** a profile with all settings configured, **When** I export it, **Then** I receive a configuration that can be imported on another instance

---

### User Story 4 - Test Profile Before Deployment (Priority: P2)

As an administrator, I want to test a relay profile against a sample stream before deploying it to production channels so that I can validate settings work correctly and produce acceptable quality.

**Why this priority**: Profile misconfiguration causes relay failures. Testing prevents outages by catching errors before profiles are assigned to live channels.

**Independent Test**: Can be fully tested by using the "Test Profile" feature with a sample stream URL and observing the test results including any errors or warnings.

**Acceptance Scenarios**:

1. **Given** a configured profile and a test stream URL, **When** I click "Test Profile", **Then** the system runs a short transcoding test and reports success/failure
2. **Given** a profile test in progress, **When** FFmpeg encounters an error, **Then** the error message is displayed with suggestions for common fixes
3. **Given** a successful profile test, **When** viewing results, **Then** I see codec detection, bitrate measurement, and estimated resource usage
4. **Given** a profile with hardware acceleration, **When** testing, **Then** the test verifies hardware acceleration is actually being used (not falling back to software)

---

### User Story 5 - View FFmpeg Command Preview (Priority: P3)

As an administrator, I want to preview the exact FFmpeg command that will be generated from a profile so that I can debug issues and verify my configuration is correct.

**Why this priority**: Debugging FFmpeg issues requires seeing the actual command. This transparency helps users understand how their profile settings translate to FFmpeg arguments.

**Independent Test**: Can be fully tested by configuring a profile and viewing the generated FFmpeg command preview.

**Acceptance Scenarios**:

1. **Given** a configured profile, **When** I view the profile details, **Then** I see a read-only preview of the FFmpeg command that would be generated
2. **Given** the command preview, **When** I click "Copy Command", **Then** the command is copied to clipboard for manual testing
3. **Given** changes to profile settings, **When** I modify a field, **Then** the command preview updates in real-time

---

### Edge Cases

- What happens when an H.264 stream in AVCC format is relayed to MPEG-TS?
  - System automatically applies h264_mp4toannexb bitstream filter to convert to Annex B format with proper start codes
- What happens when a client joins mid-stream?
  - Client receives SPS/PPS at the next keyframe due to dump_extra filter; frequent PAT/PMT aids decoder initialization
- What happens when source timestamps are corrupt or discontinuous?
  - System applies genpts+discardcorrupt on input and avoid_negative_ts on output to regenerate valid timestamps
- What happens when custom flags conflict with structured settings?
  - Custom flags are appended last and can override structured settings
- How does the system handle hardware acceleration failure mid-stream?
  - Falls back to software encoding with logged warning; admin notification optional
- What happens when a profile references hardware that doesn't exist?
  - Relay fails fast with clear error message; suggests running hardware detection
- How are profiles validated when FFmpeg version changes?
  - System validates flag syntax on save but warns that runtime behavior depends on FFmpeg version

## Requirements *(mandatory)*

### Functional Requirements

**Stream Reliability (P0 - CRITICAL)**

- **FR-100**: System MUST apply `-bsf:v h264_mp4toannexb` bitstream filter when copying H.264 video to MPEG-TS output
- **FR-101**: System MUST apply `-bsf:v dump_extra` or equivalent to ensure SPS/PPS NAL units are re-inserted at keyframes
- **FR-102**: System MUST apply `-fflags +genpts+discardcorrupt` to INPUT (not output) for proper PTS generation
- **FR-103**: System MUST apply `-flush_packets 1` for immediate packet output in live streaming
- **FR-104**: System MUST apply `-muxdelay 0` to minimize muxing latency
- **FR-105**: System MUST NOT use `-mpegts_copyts 1` with corrupt source timestamps; use `-avoid_negative_ts make_zero` instead
- **FR-106**: System MUST detect H.264 codec and automatically apply appropriate bitstream filters based on output format
- **FR-107**: System MUST support HEVC (H.265) streams with equivalent bitstream filters (`hevc_mp4toannexb`)
- **FR-108**: System SHOULD prefer `-pat_period 0.1` for frequent PAT/PMT insertion to aid mid-stream joins

**Profile Configuration**

- **FR-001**: System MUST support custom input flags field on relay profiles (string, whitespace-separated)
- **FR-002**: System MUST support custom output flags field on relay profiles (string, whitespace-separated)
- **FR-003**: System MUST support custom filter flags field for complex filter chains
- **FR-004**: Custom flags MUST be appended after corresponding structured settings in the FFmpeg command
- **FR-005**: System MUST validate custom flags syntax on save (balanced quotes, no shell injection)

**Hardware Acceleration**

- **FR-006**: System MUST support hardware acceleration device selection (device index or path)
- **FR-007**: System MUST support hardware decoder configuration options
- **FR-008**: System MUST support hardware encoder preset/quality settings
- **FR-009**: System MUST detect available hardware acceleration capabilities at startup
- **FR-010**: System MUST provide graceful fallback when hardware acceleration fails

**Profile Management**

- **FR-011**: System MUST allow creating custom profiles with unique names
- **FR-012**: System MUST allow cloning existing profiles (system or custom)
- **FR-013**: System MUST allow editing custom profiles (system profiles remain read-only except enable/disable)
- **FR-014**: System MUST allow deleting custom profiles with dependency warning
- **FR-015**: System MUST support profile import/export in portable format

**Testing and Preview**

- **FR-016**: System MUST provide profile testing capability against sample streams
- **FR-017**: System MUST display FFmpeg command preview for configured profiles
- **FR-018**: System MUST capture and display FFmpeg errors during relay with context
- **FR-019**: Profile tests MUST timeout after configurable duration (default 30 seconds)

**Reliability**

- **FR-020**: System MUST log full FFmpeg command on relay start for debugging
- **FR-021**: System MUST detect and report common FFmpeg failure patterns
- **FR-022**: System MUST track profile success/failure rates per profile

### Key Entities

- **RelayProfile**: Configuration for stream transcoding including codecs, quality settings, hardware acceleration, and custom flags. Key attributes: name, description, video settings, audio settings, hw acceleration settings, custom input/output/filter flags, is_system flag, enabled flag.

- **HardwareCapability**: Detected hardware acceleration capability on the system. Key attributes: type (NVENC, QSV, VAAPI, etc.), device identifier, supported codecs, detected at timestamp.

- **ProfileTestResult**: Result of testing a profile against a sample stream. Key attributes: profile reference, test stream URL, success/failure status, error messages, detected codecs, measured bitrate, resource usage, tested at timestamp.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-000**: Relay streams produce zero "non-existing PPS" or "Packet corrupt" errors when played with mpv/ffplay (P0 CRITICAL)
- **SC-001**: Administrators can configure custom FFmpeg flags and have them appear in the generated command within 1 minute of profile creation
- **SC-002**: Hardware acceleration configuration results in measurable GPU utilization (>10%) during transcoding on supported hardware
- **SC-003**: Profile testing provides pass/fail feedback within 30 seconds for typical streams
- **SC-004**: Relay failure rate decreases by 50% for streams using properly configured profiles (compared to default settings)
- **SC-005**: 90% of profile configuration changes take effect without requiring service restart
- **SC-006**: FFmpeg errors are captured and displayed to administrators within 5 seconds of occurrence
- **SC-007**: System detects available hardware acceleration within 10 seconds of startup
- **SC-008**: Clients joining mid-stream can begin decoding video within 2 GOP intervals (typically <10 seconds)

## Assumptions

- FFmpeg binary is installed and accessible on the system
- Administrators have basic understanding of FFmpeg concepts (codecs, flags, hardware acceleration)
- Hardware acceleration drivers are properly installed when hardware encoding is desired
- Test streams are accessible from the server running tvarr
- Profiles are stored in the database and can be modified at runtime
