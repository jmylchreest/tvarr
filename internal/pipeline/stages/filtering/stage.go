// Package filtering implements the channel/program filtering pipeline stage.
package filtering

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
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
	logger                    *slog.Logger
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
// Filters are loaded from the proxy's associated filters at execution time (not from global repository).
func NewConstructor() core.StageConstructor {
	return func(deps *core.Dependencies) core.Stage {
		stage := New()

		// Inject logger
		if deps.Logger != nil {
			stage.logger = deps.Logger.With("stage", StageID)
		}

		// Note: Filters are loaded from state.Proxy.Filters in Execute(), not here.
		// This ensures we use only the filters assigned to this specific proxy.

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
// Filtering logic:
//   - Output starts empty
//   - Include filters add matching channels from SOURCE to output (appending)
//   - Exclude filters remove matching channels from OUTPUT
//   - Filters apply in priority order (lower priority number = first)
//   - If no filters are assigned to the proxy, all channels pass through
func (s *Stage) Execute(ctx context.Context, state *core.State) (*core.StageResult, error) {
	result := shared.NewResult()

	// Load filters from proxy's assigned filters (not global filters)
	if err := s.loadFiltersFromProxy(ctx, state.Proxy); err != nil {
		s.log(ctx, slog.LevelError, "failed to load filters from proxy",
			slog.String("error", err.Error()))
		return result, fmt.Errorf("loading filters from proxy: %w", err)
	}

	// If no filters assigned to proxy, pass through all channels
	if len(s.expressionFilters) == 0 {
		s.log(ctx, slog.LevelInfo, "no filters assigned to proxy, passing all channels through")
		result.Message = "No filters assigned to proxy"
		return result, nil
	}

	// T031: Log filter stats at start
	s.log(ctx, slog.LevelInfo, "starting filtering",
		slog.Int("filter_count", len(s.expressionFilters)),
		slog.Int("input_channels", len(state.Channels)),
		slog.Int("input_programs", len(state.Programs)))

	// Compile expression filters
	if err := s.compileExpressionFilters(); err != nil {
		s.log(ctx, slog.LevelError, "failed to compile expression filters",
			slog.String("error", err.Error()))
		return result, fmt.Errorf("compiling expression filters: %w", err)
	}

	originalChannelCount := len(state.Channels)
	originalProgramCount := len(state.Programs)

	// Apply channel filters using the sequential logic
	filteredChannels, err := s.applyChannelFilters(ctx, state.Channels)
	if err != nil {
		return result, err
	}

	// Build set of included channel IDs for program filtering
	filteredChannelIDs := make(map[string]bool)
	for _, ch := range filteredChannels {
		if ch.TvgID != "" {
			filteredChannelIDs[ch.TvgID] = true
		}
	}

	// Replace channels in state with filtered result
	state.Channels = filteredChannels

	// Update channel map
	newChannelMap := make(map[string]*models.Channel)
	for tvgID, ch := range state.ChannelMap {
		if filteredChannelIDs[tvgID] {
			newChannelMap[tvgID] = ch
		}
	}
	state.ChannelMap = newChannelMap

	// Apply program filters (only to programs for included channels)
	filteredPrograms, err := s.applyProgramFilters(ctx, state.Programs, filteredChannelIDs)
	if err != nil {
		return result, err
	}

	// Replace programs in state with filtered result
	state.Programs = filteredPrograms

	channelsKept := len(filteredChannels)
	programsKept := len(filteredPrograms)
	channelsRemoved := originalChannelCount - channelsKept
	programsRemoved := originalProgramCount - programsKept

	result.RecordsProcessed = originalChannelCount + originalProgramCount
	result.RecordsModified = channelsRemoved + programsRemoved
	result.Message = fmt.Sprintf("Filtered: %d/%d channels, %d/%d programs removed",
		channelsRemoved, originalChannelCount,
		programsRemoved, originalProgramCount)

	// T031: Log filter completion stats
	s.log(ctx, slog.LevelInfo, "filtering complete",
		slog.Int("channels_kept", channelsKept),
		slog.Int("channels_removed", channelsRemoved),
		slog.Int("programs_kept", programsKept),
		slog.Int("programs_removed", programsRemoved))

	// Create artifact
	artifact := core.NewArtifact(core.ArtifactTypeChannels, core.ProcessingStageFiltered, StageID).
		WithRecordCount(channelsKept).
		WithMetadata("channels_removed", channelsRemoved).
		WithMetadata("programs_removed", programsRemoved)
	result.Artifacts = append(result.Artifacts, artifact)

	return result, nil
}

// applyChannelFilters applies filters sequentially to channels.
// - Include: adds matching channels from source to output
// - Exclude: removes matching channels from output
func (s *Stage) applyChannelFilters(ctx context.Context, sourceChannels []*models.Channel) ([]*models.Channel, error) {
	// Build source lookup map for efficient include operations
	// Key by a unique identifier (we'll use pointer address since channels are unique)
	sourceSet := make(map[*models.Channel]bool)
	for _, ch := range sourceChannels {
		sourceSet[ch] = true
	}

	// Output starts empty
	outputSet := make(map[*models.Channel]bool)

	// Get channel filters sorted by priority (already sorted by DB query, but let's be safe)
	channelFilters := make([]*compiledExpressionFilter, 0)
	for _, cef := range s.compiledExpressionFilters {
		if cef.filter.Target == FilterTargetChannel {
			channelFilters = append(channelFilters, cef)
		}
	}

	// Apply filters in order
	for _, cef := range channelFilters {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		switch cef.filter.Action {
		case FilterActionInclude:
			// Include: add matching channels from SOURCE to output
			for ch := range sourceSet {
				if s.channelMatchesFilter(ch, cef) {
					outputSet[ch] = true
				}
			}
		case FilterActionExclude:
			// Exclude: remove matching channels from OUTPUT
			for ch := range outputSet {
				if s.channelMatchesFilter(ch, cef) {
					delete(outputSet, ch)
				}
			}
		}
	}

	// Convert output set to slice, preserving original order
	result := make([]*models.Channel, 0, len(outputSet))
	for _, ch := range sourceChannels {
		if outputSet[ch] {
			result = append(result, ch)
		}
	}

	return result, nil
}

// applyProgramFilters applies filters sequentially to programs.
// Only processes programs that belong to included channels.
func (s *Stage) applyProgramFilters(ctx context.Context, sourcePrograms []*models.EpgProgram, includedChannelIDs map[string]bool) ([]*models.EpgProgram, error) {
	// Build source set (only programs for included channels)
	sourceSet := make(map[*models.EpgProgram]bool)
	for _, prog := range sourcePrograms {
		if includedChannelIDs[prog.ChannelID] {
			sourceSet[prog] = true
		}
	}

	// Output starts empty
	outputSet := make(map[*models.EpgProgram]bool)

	// Get program filters
	programFilters := make([]*compiledExpressionFilter, 0)
	for _, cef := range s.compiledExpressionFilters {
		if cef.filter.Target == FilterTargetProgram {
			programFilters = append(programFilters, cef)
		}
	}

	// If no program filters, include all programs from included channels
	if len(programFilters) == 0 {
		for prog := range sourceSet {
			outputSet[prog] = true
		}
	} else {
		// Apply filters in order
		for _, cef := range programFilters {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			switch cef.filter.Action {
			case FilterActionInclude:
				// Include: add matching programs from SOURCE to output
				for prog := range sourceSet {
					if s.programMatchesFilter(prog, cef) {
						outputSet[prog] = true
					}
				}
			case FilterActionExclude:
				// Exclude: remove matching programs from OUTPUT
				for prog := range outputSet {
					if s.programMatchesFilter(prog, cef) {
						delete(outputSet, prog)
					}
				}
			}
		}
	}

	// Convert output set to slice, preserving original order
	result := make([]*models.EpgProgram, 0, len(outputSet))
	for _, prog := range sourcePrograms {
		if outputSet[prog] {
			result = append(result, prog)
		}
	}

	return result, nil
}

// channelMatchesFilter checks if a channel matches a filter expression.
func (s *Stage) channelMatchesFilter(ch *models.Channel, cef *compiledExpressionFilter) bool {
	evalCtx := s.createChannelEvalContext(ch)
	evalResult, err := cef.evaluator.Evaluate(cef.parsed, evalCtx)
	if err != nil {
		// Log error but treat as non-match
		return false
	}
	return evalResult.Matches
}

// programMatchesFilter checks if a program matches a filter expression.
func (s *Stage) programMatchesFilter(prog *models.EpgProgram, cef *compiledExpressionFilter) bool {
	evalCtx := s.createProgramEvalContext(prog)
	evalResult, err := cef.evaluator.Evaluate(cef.parsed, evalCtx)
	if err != nil {
		// Log error but treat as non-match
		return false
	}
	return evalResult.Matches
}

// log logs a message if the logger is set.
func (s *Stage) log(ctx context.Context, level slog.Level, msg string, attrs ...any) {
	if s.logger != nil {
		s.logger.Log(ctx, level, msg, attrs...)
	}
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

		evaluator := expression.NewEvaluator()
		// Use case-insensitive matching by default (consistent with expression test handler).
		// Per-condition case_sensitive modifier can override this for specific comparisons.
		evaluator.SetCaseSensitive(false)

		cef := &compiledExpressionFilter{
			filter:    filter,
			parsed:    parsed,
			evaluator: evaluator,
		}

		s.compiledExpressionFilters = append(s.compiledExpressionFilters, cef)
	}

	return nil
}

// loadFiltersFromProxy loads filters from the proxy's assigned filters.
// Filters are sorted by priority (lower = first) and only enabled filters are included.
func (s *Stage) loadFiltersFromProxy(ctx context.Context, proxy *models.StreamProxy) error {
	if proxy == nil {
		return nil
	}

	// Sort proxy filters by priority (lower = first)
	proxyFilters := make([]models.ProxyFilter, len(proxy.Filters))
	copy(proxyFilters, proxy.Filters)
	sort.Slice(proxyFilters, func(i, j int) bool {
		return proxyFilters[i].Priority < proxyFilters[j].Priority
	})

	s.expressionFilters = make([]ExpressionFilter, 0, len(proxyFilters))

	for _, pf := range proxyFilters {
		// Skip inactive filter assignments (disabled at the proxy level)
		// IsActive is a pointer; nil or true means active, false means inactive
		if pf.IsActive != nil && !*pf.IsActive {
			s.log(ctx, slog.LevelDebug, "skipping inactive filter assignment",
				slog.String("proxy_filter_id", pf.ID.String()),
				slog.String("filter_id", pf.FilterID.String()))
			continue
		}

		// Skip if filter relationship is not loaded
		if pf.Filter == nil {
			s.log(ctx, slog.LevelWarn, "proxy filter has no loaded filter relationship",
				slog.String("proxy_filter_id", pf.ID.String()),
				slog.String("filter_id", pf.FilterID.String()))
			continue
		}

		f := pf.Filter

		// Skip disabled filters (disabled at the filter level)
		if !models.BoolVal(f.IsEnabled) {
			s.log(ctx, slog.LevelDebug, "skipping disabled filter",
				slog.String("filter_id", f.ID.String()),
				slog.String("filter_name", f.Name))
			continue
		}

		var target FilterTarget
		switch f.SourceType {
		case models.FilterSourceTypeStream:
			target = FilterTargetChannel
		case models.FilterSourceTypeEPG:
			target = FilterTargetProgram
		default:
			s.log(ctx, slog.LevelWarn, "unknown filter source type",
				slog.String("filter_id", f.ID.String()),
				slog.String("source_type", string(f.SourceType)))
			continue
		}

		var action FilterAction
		switch f.Action {
		case models.FilterActionInclude:
			action = FilterActionInclude
		case models.FilterActionExclude:
			action = FilterActionExclude
		default:
			s.log(ctx, slog.LevelWarn, "unknown filter action",
				slog.String("filter_id", f.ID.String()),
				slog.String("action", string(f.Action)))
			continue
		}

		s.expressionFilters = append(s.expressionFilters, ExpressionFilter{
			ID:         f.ID.String(),
			Name:       f.Name,
			Enabled:    models.BoolVal(f.IsEnabled),
			Target:     target,
			Action:     action,
			Expression: f.Expression,
		})

		s.log(ctx, slog.LevelDebug, "loaded filter from proxy",
			slog.String("filter_id", f.ID.String()),
			slog.String("filter_name", f.Name),
			slog.String("action", string(action)),
			slog.Int("priority", pf.Priority))
	}

	return nil
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
