// Package httpclient provides a resilient HTTP client with circuit breaker,
// automatic retries, transparent decompression, and structured logging.
package httpclient

import (
	"sync"
)

// CircuitBreakerStatus represents the status of a circuit breaker for health reporting.
type CircuitBreakerStatus struct {
	Name     string `json:"name"`
	State    string `json:"state"`
	Failures int    `json:"failures"`
}

// Registry maintains a collection of named HTTP clients for health monitoring.
// It allows components to register their HTTP clients so circuit breaker
// states can be observed via health endpoints.
type Registry struct {
	mu      sync.RWMutex
	clients map[string]*Client
}

// NewRegistry creates a new client registry.
func NewRegistry() *Registry {
	return &Registry{
		clients: make(map[string]*Client),
	}
}

// Register adds a named client to the registry.
// If a client with the same name already exists, it is replaced.
func (r *Registry) Register(name string, client *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[name] = client
}

// Unregister removes a client from the registry.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clients, name)
}

// Get returns a client by name, or nil if not found.
func (r *Registry) Get(name string) *Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.clients[name]
}

// GetCircuitBreakerStatuses returns the status of all registered circuit breakers.
func (r *Registry) GetCircuitBreakerStatuses() []CircuitBreakerStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	statuses := make([]CircuitBreakerStatus, 0, len(r.clients))
	for name, client := range r.clients {
		statuses = append(statuses, CircuitBreakerStatus{
			Name:     name,
			State:    client.CircuitState().String(),
			Failures: client.breaker.Failures(),
		})
	}
	return statuses
}

// Names returns the names of all registered clients.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.clients))
	for name := range r.clients {
		names = append(names, name)
	}
	return names
}

// DefaultRegistry is the global default registry for HTTP clients.
var DefaultRegistry = NewRegistry()
