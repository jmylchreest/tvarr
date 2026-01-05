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

// TestCoordinatorRestart tests daemon recovery when coordinator restarts.
// T111: Edge case test for coordinator restart.
func TestCoordinatorRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create initial coordinator
	registry1 := relay.NewDaemonRegistry(logger).WithHeartbeatTimeout(5 * time.Second)
	registry1.Start(ctx)

	listener1, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener1.Addr().String()

	grpcServer1 := grpc.NewServer()
	proto.RegisterFFmpegDaemonServer(grpcServer1, newTestCoordinatorServer(registry1, logger))

	go func() {
		_ = grpcServer1.Serve(listener1)
	}()

	// Register a daemon
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	client := proto.NewFFmpegDaemonClient(conn)

	regReq := &proto.RegisterRequest{
		DaemonId:   "restart-test-daemon",
		DaemonName: "Restart Test Daemon",
		Version:    "1.0.0",
		Capabilities: &proto.Capabilities{
			VideoEncoders:     []string{"libx264"},
			MaxConcurrentJobs: 2,
		},
	}

	resp, err := client.Register(ctx, regReq)
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, 1, registry1.Count())

	t.Logf("Daemon registered, simulating coordinator restart...")

	// Simulate coordinator restart: stop server
	grpcServer1.GracefulStop()
	registry1.Stop()
	conn.Close()

	// Short pause to simulate downtime
	time.Sleep(500 * time.Millisecond)

	// Start new coordinator on same address
	registry2 := relay.NewDaemonRegistry(logger).WithHeartbeatTimeout(5 * time.Second)
	registry2.Start(ctx)
	defer registry2.Stop()

	listener2, err := net.Listen("tcp", addr)
	require.NoError(t, err)
	defer listener2.Close()

	grpcServer2 := grpc.NewServer()
	proto.RegisterFFmpegDaemonServer(grpcServer2, newTestCoordinatorServer(registry2, logger))

	go func() {
		_ = grpcServer2.Serve(listener2)
	}()
	defer grpcServer2.GracefulStop()

	// Daemon should re-register after coordinator restart
	conn2, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn2.Close()
	client2 := proto.NewFFmpegDaemonClient(conn2)

	// Daemon re-registers (simulating reconnection behavior)
	resp2, err := client2.Register(ctx, regReq)
	require.NoError(t, err)
	assert.True(t, resp2.Success)

	// Verify daemon is back in new registry
	assert.Equal(t, 1, registry2.Count())
	daemon, ok := registry2.Get(types.DaemonID("restart-test-daemon"))
	require.True(t, ok)
	assert.Equal(t, types.DaemonStateConnected, daemon.State)
}

