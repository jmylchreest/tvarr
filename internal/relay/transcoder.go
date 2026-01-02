// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/observability"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/proto"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
)

// Transcoder is the interface for codec transcoding.
// ESTranscoder implements this using ffmpegd (either local subprocess or remote daemon).
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

	// GetResourceHistory returns CPU and memory history for sparkline graphs.
	// Returns nil slices if history tracking is not available.
	GetResourceHistory() (cpuHistory, memHistory []float64)

	// IsClosed returns true if the transcoder is closed.
	IsClosed() bool

	// ClosedChan returns a channel that is closed when transcoder stops.
	ClosedChan() <-chan struct{}
}

// TranscoderStats contains statistics about transcoder operation.
type TranscoderStats struct {
	ID            string
	SourceVariant CodecVariant
	TargetVariant CodecVariant
	StartedAt     time.Time
	LastActivity  time.Time
	SamplesIn     uint64
	SamplesOut    uint64
	BytesIn       uint64
	BytesOut      uint64
	Errors        uint64
	VideoCodec    string
	AudioCodec    string
	VideoEncoder  string
	AudioEncoder  string
	HWAccel       string
	HWAccelDevice string
	EncodingSpeed float64
	FFmpegCommand string
}

// TranscoderProcessStats contains process-level statistics for the transcoder.
type TranscoderProcessStats struct {
	PID           int
	CPUPercent    float64
	MemoryRSSMB   float64
	MemoryPercent float64
	BytesWritten  uint64
	WriteRateMbps float64
}

// EncoderOverridesProvider is a function that returns the current enabled encoder overrides.
// This allows the transcoder factory to fetch overrides from the service layer without
// having a direct dependency on it.
type EncoderOverridesProvider func() []*proto.EncoderOverride

// TranscoderFactory creates transcoders based on configuration and availability.
// All transcoding is done via ffmpegd (either remote daemon or local subprocess).
// Direct FFmpeg transcoding has been removed to simplify the codebase.
type TranscoderFactory struct {
	// Spawner for creating ffmpegd subprocesses.
	// Required for local transcoding when no remote daemons are available.
	Spawner *FFmpegDSpawner

	// DaemonRegistry for remote daemon selection (distributed mode).
	// If nil, only local subprocess is available.
	DaemonRegistry *DaemonRegistry

	// DaemonStreamManager manages persistent streams from remote daemons.
	// Required for remote transcoding via connected daemons.
	DaemonStreamManager *DaemonStreamManager

	// ActiveJobManager manages active transcode jobs on remote daemons.
	// Required for remote transcoding via connected daemons.
	ActiveJobManager *ActiveJobManager

	// SelectionStrategy for choosing remote daemons.
	// Defaults to DefaultSelectionStrategy() if nil.
	SelectionStrategy SelectionStrategy

	// PreferRemote indicates whether to prefer remote daemons over local subprocess.
	// When true and suitable remote daemons are available, routes to remote.
	// When false or no suitable remote available, falls back to local subprocess.
	PreferRemote bool

	// EncoderOverridesProvider fetches enabled encoder overrides.
	// If nil, no overrides are applied.
	EncoderOverridesProvider EncoderOverridesProvider

	// Logger for structured logging.
	Logger *slog.Logger
}

// TranscoderFactoryConfig configures the transcoder factory.
type TranscoderFactoryConfig struct {
	// Spawner for ffmpegd subprocesses. Required for local transcoding.
	Spawner *FFmpegDSpawner

	// DaemonRegistry for remote daemon selection (distributed mode).
	DaemonRegistry *DaemonRegistry

	// DaemonStreamManager manages persistent streams from remote daemons.
	DaemonStreamManager *DaemonStreamManager

	// ActiveJobManager manages active transcode jobs on remote daemons.
	ActiveJobManager *ActiveJobManager

	// SelectionStrategy for choosing remote daemons.
	SelectionStrategy SelectionStrategy

	// PreferRemote indicates preference for remote daemons over local subprocess.
	PreferRemote bool

	// EncoderOverridesProvider fetches enabled encoder overrides.
	EncoderOverridesProvider EncoderOverridesProvider

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
		Spawner:                  config.Spawner,
		DaemonRegistry:           config.DaemonRegistry,
		DaemonStreamManager:      config.DaemonStreamManager,
		ActiveJobManager:         config.ActiveJobManager,
		SelectionStrategy:        strategy,
		PreferRemote:             config.PreferRemote,
		EncoderOverridesProvider: config.EncoderOverridesProvider,
		Logger:                   logger,
	}
}

