package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jmylchreest/tvarr/internal/expression"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
)

// ErrRelayProfileMappingNotFound is returned when a relay profile mapping is not found.
var ErrRelayProfileMappingNotFound = errors.New("relay profile mapping not found")

// ErrRelayProfileMappingCannotDeleteSystem is returned when trying to delete a system mapping.
var ErrRelayProfileMappingCannotDeleteSystem = errors.New("cannot delete system mapping")

// ErrRelayProfileMappingCannotEditSystem is returned when trying to edit certain fields of a system mapping.
var ErrRelayProfileMappingCannotEditSystem = errors.New("cannot edit system mapping (only enable/disable allowed)")

// CodecInfo holds information about the source stream's codecs.
type CodecInfo struct {
	VideoCodec models.VideoCodec
	AudioCodec models.AudioCodec
	Container  models.ContainerFormat
}

// CodecDecision represents the result of evaluating client detection rules.
type CodecDecision struct {
	MappingID        models.ULID
	MappingName      string
	VideoAction      string // "copy" or "transcode"
	AudioAction      string // "copy" or "transcode"
	TargetVideoCodec models.VideoCodec
	TargetAudioCodec models.AudioCodec
	TargetContainer  models.ContainerFormat
}

// RelayProfileMappingService provides business logic for relay profile mappings.
type RelayProfileMappingService struct {
	repo      repository.RelayProfileMappingRepository
	evaluator *expression.Evaluator
	logger    *slog.Logger
}

// NewRelayProfileMappingService creates a new relay profile mapping service.
func NewRelayProfileMappingService(repo repository.RelayProfileMappingRepository) *RelayProfileMappingService {
	return &RelayProfileMappingService{
		repo:      repo,
		evaluator: expression.NewEvaluator(),
		logger:    slog.Default(),
	}
}

// WithLogger sets the logger for the service.
func (s *RelayProfileMappingService) WithLogger(logger *slog.Logger) *RelayProfileMappingService {
	s.logger = logger
	return s
}

// Create creates a new relay profile mapping.
func (s *RelayProfileMappingService) Create(ctx context.Context, mapping *models.RelayProfileMapping) error {
	// Validate expression syntax
	if _, err := expression.Parse(mapping.Expression); err != nil {
		return err
	}
	return s.repo.Create(ctx, mapping)
}

// GetByID retrieves a relay profile mapping by ID.
func (s *RelayProfileMappingService) GetByID(ctx context.Context, id models.ULID) (*models.RelayProfileMapping, error) {
	mapping, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if mapping == nil {
		return nil, ErrRelayProfileMappingNotFound
	}
	return mapping, nil
}

// GetAll retrieves all relay profile mappings.
func (s *RelayProfileMappingService) GetAll(ctx context.Context) ([]*models.RelayProfileMapping, error) {
	return s.repo.GetAll(ctx)
}

// GetEnabled retrieves all enabled relay profile mappings.
func (s *RelayProfileMappingService) GetEnabled(ctx context.Context) ([]*models.RelayProfileMapping, error) {
	return s.repo.GetEnabled(ctx)
}

// GetEnabledByPriority retrieves enabled mappings ordered by priority.
func (s *RelayProfileMappingService) GetEnabledByPriority(ctx context.Context) ([]*models.RelayProfileMapping, error) {
	return s.repo.GetEnabledByPriority(ctx)
}

// Update updates an existing relay profile mapping.
func (s *RelayProfileMappingService) Update(ctx context.Context, mapping *models.RelayProfileMapping) error {
	existing, err := s.repo.GetByID(ctx, mapping.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrRelayProfileMappingNotFound
	}

	// System mappings can only have IsEnabled toggled
	if existing.IsSystem {
		// Allow only IsEnabled changes for system mappings
		if mapping.Name != existing.Name ||
			mapping.Expression != existing.Expression ||
			mapping.Description != existing.Description {
			return ErrRelayProfileMappingCannotEditSystem
		}
	}

	// Validate expression syntax
	if _, err := expression.Parse(mapping.Expression); err != nil {
		return err
	}

	return s.repo.Update(ctx, mapping)
}

