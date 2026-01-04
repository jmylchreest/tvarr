---
title: Debugging
description: Troubleshooting common issues
sidebar_position: 8
---

# Debugging

When things don't work as expected, here's how to troubleshoot.

## Check the Logs

Logs are your primary debugging tool. Increase verbosity if needed:

```bash
TVARR_LOGGING_LEVEL=debug
```

### Docker Logs

```bash
# Follow logs
docker logs -f tvarr

# Last 100 lines
docker logs --tail 100 tvarr

# Filter for errors
docker logs tvarr 2>&1 | grep -i error
```

### Kubernetes Logs

```bash
# Follow logs
kubectl logs -f deploy/tvarr

# Previous container (if crashed)
kubectl logs deploy/tvarr --previous
```

## Common Issues

### "No channels after ingestion"

**Check:** Source URL is accessible

```bash
# Test from inside container
docker exec tvarr curl -I "your-m3u-url"
```

**Check:** Source format is correct

- M3U files must start with `#EXTM3U`
- Xtream credentials must be valid

**Check:** Filters aren't excluding everything

- Temporarily disable filters
- Check filter expressions for typos

### "EPG not showing"

**Check:** EPG source ingested successfully

```bash
docker logs tvarr 2>&1 | grep -i epg
```

**Check:** tvg-id matches between M3U and XMLTV

- View channel details to see tvg-id
- Compare with XMLTV channel ids

**Fix:** Use data mapping to set correct tvg-id

```
channel_name contains "BBC One" SET tvg_id = "bbc1.uk"
```

### "Stream won't play"

**Check:** Origin stream is accessible

```bash
# Test stream URL
docker exec tvarr curl -I "stream-url"
```

**Check:** Transcoding errors

```bash
docker logs tvarr 2>&1 | grep -i ffmpeg
```

**Check:** Client codec support

- Try different format (hls-ts vs dash)
- Try passthrough (no transcoding)

### "Transcoding not working"

**Check:** FFmpeg is detected

```bash
docker exec tvarr ffmpeg -version
```

**Check:** Hardware acceleration

```bash
# List available encoders
docker exec tvarr ffmpeg -encoders | grep -E "vaapi|nvenc|qsv"

# Test VAAPI
docker exec tvarr ls -la /dev/dri
```

**Check:** Worker connectivity (distributed mode)

- View Transcoders page
- Check worker logs

### "High CPU/memory usage"

**Check:** Number of active streams

- Each stream consumes resources
- Transcoding uses significant CPU (or GPU)

**Reduce:** Concurrent stream limit

```bash
TVARR_RELAY_MAX_CONCURRENT_STREAMS=5
```

**Reduce:** Use hardware acceleration

**Reduce:** Lower encoding quality

### "Database locked" (SQLite)

**Cause:** Multiple processes accessing SQLite

**Fix:** Only run one tvarr instance with SQLite

**Alternative:** Switch to PostgreSQL for multi-instance

## Useful Log Patterns

### Find errors

```bash
docker logs tvarr 2>&1 | grep -iE "error|fail|panic"
```

### Track specific channel

```bash
docker logs tvarr 2>&1 | grep "channel-id-here"
```

### Watch ingestion

```bash
docker logs -f tvarr 2>&1 | grep -i ingest
```

### Watch transcoding

```bash
docker logs -f tvarr 2>&1 | grep -iE "ffmpeg|transcode|encode"
```

## Getting Help

If you can't resolve an issue:

1. Set `TVARR_LOGGING_LEVEL=debug`
2. Reproduce the issue
3. Collect relevant logs
4. Open an issue at [GitHub Issues](https://github.com/jmylchreest/tvarr/issues)

Include:
- tvarr version
- Deployment type (Docker/Kubernetes)
- Relevant configuration (redacted)
- Error logs
- Steps to reproduce
