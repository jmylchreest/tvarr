// Package handlers provides HTTP handlers for the tvarr API.
package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/storage"
)

// OutputHandler handles serving generated M3U and XMLTV output files.
type OutputHandler struct {
	sandbox *storage.Sandbox
	logger  *slog.Logger
}

// NewOutputHandler creates a new output handler.
func NewOutputHandler(sandbox *storage.Sandbox) *OutputHandler {
	return &OutputHandler{
		sandbox: sandbox,
		logger:  slog.Default(),
	}
}

// WithLogger sets the logger for the handler.
func (h *OutputHandler) WithLogger(logger *slog.Logger) *OutputHandler {
	if logger != nil {
		h.logger = logger
	}
	return h
}

// RegisterFileServer registers the file server routes for M3U and XMLTV files.
// Routes:
//   - GET /proxy/{id}.m3u - Serve M3U playlist
//   - GET /proxy/{id}.xmltv - Serve XMLTV file
func (h *OutputHandler) RegisterFileServer(router *chi.Mux) {
	router.Get("/proxy/{proxyID}.m3u", h.serveM3U)
	router.Get("/proxy/{proxyID}.xmltv", h.serveXMLTV)
}

// Register registers the output endpoints with the API for OpenAPI documentation.
func (h *OutputHandler) Register(api huma.API) {
	// Register M3U endpoint for OpenAPI documentation
	huma.Register(api, huma.Operation{
		OperationID: "getProxyM3U",
		Method:      "GET",
		Path:        "/proxy/{proxyID}.m3u",
		Summary:     "Get proxy M3U playlist",
		Description: "Returns the generated M3U playlist for a stream proxy",
		Tags:        []string{"Proxy Output"},
	}, h.GetM3U)

	// Register XMLTV endpoint for OpenAPI documentation
	huma.Register(api, huma.Operation{
		OperationID: "getProxyXMLTV",
		Method:      "GET",
		Path:        "/proxy/{proxyID}.xmltv",
		Summary:     "Get proxy XMLTV guide",
		Description: "Returns the generated XMLTV program guide for a stream proxy",
		Tags:        []string{"Proxy Output"},
	}, h.GetXMLTV)
}

// GetM3UInput is the input for getting the M3U file.
type GetM3UInput struct {
	ProxyID string `path:"proxyID" doc:"Stream proxy ID (ULID)"`
}

// GetM3UOutput is the output for getting the M3U file.
type GetM3UOutput struct {
	ContentType string `header:"Content-Type"`
	Body        []byte
}

// GetM3U returns the M3U file for a proxy (Huma handler for OpenAPI).
func (h *OutputHandler) GetM3U(ctx context.Context, input *GetM3UInput) (*GetM3UOutput, error) {
	// Validate ULID format
	if _, err := models.ParseULID(input.ProxyID); err != nil {
		return nil, huma.Error400BadRequest("invalid proxy ID format", err)
	}

	// Read the M3U file
	data, err := h.readOutputFile(input.ProxyID, ".m3u")
	if err != nil {
		// Use errors.Is to properly detect wrapped os.ErrNotExist
		if errors.Is(err, os.ErrNotExist) {
			return nil, huma.Error404NotFound(fmt.Sprintf("M3U not found for proxy %s. Generate the proxy first.", input.ProxyID))
		}
		return nil, huma.Error500InternalServerError("failed to read M3U file", err)
	}

	return &GetM3UOutput{
		ContentType: "audio/x-mpegurl",
		Body:        data,
	}, nil
}

// GetXMLTVInput is the input for getting the XMLTV file.
type GetXMLTVInput struct {
	ProxyID string `path:"proxyID" doc:"Stream proxy ID (ULID)"`
}

// GetXMLTVOutput is the output for getting the XMLTV file.
type GetXMLTVOutput struct {
	ContentType string `header:"Content-Type"`
	Body        []byte
}

// GetXMLTV returns the XMLTV file for a proxy (Huma handler for OpenAPI).
func (h *OutputHandler) GetXMLTV(ctx context.Context, input *GetXMLTVInput) (*GetXMLTVOutput, error) {
	// Validate ULID format
	if _, err := models.ParseULID(input.ProxyID); err != nil {
		return nil, huma.Error400BadRequest("invalid proxy ID format", err)
	}

	// Read the XMLTV file
	data, err := h.readOutputFile(input.ProxyID, ".xml")
	if err != nil {
		// Use errors.Is to properly detect wrapped os.ErrNotExist
		if errors.Is(err, os.ErrNotExist) {
			return nil, huma.Error404NotFound(fmt.Sprintf("XMLTV not found for proxy %s. Generate the proxy first.", input.ProxyID))
		}
		return nil, huma.Error500InternalServerError("failed to read XMLTV file", err)
	}

	return &GetXMLTVOutput{
		ContentType: "application/xml",
		Body:        data,
	}, nil
}

// serveM3U handles direct HTTP requests for M3U files.
func (h *OutputHandler) serveM3U(w http.ResponseWriter, r *http.Request) {
	proxyID := chi.URLParam(r, "proxyID")

	// Validate ULID format
	if _, err := models.ParseULID(proxyID); err != nil {
		http.Error(w, "invalid proxy ID format", http.StatusBadRequest)
		return
	}

	data, err := h.readOutputFile(proxyID, ".m3u")
	if err != nil {
		// Use errors.Is to properly detect wrapped os.ErrNotExist
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, fmt.Sprintf("M3U not found for proxy %s", proxyID), http.StatusNotFound)
			return
		}
		h.logger.Error("failed to read M3U file",
			slog.String("proxy_id", proxyID),
			slog.String("error", err.Error()),
		)
		http.Error(w, "failed to read M3U file", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "audio/x-mpegurl")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.m3u\"", proxyID))
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// serveXMLTV handles direct HTTP requests for XMLTV files.
func (h *OutputHandler) serveXMLTV(w http.ResponseWriter, r *http.Request) {
	proxyID := chi.URLParam(r, "proxyID")

	// Validate ULID format
	if _, err := models.ParseULID(proxyID); err != nil {
		http.Error(w, "invalid proxy ID format", http.StatusBadRequest)
		return
	}

	data, err := h.readOutputFile(proxyID, ".xml")
	if err != nil {
		// Use errors.Is to properly detect wrapped os.ErrNotExist
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, fmt.Sprintf("XMLTV not found for proxy %s", proxyID), http.StatusNotFound)
			return
		}
		h.logger.Error("failed to read XMLTV file",
			slog.String("proxy_id", proxyID),
			slog.String("error", err.Error()),
		)
		http.Error(w, "failed to read XMLTV file", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.xmltv\"", proxyID))
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// readOutputFile reads an output file from the sandbox.
func (h *OutputHandler) readOutputFile(proxyID, ext string) ([]byte, error) {
	// Sanitize the proxy ID to prevent path traversal
	cleanID := filepath.Clean(proxyID)
	if cleanID != proxyID || strings.Contains(proxyID, "/") || strings.Contains(proxyID, "\\") {
		return nil, fmt.Errorf("invalid proxy ID")
	}

	// Files are stored in the "output" subdirectory of the sandbox
	filename := filepath.Join("output", proxyID+ext)
	return h.sandbox.ReadFile(filename)
}
