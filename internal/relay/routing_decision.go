// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"io"
	"log/slog"

	"github.com/jmylchreest/tvarr/internal/models"
)

// RoutingDecision represents the chosen delivery path for a stream.
type RoutingDecision int

const (
	// RoutePassthrough - Direct proxy of source segments (no processing)
	// Used when: source format matches client format, codecs compatible
	RoutePassthrough RoutingDecision = iota

	// RouteRepackage - Container change via gohlslib Muxer (no codec change)
	// Used when: source is HLS/DASH, client wants different container, codecs match
	RouteRepackage

	// RouteTranscode - FFmpeg pipeline required
	// Used when: codec mismatch, raw TS source, or profile specifies transcoding
	RouteTranscode
)

// String returns the string representation of the routing decision.
func (d RoutingDecision) String() string {
	switch d {
	case RoutePassthrough:
		return "passthrough"
	case RouteRepackage:
		return "repackage"
	case RouteTranscode:
		return "transcode"
	default:
		return "unknown"
	}
}

// MarshalJSON implements json.Marshaler for RoutingDecision.
// This allows RoutingDecision to serialize as a string in JSON.
func (d RoutingDecision) MarshalJSON() ([]byte, error) {
	return []byte(`"` + d.String() + `"`), nil
}

// UnmarshalJSON implements json.Unmarshaler for RoutingDecision.
// This allows RoutingDecision to deserialize from a string in JSON.
func (d *RoutingDecision) UnmarshalJSON(data []byte) error {
	// Remove quotes from JSON string
	str := string(data)
	if len(str) >= 2 && str[0] == '"' && str[len(str)-1] == '"' {
		str = str[1 : len(str)-1]
	}
	switch str {
	case "passthrough":
		*d = RoutePassthrough
	case "repackage":
		*d = RouteRepackage
	case "transcode":
		*d = RouteTranscode
	default:
		*d = RoutePassthrough // Default to passthrough for unknown values
	}
	return nil
}

// RoutingResult contains the full routing decision with context.
type RoutingResult struct {
	// Decision is the chosen routing path.
	Decision RoutingDecision

	// SourceFormat is the detected source stream format.
	SourceFormat SourceFormat

	// ClientFormat is the determined output format for the client.
	// Values: "hls-fmp4", "hls-ts", "mpegts", "dash"
	ClientFormat string

	// DetectionMode is the profile's detection_mode value.
	DetectionMode string

	// Reasons contains explanations for the routing decision.
	Reasons []string
}

// RoutingDecider determines the optimal routing path for a stream.
type RoutingDecider interface {
	// Decide returns the routing decision based on source, client, and profile.
	Decide(
		sourceFormat SourceFormat,
		sourceCodecs []string,
		client ClientCapabilities,
		profile *models.EncodingProfile,
	) RoutingResult
}

// DefaultRoutingDecider implements the RoutingDecider interface with smart routing logic.
type DefaultRoutingDecider struct {
	logger *slog.Logger
}

// NewDefaultRoutingDecider creates a new routing decider with optional logger.
func NewDefaultRoutingDecider(logger *slog.Logger) *DefaultRoutingDecider {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &DefaultRoutingDecider{logger: logger}
}

