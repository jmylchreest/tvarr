---
title: Transcoding
description: Real-time stream transcoding with FFmpeg
sidebar_position: 4
---

# Transcoding

tvarr can transcode streams in real-time using FFmpeg.

## When to Transcode

Transcoding is useful when:

- Your device doesn't support the source codec (e.g., HEVC on older devices)
- You want to reduce bandwidth for mobile viewers
- You need a different container format (HLS, DASH)
- The source quality is too high for your connection

## How It Works

```
Source Stream → FFmpeg → Transcoded Stream → Player
     │                         │
  HEVC/AAC              H264/AAC @ 720p
```

tvarr uses FFmpeg for transcoding, with support for:

- **Hardware acceleration** - VAAPI, NVENC, QSV, AMF
- **Distributed workers** - Scale across multiple machines
- **On-demand transcoding** - Only transcode when someone's watching

## Setup Options

### Built-in Transcoding

The standalone image (`ghcr.io/jmylchreest/tvarr:latest`) includes FFmpeg and can transcode locally.

### Distributed Transcoding

For larger setups, run separate transcoding workers:

1. Main tvarr as coordinator
2. One or more ffmpegd workers
3. Workers connect via gRPC and share the load

import DocCardList from '@theme/DocCardList';

<DocCardList />
