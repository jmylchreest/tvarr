// Package relay provides stream relay functionality with CMAF support.
package relay

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/av1"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mp4"
	mp4codecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	// Note: vp9 codec package is imported in fmp4_adapter.go for header parsing
)

// Errors for fMP4 operations
var (
	ErrCodecNotConfigured = errors.New("codec not configured")
)

// FMP4Writer wraps mediacommon's fmp4 for writing init segments and parts.
type FMP4Writer struct {
	mu sync.Mutex

	// Track configuration
	videoTrack *fmp4.InitTrack
	audioTrack *fmp4.InitTrack

	// Codec params
	h264SPS      []byte
	h264PPS      []byte
	h265VPS      []byte
	h265SPS      []byte
	h265PPS      []byte
	av1SeqHeader []byte
	// VP9 params
	vp9Width             int
	vp9Height            int
	vp9Profile           uint8
	vp9BitDepth          uint8
	vp9ChromaSubsampling uint8
	vp9ColorRange        bool
	vp9Configured        bool
	aacConf              *mpeg4audio.AudioSpecificConfig

	// Opus params
	opusChannelCount int
	opusConfigured   bool

	// AC3 params
	ac3SampleRate   int
	ac3ChannelCount int
	ac3Configured   bool

	// EAC3 params
	eac3SampleRate   int
	eac3ChannelCount int
	eac3Configured   bool

	// MP3 params
	mp3SampleRate   int
	mp3ChannelCount int
	mp3Configured   bool

	// State
	seqNum      uint32
	initialized bool
}

// NewFMP4Writer creates a new fMP4 writer using mediacommon.
func NewFMP4Writer() *FMP4Writer {
	return &FMP4Writer{
		seqNum: 1,
	}
}

// SetH264Params sets H.264 codec parameters.
func (w *FMP4Writer) SetH264Params(sps, pps []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.h264SPS = sps
	w.h264PPS = pps
}

// SetH265Params sets H.265 codec parameters.
func (w *FMP4Writer) SetH265Params(vps, sps, pps []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.h265VPS = vps
	w.h265SPS = sps
	w.h265PPS = pps
}

// SetAV1Params sets AV1 codec parameters.
func (w *FMP4Writer) SetAV1Params(seqHeader []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.av1SeqHeader = seqHeader
}

// SetVP9Params sets VP9 codec parameters.
func (w *FMP4Writer) SetVP9Params(width, height int, profile, bitDepth, chromaSubsampling uint8, colorRange bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.vp9Width = width
	w.vp9Height = height
	w.vp9Profile = profile
	w.vp9BitDepth = bitDepth
	w.vp9ChromaSubsampling = chromaSubsampling
	w.vp9ColorRange = colorRange
	w.vp9Configured = true
}

// SetAACConfig sets AAC codec configuration.
func (w *FMP4Writer) SetAACConfig(config *mpeg4audio.AudioSpecificConfig) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.aacConf = config
}

// SetOpusConfig sets Opus codec configuration.
func (w *FMP4Writer) SetOpusConfig(channelCount int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if channelCount <= 0 {
		channelCount = 2 // Default to stereo
	}
	w.opusChannelCount = channelCount
	w.opusConfigured = true
}

// SetAC3Config sets AC3 codec configuration.
func (w *FMP4Writer) SetAC3Config(sampleRate, channelCount int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if sampleRate <= 0 {
		sampleRate = 48000
	}
	if channelCount <= 0 {
		channelCount = 2
	}
	w.ac3SampleRate = sampleRate
	w.ac3ChannelCount = channelCount
	w.ac3Configured = true
}

// SetEAC3Config sets EAC3 (Enhanced AC3 / Dolby Digital Plus) codec configuration.
func (w *FMP4Writer) SetEAC3Config(sampleRate, channelCount int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if sampleRate <= 0 {
		sampleRate = 48000
	}
	if channelCount <= 0 {
		channelCount = 2
	}
	w.eac3SampleRate = sampleRate
	w.eac3ChannelCount = channelCount
	w.eac3Configured = true
}

// SetMP3Config sets MP3 codec configuration.
func (w *FMP4Writer) SetMP3Config(sampleRate, channelCount int) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if sampleRate <= 0 {
		sampleRate = 48000
	}
	if channelCount <= 0 {
		channelCount = 2
	}
	w.mp3SampleRate = sampleRate
	w.mp3ChannelCount = channelCount
	w.mp3Configured = true
}

