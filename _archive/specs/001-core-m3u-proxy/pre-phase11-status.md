# Pre-Phase 11 Status Report: TVARR vs M3U-PROXY Analysis

**Generated**: 2025-11-30
**Branch**: `001-core-m3u-proxy`
**Purpose**: Gap analysis before implementing Phase 11 (Scheduled Jobs)

## Executive Summary

A comprehensive comparison between tvarr and m3u-proxy has revealed significant gaps and differences. This document captures findings that need to be addressed before or during the remaining implementation phases.

**Completed Phases**: 1-10 (including 6.5 Expression Engine, 6.6 SSE Progress)
**Next Phase**: 11 (Scheduled Jobs)

---

## Critical Gaps (Priority Items)

### 1. Manual Source Type

| Aspect | m3u-proxy | tvarr | Status |
|--------|-----------|-------|--------|
| Source Types | M3U, Xtream, **Manual** | M3U, Xtream only | **GAP** |
| Manual Channels | Staging table materialized to channels | First-class entity but no source type | Partial |

**m3u-proxy behavior**:
- `Manual` is a third source type alongside M3U and Xtream
- Manual sources have no URL (optional)
- Channels are staged in `manual_stream_channels` table
- During ingestion, staged channels are "materialized" into the main channels table
- Supports M3U import/export for manual sources

**tvarr current state**:
- `ManualStreamChannel` model exists with `ToChannel()` conversion
- No `SourceTypeManual` enum value
- No handler or factory registration for manual sources
- No import/export M3U endpoints

**Required changes**:
1. Add `SourceTypeManual` to `SourceType` enum
2. Create `manual_handler.go` in ingestor package
3. Register manual handler in factory
4. Add manual channel endpoints to HTTP handlers
5. URL field should be optional for Manual sources

---

### 2. Ingestion Guard Pipeline Stage

| Aspect | m3u-proxy | tvarr | Status |
|--------|-----------|-------|--------|
| Guard Stage | First stage in pipeline | Not implemented | **GAP** |
| Behavior | Waits for active ingestions | N/A | Missing |

**m3u-proxy behavior**:
- First pipeline stage that executes before any processing
- Polls `IngestionStateManager` for active ingestions
- Default: 15-second polling interval, max 20 attempts (5 min total wait)
- Non-fatal: continues even if ingestion still active after timeout
- Feature-flaggable via environment variables

**Why it matters**:
- Prevents generating proxy files with partial/stale data
- Ensures data consistency when sources are being refreshed
- Critical for scheduled jobs that might overlap

**Required changes**:
1. Create `internal/pipeline/stages/ingestionguard/stage.go`
2. Stage should check `StateManager.IsAnyIngesting()`
3. Configurable polling interval and max attempts
4. Register as first stage in factory
5. Should be optional/configurable

---

### 3. Atomic File Publishing

| Aspect | m3u-proxy | tvarr | Status |
|--------|-----------|-------|--------|
| Publish Method | Atomic rename | File copy | **GAP** |
| Incomplete Files | Never served | Possible | Risk |

**m3u-proxy behavior**:
```rust
// Write to temp file, then atomic rename
temp_file.write_all(content)?;
std::fs::rename(temp_path, final_path)?;  // Atomic on same filesystem
```

**tvarr current behavior** (`internal/pipeline/stages/publish/stage.go`):
- Copies files from temp directory to output directory
- Not atomic - clients may receive partial files during copy
- No rename-based atomicity

**Required changes**:
1. Update `publish/stage.go` to use `os.Rename()` instead of copy
2. Ensure temp and output directories are on same filesystem
3. Fallback to copy-then-rename if cross-filesystem
4. Add atomic publish option to sandbox

---

### 4. Auto-EPG Linking for Xtream Sources

| Aspect | m3u-proxy | tvarr | Status |
|--------|-----------|-------|--------|
| Auto-create EPG | Yes, on stream source create | No | **GAP** |
| Credential Sharing | Automatic | Manual | Missing |
| Bidirectional Refresh | Yes, in scheduler | No | Missing |

**m3u-proxy behavior**:
1. When creating Xtream stream source:
   - System checks EPG availability via `/xmltv.php` endpoint
   - If available, automatically creates linked EpgSource with same credentials
2. Scheduler coordinates bidirectional refresh:
   - Stream refresh triggers EPG refresh (5-min grace period)
   - EPG refresh triggers Stream refresh (5-min grace period)
3. URL-based linking (sources matched by URL + type)

**Required changes**:
1. Add `check_epg_availability()` to source service
2. Update `CreateStreamSource` to auto-create EPG for Xtream
3. Add linking logic to scheduler (Phase 11)
4. Consider `LinkedXtream` tracking or URL-based matching

---

### 5. Action Shorthand in Expression Engine

| Aspect | m3u-proxy | tvarr | Status |
|--------|-----------|-------|--------|
| SET shorthand | `field = "value"` | `SET field = "value"` | **GAP** |
| SET_IF_EMPTY shorthand | `field?="value"` | `SET_IF_EMPTY field = "value"` | **GAP** |
| APPEND shorthand | `field+="value"` | `APPEND field = "value"` | **GAP** |
| REMOVE shorthand | `field-="value"` | `REMOVE field = "value"` | **GAP** |

