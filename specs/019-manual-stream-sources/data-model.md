# Data Model: Manual Stream Sources & UI/UX Overhaul

**Feature**: 019-manual-stream-sources
**Date**: 2025-01-10

## Existing Entities (No Schema Changes)

### ManualStreamChannel

The existing `ManualStreamChannel` model already supports all required fields. No schema changes needed.

```go
// internal/models/manual_stream_channel.go
type ManualStreamChannel struct {
    BaseModel

    // SourceID is the Manual stream source this channel belongs to.
    SourceID ULID `gorm:"not null;index" json:"source_id"`

    // TvgID is the EPG channel identifier for matching with program data.
    TvgID string `gorm:"size:255;index" json:"tvg_id,omitempty"`

    // TvgName is the display name.
    TvgName string `gorm:"size:512" json:"tvg_name,omitempty"`

    // TvgLogo is the URL to the channel logo (URL or @logo:token).
    TvgLogo string `gorm:"size:2048" json:"tvg_logo,omitempty"`

    // GroupTitle is the category/group.
    GroupTitle string `gorm:"size:255;index" json:"group_title,omitempty"`

    // ChannelName is the display name (required).
    ChannelName string `gorm:"not null;size:512" json:"channel_name"`

    // ChannelNumber is the channel number if specified.
    ChannelNumber int `gorm:"default:0" json:"channel_number,omitempty"`

    // StreamURL is the actual stream URL (required).
    StreamURL string `gorm:"not null;size:4096" json:"stream_url"`

    // StreamType indicates the stream format.
    StreamType string `gorm:"size:50" json:"stream_type,omitempty"`

    // Language is the channel language if known.
    Language string `gorm:"size:50" json:"language,omitempty"`

    // Country is the channel country code if known.
    Country string `gorm:"size:10" json:"country,omitempty"`

    // IsAdult indicates whether this is adult content.
    IsAdult bool `gorm:"default:false" json:"is_adult"`

    // Enabled indicates whether this channel should be included.
    Enabled *bool `gorm:"default:true" json:"enabled"`

    // Priority for ordering among manual channels.
    Priority int `gorm:"default:0" json:"priority"`

    // Extra stores additional attributes as JSON.
    Extra string `gorm:"type:text" json:"extra,omitempty"`
}
```

**Key Fields**:
- `SourceID` - Foreign key to `StreamSource` (must be type="manual")
- `ChannelName` - Required, max 512 chars
- `StreamURL` - Required, must start with `http://`, `https://`, or `rtsp://`
- `TvgLogo` - Optional, validates as empty, `@logo:*` token, or HTTP(S) URL
- `ChannelNumber` - Optional, used to detect duplicates within source

### StreamSource (Reference)

Manual channels reference the existing `StreamSource` model with `type="manual"`:

```go
// internal/models/stream_source.go
type StreamSource struct {
    // ...existing fields...
    Type SourceType `gorm:"not null;size:50;index" json:"type"` // "m3u", "xtream", "manual"
}
```

When `Type == "manual"`, the stream source has no URL requirement and channels come from `ManualStreamChannel` table.

## New API Types

### Handler Request/Response Types

