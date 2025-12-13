# Feature Specification: Relay Profile Simplification

**Feature Branch**: `014-relay-profile-simplify`
**Created**: 2025-12-12
**Status**: Draft
**Input**: User description: "do a thorough review, taking into account our recent automatic client detection code, please look at how relay profiles apply to proxies. The plan is to heavily simplify them while still allowing a manual codec profile to be set, forcing encoding to a specific version. I think perhaps we could essentially reduce it down to direct/relay modes with auto or profile based encoding, and reduce the other options. A profile could simply say 'use client detection' as an option with a default target codec which would override the fallthrough client detection one if its used (else be the only variant). Propose a plan, the aim is to simplify things for the end user, make it simple out of the box to use, have it work with the broadest set of devices without config, and reduce code complexity"

## Background & Problem Statement

The current relay profile system has grown complex with many overlapping concepts:

1. **RelayProfile** (40+ fields): Extensive FFmpeg settings, codecs, containers, detection modes, fallback settings, buffer settings, hardware acceleration, etc.
2. **RelayProfileMapping**: Expression-based client detection rules that determine accepted codecs per client type
3. **StreamProxy.ProxyMode**: `direct` vs `smart` modes
4. **StreamProxy.RelayProfileID**: Optional profile reference
5. **DetectionMode** in profiles: `auto`, `hls`, `mpegts`, `dash`
6. **ContainerFormat** in profiles: `auto`, `fmp4`, `mpegts`
7. **VideoCodec/AudioCodec**: Including `auto`, `copy`, and specific codecs

The automatic client detection system (recently implemented) already handles:
- User-Agent pattern matching for known players (VLC, Kodi, browsers, IPTV clients, etc.)
- X-Tvarr-Player header support for explicit player identification
- Accept header parsing
- Format override query parameters
- Smart routing decisions (passthrough, repackage, transcode)

### Client Detection Improvements

The client detection system should be enhanced with:

1. **First-match-wins behavior**: Detection rules are evaluated in priority order and return immediately on the first match (no further evaluation)

2. **Explicit codec request headers via expression rules**: Use the existing expression engine's `@header_req:` dynamic field resolver to support explicit codec headers. This requires adding high-priority built-in rules (not new code):

   Example built-in rules (highest priority):
   ```
   # Rule: "Explicit H.265 Video Request" (priority: 1)
   Expression: @header_req:X-Video-Codec == "h265"
   PreferredVideoCodec: h265

   # Rule: "Explicit H.264 Video Request" (priority: 2)
   Expression: @header_req:X-Video-Codec == "h264"
   PreferredVideoCodec: h264

   # Similar rules for audio codecs...
   ```

   Clients can then send `X-Video-Codec: h265` or `X-Audio-Codec: aac` headers to explicitly request codecs.

3. **Detection priority order** (achieved via rule priorities):
   1. Explicit codec header rules (`@header_req:X-Video-Codec`, `@header_req:X-Audio-Codec`) - priority 1-10
   2. Format override query parameter rules - priority 11-20
   3. `X-Tvarr-Player` header rules - priority 21-30
   4. `Accept` header rules - priority 31-40
   5. `User-Agent` pattern matching rules - priority 50+
   6. Default fallback (H.264/AAC in MPEG-TS) - lowest priority

This leverages the existing expression engine (`@header_req:` resolver) without code changes, just additional built-in rules.

**The core problem**: Users face overwhelming configuration options when the system can already make intelligent decisions automatically. The existing client detection is powerful but underutilized because profiles add redundant complexity.

## Proposed Simplification

### New Model: Two-Tier System

**Tier 1 - Proxy Mode (unchanged)**:
- `direct`: HTTP 302 redirect to source (zero processing)
- `smart`: Intelligent delivery with optional encoding profile

**Tier 2 - Encoding Mode (simplified)**:
- `auto` (default): Use built-in client detection to determine optimal delivery
- `profile`: Apply a specific encoding profile (forces transcoding to target codec)

### Simplified Encoding Profile

Instead of 40+ fields, encoding profiles become:

| Field               | Purpose                                               |
| ------------------- | ----------------------------------------------------- |
| Name                | Human-readable identifier                             |
| Description         | What this profile does                                |
| Target Video Codec  | h264, h265, vp9, av1                                  |
| Target Audio Codec  | aac, opus, ac3                                        |
| Quality Preset      | low, medium, high, ultra (maps to bitrates/CRF)       |
| Hardware Accel      | auto, none, cuda, vaapi, qsv                          |

Advanced users who need custom FFmpeg flags can use the existing "custom command" escape hatch.

### What Gets Removed/Simplified

1. **Remove RelayProfileMapping entirely (immediate removal)**: The built-in client detection handles this automatically based on User-Agent patterns. No deprecation period - the migration will convert any relevant settings and remove the table.
2. **Remove DetectionMode from profiles**: Detection is always on in `smart` mode
3. **Remove ContainerFormat from profiles**: Auto-selected based on codec and client
4. **Remove individual bitrate/buffer/timeout fields**: Replaced by quality presets
5. **Remove ForceVideoTranscode/ForceAudioTranscode**: Profiles always transcode when set
6. **Remove fallback configuration**: Simplified to a global toggle

### Smart Defaults for Proxy Creation

When creating a new proxy, the system should pre-select sensible defaults to minimize clicks:

1. **All stream sources selected by default**: Users typically want all their channels available
2. **All system filters enabled by default**: Standard filters (deduplication, cleanup, etc.) should be on
3. **Smart mode enabled by default**: Most users want intelligent delivery, not raw redirects
4. **No encoding profile by default**: Auto-detection handles most cases without forced transcoding

Users can then deselect sources/filters they don't want rather than manually adding everything.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Zero-Config Streaming (Priority: P1)

A new user sets up tvarr and creates a proxy. The proxy creation form pre-selects all available stream sources and system filters. Without any additional configuration, they connect from various devices (phone browser, VLC, Kodi, smart TV) and each receives an optimal stream format automatically.

**Why this priority**: This is the primary value proposition - works out of the box with maximum device compatibility and minimal clicks.

**Independent Test**: Create a proxy with default settings (accepting all pre-selected defaults), connect from multiple client types, verify each receives playable content without manual configuration.

**Acceptance Scenarios**:

1. **Given** the proxy creation form, **When** user opens it, **Then** all available stream sources are pre-selected
2. **Given** the proxy creation form, **When** user opens it, **Then** all system filters are pre-selected and enabled
3. **Given** the proxy creation form, **When** user opens it, **Then** smart mode is selected by default (not direct mode)
4. **Given** a proxy with default settings (smart mode, auto encoding), **When** a Chrome browser connects, **Then** it receives HLS with fMP4 segments and H.265 video (or passthrough if source is compatible)
5. **Given** a proxy with default settings, **When** VLC connects, **Then** it receives MPEG-TS with passthrough codecs (if source is compatible) or transcoded content
6. **Given** a proxy with default settings, **When** an IPTV client connects, **Then** it receives MPEG-TS format optimized for legacy devices
7. **Given** a proxy with default settings, **When** Safari connects, **Then** it receives HLS (no VP9/Opus which Safari doesn't support)

---

### User Story 2 - Force Specific Codec (Priority: P2)

An advanced user has a specific requirement: all streams must be H.264 for compatibility with an older set-top box. They create an encoding profile and apply it to their proxy.

**Why this priority**: Covers the power-user case of needing explicit control over output format.

**Independent Test**: Create an H.264 profile, apply to proxy, verify all clients receive H.264 regardless of their detected capabilities.

**Acceptance Scenarios**:

1. **Given** a proxy with an H.264 encoding profile, **When** any client connects, **Then** it receives H.264 video regardless of client capabilities
2. **Given** a proxy with an H.265 encoding profile, **When** a Firefox browser connects (which has limited H.265 support), **Then** it still receives H.265 (user explicitly chose this)
3. **Given** a proxy with a VP9 encoding profile, **When** a client connects, **Then** it receives VP9 in fMP4 container (auto-selected for VP9)

---

### User Story 3 - Direct Mode Bypass (Priority: P3)

A user wants zero overhead for a specific proxy - just redirect to the source URL. They set the proxy to direct mode.

**Why this priority**: Provides escape hatch for users who don't want any processing.

**Independent Test**: Set proxy to direct mode, verify 302 redirect is returned instead of proxied content.

**Acceptance Scenarios**:

1. **Given** a proxy in direct mode, **When** any client connects, **Then** it receives a 302 redirect to the source URL
2. **Given** a proxy in direct mode with an encoding profile assigned, **When** any client connects, **Then** it still receives a 302 redirect (direct mode ignores profiles)

---

### User Story 4 - Quality Presets (Priority: P3)

A user wants to reduce bandwidth without understanding bitrate settings. They select a "medium" quality preset on their encoding profile.

**Why this priority**: Simplifies the most common advanced use case.

**Independent Test**: Create profile with different quality presets, verify output bitrates match expected ranges.

**Acceptance Scenarios**:

1. **Given** a profile with "low" quality preset, **When** transcoding occurs, **Then** output video bitrate is approximately 1-2 Mbps
2. **Given** a profile with "high" quality preset, **When** transcoding occurs, **Then** output video bitrate is approximately 6-10 Mbps
3. **Given** a profile with "ultra" quality preset, **When** transcoding occurs, **Then** maximum quality is used with CRF-based encoding

---

### User Story 5 - Explicit Codec Headers (Priority: P2)

A developer building a custom IPTV client wants to explicitly request H.265 video regardless of what User-Agent detection would choose. They send the `X-Video-Codec: h265` header with their requests. This is handled by built-in detection rules using the existing `@header_req:` expression syntax.

**Why this priority**: Enables programmatic control for integrations and advanced users without relying on User-Agent heuristics.

**Independent Test**: Send requests with explicit codec headers, verify the built-in rules match them before User-Agent rules.

**Acceptance Scenarios**:

1. **Given** a request with `X-Video-Codec: h265` header, **When** client connects, **Then** built-in rule `@header_req:X-Video-Codec == "h265"` matches and system serves H.265 video
2. **Given** a request with `X-Audio-Codec: opus` header, **When** client connects, **Then** built-in rule matches and system serves Opus audio
3. **Given** a request with both `X-Video-Codec: h264` and `X-Audio-Codec: aac` headers, **When** client connects, **Then** system serves H.264 video with AAC audio
4. **Given** a request with `X-Video-Codec: invalid-codec` header, **When** client connects, **Then** no built-in rule matches (no rule for invalid values) and detection falls through to User-Agent rules
5. **Given** a request with `X-Video-Codec: h265` but User-Agent indicates Safari, **When** client connects, **Then** system serves H.265 (header rule has higher priority than User-Agent rules)

---

### User Story 6 - E2E Client Detection Testing (Priority: P2)

A developer wants to verify that client detection and codec selection work correctly across all supported scenarios. They run the e2e-runner with the client detection test mode.

**Why this priority**: Ensures the core auto-detection functionality works correctly and catches regressions.

**Independent Test**: Run e2e-runner with client detection test mode, verify all detection scenarios pass.

**Acceptance Scenarios**:

1. **Given** e2e-runner in client detection test mode, **When** it sends requests with `X-Video-Codec: h265`, **Then** it verifies the response contains H.265 video
2. **Given** e2e-runner in client detection test mode, **When** it sends requests with VLC User-Agent, **Then** it verifies the response matches VLC's expected format
3. **Given** e2e-runner in client detection test mode, **When** it sends requests with both `X-Video-Codec: h264` and Safari User-Agent, **Then** it verifies H.264 is served (header takes precedence)
4. **Given** e2e-runner in client detection test mode, **When** it sends requests with invalid `X-Video-Codec: fake`, **Then** it verifies fallback to User-Agent detection occurs
5. **Given** e2e-runner testing a proxy with an encoding profile, **When** it sends requests with `X-Video-Codec: h264` but profile specifies H.265, **Then** it verifies H.265 is served (profile takes precedence)

---

### Bug Fix: Invalid Default Rule Expression

The current "Default (Universal)" fallback rule in the migration uses `Expression: "true"`. While the expression parser does support `true` as a keyword, there appears to be an issue where some database configurations store it as the integer `1` rather than the string `"true"`, causing:

```
level=WARN msg="Client detection rule issue detected" mapping_name="Default (Universal)"
issue_type=invalid_expression message="Failed to parse expression: parse error at line 1, column 1: expected field name but got 1"
```

The fix is to use the unambiguous fallback pattern `user_agent contains ""` (empty string match - always true for any User-Agent) which is already the recommended tautology pattern.

**Fix**:
1. Update the migration to use `Expression: `user_agent contains ""`  instead of `Expression: `true``
2. Add a data migration to fix existing databases that have the corrupted expression

---

### Edge Cases

- What happens when a profile specifies VP9 but client only supports MPEG-TS container? System serves VP9 in fMP4 regardless (user chose the profile explicitly)
- What happens when hardware acceleration is requested but unavailable? Falls back to software encoding with a log warning
- What happens when source stream is already in the target codec? Smart passthrough is used to avoid unnecessary transcoding
- How does system handle unknown User-Agent strings? Falls back to safe defaults (H.264/AAC in MPEG-TS for maximum compatibility)
- What happens when `X-Video-Codec` contains an invalid codec name? No rule matches, detection continues with next priority level
- What happens when `X-Video-Codec` requests a codec but `X-Audio-Codec` is not specified? Video codec is honored, audio codec determined by normal detection
- What happens when explicit codec headers conflict with an assigned encoding profile? Encoding profile takes precedence (explicit admin choice over client request)

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST support two proxy modes: `direct` (302 redirect) and `smart` (intelligent delivery)
- **FR-002**: System MUST automatically detect client capabilities from User-Agent, Accept headers, and X-Tvarr-Player header without user configuration
- **FR-003**: System MUST select optimal container format (HLS-fMP4, HLS-TS, MPEG-TS, DASH) based on detected client capabilities
- **FR-004**: System MUST passthrough source content without transcoding when client supports source codecs
- **FR-005**: System MUST transcode to client-compatible codecs when source is incompatible with client
- **FR-006**: System MUST allow users to create encoding profiles that specify target codecs
- **FR-007**: When an encoding profile is applied to a proxy, system MUST use that profile's target codec regardless of client capabilities
- **FR-008**: System MUST support quality presets (low, medium, high, ultra) that map to appropriate encoding parameters
- **FR-009**: System MUST automatically select appropriate container format based on codec requirements (e.g., VP9/AV1 require fMP4)
- **FR-010**: System MUST support hardware acceleration with auto-detection and automatic fallback to software encoding
- **FR-011**: System MUST provide automatic migration from existing relay profiles to simplified encoding profiles
- **FR-012**: System MUST immediately remove the RelayProfileMapping system (client detection is built-in) with no deprecation period
- **FR-013**: System MUST migrate any existing RelayProfileMapping settings to built-in detection rules during the database migration
- **FR-014**: System MUST reduce the encoding profile configuration fields by at least 75% compared to current relay profiles
- **FR-015**: When creating a new proxy, system MUST pre-select all available stream sources by default
- **FR-016**: When creating a new proxy, system MUST pre-select all system filters by default
- **FR-017**: When creating a new proxy, system MUST default to smart mode (not direct mode)
- **FR-018**: Client detection MUST use first-match-wins behavior, returning immediately on the first matching rule
- **FR-019**: System MUST include built-in detection rules for `X-Video-Codec` header using existing expression engine (`@header_req:X-Video-Codec`)
- **FR-020**: System MUST include built-in detection rules for `X-Audio-Codec` header using existing expression engine (`@header_req:X-Audio-Codec`)
- **FR-021**: Built-in codec header rules MUST have higher priority than User-Agent rules but lower priority than assigned encoding profiles
- **FR-022**: Only valid codec values (h264, h265, vp9, av1, aac, opus, ac3, eac3, mp3) MUST have matching rules; unrecognized values fall through to next rule
- **FR-023**: BUG FIX - The "Default (Universal)" fallback rule expression MUST be changed from `true` to `user_agent contains ""` in the migration code
- **FR-024**: BUG FIX - A data migration MUST be added to fix existing databases where the expression was stored as `1` or `true` instead of `user_agent contains ""`
- **FR-025**: EPG source timezone detection MUST correctly parse and store the original timezone from XMLTV data (from programme start/stop attributes)
- **FR-026**: EPG source timezone detection MUST correctly parse and store the original timezone from Xtream API data
- **FR-027**: EPG times MUST be converted to UTC with the correct offset based on detected source timezone
- **FR-028**: System MUST log detected EPG source timezone as an INFO message when creating/updating an EPG source

### Testing Requirements

- **TR-001**: Unit tests MUST cover all `@header_req:` dynamic field resolver functionality including edge cases (missing headers, empty values, case sensitivity)
- **TR-002**: Unit tests MUST verify first-match-wins behavior in client detection rule evaluation
- **TR-003**: Unit tests MUST cover all built-in codec header rules (`X-Video-Codec`, `X-Audio-Codec`) for each valid codec value
- **TR-004**: Unit tests MUST verify rule priority ordering (explicit headers > format override > X-Tvarr-Player > Accept > User-Agent > default)
- **TR-005**: Integration tests MUST verify end-to-end client detection with various header combinations
- **TR-006**: Integration tests MUST verify encoding profile override takes precedence over client detection

### E2E Runner Requirements

The `cmd/e2e-runner` application MUST be updated to support testing the new client detection and relay system:

- **ER-001**: E2E runner MUST support sending custom headers (`X-Video-Codec`, `X-Audio-Codec`) with stream requests
- **ER-002**: E2E runner MUST include a "client detection test" mode that exercises:
  - Each built-in codec header rule (X-Video-Codec with h264, h265, vp9, av1)
  - Each built-in audio codec header rule (X-Audio-Codec with aac, opus, ac3)
  - User-Agent detection for common clients (VLC, Chrome, Safari, Firefox, Kodi, IPTV clients)
  - Priority ordering verification (header > User-Agent)
  - Invalid codec header handling (fallthrough behavior)
- **ER-003**: E2E runner MUST verify the actual served content matches the expected codec based on detection rules
- **ER-004**: E2E runner MUST support testing with simplified encoding profiles (not just legacy relay profiles)
- **ER-005**: E2E runner MUST report detection rule match information in test output for debugging
- **ER-006**: E2E runner MUST include test scenarios for:
  - Zero-config proxy (smart mode, no profile) with multiple simulated client types
  - Explicit encoding profile override ignoring client headers
  - Direct mode bypass verification

### Key Entities

- **StreamProxy**: Represents a proxy configuration with mode (direct/smart) and optional encoding profile reference
- **EncodingProfile**: Simplified profile containing target codecs, quality preset, and hardware acceleration settings. Replaces the complex RelayProfile model.
- **ClientCapabilities**: Internal representation of detected client information including supported codecs and preferred formats (not user-configurable)

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: New users can create a working multi-device proxy in 2-3 clicks (name + save) with all sources and filters pre-selected
- **SC-002**: The encoding profile entity has at least 75% fewer fields than the current relay profile (from 40+ to under 10)
- **SC-003**: 95% of common client types (Chrome, Safari, Firefox, VLC, Kodi, IPTV clients) receive playable content without manual configuration
- **SC-004**: Existing relay profile configurations are automatically migrated to new encoding profiles without data loss
- **SC-005**: System handles client detection and routing decisions in under 10ms per request
- **SC-006**: User-facing documentation for the streaming configuration system is at least 50% shorter than current documentation

## Assumptions

1. The existing client detection logic (User-Agent patterns, X-Tvarr-Player header) covers the vast majority of common use cases without needing user customization
2. Users who need fine-grained FFmpeg control are rare edge cases and can use a custom command escape hatch
3. Quality presets (low/medium/high/ultra) adequately represent the needs of 90%+ of users
4. Immediate removal of RelayProfileMapping (no deprecation period) is acceptable because built-in auto-detection provides equivalent or better functionality, and any custom mappings can be converted to built-in rules during migration
5. The migration from old relay profiles to new encoding profiles can be done automatically based on existing settings by mapping target codecs and inferring quality presets from bitrate settings
