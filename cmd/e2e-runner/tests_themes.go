package main

import (
	"context"
	"fmt"
	"strings"
)

// runThemeTests runs theme API tests.
func (r *E2ERunner) runThemeTests(ctx context.Context) {
	// List All Themes
	r.runTestWithInfo("Themes: List All Themes",
		"GET /api/v1/themes - verify built-in themes exist with default set",
		func() error {
			themes, err := r.client.GetThemes(ctx)
			if err != nil {
				return err
			}
			if len(themes.Themes) == 0 {
				return fmt.Errorf("expected at least one theme, got none")
			}
			if themes.Default == "" {
				return fmt.Errorf("expected default theme to be set")
			}

			// Count built-in vs custom themes
			var builtinCount, customCount int
			for _, t := range themes.Themes {
				if t.Source == "builtin" {
					builtinCount++
				} else if t.Source == "custom" {
					customCount++
				}
			}
			r.log("  Found %d themes (%d built-in, %d custom), default: %s",
				len(themes.Themes), builtinCount, customCount, themes.Default)

			// Verify default theme exists in list
			var defaultFound bool
			for _, t := range themes.Themes {
				if t.ID == themes.Default {
					defaultFound = true
					break
				}
			}
			if !defaultFound {
				return fmt.Errorf("default theme %q not found in theme list", themes.Default)
			}
			return nil
		})

	// Get Built-in Theme CSS
	r.runTestWithInfo("Themes: Get Built-in Theme CSS (graphite)",
		"GET /api/v1/themes/graphite/css - verify CSS contains :root, .dark, --background",
		func() error {
			css, err := r.client.GetThemeCSS(ctx, "graphite")
			if err != nil {
				return err
			}
			if len(css) == 0 {
				return fmt.Errorf("expected non-empty CSS content")
			}
			// Verify CSS structure
			if !strings.Contains(css, ":root") {
				return fmt.Errorf("CSS missing :root selector")
			}
			if !strings.Contains(css, ".dark") {
				return fmt.Errorf("CSS missing .dark selector")
			}
			if !strings.Contains(css, "--background") {
				return fmt.Errorf("CSS missing --background variable")
			}
			r.log("  Got graphite.css (%d bytes)", len(css))
			return nil
		})

	// Get Non-Existent Theme Returns 404
	r.runTestWithInfo("Themes: Get Non-Existent Theme Returns 404",
		"GET /api/v1/themes/nonexistent-theme-12345/css - expect 404 response",
		func() error {
			_, err := r.client.GetThemeCSS(ctx, "nonexistent-theme-12345")
			if err == nil {
				return fmt.Errorf("expected error for non-existent theme")
			}
			if !strings.Contains(err.Error(), "404") {
				return fmt.Errorf("expected 404 error, got: %v", err)
			}
			r.log("  Correctly returned 404 for non-existent theme")
			return nil
		})
}
