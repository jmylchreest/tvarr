package relay

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jmylchreest/tvarr/internal/observability"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
)

// DaemonRegistry manages registered ffmpegd daemons.
type DaemonRegistry struct {
	logger *slog.Logger

	mu      sync.RWMutex
	daemons map[types.DaemonID]*types.Daemon

	// Configuration
	heartbeatTimeout time.Duration // Time after which daemon is marked unhealthy
	removeTimeout    time.Duration // Time after which unhealthy daemon is removed
	cleanupInterval  time.Duration // Interval for checking daemon health
	cleanupCancel    context.CancelFunc
}

// NewDaemonRegistry creates a new daemon registry.
func NewDaemonRegistry(logger *slog.Logger) *DaemonRegistry {
	return &DaemonRegistry{
		logger:           logger,
		daemons:          make(map[types.DaemonID]*types.Daemon),
		heartbeatTimeout: 15 * time.Second, // 3 missed heartbeats at 5s interval
		removeTimeout:    30 * time.Second, // Remove after 30s of no heartbeats
		cleanupInterval:  5 * time.Second,  // Check every 5 seconds
	}
}

// WithHeartbeatTimeout sets the heartbeat timeout duration.
func (r *DaemonRegistry) WithHeartbeatTimeout(timeout time.Duration) *DaemonRegistry {
	r.heartbeatTimeout = timeout
	return r
}

// WithRemoveTimeout sets the duration after which unhealthy daemons are removed.
func (r *DaemonRegistry) WithRemoveTimeout(timeout time.Duration) *DaemonRegistry {
	r.removeTimeout = timeout
	return r
}

// Start starts the daemon registry cleanup goroutine.
func (r *DaemonRegistry) Start(ctx context.Context) {
	cleanupCtx, cancel := context.WithCancel(ctx)
	r.cleanupCancel = cancel

	go r.cleanupLoop(cleanupCtx)
}

// Stop stops the daemon registry cleanup goroutine.
func (r *DaemonRegistry) Stop() {
	if r.cleanupCancel != nil {
		r.cleanupCancel()
	}
}

// Register adds or updates a daemon in the registry.
func (r *DaemonRegistry) Register(req *proto.RegisterRequest) (*types.Daemon, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	daemonID := types.DaemonID(req.DaemonId)
	now := time.Now()

	// Check if daemon already exists
	if existing, ok := r.daemons[daemonID]; ok {
		// Update existing daemon
		existing.Name = req.DaemonName
		existing.Version = req.Version
		existing.State = types.DaemonStateConnected
		existing.Capabilities = convertProtoCapabilities(req.Capabilities)
		existing.LastHeartbeat = now
		existing.HeartbeatsMissed = 0

		r.logger.Info("daemon re-registered",
			slog.String("daemon_id", string(daemonID)),
			slog.String("name", req.DaemonName),
		)

		return existing, nil
	}

	// Create new daemon
	daemon := &types.Daemon{
		ID:            daemonID,
		Name:          req.DaemonName,
		Version:       req.Version,
		State:         types.DaemonStateConnected,
		Capabilities:  convertProtoCapabilities(req.Capabilities),
		ConnectedAt:   now,
		LastHeartbeat: now,
	}

	r.daemons[daemonID] = daemon

	r.logger.Info("daemon registered",
		slog.String("daemon_id", string(daemonID)),
		slog.String("name", req.DaemonName),
		slog.String("version", req.Version),
		slog.Int("max_jobs", daemon.Capabilities.MaxConcurrentJobs),
		slog.Int("gpus", len(daemon.Capabilities.GPUs)),
	)

	return daemon, nil
}

