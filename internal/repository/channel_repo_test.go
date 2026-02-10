package repository

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupChannelTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.StreamSource{}, &models.Channel{})
	require.NoError(t, err)

	return db
}

// createTestSource creates a StreamSource for use as a foreign key in channel tests.
func createTestSource(t *testing.T, db *gorm.DB, name string) *models.StreamSource {
	t.Helper()
	source := &models.StreamSource{
		Name:    name,
		Type:    models.SourceTypeM3U,
		URL:     "http://example.com/" + name + ".m3u",
		Enabled: models.BoolPtr(true),
	}
	err := db.Create(source).Error
	require.NoError(t, err)
	return source
}

func TestChannelRepo_Create(t *testing.T) {
	db := setupChannelTestDB(t)
	repo := NewChannelRepository(db)
	ctx := context.Background()

	source := createTestSource(t, db, "test-source")

	channel := &models.Channel{
		SourceID:    source.ID,
		ExtID:       "ch-001",
		ChannelName: "Test Channel",
		StreamURL:   "http://example.com/stream/1",
		GroupTitle:  "News",
	}

	err := repo.Create(ctx, channel)
	require.NoError(t, err)
	assert.False(t, channel.ID.IsZero())

	// Verify retrieval by ID
	found, err := repo.GetByID(ctx, channel.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "Test Channel", found.ChannelName)
	assert.Equal(t, source.ID, found.SourceID)
	assert.Equal(t, "ch-001", found.ExtID)
}

