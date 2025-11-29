package ingestor

import (
	"fmt"
	"sync"

	"github.com/jmylchreest/tvarr/internal/models"
)

// HandlerFactory creates and manages source handlers.
type HandlerFactory struct {
	mu       sync.RWMutex
	handlers map[models.SourceType]SourceHandler
}

// NewHandlerFactory creates a new handler factory with default handlers registered.
func NewHandlerFactory() *HandlerFactory {
	f := &HandlerFactory{
		handlers: make(map[models.SourceType]SourceHandler),
	}

	// Register default handlers
	f.Register(NewM3UHandler())
	f.Register(NewXtreamHandler())

	return f
}

// Register adds a handler to the factory.
func (f *HandlerFactory) Register(handler SourceHandler) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[handler.Type()] = handler
}

// Get returns a handler for the given source type.
func (f *HandlerFactory) Get(sourceType models.SourceType) (SourceHandler, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	handler, ok := f.handlers[sourceType]
	if !ok {
		return nil, fmt.Errorf("no handler registered for source type: %s", sourceType)
	}
	return handler, nil
}

// GetForSource returns a handler for the given source.
func (f *HandlerFactory) GetForSource(source *models.StreamSource) (SourceHandler, error) {
	if source == nil {
		return nil, fmt.Errorf("source is nil")
	}
	return f.Get(source.Type)
}

// SupportedTypes returns all registered source types.
func (f *HandlerFactory) SupportedTypes() []models.SourceType {
	f.mu.RLock()
	defer f.mu.RUnlock()

	types := make([]models.SourceType, 0, len(f.handlers))
	for t := range f.handlers {
		types = append(types, t)
	}
	return types
}

// EpgHandlerFactory creates and manages EPG source handlers.
type EpgHandlerFactory struct {
	mu       sync.RWMutex
	handlers map[models.EpgSourceType]EpgHandler
}

// NewEpgHandlerFactory creates a new EPG handler factory with default handlers registered.
func NewEpgHandlerFactory() *EpgHandlerFactory {
	f := &EpgHandlerFactory{
		handlers: make(map[models.EpgSourceType]EpgHandler),
	}

	// Register default handlers
	f.Register(NewXMLTVHandler())
	f.Register(NewXtreamEpgHandler())

	return f
}

// Register adds an EPG handler to the factory.
func (f *EpgHandlerFactory) Register(handler EpgHandler) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[handler.Type()] = handler
}

// Get returns an EPG handler for the given source type.
func (f *EpgHandlerFactory) Get(sourceType models.EpgSourceType) (EpgHandler, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	handler, ok := f.handlers[sourceType]
	if !ok {
		return nil, fmt.Errorf("no EPG handler registered for source type: %s", sourceType)
	}
	return handler, nil
}

// GetForSource returns an EPG handler for the given source.
func (f *EpgHandlerFactory) GetForSource(source *models.EpgSource) (EpgHandler, error) {
	if source == nil {
		return nil, fmt.Errorf("source is nil")
	}
	return f.Get(source.Type)
}

// SupportedTypes returns all registered EPG source types.
func (f *EpgHandlerFactory) SupportedTypes() []models.EpgSourceType {
	f.mu.RLock()
	defer f.mu.RUnlock()

	types := make([]models.EpgSourceType, 0, len(f.handlers))
	for t := range f.handlers {
		types = append(types, t)
	}
	return types
}
