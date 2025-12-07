package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/jmylchreest/tvarr/internal/ffmpeg"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/relay"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/jmylchreest/tvarr/internal/services"
)

// ErrRelayProfileNotFound is returned when a relay profile is not found.
var ErrRelayProfileNotFound = errors.New("relay profile not found")

// ErrChannelNotFound is returned when a channel is not found.
var ErrChannelNotFound = errors.New("channel not found")

// ErrProxyNotFound is returned when a stream proxy is not found.
var ErrProxyNotFound = errors.New("stream proxy not found")

// RelayService provides business logic for stream relay functionality.
type RelayService struct {
	relayProfileRepo   repository.RelayProfileRepository
	lastKnownCodecRepo repository.LastKnownCodecRepository
	channelRepo        repository.ChannelRepository
	streamProxyRepo    repository.StreamProxyRepository
	relayManager       *relay.Manager
	ffmpegDetector     *ffmpeg.BinaryDetector
	hardwareDetector   *services.HardwareDetector
	prober             *ffmpeg.Prober
	logger             *slog.Logger
}

// NewRelayService creates a new relay service.
func NewRelayService(
	relayProfileRepo repository.RelayProfileRepository,
	lastKnownCodecRepo repository.LastKnownCodecRepository,
	channelRepo repository.ChannelRepository,
	streamProxyRepo repository.StreamProxyRepository,
) *RelayService {
	config := relay.DefaultManagerConfig()
	// Pass codec repo for pre-probing and caching
	config.CodecRepo = lastKnownCodecRepo
	ffmpegDetector := ffmpeg.NewBinaryDetector()

	// Initialize hardware detector with detected FFmpeg path
	var hardwareDetector *services.HardwareDetector
	if binInfo, err := ffmpegDetector.Detect(context.Background()); err == nil {
		hardwareDetector = services.NewHardwareDetector(binInfo.FFmpegPath)
	}

	return &RelayService{
		relayProfileRepo:   relayProfileRepo,
		lastKnownCodecRepo: lastKnownCodecRepo,
		channelRepo:        channelRepo,
		streamProxyRepo:    streamProxyRepo,
		relayManager:       relay.NewManager(config),
		ffmpegDetector:     ffmpegDetector,
		hardwareDetector:   hardwareDetector,
		logger:             slog.Default(),
	}
}

// WithLogger sets the logger for the service.
func (s *RelayService) WithLogger(logger *slog.Logger) *RelayService {
	s.logger = logger
	return s
}

// WithHTTPClient sets the HTTP client for the relay manager.
func (s *RelayService) WithHTTPClient(client *http.Client) *RelayService {
	// Recreate manager with custom config
	config := relay.DefaultManagerConfig()
	config.HTTPClient = client
	config.CodecRepo = s.lastKnownCodecRepo
	s.relayManager.Close()
	s.relayManager = relay.NewManager(config)
	return s
}

// Close shuts down the relay service and all active sessions.
func (s *RelayService) Close() {
	if s.relayManager != nil {
		s.relayManager.Close()
	}
}

// RelayProfile CRUD operations

// GetAllProfiles returns all relay profiles.
func (s *RelayService) GetAllProfiles(ctx context.Context) ([]*models.RelayProfile, error) {
	return s.relayProfileRepo.GetAll(ctx)
}

// GetProfileByID returns a relay profile by ID.
func (s *RelayService) GetProfileByID(ctx context.Context, id models.ULID) (*models.RelayProfile, error) {
	profile, err := s.relayProfileRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrRelayProfileNotFound) {
			return nil, ErrRelayProfileNotFound
		}
		return nil, err
	}
	if profile == nil {
		return nil, ErrRelayProfileNotFound
	}
	return profile, nil
}

// GetProfileByName returns a relay profile by name.
func (s *RelayService) GetProfileByName(ctx context.Context, name string) (*models.RelayProfile, error) {
	profile, err := s.relayProfileRepo.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, models.ErrRelayProfileNotFound) {
			return nil, ErrRelayProfileNotFound
		}
		return nil, err
	}
	if profile == nil {
		return nil, ErrRelayProfileNotFound
	}
	return profile, nil
}

// GetDefaultProfile returns the default relay profile.
func (s *RelayService) GetDefaultProfile(ctx context.Context) (*models.RelayProfile, error) {
	return s.relayProfileRepo.GetDefault(ctx)
}

