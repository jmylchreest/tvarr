# Relay Package Code Review

This document analyzes the relay package code to identify what should be kept, what can be replaced with well-supported libraries, and what needs to be changed or removed.

## Refactoring Progress

| Task | Status | Date |
|------|--------|------|
| Replace `ts_demuxer.go` with mediacommon | ✅ Complete | 2025-01-06 |
| Replace `ts_muxer.go` with mediacommon | ✅ Complete | 2025-01-06 |
| Replace `cmaf_muxer.go` with fmp4 | ✅ Complete | 2025-01-06 |
| Add variant cleanup loop | ✅ Complete | 2025-01-06 |
| Update processors to use new muxers | ✅ Complete | 2025-01-11 |
| Fix HLS source + ES pipeline | ⏳ Pending | |

## Current Library Usage

### Already Using (Keep)
| Library | Purpose | Files Using It |
|---------|---------|----------------|
| `github.com/bluenviron/gohlslib/v2` | HLS client & muxer | `hls_collapser.go`, `hls_muxer.go` |
| `github.com/bluenviron/mediacommon/v2` | Codec utilities | `hls_collapser.go` |
| `github.com/asticode/go-astits` | MPEG-TS muxing (in HLSCollapser) | `hls_collapser.go` |

### Available but Not Used
| Library | Provides | Could Replace |
|---------|----------|---------------|
| `github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts` | Full MPEG-TS Reader/Writer | Our custom `ts_demuxer.go`, `ts_muxer.go` |
| `github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4` | fMP4 Init/Part marshal/unmarshal | Our custom `cmaf_muxer.go` |

## File-by-File Analysis

### ✅ KEEP - Core Architecture (Needs Updates)

| File | Purpose | Status |
|------|---------|--------|
| `shared_buffer.go` | Multi-variant ES buffer | Keep - core to architecture |
| `session.go` | Relay session management | Keep - but fix HLS+transcode issue |
| `manager.go` | Session lifecycle management | Keep |
| `processor.go` | Base processor interface | Keep |
| `processor_hls_ts.go` | HLS-TS output processor | Keep - but refactor to use mediacommon |
| `processor_hls_fmp4.go` | HLS-fMP4 output processor | Keep - but refactor to use mediacommon/fmp4 |
| `processor_dash.go` | DASH output processor | Keep - but refactor to use mediacommon/fmp4 |
| `processor_mpegts.go` | MPEG-TS stream processor | Keep - but refactor to use mediacommon |
| `ffmpeg_transcoder.go` | On-demand transcoding | Keep - but needs lifecycle fixes |
| `ingest.go` | Stream ingestion coordination | Keep |

### ✅ REPLACED - Custom Implementations with Libraries

| File | Current Purpose | Replaced With | Status |
|------|-----------------|---------------|--------|
| `ts_demuxer.go` | MPEG-TS demuxer (now ~390 lines) | `mediacommon/v2/pkg/formats/mpegts.Reader` | ✅ Done |
| `ts_muxer.go` | MPEG-TS muxer (now ~520 lines) | `mediacommon/v2/pkg/formats/mpegts.Writer` | ✅ Done |
| `cmaf_muxer.go` | fMP4/CMAF parser + FMP4Writer | `mediacommon/v2/pkg/formats/fmp4` | ✅ Done |
| `fmp4_adapter.go` | ES to fMP4 sample conversion | New adapter for FMP4Writer | ✅ Done |

**Benefits of replacement:**
- `mediacommon` is actively maintained (used by MediaMTX, gohlslib)
- Handles edge cases we likely miss (codec-specific quirks)
- Proper support for all codecs (H.264, H.265, AAC, AC-3, Opus, VP9, AV1)
- Better timestamp handling
- Fewer bugs to maintain

### ✅ KEEP - Supporting Infrastructure

