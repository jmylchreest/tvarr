# tvarr-ffmpegd: Distributed FFmpeg Transcoding Daemon

## Overview

Extract the FFmpeg wrapper logic from tvarr into a standalone daemon (`tvarr-ffmpegd`) that handles all transcoding. tvarr becomes a pure coordinator/API server with no local FFmpeg dependency. At minimum, one tvarr-ffmpegd instance runs alongside tvarr (can be co-located or containerized separately). Additional remote instances can be deployed for distributed transcoding across machines with different hardware capabilities.

## Problem Statement

Currently, tvarr bundles FFmpeg and transcoding logic directly:

1. **Monolithic image**: Docker image includes FFmpeg and all encoder libraries, bloating size
2. **Hardware encoding constraints**: Users running tvarr on NAS devices or VMs may lack GPU/hardware encoding support
3. **Resource contention**: Transcoding competes with tvarr's other functions (ingestion, serving, database)
4. **Scaling limitations**: Cannot distribute transcoding load across multiple machines
5. **Update complexity**: FFmpeg updates require full tvarr rebuild

## Design Goals

1. **Clean separation**: tvarr = coordinator/API, tvarr-ffmpegd = transcoding worker
2. **Minimal tvarr image**: No FFmpeg, no encoder libraries - just the Go binary
3. **Environment-driven config**: `TVARR_COORDINATOR_URL` and similar for easy container orchestration
4. **Scale-out ready**: Run 1 to N tvarr-ffmpegd instances with automatic load distribution
5. **No backward compatibility**: Clean break from embedded FFmpeg

## Proposed Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              tvarr (coordinator)                         │
│  ┌─────────────────┐  ┌──────────────────┐  ┌────────────────────────┐  │
│  │  RelaySession   │  │  SharedESBuffer  │  │  FFmpegDaemonRegistry  │  │
│  │                 │  │  (source variant)│  │  - Track registered    │  │
│  │  - Ingest       │──│  - Video ES      │  │    daemons             │  │
│  │  - Session mgmt │  │  - Audio ES      │  │  - Capability matching │  │
│  └─────────────────┘  └────────┬─────────┘  │  - Load balancing      │  │
│                                │            └──────────┬─────────────┘  │
│                                │                       │                │
│                     ┌──────────▼───────────────────────▼──────┐         │
│                     │        RemoteTranscoderClient           │         │
│                     │  - Tunnel ES samples over network       │         │
│                     │  - Receive transcoded samples           │         │
│                     │  - Handle reconnection/failover         │         │
│                     └──────────────────┬──────────────────────┘         │
└────────────────────────────────────────┼────────────────────────────────┘
                                         │
                              gRPC/WebSocket
                              (bidirectional streaming)
                                         │
┌────────────────────────────────────────┼────────────────────────────────┐
│                         tvarr-ffmpegd (worker)                          │
│                                        │                                │
│  ┌─────────────────────────────────────▼─────────────────────────────┐  │
│  │                    TranscodeService (gRPC)                        │  │
│  │  - RegisterWithCoordinator()  - capabilities, heartbeat           │  │
│  │  - StartTranscode()           - bidirectional ES stream           │  │
│  │  - GetStats()                 - CPU, memory, encoding speed       │  │
│  └───────────────────────────────────────────────────────────────────┘  │
│                                        │                                │
│  ┌─────────────────────┐  ┌────────────▼────────────┐  ┌─────────────┐  │
│  │  FFmpeg Process     │  │  Local SharedESBuffer   │  │  Process    │  │
│  │  Manager            │◄─│  - Receives source ES   │  │  Monitor    │  │
│  │  - Spawn/kill       │  │  - Feeds to FFmpeg      │  │  - CPU/mem  │  │
│  │  - stdin/stdout     │  │  - Collects output ES   │  │  - Speed    │  │
│  │  - Retry logic      │  └─────────────────────────┘  └─────────────┘  │
│  └─────────────────────┘                                                │
│                                                                         │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │                    Capability Reporter                            │  │
│  │  - Detect available HW encoders (vaapi, nvenc, qsv, videotoolbox) │  │
│  │  - Report supported codecs (h264, h265, vp9, av1)                 │  │
│  │  - Measure encoding performance (benchmark on startup)            │  │
│  │  - Monitor GPU utilization                                        │  │
│  └───────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
```

## Design Components

### 1. Protocol Definition (protobuf/gRPC)

```protobuf
// ffmpegd.proto

service FFmpegDaemon {
  // Registration and discovery
  rpc Register(RegisterRequest) returns (RegisterResponse);
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
  rpc Unregister(UnregisterRequest) returns (UnregisterResponse);

  // Transcoding - bidirectional streaming
  rpc Transcode(stream TranscodeMessage) returns (stream TranscodeMessage);

  // Stats and monitoring
  rpc GetStats(GetStatsRequest) returns (GetStatsResponse);
  rpc GetProcessStats(GetProcessStatsRequest) returns (GetProcessStatsResponse);
}

message RegisterRequest {
  string daemon_id = 1;           // Unique daemon identifier
  string daemon_name = 2;         // Human-readable name
  string version = 3;             // tvarr-ffmpegd version
  Capabilities capabilities = 4;  // What this daemon can do
  string coordinator_url = 5;     // Where to connect back
}

message Capabilities {
  repeated HWAccelInfo hw_accels = 1;     // Available HW accelerators
  repeated string video_encoders = 2;      // e.g., libx264, h264_nvenc
  repeated string audio_encoders = 3;      // e.g., aac, libopus
  repeated string video_decoders = 4;      // e.g., h264, hevc
  int32 max_concurrent_jobs = 5;           // Max simultaneous transcodes
  PerformanceMetrics performance = 6;      // Benchmark results
}

message HWAccelInfo {
  string type = 1;        // vaapi, cuda, qsv, videotoolbox
  string device = 2;      // /dev/dri/renderD128, etc.
  bool available = 3;
  repeated string encoders = 4;
  repeated string decoders = 5;
}

