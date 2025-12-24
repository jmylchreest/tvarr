// Package relay provides streaming relay functionality for tvarr.
package relay

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
	// Format: "video/audio" (e.g., "h264/aac", "copy/copy")
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
)
