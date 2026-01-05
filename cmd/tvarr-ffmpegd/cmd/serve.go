package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof" //nolint:gosec // G108: Intentional pprof exposure for debugging
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jmylchreest/tvarr/internal/daemon"
	"github.com/jmylchreest/tvarr/internal/version"
	"github.com/spf13/cobra"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the transcoding daemon",
	Long: `Start the tvarr-ffmpegd transcoding daemon.

The daemon will:
1. Detect hardware capabilities (FFmpeg, GPU encoders, session limits)
2. Connect to the coordinator (if TVARR_COORDINATOR_URL is set)
3. Register and report capabilities
4. Accept transcoding jobs via gRPC streaming

In standalone mode (no coordinator URL), the daemon starts but does not
connect to any coordinator. This is useful for testing FFmpeg detection.

Examples:
  # Start and connect to coordinator
  TVARR_COORDINATOR_URL=192.168.1.100:9090 tvarr-ffmpegd serve

  # Start with custom name
  TVARR_DAEMON_NAME=gpu-worker-1 tvarr-ffmpegd serve

  # Standalone mode (detection only)
  tvarr-ffmpegd serve --standalone`,
	RunE: runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)

	// Serve-specific flags
	serveCmd.Flags().Bool("standalone", false, "run in standalone mode (don't connect to coordinator)")
	serveCmd.Flags().String("daemon-id", "", "daemon ID (overrides auto-generated UUID)")
	serveCmd.Flags().String("name", "", "daemon name (overrides TVARR_DAEMON_NAME)")
	serveCmd.Flags().Int("max-jobs", 0, "max concurrent jobs - overall guard limit (0 = use config/default)")
	serveCmd.Flags().Int("max-cpu-jobs", 0, "max concurrent CPU (software) encoding jobs (0 = auto-detect from CPU cores)")
	serveCmd.Flags().Int("max-gpu-jobs", 0, "max concurrent GPU (hardware) encoding jobs (0 = auto-detect from GPU sessions)")
	serveCmd.Flags().Int("max-probe-jobs", 0, "max concurrent probe operations (0 = use max-jobs)")
	serveCmd.Flags().String("listen", "", "gRPC listen address (e.g., :9091)")
	serveCmd.Flags().String("coordinator-url", "", "coordinator gRPC URL (overrides TVARR_COORDINATOR_URL)")
	serveCmd.Flags().String("auth-token", "", "authentication token (overrides TVARR_AUTH_TOKEN)")
	serveCmd.Flags().String("log-level", "", "log level (trace, debug, info, warn, error)")

	// Profiling flags
	serveCmd.Flags().Bool("pprof", false, "enable pprof profiling server")
	serveCmd.Flags().Int("pprof-port", 6060, "port for pprof profiling server")
}

