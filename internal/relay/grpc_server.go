package relay

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/jmylchreest/tvarr/internal/observability"
	"github.com/jmylchreest/tvarr/internal/version"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

// GRPCServer implements the FFmpegDaemon gRPC service on the coordinator side.
// It accepts registration, heartbeat, and transcode requests from ffmpegd daemons.
type GRPCServer struct {
	proto.UnimplementedFFmpegDaemonServer

	logger   *slog.Logger
	config   *GRPCServerConfig
	server   *grpc.Server
	registry *DaemonRegistry

	mu      sync.RWMutex
	started bool
}

// GRPCServerConfig holds configuration for the coordinator gRPC server.
type GRPCServerConfig struct {
	// ListenAddr is the address to listen on (e.g., ":9090")
	ListenAddr string

	// AuthToken is the optional authentication token daemons must provide
	AuthToken string

	// HeartbeatInterval is the expected interval between daemon heartbeats
	HeartbeatInterval time.Duration
}

// NewGRPCServer creates a new coordinator gRPC server.
func NewGRPCServer(logger *slog.Logger, cfg *GRPCServerConfig, registry *DaemonRegistry) *GRPCServer {
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = 5 * time.Second
	}

	return &GRPCServer{
		logger:   logger,
		config:   cfg,
		registry: registry,
	}
}

// Start starts the gRPC server.
func (s *GRPCServer) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("server already started")
	}

	listener, err := net.Listen("tcp", s.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("creating listener: %w", err)
	}

	s.server = grpc.NewServer(
		grpc.UnaryInterceptor(s.unaryInterceptor),
		grpc.StreamInterceptor(s.streamInterceptor),
	)
	proto.RegisterFFmpegDaemonServer(s.server, s)

	s.started = true

	s.logger.Info("starting coordinator gRPC server",
		slog.String("address", s.config.ListenAddr),
	)

	go func() {
		if err := s.server.Serve(listener); err != nil {
			s.logger.Error("gRPC server error", slog.String("error", err.Error()))
		}
	}()

	// Start registry cleanup goroutine
	s.registry.Start(ctx)

	return nil
}

// ServeWithListener starts the gRPC server with an existing listener.
// This is useful for testing where the listener is created externally.
func (s *GRPCServer) ServeWithListener(ctx context.Context, listener net.Listener) error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("server already started")
	}

	s.server = grpc.NewServer(
		grpc.UnaryInterceptor(s.unaryInterceptor),
		grpc.StreamInterceptor(s.streamInterceptor),
	)
	proto.RegisterFFmpegDaemonServer(s.server, s)
	s.started = true
	s.mu.Unlock()

	s.logger.Info("starting coordinator gRPC server with listener",
		slog.String("address", listener.Addr().String()),
	)

	// Start registry cleanup goroutine
	s.registry.Start(ctx)

	// Serve (blocking)
	return s.server.Serve(listener)
}

// Stop stops the gRPC server gracefully.
func (s *GRPCServer) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}

	s.registry.Stop()

	if s.server != nil {
		done := make(chan struct{})
		go func() {
			s.server.GracefulStop()
			close(done)
		}()

		select {
		case <-done:
			s.logger.Info("gRPC server stopped gracefully")
		case <-ctx.Done():
			s.server.Stop()
			s.logger.Warn("gRPC server force stopped")
		}
	}

	s.started = false
	return nil
}

