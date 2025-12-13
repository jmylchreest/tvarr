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
		{"dynamic syntax", "@dynamic(request.headers):x-custom-player", true},
		{"dynamic with path", "@dynamic(request.query):format", true},
		{"incomplete dynamic syntax still has @", "@dynamic(request.headers)", true}, // has @ prefix so technically a dynamic field
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

	// Use NewDynamicFieldRegistryWithContext for @dynamic() syntax
	dynCtx := NewDynamicContext()
	dynCtx.SetRequestHeaders(headers)
	registry := NewDynamicFieldRegistryWithContext(dynCtx)

	tests := []struct {
		name          string
		field         string
		expectedValue string
		expectedFound bool
	}{
		{"dynamic X-Tvarr-Player", "@dynamic(request.headers):x-tvarr-player", "mpegts.js", true},
		{"dynamic Accept", "@dynamic(request.headers):accept", "video/mp2t", true},
		{"dynamic case insensitive key", "@dynamic(request.headers):X-TVARR-PLAYER", "mpegts.js", true},
		{"dynamic missing header", "@dynamic(request.headers):x-missing", "", false},
		{"unknown prefix", "@unknown_prefix:value", "", false},
		{"not a dynamic field", "user_agent", "", false},
		{"malformed - no colon", "@dynamic(request.headers)", "", false},
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

	// Set up dynamic registry with request headers using DynamicContext
	headers := http.Header{}
	headers.Set("X-Tvarr-Player", "hls.js")
	headers.Set("X-Custom", "dynamic-value")

	dynCtx := NewDynamicContext()
	dynCtx.SetRequestHeaders(headers)
	registry := NewDynamicFieldRegistryWithContext(dynCtx)

	accessor := NewDynamicFieldAccessor(baseAccessor, registry)

	tests := []struct {
		name          string
		field         string
		expectedValue string
		expectedFound bool
	}{
		// Dynamic fields should resolve
		{"dynamic header", "@dynamic(request.headers):x-tvarr-player", "hls.js", true},
		{"dynamic custom header", "@dynamic(request.headers):x-custom", "dynamic-value", true},
		{"dynamic missing header", "@dynamic(request.headers):x-missing", "", false},

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
	value, found = accessor.GetFieldValue("@dynamic(request.headers):x-test")
	if value != "" || found {
		t.Errorf("Expected dynamic field to fail with nil registry, got %q, %v", value, found)
	}
}

func TestEvaluatorWithDynamicFields(t *testing.T) {
	// Set up dynamic registry with request headers using DynamicContext
	headers := http.Header{}
	headers.Set("X-Tvarr-Player", "hls.js")
	headers.Set("User-Agent", "Mozilla/5.0 Safari")

	dynCtx := NewDynamicContext()
	dynCtx.SetRequestHeaders(headers)
	registry := NewDynamicFieldRegistryWithContext(dynCtx)

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
			expression: `@dynamic(request.headers):x-tvarr-player == "hls.js"`,
			expected:   true,
		},
		{
			name:       "X-Tvarr-Player contains",
			expression: `@dynamic(request.headers):x-tvarr-player contains "hls"`,
			expected:   true,
		},
		{
			name:       "X-Tvarr-Player regex",
			expression: `@dynamic(request.headers):x-tvarr-player =~ "hls\\.js"`,
			expected:   true,
		},
		{
			name:       "missing header equals empty",
			expression: `@dynamic(request.headers):x-missing == ""`,
			expected:   true,
		},
		{
			name:       "combined with base field",
			expression: `user_agent contains "Safari" && @dynamic(request.headers):x-tvarr-player == "hls.js"`,
			expected:   true,
		},
		{
			name:       "no match",
			expression: `@dynamic(request.headers):x-tvarr-player == "mpegts.js"`,
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
// This verifies that expressions using @dynamic(request.headers):key work correctly when combined
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

	// Create dynamic field registry with DynamicContext for @dynamic() syntax
	dynCtx := NewDynamicContext()
	dynCtx.SetRequestHeaders(req.Header)
	registry := NewDynamicFieldRegistryWithContext(dynCtx)

	evaluator := NewEvaluator()

	tests := []struct {
		name       string
		expression string
		expected   bool
	}{
		// Test dynamic header fields with @dynamic(request.headers) syntax
		{
			name:       "dynamic X-Tvarr-Player matches hls.js",
			expression: `@dynamic(request.headers):x-tvarr-player == "hls.js"`,
			expected:   true,
		},
		{
			name:       "dynamic custom device type header",
			expression: `@dynamic(request.headers):x-custom-device-type == "AndroidTV"`,
			expected:   true,
		},
		{
			name:       "dynamic header case insensitive lookup",
			expression: `@dynamic(request.headers):X-TVARR-PLAYER == "hls.js"`,
			expected:   true,
		},
		{
			name:       "dynamic header contains pattern",
			expression: `@dynamic(request.headers):accept contains "mpegurl"`,
			expected:   true,
		},

		// Test combining dynamic headers with base request context fields
		{
			name:       "dynamic header AND user_agent via header",
			expression: `@dynamic(request.headers):x-tvarr-player == "hls.js" && @dynamic(request.headers):user-agent contains "ExoPlayer"`,
			expected:   true,
		},
		{
			name:       "dynamic header AND client_ip (static field)",
			expression: `@dynamic(request.headers):x-custom-device-type == "AndroidTV" && client_ip starts_with "192.168"`,
			expected:   true,
		},
		{
			name:       "complex OR expression with dynamic headers",
			expression: `@dynamic(request.headers):x-tvarr-player == "mpegts.js" || @dynamic(request.headers):x-tvarr-player == "hls.js"`,
			expected:   true,
		},

		// Test non-matching scenarios
		{
			name:       "dynamic header wrong value",
			expression: `@dynamic(request.headers):x-tvarr-player == "video.js"`,
			expected:   false,
		},
		{
			name:       "dynamic header missing returns empty string",
			expression: `@dynamic(request.headers):x-non-existent-header == ""`,
			expected:   true,
		},
		{
			name:       "complex expression with missing header",
			expression: `@dynamic(request.headers):x-missing == "" && @dynamic(request.headers):x-tvarr-player == "hls.js"`,
			expected:   true,
		},

		// Test real-world client detection scenarios
		{
			name:       "detect ExoPlayer via dynamic headers",
			expression: `@dynamic(request.headers):user-agent contains "ExoPlayer" && @dynamic(request.headers):accept contains "mpegurl"`,
			expected:   true,
		},
		{
			name:       "device-specific routing with custom header",
			expression: `@dynamic(request.headers):x-custom-device-type == "AndroidTV" && @dynamic(request.headers):x-tvarr-player == "hls.js"`,
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

// TestExplicitVideoCodecHeader tests the @dynamic(request.headers):x-video-codec expression matching
// for explicit codec request scenarios (T044 - User Story 5: Explicit Codec Headers).
func TestExplicitVideoCodecHeader(t *testing.T) {
	evaluator := NewEvaluator()

	tests := []struct {
		name          string
		videoCodec    string // X-Video-Codec header value
		expression    string
		expectedMatch bool
		description   string
	}{
		{
			name:          "h265 exact match",
			videoCodec:    "h265",
			expression:    `@dynamic(request.headers):x-video-codec equals "h265"`,
			expectedMatch: true,
			description:   "Match explicit H.265 video codec request",
		},
		{
			name:          "hevc alias match",
			videoCodec:    "hevc",
			expression:    `@dynamic(request.headers):x-video-codec equals "hevc"`,
			expectedMatch: true,
			description:   "Match explicit HEVC video codec request (alias for H.265)",
		},
		{
			name:          "h264 exact match",
			videoCodec:    "h264",
			expression:    `@dynamic(request.headers):x-video-codec equals "h264"`,
			expectedMatch: true,
			description:   "Match explicit H.264 video codec request",
		},
		{
			name:          "avc alias match",
			videoCodec:    "avc",
			expression:    `@dynamic(request.headers):x-video-codec equals "avc"`,
			expectedMatch: true,
			description:   "Match explicit AVC video codec request (alias for H.264)",
		},
		{
			name:          "vp9 exact match",
			videoCodec:    "vp9",
			expression:    `@dynamic(request.headers):x-video-codec equals "vp9"`,
			expectedMatch: true,
			description:   "Match explicit VP9 video codec request",
		},
		{
			name:          "av1 exact match",
			videoCodec:    "av1",
			expression:    `@dynamic(request.headers):x-video-codec equals "av1"`,
			expectedMatch: true,
			description:   "Match explicit AV1 video codec request",
		},
		{
			name:          "h265 OR hevc expression",
			videoCodec:    "h265",
			expression:    `@dynamic(request.headers):x-video-codec equals "h265" OR @dynamic(request.headers):x-video-codec equals "hevc"`,
			expectedMatch: true,
			description:   "Match H.265 using OR expression with alias",
		},
		{
			name:          "hevc via OR expression",
			videoCodec:    "hevc",
			expression:    `@dynamic(request.headers):x-video-codec equals "h265" OR @dynamic(request.headers):x-video-codec equals "hevc"`,
			expectedMatch: true,
			description:   "Match HEVC using OR expression with canonical name",
		},
		{
			name:          "wrong codec no match",
			videoCodec:    "h264",
			expression:    `@dynamic(request.headers):x-video-codec equals "h265"`,
			expectedMatch: false,
			description:   "H.264 header should not match H.265 expression",
		},
		{
			name:          "case sensitivity - uppercase header value",
			videoCodec:    "H265",
			expression:    `@dynamic(request.headers):x-video-codec equals "h265"`,
			expectedMatch: false,
			description:   "Codec matching should be case sensitive",
		},
		{
			name:          "empty header returns empty string",
			videoCodec:    "",
			expression:    `@dynamic(request.headers):x-video-codec equals ""`,
			expectedMatch: true,
			description:   "Missing or empty header equals empty string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create HTTP request with X-Video-Codec header
			req, err := http.NewRequest("GET", "http://localhost/stream", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			if tt.videoCodec != "" {
				req.Header.Set("X-Video-Codec", tt.videoCodec)
			}

			// Create accessor with dynamic header resolver using DynamicContext
			baseAccessor := NewRequestContextAccessor(req)
			dynCtx := NewDynamicContext()
			dynCtx.SetRequestHeaders(req.Header)
			registry := NewDynamicFieldRegistryWithContext(dynCtx)

			// Parse and evaluate expression
			parsed, err := PreprocessAndParse(tt.expression)
			if err != nil {
				t.Fatalf("PreprocessAndParse(%q) error: %v", tt.expression, err)
			}

			result, err := evaluator.EvaluateWithDynamicFields(parsed, baseAccessor, registry)
			if err != nil {
				t.Fatalf("EvaluateWithDynamicFields error: %v", err)
			}

			if result.Matches != tt.expectedMatch {
				t.Errorf("%s: expression %q with X-Video-Codec=%q got %v, want %v",
					tt.description, tt.expression, tt.videoCodec, result.Matches, tt.expectedMatch)
			}
		})
	}
}

// TestExplicitAudioCodecHeader tests the @dynamic(request.headers):x-audio-codec expression matching
// for explicit codec request scenarios (T045 - User Story 5: Explicit Codec Headers).
func TestExplicitAudioCodecHeader(t *testing.T) {
	evaluator := NewEvaluator()

	tests := []struct {
		name          string
		audioCodec    string // X-Audio-Codec header value
		expression    string
		expectedMatch bool
		description   string
	}{
		{
			name:          "aac exact match",
			audioCodec:    "aac",
			expression:    `@dynamic(request.headers):x-audio-codec equals "aac"`,
			expectedMatch: true,
			description:   "Match explicit AAC audio codec request",
		},
		{
			name:          "opus exact match",
			audioCodec:    "opus",
			expression:    `@dynamic(request.headers):x-audio-codec equals "opus"`,
			expectedMatch: true,
			description:   "Match explicit Opus audio codec request",
		},
		{
			name:          "ac3 exact match",
			audioCodec:    "ac3",
			expression:    `@dynamic(request.headers):x-audio-codec equals "ac3"`,
			expectedMatch: true,
			description:   "Match explicit AC3/Dolby Digital audio codec request",
		},
		{
			name:          "eac3 exact match",
			audioCodec:    "eac3",
			expression:    `@dynamic(request.headers):x-audio-codec equals "eac3"`,
			expectedMatch: true,
			description:   "Match explicit E-AC3/Dolby Digital Plus audio codec request",
		},
		{
			name:          "mp3 exact match",
			audioCodec:    "mp3",
			expression:    `@dynamic(request.headers):x-audio-codec equals "mp3"`,
			expectedMatch: true,
			description:   "Match explicit MP3 audio codec request",
		},
		{
			name:          "wrong codec no match",
			audioCodec:    "aac",
			expression:    `@dynamic(request.headers):x-audio-codec equals "opus"`,
			expectedMatch: false,
			description:   "AAC header should not match Opus expression",
		},
		{
			name:          "case sensitivity - uppercase header value",
			audioCodec:    "AAC",
			expression:    `@dynamic(request.headers):x-audio-codec equals "aac"`,
			expectedMatch: false,
			description:   "Audio codec matching should be case sensitive",
		},
		{
			name:          "empty header returns empty string",
			audioCodec:    "",
			expression:    `@dynamic(request.headers):x-audio-codec equals ""`,
			expectedMatch: true,
			description:   "Missing or empty audio header equals empty string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create HTTP request with X-Audio-Codec header
			req, err := http.NewRequest("GET", "http://localhost/stream", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			if tt.audioCodec != "" {
				req.Header.Set("X-Audio-Codec", tt.audioCodec)
			}

			// Create accessor with dynamic header resolver using DynamicContext
			baseAccessor := NewRequestContextAccessor(req)
			dynCtx := NewDynamicContext()
			dynCtx.SetRequestHeaders(req.Header)
			registry := NewDynamicFieldRegistryWithContext(dynCtx)

			// Parse and evaluate expression
			parsed, err := PreprocessAndParse(tt.expression)
			if err != nil {
				t.Fatalf("PreprocessAndParse(%q) error: %v", tt.expression, err)
			}

			result, err := evaluator.EvaluateWithDynamicFields(parsed, baseAccessor, registry)
			if err != nil {
				t.Fatalf("EvaluateWithDynamicFields error: %v", err)
			}

			if result.Matches != tt.expectedMatch {
				t.Errorf("%s: expression %q with X-Audio-Codec=%q got %v, want %v",
					tt.description, tt.expression, tt.audioCodec, result.Matches, tt.expectedMatch)
			}
		})
	}
}

// TestCombinedVideoAndAudioCodecHeaders tests expressions that match both video and audio
// codec headers simultaneously, which is how client detection rules work in practice.
func TestCombinedVideoAndAudioCodecHeaders(t *testing.T) {
	evaluator := NewEvaluator()

	tests := []struct {
		name          string
		videoCodec    string
		audioCodec    string
		expression    string
		expectedMatch bool
		description   string
	}{
		{
			name:          "both video and audio match",
			videoCodec:    "h265",
			audioCodec:    "aac",
			expression:    `@dynamic(request.headers):x-video-codec equals "h265" AND @dynamic(request.headers):x-audio-codec equals "aac"`,
			expectedMatch: true,
			description:   "Both H.265 video and AAC audio headers match",
		},
		{
			name:          "video matches audio doesnt",
			videoCodec:    "h265",
			audioCodec:    "opus",
			expression:    `@dynamic(request.headers):x-video-codec equals "h265" AND @dynamic(request.headers):x-audio-codec equals "aac"`,
			expectedMatch: false,
			description:   "H.265 matches but audio is Opus not AAC",
		},
		{
			name:          "only video header set",
			videoCodec:    "vp9",
			audioCodec:    "",
			expression:    `@dynamic(request.headers):x-video-codec equals "vp9"`,
			expectedMatch: true,
			description:   "Only video codec header is set and matches",
		},
		{
			name:          "only audio header set",
			videoCodec:    "",
			audioCodec:    "opus",
			expression:    `@dynamic(request.headers):x-audio-codec equals "opus"`,
			expectedMatch: true,
			description:   "Only audio codec header is set and matches",
		},
		{
			name:          "av1 with opus (modern streaming combo)",
			videoCodec:    "av1",
			audioCodec:    "opus",
			expression:    `@dynamic(request.headers):x-video-codec equals "av1" AND @dynamic(request.headers):x-audio-codec equals "opus"`,
			expectedMatch: true,
			description:   "Modern AV1+Opus streaming combination matches",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create HTTP request with codec headers
			req, err := http.NewRequest("GET", "http://localhost/stream", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			if tt.videoCodec != "" {
				req.Header.Set("X-Video-Codec", tt.videoCodec)
			}
			if tt.audioCodec != "" {
				req.Header.Set("X-Audio-Codec", tt.audioCodec)
			}

			// Create accessor with dynamic header resolver using DynamicContext
			baseAccessor := NewRequestContextAccessor(req)
			dynCtx := NewDynamicContext()
			dynCtx.SetRequestHeaders(req.Header)
			registry := NewDynamicFieldRegistryWithContext(dynCtx)

			// Parse and evaluate expression
			parsed, err := PreprocessAndParse(tt.expression)
			if err != nil {
				t.Fatalf("PreprocessAndParse(%q) error: %v", tt.expression, err)
			}

			result, err := evaluator.EvaluateWithDynamicFields(parsed, baseAccessor, registry)
			if err != nil {
				t.Fatalf("EvaluateWithDynamicFields error: %v", err)
			}

			if result.Matches != tt.expectedMatch {
				t.Errorf("%s: expression %q with X-Video-Codec=%q, X-Audio-Codec=%q got %v, want %v",
					tt.description, tt.expression, tt.videoCodec, tt.audioCodec, result.Matches, tt.expectedMatch)
			}
		})
	}
}