```go
// internal/http/handlers/types.go

// ManualChannelResponse represents a manual channel in API responses.
type ManualChannelResponse struct {
    ID            models.ULID `json:"id"`
    SourceID      models.ULID `json:"source_id"`
    TvgID         string      `json:"tvg_id,omitempty"`
    TvgName       string      `json:"tvg_name,omitempty"`
    TvgLogo       string      `json:"tvg_logo,omitempty"`
    GroupTitle    string      `json:"group_title,omitempty"`
    ChannelName   string      `json:"channel_name"`
    ChannelNumber int         `json:"channel_number,omitempty"`
    StreamURL     string      `json:"stream_url"`
    StreamType    string      `json:"stream_type,omitempty"`
    Language      string      `json:"language,omitempty"`
    Country       string      `json:"country,omitempty"`
    IsAdult       bool        `json:"is_adult"`
    Enabled       bool        `json:"enabled"`
    Priority      int         `json:"priority"`
    CreatedAt     time.Time   `json:"created_at"`
    UpdatedAt     time.Time   `json:"updated_at"`
}

// ManualChannelFromModel converts a model to a response.
func ManualChannelFromModel(c *models.ManualStreamChannel) ManualChannelResponse {
    return ManualChannelResponse{
        ID:            c.ID,
        SourceID:      c.SourceID,
        TvgID:         c.TvgID,
        TvgName:       c.TvgName,
        TvgLogo:       c.TvgLogo,
        GroupTitle:    c.GroupTitle,
        ChannelName:   c.ChannelName,
        ChannelNumber: c.ChannelNumber,
        StreamURL:     c.StreamURL,
        StreamType:    c.StreamType,
        Language:      c.Language,
        Country:       c.Country,
        IsAdult:       c.IsAdult,
        Enabled:       models.BoolVal(c.Enabled),
        Priority:      c.Priority,
        CreatedAt:     c.CreatedAt,
        UpdatedAt:     c.UpdatedAt,
    }
}

// ManualChannelInput is a single channel in PUT/import requests.
type ManualChannelInput struct {
    TvgID         string `json:"tvg_id,omitempty" doc:"EPG ID for matching" maxLength:"255"`
    TvgName       string `json:"tvg_name,omitempty" doc:"Display name" maxLength:"512"`
    TvgLogo       string `json:"tvg_logo,omitempty" doc:"Logo URL or @logo:token" maxLength:"2048"`
    GroupTitle    string `json:"group_title,omitempty" doc:"Category/group" maxLength:"255"`
    ChannelName   string `json:"channel_name" doc:"Required display name" minLength:"1" maxLength:"512"`
    ChannelNumber int    `json:"channel_number,omitempty" doc:"Optional channel number"`
    StreamURL     string `json:"stream_url" doc:"Stream URL (http/https/rtsp)" minLength:"1" maxLength:"4096"`
    StreamType    string `json:"stream_type,omitempty" doc:"Stream format" maxLength:"50"`
    Language      string `json:"language,omitempty" doc:"Language code" maxLength:"50"`
    Country       string `json:"country,omitempty" doc:"Country code" maxLength:"10"`
    IsAdult       bool   `json:"is_adult,omitempty" doc:"Adult content flag"`
    Enabled       *bool  `json:"enabled,omitempty" doc:"Include in materialization (default: true)"`
    Priority      int    `json:"priority,omitempty" doc:"Sort order"`
}

// ToModel converts input to model for persistence.
func (r *ManualChannelInput) ToModel(sourceID models.ULID) *models.ManualStreamChannel {
    enabled := models.BoolPtr(true)
    if r.Enabled != nil {
        enabled = r.Enabled
    }
    return &models.ManualStreamChannel{
        SourceID:      sourceID,
        TvgID:         r.TvgID,
        TvgName:       r.TvgName,
        TvgLogo:       r.TvgLogo,
        GroupTitle:    r.GroupTitle,
        ChannelName:   r.ChannelName,
        ChannelNumber: r.ChannelNumber,
        StreamURL:     r.StreamURL,
        StreamType:    r.StreamType,
        Language:      r.Language,
        Country:       r.Country,
        IsAdult:       r.IsAdult,
        Enabled:       enabled,
        Priority:      r.Priority,
    }
}

// ReplaceManualChannelsRequest is the PUT request body.
type ReplaceManualChannelsRequest struct {
    Channels []ManualChannelInput `json:"channels" doc:"Complete list of channels (replaces all existing)"`
}

// M3UImportResult is the response for import operations.
type M3UImportResult struct {
    ParsedCount   int                      `json:"parsed_count" doc:"Number of channels parsed from M3U"`
    SkippedCount  int                      `json:"skipped_count" doc:"Number of invalid entries skipped"`
    Applied       bool                     `json:"applied" doc:"Whether changes were persisted"`
    Channels      []ManualChannelResponse  `json:"channels" doc:"Parsed/applied channels"`
    Errors        []string                 `json:"errors,omitempty" doc:"Parse errors encountered"`
}
```

## Frontend Types

### TypeScript API Types

