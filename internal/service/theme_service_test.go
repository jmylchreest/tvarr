package service

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmylchreest/tvarr/internal/assets"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupThemeService(t *testing.T) *ThemeService {
	t.Helper()
	dataDir := t.TempDir()
	return NewThemeService(dataDir)
}

// requireBuiltinThemes skips the test if embedded theme assets are not available.
// This happens in CI when `go test` runs without a prior frontend build.
// We must actually try to read the directory since fs.Sub can succeed on
// a non-existent path in an embedded FS — the error only surfaces on ReadDir.
func requireBuiltinThemes(t *testing.T) {
	t.Helper()
	themesFS, err := assets.GetThemesFS()
	if err != nil {
		t.Skip("embedded theme assets not available (frontend not built)")
	}
	if _, err := fs.ReadDir(themesFS, "."); err != nil {
		t.Skip("embedded theme assets not available (frontend not built)")
	}
}

func TestThemeService_ListThemes_BuiltinThemesReturned(t *testing.T) {
	requireBuiltinThemes(t)
	svc := setupThemeService(t)
	ctx := context.Background()

	resp, err := svc.ListThemes(ctx)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// There should be at least the built-in themes from assets/static/themes/
	// themes.json lists 11 themes: graphite, amber, amethyst, burnt-amber, candyland,
	// catppuccin, claymorphism, mocha, mono, nature, tangerine
	assert.GreaterOrEqual(t, len(resp.Themes), 11)

	// All built-in themes should have source == builtin
	for _, theme := range resp.Themes {
		if theme.Source == models.ThemeSourceBuiltin {
			assert.NotEmpty(t, theme.ID)
			assert.NotEmpty(t, theme.Name)
		}
	}
}

func TestThemeService_ListThemes_DefaultIsGraphite(t *testing.T) {
	svc := setupThemeService(t)
	ctx := context.Background()

	resp, err := svc.ListThemes(ctx)
	require.NoError(t, err)
	assert.Equal(t, "graphite", resp.Default)
}

func TestThemeService_GetThemeCSS_BuiltinTheme(t *testing.T) {
	requireBuiltinThemes(t)
	svc := setupThemeService(t)
	ctx := context.Background()

	content, source, _, err := svc.GetThemeCSS(ctx, "graphite")
	require.NoError(t, err)
	assert.Equal(t, models.ThemeSourceBuiltin, source)
	assert.NotEmpty(t, content)
	assert.Contains(t, string(content), ":root")
	assert.Contains(t, string(content), ".dark")
}

func TestThemeService_GetThemeCSS_NotFound(t *testing.T) {
	svc := setupThemeService(t)
	ctx := context.Background()

	_, _, _, err := svc.GetThemeCSS(ctx, "nonexistent-theme-xyz")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "theme not found")
}

func TestThemeService_GetThemeCSS_InvalidID(t *testing.T) {
	svc := setupThemeService(t)
	ctx := context.Background()

	_, _, _, err := svc.GetThemeCSS(ctx, "../etc/passwd")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid theme ID format")
}

func TestThemeService_GetThemeCSS_CustomThemeOverridesBuiltin(t *testing.T) {
	dataDir := t.TempDir()
	svc := NewThemeService(dataDir)
	ctx := context.Background()

	// Create a custom themes directory with a theme that has the same name as a builtin
	themesDir := filepath.Join(dataDir, "themes")
	require.NoError(t, os.MkdirAll(themesDir, 0750))

	customCSS := []byte(`:root {
  --background: #custom;
  --foreground: #000;
  --primary: #123;
}
.dark {
  --background: #custom-dark;
  --foreground: #fff;
  --primary: #456;
}`)
	require.NoError(t, os.WriteFile(filepath.Join(themesDir, "graphite.css"), customCSS, 0644))

	content, source, _, err := svc.GetThemeCSS(ctx, "graphite")
	require.NoError(t, err)
	assert.Equal(t, models.ThemeSourceCustom, source)
	assert.Contains(t, string(content), "#custom")
}

