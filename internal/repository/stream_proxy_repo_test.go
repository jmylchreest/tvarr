package repository

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

func setupProxyTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Migrate all required tables
	err = db.AutoMigrate(
		&models.StreamSource{},
		&models.EpgSource{},
		&models.StreamProxy{},
		&models.ProxySource{},
		&models.ProxyEpgSource{},
		&models.Filter{},
		&models.ProxyFilter{},
		&models.DataMappingRule{},
		&models.ProxyMappingRule{},
	)
	require.NoError(t, err)

	return db
}

func createTestStreamSource(t *testing.T, db *gorm.DB, name string) *models.StreamSource {
	t.Helper()
	source := &models.StreamSource{
		Name: name,
		Type: models.SourceTypeM3U,
		URL:  "http://example.com/" + name + ".m3u",
	}
	require.NoError(t, db.Create(source).Error)
	return source
}

func createTestEpgSource(t *testing.T, db *gorm.DB, name string) *models.EpgSource {
	t.Helper()
	source := &models.EpgSource{
		Name: name,
		Type: models.EpgSourceTypeXMLTV,
		URL:  "http://example.com/" + name + ".xml",
	}
	require.NoError(t, db.Create(source).Error)
	return source
}

func TestStreamProxyRepo_Create(t *testing.T) {
	db := setupProxyTestDB(t)
	repo := NewStreamProxyRepository(db)
	ctx := context.Background()

	proxy := &models.StreamProxy{
		Name:                  "Test Proxy",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
	}

	err := repo.Create(ctx, proxy)
	require.NoError(t, err)
	assert.False(t, proxy.ID.IsZero())

	// Verify it was created
	found, err := repo.GetByID(ctx, proxy.ID)
	require.NoError(t, err)
	assert.Equal(t, "Test Proxy", found.Name)
}

func TestStreamProxyRepo_Create_DuplicateName(t *testing.T) {
	db := setupProxyTestDB(t)
	repo := NewStreamProxyRepository(db)
	ctx := context.Background()

	proxy1 := &models.StreamProxy{
		Name:                  "Duplicate",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
	}
	require.NoError(t, repo.Create(ctx, proxy1))

	proxy2 := &models.StreamProxy{
		Name:                  "Duplicate",
		ProxyMode:             models.StreamProxyModeSmart,
		StartingChannelNumber: 1,
	}
	err := repo.Create(ctx, proxy2)
	assert.Error(t, err)
}

func TestStreamProxyRepo_GetByID(t *testing.T) {
	db := setupProxyTestDB(t)
	repo := NewStreamProxyRepository(db)
	ctx := context.Background()

	proxy := &models.StreamProxy{
		Name:                  "Find Me",
		ProxyMode:             models.StreamProxyModeSmart,
		StartingChannelNumber: 100,
		Description:           "A test proxy",
	}
	require.NoError(t, repo.Create(ctx, proxy))

	found, err := repo.GetByID(ctx, proxy.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "Find Me", found.Name)
	assert.Equal(t, models.StreamProxyModeSmart, found.ProxyMode)
	assert.Equal(t, 100, found.StartingChannelNumber)
}

