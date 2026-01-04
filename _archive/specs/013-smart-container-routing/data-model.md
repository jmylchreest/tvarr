# Data Model: Smart Container Routing

**Feature Branch**: `013-smart-container-routing`
**Date**: 2025-12-10
**Status**: Draft

## Schema Changes

### RelayProfile Model Extension

**File**: `internal/models/relay_profile.go`

Add `DetectionMode` field to control routing behavior:

```go
// DetectionMode controls client detection behavior for routing decisions.
// When "auto", the system uses client detection (headers, query params) to optimize delivery.
// When set to any other value (e.g., "hls", "mpegts"), profile settings are used as-is.
type DetectionMode string

const (
    // DetectionModeAuto enables smart routing based on client detection
    DetectionModeAuto DetectionMode = "auto"
    // DetectionModeHLS forces HLS output regardless of client
    DetectionModeHLS DetectionMode = "hls"
    // DetectionModeMPEGTS forces MPEG-TS output regardless of client
    DetectionModeMPEGTS DetectionMode = "mpegts"
    // DetectionModeDASH forces DASH output regardless of client
    DetectionModeDASH DetectionMode = "dash"
)

// RelayProfile - add field after ContainerFormat
type RelayProfile struct {
    // ... existing fields ...

    // Output settings
    ContainerFormat ContainerFormat `gorm:"size:20;default:'auto'" json:"container_format"`

    // NEW: Detection mode for routing decisions
    // "auto" enables client detection, other values use profile as-is
    DetectionMode   DetectionMode   `gorm:"size:20;default:'auto'" json:"detection_mode"`

    SegmentDuration int             `gorm:"default:6" json:"segment_duration,omitempty"`
    // ... rest of existing fields ...
}
```

### Migration

**File**: `internal/database/migrations/XXX_add_detection_mode.go`

```go
func init() {
    registerMigration(Migration{
        ID: "add_detection_mode",
        Up: func(tx *gorm.DB) error {
            return tx.Exec(`
                ALTER TABLE relay_profiles
                ADD COLUMN IF NOT EXISTS detection_mode VARCHAR(20) DEFAULT 'auto'
            `).Error
        },
        Down: func(tx *gorm.DB) error {
            return tx.Exec(`
                ALTER TABLE relay_profiles
                DROP COLUMN IF EXISTS detection_mode
            `).Error
        },
    })
}
```

## New Types

### RoutingDecision

**File**: `internal/relay/routing_decision.go`

```go
package relay

// RoutingDecision represents the chosen delivery path for a stream.
type RoutingDecision int

const (
    // RoutePassthrough - Direct proxy of source segments (no processing)
    // Used when: source format matches client format, codecs compatible
    RoutePassthrough RoutingDecision = iota

    // RouteRepackage - Container change via gohlslib Muxer (no codec change)
    // Used when: source is HLS/DASH, client wants different container, codecs match
    RouteRepackage

    // RouteTranscode - FFmpeg pipeline required
    // Used when: codec mismatch, raw TS source, or profile specifies transcoding
    RouteTranscode
)

func (d RoutingDecision) String() string {
    switch d {
    case RoutePassthrough:
        return "passthrough"
    case RouteRepackage:
        return "repackage"
    case RouteTranscode:
        return "transcode"
    default:
        return "unknown"
    }
}

// RoutingResult contains the full routing decision with context.
type RoutingResult struct {
    Decision       RoutingDecision
    SourceFormat   SourceFormat
    ClientFormat   string // "hls-fmp4", "hls-ts", "mpegts", "dash"
    DetectionMode  string // Profile's detection_mode value
    Reasons        []string
}
```

### ClientCapabilities

**File**: `internal/relay/client_detector.go`

```go
package relay

// ClientCapabilities represents detected client information.
type ClientCapabilities struct {
    // PlayerName is the player identifier (e.g., "hls.js", "mpegts.js")
    PlayerName    string

    // PlayerVersion is the version string if available
    PlayerVersion string

    // PreferredFormat is the detected format preference
    // Values: "hls-fmp4", "hls-ts", "mpegts", "dash", ""
    PreferredFormat string

    // SupportsFMP4 indicates fMP4 segment support
    SupportsFMP4  bool

    // SupportsMPEGTS indicates MPEG-TS segment support
    SupportsMPEGTS bool

    // DetectionSource indicates how capabilities were detected
    // Values: "x-tvarr-player", "user-agent", "accept", "default"
    DetectionSource string
}
```

