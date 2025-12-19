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
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
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

	t.Run("spawn_subprocess_for_job", func(t *testing.T) {
		jobID := "integration-test-job-1"

		// Spawn subprocess
		client, cleanup, err := spawner.SpawnForJob(ctx, jobID)
		require.NoError(t, err, "should spawn subprocess successfully")
		require.NotNil(t, client, "should return gRPC client")
		require.NotNil(t, cleanup, "should return cleanup function")
		defer cleanup()

		// Verify subprocess is running by calling GetStats
		statsResp, err := client.GetStats(ctx, &proto.GetStatsRequest{})
		require.NoError(t, err, "should be able to call GetStats on subprocess")
		require.NotNil(t, statsResp, "should get stats response")

		// Verify active spawns tracking
		assert.Equal(t, 1, spawner.ActiveSpawns(), "should have 1 active spawn")
		assert.Contains(t, spawner.GetActiveJobs(), jobID, "should track job ID")

		t.Logf("Subprocess stats: system=%v, capabilities=%v", statsResp.SystemStats, statsResp.Capabilities)
	})

	t.Run("cleanup_terminates_subprocess", func(t *testing.T) {
		jobID := "integration-test-job-2"

		// Spawn subprocess
		client, cleanup, err := spawner.SpawnForJob(ctx, jobID)
		require.NoError(t, err)
		require.NotNil(t, client)

		// Verify it's running
		assert.Equal(t, 1, spawner.ActiveSpawns())

		// Call cleanup
		cleanup()

		// Verify subprocess is terminated
		assert.Equal(t, 0, spawner.ActiveSpawns(), "should have 0 active spawns after cleanup")
		assert.NotContains(t, spawner.GetActiveJobs(), jobID, "should not track job ID after cleanup")

		// Client calls should fail after cleanup (connection closed)
		_, err = client.GetStats(ctx, &proto.GetStatsRequest{})
		assert.Error(t, err, "client calls should fail after cleanup")
	})

	t.Run("multiple_concurrent_spawns", func(t *testing.T) {
		jobIDs := []string{"concurrent-job-1", "concurrent-job-2", "concurrent-job-3"}
		var clients []proto.FFmpegDaemonClient
		var cleanups []func()

		// Spawn multiple subprocesses
		for _, jobID := range jobIDs {
			client, cleanup, err := spawner.SpawnForJob(ctx, jobID)
			require.NoError(t, err, "should spawn subprocess for %s", jobID)
			clients = append(clients, client)
			cleanups = append(cleanups, cleanup)
		}

		// Verify all are running
		assert.Equal(t, 3, spawner.ActiveSpawns(), "should have 3 active spawns")

		// All clients should work independently
		for i, client := range clients {
			statsResp, err := client.GetStats(ctx, &proto.GetStatsRequest{})
			require.NoError(t, err, "client %d should respond", i)
			require.NotNil(t, statsResp)
		}

		// Cleanup all
		for _, cleanup := range cleanups {
			cleanup()
		}

		assert.Equal(t, 0, spawner.ActiveSpawns(), "should have 0 active spawns after cleanup")
	})

	t.Run("cleanup_is_idempotent", func(t *testing.T) {
		jobID := "idempotent-cleanup-job"

		_, cleanup, err := spawner.SpawnForJob(ctx, jobID)
		require.NoError(t, err)

		// Call cleanup multiple times - should not panic or error
		cleanup()
		cleanup()
		cleanup()

		assert.Equal(t, 0, spawner.ActiveSpawns())
	})
}

