package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"

	"github.com/jmylchreest/tvarr/internal/expression"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
)

// Service-level errors for client detection rules.
var (
	// ErrClientDetectionRuleCannotDeleteSystem is returned when trying to delete a system rule.
	ErrClientDetectionRuleCannotDeleteSystem = errors.New("cannot delete system client detection rule")

	// ErrClientDetectionRuleCannotEditSystem is returned when trying to edit certain fields of a system rule.
	ErrClientDetectionRuleCannotEditSystem = errors.New("cannot edit system client detection rule (only enabled toggle allowed)")

	// ErrClientDetectionRuleNotFound is returned when a rule is not found.
	ErrClientDetectionRuleNotFound = errors.New("client detection rule not found")

	// ErrClientDetectionRuleInvalidExpression is returned when an expression fails to parse.
	ErrClientDetectionRuleInvalidExpression = errors.New("invalid client detection rule expression")
)

// ClientDetectionService provides business logic for client detection rules.
type ClientDetectionService struct {
	repo      repository.ClientDetectionRuleRepository
	logger    *slog.Logger
	evaluator *expression.Evaluator

	// Cache for enabled rules (refreshed periodically)
	mu          sync.RWMutex
	cachedRules []*models.ClientDetectionRule
}

// NewClientDetectionService creates a new client detection service.
func NewClientDetectionService(repo repository.ClientDetectionRuleRepository) *ClientDetectionService {
	return &ClientDetectionService{
		repo:      repo,
		logger:    slog.Default(),
		evaluator: expression.NewEvaluator(),
	}
}

// WithLogger sets the logger for the service.
func (s *ClientDetectionService) WithLogger(logger *slog.Logger) *ClientDetectionService {
	s.logger = logger
	return s
}

