package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jmylchreest/tvarr/internal/ingestor"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock EPG Source Repository
type mockEpgSourceRepo struct {
	sources     map[models.ULID]*models.EpgSource
	createErr   error
	getErr      error
	updateErr   error
	deleteErr   error
	getByNameFn func(name string) (*models.EpgSource, error)
}

func newMockEpgSourceRepo() *mockEpgSourceRepo {
	return &mockEpgSourceRepo{
		sources: make(map[models.ULID]*models.EpgSource),
	}
}

func (m *mockEpgSourceRepo) Create(ctx context.Context, source *models.EpgSource) error {
	if m.createErr != nil {
		return m.createErr
	}
	if source.ID.IsZero() {
		source.ID = models.NewULID()
	}
	m.sources[source.ID] = source
	return nil
}

func (m *mockEpgSourceRepo) GetByID(ctx context.Context, id models.ULID) (*models.EpgSource, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	source, ok := m.sources[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return source, nil
}

func (m *mockEpgSourceRepo) GetAll(ctx context.Context) ([]*models.EpgSource, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	var sources []*models.EpgSource
	for _, s := range m.sources {
		sources = append(sources, s)
	}
	return sources, nil
}

func (m *mockEpgSourceRepo) GetEnabled(ctx context.Context) ([]*models.EpgSource, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	var sources []*models.EpgSource
	for _, s := range m.sources {
		if models.BoolVal(s.Enabled) {
			sources = append(sources, s)
		}
	}
	return sources, nil
}

func (m *mockEpgSourceRepo) Update(ctx context.Context, source *models.EpgSource) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.sources[source.ID] = source
	return nil
}

func (m *mockEpgSourceRepo) Delete(ctx context.Context, id models.ULID) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.sources, id)
	return nil
}

func (m *mockEpgSourceRepo) GetByName(ctx context.Context, name string) (*models.EpgSource, error) {
	if m.getByNameFn != nil {
		return m.getByNameFn(name)
	}
	for _, s := range m.sources {
		if s.Name == name {
			return s, nil
		}
	}
	return nil, errors.New("not found")
}

func (m *mockEpgSourceRepo) UpdateLastIngestion(ctx context.Context, id models.ULID, status string, programCount int) error {
	return nil
}

func (m *mockEpgSourceRepo) GetByURL(ctx context.Context, url string) (*models.EpgSource, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	for _, s := range m.sources {
		if s.URL == url {
			return s, nil
		}
	}
	return nil, nil
}

// Mock EPG Program Repository
type mockEpgProgramRepo struct {
	// mu guards programs: source deletion now sweeps them on a background
	// goroutine, so tests and the sweep touch this map concurrently.
	mu             sync.Mutex
	programs       map[models.ULID]*models.EpgProgram
	createErr      error
	createBatchErr error
	deleteErr      error
	countBySource  map[models.ULID]int64
	// knownSources marks which sources still exist, so DeleteOrphaned can tell
	// an orphan from a live program.
	knownSources map[models.ULID]bool
}

func newMockEpgProgramRepo() *mockEpgProgramRepo {
	return &mockEpgProgramRepo{
		programs:      make(map[models.ULID]*models.EpgProgram),
		knownSources:  make(map[models.ULID]bool),
		countBySource: make(map[models.ULID]int64),
	}
}

func (m *mockEpgProgramRepo) Create(ctx context.Context, program *models.EpgProgram) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.createErr != nil {
		return m.createErr
	}
	if program.ID.IsZero() {
		program.ID = models.NewULID()
	}
	m.programs[program.ID] = program
	m.countBySource[program.SourceID]++
	return nil
}

func (m *mockEpgProgramRepo) CreateBatch(ctx context.Context, programs []*models.EpgProgram) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.createBatchErr != nil {
		return m.createBatchErr
	}
	for _, p := range programs {
		if p.ID.IsZero() {
			p.ID = models.NewULID()
		}
		m.programs[p.ID] = p
		m.countBySource[p.SourceID]++
	}
	return nil
}

