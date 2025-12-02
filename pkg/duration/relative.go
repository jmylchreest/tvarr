package duration

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Common errors for relative time parsing.
var (
	ErrEmptyRelativeString   = errors.New("duration: empty relative time string")
	ErrNoRelativeKeyword     = errors.New("duration: no relative keyword found (ago, from now, before, after, since)")
	ErrMultipleDatesFound    = errors.New("duration: multiple dates found in string; only one anchor date is allowed")
	ErrInvalidDateFormat     = errors.New("duration: could not parse date from string")
	ErrNoDurationFound       = errors.New("duration: no duration component found in relative expression")
	ErrConflictingDirections = errors.New("duration: conflicting direction keywords (e.g., both 'ago' and 'from now')")
)

// Direction indicates whether to add or subtract the duration.
type Direction int

const (
	DirectionPast   Direction = -1 // ago, before, since
	DirectionFuture Direction = 1  // from now, after
)

// relativeKeywords maps keywords to their direction.
var relativeKeywords = map[string]Direction{
	"ago":      DirectionPast,
	"before":   DirectionPast,
	"since":    DirectionPast,
	"prior to": DirectionPast,
	"from now": DirectionFuture,
	"after":    DirectionFuture,
	"later":    DirectionFuture,
}

// Common date formats to try when parsing dates.
// Order matters - more specific formats should come first.
var dateFormats = []string{
	// ISO formats
	"2006-01-02T15:04:05Z07:00", // RFC3339
	"2006-01-02T15:04:05",       // ISO without timezone
	"2006-01-02 15:04:05",       // ISO with space
	"2006-01-02",                // ISO date only

	// US formats
	"January 2, 2006",
	"January 2 2006",
	"Jan 2, 2006",
	"Jan 2 2006",
	"01/02/2006",
	"1/2/2006",

	// European formats
	"2 January 2006",
	"2 Jan 2006",
	"02/01/2006",
	"02-01-2006",

	// Other common formats
	"2006/01/02",
	"02 Jan 2006",
	"Mon, 02 Jan 2006",
	"Monday, January 2, 2006",

	// With times (12-hour)
	"January 2, 2006 3:04 PM",
	"Jan 2, 2006 3:04 PM",
	"01/02/2006 3:04 PM",

	// With times (24-hour)
	"January 2, 2006 15:04",
	"Jan 2, 2006 15:04",
	"01/02/2006 15:04",
}

// datePattern matches potential date strings for extraction.
// This is intentionally broad to catch various formats.
var datePattern = regexp.MustCompile(`(?i)` +
	`(?:` +
	// ISO format: 2006-01-02
	`\d{4}-\d{1,2}-\d{1,2}(?:[T ]\d{1,2}:\d{2}(?::\d{2})?(?:[+-]\d{2}:\d{2}|Z)?)?` +
	`|` +
	// Month name formats: January 2, 2006 or 2 January 2006
	`(?:(?:january|february|march|april|may|june|july|august|september|october|november|december|` +
	`jan|feb|mar|apr|jun|jul|aug|sep|sept|oct|nov|dec)\s+\d{1,2}(?:st|nd|rd|th)?(?:,?\s+\d{4})?` +
	`|` +
	`\d{1,2}(?:st|nd|rd|th)?\s+(?:january|february|march|april|may|june|july|august|september|october|november|december|` +
	`jan|feb|mar|apr|jun|jul|aug|sep|sept|oct|nov|dec)(?:,?\s+\d{4})?)` +
	`(?:\s+\d{1,2}:\d{2}(?::\d{2})?\s*(?:am|pm)?)?` +
	`|` +
	// Numeric formats: 01/02/2006 or 02-01-2006
	`\d{1,2}[/\-]\d{1,2}[/\-]\d{2,4}(?:\s+\d{1,2}:\d{2}(?::\d{2})?\s*(?:am|pm)?)?` +
	`|` +
	// Year first: 2006/01/02
	`\d{4}[/]\d{1,2}[/]\d{1,2}` +
	`)`)

// RelativeResult contains the parsed result of a relative time expression.
type RelativeResult struct {
	Time      time.Time     // The calculated time
	Duration  time.Duration // The parsed duration
	Direction Direction     // Past or future
	Anchor    time.Time     // The anchor date (either found or default)
	UsedNow   bool          // True if current time was used as anchor
}

// ParseRelative parses a relative time expression and returns a time.Time.
//
// Supported formats:
//   - "5 days ago" → now - 5 days
//   - "3 hours from now" → now + 3 hours
//   - "in 2 weeks" → now + 2 weeks
//   - "1 year after Sep 2, 1990" → Sep 2, 1990 + 1 year
//   - "2 weeks before January 1, 2025" → Jan 1, 2025 - 2 weeks
//   - "30 days since 2024-01-01" → 2024-01-01 + 30 days (since = past reference point, future from it)
//
// If no anchor date is found in the string, current time is used.
// Returns error if multiple dates are found in the string.
func ParseRelative(s string) (time.Time, error) {
	result, err := ParseRelativeDetailed(s, time.Now())
	if err != nil {
		return time.Time{}, err
	}
	return result.Time, nil
}

