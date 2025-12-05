package core

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
)

// activeExecutions tracks which proxies have pipelines running.
var (
	activeExecutions   = make(map[models.ULID]bool)
	activeExecutionsMu sync.Mutex
)

// Orchestrator executes a sequence of pipeline stages.
type Orchestrator struct {
	stages           []Stage
	state            *State
	logger           *slog.Logger
	outputDir        string
	progressReporter ProgressReporter
}

// NewOrchestrator creates a new Orchestrator with the given stages.
func NewOrchestrator(proxy *models.StreamProxy, stages []Stage, outputDir string, logger *slog.Logger) *Orchestrator {
	if logger == nil {
		logger = slog.Default()
	}
	return &Orchestrator{
		stages:    stages,
		state:     NewState(proxy),
		logger:    logger,
		outputDir: outputDir,
	}
}

// SetProgressReporter sets an optional progress reporter.
func (o *Orchestrator) SetProgressReporter(reporter ProgressReporter) {
	o.progressReporter = reporter
}

// SetSources sets the stream sources for the pipeline.
func (o *Orchestrator) SetSources(sources []*models.StreamSource) {
	o.state.Sources = sources
}

// SetEpgSources sets the EPG sources for the pipeline.
func (o *Orchestrator) SetEpgSources(sources []*models.EpgSource) {
	o.state.EpgSources = sources
}

// Execute runs all stages in sequence.
// Returns a Result with execution details and any errors.
func (o *Orchestrator) Execute(ctx context.Context) (*Result, error) {
	result := &Result{
		Success:      false,
		StageResults: make(map[string]*StageResult),
	}

	// Prevent duplicate executions for the same proxy
	if !o.acquireExecution() {
		return result, ErrPipelineAlreadyRunning
	}
	defer o.releaseExecution()

	// Create temporary directory for intermediate files
	tempDir, err := os.MkdirTemp("", fmt.Sprintf("tvarr-proxy-%s-*", o.state.ProxyID))
	if err != nil {
		return result, fmt.Errorf("creating temp directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			o.logger.Warn("failed to remove temp directory",
				slog.String("path", tempDir),
				slog.String("error", err.Error()),
			)
		} else {
			o.logger.Debug("removed temp directory",
				slog.String("path", tempDir),
			)
		}
	}()

	o.state.TempDir = tempDir
	o.state.OutputDir = o.outputDir
	o.state.ProgressReporter = o.progressReporter

	o.logger.InfoContext(ctx, "starting pipeline execution",
		slog.String("proxy_id", o.state.ProxyID.String()),
		slog.String("proxy_name", o.state.Proxy.Name),
		slog.Int("stage_count", len(o.stages)),
	)

	startTime := time.Now()

	// Execute each stage
	for i, stage := range o.stages {
		select {
		case <-ctx.Done():
			result.Errors = append(result.Errors, ctx.Err())
			result.Duration = time.Since(startTime)
			o.cleanupStages(ctx, o.stages[:i+1])
			return result, ctx.Err()
		default:
		}

		stageResult, err := o.executeStage(ctx, i, stage)
		result.StageResults[stage.ID()] = stageResult

		if err != nil {
			result.Errors = append(result.Errors, NewStageError(stage.ID(), stage.Name(), err))
			result.Duration = time.Since(startTime)
			o.cleanupStages(ctx, o.stages[:i+1])
			return result, err
		}

		// Force GC between stages to manage memory
		o.cleanupBetweenStages()
	}

	// Populate result
	result.Success = true
	result.ChannelCount = o.state.ChannelCount
	result.ProgramCount = o.state.ProgramCount
	result.Duration = time.Since(startTime)
	result.Errors = o.state.Errors

	// Set output paths if files were generated
	m3uPath := filepath.Join(o.state.OutputDir, fmt.Sprintf("%s.m3u", o.state.ProxyID))
	if _, err := os.Stat(m3uPath); err == nil {
		result.M3UPath = m3uPath
	}
	xmltvPath := filepath.Join(o.state.OutputDir, fmt.Sprintf("%s.xml", o.state.ProxyID))
	if _, err := os.Stat(xmltvPath); err == nil {
		result.XMLTVPath = xmltvPath
	}

	o.logger.InfoContext(ctx, "pipeline execution completed",
		slog.String("proxy_id", o.state.ProxyID.String()),
		slog.Int("channel_count", result.ChannelCount),
		slog.Int("program_count", result.ProgramCount),
		slog.Duration("duration", result.Duration),
		slog.Bool("success", result.Success),
	)

	// Cleanup all stages
	o.cleanupStages(ctx, o.stages)

	return result, nil
}

