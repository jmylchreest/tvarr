# Research: FFmpeg Relay and Stream Transcoding Proxy

**Phase**: 0 - Research | **Date**: 2025-12-05 | **Spec**: [spec.md](spec.md)

## Executive Summary

This research documents the existing relay infrastructure in tvarr and identifies the specific gaps to address in the FFmpeg relay transcoding feature. The key finding is that **most of the core functionality already exists** - the implementation focus should be on adding the RelayMode field, HTTP redirect handler, CORS middleware, and error fallback stream generation.

## Research Topic 1: RelayMode Integration

### Question
How should we add a `mode` field to RelayProfile without breaking existing profiles?

### Findings

**Current RelayProfile Model** (`internal/models/relay_profile.go`):
- No explicit `mode` field exists
- Logic currently determines mode based on codec settings:
  - If VideoCodec == "copy" AND AudioCodec == "copy" -> passthrough
  - Otherwise -> transcode
- Missing: explicit redirect mode (HTTP 302)

**Proposed Solution**:
```go
// RelayMode represents the stream delivery mode.
type RelayMode string

const (
    RelayModeRedirect  RelayMode = "redirect"   // HTTP 302 to source
    RelayModeProxy     RelayMode = "proxy"      // Fetch and forward
    RelayModeTranscode RelayMode = "transcode"  // FFmpeg processing
)
```

**Migration Strategy**:
1. Add `Mode` field to RelayProfile with default "transcode"
2. GORM auto-migration will add the column
3. Existing profiles will get default value
4. No data loss, backward compatible

### Recommendation
Add `RelayMode` enum and `Mode` field with default value of `transcode` for backward compatibility.

---

## Research Topic 2: CORS Middleware

### Question
What is the best approach for adding CORS headers in Huma/Chi?

### Findings

**Current Implementation**:
- No global CORS middleware exists
- Individual handlers set headers manually as needed

**Options Analyzed**:

1. **Chi CORS Middleware** (github.com/go-chi/cors)
   - Pros: Standard, well-tested, configurable
   - Cons: Applies globally, may affect other endpoints

2. **Per-Handler CORS Headers**
   - Pros: Fine-grained control
   - Cons: Repetitive code

3. **Huma Middleware Wrapper**
   - Pros: Integrates with Huma's middleware chain
   - Cons: Need to wrap chi middleware

**CORS Requirements for Relay Endpoints**:
```http
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, OPTIONS
Access-Control-Allow-Headers: Content-Type, Accept, Range
Access-Control-Expose-Headers: Content-Length, Content-Range
```

**Current Code Pattern** (`internal/http/handlers/relay_stream.go:137-141`):
```go
ctx.SetHeader("Content-Type", "video/mp2t")
ctx.SetHeader("Cache-Control", "no-cache, no-store")
ctx.SetHeader("Connection", "keep-alive")
```

### Recommendation
Create a reusable CORS helper function in the handlers package that can be called for relay endpoints in proxy and transcode modes. This avoids global middleware complexity while keeping code DRY.

```go
// setCORSHeaders sets permissive CORS headers for streaming endpoints.
func setCORSHeaders(ctx huma.Context) {
    ctx.SetHeader("Access-Control-Allow-Origin", "*")
    ctx.SetHeader("Access-Control-Allow-Methods", "GET, OPTIONS")
    ctx.SetHeader("Access-Control-Allow-Headers", "Content-Type, Accept, Range")
    ctx.SetHeader("Access-Control-Expose-Headers", "Content-Length, Content-Range")
}
```

---

## Research Topic 3: TS Error Stream Generation

### Question
How do we generate a Transport Stream compatible error image to serve during failures?

### Findings

**Requirement**: FR-018 specifies generating a TS-compatible fallback stream during errors.

**FFmpeg Command for Static Image to TS**:
```bash
ffmpeg -f lavfi -i color=c=black:s=1920x1080:d=1 \
       -f lavfi -i anullsrc=r=48000:cl=stereo \
       -c:v libx264 -preset ultrafast -tune stillimage \
       -c:a aac -b:a 128k \
       -t 10 -f mpegts pipe:1
```

This generates:
- 10 seconds of black video (1080p)
- Silent audio track
- MPEG-TS container output to stdout

**Alternative: Pre-generated Error Image**:
1. Generate error image with text overlay at build time
2. Store as embedded asset
3. Serve directly without FFmpeg

**Existing Error Handling** (`internal/relay/manager.go:347-352`):
```go
// Record failure/success with circuit breaker
cb := s.manager.circuitBreakers.Get(s.StreamURL)
if err != nil && !errors.Is(err, context.Canceled) {
    cb.RecordFailure()
}
```

