package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/expression"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
)

// ExpressionHandler handles expression-related API endpoints.
type ExpressionHandler struct {
	validator      *expression.Validator
	channelRepo    repository.ChannelRepository
	epgProgramRepo repository.EpgProgramRepository
}

// NewExpressionHandler creates a new expression handler.
func NewExpressionHandler(channelRepo repository.ChannelRepository, epgProgramRepo repository.EpgProgramRepository) *ExpressionHandler {
	return &ExpressionHandler{
		validator:      expression.NewValidator(nil), // Uses global registry
		channelRepo:    channelRepo,
		epgProgramRepo: epgProgramRepo,
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
		OperationID: "getDataMappingHelpers",
		Method:      "GET",
		Path:        "/api/v1/data-mapping/helpers",
		Summary:     "Get available data mapping helpers",
		Description: "Returns all helper functions available for data mapping expressions",
		Tags:        []string{"Expressions"},
	}, h.GetDataMappingHelpers)

	huma.Register(api, huma.Operation{
		OperationID: "getClientDetectionFields",
		Method:      "GET",
		Path:        "/api/v1/client-detection/fields",
		Summary:     "Get available client detection fields",
		Description: "Returns all fields available for client detection expressions (user_agent, client_ip, etc.)",
		Tags:        []string{"Expressions"},
	}, h.GetClientDetectionFields)

	huma.Register(api, huma.Operation{
		OperationID: "autocompleteChannelValues",
		Method:      "GET",
		Path:        "/api/v1/autocomplete/channel-values",
		Summary:     "Autocomplete channel field values",
		Description: "Returns distinct values for a channel field with occurrence counts, filtered by search query. Useful for expression editor autocomplete.",
		Tags:        []string{"Expressions"},
	}, h.AutocompleteChannelValues)

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

	// Register test endpoints
	huma.Register(api, huma.Operation{
		OperationID: "testFilterExpression",
		Method:      "POST",
		Path:        "/api/v1/filters/test",
		Summary:     "Test filter expression",
		Description: "Tests a filter expression against channels/programs from a source and returns match statistics",
		Tags:        []string{"Expressions"},
	}, h.TestFilterExpression)

	huma.Register(api, huma.Operation{
		OperationID: "testDataMappingExpression",
		Method:      "POST",
		Path:        "/api/v1/data-mapping/test",
		Summary:     "Test data mapping expression",
		Description: "Tests a data mapping expression against channels/programs from sources and returns affected count",
		Tags:        []string{"Expressions"},
	}, h.TestDataMappingExpression)
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
	IsValid             bool                        `json:"is_valid" doc:"Whether the expression is valid"`
	CanonicalExpression string                      `json:"canonical_expression,omitempty" doc:"The canonical form of the expression (if valid)"`
	Errors              []ExpressionValidationError `json:"errors" doc:"List of validation errors (if invalid)"`
	ExpressionTree      map[string]any              `json:"expression_tree,omitempty" doc:"JSON representation of the parsed expression tree (if valid)"`
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

// HelperCompletionOption represents a static completion option.
type HelperCompletionOption struct {
	Label       string `json:"label" doc:"Display label for the option"`
	Value       string `json:"value" doc:"Value to insert when selected"`
	Description string `json:"description,omitempty" doc:"Description of the option"`
}

