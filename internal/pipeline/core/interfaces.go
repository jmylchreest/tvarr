// Package core provides the pipeline orchestration framework.
package core

import (
	"context"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
)

// Stage represents a single step in the proxy generation pipeline.
// Each stage receives artifacts from previous stages and produces new artifacts.
type Stage interface {
	// ID returns a unique identifier for the stage (e.g., "load_channels").
	ID() string

	// Name returns a human-readable name for the stage (e.g., "Load Channels").
	Name() string

	// Execute performs the stage's work.
	// It receives input artifacts and returns output artifacts.
	Execute(ctx context.Context, state *State) (*StageResult, error)

	// Cleanup performs any necessary cleanup after execution.
	// Called regardless of success or failure.
	Cleanup(ctx context.Context) error
}

// ProgressReporter allows stages to report execution progress.
type ProgressReporter interface {
	// ReportProgress reports stage progress (0.0 to 1.0).
	ReportProgress(ctx context.Context, stageID string, progress float64, message string)

	// ReportItemProgress reports progress on individual items.
	ReportItemProgress(ctx context.Context, stageID string, current, total int, item string)
}

// State holds all data shared between pipeline stages.
type State struct {
	// ProxyID is the ID of the StreamProxy being generated.
	ProxyID models.ULID

	// Proxy is the full proxy configuration.
	Proxy *models.StreamProxy

	// Sources are the stream sources to include, ordered by priority.
	Sources []*models.StreamSource

	// EpgSources are the EPG sources to include, ordered by priority.
	EpgSources []*models.EpgSource

	// ProgressReporter allows stages to report their progress.
	ProgressReporter ProgressReporter

	// TempDir is the temporary directory for intermediate files.
	TempDir string

	// OutputDir is the final output directory for generated files.
	OutputDir string

	// Channels holds channels loaded from sources for processing.
	Channels []*models.Channel

	// Programs holds EPG programs for the included channels.
	Programs []*models.EpgProgram

	// ChannelMap maps channel TvgID to channel for EPG matching.
	ChannelMap map[string]*models.Channel

	// ChannelCount tracks the number of channels in output.
	ChannelCount int

	// ProgramCount tracks the number of EPG programs in output.
	ProgramCount int

	// StartTime records when pipeline execution began.
	StartTime time.Time

	// Errors collects non-fatal errors during execution.
	Errors []error

	// Artifacts holds output artifacts from each stage.
	Artifacts map[string][]Artifact

	// Metadata stores arbitrary stage-specific data.
	Metadata map[string]any
}

// NewState creates a new pipeline state for the given proxy.
func NewState(proxy *models.StreamProxy) *State {
	return &State{
		ProxyID:    proxy.ID,
		Proxy:      proxy,
		Channels:   make([]*models.Channel, 0),
		Programs:   make([]*models.EpgProgram, 0),
		ChannelMap: make(map[string]*models.Channel),
		StartTime:  time.Now(),
		Errors:     make([]error, 0),
		Artifacts:  make(map[string][]Artifact),
		Metadata:   make(map[string]any),
	}
}

// AddError adds a non-fatal error to the state.
func (s *State) AddError(err error) {
	if err != nil {
		s.Errors = append(s.Errors, err)
	}
}

// HasErrors returns true if any non-fatal errors were recorded.
func (s *State) HasErrors() bool {
	return len(s.Errors) > 0
}

// Duration returns the elapsed time since pipeline start.
func (s *State) Duration() time.Duration {
	return time.Since(s.StartTime)
}

// SetMetadata stores a value in the metadata map.
func (s *State) SetMetadata(key string, value any) {
	s.Metadata[key] = value
}

// GetMetadata retrieves a value from the metadata map.
func (s *State) GetMetadata(key string) (any, bool) {
	v, ok := s.Metadata[key]
	return v, ok
}

// AddArtifact adds an artifact produced by a stage.
func (s *State) AddArtifact(stageID string, artifact Artifact) {
	s.Artifacts[stageID] = append(s.Artifacts[stageID], artifact)
}

// GetArtifacts returns all artifacts produced by a stage.
func (s *State) GetArtifacts(stageID string) []Artifact {
	return s.Artifacts[stageID]
}

// GetArtifactsByType returns all artifacts of a specific type.
func (s *State) GetArtifactsByType(artifactType ArtifactType) []Artifact {
	var result []Artifact
	for _, artifacts := range s.Artifacts {
		for _, a := range artifacts {
			if a.Type == artifactType {
				result = append(result, a)
			}
		}
	}
	return result
}

// StageResult contains the outcome of a stage execution.
type StageResult struct {
	// Artifacts produced by this stage.
	Artifacts []Artifact

	// RecordsProcessed is the count of items processed.
	RecordsProcessed int

	// RecordsModified is the count of items changed.
	RecordsModified int

	// Duration is the execution time.
	Duration time.Duration

	// Message is an optional summary message.
	Message string
}

// Result represents the outcome of pipeline execution.
type Result struct {
	// Success indicates if the pipeline completed without fatal errors.
	Success bool

	// ChannelCount is the number of channels in the generated output.
	ChannelCount int

	// ProgramCount is the number of EPG programs in the generated output.
	ProgramCount int

	// Duration is the total execution time.
	Duration time.Duration

	// StageResults contains results from each stage.
	StageResults map[string]*StageResult

	// Errors contains any errors that occurred.
	Errors []error

	// M3UPath is the path to the generated M3U file.
	M3UPath string

	// XMLTVPath is the path to the generated XMLTV file.
	XMLTVPath string
}