// CanCreateLocalTranscoder returns whether local ffmpegd subprocess transcoders can be created.
func (f *TranscoderFactory) CanCreateLocalTranscoder() bool {
	return f.Spawner != nil && f.Spawner.IsAvailable()
}

// CanCreateRemoteTranscoder returns whether remote daemon transcoders are available.
// Requires both the daemon registry (for selection) and stream manager (for communication).
func (f *TranscoderFactory) CanCreateRemoteTranscoder() bool {
	return f.DaemonRegistry != nil &&
		f.DaemonStreamManager != nil &&
		f.ActiveJobManager != nil &&
		f.DaemonRegistry.CountActive() > 0 &&
		f.DaemonStreamManager.Count() > 0
}

// ShouldUseRemote returns whether remote daemon transcoding should be used.
func (f *TranscoderFactory) ShouldUseRemote() bool {
	return f.PreferRemote && f.CanCreateRemoteTranscoder()
}

// SelectRemoteDaemon selects a remote daemon for the given codec requirements.
// sourceCodec and targetCodec are normalized codec names (e.g., "h264", "hevc", "vp9").
// The daemon selection will prefer daemons with HW encoders for the target codec.
// Returns nil if no suitable daemon is available.
func (f *TranscoderFactory) SelectRemoteDaemon(sourceCodec, targetCodec string, requireGPU bool) *types.Daemon {
	if f.DaemonRegistry == nil {
		return nil
	}

	criteria := SelectionCriteria{
		SourceCodec: sourceCodec,
		TargetCodec: targetCodec,
		RequireGPU:  requireGPU,
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

	// Custom FFmpeg flags from encoding profile.
	GlobalFlags string // Flags placed at the start of the command
	InputFlags  string // Flags placed before -i input
	OutputFlags string // Flags placed after -i input

	// OutputFormat specifies the container format for daemon FFmpeg output.
	// Values: "fmp4", "mpegts". If empty, auto-selected based on target codec.
	OutputFormat string
}

// CreateTranscoderFromProfile creates a transcoder from an encoding profile.
// The targetVariant parameter can override the profile's target codecs (e.g., from client detection).
// If targetVariant is VariantCopy, the profile's target codecs are used.
// Priority order:
// 1. Remote daemon (if PreferRemote and suitable daemon available)
// 2. Local ffmpegd subprocess (fallback)
func (f *TranscoderFactory) CreateTranscoderFromProfile(
	id string,
	buffer *SharedESBuffer,
	sourceVariant CodecVariant,
	targetVariant CodecVariant,
	profile *models.EncodingProfile,
	opts CreateTranscoderOptions,
) (Transcoder, error) {
	if profile == nil {
		return nil, fmt.Errorf("profile is required")
	}

	f.Logger.Log(context.Background(), observability.LevelTrace, "CreateTranscoderFromProfile called",
		slog.String("id", id),
		slog.String("source_variant", sourceVariant.String()),
		slog.String("target_variant", targetVariant.String()),
		slog.String("profile_video_codec", string(profile.TargetVideoCodec)),
		slog.String("profile_audio_codec", string(profile.TargetAudioCodec)))

	// If targetVariant is VariantCopy or empty, use profile's target codecs
	profileVariant := NewCodecVariant(
		string(profile.TargetVideoCodec),
		string(profile.TargetAudioCodec),
	)
	if targetVariant == VariantCopy || (targetVariant.VideoCodec() == "" && targetVariant.AudioCodec() == "") || targetVariant == "" {
		f.Logger.Log(context.Background(), observability.LevelTrace, "Target variant was copy/empty, using profile variant",
			slog.String("old_target", targetVariant.String()),
			slog.String("new_target", profileVariant.String()))
		targetVariant = profileVariant
	}

	// Get encoding parameters from quality preset
	encodingParams := profile.GetEncodingParams()
	videoBitrate := profile.GetVideoBitrate()
	audioBitrate := profile.GetAudioBitrate()
	videoPreset := encodingParams.VideoPreset
	hwAccel := string(profile.HWAccel)

	// Populate custom FFmpeg flags from profile (if not already set in opts)
	if opts.GlobalFlags == "" {
		opts.GlobalFlags = profile.GlobalFlags
	}
	if opts.InputFlags == "" {
		opts.InputFlags = profile.InputFlags
	}
	if opts.OutputFlags == "" {
		opts.OutputFlags = profile.OutputFlags
	}

	// Determine encoders based on target variant
	// If target differs from profile, we need to map the target codecs to encoders
	var videoEncoder, audioEncoder string
	if targetVariant == profileVariant {
		// Use profile's encoders (may include hardware encoder preferences)
		videoEncoder = profile.GetVideoEncoder()
		audioEncoder = profile.GetAudioEncoder()
	} else {
		// Target variant was overridden (e.g., by client detection)
		// Map the target codecs to appropriate software encoders
		videoEncoder, audioEncoder = mapVariantToEncoders(targetVariant)
		f.Logger.Log(context.Background(), observability.LevelTrace, "Using override target variant with mapped encoders",
			slog.String("id", id),
			slog.String("target_variant", targetVariant.String()),
			slog.String("profile_variant", profileVariant.String()),
			slog.String("video_encoder", videoEncoder),
			slog.String("audio_encoder", audioEncoder),
		)
	}

	// Determine if HW encoding is explicitly requested via encoder name
	requireGPU := isHardwareEncoder(videoEncoder)

	// 1. Try remote daemon first if preferred
	// Use codec-level selection so daemons with HW encoders (e.g., vp9_vaapi) are found
	// even when the default software encoder (e.g., libvpx-vp9) was mapped
	if f.ShouldUseRemote() {
		// Log available daemons for debugging
		if f.DaemonRegistry != nil {
			available := f.DaemonRegistry.GetAvailable()
			f.Logger.Info("Checking remote daemons for transcoding",
				slog.String("id", id),
				slog.Int("available_daemons", len(available)),
				slog.String("source_codec", sourceVariant.VideoCodec()),
				slog.String("target_codec", targetVariant.VideoCodec()),
				slog.Bool("require_gpu", requireGPU),
			)
			for _, d := range available {
				hasCaps := d.Capabilities != nil
				var capInfo string
				if hasCaps {
					capInfo = fmt.Sprintf("encoders=%d, gpus=%d", len(d.Capabilities.VideoEncoders), len(d.Capabilities.GPUs))
				} else {
					capInfo = "no capabilities"
				}
				f.Logger.Info("Available daemon",
					slog.String("daemon_id", string(d.ID)),
					slog.String("daemon_name", d.Name),
					slog.String("state", d.State.String()),
					slog.String("capabilities", capInfo),
				)
			}
		}
		daemon := f.SelectRemoteDaemon(sourceVariant.VideoCodec(), targetVariant.VideoCodec(), requireGPU)
		if daemon != nil {
			f.Logger.Info("Selected remote daemon for transcoding",
				slog.String("id", id),
				slog.String("daemon_id", string(daemon.ID)),
				slog.String("daemon_name", daemon.Name),
				slog.String("source_codec", sourceVariant.VideoCodec()),
				slog.String("target_codec", targetVariant.VideoCodec()),
			)
			return f.createRemoteTranscoder(id, buffer, sourceVariant, targetVariant,
				videoEncoder, audioEncoder, videoBitrate, audioBitrate,
				videoPreset, hwAccel, "", daemon, opts)
		}
		f.Logger.Warn("No suitable remote daemon found, falling back to local subprocess",
			slog.String("id", id),
			slog.String("source_codec", sourceVariant.VideoCodec()),
			slog.String("target_codec", targetVariant.VideoCodec()),
			slog.Bool("require_gpu", requireGPU),
		)
	} else {
		// Detailed breakdown of why remote is not available
		hasRegistry := f.DaemonRegistry != nil
		hasStreamMgr := f.DaemonStreamManager != nil
		hasJobMgr := f.ActiveJobManager != nil
		var activeCount, streamCount int
		if hasRegistry {
			activeCount = f.DaemonRegistry.CountActive()
		}
		if hasStreamMgr {
			streamCount = f.DaemonStreamManager.Count()
		}
		f.Logger.Info("Remote transcoding not preferred or not available",
			slog.String("id", id),
			slog.Bool("prefer_remote", f.PreferRemote),
			slog.Bool("can_use_remote", f.CanCreateRemoteTranscoder()),
			slog.Bool("has_registry", hasRegistry),
			slog.Bool("has_stream_mgr", hasStreamMgr),
			slog.Bool("has_job_mgr", hasJobMgr),
			slog.Int("active_daemons", activeCount),
			slog.Int("stream_count", streamCount),
		)
	}

	// 2. Fall back to local ffmpegd subprocess
	if f.CanCreateLocalTranscoder() {
		return f.createLocalTranscoder(id, buffer, sourceVariant, targetVariant,
			videoEncoder, audioEncoder, videoBitrate, audioBitrate,
			videoPreset, hwAccel, "", opts)
	}

	return nil, fmt.Errorf("no transcoding backend available: tvarr-ffmpegd binary not found")
}

// CreateTranscoderFromVariant creates a transcoder from source and target variants.
// Uses default settings for bitrate, preset, etc.
// Priority order:
// 1. Remote daemon (if PreferRemote and suitable daemon available)
// 2. Local ffmpegd subprocess (fallback)
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
	// Use codec-level selection so daemons with HW encoders are found
	if f.ShouldUseRemote() {
		daemon := f.SelectRemoteDaemon(sourceVariant.VideoCodec(), targetVariant.VideoCodec(), requireGPU)
		if daemon != nil {
			f.Logger.Log(context.Background(), observability.LevelTrace, "Selected remote daemon for transcoding",
				slog.String("id", id),
				slog.String("daemon_id", string(daemon.ID)),
				slog.String("source_codec", sourceVariant.VideoCodec()),
				slog.String("target_codec", targetVariant.VideoCodec()),
			)
			return f.createRemoteTranscoder(id, buffer, sourceVariant, targetVariant,
				videoEncoder, audioEncoder, 0, 0, "medium", "", "", daemon, opts)
		}
	}

	// 2. Fall back to local ffmpegd subprocess
	if f.CanCreateLocalTranscoder() {
		return f.createLocalTranscoder(id, buffer, sourceVariant, targetVariant,
			videoEncoder, audioEncoder, 0, 0, "medium", "", "", opts)
	}

	return nil, fmt.Errorf("no transcoding backend available: tvarr-ffmpegd binary not found")
}

