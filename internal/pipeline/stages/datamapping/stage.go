// Package datamapping implements the data mapping/transformation pipeline stage.
package datamapping

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jmylchreest/tvarr/internal/expression"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/jmylchreest/tvarr/internal/pipeline/shared"
)

const (
	// StageID is the unique identifier for this stage.
	StageID = "datamapping"
	// StageName is the human-readable name for this stage.
	StageName = "Data Mapping"
)

// RuleTarget specifies what the rule applies to.
type RuleTarget string

const (
	RuleTargetChannel RuleTarget = "channel"
	RuleTargetProgram RuleTarget = "program"
)

// DataMappingRule represents a data mapping rule configuration.
type DataMappingRule struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Enabled    bool       `json:"enabled"`
	Target     RuleTarget `json:"target"`
	Priority   int        `json:"priority"`
	Expression string     `json:"expression"`
}

// compiledRule holds a pre-parsed rule.
type compiledRule struct {
	rule          *DataMappingRule
	parsed        *expression.ParsedExpression
	ruleProcessor *expression.RuleProcessor
}

// Stage applies data mapping rules to channels and programs.
type Stage struct {
	shared.BaseStage
	rules            []DataMappingRule
	compiledRules    []*compiledRule
	stopOnFirstMatch bool
	logger           *slog.Logger
}

// New creates a new data mapping stage.
func New() *Stage {
	return &Stage{
		BaseStage:        shared.NewBaseStage(StageID, StageName),
		rules:            make([]DataMappingRule, 0),
		compiledRules:    make([]*compiledRule, 0),
		stopOnFirstMatch: false,
	}
}

// NewConstructor returns a stage constructor for use with the factory.
// If a DataMappingRuleRepository is provided in dependencies, it loads enabled rules from the database.
func NewConstructor() core.StageConstructor {
	return func(deps *core.Dependencies) core.Stage {
		stage := New()

		// Inject logger
		if deps.Logger != nil {
			stage.logger = deps.Logger.With("stage", StageID)
		}

		// Load rules from database if repository is available
		if deps.DataMappingRuleRepo != nil {
			dbRules, err := deps.DataMappingRuleRepo.GetEnabled(context.Background())
			if err != nil {
				if deps.Logger != nil {
					deps.Logger.Warn("failed to load data mapping rules from database", "error", err)
				}
			} else {
				rules := make([]DataMappingRule, 0, len(dbRules))
				for _, r := range dbRules {
					var target RuleTarget
					switch r.SourceType {
					case models.DataMappingRuleSourceTypeStream:
						target = RuleTargetChannel
					case models.DataMappingRuleSourceTypeEPG:
						target = RuleTargetProgram
					default:
						continue
					}

					rules = append(rules, DataMappingRule{
						ID:         r.ID.String(),
						Name:       r.Name,
						Enabled:    r.IsEnabled,
						Target:     target,
						Priority:   r.Priority,
						Expression: r.Expression,
					})

					// Set stop on first match based on rule's StopOnMatch flag
					if r.StopOnMatch {
						stage.SetStopOnFirstMatch(true)
					}
				}
				stage.WithRules(rules)
			}
		}

		return stage
	}
}

// WithRules sets the rules for the stage.
func (s *Stage) WithRules(rules []DataMappingRule) *Stage {
	s.rules = rules
	return s
}

// AddRule adds a rule to the stage.
func (s *Stage) AddRule(rule DataMappingRule) *Stage {
	s.rules = append(s.rules, rule)
	return s
}

// SetStopOnFirstMatch configures whether to stop after the first matching rule.
func (s *Stage) SetStopOnFirstMatch(stop bool) *Stage {
	s.stopOnFirstMatch = stop
	return s
}

