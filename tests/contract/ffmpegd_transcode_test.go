package contract

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	pb "github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
)

// TestTranscodeStart_ValidatesRequiredFields tests that TranscodeStart messages
// can be created with required fields and serialized correctly.
func TestTranscodeStart_ValidatesRequiredFields(t *testing.T) {
	t.Run("minimal_transcode_start", func(t *testing.T) {
		start := &pb.TranscodeStart{
			JobId:            "job-123",
			SessionId:        "session-456",
			ChannelId:        "channel-789",
			ChannelName:      "Test Channel",
			SourceVideoCodec: "h264",
			SourceAudioCodec: "aac",
			TargetVideoCodec: "h265",
			TargetAudioCodec: "aac",
		}

		// Verify serialization round-trip
		data, err := proto.Marshal(start)
		require.NoError(t, err)

		decoded := &pb.TranscodeStart{}
		err = proto.Unmarshal(data, decoded)
		require.NoError(t, err)

		assert.Equal(t, start.JobId, decoded.JobId)
		assert.Equal(t, start.SessionId, decoded.SessionId)
		assert.Equal(t, start.SourceVideoCodec, decoded.SourceVideoCodec)
		assert.Equal(t, start.TargetVideoCodec, decoded.TargetVideoCodec)
	})

	t.Run("full_transcode_start_with_hw_accel", func(t *testing.T) {
		start := &pb.TranscodeStart{
			JobId:             "job-123",
			SessionId:         "session-456",
			ChannelId:         "channel-789",
			ChannelName:       "HD Movie Channel",
			SourceVideoCodec:  "h264",
			SourceAudioCodec:  "ac3",
			VideoInitData:     []byte{0x00, 0x00, 0x00, 0x01, 0x67}, // SPS start
			AudioInitData:     []byte{0x11, 0x90},                   // AAC config
			TargetVideoCodec:  "h265",
			TargetAudioCodec:  "aac",
			VideoBitrateKbps:  8000,
			AudioBitrateKbps:  192,
			VideoPreset:       "fast",
			VideoCrf:          23,
			VideoProfile:      "main",
			VideoLevel:        "4.1",
			PreferredHwAccel:  "cuda",
			HwDevice:          "GPU 0",
			ScaleWidth:        1920,
			ScaleHeight:       1080,
			ExtraOptions: map[string]string{
				"b_adapt": "2",
			},
		}

		data, err := proto.Marshal(start)
		require.NoError(t, err)

		decoded := &pb.TranscodeStart{}
		err = proto.Unmarshal(data, decoded)
		require.NoError(t, err)

		assert.Equal(t, "h265", decoded.TargetVideoCodec)
		assert.Equal(t, "cuda", decoded.PreferredHwAccel)
		assert.Equal(t, int32(8000), decoded.VideoBitrateKbps)
		assert.Equal(t, int32(1920), decoded.ScaleWidth)
		assert.Equal(t, "2", decoded.ExtraOptions["b_adapt"])
	})
}

// TestTranscodeAck_Serialization tests TranscodeAck message serialization.
func TestTranscodeAck_Serialization(t *testing.T) {
	t.Run("success_ack", func(t *testing.T) {
		ack := &pb.TranscodeAck{
			Success:            true,
			ActualVideoEncoder: "hevc_nvenc",
			ActualAudioEncoder: "aac",
			ActualHwAccel:      "cuda",
		}

		data, err := proto.Marshal(ack)
		require.NoError(t, err)

		decoded := &pb.TranscodeAck{}
		err = proto.Unmarshal(data, decoded)
		require.NoError(t, err)

		assert.True(t, decoded.Success)
		assert.Equal(t, "hevc_nvenc", decoded.ActualVideoEncoder)
		assert.Equal(t, "cuda", decoded.ActualHwAccel)
	})

	t.Run("failure_ack_with_fallback", func(t *testing.T) {
		ack := &pb.TranscodeAck{
			Success:            true,
			Error:              "", // No error, but fell back
			ActualVideoEncoder: "libx265",
			ActualAudioEncoder: "aac",
			ActualHwAccel:      "", // No HW accel used
		}

		data, err := proto.Marshal(ack)
		require.NoError(t, err)

		decoded := &pb.TranscodeAck{}
		err = proto.Unmarshal(data, decoded)
		require.NoError(t, err)

		assert.True(t, decoded.Success)
		assert.Equal(t, "libx265", decoded.ActualVideoEncoder)
		assert.Empty(t, decoded.ActualHwAccel)
	})

	t.Run("failure_ack", func(t *testing.T) {
		ack := &pb.TranscodeAck{
			Success: false,
			Error:   "encoder not available: hevc_nvenc",
		}

		data, err := proto.Marshal(ack)
		require.NoError(t, err)

		decoded := &pb.TranscodeAck{}
		err = proto.Unmarshal(data, decoded)
		require.NoError(t, err)

		assert.False(t, decoded.Success)
		assert.Contains(t, decoded.Error, "hevc_nvenc")
	})
}

