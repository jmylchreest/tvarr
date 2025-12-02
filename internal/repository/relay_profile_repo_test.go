package repository

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

func setupRelayProfileTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.RelayProfile{})
	require.NoError(t, err)

	return db
}

func TestRelayProfileRepo_Create(t *testing.T) {
	db := setupRelayProfileTestDB(t)
	repo := NewRelayProfileRepository(db)
	ctx := context.Background()

	profile := &models.RelayProfile{
		Name:        "Test Profile",
		Description: "A test profile",
		VideoCodec:  models.VideoCodecCopy,
		AudioCodec:  models.AudioCodecCopy,
	}

	err := repo.Create(ctx, profile)
	require.NoError(t, err)
	assert.False(t, profile.ID.IsZero())

	// Verify it was created
	found, err := repo.GetByID(ctx, profile.ID)
	require.NoError(t, err)
	assert.Equal(t, "Test Profile", found.Name)
}

func TestRelayProfileRepo_Create_Validation(t *testing.T) {
	db := setupRelayProfileTestDB(t)
	repo := NewRelayProfileRepository(db)
	ctx := context.Background()

	// Missing name
	profile := &models.RelayProfile{
		VideoCodec: models.VideoCodecH264,
	}

	err := repo.Create(ctx, profile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validating relay profile")
}

func TestRelayProfileRepo_Create_DuplicateName(t *testing.T) {
	db := setupRelayProfileTestDB(t)
	repo := NewRelayProfileRepository(db)
	ctx := context.Background()

	profile1 := &models.RelayProfile{
		Name:       "Duplicate",
		VideoCodec: models.VideoCodecCopy,
	}
	require.NoError(t, repo.Create(ctx, profile1))

	profile2 := &models.RelayProfile{
		Name:       "Duplicate",
		VideoCodec: models.VideoCodecH264,
	}
	err := repo.Create(ctx, profile2)
	assert.Error(t, err)
}

func TestRelayProfileRepo_GetByID(t *testing.T) {
	db := setupRelayProfileTestDB(t)
	repo := NewRelayProfileRepository(db)
	ctx := context.Background()

	profile := &models.RelayProfile{
		Name:         "Find Me",
		Description:  "A test profile",
		VideoCodec:   models.VideoCodecH264,
		AudioCodec:   models.AudioCodecAAC,
		VideoBitrate: 5000,
	}
	require.NoError(t, repo.Create(ctx, profile))

	found, err := repo.GetByID(ctx, profile.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "Find Me", found.Name)
	assert.Equal(t, models.VideoCodecH264, found.VideoCodec)
	assert.Equal(t, 5000, found.VideoBitrate)
}

func TestRelayProfileRepo_GetByID_NotFound(t *testing.T) {
	db := setupRelayProfileTestDB(t)
	repo := NewRelayProfileRepository(db)
	ctx := context.Background()

	nonExistentID := models.NewULID()
	found, err := repo.GetByID(ctx, nonExistentID)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestRelayProfileRepo_GetAll(t *testing.T) {
	db := setupRelayProfileTestDB(t)
	repo := NewRelayProfileRepository(db)
	ctx := context.Background()

	// Create multiple profiles
	for _, name := range []string{"Zebra", "Alpha", "Beta"} {
		profile := &models.RelayProfile{
			Name:       name,
			VideoCodec: models.VideoCodecCopy,
		}
		require.NoError(t, repo.Create(ctx, profile))
	}

	profiles, err := repo.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, profiles, 3)

	// Should be ordered by name ASC (default first, but none are default)
	assert.Equal(t, "Alpha", profiles[0].Name)
	assert.Equal(t, "Beta", profiles[1].Name)
	assert.Equal(t, "Zebra", profiles[2].Name)
}

func TestRelayProfileRepo_GetByName(t *testing.T) {
	db := setupRelayProfileTestDB(t)
	repo := NewRelayProfileRepository(db)
	ctx := context.Background()

	profile := &models.RelayProfile{
		Name:       "Named Profile",
		VideoCodec: models.VideoCodecCopy,
	}
	require.NoError(t, repo.Create(ctx, profile))

	found, err := repo.GetByName(ctx, "Named Profile")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, profile.ID, found.ID)

	notFound, err := repo.GetByName(ctx, "Nonexistent")
	require.NoError(t, err)
	assert.Nil(t, notFound)
}

func TestRelayProfileRepo_GetDefault(t *testing.T) {
	db := setupRelayProfileTestDB(t)
	repo := NewRelayProfileRepository(db)
	ctx := context.Background()

	// Create non-default profile
	profile1 := &models.RelayProfile{
		Name:       "Not Default",
		VideoCodec: models.VideoCodecCopy,
	}
	require.NoError(t, repo.Create(ctx, profile1))

	// Create default profile
	profile2 := &models.RelayProfile{
		Name:       "Default",
		VideoCodec: models.VideoCodecH264,
		IsDefault:  true,
	}
	require.NoError(t, repo.Create(ctx, profile2))

	found, err := repo.GetDefault(ctx)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "Default", found.Name)
	assert.True(t, found.IsDefault)
}

