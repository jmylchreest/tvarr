// Package repository defines data access interfaces for tvarr entities.
// All database access goes through these interfaces, enabling easy testing
// and database backend switching.
package repository

import (
	"context"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
)

// FieldValueResult represents a distinct field value with its occurrence count.
type FieldValueResult struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// StreamSourceRepository defines operations for stream source persistence.
type StreamSourceRepository interface {
	// Create creates a new stream source.
	Create(ctx context.Context, source *models.StreamSource) error
	// GetByID retrieves a stream source by ID.
	GetByID(ctx context.Context, id models.ULID) (*models.StreamSource, error)
	// GetAll retrieves all stream sources.
	GetAll(ctx context.Context) ([]*models.StreamSource, error)
	// GetEnabled retrieves all enabled stream sources.
	GetEnabled(ctx context.Context) ([]*models.StreamSource, error)
	// Update updates an existing stream source.
	Update(ctx context.Context, source *models.StreamSource) error
	// Delete deletes a stream source by ID.
	Delete(ctx context.Context, id models.ULID) error
	// GetByName retrieves a stream source by name.
	GetByName(ctx context.Context, name string) (*models.StreamSource, error)
	// UpdateLastIngestion updates the last ingestion timestamp and status.
	UpdateLastIngestion(ctx context.Context, id models.ULID, status string, channelCount int) error
}

// ChannelRepository defines operations for channel persistence.
type ChannelRepository interface {
	// Create creates a new channel.
	Create(ctx context.Context, channel *models.Channel) error
	// CreateBatch creates multiple channels in a single batch.
	CreateBatch(ctx context.Context, channels []*models.Channel) error
	// UpsertBatch creates or updates multiple channels, handling duplicates gracefully.
	// Uses ON CONFLICT to update existing channels based on (source_id, ext_id).
	UpsertBatch(ctx context.Context, channels []*models.Channel) error
	// GetByID retrieves a channel by ID.
	GetByID(ctx context.Context, id models.ULID) (*models.Channel, error)
	// GetByIDWithSource retrieves a channel by ID with its Source relationship preloaded.
	GetByIDWithSource(ctx context.Context, id models.ULID) (*models.Channel, error)
	// GetBySourceID retrieves all channels for a source using a callback for streaming.
	GetBySourceID(ctx context.Context, sourceID models.ULID, callback func(*models.Channel) error) error
	// GetBySourceIDPaginated retrieves channels for a source with pagination.
	GetBySourceIDPaginated(ctx context.Context, sourceID models.ULID, offset, limit int) ([]*models.Channel, int64, error)
	// Update updates an existing channel.
	Update(ctx context.Context, channel *models.Channel) error
	// Delete deletes a channel by ID.
	Delete(ctx context.Context, id models.ULID) error
	// DeleteBySourceID deletes all channels for a source.
	DeleteBySourceID(ctx context.Context, sourceID models.ULID) error
	// DeleteStaleBySourceID deletes channels for a source that haven't been updated since the given time.
	// Used for "mark and sweep" cleanup after upsert to remove channels no longer in the source.
	DeleteStaleBySourceID(ctx context.Context, sourceID models.ULID, olderThan time.Time) (int64, error)
	// CountBySourceID returns the number of channels for a source.
	CountBySourceID(ctx context.Context, sourceID models.ULID) (int64, error)
	// GetByExtID retrieves a channel by source ID and external ID.
	GetByExtID(ctx context.Context, sourceID models.ULID, extID string) (*models.Channel, error)
	// GetDistinctFieldValues returns distinct values for a channel field with occurrence counts.
	// The field parameter must be one of the allowed fields (group_title, channel_name, tvg_id, country).
	// Results are filtered by the query parameter (case-insensitive contains) and limited.
	GetDistinctFieldValues(ctx context.Context, field string, query string, limit int) ([]FieldValueResult, error)
	// Transaction executes the given function within a database transaction.
	// The provided function receives a transactional repository.
	// If the function returns an error, the transaction is rolled back.
	Transaction(ctx context.Context, fn func(ChannelRepository) error) error
}

