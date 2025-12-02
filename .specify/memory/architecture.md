# Tvarr Architecture

**Status**: APPROVED  
**Version**: 1.0.0  
**Last Updated**: 2025-11-29

## Overview

Tvarr is a high-performance IPTV/streaming management service that aggregates multiple stream sources, enriches them with EPG data, applies transformations and filters, and generates custom proxy playlists.

```
┌─────────────────────────────────────────────────────────────────────┐
│              HTTP/REST API (Huma + Chi Router)                       │
│  /api/v1/*  /proxy/{id}.m3u8  /proxy/{id}.xmltv  /relay/*  /docs    │
│                    OpenAPI 3.1 Auto-Generated                        │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
        ┌──────────────────────┼──────────────────────┐
        │                      │                      │
        v                      v                      v
   ┌─────────────┐      ┌──────────────┐      ┌──────────────┐
   │ Ingestion   │      │ Pipeline     │      │ Relay        │
   │ Service     │      │ Orchestrator │      │ Manager      │
   └─────┬───────┘      └──────┬───────┘      └──────┬───────┘
         │                     │                      │
         v                     v                      v
   ┌─────────────┐      ┌──────────────┐      ┌──────────────┐
   │ Source      │      │ Pipeline     │      │ FFmpeg       │
   │ Handlers    │      │ Stages       │      │ Wrapper      │
   │ - M3U       │      │ - Filter     │      │ - Redirect   │
   │ - Xtream    │      │ - DataMap    │      │ - Proxy      │
   │ - XMLTV     │      │ - Logo       │      │ - Relay      │
   └─────┬───────┘      │ - Number     │      └──────┬───────┘
         │              │ - Generate   │              │
         │              │ - Publish    │              │
         │              └──────┬───────┘              │
         │                     │                      │
         └──────────┬──────────┴──────────────────────┘
                    │
                    v
         ┌─────────────────────────────┐
         │  Repository Layer (GORM)    │
         │  - StreamSourceRepo         │
         │  - ChannelRepo              │
         │  - EpgSourceRepo            │
         │  - EpgProgramRepo           │
         │  - StreamProxyRepo          │
         │  - FilterRepo               │
         │  - DataMappingRuleRepo      │
         │  - RelayProfileRepo         │
         │  - LogoAssetRepo            │
         │  - JobRepo                  │
         └─────────────────────────────┘
                    │
                    v
         ┌─────────────────────────────┐
         │  Database (SQLite/PG/MySQL) │
         └─────────────────────────────┘
```

## Core Systems

### 1. Expression Filter/Rules Engine

A recursive descent parser with full AST support handling filters and data mapping rules.

**Grammar Structure:**
```
ExtendedExpression ::= ConditionOnly 
                     | ConditionWithActions 
                     | ConditionalActionGroups

ConditionTree ::= root: ConditionNode
ConditionNode ::= Condition { field, operator, value, modifiers }
                | Group { children: []ConditionNode, operator: LogicalOperator }

Action ::= { field, operator: SET|?=|REMOVE, value }
```

**Supported Operators:**
- Comparison: `equals`, `contains`, `matches` (regex), `starts_with`, `ends_with`
- Negated: `not_equals`, `not_contains`, `not_matches`, `not_starts_with`, `not_ends_with`
- Numeric: `greater_than`, `less_than`, `greater_than_or_equal`, `less_than_or_equal`
- Logical: `AND`, `OR` with parenthetical grouping

**Symbol Normalization:**
```
==  → equals          !=  → not_equals
=~  → matches         !~  → not_matches
>=  → greater_than_or_equal
<=  → less_than_or_equal
&&  → AND             ||  → OR
```

**Field Registry:**
Centralized registry with canonical fields and aliases:
- Stream Fields: `channel_name`, `group_title`, `tvg_id`, `tvg_name`, `tvg_logo`, `stream_url`
- EPG Fields: `programme_title` (alias: `program_title`), `programme_description`, `programme_category`

**Validation:**
- Structured errors with position, context, and suggestions
- Typo detection with similarity matching (e.g., "contians" → suggest "contains")
- Collects ALL errors before returning (not fail-fast)

### 2. FFmpeg Proxy Methods

Three proxy modes for stream handling:

**REDIRECT Mode:**
- Client fetches directly from original stream URL
- Minimal server involvement
- Best for compatible streams

**PROXY Mode:**
- Server fetches and relays stream to client
- Buffering and error handling
- No transcoding

**RELAY Mode (Transcode):**
- Server transcodes with FFmpeg before sending
- Supports relay profiles with codec/bitrate settings
- Hardware acceleration (VAAPI, NVENC, QSV, AMF)

