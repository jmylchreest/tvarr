package expression

// SourceMetadata contains metadata about the source of a record.
type SourceMetadata struct {
	Name string
	Type string
	URL  string
}

// BaseEvalContext provides common functionality for eval contexts.
type BaseEvalContext struct {
	fields   map[string]string
	source   SourceMetadata
	registry *FieldRegistry
}

// newBaseEvalContext creates a new base eval context.
func newBaseEvalContext(fields map[string]string, registry *FieldRegistry) *BaseEvalContext {
	if fields == nil {
		fields = make(map[string]string)
	}
	if registry == nil {
		registry = DefaultRegistry()
	}
	return &BaseEvalContext{
		fields:   fields,
		registry: registry,
	}
}

// GetFieldValue returns the value of a field, resolving aliases.
func (c *BaseEvalContext) GetFieldValue(name string) (string, bool) {
	// Resolve alias to canonical name
	canonical := c.registry.Resolve(name)

	// Check source metadata fields
	switch canonical {
	case "source_name":
		return c.source.Name, c.source.Name != ""
	case "source_type":
		return c.source.Type, c.source.Type != ""
	case "source_url":
		return c.source.URL, c.source.URL != ""
	}

	// Look up in fields map
	value, ok := c.fields[canonical]
	return value, ok
}

// SetFieldValue sets the value of a field.
func (c *BaseEvalContext) SetFieldValue(name, value string) {
	canonical := c.registry.Resolve(name)
	c.fields[canonical] = value
}

// SetSourceMetadata sets the source metadata fields.
func (c *BaseEvalContext) SetSourceMetadata(name, sourceType, url string) {
	c.source = SourceMetadata{
		Name: name,
		Type: sourceType,
		URL:  url,
	}
}

// GetAllFields returns a copy of all field values including source metadata.
func (c *BaseEvalContext) GetAllFields() map[string]string {
	result := make(map[string]string, len(c.fields)+3)
	for k, v := range c.fields {
		result[k] = v
	}
	if c.source.Name != "" {
		result["source_name"] = c.source.Name
	}
	if c.source.Type != "" {
		result["source_type"] = c.source.Type
	}
	if c.source.URL != "" {
		result["source_url"] = c.source.URL
	}
	return result
}

// ChannelEvalContext provides field access for channel records.
type ChannelEvalContext struct {
	*BaseEvalContext
}

// NewChannelEvalContext creates a new channel eval context.
func NewChannelEvalContext(fields map[string]string) *ChannelEvalContext {
	return &ChannelEvalContext{
		BaseEvalContext: newBaseEvalContext(fields, DefaultRegistry()),
	}
}

// ProgramEvalContext provides field access for EPG program records.
type ProgramEvalContext struct {
	*BaseEvalContext
}

// NewProgramEvalContext creates a new program eval context.
func NewProgramEvalContext(fields map[string]string) *ProgramEvalContext {
	return &ProgramEvalContext{
		BaseEvalContext: newBaseEvalContext(fields, DefaultRegistry()),
	}
}

// MapEvalContext is a simple map-based eval context without alias resolution.
type MapEvalContext struct {
	fields map[string]string
}

// NewMapEvalContext creates a new simple map-based eval context.
func NewMapEvalContext(fields map[string]string) *MapEvalContext {
	if fields == nil {
		fields = make(map[string]string)
	}
	return &MapEvalContext{fields: fields}
}

// GetFieldValue returns the value of a field.
func (c *MapEvalContext) GetFieldValue(name string) (string, bool) {
	value, ok := c.fields[name]
	return value, ok
}

// SetFieldValue sets the value of a field.
func (c *MapEvalContext) SetFieldValue(name, value string) {
	c.fields[name] = value
}

// GetAllFields returns a copy of all field values.
func (c *MapEvalContext) GetAllFields() map[string]string {
	result := make(map[string]string, len(c.fields))
	for k, v := range c.fields {
		result[k] = v
	}
	return result
}
