# Feature Specification: Multi-Format Streaming Support

**Feature Branch**: `008-multi-format-streaming`
**Created**: 2025-12-07
**Status**: Draft
**Input**: User description: "Investigate (and potentially implement) other streaming formats in relay mode (and proxy mode where possible) other than mpegts. HLS is the obvious additional format, please be comprehensive"

## Clarifications

### Session 2025-12-07

- Q: How do clients differentiate/request specific output formats? → A: Query parameter (`?format=hls`, `?format=dash`, `?format=mpegts`) matching existing proxy mode pattern. This is relay-only; m3u URLs remain unchanged. Default is MPEG-TS for backwards compatibility.
- Q: How should segment URLs be structured for HLS/DASH, and how does this relate to proxy mode? → A: Query-param driven throughout. Same base URL serves any format; segments referenced via `?format=hls&seg=N`. This enables dynamic reconfiguration without m3u regeneration, per-client format selection, and runtime config changes without breaking existing clients.
- Q: How should auto-format selection work? → A: Optional via proxy configuration with default of "auto". Proxy config defines the preferred format; clients can override via query parameter. Auto-detection uses User-Agent/Accept headers to serve optimal format per client.
- Q: Should codec availability depend on output format? → A: Yes. UI dropdowns should be container-aware. VP9, AV1, and Opus are only available when DASH is selected (fMP4 segments support these codecs). MPEG-TS and HLS (.ts segments) are limited to H.264/H.265 video and AAC/MP3/AC3/EAC3 audio.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - HLS Output for Device Compatibility (Priority: P1)

A user wants to serve streams via HLS (HTTP Live Streaming) to maximize device compatibility, particularly for Apple devices (iOS, tvOS, Safari) and smart TVs that prefer or require HLS playback.

**Why this priority**: HLS is the most widely supported adaptive streaming format. Apple devices require HLS for native video playback. Many smart TVs and streaming devices also prefer HLS. This dramatically expands the ecosystem of compatible playback devices.

**Independent Test**: Can be fully tested by configuring a relay profile with HLS output, playing the stream on an iOS device or Safari browser, and verifying smooth playback with proper segment delivery.

**Acceptance Scenarios**:

1. **Given** a relay profile configured with HLS output format, **When** a client requests the stream, **Then** the system returns a valid HLS master playlist (.m3u8) with segment references
2. **Given** an HLS stream is active, **When** a client requests a segment (.ts), **Then** the segment is delivered with correct content-type headers
3. **Given** multiple clients connect to the same HLS relay, **When** streaming is active, **Then** all clients receive the same segments without redundant transcoding
4. **Given** a client connects mid-stream, **When** the HLS playlist is requested, **Then** the playlist contains recent segments allowing immediate playback

---

### User Story 2 - DASH Output for Cross-Platform Streaming (Priority: P2)

A user wants to serve streams via DASH (Dynamic Adaptive Streaming over HTTP) for broad cross-platform compatibility, particularly for Android devices, Chromecast, and web browsers using modern video players.

**Why this priority**: DASH is the international standard for adaptive streaming (ISO/IEC 23009-1) and is widely supported on non-Apple platforms. Together with HLS, these two formats cover virtually all streaming devices.

**Independent Test**: Can be tested by configuring a relay profile with DASH output, playing the stream in a DASH-compatible player (dash.js, Shaka Player), and verifying manifest updates and segment delivery.

**Acceptance Scenarios**:

1. **Given** a relay profile configured with DASH output format, **When** a client requests the stream, **Then** the system returns a valid DASH manifest (MPD) with segment references
2. **Given** a DASH stream is active, **When** a client requests an initialization segment or media segment, **Then** the segment is delivered correctly
3. **Given** the source stream has both video and audio, **When** DASH output is configured, **Then** the manifest correctly describes both adaptation sets

---

### User Story 3 - Passthrough Proxy Mode Format Preservation (Priority: P1)

In proxy mode (no transcoding), users want the system to pass through the original stream format while still providing value-add features like caching, connection pooling, and client multiplexing.

**Why this priority**: Many source streams are already in HLS/DASH format. Users want the benefits of the proxy (reduced upstream connections, caching) without format conversion overhead.

**Independent Test**: Can be tested by configuring a passthrough proxy for an HLS source, connecting multiple clients, and verifying only one upstream connection is used while all clients receive valid HLS.

**Acceptance Scenarios**:

1. **Given** a source stream in HLS format, **When** proxy passthrough mode is configured, **Then** clients receive valid HLS playlists with segments from the proxy
2. **Given** multiple clients connect to an HLS passthrough proxy, **When** streaming is active, **Then** only one upstream connection fetches segments
3. **Given** an HLS passthrough proxy is active, **When** the upstream source updates segments, **Then** clients receive updated playlists within the refresh interval

---

### User Story 4 - Format Auto-Selection Based on Client (Priority: P3)

A user wants the system to automatically serve the optimal format based on the requesting client's capabilities, without requiring separate endpoints or manual configuration.

**Why this priority**: Reduces configuration complexity and ensures all clients receive the best possible experience without user intervention.

**Independent Test**: Can be tested by requesting the same stream URL from different clients (Safari, Chrome, VLC) and verifying each receives the appropriate format.

**Acceptance Scenarios**:

1. **Given** auto-format selection is enabled, **When** Safari/iOS requests a stream, **Then** the system serves HLS
2. **Given** auto-format selection is enabled, **When** a client requests with Accept header indicating DASH preference, **Then** the system serves DASH
3. **Given** auto-format selection is enabled, **When** a generic player requests the stream, **Then** the system serves MPEG-TS as the universal fallback

---

### Edge Cases

- What happens when the source stream format is incompatible with the requested output format? System should provide clear error or attempt automatic format conversion
- How does the system handle HLS/DASH segment storage when memory is limited? Use configurable ring buffer with oldest segment eviction
- What happens if a client requests a segment that has already expired from the buffer? Return 404 with appropriate messaging indicating segment unavailable
- How does the system handle HLS variant playlists when source provides multiple quality levels? Pass through as-is in proxy mode; relay mode focuses on single quality
- What happens during stream discontinuities (ad breaks, encoder restarts)? Handle EXT-X-DISCONTINUITY tags correctly, maintain sequence continuity
- What happens when switching from one format endpoint to another mid-session? Each format maintains independent state; clients must reconnect for format changes
- What happens when a user selects VP9/AV1/Opus codec with MPEG-TS or HLS format? UI prevents selection; validation blocks save with clear error message
- What happens when output format changes and current codec becomes incompatible? UI warns user and suggests compatible alternatives; does not auto-change codec

## Requirements *(mandatory)*

### Functional Requirements

#### Core Format Support

- **FR-001**: System MUST support HLS output with live playlist generation (.m3u8) and segment serving (.ts)
- **FR-002**: System MUST support DASH output with live manifest generation (MPD) and segment serving (m4s/mp4)
- **FR-003**: System MUST continue to support MPEG-TS output as the default format for maximum compatibility

#### HLS Specific Requirements

- **FR-010**: HLS output MUST generate valid EXT-X-VERSION 3+ playlists
- **FR-011**: HLS output MUST support configurable segment duration (default: 6 seconds)
- **FR-012**: HLS output MUST support configurable playlist size/segment retention (default: 5 segments)
- **FR-013**: HLS output MUST include EXT-X-TARGETDURATION, EXT-X-MEDIA-SEQUENCE, and EXT-X-DISCONTINUITY tags as appropriate
- **FR-014**: HLS segment storage MUST use a memory-based ring buffer to avoid disk I/O bottlenecks
- **FR-015**: HLS output MUST set correct Content-Type headers (application/vnd.apple.mpegurl for playlists, video/MP2T for segments)

#### DASH Specific Requirements

- **FR-020**: DASH output MUST generate valid DASH-IF compliant manifests
- **FR-021**: DASH output MUST support live profile with segment timeline or segment template
- **FR-022**: DASH output MUST support configurable segment duration matching HLS for consistency
- **FR-023**: DASH manifest MUST update with minimumUpdatePeriod for live streams

#### Container-Aware Codec Selection

- **FR-024**: UI MUST display codec options based on the selected output format (container-aware dropdowns)
- **FR-025**: MPEG-TS and HLS output formats MUST limit video codecs to: copy, H.264 (libx264, h264_nvenc, h264_qsv, h264_vaapi), H.265 (libx265, hevc_nvenc, hevc_qsv, hevc_vaapi)
- **FR-026**: MPEG-TS and HLS output formats MUST limit audio codecs to: copy, AAC, MP3, AC3, EAC3
- **FR-027**: DASH output format MUST support all codecs from FR-025/FR-026 plus: VP9 (libvpx-vp9), AV1 (libaom-av1, av1_nvenc, av1_qsv), Opus (libopus)
- **FR-028**: System MUST validate codec/format compatibility and prevent invalid combinations at profile save time
- **FR-029**: When output format changes, UI SHOULD warn if currently selected codec becomes unavailable

