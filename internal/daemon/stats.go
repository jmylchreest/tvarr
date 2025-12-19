package daemon

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
)

// StatsCollector collects system statistics for heartbeat reporting.
type StatsCollector struct {
	hostname        string
	startTime       time.Time
	lastNetStats    *net.IOCountersStat
	lastNetTime     time.Time
	gpuCapabilities []*proto.GPUInfo
}

// NewStatsCollector creates a new stats collector.
func NewStatsCollector(gpuCaps []*proto.GPUInfo) *StatsCollector {
	hostname, _ := os.Hostname()
	return &StatsCollector{
		hostname:        hostname,
		startTime:       time.Now(),
		gpuCapabilities: gpuCaps,
	}
}

// Collect gathers current system statistics.
func (c *StatsCollector) Collect(ctx context.Context) (*proto.SystemStats, error) {
	stats := &proto.SystemStats{
		Hostname: c.hostname,
		Os:       runtime.GOOS,
		Arch:     runtime.GOARCH,
	}

	// Host uptime
	if uptime, err := host.UptimeWithContext(ctx); err == nil {
		stats.UptimeSeconds = int64(uptime)
	}

	// CPU info
	if cpuCounts, err := cpu.CountsWithContext(ctx, true); err == nil {
		stats.CpuCores = int32(cpuCounts)
	}

	if cpuPercents, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(cpuPercents) > 0 {
		stats.CpuPercent = cpuPercents[0]
	}

	if cpuPerCore, err := cpu.PercentWithContext(ctx, 0, true); err == nil {
		stats.CpuPerCore = cpuPerCore
	}

	// Load average
	if loadAvg, err := load.AvgWithContext(ctx); err == nil {
		stats.LoadAvg_1M = loadAvg.Load1
		stats.LoadAvg_5M = loadAvg.Load5
		stats.LoadAvg_15M = loadAvg.Load15
	}

	// Memory info
	if memInfo, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		stats.MemoryTotalBytes = memInfo.Total
		stats.MemoryUsedBytes = memInfo.Used
		stats.MemoryAvailableBytes = memInfo.Available
		stats.MemoryPercent = memInfo.UsedPercent
	}

	if swapInfo, err := mem.SwapMemoryWithContext(ctx); err == nil {
		stats.SwapTotalBytes = swapInfo.Total
		stats.SwapUsedBytes = swapInfo.Used
	}

	// Disk info (work directory)
	workDir, _ := os.Getwd()
	if diskInfo, err := disk.UsageWithContext(ctx, workDir); err == nil {
		stats.DiskTotalBytes = diskInfo.Total
		stats.DiskUsedBytes = diskInfo.Used
		stats.DiskAvailableBytes = diskInfo.Free
		stats.DiskPercent = diskInfo.UsedPercent
	}

	// Network stats
	if netStats, err := net.IOCountersWithContext(ctx, false); err == nil && len(netStats) > 0 {
		netStat := netStats[0]
		stats.NetworkBytesSent = netStat.BytesSent
		stats.NetworkBytesRecv = netStat.BytesRecv

		// Calculate rates
		if c.lastNetStats != nil {
			elapsed := time.Since(c.lastNetTime).Seconds()
			if elapsed > 0 {
				stats.NetworkSendRateBps = float64(netStat.BytesSent-c.lastNetStats.BytesSent) / elapsed
				stats.NetworkRecvRateBps = float64(netStat.BytesRecv-c.lastNetStats.BytesRecv) / elapsed
			}
		}

		c.lastNetStats = &netStat
		c.lastNetTime = time.Now()
	}

	// GPU stats (NVIDIA via nvidia-smi)
	if gpuStats := c.collectGPUStats(ctx); len(gpuStats) > 0 {
		stats.Gpus = gpuStats
	}

	// Linux Pressure Stall Information (PSI)
	if runtime.GOOS == "linux" {
		if cpuPressure := readPSI("/proc/pressure/cpu"); cpuPressure != nil {
			stats.CpuPressure = cpuPressure
		}
		if memPressure := readPSI("/proc/pressure/memory"); memPressure != nil {
			stats.MemoryPressure = memPressure
		}
		if ioPressure := readPSI("/proc/pressure/io"); ioPressure != nil {
			stats.IoPressure = ioPressure
		}
	}

	return stats, nil
}

