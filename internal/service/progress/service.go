package progress

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/oklog/ulid/v2"
)

// Common errors.
var (
	// ErrOperationExists is returned when attempting to start a duplicate operation.
	ErrOperationExists = errors.New("operation already exists for this owner")
	// ErrOperationNotFound is returned when the operation doesn't exist.
	ErrOperationNotFound = errors.New("operation not found")
)

// Subscriber represents a client subscribed to progress events.
type Subscriber struct {
	ID     string
	Filter *OperationFilter
	Events chan *ProgressEvent
}

// Service manages progress tracking and SSE broadcasting.
type Service struct {
	mu          sync.RWMutex
	operations  map[string]*UniversalProgress
	ownerIndex  map[string]string // ownerKey -> operationID
	subscribers map[string]*Subscriber
	logger      *slog.Logger

	// Throttle tracking per operation ID (ADR-001)
	lastBroadcast map[string]time.Time

	// cleanup configuration
	staleDuration time.Duration
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

// NewService creates a new progress service.
func NewService(logger *slog.Logger) *Service {
	s := &Service{
		operations:    make(map[string]*UniversalProgress),
		ownerIndex:    make(map[string]string),
		subscribers:   make(map[string]*Subscriber),
		lastBroadcast: make(map[string]time.Time),
		logger:        logger.With("component", "progress_service"),
		staleDuration: 5 * time.Minute,
		stopCleanup:   make(chan struct{}),
	}
	return s
}

// Start begins background cleanup of stale operations.
func (s *Service) Start() {
	s.cleanupTicker = time.NewTicker(1 * time.Minute)
	go s.cleanupLoop()
}

// Stop halts the background cleanup.
func (s *Service) Stop() {
	if s.cleanupTicker != nil {
		s.cleanupTicker.Stop()
		close(s.stopCleanup)
	}
}

// cleanupLoop periodically removes stale completed operations.
func (s *Service) cleanupLoop() {
	for {
		select {
		case <-s.cleanupTicker.C:
			s.cleanupStaleOperations()
		case <-s.stopCleanup:
			return
		}
	}
}

// cleanupStaleOperations removes terminal operations older than staleDuration.
func (s *Service) cleanupStaleOperations() {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-s.staleDuration)
	var removed []string

	for opID, op := range s.operations {
		if op.State.IsTerminal() && op.CompletedAt != nil && op.CompletedAt.Before(cutoff) {
			removed = append(removed, opID)
			delete(s.operations, opID)
			delete(s.lastBroadcast, opID) // Clean up throttle tracking (ADR-001)
			ownerKey := makeOwnerKey(op.OwnerType, op.OwnerID)
			if s.ownerIndex[ownerKey] == opID {
				delete(s.ownerIndex, ownerKey)
			}
		}
	}

	if len(removed) > 0 {
		s.logger.Debug("cleaned up stale operations", "count", len(removed))
	}
}

// makeOwnerKey creates a unique key for owner-based lookups.
func makeOwnerKey(ownerType string, ownerID models.ULID) string {
	return ownerType + ":" + ownerID.String()
}

// generateOperationID creates a unique operation identifier.
func generateOperationID() string {
	return ulid.Make().String()
}

// StartOperation begins tracking a new operation.
// Returns ErrOperationExists if an active operation already exists for this owner.
func (s *Service) StartOperation(
	opType OperationType,
	ownerID models.ULID,
	ownerType string,
	ownerName string,
	stages []StageInfo,
) (*OperationManager, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ownerKey := makeOwnerKey(ownerType, ownerID)

	// Check for existing active operation
	if existingOpID, exists := s.ownerIndex[ownerKey]; exists {
		if existing, ok := s.operations[existingOpID]; ok {
			if existing.State.IsActive() {
				return nil, ErrOperationExists
			}
		}
	}

	operationID := generateOperationID()
	now := time.Now()

	// Initialize stages
	for i := range stages {
		stages[i].State = StateIdle
		stages[i].Progress = 0
	}

	progress := &UniversalProgress{
		OperationID:       operationID,
		OperationType:     opType,
		OwnerID:           ownerID,
		OwnerType:         ownerType,
		OwnerName:         ownerName,
		State:             StatePreparing,
		Progress:          0,
		Message:           "Starting operation",
		Stages:            stages,
		CurrentStageIndex: -1,
		StartedAt:         now,
		UpdatedAt:         now,
		Metadata:          make(map[string]any),
	}

	s.operations[operationID] = progress
	s.ownerIndex[ownerKey] = operationID

	s.logger.Debug("started operation",
		"operation_id", operationID,
		"operation_type", opType,
		"owner_type", ownerType,
		"owner_id", ownerID.String(),
	)

	// Broadcast initial progress
	s.broadcastLocked(progress)

	return &OperationManager{
		service:     s,
		operationID: operationID,
	}, nil
}

