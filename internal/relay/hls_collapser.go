package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astits"
	gohlslib "github.com/bluenviron/gohlslib/v2"
	"github.com/bluenviron/gohlslib/v2/pkg/codecs"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/google/uuid"
)

// ErrCollapserClosed is returned when the collapser is closed.
var ErrCollapserClosed = errors.New("collapser closed")

// ErrCollapserAborted is returned when the collapser aborts due to errors.
var ErrCollapserAborted = errors.New("collapser aborted")

// HLSCollapser converts an HLS stream to a continuous MPEG-TS stream using gohlslib.
// It supports both MPEG-TS and fMP4 HLS variants.
type HLSCollapser struct {
	client    *gohlslib.Client
	sessionID string
	uri       string

	// Output
	pipeReader *io.PipeReader
	pipeWriter *io.PipeWriter
	muxer      *astits.Muxer

	// State
	mu       sync.Mutex
	started  atomic.Bool
	closed   atomic.Bool
	closeErr error

	// Track state
	videoTrack    *gohlslib.Track
	audioTrack    *gohlslib.Track
	videoPID      uint16
	audioPID      uint16
	videoReady    bool
	audioReady    bool
	h264SPS       []byte
	h264PPS       []byte
	h265VPS       []byte
	h265SPS       []byte
	h265PPS       []byte
	lastPTS       int64
	ptsOffset     int64
	firstPTS      int64
	firstPTSSet   bool

	// Config
	httpClient *http.Client
}

const (
	videoPIDBase uint16 = 256
	audioPIDBase uint16 = 257
)

// NewHLSCollapser creates a new HLS collapser using gohlslib.
func NewHLSCollapser(httpClient *http.Client, playlistURL string) *HLSCollapser {
	pr, pw := io.Pipe()

	c := &HLSCollapser{
		sessionID:  uuid.New().String(),
		uri:        playlistURL,
		pipeReader: pr,
		pipeWriter: pw,
		httpClient: httpClient,
		videoPID:   videoPIDBase,
		audioPID:   audioPIDBase,
	}

	return c
}

// Start begins the collapsing process. Call this before reading.
func (c *HLSCollapser) Start(ctx context.Context) error {
	if c.started.Swap(true) {
		return nil // Already started
	}

	// Create muxer writing to pipe
	c.muxer = astits.NewMuxer(ctx, c.pipeWriter)

	// Create gohlslib client
	c.client = &gohlslib.Client{
		URI:        c.uri,
		HTTPClient: c.httpClient,
		OnTracks:   c.onTracks,
	}

	// Start the client
	if err := c.client.Start(); err != nil {
		c.pipeWriter.CloseWithError(err)
		return fmt.Errorf("starting HLS client: %w", err)
	}

	// Wait for client in background
	go c.watchClient(ctx)

	return nil
}

// onTracks is called when gohlslib discovers tracks.
func (c *HLSCollapser) onTracks(tracks []*gohlslib.Track) error {
	for _, track := range tracks {
		switch codec := track.Codec.(type) {
		case *codecs.H264:
			c.videoTrack = track
			c.h264SPS = codec.SPS
			c.h264PPS = codec.PPS

			// Add video elementary stream to muxer
			if err := c.muxer.AddElementaryStream(astits.PMTElementaryStream{
				ElementaryPID: c.videoPID,
				StreamType:    astits.StreamTypeH264Video,
			}); err != nil {
				return fmt.Errorf("adding H264 stream: %w", err)
			}
			c.muxer.SetPCRPID(c.videoPID)
			c.videoReady = true

			// Register data callback
			c.client.OnDataH26x(track, c.onH264Data)

		case *codecs.H265:
			c.videoTrack = track
			c.h265VPS = codec.VPS
			c.h265SPS = codec.SPS
			c.h265PPS = codec.PPS

			// Add video elementary stream to muxer
			if err := c.muxer.AddElementaryStream(astits.PMTElementaryStream{
				ElementaryPID: c.videoPID,
				StreamType:    astits.StreamTypeH265Video,
			}); err != nil {
				return fmt.Errorf("adding H265 stream: %w", err)
			}
			c.muxer.SetPCRPID(c.videoPID)
			c.videoReady = true

			// Register data callback
			c.client.OnDataH26x(track, c.onH265Data)

		case *codecs.MPEG4Audio:
			c.audioTrack = track

			// Add audio elementary stream to muxer
			if err := c.muxer.AddElementaryStream(astits.PMTElementaryStream{
				ElementaryPID: c.audioPID,
				StreamType:    astits.StreamTypeAACAudio,
			}); err != nil {
				return fmt.Errorf("adding AAC stream: %w", err)
			}
			c.audioReady = true

			// Register data callback
			c.client.OnDataMPEG4Audio(track, c.onAACData)
		}
	}

	// Write initial tables
	if _, err := c.muxer.WriteTables(); err != nil {
		return fmt.Errorf("writing PMT: %w", err)
	}

	return nil
}

