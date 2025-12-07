package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ProcessStats contains resource usage statistics for an FFmpeg process.
type ProcessStats struct {
	// Process identification
	PID int `json:"pid"`

	// CPU usage
	CPUPercent     float64       `json:"cpu_percent"`      // Current CPU usage as percentage (0-100 per core)
	CPUUser        time.Duration `json:"cpu_user"`         // Total user CPU time
	CPUSystem      time.Duration `json:"cpu_system"`       // Total system CPU time
	CPUTotal       time.Duration `json:"cpu_total"`        // Total CPU time (user + system)

	// Memory usage
	MemoryRSSBytes uint64  `json:"memory_rss_bytes"` // Resident Set Size in bytes
	MemoryRSSMB    float64 `json:"memory_rss_mb"`    // Resident Set Size in MB
	MemoryVMSBytes uint64  `json:"memory_vms_bytes"` // Virtual Memory Size in bytes
	MemoryPercent  float64 `json:"memory_percent"`   // Memory usage as percentage of total system memory

	// Bandwidth (tracked externally via CountingWriter)
	BytesWritten    uint64  `json:"bytes_written"`     // Total bytes written to output
	BytesRead       uint64  `json:"bytes_read"`        // Total bytes read from input (if tracked)
	WriteRateBps    float64 `json:"write_rate_bps"`    // Current write rate in bytes/sec
	WriteRateKbps   float64 `json:"write_rate_kbps"`   // Current write rate in kbps
	WriteRateMbps   float64 `json:"write_rate_mbps"`   // Current write rate in Mbps

	// Timing
	StartedAt   time.Time     `json:"started_at"`
	Duration    time.Duration `json:"duration"`
	LastUpdated time.Time     `json:"last_updated"`
}

// ProcessMonitor monitors resource usage of an FFmpeg process.
type ProcessMonitor struct {
	pid         int
	startedAt   time.Time
	interval    time.Duration

	mu          sync.RWMutex
	stats       ProcessStats
	running     bool

	// For CPU percentage calculation
	lastCPUTime   time.Duration
	lastCheckTime time.Time

	// For bandwidth rate calculation
	lastBytesWritten uint64
	lastBytesCheck   time.Time

	// External byte counters (set by CountingWriter)
	bytesWritten atomic.Uint64
	bytesRead    atomic.Uint64

	// System info cache
	totalMemory   uint64
	numCPU        int
	clockTicksHz  int64

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewProcessMonitor creates a new process monitor.
func NewProcessMonitor(pid int) *ProcessMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	pm := &ProcessMonitor{
		pid:          pid,
		startedAt:    time.Now(),
		interval:     time.Second,
		numCPU:       runtime.NumCPU(),
		clockTicksHz: getClockTicks(),
		ctx:          ctx,
		cancel:       cancel,
	}

	pm.totalMemory = getTotalMemory()

	return pm
}

// Start begins monitoring the process.
func (pm *ProcessMonitor) Start() {
	pm.mu.Lock()
	if pm.running {
		pm.mu.Unlock()
		return
	}
	pm.running = true
	pm.lastCheckTime = time.Now()
	pm.lastBytesCheck = time.Now()
	pm.mu.Unlock()

	pm.wg.Add(1)
	go pm.monitorLoop()
}

// Stop stops monitoring the process.
func (pm *ProcessMonitor) Stop() {
	pm.cancel()
	pm.wg.Wait()

	pm.mu.Lock()
	pm.running = false
	pm.mu.Unlock()
}

// Stats returns the current process statistics.
func (pm *ProcessMonitor) Stats() ProcessStats {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	stats := pm.stats
	stats.BytesWritten = pm.bytesWritten.Load()
	stats.BytesRead = pm.bytesRead.Load()

	return stats
}

// AddBytesWritten adds to the bytes written counter.
func (pm *ProcessMonitor) AddBytesWritten(n uint64) {
	pm.bytesWritten.Add(n)
}

