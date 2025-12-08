package service_test

import (
	"context"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/jmylchreest/tvarr/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupRelayServiceTest(t *testing.T) *service.RelayService {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Migrate all required models
	err = db.AutoMigrate(
		&models.RelayProfile{},
		&models.LastKnownCodec{},
		&models.StreamSource{},
		&models.Channel{},
		&models.StreamProxy{},
	)
	require.NoError(t, err)

	relayProfileRepo := repository.NewRelayProfileRepository(db)
	lastKnownCodecRepo := repository.NewLastKnownCodecRepository(db)
	channelRepo := repository.NewChannelRepository(db)
	streamProxyRepo := repository.NewStreamProxyRepository(db)

	svc := service.NewRelayService(relayProfileRepo, lastKnownCodecRepo, channelRepo, streamProxyRepo)
	return svc
}

func TestRelayService_CreateProfile(t *testing.T) {
	svc := setupRelayServiceTest(t)
	defer svc.Close()

	ctx := context.Background()

	t.Run("creates profile with valid data", func(t *testing.T) {
		profile := &models.RelayProfile{
			Name:            "test-profile",
			Description:     "Test profile description",
			VideoCodec:      models.VideoCodecCopy,
			AudioCodec:      models.AudioCodecCopy,
			HWAccel:         models.HWAccelNone,
			ContainerFormat: models.ContainerFormatMPEGTS,
		}

		err := svc.CreateProfile(ctx, profile)
		require.NoError(t, err)
		assert.NotZero(t, profile.ID)
	})

	t.Run("validates required name", func(t *testing.T) {
		profile := &models.RelayProfile{
			Name:       "",
			VideoCodec: models.VideoCodecCopy,
			AudioCodec: models.AudioCodecCopy,
		}

		err := svc.CreateProfile(ctx, profile)
		assert.Error(t, err)
	})
}

func TestRelayService_GetProfile(t *testing.T) {
	svc := setupRelayServiceTest(t)
	defer svc.Close()

	ctx := context.Background()

	// Create a profile first
	profile := &models.RelayProfile{
		Name:            "get-test",
		VideoCodec:      models.VideoCodecCopy,
		AudioCodec:      models.AudioCodecCopy,
		HWAccel:         models.HWAccelNone,
		ContainerFormat: models.ContainerFormatMPEGTS,
	}
	require.NoError(t, svc.CreateProfile(ctx, profile))

	t.Run("gets profile by ID", func(t *testing.T) {
		retrieved, err := svc.GetProfileByID(ctx, profile.ID)
		require.NoError(t, err)
		assert.Equal(t, profile.Name, retrieved.Name)
	})

	t.Run("gets profile by name", func(t *testing.T) {
		retrieved, err := svc.GetProfileByName(ctx, "get-test")
		require.NoError(t, err)
		assert.Equal(t, profile.ID, retrieved.ID)
	})

	t.Run("returns error for non-existent ID", func(t *testing.T) {
		nonExistent := models.NewULID()
		_, err := svc.GetProfileByID(ctx, nonExistent)
		assert.ErrorIs(t, err, service.ErrRelayProfileNotFound)
	})

	t.Run("returns error for non-existent name", func(t *testing.T) {
		_, err := svc.GetProfileByName(ctx, "non-existent")
		assert.ErrorIs(t, err, service.ErrRelayProfileNotFound)
	})
}

func TestRelayService_UpdateProfile(t *testing.T) {
	svc := setupRelayServiceTest(t)
	defer svc.Close()

	ctx := context.Background()

	// Create a profile first
	profile := &models.RelayProfile{
		Name:            "update-test",
		VideoCodec:      models.VideoCodecCopy,
		AudioCodec:      models.AudioCodecCopy,
		HWAccel:         models.HWAccelNone,
		ContainerFormat: models.ContainerFormatMPEGTS,
	}
	require.NoError(t, svc.CreateProfile(ctx, profile))

	t.Run("updates profile successfully", func(t *testing.T) {
		profile.Description = "Updated description"
		profile.VideoBitrate = 5000

		err := svc.UpdateProfile(ctx, profile)
		require.NoError(t, err)

		updated, err := svc.GetProfileByID(ctx, profile.ID)
		require.NoError(t, err)
		assert.Equal(t, "Updated description", updated.Description)
		assert.Equal(t, 5000, updated.VideoBitrate)
	})
}

func TestRelayService_DeleteProfile(t *testing.T) {
	svc := setupRelayServiceTest(t)
	defer svc.Close()

	ctx := context.Background()

	// Create a profile first
	profile := &models.RelayProfile{
		Name:            "delete-test",
		VideoCodec:      models.VideoCodecCopy,
		AudioCodec:      models.AudioCodecCopy,
		HWAccel:         models.HWAccelNone,
		ContainerFormat: models.ContainerFormatMPEGTS,
	}
	require.NoError(t, svc.CreateProfile(ctx, profile))

	t.Run("deletes profile successfully", func(t *testing.T) {
		err := svc.DeleteProfile(ctx, profile.ID)
		require.NoError(t, err)

		_, err = svc.GetProfileByID(ctx, profile.ID)
		assert.ErrorIs(t, err, service.ErrRelayProfileNotFound)
	})
}

func TestRelayService_GetAllProfiles(t *testing.T) {
	svc := setupRelayServiceTest(t)
	defer svc.Close()

	ctx := context.Background()

	// Create multiple profiles
	for i := 0; i < 3; i++ {
		profile := &models.RelayProfile{
			Name:            "list-test-" + string(rune('a'+i)),
			VideoCodec:      models.VideoCodecCopy,
			AudioCodec:      models.AudioCodecCopy,
			HWAccel:         models.HWAccelNone,
			ContainerFormat: models.ContainerFormatMPEGTS,
		}
		require.NoError(t, svc.CreateProfile(ctx, profile))
	}

	t.Run("returns all profiles", func(t *testing.T) {
		profiles, err := svc.GetAllProfiles(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(profiles), 3)
	})
}

func TestRelayService_DefaultProfile(t *testing.T) {
	svc := setupRelayServiceTest(t)
	defer svc.Close()

	ctx := context.Background()

	// Create profiles
	profile1 := &models.RelayProfile{
		Name:            "default-test-1",
		VideoCodec:      models.VideoCodecCopy,
		AudioCodec:      models.AudioCodecCopy,
		HWAccel:         models.HWAccelNone,
		ContainerFormat: models.ContainerFormatMPEGTS,
	}
	require.NoError(t, svc.CreateProfile(ctx, profile1))

	profile2 := &models.RelayProfile{
		Name:            "default-test-2",
		VideoCodec:      models.VideoCodecCopy,
		AudioCodec:      models.AudioCodecCopy,
		HWAccel:         models.HWAccelNone,
		ContainerFormat: models.ContainerFormatMPEGTS,
	}
	require.NoError(t, svc.CreateProfile(ctx, profile2))

	t.Run("sets profile as default", func(t *testing.T) {
		err := svc.SetDefaultProfile(ctx, profile1.ID)
		require.NoError(t, err)

		defaultProfile, err := svc.GetDefaultProfile(ctx)
		require.NoError(t, err)
		assert.Equal(t, profile1.ID, defaultProfile.ID)
		assert.True(t, defaultProfile.IsDefault)
	})

	t.Run("changing default unsets previous", func(t *testing.T) {
		err := svc.SetDefaultProfile(ctx, profile2.ID)
		require.NoError(t, err)

		// Verify new default
		defaultProfile, err := svc.GetDefaultProfile(ctx)
		require.NoError(t, err)
		assert.Equal(t, profile2.ID, defaultProfile.ID)

		// Verify old profile is no longer default
		oldDefault, err := svc.GetProfileByID(ctx, profile1.ID)
		require.NoError(t, err)
		assert.False(t, oldDefault.IsDefault)
	})
}

func TestRelayService_RelayStats(t *testing.T) {
	svc := setupRelayServiceTest(t)
	defer svc.Close()

	t.Run("returns empty stats initially", func(t *testing.T) {
		stats := svc.GetRelayStats()
		assert.Equal(t, 0, stats.ActiveSessions)
	})
}

func TestRelayService_Close(t *testing.T) {
	svc := setupRelayServiceTest(t)

	t.Run("close is idempotent", func(t *testing.T) {
		svc.Close()
		svc.Close() // Should not panic
	})
}