// TestESSample_Serialization tests ES sample message serialization.
func TestESSample_Serialization(t *testing.T) {
	t.Run("video_keyframe", func(t *testing.T) {
		sample := &pb.ESSample{
			Pts:        90000,  // 1 second at 90kHz
			Dts:        90000,
			Data:       []byte{0x00, 0x00, 0x00, 0x01, 0x65}, // IDR NAL
			IsKeyframe: true,
			Sequence:   1,
		}

		data, err := proto.Marshal(sample)
		require.NoError(t, err)

		decoded := &pb.ESSample{}
		err = proto.Unmarshal(data, decoded)
		require.NoError(t, err)

		assert.Equal(t, int64(90000), decoded.Pts)
		assert.True(t, decoded.IsKeyframe)
		assert.Equal(t, uint64(1), decoded.Sequence)
	})

	t.Run("video_p_frame", func(t *testing.T) {
		sample := &pb.ESSample{
			Pts:        93003, // ~1.033s
			Dts:        90000,
			Data:       []byte{0x00, 0x00, 0x00, 0x01, 0x41}, // P-frame NAL
			IsKeyframe: false,
			Sequence:   2,
		}

		data, err := proto.Marshal(sample)
		require.NoError(t, err)

		decoded := &pb.ESSample{}
		err = proto.Unmarshal(data, decoded)
		require.NoError(t, err)

		assert.Equal(t, int64(93003), decoded.Pts)
		assert.Equal(t, int64(90000), decoded.Dts)
		assert.False(t, decoded.IsKeyframe)
	})

	t.Run("audio_sample", func(t *testing.T) {
		sample := &pb.ESSample{
			Pts:        90000,
			Dts:        90000,
			Data:       []byte{0xFF, 0xF1, 0x50, 0x80}, // ADTS header start
			IsKeyframe: true,                           // Audio frames are always "keyframes"
			Sequence:   100,
		}

		data, err := proto.Marshal(sample)
		require.NoError(t, err)

		decoded := &pb.ESSample{}
		err = proto.Unmarshal(data, decoded)
		require.NoError(t, err)

		assert.Equal(t, uint64(100), decoded.Sequence)
	})
}

// TestESSampleBatch_Serialization tests ES sample batch message serialization.
func TestESSampleBatch_Serialization(t *testing.T) {
	t.Run("source_batch", func(t *testing.T) {
		batch := &pb.ESSampleBatch{
			VideoSamples: []*pb.ESSample{
				{Pts: 90000, Dts: 90000, Data: []byte{0x65}, IsKeyframe: true, Sequence: 1},
				{Pts: 93003, Dts: 90000, Data: []byte{0x41}, IsKeyframe: false, Sequence: 2},
				{Pts: 96006, Dts: 93003, Data: []byte{0x41}, IsKeyframe: false, Sequence: 3},
			},
			AudioSamples: []*pb.ESSample{
				{Pts: 90000, Data: []byte{0xFF, 0xF1}, Sequence: 1},
				{Pts: 91024, Data: []byte{0xFF, 0xF1}, Sequence: 2},
			},
			IsSource:      true,
			BatchSequence: 1,
		}

		data, err := proto.Marshal(batch)
		require.NoError(t, err)

		decoded := &pb.ESSampleBatch{}
		err = proto.Unmarshal(data, decoded)
		require.NoError(t, err)

		assert.True(t, decoded.IsSource)
		assert.Len(t, decoded.VideoSamples, 3)
		assert.Len(t, decoded.AudioSamples, 2)
		assert.Equal(t, uint64(1), decoded.BatchSequence)
	})

	t.Run("transcoded_batch", func(t *testing.T) {
		batch := &pb.ESSampleBatch{
			VideoSamples: []*pb.ESSample{
				{Pts: 90000, Dts: 90000, Data: []byte{0x40, 0x01}, IsKeyframe: true, Sequence: 1}, // HEVC IDR
			},
			AudioSamples: []*pb.ESSample{
				{Pts: 90000, Data: []byte{0xFF, 0xF1}, Sequence: 1},
			},
			IsSource:      false,
			BatchSequence: 1,
		}

		data, err := proto.Marshal(batch)
		require.NoError(t, err)

		decoded := &pb.ESSampleBatch{}
		err = proto.Unmarshal(data, decoded)
		require.NoError(t, err)

		assert.False(t, decoded.IsSource)
		assert.Len(t, decoded.VideoSamples, 1)
	})
}

