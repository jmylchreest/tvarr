package service

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
)

// Service-level errors for encoder overrides.
var (
	// ErrEncoderOverrideCannotDeleteSystem is returned when trying to delete a system override.
	ErrEncoderOverrideCannotDeleteSystem = errors.New("cannot delete system encoder override")

	// ErrEncoderOverrideCannotEditSystem is returned when trying to edit certain fields of a system override.
	ErrEncoderOverrideCannotEditSystem = errors.New("cannot edit system encoder override (only enabled toggle allowed)")

	// ErrEncoderOverrideNotFound is returned when an override is not found.
	ErrEncoderOverrideNotFound = errors.New("encoder override not found")
)

// EncoderOverrideService provides business logic for encoder overrides.
type EncoderOverrideService struct {
	repo   repository.EncoderOverrideRepository
	logger *slog.Logger

	// Cache for enabled overrides (refreshed when overrides change)
	mu              sync.RWMutex
	cachedOverrides []*models.EncoderOverride
}

// NewEncoderOverrideService creates a new encoder override service.
func NewEncoderOverrideService(repo repository.EncoderOverrideRepository) *EncoderOverrideService {
	return &EncoderOverrideService{
		repo:   repo,
		logger: slog.Default(),
	}
}

// WithLogger sets the logger for the service.
func (s *EncoderOverrideService) WithLogger(logger *slog.Logger) *EncoderOverrideService {
	s.logger = logger
	return s
}

// RefreshCache refreshes the cached enabled overrides.
// Call this on startup and when overrides change.
func (s *EncoderOverrideService) RefreshCache(ctx context.Context) error {
	overrides, err := s.repo.GetEnabled(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.cachedOverrides = overrides
	s.mu.Unlock()

	s.logger.Debug("encoder overrides cache refreshed",
		slog.Int("override_count", len(overrides)),
	)
	return nil
}

// GetEnabledProto returns the cached enabled overrides converted to proto format.
// This is used as the EncoderOverridesProvider for the TranscoderFactory.
func (s *EncoderOverrideService) GetEnabledProto() []*proto.EncoderOverride {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.cachedOverrides == nil {
		return nil
	}

	result := make([]*proto.EncoderOverride, 0, len(s.cachedOverrides))
	for _, override := range s.cachedOverrides {
		result = append(result, &proto.EncoderOverride{
			CodecType:     string(override.CodecType),
			SourceCodec:   override.SourceCodec,
			TargetEncoder: override.TargetEncoder,
			HwAccelMatch:  override.HWAccelMatch,
			CpuMatch:      override.CPUMatch,
			Priority:      int32(override.Priority),
		})
	}
	return result
}

// Create creates a new encoder override.
// Priority is auto-assigned as the next available value (max + 1, starting at 1).
func (s *EncoderOverrideService) Create(ctx context.Context, override *models.EncoderOverride) error {
	// System overrides cannot be created via the service (only via migrations)
	override.IsSystem = false

	// Auto-assign priority as next available value
	allOverrides, err := s.repo.GetAll(ctx)
	if err != nil {
		return err
	}
	maxPriority := 0
	for _, o := range allOverrides {
		if o.Priority > maxPriority {
			maxPriority = o.Priority
		}
	}
	override.Priority = maxPriority + 1

	if err := s.repo.Create(ctx, override); err != nil {
		return err
	}

	// Refresh cache after create
	_ = s.RefreshCache(ctx)
	return nil
}

// GetByID retrieves an encoder override by ID.
func (s *EncoderOverrideService) GetByID(ctx context.Context, id models.ULID) (*models.EncoderOverride, error) {
	override, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if override == nil {
		return nil, ErrEncoderOverrideNotFound
	}
	return override, nil
}

// GetAll retrieves all encoder overrides.
func (s *EncoderOverrideService) GetAll(ctx context.Context) ([]*models.EncoderOverride, error) {
	return s.repo.GetAll(ctx)
}

// GetEnabled retrieves all enabled encoder overrides.
func (s *EncoderOverrideService) GetEnabled(ctx context.Context) ([]*models.EncoderOverride, error) {
	return s.repo.GetEnabled(ctx)
}

// GetByCodecType retrieves overrides for a specific codec type.
func (s *EncoderOverrideService) GetByCodecType(ctx context.Context, codecType models.EncoderOverrideCodecType) ([]*models.EncoderOverride, error) {
	return s.repo.GetByCodecType(ctx, codecType)
}

// GetSystem retrieves all system overrides.
func (s *EncoderOverrideService) GetSystem(ctx context.Context) ([]*models.EncoderOverride, error) {
	return s.repo.GetSystem(ctx)
}

// Update updates an existing override.
// For system overrides, only the IsEnabled field can be changed.
func (s *EncoderOverrideService) Update(ctx context.Context, override *models.EncoderOverride) error {
	existing, err := s.repo.GetByID(ctx, override.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrEncoderOverrideNotFound
	}

	// System overrides can only have their IsEnabled field toggled
	if existing.IsSystem {
		if isEncoderOverrideSystemFieldChanged(existing, override) {
			return ErrEncoderOverrideCannotEditSystem
		}
		// Only allow enabled toggle for system overrides
		existing.IsEnabled = override.IsEnabled
		if err := s.repo.Update(ctx, existing); err != nil {
			return err
		}
	} else {
		// Non-system overrides can be fully updated (except IsSystem flag)
		override.IsSystem = false
		if err := s.repo.Update(ctx, override); err != nil {
			return err
		}
	}

	// Refresh cache after update
	_ = s.RefreshCache(ctx)
	return nil
}

// Delete deletes an override by ID.
// System overrides cannot be deleted.
func (s *EncoderOverrideService) Delete(ctx context.Context, id models.ULID) error {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrEncoderOverrideNotFound
	}

	if existing.IsSystem {
		return ErrEncoderOverrideCannotDeleteSystem
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	// Refresh cache after delete
	_ = s.RefreshCache(ctx)
	return nil
}

// Reorder updates priorities for multiple overrides.
func (s *EncoderOverrideService) Reorder(ctx context.Context, reorders []repository.ReorderRequest) error {
	if err := s.repo.Reorder(ctx, reorders); err != nil {
		return err
	}

	// Refresh cache after reorder
	_ = s.RefreshCache(ctx)
	return nil
}

// ToggleEnabled toggles the enabled state of an override.
func (s *EncoderOverrideService) ToggleEnabled(ctx context.Context, id models.ULID) (*models.EncoderOverride, error) {
	override, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if override == nil {
		return nil, ErrEncoderOverrideNotFound
	}

	newVal := !models.BoolVal(override.IsEnabled)
	override.IsEnabled = &newVal
	if err := s.repo.Update(ctx, override); err != nil {
		return nil, err
	}

	// Refresh cache after toggle
	_ = s.RefreshCache(ctx)
	return override, nil
}

// isEncoderOverrideSystemFieldChanged checks if any system-protected field has been changed.
func isEncoderOverrideSystemFieldChanged(existing, updated *models.EncoderOverride) bool {
	return existing.Name != updated.Name ||
		existing.Description != updated.Description ||
		existing.CodecType != updated.CodecType ||
		existing.SourceCodec != updated.SourceCodec ||
		existing.TargetEncoder != updated.TargetEncoder ||
		existing.HWAccelMatch != updated.HWAccelMatch ||
		existing.CPUMatch != updated.CPUMatch ||
		existing.Priority != updated.Priority
}
