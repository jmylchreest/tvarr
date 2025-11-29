package expression

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFieldDefinition(t *testing.T) {
	def := &FieldDefinition{
		Name:        "channel_name",
		Type:        FieldTypeString,
		Description: "The name of the channel",
		Aliases:     []string{"name", "title"},
		Domains:     []FieldDomain{DomainStream, DomainFilter},
	}

	assert.Equal(t, "channel_name", def.Name)
	assert.Equal(t, FieldTypeString, def.Type)
	assert.Contains(t, def.Aliases, "name")
	assert.Contains(t, def.Domains, DomainStream)
}

func TestFieldRegistry_Register(t *testing.T) {
	registry := NewFieldRegistry()

	def := &FieldDefinition{
		Name:    "test_field",
		Type:    FieldTypeString,
		Domains: []FieldDomain{DomainStream},
	}

	registry.Register(def)

	result, ok := registry.Get("test_field")
	require.True(t, ok)
	assert.Equal(t, "test_field", result.Name)
}

func TestFieldRegistry_Aliases(t *testing.T) {
	registry := NewFieldRegistry()

	def := &FieldDefinition{
		Name:    "channel_name",
		Type:    FieldTypeString,
		Aliases: []string{"name", "title"},
		Domains: []FieldDomain{DomainStream},
	}

	registry.Register(def)

	// Should find by canonical name
	result, ok := registry.Get("channel_name")
	require.True(t, ok)
	assert.Equal(t, "channel_name", result.Name)

	// Should find by alias
	result, ok = registry.Get("name")
	require.True(t, ok)
	assert.Equal(t, "channel_name", result.Name)

	result, ok = registry.Get("title")
	require.True(t, ok)
	assert.Equal(t, "channel_name", result.Name)
}

func TestFieldRegistry_Resolve(t *testing.T) {
	registry := NewFieldRegistry()

	def := &FieldDefinition{
		Name:    "programme_title",
		Type:    FieldTypeString,
		Aliases: []string{"program_title", "prog_title"},
		Domains: []FieldDomain{DomainEPG},
	}

	registry.Register(def)

	// Resolve alias to canonical name
	canonical := registry.Resolve("program_title")
	assert.Equal(t, "programme_title", canonical)

	// Resolve canonical name returns itself
	canonical = registry.Resolve("programme_title")
	assert.Equal(t, "programme_title", canonical)

	// Unknown field returns itself
	canonical = registry.Resolve("unknown_field")
	assert.Equal(t, "unknown_field", canonical)
}

func TestFieldRegistry_ValidateDomain(t *testing.T) {
	registry := NewFieldRegistry()

	// Register a stream-only field
	registry.Register(&FieldDefinition{
		Name:    "stream_url",
		Type:    FieldTypeString,
		Domains: []FieldDomain{DomainStream},
	})

	// Register an EPG-only field
	registry.Register(&FieldDefinition{
		Name:    "programme_description",
		Type:    FieldTypeString,
		Domains: []FieldDomain{DomainEPG},
	})

	// Register a field valid in multiple domains
	registry.Register(&FieldDefinition{
		Name:    "source_name",
		Type:    FieldTypeString,
		Domains: []FieldDomain{DomainStream, DomainEPG, DomainFilter},
	})

	// stream_url valid in Stream domain
	assert.True(t, registry.ValidateForDomain("stream_url", DomainStream))
	// stream_url invalid in EPG domain
	assert.False(t, registry.ValidateForDomain("stream_url", DomainEPG))

	// programme_description valid in EPG domain
	assert.True(t, registry.ValidateForDomain("programme_description", DomainEPG))
	// programme_description invalid in Stream domain
	assert.False(t, registry.ValidateForDomain("programme_description", DomainStream))

	// source_name valid in multiple domains
	assert.True(t, registry.ValidateForDomain("source_name", DomainStream))
	assert.True(t, registry.ValidateForDomain("source_name", DomainEPG))
	assert.True(t, registry.ValidateForDomain("source_name", DomainFilter))

	// Unknown field is always valid (permissive)
	assert.True(t, registry.ValidateForDomain("unknown_field", DomainStream))
}

