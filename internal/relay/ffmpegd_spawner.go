// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
)

// FFmpegDSpawnerConfig configures the subprocess spawner.
type FFmpegDSpawnerConfig struct {
	// BinaryPath is the explicit path to tvarr-ffmpegd binary.
	// If empty, searches PATH and common locations.
	BinaryPath string

	// PreferUnixSocket uses Unix domain sockets instead of TCP localhost.
	// More efficient on Unix systems, automatically falls back to TCP on Windows.
	PreferUnixSocket bool

	// SocketDir is the directory for Unix domain sockets.
	// Defaults to os.TempDir()/tvarr-ffmpegd.
	SocketDir string

	// StartupTimeout is how long to wait for subprocess to become ready.
	// Defaults to 10 seconds.
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
// Each transcoding job gets its own subprocess, which is terminated when done.
type FFmpegDSpawner struct {
	config FFmpegDSpawnerConfig
	logger *slog.Logger

	// Active spawns tracking
	mu           sync.RWMutex
	activeSpawns map[string]*spawnedProcess
	spawnCount   atomic.Int32

	// Cached binary path (resolved on first use)
	binaryPath     string
	binaryPathOnce sync.Once
}

// spawnedProcess tracks a spawned subprocess.
type spawnedProcess struct {
	jobID      string
	cmd        *exec.Cmd
	conn       *grpc.ClientConn
	client     proto.FFmpegDaemonClient
	address    string
	socketPath string // Unix socket path (for cleanup)
	startedAt  time.Time
	cancel     context.CancelFunc
}

// NewFFmpegDSpawner creates a new subprocess spawner.
func NewFFmpegDSpawner(config FFmpegDSpawnerConfig) *FFmpegDSpawner {
	// Apply defaults
	if config.StartupTimeout == 0 {
		config.StartupTimeout = 10 * time.Second
	}
	if config.ShutdownTimeout == 0 {
		config.ShutdownTimeout = 5 * time.Second
	}
	if config.SocketDir == "" {
		config.SocketDir = filepath.Join(os.TempDir(), "tvarr-ffmpegd")
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	// Check environment variable for binary path
	if config.BinaryPath == "" {
		if envPath := os.Getenv("TVARR_FFMPEGD_BINARY"); envPath != "" {
			config.BinaryPath = envPath
		}
	}

	return &FFmpegDSpawner{
		config:       config,
		logger:       config.Logger,
		activeSpawns: make(map[string]*spawnedProcess),
	}
}

// SpawnForJob spawns a tvarr-ffmpegd subprocess for a specific transcoding job.
// Returns a gRPC client connected to the subprocess and a cleanup function.
// The cleanup function MUST be called when the job completes to terminate the subprocess.
func (s *FFmpegDSpawner) SpawnForJob(ctx context.Context, jobID string) (proto.FFmpegDaemonClient, func(), error) {
	// Check binary availability
	binaryPath := s.findBinary()
	if binaryPath == "" {
		return nil, nil, fmt.Errorf("%w: searched PATH and common locations", ErrFFmpegDBinaryNotFound)
	}

	// Check max concurrent spawns
	if s.config.MaxConcurrentSpawns > 0 {
		if int(s.spawnCount.Load()) >= s.config.MaxConcurrentSpawns {
			return nil, nil, ErrMaxSpawnsReached
		}
	}

	// Generate address for this job
	address, socketPath := s.generateAddressAndSocket(jobID)

	s.logger.Debug("spawning ffmpegd subprocess",
		slog.String("job_id", jobID),
		slog.String("binary", binaryPath),
		slog.String("address", address))

	// Create subprocess context
	processCtx, processCancel := context.WithCancel(context.Background())

	// Build command arguments
	args := []string{
		"serve",
		"--address", address,
		"--log-level", "warn",
	}

	cmd := exec.CommandContext(processCtx, binaryPath, args...)

	// Capture stderr for debugging
	cmd.Stderr = &logWriter{
		logger: s.logger,
		jobID:  jobID,
	}

	// Start the subprocess
	if err := cmd.Start(); err != nil {
		processCancel()
		if socketPath != "" {
			os.Remove(socketPath)
		}
		return nil, nil, fmt.Errorf("%w: %v", ErrSpawnFailed, err)
	}

	s.logger.Debug("ffmpegd subprocess started",
		slog.String("job_id", jobID),
		slog.Int("pid", cmd.Process.Pid))

	// Wait for subprocess to become ready
	conn, client, err := s.waitForReady(ctx, address, jobID)
	if err != nil {
		// Kill the process if we couldn't connect
		processCancel()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if socketPath != "" {
			os.Remove(socketPath)
		}
		return nil, nil, err
	}

	// Track the spawned process
	spawned := &spawnedProcess{
		jobID:      jobID,
		cmd:        cmd,
		conn:       conn,
		client:     client,
		address:    address,
		socketPath: socketPath,
		startedAt:  time.Now(),
		cancel:     processCancel,
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
		slog.Int("pid", cmd.Process.Pid),
		slog.String("address", address))

	return client, cleanup, nil
}

// waitForReady waits for the subprocess to become ready and connects to it.
func (s *FFmpegDSpawner) waitForReady(ctx context.Context, address, jobID string) (*grpc.ClientConn, proto.FFmpegDaemonClient, error) {
	// Create timeout context for startup
	startupCtx, cancel := context.WithTimeout(ctx, s.config.StartupTimeout)
	defer cancel()

	// Determine the dial target based on address scheme
	dialTarget := address
	var dialOpts []grpc.DialOption
	dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))

	// Retry connection until ready or timeout
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		select {
		case <-startupCtx.Done():
			if lastErr != nil {
				return nil, nil, fmt.Errorf("%w: %v (last error: %v)", ErrSpawnTimeout, startupCtx.Err(), lastErr)
			}
			return nil, nil, fmt.Errorf("%w: %v", ErrSpawnTimeout, startupCtx.Err())
		case <-ticker.C:
			// Try to connect
			conn, err := grpc.NewClient(dialTarget, dialOpts...)
			if err != nil {
				lastErr = err
				continue
			}

			// Create client and test connection
			client := proto.NewFFmpegDaemonClient(conn)

			// Try a simple RPC to verify connection
			statsCtx, statsCancel := context.WithTimeout(startupCtx, 2*time.Second)
			_, err = client.GetStats(statsCtx, &proto.GetStatsRequest{})
			statsCancel()

			if err != nil {
				conn.Close()
				lastErr = err
				continue
			}

			s.logger.Debug("ffmpegd subprocess connection established",
				slog.String("job_id", jobID),
				slog.String("address", address))

			return conn, client, nil
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
		slog.String("job_id", jobID))

	// Close gRPC connection first
	if spawned.conn != nil {
		spawned.conn.Close()
	}

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

	// Clean up Unix socket if used
	if spawned.socketPath != "" {
		os.Remove(spawned.socketPath)
	}

	s.logger.Info("ffmpegd subprocess cleaned up",
		slog.String("job_id", jobID),
		slog.Duration("runtime", time.Since(spawned.startedAt)))
}

