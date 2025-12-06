package relay

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmylchreest/tvarr/internal/ffmpeg"
	"github.com/jmylchreest/tvarr/internal/models"
)

// ErrSessionNotFound is returned when a relay session is not found.
var ErrSessionNotFound = errors.New("relay session not found")

// ErrSessionClosed is returned when trying to use a closed session.
var ErrSessionClosed = errors.New("relay session closed")

// ErrUpstreamFailed is returned when the upstream stream fails.
var ErrUpstreamFailed = errors.New("upstream stream failed")

// ManagerConfig holds configuration for the relay manager.
type ManagerConfig struct {
	// MaxSessions is the maximum number of concurrent relay sessions.
	MaxSessions int
	// SessionTimeout is how long a session can run without clients.
	SessionTimeout time.Duration
	// CleanupInterval is how often to clean up stale sessions.
	CleanupInterval time.Duration
	// CircuitBreakerConfig for upstream failure handling.
	CircuitBreakerConfig CircuitBreakerConfig
	// ConnectionPoolConfig for upstream connection limits.
	ConnectionPoolConfig ConnectionPoolConfig
	// CyclicBufferConfig for multi-client streaming.
	CyclicBufferConfig CyclicBufferConfig
	// FallbackConfig for fallback stream generation.
	FallbackConfig FallbackConfig
	// HTTPClient for upstream requests.
	HTTPClient *http.Client
}

// DefaultManagerConfig returns sensible defaults for the relay manager.
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		MaxSessions:          100,
		SessionTimeout:       5 * time.Minute,
		CleanupInterval:      30 * time.Second,
		CircuitBreakerConfig: DefaultCircuitBreakerConfig(),
		ConnectionPoolConfig: DefaultConnectionPoolConfig(),
		CyclicBufferConfig:   DefaultCyclicBufferConfig(),
		FallbackConfig:       DefaultFallbackConfig(),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Manager manages relay sessions and their lifecycles.
type Manager struct {
	config     ManagerConfig
	ffmpegBin  *ffmpeg.BinaryDetector
	classifier *StreamClassifier
	logger     *slog.Logger

	mu       sync.RWMutex
	sessions map[uuid.UUID]*RelaySession
	// channelSessions maps channel IDs to session IDs for reuse
	channelSessions map[uuid.UUID]uuid.UUID

	circuitBreakers   *CircuitBreakerRegistry
	connectionPool    *ConnectionPool
	fallbackGenerator *FallbackGenerator

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewManager creates a new relay manager.
func NewManager(config ManagerConfig) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	logger := slog.Default().With(slog.String("component", "relay"))

	m := &Manager{
		config:            config,
		ffmpegBin:         ffmpeg.NewBinaryDetector(),
		classifier:        NewStreamClassifier(config.HTTPClient),
		logger:            logger,
		sessions:          make(map[uuid.UUID]*RelaySession),
		channelSessions:   make(map[uuid.UUID]uuid.UUID),
		circuitBreakers:   NewCircuitBreakerRegistry(config.CircuitBreakerConfig),
		connectionPool:    NewConnectionPool(config.ConnectionPoolConfig),
		fallbackGenerator: NewFallbackGenerator(config.FallbackConfig, logger),
		ctx:               ctx,
		cancel:            cancel,
	}

	// Start cleanup goroutine
	m.wg.Add(1)
	go m.cleanupLoop()

	return m
}

// InitializeFallback pre-generates the fallback slate. Should be called at startup.
func (m *Manager) InitializeFallback(ctx context.Context) error {
	return m.fallbackGenerator.Initialize(ctx)
}

// FallbackGenerator returns the manager's fallback generator.
func (m *Manager) FallbackGenerator() *FallbackGenerator {
	return m.fallbackGenerator
}

// GetOrCreateSession gets an existing session for the channel or creates a new one.
func (m *Manager) GetOrCreateSession(ctx context.Context, channelID uuid.UUID, streamURL string, profile *models.RelayProfile) (*RelaySession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if session exists for this channel
	if sessionID, ok := m.channelSessions[channelID]; ok {
		if session, ok := m.sessions[sessionID]; ok {
			session.mu.RLock()
			closed := session.closed
			session.mu.RUnlock()
			if !closed {
				return session, nil
			}
			// Session is closed, remove from maps
			delete(m.sessions, sessionID)
			delete(m.channelSessions, channelID)
		}
	}

	// Check session limit
	if len(m.sessions) >= m.config.MaxSessions {
		return nil, fmt.Errorf("maximum sessions (%d) reached", m.config.MaxSessions)
	}

	// Create new session
	session, err := m.createSession(ctx, channelID, streamURL, profile)
	if err != nil {
		return nil, err
	}

	m.sessions[session.ID] = session
	m.channelSessions[channelID] = session.ID

	return session, nil
}