// HandleHeartbeat processes a heartbeat from a daemon.
func (r *DaemonRegistry) HandleHeartbeat(req *proto.HeartbeatRequest) (*types.Daemon, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	daemonID := types.DaemonID(req.DaemonId)
	daemon, ok := r.daemons[daemonID]
	if !ok {
		return nil, fmt.Errorf("daemon not registered: %s", daemonID)
	}

	// Update heartbeat time
	daemon.LastHeartbeat = time.Now()
	daemon.HeartbeatsMissed = 0

	// Update state if was unhealthy
	if daemon.State == types.DaemonStateUnhealthy {
		daemon.State = types.DaemonStateConnected
		r.logger.Info("daemon recovered",
			slog.String("daemon_id", string(daemonID)),
		)
	}

	// Update system stats
	if req.SystemStats != nil {
		daemon.SystemStats = convertProtoSystemStats(req.SystemStats)
	}

	// Update active jobs count
	daemon.ActiveJobs = len(req.ActiveJobs)

	// Update GPU session counts from system stats
	if daemon.SystemStats != nil {
		for i, gpuStats := range daemon.SystemStats.GPUs {
			if i < len(daemon.Capabilities.GPUs) {
				daemon.Capabilities.GPUs[i].ActiveEncodeSessions = gpuStats.ActiveEncodeSessions
				daemon.Capabilities.GPUs[i].ActiveDecodeSessions = gpuStats.ActiveDecodeSessions
			}
		}
	}

	r.logger.Log(context.Background(), observability.LevelTrace, "heartbeat received",
		slog.String("daemon_id", string(daemonID)),
		slog.Int("active_jobs", daemon.ActiveJobs),
	)

	return daemon, nil
}

// Unregister removes a daemon from the registry.
func (r *DaemonRegistry) Unregister(daemonID types.DaemonID, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if daemon, ok := r.daemons[daemonID]; ok {
		daemon.State = types.DaemonStateDisconnected

		r.logger.Info("daemon unregistered",
			slog.String("daemon_id", string(daemonID)),
			slog.String("reason", reason),
		)

		delete(r.daemons, daemonID)
	}
}

// Get returns a daemon by ID.
func (r *DaemonRegistry) Get(daemonID types.DaemonID) (*types.Daemon, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	daemon, ok := r.daemons[daemonID]
	return daemon, ok
}

// GetAll returns all registered daemons.
func (r *DaemonRegistry) GetAll() []*types.Daemon {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*types.Daemon, 0, len(r.daemons))
	for _, daemon := range r.daemons {
		result = append(result, daemon)
	}
	return result
}

// GetActive returns all active daemons.
func (r *DaemonRegistry) GetActive() []*types.Daemon {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*types.Daemon
	for _, daemon := range r.daemons {
		if daemon.State == types.DaemonStateConnected {
			result = append(result, daemon)
		}
	}
	return result
}

// GetAvailable returns daemons that can accept new jobs.
func (r *DaemonRegistry) GetAvailable() []*types.Daemon {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*types.Daemon
	for _, daemon := range r.daemons {
		if daemon.CanAcceptJobs() {
			result = append(result, daemon)
		}
	}
	return result
}

// GetWithCapability returns daemons that have a specific encoder.
func (r *DaemonRegistry) GetWithCapability(encoder string) []*types.Daemon {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*types.Daemon
	for _, daemon := range r.daemons {
		if daemon.State == types.DaemonStateConnected && daemon.Capabilities != nil {
			if daemon.Capabilities.HasEncoder(encoder) {
				result = append(result, daemon)
			}
		}
	}
	return result
}

// GetWithAvailableGPU returns daemons with available GPU encode sessions.
func (r *DaemonRegistry) GetWithAvailableGPU() []*types.Daemon {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*types.Daemon
	for _, daemon := range r.daemons {
		if daemon.CanAcceptJobs() && daemon.HasAvailableGPUSessions() {
			result = append(result, daemon)
		}
	}
	return result
}

// SelectLeastLoaded returns the daemon with the lowest load that can accept jobs.
func (r *DaemonRegistry) SelectLeastLoaded() *types.Daemon {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var selected *types.Daemon
	lowestLoad := float64(1.0) // Max load percentage

	for _, daemon := range r.daemons {
		if !daemon.CanAcceptJobs() {
			continue
		}

		load := float64(daemon.ActiveJobs) / float64(daemon.Capabilities.MaxConcurrentJobs)
		if load < lowestLoad {
			lowestLoad = load
			selected = daemon
		}
	}

	return selected
}

// SelectForEncoder returns the best daemon for a specific encoder.
func (r *DaemonRegistry) SelectForEncoder(encoder string) *types.Daemon {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var selected *types.Daemon
	lowestLoad := float64(1.0)

	for _, daemon := range r.daemons {
		if !daemon.CanAcceptJobs() {
			continue
		}

		if daemon.Capabilities == nil || !daemon.Capabilities.HasEncoder(encoder) {
			continue
		}

		load := float64(daemon.ActiveJobs) / float64(daemon.Capabilities.MaxConcurrentJobs)
		if load < lowestLoad {
			lowestLoad = load
			selected = daemon
		}
	}

	return selected
}

