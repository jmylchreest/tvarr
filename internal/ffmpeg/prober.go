package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ProbeResult contains the complete ffprobe output.
type ProbeResult struct {
	Format  ProbeFormat   `json:"format"`
	Streams []ProbeStream `json:"streams"`
}

// ProbeFormat contains container format information.
type ProbeFormat struct {
	Filename       string            `json:"filename"`
	NumStreams     int               `json:"nb_streams"`
	NumPrograms    int               `json:"nb_programs"`
	FormatName     string            `json:"format_name"`
	FormatLongName string            `json:"format_long_name"`
	StartTime      string            `json:"start_time"`
	Duration       string            `json:"duration"`
	Size           string            `json:"size"`
	BitRate        string            `json:"bit_rate"`
	ProbeScore     int               `json:"probe_score"`
	Tags           map[string]string `json:"tags"`
}

// ProbeStream contains stream information.
type ProbeStream struct {
	Index          int               `json:"index"`
	CodecName      string            `json:"codec_name"`
	CodecLongName  string            `json:"codec_long_name"`
	Profile        string            `json:"profile"`
	CodecType      string            `json:"codec_type"` // video, audio, subtitle, data
	CodecTag       string            `json:"codec_tag_string"`
	Width          int               `json:"width,omitempty"`
	Height         int               `json:"height,omitempty"`
	CodedWidth     int               `json:"coded_width,omitempty"`
	CodedHeight    int               `json:"coded_height,omitempty"`
	HasBFrames     int               `json:"has_b_frames,omitempty"`
	SampleAspect   string            `json:"sample_aspect_ratio,omitempty"`
	DisplayAspect  string            `json:"display_aspect_ratio,omitempty"`
	PixFmt         string            `json:"pix_fmt,omitempty"`
	Level          int               `json:"level,omitempty"`
	ColorRange     string            `json:"color_range,omitempty"`
	ColorSpace     string            `json:"color_space,omitempty"`
	ColorTransfer  string            `json:"color_transfer,omitempty"`
	ColorPrimaries string            `json:"color_primaries,omitempty"`
	FieldOrder     string            `json:"field_order,omitempty"`
	Refs           int               `json:"refs,omitempty"`
	SampleFmt      string            `json:"sample_fmt,omitempty"`
	SampleRate     string            `json:"sample_rate,omitempty"`
	Channels       int               `json:"channels,omitempty"`
	ChannelLayout  string            `json:"channel_layout,omitempty"`
	BitsPerSample  int               `json:"bits_per_sample,omitempty"`
	RFrameRate     string            `json:"r_frame_rate,omitempty"`
	AvgFrameRate   string            `json:"avg_frame_rate,omitempty"`
	TimeBase       string            `json:"time_base,omitempty"`
	StartPts       int64             `json:"start_pts,omitempty"`
	StartTime      string            `json:"start_time,omitempty"`
	Duration       string            `json:"duration,omitempty"`
	DurationTs     int64             `json:"duration_ts,omitempty"`
	BitRate        string            `json:"bit_rate,omitempty"`
	MaxBitRate     string            `json:"max_bit_rate,omitempty"`
	NumFrames      string            `json:"nb_frames,omitempty"`
	Disposition    ProbeDisposition  `json:"disposition,omitempty"`
	Tags           map[string]string `json:"tags,omitempty"`
}

// ProbeDisposition contains stream disposition flags.
type ProbeDisposition struct {
	Default         int `json:"default"`
	Dub             int `json:"dub"`
	Original        int `json:"original"`
	Comment         int `json:"comment"`
	Lyrics          int `json:"lyrics"`
	Karaoke         int `json:"karaoke"`
	Forced          int `json:"forced"`
	HearingImpaired int `json:"hearing_impaired"`
	VisualImpaired  int `json:"visual_impaired"`
	CleanEffects    int `json:"clean_effects"`
	AttachedPic     int `json:"attached_pic"`
}