### Recommendation
Implement a `FallbackStreamGenerator` that:
1. Pre-generates an error TS segment at startup (configurable duration, e.g., 5 seconds)
2. Loops the segment when serving error fallback
3. Includes visual "Stream Unavailable" overlay
4. Avoids spawning FFmpeg per error (memory efficient)

---

## Research Topic 4: Frontend State Management

### Question
How should the dashboard poll/stream relay stats?

### Findings

**Existing SSE Infrastructure** (`internal/http/handlers/progress.go`):
- SSE endpoint exists at `/api/v1/progress/events`
- Pattern: `event: progress\ndata: {...}\n\n`

**Existing Stats API** (`internal/http/handlers/relay_profile.go:566`):
```go
func (h *RelayProfileHandler) GetStats(ctx context.Context, input *GetRelayStatsInput) (*GetRelayStatsOutput, error) {
    stats := h.relayService.GetRelayStats()
    // Returns: ActiveSessions, MaxSessions, Sessions[], ConnectionPool
}
```

**Frontend Options**:

1. **Polling (Recommended for MVP)**
   - Simple implementation
   - `/api/v1/relay/stats` every 5 seconds
   - Uses existing endpoint

2. **SSE Stream**
   - New endpoint `/api/v1/relay/events`
   - More complex, requires backend changes
   - Better for real-time updates

**Existing Frontend Pattern** (`frontend/src/components/`):
- React Query for data fetching
- Polling via `refetchInterval` option

### Recommendation
Use polling for MVP with React Query:
```typescript
const { data: relayStats } = useQuery({
  queryKey: ['relay-stats'],
  queryFn: () => fetchRelayStats(),
  refetchInterval: 5000, // 5 second refresh
});
```

---

## Research Topic 5: Redirect Mode Handler

### Question
How should redirect mode work at the HTTP handler level?

### Findings

**Current Flow** (`internal/http/handlers/relay_stream.go:82-184`):
1. Parse channel ID and profile ID
2. Call `relayService.StartRelay()` - starts session, FFmpeg, etc.
3. Stream response via `huma.StreamResponse`

**Redirect Mode Requirements**:
- Return HTTP 302 with `Location` header pointing to source URL
- No session creation needed
- No buffering needed

**Implementation Approach**:
```go
func (h *RelayStreamHandler) StreamChannel(ctx context.Context, input *StreamChannelInput) (*huma.StreamResponse, error) {
    // Get channel
    channel, err := h.channelService.GetByID(ctx, channelID)

    // Get profile
    profile, err := h.relayService.GetProfileByID(ctx, profileID)

    // Check mode
    if profile.Mode == models.RelayModeRedirect {
        return nil, huma.Status302Found(channel.StreamURL)
    }

    // Existing logic for proxy/transcode...
}
```

**Huma Redirect Support**:
Huma doesn't have built-in redirect. Options:
1. Return error with 302 status code
2. Use raw response writing
3. Create custom response type

### Recommendation
Use Huma's error mechanism with custom 302 status:
```go
return nil, huma.NewError(http.StatusFound, "redirecting",
    huma.WithHeader("Location", channel.StreamURL))
```

Or implement via custom response in StreamResponse body function.

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Mode migration breaks existing profiles | Low | High | Default to "transcode" |
| CORS headers expose security issues | Medium | Medium | Only apply to relay endpoints |
| Error stream generation uses too much memory | Low | Medium | Pre-generate, don't spawn FFmpeg |
| Frontend polling creates server load | Low | Low | 5 second interval, cache stats |

---

## Decisions Log

| Decision | Rationale | Alternatives Rejected |
|----------|-----------|----------------------|
| Add Mode enum to RelayProfile | Clean extension, backward compatible | Separate lookup table |
| Per-handler CORS headers | Fine-grained control | Global middleware |
| Pre-generated error TS | Memory efficient, fast | Dynamic FFmpeg generation |
| Polling for dashboard | Simple, uses existing API | SSE (defer to future) |
| HTTP 302 via Huma error | Works with existing patterns | Custom response type |

---

## Open Questions

1. **Client Details API Granularity**: Should client list be nested in session stats or separate endpoint?
   - Recommendation: Separate endpoint `/api/v1/relay/sessions/{id}/clients` for scalability

2. **Mode Inheritance**: Should proxy mode inherit from channel or always use profile?
   - Recommendation: Always use profile mode, channel provides stream URL only

3. **Dashboard Update Latency**: Is 5 seconds acceptable per SC-007?
   - Yes, spec says "less than 5 second update latency"
