# Feature Specification: Fix EPG Timezone Normalization and Canvas Layout

**Feature Branch**: `017-fix-epg-timezone-canvas`
**Created**: 2025-12-14
**Status**: Draft
**Input**: User description: "The EPG handling is broken in a few ways. We detect the EPG from upstream, but the EPG viewer is an hour behind, that is, the red bar that shows what's currently playing has a program behind it which played 1 hour before (with a +1 detected EPG). Setting the offset to +1 doesn't do anything, but I specifically want to adjust the programs to normalise them all to UTC, so an upstream timezone of +1, would move all program start/end times by -1, to normalise them to UTC. I want the timeshift to adjust that time further, before being written into the database. Additionally the EPG canvas is often too wide, and doesn't resize if I resize the viewport, it also overlaps the left hand sidebar."

## Problem Statement

The EPG system has several issues that need to be addressed:

1. **Timezone Handling**: The current time indicator ("red bar") shows programs that aired an hour ago as currently playing when the EPG source has a +1 timezone offset. The timezone normalization to UTC is not working correctly during ingestion, and the timeshift adjustment needs to be applied properly before database storage.

2. **Canvas Layout**: The EPG canvas does not respond to viewport resizing, remains too wide, and overlaps the left navigation sidebar.

3. **Time-Column Mapping**: The canvas uses a fixed pixels-per-hour constant that does not adapt during resize. Programs must map to time columns accurately, where each minute corresponds to a proportional column width, hour boundaries are clearly marked, and this mapping must be preserved during viewport scaling.

4. **UI Simplification**: The timezone selector dropdown in the EPG filter bar is unnecessary and should be removed since all times are normalized to UTC and displayed in the user's local timezone.

5. **Data Loading**: The current 12-hour lookahead is limiting; the system should lazy load additional EPG data when the user scrolls toward the end of the visible time range.

6. **Search Functionality**: The search should be comprehensive, searching across all visible fields including channel name, tvg_id, program title, and program description.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Accurate Current Program Display (Priority: P1)

As a user viewing the EPG guide, I need the "currently playing" indicator to accurately show what is playing right now, regardless of the upstream EPG source's timezone.

**Why this priority**: This is the core functionality - users cannot trust the guide if the current time indicator is wrong. An hour offset makes the entire EPG unusable for its primary purpose of showing what's on now.

**Independent Test**: Load the EPG viewer with a source that has a detected +1 timezone offset. The red "now" indicator should align with programs that are actually airing at this moment, not programs from an hour ago.

**Acceptance Scenarios**:

1. **Given** an EPG source with detected timezone of +01:00, **When** programs are ingested, **Then** all program start/end times are adjusted by -1 hour to normalize to UTC before storage.

2. **Given** an EPG source with detected timezone of -05:00, **When** programs are ingested, **Then** all program start/end times are adjusted by +5 hours to normalize to UTC before storage.

3. **Given** programs stored in UTC in the database, **When** the user views the EPG with the "now" indicator, **Then** the red bar aligns with programs currently airing based on actual current UTC time.

4. **Given** an EPG source with detected timezone +02:00 and a user-configured timeshift of +1 hour, **When** programs are ingested, **Then** times are first normalized to UTC (-2 hours) then timeshift applied (+1 hour), resulting in -1 hour total adjustment.

---

### User Story 2 - Responsive Canvas with Accurate Time-Column Mapping (Priority: P1)

As a user viewing the EPG on different screen sizes or resizing my browser window, I need the EPG canvas to fit within the available viewport while maintaining accurate time-to-column mapping where programs align precisely to their scheduled times.

**Why this priority**: The canvas must accurately represent time. Programs must be positioned and sized proportionally to their actual duration. A 15-minute program should consume exactly 1/4 of an hour column. Hour boundaries must be clearly visible. This mapping must remain accurate during resize operations.

**Independent Test**: View a program that starts at 14:00 and ends at 14:30. Regardless of viewport width, the program should start exactly at the 14:00 hour boundary and extend exactly halfway to the 15:00 boundary. Resizing the window should scale this proportionally without breaking the time alignment.

**Acceptance Scenarios**:

1. **Given** the EPG viewer is open, **When** a program runs from 14:00-14:30, **Then** it visually spans exactly half the distance between the 14:00 and 15:00 hour markers.

2. **Given** the EPG viewer is open, **When** a program runs from 14:15-14:45, **Then** it starts 1/4 into the 14:00 hour column and ends 3/4 into the 14:00 hour column (crossing no hour boundary visually, positioned correctly within the time grid).

