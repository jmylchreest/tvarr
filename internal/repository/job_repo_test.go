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

func setupJobTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	err = db.AutoMigrate(&models.Job{}, &models.JobHistory{})
	require.NoError(t, err)

	return db
}

func TestJobRepo_Create(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepository(db)
	ctx := context.Background()

	job := &models.Job{
		Type:       models.JobTypeStreamIngestion,
		TargetID:   models.NewULID(),
		TargetName: "Test Source",
		Status:     models.JobStatusPending,
	}

	err := repo.Create(ctx, job)
	require.NoError(t, err)
	assert.False(t, job.ID.IsZero())

	// Verify job was created
	found, err := repo.GetByID(ctx, job.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, job.Type, found.Type)
	assert.Equal(t, job.TargetName, found.TargetName)
}

func TestJobRepo_GetByID(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepository(db)
	ctx := context.Background()

	// Create a job
	job := &models.Job{
		Type:       models.JobTypeProxyGeneration,
		TargetID:   models.NewULID(),
		TargetName: "Test Proxy",
		Status:     models.JobStatusPending,
	}
	require.NoError(t, repo.Create(ctx, job))

	t.Run("existing job", func(t *testing.T) {
		found, err := repo.GetByID(ctx, job.ID)
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, job.ID, found.ID)
	})

	t.Run("non-existent job", func(t *testing.T) {
		found, err := repo.GetByID(ctx, models.NewULID())
		require.NoError(t, err)
		assert.Nil(t, found)
	})
}

func TestJobRepo_GetAll(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepository(db)
	ctx := context.Background()

	// Create multiple jobs with different priorities
	jobs := []*models.Job{
		{Type: models.JobTypeStreamIngestion, TargetName: "Source 1", Priority: 1, Status: models.JobStatusPending},
		{Type: models.JobTypeEpgIngestion, TargetName: "EPG 1", Priority: 5, Status: models.JobStatusPending},
		{Type: models.JobTypeProxyGeneration, TargetName: "Proxy 1", Priority: 3, Status: models.JobStatusRunning},
	}

	for _, job := range jobs {
		job.TargetID = models.NewULID()
		require.NoError(t, repo.Create(ctx, job))
	}

	all, err := repo.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	// Should be ordered by priority DESC
	assert.Equal(t, "EPG 1", all[0].TargetName)
	assert.Equal(t, "Proxy 1", all[1].TargetName)
	assert.Equal(t, "Source 1", all[2].TargetName)
}

func TestJobRepo_GetPending(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepository(db)
	ctx := context.Background()

	now := models.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	// Create jobs with different statuses and scheduled times
	jobs := []*models.Job{
		{Type: models.JobTypeStreamIngestion, TargetName: "Pending", Status: models.JobStatusPending},
		{Type: models.JobTypeEpgIngestion, TargetName: "Scheduled Past", Status: models.JobStatusScheduled, NextRunAt: &past},
		{Type: models.JobTypeProxyGeneration, TargetName: "Scheduled Future", Status: models.JobStatusScheduled, NextRunAt: &future},
		{Type: models.JobTypeStreamIngestion, TargetName: "Running", Status: models.JobStatusRunning},
		{Type: models.JobTypeStreamIngestion, TargetName: "Completed", Status: models.JobStatusCompleted},
	}

	for _, job := range jobs {
		job.TargetID = models.NewULID()
		require.NoError(t, repo.Create(ctx, job))
	}

	pending, err := repo.GetPending(ctx)
	require.NoError(t, err)

	// Should only return pending and scheduled (past)
	assert.Len(t, pending, 2)

	names := make([]string, len(pending))
	for i, j := range pending {
		names[i] = j.TargetName
	}
	assert.Contains(t, names, "Pending")
	assert.Contains(t, names, "Scheduled Past")
}

func TestJobRepo_GetByStatus(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepository(db)
	ctx := context.Background()

	jobs := []*models.Job{
		{Type: models.JobTypeStreamIngestion, TargetName: "Running 1", Status: models.JobStatusRunning},
		{Type: models.JobTypeEpgIngestion, TargetName: "Running 2", Status: models.JobStatusRunning},
		{Type: models.JobTypeProxyGeneration, TargetName: "Pending", Status: models.JobStatusPending},
	}

	for _, job := range jobs {
		job.TargetID = models.NewULID()
		require.NoError(t, repo.Create(ctx, job))
	}

	running, err := repo.GetByStatus(ctx, models.JobStatusRunning)
	require.NoError(t, err)
	assert.Len(t, running, 2)
}

func TestJobRepo_GetByType(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepository(db)
	ctx := context.Background()

	jobs := []*models.Job{
		{Type: models.JobTypeStreamIngestion, TargetName: "Source 1", Status: models.JobStatusPending},
		{Type: models.JobTypeStreamIngestion, TargetName: "Source 2", Status: models.JobStatusRunning},
		{Type: models.JobTypeEpgIngestion, TargetName: "EPG 1", Status: models.JobStatusPending},
	}

	for _, job := range jobs {
		job.TargetID = models.NewULID()
		require.NoError(t, repo.Create(ctx, job))
	}

	streamJobs, err := repo.GetByType(ctx, models.JobTypeStreamIngestion)
	require.NoError(t, err)
	assert.Len(t, streamJobs, 2)
}

