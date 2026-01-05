package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
)

func TestGPUSessionTracker_NewGPUSessionTracker(t *testing.T) {
	t.Run("initializes_with_gpu_limits", func(t *testing.T) {
		gpus := []*proto.GPUInfo{
			{Index: 0, Name: "GeForce RTX 3080", MaxEncodeSessions: 5},
			{Index: 1, Name: "GeForce RTX 3070", MaxEncodeSessions: 5},
		}

		tracker := NewGPUSessionTracker(gpus)

		require.NotNil(t, tracker)
		assert.Equal(t, 5, tracker.maxEncodeSessions[0])
		assert.Equal(t, 5, tracker.maxEncodeSessions[1])
		// Decode sessions are 2x encode by default
		assert.Equal(t, 10, tracker.maxDecodeSessions[0])
		assert.Equal(t, 10, tracker.maxDecodeSessions[1])
	})

	t.Run("initializes_empty_session_counts", func(t *testing.T) {
		gpus := []*proto.GPUInfo{
			{Index: 0, Name: "Test GPU", MaxEncodeSessions: 3},
		}

		tracker := NewGPUSessionTracker(gpus)

		assert.Equal(t, 0, tracker.encodeSessions[0])
		assert.Equal(t, 0, tracker.decodeSessions[0])
	})

	t.Run("handles_empty_gpu_list", func(t *testing.T) {
		tracker := NewGPUSessionTracker([]*proto.GPUInfo{})

		require.NotNil(t, tracker)
		assert.Empty(t, tracker.maxEncodeSessions)
	})
}

func TestGPUSessionTracker_AcquireEncodeSession(t *testing.T) {
	t.Run("acquires_session_when_available", func(t *testing.T) {
		gpus := []*proto.GPUInfo{
			{Index: 0, Name: "Test GPU", MaxEncodeSessions: 3},
		}
		tracker := NewGPUSessionTracker(gpus)

		assert.True(t, tracker.AcquireEncodeSession(0))
		assert.Equal(t, 1, tracker.encodeSessions[0])

		assert.True(t, tracker.AcquireEncodeSession(0))
		assert.Equal(t, 2, tracker.encodeSessions[0])
	})

	t.Run("fails_when_max_reached", func(t *testing.T) {
		gpus := []*proto.GPUInfo{
			{Index: 0, Name: "Test GPU", MaxEncodeSessions: 2},
		}
		tracker := NewGPUSessionTracker(gpus)

		assert.True(t, tracker.AcquireEncodeSession(0))
		assert.True(t, tracker.AcquireEncodeSession(0))
		assert.False(t, tracker.AcquireEncodeSession(0)) // Should fail
		assert.Equal(t, 2, tracker.encodeSessions[0])
	})

	t.Run("unlimited_sessions_when_max_is_zero", func(t *testing.T) {
		gpus := []*proto.GPUInfo{
			{Index: 0, Name: "Tesla A100", MaxEncodeSessions: 0}, // Unlimited
		}
		tracker := NewGPUSessionTracker(gpus)

		// Should allow many sessions
		for range 100 {
			assert.True(t, tracker.AcquireEncodeSession(0))
		}
		assert.Equal(t, 100, tracker.encodeSessions[0])
	})
}

func TestGPUSessionTracker_ReleaseEncodeSession(t *testing.T) {
	t.Run("releases_session", func(t *testing.T) {
		gpus := []*proto.GPUInfo{
			{Index: 0, Name: "Test GPU", MaxEncodeSessions: 3},
		}
		tracker := NewGPUSessionTracker(gpus)

		tracker.AcquireEncodeSession(0)
		tracker.AcquireEncodeSession(0)
		assert.Equal(t, 2, tracker.encodeSessions[0])

		tracker.ReleaseEncodeSession(0)
		assert.Equal(t, 1, tracker.encodeSessions[0])
	})

	t.Run("does_not_go_negative", func(t *testing.T) {
		gpus := []*proto.GPUInfo{
			{Index: 0, Name: "Test GPU", MaxEncodeSessions: 3},
		}
		tracker := NewGPUSessionTracker(gpus)

		// Release without acquire
		tracker.ReleaseEncodeSession(0)
		tracker.ReleaseEncodeSession(0)
		assert.Equal(t, 0, tracker.encodeSessions[0])
	})

	t.Run("allows_new_session_after_release", func(t *testing.T) {
		gpus := []*proto.GPUInfo{
			{Index: 0, Name: "Test GPU", MaxEncodeSessions: 2},
		}
		tracker := NewGPUSessionTracker(gpus)

		// Fill up
		assert.True(t, tracker.AcquireEncodeSession(0))
		assert.True(t, tracker.AcquireEncodeSession(0))
		assert.False(t, tracker.AcquireEncodeSession(0))

		// Release one
		tracker.ReleaseEncodeSession(0)

		// Should be able to acquire again
		assert.True(t, tracker.AcquireEncodeSession(0))
	})
}