// Delete deletes a relay profile mapping by ID.
func (s *RelayProfileMappingService) Delete(ctx context.Context, id models.ULID) error {
	mapping, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if mapping == nil {
		return ErrRelayProfileMappingNotFound
	}

	if mapping.IsSystem {
		return ErrRelayProfileMappingCannotDeleteSystem
	}

	return s.repo.Delete(ctx, id)
}

// Count returns the total number of relay profile mappings.
func (s *RelayProfileMappingService) Count(ctx context.Context) (int64, error) {
	return s.repo.Count(ctx)
}

// CountEnabled returns the number of enabled mappings.
func (s *RelayProfileMappingService) CountEnabled(ctx context.Context) (int64, error) {
	return s.repo.CountEnabled(ctx)
}

// CountSystem returns the number of system mappings.
func (s *RelayProfileMappingService) CountSystem(ctx context.Context) (int64, error) {
	return s.repo.CountSystem(ctx)
}

// Reorder updates the priority of multiple mappings.
func (s *RelayProfileMappingService) Reorder(ctx context.Context, requests []repository.ReorderRequest) error {
	return s.repo.Reorder(ctx, requests)
}

// EvaluateRequest evaluates client detection rules against an HTTP request.
// It returns the codec decision based on the first matching rule.
// If no rule matches, returns nil.
func (s *RelayProfileMappingService) EvaluateRequest(ctx context.Context, r *http.Request, sourceCodecs *CodecInfo) (*CodecDecision, error) {
	mappings, err := s.repo.GetEnabledByPriority(ctx)
	if err != nil {
		return nil, err
	}

	accessor := expression.NewRequestContextAccessor(r)

	for _, mapping := range mappings {
		parsed, err := expression.Parse(mapping.Expression)
		if err != nil {
			s.logger.Warn("failed to parse mapping expression",
				"mapping_id", mapping.ID,
				"mapping_name", mapping.Name,
				"error", err)
			continue
		}

		result, err := s.evaluator.Evaluate(parsed, accessor)
		if err != nil {
			s.logger.Warn("failed to evaluate mapping expression",
				"mapping_id", mapping.ID,
				"mapping_name", mapping.Name,
				"error", err)
			continue
		}

		if result.Matches {
			// Found a matching rule - determine codec decision
			decision := &CodecDecision{
				MappingID:   mapping.ID,
				MappingName: mapping.Name,
			}

			// Determine video codec
			// If source codec is unknown (empty), default to copy (optimistic passthrough)
			// Most streams use standard codecs and transcoding is expensive
			if sourceCodecs != nil && sourceCodecs.VideoCodec != "" && mapping.AcceptsVideoCodec(sourceCodecs.VideoCodec) {
				decision.VideoAction = "copy"
				decision.TargetVideoCodec = models.VideoCodecCopy
			} else if sourceCodecs == nil || sourceCodecs.VideoCodec == "" {
				// Source codec unknown - default to copy (optimistic)
				decision.VideoAction = "copy"
				decision.TargetVideoCodec = models.VideoCodecCopy
			} else {
				decision.VideoAction = "transcode"
				decision.TargetVideoCodec = mapping.PreferredVideoCodec
			}

			// Determine audio codec
			// If source codec is unknown (empty), default to copy (optimistic passthrough)
			if sourceCodecs != nil && sourceCodecs.AudioCodec != "" && mapping.AcceptsAudioCodec(sourceCodecs.AudioCodec) {
				decision.AudioAction = "copy"
				decision.TargetAudioCodec = models.AudioCodecCopy
			} else if sourceCodecs == nil || sourceCodecs.AudioCodec == "" {
				// Source codec unknown - default to copy (optimistic)
				decision.AudioAction = "copy"
				decision.TargetAudioCodec = models.AudioCodecCopy
			} else {
				decision.AudioAction = "transcode"
				decision.TargetAudioCodec = mapping.PreferredAudioCodec
			}

			// Determine container
			if sourceCodecs != nil && sourceCodecs.Container != "" && mapping.AcceptsContainer(sourceCodecs.Container) {
				decision.TargetContainer = models.ContainerFormatAuto
			} else if sourceCodecs == nil || sourceCodecs.Container == "" {
				// Source container unknown - default to auto
				decision.TargetContainer = models.ContainerFormatAuto
			} else {
				decision.TargetContainer = mapping.PreferredContainer
			}

			s.logger.Debug("matched client detection rule",
				"mapping_name", mapping.Name,
				"user_agent", r.UserAgent(),
				"video_action", decision.VideoAction,
				"audio_action", decision.AudioAction)

			return decision, nil
		}
	}

	// No matching rule found
	return nil, nil
}

