package expression

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelEvalContext_BasicFields(t *testing.T) {
	ctx := NewChannelEvalContext(map[string]string{
		"channel_name":   "BBC One HD",
		"tvg_id":         "bbc1.uk",
		"tvg_name":       "BBC One",
		"tvg_logo":       "http://example.com/bbc.png",
		"group_title":    "UK Entertainment",
		"stream_url":     "http://example.com/stream.m3u8",
		"channel_number": "101",
	})

	tests := []struct {
		field    string
		expected string
	}{
		{"channel_name", "BBC One HD"},
		{"tvg_id", "bbc1.uk"},
		{"tvg_name", "BBC One"},
		{"tvg_logo", "http://example.com/bbc.png"},
		{"group_title", "UK Entertainment"},
		{"stream_url", "http://example.com/stream.m3u8"},
		{"channel_number", "101"},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			value, ok := ctx.GetFieldValue(tt.field)
			require.True(t, ok)
			assert.Equal(t, tt.expected, value)
		})
	}
}

func TestChannelEvalContext_Aliases(t *testing.T) {
	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One HD",
		"tvg_logo":     "http://example.com/bbc.png",
		"group_title":  "UK Entertainment",
	})

	// Test common aliases
	tests := []struct {
		alias    string
		expected string
	}{
		{"name", "BBC One HD"},                 // alias for channel_name
		{"logo", "http://example.com/bbc.png"}, // alias for tvg_logo
		{"group", "UK Entertainment"},          // alias for group_title
	}

	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			value, ok := ctx.GetFieldValue(tt.alias)
			require.True(t, ok, "alias %s should resolve", tt.alias)
			assert.Equal(t, tt.expected, value)
		})
	}
}

func TestChannelEvalContext_SourceMetadata(t *testing.T) {
	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One",
	})

	ctx.SetSourceMetadata("My Provider", "m3u", "http://example.com/playlist.m3u")

	tests := []struct {
		field    string
		expected string
	}{
		{"source_name", "My Provider"},
		{"source_type", "m3u"},
		{"source_url", "http://example.com/playlist.m3u"},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			value, ok := ctx.GetFieldValue(tt.field)
			require.True(t, ok)
			assert.Equal(t, tt.expected, value)
		})
	}
}

func TestChannelEvalContext_MissingField(t *testing.T) {
	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One",
	})

	value, ok := ctx.GetFieldValue("nonexistent_field")
	assert.False(t, ok)
	assert.Equal(t, "", value)
}

func TestProgramEvalContext_BasicFields(t *testing.T) {
	ctx := NewProgramEvalContext(map[string]string{
		"programme_title":       "News at Ten",
		"programme_description": "The latest news from around the world",
		"programme_category":    "News",
		"programme_start":       "2024-01-15T22:00:00Z",
		"programme_stop":        "2024-01-15T22:30:00Z",
	})

	tests := []struct {
		field    string
		expected string
	}{
		{"programme_title", "News at Ten"},
		{"programme_description", "The latest news from around the world"},
		{"programme_category", "News"},
		{"programme_start", "2024-01-15T22:00:00Z"},
		{"programme_stop", "2024-01-15T22:30:00Z"},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			value, ok := ctx.GetFieldValue(tt.field)
			require.True(t, ok)
			assert.Equal(t, tt.expected, value)
		})
	}
}

func TestProgramEvalContext_Aliases(t *testing.T) {
	ctx := NewProgramEvalContext(map[string]string{
		"programme_title":       "News at Ten",
		"programme_description": "The latest news",
		"programme_category":    "News",
	})

	// Test common aliases
	tests := []struct {
		alias    string
		expected string
	}{
		{"program_title", "News at Ten"},           // US spelling alias
		{"title", "News at Ten"},                   // short alias
		{"program_description", "The latest news"}, // US spelling alias
		{"description", "The latest news"},         // short alias
		{"program_category", "News"},               // US spelling alias
		{"genre", "News"},                          // category alias
	}

	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			value, ok := ctx.GetFieldValue(tt.alias)
			require.True(t, ok, "alias %s should resolve", tt.alias)
			assert.Equal(t, tt.expected, value)
		})
	}
}

func TestProgramEvalContext_SourceMetadata(t *testing.T) {
	ctx := NewProgramEvalContext(map[string]string{
		"programme_title": "News",
	})

	ctx.SetSourceMetadata("EPG Provider", "xmltv", "http://example.com/epg.xml")

	tests := []struct {
		field    string
		expected string
	}{
		{"source_name", "EPG Provider"},
		{"source_type", "xmltv"},
		{"source_url", "http://example.com/epg.xml"},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			value, ok := ctx.GetFieldValue(tt.field)
			require.True(t, ok)
			assert.Equal(t, tt.expected, value)
		})
	}
}

func TestEvalContext_WithEvaluator(t *testing.T) {
	// Test that eval contexts work with the evaluator
	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One HD",
		"group_title":  "UK Entertainment",
	})

	evaluator := NewEvaluator()

	tests := []struct {
		name     string
		expr     string
		expected bool
	}{
		{
			name:     "simple match",
			expr:     `channel_name contains "BBC"`,
			expected: true,
		},
		{
			name:     "alias match",
			expr:     `name contains "BBC"`,
			expected: true,
		},
		{
			name:     "group alias",
			expr:     `group equals "UK Entertainment"`,
			expected: true,
		},
		{
			name:     "complex condition",
			expr:     `channel_name contains "BBC" AND group_title contains "UK"`,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := Parse(tt.expr)
			require.NoError(t, err)

			result, err := evaluator.Evaluate(parsed, ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Matches)
		})
	}
}

func TestMapEvalContext_Simple(t *testing.T) {
	// Test the simple map-based context
	ctx := NewMapEvalContext(map[string]string{
		"field1": "value1",
		"field2": "value2",
	})

	value, ok := ctx.GetFieldValue("field1")
	require.True(t, ok)
	assert.Equal(t, "value1", value)

	_, ok = ctx.GetFieldValue("nonexistent")
	assert.False(t, ok)
}

func TestEvalContext_SetField(t *testing.T) {
	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "Old Name",
	})

	// Set a new value
	ctx.SetFieldValue("channel_name", "New Name")

	value, ok := ctx.GetFieldValue("channel_name")
	require.True(t, ok)
	assert.Equal(t, "New Name", value)
}

func TestEvalContext_GetAllFields(t *testing.T) {
	ctx := NewChannelEvalContext(map[string]string{
		"channel_name": "BBC One",
		"tvg_id":       "bbc1",
	})
	ctx.SetSourceMetadata("Provider", "m3u", "http://example.com")

	fields := ctx.GetAllFields()

	assert.Contains(t, fields, "channel_name")
	assert.Contains(t, fields, "tvg_id")
	assert.Contains(t, fields, "source_name")
	assert.Equal(t, "BBC One", fields["channel_name"])
	assert.Equal(t, "Provider", fields["source_name"])
}
