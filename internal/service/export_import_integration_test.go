// Package service provides business logic layer for tvarr operations.
package service

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestExportImportRoundTrip_Filters tests that filters can be exported and re-imported correctly.
func TestExportImportRoundTrip_Filters(t *testing.T) {
	// Setup source database with test data
	sourceDB, _, sourceExportService := setupIntegrationTestEnv(t)
	defer closeDB(t, sourceDB)

	ctx := context.Background()

	// Create test filters in source database
	filters := []*models.Filter{
		createTestFilter("Filter A", "a > 1", models.FilterSourceTypeStream, models.FilterActionInclude, false),
		createTestFilter("Filter B", "b > 2", models.FilterSourceTypeStream, models.FilterActionInclude, false),
		createTestFilter("System Filter", "sys > 0", models.FilterSourceTypeStream, models.FilterActionInclude, true), // Should be excluded
	}
	for _, f := range filters {
		err := sourceDB.Create(f).Error
		require.NoError(t, err)
	}

	// Export filters
	export, err := sourceExportService.ExportFilters(ctx, nil, true)
	require.NoError(t, err)
	assert.Len(t, export.Items, 2, "should export only user-created filters")

	// Setup destination database
	destDB, destFilterRepo, _ := setupIntegrationTestEnv(t)
	defer closeDB(t, destDB)

	destImportService := NewImportService(
		destDB,
		destFilterRepo,
		repository.NewDataMappingRuleRepository(destDB),
		repository.NewClientDetectionRuleRepository(destDB),
		repository.NewEncodingProfileRepository(destDB),
	)

	// Import into destination database
	result, err := destImportService.ImportFilters(ctx, export.Items, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Imported)
	assert.Equal(t, 0, result.Errors)
	assert.Equal(t, 0, result.Skipped)

	// Verify imported filters in destination
	destFilters, err := destFilterRepo.GetUserCreated(ctx)
	require.NoError(t, err)
	assert.Len(t, destFilters, 2)

	// Verify filter content
	filterMap := make(map[string]*models.Filter)
	for _, f := range destFilters {
		filterMap[f.Name] = f
	}

	filterA := filterMap["Filter A"]
	require.NotNil(t, filterA)
	assert.Equal(t, "a > 1", filterA.Expression)
	assert.False(t, filterA.IsSystem)

	filterB := filterMap["Filter B"]
	require.NotNil(t, filterB)
	assert.Equal(t, "b > 2", filterB.Expression)
	assert.False(t, filterB.IsSystem)
}

// TestExportImportRoundTrip_DataMappingRules tests that data mapping rules can be exported and re-imported correctly.
func TestExportImportRoundTrip_DataMappingRules(t *testing.T) {
	sourceDB, _, sourceExportService := setupIntegrationTestEnv(t)
	defer closeDB(t, sourceDB)

	ctx := context.Background()

	// Create test data mapping rules
	rules := []*models.DataMappingRule{
		createTestDataMappingRule("Rule A", "match(.*)", models.DataMappingRuleSourceTypeStream, 100, false),
		createTestDataMappingRule("Rule B", "replace(x,y)", models.DataMappingRuleSourceTypeStream, 200, false),
	}
	for _, r := range rules {
		err := sourceDB.Create(r).Error
		require.NoError(t, err)
	}

	// Export rules
	export, err := sourceExportService.ExportDataMappingRules(ctx, nil, true)
	require.NoError(t, err)
	assert.Len(t, export.Items, 2)

	// Setup destination database
	destDB, destFilterRepo, _ := setupIntegrationTestEnv(t)
	defer closeDB(t, destDB)

	destImportService := NewImportService(
		destDB,
		destFilterRepo,
		repository.NewDataMappingRuleRepository(destDB),
		repository.NewClientDetectionRuleRepository(destDB),
		repository.NewEncodingProfileRepository(destDB),
	)

	// Import into destination
	result, err := destImportService.ImportDataMappingRules(ctx, export.Items, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Imported)
	assert.Equal(t, 0, result.Errors)

	// Verify imported rules
	var destRules []models.DataMappingRule
	err = destDB.Find(&destRules).Error
	require.NoError(t, err)
	assert.Len(t, destRules, 2)

	ruleMap := make(map[string]models.DataMappingRule)
	for _, r := range destRules {
		ruleMap[r.Name] = r
	}

	ruleA := ruleMap["Rule A"]
	assert.Equal(t, "match(.*)", ruleA.Expression)
	assert.Equal(t, 100, ruleA.Priority)

	ruleB := ruleMap["Rule B"]
	assert.Equal(t, "replace(x,y)", ruleB.Expression)
	assert.Equal(t, 200, ruleB.Priority)
}