3. **Given** the EPG viewer is open, **When** the user resizes the browser window, **Then** the time-to-pixel ratio recalculates proportionally and all programs maintain their correct time positions relative to hour boundaries.

4. **Given** the EPG viewer displays 6 hours of content, **When** the viewport width changes, **Then** the pixels-per-hour adapts to fit the available width while maintaining minimum readability thresholds.

5. **Given** the user scrolls horizontally, **When** they resize the viewport, **Then** the scroll position is adjusted to maintain the same time position in view (e.g., if 15:00 was centered, it remains approximately centered after resize).

---

### User Story 3 - Canvas Viewport Boundaries (Priority: P1)

As a user viewing the EPG, I need the canvas to stay within its designated area without overlapping the navigation sidebar.

**Why this priority**: The canvas overlapping the sidebar makes navigation impossible and renders the UI broken.

**Independent Test**: Open the EPG viewer with the navigation sidebar visible. The canvas should never extend under or overlap the sidebar at any viewport size.

**Acceptance Scenarios**:

1. **Given** the EPG viewer is open with the navigation sidebar visible, **When** the canvas renders, **Then** the canvas does not overlap or extend under the sidebar.

2. **Given** a narrow viewport (e.g., tablet width), **When** viewing the EPG, **Then** the canvas remains fully visible and scrollable within its container.

3. **Given** the EPG canvas width, **When** calculated, **Then** it equals the viewport width minus the sidebar width minus any padding/margins.

---

### User Story 4 - Infinite Scroll / Lazy Loading EPG Data (Priority: P2)

As a user browsing the EPG guide, I want to scroll further into the future without hitting a hard 12-hour limit, with additional program data loading automatically as I scroll.

**Why this priority**: Users often want to see what's on later in the day or tomorrow. The 12-hour limit is arbitrary and forces users to wait or refresh to see future programs.

**Independent Test**: Scroll the EPG horizontally toward the right edge. When approaching the end of loaded data, additional hours of programming should load seamlessly without requiring manual refresh.

**Acceptance Scenarios**:

1. **Given** the EPG is displaying programs up to 12 hours ahead, **When** the user scrolls to within 2 hours of the end, **Then** the system automatically fetches the next batch of program data.

2. **Given** additional program data is being loaded, **When** the fetch completes, **Then** the new programs appear seamlessly without disrupting the user's scroll position.

3. **Given** the user has scrolled far into the future, **When** they scroll back toward "now", **Then** all previously loaded data remains available without re-fetching.

4. **Given** no additional EPG data exists beyond a certain point, **When** the user scrolls to that boundary, **Then** the system does not repeatedly attempt to fetch unavailable data.

---

### User Story 5 - Comprehensive EPG Search (Priority: P2)

As a user searching for content in the EPG, I want my search to find matches across all relevant fields including channel names, channel IDs, program titles, and program descriptions.

**Why this priority**: Users search for content in different ways - by channel name, by show title, or by description keywords. Limited search frustrates users who can't find content they know exists.

**Independent Test**: Search for a term that appears only in a program description. The search should return that program and its channel.

**Acceptance Scenarios**:

1. **Given** a search term that matches a channel name, **When** the user searches, **Then** all programs on that channel are displayed.

2. **Given** a search term that matches a program title, **When** the user searches, **Then** channels with matching programs are displayed with those programs highlighted.

3. **Given** a search term that matches text in a program description, **When** the user searches, **Then** channels with matching programs are displayed.

4. **Given** a search term that matches a tvg_id, **When** the user searches, **Then** the corresponding channel and its programs are displayed.

5. **Given** a search term with no matches in any field, **When** the user searches, **Then** a "no results" message is displayed.

---

### User Story 6 - Manual Timeshift Adjustment (Priority: P3)

As an administrator configuring an EPG source, I need to apply a manual timeshift adjustment that modifies program times after timezone normalization, allowing me to correct for EPG providers whose schedules are offset from their claimed timezone.

**Why this priority**: Some EPG providers have incorrect timezone metadata or broadcast schedules that don't match their stated timezone. Manual adjustment allows users to correct these edge cases.

**Independent Test**: Configure an EPG source with a +1 hour timeshift. After re-ingesting, verify that all programs are shifted 1 hour later than they would be with timezone normalization alone.

**Acceptance Scenarios**:

1. **Given** an EPG source with timeshift set to +2 hours, **When** programs are ingested, **Then** all program times are shifted 2 hours later (after UTC normalization).

2. **Given** an EPG source with timeshift set to -1 hour, **When** programs are ingested, **Then** all program times are shifted 1 hour earlier (after UTC normalization).

