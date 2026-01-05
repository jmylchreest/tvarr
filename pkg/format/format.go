// Package format provides human-readable formatting utilities.
package format

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// =============================================================================
// FILE SIZE FORMATTING
// =============================================================================

// Bytes formats a byte count into human-readable format.
// Example: Bytes(1536) => "1.5 KB"
func Bytes(bytes int64) string {
	if bytes == 0 {
		return "0 B"
	}

	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	sizes := []string{"KB", "MB", "GB", "TB", "PB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), sizes[exp]) //nolint:gosec // G602: exp max is 4 (1024^6 > int64 max)
}

// FileSize is an alias for Bytes for semantic clarity.
var FileSize = Bytes

// =============================================================================
// NUMBER FORMATTING
// =============================================================================

var printer = message.NewPrinter(language.English)

// Number formats a number with thousand separators.
// Example: Number(1234567) => "1,234,567"
func Number(n int64) string {
	return printer.Sprintf("%d", n)
}

// NumberCompact formats a number in compact notation.
// Example: NumberCompact(1234567) => "1.2M"
func NumberCompact(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1_000_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return strconv.FormatInt(n, 10)
	}
}

// Percentage formats a percentage value.
// Example: Percentage(45.678, 1) => "45.7%"
func Percentage(value float64, decimals int) string {
	return fmt.Sprintf("%.*f%%", decimals, value)
}

// =============================================================================
// CRON EXPRESSION DESCRIPTION
// =============================================================================

// CronDescription returns a human-readable description of a 6-field cron expression.
// Fields: seconds minutes hours day-of-month month day-of-week
// Example: CronDescription("0 0 2 * * *") => "Daily at 2AM"
func CronDescription(cronExpr string) string {
	fields := strings.Fields(strings.TrimSpace(cronExpr))
	if len(fields) < 6 {
		return cronExpr // Not a valid 6-field cron
	}

	// Normalize to 6 fields (strip year if 7 fields)
	if len(fields) > 6 {
		fields = fields[:6]
	}

	sec, min, hour, dayOfMonth, _, dayOfWeek := fields[0], fields[1], fields[2], fields[3], fields[4], fields[5]

	// Every minute
	if min == "*" && hour == "*" && dayOfMonth == "*" && dayOfWeek == "*" {
		return "Every minute"
	}

	// Every minute during specific hour
	if min == "*" && hour != "*" && !strings.Contains(hour, "/") {
		if strings.Contains(hour, ",") {
			return fmt.Sprintf("Every minute at %s", formatHourList(hour))
		}
		if strings.Contains(hour, "-") {
			parts := strings.Split(hour, "-")
			if len(parts) == 2 {
				return fmt.Sprintf("Every minute from %s to %s", formatHour(parts[0]), formatHour(parts[1]))
			}
		}
		if h, err := strconv.Atoi(hour); err == nil {
			return fmt.Sprintf("Every minute during %s hour", formatHour(strconv.Itoa(h)))
		}
	}

	// Second intervals
	if strings.Contains(sec, "/") {
		if interval := extractInterval(sec); interval > 0 {
			return fmt.Sprintf("Every %d seconds", interval)
		}
	}

	// Minute intervals
	if strings.Contains(min, "/") {
		if interval := extractInterval(min); interval > 0 {
			if hour != "*" && !strings.Contains(hour, "/") {
				if strings.Contains(hour, ",") {
					return fmt.Sprintf("Every %d minutes at %s", interval, formatHourList(hour))
				}
				if h, err := strconv.Atoi(hour); err == nil {
					return fmt.Sprintf("Every %d minutes during %s hour", interval, formatHour(strconv.Itoa(h)))
				}
			}
			return fmt.Sprintf("Every %d minutes", interval)
		}
	}

	// Hour intervals
	if strings.Contains(hour, "/") {
		if step := extractStep(hour); step != nil {
			// Determine start hour (default to 0 if * or not specified)
			startHour := 0
			if step.start >= 0 {
				startHour = step.start
			}
			// Get the minute value
			minVal := 0
			if m, err := strconv.Atoi(min); err == nil {
				minVal = m
			}
			// Format start time
			startTimeStr := fmt.Sprintf("%02d:%02d", startHour, minVal)
			showFrom := startHour != 0 || minVal != 0

			switch step.interval {
			case 1:
				if showFrom {
					return fmt.Sprintf("Every hour from %s", startTimeStr)
				}
				return "Every hour"
			case 12:
				if showFrom {
					return fmt.Sprintf("Twice daily from %s", startTimeStr)
				}
				return "Twice daily"
			default:
				if showFrom {
					return fmt.Sprintf("Every %d hours from %s", step.interval, startTimeStr)
				}
				return fmt.Sprintf("Every %d hours", step.interval)
			}
		}
	}

	// Every hour at specific minute
	if hour == "*" {
		if m, err := strconv.Atoi(min); err == nil {
			if m == 0 {
				return "Every hour"
			}
			return fmt.Sprintf("Every hour at :%02d", m)
		}
	}

	// Multiple specific hours at specific minute
	if strings.Contains(hour, ",") {
		if m, err := strconv.Atoi(min); err == nil {
			if m == 0 {
				return fmt.Sprintf("Daily at %s", formatHourList(hour))
			}
			return fmt.Sprintf("Daily at :%02d past %s", m, formatHourList(hour))
		}
	}

	// Specific time patterns
	h, hErr := strconv.Atoi(hour)
	m, mErr := strconv.Atoi(min)
	if hErr == nil && mErr == nil {
		timeStr := formatTime(h, m)

		// Day of week patterns
		if dayOfWeek != "*" && dayOfMonth == "*" {
			if strings.Contains(dayOfWeek, ",") {
				days := strings.Split(dayOfWeek, ",")
				dayNames := make([]string, len(days))
				for i, d := range days {
					dayNames[i] = shortDayName(d)
				}
				return fmt.Sprintf("%s at %s", strings.Join(dayNames, ", "), timeStr)
			}
			if strings.Contains(dayOfWeek, "-") {
				parts := strings.Split(dayOfWeek, "-")
				if len(parts) == 2 {
					return fmt.Sprintf("%s-%s at %s", shortDayName(parts[0]), shortDayName(parts[1]), timeStr)
				}
			}
			return fmt.Sprintf("%ss at %s", fullDayName(dayOfWeek), timeStr)
		}

		// Day of month patterns
		if dayOfMonth != "*" {
			if strings.Contains(dayOfMonth, "/") {
				if interval := extractInterval(dayOfMonth); interval > 0 {
					return fmt.Sprintf("Every %d days at %s", interval, timeStr)
				}
			}
			if d, err := strconv.Atoi(dayOfMonth); err == nil {
				return fmt.Sprintf("%s of each month at %s", ordinal(d), timeStr)
			}
		}

		// Daily at specific time
		return fmt.Sprintf("Daily at %s", timeStr)
	}

	// Fallback
	return strings.Join(fields, " ")
}

