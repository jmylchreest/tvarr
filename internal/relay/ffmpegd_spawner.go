// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jmylchreest/tvarr/internal/observability"
	"github.com/jmylchreest/tvarr/internal/util"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
)

// FFmpegDSpawner errors.
var (
	// ErrFFmpegDBinaryNotFound indicates the tvarr-ffmpegd binary is not available.
	ErrFFmpegDBinaryNotFound = errors.New("tvarr-ffmpegd binary not found")

	// ErrSpawnTimeout indicates the subprocess failed to start in time.
	ErrSpawnTimeout = errors.New("tvarr-ffmpegd subprocess startup timeout")

	// ErrMaxSpawnsReached indicates the maximum concurrent spawn limit was reached.
	ErrMaxSpawnsReached = errors.New("maximum concurrent spawns reached")

	// ErrSpawnFailed indicates the subprocess failed to start.
	ErrSpawnFailed = errors.New("tvarr-ffmpegd subprocess failed to start")

	// ErrRegistrationTimeout indicates the daemon didn't register in time.
	ErrRegistrationTimeout = errors.New("tvarr-ffmpegd daemon registration timeout")
)

// FFmpegDSpawnerConfig configures the subprocess spawner.
type FFmpegDSpawnerConfig struct {
	// BinaryPath is the explicit path to tvarr-ffmpegd binary.
	// If empty, searches PATH and common locations.
	BinaryPath string

	// CoordinatorAddress is the gRPC address subprocesses should connect to.
	// Example: "localhost:9090" or "unix:///tmp/tvarr/grpc.sock"
	CoordinatorAddress string

	// AuthToken is the optional authentication token for gRPC.
	AuthToken string

	// StartupTimeout is how long to wait for subprocess to become ready.
	// Defaults to 15 seconds.
	StartupTimeout time.Duration

	// ShutdownTimeout is how long to wait for graceful subprocess shutdown.
	// Defaults to 5 seconds.
	ShutdownTimeout time.Duration

	// MaxConcurrentSpawns limits concurrent subprocess spawns.
	// 0 means unlimited.
	MaxConcurrentSpawns int

	// Logger for structured logging.
	Logger *slog.Logger
}

// Validate checks the configuration for errors.
func (c *FFmpegDSpawnerConfig) Validate() error {
	if c.StartupTimeout < 0 {
		return errors.New("startup timeout cannot be negative")
	}
	if c.ShutdownTimeout < 0 {
		return errors.New("shutdown timeout cannot be negative")
	}
	return nil
}

// FFmpegDSpawner spawns tvarr-ffmpegd subprocesses for local transcoding.
// Subprocesses connect back to the coordinator via gRPC and are managed
// through the normal daemon registry and stream management.
type FFmpegDSpawner struct {
	config   FFmpegDSpawnerConfig
	logger   *slog.Logger
	registry *DaemonRegistry

	// Active spawns tracking
	mu           sync.RWMutex
	activeSpawns map[string]*spawnedProcess
	spawnCount   atomic.Int32

	// Cached binary path (resolved on first use)
	binaryPath     string
	binaryPathOnce sync.Once

	// Cached capabilities from detect command
	capabilities     *DetectedCapabilities
	capabilitiesOnce sync.Once
}

// spawnedProcess tracks a spawned subprocess.
type spawnedProcess struct {
	jobID     string
	daemonID  types.DaemonID
	cmd       *exec.Cmd
	startedAt time.Time
	cancel    context.CancelFunc
}

// DetectedCapabilities contains capabilities from the detect command.
type DetectedCapabilities struct {
	FFmpeg       FFmpegInfo       `json:"ffmpeg"`
	Capabilities CapabilitiesInfo `json:"capabilities"`
}

// FFmpegInfo contains FFmpeg binary information.
type FFmpegInfo struct {
	Version     string `json:"version"`
	FFmpegPath  string `json:"ffmpeg_path"`
	FFprobePath string `json:"ffprobe_path"`
}

