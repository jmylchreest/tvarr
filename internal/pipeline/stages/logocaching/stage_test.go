package logocaching

import (
	"context"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/jmylchreest/tvarr/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testChannel creates a minimal channel for testing with optional logo URL.
func testChannel(name string, logoURL string) *models.Channel {
	return &models.Channel{
		ChannelName: name,
		TvgLogo:     logoURL,
		StreamURL:   "http://example.com/" + name,
	}
}

// mockLogoCacher implements LogoCacher for testing.
type mockLogoCacher struct {
	cachedURLs  map[string]*storage.CachedLogoMetadata
	cacheErrors map[string]error
}

func newMockLogoCacher() *mockLogoCacher {
	return &mockLogoCacher{
		cachedURLs:  make(map[string]*storage.CachedLogoMetadata),
		cacheErrors: make(map[string]error),
	}
}

func (m *mockLogoCacher) CacheLogo(ctx context.Context, logoURL string) (*storage.CachedLogoMetadata, error) {
	if err, ok := m.cacheErrors[logoURL]; ok {
		return nil, err
	}
	// Create new metadata for the logo
	meta := storage.NewCachedLogoMetadata(logoURL)
	meta.ContentType = "image/png"
	m.cachedURLs[logoURL] = meta
	return meta, nil
}

func (m *mockLogoCacher) Contains(logoURL string) bool {
	_, ok := m.cachedURLs[logoURL]
	return ok
}

// withCachedLogo pre-populates a cached logo.
func (m *mockLogoCacher) withCachedLogo(url string) *mockLogoCacher {
	meta := storage.NewCachedLogoMetadata(url)
	meta.ContentType = "image/png"
	m.cachedURLs[url] = meta
	return m
}

// withCacheError makes CacheLogo return an error for a specific URL.
func (m *mockLogoCacher) withCacheError(url string, err error) *mockLogoCacher {
	m.cacheErrors[url] = err
	return m
}

// getCachedCount returns the number of logos cached during the test.
func (m *mockLogoCacher) getCachedCount() int {
	return len(m.cachedURLs)
}

func TestStage_ID(t *testing.T) {
	stage := New(nil)
	assert.Equal(t, "logo_caching", stage.ID())
}

func TestStage_Name(t *testing.T) {
	stage := New(nil)
	assert.Equal(t, "Logo Caching", stage.Name())
}

func TestStage_EmptyChannels(t *testing.T) {
	cacher := newMockLogoCacher()
	stage := New(cacher)

	proxy := &models.StreamProxy{}
	state := core.NewState(proxy)
	state.Channels = []*models.Channel{}

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	assert.Equal(t, "No channels to process for logo caching", result.Message)
	assert.Equal(t, 0, result.RecordsProcessed)
}

func TestStage_NoLogosToCache(t *testing.T) {
	cacher := newMockLogoCacher()
	stage := New(cacher)

	channels := []*models.Channel{
		testChannel("Channel 1", ""),
		testChannel("Channel 2", ""),
	}

	proxy := &models.StreamProxy{}
	state := core.NewState(proxy)
	state.Channels = channels

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	assert.Equal(t, 2, result.RecordsProcessed)
	assert.Equal(t, 0, result.RecordsModified)
	assert.Equal(t, 0, cacher.getCachedCount())
}

func TestStage_CacheNewLogos(t *testing.T) {
	cacher := newMockLogoCacher()
	stage := New(cacher)

	channels := []*models.Channel{
		testChannel("Channel 1", "http://example.com/logo1.png"),
		testChannel("Channel 2", "http://example.com/logo2.png"),
		testChannel("Channel 3", ""), // No logo
	}

	proxy := &models.StreamProxy{}
	state := core.NewState(proxy)
	state.Channels = channels

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	assert.Equal(t, 3, result.RecordsProcessed)
	assert.Equal(t, 2, result.RecordsModified) // 2 logos cached
	assert.Equal(t, 2, cacher.getCachedCount())
	assert.True(t, cacher.Contains("http://example.com/logo1.png"))
	assert.True(t, cacher.Contains("http://example.com/logo2.png"))
}

func TestStage_DeduplicateLogos(t *testing.T) {
	cacher := newMockLogoCacher()
	stage := New(cacher)

	// Multiple channels share the same logo
	channels := []*models.Channel{
		testChannel("Channel 1", "http://example.com/shared-logo.png"),
		testChannel("Channel 2", "http://example.com/shared-logo.png"),
		testChannel("Channel 3", "http://example.com/unique-logo.png"),
	}

	proxy := &models.StreamProxy{}
	state := core.NewState(proxy)
	state.Channels = channels

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	// Should only cache unique logos
	assert.Equal(t, 2, cacher.getCachedCount())
	assert.Equal(t, 3, result.RecordsProcessed)
}

func TestStage_SkipAlreadyCached(t *testing.T) {
	cacher := newMockLogoCacher().
		withCachedLogo("http://example.com/cached-logo.png")
	stage := New(cacher)

	channels := []*models.Channel{
		testChannel("Channel 1", "http://example.com/cached-logo.png"),
		testChannel("Channel 2", "http://example.com/new-logo.png"),
	}

	proxy := &models.StreamProxy{}
	state := core.NewState(proxy)
	state.Channels = channels

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	// Only 1 new logo should be cached (the one not already cached)
	assert.Equal(t, 2, cacher.getCachedCount()) // 1 pre-cached + 1 new
	assert.Equal(t, 2, result.RecordsProcessed)
	assert.Equal(t, 1, result.RecordsModified) // Only 1 newly cached
}

func TestStage_ContinuesOnCacheError(t *testing.T) {
	cacher := newMockLogoCacher().
		withCacheError("http://example.com/bad-logo.png", assert.AnError)
	stage := New(cacher)

	channels := []*models.Channel{
		testChannel("Channel 1", "http://example.com/bad-logo.png"),
		testChannel("Channel 2", "http://example.com/good-logo.png"),
	}

	proxy := &models.StreamProxy{}
	state := core.NewState(proxy)
	state.Channels = channels

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	// Should continue despite error
	assert.Equal(t, 1, cacher.getCachedCount()) // Only good logo cached
	assert.Equal(t, 2, result.RecordsProcessed)
	assert.Equal(t, 1, result.RecordsModified)
}

func TestStage_ArtifactMetadata(t *testing.T) {
	cacher := newMockLogoCacher()
	stage := New(cacher)

	channels := []*models.Channel{
		testChannel("Channel 1", "http://example.com/logo1.png"),
		testChannel("Channel 2", "http://example.com/logo2.png"),
	}

	proxy := &models.StreamProxy{}
	state := core.NewState(proxy)
	state.Channels = channels

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	require.Len(t, result.Artifacts, 1)
	artifact := result.Artifacts[0]

	assert.Equal(t, 2, artifact.Metadata["unique_logos"])
	assert.Equal(t, 2, artifact.Metadata["logos_newly_cached"])
}

func TestStage_StatsTracking(t *testing.T) {
	cacher := newMockLogoCacher().
		withCachedLogo("http://example.com/cached.png")
	stage := New(cacher)

	channels := []*models.Channel{
		testChannel("Channel 1", "http://example.com/cached.png"),
		testChannel("Channel 2", "http://example.com/cached.png"),
		testChannel("Channel 3", "http://example.com/new.png"),
		testChannel("Channel 4", ""),
	}

	proxy := &models.StreamProxy{}
	state := core.NewState(proxy)
	state.Channels = channels

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	stats := stage.GetStats()
	assert.Equal(t, 4, stats.ChannelsProcessed)
	assert.Equal(t, 3, stats.ChannelsWithLogos)
	assert.Equal(t, 2, stats.UniqueLogoURLs)
	assert.Equal(t, 1, stats.AlreadyCached)
	assert.Equal(t, 1, stats.NewlyCached)
	assert.Equal(t, 0, stats.Errors)

	assert.Equal(t, 4, result.RecordsProcessed)
}

func TestStage_StatsWithErrors(t *testing.T) {
	cacher := newMockLogoCacher().
		withCacheError("http://example.com/bad.png", assert.AnError)
	stage := New(cacher)

	channels := []*models.Channel{
		testChannel("Channel 1", "http://example.com/bad.png"),
		testChannel("Channel 2", "http://example.com/good.png"),
	}

	proxy := &models.StreamProxy{}
	state := core.NewState(proxy)
	state.Channels = channels

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	stats := stage.GetStats()
	assert.Equal(t, 1, stats.Errors)
	assert.Equal(t, 1, stats.NewlyCached)
}

func TestStage_NilCacher(t *testing.T) {
	// Stage should work even without cacher (disabled mode)
	stage := New(nil)

	channels := []*models.Channel{
		testChannel("Channel 1", "http://example.com/logo.png"),
	}

	proxy := &models.StreamProxy{}
	state := core.NewState(proxy)
	state.Channels = channels

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	assert.Equal(t, 1, result.RecordsProcessed)
	assert.Equal(t, 0, result.RecordsModified)
	assert.Equal(t, "Logo caching disabled (no cacher configured)", result.Message)
}

func TestNewConstructor(t *testing.T) {
	constructor := NewConstructor(nil)
	deps := &core.Dependencies{}

	stage := constructor(deps)
	require.NotNil(t, stage)
	assert.Equal(t, "logo_caching", stage.ID())
}

func TestStage_ContextCancellation(t *testing.T) {
	cacher := newMockLogoCacher()
	stage := New(cacher)

	channels := make([]*models.Channel, 100)
	for i := range channels {
		channels[i] = testChannel("Channel", "http://example.com/logo.png")
	}

	proxy := &models.StreamProxy{}
	state := core.NewState(proxy)
	state.Channels = channels

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := stage.Execute(ctx, state)
	assert.ErrorIs(t, err, context.Canceled)
}
