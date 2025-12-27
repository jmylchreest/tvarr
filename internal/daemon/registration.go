package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jmylchreest/tvarr/internal/version"
	"github.com/jmylchreest/tvarr/pkg/ffmpeg"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"
)

// RegistrationClient handles registration with the coordinator.
type RegistrationClient struct {
	logger *slog.Logger
	config *RegistrationConfig

	mu             sync.RWMutex
	conn           *grpc.ClientConn
	client         proto.FFmpegDaemonClient
	state          types.DaemonState
	registered     bool
	capabilities   *proto.Capabilities
	statsCollector *StatsCollector

	// Heartbeat control
	heartbeatInterval time.Duration
	heartbeatCancel   context.CancelFunc

	// Active jobs for heartbeat reporting
	activeJobs map[string]*types.TranscodeJob

	// Reconnect settings
	reconnectDelay    time.Duration
	reconnectMaxDelay time.Duration
	reconnectAttempts int
}

// RegistrationConfig holds configuration for coordinator registration.
type RegistrationConfig struct {
	DaemonID          string
	DaemonName        string
	CoordinatorURL    string
	AuthToken         string
	MaxConcurrentJobs int
	HeartbeatInterval time.Duration
	ReconnectDelay    time.Duration
	ReconnectMaxDelay time.Duration
}

// NewRegistrationClient creates a new registration client.
func NewRegistrationClient(logger *slog.Logger, cfg *RegistrationConfig) *RegistrationClient {
	if cfg.HeartbeatInterval == 0 {
		cfg.HeartbeatInterval = 5 * time.Second
	}
	if cfg.ReconnectDelay == 0 {
		cfg.ReconnectDelay = 5 * time.Second
	}
	if cfg.ReconnectMaxDelay == 0 {
		cfg.ReconnectMaxDelay = 60 * time.Second
	}
	if cfg.MaxConcurrentJobs == 0 {
		cfg.MaxConcurrentJobs = 4
	}

	return &RegistrationClient{
		logger:            logger,
		config:            cfg,
		state:             types.DaemonStateDisconnected,
		activeJobs:        make(map[string]*types.TranscodeJob),
		heartbeatInterval: cfg.HeartbeatInterval,
		reconnectDelay:    cfg.ReconnectDelay,
		reconnectMaxDelay: cfg.ReconnectMaxDelay,
	}
}

// SetCapabilities sets the capabilities to report during registration.
func (c *RegistrationClient) SetCapabilities(caps *proto.Capabilities) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.capabilities = caps
	c.capabilities.MaxConcurrentJobs = int32(c.config.MaxConcurrentJobs)
}

// SetStatsCollector sets the stats collector for heartbeat reporting.
func (c *RegistrationClient) SetStatsCollector(collector *StatsCollector) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.statsCollector = collector
}

