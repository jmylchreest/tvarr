package models

import (
	"errors"
	"fmt"
)

// ErrValidation represents a validation error with field and message.
type ErrValidation struct {
	Field   string
	Message string
}

// Error implements the error interface.
func (e ErrValidation) Error() string {
	return fmt.Sprintf("validation error on field %s: %s", e.Field, e.Message)
}

// Common validation errors for models.
var (
	// ErrNameRequired indicates a required name field is empty.
	ErrNameRequired = errors.New("name is required")

	// ErrURLRequired indicates a required URL field is empty.
	ErrURLRequired = errors.New("url is required")

	// ErrInvalidSourceType indicates an invalid source type.
	ErrInvalidSourceType = errors.New("invalid source type: must be 'm3u' or 'xtream'")

	// ErrXtreamCredentialsRequired indicates missing Xtream credentials.
	ErrXtreamCredentialsRequired = errors.New("username and password are required for xtream sources")

	// ErrInvalidEpgSourceType indicates an invalid EPG source type.
	ErrInvalidEpgSourceType = errors.New("invalid epg source type: must be 'xmltv' or 'xtream'")

	// ErrExpressionRequired indicates a required expression field is empty.
	ErrExpressionRequired = errors.New("expression is required")

	// ErrInvalidFilterAction indicates an invalid filter action.
	ErrInvalidFilterAction = errors.New("invalid filter action: must be 'include' or 'exclude'")

	// ErrStreamURLRequired indicates a required stream URL field is empty.
	ErrStreamURLRequired = errors.New("stream_url is required")

	// ErrChannelIDRequired indicates a required channel ID field is empty.
	ErrChannelIDRequired = errors.New("channel_id is required")

	// ErrStartTimeRequired indicates a required start time field is empty.
	ErrStartTimeRequired = errors.New("start time is required")

	// ErrEndTimeRequired indicates a required end time field is empty.
	ErrEndTimeRequired = errors.New("end time is required")

	// ErrTitleRequired indicates a required title field is empty.
	ErrTitleRequired = errors.New("title is required")

	// ErrInvalidTimeRange indicates end time is before start time.
	ErrInvalidTimeRange = errors.New("end time must be after start time")

	// ErrSourceIDRequired indicates a required source ID field is zero.
	ErrSourceIDRequired = errors.New("source_id is required")

	// ErrProxyIDRequired indicates a required proxy ID field is zero.
	ErrProxyIDRequired = errors.New("proxy_id is required")

	// ErrEpgSourceIDRequired indicates a required EPG source ID field is zero.
	ErrEpgSourceIDRequired = errors.New("epg_source_id is required")

	// ErrFilterIDRequired indicates a required filter ID field is zero.
	ErrFilterIDRequired = errors.New("filter_id is required")

	// ErrMappingRuleIDRequired indicates a required mapping rule ID field is zero.
	ErrMappingRuleIDRequired = errors.New("mapping_rule_id is required")

	// ErrFilePathRequired indicates a required file path field is empty.
	ErrFilePathRequired = errors.New("file_path is required")
)
