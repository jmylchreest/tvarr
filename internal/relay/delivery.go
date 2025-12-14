// Package relay provides stream relay functionality with smart delivery.
package relay

import (
	"github.com/jmylchreest/tvarr/internal/codec"
	"github.com/jmylchreest/tvarr/internal/models"
)

// DeliveryDecision represents the delivery strategy for a stream.
type DeliveryDecision int

const (
	// DeliveryPassthrough passes the stream through without modification.
	// Used when source format matches client format.
	DeliveryPassthrough DeliveryDecision = iota

	// DeliveryRepackage changes the manifest format but keeps the same segments.
	// Only possible when source has pre-existing segments (HLS/DASH).
	// TS→HLS/DASH requires FFmpeg (use DeliveryTranscode instead).
	DeliveryRepackage

	// DeliveryTranscode runs the stream through FFmpeg for transcoding or segmentation.
	// Required when codecs need transcoding, or when creating segments from raw TS.
	DeliveryTranscode
)

// String returns a human-readable name for the delivery decision.
func (d DeliveryDecision) String() string {
	switch d {
	case DeliveryPassthrough:
		return "passthrough"
	case DeliveryRepackage:
		return "repackage"
	case DeliveryTranscode:
		return "transcode"
	default:
		return "unknown"
	}
}

// ClientFormat represents the format requested by the client.
type ClientFormat string

const (
	ClientFormatHLS    ClientFormat = "hls"
	ClientFormatDASH   ClientFormat = "dash"
	ClientFormatMPEGTS ClientFormat = "mpegts"
	ClientFormatAuto   ClientFormat = "auto"
)

// SelectDeliveryOptions provides optional parameters for smart delivery decision.
type SelectDeliveryOptions struct {
	// ClientCapabilities contains what codecs/formats the client accepts.
	// If provided, enables smart passthrough when client accepts source codecs.
	ClientCapabilities *ClientCapabilities

	// SourceVideoCodec is the video codec of the source stream (e.g., "h265", "hevc", "h264").
	// Used with ClientCapabilities to determine if transcoding is needed.
	SourceVideoCodec string

	// SourceAudioCodec is the audio codec of the source stream (e.g., "eac3", "aac", "ac3").
	// Used with ClientCapabilities to determine if transcoding is needed.
	SourceAudioCodec string
}

// SelectDelivery determines the optimal delivery strategy based on:
// - Source stream classification
// - Client's requested format
// - Profile transcoding requirements
// - Client codec capabilities (if provided)
//
// Decision logic:
// 1. If source format matches client format (or auto), prefer repackage for minimal latency
// 2. If client capabilities and source codecs are known, check codec compatibility
// 3. If profile requires transcoding AND we know client doesn't accept source, use DeliveryTranscode
// 4. If source has segments (HLS/DASH) and client wants different manifest, use DeliveryRepackage
// 5. Otherwise (e.g., TS source wanting HLS/DASH output), use DeliveryTranscode
//
// NOTE: DeliveryPassthrough (direct HTTP proxy) is currently not used automatically.
// All streaming goes through the ES buffer pipeline to enable:
// - Multiple clients sharing a single upstream connection
// - Bandwidth tracking and visualization
// - Buffer management and memory limits
// To enable direct passthrough, explicitly check for it in the handler.
func SelectDelivery(
	source ClassificationResult,
	clientFormat ClientFormat,
	profile *models.EncodingProfile,
	opts ...SelectDeliveryOptions,
) DeliveryDecision {
	var opt SelectDeliveryOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	// First check: if source format matches client format (or auto), we can potentially
	// pass through without transcoding. This is the fast path for most streams.
	formatMatches := sourceMatchesClient(source, clientFormat) || clientFormat == ClientFormatAuto

	// Check if we know the client can accept the source codecs
	var clientAcceptsSource bool
	var codecsKnown bool
	if opt.ClientCapabilities != nil && opt.SourceVideoCodec != "" {
		codecsKnown = true
		clientAcceptsSource = clientAcceptsSourceCodecs(opt.ClientCapabilities, opt.SourceVideoCodec, opt.SourceAudioCodec)
	}

	// Decision logic:
	// 1. If format matches AND (codecs unknown OR client accepts source) → repackage
	//    When codecs are unknown but format matches, optimistically assume compatibility.
	//    This avoids unnecessary transcoding for common cases like MPEG-TS streams.
	if formatMatches {
		if !codecsKnown || clientAcceptsSource {
			// Either we don't know the codecs (assume compatible) or we know client accepts them
			return DeliveryRepackage
		}
		// Format matches but client doesn't accept source codecs - need to transcode
		// (e.g., source is h265 but client only accepts h264)
	}

	// 2. If we know client accepts source codecs, use repackage even for format conversion
	//    This is a safety check - if format doesn't match but codecs are compatible,
	//    we still might be able to repackage (e.g., HLS↔DASH with same codecs)
	if codecsKnown && clientAcceptsSource {
		if canRepackage(source, clientFormat) {
			return DeliveryRepackage
		}
	}

	// 3. Profile requires transcoding?
	if profile != nil && profile.NeedsTranscode() {
		return DeliveryTranscode
	}

	// 4. Can repackage without transcoding?
	// Only possible if source has segments (HLS or DASH)
	if canRepackage(source, clientFormat) {
		return DeliveryRepackage
	}

	// 5. Must transcode (e.g., TS source wanting HLS/DASH output, or incompatible codecs)
	return DeliveryTranscode
}