| File | Purpose | Notes |
|------|---------|-------|
| `circuit_breaker.go` | Upstream failure handling | Keep |
| `connection_pool.go` | Connection limits | Keep |
| `fallback.go` | Fallback slate generation | Keep |
| `format_router.go` | Multi-format routing | Keep |
| `client_detector.go` + `default_client_detector.go` | Client capability detection | Keep |
| `delivery.go` | Smart delivery decisions | Keep |
| `routing_decision.go` | Route selection logic | Keep |
| `constants.go` | Shared constants | Keep |
| `types.go` | Type definitions | Keep |
| `segment.go` | Segment data structure | Keep |

### ✅ KEEP - Handlers (Already Using gohlslib)

| File | Purpose | Notes |
|------|---------|-------|
| `hls_collapser.go` | HLS→TS via gohlslib | Keep - already uses gohlslib+astits |
| `hls_muxer.go` | HLS output via gohlslib | Keep - already uses gohlslib |
| `hls_handler.go` | HLS HTTP serving | Keep |
| `hls_passthrough.go` | HLS passthrough | Keep |
| `hls_repackager.go` | HLS container change | Keep |
| `hls_demuxer.go` | HLS segment fetching | Keep - but needs integration with ES pipeline |
| `dash_handler.go` | DASH HTTP serving | Keep |
| `dash_passthrough.go` | DASH passthrough | Keep |
| `mpegts_handler.go` | MPEG-TS HTTP serving | Keep |
| `output_handler.go` | Handler interfaces | Keep |

### 📊 KEEP - Visualization & Testing

| File | Purpose |
|------|---------|
| `flow_builder.go` | Flow graph building |
| `flow_types.go` | Flow visualization types |
| `session_info.go` | Session info for UI |
| `profile_tester.go` | Profile testing |
| `command_preview.go` | FFmpeg command preview |
| `*_test.go` | Tests |

## Recommended Refactoring Plan

### Phase 1: Replace Custom MPEG-TS with mediacommon ✅ COMPLETE

The custom MPEG-TS demuxer and muxer have been replaced with wrappers around mediacommon's `mpegts.Reader` and `mpegts.Writer`.

**New TSDemuxer Features:**
- Uses `mpegts.Reader` for robust MPEG-TS parsing
- Supports H.264, H.265, AAC, AC-3, MP3, and Opus codecs
- Automatic track detection and callback setup
- Pipe-based architecture for incremental writes
- `TSDemuxerFromReader()` for direct io.Reader consumption
- Context-aware initialization with `WaitInitialized()`

**New TSMuxer Features:**
- Uses `mpegts.Writer` for standards-compliant output
- Automatic codec detection and track configuration
- Supports Annex B and raw NAL unit input
- ADTS frame extraction for AAC audio
- `TSMuxerWithTracks()` for explicit track configuration
- Backward-compatible API with `SetVideoStreamType()`/`SetAudioStreamType()`

**Old implementation (replaced):**
```go
// ~500 lines of custom PES/PAT/PMT parsing
type TSDemuxer struct {
    // Manual TS packet parsing
    // Manual PES assembly
    // Manual NAL unit extraction
}
```

**Replace with:**
```go
import "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"

type TSDemuxer struct {
    reader *mpegts.Reader
    buffer *SharedESBuffer
}

func (d *TSDemuxer) Run(ctx context.Context, r io.Reader) error {
    d.reader = &mpegts.Reader{R: r}
    if err := d.reader.Initialize(); err != nil {
        return err
    }
    
    for _, track := range d.reader.Tracks() {
        switch track.Codec.(type) {
        case *mpegts.CodecH264:
            d.reader.OnDataH264(track, func(pts, dts int64, au [][]byte) error {
                // Write to SharedESBuffer
                return nil
            })
        case *mpegts.CodecMPEG4Audio:
            d.reader.OnDataMPEG4Audio(track, func(pts int64, aus [][]byte) error {
                // Write to SharedESBuffer  
                return nil
            })
        }
    }
    
    for {
        if err := d.reader.Read(); err != nil {
            return err
        }
    }
}
```

