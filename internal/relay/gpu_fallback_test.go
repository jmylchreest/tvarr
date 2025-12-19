package relay

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
)

func TestParseGPUExhaustedPolicy(t *testing.T) {
	tests := []struct {
		input    string
		expected GPUExhaustedPolicy
		hasError bool
	}{
		{"fallback", GPUPolicyFallback, false},
		{"queue", GPUPolicyQueue, false},
		{"reject", GPUPolicyReject, false},
		{"", GPUPolicyFallback, false}, // Default
		{"invalid", GPUPolicyFallback, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			policy, err := ParseGPUExhaustedPolicy(tt.input)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expected, policy)
		})
	}
}

func TestGPUFallbackHandler_Fallback(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := NewDaemonRegistry(logger)

	// Register CPU-only daemon
	cpuDaemon := &types.Daemon{
		ID:         "cpu-daemon",
		Name:       "CPU Daemon",
		State:      types.DaemonStateConnected,
		ActiveJobs: 0,
		Capabilities: &types.Capabilities{
			VideoEncoders:     []string{"libx264", "libx265"},
			MaxConcurrentJobs: 4,
		},
	}

	// Register GPU daemon with exhausted sessions
	gpuDaemon := &types.Daemon{
		ID:         "gpu-daemon",
		Name:       "GPU Daemon",
		State:      types.DaemonStateConnected,
		ActiveJobs: 0,
		Capabilities: &types.Capabilities{
			VideoEncoders:     []string{"h264_nvenc", "libx264"},
			MaxConcurrentJobs: 8,
			GPUs: []types.GPUInfo{
				{
					Index:                0,
					Name:                 "RTX 3080",
					MaxEncodeSessions:    3,
					ActiveEncodeSessions: 3, // Exhausted
				},
			},
		},
	}

	// Add daemons manually
	registry.mu.Lock()
	registry.daemons[cpuDaemon.ID] = cpuDaemon
	registry.daemons[gpuDaemon.ID] = gpuDaemon
	registry.mu.Unlock()

	config := GPUFallbackConfig{
		Policy:       GPUPolicyFallback,
		QueueTimeout: 5 * time.Second,
	}
	handler := NewGPUFallbackHandler(registry, config, logger)

	t.Run("falls_back_to_software_encoder", func(t *testing.T) {
		criteria := SelectionCriteria{
			RequiredEncoder: "h264_nvenc",
			RequireGPU:      true,
		}
		strategy := NewStrategyCapabilityMatch()

		result := handler.HandleGPUExhausted(context.Background(), criteria, strategy)

		assert.NoError(t, result.Error)
		require.NotNil(t, result.Daemon)
		assert.True(t, result.UsedFallback)
		assert.False(t, result.WaitedForGPU)
		// Should have found a daemon with libx264
		assert.True(t, result.Daemon.Capabilities.HasEncoder("libx264"))
	})

	t.Run("maps_hardware_encoder_to_software", func(t *testing.T) {
		tests := []struct {
			hwEncoder string
			swEncoder string
		}{
			{"h264_nvenc", "libx264"},
			{"hevc_nvenc", "libx265"},
			{"h264_qsv", "libx264"},
			{"hevc_vaapi", "libx265"},
			{"h264_amf", "libx264"},
		}

		for _, tt := range tests {
			t.Run(tt.hwEncoder, func(t *testing.T) {
				fallback := handler.getSoftwareFallbackEncoder(tt.hwEncoder)
				assert.Equal(t, tt.swEncoder, fallback)
			})
		}
	})
}

func TestGPUFallbackHandler_Reject(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := NewDaemonRegistry(logger)

	config := GPUFallbackConfig{
		Policy: GPUPolicyReject,
	}
	handler := NewGPUFallbackHandler(registry, config, logger)

	t.Run("rejects_immediately", func(t *testing.T) {
		criteria := SelectionCriteria{
			RequiredEncoder: "h264_nvenc",
			RequireGPU:      true,
		}
		strategy := NewStrategyCapabilityMatch()

		result := handler.HandleGPUExhausted(context.Background(), criteria, strategy)

		assert.Error(t, result.Error)
		assert.Nil(t, result.Daemon)
		assert.Contains(t, result.Error.Error(), "reject policy")
	})
}

