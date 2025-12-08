// Package main provides an E2E test runner for validating the tvarr pipeline.
// This binary tests the complete flow from source ingestion through M3U/XMLTV output.
//
//nolint:errcheck,gocognit,gocyclo,nestif,gocritic,godot,wrapcheck,gosec,revive,goprintffuncname,modernize // E2E test runner uses relaxed linting for cleaner test code
package main

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed testdata/channel.webp testdata/program.webp testdata/test.m3u testdata/test.xml testdata/test-stream.ts
var testdataFS embed.FS

// E2E Test Data Sources - publicly accessible, compatible M3U and EPG data
const (
	// DefaultM3UURL is the default M3U stream source for E2E testing
	// Free-TV UK playlist has some channels with matching EPG IDs
	DefaultM3UURL = "https://raw.githubusercontent.com/Free-TV/IPTV/refs/heads/master/playlists/playlist_uk.m3u8"

	// DefaultEPGURL is the default EPG source for E2E testing
	// epgshare01 UK EPG has ~5 channels that match the Free-TV playlist
	// Matching channels: 4seven.uk, HobbyMaker.uk, JewelleryMaker.uk, S4C.uk, TJC.uk
	DefaultEPGURL = "https://epgshare01.online/epgshare01/epg_ripper_UK1.xml.gz"

	// DefaultTimeout for operations
	DefaultTimeout = 5 * time.Minute
)

// findFreePort finds an available high port (never 8080) for the E2E server
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

// ManagedServer represents a tvarr server managed by the E2E runner
type ManagedServer struct {
	cmd       *exec.Cmd
	port      int
	dataDir   string
	baseURL   string
	startErr  error
	logBuffer *logCapture
}

// logCapture captures log output while also writing to the original writer
type logCapture struct {
	buffer bytes.Buffer
	mu     sync.Mutex
	writer io.Writer
}

func newLogCapture(w io.Writer) *logCapture {
	return &logCapture{writer: w}
}

func (lc *logCapture) Write(p []byte) (n int, err error) {
	lc.mu.Lock()
	lc.buffer.Write(p)
	lc.mu.Unlock()
	return lc.writer.Write(p)
}

func (lc *logCapture) Contains(s string) bool {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	return strings.Contains(lc.buffer.String(), s)
}

// NewManagedServer creates and starts a new tvarr server on a random port
func NewManagedServer(binaryPath string) (*ManagedServer, error) {
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

	// Capture tvarr server output to stderr (both stdout and stderr from server)
	// This keeps e2e-runner output on stdout separate from server logs on stderr
	logBuffer := newLogCapture(os.Stderr)
	cmd.Stdout = logBuffer
	cmd.Stderr = logBuffer

	ms := &ManagedServer{
		cmd:       cmd,
		port:      port,
		dataDir:   dataDir,
		baseURL:   fmt.Sprintf("http://localhost:%d", port),
		logBuffer: logBuffer,
	}

	return ms, nil
}

// Start starts the managed server and waits for it to be ready
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

// Stop stops the managed server and cleans up
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

// Port returns the port the server is running on
func (ms *ManagedServer) Port() int {
	return ms.port
}

// BaseURL returns the base URL for the server
func (ms *ManagedServer) BaseURL() string {
	return ms.baseURL
}

// DataDir returns the data directory path
func (ms *ManagedServer) DataDir() string {
	return ms.dataDir
}

// LogContains checks if the server logs contain a specific string
func (ms *ManagedServer) LogContains(s string) bool {
	if ms.logBuffer == nil {
		return false
	}
	return ms.logBuffer.Contains(s)
}

// TestdataServer serves embedded testdata files over HTTP
type TestdataServer struct {
	server   *http.Server
	listener net.Listener
	baseURL  string
}

// NewTestdataServer creates a testdata HTTP server on a random port
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

// Start starts the testdata server
func (ts *TestdataServer) Start() {
	go ts.server.Serve(ts.listener)
}

// Stop stops the testdata server
func (ts *TestdataServer) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ts.server.Shutdown(ctx)
}

// BaseURL returns the base URL for the testdata server
func (ts *TestdataServer) BaseURL() string {
	return ts.baseURL
}

// StreamURL returns the URL to the test-stream.ts file
func (ts *TestdataServer) StreamURL() string {
	return ts.baseURL + "/test-stream.ts"
}

// ChannelLogoURL returns the URL to the channel.webp file
func (ts *TestdataServer) ChannelLogoURL() string {
	return ts.baseURL + "/channel.webp"
}

// ProgramLogoURL returns the URL to the program.webp file
func (ts *TestdataServer) ProgramLogoURL() string {
	return ts.baseURL + "/program.webp"
}

// isFFmpegAvailable checks if ffmpeg is available on the system
func isFFmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// TestResult represents the outcome of a test step
type TestResult struct {
	Name    string
	Passed  bool
	Message string
	Elapsed time.Duration
}

// ProgressEvent represents a captured SSE progress event
type ProgressEvent struct {
	Timestamp         time.Time
	OwnerID           string
	OwnerType         string
	OwnerName         string
	OperationName     string
	OperationType     string
	State             string
	CurrentStage      string
	OverallPercentage float64
	StagePercentage   float64
	Error             string
}

// SSECollector collects SSE events in the background
type SSECollector struct {
	baseURL    string
	events     []ProgressEvent
	mu         sync.Mutex
	cancel     context.CancelFunc
	done       chan struct{}
	httpClient *http.Client
	startTime  time.Time
}

// NewSSECollector creates a new SSE collector
func NewSSECollector(baseURL string) *SSECollector {
	return &SSECollector{
		baseURL:    baseURL,
		events:     make([]ProgressEvent, 0),
		done:       make(chan struct{}),
		httpClient: &http.Client{Timeout: 0}, // No timeout for SSE
		startTime:  time.Now(),
	}
}

// Start begins collecting SSE events in the background
func (c *SSECollector) Start(ctx context.Context) error {
	ctx, c.cancel = context.WithCancel(ctx)

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/progress/events", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	go func() {
		defer resp.Body.Close()
		defer close(c.done)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				var event map[string]interface{}
				if err := json.Unmarshal([]byte(data), &event); err != nil {
					continue
				}

				pe := ProgressEvent{
					Timestamp: time.Now(),
				}
				if v, ok := event["owner_id"].(string); ok {
					pe.OwnerID = v
				}
				if v, ok := event["owner_type"].(string); ok {
					pe.OwnerType = v
				}
				if v, ok := event["owner_name"].(string); ok {
					pe.OwnerName = v
				}
				if v, ok := event["operation_name"].(string); ok {
					pe.OperationName = v
				}
				if v, ok := event["operation_type"].(string); ok {
					pe.OperationType = v
				}
				if v, ok := event["state"].(string); ok {
					pe.State = v
				}
				if v, ok := event["current_stage"].(string); ok {
					pe.CurrentStage = v
				}
				if v, ok := event["overall_percentage"].(float64); ok {
					pe.OverallPercentage = v
				}
				if v, ok := event["stage_percentage"].(float64); ok {
					pe.StagePercentage = v
				}
				if v, ok := event["error"].(string); ok {
					pe.Error = v
				}

				c.mu.Lock()
				c.events = append(c.events, pe)
				c.mu.Unlock()
			}
		}
	}()

	return nil
}

// Stop stops collecting SSE events
func (c *SSECollector) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	// Wait for the goroutine to finish (with timeout)
	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
		// Force close - the goroutine will exit when it tries to read
	}
	// Close idle connections to ensure no lingering goroutines
	c.httpClient.CloseIdleConnections()
}

// GetEvents returns a copy of all collected events
func (c *SSECollector) GetEvents() []ProgressEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]ProgressEvent, len(c.events))
	copy(result, c.events)
	return result
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return "<1ms"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", minutes, seconds)
}

// truncateString truncates a string to maxLen, adding "..." if truncated
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 4 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// operationInfo holds processed information about an operation for the waterfall view
type operationInfo struct {
	name          string
	ownerID       string
	ownerType     string
	ownerName     string
	operationType string
	firstTime     time.Time
	lastTime      time.Time
	finalState    string
	error         string
	stages        []stageInfo
}

// stageInfo holds information about a stage within an operation
type stageInfo struct {
	name      string
	startTime time.Time
	endTime   time.Time
	duration  time.Duration
	updates   int
}

