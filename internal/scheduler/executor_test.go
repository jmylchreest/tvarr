package scheduler

import (
	"context"
	"errors"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockJobHandler implements JobHandler for testing.
type mockJobHandler struct {
	executeResult string
	executeErr    error
	executeCalled bool
}

func (m *mockJobHandler) Execute(ctx context.Context, job *models.Job) (string, error) {
	m.executeCalled = true
	return m.executeResult, m.executeErr
}

// mockSourceService implements SourceIngestService for testing.
type mockSourceService struct {
	ingestErr    error
	ingestCalled bool
}

func (m *mockSourceService) Ingest(ctx context.Context, sourceID models.ULID) error {
	m.ingestCalled = true
	return m.ingestErr
}

// mockEpgService implements EpgIngestService for testing.
type mockEpgService struct {
	ingestErr    error
	ingestCalled bool
}

func (m *mockEpgService) Ingest(ctx context.Context, sourceID models.ULID) error {
	m.ingestCalled = true
	return m.ingestErr
}

// mockProxyGenerateFunc creates a mock proxy generate function for testing.
func mockProxyGenerateFunc(channelCount, programCount int, err error) ProxyGenerateFunc {
	return func(ctx context.Context, proxyID models.ULID) (*ProxyGenerateResult, error) {
		if err != nil {
			return nil, err
		}
		return &ProxyGenerateResult{
			ChannelCount: channelCount,
			ProgramCount: programCount,
		}, nil
	}
}

func TestExecutor_RegisterHandler(t *testing.T) {
	jobRepo := newMockJobRepo()
	executor := NewExecutor(jobRepo)

	handler := &mockJobHandler{}
	executor.RegisterHandler(models.JobTypeStreamIngestion, handler)

	// Handler should be registered
	assert.NotNil(t, executor.handlers[models.JobTypeStreamIngestion])
}

func TestExecutor_Execute_Success(t *testing.T) {
	jobRepo := newMockJobRepo()
	executor := NewExecutor(jobRepo)

	handler := &mockJobHandler{executeResult: "success"}
	executor.RegisterHandler(models.JobTypeStreamIngestion, handler)

	job := &models.Job{
		Type:       models.JobTypeStreamIngestion,
		TargetID:   models.NewULID(),
		TargetName: "Test Source",
		Status:     models.JobStatusRunning,
	}
	job.ID = models.NewULID()
	jobRepo.jobs[job.ID] = job

	ctx := context.Background()
	err := executor.Execute(ctx, job)
	require.NoError(t, err)

	assert.True(t, handler.executeCalled)
	assert.Equal(t, models.JobStatusCompleted, job.Status)
	assert.Equal(t, "success", job.Result)
	assert.NotNil(t, job.CompletedAt)

	// History should be created
	assert.Len(t, jobRepo.history, 1)
	assert.Equal(t, models.JobStatusCompleted, jobRepo.history[0].Status)
}

func TestExecutor_Execute_Failure(t *testing.T) {
	jobRepo := newMockJobRepo()
	executor := NewExecutor(jobRepo)

	handler := &mockJobHandler{executeErr: errors.New("ingestion failed")}
	executor.RegisterHandler(models.JobTypeStreamIngestion, handler)

	now := models.Now()
	job := &models.Job{
		Type:         models.JobTypeStreamIngestion,
		TargetID:     models.NewULID(),
		TargetName:   "Test Source",
		Status:       models.JobStatusRunning,
		StartedAt:    &now,
		AttemptCount: 1, // Already attempted once
		MaxAttempts:  1, // No retries allowed
	}
	job.ID = models.NewULID()
	jobRepo.jobs[job.ID] = job

	ctx := context.Background()
	err := executor.Execute(ctx, job)
	require.NoError(t, err) // Execute returns nil, error is recorded in job

	assert.True(t, handler.executeCalled)
	assert.Equal(t, models.JobStatusFailed, job.Status)
	assert.Equal(t, "ingestion failed", job.LastError)
	assert.NotNil(t, job.CompletedAt)

	// History should be created
	assert.Len(t, jobRepo.history, 1)
	assert.Equal(t, models.JobStatusFailed, jobRepo.history[0].Status)
}

func TestExecutor_Execute_FailureWithRetry(t *testing.T) {
	jobRepo := newMockJobRepo()
	executor := NewExecutor(jobRepo)

	handler := &mockJobHandler{executeErr: errors.New("temporary error")}
	executor.RegisterHandler(models.JobTypeStreamIngestion, handler)

	now := models.Now()
	job := &models.Job{
		Type:           models.JobTypeStreamIngestion,
		TargetID:       models.NewULID(),
		TargetName:     "Test Source",
		Status:         models.JobStatusRunning,
		StartedAt:      &now,
		AttemptCount:   1,
		MaxAttempts:    3,
		BackoffSeconds: 10,
	}
	job.ID = models.NewULID()
	jobRepo.jobs[job.ID] = job

	ctx := context.Background()
	err := executor.Execute(ctx, job)
	require.NoError(t, err)

	// Should be scheduled for retry
	assert.Equal(t, models.JobStatusScheduled, job.Status)
	assert.NotNil(t, job.NextRunAt)
}

func TestExecutor_Execute_NoHandler(t *testing.T) {
	jobRepo := newMockJobRepo()
	executor := NewExecutor(jobRepo)

	job := &models.Job{
		Type:       models.JobTypeStreamIngestion,
		TargetID:   models.NewULID(),
		TargetName: "Test Source",
		Status:     models.JobStatusRunning,
	}
	job.ID = models.NewULID()

	ctx := context.Background()
	err := executor.Execute(ctx, job)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no handler registered")
}

func TestStreamIngestionHandler(t *testing.T) {
	service := &mockSourceService{}
	handler := NewStreamIngestionHandler(service)

	job := &models.Job{
		Type:       models.JobTypeStreamIngestion,
		TargetID:   models.NewULID(),
		TargetName: "Test Source",
	}
	job.ID = models.NewULID()

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		service.ingestErr = nil
		result, err := handler.Execute(ctx, job)
		require.NoError(t, err)
		assert.Contains(t, result, "ingested source")
		assert.True(t, service.ingestCalled)
	})

	t.Run("failure", func(t *testing.T) {
		service.ingestErr = errors.New("connection error")
		_, err := handler.Execute(ctx, job)
		assert.Error(t, err)
		assert.Equal(t, "connection error", err.Error())
	})
}

