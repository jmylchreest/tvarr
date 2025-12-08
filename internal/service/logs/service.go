// Package logs provides log streaming and statistics services for tvarr.
package logs

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

const (
	// DefaultMaxLogs is the maximum number of logs to retain in memory.
	DefaultMaxLogs = 1000
	// DefaultBufferSize is the subscriber event buffer size.
	DefaultBufferSize = 100
	// HeartbeatInterval is the interval for sending heartbeats to subscribers.
	HeartbeatInterval = 30 * time.Second
)

// LogEntry represents a single log entry for streaming to clients.
type LogEntry struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Module    string                 `json:"module,omitempty"`
	Target    string                 `json:"target,omitempty"`
	File      string                 `json:"file,omitempty"`
	Line      int                    `json:"line,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Context   map[string]interface{} `json:"context,omitempty"`
}

// LogStats provides statistics about the log stream.
type LogStats struct {
	TotalLogs          int64            `json:"total_logs"`
	LogsByLevel        map[string]int64 `json:"logs_by_level"`
	LogsByModule       map[string]int64 `json:"logs_by_module"`
	RecentErrors       []LogEntry       `json:"recent_errors"`
	LogRatePerMinute   float64          `json:"log_rate_per_minute"`
	OldestLogTimestamp *time.Time       `json:"oldest_log_timestamp,omitempty"`
	NewestLogTimestamp *time.Time       `json:"newest_log_timestamp,omitempty"`
}

// Subscriber represents a client subscribed to log events.
type Subscriber struct {
	ID     string
	Events chan *LogEntry
	Done   chan struct{}
}

// Service manages log streaming and statistics.
type Service struct {
	mu          sync.RWMutex
	logs        []LogEntry
	maxLogs     int
	subscribers map[string]*Subscriber
	totalLogs   int64
	stats       struct {
		byLevel  map[string]int64
		byModule map[string]int64
	}
	recentErrors   []LogEntry
	maxErrors      int
	startTime      time.Time
	wrappedHandler slog.Handler
}

// New creates a new logs service.
func New() *Service {
	s := &Service{
		logs:        make([]LogEntry, 0, DefaultMaxLogs),
		maxLogs:     DefaultMaxLogs,
		subscribers: make(map[string]*Subscriber),
		maxErrors:   10,
		startTime:   time.Now(),
	}
	s.stats.byLevel = make(map[string]int64)
	s.stats.byModule = make(map[string]int64)
	return s
}

// WrapHandler wraps an existing slog.Handler to intercept logs.
// The wrapped handler will still write to its destination, but logs
// will also be captured by this service for streaming.
func (s *Service) WrapHandler(handler slog.Handler) slog.Handler {
	s.wrappedHandler = handler
	return &logsHandler{
		service: s,
		wrapped: handler,
	}
}

// AddLog adds a log entry and broadcasts it to subscribers.
func (s *Service) AddLog(entry LogEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate ID if not set
	if entry.ID == "" {
		entry.ID = ulid.Make().String()
	}

	// Update statistics
	s.totalLogs++
	s.stats.byLevel[entry.Level]++
	if entry.Module != "" {
		s.stats.byModule[entry.Module]++
	}

	// Track recent errors
	if entry.Level == "error" {
		s.recentErrors = append(s.recentErrors, entry)
		if len(s.recentErrors) > s.maxErrors {
			s.recentErrors = s.recentErrors[1:]
		}
	}

	// Add to circular buffer
	if len(s.logs) >= s.maxLogs {
		s.logs = s.logs[1:]
	}
	s.logs = append(s.logs, entry)

	// Broadcast to subscribers (non-blocking)
	for _, sub := range s.subscribers {
		select {
		case sub.Events <- &entry:
		default:
			// Subscriber buffer full, skip
		}
	}
}

// Subscribe creates a new subscriber for log events.
func (s *Service) Subscribe(ctx context.Context) *Subscriber {
	s.mu.Lock()
	defer s.mu.Unlock()

	sub := &Subscriber{
		ID:     ulid.Make().String(),
		Events: make(chan *LogEntry, DefaultBufferSize),
		Done:   make(chan struct{}),
	}
	s.subscribers[sub.ID] = sub

	// Start cleanup goroutine
	go func() {
		select {
		case <-ctx.Done():
		case <-sub.Done:
		}
		s.Unsubscribe(sub.ID)
	}()

	return sub
}

// Unsubscribe removes a subscriber.
func (s *Service) Unsubscribe(subscriberID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sub, ok := s.subscribers[subscriberID]; ok {
		close(sub.Events)
		delete(s.subscribers, subscriberID)
	}
}

// GetStats returns current log statistics.
func (s *Service) GetStats() LogStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := LogStats{
		TotalLogs:    s.totalLogs,
		LogsByLevel:  make(map[string]int64),
		LogsByModule: make(map[string]int64),
		RecentErrors: make([]LogEntry, len(s.recentErrors)),
	}

	// Copy level stats
	for level, count := range s.stats.byLevel {
		stats.LogsByLevel[level] = count
	}

	// Ensure all levels exist
	for _, level := range []string{"trace", "debug", "info", "warn", "error"} {
		if _, ok := stats.LogsByLevel[level]; !ok {
			stats.LogsByLevel[level] = 0
		}
	}

	// Copy module stats
	for module, count := range s.stats.byModule {
		stats.LogsByModule[module] = count
	}

	// Copy recent errors
	copy(stats.RecentErrors, s.recentErrors)

	// Calculate log rate
	elapsed := time.Since(s.startTime).Minutes()
	if elapsed > 0 {
		stats.LogRatePerMinute = float64(s.totalLogs) / elapsed
	}

	// Set timestamps
	if len(s.logs) > 0 {
		oldest := s.logs[0].Timestamp
		newest := s.logs[len(s.logs)-1].Timestamp
		stats.OldestLogTimestamp = &oldest
		stats.NewestLogTimestamp = &newest
	}

	return stats
}

// GetRecentLogs returns the most recent logs up to the specified limit.
func (s *Service) GetRecentLogs(limit int) []LogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.logs) {
		limit = len(s.logs)
	}

	// Return most recent logs
	start := len(s.logs) - limit
	if start < 0 {
		start = 0
	}

	result := make([]LogEntry, limit)
	copy(result, s.logs[start:])
	return result
}

// SubscriberCount returns the number of active subscribers.
func (s *Service) SubscriberCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.subscribers)
}

// logsHandler is a slog.Handler that intercepts logs and sends them to the service.
type logsHandler struct {
	service *Service
	wrapped slog.Handler
	attrs   []slog.Attr
	groups  []string
}

// Enabled reports whether the handler handles records at the given level.
func (h *logsHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.wrapped.Enabled(ctx, level)
}

// Handle handles the Record.
func (h *logsHandler) Handle(ctx context.Context, r slog.Record) error {
	// Convert slog record to LogEntry
	entry := LogEntry{
		ID:        ulid.Make().String(),
		Timestamp: r.Time,
		Level:     levelToString(r.Level),
		Message:   r.Message,
		Fields:    make(map[string]interface{}),
	}

	// Add pre-set attributes
	for _, attr := range h.attrs {
		h.addAttr(&entry, attr)
	}

	// Add record attributes
	r.Attrs(func(a slog.Attr) bool {
		h.addAttr(&entry, a)
		return true
	})

	// Send to service
	h.service.AddLog(entry)

	// Pass through to wrapped handler
	return h.wrapped.Handle(ctx, r)
}

// WithAttrs returns a new Handler with the given attributes.
func (h *logsHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &logsHandler{
		service: h.service,
		wrapped: h.wrapped.WithAttrs(attrs),
		attrs:   newAttrs,
		groups:  h.groups,
	}
}

// WithGroup returns a new Handler with the given group appended.
func (h *logsHandler) WithGroup(name string) slog.Handler {
	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name
	return &logsHandler{
		service: h.service,
		wrapped: h.wrapped.WithGroup(name),
		attrs:   h.attrs,
		groups:  newGroups,
	}
}

// addAttr adds an attribute to the log entry.
func (h *logsHandler) addAttr(entry *LogEntry, attr slog.Attr) {
	key := attr.Key
	value := attr.Value.Any()

	// Map known attributes to LogEntry fields
	switch key {
	case "component", "module":
		if s, ok := value.(string); ok {
			entry.Module = s
			entry.Target = s
		}
	case slog.SourceKey: // "source" - handle both string and *slog.Source
		if src, ok := value.(*slog.Source); ok {
			entry.File = src.File
			entry.Line = src.Line
		} else if s, ok := value.(string); ok {
			// Handle "source" as a module name
			entry.Module = s
			entry.Target = s
		}
	case "target", "logger":
		if s, ok := value.(string); ok {
			entry.Target = s
		}
	case "request_id":
		if entry.Context == nil {
			entry.Context = make(map[string]interface{})
		}
		entry.Context["request_id"] = value
	case "correlation_id":
		if entry.Context == nil {
			entry.Context = make(map[string]interface{})
		}
		entry.Context["correlation_id"] = value
	default:
		entry.Fields[key] = value
	}
}

// levelToString converts slog.Level to a string level name.
func levelToString(level slog.Level) string {
	switch {
	case level < slog.LevelDebug:
		return "trace"
	case level == slog.LevelDebug:
		return "debug"
	case level == slog.LevelInfo:
		return "info"
	case level == slog.LevelWarn:
		return "warn"
	case level >= slog.LevelError:
		return "error"
	default:
		return "info"
	}
}
