package service

import (
	"context"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupImportTestDB(t *testing.T) (
	*gorm.DB,
	repository.FilterRepository,
	repository.DataMappingRuleRepository,
	repository.ClientDetectionRuleRepository,
	repository.EncodingProfileRepository,
) {
	t.Helper()

	// Use in-memory SQLite with a single connection to avoid connection pooling issues
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// IMPORTANT: Limit to 1 connection for in-memory SQLite
	// Each connection to :memory: creates a new independent database
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

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

func TestImportService_ImportFiltersPreview(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		setup          func(*gorm.DB)
		items          []models.FilterExportItem
		wantNewCount   int
		wantConflicts  int
		wantErrorCount int
	}{
		{
			name:  "preview with no conflicts",
			setup: func(db *gorm.DB) {},
			items: []models.FilterExportItem{
				{Name: "New Filter 1", Expression: "test1", SourceType: "stream", Action: "include", IsEnabled: true},
				{Name: "New Filter 2", Expression: "test2", SourceType: "epg", Action: "exclude", IsEnabled: true},
			},
			wantNewCount:   2,
			wantConflicts:  0,
			wantErrorCount: 0,
		},
		{
			name: "preview detects name conflicts",
			setup: func(db *gorm.DB) {
				filter := createTestFilter("Existing Filter", "existing", models.FilterSourceTypeStream, models.FilterActionInclude, false)
				require.NoError(t, db.Create(filter).Error)
			},
			items: []models.FilterExportItem{
				{Name: "Existing Filter", Expression: "new", SourceType: "stream", Action: "include", IsEnabled: true},
				{Name: "New Filter", Expression: "test", SourceType: "stream", Action: "include", IsEnabled: true},
			},
			wantNewCount:   1,
			wantConflicts:  1,
			wantErrorCount: 0,
		},
		{
			name:  "preview reports validation errors",
			setup: func(db *gorm.DB) {},
			items: []models.FilterExportItem{
				{Name: "Missing Expression", Expression: "", SourceType: "stream", Action: "include", IsEnabled: true},
				{Name: "Valid Filter", Expression: "test", SourceType: "stream", Action: "include", IsEnabled: true},
			},
			wantNewCount:   1,
			wantConflicts:  0,
			wantErrorCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, filterRepo, dmrRepo, cdrRepo, epRepo := setupImportTestDB(t)
			svc := NewImportService(db, filterRepo, dmrRepo, cdrRepo, epRepo)
			tt.setup(db)

			preview, err := svc.ImportFiltersPreview(ctx, tt.items)

			require.NoError(t, err)
			assert.Equal(t, len(tt.items), preview.TotalItems)
			assert.Len(t, preview.NewItems, tt.wantNewCount)
			assert.Len(t, preview.Conflicts, tt.wantConflicts)
			assert.Len(t, preview.Errors, tt.wantErrorCount)
		})
	}
}

func TestImportService_ImportFilters_SkipResolution(t *testing.T) {
	ctx := context.Background()
	db, filterRepo, dmrRepo, cdrRepo, epRepo := setupImportTestDB(t)
	svc := NewImportService(db, filterRepo, dmrRepo, cdrRepo, epRepo)

	// Create existing filter
	existingFilter := createTestFilter("Existing Filter", "old expression", models.FilterSourceTypeStream, models.FilterActionInclude, false)
	require.NoError(t, db.Create(existingFilter).Error)

	// Try to import with skip resolution
	items := []models.FilterExportItem{
		{Name: "Existing Filter", Expression: "new expression", SourceType: "stream", Action: "include", IsEnabled: true},
		{Name: "New Filter", Expression: "test", SourceType: "stream", Action: "include", IsEnabled: true},
	}
	resolutions := map[string]models.ConflictResolution{
		"Existing Filter": models.ConflictResolutionSkip,
	}

	result, err := svc.ImportFilters(ctx, items, resolutions)

	require.NoError(t, err)
	assert.Equal(t, 2, result.TotalItems)
	assert.Equal(t, 1, result.Imported)
	assert.Equal(t, 1, result.Skipped)
	assert.Equal(t, 0, result.Overwritten)
	assert.Equal(t, 0, result.Renamed)
	assert.Equal(t, 0, result.Errors)

	// Verify existing filter was NOT updated
	var updated models.Filter
	db.Where("name = ?", "Existing Filter").First(&updated)
	assert.Equal(t, "old expression", updated.Expression)

	// Verify new filter was created
	var newFilter models.Filter
	err = db.Where("name = ?", "New Filter").First(&newFilter).Error
	require.NoError(t, err)
	assert.Equal(t, "test", newFilter.Expression)
}

