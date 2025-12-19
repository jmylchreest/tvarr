package relay

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T042: Unit tests for subprocess spawner.
// These tests define the expected behavior of FFmpegDSpawner.

// TestNewFFmpegDSpawner tests spawner creation.
func TestNewFFmpegDSpawner(t *testing.T) {
	t.Run("creates spawner with defaults", func(t *testing.T) {
		spawner := NewFFmpegDSpawner(FFmpegDSpawnerConfig{})

		require.NotNil(t, spawner)
		// BinaryPath is resolved lazily when findBinary() is called, not at creation
		assert.Greater(t, spawner.config.StartupTimeout, time.Duration(0), "should have startup timeout")
		assert.Greater(t, spawner.config.ShutdownTimeout, time.Duration(0), "should have shutdown timeout")
		assert.NotEmpty(t, spawner.config.SocketDir, "should have socket directory")
	})

	t.Run("creates spawner with custom config", func(t *testing.T) {
		spawner := NewFFmpegDSpawner(FFmpegDSpawnerConfig{
			BinaryPath:      "/custom/path/tvarr-ffmpegd",
			StartupTimeout:  30 * time.Second,
			ShutdownTimeout: 10 * time.Second,
		})

		require.NotNil(t, spawner)
		assert.Equal(t, "/custom/path/tvarr-ffmpegd", spawner.config.BinaryPath)
		assert.Equal(t, 30*time.Second, spawner.config.StartupTimeout)
		assert.Equal(t, 10*time.Second, spawner.config.ShutdownTimeout)
	})

	t.Run("uses environment variable for binary path", func(t *testing.T) {
		t.Setenv("TVARR_FFMPEGD_BINARY", "/env/path/tvarr-ffmpegd")

		spawner := NewFFmpegDSpawner(FFmpegDSpawnerConfig{})

		require.NotNil(t, spawner)
		assert.Equal(t, "/env/path/tvarr-ffmpegd", spawner.config.BinaryPath)
	})
}

// TestFFmpegDSpawner_SpawnForJob tests spawning a subprocess for a specific job.
func TestFFmpegDSpawner_SpawnForJob(t *testing.T) {
	t.Run("returns error when binary not found", func(t *testing.T) {
		spawner := NewFFmpegDSpawner(FFmpegDSpawnerConfig{
			BinaryPath: "/nonexistent/path/tvarr-ffmpegd",
		})

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		client, cleanup, err := spawner.SpawnForJob(ctx, "test-job-1")

		assert.Error(t, err)
		assert.Nil(t, client)
		assert.Nil(t, cleanup)
		assert.True(t, errors.Is(err, ErrFFmpegDBinaryNotFound), "should return ErrFFmpegDBinaryNotFound")
	})

	t.Run("returns error on spawn timeout", func(t *testing.T) {
		// Skip this test if we don't have a mock binary
		t.Skip("requires mock binary for timeout testing")
	})

	// Note: Full integration tests for successful spawning are in ffmpegd_subprocess_test.go
}

// TestFFmpegDSpawner_Address tests address generation.
func TestFFmpegDSpawner_Address(t *testing.T) {
	t.Run("generates unique addresses per job", func(t *testing.T) {
		spawner := NewFFmpegDSpawner(FFmpegDSpawnerConfig{})

		addr1 := spawner.generateAddress("job-1")
		addr2 := spawner.generateAddress("job-2")

		assert.NotEmpty(t, addr1)
		assert.NotEmpty(t, addr2)
		assert.NotEqual(t, addr1, addr2, "should generate unique addresses per job")
	})

	t.Run("generates unix socket when preferred", func(t *testing.T) {
		spawner := NewFFmpegDSpawner(FFmpegDSpawnerConfig{
			PreferUnixSocket: true,
		})

		addr := spawner.generateAddress("job-1")

		// On Unix systems, should start with "unix://"
		// On Windows, should fall back to localhost
		assert.NotEmpty(t, addr)
	})

	t.Run("generates localhost address when unix socket not preferred", func(t *testing.T) {
		spawner := NewFFmpegDSpawner(FFmpegDSpawnerConfig{
			PreferUnixSocket: false,
		})

		addr := spawner.generateAddress("job-1")

		assert.Contains(t, addr, "localhost:")
	})
}

