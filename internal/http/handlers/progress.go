package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/service/progress"
)

// ProgressHandler handles progress tracking and SSE endpoints.
type ProgressHandler struct {
	service           *progress.Service
	heartbeatInterval time.Duration
}

// NewProgressHandler creates a new progress handler.
func NewProgressHandler(service *progress.Service) *ProgressHandler {
	return &ProgressHandler{
		service:           service,
		heartbeatInterval: 30 * time.Second,
	}
}

// SetHeartbeatInterval sets the SSE heartbeat interval (for testing).
func (h *ProgressHandler) SetHeartbeatInterval(interval time.Duration) {
	h.heartbeatInterval = interval
}

// ProgressResponse represents a progress operation in API responses.
// Field names match frontend ProgressEvent type.
type ProgressResponse struct {
	ID                string            `json:"id"`
	OperationName     string            `json:"operation_name"`
	OperationType     string            `json:"operation_type"`
	OwnerID           string            `json:"owner_id"`
	OwnerType         string            `json:"owner_type"`
	State             string            `json:"state"`
	OverallPercentage float64           `json:"overall_percentage"`
	Error             string            `json:"error,omitempty"`
	Stages            []StageResponse   `json:"stages,omitempty"`
	CurrentStage      string            `json:"current_stage"`
	StartedAt         time.Time         `json:"started_at"`
	LastUpdate        time.Time         `json:"last_update"`
	CompletedAt       *time.Time        `json:"completed_at,omitempty"`
	Metadata          map[string]any    `json:"metadata,omitempty"`
}

// StageResponse represents a stage in API responses.
// Field names match frontend ProgressStage type.
type StageResponse struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	State      string  `json:"state"`
	Percentage float64 `json:"percentage"`
	StageStep  string  `json:"stage_step,omitempty"`
}

// ProgressFromService converts a service progress to a response.
func ProgressFromService(p *progress.UniversalProgress) ProgressResponse {
	// Determine the current stage ID from the index
	currentStage := ""
	if p.CurrentStageIndex >= 0 && p.CurrentStageIndex < len(p.Stages) {
		currentStage = p.Stages[p.CurrentStageIndex].ID
	}

	// Generate an operation name from metadata or use a default
	operationName := p.Message
	if operationName == "" {
		operationName = string(p.OperationType)
	}

	resp := ProgressResponse{
		ID:                p.OperationID,
		OperationName:     operationName,
		OperationType:     string(p.OperationType),
		OwnerID:           p.OwnerID.String(),
		OwnerType:         p.OwnerType,
		State:             string(p.State),
		OverallPercentage: p.Progress * 100, // Convert 0-1 to 0-100
		Error:             p.Error,
		CurrentStage:      currentStage,
		StartedAt:         p.StartedAt,
		LastUpdate:        p.UpdatedAt,
		CompletedAt:       p.CompletedAt,
		Metadata:          p.Metadata,
	}
	for _, s := range p.Stages {
		resp.Stages = append(resp.Stages, StageResponse{
			ID:         s.ID,
			Name:       s.Name,
			State:      string(s.State),
			Percentage: s.Progress * 100, // Convert 0-1 to 0-100
			StageStep:  s.Message,
		})
	}
	return resp
}

// ListOperationsInput is the input for listing operations.
type ListOperationsInput struct {
	OperationType string `query:"operation_type" doc:"Filter by operation type"`
	OwnerID       string `query:"owner_id" doc:"Filter by owner ID"`
	ResourceID    string `query:"resource_id" doc:"Filter by resource ID"`
	State         string `query:"state" doc:"Filter by state"`
	ActiveOnly    bool   `query:"active_only" doc:"Only return active operations"`
}

// ListOperationsBody is the response body for listing operations.
type ListOperationsBody struct {
	Operations []ProgressResponse `json:"operations"`
}

// ListOperationsOutput is the output for listing operations.
type ListOperationsOutput struct {
	Body ListOperationsBody
}

