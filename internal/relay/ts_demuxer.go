// Package relay provides streaming relay functionality for tvarr.
package relay

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

// TSDemuxerConfig configures the MPEG-TS demuxer.
type TSDemuxerConfig struct {
	// Logger for structured logging.
	Logger *slog.Logger

	// TargetVariant specifies which variant to write to in the SharedESBuffer.
	// If empty, writes to the source variant.
	TargetVariant CodecVariant

	// Callbacks for demuxed samples.
	OnVideoSample func(pts, dts int64, data []byte, isKeyframe bool)
	OnAudioSample func(pts int64, data []byte)
}

// TSDemuxer demuxes MPEG-TS streams to elementary streams using mediacommon.
type TSDemuxer struct {
	config TSDemuxerConfig
	buffer *SharedESBuffer

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

	// Context for cleanup
	ctx    context.Context
	cancel context.CancelFunc
}

// NewTSDemuxer creates a new MPEG-TS demuxer backed by mediacommon.
func NewTSDemuxer(buffer *SharedESBuffer, config TSDemuxerConfig) *TSDemuxer {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()

	d := &TSDemuxer{
		config:     config,
		buffer:     buffer,
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
		})
		return
	}

	// Process discovered tracks
	for _, track := range d.reader.Tracks() {
		d.setupTrackCallback(track)
	}

	d.initOnce.Do(func() {
		d.initialized = true
		d.config.Logger.Debug("MPEG-TS demuxer initialized",
			slog.String("video_codec", d.videoCodec),
			slog.String("audio_codec", d.audioCodec))
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
			return
		default:
			if err := d.reader.Read(); err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
					return
				}
				d.config.Logger.Debug("MPEG-TS read error",
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
		// Only set source codec when NOT writing to a target variant
		// Target variant already has its codec set from the variant name
		if d.buffer != nil && d.config.TargetVariant == "" {
			d.buffer.SetVideoCodec("h264", nil)
		}
		d.reader.OnDataH264(track, func(pts, dts int64, au [][]byte) error {
			return d.handleH264(pts, dts, au)
		})
		d.config.Logger.Debug("Found H.264 video track",
			slog.Uint64("pid", uint64(track.PID)))

	case *mpegts.CodecH265:
		d.videoTrack = track
		d.videoCodec = "h265"
		// Only set source codec when NOT writing to a target variant
		// Target variant already has its codec set from the variant name
		if d.buffer != nil && d.config.TargetVariant == "" {
			d.buffer.SetVideoCodec("h265", nil)
		}
		d.reader.OnDataH265(track, func(pts, dts int64, au [][]byte) error {
			return d.handleH265(pts, dts, au)
		})
		d.config.Logger.Debug("Found H.265 video track",
			slog.Uint64("pid", uint64(track.PID)))

	case *mpegts.CodecMPEG4Audio:
		d.audioTrack = track
		d.audioCodec = "aac"
		// Store sample rate and calculate frame duration
		d.audioSampleRate = codec.Config.SampleRate
		if d.audioSampleRate <= 0 {
			d.audioSampleRate = 48000 // Default to 48kHz
		}
		// AAC frames are typically 1024 samples per frame
		// Frame duration in 90kHz ticks = 1024 * 90000 / sampleRate
		d.audioFrameDuration = int64(1024 * 90000 / d.audioSampleRate)
		// Only set source codec when NOT writing to a target variant
		if d.buffer != nil && d.config.TargetVariant == "" {
			d.buffer.SetAudioCodec("aac", nil)
		}
		d.reader.OnDataMPEG4Audio(track, func(pts int64, aus [][]byte) error {
			return d.handleMPEG4Audio(pts, aus)
		})
		d.config.Logger.Debug("Found AAC audio track",
			slog.Uint64("pid", uint64(track.PID)),
			slog.Int("sample_rate", codec.Config.SampleRate),
			slog.Int("channels", codec.Config.ChannelCount),
			slog.Int64("frame_duration_ticks", d.audioFrameDuration))

	case *mpegts.CodecAC3:
		d.audioTrack = track
		d.audioCodec = "ac3"
		// Only set source codec when NOT writing to a target variant
		if d.buffer != nil && d.config.TargetVariant == "" {
			d.buffer.SetAudioCodec("ac3", nil)
		}
		d.reader.OnDataAC3(track, func(pts int64, frame []byte) error {
			return d.handleAC3(pts, frame)
		})
		d.config.Logger.Debug("Found AC-3 audio track",
			slog.Uint64("pid", uint64(track.PID)),
			slog.Int("sample_rate", codec.SampleRate),
			slog.Int("channels", codec.ChannelCount))

	case *mpegts.CodecMPEG1Audio:
		d.audioTrack = track
		d.audioCodec = "mp3"
		// MP3 frames are typically 1152 samples at 44.1kHz or 48kHz
		// Default to 48kHz, frame duration = 1152 * 90000 / 48000 = 2160 ticks
		d.audioSampleRate = 48000
		d.audioFrameDuration = int64(1152 * 90000 / d.audioSampleRate)
		if d.buffer != nil {
			d.buffer.SetAudioCodec("mp3", nil)
		}
		d.reader.OnDataMPEG1Audio(track, func(pts int64, frames [][]byte) error {
			return d.handleMPEG1Audio(pts, frames)
		})
		d.config.Logger.Debug("Found MPEG-1 audio track",
			slog.Uint64("pid", uint64(track.PID)),
			slog.Int64("frame_duration_ticks", d.audioFrameDuration))

	case *mpegts.CodecOpus:
		d.audioTrack = track
		d.audioCodec = "opus"
		// Opus typically uses 20ms frames at 48kHz = 960 samples
		// Frame duration = 960 * 90000 / 48000 = 1800 ticks
		d.audioSampleRate = 48000
		d.audioFrameDuration = int64(960 * 90000 / d.audioSampleRate)
		if d.buffer != nil {
			d.buffer.SetAudioCodec("opus", nil)
		}
		d.reader.OnDataOpus(track, func(pts int64, packets [][]byte) error {
			return d.handleOpus(pts, packets)
		})
		d.config.Logger.Debug("Found Opus audio track",
			slog.Uint64("pid", uint64(track.PID)),
			slog.Int("channels", codec.ChannelCount),
			slog.Int64("frame_duration_ticks", d.audioFrameDuration))

	default:
		d.config.Logger.Debug("Found unsupported track",
			slog.Uint64("pid", uint64(track.PID)),
			slog.String("type", fmt.Sprintf("%T", track.Codec)))
	}
}

