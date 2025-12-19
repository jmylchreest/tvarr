// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jmylchreest/tvarr/internal/ffmpeg"
	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
)

// Transcoder is the interface for codec transcoding.
// Both FFmpegTranscoder (direct FFmpeg) and GRPCTranscoder (via ffmpegd) implement this.
type Transcoder interface {
	// Start begins the transcoding process.
	Start(ctx context.Context) error

	// Stop stops the transcoder.
	Stop()

	// Stats returns current transcoder statistics.
	Stats() TranscoderStats

	// ProcessStats returns process-level statistics (CPU, memory).
	// Returns nil if not available.
	ProcessStats() *TranscoderProcessStats

	// IsClosed returns true if the transcoder is closed.
	IsClosed() bool

	// ClosedChan returns a channel that is closed when transcoder stops.
	ClosedChan() <-chan struct{}
}

// TranscoderFactory creates transcoders based on configuration and availability.
type TranscoderFactory struct {
	// Spawner for creating ffmpegd subprocesses.
	// If nil, falls back to direct FFmpeg transcoding.
	Spawner *FFmpegDSpawner

	// DaemonRegistry for remote daemon selection (distributed mode).
	// If nil, only local subprocess or direct FFmpeg is available.
	DaemonRegistry *DaemonRegistry

	// SelectionStrategy for choosing remote daemons.
	// Defaults to DefaultSelectionStrategy() if nil.
	SelectionStrategy SelectionStrategy

	// PreferGRPC indicates whether to prefer gRPC (ffmpegd) over direct FFmpeg.
	// When true and Spawner is available, uses GRPCTranscoder.
	// When false or Spawner unavailable, uses FFmpegTranscoder.
	PreferGRPC bool

	// PreferRemote indicates whether to prefer remote daemons over local subprocess.
	// When true and suitable remote daemons are available, routes to remote.
	// When false or no suitable remote available, falls back to local.
	PreferRemote bool

	// FFmpegBin is the FFmpeg binary info for direct transcoding.
	FFmpegBin *ffmpeg.BinaryInfo

	// Logger for structured logging.
	Logger *slog.Logger
}

// TranscoderFactoryConfig configures the transcoder factory.
type TranscoderFactoryConfig struct {
	// Spawner for ffmpegd subprocesses. If nil, only direct FFmpeg is available.
	Spawner *FFmpegDSpawner

	// DaemonRegistry for remote daemon selection (distributed mode).
	DaemonRegistry *DaemonRegistry

	// SelectionStrategy for choosing remote daemons.
	SelectionStrategy SelectionStrategy

	// PreferGRPC indicates preference for gRPC transcoding when available.
	PreferGRPC bool

	// PreferRemote indicates preference for remote daemons over local subprocess.
	PreferRemote bool

	// FFmpegBin is required for direct FFmpeg transcoding.
	FFmpegBin *ffmpeg.BinaryInfo

	// Logger for logging. Defaults to slog.Default() if nil.
	Logger *slog.Logger
}

// NewTranscoderFactory creates a new transcoder factory.
func NewTranscoderFactory(config TranscoderFactoryConfig) *TranscoderFactory {
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	strategy := config.SelectionStrategy
	if strategy == nil {
		strategy = DefaultSelectionStrategy()
	}

	return &TranscoderFactory{
		Spawner:           config.Spawner,
		DaemonRegistry:    config.DaemonRegistry,
		SelectionStrategy: strategy,
		PreferGRPC:        config.PreferGRPC,
		PreferRemote:      config.PreferRemote,
		FFmpegBin:         config.FFmpegBin,
		Logger:            logger,
	}
}

// CanCreateGRPCTranscoder returns whether gRPC transcoders can be created.
func (f *TranscoderFactory) CanCreateGRPCTranscoder() bool {
	return f.Spawner != nil && f.Spawner.IsAvailable()
}

// CanCreateRemoteTranscoder returns whether remote daemon transcoders are available.
func (f *TranscoderFactory) CanCreateRemoteTranscoder() bool {
	return f.DaemonRegistry != nil && f.DaemonRegistry.CountActive() > 0
}

// CanCreateDirectTranscoder returns whether direct FFmpeg transcoders can be created.
func (f *TranscoderFactory) CanCreateDirectTranscoder() bool {
	return f.FFmpegBin != nil && f.FFmpegBin.FFmpegPath != ""
}

// ShouldUseGRPC returns whether gRPC transcoding should be used.
// This checks for local subprocess availability.
func (f *TranscoderFactory) ShouldUseGRPC() bool {
	return f.PreferGRPC && f.CanCreateGRPCTranscoder()
}

// ShouldUseRemote returns whether remote daemon transcoding should be used.
func (f *TranscoderFactory) ShouldUseRemote() bool {
	return f.PreferRemote && f.CanCreateRemoteTranscoder()
}

