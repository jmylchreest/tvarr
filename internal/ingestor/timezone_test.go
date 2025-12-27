package ingestor

import (
	"testing"
	"time"
)

func TestParseTimezoneOffset(t *testing.T) {
	tests := []struct {
		name    string
		offset  string
		wantDur time.Duration
		wantErr bool
	}{
		// Valid formats
		{
			name:    "positive HHMM format",
			offset:  "+0100",
			wantDur: 1 * time.Hour,
			wantErr: false,
		},
		{
			name:    "negative HHMM format",
			offset:  "-0500",
			wantDur: -5 * time.Hour,
			wantErr: false,
		},
		{
			name:    "positive HH:MM format with colon",
			offset:  "+01:00",
			wantDur: 1 * time.Hour,
			wantErr: false,
		},
		{
			name:    "negative HH:MM format with colon",
			offset:  "-05:00",
			wantDur: -5 * time.Hour,
			wantErr: false,
		},
		{
			name:    "positive with 30 minute offset",
			offset:  "+0530",
			wantDur: 5*time.Hour + 30*time.Minute,
			wantErr: false,
		},
		{
			name:    "negative with 30 minute offset",
			offset:  "-0530",
			wantDur: -(5*time.Hour + 30*time.Minute),
			wantErr: false,
		},
		{
			name:    "UTC Z format uppercase",
			offset:  "Z",
			wantDur: 0,
			wantErr: false,
		},
		{
			name:    "UTC z format lowercase",
			offset:  "z",
			wantDur: 0,
			wantErr: false,
		},
		{
			name:    "empty string (treated as UTC)",
			offset:  "",
			wantDur: 0,
			wantErr: false,
		},
		{
			name:    "+0000 is UTC",
			offset:  "+0000",
			wantDur: 0,
			wantErr: false,
		},
		{
			name:    "-0000 is UTC",
			offset:  "-0000",
			wantDur: 0,
			wantErr: false,
		},
		{
			name:    "+00:00 is UTC",
			offset:  "+00:00",
			wantDur: 0,
			wantErr: false,
		},
		{
			name:    "max positive offset +14:00 (Line Islands)",
			offset:  "+1400",
			wantDur: 14 * time.Hour,
			wantErr: false,
		},
		{
			name:    "typical negative US offset -0800 (PST)",
			offset:  "-0800",
			wantDur: -8 * time.Hour,
			wantErr: false,
		},
		{
			name:    "45-minute offset +0545 (Nepal)",
			offset:  "+0545",
			wantDur: 5*time.Hour + 45*time.Minute,
			wantErr: false,
		},

		// Invalid formats
		{
			name:    "invalid format - no sign",
			offset:  "0100",
			wantDur: 0,
			wantErr: true,
		},
		{
			name:    "invalid format - garbage",
			offset:  "abc",
			wantDur: 0,
			wantErr: true,
		},
		{
			name:    "invalid format - hours out of range",
			offset:  "+2500",
			wantDur: 0,
			wantErr: true,
		},
		{
			name:    "invalid format - minutes out of range",
			offset:  "+0060",
			wantDur: 0,
			wantErr: true,
		},
		{
			name:    "invalid format - incomplete",
			offset:  "+01",
			wantDur: 0,
			wantErr: true,
		},
		{
			name:    "invalid format - extra characters",
			offset:  "+0100 extra",
			wantDur: 0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDur, err := ParseTimezoneOffset(tt.offset)

			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTimezoneOffset(%q) error = %v, wantErr %v", tt.offset, err, tt.wantErr)
				return
			}

			if !tt.wantErr && gotDur != tt.wantDur {
				t.Errorf("ParseTimezoneOffset(%q) = %v, want %v", tt.offset, gotDur, tt.wantDur)
			}
		})
	}
}

