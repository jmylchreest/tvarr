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

func setupFilterTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.Filter{})
	require.NoError(t, err)

	return db
}

func TestFilterRepo_Create_Include(t *testing.T) {
	db := setupFilterTestDB(t)
	repo := NewFilterRepository(db)
	ctx := context.Background()

	filter := &models.Filter{
		Name:       "Include Sports",
		SourceType: models.FilterSourceTypeStream,
		Action:     models.FilterActionInclude,
		Expression: `group_title == "Sports"`,
	}

	err := repo.Create(ctx, filter)
	require.NoError(t, err)
	assert.False(t, filter.ID.IsZero())

	// Verify retrieval
	found, err := repo.GetByID(ctx, filter.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "Include Sports", found.Name)
	assert.Equal(t, models.FilterActionInclude, found.Action)
	assert.Equal(t, models.FilterSourceTypeStream, found.SourceType)
}

func TestFilterRepo_Create_Exclude(t *testing.T) {
	db := setupFilterTestDB(t)
	repo := NewFilterRepository(db)
	ctx := context.Background()

	filter := &models.Filter{
		Name:       "Exclude Adult",
		SourceType: models.FilterSourceTypeStream,
		Action:     models.FilterActionExclude,
		Expression: `is_adult == true`,
	}

	err := repo.Create(ctx, filter)
	require.NoError(t, err)

	found, err := repo.GetByID(ctx, filter.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, models.FilterActionExclude, found.Action)
}

func TestFilterRepo_Create_Validation(t *testing.T) {
	db := setupFilterTestDB(t)
	repo := NewFilterRepository(db)
	ctx := context.Background()

	tests := []struct {
		name    string
		filter  *models.Filter
		wantErr string
	}{
		{
			name: "missing name",
			filter: &models.Filter{
				SourceType: models.FilterSourceTypeStream,
				Action:     models.FilterActionInclude,
				Expression: `group_title == "Sports"`,
			},
			wantErr: "name",
		},
		{
			name: "missing expression",
			filter: &models.Filter{
				Name:       "Bad Filter",
				SourceType: models.FilterSourceTypeStream,
				Action:     models.FilterActionInclude,
			},
			wantErr: "expression",
		},
		{
			name: "missing source_type",
			filter: &models.Filter{
				Name:       "Bad Filter",
				Action:     models.FilterActionInclude,
				Expression: `group_title == "Sports"`,
			},
			wantErr: "source_type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Create(ctx, tt.filter)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestFilterRepo_GetByID(t *testing.T) {
	db := setupFilterTestDB(t)
	repo := NewFilterRepository(db)
	ctx := context.Background()

	filter := &models.Filter{
		Name:       "Find Me",
		SourceType: models.FilterSourceTypeEPG,
		Action:     models.FilterActionInclude,
		Expression: `category == "Movie"`,
	}
	require.NoError(t, repo.Create(ctx, filter))

	t.Run("found", func(t *testing.T) {
		found, err := repo.GetByID(ctx, filter.ID)
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, "Find Me", found.Name)
	})

	t.Run("not found", func(t *testing.T) {
		found, err := repo.GetByID(ctx, models.NewULID())
		require.NoError(t, err)
		assert.Nil(t, found)
	})
}

func TestFilterRepo_GetAll(t *testing.T) {
	db := setupFilterTestDB(t)
	repo := NewFilterRepository(db)
	ctx := context.Background()

	// Create multiple filters
	names := []string{"Filter A", "Filter B", "Filter C"}
	for _, name := range names {
		f := &models.Filter{
			Name:       name,
			SourceType: models.FilterSourceTypeStream,
			Action:     models.FilterActionInclude,
			Expression: `channel_name contains "` + name + `"`,
		}
		require.NoError(t, repo.Create(ctx, f))
	}

	filters, err := repo.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, filters, 3)
}

func TestFilterRepo_Update(t *testing.T) {
	db := setupFilterTestDB(t)
	repo := NewFilterRepository(db)
	ctx := context.Background()

	filter := &models.Filter{
		Name:        "Original",
		Description: "Original description",
		SourceType:  models.FilterSourceTypeStream,
		Action:      models.FilterActionInclude,
		Expression:  `group_title == "Sports"`,
	}
	require.NoError(t, repo.Create(ctx, filter))

	// Update
	filter.Name = "Updated"
	filter.Description = "Updated description"
	filter.Action = models.FilterActionExclude
	filter.Expression = `group_title == "Adult"`

	err := repo.Update(ctx, filter)
	require.NoError(t, err)

	// Verify
	found, err := repo.GetByID(ctx, filter.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "Updated", found.Name)
	assert.Equal(t, "Updated description", found.Description)
	assert.Equal(t, models.FilterActionExclude, found.Action)
	assert.Equal(t, `group_title == "Adult"`, found.Expression)
}

