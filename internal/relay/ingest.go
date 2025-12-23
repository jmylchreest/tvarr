// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Ingest errors.
var (
	ErrIngestClosed           = errors.New("ingest closed")
	ErrIngestAlreadyStart     = errors.New("ingest already started")
	ErrIngestUnsupportedInput = errors.New("unsupported input format")
	ErrNoVideoTrack           = errors.New("no video track found")
)

// InputFormat represents the type of input stream.
type InputFormat string

const (
	InputFormatHLS    InputFormat = "hls"
	InputFormatDASH   InputFormat = "dash"
	InputFormatMPEGTS InputFormat = "mpegts"
	InputFormatRTSP   InputFormat = "rtsp"
)

// IngestConfig configures the ingest pipeline.
type IngestConfig struct {
	// HTTPClient for upstream connections.
	HTTPClient *http.Client

	// BufferCapacity is the number of samples to buffer per track.
	VideoBufferCapacity int
	AudioBufferCapacity int

	// PlaylistRefreshInterval for HLS/DASH.
	PlaylistRefreshInterval time.Duration

	// SegmentFetchTimeout for individual segment downloads.
	SegmentFetchTimeout time.Duration

	// UserAgent for upstream requests.
	UserAgent string

	// Logger for structured logging.
	Logger *slog.Logger
}

// DefaultIngestConfig returns sensible defaults.
func DefaultIngestConfig() IngestConfig {
	return IngestConfig{
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
		VideoBufferCapacity:     3600, // ~2 minutes at 30fps
		AudioBufferCapacity:     6000, // ~2 minutes at ~47fps AAC
		PlaylistRefreshInterval: 2 * time.Second,
		SegmentFetchTimeout:     10 * time.Second,
		UserAgent:               "tvarr/1.0",
		Logger:                  slog.Default(),
	}
}

// IngestStats provides statistics about the ingest pipeline.
type IngestStats struct {
	Format            InputFormat
	StartedAt         time.Time
	LastActivity      time.Time
	BytesIngested     uint64
	SegmentsFetched   uint64
	VideoSamples      uint64
	AudioSamples      uint64
	Errors            uint64
	CurrentBitrateBps uint64
}

// Ingest represents an ingest pipeline that demuxes input streams to elementary streams.
type Ingest struct {
	config    IngestConfig
	buffer    *SharedESBuffer
	sourceURL string
	format    InputFormat
	channelID string
	proxyID   string

	// Timing
	startedAt    time.Time
	lastActivity atomic.Value // time.Time

	// Stats
	bytesIngested   atomic.Uint64
	segmentsFetched atomic.Uint64
	videoSamples    atomic.Uint64
	audioSamples    atomic.Uint64
	errorCount      atomic.Uint64

	// Bitrate calculation
	bitrateWindow     []bitratePoint
	bitrateWindowMu   sync.Mutex
	currentBitrateBps atomic.Uint64

	// Lifecycle
	ctx      context.Context
	cancel   context.CancelFunc
	started  atomic.Bool
	closed   atomic.Bool
	closedCh chan struct{}
	wg       sync.WaitGroup
}

// bitratePoint records bytes at a point in time for bitrate calculation.
type bitratePoint struct {
	bytes uint64
	time  time.Time
}

// NewIngest creates a new ingest pipeline.
func NewIngest(channelID, proxyID, sourceURL string, format InputFormat, buffer *SharedESBuffer, config IngestConfig) *Ingest {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.HTTPClient == nil {
		config.HTTPClient = DefaultIngestConfig().HTTPClient
	}
	if config.VideoBufferCapacity <= 0 {
		config.VideoBufferCapacity = DefaultIngestConfig().VideoBufferCapacity
	}
	if config.AudioBufferCapacity <= 0 {
		config.AudioBufferCapacity = DefaultIngestConfig().AudioBufferCapacity
	}

	i := &Ingest{
		config:        config,
		buffer:        buffer,
		sourceURL:     sourceURL,
		format:        format,
		channelID:     channelID,
		proxyID:       proxyID,
		closedCh:      make(chan struct{}),
		bitrateWindow: make([]bitratePoint, 0, 60),
	}
	i.lastActivity.Store(time.Now())
	return i
}

