// Package publish implements the file publishing pipeline stage.
package publish

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/jmylchreest/tvarr/internal/pipeline/shared"
	"github.com/jmylchreest/tvarr/internal/pipeline/stages/generatem3u"
	"github.com/jmylchreest/tvarr/internal/pipeline/stages/generatexmltv"
	"github.com/jmylchreest/tvarr/internal/storage"
)

const (
	// StageID is the unique identifier for this stage.
	StageID = "publish"
	// StageName is the human-readable name for this stage.
	StageName = "Publish"
)

// Stage atomically publishes generated files to the output directory.
type Stage struct {
	shared.BaseStage
	sandbox *storage.Sandbox
}

// New creates a new publish stage.
func New(sandbox *storage.Sandbox) *Stage {
	return &Stage{
		BaseStage: shared.NewBaseStage(StageID, StageName),
		sandbox:   sandbox,
	}
}

// NewConstructor returns a stage constructor for use with the factory.
func NewConstructor() core.StageConstructor {
	return func(deps *core.Dependencies) core.Stage {
		return New(deps.Sandbox)
	}
}

// Execute moves generated files from temp to output directory atomically.
func (s *Stage) Execute(ctx context.Context, state *core.State) (*core.StageResult, error) {
	result := shared.NewResult()

	// Ensure output directory exists
	if err := os.MkdirAll(state.OutputDir, 0755); err != nil {
		return result, fmt.Errorf("creating output directory: %w", err)
	}

	filesPublished := 0

	// Publish M3U file if generated
	if m3uPath, ok := state.GetMetadata(generatem3u.MetadataKeyTempPath); ok {
		destName := fmt.Sprintf("%s.m3u", state.ProxyID)
		if err := s.publishFile(ctx, m3uPath.(string), state.OutputDir, destName); err != nil {
			return result, fmt.Errorf("publishing M3U: %w", err)
		}
		filesPublished++

		// Create artifact for published M3U
		artifact := core.NewArtifact(core.ArtifactTypeM3U, core.ProcessingStagePublished, StageID).
			WithFilePath(filepath.Join(state.OutputDir, destName))
		result.Artifacts = append(result.Artifacts, artifact)
	}

	// Publish XMLTV file if generated
	if xmltvPath, ok := state.GetMetadata(generatexmltv.MetadataKeyTempPath); ok {
		destName := fmt.Sprintf("%s.xml", state.ProxyID)
		if err := s.publishFile(ctx, xmltvPath.(string), state.OutputDir, destName); err != nil {
			return result, fmt.Errorf("publishing XMLTV: %w", err)
		}
		filesPublished++

		// Create artifact for published XMLTV
		artifact := core.NewArtifact(core.ArtifactTypeXMLTV, core.ProcessingStagePublished, StageID).
			WithFilePath(filepath.Join(state.OutputDir, destName))
		result.Artifacts = append(result.Artifacts, artifact)
	}

	result.RecordsProcessed = filesPublished
	result.Message = fmt.Sprintf("Published %d files to %s", filesPublished, state.OutputDir)

	return result, nil
}

// publishFile atomically moves a file from temp to output directory.
// It first copies to a temp file in the destination, then renames for atomicity.
func (s *Stage) publishFile(ctx context.Context, srcPath, destDir, destName string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	destPath := filepath.Join(destDir, destName)
	tempDestPath := destPath + ".tmp"

	// Read source file
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("reading source file: %w", err)
	}

	// Write to temp destination
	if err := os.WriteFile(tempDestPath, data, 0644); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempDestPath, destPath); err != nil {
		// Clean up temp file on failure
		os.Remove(tempDestPath)
		return fmt.Errorf("renaming to final path: %w", err)
	}

	return nil
}

// Ensure Stage implements core.Stage.
var _ core.Stage = (*Stage)(nil)
