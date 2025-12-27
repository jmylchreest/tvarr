package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/jmylchreest/tvarr/internal/observability"
	"github.com/jmylchreest/tvarr/internal/version"
	"github.com/jmylchreest/tvarr/pkg/ffmpeg"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

// Server implements the FFmpegDaemon gRPC service.
type Server struct {
	proto.UnimplementedFFmpegDaemonServer

	logger     *slog.Logger
	config     *Config
	grpcServer *grpc.Server

	// Daemon state
	mu             sync.RWMutex
	id             string
	name           string
	state          types.DaemonState
	capabilities   *proto.Capabilities
	binInfo        *ffmpeg.BinaryInfo
	statsCollector *StatsCollector

	// Job tracking
	activeJobs     map[string]*types.TranscodeJob
	totalCompleted uint64
	totalFailed    uint64

	// Registration state
	registered      bool
	heartbeatCancel context.CancelFunc
}

// Config holds daemon server configuration.
type Config struct {
	ID                string
	Name              string
	ListenAddr        string
	MaxConcurrentJobs int
	HeartbeatInterval time.Duration
	AuthToken         string
}

// NewServer creates a new daemon server.
func NewServer(logger *slog.Logger, cfg *Config) *Server {
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = 5 * time.Second
	}
	if cfg.MaxConcurrentJobs == 0 {
		cfg.MaxConcurrentJobs = 4
	}

	return &Server{
		logger:     logger,
		config:     cfg,
		id:         cfg.ID,
		name:       cfg.Name,
		state:      types.DaemonStateConnecting,
		activeJobs: make(map[string]*types.TranscodeJob),
	}
}

// Start starts the gRPC server.
func (s *Server) Start(ctx context.Context) error {
	// Detect capabilities
	s.logger.Info("detecting FFmpeg capabilities")
	capDetector := NewCapabilityDetector()
	caps, binInfo, err := capDetector.Detect(ctx)
	if err != nil {
		return fmt.Errorf("detecting capabilities: %w", err)
	}

	s.mu.Lock()
	s.capabilities = caps
	s.capabilities.MaxConcurrentJobs = int32(s.config.MaxConcurrentJobs)
	s.binInfo = binInfo
	s.statsCollector = NewStatsCollector(caps.Gpus)
	s.mu.Unlock()

	s.logger.Info("capabilities detected",
		slog.String("ffmpeg_version", binInfo.Version),
		slog.Int("video_encoders", len(caps.VideoEncoders)),
		slog.Int("hw_accels", len(caps.HwAccels)),
		slog.Int("gpus", len(caps.Gpus)),
	)

	// Create gRPC server
	s.grpcServer = grpc.NewServer(
		grpc.UnaryInterceptor(s.unaryInterceptor),
		grpc.StreamInterceptor(s.streamInterceptor),
	)
	proto.RegisterFFmpegDaemonServer(s.grpcServer, s)

	// Start listener if address is configured
	if s.config.ListenAddr != "" {
		listener, err := net.Listen("tcp", s.config.ListenAddr)
		if err != nil {
			return fmt.Errorf("creating listener: %w", err)
		}

		s.logger.Info("starting gRPC server",
			slog.String("address", s.config.ListenAddr),
		)

		go func() {
			if err := s.grpcServer.Serve(listener); err != nil {
				s.logger.Error("gRPC server error", slog.String("error", err.Error()))
			}
		}()
	}

	s.mu.Lock()
	s.state = types.DaemonStateConnected
	s.mu.Unlock()

	return nil
}

// Stop stops the gRPC server gracefully.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	s.state = types.DaemonStateDraining
	if s.heartbeatCancel != nil {
		s.heartbeatCancel()
	}
	s.mu.Unlock()

	if s.grpcServer != nil {
		// Graceful stop with timeout
		done := make(chan struct{})
		go func() {
			s.grpcServer.GracefulStop()
			close(done)
		}()

		select {
		case <-done:
			s.logger.Info("gRPC server stopped gracefully")
		case <-ctx.Done():
			s.grpcServer.Stop()
			s.logger.Warn("gRPC server force stopped")
		}
	}

	s.mu.Lock()
	s.state = types.DaemonStateDisconnected
	s.mu.Unlock()

	return nil
}