// Start begins the ingest pipeline.
func (i *Ingest) Start(ctx context.Context) error {
	if !i.started.CompareAndSwap(false, true) {
		return ErrIngestAlreadyStart
	}

	i.ctx, i.cancel = context.WithCancel(ctx)
	i.startedAt = time.Now()

	i.config.Logger.Info("Starting ingest pipeline",
		slog.String("channel_id", i.channelID),
		slog.String("format", string(i.format)),
		slog.String("url", i.sourceURL))

	i.wg.Add(1)
	go func() {
		defer i.wg.Done()
		i.runIngestLoop()
	}()

	// Start bitrate calculation goroutine
	i.wg.Add(1)
	go func() {
		defer i.wg.Done()
		i.runBitrateCalculator()
	}()

	return nil
}

// Stop stops the ingest pipeline.
func (i *Ingest) Stop() {
	if i.closed.CompareAndSwap(false, true) {
		if i.cancel != nil {
			i.cancel()
		}
		close(i.closedCh)
		i.wg.Wait()

		i.config.Logger.Info("Ingest pipeline stopped",
			slog.String("channel_id", i.channelID))
	}
}

// Stats returns current ingest statistics.
func (i *Ingest) Stats() IngestStats {
	lastActivity, _ := i.lastActivity.Load().(time.Time)
	return IngestStats{
		Format:            i.format,
		StartedAt:         i.startedAt,
		LastActivity:      lastActivity,
		BytesIngested:     i.bytesIngested.Load(),
		SegmentsFetched:   i.segmentsFetched.Load(),
		VideoSamples:      i.videoSamples.Load(),
		AudioSamples:      i.audioSamples.Load(),
		Errors:            i.errorCount.Load(),
		CurrentBitrateBps: i.currentBitrateBps.Load(),
	}
}

// IsClosed returns true if the ingest is closed.
func (i *Ingest) IsClosed() bool {
	return i.closed.Load()
}

// ClosedChan returns a channel that is closed when ingest stops.
func (i *Ingest) ClosedChan() <-chan struct{} {
	return i.closedCh
}

// runIngestLoop runs the main ingest loop based on input format.
func (i *Ingest) runIngestLoop() {
	var err error

	for {
		select {
		case <-i.ctx.Done():
			return
		default:
		}

		switch i.format {
		case InputFormatHLS:
			err = i.runHLSIngest()
		case InputFormatDASH:
			err = i.runDASHIngest()
		case InputFormatMPEGTS:
			err = i.runMPEGTSIngest()
		default:
			i.config.Logger.Error("Unsupported input format",
				slog.String("format", string(i.format)))
			return
		}

		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			i.errorCount.Add(1)
			i.config.Logger.Warn("Ingest error, retrying",
				slog.String("channel_id", i.channelID),
				slog.String("error", err.Error()))

			// Backoff before retry
			select {
			case <-i.ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}
}

// runBitrateCalculator calculates ingress bitrate.
func (i *Ingest) runBitrateCalculator() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-i.ctx.Done():
			return
		case <-ticker.C:
			i.calculateBitrate()
		}
	}
}