// Decide determines the optimal routing path for a stream based on source, client, and profile.
// The decision flow is:
// 1. If profile requires transcoding (non-copy codecs), route to FFmpeg
// 2. If source is not HLS/DASH (raw TS), require FFmpeg for segmentation
// 3. Otherwise, attempt passthrough or repackaging based on format compatibility
func (d *DefaultRoutingDecider) Decide(
	sourceFormat SourceFormat,
	sourceCodecs []string,
	client ClientCapabilities,
	profile *models.EncodingProfile,
) RoutingResult {
	result := RoutingResult{
		SourceFormat:  sourceFormat,
		DetectionMode: "auto", // EncodingProfile always uses auto detection
		Reasons:       make([]string, 0),
	}

	// Step 1: Check if transcoding is actually needed
	// Even if a profile has target codecs, we only transcode if the client
	// cannot accept the source codecs. This allows passthrough when possible.
	if profile != nil && profile.NeedsTranscode() {
		// Check if client can accept ALL source codecs
		clientAcceptsSource := d.clientAcceptsSourceCodecs(sourceCodecs, client)

		if !clientAcceptsSource {
			// Client cannot accept source codecs - transcoding required
			result.Decision = RouteTranscode
			result.ClientFormat = d.determineOutputFormat(client, profile)
			result.Reasons = append(result.Reasons, "client does not accept source codecs - transcoding to profile targets")
			d.logRoutingDecision(result, sourceCodecs, client, profile)
			return result
		}
		// Client accepts source codecs - skip transcoding, allow passthrough/repackage
		result.Reasons = append(result.Reasons, "client accepts source codecs - skipping unnecessary transcoding")
	}

	// Step 2: Profile uses passthrough (copy) - check if source format allows repackaging
	if !sourceFormat.IsSegmented() {
		// Raw TS or unknown source - needs FFmpeg for segmentation
		result.Decision = RouteTranscode
		result.ClientFormat = d.determineOutputFormat(client, profile)
		result.Reasons = append(result.Reasons, "source is not segmented (raw TS) - FFmpeg needed for segmentation")
		d.logRoutingDecision(result, sourceCodecs, client, profile)
		return result
	}

	// Step 3: Source is HLS/DASH - check format compatibility for passthrough/repackage
	clientFormat := d.determineOutputFormat(client, profile)
	result.ClientFormat = clientFormat

	// Check if source and client formats are compatible for passthrough
	if d.isPassthroughCompatible(sourceFormat, clientFormat, sourceCodecs) {
		result.Decision = RoutePassthrough
		result.Reasons = append(result.Reasons, "source and client formats are compatible - passthrough possible")
		d.logRoutingDecision(result, sourceCodecs, client, profile)
		return result
	}

	// Check if repackaging is possible (same codecs, different container)
	if d.isRepackageCompatible(sourceFormat, clientFormat, sourceCodecs) {
		result.Decision = RouteRepackage
		result.Reasons = append(result.Reasons, "container repackaging possible without codec change")
		d.logRoutingDecision(result, sourceCodecs, client, profile)
		return result
	}

	// Fallback to transcoding
	result.Decision = RouteTranscode
	result.Reasons = append(result.Reasons, "format/codec mismatch - transcoding required")

	d.logRoutingDecision(result, sourceCodecs, client, profile)
	return result
}

// determineOutputFormat determines the output format based on client preferences and profile.
func (d *DefaultRoutingDecider) determineOutputFormat(client ClientCapabilities, profile *models.EncodingProfile) string {
	// Priority: client preferred format > profile container determination > default
	if client.PreferredFormat != "" {
		return client.PreferredFormat
	}
	if profile != nil {
		container := profile.DetermineContainer()
		if container != "" && container != models.ContainerFormatAuto {
			return string(container)
		}
	}
	// Default to HLS with fMP4 for modern clients
	if client.SupportsFMP4 {
		return FormatValueHLSFMP4
	}
	return FormatValueMPEGTS
}

// isPassthroughCompatible checks if source can be passed through directly to client.
func (d *DefaultRoutingDecider) isPassthroughCompatible(sourceFormat SourceFormat, clientFormat string, codecs []string) bool {
	// For passthrough, source format must match client format exactly
	switch clientFormat {
	case FormatValueHLS, FormatValueHLSFMP4, FormatValueHLSTS:
		return sourceFormat == SourceFormatHLS
	case FormatValueDASH:
		return sourceFormat == SourceFormatDASH
	case FormatValueMPEGTS:
		return sourceFormat == SourceFormatMPEGTS
	default:
		return false
	}
}

// isRepackageCompatible checks if source can be repackaged (container change) without transcoding.
func (d *DefaultRoutingDecider) isRepackageCompatible(sourceFormat SourceFormat, clientFormat string, codecs []string) bool {
	// Repackaging is possible when source is segmented and codecs are compatible
	if !sourceFormat.IsSegmented() {
		return false
	}

	// Use the codec compatibility module to check container compatibility
	containerFormat := ParseContainerFormat(clientFormat)
	return AreCodecsCompatible(containerFormat, codecs)
}

// codecsCompatibleWithMPEGTS checks if codecs can be muxed into MPEG-TS container.
// Delegates to the codec_compatibility module for consistent behavior.
func (d *DefaultRoutingDecider) codecsCompatibleWithMPEGTS(codecs []string) bool {
	return CodecsCompatibleWithMPEGTS(codecs)
}

