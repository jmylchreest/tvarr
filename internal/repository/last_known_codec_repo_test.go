package repository

import (
	"context"
	"testing"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupLastKnownCodecTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.LastKnownCodec{})
	require.NoError(t, err)

	return db
}

func createTestCodec(t *testing.T, streamURL string) *models.LastKnownCodec {
	t.Helper()
	return &models.LastKnownCodec{
		StreamURL:   streamURL,
		VideoCodec:  "h264",
		VideoWidth:  1920,
		VideoHeight: 1080,
		AudioCodec:  "aac",
		ProbedAt:    models.Now(),
	}
}

func TestLastKnownCodecRepo_Create(t *testing.T) {
	db := setupLastKnownCodecTestDB(t)
	repo := NewLastKnownCodecRepository(db)
	ctx := context.Background()

	codec := createTestCodec(t, "http://example.com/stream.m3u8")

	err := repo.Create(ctx, codec)
	require.NoError(t, err)
	assert.False(t, codec.ID.IsZero())

	// Verify it was created
	found, err := repo.GetByID(ctx, codec.ID)
	require.NoError(t, err)
	assert.Equal(t, "http://example.com/stream.m3u8", found.StreamURL)
	assert.Equal(t, "h264", found.VideoCodec)
}

func TestLastKnownCodecRepo_Create_Validation(t *testing.T) {
	db := setupLastKnownCodecTestDB(t)
	repo := NewLastKnownCodecRepository(db)
	ctx := context.Background()

	// Missing stream URL
	codec := &models.LastKnownCodec{
		VideoCodec: "h264",
	}

	err := repo.Create(ctx, codec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validating codec entry")
}

func TestLastKnownCodecRepo_Create_DuplicateURL(t *testing.T) {
	db := setupLastKnownCodecTestDB(t)
	repo := NewLastKnownCodecRepository(db)
	ctx := context.Background()

	codec1 := createTestCodec(t, "http://duplicate.com/stream")
	require.NoError(t, repo.Create(ctx, codec1))

	codec2 := createTestCodec(t, "http://duplicate.com/stream")
	err := repo.Create(ctx, codec2)
	assert.Error(t, err)
}

func TestLastKnownCodecRepo_GetByID(t *testing.T) {
	db := setupLastKnownCodecTestDB(t)
	repo := NewLastKnownCodecRepository(db)
	ctx := context.Background()

	codec := createTestCodec(t, "http://example.com/stream.m3u8")
	codec.VideoProfile = "High"
	codec.VideoLevel = "4.1"
	require.NoError(t, repo.Create(ctx, codec))

	found, err := repo.GetByID(ctx, codec.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "h264", found.VideoCodec)
	assert.Equal(t, "High", found.VideoProfile)
	assert.Equal(t, "4.1", found.VideoLevel)
}

func TestLastKnownCodecRepo_GetByID_NotFound(t *testing.T) {
	db := setupLastKnownCodecTestDB(t)
	repo := NewLastKnownCodecRepository(db)
	ctx := context.Background()

	nonExistentID := models.NewULID()
	found, err := repo.GetByID(ctx, nonExistentID)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestLastKnownCodecRepo_GetByStreamURL(t *testing.T) {
	db := setupLastKnownCodecTestDB(t)
	repo := NewLastKnownCodecRepository(db)
	ctx := context.Background()

	codec := createTestCodec(t, "http://findme.com/stream")
	require.NoError(t, repo.Create(ctx, codec))

	found, err := repo.GetByStreamURL(ctx, "http://findme.com/stream")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, codec.ID, found.ID)

	notFound, err := repo.GetByStreamURL(ctx, "http://notfound.com/stream")
	require.NoError(t, err)
	assert.Nil(t, notFound)
}

func TestLastKnownCodecRepo_GetBySourceID(t *testing.T) {
	db := setupLastKnownCodecTestDB(t)
	repo := NewLastKnownCodecRepository(db)
	ctx := context.Background()

	sourceID := models.NewULID()

	// Create codecs for this source
	for i := 0; i < 3; i++ {
		codec := createTestCodec(t, "http://source.com/stream"+string(rune('1'+i)))
		codec.SourceID = sourceID
		require.NoError(t, repo.Create(ctx, codec))
	}

	// Create codec for different source
	otherCodec := createTestCodec(t, "http://other.com/stream")
	otherCodec.SourceID = models.NewULID()
	require.NoError(t, repo.Create(ctx, otherCodec))

	codecs, err := repo.GetBySourceID(ctx, sourceID)
	require.NoError(t, err)
	assert.Len(t, codecs, 3)
}

func TestLastKnownCodecRepo_Upsert_Create(t *testing.T) {
	db := setupLastKnownCodecTestDB(t)
	repo := NewLastKnownCodecRepository(db)
	ctx := context.Background()

	codec := createTestCodec(t, "http://upsert.com/stream")

	err := repo.Upsert(ctx, codec)
	require.NoError(t, err)

	found, err := repo.GetByStreamURL(ctx, "http://upsert.com/stream")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "h264", found.VideoCodec)
}

