# Relay Shared Buffer Architecture

This document describes the shared buffer architecture for tvarr's stream relay system, enabling a single origin connection to serve multiple output formats and codec variants simultaneously.

## Problem Statement

Many upstream origins only allow a single concurrent connection per channel. Without a shared architecture:
- Each client would require a separate origin connection (violating upstream limits)
- Bandwidth is wasted fetching the same stream multiple times
- Supporting multiple output formats (HLS-TS, HLS-fMP4, DASH, raw TS) would multiply connections
- Transcoding to different codecs would require separate pipelines

## Architecture Overview

```
                                                    ┌─> HLS-TS Processor ────> Clients
                                                    │
                        ┌─────────────────────────┐ ├─> HLS-fMP4 Processor ──> Clients
                        │   SharedESBuffer        │ │
Origin ─> Ingest ─────> │                         │─┼─> DASH Processor ──────> Clients
  │       (Demuxer)     │  ┌─────────────────┐    │ │
  │                     │  │ Source Variant  │    │ └─> MPEG-TS Processor ──> Clients
  └─ detect format      │  │ (h264/aac)      │    │
     (HLS/DASH/TS)      │  │  - videoTrack   │    │
                        │  │  - audioTrack   │    │
                        │  └─────────────────┘    │
                        │           │             │
                        │           │ (on-demand) │
                        │           ▼             │
                        │  ┌─────────────────┐    │
                        │  │ FFmpeg          │    │     ┌─> HLS-TS Processor ──> Clients
                        │  │ Transcoder      │────│─────┤
                        │  └─────────────────┘    │     └─> DASH Processor ────> Clients
                        │           │             │
                        │           ▼             │
                        │  ┌─────────────────┐    │
                        │  │ Target Variant  │    │
                        │  │ (vp9/opus)      │    │
                        │  │  - videoTrack   │    │
                        │  │  - audioTrack   │    │
                        │  └─────────────────┘    │
                        └─────────────────────────┘
```

### Key Concepts

1. **Single Origin Connection**: One ingest pipeline per channel, regardless of client count
2. **Multi-Variant Storage**: SharedESBuffer stores multiple codec variants (original + transcoded)
3. **On-Demand Transcoding**: FFmpeg transcoders spawn only when a variant is requested
4. **Format-Agnostic ES Storage**: Elementary streams stored codec-agnostically; processors mux to output formats
5. **Lifecycle Management**: Everything tears down when no clients remain

## Core Components

### 1. Ingest Layer

The ingest layer connects to the origin and demuxes input to elementary streams.

**Responsibilities:**
- Probe origin URL to detect format (HLS, DASH, RTSP, raw MPEG-TS)
- Maintain single origin connection per channel
- For ABR sources: select highest quality variant (profile-based limits in future)
- Demux container to raw elementary streams:
  - Video: H.264/H.265 NAL units with timing (PTS/DTS)
  - Audio: AAC/AC3/MP3 frames with timing
- Write elementary stream data to source variant in SharedESBuffer

**Supported Input Formats:**
| Format | Demuxer | Notes |
|--------|---------|-------|
| HLS (TS segments) | HLSDemuxer → TSDemuxer | Fetches segments, demuxes MPEG-TS |
| HLS (fMP4 segments) | HLSDemuxer → FMP4Demuxer | Fetches segments, demuxes fMP4 |
| DASH | DASHDemuxer → FMP4Demuxer | Fetches segments, demuxes fMP4 |
| Raw MPEG-TS | TSDemuxer | Direct stream demuxing |
| RTSP | RTSPDemuxer | Input only, never output |

**ABR Handling:**
- Collapse ABR manifests to single highest-quality variant
- Future: Profile-based quality limits (e.g., max 1080p for certain relays)

### 2. SharedESBuffer (Multi-Variant Elementary Stream Buffer)

A per-channel buffer storing elementary stream data with support for multiple codec variants.

