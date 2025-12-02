package loadchannels

import (
	"context"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestState(t *testing.T) *core.State {
	t.Helper()
	proxy := &models.StreamProxy{
		BaseModel: models.BaseModel{ID: models.NewULID()},
		Name:      "Test Proxy",
	}
	return core.NewState(proxy)
}

// T017-TEST: Test "no sources" error case
func TestStage_Execute_NoSourcesError(t *testing.T) {
	t.Run("returns error when no sources configured", func(t *testing.T) {
		state := newTestState(t)
		state.Sources = []*models.StreamSource{} // Empty sources

		stage := New(nil) // No repo needed since we'll error before using it
		_, err := stage.Execute(context.Background(), state)

		// Should return ErrNoSources
		require.Error(t, err)
		assert.ErrorIs(t, err, core.ErrNoSources)
	})

	t.Run("returns error when sources is nil", func(t *testing.T) {
		state := newTestState(t)
		state.Sources = nil // Nil sources

		stage := New(nil)
		_, err := stage.Execute(context.Background(), state)

		require.Error(t, err)
		assert.ErrorIs(t, err, core.ErrNoSources)
	})
}

func TestStage_Interface(t *testing.T) {
	stage := New(nil)
	assert.Equal(t, StageID, stage.ID())
	assert.Equal(t, StageName, stage.Name())
}

func TestNewConstructor(t *testing.T) {
	constructor := NewConstructor()
	stage := constructor(&core.Dependencies{})
	assert.NotNil(t, stage)
	assert.Equal(t, StageID, stage.ID())
}