// clientAcceptsSourceCodecs checks if the client accepts the source's video and audio codecs.
// Returns true if client can play the source directly without transcoding.
func clientAcceptsSourceCodecs(caps *ClientCapabilities, sourceVideoCodec, sourceAudioCodec string) bool {
	if caps == nil {
		return false
	}

	// Normalize codec names using the shared codec package (hevc → h265, ec-3 → eac3, etc.)
	sourceVideoCodec = codec.Normalize(sourceVideoCodec)
	sourceAudioCodec = codec.Normalize(sourceAudioCodec)

	// Check video codec acceptance
	videoAccepted := caps.AcceptsVideoCodec(sourceVideoCodec)

	// Check audio codec acceptance (empty source audio = video-only stream, always accepted)
	audioAccepted := sourceAudioCodec == "" || caps.AcceptsAudioCodec(sourceAudioCodec)

	return videoAccepted && audioAccepted
}

// sourceMatchesClient checks if the source format matches what the client wants.
func sourceMatchesClient(source ClassificationResult, clientFormat ClientFormat) bool {
	switch clientFormat {
	case ClientFormatHLS:
		return source.SourceFormat == SourceFormatHLS
	case ClientFormatDASH:
		return source.SourceFormat == SourceFormatDASH
	case ClientFormatMPEGTS:
		return source.SourceFormat == SourceFormatMPEGTS
	case ClientFormatAuto:
		// Auto format always matches - serve source as-is
		return true
	default:
		return false
	}
}

// canRepackage checks if we can serve a different manifest format
// using the source's existing segments.
//
// Repackaging is only possible when:
// - Source is HLS or DASH (has segments)
// - Client wants a different manifest format (HLS↔DASH)
//
// Raw TS cannot be repackaged because it has no pre-existing segments.
// Creating segments from TS requires FFmpeg (DeliveryTranscode).
func canRepackage(source ClassificationResult, clientFormat ClientFormat) bool {
	// Only segmented sources can be repackaged
	if source.SourceFormat != SourceFormatHLS && source.SourceFormat != SourceFormatDASH {
		return false
	}

	// Client wants MPEG-TS from segmented source - can passthrough segments as TS
	// (Note: This is a special case handled differently)
	if clientFormat == ClientFormatMPEGTS {
		return false // Let transcode handle TS output
	}

	// Source is HLS, client wants DASH (or vice versa) - can repackage
	if source.SourceFormat == SourceFormatHLS && clientFormat == ClientFormatDASH {
		return true
	}
	if source.SourceFormat == SourceFormatDASH && clientFormat == ClientFormatHLS {
		return true
	}

	return false
}

// DeliveryContext holds all information needed for delivery decision.
type DeliveryContext struct {
	Source          ClassificationResult
	ClientFormat    ClientFormat
	EncodingProfile *models.EncodingProfile
	Decision        DeliveryDecision
}

// NewDeliveryContext creates a context with the delivery decision already computed.
func NewDeliveryContext(
	source ClassificationResult,
	clientFormat ClientFormat,
	profile *models.EncodingProfile,
) DeliveryContext {
	return DeliveryContext{
		Source:          source,
		ClientFormat:    clientFormat,
		EncodingProfile: profile,
		Decision:        SelectDelivery(source, clientFormat, profile),
	}
}
