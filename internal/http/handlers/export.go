package handlers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/service"
	"github.com/jmylchreest/tvarr/internal/version"
)

// ExportHandler handles configuration export API endpoints.
type ExportHandler struct {
	exportService *service.ExportService
	importService *service.ImportService
}

// NewExportHandler creates a new export handler.
func NewExportHandler(exportService *service.ExportService, importService *service.ImportService) *ExportHandler {
	return &ExportHandler{
		exportService: exportService,
		importService: importService,
	}
}

// Register registers the export routes with the API.
func (h *ExportHandler) Register(api huma.API) {
	// Export endpoints
	huma.Register(api, huma.Operation{
		OperationID: "exportFilters",
		Method:      "POST",
		Path:        "/api/v1/export/filters",
		Summary:     "Export filters",
		Description: "Exports selected filters or all user-created filters as JSON",
		Tags:        []string{"Export/Import"},
	}, h.ExportFilters)

	huma.Register(api, huma.Operation{
		OperationID: "exportDataMappingRules",
		Method:      "POST",
		Path:        "/api/v1/export/data-mapping-rules",
		Summary:     "Export data mapping rules",
		Description: "Exports selected data mapping rules or all user-created rules as JSON",
		Tags:        []string{"Export/Import"},
	}, h.ExportDataMappingRules)

	huma.Register(api, huma.Operation{
		OperationID: "exportClientDetectionRules",
		Method:      "POST",
		Path:        "/api/v1/export/client-detection-rules",
		Summary:     "Export client detection rules",
		Description: "Exports selected client detection rules or all user-created rules as JSON",
		Tags:        []string{"Export/Import"},
	}, h.ExportClientDetectionRules)

	huma.Register(api, huma.Operation{
		OperationID: "exportEncodingProfiles",
		Method:      "POST",
		Path:        "/api/v1/export/encoding-profiles",
		Summary:     "Export encoding profiles",
		Description: "Exports selected encoding profiles or all user-created profiles as JSON",
		Tags:        []string{"Export/Import"},
	}, h.ExportEncodingProfiles)

	// Import preview endpoints
	huma.Register(api, huma.Operation{
		OperationID: "importFiltersPreview",
		Method:      "POST",
		Path:        "/api/v1/import/filters/preview",
		Summary:     "Preview filter import",
		Description: "Returns a preview of what will happen when importing filters",
		Tags:        []string{"Export/Import"},
	}, h.ImportFiltersPreview)

	huma.Register(api, huma.Operation{
		OperationID: "importDataMappingRulesPreview",
		Method:      "POST",
		Path:        "/api/v1/import/data-mapping-rules/preview",
		Summary:     "Preview data mapping rules import",
		Description: "Returns a preview of what will happen when importing data mapping rules",
		Tags:        []string{"Export/Import"},
	}, h.ImportDataMappingRulesPreview)

	huma.Register(api, huma.Operation{
		OperationID: "importClientDetectionRulesPreview",
		Method:      "POST",
		Path:        "/api/v1/import/client-detection-rules/preview",
		Summary:     "Preview client detection rules import",
		Description: "Returns a preview of what will happen when importing client detection rules",
		Tags:        []string{"Export/Import"},
	}, h.ImportClientDetectionRulesPreview)

	huma.Register(api, huma.Operation{
		OperationID: "importEncodingProfilesPreview",
		Method:      "POST",
		Path:        "/api/v1/import/encoding-profiles/preview",
		Summary:     "Preview encoding profiles import",
		Description: "Returns a preview of what will happen when importing encoding profiles",
		Tags:        []string{"Export/Import"},
	}, h.ImportEncodingProfilesPreview)

	// Import execute endpoints
	huma.Register(api, huma.Operation{
		OperationID: "importFilters",
		Method:      "POST",
		Path:        "/api/v1/import/filters",
		Summary:     "Import filters",
		Description: "Imports filters with specified conflict resolutions",
		Tags:        []string{"Export/Import"},
	}, h.ImportFilters)

	huma.Register(api, huma.Operation{
		OperationID: "importDataMappingRules",
		Method:      "POST",
		Path:        "/api/v1/import/data-mapping-rules",
		Summary:     "Import data mapping rules",
		Description: "Imports data mapping rules with specified conflict resolutions",
		Tags:        []string{"Export/Import"},
	}, h.ImportDataMappingRules)

	huma.Register(api, huma.Operation{
		OperationID: "importClientDetectionRules",
		Method:      "POST",
		Path:        "/api/v1/import/client-detection-rules",
		Summary:     "Import client detection rules",
		Description: "Imports client detection rules with specified conflict resolutions",
		Tags:        []string{"Export/Import"},
	}, h.ImportClientDetectionRules)

	huma.Register(api, huma.Operation{
		OperationID: "importEncodingProfiles",
		Method:      "POST",
		Path:        "/api/v1/import/encoding-profiles",
		Summary:     "Import encoding profiles",
		Description: "Imports encoding profiles with specified conflict resolutions",
		Tags:        []string{"Export/Import"},
	}, h.ImportEncodingProfiles)
}

