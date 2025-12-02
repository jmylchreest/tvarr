// Package generatexmltv implements the XMLTV generation pipeline stage.
package generatexmltv

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/jmylchreest/tvarr/internal/pipeline/shared"
	"github.com/jmylchreest/tvarr/pkg/xmltv"
)

const (
	// StageID is the unique identifier for this stage.
	StageID = "generate_xmltv"
	// StageName is the human-readable name for this stage.
	StageName = "Generate XMLTV"
	// MetadataKeyTempPath is the metadata key for the temp file path.
	MetadataKeyTempPath = "xmltv_temp_path"
)

// Stage generates an XMLTV file from the pipeline programs.
type Stage struct {
	shared.BaseStage
	logger *slog.Logger
}

// New creates a new XMLTV generation stage.
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

// Execute generates the XMLTV file.
func (s *Stage) Execute(ctx context.Context, state *core.State) (*core.StageResult, error) {
	result := shared.NewResult()

	// T036: Log stage start
	s.log(ctx, slog.LevelInfo, "starting XMLTV generation",
		slog.Int("input_channels", len(state.Channels)),
		slog.Int("input_programs", len(state.Programs)))

	// Create output file in temp directory
	outputPath := filepath.Join(state.TempDir, fmt.Sprintf("%s.xml", state.ProxyID))
	file, err := os.Create(outputPath)
	if err != nil {
		// T039: ERROR logging with full context
		s.log(ctx, slog.LevelError, "failed to create XMLTV file",
			slog.String("output_path", outputPath),
			slog.String("error", err.Error()))
		return result, fmt.Errorf("creating XMLTV file: %w", err)
	}
	defer file.Close()

	writer := xmltv.NewWriter(file)

	// Write header
	if err := writer.WriteHeader(); err != nil {
		// T039: ERROR logging with full context
		s.log(ctx, slog.LevelError, "failed to write XMLTV header",
			slog.String("output_path", outputPath),
			slog.String("error", err.Error()))
		return result, fmt.Errorf("writing XMLTV header: %w", err)
	}

	// Write channel definitions for channels that have TvgID
	channelsWritten := make(map[string]bool)
	for _, ch := range state.Channels {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		if ch.TvgID == "" {
			continue
		}

		// Only write each channel once
		if channelsWritten[ch.TvgID] {
			continue
		}

		xmlCh := shared.ChannelToXMLTVChannel(ch)
		if err := writer.WriteChannel(xmlCh); err != nil {
			state.AddError(fmt.Errorf("writing channel %s: %w", ch.TvgID, err))
			continue
		}

		channelsWritten[ch.TvgID] = true
	}

	// Sort programs by channel and start time for consistent output
	sortedPrograms := make([]*models.EpgProgram, len(state.Programs))
	copy(sortedPrograms, state.Programs)
	sort.Slice(sortedPrograms, func(i, j int) bool {
		if sortedPrograms[i].ChannelID != sortedPrograms[j].ChannelID {
			return sortedPrograms[i].ChannelID < sortedPrograms[j].ChannelID
		}
		return sortedPrograms[i].Start.Before(sortedPrograms[j].Start)
	})

	// Write programmes
	// T038: DEBUG logging for batch progress
	const batchSize = 1000
	totalPrograms := len(sortedPrograms)
	programCount := 0
	for i, prog := range sortedPrograms {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		// Skip programs with missing required fields (T013)
		if prog.Title == "" {
			state.AddError(fmt.Errorf("program skipped: empty title for channel %q", prog.ChannelID))
			continue
		}

		// Only include programs for channels we wrote
		if !channelsWritten[prog.ChannelID] {
			continue
		}

		xmlProg := shared.ProgramToXMLTVProgramme(prog)
		if err := writer.WriteProgramme(xmlProg); err != nil {
			state.AddError(fmt.Errorf("writing program %s: %w", prog.Title, err))
			continue
		}

		programCount++

		// T038: DEBUG logging for batch progress
		if (i+1)%batchSize == 0 {
			batchNum := (i + 1) / batchSize
			totalBatches := (totalPrograms + batchSize - 1) / batchSize
			s.log(ctx, slog.LevelDebug, "XMLTV generation batch progress",
				slog.Int("batch_num", batchNum),
				slog.Int("total_batches", totalBatches),
				slog.Int("items_processed", i+1),
				slog.Int("total_items", totalPrograms))
		}
	}

	// Write footer
	if err := writer.WriteFooter(); err != nil {
		// T039: ERROR logging with full context
		s.log(ctx, slog.LevelError, "failed to write XMLTV footer",
			slog.String("output_path", outputPath),
			slog.String("error", err.Error()))
		return result, fmt.Errorf("writing XMLTV footer: %w", err)
	}

	state.ProgramCount = programCount
	state.SetMetadata(MetadataKeyTempPath, outputPath)

	// Get file size
	fileInfo, _ := file.Stat()
	var fileSize int64
	if fileInfo != nil {
		fileSize = fileInfo.Size()
	}

	result.RecordsProcessed = programCount
	result.Message = fmt.Sprintf("Generated XMLTV with %d channels and %d programs", len(channelsWritten), programCount)

	// T036: Log stage completion with program count and file size
	s.log(ctx, slog.LevelInfo, "XMLTV generation complete",
		slog.Int("channel_count", len(channelsWritten)),
		slog.Int("program_count", programCount),
		slog.Int64("file_size_bytes", fileSize),
		slog.String("output_path", outputPath))

	// Create artifact
	artifact := core.NewArtifact(core.ArtifactTypeXMLTV, core.ProcessingStageGenerated, StageID).
		WithFilePath(outputPath).
		WithRecordCount(programCount).
		WithFileSize(fileSize).
		WithMetadata("channel_count", len(channelsWritten))
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
