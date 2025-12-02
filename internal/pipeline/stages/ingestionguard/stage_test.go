package ingestionguard

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jmylchreest/tvarr/internal/ingestor"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockStateChecker implements StateChecker for testing.
type MockStateChecker struct {
	mu          sync.RWMutex
	isIngesting bool
	activeCount int
	states      []*ingestor.IngestionState
}

func (m *MockStateChecker) IsAnyIngesting() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isIngesting
}

func (m *MockStateChecker) ActiveIngestionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeCount
}

func (m *MockStateChecker) GetAllStates() []*ingestor.IngestionState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.states
}

func (m *MockStateChecker) SetIngesting(ingesting bool, count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isIngesting = ingesting
	m.activeCount = count
}

func (m *MockStateChecker) SetStates(states []*ingestor.IngestionState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states = states
}

func TestNew(t *testing.T) {
	checker := &MockStateChecker{}
	stage := New(checker)

	assert.NotNil(t, stage)
	assert.Equal(t, StageID, stage.ID())
	assert.Equal(t, StageName, stage.Name())
	assert.Equal(t, DefaultPollInterval, stage.pollInterval)
	assert.Equal(t, DefaultMaxWaitTime, stage.maxWaitTime)
	assert.True(t, stage.enabled)
}

func TestWithPollInterval(t *testing.T) {
	checker := &MockStateChecker{}
	stage := New(checker).WithPollInterval(500 * time.Millisecond)

	assert.Equal(t, 500*time.Millisecond, stage.pollInterval)
}

func TestWithMaxWaitTime(t *testing.T) {
	checker := &MockStateChecker{}
	stage := New(checker).WithMaxWaitTime(10 * time.Second)

	assert.Equal(t, 10*time.Second, stage.maxWaitTime)
}

func TestWithEnabled(t *testing.T) {
	checker := &MockStateChecker{}
	stage := New(checker).WithEnabled(false)

	assert.False(t, stage.enabled)
}

func TestExecute_Disabled(t *testing.T) {
	checker := &MockStateChecker{isIngesting: true, activeCount: 1}
	stage := New(checker).WithEnabled(false)

	state := core.NewState(&models.StreamProxy{})
	result, err := stage.Execute(context.Background(), state)

	require.NoError(t, err)
	assert.Contains(t, result.Message, "disabled")
}

func TestExecute_NoStateChecker(t *testing.T) {
	stage := New(nil)

	state := core.NewState(&models.StreamProxy{})
	result, err := stage.Execute(context.Background(), state)

	require.NoError(t, err)
	assert.Contains(t, result.Message, "No state checker")
}

func TestExecute_NoActiveIngestions(t *testing.T) {
	checker := &MockStateChecker{isIngesting: false, activeCount: 0}
	stage := New(checker)

	state := core.NewState(&models.StreamProxy{})
	result, err := stage.Execute(context.Background(), state)

	require.NoError(t, err)
	assert.Contains(t, result.Message, "No active ingestions")
}

func TestExecute_WaitsForIngestionCompletion(t *testing.T) {
	checker := &MockStateChecker{isIngesting: true, activeCount: 1}
	stage := New(checker).
		WithPollInterval(50 * time.Millisecond).
		WithMaxWaitTime(5 * time.Second)

	// Complete the ingestion after a short delay
	go func() {
		time.Sleep(150 * time.Millisecond)
		checker.SetIngesting(false, 0)
	}()

	state := core.NewState(&models.StreamProxy{})
	start := time.Now()
	result, err := stage.Execute(context.Background(), state)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Contains(t, result.Message, "Waited")
	assert.GreaterOrEqual(t, elapsed, 100*time.Millisecond)
	assert.Equal(t, 1, result.RecordsProcessed)
}

func TestExecute_Timeout(t *testing.T) {
	checker := &MockStateChecker{
		isIngesting: true,
		activeCount: 2,
		states: []*ingestor.IngestionState{
			{SourceName: "StreamCast News", Status: "ingesting"},
			{SourceName: "ViewMedia Sports", Status: "ingesting"},
		},
	}
	stage := New(checker).
		WithPollInterval(50 * time.Millisecond).
		WithMaxWaitTime(200 * time.Millisecond)

	state := core.NewState(&models.StreamProxy{})
	_, err := stage.Execute(context.Background(), state)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
	assert.Contains(t, err.Error(), "still active")
}

func TestExecute_ContextCancellation(t *testing.T) {
	checker := &MockStateChecker{isIngesting: true, activeCount: 1}
	stage := New(checker).
		WithPollInterval(50 * time.Millisecond).
		WithMaxWaitTime(5 * time.Second)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	state := core.NewState(&models.StreamProxy{})
	_, err := stage.Execute(ctx, state)

	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestExecute_MultipleIngestionsComplete(t *testing.T) {
	checker := &MockStateChecker{isIngesting: true, activeCount: 3}
	stage := New(checker).
		WithPollInterval(50 * time.Millisecond).
		WithMaxWaitTime(5 * time.Second)

	// Simulate gradual completion
	go func() {
		time.Sleep(100 * time.Millisecond)
		checker.SetIngesting(true, 2)
		time.Sleep(100 * time.Millisecond)
		checker.SetIngesting(true, 1)
		time.Sleep(100 * time.Millisecond)
		checker.SetIngesting(false, 0)
	}()

	state := core.NewState(&models.StreamProxy{})
	result, err := stage.Execute(context.Background(), state)

	require.NoError(t, err)
	assert.Contains(t, result.Message, "Waited")
	// Should record the initial count of 3 ingestions
	assert.Equal(t, 3, result.RecordsProcessed)
}

func TestStageID(t *testing.T) {
	assert.Equal(t, "ingestion_guard", StageID)
}

func TestStageName(t *testing.T) {
	assert.Equal(t, "Ingestion Guard", StageName)
}

func TestCleanup(t *testing.T) {
	checker := &MockStateChecker{}
	stage := New(checker)

	// Cleanup should be a no-op
	err := stage.Cleanup(context.Background())
	require.NoError(t, err)
}

func TestNewConstructor(t *testing.T) {
	checker := &MockStateChecker{}
	constructor := NewConstructor(checker)

	deps := &core.Dependencies{}
	stage := constructor(deps)

	require.NotNil(t, stage)
	assert.Equal(t, StageID, stage.ID())
}

func TestIntegrationWithRealStateManager(t *testing.T) {
	// Test with the real StateManager to ensure interface compatibility
	stateManager := ingestor.NewStateManager()

	stage := New(stateManager).
		WithPollInterval(50 * time.Millisecond).
		WithMaxWaitTime(1 * time.Second)

	t.Run("no active ingestions", func(t *testing.T) {
		state := core.NewState(&models.StreamProxy{})
		result, err := stage.Execute(context.Background(), state)

		require.NoError(t, err)
		assert.Contains(t, result.Message, "No active ingestions")
	})

	t.Run("waits for ingestion", func(t *testing.T) {
		sourceID := models.NewULID()
		err := stateManager.StartWithID(sourceID, "StreamCast News")
		require.NoError(t, err)

		// Complete after delay
		go func() {
			time.Sleep(100 * time.Millisecond)
			stateManager.Complete(sourceID, 100)
		}()

		state := core.NewState(&models.StreamProxy{})
		result, err := stage.Execute(context.Background(), state)

		require.NoError(t, err)
		assert.Contains(t, result.Message, "Waited")
	})
}