// Connect establishes connection to the coordinator.
func (c *RegistrationClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	c.state = types.DaemonStateConnecting
	c.mu.Unlock()

	c.logger.Info("connecting to coordinator",
		slog.String("url", c.config.CoordinatorURL),
	)

	// Establish gRPC connection
	conn, err := grpc.NewClient(c.config.CoordinatorURL,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		c.mu.Lock()
		c.state = types.DaemonStateDisconnected
		c.mu.Unlock()
		return fmt.Errorf("connecting to coordinator: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.client = proto.NewFFmpegDaemonClient(conn)
	c.mu.Unlock()

	return nil
}

// Register registers this daemon with the coordinator.
func (c *RegistrationClient) Register(ctx context.Context) error {
	c.mu.RLock()
	client := c.client
	caps := c.capabilities
	c.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("not connected to coordinator")
	}

	if caps == nil {
		return fmt.Errorf("capabilities not set")
	}

	c.logger.Info("registering with coordinator",
		slog.String("daemon_id", c.config.DaemonID),
		slog.String("daemon_name", c.config.DaemonName),
	)

	req := &proto.RegisterRequest{
		DaemonId:     c.config.DaemonID,
		DaemonName:   c.config.DaemonName,
		Version:      version.Short(),
		AuthToken:    c.config.AuthToken,
		Capabilities: caps,
	}

	resp, err := client.Register(ctx, req)
	if err != nil {
		c.mu.Lock()
		c.state = types.DaemonStateDisconnected
		c.mu.Unlock()
		return fmt.Errorf("registration failed: %w", err)
	}

	if !resp.Success {
		c.mu.Lock()
		c.state = types.DaemonStateDisconnected
		c.mu.Unlock()
		return fmt.Errorf("registration rejected: %s", resp.Error)
	}

	c.mu.Lock()
	c.registered = true
	c.state = types.DaemonStateConnected
	if resp.HeartbeatInterval != nil {
		c.heartbeatInterval = resp.HeartbeatInterval.AsDuration()
	}
	c.mu.Unlock()

	c.logger.Info("registered with coordinator",
		slog.String("coordinator_version", resp.CoordinatorVersion),
		slog.Duration("heartbeat_interval", c.heartbeatInterval),
	)

	return nil
}

// StartHeartbeat starts the heartbeat loop.
func (c *RegistrationClient) StartHeartbeat(ctx context.Context) {
	c.mu.Lock()
	if c.heartbeatCancel != nil {
		c.heartbeatCancel()
	}
	heartbeatCtx, cancel := context.WithCancel(ctx)
	c.heartbeatCancel = cancel
	interval := c.heartbeatInterval
	c.mu.Unlock()

	go c.heartbeatLoop(heartbeatCtx, interval)
}

// heartbeatLoop sends periodic heartbeats to the coordinator.
func (c *RegistrationClient) heartbeatLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	consecutiveFailures := 0
	const maxConsecutiveFailures = 3

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.sendHeartbeat(ctx); err != nil {
				consecutiveFailures++
				c.logger.Warn("heartbeat failed",
					slog.String("error", err.Error()),
					slog.Int("consecutive_failures", consecutiveFailures),
				)

				// After multiple consecutive failures, attempt reconnection
				if consecutiveFailures >= maxConsecutiveFailures {
					c.logger.Warn("too many heartbeat failures, attempting reconnection",
						slog.Int("failures", consecutiveFailures),
					)

					// Mark as unhealthy during reconnection
					c.mu.Lock()
					c.state = types.DaemonStateUnhealthy
					c.mu.Unlock()

					// Try to reconnect with exponential backoff
					if err := c.reconnect(ctx); err != nil {
						c.logger.Error("reconnection failed, will keep trying",
							slog.String("error", err.Error()),
						)
						// Continue the loop, we'll try again on next tick
					} else {
						c.logger.Info("reconnection successful")
						consecutiveFailures = 0
					}
				}
			} else {
				// Reset failure count on successful heartbeat
				if consecutiveFailures > 0 {
					c.logger.Info("heartbeat recovered after failures",
						slog.Int("previous_failures", consecutiveFailures),
					)
				}
				consecutiveFailures = 0
			}
		}
	}
}

// reconnect attempts to reconnect and re-register with exponential backoff.
func (c *RegistrationClient) reconnect(ctx context.Context) error {
	// Close existing connection
	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
		c.client = nil
	}
	c.registered = false
	c.mu.Unlock()

	delay := c.reconnectDelay
	maxAttempts := 5 // Limit reconnection attempts before returning to heartbeat loop

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		c.logger.Info("attempting reconnection",
			slog.Int("attempt", attempt),
			slog.Int("max_attempts", maxAttempts),
			slog.Duration("delay", delay),
		)

		// Try to connect
		if err := c.Connect(ctx); err != nil {
			c.logger.Warn("reconnection connect failed",
				slog.String("error", err.Error()),
				slog.Int("attempt", attempt),
			)
			time.Sleep(delay)
			delay = min(delay*2, c.reconnectMaxDelay)
			continue
		}

		// Try to register
		if err := c.Register(ctx); err != nil {
			c.logger.Warn("reconnection register failed",
				slog.String("error", err.Error()),
				slog.Int("attempt", attempt),
			)
			c.mu.Lock()
			if c.conn != nil {
				c.conn.Close()
				c.conn = nil
				c.client = nil
			}
			c.mu.Unlock()
			time.Sleep(delay)
			delay = min(delay*2, c.reconnectMaxDelay)
			continue
		}

		// Success - state already set to Active in Register()
		c.mu.Lock()
		c.reconnectAttempts = 0
		c.mu.Unlock()
		return nil
	}

	return fmt.Errorf("reconnection failed after %d attempts", maxAttempts)
}

