package handlers

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jmylchreest/tvarr/internal/service"
	"github.com/jmylchreest/tvarr/internal/storage"
)

// LogoHandler handles logo API endpoints.
type LogoHandler struct {
	logoService *service.LogoService
}

// NewLogoHandler creates a new logo handler.
func NewLogoHandler(logoService *service.LogoService) *LogoHandler {
	return &LogoHandler{logoService: logoService}
}

// Register registers the logo routes with the API.
func (h *LogoHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "getLogos",
		Method:      "GET",
		Path:        "/api/v1/logos",
		Summary:     "List logo assets",
		Description: "Returns paginated list of logo assets",
		Tags:        []string{"Logos"},
	}, h.GetLogos)

	huma.Register(api, huma.Operation{
		OperationID: "getLogoStats",
		Method:      "GET",
		Path:        "/api/v1/logos/stats",
		Summary:     "Get logo statistics",
		Description: "Returns logo cache statistics",
		Tags:        []string{"Logos"},
	}, h.GetLogoStats)

	huma.Register(api, huma.Operation{
		OperationID: "rescanLogoCache",
		Method:      "POST",
		Path:        "/api/v1/logos/rescan",
		Summary:     "Rescan logo cache",
		Description: "Triggers a rescan of the logo cache directory",
		Tags:        []string{"Logos"},
	}, h.RescanLogoCache)

	huma.Register(api, huma.Operation{
		OperationID: "clearLogoCache",
		Method:      "DELETE",
		Path:        "/api/v1/logos/clear-cache",
		Summary:     "Clear logo cache",
		Description: "Clears all cached logos (not uploaded logos)",
		Tags:        []string{"Logos"},
	}, h.ClearLogoCache)

	huma.Register(api, huma.Operation{
		OperationID: "getLogo",
		Method:      "GET",
		Path:        "/api/v1/logos/{id}",
		Summary:     "Get logo by ID",
		Description: "Returns a specific logo asset metadata",
		Tags:        []string{"Logos"},
	}, h.GetLogo)

	huma.Register(api, huma.Operation{
		OperationID: "deleteLogo",
		Method:      "DELETE",
		Path:        "/api/v1/logos/{id}",
		Summary:     "Delete logo",
		Description: "Deletes a logo asset",
		Tags:        []string{"Logos"},
	}, h.DeleteLogo)

	huma.Register(api, huma.Operation{
		OperationID:      "uploadLogo",
		Method:           "POST",
		Path:             "/api/v1/logos/upload",
		Summary:          "Upload logo",
		Description:      "Uploads a new logo asset",
		Tags:             []string{"Logos"},
		RequestBody:      &huma.RequestBody{Content: map[string]*huma.MediaType{"multipart/form-data": {}}},
		SkipValidateBody: true,
	}, h.UploadLogo)

	huma.Register(api, huma.Operation{
		OperationID:      "replaceLogo",
		Method:           "PUT",
		Path:             "/api/v1/logos/{id}/replace",
		Summary:          "Replace logo image",
		Description:      "Replaces an existing logo's image files. Old linked assets are deleted.",
		Tags:             []string{"Logos"},
		RequestBody:      &huma.RequestBody{Content: map[string]*huma.MediaType{"multipart/form-data": {}}},
		SkipValidateBody: true,
	}, h.ReplaceLogo)
}

// RegisterFileServer registers a file server route to serve logo images.
// This serves files at /logos/{filename} from the logo cache.
func (h *LogoHandler) RegisterFileServer(router chi.Router) {
	router.Get("/logos/{filename}", h.ServeLogoFile)
	router.Head("/logos/{filename}", h.ServeLogoFile) // Support HEAD requests for browsers
}

