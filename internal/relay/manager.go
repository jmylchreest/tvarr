package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	// CollapserConfig for HLS collapsing.
	CollapserConfig CollapserConfig
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
		CollapserConfig:      DefaultCollapserConfig(),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// RelaySession represents an active relay session.
type RelaySession struct {
	ID             uuid.UUID
	ChannelID      uuid.UUID
	StreamURL      string
	Profile        *models.RelayProfile
	Classification ClassificationResult
	StartedAt      time.Time
	LastActivity   time.Time

	manager *Manager
	buffer  *CyclicBuffer
	ctx     context.Context
	cancel  context.CancelFunc

	mu          sync.RWMutex
	ffmpegCmd   *ffmpeg.CommandBuilder
	hlsCollapser *HLSCollapser
	inputReader  io.ReadCloser
	closed      bool
	err         error
}

// Manager manages relay sessions and their lifecycles.
type Manager struct {
	config     ManagerConfig
	ffmpegBin  *ffmpeg.BinaryDetector
	classifier *StreamClassifier

	mu       sync.RWMutex
	sessions map[uuid.UUID]*RelaySession
	// channelSessions maps channel IDs to session IDs for reuse
	channelSessions map[uuid.UUID]uuid.UUID

	circuitBreakers *CircuitBreakerRegistry
	connectionPool  *ConnectionPool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewManager creates a new relay manager.
func NewManager(config ManagerConfig) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		config:          config,
		ffmpegBin:       ffmpeg.NewBinaryDetector(),
		classifier:      NewStreamClassifier(config.HTTPClient),
		sessions:        make(map[uuid.UUID]*RelaySession),
		channelSessions: make(map[uuid.UUID]uuid.UUID),
		circuitBreakers: NewCircuitBreakerRegistry(config.CircuitBreakerConfig),
		connectionPool:  NewConnectionPool(config.ConnectionPoolConfig),
		ctx:             ctx,
		cancel:          cancel,
	}

	// Start cleanup goroutine
	m.wg.Add(1)
	go m.cleanupLoop()

	return m
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

// start begins the relay session.
func (s *RelaySession) start(ctx context.Context) error {
	// Acquire connection slot
	release, err := s.manager.connectionPool.Acquire(ctx, s.StreamURL)
	if err != nil {
		return fmt.Errorf("acquiring connection: %w", err)
	}

	// Start appropriate pipeline based on classification and profile
	go func() {
		defer release()
		s.runPipeline()
	}()

	return nil
}

// runPipeline runs the relay pipeline.
func (s *RelaySession) runPipeline() {
	var err error
	defer func() {
		s.mu.Lock()
		s.err = err
		s.closed = true
		s.mu.Unlock()
		s.buffer.Close()
	}()

	// Determine pipeline based on profile and classification
	needsTranscoding := s.Profile != nil &&
		(s.Profile.VideoCodec != models.VideoCodecCopy ||
			s.Profile.AudioCodec != models.AudioCodecCopy)

	if needsTranscoding {
		err = s.runFFmpegPipeline()
	} else if s.Classification.Mode == StreamModeCollapsedHLS {
		err = s.runHLSCollapsePipeline()
	} else {
		err = s.runPassthroughPipeline()
	}

	// Record failure/success with circuit breaker
	cb := s.manager.circuitBreakers.Get(s.StreamURL)
	if err != nil && !errors.Is(err, context.Canceled) {
		cb.RecordFailure()
	}
}

// runFFmpegPipeline runs the FFmpeg transcoding pipeline.
func (s *RelaySession) runFFmpegPipeline() error {
	// Detect FFmpeg
	binInfo, err := s.manager.ffmpegBin.Detect(s.ctx)
	if err != nil {
		return fmt.Errorf("detecting ffmpeg: %w", err)
	}

	// Build command
	inputURL := s.StreamURL
	if s.Classification.SelectedMediaPlaylist != "" {
		inputURL = s.Classification.SelectedMediaPlaylist
	}

	builder := ffmpeg.NewCommandBuilder(binInfo.FFmpegPath).
		Input(inputURL).
		InputArgs("-re") // Read at native frame rate

	// Apply profile settings
	if s.Profile != nil {
		if s.Profile.VideoCodec == models.VideoCodecCopy {
			builder.VideoCodec("copy")
		} else {
			builder.VideoCodec(string(s.Profile.VideoCodec))
			if s.Profile.VideoBitrate > 0 {
				builder.VideoBitrate(fmt.Sprintf("%dk", s.Profile.VideoBitrate))
			}
			if s.Profile.VideoWidth > 0 || s.Profile.VideoHeight > 0 {
				builder.VideoScale(s.Profile.VideoWidth, s.Profile.VideoHeight)
			}
			if s.Profile.VideoPreset != "" {
				builder.VideoPreset(s.Profile.VideoPreset)
			}
		}

		if s.Profile.AudioCodec == models.AudioCodecCopy {
			builder.AudioCodec("copy")
		} else {
			builder.AudioCodec(string(s.Profile.AudioCodec))
			if s.Profile.AudioBitrate > 0 {
				builder.AudioBitrate(fmt.Sprintf("%dk", s.Profile.AudioBitrate))
			}
			if s.Profile.AudioSampleRate > 0 {
				builder.AudioSampleRate(s.Profile.AudioSampleRate)
			}
			if s.Profile.AudioChannels > 0 {
				builder.AudioChannels(s.Profile.AudioChannels)
			}
		}

		// Apply hardware acceleration
		if s.Profile.HWAccel != models.HWAccelNone {
			builder.HWAccel(string(s.Profile.HWAccel))
		}
	}

	// Output to pipe
	builder.OutputFormat(string(s.Profile.OutputFormat)).
		Output("pipe:1").
		OutputArgs("-fflags", "+genpts")

	// Build and run FFmpeg, write to buffer
	cmd := builder.Build()
	writer := NewStreamWriter(s.buffer)
	return cmd.StreamToWriter(s.ctx, writer)
}

