// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"context"
	"net/http"
	"time"
)

// SegmentProvider defines the interface for accessing segments.
type SegmentProvider interface {
	// GetSegmentInfos returns metadata for all available segments (for playlist generation).
	GetSegmentInfos() []SegmentInfo

	// GetSegment returns a segment by sequence number (includes data).
	GetSegment(sequence uint64) (*Segment, error)

	// TargetDuration returns the target segment duration in seconds.
	TargetDuration() int
}

// FMP4SegmentProvider extends SegmentProvider with fMP4-specific methods.
type FMP4SegmentProvider interface {
	SegmentProvider

	// IsFMP4Mode returns true if the provider is in fMP4/CMAF mode.
	IsFMP4Mode() bool

	// GetInitSegment returns the initialization segment (ftyp+moov) for fMP4 streams.
	// Returns nil if no init segment is available or not in fMP4 mode.
	GetInitSegment() *InitSegment

	// HasInitSegment returns true if an initialization segment is available.
	HasInitSegment() bool

	// GetFilteredInitSegment returns an init segment containing only the specified track type.
	// trackType should be "video" or "audio". Returns the full init segment if trackType is empty.
	// This is used for DASH to serve separate video-only and audio-only init segments.
	GetFilteredInitSegment(trackType string) ([]byte, error)

	// GetStreamStartTime returns the time when the first segment was created.
	// This is used for availabilityStartTime in DASH manifests which must be constant.
	// Returns zero time if no segments have been created yet.
	GetStreamStartTime() time.Time
}

// SegmentInfo contains segment metadata for playlist generation.
// This is a lightweight view without the actual segment data.
type SegmentInfo struct {
	Sequence      uint64
	Duration      float64
	IsKeyframe    bool
	Timestamp     time.Time
	Discontinuity bool // True if this segment marks a discontinuity (stream restart, format change)
	IsFMP4        bool // True if this is an fMP4/CMAF segment (.m4s)
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
