// Package service provides business logic layer for tvarr operations.
package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jmylchreest/tvarr/internal/ingestor"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
)

// SourceService provides business logic for stream source management.
type SourceService struct {
	sourceRepo   repository.StreamSourceRepository
	channelRepo  repository.ChannelRepository
	factory      *ingestor.HandlerFactory
	stateManager *ingestor.StateManager
	logger       *slog.Logger
}

// NewSourceService creates a new source service.
func NewSourceService(
	sourceRepo repository.StreamSourceRepository,
	channelRepo repository.ChannelRepository,
	factory *ingestor.HandlerFactory,
	stateManager *ingestor.StateManager,
) *SourceService {
	return &SourceService{
		sourceRepo:   sourceRepo,
		channelRepo:  channelRepo,
		factory:      factory,
		stateManager: stateManager,
		logger:       slog.Default(),
	}
}

// WithLogger sets the logger for the service.
func (s *SourceService) WithLogger(logger *slog.Logger) *SourceService {
	s.logger = logger
	return s
}

// Create creates a new stream source.
func (s *SourceService) Create(ctx context.Context, source *models.StreamSource) error {
	if err := source.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if err := s.sourceRepo.Create(ctx, source); err != nil {
		return fmt.Errorf("creating source: %w", err)
	}

	s.logger.Info("created stream source",
		"id", source.ID.String(),
		"name", source.Name,
		"type", source.Type,
	)

	return nil
}

// Update updates an existing stream source.
func (s *SourceService) Update(ctx context.Context, source *models.StreamSource) error {
	if err := source.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if err := s.sourceRepo.Update(ctx, source); err != nil {
		return fmt.Errorf("updating source: %w", err)
	}

	s.logger.Info("updated stream source",
		"id", source.ID.String(),
		"name", source.Name,
	)

	return nil
}

// Delete deletes a stream source and all its channels.
func (s *SourceService) Delete(ctx context.Context, id models.ULID) error {
	// First delete all channels for this source
	if err := s.channelRepo.DeleteBySourceID(ctx, id); err != nil {
		return fmt.Errorf("deleting channels: %w", err)
	}

	// Then delete the source
	if err := s.sourceRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("deleting source: %w", err)
	}

	s.logger.Info("deleted stream source", "id", id.String())

	return nil
}

// GetByID retrieves a stream source by ID.
func (s *SourceService) GetByID(ctx context.Context, id models.ULID) (*models.StreamSource, error) {
	source, err := s.sourceRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("getting source: %w", err)
	}
	return source, nil
}

// GetByName retrieves a stream source by name.
func (s *SourceService) GetByName(ctx context.Context, name string) (*models.StreamSource, error) {
	source, err := s.sourceRepo.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("getting source by name: %w", err)
	}
	return source, nil
}

// List returns all stream sources.
func (s *SourceService) List(ctx context.Context) ([]*models.StreamSource, error) {
	sources, err := s.sourceRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing sources: %w", err)
	}
	return sources, nil
}

// ListEnabled returns all enabled stream sources.
func (s *SourceService) ListEnabled(ctx context.Context) ([]*models.StreamSource, error) {
	sources, err := s.sourceRepo.GetEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing enabled sources: %w", err)
	}
	return sources, nil
}

// Ingest triggers ingestion for a stream source.
func (s *SourceService) Ingest(ctx context.Context, id models.ULID) error {
	// Get the source
	source, err := s.sourceRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("getting source: %w", err)
	}

	// Check if already ingesting
	if s.stateManager.IsIngesting(id) {
		return fmt.Errorf("ingestion already in progress for source %s", id)
	}

	// Get the appropriate handler
	handler, err := s.factory.GetForSource(source)
	if err != nil {
		return fmt.Errorf("getting handler: %w", err)
	}

	// Start state tracking
	if err := s.stateManager.Start(source); err != nil {
		return fmt.Errorf("starting state tracking: %w", err)
	}

	// Mark source as ingesting
	source.MarkIngesting()
	if err := s.sourceRepo.Update(ctx, source); err != nil {
		s.stateManager.Fail(id, err)
		return fmt.Errorf("updating source status: %w", err)
	}

	s.logger.Info("starting ingestion",
		"source_id", id.String(),
		"source_name", source.Name,
		"type", source.Type,
	)

	// Delete existing channels before re-ingesting
	if err := s.channelRepo.DeleteBySourceID(ctx, id); err != nil {
		s.stateManager.Fail(id, err)
		source.MarkFailed(err)
		_ = s.sourceRepo.Update(ctx, source)
		return fmt.Errorf("deleting existing channels: %w", err)
	}

	// Perform ingestion
	var channelCount int
	var batchChannels []*models.Channel
	const batchSize = 1000

	err = handler.Ingest(ctx, source, func(channel *models.Channel) error {
		batchChannels = append(batchChannels, channel)
		channelCount++

		// Update progress periodically
		if channelCount%100 == 0 {
			s.stateManager.UpdateProgress(id, channelCount, 0)
		}

		// Flush batch when full
		if len(batchChannels) >= batchSize {
			if err := s.channelRepo.CreateBatch(ctx, batchChannels); err != nil {
				return fmt.Errorf("batch insert: %w", err)
			}
			batchChannels = batchChannels[:0]
		}

		return nil
	})

	// Flush remaining channels
	if len(batchChannels) > 0 {
		if batchErr := s.channelRepo.CreateBatch(ctx, batchChannels); batchErr != nil {
			if err == nil {
				err = fmt.Errorf("final batch insert: %w", batchErr)
			}
		}
	}

	if err != nil {
		s.stateManager.Fail(id, err)
		source.MarkFailed(err)
		_ = s.sourceRepo.Update(ctx, source)
		s.logger.Error("ingestion failed",
			"source_id", id.String(),
			"error", err,
		)
		return fmt.Errorf("ingestion failed: %w", err)
	}

	// Mark success
	source.MarkSuccess(channelCount)
	if err := s.sourceRepo.Update(ctx, source); err != nil {
		s.logger.Error("failed to update source status",
			"source_id", id.String(),
			"error", err,
		)
	}

	s.stateManager.Complete(id, channelCount)

	s.logger.Info("ingestion completed",
		"source_id", id.String(),
		"source_name", source.Name,
		"channel_count", channelCount,
	)

	return nil
}