// Export Request/Response types

// ExportInput is the input for export endpoints.
type ExportInput struct {
	Body struct {
		IDs       []string `json:"ids,omitempty" doc:"List of specific IDs to export (empty = export all user-created)"`
		ExportAll bool     `json:"export_all,omitempty" doc:"Export all user-created items (default: true if ids is empty)"`
	}
}

// ExportFiltersOutput is the output for filter export.
type ExportFiltersOutput struct {
	Body models.ConfigExport[models.FilterExportItem]
}

// ExportDataMappingRulesOutput is the output for data mapping rules export.
type ExportDataMappingRulesOutput struct {
	Body models.ConfigExport[models.DataMappingRuleExportItem]
}

// ExportClientDetectionRulesOutput is the output for client detection rules export.
type ExportClientDetectionRulesOutput struct {
	Body models.ConfigExport[models.ClientDetectionRuleExportItem]
}

// ExportEncodingProfilesOutput is the output for encoding profiles export.
type ExportEncodingProfilesOutput struct {
	Body models.ConfigExport[models.EncodingProfileExportItem]
}

// Export handlers

// ExportFilters exports filters.
func (h *ExportHandler) ExportFilters(ctx context.Context, input *ExportInput) (*ExportFiltersOutput, error) {
	ids, err := parseULIDs(input.Body.IDs)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	exportAll := input.Body.ExportAll || len(ids) == 0
	export, err := h.exportService.ExportFilters(ctx, ids, exportAll)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to export filters", err)
	}

	return &ExportFiltersOutput{Body: *export}, nil
}

// ExportDataMappingRules exports data mapping rules.
func (h *ExportHandler) ExportDataMappingRules(ctx context.Context, input *ExportInput) (*ExportDataMappingRulesOutput, error) {
	ids, err := parseULIDs(input.Body.IDs)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	exportAll := input.Body.ExportAll || len(ids) == 0
	export, err := h.exportService.ExportDataMappingRules(ctx, ids, exportAll)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to export data mapping rules", err)
	}

	return &ExportDataMappingRulesOutput{Body: *export}, nil
}

// ExportClientDetectionRules exports client detection rules.
func (h *ExportHandler) ExportClientDetectionRules(ctx context.Context, input *ExportInput) (*ExportClientDetectionRulesOutput, error) {
	ids, err := parseULIDs(input.Body.IDs)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	exportAll := input.Body.ExportAll || len(ids) == 0
	export, err := h.exportService.ExportClientDetectionRules(ctx, ids, exportAll)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to export client detection rules", err)
	}

	return &ExportClientDetectionRulesOutput{Body: *export}, nil
}

