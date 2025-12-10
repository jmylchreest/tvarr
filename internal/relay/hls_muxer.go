// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	gohlslib "github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
)

// HLSMuxer errors.
var (
	ErrHLSMuxerNotStarted = errors.New("HLS muxer not started")
	ErrHLSMuxerClosed     = errors.New("HLS muxer closed")
	ErrNoTracksConfigured = errors.New("no tracks configured")
)

// HLSMuxerVariant represents the HLS output segment type.
type HLSMuxerVariant int

const (
	// HLSMuxerVariantMPEGTS produces MPEG-TS segments (.ts files).
	HLSMuxerVariantMPEGTS HLSMuxerVariant = iota
	// HLSMuxerVariantFMP4 produces fMP4/CMAF segments (.m4s files with init.mp4).
	HLSMuxerVariantFMP4
	// HLSMuxerVariantLowLatency produces low-latency HLS with parts.
	HLSMuxerVariantLowLatency
)

// String returns the string representation of the variant.
func (v HLSMuxerVariant) String() string {
	switch v {
	case HLSMuxerVariantMPEGTS:
		return "mpegts"
	case HLSMuxerVariantFMP4:
		return "fmp4"
	case HLSMuxerVariantLowLatency:
		return "lowlatency"
	default:
		return "unknown"
	}
}

// toGohlslibVariant converts to gohlslib's MuxerVariant.
func (v HLSMuxerVariant) toGohlslibVariant() gohlslib.MuxerVariant {
	switch v {
	case HLSMuxerVariantMPEGTS:
		return gohlslib.MuxerVariantMPEGTS
	case HLSMuxerVariantFMP4:
		return gohlslib.MuxerVariantFMP4
	case HLSMuxerVariantLowLatency:
		return gohlslib.MuxerVariantLowLatency
	default:
		return gohlslib.MuxerVariantMPEGTS
	}
}

// HLSMuxerConfig configures the HLS muxer.
type HLSMuxerConfig struct {
	// Variant specifies the segment container type (MPEG-TS, fMP4, or low-latency).
	Variant HLSMuxerVariant

	// SegmentCount is the number of segments to keep in the playlist.
	// Default: 7
	SegmentCount int

	// SegmentMinDuration is the minimum segment duration.
	// Default: 1 second
	SegmentMinDuration time.Duration

	// PartMinDuration is the minimum part duration for low-latency HLS.
	// Default: 200ms
	PartMinDuration time.Duration

	// SegmentMaxSize is the maximum segment size in bytes.
	// Default: 50MB
	SegmentMaxSize uint64

	// Logger for structured logging.
	Logger *slog.Logger
}

// DefaultHLSMuxerConfig returns a sensible default configuration.
func DefaultHLSMuxerConfig() HLSMuxerConfig {
	return HLSMuxerConfig{
		Variant:            HLSMuxerVariantMPEGTS,
		SegmentCount:       7,
		SegmentMinDuration: 1 * time.Second,
		PartMinDuration:    200 * time.Millisecond,
		SegmentMaxSize:     50 * 1024 * 1024, // 50MB
		Logger:             slog.Default(),
	}
}

// HLSMuxerTrack represents a track to be muxed.
type HLSMuxerTrack struct {
	// Track is the gohlslib track reference (from gohlslib.Client OnTracks callback).
	Track *gohlslib.Track

	// Codec is the codec type for the track.
	Codec codecs.Codec
}

