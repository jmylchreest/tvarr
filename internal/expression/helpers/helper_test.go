package helpers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHelperRegistry_Register(t *testing.T) {
	registry := NewHelperRegistry()

	helper := &mockHelper{name: "test"}
	registry.Register(helper)

	result, ok := registry.Get("test")
	require.True(t, ok)
	assert.Equal(t, "test", result.Name())
}

func TestHelperRegistry_GetNotFound(t *testing.T) {
	registry := NewHelperRegistry()

	_, ok := registry.Get("nonexistent")
	assert.False(t, ok)
}

func TestHelperRegistry_Process(t *testing.T) {
	registry := NewHelperRegistry()
	registry.Register(&mockHelper{
		name: "upper",
		processFunc: func(value, args string) (string, error) {
			return "PROCESSED:" + value, nil
		},
	})

	result, err := registry.Process("@upper:test")
	require.NoError(t, err)
	assert.Equal(t, "PROCESSED:test", result)
}

func TestHelperRegistry_ProcessNotHelper(t *testing.T) {
	registry := NewHelperRegistry()

	// Non-helper values should be returned unchanged
	result, err := registry.Process("regular value")
	require.NoError(t, err)
	assert.Equal(t, "regular value", result)
}

func TestHelperRegistry_ProcessUnknownHelper(t *testing.T) {
	registry := NewHelperRegistry()

	// Unknown helpers should return the original value
	result, err := registry.Process("@unknown:value")
	require.NoError(t, err)
	assert.Equal(t, "@unknown:value", result)
}

func TestTimeHelper_Now(t *testing.T) {
	helper := NewTimeHelper()

	before := time.Now()
	result, err := helper.Process("now", "")
	require.NoError(t, err)
	after := time.Now()

	// Parse the result
	parsed, err := time.Parse(time.RFC3339, result)
	require.NoError(t, err)

	// Should be within the test window
	assert.True(t, !parsed.Before(before.Truncate(time.Second)))
	assert.True(t, !parsed.After(after.Add(time.Second)))
}

func TestTimeHelper_NowWithFormat(t *testing.T) {
	helper := NewTimeHelper()

	result, err := helper.Process("now", "2006-01-02")
	require.NoError(t, err)

	// Should be a date in YYYY-MM-DD format
	_, err = time.Parse("2006-01-02", result)
	require.NoError(t, err)
}

func TestTimeHelper_Parse(t *testing.T) {
	helper := NewTimeHelper()

	result, err := helper.Process("parse", "2024-01-15T10:30:00Z")
	require.NoError(t, err)

	// Default output is RFC3339
	assert.Equal(t, "2024-01-15T10:30:00Z", result)
}

func TestTimeHelper_Format(t *testing.T) {
	helper := NewTimeHelper()

	// Format takes input|output_format
	result, err := helper.Process("format", "2024-01-15T10:30:00Z|2006-01-02")
	require.NoError(t, err)

	assert.Equal(t, "2024-01-15", result)
}

func TestTimeHelper_Add(t *testing.T) {
	helper := NewTimeHelper()

	// Add takes base_time|duration
	result, err := helper.Process("add", "2024-01-15T10:30:00Z|1h")
	require.NoError(t, err)

	assert.Equal(t, "2024-01-15T11:30:00Z", result)
}

func TestTimeHelper_InvalidOperation(t *testing.T) {
	helper := NewTimeHelper()

	_, err := helper.Process("invalid_operation", "")
	assert.Error(t, err)
}

func TestLogoHelper_ValidULID_Exists(t *testing.T) {
	validULID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	resolver := &mockLogoResolver{
		exists: map[string]bool{
			validULID: true,
		},
	}

	helper := NewLogoHelperWithConfig(LogoHelperConfig{
		BaseURL:  "https://example.com",
		Resolver: resolver,
	})

	result, err := helper.Process(validULID, "")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/api/v1/logos/01ARZ3NDEKTSV4RRFFQ69G5FAV", result)
}

