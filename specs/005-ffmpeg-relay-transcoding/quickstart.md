# Quickstart: FFmpeg Relay and Stream Transcoding Proxy

**Phase**: 1 - Design | **Date**: 2025-12-05 | **Spec**: [spec.md](spec.md)

## Overview

This guide explains how stream delivery modes work in tvarr and how to configure relay profiles for transcoding.

## Architecture Overview

**Mode selection happens at the StreamProxy level**, not the RelayProfile level:

- **StreamProxy** contains `proxy_mode` (redirect/proxy/relay)
- **RelayProfile** contains FFmpeg settings (codecs, bitrates, HW acceleration)
- RelayProfile is **only used** when `proxy_mode = relay`

```
StreamProxy
├── proxy_mode: redirect | proxy | relay
├── relay_profile_id: (optional, only for relay mode)
└── output_format: mpegts | hls (for proxy/relay modes)

RelayProfile (used only with relay mode)
├── video_codec: copy | libx264 | h264_nvenc | ...
├── audio_codec: copy | aac | ...
├── hw_accel: none | cuda | qsv | vaapi | ...
└── video_bitrate, audio_bitrate, etc.
```

## Prerequisites

- Tvarr server running
- FFmpeg installed (only required for relay mode)
- Stream sources configured with channels

## Stream Delivery Modes

### Redirect Mode (`redirect`)

The simplest mode. Tvarr returns an HTTP 302 redirect to the original stream URL.

**When to use:**
- Direct playback from source is acceptable
- No CORS issues (native apps, not browsers)
- Minimal server load required
- Credentials don't need to be hidden

**Trade-offs:**
- Zero server overhead
- Exposes source URL to client
- No stream monitoring or statistics
- Client handles connection directly

**No RelayProfile needed** - mode is set on the StreamProxy.

### Proxy Mode (`proxy`)

Tvarr fetches the stream, performs HLS collapse (variant selection), and repackages to the output format with CORS headers.

**When to use:**
- Browser-based playback (web players)
- Hide upstream credentials from clients
- HLS variant consolidation needed
- Source doesn't support CORS

**Capabilities:**
- HLS collapse (select best variant from adaptive streams)
- Container repackaging (no codec change)
- Output format: MPEG-TS (default) or HLS
- CORS headers for browser playback
- Client tracking and statistics

**Trade-offs:**
- Server bandwidth used (ingress + egress)
- No codec transcoding (stream copy only)
- Lower CPU usage than relay mode

**No RelayProfile needed** - mode is set on the StreamProxy.

### Relay Mode (`relay`)

Full FFmpeg processing with optional codec conversion. Requires a RelayProfile.

**When to use:**
- Need format conversion (e.g., HEVC to H.264)
- Hardware acceleration available
- Quality/bitrate adjustment needed
- Maximum control over output

**Capabilities:**
- Full codec transcoding
- Hardware acceleration (NVIDIA, Intel, AMD, Apple)
- Bitrate and resolution control
- Output format: MPEG-TS (default) or HLS

**Trade-offs:**
- Highest server resource usage
- Requires FFmpeg installation
- Full codec and quality control

**Requires RelayProfile** - associates a transcoding profile with the proxy.

## Configuration via UI

### Setting Proxy Mode

1. Navigate to **Admin > Proxies**
2. Create or edit a proxy
3. Select **Proxy Mode**:
   - Redirect: Direct 302 redirect
   - Proxy: Fetch and repackage
   - Relay: FFmpeg transcoding
4. If Relay mode, select a **Relay Profile**
5. Set **Output Format**: MPEG-TS or HLS

### Creating Relay Profiles

1. Navigate to **Admin > Relay Profiles**
2. Click **Create Profile**
3. Configure transcoding settings:
   - Video codec, bitrate, resolution
   - Audio codec, bitrate
   - Hardware acceleration
4. Save the profile

## Configuration via API

### Create a Proxy with Redirect Mode

```bash
curl -X POST http://localhost:8080/api/v1/proxies \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Direct Access",
    "proxy_mode": "redirect"
  }'
```

### Create a Proxy with Proxy Mode

```bash
curl -X POST http://localhost:8080/api/v1/proxies \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Browser-Compatible",
    "proxy_mode": "proxy",
    "output_format": "mpegts"
  }'
```