func TestGPUFallbackHandler_Queue(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := NewDaemonRegistry(logger)

	// Register GPU daemon with exhausted sessions
	gpuDaemon := &types.Daemon{
		ID:         "gpu-daemon",
		Name:       "GPU Daemon",
		State:      types.DaemonStateConnected,
		ActiveJobs: 0,
		Capabilities: &types.Capabilities{
			VideoEncoders:     []string{"h264_nvenc"},
			MaxConcurrentJobs: 8,
			GPUs: []types.GPUInfo{
				{
					Index:                0,
					Name:                 "RTX 3080",
					MaxEncodeSessions:    3,
					ActiveEncodeSessions: 3, // Exhausted
				},
			},
		},
	}

	registry.mu.Lock()
	registry.daemons[gpuDaemon.ID] = gpuDaemon
	registry.mu.Unlock()

	config := GPUFallbackConfig{
		Policy:       GPUPolicyQueue,
		QueueTimeout: 100 * time.Millisecond, // Short timeout for testing
		MaxQueueSize: 10,
	}
	handler := NewGPUFallbackHandler(registry, config, logger)

	t.Run("times_out_when_no_gpu_available", func(t *testing.T) {
		criteria := SelectionCriteria{
			RequiredEncoder: "h264_nvenc",
			RequireGPU:      true,
		}
		strategy := NewStrategyGPUAware()

		start := time.Now()
		result := handler.HandleGPUExhausted(context.Background(), criteria, strategy)
		elapsed := time.Since(start)

		assert.Error(t, result.Error)
		assert.Contains(t, result.Error.Error(), "timeout")
		assert.GreaterOrEqual(t, elapsed, 90*time.Millisecond)
	})

	t.Run("respects_context_cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		criteria := SelectionCriteria{
			RequiredEncoder: "h264_nvenc",
			RequireGPU:      true,
		}
		strategy := NewStrategyGPUAware()

		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()

		start := time.Now()
		result := handler.HandleGPUExhausted(ctx, criteria, strategy)
		elapsed := time.Since(start)

		assert.Error(t, result.Error)
		assert.ErrorIs(t, result.Error, context.Canceled)
		assert.Less(t, elapsed, 100*time.Millisecond)
	})

	t.Run("succeeds_when_gpu_becomes_available", func(t *testing.T) {
		criteria := SelectionCriteria{
			RequiredEncoder: "h264_nvenc",
			RequireGPU:      true,
		}
		strategy := NewStrategyGPUAware()

		// Start waiting in background
		done := make(chan FallbackResult)
		go func() {
			done <- handler.HandleGPUExhausted(context.Background(), criteria, strategy)
		}()

		// Simulate GPU session becoming available
		time.Sleep(20 * time.Millisecond)
		registry.mu.Lock()
		gpuDaemon.Capabilities.GPUs[0].ActiveEncodeSessions = 2 // Free 1 session
		registry.mu.Unlock()
		handler.NotifyGPUAvailable()

		result := <-done

		assert.NoError(t, result.Error)
		require.NotNil(t, result.Daemon)
		assert.True(t, result.WaitedForGPU)
	})

	t.Run("rejects_when_queue_full", func(t *testing.T) {
		smallQueueConfig := GPUFallbackConfig{
			Policy:       GPUPolicyQueue,
			QueueTimeout: time.Second,
			MaxQueueSize: 2,
		}
		smallHandler := NewGPUFallbackHandler(registry, smallQueueConfig, logger)

		// Re-exhaust GPU
		registry.mu.Lock()
		gpuDaemon.Capabilities.GPUs[0].ActiveEncodeSessions = 3
		registry.mu.Unlock()

		criteria := SelectionCriteria{
			RequiredEncoder: "h264_nvenc",
			RequireGPU:      true,
		}
		strategy := NewStrategyGPUAware()

		// Fill queue
		for i := 0; i < 2; i++ {
			go func() {
				smallHandler.HandleGPUExhausted(context.Background(), criteria, strategy)
			}()
		}
		time.Sleep(10 * time.Millisecond) // Let goroutines start

		// This should fail - queue full
		result := smallHandler.HandleGPUExhausted(context.Background(), criteria, strategy)

		assert.Error(t, result.Error)
		assert.Contains(t, result.Error.Error(), "queue full")
	})
}

