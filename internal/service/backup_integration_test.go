// Package service provides business logic layer for tvarr operations.
package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmylchreest/tvarr/internal/config"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestBackupRestore_IntegrationRoundTrip verifies that a backup/restore cycle
// preserves all data and entity relationships (ClientDetectionRule→EncodingProfile FK,
// Filter→Source FK).
func TestBackupRestore_IntegrationRoundTrip(t *testing.T) {
	// Create temp directories
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	backupDir := filepath.Join(tempDir, "backups")

	// Setup original database with all entity types
	db := setupIntegrationTestDB(t, dbPath)

	// Create test data with relationships
	ctx := context.Background()

	// Helper to create bool pointer
	boolPtr := func(b bool) *bool { return &b }

	// 1. Create EncodingProfile (referenced by ClientDetectionRule)
	profile := &models.EncodingProfile{
		Name:             "Test Profile",
		TargetVideoCodec: "h264",
		TargetAudioCodec: "aac",
		QualityPreset:    models.QualityPresetMedium,
		HWAccel:          models.HWAccelNone,
		Enabled:          boolPtr(true),
	}
	profile.ID = models.NewULID()
	err := db.Create(profile).Error
	require.NoError(t, err)

	// 2. Create ClientDetectionRule with FK to EncodingProfile
	rule := &models.ClientDetectionRule{
		Name:              "Test Rule",
		Expression:        "user_agent contains 'Chrome'",
		Priority:          1,
		IsEnabled:         boolPtr(true),
		EncodingProfileID: &profile.ID,
	}
	rule.ID = models.NewULID()
	err = db.Create(rule).Error
	require.NoError(t, err)

	// 3. Create Filter
	filter := &models.Filter{
		Name:       "Test Filter",
		Expression: "name contains 'sports'",
		SourceType: models.FilterSourceTypeStream,
		Action:     models.FilterActionInclude,
		IsSystem:   false,
	}
	filter.ID = models.NewULID()
	err = db.Create(filter).Error
	require.NoError(t, err)

	// 4. Create DataMappingRule
	mappingRule := &models.DataMappingRule{
		Name:       "Test Mapping",
		Expression: "name = concat(name, ' - HD')",
		SourceType: models.DataMappingRuleSourceTypeStream,
		Priority:   1,
		IsEnabled:  boolPtr(true),
		IsSystem:   false,
	}
	mappingRule.ID = models.NewULID()
	err = db.Create(mappingRule).Error
	require.NoError(t, err)

	// Store original IDs for verification
	originalProfileID := profile.ID
	originalRuleID := rule.ID
	originalFilterID := filter.ID
	originalMappingRuleID := mappingRule.ID

	// Close original database
	sqlDB, _ := db.DB()
	sqlDB.Close()

	// Reopen for backup service
	db, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Create backup
	cfg := config.BackupConfig{
		Directory: backupDir,
		Schedule: config.BackupScheduleConfig{
			Enabled:   false,
			Retention: 7,
		},
	}
	backupService := NewBackupService(db, cfg, tempDir)

	meta, err := backupService.CreateBackup(ctx)
	require.NoError(t, err)
	require.NotNil(t, meta)

	// Close database
	sqlDB, _ = db.DB()
	sqlDB.Close()

	// Now simulate "wiping" the database by deleting it and creating a fresh empty one
	// at the same path - this simulates the "new installation" scenario
	err = os.Remove(dbPath)
	require.NoError(t, err)

	// Create fresh empty database at original path
	db = setupIntegrationTestDB(t, dbPath)

	// Verify the new database is empty
	var filterCount int64
	db.Model(&models.Filter{}).Count(&filterCount)
	assert.Zero(t, filterCount, "Fresh database should be empty")

	// Create a new backup service pointing to the same database path
	backupService = NewBackupService(db, cfg, tempDir)

	// Close the DB connection before restore (simulates real restore scenario)
	sqlDB, _ = db.DB()
	sqlDB.Close()

	// Reopen with fresh connection for restore
	db, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	backupService = NewBackupService(db, cfg, tempDir)

	// Restore from backup
	err = backupService.RestoreBackup(ctx, meta.Filename)
	require.NoError(t, err)

	// Close and reopen to get fresh connection to restored data
	sqlDB, _ = db.DB()
	sqlDB.Close()

	// Open the restored database
	restoredDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	defer func() {
		sqlDB, _ := restoredDB.DB()
		sqlDB.Close()
	}()

	// Verify all entities are restored with correct data

	// 1. Verify EncodingProfile
	var restoredProfile models.EncodingProfile
	err = restoredDB.First(&restoredProfile, "id = ?", originalProfileID.String()).Error
	require.NoError(t, err)
	assert.Equal(t, "Test Profile", restoredProfile.Name)
	assert.Equal(t, models.VideoCodec("h264"), restoredProfile.TargetVideoCodec)
	assert.Equal(t, models.AudioCodec("aac"), restoredProfile.TargetAudioCodec)
	require.NotNil(t, restoredProfile.Enabled)
	assert.True(t, *restoredProfile.Enabled)

	// 2. Verify ClientDetectionRule with FK relationship
	var restoredRule models.ClientDetectionRule
	err = restoredDB.First(&restoredRule, "id = ?", originalRuleID.String()).Error
	require.NoError(t, err)
	assert.Equal(t, "Test Rule", restoredRule.Name)
	assert.Equal(t, "user_agent contains 'Chrome'", restoredRule.Expression)
	require.NotNil(t, restoredRule.EncodingProfileID, "EncodingProfileID FK should be preserved")
	assert.Equal(t, originalProfileID, *restoredRule.EncodingProfileID, "FK relationship should point to correct profile")

	// 3. Verify Filter
	var restoredFilter models.Filter
	err = restoredDB.First(&restoredFilter, "id = ?", originalFilterID.String()).Error
	require.NoError(t, err)
	assert.Equal(t, "Test Filter", restoredFilter.Name)
	assert.Equal(t, "name contains 'sports'", restoredFilter.Expression)
	assert.Equal(t, models.FilterSourceTypeStream, restoredFilter.SourceType)
	assert.Equal(t, models.FilterActionInclude, restoredFilter.Action)

	// 4. Verify DataMappingRule
	var restoredMappingRule models.DataMappingRule
	err = restoredDB.First(&restoredMappingRule, "id = ?", originalMappingRuleID.String()).Error
	require.NoError(t, err)
	assert.Equal(t, "Test Mapping", restoredMappingRule.Name)
	assert.Equal(t, "name = concat(name, ' - HD')", restoredMappingRule.Expression)
	assert.Equal(t, models.DataMappingRuleSourceTypeStream, restoredMappingRule.SourceType)

	// Verify counts match
	var profileCount, ruleCount, filterCountAfter, mappingCount int64
	restoredDB.Model(&models.EncodingProfile{}).Count(&profileCount)
	restoredDB.Model(&models.ClientDetectionRule{}).Count(&ruleCount)
	restoredDB.Model(&models.Filter{}).Count(&filterCountAfter)
	restoredDB.Model(&models.DataMappingRule{}).Count(&mappingCount)

	assert.Equal(t, int64(1), profileCount, "Should have 1 encoding profile")
	assert.Equal(t, int64(1), ruleCount, "Should have 1 client detection rule")
	assert.Equal(t, int64(1), filterCountAfter, "Should have 1 filter")
	assert.Equal(t, int64(1), mappingCount, "Should have 1 data mapping rule")
}

func setupIntegrationTestDB(t *testing.T, dbPath string) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Auto-migrate all relevant tables
	err = db.AutoMigrate(
		&models.Filter{},
		&models.DataMappingRule{},
		&models.ClientDetectionRule{},
		&models.EncodingProfile{},
	)
	require.NoError(t, err)

	return db
}
