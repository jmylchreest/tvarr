# Research: Smart Container Routing

**Feature Branch**: `013-smart-container-routing`
**Date**: 2025-12-10
**Status**: Complete

## Executive Summary

This research validates the technical feasibility of smart container routing using gohlslib for HLS repackaging, X-Tvarr-Player header injection in frontend players, and detection_mode-based routing decisions. All key assumptions from the spec are validated.

## Research Questions

### RQ-1: gohlslib Muxer API for HLS Output

**Question**: Can gohlslib v2 Muxer produce HLS output with both MPEG-TS and fMP4 segments?

**Finding**: YES - gohlslib v2 provides `gohlslib.Muxer` that supports both segment types.

**Evidence**: From gohlslib v2 source code:

```go
// Muxer configuration options
type MuxerVariant int

const (
    MuxerVariantMPEGTS MuxerVariant = iota  // MPEG-TS segments
    MuxerVariantFMP4                         // fMP4 segments (CMAF)
    MuxerVariantLowLatency                   // Low-latency HLS
)

type Muxer struct {
    Variant          MuxerVariant
    SegmentDuration  time.Duration
    SegmentMaxSize   uint64
    Directory        string  // Filesystem path OR use WriteSegment callback
    OnSegmentReady   func(path string) error
    // ... other options
}
```

**Implementation Pattern**: The existing `HLSCollapser` uses gohlslib Client for input. We can create an `HLSMuxer` wrapper that:
1. Takes track data from gohlslib Client callbacks (OnDataH26x, OnDataMPEG4Audio)
2. Feeds data to gohlslib Muxer
3. Serves segments via HTTP using the OnSegmentReady callback

**Existing Code Reference**: `internal/relay/hls_collapser.go:98-102` shows gohlslib.Client usage pattern.

### RQ-2: Frontend Header Injection (mpegts.js, hls.js)

**Question**: Can mpegts.js and hls.js inject custom headers into requests?

**Finding**: YES for both, with different APIs.

**mpegts.js Pattern**:
```typescript
const player = mpegts.createPlayer({
    type: 'mpegts',
    url: streamUrl,
}, {
    headers: {
        'X-Tvarr-Player': 'mpegts.js/1.7.3'
    }
});
```

**hls.js Pattern**:
```typescript
const hls = new Hls({
    xhrSetup: function(xhr: XMLHttpRequest, url: string) {
        xhr.setRequestHeader('X-Tvarr-Player', 'hls.js/1.5.8');
    }
});
```

**Existing Code Reference**: `frontend/src/player/MpegTsAdapter.ts:140-146` shows mediaDataSource configuration where headers can be added.

### RQ-3: detection_mode Field Location

**Question**: Where should `detection_mode` be added to the data model?

**Finding**: Add to `RelayProfile` model in `internal/models/relay_profile.go`.

**Reasoning**:
- The routing decision is per-profile, not per-channel
- `detection_mode` controls whether client detection applies
- Values: `"auto"` (enable client detection) or explicit mode (`"hls"`, `"mpegts"`, etc.)

**Schema Addition**:
```go
// DetectionMode controls client detection behavior.
// "auto" enables smart routing based on client detection.
// Other values (e.g., "hls", "mpegts") use profile settings as-is.
DetectionMode string `gorm:"size:20;default:'auto'" json:"detection_mode"`
```

**Migration Strategy**: Add column with default `"auto"` - backwards compatible, existing profiles use smart routing.

### RQ-4: Format Router Extension Points

**Question**: How should the FormatRouter be extended for client detection?

**Finding**: Extend `OutputRequest` struct and add `ClientDetector` component.

**Current OutputRequest** (`internal/relay/format_router.go:23-40`):
```go
type OutputRequest struct {
    Format    string   // Requested output format
    Segment   *uint64  // Segment number
    InitType  string   // DASH init segment type
    UserAgent string   // Client User-Agent
    Accept    string   // Accept header
}
```

