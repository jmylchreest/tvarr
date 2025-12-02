package xtream

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFlexInt_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{"integer", `123`, 123},
		{"string number", `"456"`, 456},
		{"empty string", `""`, 0},
		{"zero", `0`, 0},
		{"negative", `-100`, -100},
		{"string negative", `"-200"`, -200},
		{"large number", `1704067200`, 1704067200},
		{"string large", `"1704067200"`, 1704067200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f FlexInt
			err := json.Unmarshal([]byte(tt.input), &f)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if f.Int() != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, f.Int())
			}
		})
	}
}

func TestFlexFloat_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected float64
	}{
		{"float", `3.14`, 3.14},
		{"integer", `42`, 42.0},
		{"string float", `"2.71"`, 2.71},
		{"string integer", `"100"`, 100.0},
		{"empty string", `""`, 0},
		{"zero", `0`, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f FlexFloat
			err := json.Unmarshal([]byte(tt.input), &f)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if f.Float() != tt.expected {
				t.Errorf("expected %f, got %f", tt.expected, f.Float())
			}
		})
	}
}

func TestFlexString_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"string", `"hello"`, "hello"},
		{"number", `123`, "123"},
		{"float number", `3.14`, "3.14"},
		{"empty string", `""`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f FlexString
			err := json.Unmarshal([]byte(tt.input), &f)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if f.String() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, f.String())
			}
		})
	}
}

