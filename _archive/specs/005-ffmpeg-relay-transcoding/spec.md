# Feature Specification: FFmpeg Relay and Stream Transcoding Proxy

**Feature Branch**: `005-ffmpeg-relay-transcoding`
**Created**: 2025-12-05
**Status**: Draft
**Input**: User description: "Support wrapping ffmpeg and providing a cyclic buffer with different hw accelerated profiles to transcode the source stream behind a proxy endpoint. When a client queries the endpoint it should either redirect to the original source (redirect mode), proxy it on the clients behalf to the original source (proxy, care of CORS headers etc here), or spawn an on-demand ffmpeg instance and stream the cyclic buffer content. Multiple clients would attach to the same ffmpeg instance, when no clients remain the ffmpeg instance would be terminated. Support HLS collapsing, dashboard ffmpeg health information, and relay client connection tracking."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Redirect Mode Stream Access (Priority: P1)

An administrator configures a channel to use "redirect" mode in the relay profile. When a media player requests the stream URL, the system returns an HTTP redirect to the original source URL, allowing the player to connect directly to the upstream provider.

**Why this priority**: This is the simplest streaming mode with zero server overhead, essential for users who don't need transcoding or CORS handling.

**Independent Test**: Can be fully tested by configuring a channel with redirect mode, requesting the stream URL, and verifying the HTTP 302 redirect response points to the original source.

**Acceptance Scenarios**:

1. **Given** a channel configured with redirect mode relay profile, **When** a client requests the stream URL, **Then** the system returns HTTP 302 redirect to the original upstream URL
2. **Given** a channel using redirect mode, **When** the upstream source changes, **Then** subsequent redirects point to the updated URL
3. **Given** a redirect mode request, **When** the client follows the redirect, **Then** the original source responds directly to the client

---

### User Story 2 - Proxy Mode Stream Access (Priority: P1)

An administrator configures a channel to use "proxy" mode to handle CORS restrictions or hide upstream credentials. When a client requests the stream, the system fetches from the upstream source and forwards the content with appropriate CORS headers, without transcoding.

**Why this priority**: Proxy mode solves common browser CORS issues and credential hiding without the complexity of transcoding, making it essential for web-based players.

**Independent Test**: Can be fully tested by configuring proxy mode, requesting the stream from a browser, and verifying CORS headers are present and content streams successfully.

**Acceptance Scenarios**:

1. **Given** a channel configured with proxy mode, **When** a browser requests the stream, **Then** the response includes permissive CORS headers (Access-Control-Allow-Origin: *)
2. **Given** a proxy mode stream, **When** the upstream source requires authentication, **Then** the credentials are used server-side and not exposed to the client
3. **Given** a proxy mode request, **When** an OPTIONS preflight request arrives, **Then** the system responds with appropriate CORS preflight headers
4. **Given** an HLS source with multiple quality variants, **When** proxy mode is enabled with HLS collapsing, **Then** the system selects the highest quality variant and serves a single stream

---

### User Story 3 - Transcoded Stream Access with Shared FFmpeg Instance (Priority: P1)

A user requests a transcoded stream for a channel. The system spawns an FFmpeg process (or joins an existing one) that transcodes the upstream source according to the relay profile configuration. Multiple clients viewing the same channel share the single FFmpeg instance via a cyclic buffer.

**Why this priority**: On-demand transcoding with client sharing is the core value proposition, enabling format conversion and bandwidth management while optimizing server resources.

**Independent Test**: Can be fully tested by requesting a transcoded stream, verifying video playback, then connecting a second client and confirming both receive the same transcoded content from a single FFmpeg process.

**Acceptance Scenarios**:

1. **Given** a channel with transcode mode, **When** the first client requests the stream, **Then** an FFmpeg process starts and begins transcoding
2. **Given** an active transcoding session, **When** a second client requests the same stream, **Then** they join the existing FFmpeg session without spawning a new process
3. **Given** multiple clients on a shared transcode session, **When** each client disconnects at different times, **Then** each receives their own independent stream position from the cyclic buffer
4. **Given** all clients have disconnected from a transcode session, **When** the idle timeout expires (default 60 seconds), **Then** the FFmpeg process terminates automatically

---

### User Story 4 - Hardware Accelerated Transcoding (Priority: P2)

An administrator configures hardware acceleration for transcoding to reduce CPU usage. The system auto-detects available hardware encoders (VAAPI, NVENC, QSV, AMF, VideoToolbox) and uses them when the relay profile specifies hardware acceleration.

**Why this priority**: Hardware acceleration significantly reduces server load and enables more concurrent streams, but the system works without it using software encoding.

