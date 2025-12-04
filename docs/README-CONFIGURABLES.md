# tvarr Configuration Reference

This document provides a comprehensive reference for all configuration options available in tvarr.

## Configuration Methods

Configuration can be set via:
1. **YAML config file** - `.tvarr.yaml` in `.`, `/etc/tvarr`, or `$HOME`
2. **Environment variables** - Prefixed with `TVARR_`, nested with underscores (e.g., `TVARR_SERVER_PORT`)
3. **Command-line flags** - Where applicable

**Precedence order:** CLI flags > Environment variables > Config file > Defaults

---

## Command-Line Interface

### Global Flags (all commands)

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `$HOME/.tvarr.yaml` | Path to config file |
| `--log-level` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `--log-format` | `text` | Log format: `text`, `json` |

### Serve Command Flags

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--host` | `TVARR_SERVER_HOST` | `0.0.0.0` | Host to bind to |
| `--port` | `TVARR_SERVER_PORT` | `8080` | Port to listen on |
| `--base-url` | `TVARR_SERVER_BASE_URL` | `` | Base URL for external access (e.g., `https://mysite.com`). Used for logo URLs in generated M3U/XMLTV. Defaults to `http://host:port` |
| `--database` | `TVARR_DATABASE_DSN` | `tvarr.db` | Database DSN (file path for SQLite) |
| `--data-dir` | `TVARR_STORAGE_BASE_DIR` | `./data` | Data directory for output files |
| `--ingestion-guard` | `TVARR_PIPELINE_INGESTION_GUARD` | `true` | Enable ingestion guard (waits for active ingestions before proxy generation) |

---

## Server Configuration

| Config Key | Environment Variable | Default | Description |
|------------|---------------------|---------|-------------|
| `server.host` | `TVARR_SERVER_HOST` | `0.0.0.0` | IP address to bind the HTTP server |
| `server.port` | `TVARR_SERVER_PORT` | `8080` | Port to listen on |
| `server.base_url` | `TVARR_SERVER_BASE_URL` | `` | Base URL for external access (e.g., `https://mysite.com`). Used for generating fully qualified logo URLs in M3U/XMLTV output. Defaults to `http://host:port` |
| `server.read_timeout` | `TVARR_SERVER_READ_TIMEOUT` | `30s` | Maximum duration for reading request |
| `server.write_timeout` | `TVARR_SERVER_WRITE_TIMEOUT` | `30s` | Maximum duration for writing response |
| `server.shutdown_timeout` | `TVARR_SERVER_SHUTDOWN_TIMEOUT` | `10s` | Graceful shutdown timeout |
| `server.cors_origins` | `TVARR_SERVER_CORS_ORIGINS` | `["*"]` | Allowed CORS origins |

---

## Database Configuration

| Config Key | Environment Variable | Default | Description |
|------------|---------------------|---------|-------------|
| `database.driver` | `TVARR_DATABASE_DRIVER` | `sqlite` | Database driver: `sqlite`, `postgres`, `mysql` |
| `database.dsn` | `TVARR_DATABASE_DSN` | `tvarr.db` | Data source name / connection string |
| `database.max_open_conns` | `TVARR_DATABASE_MAX_OPEN_CONNS` | `10` | Maximum open connections |
| `database.max_idle_conns` | `TVARR_DATABASE_MAX_IDLE_CONNS` | `5` | Maximum idle connections |
| `database.conn_max_lifetime` | `TVARR_DATABASE_CONN_MAX_LIFETIME` | `1h` | Maximum connection lifetime |
| `database.conn_max_idle_time` | `TVARR_DATABASE_CONN_MAX_IDLE_TIME` | `30m` | Maximum idle time before closing |
| `database.log_level` | `TVARR_DATABASE_LOG_LEVEL` | `warn` | DB log level: `silent`, `error`, `warn`, `info` |

---

## Storage Configuration

| Config Key | Environment Variable | Default | Description |
|------------|---------------------|---------|-------------|
| `storage.base_dir` | `TVARR_STORAGE_BASE_DIR` | `./data` | Base directory for all storage |
| `storage.logo_dir` | `TVARR_STORAGE_LOGO_DIR` | `logos` | Subdirectory for cached logos |
| `storage.output_dir` | `TVARR_STORAGE_OUTPUT_DIR` | `output` | Subdirectory for generated files |
| `storage.temp_dir` | `TVARR_STORAGE_TEMP_DIR` | `temp` | Subdirectory for temporary files |
| `storage.logo_retention` | `TVARR_STORAGE_LOGO_RETENTION` | `720h` | Logo cache retention (30 days) |
| `storage.max_logo_size` | `TVARR_STORAGE_MAX_LOGO_SIZE` | `5242880` | Max logo size in bytes (5MB) |

