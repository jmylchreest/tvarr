// Package relay provides streaming relay functionality for tvarr.
package relay

import "time"

// Header constants for client identification.
const (
	// HeaderXTvarrPlayer is the custom header for player identification.
	// Frontend players (hls.js, mpegts.js) send this header to identify themselves
	// for optimal routing decisions.
	// Format: "player-name/version" (e.g., "hls.js/1.5.8", "mpegts.js/1.7.3")
	HeaderXTvarrPlayer = "X-Tvarr-Player"
)

// Content types for streaming formats.
const (
	// ContentTypeHLSPlaylist is the MIME type for HLS playlists (.m3u8).
	ContentTypeHLSPlaylist = "application/vnd.apple.mpegurl"

	// ContentTypeHLSSegment is the MIME type for HLS segments (.ts).
	ContentTypeHLSSegment = "video/MP2T"

	// ContentTypeDASHManifest is the MIME type for DASH manifests (.mpd).
	ContentTypeDASHManifest = "application/dash+xml"

	// ContentTypeDASHSegment is the MIME type for DASH media segments (.m4s).
	ContentTypeDASHSegment = "video/iso.segment"

	// ContentTypeDASHInit is the MIME type for DASH initialization segments (.mp4).
	ContentTypeDASHInit = "video/mp4"

	// ContentTypeMPEGTS is the MIME type for MPEG-TS streams.
	ContentTypeMPEGTS = "video/MP2T"

	// ContentTypeFMP4Segment is the MIME type for fMP4/CMAF media segments (.m4s).
	// Used for both HLS v7+ and DASH with CMAF.
	ContentTypeFMP4Segment = "video/mp4"

	// ContentTypeFMP4Init is the MIME type for fMP4/CMAF initialization segments.
	// Same as ContentTypeDASHInit but explicitly for CMAF.
	ContentTypeFMP4Init = "video/mp4"
)

// Query parameter names for format selection.
const (
	// QueryParamFormat is the query parameter for output format selection.
	QueryParamFormat = "format"

	// QueryParamSegment is the query parameter for segment sequence number.
	QueryParamSegment = "seg"

	// QueryParamInit is the query parameter for DASH initialization segment type.
	QueryParamInit = "init"

	// QueryParamVariant is the query parameter for codec variant selection.
	// Format: "video/audio" (e.g., "h264/aac", "source/source")
	// This ensures segment requests are routed to the correct processor.
	QueryParamVariant = "variant"
)

// Format parameter values.
const (
	// FormatValueHLS requests HLS output format.
	FormatValueHLS = "hls"

	// FormatValueDASH requests DASH output format.
	FormatValueDASH = "dash"

	// FormatValueMPEGTS requests MPEG-TS output format (default).
	FormatValueMPEGTS = "mpegts"

	// FormatValueAuto requests auto-detection of optimal format.
	FormatValueAuto = "auto"

	// FormatValueHLSFMP4 is the sub-format value for HLS with fMP4 segments (CMAF).
	// Modern browsers and players support this for better seeking and caching.
	FormatValueHLSFMP4 = "hls-fmp4"

	// FormatValueHLSTS is the sub-format value for HLS with MPEG-TS segments.
	// Maximum compatibility with legacy devices and players.
	FormatValueHLSTS = "hls-ts"

	// FormatValueFMP4 is an alias for fMP4 container format selection.
	FormatValueFMP4 = "fmp4"
)

// Default segment buffer configuration.
const (
	// DefaultSegmentDuration is the default segment duration in seconds.
	DefaultSegmentDuration = 6

	// DefaultPlaylistSize is the default number of segments in the playlist.
	DefaultPlaylistSize = 5

	// DefaultMaxBufferSize is the default maximum buffer size in bytes (100MB).
	DefaultMaxBufferSize = 100 * 1024 * 1024

	// SegmentWaitTimeout is the maximum time to wait for segments to become available.
	// This is set to 60s to accommodate slow software transcoders (e.g., VP9/AV1 without GPU).
	// The HTTP server WriteTimeout should be configured to exceed this value.
	SegmentWaitTimeout = 60 * time.Second
)

// Default DASH manifest values when metadata is not available.
const (
	// DefaultVideoWidth is the default video width in pixels.
	DefaultVideoWidth = 1920

	// DefaultVideoHeight is the default video height in pixels.
	DefaultVideoHeight = 1080

	// DefaultVideoBandwidth is the default video bandwidth in bits per second (5 Mbps).
	DefaultVideoBandwidth = 5_000_000

	// DefaultAudioBandwidth is the default audio bandwidth in bits per second (128 kbps).
	DefaultAudioBandwidth = 128_000

	// DefaultAudioChannels is the default number of audio channels.
	DefaultAudioChannels = 2
)
