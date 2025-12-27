package types

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDaemonState_String(t *testing.T) {
	tests := []struct {
		state    DaemonState
		expected string
	}{
		{DaemonStateConnecting, "connecting"},
		{DaemonStateConnected, "connected"},
		{DaemonStateDraining, "draining"},
		{DaemonStateUnhealthy, "unhealthy"},
		{DaemonStateDisconnected, "disconnected"},
		{DaemonState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.state.String())
		})
	}
}

func TestDaemon_IsHealthy(t *testing.T) {
	d := &Daemon{State: DaemonStateConnected}
	assert.True(t, d.IsHealthy())

	d.State = DaemonStateUnhealthy
	assert.False(t, d.IsHealthy())
}

func TestDaemon_CanAcceptJobs(t *testing.T) {
	d := &Daemon{
		State: DaemonStateConnected,
		Capabilities: &Capabilities{
			MaxConcurrentJobs: 4,
		},
		ActiveJobs: 2,
	}
	assert.True(t, d.CanAcceptJobs())

	// At capacity
	d.ActiveJobs = 4
	assert.False(t, d.CanAcceptJobs())

	// Not active
	d.State = DaemonStateDraining
	d.ActiveJobs = 0
	assert.False(t, d.CanAcceptJobs())

	// No capabilities
	d.State = DaemonStateConnected
	d.Capabilities = nil
	assert.False(t, d.CanAcceptJobs())
}

func TestDaemon_HasAvailableGPUSessions(t *testing.T) {
	d := &Daemon{
		Capabilities: &Capabilities{
			GPUs: []GPUInfo{
				{MaxEncodeSessions: 5, ActiveEncodeSessions: 3},
			},
		},
	}
	assert.True(t, d.HasAvailableGPUSessions())

	// All sessions used
	d.Capabilities.GPUs[0].ActiveEncodeSessions = 5
	assert.False(t, d.HasAvailableGPUSessions())

	// No capabilities
	d.Capabilities = nil
	assert.False(t, d.HasAvailableGPUSessions())
}

func TestCapabilities_HasEncoder(t *testing.T) {
	c := &Capabilities{
		VideoEncoders: []string{"libx264", "h264_nvenc"},
		AudioEncoders: []string{"aac", "libopus"},
	}

	assert.True(t, c.HasEncoder("libx264"))
	assert.True(t, c.HasEncoder("h264_nvenc"))
	assert.True(t, c.HasEncoder("aac"))
	assert.False(t, c.HasEncoder("hevc_nvenc"))
}

func TestCapabilities_HasHWAccel(t *testing.T) {
	c := &Capabilities{
		HWAccels: []HWAccelInfo{
			{Type: HWAccelCUDA, Available: true},
			{Type: HWAccelVAAPI, Available: false},
		},
	}

	assert.True(t, c.HasHWAccel(HWAccelCUDA))
	assert.False(t, c.HasHWAccel(HWAccelVAAPI)) // Not available
	assert.False(t, c.HasHWAccel(HWAccelQSV))   // Not present
}

func TestGPUClass_DefaultMaxEncodeSessions(t *testing.T) {
	tests := []struct {
		class    GPUClass
		expected int
	}{
		{GPUClassConsumer, 5},
		{GPUClassProfessional, 32},
		{GPUClassDatacenter, 0},
		{GPUClassIntegrated, 2},
		{GPUClassUnknown, 3},
	}

	for _, tt := range tests {
		t.Run(string(tt.class), func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.class.DefaultMaxEncodeSessions())
		})
	}
}

func TestJobState_String(t *testing.T) {
	tests := []struct {
		state    JobState
		expected string
	}{
		{JobStatePending, "pending"},
		{JobStateAssigned, "assigned"},
		{JobStateStarting, "starting"},
		{JobStateRunning, "running"},
		{JobStateCompleted, "completed"},
		{JobStateFailed, "failed"},
		{JobStateCancelled, "cancelled"},
		{JobState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.state.String())
		})
	}
}

func TestJobState_IsTerminal(t *testing.T) {
	assert.True(t, JobStateCompleted.IsTerminal())
	assert.True(t, JobStateFailed.IsTerminal())
	assert.True(t, JobStateCancelled.IsTerminal())
	assert.False(t, JobStatePending.IsTerminal())
	assert.False(t, JobStateRunning.IsTerminal())
}

func TestJobState_IsActive(t *testing.T) {
	assert.True(t, JobStateAssigned.IsActive())
	assert.True(t, JobStateStarting.IsActive())
	assert.True(t, JobStateRunning.IsActive())
	assert.False(t, JobStatePending.IsActive())
	assert.False(t, JobStateCompleted.IsActive())
}

func TestTranscodeJob_RunningTime(t *testing.T) {
	now := time.Now()
	job := &TranscodeJob{
		StartedAt:   now.Add(-time.Minute),
		CompletedAt: now,
	}
	assert.InDelta(t, time.Minute.Seconds(), job.RunningTime().Seconds(), 0.1)

	// Still running
	job.CompletedAt = time.Time{}
	job.StartedAt = time.Now().Add(-30 * time.Second)
	assert.InDelta(t, 30, job.RunningTime().Seconds(), 1)

	// Not started
	job.StartedAt = time.Time{}
	assert.Equal(t, time.Duration(0), job.RunningTime())
}

func TestTranscodeStats_CompressionRatio(t *testing.T) {
	stats := &TranscodeStats{
		BytesIn:  1000,
		BytesOut: 500,
	}
	assert.Equal(t, 0.5, stats.CompressionRatio())

	// Zero input
	stats.BytesIn = 0
	assert.Equal(t, float64(0), stats.CompressionRatio())
}