func TestNormalizeProgramTime(t *testing.T) {
	// Base time: 2025-12-14 14:00:00 in a +0100 timezone
	// This represents 14:00 local time in +0100, which is 13:00 UTC
	baseTime := time.Date(2025, 12, 14, 14, 0, 0, 0, time.FixedZone("+0100", 3600))

	tests := []struct {
		name           string
		inputTime      time.Time
		detectedOffset time.Duration
		timeshiftHours int
		wantUTC        time.Time
	}{
		{
			name:           "no adjustment needed - zero offset and shift",
			inputTime:      baseTime,
			detectedOffset: 1 * time.Hour,
			timeshiftHours: 0,
			// baseTime already has timezone info, .UTC() converts correctly
			wantUTC: time.Date(2025, 12, 14, 13, 0, 0, 0, time.UTC),
		},
		{
			name:           "with positive timeshift",
			inputTime:      baseTime,
			detectedOffset: 1 * time.Hour,
			timeshiftHours: 1,
			// After UTC normalization (13:00), add 1 hour timeshift = 14:00 UTC
			wantUTC: time.Date(2025, 12, 14, 14, 0, 0, 0, time.UTC),
		},
		{
			name:           "with negative timeshift",
			inputTime:      baseTime,
			detectedOffset: 1 * time.Hour,
			timeshiftHours: -2,
			// After UTC normalization (13:00), subtract 2 hours timeshift = 11:00 UTC
			wantUTC: time.Date(2025, 12, 14, 11, 0, 0, 0, time.UTC),
		},
		{
			name:           "UTC time with no adjustment",
			inputTime:      time.Date(2025, 12, 14, 14, 0, 0, 0, time.UTC),
			detectedOffset: 0,
			timeshiftHours: 0,
			wantUTC:        time.Date(2025, 12, 14, 14, 0, 0, 0, time.UTC),
		},
		{
			name: "negative timezone offset (-0500)",
			// 14:00 in -0500 = 19:00 UTC
			inputTime:      time.Date(2025, 12, 14, 14, 0, 0, 0, time.FixedZone("-0500", -5*3600)),
			detectedOffset: -5 * time.Hour,
			timeshiftHours: 0,
			wantUTC:        time.Date(2025, 12, 14, 19, 0, 0, 0, time.UTC),
		},
		{
			name: "crossing midnight with normalization",
			// 23:00 in +0100 = 22:00 UTC
			inputTime:      time.Date(2025, 12, 14, 23, 0, 0, 0, time.FixedZone("+0100", 3600)),
			detectedOffset: 1 * time.Hour,
			timeshiftHours: 3,
			// After UTC normalization (22:00), add 3 hours = 01:00 UTC next day
			wantUTC: time.Date(2025, 12, 15, 1, 0, 0, 0, time.UTC),
		},
		{
			name: "30-minute timezone offset",
			// 14:30 in +0530 = 09:00 UTC
			inputTime:      time.Date(2025, 12, 14, 14, 30, 0, 0, time.FixedZone("+0530", 5*3600+30*60)),
			detectedOffset: 5*time.Hour + 30*time.Minute,
			timeshiftHours: 0,
			wantUTC:        time.Date(2025, 12, 14, 9, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeProgramTime(tt.inputTime, tt.detectedOffset, tt.timeshiftHours)

			if !got.Equal(tt.wantUTC) {
				t.Errorf("NormalizeProgramTime() = %v, want %v", got, tt.wantUTC)
			}

			// Verify the result is in UTC
			if got.Location().String() != "UTC" {
				t.Errorf("NormalizeProgramTime() location = %v, want UTC", got.Location())
			}
		})
	}
}

func TestFormatTimezoneOffset(t *testing.T) {
	tests := []struct {
		name   string
		offset string
		want   string
	}{
		{
			name:   "HHMM to HH:MM positive",
			offset: "+0100",
			want:   "+01:00",
		},
		{
			name:   "HHMM to HH:MM negative",
			offset: "-0500",
			want:   "-05:00",
		},
		{
			name:   "already in HH:MM format",
			offset: "+01:00",
			want:   "+01:00",
		},
		{
			name:   "Z to +00:00",
			offset: "Z",
			want:   "+00:00",
		},
		{
			name:   "empty to +00:00",
			offset: "",
			want:   "+00:00",
		},
		{
			name:   "z lowercase to +00:00",
			offset: "z",
			want:   "+00:00",
		},
		{
			name:   "passthrough for unknown format",
			offset: "unknown",
			want:   "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTimezoneOffset(tt.offset)
			if got != tt.want {
				t.Errorf("FormatTimezoneOffset(%q) = %q, want %q", tt.offset, got, tt.want)
			}
		})
	}
}
