// Package service provides business logic layer for tvarr operations.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
	"gorm.io/gorm"
)

// ImportService provides business logic for importing configuration items.
type ImportService struct {
	db                       *gorm.DB
	filterRepo               repository.FilterRepository
	dataMappingRuleRepo      repository.DataMappingRuleRepository
	clientDetectionRuleRepo  repository.ClientDetectionRuleRepository
	encodingProfileRepo      repository.EncodingProfileRepository
	logger                   *slog.Logger
}

// NewImportService creates a new import service.
func NewImportService(
	db *gorm.DB,
	filterRepo repository.FilterRepository,
	dataMappingRuleRepo repository.DataMappingRuleRepository,
	clientDetectionRuleRepo repository.ClientDetectionRuleRepository,
	encodingProfileRepo repository.EncodingProfileRepository,
) *ImportService {
	return &ImportService{
		db:                       db,
		filterRepo:               filterRepo,
		dataMappingRuleRepo:      dataMappingRuleRepo,
		clientDetectionRuleRepo:  clientDetectionRuleRepo,
		encodingProfileRepo:      encodingProfileRepo,
		logger:                   slog.Default(),
	}
}

// WithLogger sets the logger for the service.
func (s *ImportService) WithLogger(logger *slog.Logger) *ImportService {
	s.logger = logger
	return s
}

// ImportOptions configures import behavior.
type ImportOptions struct {
	// Resolutions maps item names to specific conflict resolutions.
	Resolutions map[string]models.ConflictResolution
	// BulkResolution is applied to all conflicts not in Resolutions map.
	// If empty, defaults to "skip".
	BulkResolution models.ConflictResolution
}

// getResolution returns the resolution for a given item name.
// It first checks the Resolutions map, then falls back to BulkResolution.
func (o *ImportOptions) getResolution(name string) models.ConflictResolution {
	if o == nil {
		return models.ConflictResolutionSkip
	}
	if res, ok := o.Resolutions[name]; ok {
		return res
	}
	if o.BulkResolution != "" {
		return o.BulkResolution
	}
	return models.ConflictResolutionSkip
}

// ImportFiltersPreview returns a preview of what will happen on import.
func (s *ImportService) ImportFiltersPreview(ctx context.Context, items []models.FilterExportItem) (*models.ImportPreview, error) {
	preview := &models.ImportPreview{
		TotalItems: len(items),
		NewItems:   make([]models.PreviewItem, 0),
		Conflicts:  make([]models.ConflictItem, 0),
		Errors:     make([]models.ImportError, 0),
	}

	// Check each item for conflicts
	for _, item := range items {
		// Validate expression (basic validation)
		if item.Expression == "" {
			preview.Errors = append(preview.Errors, models.ImportError{
				ItemName: item.Name,
				Error:    "expression is required",
			})
			continue
		}

		// Check for name conflict
		existing, err := s.filterRepo.GetByName(ctx, item.Name)
		if err != nil {
			preview.Errors = append(preview.Errors, models.ImportError{
				ItemName: item.Name,
				Error:    err.Error(),
			})
			continue
		}

		if existing != nil {
			preview.Conflicts = append(preview.Conflicts, models.ConflictItem{
				ImportName:   item.Name,
				ExistingID:   existing.ID.String(),
				ExistingName: existing.Name,
				Resolution:   models.ConflictResolutionSkip, // Default
			})
		} else {
			preview.NewItems = append(preview.NewItems, models.PreviewItem{
				Name: item.Name,
			})
		}
	}

	return preview, nil
}

// ImportFilters imports filters with specified conflict resolutions.
// For bulk resolution support, use ImportFiltersWithOptions instead.
func (s *ImportService) ImportFilters(
	ctx context.Context,
	items []models.FilterExportItem,
	resolutions map[string]models.ConflictResolution,
) (*models.ImportResult, error) {
	return s.ImportFiltersWithOptions(ctx, items, &ImportOptions{Resolutions: resolutions})
}

