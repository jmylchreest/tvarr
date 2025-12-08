# Research: Codebase Cleanup & Migration Compaction

**Feature**: 010-codebase-cleanup
**Date**: 2025-12-07

## Executive Summary

Comprehensive analysis reveals significant cleanup opportunities:
- **Backend (Go)**: ~100 golangci-lint warnings (primarily errcheck)
- **Frontend (TypeScript)**: 422 ESLint warnings
- **Migrations**: 34 migrations (1766 lines) → can compact to 2-3
- **Dead Code**: 8 unused functions, 8 unused struct fields, 2 unused constants
- **Wrapper Methods**: 1 confirmed duplicate method, 1 questionable wrapper
- **Memory Optimization**: 10+ locations for zero-allocation/pre-allocation improvements

---

## 1. Go Lint Analysis (golangci-lint)

### Summary: ~100 errcheck warnings across codebase

### High-Priority Files (Internal/Production Code)

#### 1.1 Command Layer (`cmd/`)

**`cmd/tvarr/cmd/root.go`** (2 issues)
- Lines 52-53: `viper.BindPFlag` return values unchecked

**`cmd/tvarr/cmd/serve.go`** (10 issues)
- Lines 71-80: Multiple `viper.BindPFlag` return values unchecked
- **Fix**: Wrap in helper function that panics on error (startup-time only)

**`cmd/e2e-runner/main.go`** (37 issues)
- Lines throughout: `json.Marshal`, `io.ReadAll`, `w.Write` return values unchecked
- **Note**: E2E runner is test infrastructure, lower priority but should still be fixed

#### 1.2 FFmpeg Integration (`internal/ffmpeg/`)

**`internal/ffmpeg/binary.go`** (3 issues)
- Lines 236-237: `strconv.Atoi` for version parsing
- Line 490: `json.MarshalIndent` for debug output
- **Fix**: These are parsing best-effort scenarios; add explicit `_ =` or proper handling

**`internal/ffmpeg/process_monitor.go`** (5 issues)
- Lines 227-228, 263-264, 316: `strconv.Parse*` for process stats
- **Fix**: Best-effort metrics; use `_ =` to acknowledge or return errors

**`internal/ffmpeg/wrapper.go`** (10 issues)
- Lines 756-791: Progress parsing from FFmpeg output
- **Fix**: Progress is best-effort; acknowledge with `_ =` or proper handling

#### 1.3 HTTP Handlers (`internal/http/handlers/`)

**`internal/http/handlers/docs.go`** (1 issue)
- Line 122: `w.Write` unchecked
- **Fix**: HTTP writes should handle errors for proper logging

**`internal/http/handlers/logo.go`** (1 issue)
- Line 182: `io.Copy` unchecked
- **Fix**: Log copy errors for debugging

**`internal/http/handlers/output.go`** (2 issues)
- Lines 170, 201: `w.Write` unchecked
- **Fix**: Handle or log write errors

**`internal/http/handlers/static.go`** (2 issues)
- Line 55: `w.Write` unchecked
- Line 94: `assets.GetStaticFS` unchecked
- **Fix**: Handle errors properly

#### 1.4 Pipeline Stages (`internal/pipeline/stages/`)

**`internal/pipeline/stages/generatem3u/stage.go`** (1 issue)
- Line 130: `file.Stat` unchecked

**`internal/pipeline/stages/generatexmltv/stage.go`** (1 issue)
- Line 180: `file.Stat` unchecked

**`internal/pipeline/stages/publish/stage.go`** (4 issues)
- Lines 74, 77, 93, 96: Type assertions unchecked
- **Fix**: Use two-value form `val, ok := x.(type)` for safety

#### 1.5 Relay System (`internal/relay/`)

**`internal/relay/hls_passthrough.go`** (2 issues)
- Lines 199, 203: `fmt.Sscanf` unchecked
- **Fix**: Best-effort parsing; acknowledge with `_ =`