// CapabilitiesInfo contains detected capabilities.
type CapabilitiesInfo struct {
	VideoEncoders     []string      `json:"video_encoders"`
	VideoDecoders     []string      `json:"video_decoders"`
	AudioEncoders     []string      `json:"audio_encoders"`
	AudioDecoders     []string      `json:"audio_decoders"`
	HardwareAccels    []HWAccelInfo `json:"hardware_accels"`
	GPUs              []GPUInfo     `json:"gpus"`
	MaxConcurrentJobs int           `json:"max_concurrent_jobs"`
}

// HWAccelInfo contains hardware accelerator information.
type HWAccelInfo struct {
	Type      string   `json:"type"`
	Device    string   `json:"device,omitempty"`
	Available bool     `json:"available"`
	Encoders  []string `json:"encoders,omitempty"`
	Decoders  []string `json:"decoders,omitempty"`
}

// GPUInfo contains GPU information.
type GPUInfo struct {
	Index             int    `json:"index"`
	Name              string `json:"name"`
	Class             string `json:"class"`
	MaxEncodeSessions int    `json:"max_encode_sessions"`
	MaxDecodeSessions int    `json:"max_decode_sessions,omitempty"`
}

// NewFFmpegDSpawner creates a new subprocess spawner.
func NewFFmpegDSpawner(config FFmpegDSpawnerConfig) *FFmpegDSpawner {
	// Apply defaults
	if config.StartupTimeout == 0 {
		config.StartupTimeout = 15 * time.Second
	}
	if config.ShutdownTimeout == 0 {
		config.ShutdownTimeout = 5 * time.Second
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	return &FFmpegDSpawner{
		config:       config,
		logger:       config.Logger,
		activeSpawns: make(map[string]*spawnedProcess),
	}
}

// SetRegistry sets the daemon registry for waiting on registrations.
func (s *FFmpegDSpawner) SetRegistry(registry *DaemonRegistry) {
	s.registry = registry
}

// SetCoordinatorAddress sets the coordinator address for subprocess connections.
func (s *FFmpegDSpawner) SetCoordinatorAddress(addr string) {
	s.config.CoordinatorAddress = addr
}

// SpawnForJob spawns a tvarr-ffmpegd subprocess for a specific transcoding job.
// The subprocess connects to the coordinator and registers as a daemon.
// Returns the daemon ID and a cleanup function.
// The cleanup function MUST be called when the job completes to terminate the subprocess.
func (s *FFmpegDSpawner) SpawnForJob(ctx context.Context, jobID string) (types.DaemonID, func(), error) {
	// Check binary availability
	binaryPath := s.findBinary()
	if binaryPath == "" {
		return "", nil, fmt.Errorf("%w: searched PATH and common locations", ErrFFmpegDBinaryNotFound)
	}

	// Check coordinator address
	if s.config.CoordinatorAddress == "" {
		return "", nil, errors.New("coordinator address not configured")
	}

	// Check registry is set
	if s.registry == nil {
		return "", nil, errors.New("daemon registry not configured")
	}

	// Check max concurrent spawns
	if s.config.MaxConcurrentSpawns > 0 {
		if int(s.spawnCount.Load()) >= s.config.MaxConcurrentSpawns {
			return "", nil, ErrMaxSpawnsReached
		}
	}

	// Generate unique daemon ID for this spawn
	daemonID := types.DaemonID(fmt.Sprintf("local-%s", jobID))

	s.logger.Debug("spawning ffmpegd subprocess",
		slog.String("job_id", jobID),
		slog.String("daemon_id", string(daemonID)),
		slog.String("binary", binaryPath),
		slog.String("coordinator", s.config.CoordinatorAddress))

	// Create subprocess context
	processCtx, processCancel := context.WithCancel(context.Background())

	// Build command arguments
	args := []string{
		"serve",
		"--coordinator-url", s.config.CoordinatorAddress,
		"--daemon-id", string(daemonID),
		"--name", string(daemonID),
		"--log-level", "info",
	}

	// Add auth token if configured
	if s.config.AuthToken != "" {
		args = append(args, "--auth-token", s.config.AuthToken)
	}

	cmd := exec.CommandContext(processCtx, binaryPath, args...)

	// Capture both stdout and stderr - subprocess outputs JSON logs
	logCapture := &logWriter{
		logger: s.logger,
		jobID:  jobID,
	}
	cmd.Stdout = logCapture
	cmd.Stderr = logCapture

	// Start the subprocess
	if err := cmd.Start(); err != nil {
		processCancel()
		return "", nil, fmt.Errorf("%w: %v", ErrSpawnFailed, err)
	}

	s.logger.Debug("ffmpegd subprocess started",
		slog.String("job_id", jobID),
		slog.String("daemon_id", string(daemonID)),
		slog.Int("pid", cmd.Process.Pid))

	// Wait for daemon to register
	if err := s.waitForRegistration(ctx, daemonID); err != nil {
		// Kill the process if registration failed
		processCancel()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return "", nil, err
	}

	// Track the spawned process
	spawned := &spawnedProcess{
		jobID:     jobID,
		daemonID:  daemonID,
		cmd:       cmd,
		startedAt: time.Now(),
		cancel:    processCancel,
	}

	s.mu.Lock()
	s.activeSpawns[jobID] = spawned
	s.spawnCount.Add(1)
	s.mu.Unlock()

	// Create cleanup function
	cleanup := func() {
		s.cleanupSpawn(jobID, spawned)
	}

	s.logger.Info("ffmpegd subprocess ready",
		slog.String("job_id", jobID),
		slog.String("daemon_id", string(daemonID)),
		slog.Int("pid", cmd.Process.Pid))

	return daemonID, cleanup, nil
}

// waitForRegistration waits for the daemon to appear in the registry.
func (s *FFmpegDSpawner) waitForRegistration(ctx context.Context, daemonID types.DaemonID) error {
	// Create timeout context for startup
	startupCtx, cancel := context.WithTimeout(ctx, s.config.StartupTimeout)
	defer cancel()

	// Poll registry until daemon appears
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-startupCtx.Done():
			return fmt.Errorf("%w: daemon %s did not register", ErrRegistrationTimeout, daemonID)
		case <-ticker.C:
			if daemon, ok := s.registry.Get(daemonID); ok {
				if daemon.State == types.DaemonStateConnected {
					s.logger.Debug("ffmpegd subprocess registered",
						slog.String("daemon_id", string(daemonID)))
					return nil
				}
			}
		}
	}
}

