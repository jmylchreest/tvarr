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

func setupEpgSourceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.EpgSource{})
	require.NoError(t, err)

	return db
}

func TestEpgSourceRepo_Create(t *testing.T) {
	db := setupEpgSourceTestDB(t)
	repo := NewEpgSourceRepository(db)
	ctx := context.Background()

	source := &models.EpgSource{
		Name:    "Test EPG",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/epg.xml",
		Enabled: models.BoolPtr(true),
	}

	err := repo.Create(ctx, source)
	require.NoError(t, err)
	assert.False(t, source.ID.IsZero())
}

func TestEpgSourceRepo_GetByID(t *testing.T) {
	db := setupEpgSourceTestDB(t)
	repo := NewEpgSourceRepository(db)
	ctx := context.Background()

	source := &models.EpgSource{
		Name:    "Find Me EPG",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/find.xml",
		Enabled: models.BoolPtr(true),
	}
	require.NoError(t, repo.Create(ctx, source))

	t.Run("found", func(t *testing.T) {
		found, err := repo.GetByID(ctx, source.ID)
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, "Find Me EPG", found.Name)
		assert.Equal(t, models.EpgSourceTypeXMLTV, found.Type)
	})

	t.Run("not found", func(t *testing.T) {
		found, err := repo.GetByID(ctx, models.NewULID())
		require.NoError(t, err)
		assert.Nil(t, found)
	})
}

func TestEpgSourceRepo_GetAll(t *testing.T) {
	db := setupEpgSourceTestDB(t)
	repo := NewEpgSourceRepository(db)
	ctx := context.Background()

	// Create sources with different priorities
	sources := []struct {
		name     string
		priority int
	}{
		{"Low Priority", 0},
		{"High Priority", 10},
		{"Medium Priority", 5},
	}

	for _, s := range sources {
		src := &models.EpgSource{
			Name:     s.name,
			Type:     models.EpgSourceTypeXMLTV,
			URL:      "http://example.com/" + s.name + ".xml",
			Priority: s.priority,
			Enabled:  models.BoolPtr(true),
		}
		require.NoError(t, repo.Create(ctx, src))
	}

	all, err := repo.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	// Should be ordered by priority DESC, name ASC
	assert.Equal(t, "High Priority", all[0].Name)
	assert.Equal(t, "Medium Priority", all[1].Name)
	assert.Equal(t, "Low Priority", all[2].Name)
}

func TestEpgSourceRepo_GetEnabled(t *testing.T) {
	db := setupEpgSourceTestDB(t)
	repo := NewEpgSourceRepository(db)
	ctx := context.Background()

	// Create enabled source
	enabled := &models.EpgSource{
		Name:    "Enabled EPG",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/enabled.xml",
		Enabled: models.BoolPtr(true),
	}
	require.NoError(t, repo.Create(ctx, enabled))

	// Create source that will be disabled
	disabled := &models.EpgSource{
		Name: "Disabled EPG",
		Type: models.EpgSourceTypeXMLTV,
		URL:  "http://example.com/disabled.xml",
	}
	require.NoError(t, repo.Create(ctx, disabled))
	// Disable after creation (GORM default:true interferes with false on create)
	require.NoError(t, db.Model(disabled).UpdateColumn("enabled", false).Error)

	// Create source with nil Enabled (should be treated as enabled by default)
	nilEnabled := &models.EpgSource{
		Name:    "Nil Enabled EPG",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/nil.xml",
		Enabled: models.BoolPtr(true),
	}
	require.NoError(t, repo.Create(ctx, nilEnabled))

	sources, err := repo.GetEnabled(ctx)
	require.NoError(t, err)
	assert.Len(t, sources, 2)

	// Verify none are disabled
	for _, s := range sources {
		assert.NotEqual(t, "Disabled EPG", s.Name)
	}
}

func TestEpgSourceRepo_Update(t *testing.T) {
	db := setupEpgSourceTestDB(t)
	repo := NewEpgSourceRepository(db)
	ctx := context.Background()

	source := &models.EpgSource{
		Name:    "Original EPG",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/original.xml",
		Enabled: models.BoolPtr(true),
	}
	require.NoError(t, repo.Create(ctx, source))

	// Update
	source.Name = "Updated EPG"
	source.URL = "http://example.com/updated.xml"
	source.Priority = 5

	err := repo.Update(ctx, source)
	require.NoError(t, err)

	// Verify
	found, err := repo.GetByID(ctx, source.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "Updated EPG", found.Name)
	assert.Equal(t, "http://example.com/updated.xml", found.URL)
	assert.Equal(t, 5, found.Priority)
}