// SelectRemoteDaemon selects a remote daemon for the given encoder requirement.
// Returns nil if no suitable daemon is available.
func (f *TranscoderFactory) SelectRemoteDaemon(requiredEncoder string, requireGPU bool) *types.Daemon {
	if f.DaemonRegistry == nil {
		return nil
	}

	criteria := SelectionCriteria{
		RequiredEncoder: requiredEncoder,
		RequireGPU:      requireGPU,
	}

	return f.DaemonRegistry.SelectDaemon(f.SelectionStrategy, criteria)
}

// CreateTranscoderOptions contains options for creating a transcoder.
type CreateTranscoderOptions struct {
	// SourceURL for direct input mode (bypasses ES demux/mux).
	SourceURL string

	// UseDirectInput enables direct URL input when audio codec can't be demuxed.
	UseDirectInput bool

	// ChannelName for job identification (used in gRPC mode).
	ChannelName string
}

// CreateTranscoderFromProfile creates a transcoder from an encoding profile.
// Priority order:
// 1. Remote daemon (if PreferRemote and suitable daemon available)
// 2. Local gRPC subprocess (if PreferGRPC and spawner available)
// 3. Direct FFmpeg (fallback)
func (f *TranscoderFactory) CreateTranscoderFromProfile(
	id string,
	buffer *SharedESBuffer,
	sourceVariant CodecVariant,
	profile *models.EncodingProfile,
	opts CreateTranscoderOptions,
) (Transcoder, error) {
	if profile == nil {
		return nil, fmt.Errorf("profile is required")
	}

	// Determine target variant from profile codecs
	targetVariant := MakeCodecVariant(
		string(profile.TargetVideoCodec),
		string(profile.TargetAudioCodec),
	)

	// Get encoding parameters from quality preset
	encodingParams := profile.GetEncodingParams()
	videoEncoder := profile.GetVideoEncoder()
	audioEncoder := profile.GetAudioEncoder()
	videoBitrate := profile.GetVideoBitrate()
	audioBitrate := profile.GetAudioBitrate()
	videoPreset := encodingParams.VideoPreset
	hwAccel := string(profile.HWAccel)

	// Determine if HW encoding is requested
	requireGPU := isHardwareEncoder(videoEncoder)

	// 1. Try remote daemon first if preferred
	if f.ShouldUseRemote() {
		daemon := f.SelectRemoteDaemon(videoEncoder, requireGPU)
		if daemon != nil {
			f.Logger.Debug("Selected remote daemon for transcoding",
				slog.String("id", id),
				slog.String("daemon_id", string(daemon.ID)),
				slog.String("daemon_name", daemon.Name),
				slog.String("video_encoder", videoEncoder),
			)
			return f.createRemoteTranscoder(id, buffer, sourceVariant, targetVariant,
				videoEncoder, audioEncoder, videoBitrate, audioBitrate,
				videoPreset, hwAccel, "", daemon, opts)
		}
		f.Logger.Debug("No suitable remote daemon found, falling back to local",
			slog.String("id", id),
			slog.String("video_encoder", videoEncoder),
		)
	}

	// 2. Try local gRPC subprocess
	if f.ShouldUseGRPC() {
		return f.createGRPCTranscoder(id, buffer, sourceVariant, targetVariant,
			videoEncoder, audioEncoder, videoBitrate, audioBitrate,
			videoPreset, hwAccel, "", opts)
	}

	// 3. Fall back to direct FFmpeg
	if f.CanCreateDirectTranscoder() {
		return f.createDirectTranscoder(id, buffer, sourceVariant, targetVariant,
			videoEncoder, audioEncoder, videoBitrate, audioBitrate,
			videoPreset, hwAccel, "", opts)
	}

	return nil, fmt.Errorf("no transcoding backend available")
}

