# Quickstart: Codebase Cleanup & Migration Compaction

**Feature**: 010-codebase-cleanup
**Date**: 2025-12-07

## Overview

This document provides a quick reference for implementing the codebase cleanup feature. Refer to `research.md` for detailed findings and `spec.md` for requirements.

## Prerequisites

- Go 1.25.x
- Node.js 20+ with pnpm
- golangci-lint v2.6+
- ESLint 9+

## Quick Validation Commands

```bash
# Go lint (target: 0 warnings)
golangci-lint run ./...

# Frontend lint (target: 0 warnings)
cd frontend && pnpm run lint

# Build
task build

# Test
task test

# E2E
task e2e:full
```

## Implementation Phases

### Phase 1: Quick Wins (~2 hours)

```bash
# Fix Go unused imports automatically
goimports -w ./...

# Fix Frontend unused imports
cd frontend && pnpm exec eslint --fix .
```

**Manual fixes:**
- Add `_ =` prefix to intentionally ignored returns (see research.md Section 1)
- Remove duplicate `GetEnabledByPriority` method

### Phase 2: Dead Code Removal (~3 hours)

**Files to modify (see research.md Section 3):**
- `internal/http/handlers/relay_stream.go` - Remove `parseFormatParams()`, `streamRawHLSCollapsed()`
- `internal/pipeline/stages/filtering/stage.go` - Remove `shouldIncludeChannel()`, `shouldIncludeProgram()`
- `internal/service/progress/service.go` - Remove `updateOperation()`, `updateOperationSilent()`, `recalculateProgressImmediate()`
- `internal/storage/logo_metadata.go` - Remove `logoMetadataExtensionFromContentType()`
- `internal/ffmpeg/wrapper.go` - Remove unused `progressCh`, `errorCh` fields
- `internal/relay/segment_extractor.go` - Remove unused PTS fields
- `internal/ingestor/m3u_handler.go` - Remove unused constants

### Phase 3: Migration Compaction (~4 hours)

1. Create `migration001_schema.go`:
```go
func migration001Schema() Migration {
    return Migration{
        Version: "001",
        Description: "Create all tables with final schema",
        Up: func(tx *gorm.DB) error {
            return tx.AutoMigrate(
                // All models in dependency order
            )
        },
    }
}
```

2. Create `migration002_system_data.go`:
```go
func migration002SystemData() Migration {
    return Migration{
        Version: "002",
        Description: "Insert system defaults",
        Up: func(tx *gorm.DB) error {
            // Insert filters (2)
            // Insert data mapping rules (1)
            // Insert relay profiles (6)
            // Insert relay profile mappings (23)
        },
    }
}
```

3. Test:
```bash
rm -f tvarr.db  # Delete test DB
task test       # Run tests (creates fresh DB)
task e2e:full   # Run E2E tests
```

### Phase 4: Memory Optimization (~3 hours)

**Pre-allocation fixes (see research.md Section 7):**

```go
// Before
channelsNeedingNumbers := make([]channelAssignment, 0)

// After
channelsNeedingNumbers := make([]channelAssignment, 0, len(channels))
```

**Files to modify:**
- `internal/pipeline/stages/numbering/stage.go:205`
- `internal/pipeline/stages/filtering/stage.go:254,313`
- `internal/pipeline/stages/datamapping/stage.go:61-62`
- `internal/relay/cmaf_muxer.go:98`
- `internal/relay/dash_passthrough.go:85-88`

### Phase 5: Type Safety (~4 hours)

**Go type assertions:**
```go
// Before
m3uPath := state.Metadata["m3u_path"].(string)

// After
m3uPath, ok := state.Metadata["m3u_path"].(string)
if !ok {
    return fmt.Errorf("m3u_path not found in metadata")
}
```

**TypeScript any types:**
```typescript
// Before
const handleChange = (e: any) => { ... }

// After
const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => { ... }
```

### Phase 6: Modernization (~2 hours)

**Go patterns:**
```go
// Use clear() for maps (Go 1.21+)
clear(m.segments)

// Use errors.Is/As instead of type checks
if errors.Is(err, ErrNotFound) { ... }
```

**React patterns:**
```typescript
// Fix useEffect dependencies with useCallback
const fetchData = useCallback(async () => {
    // ...
}, [dependency1, dependency2]);

useEffect(() => {
    fetchData();
}, [fetchData]);
```

## Verification Checklist

- [ ] `golangci-lint run ./...` returns 0 warnings
- [ ] `pnpm run lint` returns 0 warnings
- [ ] `task build` succeeds
- [ ] `task test` passes all tests
- [ ] `task e2e:full` passes all E2E tests
- [ ] Fresh DB initialization < 1 second
- [ ] No `any` types in TypeScript
- [ ] All slice allocations have capacity hints

## Files Changed Summary

### Go Backend (~50 files)
- `cmd/tvarr/cmd/*.go` - Error handling for viper
- `cmd/e2e-runner/main.go` - Error handling cleanup
- `internal/ffmpeg/*.go` - Remove unused fields, fix errcheck
- `internal/http/handlers/*.go` - Fix write errors, remove dead code
- `internal/pipeline/stages/*.go` - Pre-allocation, dead code removal
- `internal/relay/*.go` - Pre-allocation, remove unused fields
- `internal/repository/*.go` - Remove duplicate method
- `internal/service/*.go` - Remove unused methods
- `internal/database/migrations/registry.go` - Complete rewrite

### Frontend (~30 files)
- `src/app/channels/page.tsx` - Remove unused imports, fix any types
- `src/app/epg/page.tsx` - Remove unused imports, fix hooks
- `src/components/*.tsx` - Remove unused imports, fix hooks, add types
