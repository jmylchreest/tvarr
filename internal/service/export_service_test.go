package service

import (
	"context"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/jmylchreest/tvarr/internal/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupExportTestDB(t *testing.T) (
	*gorm.DB,
	repository.FilterRepository,
	repository.DataMappingRuleRepository,
	repository.ClientDetectionRuleRepository,
	repository.EncodingProfileRepository,
) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	err = db.AutoMigrate(
		&models.Filter{},
		&models.DataMappingRule{},
		&models.ClientDetectionRule{},
		&models.EncodingProfile{},
	)
	require.NoError(t, err)

	filterRepo := repository.NewFilterRepository(db)
	dmrRepo := repository.NewDataMappingRuleRepository(db)
	cdrRepo := repository.NewClientDetectionRuleRepository(db)
	epRepo := repository.NewEncodingProfileRepository(db)

	return db, filterRepo, dmrRepo, cdrRepo, epRepo
}

// createTestFilter creates a test filter with the given parameters
func createTestFilter(name, expression string, sourceType models.FilterSourceType, action models.FilterAction, isSystem bool) *models.Filter {
	return &models.Filter{
		BaseModel:  models.BaseModel{ID: models.NewULID()},
		Name:       name,
		Expression: expression,
		SourceType: sourceType,
		Action:     action,
		IsSystem:   isSystem,
	}
}

// createTestDataMappingRule creates a test data mapping rule with the given parameters
func createTestDataMappingRule(name, expression string, sourceType models.DataMappingRuleSourceType, priority int, isSystem bool) *models.DataMappingRule {
	enabled := true
	return &models.DataMappingRule{
		BaseModel:  models.BaseModel{ID: models.NewULID()},
		Name:       name,
		Expression: expression,
		SourceType: sourceType,
		Priority:   priority,
		IsEnabled:  &enabled,
		IsSystem:   isSystem,
	}
}

// createTestClientDetectionRule creates a test client detection rule with the given parameters
func createTestClientDetectionRule(name, expression string, priority int, isSystem bool) *models.ClientDetectionRule {
	enabled := true
	return &models.ClientDetectionRule{
		BaseModel:  models.BaseModel{ID: models.NewULID()},
		Name:       name,
		Expression: expression,
		Priority:   priority,
		IsEnabled:  &enabled,
		IsSystem:   isSystem,
	}
}

// createTestEncodingProfile creates a test encoding profile with the given parameters
func createTestEncodingProfile(name string, videoCodec models.VideoCodec, audioCodec models.AudioCodec, isSystem bool) *models.EncodingProfile {
	enabled := true
	return &models.EncodingProfile{
		BaseModel:        models.BaseModel{ID: models.NewULID()},
		Name:             name,
		TargetVideoCodec: videoCodec,
		TargetAudioCodec: audioCodec,
		QualityPreset:    models.QualityPresetMedium,
		HWAccel:          models.HWAccelNone, // Required for validation
		Enabled:          &enabled,
		IsSystem:         isSystem,
	}
}

