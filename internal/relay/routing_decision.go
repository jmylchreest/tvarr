// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"io"
	"log/slog"
	"strings"

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
		profile *models.RelayProfile,
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
// 1. If detection_mode != "auto", use profile settings directly (no client detection)
// 2. If profile requires transcoding (non-copy codecs), route to FFmpeg
// 3. If source is not HLS/DASH (raw TS), require FFmpeg for segmentation
// 4. Otherwise, attempt passthrough or repackaging based on format compatibility
func (d *DefaultRoutingDecider) Decide(
	sourceFormat SourceFormat,
	sourceCodecs []string,
	client ClientCapabilities,
	profile *models.RelayProfile,
) RoutingResult {
	result := RoutingResult{
		SourceFormat:  sourceFormat,
		DetectionMode: string(profile.DetectionMode),
		Reasons:       make([]string, 0),
	}

	// Step 1: Check if detection_mode != "auto" - use profile settings directly
	if !profile.IsAutoDetection() {
		result.Reasons = append(result.Reasons, "detection_mode is not auto - using profile settings directly")
		result = d.decideFromProfile(result, sourceFormat, sourceCodecs, profile)
		d.logRoutingDecision(result, sourceCodecs, client, profile)
		return result
	}

	// Step 2: Auto detection mode - consider client capabilities
	result.Reasons = append(result.Reasons, "detection_mode is auto - applying client detection")

	// Step 3: Check if profile requires transcoding (non-copy codecs)
	if profile.NeedsTranscode() {
		result.Decision = RouteTranscode
		result.ClientFormat = d.determineOutputFormat(client, profile)
		result.Reasons = append(result.Reasons, "profile specifies non-copy codecs - transcoding required")
		d.logRoutingDecision(result, sourceCodecs, client, profile)
		return result
	}

	// Step 4: Profile uses passthrough (copy) - check if source format allows repackaging
	if !sourceFormat.IsSegmented() {
		// Raw TS or unknown source - needs FFmpeg for segmentation
		result.Decision = RouteTranscode
		result.ClientFormat = d.determineOutputFormat(client, profile)
		result.Reasons = append(result.Reasons, "source is not segmented (raw TS) - FFmpeg needed for segmentation")
		d.logRoutingDecision(result, sourceCodecs, client, profile)
		return result
	}

	// Step 5: Source is HLS/DASH - check format compatibility for passthrough/repackage
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

// decideFromProfile makes routing decision based on profile settings without client detection.
func (d *DefaultRoutingDecider) decideFromProfile(
	result RoutingResult,
	sourceFormat SourceFormat,
	sourceCodecs []string,
	profile *models.RelayProfile,
) RoutingResult {
	// Determine output format from profile's detection_mode
	switch profile.DetectionMode {
	case models.DetectionModeHLS:
		result.ClientFormat = FormatValueHLS
	case models.DetectionModeMPEGTS:
		result.ClientFormat = FormatValueMPEGTS
	case models.DetectionModeDASH:
		result.ClientFormat = FormatValueDASH
	default:
		// Fallback to profile's container format
		result.ClientFormat = string(profile.ContainerFormat)
	}

	// If profile requires transcoding
	if profile.NeedsTranscode() {
		result.Decision = RouteTranscode
		result.Reasons = append(result.Reasons, "profile specifies non-copy codecs")
		return result
	}

	// Profile uses passthrough - check source compatibility
	if !sourceFormat.IsSegmented() {
		result.Decision = RouteTranscode
		result.Reasons = append(result.Reasons, "source is not segmented - FFmpeg required")
		return result
	}

	// Check if we can passthrough or need to repackage
	if d.isPassthroughCompatible(sourceFormat, result.ClientFormat, sourceCodecs) {
		result.Decision = RoutePassthrough
		result.Reasons = append(result.Reasons, "format compatible - passthrough selected")
		return result
	}

	if d.isRepackageCompatible(sourceFormat, result.ClientFormat, sourceCodecs) {
		result.Decision = RouteRepackage
		result.Reasons = append(result.Reasons, "repackaging selected")
		return result
	}

	result.Decision = RouteTranscode
	result.Reasons = append(result.Reasons, "format incompatible - transcoding required")
	return result
}

// determineOutputFormat determines the output format based on client preferences and profile.
func (d *DefaultRoutingDecider) determineOutputFormat(client ClientCapabilities, profile *models.RelayProfile) string {
	// Priority: client preferred format > profile container format > default
	if client.PreferredFormat != "" {
		return client.PreferredFormat
	}
	if profile.ContainerFormat != "" && profile.ContainerFormat != models.ContainerFormatAuto {
		return string(profile.ContainerFormat)
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

	// Check codec compatibility with target container
	// fMP4 containers support all codecs
	// MPEG-TS containers only support H.264/H.265/AAC/AC3/EAC3/MP3
	if clientFormat == FormatValueMPEGTS || clientFormat == FormatValueHLSTS {
		return d.codecsCompatibleWithMPEGTS(codecs)
	}

	// fMP4/HLS-fMP4/DASH support all codecs
	return true
}

// codecsCompatibleWithMPEGTS checks if codecs can be muxed into MPEG-TS container.
func (d *DefaultRoutingDecider) codecsCompatibleWithMPEGTS(codecs []string) bool {
	mpegTSCompatible := map[string]bool{
		"h264": true, "avc": true, "avc1": true,
		"h265": true, "hevc": true, "hvc1": true, "hev1": true,
		"aac": true, "mp4a": true,
		"mp3": true,
		"ac3": true, "ec3": true, "eac3": true,
	}

	for _, codec := range codecs {
		codecLower := strings.ToLower(codec)
		// Extract base codec from full codec string (e.g., "avc1.64001f" -> "avc1")
		parts := strings.Split(codecLower, ".")
		baseCodec := parts[0]
		if !mpegTSCompatible[baseCodec] {
			return false
		}
	}
	return true
}

// logRoutingDecision logs the routing decision with all relevant context per FR-009.
func (d *DefaultRoutingDecider) logRoutingDecision(
	result RoutingResult,
	sourceCodecs []string,
	client ClientCapabilities,
	profile *models.RelayProfile,
) {
	d.logger.Info("routing decision made",
		slog.String("decision", result.Decision.String()),
		slog.String("source_format", string(result.SourceFormat)),
		slog.String("client_format", result.ClientFormat),
		slog.String("detection_mode", result.DetectionMode),
		slog.Any("source_codecs", sourceCodecs),
		slog.String("client_player", client.PlayerName),
		slog.String("client_preferred_format", client.PreferredFormat),
		slog.Bool("client_supports_fmp4", client.SupportsFMP4),
		slog.String("client_detection_source", client.DetectionSource),
		slog.String("profile_name", profile.Name),
		slog.String("profile_video_codec", string(profile.VideoCodec)),
		slog.String("profile_audio_codec", string(profile.AudioCodec)),
		slog.String("profile_container", string(profile.ContainerFormat)),
		slog.Any("reasons", result.Reasons),
	)
}
