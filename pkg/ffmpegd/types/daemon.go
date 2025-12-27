// Package types defines shared types for the tvarr-ffmpegd distributed transcoding system.
package types

import (
	"time"
)

// DaemonID is a unique identifier for a daemon instance.
type DaemonID string

// String implements fmt.Stringer.
func (d DaemonID) String() string {
	return string(d)
}

// DaemonState represents the connection state of a daemon.
type DaemonState int

const (
	DaemonStateConnecting DaemonState = iota
	DaemonStateConnected              // Successfully registered and ready for jobs
	DaemonStateDraining               // Not accepting new jobs, finishing existing
	DaemonStateUnhealthy              // Missed heartbeats
	DaemonStateDisconnected
)

// String returns a human-readable state name.
func (s DaemonState) String() string {
	switch s {
	case DaemonStateConnecting:
		return "connecting"
	case DaemonStateConnected:
		return "connected"
	case DaemonStateDraining:
		return "draining"
	case DaemonStateUnhealthy:
		return "unhealthy"
	case DaemonStateDisconnected:
		return "disconnected"
	default:
		return "unknown"
	}
}

// Daemon represents a registered transcoding worker.
type Daemon struct {
	ID           DaemonID      `json:"id"`
	Name         string        `json:"name"`
	Version      string        `json:"version"`
	Address      string        `json:"address"` // gRPC address
	State        DaemonState   `json:"state"`
	Capabilities *Capabilities `json:"capabilities"`

	// Health tracking
	ConnectedAt      time.Time `json:"connected_at"`
	LastHeartbeat    time.Time `json:"last_heartbeat"`
	HeartbeatsMissed int       `json:"heartbeats_missed"`

	// Load tracking
	ActiveJobs         int    `json:"active_jobs"`
	TotalJobsCompleted uint64 `json:"total_jobs_completed"`
	TotalJobsFailed    uint64 `json:"total_jobs_failed"`

	// Latest system stats (from heartbeat)
	SystemStats *SystemStats `json:"system_stats,omitempty"`
}

// IsHealthy returns true if the daemon is in a healthy state.
func (d *Daemon) IsHealthy() bool {
	return d.State == DaemonStateConnected
}

// CanAcceptJobs returns true if the daemon can accept new jobs.
func (d *Daemon) CanAcceptJobs() bool {
	if d.State != DaemonStateConnected {
		return false
	}
	if d.Capabilities == nil {
		return false
	}
	return d.ActiveJobs < d.Capabilities.MaxConcurrentJobs
}

// HasAvailableGPUSessions returns true if any GPU has available encode sessions.
// GPUs with MaxEncodeSessions == 0 are considered to have unlimited sessions.
func (d *Daemon) HasAvailableGPUSessions() bool {
	if d.Capabilities == nil {
		return false
	}
	for _, gpu := range d.Capabilities.GPUs {
		// MaxEncodeSessions == 0 means unlimited
		if gpu.MaxEncodeSessions == 0 {
			return true
		}
		if gpu.ActiveEncodeSessions < gpu.MaxEncodeSessions {
			return true
		}
	}
	return false
}

// GPUSessionAvailability returns the total available and maximum GPU encode sessions.
// For GPUs with MaxEncodeSessions == 0 (unlimited), returns a large number to indicate availability.
func (d *Daemon) GPUSessionAvailability() (available, total int) {
	if d.Capabilities == nil {
		return 0, 0
	}
	const unlimitedValue = 1000 // Arbitrary large number for unlimited GPUs
	for _, gpu := range d.Capabilities.GPUs {
		if gpu.MaxEncodeSessions == 0 {
			// Unlimited sessions
			total += unlimitedValue
			available += unlimitedValue
		} else {
			total += gpu.MaxEncodeSessions
			available += gpu.MaxEncodeSessions - gpu.ActiveEncodeSessions
		}
	}
	return available, total
}
