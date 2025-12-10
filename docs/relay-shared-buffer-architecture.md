# Relay Shared Buffer Architecture

This document describes the shared buffer architecture for tvarr's stream relay system, enabling a single origin connection to serve multiple output formats simultaneously.

## Problem Statement

Many upstream origins only allow a single concurrent connection per channel. The current architecture creates a separate origin connection for each client, which:
- Violates upstream connection limits
- Wastes bandwidth by fetching the same stream multiple times
- Makes it impossible to serve multiple output formats (HLS-TS, HLS-fMP4, DASH, raw TS, transcoded)

## Architecture Overview

```
                                            ┌─> HLS-TS Processor ───> Clients (HLS TS)
                                            │
Origin ─> Probe ─> Ingest ─> Shared Buffer ─┼─> HLS-fMP4 Processor ─> Clients (fMP4 HLS)
           │                                │
           └─ detect format                 ├─> DASH Processor ────> Clients (DASH)
              (HLS/DASH/RTSP/TS)            │
                                            ├─> TS Processor ──────> Clients (raw TS)
                                            │
                                            └─> FFmpeg Processor ──> Transcode Buffer ─> Clients
```

## Core Components

### 1. Ingest Layer

The ingest layer connects to the origin and demuxes the input to elementary streams.

**Responsibilities:**
- Probe origin URL to detect format (HLS, DASH, RTSP, raw MPEG-TS)
- Connect and maintain single origin connection
- For ABR sources: select highest quality variant (with profile-based limits in future)
- Demux container to raw elementary streams:
  - Video: H.264/H.265 NAL units
  - Audio: AAC/AC3/MP3 frames
- Write elementary stream data to shared buffer

**Supported Input Formats:**
- HLS (TS and fMP4 segments)
- DASH (fMP4 segments)
- RTSP (input only, never output)
- Raw MPEG-TS

**ABR Handling:**
- Collapse ABR manifests to single highest-quality variant
- Future: Profile-based quality limits (e.g., max 1080p for certain proxies)

### 2. Shared Buffer

A per-channel buffer storing elementary stream data. All processors read from this shared buffer.

**Structure:**
```go
type SharedBuffer struct {
    channelID     string
    proxyID       string

    // Elementary stream storage
    videoTrack    *ESTrack  // Video NAL units
    audioTrack    *ESTrack  // Audio frames

    // Timing
    baseTime      time.Time

    // Reference counting
    processors    map[string]Processor
    processorMu   sync.RWMutex
}

type ESTrack struct {
    codec        string     // h264, h265, aac, ac3, etc.
    samples      []ESSample // Ring buffer of samples
    sampleMu     sync.RWMutex

    // Codec-specific init data
    initData     []byte     // SPS/PPS for H.264, etc.
}

type ESSample struct {
    PTS       int64   // Presentation timestamp
    DTS       int64   // Decode timestamp (video only)
    Data      []byte  // Raw NAL unit or audio frame
    IsKeyframe bool   // For video: IDR frame
    Sequence  uint64  // Monotonic sequence number
}
```

**Lifecycle:**
- Created when first processor requests a channel
- Destroyed when all processors disconnect
- Maintains sliding window of samples (configurable retention)

### 3. Processors

Format-specific processors that read from the shared buffer and produce output for clients.

**Processor Types:**

| Processor | Input | Output | Notes |
|-----------|-------|--------|-------|
| HLS-TS | Elementary streams | HLS with MPEG-TS segments | Standard HLS |
| HLS-fMP4 | Elementary streams | HLS with fMP4/CMAF segments | Modern HLS |
| DASH | Elementary streams | DASH with fMP4 segments | MPEG-DASH |
| TS | Elementary streams | Raw MPEG-TS stream | Direct streaming |
| FFmpeg | Elementary streams | Via transcode buffer | Transcoding |

**Processor Responsibilities:**
- Mux elementary streams into target container format
- Generate manifests (HLS playlists, DASH MPD)
- Manage segment buffer for seeking
- Track connected clients

**Processor Sharing:**
- Clients requesting the same output format share the same processor
- Example: 5 clients requesting HLS-TS all connect to one HLS-TS processor

**Processor Lifecycle:**
- Created on-demand when client requests format
- Destroyed when all clients disconnect
- Automatic cleanup after idle timeout

### 4. FFmpeg Processor

Special processor for transcoding that interfaces with FFmpeg.

```
Shared Buffer ─> FFmpeg Processor ─> Pipe ─> FFmpeg ─> Transcode Buffer ─> Clients
```

**Design:**
- Reads elementary streams from shared buffer
- Muxes to MPEG-TS and pipes to FFmpeg stdin
- FFmpeg outputs to transcode-specific buffer
- Transcode buffer operates like shared buffer for transcoded clients

