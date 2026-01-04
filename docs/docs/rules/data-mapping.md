---
title: Data Mapping
description: Transform channel metadata
sidebar_position: 3
---

# Data Mapping

Data mapping rules transform channel fields before filtering.

## Creating a Rule

Go to **Admin > Data Mapping** and click **Add Rule**.

1. **Name** - Descriptive name
2. **Domain** - What it applies to:
   - `stream_mapping` - Transform channels
   - `epg_mapping` - Transform EPG programs
3. **Expression** - Condition + SET actions

## Basic Structure

```
condition SET field1 = "value1", field2 = "value2"
```

The condition is optional. Without it, the rule applies to all channels:

```
SET group_title = "All Channels"
```

## Use Cases

### Fix Channel Names

Remove unwanted suffixes:

```
channel_name ends_with " HD" SET channel_name = $tvg_name
```

Extract clean name with regex:

```
channel_name matches "^(.+) \\| .+$" SET channel_name = "$1"
```

### Organize Groups

Consolidate messy groups:

```
group_title contains "Sport" SET group_title = "Sports"
```

Create new groups:

```
channel_name starts_with "BBC" SET group_title = "BBC Channels"
```

### Fix EPG IDs

When EPG doesn't match automatically:

```
channel_name equals "BBC One" SET tvg_id = "bbc1.uk"
```

### Set Logos

Replace missing or broken logos:

```
tvg_logo equals "" SET tvg_logo = "https://example.com/default.png"
```

Use cached logos:

```
channel_name contains "BBC" SET tvg_logo = "@logo:01ABC123DEF456"
```

### Regex Capture Groups

Extract parts of a field:

```
channel_name matches "^(.+) (HD|SD|4K)$" SET tvg_name = "$1", quality = "$2"
```

`$1` is the first capture group (name), `$2` is the second (quality).

## Execution Order

Data mapping rules run in order (by priority), and each rule can see changes from previous rules.

Example:

```
Rule 1: channel_name matches "(.+) HD" SET tvg_name = "$1"
Rule 2: tvg_name not_equals "" SET clean_name = $tvg_name
```

Rule 2 can use `tvg_name` that was set by Rule 1.

## Conditional SET

Only set a field if it's empty:

```
tvg_logo equals "" SET_IF_EMPTY tvg_logo = "https://fallback.com/logo.png"
```

Or using `SET_IF_EMPTY`:

```
SET_IF_EMPTY tvg_id = "unknown"
```
