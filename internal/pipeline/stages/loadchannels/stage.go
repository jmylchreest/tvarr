// Package loadchannels implements the channel loading pipeline stage.
package loadchannels

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/jmylchreest/tvarr/internal/pipeline/shared"
	"github.com/jmylchreest/tvarr/internal/repository"
)

const (
	// StageID is the unique identifier for this stage.
	StageID = "load_channels"
	// StageName is the human-readable name for this stage.
	StageName = "Load Channels"
)

// Stage loads channels from all configured stream sources.
type Stage struct {
	shared.BaseStage
	channelRepo repository.ChannelRepository
	logger      *slog.Logger
}

// New creates a new load channels stage.
func New(channelRepo repository.ChannelRepository) *Stage {
	return &Stage{
		BaseStage:   shared.NewBaseStage(StageID, StageName),
		channelRepo: channelRepo,
	}
}

// NewConstructor returns a stage constructor for use with the factory.
func NewConstructor() core.StageConstructor {
	return func(deps *core.Dependencies) core.Stage {
		s := New(deps.ChannelRepo)
		if deps.Logger != nil {
			s.logger = deps.Logger.With("stage", StageID)
		}
		return s
	}
}

// Execute loads channels from all sources in the state.
func (s *Stage) Execute(ctx context.Context, state *core.State) (*core.StageResult, error) {
	result := shared.NewResult()

	// T017: Return clear error when no sources configured
	if len(state.Sources) == 0 {
		return result, core.ErrNoSources
	}

	// T027: Log stage start
	s.log(ctx, slog.LevelInfo, "starting channel load",
		slog.Int("source_count", len(state.Sources)))

	channelMap := make(map[string]*models.Channel)
	totalChannels := 0

	// Load channels from each source in priority order
	for _, source := range state.Sources {
		if !source.Enabled {
			s.log(ctx, slog.LevelDebug, "skipping disabled source",
				slog.String("source_id", source.ID.String()),
				slog.String("source_name", source.Name))
			continue
		}

		sourceChannelCount := 0
		err := s.channelRepo.GetBySourceID(ctx, source.ID, func(ch *models.Channel) error {
			// Check for context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			state.Channels = append(state.Channels, ch)
			sourceChannelCount++
			totalChannels++

			// Build channel map for EPG matching
			if ch.TvgID != "" {
				// Only add if not already present (priority ordering)
				if _, exists := channelMap[ch.TvgID]; !exists {
					channelMap[ch.TvgID] = ch
				}
			}

			return nil
		})

		if err != nil {
			// T039: ERROR logging with full context
			s.log(ctx, slog.LevelError, "failed to load channels from source",
				slog.String("source_id", source.ID.String()),
				slog.String("source_name", source.Name),
				slog.String("error", err.Error()))
			return result, fmt.Errorf("loading channels from source %s (%s): %w", source.ID, source.Name, err)
		}

		// T028: Log source processing
		s.log(ctx, slog.LevelInfo, "loaded channels from source",
			slog.String("source_id", source.ID.String()),
			slog.String("source_name", source.Name),
			slog.Int("channel_count", sourceChannelCount))
	}

	state.ChannelMap = channelMap

	result.RecordsProcessed = totalChannels
	result.Message = fmt.Sprintf("Loaded %d channels from %d sources", totalChannels, len(state.Sources))

	// T029: Log stage completion
	s.log(ctx, slog.LevelInfo, "channel load complete",
		slog.Int("total_channels", totalChannels),
		slog.Int("unique_tvg_ids", len(channelMap)))

	// Create artifact for loaded channels
	artifact := core.NewArtifact(core.ArtifactTypeChannels, core.ProcessingStageRaw, StageID).
		WithRecordCount(totalChannels)
	result.Artifacts = append(result.Artifacts, artifact)

	return result, nil
}

// log logs a message if the logger is set.
func (s *Stage) log(ctx context.Context, level slog.Level, msg string, attrs ...any) {
	if s.logger != nil {
		s.logger.Log(ctx, level, msg, attrs...)
	}
}

// Ensure Stage implements core.Stage.
var _ core.Stage = (*Stage)(nil)
