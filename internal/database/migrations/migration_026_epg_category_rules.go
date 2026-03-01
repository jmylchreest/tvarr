package migrations

import (
	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// migration026EpgCategoryRules adds system EPG data mapping rules that infer
// programme_category from the parent channel's group_title when no category is
// already present. This enables Jellyfin's Live TV programme sections (Sports,
// Movies, News, Kids, etc.) for providers whose EPG feeds (e.g. Xtream/XC) do
// not include genre/category information.
//
// Rules use SET_IF_EMPTY (?=) so they only fire when programme_category is
// currently empty, preserving real category data from XMLTV sources.
// All rules are enabled by default and marked IsSystem (cannot be deleted).
func migration026EpgCategoryRules() Migration {
	return Migration{
		Version:     "026",
		Description: "Add EPG category inference rules from channel group_title",
		Up: func(tx *gorm.DB) error {
			rules := []models.DataMappingRule{
				{
					Name: "EPG: Infer Sports Category",
					Description: "Sets programme_category to 'Sports' (if empty) when the channel's group_title " +
						"indicates a sports channel. Matches sport, football, soccer, basketball, hockey, " +
						"racing, tennis, golf, boxing, UFC/MMA, major league codes (NBA/NFL/NHL/MLB/MLS), " +
						"F1, dazn, bein sports, PPV events, and ESPN.",
					SourceType:  models.DataMappingRuleSourceTypeEPG,
					Expression:  `channel_group_title matches "(?i)\\b(sport|football|soccer|basket|hockey|racing|tennis|golf|boxing|ufc|mma|nba|nfl|mlb|nhl|mls|f1|formula|dazn|bein|ppv|espn|sporter)" SET programme_category ?= "Sports"`,
					Priority:    10,
					StopOnMatch: false,
					IsEnabled:   new(true),
					IsSystem:    true,
				},
				{
					Name: "EPG: Infer Movies Category",
					Description: "Sets programme_category to 'Movies' (if empty) when the channel's group_title " +
						"indicates a movie/cinema channel. Matches cinema, movie, film, films, kino, cine, " +
						"cinéma, box office.",
					SourceType:  models.DataMappingRuleSourceTypeEPG,
					Expression:  `channel_group_title matches "(?i)\\b(cinema|movie|film|kino|cine|cin[eé]ma|box.?office)" SET programme_category ?= "Movies"`,
					Priority:    11,
					StopOnMatch: false,
					IsEnabled:   new(true),
					IsSystem:    true,
				},
				{
					Name: "EPG: Infer News Category",
					Description: "Sets programme_category to 'News' (if empty) when the channel's group_title " +
						"indicates a news channel. Matches news, haber, noticias, information, actualité.",
					SourceType:  models.DataMappingRuleSourceTypeEPG,
					Expression:  `channel_group_title matches "(?i)\\b(news|haber|noticias|information|actualit)" SET programme_category ?= "News"`,
					Priority:    12,
					StopOnMatch: false,
					IsEnabled:   new(true),
					IsSystem:    true,
				},
				{
					Name: "EPG: Infer Kids Category",
					Description: "Sets programme_category to 'Kids' (if empty) when the channel's group_title " +
						"indicates a children's channel. Matches kids, cartoon, junior, family, children, " +
						"disney, bambini, enfants, djeciji, cocuk, femijë.",
					SourceType:  models.DataMappingRuleSourceTypeEPG,
					Expression:  `channel_group_title matches "(?i)\\b(kids?|cartoon|junior|family|children|disney|bambini|enfants|djecij|cocuk|femij|kinderen|barn)" SET programme_category ?= "Kids"`,
					Priority:    13,
					StopOnMatch: false,
					IsEnabled:   new(true),
					IsSystem:    true,
				},
				{
					Name: "EPG: Infer Documentary Category",
					Description: "Sets programme_category to 'Documentary' (if empty) when the channel's group_title " +
						"indicates a documentary channel. Matches documentary, dokumentar, " +
						"ντοκιμαντ, belgesel.",
					SourceType:  models.DataMappingRuleSourceTypeEPG,
					Expression:  `channel_group_title matches "(?i)\\b(document|ντοκιμαντ|belgesel)" SET programme_category ?= "Documentary"`,
					Priority:    14,
					StopOnMatch: false,
					IsEnabled:   new(true),
					IsSystem:    true,
				},
				{
					Name: "EPG: Infer Music Category",
					Description: "Sets programme_category to 'Music' (if empty) when the channel's group_title " +
						"indicates a music channel. Matches music, musica, musique, müzik, muzik, μουσικ.",
					SourceType:  models.DataMappingRuleSourceTypeEPG,
					Expression:  `channel_group_title matches "(?i)\\b(music|musica|musique|m[uü]zik|μουσικ)" SET programme_category ?= "Music"`,
					Priority:    15,
					StopOnMatch: false,
					IsEnabled:   new(true),
					IsSystem:    true,
				},
			}

			for i := range rules {
				rules[i].ID = models.NewULID()
				if err := tx.Create(&rules[i]).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(tx *gorm.DB) error {
			names := []string{
				"EPG: Infer Sports Category",
				"EPG: Infer Movies Category",
				"EPG: Infer News Category",
				"EPG: Infer Kids Category",
				"EPG: Infer Documentary Category",
				"EPG: Infer Music Category",
			}
			for _, name := range names {
				if err := tx.Where("name = ? AND is_system = ?", name, true).Delete(&models.DataMappingRule{}).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
