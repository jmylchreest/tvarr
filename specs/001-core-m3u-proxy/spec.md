# Feature Specification: Core M3U Proxy Service

**Feature Branch**: `001-core-m3u-proxy`  
**Created**: 2025-11-29  
**Status**: Draft  
**Input**: Rewrite of Rust m3u-proxy application in Go with improved modularity and memory efficiency

## Overview

Tvarr is a high-performance IPTV/streaming management service that:
- Aggregates multiple stream sources (M3U playlists, Xtream Codes APIs, manual streams)
- Enriches streams with EPG (Electronic Program Guide) data from XMLTV and Xtream sources
- Transforms metadata through sophisticated data mapping and filtering rules
- Generates custom proxy M3U playlists and XMLTV EPG feeds with applied filters
- Relays streams through FFmpeg for transcoding/format conversion
- Caches logos and probed codec information for performance
- Manages scheduled ingestion, proxy regeneration, and background maintenance tasks

## User Scenarios & Testing

### User Story 1 - Stream Source Management (Priority: P1)

As an IPTV administrator, I want to add and manage multiple stream sources (M3U URLs, Xtream Codes servers) so that I can aggregate channels from various providers into a single system.

**Why this priority**: Without stream sources, no other functionality is possible. This is the foundation of the entire system.

**Independent Test**: Can add an M3U URL, trigger ingestion, and see channels populated in the database. System works standalone with just this feature.

**Acceptance Scenarios**:

1. **Given** no sources exist, **When** I add an M3U URL source with name and URL, **Then** the source is saved and marked as pending ingestion
2. **Given** an M3U source exists, **When** ingestion runs, **Then** all channels from the M3U are parsed and stored with metadata (tvg_id, tvg_name, tvg_logo, group_title, stream_url)
3. **Given** an Xtream Codes source exists with credentials, **When** ingestion runs, **Then** channels are fetched via the Xtream API and stored
4. **Given** a source with 50,000+ channels, **When** ingestion runs, **Then** memory usage remains bounded (streaming/batched processing)
5. **Given** a compressed M3U file (gzip), **When** ingestion runs, **Then** it is automatically decompressed and parsed

---

### User Story 2 - EPG Source Management (Priority: P1)

As an IPTV administrator, I want to add EPG sources (XMLTV URLs, Xtream EPG) so that program guide data is available for my channels.

**Why this priority**: EPG data is essential for a usable IPTV experience. Runs parallel to stream sources.

**Independent Test**: Can add an XMLTV URL, trigger ingestion, and see programs stored. Works alongside US1.

**Acceptance Scenarios**:

1. **Given** no EPG sources exist, **When** I add an XMLTV URL source, **Then** the source is saved and marked as pending ingestion
2. **Given** an XMLTV source exists, **When** ingestion runs, **Then** all programs are parsed with start/end times, titles, descriptions, categories
3. **Given** an Xtream EPG source exists, **When** ingestion runs, **Then** EPG data is fetched via Xtream API
4. **Given** an EPG source with 1M+ programs, **When** ingestion runs, **Then** memory usage remains bounded (batch processing with configurable batch size)

---

### User Story 3 - Proxy Configuration (Priority: P1)

As an IPTV administrator, I want to create proxy configurations that combine sources, apply filters, and generate output playlists so that I can serve customized channel lists to clients.

**Why this priority**: This is the core value proposition - aggregating and filtering content.

**Independent Test**: Can create a proxy, assign sources, and generate an M3U output file.

**Acceptance Scenarios**:

1. **Given** stream sources exist, **When** I create a proxy with selected sources, **Then** the proxy configuration is saved
2. **Given** a proxy exists with sources, **When** generation runs, **Then** an M3U playlist is created with all channels from selected sources
3. **Given** a proxy exists with EPG sources, **When** generation runs, **Then** an XMLTV file is created with program data
4. **Given** a proxy is generated, **When** I access /proxy/{id}.m3u8, **Then** the M3U playlist is served
5. **Given** a proxy is generated, **When** I access /proxy/{id}.xmltv, **Then** the XMLTV file is served

---

### User Story 4 - Data Mapping Rules (Priority: P2)

As an IPTV administrator, I want to create transformation rules that modify channel and EPG metadata so that I can normalize and enhance data from various sources.

