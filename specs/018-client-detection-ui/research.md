# Research: Client Detection UI Improvements

**Feature**: 018-client-detection-ui
**Date**: 2025-12-15

## Research Tasks

### 1. Export/Import URL Mismatch (FR-001)

**Question**: What is causing the 405 error on export/import?

**Findings**:

The frontend API client builds URLs incorrectly:
- **Frontend calls**: `${API_CONFIG.endpoints.filters}/export` → `/api/v1/filters/export`
- **Backend exposes**: `/api/v1/export/filters`

Same pattern for all config types:

| Config Type | Frontend URL (Wrong) | Backend URL (Correct) |
|------------|---------------------|----------------------|
| Filters | `/api/v1/filters/export` | `/api/v1/export/filters` |
| Data Mapping | `/api/v1/data-mapping/export` | `/api/v1/export/data-mapping-rules` |
| Client Detection | `/api/v1/client-detection-rules/export` | `/api/v1/export/client-detection-rules` |
| Encoding Profiles | `/api/v1/encoding-profiles/export` | `/api/v1/export/encoding-profiles` |

Import has similar mismatch:
- Frontend: `/${type}/import` and `/${type}/import?preview=true`
- Backend: `/api/v1/import/${type}` and `/api/v1/import/${type}/preview`

**Decision**: Fix frontend API client URLs to match backend routes
**Rationale**: Backend routes follow RESTful resource organization (`/export/{type}`), frontend should align
**Alternatives Considered**:
1. Change backend routes to match frontend - rejected because backend pattern is more RESTful and consistent
2. Add alias routes in backend - rejected as it adds unnecessary complexity

**Files to Modify**:
- `frontend/src/lib/api-client.ts` (lines 1195-1365)

---

### 2. Expression Editor Reuse Pattern

**Question**: How can we reuse the existing expression editor infrastructure for client detection?

**Findings**:

The codebase has a well-structured expression editor infrastructure:

1. **Base Component**: `expression-editor.tsx`
   - Handles real-time validation via API
   - Field and operator autocomplete
   - Error highlighting with tooltips
   - Keyboard navigation

2. **Specialized Wrappers**:
   - `filter-expression-editor.tsx` - wraps base + adds source testing
   - `data-mapping-expression-editor.tsx` - wraps base + adds helper autocomplete via `useHelperAutocomplete`

3. **Shared Components**:
   - `autocomplete-popup.tsx` - displays suggestions with type badges
   - `expression-validation-badges.tsx` - shows validation status

4. **Hooks**:
   - `useHelperAutocomplete.ts` - manages `@helper:` syntax completion

**Current Client Detection Implementation** (client-detection-rules.tsx lines 272-286):
```typescript
<Textarea
  value={formData.expression}
  placeholder='user_agent contains "Chrome" AND NOT user_agent contains "Edge"'
/>
```
Plain textarea with no validation, autocomplete, or error feedback.

**Decision**: Create `client-detection-expression-editor.tsx` wrapper that:
1. Wraps `ExpressionEditor` base component
2. Integrates `useHelperAutocomplete` hook for `@dynamic()` helper
3. Uses `ValidationBadges` for status display
4. Calls validation endpoint with `domain=client_detection`

**Rationale**: Follows established pattern (FilterExpressionEditor, DataMappingExpressionEditor)
**Alternatives Considered**:
1. Modify base ExpressionEditor to handle all cases - rejected to maintain single responsibility
2. Inline all logic in client-detection-rules.tsx - rejected as it would duplicate code

---

### 3. @dynamic() Helper Configuration

**Question**: How should the `@dynamic()` helper be configured for client detection expressions?

**Findings**:

Existing helpers use this structure (`expression-constants.ts`):
```typescript
interface Helper {
  name: string;           // "dynamic"
  prefix: string;         // "@dynamic:"
  description: string;
  example: string;
  completion?: HelperCompletion;
}

interface HelperCompletion {
  type: 'search' | 'static' | 'function';
  // For static:
  options?: Array<{ label, value, description }>;
}
```

For `@dynamic()`, we need:
- Static completions for maps: `request.headers`, `request.query`, `request.path`
- Static completions for header keys: `user-agent`, `accept`, `x-forwarded-for`, etc.

**Decision**: Implement `@dynamic()` with nested static completions:
```typescript
{
  name: 'dynamic',
  prefix: '@dynamic(',
  description: 'Access request data dynamically',
  example: '@dynamic(request.headers, user-agent)',
  completion: {
    type: 'static',
    options: [
      { label: 'request.headers', value: 'request.headers', description: 'HTTP request headers' },
      { label: 'request.query', value: 'request.query', description: 'URL query parameters' },
      { label: 'request.path', value: 'request.path', description: 'URL path segments' }
    ],
    // Second-level completions for headers
    subCompletions: {
      'request.headers': [
        { label: 'user-agent', value: 'user-agent', description: 'Client User-Agent string' },
        { label: 'accept', value: 'accept', description: 'Accepted content types' },
        { label: 'x-forwarded-for', value: 'x-forwarded-for', description: 'Client IP address' }
      ]
    }
  }
}
```

