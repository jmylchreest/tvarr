// Package daemon provides the tvarr-ffmpegd daemon implementation.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
)

// TSDemuxerConfig configures the TS demuxer for the daemon.
type TSDemuxerConfig struct {
	Logger *slog.Logger

	// Expected output codecs (from FFmpeg transcode configuration)
	TargetVideoCodec string // "h264", "h265"
	TargetAudioCodec string // "aac", "ac3", "eac3", "mp3", "opus"

	// Callbacks for demuxed samples.
	OnVideoSample func(pts, dts int64, data []byte, isKeyframe bool)
	OnAudioSample func(pts int64, data []byte)
}

// TSDemuxer demuxes MPEG-TS streams from FFmpeg output to elementary streams.
type TSDemuxer struct {
	config TSDemuxerConfig

	// mediacommon reader
	reader *mpegts.Reader

	// Track references
	videoTrack *mpegts.Track
	audioTrack *mpegts.Track

	// Detected codecs
	videoCodec string
	audioCodec string

	// Audio frame duration for PTS calculation (in 90kHz ticks)
	audioFrameDuration int64
	audioSampleRate    int

	// Buffer for incremental writes
	pipeMu     sync.Mutex
	pipeReader *io.PipeReader
	pipeWriter *io.PipeWriter

	// State tracking
	initialized bool
	initOnce    sync.Once
	initErr     error
	initDone    chan struct{}

	// Debug logging flags
	firstH265KeyframeLogged bool

	// Context for cleanup
	ctx    context.Context
	cancel context.CancelFunc
}

// NewTSDemuxer creates a new MPEG-TS demuxer for daemon use.
func NewTSDemuxer(config TSDemuxerConfig) *TSDemuxer {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()

	d := &TSDemuxer{
		config:     config,
		pipeReader: pr,
		pipeWriter: pw,
		initDone:   make(chan struct{}),
		ctx:        ctx,
		cancel:     cancel,
	}

	// Start the reader goroutine
	go d.runReader()

	return d
}

// runReader runs the mediacommon reader in a goroutine.
func (d *TSDemuxer) runReader() {
	defer func() {
		d.pipeReader.Close()
		close(d.initDone)
	}()

	// Create the mediacommon reader
	d.reader = &mpegts.Reader{R: d.pipeReader}

	// Initialize - this reads until it finds PAT/PMT
	if err := d.reader.Initialize(); err != nil {
		d.initOnce.Do(func() {
			d.initErr = fmt.Errorf("initializing mpegts reader: %w", err)
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
				d.config.Logger.Info("MPEG-TS demuxer initialization failed",
					slog.String("error", err.Error()))
			}
		})
		return
	}

	// Process discovered tracks
	for _, track := range d.reader.Tracks() {
		d.setupTrackCallback(track)
	}

	d.initOnce.Do(func() {
		d.initialized = true
		d.config.Logger.Debug("MPEG-TS demuxer ready",
			slog.String("video_codec", d.videoCodec),
			slog.String("audio_codec", d.audioCodec),
			slog.Int("audio_sample_rate", d.audioSampleRate))
	})

	// Set up decode error callback
	d.reader.OnDecodeError(func(err error) {
		d.config.Logger.Debug("MPEG-TS decode error",
			slog.String("error", err.Error()))
	})

	// Read loop
	for {
		select {
		case <-d.ctx.Done():
			d.config.Logger.Debug("MPEG-TS demuxer context cancelled")
			return
		default:
			if err := d.reader.Read(); err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
					d.config.Logger.Debug("MPEG-TS demuxer stream ended",
						slog.String("reason", err.Error()))
					return
				}
				d.config.Logger.Info("MPEG-TS demuxer read error (exiting)",
					slog.String("video_codec", d.videoCodec),
					slog.String("audio_codec", d.audioCodec),
					slog.String("error", err.Error()))
				return
			}
		}
	}
}