**Structure:**
```go
type SharedESBuffer struct {
    channelID     string
    proxyID       string

    // Multi-variant storage: map from codec variant to ES tracks
    variants      map[CodecVariant]*ESVariant
    sourceVariant CodecVariant  // The original source codec (e.g., "h264/aac")
    variantsMu    sync.RWMutex

    // Timing
    startTime     time.Time

    // Processor tracking for lifecycle management
    processors    map[string]struct{}
    processorsMu  sync.RWMutex

    // Callback for on-demand transcoding
    onVariantRequest func(source, target CodecVariant) error

    // Lifecycle
    closed        atomic.Bool
    closedCh      chan struct{}
}

type CodecVariant string  // Format: "video/audio", e.g., "h264/aac", "vp9/opus"

type ESVariant struct {
    variant     CodecVariant
    videoTrack  *ESTrack
    audioTrack  *ESTrack
    isSource    bool       // True if this is the original (non-transcoded) variant
    createdAt   time.Time
    lastAccess  atomic.Value  // For cleanup of unused transcoded variants
}

type ESTrack struct {
    codec     string      // h264, h265, aac, ac3, vp9, opus, etc.
    initData  []byte      // SPS/PPS for H.264, AudioSpecificConfig for AAC, etc.
    samples   []ESSample  // Ring buffer of samples
    capacity  int         // Max samples
    head, tail, count int // Ring buffer pointers
    lastSeq   uint64      // Last sequence number assigned
    notify    chan struct{}  // Notification for new samples
    mu        sync.RWMutex
}

type ESSample struct {
    PTS        int64     // Presentation timestamp (90kHz timescale)
    DTS        int64     // Decode timestamp (video only)
    Data       []byte    // Raw NAL unit or audio frame
    IsKeyframe bool      // For video: IDR frame
    Sequence   uint64    // Monotonic sequence number for ordering
    Timestamp  time.Time // Wall clock time when received
}
```

**Common Codec Variants:**
| Variant | Video | Audio | Use Case |
|---------|-------|-------|----------|
| `h264/aac` | H.264/AVC | AAC | Most common, wide compatibility |
| `h264/ac3` | H.264/AVC | Dolby AC-3 | Surround sound sources |
| `hevc/aac` | H.265/HEVC | AAC | 4K/HDR content |
| `vp9/opus` | VP9 | Opus | WebM/modern browsers |
| `av1/opus` | AV1 | Opus | Next-gen codec |
| `copy/copy` | (passthrough) | (passthrough) | Use source codecs |

**Lifecycle:**
- Created when first client requests a channel
- Source variant created when ingest detects stream codecs
- Transcoded variants created on-demand when processors request them
- Destroyed when all processors disconnect (idle timeout)
- Unused transcoded variants cleaned up after configurable idle period

### 3. Processors

Processors read from a specific variant in SharedESBuffer and produce output in a specific container format. Each processor serves one or more clients.

**Processor Types:**

| Processor | Input | Output | Container |
|-----------|-------|--------|-----------|
| HLSTSProcessor | ES variant | HLS playlist + MPEG-TS segments | MPEG-TS |
| HLSfMP4Processor | ES variant | HLS playlist + fMP4/CMAF segments | fMP4 |
| DASHProcessor | ES variant | DASH MPD + fMP4 segments | fMP4 |
| MPEGTSProcessor | ES variant | Continuous MPEG-TS stream | MPEG-TS |

**Processor Configuration:**
- Each processor is created with a target `CodecVariant`
- On start, processor calls `esBuffer.GetOrCreateVariant(variant)`
- If variant doesn't exist and differs from source, `onVariantRequest` callback triggers transcoding
- Processor then reads from its assigned variant's tracks

**Processor Responsibilities:**
- Read elementary samples from variant's video/audio tracks
- Mux samples into target container format
- Generate manifests (HLS playlists, DASH MPD) with proper timing
- Manage segment buffer for client seeking
- Track connected clients and serve requests

**Processor Sharing Options:**
- **Per-client processors**: Simplest model, each client gets dedicated processor
  - Pro: Simple lifecycle, easy debugging/tracing
  - Con: More memory for segment buffers
- **Shared processors**: Multiple clients share one processor per format+variant
  - Pro: Efficient memory usage
  - Con: More complex client management

Current implementation: One processor per format per session (can serve multiple clients).

**Processor Lifecycle:**
- Created on-demand when client requests format/variant combination
- Destroyed when all clients disconnect
- Automatic cleanup after idle timeout

### 4. FFmpeg Transcoder

On-demand transcoder that reads from source variant and writes to a target variant in the same SharedESBuffer.