// RefreshCache refreshes the cached enabled rules.
// Call this on startup and periodically or when rules change.
func (s *ClientDetectionService) RefreshCache(ctx context.Context) error {
	rules, err := s.repo.GetEnabled(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.cachedRules = rules
	s.mu.Unlock()

	s.logger.Debug("client detection rules cache refreshed",
		slog.Int("rule_count", len(rules)),
	)
	return nil
}

// Create creates a new client detection rule.
func (s *ClientDetectionService) Create(ctx context.Context, rule *models.ClientDetectionRule) error {
	// System rules cannot be created via the service (only via migrations)
	rule.IsSystem = false

	// Validate the expression can be parsed
	if _, err := expression.Parse(rule.Expression); err != nil {
		return ErrClientDetectionRuleInvalidExpression
	}

	if err := s.repo.Create(ctx, rule); err != nil {
		return err
	}

	// Refresh cache after create
	_ = s.RefreshCache(ctx)
	return nil
}

// GetByID retrieves a client detection rule by ID.
func (s *ClientDetectionService) GetByID(ctx context.Context, id models.ULID) (*models.ClientDetectionRule, error) {
	rule, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if rule == nil {
		return nil, ErrClientDetectionRuleNotFound
	}
	return rule, nil
}

// GetAll retrieves all client detection rules.
func (s *ClientDetectionService) GetAll(ctx context.Context) ([]*models.ClientDetectionRule, error) {
	return s.repo.GetAll(ctx)
}

// GetEnabled retrieves all enabled client detection rules.
func (s *ClientDetectionService) GetEnabled(ctx context.Context) ([]*models.ClientDetectionRule, error) {
	return s.repo.GetEnabled(ctx)
}

// GetByName retrieves a rule by name.
func (s *ClientDetectionService) GetByName(ctx context.Context, name string) (*models.ClientDetectionRule, error) {
	rule, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if rule == nil {
		return nil, ErrClientDetectionRuleNotFound
	}
	return rule, nil
}

// GetSystem retrieves all system rules.
func (s *ClientDetectionService) GetSystem(ctx context.Context) ([]*models.ClientDetectionRule, error) {
	return s.repo.GetSystem(ctx)
}

// Update updates an existing rule.
// For system rules, only the IsEnabled field can be changed.
func (s *ClientDetectionService) Update(ctx context.Context, rule *models.ClientDetectionRule) error {
	existing, err := s.repo.GetByID(ctx, rule.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrClientDetectionRuleNotFound
	}

	// Validate the expression can be parsed
	if _, err := expression.Parse(rule.Expression); err != nil {
		return ErrClientDetectionRuleInvalidExpression
	}

	// System rules can only have their IsEnabled field toggled
	if existing.IsSystem {
		if isClientDetectionSystemFieldChanged(existing, rule) {
			return ErrClientDetectionRuleCannotEditSystem
		}
		// Only allow enabled toggle for system rules
		existing.IsEnabled = rule.IsEnabled
		if err := s.repo.Update(ctx, existing); err != nil {
			return err
		}
	} else {
		// Non-system rules can be fully updated (except IsSystem flag)
		rule.IsSystem = false
		if err := s.repo.Update(ctx, rule); err != nil {
			return err
		}
	}

	// Refresh cache after update
	_ = s.RefreshCache(ctx)
	return nil
}

// Delete deletes a rule by ID.
// System rules cannot be deleted.
func (s *ClientDetectionService) Delete(ctx context.Context, id models.ULID) error {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrClientDetectionRuleNotFound
	}

	if existing.IsSystem {
		return ErrClientDetectionRuleCannotDeleteSystem
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	// Refresh cache after delete
	_ = s.RefreshCache(ctx)
	return nil
}

// Count returns the total number of rules.
func (s *ClientDetectionService) Count(ctx context.Context) (int64, error) {
	return s.repo.Count(ctx)
}

// CountEnabled returns the number of enabled rules.
func (s *ClientDetectionService) CountEnabled(ctx context.Context) (int64, error) {
	return s.repo.CountEnabled(ctx)
}

// Reorder updates priorities for multiple rules.
func (s *ClientDetectionService) Reorder(ctx context.Context, reorders []repository.ReorderRequest) error {
	if err := s.repo.Reorder(ctx, reorders); err != nil {
		return err
	}

	// Refresh cache after reorder
	_ = s.RefreshCache(ctx)
	return nil
}

// ToggleEnabled toggles the enabled state of a rule.
func (s *ClientDetectionService) ToggleEnabled(ctx context.Context, id models.ULID) (*models.ClientDetectionRule, error) {
	rule, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if rule == nil {
		return nil, ErrClientDetectionRuleNotFound
	}

	newVal := !models.BoolVal(rule.IsEnabled)
	rule.IsEnabled = &newVal
	if err := s.repo.Update(ctx, rule); err != nil {
		return nil, err
	}

	// Refresh cache after toggle
	_ = s.RefreshCache(ctx)
	return rule, nil
}

// clientDetectionContext implements expression.ModifiableContext for receiving SET action results.
// It wraps the request accessor and captures field modifications.
type clientDetectionContext struct {
	accessor expression.FieldValueAccessor
	fields   map[string]string
}

// newClientDetectionContext creates a new context for client detection evaluation.
func newClientDetectionContext(accessor expression.FieldValueAccessor) *clientDetectionContext {
	return &clientDetectionContext{
		accessor: accessor,
		fields:   make(map[string]string),
	}
}

// GetFieldValue gets a field value, first checking captured fields then falling back to accessor.
func (c *clientDetectionContext) GetFieldValue(name string) (string, bool) {
	// Check captured fields first
	if v, ok := c.fields[name]; ok {
		return v, true
	}
	// Fall back to base accessor
	return c.accessor.GetFieldValue(name)
}

// SetFieldValue captures a field modification from a SET action.
func (c *clientDetectionContext) SetFieldValue(name, value string) {
	c.fields[name] = value
}

// GetModifiedField returns a modified field value if it was SET, empty string otherwise.
func (c *clientDetectionContext) GetModifiedField(name string) string {
	return c.fields[name]
}

// EvaluateRequest evaluates all enabled rules against the given HTTP request.
// Rules are merged: the first rule to set each codec/container attribute wins.
// Rules may use SET actions to dynamically extract values from headers.
// Once all attributes are set, evaluation stops.
// Returns the merged result, or defaults for any unset attributes.
func (s *ClientDetectionService) EvaluateRequest(r *http.Request) *models.ClientDetectionResult {
	s.mu.RLock()
	rules := s.cachedRules
	s.mu.RUnlock()

	accessor := expression.NewRequestContextAccessor(r)

	// Set up DynamicContext for @dynamic(path):key resolution
	// In client detection context, we inject request.headers and request.query
	dynCtx := expression.NewDynamicContext()
	dynCtx.SetRequestHeaders(r.Header)

	// Set query params if present
	queryParams := make(map[string]string)
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			queryParams[key] = values[0]
		}
	}
	if len(queryParams) > 0 {
		dynCtx.SetQueryParams(queryParams)
	}

	// Create registry with unified context for @dynamic() syntax
	registry := expression.NewDynamicFieldRegistryWithContext(dynCtx)

	// Configure rule processor with the dynamic field registry
	processor := expression.NewRuleProcessor().WithDynamicRegistry(registry)

	// Initialize result with unset values
	result := &models.ClientDetectionResult{
		DetectionSource: "default",
	}

	// Track which attributes have been set
	var (
		videoSet    bool
		audioSet    bool
		fmp4Set     bool
		mpegtsSet   bool
		formatSet   bool
		matchedRule *models.ClientDetectionRule
	)

	for _, rule := range rules {
		if !models.BoolVal(rule.IsEnabled) {
			continue
		}

		// Parse and evaluate the expression (may include SET actions)
		parsed, err := expression.Parse(rule.Expression)
		if err != nil {
			s.logger.Warn("failed to parse client detection rule expression",
				slog.String("rule_name", rule.Name),
				slog.String("error", err.Error()),
			)
			continue
		}

		// Create a context that can receive SET action modifications
		ctx := newClientDetectionContext(expression.NewDynamicFieldAccessor(accessor, registry))

		// Apply the rule (evaluates condition and applies any SET actions)
		ruleResult, err := processor.Apply(parsed, ctx)
		if err != nil {
			s.logger.Warn("failed to apply client detection rule",
				slog.String("rule_name", rule.Name),
				slog.String("error", err.Error()),
			)
			continue
		}

		if ruleResult.Matched {
			// Track the first matching rule
			if matchedRule == nil {
				matchedRule = rule
				result.DetectionSource = "rule"
			}

			// Build log attributes
			attrs := []any{
				slog.String("rule_name", rule.Name),
				slog.String("user_agent", r.UserAgent()),
				slog.Int("rule_priority", rule.Priority),
			}

			// Log explicit codec headers if present
			if videoCodec := r.Header.Get("X-Video-Codec"); videoCodec != "" {
				attrs = append(attrs, slog.String("x_video_codec", videoCodec))
			}
			if audioCodec := r.Header.Get("X-Audio-Codec"); audioCodec != "" {
				attrs = append(attrs, slog.String("x_audio_codec", audioCodec))
			}

			// Check for SET action modifications first, then fall back to static rule values

			// Merge video codec if not already set
			if !videoSet {
				// Check if SET action extracted a preferred_video_codec
				if setVideo := ctx.GetModifiedField("preferred_video_codec"); setVideo != "" {
					// Validate the extracted codec value
					if videoCodec, valid := models.ParseVideoCodec(setVideo); valid {
						result.AcceptedVideoCodecs = []string{string(videoCodec)}
						result.PreferredVideoCodec = string(videoCodec)
						videoSet = true
						attrs = append(attrs, slog.String("contributed", "video_dynamic"))
						attrs = append(attrs, slog.String("video_codec", string(videoCodec)))
					} else {
						// SET action extracted an invalid codec value
						s.logger.Warn("invalid video codec from SET action",
							slog.String("value", setVideo),
							slog.String("rule_name", rule.Name),
						)
					}
				}
				// Fall back to static PreferredVideoCodec if SET didn't set it
				if !videoSet && rule.PreferredVideoCodec != "" {
					result.AcceptedVideoCodecs = rule.GetAcceptedVideoCodecs()
					result.PreferredVideoCodec = string(rule.PreferredVideoCodec)
					videoSet = true
					attrs = append(attrs, slog.String("contributed", "video"))
				}
			}

			// Merge audio codec if not already set
			if !audioSet {
				// Check if SET action extracted a preferred_audio_codec
				if setAudio := ctx.GetModifiedField("preferred_audio_codec"); setAudio != "" {
					// Validate the extracted codec value
					if audioCodec, valid := models.ParseAudioCodec(setAudio); valid {
						result.AcceptedAudioCodecs = []string{string(audioCodec)}
						result.PreferredAudioCodec = string(audioCodec)
						audioSet = true
						attrs = append(attrs, slog.String("contributed", "audio_dynamic"))
						attrs = append(attrs, slog.String("audio_codec", string(audioCodec)))
					} else {
						// SET action extracted an invalid codec value
						s.logger.Warn("invalid audio codec from SET action",
							slog.String("value", setAudio),
							slog.String("rule_name", rule.Name),
						)
					}
				}
				// Fall back to static PreferredAudioCodec if SET didn't set it
				if !audioSet && rule.PreferredAudioCodec != "" {
					result.AcceptedAudioCodecs = rule.GetAcceptedAudioCodecs()
					result.PreferredAudioCodec = string(rule.PreferredAudioCodec)
					audioSet = true
					attrs = append(attrs, slog.String("contributed", "audio"))
				}
			}

			// Merge fMP4 support if not already set
			if !fmp4Set {
				result.SupportsFMP4 = models.BoolVal(rule.SupportsFMP4)
				fmp4Set = true
			}

			// Merge MPEG-TS support if not already set
			if !mpegtsSet {
				result.SupportsMPEGTS = models.BoolVal(rule.SupportsMPEGTS)
				mpegtsSet = true
			}

			// Merge preferred format if not already set
			if !formatSet {
				// Check if SET action extracted a preferred_format
				if setFormat := ctx.GetModifiedField("preferred_format"); setFormat != "" {
					// Validate the extracted format value
					if format, valid := models.ParsePreferredFormat(setFormat); valid {
						result.PreferredFormat = format
						formatSet = true
						attrs = append(attrs, slog.String("contributed", "format_dynamic"))
						attrs = append(attrs, slog.String("format", format))
					} else {
						// SET action extracted an invalid format value
						s.logger.Warn("invalid preferred format from SET action",
							slog.String("value", setFormat),
							slog.String("rule_name", rule.Name),
						)
					}
				}
				// Fall back to static PreferredFormat if SET didn't set it
				if !formatSet && rule.PreferredFormat != "" {
					result.PreferredFormat = rule.PreferredFormat
					formatSet = true
					attrs = append(attrs, slog.String("contributed", "format"))
				}
			}

			s.logger.Debug("client detection rule matched", attrs...)

			// Check if all attributes are set
			if videoSet && audioSet && fmp4Set && mpegtsSet && formatSet {
				s.logger.Debug("all client detection attributes set, stopping evaluation",
					slog.String("user_agent", r.UserAgent()),
				)
				break
			}
		}
	}

	// Store the first matched rule (for API response purposes)
	result.MatchedRule = matchedRule

	// Apply defaults for any unset attributes
	if !videoSet {
		result.AcceptedVideoCodecs = []string{"h264"}
		result.PreferredVideoCodec = "h264"
	}
	if !audioSet {
		result.AcceptedAudioCodecs = []string{"aac"}
		result.PreferredAudioCodec = "aac"
	}
	if !fmp4Set {
		result.SupportsFMP4 = true
	}
	if !mpegtsSet {
		result.SupportsMPEGTS = true
	}

	// Log final client capabilities at INFO level for visibility
	ruleName := "none (defaults)"
	if matchedRule != nil {
		ruleName = matchedRule.Name
	}
	s.logger.Info("client capabilities determined",
		slog.String("user_agent", r.UserAgent()),
		slog.String("matched_rule", ruleName),
		slog.Any("accepted_video", result.AcceptedVideoCodecs),
		slog.Any("accepted_audio", result.AcceptedAudioCodecs),
		slog.String("preferred_video", result.PreferredVideoCodec),
		slog.String("preferred_audio", result.PreferredAudioCodec),
		slog.String("preferred_format", result.PreferredFormat),
		slog.Bool("supports_fmp4", result.SupportsFMP4),
		slog.Bool("supports_mpegts", result.SupportsMPEGTS),
	)

	if matchedRule == nil {
		s.logger.Debug("no client detection rule matched, using defaults",
			slog.String("user_agent", r.UserAgent()),
		)
	}

	return result
}

