package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// E2ERunner runs the E2E test suite.
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

	// Test state - shared between test files
	StreamSourceID      string
	EpgSourceID         string
	ProxyID             string
	SmartModeProxyID    string
	RelayProfileProxyID string
	M3UContent          string
	XMLTVContent        string
}

// NewE2ERunner creates a new E2E runner.
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

// RunID returns the unique run ID for this test run.
func (r *E2ERunner) RunID() string {
	return r.runID
}

// log prints a message if verbose mode is enabled.
func (r *E2ERunner) log(format string, args ...interface{}) {
	if r.verbose {
		fmt.Printf(format+"\n", args...)
	}
}

// runTestWithInfo executes a test with an info description and records the result.
func (r *E2ERunner) runTestWithInfo(name, info string, fn func() error) {
	start := time.Now()
	r.log("Running: %s", name)
	if info != "" {
		r.log("  [INFO] %s", info)
	}

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

// Run executes the full E2E test suite.
func (r *E2ERunner) Run(ctx context.Context) error {
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

		// Check for FFmpeg availability - warn if not present
		if !r.ffmpegAvailable {
			fmt.Println("WARNING: ffmpeg not found in PATH, relay profile tests will be skipped")
			fmt.Fprintln(os.Stderr, "WARNING: ffmpeg not found in PATH, relay profile tests will be skipped")
		}
	}

	// Run all test categories
	r.runHealthTests(ctx)
	r.runClientDetectionTests(ctx)
	r.runThemeTests(ctx)
	r.runLogoUploadTests(ctx)
	r.runSourceTests(ctx)
	r.runProxyTests(ctx)
	r.runOutputValidationTests(ctx)
	r.runLogoValidationTests(ctx)

	// Run stream tests if proxy modes are enabled
	if r.testProxyModes && r.StreamSourceID != "" && r.testdataServer != nil {
		r.runStreamTests(ctx)
	}

	return nil
}

// PrintSummary prints the test results summary.
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

// writeArtifact writes content to a file in the output directory.
func (r *E2ERunner) writeArtifact(filename, content string) error {
	if r.outputDir == "" {
		return nil
	}
	if err := os.MkdirAll(r.outputDir, 0755); err != nil {
		return err
	}
	path := filepath.Join(r.outputDir, filename)
	return os.WriteFile(path, []byte(content), 0644)
}

// printSampleChannels displays sample channels from M3U content.
func (r *E2ERunner) printSampleChannels(m3uContent string) {
	fmt.Println("\n--- Sample Channels (first 5) ---")
	lines := strings.Split(m3uContent, "\n")
	count := 0
	for i, line := range lines {
		if strings.HasPrefix(line, "#EXTINF:") && count < 5 {
			name := extractChannelName(line)
			tvgID := extractAttribute(line, "tvg-id")
			tvgLogo := extractAttribute(line, "tvg-logo")
			groupTitle := extractAttribute(line, "group-title")

			fmt.Printf("  %d. %s\n", count+1, name)
			if tvgID != "" {
				fmt.Printf("     tvg-id: %s\n", tvgID)
			}
			if groupTitle != "" {
				fmt.Printf("     group: %s\n", groupTitle)
			}
			if tvgLogo != "" {
				fmt.Printf("     logo: %s\n", truncateString(tvgLogo, 60))
			}
			// Print the URL (next non-empty, non-comment line)
			for j := i + 1; j < len(lines); j++ {
				urlLine := strings.TrimSpace(lines[j])
				if urlLine != "" && !strings.HasPrefix(urlLine, "#") {
					fmt.Printf("     url: %s\n", truncateString(urlLine, 60))
					break
				}
			}
			count++
		}
	}
	fmt.Println("---")
}

// printSamplePrograms displays sample programs from XMLTV content.
func (r *E2ERunner) printSamplePrograms(xmltvContent string) {
	fmt.Println("\n--- Sample Programs (first 5) ---")
	// Find programme elements
	programmes := strings.Split(xmltvContent, "<programme ")
	count := 0
	for i := 1; i < len(programmes) && count < 5; i++ {
		prog := programmes[i]
		// Extract end of the programme element
		endIdx := strings.Index(prog, "</programme>")
		if endIdx != -1 {
			prog = prog[:endIdx]
		}

		channel := extractXMLAttribute("<programme "+prog, "channel")
		start := extractXMLAttribute("<programme "+prog, "start")
		title := extractXMLElement(prog, "title")
		desc := extractXMLElement(prog, "desc")

		fmt.Printf("  %d. %s\n", count+1, title)
		if channel != "" {
			fmt.Printf("     channel: %s\n", channel)
		}
		if start != "" {
			fmt.Printf("     start: %s\n", start)
		}
		if desc != "" {
			fmt.Printf("     desc: %s\n", truncateString(desc, 80))
		}
		count++
	}
	fmt.Println("---")
}