// createLocalTranscoder creates a local ffmpegd subprocess transcoder.
func (f *TranscoderFactory) createLocalTranscoder(
	id string,
	buffer *SharedESBuffer,
	sourceVariant, targetVariant CodecVariant,
	videoEncoder, audioEncoder string,
	videoBitrate, audioBitrate int,
	videoPreset, hwAccel, hwAccelDevice string,
	opts CreateTranscoderOptions,
) (Transcoder, error) {
	// Fetch encoder overrides from provider if available
	var encoderOverrides []*proto.EncoderOverride
	if f.EncoderOverridesProvider != nil {
		encoderOverrides = f.EncoderOverridesProvider()
	}

	// Note: VideoEncoder/AudioEncoder are no longer used - the daemon auto-selects
	// the best encoder based on target codec and available hardware capabilities.

	// Compute output format if not specified
	outputFormat := opts.OutputFormat
	if outputFormat == "" {
		// Create StreamTarget to compute format based on codec requirements
		target := NewStreamTarget(targetVariant.VideoCodec(), targetVariant.AudioCodec(), "")
		outputFormat = target.DaemonOutputFormat()
	}

	// Determine job type from encoder (for local subprocess tracking)
	jobType := JobTypeFromEncoder(videoEncoder)

	config := ESTranscoderConfig{
		SourceVariant:    sourceVariant,
		TargetVariant:    targetVariant,
		VideoBitrate:     videoBitrate,
		AudioBitrate:     audioBitrate,
		VideoPreset:      videoPreset,
		HWAccel:          hwAccel,
		HWAccelDevice:    hwAccelDevice,
		SourceURL:        opts.SourceURL,
		UseDirectInput:   opts.UseDirectInput,
		ChannelName:      opts.ChannelName,
		GlobalFlags:      opts.GlobalFlags,
		InputFlags:       opts.InputFlags,
		OutputFlags:      opts.OutputFlags,
		OutputFormat:     outputFormat,
		EncoderOverrides: encoderOverrides,
		JobType:          jobType,
		Logger:           f.Logger,
	}

	f.Logger.Log(context.Background(), observability.LevelTrace, "Creating local ES transcoder",
		slog.String("id", id),
		slog.String("source", sourceVariant.String()),
		slog.String("target", targetVariant.String()),
		slog.String("output_format", outputFormat),
		slog.String("job_type", jobType.String()),
		slog.Bool("direct_input", opts.UseDirectInput),
		slog.Int("encoder_overrides", len(encoderOverrides)))

	return NewLocalESTranscoder(id, buffer, f.Spawner, f.DaemonStreamManager, f.ActiveJobManager, config), nil
}

