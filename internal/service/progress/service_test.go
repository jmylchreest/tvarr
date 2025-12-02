package progress

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestService() *Service {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewService(logger)
}

func TestService_StartOperation(t *testing.T) {
	svc := newTestService()
	ownerID := models.NewULID()

	stages := []StageInfo{
		{ID: "load", Name: "Load Data", Weight: 0.3},
		{ID: "process", Name: "Process Data", Weight: 0.5},
		{ID: "save", Name: "Save Results", Weight: 0.2},
	}

	t.Run("creates operation successfully", func(t *testing.T) {
		mgr, err := svc.StartOperation(OpProxyRegeneration, ownerID, "stream_proxy", stages)
		require.NoError(t, err)
		require.NotNil(t, mgr)

		op, err := svc.GetOperation(mgr.OperationID())
		require.NoError(t, err)
		assert.Equal(t, OpProxyRegeneration, op.OperationType)
		assert.Equal(t, ownerID, op.OwnerID)
		assert.Equal(t, "stream_proxy", op.OwnerType)
		assert.Equal(t, StatePreparing, op.State)
		assert.Len(t, op.Stages, 3)
	})

	t.Run("blocks duplicate active operation", func(t *testing.T) {
		anotherOwner := models.NewULID()
		_, err := svc.StartOperation(OpProxyRegeneration, anotherOwner, "stream_proxy", stages)
		require.NoError(t, err)

		// Try to start another operation for the same owner
		_, err = svc.StartOperation(OpProxyRegeneration, anotherOwner, "stream_proxy", stages)
		assert.ErrorIs(t, err, ErrOperationExists)
	})

	t.Run("allows new operation after completion", func(t *testing.T) {
		newOwner := models.NewULID()
		mgr, err := svc.StartOperation(OpProxyRegeneration, newOwner, "stream_proxy", stages)
		require.NoError(t, err)

		// Complete the operation
		mgr.Complete("Done")

		// Should allow new operation
		mgr2, err := svc.StartOperation(OpProxyRegeneration, newOwner, "stream_proxy", stages)
		require.NoError(t, err)
		assert.NotEqual(t, mgr.OperationID(), mgr2.OperationID())
	})
}

func TestService_GetOperation(t *testing.T) {
	svc := newTestService()
	ownerID := models.NewULID()

	stages := []StageInfo{
		{ID: "load", Name: "Load", Weight: 1.0},
	}

	mgr, err := svc.StartOperation(OpStreamIngestion, ownerID, "stream_source", stages)
	require.NoError(t, err)

	t.Run("returns operation by ID", func(t *testing.T) {
		op, err := svc.GetOperation(mgr.OperationID())
		require.NoError(t, err)
		assert.Equal(t, mgr.OperationID(), op.OperationID)
	})

	t.Run("returns error for unknown ID", func(t *testing.T) {
		_, err := svc.GetOperation("unknown-id")
		assert.ErrorIs(t, err, ErrOperationNotFound)
	})
}

func TestService_GetOperationByOwner(t *testing.T) {
	svc := newTestService()
	ownerID := models.NewULID()

	stages := []StageInfo{
		{ID: "load", Name: "Load", Weight: 1.0},
	}

	mgr, err := svc.StartOperation(OpStreamIngestion, ownerID, "stream_source", stages)
	require.NoError(t, err)

	t.Run("returns operation by owner", func(t *testing.T) {
		op, err := svc.GetOperationByOwner("stream_source", ownerID)
		require.NoError(t, err)
		assert.Equal(t, mgr.OperationID(), op.OperationID)
	})

	t.Run("returns error for unknown owner", func(t *testing.T) {
		_, err := svc.GetOperationByOwner("stream_source", models.NewULID())
		assert.ErrorIs(t, err, ErrOperationNotFound)
	})
}

