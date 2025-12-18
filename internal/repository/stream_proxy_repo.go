package repository

import (
	"context"
	"fmt"

	"github.com/jmylchreest/tvarr/internal/models"
	"gorm.io/gorm"
)

// streamProxyRepo implements StreamProxyRepository using GORM.
type streamProxyRepo struct {
	db *gorm.DB
}

// NewStreamProxyRepository creates a new StreamProxyRepository.
func NewStreamProxyRepository(db *gorm.DB) *streamProxyRepo {
	return &streamProxyRepo{db: db}
}

// Create creates a new stream proxy.
func (r *streamProxyRepo) Create(ctx context.Context, proxy *models.StreamProxy) error {
	if err := r.db.WithContext(ctx).Create(proxy).Error; err != nil {
		return fmt.Errorf("creating stream proxy: %w", err)
	}
	return nil
}

// GetByID retrieves a stream proxy by ID.
func (r *streamProxyRepo) GetByID(ctx context.Context, id models.ULID) (*models.StreamProxy, error) {
	var proxy models.StreamProxy
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&proxy).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("getting stream proxy by ID: %w", err)
	}
	return &proxy, nil
}

// GetByIDWithRelations retrieves a stream proxy with its sources, EPG sources, and filters.
// Relations are sorted by their priority/order fields for correct UI display.
func (r *streamProxyRepo) GetByIDWithRelations(ctx context.Context, id models.ULID) (*models.StreamProxy, error) {
	var proxy models.StreamProxy
	if err := r.db.WithContext(ctx).
		Preload("Sources", func(db *gorm.DB) *gorm.DB {
			return db.Order("proxy_sources.priority ASC")
		}).
		Preload("Sources.Source").
		Preload("EpgSources", func(db *gorm.DB) *gorm.DB {
			return db.Order("proxy_epg_sources.priority ASC")
		}).
		Preload("EpgSources.EpgSource").
		Preload("Filters", func(db *gorm.DB) *gorm.DB {
			return db.Order("proxy_filters.priority ASC")
		}).
		Preload("Filters.Filter").
		Where("id = ?", id).
		First(&proxy).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("getting stream proxy with relations: %w", err)
	}
	return &proxy, nil
}

// GetAll retrieves all stream proxies.
func (r *streamProxyRepo) GetAll(ctx context.Context) ([]*models.StreamProxy, error) {
	var proxies []*models.StreamProxy
	if err := r.db.WithContext(ctx).Order("name ASC").Find(&proxies).Error; err != nil {
		return nil, fmt.Errorf("getting all stream proxies: %w", err)
	}
	return proxies, nil
}

// GetActive retrieves all active stream proxies.
func (r *streamProxyRepo) GetActive(ctx context.Context) ([]*models.StreamProxy, error) {
	var proxies []*models.StreamProxy
	if err := r.db.WithContext(ctx).Where("is_active = ?", true).Order("name ASC").Find(&proxies).Error; err != nil {
		return nil, fmt.Errorf("getting active stream proxies: %w", err)
	}
	return proxies, nil
}

// Update updates an existing stream proxy.
func (r *streamProxyRepo) Update(ctx context.Context, proxy *models.StreamProxy) error {
	if err := r.db.WithContext(ctx).Save(proxy).Error; err != nil {
		return fmt.Errorf("updating stream proxy: %w", err)
	}
	return nil
}

// Delete deletes a stream proxy by ID.
func (r *streamProxyRepo) Delete(ctx context.Context, id models.ULID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete association records first
		if err := tx.Where("proxy_id = ?", id).Delete(&models.ProxySource{}).Error; err != nil {
			return fmt.Errorf("deleting proxy sources: %w", err)
		}
		if err := tx.Where("proxy_id = ?", id).Delete(&models.ProxyEpgSource{}).Error; err != nil {
			return fmt.Errorf("deleting proxy epg sources: %w", err)
		}
		// Delete the proxy itself
		if err := tx.Where("id = ?", id).Delete(&models.StreamProxy{}).Error; err != nil {
			return fmt.Errorf("deleting stream proxy: %w", err)
		}
		return nil
	})
}

// GetByName retrieves a stream proxy by name.
func (r *streamProxyRepo) GetByName(ctx context.Context, name string) (*models.StreamProxy, error) {
	var proxy models.StreamProxy
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&proxy).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("getting stream proxy by name: %w", err)
	}
	return &proxy, nil
}

