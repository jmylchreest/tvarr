// Package filtering implements the channel/program filtering pipeline stage.
package filtering

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jmylchreest/tvarr/internal/expression"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/jmylchreest/tvarr/internal/pipeline/shared"
)

const (
	// StageID is the unique identifier for this stage.
	StageID = "filtering"
	// StageName is the human-readable name for this stage.
	StageName = "Filtering"
)

// FilterTarget specifies what the filter applies to.
type FilterTarget string

const (
	FilterTargetChannel FilterTarget = "channel"
	FilterTargetProgram FilterTarget = "program"
)

// FilterAction specifies the filter behavior.
type FilterAction string

const (
	FilterActionInclude FilterAction = "include"
	FilterActionExclude FilterAction = "exclude"
)

// FilterMatchType specifies how to match the pattern.
type FilterMatchType string

const (
	FilterMatchTypeExact    FilterMatchType = "exact"
	FilterMatchTypeContains FilterMatchType = "contains"
	FilterMatchTypePrefix   FilterMatchType = "prefix"
	FilterMatchTypeSuffix   FilterMatchType = "suffix"
	FilterMatchTypeRegex    FilterMatchType = "regex"
)

// Filter represents a legacy filter rule configuration.
// Deprecated: Use ExpressionFilter for new filters.
type Filter struct {
	Enabled   bool            `json:"enabled"`
	Target    FilterTarget    `json:"target"`
	Action    FilterAction    `json:"action"`
	Field     string          `json:"field"`
	MatchType FilterMatchType `json:"match_type"`
	Pattern   string          `json:"pattern"`
}

// ExpressionFilter represents an expression-based filter rule.
type ExpressionFilter struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	Enabled    bool         `json:"enabled"`
	Target     FilterTarget `json:"target"`
	Action     FilterAction `json:"action"`
	Expression string       `json:"expression"`
}

// compiledExpressionFilter holds a pre-parsed expression filter.
type compiledExpressionFilter struct {
	filter    *ExpressionFilter
	parsed    *expression.ParsedExpression
	evaluator *expression.Evaluator
}

// Stage applies filter rules to channels and programs.
type Stage struct {
	shared.BaseStage
	filters                    []Filter
	compiledFilters            []*compiledFilter
	expressionFilters          []ExpressionFilter
	compiledExpressionFilters  []*compiledExpressionFilter
}

// compiledFilter is a pre-compiled filter for performance.
type compiledFilter struct {
	filter *Filter
	regex  *regexp.Regexp
}

// New creates a new filtering stage.
func New() *Stage {
	return &Stage{
		BaseStage:                  shared.NewBaseStage(StageID, StageName),
		filters:                    make([]Filter, 0),
		compiledFilters:            make([]*compiledFilter, 0),
		expressionFilters:          make([]ExpressionFilter, 0),
		compiledExpressionFilters:  make([]*compiledExpressionFilter, 0),
	}
}

// NewConstructor returns a stage constructor for use with the factory.
// If a FilterRepository is provided in dependencies, it loads enabled filters from the database.
func NewConstructor() core.StageConstructor {
	return func(deps *core.Dependencies) core.Stage {
		stage := New()

		// Load filters from database if repository is available
		if deps.FilterRepo != nil {
			filters, err := deps.FilterRepo.GetEnabled(context.Background())
			if err != nil {
				if deps.Logger != nil {
					deps.Logger.Warn("failed to load filters from database", "error", err)
				}
			} else {
				expressionFilters := make([]ExpressionFilter, 0, len(filters))
				for _, f := range filters {
					var target FilterTarget
					switch f.SourceType {
					case models.FilterSourceTypeStream:
						target = FilterTargetChannel
					case models.FilterSourceTypeEPG:
						target = FilterTargetProgram
					default:
						continue
					}

					var action FilterAction
					switch f.Action {
					case models.FilterActionInclude:
						action = FilterActionInclude
					case models.FilterActionExclude:
						action = FilterActionExclude
					default:
						continue
					}

					expressionFilters = append(expressionFilters, ExpressionFilter{
						ID:         f.ID.String(),
						Name:       f.Name,
						Enabled:    f.IsEnabled,
						Target:     target,
						Action:     action,
						Expression: f.Expression,
					})
				}
				stage.WithExpressionFilters(expressionFilters)
			}
		}

		return stage
	}
}

// WithFilters sets the legacy filters for the stage.
// Deprecated: Use WithExpressionFilters for new filters.
func (s *Stage) WithFilters(filters []Filter) *Stage {
	s.filters = filters
	return s
}

// AddFilter adds a legacy filter to the stage.
// Deprecated: Use AddExpressionFilter for new filters.
func (s *Stage) AddFilter(filter Filter) *Stage {
	s.filters = append(s.filters, filter)
	return s
}

// WithExpressionFilters sets the expression-based filters for the stage.
func (s *Stage) WithExpressionFilters(filters []ExpressionFilter) *Stage {
	s.expressionFilters = filters
	return s
}

