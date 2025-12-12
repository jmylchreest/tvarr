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

	ffmpegBin := ffmpeg.NewBinaryDetector()

	// Initialize prober with detected ffprobe path
	var prober *ffmpeg.Prober
	if binInfo, err := ffmpegBin.Detect(ctx); err == nil && binInfo.FFprobePath != "" {
		prober = ffmpeg.NewProber(binInfo.FFprobePath).WithTimeout(10 * time.Second)
	}

	m := &Manager{
		config:            config,
		ffmpegBin:         ffmpegBin,
		prober:            prober,
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
// channelUpdatedAt is used to invalidate stale codec cache entries.
func (m *Manager) GetOrCreateSession(ctx context.Context, channelID uuid.UUID, channelName string, streamSourceName string, streamURL string, profile *models.RelayProfile, channelUpdatedAt time.Time) (*RelaySession, error) {
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
	session, err := m.createSession(ctx, channelID, channelName, streamSourceName, streamURL, profile, channelUpdatedAt)
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

	session.mu.RLock()
	closed := session.closed
	session.mu.RUnlock()

	if closed {
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

// GetOrProbeCodecInfo retrieves cached codec info or probes the stream.
// Returns nil if probing is not available or fails (non-fatal).
func (m *Manager) GetOrProbeCodecInfo(ctx context.Context, streamURL string) *models.LastKnownCodec {
	return m.GetOrProbeCodecInfoWithFreshness(ctx, streamURL, time.Time{})
}

// GetOrProbeCodecInfoWithFreshness retrieves cached codec info or probes the stream.
// If mustBeNewerThan is non-zero, cached entries older than this are considered stale.
// This allows forcing a re-probe when the channel has been updated.
// Returns nil if probing is not available or fails (non-fatal).
func (m *Manager) GetOrProbeCodecInfoWithFreshness(ctx context.Context, streamURL string, mustBeNewerThan time.Time) *models.LastKnownCodec {
	// If no prober available, skip
	if m.prober == nil {
		return nil
	}

	// Try to get from cache first
	if m.config.CodecRepo != nil {
		cached, err := m.config.CodecRepo.GetByStreamURL(ctx, streamURL)
		if err == nil && cached != nil && !cached.IsExpired() && cached.IsValid() {
			// Check freshness - if cache is older than mustBeNewerThan, treat as stale
			if !mustBeNewerThan.IsZero() && time.Time(cached.ProbedAt).Before(mustBeNewerThan) {
				m.logger.Debug("Cached codec info is stale (older than channel update), will re-probe",
					slog.String("stream_url", streamURL),
					slog.Time("probed_at", time.Time(cached.ProbedAt)),
					slog.Time("must_be_newer_than", mustBeNewerThan))
				// Continue to re-probe below
			} else {
				// Update hit count in background
				go func() {
					if touchErr := m.config.CodecRepo.Touch(context.Background(), streamURL); touchErr != nil {
						m.logger.Debug("Failed to touch codec cache", slog.String("error", touchErr.Error()))
					}
				}()
				m.logger.Debug("Using cached codec info",
					slog.String("stream_url", streamURL),
					slog.String("video_codec", cached.VideoCodec),
					slog.String("audio_codec", cached.AudioCodec))
				return cached
			}
		}
	}

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

// createSession creates a new relay session.
// channelUpdatedAt is used to invalidate stale codec cache entries.
func (m *Manager) createSession(ctx context.Context, channelID uuid.UUID, channelName string, streamSourceName string, streamURL string, profile *models.RelayProfile, channelUpdatedAt time.Time) (*RelaySession, error) {
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
	// Use channelUpdatedAt to invalidate stale cache entries (e.g., after channel re-ingestion)
	// This is non-blocking - if probe fails, we continue without cached info
	codecInfo := m.GetOrProbeCodecInfoWithFreshness(ctx, probeURL, channelUpdatedAt)

	sessionCtx, sessionCancel := context.WithCancel(m.ctx)

	session := &RelaySession{
		ID:               uuid.New(),
		ChannelID:        channelID,
		ChannelName:      channelName,
		StreamSourceName: streamSourceName,
		StreamURL:        streamURL,
		Profile:          profile,
		Classification:   classification,
		CachedCodecInfo:  codecInfo,
		StartedAt:        time.Now(),
		manager:          m,
		ctx:              sessionCtx,
		cancel:           sessionCancel,
		readyCh:          make(chan struct{}),
	}

	// Initialize atomic values for frequently updated fields
	session.lastActivity.Store(time.Now())
	session.idleSince.Store(time.Time{})

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
		clientCount := session.ClientCount()
		closed := session.closed
		session.mu.RUnlock()

		// Use helper methods for atomic values
		lastActivity := session.LastActivity()
		idleSince := session.IdleSince()

		shouldRemove := false

		if closed {
			shouldRemove = true
		} else if clientCount == 0 {
			// Session has no clients - check idle grace period
			if !idleSince.IsZero() && time.Since(idleSince) > m.config.IdleGracePeriod {
				// Session has been idle longer than the grace period
				m.logger.Info("Closing idle session after grace period",
					slog.String("session_id", id.String()),
					slog.Duration("idle_duration", time.Since(idleSince)))
				shouldRemove = true
			} else if idleSince.IsZero() && time.Since(lastActivity) > m.config.SessionTimeout {
				// Fallback: no idleSince set but inactive for too long
				shouldRemove = true
			}
		}

		if shouldRemove {
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