func TestGPUSessionTracker_DecodeSession(t *testing.T) {
	t.Run("acquire_and_release_decode_sessions", func(t *testing.T) {
		gpus := []*proto.GPUInfo{
			{Index: 0, Name: "Test GPU", MaxEncodeSessions: 3},
		}
		tracker := NewGPUSessionTracker(gpus)

		// Decode max is 2x encode (6 sessions)
		for range 6 {
			assert.True(t, tracker.AcquireDecodeSession(0))
		}
		assert.False(t, tracker.AcquireDecodeSession(0))

		tracker.ReleaseDecodeSession(0)
		assert.True(t, tracker.AcquireDecodeSession(0))
	})
}

func TestGPUSessionTracker_GetSessionCounts(t *testing.T) {
	t.Run("returns_correct_counts", func(t *testing.T) {
		gpus := []*proto.GPUInfo{
			{Index: 0, Name: "Test GPU", MaxEncodeSessions: 5},
		}
		tracker := NewGPUSessionTracker(gpus)

		tracker.AcquireEncodeSession(0)
		tracker.AcquireEncodeSession(0)
		tracker.AcquireDecodeSession(0)

		activeEncode, maxEncode, activeDecode, maxDecode := tracker.GetSessionCounts(0)

		assert.Equal(t, 2, activeEncode)
		assert.Equal(t, 5, maxEncode)
		assert.Equal(t, 1, activeDecode)
		assert.Equal(t, 10, maxDecode)
	})
}

func TestGPUSessionTracker_HasAvailableEncodeSessions(t *testing.T) {
	t.Run("returns_true_when_sessions_available", func(t *testing.T) {
		gpus := []*proto.GPUInfo{
			{Index: 0, Name: "Test GPU", MaxEncodeSessions: 3},
		}
		tracker := NewGPUSessionTracker(gpus)

		assert.True(t, tracker.HasAvailableEncodeSessions())

		tracker.AcquireEncodeSession(0)
		tracker.AcquireEncodeSession(0)
		assert.True(t, tracker.HasAvailableEncodeSessions())
	})

	t.Run("returns_false_when_all_exhausted", func(t *testing.T) {
		gpus := []*proto.GPUInfo{
			{Index: 0, Name: "Test GPU", MaxEncodeSessions: 2},
		}
		tracker := NewGPUSessionTracker(gpus)

		tracker.AcquireEncodeSession(0)
		tracker.AcquireEncodeSession(0)

		assert.False(t, tracker.HasAvailableEncodeSessions())
	})

	t.Run("returns_true_with_unlimited_sessions", func(t *testing.T) {
		gpus := []*proto.GPUInfo{
			{Index: 0, Name: "Tesla A100", MaxEncodeSessions: 0},
		}
		tracker := NewGPUSessionTracker(gpus)

		// Even with many sessions, unlimited should return true
		for range 50 {
			tracker.AcquireEncodeSession(0)
		}
		assert.True(t, tracker.HasAvailableEncodeSessions())
	})

	t.Run("checks_all_gpus", func(t *testing.T) {
		gpus := []*proto.GPUInfo{
			{Index: 0, Name: "GPU 0", MaxEncodeSessions: 2},
			{Index: 1, Name: "GPU 1", MaxEncodeSessions: 2},
		}
		tracker := NewGPUSessionTracker(gpus)

		// Exhaust GPU 0
		tracker.AcquireEncodeSession(0)
		tracker.AcquireEncodeSession(0)

		// GPU 1 still has capacity
		assert.True(t, tracker.HasAvailableEncodeSessions())

		// Exhaust GPU 1
		tracker.AcquireEncodeSession(1)
		tracker.AcquireEncodeSession(1)

		assert.False(t, tracker.HasAvailableEncodeSessions())
	})
}