// UpdateStatus updates the generation status.
func (r *streamProxyRepo) UpdateStatus(ctx context.Context, id models.ULID, status models.StreamProxyStatus, lastError string) error {
	// Use UpdateColumns to skip hooks (BeforeUpdate validation requires full model)
	// Note: Must explicitly set updated_at since UpdateColumns bypasses GORM auto-update
	if err := r.db.WithContext(ctx).Model(&models.StreamProxy{}).Where("id = ?", id).UpdateColumns(map[string]interface{}{
		"status":     status,
		"last_error": lastError,
		"updated_at": models.Now(),
	}).Error; err != nil {
		return fmt.Errorf("updating proxy status: %w", err)
	}
	return nil
}

// UpdateLastGeneration updates the last generation timestamp and counts.
func (r *streamProxyRepo) UpdateLastGeneration(ctx context.Context, id models.ULID, channelCount, programCount int) error {
	now := models.Now()
	// Use UpdateColumns to skip hooks (BeforeUpdate validation requires full model)
	// Note: Must explicitly set updated_at since UpdateColumns bypasses GORM auto-update
	if err := r.db.WithContext(ctx).Model(&models.StreamProxy{}).Where("id = ?", id).UpdateColumns(map[string]interface{}{
		"status":            models.StreamProxyStatusSuccess,
		"last_generated_at": now,
		"channel_count":     channelCount,
		"program_count":     programCount,
		"last_error":        "",
		"updated_at":        now,
	}).Error; err != nil {
		return fmt.Errorf("updating last generation: %w", err)
	}
	return nil
}

// SetSources sets the stream sources for a proxy (replaces existing).
func (r *streamProxyRepo) SetSources(ctx context.Context, proxyID models.ULID, sourceIDs []models.ULID, priorities map[models.ULID]int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Hard delete existing associations (these are junction tables, no need for soft delete)
		if err := tx.Unscoped().Where("proxy_id = ?", proxyID).Delete(&models.ProxySource{}).Error; err != nil {
			return fmt.Errorf("clearing existing sources: %w", err)
		}

		// Create new associations
		for _, sourceID := range sourceIDs {
			priority := 0
			if priorities != nil {
				if p, ok := priorities[sourceID]; ok {
					priority = p
				}
			}
			ps := &models.ProxySource{
				ProxyID:  proxyID,
				SourceID: sourceID,
				Priority: priority,
			}
			if err := tx.Create(ps).Error; err != nil {
				return fmt.Errorf("adding source %s: %w", sourceID, err)
			}
		}
		return nil
	})
}

// SetEpgSources sets the EPG sources for a proxy (replaces existing).
func (r *streamProxyRepo) SetEpgSources(ctx context.Context, proxyID models.ULID, sourceIDs []models.ULID, priorities map[models.ULID]int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Hard delete existing associations (these are junction tables, no need for soft delete)
		if err := tx.Unscoped().Where("proxy_id = ?", proxyID).Delete(&models.ProxyEpgSource{}).Error; err != nil {
			return fmt.Errorf("clearing existing EPG sources: %w", err)
		}

		// Create new associations
		for _, sourceID := range sourceIDs {
			priority := 0
			if priorities != nil {
				if p, ok := priorities[sourceID]; ok {
					priority = p
				}
			}
			pes := &models.ProxyEpgSource{
				ProxyID:     proxyID,
				EpgSourceID: sourceID,
				Priority:    priority,
			}
			if err := tx.Create(pes).Error; err != nil {
				return fmt.Errorf("adding EPG source %s: %w", sourceID, err)
			}
		}
		return nil
	})
}

// GetSources retrieves the stream sources for a proxy with priority ordering.
func (r *streamProxyRepo) GetSources(ctx context.Context, proxyID models.ULID) ([]*models.StreamSource, error) {
	var sources []*models.StreamSource
	if err := r.db.WithContext(ctx).
		Joins("JOIN proxy_sources ON proxy_sources.source_id = stream_sources.id AND proxy_sources.deleted_at IS NULL").
		Where("proxy_sources.proxy_id = ?", proxyID).
		Order("proxy_sources.priority DESC, stream_sources.name ASC").
		Find(&sources).Error; err != nil {
		return nil, fmt.Errorf("getting proxy sources: %w", err)
	}
	return sources, nil
}

