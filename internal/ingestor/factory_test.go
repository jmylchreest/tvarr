package ingestor

import (
	"context"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
)

func TestHandlerFactory_NewFactory(t *testing.T) {
	f := NewHandlerFactory()

	// Should have M3U and Xtream handlers registered by default
	types := f.SupportedTypes()
	if len(types) != 2 {
		t.Errorf("expected 2 handlers, got %d", len(types))
	}

	// Verify M3U handler
	h, err := f.Get(models.SourceTypeM3U)
	if err != nil {
		t.Errorf("expected M3U handler, got error: %v", err)
	}
	if h.Type() != models.SourceTypeM3U {
		t.Errorf("expected M3U type, got %s", h.Type())
	}

	// Verify Xtream handler
	h, err = f.Get(models.SourceTypeXtream)
	if err != nil {
		t.Errorf("expected Xtream handler, got error: %v", err)
	}
	if h.Type() != models.SourceTypeXtream {
		t.Errorf("expected Xtream type, got %s", h.Type())
	}
}

func TestHandlerFactory_Get_NotFound(t *testing.T) {
	f := NewHandlerFactory()

	_, err := f.Get("unknown")
	if err == nil {
		t.Error("expected error for unknown source type")
	}
}

func TestHandlerFactory_GetForSource(t *testing.T) {
	f := NewHandlerFactory()

	// Test with M3U source
	source := &models.StreamSource{
		Name: "Test",
		Type: models.SourceTypeM3U,
		URL:  "http://example.com",
	}
	h, err := f.GetForSource(source)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if h.Type() != models.SourceTypeM3U {
		t.Errorf("expected M3U handler, got %s", h.Type())
	}

	// Test with nil source
	_, err = f.GetForSource(nil)
	if err == nil {
		t.Error("expected error for nil source")
	}
}

// mockHandler is a test handler for custom registration
type mockHandler struct {
	sourceType models.SourceType
}

func (h *mockHandler) Type() models.SourceType {
	return h.sourceType
}

func (h *mockHandler) Ingest(ctx context.Context, source *models.StreamSource, callback ChannelCallback) error {
	return nil
}

func (h *mockHandler) Validate(source *models.StreamSource) error {
	return nil
}

func TestHandlerFactory_Register(t *testing.T) {
	f := NewHandlerFactory()

	// Register a custom handler
	customType := models.SourceType("custom")
	mock := &mockHandler{sourceType: customType}
	f.Register(mock)

	// Verify it was registered
	h, err := f.Get(customType)
	if err != nil {
		t.Errorf("expected custom handler, got error: %v", err)
	}
	if h.Type() != customType {
		t.Errorf("expected custom type, got %s", h.Type())
	}
}

func TestHandlerFactory_SupportedTypes(t *testing.T) {
	f := NewHandlerFactory()

	types := f.SupportedTypes()

	// Should have at least M3U and Xtream
	hasM3U := false
	hasXtream := false
	for _, t := range types {
		if t == models.SourceTypeM3U {
			hasM3U = true
		}
		if t == models.SourceTypeXtream {
			hasXtream = true
		}
	}

	if !hasM3U {
		t.Error("expected M3U in supported types")
	}
	if !hasXtream {
		t.Error("expected Xtream in supported types")
	}
}

// EPG Handler Factory Tests

func TestEpgHandlerFactory_NewFactory(t *testing.T) {
	f := NewEpgHandlerFactory()

	// Should have XMLTV and Xtream handlers registered by default
	types := f.SupportedTypes()
	if len(types) != 2 {
		t.Errorf("expected 2 EPG handlers, got %d", len(types))
	}

	// Verify XMLTV handler
	h, err := f.Get(models.EpgSourceTypeXMLTV)
	if err != nil {
		t.Errorf("expected XMLTV handler, got error: %v", err)
	}
	if h.Type() != models.EpgSourceTypeXMLTV {
		t.Errorf("expected XMLTV type, got %s", h.Type())
	}

	// Verify Xtream handler
	h, err = f.Get(models.EpgSourceTypeXtream)
	if err != nil {
		t.Errorf("expected Xtream EPG handler, got error: %v", err)
	}
	if h.Type() != models.EpgSourceTypeXtream {
		t.Errorf("expected Xtream type, got %s", h.Type())
	}
}

func TestEpgHandlerFactory_Get_NotFound(t *testing.T) {
	f := NewEpgHandlerFactory()

	_, err := f.Get("unknown")
	if err == nil {
		t.Error("expected error for unknown EPG source type")
	}
}

func TestEpgHandlerFactory_GetForSource(t *testing.T) {
	f := NewEpgHandlerFactory()

	// Test with XMLTV source
	source := &models.EpgSource{
		Name: "Test",
		Type: models.EpgSourceTypeXMLTV,
		URL:  "http://example.com/epg.xml",
	}
	h, err := f.GetForSource(source)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if h.Type() != models.EpgSourceTypeXMLTV {
		t.Errorf("expected XMLTV handler, got %s", h.Type())
	}

	// Test with nil source
	_, err = f.GetForSource(nil)
	if err == nil {
		t.Error("expected error for nil source")
	}
}

// mockEpgHandler is a test handler for custom registration
type mockEpgHandler struct {
	sourceType models.EpgSourceType
}

func (h *mockEpgHandler) Type() models.EpgSourceType {
	return h.sourceType
}

func (h *mockEpgHandler) Ingest(ctx context.Context, source *models.EpgSource, callback ProgramCallback) error {
	return nil
}

func (h *mockEpgHandler) Validate(source *models.EpgSource) error {
	return nil
}

func TestEpgHandlerFactory_Register(t *testing.T) {
	f := NewEpgHandlerFactory()

	// Register a custom handler
	customType := models.EpgSourceType("custom")
	mock := &mockEpgHandler{sourceType: customType}
	f.Register(mock)

	// Verify it was registered
	h, err := f.Get(customType)
	if err != nil {
		t.Errorf("expected custom EPG handler, got error: %v", err)
	}
	if h.Type() != customType {
		t.Errorf("expected custom type, got %s", h.Type())
	}
}

func TestEpgHandlerFactory_SupportedTypes(t *testing.T) {
	f := NewEpgHandlerFactory()

	types := f.SupportedTypes()

	// Should have at least XMLTV and Xtream
	hasXMLTV := false
	hasXtream := false
	for _, t := range types {
		if t == models.EpgSourceTypeXMLTV {
			hasXMLTV = true
		}
		if t == models.EpgSourceTypeXtream {
			hasXtream = true
		}
	}

	if !hasXMLTV {
		t.Error("expected XMLTV in supported EPG types")
	}
	if !hasXtream {
		t.Error("expected Xtream in supported EPG types")
	}
}
