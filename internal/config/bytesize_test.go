package config

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseByteSize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected ByteSize
		wantErr  bool
	}{
		{"bytes", "1024", 1024, false},
		{"kilobytes", "5KB", 5 * 1024, false},
		{"megabytes", "10MB", 10 * 1024 * 1024, false},
		{"gigabytes", "2GB", 2 * 1024 * 1024 * 1024, false},
		{"with space", "5 MB", 5 * 1024 * 1024, false},
		{"lowercase", "5mb", 5 * 1024 * 1024, false},
		{"float", "1.5MB", ByteSize(1.5 * 1024 * 1024), false},
		{"zero", "0", 0, false},
		{"invalid", "invalid", 0, true},
		{"empty", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size, err := ParseByteSize(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, size)
		})
	}
}

func TestByteSize_UnmarshalText(t *testing.T) {
	var b ByteSize
	err := b.UnmarshalText([]byte("5MB"))
	require.NoError(t, err)
	assert.Equal(t, ByteSize(5*1024*1024), b)
}

func TestByteSize_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected ByteSize
	}{
		{"string format", `"5MB"`, 5 * 1024 * 1024},
		{"string with space", `"5 MB"`, 5 * 1024 * 1024},
		{"bytes int", `5242880`, 5242880},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b ByteSize
			err := json.Unmarshal([]byte(tt.json), &b)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, b)
		})
	}
}

func TestByteSize_MarshalJSON(t *testing.T) {
	b := ByteSize(5 * 1024 * 1024)
	data, err := json.Marshal(b)
	require.NoError(t, err)
	assert.Equal(t, `"5MB"`, string(data))
}

func TestByteSize_String(t *testing.T) {
	tests := []struct {
		name     string
		size     ByteSize
		expected string
	}{
		{"bytes", 500, "500B"},
		{"kilobytes", 5 * 1024, "5KB"},
		{"megabytes", 10 * 1024 * 1024, "10MB"},
		{"gigabytes", 2 * 1024 * 1024 * 1024, "2GB"},
		{"zero", 0, "0B"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.size.String())
		})
	}
}

func TestByteSize_Bytes(t *testing.T) {
	b := ByteSize(5 * 1024 * 1024)
	assert.Equal(t, int64(5242880), b.Bytes())
}
