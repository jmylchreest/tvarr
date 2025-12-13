# Expression Engine

The expression engine provides a powerful, extensible language for filtering and transforming data in tvarr. It's used for channel filtering, EPG filtering, data mapping rules, and client detection rules.

## Quick Start

```
# Filter: Match BBC channels in UK group
channel_name contains "BBC" AND group_title equals "UK"

# Mapping: Transform channel data
channel_name matches "(.+) HD$" SET channel_name = "$1", group_title = "HD Channels"

# Client detection: Match Android TV (using dynamic header access)
@dynamic(request.headers):user-agent contains "Android" AND @dynamic(request.headers):user-agent contains "TV"

# Dynamic extraction: Get codec from header
@dynamic(request.headers):x-video-codec not_equals "" SET preferred_video_codec = @dynamic(request.headers):x-video-codec
```

## Expression Types

| Type | Description | Example |
|------|-------------|---------|
| **Filter** | Condition only, returns true/false | `channel_name contains "BBC"` |
| **Mapping** | Condition + SET actions | `channel_name contains "BBC" SET group_title = "UK"` |

---

## Operators

### Comparison Operators

| Operator | Aliases | Description | Example |
|----------|---------|-------------|---------|
| `equals` | `eq` | Exact match | `group_title equals "Sports"` |
| `not_equals` | `neq` | Not equal | `language not_equals "en"` |
| `contains` | - | Substring match | `channel_name contains "News"` |
| `not_contains` | - | No substring | `channel_name not_contains "Adult"` |
| `starts_with` | - | Prefix match | `stream_url starts_with "https"` |
| `not_starts_with` | - | No prefix | `tvg_id not_starts_with "test_"` |
| `ends_with` | - | Suffix match | `channel_name ends_with "HD"` |
| `not_ends_with` | - | No suffix | `channel_name not_ends_with "[Backup]"` |
| `matches` | - | Regex match | `channel_name matches "BBC.*"` |
| `not_matches` | - | No regex match | `tvg_id not_matches "^test_"` |
| `greater_than` | `gt` | Numeric > | `channel_number gt 100` |
| `greater_than_or_equal` | `gte` | Numeric >= | `tvg_shift gte 0` |
| `less_than` | `lt` | Numeric < | `channel_number lt 1000` |
| `less_than_or_equal` | `lte` | Numeric <= | `tvg_shift lte 24` |

### Logical Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `AND` | All conditions must match | `a equals "1" AND b equals "2"` |
| `OR` | Any condition must match | `a equals "1" OR a equals "2"` |
| `NOT` | Negate condition | `NOT channel_name contains "test"` |

**Precedence:** `()` > `NOT` > `AND` > `OR`

### Action Operators (SET clause)

| Operator | Shorthand | Description | Example |
|----------|-----------|-------------|---------|
| `SET` | `=` | Replace value | `SET group_title = "UK"` |
| `SET_IF_EMPTY` | `?=` | Set only if empty | `tvg_id ?= "default"` |
| `APPEND` | `+=` | Append to value | `channel_name += " [HD]"` |
| `REMOVE` | `-=` | Remove substring | `channel_name -= " [Backup]"` |
| `DELETE` | - | Clear field | `DELETE tvg_logo` |

---

## Fields

### Stream/Channel Fields

| Field | Aliases | Type | Modifiable | Description |
|-------|---------|------|------------|-------------|
| `channel_name` | `name` | String | Yes | Display name |
| `tvg_id` | `epg_id` | String | Yes | EPG identifier |
| `tvg_name` | - | String | Yes | TVG name attribute |
| `tvg_logo` | `logo` | String | Yes | Logo URL |
| `group_title` | `group`, `category` | String | Yes | Category/group |
| `stream_url` | `url` | String | **No** | Stream URL |
| `channel_number` | `number`, `chno` | Integer | Yes | Channel number |
| `tvg_shift` | - | Float | Yes | EPG time shift (hours) |
| `tvg_language` | `language`, `lang` | String | Yes | Language |
| `tvg_country` | `country` | String | Yes | Country |
| `radio` | - | Boolean | Yes | Is radio stream |

### EPG/Programme Fields

| Field | Aliases | Type | Modifiable | Description |
|-------|---------|------|------------|-------------|
| `programme_title` | `title`, `program_title` | String | Yes | Programme title |
| `programme_description` | `description`, `desc` | String | Yes | Description |
| `programme_start` | `start`, `start_time` | DateTime | **No** | Start time |
| `programme_stop` | `stop`, `end_time` | DateTime | **No** | End time |
| `programme_category` | `genre` | String | Yes | Category |
| `programme_episode` | `episode` | String | Yes | Episode info |
| `programme_season` | `season` | String | Yes | Season info |
| `programme_icon` | `poster` | String | Yes | Icon URL |

### Request Fields (Client Detection)

