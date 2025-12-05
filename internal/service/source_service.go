// Package service provides business logic layer for tvarr operations.
package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jmylchreest/tvarr/internal/ingestor"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/jmylchreest/tvarr/internal/service/progress"
	"github.com/jmylchreest/tvarr/pkg/xtream"
)

// EPGChecker checks EPG availability for Xtream sources.
type EPGChecker interface {
	// CheckEPGAvailability checks if an Xtream server provides EPG data.
	CheckEPGAvailability(ctx context.Context, baseURL, username, password string) (bool, error)
}

// DefaultEPGChecker implements EPGChecker using an HTTP HEAD request.
type DefaultEPGChecker struct {
	httpClient *http.Client
}

// NewDefaultEPGChecker creates a new DefaultEPGChecker.
func NewDefaultEPGChecker() *DefaultEPGChecker {
	return &DefaultEPGChecker{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// CheckEPGAvailability checks if an Xtream server provides EPG data via HEAD request to xmltv.php.
func (c *DefaultEPGChecker) CheckEPGAvailability(ctx context.Context, baseURL, username, password string) (bool, error) {
	client := xtream.NewClient(baseURL, username, password, xtream.WithHTTPClient(c.httpClient))
	xmltvURL := client.GetXMLTVURL()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, xmltvURL, nil)
	if err != nil {
		return false, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("checking EPG availability: %w", err)
	}
	defer resp.Body.Close()

	// EPG is available if we get a 2xx response
	return resp.StatusCode >= 200 && resp.StatusCode < 300, nil
}

// SourceService provides business logic for stream source management.
type SourceService struct {
	sourceRepo      repository.StreamSourceRepository
	channelRepo     repository.ChannelRepository
	epgSourceRepo   repository.EpgSourceRepository
	factory         *ingestor.HandlerFactory
	stateManager    *ingestor.StateManager
	progressService *progress.Service
	epgChecker      EPGChecker
	logger          *slog.Logger
	ingestionLocks  sync.Map // map[models.ULID]bool - tracks sources currently being ingested
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

// WithProgressService sets the progress service for progress reporting.
func (s *SourceService) WithProgressService(svc *progress.Service) *SourceService {
	s.progressService = svc
	return s
}

// WithEPGSourceRepo sets the EPG source repository for auto-EPG linking.
func (s *SourceService) WithEPGSourceRepo(repo repository.EpgSourceRepository) *SourceService {
	s.epgSourceRepo = repo
	return s
}

// WithEPGChecker sets the EPG checker for checking EPG availability.
func (s *SourceService) WithEPGChecker(checker EPGChecker) *SourceService {
	s.epgChecker = checker
	return s
}

// getIngestionStages returns the standard stages for stream source ingestion.
// Stream ingestion uses 3 stages:
// - connect: Delete existing channels and prepare for ingestion
// - download: Fetch, parse, and batch-insert channels (main work)
// - finalize: Flush remaining batch and update metadata
func getIngestionStages() []progress.StageInfo {
	return []progress.StageInfo{
		{ID: "connect", Name: "Connecting", Weight: 0.05},
		{ID: "download", Name: "Downloading", Weight: 0.85},
		{ID: "finalize", Name: "Finalizing", Weight: 0.10},
	}
}

// Create creates a new stream source.
// For Xtream sources, it automatically checks for EPG availability and creates
// a linked EPG source if EPG is available and doesn't already exist.
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

	// Auto-create linked EPG source for Xtream sources
	if source.IsXtream() {
		s.tryAutoCreateEPGSource(ctx, source)
	}

	return nil
}

// tryAutoCreateEPGSource attempts to auto-create an EPG source for an Xtream stream source.
// This is a best-effort operation - failures are logged but don't fail the stream source creation.
func (s *SourceService) tryAutoCreateEPGSource(ctx context.Context, streamSource *models.StreamSource) {
	// Skip if no EPG repo or checker configured
	if s.epgSourceRepo == nil || s.epgChecker == nil {
		return
	}

	// Check if EPG source already exists for this URL
	existing, err := s.epgSourceRepo.GetByURL(ctx, streamSource.URL)
	if err != nil {
		s.logger.Warn("failed to check existing EPG source",
			"stream_source_id", streamSource.ID.String(),
			"error", err.Error(),
		)
		return
	}
	if existing != nil {
		s.logger.Debug("EPG source already exists for URL",
			"stream_source_id", streamSource.ID.String(),
			"epg_source_id", existing.ID.String(),
		)
		return
	}

	// Check EPG availability
	available, err := s.epgChecker.CheckEPGAvailability(ctx, streamSource.URL, streamSource.Username, streamSource.Password)
	if err != nil {
		s.logger.Warn("failed to check EPG availability",
			"stream_source_id", streamSource.ID.String(),
			"error", err.Error(),
		)
		return
	}
	if !available {
		s.logger.Debug("EPG not available for Xtream source",
			"stream_source_id", streamSource.ID.String(),
		)
		return
	}

	// Create linked EPG source
	epgSource := &models.EpgSource{
		Name:      fmt.Sprintf("%s (EPG)", streamSource.Name),
		Type:      models.EpgSourceTypeXtream,
		URL:       streamSource.URL,
		Username:  streamSource.Username,
		Password:  streamSource.Password,
		UserAgent: streamSource.UserAgent,
		Enabled:   streamSource.Enabled,
		Priority:  streamSource.Priority,
	}

	if err := s.epgSourceRepo.Create(ctx, epgSource); err != nil {
		s.logger.Warn("failed to auto-create EPG source",
			"stream_source_id", streamSource.ID.String(),
			"error", err.Error(),
		)
		return
	}

	s.logger.Info("auto-created linked EPG source",
		"stream_source_id", streamSource.ID.String(),
		"epg_source_id", epgSource.ID.String(),
		"epg_source_name", epgSource.Name,
	)
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

	// Start progress tracking if service is available
	var progressMgr *progress.OperationManager
	if s.progressService != nil {
		stages := getIngestionStages()
		progressMgr, err = s.progressService.StartOperation(progress.OpStreamIngestion, id, "stream_source", source.Name, stages)
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
	if err := s.sourceRepo.Update(ctx, source); err != nil {
		if progressMgr != nil {
			progressMgr.Fail(err)
		}
		s.stateManager.Fail(id, err)
		return fmt.Errorf("updating source status: %w", err)
	}

	s.logger.Info("starting ingestion",
		"source_id", id.String(),
		"source_name", source.Name,
		"type", source.Type,
	)

	// Perform ingestion within a transaction for atomicity.
	// This ensures delete + all inserts are atomic - if any fails, everything is rolled back.
	var channelCount int
	const batchSize = 1000

	// Stage updaters - declared outside transaction to maintain references
	var connectStage, downloadStage, finalizeStage *progress.StageUpdater

	// Stage 1: Connect
	if progressMgr != nil {
		connectStage = progressMgr.StartStage("connect")
		progressMgr.SetMessage(fmt.Sprintf("Connecting to %s", source.Name))
	}

	err = s.channelRepo.Transaction(ctx, func(txRepo repository.ChannelRepository) error {
		// Delete existing channels within the transaction
		if err := txRepo.DeleteBySourceID(ctx, id); err != nil {
			return fmt.Errorf("deleting existing channels: %w", err)
		}

		// Stage 2: Download - complete connect, start download
		if progressMgr != nil && connectStage != nil {
			connectStage.Complete()
			downloadStage = progressMgr.StartStage("download")
			progressMgr.SetMessage("Downloading channels...")
		}

		// Collect channels in batches
		var batchChannels []*models.Channel

		// Perform ingestion with callback - download, parse, and batch-insert channels
		if err := handler.Ingest(ctx, source, func(channel *models.Channel) error {
			batchChannels = append(batchChannels, channel)
			channelCount++

			// Flush batch when full
			if len(batchChannels) >= batchSize {
				if err := txRepo.UpsertBatch(ctx, batchChannels); err != nil {
					return fmt.Errorf("batch insert: %w", err)
				}
				batchChannels = batchChannels[:0]
			}

			return nil
		}); err != nil {
			return fmt.Errorf("ingesting channels: %w", err)
		}

		// Stage 3: Finalize - flush remaining channels and complete
		if progressMgr != nil && downloadStage != nil {
			downloadStage.Complete()
			finalizeStage = progressMgr.StartStage("finalize")
			progressMgr.SetMessage("Finalizing...")
		}

		if len(batchChannels) > 0 {
			if err := txRepo.UpsertBatch(ctx, batchChannels); err != nil {
				return fmt.Errorf("final batch insert: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		if progressMgr != nil {
			progressMgr.Fail(err)
		}
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

	// Complete progress tracking
	if progressMgr != nil {
		// Complete the finalize stage before finalizing the operation
		if finalizeStage != nil {
			finalizeStage.Complete()
		}
		progressMgr.Complete(fmt.Sprintf("Ingested %d channels", channelCount))
	}

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

	// Atomically check if already ingesting using sync.Map
	// This prevents race conditions where two ingestions start simultaneously
	if _, loaded := s.ingestionLocks.LoadOrStore(id, true); loaded {
		return fmt.Errorf("ingestion already in progress for source %s", id)
	}

	// Also check state manager (for consistency with existing logic)
	if s.stateManager.IsIngesting(id) {
		s.ingestionLocks.Delete(id)
		return fmt.Errorf("ingestion already in progress for source %s", id)
	}

	// Start state tracking immediately
	if err := s.stateManager.Start(source); err != nil {
		s.ingestionLocks.Delete(id)
		return fmt.Errorf("starting state tracking: %w", err)
	}

	// Run ingestion in background
	go func() {
		// Ensure we release the lock when done
		defer s.ingestionLocks.Delete(id)

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

	// Start progress tracking if service is available
	var progressMgr *progress.OperationManager
	if s.progressService != nil {
		stages := getIngestionStages()
		progressMgr, err = s.progressService.StartOperation(progress.OpStreamIngestion, id, "stream_source", source.Name, stages)
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
	if err := s.sourceRepo.Update(ctx, source); err != nil {
		if progressMgr != nil {
			progressMgr.Fail(err)
		}
		s.stateManager.Fail(id, err)
		return
	}

	s.logger.Info("starting async ingestion",
		"source_id", id.String(),
		"source_name", source.Name,
	)

	// Perform ingestion within a transaction for atomicity.
	// This ensures delete + all inserts are atomic - if any fails, everything is rolled back.
	var channelCount int
	const batchSize = 1000

	// Stage updaters - declared outside transaction to maintain references
	var connectStage, downloadStage, finalizeStage *progress.StageUpdater

	// Stage 1: Connect
	if progressMgr != nil {
		connectStage = progressMgr.StartStage("connect")
		progressMgr.SetMessage(fmt.Sprintf("Connecting to %s", source.Name))
	}

	err = s.channelRepo.Transaction(ctx, func(txRepo repository.ChannelRepository) error {
		// Delete existing channels within the transaction
		if err := txRepo.DeleteBySourceID(ctx, id); err != nil {
			return fmt.Errorf("deleting existing channels: %w", err)
		}

		// Stage 2: Download - complete connect, start download
		if progressMgr != nil && connectStage != nil {
			connectStage.Complete()
			downloadStage = progressMgr.StartStage("download")
			progressMgr.SetMessage("Downloading channels...")
		}

		// Collect channels in batches
		var batchChannels []*models.Channel

		// Perform ingestion with callback - download, parse, and batch-insert channels
		if err := handler.Ingest(ctx, source, func(channel *models.Channel) error {
			batchChannels = append(batchChannels, channel)
			channelCount++

			if channelCount%100 == 0 {
				s.stateManager.UpdateProgress(id, channelCount, 0)
				if progressMgr != nil {
					progressMgr.SetMessage(fmt.Sprintf("Downloaded %d channels", channelCount))
				}
			}

			if len(batchChannels) >= batchSize {
				if err := txRepo.UpsertBatch(ctx, batchChannels); err != nil {
					return err
				}
				batchChannels = batchChannels[:0]
			}

			return nil
		}); err != nil {
			return fmt.Errorf("ingesting channels: %w", err)
		}

		// Stage 3: Finalize - flush remaining channels and complete
		if progressMgr != nil && downloadStage != nil {
			downloadStage.Complete()
			finalizeStage = progressMgr.StartStage("finalize")
			progressMgr.SetMessage("Finalizing...")
		}

		if len(batchChannels) > 0 {
			if err := txRepo.UpsertBatch(ctx, batchChannels); err != nil {
				return fmt.Errorf("final batch insert: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		if progressMgr != nil {
			progressMgr.Fail(err)
		}
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

	// Complete progress tracking
	if progressMgr != nil {
		// Complete the finalize stage before finalizing the operation
		if finalizeStage != nil {
			finalizeStage.Complete()
		}
		progressMgr.Complete(fmt.Sprintf("Ingested %d channels from %s", channelCount, source.Name))
	}

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
