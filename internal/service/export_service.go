// Package service provides business logic layer for tvarr operations.
package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/jmylchreest/tvarr/internal/version"
)

// ExportService provides business logic for exporting configuration items.
type ExportService struct {
	filterRepo               repository.FilterRepository
	dataMappingRuleRepo      repository.DataMappingRuleRepository
	clientDetectionRuleRepo  repository.ClientDetectionRuleRepository
	encodingProfileRepo      repository.EncodingProfileRepository
	logger                   *slog.Logger
}

// NewExportService creates a new export service.
func NewExportService(
	filterRepo repository.FilterRepository,
	dataMappingRuleRepo repository.DataMappingRuleRepository,
	clientDetectionRuleRepo repository.ClientDetectionRuleRepository,
	encodingProfileRepo repository.EncodingProfileRepository,
) *ExportService {
	return &ExportService{
		filterRepo:               filterRepo,
		dataMappingRuleRepo:      dataMappingRuleRepo,
		clientDetectionRuleRepo:  clientDetectionRuleRepo,
		encodingProfileRepo:      encodingProfileRepo,
		logger:                   slog.Default(),
	}
}

// WithLogger sets the logger for the service.
func (s *ExportService) WithLogger(logger *slog.Logger) *ExportService {
	s.logger = logger
	return s
}

// ExportFilters exports selected filters or all user-created filters.
// If ids is empty and exportAll is true, exports all user-created filters.
// System filters (IsSystem=true) are always excluded from export.
func (s *ExportService) ExportFilters(ctx context.Context, ids []models.ULID, exportAll bool) (*models.ConfigExport[models.FilterExportItem], error) {
	var filters []*models.Filter
	var err error

	if exportAll || len(ids) == 0 {
		// Get all user-created filters (IsSystem = false)
		filters, err = s.filterRepo.GetUserCreated(ctx)
	} else {
		// Get specific filters by ID
		filters, err = s.filterRepo.GetByIDs(ctx, ids)
	}
	if err != nil {
		return nil, err
	}

	// Filter out system filters if specific IDs were provided
	userFilters := make([]*models.Filter, 0, len(filters))
	for _, f := range filters {
		if !f.IsSystem {
			userFilters = append(userFilters, f)
		}
	}

	// Convert to export items
	items := make([]models.FilterExportItem, len(userFilters))
	for i, f := range userFilters {
		items[i] = models.FilterExportItem{
			Name:        f.Name,
			Description: f.Description,
			Expression:  f.Expression,
			SourceType:  string(f.SourceType),
			Action:      string(f.Action),
			IsEnabled:   models.BoolVal(f.IsEnabled),
			SourceID:    ulidPtrToStringPtr(f.SourceID),
		}
	}

	s.logger.Debug("exported filters", slog.Int("count", len(items)))

	return &models.ConfigExport[models.FilterExportItem]{
		Metadata: models.ExportMetadata{
			Version:      models.ExportFormatVersion,
			TvarrVersion: version.Version,
			ExportType:   models.ExportTypeFilters,
			ExportedAt:   time.Now().UTC(),
			ItemCount:    len(items),
		},
		Items: items,
	}, nil
}

// ExportDataMappingRules exports selected data mapping rules or all user-created rules.
func (s *ExportService) ExportDataMappingRules(ctx context.Context, ids []models.ULID, exportAll bool) (*models.ConfigExport[models.DataMappingRuleExportItem], error) {
	var rules []*models.DataMappingRule
	var err error

	if exportAll || len(ids) == 0 {
		rules, err = s.dataMappingRuleRepo.GetUserCreated(ctx)
	} else {
		rules, err = s.dataMappingRuleRepo.GetByIDs(ctx, ids)
	}
	if err != nil {
		return nil, err
	}

	// Filter out system rules if specific IDs were provided
	userRules := make([]*models.DataMappingRule, 0, len(rules))
	for _, r := range rules {
		if !r.IsSystem {
			userRules = append(userRules, r)
		}
	}

	// Convert to export items
	items := make([]models.DataMappingRuleExportItem, len(userRules))
	for i, r := range userRules {
		items[i] = models.DataMappingRuleExportItem{
			Name:        r.Name,
			Description: r.Description,
			Expression:  r.Expression,
			SourceType:  string(r.SourceType),
			Priority:    r.Priority,
			StopOnMatch: r.StopOnMatch,
			IsEnabled:   models.BoolVal(r.IsEnabled),
			SourceID:    ulidPtrToStringPtr(r.SourceID),
		}
	}

	s.logger.Debug("exported data mapping rules", slog.Int("count", len(items)))

	return &models.ConfigExport[models.DataMappingRuleExportItem]{
		Metadata: models.ExportMetadata{
			Version:      models.ExportFormatVersion,
			TvarrVersion: version.Version,
			ExportType:   models.ExportTypeDataMappingRules,
			ExportedAt:   time.Now().UTC(),
			ItemCount:    len(items),
		},
		Items: items,
	}, nil
}

