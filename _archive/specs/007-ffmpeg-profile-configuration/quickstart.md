# Quickstart: FFmpeg Profile Configuration

**Feature**: FFmpeg Profile Configuration
**Branch**: `007-ffmpeg-profile-configuration`
**Date**: 2025-12-06

This guide helps you test the FFmpeg Profile Configuration feature after implementation.

## Prerequisites

1. tvarr server running on `http://localhost:8080`
2. FFmpeg installed and accessible in PATH
3. A test stream URL (e.g., a public HLS stream)

## Quick Verification Steps

### 1. Check Hardware Capabilities

```bash
# Get detected hardware acceleration capabilities
curl -s http://localhost:8080/api/v1/hardware-capabilities | jq

# Expected output includes detected GPUs, encoders, decoders
```

### 2. List Existing Profiles

```bash
# List all relay profiles
curl -s http://localhost:8080/api/v1/relay-profiles | jq

# Should show system profiles with new fields:
# - input_options
# - output_options
# - hw_accel_output_format
# - success_count, failure_count
```

### 3. Create a Custom Profile with Custom Flags

```bash
# Create profile with custom FFmpeg flags
curl -X POST http://localhost:8080/api/v1/relay-profiles \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Custom Low Latency",
    "description": "Optimized for low latency streaming",
    "video_codec": "libx264",
    "audio_codec": "aac",
    "video_preset": "ultrafast",
    "video_bitrate": 4000,
    "audio_bitrate": 128,
    "input_options": "-fflags +genpts+discardcorrupt -probesize 10000000",
    "output_options": "-tune zerolatency -x264opts keyint=30:min-keyint=30"
  }' | jq
```

### 4. Validate Custom Flags

```bash
# Validate flags before saving
curl -X POST http://localhost:8080/api/v1/relay-profiles/validate-flags \
  -H "Content-Type: application/json" \
  -d '{
    "input_options": "-fflags +genpts -analyzeduration 10000000",
    "output_options": "-tune zerolatency"
  }' | jq

# Should return:
# {
#   "valid": true,
#   "flags": ["-fflags", "+genpts", "-analyzeduration", "10000000", "-tune", "zerolatency"],
#   "warnings": []
# }
```

### 5. Preview FFmpeg Command

```bash
# Get the FFmpeg command that would be generated
curl -s "http://localhost:8080/api/v1/relay-profiles/{profile_id}/preview?stream_url=http://example.com/stream.m3u8" | jq

# Returns the full FFmpeg command for debugging
```

### 6. Test Profile Against Stream

```bash
# Test profile against a real stream (5 second test)
curl -X POST http://localhost:8080/api/v1/relay-profiles/{profile_id}/test \
  -H "Content-Type: application/json" \
  -d '{
    "test_stream_url": "http://commondatastorage.googleapis.com/gtv-videos-bucket/sample/BigBuckBunny.mp4",
    "test_duration_seconds": 5
  }' | jq

# Expected output includes:
# - success: true/false
# - frames_processed
# - fps
# - hw_accel_active (if hardware acceleration was used)
# - suggestions (if errors occurred)
```

### 7. Clone a System Profile

```bash
# Clone the "Passthrough" system profile to customize it
curl -X POST http://localhost:8080/api/v1/relay-profiles/{system_profile_id}/clone \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Passthrough Custom"
  }' | jq
```

### 8. Create Hardware-Accelerated Profile (NVIDIA)

```bash
# Create NVIDIA hardware-accelerated profile
curl -X POST http://localhost:8080/api/v1/relay-profiles \
  -H "Content-Type: application/json" \
  -d '{
    "name": "NVIDIA H.264",
    "description": "NVIDIA GPU encoding with NVENC",
    "video_codec": "h264_nvenc",
    "audio_codec": "copy",
    "hw_accel": "cuda",
    "hw_accel_device": "0",
    "hw_accel_output_format": "cuda",
    "gpu_index": 0,
    "video_preset": "p4",
    "video_bitrate": 6000,
    "input_options": "-hwaccel_output_format cuda"
  }' | jq
```

### 9. View Profile Statistics

```bash
# After using a profile, check its statistics
curl -s http://localhost:8080/api/v1/relay-profiles/{profile_id} | jq '.success_count, .failure_count, .last_used_at'
```

## UI Testing

### Settings > Relay Profiles Page

1. Navigate to Settings > Relay Profiles
2. Verify system profiles show with lock icon (non-editable)
3. Click "Create Profile" - should show new fields:
   - Custom Input Flags
   - Custom Output Flags
   - Filter Complex
   - Hardware Acceleration Device
   - HW Accel Output Format
   - GPU Index

### Profile Form

1. Create a new profile with custom flags
2. Verify real-time validation warnings appear for invalid syntax
3. Click "Preview Command" to see generated FFmpeg command
4. Click "Copy Command" to copy for manual testing

### Profile Testing

1. Select a profile and click "Test Profile"
2. Enter a test stream URL
3. Verify test runs with progress indicator
4. Check results show:
   - Success/failure status
   - Frames processed
   - Hardware acceleration status
   - Error suggestions (if applicable)

### Profile Management

1. Clone a system profile
2. Verify cloned profile is editable
3. Verify system profiles can only toggle "Enabled"
4. Delete a custom profile
5. Verify delete is prevented if profile is assigned to a proxy

## Expected Behavior

### Custom Flags

| Scenario | Expected Result |
|----------|-----------------|
| Valid input flags `-fflags +genpts` | Accepted, no warnings |
| Shell injection attempt `;rm -rf` | Blocked with error |
| Unknown flag `--invalid` | Warning, but allowed |
| Balanced quotes `-filter "test"` | Accepted |
| Unbalanced quote `-filter "test` | Error |

### Hardware Acceleration

| Scenario | Expected Result |
|----------|-----------------|
| NVIDIA GPU available | hw_accel_active: true in test results |
| No GPU available | Graceful fallback to software |
| Invalid GPU index | Error with suggestion |

### Profile Testing

| Scenario | Expected Result |
|----------|-----------------|
| Valid stream URL | success: true, frames_processed > 0 |
| Invalid URL | success: false, error with suggestion |
| Timeout (>30s) | 408 Request Timeout |
| HW accel fails | success: false, suggestion to disable hw_accel |

## Troubleshooting

### FFmpeg Not Found

```bash
# Verify FFmpeg is installed
ffmpeg -version

# Check tvarr detected it
curl -s http://localhost:8080/health | jq '.components.relay_system.ffmpeg_available'
```

### Hardware Acceleration Not Detected

```bash
# Refresh hardware detection
curl -X POST http://localhost:8080/api/v1/hardware-capabilities/refresh | jq

# Check FFmpeg has hardware support
ffmpeg -hwaccels
```

### Profile Test Fails

1. Check the `ffmpeg_output` field in test result for raw errors
2. Check `suggestions` array for recommended fixes
3. Try the `command_executed` manually in terminal for debugging
