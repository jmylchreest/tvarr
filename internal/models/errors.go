package models

import (
	"errors"
	"fmt"
)

// ValidationError represents a validation error with field and message.
type ValidationError struct {
	Field   string
	Message string
}

// Error implements the error interface.
func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error on field %s: %s", e.Field, e.Message)
}

// Common validation errors for models.
var (
	// ErrNameRequired indicates a required name field is empty.
	ErrNameRequired = errors.New("name is required")

	// ErrURLRequired indicates a required URL field is empty.
	ErrURLRequired = errors.New("url is required")

	// ErrInvalidURL indicates a malformed URL.
	ErrInvalidURL = errors.New("invalid URL format")

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

	// ErrJobTypeRequired indicates a required job type field is empty.
	ErrJobTypeRequired = errors.New("job type is required")

	// ErrStreamURLNotFound indicates a stream URL was not found.
	ErrStreamURLNotFound = errors.New("stream URL not found")

	// ErrLastKnownCodecNotFound indicates a last known codec entry was not found.
	ErrLastKnownCodecNotFound = errors.New("last known codec not found")

	// ErrEncodingProfileNameRequired indicates a required encoding profile name field is empty.
	ErrEncodingProfileNameRequired = errors.New("encoding profile name is required")

	// ErrEncodingProfileNotFound indicates an encoding profile was not found.
	ErrEncodingProfileNotFound = errors.New("encoding profile not found")

	// ErrEncodingProfileInvalidVideoCodec indicates an invalid target video codec.
	ErrEncodingProfileInvalidVideoCodec = errors.New("invalid target video codec: must be h264, h265, vp9, or av1")

	// ErrEncodingProfileInvalidAudioCodec indicates an invalid target audio codec.
	ErrEncodingProfileInvalidAudioCodec = errors.New("invalid target audio codec: must be aac, opus, ac3, eac3, or mp3")

	// ErrEncodingProfileInvalidQualityPreset indicates an invalid quality preset.
	ErrEncodingProfileInvalidQualityPreset = errors.New("invalid quality preset: must be low, medium, high, or ultra")

	// ErrEncodingProfileInvalidHWAccel indicates an invalid hardware acceleration type.
	ErrEncodingProfileInvalidHWAccel = errors.New("invalid hardware acceleration: must be auto, none, cuda, vaapi, qsv, or videotoolbox")

	// ErrEncodingProfileIsSystem indicates an operation is not allowed on system profiles.
	ErrEncodingProfileIsSystem = errors.New("cannot modify or delete system encoding profile")

	// ErrEncodingProfileInUse indicates a profile is in use by proxies and cannot be deleted.
	ErrEncodingProfileInUse = errors.New("encoding profile is in use by one or more stream proxies")
)