// NOTE: tvarr-ffmpegd MUST use the same HWAccelDetector from internal/ffmpeg/hwaccel.go
// that tvarr uses internally. This ensures consistent capability detection across:
// - Local tvarr (when running without distributed daemons)
// - Each tvarr-ffmpegd instance reporting its capabilities
//
// The Capabilities message should map directly to internal/ffmpeg.HWAccelInfo:
// - type       -> HWAccelType (vaapi, cuda, qsv, videotoolbox, etc.)
// - device     -> DeviceName (/dev/dri/renderD128, GPU name, etc.)
// - available  -> Available (true if tested and working)
// - encoders   -> Encoders (h264_nvenc, hevc_vaapi, etc.)
// - decoders   -> Decoders (h264_cuvid, hevc_qsv, etc.)
//
// This allows the coordinator to make intelligent routing decisions based on
// which encoders/decoders each daemon actually supports.

message PerformanceMetrics {
  double h264_1080p_fps = 1;   // Frames/sec for H.264 1080p encode
  double h265_1080p_fps = 2;   // Frames/sec for H.265 1080p encode
  double memory_gb = 3;        // Available memory
  int32 cpu_cores = 4;         // CPU core count
}

// Bidirectional transcode stream message
message TranscodeMessage {
  oneof payload {
    TranscodeStart start = 1;         // Initial config (client → daemon)
    TranscodeAck ack = 2;             // Acknowledge start (daemon → client)
    ESSampleBatch samples = 3;        // ES samples (bidirectional)
    TranscodeStats stats = 4;         // Periodic stats (daemon → client)
    TranscodeError error = 5;         // Error notification
    TranscodeStop stop = 6;           // Stop signal
  }
}

message TranscodeStart {
  string job_id = 1;
  string session_id = 2;
  string channel_id = 3;

  // Source codec info
  string source_video_codec = 4;    // h264, hevc
  string source_audio_codec = 5;    // aac, ac3, eac3
  bytes video_init_data = 6;        // SPS/PPS for H.264
  bytes audio_init_data = 7;        // AudioSpecificConfig

  // Target codec info (from EncodingProfile)
  string target_video_codec = 8;
  string target_audio_codec = 9;
  string video_encoder = 10;        // libx264, h264_nvenc
  string audio_encoder = 11;        // aac, libopus

  // Encoding parameters
  int32 video_bitrate_kbps = 12;
  int32 audio_bitrate_kbps = 13;
  string video_preset = 14;         // ultrafast, fast, medium, slow
  string hw_accel = 15;             // Preferred HW accel (or empty for auto)
}

message ESSampleBatch {
  repeated ESSample video_samples = 1;
  repeated ESSample audio_samples = 2;
  bool is_source = 3;               // true = source samples, false = transcoded
}

message ESSample {
  int64 pts = 1;           // 90kHz timescale
  int64 dts = 2;           // 90kHz timescale
  bytes data = 3;          // NAL unit or audio frame
  bool is_keyframe = 4;
  uint64 sequence = 5;
}

message TranscodeStats {
  uint64 samples_in = 1;
  uint64 samples_out = 2;
  uint64 bytes_in = 3;
  uint64 bytes_out = 4;
  double encoding_speed = 5;     // 1.0 = realtime
  double cpu_percent = 6;
  double memory_mb = 7;
  int32 pid = 8;
}
```

### 2. tvarr Coordinator Changes

#### FFmpegDaemonRegistry

New component in tvarr to track and manage remote daemons:

```go
// internal/relay/daemon_registry.go

type DaemonRegistry struct {
    mu      sync.RWMutex
    daemons map[string]*DaemonInfo  // daemon_id -> info

    // Load balancing
    roundRobin atomic.Uint64
}

type DaemonInfo struct {
    ID           string
    Name         string
    Address      string
    Capabilities Capabilities

    // Health tracking
    LastHeartbeat time.Time
    ActiveJobs    int32

    // Connection
    conn   *grpc.ClientConn
    client FFmpegDaemonClient
}

func (r *DaemonRegistry) SelectDaemon(profile *EncodingProfile) (*DaemonInfo, error) {
    // Find daemon with required capabilities and lowest load
}

func (r *DaemonRegistry) RegisterDaemon(info *DaemonInfo) error
func (r *DaemonRegistry) UnregisterDaemon(daemonID string) error
func (r *DaemonRegistry) HandleHeartbeat(daemonID string) error
```

#### RemoteTranscoder

Replaces local `FFmpegTranscoder` when a remote daemon is available:

```go
// internal/relay/remote_transcoder.go

type RemoteTranscoder struct {
    id            string
    daemon        *DaemonInfo
    sourceVariant CodecVariant
    targetVariant CodecVariant
    buffer        *SharedESBuffer

    // gRPC stream
    stream FFmpegDaemon_TranscodeClient

    // Stats (mirroring local FFmpegTranscoder)
    samplesIn     atomic.Uint64
    samplesOut    atomic.Uint64
    bytesIn       atomic.Uint64
    bytesOut      atomic.Uint64
    encodingSpeed atomic.Value
}

// Implements same interface as FFmpegTranscoder
func (t *RemoteTranscoder) Start(ctx context.Context) error
func (t *RemoteTranscoder) Stop()
func (t *RemoteTranscoder) Stats() TranscoderStats
func (t *RemoteTranscoder) ProcessStats() *TranscoderProcessStats
```

### 3. tvarr-ffmpegd Daemon

#### Main Entry Point

```go
// cmd/tvarr-ffmpegd/main.go

func main() {
    cfg := LoadConfig()

    // Detect capabilities
    capabilities := DetectCapabilities()

    // Start gRPC server
    server := NewFFmpegDaemonServer(cfg, capabilities)

    // Register with coordinator(s)
    for _, coordinator := range cfg.Coordinators {
        go server.RegisterWithCoordinator(coordinator)
    }

    // Serve
    server.Serve()
}
```

#### Transcode Handler

```go
// internal/daemon/transcode_handler.go

