package relay

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
	"google.golang.org/grpc"
)

// JobType indicates the type of job for slot tracking purposes.
type JobType int

const (
	JobTypeCPU   JobType = iota // Software encoding (libx264, libx265, etc.)
	JobTypeGPU                  // Hardware encoding (NVENC, VAAPI, QSV, etc.)
	JobTypeProbe                // Lightweight probe operations (ffprobe)
)

func (jt JobType) String() string {
	switch jt {
	case JobTypeCPU:
		return "cpu"
	case JobTypeGPU:
		return "gpu"
	case JobTypeProbe:
		return "probe"
	default:
		return "unknown"
	}
}

// IsEncoding returns true if this job type involves actual encoding.
func (jt JobType) IsEncoding() bool {
	return jt == JobTypeCPU || jt == JobTypeGPU
}

// DaemonStream wraps a bidirectional gRPC stream to a daemon.
// The daemon opens this stream after registration and keeps it open.
// The coordinator can push transcode jobs through it.
// Supports multiple concurrent transcode jobs per stream with separate
// tracking for CPU, GPU, and probe jobs.
type DaemonStream struct {
	DaemonID types.DaemonID
	Stream   grpc.BidiStreamingServer[proto.TranscodeMessage, proto.TranscodeMessage]
	Logger   *slog.Logger

	// Capacity limits for encoding jobs
	MaxJobs    int // Maximum total concurrent encoding jobs (safety cap)
	MaxCPUJobs int // Maximum concurrent CPU (software) encoding jobs
	MaxGPUJobs int // Maximum concurrent GPU (hardware) encoding jobs (sum of all GPU sessions)

	// Probe capacity (lightweight operations, separate from encoding)
	MaxProbeJobs int // Maximum concurrent probe jobs (typically 2-3x MaxJobs)

	mu         sync.Mutex
	closed     bool
	activeJobs map[string]JobType // Active encoding job IDs with their type (CPU or GPU)

	// Pending probe requests waiting for responses
	pendingProbes map[string]chan *proto.ProbeResponse
}

// Send sends a message to the daemon through the stream.
func (s *DaemonStream) Send(msg *proto.TranscodeMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return errors.New("stream closed")
	}

	return s.Stream.Send(msg)
}

// IsIdle returns true if the stream has no active jobs.
func (s *DaemonStream) IsIdle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.activeJobs) == 0
}

// HasCapacity returns true if the stream can accept more jobs of any type.
// For type-specific capacity, use HasCPUCapacity or HasGPUCapacity.
func (s *DaemonStream) HasCapacity() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hasCapacityLocked(JobTypeCPU) || s.hasCapacityLocked(JobTypeGPU)
}

// HasCapacityForType returns true if the stream can accept a job of the given type.
func (s *DaemonStream) HasCapacityForType(jobType JobType) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hasCapacityLocked(jobType)
}

// HasCPUCapacity returns true if the stream can accept more CPU jobs.
func (s *DaemonStream) HasCPUCapacity() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hasCapacityLocked(JobTypeCPU)
}

// HasGPUCapacity returns true if the stream can accept more GPU jobs.
func (s *DaemonStream) HasGPUCapacity() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hasCapacityLocked(JobTypeGPU)
}

// hasCapacityLocked checks capacity without acquiring lock (caller must hold lock).
// MaxJobs acts as the overall guard - total active jobs cannot exceed it.
// Individual limits (MaxCPUJobs, MaxGPUJobs) provide type-specific constraints.
func (s *DaemonStream) hasCapacityLocked(jobType JobType) bool {
	if s.closed {
		return false
	}

	totalJobs := len(s.activeJobs)

	// Check total job limit - this is the overall guard
	maxJobs := s.MaxJobs
	if maxJobs <= 0 {
		maxJobs = 4 // Default safety cap
	}
	if totalJobs >= maxJobs {
		return false
	}

	// Count jobs by type
	cpuJobs, gpuJobs := s.countJobsByTypeLocked()

	switch jobType {
	case JobTypeCPU:
		maxCPU := s.MaxCPUJobs
		if maxCPU <= 0 {
			// No explicit limit - allow up to maxJobs
			maxCPU = maxJobs
		}
		return cpuJobs < maxCPU

	case JobTypeGPU:
		maxGPU := s.MaxGPUJobs
		if maxGPU <= 0 {
			return false // No GPU capacity if not set (no GPUs detected)
		}
		return gpuJobs < maxGPU

	default:
		return false
	}
}