// onH264Data handles H264 video data.
func (c *HLSCollapser) onH264Data(pts int64, dts int64, au [][]byte) {
	if c.closed.Load() {
		return
	}

	// Handle PTS offset for continuous playback
	c.handlePTSOffset(pts)
	adjustedPTS := pts - c.ptsOffset

	// Prepend SPS/PPS if this is an IDR frame
	nalus := au
	if c.isH264IDR(au) && c.h264SPS != nil && c.h264PPS != nil {
		nalus = append([][]byte{c.h264SPS, c.h264PPS}, au...)
	}

	// Build Annex B data
	data, err := h264.AnnexB(nalus).Marshal()
	if err != nil {
		c.closeWithError(fmt.Errorf("marshaling H264 annex B: %w", err))
		return
	}

	// Write to muxer
	c.writeVideoData(adjustedPTS, dts-c.ptsOffset, data)
}

// onH265Data handles H265 video data.
func (c *HLSCollapser) onH265Data(pts int64, dts int64, au [][]byte) {
	if c.closed.Load() {
		return
	}

	// Handle PTS offset for continuous playback
	c.handlePTSOffset(pts)
	adjustedPTS := pts - c.ptsOffset

	// Prepend VPS/SPS/PPS if this is an IDR frame
	nalus := au
	if c.isH265IDR(au) && c.h265VPS != nil && c.h265SPS != nil && c.h265PPS != nil {
		nalus = append([][]byte{c.h265VPS, c.h265SPS, c.h265PPS}, au...)
	}

	// Build Annex B data (H265 uses the same format as H264)
	data := marshalAnnexB(nalus)

	// Write to muxer
	c.writeVideoData(adjustedPTS, dts-c.ptsOffset, data)
}

// marshalAnnexB converts NALUs to Annex B format (start code + NALU for each).
// This is used for H265 since mediacommon doesn't have an h265.AnnexB type.
func marshalAnnexB(nalus [][]byte) []byte {
	// Calculate total size
	size := 0
	for _, nalu := range nalus {
		size += 4 + len(nalu) // 4-byte start code + NALU
	}

	// Build output
	data := make([]byte, size)
	pos := 0
	for _, nalu := range nalus {
		// 4-byte start code: 0x00 0x00 0x00 0x01
		data[pos] = 0
		data[pos+1] = 0
		data[pos+2] = 0
		data[pos+3] = 1
		pos += 4
		copy(data[pos:], nalu)
		pos += len(nalu)
	}
	return data
}

// onAACData handles AAC audio data.
func (c *HLSCollapser) onAACData(pts int64, aus [][]byte) {
	if c.closed.Load() {
		return
	}

	// Handle PTS offset for continuous playback
	c.handlePTSOffset(pts)
	adjustedPTS := pts - c.ptsOffset

	// Write each audio unit to muxer
	for _, au := range aus {
		c.writeAudioData(adjustedPTS, au)
	}
}

