# Implementation Plan: Core M3U Proxy Service

**Feature Branch**: `001-core-m3u-proxy`  
**Created**: 2025-11-29  
**Status**: Draft

## Technical Context

| Aspect | Decision |
|--------|----------|
| **Language/Version** | Go 1.25.4 (latest stable) |
| **Primary Dependencies** | GORM v2, Huma v2 + Chi, Viper, Cobra, testify |
| **Storage** | GORM with SQLite/PostgreSQL/MySQL support |
| **Testing** | testify + gomock, table-driven tests |
| **Target Platform** | Linux (primary), macOS, Windows |
| **Performance Goals** | 100k channels < 500MB RAM, 1M EPG entries < 1GB RAM |
| **Scale** | Large datasets, streaming/batched processing mandatory |

## Constitution Check

| Principle | Compliance |
|-----------|------------|
| Memory-First Design | All processing uses streaming/batching. Configurable batch sizes. |
| Modular Pipeline Architecture | Pipeline stages implement common interface. Composable design. |
| Test-First Development | Tests written before implementation. Contract tests for interfaces. |
| SOLID Principles | Repository pattern, service layer, dependency injection. |
| Idiomatic Go | Standard error handling, context propagation, defer cleanup. |
| Observable/Debuggable | Structured logging with slog. |
| Security by Default | Sandboxed file operations, input validation. |

## Project Structure

```
tvarr/
├── cmd/
│   └── tvarr/
│       └── main.go                 # CLI entry point
├── internal/
│   ├── config/                     # Configuration loading (Viper)
│   │   ├── config.go
│   │   └── config_test.go
│   ├── database/                   # Database connection and migrations
│   │   ├── database.go
│   │   ├── migrations/
│   │   └── database_test.go
│   ├── models/                     # GORM models (entities)
│   │   ├── stream_source.go
│   │   ├── channel.go
│   │   ├── epg_source.go
│   │   ├── epg_program.go
│   │   ├── stream_proxy.go
│   │   ├── filter.go
│   │   ├── data_mapping_rule.go
│   │   ├── relay_profile.go
│   │   ├── logo_asset.go
│   │   ├── last_known_codec.go
│   │   └── job.go
│   ├── repository/                 # Data access layer (Repository pattern)
│   │   ├── interfaces.go           # Repository interfaces
│   │   ├── stream_source_repo.go
│   │   ├── channel_repo.go
│   │   ├── epg_source_repo.go
│   │   ├── epg_program_repo.go
│   │   ├── stream_proxy_repo.go
│   │   ├── filter_repo.go
│   │   ├── data_mapping_rule_repo.go
│   │   ├── relay_profile_repo.go
│   │   ├── logo_asset_repo.go
│   │   ├── last_known_codec_repo.go
│   │   └── job_repo.go
│   ├── service/                    # Business logic layer
│   │   ├── source_service.go       # Source CRUD + ingestion orchestration
│   │   ├── epg_service.go          # EPG CRUD + ingestion orchestration
│   │   ├── proxy_service.go        # Proxy CRUD + generation orchestration
│   │   ├── filter_service.go
│   │   ├── mapping_service.go
│   │   ├── relay_service.go
│   │   ├── job_service.go
│   │   └── logo_service.go
│   ├── ingestor/                   # Source ingestion logic
│   │   ├── interfaces.go           # SourceHandler interface
│   │   ├── factory.go              # Handler factory
│   │   ├── m3u_handler.go          # M3U parsing
│   │   ├── xtream_handler.go       # Xtream API
│   │   ├── xmltv_handler.go        # XMLTV parsing
│   │   ├── xtream_epg_handler.go   # Xtream EPG
│   │   └── state_manager.go        # Ingestion state tracking
│   ├── pipeline/                   # Pipeline architecture
│   │   ├── interfaces.go           # PipelineStage interface
│   │   ├── orchestrator.go         # Pipeline execution
│   │   ├── factory.go              # Orchestrator factory
│   │   └── stages/
│   │       ├── ingestion_guard.go  # Wait for active ingestion
│   │       ├── data_mapping.go     # Apply mapping rules
│   │       ├── filtering.go        # Apply filter rules
│   │       ├── numbering.go        # Channel numbering
│   │       ├── logo_caching.go     # Download/cache logos
│   │       ├── generation.go       # M3U/XMLTV generation
│   │       └── publish.go          # Atomic file publish
│   ├── expression/                 # Expression engine
│   │   ├── parser.go               # Expression parser
│   │   ├── evaluator.go            # Expression evaluation
│   │   ├── field_registry.go       # Field name aliases
│   │   └── parser_test.go
│   ├── relay/                      # Stream relay system
│   │   ├── manager.go              # Relay orchestration
│   │   ├── ffmpeg_wrapper.go       # FFmpeg process management
│   │   ├── stream_prober.go        # ffprobe integration
│   │   ├── circuit_breaker.go      # Failure handling
│   │   ├── hls_collapser.go        # HLS → TS conversion
│   │   └── connection_pool.go      # Per-host concurrency
│   ├── ffmpeg/                     # FFmpeg binary management
│   │   ├── binary.go               # Binary detection/embedding
│   │   ├── hwaccel.go              # Hardware acceleration detection
│   │   └── embedded/               # Embedded binaries (optional)
│   ├── scheduler/                  # Job scheduling
│   │   ├── scheduler.go            # Cron-based scheduler
│   │   ├── queue.go                # Job queue
│   │   ├── executor.go             # Job execution
│   │   └── runner.go               # Queue runner
│   ├── storage/                    # File storage
│   │   ├── sandbox.go              # Sandboxed file manager
│   │   ├── logo_cache.go           # Logo cache implementation
│   │   └── output_manager.go       # Proxy output files
│   ├── http/                       # HTTP layer
│   │   ├── server.go               # HTTP server setup
│   │   ├── middleware/
│   │   │   ├── logging.go
│   │   │   ├── recovery.go
│   │   │   ├── cors.go
│   │   │   └── request_id.go
│   │   ├── handlers/               # HTTP handlers
│   │   │   ├── source_handler.go
│   │   │   ├── epg_handler.go
│   │   │   ├── proxy_handler.go
│   │   │   ├── filter_handler.go
│   │   │   ├── mapping_handler.go
│   │   │   ├── relay_handler.go
│   │   │   ├── job_handler.go
│   │   │   ├── health_handler.go
│   │   │   └── output_handler.go   # Serve M3U/XMLTV files
│   │   └── routes.go               # Route registration
│   └── observability/              # Logging
│       └── logger.go               # slog setup
├── pkg/                            # Exported packages (if needed)
│   └── m3u/                        # M3U parser (potentially reusable)
│       ├── parser.go
│       └── writer.go
├── tests/
│   ├── integration/                # Integration tests
│   ├── contract/                   # Contract tests
│   └── fixtures/                   # Test data
├── configs/
│   └── config.example.yaml         # Example configuration
├── scripts/
│   └── migrations/                 # SQL migrations (if needed)
├── go.mod
├── go.sum
├── Taskfile.yml
├── .golangci.yml
└── Dockerfile
```

