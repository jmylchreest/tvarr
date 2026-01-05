// Package ffmpeg provides FFmpeg/FFprobe binary detection and wrapper functionality.
package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jmylchreest/tvarr/internal/util"
)

// BinaryInfo contains information about the FFmpeg/FFprobe installation.
type BinaryInfo struct {
	FFmpegPath    string        `json:"ffmpeg_path"`
	FFprobePath   string        `json:"ffprobe_path"`
	Version       string        `json:"version"`
	MajorVersion  int           `json:"major_version"`
	MinorVersion  int           `json:"minor_version"`
	BuildDate     string        `json:"build_date,omitempty"`
	Configuration string        `json:"configuration,omitempty"`
	Codecs        []Codec       `json:"codecs,omitempty"`
	Encoders      []string      `json:"encoders,omitempty"`
	Decoders      []string      `json:"decoders,omitempty"`
	HWAccels      []HWAccelInfo `json:"hw_accels,omitempty"`
	Formats       []FormatInfo  `json:"formats,omitempty"`
}

// Codec represents codec information from FFmpeg.
type Codec struct {
	Name        string `json:"name"`
	LongName    string `json:"long_name,omitempty"`
	Type        string `json:"type"` // video, audio, subtitle, data
	CanDecode   bool   `json:"can_decode"`
	CanEncode   bool   `json:"can_encode"`
	IsLossy     bool   `json:"is_lossy,omitempty"`
	IsLossless  bool   `json:"is_lossless,omitempty"`
	IsIntraOnly bool   `json:"is_intra_only,omitempty"`
}

// FormatInfo represents format/container information from FFmpeg.
type FormatInfo struct {
	Name     string `json:"name"`
	LongName string `json:"long_name,omitempty"`
	CanMux   bool   `json:"can_mux"`
	CanDemux bool   `json:"can_demux"`
}

// BinaryDetector handles detection and caching of FFmpeg binaries.
type BinaryDetector struct {
	mu           sync.RWMutex
	info         *BinaryInfo
	lastDetected time.Time
	cacheTTL     time.Duration
}

// NewBinaryDetector creates a new binary detector.
func NewBinaryDetector() *BinaryDetector {
	return &BinaryDetector{
		cacheTTL: 5 * time.Minute,
	}
}

// WithCacheTTL sets the cache TTL for binary detection.
func (d *BinaryDetector) WithCacheTTL(ttl time.Duration) *BinaryDetector {
	d.cacheTTL = ttl
	return d
}

// Detect detects FFmpeg and FFprobe binaries and their capabilities.
func (d *BinaryDetector) Detect(ctx context.Context) (*BinaryInfo, error) {
	d.mu.RLock()
	if d.info != nil && time.Since(d.lastDetected) < d.cacheTTL {
		info := d.info
		d.mu.RUnlock()
		return info, nil
	}
	d.mu.RUnlock()

	d.mu.Lock()
	defer d.mu.Unlock()

	// Double-check after acquiring write lock
	if d.info != nil && time.Since(d.lastDetected) < d.cacheTTL {
		return d.info, nil
	}

	info, err := d.detect(ctx)
	if err != nil {
		return nil, err
	}

	d.info = info
	d.lastDetected = time.Now()
	return info, nil
}

// Clear clears the cached binary information.
func (d *BinaryDetector) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.info = nil
}

