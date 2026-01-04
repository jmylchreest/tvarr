---
title: API Reference
description: REST API endpoints
sidebar_position: 4
---

# API Reference

tvarr provides a REST API for all operations.

## API Documentation

Interactive API documentation is available at:

```
http://your-tvarr:8080/docs
```

This provides OpenAPI 3.1 documentation with try-it-out functionality.

## Authentication

Currently, the API does not require authentication. Authentication is planned for a future release.

## Common Endpoints

### Health Check

```bash
GET /health
```

Returns 200 if the service is healthy.

### Sources

```bash
# List stream sources
GET /api/v1/sources

# Get source
GET /api/v1/sources/{id}

# Create source
POST /api/v1/sources

# Update source
PUT /api/v1/sources/{id}

# Delete source
DELETE /api/v1/sources/{id}

# Trigger ingestion
POST /api/v1/sources/{id}/ingest
```

### Proxies

```bash
# List proxies
GET /api/v1/proxies

# Get proxy
GET /api/v1/proxies/{id}

# Create proxy
POST /api/v1/proxies

# Update proxy
PUT /api/v1/proxies/{id}

# Delete proxy
DELETE /api/v1/proxies/{id}

# Generate proxy
POST /api/v1/proxies/{id}/generate

# Get playlist
GET /api/v1/proxy/{id}/playlist.m3u8

# Get EPG
GET /api/v1/proxy/{id}/epg.xml
```

### Jobs

```bash
# List all jobs
GET /api/v1/jobs

# List running jobs
GET /api/v1/jobs/running

# List pending jobs
GET /api/v1/jobs/pending

# Cancel job
POST /api/v1/jobs/{id}/cancel
```

### Relay/Streaming

```bash
# Stream a channel (format: hls-ts, hls-fmp4, dash, ts)
GET /api/v1/relay/channel/{id}/stream?format=hls-ts

# HLS manifest
GET /api/v1/relay/channel/{id}/manifest.m3u8

# DASH manifest
GET /api/v1/relay/channel/{id}/dash.mpd

# HLS segment
GET /api/v1/relay/channel/{id}/segment_{n}.ts

# DASH segment
GET /api/v1/relay/channel/{id}/chunk_{n}.m4s
```

### Expression Validation

```bash
# Validate an expression
POST /api/v1/expressions/validate
Content-Type: application/json

{
  "expression": "channel_name contains \"Sports\""
}
```

## Progress Updates

Subscribe to real-time progress updates via Server-Sent Events:

```bash
GET /api/v1/progress/stream
```

Events include:
- Job started/completed/failed
- Ingestion progress
- Generation progress

## Rate Limiting

The API does not currently implement rate limiting. Consider adding a reverse proxy with rate limiting for public deployments.
