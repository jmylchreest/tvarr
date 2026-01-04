# Quickstart: Multi-Format Streaming Support

**Feature**: 008-multi-format-streaming
**Date**: 2025-12-07

## Overview

This feature adds HLS and DASH output format support to relay mode, with query-parameter-driven format selection. Clients can request streams in their preferred format without URL changes.

## Key Concepts

### Format Selection

Clients select output format using the `?format=` query parameter:

| Format | Content-Type | Use Case |
|--------|--------------|----------|
| `mpegts` | `video/MP2T` | Default, continuous stream |
| `hls` | `application/vnd.apple.mpegurl` | Apple devices, Safari |
| `dash` | `application/dash+xml` | Android, Chromecast, web |
| `auto` | Varies | Auto-detect by User-Agent |

### Container-Aware Codecs

Codec availability depends on output format:

| Codec | MPEG-TS | HLS | DASH |
|-------|---------|-----|------|
| H.264 | Yes | Yes | Yes |
| H.265 | Yes | Yes | Yes |
| VP9 | No | No | **Yes** |
| AV1 | No | No | **Yes** |
| AAC | Yes | Yes | Yes |
| Opus | No | No | **Yes** |

## Usage Examples

### Request HLS Stream

```bash
# Get HLS playlist
curl "http://localhost:8080/api/v1/proxy/{proxyId}/{channelId}?format=hls"

# Response: application/vnd.apple.mpegurl
#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXT-X-MEDIA-SEQUENCE:42
#EXTINF:6.006,
/api/v1/proxy/{proxyId}/{channelId}?format=hls&seg=42
#EXTINF:6.006,
/api/v1/proxy/{proxyId}/{channelId}?format=hls&seg=43
```

```bash
# Get HLS segment
curl "http://localhost:8080/api/v1/proxy/{proxyId}/{channelId}?format=hls&seg=42" > segment.ts
```

### Request DASH Stream

```bash
# Get DASH manifest
curl "http://localhost:8080/api/v1/proxy/{proxyId}/{channelId}?format=dash"

# Response: application/dash+xml
<?xml version="1.0" encoding="UTF-8"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" ...>
  <Period>
    <AdaptationSet mimeType="video/mp4" ...>
      <SegmentTemplate initialization="?format=dash&init=v"
                       media="?format=dash&seg=v$Number$" />
    </AdaptationSet>
  </Period>
</MPD>
```

```bash
# Get DASH init segment
curl "http://localhost:8080/api/v1/proxy/{proxyId}/{channelId}?format=dash&init=v" > init.mp4

# Get DASH media segment
curl "http://localhost:8080/api/v1/proxy/{proxyId}/{channelId}?format=dash&seg=v42" > segment.m4s
```

### Request MPEG-TS Stream (Default)

```bash
# Continuous MPEG-TS stream (existing behavior)
curl "http://localhost:8080/api/v1/proxy/{proxyId}/{channelId}" | ffplay -
# Or explicitly:
curl "http://localhost:8080/api/v1/proxy/{proxyId}/{channelId}?format=mpegts" | ffplay -
```

### Auto-Format Detection

```bash
# Let server detect optimal format
# Safari: Returns HLS
# Chrome: Returns MPEG-TS
# dash.js: Returns DASH (if Accept header set)
curl "http://localhost:8080/api/v1/proxy/{proxyId}/{channelId}?format=auto"
```

## Configuration

### Relay Profile Settings

```json
{
  "name": "HLS Profile",
  "output_format": "hls",
  "video_codec": "libx264",
  "audio_codec": "aac",
  "segment_duration": 6,
  "playlist_size": 5
}
```

```json
{
  "name": "DASH VP9 Profile",
  "output_format": "dash",
  "video_codec": "libvpx-vp9",
  "audio_codec": "libopus",
  "segment_duration": 6,
  "playlist_size": 5
}
```

### Get Available Codecs by Format

```bash
# Get codecs for HLS
curl "http://localhost:8080/api/v1/relay/codecs?format=hls"

# Response
{
  "video_codecs": [
    {"value": "copy", "label": "Copy (Passthrough)"},
    {"value": "libx264", "label": "H.264 (Software)"},
    {"value": "h264_nvenc", "label": "H.264 (NVIDIA)", "requires_hw": true}
  ],
  "audio_codecs": [
    {"value": "copy", "label": "Copy (Passthrough)"},
    {"value": "aac", "label": "AAC"}
  ]
}

# Get codecs for DASH (includes VP9, AV1, Opus)
curl "http://localhost:8080/api/v1/relay/codecs?format=dash"

# Response includes additional codecs
{
  "video_codecs": [
    {"value": "libvpx-vp9", "label": "VP9", "dash_only": true},
    {"value": "libaom-av1", "label": "AV1", "dash_only": true}
  ],
  "audio_codecs": [
    {"value": "libopus", "label": "Opus", "dash_only": true}
  ]
}
```

## Client Integration

### VLC

```bash
# HLS
vlc "http://localhost:8080/api/v1/proxy/{proxyId}/{channelId}?format=hls"

# DASH
vlc "http://localhost:8080/api/v1/proxy/{proxyId}/{channelId}?format=dash"
```

### FFplay

```bash
# HLS
ffplay "http://localhost:8080/api/v1/proxy/{proxyId}/{channelId}?format=hls"

# DASH
ffplay "http://localhost:8080/api/v1/proxy/{proxyId}/{channelId}?format=dash"
```

### hls.js (Web Browser)

```javascript
const video = document.getElementById('video');
const hls = new Hls();
hls.loadSource('/api/v1/proxy/{proxyId}/{channelId}?format=hls');
hls.attachMedia(video);
```

### dash.js (Web Browser)

```javascript
const video = document.getElementById('video');
const player = dashjs.MediaPlayer().create();
player.initialize(video, '/api/v1/proxy/{proxyId}/{channelId}?format=dash', true);
```

## Testing

### Verify HLS Playlist

```bash
curl -I "http://localhost:8080/api/v1/proxy/{proxyId}/{channelId}?format=hls"
# Content-Type: application/vnd.apple.mpegurl
```

### Verify Segment Delivery

```bash
# Get playlist, extract segment URLs, fetch segment
curl -s "http://localhost:8080/api/v1/proxy/{proxyId}/{channelId}?format=hls" | \
  grep "seg=" | head -1 | xargs curl -I
# Content-Type: video/MP2T
```

### Check Session Statistics

```bash
curl "http://localhost:8080/api/v1/relay/sessions/{sessionId}/stats"
# Returns segment buffer state, client count, etc.
```

## Common Issues

### VP9/AV1 Codec with HLS

**Error**: `VP9 requires DASH output format`

**Solution**: Change output_format to "dash" or use H.264/H.265 codec

### Segment Not Found (404)

**Error**: Segment expired from buffer

**Solution**: Request more recent segment sequence, or increase playlist_size

### Format Auto-Detection Not Working

**Cause**: User-Agent not recognized

**Solution**: Explicitly specify format parameter or configure proxy default format
