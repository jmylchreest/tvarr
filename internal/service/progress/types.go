// Package progress provides real-time progress tracking and SSE broadcasting.
package progress

import (
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
)

// UniversalState represents the current state of an operation.
type UniversalState string

const (
	// StateIdle indicates the operation has not started.
	StateIdle UniversalState = "idle"
	// StatePreparing indicates the operation is initializing.
	StatePreparing UniversalState = "preparing"
	// StateConnecting indicates the operation is connecting to a remote resource.
	StateConnecting UniversalState = "connecting"
	// StateDownloading indicates the operation is downloading data.
	StateDownloading UniversalState = "downloading"
	// StateProcessing indicates the operation is processing data.
	StateProcessing UniversalState = "processing"
	// StateSaving indicates the operation is saving results.
	StateSaving UniversalState = "saving"
	// StateCleanup indicates the operation is cleaning up.
	StateCleanup UniversalState = "cleanup"
	// StateCompleted indicates the operation completed successfully.
	StateCompleted UniversalState = "completed"
	// StateError indicates the operation failed with an error.
	StateError UniversalState = "error"
	// StateCancelled indicates the operation was cancelled.
	StateCancelled UniversalState = "cancelled"
)

// IsTerminal returns true if this is a terminal state (completed, error, or cancelled).
func (s UniversalState) IsTerminal() bool {
	return s == StateCompleted || s == StateError || s == StateCancelled
}

// IsActive returns true if the operation is currently running.
func (s UniversalState) IsActive() bool {
	return s != StateIdle && !s.IsTerminal()
}

// OperationType identifies the type of operation being tracked.
type OperationType string

const (
	// OpStreamIngestion is ingesting channels from a stream source.
	OpStreamIngestion OperationType = "stream_ingestion"
	// OpEpgIngestion is ingesting programs from an EPG source.
	OpEpgIngestion OperationType = "epg_ingestion"
	// OpProxyRegeneration is regenerating a proxy's output files.
	OpProxyRegeneration OperationType = "proxy_regeneration"
	// OpPipeline is executing a pipeline stage.
	OpPipeline OperationType = "pipeline"
	// OpDataMapping is applying data mapping rules.
	OpDataMapping OperationType = "data_mapping"
	// OpLogoCaching is caching logo images.
	OpLogoCaching OperationType = "logo_caching"
	// OpFiltering is applying filter rules.
	OpFiltering OperationType = "filtering"
	// OpMaintenance is performing system maintenance.
	OpMaintenance OperationType = "maintenance"
	// OpDatabase is performing database operations.
	OpDatabase OperationType = "database"
)

// StageInfo describes a single stage within an operation.
type StageInfo struct {
	// ID is the unique identifier for the stage.
	ID string `json:"id"`
	// Name is the human-readable stage name.
	Name string `json:"name"`
	// Weight determines the relative progress contribution (0.0 to 1.0).
	Weight float64 `json:"weight"`
	// State is the current state of the stage.
	State UniversalState `json:"state"`
	// Progress is the completion percentage within this stage (0.0 to 1.0).
	Progress float64 `json:"progress"`
	// Message describes the current activity.
	Message string `json:"message"`
	// Current is the number of items processed.
	Current int `json:"current"`
	// Total is the total number of items to process.
	Total int `json:"total"`
	// CurrentItem is the name of the item currently being processed.
	CurrentItem string `json:"current_item,omitempty"`
	// StartedAt is when the stage started.
	StartedAt *time.Time `json:"started_at,omitempty"`
	// CompletedAt is when the stage completed.
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// UniversalProgress represents the complete progress of an operation.
type UniversalProgress struct {
	// OperationID is the unique identifier for this operation.
	OperationID string `json:"operation_id"`
	// OperationType identifies what kind of operation this is.
	OperationType OperationType `json:"operation_type"`
	// OwnerID identifies the resource that owns this operation (e.g., proxy ID).
	OwnerID models.ULID `json:"owner_id"`
	// OwnerType identifies the type of owner (e.g., "stream_proxy").
	OwnerType string `json:"owner_type"`
	// ResourceID is an optional additional resource identifier.
	ResourceID *models.ULID `json:"resource_id,omitempty"`
	// State is the overall operation state.
	State UniversalState `json:"state"`
	// Progress is the overall completion percentage (0.0 to 1.0).
	Progress float64 `json:"progress"`
	// Message is the current status message.
	Message string `json:"message"`
	// Stages contains progress for each stage.
	Stages []StageInfo `json:"stages"`
	// CurrentStageIndex is the index of the currently executing stage.
	CurrentStageIndex int `json:"current_stage_index"`
	// StartedAt is when the operation started.
	StartedAt time.Time `json:"started_at"`
	// UpdatedAt is when the progress was last updated.
	UpdatedAt time.Time `json:"updated_at"`
	// CompletedAt is when the operation completed (if terminal).
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	// Error contains error details if State is StateError.
	Error string `json:"error,omitempty"`
	// Metadata contains operation-specific data.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Clone creates a deep copy of the progress for thread-safe reading.
func (p *UniversalProgress) Clone() *UniversalProgress {
	clone := *p
	clone.Stages = make([]StageInfo, len(p.Stages))
	copy(clone.Stages, p.Stages)
	if p.Metadata != nil {
		clone.Metadata = make(map[string]any, len(p.Metadata))
		for k, v := range p.Metadata {
			clone.Metadata[k] = v
		}
	}
	return &clone
}

// CurrentStage returns the currently active stage, if any.
func (p *UniversalProgress) CurrentStage() *StageInfo {
	if p.CurrentStageIndex >= 0 && p.CurrentStageIndex < len(p.Stages) {
		return &p.Stages[p.CurrentStageIndex]
	}
	return nil
}

// ProgressEvent is sent to SSE subscribers when progress changes.
type ProgressEvent struct {
	// EventType identifies the type of event.
	EventType string `json:"event_type"`
	// Progress contains the current progress state.
	Progress *UniversalProgress `json:"progress"`
	// Timestamp is when the event was generated.
	Timestamp time.Time `json:"timestamp"`
}

// SSE event types.
const (
	EventTypeProgress  = "progress"
	EventTypeCompleted = "completed"
	EventTypeError     = "error"
	EventTypeCancelled = "cancelled"
	EventTypeHeartbeat = "heartbeat"
)

// OperationFilter defines criteria for filtering progress updates.
type OperationFilter struct {
	// OperationType filters by operation type.
	OperationType *OperationType `json:"operation_type,omitempty"`
	// OwnerID filters by owner ID.
	OwnerID *models.ULID `json:"owner_id,omitempty"`
	// ResourceID filters by resource ID.
	ResourceID *models.ULID `json:"resource_id,omitempty"`
	// State filters by operation state.
	State *UniversalState `json:"state,omitempty"`
	// ActiveOnly filters to only active (non-terminal) operations.
	ActiveOnly bool `json:"active_only,omitempty"`
}

// Matches returns true if the progress matches the filter criteria.
func (f *OperationFilter) Matches(p *UniversalProgress) bool {
	if f == nil {
		return true
	}
	if f.OperationType != nil && *f.OperationType != p.OperationType {
		return false
	}
	if f.OwnerID != nil && *f.OwnerID != p.OwnerID {
		return false
	}
	if f.ResourceID != nil && (p.ResourceID == nil || *f.ResourceID != *p.ResourceID) {
		return false
	}
	if f.State != nil && *f.State != p.State {
		return false
	}
	if f.ActiveOnly && !p.State.IsActive() {
		return false
	}
	return true
}