// GetSession returns a session by ID.
func (m *Manager) GetSession(sessionID uuid.UUID) (*RelaySession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[sessionID]
	return session, ok
}

// CloseSession closes a specific session.
func (m *Manager) CloseSession(sessionID uuid.UUID) error {
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return ErrSessionNotFound
	}
	delete(m.sessions, sessionID)
	delete(m.channelSessions, session.ChannelID)
	m.mu.Unlock()

	session.Close()
	return nil
}

// Close shuts down the manager and all sessions.
func (m *Manager) Close() {
	m.cancel()

	m.mu.Lock()
	for _, session := range m.sessions {
		session.Close()
	}
	m.sessions = nil
	m.channelSessions = nil
	m.mu.Unlock()

	m.connectionPool.Close()
	m.wg.Wait()
}

// Stats returns manager statistics.
func (m *Manager) Stats() ManagerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]SessionStats, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s.Stats())
	}

	return ManagerStats{
		ActiveSessions:  len(m.sessions),
		MaxSessions:     m.config.MaxSessions,
		Sessions:        sessions,
		ConnectionPool:  m.connectionPool.Stats(),
		CircuitBreakers: m.circuitBreakers.AllStats(),
	}
}

// HTTPClient returns the HTTP client used for upstream requests.
func (m *Manager) HTTPClient() *http.Client {
	if m.config.HTTPClient != nil {
		return m.config.HTTPClient
	}
	return http.DefaultClient
}

// createSession creates a new relay session.
func (m *Manager) createSession(ctx context.Context, channelID uuid.UUID, streamURL string, profile *models.RelayProfile) (*RelaySession, error) {
	// Check circuit breaker
	cb := m.circuitBreakers.Get(streamURL)
	if !cb.Allow() {
		return nil, fmt.Errorf("%w: circuit breaker open for %s", ErrUpstreamFailed, streamURL)
	}

	// Classify stream
	classification := m.classifier.Classify(ctx, streamURL)

	sessionCtx, sessionCancel := context.WithCancel(m.ctx)

	session := &RelaySession{
		ID:             uuid.New(),
		ChannelID:      channelID,
		StreamURL:      streamURL,
		Profile:        profile,
		Classification: classification,
		StartedAt:      time.Now(),
		LastActivity:   time.Now(),
		manager:        m,
		buffer:         NewCyclicBuffer(m.config.CyclicBufferConfig),
		ctx:            sessionCtx,
		cancel:         sessionCancel,
	}

	// Initialize fallback controller if enabled in profile
	if profile != nil && profile.FallbackEnabled && m.fallbackGenerator.IsReady() {
		session.fallbackGenerator = m.fallbackGenerator
		session.fallbackController = NewFallbackController(
			m.fallbackGenerator,
			profile.FallbackErrorThreshold,
			time.Duration(profile.FallbackRecoveryInterval)*time.Second,
			m.logger.With(slog.String("session_id", session.ID.String())),
		)
	}

	// Start the appropriate relay pipeline
	if err := session.start(ctx); err != nil {
		session.Close()
		cb.RecordFailure()
		return nil, err
	}

	cb.RecordSuccess()
	return session, nil
}

// cleanupLoop periodically cleans up stale sessions.
func (m *Manager) cleanupLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.cleanupStaleSessions()
		}
	}
}

// cleanupStaleSessions removes sessions without clients.
func (m *Manager) cleanupStaleSessions() {
	m.mu.Lock()
	defer m.mu.Unlock()

	var toRemove []uuid.UUID

	for id, session := range m.sessions {
		session.mu.RLock()
		clientCount := session.buffer.ClientCount()
		lastActivity := session.LastActivity
		closed := session.closed
		session.mu.RUnlock()

		if closed || (clientCount == 0 && time.Since(lastActivity) > m.config.SessionTimeout) {
			toRemove = append(toRemove, id)
		}
	}

	for _, id := range toRemove {
		if session, ok := m.sessions[id]; ok {
			delete(m.sessions, id)
			delete(m.channelSessions, session.ChannelID)
			go session.Close()
		}
	}
}

// ManagerStats holds manager statistics.
type ManagerStats struct {
	ActiveSessions  int                     `json:"active_sessions"`
	MaxSessions     int                     `json:"max_sessions"`
	Sessions        []SessionStats          `json:"sessions,omitempty"`
	ConnectionPool  ConnectionPoolStats     `json:"connection_pool"`
	CircuitBreakers map[string]CircuitStats `json:"circuit_breakers,omitempty"`
}
