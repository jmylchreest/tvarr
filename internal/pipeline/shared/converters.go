// Package shared provides utilities shared between pipeline stages.
package shared

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/pkg/m3u"
	"github.com/jmylchreest/tvarr/pkg/xmltv"
)

// BuildProxyStreamURL builds the proxy stream URL for a channel.
// Format: {baseURL}/proxy/{proxyId}/{channelId}
func BuildProxyStreamURL(baseURL string, proxyID, channelID models.ULID) string {
	// Ensure baseURL doesn't have a trailing slash
	baseURL = strings.TrimSuffix(baseURL, "/")
	return fmt.Sprintf("%s/proxy/%s/%s", baseURL, proxyID.String(), channelID.String())
}

// ChannelToM3UEntry converts a Channel model to an M3U Entry.
// channelNum should be the final channel number (set by the numbering stage).
func ChannelToM3UEntry(ch *models.Channel, channelNum int) *m3u.Entry {
	entry := &m3u.Entry{
		Duration:      -1,
		TvgID:         ch.TvgID,
		TvgName:       ch.TvgName,
		TvgLogo:       ch.TvgLogo,
		GroupTitle:    ch.GroupTitle,
		Title:         ch.ChannelName,
		ChannelNumber: channelNum,
		URL:           ch.StreamURL,
		Extra:         make(map[string]string),
	}

	return entry
}

// ChannelToXMLTVChannel converts a Channel model to an XMLTV Channel.
func ChannelToXMLTVChannel(ch *models.Channel) *xmltv.Channel {
	displayName := ch.TvgName
	if displayName == "" {
		displayName = ch.ChannelName
	}

	channel := &xmltv.Channel{
		ID:          ch.TvgID,
		DisplayName: displayName,
		Icon:        ch.TvgLogo,
	}

	// Add channel number as a second display-name for Jellyfin compatibility
	// Jellyfin parses numeric display-names as channel numbers
	if ch.ChannelNumber > 0 {
		channel.DisplayNames = []string{displayName, fmt.Sprintf("%d", ch.ChannelNumber)}
	} else if displayName != "" {
		channel.DisplayNames = []string{displayName}
	}

	return channel
}

// ProgramToXMLTVProgramme converts an EpgProgram model to an XMLTV Programme.
func ProgramToXMLTVProgramme(prog *models.EpgProgram) *xmltv.Programme {
	xmltvProg := &xmltv.Programme{
		Start:           prog.Start,
		Stop:            prog.Stop,
		Channel:         prog.ChannelID,
		Title:           prog.Title,
		SubTitle:        prog.SubTitle,
		Description:     prog.Description,
		Category:        prog.Category,
		Icon:            prog.Icon,
		EpisodeNum:      prog.EpisodeNum,
		Rating:          prog.Rating,
		Language:        prog.Language,
		IsNew:           prog.IsNew,
		IsPremiere:      prog.IsPremiere,
		IsLive:          prog.IsLive,
		PreviouslyShown: prog.PreviouslyShown,
		Date:            prog.Date,
		StarRating:      prog.StarRating,
		SeasonNumber:    prog.SeasonNumber,
		EpisodeNumber:   prog.EpisodeNumber,
		ProgramID:       prog.ProgramID,
	}

	if prog.Category != "" {
		xmltvProg.Categories = []string{prog.Category}
	}

	// Deserialize credits JSON back to Credits struct
	if prog.Credits != "" {
		var creditsMap map[string][]string
		if err := json.Unmarshal([]byte(prog.Credits), &creditsMap); err == nil {
			xmltvProg.Credits = &xmltv.Credits{
				Directors:  creditsMap["directors"],
				Actors:     creditsMap["actors"],
				Writers:    creditsMap["writers"],
				Producers:  creditsMap["producers"],
				Presenters: creditsMap["presenters"],
			}
		}
	}

	return xmltvProg
}