func TestLastKnownCodecRepo_Upsert_Update(t *testing.T) {
	db := setupLastKnownCodecTestDB(t)
	repo := NewLastKnownCodecRepository(db)
	ctx := context.Background()

	// Create initial codec
	codec := createTestCodec(t, "http://upsert.com/stream")
	require.NoError(t, repo.Create(ctx, codec))
	originalID := codec.ID

	// Update via upsert
	updated := createTestCodec(t, "http://upsert.com/stream")
	updated.VideoCodec = "hevc"
	updated.VideoWidth = 3840
	updated.VideoHeight = 2160

	err := repo.Upsert(ctx, updated)
	require.NoError(t, err)

	// Verify update
	found, err := repo.GetByStreamURL(ctx, "http://upsert.com/stream")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, originalID, found.ID) // ID should remain the same
	assert.Equal(t, "hevc", found.VideoCodec)
	assert.Equal(t, 3840, found.VideoWidth)
}

func TestLastKnownCodecRepo_Update(t *testing.T) {
	db := setupLastKnownCodecTestDB(t)
	repo := NewLastKnownCodecRepository(db)
	ctx := context.Background()

	codec := createTestCodec(t, "http://update.com/stream")
	require.NoError(t, repo.Create(ctx, codec))

	// Update
	codec.VideoCodec = "vp9"
	codec.AudioCodec = "opus"

	err := repo.Update(ctx, codec)
	require.NoError(t, err)

	// Verify
	found, err := repo.GetByID(ctx, codec.ID)
	require.NoError(t, err)
	assert.Equal(t, "vp9", found.VideoCodec)
	assert.Equal(t, "opus", found.AudioCodec)
}

