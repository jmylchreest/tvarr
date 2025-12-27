package integration

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmylchreest/tvarr/internal/relay"
)

// TestFFmpegDSubprocessSpawning tests the subprocess spawner lifecycle.
// This is T043 from the task list.
func TestFFmpegDSubprocessSpawning(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check if tvarr-ffmpegd binary is available
	binaryPath := findFFmpegDBinary(t)
	if binaryPath == "" {
		t.Skip("tvarr-ffmpegd binary not found, skipping subprocess test")
	}

	t.Logf("Using tvarr-ffmpegd binary at: %s", binaryPath)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create spawner with explicit binary path
	spawner := relay.NewFFmpegDSpawner(relay.FFmpegDSpawnerConfig{
		BinaryPath:      binaryPath,
		StartupTimeout:  15 * time.Second,
		ShutdownTimeout: 5 * time.Second,
		Logger:          logger,
	})

	t.Run("spawner_is_available", func(t *testing.T) {
		assert.True(t, spawner.IsAvailable(), "spawner should be available with valid binary")
	})

	t.Run("spawn_requires_coordinator", func(t *testing.T) {
		jobID := "integration-test-job-1"

		// SpawnForJob requires a registry and coordinator address to be configured
		// Without them, it should fail gracefully
		_, _, err := spawner.SpawnForJob(ctx, jobID)
		require.Error(t, err, "should fail without coordinator address configured")
		assert.Contains(t, err.Error(), "coordinator address not configured")
	})

	// Note: The following tests require a full coordinator setup to work.
	// SpawnForJob returns (types.DaemonID, func(), error), not a gRPC client.
	// To actually test subprocess lifecycle, we need:
	// 1. A running coordinator gRPC server
	// 2. The spawner configured with the coordinator address
	// 3. The subprocess to register with the coordinator
	// These tests are skipped until full integration test infrastructure is available.

	t.Run("cleanup_terminates_subprocess", func(t *testing.T) {
		t.Skip("requires full coordinator setup to test subprocess lifecycle")
	})

	t.Run("multiple_concurrent_spawns", func(t *testing.T) {
		t.Skip("requires full coordinator setup to test subprocess lifecycle")
	})

	t.Run("cleanup_is_idempotent", func(t *testing.T) {
		t.Skip("requires full coordinator setup to test subprocess lifecycle")
	})
}

// TestFFmpegDSubprocessTranscoding tests actual transcoding through subprocess.
// This requires both tvarr-ffmpegd and ffmpeg to be available, plus a full
// coordinator setup since SpawnForJob returns a daemon ID, not a gRPC client.
func TestFFmpegDSubprocessTranscoding(t *testing.T) {
	// Skip: This test requires full coordinator integration.
	// SpawnForJob returns (types.DaemonID, func(), error), not a gRPC client.
	// To test actual transcoding, we need the full coordinator infrastructure.
	t.Skip("requires full coordinator setup to test transcoding through subprocess")
}

// TestFFmpegDSpawnerErrorHandling tests error cases.
func TestFFmpegDSpawnerErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	t.Run("spawn_with_invalid_binary", func(t *testing.T) {
		spawner := relay.NewFFmpegDSpawner(relay.FFmpegDSpawnerConfig{
			BinaryPath: "/nonexistent/path/tvarr-ffmpegd",
			Logger:     logger,
		})

		assert.False(t, spawner.IsAvailable(), "spawner should not be available with invalid binary")

		daemonID, cleanup, err := spawner.SpawnForJob(ctx, "test-job")
		assert.Error(t, err, "should error with invalid binary")
		assert.Empty(t, daemonID)
		assert.Nil(t, cleanup)
		assert.ErrorIs(t, err, relay.ErrFFmpegDBinaryNotFound)
	})

	t.Run("max_concurrent_spawns_limit", func(t *testing.T) {
		// Skip: This test requires full coordinator setup since SpawnForJob
		// needs a coordinator address to actually spawn subprocesses
		t.Skip("requires full coordinator setup to test concurrent spawn limits")
	})
}

// TestFFmpegDSpawnerStopAll tests stopping all spawned processes.
// Requires full coordinator setup since SpawnForJob needs a coordinator address.
func TestFFmpegDSpawnerStopAll(t *testing.T) {
	t.Skip("requires full coordinator setup to test StopAll functionality")
}

// findFFmpegDBinary searches for the tvarr-ffmpegd binary.
func findFFmpegDBinary(t *testing.T) string {
	t.Helper()

	// Check common locations
	locations := []string{
		"./tvarr-ffmpegd",
		"./dist/debug/tvarr-ffmpegd",
		"./dist/release/tvarr-ffmpegd",
		"../../cmd/tvarr-ffmpegd/tvarr-ffmpegd",
	}

	// Try to find in PATH first
	if path, err := exec.LookPath("tvarr-ffmpegd"); err == nil {
		return path
	}

	// Check explicit locations
	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc
		}
	}

	return ""
}
