package ingestor

import (
	"context"
	"fmt"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
)

// ManualHandler handles ingestion of Manual stream sources.
// Unlike M3U and Xtream handlers, Manual handlers don't fetch from a remote URL.
// Instead, they "materialize" channels from the manual_stream_channels table
// into the main channels table.
type ManualHandler struct {
	repo repository.ManualStreamChannelRepository
}

// NewManualHandler creates a new Manual handler.
// The handler requires a ManualStreamChannelRepository to read manual channel definitions.
func NewManualHandler(repo repository.ManualStreamChannelRepository) *ManualHandler {
	return &ManualHandler{
		repo: repo,
	}
}

// Type returns the source type this handler supports.
func (h *ManualHandler) Type() models.SourceType {
	return models.SourceTypeManual
}

// Validate checks if the source configuration is valid for Manual ingestion.
// Manual sources have minimal validation since they don't require a URL.
func (h *ManualHandler) Validate(source *models.StreamSource) error {
	if source == nil {
		return fmt.Errorf("source is nil")
	}
	if source.Type != models.SourceTypeManual {
		return fmt.Errorf("source type must be manual, got %s", source.Type)
	}
	// Manual sources don't require a URL - channels are defined in the database
	return nil
}

// Ingest materializes channels from the manual_stream_channels table.
// It reads all enabled manual channels for the source and calls the callback
// for each one, converting them to the main Channel model.
func (h *ManualHandler) Ingest(ctx context.Context, source *models.StreamSource, callback ChannelCallback) error {
	if err := h.Validate(source); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if h.repo == nil {
		return fmt.Errorf("manual channel repository not configured")
	}

	// Get all enabled manual channels for this source
	manualChannels, err := h.repo.GetEnabledBySourceID(ctx, source.ID)
	if err != nil {
		return fmt.Errorf("fetching manual channels: %w", err)
	}

	// Materialize each manual channel to the main Channel format
	for _, mc := range manualChannels {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Convert ManualStreamChannel to Channel
		channel := mc.ToChannel()

		// Call the callback
		if err := callback(channel); err != nil {
			return fmt.Errorf("callback error: %w", err)
		}
	}

	return nil
}
