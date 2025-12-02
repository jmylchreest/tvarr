// Package service provides business logic layer for tvarr operations.
package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jmylchreest/tvarr/internal/ingestor"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/jmylchreest/tvarr/internal/service/progress"
)

// EpgService provides business logic for EPG source management.
type EpgService struct {
	epgSourceRepo   repository.EpgSourceRepository
	epgProgramRepo  repository.EpgProgramRepository
	factory         *ingestor.EpgHandlerFactory
	stateManager    *ingestor.StateManager
	progressService *progress.Service
	logger          *slog.Logger
}

// NewEpgService creates a new EPG service.
func NewEpgService(
	epgSourceRepo repository.EpgSourceRepository,
	epgProgramRepo repository.EpgProgramRepository,
	factory *ingestor.EpgHandlerFactory,
	stateManager *ingestor.StateManager,
) *EpgService {
	return &EpgService{
		epgSourceRepo:  epgSourceRepo,
		epgProgramRepo: epgProgramRepo,
		factory:        factory,
		stateManager:   stateManager,
		logger:         slog.Default(),
	}
}

// WithLogger sets the logger for the service.
func (s *EpgService) WithLogger(logger *slog.Logger) *EpgService {
	s.logger = logger
	return s
}

// WithProgressService sets the progress service for progress reporting.
func (s *EpgService) WithProgressService(svc *progress.Service) *EpgService {
	s.progressService = svc
	return s
}

// getEpgIngestionStages returns the standard stages for EPG source ingestion.
func getEpgIngestionStages() []progress.StageInfo {
	return []progress.StageInfo{
		{ID: "connect", Name: "Connecting", Weight: 0.1},
		{ID: "download", Name: "Downloading", Weight: 0.3},
		{ID: "process", Name: "Processing", Weight: 0.4},
		{ID: "save", Name: "Saving", Weight: 0.2},
	}
}

// Create creates a new EPG source.
func (s *EpgService) Create(ctx context.Context, source *models.EpgSource) error {
	if err := source.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if err := s.epgSourceRepo.Create(ctx, source); err != nil {
		return fmt.Errorf("creating EPG source: %w", err)
	}

	s.logger.Info("created EPG source",
		"id", source.ID.String(),
		"name", source.Name,
		"type", source.Type,
	)

	return nil
}

// Update updates an existing EPG source.
func (s *EpgService) Update(ctx context.Context, source *models.EpgSource) error {
	if err := source.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if err := s.epgSourceRepo.Update(ctx, source); err != nil {
		return fmt.Errorf("updating EPG source: %w", err)
	}

	s.logger.Info("updated EPG source",
		"id", source.ID.String(),
		"name", source.Name,
	)

	return nil
}

// Delete deletes an EPG source and all its programs.
func (s *EpgService) Delete(ctx context.Context, id models.ULID) error {
	// First delete all programs for this source
	if err := s.epgProgramRepo.DeleteBySourceID(ctx, id); err != nil {
		return fmt.Errorf("deleting programs: %w", err)
	}

	// Then delete the source
	if err := s.epgSourceRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("deleting EPG source: %w", err)
	}

	s.logger.Info("deleted EPG source", "id", id.String())

	return nil
}

// GetByID retrieves an EPG source by ID.
func (s *EpgService) GetByID(ctx context.Context, id models.ULID) (*models.EpgSource, error) {
	source, err := s.epgSourceRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("getting EPG source: %w", err)
	}
	return source, nil
}

// GetByName retrieves an EPG source by name.
func (s *EpgService) GetByName(ctx context.Context, name string) (*models.EpgSource, error) {
	source, err := s.epgSourceRepo.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("getting EPG source by name: %w", err)
	}
	return source, nil
}

// List returns all EPG sources.
func (s *EpgService) List(ctx context.Context) ([]*models.EpgSource, error) {
	sources, err := s.epgSourceRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing EPG sources: %w", err)
	}
	return sources, nil
}

