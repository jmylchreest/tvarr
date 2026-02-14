package ingestor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
)

const cleanupDelay = 5 * time.Second

// IngestionState represents the state of an ongoing ingestion.
type IngestionState struct {
	SourceID    models.ULID
	SourceName  string
	StartedAt   time.Time
	Status      string
	Processed   int
	Errors      int
	LastUpdated time.Time
	Error       error
}

// StateManager tracks the state of ongoing ingestions.
type StateManager struct {
	mu     sync.RWMutex
	states map[models.ULID]*IngestionState

	// Cleanup management
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	cleanupCh chan models.ULID
}

// NewStateManager creates a new state manager.
func NewStateManager() *StateManager {
	ctx, cancel := context.WithCancel(context.Background())
	sm := &StateManager{
		states:    make(map[models.ULID]*IngestionState),
		ctx:       ctx,
		cancel:    cancel,
		cleanupCh: make(chan models.ULID, 100),
	}
	sm.startCleanupWorker()
	return sm
}

// startCleanupWorker starts a background goroutine that handles delayed cleanup.
func (m *StateManager) startCleanupWorker() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		timers := make(map[models.ULID]*time.Timer)
		for {
			select {
			case <-m.ctx.Done():
				for _, timer := range timers {
					timer.Stop()
				}
				return
			case id := <-m.cleanupCh:
				if _, exists := timers[id]; exists {
					continue
				}
				cleanupID := id
				timers[id] = time.AfterFunc(cleanupDelay, func() {
					m.mu.Lock()
					delete(m.states, cleanupID)
					m.mu.Unlock()
				})
			}
		}
	}()
}

// Stop gracefully shuts down the state manager.
func (m *StateManager) Stop() {
	m.cancel()
	m.wg.Wait()
}

// Start marks an ingestion as started for a stream source.
func (m *StateManager) Start(source *models.StreamSource) error {
	return m.StartWithID(source.ID, source.Name)
}

// StartWithID marks an ingestion as started using just the ID and name.
// This is useful for EPG sources or other entities that need state tracking.
func (m *StateManager) StartWithID(id models.ULID, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Only error if an ingestion is actively in progress (status == "ingesting")
	// Completed/failed states may still exist briefly during cleanup window
	if state, exists := m.states[id]; exists && state.Status == "ingesting" {
		return fmt.Errorf("ingestion already in progress for source %s", id)
	}

	m.states[id] = &IngestionState{
		SourceID:    id,
		SourceName:  name,
		StartedAt:   time.Now(),
		Status:      "ingesting",
		LastUpdated: time.Now(),
	}

	return nil
}

// UpdateProgress updates the progress of an ingestion.
func (m *StateManager) UpdateProgress(sourceID models.ULID, processed, errors int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.states[sourceID]; exists {
		state.Processed = processed
		state.Errors = errors
		state.LastUpdated = time.Now()
	}
}

// Complete marks an ingestion as completed successfully.
func (m *StateManager) Complete(sourceID models.ULID, processed int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.states[sourceID]; exists {
		state.Status = "completed"
		state.Processed = processed
		state.LastUpdated = time.Now()
	}

	select {
	case m.cleanupCh <- sourceID:
	default:
	}
}

// Fail marks an ingestion as failed.
func (m *StateManager) Fail(sourceID models.ULID, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.states[sourceID]; exists {
		state.Status = "failed"
		state.Error = err
		state.LastUpdated = time.Now()
	}

	select {
	case m.cleanupCh <- sourceID:
	default:
	}
}

// Cancel marks an ingestion as cancelled.
func (m *StateManager) Cancel(sourceID models.ULID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.states[sourceID]; exists {
		state.Status = "cancelled"
		state.LastUpdated = time.Now()
	}

	delete(m.states, sourceID)
}

// GetState returns the state of an ingestion.
func (m *StateManager) GetState(sourceID models.ULID) (*IngestionState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.states[sourceID]
	if !exists {
		return nil, false
	}

	// Return a copy to prevent race conditions
	copy := *state
	return &copy, true
}

// IsIngesting returns true if an ingestion is in progress for the source.
func (m *StateManager) IsIngesting(sourceID models.ULID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.states[sourceID]
	return exists && state.Status == "ingesting"
}

// IsAnyIngesting returns true if any ingestion is currently in progress.
func (m *StateManager) IsAnyIngesting() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, state := range m.states {
		if state.Status == "ingesting" {
			return true
		}
	}
	return false
}

// ActiveIngestionCount returns the number of active ingestions.
func (m *StateManager) ActiveIngestionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, state := range m.states {
		if state.Status == "ingesting" {
			count++
		}
	}
	return count
}

// GetAllStates returns all current ingestion states.
func (m *StateManager) GetAllStates() []*IngestionState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	states := make([]*IngestionState, 0, len(m.states))
	for _, state := range m.states {
		copy := *state
		states = append(states, &copy)
	}
	return states
}

// WaitForCompletion waits for an ingestion to complete or the context to be cancelled.
func (m *StateManager) WaitForCompletion(ctx context.Context, sourceID models.ULID) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			state, exists := m.GetState(sourceID)
			if !exists {
				return nil // Ingestion completed and was cleaned up
			}
			if state.Status != "ingesting" {
				if state.Error != nil {
					return state.Error
				}
				return nil
			}
		}
	}
}
