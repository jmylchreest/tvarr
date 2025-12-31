package migrations

import (
	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// migration006ExplicitCodecHeaders adds high-priority client detection rules
// for explicit codec request headers (X-Video-Codec, X-Audio-Codec).
// These rules allow clients to explicitly request specific codecs by sending
// headers like `X-Video-Codec: h265` or `X-Audio-Codec: opus`.
//
// Priority 1-8 ensures these rules are evaluated before User-Agent detection (priority 100+).
func migration006ExplicitCodecHeaders() Migration {
	return Migration{
		Version:     "006",
		Description: "Add explicit codec header detection rules",
		Up:          createExplicitCodecHeaderRules,
		Down: func(tx *gorm.DB) error {
			// Delete the explicit codec header rules by name pattern
			return tx.Where("name LIKE ?", "Explicit % Request").Delete(&models.ClientDetectionRule{}).Error
		},
	}
}

// createExplicitCodecHeaderRules creates high-priority rules for explicit codec headers.
// These use the @header_req: dynamic field resolver to check for X-Video-Codec and X-Audio-Codec headers.
func createExplicitCodecHeaderRules(tx *gorm.DB) error {
	rules := []models.ClientDetectionRule{
		// Video codec rules (priority 1-4)
		{
			Name:                "Explicit H.265 Video Request",
			Description:         "Serves H.265 when client sends X-Video-Codec: h265 header",
			Expression:          `@header_req:X-Video-Codec equals "h265" OR @header_req:X-Video-Codec equals "hevc"`,
			Priority:            1,
			IsEnabled:           models.BoolPtr(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h265"]`,
			AcceptedAudioCodecs: `["aac","mp3","ac3","eac3","opus"]`,
			PreferredVideoCodec: models.VideoCodecH265,
			PreferredAudioCodec: models.AudioCodecAAC,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(true),
			PreferredFormat:     "",
		},
		{
			Name:                "Explicit H.264 Video Request",
			Description:         "Serves H.264 when client sends X-Video-Codec: h264 header",
			Expression:          `@header_req:X-Video-Codec equals "h264" OR @header_req:X-Video-Codec equals "avc"`,
			Priority:            2,
			IsEnabled:           models.BoolPtr(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264"]`,
			AcceptedAudioCodecs: `["aac","mp3","ac3","eac3","opus"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(true),
			PreferredFormat:     "",
		},
		{
			Name:                "Explicit VP9 Video Request",
			Description:         "Serves VP9 when client sends X-Video-Codec: vp9 header",
			Expression:          `@header_req:X-Video-Codec equals "vp9"`,
			Priority:            3,
			IsEnabled:           models.BoolPtr(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["vp9"]`,
			AcceptedAudioCodecs: `["opus","aac"]`,
			PreferredVideoCodec: models.VideoCodecVP9,
			PreferredAudioCodec: models.AudioCodecOpus,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(false), // VP9 typically in fMP4/WebM
			PreferredFormat:     "dash",
		},
		{
			Name:                "Explicit AV1 Video Request",
			Description:         "Serves AV1 when client sends X-Video-Codec: av1 header",
			Expression:          `@header_req:X-Video-Codec equals "av1"`,
			Priority:            4,
			IsEnabled:           models.BoolPtr(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["av1"]`,
			AcceptedAudioCodecs: `["opus","aac"]`,
			PreferredVideoCodec: models.VideoCodecAV1,
			PreferredAudioCodec: models.AudioCodecOpus,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(false), // AV1 typically in fMP4
			PreferredFormat:     "dash",
		},
		// Audio codec rules (priority 5-8)
		{
			Name:                "Explicit AAC Audio Request",
			Description:         "Prefers AAC when client sends X-Audio-Codec: aac header",
			Expression:          `@header_req:X-Audio-Codec equals "aac"`,
			Priority:            5,
			IsEnabled:           models.BoolPtr(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264","h265","vp9","av1"]`,
			AcceptedAudioCodecs: `["aac"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(true),
			PreferredFormat:     "",
		},
		{
			Name:                "Explicit Opus Audio Request",
			Description:         "Prefers Opus when client sends X-Audio-Codec: opus header",
			Expression:          `@header_req:X-Audio-Codec equals "opus"`,
			Priority:            6,
			IsEnabled:           models.BoolPtr(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264","h265","vp9","av1"]`,
			AcceptedAudioCodecs: `["opus"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecOpus,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(false), // Opus typically in fMP4/WebM
			PreferredFormat:     "",
		},
		{
			Name:                "Explicit AC3 Audio Request",
			Description:         "Prefers AC3 when client sends X-Audio-Codec: ac3 header",
			Expression:          `@header_req:X-Audio-Codec equals "ac3"`,
			Priority:            7,
			IsEnabled:           models.BoolPtr(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264","h265"]`,
			AcceptedAudioCodecs: `["ac3"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAC3,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(true),
			PreferredFormat:     "",
		},
		{
			Name:                "Explicit MP3 Audio Request",
			Description:         "Prefers MP3 when client sends X-Audio-Codec: mp3 header",
			Expression:          `@header_req:X-Audio-Codec equals "mp3"`,
			Priority:            8,
			IsEnabled:           models.BoolPtr(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264","h265"]`,
			AcceptedAudioCodecs: `["mp3"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecMP3,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(true),
			PreferredFormat:     "",
		},
	}

	for i := range rules {
		rules[i].ID = models.NewULID()
		if err := tx.Create(&rules[i]).Error; err != nil {
			return err
		}
	}

	return nil
}