func TestUserInfo_IsAuthenticated(t *testing.T) {
	tests := []struct {
		name     string
		user     UserInfo
		expected bool
	}{
		{
			name:     "active and auth=1",
			user:     UserInfo{Auth: 1, Status: "Active"},
			expected: true,
		},
		{
			name:     "auth=0",
			user:     UserInfo{Auth: 0, Status: "Active"},
			expected: false,
		},
		{
			name:     "inactive",
			user:     UserInfo{Auth: 1, Status: "Expired"},
			expected: false,
		},
		{
			name:     "banned",
			user:     UserInfo{Auth: 1, Status: "Banned"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.user.IsAuthenticated()
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestUserInfo_ExpirationTime(t *testing.T) {
	now := time.Now()
	expTime := now.Add(30 * 24 * time.Hour)

	user := UserInfo{ExpDate: FlexInt(expTime.Unix())}
	result := user.ExpirationTime()

	if result.Unix() != expTime.Unix() {
		t.Errorf("expected %v, got %v", expTime, result)
	}
}

func TestUserInfo_ExpirationTime_Zero(t *testing.T) {
	user := UserInfo{ExpDate: 0}
	result := user.ExpirationTime()

	if !result.IsZero() {
		t.Errorf("expected zero time, got %v", result)
	}
}

func TestUserInfo_IsExpired(t *testing.T) {
	tests := []struct {
		name     string
		expDate  int64
		expected bool
	}{
		{
			name:     "not expired",
			expDate:  time.Now().Add(24 * time.Hour).Unix(),
			expected: false,
		},
		{
			name:     "expired",
			expDate:  time.Now().Add(-24 * time.Hour).Unix(),
			expected: true,
		},
		{
			name:     "no expiration",
			expDate:  0,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := UserInfo{ExpDate: FlexInt(tt.expDate)}
			result := user.IsExpired()
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestEPGListing_StartTime(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	tests := []struct {
		name     string
		listing  EPGListing
		expected time.Time
	}{
		{
			name:     "from timestamp",
			listing:  EPGListing{StartTimestamp: FlexInt(now.Unix())},
			expected: now,
		},
		{
			name:     "from string",
			listing:  EPGListing{Start: "2024-01-15 10:30:00"},
			expected: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			name:     "timestamp takes precedence",
			listing:  EPGListing{StartTimestamp: FlexInt(now.Unix()), Start: "2024-01-15 10:30:00"},
			expected: now,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.listing.StartTime()
			if result.Unix() != tt.expected.Unix() {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestStream_AddedTime(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	stream := Stream{Added: FlexInt(now.Unix())}

	result := stream.AddedTime()
	if result.Unix() != now.Unix() {
		t.Errorf("expected %v, got %v", now, result)
	}
}

func TestStream_AddedTime_Zero(t *testing.T) {
	stream := Stream{Added: 0}
	result := stream.AddedTime()

	if !result.IsZero() {
		t.Errorf("expected zero time, got %v", result)
	}
}

func TestCategory_JSON(t *testing.T) {
	// Test that real Xtream API responses can be parsed
	input := `{
		"category_id": "1",
		"category_name": "USA | News",
		"parent_id": 0
	}`

	var cat Category
	err := json.Unmarshal([]byte(input), &cat)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cat.CategoryID.String() != "1" {
		t.Errorf("expected category_id '1', got %q", cat.CategoryID.String())
	}
	if cat.CategoryName != "USA | News" {
		t.Errorf("expected category_name 'USA | News', got %q", cat.CategoryName)
	}
}

func TestStream_JSON(t *testing.T) {
	// Test parsing real Xtream API stream response
	input := `{
		"num": 1,
		"name": "CNN HD",
		"stream_type": "live",
		"stream_id": 12345,
		"stream_icon": "http://example.com/cnn.png",
		"epg_channel_id": "CNN.us",
		"added": "1704067200",
		"is_adult": "0",
		"category_id": "1",
		"tv_archive": 1,
		"tv_archive_duration": 7
	}`

	var stream Stream
	err := json.Unmarshal([]byte(input), &stream)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if stream.StreamID.Int() != 12345 {
		t.Errorf("expected stream_id 12345, got %d", stream.StreamID.Int())
	}
	if stream.Name != "CNN HD" {
		t.Errorf("expected name 'CNN HD', got %q", stream.Name)
	}
	if stream.EPGChannelID != "CNN.us" {
		t.Errorf("expected epg_channel_id 'CNN.us', got %q", stream.EPGChannelID)
	}
	if stream.Added.Int() != 1704067200 {
		t.Errorf("expected added 1704067200, got %d", stream.Added.Int())
	}
	if stream.IsAdult.Int() != 0 {
		t.Errorf("expected is_adult 0, got %d", stream.IsAdult.Int())
	}
	if stream.TVArchive.Int() != 1 {
		t.Errorf("expected tv_archive 1, got %d", stream.TVArchive.Int())
	}
}

func TestAuthInfo_JSON(t *testing.T) {
	// Test parsing real Xtream API auth response
	input := `{
		"user_info": {
			"username": "testuser",
			"password": "testpass",
			"message": "Welcome",
			"auth": 1,
			"status": "Active",
			"exp_date": "1735689600",
			"is_trial": "0",
			"active_cons": "0",
			"created_at": "1704067200",
			"max_connections": "1",
			"allowed_output_formats": ["m3u8", "ts"]
		},
		"server_info": {
			"url": "example.com",
			"port": "8080",
			"https_port": "8443",
			"server_protocol": "http",
			"rtmp_port": "1935",
			"timezone": "America/New_York",
			"timestamp_now": 1704153600,
			"time_now": "2024-01-02 00:00:00"
		}
	}`

	var info AuthInfo
	err := json.Unmarshal([]byte(input), &info)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.UserInfo.Username != "testuser" {
		t.Errorf("expected username 'testuser', got %q", info.UserInfo.Username)
	}
	if !info.UserInfo.IsAuthenticated() {
		t.Error("expected user to be authenticated")
	}
	if info.ServerInfo.Port.Int() != 8080 {
		t.Errorf("expected port 8080, got %d", info.ServerInfo.Port.Int())
	}
	if len(info.UserInfo.AllowedOutputFormats) != 2 {
		t.Errorf("expected 2 output formats, got %d", len(info.UserInfo.AllowedOutputFormats))
	}
}