// TestExportImportRoundTrip_EncodingProfiles tests that encoding profiles can be exported and re-imported correctly.
func TestExportImportRoundTrip_EncodingProfiles(t *testing.T) {
	sourceDB, _, sourceExportService := setupIntegrationTestEnv(t)
	defer closeDB(t, sourceDB)

	ctx := context.Background()

	// Create test encoding profiles
	profiles := []*models.EncodingProfile{
		createTestEncodingProfile("Profile A", models.VideoCodecH264, models.AudioCodecAAC, false),
		createTestEncodingProfile("Profile B", models.VideoCodecH265, models.AudioCodecOpus, false),
		createTestEncodingProfile("System Profile", models.VideoCodecH264, models.AudioCodecAAC, true), // Should be excluded
	}
	for _, p := range profiles {
		err := sourceDB.Create(p).Error
		require.NoError(t, err)
	}

	// Export profiles
	export, err := sourceExportService.ExportEncodingProfiles(ctx, nil, true)
	require.NoError(t, err)
	assert.Len(t, export.Items, 2, "should export only user-created profiles")

	// Setup destination database
	destDB, destFilterRepo, _ := setupIntegrationTestEnv(t)
	defer closeDB(t, destDB)

	destImportService := NewImportService(
		destDB,
		destFilterRepo,
		repository.NewDataMappingRuleRepository(destDB),
		repository.NewClientDetectionRuleRepository(destDB),
		repository.NewEncodingProfileRepository(destDB),
	)

	// Import into destination
	result, err := destImportService.ImportEncodingProfiles(ctx, export.Items, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Imported)
	assert.Equal(t, 0, result.Errors)

	// Verify imported profiles
	var destProfiles []models.EncodingProfile
	err = destDB.Find(&destProfiles).Error
	require.NoError(t, err)
	assert.Len(t, destProfiles, 2)

	profileMap := make(map[string]models.EncodingProfile)
	for _, p := range destProfiles {
		profileMap[p.Name] = p
	}

	profileA := profileMap["Profile A"]
	assert.Equal(t, models.VideoCodecH264, profileA.TargetVideoCodec)
	assert.Equal(t, models.AudioCodecAAC, profileA.TargetAudioCodec)
	assert.False(t, profileA.IsSystem)
	assert.False(t, profileA.IsDefault) // Imported profiles should never be default

	profileB := profileMap["Profile B"]
	assert.Equal(t, models.VideoCodecH265, profileB.TargetVideoCodec)
	assert.Equal(t, models.AudioCodecOpus, profileB.TargetAudioCodec)
	assert.False(t, profileB.IsSystem)
}

// TestExportImportRoundTrip_ClientDetectionRules tests that client detection rules can be exported and re-imported correctly.
func TestExportImportRoundTrip_ClientDetectionRules(t *testing.T) {
	sourceDB, _, sourceExportService := setupIntegrationTestEnv(t)
	defer closeDB(t, sourceDB)

	ctx := context.Background()

	// First create an encoding profile to reference
	profile := createTestEncodingProfile("Test Profile", models.VideoCodecH264, models.AudioCodecAAC, false)
	err := sourceDB.Create(profile).Error
	require.NoError(t, err)

	// Create test client detection rules (use the existing helper then set encoding profile)
	ruleA := createTestClientDetectionRule("Rule A", "Safari", 100, false)
	ruleA.EncodingProfileID = &profile.ID
	ruleB := createTestClientDetectionRule("Rule B", "Chrome", 200, false)
	rules := []*models.ClientDetectionRule{ruleA, ruleB}
	for _, r := range rules {
		err := sourceDB.Create(r).Error
		require.NoError(t, err)
	}

	// Export rules
	export, err := sourceExportService.ExportClientDetectionRules(ctx, nil, true)
	require.NoError(t, err)
	assert.Len(t, export.Items, 2)

	// Verify encoding profile name is included
	var foundRuleWithProfile bool
	for _, item := range export.Items {
		if item.Name == "Rule A" && item.EncodingProfileName != nil && *item.EncodingProfileName == "Test Profile" {
			foundRuleWithProfile = true
		}
	}
	assert.True(t, foundRuleWithProfile, "exported rule should include encoding profile name")

	// Setup destination database with the same encoding profile
	destDB, destFilterRepo, _ := setupIntegrationTestEnv(t)
	defer closeDB(t, destDB)

	// Create the encoding profile in destination
	destProfile := createTestEncodingProfile("Test Profile", models.VideoCodecH264, models.AudioCodecAAC, false)
	err = destDB.Create(destProfile).Error
	require.NoError(t, err)

	destImportService := NewImportService(
		destDB,
		destFilterRepo,
		repository.NewDataMappingRuleRepository(destDB),
		repository.NewClientDetectionRuleRepository(destDB),
		repository.NewEncodingProfileRepository(destDB),
	)

	// Import into destination
	result, err := destImportService.ImportClientDetectionRules(ctx, export.Items, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Imported)
	assert.Equal(t, 0, result.Errors)

	// Verify imported rules
	var destRules []models.ClientDetectionRule
	err = destDB.Find(&destRules).Error
	require.NoError(t, err)
	assert.Len(t, destRules, 2)

	ruleMap := make(map[string]models.ClientDetectionRule)
	for _, r := range destRules {
		ruleMap[r.Name] = r
	}

	importedRuleA := ruleMap["Rule A"]
	assert.Equal(t, "Safari", importedRuleA.Expression)
	assert.Equal(t, 100, importedRuleA.Priority)
	assert.NotNil(t, importedRuleA.EncodingProfileID)
	assert.Equal(t, destProfile.ID.String(), importedRuleA.EncodingProfileID.String())

	importedRuleB := ruleMap["Rule B"]
	assert.Equal(t, "Chrome", importedRuleB.Expression)
	assert.Equal(t, 200, importedRuleB.Priority)
	assert.Nil(t, importedRuleB.EncodingProfileID)
}