// Register handles daemon registration requests.
func (s *GRPCServer) Register(ctx context.Context, req *proto.RegisterRequest) (*proto.RegisterResponse, error) {
	s.logger.Info("daemon registration request",
		slog.String("daemon_id", req.DaemonId),
		slog.String("daemon_name", req.DaemonName),
		slog.String("version", req.Version),
	)

	// Validate auth token if configured
	if s.config.AuthToken != "" && req.AuthToken != s.config.AuthToken {
		s.logger.Warn("registration rejected: invalid auth token",
			slog.String("daemon_id", req.DaemonId),
		)
		return &proto.RegisterResponse{
			Success: false,
			Error:   "invalid authentication token",
		}, nil
	}

	// Register with the daemon registry
	_, err := s.registry.Register(req)
	if err != nil {
		s.logger.Error("registration failed",
			slog.String("daemon_id", req.DaemonId),
			slog.String("error", err.Error()),
		)
		return &proto.RegisterResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	s.logger.Info("daemon registered successfully",
		slog.String("daemon_id", req.DaemonId),
		slog.String("daemon_name", req.DaemonName),
		slog.Int("max_jobs", int(req.Capabilities.MaxConcurrentJobs)),
		slog.Int("gpus", len(req.Capabilities.Gpus)),
	)

	return &proto.RegisterResponse{
		Success:            true,
		HeartbeatInterval:  durationpb.New(s.config.HeartbeatInterval),
		CoordinatorVersion: version.Short(),
	}, nil
}

// Heartbeat handles periodic health updates from daemons.
func (s *GRPCServer) Heartbeat(ctx context.Context, req *proto.HeartbeatRequest) (*proto.HeartbeatResponse, error) {
	s.logger.Log(ctx, observability.LevelTrace, "heartbeat received",
		slog.String("daemon_id", req.DaemonId),
		slog.Int("active_jobs", len(req.ActiveJobs)),
	)

	// Process heartbeat in registry
	_, err := s.registry.HandleHeartbeat(req)
	if err != nil {
		s.logger.Warn("heartbeat from unknown daemon",
			slog.String("daemon_id", req.DaemonId),
			slog.String("error", err.Error()),
		)
		return &proto.HeartbeatResponse{
			Success: false,
		}, nil
	}

	return &proto.HeartbeatResponse{
		Success: true,
	}, nil
}

// Unregister handles graceful daemon removal.
func (s *GRPCServer) Unregister(ctx context.Context, req *proto.UnregisterRequest) (*proto.UnregisterResponse, error) {
	s.logger.Info("daemon unregister request",
		slog.String("daemon_id", req.DaemonId),
		slog.String("reason", req.Reason),
	)

	s.registry.Unregister(types.DaemonID(req.DaemonId), req.Reason)

	return &proto.UnregisterResponse{
		Success: true,
	}, nil
}

// Transcode handles bidirectional ES sample streaming for transcoding.
// This is a placeholder - full implementation will be in Phase 4 (US2).
func (s *GRPCServer) Transcode(stream grpc.BidiStreamingServer[proto.TranscodeMessage, proto.TranscodeMessage]) error {
	// Phase 4 implementation: The coordinator will stream ES samples to daemons
	// and receive transcoded samples back. For now, return unimplemented.
	return status.Errorf(codes.Unimplemented, "transcode streaming not yet implemented")
}

// GetStats returns current daemon statistics.
// Note: This is primarily used by daemons to report their stats.
// On the coordinator side, stats are collected via heartbeats.
func (s *GRPCServer) GetStats(ctx context.Context, req *proto.GetStatsRequest) (*proto.GetStatsResponse, error) {
	// On coordinator, this would return aggregate stats
	// For now, return empty response as daemons report via heartbeat
	return &proto.GetStatsResponse{}, nil
}

// unaryInterceptor adds logging to unary RPCs.
func (s *GRPCServer) unaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	duration := time.Since(start)

	// Use TRACE level for high-frequency heartbeat calls, DEBUG for others
	logLevel := slog.LevelDebug
	if info.FullMethod == "/ffmpegd.FFmpegDaemon/Heartbeat" {
		logLevel = observability.LevelTrace
	}

	if err != nil {
		s.logger.Log(ctx, logLevel, "gRPC call failed",
			slog.String("method", info.FullMethod),
			slog.Duration("duration", duration),
			slog.String("error", err.Error()),
		)
	} else {
		s.logger.Log(ctx, logLevel, "gRPC call completed",
			slog.String("method", info.FullMethod),
			slog.Duration("duration", duration),
		)
	}

	return resp, err
}

// streamInterceptor adds logging to streaming RPCs.
func (s *GRPCServer) streamInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	start := time.Now()
	err := handler(srv, ss)
	duration := time.Since(start)

	if err != nil {
		s.logger.Debug("gRPC stream ended with error",
			slog.String("method", info.FullMethod),
			slog.Duration("duration", duration),
			slog.String("error", err.Error()),
		)
	} else {
		s.logger.Debug("gRPC stream ended",
			slog.String("method", info.FullMethod),
			slog.Duration("duration", duration),
		)
	}

	return err
}

// GetRegistry returns the daemon registry.
func (s *GRPCServer) GetRegistry() *DaemonRegistry {
	return s.registry
}
