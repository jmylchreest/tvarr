// Package relay provides streaming relay functionality for tvarr.
package relay

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
)

// Query parameter names for format selection.
const (
	// QueryParamFormat is the query parameter for output format selection.
	QueryParamFormat = "format"

	// QueryParamSegment is the query parameter for segment sequence number.
	QueryParamSegment = "seg"

	// QueryParamInit is the query parameter for DASH initialization segment type.
	QueryParamInit = "init"
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