**Rationale**: Static completions are simpler and don't require API calls; header keys are well-known
**Alternatives Considered**:
1. API-driven completions - rejected as header names are static and well-known
2. Free-form only - rejected as it provides no guidance to users

---

### 4. Smart Remuxing Logic

**Question**: Where and how should the relay determine remux vs transcode?

**Findings**:

Current decision flow in `routing_decision.go`:

1. `DefaultRoutingDecider.Decide()` makes routing decisions (lines 91-148)
2. Profile-based check: `profile.NeedsTranscode()` → transcode
3. Format compatibility: source format == client format → passthrough
4. Codec compatibility: `codecsCompatibleWithMPEGTS()` (lines 203-222)

Current MPEG-TS compatible codecs:
```go
mpegTSCompatible := map[string]bool{
    "h264": true, "avc": true, "avc1": true,
    "h265": true, "hevc": true, "hvc1": true, "hev1": true,
    "aac": true, "mp4a": true,
    "mp3": true,
    "ac3": true, "ec3": true, "eac3": true,
}
```

**Issue**: When source is MPEG-TS with HEVC/EAC3 and client requests HLS:
- Current: Routes to transcode because formats differ
- Expected: Should remux because HLS supports same codecs

**Decision**: Enhance `Decide()` to check codec compatibility across containers:
1. Add `codecsCompatibleWithHLS()` function
2. Add `codecsCompatibleWithContainer(container, codecs)` helper
3. Before defaulting to transcode, check if source codecs are compatible with target container
4. If compatible, return `RouteRepackage`

**HLS Codec Compatibility**:
```go
hlsCompatible := map[string]bool{
    // Video
    "h264": true, "avc": true, "avc1": true,
    "h265": true, "hevc": true, "hvc1": true, "hev1": true,
    // Audio
    "aac": true, "mp4a": true,
    "mp3": true,
    "ac3": true, "ec3": true, "eac3": true,
}
```

**Rationale**: HLS-TS segments use MPEG-TS container, so codec compatibility is same
**Alternatives Considered**:
1. Always transcode on format change - rejected as wasteful
2. Add per-format compatibility matrices - considered but simplified since HLS-TS uses MPEG-TS

**Files to Modify**:
- `internal/relay/routing_decision.go`

---

### 5. Fuzzy Search Implementation

**Question**: What fuzzy search library/approach should be used?

**Findings**:

**Current search implementation**:
- Backend: SQL `LIKE` pattern matching (`WHERE name LIKE '%term%'`)
- No typo tolerance or relevance ranking

**Options Evaluated**:

| Library | Language | Features | Performance |
|---------|----------|----------|-------------|
| Fuse.js | TypeScript | Fuzzy, weighted fields, configurable threshold | Client-side, fast for <10k items |
| pg_trgm | PostgreSQL | Trigram similarity, GIN indexes | Server-side, requires PostgreSQL |
| SQLite FTS5 | SQLite | Full-text search | Server-side, built into SQLite |
| Bleve | Go | Full-text search engine | Server-side, requires index |
| Simple LIKE | SQL | Pattern matching | Limited, no fuzzy |

**Decision**: Implement hybrid approach:
1. **Backend**: Enhance SQL search with multiple `LIKE` patterns for partial matching
2. **Frontend**: Add client-side fuzzy filtering using Fuse.js for loaded results
3. **Performance**: Server returns broader results, client refines with fuzzy matching

**Implementation**:
```typescript
// Frontend: Fuse.js configuration
const fuse = new Fuse(channels, {
  keys: [
    { name: 'channel_name', weight: 0.4 },
    { name: 'tvg_name', weight: 0.2 },
    { name: 'tvg_id', weight: 0.2 },
    { name: 'group_title', weight: 0.1 },
    { name: 'channel_number', weight: 0.05 },
    { name: 'ext_id', weight: 0.05 }
  ],
  threshold: 0.4,  // 0 = exact, 1 = match anything
  distance: 100,   // Characters to search within
  includeScore: true,
  includeMatches: true  // For highlighting matched fields
});
```

**Rationale**:
- Fuse.js is lightweight (~5KB), well-maintained, and handles typos well
- Hybrid approach works with all database backends (SQLite, PostgreSQL, MySQL)
- No backend schema changes required

**Alternatives Considered**:
1. Pure backend FTS5 - rejected because it requires SQLite-specific implementation
2. Pure client-side - rejected because it doesn't scale to 100k+ channels
3. Elasticsearch - rejected as overkill for this use case