// ServeLogoFile serves a logo image file by filename.
func (h *LogoHandler) ServeLogoFile(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	if filename == "" {
		http.Error(w, "filename required", http.StatusBadRequest)
		return
	}

	// Extract ID from filename (remove extension)
	id := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Look up the logo metadata
	meta := h.logoService.GetLogoByID(id)
	if meta == nil {
		http.Error(w, "logo not found", http.StatusNotFound)
		return
	}

	// Get the file
	file, err := h.logoService.GetLogoFile(meta)
	if err != nil {
		http.Error(w, "failed to read logo file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Set content type
	contentType := meta.ContentType
	if contentType == "" {
		contentType = "image/png"
	}
	w.Header().Set("Content-Type", contentType)

	// Set cache headers (logos are immutable once cached)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")

	// Copy the file to the response
	io.Copy(w, file)
}

// LinkedAssetResponse represents a linked asset in API responses.
type LinkedAssetResponse struct {
	Type        string `json:"type"`
	Path        string `json:"path"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	URL         string `json:"url"`
}

// LogoAsset represents a logo asset in API responses.
type LogoAsset struct {
	ID                  string                `json:"id"`
	Name                string                `json:"name"`
	Description         *string               `json:"description,omitempty"`
	FileName            string                `json:"file_name"`
	FilePath            string                `json:"file_path"`
	FileSize            int64                 `json:"file_size"`
	MimeType            string                `json:"mime_type"`
	OriginalMimeType    *string               `json:"original_mime_type,omitempty"`
	OriginalFileSize    *int64                `json:"original_file_size,omitempty"`
	AssetType           string                `json:"asset_type"` // 'uploaded' | 'cached'
	SourceURL           *string               `json:"source_url,omitempty"`
	Width               *int                  `json:"width,omitempty"`
	Height              *int                  `json:"height,omitempty"`
	ParentAssetID       *string               `json:"parent_asset_id,omitempty"`
	FormatType          string                `json:"format_type"`
	CreatedAt           string                `json:"created_at"`
	UpdatedAt           string                `json:"updated_at"`
	URL                 string                `json:"url"`
	LinkedAssets        []LinkedAssetResponse `json:"linked_assets,omitempty"`
	LinkedAssetsCount   int                   `json:"linked_assets_count"`
	TotalLinkedSize     int64                 `json:"total_linked_size"`
}

// logoMetadataToAsset converts storage.CachedLogoMetadata to LogoAsset.
func logoMetadataToAsset(meta *storage.CachedLogoMetadata) LogoAsset {
	// Determine asset type
	assetType := "cached"
	if meta.Source == storage.LogoSourceUploaded {
		assetType = "uploaded"
	}

	// Extract format from content type
	formatType := "unknown"
	if meta.ContentType != "" {
		parts := strings.Split(meta.ContentType, "/")
		if len(parts) > 1 {
			formatType = parts[1]
		}
	}

	// Generate file name
	fileName := meta.GetID()
	if formatType != "unknown" {
		fileName = meta.GetID() + "." + formatType
	}

	// Source URL (may be empty for uploaded logos)
	var sourceURL *string
	if meta.OriginalURL != "" {
		sourceURL = &meta.OriginalURL
	}

	// Original content type (if different from display type)
	var originalMimeType *string
	if meta.OriginalContentType != "" && meta.OriginalContentType != meta.ContentType {
		originalMimeType = &meta.OriginalContentType
	}

	// Original file size (if original was stored)
	var originalFileSize *int64
	if meta.OriginalFileSize > 0 {
		originalFileSize = &meta.OriginalFileSize
	}

	// Use relative path for serving
	relPath := meta.RelativeImagePath()

	// Convert linked assets to response format
	linkedAssets := make([]LinkedAssetResponse, 0, len(meta.LinkedAssets))
	var totalLinkedSize int64
	for _, asset := range meta.LinkedAssets {
		linkedAssets = append(linkedAssets, LinkedAssetResponse{
			Type:        asset.Type,
			Path:        asset.Path,
			ContentType: asset.ContentType,
			Size:        asset.Size,
			URL:         "/logos/" + filepath.Base(asset.Path),
		})
		totalLinkedSize += asset.Size
	}

	// Width and height pointers
	var width, height *int
	if meta.Width > 0 {
		width = &meta.Width
	}
	if meta.Height > 0 {
		height = &meta.Height
	}

	return LogoAsset{
		ID:                meta.GetID(),
		Name:              extractNameFromURL(meta.OriginalURL, meta.GetID()),
		FileName:          fileName,
		FilePath:          relPath,
		FileSize:          meta.FileSize,
		MimeType:          meta.ContentType,
		OriginalMimeType:  originalMimeType,
		OriginalFileSize:  originalFileSize,
		AssetType:         assetType,
		SourceURL:         sourceURL,
		Width:             width,
		Height:            height,
		FormatType:        formatType,
		CreatedAt:         meta.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         meta.LastSeenAt.Format(time.RFC3339),
		URL:               "/logos/" + filepath.Base(relPath),
		LinkedAssets:      linkedAssets,
		LinkedAssetsCount: len(linkedAssets),
		TotalLinkedSize:   totalLinkedSize,
	}
}

// extractNameFromURL extracts a readable name from a URL or uses the ID.
func extractNameFromURL(url, fallbackID string) string {
	if url == "" {
		return fallbackID
	}
	// Try to extract filename from URL
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		lastPart := parts[len(parts)-1]
		// Remove query parameters
		if idx := strings.Index(lastPart, "?"); idx != -1 {
			lastPart = lastPart[:idx]
		}
		if lastPart != "" {
			return lastPart
		}
	}
	return fallbackID
}

// GetLogosInput is the input for listing logos.
type GetLogosInput struct {
	Page          int    `query:"page" default:"1" minimum:"1"`
	Limit         int    `query:"limit" default:"20" minimum:"1" maximum:"100"`
	IncludeCached bool   `query:"include_cached" default:"true"`
	Search        string `query:"search"`
}

// GetLogosOutput is the output for listing logos.
type GetLogosOutput struct {
	Body struct {
		Assets     []LogoAsset `json:"assets"`
		TotalCount int         `json:"total_count"`
		Page       int         `json:"page"`
		Limit      int         `json:"limit"`
		TotalPages int         `json:"total_pages"`
	}
}

// GetLogos returns paginated list of logo assets.
func (h *LogoHandler) GetLogos(ctx context.Context, input *GetLogosInput) (*GetLogosOutput, error) {
	allLogos := h.logoService.GetAllLogos()

	// Filter by type and search
	var filtered []*storage.CachedLogoMetadata
	for _, meta := range allLogos {
		// Filter by cached/uploaded
		if !input.IncludeCached && meta.Source == storage.LogoSourceCached {
			continue
		}

		// Search filter
		if input.Search != "" {
			searchLower := strings.ToLower(input.Search)
			if !strings.Contains(strings.ToLower(meta.OriginalURL), searchLower) &&
				!strings.Contains(strings.ToLower(meta.GetID()), searchLower) {
				continue
			}
		}

		filtered = append(filtered, meta)
	}

	// Calculate pagination
	totalCount := len(filtered)
	totalPages := totalCount / input.Limit
	if totalCount%input.Limit > 0 {
		totalPages++
	}

	// Apply pagination
	start := (input.Page - 1) * input.Limit
	end := start + input.Limit
	if start > totalCount {
		start = totalCount
	}
	if end > totalCount {
		end = totalCount
	}

	// Convert to response format
	assets := make([]LogoAsset, 0, end-start)
	for _, meta := range filtered[start:end] {
		assets = append(assets, logoMetadataToAsset(meta))
	}

	resp := &GetLogosOutput{}
	resp.Body.Assets = assets
	resp.Body.TotalCount = totalCount
	resp.Body.Page = input.Page
	resp.Body.Limit = input.Limit
	resp.Body.TotalPages = totalPages
	return resp, nil
}

// LogoStats represents logo cache statistics.
type LogoStats struct {
	TotalCachedLogos        int      `json:"total_cached_logos"`
	TotalUploadedLogos      int      `json:"total_uploaded_logos"`
	TotalStorageUsed        int64    `json:"total_storage_used"`
	TotalLinkedAssets       int      `json:"total_linked_assets"`
	CacheHitRate            *float64 `json:"cache_hit_rate,omitempty"`
	FilesystemCachedLogos   int      `json:"filesystem_cached_logos"`
	FilesystemCachedStorage int64    `json:"filesystem_cached_storage"`
}

// GetLogoStatsInput is the input for getting logo stats.
type GetLogoStatsInput struct{}

// GetLogoStatsOutput is the output for getting logo stats.
type GetLogoStatsOutput struct {
	Body LogoStats
}

// GetLogoStats returns logo cache statistics.
func (h *LogoHandler) GetLogoStats(ctx context.Context, input *GetLogoStatsInput) (*GetLogoStatsOutput, error) {
	stats := h.logoService.GetStats()

	resp := &GetLogoStatsOutput{}
	resp.Body = LogoStats{
		TotalCachedLogos:        stats.CachedLogos,
		TotalUploadedLogos:      stats.UploadedLogos,
		TotalStorageUsed:        stats.TotalSize,
		TotalLinkedAssets:       stats.TotalLogos,
		CacheHitRate:            nil, // Not tracked yet
		FilesystemCachedLogos:   stats.CachedLogos,
		FilesystemCachedStorage: stats.CachedSize,
	}
	return resp, nil
}

// RescanLogoCacheInput is the input for rescanning the logo cache.
type RescanLogoCacheInput struct{}

// RescanLogoCacheOutput is the output for rescanning the logo cache.
type RescanLogoCacheOutput struct {
	Body struct {
		Success       bool   `json:"success"`
		Message       string `json:"message"`
		LogosScanned  int    `json:"logos_scanned"`
		NewLogosFound int    `json:"new_logos_found"`
		Timestamp     string `json:"timestamp"`
	}
}

// RescanLogoCache triggers a rescan of the logo cache.
func (h *LogoHandler) RescanLogoCache(ctx context.Context, input *RescanLogoCacheInput) (*RescanLogoCacheOutput, error) {
	// Get count before rescan
	statsBefore := h.logoService.GetStats()

	// Reload index from disk
	if err := h.logoService.LoadIndex(ctx); err != nil {
		return nil, huma.Error500InternalServerError("Failed to rescan logo cache: " + err.Error())
	}

	// Get count after
	statsAfter := h.logoService.GetStats()

	resp := &RescanLogoCacheOutput{}
	resp.Body.Success = true
	resp.Body.Message = "Logo cache rescanned"
	resp.Body.LogosScanned = statsAfter.TotalLogos
	resp.Body.NewLogosFound = statsAfter.TotalLogos - statsBefore.TotalLogos
	resp.Body.Timestamp = time.Now().UTC().Format(time.RFC3339)
	return resp, nil
}

// ClearLogoCacheInput is the input for clearing the logo cache.
type ClearLogoCacheInput struct{}

// ClearLogoCacheOutput is the output for clearing the logo cache.
type ClearLogoCacheOutput struct {
	Body struct {
		Success      bool   `json:"success"`
		Message      string `json:"message"`
		LogosCleared int    `json:"logos_cleared"`
		SpaceFreed   int64  `json:"space_freed"`
		Timestamp    string `json:"timestamp"`
	}
}

// ClearLogoCache clears all cached logos (not uploaded logos).
func (h *LogoHandler) ClearLogoCache(ctx context.Context, input *ClearLogoCacheInput) (*ClearLogoCacheOutput, error) {
	// Get stats before clearing
	statsBefore := h.logoService.GetStats()

	// Delete only cached (URL-sourced) logos
	allLogos := h.logoService.GetAllLogos()
	cleared := 0
	spaceFreed := int64(0)

	for _, meta := range allLogos {
		if meta.Source == storage.LogoSourceCached {
			spaceFreed += meta.FileSize
			if err := h.logoService.DeleteLogo(meta.GetID()); err == nil {
				cleared++
			}
		}
	}

	resp := &ClearLogoCacheOutput{}
	resp.Body.Success = true
	resp.Body.Message = "Cached logos cleared"
	resp.Body.LogosCleared = cleared
	resp.Body.SpaceFreed = spaceFreed
	if cleared == 0 && statsBefore.CachedLogos > 0 {
		resp.Body.Message = "Some logos could not be cleared"
	}
	resp.Body.Timestamp = time.Now().UTC().Format(time.RFC3339)
	return resp, nil
}

// GetLogoInput is the input for getting a logo by ID.
type GetLogoInput struct {
	ID string `path:"id" required:"true"`
}

// GetLogoOutput is the output for getting a logo by ID.
type GetLogoOutput struct {
	Body LogoAsset
}

// GetLogo returns a specific logo asset.
func (h *LogoHandler) GetLogo(ctx context.Context, input *GetLogoInput) (*GetLogoOutput, error) {
	meta := h.logoService.GetLogoByID(input.ID)
	if meta == nil {
		return nil, huma.Error404NotFound("Logo not found")
	}

	return &GetLogoOutput{
		Body: logoMetadataToAsset(meta),
	}, nil
}

// DeleteLogoInput is the input for deleting a logo.
type DeleteLogoInput struct {
	ID string `path:"id" required:"true"`
}

// DeleteLogoOutput is the output for deleting a logo.
type DeleteLogoOutput struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

// DeleteLogo deletes a logo asset.
func (h *LogoHandler) DeleteLogo(ctx context.Context, input *DeleteLogoInput) (*DeleteLogoOutput, error) {
	meta := h.logoService.GetLogoByID(input.ID)
	if meta == nil {
		return nil, huma.Error404NotFound("Logo not found")
	}

	if err := h.logoService.DeleteLogo(input.ID); err != nil {
		return nil, huma.Error500InternalServerError("Failed to delete logo: " + err.Error())
	}

	return &DeleteLogoOutput{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: true,
			Message: "Logo deleted",
		},
	}, nil
}

// UploadLogoInput is the input for uploading a logo.
type UploadLogoInput struct {
	RawBody multipart.Form
}

// UploadLogoOutput is the output for uploading a logo.
type UploadLogoOutput struct {
	Body LogoAsset
}

// UploadLogo handles logo file upload.
func (h *LogoHandler) UploadLogo(ctx context.Context, input *UploadLogoInput) (*UploadLogoOutput, error) {
	// Get file from multipart form
	files := input.RawBody.File["file"]
	if len(files) == 0 {
		return nil, huma.Error400BadRequest("No file provided")
	}

	fileHeader := files[0]

	// Open the file
	file, err := fileHeader.Open()
	if err != nil {
		return nil, huma.Error400BadRequest("Failed to open uploaded file")
	}
	defer file.Close()

	// Read content to determine content type
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, huma.Error400BadRequest("Failed to read uploaded file")
	}

	// Get content type from header or detect from content
	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		// Detect content type from first bytes
		contentType = detectImageContentType(content)
	}

	// Validate it's an image
	if !strings.HasPrefix(contentType, "image/") {
		return nil, huma.Error400BadRequest("Invalid file type: must be an image")
	}

	// Get name from form or filename
	name := fileHeader.Filename
	if names := input.RawBody.Value["name"]; len(names) > 0 && names[0] != "" {
		name = names[0]
	}

	// Upload the logo
	meta, err := h.logoService.UploadLogo(ctx, name, contentType, bytes.NewReader(content))
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to upload logo: " + err.Error())
	}

	return &UploadLogoOutput{
		Body: logoMetadataToAsset(meta),
	}, nil
}

// detectImageContentType detects the content type from image magic bytes.
func detectImageContentType(data []byte) string {
	if len(data) < 8 {
		return "application/octet-stream"
	}

	// Check magic bytes
	switch {
	case data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47:
		return "image/png"
	case data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "image/jpeg"
	case data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46:
		return "image/gif"
	case data[0] == 0x52 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x46:
		// Could be WEBP
		if len(data) >= 12 && data[8] == 0x57 && data[9] == 0x45 && data[10] == 0x42 && data[11] == 0x50 {
			return "image/webp"
		}
		return "application/octet-stream"
	case data[0] == 0x00 && data[1] == 0x00 && data[2] == 0x01 && data[3] == 0x00:
		return "image/x-icon"
	default:
		// Check for SVG (text-based)
		if bytes.Contains(data[:min(len(data), 256)], []byte("<svg")) {
			return "image/svg+xml"
		}
		return "application/octet-stream"
	}
}

// ReplaceLogoInput is the input for replacing a logo image.
type ReplaceLogoInput struct {
	ID      string `path:"id" required:"true"`
	RawBody multipart.Form
}

// ReplaceLogoOutput is the output for replacing a logo image.
type ReplaceLogoOutput struct {
	Body LogoAsset
}

// ReplaceLogo handles logo image replacement.
func (h *LogoHandler) ReplaceLogo(ctx context.Context, input *ReplaceLogoInput) (*ReplaceLogoOutput, error) {
	// Get file from multipart form
	files := input.RawBody.File["file"]
	if len(files) == 0 {
		return nil, huma.Error400BadRequest("No file provided")
	}

	fileHeader := files[0]

	// Open the file
	file, err := fileHeader.Open()
	if err != nil {
		return nil, huma.Error400BadRequest("Failed to open uploaded file")
	}
	defer file.Close()

	// Read content to determine content type
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, huma.Error400BadRequest("Failed to read uploaded file")
	}

	// Get content type from header or detect from content
	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		// Detect content type from first bytes
		contentType = detectImageContentType(content)
	}

	// Validate it's an image
	if !strings.HasPrefix(contentType, "image/") {
		return nil, huma.Error400BadRequest("Invalid file type: must be an image")
	}

	// Get name from form or filename
	name := fileHeader.Filename
	if names := input.RawBody.Value["name"]; len(names) > 0 && names[0] != "" {
		name = names[0]
	}

	// Replace the logo
	meta, err := h.logoService.ReplaceLogo(ctx, input.ID, name, contentType, bytes.NewReader(content))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Logo not found")
		}
		return nil, huma.Error500InternalServerError("Failed to replace logo: " + err.Error())
	}

	return &ReplaceLogoOutput{
		Body: logoMetadataToAsset(meta),
	}, nil
}