**`internal/relay/profile_tester.go`** (5 issues)
- Lines 280-404: `strconv.Parse*` and `regexp.MatchString` unchecked
- **Fix**: Profile testing is best-effort; proper handling

#### 1.6 Service Layer (`internal/service/`)

**`internal/service/epg_service.go`** (5 issues)
- Lines 223, 284, 397, 457, 466: `s.epgSourceRepo.Update` unchecked
- **Note**: These are "fire and forget" stat updates; consider logging errors

**`internal/service/progress/service.go`** (6 issues)
- Lines 429-498: `updateOperationThrottled/Immediate` unchecked
- **Fix**: Progress updates are fire-and-forget; acknowledge with `_ =`

#### 1.7 Scheduler (`internal/scheduler/`)

**`internal/scheduler/runner.go`** (2 issues)
- Lines 344, 346: `GetPending` and `GetRunning` unchecked in debug context
- **Fix**: Log errors for debugging

### Recommended Fix Strategy

1. **Create helper function for viper.BindPFlag** - wrap with panic for startup errors
2. **Use `_ =` explicitly** for intentionally ignored returns (FFmpeg parsing, progress updates)
3. **Add error logging** for HTTP write errors and file operations
4. **Use two-value type assertions** for pipeline stage type safety

---

## 2. Frontend ESLint Analysis

### Summary: 422 warnings across frontend codebase

### Warning Categories

#### 2.1 Unused Variables/Imports (~150 warnings)
**Pattern**: `'X' is defined but never used`

**Top offenders**:
- `src/app/channels/page.tsx`: 13 unused imports/variables
- `src/app/epg/page.tsx`: 14 unused imports/variables
- `src/components/CreateProxyModal.tsx`: 24 unused variables + any types
- `src/components/NotificationBell.tsx`: 7 warnings

**Fix**: Remove unused imports and variables

#### 2.2 React Hooks Dependencies (~30 warnings)
**Pattern**: `React Hook useEffect has missing dependencies`

**Files**:
- `src/app/epg/page.tsx`: 2 useEffect dependency issues
- `src/components/ConflictNotification.tsx`: 1 issue
- `src/components/CreateProxyModal.tsx`: 1 issue
- `src/components/NotificationBell.tsx`: 4 issues
- `src/components/backend-unavailable.tsx`: 1 issue

**Fix**: Add dependencies or use `useCallback` properly

#### 2.3 `any` Types (~100 warnings)
**Pattern**: `Unexpected any. Specify a different type`

**High-volume files**:
- `src/components/CreateProxyModal.tsx`: 12 any types
- `src/app/channels/page.tsx`: 2 any types
- `src/app/epg/page.tsx`: 1 any type

**Fix**: Define proper TypeScript interfaces

#### 2.4 Next.js Image Optimization (~15 warnings)
**Pattern**: `Using <img> could result in slower LCP`

**Files**:
- `src/app/channels/page.tsx`: 4 img elements
- `src/app/epg/page.tsx`: 3 img elements
- `src/components/autocomplete-popup.tsx`: 1 img element

**Fix**: Use `next/image` component or configure loader

### Recommended Fix Strategy

1. **Batch 1**: Remove unused imports/variables (quick wins)
2. **Batch 2**: Fix useEffect dependencies with `useCallback`
3. **Batch 3**: Replace `any` with proper types (create interfaces)
4. **Batch 4**: Convert `<img>` to `next/image` where appropriate

---

## 3. Dead Code Analysis

### 3.1 Unused Functions (8 items)

| Function | File | Line | Reason |
|----------|------|------|--------|
| `parseFormatParams()` | `relay_stream.go` | 203 | Unused helper |
| `streamRawHLSCollapsed()` | `relay_stream.go` | 734 | Incomplete refactoring |
| `shouldIncludeChannel()` | `filtering/stage.go` | 431 | Unused filter method |
| `shouldIncludeProgram()` | `filtering/stage.go` | 464 | Unused filter method |
| `updateOperation()` | `progress/service.go` | 263 | Superseded by throttled version |
| `updateOperationSilent()` | `progress/service.go` | 281 | Never called |
| `recalculateProgressImmediate()` | `progress/service.go` | 565 | Never called |
| `logoMetadataExtensionFromContentType()` | `logo_metadata.go` | 363 | Duplicate of function in `logo_cache.go` |

