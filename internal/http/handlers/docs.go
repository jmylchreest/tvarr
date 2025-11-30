package handlers

import (
	"fmt"
	"net/http"
)

// DocsHandler serves the OpenAPI documentation UI using Stoplight Elements.
type DocsHandler struct {
	title       string
	specPath    string
	theme       string // "dark" or "light"
	systemTheme bool   // use system preference
}

// DocsOption is a functional option for configuring DocsHandler.
type DocsOption func(*DocsHandler)

// WithTheme sets a fixed theme ("dark" or "light").
func WithTheme(theme string) DocsOption {
	return func(h *DocsHandler) {
		h.theme = theme
		h.systemTheme = false
	}
}

// WithSystemTheme enables automatic theme based on system preference.
func WithSystemTheme() DocsOption {
	return func(h *DocsHandler) {
		h.systemTheme = true
	}
}

// NewDocsHandler creates a new documentation handler.
func NewDocsHandler(title, specPath string, opts ...DocsOption) *DocsHandler {
	h := &DocsHandler{
		title:       title,
		specPath:    specPath,
		theme:       "dark", // default to dark
		systemTheme: true,   // default to system preference
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// ServeHTTP serves the documentation page.
func (h *DocsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Build theme script based on configuration
	var themeScript string
	if h.systemTheme {
		// Use system preference with dark fallback
		themeScript = `
    <script>
      // Detect system theme preference
      const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
      document.documentElement.setAttribute('data-theme', prefersDark ? 'dark' : 'light');

      // Listen for system theme changes
      window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', e => {
        document.documentElement.setAttribute('data-theme', e.matches ? 'dark' : 'light');
      });
    </script>`
	} else {
		themeScript = fmt.Sprintf(`
    <script>
      document.documentElement.setAttribute('data-theme', '%s');
    </script>`, h.theme)
	}

	html := fmt.Sprintf(`<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="referrer" content="same-origin" />
    <meta name="viewport" content="width=device-width, initial-scale=1, shrink-to-fit=no" />
    <title>%s</title>
    <link href="https://unpkg.com/@stoplight/elements@8/styles.min.css" rel="stylesheet" />
    <script src="https://unpkg.com/@stoplight/elements@8/web-components.min.js" crossorigin="anonymous"></script>
    <style>
      /* Dark theme overrides */
      html[data-theme="dark"] {
        color-scheme: dark;
      }
      html[data-theme="dark"] body {
        background-color: #1a1a2e;
      }
      html[data-theme="dark"] .sl-elements {
        --color-canvas-100: #1a1a2e;
        --color-canvas-200: #16213e;
        --color-canvas-300: #0f3460;
        --color-canvas: #1a1a2e;
        --color-text: #e4e4e7;
        --color-text-heading: #fafafa;
        --color-text-paragraph: #d4d4d8;
        --color-text-secondary: #a1a1aa;
        --color-border: #3f3f46;
      }
      /* Light theme */
      html[data-theme="light"] {
        color-scheme: light;
      }
      html[data-theme="light"] body {
        background-color: #ffffff;
      }
    </style>
    %s
  </head>
  <body style="height: 100vh; margin: 0;">
    <elements-api
      apiDescriptionUrl="%s"
      router="hash"
      layout="sidebar"
      tryItCredentialsPolicy="same-origin"
    />
  </body>
</html>`, h.title, themeScript, h.specPath)

	w.Write([]byte(html))
}