// sendHeartbeat sends a single heartbeat to the coordinator.
func (c *RegistrationClient) sendHeartbeat(ctx context.Context) error {
	c.mu.RLock()
	client := c.client
	statsCollector := c.statsCollector
	activeJobs := c.activeJobs
	c.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("not connected")
	}

	// Collect system stats
	var systemStats *proto.SystemStats
	if statsCollector != nil {
		stats, err := statsCollector.Collect(ctx)
		if err == nil {
			systemStats = stats
		}
	}

	// Build active jobs list
	var jobs []*proto.JobStatus
	for jobID, job := range activeJobs {
		jobs = append(jobs, &proto.JobStatus{
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

	req := &proto.HeartbeatRequest{
		DaemonId:    c.config.DaemonID,
		SystemStats: systemStats,
		ActiveJobs:  jobs,
	}

	resp, err := client.Heartbeat(ctx, req)
	if err != nil {
		return fmt.Errorf("heartbeat RPC failed: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("heartbeat rejected")
	}

	// Process any commands from coordinator
	if len(resp.Commands) > 0 {
		for _, cmd := range resp.Commands {
			c.processCommand(cmd)
		}
	}

	return nil
}

// processCommand handles commands from the coordinator.
func (c *RegistrationClient) processCommand(cmd *proto.DaemonCommand) {
	switch cmd.Type {
	case proto.DaemonCommand_DRAIN:
		c.logger.Info("received DRAIN command from coordinator")
		c.mu.Lock()
		c.state = types.DaemonStateDraining
		c.mu.Unlock()

	case proto.DaemonCommand_RESUME:
		c.logger.Info("received RESUME command from coordinator")
		c.mu.Lock()
		c.state = types.DaemonStateConnected
		c.mu.Unlock()

	case proto.DaemonCommand_CANCEL_JOB:
		c.logger.Info("received CANCEL_JOB command",
			slog.String("job_id", cmd.JobId),
		)
		// TODO: Implement job cancellation

	case proto.DaemonCommand_UPDATE_CONFIG:
		c.logger.Info("received UPDATE_CONFIG command")
		// TODO: Implement config update
	}
}

// Unregister gracefully unregisters from the coordinator.
func (c *RegistrationClient) Unregister(ctx context.Context, reason string) error {
	c.mu.Lock()
	if c.heartbeatCancel != nil {
		c.heartbeatCancel()
	}
	client := c.client
	c.registered = false
	c.state = types.DaemonStateDraining
	c.mu.Unlock()

	if client == nil {
		return nil
	}

	c.logger.Info("unregistering from coordinator",
		slog.String("reason", reason),
	)

	req := &proto.UnregisterRequest{
		DaemonId: c.config.DaemonID,
		Reason:   reason,
	}

	_, err := client.Unregister(ctx, req)
	if err != nil {
		c.logger.Warn("unregister RPC failed",
			slog.String("error", err.Error()),
		)
	}

	return nil
}

// Close closes the connection to the coordinator.
func (c *RegistrationClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.heartbeatCancel != nil {
		c.heartbeatCancel()
	}

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			return fmt.Errorf("closing connection: %w", err)
		}
		c.conn = nil
		c.client = nil
	}

	c.state = types.DaemonStateDisconnected
	return nil
}

// GetState returns the current registration state.
func (c *RegistrationClient) GetState() types.DaemonState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// IsRegistered returns true if the daemon is registered.
func (c *RegistrationClient) IsRegistered() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.registered
}

