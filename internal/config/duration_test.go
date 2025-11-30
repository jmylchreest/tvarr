package config

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDuration(t *testing.T) {
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
		{"combined standard", "1h30m", 90 * time.Minute, false},

		// Extended format with days
		{"days", "30d", 30 * 24 * time.Hour, false},
		{"single day", "1d", 24 * time.Hour, false},
		{"days and hours", "1d12h", 36 * time.Hour, false},

		// Extended format with weeks
		{"weeks", "2w", 14 * 24 * time.Hour, false},
		{"single week", "1w", 7 * 24 * time.Hour, false},
		{"weeks and days", "1w2d", 9 * 24 * time.Hour, false},
		{"weeks days hours", "1w2d12h", 9*24*time.Hour + 12*time.Hour, false},

		// Complex combinations
		{"full combo", "1w2d3h4m5s", 9*24*time.Hour + 3*time.Hour + 4*time.Minute + 5*time.Second, false},

		// Zero
		{"zero", "0s", 0, false},

		// Errors
		{"invalid", "invalid", 0, true},
		{"empty", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := ParseDuration(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, d.Duration())
		})
	}
}

func TestDuration_UnmarshalText(t *testing.T) {
	var d Duration
	err := d.UnmarshalText([]byte("30d"))
	require.NoError(t, err)
	assert.Equal(t, 30*24*time.Hour, d.Duration())
}

func TestDuration_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected time.Duration
	}{
		{"string format", `"30d"`, 30 * 24 * time.Hour},
		{"standard hours", `"720h"`, 720 * time.Hour},
		{"weeks", `"2w"`, 14 * 24 * time.Hour},
		{"nanoseconds int", `2592000000000000`, 30 * 24 * time.Hour}, // 30 days in ns
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d Duration
			err := json.Unmarshal([]byte(tt.json), &d)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, d.Duration())
		})
	}
}

func TestDuration_MarshalJSON(t *testing.T) {
	d := Duration(30 * 24 * time.Hour)
	data, err := json.Marshal(d)
	require.NoError(t, err)
	// Should output as "4w2d" or similar human-readable format
	assert.Contains(t, string(data), "d")
}

func TestDuration_String(t *testing.T) {
	tests := []struct {
		name     string
		duration Duration
		contains []string // Check that output contains these substrings
	}{
		{"weeks", Duration(14 * 24 * time.Hour), []string{"2w"}},
		{"days", Duration(3 * 24 * time.Hour), []string{"3d"}},
		{"weeks and days", Duration(9 * 24 * time.Hour), []string{"1w", "2d"}},
		{"hours only", Duration(12 * time.Hour), []string{"12h"}},
		{"zero", Duration(0), []string{"0s"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.duration.String()
			for _, substr := range tt.contains {
				assert.Contains(t, s, substr, "String() = %q should contain %q", s, substr)
			}
		})
	}
}
