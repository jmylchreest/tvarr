package handlers

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/expression"
)

// ExpressionHandler handles expression-related API endpoints.
type ExpressionHandler struct {
	validator *expression.Validator
}

// NewExpressionHandler creates a new expression handler.
func NewExpressionHandler() *ExpressionHandler {
	return &ExpressionHandler{
		validator: expression.NewValidator(nil), // Uses global registry
	}
}

// Register registers the expression routes with the API.
func (h *ExpressionHandler) Register(api huma.API) {
	// Register fields endpoints for filters and data-mapping
	huma.Register(api, huma.Operation{
		OperationID: "getFilterFieldsStream",
		Method:      "GET",
		Path:        "/api/v1/filters/fields/stream",
		Summary:     "Get available stream filter fields",
		Description: "Returns all fields available for stream filtering expressions",
		Tags:        []string{"Expressions"},
	}, h.GetFilterFieldsStream)

	huma.Register(api, huma.Operation{
		OperationID: "getFilterFieldsEPG",
		Method:      "GET",
		Path:        "/api/v1/filters/fields/epg",
		Summary:     "Get available EPG filter fields",
		Description: "Returns all fields available for EPG filtering expressions",
		Tags:        []string{"Expressions"},
	}, h.GetFilterFieldsEPG)

	huma.Register(api, huma.Operation{
		OperationID: "getDataMappingFieldsStream",
		Method:      "GET",
		Path:        "/api/v1/data-mapping/fields/stream",
		Summary:     "Get available stream data mapping fields",
		Description: "Returns all fields available for stream data mapping expressions",
		Tags:        []string{"Expressions"},
	}, h.GetDataMappingFieldsStream)

	huma.Register(api, huma.Operation{
		OperationID: "getDataMappingFieldsEPG",
		Method:      "GET",
		Path:        "/api/v1/data-mapping/fields/epg",
		Summary:     "Get available EPG data mapping fields",
		Description: "Returns all fields available for EPG data mapping expressions",
		Tags:        []string{"Expressions"},
	}, h.GetDataMappingFieldsEPG)

	huma.Register(api, huma.Operation{
		OperationID: "validateExpression",
		Method:      "POST",
		Path:        "/api/v1/expressions/validate",
		Summary:     "Validate expression",
		Description: `Validate expression syntax and semantic correctness including context-specific field names.

## Domain Types
- **stream_filter** or **stream**: Stream source filtering (channel_name, group_title, stream_url, etc.)
- **epg_filter** or **epg**: EPG source filtering (programme_title, programme_description, start_time, etc.)
- **stream_mapping**: Stream data transformation mapping
- **epg_mapping**: EPG data transformation mapping

## Query Parameters
- **domain**: Comma-separated list of domains to validate against. If not specified, defaults to stream_filter,epg_filter.

## Examples
- Stream context: channel_name contains "HD" AND group_title equals "Sports"
- EPG context: programme_title contains "News" AND start_time > "18:00"
- Data mapping: channel_name matches ".*HD.*" SET mapped_name = "High Definition"

## Field Validation
The endpoint validates field names against the appropriate schema for the specified domain(s),
providing intelligent suggestions for typos and unknown fields.`,
		Tags: []string{"Expressions"},
	}, h.Validate)
}

// ValidateExpressionInput is the input for validating an expression.
type ValidateExpressionInput struct {
	// Domain query parameter - comma-separated list of domains
	Domain string `query:"domain" doc:"Comma-separated list of domains to validate against (stream_filter, epg_filter, stream_mapping, epg_mapping). Defaults to stream_filter,epg_filter if not specified." required:"false"`
	Body   ValidateExpressionRequest
}

// ValidateExpressionRequest is the request body for expression validation.
type ValidateExpressionRequest struct {
	Expression string `json:"expression" doc:"The expression to validate" example:"channel_name contains \"HD\" OR (group_title equals \"Movies\" AND stream_url starts_with \"https\")"`
}

// ValidateExpressionOutput is the output for expression validation.
type ValidateExpressionOutput struct {
	Body ValidateExpressionResponse
}

// ValidateExpressionResponse is the response body for expression validation.
type ValidateExpressionResponse struct {
	IsValid             bool                            `json:"is_valid" doc:"Whether the expression is valid"`
	CanonicalExpression string                          `json:"canonical_expression,omitempty" doc:"The canonical form of the expression (if valid)"`
	Errors              []ExpressionValidationError     `json:"errors" doc:"List of validation errors (if invalid)"`
	ExpressionTree      map[string]any                  `json:"expression_tree,omitempty" doc:"JSON representation of the parsed expression tree (if valid)"`
}

