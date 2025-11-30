package duration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRelative_SimpleExpressions(t *testing.T) {
	// Use a fixed anchor time for predictable tests
	anchor := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		input    string
		expected time.Time
	}{
		// Past (ago)
		{"5 days ago", "5 days ago", anchor.Add(-5 * Day)},
		{"3 hours ago", "3 hours ago", anchor.Add(-3 * time.Hour)},
		{"1 week ago", "1 week ago", anchor.Add(-Week)},
		{"2 months ago", "2 months ago", anchor.Add(-2 * Month)},
		{"30 minutes ago", "30 minutes ago", anchor.Add(-30 * time.Minute)},
		{"1 year ago", "1 year ago", anchor.Add(-Year)},

		// Future (from now)
		{"5 days from now", "5 days from now", anchor.Add(5 * Day)},
		{"3 hours from now", "3 hours from now", anchor.Add(3 * time.Hour)},
		{"1 week from now", "1 week from now", anchor.Add(Week)},
		{"2 months from now", "2 months from now", anchor.Add(2 * Month)},

		// Future (in X)
		{"in 5 days", "in 5 days", anchor.Add(5 * Day)},
		{"in 3 hours", "in 3 hours", anchor.Add(3 * time.Hour)},
		{"in 1 week", "in 1 week", anchor.Add(Week)},
		{"in 2 months", "in 2 months", anchor.Add(2 * Month)},

		// Future (later)
		{"5 days later", "5 days later", anchor.Add(5 * Day)},

		// Complex durations
		{"1w2d ago", "1w2d ago", anchor.Add(-(Week + 2*Day))},
		{"1 week 2 days ago", "1 week 2 days ago", anchor.Add(-(Week + 2*Day))},
		{"in 1w2d", "in 1w2d", anchor.Add(Week + 2*Day)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseRelativeFrom(tt.input, anchor)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result, "ParseRelativeFrom(%q) = %v, want %v", tt.input, result, tt.expected)
		})
	}
}

func TestParseRelative_AnchoredExpressions(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedDate time.Time
	}{
		// After date
		{
			name:         "1 year after Sep 2, 1990",
			input:        "1 year after Sep 2, 1990",
			expectedDate: time.Date(1991, 9, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "5 days after January 1, 2025",
			input:        "5 days after January 1, 2025",
			expectedDate: time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "2 weeks after 2024-06-15",
			input:        "2 weeks after 2024-06-15",
			expectedDate: time.Date(2024, 6, 29, 0, 0, 0, 0, time.UTC),
		},

		// Before date
		{
			name:         "2 weeks before January 1, 2025",
			input:        "2 weeks before January 1, 2025",
			expectedDate: time.Date(2024, 12, 18, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "1 month before Dec 25, 2024",
			input:        "1 month before Dec 25, 2024",
			expectedDate: time.Date(2024, 11, 25, 0, 0, 0, 0, time.UTC),
		},
		{
			name:         "3 days before 2025-01-01",
			input:        "3 days before 2025-01-01",
			expectedDate: time.Date(2024, 12, 29, 0, 0, 0, 0, time.UTC),
		},

		// Since date (treated as past reference, so we go forward from it)
		{
			name:         "30 days since 2024-01-01",
			input:        "30 days since 2024-01-01",
			expectedDate: time.Date(2023, 12, 2, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseRelative(tt.input)
			require.NoError(t, err, "ParseRelative(%q) failed", tt.input)

			// Compare dates (ignore time components for date-only tests)
			expectedY, expectedM, expectedD := tt.expectedDate.Date()
			resultY, resultM, resultD := result.Date()
			assert.Equal(t, expectedY, resultY, "year mismatch for %q", tt.input)
			assert.Equal(t, expectedM, resultM, "month mismatch for %q", tt.input)
			assert.Equal(t, expectedD, resultD, "day mismatch for %q", tt.input)
		})
	}
}

func TestParseRelative_DateFormats(t *testing.T) {
	// Test that various date formats are recognized
	tests := []struct {
		name  string
		input string
	}{
		{"ISO date", "1 day after 2024-06-15"},
		{"US format", "1 day after January 15, 2024"},
		{"US format short", "1 day after Jan 15, 2024"},
		{"European format", "1 day after 15 January 2024"},
		{"Slash format", "1 day after 01/15/2024"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRelative(tt.input)
			assert.NoError(t, err, "ParseRelative(%q) should parse successfully", tt.input)
		})
	}
}

