package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmylchreest/tvarr/internal/ingestor"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
)

// mockStreamSourceRepo is a mock implementation of StreamSourceRepository
type mockStreamSourceRepo struct {
	sources map[models.ULID]*models.StreamSource
}

func newMockStreamSourceRepo() *mockStreamSourceRepo {
	return &mockStreamSourceRepo{
		sources: make(map[models.ULID]*models.StreamSource),
	}
}

func (r *mockStreamSourceRepo) Create(ctx context.Context, source *models.StreamSource) error {
	source.ID = models.NewULID()
	r.sources[source.ID] = source
	return nil
}

func (r *mockStreamSourceRepo) Update(ctx context.Context, source *models.StreamSource) error {
	if _, exists := r.sources[source.ID]; !exists {
		return errors.New("source not found")
	}
	r.sources[source.ID] = source
	return nil
}

func (r *mockStreamSourceRepo) Delete(ctx context.Context, id models.ULID) error {
	delete(r.sources, id)
	return nil
}

func (r *mockStreamSourceRepo) GetByID(ctx context.Context, id models.ULID) (*models.StreamSource, error) {
	source, exists := r.sources[id]
	if !exists {
		return nil, errors.New("source not found")
	}
	return source, nil
}

func (r *mockStreamSourceRepo) GetByName(ctx context.Context, name string) (*models.StreamSource, error) {
	for _, s := range r.sources {
		if s.Name == name {
			return s, nil
		}
	}
	return nil, errors.New("source not found")
}

func (r *mockStreamSourceRepo) GetAll(ctx context.Context) ([]*models.StreamSource, error) {
	sources := make([]*models.StreamSource, 0, len(r.sources))
	for _, s := range r.sources {
		sources = append(sources, s)
	}
	return sources, nil
}

func (r *mockStreamSourceRepo) GetEnabled(ctx context.Context) ([]*models.StreamSource, error) {
	sources := make([]*models.StreamSource, 0)
	for _, s := range r.sources {
		if models.BoolVal(s.Enabled) {
			sources = append(sources, s)
		}
	}
	return sources, nil
}

func (r *mockStreamSourceRepo) UpdateLastIngestion(ctx context.Context, id models.ULID, status string, channelCount int) error {
	return nil
}

// mockChannelRepo is a mock implementation of ChannelRepository
type mockChannelRepo struct {
	channels    map[models.ULID]*models.Channel
	deleteCount int
}

func newMockChannelRepo() *mockChannelRepo {
	return &mockChannelRepo{
		channels: make(map[models.ULID]*models.Channel),
	}
}

func (r *mockChannelRepo) Create(ctx context.Context, channel *models.Channel) error {
	channel.ID = models.NewULID()
	r.channels[channel.ID] = channel
	return nil
}

func (r *mockChannelRepo) CreateBatch(ctx context.Context, channels []*models.Channel) error {
	for _, ch := range channels {
		if err := r.Create(ctx, ch); err != nil {
			return err
		}
	}
	return nil
}

