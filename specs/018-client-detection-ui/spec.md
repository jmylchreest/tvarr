# Feature Specification: Client Detection UI Improvements

**Feature Branch**: `018-client-detection-ui`
**Created**: 2025-12-15
**Status**: Draft
**Input**: User description: "Improve the client detection UI. Export/import is currently broken giving a 405 not allowed when clicking export. I cannot copy/paste the expression from the main list but I would like to. The UI in the expression editor does not pop up a helper intellisense completion UI (like the others do) and does not share the same badge like visual/tooltip solution as the others - which I'd like it to. The intellisense completion should show context specific helpers, in this case @dynamic() and it should provide suggested maps/keys, ie: request.headers & user-agent. I'd like a default system rule for VLC and MPV (are there any other very popular players?) which supports all the formats mpv & vlc would normally support across desktop + mobile."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Fix Export/Import Functionality (Priority: P1)

A user wants to back up or share their configuration (filters, data mapping rules, or client detection rules). When they click the Export button on any of these pages, they expect to select items and download them as a JSON file. Currently, this fails with a 405 error across all config types due to a URL mismatch between frontend and backend.

**Why this priority**: Export/import is broken functionality across all config types, preventing users from backing up their configurations - a critical data management feature.

**Independent Test**: Can be tested by clicking the Export button on any config page (filters, data mapping, client detection), selecting items, and verifying a JSON file downloads successfully.

**Acceptance Scenarios**:

1. **Given** the user has filters configured, **When** they click "Export" and select filters to export, **Then** a JSON file containing those filters downloads without error.
2. **Given** the user has data mapping rules configured, **When** they click "Export" and select rules to export, **Then** a JSON file containing those rules downloads without error.
3. **Given** the user has client detection rules configured, **When** they click "Export" and select rules to export, **Then** a JSON file containing those rules downloads without error.
4. **Given** the user has an exported config JSON file (any type), **When** they click "Import" and upload the file, **Then** the items are previewed and can be imported successfully.

---

### User Story 2 - Copyable Expressions in List View (Priority: P2)

A user viewing the main client detection rules list wants to quickly copy an expression to use elsewhere (e.g., documentation, troubleshooting, or duplicating with modifications). Currently, the expression text in the list cannot be easily selected and copied.

**Why this priority**: Improves usability and workflow efficiency for administrators managing multiple rules.

**Independent Test**: Can be tested by attempting to select and copy expression text from the rules list.

**Acceptance Scenarios**:

1. **Given** a user is viewing the client detection rules list, **When** they click on or interact with an expression, **Then** they can copy the full expression text to their clipboard.
2. **Given** an expression is displayed in the list, **When** the user right-clicks or uses a copy action, **Then** the expression is copied without truncation or formatting issues.

---

### User Story 3 - Intellisense/Autocomplete in Expression Editor (Priority: P2)

A user creating or editing a client detection rule wants assistance with the expression syntax. They expect the same autocomplete/intellisense experience available in filter and data-mapping expression editors, including helper suggestions for `@dynamic()` and context-specific field suggestions like `request.headers` and `user-agent`.

**Why this priority**: Consistency across expression editors improves user experience and reduces errors.

**Independent Test**: Can be tested by typing "@" in the expression editor and verifying helper suggestions appear.

**Acceptance Scenarios**:

1. **Given** a user is editing a client detection rule expression, **When** they type "@", **Then** an autocomplete popup appears showing available helpers including `@dynamic()`.
2. **Given** a user has typed "@dynamic(", **When** they continue typing, **Then** suggestions appear for valid completion options like `request.headers` with sub-suggestions for header keys like `user-agent`.
3. **Given** autocomplete suggestions are displayed, **When** the user presses Tab, Enter, or clicks a suggestion, **Then** the suggestion is inserted at the cursor position.

---

### User Story 4 - Validation Badges in Expression Editor (Priority: P2)

A user expects visual feedback about expression validity in the client detection rule editor, matching the badge-based validation display used in filter and data-mapping editors.

**Why this priority**: Visual consistency across the application and clear feedback on expression validity.

**Independent Test**: Can be tested by entering valid and invalid expressions and observing badge feedback.

**Acceptance Scenarios**:

1. **Given** a user enters a valid expression, **When** validation completes, **Then** validation badges show success states for syntax, fields, operators, and values.
2. **Given** a user enters an invalid expression, **When** validation completes, **Then** badges indicate which category (syntax, field, operator, value) has errors with tooltips explaining the issue.

---

### User Story 5 - Default System Rules for Popular Media Players (Priority: P3)

An administrator wants pre-configured rules for popular media players and media servers that automatically detect these clients and configure appropriate codec/format support.

**Why this priority**: Provides immediate value for common use cases, reducing manual configuration work.

**Rule categories**:
- **Direct players** (VLC, MPV, Kodi): Configure based on each player's known codec capabilities
- **Media servers** (Plex, Jellyfin, Emby): Configure for maximum compatibility/passthrough to avoid tvarr transcoding - these servers handle their own transcoding to end clients