**m3u-proxy examples**:
```
channel_name contains "BBC" SET tvg_logo = "@logo:123"        // Keyword
channel_name contains "BBC" tvg_logo = "@logo:123"            // Shorthand (implicit SET)
channel_name contains "BBC" tvg_logo?="Default Logo"          // SET_IF_EMPTY
channel_name contains "BBC" group_title+=" HD"                // APPEND
channel_name contains "BBC" tvg_name-="[UK]"                  // REMOVE
```

**Required changes**:
1. Update `lexer.go` to recognize `?=`, `+=`, `-=` as action operators
2. Update `parser.go` to handle implicit SET (field = value without keyword)
3. Update `operators.go` to map shorthand to action operators
4. Add tests for shorthand syntax
5. Maintain backward compatibility with keyword syntax

---

## Medium Priority Gaps (Need Clarification)

### 6. Channel & EPG Browsing Endpoints

**Missing endpoints**:
- `GET /api/v1/channels` - List/search channels with filtering
- `GET /api/v1/channels/proxy/{proxy_id}` - Channels for specific proxy
- `GET /api/v1/channels/{id}/probe` - Probe channel for codec info
- `GET /api/v1/epg/programs` - Browse EPG programs
- `GET /api/v1/epg/guide` - Full EPG guide

**Questions**:
- Do we need channel probing before relay implementation?
- What filtering/pagination scheme for large datasets?
- Should EPG guide be materialized or computed?

---

### 7. Kubernetes Health Probes

**Missing endpoints**:
- `GET /ready` - Kubernetes readiness probe
- `GET /live` - Kubernetes liveness probe

**Current state**:
- `/health` exists with basic info
- No separate readiness/liveness semantics

**Questions**:
- What conditions determine readiness vs liveness?
- Should readiness check database connectivity?
- Should we add detailed system metrics to health?

---

### 8. Circuit Breaker Management Endpoints

**Missing endpoints**:
- `GET /api/v1/circuit-breakers` - List all circuit breaker stats
- `GET /api/v1/circuit-breakers/services/{name}` - Get specific service
- `POST /api/v1/circuit-breakers/services/{name}/force` - Force state

**Questions**:
- Do we need runtime circuit breaker management?
- What services should have observable circuit breakers?
- Should this wait until relay implementation (Phase 12)?

---

### 9. Manual Channel Import/Export

**Missing endpoints**:
- `GET /sources/{id}/manual-channels/export.m3u` - Export as M3U
- `POST /sources/{id}/manual-channels/import-m3u` - Import from M3U

