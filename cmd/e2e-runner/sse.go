//nolint:errcheck,gocognit,gocyclo,nestif,gocritic,godot,wrapcheck,gosec,revive,goprintffuncname,modernize // E2E test runner uses relaxed linting
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// SSECollector collects SSE events in the background.
type SSECollector struct {
	baseURL    string
	events     []ProgressEvent
	mu         sync.Mutex
	cancel     context.CancelFunc
	done       chan struct{}
	httpClient *http.Client
	startTime  time.Time
}

// NewSSECollector creates a new SSE collector.
func NewSSECollector(baseURL string) *SSECollector {
	return &SSECollector{
		baseURL:    baseURL,
		events:     make([]ProgressEvent, 0),
		done:       make(chan struct{}),
		httpClient: &http.Client{Timeout: 0}, // No timeout for SSE
		startTime:  time.Now(),
	}
}

// Start begins collecting SSE events in the background.
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

// Stop stops collecting SSE events.
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

// GetEvents returns a copy of all collected events.
func (c *SSECollector) GetEvents() []ProgressEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]ProgressEvent, len(c.events))
	copy(result, c.events)
	return result
}

// PrintTimeline prints a waterfall-style timeline of all events grouped by operation.
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
