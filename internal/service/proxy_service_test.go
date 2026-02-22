package service

import (
	"context"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProxyRepo implements repository.StreamProxyRepository for testing.
type mockProxyRepo struct {
	proxies      map[models.ULID]*models.StreamProxy
	sources      map[models.ULID][]*models.StreamSource
	epgSources   map[models.ULID][]*models.EpgSource
	createErr    error
	updateErr    error
	deleteErr    error
	getErr       error
	setSourceErr error
}

func newMockProxyRepo() *mockProxyRepo {
	return &mockProxyRepo{
		proxies:    make(map[models.ULID]*models.StreamProxy),
		sources:    make(map[models.ULID][]*models.StreamSource),
		epgSources: make(map[models.ULID][]*models.EpgSource),
	}
}

func (m *mockProxyRepo) Create(ctx context.Context, proxy *models.StreamProxy) error {
	if m.createErr != nil {
		return m.createErr
	}
	proxy.ID = models.NewULID()
	m.proxies[proxy.ID] = proxy
	return nil
}

func (m *mockProxyRepo) GetByID(ctx context.Context, id models.ULID) (*models.StreamProxy, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.proxies[id], nil
}

func (m *mockProxyRepo) GetByIDWithRelations(ctx context.Context, id models.ULID) (*models.StreamProxy, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	proxy := m.proxies[id]
	if proxy != nil {
		proxy.Sources = make([]models.ProxySource, 0)
		for _, s := range m.sources[id] {
			proxy.Sources = append(proxy.Sources, models.ProxySource{
				ProxyID:  id,
				SourceID: s.ID,
				Source:   s,
			})
		}
		proxy.EpgSources = make([]models.ProxyEpgSource, 0)
		for _, s := range m.epgSources[id] {
			proxy.EpgSources = append(proxy.EpgSources, models.ProxyEpgSource{
				ProxyID:     id,
				EpgSourceID: s.ID,
				EpgSource:   s,
			})
		}
	}
	return proxy, nil
}

func (m *mockProxyRepo) GetAll(ctx context.Context) ([]*models.StreamProxy, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	result := make([]*models.StreamProxy, 0, len(m.proxies))
	for _, p := range m.proxies {
		result = append(result, p)
	}
	return result, nil
}

func (m *mockProxyRepo) GetActive(ctx context.Context) ([]*models.StreamProxy, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	result := make([]*models.StreamProxy, 0)
	for _, p := range m.proxies {
		if models.BoolVal(p.IsActive) {
			result = append(result, p)
		}
	}
	return result, nil
}

func (m *mockProxyRepo) Update(ctx context.Context, proxy *models.StreamProxy) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.proxies[proxy.ID] = proxy
	return nil
}

func (m *mockProxyRepo) Delete(ctx context.Context, id models.ULID) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.proxies, id)
	return nil
}

func (m *mockProxyRepo) GetByName(ctx context.Context, name string) (*models.StreamProxy, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	for _, p := range m.proxies {
		if p.Name == name {
			return p, nil
		}
	}
	return nil, nil
}

func (m *mockProxyRepo) UpdateStatus(ctx context.Context, id models.ULID, status models.StreamProxyStatus, lastError string) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	if p, ok := m.proxies[id]; ok {
		p.Status = status
		p.LastError = lastError
	}
	return nil
}

func (m *mockProxyRepo) UpdateLastGeneration(ctx context.Context, id models.ULID, channelCount, programCount int) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	if p, ok := m.proxies[id]; ok {
		p.ChannelCount = channelCount
		p.ProgramCount = programCount
		p.Status = models.StreamProxyStatusSuccess
	}
	return nil
}

func (m *mockProxyRepo) SetSources(ctx context.Context, proxyID models.ULID, sourceIDs []models.ULID, priorities map[models.ULID]int) error {
	if m.setSourceErr != nil {
		return m.setSourceErr
	}
	// Simplified - just store empty sources for the IDs
	m.sources[proxyID] = make([]*models.StreamSource, len(sourceIDs))
	for i, id := range sourceIDs {
		m.sources[proxyID][i] = &models.StreamSource{BaseModel: models.BaseModel{ID: id}, Enabled: new(true)}
	}
	return nil
}