// detect performs the actual binary detection.
func (d *BinaryDetector) detect(ctx context.Context) (*BinaryInfo, error) {
	info := &BinaryInfo{}

	// Find ffmpeg binary (required)
	// Search order: TVARR_FFMPEG_BINARY env var -> ./ffmpeg -> PATH
	ffmpegPath, err := util.FindBinary("ffmpeg", "TVARR_FFMPEG_BINARY")
	if err != nil {
		return nil, fmt.Errorf("ffmpeg not found: %w", err)
	}
	info.FFmpegPath = ffmpegPath

	// Find ffprobe binary (optional - used for codec pre-detection)
	// Search order: TVARR_FFPROBE_BINARY env var -> ./ffprobe -> PATH
	// If not found, relay will still work but without codec caching optimization
	ffprobePath, err := util.FindBinary("ffprobe", "TVARR_FFPROBE_BINARY")
	if err == nil {
		info.FFprobePath = ffprobePath
	}
	// ffprobePath will be empty if not found - this is handled gracefully downstream

	// Get version information
	version, err := d.getVersion(ctx, ffmpegPath)
	if err != nil {
		return nil, fmt.Errorf("getting ffmpeg version: %w", err)
	}
	info.Version = version.Full
	info.MajorVersion = version.Major
	info.MinorVersion = version.Minor
	info.BuildDate = version.BuildDate
	info.Configuration = version.Configuration

	// Get codecs
	codecs, err := d.getCodecs(ctx, ffmpegPath)
	if err == nil {
		info.Codecs = codecs
	}

	// Get encoders
	encoders, err := d.getEncoders(ctx, ffmpegPath)
	if err == nil {
		info.Encoders = encoders
	}

	// Get decoders
	decoders, err := d.getDecoders(ctx, ffmpegPath)
	if err == nil {
		info.Decoders = decoders
	}

	// Get hardware accelerators
	hwAccels, err := d.getHWAccels(ctx, ffmpegPath)
	if err == nil {
		info.HWAccels = hwAccels
	}

	// Get formats
	formats, err := d.getFormats(ctx, ffmpegPath)
	if err == nil {
		info.Formats = formats
	}

	return info, nil
}

// versionInfo holds parsed version information.
type versionInfo struct {
	Full          string
	Major         int
	Minor         int
	BuildDate     string
	Configuration string
}

// getVersion extracts version information from ffmpeg.
func (d *BinaryDetector) getVersion(ctx context.Context, ffmpegPath string) (*versionInfo, error) {
	cmd := exec.CommandContext(ctx, ffmpegPath, "-version")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(output), "\n")
	info := &versionInfo{}

	for _, line := range lines {
		if strings.HasPrefix(line, "ffmpeg version") {
			// Parse version string like "ffmpeg version 6.0 Copyright..."
			// or "ffmpeg version n6.0-2-g..." or "ffmpeg version 6.0.1"
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				info.Full = parts[2]
				// Extract numeric version
				versionRegex := regexp.MustCompile(`^n?(\d+)\.(\d+)`)
				matches := versionRegex.FindStringSubmatch(parts[2])
				if len(matches) >= 3 {
					info.Major, _ = strconv.Atoi(matches[1])
					info.Minor, _ = strconv.Atoi(matches[2])
				}
			}
		} else if strings.HasPrefix(line, "built with") {
			info.BuildDate = strings.TrimPrefix(line, "built with ")
		} else if strings.HasPrefix(line, "configuration:") {
			info.Configuration = strings.TrimPrefix(line, "configuration: ")
		}
	}

	if info.Full == "" {
		return nil, fmt.Errorf("failed to parse ffmpeg version")
	}

	return info, nil
}

// getCodecs retrieves available codecs.
func (d *BinaryDetector) getCodecs(ctx context.Context, ffmpegPath string) ([]Codec, error) {
	cmd := exec.CommandContext(ctx, ffmpegPath, "-codecs", "-hide_banner")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var codecs []Codec
	lines := strings.Split(string(output), "\n")
	inCodecList := false

	for _, line := range lines {
		if strings.Contains(line, "-------") {
			inCodecList = true
			continue
		}
		if !inCodecList || len(line) < 8 {
			continue
		}

		// Format: DEV.LS codec_name description
		// Position 0: D = Decoding supported
		// Position 1: E = Encoding supported
		// Position 2: V = Video, A = Audio, S = Subtitle, D = Data, T = Attachment
		// Position 3: I = Intra frame-only
		// Position 4: L = Lossy compression
		// Position 5: S = Lossless compression
		line = strings.TrimLeft(line, " ")
		if len(line) < 8 {
			continue
		}

		flags := line[:6]
		rest := strings.TrimSpace(line[6:])
		parts := strings.SplitN(rest, " ", 2)
		if len(parts) < 1 || parts[0] == "" {
			continue
		}

		codec := Codec{
			Name:        parts[0],
			CanDecode:   flags[0] == 'D',
			CanEncode:   flags[1] == 'E',
			IsIntraOnly: flags[3] == 'I',
			IsLossy:     flags[4] == 'L',
			IsLossless:  flags[5] == 'S',
		}

		switch flags[2] {
		case 'V':
			codec.Type = "video"
		case 'A':
			codec.Type = "audio"
		case 'S':
			codec.Type = "subtitle"
		case 'D':
			codec.Type = "data"
		case 'T':
			codec.Type = "attachment"
		}

		if len(parts) > 1 {
			codec.LongName = strings.TrimSpace(parts[1])
		}

		if codec.Name != "" && codec.Type != "" {
			codecs = append(codecs, codec)
		}
	}

	return codecs, nil
}