// ExpressionValidationError represents a single validation error.
type ExpressionValidationError struct {
	Category   string `json:"category" doc:"Error category (syntax, field, operator, value)"`
	ErrorType  string `json:"error_type" doc:"Specific error type"`
	Message    string `json:"message" doc:"Human-readable error message"`
	Details    string `json:"details,omitempty" doc:"Detailed error description"`
	Position   *int   `json:"position,omitempty" doc:"Character position of the error in the expression"`
	Context    string `json:"context,omitempty" doc:"Context around the error location"`
	Suggestion string `json:"suggestion,omitempty" doc:"Suggestion for fixing the error"`
}

// Validate validates an expression.
func (h *ExpressionHandler) Validate(ctx context.Context, input *ValidateExpressionInput) (*ValidateExpressionOutput, error) {
	// Parse domain parameter
	var domains []expression.ExpressionDomain
	if input.Domain != "" {
		for _, part := range strings.Split(input.Domain, ",") {
			part = strings.TrimSpace(strings.ToLower(part))
			if domain, ok := expression.ParseExpressionDomain(part); ok {
				domains = append(domains, domain)
			}
		}
	}

	// Validate the expression
	result := h.validator.Validate(input.Body.Expression, domains...)

	// Convert to response format
	resp := &ValidateExpressionOutput{
		Body: ValidateExpressionResponse{
			IsValid:             result.IsValid,
			CanonicalExpression: result.CanonicalExpression,
			Errors:              make([]ExpressionValidationError, 0, len(result.Errors)),
		},
	}

	// Convert errors
	for _, err := range result.Errors {
		resp.Body.Errors = append(resp.Body.Errors, ExpressionValidationError{
			Category:   string(err.Category),
			ErrorType:  err.ErrorType,
			Message:    err.Message,
			Details:    err.Details,
			Position:   err.Position,
			Context:    err.Context,
			Suggestion: err.Suggestion,
		})
	}

	// Convert expression tree if present
	if result.ExpressionTree != nil {
		var tree map[string]any
		if err := json.Unmarshal(result.ExpressionTree, &tree); err == nil {
			resp.Body.ExpressionTree = tree
		}
	}

	return resp, nil
}

// FieldResponse represents a field in the API response.
type FieldResponse struct {
	Name        string   `json:"name" doc:"Field name"`
	Type        string   `json:"type" doc:"Field data type (string, integer, float, boolean, datetime)"`
	Description string   `json:"description" doc:"Field description"`
	Aliases     []string `json:"aliases,omitempty" doc:"Alternative field names"`
	ReadOnly    bool     `json:"read_only" doc:"Whether the field is read-only"`
	SourceType  string   `json:"source_type" doc:"Source type (stream or epg)"`
}

// FieldsOutput is the output for field listing endpoints.
type FieldsOutput struct {
	Body []FieldResponse
}

// getFieldsForDomain returns fields for a given domain.
func (h *ExpressionHandler) getFieldsForDomain(domain expression.FieldDomain, sourceType string) *FieldsOutput {
	registry := expression.DefaultRegistry()
	fields := registry.ListByDomain(domain)

	resp := &FieldsOutput{
		Body: make([]FieldResponse, 0, len(fields)),
	}

	for _, field := range fields {
		resp.Body = append(resp.Body, FieldResponse{
			Name:        field.Name,
			Type:        string(field.Type),
			Description: field.Description,
			Aliases:     field.Aliases,
			ReadOnly:    field.ReadOnly,
			SourceType:  sourceType,
		})
	}

	return resp
}

// GetFilterFieldsStream returns fields available for stream filtering.
func (h *ExpressionHandler) GetFilterFieldsStream(ctx context.Context, input *struct{}) (*FieldsOutput, error) {
	return h.getFieldsForDomain(expression.DomainStream, "stream"), nil
}

// GetFilterFieldsEPG returns fields available for EPG filtering.
func (h *ExpressionHandler) GetFilterFieldsEPG(ctx context.Context, input *struct{}) (*FieldsOutput, error) {
	return h.getFieldsForDomain(expression.DomainEPG, "epg"), nil
}

// GetDataMappingFieldsStream returns fields available for stream data mapping.
func (h *ExpressionHandler) GetDataMappingFieldsStream(ctx context.Context, input *struct{}) (*FieldsOutput, error) {
	return h.getFieldsForDomain(expression.DomainStream, "stream"), nil
}

// GetDataMappingFieldsEPG returns fields available for EPG data mapping.
func (h *ExpressionHandler) GetDataMappingFieldsEPG(ctx context.Context, input *struct{}) (*FieldsOutput, error) {
	return h.getFieldsForDomain(expression.DomainEPG, "epg"), nil
}