### 3.2 Unused Struct Fields (8 items)

| Field | Struct | File | Line |
|-------|--------|------|------|
| `progressCh` | `Wrapper` | `ffmpeg/wrapper.go` | 32 |
| `errorCh` | `Wrapper` | `ffmpeg/wrapper.go` | 33 |
| `cached` | `logoCacheResult` | `logocaching/stage.go` | 53 |
| `skipped` | `logoCacheResult` | `logocaching/stage.go` | 54 |
| `duration` | `cachedSegment` | `hls_passthrough.go` | 66 |
| `firstPTS` | `SegmentExtractor` | `segment_extractor.go` | 65 |
| `ptsSet` | `SegmentExtractor` | `segment_extractor.go` | 66 |
| `accumulatedDuration` | `SegmentExtractor` | `segment_extractor.go` | 69 |

### 3.3 Unused Constants (2 items)

| Constant | File | Value |
|----------|------|-------|
| `httpPrefix` | `m3u_handler.go` | `"http://"` |
| `httpsPrefix` | `m3u_handler.go` | `"https://"` |

---

## 4. Wrapper Methods Analysis

### 4.1 Confirmed Duplicate: `GetEnabledByPriority`

**File**: `internal/repository/relay_profile_mapping_repo.go`

**Issue**: `GetEnabledByPriority()` (lines 66-75) is identical to `GetEnabled()` (lines 53-62)

Both methods:
- Query `is_enabled = true`
- Order by `priority ASC, created_at ASC`
- Return `[]*models.RelayProfileMapping`

**Action**: Remove `GetEnabledByPriority`, update 3 call sites in `relay_profile_mapping_service.go`

### 4.2 Questionable Wrapper: `DeleteOld`

**File**: `internal/repository/epg_program_repo.go`

```go
func (r *epgProgramRepo) DeleteOld(ctx context.Context) (int64, error) {
    before := time.Now().Add(-24 * time.Hour)
    return r.DeleteExpired(ctx, before)
}
```

**Analysis**: This encapsulates the 24-hour business rule. Keeping it provides:
- Single source of truth for default retention period
- Easy to change without touching callers
- Clear semantic meaning

**Action**: Keep (provides business rule encapsulation)

---

## 5. Migration Compaction Analysis

### 5.1 Current State

- **34 migrations** in `registry.go`
- **1766 lines** of migration code
- **17 tables** created across migrations
- **Multiple incremental schema changes** (relay_profiles evolved through 12 migrations)

### 5.2 Tables in Final Schema

**Core Tables (5)**:
1. `stream_sources` - M3U/Xtream stream sources
2. `channels` - Parsed channels with composite unique index
3. `manual_stream_channels` - User-defined channels
4. `epg_sources` - EPG program guide sources
5. `epg_programs` - TV program entries

**Proxy Tables (5)**:
6. `stream_proxies` - Proxy configurations
7. `proxy_sources` - proxy→stream source links
8. `proxy_epg_sources` - proxy→EPG source links
9. `proxy_filters` - proxy→filter links (priority column)
10. `proxy_mapping_rules` - proxy→rule links (priority column)

**Rule Tables (2)**:
11. `filters` - Expression-based filtering
12. `data_mapping_rules` - Field transformation rules

**Relay Tables (2)**:
13. `relay_profiles` - Transcoding configurations (final schema)
14. `relay_profile_mappings` - Client detection rules

**Job Tables (2)**:
15. `jobs` - Active job queue
16. `job_history` - Historical records