// GetOperationInput is the input for getting a single operation.
type GetOperationInput struct {
	OperationID string `path:"operation_id" doc:"Operation ID"`
}

// GetOperationBody is the response body for getting a single operation.
type GetOperationBody = ProgressResponse

// GetOperationOutput is the output for getting a single operation.
type GetOperationOutput struct {
	Body GetOperationBody
}

// SSEEventsInput is the input for the SSE events endpoint.
// Note: state and active_only filters are NOT supported for SSE to ensure
// terminal events are always delivered. Use the REST API for filtered queries.
type SSEEventsInput struct {
	OperationType string `query:"operation_type" doc:"Filter events by operation type"`
	OwnerID       string `query:"owner_id" doc:"Filter events by owner ID"`
	ResourceID    string `query:"resource_id" doc:"Filter events by resource ID"`
}

// Register registers the progress routes with the API.
func (h *ProgressHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listOperations",
		Method:      "GET",
		Path:        "/api/v1/progress/operations",
		Summary:     "List operations",
		Description: "Returns a list of current and recent progress operations",
		Tags:        []string{"Progress"},
	}, h.ListOperations)

	huma.Register(api, huma.Operation{
		OperationID: "getOperation",
		Method:      "GET",
		Path:        "/api/v1/progress/operations/{operation_id}",
		Summary:     "Get operation",
		Description: "Returns details for a specific progress operation",
		Tags:        []string{"Progress"},
	}, h.GetOperation)
}

// RegisterSSE registers the SSE endpoint on a chi router.
// This is separate from Register because Huma doesn't support SSE streaming natively.
func (h *ProgressHandler) RegisterSSE(router interface {
	Get(pattern string, handlerFn http.HandlerFunc)
}) {
	router.Get("/api/v1/progress/events", h.handleSSEEvents)
}

// HandleSSEEvents is the raw HTTP handler for SSE streaming.
// Exported for direct use with custom routers.
func (h *ProgressHandler) HandleSSEEvents(w http.ResponseWriter, r *http.Request) {
	h.handleSSEEvents(w, r)
}

// ListOperations returns a list of current progress operations.
func (h *ProgressHandler) ListOperations(ctx context.Context, input *ListOperationsInput) (*ListOperationsOutput, error) {
	filter := &progress.OperationFilter{
		ActiveOnly: input.ActiveOnly,
	}

	if input.OperationType != "" {
		opType := progress.OperationType(input.OperationType)
		filter.OperationType = &opType
	}

	if input.OwnerID != "" {
		ownerID, err := models.ParseULID(input.OwnerID)
		if err == nil {
			filter.OwnerID = &ownerID
		}
	}

	if input.ResourceID != "" {
		resourceID, err := models.ParseULID(input.ResourceID)
		if err == nil {
			filter.ResourceID = &resourceID
		}
	}

	if input.State != "" {
		state := progress.UniversalState(input.State)
		filter.State = &state
	}

	operations := h.service.ListOperations(filter)

	output := &ListOperationsOutput{
		Body: ListOperationsBody{
			Operations: make([]ProgressResponse, 0, len(operations)),
		},
	}

	for _, op := range operations {
		output.Body.Operations = append(output.Body.Operations, ProgressFromService(op))
	}

	return output, nil
}

// GetOperation returns details for a specific operation.
func (h *ProgressHandler) GetOperation(ctx context.Context, input *GetOperationInput) (*GetOperationOutput, error) {
	op, err := h.service.GetOperation(input.OperationID)
	if err != nil {
		return nil, huma.Error404NotFound("operation not found")
	}

	return &GetOperationOutput{
		Body: ProgressFromService(op),
	}, nil
}

