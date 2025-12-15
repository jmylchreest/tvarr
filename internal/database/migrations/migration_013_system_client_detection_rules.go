package migrations

import (
	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// migration013SystemClientDetectionRules adds system client detection rules for popular media players.
// This migration adds rules for:
// - VLC Media Player
// - MPV
// - Kodi
// - Plex
// - Jellyfin
// - Emby
func migration013SystemClientDetectionRules() Migration {
	return Migration{
		Version:     "013",
		Description: "Add system client detection rules for popular media players",
		Up: func(tx *gorm.DB) error {
			return createMediaPlayerClientDetectionRules(tx)
		},
		Down: func(tx *gorm.DB) error {
			// Delete the media player rules by name
			names := []string{
				"VLC Media Player",
				"MPV",
				"Kodi",
				"Plex",
				"Jellyfin",
				"Emby",
			}
			for _, name := range names {
				if err := tx.Where("name = ? AND is_system = ?", name, true).Delete(&models.ClientDetectionRule{}).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// createMediaPlayerClientDetectionRules creates system client detection rules for popular media players.
func createMediaPlayerClientDetectionRules(tx *gorm.DB) error {
	rules := []models.ClientDetectionRule{
		{
			Name:        "VLC Media Player",
			Description: "VLC media player on any platform. Supports h264/h265 video and aac/ac3/eac3/mp3 audio.",
			Expression:  `@dynamic(request.headers):user-agent contains "VLC"`,
			Priority:    210,
			IsEnabled:   models.BoolPtr(true),
			IsSystem:    true,
			// VLC supports most common codecs but not the newest ones
			AcceptedVideoCodecs: `["h264","h265"]`,
			AcceptedAudioCodecs: `["aac","ac3","eac3","mp3"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(true),
			PreferredFormat:     "",
		},
		{
			Name:        "MPV",
			Description: "MPV media player. Supports h264/h265/av1/vp9 video and aac/ac3/eac3/opus/mp3 audio.",
			Expression:  `@dynamic(request.headers):user-agent contains "mpv" OR @dynamic(request.headers):user-agent contains "libmpv"`,
			Priority:    220,
			IsEnabled:   models.BoolPtr(true),
			IsSystem:    true,
			// MPV supports most modern codecs
			AcceptedVideoCodecs: `["h264","h265","av1","vp9"]`,
			AcceptedAudioCodecs: `["aac","ac3","eac3","opus","mp3"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(true),
			PreferredFormat:     "",
		},
		{
			Name:        "Kodi",
			Description: "Kodi media center. Supports h264/h265/av1/vp9 video and aac/ac3/eac3/dts/mp3 audio.",
			Expression:  `@dynamic(request.headers):user-agent contains "Kodi"`,
			Priority:    230,
			IsEnabled:   models.BoolPtr(true),
			IsSystem:    true,
			// Kodi supports most codecs including DTS
			AcceptedVideoCodecs: `["h264","h265","av1","vp9"]`,
			AcceptedAudioCodecs: `["aac","ac3","eac3","dts","mp3"]`,
			PreferredVideoCodec: models.VideoCodecH264,
			PreferredAudioCodec: models.AudioCodecAAC,
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(true),
			PreferredFormat:     "",
		},
		{
			Name:        "Plex",
			Description: "Plex media server/client. Passthrough configuration - no transcoding needed.",
			Expression:  `@dynamic(request.headers):user-agent contains "Plex"`,
			Priority:    240,
			IsEnabled:   models.BoolPtr(true),
			IsSystem:    true,
			// Plex handles its own transcoding, so we pass through everything
			AcceptedVideoCodecs: "",
			AcceptedAudioCodecs: "",
			PreferredVideoCodec: "",
			PreferredAudioCodec: "",
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(true),
			PreferredFormat:     "",
		},
		{
			Name:        "Jellyfin",
			Description: "Jellyfin media server/client. Passthrough configuration - no transcoding needed.",
			Expression:  `@dynamic(request.headers):user-agent contains "Jellyfin"`,
			Priority:    250,
			IsEnabled:   models.BoolPtr(true),
			IsSystem:    true,
			// Jellyfin handles its own transcoding, so we pass through everything
			AcceptedVideoCodecs: "",
			AcceptedAudioCodecs: "",
			PreferredVideoCodec: "",
			PreferredAudioCodec: "",
			SupportsFMP4:        models.BoolPtr(true),
			SupportsMPEGTS:      models.BoolPtr(true),
			PreferredFormat:     "",
		},
		{
			Name:        "Emby",
			Description: "Emby media server/client. Passthrough configuration - no transcoding needed.",
			Expression:  `@dynamic(request.headers):user-agent contains "Emby"`,
			Priority:    260,
			IsEnabled:   models.BoolPtr(true),
			IsSystem:    true,
			// Emby handles its own transcoding, so we pass through everything
			AcceptedVideoCodecs: "",
			AcceptedAudioCodecs: "",
			PreferredVideoCodec: "",
			PreferredAudioCodec: "",
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
