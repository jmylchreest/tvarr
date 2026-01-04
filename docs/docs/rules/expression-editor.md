---
title: Expression Editor
description: The expression language for filters and rules
sidebar_position: 1
---

# Expression Editor

The expression editor is how you write filter and mapping rules.

## Basic Syntax

Expressions compare fields to values:

```
field operator "value"
```

Examples:

```
channel_name equals "BBC One"
group_title contains "Sports"
tvg_id starts_with "uk."
```

## Operators

### String Comparison

| Operator | Description | Example |
|----------|-------------|---------|
| `equals` | Exact match | `channel_name equals "BBC One"` |
| `contains` | Substring match | `channel_name contains "BBC"` |
| `starts_with` | Prefix match | `channel_name starts_with "BBC"` |
| `ends_with` | Suffix match | `channel_name ends_with "HD"` |
| `matches` | Regex match | `channel_name matches "BBC.*HD"` |

### Negated Operators

| Operator | Description |
|----------|-------------|
| `not_equals` | Not exact match |
| `not_contains` | Doesn't contain |
| `not_starts_with` | Doesn't start with |
| `not_ends_with` | Doesn't end with |
| `not_matches` | Regex doesn't match |

### Numeric Comparison

| Operator | Description |
|----------|-------------|
| `greater_than` | Value greater than number |
| `less_than` | Value less than number |
| `greater_than_or_equal` | Value greater than or equal |
| `less_than_or_equal` | Value less than or equal |

### Symbolic Aliases

You can also use familiar symbols:

| Symbol | Equivalent |
|--------|------------|
| `==` | `equals` |
| `!=` | `not_equals` |
| `=~` | `matches` |
| `!~` | `not_matches` |
| `>` | `greater_than` |
| `<` | `less_than` |
| `>=` | `greater_than_or_equal` |
| `<=` | `less_than_or_equal` |

## Combining Conditions

Use `AND`, `OR`, and parentheses:

```
channel_name contains "BBC" AND group_title equals "UK"

(channel_name contains "BBC" OR channel_name contains "ITV") AND group_title not_equals "Radio"

NOT channel_name contains "Adult"
```

## Available Fields

### Stream/Channel Fields

| Field | Description |
|-------|-------------|
| `channel_name` | Display name |
| `tvg_id` | EPG identifier |
| `tvg_name` | Alternative name |
| `tvg_logo` | Logo URL |
| `group_title` | Category/group |
| `stream_url` | Stream URL |
| `channel_number` | Assigned number |
| `source_name` | Source it came from (read-only) |

### EPG/Programme Fields

| Field | Description |
|-------|-------------|
| `programme_title` | Show title |
| `programme_description` | Show description |
| `programme_category` | Show category |

### Request Fields (Client Detection)

| Field | Description |
|-------|-------------|
| `client_ip` | Client's IP address |
| `request_path` | URL path |
| `host` | Host header |
| `@dynamic(request.headers):name` | Any request header |
| `@dynamic(request.query):name` | Any query parameter |

## SET Actions (Data Mapping Only)

Data mapping rules can modify fields:

```
condition SET field = "value"
```

### SET Action Types

| Action | Description | Example |
|--------|-------------|---------|
| `SET` | Set field value | `SET group_title = "Sports"` |
| `SET_IF_EMPTY` | Set only if empty | `SET_IF_EMPTY tvg_id = "unknown"` |
| `APPEND` | Add to value | `APPEND channel_name = " HD"` |
| `REMOVE` | Remove substring | `REMOVE channel_name = "HD"` |

### Value Sources

| Syntax | Description |
|--------|-------------|
| `"literal"` | Literal string |
| `$field` | Copy from another field |
| `$1`, `$2` | Regex capture groups |
| `@dynamic(...)` | Dynamic request value |
| `@time:now` | Current timestamp |
| `@logo:ULID` | Cached logo reference |

## Examples

### Filter: UK Sports Only

```
group_title equals "Sports" AND source_name equals "UK Provider"
```

### Mapping: Clean Channel Names

```
channel_name matches "^(.+) (HD|SD|FHD)$" SET tvg_name = "$1", quality = "$2"
```

### Mapping: Fix Logos

```
channel_name contains "BBC" SET tvg_logo = "https://example.com/bbc-logo.png"
```

### Client Detection: Mobile Gets Lower Quality

```
@dynamic(request.headers):user-agent contains "Android" SET preferred_profile = "mobile-720p"
```