func TestService_ListOperations(t *testing.T) {
	svc := newTestService()

	stages := []StageInfo{{ID: "s1", Name: "Stage 1", Weight: 1.0}}

	// Create various operations
	owner1 := models.NewULID()
	owner2 := models.NewULID()
	owner3 := models.NewULID()

	mgr1, _ := svc.StartOperation(OpProxyRegeneration, owner1, "stream_proxy", stages)
	_, _ = svc.StartOperation(OpStreamIngestion, owner2, "stream_source", stages)
	mgr3, _ := svc.StartOperation(OpProxyRegeneration, owner3, "stream_proxy", stages)
	mgr3.Complete("Done")

	t.Run("returns all operations with nil filter", func(t *testing.T) {
		ops := svc.ListOperations(nil)
		assert.Len(t, ops, 3)
	})

	t.Run("filters by operation type", func(t *testing.T) {
		opType := OpProxyRegeneration
		ops := svc.ListOperations(&OperationFilter{OperationType: &opType})
		assert.Len(t, ops, 2)
	})

	t.Run("filters by active only", func(t *testing.T) {
		ops := svc.ListOperations(&OperationFilter{ActiveOnly: true})
		assert.Len(t, ops, 2)
		for _, op := range ops {
			assert.True(t, op.State.IsActive())
		}
	})

	t.Run("filters by owner ID", func(t *testing.T) {
		ops := svc.ListOperations(&OperationFilter{OwnerID: &owner1})
		assert.Len(t, ops, 1)
		assert.Equal(t, mgr1.OperationID(), ops[0].OperationID)
	})
}

func TestService_Subscribe(t *testing.T) {
	svc := newTestService()
	ownerID := models.NewULID()

	stages := []StageInfo{
		{ID: "load", Name: "Load", Weight: 1.0},
	}

	t.Run("receives progress events", func(t *testing.T) {
		sub := svc.Subscribe(nil)
		defer svc.Unsubscribe(sub.ID)

		// Start operation after subscribing
		mgr, err := svc.StartOperation(OpProxyRegeneration, ownerID, "stream_proxy", stages)
		require.NoError(t, err)

		// Should receive initial event
		select {
		case event := <-sub.Events:
			assert.Equal(t, EventTypeProgress, event.EventType)
			assert.Equal(t, StatePreparing, event.Progress.State)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("expected to receive event")
		}

		// Update and receive update
		mgr.SetMessage("Loading...")
		select {
		case event := <-sub.Events:
			assert.Equal(t, "Loading...", event.Progress.Message)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("expected to receive update event")
		}

		// Complete and receive completion event
		mgr.Complete("Done")
		select {
		case event := <-sub.Events:
			assert.Equal(t, EventTypeCompleted, event.EventType)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("expected to receive completion event")
		}
	})

	t.Run("filters events by operation type", func(t *testing.T) {
		opType := OpStreamIngestion
		sub := svc.Subscribe(&OperationFilter{OperationType: &opType})
		defer svc.Unsubscribe(sub.ID)

		// Start a proxy operation (should not match)
		proxyOwner := models.NewULID()
		_, err := svc.StartOperation(OpProxyRegeneration, proxyOwner, "stream_proxy", stages)
		require.NoError(t, err)

		// Should not receive event
		select {
		case <-sub.Events:
			t.Fatal("should not receive event for non-matching operation type")
		case <-time.After(50 * time.Millisecond):
			// Expected
		}

		// Start a stream ingestion (should match)
		ingestOwner := models.NewULID()
		_, err = svc.StartOperation(OpStreamIngestion, ingestOwner, "stream_source", stages)
		require.NoError(t, err)

		// Should receive event
		select {
		case event := <-sub.Events:
			assert.Equal(t, OpStreamIngestion, event.Progress.OperationType)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("expected to receive event for matching operation type")
		}
	})
}

func TestOperationManager_StageWorkflow(t *testing.T) {
	svc := newTestService()
	ownerID := models.NewULID()

	stages := []StageInfo{
		{ID: "load", Name: "Load Data", Weight: 0.3},
		{ID: "process", Name: "Process Data", Weight: 0.5},
		{ID: "save", Name: "Save Results", Weight: 0.2},
	}

	mgr, err := svc.StartOperation(OpProxyRegeneration, ownerID, "stream_proxy", stages)
	require.NoError(t, err)

	// Start first stage
	stageUpdater := mgr.StartStage("load")

	op, _ := svc.GetOperation(mgr.OperationID())
	assert.Equal(t, 0, op.CurrentStageIndex)
	assert.Equal(t, StateProcessing, op.Stages[0].State)

	// Update progress
	stageUpdater.SetItemProgress(50, 100, "channel-50")

	op, _ = svc.GetOperation(mgr.OperationID())
	assert.Equal(t, 50, op.Stages[0].Current)
	assert.Equal(t, 100, op.Stages[0].Total)
	assert.InDelta(t, 0.15, op.Progress, 0.01) // 0.3 * 0.5 = 0.15

	// Complete first stage
	stageUpdater.Complete()

	op, _ = svc.GetOperation(mgr.OperationID())
	assert.Equal(t, StateCompleted, op.Stages[0].State)

	// Start second stage
	stageUpdater = mgr.StartStage("process")
	stageUpdater.SetProgress(0.5, "Processing...")

	op, _ = svc.GetOperation(mgr.OperationID())
	assert.Equal(t, 1, op.CurrentStageIndex)
	assert.InDelta(t, 0.55, op.Progress, 0.01) // 0.3*1.0 + 0.5*0.5 = 0.55

	// Complete remaining stages
	stageUpdater.Complete()
	mgr.StartStage("save").Complete()

	op, _ = svc.GetOperation(mgr.OperationID())
	assert.InDelta(t, 1.0, op.Progress, 0.01)

	// Complete operation
	mgr.Complete("All done!")

	op, _ = svc.GetOperation(mgr.OperationID())
	assert.Equal(t, StateCompleted, op.State)
	assert.NotNil(t, op.CompletedAt)
}

