---
title: Client Detection
description: Serve different quality based on device
sidebar_position: 4
---

# Client Detection

Client detection rules let you serve different encoding profiles based on who's watching.

## How It Works

When a player requests a stream:

1. tvarr examines the request (headers, IP, query params)
2. Matches against client detection rules
3. Applies the matched encoding profile

## Creating a Rule

Go to **Admin > Client Detection** and click **Add Rule**.

1. **Name** - Descriptive name (e.g., "Mobile Devices")
2. **Expression** - Condition to match
3. **Encoding Profile** - Which profile to use when matched
4. **Priority** - Higher priority rules match first

## Available Fields

| Field | Description | Example |
|-------|-------------|---------|
| `client_ip` | Client's IP address | `192.168.1.100` |
| `host` | Host header | `tvarr.example.com` |
| `request_path` | URL path | `/api/v1/relay/...` |

### Dynamic Headers

Access any request header:

```
@dynamic(request.headers):user-agent
@dynamic(request.headers):x-video-codec
@dynamic(request.headers):x-audio-codec
```

### Dynamic Query Parameters

Access query string values:

```
@dynamic(request.query):quality
@dynamic(request.query):format
```

## Common Patterns

### Android Devices

```
@dynamic(request.headers):user-agent contains "Android"
```

### iOS Devices

```
@dynamic(request.headers):user-agent contains "iPhone" OR
@dynamic(request.headers):user-agent contains "iPad"
```

### Jellyfin Clients

```
@dynamic(request.headers):user-agent contains "Jellyfin"
```

### Local Network

```
client_ip starts_with "192.168."
```

### Codec Preferences

When clients send codec preferences (like Jellyfin does):

```
@dynamic(request.headers):x-video-codec equals "h264"
```

## Example Setup

### Mobile Gets 720p

1. Create encoding profile "Mobile 720p" with lower bitrate
2. Create client detection rule:
   - Expression: `@dynamic(request.headers):user-agent contains "Android"`
   - Profile: Mobile 720p
   - Priority: 100

### TV Gets 4K

1. Create encoding profile "4K HDR"
2. Create client detection rule:
   - Expression: `@dynamic(request.headers):user-agent contains "AndroidTV"`
   - Profile: 4K HDR
   - Priority: 200 (higher than mobile)

Rules are evaluated in priority order. First match wins.