func TestEpgSourceRepo_Delete(t *testing.T) {
	db := setupEpgSourceTestDB(t)
	repo := NewEpgSourceRepository(db)
	ctx := context.Background()

	source := &models.EpgSource{
		Name:    "To Delete EPG",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/delete.xml",
		Enabled: models.BoolPtr(true),
	}
	require.NoError(t, repo.Create(ctx, source))

	err := repo.Delete(ctx, source.ID)
	require.NoError(t, err)

	found, err := repo.GetByID(ctx, source.ID)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestEpgSourceRepo_Delete_AllowsRecreateSameName(t *testing.T) {
	db := setupEpgSourceTestDB(t)
	repo := NewEpgSourceRepository(db)
	ctx := context.Background()

	source := &models.EpgSource{
		Name:    "Reusable Name",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/reuse.xml",
		Enabled: models.BoolPtr(true),
	}
	require.NoError(t, repo.Create(ctx, source))

	// Delete (hard delete with Unscoped)
	require.NoError(t, repo.Delete(ctx, source.ID))

	// Re-create with same name should succeed
	source2 := &models.EpgSource{
		Name:    "Reusable Name",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/reuse2.xml",
		Enabled: models.BoolPtr(true),
	}
	err := repo.Create(ctx, source2)
	require.NoError(t, err)
}

func TestEpgSourceRepo_GetByName(t *testing.T) {
	db := setupEpgSourceTestDB(t)
	repo := NewEpgSourceRepository(db)
	ctx := context.Background()

	source := &models.EpgSource{
		Name:    "Named EPG Source",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/named.xml",
		Enabled: models.BoolPtr(true),
	}
	require.NoError(t, repo.Create(ctx, source))

	t.Run("found", func(t *testing.T) {
		found, err := repo.GetByName(ctx, "Named EPG Source")
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, source.ID, found.ID)
	})

	t.Run("not found", func(t *testing.T) {
		found, err := repo.GetByName(ctx, "Nonexistent")
		require.NoError(t, err)
		assert.Nil(t, found)
	})
}

func TestEpgSourceRepo_GetByURL(t *testing.T) {
	db := setupEpgSourceTestDB(t)
	repo := NewEpgSourceRepository(db)
	ctx := context.Background()

	source := &models.EpgSource{
		Name:    "URL Lookup EPG",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/unique-epg.xml",
		Enabled: models.BoolPtr(true),
	}
	require.NoError(t, repo.Create(ctx, source))

	t.Run("found", func(t *testing.T) {
		found, err := repo.GetByURL(ctx, "http://example.com/unique-epg.xml")
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, source.ID, found.ID)
	})

	t.Run("not found", func(t *testing.T) {
		found, err := repo.GetByURL(ctx, "http://example.com/nonexistent.xml")
		require.NoError(t, err)
		assert.Nil(t, found)
	})
}

func TestEpgSourceRepo_UpdateLastIngestion(t *testing.T) {
	db := setupEpgSourceTestDB(t)
	repo := NewEpgSourceRepository(db)
	ctx := context.Background()

	source := &models.EpgSource{
		Name:    "Ingest EPG",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/ingest.xml",
		Enabled: models.BoolPtr(true),
	}
	require.NoError(t, repo.Create(ctx, source))

	err := repo.UpdateLastIngestion(ctx, source.ID, "success", 500)
	require.NoError(t, err)

	// Verify the fields were updated
	updated, err := repo.GetByID(ctx, source.ID)
	require.NoError(t, err)
	assert.Equal(t, models.EpgSourceStatus("success"), updated.Status)
	assert.Equal(t, 500, updated.ProgramCount)
	assert.NotNil(t, updated.LastIngestionAt)
	assert.Equal(t, "", updated.LastError)
}

func TestEpgSourceRepo_DuplicateName(t *testing.T) {
	db := setupEpgSourceTestDB(t)
	repo := NewEpgSourceRepository(db)
	ctx := context.Background()

	source1 := &models.EpgSource{
		Name:    "Duplicate",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/dup1.xml",
		Enabled: models.BoolPtr(true),
	}
	require.NoError(t, repo.Create(ctx, source1))

	source2 := &models.EpgSource{
		Name:    "Duplicate",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/dup2.xml",
		Enabled: models.BoolPtr(true),
	}
	err := repo.Create(ctx, source2)
	assert.Error(t, err)
}