// GetOperation returns the current progress for an operation.
func (s *Service) GetOperation(operationID string) (*UniversalProgress, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	op, ok := s.operations[operationID]
	if !ok {
		return nil, ErrOperationNotFound
	}
	return op.Clone(), nil
}

// GetOperationByOwner returns the current operation for an owner.
func (s *Service) GetOperationByOwner(ownerType string, ownerID models.ULID) (*UniversalProgress, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ownerKey := makeOwnerKey(ownerType, ownerID)
	opID, ok := s.ownerIndex[ownerKey]
	if !ok {
		return nil, ErrOperationNotFound
	}

	op, ok := s.operations[opID]
	if !ok {
		return nil, ErrOperationNotFound
	}
	return op.Clone(), nil
}

// ListOperations returns all operations matching the filter.
func (s *Service) ListOperations(filter *OperationFilter) []*UniversalProgress {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*UniversalProgress
	for _, op := range s.operations {
		if filter.Matches(op) {
			result = append(result, op.Clone())
		}
	}
	return result
}

// Subscribe creates a new subscriber for progress events.
func (s *Service) Subscribe(filter *OperationFilter) *Subscriber {
	s.mu.Lock()
	defer s.mu.Unlock()

	sub := &Subscriber{
		ID:     generateOperationID(),
		Filter: filter,
		Events: make(chan *ProgressEvent, 100),
	}

	s.subscribers[sub.ID] = sub

	s.logger.Debug("subscriber added", "subscriber_id", sub.ID)

	return sub
}

// Unsubscribe removes a subscriber.
func (s *Service) Unsubscribe(subscriberID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sub, ok := s.subscribers[subscriberID]; ok {
		close(sub.Events)
		delete(s.subscribers, subscriberID)
		s.logger.Debug("subscriber removed", "subscriber_id", subscriberID)
	}
}

// updateOperation updates an operation and broadcasts to subscribers.
func (s *Service) updateOperation(operationID string, updateFn func(*UniversalProgress)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	op, ok := s.operations[operationID]
	if !ok {
		return ErrOperationNotFound
	}

	updateFn(op)
	op.UpdatedAt = time.Now()

	s.broadcastLocked(op)
	return nil
}

// updateOperationSilent updates an operation without broadcasting.
// Use this for intermediate updates that will be broadcasted later.
func (s *Service) updateOperationSilent(operationID string, updateFn func(*UniversalProgress)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	op, ok := s.operations[operationID]
	if !ok {
		return ErrOperationNotFound
	}

	updateFn(op)
	op.UpdatedAt = time.Now()

	return nil
}

// updateOperationThrottled updates an operation with throttled broadcasting (ADR-001).
// Updates are accumulated but only broadcast at most every DefaultProgressBroadcastInterval
// per operation ID. Returns true if a broadcast was sent.
func (s *Service) updateOperationThrottled(operationID string, updateFn func(*UniversalProgress)) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	op, ok := s.operations[operationID]
	if !ok {
		return false, ErrOperationNotFound
	}

	updateFn(op)
	op.UpdatedAt = time.Now()

	// Check if we should broadcast based on throttle interval
	lastBroadcast, exists := s.lastBroadcast[operationID]
	if !exists || time.Since(lastBroadcast) >= DefaultProgressBroadcastInterval {
		s.lastBroadcast[operationID] = time.Now()
		s.broadcastLocked(op)
		return true, nil
	}

	return false, nil
}