**Independent Test**: Can be tested by checking for system rules after installation and verifying they match expected player signatures.

**Acceptance Scenarios**:

1. **Given** a fresh installation or database migration, **When** the user views client detection rules, **Then** system rules exist for VLC, MPV, Kodi, Plex, Jellyfin, and Emby with appropriate configurations.
2. **Given** a VLC media player connects to a stream, **When** client detection evaluates rules, **Then** the VLC system rule matches and applies appropriate settings (supports h264, h265, aac, ac3, mp3, etc.).
3. **Given** an MPV media player connects to a stream, **When** client detection evaluates rules, **Then** the MPV system rule matches and applies appropriate settings (supports wide codec range including av1, vp9, opus, eac3).
4. **Given** a Kodi media player connects to a stream, **When** client detection evaluates rules, **Then** the Kodi system rule matches and applies appropriate settings.
5. **Given** a Plex/Jellyfin/Emby media server connects to a stream, **When** client detection evaluates rules, **Then** the system rule matches and configures passthrough/maximum compatibility to avoid unnecessary transcoding.
6. **Given** system rules exist, **When** a user views or edits them, **Then** they are marked as "System" and cannot be deleted (only disabled).

---

### User Story 6 - Smart Remuxing vs Transcoding (Priority: P2)

When a client detection rule specifies an output format (e.g., HLS) that differs from the source container (e.g., MPEG-TS), the system should determine whether the source codecs are compatible with the target container. If compatible, the system should remux (repackage) rather than transcode, avoiding unnecessary CPU usage and quality loss.

**Why this priority**: Transcoding when remuxing would suffice wastes resources and can introduce quality degradation. This is a correctness issue in the relay logic.

**Independent Test**: Can be tested by configuring a client rule with HLS output, playing an MPEG-TS source with HEVC/EAC3 codecs, and verifying FFmpeg uses copy mode (`-c:v copy -c:a copy`) rather than transcoding.

**Acceptance Scenarios**:

1. **Given** a source stream is MPEG-TS with HEVC video and EAC3 audio, **When** a client detection rule requests HLS output, **Then** the relay remuxes (copies codecs) to HLS without transcoding.
2. **Given** a source stream has codecs incompatible with the target container, **When** a client detection rule requests that format, **Then** the relay transcodes only the incompatible codec(s).
3. **Given** a source stream matches the requested output format exactly, **When** the relay processes the stream, **Then** passthrough is used with no remuxing or transcoding.
4. **Given** remuxing is used instead of transcoding, **When** monitoring FFmpeg logs, **Then** the logs show `copy` for compatible codecs rather than encoder names.

---

### User Story 7 - Fuzzy Search in Channel and EPG Browsers (Priority: P2)

A user browsing channels or EPG programs wants to quickly find items using partial or approximate search terms. The current search may be limited in scope or require exact matches. Users expect fuzzy matching that searches across multiple fields to find relevant results even with typos or partial input.

**Why this priority**: Channel and EPG discovery are core user workflows; improved search significantly enhances usability for large lists.

**Independent Test**: Can be tested by searching for partial names, misspelled terms, or values from different fields and verifying relevant results appear.

**Acceptance Scenarios (Channels)**:

1. **Given** a user is in the channel browser, **When** they type a partial channel name (e.g., "BBC" for "BBC One HD"), **Then** channels with matching names appear in results.
2. **Given** a user searches with a slight typo (e.g., "Desicovery" instead of "Discovery"), **Then** fuzzy matching returns relevant channels despite the typo.
3. **Given** a user searches by tvg_id value, **When** results are returned, **Then** channels matching that tvg_id appear even if the name doesn't match the search term.
4. **Given** a user searches by group name, **When** results are returned, **Then** channels in matching groups appear in results.
5. **Given** a user searches by channel number (tvg_chno) or external ID (ext_id), **When** results are returned, **Then** channels with matching values appear.

**Acceptance Scenarios (EPG)**:

6. **Given** a user is in the EPG browser, **When** they type a partial program title (e.g., "News" for "BBC News at Ten"), **Then** programs with matching titles appear in results.
7. **Given** a user searches for a program with a slight typo, **Then** fuzzy matching returns relevant programs despite the typo.
8. **Given** a user searches by channel_id, category, or description keywords, **When** results are returned, **Then** programs matching those fields appear.

**Common Acceptance Scenarios**:

9. **Given** search results are displayed, **When** viewing results, **Then** the matched field/reason is indicated to help users understand why each result matched.

---

### Edge Cases

