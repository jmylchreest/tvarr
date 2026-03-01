// Package ingestionguard implements the ingestion guard pipeline stage.
// This stage waits for any active ingestions to complete before allowing
// the pipeline to proceed, ensuring consistent data during generation.
package ingestionguard

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jmylchreest/tvarr/internal/ingestor"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/jmylchreest/tvarr/internal/pipeline/shared"
)

const (
	// StageID is the unique identifier for this stage.
	StageID = "ingestion_guard"
	// StageName is the human-readable name for this stage.
	StageName = "Ingestion Guard"

	// DefaultPollInterval is the default interval between checks.
	DefaultPollInterval = 1 * time.Second
	// DefaultMaxWaitTime is the default maximum time to wait for ingestions.
	DefaultMaxWaitTime = 30 * time.Minute
)

// StateChecker is an interface for checking ingestion state.
// This allows for testing without depending on the full StateManager.
type StateChecker interface {
	IsAnyIngesting() bool
	IsAnyIngestingForSources(sourceIDs []models.ULID) bool
	ActiveIngestionCount() int
	ActiveIngestionNamesForSources(sourceIDs []models.ULID) []string
	GetAllStates() []*ingestor.IngestionState
}

// PendingJobChecker checks for pending ingestion jobs in the job queue.
// This catches jobs that are queued but haven't been picked up by a worker yet.
type PendingJobChecker interface {
	HasPendingIngestionJobs(ctx context.Context, sourceIDs []models.ULID) (bool, error)
}

// Stage waits for active ingestions to complete before proceeding.
type Stage struct {
	shared.BaseStage
	stateChecker      StateChecker
	pendingJobChecker PendingJobChecker
	sourceIDs         []models.ULID // proxy-scoped source IDs to watch
	pollInterval      time.Duration
	maxWaitTime       time.Duration
	enabled           bool
	logger            *slog.Logger
}

// New creates a new ingestion guard stage.
func New(stateChecker StateChecker) *Stage {
	return &Stage{
		BaseStage:    shared.NewBaseStage(StageID, StageName),
		stateChecker: stateChecker,
		pollInterval: DefaultPollInterval,
		maxWaitTime:  DefaultMaxWaitTime,
		enabled:      true,
	}
}

// NewConstructor returns a stage constructor for use with the factory.
// The optional pendingJobChecker enables queue-aware guarding (checks for pending
// ingestion jobs in addition to active ingestions in the StateManager).
func NewConstructor(stateChecker StateChecker, pendingJobChecker PendingJobChecker) core.StageConstructor {
	return func(deps *core.Dependencies) core.Stage {
		s := New(stateChecker)
		if pendingJobChecker != nil {
			s.pendingJobChecker = pendingJobChecker
		}
		if deps.Logger != nil {
			s.logger = deps.Logger.With("stage", StageID)
		}
		return s
	}
}

// WithPollInterval sets the polling interval.
func (s *Stage) WithPollInterval(interval time.Duration) *Stage {
	if interval > 0 {
		s.pollInterval = interval
	}
	return s
}

// WithMaxWaitTime sets the maximum wait time.
func (s *Stage) WithMaxWaitTime(maxWait time.Duration) *Stage {
	if maxWait > 0 {
		s.maxWaitTime = maxWait
	}
	return s
}

// WithEnabled enables or disables the guard.
func (s *Stage) WithEnabled(enabled bool) *Stage {
	s.enabled = enabled
	return s
}

// WithLogger sets the logger.
func (s *Stage) WithLogger(logger *slog.Logger) *Stage {
	s.logger = logger.With("stage", StageID)
	return s
}

// WithSourceIDs sets the proxy-scoped source IDs to watch.
// When set, the guard only waits for these specific sources instead of all sources globally.
func (s *Stage) WithSourceIDs(sourceIDs []models.ULID) *Stage {
	s.sourceIDs = sourceIDs
	return s
}

// WithPendingJobChecker sets the pending job checker for queue-aware guarding.
func (s *Stage) WithPendingJobChecker(checker PendingJobChecker) *Stage {
	s.pendingJobChecker = checker
	return s
}

// resolveSourceIDs returns the source IDs to check. If explicit sourceIDs are set
// (via WithSourceIDs), those are used. Otherwise, source IDs are extracted from the
// pipeline state (proxy's linked Sources + EpgSources), making this proxy-scoped
// automatically. Falls back to global check if neither is available.
func (s *Stage) resolveSourceIDs(state *core.State) []models.ULID {
	if len(s.sourceIDs) > 0 {
		return s.sourceIDs
	}

	// Extract from pipeline state (set by orchestrator before execution)
	var ids []models.ULID
	for _, src := range state.Sources {
		ids = append(ids, src.ID)
	}
	for _, epg := range state.EpgSources {
		ids = append(ids, epg.ID)
	}
	return ids
}