// TestExportImportRoundTrip_WithConflictResolutions tests export/import with various conflict resolutions.
func TestExportImportRoundTrip_WithConflictResolutions(t *testing.T) {
	sourceDB, _, sourceExportService := setupIntegrationTestEnv(t)
	defer closeDB(t, sourceDB)

	ctx := context.Background()

	// Create source filters
	sourceFilters := []*models.Filter{
		createTestFilter("Duplicate Filter", "original > 1", models.FilterSourceTypeStream, models.FilterActionInclude, false),
		createTestFilter("Unique Filter", "unique > 1", models.FilterSourceTypeStream, models.FilterActionInclude, false),
	}
	for _, f := range sourceFilters {
		err := sourceDB.Create(f).Error
		require.NoError(t, err)
	}

	// Export filters
	export, err := sourceExportService.ExportFilters(ctx, nil, true)
	require.NoError(t, err)
	assert.Len(t, export.Items, 2)

	// Modify export items to have different values (simulating edits before re-import)
	for i := range export.Items {
		if export.Items[i].Name == "Duplicate Filter" {
			export.Items[i].Expression = "modified > 999"
		}
	}

	// Setup destination database with pre-existing filter
	destDB, destFilterRepo, _ := setupIntegrationTestEnv(t)
	defer closeDB(t, destDB)

	existingFilter := createTestFilter("Duplicate Filter", "existing > 2", models.FilterSourceTypeStream, models.FilterActionInclude, false)
	err = destDB.Create(existingFilter).Error
	require.NoError(t, err)

	destImportService := NewImportService(
		destDB,
		destFilterRepo,
		repository.NewDataMappingRuleRepository(destDB),
		repository.NewClientDetectionRuleRepository(destDB),
		repository.NewEncodingProfileRepository(destDB),
	)

	t.Run("overwrite conflict", func(t *testing.T) {
		// Import with overwrite resolution
		result, err := destImportService.ImportFilters(ctx, export.Items, map[string]models.ConflictResolution{
			"Duplicate Filter": models.ConflictResolutionOverwrite,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, result.Overwritten)
		assert.Equal(t, 1, result.Imported)

		// Verify the filter was overwritten
		filters, err := destFilterRepo.GetAll(ctx)
		require.NoError(t, err)
		assert.Len(t, filters, 2)

		for _, f := range filters {
			if f.Name == "Duplicate Filter" {
				assert.Equal(t, "modified > 999", f.Expression)
			}
		}
	})

	t.Run("rename conflict", func(t *testing.T) {
		// Clean up and re-setup
		destDB2, destFilterRepo2, _ := setupIntegrationTestEnv(t)
		defer closeDB(t, destDB2)

		existingFilter2 := createTestFilter("Duplicate Filter", "existing > 2", models.FilterSourceTypeStream, models.FilterActionInclude, false)
		err := destDB2.Create(existingFilter2).Error
		require.NoError(t, err)

		destImportService2 := NewImportService(
			destDB2,
			destFilterRepo2,
			repository.NewDataMappingRuleRepository(destDB2),
			repository.NewClientDetectionRuleRepository(destDB2),
			repository.NewEncodingProfileRepository(destDB2),
		)

		// Import with rename resolution
		result, err := destImportService2.ImportFilters(ctx, export.Items, map[string]models.ConflictResolution{
			"Duplicate Filter": models.ConflictResolutionRename,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, result.Renamed)
		assert.Equal(t, 1, result.Imported)

		// Verify the original filter is preserved and renamed one was created
		filters, err := destFilterRepo2.GetAll(ctx)
		require.NoError(t, err)
		assert.Len(t, filters, 3) // existing + renamed + unique

		filterNames := make(map[string]bool)
		for _, f := range filters {
			filterNames[f.Name] = true
		}
		assert.True(t, filterNames["Duplicate Filter"], "original filter should exist")
		assert.True(t, filterNames["Duplicate Filter (1)"], "renamed filter should exist")
		assert.True(t, filterNames["Unique Filter"], "unique filter should exist")
	})
}

// TestExportImportRoundTrip_PreservesAllFields tests that all fields are preserved during export/import.
func TestExportImportRoundTrip_PreservesAllFields(t *testing.T) {
	sourceDB, _, sourceExportService := setupIntegrationTestEnv(t)
	defer closeDB(t, sourceDB)

	ctx := context.Background()

	// Create filter with all fields populated
	sourceID := models.NewULID()
	filter := &models.Filter{
		BaseModel:   models.BaseModel{ID: models.NewULID()},
		Name:        "Complete Filter",
		Description: "A detailed description",
		Expression:  "complex.expression > 100",
		SourceType:  models.FilterSourceTypeStream,
		Action:      models.FilterActionExclude,
		IsSystem:    false,
		SourceID:    &sourceID,
	}
	err := sourceDB.Create(filter).Error
	require.NoError(t, err)

	// Export
	export, err := sourceExportService.ExportFilters(ctx, nil, true)
	require.NoError(t, err)
	assert.Len(t, export.Items, 1)

	item := export.Items[0]
	assert.Equal(t, "Complete Filter", item.Name)
	assert.Equal(t, "A detailed description", item.Description)
	assert.Equal(t, "complex.expression > 100", item.Expression)
	assert.Equal(t, string(models.FilterSourceTypeStream), item.SourceType)
	assert.Equal(t, string(models.FilterActionExclude), item.Action)
	assert.NotNil(t, item.SourceID)
	assert.Equal(t, sourceID.String(), *item.SourceID)

	// Import to new database
	destDB, destFilterRepo, _ := setupIntegrationTestEnv(t)
	defer closeDB(t, destDB)

	destImportService := NewImportService(
		destDB,
		destFilterRepo,
		repository.NewDataMappingRuleRepository(destDB),
		repository.NewClientDetectionRuleRepository(destDB),
		repository.NewEncodingProfileRepository(destDB),
	)

	result, err := destImportService.ImportFilters(ctx, export.Items, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Imported)

	// Verify all fields preserved
	imported, err := destFilterRepo.GetByName(ctx, "Complete Filter")
	require.NoError(t, err)
	require.NotNil(t, imported)

	assert.Equal(t, "Complete Filter", imported.Name)
	assert.Equal(t, "A detailed description", imported.Description)
	assert.Equal(t, "complex.expression > 100", imported.Expression)
	assert.Equal(t, models.FilterSourceTypeStream, imported.SourceType)
	assert.Equal(t, models.FilterActionExclude, imported.Action)
	assert.False(t, imported.IsSystem) // Always false on import
	assert.NotNil(t, imported.SourceID)
	assert.Equal(t, sourceID.String(), imported.SourceID.String())
}

// Helper function to setup test environment for integration tests
func setupIntegrationTestEnv(t *testing.T) (*gorm.DB, repository.FilterRepository, *ExportService) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Limit connections for SQLite in-memory
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	// Auto-migrate tables
	err = db.AutoMigrate(
		&models.Filter{},
		&models.DataMappingRule{},
		&models.ClientDetectionRule{},
		&models.EncodingProfile{},
	)
	require.NoError(t, err)

	filterRepo := repository.NewFilterRepository(db)
	dataMappingRuleRepo := repository.NewDataMappingRuleRepository(db)
	clientDetectionRuleRepo := repository.NewClientDetectionRuleRepository(db)
	encodingProfileRepo := repository.NewEncodingProfileRepository(db)

	exportService := NewExportService(filterRepo, dataMappingRuleRepo, clientDetectionRuleRepo, encodingProfileRepo)

	return db, filterRepo, exportService
}

func closeDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	sqlDB, err := db.DB()
	if err == nil {
		sqlDB.Close()
	}
}