// ConnectAndRegister connects and registers with automatic retry.
func (c *RegistrationClient) ConnectAndRegister(ctx context.Context) error {
	delay := c.reconnectDelay

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Try to connect
		if err := c.Connect(ctx); err != nil {
			c.logger.Warn("connection failed, retrying",
				slog.String("error", err.Error()),
				slog.Duration("delay", delay),
			)
			time.Sleep(delay)
			delay = min(delay*2, c.reconnectMaxDelay)
			continue
		}

		// Try to register
		if err := c.Register(ctx); err != nil {
			c.logger.Warn("registration failed, retrying",
				slog.String("error", err.Error()),
				slog.Duration("delay", delay),
			)
			c.Close()
			time.Sleep(delay)
			delay = min(delay*2, c.reconnectMaxDelay)
			continue
		}

		// Success - start heartbeat and return
		c.StartHeartbeat(ctx)
		return nil
	}
}

// TrackJob adds a job to the active jobs map for heartbeat reporting.
func (c *RegistrationClient) TrackJob(job *types.TranscodeJob) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.activeJobs[string(job.ID)] = job
}

// UntrackJob removes a job from the active jobs map.
func (c *RegistrationClient) UntrackJob(jobID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.activeJobs, jobID)
}

// TranscodeStreamHandler handles the persistent transcode stream to the coordinator.
type TranscodeStreamHandler struct {
	logger   *slog.Logger
	client   proto.FFmpegDaemonClient
	daemonID string
	binInfo  *ffmpeg.BinaryInfo

	mu         sync.RWMutex
	stream     proto.FFmpegDaemon_TranscodeClient
	activeJob  *TranscodeJob
	cancelFunc context.CancelFunc

	// sendMu protects stream.Send() calls - gRPC streams are not thread-safe for concurrent sends
	sendMu sync.Mutex
}

// NewTranscodeStreamHandler creates a new transcode stream handler.
func NewTranscodeStreamHandler(
	logger *slog.Logger,
	client proto.FFmpegDaemonClient,
	daemonID string,
	binInfo *ffmpeg.BinaryInfo,
) *TranscodeStreamHandler {
	return &TranscodeStreamHandler{
		logger:   logger,
		client:   client,
		daemonID: daemonID,
		binInfo:  binInfo,
	}
}

// Start opens the persistent transcode stream and begins processing jobs.
func (h *TranscodeStreamHandler) Start(ctx context.Context) error {
	streamCtx, cancel := context.WithCancel(ctx)
	h.cancelFunc = cancel

	// Open the transcode stream
	stream, err := h.client.Transcode(streamCtx)
	if err != nil {
		cancel()
		return fmt.Errorf("opening transcode stream: %w", err)
	}

	h.mu.Lock()
	h.stream = stream
	h.mu.Unlock()

	// Send the "ready" message identifying this daemon
	readyMsg := &proto.TranscodeMessage{
		Payload: &proto.TranscodeMessage_Start{
			Start: &proto.TranscodeStart{
				SessionId: h.daemonID, // Coordinator expects daemon_id in session_id field
				JobId:     "",         // Empty job_id signals "ready" state
			},
		},
	}

	if err := stream.Send(readyMsg); err != nil {
		cancel()
		return fmt.Errorf("sending ready message: %w", err)
	}

	h.logger.Info("Transcode stream connected to coordinator",
		slog.String("daemon_id", h.daemonID),
	)

	// Start the message processing loop in a goroutine
	go h.processMessages(streamCtx)

	return nil
}

// Stop closes the transcode stream.
func (h *TranscodeStreamHandler) Stop() {
	h.mu.Lock()
	if h.cancelFunc != nil {
		h.cancelFunc()
	}
	if h.activeJob != nil {
		h.activeJob.Stop()
		h.activeJob = nil
	}
	h.mu.Unlock()
}

