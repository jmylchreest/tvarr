// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"time"
)

// Segment represents a discrete media segment for HLS/DASH output.
type Segment struct {
	// Sequence is the segment number (monotonically increasing).
	Sequence uint64

	// Duration is the segment duration in seconds.
	Duration float64

	// Data is the raw segment bytes (.ts for HLS, .m4s for DASH).
	Data []byte

	// Timestamp is when the segment was created.
	Timestamp time.Time

	// IsKeyframe indicates if segment starts with a keyframe.
	IsKeyframe bool

	// PTS is the presentation timestamp of the first frame.
	PTS int64

	// DTS is the decode timestamp of the first frame.
	DTS int64

	// Discontinuity marks this segment as following a stream interruption.
	// Players should reset their decoders when encountering discontinuity.
	Discontinuity bool
}

// Size returns the byte size of the segment.
func (s *Segment) Size() int {
	return len(s.Data)
}

// IsEmpty returns true if the segment has no data.
func (s *Segment) IsEmpty() bool {
	return len(s.Data) == 0
}

// Clone creates a copy of the segment with its own data buffer.
func (s *Segment) Clone() *Segment {
	clone := &Segment{
		Sequence:      s.Sequence,
		Duration:      s.Duration,
		Timestamp:     s.Timestamp,
		IsKeyframe:    s.IsKeyframe,
		PTS:           s.PTS,
		DTS:           s.DTS,
		Discontinuity: s.Discontinuity,
	}
	if len(s.Data) > 0 {
		clone.Data = make([]byte, len(s.Data))
		copy(clone.Data, s.Data)
	}
	return clone
}