func TestOperationManager_Fail(t *testing.T) {
	svc := newTestService()
	ownerID := models.NewULID()

	stages := []StageInfo{
		{ID: "load", Name: "Load", Weight: 1.0},
	}

	// Subscribe BEFORE starting operation to receive all events
	sub := svc.Subscribe(nil)
	defer svc.Unsubscribe(sub.ID)

	mgr, err := svc.StartOperation(OpProxyRegeneration, ownerID, "stream_proxy", stages)
	require.NoError(t, err)

	// Clear initial event from StartOperation
	select {
	case <-sub.Events:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected to receive initial event")
	}

	// Fail the operation
	mgr.Fail(assert.AnError)

	// Check state
	op, _ := svc.GetOperation(mgr.OperationID())
	assert.Equal(t, StateError, op.State)
	assert.Contains(t, op.Error, assert.AnError.Error())

	// Check event
	select {
	case event := <-sub.Events:
		assert.Equal(t, EventTypeError, event.EventType)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected to receive error event")
	}
}

func TestOperationManager_Cancel(t *testing.T) {
	svc := newTestService()
	ownerID := models.NewULID()

	stages := []StageInfo{
		{ID: "load", Name: "Load", Weight: 1.0},
	}

	// Subscribe BEFORE starting operation to receive all events
	sub := svc.Subscribe(nil)
	defer svc.Unsubscribe(sub.ID)

	mgr, err := svc.StartOperation(OpProxyRegeneration, ownerID, "stream_proxy", stages)
	require.NoError(t, err)

	// Clear initial event from StartOperation
	select {
	case <-sub.Events:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected to receive initial event")
	}

	// Cancel the operation
	mgr.Cancel()

	// Check state
	op, _ := svc.GetOperation(mgr.OperationID())
	assert.Equal(t, StateCancelled, op.State)

	// Check event
	select {
	case event := <-sub.Events:
		assert.Equal(t, EventTypeCancelled, event.EventType)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected to receive cancelled event")
	}
}

func TestOperationManager_Metadata(t *testing.T) {
	svc := newTestService()
	ownerID := models.NewULID()

	stages := []StageInfo{
		{ID: "load", Name: "Load", Weight: 1.0},
	}

	mgr, err := svc.StartOperation(OpProxyRegeneration, ownerID, "stream_proxy", stages)
	require.NoError(t, err)

	mgr.SetMetadata("channel_count", 100)
	mgr.SetMetadata("source_name", "Test Source")

	op, _ := svc.GetOperation(mgr.OperationID())
	assert.Equal(t, 100, op.Metadata["channel_count"])
	assert.Equal(t, "Test Source", op.Metadata["source_name"])
}

func TestService_CleanupStaleOperations(t *testing.T) {
	svc := newTestService()
	svc.staleDuration = 50 * time.Millisecond // Very short for testing

	ownerID := models.NewULID()
	stages := []StageInfo{{ID: "s1", Name: "Stage 1", Weight: 1.0}}

	mgr, err := svc.StartOperation(OpProxyRegeneration, ownerID, "stream_proxy", stages)
	require.NoError(t, err)

	// Complete the operation
	mgr.Complete("Done")

	// Verify operation exists
	_, err = svc.GetOperation(mgr.OperationID())
	require.NoError(t, err)

	// Wait for stale duration
	time.Sleep(100 * time.Millisecond)

	// Trigger cleanup
	svc.cleanupStaleOperations()

	// Should be cleaned up
	_, err = svc.GetOperation(mgr.OperationID())
	assert.ErrorIs(t, err, ErrOperationNotFound)
}
