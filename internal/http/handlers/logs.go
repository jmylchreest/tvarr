package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/sse"

	"github.com/jmylchreest/tvarr/internal/service/logs"
)

// LogsHandler handles log streaming and statistics endpoints.
type LogsHandler struct {
	service           *logs.Service
	heartbeatInterval time.Duration
}

// NewLogsHandler creates a new logs handler.
func NewLogsHandler(service *logs.Service) *LogsHandler {
	return &LogsHandler{
		service:           service,
		heartbeatInterval: logs.HeartbeatInterval,
	}
}

// LogEntryResponse represents a log entry in API responses.
// Matches frontend LogEntry type.
type LogEntryResponse struct {
	ID        string         `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Module    string         `json:"module,omitempty"`
	Target    string         `json:"target,omitempty"`
	File      string         `json:"file,omitempty"`
	Line      int            `json:"line,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
	Context   map[string]any `json:"context,omitempty"`
}

// LogStatsResponse represents log statistics in API responses.
// Matches frontend LogStats type.
type LogStatsResponse struct {
	TotalLogs          int64              `json:"total_logs"`
	LogsByLevel        map[string]int64   `json:"logs_by_level"`
	LogsByModule       map[string]int64   `json:"logs_by_module"`
	RecentErrors       []LogEntryResponse `json:"recent_errors"`
	LogRatePerMinute   float64            `json:"log_rate_per_minute"`
	OldestLogTimestamp *time.Time         `json:"oldest_log_timestamp,omitempty"`
	NewestLogTimestamp *time.Time         `json:"newest_log_timestamp,omitempty"`
}

// SSE Event Type Wrapper - required by Huma for OpenAPI schema generation.
// LogLogEvent is sent for each log entry streamed via SSE.
type LogLogEvent LogEntryResponse

// SSELogsStreamInput defines query parameters for the logs SSE endpoint.
type SSELogsStreamInput struct {
	Level   string `query:"level" doc:"Filter by minimum log level (trace, debug, info, warn, error)"`
	Module  string `query:"module" doc:"Filter by module name"`
	Initial int    `query:"initial" default:"50" minimum:"0" maximum:"500" doc:"Number of recent logs to send on connect (0-500)"`
}

// LogEntryFromService converts a service log entry to a response.
func LogEntryFromService(entry logs.LogEntry) LogEntryResponse {
	return LogEntryResponse{
		ID:        entry.ID,
		Timestamp: entry.Timestamp,
		Level:     entry.Level,
		Message:   entry.Message,
		Module:    entry.Module,
		Target:    entry.Target,
		File:      entry.File,
		Line:      entry.Line,
		Fields:    entry.Fields,
		Context:   entry.Context,
	}
}

// LogStatsFromService converts service log stats to a response.
func LogStatsFromService(stats logs.LogStats) LogStatsResponse {
	resp := LogStatsResponse{
		TotalLogs:          stats.TotalLogs,
		LogsByLevel:        stats.LogsByLevel,
		LogsByModule:       stats.LogsByModule,
		RecentErrors:       make([]LogEntryResponse, len(stats.RecentErrors)),
		LogRatePerMinute:   stats.LogRatePerMinute,
		OldestLogTimestamp: stats.OldestLogTimestamp,
		NewestLogTimestamp: stats.NewestLogTimestamp,
	}
	for i, entry := range stats.RecentErrors {
		resp.RecentErrors[i] = LogEntryFromService(entry)
	}
	return resp
}

// GetLogStatsInput is the input for getting log statistics.
type GetLogStatsInput struct{}

// GetLogStatsBody is the response body for log statistics.
type GetLogStatsBody = LogStatsResponse

// GetLogStatsOutput is the output for getting log statistics.
type GetLogStatsOutput struct {
	Body GetLogStatsBody
}

// GetRecentLogsInput is the input for getting recent logs.
type GetRecentLogsInput struct {
	Limit int `query:"limit" default:"100" doc:"Maximum number of logs to return (1-1000)"`
}

// GetRecentLogsBody is the response body for recent logs.
type GetRecentLogsBody struct {
	Logs []LogEntryResponse `json:"logs"`
}

// GetRecentLogsOutput is the output for getting recent logs.
type GetRecentLogsOutput struct {
	Body GetRecentLogsBody
}

