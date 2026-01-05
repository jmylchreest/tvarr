package daemon

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
)

// GPUSessionTracker tracks active GPU encode/decode sessions.
type GPUSessionTracker struct {
	mu sync.RWMutex

	// Active sessions per GPU index
	encodeSessions map[int]int
	decodeSessions map[int]int

	// Max sessions per GPU (from detection or env override)
	maxEncodeSessions map[int]int
	maxDecodeSessions map[int]int

	// GPU info from capability detection
	gpus []*proto.GPUInfo
}

// NewGPUSessionTracker creates a new GPU session tracker.
func NewGPUSessionTracker(gpus []*proto.GPUInfo) *GPUSessionTracker {
	t := &GPUSessionTracker{
		encodeSessions:    make(map[int]int),
		decodeSessions:    make(map[int]int),
		maxEncodeSessions: make(map[int]int),
		maxDecodeSessions: make(map[int]int),
		gpus:              gpus,
	}

	// Initialize max sessions from GPU info or env override
	envMaxSessions := getEnvMaxSessions()

	for _, gpu := range gpus {
		idx := int(gpu.Index)
		if envMaxSessions > 0 {
			// Env override applies to all GPUs
			t.maxEncodeSessions[idx] = envMaxSessions
		} else {
			t.maxEncodeSessions[idx] = int(gpu.MaxEncodeSessions)
		}
		// Decode sessions typically have higher limits
		t.maxDecodeSessions[idx] = t.maxEncodeSessions[idx] * 2
	}

	return t
}

// getEnvMaxSessions returns the max sessions from TVARR_GPU_MAX_SESSIONS env var.
// Returns 0 if not set or invalid.
func getEnvMaxSessions() int {
	val := os.Getenv("TVARR_GPU_MAX_SESSIONS")
	if val == "" {
		return 0
	}
	n, err := strconv.Atoi(val)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// AcquireEncodeSession attempts to acquire an encode session on the specified GPU.
// Returns true if successful, false if no sessions available.
func (t *GPUSessionTracker) AcquireEncodeSession(gpuIndex int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	maxSessions := t.maxEncodeSessions[gpuIndex]
	if maxSessions == 0 {
		// 0 means unlimited
		t.encodeSessions[gpuIndex]++
		return true
	}

	if t.encodeSessions[gpuIndex] >= maxSessions {
		return false
	}

	t.encodeSessions[gpuIndex]++
	return true
}

// ReleaseEncodeSession releases an encode session on the specified GPU.
func (t *GPUSessionTracker) ReleaseEncodeSession(gpuIndex int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.encodeSessions[gpuIndex] > 0 {
		t.encodeSessions[gpuIndex]--
	}
}

// AcquireDecodeSession attempts to acquire a decode session on the specified GPU.
func (t *GPUSessionTracker) AcquireDecodeSession(gpuIndex int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	maxSessions := t.maxDecodeSessions[gpuIndex]
	if maxSessions == 0 {
		t.decodeSessions[gpuIndex]++
		return true
	}

	if t.decodeSessions[gpuIndex] >= maxSessions {
		return false
	}

	t.decodeSessions[gpuIndex]++
	return true
}

// ReleaseDecodeSession releases a decode session on the specified GPU.
func (t *GPUSessionTracker) ReleaseDecodeSession(gpuIndex int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.decodeSessions[gpuIndex] > 0 {
		t.decodeSessions[gpuIndex]--
	}
}

// GetSessionCounts returns current session counts for a GPU.
func (t *GPUSessionTracker) GetSessionCounts(gpuIndex int) (activeEncode, maxEncode, activeDecode, maxDecode int) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.encodeSessions[gpuIndex],
		t.maxEncodeSessions[gpuIndex],
		t.decodeSessions[gpuIndex],
		t.maxDecodeSessions[gpuIndex]
}

// HasAvailableEncodeSessions returns true if any GPU has available encode sessions.
func (t *GPUSessionTracker) HasAvailableEncodeSessions() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for idx, max := range t.maxEncodeSessions {
		if max == 0 {
			// Unlimited
			return true
		}
		if t.encodeSessions[idx] < max {
			return true
		}
	}
	return false
}