// ManualStreamChannelRepository defines operations for manual channel persistence.
type ManualStreamChannelRepository interface {
	// Create creates a new manual channel.
	Create(ctx context.Context, channel *models.ManualStreamChannel) error
	// GetByID retrieves a manual channel by ID.
	GetByID(ctx context.Context, id models.ULID) (*models.ManualStreamChannel, error)
	// GetAll retrieves all manual channels.
	GetAll(ctx context.Context) ([]*models.ManualStreamChannel, error)
	// GetBySourceID retrieves all manual channels for a source.
	GetBySourceID(ctx context.Context, sourceID models.ULID) ([]*models.ManualStreamChannel, error)
	// GetEnabledBySourceID retrieves enabled manual channels for a source, ordered by priority.
	GetEnabledBySourceID(ctx context.Context, sourceID models.ULID) ([]*models.ManualStreamChannel, error)
	// Update updates an existing manual channel.
	Update(ctx context.Context, channel *models.ManualStreamChannel) error
	// Delete deletes a manual channel by ID.
	Delete(ctx context.Context, id models.ULID) error
	// DeleteBySourceID deletes all manual channels for a source.
	DeleteBySourceID(ctx context.Context, sourceID models.ULID) error
	// CountBySourceID returns the number of manual channels for a source.
	CountBySourceID(ctx context.Context, sourceID models.ULID) (int64, error)
}

// EpgSourceRepository defines operations for EPG source persistence.
type EpgSourceRepository interface {
	// Create creates a new EPG source.
	Create(ctx context.Context, source *models.EpgSource) error
	// GetByID retrieves an EPG source by ID.
	GetByID(ctx context.Context, id models.ULID) (*models.EpgSource, error)
	// GetAll retrieves all EPG sources.
	GetAll(ctx context.Context) ([]*models.EpgSource, error)
	// GetEnabled retrieves all enabled EPG sources.
	GetEnabled(ctx context.Context) ([]*models.EpgSource, error)
	// Update updates an existing EPG source.
	Update(ctx context.Context, source *models.EpgSource) error
	// Delete deletes an EPG source by ID.
	Delete(ctx context.Context, id models.ULID) error
	// GetByName retrieves an EPG source by name.
	GetByName(ctx context.Context, name string) (*models.EpgSource, error)
	// GetByURL retrieves an EPG source by URL.
	GetByURL(ctx context.Context, url string) (*models.EpgSource, error)
	// UpdateLastIngestion updates the last ingestion timestamp and status.
	UpdateLastIngestion(ctx context.Context, id models.ULID, status string, programCount int) error
}

// EpgProgramRepository defines operations for EPG program persistence.
type EpgProgramRepository interface {
	// Create creates a new EPG program.
	Create(ctx context.Context, program *models.EpgProgram) error
	// CreateBatch creates multiple programs in a single batch.
	CreateBatch(ctx context.Context, programs []*models.EpgProgram) error
	// GetByID retrieves an EPG program by ID.
	GetByID(ctx context.Context, id models.ULID) (*models.EpgProgram, error)
	// GetBySourceID retrieves all programs for a source using a callback for streaming.
	GetBySourceID(ctx context.Context, sourceID models.ULID, callback func(*models.EpgProgram) error) error
	// GetByChannelID retrieves programs for a channel within a time range.
	GetByChannelID(ctx context.Context, channelID string, start, end time.Time) ([]*models.EpgProgram, error)
	// GetByChannelIDWithLimit retrieves upcoming programs for a channel with a limit.
	GetByChannelIDWithLimit(ctx context.Context, channelID string, limit int) ([]*models.EpgProgram, error)
	// GetCurrentByChannelID retrieves the currently airing program for a channel.
	GetCurrentByChannelID(ctx context.Context, channelID string) (*models.EpgProgram, error)
	// Delete deletes an EPG program by ID.
	Delete(ctx context.Context, id models.ULID) error
	// DeleteBySourceID deletes all programs for a source.
	DeleteBySourceID(ctx context.Context, sourceID models.ULID) error
	// DeleteExpired deletes programs that ended before the given time.
	DeleteExpired(ctx context.Context, before time.Time) (int64, error)
	// DeleteOld deletes programs older than the configured retention period.
	DeleteOld(ctx context.Context) (int64, error)
	// CountBySourceID returns the number of programs for a source.
	CountBySourceID(ctx context.Context, sourceID models.ULID) (int64, error)
}

