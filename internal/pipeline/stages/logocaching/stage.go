// Package logocaching implements the logo caching pipeline stage.
package logocaching

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/jmylchreest/tvarr/internal/expression/helpers"
	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/jmylchreest/tvarr/internal/pipeline/shared"
	"github.com/jmylchreest/tvarr/internal/storage"
	"github.com/jmylchreest/tvarr/internal/urlutil"
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

// ConcurrentLogoCacher extends LogoCacher with concurrency information.
type ConcurrentLogoCacher interface {
	LogoCacher
	// Concurrency returns the configured concurrency level for downloads.
	Concurrency() int
}

// logoJob represents a single logo download job for the worker pool.
type logoJob struct {
	url string
}

// logoResult represents the result of a logo download job.
type logoResult struct {
	url       string
	meta      *storage.CachedLogoMetadata
	err       error
	cached    bool // true if already cached
	skipped   bool // true if skipped (local/unfetchable)
}

// Stats holds statistics from the logo caching stage execution.
type Stats struct {
	// Channel logo stats
	ChannelsProcessed      int
	ChannelsWithLogos      int
	UniqueChannelLogoURLs  int
	ChannelLogosAlready    int
	ChannelLogosNewly      int
	ChannelLogoErrors      int
	ChannelLogosLocalSkip  int // Logos served by tvarr itself (skipped)

	// Program logo stats
	ProgramsProcessed      int
	ProgramsWithLogos      int
	UniqueProgramLogoURLs  int
	ProgramLogosAlready    int
	ProgramLogosNewly      int
	ProgramLogoErrors      int
	ProgramLogosLocalSkip  int // Logos served by tvarr itself (skipped)

	// Combined stats (for backward compatibility)
	UniqueLogoURLs int
	AlreadyCached  int
	NewlyCached    int
	Errors         int
	LocalSkipped   int // Total logos skipped (served by tvarr)
}

// Stage caches channel logos during pipeline processing.
type Stage struct {
	shared.BaseStage
	cacher      LogoCacher
	baseURL     string
	logger      *slog.Logger
	stats       Stats
	concurrency int // number of concurrent download workers
}

// DefaultConcurrency is the default number of concurrent logo download workers.
const DefaultConcurrency = 10

// New creates a new logo caching stage.
func New(cacher LogoCacher) *Stage {
	concurrency := DefaultConcurrency
	// Check if cacher provides concurrency configuration
	if cc, ok := cacher.(ConcurrentLogoCacher); ok {
		concurrency = cc.Concurrency()
	}
	return &Stage{
		BaseStage:   shared.NewBaseStage(StageID, StageName),
		cacher:      cacher,
		concurrency: concurrency,
	}
}

