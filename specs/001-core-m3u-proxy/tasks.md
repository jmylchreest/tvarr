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
| **Phase 6.5: Expression Engine** | ✅ Complete | All tasks complete |
| Phase 6.6: SSE Progress Streaming | ✅ Complete | Real-time progress updates via SSE |
| Phase 7: US4 Data Mapping | ✅ Superseded | Merged into Phase 6.5 |
| Phase 8: US5 Filtering | ✅ Superseded | Merged into Phase 6.5 |
| Phase 9: Channel Numbering | ✅ Complete | T150-T153, configurable modes |
| Phase 10: Logo Caching | ✅ Complete | File-based caching with metadata sidecar |
| **Phase 10.5: Parity Fixes** | ✅ Complete | Manual source, ingestion guard, atomic publish, auto-EPG, shorthand |
| **Phase 11: Scheduled Jobs** | ✅ Complete | Job model, scheduler, runner, handlers (T180-T193) |
| Phase 12-14 | ⏸️ Pending | |

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
| Conflict Resolution | Numbering conflicts | ✅ Complete | detect/resolve in preserve mode |
| Filter Repository | CRUD operations | ✅ Complete | repository/filter_repo.go |
| DataMappingRule Repository | CRUD operations | ✅ Complete | repository/data_mapping_rule_repo.go |
| Filter API Handlers | REST CRUD endpoints | ✅ Complete | handlers/filter.go |
| DataMappingRule API Handlers | REST CRUD endpoints | ✅ Complete | handlers/data_mapping_rule.go |
| Pipeline Database Integration | Load rules from DB | ✅ Complete | orchestrator loads from DB |

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
| T345 | | Expr | [X] Implement expression/helpers/logo_helper.go - @logo:ULID resolution |
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

### Integration & Testing ✅ COMPLETE (Core)

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T390 | P | Expr | [X] Write integration tests for expression parsing |
| T391 | P | Expr | [X] Write integration tests for rule application |
| T392 | P | Expr | [X] Write integration tests for helper resolution |

> **Deferred to Phase 14 (Polish)**: T393-T395 (fuzzing, benchmarks, e2e) are non-blocking quality improvements.

---

## Phase 6.6: SSE Progress Streaming ✅ COMPLETE

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
| T410 | P | SSE | [x] Write tests for UniversalProgress types and states |
| T411 | | SSE | [x] Implement service/progress_service.go - ProgressService struct |
| T412 | | SSE | [x] Implement UniversalState enum (Idle, Preparing, Connecting, Downloading, Processing, Saving, Cleanup, Completed, Error, Cancelled) |
| T413 | | SSE | [x] Implement OperationType enum (StreamIngestion, EpgIngestion, ProxyRegeneration, Pipeline, DataMapping, LogoCaching, Filtering, Maintenance, Database) |
| T414 | | SSE | [x] Implement UniversalProgress struct with stages, timestamps, metadata |
| T415 | | SSE | [x] Implement ProgressManager for staged operations with weighted progress calculation |
| T416 | | SSE | [x] Implement StageUpdater for individual stage progress reporting |
| T417 | | SSE | [x] Implement broadcast channel mechanism for SSE subscribers |
| T418 | | SSE | [x] Implement operation blocking (prevent duplicate operations per owner) |
| T419 | | SSE | [x] Implement automatic cleanup of completed/stale operations |

### SSE HTTP Endpoint

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T420 | P | SSE | [x] Write tests for SSE progress handler |
| T421 | | SSE | [x] Implement http/handlers/progress_handler.go - SSE endpoint |
| T422 | | SSE | [x] Implement GET /api/v1/progress/events SSE stream |
| T423 | | SSE | [x] Implement query filters (operation_type, owner_id, resource_id, state, active_only) |
| T424 | | SSE | [x] Implement SSE keepalive heartbeat (30s interval) |
| T425 | | SSE | [x] Implement GET /api/v1/progress/operations - list current operations |
| T426 | | SSE | [x] Register progress routes in serve.go |

### Pipeline Integration

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T430 | P | SSE | [x] Write tests for pipeline progress integration |
| T431 | | SSE | [x] Update pipeline/core/orchestrator.go to use ProgressService |
| T432 | | SSE | [x] Register pipeline stages with ProgressManager |
| T433 | | SSE | [x] Bridge existing ProgressReporter to new ProgressService |
| T434 | | SSE | [x] Implement stage-level progress updates during pipeline execution |

### Ingestion Integration

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T440 | P | SSE | [x] Write tests for ingestion progress integration |
| T441 | | SSE | [x] Update service/source_service.go to use ProgressService |
| T442 | | SSE | [x] Update service/epg_service.go to use ProgressService |
| T443 | | SSE | [x] Implement progress stages for ingestion (Connecting, Downloading, Processing, Saving) |
| T444 | | SSE | [x] Track item counts (channels/programs processed vs total) |

