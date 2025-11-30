package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/gohlslib/v2/pkg/playlist"
	"github.com/google/uuid"
)

// StreamMode represents the classification of a stream.
type StreamMode int

const (
	// StreamModePassthroughRawTS is a direct raw MPEG-TS stream.
	StreamModePassthroughRawTS StreamMode = iota
	// StreamModeCollapsedHLS is an HLS stream that can be collapsed to continuous TS.
	StreamModeCollapsedHLS
	// StreamModeTransparentHLS is an HLS stream that must be passed through (multi-variant, encrypted, etc.).
	StreamModeTransparentHLS
	// StreamModeUnknown is an unknown stream type.
	StreamModeUnknown
)

func (m StreamMode) String() string {
	switch m {
	case StreamModePassthroughRawTS:
		return "passthrough-raw-ts"
	case StreamModeCollapsedHLS:
		return "collapsed-hls"
	case StreamModeTransparentHLS:
		return "transparent-hls"
	default:
		return "unknown"
	}
}

// ErrCollapserClosed is returned when the collapser is closed.
var ErrCollapserClosed = errors.New("collapser closed")

// ErrCollapserAborted is returned when the collapser aborts due to errors.
var ErrCollapserAborted = errors.New("collapser aborted")

// ClassificationResult holds the result of stream classification.
type ClassificationResult struct {
	Mode                  StreamMode
	VariantCount          int
	TargetDuration        float64
	IsEncrypted           bool
	UsesFMP4              bool
	EligibleForCollapse   bool
	SelectedMediaPlaylist string
	SelectedBandwidth     int64
	Reasons               []string
}

// CollapserConfig holds configuration for the HLS collapser.
type CollapserConfig struct {
	// ChannelBuffer is the buffer size for segment chunks.
	ChannelBuffer int
	// PlaylistTimeout is the timeout for playlist fetches.
	PlaylistTimeout time.Duration
	// SegmentTimeout is the timeout for segment fetches.
	SegmentTimeout time.Duration
	// MaxPlaylistBytes is the maximum playlist size to fetch.
	MaxPlaylistBytes int
	// MaxPlaylistErrors is max consecutive playlist errors before abort.
	MaxPlaylistErrors int
	// MaxSegmentErrors is max consecutive segment errors before abort.
	MaxSegmentErrors int
	// MinPollInterval is the minimum time between playlist polls.
	MinPollInterval time.Duration
}

// DefaultCollapserConfig returns sensible defaults for HLS collapsing.
func DefaultCollapserConfig() CollapserConfig {
	return CollapserConfig{
		ChannelBuffer:     4,
		PlaylistTimeout:   5 * time.Second,
		SegmentTimeout:    10 * time.Second,
		MaxPlaylistBytes:  256 * 1024,
		MaxPlaylistErrors: 6,
		MaxSegmentErrors:  6,
		MinPollInterval:   800 * time.Millisecond,
	}
}

// HLSCollapser converts an HLS stream to a continuous MPEG-TS stream.
type HLSCollapser struct {
	config      CollapserConfig
	client      *http.Client
	playlistURL string
	sessionID   string

	targetDuration float64

	mu       sync.Mutex
	closed   bool
	shutdown atomic.Bool
	started  atomic.Bool

	chunkCh chan []byte
	errCh   chan error
}

// NewHLSCollapser creates a new HLS collapser for the given playlist URL.
func NewHLSCollapser(client *http.Client, playlistURL string, targetDuration float64, config CollapserConfig) *HLSCollapser {
	if targetDuration <= 0 {
		targetDuration = 6.0 // default fallback
	}

	return &HLSCollapser{
		config:         config,
		client:         client,
		playlistURL:    playlistURL,
		sessionID:      uuid.New().String(),
		targetDuration: targetDuration,
		chunkCh:        make(chan []byte, config.ChannelBuffer),
		errCh:          make(chan error, 1),
	}
}

// Start begins the collapsing process. Call this before reading.
func (c *HLSCollapser) Start(ctx context.Context) {
	if c.started.Swap(true) {
		return // already started
	}
	go c.runLoop(ctx)
}