// countJobsByTypeLocked counts active CPU and GPU jobs (caller must hold lock).
func (s *DaemonStream) countJobsByTypeLocked() (cpu, gpu int) {
	for _, jt := range s.activeJobs {
		switch jt {
		case JobTypeCPU:
			cpu++
		case JobTypeGPU:
			gpu++
		}
	}
	return
}

// ActiveJobCount returns the number of active jobs on this stream.
func (s *DaemonStream) ActiveJobCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.activeJobs)
}

// ActiveCPUJobCount returns the number of active CPU jobs.
func (s *DaemonStream) ActiveCPUJobCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	cpu, _ := s.countJobsByTypeLocked()
	return cpu
}

// ActiveGPUJobCount returns the number of active GPU jobs.
func (s *DaemonStream) ActiveGPUJobCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, gpu := s.countJobsByTypeLocked()
	return gpu
}

// CPULoadPercent returns the CPU slot utilization as a percentage (0.0 to 1.0).
func (s *DaemonStream) CPULoadPercent() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	maxCPU := s.MaxCPUJobs
	if maxCPU <= 0 {
		// Default to MaxJobs if not explicitly set
		maxCPU = s.MaxJobs
		if maxCPU <= 0 {
			maxCPU = 4
		}
	}
	cpu, _ := s.countJobsByTypeLocked()
	return float64(cpu) / float64(maxCPU)
}

// GPULoadPercent returns the GPU slot utilization as a percentage (0.0 to 1.0).
func (s *DaemonStream) GPULoadPercent() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	maxGPU := s.MaxGPUJobs
	if maxGPU <= 0 {
		return 1.0 // Fully loaded if no capacity set
	}
	_, gpu := s.countJobsByTypeLocked()
	return float64(gpu) / float64(maxGPU)
}

// TotalLoadPercent returns the overall job slot utilization as a percentage.
func (s *DaemonStream) TotalLoadPercent() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	maxJobs := s.MaxJobs
	if maxJobs <= 0 {
		maxJobs = 4
	}
	return float64(len(s.activeJobs)) / float64(maxJobs)
}

// ActiveProbeCount returns the number of active probe operations.
func (s *DaemonStream) ActiveProbeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pendingProbes)
}

// HasProbeCapacity returns true if the stream can accept more probe jobs.
func (s *DaemonStream) HasProbeCapacity() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	maxProbes := s.MaxProbeJobs
	if maxProbes <= 0 {
		// Default to MaxJobs since probes are lightweight
		maxProbes = s.MaxJobs
		if maxProbes <= 0 {
			maxProbes = 4
		}
	}
	return len(s.pendingProbes) < maxProbes
}

// ProbeLoadPercent returns the probe slot utilization as a percentage (0.0 to 1.0).
func (s *DaemonStream) ProbeLoadPercent() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	maxProbes := s.MaxProbeJobs
	if maxProbes <= 0 {
		maxProbes = s.MaxJobs
		if maxProbes <= 0 {
			maxProbes = 4
		}
	}
	return float64(len(s.pendingProbes)) / float64(maxProbes)
}

// AddActiveJob adds a job to the active jobs set (defaults to CPU type).
// Deprecated: Use AddActiveJobWithType for explicit type tracking.
func (s *DaemonStream) AddActiveJob(jobID string) {
	s.AddActiveJobWithType(jobID, JobTypeCPU)
}

// AddActiveJobWithType adds a job with its type to the active jobs set.
func (s *DaemonStream) AddActiveJobWithType(jobID string, jobType JobType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeJobs == nil {
		s.activeJobs = make(map[string]JobType)
	}
	s.activeJobs[jobID] = jobType
}

// RemoveActiveJob removes a job from the active jobs set.
func (s *DaemonStream) RemoveActiveJob(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.activeJobs, jobID)
}

// HasActiveJob returns true if the specified job is active on this stream.
func (s *DaemonStream) HasActiveJob(jobID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists := s.activeJobs[jobID]
	return exists
}

// GetJobType returns the type of an active job, or false if not found.
func (s *DaemonStream) GetJobType(jobID string) (JobType, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	jt, exists := s.activeJobs[jobID]
	return jt, exists
}

// SetActiveJob sets the active job ID (legacy single-job compatibility).
// Deprecated: Use AddActiveJob for multi-job support.
func (s *DaemonStream) SetActiveJob(jobID string) {
	s.AddActiveJob(jobID)
}