**Independent Test**: Can be fully tested by configuring a relay profile with hardware acceleration enabled, starting a transcode session, and verifying the FFmpeg process uses hardware encoding (observable in logs/metrics).

**Acceptance Scenarios**:

1. **Given** a system with NVIDIA GPU, **When** hardware acceleration is enabled in the relay profile, **Then** FFmpeg uses NVENC for encoding
2. **Given** no compatible hardware acceleration available, **When** hardware acceleration is requested, **Then** the system falls back to software encoding gracefully
3. **Given** multiple hardware accelerators available, **When** a relay profile is configured, **Then** the administrator can select the preferred accelerator type

---

### User Story 5 - Relay Profile Configuration (Priority: P2)

An administrator creates relay profiles that define transcoding parameters including video codec, audio codec, bitrates, and hardware acceleration settings. These profiles can be assigned to channels or proxies to control how streams are delivered.

**Why this priority**: Relay profiles provide reusable transcoding configurations essential for managing diverse streaming requirements across many channels.

**Independent Test**: Can be fully tested by creating a relay profile with specific codec settings, assigning it to a channel, requesting the stream, and verifying the output format matches the profile configuration.

**Acceptance Scenarios**:

1. **Given** the admin interface, **When** creating a relay profile, **Then** video codec options include H264, H265, AV1, MPEG2, and Copy (passthrough)
2. **Given** a relay profile configuration, **When** audio codec is specified, **Then** options include AAC, MP3, AC3, EAC3, DTS, and Copy
3. **Given** a relay profile, **When** bitrate settings are configured, **Then** both video and audio bitrates can be specified independently
4. **Given** a channel or proxy, **When** a relay profile is assigned, **Then** all streams for that entity use the profile settings

---

### User Story 6 - FFmpeg Process Health Monitoring (Priority: P2)

An administrator views the dashboard to monitor active FFmpeg transcoding sessions. The dashboard displays process health including CPU usage, memory consumption, uptime, and error status for each active relay.

**Why this priority**: Health monitoring enables proactive management of transcoding resources and troubleshooting of stream issues.

**Independent Test**: Can be fully tested by starting a transcode session, viewing the dashboard, and verifying metrics update in real-time.

**Acceptance Scenarios**:

1. **Given** an active transcoding session, **When** viewing the dashboard, **Then** the relay shows process metrics (CPU, memory, uptime)
2. **Given** an FFmpeg process encounters errors, **When** error threshold is exceeded, **Then** the system activates fallback mode and displays error status
3. **Given** multiple active relays, **When** viewing the dashboard, **Then** all relays are listed with individual health indicators

---

### User Story 7 - Connected Client Tracking (Priority: P2)

An administrator views connected clients for each active relay. The dashboard shows client IP addresses, user agents, connection duration, and bytes served per client.

**Why this priority**: Client tracking enables capacity planning and troubleshooting of individual viewer issues.

**Independent Test**: Can be fully tested by connecting multiple clients to a relay, viewing the dashboard, and verifying each client appears with accurate connection details.

**Acceptance Scenarios**:

1. **Given** clients connected to a relay, **When** viewing the relay details, **Then** each client shows IP address and user agent
2. **Given** an active client connection, **When** viewing client details, **Then** connection start time and bytes served are displayed
3. **Given** a client disconnects, **When** viewing the relay details, **Then** the client is removed from the connected list

---

### User Story 8 - Error Fallback with Visual Indicator (Priority: P3)

When an FFmpeg transcode session encounters persistent errors, the system generates and serves a Transport Stream compatible error image to connected clients, ensuring graceful degradation rather than stream failure.

**Why this priority**: Error fallback maintains client connections during upstream issues, but is less critical than core streaming functionality.

**Independent Test**: Can be fully tested by simulating an upstream failure during transcoding and verifying clients receive the fallback error stream.

**Acceptance Scenarios**:

1. **Given** an FFmpeg process experiencing upstream errors, **When** error threshold is exceeded, **Then** the system switches to serving a fallback error stream
2. **Given** fallback mode is active, **When** the upstream recovers, **Then** the system resumes normal transcoding automatically
3. **Given** fallback mode, **When** new clients connect, **Then** they receive the fallback stream until normal operation resumes

---

### Edge Cases