// HelperCompletion represents the completion configuration for a helper.
type HelperCompletion struct {
	Type         string                   `json:"type" doc:"Completion type: search, static, or function"`
	Endpoint     string                   `json:"endpoint,omitempty" doc:"API endpoint for search completion"`
	QueryParam   string                   `json:"query_param,omitempty" doc:"Query parameter name for search"`
	DisplayField string                   `json:"display_field,omitempty" doc:"Field to display from search results"`
	ValueField   string                   `json:"value_field,omitempty" doc:"Field to use as value from search results"`
	PreviewField string                   `json:"preview_field,omitempty" doc:"Field to use for preview (e.g., image URL)"`
	MinChars     int                      `json:"min_chars,omitempty" doc:"Minimum characters before triggering search"`
	DebounceMs   int                      `json:"debounce_ms,omitempty" doc:"Debounce time in milliseconds"`
	MaxResults   int                      `json:"max_results,omitempty" doc:"Maximum number of results to return"`
	Placeholder  string                   `json:"placeholder,omitempty" doc:"Placeholder text for input"`
	EmptyMessage string                   `json:"empty_message,omitempty" doc:"Message when no results found"`
	Options      []HelperCompletionOption `json:"options,omitempty" doc:"Static options for static completion type"`
}

// HelperResponse represents a helper in the API response.
type HelperResponse struct {
	Name        string            `json:"name" doc:"Helper name (e.g., time, logo)"`
	Prefix      string            `json:"prefix" doc:"Helper prefix (e.g., @time:)"`
	Description string            `json:"description" doc:"Description of the helper"`
	Example     string            `json:"example" doc:"Example usage of the helper"`
	Completion  *HelperCompletion `json:"completion,omitempty" doc:"Completion configuration for autocomplete"`
}

// HelpersOutput is the output for the helpers listing endpoint.
type HelpersOutput struct {
	Body struct {
		Helpers []HelperResponse `json:"helpers" doc:"List of available helpers"`
	}
}

// GetDataMappingHelpers returns available helper functions for data mapping expressions.
func (h *ExpressionHandler) GetDataMappingHelpers(ctx context.Context, input *struct{}) (*HelpersOutput, error) {
	helpers := []HelperResponse{
		{
			Name:        "time",
			Prefix:      "@time:",
			Description: "Time-related operations for date/time manipulation",
			Example:     "@time:now",
			Completion: &HelperCompletion{
				Type: "static",
				Options: []HelperCompletionOption{
					{
						Label:       "now",
						Value:       "now",
						Description: "Current time in RFC3339 format",
					},
					{
						Label:       "parse",
						Value:       "parse",
						Description: "Parse a time string (input|format)",
					},
					{
						Label:       "format",
						Value:       "format",
						Description: "Format a time (input|output_format)",
					},
					{
						Label:       "add",
						Value:       "add",
						Description: "Add duration to time (base_time|duration)",
					},
				},
			},
		},
		{
			Name:        "logo",
			Prefix:      "@logo:",
			Description: "Logo lookup by ULID - resolves to logo URL (uploaded logos only)",
			Example:     "@logo:01ARZ3NDEKTSV4RRFFQ69G5FAV",
			Completion: &HelperCompletion{
				Type:         "search",
				Endpoint:     "/api/v1/logos?include_cached=false",
				QueryParam:   "search",
				DisplayField: "name",
				ValueField:   "id",
				PreviewField: "url",
				MinChars:     2,
				DebounceMs:   300,
				MaxResults:   10,
				Placeholder:  "Search logos...",
				EmptyMessage: "No logos found",
			},
		},
		{
			Name:        "group",
			Prefix:      "@group:",
			Description: "Autocomplete group_title values from your channel data",
			Example:     "@group:Sports → \"Sports\"",
			Completion: &HelperCompletion{
				Type:         "search",
				Endpoint:     "/api/v1/autocomplete/channel-values?field=group_title&quote=true",
				QueryParam:   "q",
				DisplayField: "value",
				ValueField:   "value",
				MinChars:     1,
				DebounceMs:   200,
				MaxResults:   20,
				Placeholder:  "Search groups...",
				EmptyMessage: "No groups found",
			},
		},
		{
			Name:        "channel",
			Prefix:      "@channel:",
			Description: "Autocomplete channel_name values from your channel data",
			Example:     "@channel:ESPN → \"ESPN HD\"",
			Completion: &HelperCompletion{
				Type:         "search",
				Endpoint:     "/api/v1/autocomplete/channel-values?field=channel_name&quote=true",
				QueryParam:   "q",
				DisplayField: "value",
				ValueField:   "value",
				MinChars:     2,
				DebounceMs:   200,
				MaxResults:   20,
				Placeholder:  "Search channels...",
				EmptyMessage: "No channels found",
			},
		},
	}

	resp := &HelpersOutput{}
	resp.Body.Helpers = helpers
	return resp, nil
}