**Static fields** (computed/URL-based, available directly):

| Field | Aliases | Description |
|-------|---------|-------------|
| `client_ip` | `ip`, `remote_addr` | Client IP (computed from X-Forwarded-For, X-Real-IP, or RemoteAddr) |
| `request_path` | `path` | URL path component |
| `request_url` | `url` | Full request URL |
| `query_params` | `query` | Raw query string |
| `method` | - | HTTP method (GET, POST, etc.) |
| `host` | - | Request host |

**Header-based fields** (use `@dynamic(request.headers):header-name`):

| Header | Dynamic Syntax | Description |
|--------|----------------|-------------|
| User-Agent | `@dynamic(request.headers):user-agent` | User-Agent header |
| Accept | `@dynamic(request.headers):accept` | Accept header |
| Accept-Language | `@dynamic(request.headers):accept-language` | Accept-Language header |
| Referer | `@dynamic(request.headers):referer` | Referer header |
| Any header | `@dynamic(request.headers):header-name` | Any HTTP header (case-insensitive) |

### Source Metadata Fields (Read-Only)

| Field | Description |
|-------|-------------|
| `source_name` | Name of data source |
| `source_type` | Type (m3u, xtream, xmltv) |
| `source_url` | Source URL |

---

## Dynamic Fields

Dynamic fields use `@dynamic(path):key` syntax to access context-specific data at runtime.
This unified syntax works in **both conditions AND SET action values**.

### Syntax

```
@dynamic(context.path):key
```

### Available Contexts by Domain

Different expression domains inject different data into the dynamic context:

| Domain | Available Paths | Description |
|--------|-----------------|-------------|
| **Client Detection** | `request.headers` | HTTP request headers (case-insensitive keys) |
| | `request.query` | URL query parameters |
| **Stream Mapping** | `source.metadata` | *(Future)* Source-specific metadata |
| **EPG Mapping** | `source.metadata` | *(Future)* Source-specific metadata |

### Client Detection Context

When evaluating client detection rules, the following data is injected:

| Path | Contents |
|------|----------|
| `request.headers` | All HTTP request headers (e.g., `x-video-codec`, `user-agent`) |
| `request.query` | URL query parameters (e.g., `format`, `quality`) |

**Examples:**

```
# Check if a custom header exists (in condition)
@dynamic(request.headers):x-video-codec not_equals ""

# Extract header value (in SET action)
SET preferred_video_codec = @dynamic(request.headers):x-video-codec

# Check query parameter
@dynamic(request.query):format equals "dash"

# Full rule: extract codec from header if present
@dynamic(request.headers):x-video-codec not_equals "" SET preferred_video_codec = @dynamic(request.headers):x-video-codec
```

### Key Behavior

- **Returns empty string** if the path or key doesn't exist
- **Case-insensitive** for HTTP header keys
- **Validated by the service** - e.g., client detection validates extracted codec values against supported codecs

---

## Helpers

Helpers process values in SET actions using `@helper:arguments` syntax.
Unlike dynamic fields, helpers perform transformations on values.

### @time - Time Operations

| Operation | Syntax | Output |
|-----------|--------|--------|
| Current time | `@time:now` | `2024-01-15T10:30:00Z` |
| Parse time | `@time:parse` | RFC3339 format |
| Format time | `@time:format` | Custom format |
| Add duration | `@time:add` | Time + duration |

```
SET created_at = "@time:now"
SET formatted = "@time:format" with args "2024-01-15T10:00:00Z|2006-01-02"
```

### @logo - Logo Resolution

Resolve logo ULIDs to URLs with validation.

```
SET tvg_logo = "@logo:01ARZ3NDEKTSV4RRFFQ69G5FAV"
```