// Register handles daemon registration from coordinator.
func (s *Server) Register(ctx context.Context, req *proto.RegisterRequest) (*proto.RegisterResponse, error) {
	s.logger.Info("registration request received",
		slog.String("daemon_id", req.DaemonId),
		slog.String("daemon_name", req.DaemonName),
	)

	// Validate auth token if configured
	if s.config.AuthToken != "" && req.AuthToken != s.config.AuthToken {
		s.logger.Warn("registration rejected: invalid auth token")
		return &proto.RegisterResponse{
			Success: false,
			Error:   "invalid auth token",
		}, nil
	}

	s.mu.Lock()
	s.registered = true
	s.mu.Unlock()

	s.logger.Info("daemon registered successfully",
		slog.String("daemon_id", req.DaemonId),
	)

	return &proto.RegisterResponse{
		Success:            true,
		HeartbeatInterval:  durationpb.New(s.config.HeartbeatInterval),
		CoordinatorVersion: version.Short(),
	}, nil
}

// Heartbeat handles periodic health updates.
func (s *Server) Heartbeat(ctx context.Context, req *proto.HeartbeatRequest) (*proto.HeartbeatResponse, error) {
	s.logger.Debug("heartbeat received",
		slog.String("daemon_id", req.DaemonId),
		slog.Int("active_jobs", len(req.ActiveJobs)),
	)

	// Validate daemon ID
	if req.DaemonId != s.id {
		return &proto.HeartbeatResponse{
			Success: false,
		}, nil
	}

	// Store the received stats (from client perspective, this would be reversed)
	// For the server side, we just acknowledge the heartbeat

	return &proto.HeartbeatResponse{
		Success: true,
	}, nil
}

// Unregister handles graceful daemon removal.
func (s *Server) Unregister(ctx context.Context, req *proto.UnregisterRequest) (*proto.UnregisterResponse, error) {
	s.logger.Info("unregister request received",
		slog.String("daemon_id", req.DaemonId),
		slog.String("reason", req.Reason),
	)

	s.mu.Lock()
	s.registered = false
	s.state = types.DaemonStateDraining
	s.mu.Unlock()

	return &proto.UnregisterResponse{
		Success: true,
	}, nil
}

