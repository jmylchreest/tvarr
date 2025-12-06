package handlers

import (
	"context"
	"testing"
)

func TestHealthHandler_GetLivez(t *testing.T) {
	handler := NewHealthHandler("1.0.0")

	output, err := handler.GetLivez(context.Background(), &LivezInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output == nil {
		t.Fatal("expected non-nil output")
	}

	if output.Body.Status != "ok" {
		t.Errorf("expected status 'ok', got '%s'", output.Body.Status)
	}
}

func TestHealthHandler_GetReadyz(t *testing.T) {
	t.Run("returns not_ready when db not configured", func(t *testing.T) {
		handler := NewHealthHandler("1.0.0")

		output, err := handler.GetReadyz(context.Background(), &ReadyzInput{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if output == nil {
			t.Fatal("expected non-nil output")
		}

		if output.Body.Status != "not_ready" {
			t.Errorf("expected status 'not_ready' when db not configured, got '%s'", output.Body.Status)
		}

		if output.Body.Components["database"] != "not_configured" {
			t.Errorf("expected database component to be 'not_configured', got '%s'", output.Body.Components["database"])
		}

		if output.Body.Components["scheduler"] != "ok" {
			t.Errorf("expected scheduler component to be 'ok', got '%s'", output.Body.Components["scheduler"])
		}
	})
}

func TestHealthHandler_GetHealth(t *testing.T) {
	handler := NewHealthHandler("1.0.0")

	output, err := handler.GetHealth(context.Background(), &HealthInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output == nil {
		t.Fatal("expected non-nil output")
	}

	if output.Body.Status != "healthy" {
		t.Errorf("expected status 'healthy', got '%s'", output.Body.Status)
	}

	if output.Body.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%s'", output.Body.Version)
	}

	if output.Body.Uptime == "" {
		t.Error("expected non-empty uptime")
	}

	if output.Body.CPUInfo.Cores == 0 {
		t.Error("expected non-zero CPU cores")
	}
}