// clientAcceptsSourceCodecs checks if the client can accept all source codecs.
// Returns true if client has no codec restrictions or accepts all source codecs.
func (d *DefaultRoutingDecider) clientAcceptsSourceCodecs(sourceCodecs []string, client ClientCapabilities) bool {
	if len(sourceCodecs) == 0 {
		return true // No source codecs known, assume compatible
	}

	for _, codec := range sourceCodecs {
		// Normalize codec name for comparison
		normalizedCodec := d.normalizeCodecName(codec)

		// Check if it's a video or audio codec and verify client accepts it
		if d.isKnownVideoCodec(normalizedCodec) {
			if !client.AcceptsVideoCodec(normalizedCodec) {
				return false
			}
		} else if d.isKnownAudioCodec(normalizedCodec) {
			if !client.AcceptsAudioCodec(normalizedCodec) {
				return false
			}
		}
		// Unknown codec type - assume compatible
	}
	return true
}

// normalizeCodecName normalizes codec names for consistent comparison.
// Handles both simple names (h264, aac) and HLS codec strings (avc1.*, mp4a.*).
func (d *DefaultRoutingDecider) normalizeCodecName(codec string) string {
	// Handle HLS codec strings (avc1.*, hev1.*, hvc1.*, mp4a.*, etc.)
	if len(codec) >= 4 {
		prefix := codec[:4]
		switch prefix {
		case "avc1", "avc3":
			return "h264"
		case "hev1", "hvc1":
			return "h265"
		case "mp4a":
			return "aac" // mp4a.40.2 is AAC-LC, mp4a.40.5 is HE-AAC, etc.
		case "ac-3":
			return "ac3"
		case "ec-3":
			return "eac3"
		case "vp09":
			return "vp9"
		case "av01":
			return "av1"
		}
	}

	// Handle common aliases
	switch codec {
	case "hevc":
		return "h265"
	case "avc":
		return "h264"
	default:
		return codec
	}
}

// isKnownVideoCodec returns true if the codec is a known video codec.
func (d *DefaultRoutingDecider) isKnownVideoCodec(codec string) bool {
	switch codec {
	case "h264", "h265", "hevc", "avc", "vp8", "vp9", "av1", "mpeg2video":
		return true
	default:
		return false
	}
}

// isKnownAudioCodec returns true if the codec is a known audio codec.
func (d *DefaultRoutingDecider) isKnownAudioCodec(codec string) bool {
	switch codec {
	case "aac", "ac3", "eac3", "mp3", "opus", "flac", "vorbis", "dts", "truehd", "pcm":
		return true
	default:
		return false
	}
}

// logRoutingDecision logs the routing decision with all relevant context per FR-009.
func (d *DefaultRoutingDecider) logRoutingDecision(
	result RoutingResult,
	sourceCodecs []string,
	client ClientCapabilities,
	profile *models.EncodingProfile,
) {
	logFields := []any{
		slog.String("decision", result.Decision.String()),
		slog.String("source_format", string(result.SourceFormat)),
		slog.String("client_format", result.ClientFormat),
		slog.String("detection_mode", result.DetectionMode),
		slog.Any("source_codecs", sourceCodecs),
		slog.String("client_player", client.PlayerName),
		slog.String("client_preferred_format", client.PreferredFormat),
		slog.Bool("client_supports_fmp4", client.SupportsFMP4),
		slog.String("client_detection_source", client.DetectionSource),
		slog.Any("reasons", result.Reasons),
	}
	if profile != nil {
		logFields = append(logFields,
			slog.String("profile_name", profile.Name),
			slog.String("profile_video_codec", string(profile.TargetVideoCodec)),
			slog.String("profile_audio_codec", string(profile.TargetAudioCodec)),
			slog.String("profile_container", string(profile.DetermineContainer())),
		)
	}
	d.logger.Info("routing decision made", logFields...)
}

// SelectRoute is a convenience function that determines the routing decision
// for a stream based on classification and client format. This replaces the
// deprecated SelectDelivery function.
//
// For preview mode (nil profile), it uses passthrough or repackage when possible.
// For regular streams with a profile, it checks if transcoding is needed.
func SelectRoute(
	classification ClassificationResult,
	clientFormat string,
	profile *models.EncodingProfile,
) RoutingDecision {
	// Create client capabilities from the format string
	caps := ClientCapabilities{
		PreferredFormat: clientFormat,
		SupportsFMP4:    true,
		SupportsMPEGTS:  true,
	}

	// Use the decider for consistent logic
	decider := NewDefaultRoutingDecider(nil)
	result := decider.Decide(
		classification.SourceFormat,
		nil, // No specific codecs needed for this decision
		caps,
		profile,
	)

	return result.Decision
}