### Create a Relay Profile

```bash
curl -X POST http://localhost:8080/api/v1/relay/profiles \
  -H "Content-Type: application/json" \
  -d '{
    "name": "H.264 720p",
    "description": "Software encode to 720p @ 3Mbps",
    "video_codec": "libx264",
    "video_bitrate": 3000,
    "video_width": 1280,
    "video_height": 720,
    "audio_codec": "aac",
    "audio_bitrate": 128
  }'
```

### Create a Proxy with Relay Mode

```bash
# Get the relay profile ID
PROFILE_ID=$(curl -s http://localhost:8080/api/v1/relay/profiles | jq -r '.profiles[0].id')

# Create proxy using the profile
curl -X POST http://localhost:8080/api/v1/proxies \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"Transcoded Stream\",
    \"proxy_mode\": \"relay\",
    \"relay_profile_id\": \"$PROFILE_ID\",
    \"output_format\": \"mpegts\"
  }"
```

### Create a Hardware-Accelerated Relay Profile

```bash
curl -X POST http://localhost:8080/api/v1/relay/profiles \
  -H "Content-Type: application/json" \
  -d '{
    "name": "NVENC 1080p",
    "description": "NVIDIA GPU transcoding to H.264 1080p",
    "hw_accel": "cuda",
    "video_codec": "h264_nvenc",
    "video_bitrate": 6000,
    "video_width": 1920,
    "video_height": 1080,
    "audio_codec": "aac",
    "audio_bitrate": 192
  }'
```

### Create a Passthrough Relay Profile

Use codec copy for passthrough (no transcoding, but FFmpeg handles the stream):

```bash
curl -X POST http://localhost:8080/api/v1/relay/profiles \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Passthrough",
    "description": "Copy codecs through FFmpeg pipeline",
    "video_codec": "copy",
    "audio_codec": "copy"
  }'
```

## Streaming Channels

### Stream URL Format

```
http://localhost:8080/proxy/{proxy_id}/{channel_id}.ts
http://localhost:8080/proxy/{proxy_id}/{channel_id}.m3u8
```

The stream behavior is determined by the proxy's `proxy_mode`:
- redirect: Returns 302 to source
- proxy: Fetches and repackages with CORS
- relay: FFmpeg transcoding via associated profile

### Example: Stream in VLC

```bash
# Get proxy and channel IDs
PROXY_ID=$(curl -s http://localhost:8080/api/v1/proxies | jq -r '.proxies[0].id')
CHANNEL_ID=$(curl -s http://localhost:8080/api/v1/channels | jq -r '.channels[0].id')

# Open in VLC
vlc "http://localhost:8080/proxy/$PROXY_ID/$CHANNEL_ID.ts"
```

## Output Formats

Output format is **client-requested**, not stored on the proxy. Clients specify the desired format via:

1. **URL extension**: `.ts` for MPEG-TS, `.m3u8` for HLS
2. **Query parameter**: `?format=ts` or `?format=hls`

| Format | Extension | Query | Use Case |
|--------|-----------|-------|----------|
| MPEG-TS | `.ts` | `?format=ts` | Native players (VLC, ffplay), STB |
| HLS | `.m3u8` | `?format=hls` | Browser players (HLS.js), mobile apps |

**Default**: MPEG-TS

### Examples

```bash
# MPEG-TS via extension
http://localhost:8080/proxy/$PROXY_ID/$CHANNEL_ID.ts

# HLS via extension
http://localhost:8080/proxy/$PROXY_ID/$CHANNEL_ID.m3u8

# MPEG-TS via query param
http://localhost:8080/proxy/$PROXY_ID/$CHANNEL_ID?format=ts

# HLS via query param
http://localhost:8080/proxy/$PROXY_ID/$CHANNEL_ID?format=hls
```

## Response Headers

Stream responses include `X-Stream-*` headers providing visibility into what processing was applied:

| Header | Description | Example Values |
|--------|-------------|----------------|
| `X-Stream-Origin-Kind` | Detected source type | `RAW_TS`, `HLS_MASTER`, `HLS_MEDIA`, `UNKNOWN` |
| `X-Stream-Decision` | Processing decision | `passthrough`, `collapsed`, `transcoded` |
| `X-Stream-Mode` | Operational mode | `passthrough-raw-ts`, `hls-to-ts`, `relay-transcode` |
| `X-Stream-Variant-Count` | HLS variants detected | `3` (for master playlists) |
| `X-Stream-Target-Duration` | HLS segment duration | `6.0` (seconds) |
| `X-Stream-Fallback` | Reason if mode not honored | `source not raw TS, using collapsed mode` |

