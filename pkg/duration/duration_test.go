package duration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
		wantErr  bool
	}{
		// Standard Go format
		{"hours", "720h", 720 * time.Hour, false},
		{"minutes", "30m", 30 * time.Minute, false},
		{"seconds", "45s", 45 * time.Second, false},
		{"milliseconds", "100ms", 100 * time.Millisecond, false},
		{"combined standard", "1h30m", 90 * time.Minute, false},

		// Extended format with days (short)
		{"days short", "30d", 30 * 24 * time.Hour, false},
		{"single day short", "1d", 24 * time.Hour, false},
		{"days and hours", "1d12h", 36 * time.Hour, false},

		// Days with full words
		{"day singular", "1 day", 24 * time.Hour, false},
		{"days plural", "30 days", 30 * 24 * time.Hour, false},
		{"days no space", "30days", 30 * 24 * time.Hour, false},

		// Extended format with weeks (short)
		{"weeks short", "2w", 14 * 24 * time.Hour, false},
		{"single week short", "1w", 7 * 24 * time.Hour, false},
		{"wk abbrev", "2wk", 14 * 24 * time.Hour, false},
		{"wks abbrev", "2wks", 14 * 24 * time.Hour, false},

		// Weeks with full words
		{"week singular", "1 week", 7 * 24 * time.Hour, false},
		{"weeks plural", "2 weeks", 14 * 24 * time.Hour, false},
		{"weeks no space", "2weeks", 14 * 24 * time.Hour, false},

		// Months
		{"month short", "1mo", 30 * 24 * time.Hour, false},
		{"months short", "2mos", 60 * 24 * time.Hour, false},
		{"month singular", "1 month", 30 * 24 * time.Hour, false},
		{"months plural", "2 months", 60 * 24 * time.Hour, false},

		// Years
		{"year short", "1y", 365 * 24 * time.Hour, false},
		{"year abbrev", "1yr", 365 * 24 * time.Hour, false},
		{"years abbrev", "2yrs", 2 * 365 * 24 * time.Hour, false},
		{"year singular", "1 year", 365 * 24 * time.Hour, false},
		{"years plural", "2 years", 2 * 365 * 24 * time.Hour, false},

		// Complex combinations
		{"weeks and days", "1w2d", 9 * 24 * time.Hour, false},
		{"weeks days hours", "1w2d12h", 9*24*time.Hour + 12*time.Hour, false},
		{"full combo short", "1w2d3h4m5s", 9*24*time.Hour + 3*time.Hour + 4*time.Minute + 5*time.Second, false},
		{"full combo words", "1 week 2 days 3h", 9*24*time.Hour + 3*time.Hour, false},
		{"year month week day", "1y1mo1w1d", (365+30+7+1)*24*time.Hour, false},

		// Case insensitive
		{"DAYS uppercase", "30DAYS", 30 * 24 * time.Hour, false},
		{"Days mixed", "30Days", 30 * 24 * time.Hour, false},
		{"WEEKS uppercase", "2WEEKS", 14 * 24 * time.Hour, false},

		// Zero
		{"zero", "0s", 0, false},
		{"zero hours", "0h", 0, false},

		// Negative
		{"negative days", "-30d", -30 * 24 * time.Hour, false},
		{"negative days words", "-30 days", -30 * 24 * time.Hour, false},
		{"negative hours", "-12h", -12 * time.Hour, false},

		// Large values
		{"one year in days", "365d", 365 * 24 * time.Hour, false},
		{"52 weeks", "52w", 52 * 7 * 24 * time.Hour, false},

		// Standard units as full words
		{"hours word", "3 hours", 3 * time.Hour, false},
		{"hour singular", "1 hour", time.Hour, false},
		{"minutes word", "30 minutes", 30 * time.Minute, false},
		{"minute singular", "1 minute", time.Minute, false},
		{"seconds word", "45 seconds", 45 * time.Second, false},
		{"second singular", "1 second", time.Second, false},
		{"hrs abbrev", "2 hrs", 2 * time.Hour, false},
		{"mins abbrev", "15 mins", 15 * time.Minute, false},
		{"secs abbrev", "30 secs", 30 * time.Second, false},
		{"mixed full words", "2 hours 30 minutes", 2*time.Hour + 30*time.Minute, false},
		{"full words no space", "2hours30minutes", 2*time.Hour + 30*time.Minute, false},

		// Errors
		{"invalid", "invalid", 0, true},
		{"empty", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := Parse(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, d, "Parse(%q) = %v, want %v", tt.input, d, tt.expected)
		})
	}
}

func TestMustParse(t *testing.T) {
	// Valid input should not panic
	assert.NotPanics(t, func() {
		d := MustParse("30d")
		assert.Equal(t, 30*24*time.Hour, d)
	})

	// Invalid input should panic
	assert.Panics(t, func() {
		MustParse("invalid")
	})
}

func TestFormat(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"zero", 0, "0s"},
		{"seconds", 45 * time.Second, "45s"},
		{"minutes", 30 * time.Minute, "30m0s"},
		{"hours", 12 * time.Hour, "12h0m0s"},
		{"one day", 24 * time.Hour, "1d"},
		{"days", 3 * 24 * time.Hour, "3d"},
		{"one week", 7 * 24 * time.Hour, "1w"},
		{"weeks", 2 * 7 * 24 * time.Hour, "2w"},
		{"weeks and days", 9 * 24 * time.Hour, "1w2d"},
		{"weeks days hours", 9*24*time.Hour + 12*time.Hour, "1w2d12h0m0s"},
		{"negative days", -3 * 24 * time.Hour, "-3d"},
		{"one month", 30 * 24 * time.Hour, "1mo"},
		{"two months", 60 * 24 * time.Hour, "2mo"},
		{"month and week", 37 * 24 * time.Hour, "1mo1w"},
		{"one year", 365 * 24 * time.Hour, "1y"},
		{"year and month", (365 + 30) * 24 * time.Hour, "1y1mo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Format(tt.duration)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRoundTrip(t *testing.T) {
	// Test that Parse(Format(d)) == d for various durations
	durations := []time.Duration{
		0,
		time.Second,
		time.Minute,
		time.Hour,
		24 * time.Hour,
		7 * 24 * time.Hour,
		30 * 24 * time.Hour,
		365 * 24 * time.Hour,
	}

	for _, d := range durations {
		formatted := Format(d)
		parsed, err := Parse(formatted)
		require.NoError(t, err, "Parse(Format(%v)) failed: %v", d, err)
		assert.Equal(t, d, parsed, "Round trip failed for %v: formatted=%q, parsed=%v", d, formatted, parsed)
	}
}

func TestParseEquivalence(t *testing.T) {
	// Test that different representations parse to the same duration
	equivalents := [][]string{
		{"1d", "1 day", "24h"},
		{"1w", "1 week", "7d", "7 days", "168h"},
		{"2w", "2 weeks", "2wks", "14d", "14 days", "336h"},
		{"1d12h", "36h"},
		{"1w1d", "1 week 1 day", "8d", "192h"},
		{"1mo", "1 month", "30d", "30 days"},
		{"1y", "1 year", "1yr", "365d", "365 days"},
		{"2mo", "2 months", "60d"},
	}

	for _, group := range equivalents {
		var expected time.Duration
		for i, s := range group {
			d, err := Parse(s)
			require.NoError(t, err)
			if i == 0 {
				expected = d
			} else {
				assert.Equal(t, expected, d, "%q should equal %q", s, group[0])
			}
		}
	}
}
