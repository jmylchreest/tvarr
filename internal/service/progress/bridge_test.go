package progress_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/jmylchreest/tvarr/internal/service/progress"
)

// mockStage implements core.Stage for testing
type mockStage struct {
	id   string
	name string
}

func (s *mockStage) ID() string   { return s.id }
func (s *mockStage) Name() string { return s.name }
func (s *mockStage) Execute(ctx context.Context, state *core.State) (*core.StageResult, error) {
	return &core.StageResult{}, nil
}
func (s *mockStage) Cleanup(ctx context.Context) error { return nil }

func newTestProgressService() *progress.Service {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return progress.NewService(logger)
}

func TestOperationManager_ReportProgress(t *testing.T) {
	t.Run("updates stage progress", func(t *testing.T) {
		svc := newTestProgressService()
		ownerID := models.NewULID()

		stages := []core.Stage{
			&mockStage{id: "load", name: "Load"},
			&mockStage{id: "process", name: "Process"},
			&mockStage{id: "save", name: "Save"},
		}

		stageInfos := progress.CreateStagesFromPipeline(stages)
		mgr, err := svc.StartOperation(progress.OpPipeline, ownerID, "test", stageInfos)
		require.NoError(t, err)

		// Use OperationManager directly as ProgressReporter
		var reporter core.ProgressReporter = mgr

		// Report progress on the "load" stage
		reporter.ReportProgress(context.Background(), "load", 0.5, "Halfway")

		op, err := svc.GetOperation(mgr.OperationID())
		require.NoError(t, err)
		loadStage := op.Stages[0]
		assert.Equal(t, 0.5, loadStage.Progress)
		assert.Equal(t, "Halfway", loadStage.Message)
	})

	t.Run("handles unknown stage IDs gracefully", func(t *testing.T) {
		svc := newTestProgressService()
		ownerID := models.NewULID()

		stages := []core.Stage{
			&mockStage{id: "load", name: "Load"},
		}

		stageInfos := progress.CreateStagesFromPipeline(stages)
		mgr, err := svc.StartOperation(progress.OpPipeline, ownerID, "test", stageInfos)
		require.NoError(t, err)

		// Report progress on an unknown stage - should not panic
		mgr.ReportProgress(context.Background(), "unknown", 0.5, "Test")

		// Operation should still be accessible
		op, err := svc.GetOperation(mgr.OperationID())
		require.NoError(t, err)
		assert.NotNil(t, op)
	})
}

func TestOperationManager_ReportItemProgress(t *testing.T) {
	t.Run("calculates progress from item counts", func(t *testing.T) {
		svc := newTestProgressService()
		ownerID := models.NewULID()

		stages := []core.Stage{
			&mockStage{id: "load", name: "Load Channels"},
		}

		stageInfos := progress.CreateStagesFromPipeline(stages)
		mgr, err := svc.StartOperation(progress.OpPipeline, ownerID, "test", stageInfos)
		require.NoError(t, err)

		// Report item progress: 25 of 100
		mgr.ReportItemProgress(context.Background(), "load", 25, 100, "channel")

		op, err := svc.GetOperation(mgr.OperationID())
		require.NoError(t, err)
		loadStage := op.Stages[0]
		assert.InDelta(t, 0.25, loadStage.Progress, 0.01)
		assert.Equal(t, 25, loadStage.Current)
		assert.Equal(t, 100, loadStage.Total)
	})

	t.Run("handles zero total gracefully", func(t *testing.T) {
		svc := newTestProgressService()
		ownerID := models.NewULID()

		stages := []core.Stage{
			&mockStage{id: "load", name: "Load"},
		}

		stageInfos := progress.CreateStagesFromPipeline(stages)
		mgr, err := svc.StartOperation(progress.OpPipeline, ownerID, "test", stageInfos)
		require.NoError(t, err)

		// Should not panic with zero total
		mgr.ReportItemProgress(context.Background(), "load", 0, 0, "item")

		// Operation should still be accessible and progress should be 0
		op, err := svc.GetOperation(mgr.OperationID())
		require.NoError(t, err)
		assert.NotNil(t, op)
	})
}

