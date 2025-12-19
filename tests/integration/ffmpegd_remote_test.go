package integration

import (
	"context"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/jmylchreest/tvarr/internal/relay"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
)

// TestRemoteDaemonRegistration tests daemon registration flow over network.
// This is T057 from the task list.
func TestRemoteDaemonRegistration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create the coordinator's daemon registry
	registry := relay.NewDaemonRegistry(logger)
	registry.Start(ctx)
	defer registry.Stop()

	// Create a gRPC server that simulates the coordinator
	coordServer := newTestCoordinatorServer(registry, logger)

	// Start listening on a random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "should start listener")
	defer listener.Close()

	grpcServer := grpc.NewServer()
	proto.RegisterFFmpegDaemonServer(grpcServer, coordServer)

	go func() {
		if err := grpcServer.Serve(listener); err != nil && err != grpc.ErrServerStopped {
			t.Logf("gRPC server error: %v", err)
		}
	}()
	defer grpcServer.GracefulStop()

	t.Logf("Coordinator listening on %s", listener.Addr().String())

	t.Run("daemon_registers_successfully", func(t *testing.T) {
		// Connect to coordinator as a daemon
		conn, err := grpc.NewClient(
			listener.Addr().String(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		require.NoError(t, err)
		defer conn.Close()

		client := proto.NewFFmpegDaemonClient(conn)

		// Register daemon
		regReq := &proto.RegisterRequest{
			DaemonId:   "test-daemon-1",
			DaemonName: "Test Daemon 1",
			Version:    "1.0.0",
			Capabilities: &proto.Capabilities{
				VideoEncoders:     []string{"libx264", "libx265", "h264_nvenc"},
				VideoDecoders:     []string{"h264", "hevc"},
				AudioEncoders:     []string{"aac", "libopus"},
				AudioDecoders:     []string{"aac", "ac3"},
				MaxConcurrentJobs: 4,
				HwAccels: []*proto.HWAccelInfo{
					{
						Type:      "nvenc",
						Device:    "/dev/nvidia0",
						Available: true,
						Encoders:  []string{"h264_nvenc", "hevc_nvenc"},
						Decoders:  []string{"h264_cuvid", "hevc_cuvid"},
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
		}

		resp, err := client.Register(ctx, regReq)
		require.NoError(t, err, "should register successfully")
		assert.True(t, resp.Success, "registration should succeed")
		assert.NotNil(t, resp.HeartbeatInterval, "should return heartbeat interval")

		// Verify daemon is in registry
		assert.Equal(t, 1, registry.Count(), "should have 1 daemon registered")

		daemon, ok := registry.Get(types.DaemonID("test-daemon-1"))
		require.True(t, ok, "daemon should be in registry")
		assert.Equal(t, "Test Daemon 1", daemon.Name)
		assert.Equal(t, types.DaemonStateConnected, daemon.State)
		assert.True(t, daemon.Capabilities.HasEncoder("h264_nvenc"), "should have nvenc encoder")
		assert.Len(t, daemon.Capabilities.GPUs, 1, "should have 1 GPU")
	})

	t.Run("daemon_sends_heartbeat", func(t *testing.T) {
		conn, err := grpc.NewClient(
			listener.Addr().String(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		require.NoError(t, err)
		defer conn.Close()

		client := proto.NewFFmpegDaemonClient(conn)

		// First register
		regReq := &proto.RegisterRequest{
			DaemonId:   "test-daemon-2",
			DaemonName: "Test Daemon 2",
			Version:    "1.0.0",
			Capabilities: &proto.Capabilities{
				VideoEncoders:     []string{"libx264"},
				MaxConcurrentJobs: 2,
			},
		}
		_, err = client.Register(ctx, regReq)
		require.NoError(t, err)

		// Send heartbeat with system stats
		hbReq := &proto.HeartbeatRequest{
			DaemonId: "test-daemon-2",
			SystemStats: &proto.SystemStats{
				Hostname:             "test-host",
				Os:                   "linux",
				Arch:                 "amd64",
				CpuCores:             8,
				CpuPercent:           45.5,
				MemoryTotalBytes:     16000000000,
				MemoryUsedBytes:      8000000000,
				MemoryAvailableBytes: 8000000000,
				MemoryPercent:        50.0,
			},
			ActiveJobs: []*proto.JobStatus{
				{
					JobId:       "job-1",
					ChannelName: "Test Channel",
				},
			},
		}

		hbResp, err := client.Heartbeat(ctx, hbReq)
		require.NoError(t, err, "heartbeat should succeed")
		assert.True(t, hbResp.Success, "heartbeat should be acknowledged")

		// Verify daemon state updated
		daemon, ok := registry.Get(types.DaemonID("test-daemon-2"))
		require.True(t, ok)
		assert.Equal(t, 1, daemon.ActiveJobs, "should track active jobs")
		assert.NotNil(t, daemon.SystemStats, "should have system stats")
		assert.InDelta(t, 45.5, daemon.SystemStats.CPUPercent, 0.1)
	})

	t.Run("heartbeat_fails_for_unregistered_daemon", func(t *testing.T) {
		conn, err := grpc.NewClient(
			listener.Addr().String(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		require.NoError(t, err)
		defer conn.Close()

		client := proto.NewFFmpegDaemonClient(conn)

		hbReq := &proto.HeartbeatRequest{
			DaemonId: "nonexistent-daemon",
		}

		_, err = client.Heartbeat(ctx, hbReq)
		assert.Error(t, err, "heartbeat should fail for unregistered daemon")
	})

	t.Run("daemon_re_registration_updates_state", func(t *testing.T) {
		conn, err := grpc.NewClient(
			listener.Addr().String(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		require.NoError(t, err)
		defer conn.Close()

		client := proto.NewFFmpegDaemonClient(conn)

		// Initial registration
		regReq := &proto.RegisterRequest{
			DaemonId:   "test-daemon-3",
			DaemonName: "Test Daemon 3 - Original",
			Version:    "1.0.0",
			Capabilities: &proto.Capabilities{
				VideoEncoders:     []string{"libx264"},
				MaxConcurrentJobs: 2,
			},
		}
		_, err = client.Register(ctx, regReq)
		require.NoError(t, err)

		// Re-register with updated info
		regReq.DaemonName = "Test Daemon 3 - Updated"
		regReq.Version = "1.0.1"
		regReq.Capabilities.VideoEncoders = []string{"libx264", "libx265"}
		regReq.Capabilities.MaxConcurrentJobs = 4

		_, err = client.Register(ctx, regReq)
		require.NoError(t, err)

		// Verify updates applied
		daemon, ok := registry.Get(types.DaemonID("test-daemon-3"))
		require.True(t, ok)
		assert.Equal(t, "Test Daemon 3 - Updated", daemon.Name)
		assert.Equal(t, "1.0.1", daemon.Version)
		assert.Equal(t, 4, daemon.Capabilities.MaxConcurrentJobs)
		assert.Contains(t, daemon.Capabilities.VideoEncoders, "libx265")
	})

	t.Run("multiple_daemons_can_register", func(t *testing.T) {
		initialCount := registry.Count()

		conn, err := grpc.NewClient(
			listener.Addr().String(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		require.NoError(t, err)
		defer conn.Close()

		client := proto.NewFFmpegDaemonClient(conn)

		// Register multiple daemons
		daemons := []struct {
			id       string
			name     string
			encoders []string
		}{
			{"multi-daemon-a", "Multi Daemon A", []string{"libx264"}},
			{"multi-daemon-b", "Multi Daemon B", []string{"libx264", "h264_nvenc"}},
			{"multi-daemon-c", "Multi Daemon C", []string{"libx264", "h264_vaapi"}},
		}

		for _, d := range daemons {
			regReq := &proto.RegisterRequest{
				DaemonId:   d.id,
				DaemonName: d.name,
				Version:    "1.0.0",
				Capabilities: &proto.Capabilities{
					VideoEncoders:     d.encoders,
					MaxConcurrentJobs: 2,
				},
			}
			_, err := client.Register(ctx, regReq)
			require.NoError(t, err, "should register %s", d.name)
		}

		assert.Equal(t, initialCount+3, registry.Count(), "should have all daemons registered")

		// Verify each daemon is accessible
		for _, d := range daemons {
			daemon, ok := registry.Get(types.DaemonID(d.id))
			require.True(t, ok, "daemon %s should be in registry", d.id)
			assert.Equal(t, d.name, daemon.Name)
		}
	})
}

// TestRemoteDaemonHealthTracking tests daemon health monitoring.
func TestRemoteDaemonHealthTracking(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create registry with short timeout for testing
	registry := relay.NewDaemonRegistry(logger).WithHeartbeatTimeout(2 * time.Second)
	registry.Start(ctx)
	defer registry.Stop()

	coordServer := newTestCoordinatorServer(registry, logger)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	grpcServer := grpc.NewServer()
	proto.RegisterFFmpegDaemonServer(grpcServer, coordServer)

	go func() {
		_ = grpcServer.Serve(listener)
	}()
	defer grpcServer.GracefulStop()

	t.Run("daemon_marked_unhealthy_after_missed_heartbeats", func(t *testing.T) {
		conn, err := grpc.NewClient(
			listener.Addr().String(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		require.NoError(t, err)
		defer conn.Close()

		client := proto.NewFFmpegDaemonClient(conn)

		// Register daemon
		regReq := &proto.RegisterRequest{
			DaemonId:   "health-test-daemon",
			DaemonName: "Health Test Daemon",
			Version:    "1.0.0",
			Capabilities: &proto.Capabilities{
				VideoEncoders:     []string{"libx264"},
				MaxConcurrentJobs: 2,
			},
		}
		_, err = client.Register(ctx, regReq)
		require.NoError(t, err)

		// Verify initially active
		daemon, _ := registry.Get(types.DaemonID("health-test-daemon"))
		assert.Equal(t, types.DaemonStateConnected, daemon.State)

		// Wait for health check to run (timeout + cleanup interval margin)
		time.Sleep(3 * time.Second)

		// Verify marked unhealthy
		daemon, _ = registry.Get(types.DaemonID("health-test-daemon"))
		assert.Equal(t, types.DaemonStateUnhealthy, daemon.State, "daemon should be marked unhealthy")

		// Send heartbeat to recover
		hbReq := &proto.HeartbeatRequest{
			DaemonId: "health-test-daemon",
		}
		_, err = client.Heartbeat(ctx, hbReq)
		require.NoError(t, err)

		// Verify recovered
		daemon, _ = registry.Get(types.DaemonID("health-test-daemon"))
		assert.Equal(t, types.DaemonStateConnected, daemon.State, "daemon should recover after heartbeat")
	})
}

// testCoordinatorServer implements the gRPC server for testing.
type testCoordinatorServer struct {
	proto.UnimplementedFFmpegDaemonServer
	registry *relay.DaemonRegistry
	logger   *slog.Logger
}

func newTestCoordinatorServer(registry *relay.DaemonRegistry, logger *slog.Logger) *testCoordinatorServer {
	return &testCoordinatorServer{
		registry: registry,
		logger:   logger,
	}
}

func (s *testCoordinatorServer) Register(ctx context.Context, req *proto.RegisterRequest) (*proto.RegisterResponse, error) {
	_, err := s.registry.Register(req)
	if err != nil {
		return &proto.RegisterResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &proto.RegisterResponse{
		Success:           true,
		HeartbeatInterval: durationpb.New(5 * time.Second),
	}, nil
}

func (s *testCoordinatorServer) Heartbeat(ctx context.Context, req *proto.HeartbeatRequest) (*proto.HeartbeatResponse, error) {
	_, err := s.registry.HandleHeartbeat(req)
	if err != nil {
		return nil, err
	}

	return &proto.HeartbeatResponse{
		Success: true,
	}, nil
}

func (s *testCoordinatorServer) GetStats(ctx context.Context, req *proto.GetStatsRequest) (*proto.GetStatsResponse, error) {
	return &proto.GetStatsResponse{}, nil
}

func (s *testCoordinatorServer) Transcode(stream proto.FFmpegDaemon_TranscodeServer) error {
	return nil
}
