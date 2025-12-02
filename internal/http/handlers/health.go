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
	version        string
	startTime      time.Time
	cbManager      *httpclient.CircuitBreakerManager
	db             *gorm.DB
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(version string) *HealthHandler {
	return &HealthHandler{
		version:   version,
		startTime: time.Now(),
		cbManager: httpclient.DefaultManager,
	}
}

// WithCircuitBreakerManager sets a custom circuit breaker manager.
func (h *HealthHandler) WithCircuitBreakerManager(manager *httpclient.CircuitBreakerManager) *HealthHandler {
	h.cbManager = manager
	return h
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
}

// GetHealth returns the health status of the service.
func (h *HealthHandler) GetHealth(ctx context.Context, input *HealthInput) (*HealthOutput, error) {
	now := time.Now()
	uptime := now.Sub(h.startTime)

	// Get CPU info
	cpuInfo := h.getCPUInfo()

	// Get memory info
	memInfo := h.getMemoryInfo()

	// Get circuit breaker statuses
	var circuitBreakers []CircuitBreakerStatus
	if h.cbManager != nil {
		stats := h.cbManager.GetAllStats()
		circuitBreakers = make([]CircuitBreakerStatus, 0, len(stats))
		for name, s := range stats {
			circuitBreakers = append(circuitBreakers, CircuitBreakerStatus{
				Name:     name,
				State:    s.State.String(),
				Failures: s.Failures,
			})
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