// PrintTimeline prints a waterfall-style timeline of all events grouped by operation
func (c *SSECollector) PrintTimeline() {
	events := c.GetEvents()
	if len(events) == 0 {
		fmt.Println("\nNo SSE events captured.")
		return
	}

	// Find the global start time
	globalStart := events[0].Timestamp

	// Group events by owner_id (which uniquely identifies an operation instance)
	operationsByOwner := make(map[string][]ProgressEvent)
	ownerOrder := make([]string, 0)
	for _, e := range events {
		key := e.OwnerID
		if key == "" {
			key = "unknown"
		}
		if _, exists := operationsByOwner[key]; !exists {
			ownerOrder = append(ownerOrder, key)
		}
		operationsByOwner[key] = append(operationsByOwner[key], e)
	}

	// Process each operation into structured info
	operations := make([]operationInfo, 0, len(ownerOrder))
	for _, ownerID := range ownerOrder {
		opEvents := operationsByOwner[ownerID]
		if len(opEvents) == 0 {
			continue
		}

		op := operationInfo{
			ownerID:   ownerID,
			firstTime: opEvents[0].Timestamp,
			lastTime:  opEvents[len(opEvents)-1].Timestamp,
		}

		// Get operation name, type, owner type, and owner name (use the most descriptive ones)
		for _, e := range opEvents {
			if e.OperationName != "" {
				op.name = e.OperationName
			}
			if e.OperationType != "" {
				op.operationType = e.OperationType
			}
			if e.OwnerType != "" {
				op.ownerType = e.OwnerType
			}
			if e.OwnerName != "" {
				op.ownerName = e.OwnerName
			}
			// Stop once we have all fields
			if op.name != "" && op.operationType != "" && op.ownerType != "" && op.ownerName != "" {
				break
			}
		}
		if op.name == "" {
			op.name = "Unknown Operation"
		}

		// Track stages in order with timing
		stageMap := make(map[string]*stageInfo)
		stageOrder := make([]string, 0)
		for _, e := range opEvents {
			if e.CurrentStage == "" {
				continue
			}
			if s, exists := stageMap[e.CurrentStage]; exists {
				s.endTime = e.Timestamp
				s.updates++
			} else {
				stageOrder = append(stageOrder, e.CurrentStage)
				stageMap[e.CurrentStage] = &stageInfo{
					name:      e.CurrentStage,
					startTime: e.Timestamp,
					endTime:   e.Timestamp,
					updates:   1,
				}
			}
		}

		// Calculate durations and build stage list
		for _, stageName := range stageOrder {
			s := stageMap[stageName]
			s.duration = s.endTime.Sub(s.startTime)
			op.stages = append(op.stages, *s)
		}

		// Get final state and any error
		for i := len(opEvents) - 1; i >= 0; i-- {
			if opEvents[i].State != "" {
				op.finalState = opEvents[i].State
				break
			}
		}
		for _, e := range opEvents {
			if e.Error != "" {
				op.error = e.Error
				break
			}
		}

		operations = append(operations, op)
	}

	// Print waterfall header
	fmt.Println("\n" + strings.Repeat("=", 90))
	fmt.Println("SSE Progress Waterfall Timeline")
	fmt.Println(strings.Repeat("=", 90))
	fmt.Println()
	fmt.Printf("%-8s %-50s %-10s %s\n", "Time(s)", "Operation / Stage", "Status", "Duration")
	fmt.Println(strings.Repeat("-", 90))

	// Print each operation in waterfall style
	for i, op := range operations {
		relStart := op.firstTime.Sub(globalStart).Seconds()
		totalDuration := op.lastTime.Sub(op.firstTime)

		// Operation header with status icon
		statusIcon := "..."
		switch op.finalState {
		case "completed":
			statusIcon = "OK"
		case "error":
			statusIcon = "ERR"
		}

		// Build operation type header (e.g., "STREAM_INGESTION | My Test Source")
		// Prefer ownerName (the source/proxy name) over operationName for the header
		opTypeHeader := ""
		if op.operationType != "" || op.ownerName != "" {
			parts := make([]string, 0, 2)
			if op.operationType != "" {
				parts = append(parts, strings.ToUpper(op.operationType))
			}
			if op.ownerName != "" {
				parts = append(parts, op.ownerName)
			}
			opTypeHeader = strings.Join(parts, " | ")
		}

		// Truncate long operation names
		opNameDisplay := op.name
		maxNameLen := 46
		if opTypeHeader != "" {
			maxNameLen = 42 // Leave room for type header
		}
		if len(opNameDisplay) > maxNameLen {
			opNameDisplay = opNameDisplay[:maxNameLen-3] + "..."
		}

		// Print operation type header if available
		if opTypeHeader != "" {
			fmt.Printf("         %-50s\n", opTypeHeader)
		}

		fmt.Printf("%-8.2f [%-46s] %-10s %s\n",
			relStart,
			opNameDisplay,
			statusIcon,
			formatDuration(totalDuration))

		// Print stages with tree-style indentation
		for j, stage := range op.stages {
			stageRelStart := stage.startTime.Sub(globalStart).Seconds()

			// Tree connector
			connector := "|--"
			if j == len(op.stages)-1 {
				connector = "+--"
			}

			// Truncate long stage names
			stageNameDisplay := stage.name
			if len(stageNameDisplay) > 38 {
				stageNameDisplay = stageNameDisplay[:35] + "..."
			}

			fmt.Printf("  %-6.2f %s %-38s %-10s %s\n",
				stageRelStart,
				connector,
				stageNameDisplay,
				"",
				formatDuration(stage.duration))
		}

		// Print error if any
		if op.error != "" {
			fmt.Printf("         !-- ERROR: %s\n", truncateString(op.error, 60))
		}

		// Add separator between operations (but not after the last one)
		if i < len(operations)-1 {
			fmt.Println(strings.Repeat("-", 90))
		}
	}

	// Print overall statistics
	fmt.Println(strings.Repeat("=", 90))

	// Calculate total time span
	totalDuration := events[len(events)-1].Timestamp.Sub(globalStart)
	fmt.Printf("Total Duration: %s | Events: %d | Operations: %d\n",
		formatDuration(totalDuration), len(events), len(operations))

	// Count by state
	stateCounts := make(map[string]int)
	for _, op := range operations {
		if op.finalState != "" {
			stateCounts[op.finalState]++
		}
	}
	if len(stateCounts) > 0 {
		fmt.Print("Operation Results: ")
		states := make([]string, 0, len(stateCounts))
		for s := range stateCounts {
			states = append(states, s)
		}
		sort.Strings(states)
		for i, s := range states {
			if i > 0 {
				fmt.Print(", ")
			}
			fmt.Printf("%s=%d", s, stateCounts[s])
		}
		fmt.Println()
	}
}

// APIClient wraps HTTP calls to the tvarr API
type APIClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewAPIClient creates a new API client
func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // Long timeout for proxy generation with logo caching
		},
	}
}

// HealthCheck verifies the API is accessible
func (c *APIClient) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}
	return nil
}

// CreateStreamSource creates a new M3U stream source
func (c *APIClient) CreateStreamSource(ctx context.Context, name, url string) (string, error) {
	body := map[string]string{
		"name": name,
		"type": "m3u",
		"url":  url,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/sources/stream", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create stream source failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	id, ok := result["id"].(string)
	if !ok {
		return "", fmt.Errorf("response missing id field")
	}
	return id, nil
}

// CreateEPGSource creates a new XMLTV EPG source
func (c *APIClient) CreateEPGSource(ctx context.Context, name, url string) (string, error) {
	body := map[string]string{
		"name": name,
		"type": "xmltv",
		"url":  url,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/sources/epg", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create EPG source failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	id, ok := result["id"].(string)
	if !ok {
		return "", fmt.Errorf("response missing id field")
	}
	return id, nil
}

// TriggerIngestion triggers ingestion for a source and waits for completion
func (c *APIClient) TriggerIngestion(ctx context.Context, sourceType, sourceID string, timeout time.Duration) error {
	// Start SSE listener for progress
	progressDone := make(chan error, 1)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Trigger ingestion
	var ingestURL string
	if sourceType == "stream" {
		ingestURL = fmt.Sprintf("%s/api/v1/sources/stream/%s/ingest", c.baseURL, sourceID)
	} else {
		ingestURL = fmt.Sprintf("%s/api/v1/sources/epg/%s/ingest", c.baseURL, sourceID)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", ingestURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("trigger ingestion failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("trigger ingestion failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// Wait for completion via SSE
	go func() {
		progressDone <- c.waitForSSECompletion(ctx, sourceID)
	}()

	select {
	case err := <-progressDone:
		return err
	case <-ctx.Done():
		return fmt.Errorf("ingestion timed out after %v", timeout)
	}
}

// waitForSSECompletion monitors SSE events for completion
func (c *APIClient) waitForSSECompletion(ctx context.Context, ownerID string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/progress/events", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			var event map[string]interface{}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			// Check if this event is for our source
			eventOwnerID, _ := event["owner_id"].(string)
			if eventOwnerID != ownerID {
				continue
			}

			// Check for completion or error
			state, _ := event["state"].(string)
			switch state {
			case "completed":
				return nil
			case "error":
				errMsg, _ := event["error"].(string)
				return fmt.Errorf("ingestion failed: %s", errMsg)
			}
		}
	}

	return scanner.Err()
}

// GetChannelCount returns the number of channels in the system
func (c *APIClient) GetChannelCount(ctx context.Context) (int, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/channels?limit=1", nil)
	if err != nil {
		return 0, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	total, ok := result["total"].(float64)
	if !ok {
		// Try to count items array
		items, ok := result["items"].([]interface{})
		if ok {
			return len(items), nil
		}
		channels, ok := result["channels"].([]interface{})
		if ok {
			return len(channels), nil
		}
		return 0, nil
	}
	return int(total), nil
}

// CreateStreamProxyOptions holds options for creating a stream proxy
type CreateStreamProxyOptions struct {
	Name              string
	StreamSourceIDs   []string
	EpgSourceIDs      []string
	CacheChannelLogos bool
	CacheProgramLogos bool
	ProxyMode         string // "direct" (HTTP 302 redirect) or "smart" (auto-optimizes delivery)
	RelayProfileID    string // Optional relay profile ID for transcoding (only used with "smart" mode)
}

// CreateStreamProxy creates a new stream proxy
func (c *APIClient) CreateStreamProxy(ctx context.Context, opts CreateStreamProxyOptions) (string, error) {
	body := map[string]interface{}{
		"name":                opts.Name,
		"source_ids":          opts.StreamSourceIDs,
		"epg_source_ids":      opts.EpgSourceIDs,
		"cache_channel_logos": opts.CacheChannelLogos,
		"cache_program_logos": opts.CacheProgramLogos,
	}
	// Add proxy_mode if specified (valid values: "direct", "smart")
	if opts.ProxyMode != "" {
		body["proxy_mode"] = opts.ProxyMode
	}
	// Add relay_profile_id if specified (enables transcoding with "smart" mode)
	if opts.RelayProfileID != "" {
		body["relay_profile_id"] = opts.RelayProfileID
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/proxies", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create proxy failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	id, ok := result["id"].(string)
	if !ok {
		return "", fmt.Errorf("response missing id field")
	}
	return id, nil
}

// TriggerProxyGeneration triggers proxy generation and waits for completion.
// Note: The generate endpoint is async - it starts generation in a goroutine.
// We poll the proxy status until generation completes (success/failed).
func (c *APIClient) TriggerProxyGeneration(ctx context.Context, proxyID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Trigger regeneration - this is async
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/proxies/"+proxyID+"/regenerate", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("trigger generation failed: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("trigger generation failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// Poll for completion - generation runs asynchronously
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("generation timeout: %w", ctx.Err())
		case <-ticker.C:
			proxy, err := c.GetProxy(ctx, proxyID)
			if err != nil {
				return fmt.Errorf("failed to check generation status: %w", err)
			}
			status, _ := proxy["status"].(string)
			switch status {
			case "success":
				return nil
			case "failed":
				lastError, _ := proxy["last_error"].(string)
				return fmt.Errorf("generation failed: %s", lastError)
			case "generating", "pending":
				// Still running, continue polling
				continue
			default:
				return fmt.Errorf("unknown proxy status: %s", status)
			}
		}
	}
}

// GetProxy fetches a proxy by ID
func (c *APIClient) GetProxy(ctx context.Context, proxyID string) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/proxies/"+proxyID, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get proxy failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetProxyM3U fetches the M3U output for a proxy
func (c *APIClient) GetProxyM3U(ctx context.Context, proxyID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/proxy/"+proxyID+".m3u", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch M3U failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("fetch M3U failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read M3U body failed: %w", err)
	}

	return string(data), nil
}

// GetProxyXMLTV fetches the XMLTV output for a proxy
func (c *APIClient) GetProxyXMLTV(ctx context.Context, proxyID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/proxy/"+proxyID+".xmltv", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch XMLTV failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("fetch XMLTV failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read XMLTV body failed: %w", err)
	}

	return string(data), nil
}

// StreamTestResult contains the result of testing a stream URL.
type StreamTestResult struct {
	StatusCode      int
	Headers         http.Header
	Location        string // For redirects
	ContentType     string
	BytesReceived   int
	HasCORSHeaders  bool
	TSSyncByteValid bool // First byte is 0x47
}

// GetFirstChannelID gets the first channel ID for a given source ID.
// The stream proxy endpoint expects /proxy/{proxyId}/{channelId}, so we need
// to get the actual channel ULID from the channels API.
func (c *APIClient) GetFirstChannelID(ctx context.Context, sourceID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		c.baseURL+"/api/v1/channels?source_id="+sourceID+"&limit=1", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("get channels failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get channels failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode channels response: %w", err)
	}

	if len(result.Items) == 0 {
		return "", fmt.Errorf("no channels found for source %s", sourceID)
	}

	return result.Items[0].ID, nil
}