func TestLastKnownCodecRepo_Delete(t *testing.T) {
	db := setupLastKnownCodecTestDB(t)
	repo := NewLastKnownCodecRepository(db)
	ctx := context.Background()

	codec := createTestCodec(t, "http://delete.com/stream")
	require.NoError(t, repo.Create(ctx, codec))

	err := repo.Delete(ctx, codec.ID)
	require.NoError(t, err)

	found, err := repo.GetByID(ctx, codec.ID)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestLastKnownCodecRepo_DeleteByStreamURL(t *testing.T) {
	db := setupLastKnownCodecTestDB(t)
	repo := NewLastKnownCodecRepository(db)
	ctx := context.Background()

	codec := createTestCodec(t, "http://deleteurl.com/stream")
	require.NoError(t, repo.Create(ctx, codec))

	err := repo.DeleteByStreamURL(ctx, "http://deleteurl.com/stream")
	require.NoError(t, err)

	found, err := repo.GetByStreamURL(ctx, "http://deleteurl.com/stream")
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestLastKnownCodecRepo_DeleteBySourceID(t *testing.T) {
	db := setupLastKnownCodecTestDB(t)
	repo := NewLastKnownCodecRepository(db)
	ctx := context.Background()

	sourceID := models.NewULID()

	// Create codecs for this source
	for i := 0; i < 3; i++ {
		codec := createTestCodec(t, "http://source.com/stream"+string(rune('1'+i)))
		codec.SourceID = sourceID
		require.NoError(t, repo.Create(ctx, codec))
	}

	// Create codec for different source
	otherCodec := createTestCodec(t, "http://other.com/stream")
	otherCodec.SourceID = models.NewULID()
	require.NoError(t, repo.Create(ctx, otherCodec))

	deleted, err := repo.DeleteBySourceID(ctx, sourceID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), deleted)

	// Verify other source codec still exists
	found, err := repo.GetByStreamURL(ctx, "http://other.com/stream")
	require.NoError(t, err)
	assert.NotNil(t, found)
}

func TestLastKnownCodecRepo_DeleteExpired(t *testing.T) {
	db := setupLastKnownCodecTestDB(t)
	repo := NewLastKnownCodecRepository(db)
	ctx := context.Background()

	// Create expired codec
	expired := createTestCodec(t, "http://expired.com/stream")
	expiredTime := models.Time(time.Now().Add(-time.Hour))
	expired.ExpiresAt = &expiredTime
	require.NoError(t, repo.Create(ctx, expired))

	// Create non-expired codec
	valid := createTestCodec(t, "http://valid.com/stream")
	validTime := models.Time(time.Now().Add(time.Hour))
	valid.ExpiresAt = &validTime
	require.NoError(t, repo.Create(ctx, valid))

	// Create codec without expiry
	noExpiry := createTestCodec(t, "http://noexpiry.com/stream")
	require.NoError(t, repo.Create(ctx, noExpiry))

	deleted, err := repo.DeleteExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	// Verify expired is gone
	found, err := repo.GetByStreamURL(ctx, "http://expired.com/stream")
	require.NoError(t, err)
	assert.Nil(t, found)

	// Verify valid still exists
	found, err = repo.GetByStreamURL(ctx, "http://valid.com/stream")
	require.NoError(t, err)
	assert.NotNil(t, found)

	// Verify no-expiry still exists
	found, err = repo.GetByStreamURL(ctx, "http://noexpiry.com/stream")
	require.NoError(t, err)
	assert.NotNil(t, found)
}

func TestLastKnownCodecRepo_Touch(t *testing.T) {
	db := setupLastKnownCodecTestDB(t)
	repo := NewLastKnownCodecRepository(db)
	ctx := context.Background()

	codec := createTestCodec(t, "http://touch.com/stream")
	require.NoError(t, repo.Create(ctx, codec))

	// Verify initial hit count
	found, _ := repo.GetByStreamURL(ctx, "http://touch.com/stream")
	assert.Equal(t, int64(0), found.HitCount)

	// Touch multiple times
	for i := 0; i < 3; i++ {
		err := repo.Touch(ctx, "http://touch.com/stream")
		require.NoError(t, err)
	}

	// Verify hit count increased
	found, _ = repo.GetByStreamURL(ctx, "http://touch.com/stream")
	assert.Equal(t, int64(3), found.HitCount)
}

func TestLastKnownCodecRepo_Touch_NotFound(t *testing.T) {
	db := setupLastKnownCodecTestDB(t)
	repo := NewLastKnownCodecRepository(db)
	ctx := context.Background()

	err := repo.Touch(ctx, "http://nonexistent.com/stream")
	assert.ErrorIs(t, err, models.ErrStreamURLNotFound)
}

func TestLastKnownCodecRepo_GetValidCount(t *testing.T) {
	db := setupLastKnownCodecTestDB(t)
	repo := NewLastKnownCodecRepository(db)
	ctx := context.Background()

	// Create valid codec
	valid := createTestCodec(t, "http://valid.com/stream")
	require.NoError(t, repo.Create(ctx, valid))

	// Create codec with error
	withError := createTestCodec(t, "http://error.com/stream")
	withError.ProbeError = "connection failed"
	require.NoError(t, repo.Create(ctx, withError))

	// Create expired codec
	expired := createTestCodec(t, "http://expired.com/stream")
	expiredTime := models.Time(time.Now().Add(-time.Hour))
	expired.ExpiresAt = &expiredTime
	require.NoError(t, repo.Create(ctx, expired))

	// Create codec without codecs (just error recorded)
	noCodecs := &models.LastKnownCodec{
		StreamURL: "http://nocodecs.com/stream",
		ProbedAt:  models.Now(),
	}
	require.NoError(t, repo.Create(ctx, noCodecs))

	count, err := repo.GetValidCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestLastKnownCodecRepo_GetStats(t *testing.T) {
	db := setupLastKnownCodecTestDB(t)
	repo := NewLastKnownCodecRepository(db)
	ctx := context.Background()

	// Create valid codecs with hits
	for i := 0; i < 3; i++ {
		codec := createTestCodec(t, "http://valid"+string(rune('1'+i))+".com/stream")
		codec.HitCount = int64(10 * (i + 1))
		require.NoError(t, repo.Create(ctx, codec))
	}

	// Create codec with error
	withError := &models.LastKnownCodec{
		StreamURL:  "http://error.com/stream",
		ProbedAt:   models.Now(),
		ProbeError: "failed",
		HitCount:   5,
	}
	require.NoError(t, repo.Create(ctx, withError))

	// Create expired codec
	expired := createTestCodec(t, "http://expired.com/stream")
	expiredTime := models.Time(time.Now().Add(-time.Hour))
	expired.ExpiresAt = &expiredTime
	expired.HitCount = 3
	require.NoError(t, repo.Create(ctx, expired))

	stats, err := repo.GetStats(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(5), stats.TotalEntries)
	assert.Equal(t, int64(3), stats.ValidEntries)
	assert.Equal(t, int64(1), stats.ExpiredEntries)
	assert.Equal(t, int64(1), stats.ErrorEntries)
	assert.Equal(t, int64(10+20+30+5+3), stats.TotalHits)
}
