// Package pipeline provides a composable pipeline architecture for proxy generation.
// Each stage implements the Stage interface and operates on shared State.
//
// The pipeline is organized into several sub-packages:
//   - core: Orchestrator, interfaces, and base types
//   - shared: Utilities shared between stages
//   - stages/*: Individual stage implementations
package pipeline

import (
	"log/slog"

	"github.com/jmylchreest/tvarr/internal/ingestor"
	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/jmylchreest/tvarr/internal/pipeline/stages/datamapping"
	"github.com/jmylchreest/tvarr/internal/pipeline/stages/filtering"
	"github.com/jmylchreest/tvarr/internal/pipeline/stages/generatem3u"
	"github.com/jmylchreest/tvarr/internal/pipeline/stages/generatexmltv"
	"github.com/jmylchreest/tvarr/internal/pipeline/stages/ingestionguard"
	"github.com/jmylchreest/tvarr/internal/pipeline/stages/loadchannels"
	"github.com/jmylchreest/tvarr/internal/pipeline/stages/loadprograms"
	"github.com/jmylchreest/tvarr/internal/pipeline/stages/logocaching"
	"github.com/jmylchreest/tvarr/internal/pipeline/stages/numbering"
	"github.com/jmylchreest/tvarr/internal/pipeline/stages/publish"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/jmylchreest/tvarr/internal/storage"
)

// Re-export core types for convenience.
type (
	// Stage is a single step in the pipeline.
	Stage = core.Stage

	// State holds shared data between stages.
	State = core.State

	// Result is the outcome of pipeline execution.
	Result = core.Result

	// StageResult is the outcome of a single stage.
	StageResult = core.StageResult

	// Orchestrator executes stages in sequence.
	Orchestrator = core.Orchestrator

	// OrchestratorFactory creates orchestrators.
	OrchestratorFactory = core.OrchestratorFactory

	// Factory creates orchestrators.
	Factory = core.Factory

	// Dependencies bundles stage dependencies.
	Dependencies = core.Dependencies

	// Config holds pipeline configuration.
	Config = core.Config

	// Builder provides fluent factory construction.
	Builder = core.Builder

	// Artifact represents stage output.
	Artifact = core.Artifact

	// ArtifactType identifies artifact content.
	ArtifactType = core.ArtifactType

	// ProcessingStage indicates processing state.
	ProcessingStage = core.ProcessingStage

	// ProgressReporter allows progress tracking.
	ProgressReporter = core.ProgressReporter

	// StageConstructor creates stages from dependencies.
	StageConstructor = core.StageConstructor
)

// Re-export artifact types.
const (
	ArtifactTypeChannels = core.ArtifactTypeChannels
	ArtifactTypePrograms = core.ArtifactTypePrograms
	ArtifactTypeM3U      = core.ArtifactTypeM3U
	ArtifactTypeXMLTV    = core.ArtifactTypeXMLTV
)

// Re-export processing stages.
const (
	ProcessingStageRaw       = core.ProcessingStageRaw
	ProcessingStageFiltered  = core.ProcessingStageFiltered
	ProcessingStageNumbered  = core.ProcessingStageNumbered
	ProcessingStageGenerated = core.ProcessingStageGenerated
	ProcessingStagePublished = core.ProcessingStagePublished
)

// Re-export errors.
var (
	ErrNoSources              = core.ErrNoSources
	ErrNoChannels             = core.ErrNoChannels
	ErrPipelineAlreadyRunning = core.ErrPipelineAlreadyRunning
	ErrStageNotFound          = core.ErrStageNotFound
	ErrInvalidConfiguration   = core.ErrInvalidConfiguration
)

// NewBuilder creates a new pipeline builder.
func NewBuilder() *Builder {
	return core.NewBuilder()
}

// NewState creates a new pipeline state.
var NewState = core.NewState

// NewFactory creates a new pipeline factory with the given dependencies.
func NewFactory(deps *Dependencies) *Factory {
	return core.NewFactory(deps)
}

// NewDefaultFactory creates a factory with the standard stage configuration.
// If stateManager is nil, ingestion guard stage is skipped.
// If logoCacher is nil, logo caching stage is skipped.
// baseURL is used to construct fully qualified URLs for cached logos (e.g., "http://localhost:8080").
func NewDefaultFactory(
	channelRepo repository.ChannelRepository,
	epgProgramRepo repository.EpgProgramRepository,
	filterRepo repository.FilterRepository,
	dataMappingRuleRepo repository.DataMappingRuleRepository,
	sandbox *storage.Sandbox,
	logger *slog.Logger,
	logoCacher logocaching.LogoCacher,
	stateManager *ingestor.StateManager,
	baseURL string,
) *Factory {
	deps := &Dependencies{
		ChannelRepo:         channelRepo,
		EpgProgramRepo:      epgProgramRepo,
		FilterRepo:          filterRepo,
		DataMappingRuleRepo: dataMappingRuleRepo,
		Sandbox:             sandbox,
		Logger:              logger,
		BaseURL:             baseURL,
	}

	factory := NewFactory(deps)

	// Register default stages in execution order
	// Ingestion guard is FIRST to ensure data consistency
	if stateManager != nil {
		factory.RegisterStage(ingestionguard.NewConstructor(stateManager))
	}

	factory.RegisterStage(loadchannels.NewConstructor())
	factory.RegisterStage(loadprograms.NewConstructor())
	factory.RegisterStage(datamapping.NewConstructor())
	factory.RegisterStage(filtering.NewConstructor())
	factory.RegisterStage(numbering.NewConstructor())

	// Logo caching (optional - only if cacher provided)
	if logoCacher != nil {
		factory.RegisterStage(logocaching.NewConstructor(logoCacher))
	}

	factory.RegisterStage(generatem3u.NewConstructor())
	factory.RegisterStage(generatexmltv.NewConstructor())
	factory.RegisterStage(publish.NewConstructor())

	return factory
}

// Stage IDs for reference.
const (
	StageIDIngestionGuard = ingestionguard.StageID
	StageIDLoadChannels   = loadchannels.StageID
	StageIDLoadPrograms   = loadprograms.StageID
	StageIDFiltering      = filtering.StageID
	StageIDDataMapping    = datamapping.StageID
	StageIDNumbering      = numbering.StageID
	StageIDLogoCaching    = logocaching.StageID
	StageIDGenerateM3U    = generatem3u.StageID
	StageIDGenerateXMLTV  = generatexmltv.StageID
	StageIDPublish        = publish.StageID
)