// Read implements io.Reader. Start must be called first.
func (c *HLSCollapser) Read(p []byte) (int, error) {
	return c.ReadContext(context.Background(), p)
}

// ReadContext reads with context support.
func (c *HLSCollapser) ReadContext(ctx context.Context, p []byte) (int, error) {
	select {
	case chunk, ok := <-c.chunkCh:
		if !ok {
			// Channel closed, check for error
			select {
			case err := <-c.errCh:
				return 0, err
			default:
				return 0, io.EOF
			}
		}
		n := copy(p, chunk)
		// If chunk is larger than buffer, we lose data. In practice,
		// callers should use larger buffers or wrap in a buffered reader.
		return n, nil
	case err := <-c.errCh:
		return 0, err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// Stop signals the collapser to stop.
func (c *HLSCollapser) Stop() {
	c.shutdown.Store(true)
}

// IsClosed returns true if the collapser is closed.
func (c *HLSCollapser) IsClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// SessionID returns the session ID for logging.
func (c *HLSCollapser) SessionID() string {
	return c.sessionID
}

// runLoop is the main collapsing loop.
func (c *HLSCollapser) runLoop(ctx context.Context) {
	defer func() {
		c.mu.Lock()
		c.closed = true
		close(c.chunkCh)
		c.mu.Unlock()
	}()

	seenSegments := make(map[string]struct{})
	seenSequences := make(map[uint64]struct{})
	var playlistErrors, segmentErrors int
	targetDuration := c.targetDuration

	for !c.shutdown.Load() {
		select {
		case <-ctx.Done():
			c.sendError(ctx.Err())
			return
		default:
		}

		fetchStart := time.Now()

		// Fetch playlist
		playlistBytes, err := c.fetchPlaylist(ctx)
		if err != nil {
			playlistErrors++
			if playlistErrors >= c.config.MaxPlaylistErrors {
				c.sendError(fmt.Errorf("playlist fetch failed after %d attempts: %w", playlistErrors, err))
				return
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}
		playlistErrors = 0

		// Parse playlist using gohlslib
		parsed, err := unmarshalMediaPlaylist(playlistBytes)
		if err != nil {
			playlistErrors++
			if playlistErrors >= c.config.MaxPlaylistErrors {
				c.sendError(fmt.Errorf("playlist parse failed after %d attempts: %w", playlistErrors, err))
				return
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}

		if parsed.TargetDuration > 0 {
			targetDuration = float64(parsed.TargetDuration)
		}

		// Emit new segments
		var emittedAny bool
		mediaSequence := uint64(parsed.MediaSequence)

		for i, seg := range parsed.Segments {
			if c.shutdown.Load() {
				break
			}

			if seg == nil {
				continue
			}

			// Dedup by sequence or URL
			var alreadySeen bool
			seq := mediaSequence + uint64(i)
			if _, ok := seenSequences[seq]; ok {
				alreadySeen = true
			} else {
				seenSequences[seq] = struct{}{}
				// Also track by URL for safety
				if _, ok := seenSegments[seg.URI]; ok {
					alreadySeen = true
				} else {
					seenSegments[seg.URI] = struct{}{}
				}
			}

			if alreadySeen {
				continue
			}

			// Fetch segment
			absoluteURL := absolutizeURL(c.playlistURL, seg.URI)
			data, err := c.fetchSegment(ctx, absoluteURL)
			if err != nil {
				segmentErrors++
				if segmentErrors >= c.config.MaxSegmentErrors {
					c.sendError(fmt.Errorf("segment fetch failed after %d attempts: %w", segmentErrors, err))
					return
				}
				continue
			}
			segmentErrors = 0
			emittedAny = true

			// Send to channel
			select {
			case c.chunkCh <- data:
			case <-ctx.Done():
				c.sendError(ctx.Err())
				return
			}
		}

		if c.shutdown.Load() {
			break
		}

		// Calculate poll interval
		elapsed := time.Since(fetchStart)
		intervalMs := targetDuration * 1000 * 0.5
		if intervalMs < 800 {
			intervalMs = 800
		}
		if intervalMs > targetDuration*1000 {
			intervalMs = targetDuration * 1000
		}
		if !emittedAny {
			// Add jitter when no new segments
			intervalMs *= 0.85
			if intervalMs < 700 {
				intervalMs = 700
			}
		}

		interval := time.Duration(intervalMs) * time.Millisecond
		if interval > elapsed && interval >= c.config.MinPollInterval {
			select {
			case <-time.After(interval - elapsed):
			case <-ctx.Done():
				c.sendError(ctx.Err())
				return
			}
		}
	}

	if c.shutdown.Load() {
		c.sendError(ErrCollapserAborted)
	}
}

// sendError sends an error to the error channel (non-blocking).
func (c *HLSCollapser) sendError(err error) {
	select {
	case c.errCh <- err:
	default:
	}
}

// fetchPlaylist fetches the playlist bytes.
func (c *HLSCollapser) fetchPlaylist(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.config.PlaylistTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.playlistURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Read with size limit
	limited := io.LimitReader(resp.Body, int64(c.config.MaxPlaylistBytes))
	return io.ReadAll(limited)
}

// fetchSegment fetches a segment's bytes.
func (c *HLSCollapser) fetchSegment(ctx context.Context, segURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.config.SegmentTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, segURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// unmarshalMediaPlaylist parses bytes into a Media playlist using gohlslib.
func unmarshalMediaPlaylist(data []byte) (*playlist.Media, error) {
	pl, err := playlist.Unmarshal(data)
	if err != nil {
		return nil, err
	}

	media, ok := pl.(*playlist.Media)
	if !ok {
		return nil, fmt.Errorf("expected media playlist, got multivariant")
	}

	return media, nil
}

// unmarshalMultivariantPlaylist parses bytes into a Multivariant playlist using gohlslib.
func unmarshalMultivariantPlaylist(data []byte) (*playlist.Multivariant, error) {
	pl, err := playlist.Unmarshal(data)
	if err != nil {
		return nil, err
	}

	mv, ok := pl.(*playlist.Multivariant)
	if !ok {
		return nil, fmt.Errorf("expected multivariant playlist, got media")
	}

	return mv, nil
}

// absolutizeURL converts a relative URL to absolute based on the playlist URL.
func absolutizeURL(playlistURL, segmentURL string) string {
	if strings.HasPrefix(segmentURL, "http://") || strings.HasPrefix(segmentURL, "https://") {
		return segmentURL
	}

	base, err := url.Parse(playlistURL)
	if err != nil {
		// Fallback: simple string manipulation
		if idx := strings.LastIndex(playlistURL, "/"); idx >= 0 {
			return playlistURL[:idx+1] + segmentURL
		}
		return segmentURL
	}

	ref, err := url.Parse(segmentURL)
	if err != nil {
		return segmentURL
	}

	return base.ResolveReference(ref).String()
}

// StreamClassifier classifies streams to determine optimal relay mode.
type StreamClassifier struct {
	client           *http.Client
	timeout          time.Duration
	maxPlaylistBytes int
}

// NewStreamClassifier creates a new stream classifier.
func NewStreamClassifier(client *http.Client) *StreamClassifier {
	return &StreamClassifier{
		client:           client,
		timeout:          6 * time.Second,
		maxPlaylistBytes: 256 * 1024,
	}
}

// Classify classifies a stream URL to determine the optimal relay mode.
func (c *StreamClassifier) Classify(ctx context.Context, streamURL string) ClassificationResult {
	result := ClassificationResult{
		Mode:    StreamModeUnknown,
		Reasons: []string{},
	}

	// Parse URL and check extension
	parsed, err := url.Parse(streamURL)
	if err != nil {
		result.Reasons = append(result.Reasons, fmt.Sprintf("Invalid URL: %v", err))
		return result
	}

	path := strings.ToLower(parsed.Path)

	// Check for raw TS
	if strings.HasSuffix(path, ".ts") {
		result.Mode = StreamModePassthroughRawTS
		result.Reasons = append(result.Reasons, "Extension .ts indicates raw MPEG-TS stream")
		return result
	}

	// Check for progressive containers (not collapsible)
	if strings.HasSuffix(path, ".mp4") || strings.HasSuffix(path, ".mkv") ||
		strings.HasSuffix(path, ".mov") || strings.HasSuffix(path, ".avi") {
		result.Mode = StreamModeUnknown
		result.Reasons = append(result.Reasons, "Progressive container detected")
		return result
	}

	// Check for HLS
	if !strings.HasSuffix(path, ".m3u8") && !strings.HasSuffix(path, ".m3u") {
		result.Mode = StreamModeUnknown
		result.Reasons = append(result.Reasons, "Path does not indicate HLS playlist")
		return result
	}

	// Fetch and analyze playlist
	playlistBytes, err := c.fetchPlaylist(ctx, streamURL)
	if err != nil {
		result.Reasons = append(result.Reasons, fmt.Sprintf("Failed to fetch playlist: %v", err))
		return result
	}

	// Try to parse as either type using gohlslib
	pl, err := playlist.Unmarshal(playlistBytes)
	if err != nil {
		result.Reasons = append(result.Reasons, fmt.Sprintf("Failed to parse playlist: %v", err))
		return result
	}

	switch p := pl.(type) {
	case *playlist.Multivariant:
		return c.classifyMultivariant(ctx, streamURL, p, &result)
	case *playlist.Media:
		return c.classifyMedia(p, &result)
	default:
		result.Reasons = append(result.Reasons, "Unknown playlist type")
		return result
	}
}

// classifyMultivariant classifies a multivariant (master) playlist.
func (c *StreamClassifier) classifyMultivariant(ctx context.Context, baseURL string, mv *playlist.Multivariant, result *ClassificationResult) ClassificationResult {
	result.VariantCount = len(mv.Variants)
	result.Reasons = append(result.Reasons, fmt.Sprintf("Multivariant playlist with %d variant(s)", len(mv.Variants)))

	if len(mv.Variants) == 0 {
		result.Mode = StreamModeTransparentHLS
		result.Reasons = append(result.Reasons, "No variants in multivariant playlist")
		return *result
	}

	// Sort variants by bandwidth (highest first) for quality selection
	variants := make([]*playlist.MultivariantVariant, len(mv.Variants))
	copy(variants, mv.Variants)
	sort.Slice(variants, func(i, j int) bool {
		return variants[i].Bandwidth > variants[j].Bandwidth
	})

	// Try to find a collapsible variant
	for _, variant := range variants {
		variantURL := absolutizeURL(baseURL, variant.URI)
		variantBytes, err := c.fetchPlaylist(ctx, variantURL)
		if err != nil {
			result.Reasons = append(result.Reasons, fmt.Sprintf("Failed to fetch variant %s: %v", variant.URI, err))
			continue
		}

		variantPL, err := playlist.Unmarshal(variantBytes)
		if err != nil {
			result.Reasons = append(result.Reasons, fmt.Sprintf("Failed to parse variant: %v", err))
			continue
		}

		media, ok := variantPL.(*playlist.Media)
		if !ok {
			result.Reasons = append(result.Reasons, "Variant is not a media playlist")
			continue
		}

		// Check if variant is eligible for collapsing
		analysis := c.analyzeMediaPlaylist(media)
		if analysis.IsEncrypted {
			result.Reasons = append(result.Reasons, fmt.Sprintf("Variant encrypted (bandwidth=%d)", variant.Bandwidth))
			continue
		}
		if analysis.UsesFMP4 {
			result.Reasons = append(result.Reasons, fmt.Sprintf("Variant uses fMP4 (bandwidth=%d)", variant.Bandwidth))
			continue
		}
		if !analysis.AllSegmentsTS {
			result.Reasons = append(result.Reasons, fmt.Sprintf("Variant has non-TS segments (bandwidth=%d)", variant.Bandwidth))
			continue
		}
		if analysis.SegmentCount == 0 {
			result.Reasons = append(result.Reasons, fmt.Sprintf("Variant has no segments (bandwidth=%d)", variant.Bandwidth))
			continue
		}

		// Found collapsible variant
		result.Mode = StreamModeCollapsedHLS
		result.EligibleForCollapse = true
		result.SelectedMediaPlaylist = variantURL
		result.SelectedBandwidth = int64(variant.Bandwidth)
		result.TargetDuration = float64(media.TargetDuration)
		result.IsEncrypted = analysis.IsEncrypted
		result.UsesFMP4 = analysis.UsesFMP4
		result.Reasons = append(result.Reasons, fmt.Sprintf("Selected variant for collapsing (bandwidth=%d)", variant.Bandwidth))
		return *result
	}

	// No collapsible variant found
	result.Mode = StreamModeTransparentHLS
	result.Reasons = append(result.Reasons, "No collapsible variant found in multivariant playlist")
	return *result
}

// classifyMedia classifies a media playlist.
func (c *StreamClassifier) classifyMedia(media *playlist.Media, result *ClassificationResult) ClassificationResult {
	result.VariantCount = 1
	result.TargetDuration = float64(media.TargetDuration)
	result.Reasons = append(result.Reasons, fmt.Sprintf("Media playlist with %d segment(s)", len(media.Segments)))

	analysis := c.analyzeMediaPlaylist(media)
	result.IsEncrypted = analysis.IsEncrypted
	result.UsesFMP4 = analysis.UsesFMP4

	if analysis.IsEncrypted {
		result.Mode = StreamModeTransparentHLS
		result.Reasons = append(result.Reasons, "Encrypted media playlist")
		return *result
	}
	if analysis.UsesFMP4 {
		result.Mode = StreamModeTransparentHLS
		result.Reasons = append(result.Reasons, "fMP4 segments detected")
		return *result
	}
	if !analysis.AllSegmentsTS {
		result.Mode = StreamModeTransparentHLS
		result.Reasons = append(result.Reasons, "Not all segments are .ts")
		return *result
	}
	if analysis.SegmentCount == 0 {
		result.Mode = StreamModeTransparentHLS
		result.Reasons = append(result.Reasons, "No segments in playlist")
		return *result
	}

	// Collapsible!
	result.Mode = StreamModeCollapsedHLS
	result.EligibleForCollapse = true
	result.Reasons = append(result.Reasons, "Eligible single-variant TS media playlist")
	return *result
}

// MediaPlaylistAnalysis holds analysis results for a media playlist.
type MediaPlaylistAnalysis struct {
	IsEncrypted   bool
	UsesFMP4      bool
	AllSegmentsTS bool
	SegmentCount  int
}

// analyzeMediaPlaylist analyzes a parsed media playlist.
func (c *StreamClassifier) analyzeMediaPlaylist(media *playlist.Media) MediaPlaylistAnalysis {
	analysis := MediaPlaylistAnalysis{
		AllSegmentsTS: true,
		SegmentCount:  len(media.Segments),
	}

	// Check for encryption (per-segment keys)
	for _, seg := range media.Segments {
		if seg != nil && seg.Key != nil {
			analysis.IsEncrypted = true
			break
		}
	}

	// Check for fMP4 (indicated by Map/init segment at playlist level)
	if media.Map != nil {
		analysis.UsesFMP4 = true
	}

	// Check segment extensions
	for _, seg := range media.Segments {
		if seg == nil {
			continue
		}
		// Strip query string for extension check
		uri := seg.URI
		if idx := strings.Index(uri, "?"); idx >= 0 {
			uri = uri[:idx]
		}
		if !strings.HasSuffix(strings.ToLower(uri), ".ts") {
			analysis.AllSegmentsTS = false
			break
		}
	}

	return analysis
}

// fetchPlaylist fetches a playlist with timeout and size limit.
func (c *StreamClassifier) fetchPlaylist(ctx context.Context, playlistURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, playlistURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, int64(c.maxPlaylistBytes))
	return io.ReadAll(limited)
}