// TestExpression tests an expression against a sample request context.
// Returns true if the expression matches, false otherwise.
func (s *RelayProfileMappingService) TestExpression(expr string, testData map[string]string) (bool, error) {
	parsed, err := expression.Parse(expr)
	if err != nil {
		return false, err
	}

	accessor := &testAccessor{data: testData}
	result, err := s.evaluator.Evaluate(parsed, accessor)
	if err != nil {
		return false, err
	}

	return result.Matches, nil
}

// testAccessor implements FieldValueAccessor for testing expressions.
type testAccessor struct {
	data map[string]string
}

func (a *testAccessor) GetFieldValue(name string) (string, bool) {
	val, ok := a.data[name]
	return val, ok
}

// GetStats returns statistics about relay profile mappings.
func (s *RelayProfileMappingService) GetStats(ctx context.Context) (*MappingStats, error) {
	total, err := s.repo.Count(ctx)
	if err != nil {
		return nil, err
	}

	enabled, err := s.repo.CountEnabled(ctx)
	if err != nil {
		return nil, err
	}

	system, err := s.repo.CountSystem(ctx)
	if err != nil {
		return nil, err
	}

	return &MappingStats{
		Total:   total,
		Enabled: enabled,
		System:  system,
		Custom:  total - system,
	}, nil
}

// MappingStats contains statistics about relay profile mappings.
type MappingStats struct {
	Total   int64 `json:"total"`
	Enabled int64 `json:"enabled"`
	System  int64 `json:"system"`
	Custom  int64 `json:"custom"`
}

// RuleValidationIssue represents a potential problem with a client detection rule.
type RuleValidationIssue struct {
	MappingID   models.ULID
	MappingName string
	Priority    int
	IssueType   string // "always_true", "possibly_shadowed", "invalid_expression"
	Message     string
}

// ValidateRulesOnLoad validates all enabled rules and logs warnings for any issues.
// This should be called when the service is initialized or rules are modified.
func (s *RelayProfileMappingService) ValidateRulesOnLoad(ctx context.Context) {
	mappings, err := s.repo.GetEnabledByPriority(ctx)
	if err != nil {
		s.logger.Warn("failed to load rules for validation", "error", err)
		return
	}

	issues := s.analyzeRules(mappings)
	for _, issue := range issues {
		s.logger.Warn("Client detection rule issue detected",
			"mapping_name", issue.MappingName,
			"mapping_id", issue.MappingID,
			"priority", issue.Priority,
			"issue_type", issue.IssueType,
			"message", issue.Message,
		)
	}

	if len(issues) == 0 {
		s.logger.Debug("All client detection rules validated successfully",
			"rule_count", len(mappings))
	}
}

