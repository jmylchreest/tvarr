package models

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJob_TableName(t *testing.T) {
	job := Job{}
	assert.Equal(t, "jobs", job.TableName())
}

func TestJobHistory_TableName(t *testing.T) {
	history := JobHistory{}
	assert.Equal(t, "job_history", history.TableName())
}

func TestJob_IsRecurring(t *testing.T) {
	tests := []struct {
		name         string
		cronSchedule string
		want         bool
	}{
		{
			name:         "recurring job with cron schedule",
			cronSchedule: "0 */6 * * *",
			want:         true,
		},
		{
			name:         "one-off job without cron schedule",
			cronSchedule: "",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &Job{CronSchedule: tt.cronSchedule}
			assert.Equal(t, tt.want, job.IsRecurring())
			assert.Equal(t, !tt.want, job.IsOneOff())
		})
	}
}

func TestJob_StatusChecks(t *testing.T) {
	tests := []struct {
		name       string
		status     JobStatus
		isPending  bool
		isRunning  bool
		isFinished bool
	}{
		{
			name:       "pending status",
			status:     JobStatusPending,
			isPending:  true,
			isRunning:  false,
			isFinished: false,
		},
		{
			name:       "scheduled status",
			status:     JobStatusScheduled,
			isPending:  true,
			isRunning:  false,
			isFinished: false,
		},
		{
			name:       "running status",
			status:     JobStatusRunning,
			isPending:  false,
			isRunning:  true,
			isFinished: false,
		},
		{
			name:       "completed status",
			status:     JobStatusCompleted,
			isPending:  false,
			isRunning:  false,
			isFinished: true,
		},
		{
			name:       "failed status",
			status:     JobStatusFailed,
			isPending:  false,
			isRunning:  false,
			isFinished: true,
		},
		{
			name:       "cancelled status",
			status:     JobStatusCancelled,
			isPending:  false,
			isRunning:  false,
			isFinished: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &Job{Status: tt.status}
			assert.Equal(t, tt.isPending, job.IsPending(), "IsPending")
			assert.Equal(t, tt.isRunning, job.IsRunning(), "IsRunning")
			assert.Equal(t, tt.isFinished, job.IsFinished(), "IsFinished")
		})
	}
}

func TestJob_CanRetry(t *testing.T) {
	tests := []struct {
		name         string
		status       JobStatus
		attemptCount int
		maxAttempts  int
		want         bool
	}{
		{
			name:         "failed with attempts remaining",
			status:       JobStatusFailed,
			attemptCount: 1,
			maxAttempts:  3,
			want:         true,
		},
		{
			name:         "failed with no attempts remaining",
			status:       JobStatusFailed,
			attemptCount: 3,
			maxAttempts:  3,
			want:         false,
		},
		{
			name:         "completed cannot retry",
			status:       JobStatusCompleted,
			attemptCount: 1,
			maxAttempts:  3,
			want:         false,
		},
		{
			name:         "running cannot retry",
			status:       JobStatusRunning,
			attemptCount: 1,
			maxAttempts:  3,
			want:         false,
		},
		{
			name:         "no max attempts",
			status:       JobStatusFailed,
			attemptCount: 1,
			maxAttempts:  0,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &Job{
				Status:       tt.status,
				AttemptCount: tt.attemptCount,
				MaxAttempts:  tt.maxAttempts,
			}
			assert.Equal(t, tt.want, job.CanRetry())
		})
	}
}

func TestJob_MarkRunning(t *testing.T) {
	job := &Job{
		Status:       JobStatusPending,
		AttemptCount: 0,
		LastError:    "previous error",
	}

	job.MarkRunning("worker-1")

	assert.Equal(t, JobStatusRunning, job.Status)
	assert.Equal(t, 1, job.AttemptCount)
	assert.Equal(t, "worker-1", job.LockedBy)
	assert.NotNil(t, job.StartedAt)
	assert.NotNil(t, job.LockedAt)
	assert.Empty(t, job.LastError)
}

