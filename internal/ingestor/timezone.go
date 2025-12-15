// Package ingestor provides source ingestion handlers for stream and EPG sources.
package ingestor

import (
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"time"
)

// timezoneOffsetPattern matches timezone offset formats:
// - +HHMM or -HHMM (e.g., +0100, -0530)
// - +HH:MM or -HH:MM (e.g., +01:00, -05:30)
// - Z (UTC)
var timezoneOffsetPattern = regexp.MustCompile(`^([+-])(\d{2}):?(\d{2})$`)

// ParseTimezoneOffset parses a timezone offset string into a time.Duration.
//
// Supported formats:
//   - "+HHMM" or "-HHMM" (e.g., "+0100", "-0530")
//   - "+HH:MM" or "-HH:MM" (e.g., "+01:00", "-05:30")
//   - "Z" (UTC, returns 0)
//   - Empty string (treated as UTC, returns 0)
//
// Returns the offset as a duration (positive for east of UTC, negative for west).
// Returns an error for invalid offset formats.
func ParseTimezoneOffset(offset string) (time.Duration, error) {
	// Handle empty string as UTC
	if offset == "" {
		return 0, nil
	}

	// Handle "Z" as UTC
	if offset == "Z" || offset == "z" {
		return 0, nil
	}

	// Match the pattern
	matches := timezoneOffsetPattern.FindStringSubmatch(offset)
	if matches == nil {
		return 0, fmt.Errorf("invalid timezone offset format: %q", offset)
	}

	// Parse sign
	sign := 1
	if matches[1] == "-" {
		sign = -1
	}

	// Parse hours
	hours, err := strconv.Atoi(matches[2])
	if err != nil {
		return 0, fmt.Errorf("invalid hours in timezone offset %q: %w", offset, err)
	}
	if hours > 14 {
		return 0, fmt.Errorf("invalid timezone offset: hours out of range (0-14): %q", offset)
	}

	// Parse minutes
	minutes, err := strconv.Atoi(matches[3])
	if err != nil {
		return 0, fmt.Errorf("invalid minutes in timezone offset %q: %w", offset, err)
	}
	if minutes > 59 {
		return 0, fmt.Errorf("invalid timezone offset: minutes out of range (0-59): %q", offset)
	}

	// Calculate total duration
	totalMinutes := hours*60 + minutes
	return time.Duration(sign*totalMinutes) * time.Minute, nil
}

// NormalizeProgramTime normalizes a program time to UTC by applying the inverse
// of the detected timezone offset, then applies the user-configured timeshift.
//
// The normalization process:
//  1. Apply inverse of detected offset to convert from source timezone to UTC
//     Example: 14:00 +0100 → subtract 1 hour → 13:00 UTC
//  2. Apply user timeshift to adjust for provider-specific timing issues
//     Example: 13:00 UTC + 1 hour shift → 14:00 UTC
//
// Parameters:
//   - t: The original program time (with or without timezone info)
//   - detectedOffset: The timezone offset detected from the EPG source
//   - timeshiftHours: User-configured timeshift adjustment in hours
//
// Returns the normalized UTC time.
func NormalizeProgramTime(t time.Time, detectedOffset time.Duration, timeshiftHours int) time.Time {
	// Step 1: If the time already has timezone info embedded (from parsing),
	// calling UTC() will correctly convert it. However, if we're given a
	// detected offset that differs, we need to handle that case.
	//
	// The Go time.Parse with format "-0700" already embeds timezone info,
	// so calling .UTC() handles normalization correctly. The detectedOffset
	// parameter is primarily for logging and cases where the time was parsed
	// without timezone info.

	// Convert to UTC first (handles embedded timezone from parsing)
	utcTime := t.UTC()

	// Step 2: Apply user-configured timeshift
	if timeshiftHours != 0 {
		utcTime = utcTime.Add(time.Duration(timeshiftHours) * time.Hour)
	}

	return utcTime
}