// AddExpressionFilter adds an expression-based filter to the stage.
func (s *Stage) AddExpressionFilter(filter ExpressionFilter) *Stage {
	s.expressionFilters = append(s.expressionFilters, filter)
	return s
}

// Execute applies filters to channels and programs.
func (s *Stage) Execute(ctx context.Context, state *core.State) (*core.StageResult, error) {
	result := shared.NewResult()

	// Use configured filters, or skip if none
	if len(s.filters) == 0 && len(s.expressionFilters) == 0 {
		result.Message = "No filters configured"
		return result, nil
	}

	// Compile legacy filters
	if err := s.compileFilters(s.filters); err != nil {
		return result, fmt.Errorf("compiling legacy filters: %w", err)
	}

	// Compile expression filters
	if err := s.compileExpressionFilters(); err != nil {
		return result, fmt.Errorf("compiling expression filters: %w", err)
	}

	originalChannelCount := len(state.Channels)
	originalProgramCount := len(state.Programs)

	// Filter channels
	filteredChannels := make([]*models.Channel, 0, len(state.Channels))
	filteredChannelIDs := make(map[string]bool)

	for _, ch := range state.Channels {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		if s.shouldIncludeChannel(ch) {
			filteredChannels = append(filteredChannels, ch)
			if ch.TvgID != "" {
				filteredChannelIDs[ch.TvgID] = true
			}
		}
	}

	state.Channels = filteredChannels

	// Update channel map
	newChannelMap := make(map[string]*models.Channel)
	for tvgID, ch := range state.ChannelMap {
		if filteredChannelIDs[tvgID] {
			newChannelMap[tvgID] = ch
		}
	}
	state.ChannelMap = newChannelMap

	// Filter programs (only keep programs for included channels)
	filteredPrograms := make([]*models.EpgProgram, 0, len(state.Programs))
	for _, prog := range state.Programs {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		if filteredChannelIDs[prog.ChannelID] && s.shouldIncludeProgram(prog) {
			filteredPrograms = append(filteredPrograms, prog)
		}
	}

	state.Programs = filteredPrograms

	channelsRemoved := originalChannelCount - len(state.Channels)
	programsRemoved := originalProgramCount - len(state.Programs)

	result.RecordsProcessed = originalChannelCount + originalProgramCount
	result.RecordsModified = channelsRemoved + programsRemoved
	result.Message = fmt.Sprintf("Filtered: %d/%d channels, %d/%d programs removed",
		channelsRemoved, originalChannelCount,
		programsRemoved, originalProgramCount)

	// Create artifact
	artifact := core.NewArtifact(core.ArtifactTypeChannels, core.ProcessingStageFiltered, StageID).
		WithRecordCount(len(state.Channels)).
		WithMetadata("channels_removed", channelsRemoved).
		WithMetadata("programs_removed", programsRemoved)
	result.Artifacts = append(result.Artifacts, artifact)

	return result, nil
}

// compileFilters pre-compiles regex patterns for performance.
func (s *Stage) compileFilters(filters []Filter) error {
	s.compiledFilters = make([]*compiledFilter, 0, len(filters))

	for i := range filters {
		filter := &filters[i]
		if !filter.Enabled {
			continue
		}

		cf := &compiledFilter{filter: filter}

		// Compile regex if using regex match type
		if filter.MatchType == FilterMatchTypeRegex && filter.Pattern != "" {
			re, err := regexp.Compile(filter.Pattern)
			if err != nil {
				return fmt.Errorf("invalid regex pattern %q: %w", filter.Pattern, err)
			}
			cf.regex = re
		}

		s.compiledFilters = append(s.compiledFilters, cf)
	}

	return nil
}

// compileExpressionFilters pre-parses expression filters.
func (s *Stage) compileExpressionFilters() error {
	s.compiledExpressionFilters = make([]*compiledExpressionFilter, 0, len(s.expressionFilters))

	for i := range s.expressionFilters {
		filter := &s.expressionFilters[i]
		if !filter.Enabled {
			continue
		}

		// Skip empty expressions
		if strings.TrimSpace(filter.Expression) == "" {
			continue
		}

		// Preprocess and parse the expression
		parsed, err := expression.PreprocessAndParse(filter.Expression)
		if err != nil {
			return fmt.Errorf("parsing expression filter %s: %w", filter.ID, err)
		}

		if parsed == nil {
			continue
		}

		cef := &compiledExpressionFilter{
			filter:    filter,
			parsed:    parsed,
			evaluator: expression.NewEvaluator(),
		}

		s.compiledExpressionFilters = append(s.compiledExpressionFilters, cef)
	}

	return nil
}