// cleanupSpawn terminates a spawned subprocess.
func (s *FFmpegDSpawner) cleanupSpawn(jobID string, spawned *spawnedProcess) {
	s.mu.Lock()
	if _, exists := s.activeSpawns[jobID]; !exists {
		// Already cleaned up
		s.mu.Unlock()
		return
	}
	delete(s.activeSpawns, jobID)
	s.spawnCount.Add(-1)
	s.mu.Unlock()

	s.logger.Debug("cleaning up ffmpegd subprocess",
		slog.String("job_id", jobID),
		slog.String("daemon_id", string(spawned.daemonID)))

	// Cancel process context (signals graceful shutdown)
	if spawned.cancel != nil {
		spawned.cancel()
	}

	// Wait for process to exit with timeout
	if spawned.cmd != nil && spawned.cmd.Process != nil {
		done := make(chan error, 1)
		go func() {
			done <- spawned.cmd.Wait()
		}()

		select {
		case <-done:
			// Process exited cleanly
		case <-time.After(s.config.ShutdownTimeout):
			// Force kill if didn't exit in time
			s.logger.Warn("ffmpegd subprocess did not exit gracefully, killing",
				slog.String("job_id", jobID),
				slog.Int("pid", spawned.cmd.Process.Pid))
			_ = spawned.cmd.Process.Kill()
			<-done
		}
	}

	// Unregister daemon from registry
	if s.registry != nil {
		s.registry.Unregister(spawned.daemonID, "subprocess terminated")
	}

	s.logger.Info("ffmpegd subprocess cleaned up",
		slog.String("job_id", jobID),
		slog.String("daemon_id", string(spawned.daemonID)),
		slog.Duration("runtime", time.Since(spawned.startedAt)))
}

