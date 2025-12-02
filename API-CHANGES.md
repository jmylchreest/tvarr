# API Changes from m3u-proxy

This document tracks breaking changes between the original m3u-proxy Rust API and the new tvarr Go API. Use this as a reference when updating frontend code.

## Global Changes

### Identifier Type Change

**Original (m3u-proxy):** Used UUID v4 strings (e.g., `"550e8400-e29b-41d4-a716-446655440000"`)

**New (tvarr):** Uses ULID strings (e.g., `"01ARZ3NDEKTSV4RRFFQ69G5FAV"`)

ULIDs are:
- Lexicographically sortable (newer IDs sort after older ones)
- 26 characters (vs 36 for UUID with dashes)
- URL-safe (no special characters)
- Compatible with UUID columns in most databases

All `id`, `source_id`, `proxy_id`, and other identifier fields use ULID format.

## Stream Source API

### Field Renames

| Original (m3u-proxy) | New (tvarr) | Notes |
|---------------------|-------------|-------|
| `source_type` | `type` | Values unchanged: `m3u`, `xtream` |
| `is_active` | `enabled` | Boolean, same semantics |
| `last_sync_at` | `last_ingestion_at` | Nullable timestamp |
| `last_sync_error` | `last_error` | Error message string |
| `channel_count` | `channel_count` | No change |

### New Fields

| Field | Type | Description |
|-------|------|-------------|
| `user_agent` | string | Custom User-Agent header for requests |
| `cron_schedule` | string | Cron expression for auto-ingestion |
| `priority` | int | Priority for channel merging (higher = preferred) |

### Removed Fields

| Field | Notes |
|-------|-------|
| `sync_interval_minutes` | Replaced by `cron_schedule` |

### Status Values

| Original | New | Notes |
|----------|-----|-------|
| `idle` | `pending` | Initial state |
| `syncing` | `ingesting` | Currently fetching |
| `synced` | `success` | Completed successfully |
| `error` | `failed` | Last operation failed |

## EPG Source API

### Field Renames

| Original (m3u-proxy) | New (tvarr) | Notes |
|---------------------|-------------|-------|
| `source_type` | `type` | Values: `xmltv`, `xtream` |
| `is_active` | `enabled` | Boolean |
| `last_sync_at` | `last_ingestion_at` | Nullable timestamp |
| `last_sync_error` | `last_error` | Error message string |

### New Fields

| Field | Type | Description |
|-------|------|-------------|
| `user_agent` | string | Custom User-Agent header |
| `cron_schedule` | string | Cron expression for auto-ingestion |
| `priority` | int | Priority for program merging |
| `retention_days` | int | Days to retain EPG data after expiry |
| `program_count` | int | Number of programs from last ingestion |

## Stream Proxy API

### Field Renames

| Original (m3u-proxy) | New (tvarr) | Notes |
|---------------------|-------------|-------|
| `mode` | `proxy_mode` | Values: `redirect`, `proxy`, `relay` |
| `active` | `is_active` | Boolean |
| `last_generated` | `last_generated_at` | Nullable timestamp |
| `sources` | `source_ids` | In create/update requests |
| `epg_sources` | `epg_source_ids` | In create/update requests |

### New Fields

| Field | Type | Description |
|-------|------|-------------|
| `auto_regenerate` | bool | Auto-regenerate when sources change |
| `starting_channel_number` | int | Base channel number for output |
| `upstream_timeout` | int | Timeout in seconds for upstream |
| `buffer_size` | int | Buffer size for proxy mode |
| `max_concurrent_streams` | int | Limit concurrent streams (0 = unlimited) |
| `cache_channel_logos` | bool | Cache channel logos locally |
| `cache_program_logos` | bool | Cache EPG program logos locally |
| `relay_profile_id` | ULID | Reference to transcoding profile |
| `channel_count` | int | Channels in last generation |
| `program_count` | int | Programs in last generation |
| `output_path` | string | Path for generated files |

### Status Values

| Original | New | Notes |
|----------|-----|-------|
| `idle` | `pending` | Initial state |
| `generating` | `generating` | Currently generating |
| `ready` | `ready` | Generation complete |
| `error` | `failed` | Last generation failed |

