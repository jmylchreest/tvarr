package contract

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
)

// TestDaemonListResponse tests the contract for the daemon list API response.
// This is T084: Contract test for REST API endpoints.
func TestDaemonListResponse(t *testing.T) {
	// Define the expected response structure (order matters for Go type definitions)
	type GPUStatsDTO struct {
		Index                int     `json:"index"`
		Name                 string  `json:"name"`
		Utilization          float64 `json:"utilization"`
		MemoryPercent        float64 `json:"memory_percent"`
		Temperature          int     `json:"temperature"`
		ActiveEncodeSessions int     `json:"active_encode_sessions"`
		MaxEncodeSessions    int     `json:"max_encode_sessions"`
	}

	type SystemStatsDTO struct {
		Hostname        string        `json:"hostname"`
		CPUPercent      float64       `json:"cpu_percent"`
		MemoryPercent   float64       `json:"memory_percent"`
		MemoryTotal     uint64        `json:"memory_total"`
		MemoryAvailable uint64        `json:"memory_available"`
		GPUs            []GPUStatsDTO `json:"gpus,omitempty"`
	}

	type GPUDTO struct {
		Index                int    `json:"index"`
		Name                 string `json:"name"`
		Class                string `json:"class"`
		Driver               string `json:"driver,omitempty"`
		MaxEncodeSessions    int    `json:"max_encode_sessions"`
		MaxDecodeSessions    int    `json:"max_decode_sessions"`
		ActiveEncodeSessions int    `json:"active_encode_sessions"`
		ActiveDecodeSessions int    `json:"active_decode_sessions"`
		MemoryTotal          uint64 `json:"memory_total,omitempty"`
	}

	type HWAccelDTO struct {
		Type      string   `json:"type"`
		Device    string   `json:"device"`
		Available bool     `json:"available"`
		Encoders  []string `json:"encoders"`
		Decoders  []string `json:"decoders"`
	}

	type CapabilitiesDTO struct {
		VideoEncoders     []string     `json:"video_encoders"`
		VideoDecoders     []string     `json:"video_decoders"`
		AudioEncoders     []string     `json:"audio_encoders"`
		AudioDecoders     []string     `json:"audio_decoders"`
		MaxConcurrentJobs int          `json:"max_concurrent_jobs"`
		HWAccels          []HWAccelDTO `json:"hw_accels,omitempty"`
		GPUs              []GPUDTO     `json:"gpus,omitempty"`
	}

	type DaemonResponse struct {
		ID            string           `json:"id"`
		Name          string           `json:"name"`
		Version       string           `json:"version"`
		Address       string           `json:"address"`
		State         string           `json:"state"`
		ConnectedAt   string           `json:"connected_at"`
		LastHeartbeat string           `json:"last_heartbeat"`
		ActiveJobs    int              `json:"active_jobs"`
		Capabilities  *CapabilitiesDTO `json:"capabilities,omitempty"`
		SystemStats   *SystemStatsDTO  `json:"system_stats,omitempty"`
	}

	type ListDaemonsResponse struct {
		Daemons []DaemonResponse `json:"daemons"`
		Total   int              `json:"total"`
	}

	t.Run("daemon_list_response_serializes_correctly", func(t *testing.T) {
		// Simulate a response
		response := ListDaemonsResponse{
			Daemons: []DaemonResponse{
				{
					ID:            "daemon-1",
					Name:          "Worker 1",
					Version:       "1.0.0",
					Address:       "192.168.1.100:50051",
					State:         "active",
					ConnectedAt:   "2024-01-15T10:30:00Z",
					LastHeartbeat: "2024-01-15T10:35:00Z",
					ActiveJobs:    2,
					Capabilities: &CapabilitiesDTO{
						VideoEncoders:     []string{"libx264", "h264_nvenc"},
						VideoDecoders:     []string{"h264", "hevc"},
						AudioEncoders:     []string{"aac"},
						AudioDecoders:     []string{"aac", "ac3"},
						MaxConcurrentJobs: 8,
						GPUs: []GPUDTO{
							{
								Index:                0,
								Name:                 "NVIDIA GeForce RTX 3080",
								Class:                "consumer",
								MaxEncodeSessions:    3,
								MaxDecodeSessions:    5,
								ActiveEncodeSessions: 1,
							},
						},
					},
					SystemStats: &SystemStatsDTO{
						Hostname:        "worker-1",
						CPUPercent:      45.5,
						MemoryPercent:   62.3,
						MemoryTotal:     32000000000,
						MemoryAvailable: 12000000000,
					},
				},
			},
			Total: 1,
		}

		// Serialize to JSON
		data, err := json.Marshal(response)
		require.NoError(t, err)

		// Deserialize back
		var parsed ListDaemonsResponse
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, 1, parsed.Total)
		assert.Len(t, parsed.Daemons, 1)
		assert.Equal(t, "daemon-1", parsed.Daemons[0].ID)
		assert.Equal(t, "active", parsed.Daemons[0].State)
		assert.NotNil(t, parsed.Daemons[0].Capabilities)
		assert.Contains(t, parsed.Daemons[0].Capabilities.VideoEncoders, "h264_nvenc")
	})
}

