---
title: Channels
description: Ingested channels and their metadata
sidebar_position: 4
---

# Channels

Channels are the streams ingested from your sources.

## Channel Metadata

Each channel has these fields:

| Field | Description |
|-------|-------------|
| `channel_name` | Display name from M3U |
| `tvg_id` | EPG identifier for guide matching |
| `tvg_name` | Alternative name |
| `tvg_logo` | Logo URL |
| `group_title` | Category/group |
| `stream_url` | Actual stream URL |
| `channel_number` | Assigned number |

These fields can all be used in [filters](/docs/next/rules/filters) and [data mapping](/docs/next/rules/data-mapping).

## Browsing Channels

The **Channels** page shows all ingested channels across all sources:

- **Search** - Find channels by name
- **Filter by source** - View channels from specific sources
- **Filter by group** - View channels in specific categories

## EPG Matching

Channels link to EPG data via `tvg_id`:

1. Your M3U has `tvg-id="news1"`
2. Your XMLTV has `<channel id="news1">`
3. tvarr links them automatically

:::tip Fixing EPG Matches
If EPG isn't matching, use [data mapping](/docs/next/rules/data-mapping) to set the correct `tvg_id`:

```
channel_name contains "News Channel" SET tvg_id = "news1.example"
```
:::

## Channel Numbers

Channel numbers can be assigned in three modes:

- **Preserve** - Keep numbers from source if present
- **Sequential** - Auto-number 1, 2, 3...
- **Source-based** - Each source gets a range (1-999, 1000-1999, etc.)

Set the numbering mode in your proxy settings.