func TestESSampleBatch_TotalSamples(t *testing.T) {
	batch := &ESSampleBatch{
		VideoSamples: []ESSample{{}, {}, {}},
		AudioSamples: []ESSample{{}, {}},
	}
	assert.Equal(t, 5, batch.TotalSamples())
}

func TestESSampleBatch_TotalBytes(t *testing.T) {
	batch := &ESSampleBatch{
		VideoSamples: []ESSample{
			{Data: make([]byte, 100)},
			{Data: make([]byte, 200)},
		},
		AudioSamples: []ESSample{
			{Data: make([]byte, 50)},
		},
	}
	assert.Equal(t, 350, batch.TotalBytes())
}

func TestESSampleBatch_HasKeyframe(t *testing.T) {
	batch := &ESSampleBatch{
		VideoSamples: []ESSample{
			{IsKeyframe: false},
			{IsKeyframe: true},
		},
	}
	assert.True(t, batch.HasKeyframe())

	batch.VideoSamples = []ESSample{{IsKeyframe: false}}
	assert.False(t, batch.HasKeyframe())
}

func TestDaemon_JSONRoundTrip(t *testing.T) {
	daemon := &Daemon{
		ID:      DaemonID("test-daemon-1"),
		Name:    "Test Daemon",
		Version: "1.0.0",
		Address: "localhost:9090",
		State:   DaemonStateConnected,
		Capabilities: &Capabilities{
			VideoEncoders:     []string{"libx264", "h264_nvenc"},
			MaxConcurrentJobs: 4,
			GPUs: []GPUInfo{
				{
					Index: 0,
					Name:  "NVIDIA GeForce RTX 3080",
					Class: GPUClassConsumer,
				},
			},
		},
		ConnectedAt:   time.Now(),
		LastHeartbeat: time.Now(),
		ActiveJobs:    2,
	}

	// Serialize to JSON
	data, err := json.Marshal(daemon)
	require.NoError(t, err)

	// Deserialize from JSON
	var decoded Daemon
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Verify key fields
	assert.Equal(t, daemon.ID, decoded.ID)
	assert.Equal(t, daemon.Name, decoded.Name)
	assert.Equal(t, daemon.Version, decoded.Version)
	assert.Equal(t, daemon.ActiveJobs, decoded.ActiveJobs)
	assert.Equal(t, len(daemon.Capabilities.VideoEncoders), len(decoded.Capabilities.VideoEncoders))
	assert.Equal(t, daemon.Capabilities.GPUs[0].Name, decoded.Capabilities.GPUs[0].Name)
}

func TestTranscodeConfig_JSONRoundTrip(t *testing.T) {
	config := &TranscodeConfig{
		SourceVideoCodec:   "h264",
		SourceAudioCodec:   "aac",
		TargetVideoCodec:   "hevc",
		TargetAudioCodec:   "aac",
		VideoBitrateKbps:   5000,
		AudioBitrateKbps:   192,
		VideoPreset:        "fast",
		PreferredHWAccel:   "cuda",
		GPUExhaustedPolicy: GPUPolicyFallback,
		ExtraOptions: map[string]string{
			"profile": "main",
		},
	}

	// Serialize to JSON
	data, err := json.Marshal(config)
	require.NoError(t, err)

	// Deserialize from JSON
	var decoded TranscodeConfig
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Verify fields
	assert.Equal(t, config.SourceVideoCodec, decoded.SourceVideoCodec)
	assert.Equal(t, config.TargetVideoCodec, decoded.TargetVideoCodec)
	assert.Equal(t, config.VideoBitrateKbps, decoded.VideoBitrateKbps)
	assert.Equal(t, config.GPUExhaustedPolicy, decoded.GPUExhaustedPolicy)
	assert.Equal(t, config.ExtraOptions["profile"], decoded.ExtraOptions["profile"])
}

func TestSystemStats_JSON(t *testing.T) {
	stats := &SystemStats{
		Hostname:    "worker-1",
		OS:          "linux",
		Arch:        "amd64",
		CPUCores:    8,
		CPUPercent:  45.5,
		LoadAvg1m:   2.5,
		MemoryTotal: 16 * 1024 * 1024 * 1024,
		MemoryUsed:  8 * 1024 * 1024 * 1024,
		GPUs: []GPUStats{
			{
				Index:                0,
				Name:                 "NVIDIA GeForce RTX 3080",
				Utilization:          75.0,
				MaxEncodeSessions:    5,
				ActiveEncodeSessions: 3,
			},
		},
	}

	// Serialize to JSON
	data, err := json.Marshal(stats)
	require.NoError(t, err)

	// Deserialize from JSON
	var decoded SystemStats
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Verify fields
	assert.Equal(t, stats.Hostname, decoded.Hostname)
	assert.Equal(t, stats.CPUPercent, decoded.CPUPercent)
	assert.Equal(t, len(stats.GPUs), len(decoded.GPUs))
	assert.Equal(t, stats.GPUs[0].Utilization, decoded.GPUs[0].Utilization)
}

func TestGPUStats_AvailableEncodeSessions(t *testing.T) {
	stats := &GPUStats{
		MaxEncodeSessions:    5,
		ActiveEncodeSessions: 3,
	}
	assert.Equal(t, 2, stats.AvailableEncodeSessions())
}

func TestPressureStats_HasPressure(t *testing.T) {
	stats := &PressureStats{Avg10: 0, Avg60: 0, Avg300: 0}
	assert.False(t, stats.HasPressure())

	stats.Avg10 = 5.0
	assert.True(t, stats.HasPressure())
}
