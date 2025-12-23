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

// formatBitrateKbps returns bitrate in kbps as a string, or "unknown" if 0
func formatBitrateKbps(bitrate int) string {
	if bitrate == 0 {
		return "unknown"
	}
	return fmt.Sprintf("%d", bitrate/1000)
}

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
	// DaemonRegistry for distributed transcoding.
	// If set, sessions can use remote ffmpegd daemons for transcoding.
	DaemonRegistry *DaemonRegistry
	// DaemonStreamManager manages persistent streams from remote daemons.
	DaemonStreamManager *DaemonStreamManager
	// ActiveJobManager manages active transcode jobs on remote daemons.
	ActiveJobManager *ActiveJobManager
	// FFmpegDSpawner for spawning local ffmpegd subprocesses.
	// Used when transcoding is needed but no remote daemons are available.
	FFmpegDSpawner *FFmpegDSpawner
	// PreferRemote indicates whether to prefer remote daemons over local FFmpeg.
	PreferRemote bool
	// EncoderOverridesProvider fetches enabled encoder overrides for transcoding.
	EncoderOverridesProvider EncoderOverridesProvider
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
	ffmpegBin  *ffmpeg.BinaryDetector // Used for ffprobe detection (stream probing)
	prober     *ffmpeg.Prober
	classifier *StreamClassifier
	logger     *slog.Logger

	mu       sync.RWMutex
	sessions map[models.ULID]*RelaySession
	// channelSessions maps channel IDs to session IDs for reuse
	channelSessions map[models.ULID]models.ULID

	circuitBreakers          *CircuitBreakerRegistry
	connectionPool           *ConnectionPool
	fallbackGenerator        *FallbackGenerator
	daemonRegistry           *DaemonRegistry
	daemonStreamMgr          *DaemonStreamManager
	activeJobMgr             *ActiveJobManager
	ffmpegDSpawner           *FFmpegDSpawner // For local transcoding via tvarr-ffmpegd subprocess
	encoderOverridesProvider EncoderOverridesProvider

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewManager creates a new relay manager.
func NewManager(config ManagerConfig) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	logger := slog.Default().With(slog.String("component", "relay"))

	ffmpegBin := ffmpeg.NewBinaryDetector()

	// Initialize prober with detected ffprobe path (for stream probing, not transcoding)
	var prober *ffmpeg.Prober
	if binInfo, err := ffmpegBin.Detect(ctx); err == nil && binInfo.FFprobePath != "" {
		prober = ffmpeg.NewProber(binInfo.FFprobePath).WithTimeout(10 * time.Second)
	}

	// Use spawner from config if provided, otherwise create a basic one
	// Note: When using distributed transcoding via serve.go, the spawner is configured
	// with the coordinator address. This fallback is for standalone manager usage.
	spawner := config.FFmpegDSpawner
	if spawner == nil {
		spawner = NewFFmpegDSpawner(FFmpegDSpawnerConfig{
			Logger: logger.With(slog.String("subcomponent", "ffmpegd-spawner")),
		})
	}

	if spawner.IsAvailable() {
		version := spawner.GetVersion()
		if version != "" {
			logger.Info("Local tvarr-ffmpegd binary found for transcoding",
				slog.String("version", version),
				slog.String("path", spawner.BinaryPath()))
		} else {
			logger.Info("Local tvarr-ffmpegd binary found for transcoding",
				slog.String("path", spawner.BinaryPath()))
		}
	} else {
		logger.Debug("tvarr-ffmpegd binary not found - local subprocess transcoding unavailable")
	}

	m := &Manager{
		config:                   config,
		ffmpegBin:                ffmpegBin,
		prober:                   prober,
		classifier:               NewStreamClassifier(config.HTTPClient),
		logger:                   logger,
		sessions:                 make(map[models.ULID]*RelaySession),
		channelSessions:          make(map[models.ULID]models.ULID),
		circuitBreakers:          NewCircuitBreakerRegistry(config.CircuitBreakerConfig),
		connectionPool:           NewConnectionPool(config.ConnectionPoolConfig),
		fallbackGenerator:        NewFallbackGenerator(config.FallbackConfig, logger),
		daemonRegistry:           config.DaemonRegistry,
		daemonStreamMgr:          config.DaemonStreamManager,
		activeJobMgr:             config.ActiveJobManager,
		ffmpegDSpawner:           spawner,
		encoderOverridesProvider: config.EncoderOverridesProvider,
		ctx:                      ctx,
		cancel:                   cancel,
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

// DaemonRegistry returns the manager's daemon registry for distributed transcoding.
func (m *Manager) DaemonRegistry() *DaemonRegistry {
	return m.daemonRegistry
}

// DaemonStreamManager returns the manager's daemon stream manager for remote transcoding.
func (m *Manager) DaemonStreamManager() *DaemonStreamManager {
	return m.daemonStreamMgr
}

// ActiveJobManager returns the manager's active job manager for remote transcoding.
func (m *Manager) ActiveJobManager() *ActiveJobManager {
	return m.activeJobMgr
}

// PreferRemote returns whether remote daemons should be preferred over local subprocess.
func (m *Manager) PreferRemote() bool {
	return m.config.PreferRemote
}

// FFmpegDSpawner returns the manager's ffmpegd spawner for local transcoding.
func (m *Manager) FFmpegDSpawner() *FFmpegDSpawner {
	return m.ffmpegDSpawner
}

// EncoderOverridesProvider returns the manager's encoder overrides provider.
func (m *Manager) EncoderOverridesProvider() EncoderOverridesProvider {
	return m.encoderOverridesProvider
}

// GetOrCreateSession gets an existing session for the channel or creates a new one.
//
// This function is carefully designed to avoid holding the manager lock during slow
// operations (stream classification, codec probing) to prevent blocking API requests
// like /api/v1/relay/sessions while a new session is being created.
func (m *Manager) GetOrCreateSession(ctx context.Context, channelID models.ULID, channelName string, sourceID models.ULID, streamSourceName string, streamURL string, sourceMaxConcurrentStreams int, profile *models.EncodingProfile) (*RelaySession, error) {
	// First, check if session already exists (fast path with read lock)
	m.mu.RLock()
	m.logger.Debug("GetOrCreateSession lookup",
		slog.String("channel_id", channelID.String()),
		slog.Int("total_channel_sessions", len(m.channelSessions)),
		slog.Int("total_sessions", len(m.sessions)))
	if sessionID, ok := m.channelSessions[channelID]; ok {
		if session, ok := m.sessions[sessionID]; ok {
			isClosed := session.closed.Load()
			ingestCompleted := session.IngestCompleted()
			m.logger.Debug("Found session in lookup",
				slog.String("channel_id", channelID.String()),
				slog.String("session_id", sessionID.String()),
				slog.Bool("is_closed", isClosed),
				slog.Bool("ingest_completed", ingestCompleted))
			if !isClosed {
				// Session is not closed - check if we can reuse it
				if !ingestCompleted {
					// Origin is still connected - reuse it
					m.mu.RUnlock()
					m.logger.Debug("Reusing existing session for channel (origin connected)",
						slog.String("channel_id", channelID.String()),
						slog.String("session_id", session.ID.String()))
					return session, nil
				}
				// Ingest completed (finite stream EOF) - but check if we still have active content
				// This handles IPTV error videos where transcoding is still in progress
				if session.HasActiveContent() {
					m.mu.RUnlock()
					m.logger.Debug("Reusing existing session for channel (ingest completed but has active content)",
						slog.String("channel_id", channelID.String()),
						slog.String("session_id", session.ID.String()))
					return session, nil
				}
				m.logger.Debug("Found existing session but origin has disconnected (EOF) and no active content",
					slog.String("channel_id", channelID.String()),
					slog.String("session_id", sessionID.String()))
			} else {
				m.logger.Debug("Found existing session but it's closed",
					slog.String("channel_id", channelID.String()),
					slog.String("session_id", sessionID.String()))
			}
		} else {
			m.logger.Debug("channelSessions has entry but sessions map doesn't",
				slog.String("channel_id", channelID.String()),
				slog.String("session_id", sessionID.String()))
		}
	} else {
		m.logger.Debug("No existing session for channel",
			slog.String("channel_id", channelID.String()))
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
	session, err := m.createSession(ctx, channelID, channelName, sourceID, streamSourceName, streamURL, sourceMaxConcurrentStreams, profile)
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
				// Check if we should reuse the existing session
				if !existingSession.IngestCompleted() || existingSession.HasActiveContent() {
					// Another goroutine won the race - close our session and return the existing one
					// Reuse if origin is still connected OR if there's active content (finite stream with active transcoders)
					session.Close()
					return existingSession, nil
				}
			}
			// Existing session is closed or has no active content, remove it
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
func (m *Manager) GetSession(sessionID models.ULID) (*RelaySession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[sessionID]
	return session, ok
}

// GetSessionForChannel returns an existing session for the channel if one exists.
// Unlike GetOrCreateSession, this does not create a new session if none exists.
// Returns nil if no active session exists for the channel.
func (m *Manager) GetSessionForChannel(channelID models.ULID) *RelaySession {
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
func (m *Manager) HasSessionForChannel(channelID models.ULID) bool {
	return m.GetSessionForChannel(channelID) != nil
}

// CountActiveSessionsForSource counts how many active (non-closed) sessions are
// connected to a given source. This is used to check if a new connection would
// exceed the source's max_concurrent_streams limit.
func (m *Manager) CountActiveSessionsForSource(sourceID models.ULID) int {
	if sourceID.IsZero() {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, session := range m.sessions {
		if !session.closed.Load() && session.SourceID == sourceID {
			count++
		}
	}
	return count
}

// CloseSession closes a specific session.
func (m *Manager) CloseSession(sessionID models.ULID) error {
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
		slog.String("container", codecInfo.ContainerFormat),
		slog.String("video_codec", codecInfo.VideoCodec),
		slog.String("video_profile", codecInfo.VideoProfile),
		slog.String("resolution", fmt.Sprintf("%dx%d", codecInfo.VideoWidth, codecInfo.VideoHeight)),
		slog.Float64("framerate", codecInfo.VideoFramerate),
		slog.String("video_bitrate_kbps", formatBitrateKbps(codecInfo.VideoBitrate)),
		slog.String("audio_codec", codecInfo.AudioCodec),
		slog.Int("audio_channels", codecInfo.AudioChannels),
		slog.Int("audio_sample_rate", codecInfo.AudioSampleRate),
		slog.String("audio_bitrate_kbps", formatBitrateKbps(codecInfo.AudioBitrate)),
		slog.Int("stream_count", codecInfo.StreamCount),
		slog.Bool("is_live", codecInfo.IsLiveStream),
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
func (m *Manager) GetOrProbeCodecInfo(ctx context.Context, channelID models.ULID, streamURL string) *models.LastKnownCodec {
	var result *models.LastKnownCodec
	var source string

	// Priority 1: Check for active session - this is the fastest path and doesn't require a network call
	session := m.GetSessionForChannel(channelID)
	if session != nil && session.CachedCodecInfo != nil {
		result = session.CachedCodecInfo
		source = "session"
		m.logger.Debug("Codec info resolved",
			slog.String("channel_id", channelID.String()),
			slog.String("source", source),
			slog.String("video_codec", result.VideoCodec),
			slog.String("audio_codec", result.AudioCodec))
		return result
	}

	// Priority 2: Check if connection pool would allow a probe
	host, err := extractHost(streamURL)
	if err != nil {
		// Fall back to cached database info
		result = m.getCachedCodecInfo(ctx, streamURL)
		source = "cache"
		m.logCodecResolution(channelID, source, result, "host_extraction_failed")
		return result
	}

	// Check connection pool stats - if we're at or near the limit, don't probe
	poolStats := m.connectionPool.Stats()
	currentHostConns := poolStats.HostConnections[host]
	atConnectionLimit := currentHostConns >= poolStats.MaxPerHost

	if atConnectionLimit {
		// Fall back to cached database info
		result = m.getCachedCodecInfo(ctx, streamURL)
		source = "cache"
		m.logCodecResolution(channelID, source, result, "connection_limit")
		return result
	}

	// Priority 3: We have capacity - probe fresh
	result = m.ProbeAndStoreCodecInfo(ctx, streamURL)
	source = "probe"
	m.logCodecResolution(channelID, source, result, "")
	return result
}

// logCodecResolution emits a single consolidated log for codec resolution
func (m *Manager) logCodecResolution(channelID models.ULID, source string, codec *models.LastKnownCodec, reason string) {
	attrs := []any{
		slog.String("channel_id", channelID.String()),
		slog.String("source", source),
		slog.Bool("found", codec != nil),
	}
	if codec != nil {
		attrs = append(attrs,
			slog.String("video_codec", codec.VideoCodec),
			slog.String("audio_codec", codec.AudioCodec))
	}
	if reason != "" {
		attrs = append(attrs, slog.String("reason", reason))
	}
	m.logger.Debug("Codec info resolved", attrs...)
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
		return cached
	}

	return nil
}

// createSession creates a new relay session.
func (m *Manager) createSession(ctx context.Context, channelID models.ULID, channelName string, sourceID models.ULID, streamSourceName string, streamURL string, sourceMaxConcurrentStreams int, profile *models.EncodingProfile) (*RelaySession, error) {
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

	// Get codec info - use recent cache if available to avoid re-probing
	// Cache is valid for 60 minutes since codec info rarely changes for a source
	var codecInfo *models.LastKnownCodec
	var codecSource string
	if cached := m.getCachedCodecInfo(ctx, probeURL); cached != nil {
		// Use cached info if it's recent (probed within last 60 minutes)
		if cached.ProbedAt.After(time.Now().Add(-60 * time.Minute)) {
			codecInfo = cached
			codecSource = "cache"
		}
	}

	// Only probe fresh if no recent cache
	if codecInfo == nil {
		codecInfo = m.ProbeAndStoreCodecInfo(ctx, probeURL)
		codecSource = "probe"
	}

	// Log codec resolution result
	if codecInfo != nil {
		m.logger.Debug("Codec info resolved",
			slog.String("stream_url", probeURL),
			slog.String("source", codecSource),
			slog.String("video_codec", codecInfo.VideoCodec),
			slog.String("audio_codec", codecInfo.AudioCodec),
			slog.Duration("cache_age", time.Since(codecInfo.ProbedAt)))
	}

	sessionCtx, sessionCancel := context.WithCancel(m.ctx)

	session := &RelaySession{
		ID:                        models.NewULID(),
		ChannelID:                 channelID,
		ChannelName:               channelName,
		SourceID:                  sourceID,
		StreamSourceName:          streamSourceName,
		StreamURL:                 streamURL,
		SourceMaxConcurrentStreams: sourceMaxConcurrentStreams,
		EncodingProfile:           profile,
		Classification:            classification,
		CachedCodecInfo:           codecInfo,
		StartedAt:                 time.Now(),
		manager:                   m,
		ctx:                       sessionCtx,
		cancel:                    sessionCancel,
		readyCh:                   make(chan struct{}),
		resourceHistory:           NewResourceHistory(),
		edgeBandwidth:             NewEdgeBandwidthTrackers(),
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
	sessionIDs := make([]models.ULID, 0, len(m.sessions))
	for id, session := range m.sessions {
		sessionList = append(sessionList, session)
		sessionIDs = append(sessionIDs, id)
	}
	idleGracePeriod := m.config.IdleGracePeriod
	sessionTimeout := m.config.SessionTimeout
	m.mu.RUnlock()

	// Evaluate each session WITHOUT holding the manager lock
	// This allows Stats() and other operations to proceed concurrently
	var toRemove []models.ULID

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
			// BUT don't remove if session has active transcoders still running.
			// This is important for finite streams where transcoding may still be in progress.
			// Note: Buffer data alone (without active transcoders) doesn't keep a session alive.
			hasActiveContent := session.HasActiveContent()

			if !idleSince.IsZero() && time.Since(idleSince) > idleGracePeriod {
				if hasActiveContent {
					// Don't clean up - transcoders still running
					m.logger.Debug("Session idle but has active content, keeping alive",
						slog.String("session_id", id.String()),
						slog.Duration("idle_duration", time.Since(idleSince)))
				} else {
					// Session has been idle longer than the grace period and no active content
					m.logger.Debug("Closing idle session after grace period",
						slog.String("session_id", id.String()),
						slog.Duration("idle_duration", time.Since(idleSince)))
					shouldRemove = true
				}
			} else if idleSince.IsZero() && time.Since(lastActivity) > sessionTimeout {
				// Fallback: no idleSince set but inactive for too long
				// Still respect active content check
				if !hasActiveContent {
					shouldRemove = true
				}
			}
		}

		if shouldRemove {
			m.logger.Info("Session scheduled for cleanup",
				slog.String("session_id", id.String()),
				slog.String("channel_id", session.ChannelID.String()),
				slog.Bool("closed", closed),
				slog.Int("client_count", clientCount),
				slog.Time("idle_since", idleSince),
				slog.Time("last_activity", lastActivity),
				slog.Duration("idle_grace_period", idleGracePeriod))
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
			m.logger.Info("Removing session from manager",
				slog.String("session_id", id.String()),
				slog.String("channel_id", session.ChannelID.String()))
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