// ImportFiltersWithOptions imports filters with full options including bulk resolution.
func (s *ImportService) ImportFiltersWithOptions(
	ctx context.Context,
	items []models.FilterExportItem,
	opts *ImportOptions,
) (*models.ImportResult, error) {
	result := &models.ImportResult{
		TotalItems:    len(items),
		ImportedItems: make([]models.ImportedItem, 0),
		ErrorDetails:  make([]models.ImportError, 0),
	}

	// Use transaction for atomicity
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			imported, err := s.importSingleFilterWithOpts(ctx, tx, item, opts)
			if err != nil {
				result.Errors++
				result.ErrorDetails = append(result.ErrorDetails, models.ImportError{
					ItemName: item.Name,
					Error:    err.Error(),
				})
				continue
			}

			if imported != nil {
				result.ImportedItems = append(result.ImportedItems, *imported)
				switch imported.Action {
				case "created":
					result.Imported++
				case "overwritten":
					result.Overwritten++
				case "renamed":
					result.Renamed++
				}
			} else {
				result.Skipped++
			}
		}

		// Rollback if any errors occurred (atomic import)
		if result.Errors > 0 {
			return fmt.Errorf("import failed with %d errors", result.Errors)
		}

		return nil
	})

	if err != nil {
		return result, err
	}

	s.logger.Info("imported filters",
		slog.Int("total", result.TotalItems),
		slog.Int("imported", result.Imported),
		slog.Int("skipped", result.Skipped),
		slog.Int("overwritten", result.Overwritten),
		slog.Int("renamed", result.Renamed),
	)

	return result, nil
}

func (s *ImportService) importSingleFilterWithOpts(
	ctx context.Context,
	tx *gorm.DB,
	item models.FilterExportItem,
	opts *ImportOptions,
) (*models.ImportedItem, error) {
	// Check for existing filter with same name
	var existing models.Filter
	err := tx.Where("name = ?", item.Name).First(&existing).Error

	if err == nil {
		// Conflict exists - get resolution from options
		resolution := opts.getResolution(item.Name)

		switch resolution {
		case models.ConflictResolutionSkip:
			return nil, nil // Skip this item

		case models.ConflictResolutionRename:
			// Find unique name
			newName := s.findUniqueFilterName(tx, item.Name)
			filter := filterFromExportItem(item)
			filter.Name = newName
			if err := tx.Create(&filter).Error; err != nil {
				return nil, err
			}
			return &models.ImportedItem{
				OriginalName: item.Name,
				FinalName:    newName,
				ID:           filter.ID.String(),
				Action:       "renamed",
			}, nil

		case models.ConflictResolutionOverwrite:
			// Update existing record, preserving ID
			updateFilter(&existing, item)
			if err := tx.Save(&existing).Error; err != nil {
				return nil, err
			}
			return &models.ImportedItem{
				OriginalName: item.Name,
				FinalName:    item.Name,
				ID:           existing.ID.String(),
				Action:       "overwritten",
			}, nil
		}
	} else if err == gorm.ErrRecordNotFound {
		// No conflict - create new
		filter := filterFromExportItem(item)
		filter.ID = models.NewULID()
		if err := tx.Create(&filter).Error; err != nil {
			return nil, err
		}
		return &models.ImportedItem{
			OriginalName: item.Name,
			FinalName:    item.Name,
			ID:           filter.ID.String(),
			Action:       "created",
		}, nil
	}

	return nil, err
}

func (s *ImportService) findUniqueFilterName(tx *gorm.DB, baseName string) string {
	suffix := 1
	for {
		newName := fmt.Sprintf("%s (%d)", baseName, suffix)
		var count int64
		tx.Model(&models.Filter{}).Where("name = ?", newName).Count(&count)
		if count == 0 {
			return newName
		}
		suffix++
	}
}

func filterFromExportItem(item models.FilterExportItem) models.Filter {
	enabled := item.IsEnabled
	var sourceID *models.ULID
	if item.SourceID != nil {
		id, err := models.ParseULID(*item.SourceID)
		if err == nil {
			sourceID = &id
		}
	}
	return models.Filter{
		Name:        item.Name,
		Description: item.Description,
		Expression:  item.Expression,
		SourceType:  models.FilterSourceType(item.SourceType),
		Action:      models.FilterAction(item.Action),
		IsEnabled:   &enabled,
		IsSystem:    false, // Always user-created on import
		SourceID:    sourceID,
	}
}