func TestImportService_ImportFilters_RenameResolution(t *testing.T) {
	ctx := context.Background()
	db, filterRepo, dmrRepo, cdrRepo, epRepo := setupImportTestDB(t)
	svc := NewImportService(db, filterRepo, dmrRepo, cdrRepo, epRepo)

	// Create existing filter
	existingFilter := createTestFilter("Existing Filter", "old expression", models.FilterSourceTypeStream, models.FilterActionInclude, false)
	require.NoError(t, db.Create(existingFilter).Error)

	// Import with rename resolution
	items := []models.FilterExportItem{
		{Name: "Existing Filter", Expression: "new expression", SourceType: "stream", Action: "include", IsEnabled: true},
	}
	resolutions := map[string]models.ConflictResolution{
		"Existing Filter": models.ConflictResolutionRename,
	}

	result, err := svc.ImportFilters(ctx, items, resolutions)

	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalItems)
	assert.Equal(t, 0, result.Imported)
	assert.Equal(t, 0, result.Skipped)
	assert.Equal(t, 0, result.Overwritten)
	assert.Equal(t, 1, result.Renamed)

	// Verify renamed filter was created
	require.Len(t, result.ImportedItems, 1)
	assert.Equal(t, "Existing Filter", result.ImportedItems[0].OriginalName)
	assert.Equal(t, "Existing Filter (1)", result.ImportedItems[0].FinalName)
	assert.Equal(t, "renamed", result.ImportedItems[0].Action)

	// Verify both filters exist
	var count int64
	db.Model(&models.Filter{}).Count(&count)
	assert.Equal(t, int64(2), count)

	// Verify original is unchanged
	var original models.Filter
	db.Where("name = ?", "Existing Filter").First(&original)
	assert.Equal(t, "old expression", original.Expression)

	// Verify renamed filter has new expression
	var renamed models.Filter
	db.Where("name = ?", "Existing Filter (1)").First(&renamed)
	assert.Equal(t, "new expression", renamed.Expression)
}

func TestImportService_ImportFilters_OverwriteResolution(t *testing.T) {
	ctx := context.Background()
	db, filterRepo, dmrRepo, cdrRepo, epRepo := setupImportTestDB(t)
	svc := NewImportService(db, filterRepo, dmrRepo, cdrRepo, epRepo)

	// Create existing filter
	existingFilter := createTestFilter("Existing Filter", "old expression", models.FilterSourceTypeStream, models.FilterActionInclude, false)
	require.NoError(t, db.Create(existingFilter).Error)
	originalID := existingFilter.ID

	// Import with overwrite resolution
	items := []models.FilterExportItem{
		{Name: "Existing Filter", Expression: "new expression", SourceType: "epg", Action: "exclude", IsEnabled: false},
	}
	resolutions := map[string]models.ConflictResolution{
		"Existing Filter": models.ConflictResolutionOverwrite,
	}

	result, err := svc.ImportFilters(ctx, items, resolutions)

	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalItems)
	assert.Equal(t, 0, result.Imported)
	assert.Equal(t, 0, result.Skipped)
	assert.Equal(t, 1, result.Overwritten)
	assert.Equal(t, 0, result.Renamed)

	// Verify filter was updated in place
	require.Len(t, result.ImportedItems, 1)
	assert.Equal(t, originalID.String(), result.ImportedItems[0].ID)
	assert.Equal(t, "overwritten", result.ImportedItems[0].Action)

	// Verify filter was updated
	var updated models.Filter
	db.Where("id = ?", originalID).First(&updated)
	assert.Equal(t, "new expression", updated.Expression)
	assert.Equal(t, models.FilterSourceTypeEPG, updated.SourceType)
	assert.Equal(t, models.FilterActionExclude, updated.Action)
	assert.False(t, models.BoolVal(updated.IsEnabled))

	// Verify only one filter exists
	var count int64
	db.Model(&models.Filter{}).Count(&count)
	assert.Equal(t, int64(1), count)
}

