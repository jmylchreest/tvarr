package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/jmylchreest/tvarr/internal/scheduler"
	"github.com/jmylchreest/tvarr/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockJobRepo implements repository.JobRepository for testing.
type mockJobRepo struct {
	jobs    map[models.ULID]*models.Job
	history []*models.JobHistory
	err     error
}

func newMockJobRepo() *mockJobRepo {
	return &mockJobRepo{
		jobs: make(map[models.ULID]*models.Job),
	}
}

func (m *mockJobRepo) Create(ctx context.Context, job *models.Job) error {
	if m.err != nil {
		return m.err
	}
	if job.ID.IsZero() {
		job.ID = models.NewULID()
	}
	m.jobs[job.ID] = job
	return nil
}

func (m *mockJobRepo) GetByID(ctx context.Context, id models.ULID) (*models.Job, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.jobs[id], nil
}

func (m *mockJobRepo) GetAll(ctx context.Context) ([]*models.Job, error) {
	if m.err != nil {
		return nil, m.err
	}
	var jobs []*models.Job
	for _, j := range m.jobs {
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (m *mockJobRepo) GetPending(ctx context.Context) ([]*models.Job, error) {
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

func (m *mockJobRepo) GetByStatus(ctx context.Context, status models.JobStatus) ([]*models.Job, error) {
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

func (m *mockJobRepo) GetByType(ctx context.Context, jobType models.JobType) ([]*models.Job, error) {
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

func (m *mockJobRepo) GetByTargetID(ctx context.Context, targetID models.ULID) ([]*models.Job, error) {
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

func (m *mockJobRepo) GetRunning(ctx context.Context) ([]*models.Job, error) {
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

func (m *mockJobRepo) Update(ctx context.Context, job *models.Job) error {
	if m.err != nil {
		return m.err
	}
	m.jobs[job.ID] = job
	return nil
}

func (m *mockJobRepo) Delete(ctx context.Context, id models.ULID) error {
	if m.err != nil {
		return m.err
	}
	delete(m.jobs, id)
	return nil
}

func (m *mockJobRepo) DeleteCompleted(ctx context.Context, before time.Time) (int64, error) {
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

func (m *mockJobRepo) AcquireJob(ctx context.Context, workerID string) (*models.Job, error) {
	return nil, nil
}

func (m *mockJobRepo) ReleaseJob(ctx context.Context, id models.ULID) error {
	return nil
}

func (m *mockJobRepo) FindDuplicatePending(ctx context.Context, jobType models.JobType, targetID models.ULID) (*models.Job, error) {
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

func (m *mockJobRepo) CreateHistory(ctx context.Context, history *models.JobHistory) error {
	if m.err != nil {
		return m.err
	}
	if history.ID.IsZero() {
		history.ID = models.NewULID()
	}
	m.history = append(m.history, history)
	return nil
}

func (m *mockJobRepo) GetHistory(ctx context.Context, jobType *models.JobType, offset, limit int) ([]*models.JobHistory, int64, error) {
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

func (m *mockJobRepo) DeleteHistory(ctx context.Context, before time.Time) (int64, error) {
	return 0, nil
}

// mockStreamSourceRepoForJob implements repository.StreamSourceRepository for testing.
type mockStreamSourceRepoForJob struct {
	sources map[models.ULID]*models.StreamSource
}

func newMockStreamSourceRepoForJob() *mockStreamSourceRepoForJob {
	return &mockStreamSourceRepoForJob{
		sources: make(map[models.ULID]*models.StreamSource),
	}
}

func (m *mockStreamSourceRepoForJob) Create(ctx context.Context, source *models.StreamSource) error {
	return nil
}

func (m *mockStreamSourceRepoForJob) GetByID(ctx context.Context, id models.ULID) (*models.StreamSource, error) {
	return m.sources[id], nil
}

func (m *mockStreamSourceRepoForJob) GetAll(ctx context.Context) ([]*models.StreamSource, error) {
	var sources []*models.StreamSource
	for _, s := range m.sources {
		sources = append(sources, s)
	}
	return sources, nil
}

func (m *mockStreamSourceRepoForJob) GetEnabled(ctx context.Context) ([]*models.StreamSource, error) {
	return nil, nil
}

func (m *mockStreamSourceRepoForJob) Update(ctx context.Context, source *models.StreamSource) error {
	return nil
}

func (m *mockStreamSourceRepoForJob) Delete(ctx context.Context, id models.ULID) error {
	return nil
}

func (m *mockStreamSourceRepoForJob) GetByName(ctx context.Context, name string) (*models.StreamSource, error) {
	return nil, nil
}

func (m *mockStreamSourceRepoForJob) UpdateLastIngestion(ctx context.Context, id models.ULID, status string, channelCount int) error {
	return nil
}

// mockEpgSourceRepoForJob implements repository.EpgSourceRepository for testing.
type mockEpgSourceRepoForJob struct {
	sources map[models.ULID]*models.EpgSource
}

func newMockEpgSourceRepoForJob() *mockEpgSourceRepoForJob {
	return &mockEpgSourceRepoForJob{
		sources: make(map[models.ULID]*models.EpgSource),
	}
}

func (m *mockEpgSourceRepoForJob) Create(ctx context.Context, source *models.EpgSource) error {
	return nil
}

func (m *mockEpgSourceRepoForJob) GetByID(ctx context.Context, id models.ULID) (*models.EpgSource, error) {
	return m.sources[id], nil
}

func (m *mockEpgSourceRepoForJob) GetAll(ctx context.Context) ([]*models.EpgSource, error) {
	return nil, nil
}

func (m *mockEpgSourceRepoForJob) GetEnabled(ctx context.Context) ([]*models.EpgSource, error) {
	return nil, nil
}

func (m *mockEpgSourceRepoForJob) Update(ctx context.Context, source *models.EpgSource) error {
	return nil
}

func (m *mockEpgSourceRepoForJob) Delete(ctx context.Context, id models.ULID) error {
	return nil
}

func (m *mockEpgSourceRepoForJob) GetByName(ctx context.Context, name string) (*models.EpgSource, error) {
	return nil, nil
}

func (m *mockEpgSourceRepoForJob) GetByURL(ctx context.Context, url string) (*models.EpgSource, error) {
	return nil, nil
}

func (m *mockEpgSourceRepoForJob) UpdateLastIngestion(ctx context.Context, id models.ULID, status string, programCount int) error {
	return nil
}

// mockProxyRepoForJob implements repository.StreamProxyRepository for testing.
type mockProxyRepoForJob struct {
	proxies map[models.ULID]*models.StreamProxy
}

func newMockProxyRepoForJob() *mockProxyRepoForJob {
	return &mockProxyRepoForJob{
		proxies: make(map[models.ULID]*models.StreamProxy),
	}
}

func (m *mockProxyRepoForJob) Create(ctx context.Context, proxy *models.StreamProxy) error {
	return nil
}

func (m *mockProxyRepoForJob) GetByID(ctx context.Context, id models.ULID) (*models.StreamProxy, error) {
	return m.proxies[id], nil
}

func (m *mockProxyRepoForJob) GetByIDWithRelations(ctx context.Context, id models.ULID) (*models.StreamProxy, error) {
	return nil, nil
}

func (m *mockProxyRepoForJob) GetAll(ctx context.Context) ([]*models.StreamProxy, error) {
	return nil, nil
}

func (m *mockProxyRepoForJob) GetActive(ctx context.Context) ([]*models.StreamProxy, error) {
	return nil, nil
}

func (m *mockProxyRepoForJob) Update(ctx context.Context, proxy *models.StreamProxy) error {
	return nil
}

func (m *mockProxyRepoForJob) Delete(ctx context.Context, id models.ULID) error {
	return nil
}

func (m *mockProxyRepoForJob) GetByName(ctx context.Context, name string) (*models.StreamProxy, error) {
	return nil, nil
}

func (m *mockProxyRepoForJob) UpdateStatus(ctx context.Context, id models.ULID, status models.StreamProxyStatus, lastError string) error {
	return nil
}

func (m *mockProxyRepoForJob) GetBySourceID(ctx context.Context, sourceID models.ULID) ([]*models.StreamProxy, error) {
	return nil, nil
}

func (m *mockProxyRepoForJob) GetByEpgSourceID(ctx context.Context, epgSourceID models.ULID) ([]*models.StreamProxy, error) {
	return nil, nil
}

func (m *mockProxyRepoForJob) UpdateLastGeneration(ctx context.Context, id models.ULID, channelCount, programCount int) error {
	return nil
}

func (m *mockProxyRepoForJob) SetSources(ctx context.Context, proxyID models.ULID, sourceIDs []models.ULID, priorities map[models.ULID]int) error {
	return nil
}

func (m *mockProxyRepoForJob) SetEpgSources(ctx context.Context, proxyID models.ULID, sourceIDs []models.ULID, priorities map[models.ULID]int) error {
	return nil
}

func (m *mockProxyRepoForJob) GetSources(ctx context.Context, proxyID models.ULID) ([]*models.StreamSource, error) {
	return nil, nil
}

func (m *mockProxyRepoForJob) GetEpgSources(ctx context.Context, proxyID models.ULID) ([]*models.EpgSource, error) {
	return nil, nil
}

func (m *mockProxyRepoForJob) GetFilters(ctx context.Context, proxyID models.ULID) ([]*models.Filter, error) {
	return nil, nil
}

func (m *mockProxyRepoForJob) SetFilters(ctx context.Context, proxyID models.ULID, filterIDs []models.ULID, orders map[models.ULID]int, isActive map[models.ULID]bool) error {
	return nil
}

func (m *mockProxyRepoForJob) CountByEncodingProfileID(ctx context.Context, profileID models.ULID) (int64, error) {
	return 0, nil
}

func (m *mockProxyRepoForJob) GetByEncodingProfileID(ctx context.Context, profileID models.ULID) ([]*models.StreamProxy, error) {
	return nil, nil
}

func createTestJobService(jobRepo repository.JobRepository, streamRepo repository.StreamSourceRepository, epgRepo repository.EpgSourceRepository, proxyRepo repository.StreamProxyRepository) *service.JobService {
	svc := service.NewJobService(jobRepo, streamRepo, epgRepo, proxyRepo)
	sched := scheduler.NewScheduler(jobRepo, streamRepo, epgRepo, proxyRepo)
	svc.WithScheduler(sched)
	return svc
}

func TestJobHandler_List(t *testing.T) {
	jobRepo := newMockJobRepo()
	svc := createTestJobService(jobRepo, newMockStreamSourceRepoForJob(), newMockEpgSourceRepoForJob(), newMockProxyRepoForJob())
	handler := NewJobHandler(svc)

	ctx := context.Background()

	// Create test jobs
	job1 := &models.Job{Type: models.JobTypeStreamIngestion, Status: models.JobStatusPending}
	job1.ID = models.NewULID()
	job2 := &models.Job{Type: models.JobTypeEpgIngestion, Status: models.JobStatusRunning}
	job2.ID = models.NewULID()

	jobRepo.jobs[job1.ID] = job1
	jobRepo.jobs[job2.ID] = job2

	resp, err := handler.List(ctx, &ListJobsInput{})
	require.NoError(t, err)
	assert.Len(t, resp.Body.Jobs, 2)
}

func TestJobHandler_GetByID(t *testing.T) {
	jobRepo := newMockJobRepo()
	svc := createTestJobService(jobRepo, newMockStreamSourceRepoForJob(), newMockEpgSourceRepoForJob(), newMockProxyRepoForJob())
	handler := NewJobHandler(svc)

	ctx := context.Background()

	job := &models.Job{
		Type:       models.JobTypeStreamIngestion,
		TargetName: "Test Source",
		Status:     models.JobStatusPending,
	}
	job.ID = models.NewULID()
	jobRepo.jobs[job.ID] = job

	t.Run("found", func(t *testing.T) {
		resp, err := handler.GetByID(ctx, &GetJobInput{ID: job.ID.String()})
		require.NoError(t, err)
		assert.Equal(t, job.ID, resp.Body.ID)
		assert.Equal(t, job.Type, resp.Body.Type)
	})

	t.Run("not found", func(t *testing.T) {
		_, err := handler.GetByID(ctx, &GetJobInput{ID: models.NewULID().String()})
		assert.Error(t, err)
	})

	t.Run("invalid id", func(t *testing.T) {
		_, err := handler.GetByID(ctx, &GetJobInput{ID: "invalid"})
		assert.Error(t, err)
	})
}

func TestJobHandler_ListByType(t *testing.T) {
	jobRepo := newMockJobRepo()
	svc := createTestJobService(jobRepo, newMockStreamSourceRepoForJob(), newMockEpgSourceRepoForJob(), newMockProxyRepoForJob())
	handler := NewJobHandler(svc)

	ctx := context.Background()

	// Create jobs of different types
	job1 := &models.Job{Type: models.JobTypeStreamIngestion, Status: models.JobStatusPending}
	job1.ID = models.NewULID()
	job2 := &models.Job{Type: models.JobTypeStreamIngestion, Status: models.JobStatusRunning}
	job2.ID = models.NewULID()
	job3 := &models.Job{Type: models.JobTypeEpgIngestion, Status: models.JobStatusPending}
	job3.ID = models.NewULID()

	jobRepo.jobs[job1.ID] = job1
	jobRepo.jobs[job2.ID] = job2
	jobRepo.jobs[job3.ID] = job3

	resp, err := handler.ListByType(ctx, &ListJobsByTypeInput{Type: string(models.JobTypeStreamIngestion)})
	require.NoError(t, err)
	assert.Len(t, resp.Body.Jobs, 2)
	for _, j := range resp.Body.Jobs {
		assert.Equal(t, models.JobTypeStreamIngestion, j.Type)
	}
}

func TestJobHandler_ListPending(t *testing.T) {
	jobRepo := newMockJobRepo()
	svc := createTestJobService(jobRepo, newMockStreamSourceRepoForJob(), newMockEpgSourceRepoForJob(), newMockProxyRepoForJob())
	handler := NewJobHandler(svc)

	ctx := context.Background()

	// Create jobs with different statuses
	pending := &models.Job{Type: models.JobTypeStreamIngestion, Status: models.JobStatusPending}
	pending.ID = models.NewULID()
	running := &models.Job{Type: models.JobTypeEpgIngestion, Status: models.JobStatusRunning}
	running.ID = models.NewULID()
	completed := &models.Job{Type: models.JobTypeProxyGeneration, Status: models.JobStatusCompleted}
	completed.ID = models.NewULID()

	jobRepo.jobs[pending.ID] = pending
	jobRepo.jobs[running.ID] = running
	jobRepo.jobs[completed.ID] = completed

	resp, err := handler.ListPending(ctx, &ListPendingJobsInput{})
	require.NoError(t, err)
	assert.Len(t, resp.Body.Jobs, 1)
	assert.Equal(t, models.JobStatusPending, resp.Body.Jobs[0].Status)
}

func TestJobHandler_ListRunning(t *testing.T) {
	jobRepo := newMockJobRepo()
	svc := createTestJobService(jobRepo, newMockStreamSourceRepoForJob(), newMockEpgSourceRepoForJob(), newMockProxyRepoForJob())
	handler := NewJobHandler(svc)

	ctx := context.Background()

	// Create jobs with different statuses
	pending := &models.Job{Type: models.JobTypeStreamIngestion, Status: models.JobStatusPending}
	pending.ID = models.NewULID()
	running := &models.Job{Type: models.JobTypeEpgIngestion, Status: models.JobStatusRunning}
	running.ID = models.NewULID()

	jobRepo.jobs[pending.ID] = pending
	jobRepo.jobs[running.ID] = running

	resp, err := handler.ListRunning(ctx, &ListRunningJobsInput{})
	require.NoError(t, err)
	assert.Len(t, resp.Body.Jobs, 1)
	assert.Equal(t, models.JobStatusRunning, resp.Body.Jobs[0].Status)
}

func TestJobHandler_GetHistory(t *testing.T) {
	jobRepo := newMockJobRepo()
	svc := createTestJobService(jobRepo, newMockStreamSourceRepoForJob(), newMockEpgSourceRepoForJob(), newMockProxyRepoForJob())
	handler := NewJobHandler(svc)

	ctx := context.Background()

	// Create history records
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

	resp, err := handler.GetHistory(ctx, &GetJobHistoryInput{Offset: 0, Limit: 50})
	require.NoError(t, err)
	assert.Len(t, resp.Body.History, 5)
	assert.Equal(t, int64(5), resp.Body.Pagination.TotalItems)
}

func TestJobHandler_GetStats(t *testing.T) {
	jobRepo := newMockJobRepo()
	svc := createTestJobService(jobRepo, newMockStreamSourceRepoForJob(), newMockEpgSourceRepoForJob(), newMockProxyRepoForJob())
	handler := NewJobHandler(svc)

	ctx := context.Background()

	// Create jobs with various statuses
	jobs := []*models.Job{
		{Type: models.JobTypeStreamIngestion, Status: models.JobStatusPending},
		{Type: models.JobTypeStreamIngestion, Status: models.JobStatusRunning},
		{Type: models.JobTypeEpgIngestion, Status: models.JobStatusCompleted},
		{Type: models.JobTypeEpgIngestion, Status: models.JobStatusFailed},
	}

	for _, j := range jobs {
		j.ID = models.NewULID()
		jobRepo.jobs[j.ID] = j
	}

	resp, err := handler.GetStats(ctx, &GetJobStatsInput{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), resp.Body.PendingCount)
	assert.Equal(t, int64(1), resp.Body.RunningCount)
	assert.Equal(t, int64(1), resp.Body.CompletedCount)
	assert.Equal(t, int64(1), resp.Body.FailedCount)
}

func TestJobHandler_Cancel(t *testing.T) {
	jobRepo := newMockJobRepo()
	svc := createTestJobService(jobRepo, newMockStreamSourceRepoForJob(), newMockEpgSourceRepoForJob(), newMockProxyRepoForJob())
	handler := NewJobHandler(svc)

	ctx := context.Background()

	t.Run("cancel pending job", func(t *testing.T) {
		job := &models.Job{
			Type:   models.JobTypeStreamIngestion,
			Status: models.JobStatusPending,
		}
		job.ID = models.NewULID()
		jobRepo.jobs[job.ID] = job

		resp, err := handler.Cancel(ctx, &CancelJobInput{ID: job.ID.String()})
		require.NoError(t, err)
		assert.Contains(t, resp.Body.Message, "cancelled")
		assert.Equal(t, models.JobStatusCancelled, job.Status)
	})

	t.Run("cannot cancel completed job", func(t *testing.T) {
		job := &models.Job{
			Type:   models.JobTypeStreamIngestion,
			Status: models.JobStatusCompleted,
		}
		job.ID = models.NewULID()
		jobRepo.jobs[job.ID] = job

		_, err := handler.Cancel(ctx, &CancelJobInput{ID: job.ID.String()})
		assert.Error(t, err)
	})

	t.Run("job not found", func(t *testing.T) {
		_, err := handler.Cancel(ctx, &CancelJobInput{ID: models.NewULID().String()})
		assert.Error(t, err)
	})
}

func TestJobHandler_Delete(t *testing.T) {
	jobRepo := newMockJobRepo()
	svc := createTestJobService(jobRepo, newMockStreamSourceRepoForJob(), newMockEpgSourceRepoForJob(), newMockProxyRepoForJob())
	handler := NewJobHandler(svc)

	ctx := context.Background()

	t.Run("delete completed job", func(t *testing.T) {
		job := &models.Job{
			Type:   models.JobTypeStreamIngestion,
			Status: models.JobStatusCompleted,
		}
		job.ID = models.NewULID()
		jobRepo.jobs[job.ID] = job

		_, err := handler.Delete(ctx, &DeleteJobInput{ID: job.ID.String()})
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

		_, err := handler.Delete(ctx, &DeleteJobInput{ID: job.ID.String()})
		assert.Error(t, err)
	})
}

func TestJobHandler_TriggerStreamIngestion(t *testing.T) {
	jobRepo := newMockJobRepo()
	streamRepo := newMockStreamSourceRepoForJob()
	svc := createTestJobService(jobRepo, streamRepo, newMockEpgSourceRepoForJob(), newMockProxyRepoForJob())
	handler := NewJobHandler(svc)

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		sourceID := models.NewULID()
		source := &models.StreamSource{Name: "Test Source", Enabled: models.BoolPtr(true)}
		source.ID = sourceID
		streamRepo.sources[sourceID] = source

		resp, err := handler.TriggerStreamIngestion(ctx, &TriggerStreamIngestionInput{ID: sourceID.String()})
		require.NoError(t, err)
		assert.Equal(t, models.JobTypeStreamIngestion, resp.Body.Type)
		assert.Equal(t, sourceID, resp.Body.TargetID)
	})

	t.Run("source not found", func(t *testing.T) {
		_, err := handler.TriggerStreamIngestion(ctx, &TriggerStreamIngestionInput{ID: models.NewULID().String()})
		assert.Error(t, err)
	})
}

func TestJobHandler_TriggerEpgIngestion(t *testing.T) {
	jobRepo := newMockJobRepo()
	epgRepo := newMockEpgSourceRepoForJob()
	svc := createTestJobService(jobRepo, newMockStreamSourceRepoForJob(), epgRepo, newMockProxyRepoForJob())
	handler := NewJobHandler(svc)

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		sourceID := models.NewULID()
		source := &models.EpgSource{Name: "Test EPG", Enabled: models.BoolPtr(true)}
		source.ID = sourceID
		epgRepo.sources[sourceID] = source

		resp, err := handler.TriggerEpgIngestion(ctx, &TriggerEpgIngestionInput{ID: sourceID.String()})
		require.NoError(t, err)
		assert.Equal(t, models.JobTypeEpgIngestion, resp.Body.Type)
		assert.Equal(t, sourceID, resp.Body.TargetID)
	})

	t.Run("source not found", func(t *testing.T) {
		_, err := handler.TriggerEpgIngestion(ctx, &TriggerEpgIngestionInput{ID: models.NewULID().String()})
		assert.Error(t, err)
	})
}

func TestJobHandler_TriggerProxyGeneration(t *testing.T) {
	jobRepo := newMockJobRepo()
	proxyRepo := newMockProxyRepoForJob()
	svc := createTestJobService(jobRepo, newMockStreamSourceRepoForJob(), newMockEpgSourceRepoForJob(), proxyRepo)
	handler := NewJobHandler(svc)

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		proxyID := models.NewULID()
		proxy := &models.StreamProxy{Name: "Test Proxy", IsActive: models.BoolPtr(true)}
		proxy.ID = proxyID
		proxyRepo.proxies[proxyID] = proxy

		resp, err := handler.TriggerProxyGeneration(ctx, &TriggerProxyGenerationInput{ID: proxyID.String()})
		require.NoError(t, err)
		assert.Equal(t, models.JobTypeProxyGeneration, resp.Body.Type)
		assert.Equal(t, proxyID, resp.Body.TargetID)
	})

	t.Run("proxy not found", func(t *testing.T) {
		_, err := handler.TriggerProxyGeneration(ctx, &TriggerProxyGenerationInput{ID: models.NewULID().String()})
		assert.Error(t, err)
	})
}

func TestJobHandler_ValidateCron(t *testing.T) {
	jobRepo := newMockJobRepo()
	svc := createTestJobService(jobRepo, newMockStreamSourceRepoForJob(), newMockEpgSourceRepoForJob(), newMockProxyRepoForJob())
	handler := NewJobHandler(svc)

	ctx := context.Background()

	t.Run("valid cron", func(t *testing.T) {
		resp, err := handler.ValidateCron(ctx, &ValidateCronInput{
			Body: ValidateCronRequest{Expression: "0 0 */6 * * *"}, // 6-field cron: second minute hour day-of-month month day-of-week
		})
		require.NoError(t, err)
		assert.True(t, resp.Body.Valid)
		assert.NotNil(t, resp.Body.NextRun)
	})

	t.Run("invalid cron", func(t *testing.T) {
		resp, err := handler.ValidateCron(ctx, &ValidateCronInput{
			Body: ValidateCronRequest{Expression: "invalid"},
		})
		require.NoError(t, err) // Returns validation result, not error
		assert.False(t, resp.Body.Valid)
		assert.NotEmpty(t, resp.Body.Error)
	})
}

func TestJobFromModel(t *testing.T) {
	now := models.Now()
	nextRun := models.Now().Add(time.Hour)
	started := models.Now().Add(-10 * time.Minute)
	completed := models.Now().Add(-5 * time.Minute)
	locked := models.Now().Add(-8 * time.Minute)

	job := &models.Job{
		Type:           models.JobTypeStreamIngestion,
		TargetID:       models.NewULID(),
		TargetName:     "Test Source",
		Status:         models.JobStatusCompleted,
		CronSchedule:   "0 */6 * * *",
		NextRunAt:      &nextRun,
		StartedAt:      &started,
		CompletedAt:    &completed,
		DurationMs:     300000,
		AttemptCount:   1,
		MaxAttempts:    3,
		BackoffSeconds: 60,
		LastError:      "",
		Result:         "success",
		Priority:       5,
		LockedBy:       "worker-1",
		LockedAt:       &locked,
	}
	job.ID = models.NewULID()
	job.CreatedAt = now
	job.UpdatedAt = now

	resp := JobFromModel(job)

	assert.Equal(t, job.ID, resp.ID)
	assert.Equal(t, job.Type, resp.Type)
	assert.Equal(t, job.TargetID, resp.TargetID)
	assert.Equal(t, job.TargetName, resp.TargetName)
	assert.Equal(t, job.Status, resp.Status)
	assert.Equal(t, job.CronSchedule, resp.CronSchedule)
	assert.NotNil(t, resp.NextRunAt)
	assert.NotNil(t, resp.StartedAt)
	assert.NotNil(t, resp.CompletedAt)
	assert.NotNil(t, resp.LockedAt)
	assert.Equal(t, job.DurationMs, resp.DurationMs)
	assert.Equal(t, job.AttemptCount, resp.AttemptCount)
	assert.Equal(t, job.MaxAttempts, resp.MaxAttempts)
	assert.Equal(t, job.BackoffSeconds, resp.BackoffSeconds)
	assert.Equal(t, job.Result, resp.Result)
	assert.Equal(t, job.Priority, resp.Priority)
	assert.Equal(t, job.LockedBy, resp.LockedBy)
}

func TestJobHistoryFromModel(t *testing.T) {
	now := models.Now()
	started := models.Now().Add(-10 * time.Minute)
	completed := models.Now().Add(-5 * time.Minute)

	history := &models.JobHistory{
		JobID:         models.NewULID(),
		Type:          models.JobTypeStreamIngestion,
		TargetID:      models.NewULID(),
		TargetName:    "Test Source",
		Status:        models.JobStatusCompleted,
		StartedAt:     &started,
		CompletedAt:   &completed,
		DurationMs:    300000,
		AttemptNumber: 1,
		Error:         "",
		Result:        "ingested 100 channels",
	}
	history.ID = models.NewULID()
	history.CreatedAt = now

	resp := JobHistoryFromModel(history)

	assert.Equal(t, history.ID, resp.ID)
	assert.Equal(t, history.JobID, resp.JobID)
	assert.Equal(t, history.Type, resp.Type)
	assert.Equal(t, history.TargetID, resp.TargetID)
	assert.Equal(t, history.TargetName, resp.TargetName)
	assert.Equal(t, history.Status, resp.Status)
	assert.NotNil(t, resp.StartedAt)
	assert.NotNil(t, resp.CompletedAt)
	assert.Equal(t, history.DurationMs, resp.DurationMs)
	assert.Equal(t, history.AttemptNumber, resp.AttemptNumber)
	assert.Equal(t, history.Result, resp.Result)
}
