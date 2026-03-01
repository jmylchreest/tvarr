package migrations

import (
	"gorm.io/gorm"
)

// migration029RemoveGroupChannelRules hard-deletes all "Group * Channels" stream
// data mapping rules. These rules normalise channel group_title values in the M3U
// output (e.g. "UK ❖ SPORTS ᴴᴰ" → "Sports"), but that normalisation is irrelevant
// for Jellyfin Live TV where only programme_category matters. The EPG category
// inference rules (added in migration 026) already perform equivalent regex
// matching directly against the raw group_title, so the stream-side Group rules
// are redundant and only add noise to the UI.
//
// Rules deleted: Group Sports Channels, Group Movie Channels, Group News Channels,
// Group Documentary Channels, Group Kids Channels, Group Entertainment Channels,
// Group Comedy Channels, Group Drama Channels, Group Reality Channels,
// Group Music Channels, Group Classic Channels, Group 24/7 Channels,
// Group PPV Channels.
//
// Extract Country Code and Detect Adult Content are NOT deleted — they serve
// distinct purposes (country extraction and adult content flagging respectively).
func migration029RemoveGroupChannelRules() Migration {
	names := []string{
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

	return Migration{
		Version:     "029",
		Description: "Hard-delete Group * Channels stream mapping rules (superseded by EPG category inference rules)",
		Up: func(tx *gorm.DB) error {
			for _, name := range names {
				if err := tx.Exec(
					"DELETE FROM data_mapping_rules WHERE name = ? AND source_type = 'stream' AND is_system = 1",
					name,
				).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Down: func(tx *gorm.DB) error {
			// Cannot restore hard-deleted rows; this migration is irreversible.
			// Re-running migration 016 Up would recreate them.
			return nil
		},
	}
}
