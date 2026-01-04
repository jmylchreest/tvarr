# Research: FFmpeg Profile Configuration

**Feature**: FFmpeg Profile Configuration
**Branch**: `007-ffmpeg-profile-configuration`
**Date**: 2025-12-06

## Research Objectives

1. **P0 CRITICAL**: Fix H.264 stream corruption in relay mode (missing SPS/PPS, corrupt packets)
2. Determine best practices for FFmpeg flag injection safety
3. Research hardware acceleration configuration patterns
4. Investigate profile testing approaches for live streams
5. Analyze existing relay profile implementation gaps

## Findings

### R0: H.264 Stream Corruption Root Cause (P0 CRITICAL)

**Observed Symptoms** (from user testing with mpv):
```
[ffmpeg/video] h264: non-existing PPS 0 referenced
[ffmpeg/video] h264: no frame!
[ffmpeg/demuxer] mpegts: Packet corrupt (stream = 0, dts = 18000)
[ffmpeg/demuxer] mpegts: DTS 2512800 < 2516400 out of order
Invalid video timestamp: 3.614333 -> 3.614333
```

**Root Cause**: The FFmpeg command in `session.go:runFFmpegPipeline()` is missing critical flags:

**Current (Broken) Code**:
```go
builder := ffmpeg.NewCommandBuilder(binInfo.FFmpegPath).
    InputArgs("-analyzeduration", "10000000").
    InputArgs("-probesize", "10000000").
    Input(inputURL)
// ... codec settings ...
builder.OutputFormat(string(s.Profile.OutputFormat)).
    OutputArgs("-mpegts_copyts", "1").
    OutputArgs("-avoid_negative_ts", "disabled").
    OutputArgs("-fflags", "+genpts").  // WRONG: on output, not input
    Output("pipe:1")
```

**Issues**:
1. **Missing `-bsf:v h264_mp4toannexb`**: HLS/MP4 sources use AVCC format (length-prefixed NALUs). MPEG-TS requires Annex B format (start codes). Without this filter, the NAL unit headers are wrong.
2. **Missing `-bsf:v dump_extra`**: SPS/PPS NAL units are only sent once at stream start. Late-joining clients never receive them. This filter re-inserts them at keyframes.
3. **`-fflags +genpts` on wrong side**: Should be on INPUT to fix source timestamp issues, not output.
4. **`-mpegts_copyts` with corrupt source**: Copies broken timestamps. Should regenerate them.
5. **Missing `-flush_packets 1`**: Packets may be buffered, causing delays.

**Correct FFmpeg Command Pattern**:
```bash
ffmpeg \
  -fflags +genpts+discardcorrupt \
  -analyzeduration 10000000 \
  -probesize 10000000 \
  -i "input.m3u8" \
  -map 0:v:0 -map 0:a:0? \
  -c:v copy \
  -c:a copy \
  -bsf:v h264_mp4toannexb \
  -f mpegts \
  -flush_packets 1 \
  -muxdelay 0 \
  -avoid_negative_ts make_zero \
  -pat_period 0.1 \
  pipe:1
```

**Key Fixes**:
| Fix | Flag | Purpose |
|-----|------|---------|
| NAL format conversion | `-bsf:v h264_mp4toannexb` | Convert AVCC to Annex B |
| SPS/PPS re-insertion | (automatic with annexb) | Include at keyframes |
| Timestamp generation | `-fflags +genpts` on INPUT | Generate valid PTS |
| Corrupt frame handling | `-fflags +discardcorrupt` | Drop bad frames |
| Immediate output | `-flush_packets 1` | No buffering delay |
| Mux delay | `-muxdelay 0` | Zero muxing delay |
| Negative TS fix | `-avoid_negative_ts make_zero` | Fix timestamp wrap |
| PAT/PMT frequency | `-pat_period 0.1` | 100ms for fast channel joins |

**Complete Codec-to-Bitstream-Filter Matrix**:

| Source Codec | Output Format | Video BSF | Audio BSF | Notes |
|--------------|---------------|-----------|-----------|-------|
| H.264 (any) | MPEG-TS | `h264_mp4toannexb` | - | Converts AVCC to Annex B |
| H.264 (any) | HLS | `h264_mp4toannexb` | - | HLS uses MPEG-TS segments |
| H.264 (any) | FLV | - | `aac_adtstoasc` | FLV uses AVCC natively |
| H.264 (any) | MP4 | - | `aac_adtstoasc` | MP4 uses AVCC natively |
| H.264 (any) | MKV | - | - | MKV handles both formats |
| HEVC/H.265 (any) | MPEG-TS | `hevc_mp4toannexb` | - | Converts HVCC to Annex B |
| HEVC/H.265 (any) | HLS | `hevc_mp4toannexb` | - | HLS uses MPEG-TS segments |
| HEVC/H.265 (any) | FLV | - | - | Limited HEVC support in FLV |
| HEVC/H.265 (any) | MP4 | - | `aac_adtstoasc` | MP4 uses HVCC natively |
| VP9 | MPEG-TS | `vp9_superframe` | - | May need superframe for some sources |
| VP9 | WebM | - | - | Native format |
| AV1 | MPEG-TS | - | - | AV1 uses OBUs, no conversion needed |
| AV1 | MP4 | - | - | Native support |

**Hardware Encoder Codec Families**:

| Encoder | Codec Family | BSF for MPEG-TS |
|---------|--------------|-----------------|
| `h264_nvenc` | H.264 | `h264_mp4toannexb` |
| `h264_qsv` | H.264 | `h264_mp4toannexb` |
| `h264_vaapi` | H.264 | `h264_mp4toannexb` |
| `h264_videotoolbox` | H.264 | `h264_mp4toannexb` |
| `hevc_nvenc` | HEVC | `hevc_mp4toannexb` |
| `hevc_qsv` | HEVC | `hevc_mp4toannexb` |
| `hevc_vaapi` | HEVC | `hevc_mp4toannexb` |
| `hevc_videotoolbox` | HEVC | `hevc_mp4toannexb` |
| `libx264` | H.264 | `h264_mp4toannexb` |
| `libx265` | HEVC | `hevc_mp4toannexb` |
| `libvpx-vp9` | VP9 | `vp9_superframe` (conditional) |
| `libaom-av1` | AV1 | - |

**Audio Codec Handling**:

| Audio Codec | To MPEG-TS | To MP4/FLV | Notes |
|-------------|------------|------------|-------|
| AAC | Default (ADTS) | `aac_adtstoasc` | ADTS for TS, ASC for MP4 |
| AC3 | Native | Native | Direct passthrough |
| EAC3 | Native | Native | Direct passthrough |
| MP3 | Native | Native | Direct passthrough |
| Opus | Transcode to AAC | Native | Opus not in MPEG-TS spec |

**Decision**: Fix the FFmpeg command builder in session.go to apply these flags automatically based on codec detection and output format.

---

### R1: Existing RelayProfile Fields Analysis

**Current Model Fields (internal/models/relay_profile.go)**:

```go
// Already exists but NOT wired to FFmpeg command builder:
InputOptions  string `gorm:"size:1000" json:"input_options,omitempty"`
OutputOptions string `gorm:"size:1000" json:"output_options,omitempty"`
FilterComplex string `gorm:"size:2000" json:"filter_complex,omitempty"`

// Hardware acceleration (exists):
HWAccel       HWAccelType `gorm:"size:50;default:'none'" json:"hw_accel"`
HWAccelDevice string      `gorm:"size:100" json:"hw_accel_device,omitempty"`
```

**Gap Analysis**:
- `InputOptions`, `OutputOptions`, `FilterComplex` fields exist but are NOT applied in `session.go:runFFmpegPipeline()`
- No validation for these fields in `RelayProfile.Validate()`
- No frontend UI exposes these fields
- Hardware acceleration options are limited (no decoder-specific options)

**Decision**: Wire existing fields into command builder, add validation, extend HW accel options

### R2: FFmpeg Flag Injection Prevention

**Attack Vectors**:
1. Shell metacharacter injection (`;`, `|`, `&&`, `$()`)
2. File path traversal via `-i` overwrite
3. Command substitution via backticks