- What happens when the user tries to export with no rules selected? (Should show validation message)
- What happens when importing a file with conflicting rule names? (Should show preview with conflict resolution options)
- What happens when autocomplete API is slow or fails? (Should show loading state and gracefully handle errors)
- What happens when a system rule's User-Agent pattern becomes outdated? (User can disable and create custom rule)
- What happens when only the video codec is compatible but audio is not? (Should copy video, transcode audio only)
- What happens when codec detection fails or is unknown? (Should fall back to transcoding for safety)
- What happens when fuzzy search returns too many results? (Should rank by relevance/match quality)
- What happens when searching with very short terms (1-2 characters)? (Should require minimum length or show warning)

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST fix the export/import endpoint URL mismatch across all config types (filters, data mapping rules, client detection rules) - frontend calls `/{type}/export` but backend exposes `/api/v1/export/{type}` and `/api/v1/import/{type}`
- **FR-002**: System MUST allow users to select and copy expression text from the client detection rules list
- **FR-003**: System MUST display autocomplete popup when user types "@" in client detection expression editor
- **FR-004**: System MUST provide `@dynamic()` helper suggestion for client detection expressions with completion options for `request.headers` and header key suggestions
- **FR-005**: System MUST display validation badges (syntax, field, operator, value) in client detection expression editor matching filter/data-mapping editors
- **FR-006**: System MUST include default system rules for direct players (VLC, MPV, Kodi) and media servers (Plex, Jellyfin, Emby) on new installations
- **FR-007**: System rules MUST be marked as "is_system: true" and cannot be deleted by users
- **FR-008**: Import functionality MUST work with exported client detection rule files
- **FR-009**: Relay MUST remux (copy codecs) when source codecs are compatible with target container format, rather than transcoding
- **FR-010**: Relay MUST only transcode codecs that are incompatible with the target container, copying compatible codecs
- **FR-011**: Channel browser search MUST support fuzzy matching for approximate/partial term matching
- **FR-012**: Channel browser search MUST search across multiple fields: name, tvg_name, tvg_id, group, tvg_chno, and ext_id
- **FR-013**: EPG browser search MUST support fuzzy matching for approximate/partial term matching
- **FR-014**: EPG browser search MUST search across multiple fields: title, channel_id, category, and description
- **FR-015**: Search results (channels and EPG) SHOULD indicate which field matched the search term

### Key Entities *(include if feature involves data)*

- **ClientDetectionRule**: Rule definition with expression, priority, codec settings, and system flag
- **ExpressionHelper**: Configuration for `@dynamic()` helper including completion options and suggestions
- **SystemRule**: Pre-configured client detection rule shipped with the application (VLC, MPV, Kodi, Plex, Jellyfin, Emby)
- **ContainerCodecCompatibility**: Mapping of which codecs are supported by which container formats (HLS, MPEG-TS, etc.)
- **ChannelSearchResult**: Search result with matched channel and indication of which field(s) matched
- **EpgSearchResult**: Search result with matched EPG program and indication of which field(s) matched

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Export and import operations complete successfully without HTTP errors for all config types (filters, data mapping rules, client detection rules)
- **SC-002**: Users can copy expression text from the rules list in under 2 seconds (single click or right-click action)
- **SC-003**: Autocomplete suggestions appear within 500ms of typing "@" in the expression editor
- **SC-004**: Validation badges update within 1 second of expression changes
- **SC-005**: At least 6 system rules (VLC, MPV, Kodi, Plex, Jellyfin, Emby) are present after fresh installation
- **SC-006**: System rules correctly match their respective media player User-Agent strings in testing
- **SC-007**: HEVC/EAC3 source stream repackaged to HLS uses codec copy mode (no transcoding) as verified in FFmpeg logs
- **SC-008**: Relay CPU usage for compatible remuxing operations is significantly lower than equivalent transcoding operations
- **SC-009**: Channel search returns relevant results for partial matches (e.g., "BBC" finds "BBC One HD") within 500ms
- **SC-010**: Channel search returns relevant results for typos with edit distance of 1-2 characters
- **SC-011**: Channel search matches across all specified fields (name, tvg_name, tvg_id, group, tvg_chno, ext_id)
- **SC-012**: EPG search returns relevant results for partial matches within 500ms
- **SC-013**: EPG search matches across all specified fields (title, channel_id, category, description)

## Assumptions

- The expression editor infrastructure (ExpressionEditor, AutocompletePopup, ValidationBadges) used in filters and data-mapping can be reused for client detection with appropriate field/helper configuration
- VLC, MPV, and Kodi are the primary direct media players for IPTV streaming
- Media servers (Plex, Jellyfin, Emby) handle their own transcoding to end clients, so their rules should configure passthrough/max compatibility to avoid double-transcoding
- The backend validation endpoint for client detection expressions already exists (test endpoint at `/api/v1/client-detection-rules/test`)
- Default codec/format configurations for each player are based on their documented capabilities as of 2024
- HLS container supports HEVC (H.265), H.264, AAC, AC3, EAC3, and MP3 codecs without transcoding
- MPEG-TS container supports the same codec set as HLS for practical purposes

## Out of Scope

- Adding new codec types beyond what the system currently supports
- Changes to the client detection matching logic/algorithm
- Analytics or tracking of which rules are most commonly matched
- Automatic updates to system rules when player capabilities change
