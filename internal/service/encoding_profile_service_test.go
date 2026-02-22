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

func setupEncodingProfileTestDB(t *testing.T) (*gorm.DB, repository.EncodingProfileRepository) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.EncodingProfile{})
	require.NoError(t, err)

	repo := repository.NewEncodingProfileRepository(db)
	return db, repo
}

func TestEncodingProfileService_Create(t *testing.T) {
	_, repo := setupEncodingProfileTestDB(t)
	svc := NewEncodingProfileService(repo)
	ctx := context.Background()

	profile := &models.EncodingProfile{
		Name:             "Test Profile",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
	}

	err := svc.Create(ctx, profile)
	require.NoError(t, err)
	assert.False(t, profile.ID.IsZero())
	assert.False(t, profile.IsSystem) // Service should prevent system flag
}

func TestEncodingProfileService_Create_CannotCreateSystem(t *testing.T) {
	_, repo := setupEncodingProfileTestDB(t)
	svc := NewEncodingProfileService(repo)
	ctx := context.Background()

	profile := &models.EncodingProfile{
		Name:             "Fake System",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
		IsSystem:         true, // Try to create as system
	}

	err := svc.Create(ctx, profile)
	require.NoError(t, err)

	// Verify it was saved as non-system
	found, err := svc.GetByID(ctx, profile.ID)
	require.NoError(t, err)
	assert.False(t, found.IsSystem)
}

func TestEncodingProfileService_GetByID(t *testing.T) {
	_, repo := setupEncodingProfileTestDB(t)
	svc := NewEncodingProfileService(repo)
	ctx := context.Background()

	profile := &models.EncodingProfile{
		Name:             "Find Me",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
	}
	require.NoError(t, svc.Create(ctx, profile))

	found, err := svc.GetByID(ctx, profile.ID)
	require.NoError(t, err)
	assert.Equal(t, "Find Me", found.Name)
}

func TestEncodingProfileService_GetByID_NotFound(t *testing.T) {
	_, repo := setupEncodingProfileTestDB(t)
	svc := NewEncodingProfileService(repo)
	ctx := context.Background()

	_, err := svc.GetByID(ctx, models.NewULID())
	assert.ErrorIs(t, err, models.ErrEncodingProfileNotFound)
}

func TestEncodingProfileService_GetByName(t *testing.T) {
	_, repo := setupEncodingProfileTestDB(t)
	svc := NewEncodingProfileService(repo)
	ctx := context.Background()

	profile := &models.EncodingProfile{
		Name:             "Named Profile",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
	}
	require.NoError(t, svc.Create(ctx, profile))

	found, err := svc.GetByName(ctx, "Named Profile")
	require.NoError(t, err)
	assert.Equal(t, profile.ID, found.ID)
}

func TestEncodingProfileService_GetByName_NotFound(t *testing.T) {
	_, repo := setupEncodingProfileTestDB(t)
	svc := NewEncodingProfileService(repo)
	ctx := context.Background()

	_, err := svc.GetByName(ctx, "Nonexistent")
	assert.ErrorIs(t, err, models.ErrEncodingProfileNotFound)
}

