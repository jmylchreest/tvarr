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
	default:
		return "unknown"
	}
}

// ClassificationResult holds the result of stream classification.
type ClassificationResult struct {
	Mode                  StreamMode
	VariantCount          int
	TargetDuration        float64
	IsEncrypted           bool
	UsesFMP4              bool
	EligibleForCollapse   bool
	SelectedMediaPlaylist string
	SelectedBandwidth     int64
	Reasons               []string
}