**Why this priority**: Improves data quality but system works without it.

**Independent Test**: Can create a mapping rule and see it applied during proxy generation.

**Acceptance Scenarios**:

1. **Given** I create a rule `channel_name contains "HD" SET group_title = "HD Channels"`, **When** proxy generates, **Then** matching channels have group_title updated
2. **Given** I create a rule with regex `tvg_name matches "^(.+) \\((.+)\\)$" SET channel_name = "$1"`, **When** proxy generates, **Then** capture groups are applied
3. **Given** I create a conditional rule `tvg_logo contains "missing" SET_IF_EMPTY tvg_logo = "http://default.png"`, **When** proxy generates, **Then** only empty logos are updated
4. **Given** rules for EPG, **When** proxy generates, **Then** program metadata is transformed

---

### User Story 5 - Filtering Rules (Priority: P2)

As an IPTV administrator, I want to create filter expressions that include/exclude channels and programs so that I can curate content for specific proxies.

**Why this priority**: Content curation is valuable but system works without filters.

**Independent Test**: Can create a filter and see channels excluded from generated output.

**Acceptance Scenarios**:

1. **Given** I create a filter `group_title equals "Adult"` with action REMOVE, **When** proxy generates, **Then** matching channels are excluded
2. **Given** I create a filter with AND logic `group_title contains "Sports" AND channel_name contains "HD"`, **When** proxy generates, **Then** only channels matching both conditions are affected
3. **Given** I create a filter with OR logic, **When** proxy generates, **Then** channels matching either condition are affected
4. **Given** I create a filter with nested parentheses, **When** proxy generates, **Then** boolean logic is correctly evaluated

---

### User Story 6 - Channel Numbering (Priority: P2)

As an IPTV administrator, I want channels to have deterministic numbering so that clients have consistent channel numbers across regenerations.

**Why this priority**: Improves user experience but not essential for MVP.

**Independent Test**: Can configure numbering and see channel numbers in generated M3U.

**Acceptance Scenarios**:

1. **Given** a proxy with numbering enabled, **When** generation runs, **Then** channels have sequential tvg_chno values
2. **Given** numbering rules by group, **When** generation runs, **Then** each group starts from configured base number
3. **Given** regeneration occurs, **When** no channels added/removed, **Then** numbering remains identical

---

### User Story 7 - Logo Caching (Priority: P2)

As an IPTV administrator, I want channel logos to be cached locally so that clients load logos faster and upstream bandwidth is reduced.

**Why this priority**: Performance optimization, not essential for function.

**Independent Test**: Can enable logo caching and see logos downloaded to local storage.

**Acceptance Scenarios**:

1. **Given** logo caching enabled, **When** proxy generates, **Then** logos are downloaded to local cache
2. **Given** a cached logo exists, **When** proxy generates again, **Then** cached version is used (no re-download)
3. **Given** logo URL changes, **When** proxy generates, **Then** new logo is fetched
4. **Given** retention policy configured (default: 30 days unused), **When** cleanup runs, **Then** old/unused logos are removed

---

### User Story 8 - Stream Relay (Priority: P3)

As an IPTV administrator, I want to relay streams through the proxy with optional transcoding so that I can normalize stream formats and apply hardware-accelerated encoding.

**Why this priority**: Advanced feature, system fully functional without relay.

**Independent Test**: Can access a relayed stream URL and receive transcoded output.

**Acceptance Scenarios**:

1. **Given** relay enabled for a channel, **When** client accesses relay URL, **Then** stream is proxied through FFmpeg
2. **Given** a relay profile with transcoding options, **When** relay active, **Then** FFmpeg applies codec/bitrate settings
3. **Given** HLS source with single variant, **When** relay active, **Then** HLS is collapsed to continuous TS stream
4. **Given** hardware acceleration available (VAAPI/NVENC/QSV), **When** relay configured, **Then** hardware encoder is used
5. **Given** upstream fails, **When** relay active, **Then** circuit breaker opens after configured failures (default: 5 failures within 30 seconds, reset after 60 seconds half-open)

---

### User Story 9 - Scheduled Jobs (Priority: P2)

As an IPTV administrator, I want sources to refresh automatically on a schedule so that channel lists and EPG stay up-to-date.

