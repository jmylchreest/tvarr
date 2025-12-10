# Feature Specification: Smart Container Routing

**Feature Branch**: `013-smart-container-routing`
**Created**: 2025-12-10
**Status**: Draft
**Input**: User description: "Smart Container Routing with gohlslib: Implement intelligent stream format routing that uses gohlslib Muxer for pure HLS repackaging (avoiding FFmpeg overhead when no transcoding needed), adds X-Tvarr-Player headers to frontend players (mpegts.js, hls.js) for better client detection, and routes streams based on source format (HLS/DASH/TS) x client preference (fMP4/MPEG-TS) x codec compatibility. Only invoke FFmpeg pipeline when actual transcoding is required."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Efficient HLS-to-HLS Passthrough (Priority: P1)

A user streams a channel that has an HLS source to their HLS-capable player. The system detects that both source and client use HLS and that no transcoding is required (passthrough profile or compatible codecs), so it uses lightweight repackaging instead of spawning an FFmpeg process.

**Why this priority**: This is the most common streaming scenario and provides the biggest performance win - eliminating unnecessary FFmpeg processes for the majority of streams where source and client formats already match.

**Independent Test**: Can be fully tested by streaming an HLS source to an HLS player and verifying no FFmpeg process is spawned while playback works correctly.

**Acceptance Scenarios**:

1. **Given** a channel with an HLS source and a passthrough profile, **When** an HLS client requests the stream, **Then** the system delivers segments without invoking FFmpeg.
2. **Given** a channel with an HLS source, **When** two HLS clients request the same channel simultaneously, **Then** segments are shared from a single source connection (no duplicate upstream connections).
3. **Given** an HLS source delivering fMP4 segments, **When** a client requests HLS, **Then** the system serves the segments directly with appropriate HLS playlist.

---

### User Story 2 - Player Identity Headers (Priority: P2)

A user watches streams in the tvarr web interface. The frontend player (hls.js or mpegts.js) sends a custom X-Tvarr-Player header identifying itself, allowing the server to make optimal routing decisions based on known player capabilities.

**Why this priority**: Better client detection enables more accurate format routing, reducing fallback to conservative choices that may be suboptimal.

**Independent Test**: Can be tested by observing network requests from the frontend player and verifying the X-Tvarr-Player header is present with correct player identification.

**Acceptance Scenarios**:

1. **Given** the tvarr web player using hls.js, **When** it requests a stream, **Then** the request includes header `X-Tvarr-Player: hls.js/<version>`.
2. **Given** the tvarr web player using mpegts.js, **When** it requests a stream, **Then** the request includes header `X-Tvarr-Player: mpegts.js/<version>`.
3. **Given** a request with X-Tvarr-Player header, **When** the server routes the stream, **Then** it uses the player information to select the optimal container format.

---

### User Story 3 - Format Negotiation Decision Matrix (Priority: P2)

A user's player requests a stream. The system evaluates the source format, client container preference (from headers, URL, or User-Agent), and codec requirements to choose the optimal delivery path: passthrough, repackaging, or transcoding.

**Why this priority**: Core routing logic that determines system efficiency - wrong decisions lead to unnecessary transcoding or incompatible streams.

**Independent Test**: Can be tested by making requests with different format preferences and verifying the correct delivery path is selected.

**Acceptance Scenarios**:

1. **Given** an HLS source with H.264/AAC, **When** a client requests MPEG-TS format (via `?format=mpegts`), **Then** the system repackages HLS to MPEG-TS without transcoding.
2. **Given** a raw MPEG-TS source, **When** an HLS client requests the stream, **Then** the system uses FFmpeg for segmentation (repackaging not possible from unsegmented source).
3. **Given** a source with VP9 codec, **When** a client only supports H.264, **Then** the system invokes FFmpeg for transcoding regardless of container format match.
4. **Given** matching source/client formats with compatible codecs, **When** stream is requested, **Then** no FFmpeg process is spawned.

---

### User Story 4 - Container Format Override (Priority: P3)

A user with a legacy device needs MPEG-TS instead of the default fMP4. They append `?format=mpegts` to the stream URL, and the system honors this explicit request.

**Why this priority**: Provides escape hatch for edge cases and legacy compatibility without requiring profile changes.

**Independent Test**: Can be tested by requesting stream with format query parameter and verifying output format matches request.

**Acceptance Scenarios**:

1. **Given** any source format, **When** request includes `?format=mpegts`, **Then** output is MPEG-TS container.
2. **Given** any source format, **When** request includes `?format=fmp4`, **Then** output is fMP4 container with appropriate playlist.
3. **Given** an invalid format parameter, **When** request is made, **Then** system uses default format based on client detection.

---

### User Story 5 - Profile Detection Mode Behavior (Priority: P1)

A user configures a relay profile with `detection_mode` set to either "auto" or a specific mode (e.g., "hls", "mpegts"). When `detection_mode = "auto"`, the system uses client detection to optimize delivery. When `detection_mode != "auto"`, the profile settings are used exactly as configured, bypassing client detection entirely.

**Why this priority**: Detection mode represents explicit user intent. When set to non-auto, the user has specifically chosen output settings, so the system should respect those choices rather than second-guessing with client detection.

**Independent Test**: Can be tested by configuring profiles with different detection modes and verifying behavior matches expectations.

**Acceptance Scenarios**:

1. **Given** a profile with `detection_mode = "auto"`, **When** an HLS client requests an HLS source, **Then** client detection applies and passthrough/repackaging is considered based on client capabilities.
2. **Given** a profile with `detection_mode != "auto"` (e.g., "hls"), **When** any client requests the stream, **Then** profile settings are used as-is without client detection.
3. **Given** a profile with `detection_mode != "auto"` and `video_codec=copy`, **When** client sends X-Tvarr-Player header, **Then** the header is ignored for format selection (profile takes precedence).

---

### User Story 6 - Relay Flow Visualization Dashboard (Priority: P3)

A user opens the dashboard to monitor active relay sessions. The system displays an interactive network flow diagram in the active relay processes section, showing the data path from origin sources through a processor node (FFmpeg or gohlslib) to connected clients, with animated data flow and real-time metrics.

**Why this priority**: Provides visibility into relay pipeline internals, helping debug streaming issues and understand resource usage. Tracks both FFmpeg and gohlslib clients uniformly, showing RouteType for each session.

**Independent Test**: Open dashboard while streams are active, verify flow diagram displays all session components with animated data flow and live metrics.

**Library**: React Flow (@xyflow/react) with shadcn-compatible styling.

**Acceptance Scenarios**:

1. **Given** active relay sessions, **When** user opens the dashboard, **Then** a flow diagram shows Origin -> Processor -> Client nodes with connecting edges for each active relay.
2. **Given** a passthrough/repackage session using gohlslib, **When** viewed in the diagram, **Then** the processor node shows RouteType as "passthrough" or "repackage" with codec info, but no CPU/memory graphs (since gohlslib has no process metrics).
3. **Given** an FFmpeg transcoding session, **When** viewed in the diagram, **Then** the processor node shows RouteType as "transcode", codec transformation, and includes CPU/memory usage graphs within the card.
4. **Given** active streaming, **When** data flows through the relay, **Then** edges animate to show data direction with bandwidth metrics (bytes/s) displayed.
5. **Given** multiple clients on the same channel, **When** viewing the diagram, **Then** they share a single origin/processor node with individual client nodes branching off.

---

### Edge Cases

- What happens when a source format cannot be determined (e.g., stream not yet started)?
  - System waits for initial data to probe format, with timeout leading to fallback to FFmpeg pipeline.
- How does system handle mid-stream codec changes in source?
  - Routing decision made at session start; mid-stream changes may cause playback issues (documented limitation).
- What if client headers conflict (X-Tvarr-Player says fMP4, but URL says mpegts)?
  - Explicit URL parameter takes precedence over header-based detection.
- What happens when source connection fails during repackaging?
  - System attempts reconnection; if format changed, may need to restart client session.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST detect source stream format (HLS, DASH, raw TS) before routing decisions.
- **FR-002**: When profile `detection_mode = "auto"`, system MUST detect client container preference from: (1) URL format parameter, (2) X-Tvarr-Player header, (3) Accept headers, (4) User-Agent, in priority order.
- **FR-003**: When profile `detection_mode != "auto"`, the profile's settings MUST be used as-is without client detection. This applies even when profile uses `video_codec=copy` and `audio_codec=copy`.
- **FR-004**: System MUST use lightweight repackaging (gohlslib) when: profile is in auto mode, source is HLS/DASH, and client wants compatible format with matching codecs.
- **FR-005**: System MUST invoke FFmpeg pipeline when: (a) profile specifies non-copy codecs, (b) source has no segments to reuse (raw TS), or (c) codec mismatch detected in auto mode.
- **FR-006**: Frontend players MUST include X-Tvarr-Player header with player name and version.
- **FR-007**: System MUST support explicit format override via `?format=mpegts` and `?format=fmp4` query parameters (only applies when `detection_mode = "auto"`).
- **FR-008**: System MUST share upstream source connections when multiple clients request the same channel with compatible routing.
- **FR-009**: System MUST log routing decisions including source format, detection mode, client preference (if auto), and chosen path (passthrough/repackage/transcode).

See [relay-decision-flow.md](./relay-decision-flow.md) for the complete routing decision flowchart.

### Key Entities

- **StreamSession**: Active stream connection with routing decision (passthrough, repackage, or transcode), source format, client format, and upstream connection reference.
- **ClientCapabilities**: Detected client information including player type, supported containers (fMP4, MPEG-TS), and codec support.
- **RoutingDecision**: Enumeration of delivery paths: Passthrough (direct proxy), Repackage (container change, no codec processing), Transcode (FFmpeg required).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: HLS-to-HLS streams with passthrough profiles consume less than 10% of the CPU compared to FFmpeg-based delivery for the same content.
- **SC-002**: 95% of requests from tvarr web players include valid X-Tvarr-Player headers.
- **SC-003**: System correctly routes streams without FFmpeg for at least 60% of HLS source + HLS client combinations.
- **SC-004**: Stream start time for repackaged streams is under 2 seconds (comparable to or faster than FFmpeg-based delivery).
- **SC-005**: No increase in stream failure rate compared to current FFmpeg-only approach.
- **SC-006**: Server can handle 3x more concurrent passthrough streams than transcoded streams with equivalent resources.

## Assumptions

- gohlslib library supports both HLS consumption (Client) and HLS output (Muxer) for fMP4 and MPEG-TS segments.
- Source streams provide stable codec information that can be detected before routing.
- Frontend players (hls.js, mpegts.js) allow custom header injection without modification to core libraries.
- Most IPTV sources use H.264/AAC codecs, making passthrough feasible for majority of streams.