// updateOperationImmediate updates an operation and always broadcasts immediately.
// Also cleans up throttle tracking if the operation terminates.
// Use this for state transitions and terminal events (ADR-001).
func (s *Service) updateOperationImmediate(operationID string, updateFn func(*UniversalProgress)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	op, ok := s.operations[operationID]
	if !ok {
		return ErrOperationNotFound
	}

	updateFn(op)
	op.UpdatedAt = time.Now()

	// Always broadcast immediately
	s.lastBroadcast[operationID] = time.Now()
	s.broadcastLocked(op)

	// Clean up throttle tracking on terminal states
	if op.State.IsTerminal() {
		delete(s.lastBroadcast, operationID)
	}

	return nil
}

// broadcastLocked sends progress to all matching subscribers.
// Must be called with s.mu held.
func (s *Service) broadcastLocked(progress *UniversalProgress) {
	event := &ProgressEvent{
		EventType: eventTypeForState(progress.State),
		Progress:  progress.Clone(),
		Timestamp: time.Now(),
	}

	isTerminal := progress.State.IsTerminal()

	for _, sub := range s.subscribers {
		if sub.Filter.Matches(progress) {
			if isTerminal {
				// Terminal events (completed, error, cancelled) must be delivered
				// Use a blocking send with a timeout to ensure delivery
				select {
				case sub.Events <- event:
					s.logger.Debug("broadcast terminal event delivered",
						"event_type", event.EventType,
						"subscriber_id", sub.ID,
						"operation_id", progress.OperationID,
					)
				case <-time.After(500 * time.Millisecond):
					// If channel is full for 500ms, log error but don't block forever
					s.logger.Error("failed to deliver terminal event - channel full",
						"event_type", event.EventType,
						"subscriber_id", sub.ID,
						"operation_id", progress.OperationID,
					)
				}
			} else {
				// Non-terminal events can be dropped if channel is full
				select {
				case sub.Events <- event:
				default:
					// Channel full, skip this event
					s.logger.Warn("subscriber event channel full, dropping event",
						"subscriber_id", sub.ID,
						"operation_id", progress.OperationID,
					)
				}
			}
		}
	}
}

// eventTypeForState returns the appropriate event type for a state.
func eventTypeForState(state UniversalState) string {
	switch state {
	case StateCompleted:
		return EventTypeCompleted
	case StateError:
		return EventTypeError
	case StateCancelled:
		return EventTypeCancelled
	default:
		return EventTypeProgress
	}
}

// DefaultProgressBroadcastInterval is the minimum interval between progress broadcasts.
// Updates within this interval are accumulated and sent together to reduce SSE noise.
const DefaultProgressBroadcastInterval = 2 * time.Second

// OperationManager provides methods to update a specific operation.
// Throttle tracking is handled at the Service level per operation ID (ADR-001).
type OperationManager struct {
	service     *Service
	operationID string
}

// OperationID returns the ID of the managed operation.
func (m *OperationManager) OperationID() string {
	return m.operationID
}

// SetMessage updates the operation message with throttled broadcasting (ADR-001).
// Updates are accumulated but only broadcast at most every DefaultProgressBroadcastInterval.
func (m *OperationManager) SetMessage(message string) {
	_, _ = m.service.updateOperationThrottled(m.operationID, func(op *UniversalProgress) {
		op.Message = message
	})
}

// SetState updates the operation state (always broadcasts immediately - state change).
func (m *OperationManager) SetState(state UniversalState) {
	_ = m.service.updateOperationImmediate(m.operationID, func(op *UniversalProgress) {
		op.State = state
		if state.IsTerminal() {
			now := time.Now()
			op.CompletedAt = &now
		}
	})
}

// SetMetadata sets a metadata value with throttled broadcasting (ADR-001).
func (m *OperationManager) SetMetadata(key string, value any) {
	_, _ = m.service.updateOperationThrottled(m.operationID, func(op *UniversalProgress) {
		if op.Metadata == nil {
			op.Metadata = make(map[string]any)
		}
		op.Metadata[key] = value
	})
}

