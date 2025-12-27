package expression

import "sync"

// FieldType represents the data type of a field.
type FieldType string

// Field types.
const (
	FieldTypeString   FieldType = "string"
	FieldTypeInteger  FieldType = "integer"
	FieldTypeFloat    FieldType = "float"
	FieldTypeBoolean  FieldType = "boolean"
	FieldTypeDatetime FieldType = "datetime"
)

// String returns the string representation of the field type.
func (t FieldType) String() string {
	return string(t)
}

// FieldDomain represents a context where a field can be used.
type FieldDomain string

// Field domains.
const (
	DomainStream  FieldDomain = "stream"  // Channel/stream fields
	DomainEPG     FieldDomain = "epg"     // EPG/programme fields
	DomainFilter  FieldDomain = "filter"  // Filter rule fields
	DomainRule    FieldDomain = "rule"    // Data mapping rule fields
	DomainRequest FieldDomain = "request" // HTTP request context fields (for client detection)
)

// String returns the string representation of the field domain.
func (d FieldDomain) String() string {
	return string(d)
}

// FieldDefinition describes a field that can be used in expressions.
type FieldDefinition struct {
	// Name is the canonical name of the field.
	Name string

	// Type is the data type of the field.
	Type FieldType

	// Description provides documentation for the field.
	Description string

	// Aliases are alternative names for this field.
	Aliases []string

	// Domains lists where this field can be used.
	Domains []FieldDomain

	// ReadOnly indicates if the field cannot be modified by actions.
	ReadOnly bool
}

// FieldRegistry maintains a registry of field definitions.
type FieldRegistry struct {
	mu       sync.RWMutex
	fields   map[string]*FieldDefinition
	aliases  map[string]string // alias -> canonical name
	byDomain map[FieldDomain][]*FieldDefinition
}

// NewFieldRegistry creates a new empty field registry.
func NewFieldRegistry() *FieldRegistry {
	return &FieldRegistry{
		fields:   make(map[string]*FieldDefinition),
		aliases:  make(map[string]string),
		byDomain: make(map[FieldDomain][]*FieldDefinition),
	}
}

// Register adds a field definition to the registry.
func (r *FieldRegistry) Register(def *FieldDefinition) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Register the canonical name
	r.fields[def.Name] = def

	// Register all aliases
	for _, alias := range def.Aliases {
		r.aliases[alias] = def.Name
	}

	// Index by domain
	for _, domain := range def.Domains {
		r.byDomain[domain] = append(r.byDomain[domain], def)
	}
}

// Get retrieves a field definition by name or alias.
func (r *FieldRegistry) Get(name string) (*FieldDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try canonical name first
	if def, ok := r.fields[name]; ok {
		return def, true
	}

	// Try alias
	if canonical, ok := r.aliases[name]; ok {
		if def, ok := r.fields[canonical]; ok {
			return def, true
		}
	}

	return nil, false
}

// Resolve returns the canonical name for a field name or alias.
// If the name is not found, it returns the input unchanged.
func (r *FieldRegistry) Resolve(name string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check if it's already a canonical name
	if _, ok := r.fields[name]; ok {
		return name
	}

	// Check if it's an alias
	if canonical, ok := r.aliases[name]; ok {
		return canonical
	}

	// Unknown field - return as-is
	return name
}

// ValidateForDomain checks if a field can be used in the given domain.
// Unknown fields are considered valid (permissive mode).
func (r *FieldRegistry) ValidateForDomain(name string, domain FieldDomain) bool {
	def, ok := r.Get(name)
	if !ok {
		// Unknown fields are allowed (permissive)
		return true
	}

	for _, d := range def.Domains {
		if d == domain {
			return true
		}
	}

	return false
}

// ListByDomain returns all field definitions valid for the given domain.
func (r *FieldRegistry) ListByDomain(domain FieldDomain) []*FieldDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.byDomain[domain]
}

// All returns all registered field definitions.
func (r *FieldRegistry) All() []*FieldDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*FieldDefinition, 0, len(r.fields))
	for _, def := range r.fields {
		result = append(result, def)
	}
	return result
}

// defaultRegistry is the singleton default registry.
var (
	defaultRegistry     *FieldRegistry
	defaultRegistryOnce sync.Once
)

