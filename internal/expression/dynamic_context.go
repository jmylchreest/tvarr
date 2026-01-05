package expression

import (
	"net/http"
	"strings"
)

// DynamicContext holds all dynamic data accessible via @dynamic(path):key syntax.
// The context is organized as a nested map structure where each path component
// navigates deeper into the hierarchy.
//
// Standard paths:
//   - request.headers - HTTP request headers (case-insensitive keys)
//   - request.query   - URL query parameters
//   - response.headers - HTTP response headers (if available)
//   - source.metadata  - Source-specific metadata
//
// Example usage in expressions:
//
//	Condition: @dynamic(request.headers):x-video-codec not_equals ""
//	SET value: SET preferred_video_codec = @dynamic(request.headers):x-video-codec
type DynamicContext struct {
	data map[string]any
}

// NewDynamicContext creates a new empty dynamic context.
func NewDynamicContext() *DynamicContext {
	return &DynamicContext{
		data: make(map[string]any),
	}
}

// SetRequestHeaders sets HTTP request headers in the context.
// Headers are stored with case-insensitive keys.
func (c *DynamicContext) SetRequestHeaders(headers http.Header) *DynamicContext {
	// Ensure request map exists
	request := c.getOrCreateMap("request")

	// Convert http.Header to case-insensitive map
	headerMap := make(map[string]string)
	for key, values := range headers {
		if len(values) > 0 {
			headerMap[strings.ToLower(key)] = values[0]
		}
	}
	request["headers"] = headerMap
	return c
}

// SetRequestHeadersMap sets request headers from a string map.
func (c *DynamicContext) SetRequestHeadersMap(headers map[string]string) *DynamicContext {
	request := c.getOrCreateMap("request")

	// Normalize keys to lowercase
	headerMap := make(map[string]string)
	for key, value := range headers {
		headerMap[strings.ToLower(key)] = value
	}
	request["headers"] = headerMap
	return c
}

// SetQueryParams sets URL query parameters in the context.
func (c *DynamicContext) SetQueryParams(params map[string]string) *DynamicContext {
	request := c.getOrCreateMap("request")
	request["query"] = params
	return c
}

// SetResponseHeaders sets HTTP response headers in the context.
func (c *DynamicContext) SetResponseHeaders(headers map[string]string) *DynamicContext {
	response := c.getOrCreateMap("response")

	// Normalize keys to lowercase
	headerMap := make(map[string]string)
	for key, value := range headers {
		headerMap[strings.ToLower(key)] = value
	}
	response["headers"] = headerMap
	return c
}

// SetSourceMetadata sets source-specific metadata in the context.
func (c *DynamicContext) SetSourceMetadata(metadata map[string]string) *DynamicContext {
	source := c.getOrCreateMap("source")
	source["metadata"] = metadata
	return c
}

// Set sets a value at an arbitrary path.
// Path components are separated by dots (e.g., "custom.data.key").
func (c *DynamicContext) Set(path string, value any) *DynamicContext {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return c
	}

	current := c.data
	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]
		if next, ok := current[part].(map[string]any); ok {
			current = next
		} else {
			// Create intermediate map
			next := make(map[string]any)
			current[part] = next
			current = next
		}
	}

	current[parts[len(parts)-1]] = value
	return c
}

// Resolve resolves a dynamic field reference.
// Path format: "path.to.map" and key is looked up in that map.
// Returns the value and true if found, or empty string and false if not.
func (c *DynamicContext) Resolve(path, key string) (string, bool) {
	if c.data == nil {
		return "", false
	}

	// Navigate to the target map
	target := c.navigateToMap(path)
	if target == nil {
		return "", false
	}

	// Look up the key (case-insensitive for headers)
	keyLower := strings.ToLower(key)
	for k, v := range target {
		if strings.ToLower(k) == keyLower {
			if strVal, ok := v.(string); ok {
				return strVal, strVal != ""
			}
		}
	}

	return "", false
}

// navigateToMap follows a dot-separated path to find the target map.
func (c *DynamicContext) navigateToMap(path string) map[string]any {
	if path == "" {
		return c.data
	}

	parts := strings.Split(path, ".")
	current := c.data

	for _, part := range parts {
		if next, ok := current[part]; ok {
			switch v := next.(type) {
			case map[string]any:
				current = v
			case map[string]string:
				// Convert to map[string]any
				result := make(map[string]any, len(v))
				for k, val := range v {
					result[k] = val
				}
				return result
			default:
				return nil
			}
		} else {
			return nil
		}
	}

	return current
}

// getOrCreateMap gets or creates a map at the top level.
func (c *DynamicContext) getOrCreateMap(key string) map[string]any {
	if existing, ok := c.data[key].(map[string]any); ok {
		return existing
	}
	newMap := make(map[string]any)
	c.data[key] = newMap
	return newMap
}

// ToMap returns the internal data as a map for compatibility.
func (c *DynamicContext) ToMap() map[string]any {
	return c.data
}

// DynamicContextResolver implements DynamicFieldResolver for @dynamic(path):key syntax.
// It resolves field references using the DynamicContext.
type DynamicContextResolver struct {
	ctx *DynamicContext
}

// NewDynamicContextResolver creates a resolver for a given context.
func NewDynamicContextResolver(ctx *DynamicContext) *DynamicContextResolver {
	return &DynamicContextResolver{ctx: ctx}
}

// Prefix returns "dynamic" as the resolver prefix.
// Note: The actual syntax is @dynamic(path):key, which is handled specially by the parser.
func (r *DynamicContextResolver) Prefix() string {
	return "dynamic"
}

// Resolve resolves a dynamic field reference.
// The parameter format is "path):key" (path already stripped of opening paren by caller).
func (r *DynamicContextResolver) Resolve(parameter string) (string, bool) {
	if r.ctx == nil {
		return "", false
	}

	// Parse "path):key" format
	before, after, ok := strings.Cut(parameter, ")")
	if !ok {
		return "", false
	}

	path := before
	rest := after

	// Expect ":key" after the closing paren
	if !strings.HasPrefix(rest, ":") || len(rest) < 2 {
		return "", false
	}

	key := rest[1:]
	return r.ctx.Resolve(path, key)
}

// --- Helper Functions ---

// ParseDynamicSyntax parses the @dynamic(path):key syntax.
// Returns the path, key, and whether parsing succeeded.
//
// Input: "@dynamic(request.headers):x-video-codec"
// Output: path="request.headers", key="x-video-codec", ok=true
func ParseDynamicSyntax(s string) (path, key string, ok bool) {
	// Must start with @dynamic(
	if !strings.HasPrefix(s, "@dynamic(") {
		return "", "", false
	}

	// Find closing paren
	rest := s[len("@dynamic("):]
	before, after, ok0 := strings.Cut(rest, ")")
	if !ok0 {
		return "", "", false
	}

	path = before

	// Expect ":key" after closing paren
	afterParen := after
	if !strings.HasPrefix(afterParen, ":") || len(afterParen) < 2 {
		return "", "", false
	}

	key = afterParen[1:]
	return path, key, true
}

// IsDynamicSyntax checks if a string uses the @dynamic(path):key syntax.
func IsDynamicSyntax(s string) bool {
	return strings.HasPrefix(s, "@dynamic(")
}

// FormatDynamicSyntax formats a dynamic field reference.
func FormatDynamicSyntax(path, key string) string {
	return "@dynamic(" + path + "):" + key
}