// getEncoders retrieves available encoders.
func (d *BinaryDetector) getEncoders(ctx context.Context, ffmpegPath string) ([]string, error) {
	cmd := exec.CommandContext(ctx, ffmpegPath, "-encoders", "-hide_banner")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var encoders []string
	lines := strings.Split(string(output), "\n")
	inEncoderList := false

	for _, line := range lines {
		if strings.Contains(line, "------") {
			inEncoderList = true
			continue
		}
		if !inEncoderList {
			continue
		}

		// Format: V....D encoder_name description
		line = strings.TrimLeft(line, " ")
		if len(line) < 8 {
			continue
		}

		// Skip if it's not a codec line (starts with V/A/S)
		if line[0] != 'V' && line[0] != 'A' && line[0] != 'S' {
			continue
		}

		rest := strings.TrimSpace(line[6:])
		parts := strings.Fields(rest)
		if len(parts) >= 1 && parts[0] != "" {
			encoders = append(encoders, parts[0])
		}
	}

	return encoders, nil
}

// getDecoders retrieves available decoders.
func (d *BinaryDetector) getDecoders(ctx context.Context, ffmpegPath string) ([]string, error) {
	cmd := exec.CommandContext(ctx, ffmpegPath, "-decoders", "-hide_banner")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var decoders []string
	lines := strings.Split(string(output), "\n")
	inDecoderList := false

	for _, line := range lines {
		if strings.Contains(line, "------") {
			inDecoderList = true
			continue
		}
		if !inDecoderList {
			continue
		}

		// Format: V....D decoder_name description
		line = strings.TrimLeft(line, " ")
		if len(line) < 8 {
			continue
		}

		// Skip if it's not a codec line (starts with V/A/S)
		if line[0] != 'V' && line[0] != 'A' && line[0] != 'S' {
			continue
		}

		rest := strings.TrimSpace(line[6:])
		parts := strings.Fields(rest)
		if len(parts) >= 1 && parts[0] != "" {
			decoders = append(decoders, parts[0])
		}
	}

	return decoders, nil
}

// getFormats retrieves available formats.
func (d *BinaryDetector) getFormats(ctx context.Context, ffmpegPath string) ([]FormatInfo, error) {
	cmd := exec.CommandContext(ctx, ffmpegPath, "-formats", "-hide_banner")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var formats []FormatInfo
	lines := strings.Split(string(output), "\n")
	inFormatList := false

	for _, line := range lines {
		if strings.Contains(line, "--") {
			inFormatList = true
			continue
		}
		if !inFormatList || len(line) < 4 {
			continue
		}

		flags := strings.TrimSpace(line[:3])
		rest := strings.TrimSpace(line[3:])
		parts := strings.SplitN(rest, " ", 2)
		if len(parts) < 1 {
			continue
		}

		format := FormatInfo{
			Name:     parts[0],
			CanDemux: strings.Contains(flags, "D"),
			CanMux:   strings.Contains(flags, "E"),
		}

		if len(parts) > 1 {
			format.LongName = strings.TrimSpace(parts[1])
		}

		if format.Name != "" {
			formats = append(formats, format)
		}
	}

	return formats, nil
}

// HasEncoder returns true if the encoder is available.
func (info *BinaryInfo) HasEncoder(name string) bool {
	return slices.Contains(info.Encoders, name)
}

// HasDecoder returns true if the decoder is available.
func (info *BinaryInfo) HasDecoder(name string) bool {
	return slices.Contains(info.Decoders, name)
}

// HasFormat returns true if the format is available for muxing.
func (info *BinaryInfo) HasFormat(name string) bool {
	for _, fmt := range info.Formats {
		if fmt.Name == name && fmt.CanMux {
			return true
		}
	}
	return false
}

// JSON returns the binary info as JSON string.
func (info *BinaryInfo) JSON() string {
	data, _ := json.MarshalIndent(info, "", "  ")
	return string(data)
}

// SupportsMinVersion returns true if FFmpeg version meets minimum requirement.
func (info *BinaryInfo) SupportsMinVersion(major, minor int) bool {
	if info.MajorVersion > major {
		return true
	}
	if info.MajorVersion == major && info.MinorVersion >= minor {
		return true
	}
	return false
}