// generateAddress generates a gRPC address for a job.
// Exported for testing.
func (s *FFmpegDSpawner) generateAddress(jobID string) string {
	addr, _ := s.generateAddressAndSocket(jobID)
	return addr
}

// generateAddressAndSocket generates a gRPC address and optional socket path for a job.
func (s *FFmpegDSpawner) generateAddressAndSocket(jobID string) (address string, socketPath string) {
	if s.config.PreferUnixSocket && runtime.GOOS != "windows" {
		// Use Unix domain socket
		socketPath = filepath.Join(s.config.SocketDir, fmt.Sprintf("ffmpegd-%s.sock", jobID))

		// Ensure socket directory exists
		os.MkdirAll(s.config.SocketDir, 0755)

		// Remove existing socket if present
		os.Remove(socketPath)

		return "unix://" + socketPath, socketPath
	}

	// Use TCP localhost with random port
	// Find an available port
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		// Fall back to a predictable port based on job ID hash
		// This shouldn't happen in practice
		return fmt.Sprintf("localhost:5%04d", hashJobID(jobID)%10000), ""
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	return fmt.Sprintf("localhost:%d", port), ""
}

// findBinary locates the tvarr-ffmpegd binary.
func (s *FFmpegDSpawner) findBinary() string {
	s.binaryPathOnce.Do(func() {
		// Check explicit config first
		if s.config.BinaryPath != "" {
			if _, err := os.Stat(s.config.BinaryPath); err == nil {
				s.binaryPath = s.config.BinaryPath
				return
			}
		}

		// Try to find in PATH
		if path, err := exec.LookPath("tvarr-ffmpegd"); err == nil {
			s.binaryPath = path
			return
		}

		// Check common locations
		commonPaths := []string{
			"/usr/local/bin/tvarr-ffmpegd",
			"/usr/bin/tvarr-ffmpegd",
			"./tvarr-ffmpegd",
			"./dist/debug/tvarr-ffmpegd",
			"./dist/release/tvarr-ffmpegd",
		}

		for _, path := range commonPaths {
			if _, err := os.Stat(path); err == nil {
				s.binaryPath = path
				return
			}
		}

		// Not found
		s.binaryPath = ""
	})

	return s.binaryPath
}

// IsAvailable returns whether the tvarr-ffmpegd binary is available.
func (s *FFmpegDSpawner) IsAvailable() bool {
	return s.findBinary() != ""
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
type logWriter struct {
	logger *slog.Logger
	jobID  string
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	line := string(p)
	if line != "" && line != "\n" {
		w.logger.Debug("ffmpegd subprocess output",
			slog.String("job_id", w.jobID),
			slog.String("line", line))
	}
	return len(p), nil
}

// hashJobID creates a simple hash of the job ID for port generation fallback.
func hashJobID(jobID string) int {
	h := 0
	for _, c := range jobID {
		h = h*31 + int(c)
	}
	if h < 0 {
		h = -h
	}
	return h
}