func (m *mockProxyRepo) SetEpgSources(ctx context.Context, proxyID models.ULID, sourceIDs []models.ULID, priorities map[models.ULID]int) error {
	if m.setSourceErr != nil {
		return m.setSourceErr
	}
	m.epgSources[proxyID] = make([]*models.EpgSource, len(sourceIDs))
	for i, id := range sourceIDs {
		m.epgSources[proxyID][i] = &models.EpgSource{BaseModel: models.BaseModel{ID: id}, Enabled: new(true)}
	}
	return nil
}

func (m *mockProxyRepo) GetSources(ctx context.Context, proxyID models.ULID) ([]*models.StreamSource, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.sources[proxyID], nil
}

func (m *mockProxyRepo) GetEpgSources(ctx context.Context, proxyID models.ULID) ([]*models.EpgSource, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.epgSources[proxyID], nil
}

func (m *mockProxyRepo) GetFilters(ctx context.Context, proxyID models.ULID) ([]*models.Filter, error) {
	return nil, nil
}

func (m *mockProxyRepo) SetFilters(ctx context.Context, proxyID models.ULID, filterIDs []models.ULID, orders map[models.ULID]int, isActive map[models.ULID]bool) error {
	return nil
}

func (m *mockProxyRepo) GetBySourceID(ctx context.Context, sourceID models.ULID) ([]*models.StreamProxy, error) {
	return nil, nil
}

func (m *mockProxyRepo) GetByEpgSourceID(ctx context.Context, epgSourceID models.ULID) ([]*models.StreamProxy, error) {
	return nil, nil
}

func (m *mockProxyRepo) CountByEncodingProfileID(ctx context.Context, profileID models.ULID) (int64, error) {
	return 0, nil
}

func (m *mockProxyRepo) GetByEncodingProfileID(ctx context.Context, profileID models.ULID) ([]*models.StreamProxy, error) {
	return nil, nil
}

func (m *mockProxyRepo) CountByEpgSourceID(ctx context.Context, epgSourceID models.ULID) (int64, error) {
	return 0, nil
}

func (m *mockProxyRepo) CountByFilterID(ctx context.Context, filterID models.ULID) (int64, error) {
	return 0, nil
}

func (m *mockProxyRepo) CountByStreamSourceID(ctx context.Context, sourceID models.ULID) (int64, error) {
	return 0, nil
}

func (m *mockProxyRepo) GetProxyNamesByEncodingProfileID(ctx context.Context, profileID models.ULID) ([]string, error) {
	return nil, nil
}

func (m *mockProxyRepo) GetProxyNamesByEpgSourceID(ctx context.Context, epgSourceID models.ULID) ([]string, error) {
	return nil, nil
}

func (m *mockProxyRepo) GetProxyNamesByFilterID(ctx context.Context, filterID models.ULID) ([]string, error) {
	return nil, nil
}

func (m *mockProxyRepo) GetProxyNamesByStreamSourceID(ctx context.Context, sourceID models.ULID) ([]string, error) {
	return nil, nil
}

func TestProxyService_Create(t *testing.T) {
	repo := newMockProxyRepo()
	svc := NewProxyService(repo, nil)
	ctx := context.Background()

	proxy := &models.StreamProxy{
		Name:                  "Test Proxy",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
	}

	err := svc.Create(ctx, proxy)
	require.NoError(t, err)
	assert.False(t, proxy.ID.IsZero())

	// Verify it was stored
	stored, err := svc.GetByID(ctx, proxy.ID)
	require.NoError(t, err)
	assert.Equal(t, "Test Proxy", stored.Name)
}