func updateFilter(existing *models.Filter, item models.FilterExportItem) {
	enabled := item.IsEnabled
	existing.Description = item.Description
	existing.Expression = item.Expression
	existing.SourceType = models.FilterSourceType(item.SourceType)
	existing.Action = models.FilterAction(item.Action)
	existing.IsEnabled = &enabled
	if item.SourceID != nil {
		id, err := models.ParseULID(*item.SourceID)
		if err == nil {
			existing.SourceID = &id
		}
	} else {
		existing.SourceID = nil
	}
}

// ImportDataMappingRulesPreview returns a preview of what will happen on import.
func (s *ImportService) ImportDataMappingRulesPreview(ctx context.Context, items []models.DataMappingRuleExportItem) (*models.ImportPreview, error) {
	preview := &models.ImportPreview{
		TotalItems: len(items),
		NewItems:   make([]models.PreviewItem, 0),
		Conflicts:  make([]models.ConflictItem, 0),
		Errors:     make([]models.ImportError, 0),
	}

	for _, item := range items {
		if item.Expression == "" {
			preview.Errors = append(preview.Errors, models.ImportError{
				ItemName: item.Name,
				Error:    "expression is required",
			})
			continue
		}

		existing, err := s.dataMappingRuleRepo.GetByName(ctx, item.Name)
		if err != nil {
			preview.Errors = append(preview.Errors, models.ImportError{
				ItemName: item.Name,
				Error:    err.Error(),
			})
			continue
		}

		if existing != nil {
			preview.Conflicts = append(preview.Conflicts, models.ConflictItem{
				ImportName:   item.Name,
				ExistingID:   existing.ID.String(),
				ExistingName: existing.Name,
				Resolution:   models.ConflictResolutionSkip,
			})
		} else {
			preview.NewItems = append(preview.NewItems, models.PreviewItem{
				Name: item.Name,
			})
		}
	}

	return preview, nil
}

// ImportDataMappingRules imports data mapping rules with specified conflict resolutions.
// For bulk resolution support, use ImportDataMappingRulesWithOptions instead.
func (s *ImportService) ImportDataMappingRules(
	ctx context.Context,
	items []models.DataMappingRuleExportItem,
	resolutions map[string]models.ConflictResolution,
) (*models.ImportResult, error) {
	return s.ImportDataMappingRulesWithOptions(ctx, items, &ImportOptions{Resolutions: resolutions})
}

// ImportDataMappingRulesWithOptions imports data mapping rules with full options including bulk resolution.
func (s *ImportService) ImportDataMappingRulesWithOptions(
	ctx context.Context,
	items []models.DataMappingRuleExportItem,
	opts *ImportOptions,
) (*models.ImportResult, error) {
	result := &models.ImportResult{
		TotalItems:    len(items),
		ImportedItems: make([]models.ImportedItem, 0),
		ErrorDetails:  make([]models.ImportError, 0),
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			imported, err := s.importSingleDataMappingRuleWithOpts(ctx, tx, item, opts)
			if err != nil {
				result.Errors++
				result.ErrorDetails = append(result.ErrorDetails, models.ImportError{
					ItemName: item.Name,
					Error:    err.Error(),
				})
				continue
			}

			if imported != nil {
				result.ImportedItems = append(result.ImportedItems, *imported)
				switch imported.Action {
				case "created":
					result.Imported++
				case "overwritten":
					result.Overwritten++
				case "renamed":
					result.Renamed++
				}
			} else {
				result.Skipped++
			}
		}

		if result.Errors > 0 {
			return fmt.Errorf("import failed with %d errors", result.Errors)
		}

		return nil
	})

	if err != nil {
		return result, err
	}

	s.logger.Info("imported data mapping rules",
		slog.Int("total", result.TotalItems),
		slog.Int("imported", result.Imported),
	)

	return result, nil
}

