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

func setupEpgProgramTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	// foreign_keys(ON) matches production (see internal/database/database.go).
	// Without it SQLite silently ignores the epg_programs -> epg_sources
	// constraint, and a delete ordering that fails in production passes here.
	db, err := gorm.Open(sqlite.Open(":memory:?_pragma=foreign_keys(ON)"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.EpgSource{}, &models.EpgProgram{})
	require.NoError(t, err)

	var fk int
	require.NoError(t, db.Raw("PRAGMA foreign_keys").Scan(&fk).Error)
	require.Equal(t, 1, fk, "foreign keys must be enforced or these tests do not reflect production")

	return db
}

func TestEpgProgramRepo_CreateBatch(t *testing.T) {
	db := setupEpgProgramTestDB(t)
	repo := NewEpgProgramRepository(db)
	ctx := context.Background()

	source := createTestEpgSource(t, db, "batch-epg")

	now := time.Now().Truncate(time.Second)
	programs := []*models.EpgProgram{
		{
			SourceID:  source.ID,
			ChannelID: "ch.1",
			Start:     now,
			Stop:      now.Add(30 * time.Minute),
			Title:     "Program A",
			Category:  "News",
		},
		{
			SourceID:  source.ID,
			ChannelID: "ch.1",
			Start:     now.Add(30 * time.Minute),
			Stop:      now.Add(60 * time.Minute),
			Title:     "Program B",
			Category:  "Sports",
		},
		{
			SourceID:  source.ID,
			ChannelID: "ch.2",
			Start:     now,
			Stop:      now.Add(60 * time.Minute),
			Title:     "Program C",
			Category:  "Movie",
		},
	}

	err := repo.CreateBatch(ctx, programs)
	require.NoError(t, err)

	// Verify all were created
	count, err := repo.CountBySourceID(ctx, source.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)

	// Verify each has an ID assigned
	for _, p := range programs {
		assert.False(t, p.ID.IsZero())
	}
}

func TestEpgProgramRepo_CreateBatch_Empty(t *testing.T) {
	db := setupEpgProgramTestDB(t)
	repo := NewEpgProgramRepository(db)
	ctx := context.Background()

	err := repo.CreateBatch(ctx, []*models.EpgProgram{})
	require.NoError(t, err)
}

