package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/jmylchreest/tvarr/internal/config"
	"github.com/jmylchreest/tvarr/internal/database/migrations"
	"github.com/jmylchreest/tvarr/internal/ffmpeg"
	internalhttp "github.com/jmylchreest/tvarr/internal/http"
	"github.com/jmylchreest/tvarr/internal/http/handlers"
	"github.com/jmylchreest/tvarr/internal/ingestor"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/observability"
	"github.com/jmylchreest/tvarr/internal/pipeline"
	"github.com/jmylchreest/tvarr/internal/relay"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/jmylchreest/tvarr/internal/scheduler"
	"github.com/jmylchreest/tvarr/internal/service"
	"github.com/jmylchreest/tvarr/internal/service/logs"
	"github.com/jmylchreest/tvarr/internal/service/progress"
	"github.com/jmylchreest/tvarr/internal/startup"
	"github.com/jmylchreest/tvarr/internal/storage"
	"github.com/jmylchreest/tvarr/internal/urlutil"
	"github.com/jmylchreest/tvarr/internal/version"
	"github.com/jmylchreest/tvarr/pkg/duration"
	"github.com/jmylchreest/tvarr/pkg/httpclient"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the tvarr server",
	Long: `Start the tvarr HTTP server and API.

The server provides:
- REST API for managing stream sources, EPG sources, and proxies
- Health check endpoint
- OpenAPI documentation at /docs`,
	RunE: runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)

	// Server flags
	serveCmd.Flags().String("host", "0.0.0.0", "Host to bind to")
	serveCmd.Flags().Int("port", 8080, "Port to listen on")
	serveCmd.Flags().String("base-url", "", "Base URL for external access (e.g., https://www.mysite.com). Defaults to http://host:port")
	serveCmd.Flags().String("database", "tvarr.db", "Database DSN (file path for SQLite, connection string for others)")
	serveCmd.Flags().String("data-dir", "./data", "Data directory for output files")

	// Pipeline flags
	serveCmd.Flags().Bool("ingestion-guard", true, "Enable ingestion guard (waits for active ingestions)")

	// Scheduler flags
	serveCmd.Flags().Duration("scheduler-sync-interval", time.Minute, "Interval for syncing schedules from database")
	serveCmd.Flags().Int("scheduler-workers", 2, "Number of concurrent job workers")
	serveCmd.Flags().String("logo-scan-schedule", scheduler.DefaultLogoScanSchedule, "Cron schedule for logo scan job (6-field: sec min hour dom month dow). 7-field with year also accepted for legacy. Empty to disable.")
	serveCmd.Flags().Duration("job-history-retention", 14*24*time.Hour, "Retention period for job history records (older records are deleted on startup)")

	// gRPC server flags (for ffmpegd daemon registration)
	serveCmd.Flags().Bool("grpc-enabled", false, "Enable gRPC server for ffmpegd daemon registration")
	serveCmd.Flags().Int("grpc-port", 9090, "Port for gRPC server")
	serveCmd.Flags().String("grpc-auth-token", "", "Authentication token for ffmpegd daemons (optional)")

	// Bind flags to viper
	mustBindPFlag("server.host", serveCmd.Flags().Lookup("host"))
	mustBindPFlag("server.port", serveCmd.Flags().Lookup("port"))
	mustBindPFlag("server.base_url", serveCmd.Flags().Lookup("base-url"))
	mustBindPFlag("database.dsn", serveCmd.Flags().Lookup("database"))
	mustBindPFlag("storage.base_dir", serveCmd.Flags().Lookup("data-dir"))
	mustBindPFlag("pipeline.ingestion_guard", serveCmd.Flags().Lookup("ingestion-guard"))
	mustBindPFlag("scheduler.sync_interval", serveCmd.Flags().Lookup("scheduler-sync-interval"))
	mustBindPFlag("scheduler.workers", serveCmd.Flags().Lookup("scheduler-workers"))
	mustBindPFlag("scheduler.logo_scan_schedule", serveCmd.Flags().Lookup("logo-scan-schedule"))
	mustBindPFlag("scheduler.job_history_retention", serveCmd.Flags().Lookup("job-history-retention"))
	mustBindPFlag("grpc.enabled", serveCmd.Flags().Lookup("grpc-enabled"))
	mustBindPFlag("grpc.port", serveCmd.Flags().Lookup("grpc-port"))
	mustBindPFlag("grpc.auth_token", serveCmd.Flags().Lookup("grpc-auth-token"))
}

