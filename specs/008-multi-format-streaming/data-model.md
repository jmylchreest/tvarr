# Data Model: Multi-Format Streaming Support

**Feature**: 008-multi-format-streaming
**Date**: 2025-12-07

## Entity Overview

```
┌─────────────────────┐       ┌──────────────────────┐
│   RelayProfile      │       │   StreamSession      │
│   (existing)        │──────▶│   (existing)         │
│   + OutputFormat    │       │   + formatRouter     │
│   + codec validation│       │   + segmentBuffer    │
└─────────────────────┘       └──────────┬───────────┘
                                         │
                              ┌──────────▼───────────┐
                              │   SegmentBuffer      │
                              │   (NEW)              │
                              │   - segments[]       │
                              │   - playlistGen      │
                              │   - manifestGen      │
                              └──────────┬───────────┘
                                         │
                              ┌──────────▼───────────┐
                              │   Segment            │
                              │   (NEW)              │
                              │   - sequence         │
                              │   - duration         │
                              │   - data             │
                              └──────────────────────┘
```

## Modified Entities

### RelayProfile (models/relay_profile.go)

**Changes**: Add DASH output format, codec validation

```go
// OutputFormat represents the output container format.
type OutputFormat string

const (
    OutputFormatMPEGTS OutputFormat = "mpegts"   // MPEG Transport Stream (default)
    OutputFormatHLS    OutputFormat = "hls"      // HTTP Live Streaming
    OutputFormatDASH   OutputFormat = "dash"     // Dynamic Adaptive Streaming over HTTP
    OutputFormatFLV    OutputFormat = "flv"      // Flash Video
    OutputFormatMKV    OutputFormat = "matroska"
    OutputFormatMP4    OutputFormat = "mp4"
)

// VideoCodec additions for DASH support
const (
    // ... existing codecs ...
    VideoCodecVP9     VideoCodec = "libvpx-vp9"  // VP9 (DASH only)
    VideoCodecAV1     VideoCodec = "libaom-av1"  // AV1 software
    VideoCodecAV1NVENC VideoCodec = "av1_nvenc"  // AV1 NVIDIA
    VideoCodecAV1QSV  VideoCodec = "av1_qsv"     // AV1 QuickSync
)

// AudioCodec additions for DASH support
const (
    // ... existing codecs ...
    AudioCodecOpus AudioCodec = "libopus" // Opus (DASH only)
)
```

**New Fields on RelayProfile**:

| Field | Type | GORM | JSON | Description |
|-------|------|------|------|-------------|
| SegmentDuration | int | `default:6` | `segment_duration` | HLS/DASH segment duration (seconds) |
| PlaylistSize | int | `default:5` | `playlist_size` | Number of segments in playlist |

**Validation Rules**:
- If OutputFormat is MPEG-TS or HLS: VP9, AV1, Opus codecs are invalid
- SegmentDuration: 2-10 seconds
- PlaylistSize: 3-20 segments

```go
func (p *RelayProfile) ValidateCodecFormat() error {
    dashOnlyVideoCodecs := []VideoCodec{
        VideoCodecVP9, VideoCodecAV1, VideoCodecAV1NVENC, VideoCodecAV1QSV,
    }
    dashOnlyAudioCodecs := []AudioCodec{AudioCodecOpus}

    if p.OutputFormat != OutputFormatDASH {
        for _, codec := range dashOnlyVideoCodecs {
            if p.VideoCodec == codec {
                return fmt.Errorf("%s requires DASH output format", codec)
            }
        }
        for _, codec := range dashOnlyAudioCodecs {
            if p.AudioCodec == codec {
                return fmt.Errorf("%s requires DASH output format", codec)
            }
        }
    }
    return nil
}
```

### StreamSession (relay/session.go)

**Changes**: Add format router and segment buffer

```go
type RelaySession struct {
    // ... existing fields ...

    // Format-aware output
    formatRouter   *FormatRouter       // Routes requests to appropriate output handler
    segmentBuffer  *SegmentBuffer      // Segment storage for HLS/DASH
    outputFormat   OutputFormat        // Current output format
}
```

## New Entities

### Segment (relay/segment.go)

```go
// Segment represents a discrete media segment for HLS/DASH output.
type Segment struct {
    // Sequence is the segment number (monotonically increasing).
    Sequence uint64

    // Duration is the segment duration in seconds.
    Duration float64

    // Data is the raw segment bytes (.ts for HLS, .m4s for DASH).
    Data []byte

    // Timestamp is when the segment was created.
    Timestamp time.Time

    // IsKeyframe indicates if segment starts with a keyframe.
    IsKeyframe bool

    // PTS is the presentation timestamp of the first frame.
    PTS int64

    // DTS is the decode timestamp of the first frame.
    DTS int64
}

// Size returns the byte size of the segment.
func (s *Segment) Size() int {
    return len(s.Data)
}
```