// TestExpression tests an expression against a sample request.
// Returns whether it matches and any parsing errors.
func (s *ClientDetectionService) TestExpression(expr string, r *http.Request) (bool, error) {
	parsed, err := expression.Parse(expr)
	if err != nil {
		return false, err
	}

	accessor := expression.NewRequestContextAccessor(r)

	// Set up DynamicContext for @dynamic(path):key resolution
	dynCtx := expression.NewDynamicContext()
	dynCtx.SetRequestHeaders(r.Header)

	// Set query params if present
	queryParams := make(map[string]string)
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			queryParams[key] = values[0]
		}
	}
	if len(queryParams) > 0 {
		dynCtx.SetQueryParams(queryParams)
	}

	// Create registry with unified context for @dynamic() syntax
	registry := expression.NewDynamicFieldRegistryWithContext(dynCtx)

	result, err := s.evaluator.EvaluateWithDynamicFields(parsed, accessor, registry)
	if err != nil {
		return false, err
	}

	return result.Matches, nil
}

// isClientDetectionSystemFieldChanged checks if any system-protected field has been changed.
func isClientDetectionSystemFieldChanged(existing, updated *models.ClientDetectionRule) bool {
	return existing.Name != updated.Name ||
		existing.Description != updated.Description ||
		existing.Expression != updated.Expression ||
		existing.Priority != updated.Priority ||
		existing.AcceptedVideoCodecs != updated.AcceptedVideoCodecs ||
		existing.AcceptedAudioCodecs != updated.AcceptedAudioCodecs ||
		existing.PreferredVideoCodec != updated.PreferredVideoCodec ||
		existing.PreferredAudioCodec != updated.PreferredAudioCodec ||
		existing.SupportsFMP4 != updated.SupportsFMP4 ||
		existing.SupportsMPEGTS != updated.SupportsMPEGTS ||
		existing.PreferredFormat != updated.PreferredFormat
}