// GetClientDetectionFields returns fields available for client detection expressions.
func (h *ExpressionHandler) GetClientDetectionFields(ctx context.Context, input *struct{}) (*FieldsOutput, error) {
	// Client detection fields are HTTP request properties
	fields := []FieldResponse{
		{Name: "user_agent", Type: "string", Description: "HTTP User-Agent header", SourceType: "client"},
		{Name: "client_ip", Type: "string", Description: "Client IP address", SourceType: "client"},
		{Name: "request_path", Type: "string", Description: "Request URL path", SourceType: "client"},
		{Name: "request_url", Type: "string", Description: "Full request URL", SourceType: "client"},
		{Name: "query_params", Type: "string", Description: "Raw query string", SourceType: "client"},
		{Name: "x_forwarded_for", Type: "string", Description: "X-Forwarded-For header", SourceType: "client"},
		{Name: "x_real_ip", Type: "string", Description: "X-Real-IP header", SourceType: "client"},
		{Name: "accept", Type: "string", Description: "Accept header", SourceType: "client"},
		{Name: "accept_language", Type: "string", Description: "Accept-Language header", SourceType: "client"},
		{Name: "host", Type: "string", Description: "Host header", SourceType: "client"},
		{Name: "referer", Type: "string", Description: "Referer header", SourceType: "client"},
	}

	return &FieldsOutput{Body: fields}, nil
}

// AutocompleteChannelValuesInput is the input for autocompleting channel field values.
type AutocompleteChannelValuesInput struct {
	Field string `query:"field" doc:"Field name to get values for (group_title, channel_name, tvg_id, tvg_name, country, language)" required:"true"`
	Query string `query:"q" doc:"Search query to filter values (case-insensitive contains)" required:"false"`
	Limit int    `query:"limit" doc:"Maximum number of results to return (default: 20, max: 100)" required:"false"`
	Quote bool   `query:"quote" doc:"If true, wrap values in double quotes for expression insertion" required:"false"`
}

// AutocompleteValueResponse represents a single autocomplete suggestion.
type AutocompleteValueResponse struct {
	Value       string `json:"value" doc:"The field value"`
	Count       int64  `json:"count" doc:"Number of channels with this value"`
	Description string `json:"description,omitempty" doc:"Description (shows count)"`
}

// AutocompleteChannelValuesOutput is the output for channel value autocomplete.
type AutocompleteChannelValuesOutput struct {
	Body []AutocompleteValueResponse
}

