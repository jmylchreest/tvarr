// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"context"
	"net/http"
	"time"
)

// SegmentProvider defines the interface for accessing segments.
// Both SegmentBuffer and UnifiedBuffer implement this interface.
type SegmentProvider interface {
	// GetSegmentInfos returns metadata for all available segments (for playlist generation).
	GetSegmentInfos() []SegmentInfo

	// GetSegment returns a segment by sequence number (includes data).
	GetSegment(sequence uint64) (*Segment, error)

	// TargetDuration returns the target segment duration in seconds.
	TargetDuration() int
}

// SegmentInfo contains segment metadata for playlist generation.
// This is a lightweight view without the actual segment data.
type SegmentInfo struct {
	Sequence      uint64
	Duration      float64
	IsKeyframe    bool
	Timestamp     time.Time
	Discontinuity bool // True if this segment marks a discontinuity (stream restart, format change)
}

// OutputHandler handles output for a specific format.
type OutputHandler interface {
	// Format returns the output format this handler serves.
	Format() string

	// ContentType returns the Content-Type for playlist/manifest responses.
	ContentType() string

	// SegmentContentType returns the Content-Type for segment responses.
	SegmentContentType() string

	// ServePlaylist serves the playlist/manifest.
	ServePlaylist(w http.ResponseWriter, baseURL string) error

	// ServeSegment serves a specific segment.
	ServeSegment(w http.ResponseWriter, sequence uint64) error

	// ServeStream serves a continuous stream (MPEG-TS only).
	// For HLS/DASH handlers, this returns ErrUnsupportedOperation.
	ServeStream(ctx context.Context, w http.ResponseWriter) error

	// SupportsStreaming returns true if this handler supports continuous streaming.
	SupportsStreaming() bool
}

// OutputHandlerBase provides common functionality for output handlers.
type OutputHandlerBase struct {
	provider SegmentProvider
}

// NewOutputHandlerBase creates a new base handler with a SegmentProvider.
func NewOutputHandlerBase(provider SegmentProvider) OutputHandlerBase {
	return OutputHandlerBase{provider: provider}
}

// Provider returns the segment provider.
func (b *OutputHandlerBase) Provider() SegmentProvider {
	return b.provider
}