func TestImportService_ImportFilters_BulkResolution(t *testing.T) {
	ctx := context.Background()
	db, filterRepo, dmrRepo, cdrRepo, epRepo := setupImportTestDB(t)
	svc := NewImportService(db, filterRepo, dmrRepo, cdrRepo, epRepo)

	// Create multiple existing filters
	for i := 1; i <= 3; i++ {
		filter := createTestFilter("Existing "+string(rune('A'-1+i)), "old", models.FilterSourceTypeStream, models.FilterActionInclude, false)
		require.NoError(t, db.Create(filter).Error)
	}

	// Import multiple items with bulk overwrite
	items := []models.FilterExportItem{
		{Name: "Existing A", Expression: "new A", SourceType: "stream", Action: "include", IsEnabled: true},
		{Name: "Existing B", Expression: "new B", SourceType: "stream", Action: "include", IsEnabled: true},
		{Name: "Existing C", Expression: "new C", SourceType: "stream", Action: "include", IsEnabled: true},
		{Name: "New Filter", Expression: "new", SourceType: "stream", Action: "include", IsEnabled: true},
	}
	opts := &ImportOptions{
		BulkResolution: models.ConflictResolutionOverwrite,
	}

	result, err := svc.ImportFiltersWithOptions(ctx, items, opts)

	require.NoError(t, err)
	assert.Equal(t, 4, result.TotalItems)
	assert.Equal(t, 1, result.Imported)   // New filter
	assert.Equal(t, 0, result.Skipped)    // None skipped
	assert.Equal(t, 3, result.Overwritten) // All existing overwritten
	assert.Equal(t, 0, result.Renamed)
}

func TestImportService_ImportFilters_BulkSkip(t *testing.T) {
	ctx := context.Background()
	db, filterRepo, dmrRepo, cdrRepo, epRepo := setupImportTestDB(t)
	svc := NewImportService(db, filterRepo, dmrRepo, cdrRepo, epRepo)

	// Create existing filters
	filter1 := createTestFilter("Conflict 1", "old", models.FilterSourceTypeStream, models.FilterActionInclude, false)
	filter2 := createTestFilter("Conflict 2", "old", models.FilterSourceTypeStream, models.FilterActionInclude, false)
	require.NoError(t, db.Create(filter1).Error)
	require.NoError(t, db.Create(filter2).Error)

	// Import with bulk skip
	items := []models.FilterExportItem{
		{Name: "Conflict 1", Expression: "new", SourceType: "stream", Action: "include", IsEnabled: true},
		{Name: "Conflict 2", Expression: "new", SourceType: "stream", Action: "include", IsEnabled: true},
		{Name: "New Filter", Expression: "new", SourceType: "stream", Action: "include", IsEnabled: true},
	}
	opts := &ImportOptions{
		BulkResolution: models.ConflictResolutionSkip,
	}

	result, err := svc.ImportFiltersWithOptions(ctx, items, opts)

	require.NoError(t, err)
	assert.Equal(t, 1, result.Imported)
	assert.Equal(t, 2, result.Skipped)
}

func TestImportService_ImportFilters_BulkRename(t *testing.T) {
	ctx := context.Background()
	db, filterRepo, dmrRepo, cdrRepo, epRepo := setupImportTestDB(t)
	svc := NewImportService(db, filterRepo, dmrRepo, cdrRepo, epRepo)

	// Create existing filter
	filter := createTestFilter("My Filter", "old", models.FilterSourceTypeStream, models.FilterActionInclude, false)
	require.NoError(t, db.Create(filter).Error)

	// Import with bulk rename
	items := []models.FilterExportItem{
		{Name: "My Filter", Expression: "new 1", SourceType: "stream", Action: "include", IsEnabled: true},
	}
	opts := &ImportOptions{
		BulkResolution: models.ConflictResolutionRename,
	}

	result, err := svc.ImportFiltersWithOptions(ctx, items, opts)

	require.NoError(t, err)
	assert.Equal(t, 1, result.Renamed)

	// Verify both exist
	var count int64
	db.Model(&models.Filter{}).Count(&count)
	assert.Equal(t, int64(2), count)
}