// StreamProxyRepository defines operations for stream proxy persistence.
type StreamProxyRepository interface {
	// Create creates a new stream proxy.
	Create(ctx context.Context, proxy *models.StreamProxy) error
	// GetByID retrieves a stream proxy by ID.
	GetByID(ctx context.Context, id models.ULID) (*models.StreamProxy, error)
	// GetByIDWithRelations retrieves a stream proxy with its sources and EPG sources.
	GetByIDWithRelations(ctx context.Context, id models.ULID) (*models.StreamProxy, error)
	// GetAll retrieves all stream proxies.
	GetAll(ctx context.Context) ([]*models.StreamProxy, error)
	// GetActive retrieves all active stream proxies.
	GetActive(ctx context.Context) ([]*models.StreamProxy, error)
	// Update updates an existing stream proxy.
	Update(ctx context.Context, proxy *models.StreamProxy) error
	// Delete deletes a stream proxy by ID.
	Delete(ctx context.Context, id models.ULID) error
	// GetByName retrieves a stream proxy by name.
	GetByName(ctx context.Context, name string) (*models.StreamProxy, error)
	// UpdateStatus updates the generation status.
	UpdateStatus(ctx context.Context, id models.ULID, status models.StreamProxyStatus, lastError string) error
	// UpdateLastGeneration updates the last generation timestamp and counts.
	UpdateLastGeneration(ctx context.Context, id models.ULID, channelCount, programCount int) error
	// SetSources sets the stream sources for a proxy (replaces existing).
	SetSources(ctx context.Context, proxyID models.ULID, sourceIDs []models.ULID, priorities map[models.ULID]int) error
	// SetEpgSources sets the EPG sources for a proxy (replaces existing).
	SetEpgSources(ctx context.Context, proxyID models.ULID, sourceIDs []models.ULID, priorities map[models.ULID]int) error
	// GetSources retrieves the stream sources for a proxy with priority ordering.
	GetSources(ctx context.Context, proxyID models.ULID) ([]*models.StreamSource, error)
	// GetEpgSources retrieves the EPG sources for a proxy with priority ordering.
	GetEpgSources(ctx context.Context, proxyID models.ULID) ([]*models.EpgSource, error)
	// SetFilters sets the filters for a proxy (replaces existing).
	// The isActive map controls whether each filter is active (applied during generation).
	SetFilters(ctx context.Context, proxyID models.ULID, filterIDs []models.ULID, orders map[models.ULID]int, isActive map[models.ULID]bool) error
	// GetFilters retrieves the filters for a proxy with order.
	GetFilters(ctx context.Context, proxyID models.ULID) ([]*models.Filter, error)
	// GetBySourceID retrieves all proxies that use a specific stream source.
	// Used for auto-regeneration when a source is updated.
	GetBySourceID(ctx context.Context, sourceID models.ULID) ([]*models.StreamProxy, error)
	// GetByEpgSourceID retrieves all proxies that use a specific EPG source.
	// Used for auto-regeneration when an EPG source is updated.
	GetByEpgSourceID(ctx context.Context, epgSourceID models.ULID) ([]*models.StreamProxy, error)
	// CountByEncodingProfileID returns the count of stream proxies using a given encoding profile.
	CountByEncodingProfileID(ctx context.Context, profileID models.ULID) (int64, error)
	// GetByEncodingProfileID returns stream proxies using a given encoding profile.
	GetByEncodingProfileID(ctx context.Context, profileID models.ULID) ([]*models.StreamProxy, error)
	// CountByStreamSourceID returns the count of all proxies using a stream source.
	CountByStreamSourceID(ctx context.Context, sourceID models.ULID) (int64, error)
	// CountByEpgSourceID returns the count of all proxies using an EPG source.
	CountByEpgSourceID(ctx context.Context, epgSourceID models.ULID) (int64, error)
	// CountByFilterID returns the count of all proxies using a filter.
	CountByFilterID(ctx context.Context, filterID models.ULID) (int64, error)
	// GetProxyNamesByStreamSourceID returns names of proxies using a stream source.
	GetProxyNamesByStreamSourceID(ctx context.Context, sourceID models.ULID) ([]string, error)
	// GetProxyNamesByEpgSourceID returns names of proxies using an EPG source.
	GetProxyNamesByEpgSourceID(ctx context.Context, epgSourceID models.ULID) ([]string, error)
	// GetProxyNamesByFilterID returns names of proxies using a filter.
	GetProxyNamesByFilterID(ctx context.Context, filterID models.ULID) ([]string, error)
	// GetProxyNamesByEncodingProfileID returns names of proxies using an encoding profile.
	GetProxyNamesByEncodingProfileID(ctx context.Context, profileID models.ULID) ([]string, error)
}

