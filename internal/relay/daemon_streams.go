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

// DaemonStream wraps a bidirectional gRPC stream to a daemon.
// The daemon opens this stream after registration and keeps it open.
// The coordinator can push transcode jobs through it.
type DaemonStream struct {
	DaemonID types.DaemonID
	Stream   grpc.BidiStreamingServer[proto.TranscodeMessage, proto.TranscodeMessage]
	Logger   *slog.Logger

	mu        sync.Mutex
	closed    bool
	activeJob string // Currently active job ID (empty if idle)
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

// IsIdle returns true if the stream has no active job.
func (s *DaemonStream) IsIdle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeJob == ""
}

// SetActiveJob sets the active job ID.
func (s *DaemonStream) SetActiveJob(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeJob = jobID
}

// ClearActiveJob clears the active job.
func (s *DaemonStream) ClearActiveJob() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeJob = ""
}

// Close marks the stream as closed.
func (s *DaemonStream) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
}

// DaemonStreamManager manages active transcode streams from daemons.
type DaemonStreamManager struct {
	logger *slog.Logger

	mu      sync.RWMutex
	streams map[types.DaemonID]*DaemonStream
}

// NewDaemonStreamManager creates a new stream manager.
func NewDaemonStreamManager(logger *slog.Logger) *DaemonStreamManager {
	return &DaemonStreamManager{
		logger:  logger,
		streams: make(map[types.DaemonID]*DaemonStream),
	}
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

	ds := &DaemonStream{
		DaemonID: daemonID,
		Stream:   stream,
		Logger:   m.logger,
	}

	m.streams[daemonID] = ds

	m.logger.Info("Daemon transcode stream registered",
		slog.String("daemon_id", string(daemonID)),
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
		m.logger.Info("Daemon transcode stream unregistered",
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
func (m *DaemonStreamManager) GetIdleStream(daemonID types.DaemonID) (*DaemonStream, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ds, ok := m.streams[daemonID]
	if !ok {
		return nil, false
	}

	if !ds.IsIdle() {
		return nil, false
	}

	return ds, true
}

// GetAnyIdleStream returns any idle stream from available daemons.
func (m *DaemonStreamManager) GetAnyIdleStream() (*DaemonStream, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, ds := range m.streams {
		if ds.IsIdle() {
			return ds, true
		}
	}

	return nil, false
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

	j.Stream.ClearActiveJob()
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

// CreateJob creates a new active job.
func (m *ActiveJobManager) CreateJob(
	jobID string,
	sessionID string,
	channelID string,
	channelName string,
	daemonID types.DaemonID,
	stream *DaemonStream,
	buffer *SharedESBuffer,
) *ActiveJob {
	job := &ActiveJob{
		ID:          jobID,
		SessionID:   sessionID,
		ChannelID:   channelID,
		ChannelName: channelName,
		DaemonID:    daemonID,
		Stream:      stream,
		Buffer:      buffer,
		Samples:     make(chan *proto.ESSampleBatch, 10000), // Large buffer - will fill SharedESBuffer
		Done:        make(chan struct{}),
	}

	stream.SetActiveJob(jobID)

	m.mu.Lock()
	m.jobs[jobID] = job
	m.mu.Unlock()

	m.logger.Debug("Created active transcode job",
		slog.String("job_id", jobID),
		slog.String("daemon_id", string(daemonID)),
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
) (*ActiveJob, error) {
	// Get the daemon's stream
	stream, ok := streamMgr.GetIdleStream(daemonID)
	if !ok {
		return nil, errors.New("daemon stream not available or busy")
	}

	// Create active job
	job := jobMgr.CreateJob(jobID, sessionID, channelID, channelName, daemonID, stream, buffer)

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
