package expression

import (
	"net/http"
	"testing"
)

func TestDynamicContext_RequestHeaders(t *testing.T) {
	ctx := NewDynamicContext()
	headers := http.Header{
		"X-Video-Codec": []string{"h265"},
		"X-Audio-Codec": []string{"aac"},
		"User-Agent":    []string{"Mozilla/5.0"},
	}
	ctx.SetRequestHeaders(headers)

	tests := []struct {
		name     string
		path     string
		key      string
		expected string
		found    bool
	}{
		{
			name:     "video codec header",
			path:     "request.headers",
			key:      "x-video-codec",
			expected: "h265",
			found:    true,
		},
		{
			name:     "audio codec header case insensitive",
			path:     "request.headers",
			key:      "X-AUDIO-CODEC",
			expected: "aac",
			found:    true,
		},
		{
			name:     "user agent",
			path:     "request.headers",
			key:      "user-agent",
			expected: "Mozilla/5.0",
			found:    true,
		},
		{
			name:     "missing header",
			path:     "request.headers",
			key:      "x-missing",
			expected: "",
			found:    false,
		},
		{
			name:     "wrong path",
			path:     "response.headers",
			key:      "x-video-codec",
			expected: "",
			found:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := ctx.Resolve(tt.path, tt.key)
			if ok != tt.found {
				t.Errorf("expected found=%v, got found=%v", tt.found, ok)
			}
			if val != tt.expected {
				t.Errorf("expected value=%q, got value=%q", tt.expected, val)
			}
		})
	}
}

func TestDynamicContext_QueryParams(t *testing.T) {
	ctx := NewDynamicContext()
	ctx.SetQueryParams(map[string]string{
		"format":  "hls",
		"quality": "1080p",
	})

	val, ok := ctx.Resolve("request.query", "format")
	if !ok || val != "hls" {
		t.Errorf("expected format=hls, got %q (ok=%v)", val, ok)
	}

	val, ok = ctx.Resolve("request.query", "quality")
	if !ok || val != "1080p" {
		t.Errorf("expected quality=1080p, got %q (ok=%v)", val, ok)
	}

	val, ok = ctx.Resolve("request.query", "missing")
	if ok || val != "" {
		t.Errorf("expected empty for missing param, got %q (ok=%v)", val, ok)
	}
}

func TestDynamicContext_CustomPath(t *testing.T) {
	ctx := NewDynamicContext()
	ctx.Set("custom.data", map[string]string{
		"key1": "value1",
		"key2": "value2",
	})

	val, ok := ctx.Resolve("custom.data", "key1")
	if !ok || val != "value1" {
		t.Errorf("expected key1=value1, got %q (ok=%v)", val, ok)
	}
}

func TestParseDynamicSyntax(t *testing.T) {
	tests := []struct {
		input    string
		wantPath string
		wantKey  string
		wantOk   bool
	}{
		{
			input:    "@dynamic(request.headers):x-video-codec",
			wantPath: "request.headers",
			wantKey:  "x-video-codec",
			wantOk:   true,
		},
		{
			input:    "@dynamic(request.query):format",
			wantPath: "request.query",
			wantKey:  "format",
			wantOk:   true,
		},
		{
			input:    "@dynamic(source.metadata):bitrate",
			wantPath: "source.metadata",
			wantKey:  "bitrate",
			wantOk:   true,
		},
		{
			input:    "@header_req:x-video-codec", // legacy syntax
			wantPath: "",
			wantKey:  "",
			wantOk:   false,
		},
		{
			input:    "not_dynamic",
			wantPath: "",
			wantKey:  "",
			wantOk:   false,
		},
		{
			input:    "@dynamic(path)", // missing :key
			wantPath: "",
			wantKey:  "",
			wantOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			path, key, ok := ParseDynamicSyntax(tt.input)
			if ok != tt.wantOk {
				t.Errorf("ParseDynamicSyntax(%q) ok=%v, want %v", tt.input, ok, tt.wantOk)
			}
			if path != tt.wantPath {
				t.Errorf("ParseDynamicSyntax(%q) path=%q, want %q", tt.input, path, tt.wantPath)
			}
			if key != tt.wantKey {
				t.Errorf("ParseDynamicSyntax(%q) key=%q, want %q", tt.input, key, tt.wantKey)
			}
		})
	}
}