// setupTrackCallback configures callbacks for a discovered track.
func (d *TSDemuxer) setupTrackCallback(track *mpegts.Track) {
	switch codec := track.Codec.(type) {
	case *mpegts.CodecH264:
		d.videoTrack = track
		d.videoCodec = "h264"
		d.reader.OnDataH264(track, func(pts, dts int64, au [][]byte) error {
			return d.handleH264(pts, dts, au)
		})
		d.config.Logger.Debug("Found video track",
			slog.String("codec", "h264"),
			slog.Uint64("pid", uint64(track.PID)))

	case *mpegts.CodecH265:
		d.videoTrack = track
		d.videoCodec = "h265"
		d.reader.OnDataH265(track, func(pts, dts int64, au [][]byte) error {
			return d.handleH265(pts, dts, au)
		})
		d.config.Logger.Debug("Found video track",
			slog.String("codec", "h265"),
			slog.Uint64("pid", uint64(track.PID)))

	case *mpegts.CodecMPEG4Audio:
		d.audioTrack = track
		d.audioCodec = "aac"
		d.audioSampleRate = codec.Config.SampleRate
		if d.audioSampleRate <= 0 {
			d.audioSampleRate = 48000
		}
		d.audioFrameDuration = int64(1024 * 90000 / d.audioSampleRate)
		d.reader.OnDataMPEG4Audio(track, func(pts int64, aus [][]byte) error {
			return d.handleMPEG4Audio(pts, aus)
		})
		d.config.Logger.Debug("Found audio track",
			slog.String("codec", "aac"),
			slog.Uint64("pid", uint64(track.PID)),
			slog.Int("sample_rate", codec.Config.SampleRate),
			slog.Int("channels", codec.Config.ChannelCount))

	case *mpegts.CodecAC3:
		d.audioTrack = track
		d.audioCodec = "ac3"
		d.reader.OnDataAC3(track, func(pts int64, frame []byte) error {
			return d.handleAC3(pts, frame)
		})
		d.config.Logger.Debug("Found audio track",
			slog.String("codec", "ac3"),
			slog.Uint64("pid", uint64(track.PID)),
			slog.Int("sample_rate", codec.SampleRate),
			slog.Int("channels", codec.ChannelCount))

	case *mpegts.CodecEAC3:
		d.audioTrack = track
		d.audioCodec = "eac3"
		d.audioSampleRate = codec.SampleRate
		if d.audioSampleRate <= 0 {
			d.audioSampleRate = 48000
		}
		d.audioFrameDuration = int64(1536 * 90000 / d.audioSampleRate)
		d.reader.OnDataEAC3(track, func(pts int64, frame []byte) error {
			return d.handleEAC3(pts, frame)
		})
		d.config.Logger.Debug("Found audio track",
			slog.String("codec", "eac3"),
			slog.Uint64("pid", uint64(track.PID)),
			slog.Int("sample_rate", codec.SampleRate),
			slog.Int("channels", codec.ChannelCount))

	case *mpegts.CodecMPEG1Audio:
		d.audioTrack = track
		d.audioCodec = "mp3"
		d.audioSampleRate = 48000
		d.audioFrameDuration = int64(1152 * 90000 / d.audioSampleRate)
		d.reader.OnDataMPEG1Audio(track, func(pts int64, frames [][]byte) error {
			return d.handleMPEG1Audio(pts, frames)
		})
		d.config.Logger.Debug("Found audio track",
			slog.String("codec", "mp3"),
			slog.Uint64("pid", uint64(track.PID)))

	case *mpegts.CodecOpus:
		d.audioTrack = track
		d.audioCodec = "opus"
		d.audioSampleRate = 48000
		d.audioFrameDuration = int64(960 * 90000 / d.audioSampleRate)
		d.reader.OnDataOpus(track, func(pts int64, packets [][]byte) error {
			return d.handleOpus(pts, packets)
		})
		d.config.Logger.Debug("Found audio track",
			slog.String("codec", "opus"),
			slog.Uint64("pid", uint64(track.PID)),
			slog.Int("channels", codec.ChannelCount))

	default:
		d.config.Logger.Debug("Found unsupported track",
			slog.Uint64("pid", uint64(track.PID)),
			slog.String("type", fmt.Sprintf("%T", track.Codec)))
	}
}

// handleH264 processes H.264 access units.
func (d *TSDemuxer) handleH264(pts, dts int64, au [][]byte) error {
	if len(au) == 0 {
		return nil
	}

	isKeyframe := h264.IsRandomAccess(au)

	annexB, err := h264.AnnexB(au).Marshal()
	if err != nil || len(annexB) == 0 {
		return nil
	}

	d.emitVideoSample(pts, dts, annexB, isKeyframe)
	return nil
}

// handleH265 processes H.265 access units.
func (d *TSDemuxer) handleH265(pts, dts int64, au [][]byte) error {
	if len(au) == 0 {
		return nil
	}

	isKeyframe := h265.IsRandomAccess(au)

	// Log first keyframe's NAL types for debugging H.265 header issues
	if isKeyframe && !d.firstH265KeyframeLogged {
		d.firstH265KeyframeLogged = true
		nalTypes := make([]int, 0, len(au))
		hasVPS, hasSPS, hasPPS := false, false, false
		for _, nalu := range au {
			if len(nalu) > 0 {
				naluType := int((nalu[0] >> 1) & 0x3F)
				nalTypes = append(nalTypes, naluType)
				if naluType == 32 {
					hasVPS = true
				} else if naluType == 33 {
					hasSPS = true
				} else if naluType == 34 {
					hasPPS = true
				}
			}
		}
		d.config.Logger.Info("First H.265 keyframe NAL analysis from FFmpeg",
			slog.Any("nal_types", nalTypes),
			slog.Bool("has_vps", hasVPS),
			slog.Bool("has_sps", hasSPS),
			slog.Bool("has_pps", hasPPS),
			slog.Int("nal_count", len(au)))
	}

	// H.265 also uses Annex B format
	annexB, err := h264.AnnexB(au).Marshal()
	if err != nil || len(annexB) == 0 {
		return nil
	}

	d.emitVideoSample(pts, dts, annexB, isKeyframe)
	return nil
}