### Example Response

```http
HTTP/1.1 200 OK
Content-Type: video/mp2t
X-Stream-Origin-Kind: HLS_MASTER
X-Stream-Decision: collapsed
X-Stream-Mode: hls-to-ts
X-Stream-Variant-Count: 3
Access-Control-Allow-Origin: *
Cache-Control: no-cache, no-store
```

These headers help clients and debugging tools understand:
- What type of source was detected
- What processing was applied
- Why a particular mode was chosen

## Monitoring

### List Active Relay Sessions

```bash
curl http://localhost:8080/api/v1/relay/sessions
```

### Get Session Clients

```bash
curl http://localhost:8080/api/v1/relay/sessions/{session_id}/clients
```

### Get Relay Statistics

```bash
curl http://localhost:8080/api/v1/relay/stats
```

### Get Relay Health

```bash
curl http://localhost:8080/api/v1/relay/health
```

## System Relay Profiles

Tvarr includes pre-configured system profiles for relay mode:

| Profile | Video Codec | Audio Codec | Description |
|---------|-------------|-------------|-------------|
| Passthrough | copy | copy | No transcoding |
| H.264 720p | libx264 | aac | Software encode @ 3Mbps |
| H.264 1080p | libx264 | aac | Software encode @ 6Mbps |

## Error Fallback

When a relay session encounters errors (upstream failure, FFmpeg crash), Tvarr can serve a fallback stream:

1. **Error Detection**: Circuit breaker tracks consecutive failures
2. **Threshold Reached**: After 3 consecutive errors (configurable)
3. **Fallback Stream**: Pre-generated TS segment with "Stream Unavailable" message
4. **Recovery Check**: Periodically attempts to reconnect (default: 30 seconds)
5. **Auto-Recovery**: Resumes normal streaming when upstream recovers

Configure fallback behavior per relay profile:

```bash
curl -X PATCH http://localhost:8080/api/v1/relay/profiles/{id} \
  -H "Content-Type: application/json" \
  -d '{
    "fallback_enabled": true,
    "fallback_error_threshold": 3,
    "fallback_recovery_interval": 30
  }'
```

## Browser Playback (CORS)

For web-based players (HLS.js, Video.js), use proxy or relay mode:

```html
<video id="player" controls></video>
<script src="https://cdn.jsdelivr.net/npm/hls.js@latest"></script>
<script>
  const video = document.getElementById('player');
  const streamUrl = 'http://localhost:8080/proxy/{proxy_id}/{channel_id}.m3u8';

  if (Hls.isSupported()) {
    const hls = new Hls();
    hls.loadSource(streamUrl);
    hls.attachMedia(video);
  } else if (video.canPlayType('application/vnd.apple.mpegurl')) {
    video.src = streamUrl;
  }
</script>
```

## Troubleshooting

### "FFmpeg not found" Error

Relay mode requires FFmpeg in PATH:
```bash
# Check FFmpeg installation
ffmpeg -version

# If not installed (Ubuntu/Debian)
sudo apt install ffmpeg

# If not installed (macOS)
brew install ffmpeg
```

### "CORS blocked" in Browser

Ensure you're using proxy or relay mode, not redirect:
```bash
# Check proxy mode
curl http://localhost:8080/api/v1/proxies/{id} | jq '.proxy_mode'
```

### High CPU Usage

Consider hardware acceleration (relay mode only):
```bash
# Check available hardware encoders
ffmpeg -encoders | grep -E 'nvenc|qsv|vaapi|amf'

# Update relay profile to use hardware encoding
curl -X PATCH http://localhost:8080/api/v1/relay/profiles/{id} \
  -d '{"hw_accel": "cuda", "video_codec": "h264_nvenc"}'
```

### Session Not Starting

Check relay health and logs:
```bash
# Check health endpoint
curl http://localhost:8080/api/v1/relay/health

# Check server logs for errors
journalctl -u tvarr -f
```
