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

func setupEncodingProfileTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.EncodingProfile{})
	require.NoError(t, err)

	return db
}

func TestEncodingProfileRepo_Create(t *testing.T) {
	db := setupEncodingProfileTestDB(t)
	repo := NewEncodingProfileRepository(db)
	ctx := context.Background()

	profile := &models.EncodingProfile{
		Name:             "Test Profile",
		Description:      "A test profile",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
		HWAccel:          models.HWAccelAuto,
	}

	err := repo.Create(ctx, profile)
	require.NoError(t, err)
	assert.False(t, profile.ID.IsZero())

	// Verify it was created
	found, err := repo.GetByID(ctx, profile.ID)
	require.NoError(t, err)
	assert.Equal(t, "Test Profile", found.Name)
}

func TestEncodingProfileRepo_Create_Validation(t *testing.T) {
	db := setupEncodingProfileTestDB(t)
	repo := NewEncodingProfileRepository(db)
	ctx := context.Background()

	// Missing name
	profile := &models.EncodingProfile{
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
	}

	err := repo.Create(ctx, profile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validating encoding profile")
}

func TestEncodingProfileRepo_Create_DuplicateName(t *testing.T) {
	db := setupEncodingProfileTestDB(t)
	repo := NewEncodingProfileRepository(db)
	ctx := context.Background()

	profile1 := &models.EncodingProfile{
		Name:             "Duplicate",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
	}
	require.NoError(t, repo.Create(ctx, profile1))

	profile2 := &models.EncodingProfile{
		Name:             "Duplicate",
		TargetVideoCodec: models.VideoCodecH265,
		TargetAudioCodec: models.AudioCodecOpus,
		QualityPreset:    models.QualityPresetHigh,
	}
	err := repo.Create(ctx, profile2)
	assert.Error(t, err)
}

func TestEncodingProfileRepo_GetByID(t *testing.T) {
	db := setupEncodingProfileTestDB(t)
	repo := NewEncodingProfileRepository(db)
	ctx := context.Background()

	profile := &models.EncodingProfile{
		Name:             "Find Me",
		Description:      "A test profile",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetHigh,
		HWAccel:          models.HWAccelNone,
	}
	require.NoError(t, repo.Create(ctx, profile))

	found, err := repo.GetByID(ctx, profile.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "Find Me", found.Name)
	assert.Equal(t, models.VideoCodecH264, found.TargetVideoCodec)
	assert.Equal(t, models.QualityPresetHigh, found.QualityPreset)
}

func TestEncodingProfileRepo_GetByID_NotFound(t *testing.T) {
	db := setupEncodingProfileTestDB(t)
	repo := NewEncodingProfileRepository(db)
	ctx := context.Background()

	nonExistentID := models.NewULID()
	found, err := repo.GetByID(ctx, nonExistentID)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestEncodingProfileRepo_GetAll(t *testing.T) {
	db := setupEncodingProfileTestDB(t)
	repo := NewEncodingProfileRepository(db)
	ctx := context.Background()

	// Create multiple profiles
	for _, name := range []string{"Zebra", "Alpha", "Beta"} {
		profile := &models.EncodingProfile{
			Name:             name,
			TargetVideoCodec: models.VideoCodecH264,
			TargetAudioCodec: models.AudioCodecAAC,
			QualityPreset:    models.QualityPresetMedium,
		}
		require.NoError(t, repo.Create(ctx, profile))
	}

	profiles, err := repo.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, profiles, 3)

	// Should be ordered by name ASC (default first, system next, then by name)
	assert.Equal(t, "Alpha", profiles[0].Name)
	assert.Equal(t, "Beta", profiles[1].Name)
	assert.Equal(t, "Zebra", profiles[2].Name)
}

func TestEncodingProfileRepo_GetEnabled(t *testing.T) {
	db := setupEncodingProfileTestDB(t)
	repo := NewEncodingProfileRepository(db)
	ctx := context.Background()

	// Create enabled profile
	enabled := &models.EncodingProfile{
		Name:             "Enabled",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
		Enabled:          true,
	}
	require.NoError(t, repo.Create(ctx, enabled))

	// Create profile that will be disabled
	disabled := &models.EncodingProfile{
		Name:             "Disabled",
		TargetVideoCodec: models.VideoCodecH265,
		TargetAudioCodec: models.AudioCodecOpus,
		QualityPreset:    models.QualityPresetHigh,
	}
	require.NoError(t, repo.Create(ctx, disabled))
	// Disable it after creation (GORM default:true interferes with false on create)
	require.NoError(t, db.Model(disabled).UpdateColumn("enabled", false).Error)

	profiles, err := repo.GetEnabled(ctx)
	require.NoError(t, err)
	assert.Len(t, profiles, 1)
	assert.Equal(t, "Enabled", profiles[0].Name)
}

func TestEncodingProfileRepo_GetByName(t *testing.T) {
	db := setupEncodingProfileTestDB(t)
	repo := NewEncodingProfileRepository(db)
	ctx := context.Background()

	profile := &models.EncodingProfile{
		Name:             "Named Profile",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
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

func TestEncodingProfileRepo_GetDefault(t *testing.T) {
	db := setupEncodingProfileTestDB(t)
	repo := NewEncodingProfileRepository(db)
	ctx := context.Background()

	// Create non-default profile
	profile1 := &models.EncodingProfile{
		Name:             "Not Default",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
	}
	require.NoError(t, repo.Create(ctx, profile1))

	// Create default profile
	profile2 := &models.EncodingProfile{
		Name:             "Default",
		TargetVideoCodec: models.VideoCodecH265,
		TargetAudioCodec: models.AudioCodecOpus,
		QualityPreset:    models.QualityPresetHigh,
		IsDefault:        true,
	}
	require.NoError(t, repo.Create(ctx, profile2))

	found, err := repo.GetDefault(ctx)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "Default", found.Name)
	assert.True(t, found.IsDefault)
}

func TestEncodingProfileRepo_GetDefault_NotFound(t *testing.T) {
	db := setupEncodingProfileTestDB(t)
	repo := NewEncodingProfileRepository(db)
	ctx := context.Background()

	// Create non-default profile
	profile := &models.EncodingProfile{
		Name:             "Not Default",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
	}
	require.NoError(t, repo.Create(ctx, profile))

	found, err := repo.GetDefault(ctx)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestEncodingProfileRepo_GetSystem(t *testing.T) {
	db := setupEncodingProfileTestDB(t)
	repo := NewEncodingProfileRepository(db)
	ctx := context.Background()

	// Create system profile
	system := &models.EncodingProfile{
		Name:             "System Profile",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
		IsSystem:         true,
	}
	require.NoError(t, repo.Create(ctx, system))

	// Create user profile
	user := &models.EncodingProfile{
		Name:             "User Profile",
		TargetVideoCodec: models.VideoCodecH265,
		TargetAudioCodec: models.AudioCodecOpus,
		QualityPreset:    models.QualityPresetHigh,
		IsSystem:         false,
	}
	require.NoError(t, repo.Create(ctx, user))

	profiles, err := repo.GetSystem(ctx)
	require.NoError(t, err)
	assert.Len(t, profiles, 1)
	assert.Equal(t, "System Profile", profiles[0].Name)
	assert.True(t, profiles[0].IsSystem)
}

func TestEncodingProfileRepo_Update(t *testing.T) {
	db := setupEncodingProfileTestDB(t)
	repo := NewEncodingProfileRepository(db)
	ctx := context.Background()

	profile := &models.EncodingProfile{
		Name:             "Original",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
	}
	require.NoError(t, repo.Create(ctx, profile))

	// Update
	profile.Name = "Updated"
	profile.TargetVideoCodec = models.VideoCodecH265
	profile.QualityPreset = models.QualityPresetHigh

	err := repo.Update(ctx, profile)
	require.NoError(t, err)

	// Verify
	found, err := repo.GetByID(ctx, profile.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", found.Name)
	assert.Equal(t, models.VideoCodecH265, found.TargetVideoCodec)
	assert.Equal(t, models.QualityPresetHigh, found.QualityPreset)
}

func TestEncodingProfileRepo_Delete(t *testing.T) {
	db := setupEncodingProfileTestDB(t)
	repo := NewEncodingProfileRepository(db)
	ctx := context.Background()

	profile := &models.EncodingProfile{
		Name:             "To Delete",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
	}
	require.NoError(t, repo.Create(ctx, profile))

	err := repo.Delete(ctx, profile.ID)
	require.NoError(t, err)

	// Verify it's gone
	found, err := repo.GetByID(ctx, profile.ID)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestEncodingProfileRepo_Count(t *testing.T) {
	db := setupEncodingProfileTestDB(t)
	repo := NewEncodingProfileRepository(db)
	ctx := context.Background()

	count, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Create profiles
	for i := 0; i < 5; i++ {
		profile := &models.EncodingProfile{
			Name:             "Profile " + string(rune('A'+i)),
			TargetVideoCodec: models.VideoCodecH264,
			TargetAudioCodec: models.AudioCodecAAC,
			QualityPreset:    models.QualityPresetMedium,
		}
		require.NoError(t, repo.Create(ctx, profile))
	}

	count, err = repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(5), count)
}

func TestEncodingProfileRepo_CountEnabled(t *testing.T) {
	db := setupEncodingProfileTestDB(t)
	repo := NewEncodingProfileRepository(db)
	ctx := context.Background()

	// Create enabled profiles
	for i := 0; i < 3; i++ {
		profile := &models.EncodingProfile{
			Name:             "Enabled " + string(rune('A'+i)),
			TargetVideoCodec: models.VideoCodecH264,
			TargetAudioCodec: models.AudioCodecAAC,
			QualityPreset:    models.QualityPresetMedium,
			Enabled:          true,
		}
		require.NoError(t, repo.Create(ctx, profile))
	}

	// Create profiles to be disabled (GORM default:true interferes with false on create)
	for i := 0; i < 2; i++ {
		profile := &models.EncodingProfile{
			Name:             "Disabled " + string(rune('A'+i)),
			TargetVideoCodec: models.VideoCodecH265,
			TargetAudioCodec: models.AudioCodecOpus,
			QualityPreset:    models.QualityPresetHigh,
		}
		require.NoError(t, repo.Create(ctx, profile))
		// Disable after creation
		require.NoError(t, db.Model(profile).UpdateColumn("enabled", false).Error)
	}

	count, err := repo.CountEnabled(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

func TestEncodingProfileRepo_SetDefault(t *testing.T) {
	db := setupEncodingProfileTestDB(t)
	repo := NewEncodingProfileRepository(db)
	ctx := context.Background()

	// Create profiles
	profile1 := &models.EncodingProfile{
		Name:             "Profile 1",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
		IsDefault:        true,
	}
	require.NoError(t, repo.Create(ctx, profile1))

	profile2 := &models.EncodingProfile{
		Name:             "Profile 2",
		TargetVideoCodec: models.VideoCodecH265,
		TargetAudioCodec: models.AudioCodecOpus,
		QualityPreset:    models.QualityPresetHigh,
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

func TestEncodingProfileRepo_SetDefault_NotFound(t *testing.T) {
	db := setupEncodingProfileTestDB(t)
	repo := NewEncodingProfileRepository(db)
	ctx := context.Background()

	nonExistentID := models.NewULID()
	err := repo.SetDefault(ctx, nonExistentID)
	assert.Error(t, err)
}

func TestEncodingProfileRepo_GetAll_OrderingByDefaultAndSystem(t *testing.T) {
	db := setupEncodingProfileTestDB(t)
	repo := NewEncodingProfileRepository(db)
	ctx := context.Background()

	// Create user profile first
	user := &models.EncodingProfile{
		Name:             "Alpha User",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
	}
	require.NoError(t, repo.Create(ctx, user))

	// Create system profile
	system := &models.EncodingProfile{
		Name:             "Beta System",
		TargetVideoCodec: models.VideoCodecH265,
		TargetAudioCodec: models.AudioCodecOpus,
		QualityPreset:    models.QualityPresetHigh,
		IsSystem:         true,
	}
	require.NoError(t, repo.Create(ctx, system))

	// Create default profile
	defaultProfile := &models.EncodingProfile{
		Name:             "Zeta Default",
		TargetVideoCodec: models.VideoCodecVP9,
		TargetAudioCodec: models.AudioCodecOpus,
		QualityPreset:    models.QualityPresetMedium,
		IsDefault:        true,
	}
	require.NoError(t, repo.Create(ctx, defaultProfile))

	profiles, err := repo.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, profiles, 3)

	// Order should be: default first, then system, then by name
	assert.Equal(t, "Zeta Default", profiles[0].Name)
	assert.True(t, profiles[0].IsDefault)
	assert.Equal(t, "Beta System", profiles[1].Name)
	assert.True(t, profiles[1].IsSystem)
	assert.Equal(t, "Alpha User", profiles[2].Name)
}

func TestEncodingProfileRepo_QualityPresets(t *testing.T) {
	db := setupEncodingProfileTestDB(t)
	repo := NewEncodingProfileRepository(db)
	ctx := context.Background()

	presets := []models.QualityPreset{
		models.QualityPresetLow,
		models.QualityPresetMedium,
		models.QualityPresetHigh,
		models.QualityPresetUltra,
	}

	for _, preset := range presets {
		profile := &models.EncodingProfile{
			Name:             "Profile " + string(preset),
			TargetVideoCodec: models.VideoCodecH264,
			TargetAudioCodec: models.AudioCodecAAC,
			QualityPreset:    preset,
		}
		err := repo.Create(ctx, profile)
		require.NoError(t, err, "should create profile with preset %s", preset)

		found, err := repo.GetByID(ctx, profile.ID)
		require.NoError(t, err)
		assert.Equal(t, preset, found.QualityPreset)
	}
}

func TestEncodingProfileRepo_HWAccelTypes(t *testing.T) {
	db := setupEncodingProfileTestDB(t)
	repo := NewEncodingProfileRepository(db)
	ctx := context.Background()

	hwTypes := []models.HWAccelType{
		models.HWAccelAuto,
		models.HWAccelNone,
		models.HWAccelNVDEC,
		models.HWAccelVAAPI,
		models.HWAccelQSV,
		models.HWAccelVT,
	}

	for _, hw := range hwTypes {
		profile := &models.EncodingProfile{
			Name:             "Profile " + string(hw),
			TargetVideoCodec: models.VideoCodecH264,
			TargetAudioCodec: models.AudioCodecAAC,
			QualityPreset:    models.QualityPresetMedium,
			HWAccel:          hw,
		}
		err := repo.Create(ctx, profile)
		require.NoError(t, err, "should create profile with hw accel %s", hw)

		found, err := repo.GetByID(ctx, profile.ID)
		require.NoError(t, err)
		assert.Equal(t, hw, found.HWAccel)
	}
}
