# Data Model: Frontend Theme Polish

**Feature**: 011-frontend-theme-polish
**Date**: 2025-12-08

## Entities

### Theme (Backend)

Represents a color theme that can be applied to the UI. Themes can be built-in (embedded in the binary) or custom (loaded from `$DATA/themes/`).

```go
// Theme represents a UI color theme
type Theme struct {
    ID          string       `json:"id"`           // Unique identifier (filename without .css)
    Name        string       `json:"name"`         // Display name (derived from ID or metadata)
    Description string       `json:"description"`  // Optional description
    Source      ThemeSource  `json:"source"`       // "builtin" or "custom"
    ModifiedAt  time.Time    `json:"modified_at"`  // File modification time (for caching)
    Colors      *ThemeColors `json:"colors,omitempty"` // Extracted color values
}

// ThemeSource indicates where the theme comes from
type ThemeSource string

const (
    ThemeSourceBuiltin ThemeSource = "builtin"
    ThemeSourceCustom  ThemeSource = "custom"
)

// ThemeColors holds extracted color values for preview
type ThemeColors struct {
    Light ThemePalette `json:"light"`
    Dark  ThemePalette `json:"dark"`
}

// ThemePalette holds color values for a single mode
type ThemePalette struct {
    Background string `json:"background"`
    Foreground string `json:"foreground"`
    Primary    string `json:"primary"`
    Secondary  string `json:"secondary"`
    Accent     string `json:"accent"`
}
```

### ThemeDefinition (Frontend)

TypeScript interface for theme data in the frontend.

```typescript
interface ThemeDefinition {
  id: string;
  name: string;
  description?: string;
  source: 'builtin' | 'custom';
  modifiedAt?: string;
  colors?: {
    light: ThemePalette;
    dark: ThemePalette;
  };
}

interface ThemePalette {
  background: string;
  foreground: string;
  primary: string;
  secondary?: string;
  accent?: string;
}
```

### UserThemePreference (Frontend - localStorage)

User's theme preference stored in browser localStorage.

```typescript
// Stored as separate keys for simplicity
localStorage.setItem('theme', 'graphite');     // Theme ID
localStorage.setItem('mode', 'dark');          // 'light' | 'dark' | 'system'
```

## Validation Rules

### Theme CSS File Validation

A valid theme CSS file must contain:

1. **Required variables in `:root` selector**:
   - `--background`
   - `--foreground`
   - `--primary`

2. **Required variables in `.dark` selector**:
   - `--background`
   - `--foreground`
   - `--primary`

3. **File constraints**:
   - Maximum size: 100KB
   - File extension: `.css`
   - Filename: alphanumeric, hyphens, underscores only

### Validation Regex Patterns

```go
var (
    // Required CSS variables (must appear in both :root and .dark)
    requiredVars = []string{"--background", "--foreground", "--primary"}

    // Pattern to extract :root block
    rootBlockPattern = regexp.MustCompile(`(?s):root\s*\{([^}]+)\}`)

    // Pattern to extract .dark block
    darkBlockPattern = regexp.MustCompile(`(?s)\.dark\s*\{([^}]+)\}`)

    // Pattern to check if variable is defined
    varPattern = regexp.MustCompile(`--[\w-]+:\s*[^;]+;`)

    // Valid filename pattern
    filenamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+\.css$`)
)
```

## State Transitions

### Theme Loading State (Frontend)

```
┌─────────────┐
│   INITIAL   │ ← Page load starts
└──────┬──────┘
       │ Inline script reads localStorage
       ▼
┌─────────────┐
│  APPLYING   │ ← Theme class set on <html>
└──────┬──────┘
       │ Theme CSS loaded
       ▼
┌─────────────┐
│   READY     │ ← React hydration complete
└──────┬──────┘
       │ User changes theme
       ▼
┌─────────────┐
│  SWITCHING  │ ← New theme CSS loading
└──────┬──────┘
       │ CSS applied
       ▼
┌─────────────┐
│   READY     │
└─────────────┘
```

### Theme Validation State (Backend)

```
┌─────────────┐
│  SCANNING   │ ← Reading $DATA/themes/ directory
└──────┬──────┘
       │ For each .css file
       ▼
┌─────────────┐
│ VALIDATING  │ ← Checking required variables
└──────┬──────┘
       │
   ┌───┴───┐
   │       │
   ▼       ▼
┌──────┐ ┌──────────┐
│ VALID│ │ INVALID  │
└──┬───┘ └────┬─────┘
   │          │ Log warning, skip file
   ▼          ▼
┌─────────────┐
│  AVAILABLE  │ ← Added to theme list
└─────────────┘
```

## Relationships

```
┌─────────────────────────────────────────────────────┐
│                    Backend                          │
│  ┌─────────────┐       ┌─────────────────────────┐ │
│  │ ThemeService│──────▶│ $DATA/themes/*.css      │ │
│  │             │       │ (custom themes)          │ │
│  └──────┬──────┘       └─────────────────────────┘ │
│         │                                           │
│         │ merges with                               │
│         ▼                                           │
│  ┌─────────────────────────────────────────────┐   │
│  │ internal/assets/themes/*.css (built-in)     │   │
│  └─────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────┘
                         │
                         │ API: GET /api/v1/themes
                         ▼
┌─────────────────────────────────────────────────────┐
│                    Frontend                         │
│  ┌─────────────────┐     ┌────────────────────────┐│
│  │EnhancedTheme    │────▶│ ThemeDefinition[]      ││
│  │Provider         │     │ (theme list)           ││
│  └────────┬────────┘     └────────────────────────┘│
│           │                                         │
│           │ stores preference                       │
│           ▼                                         │
│  ┌─────────────────────────────────────────────┐   │
│  │ localStorage: theme="graphite", mode="dark" │   │
│  └─────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────┘
```

## Data Volume Assumptions

| Data Type | Expected Volume | Storage Location |
|-----------|-----------------|------------------|
| Built-in themes | 10-15 themes | Embedded in binary |
| Custom themes | 0-50 themes | $DATA/themes/ |
| Theme CSS size | 3-10 KB each | File system |
| localStorage | ~100 bytes | Browser |

## Migration Notes

No database migration required. This feature uses:
- File system for custom themes
- Browser localStorage for preferences
- Embedded assets for built-in themes

Existing user preferences in localStorage will continue to work as the storage keys remain unchanged.
