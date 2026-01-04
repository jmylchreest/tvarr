# Feature Specification: Pipeline Logging, Error Feedback, and M3U/XMLTV Generation

**Feature Branch**: `002-pipeline-logging-fixes`
**Created**: 2025-12-02
**Status**: Draft
**Input**: User description: "Improve logging for pipeline runs, ingests, and errors. Provide client feedback for errors using the UI. Align with m3u-proxy reference codebase patterns. Handle artifacts efficiently and ensure cleanup (including failed runs). Ensure database consistency. Enable full pipeline execution generating m3u/xmltv files."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Full Pipeline Execution with M3U/XMLTV Output (Priority: P1)

As an administrator, I want to trigger a proxy generation pipeline that processes channels and EPG data, applies filters and data mappings, and produces valid M3U and XMLTV output files that can be consumed by IPTV clients.

**Why this priority**: This is the core functionality that delivers end-to-end value. Without complete pipeline execution producing consumable output files, the system provides no practical benefit.

**Independent Test**: Can be tested by creating a stream proxy with at least one source, triggering generation, and verifying that M3U and XMLTV files are created in the output directory with valid content.

**Acceptance Scenarios**:

1. **Given** a stream proxy with one or more configured stream sources, **When** the user triggers "Generate" from the UI, **Then** the pipeline executes all stages (load channels, load programs, filter, data mapping, numbering, logo caching, generate M3U, generate XMLTV, publish) and creates valid .m3u and .xml files in the configured output directory.

2. **Given** a stream proxy with no sources configured, **When** the user triggers "Generate", **Then** the system displays a clear error message explaining that at least one stream source is required.

3. **Given** a successful generation, **When** the M3U file is opened in an IPTV player, **Then** all channels are accessible with correct metadata (tvg-id, tvg-name, tvg-logo, group-title, channel number).

---

### User Story 2 - Detailed Pipeline Logging (Priority: P2)

As an administrator, I want comprehensive logging during pipeline execution so I can understand what is happening at each stage, diagnose performance issues, and identify failures without inspecting database state.

**Why this priority**: Logging enables debugging and operational visibility. Without it, users cannot troubleshoot issues or understand system behavior.

**Independent Test**: Can be tested by triggering a pipeline and verifying that logs contain stage start/end, item counts, timing, and any errors with sufficient context.

**Acceptance Scenarios**:

1. **Given** a pipeline execution starts, **When** each stage begins, **Then** logs include: stage name, stage ID, stage sequence number (e.g., "1/10"), input item count where applicable.

2. **Given** a pipeline stage completes, **When** the stage finishes, **Then** logs include: stage name, duration, records processed, artifacts produced, any non-fatal warnings.

3. **Given** a pipeline stage processes items in batches, **When** DEBUG log level is enabled, **Then** logs include batch progress (e.g., "Processing batch 3/10, items 2001-3000").

4. **Given** a pipeline encounters an error, **When** any stage fails, **Then** logs include: error message, stage context, partial state (items processed before failure), and stack trace at ERROR level.

---

### User Story 3 - UI Error Feedback for Pipeline Failures (Priority: P2)

As an administrator, I want to see clear error feedback in the UI when a pipeline fails, so I can understand what went wrong and take corrective action without checking server logs.

**Why this priority**: User-facing error feedback enables self-service troubleshooting and reduces operational friction.

**Independent Test**: Can be tested by triggering a pipeline that fails (e.g., due to network error or invalid configuration) and verifying the UI displays a meaningful error message.

**Acceptance Scenarios**:

1. **Given** a pipeline execution fails at any stage, **When** the failure is detected, **Then** the UI displays an error notification with: the failed stage name, a user-friendly error message, and timestamp.

2. **Given** an ingestion fails during source refresh, **When** the user views the sources list, **Then** the failed source shows an error indicator with the failure reason accessible via tooltip or detail view.

3. **Given** a pipeline completes with warnings (non-fatal errors), **When** the user views the generation result, **Then** a warning indicator is shown with details accessible (e.g., "10 channels skipped due to missing data").

---

### User Story 4 - Artifact Cleanup on Failure and Completion (Priority: P2)

As an administrator, I want the system to properly clean up temporary files and artifacts after pipeline execution (whether successful or failed), so disk space is not consumed by stale data.

**Why this priority**: Resource leaks degrade system reliability over time. Proper cleanup ensures stable operation.

**Independent Test**: Can be tested by triggering pipelines (both successful and failed), then verifying that temporary directories are removed and only final output files remain.

**Acceptance Scenarios**:

1. **Given** a pipeline completes successfully, **When** the final output is published, **Then** all temporary files (intermediate stage outputs in temp directories) are removed.

2. **Given** a pipeline fails at any stage, **When** the orchestrator handles the error, **Then** all temporary files created during that run are removed.

3. **Given** the application crashes during pipeline execution, **When** the application restarts, **Then** orphaned temp directories from previous runs are cleaned up during startup.

