# tvarr TODO

## Active Work

### ffmpegd Probe Capability

Move ffprobe logic from coordinator to daemons, enabling probing even when ffprobe isn't available locally on tvarr. See [TODO/ffmpegd-probe.md](TODO/ffmpegd-probe.md) for detailed design.

**Status**: Design complete, ready for implementation

**Summary**:
- Add `can_probe` and `can_transcode` capability flags to daemon registration
- Add `Probe` RPC to ffmpegd service (supports full/quick/health-check modes)
- Create `ProbeService` abstraction with local/remote/composite implementations
- Update relay manager and bitstream filter code to use the abstraction

**Phases**:

- [ ] **Phase 1**: Proto definitions + CLI probe command (~2-3 hours)
  - Update `pkg/ffmpegd/proto/ffmpegd.proto` with Probe RPC
  - Implement daemon-side probe handler
  - Add `tvarr-ffmpegd probe <url>` CLI command for testing

- [ ] **Phase 2**: Coordinator integration (~4-6 hours)
  - Create `internal/probe/` package with Prober interface
  - Implement LocalProber, RemoteProber, CompositeProber
  - Update `relay/manager.go` and `bitstream_filters.go`
  - Add `POST /api/v1/probe` API endpoint

- [ ] **Phase 3**: Future transcoder types (deferred)
  - Design generic TranscoderCapabilities interface
  - Support non-FFmpeg transcoders (GStreamer, hardware-only, etc.)

**Recommended first step**: Start with Phase 1 - add the proto definitions and implement `tvarr-ffmpegd probe` CLI command. This validates the design with minimal coordinator changes.

---

### Refactor 'copy' Codec to Buffer/Variant Level

Move "copy" codec handling from scattered resolution logic to a single point in the FFmpeg command builder. See [TODO/copy-copy-fix.md](TODO/copy-copy-fix.md) for detailed design.

**Status**: Design complete, ready for implementation

**Summary**:
- Rename `VariantCopy` to `VariantSource` for clarity
- Track actual codecs (e.g., "h265/eac3") in variants instead of "copy/copy"
- Remove "copy" resolution logic from flow_builder, session, transcoder
- Only use "copy" encoder in FFmpeg command builder when source == target codec

**Phases**:

- [ ] **Phase 1**: Rename VariantCopy to VariantSource (~2 hours)
  - Rename constant and update all references
  - No behavioral changes

- [ ] **Phase 2**: Track actual codecs in source variant (~4 hours)
  - Populate source variant with detected codecs from CachedCodecInfo
  - Handle variant key updates in buffer

- [ ] **Phase 3**: Remove "copy" from variant resolution (~3 hours)
  - `DetermineTargetVariant()` returns actual codecs
  - Remove `!= "copy"` checks from flow_builder.go

- [ ] **Phase 4**: "copy" only in FFmpeg command builder (~2 hours)
  - Compare source/target variants to determine if copy is appropriate
  - Single location for "copy" logic

**Recommended first step**: Start with Phase 1 - rename `VariantCopy` to `VariantSource`. This clarifies intent with minimal risk.

---

## Backlog

See [TODO/ffmpegd.md](TODO/ffmpegd.md) for the full distributed transcoding design.
