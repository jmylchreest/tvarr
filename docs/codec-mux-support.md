# Codec & Container Muxer/Demuxer Support

This document describes the codec and container format support in tvarr's relay system, including the data flows through demuxers, muxers, and transcoders.

## Buffer Format (Central Storage)

All samples in `SharedESBuffer` are stored in a normalized format:

| Codec Type | Buffer Format | Description |
|------------|---------------|-------------|
| **H.264** | Annex B | NAL units with 4-byte start codes (`00 00 00 01`) |
| **H.265** | Annex B | NAL units with 4-byte start codes (`00 00 00 01`) |
| **AV1** | Raw OBU | Open Bitstream Unit sequences |
| **VP9** | Raw frames | Superframe format |
| **AAC** | Raw frames | Without ADTS headers |
| **AC3/EAC3** | Raw frames | Dolby Digital frames |
| **MP3** | Raw frames | MPEG-1 Audio Layer III |
| **Opus** | Raw packets | Opus codec packets |

## Container Compatibility Matrix

| Codec | MPEG-TS | HLS-TS | HLS-fMP4 | DASH |
|-------|:-------:|:------:|:--------:|:----:|
| H.264 | Yes | Yes | Yes | Yes |
| H.265 | Yes | Yes | Yes | Yes |
| AV1 | No | No | Yes | Yes |
| VP9 | No | No | Yes | Yes |
| AAC | Yes | Yes | Yes | Yes |
| AC3 | Yes | Yes | Yes | Yes |
| EAC3 | Yes | Yes | Yes | Yes |
| MP3 | Yes | Yes | Yes | Yes |
| Opus | No | No | Yes | Yes |

**Note**: AV1, VP9, and Opus require fMP4 containers (not compatible with MPEG-TS by specification).

## Source Ingestion Flows

### Supported Source Formats

| Source Format | Demuxer | Video Codecs | Audio Codecs |
|---------------|---------|--------------|--------------|
| **MPEG-TS** | `relay/ts_demuxer.go` | H.264, H.265 | AAC, AC3, EAC3, MP3 |
| **HLS-TS** | `relay/hls_demuxer.go` | H.264, H.265 | AAC, AC3, EAC3, MP3 |
| **HLS-fMP4** | `relay/hls_demuxer.go` | H.264, H.265, AV1, VP9 | AAC, AC3, EAC3, Opus |
| **DASH** | (future) | H.264, H.265, AV1, VP9 | AAC, AC3, EAC3, Opus |

### Demuxer Actions

**MPEG-TS Demuxer** (`relay/ts_demuxer.go`):
- Parses PAT/PMT tables to discover tracks
- Extracts video NAL units in Annex B format
- Extracts raw audio frames
- Detects codec type from stream descriptors

**HLS Demuxer** (`relay/hls_demuxer.go`):
- Downloads M3U8 playlists and segments
- Delegates to TS or fMP4 demuxer based on segment type
- Handles playlist refresh for live streams

**fMP4 Demuxer** (source side):
- Parses moov box for track definitions
- Extracts codec parameters from codec-specific boxes
- Converts length-prefixed NALs to Annex B for buffer storage

## Output Processor Flows

### Supported Output Formats

| Output Format | Processor | Supported Codecs |
|---------------|-----------|------------------|
| **MPEG-TS** | `processor_mpegts.go` | H.264, H.265, AAC, AC3, EAC3, MP3 |
| **HLS-TS** | `processor_hls_ts.go` | H.264, H.265, AAC, AC3, EAC3, MP3 |
| **HLS-fMP4** | `processor_hls_fmp4.go` | H.264, H.265, AV1, VP9, AAC, AC3, EAC3, Opus |
| **DASH** | `processor_dash.go` | H.264, H.265, AV1, VP9, AAC, AC3, EAC3, Opus |

### Processor Actions

