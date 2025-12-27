package integration

import (
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmylchreest/tvarr/internal/relay"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
)

// TestGPUSessionExhaustionHandling tests T072: Integration test for session exhaustion handling.
// This tests the complete flow of GPU session tracking, exhaustion, and recovery.
func TestGPUSessionExhaustionHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := relay.NewDaemonRegistry(logger)

	// Setup: Register multiple daemons with different GPU configurations
	t.Run("setup_gpu_daemons", func(t *testing.T) {
		// Consumer GPU daemon (limited sessions)
		_, err := registry.Register(&proto.RegisterRequest{
			DaemonId:   "consumer-gpu",
			DaemonName: "Consumer GPU Daemon",
			Version:    "1.0.0",
			Capabilities: &proto.Capabilities{
				VideoEncoders:     []string{"libx264", "h264_nvenc", "hevc_nvenc"},
				MaxConcurrentJobs: 10,
				Gpus: []*proto.GPUInfo{
					{
						Index:             0,
						Name:              "NVIDIA GeForce RTX 3080",
						GpuClass:          proto.GPUClass_GPU_CLASS_CONSUMER,
						MaxEncodeSessions: 3, // Consumer limit
						MaxDecodeSessions: 5,
						MemoryTotalBytes:  10737418240,
					},
				},
			},
		})
		require.NoError(t, err)

		// Professional GPU daemon (higher session limits)
		_, err = registry.Register(&proto.RegisterRequest{
			DaemonId:   "pro-gpu",
			DaemonName: "Professional GPU Daemon",
			Version:    "1.0.0",
			Capabilities: &proto.Capabilities{
				VideoEncoders:     []string{"libx264", "h264_nvenc", "hevc_nvenc"},
				MaxConcurrentJobs: 32,
				Gpus: []*proto.GPUInfo{
					{
						Index:             0,
						Name:              "NVIDIA RTX A5000",
						GpuClass:          proto.GPUClass_GPU_CLASS_PROFESSIONAL,
						MaxEncodeSessions: 32, // Professional limit
						MaxDecodeSessions: 64,
						MemoryTotalBytes:  25769803776,
					},
				},
			},
		})
		require.NoError(t, err)

		// CPU-only daemon (fallback)
		_, err = registry.Register(&proto.RegisterRequest{
			DaemonId:   "cpu-only",
			DaemonName: "CPU-Only Daemon",
			Version:    "1.0.0",
			Capabilities: &proto.Capabilities{
				VideoEncoders:     []string{"libx264", "libx265"},
				MaxConcurrentJobs: 4,
			},
		})
		require.NoError(t, err)

		assert.Equal(t, 3, registry.Count())
	})

	t.Run("initial_state_all_gpu_sessions_available", func(t *testing.T) {
		gpuDaemons := registry.GetWithAvailableGPU()
		assert.Len(t, gpuDaemons, 2, "should have 2 GPU daemons initially")

		// Both GPU daemons should have available sessions
		consumer, _ := registry.Get(types.DaemonID("consumer-gpu"))
		assert.True(t, consumer.HasAvailableGPUSessions())

		pro, _ := registry.Get(types.DaemonID("pro-gpu"))
		assert.True(t, pro.HasAvailableGPUSessions())
	})

	t.Run("consumer_gpu_sessions_exhaust_first", func(t *testing.T) {
		// Simulate all 3 consumer GPU sessions being used
		_, err := registry.HandleHeartbeat(&proto.HeartbeatRequest{
			DaemonId: "consumer-gpu",
			SystemStats: &proto.SystemStats{
				Gpus: []*proto.GPUStats{
					{
						Index:                0,
						MaxEncodeSessions:    3,
						ActiveEncodeSessions: 3, // All sessions used
					},
				},
			},
		})
		require.NoError(t, err)

		consumer, _ := registry.Get(types.DaemonID("consumer-gpu"))
		assert.False(t, consumer.HasAvailableGPUSessions(), "consumer should have no sessions")

		// Pro daemon should still have sessions
		pro, _ := registry.Get(types.DaemonID("pro-gpu"))
		assert.True(t, pro.HasAvailableGPUSessions(), "pro should still have sessions")
	})

	t.Run("gpu_selection_excludes_exhausted_daemon", func(t *testing.T) {
		// GetWithAvailableGPU should only return pro daemon
		gpuDaemons := registry.GetWithAvailableGPU()
		assert.Len(t, gpuDaemons, 1, "only 1 GPU daemon should have available sessions")
		assert.Equal(t, types.DaemonID("pro-gpu"), gpuDaemons[0].ID)
	})

	t.Run("strategy_gpu_aware_selects_available_daemon", func(t *testing.T) {
		strategy := relay.NewStrategyGPUAware()

		daemons := registry.GetActive()
		criteria := relay.SelectionCriteria{
			RequiredEncoder: "h264_nvenc",
			RequireGPU:      true,
		}

		selected := strategy.Select(daemons, criteria)
		require.NotNil(t, selected, "should select a daemon with available GPU")
		assert.Equal(t, types.DaemonID("pro-gpu"), selected.ID, "should select pro GPU")
	})

	t.Run("both_gpus_exhausted_returns_nil", func(t *testing.T) {
		// Exhaust pro GPU sessions too
		_, err := registry.HandleHeartbeat(&proto.HeartbeatRequest{
			DaemonId: "pro-gpu",
			SystemStats: &proto.SystemStats{
				Gpus: []*proto.GPUStats{
					{
						Index:                0,
						MaxEncodeSessions:    32,
						ActiveEncodeSessions: 32, // All sessions used
					},
				},
			},
		})
		require.NoError(t, err)

		// No GPU daemons should be available
		gpuDaemons := registry.GetWithAvailableGPU()
		assert.Len(t, gpuDaemons, 0, "no GPU daemons should have available sessions")

		// Strategy should return nil when requiring GPU
		strategy := relay.NewStrategyGPUAware()
		criteria := relay.SelectionCriteria{
			RequiredEncoder: "h264_nvenc",
			RequireGPU:      true,
		}
		selected := strategy.Select(registry.GetActive(), criteria)
		assert.Nil(t, selected, "should return nil when all GPUs exhausted and GPU required")
	})

	t.Run("fallback_to_software_encoding_available", func(t *testing.T) {
		// Without RequireGPU, should still be able to select a daemon for software encoding
		// Note: The selection will pick the least loaded daemon with the encoder,
		// which may be a GPU daemon that also has libx264
		strategy := relay.NewStrategyCapabilityMatch()
		criteria := relay.SelectionCriteria{
			RequiredEncoder: "libx264",
			RequireGPU:      false, // Allow fallback
		}

		selected := strategy.Select(registry.GetActive(), criteria)
		require.NotNil(t, selected, "should select a daemon for software encoding")
		// Verify the selected daemon has the required encoder
		assert.True(t, selected.Capabilities.HasEncoder("libx264"),
			"selected daemon should have libx264 encoder")
	})

	t.Run("session_recovery_restores_availability", func(t *testing.T) {
		// Consumer GPU frees 2 sessions
		_, err := registry.HandleHeartbeat(&proto.HeartbeatRequest{
			DaemonId: "consumer-gpu",
			SystemStats: &proto.SystemStats{
				Gpus: []*proto.GPUStats{
					{
						Index:                0,
						MaxEncodeSessions:    3,
						ActiveEncodeSessions: 1, // 2 sessions freed
					},
				},
			},
		})
		require.NoError(t, err)

		consumer, _ := registry.Get(types.DaemonID("consumer-gpu"))
		assert.True(t, consumer.HasAvailableGPUSessions(), "consumer should have sessions again")

		gpuDaemons := registry.GetWithAvailableGPU()
		assert.Len(t, gpuDaemons, 1, "consumer GPU should be available again")
	})

	t.Run("multi_gpu_session_tracking", func(t *testing.T) {
		// Register daemon with multiple GPUs
		_, err := registry.Register(&proto.RegisterRequest{
			DaemonId:   "multi-gpu",
			DaemonName: "Multi-GPU Daemon",
			Version:    "1.0.0",
			Capabilities: &proto.Capabilities{
				VideoEncoders:     []string{"h264_nvenc"},
				MaxConcurrentJobs: 16,
				Gpus: []*proto.GPUInfo{
					{
						Index:             0,
						Name:              "NVIDIA GeForce RTX 3080",
						GpuClass:          proto.GPUClass_GPU_CLASS_CONSUMER,
						MaxEncodeSessions: 3,
					},
					{
						Index:             1,
						Name:              "NVIDIA GeForce RTX 3080",
						GpuClass:          proto.GPUClass_GPU_CLASS_CONSUMER,
						MaxEncodeSessions: 3,
					},
				},
			},
		})
		require.NoError(t, err)

		multiGPU, _ := registry.Get(types.DaemonID("multi-gpu"))
		assert.True(t, multiGPU.HasAvailableGPUSessions())

		// Exhaust first GPU, second still available
		_, err = registry.HandleHeartbeat(&proto.HeartbeatRequest{
			DaemonId: "multi-gpu",
			SystemStats: &proto.SystemStats{
				Gpus: []*proto.GPUStats{
					{
						Index:                0,
						MaxEncodeSessions:    3,
						ActiveEncodeSessions: 3, // GPU 0 exhausted
					},
					{
						Index:                1,
						MaxEncodeSessions:    3,
						ActiveEncodeSessions: 1, // GPU 1 has 2 available
					},
				},
			},
		})
		require.NoError(t, err)

		multiGPU, _ = registry.Get(types.DaemonID("multi-gpu"))
		assert.True(t, multiGPU.HasAvailableGPUSessions(), "should have sessions on GPU 1")

		// Exhaust both GPUs
		_, err = registry.HandleHeartbeat(&proto.HeartbeatRequest{
			DaemonId: "multi-gpu",
			SystemStats: &proto.SystemStats{
				Gpus: []*proto.GPUStats{
					{
						Index:                0,
						MaxEncodeSessions:    3,
						ActiveEncodeSessions: 3,
					},
					{
						Index:                1,
						MaxEncodeSessions:    3,
						ActiveEncodeSessions: 3,
					},
				},
			},
		})
		require.NoError(t, err)

		multiGPU, _ = registry.Get(types.DaemonID("multi-gpu"))
		assert.False(t, multiGPU.HasAvailableGPUSessions(), "no sessions on any GPU")
	})
}