func TestEncodingProfileService_Update(t *testing.T) {
	_, repo := setupEncodingProfileTestDB(t)
	svc := NewEncodingProfileService(repo)
	ctx := context.Background()

	profile := &models.EncodingProfile{
		Name:             "Original",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
	}
	require.NoError(t, svc.Create(ctx, profile))

	// Update
	profile.Name = "Updated"
	profile.QualityPreset = models.QualityPresetHigh
	err := svc.Update(ctx, profile)
	require.NoError(t, err)

	// Verify
	found, err := svc.GetByID(ctx, profile.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", found.Name)
	assert.Equal(t, models.QualityPresetHigh, found.QualityPreset)
}

func TestEncodingProfileService_Update_SystemProfile_OnlyEnabledToggle(t *testing.T) {
	db, repo := setupEncodingProfileTestDB(t)
	svc := NewEncodingProfileService(repo)
	ctx := context.Background()

	// Create system profile directly via DB (not service)
	systemProfile := &models.EncodingProfile{
		Name:             "System Profile",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
		IsSystem:         true,
		Enabled:          new(true),
	}
	require.NoError(t, db.Create(systemProfile).Error)

	// Try to update name - should fail
	systemProfile.Name = "Renamed System"
	err := svc.Update(ctx, systemProfile)
	assert.ErrorIs(t, err, ErrEncodingProfileCannotEditSystem)

	// Toggle enabled - should work
	systemProfile.Name = "System Profile" // Reset to original
	systemProfile.Enabled = new(false)
	err = svc.Update(ctx, systemProfile)
	require.NoError(t, err)

	// Verify enabled was toggled
	found, err := svc.GetByID(ctx, systemProfile.ID)
	require.NoError(t, err)
	assert.False(t, models.BoolVal(found.Enabled))
}

func TestEncodingProfileService_Delete(t *testing.T) {
	_, repo := setupEncodingProfileTestDB(t)
	svc := NewEncodingProfileService(repo)
	ctx := context.Background()

	profile := &models.EncodingProfile{
		Name:             "To Delete",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
	}
	require.NoError(t, svc.Create(ctx, profile))

	err := svc.Delete(ctx, profile.ID)
	require.NoError(t, err)

	// Verify deleted
	_, err = svc.GetByID(ctx, profile.ID)
	assert.ErrorIs(t, err, models.ErrEncodingProfileNotFound)
}

func TestEncodingProfileService_Delete_SystemProfile(t *testing.T) {
	db, repo := setupEncodingProfileTestDB(t)
	svc := NewEncodingProfileService(repo)
	ctx := context.Background()

	// Create system profile directly via DB
	systemProfile := &models.EncodingProfile{
		Name:             "System Profile",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
		IsSystem:         true,
	}
	require.NoError(t, db.Create(systemProfile).Error)

	// Try to delete system profile - should fail
	err := svc.Delete(ctx, systemProfile.ID)
	assert.ErrorIs(t, err, ErrEncodingProfileCannotDeleteSystem)
}

func TestEncodingProfileService_Delete_NotFound(t *testing.T) {
	_, repo := setupEncodingProfileTestDB(t)
	svc := NewEncodingProfileService(repo)
	ctx := context.Background()

	err := svc.Delete(ctx, models.NewULID())
	assert.ErrorIs(t, err, models.ErrEncodingProfileNotFound)
}

func TestEncodingProfileService_SetDefault(t *testing.T) {
	_, repo := setupEncodingProfileTestDB(t)
	svc := NewEncodingProfileService(repo)
	ctx := context.Background()

	// Create profiles
	profile1 := &models.EncodingProfile{
		Name:             "Profile 1",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
		IsDefault:        true,
	}
	require.NoError(t, svc.Create(ctx, profile1))

	profile2 := &models.EncodingProfile{
		Name:             "Profile 2",
		TargetVideoCodec: models.VideoCodecH265,
		TargetAudioCodec: models.AudioCodecOpus,
		QualityPreset:    models.QualityPresetHigh,
	}
	require.NoError(t, svc.Create(ctx, profile2))

	// Set profile2 as default
	err := svc.SetDefault(ctx, profile2.ID)
	require.NoError(t, err)

	// Verify
	defaultProfile, err := svc.GetDefault(ctx)
	require.NoError(t, err)
	assert.Equal(t, profile2.ID, defaultProfile.ID)

	// Verify profile1 is no longer default
	p1, err := svc.GetByID(ctx, profile1.ID)
	require.NoError(t, err)
	assert.False(t, p1.IsDefault)
}

func TestEncodingProfileService_ToggleEnabled(t *testing.T) {
	_, repo := setupEncodingProfileTestDB(t)
	svc := NewEncodingProfileService(repo)
	ctx := context.Background()

	profile := &models.EncodingProfile{
		Name:             "Toggle Me",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetMedium,
		Enabled:          new(true),
	}
	require.NoError(t, svc.Create(ctx, profile))

	// Toggle off
	toggled, err := svc.ToggleEnabled(ctx, profile.ID)
	require.NoError(t, err)
	assert.False(t, models.BoolVal(toggled.Enabled))

	// Toggle on
	toggled, err = svc.ToggleEnabled(ctx, profile.ID)
	require.NoError(t, err)
	assert.True(t, models.BoolVal(toggled.Enabled))
}

func TestEncodingProfileService_Clone(t *testing.T) {
	_, repo := setupEncodingProfileTestDB(t)
	svc := NewEncodingProfileService(repo)
	ctx := context.Background()

	original := &models.EncodingProfile{
		Name:             "Original",
		Description:      "Original description",
		TargetVideoCodec: models.VideoCodecH264,
		TargetAudioCodec: models.AudioCodecAAC,
		QualityPreset:    models.QualityPresetHigh,
		HWAccel:          models.HWAccelVAAPI,
	}
	require.NoError(t, svc.Create(ctx, original))

	// Clone
	clone, err := svc.Clone(ctx, original.ID, "Cloned Profile")
	require.NoError(t, err)

	// Verify clone
	assert.NotEqual(t, original.ID, clone.ID)
	assert.Equal(t, "Cloned Profile", clone.Name)
	assert.Contains(t, clone.Description, "Cloned from Original")
	assert.Equal(t, original.TargetVideoCodec, clone.TargetVideoCodec)
	assert.Equal(t, original.TargetAudioCodec, clone.TargetAudioCodec)
	assert.Equal(t, original.QualityPreset, clone.QualityPreset)
	assert.Equal(t, original.HWAccel, clone.HWAccel)
	assert.False(t, clone.IsSystem)
	assert.False(t, clone.IsDefault)
}

func TestEncodingProfileService_Count(t *testing.T) {
	_, repo := setupEncodingProfileTestDB(t)
	svc := NewEncodingProfileService(repo)
	ctx := context.Background()

	count, err := svc.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Create profiles
	for i := range 5 {
		profile := &models.EncodingProfile{
			Name:             "Profile " + string(rune('A'+i)),
			TargetVideoCodec: models.VideoCodecH264,
			TargetAudioCodec: models.AudioCodecAAC,
			QualityPreset:    models.QualityPresetMedium,
		}
		require.NoError(t, svc.Create(ctx, profile))
	}

	count, err = svc.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(5), count)
}
