// Package bytesize provides human-readable byte size parsing and formatting.
// It supports common size units (B, KB, MB, GB, TB, PB) in both SI (1000) and
// binary (1024) bases.
//
// Supported units (case-insensitive):
//   - B: bytes
//   - KB/K: kilobytes (1024 bytes, binary)
//   - MB/M: megabytes (1024^2 bytes, binary)
//   - GB/G: gigabytes (1024^3 bytes, binary)
//   - TB/T: terabytes (1024^4 bytes, binary)
//   - PB/P: petabytes (1024^5 bytes, binary)
//   - KiB, MiB, GiB, TiB, PiB: explicit binary units
//
// Examples:
//   - "5MB" = 5 * 1024 * 1024 bytes
//   - "1.5 GB" = 1.5 * 1024^3 bytes
//   - "500KB" = 500 * 1024 bytes
//   - "1024" = 1024 bytes (no unit = bytes)
package bytesize

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Size represents a byte size as int64.
type Size int64

// Common size constants using binary (1024) base.
const (
	B  Size = 1
	KB Size = 1024
	MB Size = 1024 * KB
	GB Size = 1024 * MB
	TB Size = 1024 * GB
	PB Size = 1024 * TB
)

// unitMultipliers maps unit names to their byte multiplier.
var unitMultipliers = map[string]Size{
	// Bytes
	"b":     B,
	"byte":  B,
	"bytes": B,

	// Kilobytes (binary)
	"k":   KB,
	"kb":  KB,
	"kib": KB,

	// Megabytes (binary)
	"m":   MB,
	"mb":  MB,
	"mib": MB,

	// Gigabytes (binary)
	"g":   GB,
	"gb":  GB,
	"gib": GB,

	// Terabytes (binary)
	"t":   TB,
	"tb":  TB,
	"tib": TB,

	// Petabytes (binary)
	"p":   PB,
	"pb":  PB,
	"pib": PB,
}

// sizePattern matches a number (int or float) followed by optional unit.
var sizePattern = regexp.MustCompile(`(?i)^\s*([0-9]+(?:\.[0-9]+)?)\s*([a-z]*)\s*$`)

// Parse parses a human-readable byte size string.
// Supports integer and floating-point values with optional units.
// If no unit is specified, bytes are assumed.
//
// Examples:
//   - "5MB" → 5242880
//   - "1.5 GB" → 1610612736
//   - "1024" → 1024
//   - "500 KB" → 512000
func Parse(s string) (Size, error) {
	if s == "" {
		return 0, fmt.Errorf("bytesize: empty string")
	}

	matches := sizePattern.FindStringSubmatch(s)
	if matches == nil {
		return 0, fmt.Errorf("bytesize: invalid format %q", s)
	}

	valueStr := matches[1]
	unitStr := strings.ToLower(matches[2])

	// Parse the numeric value
	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return 0, fmt.Errorf("bytesize: invalid number %q: %w", valueStr, err)
	}

	// Determine multiplier
	var multiplier Size = B
	if unitStr != "" {
		var ok bool
		multiplier, ok = unitMultipliers[unitStr]
		if !ok {
			return 0, fmt.Errorf("bytesize: unknown unit %q", unitStr)
		}
	}

	// Calculate final size
	result := Size(value * float64(multiplier))

	return result, nil
}

// MustParse is like Parse but panics if the string cannot be parsed.
// Use only for compile-time constants.
func MustParse(s string) Size {
	size, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return size
}

// Format converts a byte size to a human-readable string.
// Uses the largest appropriate unit that results in a value >= 1.
func Format(s Size) string {
	if s == 0 {
		return "0B"
	}

	negative := s < 0
	if negative {
		s = -s
	}

	var result string

	switch {
	case s >= PB:
		result = formatFloat(float64(s)/float64(PB), "PB")
	case s >= TB:
		result = formatFloat(float64(s)/float64(TB), "TB")
	case s >= GB:
		result = formatFloat(float64(s)/float64(GB), "GB")
	case s >= MB:
		result = formatFloat(float64(s)/float64(MB), "MB")
	case s >= KB:
		result = formatFloat(float64(s)/float64(KB), "KB")
	default:
		result = fmt.Sprintf("%dB", s)
	}

	if negative {
		return "-" + result
	}
	return result
}

// formatFloat formats a float with appropriate precision.
func formatFloat(value float64, unit string) string {
	// Use integer format if it's a whole number
	if value == float64(int64(value)) {
		return fmt.Sprintf("%d%s", int64(value), unit)
	}
	// Otherwise use 1-2 decimal places
	formatted := fmt.Sprintf("%.2f", value)
	// Trim trailing zeros
	formatted = strings.TrimRight(formatted, "0")
	formatted = strings.TrimRight(formatted, ".")
	return formatted + unit
}

// Bytes returns the size in bytes as int64.
func (s Size) Bytes() int64 {
	return int64(s)
}

// String returns a human-readable string representation.
func (s Size) String() string {
	return Format(s)
}

// Int64 returns the size as int64 (alias for Bytes).
func (s Size) Int64() int64 {
	return int64(s)
}
