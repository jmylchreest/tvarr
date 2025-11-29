package expression

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidator_ValidExpression(t *testing.T) {
	v := NewValidator(nil)

	result := v.Validate(`channel_name contains "BBC"`, DomainStreamFilter)

	assert.True(t, result.IsValid)
	assert.Empty(t, result.Errors)
	assert.NotEmpty(t, result.CanonicalExpression)
	assert.NotNil(t, result.ExpressionTree)
}

func TestValidator_EmptyExpression(t *testing.T) {
	v := NewValidator(nil)

	result := v.Validate("", DomainStreamFilter)

	assert.True(t, result.IsValid)
	assert.Empty(t, result.Errors)
}

func TestValidator_WhitespaceExpression(t *testing.T) {
	v := NewValidator(nil)

	result := v.Validate("   ", DomainStreamFilter)

	assert.True(t, result.IsValid)
	assert.Empty(t, result.Errors)
}

func TestValidator_InvalidSyntax(t *testing.T) {
	v := NewValidator(nil)

	result := v.Validate(`channel_name contains`, DomainStreamFilter)

	assert.False(t, result.IsValid)
	assert.NotEmpty(t, result.Errors)
	assert.Equal(t, ErrorCategorySyntax, result.Errors[0].Category)
}

func TestValidator_UnknownField(t *testing.T) {
	v := NewValidator(nil)

	result := v.Validate(`unknown_field contains "test"`, DomainStreamFilter)

	assert.False(t, result.IsValid)
	assert.NotEmpty(t, result.Errors)
	assert.Equal(t, ErrorCategoryField, result.Errors[0].Category)
	assert.Equal(t, "unknown_field", result.Errors[0].ErrorType)
}

func TestValidator_FieldWithSuggestion(t *testing.T) {
	v := NewValidator(nil)

	// "chanel_name" is a typo for "channel_name"
	result := v.Validate(`chanel_name contains "test"`, DomainStreamFilter)

	assert.False(t, result.IsValid)
	assert.NotEmpty(t, result.Errors)
	// Should have a suggestion in the details
	assert.Contains(t, result.Errors[0].Details, "channel_name")
}

func TestValidator_DefaultDomains(t *testing.T) {
	v := NewValidator(nil)

	// Should validate against both stream and EPG domains by default
	result := v.Validate(`channel_name contains "BBC"`)

	assert.True(t, result.IsValid)
}

func TestValidator_StreamDomain(t *testing.T) {
	v := NewValidator(nil)

	// Stream field should be valid in stream domain
	result := v.Validate(`channel_name contains "test"`, DomainStreamFilter)
	assert.True(t, result.IsValid)

	// Stream URL should be valid
	result = v.Validate(`stream_url starts_with "https"`, DomainStreamFilter)
	assert.True(t, result.IsValid)
}

func TestValidator_EPGDomain(t *testing.T) {
	v := NewValidator(nil)

	// EPG field should be valid in EPG domain
	result := v.Validate(`programme_title contains "News"`, DomainEPGFilter)
	assert.True(t, result.IsValid)

	// Programme description should be valid
	result = v.Validate(`programme_description contains "documentary"`, DomainEPGFilter)
	assert.True(t, result.IsValid)
}

func TestValidator_FieldAlias(t *testing.T) {
	v := NewValidator(nil)

	// "name" is an alias for "channel_name"
	result := v.Validate(`name contains "BBC"`, DomainStreamFilter)
	assert.True(t, result.IsValid)

	// "group" is an alias for "group_title"
	result = v.Validate(`group equals "Movies"`, DomainStreamFilter)
	assert.True(t, result.IsValid)

	// "title" is an alias for "programme_title"
	result = v.Validate(`title contains "News"`, DomainEPGFilter)
	assert.True(t, result.IsValid)
}

func TestValidator_ComplexExpression(t *testing.T) {
	v := NewValidator(nil)

	result := v.Validate(
		`channel_name contains "BBC" AND (group_title equals "News" OR group_title equals "Sports")`,
		DomainStreamFilter,
	)

	assert.True(t, result.IsValid)
	assert.Empty(t, result.Errors)
}

func TestValidator_ExpressionWithActions(t *testing.T) {
	v := NewValidator(nil)

	result := v.Validate(
		`channel_name contains "BBC" SET group_title = "UK Channels"`,
		DomainStreamMapping,
	)

	assert.True(t, result.IsValid)
	assert.Empty(t, result.Errors)
}

func TestValidator_SymbolicOperators(t *testing.T) {
	v := NewValidator(nil)

	// Test symbolic operators get preprocessed correctly
	result := v.Validate(`channel_name == "BBC"`, DomainStreamFilter)
	assert.True(t, result.IsValid)

	result = v.Validate(`channel_name != "BBC"`, DomainStreamFilter)
	assert.True(t, result.IsValid)

	result = v.Validate(`channel_name == "A" && group_title != "B"`, DomainStreamFilter)
	assert.True(t, result.IsValid)
}

func TestValidator_MultipleDomains(t *testing.T) {
	v := NewValidator(nil)

	// When validating against multiple domains, union of fields should be valid
	result := v.Validate(`channel_name contains "test"`, DomainStreamFilter, DomainEPGFilter)
	assert.True(t, result.IsValid)
}

func TestValidator_ExpressionTree(t *testing.T) {
	v := NewValidator(nil)

	result := v.Validate(`channel_name contains "BBC"`, DomainStreamFilter)

	assert.True(t, result.IsValid)
	assert.NotNil(t, result.ExpressionTree)

	// Verify the tree contains expected structure
	var tree map[string]any
	err := json.Unmarshal(result.ExpressionTree, &tree)
	assert.NoError(t, err)
	assert.Equal(t, "condition_only", tree["type"])
}