func TestDynamicFieldRegistry_UnifiedSyntax(t *testing.T) {
	ctx := NewDynamicContext()
	ctx.SetRequestHeadersMap(map[string]string{
		"x-video-codec": "h265",
		"x-audio-codec": "aac",
	})
	ctx.SetQueryParams(map[string]string{
		"format": "dash",
	})

	registry := NewDynamicFieldRegistryWithContext(ctx)

	tests := []struct {
		name     string
		field    string
		expected string
		found    bool
	}{
		{
			name:     "unified video codec",
			field:    "@dynamic(request.headers):x-video-codec",
			expected: "h265",
			found:    true,
		},
		{
			name:     "unified audio codec",
			field:    "@dynamic(request.headers):x-audio-codec",
			expected: "aac",
			found:    true,
		},
		{
			name:     "unified query param",
			field:    "@dynamic(request.query):format",
			expected: "dash",
			found:    true,
		},
		{
			name:     "unified missing key",
			field:    "@dynamic(request.headers):x-missing",
			expected: "",
			found:    false,
		},
		{
			name:     "unified invalid path",
			field:    "@dynamic(nonexistent.path):key",
			expected: "",
			found:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := registry.Resolve(tt.field)
			if ok != tt.found {
				t.Errorf("Resolve(%q) ok=%v, want %v", tt.field, ok, tt.found)
			}
			if val != tt.expected {
				t.Errorf("Resolve(%q) val=%q, want %q", tt.field, val, tt.expected)
			}
		})
	}
}

func TestLexer_DynamicSyntax(t *testing.T) {
	tests := []struct {
		input    string
		wantType TokenType
		wantVal  string
	}{
		{
			input:    "@dynamic(request.headers):x-video-codec",
			wantType: TokenIdent,
			wantVal:  "@dynamic(request.headers):x-video-codec",
		},
		{
			input:    "@dynamic(request.query):format",
			wantType: TokenIdent,
			wantVal:  "@dynamic(request.query):format",
		},
		{
			input:    "@dynamic(source.metadata):bitrate",
			wantType: TokenIdent,
			wantVal:  "@dynamic(source.metadata):bitrate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			lexer := NewLexer(tt.input)
			tokens, err := lexer.Tokenize()
			if err != nil {
				t.Fatalf("Tokenize error: %v", err)
			}
			if len(tokens) < 1 {
				t.Fatalf("expected at least 1 token, got %d", len(tokens))
			}
			if tokens[0].Type != tt.wantType {
				t.Errorf("token type = %v, want %v", tokens[0].Type, tt.wantType)
			}
			if tokens[0].Value != tt.wantVal {
				t.Errorf("token value = %q, want %q", tokens[0].Value, tt.wantVal)
			}
		})
	}
}

func TestParser_DynamicSyntaxInSET(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantPath string
		wantKey  string
	}{
		{
			name:     "unified syntax in SET for headers",
			input:    `true SET preferred_video_codec = @dynamic(request.headers):x-video-codec`,
			wantPath: "request.headers",
			wantKey:  "x-video-codec",
		},
		{
			name:     "unified syntax in SET for query params",
			input:    `true SET preferred_format = @dynamic(request.query):format`,
			wantPath: "request.query",
			wantKey:  "format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			cwa, ok := parsed.Expression.(*ConditionWithActions)
			if !ok {
				t.Fatalf("expected ConditionWithActions, got %T", parsed.Expression)
			}

			if len(cwa.Actions) < 1 {
				t.Fatalf("expected at least 1 action")
			}

			action := cwa.Actions[0]
			dfr, ok := action.Value.(*DynamicFieldReference)
			if !ok {
				t.Fatalf("expected DynamicFieldReference, got %T", action.Value)
			}

			if !dfr.IsUnified() {
				t.Errorf("expected unified syntax")
			}
			if dfr.Path != tt.wantPath {
				t.Errorf("path = %q, want %q", dfr.Path, tt.wantPath)
			}
			if dfr.Key != tt.wantKey {
				t.Errorf("key = %q, want %q", dfr.Key, tt.wantKey)
			}
		})
	}
}

func TestRuleProcessor_DynamicUnifiedSyntax(t *testing.T) {
	ctx := NewDynamicContext()
	ctx.SetRequestHeadersMap(map[string]string{
		"x-video-codec": "h265",
	})

	registry := NewDynamicFieldRegistryWithContext(ctx)

	parsed, err := Parse(`true SET preferred_video_codec = @dynamic(request.headers):x-video-codec`)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Create a modifiable context for testing
	modCtx := &testModifiableContext{
		fields: map[string]string{},
	}

	processor := NewRuleProcessor().WithDynamicRegistry(registry)
	result, err := processor.Apply(parsed, modCtx)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	if !result.Matched {
		t.Error("expected rule to match")
	}

	if len(result.Modifications) != 1 {
		t.Fatalf("expected 1 modification, got %d", len(result.Modifications))
	}

	if result.Modifications[0].NewValue != "h265" {
		t.Errorf("expected NewValue=h265, got %q", result.Modifications[0].NewValue)
	}
}

// testModifiableContext is a simple test implementation of ModifiableContext
type testModifiableContext struct {
	fields map[string]string
}

func (c *testModifiableContext) GetFieldValue(name string) (string, bool) {
	v, ok := c.fields[name]
	return v, ok
}

func (c *testModifiableContext) SetFieldValue(name, value string) {
	c.fields[name] = value
}
