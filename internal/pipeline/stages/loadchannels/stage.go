// Package loadchannels implements the channel loading pipeline stage.
package loadchannels

import (
	"context"
	"fmt"

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
		return New(deps.ChannelRepo)
	}
}

// Execute loads channels from all sources in the state.
func (s *Stage) Execute(ctx context.Context, state *core.State) (*core.StageResult, error) {
	result := shared.NewResult()

	if len(state.Sources) == 0 {
		return result, nil // No sources configured
	}

	channels := make([]*models.Channel, 0)
	channelMap := make(map[string]*models.Channel)

	// Load channels from each source in priority order
	for _, source := range state.Sources {
		if !source.Enabled {
			continue
		}

		err := s.channelRepo.GetBySourceID(ctx, source.ID, func(ch *models.Channel) error {
			// Check for context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			channels = append(channels, ch)

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
			return result, fmt.Errorf("loading channels from source %s (%s): %w", source.ID, source.Name, err)
		}
	}

	state.Channels = channels
	state.ChannelMap = channelMap

	result.RecordsProcessed = len(channels)
	result.Message = fmt.Sprintf("Loaded %d channels from %d sources", len(channels), len(state.Sources))

	// Create artifact for loaded channels
	artifact := core.NewArtifact(core.ArtifactTypeChannels, core.ProcessingStageRaw, StageID).
		WithRecordCount(len(channels))
	result.Artifacts = append(result.Artifacts, artifact)

	return result, nil
}

// Ensure Stage implements core.Stage.
var _ core.Stage = (*Stage)(nil)