- Validates ULID format (26-char Crockford's base32)
- Returns `/api/v1/logos/{ULID}` or full URL
- Returns empty string if invalid

---

## SET Action Values

### Literal Values

```
SET group_title = "UK Channels"
SET channel_number = 100
```

### Field References

Copy another field's value using `$fieldname`:

```
SET backup_name = $channel_name
SET tvg_name = $channel_name
```

### Capture Groups

Reference regex capture groups using `$N` (1-based):

```
channel_name matches "([A-Z]{2}) (.+)" SET country = "$1", channel_name = "$2"
# "UK BBC One" -> country="UK", channel_name="BBC One"
```

### Dynamic Field References

Use `@dynamic(path):key` syntax in SET values (same syntax as conditions):

```
# Extract video codec from header
SET preferred_video_codec = @dynamic(request.headers):x-video-codec

# Extract query parameter
SET format = @dynamic(request.query):format
```

### Helper Invocations

```
SET created_at = "@time:now"
SET logo_url = "@logo:01ARZ3NDEKTSV4RRFFQ69G5FAV"
```

---

## Expression Examples

### Basic Filtering

```
# Match channels with "BBC" in name
channel_name contains "BBC"

# Match HD channels
channel_name ends_with "HD" OR channel_name contains "[HD]"

# Exclude adult content
group_title not_contains "adult" AND channel_name not_contains "xxx"

# Match specific language
language equals "en" OR language equals "english"
```

### Complex Conditions

```
# Match UK sports channels
(group_title contains "Sport" OR channel_name contains "Sport") AND language equals "en"

# Match specific channel range
channel_number gte 100 AND channel_number lte 199

# Match with regex
channel_name matches "^(BBC|ITV|Channel [0-9]).*"
```

### Data Mapping

```
# Set group for BBC channels
channel_name contains "BBC" SET group_title = "BBC"

# Extract timeshift from name
channel_name matches "(.+) \\+([0-9]+)" SET tvg_shift = "$2", channel_name = "$1"

# Set default values
tvg_id equals "" SET_IF_EMPTY tvg_id = $channel_name

# Append suffix
group_title contains "HD" APPEND channel_name = " [HD]"

# Multiple actions
channel_name contains "News" SET group_title = "News", tvg_logo = "http://example.com/news.png"
```

### Client Detection

```
# Android TV (using dynamic header access)
@dynamic(request.headers):user-agent contains "Android" AND @dynamic(request.headers):user-agent contains "TV"

# iOS devices
@dynamic(request.headers):user-agent contains "iPhone" OR @dynamic(request.headers):user-agent contains "iPad"

# Chrome browser
@dynamic(request.headers):user-agent contains "Chrome" AND NOT @dynamic(request.headers):user-agent contains "Edge"

# Match by client IP (static field)
client_ip starts_with "192.168."

# Match by request path (static field)
request_path contains "/api/v1/"

# Dynamic codec extraction from header
@dynamic(request.headers):x-video-codec not_equals "" SET preferred_video_codec = @dynamic(request.headers):x-video-codec

# Dynamic audio codec extraction
@dynamic(request.headers):x-audio-codec not_equals "" SET preferred_audio_codec = @dynamic(request.headers):x-audio-codec

# Check query parameter for format preference
@dynamic(request.query):format equals "hls" SET preferred_format = "hls-fmp4"
```

---

## Boolean Literals

| Literal | Behavior |
|---------|----------|
| `true`, `TRUE` | Always matches |
| `false`, `FALSE` | Never matches |

```
# Always include (useful for catch-all rules)
true

# Conditionally enable
true AND channel_name contains "BBC"
```

---

## Case Sensitivity

| Element | Case Sensitive |
|---------|---------------|
| Field names | No |
| Operators | No |
| Keywords (AND, OR, SET) | No |
| String values | Yes (default) |
| Regex patterns | Yes (use `(?i)` for insensitive) |
| Dynamic field keys | No (for headers) |

```
# Case-insensitive regex
channel_name matches "(?i)bbc.*"
```

---

## Syntax Notes

1. **Parentheses** for grouping: `(a OR b) AND c`
2. **Quotes** for string values: `"value with spaces"`
3. **Escaping** in regex: `\\+` for literal `+`
4. **Multiple SET actions**: `SET a = "1", b = "2"` or `SET a = "1" APPEND b = "x"`
5. **Negation forms**: `NOT a contains "x"` or `a NOT contains "x"`

---

## Domains

Expressions are validated against domains that define available fields:

| Domain | Use Case | Injected Context |
|--------|----------|------------------|
| `stream_filter` / `stream` | Channel filtering | None |
| `epg_filter` / `epg` | Programme filtering | None |
| `stream_mapping` | Channel data transformation | None |
| `epg_mapping` | Programme data transformation | None |
| `request` | Client detection | `request.headers`, `request.query` |

---

## Error Handling

Common parse errors:

| Error | Cause | Solution |
|-------|-------|----------|
| `unexpected token` | Invalid syntax | Check operator spelling |
| `expected value` | Missing value after operator | Add quoted value |
| `invalid regex` | Bad regex pattern | Validate regex syntax |
| `unknown operator` | Typo in operator | Check operator table |

Runtime evaluation errors:

| Error | Cause | Solution |
|-------|-------|----------|
| `cannot parse as number` | Numeric comparison on string | Ensure field is numeric |
| `field not found` | Unknown field name | Check field table |

---

## Migration Notes

### Legacy Syntax (Deprecated)

The following syntaxes are deprecated but still supported for backward compatibility:

| Legacy Syntax | New Unified Syntax |
|---------------|-------------------|
| `@header_req:x-video-codec` | `@dynamic(request.headers):x-video-codec` |
| `@req:header:X-Video-Codec` | `@dynamic(request.headers):x-video-codec` |
| `@req:query:format` | `@dynamic(request.query):format` |

The unified `@dynamic(path):key` syntax should be used for all new expressions.
