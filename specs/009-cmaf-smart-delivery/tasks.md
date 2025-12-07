# Tasks: CMAF + Smart Delivery Architecture

**Input**: spec.md, plan.md
**Feature ID**: 009-cmaf-smart-delivery

## Format: `[ID] [P?] [Phase] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Phase]**: A=Model, B=SmartDelivery, C=CMAF, D=Frontend, E=Testing

---

## Phase A: Data Model Migration

**Purpose**: Update models for new proxy modes and container format

- [ ] A01 Add ContainerFormat type and constants to internal/models/relay_profile.go
- [ ] A02 Add ContainerFormat field to RelayProfile struct
- [ ] A03 [P] Add RequiresFMP4() method to RelayProfile for codec validation
- [ ] A04 [P] Add DetermineContainer() method to RelayProfile for runtime selection
- [ ] A05 Update RelayProfile BeforeSave hook to validate codec↔container
- [ ] A06 Add StreamProxyModeDirect and StreamProxyModeSmart constants to internal/models/stream_proxy.go
- [ ] A07 [P] Deprecate old proxy mode constants (redirect/proxy/relay)
- [ ] A08 Create database migration for ContainerFormat column
- [ ] A09 Create database migration for proxy_mode value conversion
- [ ] A10 [P] Add API endpoint backward compatibility mapping
- [ ] A11 Update system profile seed data (3 profiles: Universal, Passthrough, Efficiency)
- [ ] A12 [P] Unit tests for ContainerFormat validation

**Checkpoint A**: Models updated, migrations created, validation working

---

## Phase B: Smart Delivery Logic

**Purpose**: Implement unified smart delivery dispatch

- [ ] B01 Create DeliveryDecision type in internal/relay/delivery.go
- [ ] B02 Create SelectDelivery function with decision logic
- [ ] B03 [P] Create sourceMatchesClient helper function
- [ ] B04 [P] Create canRepackage helper function
- [ ] B05 Create handleSmartDelivery in internal/http/handlers/relay_stream.go
- [ ] B06 Update handleRawStream dispatch to use direct/smart modes
- [ ] B07 Integrate SelectDelivery into handleSmartDelivery
- [ ] B08 Remove handleRawProxyMode (logic merged into smart)
- [ ] B09 Remove handleRawRelayMode (logic merged into smart)
- [ ] B10 Update RelaySession.runNormalPipeline for smart delivery
- [ ] B11 [P] Add X-Stream-Decision header values for observability
- [ ] B12 [P] Unit tests for SelectDelivery decision logic

**Checkpoint B**: Smart delivery working, old modes removed

---

## Phase C: CMAF Implementation

**Purpose**: Enable fMP4 segments for HLS v7 and DASH

### C.1 fMP4 Muxer

- [ ] C01 Create CMAFMuxer struct in internal/relay/cmaf_muxer.go
- [ ] C02 Implement fMP4 fragment parsing (moof+mdat detection)
- [ ] C03 Implement segment boundary detection (keyframe-aligned)
- [ ] C04 Add initialization segment (ftyp+moov) extraction
- [ ] C05 [P] Unit tests for fMP4 parsing

### C.2 Buffer Integration

- [ ] C06 Add containerFormat field to UnifiedBuffer
- [ ] C07 Update UnifiedBuffer.WriteChunk for fMP4 mode
- [ ] C08 Create Segment type for fMP4 segments (init + media)
- [ ] C09 Implement fMP4 segment storage in UnifiedBuffer
- [ ] C10 [P] Unit tests for UnifiedBuffer fMP4 mode

### C.3 FFmpeg Output

- [ ] C11 Add fMP4 output args to CommandBuilder (-movflags frag_keyframe+empty_moov)
- [ ] C12 Update RelaySession.buildFFmpegCommand for ContainerFormat
- [ ] C13 Add frag_duration configuration based on SegmentDuration
- [ ] C14 [P] Integration test: FFmpeg → fMP4 → UnifiedBuffer

### C.4 Handler Updates