// HLSMuxer wraps gohlslib.Muxer to produce HLS output from track data.
// It receives media data via WriteH264, WriteH265, and WriteMPEG4Audio methods
// and serves HLS playlists and segments via HTTP.
//
// HLSMuxer bridges the gap between:
//   - Input: gohlslib.Client track callbacks (OnDataH26x, OnDataMPEG4Audio)
//   - Output: HLS playlist/segment serving via HTTP
//
// This enables HLS-to-HLS passthrough without FFmpeg when only container
// format changes are needed (e.g., source fMP4 -> client wants MPEG-TS).
type HLSMuxer struct {
	config HLSMuxerConfig
	muxer  *gohlslib.Muxer

	// Track mappings
	mu           sync.RWMutex
	tracks       []*gohlslib.Track
	tracksByType map[string]*gohlslib.Track // "video", "audio"

	// State
	started atomic.Bool
	closed  atomic.Bool
	ctx     context.Context
	cancel  context.CancelFunc

	// Subscriber tracking for connection sharing
	subscribers   atomic.Int32
	lastActivity  atomic.Value // time.Time

	// Metrics
	segmentsProduced atomic.Uint64
	bytesProduced    atomic.Uint64
}

// NewHLSMuxer creates a new HLS muxer with the given configuration.
func NewHLSMuxer(config HLSMuxerConfig) *HLSMuxer {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.SegmentCount == 0 {
		config.SegmentCount = 7
	}
	if config.SegmentMinDuration == 0 {
		config.SegmentMinDuration = 1 * time.Second
	}
	if config.SegmentMaxSize == 0 {
		config.SegmentMaxSize = 50 * 1024 * 1024
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &HLSMuxer{
		config:       config,
		tracksByType: make(map[string]*gohlslib.Track),
		ctx:          ctx,
		cancel:       cancel,
	}
	m.lastActivity.Store(time.Now())

	return m
}

// AddTrack adds a track to the muxer. Must be called before Start().
// Returns the track reference for use in Write* methods.
func (m *HLSMuxer) AddTrack(codec codecs.Codec) (*gohlslib.Track, error) {
	if m.started.Load() {
		return nil, errors.New("cannot add track after muxer started")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	track := &gohlslib.Track{Codec: codec}
	m.tracks = append(m.tracks, track)

	// Track by type for easy lookup
	switch codec.(type) {
	case *codecs.H264, *codecs.H265:
		m.tracksByType["video"] = track
	case *codecs.MPEG4Audio, *codecs.Opus:
		m.tracksByType["audio"] = track
	}

	m.config.Logger.Debug("HLSMuxer: track added",
		slog.String("codec", fmt.Sprintf("%T", codec)),
		slog.Int("track_count", len(m.tracks)),
	)

	return track, nil
}

// Start initializes and starts the muxer. Call this after adding all tracks.
func (m *HLSMuxer) Start() error {
	if m.started.Swap(true) {
		return nil // Already started
	}

	m.mu.RLock()
	trackCount := len(m.tracks)
	m.mu.RUnlock()

	if trackCount == 0 {
		return ErrNoTracksConfigured
	}

	// Create gohlslib Muxer
	m.mu.Lock()
	m.muxer = &gohlslib.Muxer{
		Variant:            m.config.Variant.toGohlslibVariant(),
		SegmentCount:       m.config.SegmentCount,
		SegmentMinDuration: m.config.SegmentMinDuration,
		PartMinDuration:    m.config.PartMinDuration,
		SegmentMaxSize:     m.config.SegmentMaxSize,
		Tracks:             m.tracks,
	}
	m.mu.Unlock()

	// Start the muxer
	if err := m.muxer.Start(); err != nil {
		m.started.Store(false)
		return fmt.Errorf("starting gohlslib muxer: %w", err)
	}

	m.config.Logger.Info("HLSMuxer: started",
		slog.String("variant", m.config.Variant.String()),
		slog.Int("segment_count", m.config.SegmentCount),
		slog.Duration("segment_min_duration", m.config.SegmentMinDuration),
		slog.Int("track_count", trackCount),
	)

	return nil
}

// Close stops the muxer and releases resources.
func (m *HLSMuxer) Close() error {
	if m.closed.Swap(true) {
		return nil // Already closed
	}

	m.cancel()

	m.mu.Lock()
	muxer := m.muxer
	m.muxer = nil
	m.mu.Unlock()

	if muxer != nil {
		muxer.Close()
	}

	m.config.Logger.Info("HLSMuxer: closed",
		slog.Uint64("segments_produced", m.segmentsProduced.Load()),
		slog.Uint64("bytes_produced", m.bytesProduced.Load()),
	)

	return nil
}

// Handle serves HTTP requests for HLS playlists and segments.
// This method delegates to gohlslib.Muxer.Handle().
func (m *HLSMuxer) Handle(w http.ResponseWriter, r *http.Request) {
	if !m.started.Load() || m.closed.Load() {
		http.Error(w, "muxer not available", http.StatusServiceUnavailable)
		return
	}

	m.mu.RLock()
	muxer := m.muxer
	m.mu.RUnlock()

	if muxer == nil {
		http.Error(w, "muxer not available", http.StatusServiceUnavailable)
		return
	}

	m.lastActivity.Store(time.Now())
	muxer.Handle(w, r)
}

// WriteH264 writes H.264 video data to the muxer.
// ntp is the wall clock time, pts is the presentation timestamp (90kHz clock).
// au is the access unit as a slice of NAL units.
func (m *HLSMuxer) WriteH264(track *gohlslib.Track, ntp time.Time, pts int64, au [][]byte) error {
	if !m.started.Load() {
		return ErrHLSMuxerNotStarted
	}
	if m.closed.Load() {
		return ErrHLSMuxerClosed
	}

	m.mu.RLock()
	muxer := m.muxer
	m.mu.RUnlock()

	if muxer == nil {
		return ErrHLSMuxerClosed
	}

	if err := muxer.WriteH264(track, ntp, pts, au); err != nil {
		return fmt.Errorf("writing H264: %w", err)
	}

	m.lastActivity.Store(time.Now())
	return nil
}

// WriteH265 writes H.265/HEVC video data to the muxer.
// ntp is the wall clock time, pts is the presentation timestamp (90kHz clock).
// au is the access unit as a slice of NAL units.
func (m *HLSMuxer) WriteH265(track *gohlslib.Track, ntp time.Time, pts int64, au [][]byte) error {
	if !m.started.Load() {
		return ErrHLSMuxerNotStarted
	}
	if m.closed.Load() {
		return ErrHLSMuxerClosed
	}

	m.mu.RLock()
	muxer := m.muxer
	m.mu.RUnlock()

	if muxer == nil {
		return ErrHLSMuxerClosed
	}

	if err := muxer.WriteH265(track, ntp, pts, au); err != nil {
		return fmt.Errorf("writing H265: %w", err)
	}

	m.lastActivity.Store(time.Now())
	return nil
}

// WriteMPEG4Audio writes AAC audio data to the muxer.
// ntp is the wall clock time, pts is the presentation timestamp (90kHz clock).
// aus is the audio access units.
func (m *HLSMuxer) WriteMPEG4Audio(track *gohlslib.Track, ntp time.Time, pts int64, aus [][]byte) error {
	if !m.started.Load() {
		return ErrHLSMuxerNotStarted
	}
	if m.closed.Load() {
		return ErrHLSMuxerClosed
	}

	m.mu.RLock()
	muxer := m.muxer
	m.mu.RUnlock()

	if muxer == nil {
		return ErrHLSMuxerClosed
	}

	if err := muxer.WriteMPEG4Audio(track, ntp, pts, aus); err != nil {
		return fmt.Errorf("writing MPEG4Audio: %w", err)
	}

	m.lastActivity.Store(time.Now())
	return nil
}

// WriteOpus writes Opus audio data to the muxer.
// ntp is the wall clock time, pts is the presentation timestamp (90kHz clock).
// packets is the Opus packets.
func (m *HLSMuxer) WriteOpus(track *gohlslib.Track, ntp time.Time, pts int64, packets [][]byte) error {
	if !m.started.Load() {
		return ErrHLSMuxerNotStarted
	}
	if m.closed.Load() {
		return ErrHLSMuxerClosed
	}

	m.mu.RLock()
	muxer := m.muxer
	m.mu.RUnlock()

	if muxer == nil {
		return ErrHLSMuxerClosed
	}

	if err := muxer.WriteOpus(track, ntp, pts, packets); err != nil {
		return fmt.Errorf("writing Opus: %w", err)
	}

	m.lastActivity.Store(time.Now())
	return nil
}

// ===== Subscriber Reference Counting =====

// Subscribe increments the subscriber count. Call when a new client connects.
func (m *HLSMuxer) Subscribe() {
	count := m.subscribers.Add(1)
	m.config.Logger.Debug("HLSMuxer: subscriber added",
		slog.Int("count", int(count)),
	)
}

// Unsubscribe decrements the subscriber count. Call when a client disconnects.
func (m *HLSMuxer) Unsubscribe() {
	count := m.subscribers.Add(-1)
	m.config.Logger.Debug("HLSMuxer: subscriber removed",
		slog.Int("count", int(count)),
	)
}

// SubscriberCount returns the current number of subscribers.
func (m *HLSMuxer) SubscriberCount() int {
	return int(m.subscribers.Load())
}

// HasSubscribers returns true if there are active subscribers.
func (m *HLSMuxer) HasSubscribers() bool {
	return m.subscribers.Load() > 0
}

// LastActivity returns the time of the last activity (write or handle).
func (m *HLSMuxer) LastActivity() time.Time {
	if t, ok := m.lastActivity.Load().(time.Time); ok {
		return t
	}
	return time.Time{}
}

// IsIdle returns true if the muxer has no subscribers and has been idle
// for longer than the given duration.
func (m *HLSMuxer) IsIdle(idleTimeout time.Duration) bool {
	if m.HasSubscribers() {
		return false
	}
	return time.Since(m.LastActivity()) > idleTimeout
}

// ===== Status and Metrics =====

// IsStarted returns true if the muxer has been started.
func (m *HLSMuxer) IsStarted() bool {
	return m.started.Load()
}

// IsClosed returns true if the muxer has been closed.
func (m *HLSMuxer) IsClosed() bool {
	return m.closed.Load()
}

// Variant returns the configured variant.
func (m *HLSMuxer) Variant() HLSMuxerVariant {
	return m.config.Variant
}

// GetVideoTrack returns the video track, if configured.
func (m *HLSMuxer) GetVideoTrack() *gohlslib.Track {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tracksByType["video"]
}

// GetAudioTrack returns the audio track, if configured.
func (m *HLSMuxer) GetAudioTrack() *gohlslib.Track {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tracksByType["audio"]
}

// Stats returns muxer statistics.
type HLSMuxerStats struct {
	Variant          string        `json:"variant"`
	Started          bool          `json:"started"`
	Closed           bool          `json:"closed"`
	SubscriberCount  int           `json:"subscriber_count"`
	TrackCount       int           `json:"track_count"`
	SegmentsProduced uint64        `json:"segments_produced"`
	BytesProduced    uint64        `json:"bytes_produced"`
	LastActivity     time.Time     `json:"last_activity"`
	IdleDuration     time.Duration `json:"idle_duration"`
}

// Stats returns current muxer statistics.
func (m *HLSMuxer) Stats() HLSMuxerStats {
	m.mu.RLock()
	trackCount := len(m.tracks)
	m.mu.RUnlock()

	lastActivity := m.LastActivity()
	idleDuration := time.Duration(0)
	if !lastActivity.IsZero() {
		idleDuration = time.Since(lastActivity)
	}

	return HLSMuxerStats{
		Variant:          m.config.Variant.String(),
		Started:          m.started.Load(),
		Closed:           m.closed.Load(),
		SubscriberCount:  m.SubscriberCount(),
		TrackCount:       trackCount,
		SegmentsProduced: m.segmentsProduced.Load(),
		BytesProduced:    m.bytesProduced.Load(),
		LastActivity:     lastActivity,
		IdleDuration:     idleDuration,
	}
}
