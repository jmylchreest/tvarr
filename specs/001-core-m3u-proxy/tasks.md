# Task Breakdown: Core M3U Proxy Service

**Feature Branch**: `001-core-m3u-proxy`  
**Created**: 2025-11-29  
**Status**: In Progress

## Implementation Status

| Phase | Status | Notes |
|-------|--------|-------|
| Phase 1: Setup | ✅ Complete | T001-T007 |
| Phase 2: Database | ✅ Complete | T010-T015 |
| Phase 3: US1 Stream Sources | ✅ Complete | T020-T043 |
| Phase 4: US2 EPG Sources | ✅ Complete | T050-T066 |
| Phase 5: US3 Proxy Config | ✅ Complete | T070-T087, pipeline refactored |
| Phase 6: US10 REST API | ✅ Complete | T090-T107 |
| **Phase 6.5: Expression Engine** | 🟡 In Progress | Core complete, API integration needed |
| Phase 6.6: SSE Progress Streaming | ⏸️ Pending | Real-time progress updates via SSE |
| Phase 7: US4 Data Mapping | ⏸️ Blocked | Needs Phase 6.5 |
| Phase 8: US5 Filtering | ⏸️ Blocked | Needs Phase 6.5 |
| Phase 9-14 | ⏸️ Pending | |

---

## Phase 6.5: Expression Engine & Pipeline Parity (BLOCKING)

**Priority**: P0 - Must complete before Phases 7-14  
**Rationale**: The pipeline stages cannot function correctly without the expression engine, helper system, and proper rule processing. Current filtering/numbering stages are non-functional placeholders.

### Gap Analysis (m3u-proxy vs tvarr)

| Feature | m3u-proxy | tvarr | Status |
|---------|-----------|-------|--------|
| Expression Parser | Full AST parser | ✅ Complete | lexer, parser, AST |
| Field Registry | Aliases, types, validation | ✅ Complete | with domain validation |
| Helper System | @logo:, @time: resolution | ✅ Complete | TimeHelper, LogoHelper |
| Rule Processor | Conditions + Actions | ✅ Complete | all action operators |
| Eval Context | Field value accessor | ✅ Complete | channel/program contexts |
| Boolean Logic | AND/OR/NOT with nesting | ✅ Complete | full support |
| Regex Captures | $1, $2 substitution | ✅ Complete | capture groups work |
| Expression Preprocessing | Symbolic ops, fused negations | ✅ Complete | matches m3u-proxy |
| Validate Expression API | POST /api/v1/expressions/validate | ✅ Complete | registered in routes |
| Filtering Stage | Expression-based filtering | ✅ Complete | uses expression engine |
| Data Mapping Stage | Expression-based transforms | ✅ Complete | stage implemented |
| Filter Model | Database persistence | ✅ Complete | migration added |
| DataMappingRule Model | Database persistence | ✅ Complete | migration added |
| Default Filters/Rules | Migration seed data | ✅ Complete | matches m3u-proxy |
| Conflict Resolution | Numbering conflicts | ❌ Pending | MEDIUM |
| Filter Repository | CRUD operations | ❌ Pending | Needed for US5 |
| DataMappingRule Repository | CRUD operations | ❌ Pending | Needed for US4 |
| Filter API Handlers | REST CRUD endpoints | ❌ Pending | Needed for US5 |
| DataMappingRule API Handlers | REST CRUD endpoints | ❌ Pending | Needed for US4 |
| Pipeline Database Integration | Load rules from DB | ❌ Pending | Final wiring |

### Expression Engine Foundation ✅ COMPLETE

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T300 | P | Expr | [X] Write tests for Lexer/Tokenizer |
| T301 | | Expr | [X] Implement expression/lexer.go - tokenize expression strings |
| T302 | P | Expr | [X] Write tests for expression AST types |
| T303 | | Expr | [X] Implement expression/ast.go - ConditionNode, ConditionTree, Action, ExtendedExpression |
| T304 | P | Expr | [X] Write tests for expression parser |
| T305 | | Expr | [X] Implement expression/parser.go - parse tokens into AST |
| T306 | | Expr | [X] Implement expression/operators.go - all filter/action operators |

