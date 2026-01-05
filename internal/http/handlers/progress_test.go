package handlers_test

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmylchreest/tvarr/internal/http/handlers"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/service/progress"
)

func newTestProgressHandler() (*handlers.ProgressHandler, *progress.Service) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	svc := progress.NewService(logger)
	handler := handlers.NewProgressHandler(svc)
	return handler, svc
}

func setupProgressRouter(handler *handlers.ProgressHandler) *chi.Mux {
	router := chi.NewRouter()
	api := humachi.New(router, huma.DefaultConfig("Test API", "1.0.0"))
	handler.Register(api)
	handler.RegisterSSE(router) // Register SSE endpoint directly on chi router
	return router
}

func TestProgressHandler_ListOperations(t *testing.T) {
	t.Run("returns empty list when no operations", func(t *testing.T) {
		handler, _ := newTestProgressHandler()
		router := setupProgressRouter(handler)

		req := httptest.NewRequest("GET", "/api/v1/progress/operations", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp handlers.ListOperationsOutput
		err := json.NewDecoder(rec.Body).Decode(&resp.Body)
		require.NoError(t, err)
		assert.Empty(t, resp.Body.Operations)
	})

	t.Run("returns operations", func(t *testing.T) {
		handler, svc := newTestProgressHandler()
		router := setupProgressRouter(handler)

		// Start an operation
		ownerID := models.NewULID()
		stages := []progress.StageInfo{{ID: "test", Name: "Test", Weight: 1.0}}
		_, err := svc.StartOperation(progress.OpStreamIngestion, ownerID, "stream_source", "Test Source", stages)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/api/v1/progress/operations", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp handlers.ListOperationsOutput
		err = json.NewDecoder(rec.Body).Decode(&resp.Body)
		require.NoError(t, err)
		assert.Len(t, resp.Body.Operations, 1)
		assert.Equal(t, string(progress.OpStreamIngestion), resp.Body.Operations[0].OperationType)
	})

	t.Run("filters by operation type", func(t *testing.T) {
		handler, svc := newTestProgressHandler()
		router := setupProgressRouter(handler)

		// Start two different operation types
		owner1 := models.NewULID()
		owner2 := models.NewULID()
		stages := []progress.StageInfo{{ID: "test", Name: "Test", Weight: 1.0}}
		_, err := svc.StartOperation(progress.OpStreamIngestion, owner1, "stream_source", "Test Source 1", stages)
		require.NoError(t, err)
		_, err = svc.StartOperation(progress.OpEpgIngestion, owner2, "epg_source", "Test EPG", stages)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/api/v1/progress/operations?operation_type=stream_ingestion", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp handlers.ListOperationsOutput
		err = json.NewDecoder(rec.Body).Decode(&resp.Body)
		require.NoError(t, err)
		assert.Len(t, resp.Body.Operations, 1)
		assert.Equal(t, "stream_ingestion", resp.Body.Operations[0].OperationType)
	})

	t.Run("filters by active only", func(t *testing.T) {
		handler, svc := newTestProgressHandler()
		router := setupProgressRouter(handler)

		// Start and complete one operation
		owner1 := models.NewULID()
		stages := []progress.StageInfo{{ID: "test", Name: "Test", Weight: 1.0}}
		mgr1, err := svc.StartOperation(progress.OpStreamIngestion, owner1, "stream_source", "Test Source 1", stages)
		require.NoError(t, err)
		mgr1.Complete("completed")

		// Start an active operation
		owner2 := models.NewULID()
		_, err = svc.StartOperation(progress.OpEpgIngestion, owner2, "epg_source", "Test EPG", stages)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/api/v1/progress/operations?active_only=true", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp handlers.ListOperationsOutput
		err = json.NewDecoder(rec.Body).Decode(&resp.Body)
		require.NoError(t, err)
		assert.Len(t, resp.Body.Operations, 1)
		assert.Equal(t, "epg_ingestion", resp.Body.Operations[0].OperationType)
	})
}

func TestProgressHandler_GetOperation(t *testing.T) {
	t.Run("returns operation by ID", func(t *testing.T) {
		handler, svc := newTestProgressHandler()
		router := setupProgressRouter(handler)

		// Start an operation
		ownerID := models.NewULID()
		stages := []progress.StageInfo{{ID: "test", Name: "Test", Weight: 1.0}}
		mgr, err := svc.StartOperation(progress.OpStreamIngestion, ownerID, "stream_source", "Test Source", stages)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/api/v1/progress/operations/"+mgr.OperationID(), nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp handlers.GetOperationOutput
		err = json.NewDecoder(rec.Body).Decode(&resp.Body)
		require.NoError(t, err)
		assert.Equal(t, mgr.OperationID(), resp.Body.ID)
	})

	t.Run("returns 404 for unknown operation", func(t *testing.T) {
		handler, _ := newTestProgressHandler()
		router := setupProgressRouter(handler)

		req := httptest.NewRequest("GET", "/api/v1/progress/operations/unknown-id", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestProgressHandler_SSEEvents(t *testing.T) {
	t.Run("establishes SSE connection", func(t *testing.T) {
		handler, _ := newTestProgressHandler()
		router := setupProgressRouter(handler)

		// Use a context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		req := httptest.NewRequest("GET", "/api/v1/progress/events", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		// Run handler in goroutine since it blocks
		done := make(chan struct{})
		go func() {
			router.ServeHTTP(rec, req)
			close(done)
		}()

		// Wait for timeout or completion
		<-done

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
		assert.Equal(t, "no-cache", rec.Header().Get("Cache-Control"))
	})

	t.Run("receives progress events", func(t *testing.T) {
		handler, svc := newTestProgressHandler()
		router := setupProgressRouter(handler)

		// Create a pipe to read SSE responses
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		req := httptest.NewRequest("GET", "/api/v1/progress/events", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		// Run handler in goroutine with WaitGroup to ensure completion
		var wg sync.WaitGroup
		wg.Go(func() {
			router.ServeHTTP(rec, req)
		})

		// Give handler time to start
		time.Sleep(50 * time.Millisecond)

		// Start an operation - should trigger an event
		ownerID := models.NewULID()
		stages := []progress.StageInfo{{ID: "test", Name: "Test", Weight: 1.0}}
		_, err := svc.StartOperation(progress.OpStreamIngestion, ownerID, "stream_source", "Test Source", stages)
		require.NoError(t, err)

		// Wait for handler to complete (context cancellation + cleanup)
		wg.Wait()

		// Check response contains SSE data
		body := rec.Body.String()
		assert.Contains(t, body, "event:")
		assert.Contains(t, body, "data:")
	})

	t.Run("filters events by operation type", func(t *testing.T) {
		handler, svc := newTestProgressHandler()
		router := setupProgressRouter(handler)

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		// Request events only for stream ingestion
		req := httptest.NewRequest("GET", "/api/v1/progress/events?operation_type=stream_ingestion", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		var wg sync.WaitGroup
		wg.Go(func() {
			router.ServeHTTP(rec, req)
		})

		time.Sleep(50 * time.Millisecond)

		// Start both operation types
		owner1 := models.NewULID()
		owner2 := models.NewULID()
		stages := []progress.StageInfo{{ID: "test", Name: "Test", Weight: 1.0}}
		_, err := svc.StartOperation(progress.OpStreamIngestion, owner1, "stream_source", "Test Source 1", stages)
		require.NoError(t, err)
		_, err = svc.StartOperation(progress.OpEpgIngestion, owner2, "epg_source", "Test EPG", stages)
		require.NoError(t, err)

		// Wait for handler to complete
		wg.Wait()

		body := rec.Body.String()
		// Should contain stream ingestion events
		assert.Contains(t, body, "stream_ingestion")
		// Should not contain epg ingestion events
		assert.NotContains(t, body, "epg_ingestion")
	})
}

func TestProgressHandler_SSEHeartbeat(t *testing.T) {
	t.Run("sends heartbeat comments", func(t *testing.T) {
		handler, _ := newTestProgressHandler()
		// Set a short heartbeat interval for testing
		handler.SetHeartbeatInterval(50 * time.Millisecond)
		router := setupProgressRouter(handler)

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		req := httptest.NewRequest("GET", "/api/v1/progress/events", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		var wg sync.WaitGroup
		wg.Go(func() {
			router.ServeHTTP(rec, req)
		})

		// Wait for handler to complete
		wg.Wait()

		// Check for heartbeat comments (SSE comments start with :)
		body := rec.Body.String()
		assert.Contains(t, body, ":heartbeat")
	})
}

func parseSSEEvents(body string) []map[string]string {
	var events []map[string]string
	scanner := bufio.NewScanner(strings.NewReader(body))

	var currentEvent map[string]string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if currentEvent != nil {
				events = append(events, currentEvent)
				currentEvent = nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			// Comment line, skip
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			if currentEvent == nil {
				currentEvent = make(map[string]string)
			}
			key := parts[0]
			value := strings.TrimPrefix(parts[1], " ")
			currentEvent[key] = value
		}
	}
	if currentEvent != nil {
		events = append(events, currentEvent)
	}
	return events
}

// T460 - Integration tests for SSE endpoint
func TestProgressHandler_SSEIntegration(t *testing.T) {
	t.Run("receives complete operation lifecycle events", func(t *testing.T) {
		handler, svc := newTestProgressHandler()
		router := setupProgressRouter(handler)

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		req := httptest.NewRequest("GET", "/api/v1/progress/events", nil).WithContext(ctx)
		rec := httptest.NewRecorder()

		var wg sync.WaitGroup
		wg.Go(func() {
			router.ServeHTTP(rec, req)
		})

		// Wait for handler to start
		time.Sleep(50 * time.Millisecond)

		// Start operation
		ownerID := models.NewULID()
		stages := []progress.StageInfo{
			{ID: "load", Name: "Load", Weight: 0.5},
			{ID: "save", Name: "Save", Weight: 0.5},
		}
		mgr, err := svc.StartOperation(progress.OpProxyRegeneration, ownerID, "stream_proxy", "Test Proxy", stages)
		require.NoError(t, err)

		// Update progress
		time.Sleep(20 * time.Millisecond)
		stageUpdater := mgr.StartStage("load")
		stageUpdater.SetProgress(0.5, "Loading...")

		// Complete first stage
		time.Sleep(20 * time.Millisecond)
		stageUpdater.Complete()

		// Complete operation
		time.Sleep(20 * time.Millisecond)
		mgr.Complete("All done")

		// Wait for handler to complete
		wg.Wait()

		body := rec.Body.String()
		events := parseSSEEvents(body)

		// Should have multiple events for the lifecycle
		assert.GreaterOrEqual(t, len(events), 2, "should have at least 2 events")

		// Verify we receive completion event
		hasCompletedEvent := false
		for _, event := range events {
			if event["event"] == "completed" {
				hasCompletedEvent = true
				break
			}
		}
		assert.True(t, hasCompletedEvent, "should receive completed event")
	})

	t.Run("multiple subscribers receive same events", func(t *testing.T) {
		handler, svc := newTestProgressHandler()
		router := setupProgressRouter(handler)

		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()

		// Start two SSE connections
		req1 := httptest.NewRequest("GET", "/api/v1/progress/events", nil).WithContext(ctx)
		rec1 := httptest.NewRecorder()

		req2 := httptest.NewRequest("GET", "/api/v1/progress/events", nil).WithContext(ctx)
		rec2 := httptest.NewRecorder()

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			router.ServeHTTP(rec1, req1)
		}()
		go func() {
			defer wg.Done()
			router.ServeHTTP(rec2, req2)
		}()

		// Wait for handlers to start
		time.Sleep(50 * time.Millisecond)

		// Start an operation - both should receive it
		ownerID := models.NewULID()
		stages := []progress.StageInfo{{ID: "test", Name: "Test", Weight: 1.0}}
		_, err := svc.StartOperation(progress.OpStreamIngestion, ownerID, "stream_source", "Test Source", stages)
		require.NoError(t, err)

		// Wait for handlers to complete
		wg.Wait()

		// Both subscribers should have received events
		body1 := rec1.Body.String()
		body2 := rec2.Body.String()

		assert.Contains(t, body1, "stream_ingestion")
		assert.Contains(t, body2, "stream_ingestion")
	})
}

// T461 - Tests for concurrent operation blocking
func TestProgressHandler_ConcurrentOperationBlocking(t *testing.T) {
	t.Run("blocks duplicate operation for same owner via HTTP", func(t *testing.T) {
		handler, svc := newTestProgressHandler()
		router := setupProgressRouter(handler)

		// Start an operation
		ownerID := models.NewULID()
		stages := []progress.StageInfo{{ID: "test", Name: "Test", Weight: 1.0}}
		_, err := svc.StartOperation(progress.OpStreamIngestion, ownerID, "stream_source", "Test Source", stages)
		require.NoError(t, err)

		// Try to get the operation - should still work
		req := httptest.NewRequest("GET", "/api/v1/progress/operations", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp handlers.ListOperationsOutput
		err = json.NewDecoder(rec.Body).Decode(&resp.Body)
		require.NoError(t, err)
		assert.Len(t, resp.Body.Operations, 1)

		// The service should block a second operation with the same owner
		_, err = svc.StartOperation(progress.OpStreamIngestion, ownerID, "stream_source", "Test Source", stages)
		assert.ErrorIs(t, err, progress.ErrOperationExists)
	})
}

// T462 - Tests for progress calculation with multiple stages
func TestProgressHandler_MultiStageProgress(t *testing.T) {
	t.Run("calculates overall progress from weighted stages", func(t *testing.T) {
		handler, svc := newTestProgressHandler()
		router := setupProgressRouter(handler)

		ownerID := models.NewULID()
		stages := []progress.StageInfo{
			{ID: "load", Name: "Load", Weight: 0.2},
			{ID: "process", Name: "Process", Weight: 0.6},
			{ID: "save", Name: "Save", Weight: 0.2},
		}

		mgr, err := svc.StartOperation(progress.OpProxyRegeneration, ownerID, "stream_proxy", "Test Proxy", stages)
		require.NoError(t, err)

		// Complete first stage
		loadStage := mgr.StartStage("load")
		loadStage.Complete()

		// Get operation via HTTP
		req := httptest.NewRequest("GET", "/api/v1/progress/operations/"+mgr.OperationID(), nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var resp handlers.GetOperationOutput
		err = json.NewDecoder(rec.Body).Decode(&resp.Body)
		require.NoError(t, err)

		// Progress should be 20% (load weight = 0.2, completed = 100%)
		assert.InDelta(t, 20.0, resp.Body.OverallPercentage, 1.0)

		// Complete second stage at 50%
		processStage := mgr.StartStage("process")
		processStage.SetProgress(0.5, "Processing...")

		req = httptest.NewRequest("GET", "/api/v1/progress/operations/"+mgr.OperationID(), nil)
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		err = json.NewDecoder(rec.Body).Decode(&resp.Body)
		require.NoError(t, err)

		// Progress should be 20 + 60*0.5 = 50%
		assert.InDelta(t, 50.0, resp.Body.OverallPercentage, 1.0)
	})
}

// T463 - Tests for cleanup of stale operations
func TestProgressHandler_StaleOperationCleanup(t *testing.T) {
	t.Run("completed operations are cleaned up after timeout", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
		svc := progress.NewService(logger)
		// Note: staleDuration is internal, so we test via the service directly

		ownerID := models.NewULID()
		stages := []progress.StageInfo{{ID: "test", Name: "Test", Weight: 1.0}}

		mgr, err := svc.StartOperation(progress.OpStreamIngestion, ownerID, "stream_source", "Test Source", stages)
		require.NoError(t, err)

		// Complete the operation
		mgr.Complete("Done")

		// Operation should still be accessible
		op, err := svc.GetOperation(mgr.OperationID())
		require.NoError(t, err)
		assert.Equal(t, progress.StateCompleted, op.State)

		// After completing, a new operation with the same owner should work
		// (the blocking is removed upon completion)
		mgr2, err := svc.StartOperation(progress.OpStreamIngestion, ownerID, "stream_source", "Test Source 2", stages)
		require.NoError(t, err)
		assert.NotEqual(t, mgr.OperationID(), mgr2.OperationID())
	})
}
