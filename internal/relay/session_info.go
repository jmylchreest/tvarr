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
	OutputFormat string `json:"output_format"` // hls, dash, mpegts, etc.

	// Codec information
	VideoCodec string `json:"video_codec,omitempty"` // Source or transcoded codec
	AudioCodec string `json:"audio_codec,omitempty"` // Source or transcoded codec

	// Timing
	StartedAt    time.Time `json:"started_at"`
	LastActivity time.Time `json:"last_activity"`
	DurationSecs float64   `json:"duration_secs"`

	// Connected clients
	ClientCount int                      `json:"client_count"`
	Clients     []RelayClientInfo        `json:"clients,omitempty"`

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

	// FFmpeg process info (only for transcode sessions)
	FFmpegPID *int `json:"ffmpeg_pid,omitempty"`

	// Buffer statistics
	BufferUtilization *float64 `json:"buffer_utilization,omitempty"` // 0-100 percentage
	SegmentCount      *int     `json:"segment_count,omitempty"`

	// Status
	InFallback bool   `json:"in_fallback"`
	Error      string `json:"error,omitempty"`
}

// RelayClientInfo provides information about a connected client.
type RelayClientInfo struct {
	ClientID    string    `json:"client_id"`
	UserAgent   string    `json:"user_agent,omitempty"`
	RemoteAddr  string    `json:"remote_addr,omitempty"`
	PlayerType  string    `json:"player_type,omitempty"` // Extracted from X-Tvarr-Player header
	ConnectedAt time.Time `json:"connected_at"`
	BytesRead   uint64    `json:"bytes_read"`
}

// ToSessionInfo converts SessionStats to RelaySessionInfo for flow visualization.
func (s *SessionStats) ToSessionInfo() RelaySessionInfo {
	info := RelaySessionInfo{
		SessionID:        s.ID,
		ChannelID:        s.ChannelID,
		ChannelName:      s.ChannelName,
		StreamSourceName: s.StreamSourceName,
		ProfileName:      s.ProfileName,
		SourceURL:        s.StreamURL,
		SourceFormat:     s.SourceFormat,
		OutputFormat:     s.ClientFormat,
		StartedAt:        s.StartedAt,
		LastActivity:     s.LastActivity,
		ClientCount:      s.ClientCount,
		BytesIn:          s.BytesFromUpstream,
		BytesOut:         s.BytesWritten,
		InFallback:       s.InFallback,
		Error:            s.Error,
	}

	// Copy codec info
	info.VideoCodec = s.VideoCodec
	info.AudioCodec = s.AudioCodec

	// Calculate duration
	if !s.StartedAt.IsZero() {
		info.DurationSecs = time.Since(s.StartedAt).Seconds()
	}

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
	}

	// Copy buffer stats if present
	if s.SegmentBufferStats != nil {
		info.BufferUtilization = &s.SegmentBufferStats.BufferUtilization
		info.SegmentCount = &s.SegmentBufferStats.SegmentCount
	}

	// Convert clients
	for _, c := range s.Clients {
		clientInfo := RelayClientInfo{
			ClientID:    c.ID,
			UserAgent:   c.UserAgent,
			RemoteAddr:  c.RemoteAddr,
			ConnectedAt: c.ConnectedAt,
			BytesRead:   c.BytesRead,
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