// ListEnabled returns all enabled EPG sources.
func (s *EpgService) ListEnabled(ctx context.Context) ([]*models.EpgSource, error) {
	sources, err := s.epgSourceRepo.GetEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing enabled EPG sources: %w", err)
	}
	return sources, nil
}

// Ingest triggers ingestion for an EPG source.
func (s *EpgService) Ingest(ctx context.Context, id models.ULID) error {
	// Get the source
	source, err := s.epgSourceRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("getting EPG source: %w", err)
	}

	// Check if already ingesting - use the EPG source's own ID (ULIDs are globally unique)
	if s.stateManager.IsIngesting(id) {
		return fmt.Errorf("ingestion already in progress for EPG source %s", id)
	}

	// Get the appropriate handler
	handler, err := s.factory.GetForSource(source)
	if err != nil {
		return fmt.Errorf("getting EPG handler: %w", err)
	}

	// Start state tracking using the EPG source's ID directly
	if err := s.stateManager.StartWithID(id, source.Name); err != nil {
		return fmt.Errorf("starting state tracking: %w", err)
	}

	// Start progress tracking if service is available
	var progressMgr *progress.OperationManager
	if s.progressService != nil {
		stages := getEpgIngestionStages()
		progressMgr, err = s.progressService.StartOperation(progress.OpEpgIngestion, id, "epg_source", stages)
		if err != nil {
			// Log but don't fail - progress tracking is non-essential
			s.logger.Warn("failed to start progress tracking",
				"source_id", id.String(),
				"error", err.Error(),
			)
			progressMgr = nil
		}
	}

	// Mark source as ingesting
	source.MarkIngesting()
	if err := s.epgSourceRepo.Update(ctx, source); err != nil {
		if progressMgr != nil {
			progressMgr.Fail(err)
		}
		s.stateManager.Fail(id, err)
		return fmt.Errorf("updating EPG source status: %w", err)
	}

	s.logger.Info("starting EPG ingestion",
		"source_id", id.String(),
		"source_name", source.Name,
		"type", source.Type,
	)

	// Stage 1: Connect
	if progressMgr != nil {
		progressMgr.StartStage("connect")
	}

	// Delete existing programs before re-ingesting
	if err := s.epgProgramRepo.DeleteBySourceID(ctx, id); err != nil {
		if progressMgr != nil {
			progressMgr.Fail(err)
		}
		s.stateManager.Fail(id, err)
		source.MarkFailed(err)
		_ = s.epgSourceRepo.Update(ctx, source)
		return fmt.Errorf("deleting existing programs: %w", err)
	}

	// Stage 2: Download
	if progressMgr != nil {
		connectStage := progressMgr.StartStage("connect")
		connectStage.Complete()
		progressMgr.StartStage("download")
	}

	// Perform ingestion
	var programCount int
	var batchPrograms []*models.EpgProgram
	const batchSize = 1000

	err = handler.Ingest(ctx, source, func(program *models.EpgProgram) error {
		batchPrograms = append(batchPrograms, program)
		programCount++

		// Update progress periodically
		if programCount%500 == 0 {
			s.stateManager.UpdateProgress(id, programCount, 0)
			if progressMgr != nil {
				progressMgr.SetMessage(fmt.Sprintf("Downloaded %d programs", programCount))
			}
		}

		// Flush batch when full
		if len(batchPrograms) >= batchSize {
			if err := s.epgProgramRepo.CreateBatch(ctx, batchPrograms); err != nil {
				return fmt.Errorf("batch insert: %w", err)
			}
			batchPrograms = batchPrograms[:0]
		}

		return nil
	})

	// Stage 3: Process
	if progressMgr != nil {
		downloadStage := progressMgr.StartStage("download")
		downloadStage.Complete()
		progressMgr.StartStage("process")
	}

	// Stage 4: Save - flush remaining programs
	if progressMgr != nil {
		processStage := progressMgr.StartStage("process")
		processStage.Complete()
		progressMgr.StartStage("save")
	}

	if len(batchPrograms) > 0 {
		if batchErr := s.epgProgramRepo.CreateBatch(ctx, batchPrograms); batchErr != nil {
			if err == nil {
				err = fmt.Errorf("final batch insert: %w", batchErr)
			}
		}
	}

	if err != nil {
		if progressMgr != nil {
			progressMgr.Fail(err)
		}
		s.stateManager.Fail(id, err)
		source.MarkFailed(err)
		_ = s.epgSourceRepo.Update(ctx, source)
		s.logger.Error("EPG ingestion failed",
			"source_id", id.String(),
			"error", err,
		)
		return fmt.Errorf("EPG ingestion failed: %w", err)
	}

	// Mark success
	source.MarkSuccess(programCount)
	if err := s.epgSourceRepo.Update(ctx, source); err != nil {
		s.logger.Error("failed to update EPG source status",
			"source_id", id.String(),
			"error", err,
		)
	}

	s.stateManager.Complete(id, programCount)

	// Complete progress tracking
	if progressMgr != nil {
		progressMgr.Complete(fmt.Sprintf("Ingested %d programs", programCount))
	}

	s.logger.Info("EPG ingestion completed",
		"source_id", id.String(),
		"source_name", source.Name,
		"program_count", programCount,
	)

	return nil
}

