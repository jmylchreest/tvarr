# Data Model: FFmpeg Relay and Stream Transcoding Proxy

**Phase**: 1 - Design | **Date**: 2025-12-05 | **Spec**: [spec.md](spec.md)

## Overview

This document describes the **existing** data model architecture for relay and proxy modes, and specifies the minimal extensions needed for CORS and error fallback features.

## Existing Architecture (No Changes Needed)

### StreamProxy.ProxyMode (Already Implemented)

**File**: `internal/models/stream_proxy.go:21-31`

The mode selection already exists at the **proxy level**, not the profile level:

```go
// StreamProxyMode represents how the proxy serves streams.
type StreamProxyMode string

const (
    // StreamProxyModeRedirect redirects clients directly to the upstream URL.
    StreamProxyModeRedirect StreamProxyMode = "redirect"
    // StreamProxyModeProxy proxies the stream through tvarr.
    StreamProxyModeProxy StreamProxyMode = "proxy"
    // StreamProxyModeRelay relays the stream through FFmpeg for transcoding.
    StreamProxyModeRelay StreamProxyMode = "relay"
)
```

### StreamProxy Fields (Already Implemented)

**File**: `internal/models/stream_proxy.go:47-121`

```go
type StreamProxy struct {
    // ...
    ProxyMode      StreamProxyMode `gorm:"not null;default:'redirect';size:20" json:"proxy_mode"`
    RelayProfileID *ULID           `gorm:"type:varchar(26)" json:"relay_profile_id,omitempty"`
    // ...
}
```

- When `proxy_mode = redirect`: HTTP 302 redirect to source
- When `proxy_mode = proxy`: Fetch and forward stream
- When `proxy_mode = relay`: Use associated `RelayProfile` for FFmpeg transcoding

### RelayProfile (Already Implemented)

**File**: `internal/models/relay_profile.go:59-121`

Contains all transcoding settings (video/audio codecs, bitrates, HW acceleration, etc.)

**No `Mode` field needed on RelayProfile** - the mode is determined by `StreamProxy.ProxyMode`.

---

## Entity Extensions (New)

### RelayProfile Fallback Settings

Add error fallback configuration to existing `RelayProfile`:

**File**: `internal/models/relay_profile.go` (add after existing fields)

```go
type RelayProfile struct {
    // ... existing fields (Name, VideoCodec, AudioCodec, etc.) ...

    // Fallback settings for error handling (NEW)
    FallbackEnabled          bool `gorm:"default:true" json:"fallback_enabled"`
    FallbackErrorThreshold   int  `gorm:"default:3" json:"fallback_error_threshold"`
    FallbackRecoveryInterval int  `gorm:"default:30" json:"fallback_recovery_interval"` // seconds
}
```

---

## Database Migration

### Migration Strategy

1. **Automatic Migration**: GORM will auto-add the new fallback columns with defaults
2. **Data Migration**: Not required - existing profiles get default values
3. **Rollback**: Remove columns (data loss acceptable for new feature)

### Migration SQL (Reference)

```sql
-- Add fallback columns to relay_profiles
ALTER TABLE relay_profiles ADD COLUMN fallback_enabled BOOLEAN DEFAULT true;
ALTER TABLE relay_profiles ADD COLUMN fallback_error_threshold INTEGER DEFAULT 3;
ALTER TABLE relay_profiles ADD COLUMN fallback_recovery_interval INTEGER DEFAULT 30;
```

**Note**: No `mode` column needed on `relay_profiles` - mode is already on `stream_proxies`.

---

## Runtime Entities

These entities exist only in memory and are not persisted to the database.

### ActiveRelay (Existing: `internal/relay/manager.go`)

No changes required. The `RelaySession` struct already tracks:
- Session ID, Channel ID, Stream URL
- Profile reference
- Classification result
- Buffer, FFmpeg command references
- Client list (via CyclicBuffer)

### RelayClient (Existing: `internal/relay/cyclic_buffer.go`)

The `BufferClient` struct already tracks:
- Client ID
- User Agent
- Remote Address
- Bytes Read
- Connected At
- Last Read time

**Enhancement**: Add explicit IP field parsing from RemoteAddr.

---

## System Profiles (Reference)

System relay profiles for transcoding are seeded on first startup. These are **only** used when `proxy_mode = relay`:

| Profile | VideoCodec | AudioCodec | Purpose |
|---------|-----------|-----------|---------|
| Passthrough | copy | copy | HLS collapse, no transcoding |
| H.264 720p | libx264 | aac | Software encode 720p @ 3Mbps |
| H.264 1080p | libx264 | aac | Software encode 1080p @ 6Mbps |

**Note**: "Redirect" and "Proxy" modes are selected via `StreamProxy.ProxyMode`, not via RelayProfile.

---

## Entity Relationships

```
┌─────────────────────┐
│    StreamProxy      │
├─────────────────────┤
│ ProxyMode           │ ← redirect | proxy | relay
│ RelayProfileID      │──────┐ (only used when mode = relay)
└─────────────────────┘      │
Note: Output format (mpegts/hls) is NOT stored on StreamProxy.
      It's determined by client request (URL extension or ?format=).
                             │
┌─────────────────────┐      │
│      Channel        │      │
├─────────────────────┤      │  N:1
│ StreamURL           │      │
│ (inherits from      │      ▼
│  proxy config)      │  ┌─────────────────────┐
└─────────────────────┘  │   RelayProfile      │
                         ├─────────────────────┤
                         │ VideoCodec          │
                         │ AudioCodec          │
                         │ HWAccel             │
                         │ VideoBitrate        │
                         │ AudioBitrate        │
                         │ FallbackEnabled     │
                         │ FallbackThreshold   │
                         └─────────────────────┘
                                │
                                │ runtime (relay mode only)
                                ▼
                         ┌─────────────────────┐
                         │   RelaySession      │ (in-memory)
                         ├─────────────────────┤
                         │ SessionID           │
                         │ ProxyID             │
                         │ ChannelID           │
                         │ Mode (from proxy)   │
                         │ Profile*            │
                         │ CyclicBuffer*       │
                         └────────┬────────────┘
                                  │ 1:N
                                  ▼
                         ┌─────────────────────┐
                         │   BufferClient      │ (in-memory)
                         ├─────────────────────┤
                         │ ClientID            │
                         │ UserAgent           │
                         │ RemoteAddr          │
                         │ BytesRead           │
                         └─────────────────────┘
```

---

## Constraints and Validation

| Field | Constraint | Validation |
|-------|-----------|------------|
| Mode | ENUM | Must be redirect, proxy, or transcode |
| Mode + VideoCodec | Logical | If mode=redirect, codecs ignored |
| Mode + VideoCodec | Logical | If mode=proxy, VideoCodec must be copy |
| FallbackErrorThreshold | Range | 1-10 |
| FallbackRecoveryInterval | Range | 5-300 seconds |

---

## Backward Compatibility

| Scenario | Behavior |
|----------|----------|
| Existing profile, no mode | Gets default "transcode" |
| Profile with VideoCodec=copy, AudioCodec=copy | Works as before (passthrough) |
| Profile with transcoding settings | Works as before |
| API requests without mode field | Defaults to "transcode" |