**Mitigations (Best Practices)**:
1. **Never use shell execution** - Use `exec.Command()` directly (already done in tvarr)
2. **Validate flag patterns** - Must start with `-` or `--`
3. **Blocklist dangerous flags** - `-i`, `-y`, `-filter_script`, etc.
4. **Quote validation** - Balanced quotes only
5. **No environment variable expansion** - Reject `$VAR` patterns

**Validation Regex Patterns**:
```go
// Dangerous patterns to reject
dangerousPatterns := []string{
    `\$\(`,           // Command substitution $(...)
    "`",              // Backtick substitution
    `\$\{`,           // Variable expansion ${...}
    `\$[A-Za-z_]`,    // Variable reference $VAR
    `;`,              // Command separator
    `\|(?!\|)`,       // Pipe (but allow ||)
    `&&`,             // Command chaining
    `>>?`,            // Redirection
    `<`,              // Input redirection
}

// Flags that should never be in custom options
blockedFlags := []string{
    "-i",             // Input specification (controlled separately)
    "-y",             // Overwrite output (controlled separately)
    "-n",             // Never overwrite
    "-filter_script", // Could load arbitrary script files
    "-f concat",      // Concat demuxer security risk
    "-protocol_whitelist", // Could enable dangerous protocols
}
```

**Decision**: Implement validation service with warning-only mode for advanced users

### R3: Hardware Acceleration Configuration Patterns

**FFmpeg Hardware Acceleration Stack**:

```
┌─────────────────────────────────────────────────────────┐
│                    Input Stream                          │
└─────────────────────────┬───────────────────────────────┘
                          │
              ┌───────────▼───────────┐
              │   Hardware Decoder    │ (-hwaccel, -c:v xxx_cuvid)
              │   NVDEC/QSV/VAAPI     │
              └───────────┬───────────┘
                          │
              ┌───────────▼───────────┐
              │   GPU Memory Format   │ (-hwaccel_output_format)
              │   cuda/qsv/vaapi      │
              └───────────┬───────────┘
                          │
              ┌───────────▼───────────┐
              │   Video Filters       │ (scale_cuda, overlay_cuda)
              │   (GPU-accelerated)   │
              └───────────┬───────────┘
                          │
              ┌───────────▼───────────┐
              │   Hardware Encoder    │ (-c:v h264_nvenc)
              │   NVENC/QSV/VAAPI     │
              └───────────┬───────────┘
                          │
              ┌───────────▼───────────┐
              │    Output Stream      │
              └───────────────────────┘