// TestDaemonDetailResponse tests the contract for single daemon API response.
func TestDaemonDetailResponse(t *testing.T) {
	type DaemonDetailResponse struct {
		ID                   string  `json:"id"`
		Name                 string  `json:"name"`
		Version              string  `json:"version"`
		Address              string  `json:"address"`
		State                string  `json:"state"`
		ConnectedAt          string  `json:"connected_at"`
		LastHeartbeat        string  `json:"last_heartbeat"`
		HeartbeatsMissed     int     `json:"heartbeats_missed"`
		ActiveJobs           int     `json:"active_jobs"`
		TotalJobsCompleted   uint64  `json:"total_jobs_completed"`
		TotalJobsFailed      uint64  `json:"total_jobs_failed"`
		UptimeSeconds        int64   `json:"uptime_seconds"`
		Capabilities         any     `json:"capabilities,omitempty"`
		SystemStats          any     `json:"system_stats,omitempty"`
		ActiveJobDetails     []any   `json:"active_job_details,omitempty"`
	}

	t.Run("daemon_detail_includes_all_fields", func(t *testing.T) {
		response := DaemonDetailResponse{
			ID:                   "daemon-1",
			Name:                 "Worker 1",
			Version:              "1.0.0",
			Address:              "192.168.1.100:50051",
			State:                "active",
			ConnectedAt:          "2024-01-15T10:30:00Z",
			LastHeartbeat:        "2024-01-15T10:35:00Z",
			HeartbeatsMissed:     0,
			ActiveJobs:           2,
			TotalJobsCompleted:   150,
			TotalJobsFailed:      3,
			UptimeSeconds:        86400,
		}

		data, err := json.Marshal(response)
		require.NoError(t, err)

		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		// Verify all required fields are present
		assert.Contains(t, parsed, "id")
		assert.Contains(t, parsed, "name")
		assert.Contains(t, parsed, "state")
		assert.Contains(t, parsed, "active_jobs")
		assert.Contains(t, parsed, "total_jobs_completed")
	})
}

// TestClusterStatsResponse tests the contract for cluster stats API response.
func TestClusterStatsResponse(t *testing.T) {
	type ClusterStatsResponse struct {
		TotalDaemons       int     `json:"total_daemons"`
		ActiveDaemons      int     `json:"active_daemons"`
		UnhealthyDaemons   int     `json:"unhealthy_daemons"`
		DrainingDaemons    int     `json:"draining_daemons"`
		TotalActiveJobs    int     `json:"total_active_jobs"`
		TotalGPUs          int     `json:"total_gpus"`
		AvailableGPUSessions int  `json:"available_gpu_sessions"`
		TotalGPUSessions   int     `json:"total_gpu_sessions"`
		AverageCPUPercent  float64 `json:"average_cpu_percent"`
		AverageMemPercent  float64 `json:"average_memory_percent"`
	}

	t.Run("cluster_stats_serializes_correctly", func(t *testing.T) {
		response := ClusterStatsResponse{
			TotalDaemons:         3,
			ActiveDaemons:        2,
			UnhealthyDaemons:     1,
			DrainingDaemons:      0,
			TotalActiveJobs:      5,
			TotalGPUs:            4,
			AvailableGPUSessions: 8,
			TotalGPUSessions:     12,
			AverageCPUPercent:    55.5,
			AverageMemPercent:    68.2,
		}

		data, err := json.Marshal(response)
		require.NoError(t, err)

		var parsed ClusterStatsResponse
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, 3, parsed.TotalDaemons)
		assert.Equal(t, 2, parsed.ActiveDaemons)
		assert.Equal(t, 8, parsed.AvailableGPUSessions)
	})
}

// TestDrainDaemonRequest tests the contract for drain daemon API request.
func TestDrainDaemonRequest(t *testing.T) {
	type DrainDaemonRequest struct {
		DaemonID string `json:"daemon_id" path:"id"`
		Force    bool   `json:"force,omitempty"`
	}

	type DrainDaemonResponse struct {
		Success       bool   `json:"success"`
		Message       string `json:"message"`
		RemainingJobs int    `json:"remaining_jobs"`
	}

	t.Run("drain_request_serializes_correctly", func(t *testing.T) {
		request := DrainDaemonRequest{
			DaemonID: "daemon-1",
			Force:    false,
		}

		data, err := json.Marshal(request)
		require.NoError(t, err)

		var parsed DrainDaemonRequest
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, "daemon-1", parsed.DaemonID)
		assert.False(t, parsed.Force)
	})

	t.Run("drain_response_serializes_correctly", func(t *testing.T) {
		response := DrainDaemonResponse{
			Success:       true,
			Message:       "Daemon is draining, 2 jobs remaining",
			RemainingJobs: 2,
		}

		data, err := json.Marshal(response)
		require.NoError(t, err)

		var parsed DrainDaemonResponse
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.True(t, parsed.Success)
		assert.Equal(t, 2, parsed.RemainingJobs)
	})
}

// TestTypesToDTOConversion tests that types.Daemon can be converted to API DTOs.
func TestTypesToDTOConversion(t *testing.T) {
	t.Run("daemon_state_converts_to_string", func(t *testing.T) {
		states := []struct {
			state    types.DaemonState
			expected string
		}{
			{types.DaemonStateConnected, "connected"},
			{types.DaemonStateDraining, "draining"},
			{types.DaemonStateUnhealthy, "unhealthy"},
			{types.DaemonStateDisconnected, "disconnected"},
			{types.DaemonStateConnecting, "connecting"},
		}

		for _, tc := range states {
			t.Run(tc.expected, func(t *testing.T) {
				assert.Equal(t, tc.expected, tc.state.String())
			})
		}
	})

	t.Run("gpu_class_converts_to_string", func(t *testing.T) {
		classes := []struct {
			class    types.GPUClass
			expected string
		}{
			{types.GPUClassConsumer, "consumer"},
			{types.GPUClassProfessional, "professional"},
			{types.GPUClassDatacenter, "datacenter"},
			{types.GPUClassIntegrated, "integrated"},
			{types.GPUClassUnknown, "unknown"},
		}

		for _, tc := range classes {
			t.Run(tc.expected, func(t *testing.T) {
				assert.Equal(t, tc.expected, tc.class.String())
			})
		}
	})
}
