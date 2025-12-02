// Package generatexmltv implements the XMLTV generation pipeline stage.
package generatexmltv

import (
	"context"
	"fmt"
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
		return New()
	}
}

// Execute generates the XMLTV file.
func (s *Stage) Execute(ctx context.Context, state *core.State) (*core.StageResult, error) {
	result := shared.NewResult()

	// Create output file in temp directory
	outputPath := filepath.Join(state.TempDir, fmt.Sprintf("%s.xml", state.ProxyID))
	file, err := os.Create(outputPath)
	if err != nil {
		return result, fmt.Errorf("creating XMLTV file: %w", err)
	}
	defer file.Close()

	writer := xmltv.NewWriter(file)

	// Write header
	if err := writer.WriteHeader(); err != nil {
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
	programCount := 0
	for _, prog := range sortedPrograms {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
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
	}

	// Write footer
	if err := writer.WriteFooter(); err != nil {
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

	// Create artifact
	artifact := core.NewArtifact(core.ArtifactTypeXMLTV, core.ProcessingStageGenerated, StageID).
		WithFilePath(outputPath).
		WithRecordCount(programCount).
		WithFileSize(fileSize).
		WithMetadata("channel_count", len(channelsWritten))
	result.Artifacts = append(result.Artifacts, artifact)

	return result, nil
}

// Ensure Stage implements core.Stage.
var _ core.Stage = (*Stage)(nil)
