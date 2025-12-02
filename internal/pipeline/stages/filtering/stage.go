// Package filtering implements the channel/program filtering pipeline stage.
package filtering

import (
	"context"
	"fmt"
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
	expressionFilters         []ExpressionFilter
	compiledExpressionFilters []*compiledExpressionFilter
}

// New creates a new filtering stage.
func New() *Stage {
	return &Stage{
		BaseStage:                 shared.NewBaseStage(StageID, StageName),
		expressionFilters:         make([]ExpressionFilter, 0),
		compiledExpressionFilters: make([]*compiledExpressionFilter, 0),
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
	if len(s.expressionFilters) == 0 {
		result.Message = "No filters configured"
		return result, nil
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

// shouldIncludeChannel checks if a channel passes all expression filters.
func (s *Stage) shouldIncludeChannel(ch *models.Channel) bool {
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

// shouldIncludeProgram checks if a program passes all expression filters.
func (s *Stage) shouldIncludeProgram(prog *models.EpgProgram) bool {
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

// Ensure Stage implements core.Stage.
var _ core.Stage = (*Stage)(nil)