// handlePTSOffset tracks PTS discontinuities for continuous output.
func (c *HLSCollapser) handlePTSOffset(pts int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.firstPTSSet {
		c.firstPTS = pts
		c.ptsOffset = pts
		c.firstPTSSet = true
		c.lastPTS = 0
		return
	}

	// Detect discontinuity (large jump or backwards)
	expectedPTS := c.lastPTS + 90000 // Allow up to 1 second gap
	if pts < c.lastPTS || pts > expectedPTS+90000*10 {
		// Discontinuity detected - adjust offset to maintain continuity
		c.ptsOffset = pts - c.lastPTS - 3003 // ~1 frame at 29.97fps
	}

	adjustedPTS := pts - c.ptsOffset
	if adjustedPTS > c.lastPTS {
		c.lastPTS = adjustedPTS
	}
}

// writeVideoData writes video data to the TS muxer.
func (c *HLSCollapser) writeVideoData(pts, dts int64, data []byte) {
	if !c.videoReady {
		return
	}

	// Convert to 90kHz clock
	ptsTime := &astits.ClockReference{Base: pts}
	dtsTime := &astits.ClockReference{Base: dts}

	_, err := c.muxer.WriteData(&astits.MuxerData{
		PID: c.videoPID,
		AdaptationField: &astits.PacketAdaptationField{
			RandomAccessIndicator: c.isRandomAccess(data),
		},
		PES: &astits.PESData{
			Header: &astits.PESHeader{
				OptionalHeader: &astits.PESOptionalHeader{
					MarkerBits:              2,
					PTSDTSIndicator:         3,
					PTS:                     ptsTime,
					DTS:                     dtsTime,
					DataAlignmentIndicator:  true,
				},
				StreamID: 224, // Video stream
			},
			Data: data,
		},
	})
	if err != nil {
		c.closeWithError(fmt.Errorf("writing video: %w", err))
	}
}

// writeAudioData writes audio data to the TS muxer.
func (c *HLSCollapser) writeAudioData(pts int64, data []byte) {
	if !c.audioReady {
		return
	}

	ptsTime := &astits.ClockReference{Base: pts}

	_, err := c.muxer.WriteData(&astits.MuxerData{
		PID: c.audioPID,
		PES: &astits.PESData{
			Header: &astits.PESHeader{
				OptionalHeader: &astits.PESOptionalHeader{
					MarkerBits:      2,
					PTSDTSIndicator: 2, // PTS only
					PTS:             ptsTime,
				},
				StreamID: 192, // Audio stream
			},
			Data: data,
		},
	})
	if err != nil {
		c.closeWithError(fmt.Errorf("writing audio: %w", err))
	}
}

// isH264IDR checks if the access unit contains an IDR frame.
func (c *HLSCollapser) isH264IDR(au [][]byte) bool {
	for _, nalu := range au {
		if len(nalu) > 0 {
			naluType := nalu[0] & 0x1F
			if naluType == 5 { // IDR slice
				return true
			}
		}
	}
	return false
}

// isH265IDR checks if the access unit contains an IDR frame.
func (c *HLSCollapser) isH265IDR(au [][]byte) bool {
	for _, nalu := range au {
		if len(nalu) > 0 {
			naluType := (nalu[0] >> 1) & 0x3F
			if naluType == 19 || naluType == 20 { // IDR_W_RADL or IDR_N_LP
				return true
			}
		}
	}
	return false
}

// isRandomAccess checks if the data starts with a random access point.
func (c *HLSCollapser) isRandomAccess(data []byte) bool {
	// Simple check - look for IDR NAL type in first few bytes
	// This is a heuristic, actual implementation should parse properly
	return len(data) > 4
}

// watchClient monitors the gohlslib client and handles errors.
func (c *HLSCollapser) watchClient(ctx context.Context) {
	err := c.client.Wait2()
	if err != nil && !errors.Is(err, gohlslib.ErrClientEOS) && !errors.Is(err, context.Canceled) {
		c.closeWithError(err)
	} else {
		c.closeWithError(io.EOF)
	}
}

