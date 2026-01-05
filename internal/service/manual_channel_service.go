// Package service provides business logic layer for tvarr operations.
package service

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/jmylchreest/tvarr/pkg/m3u"
)

// ManualChannelRepository defines the interface for manual channel persistence.
type ManualChannelRepository interface {
	Create(ctx context.Context, channel *models.ManualStreamChannel) error
	GetByID(ctx context.Context, id models.ULID) (*models.ManualStreamChannel, error)
	GetAll(ctx context.Context) ([]*models.ManualStreamChannel, error)
	GetBySourceID(ctx context.Context, sourceID models.ULID) ([]*models.ManualStreamChannel, error)
	GetEnabledBySourceID(ctx context.Context, sourceID models.ULID) ([]*models.ManualStreamChannel, error)
	Update(ctx context.Context, channel *models.ManualStreamChannel) error
	Delete(ctx context.Context, id models.ULID) error
	DeleteBySourceID(ctx context.Context, sourceID models.ULID) error
	CountBySourceID(ctx context.Context, sourceID models.ULID) (int64, error)
}

// M3UImportResult represents the result of an M3U import operation.
type M3UImportResult struct {
	ParsedCount  int                           `json:"parsed_count"`
	SkippedCount int                           `json:"skipped_count"`
	Applied      bool                          `json:"applied"`
	Channels     []*models.ManualStreamChannel `json:"channels"`
	Errors       []string                      `json:"errors,omitempty"`
}

// ManualChannelServiceInterface defines the service interface for manual channels.
type ManualChannelServiceInterface interface {
	ListBySourceID(ctx context.Context, sourceID models.ULID) ([]*models.ManualStreamChannel, error)
	ReplaceChannels(ctx context.Context, sourceID models.ULID, channels []*models.ManualStreamChannel) ([]*models.ManualStreamChannel, error)
	ValidateChannel(ctx context.Context, channel *models.ManualStreamChannel) error
	DetectDuplicateChannelNumbers(ctx context.Context, channels []*models.ManualStreamChannel) []string
	ParseM3U(ctx context.Context, sourceID models.ULID, m3uContent string) (*M3UImportResult, error)
	ImportM3U(ctx context.Context, sourceID models.ULID, m3uContent string, apply bool) (*M3UImportResult, error)
	ExportM3U(ctx context.Context, sourceID models.ULID) (string, error)
}

// ManualChannelService provides business logic for manual channel management.
type ManualChannelService struct {
	channelRepo ManualChannelRepository
	sourceRepo  repository.StreamSourceRepository
	logger      *slog.Logger
}

// NewManualChannelService creates a new manual channel service.
func NewManualChannelService(
	channelRepo ManualChannelRepository,
	sourceRepo repository.StreamSourceRepository,
) *ManualChannelService {
	return &ManualChannelService{
		channelRepo: channelRepo,
		sourceRepo:  sourceRepo,
		logger:      slog.Default(),
	}
}

// WithLogger sets the logger for the service.
func (s *ManualChannelService) WithLogger(logger *slog.Logger) *ManualChannelService {
	if logger != nil {
		s.logger = logger
	}
	return s
}

// ListBySourceID retrieves all manual channels for a source.
// Returns an error if the source doesn't exist or is not a manual source.
func (s *ManualChannelService) ListBySourceID(ctx context.Context, sourceID models.ULID) ([]*models.ManualStreamChannel, error) {
	// Verify source exists and is manual type
	source, err := s.sourceRepo.GetByID(ctx, sourceID)
	if err != nil {
		return nil, fmt.Errorf("getting source: %w", err)
	}
	if source == nil {
		return nil, fmt.Errorf("source not found")
	}
	if source.Type != models.SourceTypeManual {
		return nil, fmt.Errorf("operation only valid for manual sources")
	}

	channels, err := s.channelRepo.GetBySourceID(ctx, sourceID)
	if err != nil {
		return nil, fmt.Errorf("listing channels: %w", err)
	}

	s.logger.Debug("listed manual channels",
		"source_id", sourceID,
		"count", len(channels))

	return channels, nil
}

