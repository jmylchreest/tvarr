// Package migrations provides database migration management for tvarr.
package migrations

import (
	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// AllMigrations returns all registered migrations in order.
func AllMigrations() []Migration {
	return []Migration{
		// Phase 3: Stream Source Management
		migration001StreamSources(),
		migration002Channels(),
		migration003ManualStreamChannels(),

		// Phase 4: EPG Source Management
		migration004EpgSources(),
		migration005EpgPrograms(),

		// Phase 5: Proxy Configuration
		migration006StreamProxies(),
		migration007ProxySources(),
		migration008ProxyEpgSources(),
		migration009ProxyFilters(),
		migration010ProxyMappingRules(),

		// Phase 6.5: Expression Engine - Filters and Data Mapping Rules
		migration011Filters(),
		migration012DataMappingRules(),
		migration013DefaultFiltersAndRules(),

		// Note: Logo caching (Phase 10) uses file-based storage with
		// in-memory indexing, no database tables required.
	}
}

// migration001StreamSources creates the stream_sources table.
func migration001StreamSources() Migration {
	return Migration{
		Version:     "001",
		Description: "Create stream_sources table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.StreamSource{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("stream_sources")
		},
	}
}

// migration002Channels creates the channels table.
func migration002Channels() Migration {
	return Migration{
		Version:     "002",
		Description: "Create channels table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.Channel{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("channels")
		},
	}
}

// migration003ManualStreamChannels creates the manual_stream_channels table.
func migration003ManualStreamChannels() Migration {
	return Migration{
		Version:     "003",
		Description: "Create manual_stream_channels table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.ManualStreamChannel{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("manual_stream_channels")
		},
	}
}

// migration004EpgSources creates the epg_sources table.
func migration004EpgSources() Migration {
	return Migration{
		Version:     "004",
		Description: "Create epg_sources table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.EpgSource{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("epg_sources")
		},
	}
}

// migration005EpgPrograms creates the epg_programs table.
func migration005EpgPrograms() Migration {
	return Migration{
		Version:     "005",
		Description: "Create epg_programs table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.EpgProgram{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("epg_programs")
		},
	}
}

// migration006StreamProxies creates the stream_proxies table.
func migration006StreamProxies() Migration {
	return Migration{
		Version:     "006",
		Description: "Create stream_proxies table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.StreamProxy{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("stream_proxies")
		},
	}
}

// migration007ProxySources creates the proxy_sources join table.
func migration007ProxySources() Migration {
	return Migration{
		Version:     "007",
		Description: "Create proxy_sources join table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.ProxySource{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("proxy_sources")
		},
	}
}

// migration008ProxyEpgSources creates the proxy_epg_sources join table.
func migration008ProxyEpgSources() Migration {
	return Migration{
		Version:     "008",
		Description: "Create proxy_epg_sources join table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.ProxyEpgSource{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("proxy_epg_sources")
		},
	}
}

// migration009ProxyFilters creates the proxy_filters join table.
func migration009ProxyFilters() Migration {
	return Migration{
		Version:     "009",
		Description: "Create proxy_filters join table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.ProxyFilter{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("proxy_filters")
		},
	}
}

// migration010ProxyMappingRules creates the proxy_mapping_rules join table.
func migration010ProxyMappingRules() Migration {
	return Migration{
		Version:     "010",
		Description: "Create proxy_mapping_rules join table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.ProxyMappingRule{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("proxy_mapping_rules")
		},
	}
}

// migration011Filters creates the filters table for expression-based filtering.
func migration011Filters() Migration {
	return Migration{
		Version:     "011",
		Description: "Create filters table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.Filter{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("filters")
		},
	}
}

// migration012DataMappingRules creates the data_mapping_rules table.
func migration012DataMappingRules() Migration {
	return Migration{
		Version:     "012",
		Description: "Create data_mapping_rules table",
		Up: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&models.DataMappingRule{})
		},
		Down: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("data_mapping_rules")
		},
	}
}

// migration013DefaultFiltersAndRules inserts default filters and data mapping rules.
func migration013DefaultFiltersAndRules() Migration {
	return Migration{
		Version:     "013",
		Description: "Insert default filters and data mapping rules",
		Up: func(tx *gorm.DB) error {
			// Create default filters matching m3u-proxy
			defaultFilters := []models.Filter{
				{
					Name:       "Include All Valid Stream URLs",
					SourceType: models.FilterSourceTypeStream,
					Action:     models.FilterActionInclude,
					Expression: `stream_url starts_with "http"`,
					Priority:   0,
					IsEnabled:  true,
				},
				{
					Name:        "Exclude Adult Content",
					Description: "Excludes channels with adult content keywords in name or group",
					SourceType:  models.FilterSourceTypeStream,
					Action:      models.FilterActionExclude,
					Expression:  `group_title contains "adult" OR group_title contains "xxx" OR group_title contains "porn" OR channel_name contains "adult" OR channel_name contains "xxx" OR channel_name contains "porn"`,
					Priority:    1,
					IsEnabled:   true,
				},
			}

			for _, filter := range defaultFilters {
				if err := tx.Create(&filter).Error; err != nil {
					return err
				}
			}

			// Create default data mapping rules matching m3u-proxy
			defaultRules := []models.DataMappingRule{
				{
					Name:        "Default Timeshift Detection (Regex)",
					Description: "Automatically detects timeshift channels (+1, +24, etc.) and sets tvg-shift field using regex capture groups.",
					SourceType:  models.DataMappingRuleSourceTypeStream,
					Expression:  `channel_name matches ".*[ ](?:\\+([0-9]{1,2})|(-[0-9]{1,2}))([hH]?)(?:$|[ ]).*" AND channel_name not matches ".*(?:start:|stop:|24[-/]7).*" AND tvg_id matches "^.+$" SET tvg_shift = "$1$2"`,
					Priority:    1,
					StopOnMatch: false,
					IsEnabled:   true,
				},
			}

			for _, rule := range defaultRules {
				if err := tx.Create(&rule).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Down: func(tx *gorm.DB) error {
			// Delete default filters
			if err := tx.Where("name IN ?", []string{
				"Include All Valid Stream URLs",
				"Exclude Adult Content",
			}).Delete(&models.Filter{}).Error; err != nil {
				return err
			}

			// Delete default data mapping rules
			if err := tx.Where("name = ?", "Default Timeshift Detection (Regex)").Delete(&models.DataMappingRule{}).Error; err != nil {
				return err
			}

			return nil
		},
	}
}