// NewConstructor returns a stage constructor for use with the factory.
func NewConstructor(cacher LogoCacher) core.StageConstructor {
	return func(deps *core.Dependencies) core.Stage {
		s := New(cacher)
		s.baseURL = deps.BaseURL
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

// Execute processes channels and programs, caching their logos based on proxy settings.
func (s *Stage) Execute(ctx context.Context, state *core.State) (*core.StageResult, error) {
	result := shared.NewResult()

	// Reset stats for this execution
	s.stats = Stats{}

	// Handle disabled mode (no cacher)
	if s.cacher == nil {
		s.log(ctx, slog.LevelInfo, "logo caching disabled, skipping",
			slog.Int("channel_count", len(state.Channels)),
			slog.Int("program_count", len(state.Programs)))
		result.RecordsProcessed = len(state.Channels) + len(state.Programs)
		result.Message = "Logo caching disabled (no cacher configured)"
		return result, nil
	}

	// Determine which logo types to cache based on proxy settings
	cacheChannelLogos := state.Proxy != nil && state.Proxy.CacheChannelLogos
	cacheProgramLogos := state.Proxy != nil && state.Proxy.CacheProgramLogos

	// If neither is enabled, skip entirely
	if !cacheChannelLogos && !cacheProgramLogos {
		s.log(ctx, slog.LevelInfo, "logo caching disabled in proxy settings, skipping",
			slog.Int("channel_count", len(state.Channels)),
			slog.Int("program_count", len(state.Programs)))
		result.RecordsProcessed = len(state.Channels) + len(state.Programs)
		result.Message = "Logo caching disabled in proxy settings"
		return result, nil
	}

	// T034: Log stage start
	s.log(ctx, slog.LevelInfo, "starting logo caching",
		slog.Bool("cache_channel_logos", cacheChannelLogos),
		slog.Bool("cache_program_logos", cacheProgramLogos),
		slog.Int("channel_count", len(state.Channels)),
		slog.Int("program_count", len(state.Programs)))

	const batchSize = 100

	// Process logo helper references (@logo:ULID -> /api/v1/logos/ULID) FIRST
	// This must happen before caching so that:
	// 1. @logo:ULID references are converted to local URLs
	// 2. The caching logic correctly identifies them as local (not needing remote fetch)
	s.processLogoHelpers(ctx, state)

	// Calculate total for progress reporting
	totalItems := 0
	if cacheChannelLogos {
		totalItems += len(state.Channels)
	}
	if cacheProgramLogos {
		totalItems += len(state.Programs)
	}
	processedItems := 0

	// Process channel logos if enabled
	if cacheChannelLogos && len(state.Channels) > 0 {
		if err := s.cacheChannelLogos(ctx, state, batchSize, &processedItems, totalItems); err != nil {
			return nil, err
		}
	}

	// Process program logos if enabled
	if cacheProgramLogos && len(state.Programs) > 0 {
		if err := s.cacheProgramLogos(ctx, state, batchSize, &processedItems, totalItems); err != nil {
			return nil, err
		}
	}

	// Update combined stats
	s.stats.UniqueLogoURLs = s.stats.UniqueChannelLogoURLs + s.stats.UniqueProgramLogoURLs
	s.stats.AlreadyCached = s.stats.ChannelLogosAlready + s.stats.ProgramLogosAlready
	s.stats.NewlyCached = s.stats.ChannelLogosNewly + s.stats.ProgramLogosNewly
	s.stats.Errors = s.stats.ChannelLogoErrors + s.stats.ProgramLogoErrors
	s.stats.LocalSkipped = s.stats.ChannelLogosLocalSkip + s.stats.ProgramLogosLocalSkip

	result.RecordsProcessed = s.stats.ChannelsProcessed + s.stats.ProgramsProcessed
	result.RecordsModified = s.stats.NewlyCached

	// Build result message
	var messageParts []string
	if cacheChannelLogos {
		messageParts = append(messageParts, fmt.Sprintf("channel: %d unique (%d new, %d cached, %d errors)",
			s.stats.UniqueChannelLogoURLs, s.stats.ChannelLogosNewly, s.stats.ChannelLogosAlready, s.stats.ChannelLogoErrors))
	}
	if cacheProgramLogos {
		messageParts = append(messageParts, fmt.Sprintf("program: %d unique (%d new, %d cached, %d errors)",
			s.stats.UniqueProgramLogoURLs, s.stats.ProgramLogosNewly, s.stats.ProgramLogosAlready, s.stats.ProgramLogoErrors))
	}
	if len(messageParts) > 0 {
		result.Message = fmt.Sprintf("Logo caching: %s", fmt.Sprintf("%v", messageParts))
	} else {
		result.Message = "No logos to cache"
	}

	// T034: Log stage completion with cache hit/miss stats
	s.log(ctx, slog.LevelInfo, "logo caching complete",
		slog.Int("channel_logos", s.stats.UniqueChannelLogoURLs),
		slog.Int("program_logos", s.stats.UniqueProgramLogoURLs),
		slog.Int("newly_cached", s.stats.NewlyCached),
		slog.Int("already_cached", s.stats.AlreadyCached),
		slog.Int("errors", s.stats.Errors))

	// Create artifact with metadata
	artifact := core.NewArtifact(core.ArtifactTypeChannels, core.ProcessingStageFiltered, StageID).
		WithRecordCount(len(state.Channels)).
		WithMetadata("unique_logos", s.stats.UniqueLogoURLs).
		WithMetadata("logos_newly_cached", s.stats.NewlyCached).
		WithMetadata("logos_already_cached", s.stats.AlreadyCached).
		WithMetadata("logos_errors", s.stats.Errors).
		WithMetadata("channel_logos_cached", s.stats.ChannelLogosNewly).
		WithMetadata("program_logos_cached", s.stats.ProgramLogosNewly)
	result.Artifacts = append(result.Artifacts, artifact)

	return result, nil
}

// cacheChannelLogos processes and caches logos from channel TvgLogo fields.
func (s *Stage) cacheChannelLogos(ctx context.Context, state *core.State, batchSize int, processedItems *int, totalItems int) error {
	// Collect unique channel logo URLs
	logoURLs := make(map[string]struct{})
	for _, ch := range state.Channels {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		s.stats.ChannelsProcessed++
		if ch.TvgLogo != "" {
			s.stats.ChannelsWithLogos++
			logoURLs[ch.TvgLogo] = struct{}{}
		}

		// Report progress for scanning phase
		*processedItems++
		if state.ProgressReporter != nil && *processedItems%100 == 0 {
			state.ProgressReporter.ReportItemProgress(ctx, StageID, *processedItems, totalItems,
				fmt.Sprintf("Scanning channels: %d/%d", *processedItems, totalItems))
		}
	}

	s.stats.UniqueChannelLogoURLs = len(logoURLs)

	if len(logoURLs) == 0 {
		s.log(ctx, slog.LevelDebug, "no channel logos to cache")
		return nil
	}

	s.log(ctx, slog.LevelDebug, "caching channel logos",
		slog.Int("unique_urls", len(logoURLs)),
		slog.Int("concurrency", s.concurrency))

	// Reset progress for caching phase - report unique logos to cache
	if state.ProgressReporter != nil {
		state.ProgressReporter.ReportItemProgress(ctx, StageID, 0, s.stats.UniqueChannelLogoURLs,
			fmt.Sprintf("Caching %d unique channel logos (concurrency: %d)", s.stats.UniqueChannelLogoURLs, s.concurrency))
	}

	// Filter URLs that need fetching vs those we can skip
	urlsToFetch := make([]string, 0, len(logoURLs))
	for logoURL := range logoURLs {
		// Skip unfetchable logo URLs (relative paths without scheme, @logo: refs)
		if isUnfetchableLogoURL(logoURL) {
			s.stats.ChannelLogosLocalSkip++
			if s.logger != nil {
				s.logger.Debug("skipping unfetchable logo URL", "url", logoURL)
			}
			continue
		}

		// Skip local tvarr logo URLs (already served by us, no need to cache)
		if s.isLocalLogoURL(logoURL) {
			s.stats.ChannelLogosLocalSkip++
			if s.logger != nil {
				s.logger.Debug("skipping local tvarr logo URL", "url", logoURL)
			}
			continue
		}

		// Check if already cached
		if s.cacher.Contains(logoURL) {
			s.stats.ChannelLogosAlready++
			if s.logger != nil {
				s.logger.Debug("channel logo already cached", "url", logoURL)
			}
			continue
		}

		urlsToFetch = append(urlsToFetch, logoURL)
	}

	// If nothing to fetch, we're done
	if len(urlsToFetch) == 0 {
		return nil
	}

	// Use concurrent workers to cache logos
	var (
		processed int32
		errors    int32
		newlyCached int32
	)

	// Create job and result channels
	jobs := make(chan logoJob, len(urlsToFetch))
	results := make(chan logoResult, len(urlsToFetch))

	// Start worker pool
	var wg sync.WaitGroup
	workerCount := s.concurrency
	if workerCount > len(urlsToFetch) {
		workerCount = len(urlsToFetch)
	}

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				select {
				case <-ctx.Done():
					results <- logoResult{url: job.url, err: ctx.Err()}
					return
				default:
				}

				// Cache the logo
				meta, err := s.cacher.CacheLogo(ctx, job.url)
				results <- logoResult{
					url:  job.url,
					meta: meta,
					err:  err,
				}
			}
		}()
	}

	// Send jobs to workers
	go func() {
		for _, url := range urlsToFetch {
			jobs <- logoJob{url: url}
		}
		close(jobs)
	}()

	// Collect results in a separate goroutine
	go func() {
		wg.Wait()
		close(results)
	}()

	// Process results
	total := len(urlsToFetch)
	for result := range results {
		current := int(atomic.AddInt32(&processed, 1))

		if result.err != nil {
			atomic.AddInt32(&errors, 1)
			if s.logger != nil {
				s.logger.Warn("failed to cache channel logo",
					"url", result.url,
					"error", result.err)
			}
		} else {
			atomic.AddInt32(&newlyCached, 1)
			if s.logger != nil {
				s.logger.Debug("cached channel logo",
					"url", result.url,
					"id", result.meta.GetID())
			}
		}

		// Report progress periodically (every 10 items or at the end)
		if state.ProgressReporter != nil && (current%10 == 0 || current == total) {
			state.ProgressReporter.ReportItemProgress(ctx, StageID, current, total,
				fmt.Sprintf("Channel logos: %d/%d (new: %d, errors: %d)", current, total, newlyCached, errors))
		}
	}

	s.stats.ChannelLogosNewly = int(newlyCached)
	s.stats.ChannelLogoErrors = int(errors)

	return nil
}