// FilterRepository defines operations for filter persistence.
// Note: Filters do not have an enabled/disabled state. The enabled state is
// controlled at the proxy-filter relationship level (ProxyFilter.IsActive).
type FilterRepository interface {
	// Create creates a new filter.
	Create(ctx context.Context, filter *models.Filter) error
	// GetByID retrieves a filter by ID.
	GetByID(ctx context.Context, id models.ULID) (*models.Filter, error)
	// GetByIDs retrieves filters by multiple IDs.
	GetByIDs(ctx context.Context, ids []models.ULID) ([]*models.Filter, error)
	// GetByName retrieves a filter by name.
	GetByName(ctx context.Context, name string) (*models.Filter, error)
	// GetAll retrieves all filters.
	GetAll(ctx context.Context) ([]*models.Filter, error)
	// GetUserCreated retrieves all user-created filters (IsSystem=false).
	GetUserCreated(ctx context.Context) ([]*models.Filter, error)
	// GetBySourceType retrieves filters by source type (stream/epg).
	GetBySourceType(ctx context.Context, sourceType models.FilterSourceType) ([]*models.Filter, error)
	// GetBySourceID retrieves filters for a specific source (or global if sourceID is nil).
	GetBySourceID(ctx context.Context, sourceID *models.ULID) ([]*models.Filter, error)
	// Update updates an existing filter.
	Update(ctx context.Context, filter *models.Filter) error
	// Delete deletes a filter by ID.
	Delete(ctx context.Context, id models.ULID) error
	// Count returns the total number of filters.
	Count(ctx context.Context) (int64, error)
}