- What happens when FFmpeg is not installed or not in PATH? System should report clear error and disable transcode mode.
- How does the system handle upstream HLS sources that change segment URLs? HLS collapsing should handle playlist updates gracefully.
- What happens when the cyclic buffer fills faster than clients consume? Older data is discarded, clients may experience gaps.
- How does the system handle clients with significantly different consumption rates? Each client tracks their own buffer position independently.
- What happens during a relay profile change while clients are connected? Active sessions continue with existing settings; new connections use updated profile.
- How does the system handle upstream authentication failures? Report error, activate fallback mode, log the issue.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST support three stream delivery modes: redirect, proxy, and transcode
- **FR-002**: System MUST return HTTP 302 redirects for channels/proxies configured with redirect mode
- **FR-003**: System MUST proxy upstream content with CORS headers for channels/proxies configured with proxy mode
- **FR-004**: System MUST spawn FFmpeg processes on-demand for transcode mode requests
- **FR-005**: System MUST implement a cyclic buffer allowing multiple clients to read from a single FFmpeg output
- **FR-006**: System MUST track each client's buffer position independently for seamless playback
- **FR-007**: System MUST automatically terminate FFmpeg processes after configurable idle timeout when no clients remain
- **FR-008**: System MUST auto-detect available hardware acceleration capabilities at startup
- **FR-009**: System MUST support hardware-accelerated encoding via VAAPI, NVENC, QSV, AMF, and VideoToolbox
- **FR-010**: System MUST fall back to software encoding when hardware acceleration is unavailable
- **FR-011**: System MUST support video codecs: H264, H265, AV1, MPEG2, and passthrough (copy)
- **FR-012**: System MUST support audio codecs: AAC, MP3, AC3, EAC3, DTS, and passthrough (copy)
- **FR-013**: System MUST allow configurable video and audio bitrates in relay profiles
- **FR-014**: System MUST collapse HLS multi-variant playlists to single highest-quality stream when enabled
- **FR-015**: System MUST include permissive CORS headers (Access-Control-Allow-Origin: *) for proxy and transcode modes
- **FR-016**: System MUST handle OPTIONS preflight requests for browser compatibility
- **FR-017**: System MUST monitor FFmpeg stderr for error patterns and activate fallback mode when threshold exceeded
- **FR-018**: System MUST generate Transport Stream compatible fallback content during error conditions
- **FR-019**: System MUST expose relay health metrics via dashboard and API (CPU, memory, uptime, error state)
- **FR-020**: System MUST track connected clients per relay (IP, user agent, duration, bytes served)
- **FR-021**: System MUST automatically remove stale clients after inactivity timeout (default 30 seconds)
- **FR-022**: System MUST support custom FFmpeg argument override in relay profiles for advanced users
- **FR-023**: System MUST clean up idle FFmpeg processes via background task (default check every 10 seconds)

### Key Entities

- **Relay Profile**: Defines transcoding configuration including mode (redirect/proxy/transcode), video codec, audio codec, bitrates, hardware acceleration settings, and optional custom FFmpeg arguments
- **Active Relay**: Runtime state of a streaming session including the FFmpeg process reference, cyclic buffer, connected clients, health metrics, and associated relay profile
- **Connected Client**: Tracks individual viewer including unique ID, IP address, user agent, buffer read position, bytes served, and connection timestamps
- **Cyclic Buffer**: In-memory ring buffer storing transcoded stream chunks with sequence numbers, enabling multiple clients to read at different positions
- **Hardware Accelerator**: Detected encoding capability including type (VAAPI, NVENC, etc.), supported codecs, and availability status

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can switch between redirect, proxy, and transcode modes without service interruption
- **SC-002**: Multiple clients viewing the same transcoded channel share resources from a single process
- **SC-003**: Stream playback begins within 3 seconds of request for all delivery modes
- **SC-004**: System supports at least 10 concurrent transcoding sessions on typical hardware
- **SC-005**: Hardware acceleration reduces resource usage by at least 50% compared to software encoding when available
- **SC-006**: Clients experience less than 1 second of stream interruption when connecting to an active transcode session
- **SC-007**: Dashboard displays relay health information with less than 5 second update latency
- **SC-008**: System automatically recovers from upstream errors within 30 seconds when source becomes available
- **SC-009**: Idle FFmpeg processes terminate within 90 seconds of last client disconnect
- **SC-010**: Administrators can configure relay profiles and see changes applied to new connections immediately

## Assumptions

- FFmpeg is installed and available in the system PATH for transcode functionality
- Hardware acceleration requires appropriate drivers and permissions configured at the OS level
- The existing relay profile model in tvarr will be extended to include transcoding settings
- The cyclic buffer implementation will be inspired by the m3u-proxy Rust implementation but adapted for Go
- Stream proxy endpoints will follow the existing tvarr URL patterns (e.g., /relay/{channel_id}/stream)
- Browser clients will be the primary consumer of CORS-enabled streams
