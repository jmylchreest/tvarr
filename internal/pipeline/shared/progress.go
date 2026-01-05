package shared

import (
	"context"
	"maps"
	"sync"

	"github.com/jmylchreest/tvarr/internal/pipeline/core"
)

// ProgressManager manages progress tracking for pipeline execution.
type ProgressManager struct {
	mu       sync.RWMutex
	stages   map[string]*StageProgress
	callback ProgressCallback
}

// StageProgress tracks progress for a single stage.
type StageProgress struct {
	StageID     string
	StageName   string
	Progress    float64
	Message     string
	Current     int
	Total       int
	CurrentItem string
}

// ProgressCallback is called when progress is updated.
type ProgressCallback func(stageID string, progress *StageProgress)

// NewProgressManager creates a new ProgressManager.
func NewProgressManager(callback ProgressCallback) *ProgressManager {
	return &ProgressManager{
		stages:   make(map[string]*StageProgress),
		callback: callback,
	}
}

// ReportProgress implements core.ProgressReporter.
func (pm *ProgressManager) ReportProgress(ctx context.Context, stageID string, progress float64, message string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	sp, ok := pm.stages[stageID]
	if !ok {
		sp = &StageProgress{StageID: stageID}
		pm.stages[stageID] = sp
	}

	sp.Progress = progress
	sp.Message = message

	if pm.callback != nil {
		pm.callback(stageID, sp)
	}
}

// ReportItemProgress implements core.ProgressReporter.
func (pm *ProgressManager) ReportItemProgress(ctx context.Context, stageID string, current, total int, item string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	sp, ok := pm.stages[stageID]
	if !ok {
		sp = &StageProgress{StageID: stageID}
		pm.stages[stageID] = sp
	}

	sp.Current = current
	sp.Total = total
	sp.CurrentItem = item
	if total > 0 {
		sp.Progress = float64(current) / float64(total)
	}

	if pm.callback != nil {
		pm.callback(stageID, sp)
	}
}

// GetProgress returns the current progress for a stage.
func (pm *ProgressManager) GetProgress(stageID string) *StageProgress {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.stages[stageID]
}

// GetAllProgress returns progress for all stages.
func (pm *ProgressManager) GetAllProgress() map[string]*StageProgress {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make(map[string]*StageProgress, len(pm.stages))
	maps.Copy(result, pm.stages)
	return result
}

// Reset clears all progress.
func (pm *ProgressManager) Reset() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.stages = make(map[string]*StageProgress)
}

// Ensure ProgressManager implements core.ProgressReporter.
var _ core.ProgressReporter = (*ProgressManager)(nil)