// TestNetworkPartition tests behavior during network partition.
// T111: Edge case test for network partition.
func TestNetworkPartition(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create coordinator with short heartbeat timeout and cleanup interval to speed up test
	registry := relay.NewDaemonRegistry(logger).
		WithHeartbeatTimeout(2 * time.Second).
		WithCleanupInterval(500 * time.Millisecond)
	registry.Start(ctx)
	defer registry.Stop()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	grpcServer := grpc.NewServer()
	proto.RegisterFFmpegDaemonServer(grpcServer, newTestCoordinatorServer(registry, logger))

	go func() {
		_ = grpcServer.Serve(listener)
	}()
	defer grpcServer.GracefulStop()

	// Register daemon
	conn, err := grpc.NewClient(
		listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()
	client := proto.NewFFmpegDaemonClient(conn)

	regReq := &proto.RegisterRequest{
		DaemonId:   "partition-test-daemon",
		DaemonName: "Partition Test Daemon",
		Version:    "1.0.0",
		Capabilities: &proto.Capabilities{
			VideoEncoders:     []string{"libx264"},
			MaxConcurrentJobs: 2,
		},
	}

	_, err = client.Register(ctx, regReq)
	require.NoError(t, err)

	// Helper to get daemon state safely (thread-safe via registry)
	getDaemonState := func() types.DaemonState {
		return registry.GetState(types.DaemonID("partition-test-daemon"))
	}

	// Verify daemon is active using Eventually to avoid race
	require.Eventually(t, func() bool {
		return getDaemonState() == types.DaemonStateConnected
	}, 1*time.Second, 50*time.Millisecond, "daemon should be connected initially")

	t.Logf("Daemon registered, simulating network partition (no heartbeats)...")

	// Simulate network partition by not sending heartbeats
	// Wait for daemon to be marked unhealthy
	require.Eventually(t, func() bool {
		return getDaemonState() == types.DaemonStateUnhealthy
	}, 5*time.Second, 100*time.Millisecond, "daemon should be unhealthy after missed heartbeats")

	t.Logf("Network partition healed, sending heartbeat...")

	// Simulate network partition healing by sending heartbeat
	hbReq := &proto.HeartbeatRequest{
		DaemonId: "partition-test-daemon",
	}
	_, err = client.Heartbeat(ctx, hbReq)
	require.NoError(t, err)

	// Daemon should recover to active state
	require.Eventually(t, func() bool {
		return getDaemonState() == types.DaemonStateConnected
	}, 2*time.Second, 50*time.Millisecond, "daemon should recover after heartbeat")
}

// TestDaemonCrashRecovery tests coordinator handling of daemon crash.
// T111: Edge case test for daemon crash.
func TestDaemonCrashRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Create coordinator with short timeouts and cleanup interval
	registry := relay.NewDaemonRegistry(logger).
		WithHeartbeatTimeout(2 * time.Second).
		WithCleanupInterval(500 * time.Millisecond)
	registry.Start(ctx)
	defer registry.Stop()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	grpcServer := grpc.NewServer()
	proto.RegisterFFmpegDaemonServer(grpcServer, newTestCoordinatorServer(registry, logger))

	go func() {
		_ = grpcServer.Serve(listener)
	}()
	defer grpcServer.GracefulStop()

	// Register daemon
	conn, err := grpc.NewClient(
		listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	client := proto.NewFFmpegDaemonClient(conn)

	regReq := &proto.RegisterRequest{
		DaemonId:   "crash-test-daemon",
		DaemonName: "Crash Test Daemon",
		Version:    "1.0.0",
		Capabilities: &proto.Capabilities{
			VideoEncoders:     []string{"libx264", "h264_nvenc"},
			MaxConcurrentJobs: 4,
		},
	}

	_, err = client.Register(ctx, regReq)
	require.NoError(t, err)

	t.Logf("Daemon registered, simulating crash by closing connection...")

	// Simulate daemon crash by closing connection abruptly
	conn.Close()

	// Wait for timeout
	time.Sleep(3 * time.Second)

	// Daemon should be marked unhealthy
	daemon, _ := registry.Get(types.DaemonID("crash-test-daemon"))
	assert.Equal(t, types.DaemonStateUnhealthy, daemon.State, "crashed daemon should be marked unhealthy")

	t.Logf("Daemon restarting (re-registering)...")

	// Simulate daemon restart by re-connecting and re-registering
	conn2, err := grpc.NewClient(
		listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn2.Close()
	client2 := proto.NewFFmpegDaemonClient(conn2)

	// Re-registration after crash
	resp, err := client2.Register(ctx, regReq)
	require.NoError(t, err)
	assert.True(t, resp.Success)

	// Daemon should be active again
	daemon, _ = registry.Get(types.DaemonID("crash-test-daemon"))
	assert.Equal(t, types.DaemonStateConnected, daemon.State, "restarted daemon should be active")
	assert.Equal(t, "Crash Test Daemon", daemon.Name)
}

// TestDaemonGracefulUnregister tests graceful daemon shutdown.
// T111: Edge case test - graceful vs abrupt disconnection.
func TestDaemonGracefulUnregister(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	registry := relay.NewDaemonRegistry(logger)
	registry.Start(ctx)
	defer registry.Stop()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	// Create server with unregister support
	coordServer := &unregisterTestServer{
		testCoordinatorServer: *newTestCoordinatorServer(registry, logger),
	}
	grpcServer := grpc.NewServer()
	proto.RegisterFFmpegDaemonServer(grpcServer, coordServer)

	go func() {
		_ = grpcServer.Serve(listener)
	}()
	defer grpcServer.GracefulStop()

	conn, err := grpc.NewClient(
		listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()
	client := proto.NewFFmpegDaemonClient(conn)

	// Register daemon
	regReq := &proto.RegisterRequest{
		DaemonId:   "graceful-test-daemon",
		DaemonName: "Graceful Test Daemon",
		Version:    "1.0.0",
		Capabilities: &proto.Capabilities{
			VideoEncoders:     []string{"libx264"},
			MaxConcurrentJobs: 2,
		},
	}

	_, err = client.Register(ctx, regReq)
	require.NoError(t, err)
	assert.Equal(t, 1, registry.Count())

	t.Logf("Daemon unregistering gracefully...")

	// Graceful unregister
	unregReq := &proto.UnregisterRequest{
		DaemonId: "graceful-test-daemon",
		Reason:   "shutdown",
	}

	_, err = client.Unregister(ctx, unregReq)
	require.NoError(t, err)

	// Daemon should be removed immediately for graceful shutdown
	daemon, ok := registry.Get(types.DaemonID("graceful-test-daemon"))
	if ok {
		// If still in registry, should be disconnected
		assert.Equal(t, types.DaemonStateDisconnected, daemon.State)
	}
}

// TestMultipleDaemonsReconnect tests multiple daemons reconnecting simultaneously.
// T111: Edge case test - multiple daemon reconnection race.
func TestMultipleDaemonsReconnect(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	registry := relay.NewDaemonRegistry(logger)
	registry.Start(ctx)
	defer registry.Stop()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	grpcServer := grpc.NewServer()
	proto.RegisterFFmpegDaemonServer(grpcServer, newTestCoordinatorServer(registry, logger))

	go func() {
		_ = grpcServer.Serve(listener)
	}()
	defer grpcServer.GracefulStop()

	// Register multiple daemons concurrently
	const numDaemons = 10
	errChan := make(chan error, numDaemons)
	successChan := make(chan string, numDaemons)

	for i := range numDaemons {
		go func(idx int) {
			conn, err := grpc.NewClient(
				listener.Addr().String(),
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			)
			if err != nil {
				errChan <- err
				return
			}
			defer conn.Close()

			client := proto.NewFFmpegDaemonClient(conn)

			regReq := &proto.RegisterRequest{
				DaemonId:   types.DaemonID(string(rune('a'+idx)) + "-daemon").String(),
				DaemonName: string(rune('A'+idx)) + " Daemon",
				Version:    "1.0.0",
				Capabilities: &proto.Capabilities{
					VideoEncoders:     []string{"libx264"},
					MaxConcurrentJobs: 2,
				},
			}

			resp, err := client.Register(ctx, regReq)
			if err != nil {
				errChan <- err
				return
			}
			if !resp.Success {
				errChan <- err
				return
			}
			successChan <- regReq.DaemonId
		}(i)
	}

	// Collect results
	var registered []string
	var errors []error
	for range numDaemons {
		select {
		case err := <-errChan:
			errors = append(errors, err)
		case id := <-successChan:
			registered = append(registered, id)
		case <-ctx.Done():
			t.Fatal("timeout waiting for daemon registrations")
		}
	}

	assert.Empty(t, errors, "no registration errors expected")
	assert.Len(t, registered, numDaemons, "all daemons should register")
	assert.Equal(t, numDaemons, registry.Count(), "registry should have all daemons")
}

// unregisterTestServer adds Unregister support for testing.
type unregisterTestServer struct {
	testCoordinatorServer
}

func (s *unregisterTestServer) Unregister(ctx context.Context, req *proto.UnregisterRequest) (*proto.UnregisterResponse, error) {
	s.registry.Unregister(types.DaemonID(req.DaemonId), req.Reason)
	return &proto.UnregisterResponse{
		Success: true,
	}, nil
}