3. **Given** an EPG source with timeshift of 0, **When** programs are ingested, **Then** program times are only affected by UTC normalization, no additional shift applied.

---

### User Story 7 - Simplified UI (Priority: P3)

As a user viewing the EPG, I should not see unnecessary controls that add confusion. The timezone selector dropdown should be removed since times are normalized and displayed in my local timezone automatically.

**Why this priority**: UI simplification reduces cognitive load. The timezone dropdown serves no practical purpose when times are already normalized to UTC and converted to local time for display.

**Independent Test**: Open the EPG viewer and verify the timezone dropdown is no longer present in the filter bar.

**Acceptance Scenarios**:

1. **Given** the EPG viewer is open, **When** the user views the filter bar, **Then** there is no timezone selector dropdown.

2. **Given** times are stored in UTC, **When** displaying program times, **Then** times are shown in the user's browser/system timezone automatically.

---

### Edge Cases

- What happens when an EPG source has no detectable timezone? Assume UTC (no adjustment needed for normalization).
- What happens when timezone detection changes between ingestion runs? Re-normalize all programs using the newly detected timezone.
- What happens when the viewport is extremely narrow (< 400px)? Canvas should have a minimum usable width with horizontal scrolling enabled.
- What happens when programs span midnight across timezone boundaries? Times should convert correctly without creating invalid time ranges.
- What happens when the browser window is resized during canvas animation/rendering? Resize should be debounced to prevent rendering glitches.
- What happens when lazy loading fails due to network error? Display a subtle error indicator and allow manual retry.
- What happens when searching with special characters or very long strings? Sanitize input and handle gracefully.
- What happens when the user scrolls very rapidly past multiple load thresholds? Batch or throttle fetch requests to avoid API flooding.
- What happens with very short programs (< 5 minutes)? Enforce minimum visible width with tooltip showing full details.
- What happens with very long programs (> 6 hours)? Display correctly spanning multiple hour columns.
- What happens when viewport shrinks below minimum readable pixels-per-hour? Enable horizontal scrolling rather than compressing time scale below readability threshold.

## Requirements *(mandatory)*

### Functional Requirements

#### Timezone Normalization (Ingestion)

- **FR-001**: System MUST detect the timezone from upstream EPG data (XMLTV timezone offset or Xtream server timezone).
- **FR-002**: System MUST normalize all program start and end times to UTC during ingestion by applying the inverse of the detected timezone offset.
- **FR-003**: System MUST apply the user-configured timeshift adjustment AFTER UTC normalization, before storing programs in the database.
- **FR-004**: System MUST store the detected timezone on the EPG source record for user reference.
- **FR-005**: System MUST treat EPG sources with no detectable timezone as UTC (no normalization adjustment).
- **FR-006**: System MUST re-normalize existing programs when the detected timezone changes on subsequent ingestion runs.

#### Current Time Indicator

- **FR-007**: System MUST calculate the "now" indicator position using the current UTC time.
- **FR-008**: System MUST display the "now" indicator aligned with programs whose UTC start/end times encompass the current UTC moment.
- **FR-009**: System MUST update the "now" indicator position at least every 30 seconds to maintain accuracy.
- **FR-010**: System MUST recalculate the "now" indicator position using the current time-to-pixel ratio after any resize event.

#### Time-Column Mapping Architecture

- **FR-011**: System MUST calculate time-to-pixel ratio dynamically based on available viewport width and the number of hours to display.
- **FR-012**: System MUST position programs horizontally based on their start time using the formula: `position = (start_time_offset_in_minutes / 60) * pixels_per_hour`.
- **FR-013**: System MUST size program widths based on their duration using the formula: `width = (duration_in_minutes / 60) * pixels_per_hour`.
- **FR-014**: System MUST render hour boundary markers at exact pixel positions calculated as `hour_index * pixels_per_hour`.
- **FR-015**: System MUST recalculate the pixels-per-hour value when the viewport is resized.
- **FR-016**: System MUST maintain a minimum pixels-per-hour threshold to ensure program text remains readable (minimum 50 pixels per hour).
- **FR-017**: System MUST enable horizontal scrolling when the required canvas width (hours * minimum_pixels_per_hour) exceeds available viewport width.
- **FR-018**: System MUST adjust scroll position proportionally during resize to maintain the same time position in view.
- **FR-019**: System MUST enforce a minimum program width to ensure very short programs remain visible and clickable.

#### Canvas Layout

- **FR-020**: System MUST constrain the EPG canvas width to fit within the available content area (viewport width minus sidebar width).
- **FR-021**: System MUST recalculate canvas dimensions when the viewport is resized.
- **FR-022**: System MUST NOT allow the canvas to overlap or extend under the navigation sidebar.
- **FR-023**: System MUST debounce resize events to prevent excessive re-rendering during continuous resize operations.