// CollectTypes gathers current system statistics in types format.
func (c *StatsCollector) CollectTypes(ctx context.Context) (*types.SystemStats, error) {
	stats := &types.SystemStats{
		Hostname: c.hostname,
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
	}

	// Host uptime
	if uptime, err := host.UptimeWithContext(ctx); err == nil {
		stats.Uptime = time.Duration(uptime) * time.Second
	}

	// CPU info
	if cpuCounts, err := cpu.CountsWithContext(ctx, true); err == nil {
		stats.CPUCores = cpuCounts
	}

	if cpuPercents, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(cpuPercents) > 0 {
		stats.CPUPercent = cpuPercents[0]
	}

	if cpuPerCore, err := cpu.PercentWithContext(ctx, 0, true); err == nil {
		stats.CPUPerCore = cpuPerCore
	}

	// Load average
	if loadAvg, err := load.AvgWithContext(ctx); err == nil {
		stats.LoadAvg1m = loadAvg.Load1
		stats.LoadAvg5m = loadAvg.Load5
		stats.LoadAvg15m = loadAvg.Load15
	}

	// Memory info
	if memInfo, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		stats.MemoryTotal = memInfo.Total
		stats.MemoryUsed = memInfo.Used
		stats.MemoryAvailable = memInfo.Available
		stats.MemoryPercent = memInfo.UsedPercent
	}

	if swapInfo, err := mem.SwapMemoryWithContext(ctx); err == nil {
		stats.SwapTotal = swapInfo.Total
		stats.SwapUsed = swapInfo.Used
	}

	// Disk info (work directory)
	workDir, _ := os.Getwd()
	if diskInfo, err := disk.UsageWithContext(ctx, workDir); err == nil {
		stats.DiskTotal = diskInfo.Total
		stats.DiskUsed = diskInfo.Used
		stats.DiskAvailable = diskInfo.Free
		stats.DiskPercent = diskInfo.UsedPercent
	}

	// Network stats
	if netStats, err := net.IOCountersWithContext(ctx, false); err == nil && len(netStats) > 0 {
		netStat := netStats[0]
		stats.NetworkBytesSent = netStat.BytesSent
		stats.NetworkBytesRecv = netStat.BytesRecv

		if c.lastNetStats != nil {
			elapsed := time.Since(c.lastNetTime).Seconds()
			if elapsed > 0 {
				stats.NetworkSendRate = float64(netStat.BytesSent-c.lastNetStats.BytesSent) / elapsed
				stats.NetworkRecvRate = float64(netStat.BytesRecv-c.lastNetStats.BytesRecv) / elapsed
			}
		}

		c.lastNetStats = &netStat
		c.lastNetTime = time.Now()
	}

	// GPU stats
	if gpuStats := c.collectGPUStatsTypes(ctx); len(gpuStats) > 0 {
		stats.GPUs = gpuStats
	}

	// Linux PSI
	if runtime.GOOS == "linux" {
		if cpuPressure := readPSITypes("/proc/pressure/cpu"); cpuPressure != nil {
			stats.CPUPressure = cpuPressure
		}
		if memPressure := readPSITypes("/proc/pressure/memory"); memPressure != nil {
			stats.MemoryPressure = memPressure
		}
		if ioPressure := readPSITypes("/proc/pressure/io"); ioPressure != nil {
			stats.IOPressure = ioPressure
		}
	}

	return stats, nil
}