### Extended OutputRequest

**File**: `internal/relay/format_router.go`

Extend existing struct:

```go
// OutputRequest represents a client request for stream output.
type OutputRequest struct {
    // Existing fields
    Format    string
    Segment   *uint64
    InitType  string
    UserAgent string
    Accept    string

    // NEW: X-Tvarr-Player header value for player identification
    XTvarrPlayer string

    // NEW: ?format= query parameter for explicit override
    FormatOverride string
}
```

## Constants

**File**: `internal/relay/constants.go`

```go
package relay

// Header constants
const (
    // HeaderXTvarrPlayer is the custom header for player identification
    HeaderXTvarrPlayer = "X-Tvarr-Player"
)

// Format values (existing, ensure defined)
const (
    FormatValueHLS    = "hls"
    FormatValueDASH   = "dash"
    FormatValueMPEGTS = "mpegts"
    FormatValueAuto   = "auto"

    // NEW: Sub-format values for HLS container type
    FormatValueHLSFMP4 = "hls-fmp4"
    FormatValueHLSTS   = "hls-ts"
)

// Query parameter names
const (
    QueryParamFormat = "format"
)
```

## Interfaces

### ClientDetector Interface

**File**: `internal/relay/client_detector.go`

```go
// ClientDetector detects client capabilities from request metadata.
type ClientDetector interface {
    // Detect analyzes the request and returns client capabilities.
    // Detection priority: FormatOverride > XTvarrPlayer > Accept > UserAgent
    Detect(req OutputRequest) ClientCapabilities
}
```

### RoutingDecider Interface

**File**: `internal/relay/routing_decision.go`

```go
// RoutingDecider determines the optimal routing path for a stream.
type RoutingDecider interface {
    // Decide returns the routing decision based on source, client, and profile.
    Decide(
        sourceFormat SourceFormat,
        sourceCodecs []string,
        client ClientCapabilities,
        profile *models.RelayProfile,
    ) RoutingResult
}
```

## Entity Relationships

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  RelayProfile   │     │  OutputRequest   │     │ClientCapabilities│
├─────────────────┤     ├──────────────────┤     ├─────────────────┤
│ DetectionMode   │────▶│ XTvarrPlayer     │────▶│ PlayerName      │
│ ContainerFormat │     │ FormatOverride   │     │ PreferredFormat │
│ VideoCodec      │     │ UserAgent        │     │ SupportsFMP4    │
│ AudioCodec      │     │ Accept           │     │ SupportsMPEGTS  │
└─────────────────┘     └──────────────────┘     └─────────────────┘
         │                       │                        │
         │                       │                        │
         ▼                       ▼                        ▼
┌─────────────────────────────────────────────────────────────────┐
│                       RoutingDecider                             │
├─────────────────────────────────────────────────────────────────┤
│ Decide(sourceFormat, sourceCodecs, client, profile) Result      │
└─────────────────────────────────────────────────────────────────┘
                                 │
                                 ▼
                    ┌─────────────────────┐
                    │   RoutingResult     │
                    ├─────────────────────┤
                    │ Decision            │
                    │ SourceFormat        │
                    │ ClientFormat        │
                    │ Reasons             │
                    └─────────────────────┘
```

## Validation Rules

### RelayProfile.DetectionMode

- Valid values: `"auto"`, `"hls"`, `"mpegts"`, `"dash"`
- Default: `"auto"`
- When not `"auto"`, client detection is bypassed

### Routing Decision Constraints

1. If `DetectionMode != "auto"`:
   - Use profile's ContainerFormat and codec settings directly
   - Skip client detection entirely

2. If `DetectionMode == "auto"`:
   - Apply client detection priority: FormatOverride > XTvarrPlayer > Accept > UserAgent
   - Consider source format and codec compatibility
   - Choose optimal path: Passthrough > Repackage > Transcode

3. Codec compatibility check:
   - H.264/AAC: Compatible with all paths
   - H.265/AAC: Compatible with all paths
   - VP9/AV1/Opus: Require fMP4 container, may force Transcode if client needs TS
