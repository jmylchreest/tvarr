// Package main provides an E2E test runner for validating the tvarr pipeline.
// This binary tests the complete flow from source ingestion through M3U/XMLTV output.
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

//go:embed testdata/channel.webp testdata/program.webp
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
	cmd      *exec.Cmd
	port     int
	dataDir  string
	baseURL  string
	startErr error
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

	// Redirect output for debugging (optional)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	ms := &ManagedServer{
		cmd:     cmd,
		port:    port,
		dataDir: dataDir,
		baseURL: fmt.Sprintf("http://localhost:%d", port),
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
	OperationName     string
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
				if v, ok := event["operation_name"].(string); ok {
					pe.OperationName = v
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
	// Give it a moment to clean up
	select {
	case <-c.done:
	case <-time.After(time.Second):
	}
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
	name       string
	ownerID    string
	firstTime  time.Time
	lastTime   time.Time
	finalState string
	error      string
	stages     []stageInfo
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

		// Get operation name (use the most descriptive one)
		for _, e := range opEvents {
			if e.OperationName != "" {
				op.name = e.OperationName
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

		// Truncate long operation names
		opNameDisplay := op.name
		if len(opNameDisplay) > 46 {
			opNameDisplay = opNameDisplay[:43] + "..."
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

// TriggerProxyGeneration triggers proxy generation and waits for completion
// Note: The generate endpoint is synchronous - it returns when generation completes.
// We don't need to wait for SSE since a 200 response means it's done.
func (c *APIClient) TriggerProxyGeneration(ctx context.Context, proxyID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Trigger generation - this is synchronous and returns when complete
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/proxies/"+proxyID+"/generate", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("trigger generation failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("trigger generation failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// Generation is complete (endpoint is synchronous)
	return nil
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

// UploadLogo uploads a logo file and returns the URL to access it
func (c *APIClient) UploadLogo(ctx context.Context, name string, fileData []byte) (string, error) {
	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add the file field
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		return "", fmt.Errorf("create form file failed: %w", err)
	}
	if _, err := part.Write(fileData); err != nil {
		return "", fmt.Errorf("write file data failed: %w", err)
	}

	// Add the name field
	if err := writer.WriteField("name", name); err != nil {
		return "", fmt.Errorf("write name field failed: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close writer failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/logos/upload", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload logo failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload logo failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response failed: %w", err)
	}

	url, ok := result["url"].(string)
	if !ok {
		return "", fmt.Errorf("response missing url field")
	}
	return url, nil
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
	runID             string        // Unique ID for this test run to avoid name collisions
	cacheChannelLogos bool          // Enable channel logo caching
	cacheProgramLogos bool          // Enable program logo caching
	sseCollector      *SSECollector // Collects SSE events for timeline
	channelLogoURL    string        // URL of uploaded channel logo placeholder
	programLogoURL    string        // URL of uploaded program logo placeholder
	outputDir         string        // Directory to write artifact files (m3u/xmltv)
	showSamples       bool          // Display sample channels and programs to stdout
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

	// Phase 1: Setup - Health Check
	r.runTest("Health Check", func() error {
		return r.client.HealthCheck(ctx)
	})

	// Phase 1.5: Upload placeholder logos and create data mapping rules
	// This optimizes logo caching - all logos will point to the same local placeholder
	r.runTest("Upload Channel Logo Placeholder", func() error {
		channelLogoData, err := testdataFS.ReadFile("testdata/channel.webp")
		if err != nil {
			return fmt.Errorf("read channel.webp from embedded testdata: %w", err)
		}
		r.channelLogoURL, err = r.client.UploadLogo(ctx, "channel-placeholder.webp", channelLogoData)
		if err != nil {
			return err
		}
		r.log("  Uploaded channel logo: %s", r.channelLogoURL)
		return nil
	})

	r.runTest("Upload Program Logo Placeholder", func() error {
		programLogoData, err := testdataFS.ReadFile("testdata/program.webp")
		if err != nil {
			return fmt.Errorf("read program.webp from embedded testdata: %w", err)
		}
		r.programLogoURL, err = r.client.UploadLogo(ctx, "program-placeholder.webp", programLogoData)
		if err != nil {
			return err
		}
		r.log("  Uploaded program logo: %s", r.programLogoURL)
		return nil
	})

	r.runTest("Create Channel Logo Mapping Rule", func() error {
		if r.channelLogoURL == "" {
			return fmt.Errorf("no channel logo URL available")
		}
		// Rule: Replace all channel logos with our placeholder
		// Expression syntax: <field> <operator> "<value>" SET <target_field> = "<new_value>"
		expression := fmt.Sprintf(`tvg_logo starts_with "http" SET tvg_logo = "%s"`, r.channelLogoURL)
		ruleID, err := r.client.CreateDataMappingRule(ctx,
			fmt.Sprintf("E2E Channel Logo Placeholder %s", r.runID),
			"stream",
			expression,
			100, // High priority
		)
		if err != nil {
			return err
		}
		r.log("  Created channel logo mapping rule: %s", ruleID)
		return nil
	})

	r.runTest("Create Program Icon Mapping Rule", func() error {
		if r.programLogoURL == "" {
			return fmt.Errorf("no program logo URL available")
		}
		// Rule: Replace all program icons with our placeholder
		expression := fmt.Sprintf(`programme_icon starts_with "http" SET programme_icon = "%s"`, r.programLogoURL)
		ruleID, err := r.client.CreateDataMappingRule(ctx,
			fmt.Sprintf("E2E Program Icon Placeholder %s", r.runID),
			"epg",
			expression,
			100, // High priority
		)
		if err != nil {
			return err
		}
		r.log("  Created program icon mapping rule: %s", ruleID)
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
		// Use unique name with runID to avoid conflicts
		proxyID, err = r.client.CreateStreamProxy(ctx, CreateStreamProxyOptions{
			Name:              fmt.Sprintf("E2E Test Proxy %s", r.runID),
			StreamSourceIDs:   sourceIDs,
			EpgSourceIDs:      epgIDs,
			CacheChannelLogos: r.cacheChannelLogos,
			CacheProgramLogos: r.cacheProgramLogos,
		})
		if err != nil {
			return err
		}
		r.log("  Created proxy: %s (logo caching: channel=%v, program=%v)", proxyID, r.cacheChannelLogos, r.cacheProgramLogos)
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
	r.runTest("Fetch and Validate M3U Output", func() error {
		if proxyID == "" {
			return fmt.Errorf("no proxy to fetch M3U from")
		}
		m3uContent, err := r.client.GetProxyM3U(ctx, proxyID)
		if err != nil {
			return err
		}
		r.log("  M3U size: %d bytes", len(m3uContent))

		channelCount, err := ValidateM3U(m3uContent)
		if err != nil {
			return err
		}
		r.log("  M3U channels: %d", channelCount)

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
		xmltvContent, err := r.client.GetProxyXMLTV(ctx, proxyID)
		if err != nil {
			return err
		}
		r.log("  XMLTV size: %d bytes", len(xmltvContent))

		channelCount, programCount, err := ValidateXMLTV(xmltvContent)
		if err != nil {
			return err
		}
		r.log("  XMLTV channels: %d, programs: %d", channelCount, programCount)

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
		m3uURL            = flag.String("m3u-url", DefaultM3UURL, "M3U stream source URL for testing")
		epgURL            = flag.String("epg-url", DefaultEPGURL, "EPG source URL for testing")
		verbose           = flag.Bool("verbose", true, "Enable verbose output")
		timeout           = flag.Duration("timeout", DefaultTimeout, "Overall test timeout")
		cacheChannelLogos = flag.Bool("cache-channel-logos", true, "Enable channel logo caching during proxy generation")
		cacheProgramLogos = flag.Bool("cache-program-logos", true, "Enable program logo caching during proxy generation")
		outputDir         = flag.String("output-dir", "", "Directory to write artifact files (M3U/XMLTV)")
		showSamples       = flag.Bool("show-samples", true, "Display sample channels and programs to stdout")
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	var server *ManagedServer
	var effectiveBaseURL string

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
		fmt.Printf("M3U URL:             %s\n", *m3uURL)
		fmt.Printf("EPG URL:             %s\n", *epgURL)
		fmt.Printf("Timeout:             %v\n", *timeout)
		fmt.Printf("Cache Channel Logos: %v\n", *cacheChannelLogos)
		fmt.Printf("Cache Program Logos: %v\n", *cacheProgramLogos)
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
		fmt.Printf("M3U URL:             %s\n", *m3uURL)
		fmt.Printf("EPG URL:             %s\n", *epgURL)
		fmt.Printf("Timeout:             %v\n", *timeout)
		fmt.Printf("Cache Channel Logos: %v\n", *cacheChannelLogos)
		fmt.Printf("Cache Program Logos: %v\n", *cacheProgramLogos)
		if *outputDir != "" {
			fmt.Printf("Output Directory:    %s\n", *outputDir)
		}
		fmt.Printf("Show Samples:        %v\n", *showSamples)
	}

	runner := NewE2ERunner(E2ERunnerOptions{
		BaseURL:           effectiveBaseURL,
		M3UURL:            *m3uURL,
		EPGURL:            *epgURL,
		Verbose:           *verbose,
		CacheChannelLogos: *cacheChannelLogos,
		CacheProgramLogos: *cacheProgramLogos,
		OutputDir:         *outputDir,
		ShowSamples:       *showSamples,
	})
	fmt.Printf("Run ID:              %s\n", runner.runID)
	fmt.Println()
	_ = runner.Run(ctx)

	exitCode := runner.PrintSummary()
	os.Exit(exitCode)
}