// GetProxyStreamURL constructs the URL to stream a channel through a proxy.
// The format is /proxy/{proxyId}/{channelId}.
func (c *APIClient) GetProxyStreamURL(proxyID, channelID string) string {
	return fmt.Sprintf("%s/proxy/%s/%s", c.baseURL, proxyID, channelID)
}

// TestStreamRequest tests a stream URL and returns information about the response.
// It does not follow redirects automatically.
func (c *APIClient) TestStreamRequest(ctx context.Context, streamURL string) (*StreamTestResult, error) {
	// Create client that doesn't follow redirects
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", streamURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stream request failed: %w", err)
	}
	defer resp.Body.Close()

	result := &StreamTestResult{
		StatusCode:  resp.StatusCode,
		Headers:     resp.Header,
		ContentType: resp.Header.Get("Content-Type"),
	}

	// Check for redirect location
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently {
		result.Location = resp.Header.Get("Location")
	}

	// Check for CORS headers
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		result.HasCORSHeaders = true
	}

	// For 200 responses, read a bit of content to verify it's valid
	if resp.StatusCode == http.StatusOK {
		buf := make([]byte, 188*2) // Read enough for 2 TS packets
		n, _ := io.ReadAtLeast(resp.Body, buf, 1)
		result.BytesReceived = n

		// Check TS sync byte
		if n > 0 && buf[0] == 0x47 {
			result.TSSyncByteValid = true
		}
	}

	return result, nil
}

// UploadLogoResult contains the result of uploading a logo.
type UploadLogoResult struct {
	ID  string // ULID of the uploaded logo
	URL string // Full URL to access the logo
}

// UploadLogo uploads a logo file and returns the ID and URL to access it
func (c *APIClient) UploadLogo(ctx context.Context, name string, fileData []byte) (*UploadLogoResult, error) {
	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add the file field
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		return nil, fmt.Errorf("create form file failed: %w", err)
	}
	if _, err := part.Write(fileData); err != nil {
		return nil, fmt.Errorf("write file data failed: %w", err)
	}

	// Add the name field
	if err := writer.WriteField("name", name); err != nil {
		return nil, fmt.Errorf("write name field failed: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close writer failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/logos/upload", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload logo failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upload logo failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	id, ok := result["id"].(string)
	if !ok {
		return nil, fmt.Errorf("response missing id field")
	}
	url, ok := result["url"].(string)
	if !ok {
		return nil, fmt.Errorf("response missing url field")
	}
	return &UploadLogoResult{ID: id, URL: url}, nil
}

// CreateDataMappingRule creates a new data mapping rule
func (c *APIClient) CreateDataMappingRule(ctx context.Context, name, sourceType, expression string, priority int) (string, error) {
	isEnabled := true
	body := map[string]interface{}{
		"name":        name,
		"source_type": sourceType,
		"expression":  expression,
		"priority":    priority,
		"is_enabled":  isEnabled,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/data-mapping", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("create data mapping rule failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create data mapping rule failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response failed: %w", err)
	}

	id, ok := result["id"].(string)
	if !ok {
		return "", fmt.Errorf("response missing id field")
	}
	return id, nil
}

// GetRelayProfiles fetches all relay profiles
func (c *APIClient) GetRelayProfiles(ctx context.Context) ([]map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/relay/profiles", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch relay profiles failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch relay profiles failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	profiles, ok := result["profiles"].([]interface{})
	if !ok {
		// Also try "items" key
		profiles, ok = result["items"].([]interface{})
		if !ok {
			return nil, nil // No profiles
		}
	}

	var profileMaps []map[string]interface{}
	for _, p := range profiles {
		if pm, ok := p.(map[string]interface{}); ok {
			profileMaps = append(profileMaps, pm)
		}
	}
	return profileMaps, nil
}

// GetFirstRelayProfileID returns the ID of the first relay profile, or empty string if none
func (c *APIClient) GetFirstRelayProfileID(ctx context.Context) (string, error) {
	profiles, err := c.GetRelayProfiles(ctx)
	if err != nil {
		return "", err
	}
	if len(profiles) == 0 {
		return "", nil
	}
	id, ok := profiles[0]["id"].(string)
	if !ok {
		return "", nil
	}
	return id, nil
}

// ClientDetectionMapping represents a client detection rule
type ClientDetectionMapping struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Description         string   `json:"description"`
	Expression          string   `json:"expression"`
	Priority            int      `json:"priority"`
	IsEnabled           bool     `json:"is_enabled"`
	IsSystem            bool     `json:"is_system"`
	AcceptedVideoCodecs []string `json:"accepted_video_codecs"`
	AcceptedAudioCodecs []string `json:"accepted_audio_codecs"`
	AcceptedContainers  []string `json:"accepted_containers"`
	PreferredVideoCodec string   `json:"preferred_video_codec"`
	PreferredAudioCodec string   `json:"preferred_audio_codec"`
	PreferredContainer  string   `json:"preferred_container"`
}

// ClientDetectionStats represents statistics about client detection mappings
type ClientDetectionStats struct {
	Total   int `json:"total"`
	Enabled int `json:"enabled"`
	System  int `json:"system"`
	Custom  int `json:"custom"`
}

// GetClientDetectionMappings fetches all client detection mappings
func (c *APIClient) GetClientDetectionMappings(ctx context.Context) ([]ClientDetectionMapping, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/relay-profile-mappings", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch client detection mappings failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch client detection mappings failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Mappings []ClientDetectionMapping `json:"mappings"`
		Count    int                       `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	return result.Mappings, nil
}

// GetClientDetectionStats fetches client detection statistics
func (c *APIClient) GetClientDetectionStats(ctx context.Context) (*ClientDetectionStats, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/relay-profile-mappings/stats", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch client detection stats failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch client detection stats failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var stats ClientDetectionStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	return &stats, nil
}

// TestClientDetectionExpression tests an expression against sample data
func (c *APIClient) TestClientDetectionExpression(ctx context.Context, expression string, testData map[string]string) (bool, error) {
	body := map[string]interface{}{
		"expression": expression,
		"test_data":  testData,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/relay-profile-mappings/test", bytes.NewReader(jsonBody))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("test expression failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("test expression failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Matches bool `json:"matches"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("decode response failed: %w", err)
	}

	return result.Matches, nil
}