func TestRelayProfileRepo_GetDefault_NotFound(t *testing.T) {
	db := setupRelayProfileTestDB(t)
	repo := NewRelayProfileRepository(db)
	ctx := context.Background()

	// Create non-default profile
	profile := &models.RelayProfile{
		Name:       "Not Default",
		VideoCodec: models.VideoCodecCopy,
	}
	require.NoError(t, repo.Create(ctx, profile))

	found, err := repo.GetDefault(ctx)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestRelayProfileRepo_Update(t *testing.T) {
	db := setupRelayProfileTestDB(t)
	repo := NewRelayProfileRepository(db)
	ctx := context.Background()

	profile := &models.RelayProfile{
		Name:       "Original",
		VideoCodec: models.VideoCodecCopy,
	}
	require.NoError(t, repo.Create(ctx, profile))

	// Update
	profile.Name = "Updated"
	profile.VideoCodec = models.VideoCodecH265
	profile.VideoBitrate = 8000

	err := repo.Update(ctx, profile)
	require.NoError(t, err)

	// Verify
	found, err := repo.GetByID(ctx, profile.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", found.Name)
	assert.Equal(t, models.VideoCodecH265, found.VideoCodec)
	assert.Equal(t, 8000, found.VideoBitrate)
}

func TestRelayProfileRepo_Delete(t *testing.T) {
	db := setupRelayProfileTestDB(t)
	repo := NewRelayProfileRepository(db)
	ctx := context.Background()

	profile := &models.RelayProfile{
		Name:       "To Delete",
		VideoCodec: models.VideoCodecCopy,
	}
	require.NoError(t, repo.Create(ctx, profile))

	err := repo.Delete(ctx, profile.ID)
	require.NoError(t, err)

	// Verify it's gone
	found, err := repo.GetByID(ctx, profile.ID)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestRelayProfileRepo_Count(t *testing.T) {
	db := setupRelayProfileTestDB(t)
	repo := NewRelayProfileRepository(db)
	ctx := context.Background()

	count, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Create profiles
	for i := 0; i < 5; i++ {
		profile := &models.RelayProfile{
			Name:       "Profile " + string(rune('A'+i)),
			VideoCodec: models.VideoCodecCopy,
		}
		require.NoError(t, repo.Create(ctx, profile))
	}

	count, err = repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(5), count)
}

func TestRelayProfileRepo_SetDefault(t *testing.T) {
	db := setupRelayProfileTestDB(t)
	repo := NewRelayProfileRepository(db)
	ctx := context.Background()

	// Create profiles
	profile1 := &models.RelayProfile{
		Name:       "Profile 1",
		VideoCodec: models.VideoCodecCopy,
		IsDefault:  true,
	}
	require.NoError(t, repo.Create(ctx, profile1))

	profile2 := &models.RelayProfile{
		Name:       "Profile 2",
		VideoCodec: models.VideoCodecH264,
	}
	require.NoError(t, repo.Create(ctx, profile2))

	// Set profile2 as default
	err := repo.SetDefault(ctx, profile2.ID)
	require.NoError(t, err)

	// Verify profile2 is now default
	found, err := repo.GetByID(ctx, profile2.ID)
	require.NoError(t, err)
	assert.True(t, found.IsDefault)

	// Verify profile1 is no longer default
	found, err = repo.GetByID(ctx, profile1.ID)
	require.NoError(t, err)
	assert.False(t, found.IsDefault)

	// Verify GetDefault returns profile2
	defaultProfile, err := repo.GetDefault(ctx)
	require.NoError(t, err)
	assert.Equal(t, profile2.ID, defaultProfile.ID)
}

func TestRelayProfileRepo_SetDefault_NotFound(t *testing.T) {
	db := setupRelayProfileTestDB(t)
	repo := NewRelayProfileRepository(db)
	ctx := context.Background()

	nonExistentID := models.NewULID()
	err := repo.SetDefault(ctx, nonExistentID)
	assert.Error(t, err)
}

func TestRelayProfileRepo_GetAll_DefaultFirst(t *testing.T) {
	db := setupRelayProfileTestDB(t)
	repo := NewRelayProfileRepository(db)
	ctx := context.Background()

	// Create non-default profiles first
	profile1 := &models.RelayProfile{
		Name:       "Alpha",
		VideoCodec: models.VideoCodecCopy,
	}
	require.NoError(t, repo.Create(ctx, profile1))

	profile2 := &models.RelayProfile{
		Name:       "Zebra",
		VideoCodec: models.VideoCodecH264,
	}
	require.NoError(t, repo.Create(ctx, profile2))

	// Create default profile
	defaultProfile := &models.RelayProfile{
		Name:       "Middle",
		VideoCodec: models.VideoCodecH265,
		IsDefault:  true,
	}
	require.NoError(t, repo.Create(ctx, defaultProfile))

	profiles, err := repo.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, profiles, 3)

	// Default should come first, then sorted by name
	assert.Equal(t, "Middle", profiles[0].Name)
	assert.True(t, profiles[0].IsDefault)
	assert.Equal(t, "Alpha", profiles[1].Name)
	assert.Equal(t, "Zebra", profiles[2].Name)
}
