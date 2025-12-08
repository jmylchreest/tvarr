// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DASHHandler handles DASH output.
// Implements the OutputHandler interface for serving DASH manifests and segments.
type DASHHandler struct {
	OutputHandlerBase

	mu           sync.RWMutex
	initVideoSeg []byte // Video initialization segment
	initAudioSeg []byte // Audio initialization segment

	// Stream metadata for manifest generation
	videoWidth     int
	videoHeight    int
	videoBandwidth int
	audioChannels  int
	audioBandwidth int
	publishTime    time.Time
}

// NewDASHHandler creates a DASH output handler with a SegmentProvider.
func NewDASHHandler(provider SegmentProvider) *DASHHandler {
	return &DASHHandler{
		OutputHandlerBase: NewOutputHandlerBase(provider),
		publishTime:       time.Now(),
	}
}

// Format returns the output format this handler serves.
func (d *DASHHandler) Format() string {
	return FormatValueDASH
}

// ContentType returns DASH manifest content type.
func (d *DASHHandler) ContentType() string {
	return ContentTypeDASHManifest
}

// SegmentContentType returns the Content-Type for DASH media segments.
func (d *DASHHandler) SegmentContentType() string {
	return ContentTypeDASHSegment
}

// SupportsStreaming returns false as DASH uses manifest-based delivery.
func (d *DASHHandler) SupportsStreaming() bool {
	return false
}

// SetInitSegments sets the initialization segments for video and audio.
func (d *DASHHandler) SetInitSegments(video, audio []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(video) > 0 {
		d.initVideoSeg = make([]byte, len(video))
		copy(d.initVideoSeg, video)
	}
	if len(audio) > 0 {
		d.initAudioSeg = make([]byte, len(audio))
		copy(d.initAudioSeg, audio)
	}
}

// SetStreamMetadata sets stream metadata for manifest generation.
func (d *DASHHandler) SetStreamMetadata(videoWidth, videoHeight, videoBandwidth, audioChannels, audioBandwidth int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.videoWidth = videoWidth
	d.videoHeight = videoHeight
	d.videoBandwidth = videoBandwidth
	d.audioChannels = audioChannels
	d.audioBandwidth = audioBandwidth
}

// ServePlaylist generates and serves the DASH MPD manifest.
func (d *DASHHandler) ServePlaylist(w http.ResponseWriter, baseURL string) error {
	manifest := d.GenerateManifest(baseURL)

	w.Header().Set("Content-Type", ContentTypeDASHManifest)
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.WriteHeader(http.StatusOK)

	_, err := w.Write([]byte(manifest))
	return err
}

// ServeSegment serves a media segment (.m4s).
func (d *DASHHandler) ServeSegment(w http.ResponseWriter, sequence uint64) error {
	seg, err := d.provider.GetSegment(sequence)
	if err != nil {
		if err == ErrSegmentNotFound {
			http.Error(w, "segment not found", http.StatusNotFound)
			return err
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return err
	}

	// Use appropriate content type based on segment format
	contentType := ContentTypeDASHSegment
	if seg.IsFMP4() {
		contentType = ContentTypeFMP4Segment // video/mp4 for CMAF
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(seg.Data)))
	w.Header().Set("Cache-Control", "max-age=86400") // Segments can be cached
	w.WriteHeader(http.StatusOK)

	_, err = w.Write(seg.Data)
	return err
}