// Helper functions for cron description

// stepInfo holds start and interval for cron step expressions
type stepInfo struct {
	start    int // -1 means '*' (no specific start)
	interval int
}

func extractStep(field string) *stepInfo {
	idx := strings.Index(field, "/")
	if idx < 0 {
		return nil
	}
	interval, err := strconv.Atoi(field[idx+1:])
	if err != nil {
		return nil
	}
	startPart := field[:idx]
	start := -1
	if startPart != "*" {
		if s, err := strconv.Atoi(startPart); err == nil {
			start = s
		}
	}
	return &stepInfo{start: start, interval: interval}
}

// extractInterval is kept for backward compatibility
func extractInterval(field string) int {
	if step := extractStep(field); step != nil {
		return step.interval
	}
	return 0
}

func formatHour(h string) string {
	hour, err := strconv.Atoi(h)
	if err != nil {
		return h
	}
	if hour == 0 {
		return "12AM"
	}
	if hour == 12 {
		return "12PM"
	}
	if hour > 12 {
		return fmt.Sprintf("%dPM", hour-12)
	}
	return fmt.Sprintf("%dAM", hour)
}

func formatHourList(hourField string) string {
	hours := strings.Split(hourField, ",")
	formatted := make([]string, len(hours))
	for i, h := range hours {
		formatted[i] = formatHour(h)
	}
	if len(formatted) == 2 {
		return fmt.Sprintf("%s and %s", formatted[0], formatted[1])
	}
	if len(formatted) > 2 {
		last := formatted[len(formatted)-1]
		return fmt.Sprintf("%s, and %s", strings.Join(formatted[:len(formatted)-1], ", "), last)
	}
	return formatted[0]
}

func formatTime(hour, minute int) string {
	if hour == 0 && minute == 0 {
		return "midnight"
	}
	if hour == 12 && minute == 0 {
		return "noon"
	}

	period := "AM"
	hour12 := hour
	if hour >= 12 {
		period = "PM"
		if hour > 12 {
			hour12 = hour - 12
		}
	}
	if hour == 0 {
		hour12 = 12
	}

	if minute == 0 {
		return fmt.Sprintf("%d%s", hour12, period)
	}
	return fmt.Sprintf("%d:%02d%s", hour12, minute, period)
}

var dayNames = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
var shortDayNames = []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

func fullDayName(day string) string {
	if d, err := strconv.Atoi(day); err == nil && d >= 0 && d < 7 {
		return dayNames[d]
	}
	return day
}

func shortDayName(day string) string {
	if d, err := strconv.Atoi(day); err == nil && d >= 0 && d < 7 {
		return shortDayNames[d]
	}
	return day
}

func ordinal(n int) string {
	suffix := "th"
	switch n % 10 {
	case 1:
		if n%100 != 11 {
			suffix = "st"
		}
	case 2:
		if n%100 != 12 {
			suffix = "nd"
		}
	case 3:
		if n%100 != 13 {
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

// =============================================================================
// DATE/TIME FORMATTING
// =============================================================================

// RelativeTime formats a time as a relative duration from now.
// Example: RelativeTime(time.Now().Add(-5*time.Minute)) => "5 minutes ago"
func RelativeTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	if diff < 0 {
		diff = -diff
		return formatRelativeFuture(diff)
	}
	return formatRelativePast(diff)
}

func formatRelativePast(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

func formatRelativeFuture(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "in a moment"
	case d < time.Hour:
		mins := int(d.Minutes())
		if mins == 1 {
			return "in 1 minute"
		}
		return fmt.Sprintf("in %d minutes", mins)
	case d < 24*time.Hour:
		hours := int(d.Hours())
		if hours == 1 {
			return "in 1 hour"
		}
		return fmt.Sprintf("in %d hours", hours)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "in 1 day"
		}
		return fmt.Sprintf("in %d days", days)
	}
}

// RelativeTimeShort formats a time as a short relative duration.
// Example: RelativeTimeShort(time.Now().Add(-5*time.Minute)) => "5m ago"
func RelativeTimeShort(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	if diff < 0 {
		return "soon"
	}

	switch {
	case diff < time.Minute:
		return "now"
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(diff.Hours()/24))
	}
}