func TestExportService_ExportFilters(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		setup       func(*gorm.DB) []models.ULID
		ids         []models.ULID
		exportAll   bool
		wantCount   int
		wantError   bool
		verifyItems func(*testing.T, []models.FilterExportItem)
	}{
		{
			name: "export all user-created filters",
			setup: func(db *gorm.DB) []models.ULID {
				filters := []*models.Filter{
					createTestFilter("User Filter 1", "name contains 'test'", models.FilterSourceTypeStream, models.FilterActionInclude, false),
					createTestFilter("User Filter 2", "name matches 'news.*'", models.FilterSourceTypeEPG, models.FilterActionExclude, false),
					createTestFilter("System Filter", "internal", models.FilterSourceTypeStream, models.FilterActionInclude, true), // Should be excluded
				}
				for _, f := range filters {
					require.NoError(t, db.Create(f).Error)
				}
				return nil
			},
			exportAll: true,
			wantCount: 2, // Only user-created filters
			verifyItems: func(t *testing.T, items []models.FilterExportItem) {
				names := make([]string, len(items))
				for i, item := range items {
					names[i] = item.Name
				}
				assert.Contains(t, names, "User Filter 1")
				assert.Contains(t, names, "User Filter 2")
				assert.NotContains(t, names, "System Filter")
			},
		},
		{
			name: "export specific filters by ID",
			setup: func(db *gorm.DB) []models.ULID {
				f1 := createTestFilter("Selected Filter", "group = 'sports'", models.FilterSourceTypeStream, models.FilterActionInclude, false)
				f2 := createTestFilter("Not Selected", "other", models.FilterSourceTypeStream, models.FilterActionInclude, false)
				require.NoError(t, db.Create(f1).Error)
				require.NoError(t, db.Create(f2).Error)
				return []models.ULID{f1.ID}
			},
			exportAll: false,
			wantCount: 1,
			verifyItems: func(t *testing.T, items []models.FilterExportItem) {
				require.Len(t, items, 1)
				assert.Equal(t, "Selected Filter", items[0].Name)
			},
		},
		{
			name: "excludes system filters even when requested by ID",
			setup: func(db *gorm.DB) []models.ULID {
				sysFilter := createTestFilter("System Filter", "system", models.FilterSourceTypeStream, models.FilterActionInclude, true)
				require.NoError(t, db.Create(sysFilter).Error)
				return []models.ULID{sysFilter.ID}
			},
			exportAll: false,
			wantCount: 0,
		},
		{
			name: "generates correct metadata",
			setup: func(db *gorm.DB) []models.ULID {
				f := createTestFilter("Test Filter", "test", models.FilterSourceTypeStream, models.FilterActionInclude, false)
				require.NoError(t, db.Create(f).Error)
				return nil
			},
			exportAll: true,
			wantCount: 1,
		},
		{
			name:      "empty export when no filters exist",
			setup:     func(db *gorm.DB) []models.ULID { return nil },
			exportAll: true,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, filterRepo, dmrRepo, cdrRepo, epRepo := setupExportTestDB(t)
			svc := NewExportService(filterRepo, dmrRepo, cdrRepo, epRepo)

			ids := tt.setup(db)
			if tt.ids != nil {
				ids = tt.ids
			}

			result, err := svc.ExportFilters(ctx, ids, tt.exportAll)

			if tt.wantError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.Len(t, result.Items, tt.wantCount)

			// Verify metadata
			assert.Equal(t, models.ExportFormatVersion, result.Metadata.Version)
			assert.Equal(t, version.Version, result.Metadata.TvarrVersion)
			assert.Equal(t, models.ExportTypeFilters, result.Metadata.ExportType)
			assert.Equal(t, tt.wantCount, result.Metadata.ItemCount)
			assert.False(t, result.Metadata.ExportedAt.IsZero())

			if tt.verifyItems != nil {
				tt.verifyItems(t, result.Items)
			}
		})
	}
}

func TestExportService_ExportFilters_ItemFields(t *testing.T) {
	ctx := context.Background()
	db, filterRepo, dmrRepo, cdrRepo, epRepo := setupExportTestDB(t)
	svc := NewExportService(filterRepo, dmrRepo, cdrRepo, epRepo)

	// Create filter with all fields
	sourceID := models.NewULID()
	filter := &models.Filter{
		BaseModel:   models.BaseModel{ID: models.NewULID()},
		Name:        "Full Filter",
		Description: "A complete filter with all fields",
		Expression:  "group = 'movies' AND name contains 'HD'",
		SourceType:  models.FilterSourceTypeStream,
		Action:      models.FilterActionExclude,
		IsSystem:    false,
		SourceID:    &sourceID,
	}
	require.NoError(t, db.Create(filter).Error)

	result, err := svc.ExportFilters(ctx, nil, true)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)

	item := result.Items[0]
	assert.Equal(t, "Full Filter", item.Name)
	assert.Equal(t, "A complete filter with all fields", item.Description)
	assert.Equal(t, "group = 'movies' AND name contains 'HD'", item.Expression)
	assert.Equal(t, "stream", item.SourceType)
	assert.Equal(t, "exclude", item.Action)
	require.NotNil(t, item.SourceID)
	assert.Equal(t, sourceID.String(), *item.SourceID)
}