// SelectDaemon selects the best daemon using the given selection strategy.
// This is the preferred method for daemon selection as it supports composable strategies.
func (r *DaemonRegistry) SelectDaemon(strategy SelectionStrategy, criteria SelectionCriteria) *types.Daemon {
	available := r.GetAvailable()
	if len(available) == 0 {
		return nil
	}
	return strategy.Select(available, criteria)
}

// Count returns the number of registered daemons.
func (r *DaemonRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.daemons)
}

// CountActive returns the number of active daemons.
func (r *DaemonRegistry) CountActive() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, daemon := range r.daemons {
		if daemon.State == types.DaemonStateConnected {
			count++
		}
	}
	return count
}

// cleanupLoop periodically checks for unhealthy/disconnected daemons.
func (r *DaemonRegistry) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(r.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.checkHeartbeats()
		}
	}
}

// checkHeartbeats marks daemons as unhealthy if they've missed heartbeats,
// and removes them if they've been unhealthy for too long.
func (r *DaemonRegistry) checkHeartbeats() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	var toRemove []types.DaemonID

	for daemonID, daemon := range r.daemons {
		if daemon.State == types.DaemonStateDisconnected {
			// Already disconnected, remove immediately
			toRemove = append(toRemove, daemonID)
			continue
		}

		timeSinceHeartbeat := now.Sub(daemon.LastHeartbeat)

		// Check if daemon should be removed (no heartbeat for removeTimeout)
		if timeSinceHeartbeat > r.removeTimeout {
			r.logger.Warn("removing stale daemon",
				slog.String("daemon_id", string(daemonID)),
				slog.String("name", daemon.Name),
				slog.Duration("since_heartbeat", timeSinceHeartbeat),
			)
			toRemove = append(toRemove, daemonID)
			continue
		}

		// Check if daemon should be marked unhealthy
		if timeSinceHeartbeat > r.heartbeatTimeout {
			if daemon.State != types.DaemonStateUnhealthy {
				daemon.State = types.DaemonStateUnhealthy
				daemon.HeartbeatsMissed++

				r.logger.Warn("daemon marked unhealthy",
					slog.String("daemon_id", string(daemonID)),
					slog.String("name", daemon.Name),
					slog.Duration("since_heartbeat", timeSinceHeartbeat),
				)
			}
		}
	}

	// Remove stale daemons
	for _, daemonID := range toRemove {
		delete(r.daemons, daemonID)
	}
}

// convertProtoCapabilities converts proto capabilities to types.
func convertProtoCapabilities(caps *proto.Capabilities) *types.Capabilities {
	if caps == nil {
		return nil
	}

	result := &types.Capabilities{
		VideoEncoders:     caps.VideoEncoders,
		VideoDecoders:     caps.VideoDecoders,
		AudioEncoders:     caps.AudioEncoders,
		AudioDecoders:     caps.AudioDecoders,
		MaxConcurrentJobs: int(caps.MaxConcurrentJobs),
	}

	// Convert HW accels
	for _, hw := range caps.HwAccels {
		result.HWAccels = append(result.HWAccels, types.HWAccelInfo{
			Type:      types.HWAccelType(hw.Type),
			Device:    hw.Device,
			Available: hw.Available,
			Encoders:  hw.Encoders,
			Decoders:  hw.Decoders,
		})
	}

	// Convert GPUs
	for _, gpu := range caps.Gpus {
		gpuClass := types.GPUClassUnknown
		switch gpu.GpuClass {
		case proto.GPUClass_GPU_CLASS_CONSUMER:
			gpuClass = types.GPUClassConsumer
		case proto.GPUClass_GPU_CLASS_PROFESSIONAL:
			gpuClass = types.GPUClassProfessional
		case proto.GPUClass_GPU_CLASS_DATACENTER:
			gpuClass = types.GPUClassDatacenter
		case proto.GPUClass_GPU_CLASS_INTEGRATED:
			gpuClass = types.GPUClassIntegrated
		}

		result.GPUs = append(result.GPUs, types.GPUInfo{
			Index:             int(gpu.Index),
			Name:              gpu.Name,
			Class:             gpuClass,
			Driver:            gpu.DriverVersion,
			MaxEncodeSessions: int(gpu.MaxEncodeSessions),
			MaxDecodeSessions: int(gpu.MaxDecodeSessions),
			MemoryTotal:       gpu.MemoryTotalBytes,
		})
	}

	return result
}

