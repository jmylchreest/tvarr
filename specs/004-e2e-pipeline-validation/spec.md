# Feature Specification: End-to-End Pipeline Validation

**Feature Branch**: `004-e2e-pipeline-validation`
**Created**: 2025-12-03
**Status**: Draft
**Input**: User description: "I want to test and fix any issues in the full end-to-end of the pipeline, that includes ingesting channel and xmltv content, processing it through every stage, and ultimately generating and serving a working m3u and xmltv"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Complete Stream Source Ingestion (Priority: P1)

As an administrator, I want to configure a stream source (M3U or Xtream) and successfully ingest all channel data so that I have channels available for processing and proxy generation.

**Why this priority**: Without successful ingestion, no downstream pipeline stages can operate. This is the foundational data entry point for the entire system.

**Independent Test**: Can be fully tested by adding a stream source, triggering ingestion, and verifying channels appear in the channel list with correct metadata (name, logo URL, group, etc.).

**Acceptance Scenarios**:

1. **Given** a configured M3U stream source, **When** ingestion is triggered, **Then** all channels from the M3U are imported with their names, logos, groups, and stream URLs preserved
2. **Given** a configured Xtream stream source, **When** ingestion is triggered, **Then** all channels are imported with their Xtream-specific metadata (stream ID, category) preserved
3. **Given** an ingestion in progress, **When** monitoring the operation, **Then** progress updates show the current stage and percentage completion
4. **Given** a completed ingestion, **When** viewing the channel list, **Then** all imported channels are visible with accurate metadata

---

### User Story 2 - Complete EPG Source Ingestion (Priority: P1)

As an administrator, I want to configure an EPG source (XMLTV) and successfully ingest all program guide data so that my generated XMLTV output contains accurate program schedules.

**Why this priority**: EPG data is essential for a usable TV guide. Without it, the generated XMLTV is empty or incomplete.

**Independent Test**: Can be fully tested by adding an EPG source, triggering ingestion, and verifying programs appear with correct channel mappings and schedule data.

**Acceptance Scenarios**:

1. **Given** a configured XMLTV EPG source, **When** ingestion is triggered, **Then** all programs are imported with their titles, descriptions, start times, and end times
2. **Given** an EPG source with timezone information, **When** ingestion completes, **Then** program times are correctly adjusted to the system timezone
3. **Given** multiple EPG sources, **When** both are ingested, **Then** programs from all sources are available for matching to channels

---

### User Story 3 - Stream Proxy Configuration and Generation (Priority: P1)

As an administrator, I want to create a stream proxy configuration that filters and transforms my channels so that I can generate customized M3U and XMLTV outputs for different clients.

**Why this priority**: The stream proxy is the primary output mechanism. Users need to generate working playlists and guides for their media players.

**Independent Test**: Can be fully tested by creating a stream proxy, configuring channel filters/mappings, generating output, and verifying the M3U and XMLTV files are valid and contain expected content.

**Acceptance Scenarios**:

1. **Given** a stream proxy with no filters, **When** output is generated, **Then** all ingested channels appear in the M3U output
2. **Given** a stream proxy with channel filters, **When** output is generated, **Then** only matching channels appear in the M3U output
3. **Given** a stream proxy with data mapping rules, **When** output is generated, **Then** channel names and groups are transformed according to the rules
4. **Given** a stream proxy with EPG enabled, **When** output is generated, **Then** the XMLTV contains program data for all included channels

---

### User Story 4 - Serving Generated M3U Output (Priority: P2)

As an end user, I want to access the generated M3U playlist via a URL so that I can load it into my media player or IPTV application.

**Why this priority**: The M3U output is the primary deliverable - without accessible output, the system provides no value to end users.

**Independent Test**: Can be fully tested by requesting the M3U URL and verifying the response is a valid M3U file playable in a media player.

**Acceptance Scenarios**:

1. **Given** a generated stream proxy M3U, **When** the M3U URL is requested, **Then** a valid M3U playlist is returned with correct headers
2. **Given** an M3U with stream URLs, **When** a media player opens the playlist, **Then** streams can be selected and played
3. **Given** a stream proxy with logo caching enabled, **When** the M3U is served, **Then** logo URLs point to cached versions

---

### User Story 5 - Serving Generated XMLTV Output (Priority: P2)

As an end user, I want to access the generated XMLTV guide via a URL so that I can load it into my media player or EPG application.

**Why this priority**: The XMLTV output provides the program guide experience - essential for a complete TV viewing experience.

**Independent Test**: Can be fully tested by requesting the XMLTV URL and verifying the response is a valid XMLTV file parseable by EPG applications.

**Acceptance Scenarios**:

1. **Given** a generated stream proxy XMLTV, **When** the XMLTV URL is requested, **Then** a valid XMLTV file is returned with correct XML structure
2. **Given** an XMLTV with program data, **When** an EPG application loads it, **Then** program schedules display correctly for each channel
3. **Given** channels with matched EPG data, **When** the XMLTV is served, **Then** each channel has corresponding program entries

---

### User Story 6 - Pipeline Stage Progression (Priority: P3)

As an administrator, I want to see the pipeline progress through all stages during proxy generation so that I can monitor and troubleshoot the process.

