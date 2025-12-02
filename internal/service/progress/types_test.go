package progress

import (
	"encoding/json"
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

// T004-TEST: Test ErrorDetail type JSON serialization
func TestErrorDetail_JSONSerialization(t *testing.T) {
	t.Run("serializes all fields", func(t *testing.T) {
		detail := &ErrorDetail{
			Stage:      "generate_m3u",
			Message:    "Failed to write M3U file",
			Technical:  "permission denied: /output/proxy.m3u",
			Suggestion: "Check output directory permissions",
		}

		data, err := json.Marshal(detail)
		assert.NoError(t, err)

		var parsed map[string]string
		err = json.Unmarshal(data, &parsed)
		assert.NoError(t, err)

		assert.Equal(t, "generate_m3u", parsed["stage"])
		assert.Equal(t, "Failed to write M3U file", parsed["message"])
		assert.Equal(t, "permission denied: /output/proxy.m3u", parsed["technical"])
		assert.Equal(t, "Check output directory permissions", parsed["suggestion"])
	})

	t.Run("omits empty optional fields", func(t *testing.T) {
		detail := &ErrorDetail{
			Stage:   "load_channels",
			Message: "Failed to load channels",
		}

		data, err := json.Marshal(detail)
		assert.NoError(t, err)

		var parsed map[string]any
		err = json.Unmarshal(data, &parsed)
		assert.NoError(t, err)

		assert.Equal(t, "load_channels", parsed["stage"])
		assert.Equal(t, "Failed to load channels", parsed["message"])
		// Technical and Suggestion should be omitted when empty
		_, hasTechnical := parsed["technical"]
		_, hasSuggestion := parsed["suggestion"]
		assert.False(t, hasTechnical, "technical should be omitted when empty")
		assert.False(t, hasSuggestion, "suggestion should be omitted when empty")
	})
}

// T005-TEST: Test UniversalProgress with error fields
func TestUniversalProgress_ErrorFields(t *testing.T) {
	t.Run("includes ErrorDetail in JSON", func(t *testing.T) {
		progress := &UniversalProgress{
			OperationID:   "op-123",
			OperationType: OpProxyRegeneration,
			OwnerID:       models.NewULID(),
			State:         StateError,
			Error:         "Pipeline failed",
			ErrorDetail: &ErrorDetail{
				Stage:      "generate_m3u",
				Message:    "Failed to write M3U file",
				Technical:  "permission denied",
				Suggestion: "Check permissions",
			},
			WarningCount: 2,
			Warnings:     []string{"Channel X skipped", "Channel Y skipped"},
			StartedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}

		data, err := json.Marshal(progress)
		assert.NoError(t, err)

		var parsed map[string]any
		err = json.Unmarshal(data, &parsed)
		assert.NoError(t, err)

		// Verify error_detail is present
		errorDetail, ok := parsed["error_detail"].(map[string]any)
		assert.True(t, ok, "error_detail should be present")
		assert.Equal(t, "generate_m3u", errorDetail["stage"])
		assert.Equal(t, "Failed to write M3U file", errorDetail["message"])

		// Verify warning fields
		assert.Equal(t, float64(2), parsed["warning_count"])
		warnings, ok := parsed["warnings"].([]any)
		assert.True(t, ok, "warnings should be an array")
		assert.Len(t, warnings, 2)
	})

	t.Run("omits error fields when nil or empty", func(t *testing.T) {
		progress := &UniversalProgress{
			OperationID:   "op-456",
			OperationType: OpProxyRegeneration,
			OwnerID:       models.NewULID(),
			State:         StateCompleted,
			StartedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}

		data, err := json.Marshal(progress)
		assert.NoError(t, err)

		var parsed map[string]any
		err = json.Unmarshal(data, &parsed)
		assert.NoError(t, err)

		_, hasErrorDetail := parsed["error_detail"]
		_, hasWarningCount := parsed["warning_count"]
		_, hasWarnings := parsed["warnings"]
		assert.False(t, hasErrorDetail, "error_detail should be omitted when nil")
		assert.False(t, hasWarningCount, "warning_count should be omitted when zero")
		assert.False(t, hasWarnings, "warnings should be omitted when empty")
	})

	t.Run("Clone preserves error fields", func(t *testing.T) {
		original := &UniversalProgress{
			OperationID:   "op-789",
			OperationType: OpProxyRegeneration,
			OwnerID:       models.NewULID(),
			State:         StateError,
			ErrorDetail: &ErrorDetail{
				Stage:   "test_stage",
				Message: "Test error",
			},
			WarningCount: 3,
			Warnings:     []string{"Warning 1", "Warning 2", "Warning 3"},
			StartedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}

		clone := original.Clone()

		// Verify clone has same values
		assert.NotNil(t, clone.ErrorDetail)
		assert.Equal(t, original.ErrorDetail.Stage, clone.ErrorDetail.Stage)
		assert.Equal(t, original.ErrorDetail.Message, clone.ErrorDetail.Message)
		assert.Equal(t, original.WarningCount, clone.WarningCount)
		assert.Equal(t, original.Warnings, clone.Warnings)

		// Verify modifying clone doesn't affect original
		clone.ErrorDetail.Stage = "modified"
		clone.Warnings[0] = "Modified"
		assert.Equal(t, "test_stage", original.ErrorDetail.Stage)
		assert.Equal(t, "Warning 1", original.Warnings[0])
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