// convertProtoSystemStats converts proto system stats to types.
func convertProtoSystemStats(stats *proto.SystemStats) *types.SystemStats {
	if stats == nil {
		return nil
	}

	result := &types.SystemStats{
		Hostname:        stats.Hostname,
		OS:              stats.Os,
		Arch:            stats.Arch,
		Uptime:          time.Duration(stats.UptimeSeconds) * time.Second,
		CPUCores:        int(stats.CpuCores),
		CPUPercent:      stats.CpuPercent,
		CPUPerCore:      stats.CpuPerCore,
		LoadAvg1m:       stats.LoadAvg_1M,
		LoadAvg5m:       stats.LoadAvg_5M,
		LoadAvg15m:      stats.LoadAvg_15M,
		MemoryTotal:     stats.MemoryTotalBytes,
		MemoryUsed:      stats.MemoryUsedBytes,
		MemoryAvailable: stats.MemoryAvailableBytes,
		MemoryPercent:   stats.MemoryPercent,
		SwapTotal:       stats.SwapTotalBytes,
		SwapUsed:        stats.SwapUsedBytes,
		DiskTotal:       stats.DiskTotalBytes,
		DiskUsed:        stats.DiskUsedBytes,
		DiskAvailable:   stats.DiskAvailableBytes,
		DiskPercent:     stats.DiskPercent,
		NetworkBytesSent: stats.NetworkBytesSent,
		NetworkBytesRecv: stats.NetworkBytesRecv,
		NetworkSendRate:  stats.NetworkSendRateBps,
		NetworkRecvRate:  stats.NetworkRecvRateBps,
	}

	// Convert GPU stats
	for _, gpu := range stats.Gpus {
		gpuClass := types.GPUClassUnknown
		switch gpu.GpuClass {
		case proto.GPUClass_GPU_CLASS_CONSUMER:
			gpuClass = types.GPUClassConsumer
		case proto.GPUClass_GPU_CLASS_PROFESSIONAL:
			gpuClass = types.GPUClassProfessional
		case proto.GPUClass_GPU_CLASS_DATACENTER:
			gpuClass = types.GPUClassDatacenter
		case proto.GPUClass_GPU_CLASS_INTEGRATED:
			gpuClass = types.GPUClassIntegrated
		}

		result.GPUs = append(result.GPUs, types.GPUStats{
			Index:                int(gpu.Index),
			Name:                 gpu.Name,
			DriverVersion:        gpu.DriverVersion,
			Utilization:          gpu.UtilizationPercent,
			MemoryTotal:          gpu.MemoryTotalBytes,
			MemoryUsed:           gpu.MemoryUsedBytes,
			MemoryPercent:        gpu.MemoryPercent,
			Temperature:          int(gpu.TemperatureCelsius),
			PowerWatts:           int(gpu.PowerWatts),
			EncoderUtilization:   gpu.EncoderUtilization,
			DecoderUtilization:   gpu.DecoderUtilization,
			MaxEncodeSessions:    int(gpu.MaxEncodeSessions),
			ActiveEncodeSessions: int(gpu.ActiveEncodeSessions),
			MaxDecodeSessions:    int(gpu.MaxDecodeSessions),
			ActiveDecodeSessions: int(gpu.ActiveDecodeSessions),
			Class:                gpuClass,
		})
	}

	// Convert pressure stats
	if stats.CpuPressure != nil {
		result.CPUPressure = &types.PressureStats{
			Avg10:   stats.CpuPressure.Avg10,
			Avg60:   stats.CpuPressure.Avg60,
			Avg300:  stats.CpuPressure.Avg300,
			TotalUs: stats.CpuPressure.TotalUs,
		}
	}
	if stats.MemoryPressure != nil {
		result.MemoryPressure = &types.PressureStats{
			Avg10:   stats.MemoryPressure.Avg10,
			Avg60:   stats.MemoryPressure.Avg60,
			Avg300:  stats.MemoryPressure.Avg300,
			TotalUs: stats.MemoryPressure.TotalUs,
		}
	}
	if stats.IoPressure != nil {
		result.IOPressure = &types.PressureStats{
			Avg10:   stats.IoPressure.Avg10,
			Avg60:   stats.IoPressure.Avg60,
			Avg300:  stats.IoPressure.Avg300,
			TotalUs: stats.IoPressure.TotalUs,
		}
	}

	return result
}
