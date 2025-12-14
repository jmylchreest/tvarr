package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/service"
)

// BackupHandler handles backup and restore API endpoints.
type BackupHandler struct {
	backupService *service.BackupService
}

// NewBackupHandler creates a new backup handler.
func NewBackupHandler(backupService *service.BackupService) *BackupHandler {
	return &BackupHandler{
		backupService: backupService,
	}
}

// Register registers the backup routes with the Huma API.
func (h *BackupHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listBackups",
		Method:      "GET",
		Path:        "/api/v1/backups",
		Summary:     "List all backups",
		Description: "Returns a list of all available database backups sorted by creation time (newest first)",
		Tags:        []string{"Backup"},
	}, h.ListBackups)

	huma.Register(api, huma.Operation{
		OperationID: "createBackup",
		Method:      "POST",
		Path:        "/api/v1/backups",
		Summary:     "Create a backup",
		Description: "Creates a new full database backup with gzip compression",
		Tags:        []string{"Backup"},
	}, h.CreateBackup)

	huma.Register(api, huma.Operation{
		OperationID: "getBackup",
		Method:      "GET",
		Path:        "/api/v1/backups/{filename}",
		Summary:     "Get backup details",
		Description: "Returns metadata for a specific backup",
		Tags:        []string{"Backup"},
	}, h.GetBackup)

	huma.Register(api, huma.Operation{
		OperationID: "deleteBackup",
		Method:      "DELETE",
		Path:        "/api/v1/backups/{filename}",
		Summary:     "Delete a backup",
		Description: "Deletes a backup file and its metadata",
		Tags:        []string{"Backup"},
	}, h.DeleteBackup)

	huma.Register(api, huma.Operation{
		OperationID: "restoreBackup",
		Method:      "POST",
		Path:        "/api/v1/backups/{filename}/restore",
		Summary:     "Restore from backup",
		Description: "Restores the database from a backup file. Requires confirm=true query parameter. Creates a pre-restore backup for rollback capability. NOTE: Application restart may be required after restore.",
		Tags:        []string{"Backup"},
	}, h.RestoreBackup)
}

// RegisterChiRoutes registers Chi-specific routes for file downloads and uploads.
func (h *BackupHandler) RegisterChiRoutes(r chi.Router) {
	r.Get("/api/v1/backups/{filename}/download", h.DownloadBackup)
	r.Post("/api/v1/backups/upload", h.UploadBackup)
}

// List types

// ListBackupsInput is the input for listing backups.
type ListBackupsInput struct{}

// ListBackupsOutput is the output for listing backups.
type ListBackupsOutput struct {
	Body struct {
		Backups         []*models.BackupMetadata  `json:"backups"`
		BackupDirectory string                    `json:"backup_directory"`
		Schedule        models.BackupScheduleInfo `json:"schedule"`
	}
}

// ListBackups lists all available backups.
func (h *BackupHandler) ListBackups(ctx context.Context, _ *ListBackupsInput) (*ListBackupsOutput, error) {
	backups, err := h.backupService.ListBackups(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list backups", err)
	}

	return &ListBackupsOutput{
		Body: struct {
			Backups         []*models.BackupMetadata  `json:"backups"`
			BackupDirectory string                    `json:"backup_directory"`
			Schedule        models.BackupScheduleInfo `json:"schedule"`
		}{
			Backups:         backups,
			BackupDirectory: h.backupService.GetBackupDirectory(),
			Schedule:        h.backupService.GetScheduleInfo(),
		},
	}, nil
}

// Create types

// CreateBackupInput is the input for creating a backup.
type CreateBackupInput struct{}

// CreateBackupOutput is the output for creating a backup.
type CreateBackupOutput struct {
	Body *models.BackupMetadata
}

// CreateBackup creates a new database backup.
func (h *BackupHandler) CreateBackup(ctx context.Context, _ *CreateBackupInput) (*CreateBackupOutput, error) {
	meta, err := h.backupService.CreateBackup(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to create backup", err)
	}

	return &CreateBackupOutput{Body: meta}, nil
}

// Get types

// GetBackupInput is the input for getting backup details.
type GetBackupInput struct {
	Filename string `path:"filename" doc:"Backup filename (e.g., tvarr-backup-2024-01-01T12-00-00.db.gz)"`
}

// GetBackupOutput is the output for getting backup details.
type GetBackupOutput struct {
	Body *models.BackupMetadata
}

// GetBackup returns metadata for a specific backup.
func (h *BackupHandler) GetBackup(ctx context.Context, input *GetBackupInput) (*GetBackupOutput, error) {
	// Validate filename (prevent path traversal)
	if err := validateBackupFilename(input.Filename); err != nil {
		return nil, huma.Error400BadRequest("invalid filename", err)
	}

	meta, err := h.backupService.GetBackup(ctx, input.Filename)
	if err != nil {
		return nil, huma.Error404NotFound("backup not found")
	}

	return &GetBackupOutput{Body: meta}, nil
}

// Delete types

// DeleteBackupInput is the input for deleting a backup.
type DeleteBackupInput struct {
	Filename string `path:"filename" doc:"Backup filename"`
}

// DeleteBackupOutput is the output for deleting a backup.
type DeleteBackupOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

