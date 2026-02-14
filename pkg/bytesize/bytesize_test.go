package bytesize

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Size
		wantErr  bool
	}{
		// Bytes
		{"bytes numeric only", "1024", 1024, false},
		{"bytes with B", "1024B", 1024, false},
		{"bytes with byte", "100 byte", 100, false},
		{"bytes with bytes", "100 bytes", 100, false},

		// Kilobytes
		{"kilobytes K", "5K", 5 * KB, false},
		{"kilobytes KB", "5KB", 5 * KB, false},
		{"kilobytes KiB", "5KiB", 5 * KB, false},
		{"kilobytes with space", "5 KB", 5 * KB, false},
		{"kilobytes lowercase", "5kb", 5 * KB, false},

		// Megabytes
		{"megabytes M", "10M", 10 * MB, false},
		{"megabytes MB", "10MB", 10 * MB, false},
		{"megabytes MiB", "10MiB", 10 * MB, false},
		{"megabytes with space", "10 MB", 10 * MB, false},

		// Gigabytes
		{"gigabytes G", "2G", 2 * GB, false},
		{"gigabytes GB", "2GB", 2 * GB, false},
		{"gigabytes GiB", "2GiB", 2 * GB, false},

		// Terabytes
		{"terabytes T", "1T", 1 * TB, false},
		{"terabytes TB", "1TB", 1 * TB, false},
		{"terabytes TiB", "1TiB", 1 * TB, false},

		// Petabytes
		{"petabytes P", "1P", 1 * PB, false},
		{"petabytes PB", "1PB", 1 * PB, false},
		{"petabytes PiB", "1PiB", 1 * PB, false},

		// Floating point
		{"float megabytes", "1.5MB", Size(1.5 * float64(MB)), false},
		{"float gigabytes", "2.5GB", Size(2.5 * float64(GB)), false},
		{"float with space", "1.5 GB", Size(1.5 * float64(GB)), false},

		// Case insensitive
		{"uppercase MB", "5MB", 5 * MB, false},
		{"lowercase mb", "5mb", 5 * MB, false},
		{"mixed case Mb", "5Mb", 5 * MB, false},

		// Whitespace
		{"leading whitespace", "  5MB", 5 * MB, false},
		{"trailing whitespace", "5MB  ", 5 * MB, false},
		{"whitespace between", "5 MB", 5 * MB, false},

		// Zero
		{"zero", "0", 0, false},
		{"zero with unit", "0MB", 0, false},

		// Common config values
		{"5 megabytes", "5242880", 5242880, false},
		{"max logo size example", "5MB", 5 * MB, false},

		// Errors
		{"invalid format", "invalid", 0, true},
		{"empty", "", 0, true},
		{"unknown unit", "5XB", 0, true},
		{"negative explicit", "-5MB", 0, true}, // Negative not supported in pattern
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size, err := Parse(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, size, "Parse(%q) = %v, want %v", tt.input, size, tt.expected)
		})
	}
}

func TestMustParse(t *testing.T) {
	// Valid input should not panic
	assert.NotPanics(t, func() {
		size := MustParse("5MB")
		assert.Equal(t, 5*MB, size)
	})

	// Invalid input should panic
	assert.Panics(t, func() {
		MustParse("invalid")
	})
}

func TestFormat(t *testing.T) {
	tests := []struct {
		name     string
		size     Size
		expected string
	}{
		{"zero", 0, "0B"},
		{"bytes", 500, "500B"},
		{"one kilobyte", KB, "1KB"},
		{"kilobytes", 5 * KB, "5KB"},
		{"one megabyte", MB, "1MB"},
		{"megabytes", 10 * MB, "10MB"},
		{"one gigabyte", GB, "1GB"},
		{"gigabytes", 2 * GB, "2GB"},
		{"one terabyte", TB, "1TB"},
		{"one petabyte", PB, "1PB"},
		{"fractional MB", Size(1.5 * float64(MB)), "1.5MB"},
		{"fractional GB", Size(2.25 * float64(GB)), "2.25GB"},
		{"1023 bytes", 1023, "1023B"},
		{"1024 bytes", 1024, "1KB"},
		{"1025 bytes", 1025, "1KB"}, // Rounds to nearest unit
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Format(tt.size)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSize_String(t *testing.T) {
	size := 5 * MB
	assert.Equal(t, "5MB", size.String())
}

func TestSize_Bytes(t *testing.T) {
	size := 5 * MB
	assert.Equal(t, int64(5242880), size.Bytes())
}

func TestConstants(t *testing.T) {
	assert.Equal(t, Size(1), B)
	assert.Equal(t, Size(1024), KB)
	assert.Equal(t, Size(1024*1024), MB)
	assert.Equal(t, Size(1024*1024*1024), GB)
	assert.Equal(t, Size(1024*1024*1024*1024), TB)
	assert.Equal(t, Size(1024*1024*1024*1024*1024), PB)
}

func TestParseEquivalence(t *testing.T) {
	// Test that different representations parse to the same size
	equivalents := [][]string{
		{"1KB", "1 KB", "1kb", "1kib", "1024", "1024B"},
		{"1MB", "1 MB", "1mb", "1mib", "1M"},
		{"1GB", "1 GB", "1gb", "1gib", "1G"},
	}

	for _, group := range equivalents {
		var expected Size
		for i, s := range group {
			size, err := Parse(s)
			require.NoError(t, err, "Failed to parse %q", s)
			if i == 0 {
				expected = size
			} else {
				assert.Equal(t, expected, size, "%q should equal %q", s, group[0])
			}
		}
	}
}

func TestRoundTrip(t *testing.T) {
	// Test that Parse(Format(s)) preserves the value for clean sizes
	sizes := []Size{
		0,
		B,
		KB,
		MB,
		GB,
		TB,
		5 * MB,
		10 * GB,
	}

	for _, s := range sizes {
		formatted := Format(s)
		parsed, err := Parse(formatted)
		require.NoError(t, err, "Parse(Format(%v)) failed: %v", s, err)
		assert.Equal(t, s, parsed, "Round trip failed for %v: formatted=%q, parsed=%v", s, formatted, parsed)
	}
}