// ValidateM3U checks if the M3U content is valid
func ValidateM3U(content string) (channelCount int, err error) {
	if !strings.HasPrefix(content, "#EXTM3U") {
		return 0, fmt.Errorf("invalid M3U: missing #EXTM3U header")
	}

	// Count EXTINF entries (one per channel)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "#EXTINF:") {
			channelCount++
		}
	}

	if channelCount == 0 {
		return 0, fmt.Errorf("invalid M3U: no channels found")
	}

	return channelCount, nil
}

// ValidateXMLTV checks if the XMLTV content is valid
func ValidateXMLTV(content string) (channelCount, programCount int, err error) {
	if !strings.Contains(content, "<?xml") || !strings.Contains(content, "<tv") {
		return 0, 0, fmt.Errorf("invalid XMLTV: missing XML declaration or tv element")
	}

	// Count channel and programme elements
	channelCount = strings.Count(content, "<channel ")
	if channelCount == 0 {
		channelCount = strings.Count(content, "<channel>")
	}
	programCount = strings.Count(content, "<programme ")
	if programCount == 0 {
		programCount = strings.Count(content, "<programme>")
	}

	if channelCount == 0 {
		return 0, 0, fmt.Errorf("invalid XMLTV: no channels found")
	}

	return channelCount, programCount, nil
}

// E2ERunner runs the E2E test suite
type E2ERunner struct {
	client            *APIClient
	m3uURL            string
	epgURL            string
	verbose           bool
	results           []TestResult
	runID             string          // Unique ID for this test run to avoid name collisions
	cacheChannelLogos bool            // Enable channel logo caching
	cacheProgramLogos bool            // Enable program logo caching
	sseCollector      *SSECollector   // Collects SSE events for timeline
	channelLogoID     string          // ULID of uploaded channel logo placeholder
	channelLogoURL    string          // URL of uploaded channel logo placeholder
	programLogoID     string          // ULID of uploaded program logo placeholder
	programLogoURL    string          // URL of uploaded program logo placeholder
	outputDir         string          // Directory to write artifact files (m3u/xmltv)
	showSamples       bool            // Display sample channels and programs to stdout
	expectedChannels  int             // Expected channel count in output (0 to skip validation)
	expectedPrograms  int             // Expected program count in output (0 to skip validation)
	server            *ManagedServer  // Reference to managed server for log validation
	ffmpegAvailable   bool            // Whether ffmpeg is available for relay tests
	testdataServer    *TestdataServer // Server for serving testdata files
	testProxyModes    bool            // Whether to test different proxy modes
}

// E2ERunnerOptions holds configuration options for the E2E runner
type E2ERunnerOptions struct {
	BaseURL           string
	M3UURL            string
	EPGURL            string
	Verbose           bool
	CacheChannelLogos bool
	CacheProgramLogos bool
	OutputDir         string
	ShowSamples       bool
	ExpectedChannels  int             // Expected channel count in output (0 to skip validation)
	ExpectedPrograms  int             // Expected program count in output (0 to skip validation)
	Server            *ManagedServer  // Reference to managed server for log validation
	TestProxyModes    bool            // Whether to test different proxy modes (redirect, proxy, relay)
	TestdataServer    *TestdataServer // Pre-created testdata server (optional, one will be created if nil and TestProxyModes is true)
}

// NewE2ERunner creates a new E2E runner
func NewE2ERunner(opts E2ERunnerOptions) *E2ERunner {
	// Generate a unique run ID using timestamp
	runID := fmt.Sprintf("%d", time.Now().UnixNano())
	return &E2ERunner{
		client:            NewAPIClient(opts.BaseURL),
		m3uURL:            opts.M3UURL,
		epgURL:            opts.EPGURL,
		verbose:           opts.Verbose,
		runID:             runID,
		cacheChannelLogos: opts.CacheChannelLogos,
		cacheProgramLogos: opts.CacheProgramLogos,
		sseCollector:      NewSSECollector(opts.BaseURL),
		outputDir:         opts.OutputDir,
		showSamples:       opts.ShowSamples,
		expectedChannels:  opts.ExpectedChannels,
		expectedPrograms:  opts.ExpectedPrograms,
		server:            opts.Server,
		ffmpegAvailable:   isFFmpegAvailable(),
		testProxyModes:    opts.TestProxyModes,
		testdataServer:    opts.TestdataServer, // Use pre-created testdata server if provided
	}
}

// log prints a message if verbose mode is enabled
func (r *E2ERunner) log(format string, args ...interface{}) {
	if r.verbose {
		fmt.Printf(format+"\n", args...)
	}
}

// runTest executes a test and records the result
func (r *E2ERunner) runTest(name string, fn func() error) {
	start := time.Now()
	r.log("Running: %s", name)

	err := fn()
	elapsed := time.Since(start)

	result := TestResult{
		Name:    name,
		Passed:  err == nil,
		Elapsed: elapsed,
	}

	if err != nil {
		result.Message = err.Error()
		r.log("  FAILED: %s (%.2fs)", err.Error(), elapsed.Seconds())
	} else {
		result.Message = "OK"
		r.log("  PASSED (%.2fs)", elapsed.Seconds())
	}

	r.results = append(r.results, result)
}