### Service Integration

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T450 | P | SSE | [x] Write tests for proxy regeneration progress |
| T451 | | SSE | [x] Update service/proxy_service.go to use ProgressService |
| T452 | | SSE | [x] Implement progress tracking for proxy regeneration |
| T453 | | SSE | [x] Add ProgressService to application dependencies in serve.go |

### Testing

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T460 | P | SSE | [x] Write integration tests for SSE endpoint |
| T461 | | SSE | [x] Write tests for concurrent operation blocking |
| T462 | | SSE | [x] Write tests for progress calculation with multiple stages |
| T463 | | SSE | [x] Write tests for cleanup of stale operations |

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

## Phase 7: User Story 4 - Data Mapping Rules (P2) ✅ SUPERSEDED

**Status**: Superseded by Phase 6.5 (Expression Engine)

> **Note**: All tasks in this phase have been completed as part of Phase 6.5, which consolidated
> the expression engine, data mapping, and filtering implementations. See Phase 6.5 tasks:
> - T360-T364: Data Mapping Engine
> - T401-T402, T404: DataMappingRule model and migration
> - T380, T382: DataMappingStage pipeline implementation
> - Handlers in http/handlers/data_mapping_rule.go
> - Repository in repository/data_mapping_rule_repo.go

### Models & Pipeline (SUPERSEDED - see Phase 6.5)

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T116 | P | US4 | ~~Write tests for DataMappingRule model~~ → T360 |
| T117 | | US4 | ~~Implement models/data_mapping_rule.go~~ → T402 |
| T118 | | US4 | ~~Create migration for data_mapping_rules table~~ → T404 |
| T119 | P | US4 | ~~Write tests for DataMappingRuleRepository~~ → implemented |
| T120 | | US4 | ~~Implement repository/data_mapping_rule_repo.go~~ → implemented |
| T121 | P | US4 | ~~Write tests for data mapping pipeline stage~~ → T360 |
| T122 | | US4 | ~~Implement pipeline/stages/data_mapping.go~~ → T380 |

### Service & API (SUPERSEDED - see Phase 6.5)

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T123 | P | US4 | ~~Write tests for MappingService~~ → handlers have tests |
| T124 | | US4 | ~~Implement service/mapping_service.go~~ → handled via repository |
| T125 | | US4 | ~~Implement http/handlers/mapping_handler.go~~ → data_mapping_rule.go |
| T126 | | US4 | ~~Write integration tests for mapping flow~~ → T391 |

## Phase 8: User Story 5 - Filtering Rules (P2) ✅ SUPERSEDED

**Status**: Superseded by Phase 6.5 (Expression Engine)

> **Note**: All tasks in this phase have been completed as part of Phase 6.5, which consolidated
> the expression engine, data mapping, and filtering implementations. See Phase 6.5 tasks:
> - T370-T374: Filter Processor with expression-based filtering
> - T401, T403: Filter model and migration
> - T381, T374: FilteringStage pipeline updates
> - Handlers in http/handlers/filter.go
> - Repository in repository/filter_repo.go

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T130 | P | US5 | ~~Write tests for Filter model~~ → T370 |
| T131 | | US5 | ~~Implement models/filter.go~~ → T401 |
| T132 | | US5 | ~~Create migration for filters table~~ → T403 |
| T133 | P | US5 | ~~Write tests for FilterRepository~~ → implemented |
| T134 | | US5 | ~~Implement repository/filter_repo.go~~ → implemented |
| T135 | P | US5 | ~~Write tests for filtering pipeline stage~~ → T370 |
| T136 | | US5 | ~~Update pipeline/stages/filtering/~~ → T374, T381 |
| T137 | | US5 | ~~Boolean logic~~ → T326-T327 |
| T138 | P | US5 | ~~Write tests for FilterService~~ → handlers have tests |
| T139 | | US5 | ~~Implement service/filter_service.go~~ → handled via repository |
| T140 | | US5 | ~~Implement http/handlers/filter_handler.go~~ → filter.go |
| T141 | | US5 | ~~Write integration tests for filtering flow~~ → T390-T392 |

## Phase 9: User Story 6 - Channel Numbering (P2) ✅ COMPLETE

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T150 | P | US6 | [X] Write tests for numbering pipeline stage |
| T151 | | US6 | [X] Update pipeline/stages/numbering/ with conflict detection |
| T152 | | US6 | [X] Implement numbering configuration (base number, group-based) |
| T153 | | US6 | [X] Write integration tests for numbering |

