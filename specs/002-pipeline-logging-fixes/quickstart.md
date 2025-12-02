# Quickstart: Pipeline Logging, Error Feedback, and M3U/XMLTV Generation

**Date**: 2025-12-02
**Feature Branch**: `002-pipeline-logging-fixes`

## Development Setup

### Prerequisites

```bash
# Ensure you're on the feature branch
git checkout 002-pipeline-logging-fixes

# Build and test
task build
task test
task lint
```

### Running Development Server

```bash
# Start backend with hot reload
task run:dev

# In another terminal, start frontend
cd frontend && npm run dev
```

## Implementation Order

### Phase 1: Backend - Progress Types Enhancement

1. **Add ErrorDetail type** (`internal/service/progress/types.go`)
   ```go
   type ErrorDetail struct {
       Stage      string `json:"stage"`
       Message    string `json:"message"`
       Technical  string `json:"technical,omitempty"`
       Suggestion string `json:"suggestion,omitempty"`
   }
   ```

2. **Update UniversalProgress** (same file)
   - Add `ErrorDetail *ErrorDetail` field
   - Add `WarningCount int` field
   - Add `Warnings []string` field

3. **Add FailWithDetail method** (`internal/service/progress/bridge.go`)
   ```go
   func (m *OperationManager) FailWithDetail(detail ErrorDetail) {
       // Set error and error_detail
   }
   ```

### Phase 2: Backend - Pipeline Stage Logging

For each stage in `internal/pipeline/stages/*/stage.go`:

1. **Add logger field to stage struct**
   ```go
   type Stage struct {
       shared.BaseStage
       channelRepo repository.ChannelRepository
       logger      *slog.Logger  // NEW
   }
   ```

2. **Inject logger in constructor**
   ```go
   func NewConstructor() core.StageConstructor {
       return func(deps *core.Dependencies) core.Stage {
           return &Stage{
               BaseStage:   shared.NewBaseStage(StageID, StageName),
               channelRepo: deps.ChannelRepo,
               logger:      deps.Logger,
           }
       }
   }
   ```

3. **Add logging to Execute method**
   ```go
   func (s *Stage) Execute(ctx context.Context, state *core.State) (*core.StageResult, error) {
       // Log source processing
       for _, source := range state.Sources {
           s.logger.InfoContext(ctx, "processing source",
               slog.String("source_id", source.ID.String()),
               slog.String("source_name", source.Name),
           )
           // ... existing logic
       }
       return result, nil
   }
   ```

### Phase 3: Backend - Startup Cleanup

1. **Create cleanup package** (`internal/startup/cleanup.go`)
   ```go
   package startup

   func CleanupOrphanedTempDirs(logger *slog.Logger) error {
       pattern := filepath.Join(os.TempDir(), "tvarr-proxy-*")
       // Find and remove directories older than 1 hour
   }
   ```

2. **Call from main** (`cmd/tvarr/cmd/serve.go`)
   ```go
   if err := startup.CleanupOrphanedTempDirs(logger); err != nil {
       logger.Warn("failed to cleanup orphaned dirs", slog.String("error", err.Error()))
   }
   ```

### Phase 4: Frontend - Error Display

1. **Update TypeScript types** (`frontend/src/types/progress.ts`)
   ```typescript
   interface ErrorDetail {
     stage: string;
     message: string;
     technical?: string;
     suggestion?: string;
   }
   ```

2. **Add error indicator to progress display**
   - Show red badge when state is "error"
   - Display error_detail.message in toast
   - Show suggestion if available

## Testing the Feature

### Manual Testing

1. **Test successful pipeline**
   ```bash
   # Create a proxy with sources
   # Trigger generation from UI
   # Verify logs show stage progress
   # Verify M3U and XMLTV files created
   ```

2. **Test error handling**
   ```bash
   # Configure invalid output directory
   # Trigger generation
   # Verify UI shows error with suggestion
   ```

3. **Test cleanup**
   ```bash
   # Kill server during generation (simulate crash)
   # Verify orphaned temp dir exists
   # Restart server
   # Verify orphaned temp dir removed
   ```

### Automated Tests

```bash
# Run all tests
task test

# Run specific stage tests
go test ./internal/pipeline/stages/... -v

# Run progress service tests
go test ./internal/service/progress/... -v
```

## Logging Examples

### Expected INFO Logs (default level)

```
level=INFO msg="starting pipeline execution" proxy_id=01JDGE3... proxy_name="My Proxy" stage_count=10
level=INFO msg="executing stage" stage_num=1 total_stages=10 stage_id=load_channels stage_name="Load Channels"
level=INFO msg="processing source" source_id=01JDGE2... source_name="My Source" channel_count=5000
level=INFO msg="stage completed" stage_id=load_channels duration=1.234s records_processed=5000
level=INFO msg="pipeline execution completed" proxy_id=01JDGE3... channel_count=5000 duration=45.678s
```

### Expected DEBUG Logs (debug level)

```
level=DEBUG msg="processing batch" batch_num=3 total_batches=10 items_start=2001 items_end=3000
level=DEBUG msg="applying filter rule" rule_id=01JDGE4... rule_name="Remove Adult"
```

### Expected ERROR Logs

```
level=ERROR msg="stage failed" stage_id=generate_m3u stage_name="Generate M3U" error="permission denied: /output/proxy.m3u" duration=0.123s
```

## Files Changed Summary

| Category | Files |
|----------|-------|
| Progress Types | `internal/service/progress/types.go`, `bridge.go` |
| Pipeline Stages | `internal/pipeline/stages/*/stage.go` (all 10 stages) |
| Orchestrator | `internal/pipeline/core/orchestrator.go` |
| Dependencies | `internal/pipeline/core/interfaces.go` (add Logger) |
| Startup | `internal/startup/cleanup.go` (NEW) |
| Server | `cmd/tvarr/cmd/serve.go` |
| Frontend Types | `frontend/src/types/progress.ts` |
| Frontend Components | `frontend/src/components/progress-*.tsx` |

## Troubleshooting

### Logs not appearing

- Check log level: `TVARR_LOG_LEVEL=debug`
- Verify logger is injected into stages

### Progress events not updating

- Check SSE connection in browser DevTools
- Verify progress service is started

### Temp directories not cleaned

- Check temp dir pattern matches `tvarr-proxy-*`
- Verify cleanup runs at startup (check logs)

### M3U/XMLTV not generated

- Check output directory exists and is writable
- Check for stage errors in logs
- Verify sources have channels
