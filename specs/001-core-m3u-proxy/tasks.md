# Task Breakdown: Core M3U Proxy Service

**Feature Branch**: `001-core-m3u-proxy`  
**Created**: 2025-11-29  
**Status**: Draft

## Phase 1: Setup (BLOCKING)

All tasks in this phase must complete before any user story work.

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T001 | | Setup | Initialize Go module `github.com/jmylchreest/tvarr` with Go 1.25.4 |
| T002 | | Setup | Create project directory structure per plan.md |
| T003 | | Setup | Create Taskfile.yml with targets: build, test, lint, run, migrate |
| T004 | | Setup | Configure golangci-lint with strict settings (.golangci.yml) |
| T005 | | Setup | Implement config package with Viper (config.go, config_test.go) |
| T006 | | Setup | Implement observability/logger.go with slog structured logging |
| T007 | | Setup | Create config.example.yaml with all configuration options |

## Phase 2: Database Foundation (BLOCKING)

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T010 | | Found | Implement database/database.go - connection manager with GORM |
| T011 | | Found | Implement database multi-driver support (SQLite, PostgreSQL, MySQL) |
| T012 | | Found | Create database migration framework |
| T013 | | Found | Implement repository/interfaces.go - all repository interfaces |
| T014 | | Found | Write integration tests for database connection |
| T015 | | Found | Implement storage/sandbox.go - sandboxed file manager (needed for atomic publish) |

## Phase 3: User Story 1 - Stream Source Management (P1)

### Models & Repository

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T020 | P | US1 | Write tests for StreamSource model |
| T021 | | US1 | Implement models/stream_source.go |
| T022 | P | US1 | Write tests for Channel model |
| T023 | | US1 | Implement models/channel.go |
| T024 | P | US1 | Write tests for ManualStreamChannel model |
| T025 | | US1 | Implement models/manual_stream_channel.go |
| T026 | | US1 | Create migration for stream_sources table |
| T027 | | US1 | Create migration for channels table |
| T028 | | US1 | Create migration for manual_stream_channels table |
| T029 | P | US1 | Write tests for StreamSourceRepository |
| T030 | | US1 | Implement repository/stream_source_repo.go |
| T031 | P | US1 | Write tests for ChannelRepository (including batch operations) |
| T032 | | US1 | Implement repository/channel_repo.go with FindInBatches |

### Ingestors

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T033 | | US1 | Implement ingestor/interfaces.go - SourceHandler interface |
| T034 | P | US1 | Write tests for M3U parser (streaming, compressed) |
| T035 | | US1 | Implement pkg/m3u/parser.go - streaming M3U parser |
| T036 | | US1 | Implement ingestor/m3u_handler.go |
| T037 | P | US1 | Write tests for Xtream handler |
| T038 | | US1 | Implement ingestor/xtream_handler.go |
| T039 | | US1 | Implement ingestor/factory.go - handler factory |
| T040 | | US1 | Implement ingestor/state_manager.go - ingestion state tracking |

### Service Layer

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T041 | P | US1 | Write tests for SourceService |
| T042 | | US1 | Implement service/source_service.go |
| T043 | | US1 | Write integration tests for source ingestion flow |

## Phase 4: User Story 2 - EPG Source Management (P1)

### Models & Repository

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T050 | P | US2 | Write tests for EpgSource model |
| T051 | | US2 | Implement models/epg_source.go |
| T052 | P | US2 | Write tests for EpgProgram model |
| T053 | | US2 | Implement models/epg_program.go |
| T054 | | US2 | Create migration for epg_sources table |
| T055 | | US2 | Create migration for epg_programs table |
| T056 | P | US2 | Write tests for EpgSourceRepository |
| T057 | | US2 | Implement repository/epg_source_repo.go |
| T058 | P | US2 | Write tests for EpgProgramRepository (batch operations critical) |
| T059 | | US2 | Implement repository/epg_program_repo.go with CreateInBatches |

### Ingestors

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T060 | P | US2 | Write tests for XMLTV parser (streaming) |
| T061 | | US2 | Implement ingestor/xmltv_handler.go - streaming XML parser |
| T062 | P | US2 | Write tests for Xtream EPG handler |
| T063 | | US2 | Implement ingestor/xtream_epg_handler.go |

