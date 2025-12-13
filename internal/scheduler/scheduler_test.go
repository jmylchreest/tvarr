package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockJobRepo implements repository.JobRepository for testing.
type mockJobRepo struct {
	jobs           map[models.ULID]*models.Job
	history        []*models.JobHistory
	acquireErr     error
	acquireReturns *models.Job
}

func newMockJobRepo() *mockJobRepo {
	return &mockJobRepo{
		jobs: make(map[models.ULID]*models.Job),
	}
}

func (m *mockJobRepo) Create(ctx context.Context, job *models.Job) error {
	if job.ID.IsZero() {
		job.ID = models.NewULID()
	}
	m.jobs[job.ID] = job
	return nil
}

func (m *mockJobRepo) GetByID(ctx context.Context, id models.ULID) (*models.Job, error) {
	return m.jobs[id], nil
}

func (m *mockJobRepo) GetAll(ctx context.Context) ([]*models.Job, error) {
	var jobs []*models.Job
	for _, j := range m.jobs {
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (m *mockJobRepo) GetPending(ctx context.Context) ([]*models.Job, error) {
	var jobs []*models.Job
	for _, j := range m.jobs {
		if j.Status == models.JobStatusPending || j.Status == models.JobStatusScheduled {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

func (m *mockJobRepo) GetByStatus(ctx context.Context, status models.JobStatus) ([]*models.Job, error) {
	var jobs []*models.Job
	for _, j := range m.jobs {
		if j.Status == status {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

func (m *mockJobRepo) GetByType(ctx context.Context, jobType models.JobType) ([]*models.Job, error) {
	var jobs []*models.Job
	for _, j := range m.jobs {
		if j.Type == jobType {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

func (m *mockJobRepo) GetByTargetID(ctx context.Context, targetID models.ULID) ([]*models.Job, error) {
	var jobs []*models.Job
	for _, j := range m.jobs {
		if j.TargetID == targetID {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

func (m *mockJobRepo) GetRunning(ctx context.Context) ([]*models.Job, error) {
	var jobs []*models.Job
	for _, j := range m.jobs {
		if j.Status == models.JobStatusRunning {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

func (m *mockJobRepo) Update(ctx context.Context, job *models.Job) error {
	m.jobs[job.ID] = job
	return nil
}

func (m *mockJobRepo) Delete(ctx context.Context, id models.ULID) error {
	delete(m.jobs, id)
	return nil
}

func (m *mockJobRepo) DeleteCompleted(ctx context.Context, before time.Time) (int64, error) {
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
	if m.acquireErr != nil {
		return nil, m.acquireErr
	}
	if m.acquireReturns != nil {
		return m.acquireReturns, nil
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

func (m *mockJobRepo) ReleaseJob(ctx context.Context, id models.ULID) error {
	if j, ok := m.jobs[id]; ok {
		j.LockedBy = ""
		j.LockedAt = nil
		j.Status = models.JobStatusPending
	}
	return nil
}

func (m *mockJobRepo) FindDuplicatePending(ctx context.Context, jobType models.JobType, targetID models.ULID) (*models.Job, error) {
	for _, j := range m.jobs {
		if j.Type == jobType && j.TargetID == targetID && j.IsPending() {
			return j, nil
		}
	}
	return nil, nil
}

func (m *mockJobRepo) CreateHistory(ctx context.Context, history *models.JobHistory) error {
	if history.ID.IsZero() {
		history.ID = models.NewULID()
	}
	m.history = append(m.history, history)
	return nil
}

func (m *mockJobRepo) GetHistory(ctx context.Context, jobType *models.JobType, offset, limit int) ([]*models.JobHistory, int64, error) {
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

// mockStreamSourceRepo implements repository.StreamSourceRepository for testing.
type mockStreamSourceRepo struct {
	sources []*models.StreamSource
}

func (m *mockStreamSourceRepo) Create(ctx context.Context, source *models.StreamSource) error {
	return nil
}

func (m *mockStreamSourceRepo) GetByID(ctx context.Context, id models.ULID) (*models.StreamSource, error) {
	for _, s := range m.sources {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, nil
}

func (m *mockStreamSourceRepo) GetAll(ctx context.Context) ([]*models.StreamSource, error) {
	return m.sources, nil
}

func (m *mockStreamSourceRepo) GetEnabled(ctx context.Context) ([]*models.StreamSource, error) {
	var enabled []*models.StreamSource
	for _, s := range m.sources {
		if models.BoolVal(s.Enabled) {
			enabled = append(enabled, s)
		}
	}
	return enabled, nil
}

func (m *mockStreamSourceRepo) Update(ctx context.Context, source *models.StreamSource) error {
	return nil
}

func (m *mockStreamSourceRepo) Delete(ctx context.Context, id models.ULID) error {
	return nil
}

func (m *mockStreamSourceRepo) GetByName(ctx context.Context, name string) (*models.StreamSource, error) {
	return nil, nil
}

func (m *mockStreamSourceRepo) UpdateLastIngestion(ctx context.Context, id models.ULID, status string, channelCount int) error {
	return nil
}

// mockEpgSourceRepo implements repository.EpgSourceRepository for testing.
type mockEpgSourceRepo struct {
	sources []*models.EpgSource
}

func (m *mockEpgSourceRepo) Create(ctx context.Context, source *models.EpgSource) error {
	return nil
}

func (m *mockEpgSourceRepo) GetByID(ctx context.Context, id models.ULID) (*models.EpgSource, error) {
	return nil, nil
}

func (m *mockEpgSourceRepo) GetAll(ctx context.Context) ([]*models.EpgSource, error) {
	return m.sources, nil
}

func (m *mockEpgSourceRepo) GetEnabled(ctx context.Context) ([]*models.EpgSource, error) {
	var enabled []*models.EpgSource
	for _, s := range m.sources {
		if models.BoolVal(s.Enabled) {
			enabled = append(enabled, s)
		}
	}
	return enabled, nil
}

func (m *mockEpgSourceRepo) Update(ctx context.Context, source *models.EpgSource) error {
	return nil
}

func (m *mockEpgSourceRepo) Delete(ctx context.Context, id models.ULID) error {
	return nil
}

func (m *mockEpgSourceRepo) GetByName(ctx context.Context, name string) (*models.EpgSource, error) {
	return nil, nil
}

func (m *mockEpgSourceRepo) GetByURL(ctx context.Context, url string) (*models.EpgSource, error) {
	return nil, nil
}

func (m *mockEpgSourceRepo) UpdateLastIngestion(ctx context.Context, id models.ULID, status string, programCount int) error {
	return nil
}

// mockProxyRepo implements repository.StreamProxyRepository for testing.
type mockProxyRepo struct {
	proxies []*models.StreamProxy
}

func (m *mockProxyRepo) Create(ctx context.Context, proxy *models.StreamProxy) error {
	return nil
}

func (m *mockProxyRepo) GetByID(ctx context.Context, id models.ULID) (*models.StreamProxy, error) {
	return nil, nil
}

func (m *mockProxyRepo) GetByIDWithRelations(ctx context.Context, id models.ULID) (*models.StreamProxy, error) {
	return nil, nil
}

func (m *mockProxyRepo) GetAll(ctx context.Context) ([]*models.StreamProxy, error) {
	return m.proxies, nil
}

func (m *mockProxyRepo) GetActive(ctx context.Context) ([]*models.StreamProxy, error) {
	var active []*models.StreamProxy
	for _, p := range m.proxies {
		if models.BoolVal(p.IsActive) {
			active = append(active, p)
		}
	}
	return active, nil
}

func (m *mockProxyRepo) Update(ctx context.Context, proxy *models.StreamProxy) error {
	return nil
}

func (m *mockProxyRepo) Delete(ctx context.Context, id models.ULID) error {
	return nil
}

func (m *mockProxyRepo) GetByName(ctx context.Context, name string) (*models.StreamProxy, error) {
	return nil, nil
}

func (m *mockProxyRepo) UpdateStatus(ctx context.Context, id models.ULID, status models.StreamProxyStatus, lastError string) error {
	return nil
}

func (m *mockProxyRepo) UpdateLastGeneration(ctx context.Context, id models.ULID, channelCount, programCount int) error {
	return nil
}

func (m *mockProxyRepo) SetSources(ctx context.Context, proxyID models.ULID, sourceIDs []models.ULID, priorities map[models.ULID]int) error {
	return nil
}

func (m *mockProxyRepo) SetEpgSources(ctx context.Context, proxyID models.ULID, sourceIDs []models.ULID, priorities map[models.ULID]int) error {
	return nil
}

func (m *mockProxyRepo) GetSources(ctx context.Context, proxyID models.ULID) ([]*models.StreamSource, error) {
	return nil, nil
}

func (m *mockProxyRepo) GetEpgSources(ctx context.Context, proxyID models.ULID) ([]*models.EpgSource, error) {
	return nil, nil
}

func (m *mockProxyRepo) GetFilters(ctx context.Context, proxyID models.ULID) ([]*models.Filter, error) {
	return nil, nil
}

func (m *mockProxyRepo) SetFilters(ctx context.Context, proxyID models.ULID, filterIDs []models.ULID, orders map[models.ULID]int, isActive map[models.ULID]bool) error {
	return nil
}

func (m *mockProxyRepo) GetBySourceID(ctx context.Context, sourceID models.ULID) ([]*models.StreamProxy, error) {
	return nil, nil
}

func (m *mockProxyRepo) GetByEpgSourceID(ctx context.Context, epgSourceID models.ULID) ([]*models.StreamProxy, error) {
	return nil, nil
}

func (m *mockProxyRepo) CountByEncodingProfileID(ctx context.Context, profileID models.ULID) (int64, error) {
	return 0, nil
}

func (m *mockProxyRepo) GetByEncodingProfileID(ctx context.Context, profileID models.ULID) ([]*models.StreamProxy, error) {
	return nil, nil
}

func TestScheduler_ValidateCron(t *testing.T) {
	jobRepo := newMockJobRepo()
	scheduler := NewScheduler(jobRepo, &mockStreamSourceRepo{}, &mockEpgSourceRepo{}, &mockProxyRepo{})

	tests := []struct {
		name    string
		cron    string
		wantErr bool
	}{
		// 6-field format (default)
		{"valid 6-field every 6 hours", "0 0 */6 * * *", false},
		{"valid 6-field every minute", "0 * * * * *", false},
		{"valid 6-field daily at midnight", "0 0 0 * * *", false},
		{"valid 6-field weekly", "0 0 0 * * 0", false},
		// 7-field format (legacy with year)
		{"valid 7-field with year wildcard", "0 0 */6 * * * *", false},
		{"valid 7-field daily with year", "0 0 0 * * * *", false},
		{"valid 7-field with specific year", "0 0 0 * * * 2024", false},
		{"valid 7-field with year range", "0 0 0 * * * 2024-2030", false},
		// Special descriptors
		{"valid @every descriptor", "@every 1h", false},
		{"valid @daily descriptor", "@daily", false},
		// Invalid formats
		{"invalid format", "invalid", true},
		{"too few fields", "* * *", true},
		{"too many fields", "0 0 0 * * * * *", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := scheduler.ValidateCron(tt.cron)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestScheduler_ParseCron(t *testing.T) {
	jobRepo := newMockJobRepo()
	scheduler := NewScheduler(jobRepo, &mockStreamSourceRepo{}, &mockEpgSourceRepo{}, &mockProxyRepo{})

	// Test 6-field cron (default)
	nextRun, err := scheduler.ParseCron("0 0 */6 * * *")
	require.NoError(t, err)
	assert.True(t, nextRun.After(time.Now()))

	// Test 7-field cron (legacy) - should also work
	nextRun7, err := scheduler.ParseCron("0 0 */6 * * * *")
	require.NoError(t, err)
	assert.True(t, nextRun7.After(time.Now()))
}

func TestNormalizeCronExpression(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		// 6-field (pass through)
		{"6-field pass through", "0 0 */6 * * *", "0 0 */6 * * *", false},
		{"6-field every minute", "0 * * * * *", "0 * * * * *", false},
		// 7-field (strip year)
		{"7-field strip year wildcard", "0 0 */6 * * * *", "0 0 */6 * * *", false},
		{"7-field strip specific year", "0 0 0 * * * 2024", "0 0 0 * * *", false},
		{"7-field strip year range", "0 0 0 * * * 2024-2030", "0 0 0 * * *", false},
		// Special descriptors
		{"@every descriptor", "@every 1h", "@every 1h", false},
		{"@daily descriptor", "@daily", "@daily", false},
		// Invalid
		{"empty", "", "", true},
		{"5 fields", "0 0 * * *", "", true},
		{"8 fields", "0 0 0 * * * * *", "", true},
		{"invalid year field", "0 0 0 * * * invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeCronExpression(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestScheduler_ScheduleImmediate(t *testing.T) {
	jobRepo := newMockJobRepo()
	scheduler := NewScheduler(jobRepo, &mockStreamSourceRepo{}, &mockEpgSourceRepo{}, &mockProxyRepo{})
	ctx := context.Background()

	targetID := models.NewULID()

	// First call creates a new job
	job1, err := scheduler.ScheduleImmediate(ctx, models.JobTypeStreamIngestion, targetID, "Test Source")
	require.NoError(t, err)
	require.NotNil(t, job1)
	assert.Equal(t, models.JobTypeStreamIngestion, job1.Type)
	assert.Equal(t, targetID, job1.TargetID)
	assert.Equal(t, models.JobStatusPending, job1.Status)

	// Second call returns the existing job (deduplication)
	job2, err := scheduler.ScheduleImmediate(ctx, models.JobTypeStreamIngestion, targetID, "Test Source")
	require.NoError(t, err)
	require.NotNil(t, job2)
	assert.Equal(t, job1.ID, job2.ID)

	// Different type creates a new job
	job3, err := scheduler.ScheduleImmediate(ctx, models.JobTypeEpgIngestion, targetID, "Test Source")
	require.NoError(t, err)
	require.NotNil(t, job3)
	assert.NotEqual(t, job1.ID, job3.ID)
}

func TestScheduler_StartStop(t *testing.T) {
	jobRepo := newMockJobRepo()
	scheduler := NewScheduler(jobRepo, &mockStreamSourceRepo{}, &mockEpgSourceRepo{}, &mockProxyRepo{}).
		WithConfig(SchedulerConfig{SyncInterval: 100 * time.Millisecond})

	ctx := context.Background()

	// Start scheduler
	err := scheduler.Start(ctx)
	require.NoError(t, err)

	// Double start should error
	err = scheduler.Start(ctx)
	assert.Error(t, err)

	// Stop scheduler
	scheduler.Stop()

	// Can restart after stop
	err = scheduler.Start(ctx)
	require.NoError(t, err)
	scheduler.Stop()
}

func TestScheduler_LoadSchedules(t *testing.T) {
	jobRepo := newMockJobRepo()

	sourceID := models.NewULID()
	source := &models.StreamSource{
		Name:         "Test Source",
		Enabled:      models.BoolPtr(true),
		CronSchedule: "0 * * * * *", // Every minute (6-field with seconds)
	}
	source.ID = sourceID

	streamSourceRepo := &mockStreamSourceRepo{sources: []*models.StreamSource{source}}
	scheduler := NewScheduler(jobRepo, streamSourceRepo, &mockEpgSourceRepo{}, &mockProxyRepo{}).
		WithConfig(SchedulerConfig{SyncInterval: time.Minute})

	ctx := context.Background()

	// Load schedules (this registers cron entries but doesn't create jobs immediately)
	err := scheduler.ForceSync(ctx)
	require.NoError(t, err)

	// Should have registered the schedule
	assert.Equal(t, 1, scheduler.GetEntryCount())
}