**Relay Profile Configuration:**
```go
type RelayProfile struct {
    VideoCodec     VideoCodec  // h264, h265, av1, mpeg2, copy
    AudioCodec     AudioCodec  // aac, mp3, ac3, copy
    VideoPreset    string      // fast, medium, slow
    VideoBitrate   uint32      // kbps
    AudioBitrate   uint32      // kbps
    EnableHWAccel  bool
    PreferredHW    string      // auto, vaapi, nvenc, qsv, amf
}
```

**Hardware Acceleration Detection:**
```go
type HWAccelCapabilities struct {
    VAAPIAvailable bool  // Intel/AMD on Linux
    NVENCAvailable bool  // NVIDIA
    QSVAvailable   bool  // Intel Quick Sync
    AMFAvailable   bool  // AMD AMF
}
```

**Cyclic Buffer Streaming:**
- Memory-efficient relay using ring buffers
- Multiple concurrent clients share buffer
- Broadcast pattern for stream distribution

### 3. Logo Registry & Caching

In-memory logo registry with disk-based caching:

**Cache ID Generation (Deterministic):**
```
URL: https://example.com/logos/channel.png?token=xyz&id=123
     ↓
Normalize:
  - Remove scheme (http/https)
  - Remove default ports
  - Remove file extension
  - Sort query params alphabetically
     ↓
Result: example.com/logos/channel?id=123&token=xyz
     ↓
SHA256 Hash → Cache ID
```

**Index Structure:**
```go
type LogoCacheService struct {
    // Primary: url_hash → LogoCacheEntry
    cacheIndex map[uint64]*LogoCacheEntry
    
    // Secondary indices for search
    channelNameIndex  map[uint64][]uint64  // name_hash → url_hashes
    channelGroupIndex map[uint64][]uint64  // group_hash → url_hashes
    
    // LRU cache for search strings
    searchStringCache *lru.Cache
}
```

**Lazy Loading:**
- Service initializes quickly without filesystem scan
- Background job loads cache entries asynchronously
- Handles 100k+ logos with minimal memory (~200 bytes/entry)

### 4. Staged Pipeline Architecture

Multi-stage processing with artifact-driven flow:

```
Pipeline Execution:
├─ 0. Ingestion Guard (wait for active ingestion)
├─ 1. Filtering Stage
│   ├─ Apply stream filters
│   └─ Apply EPG filters
├─ 2. Data Mapping Stage
│   ├─ Apply stream rules
│   └─ Apply EPG rules
├─ 3. Logo Caching Stage
│   └─ Download & cache logos
├─ 4. Numbering Stage
│   └─ Assign channel numbers
├─ 5. Generation Stage
│   ├─ Generate M3U
│   └─ Generate XMLTV
└─ 6. Publish Stage
    └─ Atomic file move to production
```

**Stage Interface:**
```go
type PipelineStage interface {
    Name() string
    Execute(ctx context.Context, state *PipelineState) error
}

type PipelineState struct {
    ProxyID       uint
    TempDir       string
    ChannelWriter io.Writer  // Streaming output
    ProgramWriter io.Writer  // Streaming output
    Stats         *StageStats
    Logger        *slog.Logger
}
```

**Source Merging:**
- Channels from multiple sources aggregated
- Duplicate handling by `tvg_id`
- Logo conflicts resolved by source priority

**Memory Efficiency:**
- Streaming I/O to disk, not buffered in memory
- JSONL format for intermediate data
- Batch processing with configurable sizes
- Explicit GC hints between stages

## Data Models

### Core Entities

```
StreamSource       → Channels
EpgSource          → EpgPrograms
StreamProxy        → ProxySources, ProxyEpgSources, ProxyFilters
Filter             → (expressions)
DataMappingRule    → (expressions + actions)
RelayProfile       → (codec/bitrate config)
LogoAsset          → (cached logo metadata)
LastKnownCodec     → (ffprobe cache)
Job                → (scheduled task)
```

### Database Schema (Key Tables)

```sql
-- Stream Sources
stream_sources (id, name, url, source_type, credentials, update_cron, is_active)
channels (id, source_id, tvg_id, tvg_name, channel_name, tvg_logo, stream_url, group_title)

-- EPG Sources  
epg_sources (id, name, url, source_type, update_cron, is_active)
epg_programs (id, source_id, channel_id, start_time, end_time, title, description, category)

-- Proxy Configuration
stream_proxies (id, name, description, is_active)
proxy_sources (proxy_id, source_id)
proxy_epg_sources (proxy_id, epg_source_id)
proxy_filters (proxy_id, filter_id)

-- Rules Engine
filters (id, name, expression, source_type, is_active)
data_mapping_rules (id, name, expression, source_type, priority_order, is_active)

-- Relay
relay_profiles (id, name, video_codec, audio_codec, bitrates, hwaccel_config, is_active)
last_known_codecs (id, stream_url_hash, video_codec, audio_codec, probed_at)

-- Caching
logo_assets (id, url_hash, cached_path, content_type, width, height, last_accessed)
```