func TestThemeService_ListThemes_IncludesCustomThemes(t *testing.T) {
	dataDir := t.TempDir()
	svc := NewThemeService(dataDir)
	ctx := context.Background()

	// Create a custom themes directory with a valid theme
	themesDir := filepath.Join(dataDir, "themes")
	require.NoError(t, os.MkdirAll(themesDir, 0750))

	customCSS := []byte(`:root {
  --background: #fff;
  --foreground: #000;
  --primary: #123;
}
.dark {
  --background: #000;
  --foreground: #fff;
  --primary: #456;
}`)
	require.NoError(t, os.WriteFile(filepath.Join(themesDir, "my-custom-theme.css"), customCSS, 0644))

	resp, err := svc.ListThemes(ctx)
	require.NoError(t, err)

	// Find the custom theme
	var found bool
	for _, theme := range resp.Themes {
		if theme.ID == "my-custom-theme" {
			found = true
			assert.Equal(t, models.ThemeSourceCustom, theme.Source)
			assert.Equal(t, "My Custom Theme", theme.Name)
			break
		}
	}
	assert.True(t, found, "custom theme should be listed")
}

func TestThemeService_ListThemes_SkipsInvalidCustomThemes(t *testing.T) {
	dataDir := t.TempDir()
	svc := NewThemeService(dataDir)
	ctx := context.Background()

	themesDir := filepath.Join(dataDir, "themes")
	require.NoError(t, os.MkdirAll(themesDir, 0750))

	// Write an invalid theme (missing .dark block)
	invalidCSS := []byte(`:root {
  --background: #fff;
  --foreground: #000;
  --primary: #123;
}`)
	require.NoError(t, os.WriteFile(filepath.Join(themesDir, "invalid-theme.css"), invalidCSS, 0644))

	resp, err := svc.ListThemes(ctx)
	require.NoError(t, err)

	for _, theme := range resp.Themes {
		assert.NotEqual(t, "invalid-theme", theme.ID, "invalid theme should not be listed")
	}
}

func TestThemeService_ListThemes_NoCustomThemesDirectory(t *testing.T) {
	requireBuiltinThemes(t)
	dataDir := t.TempDir()
	svc := NewThemeService(dataDir)
	ctx := context.Background()

	// No themes directory created - should still work with just builtins
	resp, err := svc.ListThemes(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(resp.Themes), 1)
}

func TestThemeService_ValidateTheme_ValidCSS(t *testing.T) {
	svc := setupThemeService(t)

	validCSS := []byte(`:root {
  --background: #fff;
  --foreground: #000;
  --primary: #123;
}
.dark {
  --background: #000;
  --foreground: #fff;
  --primary: #456;
}`)

	err := svc.ValidateTheme(validCSS)
	assert.NoError(t, err)
}

func TestThemeService_ValidateTheme_MissingRootBlock(t *testing.T) {
	svc := setupThemeService(t)

	css := []byte(`.dark {
  --background: #000;
  --foreground: #fff;
  --primary: #456;
}`)

	err := svc.ValidateTheme(css)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing :root block")
}

func TestThemeService_ValidateTheme_MissingDarkBlock(t *testing.T) {
	svc := setupThemeService(t)

	css := []byte(`:root {
  --background: #fff;
  --foreground: #000;
  --primary: #123;
}`)

	err := svc.ValidateTheme(css)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing .dark block")
}

func TestThemeService_ValidateTheme_MissingRequiredVariable(t *testing.T) {
	svc := setupThemeService(t)

	// Missing --primary in :root
	css := []byte(`:root {
  --background: #fff;
  --foreground: #000;
}
.dark {
  --background: #000;
  --foreground: #fff;
  --primary: #456;
}`)

	err := svc.ValidateTheme(css)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--primary")
}

func TestThemeService_ValidateTheme_MissingRequiredVariableInDark(t *testing.T) {
	svc := setupThemeService(t)

	// Missing --foreground in .dark
	css := []byte(`:root {
  --background: #fff;
  --foreground: #000;
  --primary: #123;
}
.dark {
  --background: #000;
  --primary: #456;
}`)

	err := svc.ValidateTheme(css)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--foreground")
}