// analyzeRules analyzes a set of rules for potential issues.
func (s *RelayProfileMappingService) analyzeRules(mappings []*models.RelayProfileMapping) []RuleValidationIssue {
	var issues []RuleValidationIssue

	// Track patterns we've seen to detect shadowing
	type seenPattern struct {
		mapping   *models.RelayProfileMapping
		isGeneral bool // true if this is a catch-all or very broad pattern
	}
	var seenPatterns []seenPattern

	for _, mapping := range mappings {
		// Parse the expression
		parsed, err := expression.Parse(mapping.Expression)
		if err != nil {
			issues = append(issues, RuleValidationIssue{
				MappingID:   mapping.ID,
				MappingName: mapping.Name,
				Priority:    mapping.Priority,
				IssueType:   "invalid_expression",
				Message:     "Failed to parse expression: " + err.Error(),
			})
			continue
		}

		// Check if expression is always true (catch-all)
		isAlwaysTrue := s.isAlwaysTrueExpression(parsed)
		if isAlwaysTrue && mapping.Priority != 999 {
			// Catch-all patterns should have lowest priority
			issues = append(issues, RuleValidationIssue{
				MappingID:   mapping.ID,
				MappingName: mapping.Name,
				Priority:    mapping.Priority,
				IssueType:   "always_true",
				Message:     "Rule has 'always true' condition but is not at lowest priority (999). This rule will shadow all lower-priority rules.",
			})
		}

		// Check for shadowing by earlier (higher priority) rules
		for _, seen := range seenPatterns {
			if seen.isGeneral {
				issues = append(issues, RuleValidationIssue{
					MappingID:   mapping.ID,
					MappingName: mapping.Name,
					Priority:    mapping.Priority,
					IssueType:   "possibly_shadowed",
					Message:     "Rule may be unreachable - shadowed by catch-all rule '" + seen.mapping.Name + "' (priority " + itoa(seen.mapping.Priority) + ")",
				})
				break // Only report one shadowing issue per rule
			}

			// Check for potential shadowing by broad patterns
			if s.couldShadow(seen.mapping, mapping) {
				issues = append(issues, RuleValidationIssue{
					MappingID:   mapping.ID,
					MappingName: mapping.Name,
					Priority:    mapping.Priority,
					IssueType:   "possibly_shadowed",
					Message:     "Rule may be partially shadowed by '" + seen.mapping.Name + "' (priority " + itoa(seen.mapping.Priority) + ") - check if expressions overlap",
				})
			}
		}

		seenPatterns = append(seenPatterns, seenPattern{
			mapping:   mapping,
			isGeneral: isAlwaysTrue,
		})
	}

	return issues
}

// isAlwaysTrueExpression checks if an expression will always match.
func (s *RelayProfileMappingService) isAlwaysTrueExpression(parsed *expression.ParsedExpression) bool {
	if parsed == nil || parsed.Expression == nil {
		return true // Empty expression matches everything
	}

	switch expr := parsed.Expression.(type) {
	case *expression.ConditionOnly:
		return s.isAlwaysTrueConditionTree(expr.Condition)
	case *expression.ConditionWithActions:
		return s.isAlwaysTrueConditionTree(expr.Condition)
	}
	return false
}

// isAlwaysTrueConditionTree checks if a condition tree will always match.
func (s *RelayProfileMappingService) isAlwaysTrueConditionTree(tree *expression.ConditionTree) bool {
	if tree == nil || tree.Root == nil {
		return true // Empty tree matches everything
	}
	return s.isAlwaysTrueNode(tree.Root)
}

// isAlwaysTrueNode checks if a condition node will always match.
func (s *RelayProfileMappingService) isAlwaysTrueNode(node expression.ConditionNode) bool {
	switch n := node.(type) {
	case *expression.Condition:
		return s.isAlwaysTrueCondition(n)
	case *expression.ConditionGroup:
		if n.Operator == expression.LogicalOr {
			// OR: true if any child is always true
			for _, child := range n.Children {
				if s.isAlwaysTrueNode(child) {
					return true
				}
			}
			return false
		}
		// AND: true only if all children are always true
		for _, child := range n.Children {
			if !s.isAlwaysTrueNode(child) {
				return false
			}
		}
		return true
	}
	return false
}

// isAlwaysTrueCondition checks if a single condition will always match.
func (s *RelayProfileMappingService) isAlwaysTrueCondition(cond *expression.Condition) bool {
	// Detect patterns that are always true
	// Pattern: field contains "" - empty string is contained in any string
	if cond.Operator == expression.OpContains && cond.Value == "" {
		return true
	}
	// Pattern: field equals field (self-equality)
	if cond.Operator == expression.OpEquals && cond.Field == cond.Value {
		return true
	}
	return false
}

