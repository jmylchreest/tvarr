# Research: Manual Stream Sources & UI/UX Overhaul

**Generated**: 2025-01-10
**Branch**: `019-manual-stream-sources`

## Summary

This document resolves all technical unknowns identified during planning. All NEEDS CLARIFICATION items have been researched and resolved.

---

## 1. M3U Parsing - Existing Implementation

### Decision
**Reuse existing M3U parser and writer from `pkg/m3u/`**

### Rationale
The codebase already has a production-ready M3U parser and writer with streaming support:

- **Parser**: `pkg/m3u/parser.go` - Callback-based streaming parser
- **Writer**: `pkg/m3u/writer.go` - Streaming M3U generator with `ChannelData` interface

### Key Findings

#### Parser Features (Reusable)
```go
// Entry struct from pkg/m3u/parser.go
type Entry struct {
    Duration      int               // -1 for live streams
    TvgID         string
    TvgName       string
    TvgLogo       string
    GroupTitle    string
    ChannelNumber int
    Title         string
    URL           string
    Extra         map[string]string // Custom attributes
}
```

**Supported Attributes**: `tvg-id`, `tvg-name`, `tvg-logo`, `group-title`, `tvg-chno`, plus any custom attributes stored in `Extra`.

**Streaming Design**: Uses callback pattern (`OnEntry`, `OnError`) - no memory bloat for large files.

**Compression Support**: Automatic detection and decompression of gzip, bzip2, xz.

#### Writer Features (Reusable)
```go
// ChannelData interface from pkg/m3u/writer.go
type ChannelData interface {
    GetTvgID() string
    GetTvgName() string
    GetTvgLogo() string
    GetGroupTitle() string
    GetChannelName() string
    GetChannelNumber() int
    GetStreamURL() string
}
```

**Implementation**: `ManualStreamChannel` model needs to implement `ChannelData` interface for M3U export.

### Alternatives Considered

| Option | Rejected Because |
|--------|------------------|
| New parser | Existing parser covers all requirements |
| Third-party library | Internal package is well-tested, no external deps |
| Direct string building | Writer interface ensures proper formatting |

### Implementation Notes

1. **For Import**: Use `m3u.NewParser()` with callback to convert `m3u.Entry` → `ManualStreamChannel`
2. **For Export**: Implement `ChannelData` interface on `ManualStreamChannel`, use `m3u.NewWriter()`
3. **Location**: No new package needed - service layer calls `pkg/m3u` directly

---

## 2. Handler Patterns - Existing Conventions

### Decision
**Follow established handler patterns from `stream_source.go`, `channel.go`, `filter.go`**

### Rationale
The codebase has mature, consistent patterns for REST API handlers. Following them ensures:
- Consistency with existing code
- Compatibility with existing middleware and error handling
- Familiar patterns for developers

### Key Patterns

#### Constructor Pattern
```go
type ManualChannelHandler struct {
    service *service.ManualChannelService
    logger  *slog.Logger
}

func NewManualChannelHandler(service *service.ManualChannelService) *ManualChannelHandler {
    return &ManualChannelHandler{
        service: service,
        logger:  slog.Default(),
    }
}

func (h *ManualChannelHandler) WithLogger(logger *slog.Logger) *ManualChannelHandler {
    if logger != nil {
        h.logger = logger
    }
    return h
}
```

#### Route Registration Pattern
```go
func (h *ManualChannelHandler) Register(api huma.API) {
    huma.Register(api, huma.Operation{
        OperationID: "listManualChannels",
        Method:      "GET",
        Path:        "/api/v1/sources/stream/{sourceId}/manual-channels",
        Summary:     "List manual channels",
        Tags:        []string{"Manual Channels"},
    }, h.List)
    // ... more routes
}
```

#### Error Handling Pattern
```go
// 400 - Invalid input
return nil, huma.Error400BadRequest("invalid ID format", err)

// 404 - Not found
return nil, huma.Error404NotFound("manual channel not found")

// 409 - Conflict
return nil, huma.Error409Conflict("duplicate channel name")

// 500 - Server error
return nil, huma.Error500InternalServerError("failed to create channel", err)
```

#### Response Patterns
```go
// List response with pagination
type ListManualChannelsOutput struct {
    Body struct {
        Items      []ManualChannelResponse `json:"items"`
        Total      int64                   `json:"total"`
        Page       int                     `json:"page"`
        PerPage    int                     `json:"per_page"`
        TotalPages int                     `json:"total_pages"`
        HasNext    bool                    `json:"has_next"`
        HasPrev    bool                    `json:"has_previous"`
    }
}

// Single item response
type GetManualChannelOutput struct {
    Body ManualChannelResponse
}
```

### File Locations
- **Handler**: `internal/http/handlers/manual_channel.go`
- **Types**: `internal/http/handlers/types.go` (extend with manual channel types)
- **Registration**: `cmd/tvarr/cmd/serve.go` (add after stream source handler)

---

## 3. @tanstack/react-table Patterns

### Decision
**Use @tanstack/react-table v8 with React Hook Form + shadcn/ui for inline editable tables**

### Rationale
This combination provides:
- Type-safe table operations
- Built-in row selection, column visibility, sorting
- Seamless integration with existing shadcn/ui components
- Form-level validation with Zod schemas

### Key Patterns

#### Inline Editing with React Hook Form
```typescript
import { useForm, useFieldArray, Controller } from "react-hook-form"
import { useReactTable, getCoreRowModel, ColumnDef } from "@tanstack/react-table"

function InlineEditTable() {
  const { control, handleSubmit } = useForm({
    defaultValues: { channels: [] },
  })

  const { fields, append, remove } = useFieldArray({
    control,
    name: "channels",
  })

  const columns: ColumnDef<Channel>[] = [
    {
      accessorKey: "name",
      header: "Name",
      cell: ({ row }) => (
        <Controller
          name={`channels.${row.index}.name`}
          control={control}
          render={({ field }) => <Input {...field} />}
        />
      ),
    },
  ]

  const table = useReactTable({
    data: fields,
    columns,
    getCoreRowModel: getCoreRowModel(),
  })

  return <form onSubmit={handleSubmit(onSave)}>...</form>
}
```