// ExportClientDetectionRules exports selected client detection rules or all user-created rules.
func (s *ExportService) ExportClientDetectionRules(ctx context.Context, ids []models.ULID, exportAll bool) (*models.ConfigExport[models.ClientDetectionRuleExportItem], error) {
	var rules []*models.ClientDetectionRule
	var err error

	if exportAll || len(ids) == 0 {
		rules, err = s.clientDetectionRuleRepo.GetUserCreated(ctx)
	} else {
		rules, err = s.clientDetectionRuleRepo.GetByIDs(ctx, ids)
	}
	if err != nil {
		return nil, err
	}

	// Filter out system rules if specific IDs were provided
	userRules := make([]*models.ClientDetectionRule, 0, len(rules))
	for _, r := range rules {
		if !r.IsSystem {
			userRules = append(userRules, r)
		}
	}

	// Convert to export items
	items := make([]models.ClientDetectionRuleExportItem, len(userRules))
	for i, r := range userRules {
		// Resolve encoding profile name if present
		var encodingProfileName *string
		if r.EncodingProfile != nil {
			encodingProfileName = &r.EncodingProfile.Name
		}

		items[i] = models.ClientDetectionRuleExportItem{
			Name:                r.Name,
			Description:         r.Description,
			Expression:          r.Expression,
			Priority:            r.Priority,
			IsEnabled:           models.BoolVal(r.IsEnabled),
			AcceptedVideoCodecs: r.GetAcceptedVideoCodecs(), // Decode from JSON string
			AcceptedAudioCodecs: r.GetAcceptedAudioCodecs(), // Decode from JSON string
			PreferredVideoCodec: string(r.PreferredVideoCodec),
			PreferredAudioCodec: string(r.PreferredAudioCodec),
			SupportsFMP4:        models.BoolVal(r.SupportsFMP4),
			SupportsMPEGTS:      models.BoolVal(r.SupportsMPEGTS),
			PreferredFormat:     r.PreferredFormat,
			EncodingProfileName: encodingProfileName,
		}
	}

	s.logger.Debug("exported client detection rules", slog.Int("count", len(items)))

	return &models.ConfigExport[models.ClientDetectionRuleExportItem]{
		Metadata: models.ExportMetadata{
			Version:      models.ExportFormatVersion,
			TvarrVersion: version.Version,
			ExportType:   models.ExportTypeClientDetectionRules,
			ExportedAt:   time.Now().UTC(),
			ItemCount:    len(items),
		},
		Items: items,
	}, nil
}

// ExportEncodingProfiles exports selected encoding profiles or all user-created profiles.
func (s *ExportService) ExportEncodingProfiles(ctx context.Context, ids []models.ULID, exportAll bool) (*models.ConfigExport[models.EncodingProfileExportItem], error) {
	var profiles []*models.EncodingProfile
	var err error

	if exportAll || len(ids) == 0 {
		profiles, err = s.encodingProfileRepo.GetUserCreated(ctx)
	} else {
		profiles, err = s.encodingProfileRepo.GetByIDs(ctx, ids)
	}
	if err != nil {
		return nil, err
	}

	// Filter out system profiles if specific IDs were provided
	userProfiles := make([]*models.EncodingProfile, 0, len(profiles))
	for _, p := range profiles {
		if !p.IsSystem {
			userProfiles = append(userProfiles, p)
		}
	}

	// Convert to export items
	items := make([]models.EncodingProfileExportItem, len(userProfiles))
	for i, p := range userProfiles {
		items[i] = models.EncodingProfileExportItem{
			Name:             p.Name,
			Description:      p.Description,
			TargetVideoCodec: string(p.TargetVideoCodec),
			TargetAudioCodec: string(p.TargetAudioCodec),
			QualityPreset:    string(p.QualityPreset),
			HWAccel:          string(p.HWAccel),
			GlobalFlags:      p.GlobalFlags,
			InputFlags:       p.InputFlags,
			OutputFlags:      p.OutputFlags,
			IsDefault:        p.IsDefault,
			Enabled:          models.BoolVal(p.Enabled),
		}
	}

	s.logger.Debug("exported encoding profiles", slog.Int("count", len(items)))

	return &models.ConfigExport[models.EncodingProfileExportItem]{
		Metadata: models.ExportMetadata{
			Version:      models.ExportFormatVersion,
			TvarrVersion: version.Version,
			ExportType:   models.ExportTypeEncodingProfiles,
			ExportedAt:   time.Now().UTC(),
			ItemCount:    len(items),
		},
		Items: items,
	}, nil
}

// Helper function to convert ULID pointer to string pointer
func ulidPtrToStringPtr(id *models.ULID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}
