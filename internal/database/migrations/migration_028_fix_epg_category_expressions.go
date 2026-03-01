package migrations

import (
	"gorm.io/gorm"
)

// migration028FixEpgCategoryExpressions fixes the EPG category inference rules
// created by migration 026. Those rules used the ?= shorthand for SET_IF_EMPTY
// which misparsed when the regex condition contained | alternations — the parser
// treated the | inside the regex string as a logical OR operator, causing a
// syntax error. This migration updates the stored expressions to use the
// SET_IF_EMPTY keyword form instead, which parses correctly.
func migration028FixEpgCategoryExpressions() Migration {
	type fix struct {
		name string
		expr string
	}

	fixes := []fix{
		{
			name: "EPG: Infer Sports Category",
			expr: `channel_group_title matches "(?i)\\b(sport|football|soccer|basket|hockey|racing|tennis|golf|boxing|ufc|mma|nba|nfl|mlb|nhl|mls|f1|formula|dazn|bein|ppv|espn|sporter)" SET_IF_EMPTY programme_category = "Sports"`,
		},
		{
			name: "EPG: Infer Movies Category",
			expr: `channel_group_title matches "(?i)\\b(cinema|movie|film|kino|cine|cin[eé]ma|box.?office)" SET_IF_EMPTY programme_category = "Movies"`,
		},
		{
			name: "EPG: Infer News Category",
			expr: `channel_group_title matches "(?i)\\b(news|haber|noticias|information|actualit)" SET_IF_EMPTY programme_category = "News"`,
		},
		{
			name: "EPG: Infer Kids Category",
			expr: `channel_group_title matches "(?i)\\b(kids?|cartoon|junior|family|children|disney|bambini|enfants|djecij|cocuk|femij|kinderen|barn)" SET_IF_EMPTY programme_category = "Kids"`,
		},
		{
			name: "EPG: Infer Documentary Category",
			expr: `channel_group_title matches "(?i)\\b(document|ντοκιμαντ|belgesel)" SET_IF_EMPTY programme_category = "Documentary"`,
		},
		{
			name: "EPG: Infer Music Category",
			expr: `channel_group_title matches "(?i)\\b(music|musica|musique|m[uü]zik|μουσικ)" SET_IF_EMPTY programme_category = "Music"`,
		},
	}

	return Migration{
		Version:     "028",
		Description: "Fix EPG category rule expressions: replace ?= shorthand with SET_IF_EMPTY keyword",
		Up: func(tx *gorm.DB) error {
			for _, f := range fixes {
				if err := tx.Exec(
					"UPDATE data_mapping_rules SET expression = ? WHERE name = ? AND is_system = 1 AND deleted_at IS NULL",
					f.expr, f.name,
				).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Down: func(tx *gorm.DB) error {
			// Restore the broken ?= expressions (for completeness, though they don't work)
			broken := []fix{
				{
					name: "EPG: Infer Sports Category",
					expr: `channel_group_title matches "(?i)\\b(sport|football|soccer|basket|hockey|racing|tennis|golf|boxing|ufc|mma|nba|nfl|mlb|nhl|mls|f1|formula|dazn|bein|ppv|espn|sporter)" SET programme_category ?= "Sports"`,
				},
				{
					name: "EPG: Infer Movies Category",
					expr: `channel_group_title matches "(?i)\\b(cinema|movie|film|kino|cine|cin[eé]ma|box.?office)" SET programme_category ?= "Movies"`,
				},
				{
					name: "EPG: Infer News Category",
					expr: `channel_group_title matches "(?i)\\b(news|haber|noticias|information|actualit)" SET programme_category ?= "News"`,
				},
				{
					name: "EPG: Infer Kids Category",
					expr: `channel_group_title matches "(?i)\\b(kids?|cartoon|junior|family|children|disney|bambini|enfants|djecij|cocuk|femij|kinderen|barn)" SET programme_category ?= "Kids"`,
				},
				{
					name: "EPG: Infer Documentary Category",
					expr: `channel_group_title matches "(?i)\\b(document|ντοκιμαντ|belgesel)" SET programme_category ?= "Documentary"`,
				},
				{
					name: "EPG: Infer Music Category",
					expr: `channel_group_title matches "(?i)\\b(music|musica|musique|m[uü]zik|μουσικ)" SET programme_category ?= "Music"`,
				},
			}
			for _, f := range broken {
				if err := tx.Exec(
					"UPDATE data_mapping_rules SET expression = ? WHERE name = ? AND is_system = 1 AND deleted_at IS NULL",
					f.expr, f.name,
				).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
