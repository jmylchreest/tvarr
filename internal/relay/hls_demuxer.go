// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HLSDemuxerConfig configures the HLS demuxer.
type HLSDemuxerConfig struct {
	// HTTPClient for fetching playlists and segments.
	HTTPClient *http.Client

	// PlaylistRefreshInterval for live streams.
	PlaylistRefreshInterval time.Duration

	// SegmentFetchTimeout for individual segment fetches.
	SegmentFetchTimeout time.Duration

	// UserAgent for upstream requests.
	UserAgent string

	// Logger for structured logging.
	Logger *slog.Logger

	// Callbacks for demuxed samples.
	OnVideoSample    func(pts, dts int64, data []byte, isKeyframe bool)
	OnAudioSample    func(pts int64, data []byte)
	OnSegmentFetched func()
}

// HLSDemuxer demuxes HLS streams to elementary streams.
type HLSDemuxer struct {
	config    HLSDemuxerConfig
	buffer    *SharedESBuffer
	sourceURL string
	baseURL   string

	// Playlist state
	mu               sync.RWMutex
	mediaSequence    uint64
	targetDuration   float64
	segments         []hlsSegmentInfo
	lastSegmentFetch map[string]time.Time
	isLive           bool

	// TS demuxer for MPEG-TS segments
	tsDemuxer *TSDemuxer
}

// hlsSegmentInfo holds information about an HLS segment.
type hlsSegmentInfo struct {
	URL      string
	Duration float64
	Sequence uint64
}

// NewHLSDemuxer creates a new HLS demuxer.
func NewHLSDemuxer(sourceURL string, buffer *SharedESBuffer, config HLSDemuxerConfig) *HLSDemuxer {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.HTTPClient == nil {
		// For HLS segment fetching, we use a transport with connection timeouts.
		// Individual segment fetches are short-lived so a timeout is appropriate.
		config.HTTPClient = &http.Client{
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				IdleConnTimeout:       90 * time.Second,
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   10,
			},
			Timeout: 60 * time.Second, // Overall timeout for segment fetches
		}
	}
	if config.PlaylistRefreshInterval == 0 {
		config.PlaylistRefreshInterval = 2 * time.Second
	}
	if config.SegmentFetchTimeout == 0 {
		config.SegmentFetchTimeout = 10 * time.Second
	}

	// Parse base URL for resolving relative segment URLs
	baseURL := sourceURL
	if idx := strings.LastIndex(sourceURL, "/"); idx > 0 {
		baseURL = sourceURL[:idx+1]
	}

	d := &HLSDemuxer{
		config:           config,
		buffer:           buffer,
		sourceURL:        sourceURL,
		baseURL:          baseURL,
		lastSegmentFetch: make(map[string]time.Time),
		isLive:           true, // Assume live until we detect VOD
	}

	// Create TS demuxer for parsing MPEG-TS segments
	d.tsDemuxer = NewTSDemuxer(buffer, TSDemuxerConfig{
		Logger:        config.Logger,
		OnVideoSample: config.OnVideoSample,
		OnAudioSample: config.OnAudioSample,
	})

	return d
}

// Run starts the HLS demuxer loop.
func (d *HLSDemuxer) Run(ctx context.Context) error {
	d.config.Logger.Info("Starting HLS demuxer",
		slog.String("url", d.sourceURL))

	// Initial playlist fetch
	if err := d.refreshPlaylist(ctx); err != nil {
		return fmt.Errorf("initial playlist fetch: %w", err)
	}

	ticker := time.NewTicker(d.config.PlaylistRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Refresh playlist for live streams
			if d.isLive {
				if err := d.refreshPlaylist(ctx); err != nil {
					d.config.Logger.Warn("Playlist refresh failed",
						slog.String("error", err.Error()))
					continue
				}
			}

			// Fetch pending segments
			if err := d.fetchPendingSegments(ctx); err != nil {
				d.config.Logger.Warn("Segment fetch failed",
					slog.String("error", err.Error()))
			}
		}
	}
}

// refreshPlaylist fetches and parses the HLS playlist.
func (d *HLSDemuxer) refreshPlaylist(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.sourceURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	if d.config.UserAgent != "" {
		req.Header.Set("User-Agent", d.config.UserAgent)
	}

	resp, err := d.config.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching playlist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("playlist fetch status: %d", resp.StatusCode)
	}

	return d.parsePlaylist(resp.Body)
}