**Files to Modify**:
- `frontend/src/app/channels/page.tsx`
- `frontend/src/app/epg/page.tsx`
- Add `frontend/src/lib/fuzzy-search.ts` utility

---

### 6. Media Player User-Agent Patterns

**Question**: What User-Agent patterns identify VLC, MPV, Kodi, Plex, Jellyfin, Emby?

**Findings**:

| Player | User-Agent Pattern | Regex |
|--------|-------------------|-------|
| **VLC** | `VLC/3.0.18 LibVLC/3.0.18` | `VLC/` or `LibVLC/` |
| **MPV** | `mpv 0.35.0` or `libmpv` | `mpv` or `libmpv` |
| **Kodi** | `Kodi/20.0` or `XBMC/` | `Kodi/` or `XBMC/` |
| **Plex** | `PlexMediaServer/` or `Plex/` | `Plex` |
| **Jellyfin** | `Jellyfin/` | `Jellyfin/` |
| **Emby** | `Emby/` or `EmbyServer/` | `Emby` |

**Decision**: Create system rules with these expressions:

```
VLC: @dynamic(request.headers, user-agent) contains "VLC" OR @dynamic(request.headers, user-agent) contains "LibVLC"
MPV: @dynamic(request.headers, user-agent) contains "mpv" OR @dynamic(request.headers, user-agent) contains "libmpv"
Kodi: @dynamic(request.headers, user-agent) contains "Kodi" OR @dynamic(request.headers, user-agent) contains "XBMC"
Plex: @dynamic(request.headers, user-agent) contains "Plex"
Jellyfin: @dynamic(request.headers, user-agent) contains "Jellyfin"
Emby: @dynamic(request.headers, user-agent) contains "Emby"
```

**Codec Configurations**:

| Player | Type | Video Codecs | Audio Codecs | Format |
|--------|------|--------------|--------------|--------|
| VLC | Direct | h264, h265 | aac, ac3, eac3, mp3 | MPEG-TS |
| MPV | Direct | h264, h265, av1, vp9 | aac, ac3, eac3, opus, mp3 | MPEG-TS |
| Kodi | Direct | h264, h265, av1, vp9 | aac, ac3, eac3, dts, mp3 | MPEG-TS |
| Plex | Server | passthrough | passthrough | source |
| Jellyfin | Server | passthrough | passthrough | source |
| Emby | Server | passthrough | passthrough | source |

**Rationale**: Direct players get specific codec lists; media servers get passthrough to avoid double-transcoding
**Files to Modify**:
- `internal/database/migrations/migration_013_system_client_detection_rules.go` (new)
- `internal/database/migrations/registry.go`

---

### 7. Copyable Expressions UI Pattern

**Question**: What's the best UX for copying expressions in the list view?

**Findings**:

Options evaluated:
1. **Click-to-copy with toast** - User clicks expression, copies to clipboard, shows success toast
2. **Copy button icon** - Small copy icon next to expression
3. **Right-click context menu** - Native browser context menu with copy option
4. **Hover tooltip with copy button** - Shows copy button on hover

**Decision**: Click-to-copy with visual feedback
- Expression displayed in `<code>` element with `cursor-pointer` style
- Click triggers `navigator.clipboard.writeText(expression)`
- Show brief "Copied!" tooltip or toast
- Add subtle visual indicator (e.g., copy icon on hover)

**Rationale**: Most intuitive; single action to copy; consistent with code block patterns
**Alternatives Considered**:
1. Right-click menu - rejected as not discoverable
2. Always-visible copy button - rejected as clutters the UI

**Implementation**:
```tsx
<code
  className="cursor-pointer hover:bg-muted px-2 py-1 rounded text-sm font-mono group"
  onClick={() => copyToClipboard(rule.expression)}
  title="Click to copy"
>
  {rule.expression}
  <Copy className="h-3 w-3 ml-1 opacity-0 group-hover:opacity-50" />
</code>
```

---

## Summary of Decisions

| Area | Decision | Key Files |
|------|----------|-----------|
| Export/Import | Fix frontend URLs to match backend `/api/v1/export/{type}` | `api-client.ts` |
| Expression Editor | Create wrapper component reusing existing infrastructure | `client-detection-expression-editor.tsx` |
| @dynamic() Helper | Static completions for maps and header keys | `expression-constants.ts` |
| Smart Remuxing | Add container codec compatibility check before routing | `routing_decision.go` |
| Fuzzy Search | Hybrid: backend LIKE + client-side Fuse.js | `channels/page.tsx`, `epg/page.tsx` |
| System Rules | Migration with 6 rules (VLC, MPV, Kodi, Plex, Jellyfin, Emby) | `migration_013_*.go` |
| Copy Expression | Click-to-copy with hover icon and toast feedback | `client-detection-rules.tsx` |