func TestExportService_ExportDataMappingRules(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		setup       func(*gorm.DB) []models.ULID
		exportAll   bool
		wantCount   int
		verifyItems func(*testing.T, []models.DataMappingRuleExportItem)
	}{
		{
			name: "export all user-created rules",
			setup: func(db *gorm.DB) []models.ULID {
				enabled := true
				rule1 := &models.DataMappingRule{
					BaseModel:   models.BaseModel{ID: models.NewULID()},
					Name:        "User Rule 1",
					Expression:  "name = name.toUpperCase()",
					SourceType:  models.DataMappingRuleSourceTypeStream,
					Priority:    10,
					StopOnMatch: true,
					IsEnabled:   &enabled,
					IsSystem:    false,
				}
				rule2 := &models.DataMappingRule{
					BaseModel:  models.BaseModel{ID: models.NewULID()},
					Name:       "System Rule",
					Expression: "internal",
					SourceType: models.DataMappingRuleSourceTypeStream,
					Priority:   1,
					IsEnabled:  &enabled,
					IsSystem:   true,
				}
				require.NoError(t, db.Create(rule1).Error)
				require.NoError(t, db.Create(rule2).Error)
				return nil
			},
			exportAll: true,
			wantCount: 1,
			verifyItems: func(t *testing.T, items []models.DataMappingRuleExportItem) {
				require.Len(t, items, 1)
				assert.Equal(t, "User Rule 1", items[0].Name)
				assert.Equal(t, 10, items[0].Priority)
				assert.True(t, items[0].StopOnMatch)
			},
		},
		{
			name:      "empty export when no rules exist",
			setup:     func(db *gorm.DB) []models.ULID { return nil },
			exportAll: true,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, filterRepo, dmrRepo, cdrRepo, epRepo := setupExportTestDB(t)
			svc := NewExportService(filterRepo, dmrRepo, cdrRepo, epRepo)

			ids := tt.setup(db)

			result, err := svc.ExportDataMappingRules(ctx, ids, tt.exportAll)

			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.Len(t, result.Items, tt.wantCount)
			assert.Equal(t, models.ExportTypeDataMappingRules, result.Metadata.ExportType)

			if tt.verifyItems != nil {
				tt.verifyItems(t, result.Items)
			}
		})
	}
}

func TestExportService_ExportClientDetectionRules(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		setup       func(*gorm.DB) []models.ULID
		exportAll   bool
		wantCount   int
		verifyItems func(*testing.T, []models.ClientDetectionRuleExportItem)
	}{
		{
			name: "export with encoding profile name resolution",
			setup: func(db *gorm.DB) []models.ULID {
				enabled := true
				supportsFMP4 := true
				supportsMPEGTS := true

				// Create encoding profile
				profile := createTestEncodingProfile("Test Profile", models.VideoCodecH264, models.AudioCodecAAC, false)
				require.NoError(t, db.Create(profile).Error)

				// Create client detection rule with encoding profile
				rule := &models.ClientDetectionRule{
					BaseModel:           models.BaseModel{ID: models.NewULID()},
					Name:                "Client Rule with Profile",
					Expression:          "user_agent contains 'VLC'",
					Priority:            5,
					IsEnabled:           &enabled,
					IsSystem:            false,
					AcceptedVideoCodecs: `["h264","h265"]`,
					AcceptedAudioCodecs: `["aac","opus"]`,
					PreferredVideoCodec: models.VideoCodecH264,
					PreferredAudioCodec: models.AudioCodecAAC,
					SupportsFMP4:        &supportsFMP4,
					SupportsMPEGTS:      &supportsMPEGTS,
					PreferredFormat:     "fmp4",
					EncodingProfileID:   &profile.ID,
				}
				require.NoError(t, db.Create(rule).Error)

				// Reload to get encoding profile preloaded
				db.Preload("EncodingProfile").First(rule)

				return nil
			},
			exportAll: true,
			wantCount: 1,
			verifyItems: func(t *testing.T, items []models.ClientDetectionRuleExportItem) {
				require.Len(t, items, 1)
				item := items[0]
				assert.Equal(t, "Client Rule with Profile", item.Name)
				assert.Equal(t, 5, item.Priority)
				assert.Contains(t, item.AcceptedVideoCodecs, "h264")
				assert.Contains(t, item.AcceptedVideoCodecs, "h265")
				assert.Contains(t, item.AcceptedAudioCodecs, "aac")
				assert.Contains(t, item.AcceptedAudioCodecs, "opus")
				assert.True(t, item.SupportsFMP4)
				assert.True(t, item.SupportsMPEGTS)
				assert.Equal(t, "fmp4", item.PreferredFormat)
				require.NotNil(t, item.EncodingProfileName)
				assert.Equal(t, "Test Profile", *item.EncodingProfileName)
			},
		},
		{
			name: "excludes system rules",
			setup: func(db *gorm.DB) []models.ULID {
				enabled := true
				rules := []*models.ClientDetectionRule{
					{
						BaseModel:  models.BaseModel{ID: models.NewULID()},
						Name:       "User Client Rule",
						Expression: "custom",
						IsEnabled:  &enabled,
						IsSystem:   false,
					},
					{
						BaseModel:  models.BaseModel{ID: models.NewULID()},
						Name:       "System Client Rule",
						Expression: "system",
						IsEnabled:  &enabled,
						IsSystem:   true,
					},
				}
				for _, r := range rules {
					require.NoError(t, db.Create(r).Error)
				}
				return nil
			},
			exportAll: true,
			wantCount: 1,
			verifyItems: func(t *testing.T, items []models.ClientDetectionRuleExportItem) {
				require.Len(t, items, 1)
				assert.Equal(t, "User Client Rule", items[0].Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, filterRepo, dmrRepo, cdrRepo, epRepo := setupExportTestDB(t)
			svc := NewExportService(filterRepo, dmrRepo, cdrRepo, epRepo)

			ids := tt.setup(db)

			result, err := svc.ExportClientDetectionRules(ctx, ids, tt.exportAll)

			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.Len(t, result.Items, tt.wantCount)
			assert.Equal(t, models.ExportTypeClientDetectionRules, result.Metadata.ExportType)

			if tt.verifyItems != nil {
				tt.verifyItems(t, result.Items)
			}
		})
	}
}