// createRemoteTranscoder creates a transcoder that uses a remote ffmpegd daemon.
// This is for distributed transcoding where the work is offloaded to remote workers.
// The daemon maintains a persistent bidirectional gRPC stream with the coordinator,
// and we push transcoding work through that stream.
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
	// Check if we have the required managers
	if f.DaemonStreamManager == nil || f.ActiveJobManager == nil {
		f.Logger.Log(context.Background(), observability.LevelTrace, "Stream/job managers not available, falling back to local subprocess",
			slog.String("id", id),
			slog.String("daemon_id", string(daemon.ID)),
		)
		// Fall back to local ffmpegd subprocess
		if f.CanCreateLocalTranscoder() {
			return f.createLocalTranscoder(id, buffer, sourceVariant, targetVariant,
				videoEncoder, audioEncoder, videoBitrate, audioBitrate,
				videoPreset, hwAccel, hwAccelDevice, opts)
		}
		return nil, fmt.Errorf("no transcoding backend available: tvarr-ffmpegd binary not found")
	}

	// Determine job type based on encoder
	jobType := JobTypeFromEncoder(videoEncoder)

	// Check if the daemon has capacity for this job type
	if _, ok := f.DaemonStreamManager.GetStreamWithCapacityForType(daemon.ID, jobType); !ok {
		f.Logger.Log(context.Background(), observability.LevelTrace, "Daemon has no capacity for job type, falling back to local subprocess",
			slog.String("id", id),
			slog.String("daemon_id", string(daemon.ID)),
			slog.String("daemon_name", daemon.Name),
			slog.String("job_type", jobType.String()),
			slog.String("video_encoder", videoEncoder),
		)
		// Fall back to local ffmpegd subprocess
		if f.CanCreateLocalTranscoder() {
			return f.createLocalTranscoder(id, buffer, sourceVariant, targetVariant,
				videoEncoder, audioEncoder, videoBitrate, audioBitrate,
				videoPreset, hwAccel, hwAccelDevice, opts)
		}
		return nil, fmt.Errorf("no transcoding backend available: tvarr-ffmpegd binary not found")
	}

	// Fetch encoder overrides from provider if available
	var encoderOverrides []*proto.EncoderOverride
	if f.EncoderOverridesProvider != nil {
		encoderOverrides = f.EncoderOverridesProvider()
	}

	// Note: VideoEncoder/AudioEncoder are no longer used - the daemon auto-selects
	// the best encoder based on target codec and available hardware capabilities.

	// Compute output format if not specified
	outputFormat := opts.OutputFormat
	if outputFormat == "" {
		// Create StreamTarget to compute format based on codec requirements
		target := NewStreamTarget(targetVariant.VideoCodec(), targetVariant.AudioCodec(), "")
		outputFormat = target.DaemonOutputFormat()
	}

	config := ESTranscoderConfig{
		SourceVariant:    sourceVariant,
		TargetVariant:    targetVariant,
		VideoBitrate:     videoBitrate,
		AudioBitrate:     audioBitrate,
		VideoPreset:      videoPreset,
		HWAccel:          hwAccel,
		HWAccelDevice:    hwAccelDevice,
		ChannelName:      opts.ChannelName,
		SessionID:        id, // Use transcoder ID as session ID for now
		SourceURL:        opts.SourceURL,
		UseDirectInput:   opts.UseDirectInput,
		GlobalFlags:      opts.GlobalFlags,
		InputFlags:       opts.InputFlags,
		OutputFlags:      opts.OutputFlags,
		OutputFormat:     outputFormat,
		EncoderOverrides: encoderOverrides,
		JobType:          jobType,
		Logger:           f.Logger,
	}

	f.Logger.Info("Creating remote ES transcoder via daemon stream",
		slog.String("id", id),
		slog.String("daemon_id", string(daemon.ID)),
		slog.String("daemon_name", daemon.Name),
		slog.String("source", sourceVariant.String()),
		slog.String("target", targetVariant.String()),
		slog.String("output_format", outputFormat),
		slog.String("video_encoder", videoEncoder),
		slog.String("audio_encoder", audioEncoder),
		slog.String("job_type", jobType.String()),
		slog.Int("encoder_overrides", len(encoderOverrides)),
	)

	return NewRemoteESTranscoder(id, buffer, daemon, f.DaemonStreamManager, f.ActiveJobManager, config), nil
}