func TestGPUSessionTracker_GetGPUWithAvailableSession(t *testing.T) {
	t.Run("returns_gpu_with_most_available", func(t *testing.T) {
		gpus := []*proto.GPUInfo{
			{Index: 0, Name: "GPU 0", MaxEncodeSessions: 4},
			{Index: 1, Name: "GPU 1", MaxEncodeSessions: 4},
		}
		tracker := NewGPUSessionTracker(gpus)

		// Use 3 sessions on GPU 0, 1 on GPU 1
		tracker.AcquireEncodeSession(0)
		tracker.AcquireEncodeSession(0)
		tracker.AcquireEncodeSession(0)
		tracker.AcquireEncodeSession(1)

		// GPU 1 has 3 available, GPU 0 has 1
		idx := tracker.GetGPUWithAvailableSession()
		assert.Equal(t, 1, idx)
	})

	t.Run("returns_minus_one_when_all_exhausted", func(t *testing.T) {
		gpus := []*proto.GPUInfo{
			{Index: 0, Name: "GPU 0", MaxEncodeSessions: 1},
		}
		tracker := NewGPUSessionTracker(gpus)

		tracker.AcquireEncodeSession(0)

		assert.Equal(t, -1, tracker.GetGPUWithAvailableSession())
	})

	t.Run("prefers_unlimited_gpu", func(t *testing.T) {
		gpus := []*proto.GPUInfo{
			{Index: 0, Name: "GeForce", MaxEncodeSessions: 5},
			{Index: 1, Name: "Tesla A100", MaxEncodeSessions: 0}, // Unlimited
		}
		tracker := NewGPUSessionTracker(gpus)

		idx := tracker.GetGPUWithAvailableSession()
		assert.Equal(t, 1, idx) // Should prefer unlimited
	})
}

func TestGPUSessionTracker_UpdateGPUStats(t *testing.T) {
	t.Run("updates_proto_stats", func(t *testing.T) {
		gpus := []*proto.GPUInfo{
			{Index: 0, Name: "Test GPU", MaxEncodeSessions: 5},
		}
		tracker := NewGPUSessionTracker(gpus)

		tracker.AcquireEncodeSession(0)
		tracker.AcquireEncodeSession(0)
		tracker.AcquireDecodeSession(0)

		stats := []*proto.GPUStats{
			{Index: 0},
		}

		tracker.UpdateGPUStats(stats)

		assert.Equal(t, int32(2), stats[0].ActiveEncodeSessions)
		assert.Equal(t, int32(1), stats[0].ActiveDecodeSessions)
		assert.Equal(t, int32(5), stats[0].MaxEncodeSessions)
		assert.Equal(t, int32(10), stats[0].MaxDecodeSessions)
	})
}

func TestGPUSessionTracker_UpdateGPUStatsTypes(t *testing.T) {
	t.Run("updates_types_stats", func(t *testing.T) {
		gpus := []*proto.GPUInfo{
			{Index: 0, Name: "Test GPU", MaxEncodeSessions: 5},
		}
		tracker := NewGPUSessionTracker(gpus)

		tracker.AcquireEncodeSession(0)
		tracker.AcquireDecodeSession(0)
		tracker.AcquireDecodeSession(0)

		stats := []types.GPUStats{
			{Index: 0},
		}

		tracker.UpdateGPUStatsTypes(stats)

		assert.Equal(t, 1, stats[0].ActiveEncodeSessions)
		assert.Equal(t, 2, stats[0].ActiveDecodeSessions)
		assert.Equal(t, 5, stats[0].MaxEncodeSessions)
		assert.Equal(t, 10, stats[0].MaxDecodeSessions)
	})
}

