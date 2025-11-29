package core

import (
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
)

// ArtifactType identifies the type of content in an artifact.
type ArtifactType string

const (
	// ArtifactTypeChannels represents channel data.
	ArtifactTypeChannels ArtifactType = "channels"

	// ArtifactTypePrograms represents EPG program data.
	ArtifactTypePrograms ArtifactType = "programs"

	// ArtifactTypeM3U represents a generated M3U file.
	ArtifactTypeM3U ArtifactType = "m3u"

	// ArtifactTypeXMLTV represents a generated XMLTV file.
	ArtifactTypeXMLTV ArtifactType = "xmltv"
)

// ProcessingStage indicates the processing state of an artifact.
type ProcessingStage string

const (
	// ProcessingStageRaw indicates unprocessed data.
	ProcessingStageRaw ProcessingStage = "raw"

	// ProcessingStageFiltered indicates data after filtering.
	ProcessingStageFiltered ProcessingStage = "filtered"

	// ProcessingStageTransformed indicates data after data mapping/transformation.
	ProcessingStageTransformed ProcessingStage = "transformed"

	// ProcessingStageNumbered indicates data after channel numbering.
	ProcessingStageNumbered ProcessingStage = "numbered"

	// ProcessingStageGenerated indicates generated output files.
	ProcessingStageGenerated ProcessingStage = "generated"

	// ProcessingStagePublished indicates files moved to final location.
	ProcessingStagePublished ProcessingStage = "published"
)

// Artifact represents an output from a pipeline stage.
type Artifact struct {
	// ID is a unique identifier for this artifact.
	ID models.ULID

	// Type identifies the content type.
	Type ArtifactType

	// Stage indicates the processing stage.
	Stage ProcessingStage

	// FilePath is the path to the artifact file (if file-based).
	FilePath string

	// CreatedBy is the stage ID that created this artifact.
	CreatedBy string

	// RecordCount is the number of records in the artifact.
	RecordCount int

	// FileSize is the size in bytes (if file-based).
	FileSize int64

	// CreatedAt is when the artifact was created.
	CreatedAt time.Time

	// Metadata contains additional artifact-specific data.
	Metadata map[string]any
}

// NewArtifact creates a new artifact with the given type and stage.
func NewArtifact(artifactType ArtifactType, stage ProcessingStage, createdBy string) Artifact {
	return Artifact{
		ID:        models.NewULID(),
		Type:      artifactType,
		Stage:     stage,
		CreatedBy: createdBy,
		CreatedAt: time.Now(),
		Metadata:  make(map[string]any),
	}
}

// WithFilePath sets the file path for the artifact.
func (a Artifact) WithFilePath(path string) Artifact {
	a.FilePath = path
	return a
}

// WithRecordCount sets the record count for the artifact.
func (a Artifact) WithRecordCount(count int) Artifact {
	a.RecordCount = count
	return a
}

// WithFileSize sets the file size for the artifact.
func (a Artifact) WithFileSize(size int64) Artifact {
	a.FileSize = size
	return a
}

// WithMetadata adds metadata to the artifact.
func (a Artifact) WithMetadata(key string, value any) Artifact {
	a.Metadata[key] = value
	return a
}
