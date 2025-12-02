package models

// DataMappingRuleSourceType specifies the type of source the rule applies to.
type DataMappingRuleSourceType string

const (
	// DataMappingRuleSourceTypeStream applies to stream/channel data.
	DataMappingRuleSourceTypeStream DataMappingRuleSourceType = "stream"

	// DataMappingRuleSourceTypeEPG applies to EPG/program data.
	DataMappingRuleSourceTypeEPG DataMappingRuleSourceType = "epg"
)

// DataMappingRule represents an expression-based data mapping rule stored in the database.
// Rules are applied in priority order to transform field values.
type DataMappingRule struct {
	BaseModel

	// Name is a human-readable name for this rule.
	Name string `gorm:"size:255;not null" json:"name"`

	// Description provides additional details about the rule.
	Description string `gorm:"size:1024" json:"description,omitempty"`

	// SourceType specifies whether this applies to streams or EPG.
	SourceType DataMappingRuleSourceType `gorm:"size:20;not null;index" json:"source_type"`

	// Expression is the data mapping expression string.
	// Format: <condition> SET <field> = <value>, <field2> = <value2>
	// Example: channel_name contains "BBC" SET group_title = "UK Channels"
	Expression string `gorm:"type:text;not null" json:"expression"`

	// Priority determines the order of rule evaluation (lower = higher priority).
	Priority int `gorm:"default:0;index" json:"priority"`

	// StopOnMatch if true, stops processing subsequent rules when this rule matches.
	StopOnMatch bool `gorm:"default:false" json:"stop_on_match"`

	// IsEnabled determines if the rule is active.
	IsEnabled bool `gorm:"default:true" json:"is_enabled"`

	// IsSystem indicates this is a system-provided default that cannot be edited or deleted.
	// Only IsEnabled can be toggled for system rules.
	IsSystem bool `gorm:"default:false" json:"is_system"`

	// SourceID optionally restricts this rule to a specific source.
	// If nil, the rule applies to all sources of the matching SourceType.
	SourceID *ULID `gorm:"type:varchar(26);index" json:"source_id,omitempty"`
}

// TableName returns the table name for the DataMappingRule model.
func (DataMappingRule) TableName() string {
	return "data_mapping_rules"
}

// Validate checks if the rule configuration is valid.
func (r *DataMappingRule) Validate() error {
	if r.Name == "" {
		return ErrValidation{Field: "name", Message: "name is required"}
	}
	if r.Expression == "" {
		return ErrValidation{Field: "expression", Message: "expression is required"}
	}
	if r.SourceType == "" {
		return ErrValidation{Field: "source_type", Message: "source_type is required"}
	}
	if r.SourceType != DataMappingRuleSourceTypeStream && r.SourceType != DataMappingRuleSourceTypeEPG {
		return ErrValidation{Field: "source_type", Message: "source_type must be 'stream' or 'epg'"}
	}
	return nil
}