// DataMappingRuleRepository defines operations for data mapping rule persistence.
type DataMappingRuleRepository interface {
	// Create creates a new data mapping rule.
	Create(ctx context.Context, rule *models.DataMappingRule) error
	// GetByID retrieves a data mapping rule by ID.
	GetByID(ctx context.Context, id models.ULID) (*models.DataMappingRule, error)
	// GetByIDs retrieves data mapping rules by multiple IDs.
	GetByIDs(ctx context.Context, ids []models.ULID) ([]*models.DataMappingRule, error)
	// GetByName retrieves a data mapping rule by name.
	GetByName(ctx context.Context, name string) (*models.DataMappingRule, error)
	// GetAll retrieves all data mapping rules.
	GetAll(ctx context.Context) ([]*models.DataMappingRule, error)
	// GetEnabled retrieves all enabled data mapping rules.
	GetEnabled(ctx context.Context) ([]*models.DataMappingRule, error)
	// GetUserCreated retrieves all user-created data mapping rules (IsSystem=false).
	GetUserCreated(ctx context.Context) ([]*models.DataMappingRule, error)
	// GetBySourceType retrieves rules by source type (stream/epg).
	GetBySourceType(ctx context.Context, sourceType models.DataMappingRuleSourceType) ([]*models.DataMappingRule, error)
	// GetBySourceID retrieves rules for a specific source (or global if sourceID is nil).
	GetBySourceID(ctx context.Context, sourceID *models.ULID) ([]*models.DataMappingRule, error)
	// GetEnabledForSourceType retrieves enabled rules for a source type, ordered by priority.
	GetEnabledForSourceType(ctx context.Context, sourceType models.DataMappingRuleSourceType, sourceID *models.ULID) ([]*models.DataMappingRule, error)
	// Update updates an existing data mapping rule.
	Update(ctx context.Context, rule *models.DataMappingRule) error
	// Delete deletes a data mapping rule by ID.
	Delete(ctx context.Context, id models.ULID) error
	// Count returns the total number of rules.
	Count(ctx context.Context) (int64, error)
}

// LastKnownCodecRepository defines operations for codec cache persistence.
type LastKnownCodecRepository interface {
	// Create creates a new codec cache entry.
	Create(ctx context.Context, codec *models.LastKnownCodec) error
	// GetByID retrieves a codec entry by ID.
	GetByID(ctx context.Context, id models.ULID) (*models.LastKnownCodec, error)
	// GetByStreamURL retrieves codec info by stream URL.
	GetByStreamURL(ctx context.Context, streamURL string) (*models.LastKnownCodec, error)
	// GetBySourceID retrieves all codec entries for a source.
	GetBySourceID(ctx context.Context, sourceID models.ULID) ([]*models.LastKnownCodec, error)
	// Upsert creates or updates a codec entry based on stream URL.
	Upsert(ctx context.Context, codec *models.LastKnownCodec) error
	// Update updates an existing codec entry.
	Update(ctx context.Context, codec *models.LastKnownCodec) error
	// Delete deletes a codec entry by ID.
	Delete(ctx context.Context, id models.ULID) error
	// DeleteByStreamURL deletes a codec entry by stream URL.
	DeleteByStreamURL(ctx context.Context, streamURL string) error
	// DeleteBySourceID deletes all codec entries for a source.
	DeleteBySourceID(ctx context.Context, sourceID models.ULID) (int64, error)
	// DeleteExpired deletes expired codec entries.
	DeleteExpired(ctx context.Context) (int64, error)
	// DeleteAll deletes all codec cache entries.
	DeleteAll(ctx context.Context) (int64, error)
	// Touch updates the access time and increments hit count for a stream URL.
	Touch(ctx context.Context, streamURL string) error
	// GetValidCount returns the count of valid (non-expired, no error) entries.
	GetValidCount(ctx context.Context) (int64, error)
	// GetStats returns cache statistics.
	GetStats(ctx context.Context) (*CodecCacheStats, error)
}

// CodecCacheStats holds statistics about the codec cache.
type CodecCacheStats struct {
	TotalEntries   int64 `json:"total_entries"`
	ValidEntries   int64 `json:"valid_entries"`
	ExpiredEntries int64 `json:"expired_entries"`
	ErrorEntries   int64 `json:"error_entries"`
	TotalHits      int64 `json:"total_hits"`
}

// Note: Logo caching uses file-based storage with in-memory indexing.
// See internal/service/logo_indexer.go for the LogoIndexer service.