// CreateTranscoderFromVariant creates a transcoder from source and target variants.
// Uses default settings for bitrate, preset, etc.
// Priority order:
// 1. Remote daemon (if PreferRemote and suitable daemon available)
// 2. Local gRPC subprocess (if PreferGRPC and spawner available)
// 3. Direct FFmpeg (fallback)
func (f *TranscoderFactory) CreateTranscoderFromVariant(
	id string,
	buffer *SharedESBuffer,
	sourceVariant CodecVariant,
	targetVariant CodecVariant,
	opts CreateTranscoderOptions,
) (Transcoder, error) {
	// Map variant codec names to FFmpeg encoders
	videoEncoder, audioEncoder := mapVariantToEncoders(targetVariant)
	requireGPU := isHardwareEncoder(videoEncoder)

	// 1. Try remote daemon first if preferred
	if f.ShouldUseRemote() {
		daemon := f.SelectRemoteDaemon(videoEncoder, requireGPU)
		if daemon != nil {
			f.Logger.Debug("Selected remote daemon for transcoding",
				slog.String("id", id),
				slog.String("daemon_id", string(daemon.ID)),
				slog.String("video_encoder", videoEncoder),
			)
			return f.createRemoteTranscoder(id, buffer, sourceVariant, targetVariant,
				videoEncoder, audioEncoder, 0, 0, "medium", "", "", daemon, opts)
		}
	}

	// 2. Try local gRPC subprocess
	if f.ShouldUseGRPC() {
		return f.createGRPCTranscoder(id, buffer, sourceVariant, targetVariant,
			videoEncoder, audioEncoder, 0, 0, "medium", "", "", opts)
	}

	// 3. Fall back to direct FFmpeg
	if f.CanCreateDirectTranscoder() {
		return f.createDirectTranscoder(id, buffer, sourceVariant, targetVariant,
			videoEncoder, audioEncoder, 0, 0, "medium", "", "", opts)
	}

	return nil, fmt.Errorf("no transcoding backend available")
}

// createGRPCTranscoder creates a gRPC-based transcoder.
func (f *TranscoderFactory) createGRPCTranscoder(
	id string,
	buffer *SharedESBuffer,
	sourceVariant, targetVariant CodecVariant,
	videoEncoder, audioEncoder string,
	videoBitrate, audioBitrate int,
	videoPreset, hwAccel, hwAccelDevice string,
	opts CreateTranscoderOptions,
) (Transcoder, error) {
	config := GRPCTranscoderConfig{
		SourceVariant:  sourceVariant,
		TargetVariant:  targetVariant,
		VideoEncoder:   videoEncoder,
		AudioEncoder:   audioEncoder,
		VideoBitrate:   videoBitrate,
		AudioBitrate:   audioBitrate,
		VideoPreset:    videoPreset,
		HWAccel:        hwAccel,
		HWAccelDevice:  hwAccelDevice,
		SourceURL:      opts.SourceURL,
		UseDirectInput: opts.UseDirectInput,
		ChannelName:    opts.ChannelName,
		Logger:         f.Logger,
	}

	f.Logger.Debug("Creating gRPC transcoder",
		slog.String("id", id),
		slog.String("source", sourceVariant.String()),
		slog.String("target", targetVariant.String()),
		slog.String("video_encoder", videoEncoder),
		slog.String("audio_encoder", audioEncoder),
		slog.Bool("direct_input", opts.UseDirectInput))

	return NewGRPCTranscoder(id, buffer, f.Spawner, config), nil
}

// createDirectTranscoder creates a direct FFmpeg transcoder.
func (f *TranscoderFactory) createDirectTranscoder(
	id string,
	buffer *SharedESBuffer,
	sourceVariant, targetVariant CodecVariant,
	videoEncoder, audioEncoder string,
	videoBitrate, audioBitrate int,
	videoPreset, hwAccel, hwAccelDevice string,
	opts CreateTranscoderOptions,
) (Transcoder, error) {
	config := FFmpegTranscoderConfig{
		FFmpegPath:     f.FFmpegBin.FFmpegPath,
		SourceVariant:  sourceVariant,
		TargetVariant:  targetVariant,
		VideoCodec:     videoEncoder,
		AudioCodec:     audioEncoder,
		VideoBitrate:   videoBitrate,
		AudioBitrate:   audioBitrate,
		VideoPreset:    videoPreset,
		HWAccel:        hwAccel,
		HWAccelDevice:  hwAccelDevice,
		SourceURL:      opts.SourceURL,
		UseDirectInput: opts.UseDirectInput,
		Logger:         f.Logger,
	}

	f.Logger.Debug("Creating direct FFmpeg transcoder",
		slog.String("id", id),
		slog.String("source", sourceVariant.String()),
		slog.String("target", targetVariant.String()),
		slog.String("video_encoder", videoEncoder),
		slog.String("audio_encoder", audioEncoder),
		slog.Bool("direct_input", opts.UseDirectInput))

	return NewFFmpegTranscoder(id, buffer, config), nil
}

