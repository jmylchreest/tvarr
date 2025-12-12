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

// HLSRepackager errors.
var (
	ErrRepackagerClosed  = errors.New("repackager closed")
	ErrRepackagerStarted = errors.New("repackager already started")
)

// HLSRepackagerConfig configures the HLS repackager.
type HLSRepackagerConfig struct {
	// SourceURL is the upstream HLS playlist URL.
	SourceURL string

	// OutputVariant specifies the output segment container type.
	OutputVariant HLSMuxerVariant

	// SegmentCount is the number of segments to keep in output playlist.
	// Default: 7
	SegmentCount int

	// SegmentMinDuration is the minimum segment duration.
	// Default: 1 second
	SegmentMinDuration time.Duration

	// HTTPClient for fetching the upstream HLS stream.
	HTTPClient *http.Client

	// Logger for structured logging.
	Logger *slog.Logger
}

// HLSRepackager converts an HLS stream to HLS with a different segment format.
// It bridges gohlslib.Client (input) to HLSMuxer (output).
//
// Use cases:
//   - Convert HLS with MPEG-TS segments to HLS with fMP4 segments
//   - Convert HLS with fMP4 segments to HLS with MPEG-TS segments
//   - Convert source HLS to low-latency HLS
//
// This enables efficient HLS-to-HLS repackaging without FFmpeg.
type HLSRepackager struct {
	config HLSRepackagerConfig

	// Input (gohlslib.Client)
	client *gohlslib.Client

	// Output (HLSMuxer)
	muxer *HLSMuxer

	// Track mappings from client to muxer
	mu         sync.RWMutex
	videoTrack *gohlslib.Track
	audioTrack *gohlslib.Track

	// State
	started atomic.Bool
	closed  atomic.Bool
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	errMu   sync.Mutex

	// Metrics
	framesProcessed atomic.Uint64
	bytesProcessed  atomic.Uint64
	startTime       time.Time
}

