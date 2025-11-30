// Package duration provides human-readable duration parsing.
// It extends Go's standard time.ParseDuration with support for days, weeks, and months.
//
// Supported units (case-insensitive, with plural/singular variants):
//   - ns, nanosecond(s): nanoseconds
//   - us/µs, microsecond(s): microseconds
//   - ms, millisecond(s): milliseconds
//   - s, sec, second(s): seconds
//   - m, min, minute(s): minutes
//   - h, hr, hour(s): hours
//   - d, day(s): days (24 hours)
//   - w, wk, week(s): weeks (7 days)
//   - mo, month(s): months (30 days)
//   - y, yr, year(s): years (365 days)
//
// Examples:
//   - "30 days" = 30 days
//   - "2 weeks" = 2 weeks
//   - "1 month" = 30 days
//   - "1w2d12h" = 1 week, 2 days, 12 hours
//   - "720h" = 720 hours (standard Go format)
package duration

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// Day represents 24 hours.
	Day = 24 * time.Hour
	// Week represents 7 days.
	Week = 7 * Day
	// Month represents 30 days (approximate).
	Month = 30 * Day
	// Year represents 365 days (approximate).
	Year = 365 * Day
)

// unitMultipliers maps unit names to their hour multiplier.
// We use hours as the base unit for conversion since Go's time.ParseDuration
// supports up to hours natively.
var unitMultipliers = map[string]int64{
	// Years (365 days)
	"y":     365 * 24,
	"yr":    365 * 24,
	"yrs":   365 * 24,
	"year":  365 * 24,
	"years": 365 * 24,

	// Months (30 days)
	"mo":     30 * 24,
	"mos":    30 * 24,
	"month":  30 * 24,
	"months": 30 * 24,

	// Weeks
	"w":     7 * 24,
	"wk":    7 * 24,
	"wks":   7 * 24,
	"week":  7 * 24,
	"weeks": 7 * 24,

	// Days
	"d":    24,
	"day":  24,
	"days": 24,
}

// standardUnitReplacements maps full word time units to their Go duration equivalents.
// This allows users to write "3 hours" instead of "3h".
var standardUnitReplacements = map[string]string{
	// Hours
	"hour":  "h",
	"hours": "h",
	"hr":    "h",
	"hrs":   "h",

	// Minutes
	"minute":  "m",
	"minutes": "m",
	"min":     "m",
	"mins":    "m",

	// Seconds
	"second":  "s",
	"seconds": "s",
	"sec":     "s",
	"secs":    "s",

	// Milliseconds
	"millisecond":  "ms",
	"milliseconds": "ms",
	"milli":        "ms",
	"millis":       "ms",

	// Microseconds
	"microsecond":  "us",
	"microseconds": "us",
	"micro":        "us",
	"micros":       "us",

	// Nanoseconds
	"nanosecond":  "ns",
	"nanoseconds": "ns",
	"nano":        "ns",
	"nanos":       "ns",
}

// extendedUnitPattern matches extended duration units (years, months, weeks, days)
// with optional whitespace between number and unit.
// Examples: "30d", "30 days", "2weeks", "1 month"
var extendedUnitPattern = regexp.MustCompile(`(?i)(\d+)\s*(years?|yrs?|y|months?|mos?|mo|weeks?|wks?|w|days?|d)`)

// standardUnitPattern matches standard time units written as full words
// with optional whitespace between number and unit.
// Examples: "3 hours", "30 minutes", "5 seconds"
var standardUnitPattern = regexp.MustCompile(`(?i)(\d+)\s*(hours?|hrs?|minutes?|mins?|seconds?|secs?|milliseconds?|millis?|microseconds?|micros?|nanoseconds?|nanos?)`)