**MPEG-TS Processor**:
- Reads Annex B NALs from buffer
- Prepends SPS/PPS (H.264) or VPS/SPS/PPS (H.265) to keyframes
- Outputs continuous MPEG-TS stream

**HLS-TS Processor**:
- Accumulates samples into segments (~6 second target)
- Each segment starts with a keyframe
- Maintains continuity counters across segments
- Outputs M3U8 playlist + TS segments

**HLS-fMP4 Processor**:
- Converts Annex B to length-prefixed format
- Generates init segment (moov box) with codec parameters
- Outputs EXT-X-VERSION:7 playlist + fMP4 segments

**DASH Processor**:
- Similar to HLS-fMP4 but with MPD manifest
- Outputs DASH-compliant fMP4 segments

## Transcoding Flows

### FFmpeg Input Muxing (ES to FFmpeg stdin)

| Target Codec | Input Muxer | Format Sent to FFmpeg |
|--------------|-------------|----------------------|
| H.264, H.265 | `daemon/ts_muxer.go` | MPEG-TS with Annex B NALs |
| AV1, VP9 | `daemon/fmp4_muxer.go` | fMP4 with codec-native format |

**TS Muxer Actions** (`daemon/ts_muxer.go`):
1. Receives Annex B NALs from buffer
2. Extracts SPS/PPS (H.264) or VPS/SPS/PPS (H.265) from keyframes
3. Prepends parameters to every keyframe
4. Outputs MPEG-TS to FFmpeg stdin

**fMP4 Muxer Actions** (`daemon/fmp4_muxer.go`):
1. Receives Annex B NALs or raw OBUs
2. Extracts codec parameters and builds moov box
3. Outputs fMP4 fragments to FFmpeg stdin

### FFmpeg Output Demuxing (FFmpeg stdout to ES)

| Output Codec | FFmpeg Format | Demuxer | Conversion |
|--------------|---------------|---------|------------|
| H.264 | MPEG-TS | `daemon/ts_demuxer.go` | Extract NALs (already Annex B) |
| H.265 | fMP4 | `daemon/fmp4_demuxer.go` | Convert hvc1 to Annex B, prepend VPS/SPS/PPS |
| AV1 | fMP4 | `daemon/fmp4_demuxer.go` | Extract OBUs as-is |
| VP9 | fMP4 | `daemon/fmp4_demuxer.go` | Extract frames as-is |

**fMP4 Demuxer H.265 Flow** (`daemon/fmp4_demuxer.go`):
1. Parse moov box - extract VPS/SPS/PPS from hvc1 codec config
2. Parse moof+mdat - extract samples (length-prefixed NALs)
3. Convert to Annex B: replace length prefixes with start codes
4. Prepend VPS/SPS/PPS to keyframes before sending to buffer

## Codec Parameter Handling

### Video Parameter Sets

| Codec | Parameters | NAL Types | Storage |
|-------|------------|-----------|---------|
| **H.264** | SPS, PPS | 7, 8 | Extracted from keyframes |
| **H.265** | VPS, SPS, PPS | 32, 33, 34 | Extracted from keyframes or hvc1 box |
| **AV1** | Sequence Header | OBU type 1 | Extracted from keyframes |
| **VP9** | Frame Header | In frame | Self-contained |

### Parameter Prepending

The `VideoParamHelper` (`relay/video_params.go`) ensures codec parameters are present:

- `ExtractParams()` - Pulls SPS/PPS/VPS from NAL stream
- `PrependParamsToKeyframeAnnexB()` - Adds missing params to keyframes
- `ReorderNALUnits()` - Fixes parameter order (some sources send SEI before SPS/PPS)

### Audio Initialization Data

| Codec | Init Data | Source |
|-------|-----------|--------|
| **AAC** | AudioSpecificConfig | MPEG-TS codec info or esds box |
| **AC3/EAC3** | Sample rate, channels | Frame headers |
| **Opus** | Channel count, sample rate | Opus header |
| **MP3** | Sample rate, channels | Frame headers |

## Format Conversions

