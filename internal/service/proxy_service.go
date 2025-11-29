package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/pipeline"
	"github.com/jmylchreest/tvarr/internal/repository"
)

// ProxyService provides business logic for stream proxy management.
type ProxyService struct {
	proxyRepo       repository.StreamProxyRepository
	pipelineFactory pipeline.OrchestratorFactory
	logger          *slog.Logger
}

// NewProxyService creates a new proxy service.
func NewProxyService(
	proxyRepo repository.StreamProxyRepository,
	pipelineFactory pipeline.OrchestratorFactory,
) *ProxyService {
	return &ProxyService{
		proxyRepo:       proxyRepo,
		pipelineFactory: pipelineFactory,
		logger:          slog.Default(),
	}
}

// WithLogger sets the logger for the service.
func (s *ProxyService) WithLogger(logger *slog.Logger) *ProxyService {
	s.logger = logger
	return s
}

// Create creates a new stream proxy.
func (s *ProxyService) Create(ctx context.Context, proxy *models.StreamProxy) error {
	if err := proxy.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if err := s.proxyRepo.Create(ctx, proxy); err != nil {
		return fmt.Errorf("creating proxy: %w", err)
	}

	s.logger.InfoContext(ctx, "created stream proxy",
		slog.String("id", proxy.ID.String()),
		slog.String("name", proxy.Name),
		slog.Bool("is_active", proxy.IsActive),
	)

	return nil
}

// Update updates an existing stream proxy.
func (s *ProxyService) Update(ctx context.Context, proxy *models.StreamProxy) error {
	if err := proxy.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if err := s.proxyRepo.Update(ctx, proxy); err != nil {
		return fmt.Errorf("updating proxy: %w", err)
	}

	s.logger.InfoContext(ctx, "updated stream proxy",
		slog.String("id", proxy.ID.String()),
		slog.String("name", proxy.Name),
	)

	return nil
}

// Delete deletes a stream proxy by ID.
func (s *ProxyService) Delete(ctx context.Context, id models.ULID) error {
	if err := s.proxyRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("deleting proxy: %w", err)
	}

	s.logger.InfoContext(ctx, "deleted stream proxy",
		slog.String("id", id.String()),
	)

	return nil
}

// GetByID retrieves a stream proxy by ID.
func (s *ProxyService) GetByID(ctx context.Context, id models.ULID) (*models.StreamProxy, error) {
	proxy, err := s.proxyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("getting proxy: %w", err)
	}
	return proxy, nil
}

// GetByIDWithRelations retrieves a stream proxy with its sources and EPG sources.
func (s *ProxyService) GetByIDWithRelations(ctx context.Context, id models.ULID) (*models.StreamProxy, error) {
	proxy, err := s.proxyRepo.GetByIDWithRelations(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("getting proxy with relations: %w", err)
	}
	return proxy, nil
}

// GetAll retrieves all stream proxies.
func (s *ProxyService) GetAll(ctx context.Context) ([]*models.StreamProxy, error) {
	proxies, err := s.proxyRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting all proxies: %w", err)
	}
	return proxies, nil
}

// GetActive retrieves all active stream proxies.
func (s *ProxyService) GetActive(ctx context.Context) ([]*models.StreamProxy, error) {
	proxies, err := s.proxyRepo.GetActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting active proxies: %w", err)
	}
	return proxies, nil
}

// GetByName retrieves a stream proxy by name.
func (s *ProxyService) GetByName(ctx context.Context, name string) (*models.StreamProxy, error) {
	proxy, err := s.proxyRepo.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("getting proxy by name: %w", err)
	}
	return proxy, nil
}

// SetSources sets the stream sources for a proxy.
func (s *ProxyService) SetSources(ctx context.Context, proxyID models.ULID, sourceIDs []models.ULID, priorities map[models.ULID]int) error {
	if err := s.proxyRepo.SetSources(ctx, proxyID, sourceIDs, priorities); err != nil {
		return fmt.Errorf("setting sources: %w", err)
	}

	s.logger.InfoContext(ctx, "set proxy sources",
		slog.String("proxy_id", proxyID.String()),
		slog.Int("source_count", len(sourceIDs)),
	)

	return nil
}