func TestCreateStagesFromPipeline(t *testing.T) {
	t.Run("creates stage infos with equal weights", func(t *testing.T) {
		stages := []core.Stage{
			&mockStage{id: "a", name: "Stage A"},
			&mockStage{id: "b", name: "Stage B"},
			&mockStage{id: "c", name: "Stage C"},
			&mockStage{id: "d", name: "Stage D"},
		}

		infos := progress.CreateStagesFromPipeline(stages)

		assert.Len(t, infos, 4)
		for i, info := range infos {
			assert.Equal(t, stages[i].ID(), info.ID)
			assert.Equal(t, stages[i].Name(), info.Name)
			assert.InDelta(t, 0.25, info.Weight, 0.001)
		}
	})
}

func TestStartPipelineOperation(t *testing.T) {
	t.Run("creates operation with correct type for stream_proxy", func(t *testing.T) {
		svc := newTestProgressService()
		ownerID := models.NewULID()

		stages := []core.Stage{
			&mockStage{id: "load", name: "Load"},
		}

		mgr, err := progress.StartPipelineOperation(svc, "stream_proxy", ownerID, stages)
		require.NoError(t, err)
		require.NotNil(t, mgr)

		op, err := svc.GetOperation(mgr.OperationID())
		require.NoError(t, err)
		assert.Equal(t, progress.OpProxyRegeneration, op.OperationType)
	})

	t.Run("creates operation with correct type for stream_source", func(t *testing.T) {
		svc := newTestProgressService()
		ownerID := models.NewULID()

		stages := []core.Stage{
			&mockStage{id: "ingest", name: "Ingest"},
		}

		mgr, err := progress.StartPipelineOperation(svc, "stream_source", ownerID, stages)
		require.NoError(t, err)
		require.NotNil(t, mgr)

		op, err := svc.GetOperation(mgr.OperationID())
		require.NoError(t, err)
		assert.Equal(t, progress.OpStreamIngestion, op.OperationType)
	})

	t.Run("creates operation with correct type for epg_source", func(t *testing.T) {
		svc := newTestProgressService()
		ownerID := models.NewULID()

		stages := []core.Stage{
			&mockStage{id: "ingest", name: "Ingest"},
		}

		mgr, err := progress.StartPipelineOperation(svc, "epg_source", ownerID, stages)
		require.NoError(t, err)
		require.NotNil(t, mgr)

		op, err := svc.GetOperation(mgr.OperationID())
		require.NoError(t, err)
		assert.Equal(t, progress.OpEpgIngestion, op.OperationType)
	})

	t.Run("returns error for duplicate operation", func(t *testing.T) {
		svc := newTestProgressService()
		ownerID := models.NewULID()

		stages := []core.Stage{
			&mockStage{id: "load", name: "Load"},
		}

		// First operation succeeds
		mgr1, err := progress.StartPipelineOperation(svc, "stream_proxy", ownerID, stages)
		require.NoError(t, err)
		require.NotNil(t, mgr1)

		// Second operation with same owner should fail
		mgr2, err := progress.StartPipelineOperation(svc, "stream_proxy", ownerID, stages)
		assert.Error(t, err)
		assert.Nil(t, mgr2)
	})

	t.Run("OperationManager can be used as ProgressReporter", func(t *testing.T) {
		svc := newTestProgressService()
		ownerID := models.NewULID()

		stages := []core.Stage{
			&mockStage{id: "load", name: "Load"},
		}

		mgr, err := progress.StartPipelineOperation(svc, "stream_proxy", ownerID, stages)
		require.NoError(t, err)

		// The manager should be usable as a core.ProgressReporter
		var reporter core.ProgressReporter = mgr
		reporter.ReportProgress(context.Background(), "load", 0.5, "Testing")

		op, err := svc.GetOperation(mgr.OperationID())
		require.NoError(t, err)
		assert.Equal(t, 0.5, op.Stages[0].Progress)
	})
}
