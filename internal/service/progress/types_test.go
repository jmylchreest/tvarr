package progress

import (
	"testing"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestUniversalState_IsTerminal(t *testing.T) {
	tests := []struct {
		state    UniversalState
		expected bool
	}{
		{StateIdle, false},
		{StatePreparing, false},
		{StateConnecting, false},
		{StateDownloading, false},
		{StateProcessing, false},
		{StateSaving, false},
		{StateCleanup, false},
		{StateCompleted, true},
		{StateError, true},
		{StateCancelled, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.state.IsTerminal())
		})
	}
}

func TestUniversalState_IsActive(t *testing.T) {
	tests := []struct {
		state    UniversalState
		expected bool
	}{
		{StateIdle, false},
		{StatePreparing, true},
		{StateConnecting, true},
		{StateDownloading, true},
		{StateProcessing, true},
		{StateSaving, true},
		{StateCleanup, true},
		{StateCompleted, false},
		{StateError, false},
		{StateCancelled, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.state.IsActive())
		})
	}
}

func TestUniversalProgress_Clone(t *testing.T) {
	now := time.Now()
	original := &UniversalProgress{
		OperationID:   "op-123",
		OperationType: OpProxyRegeneration,
		OwnerID:       models.NewULID(),
		State:         StateProcessing,
		Progress:      0.5,
		Message:       "Processing channels",
		Stages: []StageInfo{
			{ID: "stage1", Name: "Stage 1", Progress: 1.0, State: StateCompleted},
			{ID: "stage2", Name: "Stage 2", Progress: 0.5, State: StateProcessing},
		},
		CurrentStageIndex: 1,
		StartedAt:         now,
		UpdatedAt:         now,
		Metadata: map[string]any{
			"channel_count": 100,
		},
	}

	clone := original.Clone()

	// Verify clone is a separate instance
	assert.NotSame(t, original, clone)
	assert.NotSame(t, &original.Stages, &clone.Stages)
	assert.NotSame(t, &original.Metadata, &clone.Metadata)

	// Verify values are equal
	assert.Equal(t, original.OperationID, clone.OperationID)
	assert.Equal(t, original.OperationType, clone.OperationType)
	assert.Equal(t, original.OwnerID, clone.OwnerID)
	assert.Equal(t, original.State, clone.State)
	assert.Equal(t, original.Progress, clone.Progress)
	assert.Equal(t, original.Message, clone.Message)
	assert.Equal(t, len(original.Stages), len(clone.Stages))
	assert.Equal(t, original.Metadata["channel_count"], clone.Metadata["channel_count"])

	// Verify modifying clone doesn't affect original
	clone.Stages[0].Progress = 0.0
	clone.Metadata["channel_count"] = 200
	assert.Equal(t, 1.0, original.Stages[0].Progress)
	assert.Equal(t, 100, original.Metadata["channel_count"])
}

func TestUniversalProgress_CurrentStage(t *testing.T) {
	t.Run("returns current stage", func(t *testing.T) {
		p := &UniversalProgress{
			Stages: []StageInfo{
				{ID: "stage1", Name: "Stage 1"},
				{ID: "stage2", Name: "Stage 2"},
			},
			CurrentStageIndex: 1,
		}

		stage := p.CurrentStage()
		assert.NotNil(t, stage)
		assert.Equal(t, "stage2", stage.ID)
	})

	t.Run("returns nil for invalid index", func(t *testing.T) {
		p := &UniversalProgress{
			Stages: []StageInfo{
				{ID: "stage1", Name: "Stage 1"},
			},
			CurrentStageIndex: 5,
		}

		stage := p.CurrentStage()
		assert.Nil(t, stage)
	})

	t.Run("returns nil for negative index", func(t *testing.T) {
		p := &UniversalProgress{
			Stages: []StageInfo{
				{ID: "stage1", Name: "Stage 1"},
			},
			CurrentStageIndex: -1,
		}

		stage := p.CurrentStage()
		assert.Nil(t, stage)
	})
}

func TestOperationFilter_Matches(t *testing.T) {
	ownerID := models.NewULID()
	resourceID := models.NewULID()
	opType := OpProxyRegeneration
	state := StateProcessing

	progress := &UniversalProgress{
		OperationType: OpProxyRegeneration,
		OwnerID:       ownerID,
		ResourceID:    &resourceID,
		State:         StateProcessing,
	}

	t.Run("nil filter matches everything", func(t *testing.T) {
		var f *OperationFilter
		assert.True(t, f.Matches(progress))
	})

	t.Run("empty filter matches everything", func(t *testing.T) {
		f := &OperationFilter{}
		assert.True(t, f.Matches(progress))
	})

	t.Run("matches by operation type", func(t *testing.T) {
		f := &OperationFilter{OperationType: &opType}
		assert.True(t, f.Matches(progress))

		otherType := OpStreamIngestion
		f.OperationType = &otherType
		assert.False(t, f.Matches(progress))
	})

	t.Run("matches by owner ID", func(t *testing.T) {
		f := &OperationFilter{OwnerID: &ownerID}
		assert.True(t, f.Matches(progress))

		otherID := models.NewULID()
		f.OwnerID = &otherID
		assert.False(t, f.Matches(progress))
	})

	t.Run("matches by resource ID", func(t *testing.T) {
		f := &OperationFilter{ResourceID: &resourceID}
		assert.True(t, f.Matches(progress))

		otherID := models.NewULID()
		f.ResourceID = &otherID
		assert.False(t, f.Matches(progress))
	})

	t.Run("matches by state", func(t *testing.T) {
		f := &OperationFilter{State: &state}
		assert.True(t, f.Matches(progress))

		otherState := StateCompleted
		f.State = &otherState
		assert.False(t, f.Matches(progress))
	})

	t.Run("matches active only", func(t *testing.T) {
		f := &OperationFilter{ActiveOnly: true}
		assert.True(t, f.Matches(progress))

		completedProgress := &UniversalProgress{
			State: StateCompleted,
		}
		assert.False(t, f.Matches(completedProgress))
	})

	t.Run("combines multiple filters", func(t *testing.T) {
		f := &OperationFilter{
			OperationType: &opType,
			OwnerID:       &ownerID,
			State:         &state,
		}
		assert.True(t, f.Matches(progress))

		// Change one filter to not match
		otherType := OpStreamIngestion
		f.OperationType = &otherType
		assert.False(t, f.Matches(progress))
	})
}

func TestStageInfo_Weight(t *testing.T) {
	// Verify that weights can be used for weighted progress calculation
	stages := []StageInfo{
		{ID: "load", Weight: 0.1, Progress: 1.0},
		{ID: "process", Weight: 0.7, Progress: 0.5},
		{ID: "save", Weight: 0.2, Progress: 0.0},
	}

	// Calculate weighted progress
	var totalProgress float64
	for _, s := range stages {
		totalProgress += s.Weight * s.Progress
	}

	// Expected: 0.1*1.0 + 0.7*0.5 + 0.2*0.0 = 0.1 + 0.35 + 0 = 0.45
	assert.InDelta(t, 0.45, totalProgress, 0.001)
}