// IngestAsync triggers ingestion asynchronously.
func (s *SourceService) IngestAsync(ctx context.Context, id models.ULID) error {
	// Verify source exists
	source, err := s.sourceRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("getting source: %w", err)
	}

	// Check if already ingesting
	if s.stateManager.IsIngesting(id) {
		return fmt.Errorf("ingestion already in progress for source %s", id)
	}

	// Start state tracking immediately
	if err := s.stateManager.Start(source); err != nil {
		return fmt.Errorf("starting state tracking: %w", err)
	}

	// Run ingestion in background
	go func() {
		// Create a new context that isn't tied to the request
		bgCtx := context.Background()

		// Perform the actual ingestion (state already started)
		s.performIngestion(bgCtx, source)
	}()

	return nil
}

// performIngestion performs the actual ingestion work.
// Assumes state tracking has already been started.
func (s *SourceService) performIngestion(ctx context.Context, source *models.StreamSource) {
	id := source.ID

	// Get the appropriate handler
	handler, err := s.factory.GetForSource(source)
	if err != nil {
		s.stateManager.Fail(id, err)
		return
	}

	// Mark source as ingesting
	source.MarkIngesting()
	if err := s.sourceRepo.Update(ctx, source); err != nil {
		s.stateManager.Fail(id, err)
		return
	}

	s.logger.Info("starting async ingestion",
		"source_id", id.String(),
		"source_name", source.Name,
	)

	// Delete existing channels
	if err := s.channelRepo.DeleteBySourceID(ctx, id); err != nil {
		s.stateManager.Fail(id, err)
		source.MarkFailed(err)
		_ = s.sourceRepo.Update(ctx, source)
		return
	}

	// Perform ingestion
	var channelCount int
	var batchChannels []*models.Channel
	const batchSize = 1000

	err = handler.Ingest(ctx, source, func(channel *models.Channel) error {
		batchChannels = append(batchChannels, channel)
		channelCount++

		if channelCount%100 == 0 {
			s.stateManager.UpdateProgress(id, channelCount, 0)
		}

		if len(batchChannels) >= batchSize {
			if err := s.channelRepo.CreateBatch(ctx, batchChannels); err != nil {
				return err
			}
			batchChannels = batchChannels[:0]
		}

		return nil
	})

	// Flush remaining
	if len(batchChannels) > 0 {
		if batchErr := s.channelRepo.CreateBatch(ctx, batchChannels); batchErr != nil {
			if err == nil {
				err = batchErr
			}
		}
	}

	if err != nil {
		s.stateManager.Fail(id, err)
		source.MarkFailed(err)
		_ = s.sourceRepo.Update(ctx, source)
		s.logger.Error("async ingestion failed",
			"source_id", id.String(),
			"error", err,
		)
		return
	}

	source.MarkSuccess(channelCount)
	_ = s.sourceRepo.Update(ctx, source)
	s.stateManager.Complete(id, channelCount)

	s.logger.Info("async ingestion completed",
		"source_id", id.String(),
		"channel_count", channelCount,
	)
}

// GetIngestionState returns the current ingestion state for a source.
func (s *SourceService) GetIngestionState(id models.ULID) (*ingestor.IngestionState, bool) {
	return s.stateManager.GetState(id)
}

// IsIngesting returns true if an ingestion is in progress for the source.
func (s *SourceService) IsIngesting(id models.ULID) bool {
	return s.stateManager.IsIngesting(id)
}

// GetAllIngestionStates returns all current ingestion states.
func (s *SourceService) GetAllIngestionStates() []*ingestor.IngestionState {
	return s.stateManager.GetAllStates()
}

// GetChannelCount returns the number of channels for a source.
func (s *SourceService) GetChannelCount(ctx context.Context, id models.ULID) (int64, error) {
	return s.channelRepo.CountBySourceID(ctx, id)
}