// GenerateInit generates an fMP4 initialization segment.
// Returns an error if hasVideo is true but no valid video codec can be created.
func (w *FMP4Writer) GenerateInit(hasVideo, hasAudio bool, videoTimescale, audioTimescale uint32) ([]byte, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	init := fmp4.Init{
		Tracks: make([]*fmp4.InitTrack, 0),
	}

	trackID := 1

	if hasVideo {
		var codec mp4.Codec
		var codecErr error

		if w.h264SPS != nil && w.h264PPS != nil {
			var spsp h264.SPS
			if err := spsp.Unmarshal(w.h264SPS); err == nil {
				codec = &mp4.CodecH264{
					SPS: w.h264SPS,
					PPS: w.h264PPS,
				}
			} else {
				codecErr = fmt.Errorf("H.264 SPS unmarshal failed: %w (SPS len=%d, PPS len=%d)", err, len(w.h264SPS), len(w.h264PPS))
			}
		} else if w.h265SPS != nil && w.h265PPS != nil {
			var spsp h265.SPS
			if err := spsp.Unmarshal(w.h265SPS); err == nil {
				codec = &mp4.CodecH265{
					VPS: w.h265VPS,
					SPS: w.h265SPS,
					PPS: w.h265PPS,
				}
			} else {
				codecErr = fmt.Errorf("H.265 SPS unmarshal failed: %w (VPS len=%d, SPS len=%d, PPS len=%d)", err, len(w.h265VPS), len(w.h265SPS), len(w.h265PPS))
			}
		} else if w.av1SeqHeader != nil {
			// AV1 codec - verify sequence header is valid
			var seqHdr av1.SequenceHeader
			if err := seqHdr.Unmarshal(w.av1SeqHeader); err == nil {
				codec = &mp4.CodecAV1{
					SequenceHeader: w.av1SeqHeader,
				}
			} else {
				codecErr = fmt.Errorf("AV1 sequence header unmarshal failed: %w (len=%d)", err, len(w.av1SeqHeader))
			}
		} else if w.vp9Configured {
			// VP9 codec
			codec = &mp4.CodecVP9{
				Width:             w.vp9Width,
				Height:            w.vp9Height,
				Profile:           w.vp9Profile,
				BitDepth:          w.vp9BitDepth,
				ChromaSubsampling: w.vp9ChromaSubsampling,
				ColorRange:        w.vp9ColorRange,
			}
		} else {
			// No video codec params available
			codecErr = errors.New("no video codec params configured (missing SPS/PPS/VPS or sequence header)")
		}

		if codec != nil {
			w.videoTrack = &fmp4.InitTrack{
				ID:        trackID,
				TimeScale: videoTimescale,
				Codec:     codec,
			}
			init.Tracks = append(init.Tracks, w.videoTrack)
			trackID++
		} else if codecErr != nil {
			// Video was expected but we couldn't create a codec - return error
			return nil, fmt.Errorf("video codec creation failed: %w", codecErr)
		}
	}

	if hasAudio {
		var audioCodec mp4.Codec

		// Check audio codecs in order of priority
		if w.opusConfigured {
			audioCodec = &mp4.CodecOpus{
				ChannelCount: w.opusChannelCount,
			}
		} else if w.ac3Configured {
			audioCodec = &mp4.CodecAC3{
				SampleRate:   w.ac3SampleRate,
				ChannelCount: w.ac3ChannelCount,
			}
		} else if w.eac3Configured {
			audioCodec = &mp4codecs.EAC3{
				SampleRate:   w.eac3SampleRate,
				ChannelCount: w.eac3ChannelCount,
			}
		} else if w.mp3Configured {
			audioCodec = &mp4.CodecMPEG1Audio{
				SampleRate:   w.mp3SampleRate,
				ChannelCount: w.mp3ChannelCount,
			}
		} else if w.aacConf != nil {
			audioCodec = &mp4.CodecMPEG4Audio{
				Config: *w.aacConf,
			}
		}

		if audioCodec != nil {
			w.audioTrack = &fmp4.InitTrack{
				ID:        trackID,
				TimeScale: audioTimescale,
				Codec:     audioCodec,
			}
			init.Tracks = append(init.Tracks, w.audioTrack)
		}
	}

	if len(init.Tracks) == 0 {
		return nil, ErrCodecNotConfigured
	}

	// Marshal to bytes
	var buf seekablebuffer.Buffer
	if err := init.Marshal(&buf); err != nil {
		return nil, fmt.Errorf("marshaling init: %w", err)
	}

	w.initialized = true
	return buf.Bytes(), nil
}

