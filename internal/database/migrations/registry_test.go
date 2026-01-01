package migrations

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	return db
}

func TestAllMigrations_ReturnsExpectedCount(t *testing.T) {
	migrations := AllMigrations()

	// Migrations:
	// 001: Create all database tables (schema)
	// 002: Insert default filters, rules, profiles, and mappings (system data)
	// 003: Legacy cleanup (no-op)
	// 004: Add EncodingProfile model
	// 005: Add ClientDetectionRule model
	// 006: Add explicit codec header detection rules
	// 007: Rename EPG timezone fields
	// 008: Remove redundant priority column from filters table
	// 009: Add dynamic codec header fields to client detection rules
	// 010: Update client detection rules to use @dynamic() syntax for user-agent
	// 011: Add is_active column to proxy_filters
	// 012: Add auto_shift_timezone column to epg_sources
	// 013: Add system client detection rules for popular media players
	// 014: Rename system timeshift detection rule to shorter name
	// 015: Add default channel grouping data mapping rules and filters
	// 016: Fix grouping rules: enable country/adult, reorder priorities, rename to Group
	// 017: Remove is_enabled column from filters (filters are enabled/disabled at proxy level)
	// 018: Add backup_settings table for user-configurable backup schedule
	// 019: Fix duplicate Exclude Adult Content filters and upgrade expression
	// 020: Add ffmpegd_config table for distributed transcoding configuration
	// 021: Add max_concurrent_streams column to stream_sources table
	// 022: Add encoder_overrides table for hardware encoder workarounds
	// 023: Remove deprecated client_detection_enabled column from stream_proxies
	// 024: Fix dynamic codec rules to only contribute their own field
	assert.Len(t, migrations, 24)
}

func TestAllMigrations_VersionsAreUnique(t *testing.T) {
	migrations := AllMigrations()
	versions := make(map[string]bool)

	for _, m := range migrations {
		assert.False(t, versions[m.Version], "duplicate version: %s", m.Version)
		versions[m.Version] = true
	}
}

func TestAllMigrations_VersionsAreOrdered(t *testing.T) {
	migrations := AllMigrations()

	for i := 1; i < len(migrations); i++ {
		assert.Less(t, migrations[i-1].Version, migrations[i].Version,
			"migrations should be in ascending version order")
	}
}

func TestMigrator_Up_AllMigrations(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	migrator := NewMigrator(db, nil)
	migrator.RegisterAll(AllMigrations())

	err := migrator.Up(ctx)
	require.NoError(t, err)

	// Verify all tables exist
	assert.True(t, db.Migrator().HasTable("stream_sources"))
	assert.True(t, db.Migrator().HasTable("channels"))
	assert.True(t, db.Migrator().HasTable("manual_stream_channels"))
	assert.True(t, db.Migrator().HasTable("epg_sources"))
	assert.True(t, db.Migrator().HasTable("epg_programs"))
	assert.True(t, db.Migrator().HasTable("stream_proxies"))
	assert.True(t, db.Migrator().HasTable("proxy_sources"))
	assert.True(t, db.Migrator().HasTable("proxy_epg_sources"))
	assert.True(t, db.Migrator().HasTable("proxy_filters"))
	assert.True(t, db.Migrator().HasTable("proxy_mapping_rules"))
	assert.True(t, db.Migrator().HasTable("filters"))
	assert.True(t, db.Migrator().HasTable("data_mapping_rules"))
	// Note: logo_assets table removed - logo caching uses file-based storage
}

func TestMigrator_Up_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	migrator := NewMigrator(db, nil)
	migrator.RegisterAll(AllMigrations())

	// Run migrations twice - should not error
	err := migrator.Up(ctx)
	require.NoError(t, err)

	err = migrator.Up(ctx)
	require.NoError(t, err)
}

func TestMigrator_Status(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	migrator := NewMigrator(db, nil)
	migrator.RegisterAll(AllMigrations())

	// Before running migrations (24 migrations total)
	statuses, err := migrator.Status(ctx)
	require.NoError(t, err)
	assert.Len(t, statuses, 24)

	for _, s := range statuses {
		assert.False(t, s.Applied)
		assert.Nil(t, s.AppliedAt)
	}

	// After running migrations
	err = migrator.Up(ctx)
	require.NoError(t, err)

	statuses, err = migrator.Status(ctx)
	require.NoError(t, err)

	for _, s := range statuses {
		assert.True(t, s.Applied)
		assert.NotNil(t, s.AppliedAt)
	}
}