// VideoTrackInfo contains information about a video track.
type VideoTrackInfo struct {
	Index     int     `json:"index"`               // Stream index in the container
	Codec     string  `json:"codec"`               // Codec name (h264, hevc, vp9, av1, etc.)
	Profile   string  `json:"profile,omitempty"`   // Codec profile (High, Main, etc.)
	Level     string  `json:"level,omitempty"`     // Codec level (4.1, 5.0, etc.)
	Width     int     `json:"width"`               // Frame width
	Height    int     `json:"height"`              // Frame height
	Framerate float64 `json:"framerate,omitempty"` // Framerate (fps)
	Bitrate   int     `json:"bitrate,omitempty"`   // Bitrate in bits/second
	PixFmt    string  `json:"pix_fmt,omitempty"`   // Pixel format (yuv420p, etc.)
	IsDefault bool    `json:"is_default"`          // True if marked as default track
	Language  string  `json:"language,omitempty"`  // Language tag if available
	Title     string  `json:"title,omitempty"`     // Track title if available
}

// AudioTrackInfo contains information about an audio track.
type AudioTrackInfo struct {
	Index         int    `json:"index"`                    // Stream index in the container
	Codec         string `json:"codec"`                    // Codec name (aac, eac3, ac3, mp3, opus, etc.)
	Profile       string `json:"profile,omitempty"`        // Codec profile (LC, HE-AAC, etc.)
	SampleRate    int    `json:"sample_rate,omitempty"`    // Sample rate in Hz
	Channels      int    `json:"channels"`                 // Number of audio channels
	ChannelLayout string `json:"channel_layout,omitempty"` // Channel layout (stereo, 5.1, etc.)
	Bitrate       int    `json:"bitrate,omitempty"`        // Bitrate in bits/second
	IsDefault     bool   `json:"is_default"`               // True if marked as default track
	Language      string `json:"language,omitempty"`       // Language tag if available
	Title         string `json:"title,omitempty"`          // Track title if available
}

// SubtitleTrackInfo contains information about a subtitle track.
type SubtitleTrackInfo struct {
	Index     int    `json:"index"`              // Stream index in the container
	Codec     string `json:"codec"`              // Codec name (subrip, ass, dvd_subtitle, etc.)
	IsDefault bool   `json:"is_default"`         // True if marked as default track
	IsForced  bool   `json:"is_forced"`          // True if marked as forced
	Language  string `json:"language,omitempty"` // Language tag if available
	Title     string `json:"title,omitempty"`    // Track title if available
}

// StreamInfo is a simplified view of stream information.
type StreamInfo struct {
	// Video properties (from selected/default track)
	VideoCodec     string  `json:"video_codec,omitempty"`
	VideoProfile   string  `json:"video_profile,omitempty"`
	VideoLevel     string  `json:"video_level,omitempty"`
	VideoWidth     int     `json:"video_width,omitempty"`
	VideoHeight    int     `json:"video_height,omitempty"`
	VideoFramerate float64 `json:"video_framerate,omitempty"`
	VideoBitrate   int     `json:"video_bitrate,omitempty"`
	VideoPixFmt    string  `json:"video_pix_fmt,omitempty"`

	// Audio properties (from selected/default track)
	AudioCodec      string `json:"audio_codec,omitempty"`
	AudioSampleRate int    `json:"audio_sample_rate,omitempty"`
	AudioChannels   int    `json:"audio_channels,omitempty"`
	AudioBitrate    int    `json:"audio_bitrate,omitempty"`

	// Container properties
	ContainerFormat string `json:"container_format,omitempty"`
	Duration        int64  `json:"duration,omitempty"` // milliseconds, 0 for live
	IsLiveStream    bool   `json:"is_live_stream"`
	HasSubtitles    bool   `json:"has_subtitles"`
	StreamCount     int    `json:"stream_count"`
	Title           string `json:"title,omitempty"`

	// All discovered tracks (for user selection or advanced display)
	VideoTracks    []VideoTrackInfo    `json:"video_tracks,omitempty"`
	AudioTracks    []AudioTrackInfo    `json:"audio_tracks,omitempty"`
	SubtitleTracks []SubtitleTrackInfo `json:"subtitle_tracks,omitempty"`

	// Selected track indices (-1 means auto/first)
	SelectedVideoTrack int `json:"selected_video_track"`
	SelectedAudioTrack int `json:"selected_audio_track"`
}

// Prober handles ffprobe operations.
type Prober struct {
	ffprobePath string
	timeout     time.Duration
}

// NewProber creates a new stream prober.
func NewProber(ffprobePath string) *Prober {
	return &Prober{
		ffprobePath: ffprobePath,
		timeout:     30 * time.Second,
	}
}

