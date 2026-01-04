---
title: Relay Architecture
description: How the streaming relay and shared buffer work
sidebar_position: 2
---

# Relay Architecture

The relay system handles real-time stream processing and transcoding.

## Overview

```
                    ┌─────────────────────────────────────┐
                    │         Shared ES Buffer            │
                    │  ┌──────────┐    ┌──────────┐      │
Origin Stream ──────┼─▶│  Source  │    │  Target  │      │
                    │  │ Variant  │    │ Variant  │      │
                    │  │ (h264)   │    │  (vp9)   │      │
                    │  └────┬─────┘    └────┬─────┘      │
                    │       │               │            │
                    └───────┼───────────────┼────────────┘
                            │               │
              ┌─────────────┼───────────────┼─────────────┐
              │             ▼               ▼             │
              │     ┌─────────────┐  ┌─────────────┐     │
              │     │  Processor  │  │  Processor  │     │
              │     │  (HLS-TS)   │  │   (DASH)    │     │
              │     └──────┬──────┘  └──────┬──────┘     │
              │            │                │            │
              └────────────┼────────────────┼────────────┘
                           ▼                ▼
                       Clients          Clients
```

## Components

### Session Manager

Manages streaming sessions for each channel:

- Creates sessions on first request
- Shares sessions across clients
- Cleans up idle sessions

### Shared Elementary Stream (ES) Buffer

The core of efficient multi-client streaming:

- **Single origin connection** - Only one connection to upstream per channel
- **Multiple variants** - Source codec + transcoded variants
- **Ring buffers** - Efficient memory usage with fixed-size buffers
- **On-demand transcoding** - Variants created when first requested

### Variants

A variant is a codec combination (video + audio):

```
Source Variant:  h264 + aac (from origin)
Target Variant:  vp9 + opus (transcoded on demand)
```

Variants are created when:
1. A client requests a codec the source doesn't provide
2. An encoding profile requires transcoding

### Processors

Format-specific output handlers:

| Processor | Output Format | Use Case |
|-----------|---------------|----------|
| `processor_hls_ts.go` | HLS + MPEG-TS segments | Wide compatibility |
| `processor_hls_fmp4.go` | HLS + fMP4 segments | Modern players |
| `processor_dash.go` | DASH MPD + fMP4 | Web players |
| `processor_mpegts.go` | Continuous TS | Legacy players |

### Format Router

Routes client requests to appropriate processors:

1. Parses client request (headers, query params)
2. Determines preferred codec/format
3. Selects or creates appropriate variant
4. Returns formatted stream

## Data Flow

### Client Requests Stream

```
1. Client: GET /relay/channel/123/stream?format=dash
2. Router: Parse request, detect client capabilities
3. Manager: Get or create session for channel 123
4. Session: Connect to origin if not connected
5. Buffer: Store source ES data in ring buffer
6. Variant: Check if client's codec variant exists
7. Transcoder: If needed, spawn FFmpeg for transcoding
8. Processor: Format data as DASH segments
9. Client: Receive DASH MPD + segments
```

### Multiple Clients, Same Channel

```
Client A (wants HLS-TS, h264) ──┐
                                │     ┌─────────┐
Client B (wants DASH, vp9) ─────┼────▶│ Session │──▶ Origin
                                │     └─────────┘
Client C (wants HLS-fMP4, h264)─┘         │
                                          ▼
                                   Shared Buffer
                                    (one copy)
```

All clients share the same origin connection and buffer.

## Memory Management

### Ring Buffers

Each variant maintains ring buffers:

```go
VideoBuffer: 1000 samples (configurable)
AudioBuffer: 2000 samples (configurable)
```

Old samples are overwritten when buffers fill.

### Idle Cleanup

Resources are cleaned up when idle:

| Resource | Idle Timeout |
|----------|--------------|
| Client connection | 10s |
| Variant | 60s |
| Session | 60s |
| Transcoder process | Immediate on variant cleanup |

## Transcoding Integration

When transcoding is needed:

1. **Job dispatch** - Session requests transcoding job
2. **Worker selection** - Coordinator picks available worker
3. **FFmpeg spawn** - Worker starts FFmpeg process
4. **Data streaming** - Source ES → FFmpeg → Target ES
5. **Buffer storage** - Target variant stored in shared buffer

See [Distributed Transcoding](/docs/next/advanced/distributed-transcoding) for multi-worker setups.