### Field Registry & Aliases ✅ COMPLETE

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T310 | P | Expr | [X] Write tests for field registry |
| T311 | | Expr | [X] Implement expression/field_registry.go - field definitions |
| T312 | | Expr | [X] Define channel fields (channel_name, tvg_id, tvg_name, group_title, stream_url, etc.) |
| T313 | | Expr | [X] Define program fields (programme_title, description, category, etc.) |
| T314 | | Expr | [X] Implement alias mapping (program_title → programme_title, etc.) |
| T315 | | Expr | [X] Implement field validation per domain (stream/epg/filter/rule) |

### Expression Evaluation ✅ COMPLETE

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T320 | P | Expr | [X] Write tests for condition evaluation |
| T321 | | Expr | [X] Implement expression/evaluator.go - evaluate conditions against records |
| T322 | | Expr | [X] Implement string operators (equals, contains, starts_with, ends_with) |
| T323 | | Expr | [X] Implement negated operators (not_equals, not_contains, etc.) |
| T324 | | Expr | [X] Implement regex matching with capture group extraction |
| T325 | | Expr | [X] Implement comparison operators (greater_than, less_than, etc.) |
| T326 | | Expr | [X] Implement boolean logic (AND, OR) with short-circuit evaluation |
| T327 | | Expr | [X] Implement parentheses/nesting support |

### Eval Context System ✅ COMPLETE

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T330 | P | Expr | [X] Write tests for eval context |
| T331 | | Expr | [X] Implement expression/eval_context.go - FieldValueAccessor interface |
| T332 | | Expr | [X] Implement ChannelEvalContext - access channel fields by name |
| T333 | | Expr | [X] Implement ProgramEvalContext - access program fields by name |
| T334 | | Expr | [X] Implement source metadata injection (source_name, source_type, source_url) |

### Helper System ✅ COMPLETE

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T340 | P | Expr | [X] Write tests for helper processor interface |
| T341 | | Expr | [X] Implement expression/helpers/helper.go - HelperProcessor interface |
| T342 | P | Expr | [X] Write tests for time helper |
| T343 | | Expr | [X] Implement expression/helpers/time_helper.go - @time:now(), @time:parse() |
| T344 | P | Expr | [X] Write tests for logo helper |
| T345 | | Expr | [X] Implement expression/helpers/logo_helper.go - @logo:UUID resolution |
| T346 | | Expr | [X] Implement expression/helpers/processor.go - HelperPostProcessor registry |

### Rule Processor ✅ COMPLETE

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T350 | P | Expr | [X] Write tests for rule processor |
| T351 | | Expr | [X] Implement expression/rule_processor.go - RuleProcessor interface |
| T352 | | Expr | [X] Implement StreamRuleProcessor - apply rules to channels |
| T353 | | Expr | [X] Implement ProgramRuleProcessor - apply rules to programs |
| T354 | | Expr | [X] Implement action operators (SET, SET_IF_EMPTY, APPEND, REMOVE, DELETE) |
| T355 | | Expr | [X] Implement regex capture substitution ($1, $2, etc.) |
| T356 | | Expr | [X] Implement field modification tracking (old_value, new_value, type) |

### Data Mapping Engine ✅ COMPLETE

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T360 | P | Expr | [X] Write tests for data mapping engine |
| T361 | | Expr | [X] Implement expression/data_mapping_engine.go - DataMappingEngine interface |
| T362 | | Expr | [X] Implement ChannelDataMappingEngine - process channel records |
| T363 | | Expr | [X] Implement ProgramDataMappingEngine - process program records |
| T364 | | Expr | [X] Implement rule chaining with aggregated results |

### Filter Processor ✅ COMPLETE

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T370 | P | Expr | [X] Write tests for expression-based filter processor |
| T371 | | Expr | [X] Implement expression/filter_processor.go - FilterProcessor interface |
| T372 | | Expr | [X] Implement StreamFilterProcessor - filter channels with expressions |
| T373 | | Expr | [X] Implement ProgramFilterProcessor - filter programs with expressions |
| T374 | | Expr | [X] Update pipeline/stages/filtering/ to use new filter processor |

### Pipeline Stage Updates ✅ COMPLETE

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T380 | | Expr | [X] Add DataMappingStage to pipeline/stages/datamapping/ |
| T381 | | Expr | [X] Update FilteringStage to use expression-based filtering |
| T382 | | Expr | [X] Add helper post-processing to DataMappingStage |
| T383 | | Expr | [X] Update NumberingStage with conflict detection/resolution |
| T384 | | Expr | [X] Update factory to register stages in correct order |
| T385 | | Expr | [X] Add stage result tracking with modification counts |

