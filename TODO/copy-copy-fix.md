# Refactor 'copy' Codec Resolution to Buffer/Variant Level

## Problem

Currently, the `VariantCopy = "copy/copy"` special value flows through the entire system, with "copy" being resolved to actual codecs at various points (flow builder, session management, transcoder creation). This creates complexity and requires "copy" resolution logic in multiple places.

The ideal design: **Track actual codecs at the buffer/variant level from the start, and only use "copy" as an encoder hint in the FFmpeg command builder.**

## Current State

### Where "copy" is used:

1. **`internal/relay/shared_buffer.go:407`**
   ```go
   VariantCopy CodecVariant = "copy/copy" // Passthrough - use source codecs
   ```
   - Used as a special variant key for passthrough streams
   - Buffer nodes are keyed by this variant

2. **`internal/relay/session.go`** (multiple locations)
   - `VariantCopy` used for default processors (lines 567-598)
   - `DetermineTargetVariant()` returns `VariantCopy` when no transcoding needed
   - "copy" used as codec value when resolving (lines 824-849)

3. **`internal/relay/flow_builder.go`** (lines 96-350)
   - Multiple places resolve "copy" to actual source codec for display
   - Pattern: `if codec != "" && codec != "copy" { use actual } else { use source }`

4. **`internal/relay/transcoder.go`** (lines 433-449)
   - `mapVariantToEncoders()` maps "copy" to "copy" encoder

5. **`internal/relay/ffmpeg_transcoder.go`** (lines 550-1040)
   - Uses "copy" as encoder when no transcoding needed
   - `buildFFmpegCommand()` passes "copy" to ffmpeg `-c:v copy -c:a copy`

## Proposed Design

### Core Principle
The buffer and variant system should always track **actual codecs** (e.g., "h265/eac3"), never "copy". The "copy" encoder should only be determined at the point where we build FFmpeg commands, by comparing source and target variants.

### Phase 1: Rename VariantCopy to VariantSource

**Goal**: Clarify that this variant represents the source stream, not a "copy" operation.

1. Rename `VariantCopy` to `VariantSource` in `shared_buffer.go`
2. Update all references (session.go, processors, etc.)
3. Keep the value as `"source/source"` or similar

**Files affected**:
- `internal/relay/shared_buffer.go`
- `internal/relay/session.go`
- `internal/relay/processor_*.go` (all processor files)
- `internal/relay/ffmpeg_transcoder.go`
- `internal/relay/ffmpeg_transcoder_test.go`

### Phase 2: Track Actual Codecs in Source Variant

**Goal**: When source variant is created, populate it with actual detected codecs.

1. Modify `SharedESBuffer.CreateVariant()` to accept actual codec info
2. When demuxer detects codecs, update the source variant key
3. Source variant becomes e.g., `"h265/eac3"` instead of `"source/source"`

**Key changes**:
- `shared_buffer.go`: Add method to update variant key after codec detection
- `session.go`: Populate source variant with `CachedCodecInfo` codecs
- Buffer node map may need to handle variant key updates

### Phase 3: Remove "copy" from Variant Resolution

**Goal**: `DetermineTargetVariant()` returns actual codecs, never "copy".

1. When profile says "copy" for video/audio, resolve to source codec immediately
2. Target variant always contains actual codec names
3. Remove all `codec != "copy"` checks from flow_builder.go

**Key changes**:
- `session.go`: `DetermineTargetVariant()` uses `CachedCodecInfo` to resolve
- `flow_builder.go`: Remove "copy" resolution logic
- `transcoder.go`: Remove "copy" handling from `mapVariantToEncoders()`

### Phase 4: "copy" Only in FFmpeg Command Builder

**Goal**: Only the FFmpeg command builder uses "copy", determined by comparing source == target.

1. `buildFFmpegCommand()` compares source and target variants
2. If source codec == target codec for a stream, use "copy" encoder
3. This is the only place "copy" appears in the system

**Key changes**:
- `ffmpeg_transcoder.go`: Compare variants to determine if copy is appropriate
- `internal/daemon/transcode.go`: Same logic for daemon-side transcoding
- Remove `mapVariantToEncoders()` "copy" case

## Migration Strategy

1. **Phase 1** is purely a rename and can be done first with minimal risk
2. **Phase 2-3** require codec detection to complete before variant creation
3. **Phase 4** simplifies the transcoder logic significantly

## Benefits

1. **Clarity**: Variants always represent actual codecs
2. **Simplicity**: No scattered "copy" resolution logic
3. **Correctness**: Display always shows actual codecs without special cases
4. **Flexibility**: Easier to add codec conversion logic (e.g., hevc -> h264)

## Risks

1. **Codec detection timing**: Source variant must wait for codec detection
2. **Buffer key changes**: If variant key changes after creation, map lookups must handle this
3. **Backward compatibility**: API responses may change format

## Estimated Effort

- Phase 1: ~2 hours (rename + update references)
- Phase 2: ~4 hours (codec tracking in buffer)
- Phase 3: ~3 hours (remove copy resolution)
- Phase 4: ~2 hours (ffmpeg builder changes)

**Total**: ~11 hours across multiple PRs

## Files Summary

Primary files to modify:
- `internal/relay/shared_buffer.go` - Core variant handling
- `internal/relay/session.go` - Variant determination
- `internal/relay/flow_builder.go` - Display logic
- `internal/relay/transcoder.go` - Transcoder creation
- `internal/relay/ffmpeg_transcoder.go` - Command building
- `internal/daemon/transcode.go` - Daemon-side command building
- `internal/relay/processor_*.go` - All processors reference VariantCopy