// GetGPUWithAvailableSession returns the index of a GPU with available encode sessions.
// Returns -1 if none available.
func (t *GPUSessionTracker) GetGPUWithAvailableSession() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Prefer GPUs with more available sessions
	bestIdx := -1
	bestAvailable := 0

	for idx, max := range t.maxEncodeSessions {
		if max == 0 {
			// Unlimited - always prefer this
			return idx
		}
		available := max - t.encodeSessions[idx]
		if available > bestAvailable {
			bestAvailable = available
			bestIdx = idx
		}
	}

	return bestIdx
}

// UpdateGPUStats updates GPU stats with session tracking info.
func (t *GPUSessionTracker) UpdateGPUStats(stats []*proto.GPUStats) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, stat := range stats {
		idx := int(stat.Index)
		stat.ActiveEncodeSessions = int32(t.encodeSessions[idx])
		stat.ActiveDecodeSessions = int32(t.decodeSessions[idx])
		if max, ok := t.maxEncodeSessions[idx]; ok {
			stat.MaxEncodeSessions = int32(max)
		}
		if max, ok := t.maxDecodeSessions[idx]; ok {
			stat.MaxDecodeSessions = int32(max)
		}
	}
}

// UpdateGPUStatsTypes updates types format GPU stats with session tracking info.
func (t *GPUSessionTracker) UpdateGPUStatsTypes(stats []types.GPUStats) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for i := range stats {
		idx := stats[i].Index
		stats[i].ActiveEncodeSessions = t.encodeSessions[idx]
		stats[i].ActiveDecodeSessions = t.decodeSessions[idx]
		if max, ok := t.maxEncodeSessions[idx]; ok {
			stats[i].MaxEncodeSessions = max
		}
		if max, ok := t.maxDecodeSessions[idx]; ok {
			stats[i].MaxDecodeSessions = max
		}
	}
}

// DetectGPUSessionLimits detects GPU session limits from the system.
// For NVIDIA GPUs, this queries nvidia-smi for encoder session info.
func DetectGPUSessionLimits(ctx context.Context) (map[int]GPUSessionLimits, error) {
	limits := make(map[int]GPUSessionLimits)

	// Check for env override first
	envMax := getEnvMaxSessions()

	// Try NVIDIA detection
	nvidiaLimits, err := detectNVIDIASessionLimits(ctx)
	if err == nil {
		for idx, nl := range nvidiaLimits {
			if envMax > 0 {
				nl.MaxEncodeSessions = envMax
			}
			limits[idx] = nl
		}
	}

	return limits, nil
}

// GPUSessionLimits contains session limit information for a GPU.
type GPUSessionLimits struct {
	Index             int
	MaxEncodeSessions int
	MaxDecodeSessions int
	Class             types.GPUClass
}

// detectNVIDIASessionLimits detects NVIDIA GPU session limits.
func detectNVIDIASessionLimits(ctx context.Context) (map[int]GPUSessionLimits, error) {
	limits := make(map[int]GPUSessionLimits)

	// Get GPU list with names
	cmd := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=index,name", "--format=csv,noheader,nounits")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.SplitSeq(strings.TrimSpace(string(output)), "\n")
	for line := range lines {
		parts := strings.SplitN(line, ", ", 2)
		if len(parts) < 2 {
			continue
		}

		index, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			continue
		}

		name := strings.TrimSpace(parts[1])
		gpuClass := detectGPUClassFromName(name)

		limit := GPUSessionLimits{
			Index:             index,
			MaxEncodeSessions: gpuClass.DefaultMaxEncodeSessions(),
			MaxDecodeSessions: gpuClass.DefaultMaxEncodeSessions() * 2, // Decode typically higher
			Class:             gpuClass,
		}

		// Try to get actual session limit from nvidia-smi
		if actual := getNVIDIAEncoderSessionLimit(ctx, index); actual > 0 {
			limit.MaxEncodeSessions = actual
		}

		limits[index] = limit
	}

	return limits, nil
}