### API Integration (NEW)

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T396 | | Expr | [X] Implement expression/preprocess.go - expression preprocessing |
| T397 | | Expr | [X] Implement expression/domain.go - expression domain types |
| T398 | | Expr | [X] Implement expression/validator.go - expression validation with suggestions |
| T399 | | Expr | [X] Implement http/handlers/expression.go - validate-expression endpoint |
| T400 | | Expr | [X] Register ExpressionHandler in serve.go routes |
| T401 | | Expr | [X] Implement models/filter.go - Filter database model |
| T402 | | Expr | [X] Implement models/data_mapping_rule.go - DataMappingRule database model |
| T403 | | Expr | [X] Create migration for filters table |
| T404 | | Expr | [X] Create migration for data_mapping_rules table |
| T405 | | Expr | [X] Create migration for default filters and rules |

### Integration & Testing (PARTIAL)

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T390 | P | Expr | [X] Write integration tests for expression parsing |
| T391 | P | Expr | [X] Write integration tests for rule application |
| T392 | P | Expr | [X] Write integration tests for helper resolution |
| T393 | | Expr | [ ] Write fuzzing tests for expression parser |
| T394 | | Expr | [ ] Write benchmark tests for expression evaluation |
| T395 | | Expr | [ ] End-to-end pipeline test with data mapping + filtering |

---

## Phase 6.6: SSE Progress Streaming

**Priority**: P2 - Enhances user experience for long-running operations
**Rationale**: Provides real-time feedback for ingestion, proxy regeneration, and pipeline operations. Essential for frontend integration and operation monitoring.

### Overview

This phase implements Server-Sent Events (SSE) for real-time progress streaming, matching m3u-proxy's progress system. The implementation provides:
- Centralized progress tracking across all operation types
- SSE endpoint for real-time updates with filtering
- Integration with pipeline orchestrator, ingestors, and services
- Operation blocking to prevent duplicate concurrent operations

### Progress Service

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T410 | P | SSE | [ ] Write tests for UniversalProgress types and states |
| T411 | | SSE | [ ] Implement service/progress_service.go - ProgressService struct |
| T412 | | SSE | [ ] Implement UniversalState enum (Idle, Preparing, Connecting, Downloading, Processing, Saving, Cleanup, Completed, Error, Cancelled) |
| T413 | | SSE | [ ] Implement OperationType enum (StreamIngestion, EpgIngestion, ProxyRegeneration, Pipeline, DataMapping, LogoCaching, Filtering, Maintenance, Database) |
| T414 | | SSE | [ ] Implement UniversalProgress struct with stages, timestamps, metadata |
| T415 | | SSE | [ ] Implement ProgressManager for staged operations with weighted progress calculation |
| T416 | | SSE | [ ] Implement StageUpdater for individual stage progress reporting |
| T417 | | SSE | [ ] Implement broadcast channel mechanism for SSE subscribers |
| T418 | | SSE | [ ] Implement operation blocking (prevent duplicate operations per owner) |
| T419 | | SSE | [ ] Implement automatic cleanup of completed/stale operations |

### SSE HTTP Endpoint

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T420 | P | SSE | [ ] Write tests for SSE progress handler |
| T421 | | SSE | [ ] Implement http/handlers/progress_handler.go - SSE endpoint |
| T422 | | SSE | [ ] Implement GET /api/v1/progress/events SSE stream |
| T423 | | SSE | [ ] Implement query filters (operation_type, owner_id, resource_id, state, active_only) |
| T424 | | SSE | [ ] Implement SSE keepalive heartbeat (30s interval) |
| T425 | | SSE | [ ] Implement GET /api/v1/progress/operations - list current operations |
| T426 | | SSE | [ ] Register progress routes in serve.go |

### Pipeline Integration

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T430 | P | SSE | [ ] Write tests for pipeline progress integration |
| T431 | | SSE | [ ] Update pipeline/core/orchestrator.go to use ProgressService |
| T432 | | SSE | [ ] Register pipeline stages with ProgressManager |
| T433 | | SSE | [ ] Bridge existing ProgressReporter to new ProgressService |
| T434 | | SSE | [ ] Implement stage-level progress updates during pipeline execution |