func TestExportService_ExportEncodingProfiles(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		setup       func(*gorm.DB) []models.ULID
		exportAll   bool
		wantCount   int
		verifyItems func(*testing.T, []models.EncodingProfileExportItem)
	}{
		{
			name: "export all user-created profiles",
			setup: func(db *gorm.DB) []models.ULID {
				enabled := true
				profiles := []*models.EncodingProfile{
					{
						BaseModel:        models.BaseModel{ID: models.NewULID()},
						Name:             "User Profile 1",
						Description:      "Custom transcoding profile",
						TargetVideoCodec: models.VideoCodecH265,
						TargetAudioCodec: models.AudioCodecOpus,
						QualityPreset:    models.QualityPresetHigh,
						HWAccel:          models.HWAccelVAAPI,
						GlobalFlags:      "-hide_banner",
						InputFlags:       "-re",
						OutputFlags:      "-movflags +faststart",
						IsDefault:        true,
						Enabled:          &enabled,
						IsSystem:         false,
					},
					{
						BaseModel:        models.BaseModel{ID: models.NewULID()},
						Name:             "System Passthrough",
						TargetVideoCodec: models.VideoCodecH264,
						TargetAudioCodec: models.AudioCodecAAC,
						QualityPreset:    models.QualityPresetMedium,
						Enabled:          &enabled,
						IsSystem:         true,
					},
				}
				for _, p := range profiles {
					require.NoError(t, db.Create(p).Error)
				}
				return nil
			},
			exportAll: true,
			wantCount: 1,
			verifyItems: func(t *testing.T, items []models.EncodingProfileExportItem) {
				require.Len(t, items, 1)
				item := items[0]
				assert.Equal(t, "User Profile 1", item.Name)
				assert.Equal(t, "Custom transcoding profile", item.Description)
				assert.Equal(t, "h265", item.TargetVideoCodec)
				assert.Equal(t, "opus", item.TargetAudioCodec)
				assert.Equal(t, "high", item.QualityPreset)
				assert.Equal(t, "vaapi", item.HWAccel)
				assert.Equal(t, "-hide_banner", item.GlobalFlags)
				assert.Equal(t, "-re", item.InputFlags)
				assert.Equal(t, "-movflags +faststart", item.OutputFlags)
				assert.True(t, item.IsDefault)
				assert.True(t, item.Enabled)
			},
		},
		{
			name: "export specific profiles by ID",
			setup: func(db *gorm.DB) []models.ULID {
				p1 := createTestEncodingProfile("Selected Profile", models.VideoCodecH264, models.AudioCodecAAC, false)
				p2 := createTestEncodingProfile("Not Selected", models.VideoCodecH264, models.AudioCodecAAC, false)
				require.NoError(t, db.Create(p1).Error)
				require.NoError(t, db.Create(p2).Error)
				return []models.ULID{p1.ID}
			},
			exportAll: false,
			wantCount: 1,
			verifyItems: func(t *testing.T, items []models.EncodingProfileExportItem) {
				require.Len(t, items, 1)
				assert.Equal(t, "Selected Profile", items[0].Name)
			},
		},
		{
			name:      "empty export when no profiles exist",
			setup:     func(db *gorm.DB) []models.ULID { return nil },
			exportAll: true,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, filterRepo, dmrRepo, cdrRepo, epRepo := setupExportTestDB(t)
			svc := NewExportService(filterRepo, dmrRepo, cdrRepo, epRepo)

			ids := tt.setup(db)

			result, err := svc.ExportEncodingProfiles(ctx, ids, tt.exportAll)

			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.Len(t, result.Items, tt.wantCount)
			assert.Equal(t, models.ExportTypeEncodingProfiles, result.Metadata.ExportType)

			if tt.verifyItems != nil {
				tt.verifyItems(t, result.Items)
			}
		})
	}
}
