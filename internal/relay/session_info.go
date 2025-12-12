// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"time"
)

// RouteType represents the type of route used for a relay session.
type RouteType string

const (
	// RouteTypePassthrough indicates the stream is passed through without modification.
	RouteTypePassthrough RouteType = "passthrough"
	// RouteTypeRepackage indicates the stream is repackaged (container change without transcode).
	RouteTypeRepackage RouteType = "repackage"
	// RouteTypeTranscode indicates the stream is transcoded via FFmpeg.
	RouteTypeTranscode RouteType = "transcode"
)

// RelaySessionInfo provides a view of relay session data optimized for flow visualization.
// It aggregates information needed to render the relay flow diagram with nodes and edges.
type RelaySessionInfo struct {
	// Session identity
	SessionID        string `json:"session_id"`
	ChannelID        string `json:"channel_id"`
	ChannelName      string `json:"channel_name"`
	StreamSourceName string `json:"stream_source_name,omitempty"` // Name of the stream source (e.g., "s8k")
	ProfileName      string `json:"profile_name"`

	// Route type determines the processing path
	RouteType RouteType `json:"route_type"`

	// Source information
	SourceURL    string `json:"source_url"`
	SourceFormat string `json:"source_format"` // hls, dash, mpegts, etc.

	// Output information
	OutputFormat           string   `json:"output_format"`                      // hls, dash, mpegts, etc. (primary/default)
	ActiveProcessorFormats []string `json:"active_processor_formats,omitempty"` // Actually running processors (mpegts, hls, dash)

	// Codec information (source)
	VideoCodec  string  `json:"video_codec,omitempty"`  // Source video codec
	AudioCodec  string  `json:"audio_codec,omitempty"`  // Source audio codec
	Framerate   float64 `json:"framerate,omitempty"`    // Video framerate (fps)
	VideoWidth  int     `json:"video_width,omitempty"`  // Video width in pixels
	VideoHeight int     `json:"video_height,omitempty"` // Video height in pixels

	// Target codec information (only for transcode sessions)
	TargetVideoCodec string `json:"target_video_codec,omitempty"` // Target video codec (e.g., h265)
	TargetAudioCodec string `json:"target_audio_codec,omitempty"` // Target audio codec (e.g., aac)

	// Encoder information (only for transcode sessions) - what FFmpeg uses
	VideoEncoder string `json:"video_encoder,omitempty"` // FFmpeg video encoder (e.g., libx265, h264_nvenc)
	AudioEncoder string `json:"audio_encoder,omitempty"` // FFmpeg audio encoder (e.g., aac, libopus)

	// Hardware acceleration (only for transcode sessions)
	HWAccelType   string `json:"hwaccel_type,omitempty"`   // Hardware acceleration type (e.g., cuda, qsv, vaapi)
	HWAccelDevice string `json:"hwaccel_device,omitempty"` // Hardware acceleration device (e.g., /dev/dri/renderD128)

	// Timing
	StartedAt    time.Time `json:"started_at"`
	LastActivity time.Time `json:"last_activity"`
	DurationSecs float64   `json:"duration_secs"`

	// Connected clients
	ClientCount int               `json:"client_count"`
	Clients     []RelayClientInfo `json:"clients,omitempty"`

	// Bytes transferred
	BytesIn  uint64 `json:"bytes_in"`  // From upstream
	BytesOut uint64 `json:"bytes_out"` // To all clients combined

	// Throughput (bytes per second, calculated over recent window)
	IngressRateBps uint64 `json:"ingress_rate_bps"`
	EgressRateBps  uint64 `json:"egress_rate_bps"`

	// Resource usage (only populated for transcode sessions with FFmpeg)
	// These are nullable/omitempty since passthrough/repackage don't use FFmpeg
	CPUPercent    *float64 `json:"cpu_percent,omitempty"`
	MemoryBytes   *uint64  `json:"memory_bytes,omitempty"`
	MemoryPercent *float64 `json:"memory_percent,omitempty"`

	// Encoding speed (1.0 = realtime, 2.0 = 2x realtime, 0.5 = half realtime)
	EncodingSpeed *float64 `json:"encoding_speed,omitempty"`

	// Resource history for sparkline graphs (last 30 samples, ~1 sample/sec)
	CPUHistory    []float64 `json:"cpu_history,omitempty"`    // Historical CPU percentage values
	MemoryHistory []float64 `json:"memory_history,omitempty"` // Historical memory MB values

	// FFmpeg process info (only for transcode sessions)
	FFmpegPID   *int             `json:"ffmpeg_pid,omitempty"`
	FFmpegStats *FFmpegStatsInfo `json:"ffmpeg_stats,omitempty"` // Detailed FFmpeg stats

	// Buffer statistics
	BufferUtilization *float64            `json:"buffer_utilization,omitempty"` // 0-100 percentage
	SegmentCount      *int                `json:"segment_count,omitempty"`
	BufferVariants    []BufferVariantInfo `json:"buffer_variants,omitempty"` // Variants in shared buffer

	// Status
	InFallback bool   `json:"in_fallback"`
	Error      string `json:"error,omitempty"`
}

