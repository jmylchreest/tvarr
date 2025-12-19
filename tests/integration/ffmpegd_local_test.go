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

	"github.com/jmylchreest/tvarr/internal/relay"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
)

// TestLocalDaemonAutoRegistration tests the scenario where a local ffmpegd
// daemon automatically registers with the coordinator when both are running
// in the same container (US1: All-in-One Deployment).
func TestLocalDaemonAutoRegistration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Start coordinator gRPC server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	addr := listener.Addr().String()
	t.Logf("Coordinator listening on %s", addr)

	// Create daemon registry (coordinator side)
	registry := relay.NewDaemonRegistry(logger)

	// Create and start gRPC server
	grpcConfig := &relay.GRPCServerConfig{
		ListenAddr:        addr,
		AuthToken:         "", // No auth for test
		HeartbeatInterval: 5 * time.Second,
	}
	grpcServer := relay.NewGRPCServer(logger, grpcConfig, registry)

	// Start server in background with the listener
	go func() {
		if err := grpcServer.ServeWithListener(ctx, listener); err != nil {
			t.Logf("gRPC server stopped: %v", err)
		}
	}()
	defer grpcServer.Stop(ctx)

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Create client connection (simulating local ffmpegd)
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := proto.NewFFmpegDaemonClient(conn)

	// Test 1: Register daemon
	t.Run("daemon_registers_successfully", func(t *testing.T) {
		registerReq := &proto.RegisterRequest{
			DaemonId:   "local-test-daemon",
			DaemonName: "Local Test Daemon",
			Version:    "0.0.1-test",
			Capabilities: &proto.Capabilities{
				VideoEncoders:     []string{"libx264", "libx265"},
				VideoDecoders:     []string{"h264", "hevc"},
				AudioEncoders:     []string{"aac"},
				AudioDecoders:     []string{"aac", "ac3"},
				MaxConcurrentJobs: 4,
			},
		}

		resp, err := client.Register(ctx, registerReq)
		require.NoError(t, err)
		assert.True(t, resp.Success, "registration should succeed")
		assert.NotEmpty(t, resp.CoordinatorVersion)
		assert.Empty(t, resp.Error)

		// Verify daemon is in registry
		daemon, found := registry.Get(types.DaemonID("local-test-daemon"))
		require.True(t, found, "daemon should be in registry")
		require.NotNil(t, daemon, "daemon should not be nil")
		assert.Equal(t, "Local Test Daemon", daemon.Name)
		assert.Equal(t, "0.0.1-test", daemon.Version)
	})

	// Test 2: Heartbeat updates daemon state
	t.Run("heartbeat_updates_state", func(t *testing.T) {
		heartbeatReq := &proto.HeartbeatRequest{
			DaemonId: "local-test-daemon",
			SystemStats: &proto.SystemStats{
				Hostname:             "test-host",
				CpuPercent:           25.5,
				MemoryTotalBytes:     16 * 1024 * 1024 * 1024, // 16GB
				MemoryUsedBytes:      8 * 1024 * 1024 * 1024,  // 8GB
				MemoryAvailableBytes: 8 * 1024 * 1024 * 1024,
				MemoryPercent:        50.0,
			},
			ActiveJobs: []*proto.JobStatus{
				{JobId: "job-1", ChannelName: "Test Channel 1"},
				{JobId: "job-2", ChannelName: "Test Channel 2"},
			},
		}

		resp, err := client.Heartbeat(ctx, heartbeatReq)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		// Verify daemon state was updated
		daemon, found := registry.Get(types.DaemonID("local-test-daemon"))
		require.True(t, found)
		require.NotNil(t, daemon)
		assert.Equal(t, 2, daemon.ActiveJobs)
		assert.NotNil(t, daemon.SystemStats)
	})

	// Test 3: Daemon appears in registry list
	t.Run("daemon_in_registry_list", func(t *testing.T) {
		daemons := registry.GetAll()
		require.Len(t, daemons, 1)
		assert.Equal(t, types.DaemonID("local-test-daemon"), daemons[0].ID)
	})

	// Test 4: Unregister removes daemon
	t.Run("unregister_removes_daemon", func(t *testing.T) {
		unregisterReq := &proto.UnregisterRequest{
			DaemonId: "local-test-daemon",
			Reason:   "test cleanup",
		}

		resp, err := client.Unregister(ctx, unregisterReq)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		// Verify daemon is removed from registry
		_, found := registry.Get(types.DaemonID("local-test-daemon"))
		assert.False(t, found, "daemon should be removed from registry")
	})
}