```
┌─────────────────────────────────────────────────────────────────┐
│                      SharedESBuffer                              │
│                                                                  │
│  ┌──────────────┐         ┌──────────────────┐                  │
│  │Source Variant│         │ FFmpegTranscoder │                  │
│  │ (h264/aac)   │────────>│                  │                  │
│  │  videoTrack ─┼─read──> │  1. Read ES      │                  │
│  │  audioTrack ─┼─read──> │  2. Mux to TS    │                  │
│  └──────────────┘         │  3. Pipe to FFmpeg│                  │
│                           │  4. Demux output  │                  │
│                           │  5. Write ES      │                  │
│  ┌──────────────┐         │                  │                  │
│  │Target Variant│<────────┤                  │                  │
│  │ (vp9/opus)   │         └──────────────────┘                  │
│  │  videoTrack <┼─write───                                      │
│  │  audioTrack <┼─write───                                      │
│  └──────────────┘                                                │
└─────────────────────────────────────────────────────────────────┘
```

**Design:**
1. Reads elementary streams from source variant
2. Muxes to MPEG-TS and pipes to FFmpeg stdin
3. FFmpeg transcodes and outputs MPEG-TS to stdout
4. TSDemuxer parses FFmpeg output
5. Writes demuxed samples to target variant in same SharedESBuffer

**Key Points:**
- Transcoded data stays in SharedESBuffer (no separate "transcode buffer")
- Multiple processors can read from the same transcoded variant
- One FFmpeg process per unique transcode (source→target variant pair)

**Process Management:**
- FFmpeg runs as external process with monitored stdin/stdout pipes
- CPU/memory monitoring for stats and resource limits
- Automatic restart on failure (with backoff)
- Graceful shutdown when no processors need the variant

**Transcoder Lifecycle:**
- Created when `onVariantRequest(source, target)` callback is invoked
- Runs until context cancelled or no processors need target variant
- Should be cleaned up when target variant has no recent access (TODO: implement cleanup loop)

## Format and Codec Selection

### Format Selection (Container)

When a client requests a stream, the output format (container) is determined by:

1. **Explicit format parameter**: `?format=hls-ts`, `?format=hls-fmp4`, `?format=dash`, `?format=ts`
2. **Profile setting**: Profile can specify preferred output format
3. **Client detection**: "automatic" profile uses client detection rules (User-Agent analysis)
4. **Default**: Falls back to HLS-TS for maximum compatibility

### Codec Selection (Variant)

The codec variant is determined by:

1. **Profile setting**: Profile specifies target video/audio codecs
2. **Automatic mode**: When profile is "automatic" or unspecified:
   - Use client detection rules to determine optimal codecs
   - Fall back to source codecs (passthrough) if detection is inconclusive
3. **Passthrough**: `copy/copy` variant means use whatever the source provides

**Client Detection Example:**
| Client | Detected Format | Detected Codecs |
|--------|-----------------|-----------------|
| Safari/iOS | HLS-TS | h264/aac |
| Chrome | HLS-fMP4 or DASH | h264/aac or vp9/opus |
| VLC | MPEG-TS | h264/aac |
| Smart TV | HLS-TS | h264/aac |

## Data Flow

### Complete Request Flow

1. **Client Request**: Client requests `/api/v1/relay/channel/{id}/stream?format=hls-ts`

2. **Session Lookup/Creation**:
   - Manager checks for existing session for channel
   - If none exists, creates new session with:
     - Stream classification (detect HLS/DASH/TS source)
     - Profile resolution (explicit or automatic)
     - Codec probing (optional, for faster startup)

3. **Pipeline Startup** (for ES-based pipeline):
   - Create SharedESBuffer for channel
   - Set variant request callback for on-demand transcoding
   - Start appropriate demuxer based on source format:
     - HLS source → HLSDemuxer (fetches segments) → TSDemuxer
     - DASH source → DASHDemuxer → FMP4Demuxer
     - Raw TS → Direct TSDemuxer
   - Source variant created when demuxer detects codecs

4. **Processor Creation**:
   - Determine target variant from profile
   - Create processor(s) for requested format(s)
   - Each processor calls `GetOrCreateVariant(targetVariant)`
   - If target ≠ source, `onVariantRequest` spawns FFmpegTranscoder

5. **Streaming**:
   - Demuxer writes samples to source variant
   - Transcoder (if active) reads source, writes to target variant
   - Processors read from their variant, mux to output format
   - Clients receive segments/stream data