// TestGPUExhaustedPolicySelection tests that the correct fallback policy is applied
// when GPU sessions are exhausted.
func TestGPUExhaustedPolicySelection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := relay.NewDaemonRegistry(logger)

	// Setup: Register GPU daemon with all sessions used
	_, err := registry.Register(&proto.RegisterRequest{
		DaemonId:   "exhausted-gpu",
		DaemonName: "Exhausted GPU Daemon",
		Version:    "1.0.0",
		Capabilities: &proto.Capabilities{
			VideoEncoders:     []string{"h264_nvenc", "libx264"},
			MaxConcurrentJobs: 10,
			Gpus: []*proto.GPUInfo{
				{
					Index:             0,
					Name:              "NVIDIA GeForce RTX 3060",
					GpuClass:          proto.GPUClass_GPU_CLASS_CONSUMER,
					MaxEncodeSessions: 3,
				},
			},
		},
	})
	require.NoError(t, err)

	// Exhaust sessions
	_, err = registry.HandleHeartbeat(&proto.HeartbeatRequest{
		DaemonId: "exhausted-gpu",
		SystemStats: &proto.SystemStats{
			Gpus: []*proto.GPUStats{
				{Index: 0, MaxEncodeSessions: 3, ActiveEncodeSessions: 3},
			},
		},
	})
	require.NoError(t, err)

	t.Run("policy_fallback_allows_software_encoding", func(t *testing.T) {
		// With GPUPolicyFallback, should fall back to libx264
		strategy := relay.NewStrategyCapabilityMatch()

		// First try with GPU required - should fail
		criteria := relay.SelectionCriteria{
			RequiredEncoder: "h264_nvenc",
			RequireGPU:      true,
		}
		selected := strategy.Select(registry.GetActive(), criteria)
		assert.Nil(t, selected, "should fail when GPU required but exhausted")

		// Then try without GPU required - should succeed with software encoder
		criteria = relay.SelectionCriteria{
			RequiredEncoder: "libx264",
			RequireGPU:      false,
		}
		selected = strategy.Select(registry.GetActive(), criteria)
		require.NotNil(t, selected, "should succeed with software encoder")
	})

	t.Run("policy_reject_returns_nil_immediately", func(t *testing.T) {
		// With GPUPolicyReject, should immediately fail
		strategy := relay.NewStrategyGPUAware()
		criteria := relay.SelectionCriteria{
			RequiredEncoder: "h264_nvenc",
			RequireGPU:      true,
		}
		selected := strategy.Select(registry.GetActive(), criteria)
		assert.Nil(t, selected, "should reject immediately when GPU exhausted")
	})
}

