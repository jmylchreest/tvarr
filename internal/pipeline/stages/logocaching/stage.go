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
		result.Message = "No channels to process for logo caching"
		return result, nil
	}

	// Handle disabled mode (no cacher)
	if s.cacher == nil {
		result.RecordsProcessed = len(state.Channels)
		result.Message = "Logo caching disabled (no cacher configured)"
		return result, nil
	}

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
	newlyCached := 0
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

// Ensure Stage implements core.Stage.
var _ core.Stage = (*Stage)(nil)
