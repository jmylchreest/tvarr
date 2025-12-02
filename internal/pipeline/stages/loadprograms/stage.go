// Package loadprograms implements the EPG program loading pipeline stage.
package loadprograms

import (
	"context"
	"fmt"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/jmylchreest/tvarr/internal/pipeline/shared"
	"github.com/jmylchreest/tvarr/internal/repository"
)

const (
	// StageID is the unique identifier for this stage.
	StageID = "load_programs"
	// StageName is the human-readable name for this stage.
	StageName = "Load Programs"
	// DefaultEPGDays is the default number of days to load EPG data for.
	DefaultEPGDays = 7
)

// Stage loads EPG programs for the channels in the pipeline.
type Stage struct {
	shared.BaseStage
	programRepo repository.EpgProgramRepository
	epgDays     int
}

// New creates a new load programs stage.
func New(programRepo repository.EpgProgramRepository) *Stage {
	return &Stage{
		BaseStage:   shared.NewBaseStage(StageID, StageName),
		programRepo: programRepo,
		epgDays:     DefaultEPGDays,
	}
}

// NewConstructor returns a stage constructor for use with the factory.
func NewConstructor() core.StageConstructor {
	return func(deps *core.Dependencies) core.Stage {
		return New(deps.EpgProgramRepo)
	}
}

// WithEPGDays sets the number of days to load EPG data for.
func (s *Stage) WithEPGDays(days int) *Stage {
	s.epgDays = days
	return s
}

// Execute loads EPG programs for all channels with matching TvgIDs.
func (s *Stage) Execute(ctx context.Context, state *core.State) (*core.StageResult, error) {
	result := shared.NewResult()

	if len(state.EpgSources) == 0 || len(state.ChannelMap) == 0 {
		result.Message = "No EPG sources or no channels with TvgIDs"
		return result, nil
	}

	// Get the set of TvgIDs we need programs for
	tvgIDs := make(map[string]bool)
	for tvgID := range state.ChannelMap {
		tvgIDs[tvgID] = true
	}

	programs := make([]*models.EpgProgram, 0)

	// Define time range for EPG
	now := time.Now()
	endTime := now.Add(time.Duration(s.epgDays) * 24 * time.Hour)

	// Load programs from each EPG source
	for _, source := range state.EpgSources {
		if !source.Enabled {
			continue
		}

		err := s.programRepo.GetBySourceID(ctx, source.ID, func(prog *models.EpgProgram) error {
			// Check for context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			// Only include programs for channels we have
			if !tvgIDs[prog.ChannelID] {
				return nil
			}

			// Only include programs within our time window
			if prog.Stop.Before(now) || prog.Start.After(endTime) {
				return nil
			}

			programs = append(programs, prog)
			return nil
		})

		if err != nil {
			// Log error but continue with other sources
			state.AddError(fmt.Errorf("loading programs from source %s (%s): %w", source.ID, source.Name, err))
		}
	}

	state.Programs = programs

	result.RecordsProcessed = len(programs)
	result.Message = fmt.Sprintf("Loaded %d programs from %d EPG sources", len(programs), len(state.EpgSources))

	// Create artifact for loaded programs
	artifact := core.NewArtifact(core.ArtifactTypePrograms, core.ProcessingStageRaw, StageID).
		WithRecordCount(len(programs))
	result.Artifacts = append(result.Artifacts, artifact)

	return result, nil
}

// Ensure Stage implements core.Stage.
var _ core.Stage = (*Stage)(nil)