// ExportEncodingProfiles exports encoding profiles.
func (h *ExportHandler) ExportEncodingProfiles(ctx context.Context, input *ExportInput) (*ExportEncodingProfilesOutput, error) {
	ids, err := parseULIDs(input.Body.IDs)
	if err != nil {
		return nil, huma.Error400BadRequest("invalid ID format", err)
	}

	exportAll := input.Body.ExportAll || len(ids) == 0
	export, err := h.exportService.ExportEncodingProfiles(ctx, ids, exportAll)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to export encoding profiles", err)
	}

	return &ExportEncodingProfilesOutput{Body: *export}, nil
}

// Import Preview Request/Response types

// ImportPreviewInput is the input for import preview endpoints.
type ImportPreviewInput struct {
	Body json.RawMessage `doc:"The exported configuration JSON to preview"`
}

// ImportPreviewOutput is the output for import preview endpoints.
type ImportPreviewOutput struct {
	Body models.ImportPreview
}

// Import preview handlers

// ImportFiltersPreview previews filter import.
func (h *ExportHandler) ImportFiltersPreview(ctx context.Context, input *ImportPreviewInput) (*ImportPreviewOutput, error) {
	var export models.ConfigExport[models.FilterExportItem]
	if err := json.Unmarshal(input.Body, &export); err != nil {
		return nil, huma.Error400BadRequest("invalid export format", err)
	}

	if err := validateExportMetadata(export.Metadata, models.ExportTypeFilters); err != nil {
		return nil, huma.Error400BadRequest("invalid export metadata", err)
	}

	preview, err := h.importService.ImportFiltersPreview(ctx, export.Items)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to preview import", err)
	}

	// Add version warning if applicable
	preview.VersionWarning = getVersionWarning(export.Metadata)

	return &ImportPreviewOutput{Body: *preview}, nil
}

// ImportDataMappingRulesPreview previews data mapping rules import.
func (h *ExportHandler) ImportDataMappingRulesPreview(ctx context.Context, input *ImportPreviewInput) (*ImportPreviewOutput, error) {
	var export models.ConfigExport[models.DataMappingRuleExportItem]
	if err := json.Unmarshal(input.Body, &export); err != nil {
		return nil, huma.Error400BadRequest("invalid export format", err)
	}

	if err := validateExportMetadata(export.Metadata, models.ExportTypeDataMappingRules); err != nil {
		return nil, huma.Error400BadRequest("invalid export metadata", err)
	}

	preview, err := h.importService.ImportDataMappingRulesPreview(ctx, export.Items)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to preview import", err)
	}

	// Add version warning if applicable
	preview.VersionWarning = getVersionWarning(export.Metadata)

	return &ImportPreviewOutput{Body: *preview}, nil
}

// ImportClientDetectionRulesPreview previews client detection rules import.
func (h *ExportHandler) ImportClientDetectionRulesPreview(ctx context.Context, input *ImportPreviewInput) (*ImportPreviewOutput, error) {
	var export models.ConfigExport[models.ClientDetectionRuleExportItem]
	if err := json.Unmarshal(input.Body, &export); err != nil {
		return nil, huma.Error400BadRequest("invalid export format", err)
	}

	if err := validateExportMetadata(export.Metadata, models.ExportTypeClientDetectionRules); err != nil {
		return nil, huma.Error400BadRequest("invalid export metadata", err)
	}

	preview, err := h.importService.ImportClientDetectionRulesPreview(ctx, export.Items)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to preview import", err)
	}

	// Add version warning if applicable
	preview.VersionWarning = getVersionWarning(export.Metadata)

	return &ImportPreviewOutput{Body: *preview}, nil
}

// ImportEncodingProfilesPreview previews encoding profiles import.
func (h *ExportHandler) ImportEncodingProfilesPreview(ctx context.Context, input *ImportPreviewInput) (*ImportPreviewOutput, error) {
	var export models.ConfigExport[models.EncodingProfileExportItem]
	if err := json.Unmarshal(input.Body, &export); err != nil {
		return nil, huma.Error400BadRequest("invalid export format", err)
	}

	if err := validateExportMetadata(export.Metadata, models.ExportTypeEncodingProfiles); err != nil {
		return nil, huma.Error400BadRequest("invalid export metadata", err)
	}

	preview, err := h.importService.ImportEncodingProfilesPreview(ctx, export.Items)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to preview import", err)
	}

	// Add version warning if applicable
	preview.VersionWarning = getVersionWarning(export.Metadata)

	return &ImportPreviewOutput{Body: *preview}, nil
}