func TestThemeService_FormatThemeName(t *testing.T) {
	svc := setupThemeService(t)

	tests := []struct {
		id       string
		expected string
	}{
		{"graphite", "Graphite"},
		{"burnt-amber", "Burnt Amber"},
		{"my_custom_theme", "My Custom Theme"},
		{"mono", "Mono"},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			result := svc.formatThemeName(tt.id)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestThemeService_ExtractColors(t *testing.T) {
	svc := setupThemeService(t)

	css := []byte(`:root {
  --background: oklch(0.95 0 0);
  --foreground: oklch(0.32 0 0);
  --primary: oklch(0.49 0 0);
  --secondary: oklch(0.90 0 0);
  --accent: oklch(0.80 0 0);
}
.dark {
  --background: oklch(0.22 0 0);
  --foreground: oklch(0.89 0 0);
  --primary: oklch(0.71 0 0);
  --secondary: oklch(0.31 0 0);
  --accent: oklch(0.37 0 0);
}`)

	colors := svc.extractColors(css)
	require.NotNil(t, colors)

	assert.Equal(t, "oklch(0.95 0 0)", colors.Light.Background)
	assert.Equal(t, "oklch(0.32 0 0)", colors.Light.Foreground)
	assert.Equal(t, "oklch(0.49 0 0)", colors.Light.Primary)
	assert.Equal(t, "oklch(0.90 0 0)", colors.Light.Secondary)
	assert.Equal(t, "oklch(0.80 0 0)", colors.Light.Accent)

	assert.Equal(t, "oklch(0.22 0 0)", colors.Dark.Background)
	assert.Equal(t, "oklch(0.89 0 0)", colors.Dark.Foreground)
	assert.Equal(t, "oklch(0.71 0 0)", colors.Dark.Primary)
}

func TestThemeService_EnsureThemesDirectory(t *testing.T) {
	dataDir := t.TempDir()
	svc := NewThemeService(dataDir)

	err := svc.EnsureThemesDirectory()
	require.NoError(t, err)

	// Verify directory was created
	themesDir := filepath.Join(dataDir, "themes")
	info, err := os.Stat(themesDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestThemeService_EnsureThemesDirectory_AlreadyExists(t *testing.T) {
	dataDir := t.TempDir()
	svc := NewThemeService(dataDir)

	themesDir := filepath.Join(dataDir, "themes")
	require.NoError(t, os.MkdirAll(themesDir, 0750))

	// Should not error when directory already exists
	err := svc.EnsureThemesDirectory()
	assert.NoError(t, err)
}

func TestThemeService_GetThemeCSS_AllBuiltinThemes(t *testing.T) {
	requireBuiltinThemes(t)
	svc := setupThemeService(t)
	ctx := context.Background()

	resp, err := svc.ListThemes(ctx)
	require.NoError(t, err)

	for _, theme := range resp.Themes {
		if theme.Source != models.ThemeSourceBuiltin {
			continue
		}
		t.Run(theme.ID, func(t *testing.T) {
			content, source, _, err := svc.GetThemeCSS(ctx, theme.ID)
			require.NoError(t, err)
			assert.Equal(t, models.ThemeSourceBuiltin, source)
			assert.NotEmpty(t, content)
		})
	}
}

func TestThemeService_ListThemes_BuiltinThemesHaveColors(t *testing.T) {
	requireBuiltinThemes(t)
	svc := setupThemeService(t)
	ctx := context.Background()

	resp, err := svc.ListThemes(ctx)
	require.NoError(t, err)

	for _, theme := range resp.Themes {
		if theme.Source != models.ThemeSourceBuiltin {
			continue
		}
		t.Run(theme.ID, func(t *testing.T) {
			require.NotNil(t, theme.Colors, "theme %s should have colors extracted", theme.ID)
			assert.NotEmpty(t, theme.Colors.Light.Background, "theme %s should have light background", theme.ID)
			assert.NotEmpty(t, theme.Colors.Dark.Background, "theme %s should have dark background", theme.ID)
		})
	}
}
