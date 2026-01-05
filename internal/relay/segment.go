// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"maps"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
)

// Segment represents a discrete media segment for HLS/DASH output.
type Segment struct {
	// Sequence is the segment number (monotonically increasing).
	Sequence uint64

	// Duration is the segment duration in seconds.
	Duration float64

	// Data is the raw segment bytes (.ts for HLS, .m4s for DASH/CMAF).
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

	// ContainerFormat indicates the container format of this segment.
	// For fMP4/CMAF segments, this is ContainerFormatFMP4.
	ContainerFormat models.ContainerFormat

	// IsFragmented indicates if this is a fragmented MP4 segment (CMAF).
	// When true, Data contains moof+mdat boxes rather than MPEG-TS.
	IsFragmented bool

	// FragmentSequence is the fragment sequence number within the fMP4 stream.
	// This corresponds to the mfhd sequence_number in fMP4.
	FragmentSequence uint32
}

// InitSegment represents the initialization segment for fMP4/CMAF streams.
// This contains ftyp+moov boxes required before any media segments can be played.
type InitSegment struct {
	// Data is the raw init segment bytes (ftyp+moov).
	Data []byte

	// ETag is a hash of the init segment data for HTTP caching.
	// Used for conditional requests (If-None-Match) to avoid duplicate fetches.
	ETag string

	// Timestamp is when the init segment was created.
	Timestamp time.Time

	// HasVideo indicates if the stream contains video.
	HasVideo bool

	// HasAudio indicates if the stream contains audio.
	HasAudio bool

	// Timescale is the media timescale from mvhd (fallback).
	Timescale uint32

	// TrackTimescales maps track IDs to their timescales from mdhd boxes.
	// Fragment durations are in track timescale units, not movie timescale.
	TrackTimescales map[uint32]uint32

	// VideoTrackID is the track ID for the video track (if present).
	VideoTrackID uint32

	// AudioTrackID is the track ID for the audio track (if present).
	AudioTrackID uint32

	// VideoCodec is the RFC 6381 codec string for the video track (e.g., "vp09.02.10.08", "avc1.64001f").
	VideoCodec string

	// AudioCodec is the RFC 6381 codec string for the audio track (e.g., "opus", "mp4a.40.2").
	AudioCodec string
}

// GetTimescale returns the timescale for a specific track.
// Falls back to movie timescale if track-specific timescale is not available.
func (i *InitSegment) GetTimescale(trackID uint32) uint32 {
	if i.TrackTimescales != nil {
		if ts, ok := i.TrackTimescales[trackID]; ok {
			return ts
		}
	}
	// Fallback to movie timescale
	return i.Timescale
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
		Sequence:         s.Sequence,
		Duration:         s.Duration,
		Timestamp:        s.Timestamp,
		IsKeyframe:       s.IsKeyframe,
		PTS:              s.PTS,
		DTS:              s.DTS,
		Discontinuity:    s.Discontinuity,
		ContainerFormat:  s.ContainerFormat,
		IsFragmented:     s.IsFragmented,
		FragmentSequence: s.FragmentSequence,
	}
	if len(s.Data) > 0 {
		clone.Data = make([]byte, len(s.Data))
		copy(clone.Data, s.Data)
	}
	return clone
}

// IsFMP4 returns true if this is a fragmented MP4 segment.
func (s *Segment) IsFMP4() bool {
	return s.IsFragmented || s.ContainerFormat == models.ContainerFormatFMP4
}

// ContentType returns the MIME type for this segment.
func (s *Segment) ContentType() string {
	if s.IsFMP4() {
		return ContentTypeFMP4Segment
	}
	return ContentTypeMPEGTS
}

// Clone creates a copy of the init segment with its own data buffer.
func (i *InitSegment) Clone() *InitSegment {
	clone := &InitSegment{
		Timestamp:    i.Timestamp,
		HasVideo:     i.HasVideo,
		HasAudio:     i.HasAudio,
		Timescale:    i.Timescale,
		VideoTrackID: i.VideoTrackID,
		AudioTrackID: i.AudioTrackID,
	}
	if len(i.Data) > 0 {
		clone.Data = make([]byte, len(i.Data))
		copy(clone.Data, i.Data)
	}
	if len(i.TrackTimescales) > 0 {
		clone.TrackTimescales = make(map[uint32]uint32, len(i.TrackTimescales))
		maps.Copy(clone.TrackTimescales, i.TrackTimescales)
	}
	return clone
}

// Size returns the byte size of the init segment.
func (i *InitSegment) Size() int {
	return len(i.Data)
}

// IsEmpty returns true if the init segment has no data.
func (i *InitSegment) IsEmpty() bool {
	return len(i.Data) == 0
}
