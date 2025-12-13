package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
)

// Service-level errors for encoding profiles.
var (
	// ErrEncodingProfileCannotDeleteSystem is returned when trying to delete a system profile.
	ErrEncodingProfileCannotDeleteSystem = errors.New("cannot delete system encoding profile")

	// ErrEncodingProfileCannotEditSystem is returned when trying to edit certain fields of a system profile.
	ErrEncodingProfileCannotEditSystem = errors.New("cannot edit system encoding profile (only enabled toggle allowed)")
)

// EncodingProfileService provides business logic for encoding profiles.
type EncodingProfileService struct {
	repo   repository.EncodingProfileRepository
	logger *slog.Logger
}

// NewEncodingProfileService creates a new encoding profile service.
func NewEncodingProfileService(repo repository.EncodingProfileRepository) *EncodingProfileService {
	return &EncodingProfileService{
		repo:   repo,
		logger: slog.Default(),
	}
}

// WithLogger sets the logger for the service.
func (s *EncodingProfileService) WithLogger(logger *slog.Logger) *EncodingProfileService {
	s.logger = logger
	return s
}

// Create creates a new encoding profile.
func (s *EncodingProfileService) Create(ctx context.Context, profile *models.EncodingProfile) error {
	// System profiles cannot be created via the service (only via migrations)
	profile.IsSystem = false
	return s.repo.Create(ctx, profile)
}

// GetByID retrieves an encoding profile by ID.
func (s *EncodingProfileService) GetByID(ctx context.Context, id models.ULID) (*models.EncodingProfile, error) {
	profile, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, models.ErrEncodingProfileNotFound
	}
	return profile, nil
}

// GetAll retrieves all encoding profiles.
func (s *EncodingProfileService) GetAll(ctx context.Context) ([]*models.EncodingProfile, error) {
	return s.repo.GetAll(ctx)
}

// GetEnabled retrieves all enabled encoding profiles.
func (s *EncodingProfileService) GetEnabled(ctx context.Context) ([]*models.EncodingProfile, error) {
	return s.repo.GetEnabled(ctx)
}

// GetByName retrieves an encoding profile by name.
func (s *EncodingProfileService) GetByName(ctx context.Context, name string) (*models.EncodingProfile, error) {
	profile, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, models.ErrEncodingProfileNotFound
	}
	return profile, nil
}

// GetDefault retrieves the default encoding profile.
func (s *EncodingProfileService) GetDefault(ctx context.Context) (*models.EncodingProfile, error) {
	return s.repo.GetDefault(ctx)
}

// GetSystem retrieves all system encoding profiles.
func (s *EncodingProfileService) GetSystem(ctx context.Context) ([]*models.EncodingProfile, error) {
	return s.repo.GetSystem(ctx)
}

// Update updates an existing encoding profile.
// For system profiles, only the Enabled field can be changed.
func (s *EncodingProfileService) Update(ctx context.Context, profile *models.EncodingProfile) error {
	existing, err := s.repo.GetByID(ctx, profile.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return models.ErrEncodingProfileNotFound
	}

	// System profiles can only have their Enabled field toggled
	if existing.IsSystem {
		if isSystemFieldChanged(existing, profile) {
			return ErrEncodingProfileCannotEditSystem
		}
		// Only allow enabled toggle for system profiles
		existing.Enabled = profile.Enabled
		return s.repo.Update(ctx, existing)
	}

	// Non-system profiles can be fully updated (except IsSystem flag)
	profile.IsSystem = false
	return s.repo.Update(ctx, profile)
}

// Delete deletes an encoding profile by ID.
// System profiles cannot be deleted.
func (s *EncodingProfileService) Delete(ctx context.Context, id models.ULID) error {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return models.ErrEncodingProfileNotFound
	}

	if existing.IsSystem {
		return ErrEncodingProfileCannotDeleteSystem
	}

	return s.repo.Delete(ctx, id)
}

// Count returns the total number of encoding profiles.
func (s *EncodingProfileService) Count(ctx context.Context) (int64, error) {
	return s.repo.Count(ctx)
}

// CountEnabled returns the number of enabled encoding profiles.
func (s *EncodingProfileService) CountEnabled(ctx context.Context) (int64, error) {
	return s.repo.CountEnabled(ctx)
}

// SetDefault sets a profile as the default (unsets previous default).
func (s *EncodingProfileService) SetDefault(ctx context.Context, id models.ULID) error {
	profile, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if profile == nil {
		return models.ErrEncodingProfileNotFound
	}

	return s.repo.SetDefault(ctx, id)
}

// ToggleEnabled toggles the enabled state of a profile.
func (s *EncodingProfileService) ToggleEnabled(ctx context.Context, id models.ULID) (*models.EncodingProfile, error) {
	profile, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, models.ErrEncodingProfileNotFound
	}

	profile.Enabled = !profile.Enabled
	if err := s.repo.Update(ctx, profile); err != nil {
		return nil, err
	}

	return profile, nil
}

// Clone creates a copy of an existing profile with a new name.
func (s *EncodingProfileService) Clone(ctx context.Context, id models.ULID, newName string) (*models.EncodingProfile, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, models.ErrEncodingProfileNotFound
	}

	clone := existing.Clone()
	clone.Name = newName
	clone.Description = "Cloned from " + existing.Name

	if err := s.repo.Create(ctx, clone); err != nil {
		return nil, err
	}

	return clone, nil
}

// isSystemFieldChanged checks if any system-protected field has been changed.
func isSystemFieldChanged(existing, updated *models.EncodingProfile) bool {
	return existing.Name != updated.Name ||
		existing.Description != updated.Description ||
		existing.TargetVideoCodec != updated.TargetVideoCodec ||
		existing.TargetAudioCodec != updated.TargetAudioCodec ||
		existing.QualityPreset != updated.QualityPreset ||
		existing.HWAccel != updated.HWAccel ||
		existing.IsDefault != updated.IsDefault
}