// CreateProfile creates a new relay profile.
func (s *RelayService) CreateProfile(ctx context.Context, profile *models.RelayProfile) error {
	// Validate the profile settings
	if err := s.validateProfile(ctx, profile); err != nil {
		return err
	}

	return s.relayProfileRepo.Create(ctx, profile)
}

// UpdateProfile updates an existing relay profile.
func (s *RelayService) UpdateProfile(ctx context.Context, profile *models.RelayProfile) error {
	// Validate the profile settings
	if err := s.validateProfile(ctx, profile); err != nil {
		return err
	}

	return s.relayProfileRepo.Update(ctx, profile)
}

// DeleteProfile deletes a relay profile by ID.
func (s *RelayService) DeleteProfile(ctx context.Context, id models.ULID) error {
	return s.relayProfileRepo.Delete(ctx, id)
}

// SetDefaultProfile sets a relay profile as the default.
func (s *RelayService) SetDefaultProfile(ctx context.Context, id models.ULID) error {
	return s.relayProfileRepo.SetDefault(ctx, id)
}

// validateProfile validates relay profile settings against available FFmpeg capabilities.
func (s *RelayService) validateProfile(ctx context.Context, profile *models.RelayProfile) error {
	// Basic validation is done in model hooks
	// Additional validation could check FFmpeg capabilities

	if profile.VideoCodec != models.VideoCodecCopy {
		binInfo, err := s.ffmpegDetector.Detect(ctx)
		if err != nil {
			s.logger.Warn("Could not detect FFmpeg for encoder validation", "error", err)
			return nil // Allow creation but warn
		}

		if !binInfo.HasEncoder(string(profile.VideoCodec)) {
			return fmt.Errorf("video encoder %s not available in FFmpeg", profile.VideoCodec)
		}
	}

	if profile.AudioCodec != models.AudioCodecCopy {
		binInfo, err := s.ffmpegDetector.Detect(ctx)
		if err != nil {
			return nil // Already warned above
		}

		if !binInfo.HasEncoder(string(profile.AudioCodec)) {
			return fmt.Errorf("audio encoder %s not available in FFmpeg", profile.AudioCodec)
		}
	}

	return nil
}

// LastKnownCodec operations

// GetLastKnownCodec returns the cached codec info for a stream URL.
func (s *RelayService) GetLastKnownCodec(ctx context.Context, streamURL string) (*models.LastKnownCodec, error) {
	return s.lastKnownCodecRepo.GetByStreamURL(ctx, streamURL)
}

// ProbeStream probes a stream URL for codec information.
func (s *RelayService) ProbeStream(ctx context.Context, streamURL string) (*models.LastKnownCodec, error) {
	binInfo, err := s.ffmpegDetector.Detect(ctx)
	if err != nil {
		return nil, fmt.Errorf("detecting FFmpeg: %w", err)
	}

	if s.prober == nil {
		s.prober = ffmpeg.NewProber(binInfo.FFprobePath)
	}

	// Use QuickProbe for stream info
	streamInfo, err := s.prober.QuickProbe(ctx, streamURL)
	if err != nil {
		return nil, fmt.Errorf("probing stream: %w", err)
	}

	// Convert to LastKnownCodec model (StreamInfo has direct video/audio fields)
	codec := &models.LastKnownCodec{
		StreamURL:       streamURL,
		VideoCodec:      streamInfo.VideoCodec,
		VideoWidth:      streamInfo.VideoWidth,
		VideoHeight:     streamInfo.VideoHeight,
		VideoFramerate:  streamInfo.VideoFramerate,
		VideoBitrate:    streamInfo.VideoBitrate,
		AudioCodec:      streamInfo.AudioCodec,
		AudioSampleRate: streamInfo.AudioSampleRate,
		AudioChannels:   streamInfo.AudioChannels,
		AudioBitrate:    streamInfo.AudioBitrate,
	}

	// Store the probed information
	if err := s.lastKnownCodecRepo.Upsert(ctx, codec); err != nil {
		s.logger.Warn("Failed to cache codec info", "url", streamURL, "error", err)
		// Don't fail the request, just log
	}

	return codec, nil
}

// GetCodecCacheStats returns statistics about the codec cache.
func (s *RelayService) GetCodecCacheStats(ctx context.Context) (*repository.CodecCacheStats, error) {
	return s.lastKnownCodecRepo.GetStats(ctx)
}

