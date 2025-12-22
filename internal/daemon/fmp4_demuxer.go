// Package daemon provides the tvarr-ffmpegd daemon implementation.
package daemon

import (
	"bytes"
	"context"
	"log/slog"
	"sync"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/jmylchreest/tvarr/internal/observability"
)

// FMP4DemuxerConfig configures the fMP4 demuxer.
type FMP4DemuxerConfig struct {
	Logger           *slog.Logger
	TargetVideoCodec string
	TargetAudioCodec string
	OnVideoSample    func(pts, dts int64, data []byte, isKeyframe bool)
	OnAudioSample    func(pts int64, data []byte)
}

// FMP4Demuxer parses fragmented MP4 output from FFmpeg and extracts ES samples.
// Used for codecs that don't work in MPEG-TS (AV1, VP9).
type FMP4Demuxer struct {
	config FMP4DemuxerConfig
	logger *slog.Logger

	// Accumulates data until we have complete boxes
	buf bytes.Buffer
	mu  sync.Mutex

	// Track state from initialization segment
	init           *fmp4.Init
	videoTrackID   int
	audioTrackID   int
	videoTimescale uint32
	audioTimescale uint32

	// Timing state per track
	videoBaseTime uint64
	audioBaseTime uint64

	// Track whether we've seen the init segment
	initDone bool
}

// NewFMP4Demuxer creates a new fragmented MP4 demuxer.
func NewFMP4Demuxer(config FMP4DemuxerConfig) *FMP4Demuxer {
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &FMP4Demuxer{
		config: config,
		logger: logger,
	}
}

// Write processes data from FFmpeg output.
// The demuxer buffers data and parses complete fMP4 boxes.
func (d *FMP4Demuxer) Write(data []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.buf.Write(data)

	// Try to parse what we have
	return d.parse()
}

// Close releases resources.
func (d *FMP4Demuxer) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.buf.Reset()
}

// parse attempts to parse complete fMP4 structures from the buffer.
func (d *FMP4Demuxer) parse() error {
	for d.buf.Len() >= 8 {
		// Peek at box size and type
		header := d.buf.Bytes()[:8]
		boxSize := uint32(header[0])<<24 | uint32(header[1])<<16 | uint32(header[2])<<8 | uint32(header[3])
		boxType := string(header[4:8])

		// Handle extended size
		if boxSize == 1 && d.buf.Len() >= 16 {
			extHeader := d.buf.Bytes()[:16]
			boxSize = uint32(uint64(extHeader[8])<<56 | uint64(extHeader[9])<<48 | uint64(extHeader[10])<<40 | uint64(extHeader[11])<<32 |
				uint64(extHeader[12])<<24 | uint64(extHeader[13])<<16 | uint64(extHeader[14])<<8 | uint64(extHeader[15]))
		}

		// Wait for complete box
		if uint32(d.buf.Len()) < boxSize {
			return nil
		}

		// Extract the complete box
		boxData := make([]byte, boxSize)
		d.buf.Read(boxData)

		// Process based on box type
		switch boxType {
		case "ftyp":
			// File type box - just skip it
			d.logger.Debug("fMP4 demuxer: ftyp box received", slog.Int("size", int(boxSize)))

		case "moov":
			// Initialization segment
			if err := d.parseInit(boxData); err != nil {
				d.logger.Warn("fMP4 demuxer: failed to parse moov",
					slog.String("error", err.Error()))
				return err
			}

		case "moof":
			// Media fragment - we need both moof and mdat
			// Put moof back and wait for mdat
			d.buf = *bytes.NewBuffer(append(boxData, d.buf.Bytes()...))
			return d.tryParseMoofMdat()

		case "mdat":
			// mdat without moof - shouldn't happen with valid fMP4
			d.logger.Warn("fMP4 demuxer: mdat without moof")
		}
	}

	return nil
}

// tryParseMoofMdat attempts to parse a moof+mdat pair from the buffer.
func (d *FMP4Demuxer) tryParseMoofMdat() error {
	if d.buf.Len() < 16 {
		return nil
	}

	// Find moof size
	data := d.buf.Bytes()
	moofSize := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])

	// Check if we have moof + mdat header
	if uint32(len(data)) < moofSize+8 {
		return nil
	}

	// Check for mdat after moof
	mdatHeader := data[moofSize : moofSize+8]
	mdatSize := uint32(mdatHeader[0])<<24 | uint32(mdatHeader[1])<<16 | uint32(mdatHeader[2])<<8 | uint32(mdatHeader[3])
	mdatType := string(mdatHeader[4:8])

	if mdatType != "mdat" {
		// Not mdat after moof - skip the moof and continue
		d.buf.Next(int(moofSize))
		return nil
	}

	// Check if we have complete moof+mdat
	totalSize := moofSize + mdatSize
	if uint32(len(data)) < totalSize {
		return nil
	}

	// Extract moof+mdat
	fragment := make([]byte, totalSize)
	d.buf.Read(fragment)

	// Parse the fragment
	return d.parseFragment(fragment)
}