### Ingestion Integration

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T440 | P | SSE | [ ] Write tests for ingestion progress integration |
| T441 | | SSE | [ ] Update service/source_service.go to use ProgressService |
| T442 | | SSE | [ ] Update service/epg_service.go to use ProgressService |
| T443 | | SSE | [ ] Implement progress stages for ingestion (Connecting, Downloading, Processing, Saving) |
| T444 | | SSE | [ ] Track item counts (channels/programs processed vs total) |

### Service Integration

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T450 | P | SSE | [ ] Write tests for proxy regeneration progress |
| T451 | | SSE | [ ] Update service/proxy_service.go to use ProgressService |
| T452 | | SSE | [ ] Implement progress tracking for proxy regeneration |
| T453 | | SSE | [ ] Add ProgressService to application dependencies in serve.go |

### Testing

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T460 | P | SSE | [ ] Write integration tests for SSE endpoint |
| T461 | | SSE | [ ] Write tests for concurrent operation blocking |
| T462 | | SSE | [ ] Write tests for progress calculation with multiple stages |
| T463 | | SSE | [ ] Write tests for cleanup of stale operations |

---

## Phase 1: Setup (BLOCKING) ✅ COMPLETE

All tasks in this phase must complete before any user story work.

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T001 | | Setup | [X] Initialize Go module `github.com/jmylchreest/tvarr` with Go 1.25.4 |
| T002 | | Setup | [X] Create project directory structure per plan.md |
| T003 | | Setup | [X] Create Taskfile.yml with targets: build, test, lint, run, migrate |
| T004 | | Setup | [X] Configure golangci-lint with strict settings (.golangci.yml) |
| T005 | | Setup | [X] Implement config package with Viper (config.go, config_test.go) |
| T006 | | Setup | [X] Implement observability/logger.go with slog structured logging |
| T007 | | Setup | [X] Create config.example.yaml with all configuration options |

## Phase 2: Database Foundation (BLOCKING) ✅ COMPLETE

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T010 | | Found | [X] Implement database/database.go - connection manager with GORM |
| T011 | | Found | [X] Implement database multi-driver support (SQLite, PostgreSQL, MySQL) |
| T012 | | Found | [X] Create database migration framework |
| T013 | | Found | [X] Implement repository/interfaces.go - all repository interfaces |
| T014 | | Found | [X] Write integration tests for database connection |
| T015 | | Found | [X] Implement storage/sandbox.go - sandboxed file manager (needed for atomic publish) |

## Phase 3: User Story 1 - Stream Source Management (P1) ✅ COMPLETE

### Models & Repository

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T020 | P | US1 | [X] Write tests for StreamSource model |
| T021 | | US1 | [X] Implement models/stream_source.go |
| T022 | P | US1 | [X] Write tests for Channel model |
| T023 | | US1 | [X] Implement models/channel.go |
| T024 | P | US1 | [X] Write tests for ManualStreamChannel model |
| T025 | | US1 | [X] Implement models/manual_stream_channel.go |
| T026 | | US1 | [X] Create migration for stream_sources table |
| T027 | | US1 | [X] Create migration for channels table |
| T028 | | US1 | [X] Create migration for manual_stream_channels table |
| T029 | P | US1 | [X] Write tests for StreamSourceRepository |
| T030 | | US1 | [X] Implement repository/stream_source_repo.go |
| T031 | P | US1 | [X] Write tests for ChannelRepository (including batch operations) |
| T032 | | US1 | [X] Implement repository/channel_repo.go with FindInBatches |

### Ingestors

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T033 | | US1 | [X] Implement ingestor/interfaces.go - SourceHandler interface |
| T034 | P | US1 | [X] Write tests for M3U parser (streaming, compressed) |
| T035 | | US1 | [X] Implement pkg/m3u/parser.go - streaming M3U parser |
| T036 | | US1 | [X] Implement ingestor/m3u_handler.go |
| T037 | P | US1 | [X] Write tests for Xtream handler |
| T038 | | US1 | [X] Implement ingestor/xtream_handler.go |
| T039 | | US1 | [X] Implement ingestor/factory.go - handler factory |
| T040 | | US1 | [X] Implement ingestor/state_manager.go - ingestion state tracking |