// couldShadow checks if rule A could potentially shadow rule B.
// This is a heuristic check - it's not always accurate but helps catch obvious issues.
func (s *RelayProfileMappingService) couldShadow(higher, lower *models.RelayProfileMapping) bool {
	// Both must be testing the same field for potential shadowing
	higherParsed, err := expression.Parse(higher.Expression)
	if err != nil {
		return false
	}
	lowerParsed, err := expression.Parse(lower.Expression)
	if err != nil {
		return false
	}

	// Get the fields used in each expression
	higherFields := s.getConditionFields(higherParsed)
	lowerFields := s.getConditionFields(lowerParsed)

	// If they don't share any fields, no shadowing possible
	hasCommonField := false
	for field := range higherFields {
		if lowerFields[field] {
			hasCommonField = true
			break
		}
	}
	if !hasCommonField {
		return false
	}

	// Check if the higher rule uses a "contains" that could match the lower rule's more specific pattern
	// For example: "user_agent contains 'Chrome'" shadows "user_agent contains 'Chrome' AND user_agent contains 'Windows'"
	higherHasContains := s.hasOperator(higherParsed, expression.OpContains)
	lowerHasAnd := s.hasLogicalAnd(lowerParsed)

	// If higher has a simple contains and lower has AND conditions on the same field,
	// the higher rule might shadow the lower one
	if higherHasContains && lowerHasAnd && len(higherFields) < len(lowerFields) {
		// Higher is simpler (fewer fields) - might shadow
		return true
	}

	return false
}

// getConditionFields extracts all field names from an expression.
func (s *RelayProfileMappingService) getConditionFields(parsed *expression.ParsedExpression) map[string]bool {
	fields := make(map[string]bool)
	if parsed == nil || parsed.Expression == nil {
		return fields
	}

	var extract func(node expression.ConditionNode)
	extract = func(node expression.ConditionNode) {
		if node == nil {
			return
		}
		switch n := node.(type) {
		case *expression.Condition:
			fields[n.Field] = true
		case *expression.ConditionGroup:
			for _, child := range n.Children {
				extract(child)
			}
		}
	}

	switch expr := parsed.Expression.(type) {
	case *expression.ConditionOnly:
		if expr.Condition != nil {
			extract(expr.Condition.Root)
		}
	case *expression.ConditionWithActions:
		if expr.Condition != nil {
			extract(expr.Condition.Root)
		}
	}

	return fields
}

// hasOperator checks if an expression uses a specific operator.
func (s *RelayProfileMappingService) hasOperator(parsed *expression.ParsedExpression, op expression.FilterOperator) bool {
	if parsed == nil || parsed.Expression == nil {
		return false
	}

	var check func(node expression.ConditionNode) bool
	check = func(node expression.ConditionNode) bool {
		if node == nil {
			return false
		}
		switch n := node.(type) {
		case *expression.Condition:
			return n.Operator == op
		case *expression.ConditionGroup:
			for _, child := range n.Children {
				if check(child) {
					return true
				}
			}
		}
		return false
	}

	switch expr := parsed.Expression.(type) {
	case *expression.ConditionOnly:
		if expr.Condition != nil {
			return check(expr.Condition.Root)
		}
	case *expression.ConditionWithActions:
		if expr.Condition != nil {
			return check(expr.Condition.Root)
		}
	}

	return false
}

// hasLogicalAnd checks if an expression uses AND logic.
func (s *RelayProfileMappingService) hasLogicalAnd(parsed *expression.ParsedExpression) bool {
	if parsed == nil || parsed.Expression == nil {
		return false
	}

	var check func(node expression.ConditionNode) bool
	check = func(node expression.ConditionNode) bool {
		if node == nil {
			return false
		}
		switch n := node.(type) {
		case *expression.ConditionGroup:
			if n.Operator == expression.LogicalAnd {
				return true
			}
			for _, child := range n.Children {
				if check(child) {
					return true
				}
			}
		}
		return false
	}

	switch expr := parsed.Expression.(type) {
	case *expression.ConditionOnly:
		if expr.Condition != nil {
			return check(expr.Condition.Root)
		}
	case *expression.ConditionWithActions:
		if expr.Condition != nil {
			return check(expr.Condition.Root)
		}
	}

	return false
}

// itoa converts an int to a string (simple helper).
func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
