package migrations

import (
	"context"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
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

	// We have 18 migrations: 3 for stream sources, 2 for EPG, 5 for proxy, 3 for filters/rules,
	// 2 for relay, 1 for is_system column, 1 for EPG timezone fields, 1 for channel index fix
	// (Logo caching uses file-based storage with in-memory indexing, no database tables)
	assert.Len(t, migrations, 18)
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

	// Before running migrations
	statuses, err := migrator.Status(ctx)
	require.NoError(t, err)
	assert.Len(t, statuses, 18)

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

	// Roll back migrations one by one until we've removed data_mapping_rules and filters tables
	// Current order: 018, 017, 016, 015, 014, 013, 012, 011...
	// Need to roll back through 018 (channel index fix), 017 (EPG timezone), 016 (is_system),
	// 015 (default relay profiles), 014 (relay_profiles table), 013 (default filters/rules),
	// 012 (data_mapping_rules), 011 (filters)

	// Roll back 018-013 (channel index, EPG timezone, is_system, relay profiles, default data)
	for i := 0; i < 6; i++ {
		err = migrator.Down(ctx)
		require.NoError(t, err)
	}

	// Tables still exist after rolling back to 012
	assert.True(t, db.Migrator().HasTable("filters"))
	assert.True(t, db.Migrator().HasTable("data_mapping_rules"))

	// Roll back 012 (data_mapping_rules table)
	err = migrator.Down(ctx)
	require.NoError(t, err)

	assert.False(t, db.Migrator().HasTable("data_mapping_rules"))
	assert.True(t, db.Migrator().HasTable("filters")) // Still exists

	// Roll back 011 (filters table)
	err = migrator.Down(ctx)
	require.NoError(t, err)

	assert.False(t, db.Migrator().HasTable("filters"))
}

func TestMigrator_Pending(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	migrator := NewMigrator(db, nil)
	migrator.RegisterAll(AllMigrations())

	// All should be pending initially
	pending, err := migrator.Pending(ctx)
	require.NoError(t, err)
	assert.Len(t, pending, 18)

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
		ProxyMode:             models.StreamProxyModeRedirect,
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
		ProxyMode:             models.StreamProxyModeProxy,
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