func TestMigrator_Down_RollsBackLastMigration(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	migrator := NewMigrator(db, nil)
	migrator.RegisterAll(AllMigrations())

	err := migrator.Up(ctx)
	require.NoError(t, err)

	// After running all migrations, tables should exist
	assert.True(t, db.Migrator().HasTable("filters"))
	assert.True(t, db.Migrator().HasTable("data_mapping_rules"))
	assert.True(t, db.Migrator().HasTable("encoding_profiles"))
	assert.True(t, db.Migrator().HasTable("client_detection_rules"))
	assert.True(t, db.Migrator().HasTable("backup_settings"))
	assert.True(t, db.Migrator().HasTable("ffmpegd_config"))
	assert.True(t, db.Migrator().HasTable("encoder_overrides"))

	// Roll back migration 024 (fix dynamic codec rules)
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Tables still exist after rolling back codec rule fix
	assert.True(t, db.Migrator().HasTable("client_detection_rules"))
	assert.True(t, db.Migrator().HasTable("encoder_overrides"))

	// Roll back migration 023 (remove deprecated column)
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Tables still exist after rolling back column removal
	assert.True(t, db.Migrator().HasTable("stream_proxies"))
	assert.True(t, db.Migrator().HasTable("encoder_overrides"))

	// Roll back migration 022 (encoder_overrides table)
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// encoder_overrides table should be removed
	assert.False(t, db.Migrator().HasTable("encoder_overrides"))
	assert.True(t, db.Migrator().HasTable("stream_sources"))

	// Roll back migration 021 (max_concurrent_streams column)
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Tables still exist after rolling back max_concurrent_streams column
	assert.True(t, db.Migrator().HasTable("stream_sources"))
	assert.True(t, db.Migrator().HasTable("ffmpegd_config"))

	// Roll back migration 020 (ffmpegd_config table)
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// ffmpegd_config table should be removed
	assert.False(t, db.Migrator().HasTable("ffmpegd_config"))
	assert.True(t, db.Migrator().HasTable("backup_settings"))

	// Roll back migration 019 (fix duplicate filters)
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Tables still exist after rolling back duplicate filter fix
	assert.True(t, db.Migrator().HasTable("filters"))
	assert.True(t, db.Migrator().HasTable("backup_settings"))

	// Roll back migration 018 (backup_settings table)
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// backup_settings table should be removed
	assert.False(t, db.Migrator().HasTable("backup_settings"))

	// Roll back migration 017 (remove filter is_enabled column)
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Tables still exist after rolling back is_enabled column removal
	assert.True(t, db.Migrator().HasTable("filters"))
	assert.True(t, db.Migrator().HasTable("data_mapping_rules"))

	// Roll back migration 016 (fix grouping rules)
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Tables still exist after rolling back fix grouping rules
	assert.True(t, db.Migrator().HasTable("filters"))
	assert.True(t, db.Migrator().HasTable("data_mapping_rules"))

	// Roll back migration 015 (default grouping rules and filters)
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Tables still exist after rolling back grouping rules
	assert.True(t, db.Migrator().HasTable("filters"))
	assert.True(t, db.Migrator().HasTable("data_mapping_rules"))

	// Roll back migration 014 (rename timeshift rule)
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Tables still exist after rolling back timeshift rule rename
	assert.True(t, db.Migrator().HasTable("filters"))
	assert.True(t, db.Migrator().HasTable("data_mapping_rules"))

	// Roll back migration 013 (system client detection rules)
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Tables still exist after rolling back client detection rules
	assert.True(t, db.Migrator().HasTable("filters"))
	assert.True(t, db.Migrator().HasTable("client_detection_rules"))

	// Roll back migration 012 (auto_shift_timezone column)
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Tables still exist after rolling back auto_shift_timezone column
	assert.True(t, db.Migrator().HasTable("filters"))
	assert.True(t, db.Migrator().HasTable("epg_sources"))

	// Roll back migration 011 (proxy_filter is_active column)
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Tables still exist after rolling back is_active column
	assert.True(t, db.Migrator().HasTable("filters"))
	assert.True(t, db.Migrator().HasTable("proxy_filters"))

	// Roll back migration 010 (update user-agent syntax)
	// This reverts expression syntax in client detection rules
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Tables still exist after rolling back user-agent syntax update
	assert.True(t, db.Migrator().HasTable("filters"))
	assert.True(t, db.Migrator().HasTable("client_detection_rules"))

	// Roll back migration 009 (dynamic codec header fields)
	// This removes columns and reverts data, tables remain
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Tables still exist after rolling back dynamic codec headers
	assert.True(t, db.Migrator().HasTable("filters"))
	assert.True(t, db.Migrator().HasTable("client_detection_rules"))

	// Roll back migration 008 (remove filter priority column)
	// This only re-adds a column, tables remain
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Tables still exist after rolling back filter priority removal
	assert.True(t, db.Migrator().HasTable("filters"))
	assert.True(t, db.Migrator().HasTable("encoding_profiles"))
	assert.True(t, db.Migrator().HasTable("client_detection_rules"))

	// Roll back migration 007 (EPG timezone field renames)
	// This only renames columns, tables remain
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Tables still exist after rolling back EPG timezone renames
	assert.True(t, db.Migrator().HasTable("filters"))
	assert.True(t, db.Migrator().HasTable("epg_sources"))
	assert.True(t, db.Migrator().HasTable("client_detection_rules"))

	// Roll back migration 006 (explicit codec header rules)
	// This only deletes data, tables remain
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Tables still exist after rolling back explicit codec header rules
	assert.True(t, db.Migrator().HasTable("filters"))
	assert.True(t, db.Migrator().HasTable("encoding_profiles"))
	assert.True(t, db.Migrator().HasTable("client_detection_rules"))

	// Roll back migration 005 (client detection)
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Tables still exist after rolling back client detection migration
	assert.True(t, db.Migrator().HasTable("filters"))
	assert.True(t, db.Migrator().HasTable("encoding_profiles"))
	assert.False(t, db.Migrator().HasTable("client_detection_rules"))

	// Roll back migration 004 (encoding profiles)
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Tables still exist after rolling back encoding profile migration
	assert.True(t, db.Migrator().HasTable("filters"))
	assert.False(t, db.Migrator().HasTable("encoding_profiles"))

	// Roll back migration 003 (cleanup - no-op down)
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Tables still exist after rolling back cleanup migration
	assert.True(t, db.Migrator().HasTable("filters"))

	// Roll back migration 002 (system data)
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Tables still exist after rolling back system data (only data deleted)
	assert.True(t, db.Migrator().HasTable("filters"))

	// Roll back migration 001 (schema)
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Tables should no longer exist
	assert.False(t, db.Migrator().HasTable("filters"))
	assert.False(t, db.Migrator().HasTable("data_mapping_rules"))
}

