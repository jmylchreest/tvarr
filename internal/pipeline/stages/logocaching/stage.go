// Package logocaching implements the logo caching pipeline stage.
package logocaching

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/jmylchreest/tvarr/internal/pipeline/shared"
	"github.com/jmylchreest/tvarr/internal/storage"
)

const (
	// StageID is the unique identifier for this stage.
	StageID = "logo_caching"
	// StageName is the human-readable name for this stage.
	StageName = "Logo Caching"
)

// LogoCacher defines the interface for caching logos.
type LogoCacher interface {
	// CacheLogo downloads and caches a logo from the given URL.
	// If already cached, returns the existing metadata.
	CacheLogo(ctx context.Context, logoURL string) (*storage.CachedLogoMetadata, error)

	// Contains checks if a logo URL is already cached.
	Contains(logoURL string) bool
}

// Stats holds statistics from the logo caching stage execution.
type Stats struct {
	ChannelsProcessed int
	ChannelsWithLogos int
	UniqueLogoURLs    int
	AlreadyCached     int
	NewlyCached       int
	Errors            int
}

// Stage caches channel logos during pipeline processing.
type Stage struct {
	shared.BaseStage
	cacher LogoCacher
	logger *slog.Logger
	stats  Stats
}

// New creates a new logo caching stage.
func New(cacher LogoCacher) *Stage {
	return &Stage{
		BaseStage: shared.NewBaseStage(StageID, StageName),
		cacher:    cacher,
	}
}

// NewConstructor returns a stage constructor for use with the factory.
func NewConstructor(cacher LogoCacher) core.StageConstructor {
	return func(deps *core.Dependencies) core.Stage {
		s := New(cacher)
		if deps.Logger != nil {
			s.logger = deps.Logger.With("stage", StageID)
		}
		return s
	}
}

// GetStats returns the statistics from the last execution.
func (s *Stage) GetStats() Stats {
	return s.stats
}

// Execute processes channels and caches their logos.
func (s *Stage) Execute(ctx context.Context, state *core.State) (*core.StageResult, error) {
	result := shared.NewResult()

	// Reset stats for this execution
	s.stats = Stats{}

	if len(state.Channels) == 0 {
		s.log(ctx, slog.LevelInfo, "no channels to process for logo caching, skipping")
		result.Message = "No channels to process for logo caching"
		return result, nil
	}

	// Handle disabled mode (no cacher)
	if s.cacher == nil {
		s.log(ctx, slog.LevelInfo, "logo caching disabled, skipping",
			slog.Int("channel_count", len(state.Channels)))
		result.RecordsProcessed = len(state.Channels)
		result.Message = "Logo caching disabled (no cacher configured)"
		return result, nil
	}

	// T034: Log stage start
	s.log(ctx, slog.LevelInfo, "starting logo caching",
		slog.Int("channel_count", len(state.Channels)))

	// Collect unique logo URLs
	logoURLs := make(map[string]struct{})
	for _, ch := range state.Channels {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		s.stats.ChannelsProcessed++
		if ch.TvgLogo != "" {
			s.stats.ChannelsWithLogos++
			logoURLs[ch.TvgLogo] = struct{}{}
		}
	}

	s.stats.UniqueLogoURLs = len(logoURLs)

	// Process each unique logo URL
	// T038: DEBUG logging for batch progress
	const batchSize = 100
	totalLogos := len(logoURLs)
	newlyCached := 0
	processed := 0
	for logoURL := range logoURLs {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Check if already cached
		if s.cacher.Contains(logoURL) {
			s.stats.AlreadyCached++
			if s.logger != nil {
				s.logger.Debug("logo already cached", "url", logoURL)
			}
			continue
		}

		// Cache the logo (download and store)
		meta, err := s.cacher.CacheLogo(ctx, logoURL)
		if err != nil {
			s.stats.Errors++
			if s.logger != nil {
				s.logger.Warn("failed to cache logo",
					"url", logoURL,
					"error", err)
			}
			continue
		}

		s.stats.NewlyCached++
		newlyCached++

		if s.logger != nil {
			s.logger.Debug("cached logo",
				"url", logoURL,
				"id", meta.GetID())
		}

		// T038: DEBUG logging for batch progress
		processed++
		if processed%batchSize == 0 {
			batchNum := processed / batchSize
			totalBatches := (totalLogos + batchSize - 1) / batchSize
			s.log(ctx, slog.LevelDebug, "logo caching batch progress",
				slog.Int("batch_num", batchNum),
				slog.Int("total_batches", totalBatches),
				slog.Int("items_processed", processed),
				slog.Int("total_items", totalLogos))
		}
	}

	result.RecordsProcessed = len(state.Channels)
	result.RecordsModified = newlyCached

	// Build result message
	if s.stats.UniqueLogoURLs > 0 {
		result.Message = fmt.Sprintf("Processed %d unique logos (%d newly cached, %d already cached, %d errors)",
			s.stats.UniqueLogoURLs, s.stats.NewlyCached, s.stats.AlreadyCached, s.stats.Errors)
	} else {
		result.Message = "No logos found in channels"
	}

	// T034: Log stage completion with cache hit/miss stats
	s.log(ctx, slog.LevelInfo, "logo caching complete",
		slog.Int("unique_logos", s.stats.UniqueLogoURLs),
		slog.Int("newly_cached", s.stats.NewlyCached),
		slog.Int("already_cached", s.stats.AlreadyCached),
		slog.Int("errors", s.stats.Errors))

	// Create artifact with metadata
	artifact := core.NewArtifact(core.ArtifactTypeChannels, core.ProcessingStageFiltered, StageID).
		WithRecordCount(len(state.Channels)).
		WithMetadata("unique_logos", s.stats.UniqueLogoURLs).
		WithMetadata("logos_newly_cached", s.stats.NewlyCached).
		WithMetadata("logos_already_cached", s.stats.AlreadyCached).
		WithMetadata("logos_errors", s.stats.Errors)
	result.Artifacts = append(result.Artifacts, artifact)

	return result, nil
}

// log logs a message if the logger is set.
func (s *Stage) log(ctx context.Context, level slog.Level, msg string, attrs ...any) {
	if s.logger != nil {
		s.logger.Log(ctx, level, msg, attrs...)
	}
}

// Ensure Stage implements core.Stage.
var _ core.Stage = (*Stage)(nil)
