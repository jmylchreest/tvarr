# Implementation Plan: FFmpeg Profile Configuration

**Branch**: `007-ffmpeg-profile-configuration` | **Date**: 2025-12-06 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/007-ffmpeg-profile-configuration/spec.md`

## Summary

This feature addresses critical H.264 stream corruption in relay mode (P0) and adds comprehensive FFmpeg profile configuration including custom flags, hardware acceleration parameters, and profile management. The primary issue is missing bitstream filters (`h264_mp4toannexb`) and misplaced FFmpeg flags causing SPS/PPS errors and corrupt MPEG-TS output.

## Technical Context

**Language/Version**: Go 1.25.x (latest stable)
**Primary Dependencies**: Huma v2.34+ (Chi router), GORM v2, FFmpeg (external binary)
**Storage**: SQLite/PostgreSQL/MySQL (configurable via GORM)
**Testing**: Go test with integration tests
**Target Platform**: Linux server (primary), macOS, Windows
**Project Type**: Web application (Go backend + Next.js frontend)
**Performance Goals**: Zero stream corruption errors, <100ms latency for profile operations
**Constraints**: Must maintain backward compatibility with existing profiles
**Scale/Scope**: Existing relay profile system extension

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- [x] No new external dependencies required (using existing FFmpeg binary)
- [x] Extends existing models rather than creating parallel structures
- [x] API endpoints follow existing patterns in `internal/http/handlers/`
- [x] Frontend components follow existing patterns in `frontend/src/components/`

## Project Structure

### Documentation (this feature)

```text
specs/007-ffmpeg-profile-configuration/
├── plan.md              # This file
├── research.md          # Phase 0 output - completed
├── data-model.md        # Phase 1 output - completed
├── quickstart.md        # Phase 1 output - completed
├── contracts/           # Phase 1 output - completed
│   └── openapi.yaml
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
internal/
├── models/
│   └── relay_profile.go     # Extended with new fields
├── ffmpeg/
│   ├── wrapper.go           # Extended command builder
│   ├── bitstream_filters.go # NEW: Codec-to-BSF mapping
│   └── validator.go         # NEW: Custom flag validation
├── relay/
│   └── session.go           # Fixed FFmpeg command generation
├── http/handlers/
│   └── relay_profile.go     # Extended with new endpoints
└── services/
    └── hardware_detector.go # NEW: Hardware capability detection

frontend/src/
├── components/
│   └── relay-profiles/
│       ├── profile-form.tsx     # Extended with new fields
│       ├── profile-test.tsx     # NEW: Test dialog
│       └── command-preview.tsx  # NEW: Preview modal
├── types/
│   └── relay-profile.ts     # Extended types
└── pages/settings/
    └── relay-profiles.tsx   # Extended UI
```

**Structure Decision**: Web application structure - extending existing backend and frontend directories.

## Implementation Phases

### Phase 1: P0 CRITICAL - Fix Stream Corruption (FR-100 to FR-152)

This is the highest priority work and MUST be completed first.

#### 1.1 Create Bitstream Filter Service

**File**: `internal/ffmpeg/bitstream_filters.go`

Create a service that maps codec + output format combinations to required bitstream filters:

| Source Codec | Output Format | Video BSF | Notes |
|--------------|---------------|-----------|-------|
| H.264 | MPEG-TS/HLS | `h264_mp4toannexb` | Converts AVCC to Annex B |
| H.265/HEVC | MPEG-TS/HLS | `hevc_mp4toannexb` | Converts HVCC to Annex B |
| VP9 | MPEG-TS | `vp9_superframe` | Conditional for some sources |
| AV1 | MPEG-TS | - | OBUs, no conversion needed |

Hardware encoders map to their codec family:
- `h264_nvenc`, `h264_qsv`, `h264_vaapi` → `h264_mp4toannexb`
- `hevc_nvenc`, `hevc_qsv`, `hevc_vaapi` → `hevc_mp4toannexb`

#### 1.2 Fix FFmpeg Command Builder in session.go

**File**: `internal/relay/session.go` - `runFFmpegPipeline()`

Current (broken):
```go
builder.OutputFormat(string(s.Profile.OutputFormat)).
    OutputArgs("-mpegts_copyts", "1").
    OutputArgs("-avoid_negative_ts", "disabled").
    OutputArgs("-fflags", "+genpts").  // WRONG: on output
```

Fixed:
```go
// INPUT flags (before -i)
builder.InputArgs("-fflags", "+genpts+discardcorrupt").
    InputArgs("-analyzeduration", "10000000").
    InputArgs("-probesize", "10000000").
    Input(inputURL)

// ... codec settings ...

// Determine BSF based on output codec and format
bsf := GetBitstreamFilter(videoCodec, outputFormat)
if bsf != "" {
    builder.OutputArgs("-bsf:v", bsf)
}

// OUTPUT flags
builder.OutputFormat(string(s.Profile.OutputFormat)).
    OutputArgs("-flush_packets", "1").
    OutputArgs("-muxdelay", "0").
    OutputArgs("-avoid_negative_ts", "make_zero").
    OutputArgs("-pat_period", "0.1").
    Output("pipe:1")