// collectGPUStats collects NVIDIA GPU stats via nvidia-smi.
func (c *StatsCollector) collectGPUStats(ctx context.Context) []*proto.GPUStats {
	// Check for nvidia-smi
	cmd := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu=index,name,driver_version,utilization.gpu,memory.used,memory.total,temperature.gpu,power.draw,utilization.encoder,utilization.decoder",
		"--format=csv,noheader,nounits")

	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var stats []*proto.GPUStats
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		parts := strings.Split(line, ", ")
		if len(parts) < 10 {
			continue
		}

		index, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
		utilization, _ := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
		memUsed, _ := strconv.ParseUint(strings.TrimSpace(parts[4]), 10, 64)
		memTotal, _ := strconv.ParseUint(strings.TrimSpace(parts[5]), 10, 64)
		temp, _ := strconv.Atoi(strings.TrimSpace(parts[6]))
		power, _ := strconv.ParseFloat(strings.TrimSpace(parts[7]), 64)
		encUtil, _ := strconv.ParseFloat(strings.TrimSpace(parts[8]), 64)
		decUtil, _ := strconv.ParseFloat(strings.TrimSpace(parts[9]), 64)

		stat := &proto.GPUStats{
			Index:                int32(index),
			Name:                 strings.TrimSpace(parts[1]),
			DriverVersion:        strings.TrimSpace(parts[2]),
			UtilizationPercent:   utilization,
			MemoryTotalBytes:     memTotal * 1024 * 1024, // MiB to bytes
			MemoryUsedBytes:      memUsed * 1024 * 1024,
			MemoryPercent:        float64(memUsed) / float64(memTotal) * 100,
			TemperatureCelsius:   int32(temp),
			PowerWatts:           int32(power),
			EncoderUtilization:   encUtil,
			DecoderUtilization:   decUtil,
			MaxEncodeSessions:    5, // Default for consumer NVIDIA
			ActiveEncodeSessions: 0, // TODO: Track actual sessions
		}

		// Update max sessions from capabilities if available
		for _, gpuCap := range c.gpuCapabilities {
			if int(gpuCap.Index) == index {
				stat.MaxEncodeSessions = gpuCap.MaxEncodeSessions
				stat.GpuClass = gpuCap.GpuClass
				break
			}
		}

		stats = append(stats, stat)
	}

	return stats
}

// collectGPUStatsTypes collects GPU stats in types format.
func (c *StatsCollector) collectGPUStatsTypes(ctx context.Context) []types.GPUStats {
	protoStats := c.collectGPUStats(ctx)
	var result []types.GPUStats

	for _, ps := range protoStats {
		stat := types.GPUStats{
			Index:              int(ps.Index),
			Name:               ps.Name,
			DriverVersion:      ps.DriverVersion,
			Utilization:        ps.UtilizationPercent,
			MemoryTotal:        ps.MemoryTotalBytes,
			MemoryUsed:         ps.MemoryUsedBytes,
			MemoryPercent:      ps.MemoryPercent,
			Temperature:        int(ps.TemperatureCelsius),
			PowerWatts:         int(ps.PowerWatts),
			EncoderUtilization: ps.EncoderUtilization,
			DecoderUtilization: ps.DecoderUtilization,
			MaxEncodeSessions:  int(ps.MaxEncodeSessions),
		}

		// Convert GPU class
		switch ps.GpuClass {
		case proto.GPUClass_GPU_CLASS_CONSUMER:
			stat.Class = types.GPUClassConsumer
		case proto.GPUClass_GPU_CLASS_PROFESSIONAL:
			stat.Class = types.GPUClassProfessional
		case proto.GPUClass_GPU_CLASS_DATACENTER:
			stat.Class = types.GPUClassDatacenter
		case proto.GPUClass_GPU_CLASS_INTEGRATED:
			stat.Class = types.GPUClassIntegrated
		default:
			stat.Class = types.GPUClassUnknown
		}

		result = append(result, stat)
	}

	return result
}

// readPSI reads Linux Pressure Stall Information.
func readPSI(path string) *proto.PressureStats {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	stats := &proto.PressureStats{}
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		if strings.HasPrefix(line, "some") {
			// Parse: some avg10=X.XX avg60=X.XX avg300=X.XX total=XXXX
			parts := strings.Fields(line)
			for _, part := range parts[1:] {
				kv := strings.Split(part, "=")
				if len(kv) != 2 {
					continue
				}
				val, _ := strconv.ParseFloat(kv[1], 64)
				switch kv[0] {
				case "avg10":
					stats.Avg10 = val
				case "avg60":
					stats.Avg60 = val
				case "avg300":
					stats.Avg300 = val
				case "total":
					stats.TotalUs = uint64(val)
				}
			}
		}
	}

	return stats
}

// readPSITypes reads Linux PSI in types format.
func readPSITypes(path string) *types.PressureStats {
	protoStats := readPSI(path)
	if protoStats == nil {
		return nil
	}

	return &types.PressureStats{
		Avg10:   protoStats.Avg10,
		Avg60:   protoStats.Avg60,
		Avg300:  protoStats.Avg300,
		TotalUs: protoStats.TotalUs,
	}
}