// Execute applies data mapping rules to channels and programs.
func (s *Stage) Execute(ctx context.Context, state *core.State) (*core.StageResult, error) {
	result := shared.NewResult()

	// Skip if no rules configured
	if len(s.rules) == 0 {
		s.log(ctx, slog.LevelInfo, "no data mapping rules configured, skipping")
		result.Message = "No data mapping rules configured"
		return result, nil
	}

	// T032: Log stage start
	s.log(ctx, slog.LevelInfo, "starting data mapping",
		slog.Int("rule_count", len(s.rules)),
		slog.Int("input_channels", len(state.Channels)),
		slog.Int("input_programs", len(state.Programs)))

	// Compile rules
	if err := s.compileRules(); err != nil {
		// T039: ERROR logging with full context
		s.log(ctx, slog.LevelError, "failed to compile data mapping rules",
			slog.Int("rule_count", len(s.rules)),
			slog.String("error", err.Error()))
		return result, fmt.Errorf("compiling rules: %w", err)
	}

	channelsModified := 0
	programsModified := 0
	totalModifications := 0

	// Process channels
	for _, ch := range state.Channels {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		modifications, err := s.processChannel(ch)
		if err != nil {
			// Log error but continue processing
			continue
		}
		if modifications > 0 {
			channelsModified++
			totalModifications += modifications
		}
	}

	// Update channel map with modified channels
	// Need to re-iterate since TvgLogo etc. may have changed
	for _, ch := range state.Channels {
		if ch.TvgID != "" {
			state.ChannelMap[ch.TvgID] = ch
		}
	}

	// Process programs
	for _, prog := range state.Programs {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		modifications, err := s.processProgram(prog)
		if err != nil {
			// Log error but continue processing
			continue
		}
		if modifications > 0 {
			programsModified++
			totalModifications += modifications
		}
	}

	result.RecordsProcessed = len(state.Channels) + len(state.Programs)
	result.RecordsModified = channelsModified + programsModified
	result.Message = fmt.Sprintf("Data mapping: %d channels, %d programs modified (%d total modifications)",
		channelsModified, programsModified, totalModifications)

	// T032: Log stage completion with rule application stats
	s.log(ctx, slog.LevelInfo, "data mapping complete",
		slog.Int("channels_modified", channelsModified),
		slog.Int("programs_modified", programsModified),
		slog.Int("total_modifications", totalModifications),
		slog.Int("rules_applied", len(s.compiledRules)))

	// Create artifact
	artifact := core.NewArtifact(core.ArtifactTypeChannels, core.ProcessingStageTransformed, StageID).
		WithRecordCount(len(state.Channels)).
		WithMetadata("channels_modified", channelsModified).
		WithMetadata("programs_modified", programsModified).
		WithMetadata("total_modifications", totalModifications)
	result.Artifacts = append(result.Artifacts, artifact)

	return result, nil
}

// compileRules pre-parses all rules.
func (s *Stage) compileRules() error {
	s.compiledRules = make([]*compiledRule, 0, len(s.rules))

	for i := range s.rules {
		rule := &s.rules[i]
		if !rule.Enabled {
			continue
		}

		// Skip empty expressions
		if strings.TrimSpace(rule.Expression) == "" {
			continue
		}

		// Preprocess and parse the expression
		parsed, err := expression.PreprocessAndParse(rule.Expression)
		if err != nil {
			return fmt.Errorf("parsing rule %s (%s): %w", rule.ID, rule.Name, err)
		}

		if parsed == nil {
			continue
		}

		cr := &compiledRule{
			rule:          rule,
			parsed:        parsed,
			ruleProcessor: expression.NewRuleProcessor(),
		}

		s.compiledRules = append(s.compiledRules, cr)
	}

	return nil
}

// processChannel applies rules to a channel.
func (s *Stage) processChannel(ch *models.Channel) (int, error) {
	totalModifications := 0

	for _, cr := range s.compiledRules {
		if cr.rule.Target != RuleTargetChannel {
			continue
		}

		// Create modifiable context for the channel
		ctx := s.createChannelContext(ch)

		// Apply the rule
		ruleResult, err := cr.ruleProcessor.Apply(cr.parsed, ctx)
		if err != nil {
			return totalModifications, err
		}

		if ruleResult.Matched && len(ruleResult.Modifications) > 0 {
			// Apply modifications back to the channel
			s.applyChannelModifications(ch, ruleResult.Modifications)
			totalModifications += len(ruleResult.Modifications)

			if s.stopOnFirstMatch {
				break
			}
		}
	}

	return totalModifications, nil
}

// processProgram applies rules to a program.
func (s *Stage) processProgram(prog *models.EpgProgram) (int, error) {
	totalModifications := 0

	for _, cr := range s.compiledRules {
		if cr.rule.Target != RuleTargetProgram {
			continue
		}

		// Create modifiable context for the program
		ctx := s.createProgramContext(prog)

		// Apply the rule
		ruleResult, err := cr.ruleProcessor.Apply(cr.parsed, ctx)
		if err != nil {
			return totalModifications, err
		}

		if ruleResult.Matched && len(ruleResult.Modifications) > 0 {
			// Apply modifications back to the program
			s.applyProgramModifications(prog, ruleResult.Modifications)
			totalModifications += len(ruleResult.Modifications)

			if s.stopOnFirstMatch {
				break
			}
		}
	}

	return totalModifications, nil
}