// WithTimeout sets the probe timeout.
func (p *Prober) WithTimeout(timeout time.Duration) *Prober {
	p.timeout = timeout
	return p
}

// Probe probes a stream URL and returns detailed information.
func (p *Prober) Probe(ctx context.Context, url string) (*ProbeResult, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		"-timeout", strconv.FormatInt(int64(p.timeout.Seconds())*1000000, 10),
	}

	// Add URL-specific options for network streams
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		args = append(args,
			"-reconnect", "1",
			"-reconnect_streamed", "1",
			"-reconnect_delay_max", "5",
		)
	}

	args = append(args, url)

	cmd := exec.CommandContext(ctx, p.ffprobePath, args...)
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("probe timeout after %v", p.timeout)
		}
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	var result ProbeResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parsing ffprobe output: %w", err)
	}

	return &result, nil
}

// ProbeSimple probes a stream and returns simplified information.
func (p *Prober) ProbeSimple(ctx context.Context, url string) (*StreamInfo, error) {
	result, err := p.Probe(ctx, url)
	if err != nil {
		return nil, err
	}

	return p.simplify(result), nil
}

// simplify converts detailed probe result to simplified stream info.
func (p *Prober) simplify(result *ProbeResult) *StreamInfo {
	info := &StreamInfo{
		ContainerFormat:    result.Format.FormatName,
		StreamCount:        result.Format.NumStreams,
		SelectedVideoTrack: -1, // -1 means auto (use first/default)
		SelectedAudioTrack: -1,
	}

	// Parse duration
	if result.Format.Duration != "" {
		if dur, err := strconv.ParseFloat(result.Format.Duration, 64); err == nil {
			info.Duration = int64(dur * 1000) // Convert to milliseconds
		}
	}

	// Check for live stream indicators
	info.IsLiveStream = info.Duration == 0 ||
		strings.Contains(result.Format.FormatName, "hls") ||
		strings.Contains(result.Format.FormatName, "mpegts")

	// Get title from tags
	if title, ok := result.Format.Tags["title"]; ok {
		info.Title = title
	}

	// Track the default track indices for selection
	defaultVideoIdx := -1
	defaultAudioIdx := -1

	// Process streams - collect ALL tracks
	for _, stream := range result.Streams {
		switch stream.CodecType {
		case "video":
			// Build video track info
			track := VideoTrackInfo{
				Index:     stream.Index,
				Codec:     stream.CodecName,
				Profile:   stream.Profile,
				Width:     stream.Width,
				Height:    stream.Height,
				PixFmt:    stream.PixFmt,
				IsDefault: stream.Disposition.Default == 1,
			}

			// Parse level
			if stream.Level > 0 {
				track.Level = fmt.Sprintf("%.1f", float64(stream.Level)/10)
			}

			// Parse framerate
			if stream.AvgFrameRate != "" {
				track.Framerate = parseFramerate(stream.AvgFrameRate)
			} else if stream.RFrameRate != "" {
				track.Framerate = parseFramerate(stream.RFrameRate)
			}

			// Parse bitrate
			if stream.BitRate != "" {
				if br, err := strconv.Atoi(stream.BitRate); err == nil {
					track.Bitrate = br
				}
			}

			// Extract language and title from tags
			if lang, ok := stream.Tags["language"]; ok {
				track.Language = lang
			}
			if title, ok := stream.Tags["title"]; ok {
				track.Title = title
			}

			info.VideoTracks = append(info.VideoTracks, track)

			// Track which is the default video track
			if track.IsDefault && defaultVideoIdx == -1 {
				defaultVideoIdx = len(info.VideoTracks) - 1
			}

		case "audio":
			// Build audio track info
			track := AudioTrackInfo{
				Index:         stream.Index,
				Codec:         stream.CodecName,
				Profile:       stream.Profile,
				Channels:      stream.Channels,
				ChannelLayout: stream.ChannelLayout,
				IsDefault:     stream.Disposition.Default == 1,
			}

			// Parse sample rate
			if stream.SampleRate != "" {
				if sr, err := strconv.Atoi(stream.SampleRate); err == nil {
					track.SampleRate = sr
				}
			}

			// Parse bitrate
			if stream.BitRate != "" {
				if br, err := strconv.Atoi(stream.BitRate); err == nil {
					track.Bitrate = br
				}
			}

			// Extract language and title from tags
			if lang, ok := stream.Tags["language"]; ok {
				track.Language = lang
			}
			if title, ok := stream.Tags["title"]; ok {
				track.Title = title
			}

			info.AudioTracks = append(info.AudioTracks, track)

			// Track which is the default audio track
			if track.IsDefault && defaultAudioIdx == -1 {
				defaultAudioIdx = len(info.AudioTracks) - 1
			}

		case "subtitle":
			// Build subtitle track info
			track := SubtitleTrackInfo{
				Index:     stream.Index,
				Codec:     stream.CodecName,
				IsDefault: stream.Disposition.Default == 1,
				IsForced:  stream.Disposition.Forced == 1,
			}

			// Extract language and title from tags
			if lang, ok := stream.Tags["language"]; ok {
				track.Language = lang
			}
			if title, ok := stream.Tags["title"]; ok {
				track.Title = title
			}

			info.SubtitleTracks = append(info.SubtitleTracks, track)
			info.HasSubtitles = true
		}
	}

	// Set the "primary" video/audio properties from the selected/default track
	// Default to first track if no default is marked
	selectedVideo := 0
	if defaultVideoIdx >= 0 {
		selectedVideo = defaultVideoIdx
	}
	if len(info.VideoTracks) > 0 {
		vt := info.VideoTracks[selectedVideo]
		info.VideoCodec = vt.Codec
		info.VideoProfile = vt.Profile
		info.VideoLevel = vt.Level
		info.VideoWidth = vt.Width
		info.VideoHeight = vt.Height
		info.VideoFramerate = vt.Framerate
		info.VideoBitrate = vt.Bitrate
		info.VideoPixFmt = vt.PixFmt
		info.SelectedVideoTrack = selectedVideo
	}

	selectedAudio := 0
	if defaultAudioIdx >= 0 {
		selectedAudio = defaultAudioIdx
	}
	if len(info.AudioTracks) > 0 {
		at := info.AudioTracks[selectedAudio]
		info.AudioCodec = at.Codec
		info.AudioSampleRate = at.SampleRate
		info.AudioChannels = at.Channels
		info.AudioBitrate = at.Bitrate
		info.SelectedAudioTrack = selectedAudio
	}

	return info
}