func TestStreamProxyRepo_GetByID_NotFound(t *testing.T) {
	db := setupProxyTestDB(t)
	repo := NewStreamProxyRepository(db)
	ctx := context.Background()

	nonExistentID := models.NewULID()
	found, err := repo.GetByID(ctx, nonExistentID)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestStreamProxyRepo_GetByIDWithRelations(t *testing.T) {
	db := setupProxyTestDB(t)
	repo := NewStreamProxyRepository(db)
	ctx := context.Background()

	// Create sources
	streamSource1 := createTestStreamSource(t, db, "source1")
	streamSource2 := createTestStreamSource(t, db, "source2")
	epgSource := createTestEpgSource(t, db, "epg1")

	// Create proxy
	proxy := &models.StreamProxy{
		Name:                  "With Relations",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
	}
	require.NoError(t, repo.Create(ctx, proxy))

	// Set sources
	err := repo.SetSources(ctx, proxy.ID, []models.ULID{streamSource1.ID, streamSource2.ID}, nil)
	require.NoError(t, err)
	err = repo.SetEpgSources(ctx, proxy.ID, []models.ULID{epgSource.ID}, nil)
	require.NoError(t, err)

	// Get with relations
	found, err := repo.GetByIDWithRelations(ctx, proxy.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Len(t, found.Sources, 2)
	assert.Len(t, found.EpgSources, 1)
}

func TestStreamProxyRepo_GetAll(t *testing.T) {
	db := setupProxyTestDB(t)
	repo := NewStreamProxyRepository(db)
	ctx := context.Background()

	// Create multiple proxies
	for _, name := range []string{"Zebra", "Alpha", "Beta"} {
		proxy := &models.StreamProxy{
			Name:                  name,
			ProxyMode:             models.StreamProxyModeDirect,
			StartingChannelNumber: 1,
		}
		require.NoError(t, repo.Create(ctx, proxy))
	}

	proxies, err := repo.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, proxies, 3)

	// Should be ordered by name ASC
	assert.Equal(t, "Alpha", proxies[0].Name)
	assert.Equal(t, "Beta", proxies[1].Name)
	assert.Equal(t, "Zebra", proxies[2].Name)
}

func TestStreamProxyRepo_GetActive(t *testing.T) {
	db := setupProxyTestDB(t)
	repo := NewStreamProxyRepository(db)
	ctx := context.Background()

	// Create active proxy
	active := &models.StreamProxy{
		Name:                  "Active",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
		IsActive:              new(true),
	}
	require.NoError(t, repo.Create(ctx, active))

	// Create inactive proxy - need to update after create since GORM
	// ignores false values when there's a default:true tag
	inactive := &models.StreamProxy{
		Name:                  "Inactive",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
	}
	require.NoError(t, repo.Create(ctx, inactive))
	// Explicitly set to inactive
	db.Model(inactive).Update("is_active", false)

	proxies, err := repo.GetActive(ctx)
	require.NoError(t, err)
	assert.Len(t, proxies, 1)
	assert.Equal(t, "Active", proxies[0].Name)
}

func TestStreamProxyRepo_Update(t *testing.T) {
	db := setupProxyTestDB(t)
	repo := NewStreamProxyRepository(db)
	ctx := context.Background()

	proxy := &models.StreamProxy{
		Name:                  "Original",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
	}
	require.NoError(t, repo.Create(ctx, proxy))

	// Update
	proxy.Name = "Updated"
	proxy.ProxyMode = models.StreamProxyModeSmart
	proxy.StartingChannelNumber = 500

	err := repo.Update(ctx, proxy)
	require.NoError(t, err)

	// Verify
	found, err := repo.GetByID(ctx, proxy.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", found.Name)
	assert.Equal(t, models.StreamProxyModeSmart, found.ProxyMode)
	assert.Equal(t, 500, found.StartingChannelNumber)
}

func TestStreamProxyRepo_Delete(t *testing.T) {
	db := setupProxyTestDB(t)
	repo := NewStreamProxyRepository(db)
	ctx := context.Background()

	// Create proxy with associations
	streamSource := createTestStreamSource(t, db, "source")
	epgSource := createTestEpgSource(t, db, "epg")

	proxy := &models.StreamProxy{
		Name:                  "To Delete",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
	}
	require.NoError(t, repo.Create(ctx, proxy))

	err := repo.SetSources(ctx, proxy.ID, []models.ULID{streamSource.ID}, nil)
	require.NoError(t, err)
	err = repo.SetEpgSources(ctx, proxy.ID, []models.ULID{epgSource.ID}, nil)
	require.NoError(t, err)

	// Delete
	err = repo.Delete(ctx, proxy.ID)
	require.NoError(t, err)

	// Verify proxy is gone
	found, err := repo.GetByID(ctx, proxy.ID)
	require.NoError(t, err)
	assert.Nil(t, found)

	// Verify associations are gone
	var proxySourceCount int64
	db.Model(&models.ProxySource{}).Where("proxy_id = ?", proxy.ID).Count(&proxySourceCount)
	assert.Equal(t, int64(0), proxySourceCount)

	var proxyEpgCount int64
	db.Model(&models.ProxyEpgSource{}).Where("proxy_id = ?", proxy.ID).Count(&proxyEpgCount)
	assert.Equal(t, int64(0), proxyEpgCount)
}

func TestStreamProxyRepo_GetByName(t *testing.T) {
	db := setupProxyTestDB(t)
	repo := NewStreamProxyRepository(db)
	ctx := context.Background()

	proxy := &models.StreamProxy{
		Name:                  "Named Proxy",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
	}
	require.NoError(t, repo.Create(ctx, proxy))

	found, err := repo.GetByName(ctx, "Named Proxy")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, proxy.ID, found.ID)

	notFound, err := repo.GetByName(ctx, "Nonexistent")
	require.NoError(t, err)
	assert.Nil(t, notFound)
}

func TestStreamProxyRepo_UpdateStatus(t *testing.T) {
	db := setupProxyTestDB(t)
	repo := NewStreamProxyRepository(db)
	ctx := context.Background()

	proxy := &models.StreamProxy{
		Name:                  "Status Test",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
		Status:                models.StreamProxyStatusPending,
	}
	require.NoError(t, repo.Create(ctx, proxy))

	// Update to generating
	err := repo.UpdateStatus(ctx, proxy.ID, models.StreamProxyStatusGenerating, "")
	require.NoError(t, err)

	found, _ := repo.GetByID(ctx, proxy.ID)
	assert.Equal(t, models.StreamProxyStatusGenerating, found.Status)

	// Update to failed with error
	err = repo.UpdateStatus(ctx, proxy.ID, models.StreamProxyStatusFailed, "connection timeout")
	require.NoError(t, err)

	found, _ = repo.GetByID(ctx, proxy.ID)
	assert.Equal(t, models.StreamProxyStatusFailed, found.Status)
	assert.Equal(t, "connection timeout", found.LastError)
}

func TestStreamProxyRepo_UpdateLastGeneration(t *testing.T) {
	db := setupProxyTestDB(t)
	repo := NewStreamProxyRepository(db)
	ctx := context.Background()

	proxy := &models.StreamProxy{
		Name:                  "Generation Test",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
		Status:                models.StreamProxyStatusGenerating,
	}
	require.NoError(t, repo.Create(ctx, proxy))

	err := repo.UpdateLastGeneration(ctx, proxy.ID, 150, 5000)
	require.NoError(t, err)

	found, _ := repo.GetByID(ctx, proxy.ID)
	assert.Equal(t, models.StreamProxyStatusSuccess, found.Status)
	assert.Equal(t, 150, found.ChannelCount)
	assert.Equal(t, 5000, found.ProgramCount)
	assert.NotNil(t, found.LastGeneratedAt)
	assert.Empty(t, found.LastError)
}

func TestStreamProxyRepo_SetSources(t *testing.T) {
	db := setupProxyTestDB(t)
	repo := NewStreamProxyRepository(db)
	ctx := context.Background()

	source1 := createTestStreamSource(t, db, "source1")
	source2 := createTestStreamSource(t, db, "source2")
	source3 := createTestStreamSource(t, db, "source3")

	proxy := &models.StreamProxy{
		Name:                  "Sources Test",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
	}
	require.NoError(t, repo.Create(ctx, proxy))

	// Set initial sources with priorities
	priorities := map[models.ULID]int{
		source1.ID: 10,
		source2.ID: 20,
	}
	err := repo.SetSources(ctx, proxy.ID, []models.ULID{source1.ID, source2.ID}, priorities)
	require.NoError(t, err)

	sources, err := repo.GetSources(ctx, proxy.ID)
	require.NoError(t, err)
	assert.Len(t, sources, 2)

	// Replace with new set
	err = repo.SetSources(ctx, proxy.ID, []models.ULID{source3.ID}, nil)
	require.NoError(t, err)

	sources, err = repo.GetSources(ctx, proxy.ID)
	require.NoError(t, err)
	assert.Len(t, sources, 1)
	assert.Equal(t, source3.ID, sources[0].ID)
}

func TestStreamProxyRepo_SetEpgSources(t *testing.T) {
	db := setupProxyTestDB(t)
	repo := NewStreamProxyRepository(db)
	ctx := context.Background()

	epg1 := createTestEpgSource(t, db, "epg1")
	epg2 := createTestEpgSource(t, db, "epg2")

	proxy := &models.StreamProxy{
		Name:                  "EPG Sources Test",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
	}
	require.NoError(t, repo.Create(ctx, proxy))

	priorities := map[models.ULID]int{
		epg1.ID: 5,
		epg2.ID: 10,
	}
	err := repo.SetEpgSources(ctx, proxy.ID, []models.ULID{epg1.ID, epg2.ID}, priorities)
	require.NoError(t, err)

	sources, err := repo.GetEpgSources(ctx, proxy.ID)
	require.NoError(t, err)
	assert.Len(t, sources, 2)

	// Higher priority should come first
	assert.Equal(t, epg2.ID, sources[0].ID)
	assert.Equal(t, epg1.ID, sources[1].ID)
}

func TestStreamProxyRepo_GetSources_Priority(t *testing.T) {
	db := setupProxyTestDB(t)
	repo := NewStreamProxyRepository(db)
	ctx := context.Background()

	source1 := createTestStreamSource(t, db, "low_priority")
	source2 := createTestStreamSource(t, db, "high_priority")
	source3 := createTestStreamSource(t, db, "medium_priority")

	proxy := &models.StreamProxy{
		Name:                  "Priority Test",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
	}
	require.NoError(t, repo.Create(ctx, proxy))

	priorities := map[models.ULID]int{
		source1.ID: 1,
		source2.ID: 100,
		source3.ID: 50,
	}
	err := repo.SetSources(ctx, proxy.ID, []models.ULID{source1.ID, source2.ID, source3.ID}, priorities)
	require.NoError(t, err)

	sources, err := repo.GetSources(ctx, proxy.ID)
	require.NoError(t, err)
	require.Len(t, sources, 3)

	// Should be ordered by priority DESC
	assert.Equal(t, source2.ID, sources[0].ID) // priority 100
	assert.Equal(t, source3.ID, sources[1].ID) // priority 50
	assert.Equal(t, source1.ID, sources[2].ID) // priority 1
}