// handleMPEG4Audio processes AAC audio units.
func (d *TSDemuxer) handleMPEG4Audio(pts int64, aus [][]byte) error {
	currentPTS := pts
	frameDuration := d.audioFrameDuration
	if frameDuration <= 0 {
		frameDuration = 1920 // Fallback: AAC 1024 samples @ 48kHz
	}

	for _, au := range aus {
		if len(au) == 0 {
			continue
		}
		d.emitAudioSample(currentPTS, au)
		currentPTS += frameDuration
	}
	return nil
}

// handleAC3 processes AC-3 frames.
func (d *TSDemuxer) handleAC3(pts int64, frame []byte) error {
	if len(frame) == 0 {
		return nil
	}
	d.emitAudioSample(pts, frame)
	return nil
}

// handleEAC3 processes E-AC-3 frames.
func (d *TSDemuxer) handleEAC3(pts int64, frame []byte) error {
	if len(frame) == 0 {
		return nil
	}
	d.emitAudioSample(pts, frame)
	return nil
}

// handleMPEG1Audio processes MPEG-1 audio frames.
func (d *TSDemuxer) handleMPEG1Audio(pts int64, frames [][]byte) error {
	currentPTS := pts
	frameDuration := d.audioFrameDuration
	if frameDuration <= 0 {
		frameDuration = 2160 // Fallback: MP3 1152 samples @ 48kHz
	}

	for _, frame := range frames {
		if len(frame) == 0 {
			continue
		}
		d.emitAudioSample(currentPTS, frame)
		currentPTS += frameDuration
	}
	return nil
}

// handleOpus processes Opus packets.
func (d *TSDemuxer) handleOpus(pts int64, packets [][]byte) error {
	currentPTS := pts
	frameDuration := d.audioFrameDuration
	if frameDuration <= 0 {
		frameDuration = 1800 // Fallback: Opus 960 samples @ 48kHz
	}

	for _, packet := range packets {
		if len(packet) == 0 {
			continue
		}
		d.emitAudioSample(currentPTS, packet)
		currentPTS += frameDuration
	}
	return nil
}

// emitVideoSample invokes the video sample callback.
func (d *TSDemuxer) emitVideoSample(pts, dts int64, data []byte, isKeyframe bool) {
	if d.config.OnVideoSample != nil {
		d.config.OnVideoSample(pts, dts, data, isKeyframe)
	}
}

// emitAudioSample invokes the audio sample callback.
func (d *TSDemuxer) emitAudioSample(pts int64, data []byte) {
	if d.config.OnAudioSample != nil {
		d.config.OnAudioSample(pts, data)
	}
}

// Write processes MPEG-TS data.
func (d *TSDemuxer) Write(data []byte) error {
	d.pipeMu.Lock()
	defer d.pipeMu.Unlock()

	_, err := d.pipeWriter.Write(data)
	if err != nil {
		return fmt.Errorf("writing to demuxer pipe: %w", err)
	}

	return nil
}

// Flush signals end of data and waits for processing to complete.
func (d *TSDemuxer) Flush() {
	d.pipeMu.Lock()
	d.pipeWriter.Close()
	d.pipeMu.Unlock()

	// Wait for reader to finish
	<-d.initDone
}

// Close stops the demuxer.
func (d *TSDemuxer) Close() {
	d.cancel()
	d.pipeWriter.Close()
}

// VideoCodec returns the detected video codec.
func (d *TSDemuxer) VideoCodec() string {
	return d.videoCodec
}

// AudioCodec returns the detected audio codec.
func (d *TSDemuxer) AudioCodec() string {
	return d.audioCodec
}

// WaitInitialized waits for the demuxer to initialize and returns any error.
func (d *TSDemuxer) WaitInitialized(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-d.initDone:
		return d.initErr
	}
}

// IsInitialized returns whether the demuxer has successfully initialized.
func (d *TSDemuxer) IsInitialized() bool {
	select {
	case <-d.initDone:
		return d.initialized
	default:
		return false
	}
}
