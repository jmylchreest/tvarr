---
title: Filters
description: Include or exclude channels
sidebar_position: 2
---

# Filters

Filters include or exclude channels from your proxy output.

## Creating a Filter

Go to **Admin > Filters** and click **Add Filter**.

1. **Name** - Descriptive name for the filter
2. **Domain** - What it applies to:
   - `stream_filter` - Filter channels
   - `epg_filter` - Filter EPG programs
3. **Action** - What happens when matched:
   - `include` - Only matching channels appear
   - `exclude` - Matching channels are removed
4. **Expression** - The condition to match

## Filter Logic

### Include Filters

When you have include filters:
- **Only** channels matching at least one include filter appear
- If no include filters exist, all channels start included

### Exclude Filters

Exclude filters remove matching channels:
- Channels matching any exclude filter are removed
- Exclude is applied after include

### Example Setup

To get only UK sports channels:

```
Filter 1 (include): group_title contains "Sports"
Filter 2 (include): group_title contains "UK"
```

To get everything except adult content:

```
Filter 1 (exclude): group_title contains "Adult"
Filter 2 (exclude): channel_name contains "XXX"
```

## Common Patterns

### Keep Specific Groups

```
group_title equals "Movies" OR group_title equals "Series"
```

### Exclude by Name Pattern

```
channel_name matches ".*(Test|Backup|OLD).*"
```

### Keep Specific Sources

```
source_name equals "Premium Provider"
```

### Exclude Broken Streams

```
stream_url contains "offline"
```

## Linking to Proxies

Filters must be linked to a proxy to take effect:

1. Go to your proxy settings
2. Under **Filters**, select which filters to apply
3. Regenerate the proxy

The same filter can be used by multiple proxies.
