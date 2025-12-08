package models

// FilterSourceType specifies the type of source the filter applies to.
type FilterSourceType string

const (
	// FilterSourceTypeStream applies to stream/channel data.
	FilterSourceTypeStream FilterSourceType = "stream"

	// FilterSourceTypeEPG applies to EPG/program data.
	FilterSourceTypeEPG FilterSourceType = "epg"
)

// FilterAction specifies the filter behavior.
type FilterAction string

const (
	// FilterActionInclude includes matching records.
	FilterActionInclude FilterAction = "include"

	// FilterActionExclude excludes matching records.
	FilterActionExclude FilterAction = "exclude"
)

// Filter represents an expression-based filter rule stored in the database.
type Filter struct {
	BaseModel

	// Name is a human-readable name for this filter.
	Name string `gorm:"size:255;not null" json:"name"`

	// Description provides additional details about the filter.
	Description string `gorm:"size:1024" json:"description,omitempty"`

	// SourceType specifies whether this applies to streams or EPG.
	SourceType FilterSourceType `gorm:"size:20;not null;index" json:"source_type"`

	// Action specifies whether to include or exclude matching records.
	Action FilterAction `gorm:"size:20;not null;default:'include'" json:"action"`

	// Expression is the filter expression string.
	Expression string `gorm:"type:text;not null" json:"expression"`

	// Priority determines the order of filter evaluation (lower = higher priority).
	Priority int `gorm:"default:0" json:"priority"`

	// IsEnabled determines if the filter is active.
	IsEnabled bool `gorm:"default:true" json:"is_enabled"`

	// IsSystem indicates this is a system-provided default that cannot be edited or deleted.
	// Only IsEnabled can be toggled for system filters.
	IsSystem bool `gorm:"default:false" json:"is_system"`

	// SourceID optionally restricts this filter to a specific source.
	// If nil, the filter applies to all sources of the matching SourceType.
	SourceID *ULID `gorm:"type:varchar(26);index" json:"source_id,omitempty"`
}

// TableName returns the table name for the Filter model.
func (Filter) TableName() string {
	return "filters"
}

// Validate checks if the filter configuration is valid.
func (f *Filter) Validate() error {
	if f.Name == "" {
		return ValidationError{Field: "name", Message: "name is required"}
	}
	if f.Expression == "" {
		return ValidationError{Field: "expression", Message: "expression is required"}
	}
	if f.SourceType == "" {
		return ValidationError{Field: "source_type", Message: "source_type is required"}
	}
	if f.SourceType != FilterSourceTypeStream && f.SourceType != FilterSourceTypeEPG {
		return ValidationError{Field: "source_type", Message: "source_type must be 'stream' or 'epg'"}
	}
	if f.Action == "" {
		f.Action = FilterActionInclude
	}
	if f.Action != FilterActionInclude && f.Action != FilterActionExclude {
		return ValidationError{Field: "action", Message: "action must be 'include' or 'exclude'"}
	}
	return nil
}