// ParseRelativeFrom is like ParseRelative but uses the provided time as the
// default anchor when no date is found in the string.
func ParseRelativeFrom(s string, defaultAnchor time.Time) (time.Time, error) {
	result, err := ParseRelativeDetailed(s, defaultAnchor)
	if err != nil {
		return time.Time{}, err
	}
	return result.Time, nil
}

// ParseRelativeDetailed parses a relative time expression and returns detailed results.
func ParseRelativeDetailed(s string, defaultAnchor time.Time) (*RelativeResult, error) {
	if s == "" {
		return nil, ErrEmptyRelativeString
	}

	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)

	// Handle "in X duration" format (e.g., "in 5 days")
	if strings.HasPrefix(lower, "in ") {
		remaining := strings.TrimPrefix(s, s[:3]) // Preserve original case for duration
		dur, err := Parse(strings.TrimSpace(remaining))
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrNoDurationFound, err)
		}
		result := &RelativeResult{
			Time:      defaultAnchor.Add(dur),
			Duration:  dur,
			Direction: DirectionFuture,
			Anchor:    defaultAnchor,
			UsedNow:   true,
		}
		return result, nil
	}

	// Find direction keyword and its position
	var foundKeyword string
	var keywordPos int = -1
	var direction Direction

	for keyword, dir := range relativeKeywords {
		pos := strings.Index(lower, keyword)
		if pos != -1 {
			// Check for conflicting keywords
			if foundKeyword != "" && relativeKeywords[foundKeyword] != dir {
				return nil, ErrConflictingDirections
			}
			// Prefer longer keywords (e.g., "from now" over partial matches)
			if foundKeyword == "" || len(keyword) > len(foundKeyword) {
				foundKeyword = keyword
				keywordPos = pos
				direction = dir
			}
		}
	}

	if foundKeyword == "" {
		return nil, ErrNoRelativeKeyword
	}

	// Split string around the keyword
	beforeKeyword := strings.TrimSpace(s[:keywordPos])
	afterKeyword := strings.TrimSpace(s[keywordPos+len(foundKeyword):])

	// Find dates in both parts
	var dateMatches []string
	var datePositions []int

	for _, match := range datePattern.FindAllStringIndex(lower, -1) {
		dateStr := s[match[0]:match[1]]
		dateMatches = append(dateMatches, dateStr)
		datePositions = append(datePositions, match[0])
	}

	if len(dateMatches) > 1 {
		return nil, fmt.Errorf("%w: found %q and %q", ErrMultipleDatesFound, dateMatches[0], dateMatches[1])
	}

	// Determine anchor time
	var anchor time.Time
	var usedNow bool
	var durationStr string

	if len(dateMatches) == 1 {
		// Parse the found date
		parsedDate, err := parseDate(dateMatches[0])
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidDateFormat, err)
		}
		anchor = parsedDate
		usedNow = false

		// Remove the date from the string to get the duration part
		datePos := datePositions[0]
		if datePos < keywordPos {
			// Date is before keyword: "January 1, 2025 2 weeks before" - unusual but handle it
			durationStr = strings.TrimSpace(s[datePos+len(dateMatches[0]) : keywordPos])
		} else {
			// Date is after keyword: "2 weeks after January 1, 2025"
			durationStr = beforeKeyword
		}
	} else {
		// No date found, use default anchor
		anchor = defaultAnchor
		usedNow = true

		// Duration is everything before the keyword
		durationStr = beforeKeyword

		// For "after" and "before" without a date, check if there's anything after the keyword
		// that might be a duration (e.g., "after 5 days" is unusual but possible)
		if durationStr == "" && afterKeyword != "" {
			durationStr = afterKeyword
		}
	}

	// Clean up duration string
	durationStr = strings.TrimSpace(durationStr)
	if durationStr == "" {
		return nil, ErrNoDurationFound
	}

	// Parse the duration
	dur, err := Parse(durationStr)
	if err != nil {
		return nil, fmt.Errorf("%w: could not parse %q as duration: %v", ErrNoDurationFound, durationStr, err)
	}

	// Calculate the result time
	resultTime := anchor.Add(time.Duration(direction) * dur)

	return &RelativeResult{
		Time:      resultTime,
		Duration:  dur,
		Direction: direction,
		Anchor:    anchor,
		UsedNow:   usedNow,
	}, nil
}

// parseDate attempts to parse a date string using multiple formats.
func parseDate(s string) (time.Time, error) {
	// Clean up the string
	s = strings.TrimSpace(s)

	// Remove ordinal suffixes (1st, 2nd, 3rd, 4th)
	s = regexp.MustCompile(`(\d+)(?:st|nd|rd|th)`).ReplaceAllString(s, "$1")

	// Try each format
	for _, format := range dateFormats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	// Try with local timezone
	for _, format := range dateFormats {
		if t, err := time.ParseInLocation(format, s, time.Local); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("could not parse %q as a date", s)
}

// FormatRelative formats a time relative to now in a human-readable way.
func FormatRelative(t time.Time) string {
	return FormatRelativeFrom(t, time.Now())
}

// FormatRelativeFrom formats a time relative to the given anchor.
func FormatRelativeFrom(t time.Time, anchor time.Time) string {
	diff := t.Sub(anchor)

	if diff == 0 {
		return "now"
	}

	if diff < 0 {
		return Format(-diff) + " ago"
	}
	return "in " + Format(diff)
}