4. **Given** multiple proxies are being generated, **When** each proxy's pipeline runs, **Then** each uses an isolated temp directory that does not interfere with others.

---

### User Story 5 - Real-time Progress Updates in UI (Priority: P3)

As an administrator, I want to see real-time progress updates in the UI during pipeline execution, so I can monitor long-running operations without uncertainty.

**Why this priority**: Progress feedback improves user experience but is not essential for core functionality.

**Independent Test**: Can be tested by triggering a pipeline and observing that the UI shows progress percentages and stage names updating in real-time.

**Acceptance Scenarios**:

1. **Given** a pipeline is executing, **When** stages progress, **Then** the UI shows: current stage name, overall percentage, items processed / total (where known).

2. **Given** an ingestion is downloading an M3U source, **When** download progresses, **Then** the UI shows download percentage and current status.

3. **Given** multiple operations run concurrently, **When** viewing the dashboard, **Then** each active operation shows its own progress indicator.

---

### User Story 6 - Database Consistency During Pipeline Operations (Priority: P3)

As an administrator, I want pipeline operations to maintain database consistency, so partial failures do not leave the system in an inconsistent state.

**Why this priority**: Data integrity is foundational for system reliability.

**Independent Test**: Can be tested by simulating failures during ingestion or generation and verifying that the database reflects either the complete old state or the complete new state, not a mix.

**Acceptance Scenarios**:

1. **Given** a channel ingestion fails midway, **When** the failure is handled, **Then** no partial channel data from the failed run exists in the database (atomic rollback).

2. **Given** a pipeline generation fails, **When** the proxy status is checked, **Then** it shows "failed" with error message, and previous generation timestamps/counts remain unchanged.

3. **Given** concurrent operations on different resources, **When** both complete, **Then** no cross-contamination occurs (each resource has correct associated data).

---

### Edge Cases

- What happens when output directory does not exist? System should create it or fail with clear error.
- How does system handle very large sources (100k+ channels)? Memory-bounded processing with streaming.
- What happens when logo caching service is unavailable? Pipeline continues with warning, channels use original logo URLs.
- How does system handle concurrent generation requests for the same proxy? Second request rejected with "already running" error.
- What happens when disk space is exhausted during M3U generation? Stage fails with clear error, cleanup attempted.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Pipeline MUST execute all registered stages in order: ingestion guard, load channels, load programs, filtering, data mapping, numbering, logo caching (optional), generate M3U, generate XMLTV, publish.
- **FR-002**: Pipeline MUST create valid M3U files with `#EXTM3U` header and `#EXTINF` entries containing tvg-id, tvg-name, tvg-logo, tvg-chno, and group-title attributes.
- **FR-003**: Pipeline MUST create valid XMLTV files with proper XML structure including `<tv>`, `<channel>`, and `<programme>` elements.
- **FR-004**: Pipeline MUST log stage start/completion with timing at INFO level.
- **FR-005**: Pipeline MUST log errors with full context (stage, item being processed, error details) at ERROR level.
- **FR-006**: Pipeline MUST clean up temporary directories on both success and failure.
- **FR-007**: Pipeline MUST report progress via the progress service for SSE broadcast to UI.
- **FR-008**: UI MUST display error notifications when pipeline operations fail.
- **FR-009**: UI MUST show error indicators on resources (sources, proxies) that have failed operations.
- **FR-010**: Ingestion MUST use atomic transactions (delete + insert) to prevent partial data states.
- **FR-011**: System MUST prevent duplicate concurrent pipeline executions for the same proxy.
- **FR-012**: System MUST clean up orphaned temp directories on application startup.
- **FR-013**: Pipeline MUST support graceful cancellation via context cancellation.
- **FR-014**: DEBUG level MUST include batch progress and detailed stage information.
- **FR-015**: Pipeline stage errors MUST be surfaced to the UI via progress events with state "error".

### Key Entities

- **Pipeline Artifact**: Represents stage output (M3U content, XMLTV content, channel list). Contains: type, processing stage, source stage ID, file path (optional), record count, file size.
- **Stage Result**: Outcome of a pipeline stage execution. Contains: records processed, artifacts produced, duration, message, errors.
- **Progress Event**: Real-time status update for UI. Contains: operation type, owner ID, state, progress percentage, current stage, stages array, error message.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Pipeline execution produces valid M3U files that load successfully in VLC/IPTV players.
- **SC-002**: Pipeline execution produces valid XMLTV files that parse successfully with standard EPG tools.
- **SC-003**: All pipeline executions leave zero orphaned temporary files after 24 hours of operation.
- **SC-004**: Error messages in UI contain actionable information (what failed, suggested resolution) for all categorized failure types: permission errors, network errors, validation errors, and resource not found errors.
- **SC-005**: Pipeline progress updates appear in UI within 500ms of stage transitions.
- **SC-006**: Atomic ingestion ensures zero partial channel datasets exist in database after any ingestion failure.
- **SC-007**: Log output for a typical pipeline run includes stage timing, record counts, and any warnings without requiring DEBUG level.
