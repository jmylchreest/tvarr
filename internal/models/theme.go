package models

import (
	"time"
)

// ThemeSource indicates where the theme comes from.
type ThemeSource string

const (
	// ThemeSourceBuiltin indicates a built-in theme embedded in the binary.
	ThemeSourceBuiltin ThemeSource = "builtin"
	// ThemeSourceCustom indicates a user-provided theme from $DATA/themes/.
	ThemeSourceCustom ThemeSource = "custom"
)

// ThemePalette holds color values for a single mode (light or dark).
type ThemePalette struct {
	Background string `json:"background"`
	Foreground string `json:"foreground"`
	Primary    string `json:"primary"`
	Secondary  string `json:"secondary,omitempty"`
	Accent     string `json:"accent,omitempty"`
}

// ThemeColors holds extracted color values for preview in both modes.
type ThemeColors struct {
	Light ThemePalette `json:"light"`
	Dark  ThemePalette `json:"dark"`
}

// Theme represents a UI color theme.
type Theme struct {
	// ID is the unique identifier (filename without .css extension).
	ID string `json:"id"`

	// Name is the human-readable display name.
	Name string `json:"name"`

	// Description provides additional context about the theme.
	Description string `json:"description,omitempty"`

	// Source indicates whether this is a builtin or custom theme.
	Source ThemeSource `json:"source"`

	// ModifiedAt is the file modification time (for custom themes and caching).
	ModifiedAt time.Time `json:"modified_at"`

	// Colors contains extracted color values for theme previews.
	Colors *ThemeColors `json:"colors,omitempty"`
}

// ThemeListResponse is the API response for listing themes.
type ThemeListResponse struct {
	Themes  []Theme `json:"themes"`
	Default string  `json:"default"`
}

// ThemeMetadata represents theme info from themes.json.
type ThemeMetadata struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ThemesJSON represents the structure of themes.json.
type ThemesJSON struct {
	Themes  []ThemeMetadata `json:"themes"`
	Default string          `json:"default"`
}

// RequiredCSSVariables lists the minimum CSS variables a valid theme must define.
var RequiredCSSVariables = []string{
	"--background",
	"--foreground",
	"--primary",
}

// ThemeFileMaxSize is the maximum allowed size for custom theme CSS files (100KB).
const ThemeFileMaxSize int64 = 100 * 1024
