package integration

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmylchreest/tvarr/internal/relay"
	"github.com/jmylchreest/tvarr/internal/service"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
)

// TestFFmpegDServiceListDaemons tests T085: API integration test for daemon list.
func TestFFmpegDServiceListDaemons(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := relay.NewDaemonRegistry(logger)

	// Create the service
	svc := service.NewFFmpegDService(registry, logger)

	t.Run("list_empty_returns_empty_slice", func(t *testing.T) {
		daemons := svc.ListDaemons()
		assert.Empty(t, daemons)
	})

	t.Run("list_returns_registered_daemons", func(t *testing.T) {
		// Register some daemons
		_, err := registry.Register(&proto.RegisterRequest{
			DaemonId:   "test-daemon-1",
			DaemonName: "Test Daemon 1",
			Version:    "1.0.0",
			Capabilities: &proto.Capabilities{
				VideoEncoders:     []string{"libx264", "h264_nvenc"},
				MaxConcurrentJobs: 4,
			},
		})
		require.NoError(t, err)

		_, err = registry.Register(&proto.RegisterRequest{
			DaemonId:   "test-daemon-2",
			DaemonName: "Test Daemon 2",
			Version:    "1.0.0",
			Capabilities: &proto.Capabilities{
				VideoEncoders:     []string{"libx264"},
				MaxConcurrentJobs: 2,
			},
		})
		require.NoError(t, err)

		daemons := svc.ListDaemons()
		assert.Len(t, daemons, 2)

		// Verify daemon data
		var names []string
		for _, d := range daemons {
			names = append(names, d.Name)
		}
		assert.Contains(t, names, "Test Daemon 1")
		assert.Contains(t, names, "Test Daemon 2")
	})
}

func TestFFmpegDServiceGetDaemon(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := relay.NewDaemonRegistry(logger)
	svc := service.NewFFmpegDService(registry, logger)

	// Register a daemon
	_, err := registry.Register(&proto.RegisterRequest{
		DaemonId:   "get-test-daemon",
		DaemonName: "Get Test Daemon",
		Version:    "1.0.0",
		Capabilities: &proto.Capabilities{
			VideoEncoders:     []string{"libx264"},
			MaxConcurrentJobs: 4,
			Gpus: []*proto.GPUInfo{
				{
					Index:             0,
					Name:              "Test GPU",
					MaxEncodeSessions: 3,
				},
			},
		},
	})
	require.NoError(t, err)

	t.Run("get_existing_daemon", func(t *testing.T) {
		daemon, err := svc.GetDaemon(types.DaemonID("get-test-daemon"))
		require.NoError(t, err)
		require.NotNil(t, daemon)
		assert.Equal(t, "Get Test Daemon", daemon.Name)
		assert.Equal(t, types.DaemonStateConnected, daemon.State)
	})

	t.Run("get_nonexistent_daemon_returns_error", func(t *testing.T) {
		daemon, err := svc.GetDaemon(types.DaemonID("nonexistent"))
		assert.Error(t, err)
		assert.Nil(t, daemon)
	})
}

func TestFFmpegDServiceGetClusterStats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := relay.NewDaemonRegistry(logger)
	svc := service.NewFFmpegDService(registry, logger)

	t.Run("empty_cluster_stats", func(t *testing.T) {
		stats := svc.GetClusterStats()
		assert.Equal(t, 0, stats.TotalDaemons)
		assert.Equal(t, 0, stats.ActiveDaemons)
	})

	t.Run("cluster_stats_with_daemons", func(t *testing.T) {
		// Register daemons with GPUs
		_, err := registry.Register(&proto.RegisterRequest{
			DaemonId:   "stats-daemon-1",
			DaemonName: "Stats Daemon 1",
			Version:    "1.0.0",
			Capabilities: &proto.Capabilities{
				VideoEncoders:     []string{"h264_nvenc"},
				MaxConcurrentJobs: 8,
				Gpus: []*proto.GPUInfo{
					{
						Index:             0,
						Name:              "RTX 3080",
						MaxEncodeSessions: 3,
					},
				},
			},
		})
		require.NoError(t, err)

		_, err = registry.Register(&proto.RegisterRequest{
			DaemonId:   "stats-daemon-2",
			DaemonName: "Stats Daemon 2",
			Version:    "1.0.0",
			Capabilities: &proto.Capabilities{
				VideoEncoders:     []string{"libx264"},
				MaxConcurrentJobs: 4,
			},
		})
		require.NoError(t, err)

		// Add heartbeat with system stats
		_, err = registry.HandleHeartbeat(&proto.HeartbeatRequest{
			DaemonId: "stats-daemon-1",
			SystemStats: &proto.SystemStats{
				CpuPercent:    45.5,
				MemoryPercent: 62.0,
				Gpus: []*proto.GPUStats{
					{
						Index:                0,
						MaxEncodeSessions:    3,
						ActiveEncodeSessions: 1,
					},
				},
			},
		})
		require.NoError(t, err)

		stats := svc.GetClusterStats()
		assert.GreaterOrEqual(t, stats.TotalDaemons, 2)
		assert.GreaterOrEqual(t, stats.ActiveDaemons, 2)
		assert.GreaterOrEqual(t, stats.TotalGPUs, 1)
	})
}

