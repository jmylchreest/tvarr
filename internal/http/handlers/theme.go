package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/service"
)

// ThemeHandler handles theme API endpoints.
type ThemeHandler struct {
	themeService *service.ThemeService
}

// NewThemeHandler creates a new theme handler.
func NewThemeHandler(themeService *service.ThemeService) *ThemeHandler {
	return &ThemeHandler{
		themeService: themeService,
	}
}

// Register registers the theme routes with the Huma API.
func (h *ThemeHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listThemes",
		Method:      "GET",
		Path:        "/api/v1/themes",
		Summary:     "List all themes",
		Description: "Returns all available themes (built-in and custom)",
		Tags:        []string{"Themes"},
	}, h.ListThemes)
}

// RegisterChiRoutes registers additional Chi routes for theme CSS serving.
// This is separate because CSS serving needs custom content-type and caching headers.
func (h *ThemeHandler) RegisterChiRoutes(r *chi.Mux) {
	r.Get("/api/v1/themes/{themeId}.css", h.serveThemeCSS)
}

// ListThemesInput is the input for listing themes.
type ListThemesInput struct{}

// ListThemesOutput is the output for listing themes.
type ListThemesOutput struct {
	Body models.ThemeListResponse
}

// ListThemes returns all available themes.
func (h *ThemeHandler) ListThemes(ctx context.Context, input *ListThemesInput) (*ListThemesOutput, error) {
	response, err := h.themeService.ListThemes(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list themes", err)
	}

	return &ListThemesOutput{Body: *response}, nil
}

// serveThemeCSS serves a theme CSS file with appropriate caching headers.
// Serves both built-in themes (from embedded FS) and custom themes (from data dir).
func (h *ThemeHandler) serveThemeCSS(w http.ResponseWriter, r *http.Request) {
	themeID := chi.URLParam(r, "themeId")
	if themeID == "" {
		http.Error(w, "theme ID required", http.StatusBadRequest)
		return
	}

	// Remove .css suffix if present (chi extracts without extension from route pattern)
	themeID = strings.TrimSuffix(themeID, ".css")

	css, source, modTime, err := h.themeService.GetThemeCSS(r.Context(), themeID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "theme not found", http.StatusNotFound)
			return
		}
		if strings.Contains(err.Error(), "invalid theme ID") {
			http.Error(w, "invalid theme ID format", http.StatusBadRequest)
			return
		}
		http.Error(w, "failed to load theme", http.StatusInternalServerError)
		return
	}

	// Set content type
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(css)))

	// Set caching headers based on source
	if source == models.ThemeSourceBuiltin {
		// Built-in themes never change at runtime - cache for 24 hours
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Set("ETag", fmt.Sprintf(`"builtin-%s"`, themeID))
	} else {
		// Custom themes may change, use shorter cache with revalidation
		w.Header().Set("Cache-Control", "public, max-age=3600, must-revalidate")
		w.Header().Set("ETag", fmt.Sprintf(`"mtime-%d"`, modTime.Unix()))
		w.Header().Set("Last-Modified", modTime.UTC().Format(http.TimeFormat))
	}

	// Check If-None-Match header for conditional requests
	if etag := r.Header.Get("If-None-Match"); etag != "" {
		expectedETag := w.Header().Get("ETag")
		if etag == expectedETag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	// Check If-Modified-Since for custom themes
	if source == models.ThemeSourceCustom {
		if ims := r.Header.Get("If-Modified-Since"); ims != "" {
			if t, err := time.Parse(http.TimeFormat, ims); err == nil {
				if !modTime.After(t) {
					w.WriteHeader(http.StatusNotModified)
					return
				}
			}
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write(css)
}