**Current (ts_muxer.go):**
```go
// ~400 lines of custom PAT/PMT/PES generation
type TSMuxer struct {
    // Manual CRC32 calculation
    // Manual PES header building
    // Manual TS packet assembly
}
```

**Replace with:**
```go
import "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"

type TSMuxer struct {
    writer *mpegts.Writer
}

func NewTSMuxer(w io.Writer, videoCodec, audioCodec string) *TSMuxer {
    tracks := []*mpegts.Track{
        {PID: 256, Codec: &mpegts.CodecH264{}},
        {PID: 257, Codec: &mpegts.CodecMPEG4Audio{Config: ...}},
    }
    
    writer := &mpegts.Writer{W: w, Tracks: tracks}
    writer.Initialize()
    
    return &TSMuxer{writer: writer}
}

func (m *TSMuxer) WriteH264(pts, dts int64, au [][]byte) error {
    return m.writer.WriteH264(m.videoTrack, pts, dts, au)
}
```

### Phase 2: Replace Custom fMP4/CMAF with mediacommon ✅ COMPLETE

The `cmaf_muxer.go` has been updated to use mediacommon's fMP4 package for parsing and writing.

**New CMAFMuxer Features:**
- Uses `fmp4.Init.Unmarshal()` for parsing init segments
- Falls back to manual parsing if mediacommon fails (for compatibility)
- Maintains the same API for reading fragments and init segments

**New FMP4Writer:**
- Uses `fmp4.Init` and `fmp4.Part` for generating fMP4 content
- Supports H.264, H.265, AAC, and other codecs via `mp4.Codec` types
- Proper sequence numbering and track management
- `GenerateInit()` - creates initialization segment with track configuration
- `GeneratePart()` - creates media parts with samples

```go
// Example usage of new FMP4Writer:
writer := NewFMP4Writer()
writer.SetH264Params(sps, pps)
writer.SetAACConfig(&mpeg4audio.Config{...})

initData, err := writer.GenerateInit(hasVideo, hasAudio, 90000, 48000)
partData, err := writer.GeneratePart(videoSamples, audioSamples, videoBaseTime, audioBaseTime)
```

### Phase 3: Add Variant Cleanup Loop ✅ COMPLETE

Added automatic cleanup of unused transcoded variants in `session.go`:

**Constants:**
- `VariantCleanupInterval = 30 * time.Second` - cleanup check frequency
- `VariantIdleTimeout = 60 * time.Second` - how long before cleanup

**New Functions in session.go:**
- `runVariantCleanupLoop()` - background goroutine checking for unused variants
- `cleanupUnusedVariants()` - removes idle variants and stops their transcoders
- `stopTranscodersForVariants()` - stops FFmpeg transcoders for removed variants
- `cleanupIdleTranscoders()` - stops transcoders that have been idle too long

This prevents memory and process leaks when clients stop requesting certain codec variants.

### Phase 4: Update Processors to Use New Muxers ✅ COMPLETE

Updated `processor_hls_fmp4.go` and `processor_dash.go` to use the mediacommon-based `FMP4Writer` instead of the custom `FMP4Muxer`.

**Changes Made:**
- Removed ~800 lines of custom fMP4 box-writing code (writeFtyp, writeMoov, writeMoof, etc.)
- Added `fmp4_adapter.go` with:
  - `ESSampleAdapter` - converts ES samples to fmp4.Sample format
  - `ExtractVideoCodecParams()` - extracts SPS/PPS from H.264 or VPS/SPS/PPS from H.265
  - `ExtractAudioCodecParams()` - extracts AAC config from ADTS headers
  - `ConvertESSamplesToFMP4Video/Audio()` - converts samples with duration calculation
- Updated `HLSfMP4Processor` to use `FMP4Writer` and `ESSampleAdapter`
- Updated `DASHProcessor` to use `FMP4Writer` and `ESSampleAdapter`

**Benefits:**
- Uses battle-tested mediacommon library for fMP4 generation
- Proper support for H.264, H.265, and AAC codecs
- Automatic codec parameter extraction from stream
- Reduced code complexity and maintenance burden