// ServeInitSegment serves the initialization segment.
// streamType should be "v" for video or "a" for audio.
// For CMAF streams with unified init segment, streamType can be empty or "cmaf".
func (d *DASHHandler) ServeInitSegment(w http.ResponseWriter, streamType string) error {
	// First, try to get init segment from FMP4SegmentProvider (CMAF mode)
	// In CMAF mode, video and audio share a single init segment
	if fmp4Provider, ok := d.provider.(FMP4SegmentProvider); ok {
		if fmp4Provider.IsFMP4Mode() && fmp4Provider.HasInitSegment() {
			initSeg := fmp4Provider.GetInitSegment()
			if initSeg != nil && !initSeg.IsEmpty() {
				w.Header().Set("Content-Type", ContentTypeFMP4Init)
				w.Header().Set("Content-Length", fmt.Sprintf("%d", len(initSeg.Data)))
				w.Header().Set("Cache-Control", "max-age=86400")
				w.WriteHeader(http.StatusOK)

				_, err := w.Write(initSeg.Data)
				return err
			}
		}
	}

	// Fall back to legacy per-stream init segments
	d.mu.RLock()
	defer d.mu.RUnlock()

	var data []byte
	switch streamType {
	case "v", "video":
		data = d.initVideoSeg
	case "a", "audio":
		data = d.initAudioSeg
	case "", "cmaf":
		// For CMAF, try video first (which typically contains both tracks)
		if len(d.initVideoSeg) > 0 {
			data = d.initVideoSeg
		} else {
			data = d.initAudioSeg
		}
	default:
		http.Error(w, "invalid stream type", http.StatusBadRequest)
		return fmt.Errorf("invalid stream type: %s", streamType)
	}

	if len(data) == 0 {
		http.Error(w, "initialization segment not available", http.StatusNotFound)
		return fmt.Errorf("init segment not available for type: %s", streamType)
	}

	w.Header().Set("Content-Type", ContentTypeDASHInit)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Header().Set("Cache-Control", "max-age=86400") // Init segments can be cached
	w.WriteHeader(http.StatusOK)

	_, err := w.Write(data)
	return err
}

// ServeStream returns an error as DASH doesn't support continuous streaming.
func (d *DASHHandler) ServeStream(ctx context.Context, w http.ResponseWriter) error {
	return ErrUnsupportedOperation
}