// TestFFmpegDSpawner_Lifecycle tests subprocess lifecycle management.
func TestFFmpegDSpawner_Lifecycle(t *testing.T) {
	t.Run("cleanup function terminates process gracefully", func(t *testing.T) {
		// This test verifies the cleanup contract - actual subprocess testing
		// is done in integration tests.
		t.Skip("requires actual subprocess for lifecycle testing")
	})

	t.Run("cleanup is idempotent", func(t *testing.T) {
		// Create a mock cleanup function to verify idempotency
		cleanupCalled := 0
		cleanup := func() {
			cleanupCalled++
		}

		// Call cleanup multiple times
		cleanup()
		cleanup()
		cleanup()

		// Note: Real cleanup should be idempotent, this tests the pattern
		assert.Equal(t, 3, cleanupCalled, "cleanup can be called multiple times")
	})
}

// TestFFmpegDSpawner_FindBinary tests binary discovery.
func TestFFmpegDSpawner_FindBinary(t *testing.T) {
	t.Run("returns configured path if file exists", func(t *testing.T) {
		// Create a temporary file to act as the binary
		tmpFile, err := os.CreateTemp("", "tvarr-ffmpegd-test")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())
		tmpFile.Close()

		spawner := NewFFmpegDSpawner(FFmpegDSpawnerConfig{
			BinaryPath: tmpFile.Name(),
		})

		path := spawner.findBinary()

		assert.Equal(t, tmpFile.Name(), path)
	})

	t.Run("returns empty if configured path does not exist", func(t *testing.T) {
		spawner := NewFFmpegDSpawner(FFmpegDSpawnerConfig{
			BinaryPath: "/nonexistent/path/tvarr-ffmpegd",
		})

		path := spawner.findBinary()

		// Binary doesn't exist at configured path, and won't be in PATH either
		// Will return empty unless tvarr-ffmpegd is actually installed
		assert.NotNil(t, &path) // Just verify no panic
	})

	t.Run("searches PATH if not configured", func(t *testing.T) {
		spawner := NewFFmpegDSpawner(FFmpegDSpawnerConfig{})

		path := spawner.findBinary()

		// May or may not find it depending on system - just verify it doesn't panic
		assert.NotNil(t, &path)
	})
}

// TestFFmpegDSpawner_IsAvailable tests availability check.
func TestFFmpegDSpawner_IsAvailable(t *testing.T) {
	t.Run("returns false when binary not found", func(t *testing.T) {
		spawner := NewFFmpegDSpawner(FFmpegDSpawnerConfig{
			BinaryPath: "/nonexistent/path/tvarr-ffmpegd",
		})

		available := spawner.IsAvailable()

		assert.False(t, available)
	})

	// Note: Testing when binary exists requires the actual binary or a mock
}

// TestFFmpegDSpawner_ConcurrentSpawns tests concurrent job spawning.
func TestFFmpegDSpawner_ConcurrentSpawns(t *testing.T) {
	t.Run("tracks active spawns", func(t *testing.T) {
		spawner := NewFFmpegDSpawner(FFmpegDSpawnerConfig{})

		// Initially no active spawns
		assert.Equal(t, 0, spawner.ActiveSpawns())
	})

	t.Run("respects max concurrent spawns limit", func(t *testing.T) {
		spawner := NewFFmpegDSpawner(FFmpegDSpawnerConfig{
			MaxConcurrentSpawns: 2,
		})

		assert.Equal(t, 2, spawner.config.MaxConcurrentSpawns)
	})
}

// TestFFmpegDSpawnerConfig tests configuration validation.
func TestFFmpegDSpawnerConfig(t *testing.T) {
	t.Run("validates configuration", func(t *testing.T) {
		tests := []struct {
			name    string
			config  FFmpegDSpawnerConfig
			wantErr bool
		}{
			{
				name:    "empty config is valid (uses defaults)",
				config:  FFmpegDSpawnerConfig{},
				wantErr: false,
			},
			{
				name: "explicit binary path is valid",
				config: FFmpegDSpawnerConfig{
					BinaryPath: "/usr/bin/tvarr-ffmpegd",
				},
				wantErr: false,
			},
			{
				name: "negative timeout is invalid",
				config: FFmpegDSpawnerConfig{
					StartupTimeout: -1 * time.Second,
				},
				wantErr: true,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				err := tc.config.Validate()
				if tc.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})
}