**Why this priority**: Automation is valuable but manual refresh works for MVP.

**Independent Test**: Can configure cron schedule and see ingestion run automatically.

**Acceptance Scenarios**:

1. **Given** source has cron schedule configured, **When** schedule triggers, **Then** ingestion runs automatically
2. **Given** multiple jobs scheduled for same time, **When** trigger fires, **Then** jobs are deduplicated
3. **Given** ingestion fails, **When** next trigger fires, **Then** backoff is applied before retry
4. **Given** proxy has schedule configured, **When** schedule triggers, **Then** regeneration runs automatically

---

### User Story 10 - REST API (Priority: P1)

As a developer, I want a REST API to manage all entities so that I can integrate with external tools and build custom UIs.

**Why this priority**: API is the primary interface for the system.

**Independent Test**: Can perform CRUD operations on all entities via HTTP.

**Acceptance Scenarios**:

1. **Given** API is running, **When** I GET /api/v1/sources, **Then** list of sources is returned
2. **Given** API is running, **When** I POST /api/v1/sources with valid payload, **Then** source is created
3. **Given** API is running, **When** I request with invalid data, **Then** appropriate error response is returned
4. **Given** API is running, **When** I access /docs, **Then** OpenAPI documentation is served

---

### Edge Cases

- What happens when an M3U URL returns 404? → Source marked as failed, error logged, other sources continue
- What happens when XMLTV is malformed? → Parser handles gracefully, logs errors, continues with valid entries
- What happens when FFmpeg crashes during relay? → Process restarted, circuit breaker tracks failures
- What happens when database is full? → Graceful error handling, cleanup job prioritized
- What happens when two proxies use same source during concurrent generation? → Source data is shared, no duplicate fetching
- How does system handle 100k+ channels? → Streaming processing, batch database operations, memory cleanup between stages
- What happens when logo cache or output directory fills disk? → Logo caching skipped with warning logged, proxy generation fails gracefully with clear error message, system continues serving existing cached files

## Requirements

### Functional Requirements

#### Source Management
- **FR-001**: System MUST support M3U playlist sources with URL, name, and optional authentication
- **FR-002**: System MUST support Xtream Codes sources with server URL, username, password
- **FR-003**: System MUST support manual stream entry (individual channels without source URL)
- **FR-004**: System MUST handle compressed M3U files (gzip, bzip2, xz)
- **FR-005**: System MUST parse EXTINF metadata including tvg_id, tvg_name, tvg_logo, group_title
- **FR-006**: System MUST support custom field mapping via regex patterns

#### EPG Management
- **FR-010**: System MUST support XMLTV EPG sources
- **FR-011**: System MUST support Xtream EPG sources
- **FR-012**: System MUST parse program data: start/end times, title, description, category, episode info
- **FR-013**: System MUST associate EPG channels with stream channels via tvg_id matching

#### Proxy Management
- **FR-020**: System MUST support multiple proxy configurations
- **FR-021**: System MUST allow selecting multiple sources per proxy
- **FR-022**: System MUST allow selecting multiple EPG sources per proxy
- **FR-023**: System MUST allow assigning filters per proxy
- **FR-024**: System MUST generate M3U playlist output
- **FR-025**: System MUST generate XMLTV EPG output
- **FR-026**: System MUST support atomic file publishing (temp file → rename)

#### Expression Engine
- **FR-030**: System MUST support boolean operators (AND, OR, NOT)
- **FR-031**: System MUST support comparison operators (equals, contains, matches, starts_with, ends_with)
- **FR-032**: System MUST support regex matching with capture group substitution
- **FR-033**: System MUST support actions (SET, SET_IF_EMPTY, APPEND, REMOVE, DELETE)
- **FR-034**: System MUST support nested parentheses for complex expressions
- **FR-035**: System MUST support case-sensitive and case-insensitive matching

#### Relay System
- **FR-040**: System MUST proxy streams through FFmpeg
- **FR-041**: System MUST support relay profiles with codec/bitrate configuration
- **FR-042**: System MUST support hardware acceleration (VAAPI, NVENC, QSV, AMF)
- **FR-043**: System MUST support HLS collapsing (variant playlist → continuous stream)
- **FR-044**: System MUST implement circuit breaker for upstream failures (default: 5 failures within 30 seconds opens circuit, 60 seconds half-open reset)
- **FR-045**: System MUST support connection pooling with per-host concurrency limits