// DetectCapabilities runs the detect command and returns capabilities.
// Results are cached after the first call.
func (s *FFmpegDSpawner) DetectCapabilities(ctx context.Context) (*DetectedCapabilities, error) {
	var detectErr error

	s.capabilitiesOnce.Do(func() {
		binaryPath := s.findBinary()
		if binaryPath == "" {
			detectErr = ErrFFmpegDBinaryNotFound
			return
		}

		// Run detect command with timeout
		detectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		cmd := exec.CommandContext(detectCtx, binaryPath, "detect")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			detectErr = fmt.Errorf("detect command failed: %w (stderr: %s)", err, stderr.String())
			return
		}

		// Parse JSON output
		var caps DetectedCapabilities
		if err := json.Unmarshal(stdout.Bytes(), &caps); err != nil {
			detectErr = fmt.Errorf("parsing detect output: %w", err)
			return
		}

		s.capabilities = &caps
	})

	if detectErr != nil {
		return nil, detectErr
	}

	return s.capabilities, nil
}

// LogCapabilities runs detection and logs the results.
func (s *FFmpegDSpawner) LogCapabilities(ctx context.Context) {
	caps, err := s.DetectCapabilities(ctx)
	if err != nil {
		s.logger.Warn("failed to detect local ffmpegd capabilities",
			slog.String("error", err.Error()))
		return
	}

	// Log FFmpeg info
	s.logger.Info("local ffmpegd detected",
		slog.String("ffmpeg_version", caps.FFmpeg.Version),
		slog.String("ffmpeg_path", caps.FFmpeg.FFmpegPath),
		slog.String("ffprobe_path", caps.FFmpeg.FFprobePath))

	// Log hardware accels with encoder/decoder details
	var availableAccels []string
	for _, hw := range caps.Capabilities.HardwareAccels {
		if hw.Available {
			name := hw.Type
			if hw.Device != "" {
				name = fmt.Sprintf("%s (%s)", hw.Type, hw.Device)
			}
			availableAccels = append(availableAccels, name)

			// Log per-hwaccel encoder/decoder details
			s.logger.Info("local hwaccel details",
				slog.String("type", hw.Type),
				slog.String("device", hw.Device),
				slog.Any("encoders", hw.Encoders),
				slog.Any("decoders", hw.Decoders))
		}
	}

	if len(availableAccels) > 0 {
		s.logger.Info("local hardware acceleration available",
			slog.Any("hw_accels", availableAccels))
	}

	// Log GPUs
	for _, gpu := range caps.Capabilities.GPUs {
		s.logger.Info("local GPU detected",
			slog.Int("index", gpu.Index),
			slog.String("name", gpu.Name),
			slog.String("class", gpu.Class),
			slog.Int("max_encode_sessions", gpu.MaxEncodeSessions))
	}

	// Log encoder summary
	s.logger.Info("local ffmpegd encoder summary",
		slog.Int("video_encoders", len(caps.Capabilities.VideoEncoders)),
		slog.Int("audio_encoders", len(caps.Capabilities.AudioEncoders)),
		slog.Int("max_concurrent_jobs", caps.Capabilities.MaxConcurrentJobs))
}

// GetSpawnedDaemonID returns the daemon ID for a job if it was spawned locally.
func (s *FFmpegDSpawner) GetSpawnedDaemonID(jobID string) (types.DaemonID, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if spawned, ok := s.activeSpawns[jobID]; ok {
		return spawned.daemonID, true
	}
	return "", false
}

// findBinary locates the tvarr-ffmpegd binary using the shared FindBinary utility.
// Search order:
//  1. TVARR_FFMPEGD_BINARY env var
//  2. ./tvarr-ffmpegd (local development)
//  3. tvarr-ffmpegd on PATH
func (s *FFmpegDSpawner) findBinary() string {
	s.binaryPathOnce.Do(func() {
		path, err := util.FindBinary("tvarr-ffmpegd", "TVARR_FFMPEGD_BINARY")
		if err == nil {
			s.binaryPath = path
		}
	})

	return s.binaryPath
}

// IsAvailable returns whether the tvarr-ffmpegd binary is available.
func (s *FFmpegDSpawner) IsAvailable() bool {
	return s.findBinary() != ""
}

// BinaryPath returns the path to the tvarr-ffmpegd binary.
func (s *FFmpegDSpawner) BinaryPath() string {
	return s.findBinary()
}