// CleanupExpiredCodecs removes expired codec cache entries.
func (s *RelayService) CleanupExpiredCodecs(ctx context.Context) (int64, error) {
	return s.lastKnownCodecRepo.DeleteExpired(ctx)
}

// Relay session operations

// StartRelay starts a relay session for a channel.
func (s *RelayService) StartRelay(ctx context.Context, channelID models.ULID, profileID *models.ULID) (*relay.RelaySession, error) {
	// Get channel
	channel, err := s.channelRepo.GetByID(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrChannelNotFound, err)
	}

	// Get profile (use default if not specified)
	var profile *models.RelayProfile
	if profileID != nil {
		profile, err = s.relayProfileRepo.GetByID(ctx, *profileID)
		if err != nil {
			if errors.Is(err, models.ErrRelayProfileNotFound) {
				return nil, fmt.Errorf("%w: %v", ErrRelayProfileNotFound, *profileID)
			}
			return nil, err
		}
	} else {
		profile, err = s.relayProfileRepo.GetDefault(ctx)
		if err != nil && !errors.Is(err, models.ErrRelayProfileNotFound) {
			return nil, err
		}
		// profile can be nil if no default is set (use passthrough)
	}

	// Convert ULID to UUID for relay manager
	channelUUID := uuid.UUID(channelID)

	// Start the relay session
	session, err := s.relayManager.GetOrCreateSession(ctx, channelUUID, channel.ChannelName, channel.StreamURL, profile)
	if err != nil {
		return nil, fmt.Errorf("starting relay session: %w", err)
	}

	s.logger.Info("Started relay session",
		"session_id", session.ID,
		"channel_id", channelID,
		"stream_url", channel.StreamURL,
		"profile", func() string {
			if profile != nil {
				return profile.Name
			}
			return "passthrough"
		}(),
	)

	return session, nil
}

// AddRelayClient adds a client to a relay session and returns a reader.
func (s *RelayService) AddRelayClient(sessionID uuid.UUID, userAgent, remoteAddr string) (*relay.BufferClient, *relay.StreamReader, error) {
	session, ok := s.relayManager.GetSession(sessionID)
	if !ok {
		return nil, nil, relay.ErrSessionNotFound
	}

	return session.AddClient(userAgent, remoteAddr)
}

// RemoveRelayClient removes a client from a relay session.
func (s *RelayService) RemoveRelayClient(sessionID, clientID uuid.UUID) error {
	session, ok := s.relayManager.GetSession(sessionID)
	if !ok {
		return relay.ErrSessionNotFound
	}

	session.RemoveClient(clientID)
	return nil
}

// StopRelay stops a relay session.
func (s *RelayService) StopRelay(sessionID uuid.UUID) error {
	return s.relayManager.CloseSession(sessionID)
}

// GetRelayStats returns relay manager statistics.
func (s *RelayService) GetRelayStats() relay.ManagerStats {
	return s.relayManager.Stats()
}

// GetFFmpegInfo returns information about the detected FFmpeg installation.
func (s *RelayService) GetFFmpegInfo(ctx context.Context) (*ffmpeg.BinaryInfo, error) {
	return s.ffmpegDetector.Detect(ctx)
}

// ClassificationResult is an alias for relay.ClassificationResult for external use.
type ClassificationResult = relay.ClassificationResult

// ClassifyStream classifies a stream URL.
func (s *RelayService) ClassifyStream(ctx context.Context, streamURL string) ClassificationResult {
	classifier := relay.NewStreamClassifier(s.GetHTTPClient())
	return classifier.Classify(ctx, streamURL)
}

// CreateHLSCollapser creates an HLS collapser for the given playlist URL.
func (s *RelayService) CreateHLSCollapser(playlistURL string) *relay.HLSCollapser {
	return relay.NewHLSCollapser(s.GetHTTPClient(), playlistURL)
}

// GetHTTPClient returns the HTTP client used by the relay manager.
func (s *RelayService) GetHTTPClient() *http.Client {
	if s.relayManager != nil {
		return s.relayManager.HTTPClient()
	}
	return http.DefaultClient
}

// StreamInfo contains the information needed to stream a channel through a proxy.
type StreamInfo struct {
	Proxy   *models.StreamProxy
	Channel *models.Channel
	Profile *models.RelayProfile
}

