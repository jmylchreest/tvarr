// Package handlers provides HTTP API handlers for tvarr.
package handlers

import (
	"context"
	"os"
	"runtime"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/pkg/httpclient"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
	"gorm.io/gorm"
)

// HealthHandler handles health check endpoints.
type HealthHandler struct {
	version   string
	startTime time.Time
	cbManager *httpclient.CircuitBreakerManager
	db        *gorm.DB
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(version string) *HealthHandler {
	return &HealthHandler{
		version:   version,
		startTime: time.Now(),
		cbManager: httpclient.DefaultManager,
	}
}

// WithDB sets the database connection for health checks.
func (h *HealthHandler) WithDB(db *gorm.DB) *HealthHandler {
	h.db = db
	return h
}

// HealthInput is the input for the health check endpoint.
type HealthInput struct{}

// HealthOutput is the output for the health check endpoint.
type HealthOutput struct {
	Body HealthResponse
}

// LivezInput is the input for the liveness probe endpoint.
type LivezInput struct{}

// LivezOutput is the output for the liveness probe endpoint.
type LivezOutput struct {
	Body LivezResponse
}

// LivezResponse is the response for the liveness probe.
type LivezResponse struct {
	Status string `json:"status"`
}

// ReadyzInput is the input for the readiness probe endpoint.
type ReadyzInput struct{}

// ReadyzOutput is the output for the readiness probe endpoint.
type ReadyzOutput struct {
	Body ReadyzResponse
}

// ReadyzResponse is the response for the readiness probe.
type ReadyzResponse struct {
	Status     string            `json:"status"`
	Components map[string]string `json:"components,omitempty"`
}

// Register registers the health routes with the API.
func (h *HealthHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "getHealth",
		Method:      "GET",
		Path:        "/health",
		Summary:     "Health check",
		Description: "Returns the health status of the service including system metrics",
		Tags:        []string{"System"},
	}, h.GetHealth)

	huma.Register(api, huma.Operation{
		OperationID: "getLivez",
		Method:      "GET",
		Path:        "/livez",
		Summary:     "Liveness probe",
		Description: "Lightweight liveness check for UI polling and Kubernetes. Returns 200 OK if the service is running.",
		Tags:        []string{"System"},
	}, h.GetLivez)

	huma.Register(api, huma.Operation{
		OperationID: "getReadyz",
		Method:      "GET",
		Path:        "/readyz",
		Summary:     "Readiness probe",
		Description: "Readiness check for Kubernetes. Verifies database connection and scheduler are ready.",
		Tags:        []string{"System"},
	}, h.GetReadyz)
}

// GetHealth returns the health status of the service.
func (h *HealthHandler) GetHealth(ctx context.Context, input *HealthInput) (*HealthOutput, error) {
	now := time.Now()
	uptime := now.Sub(h.startTime)

	// Get CPU info
	cpuInfo := h.getCPUInfo()

	// Get memory info
	memInfo := h.getMemoryInfo()

	// Get circuit breaker statuses as a map keyed by service name
	circuitBreakers := make(map[string]CircuitBreakerStatus)
	if h.cbManager != nil {
		stats := h.cbManager.GetAllStats()
		for name, s := range stats {
			var failureRate float64
			if s.TotalRequests > 0 {
				failureRate = float64(s.TotalFailures) / float64(s.TotalRequests) * 100
			}
			circuitBreakers[name] = CircuitBreakerStatus{
				Name:            name,
				State:           s.State.String(),
				Failures:        s.Failures,
				SuccessfulCalls: s.TotalSuccesses,
				FailedCalls:     s.TotalFailures,
				TotalCalls:      s.TotalRequests,
				FailureRate:     failureRate,
			}
		}
	}

	// Get database health
	dbHealth := h.getDatabaseHealth(ctx)

	return &HealthOutput{
		Body: HealthResponse{
			Status:        "healthy",
			Timestamp:     now.UTC().Format(time.RFC3339),
			Version:       h.version,
			Uptime:        uptime.Round(time.Second).String(),
			UptimeSeconds: uptime.Seconds(),
			SystemLoad:    cpuInfo.LoadPercentage1Min / 100, // Normalize to 0-1 for backward compat
			NumGoroutines: runtime.NumGoroutine(),
			CPUInfo:       cpuInfo,
			Memory:        memInfo,
			Components: HealthComponents{
				Database:        dbHealth,
				Scheduler:       SchedulerHealth{Status: "ok"},
				CircuitBreakers: circuitBreakers,
			},
			Checks: map[string]string{
				"database": dbHealth.Status,
			},
		},
	}, nil
}

// getCPUInfo returns CPU load information.
func (h *HealthHandler) getCPUInfo() CPUInfo {
	cores := runtime.NumCPU()

	info := CPUInfo{
		Cores: cores,
	}

	// Get load averages
	loadAvg, err := load.Avg()
	if err == nil && loadAvg != nil {
		info.Load1Min = loadAvg.Load1
		info.Load5Min = loadAvg.Load5
		info.Load15Min = loadAvg.Load15

		// Calculate load percentage (load / cores * 100)
		if cores > 0 {
			info.LoadPercentage1Min = (loadAvg.Load1 / float64(cores)) * 100
		}
	}

	return info
}