### Service Layer

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T064 | P | US2 | Write tests for EpgService |
| T065 | | US2 | Implement service/epg_service.go |
| T066 | | US2 | Write integration tests for EPG ingestion flow |

## Phase 5: User Story 3 - Proxy Configuration (P1)

### Models & Repository

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T070 | P | US3 | Write tests for StreamProxy model |
| T071 | | US3 | Implement models/stream_proxy.go |
| T072 | | US3 | Create migration for stream_proxies table |
| T073 | | US3 | Create migration for proxy association tables (proxy_sources, proxy_epg_sources, proxy_filters) |
| T074 | P | US3 | Write tests for StreamProxyRepository |
| T075 | | US3 | Implement repository/stream_proxy_repo.go |

### Pipeline Core

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T076 | | US3 | Implement pipeline/interfaces.go - PipelineStage, PipelineState |
| T077 | | US3 | Implement pipeline/orchestrator.go |
| T078 | | US3 | Implement pipeline/factory.go |
| T079 | P | US3 | Write tests for M3U generation stage |
| T080 | | US3 | Implement pkg/m3u/writer.go - streaming M3U writer |
| T081 | | US3 | Implement pipeline/stages/generation.go |
| T082 | P | US3 | Write tests for XMLTV generation stage |
| T083 | | US3 | Implement XMLTV streaming writer |
| T084 | | US3 | Implement pipeline/stages/publish.go - atomic file publish |

### Service Layer

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T085 | P | US3 | Write tests for ProxyService |
| T086 | | US3 | Implement service/proxy_service.go |
| T087 | | US3 | Write integration tests for proxy generation flow |

## Phase 6: User Story 10 - REST API (P1)

### HTTP Infrastructure

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T090 | | US10 | Implement http/server.go - HTTP server setup with Chi |
| T091 | P | US10 | Implement http/middleware/logging.go |
| T092 | P | US10 | Implement http/middleware/recovery.go |
| T093 | P | US10 | Implement http/middleware/cors.go |
| T094 | P | US10 | Implement http/middleware/request_id.go |
| T095 | | US10 | Implement http/routes.go - route registration |

### Handlers

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T096 | P | US10 | Write tests for source handlers |
| T097 | | US10 | Implement http/handlers/source_handler.go |
| T098 | P | US10 | Write tests for EPG handlers |
| T099 | | US10 | Implement http/handlers/epg_handler.go |
| T100 | P | US10 | Write tests for proxy handlers |
| T101 | | US10 | Implement http/handlers/proxy_handler.go |
| T102 | | US10 | Implement http/handlers/output_handler.go - serve M3U/XMLTV |
| T103 | | US10 | Implement http/handlers/health_handler.go |
| T103a | P | US10 | Write tests for graceful shutdown (NFR-006) |
| T103b | | US10 | Implement graceful shutdown in http/server.go with configurable timeout (default: 30s) |

### CLI Entry Point

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T104 | | US10 | Implement cmd/tvarr/main.go with Cobra |
| T105 | | US10 | Add serve command to start HTTP server |
| T106 | | US10 | Add migrate command for database migrations |
| T107 | | US10 | Write integration tests for API endpoints |

## Phase 7: User Story 4 - Data Mapping Rules (P2)

### Expression Engine

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T110 | P | US4 | Write comprehensive tests for expression parser |
| T111 | | US4 | Implement expression/parser.go - tokenizer and parser |
| T112 | | US4 | Implement expression/evaluator.go - expression evaluation |
| T113 | | US4 | Implement expression/field_registry.go - field aliases |
| T114 | | US4 | Add regex support with capture groups |
| T115 | | US4 | Write fuzzing tests for expression parser |

### Models & Pipeline

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T116 | P | US4 | Write tests for DataMappingRule model |
| T117 | | US4 | Implement models/data_mapping_rule.go |
| T118 | | US4 | Create migration for data_mapping_rules table |
| T119 | P | US4 | Write tests for DataMappingRuleRepository |
| T120 | | US4 | Implement repository/data_mapping_rule_repo.go |
| T121 | P | US4 | Write tests for data mapping pipeline stage |
| T122 | | US4 | Implement pipeline/stages/data_mapping.go |