// JobRepository defines operations for job persistence.
type JobRepository interface {
	// Create creates a new job.
	Create(ctx context.Context, job *models.Job) error
	// GetByID retrieves a job by ID.
	GetByID(ctx context.Context, id models.ULID) (*models.Job, error)
	// GetAll retrieves all jobs.
	GetAll(ctx context.Context) ([]*models.Job, error)
	// GetPending retrieves all pending/scheduled jobs ready for execution.
	GetPending(ctx context.Context) ([]*models.Job, error)
	// GetByStatus retrieves jobs by status.
	GetByStatus(ctx context.Context, status models.JobStatus) ([]*models.Job, error)
	// GetByType retrieves jobs by type.
	GetByType(ctx context.Context, jobType models.JobType) ([]*models.Job, error)
	// GetByTargetID retrieves jobs for a specific target.
	GetByTargetID(ctx context.Context, targetID models.ULID) ([]*models.Job, error)
	// GetRunning retrieves all currently running jobs.
	GetRunning(ctx context.Context) ([]*models.Job, error)
	// Update updates an existing job.
	Update(ctx context.Context, job *models.Job) error
	// Delete deletes a job by ID.
	Delete(ctx context.Context, id models.ULID) error
	// DeleteCompleted deletes completed jobs older than the specified duration.
	DeleteCompleted(ctx context.Context, before time.Time) (int64, error)
	// AcquireJob atomically acquires a pending job for execution (sets status to running).
	// Returns nil if no jobs are available or if another worker acquired it first.
	AcquireJob(ctx context.Context, workerID string) (*models.Job, error)
	// ReleaseJob releases a job lock (used when a worker fails unexpectedly).
	ReleaseJob(ctx context.Context, id models.ULID) error
	// FindDuplicatePending finds an existing pending/scheduled job for the same type and target.
	// Used for deduplication of concurrent job requests.
	FindDuplicatePending(ctx context.Context, jobType models.JobType, targetID models.ULID) (*models.Job, error)
	// CreateHistory creates a job history record.
	CreateHistory(ctx context.Context, history *models.JobHistory) error
	// GetHistory retrieves job history with pagination.
	GetHistory(ctx context.Context, jobType *models.JobType, offset, limit int) ([]*models.JobHistory, int64, error)
	// DeleteHistory deletes history records older than the specified time.
	DeleteHistory(ctx context.Context, before time.Time) (int64, error)
}

// ReorderRequest represents a request to update an entity's priority.
type ReorderRequest struct {
	ID       models.ULID `json:"id"`
	Priority int         `json:"priority"`
}

// EncodingProfileRepository defines operations for encoding profile persistence.
type EncodingProfileRepository interface {
	// Create creates a new encoding profile.
	Create(ctx context.Context, profile *models.EncodingProfile) error
	// GetByID retrieves an encoding profile by ID.
	GetByID(ctx context.Context, id models.ULID) (*models.EncodingProfile, error)
	// GetByIDs retrieves encoding profiles by multiple IDs.
	GetByIDs(ctx context.Context, ids []models.ULID) ([]*models.EncodingProfile, error)
	// GetAll retrieves all encoding profiles.
	GetAll(ctx context.Context) ([]*models.EncodingProfile, error)
	// GetEnabled retrieves all enabled encoding profiles.
	GetEnabled(ctx context.Context) ([]*models.EncodingProfile, error)
	// GetUserCreated retrieves all user-created encoding profiles (IsSystem=false).
	GetUserCreated(ctx context.Context) ([]*models.EncodingProfile, error)
	// GetByName retrieves an encoding profile by name.
	GetByName(ctx context.Context, name string) (*models.EncodingProfile, error)
	// GetDefault retrieves the default encoding profile.
	GetDefault(ctx context.Context) (*models.EncodingProfile, error)
	// GetSystem retrieves all system encoding profiles.
	GetSystem(ctx context.Context) ([]*models.EncodingProfile, error)
	// Update updates an existing encoding profile.
	Update(ctx context.Context, profile *models.EncodingProfile) error
	// Delete deletes an encoding profile by ID.
	Delete(ctx context.Context, id models.ULID) error
	// Count returns the total number of encoding profiles.
	Count(ctx context.Context) (int64, error)
	// CountEnabled returns the number of enabled profiles.
	CountEnabled(ctx context.Context) (int64, error)
	// SetDefault sets a profile as the default (unsets previous default).
	SetDefault(ctx context.Context, id models.ULID) error
}