// channelContext implements ModifiableContext for channels.
type channelContext struct {
	ch     *models.Channel
	fields map[string]string
}

func (c *channelContext) GetFieldValue(name string) (string, bool) {
	// Check if we have the field in our map
	if val, ok := c.fields[name]; ok {
		return val, true
	}
	// Check aliases
	switch name {
	case "name":
		return c.fields["channel_name"], true
	case "group":
		return c.fields["group_title"], true
	case "url":
		return c.fields["stream_url"], true
	case "logo":
		return c.fields["tvg_logo"], true
	}
	return "", false
}

func (c *channelContext) SetFieldValue(name string, value string) {
	c.fields[name] = value
}

// createChannelContext creates a modifiable context for a channel.
func (s *Stage) createChannelContext(ch *models.Channel) expression.ModifiableContext {
	fields := map[string]string{
		"channel_name": ch.ChannelName,
		"tvg_id":       ch.TvgID,
		"tvg_name":     ch.TvgName,
		"tvg_logo":     ch.TvgLogo,
		"group_title":  ch.GroupTitle,
		"stream_url":   ch.StreamURL,
	}

	return &channelContext{
		ch:     ch,
		fields: fields,
	}
}

// applyChannelModifications applies modifications back to the channel model.
func (s *Stage) applyChannelModifications(ch *models.Channel, modifications []expression.FieldModification) {
	for _, mod := range modifications {
		switch mod.Field {
		case "channel_name", "name":
			ch.ChannelName = mod.NewValue
		case "tvg_id":
			ch.TvgID = mod.NewValue
		case "tvg_name":
			ch.TvgName = mod.NewValue
		case "tvg_logo", "logo":
			ch.TvgLogo = mod.NewValue
		case "group_title", "group":
			ch.GroupTitle = mod.NewValue
			// stream_url is typically read-only
		}
	}
}

// programContext implements ModifiableContext for programs.
type programContext struct {
	prog   *models.EpgProgram
	fields map[string]string
}

func (c *programContext) GetFieldValue(name string) (string, bool) {
	if val, ok := c.fields[name]; ok {
		return val, true
	}
	// Check aliases
	switch name {
	case "title":
		return c.fields["programme_title"], true
	case "description", "desc":
		return c.fields["programme_description"], true
	case "genre":
		return c.fields["programme_category"], true
	case "icon":
		return c.fields["programme_icon"], true
	}
	return "", false
}

func (c *programContext) SetFieldValue(name string, value string) {
	c.fields[name] = value
}

// createProgramContext creates a modifiable context for a program.
func (s *Stage) createProgramContext(prog *models.EpgProgram) expression.ModifiableContext {
	fields := map[string]string{
		"programme_title":       prog.Title,
		"programme_description": prog.Description,
		"programme_category":    prog.Category,
		"programme_icon":        prog.Icon,
	}

	// Add time fields as strings
	if !prog.Start.IsZero() {
		fields["programme_start"] = prog.Start.Format("2006-01-02T15:04:05Z07:00")
	}
	if !prog.Stop.IsZero() {
		fields["programme_stop"] = prog.Stop.Format("2006-01-02T15:04:05Z07:00")
	}

	return &programContext{
		prog:   prog,
		fields: fields,
	}
}

// applyProgramModifications applies modifications back to the program model.
func (s *Stage) applyProgramModifications(prog *models.EpgProgram, modifications []expression.FieldModification) {
	for _, mod := range modifications {
		switch mod.Field {
		case "programme_title", "title":
			prog.Title = mod.NewValue
		case "programme_description", "description", "desc":
			prog.Description = mod.NewValue
		case "programme_category", "genre":
			prog.Category = mod.NewValue
		case "programme_icon", "icon":
			prog.Icon = mod.NewValue
			// Time fields are typically read-only
		}
	}
}

// log logs a message if the logger is set.
func (s *Stage) log(ctx context.Context, level slog.Level, msg string, attrs ...any) {
	if s.logger != nil {
		s.logger.Log(ctx, level, msg, attrs...)
	}
}

// Ensure Stage implements core.Stage.
var _ core.Stage = (*Stage)(nil)
