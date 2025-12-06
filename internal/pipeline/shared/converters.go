// Package shared provides utilities shared between pipeline stages.
package shared

import (
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

	// Use existing channel number if specified and we're not renumbering
	if ch.ChannelNumber > 0 && channelNum == 0 {
		entry.ChannelNumber = ch.ChannelNumber
	}

	return entry
}

// ChannelToXMLTVChannel converts a Channel model to an XMLTV Channel.
func ChannelToXMLTVChannel(ch *models.Channel) *xmltv.Channel {
	displayName := ch.TvgName
	if displayName == "" {
		displayName = ch.ChannelName
	}

	return &xmltv.Channel{
		ID:          ch.TvgID,
		DisplayName: displayName,
		Icon:        ch.TvgLogo,
	}
}

// ProgramToXMLTVProgramme converts an EpgProgram model to an XMLTV Programme.
func ProgramToXMLTVProgramme(prog *models.EpgProgram) *xmltv.Programme {
	return &xmltv.Programme{
		Start:       prog.Start,
		Stop:        prog.Stop,
		Channel:     prog.ChannelID,
		Title:       prog.Title,
		SubTitle:    prog.SubTitle,
		Description: prog.Description,
		Category:    prog.Category,
		Icon:        prog.Icon,
		EpisodeNum:  prog.EpisodeNum,
		Rating:      prog.Rating,
		Language:    prog.Language,
		IsNew:       prog.IsNew,
		IsPremiere:  prog.IsPremiere,
	}
}