### SegmentBuffer (relay/segment_buffer.go)

```go
// SegmentBufferConfig configures the segment buffer.
type SegmentBufferConfig struct {
    // MaxSegments is the maximum number of segments to keep.
    MaxSegments int

    // TargetDuration is the target segment duration (seconds).
    TargetDuration int

    // MaxBufferSize is the maximum total buffer size in bytes.
    MaxBufferSize int64
}

// DefaultSegmentBufferConfig returns defaults per spec requirements.
func DefaultSegmentBufferConfig() SegmentBufferConfig {
    return SegmentBufferConfig{
        MaxSegments:    5,    // FR-012
        TargetDuration: 6,    // FR-011
        MaxBufferSize:  100 * 1024 * 1024, // SC-004: 100MB per stream
    }
}

// SegmentBuffer manages segments for HLS/DASH delivery.
type SegmentBuffer struct {
    config   SegmentBufferConfig
    mu       sync.RWMutex
    segments []Segment
    sequence atomic.Uint64
    closed   bool

    // Clients tracking
    clientsMu sync.RWMutex
    clients   map[uuid.UUID]*SegmentClient

    // Metrics
    totalBytes   atomic.Uint64
    currentSize  atomic.Int64
}

// NewSegmentBuffer creates a new segment buffer.
func NewSegmentBuffer(config SegmentBufferConfig) *SegmentBuffer

// AddSegment adds a segment to the buffer.
func (sb *SegmentBuffer) AddSegment(seg Segment) error

// GetSegment retrieves a segment by sequence number.
func (sb *SegmentBuffer) GetSegment(sequence uint64) (*Segment, error)

// GetSegments returns all available segments.
func (sb *SegmentBuffer) GetSegments() []Segment

// AddClient adds a client to track.
func (sb *SegmentBuffer) AddClient(userAgent, remoteAddr string) (*SegmentClient, error)

// RemoveClient removes a client.
func (sb *SegmentBuffer) RemoveClient(clientID uuid.UUID) bool

// Close closes the buffer.
func (sb *SegmentBuffer) Close()

// Stats returns buffer statistics.
func (sb *SegmentBuffer) Stats() SegmentBufferStats
```

### SegmentClient (relay/segment_buffer.go)

```go
// SegmentClient tracks a client's position in the segment buffer.
type SegmentClient struct {
    ID           uuid.UUID
    UserAgent    string
    RemoteAddr   string
    ConnectedAt  time.Time
    LastRequest  time.Time

    lastSegment  atomic.Uint64  // Last segment sequence requested
    bytesServed  atomic.Uint64
}
```

### FormatRouter (relay/format_router.go)

```go
// OutputRequest represents a client request for stream output.
type OutputRequest struct {
    Format     OutputFormat // Requested format (hls, dash, mpegts, auto)
    Segment    *int         // Segment number (for HLS/DASH segment requests)
    InitType   string       // For DASH: "v" (video) or "a" (audio)
    UserAgent  string
    Accept     string
}

// FormatRouter routes requests to appropriate output handlers.
type FormatRouter struct {
    defaultFormat OutputFormat
    session       *RelaySession
}

// NewFormatRouter creates a new format router.
func NewFormatRouter(defaultFormat OutputFormat) *FormatRouter

// Route determines the output handler for a request.
func (r *FormatRouter) Route(req OutputRequest) (OutputHandler, error)

// DetectOptimalFormat auto-detects the best format for a client.
func (r *FormatRouter) DetectOptimalFormat(userAgent, accept string) OutputFormat
```

### OutputHandler Interface (relay/output_handler.go)

```go
// OutputHandler handles output for a specific format.
type OutputHandler interface {
    // ContentType returns the Content-Type for the response.
    ContentType() string

    // ServePlaylist serves the playlist/manifest.
    ServePlaylist(w http.ResponseWriter, baseURL string) error

    // ServeSegment serves a specific segment.
    ServeSegment(w http.ResponseWriter, sequence uint64) error

    // ServeStream serves a continuous stream (MPEG-TS only).
    ServeStream(ctx context.Context, w http.ResponseWriter) error
}
```

### HLSHandler (relay/hls_handler.go)

