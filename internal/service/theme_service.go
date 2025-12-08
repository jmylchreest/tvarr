package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jmylchreest/tvarr/internal/assets"
	"github.com/jmylchreest/tvarr/internal/models"
)

var (
	// rootBlockPattern extracts the :root block content from CSS.
	rootBlockPattern = regexp.MustCompile(`(?s):root\s*\{([^}]+)\}`)
	// darkBlockPattern extracts the .dark block content from CSS.
	darkBlockPattern = regexp.MustCompile(`(?s)\.dark\s*\{([^}]+)\}`)
	// varValuePattern extracts CSS variable name and value.
	varValuePattern = regexp.MustCompile(`(--[\w-]+)\s*:\s*([^;]+);`)
	// filenamePattern validates theme filename.
	filenamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+\.css$`)
)

// ThemeService provides theme management functionality.
type ThemeService struct {
	dataDir string
	logger  *slog.Logger
}

// NewThemeService creates a new theme service.
func NewThemeService(dataDir string) *ThemeService {
	return &ThemeService{
		dataDir: dataDir,
		logger:  slog.Default(),
	}
}

// WithLogger sets the logger for the service.
func (s *ThemeService) WithLogger(logger *slog.Logger) *ThemeService {
	s.logger = logger
	return s
}

// ListThemes returns all available themes (built-in and custom).
func (s *ThemeService) ListThemes(ctx context.Context) (*models.ThemeListResponse, error) {
	themes := make([]models.Theme, 0)
	defaultTheme := "graphite"

	// Load built-in themes from embedded filesystem
	builtinThemes, builtinDefault, err := s.loadBuiltinThemes()
	if err != nil {
		s.logger.WarnContext(ctx, "failed to load built-in themes", slog.String("error", err.Error()))
	} else {
		themes = append(themes, builtinThemes...)
		if builtinDefault != "" {
			defaultTheme = builtinDefault
		}
	}

	// Load custom themes from data directory
	customThemes, err := s.loadCustomThemes(ctx)
	if err != nil {
		s.logger.WarnContext(ctx, "failed to load custom themes", slog.String("error", err.Error()))
	} else {
		themes = append(themes, customThemes...)
	}

	return &models.ThemeListResponse{
		Themes:  themes,
		Default: defaultTheme,
	}, nil
}

// GetThemeCSS returns the CSS content for a theme.
func (s *ThemeService) GetThemeCSS(ctx context.Context, themeID string) ([]byte, models.ThemeSource, time.Time, error) {
	// Validate theme ID format
	if !filenamePattern.MatchString(themeID + ".css") {
		return nil, "", time.Time{}, fmt.Errorf("invalid theme ID format: %s", themeID)
	}

	// Try custom themes first (user themes override built-in)
	customPath := s.customThemePath(themeID)
	if customPath != "" {
		info, err := os.Stat(customPath)
		if err == nil {
			content, err := os.ReadFile(customPath)
			if err == nil {
				return content, models.ThemeSourceCustom, info.ModTime(), nil
			}
		}
	}

	// Try built-in themes from embedded filesystem
	themesFS, err := assets.GetThemesFS()
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("failed to access built-in themes: %w", err)
	}

	content, err := fs.ReadFile(themesFS, themeID+".css")
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("theme not found: %s", themeID)
	}

	return content, models.ThemeSourceBuiltin, time.Time{}, nil
}

// ValidateTheme validates a theme CSS file for required variables.
func (s *ThemeService) ValidateTheme(css []byte) error {
	content := string(css)

	// Check :root block
	rootMatch := rootBlockPattern.FindStringSubmatch(content)
	if rootMatch == nil {
		return fmt.Errorf("missing :root block")
	}

	// Check .dark block
	darkMatch := darkBlockPattern.FindStringSubmatch(content)
	if darkMatch == nil {
		return fmt.Errorf("missing .dark block")
	}

	// Check required variables in :root
	for _, varName := range models.RequiredCSSVariables {
		if !strings.Contains(rootMatch[1], varName+":") {
			return fmt.Errorf("missing required variable %s in :root", varName)
		}
	}

	// Check required variables in .dark
	for _, varName := range models.RequiredCSSVariables {
		if !strings.Contains(darkMatch[1], varName+":") {
			return fmt.Errorf("missing required variable %s in .dark", varName)
		}
	}

	return nil
}

// loadBuiltinThemes loads themes from the embedded filesystem.
func (s *ThemeService) loadBuiltinThemes() ([]models.Theme, string, error) {
	themesFS, err := assets.GetThemesFS()
	if err != nil {
		return nil, "", fmt.Errorf("failed to access built-in themes: %w", err)
	}

	// Load themes.json for metadata
	metadataMap := make(map[string]models.ThemeMetadata)
	defaultTheme := "graphite"

	jsonData, err := fs.ReadFile(themesFS, "themes.json")
	if err == nil {
		var themesJSON models.ThemesJSON
		if err := json.Unmarshal(jsonData, &themesJSON); err == nil {
			for _, m := range themesJSON.Themes {
				metadataMap[m.ID] = m
			}
			if themesJSON.Default != "" {
				defaultTheme = themesJSON.Default
			}
		}
	}

	// Scan for CSS files
	themes := make([]models.Theme, 0)
	entries, err := fs.ReadDir(themesFS, ".")
	if err != nil {
		return nil, defaultTheme, fmt.Errorf("failed to read themes directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".css") {
			continue
		}

		themeID := strings.TrimSuffix(entry.Name(), ".css")
		theme := models.Theme{
			ID:     themeID,
			Name:   s.formatThemeName(themeID),
			Source: models.ThemeSourceBuiltin,
		}

		// Apply metadata if available
		if meta, ok := metadataMap[themeID]; ok {
			theme.Name = meta.Name
			theme.Description = meta.Description
		}

		// Extract colors for preview
		cssContent, err := fs.ReadFile(themesFS, entry.Name())
		if err == nil {
			theme.Colors = s.extractColors(cssContent)
		}

		themes = append(themes, theme)
	}

	return themes, defaultTheme, nil
}

// loadCustomThemes loads themes from the custom themes directory.
func (s *ThemeService) loadCustomThemes(ctx context.Context) ([]models.Theme, error) {
	themesDir := filepath.Join(s.dataDir, "themes")

	// Check if directory exists
	info, err := os.Stat(themesDir)
	if os.IsNotExist(err) {
		return nil, nil // No custom themes directory
	}
	if err != nil {
		return nil, fmt.Errorf("failed to access themes directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("themes path is not a directory")
	}

	// Scan for CSS files
	themes := make([]models.Theme, 0)
	entries, err := os.ReadDir(themesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read themes directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".css") {
			continue
		}

		// Validate filename
		if !filenamePattern.MatchString(entry.Name()) {
			s.logger.WarnContext(ctx, "skipping invalid theme filename",
				slog.String("filename", entry.Name()))
			continue
		}

		// Check file size
		info, err := entry.Info()
		if err != nil {
			s.logger.WarnContext(ctx, "failed to get theme file info",
				slog.String("filename", entry.Name()),
				slog.String("error", err.Error()))
			continue
		}
		if info.Size() > models.ThemeFileMaxSize {
			s.logger.WarnContext(ctx, "skipping theme file exceeding size limit",
				slog.String("filename", entry.Name()),
				slog.Int64("size", info.Size()),
				slog.Int64("max_size", models.ThemeFileMaxSize))
			continue
		}

		// Read and validate theme
		cssPath := filepath.Join(themesDir, entry.Name())
		cssContent, err := os.ReadFile(cssPath)
		if err != nil {
			s.logger.WarnContext(ctx, "failed to read theme file",
				slog.String("filename", entry.Name()),
				slog.String("error", err.Error()))
			continue
		}

		if err := s.ValidateTheme(cssContent); err != nil {
			s.logger.WarnContext(ctx, "invalid theme file",
				slog.String("filename", entry.Name()),
				slog.String("error", err.Error()))
			continue
		}

		themeID := strings.TrimSuffix(entry.Name(), ".css")
		theme := models.Theme{
			ID:         themeID,
			Name:       s.formatThemeName(themeID),
			Source:     models.ThemeSourceCustom,
			ModifiedAt: info.ModTime(),
			Colors:     s.extractColors(cssContent),
		}

		themes = append(themes, theme)
	}

	return themes, nil
}

// customThemePath returns the path to a custom theme file, or empty if not found.
func (s *ThemeService) customThemePath(themeID string) string {
	path := filepath.Join(s.dataDir, "themes", themeID+".css")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

// formatThemeName converts a theme ID to a human-readable name.
func (s *ThemeService) formatThemeName(id string) string {
	// Replace hyphens/underscores with spaces and title case
	name := strings.ReplaceAll(id, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")
	return strings.Title(name)
}

// extractColors extracts color values from CSS for theme previews.
func (s *ThemeService) extractColors(css []byte) *models.ThemeColors {
	content := string(css)

	colors := &models.ThemeColors{
		Light: models.ThemePalette{},
		Dark:  models.ThemePalette{},
	}

	// Extract :root (light mode) colors
	if rootMatch := rootBlockPattern.FindStringSubmatch(content); rootMatch != nil {
		colors.Light = s.extractPalette(rootMatch[1])
	}

	// Extract .dark colors
	if darkMatch := darkBlockPattern.FindStringSubmatch(content); darkMatch != nil {
		colors.Dark = s.extractPalette(darkMatch[1])
	}

	return colors
}

// extractPalette extracts color values from a CSS block.
func (s *ThemeService) extractPalette(cssBlock string) models.ThemePalette {
	palette := models.ThemePalette{}

	matches := varValuePattern.FindAllStringSubmatch(cssBlock, -1)
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		varName := match[1]
		value := strings.TrimSpace(match[2])

		switch varName {
		case "--background":
			palette.Background = value
		case "--foreground":
			palette.Foreground = value
		case "--primary":
			palette.Primary = value
		case "--secondary":
			palette.Secondary = value
		case "--accent":
			palette.Accent = value
		}
	}

	return palette
}

// EnsureThemesDirectory creates the custom themes directory if it doesn't exist.
func (s *ThemeService) EnsureThemesDirectory() error {
	themesDir := filepath.Join(s.dataDir, "themes")
	return os.MkdirAll(themesDir, 0755)
}

// IsBuiltinTheme returns true if the theme ID is a built-in theme.
func (s *ThemeService) IsBuiltinTheme(themeID string) bool {
	themesFS, err := assets.GetThemesFS()
	if err != nil {
		return false
	}

	_, err = fs.ReadFile(themesFS, themeID+".css")
	return err == nil
}
