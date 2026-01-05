// Package main provides an E2E test runner for validating the tvarr pipeline.
// This binary tests the complete flow from source ingestion through M3U/XMLTV output.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
)

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
		captureLogs       = flag.Bool("capture-logs", true, "Capture tvarr stdout/stderr to logs/ directory")

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
	var logFiles *LogFiles

	// Set up log file capture if enabled
	if *captureLogs {
		var err error
		logFiles, err = SetupLogFiles()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to setup log files: %v\n", err)
			os.Exit(1)
		}
		defer logFiles.Close()
		fmt.Printf("Log capture enabled: %s\n", logFiles.Dir())
	}

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
		// Use provided URLs (no defaults since they're typically generated)
		if *m3uURL != "" {
			effectiveM3UURL = *m3uURL
		}
		if *epgURL != "" {
			effectiveEPGURL = *epgURL
		}
	}

	// If binary path is provided, start a managed server
	if *binaryPath != "" {
		var err error
		server, err = NewManagedServer(*binaryPath, logFiles)
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
		fmt.Printf("Capture Logs:        %v\n", *captureLogs)
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
		TestdataServer:    testdataServer,
	})
	fmt.Printf("Run ID:              %s\n", runner.RunID())
	fmt.Println()
	_ = runner.Run(ctx)

	exitCode := runner.PrintSummary()

	// Ensure stdout is flushed before exit (helps when piped)
	os.Stdout.Sync()
	os.Stderr.Sync()

	os.Exit(exitCode)
}