// parsePlaylist parses an HLS playlist.
func (d *HLSDemuxer) parsePlaylist(r io.Reader) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	scanner := bufio.NewScanner(r)
	var segments []hlsSegmentInfo
	var currentDuration float64
	var mediaSequence uint64
	var targetDuration float64
	isMaster := false
	isLive := true

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			continue
		}

		// Check for master playlist
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			isMaster = true
			continue
		}

		// VOD indicator
		if line == "#EXT-X-ENDLIST" {
			isLive = false
			continue
		}

		// Media sequence
		if strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:") {
			seqStr := strings.TrimPrefix(line, "#EXT-X-MEDIA-SEQUENCE:")
			if seq, err := strconv.ParseUint(seqStr, 10, 64); err == nil {
				mediaSequence = seq
			}
			continue
		}

		// Target duration
		if strings.HasPrefix(line, "#EXT-X-TARGETDURATION:") {
			durStr := strings.TrimPrefix(line, "#EXT-X-TARGETDURATION:")
			if dur, err := strconv.ParseFloat(durStr, 64); err == nil {
				targetDuration = dur
			}
			continue
		}

		// Segment duration
		if strings.HasPrefix(line, "#EXTINF:") {
			durStr := strings.TrimPrefix(line, "#EXTINF:")
			if idx := strings.Index(durStr, ","); idx >= 0 {
				durStr = durStr[:idx]
			}
			if dur, err := strconv.ParseFloat(durStr, 64); err == nil {
				currentDuration = dur
			}
			continue
		}

		// Segment URL (non-comment line after #EXTINF)
		if !strings.HasPrefix(line, "#") && currentDuration > 0 {
			segmentURL := d.resolveURL(line)
			segments = append(segments, hlsSegmentInfo{
				URL:      segmentURL,
				Duration: currentDuration,
				Sequence: mediaSequence + uint64(len(segments)),
			})
			currentDuration = 0
			continue
		}

		// For master playlists, get the first variant
		if isMaster && !strings.HasPrefix(line, "#") {
			// Switch to variant playlist
			variantURL := d.resolveURL(line)
			d.config.Logger.Info("Following variant playlist",
				slog.String("url", variantURL))
			d.sourceURL = variantURL
			d.baseURL = variantURL
			if idx := strings.LastIndex(variantURL, "/"); idx > 0 {
				d.baseURL = variantURL[:idx+1]
			}
			d.mu.Unlock()
			// Recursively fetch variant playlist
			err := d.refreshPlaylist(context.Background())
			d.mu.Lock()
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanning playlist: %w", err)
	}

	d.segments = segments
	d.mediaSequence = mediaSequence
	d.targetDuration = targetDuration
	d.isLive = isLive

	d.config.Logger.Debug("Parsed HLS playlist",
		slog.Int("segments", len(segments)),
		slog.Uint64("media_sequence", mediaSequence),
		slog.Bool("live", isLive))

	return nil
}

// resolveURL resolves a potentially relative URL against the base URL.
func (d *HLSDemuxer) resolveURL(urlStr string) string {
	if strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://") {
		return urlStr
	}

	// Resolve relative URL
	base, err := url.Parse(d.baseURL)
	if err != nil {
		return d.baseURL + urlStr
	}

	ref, err := url.Parse(urlStr)
	if err != nil {
		return d.baseURL + urlStr
	}

	return base.ResolveReference(ref).String()
}

// fetchPendingSegments fetches segments that haven't been processed yet.
func (d *HLSDemuxer) fetchPendingSegments(ctx context.Context) error {
	d.mu.RLock()
	segments := make([]hlsSegmentInfo, len(d.segments))
	copy(segments, d.segments)
	d.mu.RUnlock()

	for _, seg := range segments {
		// Check if already fetched
		d.mu.RLock()
		_, fetched := d.lastSegmentFetch[seg.URL]
		d.mu.RUnlock()

		if fetched {
			continue
		}

		// Fetch and demux segment
		if err := d.fetchAndDemuxSegment(ctx, seg); err != nil {
			return fmt.Errorf("segment %s: %w", seg.URL, err)
		}

		// Mark as fetched
		d.mu.Lock()
		d.lastSegmentFetch[seg.URL] = time.Now()

		// Clean up old entries
		cutoff := time.Now().Add(-5 * time.Minute)
		for url, fetchTime := range d.lastSegmentFetch {
			if fetchTime.Before(cutoff) {
				delete(d.lastSegmentFetch, url)
			}
		}
		d.mu.Unlock()
	}

	return nil
}