- [ ] C15 Update HLSHandler for v7 playlists (#EXT-X-MAP, .m4s segments)
- [ ] C16 Add EXT-X-VERSION:7 when serving fMP4
- [ ] C17 Verify DASHHandler works with CMAF segments (should work as-is)
- [ ] C18 Update ServeSegment content types (video/mp4 for fMP4)
- [ ] C19 [P] Integration test: HLS v7 playlist serving
- [ ] C20 [P] Integration test: DASH MPD serving same segments as HLS

**Checkpoint C**: CMAF working, HLS v7 and DASH from same segments

---

## Phase D: Frontend Updates

**Purpose**: Update UI for simplified configuration

### D.1 Proxy Configuration

- [ ] D01 Update ProxyModeSelect component for direct/smart options
- [ ] D02 Add descriptions to mode options explaining behavior
- [ ] D03 [P] Add backward compatibility for old mode values in UI
- [ ] D04 Update proxy form validation for new modes

### D.2 Profile Configuration

- [ ] D05 Remove OutputFormat field from profile form
- [ ] D06 Add ContainerFormat dropdown to profile form (in Advanced section)
- [ ] D07 Add "auto" as default ContainerFormat selection
- [ ] D08 Update codec dropdowns to indicate container requirements
- [ ] D09 [P] Add tooltip explaining fMP4 requirement for VP9/AV1/Opus
- [ ] D10 [P] Add visual indicator when codec forces container

### D.3 Profile Selection

- [ ] D11 Update profile dropdown with descriptions
- [ ] D12 Add "Universal", "Passthrough", "Efficiency" as primary options
- [ ] D13 [P] Add "+ Create Custom Profile" option at bottom
- [ ] D14 Update profile list page with container format column

**Checkpoint D**: Frontend updated, users can configure with new UI

---

## Phase E: Testing & Documentation

**Purpose**: Ensure quality and document changes

### E.1 Integration Tests

- [ ] E01 E2E test: Direct mode returns 302 redirect
- [ ] E02 E2E test: Smart mode passthrough when formats match
- [ ] E03 E2E test: Smart mode transcode when profile requires
- [ ] E04 E2E test: CMAF HLS v7 playlist with fMP4 segments
- [ ] E05 E2E test: CMAF DASH MPD with fMP4 segments
- [ ] E06 [P] E2E test: Same segments served to HLS and DASH clients
- [ ] E07 [P] E2E test: VP9 profile creates valid fMP4 output
- [ ] E08 [P] E2E test: Legacy MPEG-TS mode still works

### E.2 Migration Testing

- [ ] E09 Test migration: redirect → direct
- [ ] E10 Test migration: proxy → smart
- [ ] E11 Test migration: relay → smart
- [ ] E12 [P] Test: old API mode values accepted

### E.3 Documentation

- [ ] E13 Update docs/relay-streaming-api.md with new modes
- [ ] E14 [P] Update API documentation for ContainerFormat
- [ ] E15 Create migration guide for existing users
- [ ] E16 [P] Update quickstart.md with simplified configuration

**Checkpoint E**: All tests passing, documentation complete

---

## Dependencies & Execution Order

```
Phase A (Model) ─────────────────────────────────────────►
       │
       └─► Phase B (Smart Delivery) ─────────────────────►
                    │
                    └─► Phase C (CMAF) ──────────────────►
                                │
Phase D (Frontend) can start after A completes ──────────►
                                │
                                └─► Phase E (Testing) ───►
```

### Parallel Opportunities

**Within Phase A:**
```
A01-A02 sequential, then A03+A04+A07+A10+A12 parallel
```

**Within Phase B:**
```
B01-B02 sequential, then B03+B04+B11+B12 parallel
```

**Within Phase C:**
```
C.1 (Muxer) can run in parallel with C.3 (FFmpeg)
C.2 (Buffer) depends on C.1
C.4 (Handlers) depends on C.2
```

**Phase D:**
```
D.1 (Proxy) and D.2 (Profile) can run in parallel
D.3 (Selection) depends on D.2
```

---

## MVP Scope

For minimum viable delivery:

1. **Phase A**: All tasks (required for any functionality)
2. **Phase B**: B01-B07 (core smart delivery)
3. **Phase C**: C01-C05, C11-C14 (fMP4 output, no HLS v7 yet)
4. **Phase D**: D01-D04 (proxy UI only)
5. **Phase E**: E01-E03 (basic tests)

This delivers:
- 2-mode proxy system
- fMP4 container support
- Smart delivery logic
- Basic UI update

HLS v7 playlists and DASH CMAF can follow in next iteration.

---

## Notes

- All tests use fictional channel/proxy IDs per constitution XIII
- Commit after each task or logical group
- VP9/AV1 require FFmpeg with appropriate libraries
- Legacy MPEG-TS must continue working (no breaking changes)
