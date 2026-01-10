package relay

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
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

	// Stream and job management for distributed transcoding
	streamMgr *DaemonStreamManager
	jobMgr    *ActiveJobManager

	// Listeners
	internalListener net.Listener // Unix socket for local subprocess connections (always available)
	externalListener net.Listener // TCP listener for remote daemon connections (optional)
	internalAddr     string       // Full address for internal connections (e.g., "unix:///tmp/tvarr/grpc.sock")

	mu      sync.RWMutex
	started bool
}

// DefaultInternalSocketPath is the default path for the internal Unix socket.
const DefaultInternalSocketPath = "/tmp/tvarr/grpc.sock"

// GRPCServerConfig holds configuration for the coordinator gRPC server.
type GRPCServerConfig struct {
	// InternalSocketPath is the path for the internal Unix socket (always created).
	// Defaults to DefaultInternalSocketPath if empty.
	InternalSocketPath string

	// ExternalListenAddr is the optional TCP address for remote daemon connections (e.g., ":9090").
	// If empty, only the internal Unix socket is available.
	ExternalListenAddr string

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
	if cfg.InternalSocketPath == "" {
		cfg.InternalSocketPath = DefaultInternalSocketPath
	}

	// Build internal address (gRPC dial format)
	internalAddr := "unix://" + cfg.InternalSocketPath

	return &GRPCServer{
		logger:       logger,
		config:       cfg,
		registry:     registry,
		streamMgr:    NewDaemonStreamManager(logger, registry),
		jobMgr:       NewActiveJobManager(logger),
		internalAddr: internalAddr,
	}
}

// Start starts the gRPC server with internal Unix socket (always) and optional external TCP listener.
func (s *GRPCServer) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("server already started")
	}

	// Ensure socket directory exists
	socketDir := filepath.Dir(s.config.InternalSocketPath)
	if err := os.MkdirAll(socketDir, 0750); err != nil {
		return fmt.Errorf("creating socket directory: %w", err)
	}

	// Remove stale socket file if exists
	if err := os.Remove(s.config.InternalSocketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing stale socket: %w", err)
	}

	// Create internal Unix socket listener (always)
	internalListener, err := net.Listen("unix", s.config.InternalSocketPath)
	if err != nil {
		return fmt.Errorf("creating internal Unix socket listener: %w", err)
	}
	s.internalListener = internalListener

	// Create external TCP listener (optional)
	if s.config.ExternalListenAddr != "" {
		externalListener, err := net.Listen("tcp", s.config.ExternalListenAddr)
		if err != nil {
			_ = s.internalListener.Close()
			return fmt.Errorf("creating external TCP listener: %w", err)
		}
		s.externalListener = externalListener
	}

	// Create gRPC server
	s.server = grpc.NewServer(
		grpc.UnaryInterceptor(s.unaryInterceptor),
		grpc.StreamInterceptor(s.streamInterceptor),
	)
	proto.RegisterFFmpegDaemonServer(s.server, s)

	s.started = true

	// Log startup with consolidated listener information
	externalAddr := ""
	if s.externalListener != nil {
		externalAddr = s.config.ExternalListenAddr
	}
	s.logger.Info("grpc server started",
		slog.String("internal_socket", s.config.InternalSocketPath),
		slog.String("external_addr", externalAddr),
	)

	// Start serving on internal socket
	go func() {
		if err := s.server.Serve(s.internalListener); err != nil {
			s.logger.Error("gRPC internal server error", slog.String("error", err.Error()))
		}
	}()

	// Start serving on external listener if enabled
	if s.externalListener != nil {
		go func() {
			if err := s.server.Serve(s.externalListener); err != nil {
				s.logger.Error("gRPC external server error", slog.String("error", err.Error()))
			}
		}()
	}

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

	s.logger.Info("grpc server started",
		slog.String("listen_addr", listener.Addr().String()),
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

	// Clean up socket file
	if s.config.InternalSocketPath != "" {
		_ = os.Remove(s.config.InternalSocketPath)
	}

	s.started = false
	return nil
}

// InternalAddress returns the gRPC dial address for the internal Unix socket.
// Format: "unix:///tmp/tvarr/grpc.sock"
func (s *GRPCServer) InternalAddress() string {
	return s.internalAddr
}

