package expression

import (
	"net/http"
	"testing"
)

func TestIsDynamicField(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		expected bool
	}{
		{"header_req field", "@header_req:X-Custom-Player", true},
		{"header_req with hyphen", "@header_req:Content-Type", true},
		{"missing colon", "@header_req", false},
		{"no prefix", "user_agent", false},
		{"empty string", "", false},
		{"just at sign", "@", false},
		{"at with colon but no prefix", "@:value", true}, // technically valid syntax
		{"future header_res field", "@header_res:Content-Type", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsDynamicField(tt.field)
			if got != tt.expected {
				t.Errorf("IsDynamicField(%q) = %v, want %v", tt.field, got, tt.expected)
			}
		})
	}
}

func TestRequestHeaderFieldResolver(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Tvarr-Player", "hls.js")
	headers.Set("User-Agent", "Mozilla/5.0")
	headers.Set("Accept", "application/vnd.apple.mpegurl")
	headers.Set("X-Custom-Header", "custom-value")

	resolver := NewRequestHeaderFieldResolver(headers)

	tests := []struct {
		name          string
		headerName    string
		expectedValue string
		expectedFound bool
	}{
		{"X-Tvarr-Player", "X-Tvarr-Player", "hls.js", true},
		{"User-Agent", "User-Agent", "Mozilla/5.0", true},
		{"Accept", "Accept", "application/vnd.apple.mpegurl", true},
		{"custom header", "X-Custom-Header", "custom-value", true},
		{"case insensitive", "x-tvarr-player", "hls.js", true},
		{"missing header", "X-Missing-Header", "", false},
		{"empty header name", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, found := resolver.Resolve(tt.headerName)
			if value != tt.expectedValue {
				t.Errorf("Resolve(%q) value = %q, want %q", tt.headerName, value, tt.expectedValue)
			}
			if found != tt.expectedFound {
				t.Errorf("Resolve(%q) found = %v, want %v", tt.headerName, found, tt.expectedFound)
			}
		})
	}
}

func TestRequestHeaderFieldResolverNilHeaders(t *testing.T) {
	resolver := NewRequestHeaderFieldResolver(nil)

	value, found := resolver.Resolve("X-Test")
	if value != "" || found != false {
		t.Errorf("Expected empty value and false for nil headers, got %q, %v", value, found)
	}
}

func TestDynamicFieldRegistry(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Tvarr-Player", "mpegts.js")
	headers.Set("Accept", "video/mp2t")

	registry := NewDynamicFieldRegistry()
	registry.Register(NewRequestHeaderFieldResolver(headers))

	tests := []struct {
		name          string
		field         string
		expectedValue string
		expectedFound bool
	}{
		{"header_req:X-Tvarr-Player", "@header_req:X-Tvarr-Player", "mpegts.js", true},
		{"header_req:Accept", "@header_req:Accept", "video/mp2t", true},
		{"case insensitive prefix", "@HEADER_REQ:X-Tvarr-Player", "mpegts.js", true},
		{"missing header", "@header_req:X-Missing", "", false},
		{"unknown prefix", "@unknown_prefix:value", "", false},
		{"not a dynamic field", "user_agent", "", false},
		{"malformed - no colon", "@header_req", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, found := registry.Resolve(tt.field)
			if value != tt.expectedValue {
				t.Errorf("Resolve(%q) value = %q, want %q", tt.field, value, tt.expectedValue)
			}
			if found != tt.expectedFound {
				t.Errorf("Resolve(%q) found = %v, want %v", tt.field, found, tt.expectedFound)
			}
		})
	}
}

