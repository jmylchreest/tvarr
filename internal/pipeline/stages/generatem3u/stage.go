// Package generatem3u implements the M3U generation pipeline stage.
package generatem3u

import (
	"context"
	"fmt"
	"log/slog"
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
	logger *slog.Logger
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
		s := New()
		if deps != nil && deps.Logger != nil {
			s.logger = deps.Logger.With("stage", StageID)
		}
		return s
	}
}

// Execute generates the M3U file.
func (s *Stage) Execute(ctx context.Context, state *core.State) (*core.StageResult, error) {
	result := shared.NewResult()

	if len(state.Channels) == 0 {
		s.log(ctx, slog.LevelInfo, "no channels to write, skipping M3U generation")
		result.Message = "No channels to write"
		return result, nil
	}

	// T035: Log stage start
	s.log(ctx, slog.LevelInfo, "starting M3U generation",
		slog.Int("input_channels", len(state.Channels)))

	// Create output file in temp directory
	outputPath := filepath.Join(state.TempDir, fmt.Sprintf("%s.m3u", state.ProxyID))
	file, err := os.Create(outputPath)
	if err != nil {
		// T039: ERROR logging with full context
		s.log(ctx, slog.LevelError, "failed to create M3U file",
			slog.String("output_path", outputPath),
			slog.String("error", err.Error()))
		return result, fmt.Errorf("creating M3U file: %w", err)
	}
	defer file.Close()

	writer := m3u.NewWriter(file)

	// Write header
	if err := writer.WriteHeader(); err != nil {
		// T039: ERROR logging with full context
		s.log(ctx, slog.LevelError, "failed to write M3U header",
			slog.String("output_path", outputPath),
			slog.String("error", err.Error()))
		return result, fmt.Errorf("writing M3U header: %w", err)
	}

	channelCount := 0
	channelNum := state.Proxy.StartingChannelNumber
	var skippedCount int

	for _, ch := range state.Channels {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		// Skip channels with empty StreamURL (T011)
		if ch.StreamURL == "" {
			state.AddError(fmt.Errorf("channel %q skipped: empty stream URL", ch.ChannelName))
			skippedCount++
			continue
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

	// T035: Log stage completion with file size and channel count
	s.log(ctx, slog.LevelInfo, "M3U generation complete",
		slog.Int("channel_count", channelCount),
		slog.Int("skipped_count", skippedCount),
		slog.Int64("file_size_bytes", fileSize),
		slog.String("output_path", outputPath))

	// Create artifact
	artifact := core.NewArtifact(core.ArtifactTypeM3U, core.ProcessingStageGenerated, StageID).
		WithFilePath(outputPath).
		WithRecordCount(channelCount).
		WithFileSize(fileSize)
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