func (m *mockEpgProgramRepo) GetByID(ctx context.Context, id models.ULID) (*models.EpgProgram, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	program, ok := m.programs[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return program, nil
}

func (m *mockEpgProgramRepo) GetBySourceID(ctx context.Context, sourceID models.ULID, callback func(*models.EpgProgram) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, p := range m.programs {
		if p.SourceID == sourceID {
			if err := callback(p); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *mockEpgProgramRepo) GetByChannelID(ctx context.Context, channelID string, start, end time.Time) ([]*models.EpgProgram, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var programs []*models.EpgProgram
	for _, p := range m.programs {
		if p.ChannelID == channelID && p.Start.Before(end) && p.Stop.After(start) {
			programs = append(programs, p)
		}
	}
	return programs, nil
}

func (m *mockEpgProgramRepo) GetByChannelIDWithLimit(ctx context.Context, channelID string, limit int) ([]*models.EpgProgram, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var programs []*models.EpgProgram
	for _, p := range m.programs {
		if p.ChannelID == channelID {
			programs = append(programs, p)
			if len(programs) >= limit {
				break
			}
		}
	}
	return programs, nil
}

func (m *mockEpgProgramRepo) GetCurrentByChannelID(ctx context.Context, channelID string) (*models.EpgProgram, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for _, p := range m.programs {
		if p.ChannelID == channelID && p.Start.Before(now) && p.Stop.After(now) {
			return p, nil
		}
	}
	return nil, nil
}

func (m *mockEpgProgramRepo) Delete(ctx context.Context, id models.ULID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.deleteErr != nil {
		return m.deleteErr
	}
	if p, ok := m.programs[id]; ok {
		m.countBySource[p.SourceID]--
	}
	delete(m.programs, id)
	return nil
}

func (m *mockEpgProgramRepo) DeleteBySourceID(ctx context.Context, sourceID models.ULID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.deleteErr != nil {
		return m.deleteErr
	}
	for id, p := range m.programs {
		if p.SourceID == sourceID {
			delete(m.programs, id)
		}
	}
	m.countBySource[sourceID] = 0
	return nil
}

func (m *mockEpgProgramRepo) DeleteStaleBySourceID(ctx context.Context, sourceID models.ULID, olderThan time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var count int64
	for id, p := range m.programs {
		if p.SourceID == sourceID && p.UpdatedAt.Before(olderThan) {
			delete(m.programs, id)
			count++
		}
	}
	m.countBySource[sourceID] -= count
	return count, nil
}

func (m *mockEpgProgramRepo) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var count int64
	for id, p := range m.programs {
		if p.Stop.Before(before) {
			delete(m.programs, id)
			count++
		}
	}
	return count, nil
}

// DeleteOld delegates to DeleteExpired, which takes the lock itself -- taking it
// here as well would deadlock on the non-reentrant mutex.
func (m *mockEpgProgramRepo) DeleteOld(ctx context.Context) (int64, error) {
	return m.DeleteExpired(ctx, time.Now().Add(-24*time.Hour))
}

// DeleteOrphaned removes programs whose source is no longer known to the mock.
func (m *mockEpgProgramRepo) DeleteOrphaned(ctx context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var deleted int64
	for id, p := range m.programs {
		if !m.knownSources[p.SourceID] {
			delete(m.programs, id)
			deleted++
		}
	}
	return deleted, nil
}

// countPrograms reports how many programs remain, for assertions against the
// background sweep.
func (m *mockEpgProgramRepo) countPrograms() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.programs)
}

func (m *mockEpgProgramRepo) CountBySourceID(ctx context.Context, sourceID models.ULID) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.countBySource[sourceID], nil
}

// Tests