// cacheProgramLogos processes and caches logos from program Icon fields.
func (s *Stage) cacheProgramLogos(ctx context.Context, state *core.State, batchSize int, processedItems *int, totalItems int) error {
	// Collect unique program logo URLs
	logoURLs := make(map[string]struct{})
	for _, prog := range state.Programs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		s.stats.ProgramsProcessed++
		if prog.Icon != "" {
			s.stats.ProgramsWithLogos++
			logoURLs[prog.Icon] = struct{}{}
		}

		// Report progress for scanning phase
		*processedItems++
		if state.ProgressReporter != nil && *processedItems%1000 == 0 {
			state.ProgressReporter.ReportItemProgress(ctx, StageID, *processedItems, totalItems,
				fmt.Sprintf("Scanning programs: %d/%d", *processedItems, totalItems))
		}
	}

	s.stats.UniqueProgramLogoURLs = len(logoURLs)

	if len(logoURLs) == 0 {
		s.log(ctx, slog.LevelDebug, "no program logos to cache")
		return nil
	}

	s.log(ctx, slog.LevelDebug, "caching program logos",
		slog.Int("unique_urls", len(logoURLs)),
		slog.Int("concurrency", s.concurrency))

	// Reset progress for caching phase - report unique logos to cache
	if state.ProgressReporter != nil {
		state.ProgressReporter.ReportItemProgress(ctx, StageID, 0, s.stats.UniqueProgramLogoURLs,
			fmt.Sprintf("Caching %d unique program logos (concurrency: %d)", s.stats.UniqueProgramLogoURLs, s.concurrency))
	}

	// Filter URLs that need fetching vs those we can skip
	urlsToFetch := make([]string, 0, len(logoURLs))
	for logoURL := range logoURLs {
		// Skip unfetchable logo URLs (relative paths, deferred references, @logo: refs)
		if isUnfetchableLogoURL(logoURL) {
			s.stats.ProgramLogosLocalSkip++
			if s.logger != nil {
				s.logger.Debug("skipping unfetchable program logo URL", "url", logoURL)
			}
			continue
		}

		// Skip local tvarr logo URLs (already served by us, no need to cache)
		if s.isLocalLogoURL(logoURL) {
			s.stats.ProgramLogosLocalSkip++
			if s.logger != nil {
				s.logger.Debug("skipping local tvarr program logo URL", "url", logoURL)
			}
			continue
		}

		// Check if already cached
		if s.cacher.Contains(logoURL) {
			s.stats.ProgramLogosAlready++
			if s.logger != nil {
				s.logger.Debug("program logo already cached", "url", logoURL)
			}
			continue
		}

		urlsToFetch = append(urlsToFetch, logoURL)
	}

	// If nothing to fetch, we're done
	if len(urlsToFetch) == 0 {
		return nil
	}

	// Use concurrent workers to cache logos
	var (
		processed   int32
		errors      int32
		newlyCached int32
	)

	// Create job and result channels
	jobs := make(chan logoJob, len(urlsToFetch))
	results := make(chan logoResult, len(urlsToFetch))

	// Start worker pool
	var wg sync.WaitGroup
	workerCount := s.concurrency
	if workerCount > len(urlsToFetch) {
		workerCount = len(urlsToFetch)
	}

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				select {
				case <-ctx.Done():
					results <- logoResult{url: job.url, err: ctx.Err()}
					return
				default:
				}

				// Cache the logo
				meta, err := s.cacher.CacheLogo(ctx, job.url)
				results <- logoResult{
					url:  job.url,
					meta: meta,
					err:  err,
				}
			}
		}()
	}

	// Send jobs to workers
	go func() {
		for _, url := range urlsToFetch {
			jobs <- logoJob{url: url}
		}
		close(jobs)
	}()

	// Collect results in a separate goroutine
	go func() {
		wg.Wait()
		close(results)
	}()

	// Process results
	total := len(urlsToFetch)
	for result := range results {
		current := int(atomic.AddInt32(&processed, 1))

		if result.err != nil {
			atomic.AddInt32(&errors, 1)
			if s.logger != nil {
				s.logger.Warn("failed to cache program logo",
					"url", result.url,
					"error", result.err)
			}
		} else {
			atomic.AddInt32(&newlyCached, 1)
			if s.logger != nil {
				s.logger.Debug("cached program logo",
					"url", result.url,
					"id", result.meta.GetID())
			}
		}

		// Report progress periodically (every 50 items or at the end)
		if state.ProgressReporter != nil && (current%50 == 0 || current == total) {
			state.ProgressReporter.ReportItemProgress(ctx, StageID, current, total,
				fmt.Sprintf("Program logos: %d/%d (new: %d, errors: %d)", current, total, newlyCached, errors))
		}
	}

	s.stats.ProgramLogosNewly = int(newlyCached)
	s.stats.ProgramLogoErrors = int(errors)

	return nil
}

