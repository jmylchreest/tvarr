package integration

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmylchreest/tvarr/internal/relay"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
)

// TestCapabilityBasedRouting tests that the daemon registry correctly routes jobs
// based on daemon capabilities.
// This is T058 from the task list.
func TestCapabilityBasedRouting(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := relay.NewDaemonRegistry(logger)

	// Register daemons with different capabilities
	t.Run("setup_daemons", func(t *testing.T) {
		// Daemon 1: Software-only (CPU)
		_, err := registry.Register(&proto.RegisterRequest{
			DaemonId:   "daemon-cpu-only",
			DaemonName: "CPU-Only Daemon",
			Version:    "1.0.0",
			Capabilities: &proto.Capabilities{
				VideoEncoders:     []string{"libx264", "libx265", "libvpx-vp9"},
				VideoDecoders:     []string{"h264", "hevc", "vp9"},
				AudioEncoders:     []string{"aac", "libopus"},
				AudioDecoders:     []string{"aac", "ac3", "opus"},
				MaxConcurrentJobs: 4,
			},
		})
		require.NoError(t, err)

		// Daemon 2: NVIDIA GPU (NVENC)
		_, err = registry.Register(&proto.RegisterRequest{
			DaemonId:   "daemon-nvenc",
			DaemonName: "NVENC Daemon",
			Version:    "1.0.0",
			Capabilities: &proto.Capabilities{
				VideoEncoders:     []string{"libx264", "h264_nvenc", "hevc_nvenc"},
				VideoDecoders:     []string{"h264", "hevc", "h264_cuvid", "hevc_cuvid"},
				AudioEncoders:     []string{"aac"},
				AudioDecoders:     []string{"aac", "ac3"},
				MaxConcurrentJobs: 8,
				HwAccels: []*proto.HWAccelInfo{
					{
						Type:       "nvenc",
						Device:     "/dev/nvidia0",
						Available:  true,
						HwEncoders: []string{"h264_nvenc", "hevc_nvenc"},
						HwDecoders: []string{"h264_cuvid", "hevc_cuvid"},
					},
				},
				Gpus: []*proto.GPUInfo{
					{
						Index:             0,
						Name:              "NVIDIA GeForce RTX 3080",
						GpuClass:          proto.GPUClass_GPU_CLASS_CONSUMER,
						DriverVersion:     "535.104.05",
						MaxEncodeSessions: 3,
						MaxDecodeSessions: 5,
						MemoryTotalBytes:  10737418240, // 10GB
					},
				},
			},
		})
		require.NoError(t, err)

		// Daemon 3: Intel QuickSync (VAAPI)
		_, err = registry.Register(&proto.RegisterRequest{
			DaemonId:   "daemon-vaapi",
			DaemonName: "VAAPI Daemon",
			Version:    "1.0.0",
			Capabilities: &proto.Capabilities{
				VideoEncoders:     []string{"libx264", "h264_vaapi", "hevc_vaapi"},
				VideoDecoders:     []string{"h264", "hevc"},
				AudioEncoders:     []string{"aac"},
				AudioDecoders:     []string{"aac"},
				MaxConcurrentJobs: 6,
				HwAccels: []*proto.HWAccelInfo{
					{
						Type:       "vaapi",
						Device:     "/dev/dri/renderD128",
						Available:  true,
						HwEncoders: []string{"h264_vaapi", "hevc_vaapi"},
						HwDecoders: []string{},
					},
				},
				Gpus: []*proto.GPUInfo{
					{
						Index:             0,
						Name:              "Intel UHD Graphics 770",
						GpuClass:          proto.GPUClass_GPU_CLASS_INTEGRATED,
						MaxEncodeSessions: 0, // Unlimited for Intel iGPU
						MaxDecodeSessions: 0,
					},
				},
			},
		})
		require.NoError(t, err)

		assert.Equal(t, 3, registry.Count(), "should have 3 daemons")
	})

	t.Run("select_daemon_for_nvenc_encoder", func(t *testing.T) {
		// Should select the NVENC daemon for h264_nvenc
		daemon := registry.SelectForEncoder("h264_nvenc")
		require.NotNil(t, daemon, "should find a daemon for h264_nvenc")
		assert.Equal(t, "daemon-nvenc", string(daemon.ID), "should select NVENC daemon")
	})

	t.Run("select_daemon_for_vaapi_encoder", func(t *testing.T) {
		// Should select the VAAPI daemon for h264_vaapi
		daemon := registry.SelectForEncoder("h264_vaapi")
		require.NotNil(t, daemon, "should find a daemon for h264_vaapi")
		assert.Equal(t, "daemon-vaapi", string(daemon.ID), "should select VAAPI daemon")
	})

	t.Run("select_daemon_for_libx264_returns_least_loaded", func(t *testing.T) {
		// All daemons have libx264, should pick least loaded
		// Since all are at 0 jobs, order is non-deterministic
		// but it should return a valid daemon
		daemon := registry.SelectForEncoder("libx264")
		require.NotNil(t, daemon, "should find a daemon for libx264")
		assert.True(t, daemon.Capabilities.HasEncoder("libx264"), "selected daemon should have libx264")
	})

	t.Run("no_daemon_for_unsupported_encoder", func(t *testing.T) {
		// No daemon has av1_nvenc
		daemon := registry.SelectForEncoder("av1_nvenc")
		assert.Nil(t, daemon, "should not find daemon for unsupported encoder")
	})

	t.Run("get_daemons_with_capability", func(t *testing.T) {
		// Get all daemons that support hevc_nvenc
		daemons := registry.GetWithCapability("hevc_nvenc")
		assert.Len(t, daemons, 1, "should have 1 daemon with hevc_nvenc")
		assert.Equal(t, "daemon-nvenc", string(daemons[0].ID))

		// Get all daemons that support libx265
		daemons = registry.GetWithCapability("libx265")
		assert.Len(t, daemons, 1, "should have 1 daemon with libx265")
		assert.Equal(t, "daemon-cpu-only", string(daemons[0].ID))
	})

	t.Run("get_daemons_with_available_gpu", func(t *testing.T) {
		// Initially all GPU daemons should have available sessions
		daemons := registry.GetWithAvailableGPU()
		// NVENC daemon has 3 max sessions, VAAPI has unlimited
		assert.GreaterOrEqual(t, len(daemons), 1, "should have at least 1 daemon with available GPU")
	})

	t.Run("load_balancing_prefers_least_loaded", func(t *testing.T) {
		// Verify NVENC daemon exists before simulating load
		_, ok := registry.Get(types.DaemonID("daemon-nvenc"))
		require.True(t, ok)

		// Simulate adding jobs via heartbeat
		registry.HandleHeartbeat(&proto.HeartbeatRequest{
			DaemonId: "daemon-nvenc",
			ActiveJobs: []*proto.JobStatus{
				{JobId: "job-1"},
				{JobId: "job-2"},
				{JobId: "job-3"},
			},
		})

		// Now NVENC daemon has 3 jobs, others have 0
		// SelectForEncoder should prefer less loaded daemon if encoder available
		daemon := registry.SelectForEncoder("libx264")
		require.NotNil(t, daemon)
		// Should NOT be NVENC daemon since it's most loaded
		if daemon.Capabilities.MaxConcurrentJobs > 0 {
			if daemon.ID == types.DaemonID("daemon-nvenc") {
				// If selected nvenc, others must be at same or higher load
				t.Log("Selected NVENC daemon despite higher load - checking if it's the only option")
			}
		}

		// SelectLeastLoaded should definitely not select NVENC
		daemon = registry.SelectLeastLoaded()
		require.NotNil(t, daemon)
		assert.NotEqual(t, "daemon-nvenc", string(daemon.ID), "should not select most loaded daemon")
	})

	t.Run("unavailable_daemon_not_selected", func(t *testing.T) {
		// Mark CPU daemon as unhealthy
		cpuDaemon, ok := registry.Get(types.DaemonID("daemon-cpu-only"))
		require.True(t, ok)
		cpuDaemon.State = types.DaemonStateUnhealthy

		// SelectForEncoder should not return unhealthy daemon
		daemon := registry.SelectForEncoder("libx265") // Only CPU daemon has libx265
		assert.Nil(t, daemon, "should not select unhealthy daemon")

		// Restore state
		cpuDaemon.State = types.DaemonStateConnected
	})

	t.Run("draining_daemon_not_selected_for_new_jobs", func(t *testing.T) {
		// Mark VAAPI daemon as draining
		vaapiDaemon, ok := registry.Get(types.DaemonID("daemon-vaapi"))
		require.True(t, ok)
		vaapiDaemon.State = types.DaemonStateDraining

		// SelectForEncoder should not return draining daemon
		daemon := registry.SelectForEncoder("h264_vaapi") // Only VAAPI daemon has h264_vaapi
		assert.Nil(t, daemon, "should not select draining daemon for new jobs")

		// Restore state
		vaapiDaemon.State = types.DaemonStateConnected
	})
}