// Transcode handles bidirectional ES sample streaming for transcoding.
func (s *Server) Transcode(stream grpc.BidiStreamingServer[proto.TranscodeMessage, proto.TranscodeMessage]) error {
	s.mu.RLock()
	state := s.state
	activeJobCount := len(s.activeJobs)
	maxJobs := int(s.capabilities.MaxConcurrentJobs)
	binInfo := s.binInfo
	s.mu.RUnlock()

	// Check if we can accept jobs
	if state != types.DaemonStateConnected {
		return status.Errorf(codes.Unavailable, "daemon not active (state: %s)", state.String())
	}

	if activeJobCount >= maxJobs {
		return status.Errorf(codes.ResourceExhausted, "max concurrent jobs reached (%d/%d)", activeJobCount, maxJobs)
	}

	// Wait for start message
	msg, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.Internal, "receiving start message: %v", err)
	}

	startMsg, ok := msg.Payload.(*proto.TranscodeMessage_Start)
	if !ok {
		return status.Errorf(codes.InvalidArgument, "expected TranscodeStart message, got %T", msg.Payload)
	}

	jobID := startMsg.Start.JobId
	s.logger.Info("transcode job started",
		slog.String("job_id", jobID),
		slog.String("channel", startMsg.Start.ChannelName),
		slog.String("target_video_codec", startMsg.Start.TargetVideoCodec),
	)

	// Track the job for stats
	jobRecord := &types.TranscodeJob{
		ID:          types.JobID(jobID),
		SessionID:   startMsg.Start.SessionId,
		ChannelID:   startMsg.Start.ChannelId,
		ChannelName: startMsg.Start.ChannelName,
		State:       types.JobStateRunning,
		StartedAt:   time.Now(),
		Config: &types.TranscodeConfig{
			SourceVideoCodec: startMsg.Start.SourceVideoCodec,
			SourceAudioCodec: startMsg.Start.SourceAudioCodec,
			TargetVideoCodec: startMsg.Start.TargetVideoCodec,
			TargetAudioCodec: startMsg.Start.TargetAudioCodec,
			VideoBitrateKbps: int(startMsg.Start.VideoBitrateKbps),
			AudioBitrateKbps: int(startMsg.Start.AudioBitrateKbps),
			VideoPreset:      startMsg.Start.VideoPreset,
			PreferredHWAccel: startMsg.Start.PreferredHwAccel,
		},
		Stats: &types.TranscodeStats{},
	}

	s.mu.Lock()
	s.activeJobs[jobID] = jobRecord
	s.mu.Unlock()

	// Create the actual transcoding job
	transcodeJob := NewTranscodeJob(
		jobID,
		startMsg.Start,
		binInfo,
		s.logger.With(slog.String("job_id", jobID)),
	)

	// Create a context for this job
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	// Start the FFmpeg process
	ack, err := transcodeJob.Start(ctx)
	if err != nil {
		s.logger.Error("failed to start transcode job",
			slog.String("job_id", jobID),
			slog.String("error", err.Error()),
		)
		// Send error back
		if sendErr := stream.Send(&proto.TranscodeMessage{
			Payload: &proto.TranscodeMessage_Error{
				Error: &proto.TranscodeError{
					Message: fmt.Sprintf("failed to start FFmpeg: %v", err),
					Code:    proto.TranscodeError_FFMPEG_START_FAILED,
				},
			},
		}); sendErr != nil {
			s.logger.Error("failed to send error message", slog.String("error", sendErr.Error()))
		}
		return status.Errorf(codes.Internal, "failed to start FFmpeg: %v", err)
	}

	// Send acknowledgment
	if err := stream.Send(&proto.TranscodeMessage{
		Payload: &proto.TranscodeMessage_Ack{
			Ack: ack,
		},
	}); err != nil {
		transcodeJob.Stop()
		return status.Errorf(codes.Internal, "sending ack: %v", err)
	}

	if !ack.Success {
		s.logger.Warn("transcode job start failed",
			slog.String("job_id", jobID),
			slog.String("error", ack.Error),
		)
		return status.Errorf(codes.FailedPrecondition, "transcode start failed: %s", ack.Error)
	}

	s.logger.Info("transcode job FFmpeg started",
		slog.String("job_id", jobID),
		slog.String("video_encoder", ack.ActualVideoEncoder),
		slog.String("audio_encoder", ack.ActualAudioEncoder),
		slog.String("hw_accel", ack.ActualHwAccel),
	)

	defer func() {
		transcodeJob.Stop()

		s.mu.Lock()
		delete(s.activeJobs, jobID)
		if jobRecord.State == types.JobStateRunning {
			jobRecord.State = types.JobStateCompleted
			jobRecord.CompletedAt = time.Now()
			s.totalCompleted++
		}
		s.mu.Unlock()

		s.logger.Info("transcode job ended",
			slog.String("job_id", jobID),
			slog.String("state", jobRecord.State.String()),
			slog.Duration("duration", jobRecord.RunningTime()),
		)
	}()

	// Start goroutine to forward transcoded output to the stream
	outputDone := make(chan error, 1)
	go func() {
		outputDone <- s.forwardTranscodedOutput(stream, transcodeJob)
	}()

	// Start goroutine to periodically send stats
	statsDone := make(chan struct{})
	go func() {
		s.sendStatsLoop(ctx, stream, transcodeJob, jobRecord, statsDone)
	}()
	defer close(statsDone)

	// Main loop: receive samples and feed to transcoder
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-outputDone:
			if err != nil {
				s.logger.Error("output forwarding error",
					slog.String("job_id", jobID),
					slog.String("error", err.Error()),
				)
				return status.Errorf(codes.Internal, "output error: %v", err)
			}
			return nil
		default:
		}

		msg, err := stream.Recv()
		if err != nil {
			s.mu.Lock()
			jobRecord.State = types.JobStateFailed
			jobRecord.Error = err.Error()
			s.totalFailed++
			s.mu.Unlock()
			return err
		}

		switch payload := msg.Payload.(type) {
		case *proto.TranscodeMessage_Samples:
			// Feed samples to FFmpeg
			if err := transcodeJob.ProcessSamples(payload.Samples); err != nil {
				s.logger.Warn("error processing samples",
					slog.String("job_id", jobID),
					slog.String("error", err.Error()),
				)
				// Continue processing - don't fail the job on individual sample errors
			}

		case *proto.TranscodeMessage_Stop:
			s.logger.Info("transcode stop received",
				slog.String("job_id", jobID),
				slog.String("reason", payload.Stop.Reason),
			)
			return nil

		case *proto.TranscodeMessage_Error:
			s.logger.Error("transcode error from coordinator",
				slog.String("job_id", jobID),
				slog.String("message", payload.Error.Message),
			)
			s.mu.Lock()
			jobRecord.State = types.JobStateFailed
			jobRecord.Error = payload.Error.Message
			s.totalFailed++
			s.mu.Unlock()
			return status.Errorf(codes.Aborted, "coordinator error: %s", payload.Error.Message)
		}
	}
}