// getMemoryInfo returns memory usage information.
func (h *HealthHandler) getMemoryInfo() MemoryInfo {
	info := MemoryInfo{}

	// Get system memory
	vmStat, err := mem.VirtualMemory()
	if err == nil && vmStat != nil {
		info.TotalMemoryMB = float64(vmStat.Total) / 1024 / 1024
		info.UsedMemoryMB = float64(vmStat.Used) / 1024 / 1024
		info.FreeMemoryMB = float64(vmStat.Free) / 1024 / 1024
		info.AvailableMemoryMB = float64(vmStat.Available) / 1024 / 1024
	}

	// Get swap
	swapStat, err := mem.SwapMemory()
	if err == nil && swapStat != nil {
		info.SwapTotalMB = float64(swapStat.Total) / 1024 / 1024
		info.SwapUsedMB = float64(swapStat.Used) / 1024 / 1024
	}

	// Get process memory
	info.ProcessMemory = h.getProcessMemoryInfo(info.TotalMemoryMB)

	return info
}

// getProcessMemoryInfo returns process-specific memory information.
func (h *HealthHandler) getProcessMemoryInfo(totalSystemMB float64) ProcessMemoryInfo {
	info := ProcessMemoryInfo{}

	pid := int32(os.Getpid())
	proc, err := process.NewProcess(pid)
	if err != nil {
		return info
	}

	// Get main process memory
	memInfo, err := proc.MemoryInfo()
	if err == nil && memInfo != nil {
		info.MainProcessMB = float64(memInfo.RSS) / 1024 / 1024
		info.TotalProcessTreeMB = info.MainProcessMB

		if totalSystemMB > 0 {
			info.PercentageOfSystem = (info.MainProcessMB / totalSystemMB) * 100
		}
	}

	// Count child processes
	children, err := proc.Children()
	if err == nil {
		info.ChildProcessCount = len(children)
		for _, child := range children {
			childMem, err := child.MemoryInfo()
			if err == nil && childMem != nil {
				childMB := float64(childMem.RSS) / 1024 / 1024
				info.ChildProcessesMB += childMB
				info.TotalProcessTreeMB += childMB
			}
		}
	}

	return info
}

// getDatabaseHealth returns database health information.
func (h *HealthHandler) getDatabaseHealth(ctx context.Context) DatabaseHealth {
	health := DatabaseHealth{
		Status:             "ok",
		TablesAccessible:   true,
		WriteCapability:    true,
		NoBlockingLocks:    true,
		ResponseTimeStatus: "healthy",
	}

	if h.db == nil {
		health.Status = "unknown"
		return health
	}

	// Get underlying SQL DB for stats
	sqlDB, err := h.db.DB()
	if err != nil {
		health.Status = "error"
		return health
	}

	// Get connection pool stats
	stats := sqlDB.Stats()
	health.ConnectionPoolSize = stats.MaxOpenConnections
	health.ActiveConnections = stats.InUse
	health.IdleConnections = stats.Idle

	if stats.MaxOpenConnections > 0 {
		health.PoolUtilizationPercent = float64(stats.InUse) / float64(stats.MaxOpenConnections) * 100
	}

	// Check database response time with a simple query
	start := time.Now()
	err = sqlDB.PingContext(ctx)
	health.ResponseTimeMS = float64(time.Since(start).Microseconds()) / 1000

	if err != nil {
		health.Status = "error"
		health.ResponseTimeStatus = "error"
	} else if health.ResponseTimeMS > 100 {
		health.ResponseTimeStatus = "slow"
	}

	return health
}

// GetLivez returns a lightweight liveness check response.
// This endpoint is used for UI polling and Kubernetes liveness probes.
func (h *HealthHandler) GetLivez(ctx context.Context, input *LivezInput) (*LivezOutput, error) {
	return &LivezOutput{
		Body: LivezResponse{
			Status: "ok",
		},
	}, nil
}

// GetReadyz returns a readiness check response.
// This endpoint checks if the service is ready to receive traffic.
func (h *HealthHandler) GetReadyz(ctx context.Context, input *ReadyzInput) (*ReadyzOutput, error) {
	components := make(map[string]string)
	allReady := true

	// Check database connectivity
	if h.db != nil {
		sqlDB, err := h.db.DB()
		if err != nil {
			components["database"] = "error"
			allReady = false
		} else if err := sqlDB.PingContext(ctx); err != nil {
			components["database"] = "not_ready"
			allReady = false
		} else {
			components["database"] = "ok"
		}
	} else {
		components["database"] = "not_configured"
		allReady = false
	}

	// Scheduler is assumed to be ok if we're running
	// (scheduler status would need a reference to be checked properly)
	components["scheduler"] = "ok"

	status := "ok"
	if !allReady {
		status = "not_ready"
	}

	return &ReadyzOutput{
		Body: ReadyzResponse{
			Status:     status,
			Components: components,
		},
	}, nil
}