```

#### 1.3 Add Codec Detection for Copy Mode

When profile uses `copy` mode, probe the source stream to detect the actual codec and apply the appropriate BSF:

```go
func (s *Session) detectSourceCodec(inputURL string) (string, error) {
    // Use ffprobe to detect source codec
    // Return codec name for BSF selection
}
```

### Phase 2: Wire Existing Custom Flag Fields (FR-001 to FR-005)

The `InputOptions`, `OutputOptions`, and `FilterComplex` fields exist in the model but are NOT connected to the FFmpeg command builder.

#### 2.1 Extend CommandBuilder

**File**: `internal/ffmpeg/wrapper.go`

Add methods to apply custom options:
```go
func (b *CommandBuilder) ApplyInputOptions(opts string) *CommandBuilder
func (b *CommandBuilder) ApplyOutputOptions(opts string) *CommandBuilder
func (b *CommandBuilder) ApplyFilterComplex(filter string) *CommandBuilder
```

#### 2.2 Create Flag Validator

**File**: `internal/ffmpeg/validator.go`

Implement validation with dangerous pattern detection:
- Shell injection patterns: `;`, `|`, `&&`, `$()`
- Blocked flags: `-i`, `-y`, `-filter_script`
- Quote balancing

Warning-only mode allows saving with warnings for advanced users.

#### 2.3 Update session.go to Apply Custom Options

Wire the custom options into the command builder in proper order:
1. Structured input settings
2. Custom input options (override)
3. Input URL
4. Codec settings
5. Filter complex
6. Structured output settings
7. Custom output options (override)

### Phase 3: Hardware Acceleration Extensions (FR-006 to FR-010)

#### 3.1 Add New Model Fields

**File**: `internal/models/relay_profile.go`

Add fields (as documented in data-model.md):
- `HWAccelOutputFormat`
- `HWAccelDecoderCodec`
- `HWAccelExtraOptions`
- `GpuIndex`

#### 3.2 Create Hardware Detector Service

**File**: `internal/services/hardware_detector.go`

Detect available hardware at startup:
- Parse `ffmpeg -hwaccels` output
- Parse `ffmpeg -encoders` for hardware encoders
- Detect GPU devices and paths
- Cache results in memory

#### 3.3 Wire Hardware Settings to Command Builder

Apply hardware acceleration flags in proper order:
```go
// Decoder settings (before input)
if profile.HWAccel != "none" {
    builder.InputArgs("-hwaccel", profile.HWAccel)
    if profile.HWAccelDevice != "" {
        builder.InputArgs("-hwaccel_device", profile.HWAccelDevice)
    }
    if profile.HWAccelOutputFormat != "" {
        builder.InputArgs("-hwaccel_output_format", profile.HWAccelOutputFormat)
    }
}
```

### Phase 4: Profile Management API (FR-011 to FR-015)

#### 4.1 Extend Handler with New Endpoints

**File**: `internal/http/handlers/relay_profile.go`

Add endpoints:
- `POST /api/v1/relay-profiles/{id}/clone` - Clone profile
- `POST /api/v1/relay-profiles/{id}/test` - Test profile
- `GET /api/v1/relay-profiles/{id}/preview` - Preview command
- `POST /api/v1/relay-profiles/validate-flags` - Validate flags
- `GET /api/v1/hardware-capabilities` - Get capabilities
- `POST /api/v1/hardware-capabilities/refresh` - Refresh detection

#### 4.2 Implement Profile Cloning

Clone any profile (system or custom) to create an editable copy.

#### 4.3 Implement Profile Testing

Run a short FFmpeg test (5-30 seconds) against a provided stream URL:
- Capture stderr for diagnostics
- Parse frame count, FPS, codec detection
- Verify hardware acceleration is active
- Return structured results with suggestions

### Phase 5: Frontend UI (P2-P3)

#### 5.1 Extend Profile Form

Add form fields for:
- Custom Input Flags (textarea)
- Custom Output Flags (textarea)
- Filter Complex (textarea)
- Hardware Acceleration Device
- HW Accel Output Format
- GPU Index

Real-time validation with warning display.

#### 5.2 Profile Test Dialog

Modal dialog for testing profiles:
- Stream URL input
- Duration selector
- Progress indicator
- Results display with suggestions

#### 5.3 Command Preview Modal

Display generated FFmpeg command:
- Syntax-highlighted command
- Copy to clipboard button
- Warning display

### Phase 6: Database Migration

**File**: `internal/database/migrations/NNNN_ffmpeg_profile_extensions.go`

Add migration for new fields as documented in data-model.md.

## Complexity Tracking

No constitution violations identified. All changes extend existing patterns without introducing new architectural complexity.

## Dependencies

| Phase | Depends On | Reason |
|-------|------------|--------|
| Phase 2 | Phase 1 | Custom flags build on fixed command builder |
| Phase 3 | Phase 1 | HW accel uses fixed command structure |
| Phase 4 | Phase 1-3 | API exposes all functionality |
| Phase 5 | Phase 4 | Frontend calls new API endpoints |
| Phase 6 | Phase 3 | Migration adds new fields |

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| Breaking existing profiles | Phase 1 changes are additive, existing profiles continue working |
| FFmpeg version differences | Log applied flags, validate syntax only (not runtime behavior) |
| Hardware detection fails | Graceful fallback to software, logged warning |
| Custom flags cause crashes | Capture FFmpeg stderr, report suggestions |

## Success Metrics

- **SC-000**: Zero "non-existing PPS" or "Packet corrupt" errors (P0 CRITICAL)
- **SC-001**: Custom flags appear in generated command within 1 minute
- **SC-002**: GPU utilization >10% with hardware acceleration
- **SC-003**: Profile testing completes within 30 seconds
- **SC-008**: Mid-stream clients decode within 2 GOP intervals