func TestJobRepo_GetByTargetID(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepository(db)
	ctx := context.Background()

	targetID := models.NewULID()

	jobs := []*models.Job{
		{Type: models.JobTypeStreamIngestion, TargetID: targetID, TargetName: "Target Job 1", Status: models.JobStatusCompleted},
		{Type: models.JobTypeStreamIngestion, TargetID: targetID, TargetName: "Target Job 2", Status: models.JobStatusPending},
		{Type: models.JobTypeStreamIngestion, TargetID: models.NewULID(), TargetName: "Other Job", Status: models.JobStatusPending},
	}

	for _, job := range jobs {
		require.NoError(t, repo.Create(ctx, job))
	}

	targetJobs, err := repo.GetByTargetID(ctx, targetID)
	require.NoError(t, err)
	assert.Len(t, targetJobs, 2)
}

func TestJobRepo_Update(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepository(db)
	ctx := context.Background()

	job := &models.Job{
		Type:       models.JobTypeStreamIngestion,
		TargetID:   models.NewULID(),
		TargetName: "Test Source",
		Status:     models.JobStatusPending,
	}
	require.NoError(t, repo.Create(ctx, job))

	// Update the job
	job.Status = models.JobStatusRunning
	job.LockedBy = "worker-1"
	err := repo.Update(ctx, job)
	require.NoError(t, err)

	// Verify update
	found, err := repo.GetByID(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, models.JobStatusRunning, found.Status)
	assert.Equal(t, "worker-1", found.LockedBy)
}

func TestJobRepo_Delete(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepository(db)
	ctx := context.Background()

	job := &models.Job{
		Type:     models.JobTypeStreamIngestion,
		TargetID: models.NewULID(),
		Status:   models.JobStatusPending,
	}
	require.NoError(t, repo.Create(ctx, job))

	err := repo.Delete(ctx, job.ID)
	require.NoError(t, err)

	// Verify deletion
	found, err := repo.GetByID(ctx, job.ID)
	require.NoError(t, err)
	assert.Nil(t, found)
}

func TestJobRepo_DeleteCompleted(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepository(db)
	ctx := context.Background()

	now := models.Now()
	oldTime := now.Add(-48 * time.Hour)
	recentTime := now.Add(-time.Hour)

	jobs := []*models.Job{
		{Type: models.JobTypeStreamIngestion, TargetID: models.NewULID(), Status: models.JobStatusCompleted, CompletedAt: &oldTime},
		{Type: models.JobTypeEpgIngestion, TargetID: models.NewULID(), Status: models.JobStatusFailed, CompletedAt: &oldTime},
		{Type: models.JobTypeProxyGeneration, TargetID: models.NewULID(), Status: models.JobStatusCompleted, CompletedAt: &recentTime},
		{Type: models.JobTypeStreamIngestion, TargetID: models.NewULID(), Status: models.JobStatusPending},
	}

	for _, job := range jobs {
		require.NoError(t, repo.Create(ctx, job))
	}

	// Delete jobs completed before 24 hours ago
	cutoff := now.Add(-24 * time.Hour)
	deleted, err := repo.DeleteCompleted(ctx, cutoff)
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted)

	// Verify remaining jobs
	all, err := repo.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestJobRepo_FindDuplicatePending(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepository(db)
	ctx := context.Background()

	targetID := models.NewULID()

	// Create a pending job for this target
	existing := &models.Job{
		Type:     models.JobTypeStreamIngestion,
		TargetID: targetID,
		Status:   models.JobStatusPending,
	}
	require.NoError(t, repo.Create(ctx, existing))

	t.Run("finds duplicate", func(t *testing.T) {
		found, err := repo.FindDuplicatePending(ctx, models.JobTypeStreamIngestion, targetID)
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, existing.ID, found.ID)
	})

	t.Run("different type no duplicate", func(t *testing.T) {
		found, err := repo.FindDuplicatePending(ctx, models.JobTypeEpgIngestion, targetID)
		require.NoError(t, err)
		assert.Nil(t, found)
	})

	t.Run("different target no duplicate", func(t *testing.T) {
		found, err := repo.FindDuplicatePending(ctx, models.JobTypeStreamIngestion, models.NewULID())
		require.NoError(t, err)
		assert.Nil(t, found)
	})
}