func TestEpgService_Create(t *testing.T) {
	sourceRepo := newMockEpgSourceRepo()
	programRepo := newMockEpgProgramRepo()
	factory := ingestor.NewEpgHandlerFactory()
	stateManager := ingestor.NewStateManager()

	service := NewEpgService(sourceRepo, programRepo, factory, stateManager)

	source := &models.EpgSource{
		Name:    "Test EPG",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/epg.xml",
		Enabled: new(true),
	}

	err := service.Create(context.Background(), source)
	require.NoError(t, err)
	assert.False(t, source.ID.IsZero())
}

func TestEpgService_Create_ValidationError(t *testing.T) {
	sourceRepo := newMockEpgSourceRepo()
	programRepo := newMockEpgProgramRepo()
	factory := ingestor.NewEpgHandlerFactory()
	stateManager := ingestor.NewStateManager()

	service := NewEpgService(sourceRepo, programRepo, factory, stateManager)

	source := &models.EpgSource{
		Name: "", // Invalid - empty name
		Type: models.EpgSourceTypeXMLTV,
		URL:  "http://example.com/epg.xml",
	}

	err := service.Create(context.Background(), source)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestEpgService_GetByID(t *testing.T) {
	sourceRepo := newMockEpgSourceRepo()
	programRepo := newMockEpgProgramRepo()
	factory := ingestor.NewEpgHandlerFactory()
	stateManager := ingestor.NewStateManager()

	service := NewEpgService(sourceRepo, programRepo, factory, stateManager)

	// Create a source first
	source := &models.EpgSource{
		Name:    "Test EPG",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/epg.xml",
		Enabled: new(true),
	}
	_ = service.Create(context.Background(), source)

	// Get by ID
	retrieved, err := service.GetByID(context.Background(), source.ID)
	require.NoError(t, err)
	assert.Equal(t, source.Name, retrieved.Name)
}

func TestEpgService_Update(t *testing.T) {
	sourceRepo := newMockEpgSourceRepo()
	programRepo := newMockEpgProgramRepo()
	factory := ingestor.NewEpgHandlerFactory()
	stateManager := ingestor.NewStateManager()

	service := NewEpgService(sourceRepo, programRepo, factory, stateManager)

	source := &models.EpgSource{
		Name:    "Test EPG",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/epg.xml",
		Enabled: new(true),
	}
	_ = service.Create(context.Background(), source)

	// Update the source
	source.Name = "Updated EPG"
	err := service.Update(context.Background(), source)
	require.NoError(t, err)

	// Verify update
	retrieved, _ := service.GetByID(context.Background(), source.ID)
	assert.Equal(t, "Updated EPG", retrieved.Name)
}

func TestEpgService_Delete(t *testing.T) {
	sourceRepo := newMockEpgSourceRepo()
	programRepo := newMockEpgProgramRepo()
	factory := ingestor.NewEpgHandlerFactory()
	stateManager := ingestor.NewStateManager()

	service := NewEpgService(sourceRepo, programRepo, factory, stateManager)

	source := &models.EpgSource{
		Name:    "Test EPG",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/epg.xml",
		Enabled: new(true),
	}
	_ = service.Create(context.Background(), source)

	// Delete the source
	err := service.Delete(context.Background(), source.ID)
	require.NoError(t, err)

	// Verify deletion
	_, err = service.GetByID(context.Background(), source.ID)
	require.Error(t, err)
}

func TestEpgService_List(t *testing.T) {
	sourceRepo := newMockEpgSourceRepo()
	programRepo := newMockEpgProgramRepo()
	factory := ingestor.NewEpgHandlerFactory()
	stateManager := ingestor.NewStateManager()

	service := NewEpgService(sourceRepo, programRepo, factory, stateManager)

	// Create multiple sources
	for range 3 {
		source := &models.EpgSource{
			Name:    "Test EPG",
			Type:    models.EpgSourceTypeXMLTV,
			URL:     "http://example.com/epg.xml",
			Enabled: new(true),
		}
		_ = service.Create(context.Background(), source)
	}

	sources, err := service.List(context.Background())
	require.NoError(t, err)
	assert.Len(t, sources, 3)
}

func TestEpgService_ListEnabled(t *testing.T) {
	sourceRepo := newMockEpgSourceRepo()
	programRepo := newMockEpgProgramRepo()
	factory := ingestor.NewEpgHandlerFactory()
	stateManager := ingestor.NewStateManager()

	service := NewEpgService(sourceRepo, programRepo, factory, stateManager)

	// Create sources with different enabled states
	source1 := &models.EpgSource{
		Name:    "Enabled EPG",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/epg1.xml",
		Enabled: new(true),
	}
	source2 := &models.EpgSource{
		Name:    "Disabled EPG",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/epg2.xml",
		Enabled: new(false),
	}
	_ = service.Create(context.Background(), source1)
	_ = service.Create(context.Background(), source2)

	sources, err := service.ListEnabled(context.Background())
	require.NoError(t, err)
	assert.Len(t, sources, 1)
	assert.Equal(t, "Enabled EPG", sources[0].Name)
}

func TestEpgService_GetProgramCount(t *testing.T) {
	sourceRepo := newMockEpgSourceRepo()
	programRepo := newMockEpgProgramRepo()
	factory := ingestor.NewEpgHandlerFactory()
	stateManager := ingestor.NewStateManager()

	service := NewEpgService(sourceRepo, programRepo, factory, stateManager)

	// Create a source and add some programs
	source := &models.EpgSource{
		Name:    "Test EPG",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/epg.xml",
		Enabled: new(true),
	}
	_ = service.Create(context.Background(), source)

	// Add programs to the mock
	programRepo.countBySource[source.ID] = 100

	count, err := service.GetProgramCount(context.Background(), source.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(100), count)
}

func TestEpgService_IsIngesting(t *testing.T) {
	sourceRepo := newMockEpgSourceRepo()
	programRepo := newMockEpgProgramRepo()
	factory := ingestor.NewEpgHandlerFactory()
	stateManager := ingestor.NewStateManager()

	service := NewEpgService(sourceRepo, programRepo, factory, stateManager)

	// Create a source
	source := &models.EpgSource{
		Name:    "Test EPG",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/epg.xml",
		Enabled: new(true),
	}
	_ = service.Create(context.Background(), source)

	// Initially not ingesting
	assert.False(t, service.IsIngesting(source.ID))
}

func TestEpgService_GetIngestionState(t *testing.T) {
	sourceRepo := newMockEpgSourceRepo()
	programRepo := newMockEpgProgramRepo()
	factory := ingestor.NewEpgHandlerFactory()
	stateManager := ingestor.NewStateManager()

	service := NewEpgService(sourceRepo, programRepo, factory, stateManager)

	// Create a source
	source := &models.EpgSource{
		Name:    "Test EPG",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/epg.xml",
		Enabled: new(true),
	}
	_ = service.Create(context.Background(), source)

	// No state for non-ingesting source
	state, ok := service.GetIngestionState(source.ID)
	assert.False(t, ok)
	assert.Nil(t, state)
}

func TestEpgService_DeleteOldPrograms(t *testing.T) {
	sourceRepo := newMockEpgSourceRepo()
	programRepo := newMockEpgProgramRepo()
	factory := ingestor.NewEpgHandlerFactory()
	stateManager := ingestor.NewStateManager()

	service := NewEpgService(sourceRepo, programRepo, factory, stateManager)

	// Create a source
	source := &models.EpgSource{
		Name:    "Test EPG",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/epg.xml",
		Enabled: new(true),
	}
	_ = service.Create(context.Background(), source)

	// Add some old programs
	oldTime := time.Now().Add(-48 * time.Hour)
	programID := models.NewULID()
	programRepo.programs[programID] = &models.EpgProgram{
		BaseModel: models.BaseModel{ID: programID},
		SourceID:  source.ID,
		ChannelID: "ch1",
		Start:     oldTime,
		Stop:      oldTime.Add(time.Hour),
		Title:     "Old Show",
	}

	count, err := service.DeleteOldPrograms(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
}

func TestEpgService_CreateXtreamWithAutoStreamSource(t *testing.T) {
	sourceRepo := newMockEpgSourceRepo()
	programRepo := newMockEpgProgramRepo()
	streamSourceRepo := newMockStreamSourceRepo()
	factory := ingestor.NewEpgHandlerFactory()
	stateManager := ingestor.NewStateManager()

	svc := NewEpgService(sourceRepo, programRepo, factory, stateManager).
		WithStreamSourceRepo(streamSourceRepo)

	epgSource := &models.EpgSource{
		Name:     "Xtream EPG",
		Type:     models.EpgSourceTypeXtream,
		URL:      "http://xtream.example.com",
		Username: "testuser",
		Password: "testpass",
	}

	err := svc.Create(context.Background(), epgSource)
	require.NoError(t, err)

	// Check stream source was auto-created
	streamSources, _ := streamSourceRepo.GetAll(context.Background())
	assert.Len(t, streamSources, 1)

	ss := streamSources[0]
	assert.Equal(t, "Xtream EPG (Streams)", ss.Name)
	assert.Equal(t, models.SourceTypeXtream, ss.Type)
	assert.Equal(t, "http://xtream.example.com", ss.URL)
	assert.Equal(t, "testuser", ss.Username)
	assert.Equal(t, "testpass", ss.Password)
}

func TestEpgService_CreateXtreamNoAutoStreamSource_WhenAlreadyExists(t *testing.T) {
	sourceRepo := newMockEpgSourceRepo()
	programRepo := newMockEpgProgramRepo()
	streamSourceRepo := newMockStreamSourceRepo()
	factory := ingestor.NewEpgHandlerFactory()
	stateManager := ingestor.NewStateManager()

	svc := NewEpgService(sourceRepo, programRepo, factory, stateManager).
		WithStreamSourceRepo(streamSourceRepo)

	// Pre-create a stream source with the same URL
	existingStream := &models.StreamSource{
		Name:     "Existing Stream",
		Type:     models.SourceTypeXtream,
		URL:      "http://xtream.example.com",
		Username: "testuser",
		Password: "testpass",
	}
	_ = streamSourceRepo.Create(context.Background(), existingStream)

	epgSource := &models.EpgSource{
		Name:     "Xtream EPG",
		Type:     models.EpgSourceTypeXtream,
		URL:      "http://xtream.example.com",
		Username: "testuser",
		Password: "testpass",
	}

	err := svc.Create(context.Background(), epgSource)
	require.NoError(t, err)

	// Should not create a duplicate stream source
	streamSources, _ := streamSourceRepo.GetAll(context.Background())
	assert.Len(t, streamSources, 1)
	assert.Equal(t, "Existing Stream", streamSources[0].Name)
}

func TestEpgService_CreateXMLTV_NoAutoStreamSource(t *testing.T) {
	sourceRepo := newMockEpgSourceRepo()
	programRepo := newMockEpgProgramRepo()
	streamSourceRepo := newMockStreamSourceRepo()
	factory := ingestor.NewEpgHandlerFactory()
	stateManager := ingestor.NewStateManager()

	svc := NewEpgService(sourceRepo, programRepo, factory, stateManager).
		WithStreamSourceRepo(streamSourceRepo)

	// XMLTV EPG sources should not trigger auto-stream-source creation
	epgSource := &models.EpgSource{
		Name: "XMLTV EPG",
		Type: models.EpgSourceTypeXMLTV,
		URL:  "http://example.com/epg.xml",
	}

	err := svc.Create(context.Background(), epgSource)
	require.NoError(t, err)

	// No stream source should be created
	streamSources, _ := streamSourceRepo.GetAll(context.Background())
	assert.Len(t, streamSources, 0)
}

// TestEpgService_DeleteReturnsBeforeSweepingPrograms covers the reason this path
// changed: deleting a source with millions of programs used to remove them inline
// and the request timed out before the source was gone, leaving the user unable
// to tell whether the delete had worked.
func TestEpgService_DeleteReturnsBeforeSweepingPrograms(t *testing.T) {
	sourceRepo := newMockEpgSourceRepo()
	programRepo := newMockEpgProgramRepo()
	service := NewEpgService(sourceRepo, programRepo,
		ingestor.NewEpgHandlerFactory(), ingestor.NewStateManager())

	source := &models.EpgSource{
		Name:    "Large EPG",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/epg.xml",
		Enabled: new(true),
	}
	require.NoError(t, service.Create(context.Background(), source))

	for range 500 {
		require.NoError(t, programRepo.Create(context.Background(), &models.EpgProgram{
			SourceID:  source.ID,
			ChannelID: "ch1",
			Title:     "Programme",
			Start:     time.Now(),
			Stop:      time.Now().Add(time.Hour),
		}))
	}

	require.NoError(t, service.Delete(context.Background(), source.ID))

	// The source must be gone the moment Delete returns: that is what the UI
	// reflects, and what makes the request fast.
	_, err := service.GetByID(context.Background(), source.ID)
	require.Error(t, err)

	// Programs are swept behind the response.
	require.Eventually(t, func() bool {
		return programRepo.countPrograms() == 0
	}, 5*time.Second, 10*time.Millisecond, "background sweep did not remove the programs")
}

// TestEpgService_DeleteSurvivesRequestCancellation guards the detached context:
// the request context is cancelled as soon as the response is written, and a
// sweep bound to it would abort immediately, orphaning every remaining row.
func TestEpgService_DeleteSurvivesRequestCancellation(t *testing.T) {
	sourceRepo := newMockEpgSourceRepo()
	programRepo := newMockEpgProgramRepo()
	service := NewEpgService(sourceRepo, programRepo,
		ingestor.NewEpgHandlerFactory(), ingestor.NewStateManager())

	source := &models.EpgSource{
		Name:    "Cancelled EPG",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/epg.xml",
		Enabled: new(true),
	}
	require.NoError(t, service.Create(context.Background(), source))
	require.NoError(t, programRepo.Create(context.Background(), &models.EpgProgram{
		SourceID:  source.ID,
		ChannelID: "ch1",
		Title:     "Programme",
		Start:     time.Now(),
		Stop:      time.Now().Add(time.Hour),
	}))

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, service.Delete(ctx, source.ID))
	cancel() // as the HTTP layer does once the response is written

	require.Eventually(t, func() bool {
		return programRepo.countPrograms() == 0
	}, 5*time.Second, 10*time.Millisecond, "sweep aborted when the request context was cancelled")
}

// TestEpgService_SweepOrphanedPrograms covers the recovery path for a sweep that
// did not finish, since the source row is removed first and a restart in between
// would otherwise strand its programs permanently.
func TestEpgService_SweepOrphanedPrograms(t *testing.T) {
	sourceRepo := newMockEpgSourceRepo()
	programRepo := newMockEpgProgramRepo()
	service := NewEpgService(sourceRepo, programRepo,
		ingestor.NewEpgHandlerFactory(), ingestor.NewStateManager())

	live := models.NewULID()
	orphaned := models.NewULID()
	programRepo.knownSources[live] = true

	for _, sourceID := range []models.ULID{live, live, orphaned, orphaned, orphaned} {
		require.NoError(t, programRepo.Create(context.Background(), &models.EpgProgram{
			SourceID:  sourceID,
			ChannelID: "ch1",
			Title:     "Programme",
			Start:     time.Now(),
			Stop:      time.Now().Add(time.Hour),
		}))
	}

	require.NoError(t, service.SweepOrphanedPrograms(context.Background()))

	if got := programRepo.countPrograms(); got != 2 {
		t.Errorf("after sweep %d programs remain, want the 2 belonging to the live source", got)
	}
}