**Process Management:**
- FFmpeg runs as external process
- CPU/memory monitoring for stats
- Automatic restart on failure
- Graceful shutdown on last client disconnect

## Flow Visualization

The relay flow visualization will display:

```
┌─────────────┐         ┌────────────────┐         ┌────────────┐
│   Origin    │─────────│ Shared Buffer  │─────────│ Processor  │─────> Clients
│ (s8k:1080p) │  3.2MB/s│   [ES Data]    │  3.2MB/s│  HLS-TS    │
└─────────────┘         └────────────────┘         └────────────┘
                                │
                                │                  ┌────────────┐
                                └──────────────────│ Processor  │─────> Clients
                                           3.2MB/s │  HLS-fMP4  │
                                                   └────────────┘
                                │
                                │                  ┌────────────┐
                                └──────────────────│ FFmpeg     │
                                           3.2MB/s │ (720p-fast)│
                                                   └────────────┘
                                                         │
                                                   ┌─────┴─────┐
                                                   │ Transcode │
                                                   │  Buffer   │─────> Clients
                                                   │ [ES Data] │
                                                   └───────────┘
```

**Node Information:**

| Node Type | Displayed Info |
|-----------|----------------|
| Origin | Source name, format, resolution, codecs, ingress bandwidth |
| Shared Buffer | Sample counts, buffer utilization |
| Processor | Output format, codec info, egress bandwidth |
| FFmpeg Processor | Profile name, CPU%, memory, transcode speed |
| Client | Player type, remote IP, bytes served |

**Edge Information:**
- Bandwidth (bytes/second)
- Codec info (video/audio)
- Format (ts, fmp4, etc.)

## Format Selection

When a client requests a stream, format is determined by:

1. **Explicit format parameter**: `?format=hls-ts`, `?format=hls-fmp4`, `?format=dash`
2. **Profile-based**: Profile specifies output format
3. **Auto-detection**: "auto" profile examines input format

**Format Mapping:**

| Input | Default Output | Reason |
|-------|---------------|--------|
| HLS-TS | HLS-TS | Passthrough |
| HLS-fMP4 | HLS-fMP4 | Passthrough |
| DASH | DASH | Passthrough |
| MPEG-TS | HLS-TS | Repackage to segments |
| RTSP | HLS-TS | Requires repackaging |

## Data Flow

### Request Flow

1. Client requests `/channel/{id}/stream?format=hls-ts`
2. Relay manager looks up or creates shared buffer for channel
3. If no buffer exists:
   - Probe origin URL
   - Create ingest pipeline
   - Create shared buffer
4. Look up or create HLS-TS processor
5. Register client with processor
6. Serve HLS manifest

### Data Flow

1. Ingest reads from origin
2. Demux to elementary streams
3. Write samples to shared buffer (ring buffer)
4. Processors read samples (each maintains read pointer)
5. Processors mux to output format
6. Serve to clients

### Cleanup Flow

1. Client disconnects
2. Remove client from processor
3. If processor has no clients, mark for cleanup
4. After idle timeout, destroy processor
5. If no processors reference shared buffer, destroy it
6. Close origin connection

## Implementation Notes

### Concurrency

- Shared buffer uses read-write locks for sample access
- Processors maintain independent read positions
- No locking between processor reads (all read-only)

### Memory Management

- Elementary stream samples use ring buffer with configurable size
- Old samples evicted based on time, not count
- Keyframe retention ensures seeking works

### Error Handling

- Origin failures trigger fallback behavior
- Processor failures isolated from other processors
- Client errors don't affect other clients

### Metrics

Track per-channel:
- Ingress bandwidth
- Per-processor egress bandwidth
- Buffer utilization
- Sample counts
- Client counts

## Configuration

```yaml
relay:
  shared_buffer:
    sample_retention: 30s      # How long to keep samples
    keyframe_retention: 60s    # Minimum keyframe retention
    max_samples: 1000          # Max samples per track

  processor:
    idle_timeout: 10s          # Destroy after no clients
    segment_duration: 6s       # HLS/DASH segment duration
    segment_count: 4           # Segments in playlist

  ffmpeg:
    max_processes: 4           # Max concurrent transcodes
    startup_timeout: 10s       # Time to first output
```

## Future Enhancements

1. **Quality Limits**: Profile-based quality selection for ABR sources
2. **Subtitle Support**: Extract and serve subtitle tracks
3. **DVR**: Extended buffer for time-shift viewing
4. **Multi-audio**: Select audio track per client
5. **Adaptive Bitrate Output**: Generate ABR output from single input
