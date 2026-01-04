# Feature Specification: Manual Stream Sources & UI/UX Overhaul

**Feature Branch**: `019-manual-stream-sources`
**Created**: 2025-01-10
**Status**: Draft
**Scope**: Manual stream sources backend + Application-wide UI/UX redesign

## Executive Summary

This specification covers two interconnected concerns:

1. **Manual Stream Sources**: Backend API implementation to complete the partially-built manual channel feature, enabling users to define custom channels without external M3U/Xtream sources.

2. **UI/UX Overhaul**: A comprehensive redesign of the frontend editing patterns, data display, and interaction models across all pages. The manual channel editor serves as the catalyst for this broader redesign.

---

# Part 1: Manual Stream Sources

## Current Implementation Status

The feature has partial implementation:

### Already Implemented
- **Backend Model**: `ManualStreamChannel` model with all required fields (`internal/models/manual_stream_channel.go`)
- **Backend Repository**: CRUD operations for manual channels (`internal/repository/manual_channel_repo.go`)
- **Backend Ingestor**: `ManualHandler` materializes manual channels to the main channels table (`internal/ingestor/manual_handler.go`)
- **Frontend Editor**: `ManualChannelEditor` component - card-based editor with validation (`frontend/src/components/manual-channel-editor.tsx`)
- **Frontend Import/Export UI**: `ManualM3UImportExport` component - M3U paste/preview/download interface (`frontend/src/components/manual-m3u-import-export.tsx`)
- **Frontend Integration**: Stream sources form supports `source_type="manual"` (`frontend/src/components/stream-sources.tsx`)

### Missing (Backend API Layer)
- **API Endpoints**: The frontend expects these endpoints but they do not exist:
  - `GET /api/v1/sources/stream/{id}/manual-channels` - List manual channels
  - `PUT /api/v1/sources/stream/{id}/manual-channels` - Replace all manual channels
  - `POST /api/v1/sources/stream/{id}/manual-channels/import-m3u` - Parse/apply M3U
  - `GET /api/v1/sources/stream/{id}/manual-channels/export.m3u` - Export as M3U
- **Manual Channel Service**: Service layer for coordinating manual channel operations
- **M3U Parser**: Server-side M3U parsing for import functionality

## User Scenarios & Testing

### User Story 1 - Create Manual Source with Channels via Tabular Editor (Priority: P1)

A user wants to add their doorbell camera stream and a few local RTSP feeds to tvarr without having an M3U file.

**Why this priority**: This is the core MVP - users must be able to create and edit manual channels directly in the UI.

**Acceptance Scenarios**:

1. **Given** I am on the Stream Sources page, **When** I click "Add Source" and select type "Manual (Static)", **Then** I see a tabular channel editor instead of URL fields
2. **Given** I am creating a manual source, **When** I enter a channel name, stream URL, and logo reference, **Then** the row shows validation status inline
3. **Given** I have entered valid channel data, **When** I click "Save", **Then** the source is created and channels are materialized to the main channels table
4. **Given** a manual source exists, **When** I trigger ingestion, **Then** channels are refreshed from the manual definitions

---

### User Story 2 - Edit Existing Manual Channels (Priority: P1)

A user needs to update the stream URL for their doorbell camera after changing their network configuration.

**Why this priority**: Essential for ongoing maintenance of manual channels.

**Acceptance Scenarios**:

1. **Given** a manual source exists with channels, **When** I select it in the list, **Then** the existing channels load into the tabular editor in the detail panel
2. **Given** I am editing a manual source, **When** I modify a channel's stream URL and click "Save", **Then** the channel is updated (no separate "Apply Changes" step)
3. **Given** I am editing a manual source, **When** I delete a channel row and save, **Then** that channel is deleted from the system

---

### User Story 3 - Import Channels from M3U (Priority: P2)

A user has a small personal M3U file with a few streams and wants to import them into a manual source.

**Acceptance Scenarios**:

1. **Given** I am editing a manual source, **When** I click "Import M3U" and paste valid M3U content, **Then** I can preview the parsed channels
2. **Given** I have previewed imported channels, **When** I click "Apply", **Then** existing manual channels are replaced with imported ones
3. **Given** I paste invalid M3U content, **When** I click "Preview", **Then** I see an appropriate error message

---

### User Story 4 - Export Channels as M3U (Priority: P2)

A user wants to share their manual channel configuration or back it up as an M3U file.

**Acceptance Scenarios**:

1. **Given** a manual source exists with channels, **When** I click "Export M3U", **Then** I see the M3U content in a preview dialog
2. **Given** I am viewing the export dialog, **When** I click "Download", **Then** an M3U file is downloaded to my device
3. **Given** I am viewing the export dialog, **When** I click "Copy", **Then** the M3U content is copied to clipboard

---

### User Story 5 - Logo Integration via Data Mapping (Priority: P3)

A user wants channel logos to resolve using tvarr's logo service with data mapping tokens.

**Acceptance Scenarios**:

1. **Given** I am adding a manual channel, **When** I enter `@logo:doorbell` as the logo, **Then** the system validates it as a valid logo reference
2. **Given** I enter a full URL as the logo, **When** the channel is displayed, **Then** the logo loads from that URL
3. **Given** I leave the logo field empty, **When** the channel is displayed, **Then** a default placeholder is shown

---

### Edge Cases

- What happens when a user tries to create a manual source with zero channels? **System should require at least one valid channel**
- What happens when duplicate channel numbers are entered? **Validation should flag duplicates**
- What happens when the stream URL becomes unreachable? **Channel remains in list but will fail during streaming (handled by relay layer)**
- What happens when importing M3U with malformed EXTINF lines? **Parser should skip invalid entries and report count**

## Backend Requirements

### API Endpoints

- **FR-001**: System MUST implement `GET /api/v1/sources/stream/{id}/manual-channels` to list manual channel definitions
- **FR-002**: System MUST implement `PUT /api/v1/sources/stream/{id}/manual-channels` to replace all manual channels (full overwrite)
- **FR-003**: System MUST implement `POST /api/v1/sources/stream/{id}/manual-channels/import-m3u` with `?apply=true/false` query parameter
- **FR-004**: System MUST implement `GET /api/v1/sources/stream/{id}/manual-channels/export.m3u` returning M3U content
- **FR-005**: System MUST return 400 Bad Request if manual channel operations are attempted on non-manual sources

### M3U Import/Export

- **FR-006**: System MUST parse extended M3U format including `#EXTINF` attributes: `tvg-id`, `tvg-name`, `tvg-logo`, `group-title`, `tvg-chno`
- **FR-007**: System MUST generate valid extended M3U output with all channel metadata
- **FR-008**: Import preview (apply=false) MUST return parsed channels without persisting
- **FR-009**: Import apply (apply=true) MUST atomically replace existing channels and trigger materialization

### Validation

- **FR-010**: System MUST require `channel_name` (non-empty) for all manual channels
- **FR-011**: System MUST require `stream_url` starting with `http://`, `https://`, or `rtsp://` for all manual channels
- **FR-012**: System MUST validate `tvg_logo` as empty OR `@logo:*` token OR http(s):// URL
- **FR-013**: System MUST detect and flag duplicate `channel_number` values within a source

### Backend Implementation

1. **Create Handler**: `internal/http/handlers/manual_channel.go`
2. **Create Service**: `internal/service/manual_channel_service.go`
3. **Create M3U Parser**: `internal/m3u/parser.go` or integrate with existing m3u parser
4. **M3U Generator**: Generate valid M3U from manual channel list

---

# Part 2: UI/UX Design System Overhaul

## Current State Analysis

### Problems Identified

#### 1. Sheet Overload
Every entity uses slide-in sheets for create/edit:
- Sheets designed for **quick actions**, not complex multi-field forms
- `sm:max-w-2xl` (672px) is too narrow for complex forms
- Sheets inside sheets (manual channels inside stream sources) create poor UX
- Form scrolling inside already constrained panels

#### 2. View Mode Proliferation
Nearly every list has 3 view modes (Grid/List/Table):
- Significant code duplication across 10+ components
- Users rarely switch modes after initial setup
- Table view is almost always most efficient for data management
- Inconsistent implementation across components

#### 3. Card-Based Lists Inefficiency
- Each card ~120-150px height
- 10 items = 1200-1500px scrolling
- Poor information density vs. tables (~40px per row)
- Duplicate information display (badges, dates shown multiple times)

#### 4. Manual Channel Editor Issues
- **Two-stage save** ("Apply Changes" then "Save") - confusing
- **Card-per-row** instead of tabular despite user request
- Nested inside narrow sheet
- Excessive vertical space consumption

#### 5. Inconsistent Interaction Patterns
- Delete: Some use confirmation dialog, some instant
- Actions: Some hover-reveal, some always visible, some dropdown
- Reordering: Some drag handles, some arrow buttons, some priority fields