// GenerateManifest creates a DASH MPD manifest from current segments.
// Implements DASH-IF compliant manifest with SegmentTemplate.
func (d *DASHHandler) GenerateManifest(baseURL string) string {
	segments := d.provider.GetSegmentInfos()
	targetDuration := d.provider.TargetDuration()

	d.mu.RLock()
	videoWidth := d.videoWidth
	videoHeight := d.videoHeight
	videoBandwidth := d.videoBandwidth
	audioChannels := d.audioChannels
	audioBandwidth := d.audioBandwidth
	publishTime := d.publishTime
	hasVideoInit := len(d.initVideoSeg) > 0
	hasAudioInit := len(d.initAudioSeg) > 0
	d.mu.RUnlock()

	// Use defaults if metadata not set
	if videoWidth == 0 {
		videoWidth = 1920
	}
	if videoHeight == 0 {
		videoHeight = 1080
	}
	if videoBandwidth == 0 {
		videoBandwidth = 5000000 // 5 Mbps default
	}
	if audioChannels == 0 {
		audioChannels = 2
	}
	if audioBandwidth == 0 {
		audioBandwidth = 128000 // 128 kbps default
	}

	// Ensure baseURL doesn't have trailing slash
	baseURL = strings.TrimSuffix(baseURL, "/")

	// Calculate timing
	var availabilityStartTime time.Time
	var firstSegment, lastSegment uint64
	segmentCount := len(segments)

	if segmentCount > 0 {
		firstSegment = segments[0].Sequence
		lastSegment = segments[segmentCount-1].Sequence
		availabilityStartTime = segments[0].Timestamp
	} else {
		availabilityStartTime = publishTime
	}

	// Build manifest
	var sb strings.Builder

	// XML header and MPD root
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf(`<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" `+
		`xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" `+
		`xsi:schemaLocation="urn:mpeg:dash:schema:mpd:2011 DASH-MPD.xsd" `+
		`type="dynamic" `+
		`profiles="urn:mpeg:dash:profile:isoff-live:2011" `+
		`availabilityStartTime="%s" `+
		`publishTime="%s" `+
		`minimumUpdatePeriod="PT%dS" `+
		`minBufferTime="PT%dS" `+
		`suggestedPresentationDelay="PT%dS">`,
		availabilityStartTime.UTC().Format(time.RFC3339),
		publishTime.UTC().Format(time.RFC3339),
		targetDuration,   // minimumUpdatePeriod
		targetDuration*2, // minBufferTime
		targetDuration*2, // suggestedPresentationDelay
	))
	sb.WriteString("\n")

	// Period
	sb.WriteString(`  <Period id="0" start="PT0S">`)
	sb.WriteString("\n")

	// Video AdaptationSet
	sb.WriteString(fmt.Sprintf(`    <AdaptationSet id="0" mimeType="video/mp4" codecs="avc1.64001f" `+
		`width="%d" height="%d" frameRate="30" segmentAlignment="true" startWithSAP="1">`,
		videoWidth, videoHeight,
	))
	sb.WriteString("\n")

	// Video SegmentTemplate
	if hasVideoInit {
		sb.WriteString(fmt.Sprintf(`      <SegmentTemplate `+
			`initialization="%s?%s=%s&amp;%s=v" `+
			`media="%s?%s=%s&amp;%s=$Number$" `+
			`timescale="1" `+
			`duration="%d" `+
			`startNumber="%d"/>`,
			baseURL, QueryParamFormat, FormatValueDASH, QueryParamInit,
			baseURL, QueryParamFormat, FormatValueDASH, QueryParamSegment,
			targetDuration,
			firstSegment,
		))
	} else {
		sb.WriteString(fmt.Sprintf(`      <SegmentTemplate `+
			`media="%s?%s=%s&amp;%s=$Number$" `+
			`timescale="1" `+
			`duration="%d" `+
			`startNumber="%d"/>`,
			baseURL, QueryParamFormat, FormatValueDASH, QueryParamSegment,
			targetDuration,
			firstSegment,
		))
	}
	sb.WriteString("\n")

	// Video Representation
	sb.WriteString(fmt.Sprintf(`      <Representation id="video" bandwidth="%d"/>`, videoBandwidth))
	sb.WriteString("\n")
	sb.WriteString(`    </AdaptationSet>`)
	sb.WriteString("\n")

	// Audio AdaptationSet
	sb.WriteString(fmt.Sprintf(`    <AdaptationSet id="1" mimeType="audio/mp4" codecs="mp4a.40.2" ` +
		`audioSamplingRate="48000" segmentAlignment="true" startWithSAP="1">`,
	))
	sb.WriteString("\n")

	// AudioChannelConfiguration
	sb.WriteString(fmt.Sprintf(`      <AudioChannelConfiguration schemeIdUri="urn:mpeg:dash:23003:3:audio_channel_configuration:2011" value="%d"/>`,
		audioChannels,
	))
	sb.WriteString("\n")

	// Audio SegmentTemplate
	if hasAudioInit {
		sb.WriteString(fmt.Sprintf(`      <SegmentTemplate `+
			`initialization="%s?%s=%s&amp;%s=a" `+
			`media="%s?%s=%s&amp;%s=$Number$" `+
			`timescale="1" `+
			`duration="%d" `+
			`startNumber="%d"/>`,
			baseURL, QueryParamFormat, FormatValueDASH, QueryParamInit,
			baseURL, QueryParamFormat, FormatValueDASH, QueryParamSegment,
			targetDuration,
			firstSegment,
		))
	} else {
		sb.WriteString(fmt.Sprintf(`      <SegmentTemplate `+
			`media="%s?%s=%s&amp;%s=$Number$" `+
			`timescale="1" `+
			`duration="%d" `+
			`startNumber="%d"/>`,
			baseURL, QueryParamFormat, FormatValueDASH, QueryParamSegment,
			targetDuration,
			firstSegment,
		))
	}
	sb.WriteString("\n")

	// Audio Representation
	sb.WriteString(fmt.Sprintf(`      <Representation id="audio" bandwidth="%d"/>`, audioBandwidth))
	sb.WriteString("\n")
	sb.WriteString(`    </AdaptationSet>`)
	sb.WriteString("\n")

	// Close Period and MPD
	sb.WriteString(`  </Period>`)
	sb.WriteString("\n")

	// UTCTiming for live sync (optional but recommended)
	sb.WriteString(`  <UTCTiming schemeIdUri="urn:mpeg:dash:utc:http-xsdate:2014" value="https://time.akamai.com/?iso"/>`)
	sb.WriteString("\n")

	sb.WriteString(`</MPD>`)
	sb.WriteString("\n")

	_ = lastSegment // Suppress unused warning (used for segment calculation)

	return sb.String()
}