func (s *ImportService) importSingleDataMappingRuleWithOpts(
	ctx context.Context,
	tx *gorm.DB,
	item models.DataMappingRuleExportItem,
	opts *ImportOptions,
) (*models.ImportedItem, error) {
	var existing models.DataMappingRule
	err := tx.Where("name = ?", item.Name).First(&existing).Error

	if err == nil {
		resolution := opts.getResolution(item.Name)

		switch resolution {
		case models.ConflictResolutionSkip:
			return nil, nil

		case models.ConflictResolutionRename:
			newName := s.findUniqueDataMappingRuleName(tx, item.Name)
			rule := dataMappingRuleFromExportItem(item)
			rule.Name = newName
			if err := tx.Create(&rule).Error; err != nil {
				return nil, err
			}
			return &models.ImportedItem{
				OriginalName: item.Name,
				FinalName:    newName,
				ID:           rule.ID.String(),
				Action:       "renamed",
			}, nil

		case models.ConflictResolutionOverwrite:
			updateDataMappingRule(&existing, item)
			if err := tx.Save(&existing).Error; err != nil {
				return nil, err
			}
			return &models.ImportedItem{
				OriginalName: item.Name,
				FinalName:    item.Name,
				ID:           existing.ID.String(),
				Action:       "overwritten",
			}, nil
		}
	} else if err == gorm.ErrRecordNotFound {
		rule := dataMappingRuleFromExportItem(item)
		rule.ID = models.NewULID()
		if err := tx.Create(&rule).Error; err != nil {
			return nil, err
		}
		return &models.ImportedItem{
			OriginalName: item.Name,
			FinalName:    item.Name,
			ID:           rule.ID.String(),
			Action:       "created",
		}, nil
	}

	return nil, err
}

func (s *ImportService) findUniqueDataMappingRuleName(tx *gorm.DB, baseName string) string {
	suffix := 1
	for {
		newName := fmt.Sprintf("%s (%d)", baseName, suffix)
		var count int64
		tx.Model(&models.DataMappingRule{}).Where("name = ?", newName).Count(&count)
		if count == 0 {
			return newName
		}
		suffix++
	}
}

func dataMappingRuleFromExportItem(item models.DataMappingRuleExportItem) models.DataMappingRule {
	enabled := item.IsEnabled
	var sourceID *models.ULID
	if item.SourceID != nil {
		id, err := models.ParseULID(*item.SourceID)
		if err == nil {
			sourceID = &id
		}
	}
	return models.DataMappingRule{
		Name:        item.Name,
		Description: item.Description,
		Expression:  item.Expression,
		SourceType:  models.DataMappingRuleSourceType(item.SourceType),
		Priority:    item.Priority,
		StopOnMatch: item.StopOnMatch,
		IsEnabled:   &enabled,
		IsSystem:    false,
		SourceID:    sourceID,
	}
}

func updateDataMappingRule(existing *models.DataMappingRule, item models.DataMappingRuleExportItem) {
	enabled := item.IsEnabled
	existing.Description = item.Description
	existing.Expression = item.Expression
	existing.SourceType = models.DataMappingRuleSourceType(item.SourceType)
	existing.Priority = item.Priority
	existing.StopOnMatch = item.StopOnMatch
	existing.IsEnabled = &enabled
	if item.SourceID != nil {
		id, err := models.ParseULID(*item.SourceID)
		if err == nil {
			existing.SourceID = &id
		}
	} else {
		existing.SourceID = nil
	}
}

// ImportClientDetectionRulesPreview returns a preview of what will happen on import.
func (s *ImportService) ImportClientDetectionRulesPreview(ctx context.Context, items []models.ClientDetectionRuleExportItem) (*models.ImportPreview, error) {
	preview := &models.ImportPreview{
		TotalItems: len(items),
		NewItems:   make([]models.PreviewItem, 0),
		Conflicts:  make([]models.ConflictItem, 0),
		Errors:     make([]models.ImportError, 0),
	}

	for _, item := range items {
		if item.Expression == "" {
			preview.Errors = append(preview.Errors, models.ImportError{
				ItemName: item.Name,
				Error:    "expression is required",
			})
			continue
		}

		existing, err := s.clientDetectionRuleRepo.GetByName(ctx, item.Name)
		if err != nil {
			preview.Errors = append(preview.Errors, models.ImportError{
				ItemName: item.Name,
				Error:    err.Error(),
			})
			continue
		}

		if existing != nil {
			preview.Conflicts = append(preview.Conflicts, models.ConflictItem{
				ImportName:   item.Name,
				ExistingID:   existing.ID.String(),
				ExistingName: existing.Name,
				Resolution:   models.ConflictResolutionSkip,
			})
		} else {
			preview.NewItems = append(preview.NewItems, models.PreviewItem{
				Name: item.Name,
			})
		}
	}

	return preview, nil
}

