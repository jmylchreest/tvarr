# Tvarr - Detailed Technical Documentation

**Tvarr** is a self-hosted IPTV stream proxy and management service written in Go. It aggregates multiple M3U/Xtream sources, merges EPG data, and generates unified playlists with powerful filtering and transformation capabilities.

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Core Components](#core-components)
- [Data Models](#data-models)
- [Expression Engine](#expression-engine)
- [Pipeline System](#pipeline-system)
- [Scheduled Jobs](#scheduled-jobs)
- [REST API](#rest-api)
- [Configuration](#configuration)
- [Building & Running](#building--running)

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              REST API (Huma v2)                         │
│   /sources  /epg-sources  /proxies  /filters  /rules  /jobs  /health   │
└──────────────────────────────────┬──────────────────────────────────────┘
                                   │
┌──────────────────────────────────┴──────────────────────────────────────┐
│                           Service Layer                                  │
│  SourceService  EpgService  ProxyService  JobService  LogoService       │
└──────────────────────────────────┬──────────────────────────────────────┘
                                   │
┌──────────────────────────────────┴──────────────────────────────────────┐
│                         Pipeline Orchestrator                            │
│  Guard → Load → Programs → Map → Filter → Number → Logo → Generate      │
└──────────────────────────────────┬──────────────────────────────────────┘
                                   │
┌──────────────────────────────────┴──────────────────────────────────────┐
│                          Repository Layer                                │
│  StreamSourceRepo  EpgSourceRepo  ChannelRepo  JobRepo  FilterRepo      │
└──────────────────────────────────┬──────────────────────────────────────┘
                                   │
┌──────────────────────────────────┴──────────────────────────────────────┐
│                         SQLite Database (GORM)                           │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Core Components

### Command Line Interface

```
cmd/tvarr/
├── main.go           # Entry point
└── cmd/
    ├── root.go       # Root command with global flags
    ├── serve.go      # HTTP server command
    └── version.go    # Version information
```

**Usage:**
```bash
tvarr serve --config /path/to/config.yaml
tvarr version
```

### HTTP Server

The server uses [Huma v2](https://huma.rocks/) for OpenAPI-compliant REST APIs with automatic documentation generation.

**Features:**
- Structured logging with slog
- Request ID middleware
- CORS support
- Panic recovery
- SSE (Server-Sent Events) for real-time progress updates

### Database Layer

SQLite with GORM ORM, featuring:
- Auto-migrations on startup
- Versioned migration registry
- ULID primary keys for all entities
- Soft deletes where appropriate

---

## Data Models

### Stream Sources (`internal/models/stream_source.go`)

Represents an M3U playlist or Xtream server:

| Field | Type | Description |
|-------|------|-------------|
| `id` | ULID | Unique identifier |
| `name` | string | User-friendly name |
| `type` | enum | `m3u`, `xtream`, or `manual` |
| `url` | string | Source URL |
| `username` | string | Xtream username (optional) |
| `password` | string | Xtream password (encrypted) |
| `enabled` | bool | Whether source is active |
| `priority` | int | Merge priority (higher = preferred) |
| `cron_schedule` | string | Auto-ingestion schedule |
| `status` | enum | `pending`, `ingesting`, `ready`, `error` |

**Source Types:**
- **M3U**: Standard M3U/M3U8 playlist URLs
- **Xtream**: Xtream Codes API compatible servers
- **Manual**: User-defined channels (no external source)

### EPG Sources (`internal/models/epg_source.go`)

Electronic Program Guide data sources:

| Field | Type | Description |
|-------|------|-------------|
| `id` | ULID | Unique identifier |
| `name` | string | User-friendly name |
| `type` | enum | `xmltv` or `xtream` |
| `url` | string | XMLTV URL or Xtream server |
| `enabled` | bool | Whether source is active |
| `priority` | int | Merge priority |
| `cron_schedule` | string | Auto-ingestion schedule |
| `retention_days` | int | Days to keep EPG data |

### Stream Proxies (`internal/models/stream_proxy.go`)

Output configuration for generated playlists:

| Field | Type | Description |
|-------|------|-------------|
| `id` | ULID | Unique identifier |
| `name` | string | Unique proxy name |
| `proxy_mode` | enum | `redirect`, `proxy`, `relay` |
| `is_active` | bool | Whether proxy is enabled |
| `auto_regenerate` | bool | Auto-regenerate on source changes |
| `starting_channel_number` | int | Base channel number |
| `cache_channel_logos` | bool | Cache channel logos locally |
| `cache_program_logos` | bool | Cache EPG program logos |
| `output_path` | string | File output path |

**Proxy Modes:**
- **Redirect**: Direct links to upstream (no bandwidth usage)
- **Proxy**: Pass-through proxy (bandwidth counted)
- **Relay**: FFmpeg transcoding (Phase 12)

### Channels (`internal/models/channel.go`)

Ingested channel data:

| Field | Type | Description |
|-------|------|-------------|
| `source_id` | ULID | Parent source |
| `ext_id` | string | External identifier |
| `tvg_id` | string | EPG mapping ID |
| `tvg_name` | string | EPG display name |
| `tvg_logo` | string | Logo URL |
| `group_title` | string | Category/group |
| `channel_name` | string | Display name |
| `channel_number` | int | Assigned number |
| `stream_url` | string | Stream URL |

### Filters (`internal/models/filter.go`)

Expression-based channel/program filters:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Filter name |
| `description` | string | Human-readable description |
| `domain` | enum | `stream` or `epg` |
| `expression` | string | Filter expression |
| `action` | enum | `include` or `exclude` |
| `priority` | int | Evaluation order |
| `enabled` | bool | Whether filter is active |

### Data Mapping Rules (`internal/models/data_mapping_rule.go`)

Expression-based field transformations:

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Rule name |
| `description` | string | Human-readable description |
| `domain` | enum | `stream` or `epg` |
| `expression` | string | Full rule expression |
| `priority` | int | Evaluation order |
| `enabled` | bool | Whether rule is active |

### Jobs (`internal/models/job.go`)

Scheduled and immediate job execution:

| Field | Type | Description |
|-------|------|-------------|
| `type` | enum | `stream_ingestion`, `epg_ingestion`, `proxy_generation`, `logo_cleanup` |
| `target_id` | ULID | Source/proxy being processed |
| `status` | enum | `pending`, `scheduled`, `running`, `completed`, `failed`, `cancelled` |
| `cron_schedule` | string | For recurring jobs |
| `next_run_at` | time | Next scheduled execution |
| `attempt_count` | int | Retry attempts |
| `max_attempts` | int | Maximum retries (default: 3) |
| `backoff_seconds` | int | Base backoff duration |
| `last_error` | string | Error from last attempt |
| `result` | string | Success result message |

---

## Expression Engine

The expression engine (`internal/expression/`) provides a powerful DSL for filtering and transforming data.

### Expression Syntax

**Conditions:**
```
field operator "value"
field operator "value" AND field2 operator2 "value2"
field operator "value" OR field2 operator2 "value2"
(field operator "value" AND field2 operator2 "value2") OR field3 operator3 "value3"
```

**Operators:**
| Operator | Description | Example |
|----------|-------------|---------|
| `equals` / `==` | Exact match | `group_title equals "Sports"` |
| `not_equals` / `!=` | Not equal | `group_title != "Adult"` |
| `contains` | Substring match | `channel_name contains "HD"` |
| `not_contains` | No substring | `channel_name not_contains "SD"` |
| `starts_with` | Prefix match | `tvg_id starts_with "us."` |
| `ends_with` | Suffix match | `stream_url ends_with ".m3u8"` |
| `matches` | Regex match | `channel_name matches "^ESPN\\d+"` |
| `not_matches` | Regex no match | `group_title not_matches "XXX"` |
| `greater_than` / `>` | Numeric comparison | `channel_number > 100` |
| `less_than` / `<` | Numeric comparison | `channel_number < 1000` |
| `is_empty` | Field is empty | `tvg_logo is_empty` |
| `is_not_empty` | Field has value | `tvg_id is_not_empty` |

**Actions (for data mapping rules):**
```
condition => field = "value"
condition => field SET "value"
condition => field SET_IF_EMPTY "value"
condition => field APPEND " HD"
condition => field PREPEND "US: "
condition => field REMOVE "pattern"
condition => field DELETE
```

**Regex Capture Groups:**
```
channel_name matches "^(.+) (HD|SD)$" => tvg_name SET "$1", quality SET "$2"
```

**Shorthand Syntax:**
```
# These are equivalent:
group_title equals "Sports"
group_title: Sports

# Wildcard matching:
channel_name: *HD*        # contains "HD"
channel_name: ESPN*       # starts with "ESPN"
channel_name: *News       # ends with "News"
```

### Available Fields

**Stream Domain (Channels):**
- `channel_name`, `tvg_name`, `tvg_id`, `tvg_logo`
- `group_title`, `stream_url`, `stream_type`
- `channel_number`, `ext_id`
- `language`, `country`, `is_adult`
- `source_name`, `source_type`, `source_url` (injected metadata)

**EPG Domain (Programs):**
- `programme_title`, `programme_description`, `programme_category`
- `programme_start`, `programme_stop`
- `programme_icon`, `programme_rating`
- `channel_id`, `is_new`, `is_premiere`, `is_live`

---

## Pipeline System

The pipeline (`internal/pipeline/`) processes data through configurable stages.

### Pipeline Stages

```
┌─────────────────┐
│ Ingestion Guard │  Waits for active ingestions to complete (optional)
└────────┬────────┘
         ▼
┌─────────────────┐
│  Load Channels  │  Load channels from database for proxy sources
└────────┬────────┘
         ▼
┌─────────────────┐
│  Load Programs  │  Load EPG programs for channels from EPG sources
└────────┬────────┘
         ▼
┌─────────────────┐
│  Data Mapping   │  Apply transformation rules to channels/programs
└────────┬────────┘
         ▼
┌─────────────────┐
│   Filtering     │  Apply include/exclude filter rules
└────────┬────────┘
         ▼
┌─────────────────┐
│   Numbering     │  Assign channel numbers
└────────┬────────┘
         ▼
┌─────────────────┐
│  Logo Caching   │  Download and cache logos (optional)
└────────┬────────┘
         ▼
┌─────────────────┐
│  Generate M3U   │  Create M3U playlist file
└────────┬────────┘
         ▼
┌─────────────────┐
│ Generate XMLTV  │  Create XMLTV EPG file
└────────┬────────┘
         ▼
┌─────────────────┐
│    Publish      │  Move files to final output location
└─────────────────┘
```

### Numbering Modes

| Mode | Description |
|------|-------------|
| `preserve` | Keep original channel numbers, resolve conflicts |
| `sequential` | Assign sequential numbers from starting point |
| `source_based` | Prefix with source priority (e.g., 1001, 1002...) |

### Logo Caching

Logos are cached to the filesystem with metadata sidecars:

```
data/logos/
├── ab/
│   └── cdef123456.png          # Cached logo
│       cdef123456.png.meta     # Metadata (URL, ETag, expires)
```

**Features:**
- Content-addressable storage (SHA256 hash)
- HTTP cache headers respected (ETag, Cache-Control)
- Automatic cleanup of unused logos
- In-memory index for fast lookups

---

## Scheduled Jobs

The job system (`internal/scheduler/`) provides reliable background task execution.

### Components

**Scheduler** (`scheduler.go`):
- Parses cron expressions (robfig/cron/v3)
- Syncs scheduled jobs from sources/EPG/proxies
- Creates jobs in database for execution

**Executor** (`executor.go`):
- Dispatches jobs to registered handlers
- Records execution results and history
- Handles retry scheduling on failure

**Runner** (`runner.go`):
- Worker pool with configurable concurrency
- Polls database for pending jobs
- Concurrent-safe job acquisition (SELECT FOR UPDATE SKIP LOCKED)
- Stale job recovery for crashed workers
- Automatic cleanup of old jobs/history

### Job Lifecycle

```
┌─────────┐     ┌───────────┐     ┌─────────┐     ┌───────────┐
│ PENDING │ ──▶ │  RUNNING  │ ──▶ │COMPLETED│  or │  FAILED   │
└─────────┘     └───────────┘     └─────────┘     └─────────┘
                     │                                  │
                     │ (if retries remaining)           │
                     └──────────┐                       │
                                ▼                       │
                          ┌───────────┐                 │
                          │ SCHEDULED │ ◀───────────────┘
                          └───────────┘
                          (with backoff)
```

### Retry Strategy

Exponential backoff with configurable base:
- Attempt 1: immediate
- Attempt 2: base × 1 (e.g., 60s)
- Attempt 3: base × 2 (e.g., 120s)
- Attempt 4: base × 4 (e.g., 240s)
- Maximum backoff capped at 1 hour

---

## REST API

### Endpoints

**Stream Sources:**
```
GET    /api/v1/sources              List all sources
GET    /api/v1/sources/{id}         Get source by ID
POST   /api/v1/sources              Create source
PUT    /api/v1/sources/{id}         Update source
DELETE /api/v1/sources/{id}         Delete source
POST   /api/v1/sources/{id}/ingest  Trigger ingestion
```

**EPG Sources:**
```
GET    /api/v1/epg-sources              List all EPG sources
GET    /api/v1/epg-sources/{id}         Get EPG source by ID
POST   /api/v1/epg-sources              Create EPG source
PUT    /api/v1/epg-sources/{id}         Update EPG source
DELETE /api/v1/epg-sources/{id}         Delete EPG source
POST   /api/v1/epg-sources/{id}/ingest  Trigger EPG ingestion
```

**Stream Proxies:**
```
GET    /api/v1/proxies                      List all proxies
GET    /api/v1/proxies/{id}                 Get proxy by ID
POST   /api/v1/proxies                      Create proxy
PUT    /api/v1/proxies/{id}                 Update proxy
DELETE /api/v1/proxies/{id}                 Delete proxy
POST   /api/v1/proxies/{id}/generate        Trigger generation
PUT    /api/v1/proxies/{id}/sources         Set stream sources
PUT    /api/v1/proxies/{id}/epg-sources     Set EPG sources
```

**Filters:**
```
GET    /api/v1/filters              List all filters
GET    /api/v1/filters/{id}         Get filter by ID
POST   /api/v1/filters              Create filter
PUT    /api/v1/filters/{id}         Update filter
DELETE /api/v1/filters/{id}         Delete filter
```

**Data Mapping Rules:**
```
GET    /api/v1/rules                List all rules
GET    /api/v1/rules/{id}           Get rule by ID
POST   /api/v1/rules                Create rule
PUT    /api/v1/rules/{id}           Update rule
DELETE /api/v1/rules/{id}           Delete rule
```

**Jobs:**
```
GET    /api/v1/jobs                         List all jobs
GET    /api/v1/jobs/{id}                    Get job by ID
GET    /api/v1/jobs/pending                 List pending jobs
GET    /api/v1/jobs/running                 List running jobs
GET    /api/v1/jobs/history                 Get job history
GET    /api/v1/jobs/stats                   Get job statistics
GET    /api/v1/jobs/runner                  Get runner status
POST   /api/v1/jobs/{id}/cancel             Cancel job
DELETE /api/v1/jobs/{id}                    Delete completed job
POST   /api/v1/jobs/trigger/stream/{id}     Trigger stream ingestion
POST   /api/v1/jobs/trigger/epg/{id}        Trigger EPG ingestion
POST   /api/v1/jobs/trigger/proxy/{id}      Trigger proxy generation
POST   /api/v1/jobs/cron/validate           Validate cron expression
```

**Expressions:**
```
POST   /api/v1/expressions/validate   Validate expression syntax
```

**Progress (SSE):**
```
GET    /api/v1/progress/stream        SSE stream for real-time updates
```

**Health:**
```
GET    /health                        Health check endpoint
```

---

## Configuration

Configuration via YAML file or environment variables:

```yaml
server:
  host: "0.0.0.0"
  port: 8080

database:
  path: "./data/tvarr.db"

storage:
  data_dir: "./data"
  logo_cache_dir: "./data/logos"

logging:
  level: "info"      # debug, info, warn, error
  format: "json"     # json or text

scheduler:
  worker_count: 2
  poll_interval: "5s"
  job_timeout: "1h"
  cleanup_age: "168h"  # 7 days
```

**Environment Variables:**
- `TVARR_SERVER_HOST`
- `TVARR_SERVER_PORT`
- `TVARR_DATABASE_PATH`
- `TVARR_STORAGE_DATA_DIR`
- `TVARR_LOGGING_LEVEL`

---

## Building & Running

### Prerequisites

- Go 1.21+
- SQLite3

### Build

```bash
go build -o tvarr ./cmd/tvarr
```

### Run

```bash
./tvarr serve --config config.yaml
```

### Docker

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o tvarr ./cmd/tvarr

FROM alpine:latest
COPY --from=builder /app/tvarr /usr/local/bin/
EXPOSE 8080
CMD ["tvarr", "serve"]
```

---

## Project Structure

```
tvarr/
├── cmd/tvarr/              # CLI application
│   └── cmd/                # Cobra commands
├── internal/
│   ├── config/             # Configuration loading
│   ├── database/           # Database setup & migrations
│   ├── expression/         # Expression engine (lexer, parser, evaluator)
│   │   └── helpers/        # @time:, @logo: helpers
│   ├── http/               # HTTP server
│   │   ├── handlers/       # API handlers
│   │   └── middleware/     # HTTP middleware
│   ├── ingestor/           # Source ingestion handlers
│   ├── models/             # GORM models
│   ├── pipeline/           # Data processing pipeline
│   │   ├── core/           # Pipeline infrastructure
│   │   ├── shared/         # Shared utilities
│   │   └── stages/         # Pipeline stages
│   ├── repository/         # Data access layer
│   ├── scheduler/          # Job scheduling system
│   ├── service/            # Business logic layer
│   └── storage/            # File storage (logos)
├── pkg/
│   ├── bytesize/           # Human-readable byte sizes
│   ├── duration/           # Human-readable durations
│   ├── m3u/                # M3U parser/writer
│   ├── xmltv/              # XMLTV parser/writer
│   └── xtream/             # Xtream Codes API client
└── specs/                  # Feature specifications
```

---

## License

[To be determined]
