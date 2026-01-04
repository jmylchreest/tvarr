---
title: Sources
description: Stream and EPG sources
sidebar_position: 2
---

# Sources

Sources are where tvarr gets its streams and guide data.

## Stream Sources

Stream sources provide channel lists with stream URLs.

### M3U/M3U8

Standard playlist format supported by most providers:

```m3u
#EXTM3U
#EXTINF:-1 tvg-id="bbc1" tvg-name="BBC One" tvg-logo="http://..." group-title="UK",BBC One
http://stream.example.com/live/bbc1.m3u8
```

### Xtream Codes API

Many providers use Xtream Codes. Enter your credentials:

- **Server URL** - Provider's base URL
- **Username** - Your username
- **Password** - Your password

tvarr will fetch channels via the Xtream API automatically.

### Manual Channels

Create channels manually when you have direct stream URLs that aren't part of a playlist.

## EPG Sources

EPG (Electronic Program Guide) sources provide schedule data.

### XMLTV

Standard format for TV guide data:

```xml
<tv>
  <channel id="bbc1">
    <display-name>BBC One</display-name>
  </channel>
  <programme channel="bbc1" start="20240101120000" stop="20240101130000">
    <title>News at Noon</title>
  </programme>
</tv>
```

### Xtream EPG

If your stream provider uses Xtream, their EPG is often available through the same API. Use the same credentials as your stream source.

## Linking Sources to Proxies

Sources alone don't output anything. They must be linked to a **Proxy** to generate playlists:

1. Create a Proxy
2. Add Stream Sources to it
3. Add EPG Sources to it
4. Generate the proxy

Each proxy can combine multiple sources, applying its own set of filters and mappings.