// ImportClientDetectionRules imports client detection rules with specified conflict resolutions.
// For bulk resolution support, use ImportClientDetectionRulesWithOptions instead.
func (s *ImportService) ImportClientDetectionRules(
	ctx context.Context,
	items []models.ClientDetectionRuleExportItem,
	resolutions map[string]models.ConflictResolution,
) (*models.ImportResult, error) {
	return s.ImportClientDetectionRulesWithOptions(ctx, items, &ImportOptions{Resolutions: resolutions})
}

// ImportClientDetectionRulesWithOptions imports client detection rules with full options including bulk resolution.
func (s *ImportService) ImportClientDetectionRulesWithOptions(
	ctx context.Context,
	items []models.ClientDetectionRuleExportItem,
	opts *ImportOptions,
) (*models.ImportResult, error) {
	result := &models.ImportResult{
		TotalItems:    len(items),
		ImportedItems: make([]models.ImportedItem, 0),
		ErrorDetails:  make([]models.ImportError, 0),
	}

	// Pre-resolve all encoding profile names to IDs BEFORE starting transaction
	// This avoids deadlock issues with SQLite single-connection in-memory databases
	profileIDMap := make(map[string]*models.ULID)
	for _, item := range items {
		if item.EncodingProfileName != nil && *item.EncodingProfileName != "" {
			if _, exists := profileIDMap[*item.EncodingProfileName]; !exists {
				profile, err := s.encodingProfileRepo.GetByName(ctx, *item.EncodingProfileName)
				if err != nil {
					s.logger.Warn("encoding profile lookup error for client detection rule",
						slog.String("rule", item.Name),
						slog.String("profile", *item.EncodingProfileName),
						slog.String("error", err.Error()),
					)
					profileIDMap[*item.EncodingProfileName] = nil
				} else if profile != nil {
					profileIDMap[*item.EncodingProfileName] = &profile.ID
				} else {
					s.logger.Debug("encoding profile not found for client detection rule",
						slog.String("rule", item.Name),
						slog.String("profile", *item.EncodingProfileName),
					)
					profileIDMap[*item.EncodingProfileName] = nil
				}
			}
		}
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			imported, err := s.importSingleClientDetectionRuleWithOpts(ctx, tx, item, opts, profileIDMap)
			if err != nil {
				result.Errors++
				result.ErrorDetails = append(result.ErrorDetails, models.ImportError{
					ItemName: item.Name,
					Error:    err.Error(),
				})
				continue
			}

			if imported != nil {
				result.ImportedItems = append(result.ImportedItems, *imported)
				switch imported.Action {
				case "created":
					result.Imported++
				case "overwritten":
					result.Overwritten++
				case "renamed":
					result.Renamed++
				}
			} else {
				result.Skipped++
			}
		}

		if result.Errors > 0 {
			return fmt.Errorf("import failed with %d errors", result.Errors)
		}

		return nil
	})

	if err != nil {
		return result, err
	}

	s.logger.Info("imported client detection rules",
		slog.Int("total", result.TotalItems),
		slog.Int("imported", result.Imported),
	)

	return result, nil
}

