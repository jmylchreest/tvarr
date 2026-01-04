# Quickstart: Frontend Theme Polish

**Feature**: 011-frontend-theme-polish
**Date**: 2025-12-08

## Overview

This guide covers the implementation approach for fixing the theme flashbang issue, adding custom theme support, and ensuring component consistency.

## Implementation Phases

### Phase A: Fix FOUC/Flashbang (P1)

The flashbang occurs because the theme CSS is loaded dynamically after React renders. Fix this by:

1. **Modify `layout.tsx`** to set background color on `<html>` element via inline style
2. **Modify `enhanced-theme-script.ts`** to be synchronous and execute before body renders
3. **Ensure theme CSS `<link>` tag is in `<head>`** before the inline script

**Key Changes**:

```tsx
// frontend/src/app/layout.tsx
export default function RootLayout({ children }) {
  return (
    <html lang="en" suppressHydrationWarning style={{ backgroundColor: 'var(--background)' }}>
      <head>
        {/* Theme CSS loaded first */}
        <link id="theme-css" rel="stylesheet" href="/themes/graphite.css" />
        {/* Then theme script runs synchronously */}
        <script dangerouslySetInnerHTML={{ __html: enhancedThemeScript }} />
      </head>
      <body className="bg-background text-foreground">
        <EnhancedThemeProvider>
          {children}
        </EnhancedThemeProvider>
      </body>
    </html>
  );
}
```

```typescript
// frontend/src/lib/enhanced-theme-script.ts
export const enhancedThemeScript = `
(function() {
  // Read preferences synchronously
  var theme = localStorage.getItem('theme') || 'graphite';
  var mode = localStorage.getItem('mode') || 'system';

  // Determine actual mode
  var actualMode = mode === 'system'
    ? (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
    : mode;

  // Apply dark class immediately
  if (actualMode === 'dark') {
    document.documentElement.classList.add('dark');
  }

  // Update theme CSS link href
  var link = document.getElementById('theme-css');
  if (link) {
    link.href = '/themes/' + theme + '.css';
  }
})();
`;
```

### Phase B: Custom Theme Backend (P2)

1. **Create `internal/service/theme_service.go`**:
   - Scan `$DATA/themes/` for .css files
   - Validate required CSS variables
   - Extract color values for previews
   - Merge with built-in themes

2. **Create `internal/http/handlers/theme.go`**:
   - `GET /api/v1/themes` - List all themes
   - `GET /api/v1/themes/{id}.css` - Serve theme CSS with caching headers

**Key Implementation**:

```go
// internal/service/theme_service.go
type ThemeService struct {
    dataDir      string
    builtinDir   embed.FS
    logger       *slog.Logger
}

func (s *ThemeService) ListThemes(ctx context.Context) ([]Theme, error) {
    themes := make([]Theme, 0)

    // Load built-in themes from embedded FS
    builtins, _ := s.loadBuiltinThemes()
    themes = append(themes, builtins...)

    // Load custom themes from data directory
    customs, _ := s.loadCustomThemes()
    themes = append(themes, customs...)

    return themes, nil
}

func (s *ThemeService) ValidateTheme(css []byte) error {
    content := string(css)

    // Check for :root block with required vars
    rootMatch := rootBlockPattern.FindStringSubmatch(content)
    if rootMatch == nil {
        return errors.New("missing :root block")
    }

    for _, v := range requiredVars {
        if !strings.Contains(rootMatch[1], v+":") {
            return fmt.Errorf("missing required variable %s in :root", v)
        }
    }

    // Same check for .dark block
    // ...

    return nil
}
```

### Phase C: Frontend Custom Theme Support (P2)

1. **Modify `enhanced-theme-provider.tsx`**:
   - Fetch themes from `/api/v1/themes` instead of static JSON
   - Handle custom themes with "Custom" label
   - Fall back to static files if API unavailable

2. **Modify `enhanced-theme-selector.tsx`**:
   - Display custom themes with visual distinction
   - Show "Custom" badge for user themes

### Phase D: Component Consistency Audit (P3)

1. **Audit all pages** for component usage
2. **Standardize variants**:
   - Primary buttons: `variant="default"` with consistent size
   - Inputs: Use shadcn Input component with no custom styling
   - Cards: Use Card component with standard padding
   - Dialogs: Use Dialog with standard backdrop

**Checklist**:
- [ ] Dashboard page buttons
- [ ] Sources page forms
- [ ] Proxies page tables
- [ ] Settings page inputs
- [ ] All dialog/modal components

### Phase E: Theme Selector UI Enhancement (P4)

1. **Add visual color swatches** (already implemented)
2. **Add "Custom" label** for custom themes
3. **Group themes** by source (Built-in / Custom)

## Testing Approach

### Visual Testing
1. Navigate between pages in dark mode - no white flash
2. Refresh page in dark mode - dark background from first paint
3. Test with browser DevTools throttled to Slow 3G

### API Testing
```bash
# List themes
curl http://localhost:8080/api/v1/themes | jq

# Get theme CSS
curl -I http://localhost:8080/api/v1/themes/graphite.css
# Check ETag and Cache-Control headers

# Test custom theme
mkdir -p $DATA/themes
cat > $DATA/themes/test-theme.css << 'EOF'
:root {
  --background: #ffffff;
  --foreground: #000000;
  --primary: #0066cc;
}
.dark {
  --background: #1a1a1a;
  --foreground: #ffffff;
  --primary: #66aaff;
}
EOF

# Verify custom theme appears
curl http://localhost:8080/api/v1/themes | jq '.themes[] | select(.source == "custom")'
```

### Component Audit Process
1. Open each page in browser
2. Screenshot all buttons, inputs, cards
3. Compare padding, sizes, colors
4. Document inconsistencies
5. Fix by using standard shadcn variants

## Dependencies

### Backend
- No new dependencies required
- Uses existing embed.FS for built-in themes

### Frontend
- No new dependencies required
- Uses existing shadcn/ui components

## Migration Steps

1. Deploy backend with theme API (no breaking changes)
2. Deploy frontend with FOUC fix
3. Users can add custom themes to `$DATA/themes/`
4. No data migration required