// shouldIncludeChannel checks if a channel passes all filters.
func (s *Stage) shouldIncludeChannel(ch *models.Channel) bool {
	// Check legacy filters first
	for _, cf := range s.compiledFilters {
		if cf.filter.Target != FilterTargetChannel {
			continue
		}

		matches := s.matchesFilter(cf, s.getChannelFieldValue(ch, cf.filter.Field))

		// If action is exclude and matches, exclude the channel
		if cf.filter.Action == FilterActionExclude && matches {
			return false
		}

		// If action is include and doesn't match, exclude the channel
		if cf.filter.Action == FilterActionInclude && !matches {
			return false
		}
	}

	// Check expression filters
	for _, cef := range s.compiledExpressionFilters {
		if cef.filter.Target != FilterTargetChannel {
			continue
		}

		// Create evaluation context for the channel
		ctx := s.createChannelEvalContext(ch)

		// Evaluate the expression
		evalResult, err := cef.evaluator.Evaluate(cef.parsed, ctx)
		if err != nil {
			// Log error but treat as non-match
			continue
		}

		matches := evalResult.Matches

		// If action is exclude and matches, exclude the channel
		if cef.filter.Action == FilterActionExclude && matches {
			return false
		}

		// If action is include and doesn't match, exclude the channel
		if cef.filter.Action == FilterActionInclude && !matches {
			return false
		}
	}

	return true
}

// shouldIncludeProgram checks if a program passes all filters.
func (s *Stage) shouldIncludeProgram(prog *models.EpgProgram) bool {
	// Check legacy filters first
	for _, cf := range s.compiledFilters {
		if cf.filter.Target != FilterTargetProgram {
			continue
		}

		matches := s.matchesFilter(cf, s.getProgramFieldValue(prog, cf.filter.Field))

		if cf.filter.Action == FilterActionExclude && matches {
			return false
		}

		if cf.filter.Action == FilterActionInclude && !matches {
			return false
		}
	}

	// Check expression filters
	for _, cef := range s.compiledExpressionFilters {
		if cef.filter.Target != FilterTargetProgram {
			continue
		}

		// Create evaluation context for the program
		ctx := s.createProgramEvalContext(prog)

		// Evaluate the expression
		evalResult, err := cef.evaluator.Evaluate(cef.parsed, ctx)
		if err != nil {
			// Log error but treat as non-match
			continue
		}

		matches := evalResult.Matches

		// If action is exclude and matches, exclude the program
		if cef.filter.Action == FilterActionExclude && matches {
			return false
		}

		// If action is include and doesn't match, exclude the program
		if cef.filter.Action == FilterActionInclude && !matches {
			return false
		}
	}

	return true
}

// createChannelEvalContext creates an evaluation context for a channel.
func (s *Stage) createChannelEvalContext(ch *models.Channel) expression.FieldValueAccessor {
	fields := map[string]string{
		"channel_name": ch.ChannelName,
		"tvg_id":       ch.TvgID,
		"tvg_name":     ch.TvgName,
		"tvg_logo":     ch.TvgLogo,
		"group_title":  ch.GroupTitle,
		"stream_url":   ch.StreamURL,
	}

	return expression.NewChannelEvalContext(fields)
}

// createProgramEvalContext creates an evaluation context for a program.
func (s *Stage) createProgramEvalContext(prog *models.EpgProgram) expression.FieldValueAccessor {
	fields := map[string]string{
		"programme_title":       prog.Title,
		"programme_description": prog.Description,
		"programme_category":    prog.Category,
	}

	// Add time fields as strings
	if !prog.Start.IsZero() {
		fields["programme_start"] = prog.Start.Format("2006-01-02T15:04:05Z07:00")
	}
	if !prog.Stop.IsZero() {
		fields["programme_stop"] = prog.Stop.Format("2006-01-02T15:04:05Z07:00")
	}

	return expression.NewChannelEvalContext(fields)
}

// matchesFilter checks if a value matches a filter.
func (s *Stage) matchesFilter(cf *compiledFilter, value string) bool {
	pattern := cf.filter.Pattern

	switch cf.filter.MatchType {
	case FilterMatchTypeExact:
		return value == pattern

	case FilterMatchTypeContains:
		return strings.Contains(strings.ToLower(value), strings.ToLower(pattern))

	case FilterMatchTypePrefix:
		return strings.HasPrefix(strings.ToLower(value), strings.ToLower(pattern))

	case FilterMatchTypeSuffix:
		return strings.HasSuffix(strings.ToLower(value), strings.ToLower(pattern))

	case FilterMatchTypeRegex:
		if cf.regex != nil {
			return cf.regex.MatchString(value)
		}
		return false

	default:
		return false
	}
}

// getChannelFieldValue returns the value of a channel field for filtering.
func (s *Stage) getChannelFieldValue(ch *models.Channel, field string) string {
	switch field {
	case "name", "channel_name":
		return ch.ChannelName
	case "tvg_id":
		return ch.TvgID
	case "tvg_name":
		return ch.TvgName
	case "group", "group_title":
		return ch.GroupTitle
	case "url", "stream_url":
		return ch.StreamURL
	default:
		return ""
	}
}

// getProgramFieldValue returns the value of a program field for filtering.
func (s *Stage) getProgramFieldValue(prog *models.EpgProgram, field string) string {
	switch field {
	case "title":
		return prog.Title
	case "description":
		return prog.Description
	case "category":
		return prog.Category
	case "channel_id":
		return prog.ChannelID
	default:
		return ""
	}
}

// Ensure Stage implements core.Stage.
var _ core.Stage = (*Stage)(nil)