```typescript
// frontend/src/types/api.ts

// ManualChannel represents a channel in a manual stream source
export interface ManualChannel {
  id: string;
  source_id: string;
  tvg_id?: string;
  tvg_name?: string;
  tvg_logo?: string;
  group_title?: string;
  channel_name: string;
  channel_number?: number;
  stream_url: string;
  stream_type?: string;
  language?: string;
  country?: string;
  is_adult: boolean;
  enabled: boolean;
  priority: number;
  created_at: string;
  updated_at: string;
}

// ManualChannelInput for creating/updating channels
export interface ManualChannelInput {
  tvg_id?: string;
  tvg_name?: string;
  tvg_logo?: string;
  group_title?: string;
  channel_name: string;
  channel_number?: number;
  stream_url: string;
  stream_type?: string;
  language?: string;
  country?: string;
  is_adult?: boolean;
  enabled?: boolean;
  priority?: number;
}

// M3UImportResult from import endpoint
export interface M3UImportResult {
  parsed_count: number;
  skipped_count: number;
  applied: boolean;
  channels: ManualChannel[];
  errors?: string[];
}
```

### Shared Component Props (TypeScript)

```typescript
// frontend/src/components/shared/layouts/MasterDetailLayout.tsx

export interface MasterDetailLayoutProps<T extends { id: string }> {
  // List configuration
  items: T[];
  columns: ColumnDef<T>[];
  selectedId: string | null;
  onSelect: (id: string | null) => void;

  // Detail panel
  detailContent: React.ReactNode;
  emptyDetailContent?: React.ReactNode;

  // List features
  searchPlaceholder?: string;
  onSearch?: (query: string) => void;
  searchValue?: string;

  // Multi-select & bulk actions
  enableSelection?: boolean;
  selectedIds?: string[];
  onSelectionChange?: (ids: string[]) => void;
  bulkActions?: BulkAction[];

  // Layout options
  listWidth?: string; // Default "40%"
  minListWidth?: number; // Default 300
  maxListWidth?: number; // Default 600

  // Loading states
  isLoading?: boolean;
  isDetailLoading?: boolean;

  // Empty state
  emptyState?: React.ReactNode;
}

export interface BulkAction {
  id: string;
  label: string;
  icon?: React.ReactNode;
  variant?: 'default' | 'destructive';
  onExecute: (ids: string[]) => Promise<void>;
}
```

```typescript
// frontend/src/components/shared/inline-edit-table/InlineEditTable.tsx

export interface InlineEditTableProps<T extends { id?: string }> {
  // Data
  columns: InlineEditColumn<T>[];
  value: T[];
  onChange: (value: T[]) => void;

  // Row behavior
  getRowId?: (row: T, index: number) => string;
  createEmptyRow: () => Partial<T>;

  // Features
  enableReorder?: boolean;
  enableRowDelete?: boolean;
  enableColumnVisibility?: boolean;

  // Validation
  validateRow?: (row: T) => Record<string, string> | null;

  // Import/export
  importOptions?: ImportOption[];
  exportOptions?: ExportOption[];
}

export interface InlineEditColumn<T> {
  id: keyof T | string;
  header: string;
  type: 'text' | 'number' | 'select' | 'checkbox' | 'url' | 'image-preview';
  width?: string;
  hidden?: boolean;
  hideable?: boolean;
  required?: boolean;
  placeholder?: string;
  options?: { label: string; value: string }[]; // For select type
  validate?: (value: unknown, row: T) => string | null;
  renderCell?: (value: unknown, row: T) => React.ReactNode; // Custom renderer
}
```

## Entity Relationships

```
┌─────────────────────┐
│ StreamSource        │
├─────────────────────┤
│ id                  │
│ type = "manual"     │───────────┐
│ name                │           │
│ ...                 │           │
└─────────────────────┘           │
          │                       │
          │ 1:N                   │
          ▼                       │
┌─────────────────────┐           │
│ ManualStreamChannel │           │
├─────────────────────┤           │
│ id                  │           │
│ source_id          ◄────────────┘
│ channel_name        │
│ stream_url          │
│ tvg_id              │
│ tvg_logo            │
│ ...                 │
└─────────────────────┘
          │
          │ Materialization (ingestor)
          ▼
┌─────────────────────┐
│ Channel             │
├─────────────────────┤
│ id                  │
│ source_id           │◄── Same as ManualStreamChannel.source_id
│ ext_id              │◄── ManualStreamChannel.id (for deduplication)
│ channel_name        │
│ stream_url          │
│ ...                 │
└─────────────────────┘
```

## Validation Rules

### ManualStreamChannel