### Data Flow Diagram

```
┌──────────┐    ┌─────────┐    ┌─────────────────────────────────────────┐
│  Origin  │───>│ Demuxer │───>│            SharedESBuffer               │
│  (HLS)   │    │(HLS+TS) │    │                                         │
└──────────┘    └─────────┘    │  ┌─────────────┐                        │
                               │  │   h264/aac  │ (source)               │
                               │  │  video:[...]│───────┬────────────────┼──> HLS-TS Proc ──> Client A
                               │  │  audio:[...]│       │                │
                               │  └─────────────┘       │                │
                               │         │              │                │
                               │         │ FFmpeg       │                │
                               │         ▼              │                │
                               │  ┌─────────────┐       │                │
                               │  │   vp9/opus  │ (transcoded)           │
                               │  │  video:[...]│───────┴────────────────┼──> DASH Proc ────> Client B
                               │  │  audio:[...]│                        │
                               │  └─────────────┘                        │
                               └─────────────────────────────────────────┘
```

### Cleanup Flow

1. Client disconnects
2. Remove client from processor
3. If processor has no clients:
   - Mark processor for cleanup
   - After idle timeout, destroy processor
   - Unregister processor from SharedESBuffer
4. If no processors reference a transcoded variant:
   - Stop associated FFmpegTranscoder
   - After idle timeout, remove variant from buffer
5. If no processors reference SharedESBuffer:
   - Close ingest/demuxer
   - Close origin connection
   - Destroy SharedESBuffer

## Implementation Notes

### Concurrency

- SharedESBuffer uses RWMutex for variant map access
- ESTrack uses RWMutex for sample ring buffer access
- Processors maintain independent read positions (sequence numbers)
- Multiple readers can read simultaneously (read-only access)
- Writers (demuxer, transcoder) acquire write locks briefly

### Memory Management

- Ring buffers with configurable capacity per track
- Samples evicted when buffer is full (oldest first)
- Keyframe retention ensures new clients can start playback
- Transcoded variants cleaned up after idle period

### Error Handling

- Origin failures can trigger fallback slate (if configured)
- Transcoder failures: log error, variant remains empty until retry
- Processor failures isolated from other processors
- Client errors don't affect other clients or the pipeline

### Metrics (per channel)

- Ingress bandwidth (from origin)
- Per-variant sample counts and byte counts
- Per-processor egress bandwidth
- Buffer utilization per variant
- Transcoder CPU/memory usage
- Client counts per processor

## Configuration

```yaml
relay:
  shared_buffer:
    video_capacity: 1000       # Max video samples per variant
    audio_capacity: 2000       # Max audio samples per variant
    variant_idle_timeout: 60s  # Remove unused transcoded variants after this

  processor:
    idle_timeout: 10s          # Destroy processor after no clients
    segment_duration: 6s       # HLS/DASH segment duration
    max_segments: 7            # Segments in playlist sliding window

  transcoder:
    max_concurrent: 4          # Max concurrent FFmpeg processes
    startup_timeout: 10s       # Time to first output before considering failed
    idle_timeout: 30s          # Stop transcoder if variant unused
```

## Known Issues / TODO

1. **HLS source + transcoding**: The ES pipeline currently assumes raw MPEG-TS input. When an HLS source requires transcoding, it should use HLSDemuxer to fetch segments, not direct HTTP GET.

2. **Variant cleanup loop**: `CleanupUnusedVariants()` exists but needs to be called periodically to remove stale transcoded variants and stop their associated transcoders.

3. **Transcoder lifecycle**: Need to implement proper monitoring of variant access to know when to stop transcoders.

4. **Init data propagation**: Ensure SPS/PPS (H.264) and AudioSpecificConfig (AAC) are properly extracted and stored in ESTrack.initData for correct muxing.

## Future Enhancements

1. **Quality Limits**: Profile-based quality selection for ABR sources (e.g., max 720p)
2. **Subtitle Support**: Extract and serve subtitle tracks as separate variant
3. **DVR Mode**: Extended buffer for time-shift viewing
4. **Multi-audio**: Support multiple audio tracks, selectable per client
5. **ABR Output**: Generate adaptive bitrate output from single input (multiple quality variants)
6. **Hardware Transcoding**: Better support for NVENC, QSV, VAAPI acceleration