// ClearActiveJob clears all active jobs (legacy single-job compatibility).
// Deprecated: Use RemoveActiveJob(jobID) for multi-job support.
func (s *DaemonStream) ClearActiveJob() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeJobs = nil
}

// UpdateJobType updates the type of an active job.
// This is called when the daemon reports the actual encoder used,
// which may differ from the coordinator's prediction.
// Returns true if the job was found and updated, false otherwise.
func (s *DaemonStream) UpdateJobType(jobID string, newType JobType) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.activeJobs == nil {
		return false
	}

	oldType, exists := s.activeJobs[jobID]
	if !exists {
		return false
	}

	if oldType != newType {
		s.activeJobs[jobID] = newType
		if s.Logger != nil {
			s.Logger.Debug("Job type updated from daemon feedback",
				slog.String("job_id", jobID),
				slog.String("old_type", oldType.String()),
				slog.String("new_type", newType.String()),
			)
		}
	}

	return true
}

// Close marks the stream as closed.
func (s *DaemonStream) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true

	// Cancel any pending probes
	for _, ch := range s.pendingProbes {
		close(ch)
	}
	s.pendingProbes = nil
}

// Probe sends a probe request to the daemon and waits for the response.
// Uses the stream URL as the request ID for matching responses.
func (s *DaemonStream) Probe(ctx context.Context, streamURL string, timeoutMs int32) (*proto.ProbeResponse, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, errors.New("stream closed")
	}

	// Create response channel
	if s.pendingProbes == nil {
		s.pendingProbes = make(map[string]chan *proto.ProbeResponse)
	}
	responseCh := make(chan *proto.ProbeResponse, 1)
	s.pendingProbes[streamURL] = responseCh
	s.mu.Unlock()

	// Cleanup on exit
	defer func() {
		s.mu.Lock()
		delete(s.pendingProbes, streamURL)
		s.mu.Unlock()
	}()

	// Send probe request
	err := s.Stream.Send(&proto.TranscodeMessage{
		Payload: &proto.TranscodeMessage_ProbeRequest{
			ProbeRequest: &proto.ProbeRequest{
				StreamUrl: streamURL,
				TimeoutMs: timeoutMs,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	// Wait for response with timeout
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp, ok := <-responseCh:
		if !ok {
			return nil, errors.New("stream closed while waiting for probe response")
		}
		return resp, nil
	}
}

// DeliverProbeResponse delivers a probe response to the waiting caller.
// Returns true if a caller was waiting for this response.
func (s *DaemonStream) DeliverProbeResponse(resp *proto.ProbeResponse, streamURL string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.pendingProbes == nil {
		return false
	}

	ch, ok := s.pendingProbes[streamURL]
	if !ok {
		return false
	}

	select {
	case ch <- resp:
		return true
	default:
		return false
	}
}

// DaemonStreamManager manages active transcode streams from daemons.
type DaemonStreamManager struct {
	logger *slog.Logger

	mu      sync.RWMutex
	streams map[types.DaemonID]*DaemonStream

	// Registry for strategy-based daemon selection
	registry *DaemonRegistry

	// Strategy provider for probe operations
	probeStrategyProvider ProbeStrategyProvider
}

// NewDaemonStreamManager creates a new stream manager.
func NewDaemonStreamManager(logger *slog.Logger, registry *DaemonRegistry) *DaemonStreamManager {
	return &DaemonStreamManager{
		logger:                logger,
		streams:               make(map[types.DaemonID]*DaemonStream),
		registry:              registry,
		probeStrategyProvider: NewDefaultProbeStrategyProvider(),
	}
}

// WithProbeStrategyProvider sets a custom probe strategy provider.
func (m *DaemonStreamManager) WithProbeStrategyProvider(provider ProbeStrategyProvider) *DaemonStreamManager {
	m.probeStrategyProvider = provider
	return m
}

// RegisterStream registers a new daemon stream.
func (m *DaemonStreamManager) RegisterStream(
	daemonID types.DaemonID,
	stream grpc.BidiStreamingServer[proto.TranscodeMessage, proto.TranscodeMessage],
) *DaemonStream {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Close existing stream if any
	if existing, ok := m.streams[daemonID]; ok {
		existing.Close()
		m.logger.Debug("Closed existing daemon stream",
			slog.String("daemon_id", string(daemonID)),
		)
	}

	// Get capacity limits from the daemon's capabilities
	// MaxJobs is the overall safety guard - no combination of jobs can exceed it
	maxJobs := 4    // Default total job limit
	maxCPUJobs := 0 // Default: will be detected from CPU cores
	maxGPUJobs := 0 // Default: will be calculated from GPU sessions
	maxProbeJobs := 0

	if m.registry != nil {
		if daemon, ok := m.registry.Get(daemonID); ok && daemon.Capabilities != nil {
			caps := daemon.Capabilities

			// Total job limit (the guard limit)
			if caps.MaxConcurrentJobs > 0 {
				maxJobs = caps.MaxConcurrentJobs
			}

			// CPU job limit - priority: explicit config > detected CPU cores > maxJobs
			if caps.MaxCPUJobs > 0 {
				maxCPUJobs = caps.MaxCPUJobs
			} else if daemon.SystemStats != nil && daemon.SystemStats.CPUCores > 0 {
				maxCPUJobs = daemon.SystemStats.CPUCores
			} else if caps.Performance != nil && caps.Performance.CPUCores > 0 {
				maxCPUJobs = caps.Performance.CPUCores
			} else {
				maxCPUJobs = maxJobs // Fallback to total job limit
			}

			// GPU job limit - priority: explicit config > sum of GPU sessions > 0
			if caps.MaxGPUJobs > 0 {
				maxGPUJobs = caps.MaxGPUJobs
			} else {
				for _, gpu := range caps.GPUs {
					if gpu.MaxEncodeSessions == 0 {
						// 0 means unlimited - use a large number
						maxGPUJobs += 100
					} else {
						maxGPUJobs += int(gpu.MaxEncodeSessions)
					}
				}
			}

			// Probe job limit - priority: explicit config > maxJobs
			if caps.MaxProbeJobs > 0 {
				maxProbeJobs = caps.MaxProbeJobs
			}
		}
	}

	// Apply defaults for probes (if not explicitly set, default to maxJobs)
	if maxProbeJobs == 0 {
		maxProbeJobs = maxJobs
	}

	ds := &DaemonStream{
		DaemonID:     daemonID,
		Stream:       stream,
		Logger:       m.logger,
		MaxJobs:      maxJobs,
		MaxCPUJobs:   maxCPUJobs,
		MaxGPUJobs:   maxGPUJobs,
		MaxProbeJobs: maxProbeJobs,
	}

	m.streams[daemonID] = ds

	m.logger.Info("daemon transcode stream connected",
		slog.String("daemon_id", string(daemonID)),
		slog.Int("max_jobs", maxJobs),
		slog.Int("max_cpu_jobs", maxCPUJobs),
		slog.Int("max_gpu_jobs", maxGPUJobs),
		slog.Int("max_probe_jobs", maxProbeJobs),
	)

	return ds
}

// UnregisterStream removes a daemon stream.
func (m *DaemonStreamManager) UnregisterStream(daemonID types.DaemonID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ds, ok := m.streams[daemonID]; ok {
		ds.Close()
		delete(m.streams, daemonID)
		m.logger.Debug("daemon transcode stream disconnected",
			slog.String("daemon_id", string(daemonID)),
		)
	}
}

// GetStream returns the stream for a daemon.
func (m *DaemonStreamManager) GetStream(daemonID types.DaemonID) (*DaemonStream, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ds, ok := m.streams[daemonID]
	return ds, ok
}

// GetIdleStream returns an idle stream for a daemon that matches the criteria.
// Deprecated: Use GetStreamWithCapacity for multi-job support.
func (m *DaemonStreamManager) GetIdleStream(daemonID types.DaemonID) (*DaemonStream, bool) {
	return m.GetStreamWithCapacity(daemonID)
}

// GetStreamWithCapacity returns a stream for a daemon if it has capacity for more jobs.
func (m *DaemonStreamManager) GetStreamWithCapacity(daemonID types.DaemonID) (*DaemonStream, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ds, ok := m.streams[daemonID]
	if !ok {
		return nil, false
	}

	if !ds.HasCapacity() {
		return nil, false
	}

	return ds, true
}

// GetStreamWithCapacityForType returns a stream for a daemon if it has capacity for a specific job type.
func (m *DaemonStreamManager) GetStreamWithCapacityForType(daemonID types.DaemonID, jobType JobType) (*DaemonStream, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ds, ok := m.streams[daemonID]
	if !ok {
		return nil, false
	}

	if !ds.HasCapacityForType(jobType) {
		return nil, false
	}

	return ds, true
}

// GetAnyIdleStream returns any idle stream from available daemons.
// Deprecated: Use GetAnyStreamWithCapacity for multi-job support.
func (m *DaemonStreamManager) GetAnyIdleStream() (*DaemonStream, bool) {
	return m.GetAnyStreamWithCapacity()
}

// GetAnyStreamWithCapacity returns any stream that has capacity for more jobs.
func (m *DaemonStreamManager) GetAnyStreamWithCapacity() (*DaemonStream, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, ds := range m.streams {
		if ds.HasCapacity() {
			return ds, true
		}
	}

	return nil, false
}

// GetAnyStreamWithCapacityForType returns any stream with capacity for a specific job type.
func (m *DaemonStreamManager) GetAnyStreamWithCapacityForType(jobType JobType) (*DaemonStream, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, ds := range m.streams {
		if ds.HasCapacityForType(jobType) {
			return ds, true
		}
	}

	return nil, false
}

// GetLeastLoadedStreamForType returns the stream with the lowest load for a specific job type.
func (m *DaemonStreamManager) GetLeastLoadedStreamForType(jobType JobType) (*DaemonStream, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var bestStream *DaemonStream
	lowestLoad := float64(1.0)

	for _, ds := range m.streams {
		if !ds.HasCapacityForType(jobType) {
			continue
		}

		var load float64
		switch jobType {
		case JobTypeCPU:
			load = ds.CPULoadPercent()
		case JobTypeGPU:
			load = ds.GPULoadPercent()
		default:
			load = ds.TotalLoadPercent()
		}

		if load < lowestLoad {
			lowestLoad = load
			bestStream = ds
		}
	}

	if bestStream == nil {
		return nil, false
	}
	return bestStream, true
}

// WaitForStream waits for a daemon's transcode stream to become available.
// This is useful after spawning a subprocess - we need to wait for it to
// connect and open its transcode stream.
func (m *DaemonStreamManager) WaitForStream(ctx context.Context, daemonID types.DaemonID) (*DaemonStream, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			if stream, ok := m.GetStream(daemonID); ok {
				return stream, nil
			}
		}
	}
}