// Complete marks the operation as completed successfully (always broadcasts immediately).
func (m *OperationManager) Complete(message string) {
	_ = m.service.updateOperationImmediate(m.operationID, func(op *UniversalProgress) {
		op.State = StateCompleted
		op.Progress = 1.0
		op.Message = message
		now := time.Now()
		op.CompletedAt = &now

		// Mark all stages as completed
		for i := range op.Stages {
			if op.Stages[i].State != StateCompleted {
				op.Stages[i].State = StateCompleted
				op.Stages[i].Progress = 1.0
				op.Stages[i].CompletedAt = &now
			}
		}
	})

	m.service.logger.Debug("operation completed",
		"operation_id", m.operationID,
		"message", message,
	)
}

// Fail marks the operation as failed with an error (always broadcasts immediately).
func (m *OperationManager) Fail(err error) {
	_ = m.service.updateOperationImmediate(m.operationID, func(op *UniversalProgress) {
		op.State = StateError
		op.Error = err.Error()
		op.Message = "Operation failed: " + err.Error()
		now := time.Now()
		op.CompletedAt = &now
	})

	m.service.logger.Error("operation failed",
		"operation_id", m.operationID,
		"error", err,
	)
}

// FailWithDetail marks the operation as failed with structured error details (always broadcasts immediately).
func (m *OperationManager) FailWithDetail(detail ErrorDetail) {
	_ = m.service.updateOperationImmediate(m.operationID, func(op *UniversalProgress) {
		op.State = StateError
		op.Error = detail.Message
		op.ErrorDetail = &detail
		op.Message = "Operation failed: " + detail.Message
		now := time.Now()
		op.CompletedAt = &now
	})

	m.service.logger.Error("operation failed",
		"operation_id", m.operationID,
		"stage", detail.Stage,
		"message", detail.Message,
		"technical", detail.Technical,
	)
}

// AddWarning adds a warning message to the operation with throttled broadcasting (ADR-001).
func (m *OperationManager) AddWarning(warning string) {
	_, _ = m.service.updateOperationThrottled(m.operationID, func(op *UniversalProgress) {
		op.Warnings = append(op.Warnings, warning)
		op.WarningCount = len(op.Warnings)
	})

	m.service.logger.Warn("operation warning",
		"operation_id", m.operationID,
		"warning", warning,
	)
}

// Cancel marks the operation as cancelled (always broadcasts immediately).
func (m *OperationManager) Cancel() {
	_ = m.service.updateOperationImmediate(m.operationID, func(op *UniversalProgress) {
		op.State = StateCancelled
		op.Message = "Operation cancelled"
		now := time.Now()
		op.CompletedAt = &now
	})

	m.service.logger.Debug("operation cancelled", "operation_id", m.operationID)
}

// StartStage begins a new stage (always broadcasts immediately - state change).
func (m *OperationManager) StartStage(stageID string) *StageUpdater {
	_ = m.service.updateOperationImmediate(m.operationID, func(op *UniversalProgress) {
		for i := range op.Stages {
			if op.Stages[i].ID == stageID {
				op.CurrentStageIndex = i
				now := time.Now()
				op.Stages[i].State = StateProcessing
				op.Stages[i].StartedAt = &now
				op.Stages[i].Progress = 0
				op.State = StateProcessing
				op.Message = op.Stages[i].Name
				break
			}
		}
	})

	return &StageUpdater{
		manager: m,
		stageID: stageID,
	}
}

// recalculateProgressImmediate updates the overall progress and broadcasts immediately.
// Use this for stage transitions and completions (ADR-001).
func (m *OperationManager) recalculateProgressImmediate() {
	_ = m.service.updateOperationImmediate(m.operationID, func(op *UniversalProgress) {
		var totalProgress float64
		var totalWeight float64

		for _, stage := range op.Stages {
			totalProgress += stage.Weight * stage.Progress
			totalWeight += stage.Weight
		}

		if totalWeight > 0 {
			op.Progress = totalProgress / totalWeight
		}
	})
}

// StageUpdater provides methods to update a specific stage.
type StageUpdater struct {
	manager *OperationManager
	stageID string
}

// SetProgress updates the stage progress (0.0 to 1.0) with throttled broadcasting (ADR-001).
// Updates are accumulated but only broadcast at most every DefaultProgressBroadcastInterval.
func (u *StageUpdater) SetProgress(progress float64, message string) {
	_, _ = u.manager.service.updateOperationThrottled(u.manager.operationID, func(op *UniversalProgress) {
		for i := range op.Stages {
			if op.Stages[i].ID == u.stageID {
				op.Stages[i].Progress = progress
				op.Stages[i].Message = message
				op.Message = message
				break
			}
		}
		// Recalculate overall progress inline
		var totalProgress float64
		var totalWeight float64
		for _, stage := range op.Stages {
			totalProgress += stage.Weight * stage.Progress
			totalWeight += stage.Weight
		}
		if totalWeight > 0 {
			op.Progress = totalProgress / totalWeight
		}
	})
}