func TestProxyService_Create_ValidationError(t *testing.T) {
	repo := newMockProxyRepo()
	svc := NewProxyService(repo, nil)
	ctx := context.Background()

	proxy := &models.StreamProxy{
		Name:                  "", // Invalid - empty name
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
	}

	err := svc.Create(ctx, proxy)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestProxyService_Update(t *testing.T) {
	repo := newMockProxyRepo()
	svc := NewProxyService(repo, nil)
	ctx := context.Background()

	// Create first
	proxy := &models.StreamProxy{
		Name:                  "Original",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
	}
	require.NoError(t, svc.Create(ctx, proxy))

	// Update
	proxy.Name = "Updated"
	proxy.ProxyMode = models.StreamProxyModeSmart

	err := svc.Update(ctx, proxy)
	require.NoError(t, err)

	// Verify
	stored, _ := svc.GetByID(ctx, proxy.ID)
	assert.Equal(t, "Updated", stored.Name)
	assert.Equal(t, models.StreamProxyModeSmart, stored.ProxyMode)
}

func TestProxyService_Delete(t *testing.T) {
	repo := newMockProxyRepo()
	svc := NewProxyService(repo, nil)
	ctx := context.Background()

	proxy := &models.StreamProxy{
		Name:                  "To Delete",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
	}
	require.NoError(t, svc.Create(ctx, proxy))

	err := svc.Delete(ctx, proxy.ID)
	require.NoError(t, err)

	// Verify it's gone
	stored, _ := svc.GetByID(ctx, proxy.ID)
	assert.Nil(t, stored)
}

func TestProxyService_GetAll(t *testing.T) {
	repo := newMockProxyRepo()
	svc := NewProxyService(repo, nil)
	ctx := context.Background()

	// Create multiple proxies
	for i := 1; i <= 3; i++ {
		proxy := &models.StreamProxy{
			Name:                  "Proxy",
			ProxyMode:             models.StreamProxyModeDirect,
			StartingChannelNumber: i,
		}
		require.NoError(t, svc.Create(ctx, proxy))
	}

	proxies, err := svc.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, proxies, 3)
}

func TestProxyService_GetActive(t *testing.T) {
	repo := newMockProxyRepo()
	svc := NewProxyService(repo, nil)
	ctx := context.Background()

	// Create active proxy
	active := &models.StreamProxy{
		Name:                  "Active",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
		IsActive:              new(true),
	}
	require.NoError(t, svc.Create(ctx, active))

	// Create inactive proxy
	inactive := &models.StreamProxy{
		Name:                  "Inactive",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
		IsActive:              new(false),
	}
	require.NoError(t, svc.Create(ctx, inactive))

	proxies, err := svc.GetActive(ctx)
	require.NoError(t, err)
	assert.Len(t, proxies, 1)
	assert.Equal(t, "Active", proxies[0].Name)
}

func TestProxyService_GetByName(t *testing.T) {
	repo := newMockProxyRepo()
	svc := NewProxyService(repo, nil)
	ctx := context.Background()

	proxy := &models.StreamProxy{
		Name:                  "FindMe",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
	}
	require.NoError(t, svc.Create(ctx, proxy))

	found, err := svc.GetByName(ctx, "FindMe")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, proxy.ID, found.ID)

	notFound, err := svc.GetByName(ctx, "NotFound")
	require.NoError(t, err)
	assert.Nil(t, notFound)
}

func TestProxyService_SetSources(t *testing.T) {
	repo := newMockProxyRepo()
	svc := NewProxyService(repo, nil)
	ctx := context.Background()

	proxy := &models.StreamProxy{
		Name:                  "WithSources",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
	}
	require.NoError(t, svc.Create(ctx, proxy))

	sourceIDs := []models.ULID{models.NewULID(), models.NewULID(), models.NewULID()}
	err := svc.SetSources(ctx, proxy.ID, sourceIDs, nil)
	require.NoError(t, err)

	// Verify sources were set
	sources := repo.sources[proxy.ID]
	assert.Len(t, sources, 3)
}

func TestProxyService_SetEpgSources(t *testing.T) {
	repo := newMockProxyRepo()
	svc := NewProxyService(repo, nil)
	ctx := context.Background()

	proxy := &models.StreamProxy{
		Name:                  "WithEpgSources",
		ProxyMode:             models.StreamProxyModeDirect,
		StartingChannelNumber: 1,
	}
	require.NoError(t, svc.Create(ctx, proxy))

	epgSourceIDs := []models.ULID{models.NewULID(), models.NewULID()}
	err := svc.SetEpgSources(ctx, proxy.ID, epgSourceIDs, nil)
	require.NoError(t, err)

	// Verify sources were set
	sources := repo.epgSources[proxy.ID]
	assert.Len(t, sources, 2)
}