// Import Execute Request/Response types

// ImportFiltersInput is the input for filter import.
type ImportFiltersInput struct {
	Body struct {
		Export         models.ConfigExport[models.FilterExportItem] `json:"export" doc:"The exported configuration"`
		Resolutions    map[string]models.ConflictResolution         `json:"resolutions,omitempty" doc:"Conflict resolution for each item by name"`
		BulkResolution models.ConflictResolution                    `json:"bulk_resolution,omitempty" doc:"Default resolution for all items not in resolutions map (skip, rename, overwrite)"`
	}
}

// ImportDataMappingRulesInput is the input for data mapping rules import.
type ImportDataMappingRulesInput struct {
	Body struct {
		Export         models.ConfigExport[models.DataMappingRuleExportItem] `json:"export" doc:"The exported configuration"`
		Resolutions    map[string]models.ConflictResolution                  `json:"resolutions,omitempty" doc:"Conflict resolution for each item by name"`
		BulkResolution models.ConflictResolution                             `json:"bulk_resolution,omitempty" doc:"Default resolution for all items not in resolutions map (skip, rename, overwrite)"`
	}
}

// ImportClientDetectionRulesInput is the input for client detection rules import.
type ImportClientDetectionRulesInput struct {
	Body struct {
		Export         models.ConfigExport[models.ClientDetectionRuleExportItem] `json:"export" doc:"The exported configuration"`
		Resolutions    map[string]models.ConflictResolution                      `json:"resolutions,omitempty" doc:"Conflict resolution for each item by name"`
		BulkResolution models.ConflictResolution                                 `json:"bulk_resolution,omitempty" doc:"Default resolution for all items not in resolutions map (skip, rename, overwrite)"`
	}
}

// ImportEncodingProfilesInput is the input for encoding profiles import.
type ImportEncodingProfilesInput struct {
	Body struct {
		Export         models.ConfigExport[models.EncodingProfileExportItem] `json:"export" doc:"The exported configuration"`
		Resolutions    map[string]models.ConflictResolution                  `json:"resolutions,omitempty" doc:"Conflict resolution for each item by name"`
		BulkResolution models.ConflictResolution                             `json:"bulk_resolution,omitempty" doc:"Default resolution for all items not in resolutions map (skip, rename, overwrite)"`
	}
}

// ImportResultOutput is the output for import endpoints.
type ImportResultOutput struct {
	Body models.ImportResult
}

// Import execute handlers

// ImportFilters imports filters.
func (h *ExportHandler) ImportFilters(ctx context.Context, input *ImportFiltersInput) (*ImportResultOutput, error) {
	if err := validateExportMetadata(input.Body.Export.Metadata, models.ExportTypeFilters); err != nil {
		return nil, huma.Error400BadRequest("invalid export metadata", err)
	}

	opts := &service.ImportOptions{
		Resolutions:    input.Body.Resolutions,
		BulkResolution: input.Body.BulkResolution,
	}
	result, err := h.importService.ImportFiltersWithOptions(ctx, input.Body.Export.Items, opts)
	if err != nil {
		// Return partial result even on error
		if result != nil {
			return &ImportResultOutput{Body: *result}, huma.Error500InternalServerError("import failed", err)
		}
		return nil, huma.Error500InternalServerError("import failed", err)
	}

	return &ImportResultOutput{Body: *result}, nil
}