func (s *ImportService) importSingleClientDetectionRuleWithOpts(
	ctx context.Context,
	tx *gorm.DB,
	item models.ClientDetectionRuleExportItem,
	opts *ImportOptions,
	profileIDMap map[string]*models.ULID,
) (*models.ImportedItem, error) {
	// Look up encoding profile ID from pre-resolved map
	var encodingProfileID *models.ULID
	if item.EncodingProfileName != nil && *item.EncodingProfileName != "" {
		if profileID, exists := profileIDMap[*item.EncodingProfileName]; exists && profileID != nil {
			encodingProfileID = profileID
		}
	}

	var existing models.ClientDetectionRule
	err := tx.Where("name = ?", item.Name).First(&existing).Error

	if err == nil {
		resolution := opts.getResolution(item.Name)

		switch resolution {
		case models.ConflictResolutionSkip:
			return nil, nil

		case models.ConflictResolutionRename:
			newName := s.findUniqueClientDetectionRuleName(tx, item.Name)
			rule := clientDetectionRuleFromExportItem(item, encodingProfileID)
			rule.Name = newName
			if err := tx.Create(&rule).Error; err != nil {
				return nil, err
			}
			return &models.ImportedItem{
				OriginalName: item.Name,
				FinalName:    newName,
				ID:           rule.ID.String(),
				Action:       "renamed",
			}, nil

		case models.ConflictResolutionOverwrite:
			updateClientDetectionRule(&existing, item, encodingProfileID)
			if err := tx.Save(&existing).Error; err != nil {
				return nil, err
			}
			return &models.ImportedItem{
				OriginalName: item.Name,
				FinalName:    item.Name,
				ID:           existing.ID.String(),
				Action:       "overwritten",
			}, nil
		}
	} else if err == gorm.ErrRecordNotFound {
		rule := clientDetectionRuleFromExportItem(item, encodingProfileID)
		rule.ID = models.NewULID()
		if err := tx.Create(&rule).Error; err != nil {
			return nil, err
		}
		return &models.ImportedItem{
			OriginalName: item.Name,
			FinalName:    item.Name,
			ID:           rule.ID.String(),
			Action:       "created",
		}, nil
	}

	return nil, err
}

func (s *ImportService) findUniqueClientDetectionRuleName(tx *gorm.DB, baseName string) string {
	suffix := 1
	for {
		newName := fmt.Sprintf("%s (%d)", baseName, suffix)
		var count int64
		tx.Model(&models.ClientDetectionRule{}).Where("name = ?", newName).Count(&count)
		if count == 0 {
			return newName
		}
		suffix++
	}
}

func clientDetectionRuleFromExportItem(item models.ClientDetectionRuleExportItem, encodingProfileID *models.ULID) models.ClientDetectionRule {
	enabled := item.IsEnabled
	supportsFMP4 := item.SupportsFMP4
	supportsMPEGTS := item.SupportsMPEGTS

	// Encode codec arrays to JSON strings
	var acceptedVideoCodecs string
	if len(item.AcceptedVideoCodecs) > 0 {
		data, _ := json.Marshal(item.AcceptedVideoCodecs)
		acceptedVideoCodecs = string(data)
	}
	var acceptedAudioCodecs string
	if len(item.AcceptedAudioCodecs) > 0 {
		data, _ := json.Marshal(item.AcceptedAudioCodecs)
		acceptedAudioCodecs = string(data)
	}

	return models.ClientDetectionRule{
		Name:                item.Name,
		Description:         item.Description,
		Expression:          item.Expression,
		Priority:            item.Priority,
		IsEnabled:           &enabled,
		IsSystem:            false,
		AcceptedVideoCodecs: acceptedVideoCodecs,
		AcceptedAudioCodecs: acceptedAudioCodecs,
		PreferredVideoCodec: models.VideoCodec(item.PreferredVideoCodec),
		PreferredAudioCodec: models.AudioCodec(item.PreferredAudioCodec),
		SupportsFMP4:        &supportsFMP4,
		SupportsMPEGTS:      &supportsMPEGTS,
		PreferredFormat:     item.PreferredFormat,
		EncodingProfileID:   encodingProfileID,
	}
}

