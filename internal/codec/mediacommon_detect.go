// Package codec provides runtime detection of mediacommon codec support.
// This file detects which codecs are supported by the mediacommon library
// at compile time, automatically adapting when upstream adds new codecs.
package codec

import (
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts/codecs"
)

// mediacommonSupportedCodecs tracks which codec types exist in mediacommon.
// These are detected at init time using type assertions.
var mediacommonSupportedCodecs = struct {
	// Video codecs
	H264  bool
	H265  bool
	MPEG1 bool
	MPEG4 bool
	// Audio codecs
	AAC  bool
	AC3  bool
	EAC3 bool
	MP3  bool
	Opus bool
}{}

func init() {
	// Detect supported video codecs by checking if types exist in mediacommon
	// We use type assertions against mpegts.Codec interface

	// H264 - check if CodecH264 type exists
	var h264 mpegts.Codec = &mpegts.CodecH264{}
	mediacommonSupportedCodecs.H264 = !isUnsupportedCodec(h264)

	// H265 - check if CodecH265 type exists
	var h265 mpegts.Codec = &mpegts.CodecH265{}
	mediacommonSupportedCodecs.H265 = !isUnsupportedCodec(h265)

	// MPEG1 Video - check if CodecMPEG1Video type exists
	var mpeg1 mpegts.Codec = &mpegts.CodecMPEG1Video{}
	mediacommonSupportedCodecs.MPEG1 = !isUnsupportedCodec(mpeg1)

	// MPEG4 Video - check if CodecMPEG4Video type exists
	var mpeg4 mpegts.Codec = &mpegts.CodecMPEG4Video{}
	mediacommonSupportedCodecs.MPEG4 = !isUnsupportedCodec(mpeg4)

	// AAC - check if CodecMPEG4Audio type exists
	var aac mpegts.Codec = &mpegts.CodecMPEG4Audio{}
	mediacommonSupportedCodecs.AAC = !isUnsupportedCodec(aac)

	// AC3 - check if CodecAC3 type exists
	var ac3 mpegts.Codec = &mpegts.CodecAC3{}
	mediacommonSupportedCodecs.AC3 = !isUnsupportedCodec(ac3)

	// EAC3 - check if codecs.EAC3 type exists
	// The EAC3 type is in the codecs subpackage (not aliased in mpegts package yet)
	var eac3 mpegts.Codec = &codecs.EAC3{}
	mediacommonSupportedCodecs.EAC3 = !isUnsupportedCodec(eac3)

	// MP3 - check if CodecMPEG1Audio type exists
	var mp3 mpegts.Codec = &mpegts.CodecMPEG1Audio{}
	mediacommonSupportedCodecs.MP3 = !isUnsupportedCodec(mp3)

	// Opus - check if CodecOpus type exists
	var opus mpegts.Codec = &mpegts.CodecOpus{}
	mediacommonSupportedCodecs.Opus = !isUnsupportedCodec(opus)

	// Update the audio/video registries with detected demuxability
	updateRegistryWithDetectedSupport()
}

// isUnsupportedCodec checks if a codec is the CodecUnsupported sentinel type
func isUnsupportedCodec(c mpegts.Codec) bool {
	_, isUnsupported := c.(*mpegts.CodecUnsupported)
	return isUnsupported
}

// updateRegistryWithDetectedSupport updates the Demuxable flags in registries
// based on what mediacommon actually supports.
func updateRegistryWithDetectedSupport() {
	// Video codecs
	if info, ok := videoRegistry[VideoH264]; ok {
		info.Demuxable = mediacommonSupportedCodecs.H264
	}
	if info, ok := videoRegistry[VideoH265]; ok {
		info.Demuxable = mediacommonSupportedCodecs.H265
	}
	if info, ok := videoRegistry[VideoMPEG1]; ok {
		info.Demuxable = mediacommonSupportedCodecs.MPEG1
	}
	if info, ok := videoRegistry[VideoMPEG4]; ok {
		info.Demuxable = mediacommonSupportedCodecs.MPEG4
	}

	// Audio codecs
	if info, ok := audioRegistry[AudioAAC]; ok {
		info.Demuxable = mediacommonSupportedCodecs.AAC
	}
	if info, ok := audioRegistry[AudioAC3]; ok {
		info.Demuxable = mediacommonSupportedCodecs.AC3
	}
	if info, ok := audioRegistry[AudioEAC3]; ok {
		info.Demuxable = mediacommonSupportedCodecs.EAC3
	}
	if info, ok := audioRegistry[AudioMP3]; ok {
		info.Demuxable = mediacommonSupportedCodecs.MP3
	}
	if info, ok := audioRegistry[AudioOpus]; ok {
		info.Demuxable = mediacommonSupportedCodecs.Opus
	}
}

// IsMediacommonCodecSupported returns whether mediacommon supports demuxing
// the specified codec. This is detected at runtime based on what types
// are exported from mediacommon.
func IsMediacommonCodecSupported(codecName string) bool {
	// Try video codecs first
	if video, ok := ParseVideo(codecName); ok {
		switch video {
		case VideoH264:
			return mediacommonSupportedCodecs.H264
		case VideoH265:
			return mediacommonSupportedCodecs.H265
		case VideoMPEG1, VideoMPEG2:
			return mediacommonSupportedCodecs.MPEG1
		case VideoMPEG4:
			return mediacommonSupportedCodecs.MPEG4
		}
	}

	// Try audio codecs
	if audio, ok := ParseAudio(codecName); ok {
		switch audio {
		case AudioAAC:
			return mediacommonSupportedCodecs.AAC
		case AudioAC3:
			return mediacommonSupportedCodecs.AC3
		case AudioEAC3:
			return mediacommonSupportedCodecs.EAC3
		case AudioMP3:
			return mediacommonSupportedCodecs.MP3
		case AudioOpus:
			return mediacommonSupportedCodecs.Opus
		}
	}

	return false
}