// calculateBitrate calculates current bitrate from recent samples.
func (i *Ingest) calculateBitrate() {
	now := time.Now()
	currentBytes := i.bytesIngested.Load()

	i.bitrateWindowMu.Lock()
	defer i.bitrateWindowMu.Unlock()

	// Add current point
	i.bitrateWindow = append(i.bitrateWindow, bitratePoint{
		bytes: currentBytes,
		time:  now,
	})

	// Remove points older than 10 seconds
	cutoff := now.Add(-10 * time.Second)
	validStart := 0
	for j, p := range i.bitrateWindow {
		if p.time.After(cutoff) {
			validStart = j
			break
		}
	}
	if validStart > 0 {
		// Copy to new slice to release capacity from trimmed elements
		// This prevents unbounded capacity growth from repeated append+reslice
		remaining := len(i.bitrateWindow) - validStart
		newWindow := make([]bitratePoint, remaining, max(remaining*2, 60))
		copy(newWindow, i.bitrateWindow[validStart:])
		i.bitrateWindow = newWindow
	}

	// Calculate bitrate from window
	if len(i.bitrateWindow) >= 2 {
		first := i.bitrateWindow[0]
		last := i.bitrateWindow[len(i.bitrateWindow)-1]
		duration := last.time.Sub(first.time).Seconds()
		if duration > 0 {
			bytesDiff := last.bytes - first.bytes
			bps := uint64(float64(bytesDiff*8) / duration)
			i.currentBitrateBps.Store(bps)
		}
	}
}

// recordActivity updates last activity time.
func (i *Ingest) recordActivity() {
	i.lastActivity.Store(time.Now())
}

// runHLSIngest ingests from an HLS source.
func (i *Ingest) runHLSIngest() error {
	i.config.Logger.Debug("Starting HLS ingest",
		slog.String("url", i.sourceURL))

	// Create HLS demuxer
	demuxer := NewHLSDemuxer(i.sourceURL, i.buffer, HLSDemuxerConfig{
		HTTPClient:              i.config.HTTPClient,
		PlaylistRefreshInterval: i.config.PlaylistRefreshInterval,
		SegmentFetchTimeout:     i.config.SegmentFetchTimeout,
		UserAgent:               i.config.UserAgent,
		Logger:                  i.config.Logger,
		OnVideoSample: func(pts, dts int64, data []byte, isKeyframe bool) {
			i.videoSamples.Add(1)
			i.bytesIngested.Add(uint64(len(data)))
			i.recordActivity()
		},
		OnAudioSample: func(pts int64, data []byte) {
			i.audioSamples.Add(1)
			i.bytesIngested.Add(uint64(len(data)))
			i.recordActivity()
		},
		OnSegmentFetched: func() {
			i.segmentsFetched.Add(1)
		},
	})

	return demuxer.Run(i.ctx)
}

// runDASHIngest ingests from a DASH source.
func (i *Ingest) runDASHIngest() error {
	i.config.Logger.Debug("Starting DASH ingest",
		slog.String("url", i.sourceURL))

	// DASH ingest uses similar approach to HLS but with MPD parsing
	// For now, return unsupported - DASH will be added in future
	return fmt.Errorf("DASH ingest: %w", ErrIngestUnsupportedInput)
}

// runMPEGTSIngest ingests from a raw MPEG-TS source.
func (i *Ingest) runMPEGTSIngest() error {
	i.config.Logger.Debug("Starting MPEG-TS ingest",
		slog.String("url", i.sourceURL))

	// Create MPEG-TS demuxer
	demuxer := NewTSDemuxer(i.buffer, TSDemuxerConfig{
		Logger: i.config.Logger,
		OnVideoSample: func(pts, dts int64, data []byte, isKeyframe bool) {
			i.videoSamples.Add(1)
			i.bytesIngested.Add(uint64(len(data)))
			i.recordActivity()
		},
		OnAudioSample: func(pts int64, data []byte) {
			i.audioSamples.Add(1)
			i.bytesIngested.Add(uint64(len(data)))
			i.recordActivity()
		},
	})

	// Fetch and stream the MPEG-TS
	req, err := http.NewRequestWithContext(i.ctx, http.MethodGet, i.sourceURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	if i.config.UserAgent != "" {
		req.Header.Set("User-Agent", i.config.UserAgent)
	}

	resp, err := i.config.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching stream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// Stream data through demuxer
	buf := make([]byte, 188*100) // Read in multiples of TS packet size
	for {
		select {
		case <-i.ctx.Done():
			return i.ctx.Err()
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			if writeErr := demuxer.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf("demuxer write: %w", writeErr)
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("reading stream: %w", err)
		}
	}
}