// handleH264 processes H.264 access units.
// Emits the entire access unit as a single sample in Annex B format.
func (d *TSDemuxer) handleH264(pts, dts int64, au [][]byte) error {
	if len(au) == 0 {
		return nil
	}

	// Check if this AU contains a keyframe (IDR or recovery point)
	isKeyframe := h264.IsRandomAccess(au)

	// Convert AU to Annex B format using mediacommon
	annexB, err := h264.AnnexB(au).Marshal()
	if err != nil || len(annexB) == 0 {
		return nil
	}

	d.emitVideoSample(pts, dts, annexB, isKeyframe)
	return nil
}

// handleH265 processes H.265 access units.
// Emits the entire access unit as a single sample in Annex B format.
func (d *TSDemuxer) handleH265(pts, dts int64, au [][]byte) error {
	if len(au) == 0 {
		return nil
	}

	// Check if this AU contains a keyframe (IRAP picture)
	isKeyframe := h265.IsRandomAccess(au)

	// Convert AU to Annex B format using mediacommon (same format as H.264)
	annexB, err := h264.AnnexB(au).Marshal()
	if err != nil || len(annexB) == 0 {
		return nil
	}

	d.emitVideoSample(pts, dts, annexB, isKeyframe)
	return nil
}

// handleMPEG4Audio processes AAC audio units.
// Each access unit in the slice gets an incremented PTS based on frame duration.
func (d *TSDemuxer) handleMPEG4Audio(pts int64, aus [][]byte) error {
	currentPTS := pts
	frameDuration := d.audioFrameDuration
	if frameDuration <= 0 {
		// Fallback: AAC 1024 samples @ 48kHz = 1920 ticks
		frameDuration = 1920
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

// handleMPEG1Audio processes MPEG-1 audio frames.
// Each frame in the slice gets an incremented PTS based on frame duration.
func (d *TSDemuxer) handleMPEG1Audio(pts int64, frames [][]byte) error {
	currentPTS := pts
	frameDuration := d.audioFrameDuration
	if frameDuration <= 0 {
		// Fallback: MP3 1152 samples @ 48kHz = 2160 ticks
		frameDuration = 2160
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
// Each packet in the slice gets an incremented PTS based on frame duration.
func (d *TSDemuxer) handleOpus(pts int64, packets [][]byte) error {
	currentPTS := pts
	frameDuration := d.audioFrameDuration
	if frameDuration <= 0 {
		// Fallback: Opus 960 samples @ 48kHz = 1800 ticks
		frameDuration = 1800
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

// emitVideoSample writes a video sample to the buffer and/or callback.
func (d *TSDemuxer) emitVideoSample(pts, dts int64, data []byte, isKeyframe bool) {
	// Write to buffer
	if d.buffer != nil {
		if d.config.TargetVariant != "" {
			d.buffer.WriteVideoToVariant(d.config.TargetVariant, pts, dts, data, isKeyframe)
		} else {
			d.buffer.WriteVideo(pts, dts, data, isKeyframe)
		}
	}

	// Invoke callback
	if d.config.OnVideoSample != nil {
		d.config.OnVideoSample(pts, dts, data, isKeyframe)
	}
}

// emitAudioSample writes an audio sample to the buffer and/or callback.
func (d *TSDemuxer) emitAudioSample(pts int64, data []byte) {
	// Write to buffer
	if d.buffer != nil {
		if d.config.TargetVariant != "" {
			d.buffer.WriteAudioToVariant(d.config.TargetVariant, pts, data)
		} else {
			d.buffer.WriteAudio(pts, data)
		}
	}

	// Invoke callback
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

// WriteReader reads all data from the reader and processes it.
func (d *TSDemuxer) WriteReader(r io.Reader) error {
	buf := make([]byte, 188*100) // Read multiple TS packets at once
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if writeErr := d.Write(buf[:n]); writeErr != nil {
				return writeErr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
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

// Tracks returns the discovered tracks after initialization.
func (d *TSDemuxer) Tracks() []*mpegts.Track {
	if d.reader == nil {
		return nil
	}
	return d.reader.Tracks()
}

// TSDemuxerFromReader creates a demuxer that reads from an io.Reader directly.
// This is useful when you have a continuous stream rather than chunked data.
func TSDemuxerFromReader(ctx context.Context, r io.Reader, buffer *SharedESBuffer, config TSDemuxerConfig) (*TSDemuxer, error) {
	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	ctx, cancel := context.WithCancel(ctx)

	d := &TSDemuxer{
		config:   config,
		buffer:   buffer,
		initDone: make(chan struct{}),
		ctx:      ctx,
		cancel:   cancel,
	}

	// Create the mediacommon reader directly from the io.Reader
	d.reader = &mpegts.Reader{R: r}

	// Initialize - this reads until it finds PAT/PMT
	if err := d.reader.Initialize(); err != nil {
		cancel()
		return nil, fmt.Errorf("initializing mpegts reader: %w", err)
	}

	// Process discovered tracks
	for _, track := range d.reader.Tracks() {
		d.setupTrackCallback(track)
	}

	d.initialized = true
	close(d.initDone)

	d.config.Logger.Debug("MPEG-TS demuxer initialized from reader",
		slog.String("video_codec", d.videoCodec),
		slog.String("audio_codec", d.audioCodec))

	// Set up decode error callback
	d.reader.OnDecodeError(func(err error) {
		d.config.Logger.Debug("MPEG-TS decode error",
			slog.String("error", err.Error()))
	})

	return d, nil
}

// Run processes the stream until context is cancelled or EOF.
// Only use this with TSDemuxerFromReader.
func (d *TSDemuxer) Run() error {
	for {
		select {
		case <-d.ctx.Done():
			return d.ctx.Err()
		default:
			if err := d.reader.Read(); err != nil {
				if errors.Is(err, io.EOF) {
					return nil
				}
				return err
			}
		}
	}
}

// annexBToNALUnits extracts NAL units from Annex B formatted data using mediacommon.
// This is a utility function for converting between formats if needed.
func annexBToNALUnits(data []byte) [][]byte {
	var au h264.AnnexB
	if err := au.Unmarshal(data); err != nil {
		return nil
	}
	return au
}