// TestDaemonCapabilityMatching tests the HasEncoder method.
func TestDaemonCapabilityMatching(t *testing.T) {
	caps := &types.Capabilities{
		VideoEncoders: []string{"libx264", "h264_nvenc", "hevc_nvenc"},
		AudioEncoders: []string{"aac", "libopus"},
	}

	t.Run("has_video_encoder", func(t *testing.T) {
		assert.True(t, caps.HasEncoder("libx264"))
		assert.True(t, caps.HasEncoder("h264_nvenc"))
		assert.True(t, caps.HasEncoder("hevc_nvenc"))
		assert.False(t, caps.HasEncoder("libx265"))
		assert.False(t, caps.HasEncoder("av1_nvenc"))
	})

	t.Run("has_audio_encoder", func(t *testing.T) {
		assert.True(t, caps.HasEncoder("aac"))
		assert.True(t, caps.HasEncoder("libopus"))
		assert.False(t, caps.HasEncoder("libmp3lame"))
	})

	t.Run("case_sensitive", func(t *testing.T) {
		assert.False(t, caps.HasEncoder("LIBX264"))
		assert.False(t, caps.HasEncoder("H264_NVENC"))
	})
}

// TestGPUSessionTracking tests GPU session availability tracking.
func TestGPUSessionTracking(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := relay.NewDaemonRegistry(logger)

	// Register daemon with GPU
	_, err := registry.Register(&proto.RegisterRequest{
		DaemonId:   "gpu-daemon",
		DaemonName: "GPU Daemon",
		Version:    "1.0.0",
		Capabilities: &proto.Capabilities{
			VideoEncoders:     []string{"h264_nvenc"},
			MaxConcurrentJobs: 10,
			Gpus: []*proto.GPUInfo{
				{
					Index:             0,
					Name:              "NVIDIA GeForce RTX 3060",
					GpuClass:          proto.GPUClass_GPU_CLASS_CONSUMER,
					MaxEncodeSessions: 3, // Consumer limit
					MaxDecodeSessions: 5,
				},
			},
		},
	})
	require.NoError(t, err)

	t.Run("initial_gpu_sessions_available", func(t *testing.T) {
		daemon, ok := registry.Get(types.DaemonID("gpu-daemon"))
		require.True(t, ok)

		assert.True(t, daemon.HasAvailableGPUSessions(), "should have available GPU sessions initially")
	})

	t.Run("gpu_sessions_tracked_via_heartbeat", func(t *testing.T) {
		// Send heartbeat with GPU usage
		_, err := registry.HandleHeartbeat(&proto.HeartbeatRequest{
			DaemonId: "gpu-daemon",
			SystemStats: &proto.SystemStats{
				Gpus: []*proto.GPUStats{
					{
						Index:                0,
						Name:                 "NVIDIA GeForce RTX 3060",
						MaxEncodeSessions:    3,
						ActiveEncodeSessions: 2, // 2 of 3 sessions used
					},
				},
			},
		})
		require.NoError(t, err)

		daemon, _ := registry.Get(types.DaemonID("gpu-daemon"))

		// Should still have available sessions (1 remaining)
		assert.True(t, daemon.HasAvailableGPUSessions(), "should have 1 session remaining")
		assert.Equal(t, 2, daemon.Capabilities.GPUs[0].ActiveEncodeSessions)
	})

	t.Run("gpu_sessions_exhausted", func(t *testing.T) {
		// Send heartbeat with all sessions used
		_, err := registry.HandleHeartbeat(&proto.HeartbeatRequest{
			DaemonId: "gpu-daemon",
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

		daemon, _ := registry.Get(types.DaemonID("gpu-daemon"))
		assert.False(t, daemon.HasAvailableGPUSessions(), "should have no sessions remaining")
	})

	t.Run("registry_filters_by_gpu_availability", func(t *testing.T) {
		// With sessions exhausted, GetWithAvailableGPU should exclude this daemon
		daemons := registry.GetWithAvailableGPU()
		for _, d := range daemons {
			assert.NotEqual(t, "gpu-daemon", string(d.ID), "exhausted GPU daemon should not be returned")
		}
	})

	t.Run("gpu_sessions_freed", func(t *testing.T) {
		// Send heartbeat with sessions freed
		_, err := registry.HandleHeartbeat(&proto.HeartbeatRequest{
			DaemonId: "gpu-daemon",
			SystemStats: &proto.SystemStats{
				Gpus: []*proto.GPUStats{
					{
						Index:                0,
						MaxEncodeSessions:    3,
						ActiveEncodeSessions: 1, // Sessions freed
					},
				},
			},
		})
		require.NoError(t, err)

		daemon, _ := registry.Get(types.DaemonID("gpu-daemon"))
		assert.True(t, daemon.HasAvailableGPUSessions(), "should have sessions available again")
	})
}

// TestDaemonStateTransitions tests daemon state lifecycle.
func TestDaemonStateTransitions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := relay.NewDaemonRegistry(logger).WithHeartbeatTimeout(100 * time.Millisecond)

	// Register daemon
	_, err := registry.Register(&proto.RegisterRequest{
		DaemonId:   "state-test-daemon",
		DaemonName: "State Test Daemon",
		Version:    "1.0.0",
		Capabilities: &proto.Capabilities{
			VideoEncoders:     []string{"libx264"},
			MaxConcurrentJobs: 2,
		},
	})
	require.NoError(t, err)

	t.Run("initial_state_is_active", func(t *testing.T) {
		daemon, ok := registry.Get(types.DaemonID("state-test-daemon"))
		require.True(t, ok)
		assert.Equal(t, types.DaemonStateConnected, daemon.State)
		assert.True(t, daemon.CanAcceptJobs())
	})

	t.Run("draining_state_blocks_new_jobs", func(t *testing.T) {
		daemon, _ := registry.Get(types.DaemonID("state-test-daemon"))
		daemon.State = types.DaemonStateDraining

		assert.False(t, daemon.CanAcceptJobs(), "draining daemon should not accept new jobs")

		// Restore
		daemon.State = types.DaemonStateConnected
	})

	t.Run("unhealthy_state_blocks_new_jobs", func(t *testing.T) {
		daemon, _ := registry.Get(types.DaemonID("state-test-daemon"))
		daemon.State = types.DaemonStateUnhealthy

		assert.False(t, daemon.CanAcceptJobs(), "unhealthy daemon should not accept new jobs")

		// Restore
		daemon.State = types.DaemonStateConnected
	})

	t.Run("disconnected_state_blocks_new_jobs", func(t *testing.T) {
		daemon, _ := registry.Get(types.DaemonID("state-test-daemon"))
		daemon.State = types.DaemonStateDisconnected

		assert.False(t, daemon.CanAcceptJobs(), "disconnected daemon should not accept new jobs")

		// Restore
		daemon.State = types.DaemonStateConnected
	})
}