// isHardwareEncoder returns true if the encoder name indicates hardware encoding.
func isHardwareEncoder(encoder string) bool {
	// Common hardware encoder suffixes/patterns
	hwPatterns := []string{
		"_nvenc",        // NVIDIA NVENC
		"_qsv",          // Intel QuickSync
		"_vaapi",        // VA-API
		"_videotoolbox", // Apple VideoToolbox
		"_amf",          // AMD AMF
		"_mf",           // Windows Media Foundation
		"_cuvid",        // NVIDIA CUVID (decoder but check anyway)
	}

	for _, pattern := range hwPatterns {
		if len(encoder) > len(pattern) && encoder[len(encoder)-len(pattern):] == pattern {
			return true
		}
	}
	return false
}

// JobTypeFromEncoder returns the job type (CPU or GPU) based on the encoder.
func JobTypeFromEncoder(encoder string) JobType {
	if isHardwareEncoder(encoder) {
		return JobTypeGPU
	}
	return JobTypeCPU
}

// isHardwareDecoder returns true if the decoder name indicates hardware decoding.
func isHardwareDecoder(decoder string) bool {
	// Common hardware decoder suffixes/patterns
	hwPatterns := []string{
		"_cuvid",        // NVIDIA CUDA Video Decoder
		"_qsv",          // Intel QuickSync
		"_vaapi",        // VA-API
		"_videotoolbox", // Apple VideoToolbox
		"_mediacodec",   // Android MediaCodec
		"_mmal",         // Raspberry Pi MMAL
		"_v4l2m2m",      // V4L2 Memory-to-Memory
		"_rkmpp",        // Rockchip MPP
	}

	for _, pattern := range hwPatterns {
		if len(decoder) > len(pattern) && decoder[len(decoder)-len(pattern):] == pattern {
			return true
		}
	}
	return false
}