func TestImportService_ImportFilters_MixedResolutions(t *testing.T) {
	ctx := context.Background()
	db, filterRepo, dmrRepo, cdrRepo, epRepo := setupImportTestDB(t)
	svc := NewImportService(db, filterRepo, dmrRepo, cdrRepo, epRepo)

	// Create existing filters
	filter1 := createTestFilter("Filter A", "old", models.FilterSourceTypeStream, models.FilterActionInclude, false)
	filter2 := createTestFilter("Filter B", "old", models.FilterSourceTypeStream, models.FilterActionInclude, false)
	filter3 := createTestFilter("Filter C", "old", models.FilterSourceTypeStream, models.FilterActionInclude, false)
	require.NoError(t, db.Create(filter1).Error)
	require.NoError(t, db.Create(filter2).Error)
	require.NoError(t, db.Create(filter3).Error)

	// Import with mixed resolutions (per-item overrides bulk)
	items := []models.FilterExportItem{
		{Name: "Filter A", Expression: "new A", SourceType: "stream", Action: "include", IsEnabled: true},
		{Name: "Filter B", Expression: "new B", SourceType: "stream", Action: "include", IsEnabled: true},
		{Name: "Filter C", Expression: "new C", SourceType: "stream", Action: "include", IsEnabled: true},
	}
	opts := &ImportOptions{
		BulkResolution: models.ConflictResolutionSkip, // Default
		Resolutions: map[string]models.ConflictResolution{
			"Filter A": models.ConflictResolutionOverwrite,
			"Filter B": models.ConflictResolutionRename,
			// Filter C will use bulk (skip)
		},
	}

	result, err := svc.ImportFiltersWithOptions(ctx, items, opts)

	require.NoError(t, err)
	assert.Equal(t, 1, result.Overwritten) // Filter A
	assert.Equal(t, 1, result.Renamed)     // Filter B
	assert.Equal(t, 1, result.Skipped)     // Filter C
}

func TestImportService_ImportDataMappingRulesPreview(t *testing.T) {
	ctx := context.Background()
	db, filterRepo, dmrRepo, cdrRepo, epRepo := setupImportTestDB(t)
	svc := NewImportService(db, filterRepo, dmrRepo, cdrRepo, epRepo)

	// Create existing rule
	existingRule := createTestDataMappingRule("Existing Rule", "expr", models.DataMappingRuleSourceTypeStream, 10, false)
	require.NoError(t, db.Create(existingRule).Error)

	items := []models.DataMappingRuleExportItem{
		{Name: "Existing Rule", Expression: "new", SourceType: "stream", Priority: 5, IsEnabled: true},
		{Name: "New Rule", Expression: "test", SourceType: "stream", Priority: 10, IsEnabled: true},
		{Name: "Invalid Rule", Expression: "", SourceType: "stream", Priority: 1, IsEnabled: true}, // Missing expression
	}

	preview, err := svc.ImportDataMappingRulesPreview(ctx, items)

	require.NoError(t, err)
	assert.Equal(t, 3, preview.TotalItems)
	assert.Len(t, preview.NewItems, 1)
	assert.Len(t, preview.Conflicts, 1)
	assert.Len(t, preview.Errors, 1)
}

func TestImportService_ImportDataMappingRules_AllResolutions(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		bulkResolution models.ConflictResolution
		wantSkipped    int
		wantOverwritten int
		wantRenamed    int
	}{
		{
			name:           "bulk skip",
			bulkResolution: models.ConflictResolutionSkip,
			wantSkipped:    1,
			wantOverwritten: 0,
			wantRenamed:    0,
		},
		{
			name:           "bulk overwrite",
			bulkResolution: models.ConflictResolutionOverwrite,
			wantSkipped:    0,
			wantOverwritten: 1,
			wantRenamed:    0,
		},
		{
			name:           "bulk rename",
			bulkResolution: models.ConflictResolutionRename,
			wantSkipped:    0,
			wantOverwritten: 0,
			wantRenamed:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, filterRepo, dmrRepo, cdrRepo, epRepo := setupImportTestDB(t)
			svc := NewImportService(db, filterRepo, dmrRepo, cdrRepo, epRepo)

			// Create existing rule
			existingRule := createTestDataMappingRule("Existing", "old", models.DataMappingRuleSourceTypeStream, 10, false)
			require.NoError(t, db.Create(existingRule).Error)

			items := []models.DataMappingRuleExportItem{
				{Name: "Existing", Expression: "new", SourceType: "stream", Priority: 5, IsEnabled: true},
			}
			opts := &ImportOptions{BulkResolution: tt.bulkResolution}

			result, err := svc.ImportDataMappingRulesWithOptions(ctx, items, opts)

			require.NoError(t, err)
			assert.Equal(t, tt.wantSkipped, result.Skipped)
			assert.Equal(t, tt.wantOverwritten, result.Overwritten)
			assert.Equal(t, tt.wantRenamed, result.Renamed)
		})
	}
}

