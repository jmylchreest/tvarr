// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"context"
	"fmt"
	"log/slog"
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
// If the provider implements SegmentWaiter and has no segments, it will wait
// up to 15 seconds for the first segment before returning.
func (d *DASHHandler) ServePlaylist(w http.ResponseWriter, baseURL string) error {
	return d.ServePlaylistWithContext(context.Background(), w, baseURL)
}

// ServePlaylistWithContext generates and serves the DASH MPD manifest with context support.
// If the provider implements SegmentWaiter and has no segments, it will wait
// up to 15 seconds for the first segment before returning.
// For CMAF mode, it also waits for the init segment to be ready.
func (d *DASHHandler) ServePlaylistWithContext(ctx context.Context, w http.ResponseWriter, baseURL string) error {
	// Record playlist activity - this is the heartbeat that indicates clients are watching
	if recorder, ok := d.provider.(PlaylistActivityRecorder); ok {
		recorder.RecordPlaylistRequest()
	}

	// Check if provider supports waiting for segments
	// For DASH, we need at least 2 segments before serving the manifest
	// because suggestedPresentationDelay is 2x segment duration (client expects to be 2 segments behind live edge)
	const minSegmentsForDASH = 2
	if waiter, ok := d.provider.(SegmentWaiter); ok {
		if waiter.SegmentCount() < minSegmentsForDASH {
			// Wait for at least 2 segments (timeout matching HTTP WriteTimeout)
			waitCtx, cancel := context.WithTimeout(ctx, SegmentWaitTimeout)
			defer cancel()

			if err := waiter.WaitForSegments(waitCtx, minSegmentsForDASH); err != nil {
				http.Error(w, "No segments available yet, please retry", http.StatusServiceUnavailable)
				return fmt.Errorf("waiting for segments: %w", err)
			}
		}
	}

	// For CMAF mode, also wait for init segment to be ready
	if fmp4Provider, ok := d.provider.(FMP4SegmentProvider); ok {
		if fmp4Provider.IsFMP4Mode() && !fmp4Provider.HasInitSegment() {
			// Wait for init segment with polling (timeout matching HTTP WriteTimeout)
			waitCtx, cancel := context.WithTimeout(ctx, SegmentWaitTimeout)
			defer cancel()

			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()

			for !fmp4Provider.HasInitSegment() {
				select {
				case <-waitCtx.Done():
					http.Error(w, "Init segment not available yet, please retry", http.StatusServiceUnavailable)
					return fmt.Errorf("waiting for init segment: %w", waitCtx.Err())
				case <-ticker.C:
					// Check again
				}
			}
		}
	}

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
	return d.ServeSegmentFiltered(w, sequence, "")
}