// Parse parses a human-readable duration string.
// It extends Go's standard time.ParseDuration with support for:
//   - d/day/days: days (24 hours)
//   - w/wk/week/weeks: weeks (7 days)
//   - mo/month/months: months (30 days)
//   - y/yr/year/years: years (365 days)
//
// Whitespace between number and unit is optional: "30d" and "30 days" are equivalent.
// The function converts extended units to hours before delegating to time.ParseDuration.
func Parse(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("duration: empty string")
	}

	// Normalize: trim whitespace, lowercase for unit matching
	s = strings.TrimSpace(s)

	// Handle negative durations
	negative := false
	if strings.HasPrefix(s, "-") {
		negative = true
		s = strings.TrimPrefix(s, "-")
		s = strings.TrimSpace(s)
	}

	var totalHours int64

	// Extract and convert extended units (years, months, weeks, days) to hours
	remaining := extendedUnitPattern.ReplaceAllStringFunc(s, func(match string) string {
		matches := extendedUnitPattern.FindStringSubmatch(match)
		if len(matches) == 3 {
			value, _ := strconv.ParseInt(matches[1], 10, 64)
			unit := strings.ToLower(matches[2])
			if multiplier, ok := unitMultipliers[unit]; ok {
				totalHours += value * multiplier
			}
		}
		return ""
	})

	// Convert full word time units (hours, minutes, seconds, etc.) to short form
	remaining = standardUnitPattern.ReplaceAllStringFunc(remaining, func(match string) string {
		matches := standardUnitPattern.FindStringSubmatch(match)
		if len(matches) == 3 {
			value := matches[1]
			unit := strings.ToLower(matches[2])
			if shortUnit, ok := standardUnitReplacements[unit]; ok {
				return value + shortUnit
			}
		}
		return match
	})

	// Clean up remaining string (remove extra whitespace between units)
	// Go's duration parser doesn't accept spaces between units
	remaining = strings.TrimSpace(remaining)
	remaining = strings.Join(strings.Fields(remaining), "")

	// Build final duration string
	var durationStr string
	if totalHours > 0 {
		durationStr = fmt.Sprintf("%dh", totalHours)
	}
	if remaining != "" {
		durationStr += remaining
	}

	// Handle empty result
	if durationStr == "" {
		durationStr = "0s"
	}

	// Parse using standard Go duration parser
	d, err := time.ParseDuration(durationStr)
	if err != nil {
		return 0, fmt.Errorf("duration: %w", err)
	}

	if negative {
		d = -d
	}

	return d, nil
}

// MustParse is like Parse but panics if the string cannot be parsed.
// Use only for compile-time constants.
func MustParse(s string) time.Duration {
	d, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return d
}

// Format converts a duration to a human-readable string.
// Uses the largest appropriate units (years, months, weeks, days, hours, etc.).
// Zero components are omitted: 1h0m0s becomes 1h, 1h0m10s becomes 1h10s.
func Format(d time.Duration) string {
	if d == 0 {
		return "0s"
	}

	negative := d < 0
	if negative {
		d = -d
	}

	var result strings.Builder

	// Extract years
	years := d / Year
	d -= years * Year

	// Extract months (from remaining after years)
	months := d / Month
	d -= months * Month

	// Extract weeks
	weeks := d / Week
	d -= weeks * Week

	// Extract days
	days := d / Day
	d -= days * Day

	// Extract hours
	hours := d / time.Hour
	d -= hours * time.Hour

	// Extract minutes
	minutes := d / time.Minute
	d -= minutes * time.Minute

	// Extract seconds
	seconds := d / time.Second
	d -= seconds * time.Second

	// Build result - only include non-zero components
	if years > 0 {
		fmt.Fprintf(&result, "%dy", years)
	}
	if months > 0 {
		fmt.Fprintf(&result, "%dmo", months)
	}
	if weeks > 0 {
		fmt.Fprintf(&result, "%dw", weeks)
	}
	if days > 0 {
		fmt.Fprintf(&result, "%dd", days)
	}
	if hours > 0 {
		fmt.Fprintf(&result, "%dh", hours)
	}
	if minutes > 0 {
		fmt.Fprintf(&result, "%dm", minutes)
	}
	if seconds > 0 {
		fmt.Fprintf(&result, "%ds", seconds)
	}
	// Handle sub-second remainders (milliseconds, microseconds, nanoseconds)
	if d > 0 {
		if d >= time.Millisecond {
			ms := d / time.Millisecond
			d -= ms * time.Millisecond
			fmt.Fprintf(&result, "%dms", ms)
		}
		if d >= time.Microsecond {
			us := d / time.Microsecond
			d -= us * time.Microsecond
			fmt.Fprintf(&result, "%dµs", us)
		}
		if d > 0 {
			fmt.Fprintf(&result, "%dns", d)
		}
	}

	// Handle case where only extended units with no remainder
	if result.Len() == 0 {
		return "0s"
	}

	if negative {
		return "-" + result.String()
	}
	return result.String()
}
