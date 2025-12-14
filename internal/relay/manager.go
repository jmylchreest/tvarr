package relay

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmylchreest/tvarr/internal/ffmpeg"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
)

// ErrSessionNotFound is returned when a relay session is not found.
var ErrSessionNotFound = errors.New("relay session not found")

// ErrSessionClosed is returned when trying to use a closed session.
var ErrSessionClosed = errors.New("relay session closed")

// ErrUpstreamFailed is returned when the upstream stream fails.
var ErrUpstreamFailed = errors.New("upstream stream failed")

// ErrBufferClosed is returned when the buffer has been closed.
var ErrBufferClosed = errors.New("buffer closed")

// ErrClientNotFound is returned when a client is not found in the buffer.
var ErrClientNotFound = errors.New("client not found")

// ManagerConfig holds configuration for the relay manager.
type ManagerConfig struct {
	// MaxSessions is the maximum number of concurrent relay sessions.
	MaxSessions int
	// SessionTimeout is how long a session can run without clients.
	SessionTimeout time.Duration
	// IdleGracePeriod is how long to wait after all clients disconnect before closing a session.
	// This is a shorter timeout than SessionTimeout, specifically for idle sessions.
	IdleGracePeriod time.Duration
	// CleanupInterval is how often to clean up stale sessions.
	CleanupInterval time.Duration
	// CircuitBreakerConfig for upstream failure handling.
	CircuitBreakerConfig CircuitBreakerConfig
	// ConnectionPoolConfig for upstream connection limits.
	ConnectionPoolConfig ConnectionPoolConfig
	// FallbackConfig for fallback stream generation.
	FallbackConfig FallbackConfig
	// HTTPClient for upstream requests.
	HTTPClient *http.Client
	// CodecRepo for caching stream codec information.
	CodecRepo repository.LastKnownCodecRepository
	// CodecCacheTTL is how long to cache codec information.
	CodecCacheTTL time.Duration
	// BufferConfig for elementary stream buffer settings.
	BufferConfig SharedESBufferConfig
}

// DefaultManagerConfig returns sensible defaults for the relay manager.
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		MaxSessions:          100,
		SessionTimeout:       5 * time.Minute,
		IdleGracePeriod:      5 * time.Second,
		CleanupInterval:      1 * time.Second, // Check frequently for idle sessions
		CircuitBreakerConfig: DefaultCircuitBreakerConfig(),
		ConnectionPoolConfig: DefaultConnectionPoolConfig(),
		FallbackConfig:       DefaultFallbackConfig(),
		// For streaming, we use a transport with connection timeouts but no overall
		// request timeout. The Timeout field on http.Client applies to the entire
		// request including reading the body, which would cut off long-running streams.
		HTTPClient: &http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second, // Connection timeout
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second, // Time to wait for response headers
				IdleConnTimeout:       90 * time.Second,
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   10,
			},
			// No Timeout - streaming connections run indefinitely
		},
		CodecCacheTTL: 24 * time.Hour, // Cache codec info for 24 hours
		BufferConfig:  DefaultSharedESBufferConfig(),
	}
}