// TestLocalDaemonReconnection tests that a local daemon can reconnect
// after coordinator restart (edge case handling).
func TestLocalDaemonReconnection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Start coordinator gRPC server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	addr := listener.Addr().String()

	// Create daemon registry (coordinator side)
	registry := relay.NewDaemonRegistry(logger)

	// Create and start gRPC server
	grpcConfig := &relay.GRPCServerConfig{
		ListenAddr:        addr,
		AuthToken:         "",
		HeartbeatInterval: 5 * time.Second,
	}
	grpcServer := relay.NewGRPCServer(logger, grpcConfig, registry)

	go func() {
		if err := grpcServer.ServeWithListener(ctx, listener); err != nil {
			t.Logf("gRPC server stopped: %v", err)
		}
	}()
	defer grpcServer.Stop(ctx)

	time.Sleep(100 * time.Millisecond)

	// Create client connection
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := proto.NewFFmpegDaemonClient(conn)

	// Register daemon twice (simulating reconnection after restart)
	t.Run("re-registration_after_restart", func(t *testing.T) {
		registerReq := &proto.RegisterRequest{
			DaemonId:   "reconnect-test-daemon",
			DaemonName: "Reconnect Test Daemon",
			Version:    "0.0.1-test",
			Capabilities: &proto.Capabilities{
				VideoEncoders:     []string{"libx264"},
				MaxConcurrentJobs: 2,
			},
		}

		// First registration
		resp, err := client.Register(ctx, registerReq)
		require.NoError(t, err)
		assert.True(t, resp.Success)

		// Second registration (simulating daemon reconnect)
		resp, err = client.Register(ctx, registerReq)
		require.NoError(t, err)
		assert.True(t, resp.Success, "re-registration should succeed")

		// Should still only have one daemon entry
		daemons := registry.GetAll()
		count := 0
		for _, d := range daemons {
			if d.ID == types.DaemonID("reconnect-test-daemon") {
				count++
			}
		}
		assert.Equal(t, 1, count, "should have exactly one daemon entry after re-registration")
	})
}

// TestLocalDaemonAuthToken tests authentication token validation
// between local ffmpegd and coordinator.
func TestLocalDaemonAuthToken(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Start coordinator gRPC server with auth token
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	addr := listener.Addr().String()
	authToken := "test-secret-token-12345"

	registry := relay.NewDaemonRegistry(logger)

	grpcConfig := &relay.GRPCServerConfig{
		ListenAddr:        addr,
		AuthToken:         authToken,
		HeartbeatInterval: 5 * time.Second,
	}
	grpcServer := relay.NewGRPCServer(logger, grpcConfig, registry)

	go func() {
		if err := grpcServer.ServeWithListener(ctx, listener); err != nil {
			t.Logf("gRPC server stopped: %v", err)
		}
	}()
	defer grpcServer.Stop(ctx)

	time.Sleep(100 * time.Millisecond)

	// Create client connection
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := proto.NewFFmpegDaemonClient(conn)

	t.Run("registration_with_valid_token", func(t *testing.T) {
		registerReq := &proto.RegisterRequest{
			DaemonId:   "auth-test-daemon",
			DaemonName: "Auth Test Daemon",
			Version:    "0.0.1-test",
			AuthToken:  authToken,
			Capabilities: &proto.Capabilities{
				MaxConcurrentJobs: 2,
			},
		}

		resp, err := client.Register(ctx, registerReq)
		require.NoError(t, err)
		assert.True(t, resp.Success, "registration with valid token should succeed")
	})

	t.Run("registration_with_invalid_token", func(t *testing.T) {
		registerReq := &proto.RegisterRequest{
			DaemonId:   "invalid-auth-daemon",
			DaemonName: "Invalid Auth Daemon",
			Version:    "0.0.1-test",
			AuthToken:  "wrong-token",
			Capabilities: &proto.Capabilities{
				MaxConcurrentJobs: 2,
			},
		}

		resp, err := client.Register(ctx, registerReq)
		// The server should return success=false with an error, not a gRPC error
		if err == nil {
			assert.False(t, resp.Success, "registration with invalid token should fail")
			assert.NotEmpty(t, resp.Error, "should have error message")
		}
		// If gRPC error, that's also acceptable
	})
}