### Inventory of Current Patterns

| Component | Current Pattern | Issues |
|-----------|----------------|--------|
| Stream Sources | Sheet + Cards | Sheet too narrow |
| EPG Sources | Sheet + Cards | Sheet too narrow |
| Filters | Sheet + Cards | Expression editor good |
| Data Mapping | Sheet + Cards | Has priority reorder |
| Encoding Profiles | Sheet + Cards | Complex form in narrow sheet |
| Client Detection | Sheet + Table | Good table, bad sheet |
| Proxies | Sheet (large) | Most complex, needs wizard |
| Logos | Sheet + Gallery | Gallery good for images |
| Manual Channels | Cards nested in Sheet | **Worst UX** |
| Channels Page | Table/Grid/List | Too many view modes |
| EPG Page | Guide view | **Excellent** - keep |
| Settings | Inline cards | Works well |
| Logs | Virtual list | Works well |

---

## Design System Specification

### Core Principles

1. **Efficiency First**: Maximize information density without sacrificing clarity
2. **Consistent Patterns**: Same interaction patterns across all entities
3. **Progressive Disclosure**: Show essential info by default, details on demand
4. **Single Save**: No multi-stage save patterns
5. **Keyboard Accessible**: Full keyboard navigation support

---

### Pattern 1: Page Layouts

#### 1A. Master-Detail Layout (Primary Pattern)

Replace sheets with a two-pane layout for all entity management:

```
┌─────────────────────────────────────────────────────────────────┐
│ Stream Sources                    [+ Create] [Import] [Export]  │
├─────────────────────────────────────────────────────────────────│
│ [Search...________________] [Type ▾] [Status ▾]    [Refresh]    │
├────────────────────────────┬────────────────────────────────────│
│ ☐  Name           Type  ✓ │  Edit: BBC Streams                  │
│ ☑  BBC Streams    M3U   ● │  ────────────────────────────────── │
│ ☐  Sky UK         Xtrm  ● │  Name     [BBC Streams_________]    │
│ ☐  Manual Cams    Man.  ● │  Type     [M3U ▾]                   │
│ ☐  Sports Pack    Xtrm  ○ │  URL      [http://bbc.example___]   │
│                           │  Schedule [Every 6 hours ▾]         │
│                           │                                     │
│                           │  ┌─ Advanced ─────────────────────┐ │
│                           │  │ Max Concurrent: [5____]        │ │
│                           │  │ User-Agent:     [__________]   │ │
│                           │  └────────────────────────────────┘ │
│                           │                                     │
│                           │            [Cancel] [Save Changes]  │
├────────────────────────────┴────────────────────────────────────│
│ ☐ Select All    With selected: [Delete] [Enable] [Disable]      │
└─────────────────────────────────────────────────────────────────┘
```

**Use for**: Stream Sources, EPG Sources, Filters, Data Mapping, Client Detection, Encoding Profiles

**Key Features**:
- List panel (40% width) with compact table rows
- Detail panel (60% width) with full form
- Multi-select with bulk actions
- No modal/sheet juggling
- Full height forms (no internal scrolling)
- Selection persists while editing

#### 1B. Full-Width Table Layout

```
┌─────────────────────────────────────────────────────────────────┐
│ Channels                                    [Filters] [Refresh] │
├─────────────────────────────────────────────────────────────────│
│ [Search...________________] [Source ▾] [Group ▾] [Codec ▾]      │
├─────────────────────────────────────────────────────────────────│
│ Logo │ #   │ Name          │ Group    │ Source  │ Codec │ Act. │
├──────┼─────┼───────────────┼──────────┼─────────┼───────┼──────┤
│ 🖼️   │ 1   │ BBC One       │ UK News  │ BBC M3U │ H.264 │ ▶ 👁 │
│ 🖼️   │ 2   │ BBC Two       │ UK News  │ BBC M3U │ H.264 │ ▶ 👁 │
│ ...  │ ... │ ...           │ ...      │ ...     │ ...   │ ...  │
├─────────────────────────────────────────────────────────────────│
│ Showing 1-50 of 2,847          [◀ Prev] Page 1 of 57 [Next ▶]   │
└─────────────────────────────────────────────────────────────────┘
```

**Use for**: Channels (read-only list), Logs

#### 1C. TV Guide Layout (Keep As-Is)

The EPG canvas guide view is excellent - keep it.

#### 1D. Gallery Layout (Keep As-Is)

The Logos gallery view is appropriate for image assets - keep it with side panel for editing.