func (s *Server) Transcode(stream grpc.BidiStreamingServer[TranscodeMessage]) error {
    // Receive start message
    msg, err := stream.Recv()
    start := msg.GetStart()

    // Create local SharedESBuffer for this job
    buffer := relay.NewSharedESBuffer(start.ChannelId, start.JobId, config)

    // Create source variant from init data
    buffer.CreateSourceVariant(start.SourceVideoCodec, start.SourceAudioCodec)
    buffer.SetVideoInitData(start.VideoInitData)
    buffer.SetAudioInitData(start.AudioInitData)

    // Create FFmpeg transcoder (local)
    transcoder := relay.CreateTranscoderFromConfig(...)
    transcoder.Start(ctx)

    // Goroutine: Read source samples from stream → write to buffer
    go func() {
        for {
            msg, err := stream.Recv()
            batch := msg.GetSamples()
            for _, sample := range batch.VideoSamples {
                buffer.WriteVideo(sample.Pts, sample.Dts, sample.Data, sample.IsKeyframe)
            }
            // ... audio samples
        }
    }()

    // Goroutine: Read transcoded samples from buffer → send to stream
    go func() {
        targetVariant := buffer.GetVariant(targetKey)
        for {
            samples := targetVariant.VideoTrack().ReadFrom(lastSeq, 100)
            batch := &ESSampleBatch{IsSource: false, VideoSamples: ...}
            stream.Send(&TranscodeMessage{Payload: &TranscodeMessage_Samples{batch}})
        }
    }()

    // Goroutine: Send periodic stats
    go func() {
        ticker := time.NewTicker(time.Second)
        for range ticker.C {
            stats := transcoder.Stats()
            procStats := transcoder.ProcessStats()
            stream.Send(&TranscodeMessage{Payload: &TranscodeMessage_Stats{...}})
        }
    }()
}
```

### 4. Configuration

Configuration is primarily environment-variable driven for container orchestration, with optional config file support.

#### Environment Variables

##### tvarr (coordinator)

| Variable | Default | Description |
|----------|---------|-------------|
| `TVARR_LISTEN_ADDRESS` | `0.0.0.0:8080` | HTTP API listen address |
| `TVARR_FFMPEGD_LISTEN_ADDRESS` | `0.0.0.0:9090` | gRPC address for daemon connections |
| `TVARR_FFMPEGD_STRATEGY` | `least-loaded` | Load balancing: `least-loaded`, `round-robin`, `capability-match` |
| `TVARR_FFMPEGD_SELECTION_TIMEOUT` | `5s` | Timeout waiting for daemon availability |
| `TVARR_FFMPEGD_TLS_CERT` | | Path to TLS certificate (optional) |
| `TVARR_FFMPEGD_TLS_KEY` | | Path to TLS key (optional) |
| `TVARR_FFMPEGD_AUTH_TOKEN` | | Shared token for daemon authentication |

##### tvarr-ffmpegd (worker)

| Variable | Default | Description |
|----------|---------|-------------|
| `TVARR_COORDINATOR_URL` | **required** | Coordinator gRPC address (e.g., `tvarr:9090`) |
| `TVARR_DAEMON_ID` | auto-generated | Unique daemon identifier |
| `TVARR_DAEMON_NAME` | hostname | Human-readable name for dashboard |
| `TVARR_LISTEN_ADDRESS` | `0.0.0.0:9091` | Health/metrics endpoint |
| `TVARR_AUTH_TOKEN` | | Token matching coordinator's `TVARR_FFMPEGD_AUTH_TOKEN` |
| `TVARR_FFMPEG_PATH` | `/usr/bin/ffmpeg` | Path to FFmpeg binary |
| `TVARR_FFPROBE_PATH` | `/usr/bin/ffprobe` | Path to FFprobe binary |
| `TVARR_MAX_JOBS` | `4` | Maximum concurrent transcode jobs |
| `TVARR_HW_ACCEL` | auto-detect | Force HW accel: `vaapi`, `cuda`, `qsv`, `videotoolbox` |
| `TVARR_HW_DEVICE` | auto-detect | HW device path (e.g., `/dev/dri/renderD128`) |
| `TVARR_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `TVARR_LOG_FORMAT` | `json` | Log format: `json`, `text` |

#### Config File (Optional)

For complex deployments, a config file can supplement environment variables:

##### tvarr (coordinator) - `config.yaml`

```yaml
ffmpegd:
  listen_address: "0.0.0.0:9090"
  strategy: "capability-match"
  selection_timeout: 5s

  # Static daemons (in addition to auto-registered ones)
  static_daemons:
    - address: "gpu-server-1.local:9091"
      name: "gpu-server-1"
    - address: "gpu-server-2.local:9091"
      name: "gpu-server-2"

  tls:
    cert_file: "/etc/tvarr/cert.pem"
    key_file: "/etc/tvarr/key.pem"

  auth:
    token: "${TVARR_FFMPEGD_AUTH_TOKEN}"  # Can reference env vars
```

##### tvarr-ffmpegd (worker) - `ffmpegd.yaml`

```yaml
coordinator:
  url: "${TVARR_COORDINATOR_URL}"
  token: "${TVARR_AUTH_TOKEN}"

daemon:
  id: "auto"
  name: "gpu-transcoder-1"

listen:
  address: "0.0.0.0:9091"

ffmpeg:
  path: "/usr/bin/ffmpeg"
  ffprobe_path: "/usr/bin/ffprobe"
  max_concurrent_jobs: 4
  hw_accel: "vaapi"
  hw_device: "/dev/dri/renderD128"

logging:
  level: "info"
  format: "json"
```

### 5. GPU Session Tracking & Cluster-Wide Load Balancing

Hardware encoders have **session limits** that must be tracked across the cluster:

#### The Problem

| GPU Type | Encode Sessions | Notes |
|----------|-----------------|-------|
| NVIDIA GeForce (consumer) | 3-5 | Artificial limit, can be patched but not recommended |
| NVIDIA Quadro/RTX Pro | Unlimited | Professional cards |
| NVIDIA Tesla/A-series | Unlimited | Datacenter cards |
| Intel QSV (iGPU) | 4-8 | Varies by generation, shared with display |
| AMD VCE/VCN | 4-8 | Varies by chip generation |
| Apple VideoToolbox | ~8 | Varies by chip, shared with system |

If a daemon tries to start an encode when all GPU sessions are in use, FFmpeg will fail with cryptic errors. The coordinator must track session availability **before** routing jobs.

#### Session Tracking Design