// getNVIDIAEncoderSessionLimit attempts to query the actual encoder session limit.
// This uses nvidia-smi encoder session query if available.
func getNVIDIAEncoderSessionLimit(ctx context.Context, gpuIndex int) int {
	// Try to get encoder sessions info
	// nvidia-smi has limited support for querying session limits directly
	// We can check encoder.sessions.count but not the max

	// For now, use the class-based defaults which are well-documented:
	// - Consumer (GeForce): 5 sessions (3 on older cards, 5 on 10xx+)
	// - Quadro: 32 sessions
	// - Datacenter: Unlimited
	// - Turing/Ampere consumer: 5 sessions (8 on some RTX 4xxx)

	// Query current encoder sessions to verify NVENC is working
	cmd := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu=encoder.stats.sessionCount",
		"--format=csv,noheader,nounits",
		"-i", strconv.Itoa(gpuIndex))

	output, err := cmd.Output()
	if err != nil {
		return 0 // Can't determine, use default
	}

	// If we can query sessions, NVENC is available
	count := strings.TrimSpace(string(output))
	if count == "" || count == "[N/A]" {
		return 0
	}

	// Return 0 to indicate we should use class-based defaults
	// (the actual current count isn't the limit)
	return 0
}

// detectGPUClassFromName determines GPU class from its name.
func detectGPUClassFromName(name string) types.GPUClass {
	nameLower := strings.ToLower(name)

	// Datacenter GPUs
	if strings.Contains(nameLower, "tesla") ||
		strings.Contains(nameLower, "a100") ||
		strings.Contains(nameLower, "h100") ||
		strings.Contains(nameLower, "a40") ||
		strings.Contains(nameLower, "l40") ||
		strings.Contains(nameLower, "v100") {
		return types.GPUClassDatacenter
	}

	// Professional GPUs
	if strings.Contains(nameLower, "quadro") ||
		strings.Contains(nameLower, "rtx a") ||
		strings.Contains(nameLower, "pro ") ||
		strings.Contains(nameLower, "nvidia t") { // T4, T1000, etc
		return types.GPUClassProfessional
	}

	// Consumer GPUs
	if strings.Contains(nameLower, "geforce") ||
		strings.Contains(nameLower, "gtx") ||
		strings.Contains(nameLower, "rtx") {
		return types.GPUClassConsumer
	}

	return types.GPUClassUnknown
}

// CountActiveNVIDIASessions queries nvidia-smi for the current encoder session count.
func CountActiveNVIDIASessions(ctx context.Context, gpuIndex int) (int, error) {
	cmd := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu=encoder.stats.sessionCount",
		"--format=csv,noheader,nounits",
		"-i", strconv.Itoa(gpuIndex))

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	count := strings.TrimSpace(string(output))
	if count == "" || count == "[N/A]" {
		return 0, nil
	}

	return strconv.Atoi(count)
}

// GetNVIDIAEncoderProcesses returns a list of processes using the NVIDIA encoder.
func GetNVIDIAEncoderProcesses(ctx context.Context) ([]NVIDIAEncoderProcess, error) {
	// Use nvidia-smi pmon to get encoder processes
	cmd := exec.CommandContext(ctx, "nvidia-smi", "pmon", "-s", "u", "-c", "1")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var processes []NVIDIAEncoderProcess
	scanner := bufio.NewScanner(strings.NewReader(string(output)))

	for scanner.Scan() {
		line := scanner.Text()
		// Skip header and comment lines
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}

		// Format: # gpu    pid    type    sm   mem   enc   dec   command
		gpuIdx, _ := strconv.Atoi(fields[0])
		pid, _ := strconv.Atoi(fields[1])
		encUtil, _ := strconv.Atoi(fields[5])

		if encUtil > 0 {
			processes = append(processes, NVIDIAEncoderProcess{
				GPUIndex:           gpuIdx,
				PID:                pid,
				EncoderUtilization: encUtil,
			})
		}
	}

	return processes, nil
}

// NVIDIAEncoderProcess represents a process using the NVIDIA encoder.
type NVIDIAEncoderProcess struct {
	GPUIndex           int
	PID                int
	EncoderUtilization int
}
