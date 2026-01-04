---
title: Proxies
description: Managing output playlists
sidebar_position: 4
---

# Proxies

Proxies are your output playlists that players connect to.

## Creating a Proxy

Click **Create Proxy** and configure:

### Basic Settings

| Setting | Description |
|---------|-------------|
| Name | Proxy identifier |
| Slug | URL-friendly name (auto-generated) |
| Mode | Redirect, Proxy, or Relay |

### Sources

Select which stream and EPG sources to include.

### Filters

Select which filters to apply:

- Filters control which channels appear
- Multiple filters can be combined

### Options

| Option | Description |
|--------|-------------|
| Numbering | How to assign channel numbers |
| Logo Caching | Download and serve logos locally |
| EPG Days | How many days of guide to include |

## Proxy URLs

After generating, copy your URLs:

```
Playlist: http://tvarr:8080/api/v1/proxy/{id}/playlist.m3u8
EPG:      http://tvarr:8080/api/v1/proxy/{id}/epg.xml
```

## Actions

- **Generate** - Build/rebuild the playlist
- **Edit** - Modify proxy settings
- **Copy URLs** - Copy playlist/EPG URLs
- **Delete** - Remove the proxy

## Status

Each proxy shows:

- Last generated time
- Channel count
- Any generation errors