// Register registers the logs routes with the API.
func (h *LogsHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "getLogStats",
		Method:      "GET",
		Path:        "/api/v1/logs/stats",
		Summary:     "Get log statistics",
		Description: "Returns statistics about the log stream including counts by level and module",
		Tags:        []string{"Logs"},
	}, h.GetStats)

	huma.Register(api, huma.Operation{
		OperationID: "getRecentLogs",
		Method:      "GET",
		Path:        "/api/v1/logs/recent",
		Summary:     "Get recent logs",
		Description: "Returns the most recent log entries",
		Tags:        []string{"Logs"},
	}, h.GetRecentLogs)

	// Register SSE endpoint with Huma for OpenAPI documentation.
	// The actual handler is registered separately via RegisterSSE on the chi router,
	// which takes precedence. This registration provides OpenAPI schema generation.
	sse.Register(api, huma.Operation{
		OperationID: "logsStream",
		Method:      "GET",
		Path:        "/api/v1/logs/stream",
		Summary:     "Subscribe to log events",
		Description: `Server-Sent Events stream for real-time log entries.

## Connection Protocol
- On connect: receives ` + "`" + `:connected` + "`" + ` comment
- On connect with ` + "`" + `initial=N` + "`" + `: receives up to N recent log entries before live streaming
- Every 30s without events: receives ` + "`" + `:heartbeat <unix_epoch>` + "`" + ` comment (Unix timestamp in seconds)

## Event Type
- ` + "`" + `log` + "`" + `: A log entry

## Usage Example
` + "```" + `javascript
const eventSource = new EventSource('/api/v1/logs/stream?level=error&initial=100');
eventSource.addEventListener('log', (e) => console.log(JSON.parse(e.data)));
` + "```",
		Tags: []string{"Logs"},
	}, map[string]any{
		"log": LogLogEvent{},
	}, func(ctx context.Context, input *SSELogsStreamInput, send sse.Sender) {
		// This handler is a placeholder for OpenAPI schema generation.
		// The actual SSE handling is done by RegisterSSE on the chi router.
		<-ctx.Done()
	})
}

// RegisterSSE registers the SSE endpoint on a chi router.
// This is separate from Register because Huma doesn't support SSE streaming natively.
func (h *LogsHandler) RegisterSSE(router interface {
	Get(pattern string, handlerFn http.HandlerFunc)
}) {
	router.Get("/api/v1/logs/stream", h.handleSSEStream)
}

// GetStats returns current log statistics.
func (h *LogsHandler) GetStats(ctx context.Context, input *GetLogStatsInput) (*GetLogStatsOutput, error) {
	stats := h.service.GetStats()
	return &GetLogStatsOutput{
		Body: LogStatsFromService(stats),
	}, nil
}

// GetRecentLogs returns the most recent log entries.
func (h *LogsHandler) GetRecentLogs(ctx context.Context, input *GetRecentLogsInput) (*GetRecentLogsOutput, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	entries := h.service.GetRecentLogs(limit)

	output := &GetRecentLogsOutput{
		Body: GetRecentLogsBody{
			Logs: make([]LogEntryResponse, len(entries)),
		},
	}

	for i, entry := range entries {
		output.Body.Logs[i] = LogEntryFromService(entry)
	}

	return output, nil
}

// handleSSEStream is the raw HTTP handler for SSE streaming.
func (h *LogsHandler) handleSSEStream(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers for cross-origin requests (frontend on different port)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Cache-Control")
	w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Parse filter parameters
	levelFilter := r.URL.Query().Get("level")
	moduleFilter := r.URL.Query().Get("module")

	// Parse initial count (number of recent logs to send on connect)
	initialCount := 50 // default
	if countStr := r.URL.Query().Get("initial"); countStr != "" {
		if count, err := strconv.Atoi(countStr); err == nil && count >= 0 && count <= 500 {
			initialCount = count
		}
	}

	// Subscribe to events
	sub := h.service.Subscribe(r.Context())

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

	// Send initial batch of recent logs
	if initialCount > 0 {
		recentLogs := h.service.GetRecentLogs(initialCount)
		for _, entry := range recentLogs {
			if !h.matchesFilter(entry, levelFilter, moduleFilter) {
				continue
			}
			if _, err := h.writeSSEEvent(w, entry); err != nil {
				slog.Error("failed to write initial log event", "error", err)
				return
			}
		}
		if err := rc.Flush(); err != nil {
			slog.Error("failed to flush initial logs", "error", err)
			return
		}
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
		case entry, ok := <-sub.Events:
			if !ok {
				return
			}
			// Apply filters
			if !h.matchesFilter(*entry, levelFilter, moduleFilter) {
				continue
			}
			if _, err := h.writeSSEEvent(w, *entry); err != nil {
				slog.Error("failed to write SSE log event",
					"level", entry.Level,
					"error", err,
				)
				return
			}
			if err := rc.Flush(); err != nil {
				slog.Debug("event flush failed, client likely disconnected", "error", err)
				return
			}
		}
	}
}

// matchesFilter checks if a log entry matches the specified filters.
func (h *LogsHandler) matchesFilter(entry logs.LogEntry, level, module string) bool {
	if level != "" && entry.Level != level {
		return false
	}
	if module != "" && entry.Module != module {
		return false
	}
	return true
}

// writeSSEEvent writes a log entry in SSE format.
// Returns the number of bytes written and any error.
func (h *LogsHandler) writeSSEEvent(w http.ResponseWriter, entry logs.LogEntry) (int, error) {
	data, err := json.Marshal(LogEntryFromService(entry))
	if err != nil {
		n, _ := fmt.Fprintf(w, "event: log\ndata: {\"error\": \"marshal error\"}\n\n")
		return n, err
	}

	// Write the full SSE message in one write for better atomicity
	message := fmt.Sprintf("event: log\ndata: %s\n\n", data)
	messageBytes := []byte(message)

	// Write with short write detection
	n, err := w.Write(messageBytes)
	if err != nil {
		return n, err
	}
	if n < len(messageBytes) {
		slog.Error("SSE short write detected",
			"expected", len(messageBytes),
			"written", n,
		)
		return n, fmt.Errorf("short write: wrote %d of %d bytes", n, len(messageBytes))
	}
	return n, nil
}