func TestGPUFallbackHandler_SelectWithFallback(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := NewDaemonRegistry(logger)

	// Register CPU-only daemon
	cpuDaemon := &types.Daemon{
		ID:         "cpu-daemon",
		Name:       "CPU Daemon",
		State:      types.DaemonStateConnected,
		ActiveJobs: 0,
		Capabilities: &types.Capabilities{
			VideoEncoders:     []string{"libx264", "libx265"},
			MaxConcurrentJobs: 4,
		},
	}

	// Register GPU daemon with available sessions
	gpuDaemon := &types.Daemon{
		ID:         "gpu-daemon",
		Name:       "GPU Daemon",
		State:      types.DaemonStateConnected,
		ActiveJobs: 0,
		Capabilities: &types.Capabilities{
			VideoEncoders:     []string{"h264_nvenc", "libx264"},
			MaxConcurrentJobs: 8,
			GPUs: []types.GPUInfo{
				{
					Index:                0,
					Name:                 "RTX 3080",
					MaxEncodeSessions:    3,
					ActiveEncodeSessions: 1, // 2 available
				},
			},
		},
	}

	registry.mu.Lock()
	registry.daemons[cpuDaemon.ID] = cpuDaemon
	registry.daemons[gpuDaemon.ID] = gpuDaemon
	registry.mu.Unlock()

	config := DefaultGPUFallbackConfig()
	handler := NewGPUFallbackHandler(registry, config, logger)

	t.Run("selects_gpu_daemon_when_available", func(t *testing.T) {
		criteria := SelectionCriteria{
			RequiredEncoder: "h264_nvenc",
			RequireGPU:      true,
		}
		strategy := NewStrategyGPUAware()

		result := handler.SelectWithFallback(context.Background(), criteria, strategy)

		assert.NoError(t, result.Error)
		require.NotNil(t, result.Daemon)
		assert.Equal(t, types.DaemonID("gpu-daemon"), result.Daemon.ID)
		assert.False(t, result.UsedFallback)
	})

	t.Run("falls_back_when_gpu_exhausted", func(t *testing.T) {
		// Exhaust GPU
		registry.mu.Lock()
		gpuDaemon.Capabilities.GPUs[0].ActiveEncodeSessions = 3
		registry.mu.Unlock()

		criteria := SelectionCriteria{
			RequiredEncoder: "h264_nvenc",
			RequireGPU:      true,
		}
		strategy := NewStrategyGPUAware()

		result := handler.SelectWithFallback(context.Background(), criteria, strategy)

		assert.NoError(t, result.Error)
		require.NotNil(t, result.Daemon)
		assert.True(t, result.UsedFallback)
		assert.True(t, result.Daemon.Capabilities.HasEncoder("libx264"))
	})
}

func TestGPUFallbackHandler_QueueSize(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	registry := NewDaemonRegistry(logger)

	config := GPUFallbackConfig{
		Policy:       GPUPolicyQueue,
		QueueTimeout: time.Second,
		MaxQueueSize: 100,
	}
	handler := NewGPUFallbackHandler(registry, config, logger)

	assert.Equal(t, 0, handler.QueueSize())
	assert.Equal(t, GPUPolicyQueue, handler.Policy())
}