func (r *mockChannelRepo) UpsertBatch(ctx context.Context, channels []*models.Channel) error {
	for _, ch := range channels {
		// Find existing channel by source_id + ext_id
		var existing *models.Channel
		for _, existingCh := range r.channels {
			if existingCh.SourceID == ch.SourceID && existingCh.ExtID == ch.ExtID {
				existing = existingCh
				break
			}
		}
		if existing != nil {
			// Update existing channel
			existing.TvgID = ch.TvgID
			existing.TvgName = ch.TvgName
			existing.TvgLogo = ch.TvgLogo
			existing.GroupTitle = ch.GroupTitle
			existing.ChannelName = ch.ChannelName
			existing.ChannelNumber = ch.ChannelNumber
			existing.StreamURL = ch.StreamURL
			existing.StreamType = ch.StreamType
			existing.Language = ch.Language
			existing.Country = ch.Country
			existing.IsAdult = ch.IsAdult
			existing.Extra = ch.Extra
		} else {
			// Create new channel
			if err := r.Create(ctx, ch); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *mockChannelRepo) Update(ctx context.Context, channel *models.Channel) error {
	r.channels[channel.ID] = channel
	return nil
}

func (r *mockChannelRepo) Delete(ctx context.Context, id models.ULID) error {
	delete(r.channels, id)
	r.deleteCount++
	return nil
}

func (r *mockChannelRepo) GetByID(ctx context.Context, id models.ULID) (*models.Channel, error) {
	ch, exists := r.channels[id]
	if !exists {
		return nil, errors.New("channel not found")
	}
	return ch, nil
}

func (r *mockChannelRepo) GetByIDWithSource(ctx context.Context, id models.ULID) (*models.Channel, error) {
	ch, exists := r.channels[id]
	if !exists {
		return nil, errors.New("channel not found")
	}
	return ch, nil
}

func (r *mockChannelRepo) GetBySourceID(ctx context.Context, sourceID models.ULID, callback func(*models.Channel) error) error {
	for _, ch := range r.channels {
		if ch.SourceID == sourceID {
			if err := callback(ch); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *mockChannelRepo) GetBySourceIDPaginated(ctx context.Context, sourceID models.ULID, offset, limit int) ([]*models.Channel, int64, error) {
	var channels []*models.Channel
	for _, ch := range r.channels {
		if ch.SourceID == sourceID {
			channels = append(channels, ch)
		}
	}
	total := int64(len(channels))
	// Simple pagination
	if offset >= len(channels) {
		return []*models.Channel{}, total, nil
	}
	end := min(offset+limit, len(channels))
	return channels[offset:end], total, nil
}

func (r *mockChannelRepo) GetByExtID(ctx context.Context, sourceID models.ULID, extID string) (*models.Channel, error) {
	for _, ch := range r.channels {
		if ch.SourceID == sourceID && ch.ExtID == extID {
			return ch, nil
		}
	}
	return nil, errors.New("channel not found")
}

func (r *mockChannelRepo) DeleteBySourceID(ctx context.Context, sourceID models.ULID) error {
	for id, ch := range r.channels {
		if ch.SourceID == sourceID {
			delete(r.channels, id)
			r.deleteCount++
		}
	}
	return nil
}

func (r *mockChannelRepo) DeleteStaleBySourceID(ctx context.Context, sourceID models.ULID, olderThan time.Time) (int64, error) {
	var deleted int64
	for id, ch := range r.channels {
		if ch.SourceID == sourceID && ch.UpdatedAt.Before(olderThan) {
			delete(r.channels, id)
			deleted++
		}
	}
	return deleted, nil
}

func (r *mockChannelRepo) CountBySourceID(ctx context.Context, sourceID models.ULID) (int64, error) {
	var count int64
	for _, ch := range r.channels {
		if ch.SourceID == sourceID {
			count++
		}
	}
	return count, nil
}

func (r *mockChannelRepo) Transaction(ctx context.Context, fn func(repository.ChannelRepository) error) error {
	// For the mock, just execute the function directly with this repo
	// In real usage, this would wrap in a database transaction
	return fn(r)
}

func (r *mockChannelRepo) GetDistinctFieldValues(ctx context.Context, field string, query string, limit int) ([]repository.FieldValueResult, error) {
	// Mock implementation - return empty results
	return []repository.FieldValueResult{}, nil
}

func (r *mockChannelRepo) GetAllStreaming(ctx context.Context, callback func(*models.Channel) error) error {
	for _, ch := range r.channels {
		if err := callback(ch); err != nil {
			return err
		}
	}
	return nil
}

func TestSourceService_CreateSource(t *testing.T) {
	sourceRepo := newMockStreamSourceRepo()
	channelRepo := newMockChannelRepo()
	factory := ingestor.NewHandlerFactory()
	stateManager := ingestor.NewStateManager()

	svc := NewSourceService(sourceRepo, channelRepo, factory, stateManager)

	source := &models.StreamSource{
		Name: "Test Source",
		Type: models.SourceTypeM3U,
		URL:  "http://example.com/playlist.m3u",
	}

	err := svc.Create(context.Background(), source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if source.ID.IsZero() {
		t.Error("expected ID to be set")
	}
}

func TestSourceService_GetSource(t *testing.T) {
	sourceRepo := newMockStreamSourceRepo()
	channelRepo := newMockChannelRepo()
	factory := ingestor.NewHandlerFactory()
	stateManager := ingestor.NewStateManager()

	svc := NewSourceService(sourceRepo, channelRepo, factory, stateManager)

	// Create a source
	source := &models.StreamSource{
		Name: "Test Source",
		Type: models.SourceTypeM3U,
		URL:  "http://example.com/playlist.m3u",
	}
	_ = svc.Create(context.Background(), source)

	// Get it back
	retrieved, err := svc.GetByID(context.Background(), source.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if retrieved.Name != "Test Source" {
		t.Errorf("expected name 'Test Source', got %q", retrieved.Name)
	}
}

func TestSourceService_ListSources(t *testing.T) {
	sourceRepo := newMockStreamSourceRepo()
	channelRepo := newMockChannelRepo()
	factory := ingestor.NewHandlerFactory()
	stateManager := ingestor.NewStateManager()

	svc := NewSourceService(sourceRepo, channelRepo, factory, stateManager)

	// Create sources
	_ = svc.Create(context.Background(), &models.StreamSource{
		Name: "Source 1", Type: models.SourceTypeM3U, URL: "http://1.com",
	})
	_ = svc.Create(context.Background(), &models.StreamSource{
		Name: "Source 2", Type: models.SourceTypeM3U, URL: "http://2.com",
	})

	sources, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sources) != 2 {
		t.Errorf("expected 2 sources, got %d", len(sources))
	}
}

func TestSourceService_DeleteSource(t *testing.T) {
	sourceRepo := newMockStreamSourceRepo()
	channelRepo := newMockChannelRepo()
	factory := ingestor.NewHandlerFactory()
	stateManager := ingestor.NewStateManager()

	svc := NewSourceService(sourceRepo, channelRepo, factory, stateManager)

	// Create a source
	source := &models.StreamSource{
		Name: "Test Source",
		Type: models.SourceTypeM3U,
		URL:  "http://example.com/playlist.m3u",
	}
	_ = svc.Create(context.Background(), source)

	// Delete it
	err := svc.Delete(context.Background(), source.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not exist anymore
	_, err = svc.GetByID(context.Background(), source.ID)
	if err == nil {
		t.Error("expected error for deleted source")
	}
}

func TestSourceService_DeleteSource_CleansUpChannels(t *testing.T) {
	sourceRepo := newMockStreamSourceRepo()
	channelRepo := newMockChannelRepo()
	factory := ingestor.NewHandlerFactory()
	stateManager := ingestor.NewStateManager()

	svc := NewSourceService(sourceRepo, channelRepo, factory, stateManager)

	// Create a source
	source := &models.StreamSource{
		Name: "Test Source",
		Type: models.SourceTypeM3U,
		URL:  "http://example.com/playlist.m3u",
	}
	_ = svc.Create(context.Background(), source)

	// Add some channels
	_ = channelRepo.Create(context.Background(), &models.Channel{
		SourceID:    source.ID,
		ChannelName: "Ch 1",
		StreamURL:   "http://1.com",
	})
	_ = channelRepo.Create(context.Background(), &models.Channel{
		SourceID:    source.ID,
		ChannelName: "Ch 2",
		StreamURL:   "http://2.com",
	})

	// Delete the source
	err := svc.Delete(context.Background(), source.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Channels should be deleted
	count, _ := channelRepo.CountBySourceID(context.Background(), source.ID)
	if count != 0 {
		t.Errorf("expected 0 channels after delete, got %d", count)
	}
}

func TestSourceService_GetIngestionState(t *testing.T) {
	sourceRepo := newMockStreamSourceRepo()
	channelRepo := newMockChannelRepo()
	factory := ingestor.NewHandlerFactory()
	stateManager := ingestor.NewStateManager()

	svc := NewSourceService(sourceRepo, channelRepo, factory, stateManager)

	// Create a source
	source := &models.StreamSource{
		Name: "Test Source",
		Type: models.SourceTypeM3U,
		URL:  "http://example.com/playlist.m3u",
	}
	_ = svc.Create(context.Background(), source)

	// Initially not ingesting
	_, exists := svc.GetIngestionState(source.ID)
	if exists {
		t.Error("expected no ingestion state initially")
	}

	// Simulate starting ingestion
	_ = stateManager.Start(source)

	state, exists := svc.GetIngestionState(source.ID)
	if !exists {
		t.Fatal("expected ingestion state after start")
	}
	if state.Status != "ingesting" {
		t.Errorf("expected status 'ingesting', got %q", state.Status)
	}
}

func TestSourceService_IsIngesting(t *testing.T) {
	sourceRepo := newMockStreamSourceRepo()
	channelRepo := newMockChannelRepo()
	factory := ingestor.NewHandlerFactory()
	stateManager := ingestor.NewStateManager()

	svc := NewSourceService(sourceRepo, channelRepo, factory, stateManager)

	// Create a source
	source := &models.StreamSource{
		Name: "Test Source",
		Type: models.SourceTypeM3U,
		URL:  "http://example.com/playlist.m3u",
	}
	_ = svc.Create(context.Background(), source)

	if svc.IsIngesting(source.ID) {
		t.Error("expected not ingesting initially")
	}

	_ = stateManager.Start(source)

	if !svc.IsIngesting(source.ID) {
		t.Error("expected ingesting after start")
	}
}

// mockEPGChecker is a mock implementation of EPGChecker
type mockEPGChecker struct {
	available bool
	err       error
}

func (m *mockEPGChecker) CheckEPGAvailability(ctx context.Context, baseURL, username, password string) (bool, error) {
	return m.available, m.err
}

func TestSourceService_CreateXtreamWithAutoEPG(t *testing.T) {
	sourceRepo := newMockStreamSourceRepo()
	channelRepo := newMockChannelRepo()
	epgSourceRepo := newMockEpgSourceRepo()
	factory := ingestor.NewHandlerFactory()
	stateManager := ingestor.NewStateManager()
	epgChecker := &mockEPGChecker{available: true}

	svc := NewSourceService(sourceRepo, channelRepo, factory, stateManager).
		WithEPGSourceRepo(epgSourceRepo).
		WithEPGChecker(epgChecker)

	source := &models.StreamSource{
		Name:     "Xtream Provider",
		Type:     models.SourceTypeXtream,
		URL:      "http://xtream.example.com",
		Username: "testuser",
		Password: "testpass",
	}

	err := svc.Create(context.Background(), source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check EPG source was auto-created
	epgSources, _ := epgSourceRepo.GetAll(context.Background())
	if len(epgSources) != 1 {
		t.Fatalf("expected 1 EPG source, got %d", len(epgSources))
	}

	epgSource := epgSources[0]
	if epgSource.Name != "Xtream Provider (EPG)" {
		t.Errorf("expected EPG name 'Xtream Provider (EPG)', got %q", epgSource.Name)
	}
	if epgSource.Type != models.EpgSourceTypeXtream {
		t.Errorf("expected EPG type 'xtream', got %q", epgSource.Type)
	}
	if epgSource.URL != "http://xtream.example.com" {
		t.Errorf("expected EPG URL 'http://xtream.example.com', got %q", epgSource.URL)
	}
	if epgSource.Username != "testuser" {
		t.Errorf("expected EPG username 'testuser', got %q", epgSource.Username)
	}
	if epgSource.Password != "testpass" {
		t.Errorf("expected EPG password 'testpass', got %q", epgSource.Password)
	}
}

func TestSourceService_CreateXtreamNoAutoEPG_WhenUnavailable(t *testing.T) {
	sourceRepo := newMockStreamSourceRepo()
	channelRepo := newMockChannelRepo()
	epgSourceRepo := newMockEpgSourceRepo()
	factory := ingestor.NewHandlerFactory()
	stateManager := ingestor.NewStateManager()
	epgChecker := &mockEPGChecker{available: false}

	svc := NewSourceService(sourceRepo, channelRepo, factory, stateManager).
		WithEPGSourceRepo(epgSourceRepo).
		WithEPGChecker(epgChecker)

	source := &models.StreamSource{
		Name:     "Xtream Provider",
		Type:     models.SourceTypeXtream,
		URL:      "http://xtream.example.com",
		Username: "testuser",
		Password: "testpass",
	}

	err := svc.Create(context.Background(), source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check no EPG source was created
	epgSources, _ := epgSourceRepo.GetAll(context.Background())
	if len(epgSources) != 0 {
		t.Errorf("expected 0 EPG sources when unavailable, got %d", len(epgSources))
	}
}

func TestSourceService_CreateXtreamNoAutoEPG_WhenAlreadyExists(t *testing.T) {
	sourceRepo := newMockStreamSourceRepo()
	channelRepo := newMockChannelRepo()
	epgSourceRepo := newMockEpgSourceRepo()
	factory := ingestor.NewHandlerFactory()
	stateManager := ingestor.NewStateManager()
	epgChecker := &mockEPGChecker{available: true}

	svc := NewSourceService(sourceRepo, channelRepo, factory, stateManager).
		WithEPGSourceRepo(epgSourceRepo).
		WithEPGChecker(epgChecker)

	// Pre-create an EPG source with the same URL
	existingEPG := &models.EpgSource{
		Name:     "Existing EPG",
		Type:     models.EpgSourceTypeXtream,
		URL:      "http://xtream.example.com",
		Username: "testuser",
		Password: "testpass",
	}
	_ = epgSourceRepo.Create(context.Background(), existingEPG)

	source := &models.StreamSource{
		Name:     "Xtream Provider",
		Type:     models.SourceTypeXtream,
		URL:      "http://xtream.example.com",
		Username: "testuser",
		Password: "testpass",
	}

	err := svc.Create(context.Background(), source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check no additional EPG source was created
	epgSources, _ := epgSourceRepo.GetAll(context.Background())
	if len(epgSources) != 1 {
		t.Errorf("expected 1 EPG source (existing), got %d", len(epgSources))
	}
}

func TestSourceService_CreateM3U_NoAutoEPG(t *testing.T) {
	sourceRepo := newMockStreamSourceRepo()
	channelRepo := newMockChannelRepo()
	epgSourceRepo := newMockEpgSourceRepo()
	factory := ingestor.NewHandlerFactory()
	stateManager := ingestor.NewStateManager()
	epgChecker := &mockEPGChecker{available: true}

	svc := NewSourceService(sourceRepo, channelRepo, factory, stateManager).
		WithEPGSourceRepo(epgSourceRepo).
		WithEPGChecker(epgChecker)

	// M3U sources should not trigger auto-EPG
	source := &models.StreamSource{
		Name: "M3U Provider",
		Type: models.SourceTypeM3U,
		URL:  "http://example.com/playlist.m3u",
	}

	err := svc.Create(context.Background(), source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check no EPG source was created
	epgSources, _ := epgSourceRepo.GetAll(context.Background())
	if len(epgSources) != 0 {
		t.Errorf("expected 0 EPG sources for M3U, got %d", len(epgSources))
	}
}

func TestSourceService_CreateXtreamAutoEPG_CheckerError(t *testing.T) {
	sourceRepo := newMockStreamSourceRepo()
	channelRepo := newMockChannelRepo()
	epgSourceRepo := newMockEpgSourceRepo()
	factory := ingestor.NewHandlerFactory()
	stateManager := ingestor.NewStateManager()
	epgChecker := &mockEPGChecker{available: false, err: errors.New("connection failed")}

	svc := NewSourceService(sourceRepo, channelRepo, factory, stateManager).
		WithEPGSourceRepo(epgSourceRepo).
		WithEPGChecker(epgChecker)

	source := &models.StreamSource{
		Name:     "Xtream Provider",
		Type:     models.SourceTypeXtream,
		URL:      "http://xtream.example.com",
		Username: "testuser",
		Password: "testpass",
	}

	// Should still succeed - auto-EPG check failure shouldn't fail creation
	err := svc.Create(context.Background(), source)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Stream source should be created
	if source.ID.IsZero() {
		t.Error("expected ID to be set")
	}

	// But no EPG source should be created
	epgSources, _ := epgSourceRepo.GetAll(context.Background())
	if len(epgSources) != 0 {
		t.Errorf("expected 0 EPG sources on checker error, got %d", len(epgSources))
	}
}