func TestMigrator_Pending(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	migrator := NewMigrator(db, nil)
	migrator.RegisterAll(AllMigrations())

	// All should be pending initially (24 migrations total)
	pending, err := migrator.Pending(ctx)
	require.NoError(t, err)
	assert.Len(t, pending, 24)

	// Run migrations
	err = migrator.Up(ctx)
	require.NoError(t, err)

	// None should be pending
	pending, err = migrator.Pending(ctx)
	require.NoError(t, err)
	assert.Len(t, pending, 0)
}

func TestMigrations_CanInsertData(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	migrator := NewMigrator(db, nil)
	migrator.RegisterAll(AllMigrations())

	err := migrator.Up(ctx)
	require.NoError(t, err)

	// Test StreamSource insert
	source := &models.StreamSource{
		Name: "Test Source",
		Type: models.SourceTypeM3U,
		URL:  "http://example.com/playlist.m3u",
	}
	err = db.Create(source).Error
	require.NoError(t, err)
	assert.NotZero(t, source.ID)

	// Test EpgSource insert
	epgSource := &models.EpgSource{
		Name: "Test EPG",
		Type: models.EpgSourceTypeXMLTV,
		URL:  "http://example.com/epg.xml",
	}
	err = db.Create(epgSource).Error
	require.NoError(t, err)
	assert.NotZero(t, epgSource.ID)

	// Test StreamProxy insert
	proxy := &models.StreamProxy{
		Name:                  "Test Proxy",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
	}
	err = db.Create(proxy).Error
	require.NoError(t, err)
	assert.NotZero(t, proxy.ID)

	// Test ProxySource join table
	proxySource := &models.ProxySource{
		ProxyID:  proxy.ID,
		SourceID: source.ID,
		Priority: 1,
	}
	err = db.Create(proxySource).Error
	require.NoError(t, err)

	// Test ProxyEpgSource join table
	proxyEpg := &models.ProxyEpgSource{
		ProxyID:     proxy.ID,
		EpgSourceID: epgSource.ID,
		Priority:    1,
	}
	err = db.Create(proxyEpg).Error
	require.NoError(t, err)
}

func TestMigrations_StreamProxyRelationships(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	migrator := NewMigrator(db, nil)
	migrator.RegisterAll(AllMigrations())

	err := migrator.Up(ctx)
	require.NoError(t, err)

	// Create test data
	source1 := &models.StreamSource{Name: "Source 1", Type: models.SourceTypeM3U, URL: "http://example.com/1.m3u"}
	source2 := &models.StreamSource{Name: "Source 2", Type: models.SourceTypeM3U, URL: "http://example.com/2.m3u"}
	require.NoError(t, db.Create(source1).Error)
	require.NoError(t, db.Create(source2).Error)

	epgSource := &models.EpgSource{Name: "EPG 1", Type: models.EpgSourceTypeXMLTV, URL: "http://example.com/epg.xml"}
	require.NoError(t, db.Create(epgSource).Error)

	proxy := &models.StreamProxy{
		Name:                  "Multi-Source Proxy",
		ProxyMode:             models.StreamProxyModeSmart,
		StartingChannelNumber: 100,
	}
	require.NoError(t, db.Create(proxy).Error)

	// Create associations
	require.NoError(t, db.Create(&models.ProxySource{ProxyID: proxy.ID, SourceID: source1.ID, Priority: 1}).Error)
	require.NoError(t, db.Create(&models.ProxySource{ProxyID: proxy.ID, SourceID: source2.ID, Priority: 2}).Error)
	require.NoError(t, db.Create(&models.ProxyEpgSource{ProxyID: proxy.ID, EpgSourceID: epgSource.ID, Priority: 1}).Error)

	// Load proxy with relationships
	var loadedProxy models.StreamProxy
	err = db.Preload("Sources").Preload("EpgSources").First(&loadedProxy, proxy.ID).Error
	require.NoError(t, err)

	assert.Len(t, loadedProxy.Sources, 2)
	assert.Len(t, loadedProxy.EpgSources, 1)
}