// ServeSegmentFiltered serves a media segment filtered to a specific track type.
// trackType should be "video", "audio", or empty for unfiltered segments.
func (d *DASHHandler) ServeSegmentFiltered(w http.ResponseWriter, sequence uint64, trackType string) error {
	seg, err := d.provider.GetSegment(sequence)
	if err != nil {
		if err == ErrSegmentNotFound {
			http.Error(w, "segment not found", http.StatusNotFound)
			return err
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return err
	}

	data := seg.Data

	// Filter segment if track type is specified and we have an FMP4 provider
	if trackType != "" && seg.IsFMP4() {
		if fmp4Provider, ok := d.provider.(FMP4SegmentProvider); ok {
			filteredData, err := filterSegmentByTrack(data, trackType, fmp4Provider)
			if err != nil {
				slog.Warn("Failed to filter segment by track, serving unfiltered",
					slog.String("track_type", trackType),
					slog.Int("sequence", int(sequence)),
					slog.String("error", err.Error()))
				// Fall back to unfiltered segment
			} else {
				data = filteredData
			}
		}
	}

	// Use appropriate content type based on segment format
	contentType := ContentTypeDASHSegment
	if seg.IsFMP4() {
		contentType = ContentTypeFMP4Segment // video/mp4 for CMAF
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Header().Set("Cache-Control", "max-age=86400") // Segments can be cached
	w.WriteHeader(http.StatusOK)

	_, err = w.Write(data)
	return err
}

// ServeInitSegment serves the initialization segment.
// streamType should be "video" or "audio" for track-specific init segments.
// For CMAF streams with unified init segment, streamType can be empty or "cmaf".
func (d *DASHHandler) ServeInitSegment(w http.ResponseWriter, streamType string) error {
	// Try to get init segment from FMP4SegmentProvider (CMAF mode)
	if fmp4Provider, ok := d.provider.(FMP4SegmentProvider); ok {
		if fmp4Provider.IsFMP4Mode() && fmp4Provider.HasInitSegment() {
			// For DASH with separate video/audio AdaptationSets, serve filtered init segments
			// This is required because FFmpeg's DASH demuxer assigns one stream_index per
			// representation, and muxed segments cause issues when both tracks are present
			if streamType == "video" || streamType == "audio" {
				initData, err := fmp4Provider.GetFilteredInitSegment(streamType)
				if err != nil {
					slog.Warn("Failed to get filtered init segment, falling back to full init",
						slog.String("track_type", streamType),
						slog.String("error", err.Error()))
					// Fall back to full init segment
					initSeg := fmp4Provider.GetInitSegment()
					if initSeg != nil && !initSeg.IsEmpty() {
						initData = initSeg.Data
					} else {
						slog.Warn("ServeInitSegment: fallback init segment is nil or empty")
					}
				}
				if len(initData) > 0 {
					w.Header().Set("Content-Type", ContentTypeFMP4Init)
					w.Header().Set("Content-Length", fmt.Sprintf("%d", len(initData)))
					w.Header().Set("Cache-Control", "max-age=86400")
					w.WriteHeader(http.StatusOK)
					_, err := w.Write(initData)
					return err
				}
				slog.Warn("ServeInitSegment: no init data after filtering and fallback")
			}

			// Serve full muxed init segment for unfiltered requests
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

	// Debug: Log manifest generation details
	slog.Debug("DASH manifest generation",
		slog.Int("segment_count", len(segments)),
		slog.Int("target_duration", targetDuration),
		slog.String("base_url", baseURL))

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

	// Check FMP4SegmentProvider for CMAF-style init segment
	// In CMAF mode, a single init segment contains both video and audio tracks (muxed)
	isCMAFMode := false
	if fmp4Provider, ok := d.provider.(FMP4SegmentProvider); ok {
		if fmp4Provider.IsFMP4Mode() && fmp4Provider.HasInitSegment() {
			hasVideoInit = true
			hasAudioInit = true
			isCMAFMode = true
		}
	}

	// Use defaults if metadata not set
	// This can happen before init segment is parsed or if stream setup failed
	usingDefaults := false
	if videoWidth == 0 {
		videoWidth = DefaultVideoWidth
		usingDefaults = true
	}
	if videoHeight == 0 {
		videoHeight = DefaultVideoHeight
		usingDefaults = true
	}
	if videoBandwidth == 0 {
		videoBandwidth = DefaultVideoBandwidth
		usingDefaults = true
	}
	if audioChannels == 0 {
		audioChannels = DefaultAudioChannels
	}
	if audioBandwidth == 0 {
		audioBandwidth = DefaultAudioBandwidth
		usingDefaults = true
	}
	if usingDefaults {
		slog.Warn("DASH manifest using default stream metadata - init segment may not be parsed yet")
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
	}

	// Use stream start time for availabilityStartTime (must be constant throughout stream)
	// This is critical - if availabilityStartTime changes, players get confused
	if fmp4Provider, ok := d.provider.(FMP4SegmentProvider); ok {
		availabilityStartTime = fmp4Provider.GetStreamStartTime()
	}
	if availabilityStartTime.IsZero() {
		// Fallback to first segment timestamp if stream start time not set
		if segmentCount > 0 {
			availabilityStartTime = segments[0].Timestamp
		} else {
			availabilityStartTime = publishTime
		}
	}

	// Build manifest
	var sb strings.Builder

	// XML header and MPD root
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString("\n")
	// Calculate timeShiftBufferDepth based on how many segments we keep
	// This tells clients how far back in time they can seek
	// Use PlaylistSegments * targetDuration as a conservative estimate
	timeShiftBuffer := segmentCount * targetDuration
	if timeShiftBuffer < targetDuration*3 {
		timeShiftBuffer = targetDuration * 3 // Minimum 3 segments worth
	}

	sb.WriteString(fmt.Sprintf(`<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" `+
		`xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" `+
		`xsi:schemaLocation="urn:mpeg:dash:schema:mpd:2011 DASH-MPD.xsd" `+
		`type="dynamic" `+
		`profiles="urn:mpeg:dash:profile:isoff-live:2011" `+
		`availabilityStartTime="%s" `+
		`publishTime="%s" `+
		`minimumUpdatePeriod="PT%dS" `+
		`minBufferTime="PT%dS" `+
		`suggestedPresentationDelay="PT%dS" `+
		`timeShiftBufferDepth="PT%dS">`,
		availabilityStartTime.UTC().Format(time.RFC3339),
		publishTime.UTC().Format(time.RFC3339),
		targetDuration,   // minimumUpdatePeriod
		targetDuration*2, // minBufferTime
		targetDuration*3, // suggestedPresentationDelay (3 segments behind live)
		timeShiftBuffer,  // timeShiftBufferDepth
	))
	sb.WriteString("\n")

	// Period
	sb.WriteString(`  <Period id="0" start="PT0S">`)
	sb.WriteString("\n")

	if isCMAFMode {
		// CMAF mode: separate AdaptationSets for video and audio
		// Both point to the same muxed segments, but FFmpeg's DASH demuxer
		// assigns one stream_index per Representation, so we need separate
		// AdaptationSets to get proper stream association.
		// The demuxer will open the same segments for both and extract the
		// appropriate track based on the representation's content type.

		// Helper to build SegmentTimeline
		// Calculate presentation time offset from stream start (availabilityStartTime)
		buildSegmentTimeline := func() string {
			var timeline strings.Builder
			timeline.WriteString(`        <SegmentTimeline>`)
			timeline.WriteString("\n")
			for i, seg := range segments {
				durationTicks := int64(seg.Duration * 90000)
				if i == 0 {
					// Calculate presentation time relative to availabilityStartTime
					// This ensures the timeline is consistent as segments rotate
					offsetSeconds := seg.Timestamp.Sub(availabilityStartTime).Seconds()
					if offsetSeconds < 0 {
						offsetSeconds = 0 // Safety: don't go negative
					}
					startTicks := int64(offsetSeconds * 90000)
					timeline.WriteString(fmt.Sprintf(`          <S t="%d" d="%d"/>`, startTicks, durationTicks))
				} else {
					timeline.WriteString(fmt.Sprintf(`          <S d="%d"/>`, durationTicks))
				}
				timeline.WriteString("\n")
			}
			timeline.WriteString(`        </SegmentTimeline>`)
			timeline.WriteString("\n")
			return timeline.String()
		}

		// Video AdaptationSet
		sb.WriteString(fmt.Sprintf(`    <AdaptationSet id="0" contentType="video" mimeType="video/mp4" codecs="avc1.64001f" `+
			`width="%d" height="%d" frameRate="30" segmentAlignment="true" startWithSAP="1">`,
			videoWidth, videoHeight,
		))
		sb.WriteString("\n")

		// Video SegmentTemplate - use track=video for video-only init and segments
		if hasVideoInit {
			sb.WriteString(fmt.Sprintf(`      <SegmentTemplate `+
				`initialization="%s?%s=%s&amp;%s=1&amp;track=video" `+
				`media="%s?%s=%s&amp;%s=$Number$&amp;track=video" `+
				`timescale="90000" `+
				`startNumber="%d">`,
				baseURL, QueryParamFormat, FormatValueDASH, QueryParamInit,
				baseURL, QueryParamFormat, FormatValueDASH, QueryParamSegment,
				firstSegment,
			))
		} else {
			sb.WriteString(fmt.Sprintf(`      <SegmentTemplate `+
				`media="%s?%s=%s&amp;%s=$Number$" `+
				`timescale="90000" `+
				`startNumber="%d">`,
				baseURL, QueryParamFormat, FormatValueDASH, QueryParamSegment,
				firstSegment,
			))
		}
		sb.WriteString("\n")
		sb.WriteString(buildSegmentTimeline())
		sb.WriteString(`      </SegmentTemplate>`)
		sb.WriteString("\n")

		// Video Representation
		sb.WriteString(fmt.Sprintf(`      <Representation id="video" bandwidth="%d"/>`, videoBandwidth))
		sb.WriteString("\n")
		sb.WriteString(`    </AdaptationSet>`)
		sb.WriteString("\n")

		// Audio AdaptationSet - uses same muxed segments
		sb.WriteString(`    <AdaptationSet id="1" contentType="audio" mimeType="audio/mp4" codecs="mp4a.40.2" ` +
			`lang="und" segmentAlignment="true" startWithSAP="1">`)
		sb.WriteString("\n")

		// AudioChannelConfiguration
		sb.WriteString(fmt.Sprintf(`      <AudioChannelConfiguration schemeIdUri="urn:mpeg:dash:23003:3:audio_channel_configuration:2011" value="%d"/>`,
			audioChannels,
		))
		sb.WriteString("\n")

		// Audio SegmentTemplate - use track=audio for audio-only init and segments
		if hasAudioInit {
			sb.WriteString(fmt.Sprintf(`      <SegmentTemplate `+
				`initialization="%s?%s=%s&amp;%s=1&amp;track=audio" `+
				`media="%s?%s=%s&amp;%s=$Number$&amp;track=audio" `+
				`timescale="90000" `+
				`startNumber="%d">`,
				baseURL, QueryParamFormat, FormatValueDASH, QueryParamInit,
				baseURL, QueryParamFormat, FormatValueDASH, QueryParamSegment,
				firstSegment,
			))
		} else {
			sb.WriteString(fmt.Sprintf(`      <SegmentTemplate `+
				`media="%s?%s=%s&amp;%s=$Number$" `+
				`timescale="90000" `+
				`startNumber="%d">`,
				baseURL, QueryParamFormat, FormatValueDASH, QueryParamSegment,
				firstSegment,
			))
		}
		sb.WriteString("\n")
		sb.WriteString(buildSegmentTimeline())
		sb.WriteString(`      </SegmentTemplate>`)
		sb.WriteString("\n")

		// Audio Representation
		sb.WriteString(fmt.Sprintf(`      <Representation id="audio" bandwidth="%d"/>`, audioBandwidth))
		sb.WriteString("\n")
		sb.WriteString(`    </AdaptationSet>`)
		sb.WriteString("\n")
	} else {
		// Non-CMAF mode: separate video and audio AdaptationSets

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
	}

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
