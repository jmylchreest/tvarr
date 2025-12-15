# Quickstart: Client Detection UI Improvements

**Feature**: 018-client-detection-ui
**Date**: 2025-12-15

## Prerequisites

- Go 1.25.x installed
- Node.js 20.x and npm/pnpm installed
- Task (taskfile.dev) installed
- SQLite or configured database

## Development Setup

```bash
# Clone and checkout feature branch
git checkout 018-client-detection-ui

# Backend setup
task build

# Frontend setup
cd frontend
pnpm install
pnpm run dev

# Run backend
./bin/tvarr serve
```

## Implementation Order

### Phase 1: Export/Import Fix (P1)

**Files to modify**:
- `frontend/src/lib/api-client.ts`

**Steps**:
1. Find export methods (lines 1195-1233)
2. Change URL pattern from `${endpoint}/export` to `/api/v1/export/{type}`
3. Find import methods (lines 1239-1365)
4. Change URL pattern for preview from `${endpoint}/import?preview=true` to `/api/v1/import/{type}/preview`
5. Change URL pattern for execute from `${endpoint}/import` to `/api/v1/import/{type}`

**Test**:
```bash
# Create a filter, then export
curl -X POST http://localhost:8080/api/v1/export/filters \
  -H "Content-Type: application/json" \
  -d '{"ids": [], "export_all": true}'
```

### Phase 2: Expression Editor Enhancement (P2)

**Files to create**:
- `frontend/src/components/client-detection-expression-editor.tsx`

**Files to modify**:
- `frontend/src/lib/expression-constants.ts` - Add CLIENT_DETECTION_HELPERS
- `frontend/src/components/client-detection-rules.tsx` - Replace Textarea with new editor

**Pattern to follow**: See `data-mapping-expression-editor.tsx` for reference implementation.

**Key components to reuse**:
- `ExpressionEditor` - base component
- `ValidationBadges` - status display
- `useHelperAutocomplete` - @helper syntax support

### Phase 3: Copyable Expressions (P2)

**Files to modify**:
- `frontend/src/components/client-detection-rules.tsx`

**Implementation**:
```tsx
import { Copy, Check } from 'lucide-react';
import { useState } from 'react';

function CopyableExpression({ expression }: { expression: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    await navigator.clipboard.writeText(expression);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <code
      className="cursor-pointer hover:bg-muted px-2 py-1 rounded text-sm font-mono group inline-flex items-center gap-1"
      onClick={handleCopy}
      title="Click to copy"
    >
      <span className="truncate max-w-[300px]">{expression}</span>
      {copied ? (
        <Check className="h-3 w-3 text-green-500" />
      ) : (
        <Copy className="h-3 w-3 opacity-0 group-hover:opacity-50" />
      )}
    </code>
  );
}
```

### Phase 4: Smart Remuxing (P2)

**Files to create**:
- `internal/relay/codec_compatibility.go`

**Files to modify**:
- `internal/relay/routing_decision.go`

**Key change in `Decide()` method**:
```go
// Before returning RouteTranscode, check codec compatibility
if AreCodecsCompatible(targetContainer, sourceCodecs) {
    return RouteRepackage
}
return RouteTranscode
```

**Test**:
```bash
# Play HEVC/EAC3 stream requesting HLS, verify FFmpeg uses -c:v copy -c:a copy
task run -- serve &
curl "http://localhost:8080/proxy/stream/channel-id.m3u8" \
  -H "User-Agent: mpv"
# Check logs for "copy" codec usage
```

### Phase 5: Default System Rules (P3)

**Files to create**:
- `internal/database/migrations/migration_013_system_client_detection_rules.go`

**Files to modify**:
- `internal/database/migrations/registry.go` - Add migration013 to AllMigrations()

**Test**:
```bash
# Run migration
task migrate

# Verify rules created
sqlite3 data/tvarr.db "SELECT name, is_system FROM client_detection_rules WHERE is_system = 1"
```

### Phase 6: Fuzzy Search (P2)

**Files to create**:
- `frontend/src/lib/fuzzy-search.ts`

**Files to modify**:
- `frontend/src/app/channels/page.tsx`
- `frontend/src/app/epg/page.tsx`

**Install dependency**:
```bash
cd frontend
pnpm add fuse.js
```

**Usage pattern**:
```tsx
import Fuse from 'fuse.js';
import type { FuseResult } from 'fuse.js';

const fuse = new Fuse(channels, {
  keys: [
    { name: 'channel_name', weight: 0.4 },
    { name: 'tvg_name', weight: 0.2 },
    { name: 'tvg_id', weight: 0.2 },
    { name: 'group_title', weight: 0.1 },
    { name: 'channel_number', weight: 0.05 },
    { name: 'ext_id', weight: 0.05 }
  ],
  threshold: 0.4,
  includeScore: true,
  includeMatches: true
});

const results = searchTerm ? fuse.search(searchTerm) : channels.map(item => ({ item }));
```

## Testing Checklist

### Export/Import (FR-001)
- [ ] Export filters works
- [ ] Export data mapping rules works
- [ ] Export client detection rules works
- [ ] Import preview works for all types
- [ ] Import execute works for all types

### Expression Editor (FR-003, FR-004, FR-005)
- [ ] Typing "@" shows autocomplete popup
- [ ] @dynamic() helper appears in suggestions
- [ ] request.headers completion works
- [ ] user-agent sub-completion works
- [ ] Tab/Enter/Click inserts suggestion
- [ ] Validation badges show correct states
- [ ] Invalid expressions show error badges

### Copyable Expressions (FR-002)
- [ ] Click on expression copies to clipboard
- [ ] Visual feedback (copy icon, checkmark) works
- [ ] Works with truncated expressions

### Smart Remuxing (FR-009, FR-010)
- [ ] MPEG-TS→HLS with HEVC/EAC3 uses copy
- [ ] Incompatible codecs trigger transcode
- [ ] Exact format match uses passthrough

### System Rules (FR-006, FR-007)
- [ ] 6 system rules created on fresh install
- [ ] System rules marked with badge in UI
- [ ] System rules cannot be deleted
- [ ] System rules can be disabled

### Fuzzy Search (FR-011, FR-012, FR-013, FR-014)
- [ ] Channel search finds partial matches
- [ ] Channel search tolerates typos
- [ ] Channel search covers all fields
- [ ] EPG search finds partial matches
- [ ] EPG search tolerates typos
- [ ] EPG search covers all fields
- [ ] Match indicators show which field matched

## Common Issues

### Export returns 405
**Cause**: Frontend URL mismatch
**Fix**: Check api-client.ts uses `/api/v1/export/{type}` pattern

### Autocomplete not appearing
**Cause**: Missing helper configuration
**Fix**: Ensure CLIENT_DETECTION_HELPERS added to expression-constants.ts

### System rules not created
**Cause**: Migration not run
**Fix**: Run `task migrate` or restart server

### Fuzzy search slow
**Cause**: Too many results being processed client-side
**Fix**: Ensure backend pagination limits results to ~500 per request