## Architecture Decisions

### 1. Repository Pattern with GORM

All database access goes through repository interfaces. This enables:
- Easy testing via mock repositories
- Database backend switching (SQLite/PostgreSQL/MySQL)
- Consistent batch operations

```go
type ChannelRepository interface {
    Create(ctx context.Context, channel *models.Channel) error
    CreateInBatches(ctx context.Context, channels []*models.Channel, batchSize int) error
    FindBySourceID(ctx context.Context, sourceID uint, callback func(*models.Channel) error) error
    Update(ctx context.Context, channel *models.Channel) error
    Delete(ctx context.Context, id uint) error
    DeleteBySourceID(ctx context.Context, sourceID uint) error
}
```

### 2. Streaming Processing with Callbacks

Large dataset processing uses callbacks instead of slices:

```go
// Instead of returning []Channel (memory intensive)
func (r *channelRepo) FindBySourceID(ctx context.Context, sourceID uint, callback func(*models.Channel) error) error {
    return r.db.WithContext(ctx).
        Where("source_id = ?", sourceID).
        FindInBatches(&[]models.Channel{}, 1000, func(tx *gorm.DB, batch int) error {
            for _, channel := range *tx.Statement.Dest.(*[]models.Channel) {
                if err := callback(&channel); err != nil {
                    return err
                }
            }
            return nil
        }).Error
}
```

### 3. Pipeline Stage Interface

All pipeline stages implement a common interface:

```go
type PipelineStage interface {
    Name() string
    Execute(ctx context.Context, state *PipelineState) error
}

type PipelineState struct {
    ProxyID        uint
    TempDir        string
    ChannelWriter  io.Writer  // Streaming output
    ProgramWriter  io.Writer  // Streaming output
    Stats          *StageStats
    Logger         *slog.Logger
}
```

### 4. Expression Engine

Unified parser for filters and mappings:

```go
type Expression interface {
    Evaluate(ctx *EvalContext) (bool, error)
}

type EvalContext struct {
    Fields map[string]string  // Field values
    Captures []string         // Regex capture groups
}

// Supported operators
type Operator int
const (
    OpEquals Operator = iota
    OpContains
    OpMatches      // Regex
    OpStartsWith
    OpEndsWith
    OpGreaterThan
    OpLessThan
)
```

### 5. FFmpeg Binary Strategy

Support both system and embedded FFmpeg:

```go
type FFmpegBinary interface {
    Path() string
    Available() bool
    Probe(ctx context.Context, url string) (*ProbeResult, error)
    Transcode(ctx context.Context, input string, output io.Writer, profile *RelayProfile) error
}

// Implementations:
// - SystemFFmpeg: uses system-installed ffmpeg
// - EmbeddedFFmpeg: extracts embedded binary to temp dir
// - WASMFFmpeg: uses go-ffmpreg WASM runtime (no native binary)
```

### 6. Circuit Breaker for Relay

Per-upstream circuit breaker:

```go
type CircuitBreaker struct {
    failures     int
    state        State  // Closed, Open, HalfOpen
    lastFailure  time.Time
    threshold    int           // failures before open
    resetTimeout time.Duration // time before half-open
}
```

### 7. Dependency Injection

Use constructor injection for testability:

```go
func NewProxyService(
    proxyRepo repository.StreamProxyRepository,
    channelRepo repository.ChannelRepository,
    pipelineFactory pipeline.OrchestratorFactory,
    logger *slog.Logger,
) *ProxyService {
    return &ProxyService{
        proxyRepo:       proxyRepo,
        channelRepo:     channelRepo,
        pipelineFactory: pipelineFactory,
        logger:          logger,
    }
}
```

## Memory Management Strategy

### Batch Sizes (Configurable)

| Entity | Default Batch Size | Rationale |
|--------|-------------------|-----------|
| Channels | 1000 | Balance between memory and DB round-trips |
| EPG Programs | 5000 | Larger batches for insert-heavy workload |
| Logo Downloads | 50 | Concurrent downloads, memory for images |
| M3U Generation | Streaming | Write directly to file, no buffering |
| XMLTV Generation | Streaming | Write directly to file, no buffering |

### Memory Cleanup

Explicit cleanup between pipeline stages:

```go
func (o *Orchestrator) Execute(ctx context.Context) error {
    for _, stage := range o.stages {
        if err := stage.Execute(ctx, o.state); err != nil {
            return err
        }
        // Explicit cleanup hint
        runtime.GC()
    }
    return nil
}
```

## Dependencies

### Core Dependencies

```go
// go.mod
module github.com/jmylchreest/tvarr

go 1.25.4

require (
    gorm.io/gorm v1.25.x
    gorm.io/driver/sqlite v1.5.x
    gorm.io/driver/postgres v1.5.x
    gorm.io/driver/mysql v1.5.x
    github.com/danielgtaylor/huma/v2 v2.34.x   // OpenAPI 3.1 auto-generation
    github.com/go-chi/chi/v5 v5.x.x            // Router (streaming support)
    github.com/spf13/viper v1.18.x
    github.com/spf13/cobra v1.8.x
    github.com/robfig/cron/v3 v3.0.x
    github.com/stretchr/testify v1.9.x
    github.com/golang/mock v1.6.x
)
```

### Optional Dependencies

```go
    github.com/go-ffstatic/ffstatic v0.x.x  // Embedded FFmpeg (optional)
```

## Phases

### Phase 1: Setup (Foundational)

1. Initialize Go module with dependencies
2. Set up project structure
3. Configure linting (golangci-lint)
4. Set up Taskfile.yml with common targets
5. Configure slog logging
6. Set up Viper configuration

### Phase 2: Database Layer

1. Implement GORM models for all entities
2. Implement database connection manager
3. Implement migrations
4. Implement repository interfaces
5. Implement repository implementations
6. Write integration tests for repositories