### Service & API

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T123 | P | US4 | Write tests for MappingService |
| T124 | | US4 | Implement service/mapping_service.go |
| T125 | | US4 | Implement http/handlers/mapping_handler.go |
| T126 | | US4 | Write integration tests for mapping flow |

## Phase 8: User Story 5 - Filtering Rules (P2)

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T130 | P | US5 | Write tests for Filter model |
| T131 | | US5 | Implement models/filter.go |
| T132 | | US5 | Create migration for filters table |
| T133 | P | US5 | Write tests for FilterRepository |
| T134 | | US5 | Implement repository/filter_repo.go |
| T135 | P | US5 | Write tests for filtering pipeline stage |
| T136 | | US5 | Implement pipeline/stages/filtering.go |
| T137 | | US5 | Add boolean logic (AND, OR, NOT, parentheses) to expression engine |
| T138 | P | US5 | Write tests for FilterService |
| T139 | | US5 | Implement service/filter_service.go |
| T140 | | US5 | Implement http/handlers/filter_handler.go |
| T141 | | US5 | Write integration tests for filtering flow |

## Phase 9: User Story 6 - Channel Numbering (P2)

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T150 | P | US6 | Write tests for numbering pipeline stage |
| T151 | | US6 | Implement pipeline/stages/numbering.go |
| T152 | | US6 | Implement numbering configuration (base number, group-based) |
| T153 | | US6 | Write integration tests for numbering |

## Phase 10: User Story 7 - Logo Caching (P2)

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T160 | P | US7 | Write tests for LogoAsset model |
| T161 | | US7 | Implement models/logo_asset.go |
| T162 | | US7 | Create migration for logo_assets table |
| T163 | P | US7 | Write tests for LogoAssetRepository |
| T164 | | US7 | Implement repository/logo_asset_repo.go |
| T165 | P | US7 | Write tests for logo cache storage |
| T166 | | US7 | Implement storage/logo_cache.go |
| T167 | P | US7 | Write tests for logo caching pipeline stage |
| T168 | | US7 | Implement pipeline/stages/logo_caching.go |
| T169 | | US7 | Implement logo cleanup job |
| T170 | P | US7 | Write tests for LogoService |
| T171 | | US7 | Implement service/logo_service.go |
| T172 | | US7 | Write integration tests for logo caching |

## Phase 11: User Story 9 - Scheduled Jobs (P2)

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T180 | P | US9 | Write tests for Job model |
| T181 | | US9 | Implement models/job.go |
| T182 | | US9 | Create migration for jobs table |
| T183 | P | US9 | Write tests for JobRepository |
| T184 | | US9 | Implement repository/job_repo.go |
| T185 | P | US9 | Write tests for scheduler |
| T186 | | US9 | Implement scheduler/scheduler.go - cron-based scheduler |
| T187 | | US9 | Implement scheduler/queue.go - job queue |
| T188 | | US9 | Implement scheduler/executor.go - job execution |
| T189 | | US9 | Implement scheduler/runner.go - queue runner |
| T190 | P | US9 | Write tests for JobService |
| T191 | | US9 | Implement service/job_service.go |
| T192 | | US9 | Implement http/handlers/job_handler.go |
| T193 | | US9 | Write integration tests for scheduling |

## Phase 12: User Story 8 - Stream Relay (P3)

### Models & Repository

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T200 | P | US8 | Write tests for RelayProfile model |
| T201 | | US8 | Implement models/relay_profile.go |
| T202 | P | US8 | Write tests for LastKnownCodec model |
| T203 | | US8 | Implement models/last_known_codec.go |
| T204 | | US8 | Create migrations for relay_profiles, last_known_codecs tables |
| T205 | P | US8 | Write tests for RelayProfileRepository |
| T206 | | US8 | Implement repository/relay_profile_repo.go |
| T207 | P | US8 | Write tests for LastKnownCodecRepository |
| T208 | | US8 | Implement repository/last_known_codec_repo.go |

