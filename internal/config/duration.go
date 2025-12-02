// Package config provides configuration loading and validation for tvarr.
package config

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmylchreest/tvarr/pkg/duration"
)

// Duration is a time.Duration that supports human-readable parsing.
// It extends Go's standard duration format with support for:
//   - d: days (24 hours)
//   - w: weeks (7 days)
//
// Examples:
//   - "30d" = 30 days
//   - "2w" = 2 weeks
//   - "1w2d12h" = 1 week, 2 days, 12 hours
//   - "720h" = 720 hours (standard Go format still works)
//
// This type implements encoding.TextUnmarshaler for Viper/YAML support
// and json.Unmarshaler for JSON configuration files.
type Duration time.Duration

// ParseDuration parses a human-readable duration string.
// Supports standard Go duration format plus 'd' (days) and 'w' (weeks).
func ParseDuration(s string) (Duration, error) {
	d, err := duration.Parse(s)
	if err != nil {
		return 0, err
	}
	return Duration(d), nil
}

// UnmarshalText implements encoding.TextUnmarshaler for YAML/Viper support.
func (d *Duration) UnmarshalText(text []byte) error {
	parsed, err := ParseDuration(string(text))
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		// Try as a number (nanoseconds) for backwards compatibility
		var ns int64
		if err := json.Unmarshal(data, &ns); err != nil {
			return err
		}
		*d = Duration(ns)
		return nil
	}
	return d.UnmarshalText([]byte(s))
}

// MarshalJSON implements json.Marshaler.
// Outputs in the most human-readable format possible.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// MarshalText implements encoding.TextMarshaler.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.String()), nil
}

// Duration returns the underlying time.Duration.
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

// String returns a human-readable string representation.
// Uses the most appropriate unit (weeks, days, hours, etc.).
func (d Duration) String() string {
	dur := time.Duration(d)

	// Handle zero
	if dur == 0 {
		return "0s"
	}

	// Build human-readable string
	var result string
	negative := dur < 0
	if negative {
		dur = -dur
	}

	// Extract weeks
	weeks := dur / (7 * 24 * time.Hour)
	dur -= weeks * 7 * 24 * time.Hour

	// Extract days
	days := dur / (24 * time.Hour)
	dur -= days * 24 * time.Hour

	// Use standard duration for remainder
	if weeks > 0 {
		result += fmt.Sprintf("%dw", weeks)
	}
	if days > 0 {
		result += fmt.Sprintf("%dd", days)
	}
	if dur > 0 {
		result += dur.String()
	}

	if negative {
		result = "-" + result
	}

	// If no weeks/days, just use standard format
	if result == "" {
		return time.Duration(d).String()
	}

	return result
}