func runServe(_ *cobra.Command, _ []string) error {
	// Log level is already set by initLogging() in PersistentPreRunE.
	// Here we just ensure the runtime-modifiable GlobalLogLevel is synced,
	// which is needed for runtime log level changes via API.
	logLevel := viper.GetString("logging.level")
	if rootCmd.PersistentFlags().Changed("log-level") {
		logLevel, _ = rootCmd.PersistentFlags().GetString("log-level")
	}
	if logLevel == "" {
		logLevel = "info"
	}
	observability.SetLogLevel(logLevel)

	// Initialize request logging from config
	observability.SetRequestLogging(viper.GetBool("logging.request_logging"))

	// Initialize logs service and wrap the default slog handler
	logsService := logs.New()
	wrappedHandler := logsService.WrapHandler(slog.Default().Handler())
	slog.SetDefault(slog.New(wrappedHandler))

	logger := slog.Default()

	// T055/T057: Clean up orphaned temp directories from previous runs
	orphansRemoved, err := startup.CleanupSystemTempDirs(logger)
	if err != nil {
		logger.Warn("failed to clean orphaned temp directories",
			slog.String("error", err.Error()),
		)
	} else if orphansRemoved > 0 {
		logger.Info("cleaned orphaned temp directories on startup",
			slog.Int("removed_count", orphansRemoved),
		)
	}

	// Initialize database
	db, err := initDatabase(viper.GetString("database.dsn"))
	if err != nil {
		return fmt.Errorf("initializing database: %w", err)
	}

	// Run migrations
	if err := runMigrations(db, logger); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	// Detect FFmpeg and log capabilities on startup
	ffmpegDetector := ffmpeg.NewBinaryDetector()
	ffmpegInfo, err := ffmpegDetector.Detect(context.Background())
	if err != nil {
		logger.Warn("ffmpeg not detected", slog.String("error", err.Error()))
	} else {
		// Collect available hardware accelerators as a slice
		var hwAccelNames []string
		for _, accel := range ffmpegInfo.HWAccels {
			if accel.Available {
				if accel.DeviceName != "" {
					hwAccelNames = append(hwAccelNames, fmt.Sprintf("%s (%s)", accel.Name, accel.DeviceName))
				} else {
					hwAccelNames = append(hwAccelNames, accel.Name)
				}
			}
		}

		// Get recommended hardware accelerator
		var recommendedHWAccel string
		if recommended := ffmpeg.GetRecommendedHWAccel(ffmpegInfo.HWAccels); recommended != nil {
			recommendedHWAccel = recommended.Name
		}

		logger.Info("ffmpeg detected",
			slog.String("version", ffmpegInfo.Version),
			slog.String("path", ffmpegInfo.FFmpegPath),
			slog.Int("encoder_count", len(ffmpegInfo.Encoders)),
			slog.Int("decoder_count", len(ffmpegInfo.Decoders)),
			slog.Any("hw_accels", hwAccelNames),
			slog.String("recommended_hw_accel", recommendedHWAccel),
		)

		logger.Info("ffprobe detected",
			slog.String("path", ffmpegInfo.FFprobePath),
		)
	}

	// Initialize repositories
	streamSourceRepo := repository.NewStreamSourceRepository(db)
	channelRepo := repository.NewChannelRepository(db)
	manualChannelRepo := repository.NewManualChannelRepository(db)
	epgSourceRepo := repository.NewEpgSourceRepository(db)
	epgProgramRepo := repository.NewEpgProgramRepository(db)
	proxyRepo := repository.NewStreamProxyRepository(db)
	filterRepo := repository.NewFilterRepository(db)
	dataMappingRuleRepo := repository.NewDataMappingRuleRepository(db)
	encodingProfileRepo := repository.NewEncodingProfileRepository(db)
	lastKnownCodecRepo := repository.NewLastKnownCodecRepository(db)
	clientDetectionRuleRepo := repository.NewClientDetectionRuleRepository(db)
	encoderOverrideRepo := repository.NewEncoderOverrideRepository(db)
	jobRepo := repository.NewJobRepository(db)

	// Clean up old job history on startup if retention is configured
	jobHistoryRetention := viper.GetDuration("scheduler.job_history_retention")
	if jobHistoryRetention > 0 {
		cutoff := time.Now().Add(-jobHistoryRetention)
		deleted, err := jobRepo.DeleteHistory(context.Background(), cutoff)
		if err != nil {
			logger.Warn("failed to clean job history", slog.Any("error", err))
		} else if deleted > 0 {
			logger.Info("cleaned old job history records on startup",
				slog.Int64("deleted_count", deleted),
				slog.String("retention", duration.Format(jobHistoryRetention)))
		}
	}

	// Initialize storage sandbox
	sandbox, err := storage.NewSandbox(viper.GetString("storage.base_dir"))
	if err != nil {
		return fmt.Errorf("initializing storage: %w", err)
	}

	// Initialize logo cache and service
	logoCache, err := storage.NewLogoCache(viper.GetString("storage.base_dir"))
	if err != nil {
		return fmt.Errorf("initializing logo cache: %w", err)
	}

	// Configure logo service with circuit breaker and concurrency settings from config
	logoServiceConfig := service.LogoServiceConfig{
		Timeout:            viper.GetDuration("pipeline.logo_timeout"),
		RetryAttempts:      viper.GetInt("pipeline.logo_retry_attempts"),
		CircuitBreakerName: viper.GetString("pipeline.logo_circuit_breaker"),
		Concurrency:        viper.GetInt("pipeline.logo_concurrency"),
	}

	// Apply defaults if not configured
	if logoServiceConfig.Timeout == 0 {
		logoServiceConfig.Timeout = 30 * time.Second
	}
	if logoServiceConfig.RetryAttempts == 0 {
		logoServiceConfig.RetryAttempts = 3
	}
	if logoServiceConfig.CircuitBreakerName == "" {
		logoServiceConfig.CircuitBreakerName = "logos"
	}
	if logoServiceConfig.Concurrency == 0 {
		logoServiceConfig.Concurrency = 10
	}

	logger.Info("initializing logo service",
		slog.Duration("timeout", logoServiceConfig.Timeout),
		slog.Int("retry_attempts", logoServiceConfig.RetryAttempts),
		slog.String("circuit_breaker", logoServiceConfig.CircuitBreakerName),
		slog.Int("concurrency", logoServiceConfig.Concurrency))

	logoService := service.NewLogoServiceWithConfig(logoCache, logoServiceConfig).
		WithLogger(logger)

	// Load logo index with pruning of stale cached logos
	logoRetention := viper.GetDuration("storage.logo_retention")
	if logoRetention > 0 {
		logoService = logoService.WithStalenessThreshold(logoRetention)
		result, err := logoService.LoadIndexWithOptions(context.Background(), service.LogoIndexerOptions{
			PruneStaleLogos:    true,
			StalenessThreshold: logoRetention,
		})
		if err != nil {
			return fmt.Errorf("loading logo index: %w", err)
		}
		if result.PrunedCount > 0 {
			logger.Info("pruned stale logos on startup",
				slog.Int("pruned_count", result.PrunedCount),
				slog.Int64("pruned_bytes", result.PrunedSize),
				slog.String("retention", duration.Format(logoRetention)))
		}
	} else {
		if err := logoService.LoadIndex(context.Background()); err != nil {
			return fmt.Errorf("loading logo index: %w", err)
		}
	}

	// Initialize ingestor components
	stateManager := ingestor.NewStateManager()
	streamHandlerFactory := ingestor.NewHandlerFactory()
	streamHandlerFactory.RegisterManualHandler(manualChannelRepo) // Add manual source support
	epgHandlerFactory := ingestor.NewEpgHandlerFactory()

	// Initialize pipeline factory with default stages and optional ingestion guard
	var ingestionGuardStateManager *ingestor.StateManager
	if viper.GetBool("pipeline.ingestion_guard") {
		ingestionGuardStateManager = stateManager
		logger.Info("ingestion guard enabled for proxy generation")
	}

	logger.Debug("creating pipeline factory")

	// Construct base URL for logo resolution in pipeline output
	// This allows generated M3U/XMLTV to contain fully qualified logo URLs
	// If base_url is configured, use it (normalized). Otherwise fall back to host:port.
	serverHost := viper.GetString("server.host")
	serverPort := viper.GetInt("server.port")
	baseURL := urlutil.NormalizeBaseURL(viper.GetString("server.base_url"))
	if baseURL == "" {
		// Fall back to constructing from host:port
		if serverHost == "0.0.0.0" || serverHost == "" {
			baseURL = fmt.Sprintf("http://localhost:%d", serverPort)
		} else {
			baseURL = fmt.Sprintf("http://%s:%d", serverHost, serverPort)
		}
	}

	pipelineFactory := pipeline.NewDefaultFactory(
		channelRepo,
		epgProgramRepo,
		filterRepo,
		dataMappingRuleRepo,
		sandbox,
		logger,
		logoService, // Logo caching enabled
		ingestionGuardStateManager,
		baseURL,
	)
	logger.Debug("pipeline factory created")

	// Initialize progress service
	logger.Debug("initializing progress service")
	progressService := progress.NewService(logger)
	progressService.Start()
	defer progressService.Stop()
	logger.Debug("progress service started")

	// Initialize services
	logger.Debug("initializing services")
	sourceService := service.NewSourceService(
		streamSourceRepo,
		channelRepo,
		streamHandlerFactory,
		stateManager,
	).
		WithLogger(logger).
		WithProgressService(progressService).
		WithEPGSourceRepo(epgSourceRepo).
		WithEPGChecker(service.NewDefaultEPGChecker())

	epgService := service.NewEpgService(
		epgSourceRepo,
		epgProgramRepo,
		epgHandlerFactory,
		stateManager,
	).WithLogger(logger).WithProgressService(progressService)

	proxyService := service.NewProxyService(
		proxyRepo,
		pipelineFactory,
	).WithLogger(logger).WithProgressService(progressService)

	relayService := service.NewRelayService(
		encodingProfileRepo,
		lastKnownCodecRepo,
		channelRepo,
		proxyRepo,
	).WithLogger(logger).WithBufferConfig(config.BufferConfig{
		MaxVariantBytes: viperGetByteSizePtr("relay.buffer.max_variant_bytes"),
	}).WithHLSConfig(config.HLSConfig{
		TargetSegmentDuration: viper.GetFloat64("relay.hls.target_segment_duration"),
		MaxSegments:           viper.GetInt("relay.hls.max_segments"),
		PlaylistSegments:      viper.GetInt("relay.hls.playlist_segments"),
	})

	encodingProfileService := service.NewEncodingProfileService(encodingProfileRepo).
		WithLogger(logger)

	clientDetectionService := service.NewClientDetectionService(clientDetectionRuleRepo).
		WithLogger(logger)
	// Refresh client detection rules cache on startup
	if err := clientDetectionService.RefreshCache(context.Background()); err != nil {
		logger.Warn("failed to refresh client detection rules cache", slog.String("error", err.Error()))
	}

	encoderOverrideService := service.NewEncoderOverrideService(encoderOverrideRepo).
		WithLogger(logger)
	// Refresh encoder overrides cache on startup
	if err := encoderOverrideService.RefreshCache(context.Background()); err != nil {
		logger.Warn("failed to refresh encoder overrides cache", slog.String("error", err.Error()))
	}

	logger.Debug("services initialized")

	// Initialize backup service (needed for both scheduler and HTTP handler)
	backupConfig := config.BackupConfig{
		Directory: viper.GetString("backup.directory"),
		Schedule: config.BackupScheduleConfig{
			Enabled:   viper.GetBool("backup.schedule.enabled"),
			Cron:      viper.GetString("backup.schedule.cron"),
			Retention: viper.GetInt("backup.schedule.retention"),
		},
	}
	backupService := service.NewBackupService(db, backupConfig, viper.GetString("storage.base_dir")).
		WithLogger(logger)

	// Configure internal recurring jobs
	logger.Debug("configuring scheduler")
	var internalJobs []scheduler.InternalJobConfig
	logoScanSchedule := viper.GetString("scheduler.logo_scan_schedule")
	if logoScanSchedule != "" {
		internalJobs = append(internalJobs, scheduler.InternalJobConfig{
			JobType:      models.JobTypeLogoCleanup,
			TargetName:   "Logo Maintenance",
			CronSchedule: logoScanSchedule,
		})
	}

	// Note: Backup job is added dynamically after scheduler starts to use DB settings
	// See the code block after sched.Start(ctx) below

	// Initialize scheduler and runner
	sched := scheduler.NewScheduler(
		jobRepo,
		streamSourceRepo,
		epgSourceRepo,
		proxyRepo,
	).WithLogger(logger).WithConfig(scheduler.SchedulerConfig{
		SyncInterval: viper.GetDuration("scheduler.sync_interval"),
		InternalJobs: internalJobs,
	})

	// Create auto-regeneration service
	autoRegenService := scheduler.NewAutoRegenService(proxyRepo, sched).WithLogger(logger)

	// Create job executor with handlers
	executor := scheduler.NewExecutor(jobRepo).WithLogger(logger)

	// Register stream ingestion handler with auto-regeneration
	streamIngestionHandler := scheduler.NewStreamIngestionHandler(sourceService).
		WithAutoRegeneration(autoRegenService).
		WithLogger(logger)
	executor.RegisterHandler(models.JobTypeStreamIngestion, streamIngestionHandler)

	// Register EPG ingestion handler with auto-regeneration
	epgIngestionHandler := scheduler.NewEpgIngestionHandler(epgService).
		WithAutoRegeneration(autoRegenService).
		WithLogger(logger)
	executor.RegisterHandler(models.JobTypeEpgIngestion, epgIngestionHandler)

	// Register proxy generation handler with a wrapper function
	proxyGenerateFunc := func(ctx context.Context, proxyID models.ULID) (*scheduler.ProxyGenerateResult, error) {
		result, err := proxyService.Generate(ctx, proxyID)
		if err != nil {
			return nil, err
		}
		return &scheduler.ProxyGenerateResult{
			ChannelCount: result.ChannelCount,
			ProgramCount: result.ProgramCount,
		}, nil
	}
	proxyGenHandler := scheduler.NewProxyGenerationHandler(proxyGenerateFunc)
	executor.RegisterHandler(models.JobTypeProxyGeneration, proxyGenHandler)

	// Register logo maintenance handler
	logoMaintenanceHandler := scheduler.NewLogoMaintenanceHandler(logoService).WithLogger(logger)
	executor.RegisterHandler(models.JobTypeLogoCleanup, logoMaintenanceHandler)

	// Register backup handler (using adapter to implement scheduler interface)
	backupJobHandler := scheduler.NewBackupJobHandler(&backupServiceAdapter{backupService}).WithLogger(logger)
	executor.RegisterHandler(models.JobTypeBackup, backupJobHandler)

	// Create job runner
	runner := scheduler.NewRunner(jobRepo, executor).
		WithLogger(logger).
		WithConfig(scheduler.RunnerConfig{
			WorkerCount: viper.GetInt("scheduler.workers"),
		})
	logger.Debug("scheduler and runner created")

	// Initialize job service with scheduler and runner
	jobService := service.NewJobService(jobRepo, streamSourceRepo, epgSourceRepo, proxyRepo).
		WithLogger(logger).
		WithScheduler(sched).
		WithRunner(runner)

	// Initialize HTTP server
	serverConfig := internalhttp.ServerConfig{
		Host: viper.GetString("server.host"),
		Port: viper.GetInt("server.port"),
	}
	server := internalhttp.NewServer(serverConfig, logger, version.Version)

	// Register OpenAPI docs handler with system theme detection (dark/light)
	docsHandler := handlers.NewDocsHandler("tvarr API", "/openapi.yaml", handlers.WithSystemTheme())
	server.Router().Get("/docs", docsHandler.ServeHTTP)

	// Register logo file server for serving logo images at /logos/{filename}
	logoHandler := handlers.NewLogoHandler(logoService)
	logoHandler.RegisterFileServer(server.Router())

	// Register output file server for serving M3U and XMLTV files at /proxy/{id}.m3u and /proxy/{id}.xmltv
	outputHandler := handlers.NewOutputHandler(sandbox).WithLogger(logger)
	outputHandler.RegisterFileServer(server.Router())

	// Register static handler as NotFound fallback for SPA routing
	// This ensures specific routes (like /logos/*) are matched first
	staticHandler := handlers.NewStaticHandler()
	server.Router().NotFound(staticHandler.ServeHTTP)

	// Register handlers
	healthHandler := handlers.NewHealthHandler(version.Version).WithDB(db)
	healthHandler.Register(server.API())

	streamSourceHandler := handlers.NewStreamSourceHandler(sourceService).
		WithScheduleSyncer(sched).
		WithProxyUsageChecker(proxyRepo)
	streamSourceHandler.Register(server.API())

	epgSourceHandler := handlers.NewEpgSourceHandler(epgService).
		WithScheduleSyncer(sched).
		WithProxyUsageChecker(proxyRepo)
	epgSourceHandler.Register(server.API())

	unifiedSourcesHandler := handlers.NewUnifiedSourcesHandler(sourceService, epgService)
	unifiedSourcesHandler.Register(server.API())

	proxyHandler := handlers.NewStreamProxyHandler(proxyService).WithScheduleSyncer(sched)
	proxyHandler.Register(server.API())

	expressionHandler := handlers.NewExpressionHandler(channelRepo, epgProgramRepo)
	expressionHandler.Register(server.API())

	filterHandler := handlers.NewFilterHandler(filterRepo).WithProxyUsageChecker(proxyRepo)
	filterHandler.Register(server.API())

	dataMappingRuleHandler := handlers.NewDataMappingRuleHandler(dataMappingRuleRepo)
	dataMappingRuleHandler.Register(server.API())

	progressHandler := handlers.NewProgressHandler(progressService)
	progressHandler.Register(server.API())
	progressHandler.RegisterSSE(server.Router())

	featureHandler := handlers.NewFeatureHandler()
	featureHandler.Register(server.API())

	settingsHandler := handlers.NewSettingsHandler()
	settingsHandler.Register(server.API())

	// Initialize theme service and handler
	themeService := service.NewThemeService(viper.GetString("storage.base_dir")).WithLogger(logger)
	if err := themeService.EnsureThemesDirectory(); err != nil {
		logger.Warn("failed to create themes directory", slog.String("error", err.Error()))
	}
	themeHandler := handlers.NewThemeHandler(themeService)
	themeHandler.Register(server.API())
	themeHandler.RegisterChiRoutes(server.Router())

	logoHandler.Register(server.API())

	encodingProfileHandler := handlers.NewEncodingProfileHandler(encodingProfileService).
		WithProxyUsageChecker(proxyRepo)
	encodingProfileHandler.Register(server.API())

	relayStreamHandler := handlers.NewRelayStreamHandler(relayService).
		WithLogger(logger).
		WithClientDetectionService(clientDetectionService)
	relayStreamHandler.Register(server.API())
	relayStreamHandler.RegisterChiRoutes(server.Router())

	clientDetectionRuleHandler := handlers.NewClientDetectionRuleHandler(clientDetectionService)
	clientDetectionRuleHandler.Register(server.API())

	encoderOverrideHandler := handlers.NewEncoderOverrideHandler(encoderOverrideService)
	encoderOverrideHandler.Register(server.API())

	channelHandler := handlers.NewChannelHandler(db).WithLogger(logger)
	channelHandler.Register(server.API())

	// Manual channel handler for managing channels in manual stream sources
	manualChannelService := service.NewManualChannelService(manualChannelRepo, streamSourceRepo).
		WithLogger(logger)
	manualChannelHandler := handlers.NewManualChannelHandler(manualChannelService)
	manualChannelHandler.Register(server.API())

	epgHandler := handlers.NewEpgHandler(db)
	epgHandler.Register(server.API())

	circuitBreakerHandler := handlers.NewCircuitBreakerHandler(httpclient.DefaultManager)
	circuitBreakerHandler.Register(server.API())

	logsHandler := handlers.NewLogsHandler(logsService)
	logsHandler.Register(server.API())
	logsHandler.RegisterSSE(server.Router())

	systemHandler := handlers.NewSystemHandler(relayService)
	systemHandler.Register(server.API())
	logger.Debug("http handlers registered")

	// Register job handler
	jobHandler := handlers.NewJobHandler(jobService)
	jobHandler.Register(server.API())

	// Register export/import handlers
	exportService := service.NewExportService(
		filterRepo,
		dataMappingRuleRepo,
		clientDetectionRuleRepo,
		encodingProfileRepo,
	).WithLogger(logger)

	importService := service.NewImportService(
		db,
		filterRepo,
		dataMappingRuleRepo,
		clientDetectionRuleRepo,
		encodingProfileRepo,
	).WithLogger(logger)

	exportHandler := handlers.NewExportHandler(exportService, importService)
	exportHandler.Register(server.API())

	// Register backup/restore handlers (backupService created earlier for scheduler)
	// Create schedule updater adapter that connects handler to scheduler
	scheduleUpdater := &backupScheduleUpdaterAdapter{scheduler: sched}
	backupHandler := handlers.NewBackupHandler(backupService).
		WithScheduleUpdater(scheduleUpdater)
	backupHandler.Register(server.API())
	backupHandler.RegisterChiRoutes(server.Router())

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logger.Info("received shutdown signal", slog.String("signal", sig.String()))
		cancel()
	}()
	logger.Debug("signal handler registered")

	// Start scheduler
	logger.Debug("starting scheduler")
	if err := sched.Start(ctx); err != nil {
		return fmt.Errorf("starting scheduler: %w", err)
	}
	defer sched.Stop()

	// Register backup job with effective settings from database
	// This uses DB settings merged with config defaults
	effectiveSchedule := backupService.GetScheduleInfo(ctx)
	if effectiveSchedule.Enabled && effectiveSchedule.Cron != "" {
		if err := sched.AddInternalJob(models.JobTypeBackup, "Scheduled Backup", effectiveSchedule.Cron); err != nil {
			logger.Warn("failed to register backup job", slog.Any("error", err))
		} else {
			logger.Info("scheduled backups configured",
				slog.String("schedule", effectiveSchedule.Cron),
				slog.Int("retention", effectiveSchedule.Retention))
		}
	}

	// Schedule startup jobs BEFORE starting the runner to avoid database contention.
	// The runner's workers immediately start polling for jobs, so we need all
	// startup jobs to be created first.

	// Schedule initial logo maintenance job on startup if configured
	if logoScanSchedule != "" {
		if _, err := sched.ScheduleImmediate(ctx, models.JobTypeLogoCleanup, models.ULID{}, "Logo Maintenance"); err != nil {
			logger.Warn("failed to schedule initial logo maintenance job", slog.Any("error", err))
		}
		logger.Info("logo maintenance configured",
			slog.String("schedule", logoScanSchedule))
	}

	// Catch up on any missed scheduled runs if enabled
	if viper.GetBool("scheduler.catchup_missed_runs") {
		if _, _, err := sched.CatchupMissedRuns(ctx); err != nil {
			logger.Warn("failed to catch up missed runs", slog.Any("error", err))
		}
	}

	// Start job runner (workers begin polling for jobs)
	if err := runner.Start(ctx); err != nil {
		return fmt.Errorf("starting runner: %w", err)
	}
	defer runner.Stop()

	// Start gRPC server (always created for internal Unix socket, optional TCP for remote daemons)
	// The internal Unix socket is always available for local subprocess communication
	grpcConfig := &relay.GRPCServerConfig{
		AuthToken:         viper.GetString("grpc.auth_token"),
		HeartbeatInterval: 5 * time.Second,
	}

	// Enable external TCP listener if gRPC is explicitly enabled
	if viper.GetBool("grpc.enabled") {
		grpcPort := viper.GetInt("grpc.port")
		grpcConfig.ExternalListenAddr = fmt.Sprintf(":%d", grpcPort)
	}

	// Create daemon registry and gRPC server
	daemonRegistry := relay.NewDaemonRegistry(logger)
	grpcServer := relay.NewGRPCServer(logger, grpcConfig, daemonRegistry)

	if err := grpcServer.Start(ctx); err != nil {
		return fmt.Errorf("starting gRPC server: %w", err)
	}
	defer grpcServer.Stop(ctx)

	if viper.GetBool("grpc.enabled") {
		logger.Info("gRPC server started for ffmpegd daemon registration",
			slog.Int("port", viper.GetInt("grpc.port")),
		)
	}

	// Create FFmpegD spawner for local subprocess transcoding
	// Uses the internal Unix socket to connect subprocesses to coordinator
	spawner := relay.NewFFmpegDSpawner(relay.FFmpegDSpawnerConfig{
		CoordinatorAddress: grpcServer.InternalAddress(),
		AuthToken:          viper.GetString("grpc.auth_token"),
		Logger:             logger,
	})
	spawner.SetRegistry(daemonRegistry)

	// Log local ffmpegd capabilities on startup
	spawner.LogCapabilities(ctx)

	// Register ffmpegd REST API handler for transcoder dashboard
	ffmpegdService := service.NewFFmpegDService(daemonRegistry, logger)
	ffmpegdService.SetJobProvider(grpcServer.GetJobManager())
	ffmpegdHandler := handlers.NewFFmpegDHandler(ffmpegdService)
	ffmpegdHandler.Register(server.API())

	// Configure relay service with distributed transcoding components
	// - daemonRegistry: tracks remote and local daemons
	// Configure encoder overrides provider for transcoding
	// This must be called before WithDistributedTranscoding so the provider is available
	relayService.WithEncoderOverridesProvider(encoderOverrideService.GetEnabledProto)

	// - streamMgr/jobMgr: for routing jobs through coordinator's gRPC streams
	// - spawner: for local subprocess transcoding
	// - preferRemote: true if external gRPC is enabled (remote daemons available)
	relayService.WithDistributedTranscoding(
		daemonRegistry,
		grpcServer.GetStreamManager(),
		grpcServer.GetJobManager(),
		spawner,
		viper.GetBool("grpc.enabled"), // prefer remote if external port is enabled
	)

	// Start server
	logger.Info("starting tvarr server",
		slog.String("host", serverConfig.Host),
		slog.Int("port", serverConfig.Port),
		slog.String("version", version.Version),
		slog.Int("scheduler_entries", sched.GetEntryCount()),
		slog.Int("worker_count", viper.GetInt("scheduler.workers")),
	)

	return server.ListenAndServe(ctx)
}