// isIngestionActive checks both active ingestions and pending jobs.
// When sourceIDs are available, only checks those specific sources (proxy-scoped).
// Falls back to global check when no source IDs are available.
func (s *Stage) isIngestionActive(ctx context.Context, scopedIDs []models.ULID) bool {
	// Check active ingestions (in-memory StateManager)
	if len(scopedIDs) > 0 {
		if s.stateChecker.IsAnyIngestingForSources(scopedIDs) {
			return true
		}
	} else {
		if s.stateChecker.IsAnyIngesting() {
			return true
		}
	}

	// Check pending/queued ingestion jobs (database)
	if s.pendingJobChecker != nil && len(scopedIDs) > 0 {
		hasPending, err := s.pendingJobChecker.HasPendingIngestionJobs(ctx, scopedIDs)
		if err != nil {
			s.log(slog.LevelWarn, "failed to check pending ingestion jobs",
				slog.Any("error", err))
			// On error, be conservative and assume there might be pending jobs
			return true
		}
		if hasPending {
			return true
		}
	}

	return false
}

// activeIngestionDescription returns a description of what's still active for error messages.
func (s *Stage) activeIngestionDescription(ctx context.Context, scopedIDs []models.ULID) (int, []string) {
	var activeNames []string

	if len(scopedIDs) > 0 {
		activeNames = s.stateChecker.ActiveIngestionNamesForSources(scopedIDs)
	} else {
		activeStates := s.stateChecker.GetAllStates()
		for _, as := range activeStates {
			if as.Status == "ingesting" {
				activeNames = append(activeNames, as.SourceName)
			}
		}
	}

	// Also note if there are pending jobs not yet reflected in state
	if s.pendingJobChecker != nil && len(scopedIDs) > 0 {
		hasPending, _ := s.pendingJobChecker.HasPendingIngestionJobs(ctx, scopedIDs)
		if hasPending {
			activeNames = append(activeNames, "(+ pending queued jobs)")
		}
	}

	return len(activeNames), activeNames
}

// Execute waits for any active ingestions to complete.
func (s *Stage) Execute(ctx context.Context, state *core.State) (*core.StageResult, error) {
	result := shared.NewResult()

	// If disabled, skip the guard
	if !s.enabled {
		result.Message = "Ingestion guard disabled, skipping"
		s.log(slog.LevelDebug, "ingestion guard disabled", nil)
		return result, nil
	}

	// If no state checker is configured, skip
	if s.stateChecker == nil {
		result.Message = "No state checker configured, skipping"
		s.log(slog.LevelWarn, "ingestion guard has no state checker", nil)
		return result, nil
	}

	// Resolve proxy-scoped source IDs from state
	scopedIDs := s.resolveSourceIDs(state)

	// Check if any ingestion is active (includes pending job check)
	if !s.isIngestionActive(ctx, scopedIDs) {
		result.Message = "No active ingestions, proceeding"
		s.log(slog.LevelDebug, "no active ingestions",
			slog.Int("scoped_sources", len(scopedIDs)))
		return result, nil
	}

	// Log that we're waiting
	activeCount := s.stateChecker.ActiveIngestionCount()
	s.log(slog.LevelInfo, "waiting for active ingestions to complete",
		slog.Int("active_count", activeCount),
		slog.Int("scoped_sources", len(scopedIDs)))

	// Create a timeout context
	waitCtx, cancel := context.WithTimeout(ctx, s.maxWaitTime)
	defer cancel()

	startTime := time.Now()
	attempts := 0

	// Poll until no ingestions are active or timeout
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			// Check if it was the parent context or our timeout
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// Timeout waiting for ingestions
			elapsed := time.Since(startTime)
			count, activeNames := s.activeIngestionDescription(ctx, scopedIDs)

			return nil, fmt.Errorf("timeout waiting for ingestions after %v: %d still active (%v)",
				elapsed, count, activeNames)

		case <-ticker.C:
			attempts++

			if !s.isIngestionActive(ctx, scopedIDs) {
				// All ingestions complete
				elapsed := time.Since(startTime)
				result.Message = fmt.Sprintf("Waited %v for %d ingestion(s) to complete (%d checks)",
					elapsed.Round(time.Millisecond), activeCount, attempts)
				result.RecordsProcessed = activeCount

				s.log(slog.LevelInfo, "ingestions complete, proceeding",
					slog.Duration("wait_time", elapsed),
					slog.Int("attempts", attempts))

				// Add artifact with wait metadata
				artifact := core.NewArtifact(core.ArtifactTypeChannels, core.ProcessingStageRaw, StageID).
					WithMetadata("wait_time_ms", elapsed.Milliseconds()).
					WithMetadata("poll_attempts", attempts).
					WithMetadata("ingestions_waited", activeCount)
				result.Artifacts = append(result.Artifacts, artifact)

				return result, nil
			}

			// Log progress periodically
			if attempts%10 == 0 {
				currentCount := s.stateChecker.ActiveIngestionCount()
				s.log(slog.LevelDebug, "still waiting for ingestions",
					slog.Int("active_count", currentCount),
					slog.Int("attempts", attempts))
			}
		}
	}
}

// log logs a message if the logger is set.
func (s *Stage) log(level slog.Level, msg string, attrs ...any) {
	if s.logger != nil {
		s.logger.Log(context.Background(), level, msg, attrs...)
	}
}

// Ensure Stage implements core.Stage.
var _ core.Stage = (*Stage)(nil)