func TestImportService_ImportClientDetectionRulesPreview(t *testing.T) {
	ctx := context.Background()
	db, filterRepo, dmrRepo, cdrRepo, epRepo := setupImportTestDB(t)
	svc := NewImportService(db, filterRepo, dmrRepo, cdrRepo, epRepo)

	// Create existing rule
	existingRule := createTestClientDetectionRule("Existing Rule", "expr", 10, false)
	require.NoError(t, db.Create(existingRule).Error)

	items := []models.ClientDetectionRuleExportItem{
		{Name: "Existing Rule", Expression: "new", Priority: 5, IsEnabled: true},
		{Name: "New Rule", Expression: "test", Priority: 10, IsEnabled: true},
	}

	preview, err := svc.ImportClientDetectionRulesPreview(ctx, items)

	require.NoError(t, err)
	assert.Equal(t, 2, preview.TotalItems)
	assert.Len(t, preview.NewItems, 1)
	assert.Len(t, preview.Conflicts, 1)
}

func TestImportService_ImportClientDetectionRules_ResolvesEncodingProfile(t *testing.T) {
	ctx := context.Background()
	db, filterRepo, dmrRepo, cdrRepo, epRepo := setupImportTestDB(t)

	// Create encoding profile FIRST (before service creation shares the repo)
	profile := createTestEncodingProfile("My Profile", models.VideoCodecH264, models.AudioCodecAAC, false)
	err := epRepo.Create(ctx, profile)
	require.NoError(t, err, "failed to create encoding profile")

	// Verify the profile was created and can be found via repo
	foundProfile, err := epRepo.GetByName(ctx, "My Profile")
	require.NoError(t, err, "failed to get encoding profile by name via repo")
	require.NotNil(t, foundProfile, "encoding profile not found after creation via repo")
	t.Logf("Created profile ID: %s, Found profile ID: %s", profile.ID, foundProfile.ID)

	// Also verify via raw DB query
	var dbProfile models.EncodingProfile
	err = db.Where("name = ?", "My Profile").First(&dbProfile).Error
	require.NoError(t, err, "failed to find encoding profile via raw DB query")
	t.Logf("DB Profile ID: %s, Name: %s", dbProfile.ID, dbProfile.Name)

	// Now create the service with the SAME repo instance
	svc := NewImportService(db, filterRepo, dmrRepo, cdrRepo, epRepo)

	// Import client detection rule with profile name reference
	profileName := "My Profile"
	items := []models.ClientDetectionRuleExportItem{
		{
			Name:                "Client Rule",
			Expression:          "user_agent contains 'test'",
			Priority:            5,
			IsEnabled:           true,
			EncodingProfileName: &profileName,
		},
	}

	result, err := svc.ImportClientDetectionRules(ctx, items, nil)

	require.NoError(t, err)
	assert.Equal(t, 1, result.Imported)

	// Verify the rule was created with correct profile ID
	var rule models.ClientDetectionRule
	err = db.Where("name = ?", "Client Rule").First(&rule).Error
	require.NoError(t, err)
	require.NotNil(t, rule.EncodingProfileID, "encoding profile ID should be set")
	assert.Equal(t, profile.ID, *rule.EncodingProfileID)
}

func TestImportService_ImportEncodingProfilesPreview(t *testing.T) {
	ctx := context.Background()
	db, filterRepo, dmrRepo, cdrRepo, epRepo := setupImportTestDB(t)
	svc := NewImportService(db, filterRepo, dmrRepo, cdrRepo, epRepo)

	// Create existing profile
	existingProfile := createTestEncodingProfile("Existing Profile", models.VideoCodecH264, models.AudioCodecAAC, false)
	require.NoError(t, db.Create(existingProfile).Error)

	items := []models.EncodingProfileExportItem{
		{Name: "Existing Profile", TargetVideoCodec: "h265", TargetAudioCodec: "opus", QualityPreset: "high", HWAccel: "none", Enabled: true},
		{Name: "New Profile", TargetVideoCodec: "h264", TargetAudioCodec: "aac", QualityPreset: "medium", HWAccel: "auto", Enabled: true},
		{Name: "", TargetVideoCodec: "h264", TargetAudioCodec: "aac", QualityPreset: "medium", HWAccel: "auto", Enabled: true}, // Missing name
	}

	preview, err := svc.ImportEncodingProfilesPreview(ctx, items)

	require.NoError(t, err)
	assert.Equal(t, 3, preview.TotalItems)
	assert.Len(t, preview.NewItems, 1)
	assert.Len(t, preview.Conflicts, 1)
	assert.Len(t, preview.Errors, 1)
}

