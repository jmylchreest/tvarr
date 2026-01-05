---
title: ffmpegd Workers
description: Distributed transcoding daemons
sidebar_position: 2
---

# ffmpegd Workers

ffmpegd is tvarr's distributed transcoding service.

## What is ffmpegd?

ffmpegd (FFmpeg Daemon) is a standalone service that:

- Connects to tvarr's coordinator via gRPC
- Receives transcoding jobs
- Runs FFmpeg processes
- Streams output back to clients

## Why Use Separate Workers?

### Scale Horizontally

Run multiple workers on different machines:

```
tvarr (coordinator)
    │
    ├── ffmpegd (GPU server 1)
    ├── ffmpegd (GPU server 2)
    └── ffmpegd (CPU-only server)
```

### Dedicated Hardware

Put workers on machines with:

- GPUs for hardware encoding
- More CPU cores for software encoding
- Better cooling for heavy workloads

### Isolation

Keep transcoding separate from the main tvarr process:

- Transcoding crashes don't affect the coordinator
- Easier resource allocation
- Independent scaling

## Running ffmpegd

### Docker

```bash
docker run -d \
  --name ffmpegd \
  -e TVARR_COORDINATOR_URL=tvarr:9090 \
  -e TVARR_MAX_JOBS=4 \
  --device /dev/dri:/dev/dri \
  ghcr.io/jmylchreest/tvarr-transcoder:release
```

### Docker Compose

```yaml
services:
  ffmpegd:
    image: ghcr.io/jmylchreest/tvarr-transcoder:release
    environment:
      - TVARR_COORDINATOR_URL=tvarr:9090
      - TVARR_MAX_JOBS=4
      - TVARR_DAEMON_NAME=gpu-worker-1
    devices:
      - /dev/dri:/dev/dri
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TVARR_COORDINATOR_URL` | - | Coordinator gRPC address (required) |
| `TVARR_DAEMON_NAME` | hostname | Human-readable worker name |
| `TVARR_MAX_JOBS` | auto | Max concurrent transcoding jobs |
| `TVARR_MAX_CPU_JOBS` | auto | Max software encoding jobs |
| `TVARR_MAX_GPU_JOBS` | auto | Max hardware encoding jobs |
| `TVARR_AUTH_TOKEN` | - | Authentication token |
| `TVARR_LOGGING_LEVEL` | info | Log level |

### Auto-Detection

ffmpegd automatically detects:

- Available FFmpeg encoders/decoders
- GPU hardware and session limits
- CPU cores for software encoding

## Monitoring Workers

View connected workers at **Transcoders** in the web UI:

- Worker status (online/offline)
- Current jobs
- Capabilities (encoders, GPUs)
- Resource usage

## Job Distribution

The coordinator distributes jobs based on:

1. **Encoder requirements** - Does the worker have the needed encoder?
2. **Available capacity** - How many job slots are free?
3. **Hardware preference** - Prefer GPU over CPU when available

## Standalone Mode

Run ffmpegd in standalone mode for testing:

```bash
tvarr-ffmpegd serve --standalone
```

This starts the daemon without connecting to a coordinator, useful for verifying FFmpeg detection.