func initDatabase(path string) (*gorm.DB, error) {
	// Resolve to absolute path for clarity in logs
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path // Fall back to original if resolution fails
	}

	slog.Info("initializing database",
		slog.String("path", path),
		slog.String("absolute_path", absPath),
	)

	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	return db, nil
}

func runMigrations(db *gorm.DB, logger *slog.Logger) error {
	migrator := migrations.NewMigrator(db, logger)
	migrator.RegisterAll(migrations.AllMigrations())
	return migrator.Up(context.Background())
}

// viperGetByteSizePtr returns a pointer to a ByteSize if the key is set, nil otherwise.
// This allows distinguishing between "not set" and "set to 0".
// Supports human-readable values like "100MB", "1GB", or raw byte counts.
func viperGetByteSizePtr(key string) *config.ByteSize {
	if !viper.IsSet(key) {
		return nil
	}
	// Get as string first to support human-readable format
	s := viper.GetString(key)
	if s == "" {
		return nil
	}
	bs, err := config.ParseByteSize(s)
	if err != nil {
		// Fall back to numeric value for backwards compatibility
		v := viper.GetInt64(key)
		bs = config.ByteSize(v)
	}
	return &bs
}

// backupServiceAdapter adapts *service.BackupService to implement scheduler.BackupCreateService.
type backupServiceAdapter struct {
	svc *service.BackupService
}

// backupResult wraps the backup filename for scheduler use.
type backupResult struct {
	filename string
}

func (r *backupResult) GetFilename() string {
	return r.filename
}

func (a *backupServiceAdapter) CreateBackupForScheduler(ctx context.Context) (scheduler.BackupCreateResult, error) {
	meta, err := a.svc.CreateBackup(ctx)
	if err != nil {
		return nil, err
	}
	return &backupResult{filename: meta.Filename}, nil
}

func (a *backupServiceAdapter) CleanupOldBackups(ctx context.Context) (int, error) {
	return a.svc.CleanupOldBackups(ctx)
}

// backupScheduleUpdaterAdapter adapts *scheduler.Scheduler to implement handlers.BackupScheduleUpdater.
type backupScheduleUpdaterAdapter struct {
	scheduler *scheduler.Scheduler
}

func (a *backupScheduleUpdaterAdapter) UpdateBackupSchedule(enabled bool, cron string) error {
	return a.scheduler.UpdateInternalJob(models.JobTypeBackup, "Scheduled Backup", enabled, cron)
}