// ReplaceChannels atomically replaces all channels for a manual source.
// Validates each channel and returns an error if any validation fails.
// Returns the created channels with their assigned IDs.
func (s *ManualChannelService) ReplaceChannels(ctx context.Context, sourceID models.ULID, channels []*models.ManualStreamChannel) ([]*models.ManualStreamChannel, error) {
	// Verify source exists and is manual type
	source, err := s.sourceRepo.GetByID(ctx, sourceID)
	if err != nil {
		return nil, fmt.Errorf("getting source: %w", err)
	}
	if source == nil {
		return nil, fmt.Errorf("source not found")
	}
	if source.Type != models.SourceTypeManual {
		return nil, fmt.Errorf("operation only valid for manual sources")
	}

	// Validate at least one channel (FR-011b)
	if len(channels) == 0 {
		return nil, fmt.Errorf("at least one channel is required")
	}

	// Validate all channels before making any changes
	for i, ch := range channels {
		if err := s.ValidateChannel(ctx, ch); err != nil {
			return nil, fmt.Errorf("channel %d: %w", i, err)
		}
		// Set source ID
		ch.SourceID = sourceID
	}

	// Check for duplicate channel numbers (warn but don't fail)
	warnings := s.DetectDuplicateChannelNumbers(ctx, channels)
	for _, w := range warnings {
		s.logger.Warn("duplicate channel number detected", "warning", w, "source_id", sourceID)
	}

	// Delete existing channels
	if err := s.channelRepo.DeleteBySourceID(ctx, sourceID); err != nil {
		return nil, fmt.Errorf("deleting existing channels: %w", err)
	}

	// Create new channels
	result := make([]*models.ManualStreamChannel, 0, len(channels))
	for _, ch := range channels {
		if err := s.channelRepo.Create(ctx, ch); err != nil {
			return nil, fmt.Errorf("creating channel: %w", err)
		}
		result = append(result, ch)
	}

	s.logger.Info("replaced manual channels",
		"source_id", sourceID,
		"count", len(result))

	return result, nil
}

// ValidateChannel validates a single manual channel.
// Checks:
// - channel_name is non-empty (FR-010)
// - stream_url starts with http://, https://, or rtsp:// (FR-011)
// - tvg_logo is empty, @logo:*, or http(s):// URL (FR-012)
func (s *ManualChannelService) ValidateChannel(ctx context.Context, channel *models.ManualStreamChannel) error {
	// FR-010: Require non-empty channel_name
	if strings.TrimSpace(channel.ChannelName) == "" {
		return fmt.Errorf("channel name is required")
	}

	// FR-011: Require stream_url with valid protocol
	if strings.TrimSpace(channel.StreamURL) == "" {
		return fmt.Errorf("stream URL is required")
	}

	streamURL := strings.ToLower(channel.StreamURL)
	if !strings.HasPrefix(streamURL, "http://") &&
		!strings.HasPrefix(streamURL, "https://") &&
		!strings.HasPrefix(streamURL, "rtsp://") {
		return fmt.Errorf("stream URL must be http, https, or rtsp")
	}

	// FR-012: Validate tvg_logo format
	if channel.TvgLogo != "" {
		if err := s.validateLogoFormat(channel.TvgLogo); err != nil {
			return err
		}
	}

	return nil
}

// validateLogoFormat validates that a logo reference is in an acceptable format.
// Valid formats:
// - Empty string
// - @logo:* token (e.g., @logo:channelname)
// - http:// or https:// URL
func (s *ManualChannelService) validateLogoFormat(logo string) error {
	if logo == "" {
		return nil
	}

	// Check for @logo: token format
	if after, ok := strings.CutPrefix(logo, "@logo:"); ok {
		token := after
		if token == "" {
			return fmt.Errorf("invalid logo reference format: @logo: requires a token")
		}
		return nil
	}

	// Check for valid URL
	lowerLogo := strings.ToLower(logo)
	if strings.HasPrefix(lowerLogo, "http://") || strings.HasPrefix(lowerLogo, "https://") {
		return nil
	}

	return fmt.Errorf("invalid logo reference format: must be empty, @logo:token, or http(s):// URL")
}

// DetectDuplicateChannelNumbers checks for duplicate non-zero channel numbers.
// Returns a list of warning messages for each duplicate found.
func (s *ManualChannelService) DetectDuplicateChannelNumbers(ctx context.Context, channels []*models.ManualStreamChannel) []string {
	// Track channel numbers and their occurrences
	seen := make(map[int]int)
	for _, ch := range channels {
		if ch.ChannelNumber > 0 {
			seen[ch.ChannelNumber]++
		}
	}

	// Generate warnings for duplicates
	var warnings []string
	for num, count := range seen {
		if count > 1 {
			warnings = append(warnings, fmt.Sprintf("duplicate channel number: %d (appears %d times)", num, count))
		}
	}

	return warnings
}