#### FFmpeg Integration
- **FR-050**: System MUST support external FFmpeg binary (system-installed)
- **FR-051**: System MUST support embedded FFmpeg binary (optional, via go-ffstatic or similar)
- **FR-052**: System MUST detect available hardware acceleration capabilities
- **FR-053**: System MUST cache ffprobe results per stream URL
- **FR-054**: System MUST manage FFmpeg process lifecycle with proper cleanup

#### Job Scheduling
- **FR-060**: System MUST support cron-based job scheduling
- **FR-061**: System MUST support immediate one-off job execution
- **FR-062**: System MUST deduplicate concurrent job requests
- **FR-063**: System MUST implement backoff for failed jobs
- **FR-064**: System MUST track job execution state and history

#### API
- **FR-070**: System MUST expose REST API for all CRUD operations
- **FR-071**: System MUST support OpenAPI documentation
- **FR-072**: System MUST return appropriate HTTP status codes
- **FR-073**: System MUST support JSON request/response format
- **FR-074**: System MUST support pagination for list endpoints
- **FR-075**: System MUST support configurable API authentication (MVP: none/disabled by default)

#### Storage
- **FR-080**: System MUST support SQLite database
- **FR-081**: System MUST support PostgreSQL database
- **FR-082**: System MUST support MySQL/MariaDB database
- **FR-083**: System MUST implement database migrations
- **FR-084**: System MUST sandbox file operations to configured directories

### Non-Functional Requirements

- **NFR-001**: System MUST process 100k+ channels with configurable memory limits (target: 500MB, threshold: 1GB peak RSS); streaming/batched processing required for large datasets
- **NFR-002**: System MUST process 1M+ EPG programs with configurable memory limits (target: 1GB, threshold: 2GB peak RSS); batch sizes configurable via config file

> **Note**: Memory limits are advisory targets achieved through streaming/batching design patterns. Runtime enforcement (e.g., GOMEMLIMIT, container memory limits) is a deployment concern, not application-enforced.
- **NFR-003**: System MUST be configurable via environment variables and config file
- **NFR-004**: System MUST provide structured logging with correlation IDs
- **NFR-005**: System MUST expose health check and metrics endpoints
- **NFR-006**: System MUST support graceful shutdown with configurable timeout (default: 30s)
- **NFR-007**: System MUST support configurable rate limiting for API and relay endpoints (MVP: unlimited/disabled by default)

### Key Entities

- **StreamSource**: Represents an upstream channel source (M3U URL, Xtream server)
- **Channel**: Individual channel parsed from a source
- **ManualStreamChannel**: User-defined channel not from external source
- **EpgSource**: Represents an upstream EPG source (XMLTV URL, Xtream EPG)
- **EpgProgram**: Individual program entry with schedule
- **StreamProxy**: Output configuration combining sources, filters, mappings
- **DataMappingRule**: Transformation rule for metadata
- **Filter**: Include/exclude expression for content curation
- **RelayProfile**: FFmpeg transcoding configuration
- **CachedLogoMetadata**: Cached logo with metadata (file-based storage with JSON sidecar, not database-stored per constitution Key Systems #3)
- **LastKnownCodec**: Cached ffprobe result per stream
- **Job**: Scheduled or immediate task execution record

## Success Criteria

### Measurable Outcomes

- **SC-001**: System can ingest 100k channels in under 5 minutes with memory usage under configured limit (default: 1GB)
- **SC-002**: System can ingest 1M EPG programs in under 10 minutes with memory usage under configured limit (default: 1GB)
- **SC-003**: Proxy generation for 50k channels completes in under 2 minutes
- **SC-004**: API response time under 200ms for list operations (paginated)
- **SC-005**: Relay stream startup time under 3 seconds
- **SC-006**: System remains stable under 24/7 operation with scheduled jobs
- **SC-007**: Database supports SQLite, PostgreSQL, and MySQL without code changes
- **SC-008**: All public APIs have OpenAPI documentation
- **SC-009**: Test coverage exceeds 80% for core business logic