---

## Logging Configuration

| Config Key | Environment Variable | Default | Description |
|------------|---------------------|---------|-------------|
| `logging.level` | `TVARR_LOGGING_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `logging.format` | `TVARR_LOGGING_FORMAT` | `json` | Log format: `json`, `text` |
| `logging.add_source` | `TVARR_LOGGING_ADD_SOURCE` | `false` | Include source location in logs |
| `logging.time_format` | `TVARR_LOGGING_TIME_FORMAT` | `RFC3339` | Timestamp format |

---

## Ingestion Configuration

Controls how tvarr fetches and processes M3U/XMLTV sources.

| Config Key | Environment Variable | Default | Description |
|------------|---------------------|---------|-------------|
| `ingestion.channel_batch_size` | `TVARR_INGESTION_CHANNEL_BATCH_SIZE` | `1000` | Batch size for channel processing |
| `ingestion.epg_batch_size` | `TVARR_INGESTION_EPG_BATCH_SIZE` | `5000` | Batch size for EPG/program processing |
| `ingestion.http_timeout` | `TVARR_INGESTION_HTTP_TIMEOUT` | `60s` | HTTP request timeout |
| `ingestion.max_concurrent` | `TVARR_INGESTION_MAX_CONCURRENT` | `3` | Max concurrent source fetches |
| `ingestion.retry_attempts` | `TVARR_INGESTION_RETRY_ATTEMPTS` | `3` | Number of retry attempts |
| `ingestion.retry_delay` | `TVARR_INGESTION_RETRY_DELAY` | `5s` | Delay between retries |

### Ingestion Memory Limits

Controls disk-backed slice behavior during source ingestion. When memory threshold is exceeded, data automatically spills to disk.

| Config Key | Environment Variable | Default | Description |
|------------|---------------------|---------|-------------|
| `ingestion.memory_limits.channels_threshold` | `TVARR_INGESTION_MEMORY_LIMITS_CHANNELS_THRESHOLD` | `524288000` | Channel data memory limit (500MB). Set to `0` for unlimited. |
| `ingestion.memory_limits.programs_threshold` | `TVARR_INGESTION_MEMORY_LIMITS_PROGRAMS_THRESHOLD` | `524288000` | Program/EPG data memory limit (500MB). Set to `0` for unlimited. |

---

## Pipeline Configuration

Controls the proxy generation pipeline (filtering, data mapping, M3U/XMLTV generation).

| Config Key | Environment Variable | Default | Description |
|------------|---------------------|---------|-------------|
| `pipeline.stream_batch_size` | `TVARR_PIPELINE_STREAM_BATCH_SIZE` | `1000` | Batch size for stream processing |
| `pipeline.enable_gc_hints` | `TVARR_PIPELINE_ENABLE_GC_HINTS` | `true` | Enable GC hints between stages |
| `pipeline.logo_batch_size` | `TVARR_PIPELINE_LOGO_BATCH_SIZE` | `50` | Batch size for logo caching |
| `pipeline.ingestion_guard` | `TVARR_PIPELINE_INGESTION_GUARD` | `true` | Enable ingestion guard (waits for active ingestions before generating proxies) |

### Pipeline Memory Limits

Controls disk-backed slice behavior during pipeline execution.

| Config Key | Environment Variable | Default | Description |
|------------|---------------------|---------|-------------|
| `pipeline.memory_limits.channels_threshold` | `TVARR_PIPELINE_MEMORY_LIMITS_CHANNELS_THRESHOLD` | `524288000` | Channel data memory limit (500MB). Set to `0` for unlimited. |
| `pipeline.memory_limits.programs_threshold` | `TVARR_PIPELINE_MEMORY_LIMITS_PROGRAMS_THRESHOLD` | `524288000` | Program/EPG data memory limit (500MB). Set to `0` for unlimited. |

---

## Relay Configuration

Controls the stream relay/proxy functionality.

| Config Key | Environment Variable | Default | Description |
|------------|---------------------|---------|-------------|
| `relay.enabled` | `TVARR_RELAY_ENABLED` | `false` | Enable stream relay |
| `relay.max_concurrent_streams` | `TVARR_RELAY_MAX_CONCURRENT_STREAMS` | `10` | Maximum concurrent relayed streams |
| `relay.circuit_breaker_threshold` | `TVARR_RELAY_CIRCUIT_BREAKER_THRESHOLD` | `3` | Failures before circuit opens |
| `relay.circuit_breaker_timeout` | `TVARR_RELAY_CIRCUIT_BREAKER_TIMEOUT` | `30s` | Circuit breaker reset timeout |
| `relay.connection_pool_size` | `TVARR_RELAY_CONNECTION_POOL_SIZE` | `100` | HTTP connection pool size |
| `relay.stream_timeout` | `TVARR_RELAY_STREAM_TIMEOUT` | `5m` | Stream idle timeout |

---

## FFmpeg Configuration

Controls FFmpeg binary detection and hardware acceleration.

| Config Key | Environment Variable | Default | Description |
|------------|---------------------|---------|-------------|
| `ffmpeg.binary_path` | `TVARR_FFMPEG_BINARY_PATH` | `` | Path to ffmpeg binary (empty = auto-detect) |
| `ffmpeg.probe_path` | `TVARR_FFMPEG_PROBE_PATH` | `` | Path to ffprobe binary (empty = auto-detect) |
| `ffmpeg.use_embedded` | `TVARR_FFMPEG_USE_EMBEDDED` | `false` | Use embedded ffmpeg if available |
| `ffmpeg.hwaccel_priority` | `TVARR_FFMPEG_HWACCEL_PRIORITY` | `["vaapi","nvenc","qsv","amf"]` | Hardware acceleration priority order |

---

## Memory Limits Deep Dive

The memory limits control the [disk-backed slice](architecture/disk-backed-slice.md) behavior. This feature allows tvarr to handle very large datasets without running out of memory.

### How It Works

1. Data is initially stored in memory (fast access)
2. When memory usage exceeds the threshold, data spills to disk (JSONL format)
3. Iteration and access continues transparently
4. Temporary files are automatically cleaned up

### Configuration Guidelines

| System RAM | Recommended Threshold |
|------------|----------------------|
| 2GB | `200MB` (209715200) |
| 4GB | `300MB` (314572800) |
| 8GB+ | `500MB` (524288000) |

### Disabling Disk-Backed Storage

Set thresholds to `0` to disable disk spilling and keep all data in memory:

```yaml
ingestion:
  memory_limits:
    channels_threshold: 0
    programs_threshold: 0