// GeneratePart generates an fMP4 media part.
// Only tracks with actual samples are included to avoid timestamp discontinuities.
func (w *FMP4Writer) GeneratePart(videoSamples, audioSamples []*fmp4.Sample, videoBaseTime, audioBaseTime uint64) ([]byte, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.initialized {
		return nil, errors.New("not initialized")
	}

	part := fmp4.Part{
		SequenceNumber: w.seqNum,
		Tracks:         make([]*fmp4.PartTrack, 0),
	}

	if w.videoTrack != nil && len(videoSamples) > 0 {
		part.Tracks = append(part.Tracks, &fmp4.PartTrack{
			ID:       w.videoTrack.ID,
			BaseTime: videoBaseTime,
			Samples:  videoSamples,
		})
	}

	if w.audioTrack != nil && len(audioSamples) > 0 {
		part.Tracks = append(part.Tracks, &fmp4.PartTrack{
			ID:       w.audioTrack.ID,
			BaseTime: audioBaseTime,
			Samples:  audioSamples,
		})
	}

	if len(part.Tracks) == 0 {
		return nil, errors.New("no samples to write")
	}

	var buf seekablebuffer.Buffer
	if err := part.Marshal(&buf); err != nil {
		return nil, fmt.Errorf("marshaling part: %w", err)
	}

	w.seqNum++
	return buf.Bytes(), nil
}

// VideoTrackID returns the video track ID.
func (w *FMP4Writer) VideoTrackID() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.videoTrack != nil {
		return w.videoTrack.ID
	}
	return 0
}

// AudioTrackID returns the audio track ID.
func (w *FMP4Writer) AudioTrackID() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.audioTrack != nil {
		return w.audioTrack.ID
	}
	return 0
}

// Reset resets the writer state.
func (w *FMP4Writer) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.videoTrack = nil
	w.audioTrack = nil
	w.seqNum = 1
	w.initialized = false
}

// SequenceNumber returns the current sequence number.
func (w *FMP4Writer) SequenceNumber() uint32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.seqNum
}

// filterSegmentByTrack filters an fMP4 segment to contain only the specified track type.
// trackType should be "video" or "audio".
// The function uses mediacommon's fmp4 library to parse and filter the segment.
// It determines the correct track ID by parsing the init segment from the provider.
func filterSegmentByTrack(segmentData []byte, trackType string, provider FMP4SegmentProvider) ([]byte, error) {
	if len(segmentData) == 0 {
		return nil, fmt.Errorf("empty segment data")
	}

	if trackType == "" {
		return segmentData, nil // No filtering needed
	}

	// Get the init segment to determine actual track IDs
	initSeg := provider.GetInitSegment()
	if initSeg == nil || initSeg.IsEmpty() {
		return nil, fmt.Errorf("no init segment available for track ID lookup")
	}

	// Parse the init segment to find track IDs by codec type
	var parsedInit fmp4.Init
	if err := parsedInit.Unmarshal(bytes.NewReader(initSeg.Data)); err != nil {
		return nil, fmt.Errorf("parsing init segment for track IDs: %w", err)
	}

	// Find the track ID that matches the requested type
	var targetTrackID int
	for _, track := range parsedInit.Tracks {
		isVideo := track.Codec.IsVideo()
		if (trackType == "video" && isVideo) || (trackType == "audio" && !isVideo) {
			targetTrackID = track.ID
			break
		}
	}

	if targetTrackID == 0 {
		return nil, fmt.Errorf("no %s track found in init segment", trackType)
	}

	// Parse the segment using mediacommon (Parts has Unmarshal, Part does not)
	var parts fmp4.Parts
	if err := parts.Unmarshal(segmentData); err != nil {
		return nil, fmt.Errorf("parsing segment: %w", err)
	}

	if len(parts) == 0 {
		return nil, fmt.Errorf("no parts found in segment")
	}

	// Filter tracks in each part to only include the target track
	filteredParts := make(fmp4.Parts, 0, len(parts))
	for _, part := range parts {
		filteredTracks := make([]*fmp4.PartTrack, 0)
		for _, track := range part.Tracks {
			if track.ID == targetTrackID {
				filteredTracks = append(filteredTracks, track)
			}
		}

		if len(filteredTracks) > 0 {
			filteredParts = append(filteredParts, &fmp4.Part{
				SequenceNumber: part.SequenceNumber,
				Tracks:         filteredTracks,
			})
		}
	}

	if len(filteredParts) == 0 {
		return nil, fmt.Errorf("no %s track (id=%d) found in segment", trackType, targetTrackID)
	}

	// Marshal the filtered parts
	var buf seekablebuffer.Buffer
	if err := filteredParts.Marshal(&buf); err != nil {
		return nil, fmt.Errorf("marshaling filtered segment: %w", err)
	}

	return buf.Bytes(), nil
}
