package main

import (
	"net/http"
	"time"
)

// Server and client timeout constants for E2E runner.
const (
	// DefaultTimeout is the default timeout for E2E operations.
	DefaultTimeout = 5 * time.Minute

	// HealthCheckTimeout is the timeout for health check requests.
	HealthCheckTimeout = 2 * time.Second

	// ProcessKillTimeout is the time to wait before force-killing a process.
	ProcessKillTimeout = 5 * time.Second

	// ServerReadHeaderTimeout is the ReadHeaderTimeout for HTTP servers.
	ServerReadHeaderTimeout = 10 * time.Second

	// ServerShutdownTimeout is the timeout for graceful server shutdown.
	ServerShutdownTimeout = 2 * time.Second

	// SSEWaitTimeout is the timeout for waiting on SSE events.
	SSEWaitTimeout = 2 * time.Second

	// StreamTestTimeout is the timeout for stream test requests.
	StreamTestTimeout = 10 * time.Second
)

// TestResult represents the outcome of a single test.
type TestResult struct {
	Name    string
	Passed  bool
	Message string
	Elapsed time.Duration
}

// ProgressEvent represents a captured SSE progress event.
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

// CreateStreamProxyOptions holds options for creating a stream proxy.
type CreateStreamProxyOptions struct {
	Name              string
	StreamSourceIDs   []string
	EpgSourceIDs      []string
	CacheChannelLogos bool
	CacheProgramLogos bool
	ProxyMode         string // "direct" (HTTP 302 redirect) or "smart" (auto-optimizes delivery)
	RelayProfileID    string // Optional relay profile ID for transcoding (only used with "smart" mode)
}

// ClientDetectionMapping represents a client detection rule.
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

// ClientDetectionStats represents statistics about client detection mappings.
type ClientDetectionStats struct {
	Total   int `json:"total"`
	Enabled int `json:"enabled"`
	System  int `json:"system"`
	Custom  int `json:"custom"`
}

// ThemeInfo represents theme metadata.
type ThemeInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

// ThemeListResponse represents the response from the themes API.
type ThemeListResponse struct {
	Themes  []ThemeInfo `json:"themes"`
	Default string      `json:"default"`
}

// UploadLogoResult contains the result of uploading a logo.
type UploadLogoResult struct {
	ID  string // ULID of the uploaded logo
	URL string // Full URL to access the logo
}

// E2ERunnerOptions holds configuration options for the E2E runner.
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
	TestdataServer    *TestdataServer // Pre-created testdata server (optional)
}

// operationInfo holds processed information about an operation for the waterfall view.
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

// stageInfo holds information about a stage within an operation.
type stageInfo struct {
	name      string
	startTime time.Time
	endTime   time.Time
	duration  time.Duration
	updates   int
}