### Service Layer

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T041 | P | US1 | [X] Write tests for SourceService |
| T042 | | US1 | [X] Implement service/source_service.go |
| T043 | | US1 | [X] Write integration tests for source ingestion flow |

## Phase 4: User Story 2 - EPG Source Management (P1) ✅ COMPLETE

### Models & Repository

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T050 | P | US2 | [X] Write tests for EpgSource model |
| T051 | | US2 | [X] Implement models/epg_source.go |
| T052 | P | US2 | [X] Write tests for EpgProgram model |
| T053 | | US2 | [X] Implement models/epg_program.go |
| T054 | | US2 | [X] Create migration for epg_sources table |
| T055 | | US2 | [X] Create migration for epg_programs table |
| T056 | P | US2 | [X] Write tests for EpgSourceRepository |
| T057 | | US2 | [X] Implement repository/epg_source_repo.go |
| T058 | P | US2 | [X] Write tests for EpgProgramRepository (batch operations critical) |
| T059 | | US2 | [X] Implement repository/epg_program_repo.go with CreateInBatches |

### Ingestors

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T060 | P | US2 | [X] Write tests for XMLTV parser (streaming) |
| T061 | | US2 | [X] Implement pkg/xmltv/parser.go and ingestor/xmltv_handler.go |
| T062 | P | US2 | [X] Write tests for Xtream EPG handler |
| T063 | | US2 | [X] Implement ingestor/xtream_epg_handler.go |

### Service Layer

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T064 | P | US2 | [X] Write tests for EpgService |
| T065 | | US2 | [X] Implement service/epg_service.go |
| T066 | | US2 | [X] Write integration tests for EPG ingestion flow |

## Phase 5: User Story 3 - Proxy Configuration (P1) ✅ COMPLETE

### Models & Repository

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T070 | P | US3 | [X] Write tests for StreamProxy model |
| T071 | | US3 | [X] Implement models/stream_proxy.go |
| T072 | | US3 | [X] Create migration for stream_proxies table |
| T073 | | US3 | [X] Create migration for proxy association tables (proxy_sources, proxy_epg_sources, proxy_filters, proxy_mapping_rules) |
| T074 | P | US3 | [X] Write tests for StreamProxyRepository |
| T075 | | US3 | [X] Implement repository/stream_proxy_repo.go |

### Pipeline Core

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T076 | | US3 | [X] Implement pipeline/core/interfaces.go - Stage, State, Result |
| T077 | | US3 | [X] Implement pipeline/core/orchestrator.go |
| T078 | | US3 | [X] Implement pipeline/core/factory.go with stage registration |
| T079 | P | US3 | [X] Write tests for M3U generation stage |
| T080 | | US3 | [X] Implement pkg/m3u/writer.go - streaming M3U writer |
| T081 | | US3 | [X] Implement pipeline/stages/generatem3u/stage.go |
| T082 | P | US3 | [X] Write tests for XMLTV generation stage |
| T083 | | US3 | [X] Implement pipeline/stages/generatexmltv/stage.go |
| T084 | | US3 | [X] Implement pipeline/stages/publish/stage.go - atomic file publish |

### Service Layer

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T085 | P | US3 | [X] Write tests for ProxyService |
| T086 | | US3 | [X] Implement service/proxy_service.go |
| T087 | | US3 | [X] Write integration tests for proxy generation flow |

## Phase 6: User Story 10 - REST API (P1) ✅ COMPLETE

### HTTP Infrastructure

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T090 | | US10 | [X] Implement http/server.go - HTTP server setup with Chi |
| T091 | P | US10 | [X] Implement http/middleware/logging.go |
| T092 | P | US10 | [X] Implement http/middleware/recovery.go |
| T093 | P | US10 | [X] Implement http/middleware/cors.go |
| T094 | P | US10 | [X] Implement http/middleware/request_id.go |
| T095 | | US10 | [X] Implement http/routes.go - route registration |

### Handlers

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T096 | P | US10 | [X] Write tests for source handlers |
| T097 | | US10 | [X] Implement http/handlers/source_handler.go |
| T098 | P | US10 | [X] Write tests for EPG handlers |
| T099 | | US10 | [X] Implement http/handlers/epg_handler.go |
| T100 | P | US10 | [X] Write tests for proxy handlers |
| T101 | | US10 | [X] Implement http/handlers/proxy_handler.go |
| T102 | | US10 | [X] Implement http/handlers/output_handler.go - serve M3U/XMLTV |
| T103 | | US10 | [X] Implement http/handlers/health_handler.go |
| T103a | P | US10 | [X] Write tests for graceful shutdown (NFR-006) |
| T103b | | US10 | [X] Implement graceful shutdown in http/server.go with configurable timeout (default: 30s) |

