---
title: Introduction
description: IPTV proxy and stream aggregator for home users
sidebar_position: 1
slug: /
---

# tvarr

**tvarr** (pronounced *tee-vee-arr*) is an IPTV proxy and stream aggregator for home users.

Inspired by the *arr tools (Sonarr, Radarr, etc.), it brings multiple external IPTV streams into one convenient, curated manifest for your favorite M3U8 player.

:::tip Pronunciation
**tvarr** = *tee-vee-arr* (TV + arr)
:::

## What It Does

### Aggregate Streams

Combine multiple M3U/Xtream sources into unified playlists:

- **Import from anywhere** - M3U files, URLs, or Xtream Codes APIs
- **Merge EPG data** - XMLTV and Xtream guide data combined
- **Filter and organize** - Keep only what you want, organized your way

### Proxy and Transcode

Stream through tvarr for compatibility and control:

- **Passthrough mode** - Direct proxy, minimal overhead
- **Transcode on-demand** - Convert codecs for device compatibility
- **Hardware acceleration** - VAAPI, NVENC, QSV, AMF support
- **Distributed workers** - Scale transcoding across multiple machines

### Curate Your Experience

Powerful filtering and transformation:

- **Expression-based rules** - Filter channels by name, group, or any field
- **Data mapping** - Transform channel metadata (names, logos, groups)
- **Client detection** - Serve different quality profiles based on device

## Quick Start

```bash
# Using Docker Compose (recommended)
curl -O https://raw.githubusercontent.com/jmylchreest/tvarr/main/deployment/docker/docker-compose.standalone.yml
docker compose -f docker-compose.standalone.yml up -d

# Access the web UI
open http://localhost:8080
```

## Next Steps

<div className="row">
  <div className="col col--4">
    <h3>Get Started</h3>
    <p>Install and run tvarr in 5 minutes</p>
    <a href="/docs/next/quickstart/">Quickstart Guide</a>
  </div>
  <div className="col col--4">
    <h3>Learn the Basics</h3>
    <p>Understand sources, proxies, and channels</p>
    <a href="/docs/next/concepts/">Core Concepts</a>
  </div>
  <div className="col col--4">
    <h3>Filter Channels</h3>
    <p>Create rules to curate your streams</p>
    <a href="/docs/next/rules/">Filtering & Rules</a>
  </div>
</div>
