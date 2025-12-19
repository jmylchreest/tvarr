package proto

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestRegisterRequest_RoundTrip(t *testing.T) {
	req := &RegisterRequest{
		DaemonId:   "daemon-001",
		DaemonName: "GPU Worker 1",
		Version:    "1.0.0",
		AuthToken:  "secret-token",
		Capabilities: &Capabilities{
			VideoEncoders:     []string{"libx264", "h264_nvenc", "hevc_nvenc"},
			VideoDecoders:     []string{"h264", "hevc", "h264_cuvid"},
			AudioEncoders:     []string{"aac", "libopus"},
			AudioDecoders:     []string{"aac", "ac3", "eac3"},
			MaxConcurrentJobs: 4,
			HwAccels: []*HWAccelInfo{
				{
					Type:      "cuda",
					Device:    "GPU 0",
					Available: true,
					Encoders:  []string{"h264_nvenc", "hevc_nvenc"},
					Decoders:  []string{"h264_cuvid", "hevc_cuvid"},
				},
			},
			Gpus: []*GPUInfo{
				{
					Index:              0,
					Name:               "NVIDIA GeForce RTX 3080",
					GpuClass:           GPUClass_GPU_CLASS_CONSUMER,
					DriverVersion:      "535.129.03",
					MaxEncodeSessions:  5,
					MaxDecodeSessions:  10,
					MemoryTotalBytes:   10 * 1024 * 1024 * 1024,
				},
			},
		},
	}

	// Serialize
	data, err := proto.Marshal(req)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Deserialize
	decoded := &RegisterRequest{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// Verify
	assert.Equal(t, req.DaemonId, decoded.DaemonId)
	assert.Equal(t, req.DaemonName, decoded.DaemonName)
	assert.Equal(t, req.Version, decoded.Version)
	assert.Equal(t, req.AuthToken, decoded.AuthToken)
	require.NotNil(t, decoded.Capabilities)
	assert.Equal(t, req.Capabilities.MaxConcurrentJobs, decoded.Capabilities.MaxConcurrentJobs)
	assert.Equal(t, req.Capabilities.VideoEncoders, decoded.Capabilities.VideoEncoders)
	require.Len(t, decoded.Capabilities.HwAccels, 1)
	assert.Equal(t, req.Capabilities.HwAccels[0].Type, decoded.Capabilities.HwAccels[0].Type)
	require.Len(t, decoded.Capabilities.Gpus, 1)
	assert.Equal(t, req.Capabilities.Gpus[0].Name, decoded.Capabilities.Gpus[0].Name)
	assert.Equal(t, req.Capabilities.Gpus[0].GpuClass, decoded.Capabilities.Gpus[0].GpuClass)
}

func TestRegisterResponse_RoundTrip(t *testing.T) {
	resp := &RegisterResponse{
		Success:            true,
		HeartbeatInterval:  durationpb.New(5 * time.Second),
		CoordinatorVersion: "1.0.0",
	}

	// Serialize
	data, err := proto.Marshal(resp)
	require.NoError(t, err)

	// Deserialize
	decoded := &RegisterResponse{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// Verify
	assert.True(t, decoded.Success)
	assert.Equal(t, 5*time.Second, decoded.HeartbeatInterval.AsDuration())
	assert.Equal(t, "1.0.0", decoded.CoordinatorVersion)
}

func TestHeartbeatRequest_RoundTrip(t *testing.T) {
	req := &HeartbeatRequest{
		DaemonId: "daemon-001",
		SystemStats: &SystemStats{
			Hostname:            "worker-1",
			Os:                  "linux",
			Arch:                "amd64",
			CpuCores:            8,
			CpuPercent:          45.5,
			LoadAvg_1M:          2.5,
			LoadAvg_5M:          2.0,
			LoadAvg_15M:         1.5,
			MemoryTotalBytes:    16 * 1024 * 1024 * 1024,
			MemoryUsedBytes:     8 * 1024 * 1024 * 1024,
			MemoryPercent:       50.0,
			Gpus: []*GPUStats{
				{
					Index:                0,
					Name:                 "NVIDIA GeForce RTX 3080",
					UtilizationPercent:   75.0,
					MemoryPercent:        40.0,
					TemperatureCelsius:   65,
					MaxEncodeSessions:    5,
					ActiveEncodeSessions: 2,
				},
			},
		},
		ActiveJobs: []*JobStatus{
			{
				JobId:       "job-001",
				SessionId:   "session-001",
				ChannelName: "Test Channel",
				RunningTime: durationpb.New(30 * time.Second),
				Stats: &TranscodeStats{
					SamplesIn:     1000,
					SamplesOut:    1000,
					BytesIn:       5000000,
					BytesOut:      2500000,
					EncodingSpeed: 1.2,
				},
			},
		},
	}

	// Serialize
	data, err := proto.Marshal(req)
	require.NoError(t, err)

	// Deserialize
	decoded := &HeartbeatRequest{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// Verify
	assert.Equal(t, req.DaemonId, decoded.DaemonId)
	require.NotNil(t, decoded.SystemStats)
	assert.Equal(t, req.SystemStats.Hostname, decoded.SystemStats.Hostname)
	assert.Equal(t, req.SystemStats.CpuPercent, decoded.SystemStats.CpuPercent)
	require.Len(t, decoded.SystemStats.Gpus, 1)
	assert.Equal(t, req.SystemStats.Gpus[0].UtilizationPercent, decoded.SystemStats.Gpus[0].UtilizationPercent)
	require.Len(t, decoded.ActiveJobs, 1)
	assert.Equal(t, req.ActiveJobs[0].JobId, decoded.ActiveJobs[0].JobId)
	assert.Equal(t, req.ActiveJobs[0].Stats.EncodingSpeed, decoded.ActiveJobs[0].Stats.EncodingSpeed)
}

func TestTranscodeStart_RoundTrip(t *testing.T) {
	start := &TranscodeStart{
		JobId:              "job-001",
		SessionId:          "session-001",
		ChannelId:          "channel-001",
		ChannelName:        "Test Channel",
		SourceVideoCodec:   "h264",
		SourceAudioCodec:   "aac",
		VideoInitData:      []byte{0x00, 0x00, 0x00, 0x01}, // SPS/PPS placeholder
		AudioInitData:      []byte{0x11, 0x90},              // AudioSpecificConfig placeholder
		TargetVideoCodec:   "hevc",
		TargetAudioCodec:   "aac",
		VideoEncoder:       "hevc_nvenc",
		AudioEncoder:       "aac",
		VideoBitrateKbps:   5000,
		AudioBitrateKbps:   192,
		VideoPreset:        "fast",
		VideoCrf:           23,
		PreferredHwAccel:   "cuda",
		ScaleWidth:         1920,
		ScaleHeight:        1080,
		ExtraOptions: map[string]string{
			"profile": "main",
		},
	}

	// Serialize
	data, err := proto.Marshal(start)
	require.NoError(t, err)

	// Deserialize
	decoded := &TranscodeStart{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// Verify
	assert.Equal(t, start.JobId, decoded.JobId)
	assert.Equal(t, start.SourceVideoCodec, decoded.SourceVideoCodec)
	assert.Equal(t, start.VideoEncoder, decoded.VideoEncoder)
	assert.Equal(t, start.VideoBitrateKbps, decoded.VideoBitrateKbps)
	assert.Equal(t, start.VideoInitData, decoded.VideoInitData)
	assert.Equal(t, start.ExtraOptions["profile"], decoded.ExtraOptions["profile"])
}

func TestESSampleBatch_RoundTrip(t *testing.T) {
	batch := &ESSampleBatch{
		VideoSamples: []*ESSample{
			{
				Pts:        90000,
				Dts:        90000,
				Data:       []byte{0x00, 0x00, 0x00, 0x01, 0x65}, // IDR NAL unit start
				IsKeyframe: true,
				Sequence:   1,
			},
			{
				Pts:        93003,
				Dts:        93003,
				Data:       []byte{0x00, 0x00, 0x00, 0x01, 0x41}, // Non-IDR NAL unit
				IsKeyframe: false,
				Sequence:   2,
			},
		},
		AudioSamples: []*ESSample{
			{
				Pts:        90000,
				Dts:        90000,
				Data:       []byte{0xFF, 0xF1}, // ADTS header start
				IsKeyframe: true,
				Sequence:   1,
			},
		},
		IsSource:      true,
		BatchSequence: 100,
	}

	// Serialize
	data, err := proto.Marshal(batch)
	require.NoError(t, err)

	// Deserialize
	decoded := &ESSampleBatch{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// Verify
	assert.True(t, decoded.IsSource)
	assert.Equal(t, uint64(100), decoded.BatchSequence)
	require.Len(t, decoded.VideoSamples, 2)
	assert.Equal(t, batch.VideoSamples[0].Pts, decoded.VideoSamples[0].Pts)
	assert.True(t, decoded.VideoSamples[0].IsKeyframe)
	assert.Equal(t, batch.VideoSamples[0].Data, decoded.VideoSamples[0].Data)
	require.Len(t, decoded.AudioSamples, 1)
	assert.Equal(t, batch.AudioSamples[0].Data, decoded.AudioSamples[0].Data)
}

func TestTranscodeError_RoundTrip(t *testing.T) {
	err := &TranscodeError{
		Code:         TranscodeError_SESSION_LIMIT_REACHED,
		Message:      "GPU encode session limit exceeded",
		FfmpegStderr: "Error initializing encoder: nvenc session limit reached",
		Recoverable:  true,
	}

	// Serialize
	data, errM := proto.Marshal(err)
	require.NoError(t, errM)

	// Deserialize
	decoded := &TranscodeError{}
	errM = proto.Unmarshal(data, decoded)
	require.NoError(t, errM)

	// Verify
	assert.Equal(t, TranscodeError_SESSION_LIMIT_REACHED, decoded.Code)
	assert.Equal(t, err.Message, decoded.Message)
	assert.Equal(t, err.FfmpegStderr, decoded.FfmpegStderr)
	assert.True(t, decoded.Recoverable)
}

func TestGPUClass_Values(t *testing.T) {
	// Verify enum values match expected
	assert.Equal(t, GPUClass(0), GPUClass_GPU_CLASS_UNKNOWN)
	assert.Equal(t, GPUClass(1), GPUClass_GPU_CLASS_CONSUMER)
	assert.Equal(t, GPUClass(2), GPUClass_GPU_CLASS_PROFESSIONAL)
	assert.Equal(t, GPUClass(3), GPUClass_GPU_CLASS_DATACENTER)
	assert.Equal(t, GPUClass(4), GPUClass_GPU_CLASS_INTEGRATED)
}

func TestTranscodeMessage_StartPayload(t *testing.T) {
	msg := &TranscodeMessage{
		Payload: &TranscodeMessage_Start{
			Start: &TranscodeStart{
				JobId:       "job-001",
				ChannelName: "Test Channel",
			},
		},
	}

	// Serialize
	data, err := proto.Marshal(msg)
	require.NoError(t, err)

	// Deserialize
	decoded := &TranscodeMessage{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// Verify using type switch
	switch p := decoded.Payload.(type) {
	case *TranscodeMessage_Start:
		assert.Equal(t, "job-001", p.Start.JobId)
		assert.Equal(t, "Test Channel", p.Start.ChannelName)
	default:
		t.Fatalf("unexpected payload type: %T", p)
	}
}

func TestTranscodeMessage_SamplesPayload(t *testing.T) {
	msg := &TranscodeMessage{
		Payload: &TranscodeMessage_Samples{
			Samples: &ESSampleBatch{
				VideoSamples: []*ESSample{
					{Pts: 90000, IsKeyframe: true},
				},
				IsSource:      true,
				BatchSequence: 1,
			},
		},
	}

	// Serialize
	data, err := proto.Marshal(msg)
	require.NoError(t, err)

	// Deserialize
	decoded := &TranscodeMessage{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// Verify
	samples, ok := decoded.Payload.(*TranscodeMessage_Samples)
	require.True(t, ok)
	assert.True(t, samples.Samples.IsSource)
	require.Len(t, samples.Samples.VideoSamples, 1)
	assert.True(t, samples.Samples.VideoSamples[0].IsKeyframe)
}