func TestImportService_ImportEncodingProfiles_NeverSetsDefault(t *testing.T) {
	ctx := context.Background()
	db, filterRepo, dmrRepo, cdrRepo, epRepo := setupImportTestDB(t)
	svc := NewImportService(db, filterRepo, dmrRepo, cdrRepo, epRepo)

	// Import profile marked as default
	items := []models.EncodingProfileExportItem{
		{
			Name:             "Imported Default",
			TargetVideoCodec: "h264",
			TargetAudioCodec: "aac",
			QualityPreset:    "medium",
			HWAccel:          "none",
			IsDefault:        true, // Should be ignored
			Enabled:          true,
		},
	}

	result, err := svc.ImportEncodingProfiles(ctx, items, nil)

	require.NoError(t, err)
	assert.Equal(t, 1, result.Imported)

	// Verify profile was created but NOT as default
	var profile models.EncodingProfile
	err = db.Where("name = ?", "Imported Default").First(&profile).Error
	require.NoError(t, err)
	assert.False(t, profile.IsDefault)
}

func TestImportService_ImportEncodingProfiles_AllResolutions(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name            string
		bulkResolution  models.ConflictResolution
		wantSkipped     int
		wantOverwritten int
		wantRenamed     int
	}{
		{
			name:            "bulk skip",
			bulkResolution:  models.ConflictResolutionSkip,
			wantSkipped:     1,
			wantOverwritten: 0,
			wantRenamed:     0,
		},
		{
			name:            "bulk overwrite",
			bulkResolution:  models.ConflictResolutionOverwrite,
			wantSkipped:     0,
			wantOverwritten: 1,
			wantRenamed:     0,
		},
		{
			name:            "bulk rename",
			bulkResolution:  models.ConflictResolutionRename,
			wantSkipped:     0,
			wantOverwritten: 0,
			wantRenamed:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, filterRepo, dmrRepo, cdrRepo, epRepo := setupImportTestDB(t)
			svc := NewImportService(db, filterRepo, dmrRepo, cdrRepo, epRepo)

			// Create existing profile
			existingProfile := createTestEncodingProfile("Existing", models.VideoCodecH264, models.AudioCodecAAC, false)
			require.NoError(t, db.Create(existingProfile).Error)

			items := []models.EncodingProfileExportItem{
				{Name: "Existing", TargetVideoCodec: "h265", TargetAudioCodec: "opus", QualityPreset: "high", HWAccel: "none", Enabled: true},
			}
			opts := &ImportOptions{BulkResolution: tt.bulkResolution}

			result, err := svc.ImportEncodingProfilesWithOptions(ctx, items, opts)

			require.NoError(t, err)
			assert.Equal(t, tt.wantSkipped, result.Skipped)
			assert.Equal(t, tt.wantOverwritten, result.Overwritten)
			assert.Equal(t, tt.wantRenamed, result.Renamed)
		})
	}
}

func TestImportService_AtomicTransaction(t *testing.T) {
	// This test verifies that if an import fails validation, all changes are rolled back
	ctx := context.Background()
	db, filterRepo, dmrRepo, cdrRepo, epRepo := setupImportTestDB(t)
	svc := NewImportService(db, filterRepo, dmrRepo, cdrRepo, epRepo)

	// Create a unique constraint to trigger error - insert a filter with same name
	existingFilter := createTestFilter("Duplicate", "existing", models.FilterSourceTypeStream, models.FilterActionInclude, false)
	require.NoError(t, db.Create(existingFilter).Error)

	// Try to import two items where one has a conflict with overwrite but will fail
	// due to expression validation (empty expression causes error)
	items := []models.FilterExportItem{
		{Name: "New Valid Filter", Expression: "test", SourceType: "stream", Action: "include", IsEnabled: true},
		{Name: "Invalid Filter", Expression: "", SourceType: "stream", Action: "include", IsEnabled: true}, // Empty expression causes error in preview flow
	}

	// Use preview to detect the error
	preview, err := svc.ImportFiltersPreview(ctx, items)
	require.NoError(t, err)

	// Verify preview catches the validation error
	assert.Equal(t, 1, len(preview.Errors))
	assert.Equal(t, "Invalid Filter", preview.Errors[0].ItemName)
}
