package ingestor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
)

func TestStateManager_Start(t *testing.T) {
	m := NewStateManager()

	sourceID := models.NewULID()
	source := &models.StreamSource{
		BaseModel: models.BaseModel{ID: sourceID},
		Name:      "Test Source",
	}

	// Start should succeed
	err := m.Start(source)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// State should be ingesting
	state, exists := m.GetState(sourceID)
	if !exists {
		t.Fatal("expected state to exist")
	}
	if state.Status != "ingesting" {
		t.Errorf("expected status 'ingesting', got %q", state.Status)
	}
	if state.SourceName != "Test Source" {
		t.Errorf("expected source name 'Test Source', got %q", state.SourceName)
	}

	// Starting again should fail
	err = m.Start(source)
	if err == nil {
		t.Error("expected error for duplicate start")
	}
}

func TestStateManager_UpdateProgress(t *testing.T) {
	m := NewStateManager()

	sourceID := models.NewULID()
	source := &models.StreamSource{BaseModel: models.BaseModel{ID: sourceID}, Name: "Test"}
	_ = m.Start(source)

	m.UpdateProgress(sourceID, 100, 5)

	state, _ := m.GetState(sourceID)
	if state.Processed != 100 {
		t.Errorf("expected processed 100, got %d", state.Processed)
	}
	if state.Errors != 5 {
		t.Errorf("expected errors 5, got %d", state.Errors)
	}
}

func TestStateManager_Complete(t *testing.T) {
	m := NewStateManager()

	sourceID := models.NewULID()
	source := &models.StreamSource{BaseModel: models.BaseModel{ID: sourceID}, Name: "Test"}
	_ = m.Start(source)

	m.Complete(sourceID, 500)

	state, exists := m.GetState(sourceID)
	if !exists {
		t.Fatal("expected state to exist immediately after completion")
	}
	if state.Status != "completed" {
		t.Errorf("expected status 'completed', got %q", state.Status)
	}
	if state.Processed != 500 {
		t.Errorf("expected processed 500, got %d", state.Processed)
	}
}

func TestStateManager_Fail(t *testing.T) {
	m := NewStateManager()

	sourceID := models.NewULID()
	source := &models.StreamSource{BaseModel: models.BaseModel{ID: sourceID}, Name: "Test"}
	_ = m.Start(source)

	expectedErr := errors.New("test error")
	m.Fail(sourceID, expectedErr)

	state, exists := m.GetState(sourceID)
	if !exists {
		t.Fatal("expected state to exist immediately after failure")
	}
	if state.Status != "failed" {
		t.Errorf("expected status 'failed', got %q", state.Status)
	}
	if state.Error != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, state.Error)
	}
}

func TestStateManager_Cancel(t *testing.T) {
	m := NewStateManager()

	sourceID := models.NewULID()
	source := &models.StreamSource{BaseModel: models.BaseModel{ID: sourceID}, Name: "Test"}
	_ = m.Start(source)

	m.Cancel(sourceID)

	// Should be removed immediately
	_, exists := m.GetState(sourceID)
	if exists {
		t.Error("expected state to be removed after cancel")
	}
}

func TestStateManager_IsIngesting(t *testing.T) {
	m := NewStateManager()

	sourceID := models.NewULID()

	// Not ingesting initially
	if m.IsIngesting(sourceID) {
		t.Error("expected IsIngesting to be false initially")
	}

	source := &models.StreamSource{BaseModel: models.BaseModel{ID: sourceID}, Name: "Test"}
	_ = m.Start(source)

	// Should be ingesting now
	if !m.IsIngesting(sourceID) {
		t.Error("expected IsIngesting to be true after start")
	}

	m.Complete(sourceID, 100)

	// Should not be ingesting after completion
	if m.IsIngesting(sourceID) {
		t.Error("expected IsIngesting to be false after completion")
	}
}

func TestStateManager_GetAllStates(t *testing.T) {
	m := NewStateManager()

	// Start multiple ingestions
	for range 3 {
		sourceID := models.NewULID()
		_ = m.Start(&models.StreamSource{BaseModel: models.BaseModel{ID: sourceID}, Name: "Source"})
	}

	states := m.GetAllStates()
	if len(states) != 3 {
		t.Errorf("expected 3 states, got %d", len(states))
	}
}

func TestStateManager_WaitForCompletion(t *testing.T) {
	m := NewStateManager()

	sourceID := models.NewULID()
	source := &models.StreamSource{BaseModel: models.BaseModel{ID: sourceID}, Name: "Test"}
	_ = m.Start(source)

	// Complete in a goroutine
	go func() {
		time.Sleep(50 * time.Millisecond)
		m.Complete(sourceID, 100)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := m.WaitForCompletion(ctx, sourceID)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStateManager_WaitForCompletion_ContextCancelled(t *testing.T) {
	m := NewStateManager()

	sourceID := models.NewULID()
	source := &models.StreamSource{BaseModel: models.BaseModel{ID: sourceID}, Name: "Test"}
	_ = m.Start(source)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := m.WaitForCompletion(ctx, sourceID)
	if err == nil {
		t.Error("expected context deadline error")
	}
}

func TestStateManager_WaitForCompletion_NotExists(t *testing.T) {
	m := NewStateManager()

	ctx := context.Background()
	nonExistentID := models.NewULID()
	err := m.WaitForCompletion(ctx, nonExistentID)
	if err != nil {
		t.Errorf("expected no error for non-existent source, got: %v", err)
	}
}

func TestStateManager_WaitForCompletion_Failed(t *testing.T) {
	m := NewStateManager()

	sourceID := models.NewULID()
	source := &models.StreamSource{BaseModel: models.BaseModel{ID: sourceID}, Name: "Test"}
	_ = m.Start(source)

	expectedErr := errors.New("ingestion failed")

	// Fail in a goroutine
	go func() {
		time.Sleep(50 * time.Millisecond)
		m.Fail(sourceID, expectedErr)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := m.WaitForCompletion(ctx, sourceID)
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}