// AddBytesRead adds to the bytes read counter.
func (pm *ProcessMonitor) AddBytesRead(n uint64) {
	pm.bytesRead.Add(n)
}

// SetInterval sets the monitoring interval.
func (pm *ProcessMonitor) SetInterval(d time.Duration) {
	pm.mu.Lock()
	pm.interval = d
	pm.mu.Unlock()
}

// monitorLoop is the main monitoring loop.
func (pm *ProcessMonitor) monitorLoop() {
	defer pm.wg.Done()

	ticker := time.NewTicker(pm.interval)
	defer ticker.Stop()

	// Initial sample
	pm.sample()

	for {
		select {
		case <-pm.ctx.Done():
			return
		case <-ticker.C:
			pm.sample()
		}
	}
}

// sample takes a snapshot of process statistics.
func (pm *ProcessMonitor) sample() {
	now := time.Now()

	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.stats.PID = pm.pid
	pm.stats.StartedAt = pm.startedAt
	pm.stats.Duration = now.Sub(pm.startedAt)
	pm.stats.LastUpdated = now

	// Read CPU and memory stats from /proc
	if runtime.GOOS == "linux" {
		pm.sampleLinux(now)
	} else {
		// For non-Linux platforms, we can only track what we measure ourselves
		pm.stats.BytesWritten = pm.bytesWritten.Load()
		pm.stats.BytesRead = pm.bytesRead.Load()
	}

	// Calculate bandwidth rates
	pm.calculateBandwidthRates(now)
}

// sampleLinux reads process stats from /proc filesystem.
func (pm *ProcessMonitor) sampleLinux(now time.Time) {
	// Read /proc/[pid]/stat for CPU usage
	statPath := fmt.Sprintf("/proc/%d/stat", pm.pid)
	statData, err := os.ReadFile(statPath)
	if err != nil {
		return // Process may have exited
	}

	// Parse stat file
	// Format: pid (comm) state ppid pgrp session tty_nr tpgid flags minflt cminflt majflt cmajflt utime stime ...
	// We need fields 14 (utime) and 15 (stime) which are in clock ticks
	fields := strings.Fields(string(statData))
	if len(fields) < 15 {
		return
	}

	// Find the end of the command name (enclosed in parentheses)
	statStr := string(statData)
	commEnd := strings.LastIndex(statStr, ")")
	if commEnd == -1 {
		return
	}

	// Parse fields after the command name
	afterComm := strings.Fields(statStr[commEnd+2:])
	if len(afterComm) < 13 {
		return
	}

	// utime is field 11 (0-indexed) after comm, stime is field 12
	utime, _ := strconv.ParseInt(afterComm[11], 10, 64)
	stime, _ := strconv.ParseInt(afterComm[12], 10, 64)

	// Convert clock ticks to duration
	tickDuration := time.Second / time.Duration(pm.clockTicksHz)
	cpuUser := time.Duration(utime) * tickDuration
	cpuSystem := time.Duration(stime) * tickDuration
	cpuTotal := cpuUser + cpuSystem

	pm.stats.CPUUser = cpuUser
	pm.stats.CPUSystem = cpuSystem
	pm.stats.CPUTotal = cpuTotal

	// Calculate CPU percentage
	elapsed := now.Sub(pm.lastCheckTime)
	if elapsed > 0 && pm.lastCPUTime > 0 {
		cpuDelta := cpuTotal - pm.lastCPUTime
		// CPU percent = (cpu time used / wall time) * 100
		pm.stats.CPUPercent = float64(cpuDelta) / float64(elapsed) * 100.0
	}

	pm.lastCPUTime = cpuTotal
	pm.lastCheckTime = now

	// Read /proc/[pid]/statm for memory usage
	statmPath := fmt.Sprintf("/proc/%d/statm", pm.pid)
	statmData, err := os.ReadFile(statmPath)
	if err != nil {
		return
	}

	statmFields := strings.Fields(string(statmData))
	if len(statmFields) >= 2 {
		// Fields are in pages: size resident shared text lib data dt
		pageSize := uint64(os.Getpagesize())

		vms, _ := strconv.ParseUint(statmFields[0], 10, 64)
		rss, _ := strconv.ParseUint(statmFields[1], 10, 64)

		pm.stats.MemoryVMSBytes = vms * pageSize
		pm.stats.MemoryRSSBytes = rss * pageSize
		pm.stats.MemoryRSSMB = float64(pm.stats.MemoryRSSBytes) / (1024 * 1024)

		if pm.totalMemory > 0 {
			pm.stats.MemoryPercent = float64(pm.stats.MemoryRSSBytes) / float64(pm.totalMemory) * 100.0
		}
	}
}

