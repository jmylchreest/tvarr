package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/jmylchreest/tvarr/internal/database/migrations"
	"github.com/jmylchreest/tvarr/internal/ffmpeg"
	internalhttp "github.com/jmylchreest/tvarr/internal/http"
	"github.com/jmylchreest/tvarr/internal/http/handlers"
	"github.com/jmylchreest/tvarr/internal/ingestor"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/observability"
	"github.com/jmylchreest/tvarr/internal/pipeline"
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

	// Bind flags to viper
	viper.BindPFlag("server.host", serveCmd.Flags().Lookup("host"))
	viper.BindPFlag("server.port", serveCmd.Flags().Lookup("port"))
	viper.BindPFlag("server.base_url", serveCmd.Flags().Lookup("base-url"))
	viper.BindPFlag("database.dsn", serveCmd.Flags().Lookup("database"))
	viper.BindPFlag("storage.base_dir", serveCmd.Flags().Lookup("data-dir"))
	viper.BindPFlag("pipeline.ingestion_guard", serveCmd.Flags().Lookup("ingestion-guard"))
	viper.BindPFlag("scheduler.sync_interval", serveCmd.Flags().Lookup("scheduler-sync-interval"))
	viper.BindPFlag("scheduler.workers", serveCmd.Flags().Lookup("scheduler-workers"))
	viper.BindPFlag("scheduler.logo_scan_schedule", serveCmd.Flags().Lookup("logo-scan-schedule"))
	viper.BindPFlag("scheduler.job_history_retention", serveCmd.Flags().Lookup("job-history-retention"))
}

func runServe(cmd *cobra.Command, args []string) error {
	// Initialize log level from config before creating logger
	logLevel := viper.GetString("logging.level")
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
	relayProfileRepo := repository.NewRelayProfileRepository(db)
	relayProfileMappingRepo := repository.NewRelayProfileMappingRepository(db)
	lastKnownCodecRepo := repository.NewLastKnownCodecRepository(db)
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
		relayProfileRepo,
		lastKnownCodecRepo,
		channelRepo,
		proxyRepo,
	).WithLogger(logger)

	relayProfileMappingService := service.NewRelayProfileMappingService(relayProfileMappingRepo).
		WithLogger(logger)

	// Validate client detection rules on startup
	relayProfileMappingService.ValidateRulesOnLoad(context.Background())

	logger.Debug("services initialized")

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

	streamSourceHandler := handlers.NewStreamSourceHandler(sourceService)
	streamSourceHandler.Register(server.API())

	epgSourceHandler := handlers.NewEpgSourceHandler(epgService)
	epgSourceHandler.Register(server.API())

	proxyHandler := handlers.NewStreamProxyHandler(proxyService)
	proxyHandler.Register(server.API())

	expressionHandler := handlers.NewExpressionHandler()
	expressionHandler.Register(server.API())

	filterHandler := handlers.NewFilterHandler(filterRepo)
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

	logoHandler.Register(server.API())

	relayProfileHandler := handlers.NewRelayProfileHandler(relayService)
	relayProfileHandler.Register(server.API())

	relayProfileMappingHandler := handlers.NewRelayProfileMappingHandler(relayProfileMappingService)
	relayProfileMappingHandler.Register(server.API())

	relayStreamHandler := handlers.NewRelayStreamHandler(relayService).
		WithLogger(logger).
		WithProfileMappingService(relayProfileMappingService)
	relayStreamHandler.Register(server.API())
	relayStreamHandler.RegisterChiRoutes(server.Router())

	channelHandler := handlers.NewChannelHandler(db, relayService).WithLogger(logger)
	channelHandler.Register(server.API())
	channelHandler.RegisterChiRoutes(server.Router())

	epgHandler := handlers.NewEpgHandler(db)
	epgHandler.Register(server.API())

	circuitBreakerHandler := handlers.NewCircuitBreakerHandler(httpclient.DefaultManager)
	circuitBreakerHandler.Register(server.API())

	logsHandler := handlers.NewLogsHandler(logsService)
	logsHandler.Register(server.API())
	logsHandler.RegisterSSE(server.Router())
	logger.Debug("http handlers registered")

	// Register job handler
	jobHandler := handlers.NewJobHandler(jobService)
	jobHandler.Register(server.API())

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

	// Start job runner
	if err := runner.Start(ctx); err != nil {
		return fmt.Errorf("starting runner: %w", err)
	}
	defer runner.Stop()

	// Schedule initial logo maintenance job on startup if configured
	if logoScanSchedule != "" {
		if _, err := sched.ScheduleImmediate(ctx, models.JobTypeLogoCleanup, models.ULID{}, "Logo Maintenance"); err != nil {
			logger.Warn("failed to schedule initial logo maintenance job", slog.Any("error", err))
		}
		logger.Info("logo maintenance configured",
			slog.String("schedule", logoScanSchedule))
	}

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
