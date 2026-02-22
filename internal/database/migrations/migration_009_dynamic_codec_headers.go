package migrations

import (
	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// migration009DynamicCodecHeaders replaces individual codec-specific rules with dynamic SET-based rules.
// Instead of separate rules for each codec value (H.264, H.265, VP9, AV1, etc.), we now use
// two rules that dynamically extract and validate codec values from headers using the unified
// @dynamic(path):key syntax:
//
//	@dynamic(request.headers):x-video-codec not_equals "" SET preferred_video_codec = @dynamic(request.headers):x-video-codec
//	@dynamic(request.headers):x-audio-codec not_equals "" SET preferred_audio_codec = @dynamic(request.headers):x-audio-codec
//
// This approach:
// 1. Uses the unified @dynamic(path):key syntax for both conditions and SET actions
// 2. Allows codec values to be extracted from headers dynamically
// 3. Validates extracted values against supported codecs (h264, h265, vp9, av1, aac, mp3, etc.)
// 4. Reduces the number of rules from 8 to 2
// 5. Is consistent with how data mapping rules work
// 6. The @dynamic() syntax is extensible for future use cases (query params, source metadata, etc.)
func migration009DynamicCodecHeaders() Migration {
	return Migration{
		Version:     "009",
		Description: "Replace individual codec rules with dynamic SET-based extraction",
		Up: func(tx *gorm.DB) error {
			// Delete all individual codec-specific rules created by migration 006
			// These are replaced by two dynamic rules
			if err := tx.Exec(`
				DELETE FROM client_detection_rules
				WHERE is_system = 1 AND name IN (
					'Explicit H.265 Video Request',
					'Explicit H.264 Video Request',
					'Explicit VP9 Video Request',
					'Explicit AV1 Video Request',
					'Explicit AAC Audio Request',
					'Explicit Opus Audio Request',
					'Explicit AC3 Audio Request',
					'Explicit MP3 Audio Request'
				)
			`).Error; err != nil {
				return err
			}

			// Create dynamic video codec rule
			// This rule extracts any valid video codec from the X-Video-Codec header
			// using the unified @dynamic(path):key syntax
			videoRule := models.ClientDetectionRule{
				BaseModel:   models.BaseModel{ID: models.NewULID()},
				Name:        "Dynamic Video Codec Header",
				Description: "Extracts video codec from X-Video-Codec header using @dynamic() syntax. Validates: h264, h265/hevc, vp9, av1.",
				Expression:  `@dynamic(request.headers):x-video-codec not_equals "" SET preferred_video_codec = @dynamic(request.headers):x-video-codec`,
				Priority:    1,
				IsEnabled:   new(true),
				IsSystem:    true,
				// AcceptedVideoCodecs left empty - the SET action will determine the codec
				AcceptedAudioCodecs: `["aac","mp3","ac3","eac3","opus"]`,
				PreferredVideoCodec: "", // Will be set dynamically by SET action
				PreferredAudioCodec: models.AudioCodecAAC,
				SupportsFMP4:        new(true),
				SupportsMPEGTS:      new(true),
				PreferredFormat:     "",
			}
			if err := tx.Create(&videoRule).Error; err != nil {
				return err
			}

			// Create dynamic audio codec rule
			// This rule extracts any valid audio codec from the X-Audio-Codec header
			// using the unified @dynamic(path):key syntax
			audioRule := models.ClientDetectionRule{
				BaseModel:           models.BaseModel{ID: models.NewULID()},
				Name:                "Dynamic Audio Codec Header",
				Description:         "Extracts audio codec from X-Audio-Codec header using @dynamic() syntax. Validates: aac, mp3, ac3, eac3, opus.",
				Expression:          `@dynamic(request.headers):x-audio-codec not_equals "" SET preferred_audio_codec = @dynamic(request.headers):x-audio-codec`,
				Priority:            2,
				IsEnabled:           new(true),
				IsSystem:            true,
				AcceptedVideoCodecs: `["h264","h265","vp9","av1"]`,
				// AcceptedAudioCodecs left empty - the SET action will determine the codec
				PreferredVideoCodec: models.VideoCodecH264,
				PreferredAudioCodec: "", // Will be set dynamically by SET action
				SupportsFMP4:        new(true),
				SupportsMPEGTS:      new(true),
				PreferredFormat:     "",
			}
			if err := tx.Create(&audioRule).Error; err != nil {
				return err
			}

			// Create dynamic container format rule
			// This rule extracts container/format preference from the X-Container header
			// using the unified @dynamic(path):key syntax
			containerRule := models.ClientDetectionRule{
				BaseModel:           models.BaseModel{ID: models.NewULID()},
				Name:                "Dynamic Container Format Header",
				Description:         "Extracts container format from X-Container header using @dynamic() syntax. Validates: hls-fmp4, hls-ts, dash, fmp4, mpegts, ts.",
				Expression:          `@dynamic(request.headers):x-container not_equals "" SET preferred_format = @dynamic(request.headers):x-container`,
				Priority:            3,
				IsEnabled:           new(true),
				IsSystem:            true,
				AcceptedVideoCodecs: `["h264","h265","vp9","av1"]`,
				AcceptedAudioCodecs: `["aac","mp3","ac3","eac3","opus"]`,
				PreferredVideoCodec: models.VideoCodecH264,
				PreferredAudioCodec: models.AudioCodecAAC,
				SupportsFMP4:        new(true),
				SupportsMPEGTS:      new(true),
				PreferredFormat:     "", // Will be set dynamically by SET action
			}
			if err := tx.Create(&containerRule).Error; err != nil {
				return err
			}

			return nil
		},
		Down: func(tx *gorm.DB) error {
			// Delete the dynamic rules
			if err := tx.Exec(`
				DELETE FROM client_detection_rules
				WHERE is_system = 1 AND name IN (
					'Dynamic Video Codec Header',
					'Dynamic Audio Codec Header',
					'Dynamic Container Format Header'
				)
			`).Error; err != nil {
				return err
			}

			// Recreate the original individual codec rules from migration 006
			return createExplicitCodecHeaderRulesForRollback(tx)
		},
	}
}

// createExplicitCodecHeaderRulesForRollback recreates the original rules from migration 006.
func createExplicitCodecHeaderRulesForRollback(tx *gorm.DB) error {
	rules := []models.ClientDetectionRule{
		// Video codec rules (priority 1-4)
		{
			Name:                "Explicit H.265 Video Request",
			Description:         "Serves H.265 when client sends X-Video-Codec: h265 header",
			Expression:          `@header_req:X-Video-Codec equals "h265" OR @header_req:X-Video-Codec equals "hevc"`,
			Priority:            1,
			IsEnabled:           new(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h265"]`,
			AcceptedAudioCodecs: `["aac","mp3","ac3","eac3","opus"]`,
			PreferredVideoCodec: models.VideoCodecH265,
			PreferredAudioCodec: models.AudioCodecAAC,
			SupportsFMP4:        new(true),
			SupportsMPEGTS:      new(true),
			PreferredFormat:     "",
		},
		{
			Name:                "Explicit H.264 Video Request",
			Description:         "Serves H.264 when client sends X-Video-Codec: h264 header",
			Expression:          `@header_req:X-Video-Codec equals "h264" OR @header_req:X-Video-Codec equals "avc"`,
			Priority:            2,
			IsEnabled:           new(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264"]`,
			AcceptedAudioCodecs: `["aac","mp3","ac3","eac3","opus"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			SupportsFMP4:        new(true),
			SupportsMPEGTS:      new(true),
			PreferredFormat:     "",
		},
		{
			Name:                "Explicit VP9 Video Request",
			Description:         "Serves VP9 when client sends X-Video-Codec: vp9 header",
			Expression:          `@header_req:X-Video-Codec equals "vp9"`,
			Priority:            3,
			IsEnabled:           new(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["vp9"]`,
			AcceptedAudioCodecs: `["opus","aac"]`,
			PreferredVideoCodec: models.VideoCodecVP9,
			PreferredAudioCodec: models.AudioCodecOpus,
			SupportsFMP4:        new(true),
			SupportsMPEGTS:      new(false),
			PreferredFormat:     "dash",
		},
		{
			Name:                "Explicit AV1 Video Request",
			Description:         "Serves AV1 when client sends X-Video-Codec: av1 header",
			Expression:          `@header_req:X-Video-Codec equals "av1"`,
			Priority:            4,
			IsEnabled:           new(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["av1"]`,
			AcceptedAudioCodecs: `["opus","aac"]`,
			PreferredVideoCodec: models.VideoCodecAV1,
			PreferredAudioCodec: models.AudioCodecOpus,
			SupportsFMP4:        new(true),
			SupportsMPEGTS:      new(false),
			PreferredFormat:     "dash",
		},
		// Audio codec rules (priority 5-8)
		{
			Name:                "Explicit AAC Audio Request",
			Description:         "Prefers AAC when client sends X-Audio-Codec: aac header",
			Expression:          `@header_req:X-Audio-Codec equals "aac"`,
			Priority:            5,
			IsEnabled:           new(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264","h265","vp9","av1"]`,
			AcceptedAudioCodecs: `["aac"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			SupportsFMP4:        new(true),
			SupportsMPEGTS:      new(true),
			PreferredFormat:     "",
		},
		{
			Name:                "Explicit Opus Audio Request",
			Description:         "Prefers Opus when client sends X-Audio-Codec: opus header",
			Expression:          `@header_req:X-Audio-Codec equals "opus"`,
			Priority:            6,
			IsEnabled:           new(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264","h265","vp9","av1"]`,
			AcceptedAudioCodecs: `["opus"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecOpus,
			SupportsFMP4:        new(true),
			SupportsMPEGTS:      new(false),
			PreferredFormat:     "",
		},
		{
			Name:                "Explicit AC3 Audio Request",
			Description:         "Prefers AC3 when client sends X-Audio-Codec: ac3 header",
			Expression:          `@header_req:X-Audio-Codec equals "ac3"`,
			Priority:            7,
			IsEnabled:           new(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264","h265"]`,
			AcceptedAudioCodecs: `["ac3"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAC3,
			SupportsFMP4:        new(true),
			SupportsMPEGTS:      new(true),
			PreferredFormat:     "",
		},
		{
			Name:                "Explicit MP3 Audio Request",
			Description:         "Prefers MP3 when client sends X-Audio-Codec: mp3 header",
			Expression:          `@header_req:X-Audio-Codec equals "mp3"`,
			Priority:            8,
			IsEnabled:           new(true),
			IsSystem:            true,
			AcceptedVideoCodecs: `["h264","h265"]`,
			AcceptedAudioCodecs: `["mp3"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecMP3,
			SupportsFMP4:        new(true),
			SupportsMPEGTS:      new(true),
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