// GetVersion returns the version string of the tvarr-ffmpegd binary.
// Returns empty string if the binary is not available or version cannot be determined.
func (s *FFmpegDSpawner) GetVersion() string {
	binaryPath := s.findBinary()
	if binaryPath == "" {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "--version")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Parse version from output like "tvarr-ffmpegd version dev (7d124c1d*)"
	// Extract just the version part after "version "
	line := strings.TrimSpace(string(output))
	if idx := strings.Index(line, "version "); idx != -1 {
		return strings.TrimSpace(line[idx+8:])
	}

	return line
}

// ActiveSpawns returns the number of active subprocess spawns.
func (s *FFmpegDSpawner) ActiveSpawns() int {
	return int(s.spawnCount.Load())
}

// GetActiveJobs returns the job IDs of all active spawns.
func (s *FFmpegDSpawner) GetActiveJobs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := make([]string, 0, len(s.activeSpawns))
	for jobID := range s.activeSpawns {
		jobs = append(jobs, jobID)
	}
	return jobs
}

// StopAll terminates all active spawned processes.
func (s *FFmpegDSpawner) StopAll() {
	s.mu.Lock()
	toCleanup := make([]*spawnedProcess, 0, len(s.activeSpawns))
	jobIDs := make([]string, 0, len(s.activeSpawns))
	for jobID, spawned := range s.activeSpawns {
		toCleanup = append(toCleanup, spawned)
		jobIDs = append(jobIDs, jobID)
	}
	s.mu.Unlock()

	// Clean up outside the lock
	for i, spawned := range toCleanup {
		s.cleanupSpawn(jobIDs[i], spawned)
	}
}

// logWriter implements io.Writer for subprocess stderr logging.
// It parses JSON log lines from the subprocess and re-emits them with proper log levels.
type logWriter struct {
	logger *slog.Logger
	jobID  string
	buf    []byte // Buffer for incomplete lines
}

// subprocessLogEntry represents a JSON log entry from the subprocess.
type subprocessLogEntry struct {
	Time    string         `json:"time"`
	Level   string         `json:"level"`
	Msg     string         `json:"msg"`
	Fields  map[string]any `json:"-"` // Captures all other fields
	RawJSON json.RawMessage
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	// Append to buffer
	w.buf = append(w.buf, p...)

	// Process complete lines
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}

		line := w.buf[:idx]
		w.buf = w.buf[idx+1:]

		if len(line) == 0 {
			continue
		}

		w.processLine(line)
	}

	return len(p), nil
}

func (w *logWriter) processLine(line []byte) {
	// Try to parse as JSON
	var entry map[string]any
	if err := json.Unmarshal(line, &entry); err != nil {
		// Not JSON, log as plain text
		w.logger.Info("ffmpegd output",
			slog.String("app", "tvarr-ffmpegd"),
			slog.String("job_id", w.jobID),
			slog.String("line", string(line)))
		return
	}

	// Extract standard fields
	level, _ := entry["level"].(string)
	msg, _ := entry["msg"].(string)

	// Build attributes from remaining fields (excluding time, level, msg)
	attrs := make([]any, 0, len(entry)*2+4)
	attrs = append(attrs, slog.String("app", "tvarr-ffmpegd"))
	attrs = append(attrs, slog.String("job_id", w.jobID))

	for k, v := range entry {
		if k == "time" || k == "level" || k == "msg" {
			continue
		}
		attrs = append(attrs, slog.Any(k, v))
	}

	// Re-emit with correct log level
	switch strings.ToUpper(level) {
	case "TRACE":
		// Use custom trace level from observability package
		w.logger.Log(context.Background(), observability.LevelTrace, msg, attrs...)
	case "DEBUG":
		w.logger.Debug(msg, attrs...)
	case "INFO":
		w.logger.Info(msg, attrs...)
	case "WARN", "WARNING":
		w.logger.Warn(msg, attrs...)
	case "ERROR":
		w.logger.Error(msg, attrs...)
	default:
		// Default to INFO for unknown levels
		w.logger.Info(msg, attrs...)
	}
}
