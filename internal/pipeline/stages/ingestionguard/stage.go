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
	DefaultMaxWaitTime = 5 * time.Minute
)

// StateChecker is an interface for checking ingestion state.
// This allows for testing without depending on the full StateManager.
type StateChecker interface {
	IsAnyIngesting() bool
	ActiveIngestionCount() int
	GetAllStates() []*ingestor.IngestionState
}

// Stage waits for active ingestions to complete before proceeding.
type Stage struct {
	shared.BaseStage
	stateChecker StateChecker
	pollInterval time.Duration
	maxWaitTime  time.Duration
	enabled      bool
	logger       *slog.Logger
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
func NewConstructor(stateChecker StateChecker) core.StageConstructor {
	return func(deps *core.Dependencies) core.Stage {
		s := New(stateChecker)
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

	// Check if any ingestion is active
	if !s.stateChecker.IsAnyIngesting() {
		result.Message = "No active ingestions, proceeding"
		s.log(slog.LevelDebug, "no active ingestions", nil)
		return result, nil
	}

	// Log that we're waiting
	activeCount := s.stateChecker.ActiveIngestionCount()
	s.log(slog.LevelInfo, "waiting for active ingestions to complete",
		slog.Int("active_count", activeCount))

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
			activeStates := s.stateChecker.GetAllStates()
			activeNames := make([]string, 0, len(activeStates))
			for _, as := range activeStates {
				if as.Status == "ingesting" {
					activeNames = append(activeNames, as.SourceName)
				}
			}

			return nil, fmt.Errorf("timeout waiting for ingestions after %v: %d still active (%v)",
				elapsed, len(activeNames), activeNames)

		case <-ticker.C:
			attempts++

			if !s.stateChecker.IsAnyIngesting() {
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