// ParseM3U parses M3U content and returns channels without persisting them.
func (s *ManualChannelService) ParseM3U(ctx context.Context, sourceID models.ULID, m3uContent string) (*M3UImportResult, error) {
	// Verify source exists and is manual type
	source, err := s.sourceRepo.GetByID(ctx, sourceID)
	if err != nil {
		return nil, fmt.Errorf("getting source: %w", err)
	}
	if source == nil {
		return nil, fmt.Errorf("source not found")
	}
	if source.Type != models.SourceTypeManual {
		return nil, fmt.Errorf("operation only valid for manual sources")
	}

	result := &M3UImportResult{
		Channels: make([]*models.ManualStreamChannel, 0),
		Errors:   make([]string, 0),
	}

	parser := &m3u.Parser{
		OnEntry: func(entry *m3u.Entry) error {
			// Convert M3U entry to ManualStreamChannel
			ch := &models.ManualStreamChannel{
				SourceID:      sourceID,
				TvgID:         entry.TvgID,
				TvgName:       entry.TvgName,
				TvgLogo:       entry.TvgLogo,
				GroupTitle:    entry.GroupTitle,
				ChannelName:   entry.Title,
				ChannelNumber: entry.ChannelNumber,
				StreamURL:     entry.URL,
				Enabled:       models.BoolPtr(true),
				Priority:      0,
			}

			// Validate the channel
			if err := s.ValidateChannel(ctx, ch); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("entry '%s': %v", entry.Title, err))
				result.SkippedCount++
				return nil // Continue parsing, don't fail
			}

			result.Channels = append(result.Channels, ch)
			result.ParsedCount++
			return nil
		},
		OnError: func(lineNum int, err error) {
			result.Errors = append(result.Errors, fmt.Sprintf("line %d: %v", lineNum, err))
		},
	}

	if err := parser.Parse(strings.NewReader(m3uContent)); err != nil {
		return nil, fmt.Errorf("parsing M3U: %w", err)
	}

	s.logger.Debug("parsed M3U content",
		"source_id", sourceID,
		"parsed", result.ParsedCount,
		"skipped", result.SkippedCount)

	return result, nil
}

// ImportM3U parses M3U content and optionally applies it to the source.
func (s *ManualChannelService) ImportM3U(ctx context.Context, sourceID models.ULID, m3uContent string, apply bool) (*M3UImportResult, error) {
	// Parse the M3U content
	result, err := s.ParseM3U(ctx, sourceID, m3uContent)
	if err != nil {
		return nil, err
	}

	if !apply {
		// Preview mode - just return parsed channels
		return result, nil
	}

	// Apply mode - replace existing channels
	if len(result.Channels) == 0 {
		return nil, fmt.Errorf("no valid channels to import")
	}

	replaced, err := s.ReplaceChannels(ctx, sourceID, result.Channels)
	if err != nil {
		return nil, fmt.Errorf("applying imported channels: %w", err)
	}

	result.Channels = replaced
	result.Applied = true

	s.logger.Info("imported M3U channels",
		"source_id", sourceID,
		"applied", len(replaced))

	return result, nil
}

// ExportM3U exports manual channels as M3U playlist content.
func (s *ManualChannelService) ExportM3U(ctx context.Context, sourceID models.ULID) (string, error) {
	channels, err := s.ListBySourceID(ctx, sourceID)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	writer := m3u.NewWriter(&buf)

	for _, ch := range channels {
		entry := &m3u.Entry{
			Duration:      -1,
			TvgID:         ch.TvgID,
			TvgName:       ch.TvgName,
			TvgLogo:       ch.TvgLogo,
			GroupTitle:    ch.GroupTitle,
			Title:         ch.ChannelName,
			ChannelNumber: ch.ChannelNumber,
			URL:           ch.StreamURL,
			Extra:         make(map[string]string),
		}

		if err := writer.WriteEntry(entry); err != nil {
			return "", fmt.Errorf("writing entry: %w", err)
		}
	}

	s.logger.Debug("exported M3U",
		"source_id", sourceID,
		"channels", len(channels))

	return buf.String(), nil
}
