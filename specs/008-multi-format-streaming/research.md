# Research: Multi-Format Streaming Support

**Feature**: 008-multi-format-streaming
**Date**: 2025-12-07
**Status**: Complete

## Overview

This research consolidates findings from investigations into HLS/DASH output generation, FFmpeg muxer capabilities, existing codebase patterns, and implementation approaches for multi-format streaming.

## Research Questions Resolved

### R1: How to Generate HLS Output Segments?

**Decision**: Use FFmpeg HLS muxer with file-based segment output to tmpfs/memory, with application-managed segment serving.

**Rationale**:
- FFmpeg's HLS muxer is mature and widely tested
- Cannot output segments to pipe/stdout - requires file-based output
- tmpfs/ramdisk eliminates disk I/O overhead
- Application handles segment serving and playlist generation for query-param routing

**Alternatives Considered**:
1. gohlslib Muxer - Does not exist; gohlslib is client-only
2. FFmpeg fragmented MP4 to pipe - Requires application-side segmentation
3. Custom MPEG-TS segmentation - High complexity, reinventing wheel

**FFmpeg Command Pattern**:
```bash
ffmpeg -i <input> \
  -f hls \
  -hls_time 6 \
  -hls_list_size 5 \
  -hls_flags delete_segments+omit_endlist \
  -hls_segment_type mpegts \
  -c:v libx264 -g 180 \
  -c:a aac \
  /tmp/hls_segments/stream.m3u8
```

### R2: How to Generate DASH Output Segments?

**Decision**: Use FFmpeg DASH muxer with segment template, file-based output to memory.

**Rationale**:
- FFmpeg DASH muxer generates DASH-IF compliant manifests
- Supports fMP4 segments (required for VP9/AV1/Opus codecs)
- Segment template pattern simplifies URL routing

**FFmpeg Command Pattern**:
```bash
ffmpeg -i <input> \
  -f dash \
  -seg_duration 6 \
  -window_size 5 \
  -extra_window_size 3 \
  -use_template 1 \
  -use_timeline 1 \
  -streaming 1 \
  -adaptation_sets "id=0,streams=v id=1,streams=a" \
  -c:v libx264 -g 180 \
  -c:a aac \
  /tmp/dash_segments/stream.mpd
```

### R3: Memory-Based Segment Storage Approach?

**Decision**: Use in-application segment buffer with metadata tracking, not tmpfs.

**Rationale**:
- Existing `CyclicBuffer` pattern provides memory management
- Segment metadata (sequence, duration, timestamp) needed for playlist generation
- Avoids file system dependency and cleanup complexity
- Query-param routing requires application-controlled segment serving

**Implementation Pattern**:
```go
type Segment struct {
    Sequence    uint64
    Duration    float64
    Data        []byte
    Timestamp   time.Time
    IsKeyframe  bool
}

type SegmentBuffer struct {
    segments    []Segment
    maxSegments int
    // ... ring buffer mechanics
}
```

**Alternative Considered**: tmpfs with file watching - rejected due to complexity and file system dependency.

### R4: Content-Type Headers for HLS/DASH?

**Decision**: Use MIME types per standard specifications.

| File Type | Content-Type | Notes |
|-----------|--------------|-------|
| `.m3u8` | `application/vnd.apple.mpegurl` | HLS playlist |
| `.ts` | `video/MP2T` | MPEG-TS segment |
| `.mpd` | `application/dash+xml` | DASH manifest |
| `.m4s` | `video/iso.segment` | fMP4 segment |
| `.mp4` (init) | `video/mp4` | DASH init segment |

### R5: Query-Parameter Driven Segment URLs?

**Decision**: Use consistent query-param pattern for all formats.

**Rationale**:
- Matches existing proxy mode pattern (`?format=`)
- Enables dynamic format switching without URL changes
- Single endpoint serves all formats