// LogTimezoneDetection logs the detected timezone for an EPG source.
// Uses structured logging without emojis per project guidelines.
func LogTimezoneDetection(logger *slog.Logger, sourceName string, sourceID string, detectedOffset string, programCount int) {
	if logger == nil {
		return
	}

	logger.Info("timezone detected during EPG ingestion",
		slog.String("source_name", sourceName),
		slog.String("source_id", sourceID),
		slog.String("detected_timezone", detectedOffset),
		slog.Int("program_count", programCount),
	)
}

// LogTimezoneNormalization logs a timezone normalization event.
// Uses structured logging without emojis per project guidelines.
func LogTimezoneNormalization(logger *slog.Logger, sourceName string, originalOffset string, timeshiftHours int) {
	if logger == nil {
		return
	}

	if timeshiftHours != 0 {
		logger.Debug("normalizing EPG times to UTC with timeshift",
			slog.String("source_name", sourceName),
			slog.String("original_offset", originalOffset),
			slog.Int("timeshift_hours", timeshiftHours),
		)
	} else {
		logger.Debug("normalizing EPG times to UTC",
			slog.String("source_name", sourceName),
			slog.String("original_offset", originalOffset),
		)
	}
}

// FormatTimezoneOffset converts a timezone offset to the standard "+HH:MM" or "-HH:MM" format.
// This is useful for displaying detected timezones in a consistent format.
//
// Examples:
//   - "+0100" → "+01:00"
//   - "-0530" → "-05:30"
//   - "Z" → "+00:00"
//   - "" → "+00:00"
func FormatTimezoneOffset(offset string) string {
	if offset == "" || offset == "Z" || offset == "z" {
		return "+00:00"
	}

	// Already in colon format
	if len(offset) == 6 && offset[3] == ':' {
		return offset
	}

	// Convert "+0000" to "+00:00"
	if len(offset) == 5 {
		return offset[:3] + ":" + offset[3:]
	}

	return offset
}

// GetTimezoneOffsetHours returns the UTC offset in hours for a given timezone.
// Accepts either:
//   - IANA timezone names (e.g., "Europe/Amsterdam", "America/New_York")
//   - Offset strings (e.g., "+01:00", "-05:00", "+0100")
//
// Returns the offset in hours (positive for east of UTC, negative for west).
// Returns 0 and an error for invalid timezone names or formats.
//
// Note: For IANA timezones, the offset is calculated for the current time,
// which accounts for daylight saving time if applicable.
func GetTimezoneOffsetHours(timezone string) (int, error) {
	if timezone == "" || timezone == "Z" || timezone == "z" || timezone == "UTC" {
		return 0, nil
	}

	// First try to parse as an offset string (+01:00, +0100, etc.)
	if len(timezone) >= 5 && (timezone[0] == '+' || timezone[0] == '-') {
		duration, err := ParseTimezoneOffset(timezone)
		if err == nil {
			// Round to nearest hour
			return int(duration.Hours()), nil
		}
	}

	// Try to load as IANA timezone
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return 0, fmt.Errorf("invalid timezone: %q: %w", timezone, err)
	}

	// Get the current offset for this timezone
	_, offset := time.Now().In(loc).Zone()

	// Convert seconds to hours (rounding to nearest hour)
	hours := offset / 3600
	return hours, nil
}

// CalculateAutoShift calculates the recommended EpgShift based on detected timezone.
// Many EPG providers incorrectly send Unix timestamps that are actually in their
// local timezone instead of UTC. This function calculates the inverse offset
// to correct for this common mistake.
//
// Example: Server in Europe/Amsterdam (UTC+1) sends timestamps 1 hour ahead.
// CalculateAutoShift("Europe/Amsterdam") returns -1 to shift times back.
//
// Returns 0 if timezone is UTC or empty, or if there's an error parsing.
func CalculateAutoShift(detectedTimezone string) int {
	hours, err := GetTimezoneOffsetHours(detectedTimezone)
	if err != nil {
		return 0
	}

	// Return inverse offset to compensate for provider's local time timestamps
	return -hours
}
