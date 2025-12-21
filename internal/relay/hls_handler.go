// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SegmentWaiter is an optional interface that SegmentProviders can implement
// to support waiting for segments to become available.
type SegmentWaiter interface {
	// WaitForSegments waits until at least minSegments are available or context is cancelled.
	WaitForSegments(ctx context.Context, minSegments int) error
	// SegmentCount returns the current number of segments.
	SegmentCount() int
}

// HLSHandler handles HLS output.
// Implements the OutputHandler interface for serving HLS playlists and segments.
// Supports both HLS v3 (MPEG-TS) and HLS v7 (fMP4/CMAF) formats.
type HLSHandler struct {
	OutputHandlerBase
}

// NewHLSHandler creates an HLS output handler with a SegmentProvider.
func NewHLSHandler(provider SegmentProvider) *HLSHandler {
	return &HLSHandler{
		OutputHandlerBase: NewOutputHandlerBase(provider),
	}
}

// Format returns the output format this handler serves.
func (h *HLSHandler) Format() string {
	return FormatValueHLS
}

// ContentType returns HLS playlist content type.
func (h *HLSHandler) ContentType() string {
	return ContentTypeHLSPlaylist
}

// SegmentContentType returns the Content-Type for HLS segments.
// Returns video/mp4 for fMP4 segments, video/MP2T for MPEG-TS.
func (h *HLSHandler) SegmentContentType() string {
	// Check if provider supports fMP4 mode
	if fmp4Provider, ok := h.provider.(FMP4SegmentProvider); ok {
		if fmp4Provider.IsFMP4Mode() {
			return ContentTypeFMP4Segment
		}
	}
	return ContentTypeHLSSegment
}

// SupportsStreaming returns false as HLS uses playlist-based delivery.
func (h *HLSHandler) SupportsStreaming() bool {
	return false
}

// ServePlaylist generates and serves the HLS playlist.
// If the provider implements SegmentWaiter and has no segments, it will wait
// up to 15 seconds for the first segment before returning.
func (h *HLSHandler) ServePlaylist(w http.ResponseWriter, baseURL string) error {
	return h.ServePlaylistWithContext(context.Background(), w, baseURL)
}

// ServePlaylistWithContext generates and serves the HLS playlist with context support.
// If the provider implements SegmentWaiter and has no segments, it will wait
// up to 15 seconds for the first segment before returning.
func (h *HLSHandler) ServePlaylistWithContext(ctx context.Context, w http.ResponseWriter, baseURL string) error {
	// Check if provider supports waiting for segments
	if waiter, ok := h.provider.(SegmentWaiter); ok {
		if waiter.SegmentCount() == 0 {
			// Wait for at least 1 segment (with timeout)
			waitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()

			if err := waiter.WaitForSegments(waitCtx, 1); err != nil {
				http.Error(w, "No segments available yet, please retry", http.StatusServiceUnavailable)
				return fmt.Errorf("waiting for segments: %w", err)
			}
		}
	}

	playlist := h.GeneratePlaylist(baseURL)

	w.Header().Set("Content-Type", ContentTypeHLSPlaylist)
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.WriteHeader(http.StatusOK)

	_, err := w.Write([]byte(playlist))
	return err
}

// ServeSegment serves a segment (.ts for MPEG-TS, .m4s for fMP4).
func (h *HLSHandler) ServeSegment(w http.ResponseWriter, sequence uint64) error {
	seg, err := h.provider.GetSegment(sequence)
	if err != nil {
		if err == ErrSegmentNotFound {
			http.Error(w, "segment not found", http.StatusNotFound)
			return err
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return err
	}

	// Determine content type based on segment type
	contentType := ContentTypeHLSSegment
	if seg.IsFMP4() {
		contentType = ContentTypeFMP4Segment
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(seg.Data)))
	w.Header().Set("Cache-Control", "max-age=86400") // Segments can be cached
	w.WriteHeader(http.StatusOK)

	_, err = w.Write(seg.Data)
	return err
}

// ServeInitSegment serves the fMP4 initialization segment.
// This is called when a client requests the init segment referenced by EXT-X-MAP.
func (h *HLSHandler) ServeInitSegment(w http.ResponseWriter) error {
	fmp4Provider, ok := h.provider.(FMP4SegmentProvider)
	if !ok || !fmp4Provider.IsFMP4Mode() {
		http.Error(w, "init segment not available", http.StatusNotFound)
		return ErrUnsupportedOperation
	}

	initSeg := fmp4Provider.GetInitSegment()
	if initSeg == nil || initSeg.IsEmpty() {
		http.Error(w, "init segment not ready", http.StatusServiceUnavailable)
		return ErrSegmentNotFound
	}

	w.Header().Set("Content-Type", ContentTypeFMP4Init)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(initSeg.Data)))
	w.Header().Set("Cache-Control", "max-age=86400") // Init segment can be cached
	w.WriteHeader(http.StatusOK)

	_, err := w.Write(initSeg.Data)
	return err
}