// FFmpegStatsInfo contains detailed FFmpeg process statistics.
type FFmpegStatsInfo struct {
	PID           int     `json:"pid"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryRSSMB   float64 `json:"memory_rss_mb"`
	MemoryPercent float64 `json:"memory_percent"`
	BytesWritten  uint64  `json:"bytes_written"`
	WriteRateMbps float64 `json:"write_rate_mbps"`
	DurationSecs  float64 `json:"duration_secs"`

	// Encoding speed (1.0 = realtime, 2.0 = 2x realtime, 0.5 = half realtime)
	EncodingSpeed float64 `json:"encoding_speed,omitempty"`

	// Resource history for sparkline graphs (last 30 samples, ~1 sample/sec)
	CPUHistory    []float64 `json:"cpu_history,omitempty"`    // Historical CPU percentage values
	MemoryHistory []float64 `json:"memory_history,omitempty"` // Historical memory MB values
}

// Note: BufferVariantInfo is defined in flow_types.go

// RelayClientInfo provides information about a connected client.
type RelayClientInfo struct {
	ClientID      string    `json:"client_id"`
	UserAgent     string    `json:"user_agent,omitempty"`
	RemoteAddr    string    `json:"remote_addr,omitempty"`
	PlayerType    string    `json:"player_type,omitempty"`    // Extracted from X-Tvarr-Player header
	DetectionRule string    `json:"detection_rule,omitempty"` // Client detection rule that matched
	ClientFormat  string    `json:"client_format,omitempty"`  // Format this client is using (hls, mpegts, dash)
	ConnectedAt   time.Time `json:"connected_at"`
	ConnectedSecs float64   `json:"connected_secs"` // How long the client has been connected
	BytesRead     uint64    `json:"bytes_read"`
}

// ToSessionInfo converts SessionStats to RelaySessionInfo for flow visualization.
func (s *SessionStats) ToSessionInfo() RelaySessionInfo {
	info := RelaySessionInfo{
		SessionID:              s.ID,
		ChannelID:              s.ChannelID,
		ChannelName:            s.ChannelName,
		StreamSourceName:       s.StreamSourceName,
		ProfileName:            s.ProfileName,
		SourceURL:              s.StreamURL,
		SourceFormat:           s.SourceFormat,
		OutputFormat:           s.ClientFormat,
		ActiveProcessorFormats: s.ActiveProcessorFormats,
		StartedAt:              s.StartedAt,
		LastActivity:           s.LastActivity,
		ClientCount:            s.ClientCount,
		BytesIn:                s.BytesFromUpstream,
		BytesOut:               s.BytesWritten,
		InFallback:             s.InFallback,
		Error:                  s.Error,
	}

	// Calculate duration and rates
	var durationSecs float64
	if !s.StartedAt.IsZero() {
		durationSecs = time.Since(s.StartedAt).Seconds()
		info.DurationSecs = durationSecs

		// Calculate ingress/egress rates (bytes per second)
		if durationSecs > 0 {
			info.IngressRateBps = uint64(float64(s.BytesFromUpstream) / durationSecs)
			info.EgressRateBps = uint64(float64(s.BytesWritten) / durationSecs)
		}
	}

	// Copy codec info
	info.VideoCodec = s.VideoCodec
	info.AudioCodec = s.AudioCodec
	info.Framerate = s.Framerate
	info.VideoWidth = s.VideoWidth
	info.VideoHeight = s.VideoHeight

	// Determine route type from delivery decision
	switch s.DeliveryDecision {
	case "passthrough":
		info.RouteType = RouteTypePassthrough
	case "repackage":
		info.RouteType = RouteTypeRepackage
	case "transcode":
		info.RouteType = RouteTypeTranscode
	default:
		// Infer from FFmpeg presence
		if s.FFmpegStats != nil {
			info.RouteType = RouteTypeTranscode
		} else {
			info.RouteType = RouteTypePassthrough
		}
	}

	// Copy FFmpeg stats if present
	if s.FFmpegStats != nil {
		info.FFmpegPID = &s.FFmpegStats.PID
		info.CPUPercent = &s.FFmpegStats.CPUPercent
		memBytes := uint64(s.FFmpegStats.MemoryRSSMB * 1024 * 1024)
		info.MemoryBytes = &memBytes
		info.MemoryPercent = &s.FFmpegStats.MemoryPercent

		// Copy target codecs from FFmpeg stats (these are the transcoded output codecs)
		info.TargetVideoCodec = s.FFmpegStats.VideoCodec
		info.TargetAudioCodec = s.FFmpegStats.AudioCodec

		// Copy encoder names (what FFmpeg uses to produce the codecs)
		info.VideoEncoder = s.FFmpegStats.VideoEncoder
		info.AudioEncoder = s.FFmpegStats.AudioEncoder

		// Copy hardware acceleration info
		info.HWAccelType = s.FFmpegStats.HWAccel
		info.HWAccelDevice = s.FFmpegStats.HWAccelDevice

		info.FFmpegStats = &FFmpegStatsInfo{
			PID:           s.FFmpegStats.PID,
			CPUPercent:    s.FFmpegStats.CPUPercent,
			MemoryRSSMB:   s.FFmpegStats.MemoryRSSMB,
			MemoryPercent: s.FFmpegStats.MemoryPercent,
			BytesWritten:  s.FFmpegStats.BytesWritten,
			WriteRateMbps: s.FFmpegStats.WriteRateMbps,
			DurationSecs:  s.FFmpegStats.DurationSecs,
			EncodingSpeed: s.FFmpegStats.EncodingSpeed,
			CPUHistory:    s.FFmpegStats.CPUHistory,
			MemoryHistory: s.FFmpegStats.MemoryHistory,
		}

		// Copy encoding speed to session info level
		if s.FFmpegStats.EncodingSpeed > 0 {
			info.EncodingSpeed = &s.FFmpegStats.EncodingSpeed
		}

		// Copy resource history to session info level as well
		info.CPUHistory = s.FFmpegStats.CPUHistory
		info.MemoryHistory = s.FFmpegStats.MemoryHistory
	}

	// Copy buffer stats if present
	if s.SegmentBufferStats != nil {
		info.BufferUtilization = &s.SegmentBufferStats.BufferUtilization
		info.SegmentCount = &s.SegmentBufferStats.SegmentCount
	}

	// Copy ES buffer variant stats if present
	if s.ESBufferStats != nil {
		for _, v := range s.ESBufferStats.Variants {
			// Convert from ESVariantStats to BufferVariantInfo
			info.BufferVariants = append(info.BufferVariants, BufferVariantInfo{
				Variant:        string(v.Variant),
				VideoCodec:     v.VideoCodec,
				AudioCodec:     v.AudioCodec,
				VideoSamples:   v.VideoSamples,
				AudioSamples:   v.AudioSamples,
				BytesIngested:  v.CurrentBytes, // Use current resident bytes, not total ingested
				MaxBytes:       v.MaxBytes,
				Utilization:    v.ByteUtilization,
				IsSource:       v.IsSource,
				IsEvicting:     v.IsEvicting,
				BufferDuration: v.BufferDuration.Seconds(),
				EvictedSamples: v.EvictedSamples,
				EvictedBytes:   v.EvictedBytes,
			})
		}
	}

	// Convert clients
	for _, c := range s.Clients {
		connectedSecs := 0.0
		if !c.ConnectedAt.IsZero() {
			connectedSecs = time.Since(c.ConnectedAt).Seconds()
		}
		clientInfo := RelayClientInfo{
			ClientID:      c.ID,
			UserAgent:     c.UserAgent,
			RemoteAddr:    c.RemoteAddr,
			ClientFormat:  c.ClientFormat,
			ConnectedAt:   c.ConnectedAt,
			ConnectedSecs: connectedSecs,
			BytesRead:     c.BytesRead,
		}
		// Extract player type from user agent if available
		clientInfo.PlayerType = extractPlayerType(c.UserAgent)
		info.Clients = append(info.Clients, clientInfo)
	}

	return info
}

// extractPlayerType attempts to identify the player type from the user agent or headers.
func extractPlayerType(userAgent string) string {
	// This is a simple heuristic - could be enhanced with X-Tvarr-Player header
	switch {
	case containsIgnoreCase(userAgent, "hls.js"):
		return "hls.js"
	case containsIgnoreCase(userAgent, "mpegts.js"):
		return "mpegts.js"
	case containsIgnoreCase(userAgent, "exoplayer"):
		return "ExoPlayer"
	case containsIgnoreCase(userAgent, "vlc"):
		return "VLC"
	case containsIgnoreCase(userAgent, "mpv"):
		return "mpv"
	case containsIgnoreCase(userAgent, "lavf"):
		// Lavf is libavformat, used by mpv and ffmpeg-based players
		return "mpv"
	case containsIgnoreCase(userAgent, "ffmpeg"):
		return "FFmpeg"
	case containsIgnoreCase(userAgent, "safari"):
		return "Safari"
	case containsIgnoreCase(userAgent, "chrome"):
		return "Chrome"
	case containsIgnoreCase(userAgent, "firefox"):
		return "Firefox"
	default:
		return ""
	}
}

// containsIgnoreCase checks if s contains substr (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	// Simple case-insensitive contains
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	// Convert both to lowercase for comparison
	sl := toLowerASCII(s)
	substrl := toLowerASCII(substr)
	for i := 0; i <= len(sl)-len(substrl); i++ {
		if sl[i:i+len(substrl)] == substrl {
			return true
		}
	}
	return false
}

// toLowerASCII converts ASCII letters to lowercase.
func toLowerASCII(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}