func TestFFmpegDServiceDrainDaemon(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := relay.NewDaemonRegistry(logger)
	svc := service.NewFFmpegDService(registry, logger)

	// Register a daemon
	_, err := registry.Register(&proto.RegisterRequest{
		DaemonId:   "drain-test-daemon",
		DaemonName: "Drain Test Daemon",
		Version:    "1.0.0",
		Capabilities: &proto.Capabilities{
			VideoEncoders:     []string{"libx264"},
			MaxConcurrentJobs: 4,
		},
	})
	require.NoError(t, err)

	t.Run("drain_active_daemon", func(t *testing.T) {
		err := svc.DrainDaemon(types.DaemonID("drain-test-daemon"))
		require.NoError(t, err)

		daemon, err := svc.GetDaemon(types.DaemonID("drain-test-daemon"))
		require.NoError(t, err)
		assert.Equal(t, types.DaemonStateDraining, daemon.State)
	})

	t.Run("drain_nonexistent_daemon_returns_error", func(t *testing.T) {
		err := svc.DrainDaemon(types.DaemonID("nonexistent"))
		assert.Error(t, err)
	})
}

func TestFFmpegDServiceActivateDaemon(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := relay.NewDaemonRegistry(logger)
	svc := service.NewFFmpegDService(registry, logger)

	// Register and drain a daemon
	_, err := registry.Register(&proto.RegisterRequest{
		DaemonId:   "activate-test-daemon",
		DaemonName: "Activate Test Daemon",
		Version:    "1.0.0",
		Capabilities: &proto.Capabilities{
			VideoEncoders:     []string{"libx264"},
			MaxConcurrentJobs: 4,
		},
	})
	require.NoError(t, err)

	err = svc.DrainDaemon(types.DaemonID("activate-test-daemon"))
	require.NoError(t, err)

	t.Run("activate_draining_daemon", func(t *testing.T) {
		err := svc.ActivateDaemon(types.DaemonID("activate-test-daemon"))
		require.NoError(t, err)

		daemon, err := svc.GetDaemon(types.DaemonID("activate-test-daemon"))
		require.NoError(t, err)
		assert.Equal(t, types.DaemonStateConnected, daemon.State)
	})
}

func TestFFmpegDServiceGetDaemonsByCapability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := relay.NewDaemonRegistry(logger)
	svc := service.NewFFmpegDService(registry, logger)

	// Register daemons with different capabilities
	_, err := registry.Register(&proto.RegisterRequest{
		DaemonId:   "cap-daemon-nvenc",
		DaemonName: "NVENC Daemon",
		Version:    "1.0.0",
		Capabilities: &proto.Capabilities{
			VideoEncoders:     []string{"libx264", "h264_nvenc", "hevc_nvenc"},
			MaxConcurrentJobs: 8,
		},
	})
	require.NoError(t, err)

	_, err = registry.Register(&proto.RegisterRequest{
		DaemonId:   "cap-daemon-cpu",
		DaemonName: "CPU Daemon",
		Version:    "1.0.0",
		Capabilities: &proto.Capabilities{
			VideoEncoders:     []string{"libx264", "libx265"},
			MaxConcurrentJobs: 4,
		},
	})
	require.NoError(t, err)

	t.Run("get_daemons_with_nvenc", func(t *testing.T) {
		daemons := svc.GetDaemonsByCapability("h264_nvenc")
		assert.Len(t, daemons, 1)
		assert.Equal(t, "NVENC Daemon", daemons[0].Name)
	})

	t.Run("get_daemons_with_libx264", func(t *testing.T) {
		daemons := svc.GetDaemonsByCapability("libx264")
		assert.Len(t, daemons, 2) // Both have libx264
	})

	t.Run("get_daemons_with_unavailable_encoder", func(t *testing.T) {
		daemons := svc.GetDaemonsByCapability("av1_nvenc")
		assert.Empty(t, daemons)
	})
}

func TestFFmpegDServiceFilterByState(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := relay.NewDaemonRegistry(logger).WithHeartbeatTimeout(100 * time.Millisecond)
	svc := service.NewFFmpegDService(registry, logger)

	// Register daemons
	_, err := registry.Register(&proto.RegisterRequest{
		DaemonId:   "state-daemon-1",
		DaemonName: "Active Daemon",
		Version:    "1.0.0",
		Capabilities: &proto.Capabilities{
			VideoEncoders:     []string{"libx264"},
			MaxConcurrentJobs: 4,
		},
	})
	require.NoError(t, err)

	_, err = registry.Register(&proto.RegisterRequest{
		DaemonId:   "state-daemon-2",
		DaemonName: "Draining Daemon",
		Version:    "1.0.0",
		Capabilities: &proto.Capabilities{
			VideoEncoders:     []string{"libx264"},
			MaxConcurrentJobs: 4,
		},
	})
	require.NoError(t, err)

	// Drain one daemon
	err = svc.DrainDaemon(types.DaemonID("state-daemon-2"))
	require.NoError(t, err)

	t.Run("filter_active_daemons", func(t *testing.T) {
		daemons := svc.GetDaemonsByState(types.DaemonStateConnected)
		var activeNames []string
		for _, d := range daemons {
			activeNames = append(activeNames, d.Name)
		}
		assert.Contains(t, activeNames, "Active Daemon")
	})

	t.Run("filter_draining_daemons", func(t *testing.T) {
		daemons := svc.GetDaemonsByState(types.DaemonStateDraining)
		assert.Len(t, daemons, 1)
		assert.Equal(t, "Draining Daemon", daemons[0].Name)
	})
}