// createRemoteTranscoder creates a transcoder that uses a remote ffmpegd daemon.
// This is for distributed transcoding where the work is offloaded to remote workers.
func (f *TranscoderFactory) createRemoteTranscoder(
	id string,
	buffer *SharedESBuffer,
	sourceVariant, targetVariant CodecVariant,
	videoEncoder, audioEncoder string,
	videoBitrate, audioBitrate int,
	videoPreset, hwAccel, hwAccelDevice string,
	daemon *types.Daemon,
	opts CreateTranscoderOptions,
) (Transcoder, error) {
	// For now, remote transcoding uses the same GRPCTranscoder but will connect
	// to the remote daemon's gRPC server instead of spawning a local subprocess.
	// The daemon's address is used to establish the connection.
	//
	// TODO: Implement RemoteGRPCTranscoder that connects to daemon.Address
	// For the initial implementation, we fall back to local gRPC if remote
	// is selected but true remote connection is not yet implemented.

	f.Logger.Info("Remote transcoder requested (falling back to local for now)",
		slog.String("id", id),
		slog.String("daemon_id", string(daemon.ID)),
		slog.String("daemon_address", daemon.Address),
		slog.String("source", sourceVariant.String()),
		slog.String("target", targetVariant.String()),
		slog.String("video_encoder", videoEncoder),
		slog.String("audio_encoder", audioEncoder),
	)

	// Fall back to local gRPC transcoding for now
	// The remote transcoder implementation will be added in a follow-up
	return f.createGRPCTranscoder(id, buffer, sourceVariant, targetVariant,
		videoEncoder, audioEncoder, videoBitrate, audioBitrate,
		videoPreset, hwAccel, hwAccelDevice, opts)
}

// isHardwareEncoder returns true if the encoder name indicates hardware encoding.
func isHardwareEncoder(encoder string) bool {
	// Common hardware encoder suffixes/patterns
	hwPatterns := []string{
		"_nvenc",  // NVIDIA NVENC
		"_qsv",    // Intel QuickSync
		"_vaapi",  // VA-API
		"_videotoolbox", // Apple VideoToolbox
		"_amf",    // AMD AMF
		"_mf",     // Windows Media Foundation
		"_cuvid",  // NVIDIA CUVID (decoder but check anyway)
	}

	for _, pattern := range hwPatterns {
		if len(encoder) > len(pattern) && encoder[len(encoder)-len(pattern):] == pattern {
			return true
		}
	}
	return false
}

// mapVariantToEncoders maps a codec variant to FFmpeg encoder names.
func mapVariantToEncoders(variant CodecVariant) (videoEncoder, audioEncoder string) {
	videoCodecStr := variant.VideoCodec()
	audioCodecStr := variant.AudioCodec()

	switch videoCodecStr {
	case "h264", "avc":
		videoEncoder = "libx264"
	case "h265", "hevc":
		videoEncoder = "libx265"
	case "vp9":
		videoEncoder = "libvpx-vp9"
	case "av1":
		videoEncoder = "libaom-av1"
	case "copy", "":
		videoEncoder = "copy"
	default:
		videoEncoder = videoCodecStr
	}

	switch audioCodecStr {
	case "aac":
		audioEncoder = "aac"
	case "ac3":
		audioEncoder = "ac3"
	case "opus":
		audioEncoder = "libopus"
	case "mp3":
		audioEncoder = "libmp3lame"
	case "copy", "":
		audioEncoder = "copy"
	default:
		audioEncoder = audioCodecStr
	}

	return videoEncoder, audioEncoder
}

// TranscoderConfig holds common configuration for creating transcoders.
type TranscoderConfig struct {
	// ID is the unique identifier for this transcoder instance.
	ID string

	// Buffer is the shared ES buffer for reading/writing samples.
	Buffer *SharedESBuffer

	// SourceVariant is the source codec variant to read from.
	SourceVariant CodecVariant

	// TargetVariant is the target codec variant to produce.
	TargetVariant CodecVariant

	// VideoEncoder is the target video encoder (e.g., "libx264", "h264_nvenc").
	VideoEncoder string

	// AudioEncoder is the target audio encoder (e.g., "aac", "libopus").
	AudioEncoder string

	// VideoBitrate in kbps (0 for default).
	VideoBitrate int

	// AudioBitrate in kbps (0 for default).
	AudioBitrate int

	// VideoPreset for encoding speed/quality tradeoff.
	VideoPreset string

	// HWAccel hardware acceleration type (empty for software).
	HWAccel string

	// HWAccelDevice hardware acceleration device path.
	HWAccelDevice string

	// SourceURL for direct input mode (bypasses ES demux/mux).
	SourceURL string

	// UseDirectInput enables direct URL input mode.
	UseDirectInput bool
}

// TranscoderType indicates the type of transcoder.
type TranscoderType string

const (
	TranscoderTypeDirect TranscoderType = "direct" // Direct FFmpeg process
	TranscoderTypeGRPC   TranscoderType = "grpc"   // Via ffmpegd subprocess
	TranscoderTypeRemote TranscoderType = "remote" // Via remote ffmpegd daemon
)

// TranscoderInfo contains runtime information about a transcoder.
type TranscoderInfo struct {
	ID            string
	Type          TranscoderType
	SourceVariant CodecVariant
	TargetVariant CodecVariant
	VideoEncoder  string
	AudioEncoder  string
	HWAccel       string
	StartedAt     time.Time
	LastActivity  time.Time
}