// DefaultRegistry returns the default field registry with standard fields.
func DefaultRegistry() *FieldRegistry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry = NewFieldRegistry()
		registerChannelFields(defaultRegistry)
		registerEPGFields(defaultRegistry)
		registerSourceMetadataFields(defaultRegistry)
		registerRequestContextFields(defaultRegistry)
	})
	return defaultRegistry
}

// registerChannelFields registers standard channel/stream fields.
func registerChannelFields(r *FieldRegistry) {
	// Core channel identification
	r.Register(&FieldDefinition{
		Name:        "channel_name",
		Type:        FieldTypeString,
		Description: "The display name of the channel",
		Aliases:     []string{"name"},
		Domains:     []FieldDomain{DomainStream, DomainFilter, DomainRule},
	})

	r.Register(&FieldDefinition{
		Name:        "tvg_id",
		Type:        FieldTypeString,
		Description: "The EPG identifier for the channel",
		Aliases:     []string{"epg_id"},
		Domains:     []FieldDomain{DomainStream, DomainFilter, DomainRule},
	})

	r.Register(&FieldDefinition{
		Name:        "tvg_name",
		Type:        FieldTypeString,
		Description: "The TVG name attribute",
		Domains:     []FieldDomain{DomainStream, DomainFilter, DomainRule},
	})

	r.Register(&FieldDefinition{
		Name:        "tvg_logo",
		Type:        FieldTypeString,
		Description: "URL to the channel logo",
		Aliases:     []string{"logo"},
		Domains:     []FieldDomain{DomainStream, DomainFilter, DomainRule},
	})

	r.Register(&FieldDefinition{
		Name:        "group_title",
		Type:        FieldTypeString,
		Description: "The group/category for the channel",
		Aliases:     []string{"group", "category"},
		Domains:     []FieldDomain{DomainStream, DomainFilter, DomainRule},
	})

	r.Register(&FieldDefinition{
		Name:        "stream_url",
		Type:        FieldTypeString,
		Description: "The URL of the stream",
		Aliases:     []string{"url"},
		Domains:     []FieldDomain{DomainStream, DomainFilter, DomainRule},
		ReadOnly:    true,
	})

	r.Register(&FieldDefinition{
		Name:        "channel_number",
		Type:        FieldTypeInteger,
		Description: "The assigned channel number",
		Aliases:     []string{"number", "chno"},
		Domains:     []FieldDomain{DomainStream, DomainFilter, DomainRule},
	})

	// Additional M3U attributes
	r.Register(&FieldDefinition{
		Name:        "tvg_shift",
		Type:        FieldTypeFloat,
		Description: "EPG time shift in hours",
		Domains:     []FieldDomain{DomainStream, DomainRule},
	})

	r.Register(&FieldDefinition{
		Name:        "tvg_language",
		Type:        FieldTypeString,
		Description: "Language of the channel",
		Aliases:     []string{"language", "lang"},
		Domains:     []FieldDomain{DomainStream, DomainFilter, DomainRule},
	})

	r.Register(&FieldDefinition{
		Name:        "tvg_country",
		Type:        FieldTypeString,
		Description: "Country of the channel",
		Aliases:     []string{"country"},
		Domains:     []FieldDomain{DomainStream, DomainFilter, DomainRule},
	})

	r.Register(&FieldDefinition{
		Name:        "radio",
		Type:        FieldTypeBoolean,
		Description: "Whether the stream is a radio station",
		Domains:     []FieldDomain{DomainStream, DomainFilter},
	})

	r.Register(&FieldDefinition{
		Name:        "is_adult",
		Type:        FieldTypeBoolean,
		Description: "Whether the stream contains adult content",
		Aliases:     []string{"adult"},
		Domains:     []FieldDomain{DomainStream, DomainFilter},
	})
}