#### UI Simplification

- **FR-024**: System MUST NOT display a timezone selector dropdown in the EPG filter bar.
- **FR-025**: System MUST display all program times converted to the user's local browser/system timezone.

#### Lazy Loading / Infinite Scroll

- **FR-026**: System MUST detect when the user scrolls within a threshold (e.g., 2 hours) of the end of loaded EPG data.
- **FR-027**: System MUST automatically fetch additional EPG program data when the scroll threshold is reached.
- **FR-028**: System MUST append newly loaded programs to the existing data without disrupting scroll position.
- **FR-029**: System MUST track the furthest time boundary of available data to avoid redundant fetch attempts.
- **FR-030**: System MUST throttle or debounce fetch requests to prevent excessive API calls during rapid scrolling.
- **FR-031**: System MUST retain previously loaded data when scrolling backward (no re-fetching of already-loaded time ranges).

#### Comprehensive Search

- **FR-032**: System MUST search across channel name when filtering EPG results.
- **FR-033**: System MUST search across channel tvg_id when filtering EPG results.
- **FR-034**: System MUST search across program titles when filtering EPG results.
- **FR-035**: System MUST search across program descriptions when filtering EPG results.
- **FR-036**: System MUST perform case-insensitive search across all searchable fields.
- **FR-037**: System MUST display channels that have at least one matching program or matching channel metadata.
- **FR-038**: System MUST display appropriate feedback when no search results are found.

### Key Entities

- **EPG Source**: Configuration record containing URL, detected timezone (auto-populated), and timeshift (user-configurable hours adjustment).
- **EPG Program**: Individual program record with start time, end time (both stored as UTC), title, description, and channel association.
- **Detected Timezone**: The timezone offset or identifier extracted from upstream EPG data during ingestion.
- **Timeshift**: User-configurable hour offset applied after UTC normalization to correct for provider-specific timing issues.
- **Channel**: Record with name, tvg_id, logo, and association to EPG source.
- **Time-Pixel Ratio**: Calculated value representing pixels per hour, derived from available viewport width divided by hours to display, subject to minimum threshold.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The "now" indicator accurately aligns with currently-airing programs within 1 minute of actual current time, regardless of upstream EPG timezone.
- **SC-002**: Users can resize the browser window and the EPG canvas adjusts to fit within 500ms, without overlapping the sidebar.
- **SC-003**: EPG sources with +1 to +12 and -1 to -12 hour timezone offsets all display correctly normalized program times.
- **SC-004**: 100% of ingested programs have start/end times stored in UTC after normalization.
- **SC-005**: The EPG canvas functions correctly at viewport widths from 768px to 2560px without layout breakage.
- **SC-006**: Manual timeshift adjustments of -12 to +12 hours are correctly applied and reflected in displayed program times.
- **SC-007**: Users can scroll to view EPG data at least 48 hours into the future (or until data exhausted) via lazy loading.
- **SC-008**: Search returns results matching any of: channel name, tvg_id, program title, or program description within 500ms.
- **SC-009**: The EPG filter bar contains no timezone selector dropdown.
- **SC-010**: A program with 30-minute duration visually occupies exactly half the width of a 60-minute program at any viewport size.
- **SC-011**: Hour boundary markers remain aligned with actual hour times after viewport resize operations.
- **SC-012**: Scroll position maintains the same approximate time-in-view after resize (±5 minutes tolerance).

## Assumptions

- The detected timezone from upstream EPG sources is accurate and represents the timezone of the program times in the feed.
- Programs are always provided with absolute times (not relative offsets).
- The navigation sidebar has a fixed or known width that can be accounted for in canvas calculations.
- Browser support for ResizeObserver is available (modern browsers only).
- UTC is the canonical storage format for all times in the system.
- The backend API supports time-range queries for fetching EPG data in chunks.
- Program descriptions are stored and available for search (not truncated or omitted during ingestion).
- Time accuracy to the minute level is sufficient (sub-minute precision not required).

## Out of Scope

- Per-channel timezone adjustments (all channels from a source share the same timezone).
- DST (Daylight Saving Time) automatic transitions - timeshift is a fixed hour offset.
- Historical timezone data for past programs (normalization applies at ingestion time only).
- Mobile-specific responsive layouts (focus is on desktop/tablet viewport fixes).
- Full-text search indexing or advanced search operators (simple substring matching is sufficient).
- Lazy loading of past EPG data (scrolling left beyond "now" minus some buffer).
- User-configurable zoom levels (pixels-per-hour is calculated automatically, not user-adjustable).
- Sub-minute time precision in the visual grid.