func TestChannelRepo_GetByID_NotFound(t *testing.T) {
	db := setupChannelTestDB(t)
	repo := NewChannelRepository(db)
	ctx := context.Background()

	found, err := repo.GetByID(ctx, models.NewULID())
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestChannelRepo_CreateBatch(t *testing.T) {
	db := setupChannelTestDB(t)
	repo := NewChannelRepository(db)
	ctx := context.Background()

	source := createTestSource(t, db, "batch-source")

	channels := []*models.Channel{
		{
			SourceID:    source.ID,
			ExtID:       "batch-1",
			ChannelName: "Channel A",
			StreamURL:   "http://example.com/stream/a",
		},
		{
			SourceID:    source.ID,
			ExtID:       "batch-2",
			ChannelName: "Channel B",
			StreamURL:   "http://example.com/stream/b",
		},
		{
			SourceID:    source.ID,
			ExtID:       "batch-3",
			ChannelName: "Channel C",
			StreamURL:   "http://example.com/stream/c",
		},
	}

	err := repo.CreateBatch(ctx, channels)
	require.NoError(t, err)

	// Verify all were created
	count, err := repo.CountBySourceID(ctx, source.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)

	// Verify each has an ID assigned
	for _, ch := range channels {
		assert.False(t, ch.ID.IsZero())
	}
}

func TestChannelRepo_CreateBatch_Empty(t *testing.T) {
	db := setupChannelTestDB(t)
	repo := NewChannelRepository(db)
	ctx := context.Background()

	err := repo.CreateBatch(ctx, []*models.Channel{})
	require.NoError(t, err)
}

func TestChannelRepo_GetBySourceID(t *testing.T) {
	db := setupChannelTestDB(t)
	repo := NewChannelRepository(db)
	ctx := context.Background()

	source := createTestSource(t, db, "source-1")
	otherSource := createTestSource(t, db, "source-2")

	// Create channels for source
	for i, name := range []string{"Alpha", "Beta", "Gamma"} {
		ch := &models.Channel{
			SourceID:      source.ID,
			ExtID:         "ext-" + name,
			ChannelName:   name,
			StreamURL:     "http://example.com/stream/" + name,
			GroupTitle:    "Group",
			ChannelNumber: i + 1,
		}
		require.NoError(t, repo.Create(ctx, ch))
	}

	// Create channel for other source (should not appear)
	other := &models.Channel{
		SourceID:    otherSource.ID,
		ExtID:       "other-1",
		ChannelName: "Other",
		StreamURL:   "http://example.com/stream/other",
	}
	require.NoError(t, repo.Create(ctx, other))

	// Stream results via callback
	var collected []*models.Channel
	err := repo.GetBySourceID(ctx, source.ID, func(ch *models.Channel) error {
		collected = append(collected, ch)
		return nil
	})
	require.NoError(t, err)
	assert.Len(t, collected, 3)
}

func TestChannelRepo_CountBySourceID(t *testing.T) {
	db := setupChannelTestDB(t)
	repo := NewChannelRepository(db)
	ctx := context.Background()

	source := createTestSource(t, db, "count-source")

	// Empty count
	count, err := repo.CountBySourceID(ctx, source.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Add channels
	for _, name := range []string{"A", "B"} {
		ch := &models.Channel{
			SourceID:    source.ID,
			ExtID:       "count-" + name,
			ChannelName: "Channel " + name,
			StreamURL:   "http://example.com/stream/" + name,
		}
		require.NoError(t, repo.Create(ctx, ch))
	}

	count, err = repo.CountBySourceID(ctx, source.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func TestChannelRepo_GetByExtID(t *testing.T) {
	db := setupChannelTestDB(t)
	repo := NewChannelRepository(db)
	ctx := context.Background()

	source := createTestSource(t, db, "extid-source")

	ch := &models.Channel{
		SourceID:    source.ID,
		ExtID:       "unique-ext-123",
		ChannelName: "Ext Channel",
		StreamURL:   "http://example.com/stream/ext",
	}
	require.NoError(t, repo.Create(ctx, ch))

	// Find by ext ID
	found, err := repo.GetByExtID(ctx, source.ID, "unique-ext-123")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, ch.ID, found.ID)
	assert.Equal(t, "Ext Channel", found.ChannelName)

	// Not found with wrong ext ID
	notFound, err := repo.GetByExtID(ctx, source.ID, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, notFound)

	// Not found with wrong source ID
	notFound, err = repo.GetByExtID(ctx, models.NewULID(), "unique-ext-123")
	require.NoError(t, err)
	assert.Nil(t, notFound)
}

func TestChannelRepo_DeleteBySourceID(t *testing.T) {
	db := setupChannelTestDB(t)
	repo := NewChannelRepository(db)
	ctx := context.Background()

	source := createTestSource(t, db, "del-source")
	otherSource := createTestSource(t, db, "keep-source")

	// Create channels for both sources
	for _, name := range []string{"A", "B", "C"} {
		ch := &models.Channel{
			SourceID:    source.ID,
			ExtID:       "del-" + name,
			ChannelName: "Del " + name,
			StreamURL:   "http://example.com/stream/del/" + name,
		}
		require.NoError(t, repo.Create(ctx, ch))
	}

	keepCh := &models.Channel{
		SourceID:    otherSource.ID,
		ExtID:       "keep-1",
		ChannelName: "Keep Me",
		StreamURL:   "http://example.com/stream/keep",
	}
	require.NoError(t, repo.Create(ctx, keepCh))

	// Delete all channels for source
	err := repo.DeleteBySourceID(ctx, source.ID)
	require.NoError(t, err)

	// Verify source channels are gone
	count, err := repo.CountBySourceID(ctx, source.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Verify other source channels remain
	count, err = repo.CountBySourceID(ctx, otherSource.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestChannelRepo_DeleteStaleBySourceID(t *testing.T) {
	db := setupChannelTestDB(t)
	repo := NewChannelRepository(db)
	ctx := context.Background()

	source := createTestSource(t, db, "stale-source")

	// Create channels - they'll have current timestamps
	freshCh := &models.Channel{
		SourceID:    source.ID,
		ExtID:       "fresh-1",
		ChannelName: "Fresh Channel",
		StreamURL:   "http://example.com/stream/fresh",
	}
	require.NoError(t, repo.Create(ctx, freshCh))

	staleCh := &models.Channel{
		SourceID:    source.ID,
		ExtID:       "stale-1",
		ChannelName: "Stale Channel",
		StreamURL:   "http://example.com/stream/stale",
	}
	require.NoError(t, repo.Create(ctx, staleCh))

	// Manually backdate the stale channel's updated_at
	oldTime := time.Now().Add(-2 * time.Hour)
	require.NoError(t, db.Model(staleCh).UpdateColumn("updated_at", oldTime).Error)

	// Delete channels older than 1 hour ago
	cutoff := time.Now().Add(-1 * time.Hour)
	deleted, err := repo.DeleteStaleBySourceID(ctx, source.ID, cutoff)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	// Verify only fresh channel remains
	count, err := repo.CountBySourceID(ctx, source.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	found, err := repo.GetByID(ctx, freshCh.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "Fresh Channel", found.ChannelName)

	// Verify stale channel is gone
	found, err = repo.GetByID(ctx, staleCh.ID)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestChannelRepo_GetDistinctFieldValues(t *testing.T) {
	db := setupChannelTestDB(t)
	repo := NewChannelRepository(db)
	ctx := context.Background()

	source := createTestSource(t, db, "field-source")

	// Create channels with various group titles
	groups := []struct {
		name  string
		group string
	}{
		{"Ch 1", "Sports"},
		{"Ch 2", "Sports"},
		{"Ch 3", "Sports"},
		{"Ch 4", "News"},
		{"Ch 5", "News"},
		{"Ch 6", "Entertainment"},
	}

	for i, g := range groups {
		ch := &models.Channel{
			SourceID:      source.ID,
			ExtID:         "field-" + g.name,
			ChannelName:   g.name,
			StreamURL:     "http://example.com/stream/" + g.name,
			GroupTitle:    g.group,
			ChannelNumber: i + 1,
		}
		require.NoError(t, repo.Create(ctx, ch))
	}

	t.Run("group_title distinct values", func(t *testing.T) {
		results, err := repo.GetDistinctFieldValues(ctx, "group_title", "", 20)
		require.NoError(t, err)
		assert.Len(t, results, 3)

		// Ordered by count DESC
		assert.Equal(t, "Sports", results[0].Value)
		assert.Equal(t, int64(3), results[0].Count)
		assert.Equal(t, "News", results[1].Value)
		assert.Equal(t, int64(2), results[1].Count)
		assert.Equal(t, "Entertainment", results[2].Value)
		assert.Equal(t, int64(1), results[2].Count)
	})

	t.Run("with search query", func(t *testing.T) {
		results, err := repo.GetDistinctFieldValues(ctx, "group_title", "sport", 20)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "Sports", results[0].Value)
	})

	t.Run("with limit", func(t *testing.T) {
		results, err := repo.GetDistinctFieldValues(ctx, "group_title", "", 2)
		require.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("invalid field name", func(t *testing.T) {
		_, err := repo.GetDistinctFieldValues(ctx, "invalid_field", "", 20)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid field name")
	})
}

func TestChannelRepo_Update(t *testing.T) {
	db := setupChannelTestDB(t)
	repo := NewChannelRepository(db)
	ctx := context.Background()

	source := createTestSource(t, db, "update-source")

	ch := &models.Channel{
		SourceID:    source.ID,
		ExtID:       "upd-1",
		ChannelName: "Original Name",
		StreamURL:   "http://example.com/stream/original",
		GroupTitle:  "OldGroup",
	}
	require.NoError(t, repo.Create(ctx, ch))

	// Update
	ch.ChannelName = "Updated Name"
	ch.GroupTitle = "NewGroup"
	err := repo.Update(ctx, ch)
	require.NoError(t, err)

	// Verify
	found, err := repo.GetByID(ctx, ch.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", found.ChannelName)
	assert.Equal(t, "NewGroup", found.GroupTitle)
}

func TestChannelRepo_Delete(t *testing.T) {
	db := setupChannelTestDB(t)
	repo := NewChannelRepository(db)
	ctx := context.Background()

	source := createTestSource(t, db, "delete-source")

	ch := &models.Channel{
		SourceID:    source.ID,
		ExtID:       "del-1",
		ChannelName: "To Delete",
		StreamURL:   "http://example.com/stream/delete",
	}
	require.NoError(t, repo.Create(ctx, ch))

	err := repo.Delete(ctx, ch.ID)
	require.NoError(t, err)

	found, err := repo.GetByID(ctx, ch.ID)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestChannelRepo_GetByIDWithSource(t *testing.T) {
	db := setupChannelTestDB(t)
	repo := NewChannelRepository(db)
	ctx := context.Background()

	source := createTestSource(t, db, "preload-source")

	ch := &models.Channel{
		SourceID:    source.ID,
		ExtID:       "preload-1",
		ChannelName: "Preloaded",
		StreamURL:   "http://example.com/stream/preload",
	}
	require.NoError(t, repo.Create(ctx, ch))

	found, err := repo.GetByIDWithSource(ctx, ch.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	require.NotNil(t, found.Source)
	assert.Equal(t, "preload-source", found.Source.Name)
}

func TestChannelRepo_UpsertBatch(t *testing.T) {
	db := setupChannelTestDB(t)
	repo := NewChannelRepository(db)
	ctx := context.Background()

	source := createTestSource(t, db, "upsert-source")

	// Create initial channels
	channels := []*models.Channel{
		{
			SourceID:    source.ID,
			ExtID:       "upsert-1",
			ChannelName: "Original Name",
			StreamURL:   "http://example.com/stream/1",
			GroupTitle:  "Group1",
		},
		{
			SourceID:    source.ID,
			ExtID:       "upsert-2",
			ChannelName: "Channel 2",
			StreamURL:   "http://example.com/stream/2",
		},
	}
	require.NoError(t, repo.UpsertBatch(ctx, channels))

	count, err := repo.CountBySourceID(ctx, source.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	// Upsert with updated name for existing + new channel
	upsertChannels := []*models.Channel{
		{
			SourceID:    source.ID,
			ExtID:       "upsert-1",
			ChannelName: "Updated Name",
			StreamURL:   "http://example.com/stream/1-updated",
			GroupTitle:  "Group1-Updated",
		},
		{
			SourceID:    source.ID,
			ExtID:       "upsert-3",
			ChannelName: "Channel 3",
			StreamURL:   "http://example.com/stream/3",
		},
	}
	require.NoError(t, repo.UpsertBatch(ctx, upsertChannels))

	count, err = repo.CountBySourceID(ctx, source.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)

	// Verify the updated channel
	found, err := repo.GetByExtID(ctx, source.ID, "upsert-1")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "Updated Name", found.ChannelName)
	assert.Equal(t, "Group1-Updated", found.GroupTitle)
}

func TestChannelRepo_GetAllStreaming(t *testing.T) {
	db := setupChannelTestDB(t)
	repo := NewChannelRepository(db)
	ctx := context.Background()

	source1 := createTestSource(t, db, "stream-source-1")
	source2 := createTestSource(t, db, "stream-source-2")

	for _, name := range []string{"A", "B"} {
		ch := &models.Channel{
			SourceID:    source1.ID,
			ExtID:       "all-s1-" + name,
			ChannelName: "S1 " + name,
			StreamURL:   "http://example.com/s1/" + name,
		}
		require.NoError(t, repo.Create(ctx, ch))
	}

	ch := &models.Channel{
		SourceID:    source2.ID,
		ExtID:       "all-s2-1",
		ChannelName: "S2 Channel",
		StreamURL:   "http://example.com/s2/1",
	}
	require.NoError(t, repo.Create(ctx, ch))

	var collected []*models.Channel
	err := repo.GetAllStreaming(ctx, func(ch *models.Channel) error {
		collected = append(collected, ch)
		return nil
	})
	require.NoError(t, err)
	assert.Len(t, collected, 3)
}

func TestChannelRepo_Transaction(t *testing.T) {
	db := setupChannelTestDB(t)
	repo := NewChannelRepository(db)
	ctx := context.Background()

	source := createTestSource(t, db, "tx-source")

	// Successful transaction
	err := repo.Transaction(ctx, func(txRepo ChannelRepository) error {
		ch := &models.Channel{
			SourceID:    source.ID,
			ExtID:       "tx-1",
			ChannelName: "TX Channel",
			StreamURL:   "http://example.com/tx/1",
		}
		return txRepo.Create(ctx, ch)
	})
	require.NoError(t, err)

	count, err := repo.CountBySourceID(ctx, source.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}