```go
// Coordinator tracks cluster-wide GPU state
type ClusterGPUState struct {
    mu     sync.RWMutex
    // daemon_id -> gpu_index -> GPUSessionState
    gpus   map[string]map[int]*GPUSessionState
}

type GPUSessionState struct {
    DaemonID          string
    GPUIndex          int
    GPUName           string
    GPUClass          GPUClass

    // Session limits (discovered at registration)
    MaxEncodeSessions int
    MaxDecodeSessions int

    // Current usage (updated via heartbeat)
    ActiveEncodeSessions int
    ActiveDecodeSessions int

    // Utilization metrics
    EncoderUtilization float64  // 0-100%
    MemoryUtilization  float64  // 0-100%
    Temperature        int      // Celsius

    LastUpdated time.Time
}

// Check if a GPU can accept a new encode job
func (s *GPUSessionState) CanAcceptEncodeJob() bool {
    // Hard limit check
    if s.ActiveEncodeSessions >= s.MaxEncodeSessions {
        return false
    }
    // Soft limit: avoid overloading (optional policy)
    if s.EncoderUtilization > 90 || s.MemoryUtilization > 90 {
        return false
    }
    if s.Temperature > 85 {
        return false  // Thermal throttling likely
    }
    return true
}
```

#### Session Limit Detection

At daemon startup, detect GPU session limits:

```go
// internal/daemon/gpu_limits.go

func DetectGPUSessionLimits(gpuIndex int, gpuName string) (maxEncode, maxDecode int) {
    // NVIDIA: Parse nvidia-smi or use NVML
    if strings.Contains(gpuName, "GeForce") {
        // Consumer cards: check driver version for patched limits
        // Default: 3 (older) or 5 (RTX 30/40 series)
        return detectNVIDIAConsumerLimits(gpuIndex)
    }
    if strings.Contains(gpuName, "Quadro") || strings.Contains(gpuName, "RTX A") {
        return 32, 32  // Effectively unlimited
    }
    if strings.Contains(gpuName, "Tesla") || strings.Contains(gpuName, "A100") {
        return 64, 64  // Datacenter - very high limits
    }

    // Intel QSV: Check generation
    if isIntelGPU(gpuName) {
        return detectIntelQSVLimits()
    }

    // AMD: Check VCN generation
    if isAMDGPU(gpuName) {
        return detectAMDVCNLimits()
    }

    // Default conservative limits
    return 4, 8
}

func detectNVIDIAConsumerLimits(gpuIndex int) (int, int) {
    // Try to query NVML for actual session count
    // Or: try to start sessions until we hit the limit (during capability detection)
    // Or: use known limits based on GPU model
    //
    // Known limits (as of 2024):
    // - GTX 10 series: 2 encode sessions
    // - GTX 16 series: 3 encode sessions
    // - RTX 20 series: 3 encode sessions
    // - RTX 30 series: 5 encode sessions
    // - RTX 40 series: 5 encode sessions
    return 5, 16  // Conservative default for RTX
}
```

#### Load Balancing with GPU Awareness

```go
// internal/relay/daemon_registry.go

type DaemonSelectionStrategy int

const (
    StrategyLeastLoaded DaemonSelectionStrategy = iota
    StrategyRoundRobin
    StrategyCapabilityMatch
    StrategyGPUAware  // NEW: Consider GPU session availability
)

func (r *DaemonRegistry) SelectDaemon(profile *EncodingProfile) (*DaemonInfo, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    needsHWAccel := profile.VideoEncoder != "" &&
        (strings.HasSuffix(profile.VideoEncoder, "_nvenc") ||
         strings.HasSuffix(profile.VideoEncoder, "_vaapi") ||
         strings.HasSuffix(profile.VideoEncoder, "_qsv"))

    var candidates []*DaemonInfo

    for _, daemon := range r.daemons {
        if !daemon.IsHealthy() {
            continue
        }

        if needsHWAccel {
            // Check if daemon has a GPU with available sessions
            gpu := r.findAvailableGPU(daemon, profile.VideoEncoder)
            if gpu == nil {
                continue  // No GPU with available sessions
            }
        }

        candidates = append(candidates, daemon)
    }

    if len(candidates) == 0 {
        if needsHWAccel {
            return nil, ErrNoGPUSessionsAvailable
        }
        return nil, ErrNoDaemonsAvailable
    }

    // Apply selection strategy among candidates
    return r.selectFromCandidates(candidates)
}

func (r *DaemonRegistry) findAvailableGPU(daemon *DaemonInfo, encoder string) *GPUSessionState {
    for _, gpu := range daemon.GPUs {
        if !gpu.CanAcceptEncodeJob() {
            continue
        }
        // Check encoder compatibility
        if strings.HasSuffix(encoder, "_nvenc") && gpu.GPUClass != GPU_CLASS_CONSUMER &&
           gpu.GPUClass != GPU_CLASS_PROFESSIONAL && gpu.GPUClass != GPU_CLASS_DATACENTER {
            continue
        }
        // ... similar checks for vaapi, qsv
        return gpu
    }
    return nil
}
```

#### Fallback to Software Encoding

When all GPU sessions are exhausted, the coordinator has options:

1. **Queue and wait**: Hold the job until a GPU session becomes available
2. **Fallback to software**: Route to a daemon with CPU capacity for libx264/libx265
3. **Reject**: Return error to client immediately

```go
type GPUExhaustedPolicy int

const (
    PolicyQueue    GPUExhaustedPolicy = iota  // Wait for GPU
    PolicyFallback                             // Use software encoder
    PolicyReject                               // Fail immediately
)

// Configurable per encoding profile
type EncodingProfile struct {
    // ... existing fields ...
    GPUExhaustedPolicy GPUExhaustedPolicy `json:"gpu_exhausted_policy"`
    FallbackEncoder    string             `json:"fallback_encoder,omitempty"` // e.g., "libx264"
}
```

#### Multi-GPU Support

A single daemon may have multiple GPUs. The daemon tracks sessions per-GPU and reports all:

```yaml
# Example: Server with 2 GPUs
# tvarr-ffmpegd reports:
capabilities:
  gpus:
    - index: 0
      name: "NVIDIA GeForce RTX 4090"
      max_encode_sessions: 5
      active_encode_sessions: 3
      encoders: [h264_nvenc, hevc_nvenc, av1_nvenc]
    - index: 1
      name: "NVIDIA GeForce RTX 3080"
      max_encode_sessions: 5
      active_encode_sessions: 5  # FULL
      encoders: [h264_nvenc, hevc_nvenc]
```

The coordinator can route to GPU 0 (has 2 available sessions) but not GPU 1 (full).

### 6. Network Protocol Considerations

#### Efficient Sample Transport