// executeStage runs a single stage and handles logging/progress.
func (o *Orchestrator) executeStage(ctx context.Context, index int, stage Stage) (*StageResult, error) {
	stageStart := time.Now()

	o.logger.InfoContext(ctx, "executing stage",
		slog.Int("stage_num", index+1),
		slog.Int("total_stages", len(o.stages)),
		slog.String("stage_id", stage.ID()),
		slog.String("stage_name", stage.Name()),
	)

	if o.progressReporter != nil {
		o.progressReporter.ReportProgress(ctx, stage.ID(), 0.0, "Starting")
	}

	stageResult, err := stage.Execute(ctx, o.state)
	if stageResult == nil {
		stageResult = &StageResult{}
	}
	stageResult.Duration = time.Since(stageStart)

	if err != nil {
		o.logger.ErrorContext(ctx, "stage failed",
			slog.String("stage_id", stage.ID()),
			slog.String("stage_name", stage.Name()),
			slog.String("error", err.Error()),
			slog.Duration("duration", stageResult.Duration),
		)
		return stageResult, err
	}

	// Register artifacts in state
	for _, artifact := range stageResult.Artifacts {
		o.state.AddArtifact(stage.ID(), artifact)
	}

	o.logger.InfoContext(ctx, "stage completed",
		slog.String("stage_id", stage.ID()),
		slog.String("stage_name", stage.Name()),
		slog.Duration("duration", stageResult.Duration),
		slog.Int("records_processed", stageResult.RecordsProcessed),
		slog.Int("artifacts_produced", len(stageResult.Artifacts)),
	)

	if o.progressReporter != nil {
		o.progressReporter.ReportProgress(ctx, stage.ID(), 1.0, "Complete")
	}

	return stageResult, nil
}

// cleanupStages calls Cleanup on all given stages.
func (o *Orchestrator) cleanupStages(ctx context.Context, stages []Stage) {
	for _, stage := range stages {
		if err := stage.Cleanup(ctx); err != nil {
			o.logger.Warn("stage cleanup failed",
				slog.String("stage_id", stage.ID()),
				slog.String("error", err.Error()),
			)
		}
	}
}

// cleanupBetweenStages performs memory cleanup between pipeline stages.
func (o *Orchestrator) cleanupBetweenStages() {
	runtime.GC()
}

// acquireExecution tries to acquire the execution lock for this proxy.
func (o *Orchestrator) acquireExecution() bool {
	activeExecutionsMu.Lock()
	defer activeExecutionsMu.Unlock()

	if activeExecutions[o.state.ProxyID] {
		return false
	}
	activeExecutions[o.state.ProxyID] = true
	return true
}

// releaseExecution releases the execution lock for this proxy.
func (o *Orchestrator) releaseExecution() {
	activeExecutionsMu.Lock()
	defer activeExecutionsMu.Unlock()
	delete(activeExecutions, o.state.ProxyID)
}

// State returns the current pipeline state (for testing).
func (o *Orchestrator) State() *State {
	return o.state
}

// Stages returns the configured stages (for testing).
func (o *Orchestrator) Stages() []Stage {
	return o.stages
}
