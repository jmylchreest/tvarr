package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/jmylchreest/tvarr/internal/daemon"
	"github.com/spf13/cobra"
)

// detectCmd represents the detect command
var detectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Detect FFmpeg and hardware capabilities",
	Long: `Detect FFmpeg installation and hardware acceleration capabilities.

This command runs capability detection and outputs the results as JSON.
Use this to verify what codecs, encoders, and hardware acceleration
are available on this system.

Examples:
  # Basic detection (JSON output)
  tvarr-ffmpegd detect

  # Pretty-printed JSON
  tvarr-ffmpegd detect --pretty

  # Output to file
  tvarr-ffmpegd detect > capabilities.json`,
	RunE: runDetect,
}

func init() {
	rootCmd.AddCommand(detectCmd)

	detectCmd.Flags().Bool("pretty", false, "pretty-print JSON output")
	detectCmd.Flags().Duration("timeout", 30*time.Second, "detection timeout")
}

// DetectionResult contains the full detection output.
type DetectionResult struct {
	FFmpeg       FFmpegInfo       `json:"ffmpeg"`
	Capabilities CapabilitiesInfo `json:"capabilities"`
}

// FFmpegInfo contains FFmpeg binary information.
type FFmpegInfo struct {
	Version     string `json:"version"`
	FFmpegPath  string `json:"ffmpeg_path"`
	FFprobePath string `json:"ffprobe_path"`
}

// CapabilitiesInfo contains detected capabilities.
type CapabilitiesInfo struct {
	VideoEncoders     []string       `json:"video_encoders"`
	VideoDecoders     []string       `json:"video_decoders"`
	AudioEncoders     []string       `json:"audio_encoders"`
	AudioDecoders     []string       `json:"audio_decoders"`
	HardwareAccels    []HWAccelInfo  `json:"hardware_accels"`
	GPUs              []GPUInfo      `json:"gpus"`
	MaxConcurrentJobs int            `json:"max_concurrent_jobs"`
}

// FilteredEncoderInfo describes an encoder that was filtered out.
type FilteredEncoderInfo struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// HWAccelInfo contains hardware accelerator information.
type HWAccelInfo struct {
	Type             string                `json:"type"`
	Device           string                `json:"device,omitempty"`
	Available        bool                  `json:"available"`
	Encoders         []string              `json:"hw_encoders,omitempty"`
	Decoders         []string              `json:"hw_decoders,omitempty"`
	FilteredEncoders []FilteredEncoderInfo `json:"filtered_encoders,omitempty"`
}

// GPUInfo contains GPU information.
type GPUInfo struct {
	Index             int    `json:"index"`
	Name              string `json:"name"`
	Class             string `json:"class"`
	MaxEncodeSessions int    `json:"max_encode_sessions"`
	MaxDecodeSessions int    `json:"max_decode_sessions,omitempty"`
}

func runDetect(cmd *cobra.Command, _ []string) error {
	timeout, _ := cmd.Flags().GetDuration("timeout")
	pretty, _ := cmd.Flags().GetBool("pretty")

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Run capability detection
	detector := daemon.NewCapabilityDetector()
	caps, binInfo, err := detector.Detect(ctx)
	if err != nil {
		return fmt.Errorf("detection failed: %w", err)
	}

	// Build result
	result := DetectionResult{
		FFmpeg: FFmpegInfo{
			Version:     binInfo.Version,
			FFmpegPath:  binInfo.FFmpegPath,
			FFprobePath: binInfo.FFprobePath,
		},
		Capabilities: CapabilitiesInfo{
			VideoEncoders:     caps.VideoEncoders,
			VideoDecoders:     caps.VideoDecoders,
			AudioEncoders:     caps.AudioEncoders,
			AudioDecoders:     caps.AudioDecoders,
			MaxConcurrentJobs: int(caps.MaxConcurrentJobs),
		},
	}

	// Convert hardware accels
	for _, hw := range caps.HwAccels {
		// Convert filtered encoders
		var filteredEncoders []FilteredEncoderInfo
		for _, fe := range hw.FilteredEncoders {
			filteredEncoders = append(filteredEncoders, FilteredEncoderInfo{
				Name:   fe.Name,
				Reason: fe.Reason,
			})
		}

		result.Capabilities.HardwareAccels = append(result.Capabilities.HardwareAccels, HWAccelInfo{
			Type:             hw.Type,
			Device:           hw.Device,
			Available:        hw.Available,
			Encoders:         hw.HwEncoders,
			Decoders:         hw.HwDecoders,
			FilteredEncoders: filteredEncoders,
		})
	}

	// Convert GPUs
	for _, gpu := range caps.Gpus {
		result.Capabilities.GPUs = append(result.Capabilities.GPUs, GPUInfo{
			Index:             int(gpu.Index),
			Name:              gpu.Name,
			Class:             gpu.GpuClass.String(),
			MaxEncodeSessions: int(gpu.MaxEncodeSessions),
			MaxDecodeSessions: int(gpu.MaxDecodeSessions),
		})
	}

	// Output JSON
	var output []byte
	if pretty {
		output, err = json.MarshalIndent(result, "", "  ")
	} else {
		output, err = json.Marshal(result)
	}
	if err != nil {
		return fmt.Errorf("marshaling result: %w", err)
	}

	fmt.Fprintln(os.Stdout, string(output))
	return nil
}