#### 1E. Wizard Layout (for complex entities)

```
┌────────────────────────────────────────────────────────────┐
│ Create Proxy                                               │
│ [1. Sources] [2. EPG] [3. Filters] [4. Settings]          │
├────────────────────────────────────────────────────────────│
│                                                            │
│  Select stream sources to include:                         │
│                                                            │
│  ☑ BBC Streams (M3U) - 847 channels                       │
│  ☑ Sky UK (Xtream) - 1,203 channels                       │
│  ☐ Sports Pack (Xtream) - 156 channels                    │
│                                                            │
│  [Select All] [Select None]                                │
│                                                            │
│                         [◀ Back] [Next: EPG Sources ▶]     │
└────────────────────────────────────────────────────────────┘
```

**Use for**: Proxy creation (multi-step with source/EPG/filter selection)

---

### Pattern 2: Inline Data Tables

Replace card-per-row patterns with inline editable tables:

```
┌──────┬────────────────┬──────────────────────────────┬─────────┐
│  #   │ Name           │ Stream URL                   │         │
├──────┼────────────────┼──────────────────────────────┼─────────┤
│ [1 ] │ [Doorbell____] │ [rtsp://192.168.1.50/stream] │ [🗑️]   │
│ [2 ] │ [Backyard____] │ [http://cam.local/feed.m3u8] │ [🗑️]   │
│ [  ] │ [____________] │ [____________________________] │ [+]    │
└──────┴────────────────┴──────────────────────────────┴─────────┘
[Columns ▾] [Import M3U ▾] [Export M3U]
```

**Use for**: Manual channels, Proxy source assignments, Filter assignments

**Key Features**:
- Inline editing (no separate edit mode)
- Tab to move between cells
- Enter to confirm, Escape to cancel
- Last empty row auto-adds on input
- Column visibility toggle
- **No "Apply Changes"** - changes tracked in form state, saved with parent
- Validation shown inline (red border + tooltip)

---

### Pattern 3: Forms

#### Simple Forms (≤6 fields): Single column
#### Medium Forms (7-12 fields): Two columns with collapsible sections
#### Complex Forms (>12 fields): Tabbed or wizard

#### Form Validation
- Validate on blur (not on every keystroke)
- Show validation state: neutral → valid (green checkmark) → invalid (red border)
- Error messages below field
- Disable submit until form is valid
- Show summary of errors at top if multiple

---

### Pattern 4: Actions & Confirmations

#### Action Button Placement
- **Primary action**: Top right of page header (`[+ Create]`)
- **Bulk actions**: Bottom of list when items selected
- **Row actions**: Dropdown menu (⋮) at end of row
- **Form actions**: Bottom right of form (`[Cancel] [Save]`)

#### Confirmation Patterns
- **Destructive actions**: Always confirm with dialog
- **Bulk destructive**: Confirm with count ("Delete 5 items?")
- **Non-destructive**: No confirmation needed

---

### Pattern 5: Empty States

Every list should have a helpful empty state with icon, message, and action button.

---

### Pattern 6: Loading States

- **Table loading**: Skeleton rows matching expected content shape
- **Button loading**: Spinner replaces icon, text stays
- **Form submitting**: Disable all inputs, show spinner on submit button

---

### Pattern 7: Responsive Behavior

#### Breakpoints
- **Mobile** (<640px): Single column, stacked layouts
- **Tablet** (640-1024px): Reduced columns, collapsible sidebar
- **Desktop** (>1024px): Full layouts, expanded sidebar

#### Mobile Adaptations
- Master-detail becomes stacked (list → detail on tap)
- Tables become cards with key info
- Forms become single column

---

## UI Requirements

### General

- **UI-001**: Remove view mode toggle (Grid/List/Table) from all entity pages - default to table
- **UI-002**: Replace all Sheet-based editing with Master-Detail layout
- **UI-003**: Standardize row actions to dropdown menu (⋮) pattern
- **UI-004**: Standardize delete confirmation to dialog pattern
- **UI-005**: Add multi-select capability to all entity lists
- **UI-006**: Add bulk actions (delete, enable, disable) to all entity lists

### Manual Channels Specific

- **UI-007**: Replace `ManualChannelEditor` card-based layout with inline data table
- **UI-008**: Remove "Apply Changes" button - track changes in parent form state
- **UI-009**: ~~Support CSV paste in addition to M3U import~~ *Deferred to future enhancement*
- **UI-010**: Add column visibility toggle (show/hide optional fields)
- **UI-011**: Add logo thumbnail preview in table

