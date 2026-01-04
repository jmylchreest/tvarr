---
title: Distributed Transcoding
description: Scaling transcoding across multiple workers
sidebar_position: 3
---

# Distributed Transcoding

Scale transcoding across multiple machines with ffmpegd workers.

## Architecture

```
                    ┌─────────────────────────┐
                    │   tvarr (Coordinator)   │
                    │                         │
                    │  - Session management   │
                    │  - Job scheduling       │
                    │  - Client routing       │
                    └───────────┬─────────────┘
                                │ gRPC
          ┌─────────────────────┼─────────────────────┐
          │                     │                     │
          ▼                     ▼                     ▼
    ┌──────────┐          ┌──────────┐          ┌──────────┐
    │ ffmpegd  │          │ ffmpegd  │          │ ffmpegd  │
    │ Worker 1 │          │ Worker 2 │          │ Worker 3 │
    │ (NVIDIA) │          │ (Intel)  │          │  (CPU)   │
    └──────────┘          └──────────┘          └──────────┘
```

## Communication Flow

### Worker Registration

```
1. Worker starts, detects FFmpeg capabilities
2. Worker connects to coordinator via gRPC
3. Worker registers with capabilities:
   - Available encoders (h264_nvenc, hevc_vaapi, etc.)
   - GPU info (if any)
   - Max concurrent jobs
4. Coordinator adds worker to pool
```

### Job Dispatch

```
1. Client requests transcoded stream
2. Coordinator checks if variant exists
3. If not, coordinator creates transcoding job
4. Coordinator selects best worker:
   - Has required encoder
   - Has available capacity
   - Prefers GPU over CPU
5. Job dispatched to worker via gRPC stream
6. Worker spawns FFmpeg
7. Transcoded data streamed back to coordinator
8. Coordinator stores in shared buffer
```

### Failover

If a worker disconnects:

1. Active jobs are marked failed
2. Coordinator reassigns to another worker
3. Clients may experience brief interruption
4. Stream resumes from new worker

## Worker Selection

The coordinator selects workers based on:

### 1. Encoder Availability

Worker must have the required encoder:

```
Job needs: hevc_vaapi
Worker 1: [h264_nvenc, hevc_nvenc] ❌
Worker 2: [h264_vaapi, hevc_vaapi] ✓
Worker 3: [libx264, libx265] ❌
```

### 2. Capacity

Workers report max jobs and current jobs:

```
Worker 1: 3/4 jobs (available)
Worker 2: 4/4 jobs (full) ❌
Worker 3: 0/8 jobs (available)
```

### 3. Hardware Preference

GPU encoders preferred over CPU:

```
Worker 1: hevc_nvenc (GPU) ← preferred
Worker 3: libx265 (CPU) ← fallback
```

## Scaling Strategies

### Horizontal Scaling

Add more workers for more concurrent streams:

```yaml
# Scale up workers
docker compose up -d --scale ffmpegd=5
```

### Heterogeneous Hardware

Mix worker types for flexibility:

```yaml
ffmpegd-gpu:
  image: ghcr.io/jmylchreest/tvarr-transcoder:latest
  deploy:
    resources:
      reservations:
        devices:
          - driver: nvidia
            count: 1
            capabilities: [gpu, video]

ffmpegd-cpu:
  image: ghcr.io/jmylchreest/tvarr-transcoder:latest
  environment:
    - TVARR_MAX_JOBS=8
  deploy:
    resources:
      limits:
        cpus: '8'
```

### Geographic Distribution

Workers can be anywhere with network access:

```yaml
# Remote worker in different datacenter
TVARR_COORDINATOR_URL=coordinator.example.com:9090
```

## Monitoring

### Worker Status

View worker status in the **Transcoders** page:

- Online/offline status
- Current jobs
- Capabilities
- Resource usage

### Metrics

Workers report:

- Jobs completed/failed
- Current encoding sessions
- GPU utilization (if available)

## Configuration

### Coordinator

```bash
# Enable gRPC for worker connections
TVARR_GRPC_ENABLED=true
TVARR_GRPC_PORT=9090

# Optional: require authentication
TVARR_GRPC_AUTH_TOKEN=your-secret-token
```

### Workers

```bash
# Connect to coordinator
TVARR_COORDINATOR_URL=coordinator:9090

# Authentication (must match coordinator)
TVARR_AUTH_TOKEN=your-secret-token

# Resource limits
TVARR_MAX_JOBS=4
TVARR_MAX_GPU_JOBS=2
TVARR_MAX_CPU_JOBS=4
```
