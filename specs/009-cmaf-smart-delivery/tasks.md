# Tasks: CMAF + Smart Delivery Architecture

**Input**: spec.md, plan.md
**Feature ID**: 009-cmaf-smart-delivery

## Format: `[ID] [P?] [Phase] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Phase]**: A=Model, B=SmartDelivery, C=CMAF, D=Frontend, E=Testing

---

## Phase A: Data Model Migration

**Purpose**: Update models for new proxy modes and container format

**TDD Note**: Test tasks (A12) run in parallel with implementation per Constitution III.

- [X] A01 Add ContainerFormat type and constants to internal/models/relay_profile.go
- [X] A02 Add ContainerFormat field to RelayProfile struct
- [X] A03 [P] Add RequiresFMP4() method to RelayProfile for codec validation
- [X] A04 [P] Add DetermineContainer() method to RelayProfile for runtime selection
- [X] A05 [P] Unit tests for ContainerFormat validation (TDD: write with A03/A04)
- [X] A06 Update RelayProfile BeforeSave hook to validate codec↔container
- [X] A07 Add StreamProxyModeDirect and StreamProxyModeSmart constants to internal/models/stream_proxy.go
- [X] A08 [P] Deprecate old proxy mode constants (redirect/proxy/relay)
- [X] A09 Create database migration for ContainerFormat column
- [X] A10 Create database migration for proxy_mode value conversion
- [X] A11 [P] Add API endpoint backward compatibility mapping with deprecation warning logging
- [X] A12 Update system profile seed data (3 profiles: Universal, Passthrough, Efficiency)

**Checkpoint A**: Models updated, migrations created, validation working ✓ COMPLETE

---

## Phase B: Smart Delivery Logic

**Purpose**: Implement unified smart delivery dispatch

**TDD Note**: Test tasks (B12) run in parallel with implementation per Constitution III.

**Clarification**: "Repackage" (canRepackage) means serving existing segments with different manifest format. True TS→HLS/DASH conversion without pre-existing segments requires FFmpeg pipeline (DeliveryTranscode), not repackage.

- [X] B01 Create DeliveryDecision type in internal/relay/delivery.go
- [X] B02 Create SelectDelivery function with decision logic
- [X] B03 [P] Create sourceMatchesClient helper function
- [X] B04 [P] Create canRepackage helper function (true only if source has segments, not raw TS)
- [X] B05 [P] Unit tests for SelectDelivery decision logic (TDD: write with B01-B04)
- [X] B06 Create handleSmartDelivery in internal/http/handlers/relay_stream.go
- [X] B07 Update handleRawStream dispatch to use direct/smart modes
- [X] B08 Integrate SelectDelivery into handleSmartDelivery
- [X] B09 Remove handleRawProxyMode (logic merged into smart) - Deprecated, kept for backward compatibility
- [X] B10 Remove handleRawRelayMode (logic merged into smart) - Deprecated, kept for backward compatibility
- [X] B11 Update RelaySession.runNormalPipeline for smart delivery
- [X] B12 [P] Add X-Stream-Decision header values for observability (passthrough/repackage/transcode)

**Checkpoint B**: Smart delivery working, old modes deprecated ✓ COMPLETE

---

## Phase C: CMAF Implementation

**Purpose**: Enable fMP4 segments for HLS v7 and DASH

### C.1 fMP4 Muxer

**TDD Note**: Test tasks (C05) run in parallel with implementation per Constitution III.

- [X] C01 Create CMAFMuxer struct in internal/relay/cmaf_muxer.go
- [X] C02 Implement fMP4 fragment parsing (moof+mdat detection)
- [X] C03 Implement segment boundary detection (keyframe-aligned)
- [X] C04 Add initialization segment (ftyp+moov) extraction
- [X] C05 [P] Unit tests for fMP4 parsing (TDD: write with C01-C04)

### C.2 Buffer Integration

**TDD Note**: Test tasks (C10) run in parallel with implementation per Constitution III.

**Clarification**: C08 extends existing internal/relay/segment.go (from 008) with fMP4-specific fields (InitSegment, MediaSegments with moof+mdat boundaries), not a replacement.

- [X] C06 Add containerFormat field to UnifiedBuffer
- [X] C07 Update UnifiedBuffer.WriteChunk for fMP4 mode
- [X] C08 Extend Segment type in segment.go for fMP4 (add InitSegment []byte, IsFragmented bool)
- [X] C09 Implement fMP4 segment storage in UnifiedBuffer
- [X] C10 [P] Unit tests for UnifiedBuffer fMP4 mode (TDD: write with C06-C09)

### C.3 FFmpeg Output

- [X] C11 Add fMP4 output args to CommandBuilder (-movflags frag_keyframe+empty_moov)
- [X] C12 Update RelaySession.buildFFmpegCommand for ContainerFormat
- [X] C13 Add frag_duration configuration based on SegmentDuration
- [X] C14 [P] Integration test: FFmpeg → fMP4 → UnifiedBuffer

### C.4 Handler Updates

- [X] C15 Update HLSHandler for v7 playlists (#EXT-X-MAP, .m4s segments)
- [X] C16 Add EXT-X-VERSION:7 when serving fMP4
- [X] C17 Verify DASHHandler works with CMAF segments (should work as-is)
- [X] C18 Update ServeSegment content types (video/mp4 for fMP4)
- [X] C19 [P] Integration test: HLS v7 playlist serving
- [X] C20 [P] Integration test: DASH MPD serving same segments as HLS

**Checkpoint C**: CMAF working, HLS v7 and DASH from same segments - COMPLETE

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

**Test Type**: Unit tests using GORM with in-memory SQLite database.

- [ ] E09 Unit test migration: redirect → direct (verify DB state after migration)
- [ ] E10 Unit test migration: proxy → smart (verify DB state after migration)
- [ ] E11 Unit test migration: relay → smart (verify DB state after migration)
- [ ] E12 [P] Unit test: old API mode values accepted and logged as deprecated

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

### Parallel Opportunities (TDD-Compliant)

**Within Phase A:**
```
A01-A02 sequential
A03+A04+A05 parallel (implementation + tests together)
A06 after A05
A07+A08+A11 parallel
A09-A10 sequential (migrations)
A12 after A10
```

**Within Phase B:**
```
B01-B02 sequential
B03+B04+B05 parallel (implementation + tests together)
B06-B11 sequential
B12 parallel with B06+
```

**Within Phase C:**
```
C.1 (Muxer + tests): C01-C04+C05 parallel group
C.2 (Buffer + tests): C06-C09+C10 parallel group, depends on C.1
C.3 (FFmpeg): C11-C14 can start after C.1
C.4 (Handlers): C15-C20 depends on C.2
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