### Stream/EPG Sources

- **UI-012**: Convert to Master-Detail layout
- **UI-013**: Remove sheet-based create/edit
- **UI-014**: Add inline status indicators with last sync time

### Proxies

- **UI-015**: Convert create flow to wizard (Sources → EPG → Filters → Settings)
- **UI-016**: Use inline data tables for source/EPG/filter assignments
- **UI-017**: Show channel count preview as sources are selected

### Encoding Profiles / Client Detection

- **UI-018**: Convert to Master-Detail layout
- **UI-019**: Group related settings into collapsible sections

---

## Success Criteria

### Quantitative

- **SC-001**: Users can create a manual source with channels in under 3 minutes
- **SC-002**: Manual channels appear in the main channel list within 5 seconds of save
- **SC-003**: M3U import correctly parses 100% of valid #EXTINF lines
- **SC-004**: Export produces M3U that can be re-imported with identical data
- **SC-005**: Reduce total frontend component code by 20%+ through shared patterns
- **SC-006**: All CRUD operations completable in ≤3 clicks
- **SC-007**: Form completion time reduced by 30% vs. sheet-based approach

### Qualitative

- **SC-008**: Consistent visual language across all pages
- **SC-009**: No multi-stage save patterns anywhere
- **SC-010**: Full keyboard accessibility (Tab, Enter, Escape, Arrow keys)

---

## Implementation Phases

### Phase 1: Backend API (enables manual channels)
1. Create `internal/http/handlers/manual_channel.go`
2. Create `internal/service/manual_channel_service.go`
3. Implement M3U parser and generator
4. Register routes and test endpoints

### Phase 2: Shared UI Components
5. Create `DataTable` component with selection, sorting, pagination
6. Create `InlineEditTable` component for nested collections
7. Create `MasterDetailLayout` component
8. Create shared form patterns (sections, actions)

### Phase 3: Manual Channels UI
9. Replace `ManualChannelEditor` with `InlineEditTable`
10. Remove "Apply Changes" pattern
11. Add CSV paste support

### Phase 4: Entity Pages Migration
12. Convert Stream Sources to Master-Detail
13. Convert EPG Sources to Master-Detail
14. Convert Filters to Master-Detail
15. Convert Data Mapping to Master-Detail
16. Convert Encoding Profiles to Master-Detail
17. Convert Client Detection to Master-Detail

### Phase 5: Complex Entities
18. Convert Proxy creation to wizard flow
19. Update Logos to gallery with side panel

### Phase 6: Polish
20. Remove all view mode toggles
21. Standardize empty states
22. Standardize loading states
23. Mobile responsive pass
24. Accessibility audit

---

## Component Migration Map

| Current Component | New Pattern | Priority |
|-------------------|-------------|----------|
| `manual-channel-editor.tsx` | InlineEditTable | P1 |
| `stream-sources.tsx` | MasterDetailLayout | P1 |
| `epg-sources.tsx` | MasterDetailLayout | P1 |
| `filters.tsx` | MasterDetailLayout | P2 |
| `data-mapping.tsx` | MasterDetailLayout | P2 |
| `encoding-profiles.tsx` | MasterDetailLayout | P2 |
| `client-detection-rules.tsx` | MasterDetailLayout | P2 |
| `CreateProxyModal.tsx` | WizardLayout | P2 |
| `logos.tsx` | GalleryWithPanel | P3 |
| All view mode toggles | Remove | P1 |

---

## Technical Notes

### Recommended Libraries
- **Data Tables**: `@tanstack/react-table` v8
- **Forms**: `react-hook-form` + `zod` (already in use)
- **Drag & Drop**: `@dnd-kit/core` (for reordering)
- **Virtualization**: `@tanstack/react-virtual` (for large lists)

### Shared Component Structure
```
frontend/src/components/
├── ui/                    # shadcn primitives (keep)
├── shared/                # NEW: shared patterns
│   ├── data-table/
│   │   ├── DataTable.tsx
│   │   └── columns.tsx
│   ├── inline-edit-table/
│   │   └── InlineEditTable.tsx
│   ├── layouts/
│   │   ├── MasterDetailLayout.tsx
│   │   └── WizardLayout.tsx
│   └── feedback/
│       ├── EmptyState.tsx
│       └── SkeletonTable.tsx
└── features/              # Feature-specific
    ├── stream-sources/
    ├── manual-channels/
    └── ...
```
