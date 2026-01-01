package service_test

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/jmylchreest/tvarr/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		&models.EncodingProfile{},
		&models.LastKnownCodec{},
		&models.StreamSource{},
		&models.Channel{},
		&models.StreamProxy{},
	)
	require.NoError(t, err)

	encodingProfileRepo := repository.NewEncodingProfileRepository(db)
	lastKnownCodecRepo := repository.NewLastKnownCodecRepository(db)
	channelRepo := repository.NewChannelRepository(db)
	streamProxyRepo := repository.NewStreamProxyRepository(db)

	svc := service.NewRelayService(encodingProfileRepo, lastKnownCodecRepo, channelRepo, streamProxyRepo)
	return svc
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
