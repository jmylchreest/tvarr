# Relay Streaming API

This document describes the relay streaming endpoints for accessing live streams with multi-format output support.

## Overview

The relay streaming system supports multiple output formats for maximum device compatibility:

| Format | MIME Type | Use Case |
|--------|-----------|----------|
| `mpegts` | `video/MP2T` | Universal compatibility, VLC, ffplay |
| `hls` | `application/vnd.apple.mpegurl` | iOS, Safari, Apple TV, smart TVs |
| `dash` | `application/dash+xml` | Cross-platform web players (Shaka, dash.js) |
| `auto` | varies | Automatic format detection based on client |

## Stream Endpoint

### GET /api/v1/relay/stream/{channel_id}

Streams a channel through the relay system with optional transcoding.

#### Path Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `channel_id` | ULID | Channel identifier |

#### Query Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `format` | string | `auto` | Output format: `mpegts`, `hls`, `dash`, `auto` |
| `seg` | uint64 | - | HLS/DASH segment number to retrieve |
| `init` | string | - | DASH initialization segment: `v` (video) or `a` (audio) |

#### Format Behavior

##### MPEG-TS (`format=mpegts`)

Returns a continuous MPEG-TS stream. This is the most compatible format.

```bash
# Example: Play with VLC
vlc "http://localhost:8080/api/v1/relay/stream/01ABC123DEF?format=mpegts"

# Example: Play with ffplay
ffplay "http://localhost:8080/api/v1/relay/stream/01ABC123DEF?format=mpegts"
```

**Response:**
- Content-Type: `video/MP2T`
- Transfer-Encoding: `chunked`
- Continuous stream until client disconnects

##### HLS (`format=hls`)

Returns HLS playlist (.m3u8) and supports segment requests.

**Playlist Request:**
```bash
# Get HLS playlist
curl "http://localhost:8080/api/v1/relay/stream/01ABC123DEF?format=hls"
```

**Response (playlist):**
```
#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:6
#EXT-X-MEDIA-SEQUENCE:42
#EXTINF:6.0,
/api/v1/relay/stream/01ABC123DEF?format=hls&seg=42
#EXTINF:6.0,
/api/v1/relay/stream/01ABC123DEF?format=hls&seg=43
...
```

**Segment Request:**
```bash
# Get specific segment
curl "http://localhost:8080/api/v1/relay/stream/01ABC123DEF?format=hls&seg=42"
```

**Response (segment):**
- Content-Type: `video/MP2T`
- Cache-Control: `max-age=86400`

##### DASH (`format=dash`)

Returns DASH manifest (.mpd) and supports segment/init requests.

**Manifest Request:**
```bash
# Get DASH manifest
curl "http://localhost:8080/api/v1/relay/stream/01ABC123DEF?format=dash"
```

**Response (manifest):**
- Content-Type: `application/dash+xml`
- XML MPD manifest with SegmentTemplate

**Initialization Segment Request:**
```bash
# Get video init segment
curl "http://localhost:8080/api/v1/relay/stream/01ABC123DEF?format=dash&init=v"

# Get audio init segment
curl "http://localhost:8080/api/v1/relay/stream/01ABC123DEF?format=dash&init=a"
```

**Response (init):**
- Content-Type: `video/mp4`
- Cache-Control: `max-age=86400`

**Media Segment Request:**
```bash
# Get media segment
curl "http://localhost:8080/api/v1/relay/stream/01ABC123DEF?format=dash&seg=42"
```

**Response (segment):**
- Content-Type: `video/iso.segment`
- Cache-Control: `max-age=86400`

##### Auto Detection (`format=auto` or omitted)

Automatically selects the best format based on client headers:

| Detection | Format | Trigger |
|-----------|--------|---------|
| Apple Device | HLS | User-Agent contains: iPhone, iPad, Safari, Apple TV, AppleCoreMedia |
| DASH Accept | DASH | Accept header contains: `application/dash+xml` |
| HLS Accept | HLS | Accept header contains: `application/vnd.apple.mpegurl` |
| DASH Player | DASH | User-Agent contains: shaka, dash |
| Default | MPEG-TS | No specific detection matches |

```bash
# Safari on macOS will receive HLS
curl -A "Mozilla/5.0 (Macintosh; Safari/605.1.15)" \
  "http://localhost:8080/api/v1/relay/stream/01ABC123DEF"

# Generic client will receive MPEG-TS
curl "http://localhost:8080/api/v1/relay/stream/01ABC123DEF"
```

## Passthrough Mode

When the source stream is already HLS or DASH, the relay operates in passthrough mode:

- **HLS Sources**: URLs with `.m3u8` extension are proxied with URL rewriting
- **DASH Sources**: URLs with `.mpd` extension are proxied with URL rewriting

Passthrough mode:
- Rewrites playlist/manifest URLs to route through the relay
- Caches segments to reduce upstream load
- Supports multiple clients with single upstream connection

## Codecs Endpoint

### GET /api/v1/relay/codecs

Returns available video and audio codecs, optionally filtered by output format.

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `format` | string | Filter codecs by output format: `mpegts`, `hls`, `dash` |

#### Response

```json
{
  "video_codecs": [
    {
      "value": "libx264",
      "label": "H.264 (Software)",
      "description": "Software H.264 encoding",
      "dash_only": false
    },
    {
      "value": "libvpx-vp9",
      "label": "VP9",
      "description": "VP9 software encoding",
      "dash_only": true
    }
  ],
  "audio_codecs": [
    {
      "value": "aac",
      "label": "AAC",
      "dash_only": false
    },
    {
      "value": "libopus",
      "label": "Opus",
      "dash_only": true
    }
  ],
  "format": "dash"
}
```

**DASH-Only Codecs:**

The following codecs require DASH output format (fMP4 containers):

| Video | Audio |
|-------|-------|
| VP9 (`libvpx-vp9`) | Opus (`libopus`) |
| AV1 (`libaom-av1`) | |
| AV1 NVENC (`av1_nvenc`) | |
| AV1 QSV (`av1_qsv`) | |

## Error Responses

### 404 Not Found

Returned when:
- Channel does not exist
- Segment has expired from buffer

```json
{
  "error": "segment not found"
}
```

### 400 Bad Request

Returned when:
- Invalid format parameter
- Invalid segment number

```json
{
  "error": "invalid format"
}
```

### 503 Service Unavailable

Returned when:
- Upstream source is unavailable
- Circuit breaker is open

```json
{
  "error": "upstream unavailable"
}
```

## Player Integration Examples

### Video.js with HLS

```javascript
const player = videojs('my-video', {
  sources: [{
    src: '/api/v1/relay/stream/01ABC123DEF?format=hls',
    type: 'application/x-mpegURL'
  }]
});
```

### Shaka Player with DASH

```javascript
const player = new shaka.Player(video);
player.load('/api/v1/relay/stream/01ABC123DEF?format=dash');
```

### Native HLS (Safari/iOS)

```html
<video controls>
  <source src="/api/v1/relay/stream/01ABC123DEF?format=hls" type="application/x-mpegURL">
</video>
```

### VLC Command Line

```bash
vlc "http://localhost:8080/api/v1/relay/stream/01ABC123DEF?format=mpegts"
```