// processLogoHelpers resolves @logo:ULID helper references to fully qualified URLs.
// This converts logo helper references (generated by data mapping rules via @logo:ULID syntax)
// to absolute URLs that external IPTV clients can fetch.
// Example: @logo:01ABC... -> http://localhost:8080/api/v1/logos/01ABC...
func (s *Stage) processLogoHelpers(ctx context.Context, state *core.State) {
	var channelResolved, programResolved int

	// Resolve channel logos using the helpers package
	for _, ch := range state.Channels {
		if helpers.HasLogoHelper(ch.TvgLogo) {
			ch.TvgLogo = helpers.ProcessLogoHelper(ch.TvgLogo, s.baseURL)
			channelResolved++
		}
	}

	// Resolve program logos using the helpers package
	for _, prog := range state.Programs {
		if helpers.HasLogoHelper(prog.Icon) {
			prog.Icon = helpers.ProcessLogoHelper(prog.Icon, s.baseURL)
			programResolved++
		}
	}

	if channelResolved > 0 || programResolved > 0 {
		s.log(ctx, slog.LevelDebug, "processed logo helper references",
			slog.Int("channel_logos", channelResolved),
			slog.Int("program_logos", programResolved),
			slog.String("base_url", s.baseURL))
	}
}

// log logs a message if the logger is set.
func (s *Stage) log(ctx context.Context, level slog.Level, msg string, attrs ...any) {
	if s.logger != nil {
		s.logger.Log(ctx, level, msg, attrs...)
	}
}

