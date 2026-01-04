# Research: Frontend Theme Polish

**Feature**: 011-frontend-theme-polish
**Date**: 2025-12-08

## Research Topics

### 1. FOUC (Flash of Unstyled Content) Prevention in Next.js

**Decision**: Use synchronous inline script in `<head>` that reads localStorage and applies theme class + CSS before React hydration.

**Rationale**:
- Next.js App Router renders server-side first, then hydrates client-side
- The existing `enhancedThemeScript` approach is correct but may have timing issues
- The script must execute synchronously before any content paints
- CSS must be linked before the script runs to have variables available

**Alternatives Considered**:
1. **CSS-only solution with `prefers-color-scheme`**: Rejected because it doesn't support custom color palettes, only light/dark toggle
2. **Server-side theme detection via cookies**: Rejected because it requires server round-trip and adds complexity
3. **React Suspense boundaries**: Rejected because React hydration happens after first paint

**Implementation Pattern**:
```html
<html class="dark">  <!-- Set by inline script before body renders -->
  <head>
    <link rel="stylesheet" href="/themes/graphite.css" />  <!-- Loaded first -->
    <script>/* synchronous theme application */</script>
  </head>
  <body class="bg-background">...</body>
</html>
```

### 2. Custom Theme CSS Validation

**Decision**: Validate custom themes by checking for required CSS variables using regex pattern matching on file content.

**Rationale**:
- Full CSS parsing is overkill for validation
- Required variables (--background, --foreground, --primary) are sufficient to ensure theme works
- Invalid themes should fail gracefully without breaking the app

**Alternatives Considered**:
1. **Full CSS AST parsing**: Rejected as over-engineered; simple regex is sufficient
2. **No validation**: Rejected because malformed themes could break UI
3. **Runtime validation in browser**: Rejected because it delays theme loading

**Required Variables (minimum)**:
```css
:root {
  --background: <color>;
  --foreground: <color>;
  --primary: <color>;
}
.dark {
  --background: <color>;
  --foreground: <color>;
  --primary: <color>;
}
```

### 3. Theme File Caching Strategy

**Decision**: Use HTTP ETag headers based on file modification time for cache validation.

**Rationale**:
- File modification time is easy to detect and compare
- ETags allow browser to cache but revalidate on refresh
- No manual cache busting needed when user edits theme file

**Alternatives Considered**:
1. **Content hash-based ETags**: More accurate but requires reading entire file
2. **Query string versioning**: Requires tracking version separately
3. **No caching**: Poor performance, unnecessary network traffic

**HTTP Headers**:
```
Cache-Control: public, max-age=3600, must-revalidate
ETag: "mtime-1701000000"
Last-Modified: Mon, 26 Nov 2024 12:00:00 GMT
```

### 4. Theme API Endpoint Design

**Decision**: Add `/api/v1/themes` endpoint to list all themes and `/api/v1/themes/{id}.css` to serve theme CSS.

**Rationale**:
- Separating list from CSS serving allows efficient theme enumeration
- Backend can merge built-in and custom themes transparently
- CSS serving endpoint can add proper caching headers

**Alternatives Considered**:
1. **Single endpoint returning themes with embedded CSS**: Rejected because CSS files are large
2. **Frontend fetching directly from $DATA directory**: Rejected because frontend can't access backend filesystem
3. **Embedding all themes in themes.json**: Rejected because it bloats the JSON response

### 5. Client-Side Navigation Theme Persistence

**Decision**: Ensure Next.js App Router preserves theme state during client-side navigation by:
1. Setting `class="dark"` on `<html>` element (not body)
2. Loading theme CSS as early as possible in document head
3. Using CSS custom properties that cascade to all elements

**Rationale**:
- The flashbang occurs because new page content renders before theme CSS applies
- By ensuring theme class is on `<html>` and CSS is in `<head>`, new content inherits styles immediately
- Next.js client navigation doesn't re-run `<head>` scripts, so theme persists

**Root Cause Analysis**:
The current implementation has the inline script loading theme CSS dynamically, which means:
1. Page navigates (client-side)
2. New page content renders with default (white) background
3. Theme CSS eventually loads and applies

**Solution**:
1. Load theme CSS statically in `<head>` (not dynamically via JavaScript)
2. Use `suppressHydrationWarning` to prevent React from removing the `dark` class
3. Ensure all pages use `bg-background` which reads from CSS variable

### 6. Component Consistency Audit Approach

**Decision**: Create a component style guide checklist and audit existing pages for:
- Button variants (size, padding, colors)
- Input fields (height, border style, focus states)
- Cards (padding, shadows, border radius)
- Dialogs (backdrop, padding, animation)

**Rationale**:
- shadcn/ui provides consistent base components
- Inconsistencies likely come from custom overrides or variant misuse
- Audit identifies specific fixes rather than wholesale rewrites

**Audit Scope**:
1. All pages in `frontend/src/app/`
2. All components in `frontend/src/components/`
3. Focus on: Button, Input, Card, Dialog, Sheet, Badge

## Technology Decisions

| Decision | Choice | Justification |
|----------|--------|---------------|
| Theme persistence | localStorage | No server round-trip, works offline |
| CSS loading | Static `<link>` tag | Faster than dynamic injection |
| Theme validation | Regex pattern matching | Simple, fast, sufficient |
| Cache strategy | ETag with mtime | Browser cache with freshness check |
| Dark mode class | On `<html>` element | CSS inheritance works correctly |

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| CSP blocks inline scripts | High | Use nonce-based CSP or move to external script |
| Large custom theme file | Medium | Add max file size validation (100KB) |
| Theme file parsing error | Low | Graceful fallback to default theme |
| Browser doesn't support CSS variables | Low | All modern browsers support; no fallback needed |

## Open Questions (Resolved)

1. ~~What CSS variables are required for a valid theme?~~ → --background, --foreground, --primary (minimum)
2. ~~How to handle theme updates without page refresh?~~ → Detected on theme selector open or page refresh
3. ~~Where should custom themes be stored?~~ → `$DATA/themes/` directory