// ClientDetectionRuleRepository defines operations for client detection rule persistence.
type ClientDetectionRuleRepository interface {
	// Create creates a new client detection rule.
	Create(ctx context.Context, rule *models.ClientDetectionRule) error
	// GetByID retrieves a client detection rule by ID.
	GetByID(ctx context.Context, id models.ULID) (*models.ClientDetectionRule, error)
	// GetByIDs retrieves client detection rules by multiple IDs.
	GetByIDs(ctx context.Context, ids []models.ULID) ([]*models.ClientDetectionRule, error)
	// GetAll retrieves all client detection rules ordered by priority.
	GetAll(ctx context.Context) ([]*models.ClientDetectionRule, error)
	// GetEnabled retrieves all enabled rules ordered by priority.
	GetEnabled(ctx context.Context) ([]*models.ClientDetectionRule, error)
	// GetUserCreated retrieves all user-created client detection rules (IsSystem=false).
	GetUserCreated(ctx context.Context) ([]*models.ClientDetectionRule, error)
	// GetByName retrieves a rule by name.
	GetByName(ctx context.Context, name string) (*models.ClientDetectionRule, error)
	// GetSystem retrieves all system rules.
	GetSystem(ctx context.Context) ([]*models.ClientDetectionRule, error)
	// Update updates an existing rule.
	Update(ctx context.Context, rule *models.ClientDetectionRule) error
	// Delete deletes a rule by ID.
	Delete(ctx context.Context, id models.ULID) error
	// Count returns the total number of rules.
	Count(ctx context.Context) (int64, error)
	// CountEnabled returns the number of enabled rules.
	CountEnabled(ctx context.Context) (int64, error)
	// Reorder updates priorities for multiple rules in a single transaction.
	Reorder(ctx context.Context, reorders []ReorderRequest) error
}

// EncoderOverrideRepository defines operations for encoder override persistence.
// Encoder overrides allow forcing specific encoders when conditions match,
// working around hardware encoder bugs (like AMD's hevc_vaapi with Mesa 21.1+).
type EncoderOverrideRepository interface {
	// Create creates a new encoder override.
	Create(ctx context.Context, override *models.EncoderOverride) error
	// GetByID retrieves an encoder override by ID.
	GetByID(ctx context.Context, id models.ULID) (*models.EncoderOverride, error)
	// GetAll retrieves all encoder overrides ordered by priority (highest first).
	GetAll(ctx context.Context) ([]*models.EncoderOverride, error)
	// GetEnabled retrieves all enabled overrides ordered by priority (highest first).
	GetEnabled(ctx context.Context) ([]*models.EncoderOverride, error)
	// GetByCodecType retrieves overrides for a specific codec type (video/audio).
	GetByCodecType(ctx context.Context, codecType models.EncoderOverrideCodecType) ([]*models.EncoderOverride, error)
	// GetUserCreated retrieves all user-created encoder overrides (IsSystem=false).
	GetUserCreated(ctx context.Context) ([]*models.EncoderOverride, error)
	// GetByName retrieves an override by name.
	GetByName(ctx context.Context, name string) (*models.EncoderOverride, error)
	// GetSystem retrieves all system overrides.
	GetSystem(ctx context.Context) ([]*models.EncoderOverride, error)
	// Update updates an existing override.
	Update(ctx context.Context, override *models.EncoderOverride) error
	// Delete deletes an override by ID.
	Delete(ctx context.Context, id models.ULID) error
	// Count returns the total number of overrides.
	Count(ctx context.Context) (int64, error)
	// CountEnabled returns the number of enabled overrides.
	CountEnabled(ctx context.Context) (int64, error)
	// Reorder updates priorities for multiple overrides in a single transaction.
	Reorder(ctx context.Context, reorders []ReorderRequest) error
}