// Register handles daemon registration requests.
func (s *GRPCServer) Register(ctx context.Context, req *proto.RegisterRequest) (*proto.RegisterResponse, error) {
	s.logger.Debug("daemon registration request",
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

	// Register with the daemon registry (registry logs at INFO level)
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
// Daemons open this stream after registration and keep it open.
// The coordinator pushes TranscodeStart messages when it needs transcoding done,
// and receives transcoded ESSampleBatch messages back.
func (s *GRPCServer) Transcode(stream grpc.BidiStreamingServer[proto.TranscodeMessage, proto.TranscodeMessage]) error {
	// First message must identify the daemon
	msg, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.Internal, "receiving initial message: %v", err)
	}

	// Expect a "ready" message with daemon ID in the start payload
	// For now, we'll accept a TranscodeStart with empty job_id as a "ready" signal
	var daemonID types.DaemonID

	switch payload := msg.Payload.(type) {
	case *proto.TranscodeMessage_Start:
		// Daemon sends TranscodeStart with daemon_id in session_id field as "ready" signal
		if payload.Start.SessionId == "" {
			return status.Errorf(codes.InvalidArgument, "daemon must identify itself with session_id containing daemon_id")
		}
		daemonID = types.DaemonID(payload.Start.SessionId)

		// Verify daemon is registered
		if _, ok := s.registry.Get(daemonID); !ok {
			return status.Errorf(codes.NotFound, "daemon not registered: %s", daemonID)
		}

	default:
		return status.Errorf(codes.InvalidArgument, "expected TranscodeStart as first message, got %T", msg.Payload)
	}

	// Register this stream with the stream manager
	daemonStream := s.streamMgr.RegisterStream(daemonID, stream)
	defer s.streamMgr.UnregisterStream(daemonID)

	// Main loop: receive messages from daemon
	for {
		msg, err := stream.Recv()
		if err != nil {
			s.logger.Debug("Daemon transcode stream ended",
				slog.String("daemon_id", string(daemonID)),
				slog.String("error", err.Error()),
			)
			return err
		}

		switch payload := msg.Payload.(type) {
		case *proto.TranscodeMessage_Samples:
			// Transcoded samples from daemon - route by job_id
			jobID := payload.Samples.JobId
			if jobID == "" {
				s.logger.Warn("Received samples without job_id",
					slog.String("daemon_id", string(daemonID)),
				)
				continue
			}

			job, ok := s.jobMgr.GetJob(jobID)
			if !ok {
				s.logger.Warn("Received samples for unknown job",
					slog.String("daemon_id", string(daemonID)),
					slog.String("job_id", jobID),
				)
				continue
			}

			job.SendSamples(payload.Samples)

		case *proto.TranscodeMessage_Stats:
			// Stats from daemon - route by job_id
			jobID := payload.Stats.JobId
			if jobID != "" {
				if job, ok := s.jobMgr.GetJob(jobID); ok {
					// Convert proto stats to types.TranscodeStats
					var runningTime time.Duration
					if payload.Stats.RunningTime != nil {
						runningTime = payload.Stats.RunningTime.AsDuration()
					}
					job.Stats = &types.TranscodeStats{
						SamplesIn:     payload.Stats.SamplesIn,
						SamplesOut:    payload.Stats.SamplesOut,
						BytesIn:       payload.Stats.BytesIn,
						BytesOut:      payload.Stats.BytesOut,
						EncodingSpeed: payload.Stats.EncodingSpeed,
						CPUPercent:    payload.Stats.CpuPercent,
						MemoryMB:      payload.Stats.MemoryMb,
						FFmpegPID:     int(payload.Stats.FfmpegPid),
						RunningTime:   runningTime,
						HWAccel:       payload.Stats.HwAccel,
						HWDevice:      payload.Stats.HwDevice,
						FFmpegCommand: payload.Stats.FfmpegCommand,
					}
				}
			}

			s.logger.Log(stream.Context(), observability.LevelTrace, "Transcode stats from daemon",
				slog.String("daemon_id", string(daemonID)),
				slog.String("job_id", jobID),
				slog.Uint64("samples_in", payload.Stats.SamplesIn),
				slog.Uint64("samples_out", payload.Stats.SamplesOut),
				slog.Float64("encoding_speed", payload.Stats.EncodingSpeed),
			)

		case *proto.TranscodeMessage_Ack:
			// Acknowledgment from daemon (e.g., job started)
			s.logger.Debug("Transcode ack from daemon",
				slog.String("daemon_id", string(daemonID)),
				slog.String("job_id", payload.Ack.JobId),
				slog.Bool("success", payload.Ack.Success),
				slog.String("actual_video_encoder", payload.Ack.ActualVideoEncoder),
			)

		case *proto.TranscodeMessage_Error:
			// Error from daemon - route by job_id
			jobID := payload.Error.JobId
			s.logger.Error("Transcode error from daemon",
				slog.String("daemon_id", string(daemonID)),
				slog.String("job_id", jobID),
				slog.String("message", payload.Error.Message),
				slog.Int("code", int(payload.Error.Code)),
			)

			if jobID != "" {
				if job, ok := s.jobMgr.GetJob(jobID); ok {
					job.SetError(fmt.Errorf("daemon error: %s", payload.Error.Message))
				}
			}

		case *proto.TranscodeMessage_Stop:
			// Daemon signaling job completion - route by job_id
			jobID := payload.Stop.JobId
			s.logger.Debug("Transcode stop from daemon",
				slog.String("daemon_id", string(daemonID)),
				slog.String("job_id", jobID),
				slog.String("reason", payload.Stop.Reason),
			)

			if jobID != "" {
				s.jobMgr.RemoveJob(jobID)
			}

		case *proto.TranscodeMessage_ProbeResponse:
			// Probe response from daemon
			s.logger.Debug("Probe response from daemon",
				slog.String("daemon_id", string(daemonID)),
				slog.Bool("success", payload.ProbeResponse.Success),
				slog.String("video_codec", payload.ProbeResponse.VideoCodec),
				slog.String("audio_codec", payload.ProbeResponse.AudioCodec),
			)

			// Deliver to waiting caller (the stream URL isn't in response, so we need to track it)
			// For now, we use the fact that there's typically only one pending probe per daemon
			daemonStream.mu.Lock()
			for streamURL := range daemonStream.pendingProbes {
				// Deliver to first (and typically only) pending probe
				daemonStream.mu.Unlock()
				daemonStream.DeliverProbeResponse(payload.ProbeResponse, streamURL)
				daemonStream.mu.Lock()
				break
			}
			daemonStream.mu.Unlock()
		}
	}
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
func (s *GRPCServer) unaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
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
func (s *GRPCServer) streamInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
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

// GetStreamManager returns the daemon stream manager.
func (s *GRPCServer) GetStreamManager() *DaemonStreamManager {
	return s.streamMgr
}

// GetJobManager returns the active job manager.
func (s *GRPCServer) GetJobManager() *ActiveJobManager {
	return s.jobMgr
}
