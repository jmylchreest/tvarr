package core

import (
	"log/slog"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/jmylchreest/tvarr/internal/storage"
)

// StateChecker is an interface for checking ingestion state.
// Used by the ingestion guard stage to wait for active ingestions.
type StateChecker interface {
	IsAnyIngesting() bool
}

// Dependencies bundles all dependencies needed by pipeline stages.
// This reduces parameter count and makes dependency injection cleaner.
type Dependencies struct {
	ChannelRepo         repository.ChannelRepository
	EpgProgramRepo      repository.EpgProgramRepository
	FilterRepo          repository.FilterRepository
	DataMappingRuleRepo repository.DataMappingRuleRepository
	Sandbox             *storage.Sandbox
	Logger              *slog.Logger
	// StateChecker is used by the ingestion guard stage.
	// If nil, the ingestion guard is skipped.
	StateChecker StateChecker
	// BaseURL is the base URL for constructing fully qualified URLs (e.g., "http://localhost:8080").
	// Used by the logo caching stage to generate absolute URLs for cached logos.
	BaseURL string
}

// StageConstructor is a function that creates a stage given dependencies.
type StageConstructor func(deps *Dependencies) Stage

// Factory creates configured Orchestrator instances with all required stages.
type Factory struct {
	deps              *Dependencies
	stageConstructors []StageConstructor
}

// NewFactory creates a new pipeline Factory.
func NewFactory(deps *Dependencies) *Factory {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	return &Factory{
		deps:              deps,
		stageConstructors: make([]StageConstructor, 0),
	}
}

// RegisterStage adds a stage constructor to the factory.
// Stages are executed in the order they are registered.
func (f *Factory) RegisterStage(constructor StageConstructor) {
	f.stageConstructors = append(f.stageConstructors, constructor)
}

// Create creates a new Orchestrator configured for the given proxy.
// The returned orchestrator includes all registered stages.
func (f *Factory) Create(proxy *models.StreamProxy) (*Orchestrator, error) {
	// Determine output directory from OutputPath or default
	outputDir := proxy.OutputPath
	if outputDir == "" {
		outputDir = "output"
	}

	// Resolve within sandbox
	resolvedOutput, err := f.deps.Sandbox.ResolvePath(outputDir)
	if err != nil {
		return nil, err
	}

	// Build stages from constructors
	stages := make([]Stage, 0, len(f.stageConstructors))
	for _, constructor := range f.stageConstructors {
		stage := constructor(f.deps)
		stages = append(stages, stage)
	}

	return NewOrchestrator(proxy, stages, resolvedOutput, f.deps.Logger), nil
}

// OrchestratorFactory defines the interface for creating orchestrators.
type OrchestratorFactory interface {
	Create(proxy *models.StreamProxy) (*Orchestrator, error)
}

// Ensure Factory implements OrchestratorFactory.
var _ OrchestratorFactory = (*Factory)(nil)
