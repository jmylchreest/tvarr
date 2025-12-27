package contract

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gproto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
)

// TestHeartbeatRequest_ValidatesRequiredFields tests that HeartbeatRequest
// can be properly serialized with various field combinations.
func TestHeartbeatRequest_ValidatesRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		request *proto.HeartbeatRequest
	}{
		{
			name: "minimal_request",
			request: &proto.HeartbeatRequest{
				DaemonId: "test-daemon-001",
			},
		},
		{
			name: "request_with_system_stats",
			request: &proto.HeartbeatRequest{
				DaemonId: "test-daemon-002",
				SystemStats: &proto.SystemStats{
					Hostname:      "worker-01",
					Os:            "linux",
					Arch:          "amd64",
					CpuCores:      8,
					CpuPercent:    45.5,
					MemoryPercent: 62.3,
				},
			},
		},
		{
			name: "request_with_active_jobs",
			request: &proto.HeartbeatRequest{
				DaemonId: "test-daemon-003",
				ActiveJobs: []*proto.JobStatus{
					{
						JobId:       "job-001",
						SessionId:   "session-abc",
						ChannelName: "Test Channel",
						RunningTime: durationpb.New(5 * time.Minute),
						Stats: &proto.TranscodeStats{
							SamplesIn:  10000,
							SamplesOut: 9950,
							BytesIn:    5 * 1024 * 1024,
							BytesOut:   3 * 1024 * 1024,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify serialization works
			data, err := gproto.Marshal(tt.request)
			require.NoError(t, err)
			require.NotEmpty(t, data)

			// Verify deserialization
			decoded := &proto.HeartbeatRequest{}
			err = gproto.Unmarshal(data, decoded)
			require.NoError(t, err)

			assert.Equal(t, tt.request.DaemonId, decoded.DaemonId)
		})
	}
}

// TestHeartbeatRequest_SystemStatsSerialization tests comprehensive system
// stats serialization.
func TestHeartbeatRequest_SystemStatsSerialization(t *testing.T) {
	original := &proto.HeartbeatRequest{
		DaemonId: "test-daemon",
		SystemStats: &proto.SystemStats{
			// Host info
			Hostname:      "gpu-worker-01.local",
			Os:            "linux",
			Arch:          "amd64",
			UptimeSeconds: 86400 * 7, // 1 week

			// CPU
			CpuCores:    16,
			CpuPercent:  72.5,
			CpuPerCore:  []float64{80.0, 75.0, 70.0, 65.0, 80.0, 75.0, 70.0, 65.0, 80.0, 75.0, 70.0, 65.0, 80.0, 75.0, 70.0, 65.0},
			LoadAvg_1M:  8.5,
			LoadAvg_5M:  7.2,
			LoadAvg_15M: 6.8,

			// Memory
			MemoryTotalBytes:     64 * 1024 * 1024 * 1024, // 64GB
			MemoryUsedBytes:      48 * 1024 * 1024 * 1024, // 48GB
			MemoryAvailableBytes: 16 * 1024 * 1024 * 1024, // 16GB
			MemoryPercent:        75.0,
			SwapTotalBytes:       32 * 1024 * 1024 * 1024, // 32GB
			SwapUsedBytes:        2 * 1024 * 1024 * 1024,  // 2GB

			// Disk
			DiskTotalBytes:     2 * 1024 * 1024 * 1024 * 1024, // 2TB
			DiskUsedBytes:      1 * 1024 * 1024 * 1024 * 1024, // 1TB
			DiskAvailableBytes: 1 * 1024 * 1024 * 1024 * 1024, // 1TB
			DiskPercent:        50.0,

			// Network
			NetworkBytesSent:   100 * 1024 * 1024 * 1024, // 100GB
			NetworkBytesRecv:   200 * 1024 * 1024 * 1024, // 200GB
			NetworkSendRateBps: 125 * 1024 * 1024,        // 125 MB/s
			NetworkRecvRateBps: 250 * 1024 * 1024,        // 250 MB/s

			// GPU stats
			Gpus: []*proto.GPUStats{
				{
					Index:                0,
					Name:                 "NVIDIA GeForce RTX 3080",
					DriverVersion:        "535.183.01",
					UtilizationPercent:   85.0,
					MemoryPercent:        45.0,
					MemoryTotalBytes:     10 * 1024 * 1024 * 1024,
					MemoryUsedBytes:      4.5 * 1024 * 1024 * 1024,
					TemperatureCelsius:   72,
					PowerWatts:           280,
					EncoderUtilization:   90.0,
					DecoderUtilization:   30.0,
					MaxEncodeSessions:    3,
					ActiveEncodeSessions: 2,
					MaxDecodeSessions:    8,
					ActiveDecodeSessions: 3,
					GpuClass:             proto.GPUClass_GPU_CLASS_CONSUMER,
				},
			},

			// Linux PSI pressure stats
			CpuPressure: &proto.PressureStats{
				Avg10:   1.5,
				Avg60:   2.0,
				Avg300:  1.8,
				TotalUs: 50000000,
			},
			MemoryPressure: &proto.PressureStats{
				Avg10:   0.5,
				Avg60:   0.3,
				Avg300:  0.4,
				TotalUs: 10000000,
			},
			IoPressure: &proto.PressureStats{
				Avg10:   3.0,
				Avg60:   2.5,
				Avg300:  2.8,
				TotalUs: 80000000,
			},
		},
	}

	// Marshal
	data, err := gproto.Marshal(original)
	require.NoError(t, err)

	// Unmarshal
	decoded := &proto.HeartbeatRequest{}
	err = gproto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// Verify system stats
	stats := decoded.SystemStats
	require.NotNil(t, stats)

	assert.Equal(t, "gpu-worker-01.local", stats.Hostname)
	assert.Equal(t, "linux", stats.Os)
	assert.Equal(t, int32(16), stats.CpuCores)
	assert.InDelta(t, 72.5, stats.CpuPercent, 0.01)
	assert.Len(t, stats.CpuPerCore, 16)
	assert.InDelta(t, 75.0, stats.MemoryPercent, 0.01)

	// Verify GPU stats
	require.Len(t, stats.Gpus, 1)
	gpu := stats.Gpus[0]
	assert.Equal(t, "NVIDIA GeForce RTX 3080", gpu.Name)
	assert.InDelta(t, 85.0, gpu.UtilizationPercent, 0.01)
	assert.Equal(t, int32(72), gpu.TemperatureCelsius)
	assert.Equal(t, int32(3), gpu.MaxEncodeSessions)
	assert.Equal(t, int32(2), gpu.ActiveEncodeSessions)
	assert.Equal(t, proto.GPUClass_GPU_CLASS_CONSUMER, gpu.GpuClass)

	// Verify pressure stats
	require.NotNil(t, stats.CpuPressure)
	assert.InDelta(t, 1.5, stats.CpuPressure.Avg10, 0.01)
	require.NotNil(t, stats.MemoryPressure)
	require.NotNil(t, stats.IoPressure)
}

// TestHeartbeatRequest_JobStatusSerialization tests job status serialization
// in heartbeat requests.
func TestHeartbeatRequest_JobStatusSerialization(t *testing.T) {
	original := &proto.HeartbeatRequest{
		DaemonId: "test-daemon",
		ActiveJobs: []*proto.JobStatus{
			{
				JobId:       "job-001",
				SessionId:   "session-abc-123",
				ChannelName: "HBO Max",
				RunningTime: durationpb.New(15*time.Minute + 30*time.Second),
				Stats: &proto.TranscodeStats{
					SamplesIn:     450000,
					SamplesOut:    449500,
					BytesIn:       150 * 1024 * 1024,
					BytesOut:      75 * 1024 * 1024,
					EncodingSpeed: 2.5, // 2.5x realtime
					CpuPercent:    180.0,
					MemoryMb:      512.0,
					FfmpegPid:     12345,
					RunningTime:   durationpb.New(15*time.Minute + 30*time.Second),
				},
			},
			{
				JobId:       "job-002",
				SessionId:   "session-def-456",
				ChannelName: "ESPN",
				RunningTime: durationpb.New(5 * time.Minute),
				Stats: &proto.TranscodeStats{
					SamplesIn:  150000,
					SamplesOut: 149800,
					BytesIn:    50 * 1024 * 1024,
					BytesOut:   25 * 1024 * 1024,
				},
			},
		},
	}

	// Marshal
	data, err := gproto.Marshal(original)
	require.NoError(t, err)

	// Unmarshal
	decoded := &proto.HeartbeatRequest{}
	err = gproto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// Verify jobs
	require.Len(t, decoded.ActiveJobs, 2)

	job1 := decoded.ActiveJobs[0]
	assert.Equal(t, "job-001", job1.JobId)
	assert.Equal(t, "HBO Max", job1.ChannelName)
	assert.Equal(t, 15*time.Minute+30*time.Second, job1.RunningTime.AsDuration())
	require.NotNil(t, job1.Stats)
	assert.Equal(t, uint64(450000), job1.Stats.SamplesIn)
	assert.InDelta(t, 2.5, job1.Stats.EncodingSpeed, 0.01)
	assert.Equal(t, int32(12345), job1.Stats.FfmpegPid)

	job2 := decoded.ActiveJobs[1]
	assert.Equal(t, "job-002", job2.JobId)
	assert.Equal(t, "ESPN", job2.ChannelName)
}

// TestHeartbeatResponse_Success tests successful heartbeat response.
func TestHeartbeatResponse_Success(t *testing.T) {
	response := &proto.HeartbeatResponse{
		Success: true,
	}

	data, err := gproto.Marshal(response)
	require.NoError(t, err)

	decoded := &proto.HeartbeatResponse{}
	err = gproto.Unmarshal(data, decoded)
	require.NoError(t, err)

	assert.True(t, decoded.Success)
	assert.Empty(t, decoded.Commands)
}

// TestHeartbeatResponse_WithCommands tests heartbeat response with
// coordinator commands.
func TestHeartbeatResponse_WithCommands(t *testing.T) {
	response := &proto.HeartbeatResponse{
		Success: true,
		Commands: []*proto.DaemonCommand{
			{
				Type: proto.DaemonCommand_DRAIN,
			},
			{
				Type:  proto.DaemonCommand_CANCEL_JOB,
				JobId: "job-001",
			},
			{
				Type:    proto.DaemonCommand_UPDATE_CONFIG,
				Payload: []byte(`{"max_concurrent_jobs": 2}`),
			},
		},
	}

	data, err := gproto.Marshal(response)
	require.NoError(t, err)

	decoded := &proto.HeartbeatResponse{}
	err = gproto.Unmarshal(data, decoded)
	require.NoError(t, err)

	assert.True(t, decoded.Success)
	require.Len(t, decoded.Commands, 3)

	assert.Equal(t, proto.DaemonCommand_DRAIN, decoded.Commands[0].Type)
	assert.Equal(t, proto.DaemonCommand_CANCEL_JOB, decoded.Commands[1].Type)
	assert.Equal(t, "job-001", decoded.Commands[1].JobId)
	assert.Equal(t, proto.DaemonCommand_UPDATE_CONFIG, decoded.Commands[2].Type)
	assert.JSONEq(t, `{"max_concurrent_jobs": 2}`, string(decoded.Commands[2].Payload))
}

// TestHeartbeatResponse_HeartbeatInterval tests that heartbeat interval
// is properly communicated in registration response.
func TestRegisterResponse_HeartbeatInterval(t *testing.T) {
	response := &proto.RegisterResponse{
		Success:           true,
		HeartbeatInterval: durationpb.New(5 * time.Second),
	}

	data, err := gproto.Marshal(response)
	require.NoError(t, err)

	decoded := &proto.RegisterResponse{}
	err = gproto.Unmarshal(data, decoded)
	require.NoError(t, err)

	assert.True(t, decoded.Success)
	require.NotNil(t, decoded.HeartbeatInterval)
	assert.Equal(t, 5*time.Second, decoded.HeartbeatInterval.AsDuration())
}

// TestUnregisterRequest_Serialization tests unregister request message.
func TestUnregisterRequest_Serialization(t *testing.T) {
	request := &proto.UnregisterRequest{
		DaemonId: "test-daemon-001",
		Reason:   "graceful_shutdown",
	}

	data, err := gproto.Marshal(request)
	require.NoError(t, err)

	decoded := &proto.UnregisterRequest{}
	err = gproto.Unmarshal(data, decoded)
	require.NoError(t, err)

	assert.Equal(t, "test-daemon-001", decoded.DaemonId)
	assert.Equal(t, "graceful_shutdown", decoded.Reason)
}

// TestUnregisterResponse_Serialization tests unregister response message.
func TestUnregisterResponse_Serialization(t *testing.T) {
	response := &proto.UnregisterResponse{
		Success: true,
	}

	data, err := gproto.Marshal(response)
	require.NoError(t, err)

	decoded := &proto.UnregisterResponse{}
	err = gproto.Unmarshal(data, decoded)
	require.NoError(t, err)

	assert.True(t, decoded.Success)
}
