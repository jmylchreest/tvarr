package ingestor

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
)

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
}

// NewStateManager creates a new state manager.
func NewStateManager() *StateManager {
	return &StateManager{
		states: make(map[models.ULID]*IngestionState),
	}
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

	if _, exists := m.states[id]; exists {
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

	// Remove from active states after a short delay to allow status checks
	go func() {
		time.Sleep(5 * time.Second)
		m.mu.Lock()
		delete(m.states, sourceID)
		m.mu.Unlock()
	}()
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

	// Remove from active states after a short delay
	go func() {
		time.Sleep(5 * time.Second)
		m.mu.Lock()
		delete(m.states, sourceID)
		m.mu.Unlock()
	}()
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