// processMessages handles incoming messages from the coordinator.
func (h *TranscodeStreamHandler) processMessages(ctx context.Context) {
	defer func() {
		h.mu.Lock()
		if h.activeJob != nil {
			h.activeJob.Stop()
			h.activeJob = nil
		}
		h.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		h.mu.RLock()
		stream := h.stream
		h.mu.RUnlock()

		if stream == nil {
			return
		}

		msg, err := stream.Recv()
		if err != nil {
			h.logger.Debug("Transcode stream ended",
				slog.String("daemon_id", h.daemonID),
				slog.String("error", err.Error()),
			)
			return
		}

		switch payload := msg.Payload.(type) {
		case *proto.TranscodeMessage_Start:
			// Coordinator is starting a new transcode job
			h.handleTranscodeStart(ctx, payload.Start)

		case *proto.TranscodeMessage_Samples:
			// Source samples from coordinator - feed to active job
			h.handleSourceSamples(payload.Samples)

		case *proto.TranscodeMessage_Stop:
			// Coordinator signaling to stop current job (forced shutdown)
			h.handleStop(payload.Stop)

		case *proto.TranscodeMessage_InputComplete:
			// Coordinator signaling input is complete (graceful shutdown)
			h.handleInputComplete(payload.InputComplete)

		case *proto.TranscodeMessage_ProbeRequest:
			// Coordinator requesting stream probe
			h.handleProbeRequest(ctx, payload.ProbeRequest)
		}
	}
}

// handleTranscodeStart starts a new transcode job.
func (h *TranscodeStreamHandler) handleTranscodeStart(ctx context.Context, start *proto.TranscodeStart) {
	h.logger.Info("Received transcode job from coordinator",
		slog.String("job_id", start.JobId),
		slog.String("channel", start.ChannelName),
		slog.String("target_video", start.TargetVideoCodec),
		slog.String("target_audio", start.TargetAudioCodec),
	)

	h.mu.Lock()
	// Stop any existing job
	if h.activeJob != nil {
		h.activeJob.Stop()
	}

	// Create new transcode job with proper binInfo
	h.activeJob = NewTranscodeJob(start.JobId, start, h.binInfo, h.logger)
	h.mu.Unlock()

	// Start the job
	ack, err := h.activeJob.Start(ctx)
	if err != nil {
		h.logger.Error("Failed to start transcode job",
			slog.String("job_id", start.JobId),
			slog.String("error", err.Error()),
		)
		// Send error back to coordinator
		h.sendError(proto.TranscodeError_FFMPEG_START_FAILED, err.Error())
		return
	}

	// Check if job start succeeded (ack may have Success=false even with nil error)
	if !ack.Success {
		h.logger.Error("Transcode job start failed",
			slog.String("job_id", start.JobId),
			slog.String("error", ack.Error),
		)
		h.sendError(proto.TranscodeError_FFMPEG_START_FAILED, ack.Error)
		return
	}

	// Send acknowledgment to coordinator
	h.mu.RLock()
	stream := h.stream
	h.mu.RUnlock()

	if stream != nil {
		h.sendMu.Lock()
		err := stream.Send(&proto.TranscodeMessage{
			Payload: &proto.TranscodeMessage_Ack{
				Ack: ack,
			},
		})
		h.sendMu.Unlock()
		if err != nil {
			h.logger.Error("Failed to send ack",
				slog.String("job_id", start.JobId),
				slog.String("error", err.Error()),
			)
		}
	}

	// Start forwarding transcoded output to coordinator
	go h.forwardTranscodedOutput(ctx, start.JobId)

	// Start sending stats to coordinator
	go h.sendStatsLoop(ctx, start.JobId)
}