func TestEpgProgramRepo_CreateBatch_Upsert(t *testing.T) {
	db := setupEpgProgramTestDB(t)
	repo := NewEpgProgramRepository(db)
	ctx := context.Background()

	source := createTestEpgSource(t, db, "upsert-epg")

	now := time.Now().Truncate(time.Second)

	// Initial insert
	programs := []*models.EpgProgram{
		{
			SourceID:  source.ID,
			ChannelID: "ch.1",
			Start:     now,
			Stop:      now.Add(30 * time.Minute),
			Title:     "Original Title",
		},
	}
	require.NoError(t, repo.CreateBatch(ctx, programs))

	count, err := repo.CountBySourceID(ctx, source.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Upsert with updated title (same source_id, channel_id, start)
	upsertPrograms := []*models.EpgProgram{
		{
			SourceID:  source.ID,
			ChannelID: "ch.1",
			Start:     now,
			Stop:      now.Add(45 * time.Minute),
			Title:     "Updated Title",
		},
	}
	require.NoError(t, repo.CreateBatch(ctx, upsertPrograms))

	// Should still be 1 record, not 2
	count, err = repo.CountBySourceID(ctx, source.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestEpgProgramRepo_GetBySourceID(t *testing.T) {
	db := setupEpgProgramTestDB(t)
	repo := NewEpgProgramRepository(db)
	ctx := context.Background()

	source := createTestEpgSource(t, db, "stream-epg")
	otherSource := createTestEpgSource(t, db, "other-epg")

	now := time.Now().Truncate(time.Second)

	// Create programs for source
	for i, title := range []string{"Show A", "Show B", "Show C"} {
		p := &models.EpgProgram{
			SourceID:  source.ID,
			ChannelID: "ch.1",
			Start:     now.Add(time.Duration(i) * time.Hour),
			Stop:      now.Add(time.Duration(i+1) * time.Hour),
			Title:     title,
		}
		require.NoError(t, repo.Create(ctx, p))
	}

	// Create program for other source
	other := &models.EpgProgram{
		SourceID:  otherSource.ID,
		ChannelID: "ch.1",
		Start:     now,
		Stop:      now.Add(time.Hour),
		Title:     "Other Show",
	}
	require.NoError(t, repo.Create(ctx, other))

	// Stream results via callback
	var collected []*models.EpgProgram
	err := repo.GetBySourceID(ctx, source.ID, func(p *models.EpgProgram) error {
		collected = append(collected, p)
		return nil
	})
	require.NoError(t, err)
	assert.Len(t, collected, 3)
}

func TestEpgProgramRepo_GetByChannelID(t *testing.T) {
	db := setupEpgProgramTestDB(t)
	repo := NewEpgProgramRepository(db)
	ctx := context.Background()

	source := createTestEpgSource(t, db, "channel-epg")

	now := time.Now().Truncate(time.Second)

	// Create programs spanning different time ranges
	programs := []*models.EpgProgram{
		{
			SourceID:  source.ID,
			ChannelID: "ch.1",
			Start:     now.Add(-2 * time.Hour),
			Stop:      now.Add(-1 * time.Hour),
			Title:     "Past Show",
		},
		{
			SourceID:  source.ID,
			ChannelID: "ch.1",
			Start:     now.Add(-30 * time.Minute),
			Stop:      now.Add(30 * time.Minute),
			Title:     "Current Show",
		},
		{
			SourceID:  source.ID,
			ChannelID: "ch.1",
			Start:     now.Add(1 * time.Hour),
			Stop:      now.Add(2 * time.Hour),
			Title:     "Future Show",
		},
		{
			SourceID:  source.ID,
			ChannelID: "ch.2",
			Start:     now,
			Stop:      now.Add(1 * time.Hour),
			Title:     "Other Channel",
		},
	}
	require.NoError(t, repo.CreateBatch(ctx, programs))

	t.Run("time range filter", func(t *testing.T) {
		// Query for programs overlapping with current hour
		start := now.Add(-1 * time.Hour)
		end := now.Add(1 * time.Hour)

		results, err := repo.GetByChannelID(ctx, "ch.1", start, end)
		require.NoError(t, err)
		// Should include "Current Show" (overlaps) but not "Past Show" (ended before start)
		// and not "Future Show" (starts after end)
		assert.Len(t, results, 1)
		assert.Equal(t, "Current Show", results[0].Title)
	})

	t.Run("wide time range", func(t *testing.T) {
		start := now.Add(-3 * time.Hour)
		end := now.Add(3 * time.Hour)

		results, err := repo.GetByChannelID(ctx, "ch.1", start, end)
		require.NoError(t, err)
		assert.Len(t, results, 3) // All ch.1 programs
	})

	t.Run("different channel", func(t *testing.T) {
		start := now.Add(-1 * time.Hour)
		end := now.Add(2 * time.Hour)

		results, err := repo.GetByChannelID(ctx, "ch.2", start, end)
		require.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "Other Channel", results[0].Title)
	})
}

func TestEpgProgramRepo_CountBySourceID(t *testing.T) {
	db := setupEpgProgramTestDB(t)
	repo := NewEpgProgramRepository(db)
	ctx := context.Background()

	source := createTestEpgSource(t, db, "count-epg")

	// Empty count
	count, err := repo.CountBySourceID(ctx, source.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	now := time.Now().Truncate(time.Second)
	for i := range 5 {
		p := &models.EpgProgram{
			SourceID:  source.ID,
			ChannelID: "ch.1",
			Start:     now.Add(time.Duration(i) * time.Hour),
			Stop:      now.Add(time.Duration(i+1) * time.Hour),
			Title:     "Program " + string(rune('A'+i)),
		}
		require.NoError(t, repo.Create(ctx, p))
	}

	count, err = repo.CountBySourceID(ctx, source.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(5), count)
}

func TestEpgProgramRepo_DeleteBySourceID(t *testing.T) {
	db := setupEpgProgramTestDB(t)
	repo := NewEpgProgramRepository(db)
	ctx := context.Background()

	source := createTestEpgSource(t, db, "del-epg")
	otherSource := createTestEpgSource(t, db, "keep-epg")

	now := time.Now().Truncate(time.Second)

	// Create programs for both sources
	for i := range 3 {
		p := &models.EpgProgram{
			SourceID:  source.ID,
			ChannelID: "ch.1",
			Start:     now.Add(time.Duration(i) * time.Hour),
			Stop:      now.Add(time.Duration(i+1) * time.Hour),
			Title:     "Del " + string(rune('A'+i)),
		}
		require.NoError(t, repo.Create(ctx, p))
	}

	keepProg := &models.EpgProgram{
		SourceID:  otherSource.ID,
		ChannelID: "ch.1",
		Start:     now,
		Stop:      now.Add(time.Hour),
		Title:     "Keep Me",
	}
	require.NoError(t, repo.Create(ctx, keepProg))

	// Delete all programs for source
	err := repo.DeleteBySourceID(ctx, source.ID)
	require.NoError(t, err)

	// Verify source programs are gone
	count, err := repo.CountBySourceID(ctx, source.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Verify other source programs remain
	count, err = repo.CountBySourceID(ctx, otherSource.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestEpgProgramRepo_DeleteExpired(t *testing.T) {
	db := setupEpgProgramTestDB(t)
	repo := NewEpgProgramRepository(db)
	ctx := context.Background()

	source := createTestEpgSource(t, db, "expire-epg")

	now := time.Now().Truncate(time.Second)

	// Create expired program (stopped 2 hours ago)
	expired := &models.EpgProgram{
		SourceID:  source.ID,
		ChannelID: "ch.1",
		Start:     now.Add(-3 * time.Hour),
		Stop:      now.Add(-2 * time.Hour),
		Title:     "Expired Show",
	}
	require.NoError(t, repo.Create(ctx, expired))

	// Create current program
	current := &models.EpgProgram{
		SourceID:  source.ID,
		ChannelID: "ch.1",
		Start:     now.Add(-30 * time.Minute),
		Stop:      now.Add(30 * time.Minute),
		Title:     "Current Show",
	}
	require.NoError(t, repo.Create(ctx, current))

	// Create future program
	future := &models.EpgProgram{
		SourceID:  source.ID,
		ChannelID: "ch.1",
		Start:     now.Add(1 * time.Hour),
		Stop:      now.Add(2 * time.Hour),
		Title:     "Future Show",
	}
	require.NoError(t, repo.Create(ctx, future))

	// Delete programs that stopped before 1 hour ago
	cutoff := now.Add(-1 * time.Hour)
	deleted, err := repo.DeleteExpired(ctx, cutoff)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	// Verify only expired program is gone
	count, err := repo.CountBySourceID(ctx, source.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)

	// Verify the expired one is gone
	found, err := repo.GetByID(ctx, expired.ID)
	require.NoError(t, err)
	assert.Nil(t, found)

	// Verify current and future remain
	found, err = repo.GetByID(ctx, current.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "Current Show", found.Title)

	found, err = repo.GetByID(ctx, future.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "Future Show", found.Title)
}

func TestEpgProgramRepo_GetByID(t *testing.T) {
	db := setupEpgProgramTestDB(t)
	repo := NewEpgProgramRepository(db)
	ctx := context.Background()

	source := createTestEpgSource(t, db, "getbyid-epg")

	now := time.Now().Truncate(time.Second)
	program := &models.EpgProgram{
		SourceID:    source.ID,
		ChannelID:   "ch.1",
		Start:       now,
		Stop:        now.Add(time.Hour),
		Title:       "Find This",
		Description: "A test program",
		Category:    "Drama",
	}
	require.NoError(t, repo.Create(ctx, program))

	t.Run("found", func(t *testing.T) {
		found, err := repo.GetByID(ctx, program.ID)
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, "Find This", found.Title)
		assert.Equal(t, "A test program", found.Description)
		assert.Equal(t, "Drama", found.Category)
	})

	t.Run("not found", func(t *testing.T) {
		found, err := repo.GetByID(ctx, models.NewULID())
		require.NoError(t, err)
		assert.Nil(t, found)
	})
}

func TestEpgProgramRepo_Delete(t *testing.T) {
	db := setupEpgProgramTestDB(t)
	repo := NewEpgProgramRepository(db)
	ctx := context.Background()

	source := createTestEpgSource(t, db, "single-del-epg")

	now := time.Now().Truncate(time.Second)
	program := &models.EpgProgram{
		SourceID:  source.ID,
		ChannelID: "ch.1",
		Start:     now,
		Stop:      now.Add(time.Hour),
		Title:     "To Delete",
	}
	require.NoError(t, repo.Create(ctx, program))

	err := repo.Delete(ctx, program.ID)
	require.NoError(t, err)

	found, err := repo.GetByID(ctx, program.ID)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestEpgProgramRepo_GetDistinctChannels(t *testing.T) {
	db := setupEpgProgramTestDB(t)
	repo := NewEpgProgramRepository(db)
	ctx := context.Background()

	source := createTestEpgSource(t, db, "distinct-epg")

	now := time.Now().Truncate(time.Second)

	// Create programs on multiple channels
	channelIDs := []string{"ch.3", "ch.1", "ch.2", "ch.1"} // ch.1 appears twice
	for i, chID := range channelIDs {
		p := &models.EpgProgram{
			SourceID:  source.ID,
			ChannelID: chID,
			Start:     now.Add(time.Duration(i) * time.Hour),
			Stop:      now.Add(time.Duration(i+1) * time.Hour),
			Title:     "Program " + chID,
		}
		require.NoError(t, repo.Create(ctx, p))
	}

	channels, err := repo.GetDistinctChannels(ctx)
	require.NoError(t, err)
	assert.Len(t, channels, 3) // Distinct: ch.1, ch.2, ch.3

	// Should be ordered ASC
	assert.Equal(t, "ch.1", channels[0])
	assert.Equal(t, "ch.2", channels[1])
	assert.Equal(t, "ch.3", channels[2])
}

func TestEpgProgramRepo_CreateInBatches(t *testing.T) {
	db := setupEpgProgramTestDB(t)
	repo := NewEpgProgramRepository(db)
	ctx := context.Background()

	source := createTestEpgSource(t, db, "inbatches-epg")

	now := time.Now().Truncate(time.Second)
	var programs []*models.EpgProgram
	for i := range 10 {
		p := &models.EpgProgram{
			SourceID:  source.ID,
			ChannelID: "ch.1",
			Start:     now.Add(time.Duration(i) * time.Hour),
			Stop:      now.Add(time.Duration(i+1) * time.Hour),
			Title:     "Batch Program " + string(rune('A'+i)),
		}
		programs = append(programs, p)
	}

	err := repo.CreateInBatches(ctx, programs, 3)
	require.NoError(t, err)

	count, err := repo.CountBySourceID(ctx, source.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(10), count)
}

// TestEpgProgramRepo_DeleteBySourceID_LargeSource exercises the batching path.
// The old implementation ran SELECT DISTINCT channel_id over every row for the
// source before deleting anything, which on a real source of over a million
// programs outlasted the HTTP request that triggered it.
func TestEpgProgramRepo_DeleteBySourceID_LargeSource(t *testing.T) {
	db := setupEpgProgramTestDB(t)
	repo := NewEpgProgramRepository(db)
	ctx := context.Background()

	target := createTestEpgSource(t, db, "large-epg")
	keep := createTestEpgSource(t, db, "keep-epg")

	// More rows than one delete batch, so the loop runs more than once.
	const programCount = epgProgramDeleteBatch + 250

	now := time.Now().Truncate(time.Second)
	programs := make([]*models.EpgProgram, 0, programCount+10)
	for i := range programCount {
		programs = append(programs, &models.EpgProgram{
			SourceID:  target.ID,
			ChannelID: "ch." + string(rune('a'+i%26)),
			Start:     now.Add(time.Duration(i) * time.Minute),
			Stop:      now.Add(time.Duration(i+1) * time.Minute),
			Title:     "Programme",
		})
	}
	for i := range 10 {
		programs = append(programs, &models.EpgProgram{
			SourceID:  keep.ID,
			ChannelID: "keep.1",
			Start:     now.Add(time.Duration(i) * time.Minute),
			Stop:      now.Add(time.Duration(i+1) * time.Minute),
			Title:     "Keep me",
		})
	}
	// Insert in chunks: a single CreateBatch of this many rows exceeds SQLite's
	// bound-variable limit.
	const insertChunk = 500
	for i := 0; i < len(programs); i += insertChunk {
		end := min(i+insertChunk, len(programs))
		require.NoError(t, repo.CreateBatch(ctx, programs[i:end]))
	}

	require.NoError(t, repo.DeleteBySourceID(ctx, target.ID))

	gone, err := repo.CountBySourceID(ctx, target.ID)
	require.NoError(t, err)
	assert.Zero(t, gone, "programs for the deleted source remain")

	// Deleting one source must not touch another's rows.
	kept, err := repo.CountBySourceID(ctx, keep.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(10), kept, "deleting one source removed another source's programs")
}

// TestEpgProgramRepo_DeleteBySourceID_Cancellation checks the loop honours
// cancellation: a sweep that cannot be stopped would hold the write lock through
// shutdown.
func TestEpgProgramRepo_DeleteBySourceID_Cancellation(t *testing.T) {
	db := setupEpgProgramTestDB(t)
	repo := NewEpgProgramRepository(db)

	source := createTestEpgSource(t, db, "cancel-epg")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := repo.DeleteBySourceID(ctx, source.ID)
	require.ErrorIs(t, err, context.Canceled)
}

// TestEpgProgramRepo_SourceDeleteOrdering pins the ordering the foreign key
// forces. Deleting the source row while its programs remain fails outright --
// which is exactly what shipped, because the test database did not enforce
// foreign keys and production does.
func TestEpgProgramRepo_SourceDeleteOrdering(t *testing.T) {
	db := setupEpgProgramTestDB(t)
	repo := NewEpgProgramRepository(db)
	ctx := context.Background()

	source := createTestEpgSource(t, db, "ordering-epg")

	now := time.Now().Truncate(time.Second)
	require.NoError(t, repo.CreateBatch(ctx, []*models.EpgProgram{{
		SourceID:  source.ID,
		ChannelID: "ch.1",
		Start:     now,
		Stop:      now.Add(time.Hour),
		Title:     "Programme",
	}}))

	// Parent first: rejected by the constraint.
	err := db.Unscoped().Delete(&models.EpgSource{}, "id = ?", source.ID).Error
	require.Error(t, err, "deleting a source with programs must violate the foreign key")
	require.Contains(t, err.Error(), "FOREIGN KEY")

	// Children first, then the parent: accepted.
	require.NoError(t, repo.DeleteBySourceID(ctx, source.ID))
	require.NoError(t, db.Unscoped().Delete(&models.EpgSource{}, "id = ?", source.ID).Error)
}

// TestEpgSourceRepo_SoftDeleteHidesButKeepsRow covers the mechanism the delete
// relies on: the source must vanish from ordinary queries immediately while its
// row survives to satisfy the foreign key until the programs are gone.
func TestEpgSourceRepo_SoftDeleteHidesButKeepsRow(t *testing.T) {
	db := setupEpgProgramTestDB(t)
	programRepo := NewEpgProgramRepository(db)
	sourceRepo := NewEpgSourceRepository(db)
	ctx := context.Background()

	source := createTestEpgSource(t, db, "soft-epg")

	now := time.Now().Truncate(time.Second)
	require.NoError(t, programRepo.CreateBatch(ctx, []*models.EpgProgram{{
		SourceID:  source.ID,
		ChannelID: "ch.1",
		Start:     now,
		Stop:      now.Add(time.Hour),
		Title:     "Programme",
	}}))

	require.NoError(t, sourceRepo.SoftDelete(ctx, source.ID))

	// Invisible to the UI... (GetByID reports a missing row as nil, not an error)
	got, err := sourceRepo.GetByID(ctx, source.ID)
	require.NoError(t, err)
	require.Nil(t, got, "a soft-deleted source is still visible")

	all, err := sourceRepo.GetAll(ctx)
	require.NoError(t, err)
	for _, listed := range all {
		require.NotEqual(t, source.ID, listed.ID, "soft-deleted source still listed")
	}

	// ...but still present, so the programs' foreign key holds.
	pending, err := sourceRepo.ListSoftDeleted(ctx)
	require.NoError(t, err)
	require.Contains(t, pending, source.ID, "unfinished deletion not reported for recovery")

	// And the deletion can be completed in the correct order.
	require.NoError(t, programRepo.DeleteBySourceID(ctx, source.ID))
	require.NoError(t, sourceRepo.Delete(ctx, source.ID))

	pending, err = sourceRepo.ListSoftDeleted(ctx)
	require.NoError(t, err)
	require.NotContains(t, pending, source.ID)
}