```

**New Fields Needed**:

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `HWAccelOutputFormat` | string | Memory format for hwaccel | `cuda`, `qsv`, `vaapi` |
| `HWAccelDecoderCodec` | string | Hardware decoder codec | `h264_cuvid`, `hevc_qsv` |
| `HWAccelExtraOptions` | string | Additional hwaccel options | `-extra_hw_frames 10` |
| `GpuIndex` | int | GPU device index | `0`, `1` |

**Platform-Specific Considerations**:

| Platform | Acceleration | Device Path | Init Options |
|----------|--------------|-------------|--------------|
| NVIDIA | CUDA/NVDEC | GPU index (0,1) | `-hwaccel cuda -hwaccel_device 0` |
| Intel | QSV | `/dev/dri/renderD128` | `-init_hw_device qsv=hw -filter_hw_device hw` |
| AMD/Intel Linux | VAAPI | `/dev/dri/renderD128` | `-vaapi_device /dev/dri/renderD128 -hwaccel vaapi` |
| macOS | VideoToolbox | N/A | `-hwaccel videotoolbox` |

**Decision**: Add `HWAccelOutputFormat`, `GpuIndex` fields; use `InputOptions` for advanced decoder options

### R4: Profile Testing Strategy

**Testing Approach**:

1. **Quick Validation Test** (Default):
   - Duration: 5 seconds of transcoding
   - Input: User-provided test stream URL
   - Output: `/dev/null` (discard)
   - Capture: stderr for FFmpeg diagnostics
   - Parse: Frame count, FPS, bitrate, codec detection

2. **Test Result Structure**:
```go
type ProfileTestResult struct {
    Success          bool            `json:"success"`
    Duration         time.Duration   `json:"duration"`
    FramesProcessed  int64           `json:"frames_processed"`
    FPS              float64         `json:"fps"`
    DetectedCodecs   []string        `json:"detected_codecs"`
    HWAccelActive    bool            `json:"hw_accel_active"`
    HWAccelDevice    string          `json:"hw_accel_device,omitempty"`
    Warnings         []string        `json:"warnings,omitempty"`
    Errors           []string        `json:"errors,omitempty"`
    FFmpegOutput     string          `json:"ffmpeg_output,omitempty"`
    Suggestions      []string        `json:"suggestions,omitempty"`
    CommandExecuted  string          `json:"command_executed"`
}
```

3. **Hardware Acceleration Verification**:
   - Parse FFmpeg output for device initialization messages
   - Check for encoder/decoder names containing platform suffix (_nvenc, _qsv, _vaapi)
   - Verify no fallback warnings in output

4. **Common Error Patterns & Suggestions**:

| Error Pattern | Suggestion |
|---------------|------------|
| `NVENC session cap exceeded` | Reduce concurrent encoding sessions or use consumer GPU |
| `Cannot load libcuda.so` | NVIDIA drivers not installed or not in PATH |
| `No VA display` | VAAPI not available - check /dev/dri permissions |
| `decoder (h264) not found` | Install FFmpeg with codec support |
| `Connection refused` | Stream URL not accessible - check network |
| `Invalid data found` | Stream format not recognized - try different analyzer settings |

**Decision**: Implement test endpoint with 30s timeout, structured results, and suggestions

### R5: Command Preview Implementation

**Approach**:
- Use existing `CommandBuilder.Build().String()` method
- Generate preview without executing
- Include placeholder for stream URL: `{{STREAM_URL}}`

**Preview Response Structure**:
```go
type CommandPreview struct {
    Command     string   `json:"command"`
    Arguments   []string `json:"arguments"`
    Environment []string `json:"environment,omitempty"`
    Warnings    []string `json:"warnings,omitempty"`
}
```

**Decision**: Add preview endpoint to relay profile handler, reuse CommandBuilder

### R6: Existing Code Integration Points

**Files to Modify**:

1. **internal/models/relay_profile.go**:
   - Add new HW accel fields
   - Add validation logic for custom flags

2. **internal/ffmpeg/wrapper.go**:
   - Extend `CommandBuilder` with custom options methods
   - Add methods for HW accel output format

3. **internal/relay/session.go**:
   - Wire `InputOptions`, `OutputOptions`, `FilterComplex` into FFmpeg command
   - Apply HW accel configuration properly

4. **internal/http/handlers/relay_profile.go** (or create new):
   - Add test profile endpoint
   - Add command preview endpoint

5. **Frontend components**:
   - Profile form with custom flags fields
   - Hardware acceleration configuration UI
   - Test dialog with results
   - Command preview with copy button

## Summary of Decisions

| ID | Decision | Rationale |
|----|----------|-----------|
| **D0** | **Fix FFmpeg command with h264_mp4toannexb + proper flags (P0 CRITICAL)** | **Required to fix stream corruption - SPS/PPS missing, corrupt packets** |
| D1 | Wire existing `InputOptions`/`OutputOptions`/`FilterComplex` fields | Fields already exist in model, just not connected |
| D2 | Warning-only validation for custom flags | Allow advanced users to use edge cases |
| D3 | Add `HWAccelOutputFormat`, `GpuIndex` fields | Enable proper GPU pipeline configuration |
| D4 | 30s timeout for profile testing | Balance between thorough test and responsiveness |
| D5 | Blocklist dangerous flags | Security without breaking legitimate use cases |
| D6 | Structured test results with suggestions | Improve user experience for debugging |

## Next Steps

1. Create data-model.md with extended RelayProfile schema
2. Create OpenAPI contracts for new endpoints
3. Create quickstart.md for testing the feature
