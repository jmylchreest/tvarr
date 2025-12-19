package types

import (
	"time"
)

// SystemStats contains comprehensive system metrics.
type SystemStats struct {
	// Host identification
	Hostname string        `json:"hostname"`
	OS       string        `json:"os"`   // linux, darwin, windows
	Arch     string        `json:"arch"` // amd64, arm64
	Uptime   time.Duration `json:"uptime"`

	// CPU
	CPUCores   int       `json:"cpu_cores"`
	CPUPercent float64   `json:"cpu_percent"`
	CPUPerCore []float64 `json:"cpu_per_core,omitempty"`
	LoadAvg1m  float64   `json:"load_avg_1m"`
	LoadAvg5m  float64   `json:"load_avg_5m"`
	LoadAvg15m float64   `json:"load_avg_15m"`

	// Memory
	MemoryTotal     uint64  `json:"memory_total"`
	MemoryUsed      uint64  `json:"memory_used"`
	MemoryAvailable uint64  `json:"memory_available"`
	MemoryPercent   float64 `json:"memory_percent"`
	SwapTotal       uint64  `json:"swap_total"`
	SwapUsed        uint64  `json:"swap_used"`

	// Disk (work directory)
	DiskTotal     uint64  `json:"disk_total"`
	DiskUsed      uint64  `json:"disk_used"`
	DiskAvailable uint64  `json:"disk_available"`
	DiskPercent   float64 `json:"disk_percent"`

	// Network
	NetworkBytesSent uint64  `json:"network_bytes_sent"`
	NetworkBytesRecv uint64  `json:"network_bytes_recv"`
	NetworkSendRate  float64 `json:"network_send_rate_bps"`
	NetworkRecvRate  float64 `json:"network_recv_rate_bps"`

	// GPU stats (per GPU)
	GPUs []GPUStats `json:"gpus,omitempty"`

	// Pressure indicators (Linux PSI)
	CPUPressure    *PressureStats `json:"cpu_pressure,omitempty"`
	MemoryPressure *PressureStats `json:"memory_pressure,omitempty"`
	IOPressure     *PressureStats `json:"io_pressure,omitempty"`
}

// GPUStats contains GPU-specific metrics.
type GPUStats struct {
	Index              int     `json:"index"`
	Name               string  `json:"name"`
	DriverVersion      string  `json:"driver_version"`
	Utilization        float64 `json:"utilization"`
	MemoryPercent      float64 `json:"memory_percent"`
	MemoryTotal        uint64  `json:"memory_total"`
	MemoryUsed         uint64  `json:"memory_used"`
	Temperature        int     `json:"temperature"`
	PowerWatts         int     `json:"power_watts"`
	EncoderUtilization float64 `json:"encoder_utilization"`
	DecoderUtilization float64 `json:"decoder_utilization"`

	// Session tracking
	MaxEncodeSessions    int `json:"max_encode_sessions"`
	ActiveEncodeSessions int `json:"active_encode_sessions"`
	MaxDecodeSessions    int `json:"max_decode_sessions"`
	ActiveDecodeSessions int `json:"active_decode_sessions"`

	// GPU class for limit detection
	Class GPUClass `json:"class"`
}

// AvailableEncodeSessions returns the number of available encode sessions.
func (g *GPUStats) AvailableEncodeSessions() int {
	return g.MaxEncodeSessions - g.ActiveEncodeSessions
}

// PressureStats contains Linux PSI metrics.
type PressureStats struct {
	Avg10   float64 `json:"avg10"`    // 10-second average
	Avg60   float64 `json:"avg60"`    // 60-second average
	Avg300  float64 `json:"avg300"`   // 5-minute average
	TotalUs uint64  `json:"total_us"` // Total stall time in microseconds
}

// HasPressure returns true if there is measurable pressure.
func (p *PressureStats) HasPressure() bool {
	return p.Avg10 > 0 || p.Avg60 > 0 || p.Avg300 > 0
}
