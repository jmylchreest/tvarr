// Package relay provides stream relay functionality with smart delivery.
package relay

import (
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

// SelectDelivery determines the optimal delivery strategy based on:
// - Source stream classification
// - Client's requested format
// - Profile transcoding requirements
//
// Decision logic:
// 1. If profile requires transcoding (codec != copy), use DeliveryTranscode
// 2. If source format matches client format, use DeliveryPassthrough
// 3. If source has segments (HLS/DASH) and client wants different manifest, use DeliveryRepackage
// 4. Otherwise (e.g., TS source wanting HLS/DASH), use DeliveryTranscode
func SelectDelivery(
	source ClassificationResult,
	clientFormat ClientFormat,
	profile *models.EncodingProfile,
) DeliveryDecision {
	// 1. Profile requires transcoding?
	if profile != nil && profile.NeedsTranscode() {
		return DeliveryTranscode
	}

	// 2. Source format matches client format?
	if sourceMatchesClient(source, clientFormat) {
		return DeliveryPassthrough
	}

	// 3. Can repackage without transcoding?
	// Only possible if source has segments (HLS or DASH)
	if canRepackage(source, clientFormat) {
		return DeliveryRepackage
	}

	// 4. Must transcode (e.g., TS source wanting HLS/DASH output)
	return DeliveryTranscode
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
