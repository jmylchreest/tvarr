package service

import (
	"fmt"
	"log/slog"

	"github.com/jmylchreest/tvarr/internal/relay"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
)

// FFmpegDService provides high-level operations for managing ffmpegd daemons.
// T086: Create internal/service/ffmpegd_service.go with FFmpegDService
type FFmpegDService struct {
	registry *relay.DaemonRegistry
	logger   *slog.Logger
}

// NewFFmpegDService creates a new FFmpegDService.
func NewFFmpegDService(registry *relay.DaemonRegistry, logger *slog.Logger) *FFmpegDService {
	if logger == nil {
		logger = slog.Default()
	}
	return &FFmpegDService{
		registry: registry,
		logger:   logger,
	}
}

// ClusterStats contains aggregate statistics about the daemon cluster.
type ClusterStats struct {
	TotalDaemons         int     `json:"total_daemons"`
	ActiveDaemons        int     `json:"active_daemons"`
	UnhealthyDaemons     int     `json:"unhealthy_daemons"`
	DrainingDaemons      int     `json:"draining_daemons"`
	DisconnectedDaemons  int     `json:"disconnected_daemons"`
	TotalActiveJobs      int     `json:"total_active_jobs"`
	TotalGPUs            int     `json:"total_gpus"`
	AvailableGPUSessions int     `json:"available_gpu_sessions"`
	TotalGPUSessions     int     `json:"total_gpu_sessions"`
	AverageCPUPercent    float64 `json:"average_cpu_percent"`
	AverageMemPercent    float64 `json:"average_memory_percent"`
}

// ListDaemons returns all registered daemons.
// T087: Implement ListDaemons() - returns all registered daemons with stats
func (s *FFmpegDService) ListDaemons() []*types.Daemon {
	return s.registry.GetAll()
}

// GetDaemon returns a single daemon by ID.
// T088: Implement GetDaemon() - returns single daemon details
func (s *FFmpegDService) GetDaemon(id types.DaemonID) (*types.Daemon, error) {
	daemon, ok := s.registry.Get(id)
	if !ok {
		return nil, fmt.Errorf("daemon not found: %s", id)
	}
	return daemon, nil
}

// GetDaemonJobs returns active jobs on a daemon.
// T089: Implement GetDaemonJobs() - returns active jobs on daemon
// Note: This returns the job count since full job details are tracked elsewhere
func (s *FFmpegDService) GetDaemonJobs(id types.DaemonID) (int, error) {
	daemon, ok := s.registry.Get(id)
	if !ok {
		return 0, fmt.Errorf("daemon not found: %s", id)
	}
	return daemon.ActiveJobs, nil
}

// DrainDaemon puts a daemon into draining state (no new jobs, finish existing).
// T090: Implement DrainDaemon() - stop new jobs, wait for existing to complete
func (s *FFmpegDService) DrainDaemon(id types.DaemonID) error {
	daemon, ok := s.registry.Get(id)
	if !ok {
		return fmt.Errorf("daemon not found: %s", id)
	}

	if daemon.State == types.DaemonStateDraining {
		return nil // Already draining
	}

	if daemon.State == types.DaemonStateDisconnected {
		return fmt.Errorf("cannot drain disconnected daemon: %s", id)
	}

	daemon.State = types.DaemonStateDraining

	s.logger.Info("daemon set to draining",
		slog.String("daemon_id", string(id)),
		slog.Int("active_jobs", daemon.ActiveJobs),
	)

	return nil
}

// ActivateDaemon puts a draining daemon back into active state.
func (s *FFmpegDService) ActivateDaemon(id types.DaemonID) error {
	daemon, ok := s.registry.Get(id)
	if !ok {
		return fmt.Errorf("daemon not found: %s", id)
	}

	if daemon.State == types.DaemonStateDisconnected {
		return fmt.Errorf("cannot activate disconnected daemon: %s", id)
	}

	if daemon.State == types.DaemonStateUnhealthy {
		return fmt.Errorf("cannot activate unhealthy daemon: %s", id)
	}

	daemon.State = types.DaemonStateConnected

	s.logger.Info("daemon activated",
		slog.String("daemon_id", string(id)),
	)

	return nil
}

// GetClusterStats returns aggregate statistics about the cluster.
func (s *FFmpegDService) GetClusterStats() ClusterStats {
	daemons := s.registry.GetAll()

	stats := ClusterStats{}

	var cpuTotal, memTotal float64
	var cpuCount, memCount int

	for _, d := range daemons {
		stats.TotalDaemons++

		switch d.State {
		case types.DaemonStateConnected:
			stats.ActiveDaemons++
		case types.DaemonStateUnhealthy:
			stats.UnhealthyDaemons++
		case types.DaemonStateDraining:
			stats.DrainingDaemons++
		case types.DaemonStateDisconnected:
			stats.DisconnectedDaemons++
		}

		stats.TotalActiveJobs += d.ActiveJobs

		// Count GPUs and sessions
		if d.Capabilities != nil {
			for _, gpu := range d.Capabilities.GPUs {
				stats.TotalGPUs++
				if gpu.MaxEncodeSessions > 0 {
					stats.TotalGPUSessions += gpu.MaxEncodeSessions
					stats.AvailableGPUSessions += gpu.MaxEncodeSessions - gpu.ActiveEncodeSessions
				} else {
					// Unlimited GPU
					stats.TotalGPUSessions += 1000
					stats.AvailableGPUSessions += 1000
				}
			}
		}

		// Aggregate system stats
		if d.SystemStats != nil {
			cpuTotal += d.SystemStats.CPUPercent
			cpuCount++
			memTotal += d.SystemStats.MemoryPercent
			memCount++
		}
	}

	// Calculate averages
	if cpuCount > 0 {
		stats.AverageCPUPercent = cpuTotal / float64(cpuCount)
	}
	if memCount > 0 {
		stats.AverageMemPercent = memTotal / float64(memCount)
	}

	return stats
}

// GetDaemonsByCapability returns daemons that have a specific encoder.
func (s *FFmpegDService) GetDaemonsByCapability(encoder string) []*types.Daemon {
	return s.registry.GetWithCapability(encoder)
}

// GetDaemonsByState returns daemons in a specific state.
func (s *FFmpegDService) GetDaemonsByState(state types.DaemonState) []*types.Daemon {
	all := s.registry.GetAll()
	var result []*types.Daemon
	for _, d := range all {
		if d.State == state {
			result = append(result, d)
		}
	}
	return result
}

// GetActiveDaemons returns only active daemons.
func (s *FFmpegDService) GetActiveDaemons() []*types.Daemon {
	return s.registry.GetActive()
}

// GetAvailableDaemons returns daemons that can accept new jobs.
func (s *FFmpegDService) GetAvailableDaemons() []*types.Daemon {
	return s.registry.GetAvailable()
}

// GetDaemonsWithAvailableGPU returns daemons with available GPU encode sessions.
func (s *FFmpegDService) GetDaemonsWithAvailableGPU() []*types.Daemon {
	return s.registry.GetWithAvailableGPU()
}

// UnregisterDaemon removes a daemon from the registry.
func (s *FFmpegDService) UnregisterDaemon(id types.DaemonID, reason string) {
	s.registry.Unregister(id, reason)
}

// DaemonCount returns the total number of registered daemons.
func (s *FFmpegDService) DaemonCount() int {
	return s.registry.Count()
}

// ActiveDaemonCount returns the number of active daemons.
func (s *FFmpegDService) ActiveDaemonCount() int {
	return s.registry.CountActive()
}