// daemonHasHWDecoderForCodec checks if the daemon's decoder list contains
// a hardware decoder for the given codec.
// e.g., for codec "h264", checks for "h264_cuvid", "h264_qsv", etc.
func daemonHasHWDecoderForCodec(decoders []string, codec string) bool {
	// Normalize codec aliases to match FFmpeg decoder naming
	normalizedCodec := codec
	switch codec {
	case "avc":
		normalizedCodec = "h264"
	case "h265":
		normalizedCodec = "hevc"
	}

	codecPrefix := normalizedCodec + "_"

	for _, dec := range decoders {
		// Check if decoder starts with codec_ and is a HW decoder
		if len(dec) > len(codecPrefix) &&
			dec[:len(codecPrefix)] == codecPrefix &&
			isHardwareDecoder(dec) {
			return true
		}
	}
	return false
}

// CanDoFullHWTranscode returns true if the daemon can do hardware decoding
// of the source codec AND hardware encoding of the target codec.
// This is the optimal case where frames stay on the GPU throughout.
func CanDoFullHWTranscode(sourceCodec, targetCodec string, daemon *types.Daemon) bool {
	if daemon == nil || daemon.Capabilities == nil || len(daemon.Capabilities.GPUs) == 0 {
		return false
	}

	// Check if daemon has any GPU with encode capacity
	hasGPUWithCapacity := false
	for _, gpu := range daemon.Capabilities.GPUs {
		if gpu.MaxEncodeSessions > 0 {
			hasGPUWithCapacity = true
			break
		}
	}
	if !hasGPUWithCapacity {
		return false
	}

	// Check for HW decoder for source AND HW encoder for target
	hasHWDecoder := daemonHasHWDecoderForCodec(daemon.Capabilities.VideoDecoders, sourceCodec)
	hasHWEncoder := daemonHasHWEncoderForCodec(daemon.Capabilities.VideoEncoders, targetCodec)

	return hasHWDecoder && hasHWEncoder
}

