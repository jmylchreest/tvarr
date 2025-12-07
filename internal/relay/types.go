package relay

// StreamMode represents the classification of a stream.
type StreamMode int

const (
	// StreamModePassthroughRawTS is a direct raw MPEG-TS stream.
	StreamModePassthroughRawTS StreamMode = iota
	// StreamModeCollapsedHLS is an HLS stream that can be collapsed to continuous TS.
	StreamModeCollapsedHLS
	// StreamModeTransparentHLS is an HLS stream that must be passed through (multi-variant, encrypted, etc.).
	StreamModeTransparentHLS
	// StreamModePassthroughHLS is an HLS stream that should be proxied with URL rewriting.
	StreamModePassthroughHLS
	// StreamModePassthroughDASH is a DASH stream that should be proxied with URL rewriting.
	StreamModePassthroughDASH
	// StreamModeUnknown is an unknown stream type.
	StreamModeUnknown
)

func (m StreamMode) String() string {
	switch m {
	case StreamModePassthroughRawTS:
		return "passthrough-raw-ts"
	case StreamModeCollapsedHLS:
		return "collapsed-hls"
	case StreamModeTransparentHLS:
		return "transparent-hls"
	case StreamModePassthroughHLS:
		return "passthrough-hls"
	case StreamModePassthroughDASH:
		return "passthrough-dash"
	default:
		return "unknown"
	}
}

// SourceFormat indicates the format of the source stream.
type SourceFormat string

const (
	SourceFormatMPEGTS SourceFormat = "mpegts"
	SourceFormatHLS    SourceFormat = "hls"
	SourceFormatDASH   SourceFormat = "dash"
	SourceFormatUnknown SourceFormat = "unknown"
)

// ClassificationResult holds the result of stream classification.
type ClassificationResult struct {
	Mode                  StreamMode
	SourceFormat          SourceFormat // The source stream format (HLS, DASH, MPEG-TS)
	VariantCount          int
	TargetDuration        float64
	IsEncrypted           bool
	UsesFMP4              bool
	EligibleForCollapse   bool
	SelectedMediaPlaylist string
	SelectedBandwidth     int64
	Reasons               []string
}