// handleSourceSamples feeds source samples to the active transcode job.
func (h *TranscodeStreamHandler) handleSourceSamples(samples *proto.ESSampleBatch) {
	h.mu.RLock()
	job := h.activeJob
	h.mu.RUnlock()

	if job == nil {
		h.logger.Warn("Received samples but no active job")
		return
	}

	if err := job.ProcessSamples(samples); err != nil {
		h.logger.Warn("Error processing samples",
			slog.String("error", err.Error()),
		)
	}
}

// handleStop stops the current transcode job (forced shutdown).
func (h *TranscodeStreamHandler) handleStop(stop *proto.TranscodeStop) {
	h.logger.Info("Received stop signal from coordinator",
		slog.String("reason", stop.Reason),
	)

	h.mu.Lock()
	if h.activeJob != nil {
		h.activeJob.Stop()
		h.activeJob = nil
	}
	h.mu.Unlock()
}

// handleInputComplete signals that input is complete (graceful shutdown).
// Unlike handleStop, this allows FFmpeg to flush its encoder before stopping.
func (h *TranscodeStreamHandler) handleInputComplete(inputComplete *proto.TranscodeInputComplete) {
	h.logger.Info("Received input complete signal from coordinator",
		slog.String("reason", inputComplete.Reason),
	)

	h.mu.RLock()
	job := h.activeJob
	h.mu.RUnlock()

	if job != nil {
		job.SignalInputComplete()
	}
}

// forwardTranscodedOutput reads transcoded samples from the job and sends them to coordinator.
func (h *TranscodeStreamHandler) forwardTranscodedOutput(ctx context.Context, jobID string) {
	h.mu.RLock()
	job := h.activeJob
	stream := h.stream
	h.mu.RUnlock()

	if job == nil || stream == nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case batch, ok := <-job.OutputChannel():
			if !ok {
				// Job output channel closed, job is done
				h.sendStop("job_completed")
				return
			}

			h.sendMu.Lock()
			err := stream.Send(&proto.TranscodeMessage{
				Payload: &proto.TranscodeMessage_Samples{
					Samples: batch,
				},
			})
			h.sendMu.Unlock()
			if err != nil {
				h.logger.Error("Failed to send transcoded samples",
					slog.String("job_id", jobID),
					slog.String("error", err.Error()),
				)
				return
			}
		}
	}
}

// sendError sends an error message to the coordinator.
func (h *TranscodeStreamHandler) sendError(code proto.TranscodeError_ErrorCode, message string) {
	h.mu.RLock()
	stream := h.stream
	h.mu.RUnlock()

	if stream != nil {
		h.sendMu.Lock()
		_ = stream.Send(&proto.TranscodeMessage{
			Payload: &proto.TranscodeMessage_Error{
				Error: &proto.TranscodeError{
					Code:    code,
					Message: message,
				},
			},
		})
		h.sendMu.Unlock()
	}
}

// sendStop sends a stop message to the coordinator.
func (h *TranscodeStreamHandler) sendStop(reason string) {
	h.mu.RLock()
	stream := h.stream
	h.mu.RUnlock()

	if stream != nil {
		h.sendMu.Lock()
		_ = stream.Send(&proto.TranscodeMessage{
			Payload: &proto.TranscodeMessage_Stop{
				Stop: &proto.TranscodeStop{
					Reason: reason,
				},
			},
		})
		h.sendMu.Unlock()
	}
}

// sendStatsLoop periodically sends transcode stats to the coordinator.
func (h *TranscodeStreamHandler) sendStatsLoop(ctx context.Context, jobID string) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.mu.RLock()
			job := h.activeJob
			stream := h.stream
			h.mu.RUnlock()

			if job == nil || stream == nil {
				return
			}

			if job.IsClosed() {
				return
			}

			stats := job.Stats()
			stats.RunningTime = durationpb.New(time.Since(startTime))

			h.sendMu.Lock()
			err := stream.Send(&proto.TranscodeMessage{
				Payload: &proto.TranscodeMessage_Stats{
					Stats: stats,
				},
			})
			h.sendMu.Unlock()
			if err != nil {
				h.logger.Debug("failed to send stats",
					slog.String("job_id", jobID),
					slog.String("error", err.Error()),
				)
				return
			}
		}
	}
}

