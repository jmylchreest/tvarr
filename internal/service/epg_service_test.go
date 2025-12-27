package service

import (
	"context"
	"errors"
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
	programs       map[models.ULID]*models.EpgProgram
	createErr      error
	createBatchErr error
	deleteErr      error
	countBySource  map[models.ULID]int64
}

func newMockEpgProgramRepo() *mockEpgProgramRepo {
	return &mockEpgProgramRepo{
		programs:      make(map[models.ULID]*models.EpgProgram),
		countBySource: make(map[models.ULID]int64),
	}
}

func (m *mockEpgProgramRepo) Create(ctx context.Context, program *models.EpgProgram) error {
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
	program, ok := m.programs[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return program, nil
}

func (m *mockEpgProgramRepo) GetBySourceID(ctx context.Context, sourceID models.ULID, callback func(*models.EpgProgram) error) error {
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
	var programs []*models.EpgProgram
	for _, p := range m.programs {
		if p.ChannelID == channelID && p.Start.Before(end) && p.Stop.After(start) {
			programs = append(programs, p)
		}
	}
	return programs, nil
}

func (m *mockEpgProgramRepo) GetByChannelIDWithLimit(ctx context.Context, channelID string, limit int) ([]*models.EpgProgram, error) {
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
	now := time.Now()
	for _, p := range m.programs {
		if p.ChannelID == channelID && p.Start.Before(now) && p.Stop.After(now) {
			return p, nil
		}
	}
	return nil, nil
}

func (m *mockEpgProgramRepo) Delete(ctx context.Context, id models.ULID) error {
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

func (m *mockEpgProgramRepo) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	var count int64
	for id, p := range m.programs {
		if p.Stop.Before(before) {
			delete(m.programs, id)
			count++
		}
	}
	return count, nil
}

func (m *mockEpgProgramRepo) DeleteOld(ctx context.Context) (int64, error) {
	return m.DeleteExpired(ctx, time.Now().Add(-24*time.Hour))
}

func (m *mockEpgProgramRepo) CountBySourceID(ctx context.Context, sourceID models.ULID) (int64, error) {
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
		Enabled: models.BoolPtr(true),
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
		Enabled: models.BoolPtr(true),
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
		Enabled: models.BoolPtr(true),
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
		Enabled: models.BoolPtr(true),
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
	for i := 0; i < 3; i++ {
		source := &models.EpgSource{
			Name:    "Test EPG",
			Type:    models.EpgSourceTypeXMLTV,
			URL:     "http://example.com/epg.xml",
			Enabled: models.BoolPtr(true),
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
		Enabled: models.BoolPtr(true),
	}
	source2 := &models.EpgSource{
		Name:    "Disabled EPG",
		Type:    models.EpgSourceTypeXMLTV,
		URL:     "http://example.com/epg2.xml",
		Enabled: models.BoolPtr(false),
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
		Enabled: models.BoolPtr(true),
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
		Enabled: models.BoolPtr(true),
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
		Enabled: models.BoolPtr(true),
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
		Enabled: models.BoolPtr(true),
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