// parseFramerate parses a framerate string like "30000/1001" or "25/1".
func parseFramerate(fr string) float64 {
	parts := strings.Split(fr, "/")
	if len(parts) != 2 {
		if f, err := strconv.ParseFloat(fr, 64); err == nil {
			return f
		}
		return 0
	}

	num, err1 := strconv.ParseFloat(parts[0], 64)
	den, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil || den == 0 {
		return 0
	}

	return num / den
}

// QuickProbe does a fast probe with minimal options.
// Optimized for live streaming with aggressive timeouts for fast startup.
func (p *Prober) QuickProbe(ctx context.Context, url string) (*StreamInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		// Don't use -select_streams - we need both video AND audio streams
		// Time is limited by -read_intervals, -analyzeduration, and -probesize instead
		"-read_intervals", "%+0.5", // Only read first 500ms
		"-analyzeduration", "2000000", // 2 second analyze limit
		"-probesize", "2000000", // 2MB probe limit
		"-timeout", "5000000", // 5 second timeout
	}

	// Add URL-specific options
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		args = append(args, "-reconnect", "1")
	}

	args = append(args, url)

	cmd := exec.CommandContext(ctx, p.ffprobePath, args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("quick probe failed: %w", err)
	}

	var result ProbeResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parsing ffprobe output: %w", err)
	}

	return p.simplify(&result), nil
}

// CheckStreamHealth quickly checks if a stream is accessible.
func (p *Prober) CheckStreamHealth(ctx context.Context, url string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	args := []string{
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_type",
		"-of", "csv=p=0",
		"-timeout", "5000000",
	}

	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		args = append(args, "-reconnect", "1")
	}

	args = append(args, url)

	cmd := exec.CommandContext(ctx, p.ffprobePath, args...)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("stream health check failed: %w", err)
	}

	// Check if we got valid output
	if len(strings.TrimSpace(string(output))) == 0 {
		return fmt.Errorf("no streams found")
	}

	return nil
}

// GetVideoStream returns the first video stream from probe result.
func (r *ProbeResult) GetVideoStream() *ProbeStream {
	for i := range r.Streams {
		if r.Streams[i].CodecType == "video" {
			return &r.Streams[i]
		}
	}
	return nil
}

