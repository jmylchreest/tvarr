package httpclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
	registry := NewRegistry()
	client := NewWithDefaults()

	registry.Register("test-client", client)

	got := registry.Get("test-client")
	assert.Equal(t, client, got)

	got = registry.Get("nonexistent")
	assert.Nil(t, got)
}

func TestRegistry_Unregister(t *testing.T) {
	registry := NewRegistry()
	client := NewWithDefaults()

	registry.Register("test-client", client)
	registry.Unregister("test-client")

	got := registry.Get("test-client")
	assert.Nil(t, got)
}

func TestRegistry_Names(t *testing.T) {
	registry := NewRegistry()

	registry.Register("client-a", NewWithDefaults())
	registry.Register("client-b", NewWithDefaults())

	names := registry.Names()
	assert.Len(t, names, 2)
	assert.Contains(t, names, "client-a")
	assert.Contains(t, names, "client-b")
}

func TestRegistry_GetCircuitBreakerStatuses(t *testing.T) {
	registry := NewRegistry()

	client := NewWithDefaults()
	registry.Register("test-client", client)

	statuses := registry.GetCircuitBreakerStatuses()
	require.Len(t, statuses, 1)

	assert.Equal(t, "test-client", statuses[0].Name)
	assert.Equal(t, "closed", statuses[0].State)
	assert.Equal(t, 0, statuses[0].Failures)
}

func TestRegistry_GetCircuitBreakerStatuses_WithFailures(t *testing.T) {
	registry := NewRegistry()

	client := NewWithDefaults()
	// Simulate failures by directly recording them
	client.breaker.RecordFailure()
	client.breaker.RecordFailure()

	registry.Register("failing-client", client)

	statuses := registry.GetCircuitBreakerStatuses()
	require.Len(t, statuses, 1)

	assert.Equal(t, "failing-client", statuses[0].Name)
	assert.Equal(t, "closed", statuses[0].State) // Still closed, under threshold
	assert.Equal(t, 2, statuses[0].Failures)
}

func TestDefaultRegistry(t *testing.T) {
	// Ensure default registry is initialized
	assert.NotNil(t, DefaultRegistry)

	// Clean up any existing registrations from other tests
	for _, name := range DefaultRegistry.Names() {
		DefaultRegistry.Unregister(name)
	}

	client := NewWithDefaults()
	DefaultRegistry.Register("default-test", client)

	got := DefaultRegistry.Get("default-test")
	assert.Equal(t, client, got)

	// Clean up
	DefaultRegistry.Unregister("default-test")
}