**Cache Table (1)**:
17. `last_known_codecs` - FFprobe codec cache

### 5.3 Seed Data to Preserve

**System Filters (2)**:
- "Include All Valid Stream URLs" (is_system=true)
- "Exclude Adult Content" (is_system=true)

**System Data Mapping Rules (1)**:
- "Default Timeshift Detection (Regex)" (is_system=true)

**System Relay Profiles (6)**:
- "Automatic" (default=true, is_system=true)
- "h264/AAC" (is_system=true)
- "h265/AAC" (is_system=true)
- "Passthrough" (is_system=true)
- "VP9/Opus" (is_system=true)
- "AV1/Opus" (is_system=true)

**System Relay Profile Mappings (23)**:
- Browsers: Safari, Chrome, Edge, Firefox
- Players: VLC, MPV, ffmpeg/ffplay
- Servers: Jellyfin, Plex, Emby, Kodi
- Devices: Android TV, Roku, Apple TV, Fire TV, Tizen, webOS
- Mobile: iOS, Android
- IPTV: TiviMate, IPTV Smarters, GSE Smart IPTV
- Fallback: Default (Universal)

### 5.4 Compaction Strategy

**Proposed Structure: 2 Migrations**

**Migration 001 - Schema Creation**
```go
func migration001Schema() Migration {
    return Migration{
        Version: "001",
        Description: "Create all tables with final schema",
        Up: func(tx *gorm.DB) error {
            // AutoMigrate all models in correct order
            return tx.AutoMigrate(
                &models.StreamSource{},
                &models.Channel{},
                &models.ManualStreamChannel{},
                &models.EpgSource{},
                &models.EpgProgram{},
                &models.StreamProxy{},
                &models.ProxySource{},
                &models.ProxyEpgSource{},
                &models.ProxyFilter{},
                &models.ProxyMappingRule{},
                &models.Filter{},
                &models.DataMappingRule{},
                &models.RelayProfile{},
                &models.RelayProfileMapping{},
                &models.Job{},
                &models.JobHistory{},
                &models.LastKnownCodec{},
            )
        },
    }
}
```

**Migration 002 - System Data**
```go
func migration002SystemData() Migration {
    return Migration{
        Version: "002",
        Description: "Insert system defaults (filters, rules, profiles, mappings)",
        Up: func(tx *gorm.DB) error {
            // Insert filters
            // Insert data mapping rules
            // Insert relay profiles (6)
            // Insert relay profile mappings (23)
            return nil
        },
    }
}
```

### 5.5 Estimated Impact

| Metric | Before | After | Improvement |
|--------|--------|-------|-------------|
| Migration count | 34 | 2 | 94% reduction |
| Lines of code | ~1766 | ~400 | ~77% reduction |
| Fresh DB init time | ~2s | <1s | 50%+ faster |
| Developer comprehension | Complex | Simple | Significantly easier |

---

## 6. Code Modernization Opportunities

### 6.1 Go Patterns to Update

1. **Use `errors.Is`/`errors.As`** instead of type assertions for error checking
2. **Use `context.WithCancelCause`** (Go 1.20+) for better cancellation handling
3. **Use `slog` structured logging** consistently (already mostly done)
4. **Use `slices` package** (Go 1.21+) for slice operations

### 6.2 TypeScript/React Patterns to Update

1. **Replace `any` with proper types** - create shared type definitions
2. **Use `next/image`** for image optimization
3. **Fix React hooks dependencies** - use `useCallback` properly
4. **Consider React Server Components** where appropriate

---

## 7. Memory Optimization (Zero-Allocation Patterns)

### 7.1 High-Priority Pre-allocation Opportunities

#### Pipeline Stages