#### Passthrough/Proxy Mode

- **FR-030**: Proxy mode MUST support HLS source passthrough with playlist URL rewriting
- **FR-031**: Proxy mode MUST support DASH source passthrough with manifest URL rewriting
- **FR-032**: Passthrough mode MUST rewrite segment URLs to route through proxy
- **FR-033**: Passthrough mode MUST cache segments to reduce upstream requests
- **FR-034**: Passthrough mode MUST support multiple clients sharing cached segments

#### Configuration & API

- **FR-040**: Relay profiles MUST allow output format selection (MPEG-TS, HLS, DASH)
- **FR-041**: HLS/DASH profiles MUST allow configuration of segment duration and retention count
- **FR-042**: System MUST support `?format=` query parameter on relay endpoints to select output format (hls, dash, mpegts, auto), matching existing proxy mode pattern
- **FR-043**: System MUST default to MPEG-TS format when no format parameter is specified (backwards compatibility)
- **FR-044**: For HLS/DASH formats, playlists/manifests MUST reference segments using query parameters on the same base URL (e.g., `?format=hls&seg=0`)
- **FR-045**: System MUST allow dynamic reconfiguration (relay profile changes, format availability) without requiring m3u regeneration
- **FR-046**: Proxy configuration SHOULD support a default format setting (auto, hls, dash, mpegts) that applies when clients don't specify a format
- **FR-047**: When format is "auto", system SHOULD detect optimal format based on User-Agent and Accept headers (Safari/iOS → HLS, DASH-preference → DASH, fallback → MPEG-TS)

#### Error Handling

- **FR-050**: System MUST return appropriate HTTP status codes for segment requests (200, 404, 503)
- **FR-051**: System MUST handle source stream interruptions gracefully with appropriate manifest updates
- **FR-052**: System MUST validate output format compatibility with selected codecs before starting relay

### Key Entities

- **Segment**: A discrete chunk of media content with sequence number, timestamp, duration, and binary data
- **Playlist/Manifest**: A text document describing available segments, their order, and stream metadata (HLS: .m3u8, DASH: .mpd)
- **SegmentBuffer**: An in-memory ring buffer holding recent segments for multi-client access, with configurable capacity
- **StreamSession**: An active relay session that manages segment generation and client delivery based on requested format
- **FormatParameter**: The `?format=` query parameter value (hls, dash, mpegts, auto) that determines output format for a relay session; "auto" triggers client detection logic

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: HLS streams are playable on iOS Safari, Apple TV, and VLC within 15 seconds of client connection
- **SC-002**: DASH streams are playable in dash.js and Shaka Player within 15 seconds of client connection
- **SC-003**: HLS/DASH segment delivery latency is under 500ms from segment availability to client receipt
- **SC-004**: Memory usage for segment buffering stays under 100MB per active stream with default settings (5 segments × 6 seconds)
- **SC-005**: Multiple clients (10+) can share a single HLS/DASH stream without additional transcoding or upstream connections
- **SC-006**: Stream startup time for new clients is under 20 seconds (time to first playable frame)
- **SC-007**: Passthrough proxy reduces upstream connection count by at least 80% for multi-client scenarios
- **SC-008**: System correctly serves streams to 95% of tested device/player combinations without compatibility issues

## Assumptions

- FFmpeg 5.0+ is available with HLS and DASH muxer support
- Clients have sufficient bandwidth for the selected quality/bitrate
- Source streams provide stable, continuous media (standard IPTV behavior)
- HLS Low-Latency (LL-HLS) and DASH Low-Latency (LL-DASH) are out of scope for initial implementation
- Adaptive bitrate (ABR) with multiple quality levels is out of scope; focus is on single-quality live streaming
- Segment storage is memory-only; disk-based persistence is not required
- DASH output uses fMP4 segments which support VP9, AV1, and Opus codecs not available in MPEG-TS/HLS containers

## Out of Scope

- DRM/content protection integration (Widevine, FairPlay, PlayReady)
- Adaptive bitrate transcoding with multiple renditions
- DVR/timeshift functionality beyond segment buffer retention
- HLS Low-Latency (LL-HLS) extensions
- CMAF (Common Media Application Format) output
- Recording/archival to persistent storage
- WebRTC output (requires STUN/TURN infrastructure)