// runHLSCollapsePipeline runs the HLS collapsing pipeline.
func (s *RelaySession) runHLSCollapsePipeline() error {
	playlistURL := s.StreamURL
	if s.Classification.SelectedMediaPlaylist != "" {
		playlistURL = s.Classification.SelectedMediaPlaylist
	}

	collapser := NewHLSCollapser(
		s.manager.config.HTTPClient,
		playlistURL,
		s.Classification.TargetDuration,
		s.manager.config.CollapserConfig,
	)

	s.mu.Lock()
	s.hlsCollapser = collapser
	s.mu.Unlock()

	collapser.Start(s.ctx)

	// Read from collapser and write to buffer
	buf := make([]byte, 64*1024)
	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		default:
		}

		n, err := collapser.ReadContext(s.ctx, buf)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, ErrCollapserAborted) {
				return nil
			}
			return err
		}

		if n > 0 {
			if err := s.buffer.WriteChunk(buf[:n]); err != nil {
				return err
			}
			s.mu.Lock()
			s.LastActivity = time.Now()
			s.mu.Unlock()
		}
	}
}

// runPassthroughPipeline runs the direct passthrough pipeline.
func (s *RelaySession) runPassthroughPipeline() error {
	req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, s.StreamURL, nil)
	if err != nil {
		return err
	}

	resp, err := s.manager.config.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}

	s.mu.Lock()
	s.inputReader = resp.Body
	s.mu.Unlock()

	// Read from upstream and write to buffer
	buf := make([]byte, 64*1024)
	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		default:
		}

		n, err := resp.Body.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		if n > 0 {
			if err := s.buffer.WriteChunk(buf[:n]); err != nil {
				return err
			}
			s.mu.Lock()
			s.LastActivity = time.Now()
			s.mu.Unlock()
		}
	}
}

// AddClient adds a client to the session and returns a reader.
func (s *RelaySession) AddClient(userAgent, remoteAddr string) (*BufferClient, *StreamReader, error) {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, nil, ErrSessionClosed
	}
	s.mu.RUnlock()

	client, err := s.buffer.AddClient(userAgent, remoteAddr)
	if err != nil {
		return nil, nil, err
	}

	reader := NewStreamReader(s.buffer, client)

	s.mu.Lock()
	s.LastActivity = time.Now()
	s.mu.Unlock()

	return client, reader, nil
}

// RemoveClient removes a client from the session.
func (s *RelaySession) RemoveClient(clientID uuid.UUID) bool {
	return s.buffer.RemoveClient(clientID)
}

// Close closes the session and releases resources.
func (s *RelaySession) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()

	s.cancel()

	if s.hlsCollapser != nil {
		s.hlsCollapser.Stop()
	}

	if s.inputReader != nil {
		s.inputReader.Close()
	}

	s.buffer.Close()
}

// Stats returns session statistics.
func (s *RelaySession) Stats() SessionStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bufferStats := s.buffer.Stats()

	var errStr string
	if s.err != nil {
		errStr = s.err.Error()
	}

	return SessionStats{
		ID:             s.ID.String(),
		ChannelID:      s.ChannelID.String(),
		StreamURL:      s.StreamURL,
		Classification: s.Classification.Mode.String(),
		StartedAt:      s.StartedAt,
		LastActivity:   s.LastActivity,
		ClientCount:    bufferStats.ClientCount,
		BytesWritten:   bufferStats.TotalBytesWritten,
		Closed:         s.closed,
		Error:          errStr,
	}
}

// ManagerStats holds manager statistics.
type ManagerStats struct {
	ActiveSessions  int                       `json:"active_sessions"`
	MaxSessions     int                       `json:"max_sessions"`
	Sessions        []SessionStats            `json:"sessions,omitempty"`
	ConnectionPool  ConnectionPoolStats       `json:"connection_pool"`
	CircuitBreakers map[string]CircuitStats   `json:"circuit_breakers,omitempty"`
}

// SessionStats holds session statistics.
type SessionStats struct {
	ID             string    `json:"id"`
	ChannelID      string    `json:"channel_id"`
	StreamURL      string    `json:"stream_url"`
	Classification string    `json:"classification"`
	StartedAt      time.Time `json:"started_at"`
	LastActivity   time.Time `json:"last_activity"`
	ClientCount    int       `json:"client_count"`
	BytesWritten   uint64    `json:"bytes_written"`
	Closed         bool      `json:"closed"`
	Error          string    `json:"error,omitempty"`
}
