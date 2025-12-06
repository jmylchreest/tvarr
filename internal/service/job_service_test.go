package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/scheduler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// jobMockJobRepo implements repository.JobRepository for testing.
type jobMockJobRepo struct {
	jobs    map[models.ULID]*models.Job
	history []*models.JobHistory
	err     error
}

func newJobMockJobRepo() *jobMockJobRepo {
	return &jobMockJobRepo{
		jobs: make(map[models.ULID]*models.Job),
	}
}

func (m *jobMockJobRepo) Create(ctx context.Context, job *models.Job) error {
	if m.err != nil {
		return m.err
	}
	if job.ID.IsZero() {
		job.ID = models.NewULID()
	}
	m.jobs[job.ID] = job
	return nil
}

func (m *jobMockJobRepo) GetByID(ctx context.Context, id models.ULID) (*models.Job, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.jobs[id], nil
}

func (m *jobMockJobRepo) GetAll(ctx context.Context) ([]*models.Job, error) {
	if m.err != nil {
		return nil, m.err
	}
	var jobs []*models.Job
	for _, j := range m.jobs {
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (m *jobMockJobRepo) GetPending(ctx context.Context) ([]*models.Job, error) {
	if m.err != nil {
		return nil, m.err
	}
	var jobs []*models.Job
	for _, j := range m.jobs {
		if j.Status == models.JobStatusPending || j.Status == models.JobStatusScheduled {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

func (m *jobMockJobRepo) GetByStatus(ctx context.Context, status models.JobStatus) ([]*models.Job, error) {
	if m.err != nil {
		return nil, m.err
	}
	var jobs []*models.Job
	for _, j := range m.jobs {
		if j.Status == status {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

func (m *jobMockJobRepo) GetByType(ctx context.Context, jobType models.JobType) ([]*models.Job, error) {
	if m.err != nil {
		return nil, m.err
	}
	var jobs []*models.Job
	for _, j := range m.jobs {
		if j.Type == jobType {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

func (m *jobMockJobRepo) GetByTargetID(ctx context.Context, targetID models.ULID) ([]*models.Job, error) {
	if m.err != nil {
		return nil, m.err
	}
	var jobs []*models.Job
	for _, j := range m.jobs {
		if j.TargetID == targetID {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

func (m *jobMockJobRepo) GetRunning(ctx context.Context) ([]*models.Job, error) {
	if m.err != nil {
		return nil, m.err
	}
	var jobs []*models.Job
	for _, j := range m.jobs {
		if j.Status == models.JobStatusRunning {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

func (m *jobMockJobRepo) Update(ctx context.Context, job *models.Job) error {
	if m.err != nil {
		return m.err
	}
	m.jobs[job.ID] = job
	return nil
}

func (m *jobMockJobRepo) Delete(ctx context.Context, id models.ULID) error {
	if m.err != nil {
		return m.err
	}
	delete(m.jobs, id)
	return nil
}

func (m *jobMockJobRepo) DeleteCompleted(ctx context.Context, before time.Time) (int64, error) {
	if m.err != nil {
		return 0, m.err
	}
	var count int64
	for id, j := range m.jobs {
		if j.IsFinished() && j.CompletedAt != nil && j.CompletedAt.Before(before) {
			delete(m.jobs, id)
			count++
		}
	}
	return count, nil
}

func (m *jobMockJobRepo) AcquireJob(ctx context.Context, workerID string) (*models.Job, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, j := range m.jobs {
		if j.Status == models.JobStatusPending && j.LockedBy == "" {
			j.Status = models.JobStatusRunning
			j.LockedBy = workerID
			now := models.Now()
			j.LockedAt = &now
			j.AttemptCount++
			return j, nil
		}
	}
	return nil, nil
}

func (m *jobMockJobRepo) ReleaseJob(ctx context.Context, id models.ULID) error {
	if m.err != nil {
		return m.err
	}
	if j, ok := m.jobs[id]; ok {
		j.LockedBy = ""
		j.LockedAt = nil
		j.Status = models.JobStatusPending
	}
	return nil
}

func (m *jobMockJobRepo) FindDuplicatePending(ctx context.Context, jobType models.JobType, targetID models.ULID) (*models.Job, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, j := range m.jobs {
		if j.Type == jobType && j.TargetID == targetID && j.IsPending() {
			return j, nil
		}
	}
	return nil, nil
}

func (m *jobMockJobRepo) CreateHistory(ctx context.Context, history *models.JobHistory) error {
	if m.err != nil {
		return m.err
	}
	if history.ID.IsZero() {
		history.ID = models.NewULID()
	}
	m.history = append(m.history, history)
	return nil
}

func (m *jobMockJobRepo) GetHistory(ctx context.Context, jobType *models.JobType, offset, limit int) ([]*models.JobHistory, int64, error) {
	if m.err != nil {
		return nil, 0, m.err
	}
	var filtered []*models.JobHistory
	for _, h := range m.history {
		if jobType == nil || h.Type == *jobType {
			filtered = append(filtered, h)
		}
	}
	total := int64(len(filtered))
	if offset >= len(filtered) {
		return nil, total, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], total, nil
}

func (m *jobMockJobRepo) DeleteHistory(ctx context.Context, before time.Time) (int64, error) {
	if m.err != nil {
		return 0, m.err
	}
	var remaining []*models.JobHistory
	var count int64
	for _, h := range m.history {
		if h.CompletedAt == nil || h.CompletedAt.After(before) {
			remaining = append(remaining, h)
		} else {
			count++
		}
	}
	m.history = remaining
	return count, nil
}

// jobMockStreamSourceRepo implements repository.StreamSourceRepository for testing.
type jobMockStreamSourceRepo struct {
	sources map[models.ULID]*models.StreamSource
	err     error
}

func newJobMockStreamSourceRepo() *jobMockStreamSourceRepo {
	return &jobMockStreamSourceRepo{
		sources: make(map[models.ULID]*models.StreamSource),
	}
}

func (m *jobMockStreamSourceRepo) Create(ctx context.Context, source *models.StreamSource) error {
	return nil
}

func (m *jobMockStreamSourceRepo) GetByID(ctx context.Context, id models.ULID) (*models.StreamSource, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.sources[id], nil
}

func (m *jobMockStreamSourceRepo) GetAll(ctx context.Context) ([]*models.StreamSource, error) {
	var sources []*models.StreamSource
	for _, s := range m.sources {
		sources = append(sources, s)
	}
	return sources, nil
}

func (m *jobMockStreamSourceRepo) GetEnabled(ctx context.Context) ([]*models.StreamSource, error) {
	return nil, nil
}

func (m *jobMockStreamSourceRepo) Update(ctx context.Context, source *models.StreamSource) error {
	return nil
}

func (m *jobMockStreamSourceRepo) Delete(ctx context.Context, id models.ULID) error {
	return nil
}

func (m *jobMockStreamSourceRepo) GetByName(ctx context.Context, name string) (*models.StreamSource, error) {
	return nil, nil
}

func (m *jobMockStreamSourceRepo) UpdateLastIngestion(ctx context.Context, id models.ULID, status string, channelCount int) error {
	return nil
}

// jobMockEpgSourceRepo implements repository.EpgSourceRepository for testing.
type jobMockEpgSourceRepo struct {
	sources map[models.ULID]*models.EpgSource
	err     error
}

func newJobMockEpgSourceRepo() *jobMockEpgSourceRepo {
	return &jobMockEpgSourceRepo{
		sources: make(map[models.ULID]*models.EpgSource),
	}
}

func (m *jobMockEpgSourceRepo) Create(ctx context.Context, source *models.EpgSource) error {
	return nil
}

func (m *jobMockEpgSourceRepo) GetByID(ctx context.Context, id models.ULID) (*models.EpgSource, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.sources[id], nil
}

func (m *jobMockEpgSourceRepo) GetAll(ctx context.Context) ([]*models.EpgSource, error) {
	return nil, nil
}

func (m *jobMockEpgSourceRepo) GetEnabled(ctx context.Context) ([]*models.EpgSource, error) {
	return nil, nil
}

func (m *jobMockEpgSourceRepo) Update(ctx context.Context, source *models.EpgSource) error {
	return nil
}

func (m *jobMockEpgSourceRepo) Delete(ctx context.Context, id models.ULID) error {
	return nil
}

func (m *jobMockEpgSourceRepo) GetByName(ctx context.Context, name string) (*models.EpgSource, error) {
	return nil, nil
}

func (m *jobMockEpgSourceRepo) GetByURL(ctx context.Context, url string) (*models.EpgSource, error) {
	return nil, nil
}

func (m *jobMockEpgSourceRepo) UpdateLastIngestion(ctx context.Context, id models.ULID, status string, programCount int) error {
	return nil
}

// jobMockProxyRepo implements repository.StreamProxyRepository for testing.
type jobMockProxyRepo struct {
	proxies map[models.ULID]*models.StreamProxy
	err     error
}

func newJobMockProxyRepo() *jobMockProxyRepo {
	return &jobMockProxyRepo{
		proxies: make(map[models.ULID]*models.StreamProxy),
	}
}

func (m *jobMockProxyRepo) Create(ctx context.Context, proxy *models.StreamProxy) error {
	return nil
}

func (m *jobMockProxyRepo) GetByID(ctx context.Context, id models.ULID) (*models.StreamProxy, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.proxies[id], nil
}

func (m *jobMockProxyRepo) GetByIDWithRelations(ctx context.Context, id models.ULID) (*models.StreamProxy, error) {
	return nil, nil
}

func (m *jobMockProxyRepo) GetAll(ctx context.Context) ([]*models.StreamProxy, error) {
	return nil, nil
}

func (m *jobMockProxyRepo) GetActive(ctx context.Context) ([]*models.StreamProxy, error) {
	return nil, nil
}

func (m *jobMockProxyRepo) Update(ctx context.Context, proxy *models.StreamProxy) error {
	return nil
}

func (m *jobMockProxyRepo) Delete(ctx context.Context, id models.ULID) error {
	return nil
}

func (m *jobMockProxyRepo) GetByName(ctx context.Context, name string) (*models.StreamProxy, error) {
	return nil, nil
}

func (m *jobMockProxyRepo) UpdateStatus(ctx context.Context, id models.ULID, status models.StreamProxyStatus, lastError string) error {
	return nil
}

func (m *jobMockProxyRepo) UpdateLastGeneration(ctx context.Context, id models.ULID, channelCount, programCount int) error {
	return nil
}

func (m *jobMockProxyRepo) SetSources(ctx context.Context, proxyID models.ULID, sourceIDs []models.ULID, priorities map[models.ULID]int) error {
	return nil
}

func (m *jobMockProxyRepo) SetEpgSources(ctx context.Context, proxyID models.ULID, sourceIDs []models.ULID, priorities map[models.ULID]int) error {
	return nil
}

func (m *jobMockProxyRepo) GetSources(ctx context.Context, proxyID models.ULID) ([]*models.StreamSource, error) {
	return nil, nil
}

func (m *jobMockProxyRepo) GetEpgSources(ctx context.Context, proxyID models.ULID) ([]*models.EpgSource, error) {
	return nil, nil
}

func (m *jobMockProxyRepo) GetFilters(ctx context.Context, proxyID models.ULID) ([]*models.Filter, error) {
	return nil, nil
}

func (m *jobMockProxyRepo) SetFilters(ctx context.Context, proxyID models.ULID, filterIDs []models.ULID, orders map[models.ULID]int) error {
	return nil
}

func (m *jobMockProxyRepo) GetBySourceID(ctx context.Context, sourceID models.ULID) ([]*models.StreamProxy, error) {
	return nil, nil
}

func (m *jobMockProxyRepo) GetByEpgSourceID(ctx context.Context, epgSourceID models.ULID) ([]*models.StreamProxy, error) {
	return nil, nil
}

// mockScheduler wraps the real scheduler for testing.
type mockScheduler struct {
	scheduleImmediateJob *models.Job
	scheduleImmediateErr error
	validateCronErr      error
	parseCronTime        time.Time
	parseCronErr         error
}

func TestJobService_GetByID(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	svc := NewJobService(jobRepo, newJobMockStreamSourceRepo(), newJobMockEpgSourceRepo(), newJobMockProxyRepo())

	ctx := context.Background()

	// Create a test job
	job := &models.Job{
		Type:       models.JobTypeStreamIngestion,
		TargetID:   models.NewULID(),
		TargetName: "Test Source",
		Status:     models.JobStatusPending,
	}
	job.ID = models.NewULID()
	jobRepo.jobs[job.ID] = job

	// Test GetByID
	result, err := svc.GetByID(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, job.ID, result.ID)
	assert.Equal(t, job.Type, result.Type)
}

func TestJobService_GetAll(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	svc := NewJobService(jobRepo, newJobMockStreamSourceRepo(), newJobMockEpgSourceRepo(), newJobMockProxyRepo())

	ctx := context.Background()

	// Create test jobs
	job1 := &models.Job{Type: models.JobTypeStreamIngestion, Status: models.JobStatusPending}
	job1.ID = models.NewULID()
	job2 := &models.Job{Type: models.JobTypeEpgIngestion, Status: models.JobStatusRunning}
	job2.ID = models.NewULID()

	jobRepo.jobs[job1.ID] = job1
	jobRepo.jobs[job2.ID] = job2

	// Test GetAll
	results, err := svc.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestJobService_GetPending(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	svc := NewJobService(jobRepo, newJobMockStreamSourceRepo(), newJobMockEpgSourceRepo(), newJobMockProxyRepo())

	ctx := context.Background()

	// Create jobs with different statuses
	pendingJob := &models.Job{Type: models.JobTypeStreamIngestion, Status: models.JobStatusPending}
	pendingJob.ID = models.NewULID()
	runningJob := &models.Job{Type: models.JobTypeEpgIngestion, Status: models.JobStatusRunning}
	runningJob.ID = models.NewULID()
	completedJob := &models.Job{Type: models.JobTypeProxyGeneration, Status: models.JobStatusCompleted}
	completedJob.ID = models.NewULID()

	jobRepo.jobs[pendingJob.ID] = pendingJob
	jobRepo.jobs[runningJob.ID] = runningJob
	jobRepo.jobs[completedJob.ID] = completedJob

	// Test GetPending
	results, err := svc.GetPending(ctx)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, models.JobStatusPending, results[0].Status)
}

func TestJobService_GetRunning(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	svc := NewJobService(jobRepo, newJobMockStreamSourceRepo(), newJobMockEpgSourceRepo(), newJobMockProxyRepo())

	ctx := context.Background()

	// Create jobs with different statuses
	pendingJob := &models.Job{Type: models.JobTypeStreamIngestion, Status: models.JobStatusPending}
	pendingJob.ID = models.NewULID()
	runningJob := &models.Job{Type: models.JobTypeEpgIngestion, Status: models.JobStatusRunning}
	runningJob.ID = models.NewULID()

	jobRepo.jobs[pendingJob.ID] = pendingJob
	jobRepo.jobs[runningJob.ID] = runningJob

	// Test GetRunning
	results, err := svc.GetRunning(ctx)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, models.JobStatusRunning, results[0].Status)
}

func TestJobService_GetByType(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	svc := NewJobService(jobRepo, newJobMockStreamSourceRepo(), newJobMockEpgSourceRepo(), newJobMockProxyRepo())

	ctx := context.Background()

	// Create jobs with different types
	job1 := &models.Job{Type: models.JobTypeStreamIngestion, Status: models.JobStatusPending}
	job1.ID = models.NewULID()
	job2 := &models.Job{Type: models.JobTypeStreamIngestion, Status: models.JobStatusRunning}
	job2.ID = models.NewULID()
	job3 := &models.Job{Type: models.JobTypeEpgIngestion, Status: models.JobStatusPending}
	job3.ID = models.NewULID()

	jobRepo.jobs[job1.ID] = job1
	jobRepo.jobs[job2.ID] = job2
	jobRepo.jobs[job3.ID] = job3

	// Test GetByType
	results, err := svc.GetByType(ctx, models.JobTypeStreamIngestion)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	for _, j := range results {
		assert.Equal(t, models.JobTypeStreamIngestion, j.Type)
	}
}

func TestJobService_GetByTargetID(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	svc := NewJobService(jobRepo, newJobMockStreamSourceRepo(), newJobMockEpgSourceRepo(), newJobMockProxyRepo())

	ctx := context.Background()

	targetID := models.NewULID()
	otherTargetID := models.NewULID()

	// Create jobs with different targets
	job1 := &models.Job{Type: models.JobTypeStreamIngestion, TargetID: targetID, Status: models.JobStatusPending}
	job1.ID = models.NewULID()
	job2 := &models.Job{Type: models.JobTypeEpgIngestion, TargetID: targetID, Status: models.JobStatusRunning}
	job2.ID = models.NewULID()
	job3 := &models.Job{Type: models.JobTypeProxyGeneration, TargetID: otherTargetID, Status: models.JobStatusPending}
	job3.ID = models.NewULID()

	jobRepo.jobs[job1.ID] = job1
	jobRepo.jobs[job2.ID] = job2
	jobRepo.jobs[job3.ID] = job3

	// Test GetByTargetID
	results, err := svc.GetByTargetID(ctx, targetID)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	for _, j := range results {
		assert.Equal(t, targetID, j.TargetID)
	}
}

func TestJobService_CancelJob(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	svc := NewJobService(jobRepo, newJobMockStreamSourceRepo(), newJobMockEpgSourceRepo(), newJobMockProxyRepo())

	ctx := context.Background()

	t.Run("cancel pending job", func(t *testing.T) {
		job := &models.Job{
			Type:       models.JobTypeStreamIngestion,
			TargetID:   models.NewULID(),
			TargetName: "Test Source",
			Status:     models.JobStatusPending,
		}
		job.ID = models.NewULID()
		jobRepo.jobs[job.ID] = job

		err := svc.CancelJob(ctx, job.ID)
		require.NoError(t, err)
		assert.Equal(t, models.JobStatusCancelled, job.Status)
		assert.NotNil(t, job.CompletedAt)
	})

	t.Run("cancel running job", func(t *testing.T) {
		job := &models.Job{
			Type:       models.JobTypeStreamIngestion,
			TargetID:   models.NewULID(),
			TargetName: "Test Source",
			Status:     models.JobStatusRunning,
		}
		job.ID = models.NewULID()
		jobRepo.jobs[job.ID] = job

		err := svc.CancelJob(ctx, job.ID)
		require.NoError(t, err)
		assert.Equal(t, models.JobStatusCancelled, job.Status)
	})

	t.Run("cannot cancel completed job", func(t *testing.T) {
		job := &models.Job{
			Type:   models.JobTypeStreamIngestion,
			Status: models.JobStatusCompleted,
		}
		job.ID = models.NewULID()
		jobRepo.jobs[job.ID] = job

		err := svc.CancelJob(ctx, job.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot cancel finished job")
	})

	t.Run("job not found", func(t *testing.T) {
		err := svc.CancelJob(ctx, models.NewULID())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "job not found")
	})
}

func TestJobService_DeleteJob(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	svc := NewJobService(jobRepo, newJobMockStreamSourceRepo(), newJobMockEpgSourceRepo(), newJobMockProxyRepo())

	ctx := context.Background()

	t.Run("delete completed job", func(t *testing.T) {
		job := &models.Job{
			Type:   models.JobTypeStreamIngestion,
			Status: models.JobStatusCompleted,
		}
		job.ID = models.NewULID()
		jobRepo.jobs[job.ID] = job

		err := svc.DeleteJob(ctx, job.ID)
		require.NoError(t, err)
		assert.Nil(t, jobRepo.jobs[job.ID])
	})

	t.Run("delete failed job", func(t *testing.T) {
		job := &models.Job{
			Type:   models.JobTypeStreamIngestion,
			Status: models.JobStatusFailed,
		}
		job.ID = models.NewULID()
		jobRepo.jobs[job.ID] = job

		err := svc.DeleteJob(ctx, job.ID)
		require.NoError(t, err)
		assert.Nil(t, jobRepo.jobs[job.ID])
	})

	t.Run("cannot delete pending job", func(t *testing.T) {
		job := &models.Job{
			Type:   models.JobTypeStreamIngestion,
			Status: models.JobStatusPending,
		}
		job.ID = models.NewULID()
		jobRepo.jobs[job.ID] = job

		err := svc.DeleteJob(ctx, job.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot delete unfinished job")
	})

	t.Run("cannot delete running job", func(t *testing.T) {
		job := &models.Job{
			Type:   models.JobTypeStreamIngestion,
			Status: models.JobStatusRunning,
		}
		job.ID = models.NewULID()
		jobRepo.jobs[job.ID] = job

		err := svc.DeleteJob(ctx, job.ID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot delete unfinished job")
	})
}

func TestJobService_Cleanup(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	svc := NewJobService(jobRepo, newJobMockStreamSourceRepo(), newJobMockEpgSourceRepo(), newJobMockProxyRepo())

	ctx := context.Background()

	// Create some old completed jobs
	oldTime := models.Now().Add(-48 * time.Hour)
	recentTime := models.Now().Add(-1 * time.Hour)

	oldJob := &models.Job{
		Type:        models.JobTypeStreamIngestion,
		Status:      models.JobStatusCompleted,
		CompletedAt: &oldTime,
	}
	oldJob.ID = models.NewULID()

	recentJob := &models.Job{
		Type:        models.JobTypeStreamIngestion,
		Status:      models.JobStatusCompleted,
		CompletedAt: &recentTime,
	}
	recentJob.ID = models.NewULID()

	jobRepo.jobs[oldJob.ID] = oldJob
	jobRepo.jobs[recentJob.ID] = recentJob

	// Create some history
	oldHistory := &models.JobHistory{
		JobID:       oldJob.ID,
		Type:        models.JobTypeStreamIngestion,
		Status:      models.JobStatusCompleted,
		CompletedAt: &oldTime,
	}
	oldHistory.ID = models.NewULID()
	jobRepo.history = append(jobRepo.history, oldHistory)

	// Cleanup jobs older than 24 hours
	jobsDeleted, historyDeleted, err := svc.Cleanup(ctx, 24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, int64(1), jobsDeleted)
	assert.Equal(t, int64(1), historyDeleted)

	// Verify old job was deleted, recent job remains
	assert.Nil(t, jobRepo.jobs[oldJob.ID])
	assert.NotNil(t, jobRepo.jobs[recentJob.ID])
}

func TestJobService_GetStats(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	svc := NewJobService(jobRepo, newJobMockStreamSourceRepo(), newJobMockEpgSourceRepo(), newJobMockProxyRepo())

	ctx := context.Background()

	// Create jobs with various statuses and types
	jobs := []*models.Job{
		{Type: models.JobTypeStreamIngestion, Status: models.JobStatusPending},
		{Type: models.JobTypeStreamIngestion, Status: models.JobStatusScheduled},
		{Type: models.JobTypeStreamIngestion, Status: models.JobStatusRunning},
		{Type: models.JobTypeEpgIngestion, Status: models.JobStatusCompleted},
		{Type: models.JobTypeEpgIngestion, Status: models.JobStatusFailed},
		{Type: models.JobTypeProxyGeneration, Status: models.JobStatusPending},
	}

	for _, j := range jobs {
		j.ID = models.NewULID()
		jobRepo.jobs[j.ID] = j
	}

	// Get stats
	stats, err := svc.GetStats(ctx)
	require.NoError(t, err)

	assert.Equal(t, int64(3), stats.PendingCount)  // 2 pending + 1 scheduled
	assert.Equal(t, int64(1), stats.RunningCount)  // 1 running
	assert.Equal(t, int64(1), stats.CompletedCount) // 1 completed
	assert.Equal(t, int64(1), stats.FailedCount)    // 1 failed

	// Check by type counts
	assert.Equal(t, int64(3), stats.ByType[string(models.JobTypeStreamIngestion)])
	assert.Equal(t, int64(2), stats.ByType[string(models.JobTypeEpgIngestion)])
	assert.Equal(t, int64(1), stats.ByType[string(models.JobTypeProxyGeneration)])
}

func TestJobService_TriggerStreamIngestion_NoScheduler(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	streamRepo := newJobMockStreamSourceRepo()
	svc := NewJobService(jobRepo, streamRepo, newJobMockEpgSourceRepo(), newJobMockProxyRepo())

	ctx := context.Background()

	sourceID := models.NewULID()
	source := &models.StreamSource{Name: "Test Source", Enabled: true}
	source.ID = sourceID
	streamRepo.sources[sourceID] = source

	// Try to trigger without scheduler configured
	_, err := svc.TriggerStreamIngestion(ctx, sourceID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "scheduler not configured")
}

func TestJobService_TriggerStreamIngestion_SourceNotFound(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	streamRepo := newJobMockStreamSourceRepo()
	svc := NewJobService(jobRepo, streamRepo, newJobMockEpgSourceRepo(), newJobMockProxyRepo())

	ctx := context.Background()

	// Try to trigger with non-existent source
	_, err := svc.TriggerStreamIngestion(ctx, models.NewULID())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "source not found")
}

func TestJobService_TriggerEpgIngestion_NoScheduler(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	epgRepo := newJobMockEpgSourceRepo()
	svc := NewJobService(jobRepo, newJobMockStreamSourceRepo(), epgRepo, newJobMockProxyRepo())

	ctx := context.Background()

	sourceID := models.NewULID()
	source := &models.EpgSource{Name: "Test EPG", Enabled: true}
	source.ID = sourceID
	epgRepo.sources[sourceID] = source

	// Try to trigger without scheduler configured
	_, err := svc.TriggerEpgIngestion(ctx, sourceID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "scheduler not configured")
}

func TestJobService_TriggerEpgIngestion_SourceNotFound(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	svc := NewJobService(jobRepo, newJobMockStreamSourceRepo(), newJobMockEpgSourceRepo(), newJobMockProxyRepo())

	ctx := context.Background()

	// Try to trigger with non-existent source
	_, err := svc.TriggerEpgIngestion(ctx, models.NewULID())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "EPG source not found")
}

func TestJobService_TriggerProxyGeneration_NoScheduler(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	proxyRepo := newJobMockProxyRepo()
	svc := NewJobService(jobRepo, newJobMockStreamSourceRepo(), newJobMockEpgSourceRepo(), proxyRepo)

	ctx := context.Background()

	proxyID := models.NewULID()
	proxy := &models.StreamProxy{Name: "Test Proxy", IsActive: true}
	proxy.ID = proxyID
	proxyRepo.proxies[proxyID] = proxy

	// Try to trigger without scheduler configured
	_, err := svc.TriggerProxyGeneration(ctx, proxyID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "scheduler not configured")
}

func TestJobService_TriggerProxyGeneration_ProxyNotFound(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	svc := NewJobService(jobRepo, newJobMockStreamSourceRepo(), newJobMockEpgSourceRepo(), newJobMockProxyRepo())

	ctx := context.Background()

	// Try to trigger with non-existent proxy
	_, err := svc.TriggerProxyGeneration(ctx, models.NewULID())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "proxy not found")
}

func TestJobService_ValidateCron_NoScheduler(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	svc := NewJobService(jobRepo, newJobMockStreamSourceRepo(), newJobMockEpgSourceRepo(), newJobMockProxyRepo())

	err := svc.ValidateCron("* * * * *")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "scheduler not configured")
}

func TestJobService_GetNextRun_NoScheduler(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	svc := NewJobService(jobRepo, newJobMockStreamSourceRepo(), newJobMockEpgSourceRepo(), newJobMockProxyRepo())

	_, err := svc.GetNextRun("* * * * *")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "scheduler not configured")
}

func TestJobService_GetRunnerStatus_NoRunner(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	svc := NewJobService(jobRepo, newJobMockStreamSourceRepo(), newJobMockEpgSourceRepo(), newJobMockProxyRepo())

	_, err := svc.GetRunnerStatus()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "runner not configured")
}

func TestJobService_GetHistory(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	svc := NewJobService(jobRepo, newJobMockStreamSourceRepo(), newJobMockEpgSourceRepo(), newJobMockProxyRepo())

	ctx := context.Background()

	// Create some history records
	now := models.Now()
	for i := 0; i < 5; i++ {
		h := &models.JobHistory{
			JobID:       models.NewULID(),
			Type:        models.JobTypeStreamIngestion,
			Status:      models.JobStatusCompleted,
			CompletedAt: &now,
		}
		h.ID = models.NewULID()
		jobRepo.history = append(jobRepo.history, h)
	}

	// Add some EPG history
	for i := 0; i < 3; i++ {
		h := &models.JobHistory{
			JobID:       models.NewULID(),
			Type:        models.JobTypeEpgIngestion,
			Status:      models.JobStatusCompleted,
			CompletedAt: &now,
		}
		h.ID = models.NewULID()
		jobRepo.history = append(jobRepo.history, h)
	}

	t.Run("get all history", func(t *testing.T) {
		history, total, err := svc.GetHistory(ctx, nil, 0, 100)
		require.NoError(t, err)
		assert.Equal(t, int64(8), total)
		assert.Len(t, history, 8)
	})

	t.Run("get history with pagination", func(t *testing.T) {
		history, total, err := svc.GetHistory(ctx, nil, 0, 3)
		require.NoError(t, err)
		assert.Equal(t, int64(8), total)
		assert.Len(t, history, 3)
	})

	t.Run("get history by type", func(t *testing.T) {
		jobType := models.JobTypeStreamIngestion
		history, total, err := svc.GetHistory(ctx, &jobType, 0, 100)
		require.NoError(t, err)
		assert.Equal(t, int64(5), total)
		assert.Len(t, history, 5)
	})
}

func TestJobService_WithScheduler(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	streamRepo := newJobMockStreamSourceRepo()
	epgRepo := newJobMockEpgSourceRepo()
	proxyRepo := newJobMockProxyRepo()

	svc := NewJobService(jobRepo, streamRepo, epgRepo, proxyRepo)

	// Create a real scheduler for testing
	sched := scheduler.NewScheduler(jobRepo, streamRepo, epgRepo, proxyRepo)

	// Test WithScheduler
	result := svc.WithScheduler(sched)
	assert.NotNil(t, result)
	assert.Same(t, svc, result) // Should return same instance

	// Now ValidateCron should work (scheduler uses 6-field cron: second minute hour day-of-month month day-of-week)
	err := svc.ValidateCron("0 * * * * *")
	assert.NoError(t, err)

	// Invalid cron should fail
	err = svc.ValidateCron("invalid")
	assert.Error(t, err)
}

func TestJobService_WithRunner(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	svc := NewJobService(jobRepo, newJobMockStreamSourceRepo(), newJobMockEpgSourceRepo(), newJobMockProxyRepo())

	// Create a real executor and runner for testing
	executor := scheduler.NewExecutor(jobRepo)
	runner := scheduler.NewRunner(jobRepo, executor)

	// Test WithRunner
	result := svc.WithRunner(runner)
	assert.NotNil(t, result)
	assert.Same(t, svc, result) // Should return same instance

	// Now GetRunnerStatus should work
	status, err := svc.GetRunnerStatus()
	assert.NoError(t, err)
	assert.NotNil(t, status)
	assert.False(t, status.Running) // Not started yet
}

func TestJobService_TriggerWithScheduler(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	streamRepo := newJobMockStreamSourceRepo()
	epgRepo := newJobMockEpgSourceRepo()
	proxyRepo := newJobMockProxyRepo()

	svc := NewJobService(jobRepo, streamRepo, epgRepo, proxyRepo)
	sched := scheduler.NewScheduler(jobRepo, streamRepo, epgRepo, proxyRepo)
	svc.WithScheduler(sched)

	ctx := context.Background()

	t.Run("trigger stream ingestion", func(t *testing.T) {
		sourceID := models.NewULID()
		source := &models.StreamSource{Name: "Test Source", Enabled: true}
		source.ID = sourceID
		streamRepo.sources[sourceID] = source

		job, err := svc.TriggerStreamIngestion(ctx, sourceID)
		require.NoError(t, err)
		assert.NotNil(t, job)
		assert.Equal(t, models.JobTypeStreamIngestion, job.Type)
		assert.Equal(t, sourceID, job.TargetID)
		assert.Equal(t, models.JobStatusPending, job.Status)
	})

	t.Run("trigger EPG ingestion", func(t *testing.T) {
		sourceID := models.NewULID()
		source := &models.EpgSource{Name: "Test EPG", Enabled: true}
		source.ID = sourceID
		epgRepo.sources[sourceID] = source

		job, err := svc.TriggerEpgIngestion(ctx, sourceID)
		require.NoError(t, err)
		assert.NotNil(t, job)
		assert.Equal(t, models.JobTypeEpgIngestion, job.Type)
		assert.Equal(t, sourceID, job.TargetID)
	})

	t.Run("trigger proxy generation", func(t *testing.T) {
		proxyID := models.NewULID()
		proxy := &models.StreamProxy{Name: "Test Proxy", IsActive: true}
		proxy.ID = proxyID
		proxyRepo.proxies[proxyID] = proxy

		job, err := svc.TriggerProxyGeneration(ctx, proxyID)
		require.NoError(t, err)
		assert.NotNil(t, job)
		assert.Equal(t, models.JobTypeProxyGeneration, job.Type)
		assert.Equal(t, proxyID, job.TargetID)
	})
}

func TestJobService_SourceRepoError(t *testing.T) {
	jobRepo := newJobMockJobRepo()
	streamRepo := newJobMockStreamSourceRepo()
	streamRepo.err = errors.New("database error")

	svc := NewJobService(jobRepo, streamRepo, newJobMockEpgSourceRepo(), newJobMockProxyRepo())
	sched := scheduler.NewScheduler(jobRepo, streamRepo, newJobMockEpgSourceRepo(), newJobMockProxyRepo())
	svc.WithScheduler(sched)

	ctx := context.Background()

	_, err := svc.TriggerStreamIngestion(ctx, models.NewULID())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "getting source")
}
