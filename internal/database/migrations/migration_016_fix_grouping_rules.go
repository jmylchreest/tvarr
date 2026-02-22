package migrations

import (
	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// migration016FixGroupingRules fixes the grouping rules from migration 015:
// - Enables "Extract Country Code" and "Detect Adult Content" by default
// - Reorders priorities so enabled rules are at the top
// - Renames "Normalize X Channels" to "Group X Channels"
// - Removes "Normalize Lifestyle Channels" rule
func migration016FixGroupingRules() Migration {
	return Migration{
		Version:     "016",
		Description: "Fix grouping rules: enable country/adult, reorder priorities, rename to Group",
		Up: func(tx *gorm.DB) error {
			// Delete the old "Normalize" rules and "Lifestyle" rule
			oldNames := []string{
				"Normalize Sports Channels",
				"Normalize Movie Channels",
				"Normalize News Channels",
				"Normalize Documentary Channels",
				"Normalize Kids Channels",
				"Normalize Entertainment Channels",
				"Normalize Comedy Channels",
				"Normalize Drama Channels",
				"Normalize Reality Channels",
				"Normalize Music Channels",
				"Normalize Classic Channels",
				"Normalize 24/7 Channels",
				"Normalize PPV Channels",
				"Normalize Lifestyle Channels",
				"Detect Adult Content",
				"Extract Country Code",
			}
			for _, name := range oldNames {
				if err := tx.Where("name = ? AND is_system = ?", name, true).Delete(&models.DataMappingRule{}).Error; err != nil {
					return err
				}
			}

			// Create the new rules with correct priorities and names
			disabled := false

			rules := []models.DataMappingRule{
				// Enabled by default - run first
				{
					Name:        "Extract Country Code",
					Description: "Extracts country code from group_title prefix pattern (e.g., 'US|', 'UK|', 'FR|') and sets country field.",
					SourceType:  models.DataMappingRuleSourceTypeStream,
					Expression:  `group_title matches "^([A-Z]{2,4})\\|" SET country = "$1"`,
					Priority:    1,
					StopOnMatch: false,
					IsEnabled:   new(true),
					IsSystem:    true,
				},
				{
					Name:        "Detect Adult Content",
					Description: "Detects adult content based on group_title patterns and sets is_adult flag. Also normalizes group to 'Adult'.",
					SourceType:  models.DataMappingRuleSourceTypeStream,
					Expression:  `group_title matches "(?i)\\b(adult|xxx|18\\+|porn)" SET group_title = "Adult", is_adult = true`,
					Priority:    2,
					StopOnMatch: false,
					IsEnabled:   new(true),
					IsSystem:    true,
				},
				// Category grouping rules - disabled by default
				{
					Name:        "Group Sports Channels",
					Description: "Groups various sports-related channels to 'Sports'. Matches patterns like 'sport', 'football', 'soccer', 'hockey', 'racing', 'tennis', 'golf', 'boxing', 'mma', etc.",
					SourceType:  models.DataMappingRuleSourceTypeStream,
					Expression:  `group_title matches "(?i)\\b(sport|football|soccer|basket|hockey|racing|tennis|golf|boxing|ufc|mma|nba|nfl|mlb|nhl|f1|formula)" SET group_title = "Sports"`,
					Priority:    3,
					StopOnMatch: false,
					IsEnabled:   &disabled,
					IsSystem:    true,
				},
				{
					Name:        "Group Movie Channels",
					Description: "Groups movie/cinema channels to 'Movies'. Matches patterns like 'cinema', 'movie', 'film', 'box office'.",
					SourceType:  models.DataMappingRuleSourceTypeStream,
					Expression:  `group_title matches "(?i)\\b(cinema|movie|film|box.?office)" SET group_title = "Movies"`,
					Priority:    4,
					StopOnMatch: false,
					IsEnabled:   &disabled,
					IsSystem:    true,
				},
				{
					Name:        "Group News Channels",
					Description: "Groups news-related channels to 'News'.",
					SourceType:  models.DataMappingRuleSourceTypeStream,
					Expression:  `group_title matches "(?i)\\bnews\\b" SET group_title = "News"`,
					Priority:    5,
					StopOnMatch: false,
					IsEnabled:   &disabled,
					IsSystem:    true,
				},
				{
					Name:        "Group Documentary Channels",
					Description: "Groups documentary channels to 'Documentaries'. Supports multiple languages.",
					SourceType:  models.DataMappingRuleSourceTypeStream,
					Expression:  `group_title matches "(?i)\\b(document|ντοκιμαντ)" SET group_title = "Documentaries"`,
					Priority:    6,
					StopOnMatch: false,
					IsEnabled:   &disabled,
					IsSystem:    true,
				},
				{
					Name:        "Group Kids Channels",
					Description: "Groups kids/children/cartoon channels to 'Kids'. Matches patterns like 'cartoon', 'kids', 'junior', 'family', 'children'.",
					SourceType:  models.DataMappingRuleSourceTypeStream,
					Expression:  `group_title matches "(?i)\\b(cartoon|kids?|junior|family|children)" SET group_title = "Kids"`,
					Priority:    7,
					StopOnMatch: false,
					IsEnabled:   &disabled,
					IsSystem:    true,
				},
				{
					Name:        "Group Entertainment Channels",
					Description: "Groups general/entertainment channels to 'Entertainment'. Supports multiple languages.",
					SourceType:  models.DataMappingRuleSourceTypeStream,
					Expression:  `group_title matches "(?i)\\b(entertainment|general|général|generale|γενικ)" SET group_title = "Entertainment"`,
					Priority:    8,
					StopOnMatch: false,
					IsEnabled:   &disabled,
					IsSystem:    true,
				},
				{
					Name:        "Group Comedy Channels",
					Description: "Groups comedy channels to 'Comedy'.",
					SourceType:  models.DataMappingRuleSourceTypeStream,
					Expression:  `group_title matches "(?i)\\bcomedy\\b" SET group_title = "Comedy"`,
					Priority:    9,
					StopOnMatch: false,
					IsEnabled:   &disabled,
					IsSystem:    true,
				},
				{
					Name:        "Group Drama Channels",
					Description: "Groups drama and crime channels to 'Drama'.",
					SourceType:  models.DataMappingRuleSourceTypeStream,
					Expression:  `group_title matches "(?i)\\b(crime|drama)\\b" SET group_title = "Drama"`,
					Priority:    10,
					StopOnMatch: false,
					IsEnabled:   &disabled,
					IsSystem:    true,
				},
				{
					Name:        "Group Reality Channels",
					Description: "Groups reality TV channels to 'Reality'.",
					SourceType:  models.DataMappingRuleSourceTypeStream,
					Expression:  `group_title matches "(?i)\\breality\\b" SET group_title = "Reality"`,
					Priority:    11,
					StopOnMatch: false,
					IsEnabled:   &disabled,
					IsSystem:    true,
				},
				{
					Name:        "Group Music Channels",
					Description: "Groups music channels to 'Music'. Supports multiple languages.",
					SourceType:  models.DataMappingRuleSourceTypeStream,
					Expression:  `group_title matches "(?i)\\b(music|musica|musique|μουσικ)" SET group_title = "Music"`,
					Priority:    12,
					StopOnMatch: false,
					IsEnabled:   &disabled,
					IsSystem:    true,
				},
				{
					Name:        "Group Classic Channels",
					Description: "Groups classic/retro channels to 'Classics'.",
					SourceType:  models.DataMappingRuleSourceTypeStream,
					Expression:  `group_title matches "(?i)\\bclassic\\b" SET group_title = "Classics"`,
					Priority:    13,
					StopOnMatch: false,
					IsEnabled:   &disabled,
					IsSystem:    true,
				},
				{
					Name:        "Group 24/7 Channels",
					Description: "Groups 24/7 on-demand channels to '24/7'.",
					SourceType:  models.DataMappingRuleSourceTypeStream,
					Expression:  `group_title matches "(?i)\\b24/7\\b" SET group_title = "24/7"`,
					Priority:    14,
					StopOnMatch: false,
					IsEnabled:   &disabled,
					IsSystem:    true,
				},
				{
					Name:        "Group PPV Channels",
					Description: "Groups pay-per-view channels to 'PPV'.",
					SourceType:  models.DataMappingRuleSourceTypeStream,
					Expression:  `group_title matches "(?i)\\bppv\\b" SET group_title = "PPV"`,
					Priority:    15,
					StopOnMatch: false,
					IsEnabled:   &disabled,
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
			// This is a corrective migration - down just deletes the new rules
			// The original migration 015 will recreate them if needed
			newNames := []string{
				"Extract Country Code",
				"Detect Adult Content",
				"Group Sports Channels",
				"Group Movie Channels",
				"Group News Channels",
				"Group Documentary Channels",
				"Group Kids Channels",
				"Group Entertainment Channels",
				"Group Comedy Channels",
				"Group Drama Channels",
				"Group Reality Channels",
				"Group Music Channels",
				"Group Classic Channels",
				"Group 24/7 Channels",
				"Group PPV Channels",
			}
			for _, name := range newNames {
				if err := tx.Where("name = ? AND is_system = ?", name, true).Delete(&models.DataMappingRule{}).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
