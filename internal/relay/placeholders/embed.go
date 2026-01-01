// Package placeholders provides embedded placeholder content for stream startup.
package placeholders

import "embed"

// PlaceholderDuration is the duration of each embedded placeholder in seconds.
const PlaceholderDuration = 1

//go:embed placeholder_h264_aac_1s.mp4
//go:embed placeholder_h265_aac_1s.mp4
//go:embed placeholder_vp9_opus_1s.mp4
//go:embed placeholder_av1_opus_1s.mp4
var placeholderFS embed.FS

// GetPlaceholder returns the embedded placeholder fMP4 for the given codec variant.
// Returns nil if no placeholder exists for the variant.
func GetPlaceholder(videoCodec, audioCodec string) []byte {
	filename := getFilename(videoCodec, audioCodec)
	if filename == "" {
		return nil
	}

	data, err := placeholderFS.ReadFile(filename)
	if err != nil {
		return nil
	}
	return data
}

// getFilename returns the placeholder filename for the codec combination.
func getFilename(videoCodec, audioCodec string) string {
	switch {
	case (videoCodec == "h264" || videoCodec == "avc") && audioCodec == "aac":
		return "placeholder_h264_aac_1s.mp4"
	case (videoCodec == "h265" || videoCodec == "hevc") && audioCodec == "aac":
		return "placeholder_h265_aac_1s.mp4"
	case videoCodec == "vp9" && audioCodec == "opus":
		return "placeholder_vp9_opus_1s.mp4"
	case videoCodec == "av1" && audioCodec == "opus":
		return "placeholder_av1_opus_1s.mp4"
	default:
		return ""
	}
}

// HasPlaceholder returns true if a placeholder exists for the codec combination.
func HasPlaceholder(videoCodec, audioCodec string) bool {
	return getFilename(videoCodec, audioCodec) != ""
}

// AvailableVariants returns a list of codec variants that have placeholders.
func AvailableVariants() []string {
	return []string{
		"h264/aac",
		"h265/aac",
		"vp9/opus",
		"av1/opus",
	}
}