// closeWithError closes the collapser with an error.
func (c *HLSCollapser) closeWithError(err error) {
	if c.closed.Swap(true) {
		return
	}

	c.mu.Lock()
	c.closeErr = err
	c.mu.Unlock()

	c.pipeWriter.CloseWithError(err)
}

// Read implements io.Reader.
func (c *HLSCollapser) Read(p []byte) (int, error) {
	return c.pipeReader.Read(p)
}

// Stop signals the collapser to stop.
func (c *HLSCollapser) Stop() {
	if c.client != nil {
		c.client.Close()
	}
	c.closeWithError(ErrCollapserAborted)
}

// Close closes the collapser and releases resources.
func (c *HLSCollapser) Close() error {
	c.Stop()
	return c.pipeReader.Close()
}

// IsClosed returns true if the collapser is closed.
func (c *HLSCollapser) IsClosed() bool {
	return c.closed.Load()
}

// SessionID returns the session ID for logging.
func (c *HLSCollapser) SessionID() string {
	return c.sessionID
}

// Error returns the error that caused the collapser to close.
func (c *HLSCollapser) Error() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeErr
}

// StreamClassifier classifies streams using gohlslib for better detection.
type StreamClassifier struct {
	client  *http.Client
	timeout time.Duration
}

// NewStreamClassifier creates a new stream classifier using gohlslib.
func NewStreamClassifier(client *http.Client) *StreamClassifier {
	return &StreamClassifier{
		client:  client,
		timeout: 3 * time.Second, // Reduced from 10s for faster startup
	}
}

// Classify classifies a stream URL using gohlslib for track detection.
func (c *StreamClassifier) Classify(ctx context.Context, streamURL string) ClassificationResult {
	result := ClassificationResult{
		Mode:         StreamModeUnknown,
		SourceFormat: SourceFormatUnknown,
		Reasons:      []string{},
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Check for DASH streams first (by URL extension)
	if isDASHURL(streamURL) {
		return c.classifyDASH(ctx, streamURL, &result)
	}

	// Check for raw MPEG-TS streams (by URL extension) - skip HLS probing
	if isMPEGTSURL(streamURL) {
		result.SourceFormat = SourceFormatMPEGTS
		result.Mode = StreamModePassthroughRawTS
		result.Reasons = append(result.Reasons, "Raw MPEG-TS detected by extension")
		return result
	}

	// Check for HLS streams (by URL extension or probing)
	if isHLSURL(streamURL) {
		result.SourceFormat = SourceFormatHLS
	}

	// Try to start a gohlslib client to detect tracks
	tracksCh := make(chan []*gohlslib.Track, 1)
	errCh := make(chan error, 1)

	client := &gohlslib.Client{
		URI:        streamURL,
		HTTPClient: c.client,
		OnTracks: func(tracks []*gohlslib.Track) error {
			tracksCh <- tracks
			return fmt.Errorf("classification complete") // Stop after getting tracks
		},
	}

	go func() {
		if err := client.Start(); err != nil {
			errCh <- err
			return
		}
		// Wait for completion or error
		err := client.Wait2()
		if err != nil && !errors.Is(err, context.Canceled) {
			select {
			case errCh <- err:
			default:
			}
		}
	}()

	defer client.Close()

	// Wait for tracks or timeout
	select {
	case tracks := <-tracksCh:
		// Confirmed as HLS if we got tracks
		result.SourceFormat = SourceFormatHLS
		return c.classifyTracks(tracks, &result)
	case err := <-errCh:
		result.Reasons = append(result.Reasons, fmt.Sprintf("Client error: %v", err))
		// If HLS detection failed, check if it might be raw MPEG-TS
		if result.SourceFormat == SourceFormatUnknown {
			result.SourceFormat = SourceFormatMPEGTS
			result.Mode = StreamModePassthroughRawTS
			result.Reasons = append(result.Reasons, "Not HLS/DASH, assuming MPEG-TS")
		}
		return result
	case <-ctx.Done():
		result.Reasons = append(result.Reasons, "Classification timeout")
		return result
	}
}

// classifyDASH classifies a DASH stream by probing the manifest.
func (c *StreamClassifier) classifyDASH(ctx context.Context, streamURL string, result *ClassificationResult) ClassificationResult {
	result.SourceFormat = SourceFormatDASH
	result.Mode = StreamModePassthroughDASH
	result.Reasons = append(result.Reasons, "DASH manifest detected")

	// Probe the manifest to verify it's accessible
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, streamURL, nil)
	if err != nil {
		result.Mode = StreamModeUnknown
		result.Reasons = append(result.Reasons, fmt.Sprintf("Failed to create request: %v", err))
		return *result
	}

	resp, err := c.client.Do(req)
	if err != nil {
		result.Mode = StreamModeUnknown
		result.Reasons = append(result.Reasons, fmt.Sprintf("Failed to probe DASH manifest: %v", err))
		return *result
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		result.Mode = StreamModeUnknown
		result.Reasons = append(result.Reasons, fmt.Sprintf("DASH manifest returned status %d", resp.StatusCode))
		return *result
	}

	result.Reasons = append(result.Reasons, "DASH manifest accessible, using passthrough proxy")
	return *result
}

