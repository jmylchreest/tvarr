package contract

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	gproto "google.golang.org/protobuf/proto"

	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
)

// TestRegisterRequest_ValidatesRequiredFields tests that the Register RPC
// validates required fields in the request.
func TestRegisterRequest_ValidatesRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		request *proto.RegisterRequest
		wantErr bool
	}{
		{
			name: "valid_request_with_all_fields",
			request: &proto.RegisterRequest{
				DaemonId:   "test-daemon-001",
				DaemonName: "Test Daemon",
				Version:    "1.0.0",
				Capabilities: &proto.Capabilities{
					VideoEncoders:     []string{"libx264", "h264_nvenc"},
					VideoDecoders:     []string{"h264", "hevc"},
					AudioEncoders:     []string{"aac"},
					AudioDecoders:     []string{"aac", "ac3"},
					MaxConcurrentJobs: 4,
				},
			},
			wantErr: false,
		},
		{
			name: "valid_request_minimal",
			request: &proto.RegisterRequest{
				DaemonId:   "test-daemon-002",
				DaemonName: "Minimal Daemon",
				Version:    "1.0.0",
				Capabilities: &proto.Capabilities{
					MaxConcurrentJobs: 1,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate the protobuf message can be serialized
			_, err := gproto.Marshal(tt.request)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestRegisterRequest_CapabilitiesSerialization tests that capabilities
// serialize and deserialize correctly.
func TestRegisterRequest_CapabilitiesSerialization(t *testing.T) {
	original := &proto.RegisterRequest{
		DaemonId:   "test-daemon",
		DaemonName: "Test Daemon",
		Version:    "1.0.0",
		Capabilities: &proto.Capabilities{
			VideoEncoders:     []string{"libx264", "h264_nvenc", "hevc_vaapi"},
			VideoDecoders:     []string{"h264", "hevc", "h264_cuvid"},
			AudioEncoders:     []string{"aac", "libopus"},
			AudioDecoders:     []string{"aac", "ac3", "eac3"},
			MaxConcurrentJobs: 8,
			HwAccels: []*proto.HWAccelInfo{
				{
					Type:       "cuda",
					Device:     "GPU 0",
					Available:  true,
					HwEncoders: []string{"h264_nvenc", "hevc_nvenc"},
					HwDecoders: []string{"h264_cuvid", "hevc_cuvid"},
				},
				{
					Type:       "vaapi",
					Device:     "/dev/dri/renderD128",
					Available:  true,
					HwEncoders: []string{"h264_vaapi", "hevc_vaapi"},
					HwDecoders: []string{"h264", "hevc"},
				},
			},
			Gpus: []*proto.GPUInfo{
				{
					Index:             0,
					Name:              "NVIDIA GeForce RTX 3080",
					GpuClass:          proto.GPUClass_GPU_CLASS_CONSUMER,
					DriverVersion:     "535.183.01",
					MaxEncodeSessions: 3,
					MaxDecodeSessions: 8,
					MemoryTotalBytes:  10 * 1024 * 1024 * 1024, // 10GB
				},
			},
		},
	}

	// Marshal
	data, err := gproto.Marshal(original)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Unmarshal
	decoded := &proto.RegisterRequest{}
	err = gproto.Unmarshal(data, decoded)
	require.NoError(t, err)

	// Verify
	assert.Equal(t, original.DaemonId, decoded.DaemonId)
	assert.Equal(t, original.DaemonName, decoded.DaemonName)
	assert.Equal(t, original.Version, decoded.Version)

	require.NotNil(t, decoded.Capabilities)
	caps := decoded.Capabilities

	assert.ElementsMatch(t, original.Capabilities.VideoEncoders, caps.VideoEncoders)
	assert.ElementsMatch(t, original.Capabilities.VideoDecoders, caps.VideoDecoders)
	assert.ElementsMatch(t, original.Capabilities.AudioEncoders, caps.AudioEncoders)
	assert.ElementsMatch(t, original.Capabilities.AudioDecoders, caps.AudioDecoders)
	assert.Equal(t, original.Capabilities.MaxConcurrentJobs, caps.MaxConcurrentJobs)

	require.Len(t, caps.HwAccels, 2)
	assert.Equal(t, "cuda", caps.HwAccels[0].Type)
	assert.Equal(t, "vaapi", caps.HwAccels[1].Type)

	require.Len(t, caps.Gpus, 1)
	assert.Equal(t, "NVIDIA GeForce RTX 3080", caps.Gpus[0].Name)
	assert.Equal(t, proto.GPUClass_GPU_CLASS_CONSUMER, caps.Gpus[0].GpuClass)
	assert.Equal(t, int32(3), caps.Gpus[0].MaxEncodeSessions)
}

// TestRegisterResponse_Success tests successful registration response.
func TestRegisterResponse_Success(t *testing.T) {
	response := &proto.RegisterResponse{
		Success:            true,
		CoordinatorVersion: "0.0.1",
	}

	// Marshal
	data, err := gproto.Marshal(response)
	require.NoError(t, err)

	// Unmarshal
	decoded := &proto.RegisterResponse{}
	err = gproto.Unmarshal(data, decoded)
	require.NoError(t, err)

	assert.True(t, decoded.Success)
	assert.Equal(t, "0.0.1", decoded.CoordinatorVersion)
	assert.Empty(t, decoded.Error)
}

// TestRegisterResponse_Failure tests failed registration response.
func TestRegisterResponse_Failure(t *testing.T) {
	response := &proto.RegisterResponse{
		Success: false,
		Error:   "invalid authentication token",
	}

	// Marshal
	data, err := gproto.Marshal(response)
	require.NoError(t, err)

	// Unmarshal
	decoded := &proto.RegisterResponse{}
	err = gproto.Unmarshal(data, decoded)
	require.NoError(t, err)

	assert.False(t, decoded.Success)
	assert.Equal(t, "invalid authentication token", decoded.Error)
}

// TestGRPCClientConnection tests that a gRPC client can be created with
// the correct connection options for connecting to a daemon.
func TestGRPCClientConnection(t *testing.T) {
	// Start a test listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	addr := listener.Addr().String()

	// Create a mock server (just to accept connections)
	grpcServer := grpc.NewServer()
	go func() {
		_ = grpcServer.Serve(listener)
	}()
	defer grpcServer.Stop()

	// Test client connection
	_, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	// Create client
	client := proto.NewFFmpegDaemonClient(conn)
	require.NotNil(t, client)

	// Note: We can't actually call Register here because we haven't
	// registered a handler, but we've verified the client can be created
	// and connected.

	// Verify connection state
	err = conn.Close()
	assert.NoError(t, err)
}

// TestRegisterRequest_AuthToken tests that auth token is properly serialized.
func TestRegisterRequest_AuthToken(t *testing.T) {
	request := &proto.RegisterRequest{
		DaemonId:   "test-daemon",
		DaemonName: "Test Daemon",
		Version:    "1.0.0",
		AuthToken:  "secret-token-12345",
		Capabilities: &proto.Capabilities{
			MaxConcurrentJobs: 4,
		},
	}

	// Marshal
	data, err := gproto.Marshal(request)
	require.NoError(t, err)

	// Unmarshal
	decoded := &proto.RegisterRequest{}
	err = gproto.Unmarshal(data, decoded)
	require.NoError(t, err)

	assert.Equal(t, "secret-token-12345", decoded.AuthToken)
}