func TestParseRelativeDetailed(t *testing.T) {
	anchor := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	t.Run("returns detailed result for simple expression", func(t *testing.T) {
		result, err := ParseRelativeDetailed("5 days ago", anchor)
		require.NoError(t, err)

		assert.Equal(t, 5*Day, result.Duration)
		assert.Equal(t, DirectionPast, result.Direction)
		assert.Equal(t, anchor, result.Anchor)
		assert.True(t, result.UsedNow)
		assert.Equal(t, anchor.Add(-5*Day), result.Time)
	})

	t.Run("returns detailed result for anchored expression", func(t *testing.T) {
		result, err := ParseRelativeDetailed("1 week after Jan 1, 2025", anchor)
		require.NoError(t, err)

		assert.Equal(t, Week, result.Duration)
		assert.Equal(t, DirectionFuture, result.Direction)
		assert.False(t, result.UsedNow)
		// Anchor should be the parsed date
		anchorY, anchorM, anchorD := result.Anchor.Date()
		assert.Equal(t, 2025, anchorY)
		assert.Equal(t, time.January, anchorM)
		assert.Equal(t, 1, anchorD)
	})
}

func TestParseRelative_Errors(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedErr error
	}{
		{"empty string", "", ErrEmptyRelativeString},
		{"no keyword", "5 days", ErrNoRelativeKeyword},
		{"just a date", "January 1, 2025", ErrNoRelativeKeyword},
		{"no duration", "ago", ErrNoDurationFound},
		{"invalid duration", "xyz ago", ErrNoDurationFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRelative(tt.input)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.expectedErr)
		})
	}
}

func TestParseRelative_MultipleDates(t *testing.T) {
	_, err := ParseRelative("1 day after Jan 1, 2025 and Feb 1, 2025")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMultipleDatesFound)
}

func TestParseRelative_CaseInsensitive(t *testing.T) {
	anchor := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	tests := []string{
		"5 DAYS AGO",
		"5 Days Ago",
		"5 days AGO",
		"IN 5 DAYS",
		"In 5 Days",
		"5 days FROM NOW",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := ParseRelativeFrom(input, anchor)
			assert.NoError(t, err, "ParseRelativeFrom(%q) should succeed", input)
		})
	}
}

func TestFormatRelative(t *testing.T) {
	anchor := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		time     time.Time
		expected string
	}{
		{"same time", anchor, "now"},
		{"5 days ago", anchor.Add(-5 * Day), "5d ago"},
		{"3 hours ago", anchor.Add(-3 * time.Hour), "3h0m0s ago"},
		{"in 2 weeks", anchor.Add(2 * Week), "in 2w"},
		{"in 1 day", anchor.Add(Day), "in 1d"},
		{"1 year ago", anchor.Add(-Year), "1y ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatRelativeFrom(tt.time, anchor)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseRelative_EdgeCases(t *testing.T) {
	anchor := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)

	t.Run("whitespace handling", func(t *testing.T) {
		result, err := ParseRelativeFrom("  5 days ago  ", anchor)
		require.NoError(t, err)
		assert.Equal(t, anchor.Add(-5*Day), result)
	})

	t.Run("mixed case keywords", func(t *testing.T) {
		result, err := ParseRelativeFrom("5 days FROM NOW", anchor)
		require.NoError(t, err)
		assert.Equal(t, anchor.Add(5*Day), result)
	})

	t.Run("zero duration", func(t *testing.T) {
		result, err := ParseRelativeFrom("0 days ago", anchor)
		require.NoError(t, err)
		assert.Equal(t, anchor, result)
	})
}

func TestParseDate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Time
		wantErr  bool
	}{
		{"ISO date", "2024-06-15", time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC), false},
		{"US month day year", "January 15, 2024", time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), false},
		{"US month day year short", "Jan 15, 2024", time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), false},
		{"European format", "15 January 2024", time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), false},
		{"ordinal suffix", "January 15th, 2024", time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), false},
		{"invalid", "not a date", time.Time{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseDate(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			// Compare date parts only (time.Parse may use different locations)
			expectedY, expectedM, expectedD := tt.expected.Date()
			resultY, resultM, resultD := result.Date()
			assert.Equal(t, expectedY, resultY, "year")
			assert.Equal(t, expectedM, resultM, "month")
			assert.Equal(t, expectedD, resultD, "day")
		})
	}
}

func TestDirection_Values(t *testing.T) {
	assert.Equal(t, Direction(-1), DirectionPast)
	assert.Equal(t, Direction(1), DirectionFuture)
}

func TestRelativeKeywords(t *testing.T) {
	// Verify all keywords are mapped correctly
	pastKeywords := []string{"ago", "before", "since", "prior to"}
	futureKeywords := []string{"from now", "after", "later"}

	for _, kw := range pastKeywords {
		assert.Equal(t, DirectionPast, relativeKeywords[kw], "keyword %q should be DirectionPast", kw)
	}

	for _, kw := range futureKeywords {
		assert.Equal(t, DirectionFuture, relativeKeywords[kw], "keyword %q should be DirectionFuture", kw)
	}
}