**Depends on**: Manual source type implementation (#1)

**Questions**:
- What M3U attributes should be preserved on import?
- Should import merge or replace existing manual channels?

---

### 10. Cleanup Pipeline Stage

**m3u-proxy has**:
- Dedicated cleanup stage as final pipeline stage
- Different cleanup logic for success vs error paths
- Explicit temp file management

**tvarr current state**:
- Cleanup via `defer` blocks in orchestrator
- No error-specific cleanup logic

**Questions**:
- Is defer-based cleanup sufficient?
- What resources need explicit cleanup?

---

### 11. Runtime Settings & Feature Flags

**Missing endpoints**:
- `GET /settings` - Get runtime settings
- `PUT /settings` - Update runtime settings
- `GET /api/v1/features` - Get feature flags
- `PUT /api/v1/features` - Update feature flags

**Questions**:
- What settings should be runtime-configurable?
- Should feature flags persist to database or be config-only?

---

### 12. Log Streaming

**Missing endpoints**:
- `GET /logs/stream` - SSE log streaming

**Questions**:
- Is this needed given structured logging to stdout?
- What log levels should be streamable?
- Memory implications of buffering logs?

---

### 13. Metrics & Analytics

**Missing endpoints**:
- `GET /api/v1/metrics/dashboard` - Dashboard metrics
- `GET /api/v1/metrics/realtime` - Real-time metrics
- `GET /api/v1/metrics/channels/popular` - Popular channels

**Already planned**: Prometheus `/metrics` endpoint in Phase 14

**Questions**:
- Do we need custom metrics endpoints beyond Prometheus?
- What dashboard metrics are needed?

---

### 14. Expression Engine - Per-Condition Case Sensitivity

**m3u-proxy syntax**:
```
channel_name case_sensitive contains "BBC"
channel_name not case_sensitive equals "bbc"
```

**tvarr current state**:
- Case sensitivity is global on Evaluator
- No inline modifier support

**Questions**:
- Is per-condition case sensitivity worth the complexity?
- Does current global approach cause issues?

---

### 15. Expression Engine - Conditional Action Groups

**m3u-proxy syntax**:
```
(cond1 SET action1) AND (cond2 SET action2)
```

**tvarr current state**:
- Single condition with multiple actions
- No grouped conditional actions

**Questions**:
- Is this advanced syntax needed?
- What use cases require conditional groups?

---

## TVARR Advantages (Features m3u-proxy Lacks)

These are features tvarr has that m3u-proxy does not - should be preserved:

| Feature | Location | Description |
|---------|----------|-------------|
| VOD Content Type | `Channel.StreamType` | "live", "movie", "series" categorization |
| Multiple Numbering Modes | `StreamProxy.NumberingMode` | Sequential, Preserve, Group |
| Group Numbering Size | `StreamProxy.GroupNumberingSize` | Configurable group ranges |
| EPG Retention Days | `EpgSource.RetentionDays` | Configurable EPG data retention |
| Logo Helper | `@logo:ULID` | Expression engine logo resolution |
| Status Tracking | Source/Proxy Status enums | Explicit ingestion/generation status |

---

## Pipeline Stage Comparison

| Stage | tvarr | m3u-proxy | Notes |
|-------|-------|-----------|-------|
| Ingestion Guard | **Missing** | ✅ | Add before other stages |
| Load Channels | ✅ Separate | Inline | Different approach (OK) |
| Load Programs | ✅ Separate | Inline | Different approach (OK) |
| Data Mapping | ✅ | ✅ | Similar implementation |
| Filtering | ✅ | ✅ | Similar implementation |
| Logo Caching | ✅ | ✅ | m3u-proxy more sophisticated |
| Numbering | ✅ | ✅ | tvarr has more modes |
| Generate M3U | ✅ Separate | Combined | Architectural choice (OK) |
| Generate XMLTV | ✅ Separate | Combined | Architectural choice (OK) |
| Publish | ✅ Copy-based | ✅ Atomic | **Fix needed** |
| Cleanup | Defer-based | ✅ Explicit | Consider adding |

---

## HTTP Endpoint Count Comparison

| Domain | tvarr | m3u-proxy | Gap |
|--------|-------|-----------|-----|
| Health/Monitoring | 1 | 5 | 4 |
| Stream Sources | 6 | 13 | 7 |
| EPG Sources | 6 | 8 | 2 |
| Proxies | 8 | 10 | 2 |
| Filters | 5 | 8 | 3 |
| Data Mapping | 5 | 15 | 10 |
| Expressions | 1 | 2 | 1 |
| Progress | 2 | 4+ SSE | 2+ |
| Channels | 0 | 5 | 5 |
| EPG Browsing | 0 | 4 | 4 |
| Relay | 0 | 5 | Phase 12 |
| Logos | 0 | 15 | Partial Phase 10 |
| Circuit Breakers | 0 | 5 | 5 |
| Logs | 0 | 3 | 3 |
| Settings | 0 | 4 | 4 |
| Feature Flags | 0 | 2 | 2 |
| Metrics | 0 | 4 | Phase 14 |
| Manual Channels | 0 | 4 | 4 |
| **Total** | 34 | 112+ | 78+ |

---

## Recommended Task Additions

### Priority Tasks (Add to Phase 10.5 - Pre-Phase 11)

| ID | Task | Priority |
|----|------|----------|
| T470 | Add `SourceTypeManual` and manual source handler | P0 |
| T471 | Implement ingestion guard pipeline stage | P0 |
| T472 | Update publish stage to use atomic rename | P0 |
| T473 | Implement auto-EPG linking for Xtream sources | P0 |
| T474 | Add expression engine action shorthand syntax | P0 |

### Clarification Needed Tasks (Backlog)

| ID | Task | Priority |
|----|------|----------|
| T480 | Add channel browsing endpoints | Clarify |
| T481 | Add EPG program browsing endpoints | Clarify |
| T482 | Add Kubernetes readiness/liveness probes | Clarify |
| T483 | Add circuit breaker management endpoints | Clarify |
| T484 | Add manual channel M3U import/export | Clarify |
| T485 | Add dedicated cleanup pipeline stage | Clarify |
| T486 | Add runtime settings API | Clarify |
| T487 | Add feature flags API | Clarify |
| T488 | Add log streaming SSE endpoint | Clarify |
| T489 | Add custom metrics endpoints | Clarify |
| T490 | Add per-condition case sensitivity | Clarify |
| T491 | Add conditional action groups syntax | Clarify |

---

## Wiring Verification

All current implementations are correctly wired:

| Chain | Status |
|-------|--------|
| HTTP Handler → Service → Repository | ✅ Verified |
| Pipeline Factory → Stages → Repositories | ✅ Verified |
| Expression Engine → Evaluator → Helpers | ✅ Verified |
| Progress Service → SSE Handler | ✅ Verified |
| Logo Service → Cache → Storage | ✅ Verified |

---

## Next Steps

1. **Immediate**: Add T470-T474 to tasks.md as Phase 10.5
2. **Review**: Clarify backlog items (T480-T491) for Phase 14
3. **Continue**: Proceed with Phase 11 (Scheduled Jobs) after Phase 10.5
4. **Track**: Use this document as reference during implementation