**URL Patterns**:
```
# Playlist/Manifest
/proxy/{proxyId}/{channelId}?format=hls         → HLS playlist
/proxy/{proxyId}/{channelId}?format=dash        → DASH manifest
/proxy/{proxyId}/{channelId}?format=mpegts      → MPEG-TS stream

# Segments (HLS)
/proxy/{proxyId}/{channelId}?format=hls&seg=0   → Segment 0
/proxy/{proxyId}/{channelId}?format=hls&seg=1   → Segment 1

# Segments (DASH)
/proxy/{proxyId}/{channelId}?format=dash&init=v → Video init
/proxy/{proxyId}/{channelId}?format=dash&seg=v0 → Video segment 0
/proxy/{proxyId}/{channelId}?format=dash&seg=a0 → Audio segment 0
```

### R6: Existing Codebase Patterns for Multi-Client Streaming?

**Decision**: Extend existing `CyclicBuffer` pattern for segment-aware buffering.

**Findings**:
- `CyclicBuffer` (cyclic_buffer.go) handles multi-client byte-stream delivery
- `BufferClient` tracks per-client read position
- Segment-based approach needs different data structure (discrete chunks vs continuous stream)

**Adaptation**:
- Create `SegmentBuffer` with segment-level granularity
- Maintain per-client segment sequence tracking
- Segments are immutable once written (no partial reads)

### R7: Current gohlslib/astits Usage?

**Findings**:
- **gohlslib**: Client-only library for consuming HLS streams
  - Used in `HLSCollapser` for input (HLS → MPEG-TS conversion)
  - Does NOT support HLS output generation
  - Provides track detection and media frame extraction

- **go-astits**: MPEG-TS muxer for output
  - Used to generate continuous MPEG-TS from extracted frames
  - Can generate HLS-compatible .ts segments
  - Does NOT generate HLS playlists

**Implication**: Need FFmpeg for HLS playlist generation, or implement playlist generator in Go.

### R8: Container-Aware Codec Compatibility?

**Decision**: Implement codec validation based on output format.

| Container | Supported Video | Supported Audio |
|-----------|-----------------|-----------------|
| MPEG-TS | H.264, H.265 | AAC, MP3, AC3, EAC3 |
| HLS (.ts) | H.264, H.265 | AAC, MP3, AC3, EAC3 |
| DASH (fMP4) | H.264, H.265, **VP9, AV1** | AAC, MP3, AC3, EAC3, **Opus** |

**Implementation**: Add validation in `RelayProfile.Validate()` and frontend dropdown filtering.

### R9: Auto-Format Detection Logic?

**Decision**: User-Agent and Accept header based detection with configurable default.

**Detection Rules**:
```go
func detectOptimalFormat(userAgent, accept string) OutputFormat {
    // iOS/Safari → HLS
    if strings.Contains(userAgent, "Safari") &&
       !strings.Contains(userAgent, "Chrome") {
        return OutputFormatHLS
    }

    // Accept header preference
    if strings.Contains(accept, "application/dash+xml") {
        return OutputFormatDASH
    }

    // Fallback to configured default (MPEG-TS)
    return proxyConfig.DefaultFormat
}
```

## Architecture Decisions

### AD1: HLS/DASH Output Architecture

```
                                    ┌─────────────────┐
                                    │  Client Request │
                                    │  ?format=hls    │
                                    └────────┬────────┘
                                             │
                                    ┌────────▼────────┐
                                    │  Format Router  │
                                    └────────┬────────┘
                                             │
          ┌──────────────────────────────────┼──────────────────────────────────┐
          │                                  │                                  │
┌─────────▼─────────┐           ┌────────────▼────────────┐        ┌──────────▼──────────┐
│   MPEG-TS Mode    │           │       HLS Mode          │        │     DASH Mode       │
│  (existing flow)  │           │                         │        │                     │
└─────────┬─────────┘           └────────────┬────────────┘        └──────────┬──────────┘
          │                                  │                                │
          │                     ┌────────────▼────────────┐        ┌─────────▼──────────┐
          │                     │   SegmentBuffer (HLS)   │        │  SegmentBuffer(DASH│
          │                     │   - .ts segments        │        │  - init.mp4        │
          │                     │   - sequence numbers    │        │  - .m4s segments   │
          │                     │   - playlist generator  │        │  - MPD generator   │
          │                     └────────────┬────────────┘        └─────────┬──────────┘
          │                                  │                                │
          │                     ┌────────────▼────────────┐        ┌─────────▼──────────┐
          │                     │  FFmpeg HLS Output      │        │  FFmpeg DASH Output│
          │                     │  -f hls                 │        │  -f dash           │
          │                     └─────────────────────────┘        └────────────────────┘
          │                                  │                                │
          └──────────────────────────────────┼────────────────────────────────┘
                                             │
                                    ┌────────▼────────┐
                                    │  Relay Session  │
                                    │  (FFmpeg proc)  │
                                    └────────┬────────┘
                                             │
                                    ┌────────▼────────┐
                                    │  Source Stream  │
                                    └─────────────────┘
```