### FFmpeg Integration

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T210 | | US8 | Implement ffmpeg/binary.go - binary detection |
| T211 | | US8 | Implement ffmpeg/hwaccel.go - hardware acceleration detection |
| T212 | P | US8 | Write tests for FFmpeg wrapper |
| T213 | | US8 | Implement relay/ffmpeg_wrapper.go - process management |
| T214 | P | US8 | Write tests for stream prober |
| T215 | | US8 | Implement relay/stream_prober.go - ffprobe integration |

### Relay System

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T216 | P | US8 | Write tests for circuit breaker |
| T217 | | US8 | Implement relay/circuit_breaker.go |
| T218 | P | US8 | Write tests for HLS collapser |
| T219 | | US8 | Implement relay/hls_collapser.go |
| T220 | | US8 | Implement relay/connection_pool.go - per-host concurrency |
| T221 | P | US8 | Write tests for relay manager |
| T222 | | US8 | Implement relay/manager.go |

### Service & API

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T223 | P | US8 | Write tests for RelayService |
| T224 | | US8 | Implement service/relay_service.go |
| T225 | | US8 | Implement http/handlers/relay_handler.go |
| T226 | | US8 | Write integration tests for relay flow |

## Phase 13: FFmpeg Embedding (Optional)

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T230 | | Opt | Research go-ffstatic vs go-ffmpreg for embedding strategy |
| T231 | | Opt | Implement ffmpeg/embedded/ - binary embedding |
| T232 | | Opt | Implement binary extraction and selection logic |
| T233 | | Opt | Write tests for embedded binary management |
| T234 | | Opt | Document build process with embedded binaries |

## Phase 14: Polish

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T240 | P | Polish | Implement OpenAPI documentation generation |
| T241 | P | Polish | Implement observability/tracing.go - OpenTelemetry setup |
| T242 | P | Polish | Implement observability/metrics.go - Prometheus metrics |
| T243 | | Polish | Write performance tests for 100k channel ingestion |
| T244 | | Polish | Write performance tests for 1M EPG program ingestion |
| T244a | | Polish | Write stability/soak test for 24/7 operation (SC-006) - long-running scheduled jobs |
| T245 | | Polish | Create Dockerfile |
| T246 | | Polish | Create docker-compose.yml with database options |
| T247 | | Polish | Write quickstart guide |
| T248 | | Polish | Final code review and cleanup |

## Parallel Execution Opportunities

### Within Setup Phase
- T003, T004, T005, T006, T007 can run in parallel after T001, T002

### Within User Story Phases
- Model tests (T020, T022, T024) can run in parallel
- Repository tests can run in parallel once models complete
- Handler tests can run in parallel

### Across User Stories
- US1 and US2 can start in parallel after Phase 2
- US4 and US5 can start in parallel after Phase 6
- US6 and US7 can start in parallel
- US8 and US9 can start in parallel

## Dependencies

```
Phase 1 (Setup) → Phase 2 (Database) → [US1, US2] in parallel
                                      ↓
                                     US3 (needs US1, US2)
                                      ↓
                                     US10 (API)
                                      ↓
                               [US4, US5] in parallel
                                      ↓
                               [US6, US7, US9] in parallel
                                      ↓
                                     US8 (Relay)
                                      ↓
                               Phase 13, 14 (Optional, Polish)
```

## Effort Estimates (Story Points)

| Phase | Story Points | Notes |
|-------|--------------|-------|
| Phase 1: Setup | 5 | Foundation work |
| Phase 2: Database | 8 | Critical infrastructure |
| Phase 3: US1 | 13 | Complex parsing, streaming |
| Phase 4: US2 | 13 | Similar complexity to US1 |
| Phase 5: US3 | 13 | Pipeline architecture |
| Phase 6: US10 | 8 | Standard REST API |
| Phase 7: US4 | 13 | Expression engine is complex |
| Phase 8: US5 | 5 | Builds on US4 |
| Phase 9: US6 | 3 | Straightforward |
| Phase 10: US7 | 8 | File management |
| Phase 11: US9 | 8 | Job scheduling |
| Phase 12: US8 | 21 | FFmpeg integration complex |
| Phase 13: Optional | 13 | Embedding complexity |
| Phase 14: Polish | 8 | Documentation, testing |
| **Total** | **139** | |