### Phase 3: User Story 1 - Stream Source Management

1. Implement StreamSource model and repository
2. Implement Channel model and repository
3. Implement M3U parser with streaming
4. Implement Xtream handler
5. Implement source ingestion service
6. Implement ingestion state manager
7. Write tests for all components

### Phase 4: User Story 2 - EPG Source Management

1. Implement EpgSource model and repository
2. Implement EpgProgram model and repository
3. Implement XMLTV parser with streaming
4. Implement Xtream EPG handler
5. Implement EPG ingestion service
6. Write tests for all components

### Phase 5: User Story 3 - Proxy Configuration

1. Implement StreamProxy model and repository
2. Implement association tables (proxy_sources, proxy_filters, etc.)
3. Implement pipeline stage interface
4. Implement generation stage (M3U writer)
5. Implement generation stage (XMLTV writer)
6. Implement publish stage (atomic file operations)
7. Implement pipeline orchestrator
8. Write tests for pipeline

### Phase 6: User Story 10 - REST API (P1)

1. Set up HTTP server with Huma + Chi
2. Configure Huma API with OpenAPI 3.1 metadata
3. Implement Chi middleware (logging, recovery, CORS, request ID)
4. Implement source operations (Huma typed handlers)
5. Implement EPG operations (Huma typed handlers)
6. Implement proxy operations (Huma typed handlers)
7. Implement streaming output handlers (M3U/XMLTV via huma.StreamResponse)
8. Implement health check endpoint
9. Enable /docs endpoint (Swagger UI via Huma)
10. Write integration tests for API

### Phase 7: User Story 4 - Data Mapping Rules

1. Implement expression parser
2. Implement field registry with aliases
3. Implement evaluator with operators
4. Implement DataMappingRule model and repository
5. Implement data mapping pipeline stage
6. Write tests for expression engine

### Phase 8: User Story 5 - Filtering Rules

1. Implement Filter model and repository
2. Implement filter pipeline stage
3. Add boolean logic (AND, OR, NOT, parentheses)
4. Write tests for filtering

### Phase 9: User Story 6 - Channel Numbering

1. Implement numbering pipeline stage
2. Implement numbering configuration
3. Write tests for numbering

### Phase 10: User Story 7 - Logo Caching

1. Implement LogoAsset model and repository
2. Implement logo cache storage
3. Implement logo caching pipeline stage
4. Implement cleanup job
5. Write tests for logo caching

### Phase 11: User Story 9 - Scheduled Jobs

1. Implement Job model and repository
2. Implement job scheduler (cron)
3. Implement job queue
4. Implement job executor
5. Implement job runner
6. Write tests for scheduling

### Phase 12: User Story 8 - Stream Relay

1. Implement RelayProfile model and repository
2. Implement FFmpeg wrapper
3. Implement stream prober (ffprobe)
4. Implement hardware acceleration detection
5. Implement circuit breaker
6. Implement HLS collapser
7. Implement connection pooling
8. Implement relay manager
9. Add relay endpoint to HTTP server
10. Write tests for relay system

### Phase 13: FFmpeg Embedding (Optional)

1. Evaluate go-ffstatic vs go-ffmpreg
2. Implement embedded binary extraction
3. Implement binary selection logic
4. Write tests for binary management

### Phase 14: Polish

1. Complete OpenAPI documentation
2. Add metrics endpoints
3. Performance testing and optimization
4. Documentation
5. Dockerfile
6. CI/CD configuration

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| Memory issues with large datasets | Streaming processing, batch operations, explicit GC |
| FFmpeg process leaks | Proper context cancellation, defer cleanup, process tracking |
| Database lock contention (SQLite) | Connection pooling, WAL mode, batch writes |
| Expression parser bugs | Comprehensive test suite, fuzzing |
| Circuit breaker flapping | Configurable thresholds, backoff |

## Testing Strategy

### Unit Tests
- All business logic in service layer
- Expression parser and evaluator
- Circuit breaker state machine

### Integration Tests
- Repository operations with real database
- Pipeline stage composition
- HTTP handler flows

### Contract Tests
- Repository interface compliance
- Pipeline stage interface compliance
- HTTP API contracts

### Performance Tests
- 100k channel ingestion
- 1M EPG program ingestion
- Concurrent relay streams