**Implementation Notes:**
- NumberingMode enum added to StreamProxy model (sequential, preserve, group)
- GroupNumberingSize field added for configurable group ranges
- Conflict resolution tracks original vs assigned numbers
- Proxy configuration overrides stage defaults at runtime

## Phase 10: User Story 7 - Logo Caching (P2) ✅ COMPLETE

**Implementation Note**: Per constitution principle (Key Systems #3), logo caching uses file-based storage with JSON metadata sidecars instead of database tables. This eliminates database overhead for cached assets while maintaining queryable metadata via in-memory indexing.

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T160 | P | US7 | ~~Write tests for LogoAsset model~~ → N/A (file-based) |
| T161 | | US7 | ~~Implement models/logo_asset.go~~ → N/A (file-based) |
| T162 | | US7 | ~~Create migration for logo_assets table~~ → N/A (file-based) |
| T163 | P | US7 | ~~Write tests for LogoAssetRepository~~ → N/A (file-based) |
| T164 | | US7 | ~~Implement repository/logo_asset_repo.go~~ → N/A (file-based) |
| T165 | P | US7 | [X] Write tests for logo cache storage |
| T166 | | US7 | [X] Implement storage/logo_cache.go |
| T166a | | US7 | [X] Implement storage/logo_metadata.go - JSON sidecar metadata |
| T167 | P | US7 | [X] Write tests for logo caching pipeline stage |
| T168 | | US7 | [X] Implement pipeline/stages/logocaching/stage.go |
| T169 | | US7 | [X] Implement logo cleanup in LogoService.Prune() |
| T170 | P | US7 | [X] Write tests for LogoService |
| T171 | | US7 | [X] Implement service/logo_service.go |
| T171a | | US7 | [X] Implement service/logo_indexer.go - in-memory hash-based index |
| T172 | | US7 | [X] Write integration tests for logo caching |

## Phase 10.5: Parity Fixes (P0 - BLOCKING)

**Priority**: P0 - Must complete before Phase 11
**Rationale**: Gap analysis between tvarr and m3u-proxy revealed critical missing features that affect data consistency and user experience. See `pre-phase11-status.md` for full analysis.

### Manual Source Type

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T470 | P | Parity | [ ] Write tests for Manual source type |
| T471 | | Parity | [ ] Add `SourceTypeManual` to SourceType enum in models/stream_source.go |
| T472 | | Parity | [ ] Update StreamSource.URL to be optional (for Manual sources) |
| T473 | | Parity | [ ] Implement ingestor/manual_handler.go - materialize ManualStreamChannels to Channels |
| T474 | | Parity | [ ] Register manual handler in ingestor/factory.go |
| T475 | | Parity | [ ] Write integration tests for manual source ingestion |

### Ingestion Guard Pipeline Stage

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T476 | P | Parity | [ ] Write tests for ingestion guard stage |
| T477 | | Parity | [ ] Implement pipeline/stages/ingestionguard/stage.go |
| T478 | | Parity | [ ] Add configurable polling interval and max attempts |
| T479 | | Parity | [ ] Register ingestion guard as first stage in factory |
| T480 | | Parity | [ ] Add feature flag to enable/disable ingestion guard |

### Atomic File Publishing

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T481 | P | Parity | [ ] Write tests for atomic publish |
| T482 | | Parity | [ ] Update pipeline/stages/publish/stage.go to use os.Rename() |
| T483 | | Parity | [ ] Add cross-filesystem fallback (copy-then-rename) |
| T484 | | Parity | [ ] Update storage/sandbox.go with atomic publish helper |

### Auto-EPG Linking for Xtream Sources

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T485 | P | Parity | [ ] Write tests for auto-EPG linking |
| T486 | | Parity | [ ] Add check_epg_availability() to source_service.go |
| T487 | | Parity | [ ] Update CreateStreamSource to auto-create EPG for Xtream type |
| T488 | | Parity | [ ] Add URL-based source linking for Xtream sources |
| T489 | | Parity | [ ] Write integration tests for auto-EPG creation |

### Expression Engine Action Shorthand

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T490 | P | Parity | [ ] Write tests for action shorthand syntax |
| T491 | | Parity | [ ] Update lexer.go to recognize ?=, +=, -= operators |
| T492 | | Parity | [ ] Update parser.go to handle implicit SET (field = value without keyword) |
| T493 | | Parity | [ ] Update operators.go to map shorthand to ActionOperator |
| T494 | | Parity | [ ] Ensure backward compatibility with keyword syntax |
| T495 | | Parity | [ ] Write integration tests for shorthand expressions |

---

## Phase 11: User Story 9 - Scheduled Jobs (P2) ✅ COMPLETE

| ID | P | Story | Task Description |
|----|---|-------|------------------|
| T180 | P | US9 | [X] Write tests for Job model |
| T181 | | US9 | [X] Implement models/job.go |
| T182 | | US9 | [X] Create migration for jobs table |
| T183 | P | US9 | [X] Write tests for JobRepository |
| T184 | | US9 | [X] Implement repository/job_repo.go |
| T185 | P | US9 | [X] Write tests for scheduler |
| T186 | | US9 | [X] Implement scheduler/scheduler.go - cron-based scheduler |
| T187 | | US9 | [X] Implement scheduler/queue.go - job queue (merged into runner) |
| T188 | | US9 | [X] Implement scheduler/executor.go - job execution |
| T189 | | US9 | [X] Implement scheduler/runner.go - queue runner |
| T190 | P | US9 | [X] Write tests for JobService |
| T191 | | US9 | [X] Implement service/job_service.go |
| T192 | | US9 | [X] Implement http/handlers/job_handler.go |
| T193 | | US9 | [X] Write integration tests for scheduling |

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
| T241 | P | Polish | Implement observability/tracing.go - OpenTelemetry setup with request tracing |
| T242 | P | Polish | Implement observability/metrics.go - Prometheus metrics with /metrics endpoint (NFR-005) |
| T242a | | Polish | Write tests for health check and metrics endpoints |
| T242b | | Polish | Implement configurable rate limiting middleware (NFR-007, disabled by default) |
| T243 | | Polish | Write performance tests for 100k channel ingestion |
| T244 | | Polish | Write performance tests for 1M EPG program ingestion |
| T244a | | Polish | Write stability/soak test for 24/7 operation (SC-006) - long-running scheduled jobs |
| T244b | | Polish | Write integration tests for PostgreSQL backend (FR-081) |
| T244c | | Polish | Write integration tests for MySQL/MariaDB backend (FR-082) |
| T245 | | Polish | Create Dockerfile |
| T246 | | Polish | Create docker-compose.yml with database options |
| T247 | | Polish | Write quickstart guide |
| T248 | | Polish | Final code review and cleanup |
| T249 | | Polish | Write fuzzing tests for expression parser (deferred from T393) |
| T250 | | Polish | Write benchmark tests for expression evaluation (deferred from T394) |
| T251 | | Polish | End-to-end pipeline test with data mapping + filtering (deferred from T395) |
| T252 | P | Polish | Configure CI code coverage gate (minimum 80% for core packages, SC-009) |
| T253 | | Polish | Implement API authentication configuration scaffold (FR-075, disabled by default) |

---

## Backlog: Clarification Needed

**Status**: These tasks were identified in the m3u-proxy comparison but require clarification before implementation. See `pre-phase11-status.md` for context.

### API Endpoints

| ID | P | Story | Task Description | Questions |
|----|---|-------|------------------|-----------|
| T500 | | Backlog | Add channel browsing endpoint GET /api/v1/channels | Filtering/pagination scheme? |
| T501 | | Backlog | Add EPG program browsing endpoint GET /api/v1/epg/programs | Query parameters? |
| T502 | | Backlog | Add channel probe endpoint GET /api/v1/channels/{id}/probe | Wait for Phase 12 relay? |
| T503 | | Backlog | Add Kubernetes /ready probe endpoint | What conditions determine readiness? |
| T504 | | Backlog | Add Kubernetes /live probe endpoint | What conditions determine liveness? |
| T505 | | Backlog | Add circuit breaker management endpoints | Wait for Phase 12 relay? |
| T506 | | Backlog | Add manual channel M3U export endpoint | Depends on T470-T475 |
| T507 | | Backlog | Add manual channel M3U import endpoint | Merge or replace on import? |

### Infrastructure

| ID | P | Story | Task Description | Questions |
|----|---|-------|------------------|-----------|
| T510 | | Backlog | Add dedicated cleanup pipeline stage | Is defer-based cleanup sufficient? |
| T511 | | Backlog | Add runtime settings API | What settings are runtime-configurable? |
| T512 | | Backlog | Add feature flags API | Persist to DB or config-only? |
| T513 | | Backlog | Add log streaming SSE endpoint | Needed with structured logging? |
| T514 | | Backlog | Add custom metrics endpoints (beyond Prometheus) | What dashboard metrics needed? |

### Expression Engine Enhancements

| ID | P | Story | Task Description | Questions |
|----|---|-------|------------------|-----------|
| T520 | | Backlog | Add per-condition case sensitivity modifier | Worth the complexity? |
| T521 | | Backlog | Add conditional action groups syntax | What use cases require this? |

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