| Location | Current Pattern | Optimization | Impact |
|----------|-----------------|--------------|--------|
| `numbering/stage.go:205` | `make([]channelAssignment, 0)` | `make([]channelAssignment, 0, len(channels))` | HIGH |
| `filtering/stage.go:254` | `channelFilters := make([]*compiledExpressionFilter, 0)` | Pre-allocate with `len(s.compiledExpressionFilters)` | MEDIUM |
| `filtering/stage.go:313` | `programFilters := make([]*compiledExpressionFilter, 0)` | Pre-allocate with `len(s.compiledExpressionFilters)` | MEDIUM |
| `datamapping/stage.go:61-62` | Rules slices start empty | Pre-allocate with `len(dbRules)` | MEDIUM |
| `generatexmltv/stage.go:83` | `make(map[string]bool)` | `make(map[string]bool, len(state.Channels))` | LOW |

#### Relay/Streaming

| Location | Current Pattern | Optimization | Impact |
|----------|-----------------|--------------|--------|
| `cmaf_muxer.go:98` | `fragments: make([]*FMP4Fragment, 0)` | `make([]*FMP4Fragment, 0, maxFragments)` | MEDIUM |
| `dash_passthrough.go:85-88` | Segment maps without size hint | Pre-allocate with `SegmentCacheSize` | MEDIUM |
| `dash_passthrough.go:223-224` | Map recreation on refresh | Pre-allocate with previous size | MEDIUM |
| `cyclic_buffer.go:76` | `clients: make(map[uuid.UUID]*BufferClient)` | `make(map[uuid.UUID]*BufferClient, 16)` | LOW |
| `hls_passthrough.go:283` | `currentSegments := make(map[string]bool)` | `make(map[string]bool, len(h.segmentURLs))` | LOW |

### 7.2 Memory Cleanup Patterns to Add

#### Pattern: Nil slice after use in pipeline stages
```go
// After processing, release large slices for GC
state.Channels = nil  // Allow GC to collect
state.Programs = nil  // Allow GC to collect
```

#### Pattern: Clear maps when resetting state
```go
// Instead of creating new map, clear existing
for k := range m.segments {
    delete(m.segments, k)
}
// Better: use clear() in Go 1.21+
clear(m.segments)
```

### 7.3 sync.Pool Opportunities

| Object Type | Location | Frequency | Candidate for Pool |
|-------------|----------|-----------|-------------------|
| `bytes.Buffer` | CMAF muxer | Per-segment | YES |
| `FMP4Fragment` | CMAF muxer | Per-segment | YES |
| `http.Request` | Logo fetcher | Per-logo | MAYBE |

### 7.4 Already Optimized (No Action Needed)

- `logocaching/stage.go:290` - Pre-allocates `urlsToFetch` correctly
- `circuit_breaker.go:270` - Pre-allocates stats map with known size
- `loadchannels/stage.go:66` - Pre-allocates enabledSources correctly

---

## 8. Implementation Priority

### Phase 1: Quick Wins (Low Risk)
1. Remove unused imports (Go + TypeScript)
2. Remove unused variables (TypeScript)
3. Add `_ =` for intentionally ignored returns (Go)
4. Remove duplicate `GetEnabledByPriority` method

### Phase 2: Dead Code Removal (Medium Risk)
1. Remove 8 unused functions
2. Remove 8 unused struct fields
3. Remove 2 unused constants
4. Fix React hooks dependencies

### Phase 3: Migration Compaction (Medium Risk)
1. Create compacted migrations
2. Test against fresh database
3. Verify E2E tests pass
4. Remove old migrations

### Phase 4: Memory Optimization (Low Risk)
1. Add pre-allocation hints to pipeline stages
2. Add pre-allocation hints to relay components
3. Add slice/map cleanup after use
4. Consider sync.Pool for hot-path allocations

### Phase 5: Type Safety (Medium Risk)
1. Replace `any` types with proper interfaces
2. Fix type assertions in Go code
3. Add proper error handling where meaningful

### Phase 6: Modernization (Low Risk)
1. Update Go patterns to modern idioms (clear(), errors.Is, etc.)
2. Convert `<img>` to `next/image`
3. General code consistency improvements