## Technical Research Summary

Analysis of the current implementation revealed the root cause of scaling issues:

**Current Architecture (Problem)**:
- `PIXELS_PER_HOUR` is a hardcoded constant (200px)
- During resize, only canvas dimensions change
- Time-to-pixel mapping uses fixed constant: `(time_offset / 3600000) * 200`
- Result: Time mapping breaks on resize; programs don't scale with viewport

**Required Architecture (Solution)**:
- `pixels_per_hour` must be calculated dynamically: `(available_width - sidebar_width) / hours_to_display`
- All position/width calculations must use this dynamic value
- On resize: recalculate `pixels_per_hour`, then recalculate all program positions
- Maintain minimum threshold (e.g., 50px/hour) below which horizontal scrolling is enabled
- Scroll position must be converted to time, then back to pixels after resize to maintain view position

**Professional EPG implementations** (Kodi, TiVo, etc.) use time-based positioning with adaptive pixel ratios, ensuring programs always align accurately to their scheduled times regardless of display size.

## Web-Based EPG Solutions Research

Research into existing web-based EPG implementations revealed several approaches worth considering:

### Option 1: Planby (Recommended for Evaluation)

[Planby](https://planby.app) is the most mature React EPG library, trusted by 100+ companies.

**Key Technical Approach:**
- **Virtualization**: Renders only visible programs using a custom virtual view system
- **Time Positioning**: Uses `dayWidth` prop (default 7200px = 24 hours, so 300px/hour) to calculate program positions
- **Responsive**: Inherits parent container dimensions when width/height props omitted; recalculates all positioning on resize
- **Program Width**: Automatically calculated from `since`/`till` timestamps relative to `startDate`
- **API**: Uses `useEpg()` hook returning `getEpgProps()`, `getLayoutProps()`, scroll handlers, and position tracking

**How it solves our problems:**
- Hour width is derived from `dayWidth / 24`, not hardcoded
- Parent-relative sizing means canvas adapts to container on resize
- Virtualization handles large datasets (thousands of programs) efficiently
- Provides `onScrollToNow` for current time navigation

**Potential concerns:**
- DOM-based rendering (not Canvas) - may have different performance characteristics
- Would require significant refactoring to integrate
- Dependencies on their styling system

**Resources:**
- [GitHub](https://github.com/karolkozer/planby)
- [Documentation](https://dev.to/kozerkarol/electronic-program-guide-for-react-its-so-easy-with-planby-1oa9)

### Option 2: react-tv-epg (Canvas-Based)

[react-tv-epg](https://github.com/SatadruBhattacharjee/react-tv-epg) is a Canvas-based EPG similar to tvarr's current approach.

**Key Technical Approach:**
- **HTML5 Canvas**: Similar rendering approach to current tvarr implementation
- **Key-based navigation**: Designed for TV/set-top box navigation
- **Unlimited days**: Can display unlimited program guide data

**How it could help:**
- Canvas-based like our current solution, so performance characteristics should be similar
- Could study their time positioning and resize handling code
- MIT licensed, can borrow patterns

**Potential concerns:**
- Less actively maintained than Planby
- Less documentation available

### Option 3: Improve Current Canvas Implementation

Rather than replacing, fix the core architectural issue:

**Required Changes:**
1. Replace `const PIXELS_PER_HOUR = 200` with dynamic calculation
2. Create `useCanvasMetrics()` hook that recalculates on resize
3. Store scroll position as time, not pixels; convert on render
4. Add debounced resize handler that triggers full recalculation

**Advantages:**
- Preserves existing Canvas performance (proven to work well)
- Smaller change surface area
- No new dependencies

**Implementation Pattern (from research):**
```
// Calculate dynamically based on container
pixelsPerHour = (containerWidth - sidebarWidth) / hoursToDisplay

// Position formula (time-based, then converted to pixels)
programLeft = ((programStart - guideStart) / msPerHour) * pixelsPerHour
programWidth = (programDuration / msPerHour) * pixelsPerHour

// On resize: store current time at scroll position, recalculate metrics, restore scroll to same time
```

### Recommendation

**Start with Option 3** (improve current implementation) because:
1. The current Canvas approach performs well (user confirmed this)
2. The fix is architectural, not fundamental - changing constant to calculated value
3. Lower risk than replacing entire component
4. Can evaluate Planby later if needed for additional features

If Option 3 proves insufficient or overly complex, **evaluate Planby** as a replacement since it's the most mature solution with proven virtualization and responsive sizing.
