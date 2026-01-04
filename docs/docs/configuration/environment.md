---
title: Environment Variables
description: All configuration options
sidebar_position: 1
---

# Environment Variables

All tvarr settings can be configured via environment variables.

## Naming Convention

Environment variables use the pattern:

```
TVARR_<SECTION>_<KEY>
```

For example:
- `TVARR_SERVER_PORT` → `server.port`
- `TVARR_DATABASE_DSN` → `database.dsn`

## Server Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `TVARR_SERVER_HOST` | 0.0.0.0 | Listen address |
| `TVARR_SERVER_PORT` | 8080 | HTTP port |
| `TVARR_SERVER_BASE_URL` | - | External URL for playlists |
| `TVARR_SERVER_READ_TIMEOUT` | 30s | HTTP read timeout |
| `TVARR_SERVER_WRITE_TIMEOUT` | 30s | HTTP write timeout |

## Database

| Variable | Default | Description |
|----------|---------|-------------|
| `TVARR_DATABASE_DRIVER` | sqlite | Database type |
| `TVARR_DATABASE_DSN` | /data/tvarr.db | Connection string |
| `TVARR_DATABASE_MAX_OPEN_CONNS` | 10 | Max open connections |
| `TVARR_DATABASE_LOG_LEVEL` | warn | GORM log level |

## Storage

| Variable | Default | Description |
|----------|---------|-------------|
| `TVARR_STORAGE_BASE_DIR` | /data | Base storage directory |
| `TVARR_STORAGE_LOGO_DIR` | /data/logos | Logo cache directory |
| `TVARR_STORAGE_OUTPUT_DIR` | /data/output | Generated files |
| `TVARR_STORAGE_LOGO_RETENTION` | 720h | Logo cache retention |
| `TVARR_STORAGE_MAX_LOGO_SIZE` | 5242880 | Max logo size (bytes) |

## Logging

| Variable | Default | Description |
|----------|---------|-------------|
| `TVARR_LOGGING_LEVEL` | info | Log level |
| `TVARR_LOGGING_FORMAT` | json | Log format (json, text) |
| `TVARR_LOGGING_ADD_SOURCE` | false | Include file:line |

## gRPC (Distributed Transcoding)

| Variable | Default | Description |
|----------|---------|-------------|
| `TVARR_GRPC_ENABLED` | false | Enable gRPC server |
| `TVARR_GRPC_PORT` | 9090 | gRPC listen port |
| `TVARR_GRPC_AUTH_TOKEN` | - | Authentication token |

## Relay / Transcoding

| Variable | Default | Description |
|----------|---------|-------------|
| `TVARR_RELAY_ENABLED` | false | Enable relay mode |
| `TVARR_RELAY_MAX_CONCURRENT_STREAMS` | 10 | Max concurrent streams |
| `TVARR_FFMPEG_BINARY_PATH` | /usr/bin/ffmpeg | FFmpeg path |
| `TVARR_FFMPEG_PROBE_PATH` | /usr/bin/ffprobe | FFprobe path |

## Docker Specific

| Variable | Default | Description |
|----------|---------|-------------|
| `PUID` | 1000 | User ID for files |
| `PGID` | 1000 | Group ID for files |
| `TZ` | UTC | Timezone |

## ffmpegd (Worker) Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `TVARR_COORDINATOR_URL` | - | Coordinator gRPC address |
| `TVARR_DAEMON_NAME` | hostname | Worker display name |
| `TVARR_MAX_JOBS` | auto | Max concurrent jobs |
| `TVARR_MAX_CPU_JOBS` | auto | Max software encoding jobs |
| `TVARR_MAX_GPU_JOBS` | auto | Max hardware encoding jobs |
| `TVARR_AUTH_TOKEN` | - | Authentication token |