ES samples need efficient binary encoding for network transfer:

1. **Batching**: Group multiple samples per message (reduce RPC overhead)
2. **Compression**: Optional zstd compression for sample data (configurable)
3. **Flow control**: Backpressure handling when network is slow

```go
// Batching strategy
const (
    MaxBatchSize    = 100          // Max samples per batch
    MaxBatchBytes   = 1 * 1024 * 1024  // 1MB max per batch
    FlushInterval   = 10 * time.Millisecond
)
```

#### Reconnection Handling

```go
type RemoteTranscoder struct {
    // Reconnection state
    reconnecting atomic.Bool
    reconnectCh  chan struct{}

    // Sample buffer during reconnection
    pendingSamples []ESSample
    pendingMu      sync.Mutex
}

func (t *RemoteTranscoder) handleDisconnect() {
    // Buffer samples locally during reconnection
    // Attempt reconnection with exponential backoff
    // Resume from last acknowledged sequence
}
```

### 6. Monitoring, Telemetry & Dashboard

#### System Telemetry from Daemons

Each tvarr-ffmpegd reports comprehensive system metrics to the coordinator:

```protobuf
message SystemStats {
  // Host identification
  string hostname = 1;
  string os = 2;              // linux, darwin, windows
  string arch = 3;            // amd64, arm64
  int64 uptime_seconds = 4;

  // CPU
  int32 cpu_cores = 5;
  double cpu_percent = 6;           // Overall CPU usage (0-100)
  repeated double cpu_per_core = 7; // Per-core usage
  double load_avg_1m = 8;
  double load_avg_5m = 9;
  double load_avg_15m = 10;

  // Memory
  uint64 memory_total_bytes = 11;
  uint64 memory_used_bytes = 12;
  uint64 memory_available_bytes = 13;
  double memory_percent = 14;
  uint64 swap_total_bytes = 15;
  uint64 swap_used_bytes = 16;

  // Disk (for temp/work directory)
  uint64 disk_total_bytes = 17;
  uint64 disk_used_bytes = 18;
  uint64 disk_available_bytes = 19;
  double disk_percent = 20;

  // Network
  uint64 network_bytes_sent = 21;
  uint64 network_bytes_recv = 22;
  double network_send_rate_bps = 23;
  double network_recv_rate_bps = 24;

  // GPU (if available)
  repeated GPUStats gpus = 25;

  // Pressure indicators (Linux PSI)
  PressureStats cpu_pressure = 26;
  PressureStats memory_pressure = 27;
  PressureStats io_pressure = 28;
}

message GPUStats {
  int32 index = 1;
  string name = 2;               // e.g., "NVIDIA GeForce RTX 3080"
  string driver_version = 3;
  double utilization_percent = 4;
  double memory_percent = 5;
  uint64 memory_total_bytes = 6;
  uint64 memory_used_bytes = 7;
  int32 temperature_celsius = 8;
  int32 power_watts = 9;
  int32 encoder_utilization = 10; // NVENC utilization %
  int32 decoder_utilization = 11; // NVDEC utilization %

  // Session tracking - CRITICAL for proper load balancing
  int32 max_encode_sessions = 12;     // Hardware limit (NVIDIA consumer: 3-5, Quadro: unlimited)
  int32 active_encode_sessions = 13;  // Currently in use by this daemon
  int32 max_decode_sessions = 14;     // Usually higher than encode
  int32 active_decode_sessions = 15;  // Currently in use by this daemon

  // GPU type classification for session limit detection
  GPUClass gpu_class = 16;
}

enum GPUClass {
  GPU_CLASS_UNKNOWN = 0;
  GPU_CLASS_CONSUMER = 1;      // GeForce, Radeon - limited encode sessions
  GPU_CLASS_PROFESSIONAL = 2;  // Quadro, Pro - unlimited or high limits
  GPU_CLASS_DATACENTER = 3;    // Tesla, A100, etc. - no artificial limits
  GPU_CLASS_INTEGRATED = 4;    // Intel iGPU, AMD APU - shared memory, varies
}

message PressureStats {
  double avg10 = 1;   // 10-second average
  double avg60 = 2;   // 60-second average
  double avg300 = 3;  // 5-minute average
  uint64 total_us = 4; // Total stall time in microseconds
}
```

#### Heartbeat with Telemetry

Daemons send periodic heartbeats (every 5s) including system stats:

```protobuf
message HeartbeatRequest {
  string daemon_id = 1;
  SystemStats system_stats = 2;
  repeated JobStatus active_jobs = 3;
}

message JobStatus {
  string job_id = 1;
  string session_id = 2;
  string channel_name = 3;
  TranscodeStats stats = 4;
  Duration running_time = 5;
}
```

#### Metrics (Prometheus)

```go
// Coordinator metrics
tvarr_ffmpegd_daemons_total{status="active|inactive|unhealthy"}
tvarr_ffmpegd_daemon_info{daemon_id, name, version, hw_accel}
tvarr_ffmpegd_jobs_total{daemon_id, status="running|completed|failed"}
tvarr_ffmpegd_samples_transferred_total{daemon_id, direction="in|out"}
tvarr_ffmpegd_bytes_transferred_total{daemon_id, direction="in|out"}
tvarr_ffmpegd_latency_seconds{daemon_id, quantile="0.5|0.9|0.99"}

// Per-daemon system metrics (from heartbeat)
tvarr_ffmpegd_host_cpu_percent{daemon_id}
tvarr_ffmpegd_host_memory_percent{daemon_id}
tvarr_ffmpegd_host_disk_percent{daemon_id}
tvarr_ffmpegd_host_load_avg{daemon_id, window="1m|5m|15m"}
tvarr_ffmpegd_host_pressure{daemon_id, resource="cpu|memory|io", window="10|60|300"}

// GPU metrics
tvarr_ffmpegd_gpu_utilization_percent{daemon_id, gpu_index, gpu_name}
tvarr_ffmpegd_gpu_memory_percent{daemon_id, gpu_index}
tvarr_ffmpegd_gpu_encoder_percent{daemon_id, gpu_index}
tvarr_ffmpegd_gpu_temperature_celsius{daemon_id, gpu_index}

// Per-job metrics
tvarr_ffmpegd_job_encoding_speed{daemon_id, job_id, codec}
tvarr_ffmpegd_job_cpu_percent{daemon_id, job_id}
tvarr_ffmpegd_job_memory_bytes{daemon_id, job_id}
```