// SetItemProgress updates progress with item counts with throttled broadcasting (ADR-001).
// Updates are accumulated but only broadcast at most every DefaultProgressBroadcastInterval.
func (u *StageUpdater) SetItemProgress(current, total int, currentItem string) {
	_, _ = u.manager.service.updateOperationThrottled(u.manager.operationID, func(op *UniversalProgress) {
		for i := range op.Stages {
			if op.Stages[i].ID == u.stageID {
				op.Stages[i].Current = current
				op.Stages[i].Total = total
				op.Stages[i].CurrentItem = currentItem
				op.Stages[i].Message = currentItem // Also update Message for UI display
				op.Message = currentItem           // Update overall message too
				if total > 0 {
					op.Stages[i].Progress = float64(current) / float64(total)
				}
				break
			}
		}
		// Recalculate overall progress inline
		var totalProgress float64
		var totalWeight float64
		for _, stage := range op.Stages {
			totalProgress += stage.Weight * stage.Progress
			totalWeight += stage.Weight
		}
		if totalWeight > 0 {
			op.Progress = totalProgress / totalWeight
		}
	})
}

// Complete marks the stage as completed (ADR-001).
// Stage completions always broadcast immediately (not throttled) as they represent state changes.
func (u *StageUpdater) Complete() {
	_ = u.manager.service.updateOperationImmediate(u.manager.operationID, func(op *UniversalProgress) {
		for i := range op.Stages {
			if op.Stages[i].ID == u.stageID {
				now := time.Now()
				op.Stages[i].State = StateCompleted
				op.Stages[i].Progress = 1.0
				op.Stages[i].CompletedAt = &now
				break
			}
		}
		// Recalculate overall progress inline
		var totalProgress float64
		var totalWeight float64
		for _, stage := range op.Stages {
			totalProgress += stage.Weight * stage.Progress
			totalWeight += stage.Weight
		}
		if totalWeight > 0 {
			op.Progress = totalProgress / totalWeight
		}
	})
}

// Fail marks the stage as failed (ADR-001).
// Stage failures always broadcast immediately (not throttled) as they represent state changes.
func (u *StageUpdater) Fail(err error) {
	_ = u.manager.service.updateOperationImmediate(u.manager.operationID, func(op *UniversalProgress) {
		for i := range op.Stages {
			if op.Stages[i].ID == u.stageID {
				now := time.Now()
				op.Stages[i].State = StateError
				op.Stages[i].Message = err.Error()
				op.Stages[i].CompletedAt = &now
				break
			}
		}
	})
}

// Reporter returns a ProgressReporter interface for this stage.
// This allows handlers to report progress without knowing about the Progress Service internals.
// The returned reporter automatically throttles updates per ADR-001.
func (u *StageUpdater) Reporter() ProgressReporter {
	return &stageReporter{updater: u}
}

// ReportProgress implements core.ProgressReporter for bridge to existing pipeline.
// When progress is 0, this also triggers StartStage to properly transition the stage state.
// When progress is 1, this also triggers Complete to mark the stage as finished.
func (m *OperationManager) ReportProgress(ctx context.Context, stageID string, progress float64, message string) {
	if progress == 0 {
		// Stage is starting - call StartStage to properly initialize it
		m.StartStage(stageID)
	}

	updater := &StageUpdater{manager: m, stageID: stageID}
	updater.SetProgress(progress, message)

	if progress >= 1.0 {
		// Stage is complete - mark it as finished
		updater.Complete()
	}
}

// ReportItemProgress implements core.ProgressReporter for bridge to existing pipeline.
func (m *OperationManager) ReportItemProgress(ctx context.Context, stageID string, current, total int, item string) {
	updater := &StageUpdater{manager: m, stageID: stageID}
	updater.SetItemProgress(current, total, item)
}
