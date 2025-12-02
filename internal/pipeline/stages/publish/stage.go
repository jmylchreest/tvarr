// Package publish implements the file publishing pipeline stage.
package publish

import (
	"context"
	"fmt"
	"io"
	"log/slog"
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
	logger  *slog.Logger
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
		s := New(deps.Sandbox)
		if deps.Logger != nil {
			s.logger = deps.Logger.With("stage", StageID)
		}
		return s
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
// It uses os.Rename() for atomic publishing on the same filesystem.
// If source and destination are on different filesystems, it falls back
// to copy-then-rename for atomicity.
func (s *Stage) publishFile(ctx context.Context, srcPath, destDir, destName string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	destPath := filepath.Join(destDir, destName)

	// Try direct rename first (atomic if same filesystem)
	if err := os.Rename(srcPath, destPath); err == nil {
		s.log(slog.LevelDebug, "published file via direct rename",
			slog.String("src", srcPath),
			slog.String("dest", destPath))
		return nil
	}

	// Fall back to copy-then-rename for cross-filesystem scenarios
	// This ensures atomicity even when temp and output are on different filesystems
	s.log(slog.LevelDebug, "falling back to copy-then-rename",
		slog.String("src", srcPath),
		slog.String("dest", destPath))

	return s.copyThenRename(ctx, srcPath, destPath)
}

// copyThenRename copies a file to a temp location in the destination directory,
// then renames it to the final path. This ensures atomic publishing even when
// the source and destination are on different filesystems.
func (s *Stage) copyThenRename(ctx context.Context, srcPath, destPath string) error {
	tempDestPath := destPath + ".tmp"

	// Open source file
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("opening source file: %w", err)
	}
	defer srcFile.Close()

	// Create temp destination file
	tempFile, err := os.Create(tempDestPath)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	// Copy with context cancellation check
	copyErr := func() error {
		defer tempFile.Close()

		// Copy in chunks to allow for cancellation checks
		buf := make([]byte, 32*1024) // 32KB buffer
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			n, err := srcFile.Read(buf)
			if n > 0 {
				if _, writeErr := tempFile.Write(buf[:n]); writeErr != nil {
					return fmt.Errorf("writing to temp file: %w", writeErr)
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("reading source file: %w", err)
			}
		}
		return nil
	}()

	if copyErr != nil {
		// Clean up temp file on failure
		os.Remove(tempDestPath)
		return copyErr
	}

	// Atomic rename (temp and dest are now on same filesystem)
	if err := os.Rename(tempDestPath, destPath); err != nil {
		// Clean up temp file on failure
		os.Remove(tempDestPath)
		return fmt.Errorf("renaming to final path: %w", err)
	}

	return nil
}

// log logs a message if the logger is set.
func (s *Stage) log(level slog.Level, msg string, attrs ...any) {
	if s.logger != nil {
		s.logger.Log(context.Background(), level, msg, attrs...)
	}
}

// Ensure Stage implements core.Stage.
var _ core.Stage = (*Stage)(nil)
