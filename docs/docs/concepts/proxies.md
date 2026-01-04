---
title: Proxies
description: Output playlists and streaming modes
sidebar_position: 3
---

# Proxies

Proxies are your output playlists that players connect to.

## What's a Proxy?

A proxy is a virtual playlist that:

- Combines channels from multiple sources
- Applies filters and data mappings
- Serves an M3U8 playlist and EPG XML
- Optionally proxies or transcodes streams

## Proxy Modes

How streams are delivered to your player:

### Redirect Mode

```
Player ──▶ tvarr (redirect) ──▶ Player connects directly to source
```

- **Pros**: Zero bandwidth on tvarr server, lowest latency
- **Cons**: Exposes source URLs, no transcoding, no buffering
- **Use when**: Source is reliable and player handles it well

### Proxy Mode

```
Player ──▶ tvarr (proxy) ──▶ Source
           └── buffers ────┘
```

- **Pros**: Hides source URLs, can buffer unstable sources
- **Cons**: All traffic goes through tvarr
- **Use when**: Source URLs change or need basic proxying

### Relay Mode

```
Player ──▶ tvarr (relay) ──▶ FFmpeg ──▶ Source
           └── transcodes ─────┘
```

- **Pros**: Full transcoding, codec conversion, multiple output formats
- **Cons**: CPU/GPU intensive, higher latency
- **Use when**: Device needs different codecs, want HLS/DASH output

## Output URLs

After generating a proxy, you get these URLs:

| Type | URL Pattern |
|------|-------------|
| M3U8 Playlist | `/api/v1/proxy/{id}/playlist.m3u8` |
| EPG XML | `/api/v1/proxy/{id}/epg.xml` |
| Individual Stream | `/api/v1/relay/channel/{id}/stream` |

## Relay Formats

When using Relay mode, you can serve streams in different formats:

| Format | Best For |
|--------|----------|
| `hls-ts` | Wide compatibility (default HLS) |
| `hls-fmp4` | Modern players, better seeking |
| `dash` | Adaptive bitrate, web players |
| `ts` | Continuous MPEG-TS stream |

Players request their preferred format via query parameter or headers.
