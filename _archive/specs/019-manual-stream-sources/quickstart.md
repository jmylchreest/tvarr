# Quickstart: Manual Stream Sources & UI/UX Overhaul

**Feature**: 019-manual-stream-sources
**Date**: 2025-01-10

## Overview

This feature completes the manual stream sources backend API and introduces a comprehensive UI/UX overhaul. After implementation, users will be able to:

1. Create manual stream sources with custom channel definitions
2. Import/export channels via M3U format
3. Edit channels in an inline data table (not nested cards)
4. Use consistent Master-Detail layouts across all entity pages

## Part 1: Manual Stream Sources API

### Creating a Manual Source

```bash
# Create a manual stream source
curl -X POST http://localhost:8080/api/v1/sources/stream \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My Cameras",
    "type": "manual",
    "enabled": true
  }'
```

Response:
```json
{
  "id": "01HQGZ...",
  "name": "My Cameras",
  "type": "manual",
  "enabled": true,
  "status": "pending",
  "channel_count": 0
}
```

### Adding Manual Channels

```bash
# Replace all channels for a manual source
curl -X PUT http://localhost:8080/api/v1/sources/stream/{sourceId}/manual-channels \
  -H "Content-Type: application/json" \
  -d '{
    "channels": [
      {
        "channel_name": "Front Door",
        "stream_url": "rtsp://192.168.1.50:554/stream1",
        "channel_number": 1,
        "group_title": "Cameras"
      },
      {
        "channel_name": "Backyard",
        "stream_url": "rtsp://192.168.1.51:554/stream1",
        "channel_number": 2,
        "group_title": "Cameras"
      }
    ]
  }'
```

### Importing from M3U

```bash
# Preview import (apply=false)
curl -X POST "http://localhost:8080/api/v1/sources/stream/{sourceId}/manual-channels/import-m3u?apply=false" \
  -H "Content-Type: text/plain" \
  -d '#EXTM3U
#EXTINF:-1 tvg-id="cam1" tvg-name="Front Door" group-title="Cameras",Front Door
rtsp://192.168.1.50:554/stream1
#EXTINF:-1 tvg-id="cam2" tvg-name="Backyard" group-title="Cameras",Backyard
rtsp://192.168.1.51:554/stream1'
```

Response:
```json
{
  "parsed_count": 2,
  "skipped_count": 0,
  "applied": false,
  "channels": [
    {
      "channel_name": "Front Door",
      "stream_url": "rtsp://192.168.1.50:554/stream1",
      "tvg_id": "cam1",
      "group_title": "Cameras"
    },
    {
      "channel_name": "Backyard",
      "stream_url": "rtsp://192.168.1.51:554/stream1",
      "tvg_id": "cam2",
      "group_title": "Cameras"
    }
  ]
}
```

```bash
# Apply import (apply=true)
curl -X POST "http://localhost:8080/api/v1/sources/stream/{sourceId}/manual-channels/import-m3u?apply=true" \
  -H "Content-Type: text/plain" \
  -d '...'
```

### Exporting to M3U

```bash
curl http://localhost:8080/api/v1/sources/stream/{sourceId}/manual-channels/export.m3u
```

Response:
```
#EXTM3U
#EXTINF:-1 tvg-id="cam1" tvg-name="Front Door" tvg-chno="1" group-title="Cameras",Front Door
rtsp://192.168.1.50:554/stream1
#EXTINF:-1 tvg-id="cam2" tvg-name="Backyard" tvg-chno="2" group-title="Cameras",Backyard
rtsp://192.168.1.51:554/stream1
```

## Part 2: UI/UX Patterns

### Master-Detail Layout

The new Master-Detail layout replaces sheet-based editing:

```
┌─────────────────────────────────────────────────────────────────┐
│ Stream Sources                    [+ Create] [Import] [Export]  │
├─────────────────────────────────────────────────────────────────│
│ [Search...________________] [Type ▾] [Status ▾]    [Refresh]    │
├────────────────────────────┬────────────────────────────────────│
│ ☐  Name           Type  ✓ │  Edit: My Cameras                   │
│ ☑  My Cameras     Man.  ● │  ────────────────────────────────── │
│ ☐  BBC Streams    M3U   ● │  Name     [My Cameras__________]    │
│ ☐  Sky UK         Xtrm  ● │  Type     [Manual ▾]                │
│                           │                                     │
│                           │  ┌─ Channels ────────────────────┐  │
│                           │  │ #  │ Name       │ Stream URL  │  │
│                           │  ├────┼────────────┼─────────────┤  │
│                           │  │ 1  │ Front Door │ rtsp://...  │  │
│                           │  │ 2  │ Backyard   │ rtsp://...  │  │
│                           │  │    │ [+ Add]    │             │  │
│                           │  └────────────────────────────────┘ │
│                           │                                     │
│                           │            [Cancel] [Save Changes]  │
└─────────────────────────────────────────────────────────────────┘
```

### Inline Edit Table

For manual channels and other nested collections:

```typescript
import { InlineEditTable } from '@/components/shared/inline-edit-table';

const columns = [
  { id: 'channel_number', header: '#', type: 'number', width: '60px' },
  { id: 'channel_name', header: 'Name', type: 'text', required: true },
  { id: 'stream_url', header: 'Stream URL', type: 'url', required: true },
  { id: 'group_title', header: 'Group', type: 'text' },
  { id: 'tvg_logo', header: 'Logo', type: 'image-preview', hideable: true },
];

<InlineEditTable
  columns={columns}
  value={channels}
  onChange={setChannels}
  createEmptyRow={() => ({ channel_name: '', stream_url: '' })}
  enableRowDelete
  importOptions={[{ label: 'Import M3U', handler: handleM3UImport }]}
  exportOptions={[{ label: 'Export M3U', handler: handleM3UExport }]}
/>
```

## Key Changes from Current UI

| Before | After |
|--------|-------|
| Sheet slides in for editing | Detail panel always visible |
| Cards for each manual channel | Inline editable table rows |
| "Apply Changes" then "Save" | Single "Save" button |
| Grid/List/Table view toggles | Table view only (consistent) |
| Nested sheets | Flat Master-Detail layout |

## Testing the Feature

### Backend Tests

```bash
# Run all manual channel tests
go test ./internal/http/handlers/... -run Manual -v
go test ./internal/service/... -run Manual -v
```

### Frontend Tests

```bash
# Run component tests
cd frontend
npm test -- --testPathPattern="manual|inline-edit|master-detail"
```

### Manual Testing

1. **Create manual source**: Go to Stream Sources, click "+ Create", select "Manual" type
2. **Add channels**: In the detail panel, use the inline table to add channels
3. **Import M3U**: Click "Import M3U" dropdown, paste content, preview, then apply
4. **Export M3U**: Click "Export M3U" to download current channels
5. **Trigger ingestion**: Click "Ingest" to materialize channels to the main table
6. **Verify**: Check Channels page shows the manual channels

## File Structure

```
# Backend
internal/
├── http/handlers/
│   ├── manual_channel.go        # NEW: API handlers
│   └── types.go                 # Extended with manual channel types
├── service/
│   └── manual_channel_service.go # NEW: Business logic
└── (existing)
    ├── models/manual_stream_channel.go
    ├── repository/manual_channel_repo.go
    └── ingestor/manual_handler.go

# Frontend
frontend/src/
├── components/
│   ├── shared/
│   │   ├── data-table/          # Generic data table
│   │   ├── inline-edit-table/   # Inline editable table
│   │   └── layouts/
│   │       ├── MasterDetailLayout.tsx
│   │       └── WizardLayout.tsx
│   └── (refactored)
│       ├── stream-sources.tsx   # Uses MasterDetailLayout
│       ├── manual-channel-editor.tsx → InlineEditTable
│       └── ...
└── lib/
    └── api-client.ts            # Extended with manual channel methods
```

## Common Issues

### "operation only valid for manual sources"

The source type must be "manual". Check with:
```bash
curl http://localhost:8080/api/v1/sources/stream/{sourceId} | jq .type
```

### Channels not appearing in main list

After saving manual channels, trigger ingestion:
```bash
curl -X POST http://localhost:8080/api/v1/sources/stream/{sourceId}/ingest
```

### M3U import parsing errors

Check the `errors` array in the response. Common issues:
- Missing URL after `#EXTINF` line
- Malformed attributes (missing quotes)
- Empty lines between `#EXTINF` and URL
