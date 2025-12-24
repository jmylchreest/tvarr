package migrations

import (
	"gorm.io/gorm"
)

// migration024FixDynamicCodecRules fixes the dynamic codec header rules so each rule
// only contributes its own field. Previously, the video rule was setting audio codecs
// (including eac3), which prevented the audio rule from properly restricting to just
// the codec specified in the X-Audio-Codec header.
//
// Changes:
// - Dynamic Video Codec Header: Remove all audio settings (AcceptedAudioCodecs, PreferredAudioCodec)
// - Dynamic Audio Codec Header: Remove all video settings (AcceptedVideoCodecs, PreferredVideoCodec)
// - Dynamic Container Format Header: Remove all codec settings (only sets format)
// - Adjust priorities so video=20, audio=30, container=40 (lower numbers = higher priority)
func migration024FixDynamicCodecRules() Migration {
	return Migration{
		Version:     "024",
		Description: "Fix dynamic codec rules to only contribute their own field",
		Up: func(tx *gorm.DB) error {
			// Update Dynamic Video Codec Header - only video settings
			if err := tx.Exec(`
				UPDATE client_detection_rules
				SET
					accepted_audio_codecs = '',
					preferred_audio_codec = '',
					priority = 20
				WHERE is_system = 1 AND name = 'Dynamic Video Codec Header'
			`).Error; err != nil {
				return err
			}

			// Update Dynamic Audio Codec Header - only audio settings
			if err := tx.Exec(`
				UPDATE client_detection_rules
				SET
					accepted_video_codecs = '',
					preferred_video_codec = '',
					priority = 30
				WHERE is_system = 1 AND name = 'Dynamic Audio Codec Header'
			`).Error; err != nil {
				return err
			}

			// Update Dynamic Container Format Header - only format settings
			if err := tx.Exec(`
				UPDATE client_detection_rules
				SET
					accepted_video_codecs = '',
					accepted_audio_codecs = '',
					preferred_video_codec = '',
					preferred_audio_codec = '',
					priority = 40
				WHERE is_system = 1 AND name = 'Dynamic Container Format Header'
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Down: func(tx *gorm.DB) error {
			// Restore original values from migration 009
			if err := tx.Exec(`
				UPDATE client_detection_rules
				SET
					accepted_audio_codecs = '["aac","mp3","ac3","eac3","opus"]',
					preferred_audio_codec = 'aac',
					priority = 1
				WHERE is_system = 1 AND name = 'Dynamic Video Codec Header'
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				UPDATE client_detection_rules
				SET
					accepted_video_codecs = '["h264","h265","vp9","av1"]',
					preferred_video_codec = 'h264',
					priority = 2
				WHERE is_system = 1 AND name = 'Dynamic Audio Codec Header'
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				UPDATE client_detection_rules
				SET
					accepted_video_codecs = '["h264","h265","vp9","av1"]',
					accepted_audio_codecs = '["aac","mp3","ac3","eac3","opus"]',
					preferred_video_codec = 'h264',
					preferred_audio_codec = 'aac',
					priority = 3
				WHERE is_system = 1 AND name = 'Dynamic Container Format Header'
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