// IngestAsync triggers EPG ingestion asynchronously.
func (s *EpgService) IngestAsync(ctx context.Context, id models.ULID) error {
	// Verify source exists
	source, err := s.epgSourceRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("getting EPG source: %w", err)
	}

	// Check if already ingesting
	if s.stateManager.IsIngesting(id) {
		return fmt.Errorf("ingestion already in progress for EPG source %s", id)
	}

	// Start state tracking immediately
	if err := s.stateManager.StartWithID(id, source.Name); err != nil {
		return fmt.Errorf("starting state tracking: %w", err)
	}

	// Run ingestion in background
	go func() {
		bgCtx := context.Background()
		s.performIngestion(bgCtx, source)
	}()

	return nil
}

// performIngestion performs the actual EPG ingestion work.
func (s *EpgService) performIngestion(ctx context.Context, source *models.EpgSource) {
	id := source.ID

	// Get the appropriate handler
	handler, err := s.factory.GetForSource(source)
	if err != nil {
		s.stateManager.Fail(id, err)
		return
	}

	// Start progress tracking if service is available
	var progressMgr *progress.OperationManager
	if s.progressService != nil {
		stages := getEpgIngestionStages()
		progressMgr, err = s.progressService.StartOperation(progress.OpEpgIngestion, id, "epg_source", stages)
		if err != nil {
			s.logger.Warn("failed to start progress tracking",
				"source_id", id.String(),
				"error", err.Error(),
			)
			progressMgr = nil
		}
	}

	// Mark source as ingesting
	source.MarkIngesting()
	if err := s.epgSourceRepo.Update(ctx, source); err != nil {
		if progressMgr != nil {
			progressMgr.Fail(err)
		}
		s.stateManager.Fail(id, err)
		return
	}

	s.logger.Info("starting async EPG ingestion",
		"source_id", id.String(),
		"source_name", source.Name,
	)

	// Stage 1: Connect
	if progressMgr != nil {
		progressMgr.StartStage("connect")
		progressMgr.SetMessage(fmt.Sprintf("Connecting to %s", source.Name))
	}

	// Delete existing programs
	if err := s.epgProgramRepo.DeleteBySourceID(ctx, id); err != nil {
		if progressMgr != nil {
			progressMgr.Fail(err)
		}
		s.stateManager.Fail(id, err)
		source.MarkFailed(err)
		_ = s.epgSourceRepo.Update(ctx, source)
		return
	}

	// Stage 2: Download
	if progressMgr != nil {
		connectStage := progressMgr.StartStage("connect")
		connectStage.Complete()
		progressMgr.StartStage("download")
		progressMgr.SetMessage("Downloading EPG data...")
	}

	// Perform ingestion
	var programCount int
	var batchPrograms []*models.EpgProgram
	const batchSize = 1000

	err = handler.Ingest(ctx, source, func(program *models.EpgProgram) error {
		batchPrograms = append(batchPrograms, program)
		programCount++

		if programCount%500 == 0 {
			s.stateManager.UpdateProgress(id, programCount, 0)
			if progressMgr != nil {
				progressMgr.SetMessage(fmt.Sprintf("Downloaded %d programs", programCount))
			}
		}

		if len(batchPrograms) >= batchSize {
			if err := s.epgProgramRepo.CreateBatch(ctx, batchPrograms); err != nil {
				return err
			}
			batchPrograms = batchPrograms[:0]
		}

		return nil
	})

	// Stage 3: Process
	if progressMgr != nil {
		downloadStage := progressMgr.StartStage("download")
		downloadStage.Complete()
		progressMgr.StartStage("process")
		progressMgr.SetMessage("Processing EPG data...")
	}

	// Stage 4: Save - flush remaining
	if progressMgr != nil {
		processStage := progressMgr.StartStage("process")
		processStage.Complete()
		progressMgr.StartStage("save")
		progressMgr.SetMessage("Saving programs to database...")
	}

	if len(batchPrograms) > 0 {
		if batchErr := s.epgProgramRepo.CreateBatch(ctx, batchPrograms); batchErr != nil {
			if err == nil {
				err = batchErr
			}
		}
	}

	if err != nil {
		if progressMgr != nil {
			progressMgr.Fail(err)
		}
		s.stateManager.Fail(id, err)
		source.MarkFailed(err)
		_ = s.epgSourceRepo.Update(ctx, source)
		s.logger.Error("async EPG ingestion failed",
			"source_id", id.String(),
			"error", err,
		)
		return
	}

	source.MarkSuccess(programCount)
	_ = s.epgSourceRepo.Update(ctx, source)
	s.stateManager.Complete(id, programCount)

	// Complete progress tracking
	if progressMgr != nil {
		progressMgr.Complete(fmt.Sprintf("Ingested %d programs from %s", programCount, source.Name))
	}

	s.logger.Info("async EPG ingestion completed",
		"source_id", id.String(),
		"program_count", programCount,
	)
}