func TestFieldRegistry_ListByDomain(t *testing.T) {
	registry := NewFieldRegistry()

	registry.Register(&FieldDefinition{
		Name:    "channel_name",
		Type:    FieldTypeString,
		Domains: []FieldDomain{DomainStream},
	})

	registry.Register(&FieldDefinition{
		Name:    "stream_url",
		Type:    FieldTypeString,
		Domains: []FieldDomain{DomainStream},
	})

	registry.Register(&FieldDefinition{
		Name:    "programme_title",
		Type:    FieldTypeString,
		Domains: []FieldDomain{DomainEPG},
	})

	streamFields := registry.ListByDomain(DomainStream)
	assert.Len(t, streamFields, 2)

	epgFields := registry.ListByDomain(DomainEPG)
	assert.Len(t, epgFields, 1)
	assert.Equal(t, "programme_title", epgFields[0].Name)
}

func TestFieldTypes(t *testing.T) {
	tests := []struct {
		fieldType FieldType
		name      string
	}{
		{FieldTypeString, "string"},
		{FieldTypeInteger, "integer"},
		{FieldTypeFloat, "float"},
		{FieldTypeBoolean, "boolean"},
		{FieldTypeDatetime, "datetime"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.name, tt.fieldType.String())
		})
	}
}

func TestFieldDomains(t *testing.T) {
	tests := []struct {
		domain FieldDomain
		name   string
	}{
		{DomainStream, "stream"},
		{DomainEPG, "epg"},
		{DomainFilter, "filter"},
		{DomainRule, "rule"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.name, tt.domain.String())
		})
	}
}

func TestDefaultRegistry_ChannelFields(t *testing.T) {
	registry := DefaultRegistry()

	// Check standard channel fields exist
	channelFields := []string{
		"channel_name",
		"tvg_id",
		"tvg_name",
		"tvg_logo",
		"group_title",
		"stream_url",
		"channel_number",
	}

	for _, field := range channelFields {
		t.Run(field, func(t *testing.T) {
			def, ok := registry.Get(field)
			require.True(t, ok, "field %s should exist", field)
			assert.Contains(t, def.Domains, DomainStream)
		})
	}
}

func TestDefaultRegistry_EPGFields(t *testing.T) {
	registry := DefaultRegistry()

	// Check standard EPG fields exist
	epgFields := []string{
		"programme_title",
		"programme_description",
		"programme_start",
		"programme_stop",
		"programme_category",
	}

	for _, field := range epgFields {
		t.Run(field, func(t *testing.T) {
			def, ok := registry.Get(field)
			require.True(t, ok, "field %s should exist", field)
			assert.Contains(t, def.Domains, DomainEPG)
		})
	}
}

func TestDefaultRegistry_Aliases(t *testing.T) {
	registry := DefaultRegistry()

	tests := []struct {
		alias     string
		canonical string
	}{
		{"program_title", "programme_title"},
		{"program_description", "programme_description"},
		{"name", "channel_name"},
		{"logo", "tvg_logo"},
		{"group", "group_title"},
	}

	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			resolved := registry.Resolve(tt.alias)
			assert.Equal(t, tt.canonical, resolved)
		})
	}
}

func TestDefaultRegistry_SourceMetadata(t *testing.T) {
	registry := DefaultRegistry()

	// Source metadata fields should be available in both stream and EPG domains
	metaFields := []string{
		"source_name",
		"source_type",
		"source_url",
	}

	for _, field := range metaFields {
		t.Run(field, func(t *testing.T) {
			def, ok := registry.Get(field)
			require.True(t, ok, "field %s should exist", field)
			assert.Contains(t, def.Domains, DomainStream)
			assert.Contains(t, def.Domains, DomainEPG)
		})
	}
}

func TestFieldRegistry_NotFound(t *testing.T) {
	registry := NewFieldRegistry()

	_, ok := registry.Get("nonexistent")
	assert.False(t, ok)
}