// parseInit parses the initialization segment (moov box).
func (d *FMP4Demuxer) parseInit(moovData []byte) error {
	// fMP4 Init expects ftyp+moov but we're just passing moov
	// We need to synthesize a minimal ftyp or adjust parsing
	// Actually, Init.Unmarshal expects to read from a ReadSeeker starting with moov

	d.init = &fmp4.Init{}
	reader := bytes.NewReader(moovData)
	if err := d.init.Unmarshal(reader); err != nil {
		return err
	}

	// Find video and audio track IDs and timescales
	for _, track := range d.init.Tracks {
		switch track.Codec.(type) {
		case *fmp4.CodecH264, *fmp4.CodecH265, *fmp4.CodecAV1, *fmp4.CodecVP9:
			d.videoTrackID = track.ID
			d.videoTimescale = track.TimeScale
			d.logger.Info("fMP4 demuxer: found video track",
				slog.Int("track_id", track.ID),
				slog.Uint64("timescale", uint64(track.TimeScale)),
				slog.String("codec_type", d.config.TargetVideoCodec),
			)
		case *fmp4.CodecMPEG4Audio, *fmp4.CodecOpus, *fmp4.CodecAC3, *fmp4.CodecEAC3:
			d.audioTrackID = track.ID
			d.audioTimescale = track.TimeScale
			d.logger.Info("fMP4 demuxer: found audio track",
				slog.Int("track_id", track.ID),
				slog.Uint64("timescale", uint64(track.TimeScale)),
				slog.String("codec_type", d.config.TargetAudioCodec),
			)
		}
	}

	d.initDone = true
	d.logger.Info("fMP4 demuxer: initialization complete",
		slog.Int("video_track", d.videoTrackID),
		slog.Int("audio_track", d.audioTrackID),
	)

	return nil
}

// parseFragment parses a media fragment (moof+mdat).
func (d *FMP4Demuxer) parseFragment(data []byte) error {
	if !d.initDone {
		d.logger.Warn("fMP4 demuxer: fragment received before init")
		return nil
	}

	var parts fmp4.Parts
	if err := parts.Unmarshal(data); err != nil {
		d.logger.Warn("fMP4 demuxer: failed to parse fragment",
			slog.String("error", err.Error()),
			slog.Int("data_len", len(data)))
		return err
	}

	// Log fragment parsing for diagnostics
	d.logger.Debug("fMP4 demuxer: parsing fragment",
		slog.Int("data_len", len(data)),
		slog.Int("parts_count", len(parts)))

	// Process each part (usually just one)
	for _, part := range parts {
		for _, track := range part.Tracks {
			if track.ID == d.videoTrackID {
				d.processVideoTrack(track)
			} else if track.ID == d.audioTrackID {
				d.processAudioTrack(track)
			}
		}
	}

	return nil
}

// processVideoTrack extracts video samples from a track.
func (d *FMP4Demuxer) processVideoTrack(track *fmp4.PartTrack) {
	if d.config.OnVideoSample == nil {
		return
	}

	baseTime := track.BaseTime
	timescale := d.videoTimescale
	if timescale == 0 {
		timescale = 90000 // Default to 90kHz
	}

	// Count keyframes in this batch for diagnostic logging
	keyframeCount := 0
	for i, sample := range track.Samples {
		// Convert to 90kHz timescale for PTS/DTS
		dts := int64(baseTime * 90000 / uint64(timescale))
		pts := dts + int64(sample.PTSOffset)*90000/int64(timescale)

		// Keyframe detection:
		// 1. Check IsNonSyncSample flag (standard fMP4 approach)
		// 2. For AV1/VP9 with frag_keyframe: FFmpeg may not set the flag correctly,
		//    but each fragment STARTS with a keyframe (that's what frag_keyframe means).
		//    So the first sample in each fragment (i==0) is always a keyframe.
		isKeyframe := !sample.IsNonSyncSample
		if !isKeyframe && i == 0 {
			// First sample of fragment with frag_keyframe is always a keyframe
			isKeyframe = true
		}

		if isKeyframe {
			keyframeCount++
			d.config.Logger.Log(context.Background(), observability.LevelTrace, "fMP4 demuxer: detected keyframe",
				slog.Int64("pts", pts),
				slog.Int64("dts", dts),
				slog.Int("payload_len", len(sample.Payload)),
				slog.Bool("is_non_sync_sample", sample.IsNonSyncSample),
				slog.Bool("fragment_first", i == 0))
		}

		// Get raw payload - for AV1/VP9, payload is already in the right format
		// FFmpeg outputs AV1 OBUs directly in the mdat
		d.config.OnVideoSample(pts, dts, sample.Payload, isKeyframe)

		// Advance base time
		baseTime += uint64(sample.Duration)
	}

	// Log sample processing stats for diagnostics
	if len(track.Samples) > 0 {
		d.logger.Debug("fMP4 demuxer: processed video samples",
			slog.Int("sample_count", len(track.Samples)),
			slog.Int("keyframe_count", keyframeCount),
			slog.Uint64("base_time", track.BaseTime))
	}
}

// processAudioTrack extracts audio samples from a track.
func (d *FMP4Demuxer) processAudioTrack(track *fmp4.PartTrack) {
	if d.config.OnAudioSample == nil {
		return
	}

	baseTime := track.BaseTime
	timescale := d.audioTimescale
	if timescale == 0 {
		timescale = 90000 // Default to 90kHz
	}

	for _, sample := range track.Samples {
		// Convert to 90kHz timescale for PTS
		pts := int64(baseTime * 90000 / uint64(timescale))

		d.config.OnAudioSample(pts, sample.Payload)

		// Advance base time
		baseTime += uint64(sample.Duration)
	}
}

// Verify FMP4Demuxer implements OutputDemuxer interface.
var _ OutputDemuxer = (*FMP4Demuxer)(nil)