func TestJob_MarkCompleted(t *testing.T) {
	startTime := Now()
	job := &Job{
		Status:    JobStatusRunning,
		StartedAt: &startTime,
		LockedBy:  "worker-1",
	}

	// Wait a tiny bit to ensure duration is measurable
	time.Sleep(time.Millisecond)
	job.MarkCompleted("processed 100 channels")

	assert.Equal(t, JobStatusCompleted, job.Status)
	assert.NotNil(t, job.CompletedAt)
	assert.Equal(t, "processed 100 channels", job.Result)
	assert.Empty(t, job.LockedBy)
	assert.Nil(t, job.LockedAt)
	assert.GreaterOrEqual(t, job.DurationMs, int64(0))
}

func TestJob_MarkFailed(t *testing.T) {
	startTime := Now()
	job := &Job{
		Status:    JobStatusRunning,
		StartedAt: &startTime,
		LockedBy:  "worker-1",
	}

	testErr := errors.New("connection timeout")
	job.MarkFailed(testErr)

	assert.Equal(t, JobStatusFailed, job.Status)
	assert.NotNil(t, job.CompletedAt)
	assert.Equal(t, "connection timeout", job.LastError)
	assert.Empty(t, job.LockedBy)
	assert.Nil(t, job.LockedAt)
}

func TestJob_MarkCancelled(t *testing.T) {
	job := &Job{
		Status:   JobStatusRunning,
		LockedBy: "worker-1",
	}

	job.MarkCancelled()

	assert.Equal(t, JobStatusCancelled, job.Status)
	assert.NotNil(t, job.CompletedAt)
	assert.Empty(t, job.LockedBy)
	assert.Nil(t, job.LockedAt)
}

func TestJob_CalculateNextBackoff(t *testing.T) {
	tests := []struct {
		name           string
		backoffSeconds int
		attemptCount   int
		wantMin        time.Duration
		wantMax        time.Duration
	}{
		{
			name:           "first retry with 60s base",
			backoffSeconds: 60,
			attemptCount:   1,
			wantMin:        60 * time.Second,
			wantMax:        60 * time.Second,
		},
		{
			name:           "second retry doubles backoff",
			backoffSeconds: 60,
			attemptCount:   2,
			wantMin:        120 * time.Second,
			wantMax:        120 * time.Second,
		},
		{
			name:           "third retry quadruples backoff",
			backoffSeconds: 60,
			attemptCount:   3,
			wantMin:        240 * time.Second,
			wantMax:        240 * time.Second,
		},
		{
			name:           "backoff capped at 1 hour",
			backoffSeconds: 60,
			attemptCount:   10,
			wantMin:        3600 * time.Second,
			wantMax:        3600 * time.Second,
		},
		{
			name:           "default backoff when zero",
			backoffSeconds: 0,
			attemptCount:   1,
			wantMin:        60 * time.Second,
			wantMax:        60 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &Job{
				BackoffSeconds: tt.backoffSeconds,
				AttemptCount:   tt.attemptCount,
			}
			backoff := job.CalculateNextBackoff()
			assert.GreaterOrEqual(t, backoff, tt.wantMin)
			assert.LessOrEqual(t, backoff, tt.wantMax)
		})
	}
}

func TestJob_ScheduleRetry(t *testing.T) {
	t.Run("schedules retry when can retry", func(t *testing.T) {
		job := &Job{
			Status:         JobStatusFailed,
			AttemptCount:   1,
			MaxAttempts:    3,
			BackoffSeconds: 60,
			LockedBy:       "worker-1",
		}

		job.ScheduleRetry()

		assert.Equal(t, JobStatusScheduled, job.Status)
		assert.NotNil(t, job.NextRunAt)
		assert.Empty(t, job.LockedBy)
		assert.Nil(t, job.LockedAt)
	})

	t.Run("does not schedule retry when cannot retry", func(t *testing.T) {
		job := &Job{
			Status:       JobStatusFailed,
			AttemptCount: 3,
			MaxAttempts:  3,
		}

		job.ScheduleRetry()

		// Status should remain failed
		assert.Equal(t, JobStatusFailed, job.Status)
		assert.Nil(t, job.NextRunAt)
	})
}