func TestJobRepo_ReleaseJob(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepository(db)
	ctx := context.Background()

	now := models.Now()
	job := &models.Job{
		Type:     models.JobTypeStreamIngestion,
		TargetID: models.NewULID(),
		Status:   models.JobStatusRunning,
		LockedBy: "worker-1",
		LockedAt: &now,
	}
	require.NoError(t, repo.Create(ctx, job))

	err := repo.ReleaseJob(ctx, job.ID)
	require.NoError(t, err)

	// Verify release
	found, err := repo.GetByID(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, models.JobStatusPending, found.Status)
	assert.Empty(t, found.LockedBy)
	assert.Nil(t, found.LockedAt)
}

func TestJobRepo_CreateHistory(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepository(db)
	ctx := context.Background()

	now := models.Now()
	history := &models.JobHistory{
		JobID:         models.NewULID(),
		Type:          models.JobTypeStreamIngestion,
		TargetID:      models.NewULID(),
		TargetName:    "Test Source",
		Status:        models.JobStatusCompleted,
		StartedAt:     &now,
		CompletedAt:   &now,
		AttemptNumber: 1,
		Result:        "ingested 100 channels",
	}

	err := repo.CreateHistory(ctx, history)
	require.NoError(t, err)
	assert.False(t, history.ID.IsZero())
}

func TestJobRepo_GetHistory(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepository(db)
	ctx := context.Background()

	now := models.Now()
	histories := []*models.JobHistory{
		{JobID: models.NewULID(), Type: models.JobTypeStreamIngestion, Status: models.JobStatusCompleted, CompletedAt: &now},
		{JobID: models.NewULID(), Type: models.JobTypeStreamIngestion, Status: models.JobStatusFailed, CompletedAt: &now},
		{JobID: models.NewULID(), Type: models.JobTypeEpgIngestion, Status: models.JobStatusCompleted, CompletedAt: &now},
	}

	for _, h := range histories {
		h.TargetID = models.NewULID()
		require.NoError(t, repo.CreateHistory(ctx, h))
	}

	t.Run("all history", func(t *testing.T) {
		results, total, err := repo.GetHistory(ctx, nil, 0, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(3), total)
		assert.Len(t, results, 3)
	})

	t.Run("filtered by type", func(t *testing.T) {
		jobType := models.JobTypeStreamIngestion
		results, total, err := repo.GetHistory(ctx, &jobType, 0, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(2), total)
		assert.Len(t, results, 2)
	})

	t.Run("with pagination", func(t *testing.T) {
		results, total, err := repo.GetHistory(ctx, nil, 0, 2)
		require.NoError(t, err)
		assert.Equal(t, int64(3), total)
		assert.Len(t, results, 2)
	})
}

func TestJobRepo_DeleteHistory(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepository(db)
	ctx := context.Background()

	now := models.Now()
	oldTime := now.Add(-48 * time.Hour)
	recentTime := now.Add(-time.Hour)

	histories := []*models.JobHistory{
		{JobID: models.NewULID(), Type: models.JobTypeStreamIngestion, TargetID: models.NewULID(), Status: models.JobStatusCompleted, CompletedAt: &oldTime},
		{JobID: models.NewULID(), Type: models.JobTypeEpgIngestion, TargetID: models.NewULID(), Status: models.JobStatusCompleted, CompletedAt: &recentTime},
	}

	for _, h := range histories {
		require.NoError(t, repo.CreateHistory(ctx, h))
	}

	// Delete history older than 24 hours
	cutoff := now.Add(-24 * time.Hour)
	deleted, err := repo.DeleteHistory(ctx, cutoff)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	// Verify remaining
	results, total, err := repo.GetHistory(ctx, nil, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, results, 1)
}

func TestJobRepo_AcquireJob(t *testing.T) {
	db := setupJobTestDB(t)
	repo := NewJobRepository(db)
	ctx := context.Background()

	// Create pending jobs
	job1 := &models.Job{
		Type:     models.JobTypeStreamIngestion,
		TargetID: models.NewULID(),
		Status:   models.JobStatusPending,
		Priority: 1,
	}
	job2 := &models.Job{
		Type:     models.JobTypeStreamIngestion,
		TargetID: models.NewULID(),
		Status:   models.JobStatusPending,
		Priority: 5, // Higher priority
	}
	require.NoError(t, repo.Create(ctx, job1))
	require.NoError(t, repo.Create(ctx, job2))

	// Acquire first job - should get highest priority
	acquired, err := repo.AcquireJob(ctx, "worker-1")
	require.NoError(t, err)
	require.NotNil(t, acquired)
	assert.Equal(t, job2.ID, acquired.ID)
	assert.Equal(t, models.JobStatusRunning, acquired.Status)
	assert.Equal(t, "worker-1", acquired.LockedBy)
	assert.Equal(t, 1, acquired.AttemptCount)

	// Acquire second job
	acquired2, err := repo.AcquireJob(ctx, "worker-2")
	require.NoError(t, err)
	require.NotNil(t, acquired2)
	assert.Equal(t, job1.ID, acquired2.ID)

	// No more jobs available
	acquired3, err := repo.AcquireJob(ctx, "worker-3")
	require.NoError(t, err)
	assert.Nil(t, acquired3)
}