// DeleteBackup deletes a backup file.
func (h *BackupHandler) DeleteBackup(ctx context.Context, input *DeleteBackupInput) (*DeleteBackupOutput, error) {
	if err := validateBackupFilename(input.Filename); err != nil {
		return nil, huma.Error400BadRequest("invalid filename", err)
	}

	if err := h.backupService.DeleteBackup(ctx, input.Filename); err != nil {
		return nil, huma.Error500InternalServerError("failed to delete backup", err)
	}

	return &DeleteBackupOutput{
		Body: struct {
			Message string `json:"message"`
		}{
			Message: fmt.Sprintf("backup %s deleted", input.Filename),
		},
	}, nil
}

// Restore types

// RestoreBackupInput is the input for restoring from a backup.
type RestoreBackupInput struct {
	Filename string `path:"filename" doc:"Backup filename to restore from"`
	Confirm  bool   `query:"confirm" doc:"Must be true to confirm restore operation"`
}

// RestoreBackupOutput is the output for restoring from a backup.
type RestoreBackupOutput struct {
	Body struct {
		Message           string `json:"message"`
		PreRestoreBackup  string `json:"pre_restore_backup,omitempty"`
		RestartRequired   bool   `json:"restart_required"`
	}
}

// RestoreBackup restores the database from a backup.
func (h *BackupHandler) RestoreBackup(ctx context.Context, input *RestoreBackupInput) (*RestoreBackupOutput, error) {
	if err := validateBackupFilename(input.Filename); err != nil {
		return nil, huma.Error400BadRequest("invalid filename", err)
	}

	if !input.Confirm {
		return nil, huma.Error400BadRequest("restore requires confirmation", fmt.Errorf("set confirm=true query parameter to proceed"))
	}

	if err := h.backupService.RestoreBackup(ctx, input.Filename); err != nil {
		return nil, huma.Error500InternalServerError("failed to restore backup", err)
	}

	return &RestoreBackupOutput{
		Body: struct {
			Message           string `json:"message"`
			PreRestoreBackup  string `json:"pre_restore_backup,omitempty"`
			RestartRequired   bool   `json:"restart_required"`
		}{
			Message:         fmt.Sprintf("database restored from %s", input.Filename),
			RestartRequired: true, // SQLite requires app restart after restore
		},
	}, nil
}

// UploadBackup handles uploading a backup file for later restore.
// This uses Chi directly because Huma doesn't handle multipart file uploads well.
func (h *BackupHandler) UploadBackup(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form with 100MB max size
	const maxUploadSize = 100 << 20 // 100MB
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		writeJSONError(w, fmt.Sprintf("failed to parse form: %v", err), http.StatusBadRequest)
		return
	}

	// Get the file from the form
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSONError(w, fmt.Sprintf("failed to get file: %v", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Import the backup
	meta, err := h.backupService.ImportBackup(r.Context(), file, header.Filename)
	if err != nil {
		// Determine appropriate status code based on error
		if containsAny(err.Error(), []string{"invalid filename", "already exists", "invalid filename format"}) {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
		} else {
			writeJSONError(w, fmt.Sprintf("failed to import backup: %v", err), http.StatusInternalServerError)
		}
		return
	}

	// Return success with metadata
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(meta)
}

// writeJSONError writes an error response in JSON format for consistency with API clients.
func writeJSONError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// containsAny checks if the string contains any of the substrings.
func containsAny(s string, substrings []string) bool {
	for _, sub := range substrings {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
	}
	return false
}

// DownloadBackup streams a backup file for download.
// This uses Chi directly because Huma doesn't handle file streaming well.
func (h *BackupHandler) DownloadBackup(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")

	if err := validateBackupFilename(filename); err != nil {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}

	file, err := h.backupService.OpenBackupFile(r.Context(), filename)
	if err != nil {
		http.Error(w, "backup not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	// Get file info for Content-Length
	info, err := file.Stat()
	if err != nil {
		http.Error(w, "failed to stat backup file", http.StatusInternalServerError)
		return
	}

	// Set headers for file download
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))

	// Stream the file
	if _, err := io.Copy(w, file); err != nil {
		// Client may have disconnected, log but don't return error
		return
	}
}

// validateBackupFilename checks for path traversal attempts.
func validateBackupFilename(filename string) error {
	if filename == "" {
		return fmt.Errorf("filename is required")
	}

	// Check for path traversal
	if containsPathTraversal(filename) {
		return fmt.Errorf("invalid filename: path traversal detected")
	}

	// Check for valid backup filename pattern
	if !isValidBackupFilename(filename) {
		return fmt.Errorf("invalid backup filename format")
	}

	return nil
}

// containsPathTraversal checks if a filename contains path traversal attempts.
func containsPathTraversal(filename string) bool {
	// Check for common path traversal patterns
	for _, pattern := range []string{"..", "/", "\\"} {
		for i := 0; i <= len(filename)-len(pattern); i++ {
			if filename[i:i+len(pattern)] == pattern {
				return true
			}
		}
	}
	return false
}

// isValidBackupFilename checks if a filename matches the expected backup format.
func isValidBackupFilename(filename string) bool {
	// Expected formats:
	// - tvarr-backup-YYYY-MM-DDTHH-MM-SS.db.gz (34 chars)
	// - tvarr-backup-YYYY-MM-DDTHH-MM-SS.mmm.db.gz (38 chars)
	if len(filename) < 34 { // Minimum length for valid filename
		return false
	}

	// Must start with tvarr-backup-
	prefix := "tvarr-backup-"
	if len(filename) < len(prefix) || filename[:len(prefix)] != prefix {
		return false
	}

	// Must end with .db.gz
	suffix := ".db.gz"
	if len(filename) < len(suffix) || filename[len(filename)-len(suffix):] != suffix {
		return false
	}

	return true
}