func TestFilterRepo_Delete(t *testing.T) {
	db := setupFilterTestDB(t)
	repo := NewFilterRepository(db)
	ctx := context.Background()

	filter := &models.Filter{
		Name:       "To Delete",
		SourceType: models.FilterSourceTypeStream,
		Action:     models.FilterActionInclude,
		Expression: `channel_name == "Test"`,
	}
	require.NoError(t, repo.Create(ctx, filter))

	err := repo.Delete(ctx, filter.ID)
	require.NoError(t, err)

	found, err := repo.GetByID(ctx, filter.ID)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestFilterRepo_GetBySourceType(t *testing.T) {
	db := setupFilterTestDB(t)
	repo := NewFilterRepository(db)
	ctx := context.Background()

	// Create stream filters
	for _, name := range []string{"Stream 1", "Stream 2"} {
		f := &models.Filter{
			Name:       name,
			SourceType: models.FilterSourceTypeStream,
			Action:     models.FilterActionInclude,
			Expression: `channel_name contains "test"`,
		}
		require.NoError(t, repo.Create(ctx, f))
	}

	// Create EPG filter
	epgFilter := &models.Filter{
		Name:       "EPG Filter",
		SourceType: models.FilterSourceTypeEPG,
		Action:     models.FilterActionExclude,
		Expression: `category == "Adult"`,
	}
	require.NoError(t, repo.Create(ctx, epgFilter))

	t.Run("stream filters", func(t *testing.T) {
		filters, err := repo.GetBySourceType(ctx, models.FilterSourceTypeStream)
		require.NoError(t, err)
		assert.Len(t, filters, 2)
		for _, f := range filters {
			assert.Equal(t, models.FilterSourceTypeStream, f.SourceType)
		}
	})

	t.Run("epg filters", func(t *testing.T) {
		filters, err := repo.GetBySourceType(ctx, models.FilterSourceTypeEPG)
		require.NoError(t, err)
		assert.Len(t, filters, 1)
		assert.Equal(t, "EPG Filter", filters[0].Name)
	})
}

func TestFilterRepo_Count(t *testing.T) {
	db := setupFilterTestDB(t)
	repo := NewFilterRepository(db)
	ctx := context.Background()

	count, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	for i := range 3 {
		f := &models.Filter{
			Name:       "Filter " + string(rune('A'+i)),
			SourceType: models.FilterSourceTypeStream,
			Action:     models.FilterActionInclude,
			Expression: `channel_name contains "test"`,
		}
		require.NoError(t, repo.Create(ctx, f))
	}

	count, err = repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

func TestFilterRepo_GetByName(t *testing.T) {
	db := setupFilterTestDB(t)
	repo := NewFilterRepository(db)
	ctx := context.Background()

	filter := &models.Filter{
		Name:       "Named Filter",
		SourceType: models.FilterSourceTypeStream,
		Action:     models.FilterActionInclude,
		Expression: `channel_name == "test"`,
	}
	require.NoError(t, repo.Create(ctx, filter))

	t.Run("found", func(t *testing.T) {
		found, err := repo.GetByName(ctx, "Named Filter")
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, filter.ID, found.ID)
	})

	t.Run("not found", func(t *testing.T) {
		found, err := repo.GetByName(ctx, "Nonexistent")
		require.NoError(t, err)
		assert.Nil(t, found)
	})
}

func TestFilterRepo_GetUserCreated(t *testing.T) {
	db := setupFilterTestDB(t)
	repo := NewFilterRepository(db)
	ctx := context.Background()

	// Create user filter
	userFilter := &models.Filter{
		Name:       "User Filter",
		SourceType: models.FilterSourceTypeStream,
		Action:     models.FilterActionInclude,
		Expression: `channel_name contains "test"`,
		IsSystem:   false,
	}
	require.NoError(t, repo.Create(ctx, userFilter))

	// Create system filter
	sysFilter := &models.Filter{
		Name:       "System Filter",
		SourceType: models.FilterSourceTypeStream,
		Action:     models.FilterActionExclude,
		Expression: `is_adult == true`,
		IsSystem:   true,
	}
	require.NoError(t, repo.Create(ctx, sysFilter))

	filters, err := repo.GetUserCreated(ctx)
	require.NoError(t, err)
	assert.Len(t, filters, 1)
	assert.Equal(t, "User Filter", filters[0].Name)
	assert.False(t, filters[0].IsSystem)
}

func TestFilterRepo_GetBySourceID(t *testing.T) {
	db := setupFilterTestDB(t)
	repo := NewFilterRepository(db)
	ctx := context.Background()

	sourceID := models.NewULID()

	// Create global filter (nil source_id)
	globalFilter := &models.Filter{
		Name:       "Global Filter",
		SourceType: models.FilterSourceTypeStream,
		Action:     models.FilterActionInclude,
		Expression: `channel_name contains "test"`,
	}
	require.NoError(t, repo.Create(ctx, globalFilter))

	// Create source-specific filter
	sourceFilter := &models.Filter{
		Name:       "Source Filter",
		SourceType: models.FilterSourceTypeStream,
		Action:     models.FilterActionExclude,
		Expression: `is_adult == true`,
		SourceID:   &sourceID,
	}
	require.NoError(t, repo.Create(ctx, sourceFilter))

	t.Run("by source ID returns source-specific and global", func(t *testing.T) {
		filters, err := repo.GetBySourceID(ctx, &sourceID)
		require.NoError(t, err)
		assert.Len(t, filters, 2)
	})

	t.Run("nil source ID returns only global", func(t *testing.T) {
		filters, err := repo.GetBySourceID(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, filters, 1)
		assert.Equal(t, "Global Filter", filters[0].Name)
	})
}