```go
// HLSHandler handles HLS output.
type HLSHandler struct {
    buffer *SegmentBuffer
}

// NewHLSHandler creates an HLS output handler.
func NewHLSHandler(buffer *SegmentBuffer) *HLSHandler

// ContentType returns HLS content type.
func (h *HLSHandler) ContentType() string {
    return "application/vnd.apple.mpegurl"
}

// ServePlaylist generates and serves the HLS playlist.
func (h *HLSHandler) ServePlaylist(w http.ResponseWriter, baseURL string) error

// ServeSegment serves a .ts segment.
func (h *HLSHandler) ServeSegment(w http.ResponseWriter, sequence uint64) error

// GeneratePlaylist creates an HLS playlist from current segments.
func (h *HLSHandler) GeneratePlaylist(baseURL string) string
```

### DASHHandler (relay/dash_handler.go)

```go
// DASHHandler handles DASH output.
type DASHHandler struct {
    buffer       *SegmentBuffer
    initVideoSeg []byte  // Video initialization segment
    initAudioSeg []byte  // Audio initialization segment
}

// NewDASHHandler creates a DASH output handler.
func NewDASHHandler(buffer *SegmentBuffer) *DASHHandler

// ContentType returns DASH manifest content type.
func (d *DASHHandler) ContentType() string {
    return "application/dash+xml"
}

// ServePlaylist generates and serves the DASH MPD manifest.
func (d *DASHHandler) ServePlaylist(w http.ResponseWriter, baseURL string) error

// ServeSegment serves a media segment (.m4s).
func (d *DASHHandler) ServeSegment(w http.ResponseWriter, sequence uint64) error

// ServeInitSegment serves the initialization segment.
func (d *DASHHandler) ServeInitSegment(w http.ResponseWriter, streamType string) error

// GenerateManifest creates a DASH MPD manifest from current segments.
func (d *DASHHandler) GenerateManifest(baseURL string) string
```

## Database Schema Changes

### Migration: Add SegmentDuration and PlaylistSize to relay_profiles

```sql
-- Migration: 20250312_add_segment_config
ALTER TABLE relay_profiles
ADD COLUMN segment_duration INTEGER DEFAULT 6;

ALTER TABLE relay_profiles
ADD COLUMN playlist_size INTEGER DEFAULT 5;

-- Note: output_format already exists with mpegts/hls values
-- No migration needed for DASH - just add constant to Go code
```

## Validation Rules Summary

| Entity | Field | Rule |
|--------|-------|------|
| RelayProfile | VideoCodec | VP9, AV1 require DASH format |
| RelayProfile | AudioCodec | Opus requires DASH format |
| RelayProfile | SegmentDuration | 2-10 seconds |
| RelayProfile | PlaylistSize | 3-20 segments |
| SegmentBuffer | MaxSegments | Must be >= PlaylistSize |
| SegmentBuffer | MaxBufferSize | Enforced at write time |
| Segment | Data | Non-empty byte slice |
| Segment | Sequence | Monotonically increasing |

## State Transitions

### RelaySession Output Mode

```
                          ┌───────────────┐
                          │   STOPPED     │
                          └───────┬───────┘
                                  │ start()
                          ┌───────▼───────┐
               ┌──────────│   STARTING    │──────────┐
               │          └───────────────┘          │
               │ format=mpegts                       │ format=hls|dash
        ┌──────▼──────┐                      ┌───────▼───────┐
        │  STREAMING  │                      │  SEGMENTING   │
        │  (MPEG-TS)  │                      │  (HLS/DASH)   │
        └──────┬──────┘                      └───────┬───────┘
               │                                     │
               │ error/stop                          │ error/stop
               │                                     │
               └─────────────┬───────────────────────┘
                             │
                      ┌──────▼──────┐
                      │   STOPPED   │
                      └─────────────┘
```

## Constants and Content Types

```go
// Content types for streaming formats
const (
    ContentTypeHLSPlaylist  = "application/vnd.apple.mpegurl"
    ContentTypeHLSSegment   = "video/MP2T"
    ContentTypeDASHManifest = "application/dash+xml"
    ContentTypeDASHSegment  = "video/iso.segment"
    ContentTypeDASHInit     = "video/mp4"
    ContentTypeMPEGTS       = "video/MP2T"
)

// Query parameter names
const (
    QueryParamFormat  = "format"
    QueryParamSegment = "seg"
    QueryParamInit    = "init"
)

// Format values
const (
    FormatValueHLS    = "hls"
    FormatValueDASH   = "dash"
    FormatValueMPEGTS = "mpegts"
    FormatValueAuto   = "auto"
)
```