## Channel API

### Field Renames

| Original (m3u-proxy) | New (tvarr) | Notes |
|---------------------|-------------|-------|
| `external_id` | `ext_id` | External identifier from source |
| `name` | `channel_name` | Display name |
| `number` | `channel_number` | Channel number |
| `logo_url` | `tvg_logo` | Logo URL |
| `group` | `group_title` | Channel group |
| `url` | `stream_url` | Stream URL |

### New Fields

| Field | Type | Description |
|-------|------|-------------|
| `tvg_id` | string | EPG channel ID for matching |
| `tvg_name` | string | EPG channel name |
| `stream_type` | string | Stream type (live, movie, series) |
| `language` | string | Primary language |
| `country` | string | Country code |
| `is_adult` | bool | Adult content flag |

## EPG Program API

### Field Renames

| Original (m3u-proxy) | New (tvarr) | Notes |
|---------------------|-------------|-------|
| `channel_id` | `channel_id` | No change (string, not ULID) |
| `start_time` | `start` | UTC timestamp |
| `end_time` | `stop` | UTC timestamp |
| `subtitle` | `sub_title` | Episode subtitle |
| `episode` | `episode_num` | Episode number string |

### New Fields

| Field | Type | Description |
|-------|------|-------------|
| `source_id` | ULID | EPG source reference |
| `rating` | string | Content rating |
| `language` | string | Program language |
| `is_new` | bool | New episode flag |
| `is_premiere` | bool | Premiere flag |
| `is_live` | bool | Live broadcast flag |

## Pagination

### Request Parameters

| Original | New | Notes |
|----------|-----|-------|
| `page` | `page` | 1-indexed (no change) |
| `per_page` | `limit` | Items per page |
| `offset` | - | Removed, use page-based |

### Response Structure

Original:
```json
{
  "data": [...],
  "total": 100,
  "page": 1,
  "per_page": 50
}
```

New:
```json
{
  "pagination": {
    "current_page": 1,
    "page_size": 50,
    "total_items": 100,
    "total_pages": 2
  },
  "items": [...]
}
```

Note: The `items` key varies by endpoint (e.g., `channels`, `programs`, `sources`).

## Error Responses

Original:
```json
{
  "error": "Not found",
  "code": 404
}
```

New:
```json
{
  "error": "Not found",
  "details": "Stream source with ID 01ARZ3NDEKTSV4RRFFQ69G5FAV not found"
}
```

## Endpoint Changes

### Renamed Endpoints

| Original | New | Notes |
|----------|-----|-------|
| `POST /sources/:id/sync` | `POST /sources/:id/ingest` | Trigger ingestion |
| `POST /epg/:id/sync` | `POST /epg-sources/:id/ingest` | Trigger EPG ingestion |
| `POST /proxies/:id/generate` | `POST /proxies/:id/generate` | No change |

### New Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check with version info |
| `/sources/:id/channels` | GET | List channels for a source |
| `/epg-sources/:id/programs` | GET | List programs for an EPG source |
| `/proxies/:id/sources` | PUT | Set proxy stream sources |
| `/proxies/:id/epg-sources` | PUT | Set proxy EPG sources |

## Migration Guide

### Frontend Updates Required

1. **Update ID handling**: Change UUID validation/parsing to accept ULID format (26 alphanumeric characters)

2. **Rename field mappings**: Update API response parsing to use new field names

3. **Update pagination handling**: Adjust to nested `pagination` object in responses

4. **Update status handling**: Map new status values in UI components

### Example ID Migration

```typescript
// Before (UUID)
const isValidId = (id: string) => 
  /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(id);

// After (ULID)
const isValidId = (id: string) => 
  /^[0-9A-HJKMNP-TV-Z]{26}$/i.test(id);
```

### Example Field Migration

```typescript
// Before
interface Source {
  id: string;
  source_type: 'm3u' | 'xtream';
  is_active: boolean;
  last_sync_at: string | null;
}

// After
interface Source {
  id: string;
  type: 'm3u' | 'xtream';
  enabled: boolean;
  last_ingestion_at: string | null;
}
```