// registerEPGFields registers standard EPG/programme fields.
func registerEPGFields(r *FieldRegistry) {
	r.Register(&FieldDefinition{
		Name:        "programme_title",
		Type:        FieldTypeString,
		Description: "The title of the programme",
		Aliases:     []string{"program_title", "title"},
		Domains:     []FieldDomain{DomainEPG, DomainFilter, DomainRule},
	})

	r.Register(&FieldDefinition{
		Name:        "programme_description",
		Type:        FieldTypeString,
		Description: "The description of the programme",
		Aliases:     []string{"program_description", "description", "desc"},
		Domains:     []FieldDomain{DomainEPG, DomainFilter, DomainRule},
	})

	r.Register(&FieldDefinition{
		Name:        "programme_start",
		Type:        FieldTypeDatetime,
		Description: "The start time of the programme",
		Aliases:     []string{"program_start", "start", "start_time"},
		Domains:     []FieldDomain{DomainEPG, DomainFilter},
		ReadOnly:    true,
	})

	r.Register(&FieldDefinition{
		Name:        "programme_stop",
		Type:        FieldTypeDatetime,
		Description: "The end time of the programme",
		Aliases:     []string{"program_stop", "stop", "end_time"},
		Domains:     []FieldDomain{DomainEPG, DomainFilter},
		ReadOnly:    true,
	})

	r.Register(&FieldDefinition{
		Name:        "programme_category",
		Type:        FieldTypeString,
		Description: "The category of the programme",
		Aliases:     []string{"program_category", "genre"},
		Domains:     []FieldDomain{DomainEPG, DomainFilter, DomainRule},
	})

	r.Register(&FieldDefinition{
		Name:        "programme_episode",
		Type:        FieldTypeString,
		Description: "Episode number information",
		Aliases:     []string{"program_episode", "episode"},
		Domains:     []FieldDomain{DomainEPG, DomainFilter},
	})

	r.Register(&FieldDefinition{
		Name:        "programme_season",
		Type:        FieldTypeString,
		Description: "Season number information",
		Aliases:     []string{"program_season", "season"},
		Domains:     []FieldDomain{DomainEPG, DomainFilter},
	})

	r.Register(&FieldDefinition{
		Name:        "programme_icon",
		Type:        FieldTypeString,
		Description: "URL to the programme icon/poster",
		Aliases:     []string{"program_icon", "poster"},
		Domains:     []FieldDomain{DomainEPG, DomainRule},
	})
}

// registerSourceMetadataFields registers source metadata fields.
func registerSourceMetadataFields(r *FieldRegistry) {
	r.Register(&FieldDefinition{
		Name:        "source_name",
		Type:        FieldTypeString,
		Description: "The name of the source that provided this data",
		Domains:     []FieldDomain{DomainStream, DomainEPG, DomainFilter},
		ReadOnly:    true,
	})

	r.Register(&FieldDefinition{
		Name:        "source_type",
		Type:        FieldTypeString,
		Description: "The type of source (m3u, xtream, xmltv)",
		Domains:     []FieldDomain{DomainStream, DomainEPG, DomainFilter},
		ReadOnly:    true,
	})

	r.Register(&FieldDefinition{
		Name:        "source_url",
		Type:        FieldTypeString,
		Description: "The URL of the source",
		Domains:     []FieldDomain{DomainStream, DomainEPG, DomainFilter},
		ReadOnly:    true,
	})
}