func runServe(cmd *cobra.Command, _ []string) error {
	logger := slog.Default()

	// Log version banner first
	versionInfo := version.GetInfo()
	logger.Info("tvarr-ffmpegd starting",
		slog.String("version", versionInfo.Version),
		slog.String("commit", versionInfo.CommitSHA),
		slog.String("built", versionInfo.Date),
		slog.String("go", versionInfo.GoVersion),
		slog.String("platform", versionInfo.Platform),
	)

	v := GetDaemonViper()

	// Start pprof server if enabled (check CLI flag first, then config/env)
	// Uses daemon.profiling.* to avoid conflict with tvarr in all-in-one image
	// Env vars: TVARR_DAEMON_PROFILING_PPROF, TVARR_DAEMON_PROFILING_PPROF_PORT
	pprofEnabled, _ := cmd.Flags().GetBool("pprof")
	if !pprofEnabled {
		pprofEnabled = v.GetBool("daemon.profiling.pprof")
	}
	if pprofEnabled {
		pprofPort, _ := cmd.Flags().GetInt("pprof-port")
		if pprofPort == 6060 && v.GetInt("daemon.profiling.pprof_port") != 6060 {
			// Use config value if CLI wasn't explicitly set
			pprofPort = v.GetInt("daemon.profiling.pprof_port")
		}
		pprofAddr := fmt.Sprintf("localhost:%d", pprofPort)
		go func() {
			logger.Info("pprof server starting",
				slog.String("address", pprofAddr),
				slog.String("cpu_profile", fmt.Sprintf("http://%s/debug/pprof/profile", pprofAddr)),
				slog.String("heap_profile", fmt.Sprintf("http://%s/debug/pprof/heap", pprofAddr)),
			)
			// Uses http.DefaultServeMux which has pprof handlers registered via blank import
			if err := http.ListenAndServe(pprofAddr, nil); err != nil { //nolint:gosec // G114: pprof server doesn't need timeouts
				logger.Error("pprof server failed", slog.String("error", err.Error()))
			}
		}()
	}

	// Get daemon configuration
	daemonID := v.GetString("daemon.id")
	if id, _ := cmd.Flags().GetString("daemon-id"); id != "" {
		daemonID = id
	} else if daemonID == "" {
		daemonID = uuid.New().String()
	}

	daemonName := v.GetString("daemon.name")
	if name, _ := cmd.Flags().GetString("name"); name != "" {
		daemonName = name
	}

	maxJobs := v.GetInt("daemon.max_jobs")
	if max, _ := cmd.Flags().GetInt("max-jobs"); max > 0 {
		maxJobs = max
	}

	maxCPUJobs := v.GetInt("daemon.max_cpu_jobs")
	if max, _ := cmd.Flags().GetInt("max-cpu-jobs"); max > 0 {
		maxCPUJobs = max
	}

	maxGPUJobs := v.GetInt("daemon.max_gpu_jobs")
	if max, _ := cmd.Flags().GetInt("max-gpu-jobs"); max > 0 {
		maxGPUJobs = max
	}

	maxProbeJobs := v.GetInt("daemon.max_probe_jobs")
	if max, _ := cmd.Flags().GetInt("max-probe-jobs"); max > 0 {
		maxProbeJobs = max
	}

	coordinatorURL := v.GetString("coordinator.url")
	if url, _ := cmd.Flags().GetString("coordinator-url"); url != "" {
		coordinatorURL = url
	}

	authToken := v.GetString("coordinator.auth_token")
	if token, _ := cmd.Flags().GetString("auth-token"); token != "" {
		authToken = token
	}

	standalone, _ := cmd.Flags().GetBool("standalone")
	listenAddr, _ := cmd.Flags().GetString("listen")

	heartbeatInterval := v.GetDuration("coordinator.heartbeat_interval")
	if heartbeatInterval == 0 {
		heartbeatInterval = 5 * time.Second
	}

	reconnectDelay := v.GetDuration("coordinator.reconnect_delay")
	if reconnectDelay == 0 {
		reconnectDelay = 5 * time.Second
	}

	reconnectMaxDelay := v.GetDuration("coordinator.reconnect_max_delay")
	if reconnectMaxDelay == 0 {
		reconnectMaxDelay = 60 * time.Second
	}

	// Log daemon configuration
	logger.Info("daemon configured",
		slog.String("daemon_id", daemonID),
		slog.String("daemon_name", daemonName),
		slog.Int("max_jobs", maxJobs),
		slog.Bool("standalone", standalone || coordinatorURL == ""),
	)

	// Create main context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Detect FFmpeg and hardware capabilities
	detectCtx, detectCancel := context.WithTimeout(ctx, 30*time.Second)
	defer detectCancel()

	capDetector := daemon.NewCapabilityDetector()
	caps, binInfo, err := capDetector.Detect(detectCtx)
	if err != nil {
		return fmt.Errorf("detecting FFmpeg: %w", err)
	}

	// Update job limits from config (only if explicitly set, preserve auto-detected values)
	if maxJobs > 0 {
		caps.MaxConcurrentJobs = int32(maxJobs)
	}
	if maxCPUJobs > 0 {
		caps.MaxCpuJobs = int32(maxCPUJobs)
	}
	if maxGPUJobs > 0 {
		caps.MaxGpuJobs = int32(maxGPUJobs)
	}
	if maxProbeJobs > 0 {
		caps.MaxProbeJobs = int32(maxProbeJobs)
	}

	logger.Info("ffmpeg binaries detected",
		slog.String("version", binInfo.Version),
		slog.String("ffmpeg", binInfo.FFmpegPath),
		slog.String("ffprobe", binInfo.FFprobePath),
		slog.Int("video_encoders", len(caps.VideoEncoders)),
		slog.Int("audio_encoders", len(caps.AudioEncoders)),
		slog.Int("hw_accels", len(caps.HwAccels)),
		slog.Int("gpus", len(caps.Gpus)),
	)

	// Log hardware acceleration details at DEBUG level
	for _, hwaccel := range caps.HwAccels {
		if hwaccel.Available {
			logger.Debug("hardware acceleration details",
				slog.String("type", hwaccel.Type),
				slog.String("device", hwaccel.Device),
				slog.Any("hw_encoders", hwaccel.HwEncoders),
				slog.Any("hw_decoders", hwaccel.HwDecoders),
				slog.Any("filtered_encoders", hwaccel.FilteredEncoders),
			)
		}
	}

	// Log GPU details at DEBUG level
	for _, gpu := range caps.Gpus {
		logger.Debug("GPU details",
			slog.Int("index", int(gpu.Index)),
			slog.String("name", gpu.Name),
			slog.String("class", gpu.GpuClass.String()),
			slog.Int("max_encode_sessions", int(gpu.MaxEncodeSessions)),
		)
	}

	// Check standalone mode
	if standalone || coordinatorURL == "" {
		if coordinatorURL == "" && !standalone {
			logger.Warn("TVARR_COORDINATOR_URL not set, running in standalone mode")
		}
		logger.Info("running in standalone mode - FFmpeg detection complete, not connecting to coordinator")

		// In standalone mode, optionally start a local gRPC server
		if listenAddr != "" {
			server := daemon.NewServer(logger, &daemon.Config{
				ID:                daemonID,
				Name:              daemonName,
				ListenAddr:        listenAddr,
				MaxConcurrentJobs: maxJobs,
				HeartbeatInterval: heartbeatInterval,
				AuthToken:         authToken,
			})

			if err := server.Start(ctx); err != nil {
				return fmt.Errorf("starting server: %w", err)
			}

			// Wait for shutdown
			sig := waitForSignal()
			logger.Info("received shutdown signal", slog.String("signal", sig.String()))

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer shutdownCancel()
			return server.Stop(shutdownCtx)
		}

		// No server, just wait for shutdown
		return waitForShutdown(logger)
	}

	// Validate coordinator configuration
	if authToken == "" {
		logger.Warn("TVARR_AUTH_TOKEN not set, connection may be rejected")
	}

	// Create registration client
	regClient := daemon.NewRegistrationClient(logger, &daemon.RegistrationConfig{
		DaemonID:          daemonID,
		DaemonName:        daemonName,
		CoordinatorURL:    coordinatorURL,
		AuthToken:         authToken,
		MaxConcurrentJobs: maxJobs,
		HeartbeatInterval: heartbeatInterval,
		ReconnectDelay:    reconnectDelay,
		ReconnectMaxDelay: reconnectMaxDelay,
	})

	// Set capabilities
	regClient.SetCapabilities(caps)

	// Create stats collector
	statsCollector := daemon.NewStatsCollector(caps.Gpus)
	regClient.SetStatsCollector(statsCollector)

	logger.Info("connecting to coordinator",
		slog.String("url", coordinatorURL),
		slog.Bool("has_auth", authToken != ""),
	)

	// Connect and register with automatic retry
	if err := regClient.ConnectAndRegister(ctx); err != nil {
		return fmt.Errorf("connecting to coordinator: %w", err)
	}

	// Start the persistent transcode stream (runs in background with automatic retry)
	_ = regClient.StartTranscodeStream(ctx, binInfo)

	logger.Info("daemon registered and running",
		slog.String("state", regClient.GetState().String()),
	)

	// Wait for shutdown signal
	sig := waitForSignal()
	logger.Info("received shutdown signal", slog.String("signal", sig.String()))

	// Graceful shutdown
	logger.Info("initiating graceful shutdown")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Unregister from coordinator
	if err := regClient.Unregister(shutdownCtx, "shutdown"); err != nil {
		logger.Warn("unregister failed", slog.String("error", err.Error()))
	}

	// Close connection
	if err := regClient.Close(); err != nil {
		logger.Warn("close failed", slog.String("error", err.Error()))
	}

	logger.Info("shutdown complete")
	return nil
}

// waitForSignal waits for a shutdown signal and returns it.
func waitForSignal() os.Signal {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	return <-sigCh
}

// waitForShutdown blocks until a shutdown signal is received.
func waitForShutdown(logger *slog.Logger) error {
	sig := waitForSignal()
	logger.Info("received shutdown signal", slog.String("signal", sig.String()))
	return nil
}