// Probe sends a probe request to a daemon stream selected via strategy.
// Uses the configured probe strategy (defaults to LeastLoaded) to select
// the most appropriate daemon for probing.
func (m *DaemonStreamManager) Probe(ctx context.Context, streamURL string, timeoutMs int32) (*proto.ProbeResponse, error) {
	var selectedDaemonID types.DaemonID
	var stream *DaemonStream

	// Use strategy-based selection if registry is available
	if m.registry != nil && m.probeStrategyProvider != nil {
		strategy := m.probeStrategyProvider.GetProbeStrategy()
		// Empty criteria - we just want the least loaded daemon for probing
		daemon := m.registry.SelectDaemon(strategy, SelectionCriteria{})
		if daemon != nil {
			selectedDaemonID = daemon.ID
			m.logger.Debug("selected daemon for probe via strategy",
				slog.String("daemon_id", string(daemon.ID)),
				slog.String("strategy", strategy.Name()),
			)
		}
	}

	m.mu.RLock()
	if selectedDaemonID != "" {
		// Use the strategy-selected daemon
		stream = m.streams[selectedDaemonID]
	}
	// Fallback: if strategy selection failed or stream not found, pick any available
	if stream == nil {
		for _, ds := range m.streams {
			stream = ds
			break
		}
	}
	m.mu.RUnlock()

	if stream == nil {
		return nil, errors.New("no daemon streams available for probing")
	}

	return stream.Probe(ctx, streamURL, timeoutMs)
}