// ImportDataMappingRules imports data mapping rules.
func (h *ExportHandler) ImportDataMappingRules(ctx context.Context, input *ImportDataMappingRulesInput) (*ImportResultOutput, error) {
	if err := validateExportMetadata(input.Body.Export.Metadata, models.ExportTypeDataMappingRules); err != nil {
		return nil, huma.Error400BadRequest("invalid export metadata", err)
	}

	opts := &service.ImportOptions{
		Resolutions:    input.Body.Resolutions,
		BulkResolution: input.Body.BulkResolution,
	}
	result, err := h.importService.ImportDataMappingRulesWithOptions(ctx, input.Body.Export.Items, opts)
	if err != nil {
		if result != nil {
			return &ImportResultOutput{Body: *result}, huma.Error500InternalServerError("import failed", err)
		}
		return nil, huma.Error500InternalServerError("import failed", err)
	}

	return &ImportResultOutput{Body: *result}, nil
}

// ImportClientDetectionRules imports client detection rules.
func (h *ExportHandler) ImportClientDetectionRules(ctx context.Context, input *ImportClientDetectionRulesInput) (*ImportResultOutput, error) {
	if err := validateExportMetadata(input.Body.Export.Metadata, models.ExportTypeClientDetectionRules); err != nil {
		return nil, huma.Error400BadRequest("invalid export metadata", err)
	}

	opts := &service.ImportOptions{
		Resolutions:    input.Body.Resolutions,
		BulkResolution: input.Body.BulkResolution,
	}
	result, err := h.importService.ImportClientDetectionRulesWithOptions(ctx, input.Body.Export.Items, opts)
	if err != nil {
		if result != nil {
			return &ImportResultOutput{Body: *result}, huma.Error500InternalServerError("import failed", err)
		}
		return nil, huma.Error500InternalServerError("import failed", err)
	}

	return &ImportResultOutput{Body: *result}, nil
}

// ImportEncodingProfiles imports encoding profiles.
func (h *ExportHandler) ImportEncodingProfiles(ctx context.Context, input *ImportEncodingProfilesInput) (*ImportResultOutput, error) {
	if err := validateExportMetadata(input.Body.Export.Metadata, models.ExportTypeEncodingProfiles); err != nil {
		return nil, huma.Error400BadRequest("invalid export metadata", err)
	}

	opts := &service.ImportOptions{
		Resolutions:    input.Body.Resolutions,
		BulkResolution: input.Body.BulkResolution,
	}
	result, err := h.importService.ImportEncodingProfilesWithOptions(ctx, input.Body.Export.Items, opts)
	if err != nil {
		if result != nil {
			return &ImportResultOutput{Body: *result}, huma.Error500InternalServerError("import failed", err)
		}
		return nil, huma.Error500InternalServerError("import failed", err)
	}

	return &ImportResultOutput{Body: *result}, nil
}

// Helper functions

func parseULIDs(ids []string) ([]models.ULID, error) {
	result := make([]models.ULID, 0, len(ids))
	for _, idStr := range ids {
		id, err := models.ParseULID(idStr)
		if err != nil {
			return nil, fmt.Errorf("invalid ID %q: %w", idStr, err)
		}
		result = append(result, id)
	}
	return result, nil
}

func validateExportMetadata(meta models.ExportMetadata, expectedType models.ExportType) error {
	if meta.Version != models.ExportFormatVersion {
		return fmt.Errorf("unsupported export version: %s (expected %s)", meta.Version, models.ExportFormatVersion)
	}
	if meta.ExportType != expectedType {
		return fmt.Errorf("export type mismatch: %s (expected %s)", meta.ExportType, expectedType)
	}
	return nil
}

// getVersionWarning returns a warning message if the export was created with a different tvarr version.
func getVersionWarning(meta models.ExportMetadata) string {
	currentVersion := version.Version
	exportVersion := meta.TvarrVersion

	// No warning if versions match or export version is empty
	if exportVersion == "" || exportVersion == currentVersion {
		return ""
	}

	return fmt.Sprintf("This export was created with tvarr version %s. You are running version %s. Some items may not import correctly if there are format differences between versions.",
		exportVersion, currentVersion)
}