// Run executes the full E2E test suite
func (r *E2ERunner) Run(ctx context.Context) error {
	var streamSourceID, epgSourceID, proxyID string

	// Start SSE collector before any tests
	if err := r.sseCollector.Start(ctx); err != nil {
		r.log("Warning: Failed to start SSE collector: %v", err)
	}
	defer r.sseCollector.Stop()

	// Give SSE connection time to establish
	time.Sleep(500 * time.Millisecond)

	// Start testdata server if testing proxy modes and one wasn't already provided
	if r.testProxyModes {
		if r.testdataServer == nil {
			// No testdata server was provided, create and start one
			ts, err := NewTestdataServer()
			if err != nil {
				r.log("Warning: Failed to create testdata server: %v", err)
			} else {
				r.testdataServer = ts
				r.testdataServer.Start()
				defer r.testdataServer.Stop()
				r.log("Testdata server started at: %s", r.testdataServer.BaseURL())
			}
		} else {
			// Testdata server was pre-created (for test data generation), just log its URL
			r.log("Using pre-created testdata server at: %s", r.testdataServer.BaseURL())
		}

		// Check for FFmpeg availability - warn if not present (print to both stdout and stderr for CI visibility)
		if !r.ffmpegAvailable {
			fmt.Println("WARNING: ffmpeg not found in PATH, relay profile tests will be skipped")
			fmt.Fprintln(os.Stderr, "WARNING: ffmpeg not found in PATH, relay profile tests will be skipped")
		}
	}

	// Phase 1: Setup - Health Check
	r.runTest("Health Check", func() error {
		return r.client.HealthCheck(ctx)
	})

	// Phase 1.1: Client Detection Rules (tests migration and rule engine)
	r.runTest("Client Detection: List Mappings", func() error {
		mappings, err := r.client.GetClientDetectionMappings(ctx)
		if err != nil {
			return err
		}
		if len(mappings) == 0 {
			return fmt.Errorf("expected default client detection mappings, got none")
		}
		r.log("  Found %d client detection mappings", len(mappings))

		// Verify default mappings exist and are ordered by priority
		var lastPriority int = -1
		for _, m := range mappings {
			if m.Priority < lastPriority {
				return fmt.Errorf("mappings not ordered by priority: %s (priority %d) came after priority %d",
					m.Name, m.Priority, lastPriority)
			}
			lastPriority = m.Priority
		}
		r.log("  Mappings are correctly ordered by priority")
		return nil
	})

	r.runTest("Client Detection: Get Stats", func() error {
		stats, err := r.client.GetClientDetectionStats(ctx)
		if err != nil {
			return err
		}
		if stats.Total == 0 {
			return fmt.Errorf("expected non-zero total mappings")
		}
		if stats.System == 0 {
			return fmt.Errorf("expected non-zero system mappings")
		}
		if stats.Enabled == 0 {
			return fmt.Errorf("expected non-zero enabled mappings")
		}
		r.log("  Stats: total=%d, enabled=%d, system=%d, custom=%d",
			stats.Total, stats.Enabled, stats.System, stats.Custom)
		return nil
	})

	r.runTest("Client Detection: Test Expression - Chrome Match", func() error {
		// Test Chrome user agent detection
		testData := map[string]string{
			"user_agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		}
		matches, err := r.client.TestClientDetectionExpression(ctx, `user_agent contains "Chrome"`, testData)
		if err != nil {
			return err
		}
		if !matches {
			return fmt.Errorf("expected Chrome expression to match Chrome user agent")
		}
		r.log("  Chrome expression matched correctly")
		return nil
	})

	r.runTest("Client Detection: Test Expression - Safari Match", func() error {
		// Test Safari user agent detection (Safari without Chrome)
		testData := map[string]string{
			"user_agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
		}
		matches, err := r.client.TestClientDetectionExpression(ctx, `user_agent contains "Safari" AND user_agent not_contains "Chrome"`, testData)
		if err != nil {
			return err
		}
		if !matches {
			return fmt.Errorf("expected Safari expression to match Safari user agent")
		}
		r.log("  Safari expression matched correctly")
		return nil
	})

	r.runTest("Client Detection: Test Expression - Non-Match", func() error {
		// Test that Firefox doesn't match Chrome rule
		testData := map[string]string{
			"user_agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:120.0) Gecko/20100101 Firefox/120.0",
		}
		matches, err := r.client.TestClientDetectionExpression(ctx, `user_agent contains "Chrome"`, testData)
		if err != nil {
			return err
		}
		if matches {
			return fmt.Errorf("expected Chrome expression to NOT match Firefox user agent")
		}
		r.log("  Chrome expression correctly did not match Firefox")
		return nil
	})

	r.runTest("Client Detection: Verify Default Fallback Rule", func() error {
		mappings, err := r.client.GetClientDetectionMappings(ctx)
		if err != nil {
			return err
		}
		// Find the fallback rule (should be last with priority 999)
		// The fallback uses 'user_agent contains ""' which always matches (empty string in any string)
		var fallbackFound bool
		for _, m := range mappings {
			if m.Priority == 999 && m.Expression == `user_agent contains ""` {
				fallbackFound = true
				if !m.IsSystem {
					return fmt.Errorf("fallback rule should be a system rule")
				}
				if !m.IsEnabled {
					return fmt.Errorf("fallback rule should be enabled")
				}
				r.log("  Found fallback rule: %s (priority %d)", m.Name, m.Priority)
				break
			}
		}
		if !fallbackFound {
			return fmt.Errorf("expected fallback rule with priority 999 and expression 'user_agent contains \"\"'")
		}
		return nil
	})

	// Phase 1.5: Upload placeholder logos and create data mapping rules
	// This optimizes logo caching - all logos will point to the same local placeholder
	r.runTest("Upload Channel Logo Placeholder", func() error {
		channelLogoData, err := testdataFS.ReadFile("testdata/channel.webp")
		if err != nil {
			return fmt.Errorf("read channel.webp from embedded testdata: %w", err)
		}
		result, err := r.client.UploadLogo(ctx, "channel-placeholder.webp", channelLogoData)
		if err != nil {
			return err
		}
		r.channelLogoID = result.ID
		r.channelLogoURL = result.URL
		r.log("  Uploaded channel logo: ID=%s URL=%s", r.channelLogoID, r.channelLogoURL)
		return nil
	})

	r.runTest("Upload Program Logo Placeholder", func() error {
		programLogoData, err := testdataFS.ReadFile("testdata/program.webp")
		if err != nil {
			return fmt.Errorf("read program.webp from embedded testdata: %w", err)
		}
		result, err := r.client.UploadLogo(ctx, "program-placeholder.webp", programLogoData)
		if err != nil {
			return err
		}
		r.programLogoID = result.ID
		r.programLogoURL = result.URL
		r.log("  Uploaded program logo: ID=%s URL=%s", r.programLogoID, r.programLogoURL)
		return nil
	})

	r.runTest("Create Channel Logo Mapping Rule", func() error {
		if r.channelLogoID == "" {
			return fmt.Errorf("no channel logo ID available")
		}
		// Rule: Replace all channel logos with @logo:ULID helper reference
		// The @logo:ULID syntax is resolved by the logo caching stage to the full URL
		// This ensures logos are recognized as local and not fetched remotely
		expression := fmt.Sprintf(`tvg_logo starts_with "http" SET tvg_logo = "@logo:%s"`, r.channelLogoID)
		ruleID, err := r.client.CreateDataMappingRule(ctx,
			fmt.Sprintf("E2E Channel Logo Placeholder %s", r.runID),
			"stream",
			expression,
			100, // High priority
		)
		if err != nil {
			return err
		}
		r.log("  Created channel logo mapping rule: %s (sets @logo:%s)", ruleID, r.channelLogoID)
		return nil
	})

	r.runTest("Create Program Icon Mapping Rule", func() error {
		if r.programLogoID == "" {
			return fmt.Errorf("no program logo ID available")
		}
		// Rule: Replace all program icons with @logo:ULID helper reference
		// The @logo:ULID syntax is resolved by the logo caching stage to the full URL
		expression := fmt.Sprintf(`programme_icon starts_with "http" SET programme_icon = "@logo:%s"`, r.programLogoID)
		ruleID, err := r.client.CreateDataMappingRule(ctx,
			fmt.Sprintf("E2E Program Icon Placeholder %s", r.runID),
			"epg",
			expression,
			100, // High priority
		)
		if err != nil {
			return err
		}
		r.log("  Created program icon mapping rule: %s (sets @logo:%s)", ruleID, r.programLogoID)
		return nil
	})

	// Phase 2: Stream Source Ingestion
	r.runTest("Create Stream Source", func() error {
		var err error
		// Use unique name with runID to avoid conflicts
		streamSourceID, err = r.client.CreateStreamSource(ctx, fmt.Sprintf("E2E Test M3U %s", r.runID), r.m3uURL)
		if err != nil {
			return err
		}
		r.log("  Created stream source: %s", streamSourceID)
		return nil
	})

	r.runTest("Ingest Stream Source", func() error {
		if streamSourceID == "" {
			return fmt.Errorf("no stream source to ingest")
		}
		return r.client.TriggerIngestion(ctx, "stream", streamSourceID, 3*time.Minute)
	})

	r.runTest("Verify Channel Count", func() error {
		count, err := r.client.GetChannelCount(ctx)
		if err != nil {
			return err
		}
		r.log("  Channel count: %d", count)
		if count == 0 {
			return fmt.Errorf("no channels found after ingestion")
		}
		return nil
	})

	// Phase 3: EPG Source Ingestion
	r.runTest("Create EPG Source", func() error {
		var err error
		// Use unique name with runID to avoid conflicts
		epgSourceID, err = r.client.CreateEPGSource(ctx, fmt.Sprintf("E2E Test EPG %s", r.runID), r.epgURL)
		if err != nil {
			return err
		}
		r.log("  Created EPG source: %s", epgSourceID)
		return nil
	})

	r.runTest("Ingest EPG Source", func() error {
		if epgSourceID == "" {
			return fmt.Errorf("no EPG source to ingest")
		}
		return r.client.TriggerIngestion(ctx, "epg", epgSourceID, 5*time.Minute)
	})

	// Phase 4: Proxy Configuration and Generation
	// When testProxyModes is enabled, create a redirect mode proxy explicitly.
	// When not testing proxy modes, create a default proxy (which uses redirect mode).
	r.runTest("Create Stream Proxy", func() error {
		var err error
		sourceIDs := []string{}
		epgIDs := []string{}
		if streamSourceID != "" {
			sourceIDs = []string{streamSourceID}
		}
		if epgSourceID != "" {
			epgIDs = []string{epgSourceID}
		}

		opts := CreateStreamProxyOptions{
			StreamSourceIDs:   sourceIDs,
			EpgSourceIDs:      epgIDs,
			CacheChannelLogos: r.cacheChannelLogos,
			CacheProgramLogos: r.cacheProgramLogos,
		}

		if r.testProxyModes {
			// Create direct mode proxy with explicit mode for proxy mode testing
			opts.Name = fmt.Sprintf("E2E Direct Mode Proxy %s", r.runID)
			opts.ProxyMode = "direct"
		} else {
			// Create default proxy (defaults to direct mode)
			opts.Name = fmt.Sprintf("E2E Test Proxy %s", r.runID)
		}

		proxyID, err = r.client.CreateStreamProxy(ctx, opts)
		if err != nil {
			return err
		}
		r.log("  Created proxy: %s (mode=%s, logo caching: channel=%v, program=%v)",
			proxyID, opts.ProxyMode, r.cacheChannelLogos, r.cacheProgramLogos)
		return nil
	})

	r.runTest("Generate Proxy Output", func() error {
		if proxyID == "" {
			return fmt.Errorf("no proxy to generate")
		}
		return r.client.TriggerProxyGeneration(ctx, proxyID, 5*time.Minute)
	})

	// Phase 5: Output Validation
	r.runTest("Verify Proxy Status", func() error {
		if proxyID == "" {
			return fmt.Errorf("no proxy to verify")
		}
		proxy, err := r.client.GetProxy(ctx, proxyID)
		if err != nil {
			return err
		}
		status, _ := proxy["status"].(string)
		if status == "" {
			return fmt.Errorf("proxy status not found in response")
		}
		r.log("  Proxy status: %s", status)
		channelCount, _ := proxy["channel_count"].(float64)
		r.log("  Channel count: %.0f", channelCount)
		if channelCount == 0 {
			return fmt.Errorf("proxy has no channels after generation")
		}
		return nil
	})

	// Phase 6: M3U/XMLTV Output Validation
	var m3uContent, xmltvContent string

	r.runTest("Fetch and Validate M3U Output", func() error {
		if proxyID == "" {
			return fmt.Errorf("no proxy to fetch M3U from")
		}
		var err error
		m3uContent, err = r.client.GetProxyM3U(ctx, proxyID)
		if err != nil {
			return err
		}
		r.log("  M3U size: %d bytes", len(m3uContent))

		channelCount, err := ValidateM3U(m3uContent)
		if err != nil {
			return err
		}
		r.log("  M3U channels: %d", channelCount)

		// Validate expected channel count if specified
		if r.expectedChannels > 0 && channelCount != r.expectedChannels {
			return fmt.Errorf("channel count mismatch: got %d, expected %d", channelCount, r.expectedChannels)
		}

		// Write M3U to output dir if specified
		if r.outputDir != "" {
			if err := r.writeArtifact(proxyID+".m3u", m3uContent); err != nil {
				r.log("  Warning: failed to write M3U: %v", err)
			} else {
				r.log("  Wrote M3U artifact: %s/%s.m3u", r.outputDir, proxyID)
			}
		}

		// Display sample channels if requested
		if r.showSamples {
			r.printSampleChannels(m3uContent)
		}

		return nil
	})

	r.runTest("Fetch and Validate XMLTV Output", func() error {
		if proxyID == "" {
			return fmt.Errorf("no proxy to fetch XMLTV from")
		}
		var err error
		xmltvContent, err = r.client.GetProxyXMLTV(ctx, proxyID)
		if err != nil {
			return err
		}
		r.log("  XMLTV size: %d bytes", len(xmltvContent))

		channelCount, programCount, err := ValidateXMLTV(xmltvContent)
		if err != nil {
			return err
		}
		r.log("  XMLTV channels: %d, programs: %d", channelCount, programCount)

		// Validate expected counts if specified
		if r.expectedChannels > 0 && channelCount != r.expectedChannels {
			return fmt.Errorf("XMLTV channel count mismatch: got %d, expected %d", channelCount, r.expectedChannels)
		}
		if r.expectedPrograms > 0 && programCount != r.expectedPrograms {
			return fmt.Errorf("XMLTV program count mismatch: got %d, expected %d", programCount, r.expectedPrograms)
		}

		// Write XMLTV to output dir if specified
		if r.outputDir != "" {
			if err := r.writeArtifact(proxyID+".xmltv", xmltvContent); err != nil {
				r.log("  Warning: failed to write XMLTV: %v", err)
			} else {
				r.log("  Wrote XMLTV artifact: %s/%s.xmltv", r.outputDir, proxyID)
			}
		}

		// Display sample programs if requested
		if r.showSamples {
			r.printSamplePrograms(xmltvContent)
		}

		return nil
	})

	// Phase 7: Logo Helper Validation
	// Verify that the @logo:ULID helper was resolved correctly in the output
	r.runTest("Validate Channel Logo URLs in M3U", func() error {
		if r.channelLogoID == "" {
			r.log("  Skipping: no channel logo ID available")
			return nil
		}
		if m3uContent == "" {
			return fmt.Errorf("no M3U content available for validation")
		}

		// The M3U should contain the resolved logo URL with our uploaded logo's ULID
		// Expected pattern: /api/v1/logos/ULID or http://baseurl/api/v1/logos/ULID
		expectedLogoPath := fmt.Sprintf("/api/v1/logos/%s", r.channelLogoID)

		if !strings.Contains(m3uContent, expectedLogoPath) {
			// Check if there are any tvg-logo entries at all
			if strings.Contains(m3uContent, "tvg-logo=") {
				// There are logos but they don't match our expected pattern
				// Extract a sample logo URL for debugging
				sampleLogo := extractAttribute(m3uContent, "tvg-logo")
				return fmt.Errorf("M3U contains tvg-logo but not with expected logo ID.\n  Expected path containing: %s\n  Sample logo found: %s\n  This indicates the @logo: helper was not resolved correctly", expectedLogoPath, sampleLogo)
			}
			r.log("  Warning: No tvg-logo attributes found in M3U (may be expected if source had no logos)")
			return nil
		}

		// Count how many channels have the correct logo
		logoCount := strings.Count(m3uContent, expectedLogoPath)
		r.log("  Validated: %d channel logos use uploaded placeholder (ID: %s)", logoCount, r.channelLogoID)
		return nil
	})

	r.runTest("Validate Program Icon URLs in XMLTV", func() error {
		if r.programLogoID == "" {
			r.log("  Skipping: no program logo ID available")
			return nil
		}
		if xmltvContent == "" {
			return fmt.Errorf("no XMLTV content available for validation")
		}

		// The XMLTV should contain the resolved icon URL with our uploaded logo's ULID
		// Expected pattern: /api/v1/logos/ULID or http://baseurl/api/v1/logos/ULID
		expectedIconPath := fmt.Sprintf("/api/v1/logos/%s", r.programLogoID)

		if !strings.Contains(xmltvContent, expectedIconPath) {
			// Check if there are any icon entries at all
			if strings.Contains(xmltvContent, "<icon src=") {
				// There are icons but they don't match our expected pattern
				// Extract a sample icon URL for debugging
				sampleIcon := extractXMLElement(xmltvContent, "icon")
				return fmt.Errorf("XMLTV contains icons but not with expected logo ID.\n  Expected path containing: %s\n  Sample icon found: %s\n  This indicates the @logo: helper was not resolved correctly", expectedIconPath, sampleIcon)
			}
			r.log("  Warning: No <icon> elements found in XMLTV (may be expected if source had no icons)")
			return nil
		}

		// Count how many programs have the correct icon
		iconCount := strings.Count(xmltvContent, expectedIconPath)
		r.log("  Validated: %d program icons use uploaded placeholder (ID: %s)", iconCount, r.programLogoID)
		return nil
	})

	// Phase 8: Proxy Mode Tests (optional, enabled via -test-proxy-modes)
	// Note: Channels already have correct testdata server URLs from test data generation,
	// no data mapping rule or re-ingestion needed.
	// The direct mode proxy was already created in Phase 4 (proxyID), so we only create
	// smart mode and smart+relay proxies here.
	if r.testProxyModes && streamSourceID != "" && r.testdataServer != nil {
		// Proxy IDs stored at outer scope for stream testing
		var smartModeProxyID string
		var relayProfileProxyID string

		// Create proxy with smart mode (passthrough/repackage)
		r.runTest("Create Proxy (Smart Mode)", func() error {
			var err error
			smartModeProxyID, err = r.client.CreateStreamProxy(ctx, CreateStreamProxyOptions{
				Name:            fmt.Sprintf("E2E Smart Mode Proxy %s", r.runID),
				StreamSourceIDs: []string{streamSourceID},
				ProxyMode:       "smart",
			})
			if err != nil {
				return err
			}
			r.log("  Created smart mode proxy: %s", smartModeProxyID)
			return nil
		})

		// Create proxy with smart mode + relay profile for transcoding (only if ffmpeg is available)
		if r.ffmpegAvailable {
			r.runTest("Create Proxy (Smart Mode with Relay Profile)", func() error {
				// Get the first relay profile ID (should exist from seeded profiles)
				relayProfileID, err := r.client.GetFirstRelayProfileID(ctx)
				if err != nil {
					return fmt.Errorf("failed to get relay profile: %w", err)
				}
				if relayProfileID == "" {
					return fmt.Errorf("no relay profiles available")
				}
				r.log("  Using relay profile: %s", relayProfileID)

				relayProfileProxyID, err = r.client.CreateStreamProxy(ctx, CreateStreamProxyOptions{
					Name:            fmt.Sprintf("E2E Smart+Relay Proxy %s", r.runID),
					StreamSourceIDs: []string{streamSourceID},
					ProxyMode:       "smart",
					RelayProfileID:  relayProfileID,
				})
				if err != nil {
					return err
				}
				r.log("  Created smart mode proxy with relay profile: %s", relayProfileProxyID)
				return nil
			})
		} else {
			// Log that we're skipping relay profile tests
			r.runTest("Skip Relay Profile Tests (No FFmpeg)", func() error {
				r.log("  SKIPPED: Relay profile tests require ffmpeg")
				fmt.Println("WARNING: Skipping relay profile proxy test - ffmpeg not available")
				fmt.Fprintln(os.Stderr, "WARNING: Skipping relay profile proxy test - ffmpeg not available")
				return nil
			})
		}

		// Test Direct Mode Stream Fetching
		// Uses proxyID which was created with direct mode in Phase 4
		r.runTest("Test Stream (Direct Mode)", func() error {
			if proxyID == "" {
				return fmt.Errorf("direct mode proxy was not created")
			}

			// Proxy was already generated in Phase 4, no need to regenerate

			// Get a channel ID to test streaming
			// The proxy stream endpoint expects /proxy/{proxyId}/{channelId}
			channelID, err := r.client.GetFirstChannelID(ctx, streamSourceID)
			if err != nil {
				return fmt.Errorf("get channel ID: %w", err)
			}

			// Construct the proxy stream URL
			streamURL := r.client.GetProxyStreamURL(proxyID, channelID)
			r.log("  Testing stream URL: %s", streamURL)

			// Test the stream request
			result, err := r.client.TestStreamRequest(ctx, streamURL)
			if err != nil {
				return fmt.Errorf("stream request: %w", err)
			}

			// Direct mode should return HTTP 302 redirect to source URL
			if result.StatusCode != http.StatusFound {
				return fmt.Errorf("expected HTTP 302, got %d", result.StatusCode)
			}
			if result.Location == "" {
				return fmt.Errorf("expected Location header in redirect response")
			}
			r.log("  Direct mode: HTTP %d -> %s", result.StatusCode, result.Location)
			return nil
		})

		// Test Smart Mode Stream Fetching (passthrough/repackage)
		r.runTest("Test Stream (Smart Mode)", func() error {
			if smartModeProxyID == "" {
				return fmt.Errorf("smart mode proxy was not created")
			}

			// Trigger proxy generation
			if err := r.client.TriggerProxyGeneration(ctx, smartModeProxyID, time.Minute); err != nil {
				return fmt.Errorf("trigger proxy generation: %w", err)
			}

			// Get a channel ID to test streaming
			// The proxy stream endpoint expects /proxy/{proxyId}/{channelId}
			channelID, err := r.client.GetFirstChannelID(ctx, streamSourceID)
			if err != nil {
				return fmt.Errorf("get channel ID: %w", err)
			}

			// Construct the proxy stream URL
			streamURL := r.client.GetProxyStreamURL(smartModeProxyID, channelID)
			r.log("  Testing stream URL: %s", streamURL)

			// Test the stream request
			result, err := r.client.TestStreamRequest(ctx, streamURL)
			if err != nil {
				return fmt.Errorf("stream request: %w", err)
			}

			// Smart mode should return HTTP 200 with CORS headers and content
			if result.StatusCode != http.StatusOK {
				return fmt.Errorf("expected HTTP 200, got %d", result.StatusCode)
			}
			if !result.HasCORSHeaders {
				return fmt.Errorf("expected CORS headers (Access-Control-Allow-Origin)")
			}
			if result.BytesReceived == 0 {
				return fmt.Errorf("expected to receive stream bytes")
			}
			if !result.TSSyncByteValid {
				r.log("  WARNING: First byte is not TS sync byte (0x47), may not be TS format")
			}
			r.log("  Smart mode: HTTP %d, CORS: %v, Bytes: %d, TS: %v",
				result.StatusCode, result.HasCORSHeaders, result.BytesReceived, result.TSSyncByteValid)
			return nil
		})

		// Test Smart Mode with Relay Profile Stream Fetching (only if ffmpeg is available and proxy was created)
		if r.ffmpegAvailable && relayProfileProxyID != "" {
			r.runTest("Test Stream (Smart Mode with Relay Profile)", func() error {
				// Trigger proxy generation
				if err := r.client.TriggerProxyGeneration(ctx, relayProfileProxyID, time.Minute); err != nil {
					return fmt.Errorf("trigger proxy generation: %w", err)
				}

				// Get a channel ID to test streaming
				// The proxy stream endpoint expects /proxy/{proxyId}/{channelId}
				channelID, err := r.client.GetFirstChannelID(ctx, streamSourceID)
				if err != nil {
					return fmt.Errorf("get channel ID: %w", err)
				}

				// Construct the proxy stream URL
				streamURL := r.client.GetProxyStreamURL(relayProfileProxyID, channelID)
				r.log("  Testing stream URL: %s", streamURL)

				// Test the stream request - smart mode with relay profile should return HTTP 200
				result, err := r.client.TestStreamRequest(ctx, streamURL)
				if err != nil {
					return fmt.Errorf("stream request: %w", err)
				}

				// Smart mode with relay profile should return HTTP 200 (transcoded content)
				if result.StatusCode != http.StatusOK {
					return fmt.Errorf("expected HTTP 200, got %d", result.StatusCode)
				}
				r.log("  Smart mode with relay profile: HTTP %d, Bytes: %d, TS: %v",
					result.StatusCode, result.BytesReceived, result.TSSyncByteValid)
				return nil
			})
		}
	}

	return nil
}

// writeArtifact writes content to a file in the output directory
func (r *E2ERunner) writeArtifact(filename, content string) error {
	if r.outputDir == "" {
		return nil
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(r.outputDir, 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	filePath := filepath.Join(r.outputDir, filename)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

// printSampleChannels prints the first 5 channels from M3U content
func (r *E2ERunner) printSampleChannels(m3uContent string) {
	fmt.Println("\n" + strings.Repeat("-", 60))
	fmt.Println("Sample Channels (first 5):")
	fmt.Println(strings.Repeat("-", 60))

	lines := strings.Split(m3uContent, "\n")
	channelCount := 0
	maxChannels := 5

	for i := 0; i < len(lines) && channelCount < maxChannels; i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "#EXTINF:") {
			channelCount++
			// Parse channel info
			name := extractChannelName(line)
			tvgID := extractAttribute(line, "tvg-id")
			group := extractAttribute(line, "group-title")
			logo := extractAttribute(line, "tvg-logo")

			// Get the URL (next non-empty, non-comment line)
			url := ""
			for j := i + 1; j < len(lines); j++ {
				nextLine := strings.TrimSpace(lines[j])
				if nextLine != "" && !strings.HasPrefix(nextLine, "#") {
					url = nextLine
					break
				}
			}

			fmt.Printf("\n  [%d] %s\n", channelCount, name)
			if tvgID != "" {
				fmt.Printf("      tvg-id:      %s\n", tvgID)
			}
			if group != "" {
				fmt.Printf("      group:       %s\n", group)
			}
			if logo != "" {
				fmt.Printf("      logo:        %s\n", truncateString(logo, 50))
			}
			if url != "" {
				fmt.Printf("      url:         %s\n", truncateString(url, 60))
			}
		}
	}
	fmt.Println()
}

// printSamplePrograms prints the first 5 programs from XMLTV content
func (r *E2ERunner) printSamplePrograms(xmltvContent string) {
	fmt.Println("\n" + strings.Repeat("-", 60))
	fmt.Println("Sample Programs (first 5):")
	fmt.Println(strings.Repeat("-", 60))

	// Simple XML parsing for sample display
	programCount := 0
	maxPrograms := 5

	// Find programme elements
	for programCount < maxPrograms {
		startTag := "<programme "
		startIdx := strings.Index(xmltvContent, startTag)
		if startIdx == -1 {
			break
		}

		endTag := "</programme>"
		endIdx := strings.Index(xmltvContent[startIdx:], endTag)
		if endIdx == -1 {
			break
		}

		programXML := xmltvContent[startIdx : startIdx+endIdx+len(endTag)]
		xmltvContent = xmltvContent[startIdx+endIdx+len(endTag):]

		programCount++

		// Extract key attributes
		start := extractXMLAttribute(programXML, "start")
		stop := extractXMLAttribute(programXML, "stop")
		channel := extractXMLAttribute(programXML, "channel")
		title := extractXMLElement(programXML, "title")
		desc := extractXMLElement(programXML, "desc")
		category := extractXMLElement(programXML, "category")
		icon := extractXMLAttribute(strings.Split(programXML, "<icon")[0]+extractIfContains(programXML, "<icon"), "src")

		fmt.Printf("\n  [%d] %s\n", programCount, title)
		if channel != "" {
			fmt.Printf("      channel:     %s\n", channel)
		}
		if start != "" {
			fmt.Printf("      start:       %s\n", start)
		}
		if stop != "" {
			fmt.Printf("      stop:        %s\n", stop)
		}
		if category != "" {
			fmt.Printf("      category:    %s\n", category)
		}
		if desc != "" {
			fmt.Printf("      description: %s\n", truncateString(desc, 60))
		}
		if icon != "" {
			fmt.Printf("      icon:        %s\n", truncateString(icon, 50))
		}
	}
	fmt.Println()
}

// extractChannelName extracts the channel name from an EXTINF line
func extractChannelName(line string) string {
	// Name is after the last comma in EXTINF line
	commaIdx := strings.LastIndex(line, ",")
	if commaIdx == -1 {
		return "Unknown"
	}
	return strings.TrimSpace(line[commaIdx+1:])
}

// extractAttribute extracts an attribute value from an EXTINF line
func extractAttribute(line, attr string) string {
	// Look for attr="value" or attr='value'
	patterns := []string{attr + `="`, attr + `='`}
	for _, pattern := range patterns {
		idx := strings.Index(strings.ToLower(line), strings.ToLower(pattern))
		if idx == -1 {
			continue
		}
		start := idx + len(pattern)
		quote := line[idx+len(pattern)-1]
		end := strings.IndexByte(line[start:], quote)
		if end == -1 {
			continue
		}
		return line[start : start+end]
	}
	return ""
}

// extractXMLAttribute extracts an attribute value from an XML element
func extractXMLAttribute(xml, attr string) string {
	pattern := attr + `="`
	idx := strings.Index(xml, pattern)
	if idx == -1 {
		return ""
	}
	start := idx + len(pattern)
	end := strings.IndexByte(xml[start:], '"')
	if end == -1 {
		return ""
	}
	return xml[start : start+end]
}

// extractXMLElement extracts the text content of an XML element
func extractXMLElement(xml, element string) string {
	startTag := "<" + element
	startIdx := strings.Index(xml, startTag)
	if startIdx == -1 {
		return ""
	}
	// Find the closing > of the start tag
	closeStart := strings.Index(xml[startIdx:], ">")
	if closeStart == -1 {
		return ""
	}
	contentStart := startIdx + closeStart + 1
	endTag := "</" + element + ">"
	endIdx := strings.Index(xml[contentStart:], endTag)
	if endIdx == -1 {
		return ""
	}
	return strings.TrimSpace(xml[contentStart : contentStart+endIdx])
}

// extractIfContains returns the substring if it contains the pattern, empty otherwise
func extractIfContains(s, pattern string) string {
	if strings.Contains(s, pattern) {
		idx := strings.Index(s, pattern)
		// Get until the closing >
		end := strings.Index(s[idx:], ">")
		if end == -1 {
			end = strings.Index(s[idx:], "/>")
			if end == -1 {
				return ""
			}
		}
		return s[idx : idx+end+1]
	}
	return ""
}

// PrintSummary prints the test results summary
func (r *E2ERunner) PrintSummary() int {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("E2E Test Results")
	fmt.Println(strings.Repeat("=", 60))

	passed := 0
	failed := 0
	var totalTime time.Duration

	for _, result := range r.results {
		status := "PASS"
		if !result.Passed {
			status = "FAIL"
			failed++
		} else {
			passed++
		}
		totalTime += result.Elapsed
		fmt.Printf("[%s] %s (%.2fs)\n", status, result.Name, result.Elapsed.Seconds())
		if !result.Passed {
			fmt.Printf("       Error: %s\n", result.Message)
		}
	}

	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("Total: %d tests, %d passed, %d failed (%.2fs)\n",
		len(r.results), passed, failed, totalTime.Seconds())

	// Print SSE timeline
	r.sseCollector.PrintTimeline()

	if failed > 0 {
		return 1
	}
	return 0
}

func main() {
	var (
		binaryPath        = flag.String("binary", "", "Path to tvarr binary (if set, starts managed server on random port)")
		baseURL           = flag.String("base-url", "", "Tvarr API base URL (ignored if -binary is set)")
		m3uURL            = flag.String("m3u-url", "", "M3U stream source URL for testing (ignored if -random-testdata is set)")
		epgURL            = flag.String("epg-url", "", "EPG source URL for testing (ignored if -random-testdata is set)")
		verbose           = flag.Bool("verbose", true, "Enable verbose output")
		timeout           = flag.Duration("timeout", DefaultTimeout, "Overall test timeout")
		cacheChannelLogos = flag.Bool("cache-channel-logos", true, "Enable channel logo caching during proxy generation")
		cacheProgramLogos = flag.Bool("cache-program-logos", true, "Enable program logo caching during proxy generation")
		outputDir         = flag.String("output-dir", "", "Directory to write artifact files (M3U/XMLTV)")
		showSamples       = flag.Bool("show-samples", true, "Display sample channels and programs to stdout")

		// Test data generation flags
		randomTestdata   = flag.Bool("random-testdata", true, "Generate random test data (default: true)")
		channelCount     = flag.Int("channel-count", 50, "Number of channels to generate")
		programCount     = flag.Int("program-count", 5000, "Total number of programs to generate")
		requiredChannels = flag.Int("required-channels", 0, "Required channel count (fails if mismatch, 0 to skip)")
		requiredPrograms = flag.Int("required-programs", 0, "Required program count (fails if mismatch, 0 to skip)")
		randomSeed       = flag.Int64("random-seed", 0, "Random seed for test data generation (0 for time-based)")

		// Proxy mode testing flags
		testProxyModes = flag.Bool("test-proxy-modes", false, "Test different proxy modes (redirect, proxy, relay)")
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	var server *ManagedServer
	var effectiveBaseURL string
	var effectiveM3UURL, effectiveEPGURL string
	var testDataDir string
	var testdataServer *TestdataServer

	// Start testdata server early if testing proxy modes
	// This must happen BEFORE test data generation so we can use its URL
	if *testProxyModes {
		var err error
		testdataServer, err = NewTestdataServer()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create testdata server: %v\n", err)
			os.Exit(1)
		}
		testdataServer.Start()
		defer testdataServer.Stop()
		fmt.Printf("Testdata server started at: %s\n", testdataServer.BaseURL())
		fmt.Printf("Stream URL: %s\n", testdataServer.StreamURL())
		fmt.Println()
	}

	// Handle test data generation
	if *randomTestdata {
		// Create temp directory for test data files
		var err error
		testDataDir, err = os.MkdirTemp("", "tvarr-e2e-testdata-*")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create temp directory: %v\n", err)
			os.Exit(1)
		}

		// Generate test data
		config := DefaultTestDataConfig()
		config.ChannelCount = *channelCount
		config.ProgramCount = *programCount
		if *randomSeed != 0 {
			config.RandomSeed = *randomSeed
		}

		// If testing proxy modes, use the testdata server's stream URL
		// This ensures all channels point to a real, resolvable stream
		if testdataServer != nil {
			config.BaseURL = testdataServer.BaseURL()
			config.LogoBaseURL = testdataServer.BaseURL()
		}

		generator := NewTestDataGenerator(config)
		testData, err := generator.Generate()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to generate test data: %v\n", err)
			os.Exit(1)
		}

		// Validate if required counts are specified
		if err := testData.Validate(*requiredChannels, *requiredPrograms); err != nil {
			fmt.Fprintf(os.Stderr, "Test data validation failed: %v\n", err)
			os.Exit(1)
		}

		// Write test data to files
		if err := testData.WriteToFiles(testDataDir); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write test data files: %v\n", err)
			os.Exit(1)
		}

		// Use file:// URLs
		effectiveM3UURL = testData.M3UURL()
		effectiveEPGURL = testData.XMLTVURL()

		fmt.Println("Test Data Generation")
		fmt.Println(strings.Repeat("-", 40))
		fmt.Printf("Channels Generated:  %d\n", testData.ChannelCount)
		fmt.Printf("Programs Generated:  %d\n", testData.ProgramCount)
		fmt.Printf("Test Data Dir:       %s\n", testDataDir)
		fmt.Println()

		// Set expected counts from generated data for output validation
		*requiredChannels = testData.ChannelCount
		*requiredPrograms = testData.ProgramCount

		defer func() {
			if testDataDir != "" {
				os.RemoveAll(testDataDir)
			}
		}()
	} else {
		// Use provided URLs or defaults
		if *m3uURL != "" {
			effectiveM3UURL = *m3uURL
		} else {
			effectiveM3UURL = DefaultM3UURL
		}
		if *epgURL != "" {
			effectiveEPGURL = *epgURL
		} else {
			effectiveEPGURL = DefaultEPGURL
		}
	}

	// If binary path is provided, start a managed server
	if *binaryPath != "" {
		var err error
		server, err = NewManagedServer(*binaryPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create managed server: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Tvarr E2E Test Runner (Managed Mode)")
		fmt.Println(strings.Repeat("=", 60))
		fmt.Printf("Binary:              %s\n", *binaryPath)
		fmt.Printf("Port:                %d (random, never 8080)\n", server.Port())
		fmt.Printf("Data Directory:      %s\n", server.DataDir())
		fmt.Printf("M3U URL:             %s\n", effectiveM3UURL)
		fmt.Printf("EPG URL:             %s\n", effectiveEPGURL)
		fmt.Printf("Timeout:             %v\n", *timeout)
		fmt.Printf("Cache Channel Logos: %v\n", *cacheChannelLogos)
		fmt.Printf("Cache Program Logos: %v\n", *cacheProgramLogos)
		fmt.Printf("Random Test Data:    %v\n", *randomTestdata)
		if *randomTestdata {
			fmt.Printf("Channel Count:       %d\n", *channelCount)
			fmt.Printf("Program Count:       %d\n", *programCount)
		}
		if *outputDir != "" {
			fmt.Printf("Output Directory:    %s\n", *outputDir)
		}
		fmt.Printf("Show Samples:        %v\n", *showSamples)
		fmt.Println()

		fmt.Printf("Starting server on port %d...\n", server.Port())
		if err := server.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to start server: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Server ready")
		fmt.Println()

		defer func() {
			fmt.Println("\nCleaning up...")
			server.Stop()
			fmt.Printf("Stopped server (port %d)\n", server.Port())
			fmt.Printf("Removed %s\n", server.DataDir())
		}()

		effectiveBaseURL = server.BaseURL()
	} else {
		// Legacy mode: connect to existing server
		if *baseURL == "" {
			*baseURL = "http://localhost:8080"
		}
		effectiveBaseURL = *baseURL

		fmt.Println("Tvarr E2E Test Runner")
		fmt.Println(strings.Repeat("=", 60))
		fmt.Printf("Base URL:            %s\n", effectiveBaseURL)
		fmt.Printf("M3U URL:             %s\n", effectiveM3UURL)
		fmt.Printf("EPG URL:             %s\n", effectiveEPGURL)
		fmt.Printf("Timeout:             %v\n", *timeout)
		fmt.Printf("Cache Channel Logos: %v\n", *cacheChannelLogos)
		fmt.Printf("Cache Program Logos: %v\n", *cacheProgramLogos)
		fmt.Printf("Random Test Data:    %v\n", *randomTestdata)
		if *randomTestdata {
			fmt.Printf("Channel Count:       %d\n", *channelCount)
			fmt.Printf("Program Count:       %d\n", *programCount)
		}
		if *outputDir != "" {
			fmt.Printf("Output Directory:    %s\n", *outputDir)
		}
		fmt.Printf("Show Samples:        %v\n", *showSamples)
	}

	runner := NewE2ERunner(E2ERunnerOptions{
		BaseURL:           effectiveBaseURL,
		M3UURL:            effectiveM3UURL,
		EPGURL:            effectiveEPGURL,
		Verbose:           *verbose,
		CacheChannelLogos: *cacheChannelLogos,
		CacheProgramLogos: *cacheProgramLogos,
		OutputDir:         *outputDir,
		ShowSamples:       *showSamples,
		ExpectedChannels:  *requiredChannels,
		ExpectedPrograms:  *requiredPrograms,
		Server:            server,
		TestProxyModes:    *testProxyModes,
		TestdataServer:    testdataServer, // Pass pre-created testdata server (created before test data generation)
	})
	fmt.Printf("Run ID:              %s\n", runner.runID)
	fmt.Println()
	_ = runner.Run(ctx)

	exitCode := runner.PrintSummary()

	// Ensure stdout is flushed before exit (helps when piped)
	os.Stdout.Sync()
	os.Stderr.Sync()

	os.Exit(exitCode)
}
