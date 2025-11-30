package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	internalhttp "github.com/jmylchreest/tvarr/internal/http"
	"github.com/jmylchreest/tvarr/internal/http/handlers"
	"github.com/jmylchreest/tvarr/pkg/httpclient"
	"github.com/jmylchreest/tvarr/internal/ingestor"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/pipeline"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/jmylchreest/tvarr/internal/service"
	"github.com/jmylchreest/tvarr/internal/service/progress"
	"github.com/jmylchreest/tvarr/internal/storage"
	"github.com/jmylchreest/tvarr/internal/version"
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
	serveCmd.Flags().String("database", "tvarr.db", "Database file path")
	serveCmd.Flags().String("data-dir", "data", "Data directory for output files")

	// Pipeline flags
	serveCmd.Flags().Bool("ingestion-guard", true, "Enable ingestion guard (waits for active ingestions)")

	// Bind flags to viper
	viper.BindPFlag("server.host", serveCmd.Flags().Lookup("host"))
	viper.BindPFlag("server.port", serveCmd.Flags().Lookup("port"))
	viper.BindPFlag("database.path", serveCmd.Flags().Lookup("database"))
	viper.BindPFlag("storage.data_dir", serveCmd.Flags().Lookup("data-dir"))
	viper.BindPFlag("pipeline.ingestion_guard", serveCmd.Flags().Lookup("ingestion-guard"))
}

func runServe(cmd *cobra.Command, args []string) error {
	logger := slog.Default()

	// Initialize database
	db, err := initDatabase(viper.GetString("database.path"))
	if err != nil {
		return fmt.Errorf("initializing database: %w", err)
	}

	// Run migrations
	if err := runMigrations(db); err != nil {
		return fmt.Errorf("running migrations: %w", err)
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

	// Initialize storage sandbox
	sandbox, err := storage.NewSandbox(viper.GetString("storage.data_dir"))
	if err != nil {
		return fmt.Errorf("initializing storage: %w", err)
	}

	// Initialize logo cache and service
	logoCache, err := storage.NewLogoCache(viper.GetString("storage.data_dir"))
	if err != nil {
		return fmt.Errorf("initializing logo cache: %w", err)
	}

	// Create HTTP client for logo fetching with 200 and 404 as acceptable statuses
	// (missing logos are expected and shouldn't trip the circuit breaker)
	logoHTTPConfig := httpclient.DefaultConfig()
	logoHTTPConfig.AcceptableStatusCodes = httpclient.StatusCodesFromSlice([]int{http.StatusOK, http.StatusNotFound})
	logoHTTPConfig.Logger = logger
	logoHTTPClient := httpclient.New(logoHTTPConfig)

	// Register HTTP client for health monitoring
	httpclient.DefaultRegistry.Register("logo-fetcher", logoHTTPClient)

	logoService := service.NewLogoService(logoCache).
		WithHTTPClient(logoHTTPClient.StandardClient()).
		WithLogger(logger)

	// Load logo index with pruning of stale cached logos
	logoRetention := viper.GetDuration("storage.logo_retention")
	if logoRetention > 0 {
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
				slog.Duration("retention", logoRetention))
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

	pipelineFactory := pipeline.NewDefaultFactory(
		channelRepo,
		epgProgramRepo,
		filterRepo,
		dataMappingRuleRepo,
		sandbox,
		logger,
		logoService, // Logo caching enabled
		ingestionGuardStateManager,
	)

	// Initialize progress service
	progressService := progress.NewService(logger)
	progressService.Start()
	defer progressService.Stop()

	// Initialize services
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

	// Initialize HTTP server
	serverConfig := internalhttp.ServerConfig{
		Host: viper.GetString("server.host"),
		Port: viper.GetInt("server.port"),
	}
	server := internalhttp.NewServer(serverConfig, logger, version.Version)

	// Register OpenAPI docs handler with system theme detection (dark/light)
	docsHandler := handlers.NewDocsHandler("tvarr API", "/openapi.yaml", handlers.WithSystemTheme())
	server.Router().Get("/docs", docsHandler.ServeHTTP)

	// Register static handler for embedded frontend (must be first to catch-all non-API routes)
	staticHandler := handlers.NewStaticHandler()
	server.Router().Mount("/", staticHandler)

	// Register handlers
	healthHandler := handlers.NewHealthHandler(version.Version)
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

	// Start server
	logger.Info("starting tvarr server",
		slog.String("host", serverConfig.Host),
		slog.Int("port", serverConfig.Port),
		slog.String("version", version.Version),
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

func runMigrations(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.StreamSource{},
		&models.Channel{},
		&models.EpgSource{},
		&models.EpgProgram{},
		&models.StreamProxy{},
		&models.ProxySource{},
		&models.ProxyEpgSource{},
		&models.ManualStreamChannel{},
		&models.Filter{},
		&models.DataMappingRule{},
		&models.Job{},
		&models.JobHistory{},
		&models.RelayProfile{},
		&models.LastKnownCodec{},
	)
}