// Count returns the number of active streams.
func (m *DaemonStreamManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.streams)
}

// ActiveJob represents an active transcode job on a daemon stream.
type ActiveJob struct {
	ID          string
	SessionID   string
	ChannelID   string
	ChannelName string
	DaemonID    types.DaemonID
	Stream      *DaemonStream
	Buffer      *SharedESBuffer
	JobType     JobType // CPU or GPU job slot

	// Stats from daemon (updated with each batch)
	Stats *types.TranscodeStats

	// Channels for receiving transcoded data
	Samples chan *proto.ESSampleBatch
	Done    chan struct{}
	Err     error

	mu     sync.Mutex
	closed bool
}

// Close closes the active job.
func (j *ActiveJob) Close() {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.closed {
		return
	}
	j.closed = true

	j.Stream.RemoveActiveJob(j.ID)
	close(j.Done)
}

// SendSamples sends transcoded samples to the job.
func (j *ActiveJob) SendSamples(batch *proto.ESSampleBatch) {
	select {
	case j.Samples <- batch:
	case <-j.Done:
	}
}

// SetError sets an error on the job.
func (j *ActiveJob) SetError(err error) {
	j.mu.Lock()
	j.Err = err
	j.mu.Unlock()
}

// ActiveJobManager manages active transcode jobs.
type ActiveJobManager struct {
	logger *slog.Logger

	mu   sync.RWMutex
	jobs map[string]*ActiveJob
}