#### Dashboard Integration

New "Transcoders" page in tvarr frontend showing:

##### Daemon Overview Panel
- List of all connected daemons with status badges (active/inactive/unhealthy)
- Quick stats: total daemons, total active jobs, aggregate encoding capacity
- Auto-refresh every 5 seconds

##### Per-Daemon Detail Cards
```
┌─────────────────────────────────────────────────────────────────────┐
│ gpu-server-1                                        ● Connected     │
│ 192.168.1.100:9091 | Version 0.1.0 | Uptime 3d 4h                  │
├─────────────────────────────────────────────────────────────────────┤
│ CAPABILITIES                                                        │
│ ├─ HW Accel: NVIDIA CUDA (RTX 3080)                                │
│ ├─ Encoders: h264_nvenc, hevc_nvenc, av1_nvenc                     │
│ └─ Max Jobs: 8                                                      │
├─────────────────────────────────────────────────────────────────────┤
│ SYSTEM RESOURCES                                                    │
│ CPU [████████░░░░░░░░] 48%    Load: 2.4 / 1.8 / 1.2                │
│ MEM [██████░░░░░░░░░░] 38%    12.2 GB / 32 GB                      │
│ DSK [███░░░░░░░░░░░░░] 22%    440 GB / 2 TB                        │
│ GPU [██████████░░░░░░] 65%    NVENC: 72%  Temp: 68°C               │
├─────────────────────────────────────────────────────────────────────┤
│ PRESSURE (10s avg)                                                  │
│ CPU: 0.12%   Memory: 0.00%   I/O: 0.05%                            │
├─────────────────────────────────────────────────────────────────────┤
│ ACTIVE JOBS (3/8)                                                   │
│ ├─ ESPN HD → h265/aac         1.2x realtime   CPU: 15%             │
│ ├─ NBC Sports → h264/aac      2.1x realtime   CPU: 8%              │
│ └─ HBO Max → h265/opus        0.9x realtime   CPU: 22%             │
├─────────────────────────────────────────────────────────────────────┤
│ HISTORY (30 min)                                                    │
│ CPU  ▂▃▅▆▅▄▃▂▂▃▄▅▆▇▆▅▄▃▂▁▂▃▄▅▄▃▂▁▂▃                                │
│ GPU  ▅▆▇█▇▆▅▄▅▆▇█▇▆▅▄▃▄▅▆▇▆▅▄▃▂▃▄▅                                │
│ Jobs ▂▂▃▃▃▃▂▂▃▃▃▄▄▃▃▃▂▂▂▃▃▃▃▃▃▂▂▂▃                                │
└─────────────────────────────────────────────────────────────────────┘
```

##### Registration/Setup Panel
- Display coordinator connection URL for copy/paste
- Show auth token (masked, with reveal button)
- Example docker run command:
  ```
  docker run -d \
    -e TVARR_COORDINATOR_URL=192.168.1.50:9090 \
    -e TVARR_AUTH_TOKEN=xxx \
    --gpus all \
    ghcr.io/jmylchreest/tvarr-ffmpegd:nvidia
  ```
- QR code for mobile/quick setup (encodes connection URL + token)

##### Health & Alerts
- Daemon health indicators:
  - **Healthy**: Connected, heartbeat received within 15s
  - **Degraded**: High pressure (>50%), high temperature, job failures
  - **Unhealthy**: No heartbeat for 30s, repeated job failures
- Alert conditions (shown as notifications):
  - Daemon disconnected
  - High memory/CPU pressure
  - GPU temperature above threshold
  - Encoding speed below 1.0x (falling behind realtime)
  - Disk space low on work directory

##### API Endpoints

New endpoints for frontend:

```
GET  /api/v1/ffmpegd/daemons              # List all daemons with status
GET  /api/v1/ffmpegd/daemons/{id}         # Get daemon details
GET  /api/v1/ffmpegd/daemons/{id}/jobs    # Get daemon's active jobs
GET  /api/v1/ffmpegd/daemons/{id}/stats   # Get daemon's system stats history
POST /api/v1/ffmpegd/daemons/{id}/drain   # Stop accepting new jobs (graceful)
DEL  /api/v1/ffmpegd/daemons/{id}         # Force disconnect daemon

GET  /api/v1/ffmpegd/registration         # Get registration info (URL, token)
POST /api/v1/ffmpegd/registration/token   # Regenerate auth token
```

### 7. Security Considerations

1. **Authentication**: mTLS between coordinator and daemons
2. **Authorization**: Registration tokens, job-level access control
3. **Network isolation**: Daemons should only accept connections from known coordinators
4. **Input validation**: Validate all protobuf messages, sanitize FFmpeg arguments

### 8. Container Architecture

#### Image Strategy

**tvarr** (coordinator):
- Base: `alpine:latest` or `scratch`
- Contents: Single `tvarr` binary, frontend assets
- Size target: ~50MB
- No FFmpeg, no encoder libraries

**tvarr-ffmpegd** (worker):
- Base: `ubuntu:24.04` or custom with FFmpeg
- Contents: `tvarr-ffmpegd` binary + FFmpeg + encoder libs
- Variants:
  - `tvarr-ffmpegd:latest` - Software encoders only (libx264, libx265, etc.)
  - `tvarr-ffmpegd:vaapi` - Intel/AMD VA-API support
  - `tvarr-ffmpegd:nvidia` - NVIDIA NVENC support (requires nvidia-container-toolkit)
  - `tvarr-ffmpegd:full` - All encoders (largest image)
- Size: 200-500MB depending on variant

#### Docker Compose Examples

##### Minimal Setup (Single Host)

```yaml
# docker-compose.yml
services:
  tvarr:
    image: ghcr.io/jmylchreest/tvarr:latest
    ports:
      - "8080:8080"
    volumes:
      - tvarr-data:/data
    environment:
      - TVARR_FFMPEGD_LISTEN_ADDRESS=0.0.0.0:9090
      - TVARR_FFMPEGD_AUTH_TOKEN=${TVARR_AUTH_TOKEN}
    depends_on:
      - ffmpegd

  ffmpegd:
    image: ghcr.io/jmylchreest/tvarr-ffmpegd:latest
    environment:
      - TVARR_COORDINATOR_URL=tvarr:9090
      - TVARR_AUTH_TOKEN=${TVARR_AUTH_TOKEN}
      - TVARR_DAEMON_NAME=local-transcoder

volumes:
  tvarr-data:
```

