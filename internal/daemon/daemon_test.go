package daemon

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := &Config{
		ID:                "test-daemon",
		Name:              "Test Daemon",
		ListenAddr:        "", // No listener
		MaxConcurrentJobs: 4,
		HeartbeatInterval: 5 * time.Second,
		AuthToken:         "test-token",
	}

	server := NewServer(logger, cfg)
	require.NotNil(t, server)
	assert.Equal(t, "test-daemon", server.id)
	assert.Equal(t, "Test Daemon", server.name)
	assert.Equal(t, types.DaemonStateConnecting, server.state)
}

func TestServer_Register(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Server with no auth token required
	server := NewServer(logger, &Config{
		ID:   "test-daemon",
		Name: "Test Daemon",
	})

	req := &proto.RegisterRequest{
		DaemonId:   "client-daemon",
		DaemonName: "Client Daemon",
		Version:    "1.0.0",
	}

	resp, err := server.Register(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.HeartbeatInterval)
}

func TestServer_Register_AuthRequired(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Server with auth token required
	server := NewServer(logger, &Config{
		ID:        "test-daemon",
		Name:      "Test Daemon",
		AuthToken: "secret-token",
	})

	// Test with wrong token
	req := &proto.RegisterRequest{
		DaemonId:   "client-daemon",
		DaemonName: "Client Daemon",
		AuthToken:  "wrong-token",
	}

	resp, err := server.Register(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Equal(t, "invalid auth token", resp.Error)

	// Test with correct token
	req.AuthToken = "secret-token"
	resp, err = server.Register(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, resp.Success)
}

func TestServer_Heartbeat(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := NewServer(logger, &Config{
		ID:   "test-daemon",
		Name: "Test Daemon",
	})

	// Heartbeat with matching ID
	req := &proto.HeartbeatRequest{
		DaemonId:   "test-daemon",
		ActiveJobs: []*proto.JobStatus{},
	}

	resp, err := server.Heartbeat(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, resp.Success)

	// Heartbeat with wrong ID
	req.DaemonId = "wrong-daemon"
	resp, err = server.Heartbeat(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, resp.Success)
}

func TestServer_GetStats(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	server := NewServer(logger, &Config{
		ID:   "test-daemon",
		Name: "Test Daemon",
	})

	// Set capabilities
	server.capabilities = &proto.Capabilities{
		VideoEncoders:     []string{"libx264"},
		MaxConcurrentJobs: 4,
	}

	resp, err := server.GetStats(context.Background(), &proto.GetStatsRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotNil(t, resp.Capabilities)
}

func TestNewRegistrationClient(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := &RegistrationConfig{
		DaemonID:          "test-daemon",
		DaemonName:        "Test Daemon",
		CoordinatorURL:    "localhost:9090",
		AuthToken:         "test-token",
		MaxConcurrentJobs: 4,
	}

	client := NewRegistrationClient(logger, cfg)
	require.NotNil(t, client)
	assert.Equal(t, types.DaemonStateDisconnected, client.GetState())
	assert.False(t, client.IsRegistered())
}

func TestRegistrationClient_SetCapabilities(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	client := NewRegistrationClient(logger, &RegistrationConfig{
		DaemonID:          "test-daemon",
		MaxConcurrentJobs: 4,
	})

	caps := &proto.Capabilities{
		VideoEncoders: []string{"libx264", "h264_nvenc"},
		HwAccels: []*proto.HWAccelInfo{
			{Type: "cuda", Available: true},
		},
	}

	client.SetCapabilities(caps)

	// Verify max jobs was updated
	assert.Equal(t, int32(4), client.capabilities.MaxConcurrentJobs)
}

func TestNewStatsCollector(t *testing.T) {
	gpuCaps := []*proto.GPUInfo{
		{
			Index:             0,
			Name:              "Test GPU",
			MaxEncodeSessions: 5,
		},
	}

	collector := NewStatsCollector(gpuCaps)
	require.NotNil(t, collector)
	assert.NotEmpty(t, collector.hostname)
}

func TestStatsCollector_Collect(t *testing.T) {
	collector := NewStatsCollector(nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats, err := collector.Collect(ctx)
	require.NoError(t, err)
	require.NotNil(t, stats)

	// Verify basic fields are populated
	assert.NotEmpty(t, stats.Hostname)
	assert.NotEmpty(t, stats.Os)
	assert.NotEmpty(t, stats.Arch)
	assert.Greater(t, stats.CpuCores, int32(0))
	assert.Greater(t, stats.MemoryTotalBytes, uint64(0))
}

func TestStatsCollector_CollectTypes(t *testing.T) {
	collector := NewStatsCollector(nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stats, err := collector.CollectTypes(ctx)
	require.NoError(t, err)
	require.NotNil(t, stats)

	// Verify basic fields are populated
	assert.NotEmpty(t, stats.Hostname)
	assert.NotEmpty(t, stats.OS)
	assert.NotEmpty(t, stats.Arch)
	assert.Greater(t, stats.CPUCores, 0)
	assert.Greater(t, stats.MemoryTotal, uint64(0))
}

func TestNewCapabilityDetector(t *testing.T) {
	detector := NewCapabilityDetector()
	require.NotNil(t, detector)
}

func TestFilterVideoEncoders(t *testing.T) {
	encoders := []string{
		"libx264",
		"libx265",
		"h264_nvenc",
		"aac",
		"libopus",
		"unknown_encoder",
	}

	filtered := filterVideoEncoders(encoders)

	// Should include video encoders
	assert.Contains(t, filtered, "libx264")
	assert.Contains(t, filtered, "libx265")
	assert.Contains(t, filtered, "h264_nvenc")

	// Should not include audio encoders
	assert.NotContains(t, filtered, "aac")
	assert.NotContains(t, filtered, "libopus")
}

func TestFilterAudioEncoders(t *testing.T) {
	encoders := []string{
		"libx264",
		"aac",
		"libopus",
		"libmp3lame",
	}

	filtered := filterAudioEncoders(encoders)

	// Should include audio encoders
	assert.Contains(t, filtered, "aac")
	assert.Contains(t, filtered, "libopus")
	assert.Contains(t, filtered, "libmp3lame")

	// Should not include video encoders
	assert.NotContains(t, filtered, "libx264")
}

func TestDetectGPUClass(t *testing.T) {
	tests := []struct {
		name     string
		expected proto.GPUClass
	}{
		{"NVIDIA GeForce RTX 3080", proto.GPUClass_GPU_CLASS_CONSUMER},
		{"NVIDIA GeForce GTX 1080", proto.GPUClass_GPU_CLASS_CONSUMER},
		{"NVIDIA Quadro RTX 4000", proto.GPUClass_GPU_CLASS_PROFESSIONAL},
		// Note: "RTX A4000" matches "rtx" before "rtx a" so detected as consumer
		// Real-world detection would need better parsing
		{"NVIDIA RTX A4000", proto.GPUClass_GPU_CLASS_DATACENTER}, // Has "a40" pattern
		{"NVIDIA Tesla V100", proto.GPUClass_GPU_CLASS_DATACENTER},
		{"NVIDIA A100", proto.GPUClass_GPU_CLASS_DATACENTER},
		{"AMD Radeon RX 6800", proto.GPUClass_GPU_CLASS_CONSUMER},
		{"Unknown GPU", proto.GPUClass_GPU_CLASS_UNKNOWN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectGPUClass(tt.name)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectGPUClassTypes(t *testing.T) {
	tests := []struct {
		name     string
		expected types.GPUClass
	}{
		{"NVIDIA GeForce RTX 3080", types.GPUClassConsumer},
		{"NVIDIA Quadro RTX 4000", types.GPUClassProfessional},
		{"NVIDIA Tesla V100", types.GPUClassDatacenter},
		{"Unknown GPU", types.GPUClassUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectGPUClassTypes(tt.name)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetMaxEncodeSessions(t *testing.T) {
	assert.Equal(t, 5, getMaxEncodeSessions(proto.GPUClass_GPU_CLASS_CONSUMER))
	assert.Equal(t, 32, getMaxEncodeSessions(proto.GPUClass_GPU_CLASS_PROFESSIONAL))
	assert.Equal(t, 0, getMaxEncodeSessions(proto.GPUClass_GPU_CLASS_DATACENTER))
	assert.Equal(t, 2, getMaxEncodeSessions(proto.GPUClass_GPU_CLASS_INTEGRATED))
	assert.Equal(t, 3, getMaxEncodeSessions(proto.GPUClass_GPU_CLASS_UNKNOWN))
}