// AutocompleteChannelValues returns distinct values for a channel field with occurrence counts.
func (h *ExpressionHandler) AutocompleteChannelValues(ctx context.Context, input *AutocompleteChannelValuesInput) (*AutocompleteChannelValuesOutput, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}

	results, err := h.channelRepo.GetDistinctFieldValues(ctx, input.Field, input.Query, limit)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	resp := &AutocompleteChannelValuesOutput{
		Body: make([]AutocompleteValueResponse, 0, len(results)),
	}

	for _, r := range results {
		value := r.Value
		if input.Quote {
			// Escape any existing quotes in the value and wrap in double quotes
			value = `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
		}
		resp.Body = append(resp.Body, AutocompleteValueResponse{
			Value:       value,
			Count:       r.Count,
			Description: fmt.Sprintf("%d channels", r.Count),
		})
	}

	return resp, nil
}

// TestFilterExpressionInput is the input for testing a filter expression.
type TestFilterExpressionInput struct {
	Body struct {
		SourceID         string `json:"source_id" doc:"Source ID to test against" minLength:"1"`
		SourceType       string `json:"source_type" doc:"Source type (stream or epg)" enum:"stream,epg"`
		FilterExpression string `json:"filter_expression" doc:"Filter expression to test" minLength:"1"`
		IsInverse        bool   `json:"is_inverse" doc:"Whether to invert the match result"`
	}
}

// TestFilterExpressionOutput is the output for testing a filter expression.
type TestFilterExpressionOutput struct {
	Body struct {
		Success       bool   `json:"success" doc:"Whether the test completed successfully"`
		MatchedCount  int    `json:"matched_count" doc:"Number of records that matched the expression"`
		TotalChannels int    `json:"total_channels" doc:"Total number of records tested"`
		Error         string `json:"error,omitempty" doc:"Error message if test failed"`
	}
}

// TestFilterExpression tests a filter expression against a source.
func (h *ExpressionHandler) TestFilterExpression(ctx context.Context, input *TestFilterExpressionInput) (*TestFilterExpressionOutput, error) {
	resp := &TestFilterExpressionOutput{}

	// Parse source ID
	sourceID, err := models.ParseULID(input.Body.SourceID)
	if err != nil {
		resp.Body.Error = "invalid source_id format"
		return resp, nil
	}

	// Parse the expression
	parsed, err := expression.PreprocessAndParse(input.Body.FilterExpression)
	if err != nil {
		resp.Body.Error = "invalid expression: " + err.Error()
		return resp, nil
	}

	// Create evaluator
	evaluator := expression.NewEvaluator()
	evaluator.SetCaseSensitive(false)

	var matchCount, totalCount int

	if input.Body.SourceType == "stream" {
		// Test against channels
		err = h.channelRepo.GetBySourceID(ctx, sourceID, func(ch *models.Channel) error {
			totalCount++

			// Create evaluation context
			fields := map[string]string{
				"channel_name": ch.ChannelName,
				"tvg_id":       ch.TvgID,
				"tvg_name":     ch.TvgName,
				"tvg_logo":     ch.TvgLogo,
				"group_title":  ch.GroupTitle,
				"stream_url":   ch.StreamURL,
			}
			evalCtx := expression.NewChannelEvalContext(fields)

			// Evaluate expression
			result, evalErr := evaluator.Evaluate(parsed, evalCtx)
			if evalErr != nil {
				// Skip on error
				return nil
			}

			matches := result.Matches
			if input.Body.IsInverse {
				matches = !matches
			}

			if matches {
				matchCount++
			}
			return nil
		})
		if err != nil {
			resp.Body.Error = "failed to read channels: " + err.Error()
			return resp, nil
		}
	} else {
		// Test against EPG programs
		err = h.epgProgramRepo.GetBySourceID(ctx, sourceID, func(prog *models.EpgProgram) error {
			totalCount++

			// Create evaluation context
			fields := map[string]string{
				"programme_title":       prog.Title,
				"programme_description": prog.Description,
				"programme_category":    prog.Category,
			}
			if !prog.Start.IsZero() {
				fields["programme_start"] = prog.Start.Format("2006-01-02T15:04:05Z07:00")
			}
			if !prog.Stop.IsZero() {
				fields["programme_stop"] = prog.Stop.Format("2006-01-02T15:04:05Z07:00")
			}
			evalCtx := expression.NewProgramEvalContext(fields)

			// Evaluate expression
			result, evalErr := evaluator.Evaluate(parsed, evalCtx)
			if evalErr != nil {
				// Skip on error
				return nil
			}

			matches := result.Matches
			if input.Body.IsInverse {
				matches = !matches
			}

			if matches {
				matchCount++
			}
			return nil
		})
		if err != nil {
			resp.Body.Error = "failed to read programs: " + err.Error()
			return resp, nil
		}
	}

	resp.Body.Success = true
	resp.Body.MatchedCount = matchCount
	resp.Body.TotalChannels = totalCount

	return resp, nil
}

// TestDataMappingExpressionInput is the input for testing a data mapping expression.
type TestDataMappingExpressionInput struct {
	Body struct {
		SourceIDs  []string `json:"source_ids" doc:"Source IDs to test against" minItems:"1"`
		SourceType string   `json:"source_type" doc:"Source type (stream or epg)" enum:"stream,epg"`
		Expression string   `json:"expression" doc:"Data mapping expression to test" minLength:"1"`
	}
}

// TestDataMappingExpressionOutput is the output for testing a data mapping expression.
type TestDataMappingExpressionOutput struct {
	Body struct {
		Success          bool   `json:"success" doc:"Whether the test completed successfully"`
		Message          string `json:"message,omitempty" doc:"Result message"`
		AffectedChannels int    `json:"affected_channels" doc:"Number of records that would be affected"`
		TotalChannels    int    `json:"total_channels" doc:"Total number of records tested"`
	}
}

// TestDataMappingExpression tests a data mapping expression against sources.
func (h *ExpressionHandler) TestDataMappingExpression(ctx context.Context, input *TestDataMappingExpressionInput) (*TestDataMappingExpressionOutput, error) {
	resp := &TestDataMappingExpressionOutput{}

	// Parse the expression
	parsed, err := expression.PreprocessAndParse(input.Body.Expression)
	if err != nil {
		resp.Body.Message = "invalid expression: " + err.Error()
		return resp, nil
	}

	// Create evaluator
	evaluator := expression.NewEvaluator()
	evaluator.SetCaseSensitive(false)

	var affectedCount, totalCount int

	for _, sourceIDStr := range input.Body.SourceIDs {
		sourceID, err := models.ParseULID(sourceIDStr)
		if err != nil {
			continue // Skip invalid source IDs
		}

		if input.Body.SourceType == "stream" {
			// Test against channels
			err = h.channelRepo.GetBySourceID(ctx, sourceID, func(ch *models.Channel) error {
				totalCount++

				// Create evaluation context
				fields := map[string]string{
					"channel_name": ch.ChannelName,
					"tvg_id":       ch.TvgID,
					"tvg_name":     ch.TvgName,
					"tvg_logo":     ch.TvgLogo,
					"group_title":  ch.GroupTitle,
					"stream_url":   ch.StreamURL,
				}
				evalCtx := expression.NewChannelEvalContext(fields)

				// Evaluate expression - for data mapping, any match means it would be affected
				result, evalErr := evaluator.Evaluate(parsed, evalCtx)
				if evalErr != nil {
					return nil
				}

				if result.Matches {
					affectedCount++
				}
				return nil
			})
			if err != nil {
				// Continue with other sources
				continue
			}
		} else {
			// Test against EPG programs
			err = h.epgProgramRepo.GetBySourceID(ctx, sourceID, func(prog *models.EpgProgram) error {
				totalCount++

				// Create evaluation context
				fields := map[string]string{
					"programme_title":       prog.Title,
					"programme_description": prog.Description,
					"programme_category":    prog.Category,
				}
				if !prog.Start.IsZero() {
					fields["programme_start"] = prog.Start.Format("2006-01-02T15:04:05Z07:00")
				}
				if !prog.Stop.IsZero() {
					fields["programme_stop"] = prog.Stop.Format("2006-01-02T15:04:05Z07:00")
				}
				evalCtx := expression.NewProgramEvalContext(fields)

				// Evaluate expression
				result, evalErr := evaluator.Evaluate(parsed, evalCtx)
				if evalErr != nil {
					return nil
				}

				if result.Matches {
					affectedCount++
				}
				return nil
			})
			if err != nil {
				continue
			}
		}
	}

	resp.Body.Success = true
	resp.Body.AffectedChannels = affectedCount
	resp.Body.TotalChannels = totalCount
	if totalCount > 0 {
		resp.Body.Message = fmt.Sprintf("Expression would affect %d of %d records", affectedCount, totalCount)
	}

	return resp, nil
}