// fetchAndDemuxSegment fetches a single segment and demuxes it.
func (d *HLSDemuxer) fetchAndDemuxSegment(ctx context.Context, seg hlsSegmentInfo) error {
	// Create context with timeout
	fetchCtx, cancel := context.WithTimeout(ctx, d.config.SegmentFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, seg.URL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	if d.config.UserAgent != "" {
		req.Header.Set("User-Agent", d.config.UserAgent)
	}

	resp, err := d.config.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching segment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("segment fetch status: %d", resp.StatusCode)
	}

	// Determine segment type from content or URL
	contentType := resp.Header.Get("Content-Type")
	isFMP4 := strings.Contains(contentType, "mp4") ||
		strings.HasSuffix(seg.URL, ".m4s") ||
		strings.HasSuffix(seg.URL, ".mp4")

	if isFMP4 {
		// fMP4 segment - needs different demuxer
		return d.demuxFMP4Segment(resp.Body)
	}

	// MPEG-TS segment - use TS demuxer
	return d.demuxTSSegment(resp.Body)
}

// demuxTSSegment demuxes an MPEG-TS segment.
func (d *HLSDemuxer) demuxTSSegment(r io.Reader) error {
	// Read segment data
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("reading segment: %w", err)
	}

	// Write to TS demuxer
	if err := d.tsDemuxer.Write(data); err != nil {
		return fmt.Errorf("demuxing segment: %w", err)
	}

	if d.config.OnSegmentFetched != nil {
		d.config.OnSegmentFetched()
	}

	return nil
}

// demuxFMP4Segment demuxes an fMP4 segment.
func (d *HLSDemuxer) demuxFMP4Segment(r io.Reader) error {
	// fMP4 demuxing requires more complex parsing
	// For now, we'll read the data but need proper fMP4 parsing
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("reading fMP4 segment: %w", err)
	}

	// Parse fMP4 boxes to extract samples
	if err := d.parseFMP4(data); err != nil {
		return fmt.Errorf("parsing fMP4: %w", err)
	}

	if d.config.OnSegmentFetched != nil {
		d.config.OnSegmentFetched()
	}

	return nil
}

// parseFMP4 parses an fMP4 segment and extracts samples.
func (d *HLSDemuxer) parseFMP4(data []byte) error {
	// fMP4 format:
	// - Init segment: ftyp, moov (with track info, SPS/PPS in avcC)
	// - Media segment: styp (optional), moof, mdat
	//
	// For proper implementation, we need to:
	// 1. Parse moov to get track configuration
	// 2. Parse moof to get sample timing/offsets
	// 3. Extract samples from mdat using moof info

	offset := 0
	for offset+8 <= len(data) {
		boxSize := int(data[offset])<<24 | int(data[offset+1])<<16 |
			int(data[offset+2])<<8 | int(data[offset+3])
		boxType := string(data[offset+4 : offset+8])

		if boxSize < 8 || offset+boxSize > len(data) {
			break
		}

		switch boxType {
		case "moof":
			// Movie fragment - contains sample info
			d.parseMoof(data[offset : offset+boxSize])
		case "mdat":
			// Media data - contains actual samples
			// Samples are extracted based on moof info
			d.parseMdat(data[offset+8 : offset+boxSize])
		case "moov":
			// Movie box - contains track configuration
			d.parseMoov(data[offset : offset+boxSize])
		}

		offset += boxSize
	}

	return nil
}

// parseMoov parses the moov box for track configuration.
func (d *HLSDemuxer) parseMoov(data []byte) {
	// Parse moov box to extract:
	// - Video codec info (avcC for H.264, hvcC for H.265)
	// - Audio codec info (esds for AAC)
	// This would set the init data on the buffer tracks
	d.config.Logger.Debug("Parsing moov box", slog.Int("size", len(data)))
}

// parseMoof parses the moof box for sample info.
func (d *HLSDemuxer) parseMoof(data []byte) {
	// Parse moof to get:
	// - traf boxes for each track
	// - tfhd for default sample info
	// - trun for sample sizes, durations, flags
	d.config.Logger.Debug("Parsing moof box", slog.Int("size", len(data)))
}

// parseMdat extracts samples from mdat.
func (d *HLSDemuxer) parseMdat(data []byte) {
	// For now, just log the mdat size
	// Proper implementation would use moof info to parse samples
	d.config.Logger.Debug("Parsing mdat box", slog.Int("size", len(data)))
}
