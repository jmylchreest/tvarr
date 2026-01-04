---
title: Encoding Profiles
description: Configure video and audio encoding settings
sidebar_position: 1
---

# Encoding Profiles

Encoding profiles define how streams are transcoded.

## Creating a Profile

Go to **Admin > Encoding Profiles** and click **Add Profile**.

### Basic Settings

| Setting | Description |
|---------|-------------|
| Name | Profile identifier |
| Video Codec | Output video codec (h264, hevc, vp9, av1) |
| Audio Codec | Output audio codec (aac, opus, ac3) |
| Resolution | Output resolution (1920x1080, 1280x720, etc.) |

### Video Settings

| Setting | Description | Example |
|---------|-------------|---------|
| Bitrate | Target video bitrate | 4000k, 8000k |
| Max Bitrate | Maximum bitrate (VBR) | 6000k |
| CRF/CQ | Quality target (lower = better) | 23 |
| Preset | Encoding speed/quality tradeoff | medium, fast |
| Profile | H.264/HEVC profile | main, high |

### Audio Settings

| Setting | Description | Example |
|---------|-------------|---------|
| Bitrate | Audio bitrate | 128k, 192k |
| Sample Rate | Audio sample rate | 48000 |
| Channels | Channel count | 2 (stereo) |

## Common Profiles

### High Quality (1080p)

```
Video: h264 @ 8000kbps, 1920x1080
Audio: aac @ 192kbps, stereo
```

### Mobile (720p)

```
Video: h264 @ 2500kbps, 1280x720
Audio: aac @ 128kbps, stereo
```

### Low Bandwidth (480p)

```
Video: h264 @ 1000kbps, 854x480
Audio: aac @ 96kbps, stereo
```

### Passthrough

A "passthrough" profile copies streams without re-encoding:

```
Video: copy (no transcode)
Audio: copy (no transcode)
```

## Hardware Encoder Selection

When hardware acceleration is available, tvarr can use hardware encoders:

| Codec | Software | NVENC | VAAPI | QSV |
|-------|----------|-------|-------|-----|
| H.264 | libx264 | h264_nvenc | h264_vaapi | h264_qsv |
| HEVC | libx265 | hevc_nvenc | hevc_vaapi | hevc_qsv |
| VP9 | libvpx-vp9 | - | vp9_vaapi | vp9_qsv |
| AV1 | libaom-av1 | av1_nvenc | av1_vaapi | av1_qsv |

tvarr automatically selects the best available encoder based on detected hardware.

## Linking to Clients

Profiles are used by:

1. **Default** - Set a default profile for all streams
2. **Client Detection** - Assign profiles based on device type
3. **Manual** - Players can request specific profiles via query parameters
