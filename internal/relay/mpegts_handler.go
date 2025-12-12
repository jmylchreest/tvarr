// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// StreamingProcessor defines the interface for processors that support continuous streaming.
type StreamingProcessor interface {
	// ServeStream serves a continuous stream to a client.
	ServeStream(w http.ResponseWriter, r *http.Request, clientID string) error

	// SupportsStreaming returns true if this processor supports continuous streaming.
	SupportsStreaming() bool
}

// MPEGTSHandler handles continuous MPEG-TS streaming output.
// Implements the OutputHandler interface for serving raw MPEG-TS streams.
// Unlike HLS/DASH, MPEG-TS is served as a continuous stream without segmentation.
type MPEGTSHandler struct {
	processor StreamingProcessor
}

// NewMPEGTSHandler creates an MPEG-TS output handler with a StreamingProcessor.
func NewMPEGTSHandler(processor StreamingProcessor) *MPEGTSHandler {
	return &MPEGTSHandler{
		processor: processor,
	}
}

// Format returns the output format this handler serves.
func (h *MPEGTSHandler) Format() string {
	return FormatValueMPEGTS
}

// ContentType returns MPEG-TS content type.
func (h *MPEGTSHandler) ContentType() string {
	return ContentTypeMPEGTS
}

// SegmentContentType returns the Content-Type for segments.
// MPEG-TS streaming doesn't use segments, so this returns the same as ContentType.
func (h *MPEGTSHandler) SegmentContentType() string {
	return ContentTypeMPEGTS
}

// SupportsStreaming returns true as MPEG-TS supports continuous streaming.
func (h *MPEGTSHandler) SupportsStreaming() bool {
	return true
}

// ServePlaylist returns an error as MPEG-TS doesn't have playlists.
func (h *MPEGTSHandler) ServePlaylist(w http.ResponseWriter, baseURL string) error {
	http.Error(w, "MPEG-TS streams do not have playlists", http.StatusNotFound)
	return ErrUnsupportedOperation
}

// ServeSegment returns an error as MPEG-TS doesn't have segments.
func (h *MPEGTSHandler) ServeSegment(w http.ResponseWriter, sequence uint64) error {
	http.Error(w, "MPEG-TS streams do not have segments", http.StatusNotFound)
	return ErrUnsupportedOperation
}

// ServeStream serves the continuous MPEG-TS stream to a client.
func (h *MPEGTSHandler) ServeStream(ctx context.Context, w http.ResponseWriter) error {
	if h.processor == nil || !h.processor.SupportsStreaming() {
		return ErrUnsupportedOperation
	}

	// Generate a client ID
	clientID := uuid.New().String()

	// Create a minimal request with context
	r := &http.Request{}
	r = r.WithContext(ctx)

	return h.processor.ServeStream(w, r, clientID)
}

// ServeStreamWithRequest serves the continuous MPEG-TS stream using a full HTTP request.
// This provides more context (headers, remote addr) for client tracking.
func (h *MPEGTSHandler) ServeStreamWithRequest(w http.ResponseWriter, r *http.Request) error {
	if h.processor == nil || !h.processor.SupportsStreaming() {
		return ErrUnsupportedOperation
	}

	// Generate a client ID
	clientID := uuid.New().String()

	return h.processor.ServeStream(w, r, clientID)
}