##### With NVIDIA GPU

```yaml
# docker-compose.nvidia.yml
services:
  tvarr:
    image: ghcr.io/jmylchreest/tvarr:latest
    ports:
      - "8080:8080"
    volumes:
      - tvarr-data:/data
    environment:
      - TVARR_FFMPEGD_LISTEN_ADDRESS=0.0.0.0:9090
      - TVARR_FFMPEGD_AUTH_TOKEN=${TVARR_AUTH_TOKEN}

  ffmpegd:
    image: ghcr.io/jmylchreest/tvarr-ffmpegd:nvidia
    environment:
      - TVARR_COORDINATOR_URL=tvarr:9090
      - TVARR_AUTH_TOKEN=${TVARR_AUTH_TOKEN}
      - TVARR_DAEMON_NAME=nvidia-transcoder
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 1
              capabilities: [gpu]

volumes:
  tvarr-data:
```

##### With Intel VA-API (iGPU)

```yaml
# docker-compose.vaapi.yml
services:
  tvarr:
    image: ghcr.io/jmylchreest/tvarr:latest
    ports:
      - "8080:8080"
    volumes:
      - tvarr-data:/data
    environment:
      - TVARR_FFMPEGD_LISTEN_ADDRESS=0.0.0.0:9090
      - TVARR_FFMPEGD_AUTH_TOKEN=${TVARR_AUTH_TOKEN}

  ffmpegd:
    image: ghcr.io/jmylchreest/tvarr-ffmpegd:vaapi
    environment:
      - TVARR_COORDINATOR_URL=tvarr:9090
      - TVARR_AUTH_TOKEN=${TVARR_AUTH_TOKEN}
      - TVARR_DAEMON_NAME=vaapi-transcoder
      - TVARR_HW_DEVICE=/dev/dri/renderD128
    devices:
      - /dev/dri:/dev/dri
    group_add:
      - video
      - render

volumes:
  tvarr-data:
```

##### Distributed Setup (Multiple Transcoders)

```yaml
# docker-compose.distributed.yml
services:
  tvarr:
    image: ghcr.io/jmylchreest/tvarr:latest
    ports:
      - "8080:8080"
      - "9090:9090"  # Expose for remote daemons
    volumes:
      - tvarr-data:/data
    environment:
      - TVARR_FFMPEGD_LISTEN_ADDRESS=0.0.0.0:9090
      - TVARR_FFMPEGD_STRATEGY=capability-match
      - TVARR_FFMPEGD_AUTH_TOKEN=${TVARR_AUTH_TOKEN}

  # Local software transcoder (always available)
  ffmpegd-local:
    image: ghcr.io/jmylchreest/tvarr-ffmpegd:latest
    environment:
      - TVARR_COORDINATOR_URL=tvarr:9090
      - TVARR_AUTH_TOKEN=${TVARR_AUTH_TOKEN}
      - TVARR_DAEMON_NAME=local-software
      - TVARR_MAX_JOBS=2

  # GPU transcoder on same host
  ffmpegd-gpu:
    image: ghcr.io/jmylchreest/tvarr-ffmpegd:nvidia
    environment:
      - TVARR_COORDINATOR_URL=tvarr:9090
      - TVARR_AUTH_TOKEN=${TVARR_AUTH_TOKEN}
      - TVARR_DAEMON_NAME=local-nvidia
      - TVARR_MAX_JOBS=8
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 1
              capabilities: [gpu]

volumes:
  tvarr-data:

# Remote transcoders connect via:
# docker run -e TVARR_COORDINATOR_URL=tvarr-host:9090 \
#            -e TVARR_AUTH_TOKEN=xxx \
#            ghcr.io/jmylchreest/tvarr-ffmpegd:vaapi
```

##### Kubernetes Deployment

```yaml
# k8s/tvarr-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tvarr
spec:
  replicas: 1
  selector:
    matchLabels:
      app: tvarr
  template:
    metadata:
      labels:
        app: tvarr
    spec:
      containers:
        - name: tvarr
          image: ghcr.io/jmylchreest/tvarr:latest
          ports:
            - containerPort: 8080
            - containerPort: 9090
          env:
            - name: TVARR_FFMPEGD_LISTEN_ADDRESS
              value: "0.0.0.0:9090"
            - name: TVARR_FFMPEGD_AUTH_TOKEN
              valueFrom:
                secretKeyRef:
                  name: tvarr-secrets
                  key: auth-token
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: tvarr-ffmpegd
spec:
  selector:
    matchLabels:
      app: tvarr-ffmpegd
  template:
    metadata:
      labels:
        app: tvarr-ffmpegd
    spec:
      containers:
        - name: ffmpegd
          image: ghcr.io/jmylchreest/tvarr-ffmpegd:vaapi
          env:
            - name: TVARR_COORDINATOR_URL
              value: "tvarr-service:9090"
            - name: TVARR_AUTH_TOKEN
              valueFrom:
                secretKeyRef:
                  name: tvarr-secrets
                  key: auth-token
            - name: TVARR_DAEMON_NAME
              valueFrom:
                fieldRef:
                  fieldPath: spec.nodeName
          securityContext:
            privileged: true  # For GPU access
          volumeMounts:
            - name: dri
              mountPath: /dev/dri
      volumes:
        - name: dri
          hostPath:
            path: /dev/dri
```

### 9. Implementation Roadmap

Since backward compatibility is not required, implementation follows a clean-slate approach:

#### Phase 1: Core Protocol & Daemon
1. Define protobuf service in `api/proto/ffmpegd.proto`
2. Implement `tvarr-ffmpegd` daemon with:
   - gRPC server
   - FFmpeg process management (reuse existing wrapper)
   - **Capability detection using `internal/ffmpeg.HWAccelDetector`** - same detection logic as tvarr
   - Coordinator registration
3. Basic transcode flow (no streaming yet - request/response)