### CLI Entry Point

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T104 | | US10 | [X] Implement cmd/tvarr/main.go with Cobra |
| T105 | | US10 | [X] Add serve command to start HTTP server |
| T106 | | US10 | [X] Add migrate command for database migrations |
| T107 | | US10 | [X] Write integration tests for API endpoints |

## Phase 7: User Story 4 - Data Mapping Rules (P2) ⏸️ BLOCKED

**Blocked by**: Phase 6.5 (Expression Engine)

### Expression Engine (MOVED TO PHASE 6.5)

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

## Phase 8: User Story 5 - Filtering Rules (P2) ⏸️ BLOCKED

**Blocked by**: Phase 6.5 (Expression Engine)

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T130 | P | US5 | Write tests for Filter model |
| T131 | | US5 | Implement models/filter.go |
| T132 | | US5 | Create migration for filters table |
| T133 | P | US5 | Write tests for FilterRepository |
| T134 | | US5 | Implement repository/filter_repo.go |
| T135 | P | US5 | Write tests for filtering pipeline stage (expression-based) |
| T136 | | US5 | Update pipeline/stages/filtering/ to use expression engine |
| T137 | | US5 | (MOVED) Boolean logic now in Phase 6.5 |
| T138 | P | US5 | Write tests for FilterService |
| T139 | | US5 | Implement service/filter_service.go |
| T140 | | US5 | Implement http/handlers/filter_handler.go |
| T141 | | US5 | Write integration tests for filtering flow |

## Phase 9: User Story 6 - Channel Numbering (P2)

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T150 | P | US6 | Write tests for numbering pipeline stage |
| T151 | | US6 | Update pipeline/stages/numbering/ with conflict detection |
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

---

## Updated Dependencies

```
Phase 1 (Setup) → Phase 2 (Database) → [US1, US2] in parallel
                                      ↓
                                     US3 (needs US1, US2)
                                      ↓
                                     US10 (API)
                                      ↓
                              Phase 6.5 (Expression Engine) ← BLOCKING
                                      ↓
                              Phase 6.6 (SSE Progress) ← Optional but recommended
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
| Phase 1: Setup | 5 | ✅ Complete |
| Phase 2: Database | 8 | ✅ Complete |
| Phase 3: US1 | 13 | ✅ Complete |
| Phase 4: US2 | 13 | ✅ Complete |
| Phase 5: US3 | 13 | ✅ Complete |
| Phase 6: US10 | 8 | ✅ Complete |
| **Phase 6.5: Expression Engine** | **21** | **BLOCKING - Critical path** |
| Phase 6.6: SSE Progress | 8 | Real-time progress streaming |
| Phase 7: US4 | 8 | Reduced (engine in 6.5) |
| Phase 8: US5 | 3 | Reduced (engine in 6.5) |
| Phase 9: US6 | 3 | Straightforward |
| Phase 10: US7 | 8 | File management |
| Phase 11: US9 | 8 | Job scheduling |
| Phase 12: US8 | 21 | FFmpeg integration complex |
| Phase 13: Optional | 13 | Embedding complexity |
| Phase 14: Polish | 8 | Documentation, testing |
| **Total** | **161** | |

## Phase 6.5 Task Groupings (Recommended Order)

### Group A: Foundation (T300-T306, T310-T315)
- Lexer, AST, Parser, Operators
- Field Registry, Aliases, Validation
- **Estimate**: 8 story points

### Group B: Evaluation (T320-T327, T330-T334)
- Condition evaluation, all operators
- Eval context for channels/programs
- **Estimate**: 5 story points

### Group C: Helpers (T340-T346)
- Helper interface and registry
- Time and Logo helpers
- **Estimate**: 3 story points

### Group D: Rule Processing (T350-T364)
- Rule processor with actions
- Data mapping engine
- **Estimate**: 5 story points

### Group E: Pipeline Integration (T370-T395)
- Filter processor update
- Stage updates and integration tests
- **Estimate**: 5 story points (overlaps with Phase 7/8)