| Conversion | Location | Description |
|------------|----------|-------------|
| Annex B to Length-prefixed | `fmp4_adapter.go` | Replace `00 00 00 01` with 4-byte length |
| Length-prefixed to Annex B | `fmp4_demuxer.go` | Replace 4-byte length with `00 00 00 01` |
| hvc1 config to Annex B | `fmp4_demuxer.go` | Extract VPS/SPS/PPS from codec box |
| Missing params prepend | `video_params.go` | Add SPS/PPS/VPS before IDR NALs |

## Example Flow: H.264 Source to H.265 Client

```
Source (MPEG-TS with H.264/AAC)
    |
    v
relay/ts_demuxer.go
    - Extract Annex B H.264 NALs
    - Extract raw AAC frames
    |
    v
SharedESBuffer [h264/aac variant]
    |
    v
es_transcoder.go
    - Read from h264/aac variant
    - Write to h265/aac variant
    |
    v
daemon/ts_muxer.go
    - Mux H.264 Annex B to MPEG-TS
    - Prepend SPS/PPS to keyframes
    |
    v
FFmpeg (transcode H.264 to H.265)
    |
    v
daemon/fmp4_demuxer.go
    - Demux fMP4 H.265 output
    - Extract VPS/SPS/PPS from hvc1 config
    - Convert to Annex B format
    - Prepend VPS/SPS/PPS to keyframes
    |
    v
SharedESBuffer [h265/aac variant]
    |
    v
processor_mpegts.go
    - Prepend VPS/SPS/PPS if missing
    - Mux to MPEG-TS
    |
    v
Client (MPEG-TS stream with H.265/AAC)
```

## Example Flow: H.264 Source to AV1/Opus HLS-fMP4 Client

```
Source (MPEG-TS with H.264/AAC)
    |
    v
relay/ts_demuxer.go
    - Extract Annex B H.264 NALs
    - Extract raw AAC frames
    |
    v
SharedESBuffer [h264/aac variant]
    |
    v
es_transcoder.go
    - Read from h264/aac variant
    - Write to av1/opus variant
    |
    v
daemon/ts_muxer.go
    - Mux H.264 Annex B to MPEG-TS
    |
    v
FFmpeg (transcode H.264 to AV1, AAC to Opus)
    |
    v
daemon/fmp4_demuxer.go
    - Demux fMP4 AV1+Opus output
    - Extract OBUs and Opus packets
    |
    v
SharedESBuffer [av1/opus variant]
    |
    v
processor_hls_fmp4.go
    - Convert to length-prefixed format
    - Build init segment with av1C box
    - Generate fMP4 segments
    |
    v
Client (HLS-fMP4 playlist + segments with AV1/Opus)
```

## Key Implementation Files

| Component | File Path |
|-----------|-----------|
| Shared Buffer | `internal/relay/shared_buffer.go` |
| Ingest Pipeline | `internal/relay/ingest.go` |
| TS Demuxer (relay) | `internal/relay/ts_demuxer.go` |
| HLS Demuxer | `internal/relay/hls_demuxer.go` |
| MPEG-TS Processor | `internal/relay/processor_mpegts.go` |
| HLS-TS Processor | `internal/relay/processor_hls_ts.go` |
| HLS-fMP4 Processor | `internal/relay/processor_hls_fmp4.go` |
| DASH Processor | `internal/relay/processor_dash.go` |
| ES Transcoder | `internal/relay/es_transcoder.go` |
| TS Demuxer (daemon) | `internal/daemon/ts_demuxer.go` |
| TS Muxer (daemon) | `internal/daemon/ts_muxer.go` |
| fMP4 Demuxer (daemon) | `internal/daemon/fmp4_demuxer.go` |
| fMP4 Muxer (daemon) | `internal/daemon/fmp4_muxer.go` |
| Video Params | `internal/relay/video_params.go` |
| fMP4 Adapter | `internal/relay/fmp4_adapter.go` |