// handleSSEEvents is the raw HTTP handler for SSE streaming.
func (h *ProgressHandler) handleSSEEvents(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers for cross-origin requests (frontend on different port)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Cache-Control")
	w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Parse filter from query params
	filter := h.parseSSEFilter(r)

	// Subscribe to events
	sub := h.service.Subscribe(filter)
	defer h.service.Unsubscribe(sub.ID)

	// Use ResponseController for reliable flushing with error handling (Go 1.20+)
	rc := http.NewResponseController(w)

	// Heartbeat ticker
	heartbeat := time.NewTicker(h.heartbeatInterval)
	defer heartbeat.Stop()

	ctx := r.Context()

	// Send initial comment to establish connection and trigger onopen in browser
	fmt.Fprintf(w, ":connected\n\n")
	if err := rc.Flush(); err != nil {
		slog.Error("failed to flush initial SSE connection", "error", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			// Send heartbeat comment
			fmt.Fprintf(w, ":heartbeat %d\n\n", time.Now().Unix())
			if err := rc.Flush(); err != nil {
				slog.Debug("heartbeat flush failed, client likely disconnected", "error", err)
				return
			}
		case event, ok := <-sub.Events:
			if !ok {
				return
			}
			_, err := h.writeSSEEvent(w, event)
			if err != nil {
				slog.Error("failed to write SSE event",
					"event_type", event.EventType,
					"operation_id", event.Progress.OperationID,
					"error", err,
				)
				return
			}
			if err := rc.Flush(); err != nil {
				slog.Debug("event flush failed, client likely disconnected",
					"event_type", event.EventType,
					"error", err,
				)
				return
			}
		}
	}
}

// parseSSEFilter parses filter parameters from the request.
// Note: SSE subscriptions do NOT support ActiveOnly or State filters to ensure
// terminal events (completed, error, cancelled) are always delivered to clients.
// Clients should filter events locally if needed.
func (h *ProgressHandler) parseSSEFilter(r *http.Request) *progress.OperationFilter {
	query := r.URL.Query()
	filter := &progress.OperationFilter{}

	if opType := query.Get("operation_type"); opType != "" {
		t := progress.OperationType(opType)
		filter.OperationType = &t
	}

	if ownerID := query.Get("owner_id"); ownerID != "" {
		if id, err := models.ParseULID(ownerID); err == nil {
			filter.OwnerID = &id
		}
	}

	if resourceID := query.Get("resource_id"); resourceID != "" {
		if id, err := models.ParseULID(resourceID); err == nil {
			filter.ResourceID = &id
		}
	}

	// Note: state and active_only filters are intentionally NOT supported for SSE
	// to ensure terminal events are always delivered. Use the REST API to query
	// operations with these filters.

	return filter
}

// writeSSEEvent writes a progress event in SSE format.
// Returns the number of bytes written and any error.
func (h *ProgressHandler) writeSSEEvent(w http.ResponseWriter, event *progress.ProgressEvent) (int, error) {
	data, err := json.Marshal(ProgressFromService(event.Progress))
	if err != nil {
		n, _ := fmt.Fprintf(w, "event: %s\ndata: {\"error\": \"marshal error\"}\n\n", event.EventType)
		return n, err
	}

	// Write the full SSE message in one write for better atomicity
	message := fmt.Sprintf("event: %s\ndata: %s\n\n", event.EventType, data)
	messageBytes := []byte(message)

	// Log terminal events being written to the response
	if event.EventType == progress.EventTypeCompleted ||
		event.EventType == progress.EventTypeError ||
		event.EventType == progress.EventTypeCancelled {
		slog.Debug("writing terminal SSE event to response",
			"event_type", event.EventType,
			"operation_id", event.Progress.OperationID,
			"state", event.Progress.State,
		)
	}

	// Write with short write detection
	n, err := w.Write(messageBytes)
	if err != nil {
		return n, err
	}
	if n < len(messageBytes) {
		slog.Error("SSE short write detected",
			"expected", len(messageBytes),
			"written", n,
			"event_type", event.EventType,
			"operation_id", event.Progress.OperationID,
		)
		return n, fmt.Errorf("short write: wrote %d of %d bytes", n, len(messageBytes))
	}
	return n, nil
}