// GetEpgSources retrieves the EPG sources for a proxy with priority ordering.
func (r *streamProxyRepo) GetEpgSources(ctx context.Context, proxyID models.ULID) ([]*models.EpgSource, error) {
	var sources []*models.EpgSource
	if err := r.db.WithContext(ctx).
		Joins("JOIN proxy_epg_sources ON proxy_epg_sources.epg_source_id = epg_sources.id AND proxy_epg_sources.deleted_at IS NULL").
		Where("proxy_epg_sources.proxy_id = ?", proxyID).
		Order("proxy_epg_sources.priority DESC, epg_sources.name ASC").
		Find(&sources).Error; err != nil {
		return nil, fmt.Errorf("getting proxy EPG sources: %w", err)
	}
	return sources, nil
}

// SetFilters sets the filters for a proxy (replaces existing).
// The isActive map controls whether each filter is active (applied during generation).
func (r *streamProxyRepo) SetFilters(ctx context.Context, proxyID models.ULID, filterIDs []models.ULID, priorities map[models.ULID]int, isActive map[models.ULID]bool) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Hard delete existing associations (these are junction tables, no need for soft delete)
		if err := tx.Unscoped().Where("proxy_id = ?", proxyID).Delete(&models.ProxyFilter{}).Error; err != nil {
			return fmt.Errorf("clearing existing filters: %w", err)
		}

		// Create new associations
		for _, filterID := range filterIDs {
			priority := 0
			if priorities != nil {
				if p, ok := priorities[filterID]; ok {
					priority = p
				}
			}
			// Default to active if not specified
			active := true
			if isActive != nil {
				if a, ok := isActive[filterID]; ok {
					active = a
				}
			}
			pf := &models.ProxyFilter{
				ProxyID:  proxyID,
				FilterID: filterID,
				Priority: priority,
				IsActive: &active, // Pointer allows GORM to distinguish nil from false
			}
			if err := tx.Create(pf).Error; err != nil {
				return fmt.Errorf("adding filter %s: %w", filterID, err)
			}
		}
		return nil
	})
}

// GetFilters retrieves the filters for a proxy with priority ordering.
func (r *streamProxyRepo) GetFilters(ctx context.Context, proxyID models.ULID) ([]*models.Filter, error) {
	var filters []*models.Filter
	if err := r.db.WithContext(ctx).
		Joins("JOIN proxy_filters ON proxy_filters.filter_id = filters.id AND proxy_filters.deleted_at IS NULL").
		Where("proxy_filters.proxy_id = ?", proxyID).
		Order("proxy_filters.priority ASC, filters.name ASC").
		Find(&filters).Error; err != nil {
		return nil, fmt.Errorf("getting proxy filters: %w", err)
	}
	return filters, nil
}

// GetBySourceID retrieves all proxies that use a specific stream source.
// Used for auto-regeneration when a source is updated.
func (r *streamProxyRepo) GetBySourceID(ctx context.Context, sourceID models.ULID) ([]*models.StreamProxy, error) {
	var proxies []*models.StreamProxy
	if err := r.db.WithContext(ctx).
		Joins("JOIN proxy_sources ON proxy_sources.proxy_id = stream_proxies.id AND proxy_sources.deleted_at IS NULL").
		Where("proxy_sources.source_id = ?", sourceID).
		Where("stream_proxies.is_active = ?", true).
		Order("stream_proxies.name ASC").
		Find(&proxies).Error; err != nil {
		return nil, fmt.Errorf("getting proxies by source ID: %w", err)
	}
	return proxies, nil
}

// GetByEpgSourceID retrieves all proxies that use a specific EPG source.
// Used for auto-regeneration when an EPG source is updated.
func (r *streamProxyRepo) GetByEpgSourceID(ctx context.Context, epgSourceID models.ULID) ([]*models.StreamProxy, error) {
	var proxies []*models.StreamProxy
	if err := r.db.WithContext(ctx).
		Joins("JOIN proxy_epg_sources ON proxy_epg_sources.proxy_id = stream_proxies.id AND proxy_epg_sources.deleted_at IS NULL").
		Where("proxy_epg_sources.epg_source_id = ?", epgSourceID).
		Where("stream_proxies.is_active = ?", true).
		Order("stream_proxies.name ASC").
		Find(&proxies).Error; err != nil {
		return nil, fmt.Errorf("getting proxies by EPG source ID: %w", err)
	}
	return proxies, nil
}

// CountByEncodingProfileID returns the count of stream proxies using a given encoding profile.
func (r *streamProxyRepo) CountByEncodingProfileID(ctx context.Context, profileID models.ULID) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&models.StreamProxy{}).
		Where("encoding_profile_id = ?", profileID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("counting proxies by encoding profile ID: %w", err)
	}
	return count, nil
}