// GetStreamInfo retrieves the proxy, channel, and optional relay profile for streaming.
// This is used by the stream handler to determine the delivery mode (redirect/proxy/relay).
func (s *RelayService) GetStreamInfo(ctx context.Context, proxyID, channelID models.ULID) (*StreamInfo, error) {
	// Get the stream proxy
	proxy, err := s.streamProxyRepo.GetByID(ctx, proxyID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProxyNotFound, err)
	}
	if proxy == nil {
		return nil, ErrProxyNotFound
	}

	// Get the channel
	channel, err := s.channelRepo.GetByID(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrChannelNotFound, err)
	}
	if channel == nil {
		return nil, ErrChannelNotFound
	}

	info := &StreamInfo{
		Proxy:   proxy,
		Channel: channel,
	}

	// If relay mode and a profile is specified, load it
	if proxy.ProxyMode == models.StreamProxyModeRelay && proxy.RelayProfileID != nil {
		profile, err := s.relayProfileRepo.GetByID(ctx, *proxy.RelayProfileID)
		if err != nil {
			// Log but don't fail - will use default profile
			s.logger.Warn("Failed to load relay profile for proxy",
				"proxy_id", proxyID,
				"profile_id", proxy.RelayProfileID,
				"error", err,
			)
		} else {
			info.Profile = profile
		}
	}

	return info, nil
}

// GetProxy returns a stream proxy by ID.
func (s *RelayService) GetProxy(ctx context.Context, id models.ULID) (*models.StreamProxy, error) {
	proxy, err := s.streamProxyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProxyNotFound, err)
	}
	if proxy == nil {
		return nil, ErrProxyNotFound
	}
	return proxy, nil
}

// GetChannel returns a channel by ID.
func (s *RelayService) GetChannel(ctx context.Context, id models.ULID) (*models.Channel, error) {
	channel, err := s.channelRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrChannelNotFound, err)
	}
	if channel == nil {
		return nil, ErrChannelNotFound
	}
	return channel, nil
}

// GetHardwareCapabilities returns the cached hardware capabilities, detecting if not already cached.
func (s *RelayService) GetHardwareCapabilities(ctx context.Context) (*services.HardwareCapabilities, error) {
	if s.hardwareDetector == nil {
		// Try to initialize hardware detector if not already done
		binInfo, err := s.ffmpegDetector.Detect(ctx)
		if err != nil {
			return nil, fmt.Errorf("FFmpeg not detected: %w", err)
		}
		s.hardwareDetector = services.NewHardwareDetector(binInfo.FFmpegPath)
	}

	// Return cached capabilities if available
	caps := s.hardwareDetector.GetCapabilities()
	if caps != nil {
		return caps, nil
	}

	// Otherwise detect and cache
	return s.hardwareDetector.Detect(ctx)
}

// RefreshHardwareCapabilities re-detects hardware capabilities.
func (s *RelayService) RefreshHardwareCapabilities(ctx context.Context) (*services.HardwareCapabilities, error) {
	if s.hardwareDetector == nil {
		// Try to initialize hardware detector if not already done
		binInfo, err := s.ffmpegDetector.Detect(ctx)
		if err != nil {
			return nil, fmt.Errorf("FFmpeg not detected: %w", err)
		}
		s.hardwareDetector = services.NewHardwareDetector(binInfo.FFmpegPath)
	}

	return s.hardwareDetector.Refresh(ctx)
}

// ProfileUsageInfo contains information about how a profile is being used.
type ProfileUsageInfo struct {
	ProxyCount int64
	Proxies    []*models.StreamProxy
}

// GetProfileUsage returns information about proxies using a given profile.
func (s *RelayService) GetProfileUsage(ctx context.Context, profileID models.ULID) (*ProfileUsageInfo, error) {
	count, err := s.streamProxyRepo.CountByRelayProfileID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("counting profile usage: %w", err)
	}

	var proxies []*models.StreamProxy
	if count > 0 {
		proxies, err = s.streamProxyRepo.GetByRelayProfileID(ctx, profileID)
		if err != nil {
			return nil, fmt.Errorf("getting proxies using profile: %w", err)
		}
	}

	return &ProfileUsageInfo{
		ProxyCount: count,
		Proxies:    proxies,
	}, nil
}