func TestDynamicFieldAccessor(t *testing.T) {
	// Set up base accessor with static fields
	baseFields := map[string]string{
		"user_agent": "Test Browser",
		"client_ip":  "192.168.1.1",
	}
	baseAccessor := NewMapEvalContext(baseFields)

	// Set up dynamic registry with request headers
	headers := http.Header{}
	headers.Set("X-Tvarr-Player", "hls.js")
	headers.Set("X-Custom", "dynamic-value")

	registry := NewDynamicFieldRegistry()
	registry.Register(NewRequestHeaderFieldResolver(headers))

	accessor := NewDynamicFieldAccessor(baseAccessor, registry)

	tests := []struct {
		name          string
		field         string
		expectedValue string
		expectedFound bool
	}{
		// Dynamic fields should resolve
		{"dynamic header", "@header_req:X-Tvarr-Player", "hls.js", true},
		{"dynamic custom header", "@header_req:X-Custom", "dynamic-value", true},
		{"dynamic missing header", "@header_req:X-Missing", "", false},

		// Base fields should still work
		{"base user_agent", "user_agent", "Test Browser", true},
		{"base client_ip", "client_ip", "192.168.1.1", true},
		{"base missing", "missing_field", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, found := accessor.GetFieldValue(tt.field)
			if value != tt.expectedValue {
				t.Errorf("GetFieldValue(%q) value = %q, want %q", tt.field, value, tt.expectedValue)
			}
			if found != tt.expectedFound {
				t.Errorf("GetFieldValue(%q) found = %v, want %v", tt.field, found, tt.expectedFound)
			}
		})
	}
}

func TestDynamicFieldAccessorNilRegistry(t *testing.T) {
	baseFields := map[string]string{"user_agent": "Test"}
	baseAccessor := NewMapEvalContext(baseFields)

	// Create accessor with nil registry
	accessor := NewDynamicFieldAccessor(baseAccessor, nil)

	// Base fields should still work
	value, found := accessor.GetFieldValue("user_agent")
	if value != "Test" || !found {
		t.Errorf("Expected base field to work with nil registry, got %q, %v", value, found)
	}

	// Dynamic fields should return not found (no registry)
	value, found = accessor.GetFieldValue("@header_req:X-Test")
	if value != "" || found {
		t.Errorf("Expected dynamic field to fail with nil registry, got %q, %v", value, found)
	}
}

func TestEvaluatorWithDynamicFields(t *testing.T) {
	// Set up dynamic registry with request headers
	headers := http.Header{}
	headers.Set("X-Tvarr-Player", "hls.js")
	headers.Set("User-Agent", "Mozilla/5.0 Safari")

	registry := NewDynamicFieldRegistry()
	registry.Register(NewRequestHeaderFieldResolver(headers))

	// Base accessor with standard fields
	baseFields := map[string]string{
		"user_agent": "Mozilla/5.0 Safari",
	}
	baseAccessor := NewMapEvalContext(baseFields)

	evaluator := NewEvaluator()

	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		{
			name:       "match X-Tvarr-Player",
			expression: `@header_req:X-Tvarr-Player == "hls.js"`,
			expected:   true,
		},
		{
			name:       "X-Tvarr-Player contains",
			expression: `@header_req:X-Tvarr-Player contains "hls"`,
			expected:   true,
		},
		{
			name:       "X-Tvarr-Player regex",
			expression: `@header_req:X-Tvarr-Player =~ "hls\\.js"`,
			expected:   true,
		},
		{
			name:       "missing header equals empty",
			expression: `@header_req:X-Missing == ""`,
			expected:   true,
		},
		{
			name:       "combined with base field",
			expression: `user_agent contains "Safari" && @header_req:X-Tvarr-Player == "hls.js"`,
			expected:   true,
		},
		{
			name:       "no match",
			expression: `@header_req:X-Tvarr-Player == "mpegts.js"`,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := PreprocessAndParse(tt.expression)
			if err != nil {
				t.Fatalf("PreprocessAndParse(%q) error: %v", tt.expression, err)
			}

			result, err := evaluator.EvaluateWithDynamicFields(parsed, baseAccessor, registry)
			if err != nil {
				t.Fatalf("EvaluateWithDynamicFields error: %v", err)
			}

			if result.Matches != tt.expected {
				t.Errorf("Expression %q: got %v, want %v", tt.expression, result.Matches, tt.expected)
			}
		})
	}
}