// calculateBandwidthRates calculates current bandwidth rates.
func (pm *ProcessMonitor) calculateBandwidthRates(now time.Time) {
	currentBytes := pm.bytesWritten.Load()
	elapsed := now.Sub(pm.lastBytesCheck)

	if elapsed > 0 {
		bytesDelta := currentBytes - pm.lastBytesWritten
		pm.stats.WriteRateBps = float64(bytesDelta) / elapsed.Seconds()
		pm.stats.WriteRateKbps = pm.stats.WriteRateBps * 8 / 1000        // Convert to kbps
		pm.stats.WriteRateMbps = pm.stats.WriteRateBps * 8 / 1_000_000  // Convert to Mbps
	}

	pm.stats.BytesWritten = currentBytes
	pm.stats.BytesRead = pm.bytesRead.Load()
	pm.lastBytesWritten = currentBytes
	pm.lastBytesCheck = now
}

// getClockTicks returns the system clock ticks per second (usually 100 on Linux).
func getClockTicks() int64 {
	// Default to 100 Hz which is common on Linux
	// Could use syscall.Sysconf(_SC_CLK_TCK) but that requires cgo
	return 100
}

// getTotalMemory returns the total system memory in bytes.
func getTotalMemory() uint64 {
	if runtime.GOOS != "linux" {
		return 0
	}

	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, _ := strconv.ParseUint(fields[1], 10, 64)
				return kb * 1024 // Convert KB to bytes
			}
		}
	}

	return 0
}

// CountingWriter wraps an io.Writer and counts bytes written.
type CountingWriter struct {
	w       Writer
	monitor *ProcessMonitor
}

// Writer interface for flexible writer types.
type Writer interface {
	Write(p []byte) (n int, err error)
}

// NewCountingWriter creates a writer that counts bytes and reports to monitor.
func NewCountingWriter(w Writer, monitor *ProcessMonitor) *CountingWriter {
	return &CountingWriter{
		w:       w,
		monitor: monitor,
	}
}

// Write implements io.Writer and tracks bytes written.
func (cw *CountingWriter) Write(p []byte) (n int, err error) {
	n, err = cw.w.Write(p)
	if n > 0 && cw.monitor != nil {
		cw.monitor.AddBytesWritten(uint64(n))
	}
	return n, err
}

// CountingReader wraps an io.Reader and counts bytes read.
type CountingReader struct {
	r       Reader
	monitor *ProcessMonitor
}

// Reader interface for flexible reader types.
type Reader interface {
	Read(p []byte) (n int, err error)
}

// NewCountingReader creates a reader that counts bytes and reports to monitor.
func NewCountingReader(r Reader, monitor *ProcessMonitor) *CountingReader {
	return &CountingReader{
		r:       r,
		monitor: monitor,
	}
}

// Read implements io.Reader and tracks bytes read.
func (cr *CountingReader) Read(p []byte) (n int, err error) {
	n, err = cr.r.Read(p)
	if n > 0 && cr.monitor != nil {
		cr.monitor.AddBytesRead(uint64(n))
	}
	return n, err
}