// HasHWEncoder returns true if the daemon has a hardware encoder for the target codec.
func HasHWEncoder(targetCodec string, daemon *types.Daemon) bool {
	if daemon == nil || daemon.Capabilities == nil {
		return false
	}
	return daemonHasHWEncoderForCodec(daemon.Capabilities.VideoEncoders, targetCodec)
}

// HasHWDecoder returns true if the daemon has a hardware decoder for the source codec.
func HasHWDecoder(sourceCodec string, daemon *types.Daemon) bool {
	if daemon == nil || daemon.Capabilities == nil {
		return false
	}
	return daemonHasHWDecoderForCodec(daemon.Capabilities.VideoDecoders, sourceCodec)
}

// PredictJobType predicts the job type based on video encoder and daemon capabilities.
// Video encoding determines the job type since GPU acceleration is video-focused.
// Audio is almost always CPU and doesn't affect the decision.
//
// Rules:
// 1. If video encoder is explicitly HW (e.g., h264_nvenc), returns GPU
// 2. If video encoder is bare codec (e.g., "h264") and daemon has HW encoder for it, returns GPU
// 3. Otherwise returns CPU
func PredictJobType(videoEncoder string, daemon *types.Daemon) JobType {
	// If encoder is explicitly hardware, it's GPU
	if isHardwareEncoder(videoEncoder) {
		return JobTypeGPU
	}

	// For bare codec names or unknown encoders, check daemon capabilities
	// If daemon has GPU capacity and HW encoder for this codec, predict GPU
	if daemon != nil && daemon.Capabilities != nil && len(daemon.Capabilities.GPUs) > 0 {
		// Check if daemon has any GPU with encode capacity
		hasGPUWithCapacity := false
		for _, gpu := range daemon.Capabilities.GPUs {
			if gpu.MaxEncodeSessions > 0 {
				hasGPUWithCapacity = true
				break
			}
		}

		// Check if daemon has a HW encoder for this codec
		if hasGPUWithCapacity && daemonHasHWEncoderForCodec(daemon.Capabilities.VideoEncoders, videoEncoder) {
			return JobTypeGPU
		}
	}

	// Default to CPU
	return JobTypeCPU
}

// daemonHasHWEncoderForCodec checks if the daemon's encoder list contains
// a hardware encoder that can encode the given codec.
// It works by checking if any encoder starts with "codec_" and is a HW encoder.
// e.g., for codec "h264", checks for "h264_nvenc", "h264_qsv", etc.
//
// Handles codec aliases to match FFmpeg's encoder naming:
//   - avc → h264 (FFmpeg uses h264_nvenc, not avc_nvenc)
//   - h265 → hevc (FFmpeg uses hevc_nvenc, not h265_nvenc)
func daemonHasHWEncoderForCodec(encoders []string, codec string) bool {
	// Normalize codec aliases to match FFmpeg encoder naming
	normalizedCodec := codec
	switch codec {
	case "avc":
		normalizedCodec = "h264"
	case "h265":
		normalizedCodec = "hevc"
	}

	codecPrefix := normalizedCodec + "_"

	for _, enc := range encoders {
		// Check if encoder starts with codec_ and is a HW encoder
		if len(enc) > len(codecPrefix) &&
			enc[:len(codecPrefix)] == codecPrefix &&
			isHardwareEncoder(enc) {
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