// GetIngestionState returns the current ingestion state for an EPG source.
func (s *EpgService) GetIngestionState(id models.ULID) (*ingestor.IngestionState, bool) {
	return s.stateManager.GetState(id)
}

// IsIngesting returns true if an ingestion is in progress for the EPG source.
func (s *EpgService) IsIngesting(id models.ULID) bool {
	return s.stateManager.IsIngesting(id)
}

// GetProgramCount returns the number of programs for an EPG source.
func (s *EpgService) GetProgramCount(ctx context.Context, id models.ULID) (int64, error) {
	return s.epgProgramRepo.CountBySourceID(ctx, id)
}

// GetProgramsForChannel retrieves programs for a channel with a limit.
func (s *EpgService) GetProgramsForChannel(ctx context.Context, channelID string, limit int) ([]*models.EpgProgram, error) {
	return s.epgProgramRepo.GetByChannelIDWithLimit(ctx, channelID, limit)
}

// GetCurrentProgram retrieves the currently airing program for a channel.
func (s *EpgService) GetCurrentProgram(ctx context.Context, channelID string) (*models.EpgProgram, error) {
	return s.epgProgramRepo.GetCurrentByChannelID(ctx, channelID)
}

// DeleteOldPrograms removes programs older than the specified threshold.
func (s *EpgService) DeleteOldPrograms(ctx context.Context) (int64, error) {
	count, err := s.epgProgramRepo.DeleteOld(ctx)
	if err != nil {
		return 0, fmt.Errorf("deleting old programs: %w", err)
	}

	if count > 0 {
		s.logger.Info("deleted old EPG programs", "count", count)
	}

	return count, nil
}