// SetEpgSources sets the EPG sources for a proxy.
func (s *ProxyService) SetEpgSources(ctx context.Context, proxyID models.ULID, sourceIDs []models.ULID, priorities map[models.ULID]int) error {
	if err := s.proxyRepo.SetEpgSources(ctx, proxyID, sourceIDs, priorities); err != nil {
		return fmt.Errorf("setting EPG sources: %w", err)
	}

	s.logger.InfoContext(ctx, "set proxy EPG sources",
		slog.String("proxy_id", proxyID.String()),
		slog.Int("source_count", len(sourceIDs)),
	)

	return nil
}

// Generate runs the proxy generation pipeline.
func (s *ProxyService) Generate(ctx context.Context, proxyID models.ULID) (*pipeline.Result, error) {
	// Get proxy with relations
	proxy, err := s.proxyRepo.GetByIDWithRelations(ctx, proxyID)
	if err != nil {
		return nil, fmt.Errorf("getting proxy: %w", err)
	}
	if proxy == nil {
		return nil, fmt.Errorf("proxy not found: %s", proxyID)
	}

	if !proxy.IsActive {
		return nil, fmt.Errorf("proxy is not active: %s", proxy.Name)
	}

	// Mark as generating
	if err := s.proxyRepo.UpdateStatus(ctx, proxyID, models.StreamProxyStatusGenerating, ""); err != nil {
		s.logger.WarnContext(ctx, "failed to update proxy status to generating",
			slog.String("error", err.Error()),
		)
	}

	s.logger.InfoContext(ctx, "starting proxy generation",
		slog.String("proxy_id", proxyID.String()),
		slog.String("proxy_name", proxy.Name),
	)

	// Create orchestrator
	orchestrator, err := s.pipelineFactory.Create(proxy)
	if err != nil {
		s.proxyRepo.UpdateStatus(ctx, proxyID, models.StreamProxyStatusFailed, err.Error())
		return nil, fmt.Errorf("creating pipeline: %w", err)
	}

	// Get sources with priority ordering
	sources, err := s.proxyRepo.GetSources(ctx, proxyID)
	if err != nil {
		s.proxyRepo.UpdateStatus(ctx, proxyID, models.StreamProxyStatusFailed, err.Error())
		return nil, fmt.Errorf("getting sources: %w", err)
	}
	orchestrator.SetSources(sources)

	// Get EPG sources with priority ordering
	epgSources, err := s.proxyRepo.GetEpgSources(ctx, proxyID)
	if err != nil {
		s.proxyRepo.UpdateStatus(ctx, proxyID, models.StreamProxyStatusFailed, err.Error())
		return nil, fmt.Errorf("getting EPG sources: %w", err)
	}
	orchestrator.SetEpgSources(epgSources)

	// Execute pipeline
	result, err := orchestrator.Execute(ctx)
	if err != nil {
		s.proxyRepo.UpdateStatus(ctx, proxyID, models.StreamProxyStatusFailed, err.Error())
		return result, fmt.Errorf("executing pipeline: %w", err)
	}

	// Update success status
	if err := s.proxyRepo.UpdateLastGeneration(ctx, proxyID, result.ChannelCount, result.ProgramCount); err != nil {
		s.logger.WarnContext(ctx, "failed to update proxy generation stats",
			slog.String("error", err.Error()),
		)
	}

	s.logger.InfoContext(ctx, "proxy generation completed",
		slog.String("proxy_id", proxyID.String()),
		slog.String("proxy_name", proxy.Name),
		slog.Int("channel_count", result.ChannelCount),
		slog.Int("program_count", result.ProgramCount),
		slog.Duration("duration", result.Duration),
	)

	return result, nil
}

// GenerateAll runs the generation pipeline for all active proxies.
func (s *ProxyService) GenerateAll(ctx context.Context) (map[models.ULID]*pipeline.Result, error) {
	proxies, err := s.proxyRepo.GetActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting active proxies: %w", err)
	}

	results := make(map[models.ULID]*pipeline.Result)

	for _, proxy := range proxies {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		result, err := s.Generate(ctx, proxy.ID)
		if err != nil {
			s.logger.ErrorContext(ctx, "failed to generate proxy",
				slog.String("proxy_id", proxy.ID.String()),
				slog.String("proxy_name", proxy.Name),
				slog.String("error", err.Error()),
			)
			// Continue with other proxies
			continue
		}

		results[proxy.ID] = result
	}

	return results, nil
}