// NewHLSRepackager creates a new HLS repackager.
func NewHLSRepackager(config HLSRepackagerConfig) *HLSRepackager {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	if config.SegmentCount == 0 {
		config.SegmentCount = 7
	}
	if config.SegmentMinDuration == 0 {
		config.SegmentMinDuration = 1 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &HLSRepackager{
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start begins the repackaging process.
func (r *HLSRepackager) Start() error {
	if r.started.Swap(true) {
		return ErrRepackagerStarted
	}

	r.startTime = time.Now()

	// Create the output muxer (tracks will be added when discovered)
	r.muxer = NewHLSMuxer(HLSMuxerConfig{
		Variant:            r.config.OutputVariant,
		SegmentCount:       r.config.SegmentCount,
		SegmentMinDuration: r.config.SegmentMinDuration,
		Logger:             r.config.Logger,
	})

	// Create the gohlslib client for input
	r.client = &gohlslib.Client{
		URI:        r.config.SourceURL,
		HTTPClient: r.config.HTTPClient,
		OnTracks:   r.onTracks,
	}

	// Start the client
	if err := r.client.Start(); err != nil {
		return fmt.Errorf("starting HLS client: %w", err)
	}

	r.config.Logger.Info("HLSRepackager: started",
		slog.String("source_url", r.config.SourceURL),
		slog.String("output_variant", r.config.OutputVariant.String()),
	)

	// Monitor client in background
	go r.watchClient()

	return nil
}

// onTracks is called when the gohlslib client discovers tracks.
// This bridges the input tracks to the output muxer.
func (r *HLSRepackager) onTracks(tracks []*gohlslib.Track) error {
	r.config.Logger.Debug("HLSRepackager: discovered tracks",
		slog.Int("count", len(tracks)),
	)

	for _, track := range tracks {
		switch codec := track.Codec.(type) {
		case *codecs.H264:
			r.config.Logger.Debug("HLSRepackager: H264 track found",
				slog.Int("sps_len", len(codec.SPS)),
				slog.Int("pps_len", len(codec.PPS)),
			)

			// Add track to muxer
			muxerTrack, err := r.muxer.AddTrack(codec)
			if err != nil {
				return fmt.Errorf("adding H264 track to muxer: %w", err)
			}

			r.mu.Lock()
			r.videoTrack = muxerTrack
			r.mu.Unlock()

			// Register data callback
			r.client.OnDataH26x(track, r.onH264Data)

		case *codecs.H265:
			r.config.Logger.Debug("HLSRepackager: H265 track found",
				slog.Int("vps_len", len(codec.VPS)),
				slog.Int("sps_len", len(codec.SPS)),
				slog.Int("pps_len", len(codec.PPS)),
			)

			// Add track to muxer
			muxerTrack, err := r.muxer.AddTrack(codec)
			if err != nil {
				return fmt.Errorf("adding H265 track to muxer: %w", err)
			}

			r.mu.Lock()
			r.videoTrack = muxerTrack
			r.mu.Unlock()

			// Register data callback
			r.client.OnDataH26x(track, r.onH265Data)

		case *codecs.MPEG4Audio:
			r.config.Logger.Debug("HLSRepackager: MPEG4Audio track found")

			// Add track to muxer
			muxerTrack, err := r.muxer.AddTrack(codec)
			if err != nil {
				return fmt.Errorf("adding MPEG4Audio track to muxer: %w", err)
			}

			r.mu.Lock()
			r.audioTrack = muxerTrack
			r.mu.Unlock()

			// Register data callback
			r.client.OnDataMPEG4Audio(track, r.onMPEG4AudioData)

		case *codecs.Opus:
			r.config.Logger.Debug("HLSRepackager: Opus track found")

			// Add track to muxer
			muxerTrack, err := r.muxer.AddTrack(codec)
			if err != nil {
				return fmt.Errorf("adding Opus track to muxer: %w", err)
			}

			r.mu.Lock()
			r.audioTrack = muxerTrack
			r.mu.Unlock()

			// Register data callback
			r.client.OnDataOpus(track, r.onOpusData)

		default:
			r.config.Logger.Warn("HLSRepackager: unsupported codec",
				slog.String("type", fmt.Sprintf("%T", codec)),
			)
		}
	}

	// Start the muxer now that tracks are configured
	if err := r.muxer.Start(); err != nil {
		return fmt.Errorf("starting HLS muxer: %w", err)
	}

	return nil
}

// onH264Data handles H264 video data from the client.
func (r *HLSRepackager) onH264Data(pts int64, dts int64, au [][]byte) {
	if r.closed.Load() {
		return
	}

	r.mu.RLock()
	track := r.videoTrack
	r.mu.RUnlock()

	if track == nil {
		return
	}

	// Convert PTS to NTP time (90kHz clock to nanoseconds)
	ntp := time.Now() // Use wall clock for NTP

	if err := r.muxer.WriteH264(track, ntp, pts, au); err != nil {
		r.config.Logger.Error("HLSRepackager: H264 write error",
			slog.String("error", err.Error()),
		)
		return
	}

	r.framesProcessed.Add(1)
	for _, nalu := range au {
		r.bytesProcessed.Add(uint64(len(nalu)))
	}
}

// onH265Data handles H265 video data from the client.
func (r *HLSRepackager) onH265Data(pts int64, dts int64, au [][]byte) {
	if r.closed.Load() {
		return
	}

	r.mu.RLock()
	track := r.videoTrack
	r.mu.RUnlock()

	if track == nil {
		return
	}

	ntp := time.Now()

	if err := r.muxer.WriteH265(track, ntp, pts, au); err != nil {
		r.config.Logger.Error("HLSRepackager: H265 write error",
			slog.String("error", err.Error()),
		)
		return
	}

	r.framesProcessed.Add(1)
	for _, nalu := range au {
		r.bytesProcessed.Add(uint64(len(nalu)))
	}
}

// onMPEG4AudioData handles MPEG4 audio data from the client.
func (r *HLSRepackager) onMPEG4AudioData(pts int64, aus [][]byte) {
	if r.closed.Load() {
		return
	}

	r.mu.RLock()
	track := r.audioTrack
	r.mu.RUnlock()

	if track == nil {
		return
	}

	ntp := time.Now()

	if err := r.muxer.WriteMPEG4Audio(track, ntp, pts, aus); err != nil {
		r.config.Logger.Error("HLSRepackager: MPEG4Audio write error",
			slog.String("error", err.Error()),
		)
		return
	}

	for _, au := range aus {
		r.bytesProcessed.Add(uint64(len(au)))
	}
}

// onOpusData handles Opus audio data from the client.
func (r *HLSRepackager) onOpusData(pts int64, packets [][]byte) {
	if r.closed.Load() {
		return
	}

	r.mu.RLock()
	track := r.audioTrack
	r.mu.RUnlock()

	if track == nil {
		return
	}

	ntp := time.Now()

	if err := r.muxer.WriteOpus(track, ntp, pts, packets); err != nil {
		r.config.Logger.Error("HLSRepackager: Opus write error",
			slog.String("error", err.Error()),
		)
		return
	}

	for _, pkt := range packets {
		r.bytesProcessed.Add(uint64(len(pkt)))
	}
}

// watchClient monitors the gohlslib client and handles errors.
func (r *HLSRepackager) watchClient() {
	err := r.client.Wait2()
	if err != nil && !errors.Is(err, gohlslib.ErrClientEOS) && !errors.Is(err, context.Canceled) {
		r.setError(fmt.Errorf("client error: %w", err))
	}

	// Close the repackager when client stops
	r.Close()
}

// Handle serves HTTP requests for HLS playlists and segments.
// This delegates to the internal muxer.
func (r *HLSRepackager) Handle(w http.ResponseWriter, req *http.Request) {
	if !r.started.Load() {
		http.Error(w, "repackager not started", http.StatusServiceUnavailable)
		return
	}
	if r.closed.Load() {
		http.Error(w, "repackager closed", http.StatusServiceUnavailable)
		return
	}

	r.mu.RLock()
	muxer := r.muxer
	r.mu.RUnlock()

	if muxer == nil {
		http.Error(w, "muxer not available", http.StatusServiceUnavailable)
		return
	}

	muxer.Handle(w, req)
}

// Close stops the repackager and releases resources.
func (r *HLSRepackager) Close() error {
	if r.closed.Swap(true) {
		return nil // Already closed
	}

	r.cancel()

	// Close the client
	if r.client != nil {
		r.client.Close()
	}

	// Close the muxer
	r.mu.Lock()
	muxer := r.muxer
	r.muxer = nil
	r.mu.Unlock()

	if muxer != nil {
		muxer.Close()
	}

	r.config.Logger.Info("HLSRepackager: closed",
		slog.Uint64("frames_processed", r.framesProcessed.Load()),
		slog.Uint64("bytes_processed", r.bytesProcessed.Load()),
		slog.Duration("duration", time.Since(r.startTime)),
	)

	return nil
}

// setError sets the error that caused the repackager to fail.
func (r *HLSRepackager) setError(err error) {
	r.errMu.Lock()
	defer r.errMu.Unlock()
	if r.err == nil {
		r.err = err
	}
}

// Error returns the error that caused the repackager to fail, if any.
func (r *HLSRepackager) Error() error {
	r.errMu.Lock()
	defer r.errMu.Unlock()
	return r.err
}

// IsStarted returns true if the repackager has been started.
func (r *HLSRepackager) IsStarted() bool {
	return r.started.Load()
}

// IsClosed returns true if the repackager has been closed.
func (r *HLSRepackager) IsClosed() bool {
	return r.closed.Load()
}

// GetMuxer returns the internal HLSMuxer for direct access.
// Use this for subscriber management.
func (r *HLSRepackager) GetMuxer() *HLSMuxer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.muxer
}

// Subscribe adds a subscriber to the muxer.
func (r *HLSRepackager) Subscribe() {
	if muxer := r.GetMuxer(); muxer != nil {
		muxer.Subscribe()
	}
}

// Unsubscribe removes a subscriber from the muxer.
func (r *HLSRepackager) Unsubscribe() {
	if muxer := r.GetMuxer(); muxer != nil {
		muxer.Unsubscribe()
	}
}

// SubscriberCount returns the number of subscribers.
func (r *HLSRepackager) SubscriberCount() int {
	if muxer := r.GetMuxer(); muxer != nil {
		return muxer.SubscriberCount()
	}
	return 0
}

// HasSubscribers returns true if there are active subscribers.
func (r *HLSRepackager) HasSubscribers() bool {
	if muxer := r.GetMuxer(); muxer != nil {
		return muxer.HasSubscribers()
	}
	return false
}

// Stats returns repackager statistics.
type HLSRepackagerStats struct {
	SourceURL       string         `json:"source_url"`
	OutputVariant   string         `json:"output_variant"`
	Started         bool           `json:"started"`
	Closed          bool           `json:"closed"`
	FramesProcessed uint64         `json:"frames_processed"`
	BytesProcessed  uint64         `json:"bytes_processed"`
	Duration        time.Duration  `json:"duration"`
	SubscriberCount int            `json:"subscriber_count"`
	MuxerStats      *HLSMuxerStats `json:"muxer_stats,omitempty"`
}

// Stats returns current repackager statistics.
func (r *HLSRepackager) Stats() HLSRepackagerStats {
	stats := HLSRepackagerStats{
		SourceURL:       r.config.SourceURL,
		OutputVariant:   r.config.OutputVariant.String(),
		Started:         r.started.Load(),
		Closed:          r.closed.Load(),
		FramesProcessed: r.framesProcessed.Load(),
		BytesProcessed:  r.bytesProcessed.Load(),
		Duration:        time.Since(r.startTime),
		SubscriberCount: r.SubscriberCount(),
	}

	if muxer := r.GetMuxer(); muxer != nil {
		muxerStats := muxer.Stats()
		stats.MuxerStats = &muxerStats
	}

	return stats
}
