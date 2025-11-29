# Tvarr Roadmap

**Status**: Active  
**Last Updated**: 2025-11-29

## Vision

Tvarr is a modular, memory-efficient IPTV/streaming management platform that aggregates content from multiple sources, applies intelligent transformations, and serves curated playlists to clients. The long-term vision includes VOD content management and dynamic live channel generation.

## Release Phases

### Phase 1: Core Foundation (MVP)
**Status**: In Progress  
**Target**: Q1 2026

Core functionality matching the original Rust m3u-proxy application.

#### Deliverables
- [ ] Project infrastructure (Go 1.25, GORM, Chi, Taskfile)
- [ ] Database layer with multi-driver support (SQLite/PostgreSQL/MySQL)
- [ ] Stream source management (M3U, Xtream Codes)
- [ ] EPG source management (XMLTV, Xtream EPG)
- [ ] Basic proxy configuration
- [ ] M3U/XMLTV generation
- [ ] REST API with OpenAPI documentation
- [ ] Scheduled job system

#### Success Criteria
- Ingest 100k channels in < 5 minutes
- Ingest 1M EPG programs in < 10 minutes
- Memory usage < 500MB during ingestion
- All P1 user stories complete

---

### Phase 2: Expression Engine & Rules
**Status**: Planned  
**Target**: Q1 2026

Full expression engine with filtering and data mapping capabilities.

#### Deliverables
- [ ] Expression parser (recursive descent, AST-based)
- [ ] Field registry with aliases (American/British spelling)
- [ ] Filter expressions with AND/OR/NOT support
- [ ] Nested parenthetical grouping
- [ ] Data mapping rules with SET/?=/REMOVE actions
- [ ] Regex support with capture group substitution
- [ ] Validation with structured error reporting
- [ ] Expression helpers and preprocessors

#### Success Criteria
- Full parity with Rust expression engine
- Comprehensive test coverage for parser
- User-friendly error messages with suggestions

---

### Phase 3: Pipeline Architecture
**Status**: Planned  
**Target**: Q2 2026

Staged pipeline for proxy generation.

#### Deliverables
- [ ] Pipeline stage interface
- [ ] Ingestion guard stage
- [ ] Filtering stage (stream + EPG)
- [ ] Data mapping stage (stream + EPG)
- [ ] Channel numbering stage
- [ ] Logo caching stage
- [ ] Generation stage (M3U + XMLTV)
- [ ] Publish stage (atomic file operations)
- [ ] Pipeline orchestrator
- [ ] Progress tracking and metrics

#### Success Criteria
- Generate proxy for 50k channels in < 2 minutes
- Memory-efficient streaming processing
- Detailed progress reporting

---

### Phase 4: Logo Caching System
**Status**: Planned  
**Target**: Q2 2026

In-memory logo registry with disk caching.

#### Deliverables
- [ ] URL normalization and deterministic hashing
- [ ] Cache ID generation (SHA256)
- [ ] In-memory index with secondary indices
- [ ] Lazy loading with background scanning
- [ ] Image format conversion (to PNG)
- [ ] Dimension extraction and encoding
- [ ] LRU cache for search results
- [ ] Cleanup/retention policies

#### Success Criteria
- Handle 100k+ logos with < 50MB memory overhead
- Cache hit ratio > 90% on regeneration
- Deterministic cache IDs across runs

---

### Phase 5: FFmpeg Relay System
**Status**: Planned  
**Target**: Q2 2026

Stream relay with optional transcoding.

#### Deliverables
- [ ] FFmpeg binary detection (system + embedded)
- [ ] Hardware acceleration detection (VAAPI/NVENC/QSV/AMF)
- [ ] Redirect mode (client direct fetch)
- [ ] Proxy mode (server relay without transcode)
- [ ] Relay mode (FFmpeg transcoding)
- [ ] Relay profiles (codec/bitrate configuration)
- [ ] Cyclic buffer for efficient streaming
- [ ] Circuit breaker for upstream failures
- [ ] Connection pooling per host
- [ ] ffprobe caching (LastKnownCodec)
- [ ] HLS collapsing (variant → continuous)

#### Success Criteria
- Relay startup < 3 seconds
- Support 10+ concurrent relay streams
- Hardware acceleration working on supported hardware

---

### Phase 6: VOD Content System (Future)
**Status**: Future  
**Target**: Q3 2026

Video-on-demand content management and live channel generation.

#### Deliverables
- [ ] VOD source handlers (Xtream VOD, filesystem)
- [ ] Metadata enrichment (genre, age rating, duration)
- [ ] VOD catalog with search/filter
- [ ] Live channel generator (merge VOD → pseudo-live)
- [ ] Dynamic EPG generation from VOD metadata
- [ ] Playlist templates (genres, themes, random)
- [ ] Schedule-based channel programming

#### Use Cases
1. **Genre Channels**: Auto-generate "Action Movies" channel from VOD catalog
2. **Theme Nights**: Schedule movie marathons with generated EPG
3. **Kids Channel**: Filter by age rating, create safe viewing channel
4. **Random Shuffle**: Create surprise channel from entire VOD library

#### Success Criteria
- Generate live channel from 1000 VOD items
- EPG accuracy matches VOD runtime
- Seamless transitions between VOD items

---

### Phase 7: Advanced Features (Future)
**Status**: Future  
**Target**: Q4 2026

#### Potential Features
- [ ] Web UI for management
- [ ] Multi-tenant support
- [ ] Distributed deployment (multiple workers)
- [ ] Plugin system for custom handlers
- [ ] Webhook notifications
- [ ] Metrics dashboard
- [ ] Backup/restore functionality
- [ ] Import/export configurations

---

## Technical Debt & Improvements

### Continuous
- [ ] Increase test coverage (target: 80%+)
- [ ] Performance profiling and optimization
- [ ] Documentation improvements
- [ ] Dependency updates

### Known Areas for Improvement
- [ ] Expression engine: Consider parser generator for complex grammars
- [ ] Logo cache: Evaluate distributed caching for multi-instance deployments
- [ ] FFmpeg: Investigate pure Go alternatives for basic operations

---

## Milestone Dependencies

```
Phase 1 (Core)
    └─→ Phase 2 (Expression Engine)
          └─→ Phase 3 (Pipeline)
                ├─→ Phase 4 (Logo Cache)
                └─→ Phase 5 (FFmpeg Relay)
                      └─→ Phase 6 (VOD System)
                            └─→ Phase 7 (Advanced)
```

---

## Metrics & KPIs

### Performance
- Ingestion throughput (channels/second)
- Generation throughput (channels/second)
- Memory efficiency (peak vs. baseline)
- API response times (p50, p95, p99)

### Quality
- Test coverage percentage
- Linter warnings (target: 0)
- Bug escape rate
- Time to resolution

### User Adoption
- Number of configured sources
- Number of active proxies
- Relay usage statistics
- API call volume

---

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| FFmpeg compatibility issues | Medium | High | Test across platforms, fallback to software encoding |
| Memory pressure with large datasets | Medium | High | Streaming processing, configurable batch sizes |
| Expression engine edge cases | Medium | Medium | Comprehensive test suite, fuzzing |
| Database performance at scale | Low | High | Index optimization, query analysis |
| Hardware acceleration unavailable | Medium | Low | Graceful fallback to software |

---

## Changelog

### 2025-11-29
- Initial roadmap created
- Defined 7 phases from MVP to Advanced Features
- Established VOD system as future vision
