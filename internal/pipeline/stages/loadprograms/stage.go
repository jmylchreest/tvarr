// Package loadprograms implements the EPG program loading pipeline stage.
package loadprograms

import (
	"context"
	"fmt"
	"log/slog"
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

	// progressReportInterval controls how often we report progress (every N programs).
	progressReportInterval = 5000
)

// Stage loads EPG programs for the channels in the pipeline.
type Stage struct {
	shared.BaseStage
	programRepo repository.EpgProgramRepository
	logger      *slog.Logger
}

// New creates a new load programs stage.
func New(programRepo repository.EpgProgramRepository) *Stage {
	return &Stage{
		BaseStage:   shared.NewBaseStage(StageID, StageName),
		programRepo: programRepo,
	}
}

// NewConstructor returns a stage constructor for use with the factory.
func NewConstructor() core.StageConstructor {
	return func(deps *core.Dependencies) core.Stage {
		s := New(deps.EpgProgramRepo)
		if deps.Logger != nil {
			s.logger = deps.Logger.With("stage", StageID)
		}
		return s
	}
}

// Execute loads EPG programs for all channels with matching TvgIDs.
func (s *Stage) Execute(ctx context.Context, state *core.State) (*core.StageResult, error) {
	result := shared.NewResult()

	if len(state.EpgSources) == 0 || len(state.ChannelMap) == 0 {
		s.log(ctx, slog.LevelInfo, "skipping program load - no EPG sources or channels",
			slog.Int("epg_source_count", len(state.EpgSources)),
			slog.Int("channel_map_size", len(state.ChannelMap)))
		result.Message = "No EPG sources or no channels with TvgIDs"
		return result, nil
	}

	// T030: Log stage start
	s.log(ctx, slog.LevelInfo, "starting program load",
		slog.Int("epg_source_count", len(state.EpgSources)),
		slog.Int("channel_count", len(state.ChannelMap)))

	// Get the set of TvgIDs we need programs for
	tvgIDs := make(map[string]bool)
	for tvgID := range state.ChannelMap {
		tvgIDs[tvgID] = true
	}

	// Get total program count across all enabled EPG sources for progress reporting
	var totalExpected int64
	enabledSources := make([]*models.EpgSource, 0, len(state.EpgSources))
	for _, source := range state.EpgSources {
		if !models.BoolVal(source.Enabled) {
			s.log(ctx, slog.LevelDebug, "skipping disabled EPG source",
				slog.String("source_id", source.ID.String()),
				slog.String("source_name", source.Name))
			continue
		}
		enabledSources = append(enabledSources, source)

		count, err := s.programRepo.CountBySourceID(ctx, source.ID)
		if err != nil {
			s.log(ctx, slog.LevelWarn, "failed to get program count for source",
				slog.String("source_id", source.ID.String()),
				slog.String("error", err.Error()))
			// Continue anyway - we just won't have accurate progress
		} else {
			totalExpected += count
		}
	}

	s.log(ctx, slog.LevelInfo, "expecting programs from enabled EPG sources",
		slog.Int("enabled_sources", len(enabledSources)),
		slog.Int64("total_expected", totalExpected))

	totalPrograms := 0
	totalScanned := 0
	now := time.Now()

	// Load programs from each EPG source
	for _, source := range enabledSources {
		sourceProgramCount := 0
		err := s.programRepo.GetBySourceID(ctx, source.ID, func(prog *models.EpgProgram) error {
			// Check for context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			totalScanned++

			// Report progress periodically (based on scanned, not matched)
			if totalExpected > 0 && totalScanned%progressReportInterval == 0 {
				s.reportProgress(ctx, state, totalScanned, int(totalExpected))
			}

			// Only include programs for channels we have
			if !tvgIDs[prog.ChannelID] {
				return nil
			}

			// Skip programs that have already ended
			if prog.Stop.Before(now) {
				return nil
			}

			state.Programs = append(state.Programs, prog)
			sourceProgramCount++
			totalPrograms++
			return nil
		})

		if err != nil {
			// T039: ERROR logging with full context
			s.log(ctx, slog.LevelError, "failed to load programs from source",
				slog.String("source_id", source.ID.String()),
				slog.String("source_name", source.Name),
				slog.String("error", err.Error()))
			state.AddError(fmt.Errorf("loading programs from source %s (%s): %w", source.ID, source.Name, err))
			continue
		}

		s.log(ctx, slog.LevelInfo, "loaded programs from EPG source",
			slog.String("source_id", source.ID.String()),
			slog.String("source_name", source.Name),
			slog.Int("program_count", sourceProgramCount))
	}

	// Final progress report
	if totalExpected > 0 {
		s.reportProgress(ctx, state, totalScanned, int(totalExpected))
	}

	result.RecordsProcessed = totalPrograms
	result.Message = fmt.Sprintf("Loaded %d programs from %d EPG sources (scanned %d)", totalPrograms, len(enabledSources), totalScanned)

	// T030: Log stage completion
	s.log(ctx, slog.LevelInfo, "program load complete",
		slog.Int("total_programs", totalPrograms),
		slog.Int("total_scanned", totalScanned))

	// Create artifact for loaded programs
	artifact := core.NewArtifact(core.ArtifactTypePrograms, core.ProcessingStageRaw, StageID).
		WithRecordCount(totalPrograms)
	result.Artifacts = append(result.Artifacts, artifact)

	return result, nil
}

// reportProgress reports the current progress to the progress reporter if available.
func (s *Stage) reportProgress(ctx context.Context, state *core.State, current, total int) {
	if state.ProgressReporter == nil {
		return
	}

	progress := float64(current) / float64(total)
	if progress > 1.0 {
		progress = 1.0
	}

	message := fmt.Sprintf("Loading programs (%d / %d)", current, total)
	state.ProgressReporter.ReportProgress(ctx, StageID, progress, message)
	state.ProgressReporter.ReportItemProgress(ctx, StageID, current, total, message)
}

// log logs a message if the logger is set.
func (s *Stage) log(ctx context.Context, level slog.Level, msg string, attrs ...any) {
	if s.logger != nil {
		s.logger.Log(ctx, level, msg, attrs...)
	}
}

// Ensure Stage implements core.Stage.
var _ core.Stage = (*Stage)(nil)