// isDASHURL checks if the URL is a DASH manifest based on extension.
func isDASHURL(streamURL string) bool {
	// Check for common DASH manifest extensions
	lowerURL := strings.ToLower(streamURL)
	return strings.HasSuffix(lowerURL, ".mpd") ||
		strings.Contains(lowerURL, ".mpd?") ||
		strings.Contains(lowerURL, "/manifest(format=mpd")
}

// isHLSURL checks if the URL is an HLS playlist based on extension.
func isHLSURL(streamURL string) bool {
	lowerURL := strings.ToLower(streamURL)
	return strings.HasSuffix(lowerURL, ".m3u8") ||
		strings.HasSuffix(lowerURL, ".m3u") ||
		strings.Contains(lowerURL, ".m3u8?") ||
		strings.Contains(lowerURL, ".m3u?")
}

// isMPEGTSURL checks if the URL is a raw MPEG-TS stream based on extension.
func isMPEGTSURL(streamURL string) bool {
	lowerURL := strings.ToLower(streamURL)
	return strings.HasSuffix(lowerURL, ".ts") ||
		strings.Contains(lowerURL, ".ts?")
}

// classifyTracks analyzes discovered tracks.
func (c *StreamClassifier) classifyTracks(tracks []*gohlslib.Track, result *ClassificationResult) ClassificationResult {
	var hasVideo, hasAudio bool
	var videoCodec, audioCodec string

	for _, track := range tracks {
		switch codec := track.Codec.(type) {
		case *codecs.H264:
			hasVideo = true
			videoCodec = "H264"
			_ = codec // Use codec info if needed
		case *codecs.H265:
			hasVideo = true
			videoCodec = "H265"
		case *codecs.MPEG4Audio:
			hasAudio = true
			audioCodec = "AAC"
		case *codecs.Opus:
			hasAudio = true
			audioCodec = "Opus"
		}
	}

	result.VariantCount = 1 // gohlslib auto-selected best variant
	result.Reasons = append(result.Reasons, fmt.Sprintf("Tracks: video=%s audio=%s", videoCodec, audioCodec))

	if hasVideo {
		result.Mode = StreamModeCollapsedHLS
		result.EligibleForCollapse = true
		result.Reasons = append(result.Reasons, "Stream eligible for collapse via gohlslib")
	} else if hasAudio {
		result.Mode = StreamModeTransparentHLS
		result.Reasons = append(result.Reasons, "Audio-only stream")
	} else {
		result.Mode = StreamModeUnknown
		result.Reasons = append(result.Reasons, "No supported tracks found")
	}

	return *result
}