// GetAudioStream returns the first audio stream from probe result.
func (r *ProbeResult) GetAudioStream() *ProbeStream {
	for i := range r.Streams {
		if r.Streams[i].CodecType == "audio" {
			return &r.Streams[i]
		}
	}
	return nil
}

// GetStreamsByType returns all streams of a given type.
func (r *ProbeResult) GetStreamsByType(codecType string) []ProbeStream {
	var streams []ProbeStream
	for _, s := range r.Streams {
		if s.CodecType == codecType {
			streams = append(streams, s)
		}
	}
	return streams
}

// Duration returns the duration in milliseconds.
func (r *ProbeResult) Duration() int64 {
	if r.Format.Duration == "" {
		return 0
	}
	if dur, err := strconv.ParseFloat(r.Format.Duration, 64); err == nil {
		return int64(dur * 1000)
	}
	return 0
}

// Bitrate returns the overall bitrate in bits per second.
func (r *ProbeResult) Bitrate() int {
	if r.Format.BitRate == "" {
		return 0
	}
	if br, err := strconv.Atoi(r.Format.BitRate); err == nil {
		return br
	}
	return 0
}

// Framerate returns the framerate for a video stream.
func (s *ProbeStream) Framerate() float64 {
	if s.AvgFrameRate != "" {
		return parseFramerate(s.AvgFrameRate)
	}
	if s.RFrameRate != "" {
		return parseFramerate(s.RFrameRate)
	}
	return 0
}

// IsAudioOnly returns true if the stream has audio tracks but no video tracks.
// This is typical for radio streams.
func (info *StreamInfo) IsAudioOnly() bool {
	return len(info.AudioTracks) > 0 && len(info.VideoTracks) == 0
}

// IsVideoOnly returns true if the stream has video tracks but no audio tracks.
func (info *StreamInfo) IsVideoOnly() bool {
	return len(info.VideoTracks) > 0 && len(info.AudioTracks) == 0
}

// HasVideo returns true if the stream has at least one video track.
func (info *StreamInfo) HasVideo() bool {
	return len(info.VideoTracks) > 0
}

// HasAudio returns true if the stream has at least one audio track.
func (info *StreamInfo) HasAudio() bool {
	return len(info.AudioTracks) > 0
}

// GetVideoTrackByIndex returns the video track with the specified stream index.
// Returns nil if not found.
func (info *StreamInfo) GetVideoTrackByIndex(streamIndex int) *VideoTrackInfo {
	for i := range info.VideoTracks {
		if info.VideoTracks[i].Index == streamIndex {
			return &info.VideoTracks[i]
		}
	}
	return nil
}

// GetAudioTrackByIndex returns the audio track with the specified stream index.
// Returns nil if not found.
func (info *StreamInfo) GetAudioTrackByIndex(streamIndex int) *AudioTrackInfo {
	for i := range info.AudioTracks {
		if info.AudioTracks[i].Index == streamIndex {
			return &info.AudioTracks[i]
		}
	}
	return nil
}

// GetBestAudioTrack returns the audio track that best matches the given criteria.
// Priority: language match + highest channel count + highest bitrate
func (info *StreamInfo) GetBestAudioTrack(preferredLanguage string, preferHigherChannels bool) *AudioTrackInfo {
	if len(info.AudioTracks) == 0 {
		return nil
	}

	var best *AudioTrackInfo
	for i := range info.AudioTracks {
		track := &info.AudioTracks[i]

		if best == nil {
			best = track
			continue
		}

		// If we have a language preference, prioritize matching tracks
		if preferredLanguage != "" {
			trackMatches := track.Language == preferredLanguage
			bestMatches := best.Language == preferredLanguage
			if trackMatches && !bestMatches {
				best = track
				continue
			}
			if !trackMatches && bestMatches {
				continue
			}
		}

		// If both match (or don't match) language, compare by channels or bitrate
		if preferHigherChannels {
			if track.Channels > best.Channels {
				best = track
				continue
			}
			if track.Channels == best.Channels && track.Bitrate > best.Bitrate {
				best = track
			}
		} else {
			// Prefer higher bitrate
			if track.Bitrate > best.Bitrate {
				best = track
			}
		}
	}

	return best
}