// TestFFmpegDSubprocessTranscoding tests actual transcoding through subprocess.
// This requires both tvarr-ffmpegd and ffmpeg to be available.
func TestFFmpegDSubprocessTranscoding(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check if tvarr-ffmpegd binary is available
	binaryPath := findFFmpegDBinary(t)
	if binaryPath == "" {
		t.Skip("tvarr-ffmpegd binary not found, skipping transcoding test")
	}

	// Check if ffmpeg is available
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found in PATH, skipping transcoding test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create spawner
	spawner := relay.NewFFmpegDSpawner(relay.FFmpegDSpawnerConfig{
		BinaryPath:      binaryPath,
		StartupTimeout:  15 * time.Second,
		ShutdownTimeout: 5 * time.Second,
		Logger:          logger,
	})

	t.Run("transcode_stream_lifecycle", func(t *testing.T) {
		jobID := "transcode-lifecycle-test"

		// Spawn subprocess
		client, cleanup, err := spawner.SpawnForJob(ctx, jobID)
		require.NoError(t, err)
		defer cleanup()

		// Create transcode stream
		stream, err := client.Transcode(ctx)
		require.NoError(t, err, "should create transcode stream")

		// Send transcode start message
		startMsg := &proto.TranscodeMessage{
			Payload: &proto.TranscodeMessage_Start{
				Start: &proto.TranscodeStart{
					JobId:            jobID,
					ChannelName:      "Test Channel",
					SourceVideoCodec: "h264",
					SourceAudioCodec: "aac",
					TargetVideoCodec: "h264",
					TargetAudioCodec: "aac",
					VideoEncoder:     "libx264",
					AudioEncoder:     "aac",
					VideoPreset:      "ultrafast",
				},
			},
		}

		err = stream.Send(startMsg)
		require.NoError(t, err, "should send start message")

		// Wait for ack
		ackMsg, err := stream.Recv()
		require.NoError(t, err, "should receive ack message")

		ack := ackMsg.GetAck()
		require.NotNil(t, ack, "response should be an ack")
		assert.True(t, ack.Success, "ack should indicate success: %s", ack.Error)

		t.Logf("Transcode ack: video_encoder=%s, audio_encoder=%s",
			ack.ActualVideoEncoder, ack.ActualAudioEncoder)

		// Send stop message
		stopMsg := &proto.TranscodeMessage{
			Payload: &proto.TranscodeMessage_Stop{
				Stop: &proto.TranscodeStop{
					Reason: "test complete",
				},
			},
		}
		err = stream.Send(stopMsg)
		require.NoError(t, err, "should send stop message")

		// Close send direction
		err = stream.CloseSend()
		require.NoError(t, err, "should close send")
	})
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

		client, cleanup, err := spawner.SpawnForJob(ctx, "test-job")
		assert.Error(t, err, "should error with invalid binary")
		assert.Nil(t, client)
		assert.Nil(t, cleanup)
		assert.ErrorIs(t, err, relay.ErrFFmpegDBinaryNotFound)
	})

	t.Run("max_concurrent_spawns_limit", func(t *testing.T) {
		binaryPath := findFFmpegDBinary(t)
		if binaryPath == "" {
			t.Skip("tvarr-ffmpegd binary not found")
		}

		spawner := relay.NewFFmpegDSpawner(relay.FFmpegDSpawnerConfig{
			BinaryPath:          binaryPath,
			MaxConcurrentSpawns: 2,
			StartupTimeout:      15 * time.Second,
			ShutdownTimeout:     5 * time.Second,
			Logger:              logger,
		})

		var cleanups []func()

		// Spawn up to limit
		for i := 0; i < 2; i++ {
			_, cleanup, err := spawner.SpawnForJob(ctx, "limit-test-"+string(rune('a'+i)))
			require.NoError(t, err)
			cleanups = append(cleanups, cleanup)
		}

		// Third spawn should fail
		_, cleanup, err := spawner.SpawnForJob(ctx, "limit-test-c")
		assert.ErrorIs(t, err, relay.ErrMaxSpawnsReached)
		assert.Nil(t, cleanup)

		// Cleanup existing spawns
		for _, cleanup := range cleanups {
			cleanup()
		}

		// Now spawning should work again
		_, cleanup, err = spawner.SpawnForJob(ctx, "limit-test-d")
		require.NoError(t, err)
		cleanup()
	})
}

// TestFFmpegDSpawnerStopAll tests stopping all spawned processes.
func TestFFmpegDSpawnerStopAll(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	binaryPath := findFFmpegDBinary(t)
	if binaryPath == "" {
		t.Skip("tvarr-ffmpegd binary not found")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	spawner := relay.NewFFmpegDSpawner(relay.FFmpegDSpawnerConfig{
		BinaryPath:      binaryPath,
		StartupTimeout:  15 * time.Second,
		ShutdownTimeout: 5 * time.Second,
		Logger:          logger,
	})

	t.Run("stop_all_terminates_all_subprocesses", func(t *testing.T) {
		// Spawn multiple
		for i := 0; i < 3; i++ {
			_, _, err := spawner.SpawnForJob(ctx, "stop-all-test-"+string(rune('a'+i)))
			require.NoError(t, err)
		}

		assert.Equal(t, 3, spawner.ActiveSpawns())

		// Stop all
		spawner.StopAll()

		assert.Equal(t, 0, spawner.ActiveSpawns(), "should have no active spawns after StopAll")
	})
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