func updateClientDetectionRule(existing *models.ClientDetectionRule, item models.ClientDetectionRuleExportItem, encodingProfileID *models.ULID) {
	enabled := item.IsEnabled
	supportsFMP4 := item.SupportsFMP4
	supportsMPEGTS := item.SupportsMPEGTS

	// Encode codec arrays to JSON strings
	var acceptedVideoCodecs string
	if len(item.AcceptedVideoCodecs) > 0 {
		data, _ := json.Marshal(item.AcceptedVideoCodecs)
		acceptedVideoCodecs = string(data)
	}
	var acceptedAudioCodecs string
	if len(item.AcceptedAudioCodecs) > 0 {
		data, _ := json.Marshal(item.AcceptedAudioCodecs)
		acceptedAudioCodecs = string(data)
	}

	existing.Description = item.Description
	existing.Expression = item.Expression
	existing.Priority = item.Priority
	existing.IsEnabled = &enabled
	existing.AcceptedVideoCodecs = acceptedVideoCodecs
	existing.AcceptedAudioCodecs = acceptedAudioCodecs
	existing.PreferredVideoCodec = models.VideoCodec(item.PreferredVideoCodec)
	existing.PreferredAudioCodec = models.AudioCodec(item.PreferredAudioCodec)
	existing.SupportsFMP4 = &supportsFMP4
	existing.SupportsMPEGTS = &supportsMPEGTS
	existing.PreferredFormat = item.PreferredFormat
	existing.EncodingProfileID = encodingProfileID
}

// ImportEncodingProfilesPreview returns a preview of what will happen on import.
func (s *ImportService) ImportEncodingProfilesPreview(ctx context.Context, items []models.EncodingProfileExportItem) (*models.ImportPreview, error) {
	preview := &models.ImportPreview{
		TotalItems: len(items),
		NewItems:   make([]models.PreviewItem, 0),
		Conflicts:  make([]models.ConflictItem, 0),
		Errors:     make([]models.ImportError, 0),
	}

	for _, item := range items {
		if item.Name == "" {
			preview.Errors = append(preview.Errors, models.ImportError{
				ItemName: item.Name,
				Error:    "name is required",
			})
			continue
		}

		existing, err := s.encodingProfileRepo.GetByName(ctx, item.Name)
		if err != nil {
			preview.Errors = append(preview.Errors, models.ImportError{
				ItemName: item.Name,
				Error:    err.Error(),
			})
			continue
		}

		if existing != nil {
			preview.Conflicts = append(preview.Conflicts, models.ConflictItem{
				ImportName:   item.Name,
				ExistingID:   existing.ID.String(),
				ExistingName: existing.Name,
				Resolution:   models.ConflictResolutionSkip,
			})
		} else {
			preview.NewItems = append(preview.NewItems, models.PreviewItem{
				Name: item.Name,
			})
		}
	}

	return preview, nil
}

// ImportEncodingProfiles imports encoding profiles with specified conflict resolutions.
// For bulk resolution support, use ImportEncodingProfilesWithOptions instead.
func (s *ImportService) ImportEncodingProfiles(
	ctx context.Context,
	items []models.EncodingProfileExportItem,
	resolutions map[string]models.ConflictResolution,
) (*models.ImportResult, error) {
	return s.ImportEncodingProfilesWithOptions(ctx, items, &ImportOptions{Resolutions: resolutions})
}

// ImportEncodingProfilesWithOptions imports encoding profiles with full options including bulk resolution.
func (s *ImportService) ImportEncodingProfilesWithOptions(
	ctx context.Context,
	items []models.EncodingProfileExportItem,
	opts *ImportOptions,
) (*models.ImportResult, error) {
	result := &models.ImportResult{
		TotalItems:    len(items),
		ImportedItems: make([]models.ImportedItem, 0),
		ErrorDetails:  make([]models.ImportError, 0),
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			imported, err := s.importSingleEncodingProfileWithOpts(ctx, tx, item, opts)
			if err != nil {
				result.Errors++
				result.ErrorDetails = append(result.ErrorDetails, models.ImportError{
					ItemName: item.Name,
					Error:    err.Error(),
				})
				continue
			}

			if imported != nil {
				result.ImportedItems = append(result.ImportedItems, *imported)
				switch imported.Action {
				case "created":
					result.Imported++
				case "overwritten":
					result.Overwritten++
				case "renamed":
					result.Renamed++
				}
			} else {
				result.Skipped++
			}
		}

		if result.Errors > 0 {
			return fmt.Errorf("import failed with %d errors", result.Errors)
		}

		return nil
	})

	if err != nil {
		return result, err
	}

	s.logger.Info("imported encoding profiles",
		slog.Int("total", result.TotalItems),
		slog.Int("imported", result.Imported),
	)

	return result, nil
}