// forwardTranscodedOutput reads transcoded samples from FFmpeg and sends them to the stream.
func (s *Server) forwardTranscodedOutput(stream grpc.BidiStreamingServer[proto.TranscodeMessage, proto.TranscodeMessage], job *TranscodeJob) error {
	var batchCount uint64
	var bytesSent uint64
	logTicker := time.NewTicker(5 * time.Second)
	defer logTicker.Stop()

	lastBatchCount := uint64(0)
	lastBytesSent := uint64(0)

	for {
		select {
		case <-logTicker.C:
			batchDelta := batchCount - lastBatchCount
			bytesDelta := bytesSent - lastBytesSent
			s.logger.Log(context.Background(), observability.LevelTrace, "gRPC forward throughput",
				slog.Uint64("batches_sent_5s", batchDelta),
				slog.Float64("kbps", float64(bytesDelta)/5.0/1024),
				slog.Int("output_ch_len", len(job.OutputChannel())))
			lastBatchCount = batchCount
			lastBytesSent = bytesSent
		case batch, ok := <-job.OutputChannel():
			if !ok {
				s.logger.Debug("forwardTranscodedOutput exiting: output channel closed",
					slog.Uint64("total_batches", batchCount),
					slog.Uint64("total_bytes", bytesSent))
				return nil
			}

			batchBytes := uint64(0)
			for _, sample := range batch.VideoSamples {
				batchBytes += uint64(len(sample.Data))
			}
			for _, sample := range batch.AudioSamples {
				batchBytes += uint64(len(sample.Data))
			}

			if err := stream.Send(&proto.TranscodeMessage{
				Payload: &proto.TranscodeMessage_Samples{
					Samples: batch,
				},
			}); err != nil {
				s.logger.Debug("forwardTranscodedOutput exiting: gRPC send error",
					slog.String("error", err.Error()),
					slog.Uint64("total_batches", batchCount))
				return fmt.Errorf("sending samples: %w", err)
			}
			batchCount++
			bytesSent += batchBytes
		}
	}
}