// StartTranscodeStream starts the persistent transcode stream after registration.
// Call this after ConnectAndRegister succeeds.
func (c *RegistrationClient) StartTranscodeStream(ctx context.Context, binInfo *ffmpeg.BinaryInfo) error {
	c.mu.RLock()
	client := c.client
	c.mu.RUnlock()

	if client == nil {
		return fmt.Errorf("not connected to coordinator")
	}

	handler := NewTranscodeStreamHandler(c.logger, client, c.config.DaemonID, binInfo)
	return handler.Start(ctx)
}

// handleProbeRequest handles a probe request from the coordinator.
// Uses local ffprobe to probe the stream and sends the result back.
func (h *TranscodeStreamHandler) handleProbeRequest(ctx context.Context, req *proto.ProbeRequest) {
	h.logger.Debug("Received probe request from coordinator",
		slog.String("stream_url", req.StreamUrl),
		slog.Int("timeout_ms", int(req.TimeoutMs)),
	)

	start := time.Now()

	// Check if ffprobe is available
	if h.binInfo == nil || h.binInfo.FFprobePath == "" {
		h.sendProbeResponse(&proto.ProbeResponse{
			Success:         false,
			Error:           "ffprobe not available on this daemon",
			ProbeDurationMs: int32(time.Since(start).Milliseconds()),
		})
		return
	}

	// Create prober with the configured timeout
	prober := ffmpeg.NewProber(h.binInfo.FFprobePath)
	if req.TimeoutMs > 0 {
		prober = prober.WithTimeout(time.Duration(req.TimeoutMs) * time.Millisecond)
	}

	// Probe the stream
	info, err := prober.ProbeSimple(ctx, req.StreamUrl)
	if err != nil {
		h.logger.Warn("Probe failed",
			slog.String("stream_url", req.StreamUrl),
			slog.String("error", err.Error()),
			slog.Duration("duration", time.Since(start)),
		)
		h.sendProbeResponse(&proto.ProbeResponse{
			Success:         false,
			Error:           err.Error(),
			ProbeDurationMs: int32(time.Since(start).Milliseconds()),
		})
		return
	}

	h.logger.Info("Probe completed",
		slog.String("stream_url", req.StreamUrl),
		slog.String("video_codec", info.VideoCodec),
		slog.String("audio_codec", info.AudioCodec),
		slog.Duration("duration", time.Since(start)),
	)

	h.sendProbeResponse(&proto.ProbeResponse{
		Success:         true,
		VideoCodec:      info.VideoCodec,
		VideoProfile:    info.VideoProfile,
		VideoWidth:      int32(info.VideoWidth),
		VideoHeight:     int32(info.VideoHeight),
		VideoFramerate:  info.VideoFramerate,
		VideoBitrateBps: int64(info.VideoBitrate),
		AudioCodec:      info.AudioCodec,
		AudioChannels:   int32(info.AudioChannels),
		AudioSampleRate: int32(info.AudioSampleRate),
		AudioBitrateBps: int64(info.AudioBitrate),
		ContainerFormat: info.ContainerFormat,
		IsLiveStream:    info.IsLiveStream,
		ProbeDurationMs: int32(time.Since(start).Milliseconds()),
	})
}

// sendProbeResponse sends a probe response to the coordinator.
func (h *TranscodeStreamHandler) sendProbeResponse(resp *proto.ProbeResponse) {
	h.mu.RLock()
	stream := h.stream
	h.mu.RUnlock()

	if stream != nil {
		h.sendMu.Lock()
		_ = stream.Send(&proto.TranscodeMessage{
			Payload: &proto.TranscodeMessage_ProbeResponse{
				ProbeResponse: resp,
			},
		})
		h.sendMu.Unlock()
	}
}
