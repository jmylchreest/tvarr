# ffmpegd Probe Capability

## Overview

Extend the ffmpegd RPC to support stream probing, moving ffprobe logic from tvarr to tvarr-ffmpegd. This allows probing to work even when ffprobe isn't available on the coordinator, and sets up a generic "probe-capable transcoder" abstraction for future transcoder types.

## Problem Statement

Currently:

1. **ffprobe lives on coordinator**: `internal/ffmpeg/prober.go` runs ffprobe locally on tvarr
2. **Graceful degradation exists**: If ffprobe isn't detected, probing is skipped (debug log, returns nil)
3. **Tight coupling**: Codec detection (`bitstream_filters.go`) relies on local ffprobe
4. **Future limitation**: New transcoder types (e.g., GStreamer, hardware-specific) would need their own probe implementations

The design doc (Question #6) already recommends: "tvarr-ffmpegd handles probing, reports results to coordinator"

## Proposed Solution

### 1. Daemon Capability Flags

Extend the `Capabilities` message to declare what a daemon can do:

```protobuf
message Capabilities {
  // Existing fields...
  repeated string video_encoders = 2;
  repeated string audio_encoders = 3;
  repeated string video_decoders = 4;
  int32 max_concurrent_jobs = 5;
  PerformanceMetrics performance = 6;
  repeated HWAccelInfo hw_accels = 1;
  repeated GPUInfo gpus = 7;

  // NEW: Capability flags
  bool can_probe = 20;      // Daemon can probe streams (ffprobe available)
  bool can_transcode = 21;  // Daemon can transcode (ffmpeg available)

  // Future extensibility
  // bool can_analyze = 22;   // Deep stream analysis
  // bool can_validate = 23;  // Format validation
}
```

### 2. Probe RPC Method

Add a new RPC to the FFmpegDaemon service:

```protobuf
service FFmpegDaemon {
  // Existing methods...
  rpc Register(RegisterRequest) returns (RegisterResponse);
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
  rpc Unregister(UnregisterRequest) returns (UnregisterResponse);
  rpc Transcode(stream TranscodeMessage) returns (stream TranscodeMessage);
  rpc GetStats(GetStatsRequest) returns (GetStatsResponse);

  // NEW: Stream probing
  rpc Probe(ProbeRequest) returns (ProbeResponse);
}

message ProbeRequest {
  string stream_url = 1;
  ProbeType probe_type = 2;
  int32 timeout_ms = 3;          // 0 = use default
  map<string, string> headers = 4; // HTTP headers for auth
}

enum ProbeType {
  PROBE_TYPE_FULL = 0;           // Complete stream analysis (30s timeout)
  PROBE_TYPE_QUICK = 1;          // Quick codec detection (5s timeout)
  PROBE_TYPE_HEALTH_CHECK = 2;   // Minimal connectivity check (5s timeout)
}

message ProbeResponse {
  bool success = 1;
  string error = 2;              // Error message if !success

  // Stream info (mirrors existing ProbeResult/StreamInfo)
  StreamProbeResult result = 3;

  // Timing
  int64 probe_duration_ms = 4;
}

message StreamProbeResult {
  // Format info
  string container_format = 1;   // e.g., "mpegts", "flv", "hls"
  double duration_seconds = 2;   // 0 for live streams
  bool is_live_stream = 3;

  // Primary video track
  VideoTrackInfo video = 4;

  // Primary audio track
  AudioTrackInfo audio = 5;

  // All tracks (for multi-audio, subtitles, etc.)
  repeated VideoTrackInfo video_tracks = 6;
  repeated AudioTrackInfo audio_tracks = 7;
  repeated SubtitleTrackInfo subtitle_tracks = 8;
}

message VideoTrackInfo {
  int32 index = 1;
  string codec = 2;              // e.g., "h264", "hevc", "vp9"
  string profile = 3;            // e.g., "high", "main"
  int32 level = 4;               // e.g., 41 for level 4.1
  int32 width = 5;
  int32 height = 6;
  double framerate = 7;
  int64 bitrate_bps = 8;
  string pixel_format = 9;       // e.g., "yuv420p"
  bool is_default = 10;
  string language = 11;
}

message AudioTrackInfo {
  int32 index = 1;
  string codec = 2;              // e.g., "aac", "ac3", "eac3"
  int32 sample_rate = 3;
  int32 channels = 4;
  string channel_layout = 5;     // e.g., "stereo", "5.1"
  int64 bitrate_bps = 6;
  bool is_default = 7;
  string language = 8;
}

message SubtitleTrackInfo {
  int32 index = 1;
  string codec = 2;              // e.g., "dvb_teletext", "webvtt"
  string language = 3;
  bool is_forced = 4;
}
```

### 3. Daemon-Side Implementation

The daemon implements the Probe RPC by invoking its local ffprobe:

```go
// internal/daemon/probe.go

func (s *Server) Probe(ctx context.Context, req *proto.ProbeRequest) (*proto.ProbeResponse, error) {
    if s.prober == nil {
        return &proto.ProbeResponse{
            Success: false,
            Error:   "ffprobe not available on this daemon",
        }, nil
    }

    start := time.Now()

    var result *ffmpeg.ProbeResult
    var err error

    timeout := time.Duration(req.TimeoutMs) * time.Millisecond
    if timeout == 0 {
        timeout = s.defaultProbeTimeout(req.ProbeType)
    }

    probeCtx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    switch req.ProbeType {
    case proto.ProbeType_PROBE_TYPE_QUICK:
        result, err = s.prober.QuickProbe(probeCtx, req.StreamUrl)
    case proto.ProbeType_PROBE_TYPE_HEALTH_CHECK:
        err = s.prober.CheckStreamHealth(probeCtx, req.StreamUrl)
        if err == nil {
            return &proto.ProbeResponse{
                Success:         true,
                ProbeDurationMs: time.Since(start).Milliseconds(),
            }, nil
        }
    default: // PROBE_TYPE_FULL
        result, err = s.prober.Probe(probeCtx, req.StreamUrl)
    }

    if err != nil {
        return &proto.ProbeResponse{
            Success:         false,
            Error:           err.Error(),
            ProbeDurationMs: time.Since(start).Milliseconds(),
        }, nil
    }

    return &proto.ProbeResponse{
        Success:         true,
        Result:          toProtoProbeResult(result),
        ProbeDurationMs: time.Since(start).Milliseconds(),
    }, nil
}
```

### 4. Coordinator-Side: ProberService Abstraction

Create a generic prober interface that can dispatch to local or remote probers:

```go
// internal/probe/prober.go

// Prober abstracts stream probing - could be local or remote
type Prober interface {
    // Probe performs a full stream analysis
    Probe(ctx context.Context, streamURL string) (*ProbeResult, error)

    // QuickProbe performs a fast codec detection
    QuickProbe(ctx context.Context, streamURL string) (*ProbeResult, error)

    // CheckStreamHealth verifies stream is accessible
    CheckStreamHealth(ctx context.Context, streamURL string) error

    // Available returns whether this prober is ready to use
    Available() bool
}

// LocalProber uses local ffprobe binary
type LocalProber struct {
    ffprobe *ffmpeg.Prober
}

// RemoteProber dispatches to a capable daemon
type RemoteProber struct {
    registry *DaemonRegistry
    logger   *slog.Logger
}

func (p *RemoteProber) selectProbeCapableDaemon() (*DaemonInfo, error) {
    daemons := p.registry.GetWithCapability(CapabilityProbe)
    if len(daemons) == 0 {
        return nil, ErrNoProbeCapableDaemons
    }
    // Select least loaded probe-capable daemon
    return p.registry.SelectLeastLoaded(daemons), nil
}

func (p *RemoteProber) Probe(ctx context.Context, streamURL string) (*ProbeResult, error) {
    daemon, err := p.selectProbeCapableDaemon()
    if err != nil {
        return nil, err
    }

    resp, err := daemon.Client().Probe(ctx, &proto.ProbeRequest{
        StreamUrl: streamURL,
        ProbeType: proto.ProbeType_PROBE_TYPE_FULL,
    })
    if err != nil {
        return nil, fmt.Errorf("remote probe failed: %w", err)
    }

    if !resp.Success {
        return nil, fmt.Errorf("probe error: %s", resp.Error)
    }

    return fromProtoProbeResult(resp.Result), nil
}

// CompositeProber tries local first, falls back to remote
type CompositeProber struct {
    local  *LocalProber
    remote *RemoteProber
    logger *slog.Logger
}

func (p *CompositeProber) Probe(ctx context.Context, streamURL string) (*ProbeResult, error) {
    // Try local first if available
    if p.local != nil && p.local.Available() {
        return p.local.Probe(ctx, streamURL)
    }

    // Fall back to remote
    if p.remote != nil && p.remote.Available() {
        return p.remote.Probe(ctx, streamURL)
    }

    return nil, ErrNoProberAvailable
}
```

### 5. Update Existing Probe Callers

Replace direct `ffmpeg.Prober` usage with the new abstraction:

**relay/manager.go:450** - Codec caching:
```go
// Before
if m.prober == nil {
    m.logger.Debug("Skipping probe - prober not available")
    return nil
}
result, err := m.prober.QuickProbe(ctx, streamURL)

// After
result, err := m.probeService.QuickProbe(ctx, streamURL)
if err != nil {
    if errors.Is(err, probe.ErrNoProberAvailable) {
        m.logger.Debug("Skipping probe - no prober available")
        return nil
    }
    // Handle other errors...
}
```

**bitstream_filters.go:232** - Codec detection for copy decisions:
```go
// Before
result, err := p.ffprobe.ProbeSimple(ctx, streamURL)

// After
result, err := p.probeService.QuickProbe(ctx, streamURL)
```

### 6. Current ffprobe Behavior (What Happens If Not Detected)

Currently, when ffprobe is not detected:

| Location | Current Behavior |
|----------|------------------|
| `binary.go:135` | FFprobePath set to empty string, no error |
| `cmd/tvarr/cmd/serve.go` | Logs "ffprobe not detected", continues |
| `relay/manager.go:441` | Debug log "Skipping probe", returns nil |
| `bitstream_filters.go` | Falls back to default BSF rules |
| `relay_service.go:220` | Prober remains nil |

This means:
- **Streaming works**: Transcoding proceeds without codec pre-detection
- **BSF selection is conservative**: May apply unnecessary filters (harmless but suboptimal)
- **Codec caching disabled**: More redundant transcoding decisions

With the new remote probe capability:
- **Streaming still works**: Unchanged fallback behavior
- **Remote probe fills the gap**: Coordinator can request probe from any daemon with ffprobe
- **Codec caching works**: Even without local ffprobe

## Implementation Phases

### Phase 1: Protocol & Basic Probe (Suggested First Step)

**Scope**: Add Probe RPC, implement daemon-side handler, verify end-to-end

1. **Update proto** (`pkg/ffmpegd/proto/ffmpegd.proto`):
   - Add `can_probe` and `can_transcode` capability flags
   - Add `Probe` RPC method
   - Add `ProbeRequest`, `ProbeResponse`, and result messages

2. **Daemon implementation** (`internal/daemon/probe.go`):
   - Implement `Probe` RPC handler
   - Reuse existing `ffmpeg.Prober` logic
   - Set `can_probe = true` in registration if ffprobe available

3. **Test with CLI**:
   - Add `tvarr-ffmpegd probe <url>` command for manual testing
   - Verify probe results match local ffprobe output

**Estimated effort**: ~2-3 hours

### Phase 2: Coordinator Integration

**Scope**: Add ProbeService abstraction, update callers

1. Create `internal/probe/` package with interfaces
2. Implement `LocalProber`, `RemoteProber`, `CompositeProber`
3. Wire up in `cmd/tvarr/cmd/serve.go`
4. Update `relay/manager.go` and `bitstream_filters.go` to use new service
5. Add API endpoint: `POST /api/v1/probe` for manual probing from UI

**Estimated effort**: ~4-6 hours

### Phase 3: Future Transcoder Types

**Scope**: Prepare for non-FFmpeg transcoders

1. Define `Transcoder` interface with capability introspection:
   ```go
   type TranscoderType string
   const (
       TranscoderFFmpeg    TranscoderType = "ffmpeg"
       TranscoderGStreamer TranscoderType = "gstreamer"  // future
       TranscoderHardware  TranscoderType = "hardware"   // future
   )

   type TranscoderCapabilities struct {
       Type         TranscoderType
       CanProbe     bool
       CanTranscode bool
       Encoders     []string
       Decoders     []string
   }
   ```

2. Registry tracks capabilities by transcoder type
3. Probe/transcode routing considers transcoder type

**Estimated effort**: ~2-4 hours (design), implementation deferred

## Simple Next Step

**Recommended starting point**: Add the proto definitions and a simple `tvarr-ffmpegd probe` CLI command.

This gives you:
1. Validated proto schema for probe messages
2. Working proof-of-concept you can test manually
3. No changes to coordinator yet (low risk)
4. Foundation for Phase 2 integration

```bash
# After implementation, you could test with:
tvarr-ffmpegd probe --url "http://example.com/stream.m3u8"
tvarr-ffmpegd probe --url "http://example.com/stream.m3u8" --type quick
tvarr-ffmpegd probe --url "http://example.com/stream.m3u8" --type health
```

## Files to Modify

| File | Changes |
|------|---------|
| `pkg/ffmpegd/proto/ffmpegd.proto` | Add capability flags, Probe RPC, messages |
| `internal/daemon/registration.go` | Set can_probe based on ffprobe availability |
| `internal/daemon/probe.go` | New file: Probe RPC handler |
| `cmd/tvarr-ffmpegd/cmd/probe.go` | New file: CLI probe command |
| `internal/probe/prober.go` | New file: Prober interface (Phase 2) |
| `internal/probe/local.go` | New file: LocalProber (Phase 2) |
| `internal/probe/remote.go` | New file: RemoteProber (Phase 2) |
| `internal/probe/composite.go` | New file: CompositeProber (Phase 2) |
| `internal/relay/manager.go` | Use ProbeService instead of direct prober (Phase 2) |
| `internal/ffmpeg/bitstream_filters.go` | Use ProbeService (Phase 2) |

## Open Questions

1. **Probe result caching**: Should remote probe results be cached the same as local?
   - **Recommendation**: Yes, use existing `last_known_codecs` table

2. **Probe failover**: If a remote probe fails, try another daemon?
   - **Recommendation**: Yes, with configurable retry limit

3. **Probe authentication**: How to pass stream auth headers to remote daemon?
   - **Recommendation**: Include in ProbeRequest, daemon adds to ffprobe

4. **Probe concurrency**: Limit concurrent probes per daemon?
   - **Recommendation**: No separate limit; probing is lightweight compared to transcoding
