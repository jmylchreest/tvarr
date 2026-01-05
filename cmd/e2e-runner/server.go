package main

import (
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

//go:embed testdata/channel.webp testdata/program.webp testdata/test.m3u testdata/test.xml testdata/test-stream.ts
var testdataFS embed.FS

// findFreePort finds an available high port (never 8080) for the E2E server.
func findFreePort() (int, error) {
	// Listen on port 0 to get a random free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("failed to find free port: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	// Never use port 8080 (reserved for local dev instances)
	if port == 8080 {
		return findFreePort() // Recursively try again
	}

	return port, nil
}

// ManagedServer represents a tvarr server managed by the E2E runner.
type ManagedServer struct {
	cmd       *exec.Cmd
	port      int
	dataDir   string
	baseURL   string
	startErr  error
	logBuffer *logCapture
	logFiles  *LogFiles
}

// NewManagedServer creates and starts a new tvarr server on a random port.
// If logFiles is provided, server output will also be written to those files.
func NewManagedServer(binaryPath string, logFiles *LogFiles) (*ManagedServer, error) {
	port, err := findFreePort()
	if err != nil {
		return nil, err
	}

	// Create unique data directory
	dataDir := filepath.Join(os.TempDir(), fmt.Sprintf("tvarr-e2e-%d-%d", port, time.Now().UnixNano()))
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Build base URL for the server
	baseURL := fmt.Sprintf("http://localhost:%d", port)

	// Build the command with base-url for proper logo URL resolution
	cmd := exec.Command(binaryPath, "serve",
		"--port", fmt.Sprintf("%d", port),
		"--base-url", baseURL,
	)

	// Set environment for in-memory database and unique data dir
	cmd.Env = append(os.Environ(),
		"TVARR_DATABASE_DSN=file::memory:?cache=shared",
		"TVARR_DATABASE_MAX_OPEN_CONNS=1",
		"TVARR_DATABASE_MAX_IDLE_CONNS=1",
		fmt.Sprintf("TVARR_STORAGE_BASE_DIR=%s", dataDir),
	)

	// Set up output writers
	var stdoutWriter, stderrWriter io.Writer

	if logFiles != nil {
		// Write only to log files (hide from console)
		stdoutWriter = logFiles.StdoutFile
		stderrWriter = logFiles.StderrFile
	} else {
		// No log files - write to stderr for visibility
		stdoutWriter = os.Stderr
		stderrWriter = os.Stderr
	}

	// Capture tvarr server output
	logBuffer := newLogCapture(stdoutWriter)
	cmd.Stdout = logBuffer
	cmd.Stderr = stderrWriter

	ms := &ManagedServer{
		cmd:       cmd,
		port:      port,
		dataDir:   dataDir,
		baseURL:   baseURL,
		logBuffer: logBuffer,
		logFiles:  logFiles,
	}

	return ms, nil
}

// Start starts the managed server and waits for it to be ready.
func (ms *ManagedServer) Start(ctx context.Context) error {
	if err := ms.cmd.Start(); err != nil {
		ms.startErr = err
		return fmt.Errorf("failed to start server: %w", err)
	}

	// Wait for server to be ready (up to 30 seconds)
	client := &http.Client{Timeout: 2 * time.Second}
	healthURL := ms.baseURL + "/health"

	for i := 0; i < 30; i++ {
		select {
		case <-ctx.Done():
			ms.Stop()
			return ctx.Err()
		default:
		}

		req, _ := http.NewRequestWithContext(ctx, "GET", healthURL, nil)
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(time.Second)
	}

	ms.Stop()
	return fmt.Errorf("server failed to become ready within 30 seconds")
}

// Stop stops the managed server and cleans up.
func (ms *ManagedServer) Stop() {
	if ms.cmd != nil && ms.cmd.Process != nil {
		// Send SIGTERM for graceful shutdown
		ms.cmd.Process.Signal(syscall.SIGTERM)

		// Wait a bit then force kill if needed
		done := make(chan error, 1)
		go func() {
			done <- ms.cmd.Wait()
		}()

		select {
		case <-done:
			// Process exited
		case <-time.After(5 * time.Second):
			// Force kill
			ms.cmd.Process.Kill()
			<-done
		}
	}

	// Cleanup data directory
	if ms.dataDir != "" {
		os.RemoveAll(ms.dataDir)
	}
}

// Port returns the port the server is running on.
func (ms *ManagedServer) Port() int {
	return ms.port
}

// BaseURL returns the base URL for the server.
func (ms *ManagedServer) BaseURL() string {
	return ms.baseURL
}

// DataDir returns the data directory path.
func (ms *ManagedServer) DataDir() string {
	return ms.dataDir
}

// LogContains checks if the server logs contain a specific string.
func (ms *ManagedServer) LogContains(s string) bool {
	if ms.logBuffer == nil {
		return false
	}
	return ms.logBuffer.Contains(s)
}

// TestdataServer serves embedded testdata files over HTTP.
type TestdataServer struct {
	server   *http.Server
	listener net.Listener
	baseURL  string
}

// NewTestdataServer creates a testdata HTTP server on a random port.
func NewTestdataServer() (*TestdataServer, error) {
	// Find a free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to find free port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	// Create a sub-filesystem from the embedded FS
	subFS, err := fs.Sub(testdataFS, "testdata")
	if err != nil {
		listener.Close()
		return nil, fmt.Errorf("failed to create sub filesystem: %w", err)
	}

	// Read the test stream content for serving on /live/ paths
	testStreamData, err := fs.ReadFile(subFS, "test-stream.ts")
	if err != nil {
		listener.Close()
		return nil, fmt.Errorf("failed to read test-stream.ts: %w", err)
	}

	// Create HTTP file server
	mux := http.NewServeMux()

	// Handle /live/ paths by serving the test-stream.ts content
	// This allows proxy/relay tests to fetch upstream stream content
	mux.HandleFunc("/live/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/MP2T")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(testStreamData)))
		w.WriteHeader(http.StatusOK)
		w.Write(testStreamData)
	})

	// Handle /logos/ paths by serving the channel.webp content
	mux.HandleFunc("/logos/", func(w http.ResponseWriter, r *http.Request) {
		channelLogoData, err := fs.ReadFile(subFS, "channel.webp")
		if err != nil {
			http.Error(w, "logo not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/webp")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(channelLogoData)))
		w.WriteHeader(http.StatusOK)
		w.Write(channelLogoData)
	})

	// Handle /programs/ paths by serving the program.webp content
	mux.HandleFunc("/programs/", func(w http.ResponseWriter, r *http.Request) {
		programLogoData, err := fs.ReadFile(subFS, "program.webp")
		if err != nil {
			http.Error(w, "logo not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/webp")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(programLogoData)))
		w.WriteHeader(http.StatusOK)
		w.Write(programLogoData)
	})

	// Serve static files for other paths
	mux.Handle("/", http.FileServer(http.FS(subFS)))

	ts := &TestdataServer{
		listener: listener,
		baseURL:  fmt.Sprintf("http://127.0.0.1:%d", port),
		server: &http.Server{
			Handler: mux,
		},
	}

	return ts, nil
}

// Start starts the testdata server.
func (ts *TestdataServer) Start() {
	go ts.server.Serve(ts.listener)
}

// Stop stops the testdata server.
func (ts *TestdataServer) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ts.server.Shutdown(ctx)
}

// BaseURL returns the base URL for the testdata server.
func (ts *TestdataServer) BaseURL() string {
	return ts.baseURL
}

// StreamURL returns the URL to the test-stream.ts file.
func (ts *TestdataServer) StreamURL() string {
	return ts.baseURL + "/test-stream.ts"
}

// ChannelLogoURL returns the URL to the channel.webp file.
func (ts *TestdataServer) ChannelLogoURL() string {
	return ts.baseURL + "/channel.webp"
}

// ProgramLogoURL returns the URL to the program.webp file.
func (ts *TestdataServer) ProgramLogoURL() string {
	return ts.baseURL + "/program.webp"
}

// isFFmpegAvailable checks if ffmpeg is available on the system.
func isFFmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}