**Important**: The capability detection MUST reuse `internal/ffmpeg/hwaccel.go`:
- `HWAccelDetector.Detect()` runs `ffmpeg -hwaccels` and tests each accelerator
- Tests actual encoder availability (h264_nvenc, hevc_vaapi, etc.)
- Reports device names (/dev/dri/renderD128, GPU names, etc.)
- This ensures consistent capability reporting between local tvarr and remote ffmpegd instances

#### Phase 2: Streaming Integration
1. Implement bidirectional ES sample streaming
2. Add `RemoteTranscoder` to tvarr (replaces `FFmpegTranscoder`)
3. Remove local FFmpeg dependency from tvarr
4. Add `DaemonRegistry` for tracking connected workers

#### Phase 3: Telemetry & Monitoring
1. System stats collection in daemon (CPU/mem/disk/network)
2. GPU monitoring (nvidia-smi, intel_gpu_top integration)
3. Linux PSI pressure indicators
4. Heartbeat with telemetry payload
5. Prometheus metrics export

#### Phase 4: Frontend Dashboard
1. New "Transcoders" page in frontend
2. Daemon list with status badges
3. Per-daemon detail cards (capabilities, resources, jobs)
4. Sparkline history graphs (CPU, GPU, job count)
5. Registration panel with connection info
6. Health alerts and notifications

#### Phase 5: Production Features
1. Load balancing strategies (least-loaded, capability-match)
2. Reconnection handling with sample buffering
3. TLS/authentication
4. Health checks and circuit breakers
5. Graceful drain for maintenance

#### Phase 6: Container & Distribution
1. Split Dockerfile into tvarr and tvarr-ffmpegd
2. Create image variants (software, vaapi, nvidia)
3. Docker Compose examples
4. Kubernetes manifests
5. Documentation and migration guide

## File Structure

```
cmd/
  tvarr/              # Existing CLI
  tvarr-ffmpegd/      # New daemon CLI
    main.go
    cmd/
      root.go
      serve.go
      register.go

internal/
  daemon/             # tvarr-ffmpegd internals
    server.go         # gRPC server
    transcode.go      # Transcode job handler
    capabilities.go   # HW detection
    registration.go   # Coordinator registration

  relay/
    daemon_registry.go    # Track remote daemons
    remote_transcoder.go  # Remote transcoder client
    transcoder_factory.go # Select local vs remote

  proto/              # Generated protobuf code
    ffmpegd.pb.go
    ffmpegd_grpc.pb.go

api/
  proto/
    ffmpegd.proto     # Service definitions
```

## Implementation Estimate

| Component | Complexity | Notes |
|-----------|------------|-------|
| Protocol definition (protobuf) | Low | Well-defined based on existing types |
| tvarr-ffmpegd daemon | Medium | Reuses existing FFmpeg wrapper |
| System telemetry collection | Medium | CPU/mem/disk/GPU stats, PSI pressure |
| Coordinator gRPC server | Medium | New listener, straightforward |
| DaemonRegistry | Medium | New component, straightforward |
| RemoteTranscoder | High | Network streaming, reconnection |
| Load balancing | Medium | Multiple strategies |
| Remove FFmpeg from tvarr | Medium | Extract/refactor existing code |
| Frontend - Transcoders page | Medium | New page, daemon cards, sparklines |
| Frontend - Registration panel | Low | URL/token display, docker command |
| API endpoints | Low | Standard CRUD + stats |
| Container split | Medium | New Dockerfiles, CI changes |
| TLS/Auth | Medium | Standard gRPC patterns |
| Testing | High | Integration tests, network simulation |

**Total estimate**: 6-8 weeks for full implementation

## Open Questions

1. **Protocol choice**: gRPC vs WebSocket?
   - gRPC: Better tooling, streaming support, code generation, but HTTP/2 overhead
   - WebSocket: Simpler, works through more proxies/load balancers
   - **Recommendation**: gRPC - the tooling and bidirectional streaming support outweigh HTTP/2 overhead

2. **Sample compression**: Compress ES samples before transport?
   - Pro: Reduces bandwidth for remote transcoders
   - Con: Adds latency, CPU usage on both ends
   - **Recommendation**: Optional, off by default for local, configurable for WAN

3. **Multi-coordinator support**: Should one daemon serve multiple tvarr instances?
   - Use case: High availability, shared transcoding pool across tvarr clusters
   - **Recommendation**: Single coordinator initially, multi-coordinator as future enhancement

4. **Daemon discovery**: How do remote daemons find the coordinator?
   - Static config (`TVARR_COORDINATOR_URL`) - simple, explicit
   - DNS-SD/mDNS - auto-discovery on local network
   - **Recommendation**: Static config required, optional mDNS for LAN convenience

5. **Failure behavior**: What happens when all daemons are unavailable?
   - Block and wait for daemon
   - Return error to client
   - **Recommendation**: Configurable timeout, then error - tvarr cannot transcode without daemons

6. **Probing**: Should tvarr or tvarr-ffmpegd run ffprobe for stream analysis?
   - tvarr: Keeps all logic in coordinator
   - tvarr-ffmpegd: Daemon already has FFmpeg, reduces network calls
   - **Recommendation**: tvarr-ffmpegd handles probing, reports results to coordinator

7. **GPU session exhaustion policy**: What happens when all GPU encode sessions are in use?
   - Queue: Hold job until a session becomes available (may delay stream start)
   - Fallback: Use software encoder (higher CPU, but works)
   - Reject: Fail immediately (client sees error)
   - **Recommendation**: Configurable per-profile, default to fallback with warning

8. **GPU sharing between containers**: Can multiple ffmpegd containers share one GPU?
   - Yes, but must coordinate session limits across containers
   - NVIDIA MPS (Multi-Process Service) can help with sharing
   - Kubernetes device plugins can partition GPUs
   - **Recommendation**: Support GPU sharing but track sessions per-GPU globally, not per-daemon.
     This requires daemons to report which physical GPU they're using (by PCI ID or similar)
     so the coordinator can aggregate session counts across multiple daemons sharing a GPU.

9. **Session limit detection accuracy**: How to determine actual GPU session limits?
   - NVIDIA: NVML API can query, or parse nvidia-smi, or use known model limits
   - Intel QSV: No standard API, use known generation limits
   - AMD VCN: Parse rocm-smi or use known limits
   - Option: Let user override via config (`TVARR_GPU_MAX_SESSIONS=8`)
   - **Recommendation**: Auto-detect with model database, allow user override