### Phase 5: Fix HLS Source + ES Pipeline Issue

The ES pipeline currently assumes raw MPEG-TS input. When source is HLS and transcoding is needed, we must use `HLSDemuxer` to fetch segments.

**session.go changes needed:**
```go
func (s *RelaySession) runESPipeline() error {
    // ... setup SharedESBuffer ...
    
    // Determine ingest method based on source format
    switch s.Classification.SourceFormat {
    case SourceFormatHLS:
        // Use HLSDemuxer for HLS sources
        hlsDemuxer := NewHLSDemuxer(inputURL, s.esBuffer, HLSDemuxerConfig{...})
        return hlsDemuxer.Run(s.ctx)
        
    case SourceFormatDASH:
        // Use DASH demuxer for DASH sources (TODO)
        return s.runDASHIngest()
        
    default:
        // Raw MPEG-TS - use direct demuxer
        return s.runRawTSIngest(inputURL)
    }
}
```



## Summary: Files to Change

### Delete (Replace with library)
- [x] `ts_demuxer.go` - ✅ Replaced with wrapper around mediacommon/mpegts.Reader
- [x] `ts_muxer.go` - ✅ Replaced with wrapper around mediacommon/mpegts.Writer  
- [x] `cmaf_muxer.go` - ✅ Updated with mediacommon/fmp4 (FMP4Writer added)

### Significant Refactoring Needed
- [ ] `session.go` - Fix HLS source + transcoding
- [ ] `ffmpeg_transcoder.go` - Better lifecycle management
- [x] `processor_hls_ts.go` - Use mediacommon for muxing (already using TSMuxer)
- [x] `processor_hls_fmp4.go` - Use mediacommon/fmp4 for segment creation (now using FMP4Writer)
- [x] `processor_dash.go` - Use mediacommon/fmp4 for segment creation (now using FMP4Writer)
- [x] `processor_mpegts.go` - Use mediacommon for muxing (already using TSMuxer)
- [ ] `hls_demuxer.go` - Integrate with ES pipeline properly

### Minor Changes
- [ ] `shared_buffer.go` - May need adjustments for new sample format from mediacommon
- [ ] `ingest.go` - Coordinate with new demuxer approach

## Dependencies to Add

Already have in go.mod:
```
github.com/bluenviron/mediacommon/v2 v2.5.3
```

Just need to import the additional packages:
```go
import (
    "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
    "github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
)
```

## Estimated Effort

| Task | Complexity | Estimate | Status |
|------|------------|----------|--------|
| Replace ts_demuxer with mediacommon | Medium | 2-3 hours | ✅ Done |
| Replace ts_muxer with mediacommon | Medium | 2-3 hours | ✅ Done |
| Replace cmaf_muxer with fmp4 | Medium | 2-3 hours | ✅ Done |
| Add variant cleanup loop | Low | 1-2 hours | ✅ Done |
| Update processors to use new muxers | Medium | 3-4 hours | ✅ Done |
| Fix HLS source + ES pipeline | High | 4-6 hours | Pending |
| Testing & debugging | High | 4-8 hours | Ongoing |

**Remaining: ~8-14 hours of work**

## Priority Order

1. ~~**Replace ts_demuxer/ts_muxer**~~ - ✅ Complete
2. ~~**Replace cmaf_muxer with fmp4**~~ - ✅ Complete
3. ~~**Add variant cleanup loop**~~ - ✅ Complete
4. ~~**Update processors to use new muxers**~~ - ✅ Complete
   - HLS-TS & MPEG-TS processors already used mediacommon-based TSMuxer
   - HLS-fMP4 & DASH processors now use mediacommon-based FMP4Writer
   - Added ESSampleAdapter for converting ES samples to fMP4 format
   - Removed ~800 lines of custom fMP4 box-writing code from processor_hls_fmp4.go
5. **Fix HLS source + ES pipeline** - This is blocking playback when HLS source needs transcoding