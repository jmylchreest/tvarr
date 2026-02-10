// Package daemon provides the tvarr-ffmpegd daemon implementation.
package daemon

import "github.com/jmylchreest/tvarr/internal/codec"

// InputMuxer is the interface for muxing ES samples into a container format
// that FFmpeg can read from stdin.
//
// Two implementations exist:
// - TSMuxer: For H.264/H.265 sources (outputs MPEG-TS)
// - FMP4Muxer: For VP9/AV1 sources (outputs fragmented MP4)
type InputMuxer interface {
	// WriteVideo writes a video sample to the muxer.
	// pts and dts are in 90kHz timescale.
	WriteVideo(pts, dts int64, data []byte, isKeyframe bool) error

	// WriteAudio writes an audio sample to the muxer.
	// pts is in 90kHz timescale.
	WriteAudio(pts int64, data []byte) error

	// Flush flushes any buffered data.
	Flush() error

	// InitializeAndGetHeader initializes the muxer and returns any header bytes
	// that should be written to FFmpeg before media data.
	InitializeAndGetHeader() ([]byte, error)

	// Format returns the FFmpeg input format string (e.g., "mpegts" or "mp4").
	Format() string
}

// InputMuxerConfig contains common configuration for input muxers.
type InputMuxerConfig struct {
	VideoCodec    string // Source video codec: h264, h265, vp9, av1
	AudioCodec    string // Source audio codec: aac, opus, ac3, etc.
	AudioInitData []byte // AudioSpecificConfig for AAC
}

// RequiresFMP4Input returns true if the source video codec requires fMP4 input
// because it's not compatible with MPEG-TS.
func RequiresFMP4Input(sourceVideoCodec string) bool {
	normalized := codec.Normalize(sourceVideoCodec)
	switch normalized {
	case "vp9", "av1":
		return true
	default:
		return false
	}
}