pipeline:
  memory_limits:
    channels_threshold: 0
    programs_threshold: 0
```

This is useful for:
- Systems with abundant RAM
- Datasets known to be small
- Maximum performance requirements

---

## Example Configuration File

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  base_url: ""  # e.g., "https://mysite.com" - used for logo URLs in M3U/XMLTV
  read_timeout: 30s
  write_timeout: 30s
  shutdown_timeout: 10s
  cors_origins:
    - "*"

database:
  driver: sqlite
  dsn: tvarr.db
  max_open_conns: 10
  max_idle_conns: 5
  log_level: warn

storage:
  base_dir: ./data
  logo_dir: logos
  output_dir: output
  temp_dir: temp
  logo_retention: 720h
  max_logo_size: 5242880

logging:
  level: info
  format: json
  add_source: false

ingestion:
  channel_batch_size: 1000
  epg_batch_size: 5000
  http_timeout: 60s
  max_concurrent: 3
  retry_attempts: 3
  retry_delay: 5s
  memory_limits:
    channels_threshold: 524288000  # 500MB
    programs_threshold: 524288000  # 500MB

pipeline:
  stream_batch_size: 1000
  enable_gc_hints: true
  logo_batch_size: 50
  ingestion_guard: true  # Wait for active ingestions before proxy generation
  memory_limits:
    channels_threshold: 524288000  # 500MB
    programs_threshold: 524288000  # 500MB

relay:
  enabled: false
  max_concurrent_streams: 10
  circuit_breaker_threshold: 3
  circuit_breaker_timeout: 30s
  connection_pool_size: 100
  stream_timeout: 5m

ffmpeg:
  binary_path: ""
  probe_path: ""
  use_embedded: false
  hwaccel_priority:
    - vaapi
    - nvenc
    - qsv
    - amf
```

---

## Environment Variable Examples

```bash
# Server
export TVARR_SERVER_PORT=9090
export TVARR_SERVER_HOST=127.0.0.1

# Database
export TVARR_DATABASE_DSN="postgres://user:pass@localhost/tvarr?sslmode=disable"
export TVARR_DATABASE_DRIVER=postgres

# Logging
export TVARR_LOGGING_LEVEL=debug
export TVARR_LOGGING_FORMAT=text

# Memory limits (512MB for channels, 1GB for programs)
export TVARR_PIPELINE_MEMORY_LIMITS_CHANNELS_THRESHOLD=536870912
export TVARR_PIPELINE_MEMORY_LIMITS_PROGRAMS_THRESHOLD=1073741824

# Disable disk-backed storage (unlimited memory)
export TVARR_PIPELINE_MEMORY_LIMITS_CHANNELS_THRESHOLD=0
export TVARR_PIPELINE_MEMORY_LIMITS_PROGRAMS_THRESHOLD=0
```
