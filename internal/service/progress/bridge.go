package progress

import (
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/pipeline/core"
)

// Ensure OperationManager implements core.ProgressReporter at compile time.
var _ core.ProgressReporter = (*OperationManager)(nil)

// CreateStagesFromPipeline creates StageInfo entries from pipeline stages.
// This is a helper to initialize the progress operation with the correct stages.
func CreateStagesFromPipeline(stages []core.Stage) []StageInfo {
	infos := make([]StageInfo, len(stages))
	weight := 1.0 / float64(len(stages))
	for i, stage := range stages {
		infos[i] = StageInfo{
			ID:     stage.ID(),
			Name:   stage.Name(),
			Weight: weight,
		}
	}
	return infos
}

// StartPipelineOperation is a convenience function that starts a progress operation
// for a pipeline execution and returns the OperationManager that implements core.ProgressReporter.
// It handles operation creation and stage setup in one call.
func StartPipelineOperation(
	svc *Service,
	ownerType string,
	ownerID models.ULID,
	ownerName string,
	stages []core.Stage,
) (*OperationManager, error) {
	stageInfos := CreateStagesFromPipeline(stages)

	// Determine operation type based on owner type
	var opType OperationType
	switch ownerType {
	case "stream_proxy":
		opType = OpProxyRegeneration
	case "stream_source":
		opType = OpStreamIngestion
	case "epg_source":
		opType = OpEpgIngestion
	default:
		opType = OpPipeline
	}

	return svc.StartOperation(opType, ownerID, ownerType, ownerName, stageInfos)
}