func (s *ImportService) importSingleEncodingProfileWithOpts(
	ctx context.Context,
	tx *gorm.DB,
	item models.EncodingProfileExportItem,
	opts *ImportOptions,
) (*models.ImportedItem, error) {
	var existing models.EncodingProfile
	err := tx.Where("name = ?", item.Name).First(&existing).Error

	if err == nil {
		resolution := opts.getResolution(item.Name)

		switch resolution {
		case models.ConflictResolutionSkip:
			return nil, nil

		case models.ConflictResolutionRename:
			newName := s.findUniqueEncodingProfileName(tx, item.Name)
			profile := encodingProfileFromExportItem(item)
			profile.Name = newName
			profile.IsDefault = false // Never import as default
			if err := tx.Create(&profile).Error; err != nil {
				return nil, err
			}
			return &models.ImportedItem{
				OriginalName: item.Name,
				FinalName:    newName,
				ID:           profile.ID.String(),
				Action:       "renamed",
			}, nil

		case models.ConflictResolutionOverwrite:
			updateEncodingProfile(&existing, item)
			if err := tx.Save(&existing).Error; err != nil {
				return nil, err
			}
			return &models.ImportedItem{
				OriginalName: item.Name,
				FinalName:    item.Name,
				ID:           existing.ID.String(),
				Action:       "overwritten",
			}, nil
		}
	} else if err == gorm.ErrRecordNotFound {
		profile := encodingProfileFromExportItem(item)
		profile.ID = models.NewULID()
		profile.IsDefault = false // Never import as default
		if err := tx.Create(&profile).Error; err != nil {
			return nil, err
		}
		return &models.ImportedItem{
			OriginalName: item.Name,
			FinalName:    item.Name,
			ID:           profile.ID.String(),
			Action:       "created",
		}, nil
	}

	return nil, err
}

func (s *ImportService) findUniqueEncodingProfileName(tx *gorm.DB, baseName string) string {
	suffix := 1
	for {
		newName := fmt.Sprintf("%s (%d)", baseName, suffix)
		var count int64
		tx.Model(&models.EncodingProfile{}).Where("name = ?", newName).Count(&count)
		if count == 0 {
			return newName
		}
		suffix++
	}
}

func encodingProfileFromExportItem(item models.EncodingProfileExportItem) models.EncodingProfile {
	enabled := item.Enabled
	return models.EncodingProfile{
		Name:             item.Name,
		Description:      item.Description,
		TargetVideoCodec: models.VideoCodec(item.TargetVideoCodec),
		TargetAudioCodec: models.AudioCodec(item.TargetAudioCodec),
		QualityPreset:    models.QualityPreset(item.QualityPreset),
		HWAccel:          models.HWAccelType(item.HWAccel),
		GlobalFlags:      item.GlobalFlags,
		InputFlags:       item.InputFlags,
		OutputFlags:      item.OutputFlags,
		IsDefault:        false, // Never import as default
		IsSystem:         false,
		Enabled:          &enabled,
	}
}

func updateEncodingProfile(existing *models.EncodingProfile, item models.EncodingProfileExportItem) {
	enabled := item.Enabled
	existing.Description = item.Description
	existing.TargetVideoCodec = models.VideoCodec(item.TargetVideoCodec)
	existing.TargetAudioCodec = models.AudioCodec(item.TargetAudioCodec)
	existing.QualityPreset = models.QualityPreset(item.QualityPreset)
	existing.HWAccel = models.HWAccelType(item.HWAccel)
	existing.GlobalFlags = item.GlobalFlags
	existing.InputFlags = item.InputFlags
	existing.OutputFlags = item.OutputFlags
	// Don't update IsDefault - preserve existing setting
	existing.Enabled = &enabled
}