// isUnfetchableLogoURL checks if a URL cannot be fetched remotely.
// Returns true for any URL that is not a remote URL with http/https scheme,
// including helper references (@logo:ULID, @time:now, etc.) and relative paths.
// Only full remote URLs like http://example.com/... can be fetched.
func isUnfetchableLogoURL(url string) bool {
	// Skip empty URLs (these are not "unfetchable", they're just empty)
	if url == "" {
		return false
	}

	// Check for any helper references (@name:args) - these should be resolved first
	if helpers.HasAnyHelper(url) {
		return true
	}

	// Any non-remote URL is unfetchable (relative paths, local paths, etc.)
	return !urlutil.IsRemoteURL(url)
}

// isLocalLogoURL checks if a URL points to a logo served by tvarr itself.
// These URLs contain /api/v1/logos/ and should not be cached since they're already local.
func (s *Stage) isLocalLogoURL(url string) bool {
	// Check if URL contains the tvarr logos API path
	if strings.Contains(url, "/api/v1/logos/") {
		// If we have a base URL, verify it matches
		if s.baseURL != "" && strings.HasPrefix(url, s.baseURL) {
			return true
		}
		// Also handle relative URLs that start with /api/v1/logos/
		if strings.HasPrefix(url, "/api/v1/logos/") {
			return true
		}
		// Handle case where baseURL might be different (e.g., http vs https)
		// If the path contains our logo API, it's likely local
		return true
	}
	return false
}

// Ensure Stage implements core.Stage.
var _ core.Stage = (*Stage)(nil)