// TestIntegrationDynamicFieldsWithRequestContext tests the integration of dynamic fields
// with the RequestContextAccessor, simulating how the RelayProfileMappingService uses them.
// This verifies that expressions using @header_req:<name> work correctly when combined
// with the standard request context fields.
func TestIntegrationDynamicFieldsWithRequestContext(t *testing.T) {
	// Create a mock HTTP request with various headers
	req, err := http.NewRequest("GET", "http://localhost/api/v1/relay/stream/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Set request headers (as would be sent by a player)
	req.Header.Set("X-Tvarr-Player", "hls.js")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android) ExoPlayer/2.18.0")
	req.Header.Set("Accept", "application/vnd.apple.mpegurl")
	req.Header.Set("X-Custom-Device-Type", "AndroidTV")
	req.Header.Set("X-Forwarded-For", "192.168.1.100")
	req.RemoteAddr = "10.0.0.1:54321"

	// Create the RequestContextAccessor (base accessor)
	baseAccessor := NewRequestContextAccessor(req)

	// Create dynamic field registry with request header resolver
	registry := NewDynamicFieldRegistry()
	registry.Register(NewRequestHeaderFieldResolver(req.Header))

	evaluator := NewEvaluator()

	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		// Test dynamic header fields with @header_req syntax
		{
			name:       "dynamic X-Tvarr-Player matches hls.js",
			expression: `@header_req:X-Tvarr-Player == "hls.js"`,
			expected:   true,
		},
		{
			name:       "dynamic custom device type header",
			expression: `@header_req:X-Custom-Device-Type == "AndroidTV"`,
			expected:   true,
		},
		{
			name:       "dynamic header case insensitive lookup",
			expression: `@header_req:x-tvarr-player == "hls.js"`,
			expected:   true,
		},
		{
			name:       "dynamic header contains pattern",
			expression: `@header_req:Accept contains "mpegurl"`,
			expected:   true,
		},

		// Test combining dynamic headers with base request context fields
		{
			name:       "dynamic header AND user_agent",
			expression: `@header_req:X-Tvarr-Player == "hls.js" && user_agent contains "ExoPlayer"`,
			expected:   true,
		},
		{
			name:       "dynamic header AND client_ip",
			expression: `@header_req:X-Custom-Device-Type == "AndroidTV" && client_ip starts_with "192.168"`,
			expected:   true,
		},
		{
			name:       "complex OR expression with dynamic headers",
			expression: `@header_req:X-Tvarr-Player == "mpegts.js" || @header_req:X-Tvarr-Player == "hls.js"`,
			expected:   true,
		},

		// Test non-matching scenarios
		{
			name:       "dynamic header wrong value",
			expression: `@header_req:X-Tvarr-Player == "video.js"`,
			expected:   false,
		},
		{
			name:       "dynamic header missing returns empty string",
			expression: `@header_req:X-Non-Existent-Header == ""`,
			expected:   true,
		},
		{
			name:       "complex expression with missing header",
			expression: `@header_req:X-Missing == "" && @header_req:X-Tvarr-Player == "hls.js"`,
			expected:   true,
		},

		// Test real-world client detection scenarios
		{
			name:       "detect ExoPlayer via user_agent and dynamic header",
			expression: `user_agent contains "ExoPlayer" && @header_req:Accept contains "mpegurl"`,
			expected:   true,
		},
		{
			name:       "device-specific routing with custom header",
			expression: `@header_req:X-Custom-Device-Type == "AndroidTV" && @header_req:X-Tvarr-Player == "hls.js"`,
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := PreprocessAndParse(tt.expression)
			if err != nil {
				t.Fatalf("PreprocessAndParse(%q) error: %v", tt.expression, err)
			}

			result, err := evaluator.EvaluateWithDynamicFields(parsed, baseAccessor, registry)
			if err != nil {
				t.Fatalf("EvaluateWithDynamicFields error: %v", err)
			}

			if result.Matches != tt.expected {
				t.Errorf("Expression %q: got %v, want %v", tt.expression, result.Matches, tt.expected)
			}
		})
	}
}
