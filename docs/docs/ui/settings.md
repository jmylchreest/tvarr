---
title: Settings
description: Global configuration
sidebar_position: 7
---

# Settings

Global tvarr configuration accessible from the web UI.

## Server Settings

| Setting | Description |
|---------|-------------|
| Base URL | External URL for generated playlists |
| Port | HTTP server port |

## Logging

| Setting | Description |
|---------|-------------|
| Level | Log verbosity (debug, info, warn, error) |
| Format | Log format (json, text) |

## Storage

| Setting | Description |
|---------|-------------|
| Data Directory | Where data is stored |
| Logo Retention | How long to keep cached logos |

## Relay Settings

| Setting | Description |
|---------|-------------|
| Enabled | Enable relay/transcoding mode |
| FFmpeg Path | Path to FFmpeg binary |
| Max Streams | Maximum concurrent streams |

## gRPC Settings

For distributed transcoding:

| Setting | Description |
|---------|-------------|
| Enabled | Enable gRPC server for workers |
| Port | gRPC listen port |

## Feature Flags

Experimental features that can be enabled/disabled.

:::note
Most settings can also be configured via environment variables. Environment variables take precedence over UI settings.
:::