// registerRequestContextFields registers HTTP request context fields for client detection.
func registerRequestContextFields(r *FieldRegistry) {
	r.Register(&FieldDefinition{
		Name:        "user_agent",
		Type:        FieldTypeString,
		Description: "The User-Agent header from the HTTP request",
		Aliases:     []string{"ua"},
		Domains:     []FieldDomain{DomainRequest},
		ReadOnly:    true,
	})

	r.Register(&FieldDefinition{
		Name:        "client_ip",
		Type:        FieldTypeString,
		Description: "The client IP address (considers X-Forwarded-For)",
		Aliases:     []string{"ip", "remote_addr"},
		Domains:     []FieldDomain{DomainRequest},
		ReadOnly:    true,
	})

	r.Register(&FieldDefinition{
		Name:        "request_path",
		Type:        FieldTypeString,
		Description: "The URL path of the request",
		Aliases:     []string{"path"},
		Domains:     []FieldDomain{DomainRequest},
		ReadOnly:    true,
	})

	r.Register(&FieldDefinition{
		Name:        "request_url",
		Type:        FieldTypeString,
		Description: "The full URL of the request",
		Aliases:     []string{"url"},
		Domains:     []FieldDomain{DomainRequest},
		ReadOnly:    true,
	})

	r.Register(&FieldDefinition{
		Name:        "query_params",
		Type:        FieldTypeString,
		Description: "The query string of the request",
		Aliases:     []string{"query"},
		Domains:     []FieldDomain{DomainRequest},
		ReadOnly:    true,
	})

	r.Register(&FieldDefinition{
		Name:        "x_forwarded_for",
		Type:        FieldTypeString,
		Description: "The X-Forwarded-For header value",
		Domains:     []FieldDomain{DomainRequest},
		ReadOnly:    true,
	})

	r.Register(&FieldDefinition{
		Name:        "x_real_ip",
		Type:        FieldTypeString,
		Description: "The X-Real-IP header value",
		Domains:     []FieldDomain{DomainRequest},
		ReadOnly:    true,
	})

	r.Register(&FieldDefinition{
		Name:        "accept",
		Type:        FieldTypeString,
		Description: "The Accept header value",
		Domains:     []FieldDomain{DomainRequest},
		ReadOnly:    true,
	})

	r.Register(&FieldDefinition{
		Name:        "accept_language",
		Type:        FieldTypeString,
		Description: "The Accept-Language header value",
		Domains:     []FieldDomain{DomainRequest},
		ReadOnly:    true,
	})

	r.Register(&FieldDefinition{
		Name:        "host",
		Type:        FieldTypeString,
		Description: "The Host header value",
		Domains:     []FieldDomain{DomainRequest},
		ReadOnly:    true,
	})

	r.Register(&FieldDefinition{
		Name:        "referer",
		Type:        FieldTypeString,
		Description: "The Referer header value",
		Aliases:     []string{"referrer"},
		Domains:     []FieldDomain{DomainRequest},
		ReadOnly:    true,
	})

	// Dynamic header input fields (extracted via @dynamic())
	r.Register(&FieldDefinition{
		Name:        "x_video_codec",
		Type:        FieldTypeString,
		Description: "The X-Video-Codec header value (via @dynamic())",
		Domains:     []FieldDomain{DomainRequest},
		ReadOnly:    true,
	})

	r.Register(&FieldDefinition{
		Name:        "x_audio_codec",
		Type:        FieldTypeString,
		Description: "The X-Audio-Codec header value (via @dynamic())",
		Domains:     []FieldDomain{DomainRequest},
		ReadOnly:    true,
	})

	r.Register(&FieldDefinition{
		Name:        "x_container",
		Type:        FieldTypeString,
		Description: "The X-Container header value (via @dynamic())",
		Domains:     []FieldDomain{DomainRequest},
		ReadOnly:    true,
	})

	// Client capabilities output fields (settable via SET clause)
	r.Register(&FieldDefinition{
		Name:        "accepted_video_codecs",
		Type:        FieldTypeString,
		Description: "List of video codecs the client accepts (JSON array)",
		Domains:     []FieldDomain{DomainRequest},
		ReadOnly:    false,
	})

	r.Register(&FieldDefinition{
		Name:        "accepted_audio_codecs",
		Type:        FieldTypeString,
		Description: "List of audio codecs the client accepts (JSON array)",
		Domains:     []FieldDomain{DomainRequest},
		ReadOnly:    false,
	})

	r.Register(&FieldDefinition{
		Name:        "preferred_video_codec",
		Type:        FieldTypeString,
		Description: "Client's preferred video codec",
		Domains:     []FieldDomain{DomainRequest},
		ReadOnly:    false,
	})

	r.Register(&FieldDefinition{
		Name:        "preferred_audio_codec",
		Type:        FieldTypeString,
		Description: "Client's preferred audio codec",
		Domains:     []FieldDomain{DomainRequest},
		ReadOnly:    false,
	})

	r.Register(&FieldDefinition{
		Name:        "preferred_format",
		Type:        FieldTypeString,
		Description: "Client's preferred container format (hls, hls-fmp4, dash, mpegts)",
		Domains:     []FieldDomain{DomainRequest},
		ReadOnly:    false,
	})

	r.Register(&FieldDefinition{
		Name:        "supports_fmp4",
		Type:        FieldTypeBoolean,
		Description: "Whether the client supports fMP4 segments",
		Domains:     []FieldDomain{DomainRequest},
		ReadOnly:    false,
	})

	r.Register(&FieldDefinition{
		Name:        "supports_mpegts",
		Type:        FieldTypeBoolean,
		Description: "Whether the client supports MPEG-TS",
		Domains:     []FieldDomain{DomainRequest},
		ReadOnly:    false,
	})
}