// ServeStream returns an error as HLS doesn't support continuous streaming.
func (h *HLSHandler) ServeStream(ctx context.Context, w http.ResponseWriter) error {
	return ErrUnsupportedOperation
}

// GeneratePlaylist creates an HLS playlist from current segments.
// For MPEG-TS segments: Generates HLS v3 playlist with .ts segment URLs.
// For fMP4 segments: Generates HLS v7 playlist with #EXT-X-MAP and .m4s segment URLs.
func (h *HLSHandler) GeneratePlaylist(baseURL string) string {
	segments := h.provider.GetSegmentInfos()
	if len(segments) == 0 {
		// Return minimal valid playlist when no segments available
		return h.generateEmptyPlaylist()
	}

	// Determine if we're in fMP4 mode
	isFMP4Mode := false
	hasInitSegment := false
	if fmp4Provider, ok := h.provider.(FMP4SegmentProvider); ok {
		isFMP4Mode = fmp4Provider.IsFMP4Mode()
		hasInitSegment = fmp4Provider.HasInitSegment()
	}

	// Calculate target duration (max segment duration, rounded up)
	targetDuration := h.provider.TargetDuration()
	for _, seg := range segments {
		if int(seg.Duration+0.999) > targetDuration {
			targetDuration = int(seg.Duration + 0.999)
		}
	}

	// Get media sequence (first segment's sequence number)
	mediaSequence := segments[0].Sequence

	var sb strings.Builder

	// HLS header - use version 7 for fMP4, version 3 for MPEG-TS
	sb.WriteString("#EXTM3U\n")
	if isFMP4Mode {
		sb.WriteString("#EXT-X-VERSION:7\n")
	} else {
		sb.WriteString("#EXT-X-VERSION:3\n")
	}
	sb.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", targetDuration))
	sb.WriteString(fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d\n", mediaSequence))

	// Ensure baseURL doesn't have trailing slash
	baseURL = strings.TrimSuffix(baseURL, "/")

	// Determine the correct format value for segment URLs
	// Use hls-fmp4 when in fMP4 mode to ensure proper routing
	formatValue := FormatValueHLS
	if isFMP4Mode {
		formatValue = FormatValueHLSFMP4
	}

	// For fMP4 mode, add EXT-X-MAP pointing to the initialization segment
	// This tells players where to get the ftyp+moov boxes before any media segments
	if isFMP4Mode && hasInitSegment {
		sb.WriteString(fmt.Sprintf("#EXT-X-MAP:URI=\"%s?%s=%s&%s=1\"\n",
			baseURL,
			QueryParamFormat, formatValue,
			QueryParamInit,
		))
	}

	// Add segment entries
	for i, seg := range segments {
		// Check for discontinuity - either explicitly marked or detected by sequence gap
		if seg.Discontinuity {
			sb.WriteString("#EXT-X-DISCONTINUITY\n")
		} else if i > 0 {
			prevSeg := segments[i-1]
			// Detect discontinuity by sequence gap
			if seg.Sequence != prevSeg.Sequence+1 {
				sb.WriteString("#EXT-X-DISCONTINUITY\n")
			}
		}

		// Write segment info
		sb.WriteString(fmt.Sprintf("#EXTINF:%.3f,\n", seg.Duration))
		sb.WriteString(fmt.Sprintf("%s?%s=%s&%s=%d\n",
			baseURL,
			QueryParamFormat, formatValue,
			QueryParamSegment, seg.Sequence,
		))
	}

	return sb.String()
}

// generateEmptyPlaylist returns a minimal valid HLS playlist.
func (h *HLSHandler) generateEmptyPlaylist() string {
	// Check if we're in fMP4 mode
	version := 3
	if fmp4Provider, ok := h.provider.(FMP4SegmentProvider); ok {
		if fmp4Provider.IsFMP4Mode() {
			version = 7
		}
	}
	return fmt.Sprintf("#EXTM3U\n#EXT-X-VERSION:%d\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:0\n",
		version, h.provider.TargetDuration())
}

// IsFMP4Mode returns true if the handler is serving fMP4/CMAF segments.
func (h *HLSHandler) IsFMP4Mode() bool {
	if fmp4Provider, ok := h.provider.(FMP4SegmentProvider); ok {
		return fmp4Provider.IsFMP4Mode()
	}
	return false
}