#### Row Selection Pattern
```typescript
const [rowSelection, setRowSelection] = useState({})

const table = useReactTable({
  data,
  columns,
  state: { rowSelection },
  enableRowSelection: true,
  onRowSelectionChange: setRowSelection,
  getRowId: (row) => row.id, // Use database ID, not array index
})

// Get selected data for bulk operations
const selectedData = table.getSelectedRowModel().rows.map(row => row.original)
```

#### Column Visibility with shadcn/ui
```typescript
import { DropdownMenu, DropdownMenuCheckboxItem } from "@/components/ui/dropdown-menu"

<DropdownMenu>
  <DropdownMenuTrigger asChild>
    <Button variant="outline">Columns</Button>
  </DropdownMenuTrigger>
  <DropdownMenuContent>
    {table.getAllColumns().filter(col => col.getCanHide()).map(column => (
      <DropdownMenuCheckboxItem
        key={column.id}
        checked={column.getIsVisible()}
        onCheckedChange={(value) => column.toggleVisibility(!!value)}
      >
        {column.id}
      </DropdownMenuCheckboxItem>
    ))}
  </DropdownMenuContent>
</DropdownMenu>
```

### Dependencies Required
```json
{
  "@tanstack/react-table": "^8.x",
  "react-hook-form": "^7.x",
  "@hookform/resolvers": "^3.x",
  "zod": "^3.x"
}
```

Note: Most of these are already in the project. Need to verify `@tanstack/react-table` is installed.

---

## 4. Master-Detail Layout Pattern

### Decision
**Create reusable `MasterDetailLayout` component with resizable panels**

### Rationale
Provides consistent editing UX across all entity pages, replacing sheet-based editing.

### Key Features
- **Left panel**: Entity list with selection, filtering, multi-select
- **Right panel**: Detail form for selected entity
- **Resizable**: User can drag divider to adjust panel widths
- **Responsive**: Stacks on mobile (list view → detail view)

### Implementation Notes
```typescript
interface MasterDetailLayoutProps<T> {
  // List configuration
  items: T[]
  columns: ColumnDef<T>[]
  selectedId: string | null
  onSelect: (id: string) => void

  // Detail panel
  detailContent: React.ReactNode

  // Optional
  searchPlaceholder?: string
  onSearch?: (query: string) => void
  bulkActions?: BulkAction[]
  emptyState?: React.ReactNode
}
```

### File Location
`frontend/src/components/shared/layouts/MasterDetailLayout.tsx`

---

## 5. API Endpoint Design

### Decision
**Nest manual channel endpoints under stream source path**

### Rationale
Manual channels are children of stream sources - nesting reflects this relationship and ensures source validation.

### Endpoint Structure
```
GET    /api/v1/sources/stream/{sourceId}/manual-channels
PUT    /api/v1/sources/stream/{sourceId}/manual-channels
POST   /api/v1/sources/stream/{sourceId}/manual-channels/import-m3u?apply={bool}
GET    /api/v1/sources/stream/{sourceId}/manual-channels/export.m3u
```

### Alternatives Considered

| Option | Rejected Because |
|--------|------------------|
| `/api/v1/manual-channels` | Loses source context, requires source_id in every request body |
| `/api/v1/channels/manual` | Confuses with main channels table |

---

## 6. Frontend API Client Pattern

### Decision
**Extend existing `apiClient` in `frontend/src/lib/api-client.ts`**

### Rationale
Maintains consistency with existing API patterns, includes automatic error handling and type safety.

### Implementation
```typescript
// Add to api-client.ts
export const apiClient = {
  // ... existing methods

  // Manual Channels
  listManualChannels: async (sourceId: string): Promise<ManualChannel[]> => {
    const response = await fetch(`/api/v1/sources/stream/${sourceId}/manual-channels`)
    if (!response.ok) throw new ApiError(response)
    const data = await response.json()
    return data.items
  },

  replaceManualChannels: async (sourceId: string, channels: ManualChannelInput[]): Promise<void> => {
    const response = await fetch(`/api/v1/sources/stream/${sourceId}/manual-channels`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ channels }),
    })
    if (!response.ok) throw new ApiError(response)
  },

  importM3U: async (sourceId: string, content: string, apply: boolean): Promise<ImportResult> => {
    const response = await fetch(
      `/api/v1/sources/stream/${sourceId}/manual-channels/import-m3u?apply=${apply}`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'text/plain' },
        body: content,
      }
    )
    if (!response.ok) throw new ApiError(response)
    return response.json()
  },

  exportM3U: async (sourceId: string): Promise<string> => {
    const response = await fetch(`/api/v1/sources/stream/${sourceId}/manual-channels/export.m3u`)
    if (!response.ok) throw new ApiError(response)
    return response.text()
  },
}
```

---

## Summary

All technical unknowns have been resolved:

| Topic | Decision |
|-------|----------|
| M3U Parsing | Reuse `pkg/m3u/parser.go` and `writer.go` |
| Handler Patterns | Follow `stream_source.go` conventions |
| Data Tables | @tanstack/react-table + React Hook Form + shadcn/ui |
| Layout Pattern | New `MasterDetailLayout` component |
| API Design | Nested under `/sources/stream/{id}/manual-channels` |
| API Client | Extend existing `apiClient` |

Ready to proceed to Phase 1: Data Model and Contracts.
