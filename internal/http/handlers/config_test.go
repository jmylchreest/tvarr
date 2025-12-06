package handlers

import (
	"context"
	"testing"

	"github.com/jmylchreest/tvarr/internal/observability"
	"github.com/jmylchreest/tvarr/pkg/httpclient"
)

func TestConfigHandler_GetConfig(t *testing.T) {
	// Create handler with mocked dependencies
	cbManager := httpclient.NewCircuitBreakerManager(nil)
	featureHandler := NewFeatureHandler()
	handler := NewConfigHandler(cbManager, featureHandler)

	output, err := handler.GetConfig(context.Background(), &UnifiedConfigInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output == nil {
		t.Fatal("expected non-nil output")
	}

	if !output.Body.Success {
		t.Error("expected success=true")
	}

	// Verify runtime settings are populated
	if output.Body.Runtime.Settings.LogLevel == "" {
		t.Error("expected log_level to be set")
	}

	// Verify features are populated
	if output.Body.Runtime.Features == nil {
		t.Error("expected features to be set")
	}

	// Verify circuit breaker config is populated
	if output.Body.Runtime.CircuitBreakers.Global.FailureThreshold == 0 {
		t.Error("expected circuit breaker global config to be set")
	}

	// Verify meta is populated
	if output.Body.Meta.Source == "" {
		t.Error("expected meta.source to be set")
	}
}

func TestConfigHandler_UpdateConfig(t *testing.T) {
	t.Run("updates log level", func(t *testing.T) {
		cbManager := httpclient.NewCircuitBreakerManager(nil)
		featureHandler := NewFeatureHandler()
		handler := NewConfigHandler(cbManager, featureHandler)

		// Save original log level
		originalLevel := observability.GetLogLevel()
		defer observability.SetLogLevel(originalLevel)

		// Update to debug
		output, err := handler.UpdateConfig(context.Background(), &UnifiedConfigUpdateInput{
			Body: UnifiedConfigUpdate{
				Settings: &ConfigRuntimeSettings{
					LogLevel:             "debug",
					EnableRequestLogging: false,
				},
			},
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !output.Body.Success {
			t.Error("expected success=true")
		}

		if len(output.Body.AppliedChanges) == 0 {
			t.Error("expected applied changes to be recorded")
		}

		// Verify change was applied
		if observability.GetLogLevel() != "debug" {
			t.Errorf("expected log level to be 'debug', got '%s'", observability.GetLogLevel())
		}
	})

	t.Run("updates features", func(t *testing.T) {
		cbManager := httpclient.NewCircuitBreakerManager(nil)
		featureHandler := NewFeatureHandler()
		handler := NewConfigHandler(cbManager, featureHandler)

		output, err := handler.UpdateConfig(context.Background(), &UnifiedConfigUpdateInput{
			Body: UnifiedConfigUpdate{
				Features: map[string]bool{
					"debug-frontend": true,
				},
			},
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !output.Body.Success {
			t.Error("expected success=true")
		}

		// Verify feature was updated
		features := handler.getFeatures()
		if !features["debug-frontend"] {
			t.Error("expected debug-frontend feature to be true")
		}
	})

	t.Run("updates circuit breaker config", func(t *testing.T) {
		cbManager := httpclient.NewCircuitBreakerManager(nil)
		featureHandler := NewFeatureHandler()
		handler := NewConfigHandler(cbManager, featureHandler)

		output, err := handler.UpdateConfig(context.Background(), &UnifiedConfigUpdateInput{
			Body: UnifiedConfigUpdate{
				CircuitBreakers: &CircuitBreakerConfigUpdateData{
					Global: &CircuitBreakerProfile{
						FailureThreshold: 10,
						ResetTimeout:     "60s",
						HalfOpenMax:      2,
					},
				},
			},
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !output.Body.Success {
			t.Error("expected success=true")
		}

		// Verify CB config was updated
		cfg := cbManager.GetConfig()
		if cfg.Global.FailureThreshold != 10 {
			t.Errorf("expected failure threshold to be 10, got %d", cfg.Global.FailureThreshold)
		}
	})
}

func TestConfigHandler_NilDependencies(t *testing.T) {
	// Should handle nil circuit breaker manager
	handler := NewConfigHandler(nil, nil)

	output, err := handler.GetConfig(context.Background(), &UnifiedConfigInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !output.Body.Success {
		t.Error("expected success=true")
	}

	// Features should be empty but not nil
	if output.Body.Runtime.Features == nil {
		t.Error("expected features to be an empty map, not nil")
	}
}