// GetByEncodingProfileID returns stream proxies using a given encoding profile.
func (r *streamProxyRepo) GetByEncodingProfileID(ctx context.Context, profileID models.ULID) ([]*models.StreamProxy, error) {
	var proxies []*models.StreamProxy
	if err := r.db.WithContext(ctx).
		Where("encoding_profile_id = ?", profileID).
		Order("name ASC").
		Find(&proxies).Error; err != nil {
		return nil, fmt.Errorf("getting proxies by encoding profile ID: %w", err)
	}
	return proxies, nil
}

// CountByStreamSourceID returns the count of all proxies using a stream source.
func (r *streamProxyRepo) CountByStreamSourceID(ctx context.Context, sourceID models.ULID) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&models.ProxySource{}).
		Where("source_id = ?", sourceID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("counting proxies by stream source ID: %w", err)
	}
	return count, nil
}

// CountByEpgSourceID returns the count of all proxies using an EPG source.
func (r *streamProxyRepo) CountByEpgSourceID(ctx context.Context, epgSourceID models.ULID) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&models.ProxyEpgSource{}).
		Where("epg_source_id = ?", epgSourceID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("counting proxies by EPG source ID: %w", err)
	}
	return count, nil
}

// CountByFilterID returns the count of all proxies using a filter.
func (r *streamProxyRepo) CountByFilterID(ctx context.Context, filterID models.ULID) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&models.ProxyFilter{}).
		Where("filter_id = ?", filterID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("counting proxies by filter ID: %w", err)
	}
	return count, nil
}

// GetProxyNamesByStreamSourceID returns names of proxies using a stream source.
func (r *streamProxyRepo) GetProxyNamesByStreamSourceID(ctx context.Context, sourceID models.ULID) ([]string, error) {
	var names []string
	if err := r.db.WithContext(ctx).
		Model(&models.StreamProxy{}).
		Joins("JOIN proxy_sources ON proxy_sources.proxy_id = stream_proxies.id AND proxy_sources.deleted_at IS NULL").
		Where("proxy_sources.source_id = ?", sourceID).
		Pluck("stream_proxies.name", &names).Error; err != nil {
		return nil, fmt.Errorf("getting proxy names by stream source ID: %w", err)
	}
	return names, nil
}

// GetProxyNamesByEpgSourceID returns names of proxies using an EPG source.
func (r *streamProxyRepo) GetProxyNamesByEpgSourceID(ctx context.Context, epgSourceID models.ULID) ([]string, error) {
	var names []string
	if err := r.db.WithContext(ctx).
		Model(&models.StreamProxy{}).
		Joins("JOIN proxy_epg_sources ON proxy_epg_sources.proxy_id = stream_proxies.id AND proxy_epg_sources.deleted_at IS NULL").
		Where("proxy_epg_sources.epg_source_id = ?", epgSourceID).
		Pluck("stream_proxies.name", &names).Error; err != nil {
		return nil, fmt.Errorf("getting proxy names by EPG source ID: %w", err)
	}
	return names, nil
}

// GetProxyNamesByFilterID returns names of proxies using a filter.
func (r *streamProxyRepo) GetProxyNamesByFilterID(ctx context.Context, filterID models.ULID) ([]string, error) {
	var names []string
	if err := r.db.WithContext(ctx).
		Model(&models.StreamProxy{}).
		Joins("JOIN proxy_filters ON proxy_filters.proxy_id = stream_proxies.id AND proxy_filters.deleted_at IS NULL").
		Where("proxy_filters.filter_id = ?", filterID).
		Pluck("stream_proxies.name", &names).Error; err != nil {
		return nil, fmt.Errorf("getting proxy names by filter ID: %w", err)
	}
	return names, nil
}

// GetProxyNamesByEncodingProfileID returns names of proxies using an encoding profile.
func (r *streamProxyRepo) GetProxyNamesByEncodingProfileID(ctx context.Context, profileID models.ULID) ([]string, error) {
	var names []string
	if err := r.db.WithContext(ctx).
		Model(&models.StreamProxy{}).
		Where("encoding_profile_id = ?", profileID).
		Pluck("name", &names).Error; err != nil {
		return nil, fmt.Errorf("getting proxy names by encoding profile ID: %w", err)
	}
	return names, nil
}

// Ensure streamProxyRepo implements StreamProxyRepository at compile time.
var _ StreamProxyRepository = (*streamProxyRepo)(nil)
