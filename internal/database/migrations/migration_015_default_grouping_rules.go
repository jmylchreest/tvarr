package migrations

import (
	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// migration015DefaultGroupingRules adds system data mapping rules for channel grouping/categorization
// and default filters for common use cases.
//
// Data Mapping Rules normalize group_title values from various provider formats into
// consistent category names (Sports, Movies, News, etc.).
//
// Filters provide ready-to-use filtering options that users can enable.
//
// Extract Country and Detect Adult are enabled by default; grouping rules are disabled.
func migration015DefaultGroupingRules() Migration {
	return Migration{
		Version:     "015",
		Description: "Add default channel grouping data mapping rules and filters",
		Up: func(tx *gorm.DB) error {
			if err := createGroupingDataMappingRules(tx); err != nil {
				return err
			}
			return createGroupingFilters(tx)
		},
		Down: func(tx *gorm.DB) error {
			// Delete the grouping data mapping rules by name pattern
			groupingRuleNames := []string{
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
			for _, name := range groupingRuleNames {
				if err := tx.Where("name = ? AND is_system = ?", name, true).Delete(&models.DataMappingRule{}).Error; err != nil {
					return err
				}
			}

			// Delete the grouping filters by name
			// Note: "Exclude Adult Content" is NOT in this list as it's created by migration 002
			filterNames := []string{
				"Sports Only",
				"Movies Only",
				"News Only",
				"Kids Safe",
				"Exclude PPV",
				"24/7 On-Demand Only",
			}
			for _, name := range filterNames {
				if err := tx.Where("name = ? AND is_system = ?", name, true).Delete(&models.Filter{}).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// createGroupingDataMappingRules creates system data mapping rules for normalizing channel groups.
// These rules transform provider-specific group_title values into consistent category names.
func createGroupingDataMappingRules(tx *gorm.DB) error {
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
			IsEnabled:   models.BoolPtr(true),
			IsSystem:    true,
		},
		{
			Name:        "Detect Adult Content",
			Description: "Detects adult content based on group_title patterns and sets is_adult flag. Also normalizes group to 'Adult'.",
			SourceType:  models.DataMappingRuleSourceTypeStream,
			Expression:  `group_title matches "(?i)\\b(adult|xxx|18\\+|porn)" SET group_title = "Adult", is_adult = true`,
			Priority:    2,
			StopOnMatch: false,
			IsEnabled:   models.BoolPtr(true),
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
}

// createGroupingFilters creates system filters for common use cases.
// Note: Filters do not have an enabled/disabled state. The enabled state is
// controlled at the proxy-filter relationship level (ProxyFilter.IsActive).
func createGroupingFilters(tx *gorm.DB) error {
	filters := []models.Filter{
		{
			Name:        "Sports Only",
			Description: "Includes only channels in the Sports category. Enable 'Group Sports Channels' data mapping rule first for best results.",
			SourceType:  models.FilterSourceTypeStream,
			Action:      models.FilterActionInclude,
			Expression:  `group_title == "Sports"`,
			IsSystem:    true,
		},
		{
			Name:        "Movies Only",
			Description: "Includes only channels in the Movies category. Enable 'Group Movie Channels' data mapping rule first for best results.",
			SourceType:  models.FilterSourceTypeStream,
			Action:      models.FilterActionInclude,
			Expression:  `group_title == "Movies"`,
			IsSystem:    true,
		},
		{
			Name:        "News Only",
			Description: "Includes only channels in the News category. Enable 'Group News Channels' data mapping rule first for best results.",
			SourceType:  models.FilterSourceTypeStream,
			Action:      models.FilterActionInclude,
			Expression:  `group_title == "News"`,
			IsSystem:    true,
		},
		{
			Name:        "Kids Safe",
			Description: "Includes only channels in the Kids category. Enable 'Group Kids Channels' data mapping rule first for best results.",
			SourceType:  models.FilterSourceTypeStream,
			Action:      models.FilterActionInclude,
			Expression:  `group_title == "Kids"`,
			IsSystem:    true,
		},
		{
			Name:        "Exclude PPV",
			Description: "Excludes pay-per-view channels. Enable 'Group PPV Channels' data mapping rule first for best results.",
			SourceType:  models.FilterSourceTypeStream,
			Action:      models.FilterActionExclude,
			Expression:  `group_title == "PPV"`,
			IsSystem:    true,
		},
		{
			Name:        "24/7 On-Demand Only",
			Description: "Includes only 24/7 on-demand channels. Enable 'Group 24/7 Channels' data mapping rule first for best results.",
			SourceType:  models.FilterSourceTypeStream,
			Action:      models.FilterActionInclude,
			Expression:  `group_title == "24/7"`,
			IsSystem:    true,
		},
	}

	for i := range filters {
		filters[i].ID = models.NewULID()
		if err := tx.Create(&filters[i]).Error; err != nil {
			return err
		}
	}

	return nil
}
