---
title: First Steps
description: Configure your first streams after installation
sidebar_position: 3
---

# First Steps

Now that tvarr is running, let's set up your first streams.

## 1. Add a Stream Source

Go to **Sources > Streams** and click **Add Source**.

Choose your source type:

| Type | Use When |
|------|----------|
| **M3U URL** | You have a direct URL to an M3U/M3U8 playlist |
| **M3U File** | You have a downloaded playlist file |
| **Xtream** | Your provider uses Xtream Codes API |

Enter your details and click **Save**. The ingestion will start automatically.

## 2. Add an EPG Source (Optional)

Go to **Sources > EPG** and click **Add Source**.

| Type | Use When |
|------|----------|
| **XMLTV URL** | Standard XMLTV guide URL |
| **XMLTV File** | Downloaded XMLTV file |
| **Xtream** | Same provider as your stream source |

:::tip Auto-match EPG
tvarr automatically matches EPG data to channels using `tvg-id`. If your M3U includes `tvg-id` tags, EPG data should link automatically.
:::

## 3. Create a Proxy

Go to **Proxies** and click **Create Proxy**.

A proxy is your output playlist that combines sources:

1. **Name** - Give it a descriptive name (e.g., "Main TV")
2. **Stream Sources** - Select which sources to include
3. **EPG Sources** - Select which EPG sources to include
4. **Proxy Mode** - Choose how streams are served:
   - **Redirect** - Client connects directly to source (lowest overhead)
   - **Proxy** - Streams through tvarr (can buffer, more control)
   - **Relay** - Full transcoding support (highest compatibility)

Click **Save** and then **Generate** to build your playlist.

## 4. Use Your Playlist

Your M3U playlist URL is:

```
http://your-tvarr-host:8080/api/v1/proxy/{proxy-id}/playlist.m3u8
```

Your EPG URL is:

```
http://your-tvarr-host:8080/api/v1/proxy/{proxy-id}/epg.xml
```

Copy these URLs into your IPTV player (Jellyfin, Plex, TiviMate, etc.).

## Next Steps

- **Filter channels** - [Create filters](/docs/next/rules/filters) to include/exclude channels
- **Transform data** - [Use data mapping](/docs/next/rules/data-mapping) to rename channels, fix logos
- **Set up transcoding** - [Configure encoding profiles](/docs/next/transcoding/) for device compatibility