// NewActiveJobManager creates a new active job manager.
func NewActiveJobManager(logger *slog.Logger) *ActiveJobManager {
	return &ActiveJobManager{
		logger: logger,
		jobs:   make(map[string]*ActiveJob),
	}
}

// CreateJob creates a new active job with the specified job type (CPU or GPU).
func (m *ActiveJobManager) CreateJob(
	jobID string,
	sessionID string,
	channelID string,
	channelName string,
	daemonID types.DaemonID,
	stream *DaemonStream,
	buffer *SharedESBuffer,
	jobType JobType,
) *ActiveJob {
	job := &ActiveJob{
		ID:          jobID,
		SessionID:   sessionID,
		ChannelID:   channelID,
		ChannelName: channelName,
		DaemonID:    daemonID,
		Stream:      stream,
		Buffer:      buffer,
		JobType:     jobType,
		Samples:     make(chan *proto.ESSampleBatch, 10000), // Large buffer - will fill SharedESBuffer
		Done:        make(chan struct{}),
	}

	stream.AddActiveJobWithType(jobID, jobType)

	m.mu.Lock()
	m.jobs[jobID] = job
	m.mu.Unlock()

	m.logger.Debug("Created active transcode job",
		slog.String("job_id", jobID),
		slog.String("daemon_id", string(daemonID)),
		slog.String("job_type", jobType.String()),
	)

	return job
}

// GetJob returns an active job by ID.
func (m *ActiveJobManager) GetJob(jobID string) (*ActiveJob, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	job, ok := m.jobs[jobID]
	return job, ok
}

// GetJobsByDaemon returns all active jobs for a specific daemon.
func (m *ActiveJobManager) GetJobsByDaemon(daemonID types.DaemonID) []*ActiveJob {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*ActiveJob
	for _, job := range m.jobs {
		if job.DaemonID == daemonID {
			result = append(result, job)
		}
	}
	return result
}

// GetAllJobs returns all active jobs.
func (m *ActiveJobManager) GetAllJobs() []*ActiveJob {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*ActiveJob, 0, len(m.jobs))
	for _, job := range m.jobs {
		result = append(result, job)
	}
	return result
}

// RemoveJob removes an active job.
func (m *ActiveJobManager) RemoveJob(jobID string) {
	m.mu.Lock()
	job, ok := m.jobs[jobID]
	if ok {
		delete(m.jobs, jobID)
	}
	m.mu.Unlock()

	if ok && job != nil {
		job.Close()
		m.logger.Debug("Removed active transcode job",
			slog.String("job_id", jobID),
		)
	}
}

// StartTranscodeJob starts a transcode job on a daemon stream.
// This sends the TranscodeStart message and sets up the job tracking.
func StartTranscodeJob(
	ctx context.Context,
	streamMgr *DaemonStreamManager,
	jobMgr *ActiveJobManager,
	daemonID types.DaemonID,
	jobID string,
	sessionID string,
	channelID string,
	channelName string,
	buffer *SharedESBuffer,
	config *proto.TranscodeStart,
	jobType JobType,
) (*ActiveJob, error) {
	// Get the daemon's stream with capacity for this job type
	stream, ok := streamMgr.GetStreamWithCapacityForType(daemonID, jobType)
	if !ok {
		return nil, errors.New("daemon stream not available or at capacity for job type")
	}

	// Create active job with type tracking
	job := jobMgr.CreateJob(jobID, sessionID, channelID, channelName, daemonID, stream, buffer, jobType)

	// Send TranscodeStart to daemon
	err := stream.Send(&proto.TranscodeMessage{
		Payload: &proto.TranscodeMessage_Start{
			Start: config,
		},
	})
	if err != nil {
		jobMgr.RemoveJob(jobID)
		return nil, err
	}

	return job, nil
}