**Extended OutputRequest**:
```go
type OutputRequest struct {
    Format         string   // Requested output format
    Segment        *uint64
    InitType       string
    UserAgent      string
    Accept         string
    XTvarrPlayer   string   // NEW: X-Tvarr-Player header value
    FormatOverride string   // NEW: ?format= query parameter
}
```

**New ClientDetector Interface**:
```go
type ClientCapabilities struct {
    PlayerName       string   // e.g., "hls.js", "mpegts.js"
    PlayerVersion    string
    PrefersFMP4      bool
    PrefersMPEGTS    bool
    SupportedCodecs  []string
}

type ClientDetector interface {
    Detect(req OutputRequest) ClientCapabilities
}
```

### RQ-5: Routing Decision Enumeration

**Question**: What routing decisions should be defined?

**Finding**: Three distinct routing paths with clear criteria.

```go
type RoutingDecision int

const (
    // RoutePassthrough - Direct proxy of source segments (no processing)
    RoutePassthrough RoutingDecision = iota

    // RouteRepackage - Container change via gohlslib Muxer (no codec change)
    RouteRepackage

    // RouteTranscode - FFmpeg pipeline (codec change or segmentation)
    RouteTranscode
)
```

**Decision Matrix** (when `detection_mode = "auto"`):

| Source Format | Client Wants | Codecs Match | Decision |
|---------------|--------------|--------------|----------|
| HLS (fMP4)    | HLS (fMP4)   | Yes          | Passthrough |
| HLS (fMP4)    | HLS (TS)     | Yes          | Repackage |
| HLS (TS)      | HLS (fMP4)   | Yes          | Repackage |
| HLS (TS)      | HLS (TS)     | Yes          | Passthrough |
| Raw TS        | HLS (any)    | N/A          | Transcode (segmentation) |
| Any           | Any          | No           | Transcode |

### RQ-6: Upstream Connection Sharing

**Question**: How can multiple clients share a single upstream connection?

**Finding**: Existing session management already supports this pattern.

**Evidence**: `internal/relay/manager.go` has session tracking. Sessions can be keyed by (channelID, profileID, sourceURL) to enable connection reuse.

**Enhancement Needed**: Add subscriber reference counting to sessions so multiple clients reading from the same source don't create duplicate upstream connections.

## Resolved Unknowns

| # | Unknown | Resolution |
|---|---------|------------|
| 1 | gohlslib Muxer API | Confirmed - supports MPEG-TS and fMP4 variants |
| 2 | Header injection | Confirmed - both mpegts.js and hls.js support custom headers |
| 3 | detection_mode location | RelayProfile model, default "auto" |
| 4 | FormatRouter extension | Add XTvarrPlayer and FormatOverride to OutputRequest |
| 5 | Routing decisions | Three paths: Passthrough, Repackage, Transcode |
| 6 | Connection sharing | Session management with subscriber counting |

## Dependencies Confirmed

- **gohlslib v2**: Already in use (`go.mod`), Muxer available
- **mpegts.js**: Already integrated, headers config supported
- **hls.js**: Not yet integrated, but header injection via xhrSetup confirmed

## Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| gohlslib Muxer memory usage for many concurrent streams | Medium | High | Implement session limits, monitor memory |
| Client detection accuracy for unknown user agents | Low | Medium | Fallback to MPEG-TS (universal compatibility) |
| Segment timing drift in repackaging | Low | Medium | Use gohlslib's built-in timing synchronization |

## Recommendations

1. **Start with X-Tvarr-Player header injection** - Low risk, immediate benefit for detection accuracy
2. **Add detection_mode to RelayProfile** - Schema change, do early
3. **Implement gohlslib Muxer wrapper** - Core repackaging capability
4. **Extend FormatRouter with ClientDetector** - Wire everything together
5. **Add routing decision logging** - Observability per FR-009

## Next Steps

1. Generate data-model.md with detection_mode field
2. Generate tasks.md for implementation order
3. Create feature branch `013-smart-container-routing`
