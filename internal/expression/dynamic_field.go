package expression

import (
	"net/http"
	"strings"
)

// DynamicFieldPrefix is the prefix for dynamic field resolvers.
// Dynamic fields use the syntax @prefix:<parameter> in expressions.
const DynamicFieldPrefix = "@"

// DynamicFieldResolver resolves parameterized field references at evaluation time.
// This enables expressions like @header_req:X-Custom-Player to extract arbitrary
// header values without hardcoding specific headers in the field registry.
type DynamicFieldResolver interface {
	// Prefix returns the field prefix this resolver handles (e.g., "header_req").
	// The full field syntax is @prefix:<parameter>, e.g., @header_req:X-Custom-Player.
	Prefix() string

	// Resolve extracts the value for the given parameter.
	// Returns the value and true if found, or empty string and false if not available.
	Resolve(parameter string) (string, bool)
}

// DynamicFieldRegistry manages dynamic field resolvers.
type DynamicFieldRegistry struct {
	resolvers map[string]DynamicFieldResolver
}

// NewDynamicFieldRegistry creates a new dynamic field registry.
func NewDynamicFieldRegistry() *DynamicFieldRegistry {
	return &DynamicFieldRegistry{
		resolvers: make(map[string]DynamicFieldResolver),
	}
}

// Register adds a resolver to the registry.
func (r *DynamicFieldRegistry) Register(resolver DynamicFieldResolver) {
	r.resolvers[strings.ToLower(resolver.Prefix())] = resolver
}

// Resolve attempts to resolve a dynamic field reference.
// Field format: @prefix:parameter (e.g., @header_req:X-Custom-Player)
// Returns the value and true if resolved, or empty string and false if not.
func (r *DynamicFieldRegistry) Resolve(fieldName string) (string, bool) {
	// Check if it's a dynamic field reference
	if !strings.HasPrefix(fieldName, DynamicFieldPrefix) {
		return "", false
	}

	// Remove the @ prefix
	remainder := fieldName[len(DynamicFieldPrefix):]

	// Split into prefix:parameter
	colonIdx := strings.Index(remainder, ":")
	if colonIdx == -1 {
		return "", false
	}

	prefix := strings.ToLower(remainder[:colonIdx])
	parameter := remainder[colonIdx+1:]

	// Find the resolver for this prefix
	resolver, ok := r.resolvers[prefix]
	if !ok {
		return "", false
	}

	return resolver.Resolve(parameter)
}

// IsDynamicField checks if a field name is a dynamic field reference.
func IsDynamicField(fieldName string) bool {
	if !strings.HasPrefix(fieldName, DynamicFieldPrefix) {
		return false
	}
	remainder := fieldName[len(DynamicFieldPrefix):]
	return strings.Contains(remainder, ":")
}

// RequestHeaderFieldResolver resolves @header_req:<header-name> field references.
// It extracts HTTP header values from the request context.
//
// Example usage in expressions:
//   - @header_req:X-Tvarr-Player ~ "hls.js"
//   - @header_req:User-Agent contains "Safari"
//   - @header_req:Accept == "application/vnd.apple.mpegurl"
type RequestHeaderFieldResolver struct {
	headers http.Header
}

// NewRequestHeaderFieldResolver creates a new resolver for HTTP request headers.
func NewRequestHeaderFieldResolver(headers http.Header) *RequestHeaderFieldResolver {
	return &RequestHeaderFieldResolver{headers: headers}
}

// Prefix returns "header_req" for this resolver.
func (r *RequestHeaderFieldResolver) Prefix() string {
	return "header_req"
}

// Resolve extracts the header value for the given header name.
// The header name is case-insensitive (as per HTTP spec).
func (r *RequestHeaderFieldResolver) Resolve(headerName string) (string, bool) {
	if r.headers == nil {
		return "", false
	}

	value := r.headers.Get(headerName)
	return value, value != ""
}
