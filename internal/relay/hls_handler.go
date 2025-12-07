// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// HLSHandler handles HLS output.
// Implements the OutputHandler interface for serving HLS playlists and segments.
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
func (h *HLSHandler) SegmentContentType() string {
	return ContentTypeHLSSegment
}

// SupportsStreaming returns false as HLS uses playlist-based delivery.
func (h *HLSHandler) SupportsStreaming() bool {
	return false
}

// ServePlaylist generates and serves the HLS playlist.
func (h *HLSHandler) ServePlaylist(w http.ResponseWriter, baseURL string) error {
	playlist := h.GeneratePlaylist(baseURL)

	w.Header().Set("Content-Type", ContentTypeHLSPlaylist)
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.WriteHeader(http.StatusOK)

	_, err := w.Write([]byte(playlist))
	return err
}

// ServeSegment serves a .ts segment.
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

	w.Header().Set("Content-Type", ContentTypeHLSSegment)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(seg.Data)))
	w.Header().Set("Cache-Control", "max-age=86400") // Segments can be cached
	w.WriteHeader(http.StatusOK)

	_, err = w.Write(seg.Data)
	return err
}

// ServeStream returns an error as HLS doesn't support continuous streaming.
func (h *HLSHandler) ServeStream(ctx context.Context, w http.ResponseWriter) error {
	return ErrUnsupportedOperation
}

// GeneratePlaylist creates an HLS playlist from current segments.
// Implements EXT-X-VERSION 3+ with proper tags per FR-010 through FR-015.
func (h *HLSHandler) GeneratePlaylist(baseURL string) string {
	segments := h.provider.GetSegmentInfos()
	if len(segments) == 0 {
		// Return minimal valid playlist when no segments available
		return h.generateEmptyPlaylist()
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

	// HLS header
	sb.WriteString("#EXTM3U\n")
	sb.WriteString("#EXT-X-VERSION:3\n")
	sb.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", targetDuration))
	sb.WriteString(fmt.Sprintf("#EXT-X-MEDIA-SEQUENCE:%d\n", mediaSequence))

	// Ensure baseURL doesn't have trailing slash
	baseURL = strings.TrimSuffix(baseURL, "/")

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
			QueryParamFormat, FormatValueHLS,
			QueryParamSegment, seg.Sequence,
		))
	}

	return sb.String()
}

// generateEmptyPlaylist returns a minimal valid HLS playlist.
func (h *HLSHandler) generateEmptyPlaylist() string {
	return fmt.Sprintf("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:0\n",
		h.provider.TargetDuration())
}