// Manager manages relay sessions and their lifecycles.
type Manager struct {
	config     ManagerConfig
	ffmpegBin  *ffmpeg.BinaryDetector
	prober     *ffmpeg.Prober
	classifier *StreamClassifier
	logger     *slog.Logger

	mu       sync.RWMutex
	sessions map[uuid.UUID]*RelaySession
	// channelSessions maps channel IDs to session IDs for reuse
	channelSessions map[uuid.UUID]uuid.UUID

	circuitBreakers    *CircuitBreakerRegistry
	connectionPool     *ConnectionPool
	fallbackGenerator  *FallbackGenerator
	passthroughTracker *PassthroughTracker

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewManager creates a new relay manager.
func NewManager(config ManagerConfig) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	logger := slog.Default().With(slog.String("component", "relay"))

	ffmpegBin := ffmpeg.NewBinaryDetector()

	// Initialize prober with detected ffprobe path
	var prober *ffmpeg.Prober
	if binInfo, err := ffmpegBin.Detect(ctx); err == nil && binInfo.FFprobePath != "" {
		prober = ffmpeg.NewProber(binInfo.FFprobePath).WithTimeout(10 * time.Second)
	}

	m := &Manager{
		config:             config,
		ffmpegBin:          ffmpegBin,
		prober:             prober,
		classifier:         NewStreamClassifier(config.HTTPClient),
		logger:             logger,
		sessions:           make(map[uuid.UUID]*RelaySession),
		channelSessions:    make(map[uuid.UUID]uuid.UUID),
		circuitBreakers:    NewCircuitBreakerRegistry(config.CircuitBreakerConfig),
		connectionPool:     NewConnectionPool(config.ConnectionPoolConfig),
		fallbackGenerator:  NewFallbackGenerator(config.FallbackConfig, logger),
		passthroughTracker: NewPassthroughTracker(),
		ctx:                ctx,
		cancel:             cancel,
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

// PassthroughTracker returns the manager's passthrough connection tracker.
func (m *Manager) PassthroughTracker() *PassthroughTracker {
	return m.passthroughTracker
}

// GetOrCreateSession gets an existing session for the channel or creates a new one.
//
// This function is carefully designed to avoid holding the manager lock during slow
// operations (stream classification, codec probing) to prevent blocking API requests
// like /api/v1/relay/sessions while a new session is being created.
func (m *Manager) GetOrCreateSession(ctx context.Context, channelID uuid.UUID, channelName string, streamSourceName string, streamURL string, profile *models.EncodingProfile) (*RelaySession, error) {
	// First, check if session already exists (fast path with read lock)
	m.mu.RLock()
	if sessionID, ok := m.channelSessions[channelID]; ok {
		if session, ok := m.sessions[sessionID]; ok {
			if !session.closed.Load() {
				m.mu.RUnlock()
				return session, nil
			}
		}
	}
	m.mu.RUnlock()

	// Check session limit before doing slow operations
	m.mu.RLock()
	atLimit := len(m.sessions) >= m.config.MaxSessions
	maxSessions := m.config.MaxSessions
	m.mu.RUnlock()

	if atLimit {
		return nil, fmt.Errorf("maximum sessions (%d) reached", maxSessions)
	}

	// Perform slow operations (classify, probe) WITHOUT holding the manager lock
	// This prevents blocking Stats() and other operations during session creation
	session, err := m.createSession(ctx, channelID, channelName, streamSourceName, streamURL, profile)
	if err != nil {
		return nil, err
	}

	// Now acquire the write lock to register the session
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check: another goroutine might have created a session while we were classifying/probing
	if existingSessionID, ok := m.channelSessions[channelID]; ok {
		if existingSession, ok := m.sessions[existingSessionID]; ok {
			if !existingSession.closed.Load() {
				// Another goroutine won the race - close our session and return the existing one
				session.Close()
				return existingSession, nil
			}
			// Existing session is closed, remove it
			delete(m.sessions, existingSessionID)
			delete(m.channelSessions, channelID)
		}
	}

	// Re-check session limit (might have changed while we were creating)
	if len(m.sessions) >= m.config.MaxSessions {
		session.Close()
		return nil, fmt.Errorf("maximum sessions (%d) reached", m.config.MaxSessions)
	}

	// Register the new session
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

// GetSessionForChannel returns an existing session for the channel if one exists.
// Unlike GetOrCreateSession, this does not create a new session if none exists.
// Returns nil if no active session exists for the channel.
func (m *Manager) GetSessionForChannel(channelID uuid.UUID) *RelaySession {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessionID, ok := m.channelSessions[channelID]
	if !ok {
		return nil
	}

	session, ok := m.sessions[sessionID]
	if !ok {
		return nil
	}

	if session.closed.Load() {
		return nil
	}

	return session
}

// HasSessionForChannel checks if an active session exists for the given channel.
func (m *Manager) HasSessionForChannel(channelID uuid.UUID) bool {
	return m.GetSessionForChannel(channelID) != nil
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
// This method minimizes lock contention by copying the session list first,
// then releasing the manager lock before collecting stats from individual sessions.
func (m *Manager) Stats() ManagerStats {
	// Copy session pointers while holding the lock briefly
	m.mu.RLock()
	sessionCount := len(m.sessions)
	maxSessions := m.config.MaxSessions
	sessionList := make([]*RelaySession, 0, sessionCount)
	for _, s := range m.sessions {
		sessionList = append(sessionList, s)
	}
	m.mu.RUnlock()

	// Collect stats from each session WITHOUT holding the manager lock
	// This prevents blocking other operations like GetOrCreateSession or cleanup
	sessions := make([]SessionStats, 0, len(sessionList))
	for _, s := range sessionList {
		sessions = append(sessions, s.Stats())
	}

	return ManagerStats{
		ActiveSessions:  sessionCount,
		MaxSessions:     maxSessions,
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

// ProbeAndStoreCodecInfo always probes the stream fresh and stores the result.
// The stored result is used by the channel UI to display codec information.
// Returns nil if probing is not available or fails (non-fatal).
func (m *Manager) ProbeAndStoreCodecInfo(ctx context.Context, streamURL string) *models.LastKnownCodec {
	// If no prober available, skip
	if m.prober == nil {
		m.logger.Debug("Skipping probe - prober not available (ffprobe not detected)",
			slog.String("stream_url", streamURL))
		return nil
	}

	// Always probe fresh - no cache lookup
	// Probe the stream using QuickProbe for fast detection
	start := time.Now()
	streamInfo, err := m.prober.QuickProbe(ctx, streamURL)
	probeMs := time.Since(start).Milliseconds()

	if err != nil {
		m.logger.Debug("Failed to probe stream codec",
			slog.String("stream_url", streamURL),
			slog.String("error", err.Error()),
			slog.Int64("probe_ms", probeMs))

		// Store the error in cache to avoid repeated failed probes
		if m.config.CodecRepo != nil {
			errorCodec := &models.LastKnownCodec{
				StreamURL:  streamURL,
				ProbedAt:   models.Now(),
				ProbeError: err.Error(),
				ProbeMs:    probeMs,
			}
			// Set short expiry for errors (retry in 5 minutes)
			errorCodec.SetExpiry(5 * time.Minute)
			if upsertErr := m.config.CodecRepo.Upsert(context.Background(), errorCodec); upsertErr != nil {
				m.logger.Debug("Failed to cache probe error", slog.String("error", upsertErr.Error()))
			}
		}
		return nil
	}

	// Create codec info from probe result
	codecInfo := &models.LastKnownCodec{
		StreamURL:       streamURL,
		VideoCodec:      streamInfo.VideoCodec,
		VideoProfile:    streamInfo.VideoProfile,
		VideoLevel:      streamInfo.VideoLevel,
		VideoWidth:      streamInfo.VideoWidth,
		VideoHeight:     streamInfo.VideoHeight,
		VideoFramerate:  streamInfo.VideoFramerate,
		VideoBitrate:    streamInfo.VideoBitrate,
		VideoPixFmt:     streamInfo.VideoPixFmt,
		AudioCodec:      streamInfo.AudioCodec,
		AudioSampleRate: streamInfo.AudioSampleRate,
		AudioChannels:   streamInfo.AudioChannels,
		AudioBitrate:    streamInfo.AudioBitrate,
		ContainerFormat: streamInfo.ContainerFormat,
		Duration:        streamInfo.Duration,
		IsLiveStream:    streamInfo.IsLiveStream,
		HasSubtitles:    streamInfo.HasSubtitles,
		StreamCount:     streamInfo.StreamCount,
		Title:           streamInfo.Title,
		ProbedAt:        models.Now(),
		ProbeMs:         probeMs,
	}

	// Set cache expiry
	codecInfo.SetExpiry(m.config.CodecCacheTTL)

	m.logger.Info("Probed stream codec",
		slog.String("stream_url", streamURL),
		slog.String("video_codec", codecInfo.VideoCodec),
		slog.String("audio_codec", codecInfo.AudioCodec),
		slog.String("resolution", fmt.Sprintf("%dx%d", codecInfo.VideoWidth, codecInfo.VideoHeight)),
		slog.Int64("probe_ms", probeMs))

	// Cache in database
	if m.config.CodecRepo != nil {
		if upsertErr := m.config.CodecRepo.Upsert(context.Background(), codecInfo); upsertErr != nil {
			m.logger.Debug("Failed to cache codec info", slog.String("error", upsertErr.Error()))
		}
	}

	return codecInfo
}

// GetOrProbeCodecInfo intelligently retrieves codec information using this priority:
// 1. If there's an active session for the channel, use its CachedCodecInfo (no network call)
// 2. If no active session but stream URL is already being streamed, use cached database info
// 3. If connection pool has capacity, probe fresh and store result
// 4. Otherwise, return cached database info (may be stale or nil)
//
// This prevents probing from consuming extra connections when a stream is already active.
func (m *Manager) GetOrProbeCodecInfo(ctx context.Context, channelID uuid.UUID, streamURL string) *models.LastKnownCodec {
	m.logger.Debug("GetOrProbeCodecInfo called",
		slog.String("channel_id", channelID.String()),
		slog.String("stream_url", streamURL))

	// Priority 1: Check for active session - this is the fastest path and doesn't require a network call
	session := m.GetSessionForChannel(channelID)
	if session != nil {
		if session.CachedCodecInfo != nil {
			m.logger.Debug("Using codec info from active session",
				slog.String("channel_id", channelID.String()),
				slog.String("video_codec", session.CachedCodecInfo.VideoCodec),
				slog.String("audio_codec", session.CachedCodecInfo.AudioCodec))
			return session.CachedCodecInfo
		}
		m.logger.Debug("Active session exists but has no cached codec info",
			slog.String("channel_id", channelID.String()))
	} else {
		m.logger.Debug("No active session for channel",
			slog.String("channel_id", channelID.String()))
	}

	// Priority 2: Check if connection pool would allow a probe
	// Extract host to check current connection count
	host, err := extractHost(streamURL)
	if err != nil {
		m.logger.Debug("Failed to extract host from stream URL",
			slog.String("stream_url", streamURL),
			slog.String("error", err.Error()))
		// Fall back to cached database info
		cached := m.getCachedCodecInfo(ctx, streamURL)
		m.logger.Debug("Returning cached codec info after host extraction failure",
			slog.Bool("cached_found", cached != nil))
		return cached
	}

	// Check connection pool stats - if we're at or near the limit, don't probe
	poolStats := m.connectionPool.Stats()
	currentHostConns := poolStats.HostConnections[host]
	atConnectionLimit := currentHostConns >= poolStats.MaxPerHost

	if atConnectionLimit {
		m.logger.Debug("Skipping probe - connection limit reached for host",
			slog.String("host", host),
			slog.Int("current_connections", currentHostConns),
			slog.Int("max_per_host", poolStats.MaxPerHost))
		// Fall back to cached database info
		cached := m.getCachedCodecInfo(ctx, streamURL)
		m.logger.Debug("Returning cached codec info due to connection limit",
			slog.Bool("cached_found", cached != nil))
		return cached
	}

	// Priority 3: We have capacity - probe fresh
	m.logger.Debug("Probing stream for codec info",
		slog.String("channel_id", channelID.String()),
		slog.String("stream_url", streamURL),
		slog.String("host", host),
		slog.Int("current_host_connections", currentHostConns),
		slog.Int("max_per_host", poolStats.MaxPerHost),
		slog.Bool("prober_available", m.prober != nil))

	result := m.ProbeAndStoreCodecInfo(ctx, streamURL)
	if result == nil {
		m.logger.Debug("ProbeAndStoreCodecInfo returned nil",
			slog.String("stream_url", streamURL))
	}
	return result
}

// getCachedCodecInfo retrieves cached codec info from the database.
// Returns nil if not found or expired.
func (m *Manager) getCachedCodecInfo(ctx context.Context, streamURL string) *models.LastKnownCodec {
	if m.config.CodecRepo == nil {
		return nil
	}

	cached, err := m.config.CodecRepo.GetByStreamURL(ctx, streamURL)
	if err != nil {
		m.logger.Debug("Failed to get cached codec info",
			slog.String("stream_url", streamURL),
			slog.String("error", err.Error()))
		return nil
	}

	// Check if cached info has actual codec data (not just an error entry)
	if cached != nil && (cached.VideoCodec != "" || cached.AudioCodec != "") {
		m.logger.Debug("Using cached codec info from database",
			slog.String("stream_url", streamURL),
			slog.String("video_codec", cached.VideoCodec),
			slog.String("audio_codec", cached.AudioCodec))
		return cached
	}

	return nil
}

// createSession creates a new relay session.
func (m *Manager) createSession(ctx context.Context, channelID uuid.UUID, channelName string, streamSourceName string, streamURL string, profile *models.EncodingProfile) (*RelaySession, error) {
	// Check circuit breaker
	cb := m.circuitBreakers.Get(streamURL)
	if !cb.Allow() {
		return nil, fmt.Errorf("%w: circuit breaker open for %s", ErrUpstreamFailed, streamURL)
	}

	// Classify stream
	classification := m.classifier.Classify(ctx, streamURL)

	// Determine the actual input URL (HLS may use a media playlist)
	probeURL := streamURL
	if classification.SelectedMediaPlaylist != "" {
		probeURL = classification.SelectedMediaPlaylist
	}

	// Pre-probe codec info for faster FFmpeg startup
	// Always probe fresh and store result for channel UI
	// This is non-blocking - if probe fails, we continue without cached info
	codecInfo := m.ProbeAndStoreCodecInfo(ctx, probeURL)

	sessionCtx, sessionCancel := context.WithCancel(m.ctx)

	session := &RelaySession{
		ID:               uuid.New(),
		ChannelID:        channelID,
		ChannelName:      channelName,
		StreamSourceName: streamSourceName,
		StreamURL:        streamURL,
		EncodingProfile:  profile,
		Classification:   classification,
		CachedCodecInfo:  codecInfo,
		StartedAt:        time.Now(),
		manager:          m,
		ctx:              sessionCtx,
		cancel:           sessionCancel,
		readyCh:          make(chan struct{}),
		resourceHistory:  NewResourceHistory(),
	}

	// Initialize atomic values for frequently updated fields
	session.lastActivity.Store(time.Now())
	session.idleSince.Store(time.Time{})

	// Initialize fallback controller if fallback generator is ready
	// Note: Fallback settings are now managed at the manager level, not profile level
	if m.fallbackGenerator != nil && m.fallbackGenerator.IsReady() {
		session.fallbackGenerator = m.fallbackGenerator
		session.fallbackController = NewFallbackController(
			m.fallbackGenerator,
			3,  // Default error threshold
			30, // Default recovery interval in seconds
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
// This function is optimized to minimize lock hold time to avoid blocking
// API requests like /api/v1/relay/sessions.
func (m *Manager) cleanupStaleSessions() {
	// First, copy session pointers while holding the lock briefly
	m.mu.RLock()
	sessionList := make([]*RelaySession, 0, len(m.sessions))
	sessionIDs := make([]uuid.UUID, 0, len(m.sessions))
	for id, session := range m.sessions {
		sessionList = append(sessionList, session)
		sessionIDs = append(sessionIDs, id)
	}
	idleGracePeriod := m.config.IdleGracePeriod
	sessionTimeout := m.config.SessionTimeout
	m.mu.RUnlock()

	// Evaluate each session WITHOUT holding the manager lock
	// This allows Stats() and other operations to proceed concurrently
	var toRemove []uuid.UUID

	for i, session := range sessionList {
		id := sessionIDs[i]

		// Check session state using atomic load (no locks needed)
		closed := session.closed.Load()

		// ClientCount() uses processorsMu, not the manager lock
		clientCount := session.ClientCount()

		// Use helper methods for atomic values (no locks needed)
		lastActivity := session.LastActivity()
		idleSince := session.IdleSince()

		shouldRemove := false

		if closed {
			shouldRemove = true
		} else if clientCount == 0 {
			// Session has no clients - check idle grace period
			if !idleSince.IsZero() && time.Since(idleSince) > idleGracePeriod {
				// Session has been idle longer than the grace period
				m.logger.Debug("Closing idle session after grace period",
					slog.String("session_id", id.String()),
					slog.Duration("idle_duration", time.Since(idleSince)))
				shouldRemove = true
			} else if idleSince.IsZero() && time.Since(lastActivity) > sessionTimeout {
				// Fallback: no idleSince set but inactive for too long
				shouldRemove = true
			}
		}

		if shouldRemove {
			toRemove = append(toRemove, id)
		}
	}

	// Only acquire write lock if we have sessions to remove
	if len(toRemove) == 0 {
		return
	}

	m.mu.Lock()
	for _, id := range toRemove {
		if session, ok := m.sessions[id]; ok {
			delete(m.sessions, id)
			delete(m.channelSessions, session.ChannelID)
			go session.Close()
		}
	}
	m.mu.Unlock()
}

// ManagerStats holds manager statistics.
type ManagerStats struct {
	ActiveSessions  int                     `json:"active_sessions"`
	MaxSessions     int                     `json:"max_sessions"`
	Sessions        []SessionStats          `json:"sessions,omitempty"`
	ConnectionPool  ConnectionPoolStats     `json:"connection_pool"`
	CircuitBreakers map[string]CircuitStats `json:"circuit_breakers,omitempty"`
}