| Field | Rule | Error Message |
|-------|------|---------------|
| `channel_name` | Required, non-empty, max 512 chars | "channel name is required" |
| `stream_url` | Required, must start with `http://`, `https://`, or `rtsp://` | "stream URL must be http, https, or rtsp" |
| `tvg_logo` | Optional; if provided, must be empty OR `@logo:*` token OR `http(s)://` URL | "invalid logo reference format" |
| `channel_number` | Optional; if multiple channels have same non-zero number, warn (not error) | "duplicate channel number: N" |

### M3U Import

| Condition | Behavior |
|-----------|----------|
| Invalid `#EXTINF` line | Skip entry, add to errors array |
| Missing URL after `#EXTINF` | Skip entry, add to errors array |
| Valid entry | Convert to `ManualChannelInput`, add to channels array |
| Empty M3U content | Return 400 Bad Request |
| Non-M3U content | Return 400 Bad Request |

### Source Validation

| Condition | Behavior |
|-----------|----------|
| Source not found | Return 404 Not Found |
| Source type != "manual" | Return 400 Bad Request: "operation only valid for manual sources" |

## State Transitions

### ManualStreamChannel Lifecycle

```
┌──────────┐    PUT /manual-channels    ┌─────────┐
│          │──────────────────────────►│ Created │
│  (none)  │                            │         │
│          │                            └────┬────┘
└──────────┘                                 │
                                             │ PUT (replace all)
                                             ▼
                                        ┌─────────┐
                                        │ Replaced│
                                        │         │
                                        └────┬────┘
                                             │
                                             │ Materialization
                                             │ (ingestor trigger)
                                             ▼
                                        ┌─────────────┐
                                        │ Materialized│
                                        │ to Channel  │
                                        └─────────────┘
```

### Materialization Flow

```
┌─────────────────────────┐
│ PUT /manual-channels    │
│ (channels updated)      │
└──────┬──────────────────┘
       │
       ▼
┌─────────────────────────┐
│ Create ingestion job    │
│ for manual source       │
└──────┬──────────────────┘
       │
       ▼
┌─────────────────────────┐
│ ManualHandler.Ingest()  │
│ - Clear old channels    │
│ - Create from manual    │
└──────┬──────────────────┘
       │
       ▼
┌─────────────────────────┐
│ Channels table updated  │
│ (visible in UI/proxies) │
└─────────────────────────┘
```

## Database Migrations

No new migrations required. The `manual_stream_channels` table already exists.

## API Endpoint Summary

| Method | Path | Request Body | Response |
|--------|------|--------------|----------|
| GET | `/api/v1/sources/stream/{sourceId}/manual-channels` | - | `{ items: ManualChannel[], total: number }` |
| PUT | `/api/v1/sources/stream/{sourceId}/manual-channels` | `{ channels: ManualChannelInput[] }` | `{ items: ManualChannel[] }` |
| POST | `/api/v1/sources/stream/{sourceId}/manual-channels/import-m3u?apply={bool}` | `text/plain` (M3U content) | `M3UImportResult` |
| GET | `/api/v1/sources/stream/{sourceId}/manual-channels/export.m3u` | - | `text/plain` (M3U content) |

## Frontend API Client Extensions

```typescript
// frontend/src/lib/api-client.ts

export const apiClient = {
  // ...existing methods...

  // Manual Channels
  listManualChannels: async (sourceId: string): Promise<ManualChannel[]> => {
    const response = await fetch(`/api/v1/sources/stream/${sourceId}/manual-channels`);
    if (!response.ok) throw new ApiError(response);
    const data = await response.json();
    return data.items;
  },

  replaceManualChannels: async (
    sourceId: string,
    channels: ManualChannelInput[]
  ): Promise<ManualChannel[]> => {
    const response = await fetch(`/api/v1/sources/stream/${sourceId}/manual-channels`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ channels }),
    });
    if (!response.ok) throw new ApiError(response);
    const data = await response.json();
    return data.items;
  },

  importM3U: async (
    sourceId: string,
    content: string,
    apply: boolean
  ): Promise<M3UImportResult> => {
    const response = await fetch(
      `/api/v1/sources/stream/${sourceId}/manual-channels/import-m3u?apply=${apply}`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'text/plain' },
        body: content,
      }
    );
    if (!response.ok) throw new ApiError(response);
    return response.json();
  },

  exportM3U: async (sourceId: string): Promise<string> => {
    const response = await fetch(
      `/api/v1/sources/stream/${sourceId}/manual-channels/export.m3u`
    );
    if (!response.ok) throw new ApiError(response);
    return response.text();
  },
};
```
