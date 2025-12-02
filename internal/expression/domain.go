package expression

// ExpressionDomain represents the logical domain in which an expression is evaluated.
// This allows for context-specific field validation and field set selection.
type ExpressionDomain string

const (
	// DomainStreamFilter is for stream filtering expressions.
	DomainStreamFilter ExpressionDomain = "stream_filter"

	// DomainEPGFilter is for EPG filtering expressions.
	DomainEPGFilter ExpressionDomain = "epg_filter"

	// DomainStreamMapping is for stream data mapping expressions.
	DomainStreamMapping ExpressionDomain = "stream_mapping"

	// DomainEPGMapping is for EPG data mapping expressions.
	DomainEPGMapping ExpressionDomain = "epg_mapping"
)

// ParseExpressionDomain parses a domain string into an ExpressionDomain.
// Returns the domain and true if valid, or an empty domain and false if invalid.
func ParseExpressionDomain(s string) (ExpressionDomain, bool) {
	switch s {
	case "stream_filter", "stream":
		return DomainStreamFilter, true
	case "epg_filter", "epg":
		return DomainEPGFilter, true
	case "stream_mapping", "stream_data_mapping", "stream_datamapping":
		return DomainStreamMapping, true
	case "epg_mapping", "epg_data_mapping", "epg_datamapping":
		return DomainEPGMapping, true
	default:
		return "", false
	}
}

// IsFilterDomain returns true if the domain is a filtering domain.
func (d ExpressionDomain) IsFilterDomain() bool {
	return d == DomainStreamFilter || d == DomainEPGFilter
}

// IsMappingDomain returns true if the domain is a data mapping domain.
func (d ExpressionDomain) IsMappingDomain() bool {
	return d == DomainStreamMapping || d == DomainEPGMapping
}

// IsStreamDomain returns true if the domain is for stream data.
func (d ExpressionDomain) IsStreamDomain() bool {
	return d == DomainStreamFilter || d == DomainStreamMapping
}

// IsEPGDomain returns true if the domain is for EPG data.
func (d ExpressionDomain) IsEPGDomain() bool {
	return d == DomainEPGFilter || d == DomainEPGMapping
}

// String returns the string representation of the domain.
func (d ExpressionDomain) String() string {
	return string(d)
}