// sendStatsLoop periodically sends transcode stats over the stream.
func (s *Server) sendStatsLoop(ctx context.Context, stream grpc.BidiStreamingServer[proto.TranscodeMessage, proto.TranscodeMessage], transcodeJob *TranscodeJob, jobRecord *types.TranscodeJob, done chan struct{}) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			if transcodeJob.IsClosed() {
				return
			}

			stats := transcodeJob.Stats()
			stats.RunningTime = durationpb.New(jobRecord.RunningTime())

			// Update job record stats (including per-process CPU/memory)
			s.mu.Lock()
			jobRecord.Stats.SamplesIn = stats.SamplesIn
			jobRecord.Stats.SamplesOut = stats.SamplesOut
			jobRecord.Stats.BytesIn = stats.BytesIn
			jobRecord.Stats.BytesOut = stats.BytesOut
			jobRecord.Stats.EncodingSpeed = stats.EncodingSpeed
			jobRecord.Stats.CPUPercent = stats.CpuPercent
			jobRecord.Stats.MemoryMB = stats.MemoryMb
			jobRecord.Stats.FFmpegPID = int(stats.FfmpegPid)
			s.mu.Unlock()

			if err := stream.Send(&proto.TranscodeMessage{
				Payload: &proto.TranscodeMessage_Stats{
					Stats: stats,
				},
			}); err != nil {
				s.logger.Debug("failed to send stats",
					slog.String("error", err.Error()),
				)
				return
			}
		}
	}
}

// GetStats returns current daemon statistics.
func (s *Server) GetStats(ctx context.Context, req *proto.GetStatsRequest) (*proto.GetStatsResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Collect system stats
	var systemStats *proto.SystemStats
	if s.statsCollector != nil {
		stats, err := s.statsCollector.Collect(ctx)
		if err == nil {
			systemStats = stats
		}
	}

	// Build active jobs list
	var activeJobs []*proto.JobStatus
	for jobID, job := range s.activeJobs {
		activeJobs = append(activeJobs, &proto.JobStatus{
			JobId:       string(jobID),
			SessionId:   job.SessionID,
			ChannelName: job.ChannelName,
			RunningTime: durationpb.New(job.RunningTime()),
			Stats: &proto.TranscodeStats{
				SamplesIn:     uint64(job.Stats.SamplesIn),
				SamplesOut:    uint64(job.Stats.SamplesOut),
				BytesIn:       job.Stats.BytesIn,
				BytesOut:      job.Stats.BytesOut,
				EncodingSpeed: job.Stats.EncodingSpeed,
				CpuPercent:    job.Stats.CPUPercent,
				MemoryMb:      job.Stats.MemoryMB,
				FfmpegPid:     int32(job.Stats.FFmpegPID),
			},
		})
	}

	return &proto.GetStatsResponse{
		Capabilities:       s.capabilities,
		SystemStats:        systemStats,
		ActiveJobs:         activeJobs,
		TotalJobsCompleted: s.totalCompleted,
		TotalJobsFailed:    s.totalFailed,
	}, nil
}

// GetCapabilities returns the detected capabilities.
func (s *Server) GetCapabilities() *proto.Capabilities {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.capabilities
}

// GetBinaryInfo returns the FFmpeg binary information.
func (s *Server) GetBinaryInfo() *ffmpeg.BinaryInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.binInfo
}

// GetState returns the current daemon state.
func (s *Server) GetState() types.DaemonState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// unaryInterceptor adds logging to unary RPCs.
func (s *Server) unaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	duration := time.Since(start)

	if err != nil {
		s.logger.Debug("gRPC call failed",
			slog.String("method", info.FullMethod),
			slog.Duration("duration", duration),
			slog.String("error", err.Error()),
		)
	} else {
		s.logger.Debug("gRPC call completed",
			slog.String("method", info.FullMethod),
			slog.Duration("duration", duration),
		)
	}

	return resp, err
}

// streamInterceptor adds logging to streaming RPCs.
func (s *Server) streamInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
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
