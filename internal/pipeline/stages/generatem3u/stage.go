// Package generatem3u implements the M3U generation pipeline stage.
package generatem3u

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/jmylchreest/tvarr/internal/pipeline/shared"
	"github.com/jmylchreest/tvarr/pkg/m3u"
)

const (
	// StageID is the unique identifier for this stage.
	StageID = "generate_m3u"
	// StageName is the human-readable name for this stage.
	StageName = "Generate M3U"
	// MetadataKeyTempPath is the metadata key for the temp file path.
	MetadataKeyTempPath = "m3u_temp_path"
)

// Stage generates an M3U playlist from the pipeline channels.
type Stage struct {
	shared.BaseStage
}

// New creates a new M3U generation stage.
func New() *Stage {
	return &Stage{
		BaseStage: shared.NewBaseStage(StageID, StageName),
	}
}

// NewConstructor returns a stage constructor for use with the factory.
func NewConstructor() core.StageConstructor {
	return func(deps *core.Dependencies) core.Stage {
		return New()
	}
}

// Execute generates the M3U file.
func (s *Stage) Execute(ctx context.Context, state *core.State) (*core.StageResult, error) {
	result := shared.NewResult()

	if len(state.Channels) == 0 {
		result.Message = "No channels to write"
		return result, nil
	}

	// Create output file in temp directory
	outputPath := filepath.Join(state.TempDir, fmt.Sprintf("%s.m3u", state.ProxyID))
	file, err := os.Create(outputPath)
	if err != nil {
		return result, fmt.Errorf("creating M3U file: %w", err)
	}
	defer file.Close()

	writer := m3u.NewWriter(file)

	// Write header
	if err := writer.WriteHeader(); err != nil {
		return result, fmt.Errorf("writing M3U header: %w", err)
	}

	channelCount := 0
	channelNum := state.Proxy.StartingChannelNumber

	for _, ch := range state.Channels {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		entry := shared.ChannelToM3UEntry(ch, channelNum)

		if err := writer.WriteEntry(entry); err != nil {
			state.AddError(fmt.Errorf("writing channel %s: %w", ch.ChannelName, err))
			continue
		}

		channelCount++
		channelNum++
	}

	state.ChannelCount = channelCount
	state.SetMetadata(MetadataKeyTempPath, outputPath)

	// Get file size
	fileInfo, _ := file.Stat()
	var fileSize int64
	if fileInfo != nil {
		fileSize = fileInfo.Size()
	}

	result.RecordsProcessed = channelCount
	result.Message = fmt.Sprintf("Generated M3U with %d channels", channelCount)

	// Create artifact
	artifact := core.NewArtifact(core.ArtifactTypeM3U, core.ProcessingStageGenerated, StageID).
		WithFilePath(outputPath).
		WithRecordCount(channelCount).
		WithFileSize(fileSize)
	result.Artifacts = append(result.Artifacts, artifact)

	return result, nil
}

// Ensure Stage implements core.Stage.
var _ core.Stage = (*Stage)(nil)