func TestEpgIngestionHandler(t *testing.T) {
	service := &mockEpgService{}
	handler := NewEpgIngestionHandler(service)

	job := &models.Job{
		Type:       models.JobTypeEpgIngestion,
		TargetID:   models.NewULID(),
		TargetName: "Test EPG",
	}
	job.ID = models.NewULID()

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		service.ingestErr = nil
		result, err := handler.Execute(ctx, job)
		require.NoError(t, err)
		assert.Contains(t, result, "ingested EPG source")
		assert.True(t, service.ingestCalled)
	})

	t.Run("failure", func(t *testing.T) {
		service.ingestErr = errors.New("parse error")
		_, err := handler.Execute(ctx, job)
		assert.Error(t, err)
	})
}

func TestProxyGenerationHandler(t *testing.T) {
	job := &models.Job{
		Type:       models.JobTypeProxyGeneration,
		TargetID:   models.NewULID(),
		TargetName: "Test Proxy",
	}
	job.ID = models.NewULID()

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		handler := NewProxyGenerationHandler(mockProxyGenerateFunc(100, 500, nil))
		result, err := handler.Execute(ctx, job)
		require.NoError(t, err)
		assert.Contains(t, result, "generated proxy")
		assert.Contains(t, result, "100 channels")
		assert.Contains(t, result, "500 programs")
	})

	t.Run("failure", func(t *testing.T) {
		handler := NewProxyGenerationHandler(mockProxyGenerateFunc(0, 0, errors.New("pipeline error")))
		_, err := handler.Execute(ctx, job)
		assert.Error(t, err)
	})
}

// mockBackupResult implements BackupCreateResult for testing.
type mockBackupResult struct {
	filename string
}

func (r *mockBackupResult) GetFilename() string {
	return r.filename
}

// mockBackupService implements BackupCreateService for testing.
type mockBackupService struct {
	createBackupResult   *mockBackupResult
	createBackupErr      error
	cleanupOldBackups    int
	cleanupOldBackupsErr error
	createCalled         bool
	cleanupCalled        bool
}

func (m *mockBackupService) CreateBackupForScheduler(ctx context.Context) (BackupCreateResult, error) {
	m.createCalled = true
	if m.createBackupErr != nil {
		return nil, m.createBackupErr
	}
	return m.createBackupResult, nil
}

func (m *mockBackupService) CleanupOldBackups(ctx context.Context) (int, error) {
	m.cleanupCalled = true
	return m.cleanupOldBackups, m.cleanupOldBackupsErr
}

func TestBackupJobHandler(t *testing.T) {
	job := &models.Job{
		Type:       models.JobTypeBackup,
		TargetName: "Scheduled Backup",
	}
	job.ID = models.NewULID()

	ctx := context.Background()

	t.Run("success_creates_backup_and_cleans_up", func(t *testing.T) {
		service := &mockBackupService{
			createBackupResult: &mockBackupResult{filename: "tvarr-backup-2025-01-15T10-00-00.db.gz"},
			cleanupOldBackups:  3,
		}
		handler := NewBackupJobHandler(service)

		result, err := handler.Execute(ctx, job)
		require.NoError(t, err)
		assert.True(t, service.createCalled)
		assert.True(t, service.cleanupCalled)
		assert.Contains(t, result, "created backup")
		assert.Contains(t, result, "tvarr-backup-2025-01-15T10-00-00.db.gz")
		assert.Contains(t, result, "cleaned up 3 old backups")
	})

	t.Run("success_no_cleanup_needed", func(t *testing.T) {
		service := &mockBackupService{
			createBackupResult: &mockBackupResult{filename: "tvarr-backup-2025-01-15T10-00-00.db.gz"},
			cleanupOldBackups:  0,
		}
		handler := NewBackupJobHandler(service)

		result, err := handler.Execute(ctx, job)
		require.NoError(t, err)
		assert.True(t, service.createCalled)
		assert.True(t, service.cleanupCalled)
		assert.Contains(t, result, "created backup")
		assert.NotContains(t, result, "cleaned up")
	})

	t.Run("failure_backup_creation_fails", func(t *testing.T) {
		service := &mockBackupService{
			createBackupErr: errors.New("disk full"),
		}
		handler := NewBackupJobHandler(service)

		_, err := handler.Execute(ctx, job)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "backup creation failed")
		assert.Contains(t, err.Error(), "disk full")
		assert.True(t, service.createCalled)
		assert.False(t, service.cleanupCalled)
	})

	t.Run("success_cleanup_failure_does_not_fail_job", func(t *testing.T) {
		service := &mockBackupService{
			createBackupResult:   &mockBackupResult{filename: "tvarr-backup-2025-01-15T10-00-00.db.gz"},
			cleanupOldBackupsErr: errors.New("permission denied"),
		}
		handler := NewBackupJobHandler(service)

		result, err := handler.Execute(ctx, job)
		require.NoError(t, err) // Cleanup failure doesn't fail the job
		assert.True(t, service.createCalled)
		assert.True(t, service.cleanupCalled)
		assert.Contains(t, result, "created backup")
	})
}