// TestTranscodeStats_Serialization tests TranscodeStats message serialization.
func TestTranscodeStats_Serialization(t *testing.T) {
	stats := &pb.TranscodeStats{
		SamplesIn:     1000,
		SamplesOut:    998,
		BytesIn:       10_000_000,
		BytesOut:      5_000_000,
		EncodingSpeed: 1.5,
		CpuPercent:    45.5,
		MemoryMb:      512.0,
		FfmpegPid:     12345,
		RunningTime:   durationpb.New(60_000_000_000), // 60 seconds
	}

	data, err := proto.Marshal(stats)
	require.NoError(t, err)

	decoded := &pb.TranscodeStats{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	assert.Equal(t, uint64(1000), decoded.SamplesIn)
	assert.Equal(t, uint64(998), decoded.SamplesOut)
	assert.Equal(t, 1.5, decoded.EncodingSpeed)
	assert.Equal(t, int32(12345), decoded.FfmpegPid)
	assert.Equal(t, int64(60_000_000_000), decoded.RunningTime.AsDuration().Nanoseconds())
}

// TestTranscodeError_Serialization tests TranscodeError message serialization.
func TestTranscodeError_Serialization(t *testing.T) {
	t.Run("ffmpeg_crash_error", func(t *testing.T) {
		err := &pb.TranscodeError{
			Code:         pb.TranscodeError_FFMPEG_CRASHED,
			Message:      "FFmpeg process exited with code 1",
			FfmpegStderr: "Error opening encoder hevc_nvenc\nNo NVENC capable devices found",
			Recoverable:  true,
		}

		data, protoErr := proto.Marshal(err)
		require.NoError(t, protoErr)

		decoded := &pb.TranscodeError{}
		protoErr = proto.Unmarshal(data, decoded)
		require.NoError(t, protoErr)

		assert.Equal(t, pb.TranscodeError_FFMPEG_CRASHED, decoded.Code)
		assert.Contains(t, decoded.FfmpegStderr, "NVENC")
		assert.True(t, decoded.Recoverable)
	})

	t.Run("session_limit_error", func(t *testing.T) {
		err := &pb.TranscodeError{
			Code:        pb.TranscodeError_SESSION_LIMIT_REACHED,
			Message:     "GPU encode session limit reached (3/3)",
			Recoverable: false,
		}

		data, protoErr := proto.Marshal(err)
		require.NoError(t, protoErr)

		decoded := &pb.TranscodeError{}
		protoErr = proto.Unmarshal(data, decoded)
		require.NoError(t, protoErr)

		assert.Equal(t, pb.TranscodeError_SESSION_LIMIT_REACHED, decoded.Code)
		assert.False(t, decoded.Recoverable)
	})
}

// TestTranscodeStop_Serialization tests TranscodeStop message serialization.
func TestTranscodeStop_Serialization(t *testing.T) {
	stop := &pb.TranscodeStop{
		Reason: "session_ended",
	}

	data, err := proto.Marshal(stop)
	require.NoError(t, err)

	decoded := &pb.TranscodeStop{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	assert.Equal(t, "session_ended", decoded.Reason)
}

// TestTranscodeMessage_OneofPayload tests the TranscodeMessage oneof behavior.
func TestTranscodeMessage_OneofPayload(t *testing.T) {
	t.Run("start_message", func(t *testing.T) {
		msg := &pb.TranscodeMessage{
			Payload: &pb.TranscodeMessage_Start{
				Start: &pb.TranscodeStart{
					JobId:            "job-1",
					TargetVideoCodec: "h264",
				},
			},
		}

		data, err := proto.Marshal(msg)
		require.NoError(t, err)

		decoded := &pb.TranscodeMessage{}
		err = proto.Unmarshal(data, decoded)
		require.NoError(t, err)

		start := decoded.GetStart()
		require.NotNil(t, start)
		assert.Equal(t, "job-1", start.JobId)
		assert.Nil(t, decoded.GetAck())
		assert.Nil(t, decoded.GetSamples())
	})

	t.Run("samples_message", func(t *testing.T) {
		msg := &pb.TranscodeMessage{
			Payload: &pb.TranscodeMessage_Samples{
				Samples: &pb.ESSampleBatch{
					VideoSamples: []*pb.ESSample{
						{Pts: 90000, Sequence: 1},
					},
					IsSource:      true,
					BatchSequence: 5,
				},
			},
		}

		data, err := proto.Marshal(msg)
		require.NoError(t, err)

		decoded := &pb.TranscodeMessage{}
		err = proto.Unmarshal(data, decoded)
		require.NoError(t, err)

		samples := decoded.GetSamples()
		require.NotNil(t, samples)
		assert.True(t, samples.IsSource)
		assert.Equal(t, uint64(5), samples.BatchSequence)
	})

	t.Run("stats_message", func(t *testing.T) {
		msg := &pb.TranscodeMessage{
			Payload: &pb.TranscodeMessage_Stats{
				Stats: &pb.TranscodeStats{
					EncodingSpeed: 2.0,
					SamplesOut:    500,
				},
			},
		}

		data, err := proto.Marshal(msg)
		require.NoError(t, err)

		decoded := &pb.TranscodeMessage{}
		err = proto.Unmarshal(data, decoded)
		require.NoError(t, err)

		stats := decoded.GetStats()
		require.NotNil(t, stats)
		assert.Equal(t, 2.0, stats.EncodingSpeed)
	})

	t.Run("error_message", func(t *testing.T) {
		msg := &pb.TranscodeMessage{
			Payload: &pb.TranscodeMessage_Error{
				Error: &pb.TranscodeError{
					Code:    pb.TranscodeError_CODEC_UNSUPPORTED,
					Message: "VP9 not supported",
				},
			},
		}

		data, err := proto.Marshal(msg)
		require.NoError(t, err)

		decoded := &pb.TranscodeMessage{}
		err = proto.Unmarshal(data, decoded)
		require.NoError(t, err)

		transcodeErr := decoded.GetError()
		require.NotNil(t, transcodeErr)
		assert.Equal(t, pb.TranscodeError_CODEC_UNSUPPORTED, transcodeErr.Code)
	})

	t.Run("stop_message", func(t *testing.T) {
		msg := &pb.TranscodeMessage{
			Payload: &pb.TranscodeMessage_Stop{
				Stop: &pb.TranscodeStop{
					Reason: "cancelled",
				},
			},
		}

		data, err := proto.Marshal(msg)
		require.NoError(t, err)

		decoded := &pb.TranscodeMessage{}
		err = proto.Unmarshal(data, decoded)
		require.NoError(t, err)

		stop := decoded.GetStop()
		require.NotNil(t, stop)
		assert.Equal(t, "cancelled", stop.Reason)
	})
}

// TestTranscodeMessage_LargeBatch tests handling of large sample batches.
func TestTranscodeMessage_LargeBatch(t *testing.T) {
	// Create a batch with 100 video samples and 200 audio samples
	// (typical for ~3 seconds of video at 30fps with 48kHz audio)
	batch := &pb.ESSampleBatch{
		VideoSamples:  make([]*pb.ESSample, 100),
		AudioSamples:  make([]*pb.ESSample, 200),
		IsSource:      true,
		BatchSequence: 42,
	}

	// Fill with realistic sample sizes
	for i := 0; i < 100; i++ {
		isKey := i%30 == 0 // Keyframe every 30 frames
		dataSize := 10000  // ~10KB per frame
		if isKey {
			dataSize = 50000 // ~50KB for keyframes
		}
		batch.VideoSamples[i] = &pb.ESSample{
			Pts:        int64(i * 3003), // 30fps at 90kHz
			Dts:        int64(i * 3003),
			Data:       make([]byte, dataSize),
			IsKeyframe: isKey,
			Sequence:   uint64(i + 1),
		}
	}

	for i := 0; i < 200; i++ {
		batch.AudioSamples[i] = &pb.ESSample{
			Pts:      int64(i * 1920), // 48kHz audio at 90kHz timescale
			Data:     make([]byte, 1024),
			Sequence: uint64(i + 1),
		}
	}

	msg := &pb.TranscodeMessage{
		Payload: &pb.TranscodeMessage_Samples{Samples: batch},
	}

	data, err := proto.Marshal(msg)
	require.NoError(t, err)

	// Verify we can handle larger messages (should be ~1.5MB)
	assert.Greater(t, len(data), 1_000_000, "batch should be >1MB")
	assert.Less(t, len(data), 10_000_000, "batch should be <10MB")

	decoded := &pb.TranscodeMessage{}
	err = proto.Unmarshal(data, decoded)
	require.NoError(t, err)

	samples := decoded.GetSamples()
	require.NotNil(t, samples)
	assert.Len(t, samples.VideoSamples, 100)
	assert.Len(t, samples.AudioSamples, 200)
}