### AD2: Segment Buffer vs File-Based Output

**Chosen**: Application-managed segment buffer with FFmpeg pipe input

**Rationale**:
1. FFmpeg MPEG-TS output to pipe is already working (existing relay)
2. Application parses MPEG-TS, extracts segments at keyframes
3. Segments stored in ring buffer with metadata
4. Playlist/manifest generated dynamically from buffer state

**Alternative**: FFmpeg file output + file watching
- Rejected: Adds file system dependency, cleanup complexity, harder testing

### AD3: FFmpeg Output Mode Selection

**For MPEG-TS (default, existing)**:
- Output to pipe as continuous stream
- No segmentation needed

**For HLS**:
- Output MPEG-TS to pipe
- Application segments at keyframes (6-second default)
- Application generates .m3u8 playlist

**For DASH**:
- Output fragmented MP4 to pipe
- Application extracts init segment and media fragments
- Application generates .mpd manifest

## Implementation Notes

### HLS Playlist Generation

```go
func (s *SegmentBuffer) GenerateHLSPlaylist(baseURL string) string {
    var buf strings.Builder
    buf.WriteString("#EXTM3U\n")
    buf.WriteString("#EXT-X-VERSION:3\n")
    buf.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", s.targetDuration))
    buf.WriteString(fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d\n", s.segments[0].Sequence))

    for _, seg := range s.segments {
        buf.WriteString(fmt.Sprintf("#EXTINF:%.3f,\n", seg.Duration))
        buf.WriteString(fmt.Sprintf("%s?format=hls&seg=%d\n", baseURL, seg.Sequence))
    }

    return buf.String()
}
```

### DASH Manifest Generation

```go
func (s *SegmentBuffer) GenerateDASHManifest(baseURL string) string {
    // Generate DASH-IF compliant MPD with SegmentTemplate
    // Reference: https://dashif.org/docs/DASH-IF-IOP-v4.3.pdf
}
```

### Segment Extraction from MPEG-TS Stream

Use existing `go-astits` demuxer capabilities to:
1. Detect keyframes (Random Access Indicator)
2. Extract segment boundaries
3. Calculate segment duration from PTS differences

## Dependencies

| Dependency | Version | Purpose |
|------------|---------|---------|
| go-astits | v1.14.0 | MPEG-TS muxing/demuxing (existing) |
| gohlslib | v2.2.4 | HLS input consumption (existing) |
| mediacommon | v2.5.2 | Codec utilities (existing) |
| FFmpeg | 5.0+ | Transcoding engine (existing) |

No new dependencies required - implementation uses existing libraries.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| FFmpeg fMP4 output complexity | Medium | Start with HLS (TS segments), add DASH later |
| Segment boundary detection accuracy | Medium | Use FFmpeg keyframe alignment (-force_key_frames) |
| Memory usage with many streams | High | Configurable segment buffer size, aggressive eviction |
| Browser compatibility | Low | Test against Safari, Chrome, VLC, dash.js |

## References

- [FFmpeg HLS Muxer Documentation](https://ffmpeg.org/ffmpeg-formats.html#hls-2)
- [FFmpeg DASH Muxer Documentation](https://ffmpeg.org/ffmpeg-formats.html#dash-2)
- [DASH-IF Implementation Guidelines](https://dashif.org/docs/DASH-IF-IOP-v4.3.pdf)
- [Apple HLS Authoring Specification](https://developer.apple.com/documentation/http-live-streaming)
- [gohlslib Documentation](https://github.com/bluenviron/gohlslib)
- [go-astits Documentation](https://github.com/asticode/go-astits)
