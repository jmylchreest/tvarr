// Package daemon provides the tvarr-ffmpegd daemon implementation.
package daemon

import (
	"bytes"
	"context"
	"encoding/binary"
	"log/slog"
	"sync"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mp4"
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

	// H.264 codec parameters (SPS/PPS) - stored in Annex B format for prepending to keyframes
	h264SPS []byte
	h264PPS []byte
	// NAL length size from avc1 config (usually 4)
	h264NALLengthSize int
	// Track if we've logged the first H.264 conversion
	firstH264KeyframeLogged bool

	// H.265 codec parameters (VPS/SPS/PPS) - stored in Annex B format for prepending to keyframes
	h265VPS []byte
	h265SPS []byte
	h265PPS []byte
	// NAL length size from hvc1 config (usually 4)
	h265NALLengthSize int
	// Track if we've logged the first H.265 conversion
	firstH265KeyframeLogged bool
	// Track if we've warned about missing audio in fragments
	audioMissingLogged bool
	// Track if we've logged first audio samples
	firstAudioLogged bool

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
	// Compact buffer if it's empty to release memory
	// bytes.Buffer never shrinks, so we need to reset when empty
	if d.buf.Len() == 0 && d.buf.Cap() > 1024*1024 {
		d.buf = bytes.Buffer{}
	}

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

		// For moof boxes, check if we have the complete moof+mdat pair before extracting
		if boxType == "moof" {
			// Check if we have enough data for moof + mdat header
			if uint32(d.buf.Len()) < boxSize+8 {
				return nil // Wait for more data
			}

			// Peek at what follows the moof
			bufData := d.buf.Bytes()
			mdatHeader := bufData[boxSize : boxSize+8]
			mdatSize := uint32(mdatHeader[0])<<24 | uint32(mdatHeader[1])<<16 | uint32(mdatHeader[2])<<8 | uint32(mdatHeader[3])
			mdatType := string(mdatHeader[4:8])

			if mdatType != "mdat" {
				// Not mdat after moof - skip the moof and continue
				d.buf.Next(int(boxSize))
				continue
			}

			// Check if we have complete moof+mdat
			totalSize := boxSize + mdatSize
			if uint32(d.buf.Len()) < totalSize {
				return nil // Wait for complete fragment
			}

			// Extract and parse the complete moof+mdat fragment
			fragment := make([]byte, totalSize)
			_, _ = d.buf.Read(fragment)

			if err := d.parseFragment(fragment); err != nil {
				return err
			}
			continue
		}

		// Extract the complete box for non-moof types
		boxData := make([]byte, boxSize)
		_, _ = d.buf.Read(boxData)

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

		case "mdat":
			// mdat without moof - shouldn't happen with valid fMP4
			d.logger.Warn("fMP4 demuxer: mdat without moof")
		}
	}

	return nil
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
		switch codec := track.Codec.(type) {
		case *mp4.CodecH265:
			d.videoTrackID = track.ID
			d.videoTimescale = track.TimeScale
			// Extract VPS/SPS/PPS for prepending to keyframes
			d.h265VPS = codec.VPS
			d.h265SPS = codec.SPS
			d.h265PPS = codec.PPS
			// Default NAL length size is 4 bytes
			d.h265NALLengthSize = 4
			d.logger.Info("fMP4 demuxer: found H.265 video track",
				slog.Int("track_id", track.ID),
				slog.Uint64("timescale", uint64(track.TimeScale)),
				slog.Int("vps_len", len(codec.VPS)),
				slog.Int("sps_len", len(codec.SPS)),
				slog.Int("pps_len", len(codec.PPS)),
			)
		case *mp4.CodecH264:
			d.videoTrackID = track.ID
			d.videoTimescale = track.TimeScale
			// Extract SPS/PPS for prepending to keyframes
			d.h264SPS = codec.SPS
			d.h264PPS = codec.PPS
			// Default NAL length size is 4 bytes
			d.h264NALLengthSize = 4
			d.logger.Info("fMP4 demuxer: found H.264 video track",
				slog.Int("track_id", track.ID),
				slog.Uint64("timescale", uint64(track.TimeScale)),
				slog.Int("sps_len", len(codec.SPS)),
				slog.Int("pps_len", len(codec.PPS)),
			)
		case *mp4.CodecAV1, *mp4.CodecVP9:
			d.videoTrackID = track.ID
			d.videoTimescale = track.TimeScale
			d.logger.Info("fMP4 demuxer: found video track",
				slog.Int("track_id", track.ID),
				slog.Uint64("timescale", uint64(track.TimeScale)),
				slog.String("codec_type", d.config.TargetVideoCodec),
			)
		case *mp4.CodecMPEG4Audio, *mp4.CodecOpus, *mp4.CodecAC3, *mp4.CodecEAC3, *mp4.CodecMPEG1Audio:
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
		hasVideo := false
		hasAudio := false
		for _, track := range part.Tracks {
			if track.ID == d.videoTrackID {
				hasVideo = true
				d.processVideoTrack(track)
			} else if track.ID == d.audioTrackID {
				hasAudio = true
				d.processAudioTrack(track)
			}
		}
		// Log if audio track is missing from fragments (but was in init)
		if d.audioTrackID > 0 && hasVideo && !hasAudio && !d.audioMissingLogged {
			d.logger.Warn("fMP4 demuxer: audio track missing from fragment (init had audio)",
				slog.Int("video_track_id", d.videoTrackID),
				slog.Int("audio_track_id", d.audioTrackID))
			d.audioMissingLogged = true
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

	// Check if this is H.264 or H.265 (we have SPS/PPS extracted)
	isH264 := len(d.h264SPS) > 0 || len(d.h264PPS) > 0
	isH265 := len(d.h265VPS) > 0 || len(d.h265SPS) > 0 || len(d.h265PPS) > 0

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

		// Process the payload based on codec
		var outputData []byte
		if isH264 {
			// H.264: Convert from avc1 (length-prefixed) to Annex B (start code prefixed)
			// and prepend SPS/PPS to keyframes
			outputData = d.convertH264ToAnnexB(sample.Payload, isKeyframe)
		} else if isH265 {
			// H.265: Convert from hvc1 (length-prefixed) to Annex B (start code prefixed)
			// and prepend VPS/SPS/PPS to keyframes
			outputData = d.convertH265ToAnnexB(sample.Payload, isKeyframe)
		} else {
			// For AV1/VP9, payload is already in the right format
			// FFmpeg outputs AV1 OBUs directly in the mdat
			outputData = sample.Payload
		}

		d.config.OnVideoSample(pts, dts, outputData, isKeyframe)

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

// convertH264ToAnnexB converts H.264 data from avc1 format (length-prefixed NALs)
// to Annex B format (start code prefixed NALs), prepending SPS/PPS to keyframes.
func (d *FMP4Demuxer) convertH264ToAnnexB(payload []byte, isKeyframe bool) []byte {
	nalLengthSize := d.h264NALLengthSize
	if nalLengthSize <= 0 {
		nalLengthSize = 4
	}

	// Annex B start code
	startCode := []byte{0x00, 0x00, 0x00, 0x01}

	// Build output buffer
	var out bytes.Buffer

	// For keyframes, prepend SPS/PPS
	if isKeyframe {
		if len(d.h264SPS) > 0 {
			out.Write(startCode)
			out.Write(d.h264SPS)
		}
		if len(d.h264PPS) > 0 {
			out.Write(startCode)
			out.Write(d.h264PPS)
		}

		// Log first keyframe conversion for diagnostics
		if !d.firstH264KeyframeLogged {
			d.firstH264KeyframeLogged = true
			d.logger.Info("fMP4 demuxer: first H.264 keyframe conversion to Annex B",
				slog.Int("input_payload_len", len(payload)),
				slog.Int("sps_len", len(d.h264SPS)),
				slog.Int("pps_len", len(d.h264PPS)),
				slog.Bool("has_sps", len(d.h264SPS) > 0),
				slog.Bool("has_pps", len(d.h264PPS) > 0))
		}
	}

	// Convert each NAL from length-prefixed to start code prefixed
	offset := 0
	for offset < len(payload) {
		if offset+nalLengthSize > len(payload) {
			break
		}

		// Read NAL length (big-endian)
		var nalLen uint32
		switch nalLengthSize {
		case 1:
			nalLen = uint32(payload[offset])
		case 2:
			nalLen = uint32(binary.BigEndian.Uint16(payload[offset:]))
		case 4:
			nalLen = binary.BigEndian.Uint32(payload[offset:])
		default:
			nalLen = binary.BigEndian.Uint32(payload[offset:])
		}

		offset += nalLengthSize

		if offset+int(nalLen) > len(payload) {
			break
		}

		// Write start code + NAL data
		out.Write(startCode)
		out.Write(payload[offset : offset+int(nalLen)])

		offset += int(nalLen)
	}

	return out.Bytes()
}

// convertH265ToAnnexB converts H.265 data from hvc1 format (length-prefixed NALs)
// to Annex B format (start code prefixed NALs), prepending VPS/SPS/PPS to keyframes.
func (d *FMP4Demuxer) convertH265ToAnnexB(payload []byte, isKeyframe bool) []byte {
	nalLengthSize := d.h265NALLengthSize
	if nalLengthSize <= 0 {
		nalLengthSize = 4
	}

	// Annex B start code
	startCode := []byte{0x00, 0x00, 0x00, 0x01}

	// Build output buffer
	var out bytes.Buffer

	// For keyframes, prepend VPS/SPS/PPS
	if isKeyframe {
		if len(d.h265VPS) > 0 {
			out.Write(startCode)
			out.Write(d.h265VPS)
		}
		if len(d.h265SPS) > 0 {
			out.Write(startCode)
			out.Write(d.h265SPS)
		}
		if len(d.h265PPS) > 0 {
			out.Write(startCode)
			out.Write(d.h265PPS)
		}

		// Log first keyframe conversion for diagnostics
		if !d.firstH265KeyframeLogged {
			d.firstH265KeyframeLogged = true
			d.logger.Info("fMP4 demuxer: first H.265 keyframe conversion to Annex B",
				slog.Int("input_payload_len", len(payload)),
				slog.Int("vps_len", len(d.h265VPS)),
				slog.Int("sps_len", len(d.h265SPS)),
				slog.Int("pps_len", len(d.h265PPS)),
				slog.Bool("has_vps", len(d.h265VPS) > 0),
				slog.Bool("has_sps", len(d.h265SPS) > 0),
				slog.Bool("has_pps", len(d.h265PPS) > 0))
		}
	}

	// Convert each NAL from length-prefixed to start code prefixed
	offset := 0
	for offset < len(payload) {
		if offset+nalLengthSize > len(payload) {
			break
		}

		// Read NAL length (big-endian)
		var nalLen uint32
		switch nalLengthSize {
		case 1:
			nalLen = uint32(payload[offset])
		case 2:
			nalLen = uint32(binary.BigEndian.Uint16(payload[offset:]))
		case 4:
			nalLen = binary.BigEndian.Uint32(payload[offset:])
		default:
			nalLen = binary.BigEndian.Uint32(payload[offset:])
		}

		offset += nalLengthSize

		if offset+int(nalLen) > len(payload) {
			break
		}

		// Write start code + NAL data
		out.Write(startCode)
		out.Write(payload[offset : offset+int(nalLen)])

		offset += int(nalLen)
	}

	return out.Bytes()
}

// processAudioTrack extracts audio samples from a track.
func (d *FMP4Demuxer) processAudioTrack(track *fmp4.PartTrack) {
	if d.config.OnAudioSample == nil {
		return
	}

	// Log first audio samples received
	if !d.firstAudioLogged && len(track.Samples) > 0 {
		d.logger.Info("fMP4 demuxer: first audio samples from fragment",
			slog.Int("sample_count", len(track.Samples)),
			slog.Uint64("base_time", track.BaseTime))
		d.firstAudioLogged = true
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