func TestGPUSessionTracker_Concurrency(t *testing.T) {
	t.Run("handles_concurrent_acquire_release", func(t *testing.T) {
		gpus := []*proto.GPUInfo{
			{Index: 0, Name: "Test GPU", MaxEncodeSessions: 100},
		}
		tracker := NewGPUSessionTracker(gpus)

		done := make(chan bool)

		// Spawn goroutines that acquire and release
		for range 10 {
			go func() {
				for range 100 {
					if tracker.AcquireEncodeSession(0) {
						tracker.ReleaseEncodeSession(0)
					}
				}
				done <- true
			}()
		}

		// Wait for all goroutines
		for range 10 {
			<-done
		}

		// Should end up at 0
		assert.Equal(t, 0, tracker.encodeSessions[0])
	})
}

func TestDetectGPUClassFromName(t *testing.T) {
	tests := []struct {
		name     string
		gpuName  string
		expected types.GPUClass
	}{
		// Datacenter GPUs
		{"Tesla V100", "Tesla V100-SXM2-16GB", types.GPUClassDatacenter},
		{"A100", "NVIDIA A100-SXM4-40GB", types.GPUClassDatacenter},
		{"H100", "NVIDIA H100 PCIe", types.GPUClassDatacenter},
		{"A40", "NVIDIA A40", types.GPUClassDatacenter},
		{"L40", "NVIDIA L40S", types.GPUClassDatacenter},
		{"V100", "NVIDIA V100", types.GPUClassDatacenter},

		// Professional GPUs
		{"Quadro RTX", "Quadro RTX 8000", types.GPUClassProfessional},
		{"RTX A5000", "NVIDIA RTX A5000", types.GPUClassProfessional},
		{"T4", "NVIDIA T4", types.GPUClassProfessional},
		{"T1000", "NVIDIA T1000", types.GPUClassProfessional},

		// Consumer GPUs
		{"GeForce RTX 3080", "NVIDIA GeForce RTX 3080", types.GPUClassConsumer},
		{"GTX 1080", "NVIDIA GeForce GTX 1080 Ti", types.GPUClassConsumer},
		{"RTX 4090", "NVIDIA RTX 4090", types.GPUClassConsumer},

		// Unknown
		{"Unknown GPU", "Some Random GPU", types.GPUClassUnknown},
		{"Empty", "", types.GPUClassUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectGPUClassFromName(tt.gpuName)
			assert.Equal(t, tt.expected, result, "GPU: %s", tt.gpuName)
		})
	}
}

func TestGetEnvMaxSessions(t *testing.T) {
	t.Run("returns_zero_when_not_set", func(t *testing.T) {
		t.Setenv("TVARR_GPU_MAX_SESSIONS", "")
		assert.Equal(t, 0, getEnvMaxSessions())
	})

	t.Run("returns_value_when_set", func(t *testing.T) {
		t.Setenv("TVARR_GPU_MAX_SESSIONS", "8")
		assert.Equal(t, 8, getEnvMaxSessions())
	})

	t.Run("returns_zero_for_invalid_value", func(t *testing.T) {
		t.Setenv("TVARR_GPU_MAX_SESSIONS", "invalid")
		assert.Equal(t, 0, getEnvMaxSessions())
	})

	t.Run("returns_zero_for_negative_value", func(t *testing.T) {
		t.Setenv("TVARR_GPU_MAX_SESSIONS", "-5")
		assert.Equal(t, 0, getEnvMaxSessions())
	})
}

func TestDetectGPUSessionLimits(t *testing.T) {
	t.Run("returns_empty_when_no_nvidia", func(t *testing.T) {
		// This test may succeed or fail depending on whether nvidia-smi is available
		// We just verify it doesn't panic and returns a map
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		limits, err := DetectGPUSessionLimits(ctx)
		assert.NoError(t, err)
		assert.NotNil(t, limits)
	})
}

func TestGPUSessionLimits_EnvOverride(t *testing.T) {
	t.Run("env_override_applies_to_all_gpus", func(t *testing.T) {
		t.Setenv("TVARR_GPU_MAX_SESSIONS", "10")

		gpus := []*proto.GPUInfo{
			{Index: 0, Name: "GeForce RTX 3080", MaxEncodeSessions: 5},
			{Index: 1, Name: "GeForce RTX 3070", MaxEncodeSessions: 5},
		}

		tracker := NewGPUSessionTracker(gpus)

		// All GPUs should have the env override value
		assert.Equal(t, 10, tracker.maxEncodeSessions[0])
		assert.Equal(t, 10, tracker.maxEncodeSessions[1])
	})
}