// TestUnlimitedGPUSessions tests that datacenter GPUs with unlimited sessions
// are handled correctly.
func TestUnlimitedGPUSessions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := relay.NewDaemonRegistry(logger)

	// Register datacenter GPU (unlimited sessions)
	_, err := registry.Register(&proto.RegisterRequest{
		DaemonId:   "datacenter-gpu",
		DaemonName: "Datacenter GPU Daemon",
		Version:    "1.0.0",
		Capabilities: &proto.Capabilities{
			VideoEncoders:     []string{"h264_nvenc", "hevc_nvenc"},
			MaxConcurrentJobs: 100,
			Gpus: []*proto.GPUInfo{
				{
					Index:             0,
					Name:              "NVIDIA A100",
					GpuClass:          proto.GPUClass_GPU_CLASS_DATACENTER,
					MaxEncodeSessions: 0, // 0 means unlimited
					MemoryTotalBytes:  42949672960,
				},
			},
		},
	})
	require.NoError(t, err)

	t.Run("unlimited_sessions_always_available", func(t *testing.T) {
		daemon, _ := registry.Get(types.DaemonID("datacenter-gpu"))
		assert.True(t, daemon.HasAvailableGPUSessions())

		// Even with many active sessions, should still be available
		_, err := registry.HandleHeartbeat(&proto.HeartbeatRequest{
			DaemonId: "datacenter-gpu",
			SystemStats: &proto.SystemStats{
				Gpus: []*proto.GPUStats{
					{
						Index:                0,
						MaxEncodeSessions:    0, // Unlimited
						ActiveEncodeSessions: 50,
					},
				},
			},
		})
		require.NoError(t, err)

		daemon, _ = registry.Get(types.DaemonID("datacenter-gpu"))
		assert.True(t, daemon.HasAvailableGPUSessions(), "unlimited GPU should always be available")
	})

	t.Run("strategy_prefers_unlimited_gpu", func(t *testing.T) {
		// Register a consumer GPU daemon too
		_, err := registry.Register(&proto.RegisterRequest{
			DaemonId:   "consumer-gpu",
			DaemonName: "Consumer GPU Daemon",
			Version:    "1.0.0",
			Capabilities: &proto.Capabilities{
				VideoEncoders:     []string{"h264_nvenc"},
				MaxConcurrentJobs: 5,
				Gpus: []*proto.GPUInfo{
					{
						Index:             0,
						Name:              "NVIDIA GeForce RTX 3080",
						GpuClass:          proto.GPUClass_GPU_CLASS_CONSUMER,
						MaxEncodeSessions: 3,
					},
				},
			},
		})
		require.NoError(t, err)

		// GPUAware strategy should prefer the datacenter GPU
		strategy := relay.NewStrategyGPUAware()
		criteria := relay.SelectionCriteria{
			RequiredEncoder: "h264_nvenc",
		}

		selected := strategy.Select(registry.GetActive(), criteria)
		require.NotNil(t, selected)
		// Datacenter GPU should be preferred due to unlimited sessions
		assert.Equal(t, types.DaemonID("datacenter-gpu"), selected.ID)
	})
}