## Performance Targets

| Metric | Target | Threshold |
|--------|--------|-----------|
| Channel Ingestion (100k) | < 5 min | < 10 min |
| EPG Ingestion (1M programs) | < 10 min | < 20 min |
| Proxy Generation (50k channels) | < 2 min | < 5 min |
| Memory (ingestion) | < 500 MB | < 1 GB |
| Memory (generation) | < 500 MB | < 1 GB |
| API Response (list) | < 200 ms | < 500 ms |
| Relay Startup | < 3 s | < 5 s |

## Batch Sizes (Configurable)

| Entity | Default | Use Case |
|--------|---------|----------|
| Channels | 1000 | DB operations, pipeline processing |
| EPG Programs | 5000 | High-volume inserts |
| Logo Downloads | 50 | Concurrent HTTP requests |
| M3U/XMLTV Generation | Streaming | Direct to file, no buffering |

## Technology Stack

| Layer | Technology | Rationale |
|-------|------------|-----------|
| Language | Go 1.25.x | Memory safety, simple concurrency |
| API Framework | Huma v2.34+ | Auto-generated OpenAPI 3.1 from Go types |
| Router | Chi | Lightweight, idiomatic, full streaming support |
| ORM | GORM v2 | Flexible, multi-DB support |
| Configuration | Viper | Env + file support |
| CLI | Cobra | Industry standard |
| Logging | slog | Stdlib, structured |
| Build | Taskfile | Go-native task runner |
| Testing | testify + gomock | Assertions + mocking |
| FFmpeg | External/Embedded | go-ffstatic optional |

### Why Huma + Chi?

**Huma** provides self-documenting APIs with automatic OpenAPI 3.1 generation from Go structs:
- Input/output types become JSON Schema automatically
- Validation via struct tags
- Built-in `/docs` endpoint with Swagger UI
- Request/response examples from code
- Type-safe operation registration

**Chi** is the recommended router because:
- Full HTTP/2 and streaming support (critical for video relay)
- Standard `http.Handler` compatibility
- Excellent middleware ecosystem
- Lightweight with no external dependencies
- Huma's `humachi` adapter is the most mature

**Why not other routers?**
- **Fiber**: Built on fasthttp, explicitly lacks streaming support
- **Gin**: Works but adds unnecessary complexity, non-standard handlers
- **stdlib**: Works via `humago`, but Chi has better middleware

### Streaming in Huma

For video relay and large file transfers:

```go
// Stream response for video relay
huma.Register(api, huma.Operation{
    OperationID: "relay-stream",
    Method:      http.MethodGet,
    Path:        "/relay/{id}",
}, func(ctx context.Context, input *RelayInput) (*huma.StreamResponse, error) {
    return &huma.StreamResponse{
        Body: func(ctx huma.Context) {
            ctx.SetHeader("Content-Type", "video/mp2t")
            writer := ctx.BodyWriter()
            // Pipe from FFmpeg to client
            io.Copy(writer, ffmpegStdout)
            if f, ok := writer.(http.Flusher); ok {
                f.Flush()
            }
        },
    }, nil
})
```

For Server-Sent Events (status updates, progress):

```go
sse.Register(api, huma.Operation{
    OperationID: "pipeline-progress",
    Method:      http.MethodGet,
    Path:        "/api/v1/pipeline/{id}/progress",
}, map[string]any{
    "progress": ProgressEvent{},
    "complete": CompleteEvent{},
}, func(ctx context.Context, input *ProgressInput, send sse.Sender) {
    for event := range progressChan {
        send.Data(event)
    }
})
```

## Security Considerations

1. **Sandboxed File Operations**: All file I/O through sandboxed manager
2. **Path Traversal Prevention**: Canonical path resolution
3. **Input Validation**: At all system boundaries
4. **Circuit Breaker**: For external dependencies
5. **Connection Pooling**: Per-host concurrency limits
6. **TLS**: For external communications

## Extension Points

### Future: VOD Content System

```
VOD Sources (Xtream/Files)
       ↓
  Metadata Enrichment (genre, age rating)
       ↓
  VOD Catalog (searchable)
       ↓
  Live Channel Generation (merge VOD → pseudo-live stream)
       ↓
  EPG Generation (from VOD metadata)
```

### Future: Plugin System

- Custom source handlers
- Custom pipeline stages
- Custom expression functions
