// Package scheduler provides job scheduling and execution for tvarr.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
)

// AutoRegenService handles auto-regeneration of proxies when sources are updated.
// It implements the AutoRegenerationTrigger interface.
type AutoRegenService struct {
	proxyRepo repository.StreamProxyRepository
	scheduler *Scheduler
	logger    *slog.Logger
}

// NewAutoRegenService creates a new auto-regeneration service.
func NewAutoRegenService(proxyRepo repository.StreamProxyRepository, scheduler *Scheduler) *AutoRegenService {
	return &AutoRegenService{
		proxyRepo: proxyRepo,
		scheduler: scheduler,
		logger:    slog.Default(),
	}
}

// WithLogger sets a custom logger.
func (s *AutoRegenService) WithLogger(logger *slog.Logger) *AutoRegenService {
	s.logger = logger
	return s
}

// TriggerAutoRegeneration queues proxy regeneration jobs for proxies that use the given source.
// sourceType should be "stream" or "epg".
func (s *AutoRegenService) TriggerAutoRegeneration(ctx context.Context, sourceID models.ULID, sourceType string) error {
	var proxies []*models.StreamProxy
	var err error

	switch sourceType {
	case "stream":
		proxies, err = s.proxyRepo.GetBySourceID(ctx, sourceID)
	case "epg":
		proxies, err = s.proxyRepo.GetByEpgSourceID(ctx, sourceID)
	default:
		return fmt.Errorf("unknown source type: %s", sourceType)
	}

	if err != nil {
		return fmt.Errorf("getting proxies for source %s: %w", sourceID, err)
	}

	if len(proxies) == 0 {
		s.logger.Debug("no proxies use this source",
			slog.String("source_id", sourceID.String()),
			slog.String("source_type", sourceType))
		return nil
	}

	var triggered int
	var skipped int

	for _, proxy := range proxies {
		// Only trigger for proxies with AutoRegenerate enabled
		if !proxy.AutoRegenerate {
			s.logger.Debug("skipping proxy without auto-regenerate",
				slog.String("proxy_id", proxy.ID.String()),
				slog.String("proxy_name", proxy.Name))
			skipped++
			continue
		}

		// Schedule immediate regeneration (deduplication handled by scheduler)
		job, err := s.scheduler.ScheduleImmediate(ctx, models.JobTypeProxyGeneration, proxy.ID, proxy.Name)
		if err != nil {
			s.logger.Error("failed to schedule proxy regeneration",
				slog.String("proxy_id", proxy.ID.String()),
				slog.String("proxy_name", proxy.Name),
				slog.Any("error", err))
			continue
		}

		s.logger.Info("queued auto-regeneration for proxy",
			slog.String("proxy_id", proxy.ID.String()),
			slog.String("proxy_name", proxy.Name),
			slog.String("job_id", job.ID.String()),
			slog.String("source_id", sourceID.String()),
			slog.String("source_type", sourceType))
		triggered++
	}

	s.logger.Info("auto-regeneration trigger completed",
		slog.String("source_id", sourceID.String()),
		slog.String("source_type", sourceType),
		slog.Int("proxies_found", len(proxies)),
		slog.Int("triggered", triggered),
		slog.Int("skipped", skipped))

	return nil
}

// Ensure AutoRegenService implements AutoRegenerationTrigger at compile time.
var _ AutoRegenerationTrigger = (*AutoRegenService)(nil)