**Why this priority**: Visibility into pipeline stages helps diagnose issues and provides confidence that processing is working correctly.

**Independent Test**: Can be fully tested by triggering proxy generation and observing progress events for each pipeline stage.

**Acceptance Scenarios**:

1. **Given** a proxy generation in progress, **When** monitoring via SSE, **Then** progress events show each stage (load programs, filtering, data mapping, numbering, logo caching, generate M3U, generate XMLTV, publish)
2. **Given** a stage encounters an error, **When** the error occurs, **Then** the error is reported with a descriptive message
3. **Given** a proxy generation completes, **When** the final event is received, **Then** it indicates successful completion with 100% progress

---

### Edge Cases

- What happens when the stream source URL is unreachable during ingestion?
- How does the system handle malformed M3U or XMLTV content?
- What happens when EPG channel IDs don't match any ingested channels?
- How does the system handle very large sources (10,000+ channels)?
- What happens when logo URLs return 404 errors during caching?
- How does the system behave when disk space is low during output generation?
- What happens if two ingestions are triggered simultaneously for the same source?

### Known Issues (Resolved)

- **BUG-001**: Stream Proxy creation fails with 422 validation error - **FIXED**
  - **Symptom**: Creating a proxy with stream or EPG sources returns "expected string" validation errors
  - **Root Cause**: Frontend/backend schema mismatch for `source_ids` and `epg_source_ids` fields
    - Frontend sends: `source_ids: [{source_id: "...", priority_order: 1}]` (array of objects)
    - Backend expects: `source_ids: ["..."]` (array of ULID strings)
  - **Resolution**: Fixed `frontend/src/lib/api-client.ts` to correctly extract `source_id` and `epg_source_id` from the form data objects

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST ingest M3U sources and store all channel metadata (name, logo, group, stream URL)
- **FR-002**: System MUST ingest Xtream sources and store Xtream-specific metadata (stream ID, category)
- **FR-003**: System MUST ingest XMLTV sources and store program data (title, description, start, end, channel reference)
- **FR-004**: System MUST support timezone handling for EPG data
- **FR-005**: System MUST allow creation of stream proxy configurations with filtering rules
- **FR-006**: System MUST allow data mapping rules for transforming channel names and groups
- **FR-007**: System MUST generate valid M3U playlists from stream proxy configurations
- **FR-008**: System MUST generate valid XMLTV files from stream proxy configurations
- **FR-009**: System MUST serve M3U output via HTTP endpoint
- **FR-010**: System MUST serve XMLTV output via HTTP endpoint
- **FR-011**: System MUST provide progress reporting for all long-running operations
- **FR-012**: System MUST execute all pipeline stages in correct order (load programs, filtering, data mapping, numbering, logo caching, generate M3U, generate XMLTV, publish)
- **FR-013**: System MUST match EPG programs to channels during proxy generation
- **FR-014**: System MUST cache channel logos when configured
- **FR-015**: System MUST report errors with descriptive messages when pipeline stages fail

### Key Entities

- **Stream Source**: External source of channel data (M3U or Xtream provider)
- **EPG Source**: External source of program guide data (XMLTV provider)
- **Channel**: Individual TV channel with metadata and stream URL
- **Program**: Individual program entry with schedule and description
- **Stream Proxy**: Configuration for generating filtered/transformed M3U and XMLTV outputs
- **Filter Rule**: Criteria for including/excluding channels in proxy output
- **Data Mapping Rule**: Transformation rule for channel metadata

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of ingested M3U channels are retrievable via the channel list with complete metadata
- **SC-002**: 100% of ingested XMLTV programs are stored with correct timestamps and channel references
- **SC-003**: Generated M3U playlists pass validation and are playable in standard media players
- **SC-004**: Generated XMLTV files pass XML validation and are parseable by standard EPG applications
- **SC-005**: All pipeline stages execute and report progress for every proxy generation operation
- **SC-006**: M3U and XMLTV endpoints respond within 5 seconds for outputs with up to 1,000 channels
- **SC-007**: Error conditions are reported with actionable error messages rather than generic failures
- **SC-008**: End-to-end flow from source configuration to playable output completes successfully without manual intervention

## Test Data Sources

**E2E Testing URLs**: The following public, free sources are used for E2E validation testing. These sources provide compatible M3U and EPG data with matching channel IDs.

| Type | Name | URL | Notes |
|------|------|-----|-------|
| M3U Stream | m3upt.com IPTV | `https://m3upt.com/iptv` | Free public M3U with European channels |
| EPG Source | m3upt.com EPG | `https://m3upt.com/epg` | XMLTV EPG data matching the IPTV source |

**Why these sources**:
- Free and publicly accessible (no authentication required)
- Compatible - EPG channel IDs match M3U channel IDs
- Reasonably sized for testing (not too large, not too small)
- Regularly maintained and available

**Alternative Sources** (used in initial testing):
- iptv-org US M3U: `https://iptv-org.github.io/iptv/countries/us.m3u` (1,480 channels, no matching EPG)

## Assumptions

- Stream sources provide valid M3U or Xtream API responses
- EPG sources provide valid XMLTV format
- Network connectivity is available for source fetching and logo caching
- Sufficient disk space is available for logo cache and generated outputs
- A single administrator is testing the system (not concurrent multi-user scenarios)