func TestLogoHelper_ValidULID_NotFound(t *testing.T) {
	validULID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	resolver := &mockLogoResolver{
		exists: map[string]bool{}, // ULID doesn't exist
	}

	helper := NewLogoHelperWithConfig(LogoHelperConfig{
		BaseURL:  "https://example.com",
		Resolver: resolver,
	})

	// ULID not found returns empty string (removes field)
	result, err := helper.Process(validULID, "")
	require.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestLogoHelper_InvalidULID(t *testing.T) {
	resolver := &mockLogoResolver{
		exists: map[string]bool{},
	}

	helper := NewLogoHelper(resolver)

	// Invalid ULID format returns empty string
	result, err := helper.Process("not-a-ulid", "")
	require.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestLogoHelper_NoResolver_WithBaseURL(t *testing.T) {
	validULID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	helper := NewLogoHelperWithConfig(LogoHelperConfig{
		BaseURL:  "https://example.com",
		Resolver: nil,
	})

	// With base URL but no resolver, constructs URL anyway
	result, err := helper.Process(validULID, "")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/api/v1/logos/01ARZ3NDEKTSV4RRFFQ69G5FAV", result)
}

func TestLogoHelper_NoResolver_NoBaseURL(t *testing.T) {
	validULID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	helper := NewLogoHelper(nil)

	// No resolver and no base URL returns deferred syntax for later resolution by logo caching stage
	result, err := helper.Process(validULID, "")
	require.NoError(t, err)
	assert.Equal(t, "@logo:01ARZ3NDEKTSV4RRFFQ69G5FAV", result)
}

func TestLogoHelper_BaseURL_TrailingSlash(t *testing.T) {
	validULID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	resolver := &mockLogoResolver{
		exists: map[string]bool{validULID: true},
	}

	helper := NewLogoHelperWithConfig(LogoHelperConfig{
		BaseURL:  "https://example.com/", // With trailing slash
		Resolver: resolver,
	})

	result, err := helper.Process(validULID, "")
	require.NoError(t, err)
	// Should not have double slash
	assert.Equal(t, "https://example.com/api/v1/logos/01ARZ3NDEKTSV4RRFFQ69G5FAV", result)
}

func TestLogoHelper_LowercaseULID(t *testing.T) {
	// ULIDs are case-insensitive
	lowercaseULID := "01arz3ndektsv4rrffq69g5fav"
	resolver := &mockLogoResolver{
		exists: map[string]bool{lowercaseULID: true},
	}

	helper := NewLogoHelperWithConfig(LogoHelperConfig{
		BaseURL:  "https://example.com",
		Resolver: resolver,
	})

	result, err := helper.Process(lowercaseULID, "")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/api/v1/logos/01arz3ndektsv4rrffq69g5fav", result)
}

func TestIsValidULID(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"01ARZ3NDEKTSV4RRFFQ69G5FAV", true},            // Valid ULID
		{"01arz3ndektsv4rrffq69g5fav", true},            // Valid lowercase ULID
		{"00000000000000000000000000", true},            // All zeros
		{"7ZZZZZZZZZZZZZZZZZZZZZZZZZ", true},            // Max value
		{"not-a-ulid", false},                           // Wrong format
		{"01ARZ3NDEKTSV4RRFFQ69G5FA", false},            // Too short (25 chars)
		{"01ARZ3NDEKTSV4RRFFQ69G5FAVX", false},          // Too long (27 chars)
		{"01ARZ3NDEKTSV4RRFFQ69G5FAI", false},           // Contains I (invalid)
		{"01ARZ3NDEKTSV4RRFFQ69G5FAL", false},           // Contains L (invalid)
		{"01ARZ3NDEKTSV4RRFFQ69G5FAO", false},           // Contains O (invalid)
		{"01ARZ3NDEKTSV4RRFFQ69G5FAU", false},           // Contains U (invalid)
		{"", false},                                     // Empty
		{"550e8400-e29b-41d4-a716-446655440000", false}, // UUID format (not ULID)
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isValidULID(tt.input)
			assert.Equal(t, tt.valid, result)
		})
	}
}

func TestDefaultRegistry(t *testing.T) {
	registry := DefaultRegistry()

	// Should have time helper registered
	_, ok := registry.Get("time")
	assert.True(t, ok)
}

func TestParseHelperSyntax(t *testing.T) {
	tests := []struct {
		input    string
		isHelper bool
		name     string
		args     string
	}{
		{"@time:now", true, "time", "now"},
		{"@logo:abc123", true, "logo", "abc123"},
		{"@helper:arg1:arg2", true, "helper", "arg1:arg2"},
		{"regular_value", false, "", ""},
		{"@", false, "", ""},
		{"@:", false, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			isHelper, name, args := ParseHelperSyntax(tt.input)
			assert.Equal(t, tt.isHelper, isHelper, "isHelper mismatch")
			assert.Equal(t, tt.name, name, "name mismatch")
			assert.Equal(t, tt.args, args, "args mismatch")
		})
	}
}

// mockHelper implements Helper for testing.
type mockHelper struct {
	name        string
	processFunc func(value, args string) (string, error)
}

func (h *mockHelper) Name() string {
	return h.name
}

func (h *mockHelper) Process(value, args string) (string, error) {
	if h.processFunc != nil {
		return h.processFunc(value, args)
	}
	return value, nil
}

// mockLogoResolver implements LogoResolver for testing.
type mockLogoResolver struct {
	exists    map[string]bool
	shouldErr bool
}

func (r *mockLogoResolver) ResolveLogoURL(id string) (string, error) {
	if r.shouldErr {
		return "", assert.AnError
	}
	if exists, ok := r.exists[id]; ok && exists {
		return "https://example.com/api/v1/logos/" + id, nil
	}
	return "", nil
}

func (r *mockLogoResolver) LogoExists(id string) (bool, error) {
	if r.shouldErr {
		return false, assert.AnError
	}
	if exists, ok := r.exists[id]; ok {
		return exists, nil
	}
	return false, nil
}