func TestJob_Validate(t *testing.T) {
	tests := []struct {
		name    string
		job     *Job
		wantErr error
	}{
		{
			name: "valid job",
			job: &Job{
				Type:     JobTypeStreamIngestion,
				TargetID: NewULID(),
			},
			wantErr: nil,
		},
		{
			name: "missing type",
			job: &Job{
				TargetID: NewULID(),
			},
			wantErr: ErrJobTypeRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.job.Validate()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewJobFromSource(t *testing.T) {
	source := &StreamSource{
		Name: "Test Source",
	}
	source.ID = NewULID()

	job := NewJobFromSource(source, "0 */6 * * *")

	assert.Equal(t, JobTypeStreamIngestion, job.Type)
	assert.Equal(t, source.ID, job.TargetID)
	assert.Equal(t, source.Name, job.TargetName)
	assert.Equal(t, "0 */6 * * *", job.CronSchedule)
}

func TestNewJobFromEpgSource(t *testing.T) {
	source := &EpgSource{
		Name: "Test EPG Source",
	}
	source.ID = NewULID()

	job := NewJobFromEpgSource(source, "0 */12 * * *")

	assert.Equal(t, JobTypeEpgIngestion, job.Type)
	assert.Equal(t, source.ID, job.TargetID)
	assert.Equal(t, source.Name, job.TargetName)
	assert.Equal(t, "0 */12 * * *", job.CronSchedule)
}

func TestNewJobFromProxy(t *testing.T) {
	proxy := &StreamProxy{
		Name: "Test Proxy",
	}
	proxy.ID = NewULID()

	job := NewJobFromProxy(proxy, "*/30 * * * *")

	assert.Equal(t, JobTypeProxyGeneration, job.Type)
	assert.Equal(t, proxy.ID, job.TargetID)
	assert.Equal(t, proxy.Name, job.TargetName)
	assert.Equal(t, "*/30 * * * *", job.CronSchedule)
}

func TestJob_JobTypes(t *testing.T) {
	// Verify job type constants are correct
	assert.Equal(t, JobType("stream_ingestion"), JobTypeStreamIngestion)
	assert.Equal(t, JobType("epg_ingestion"), JobTypeEpgIngestion)
	assert.Equal(t, JobType("proxy_generation"), JobTypeProxyGeneration)
	assert.Equal(t, JobType("logo_cleanup"), JobTypeLogoCleanup)
}

func TestJob_JobStatuses(t *testing.T) {
	// Verify job status constants are correct
	assert.Equal(t, JobStatus("pending"), JobStatusPending)
	assert.Equal(t, JobStatus("scheduled"), JobStatusScheduled)
	assert.Equal(t, JobStatus("running"), JobStatusRunning)
	assert.Equal(t, JobStatus("completed"), JobStatusCompleted)
	assert.Equal(t, JobStatus("failed"), JobStatusFailed)
	assert.Equal(t, JobStatus("cancelled"), JobStatusCancelled)
}

func TestJob_Integration(t *testing.T) {
	// Integration test: simulate job lifecycle
	job := &Job{
		Type:           JobTypeStreamIngestion,
		TargetID:       NewULID(),
		TargetName:     "Test Source",
		Status:         JobStatusPending,
		MaxAttempts:    3,
		BackoffSeconds: 10,
	}

	// Job starts
	require.True(t, job.IsPending())
	job.MarkRunning("worker-1")
	require.True(t, job.IsRunning())
	require.Equal(t, 1, job.AttemptCount)

	// First attempt fails
	job.MarkFailed(errors.New("network error"))
	require.True(t, job.IsFinished())
	require.True(t, job.CanRetry())

	// Schedule retry
	job.ScheduleRetry()
	require.Equal(t, JobStatusScheduled, job.Status)
	require.NotNil(t, job.NextRunAt)

	// Second attempt
	job.MarkRunning("worker-2")
	require.Equal(t, 2, job.AttemptCount)

	// Second attempt fails
	job.MarkFailed(errors.New("timeout"))
	require.True(t, job.CanRetry())

	// Third attempt succeeds
	job.MarkRunning("worker-1")
	require.Equal(t, 3, job.AttemptCount)
	job.MarkCompleted("ingested 500 channels")
	require.True(t, job.IsFinished())
	require.False(t, job.CanRetry())
	require.Equal(t, "ingested 500 channels", job.Result)
}
