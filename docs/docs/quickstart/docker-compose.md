---
title: Docker Compose
description: Deploy tvarr with Docker Compose
sidebar_position: 1
---

# Docker Compose

The fastest way to get started with tvarr.

## Standalone (Single Container)

For most home users, the standalone all-in-one image is recommended:

```yaml title="docker-compose.yml"
services:
  tvarr:
    image: ghcr.io/jmylchreest/tvarr:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=UTC
      - TVARR_SERVER_BASE_URL=http://your-host:8080
    volumes:
      - tvarr-data:/data
    # Intel/AMD GPU (VAAPI) - uncomment for hardware transcoding
    # devices:
    #   - /dev/dri:/dev/dri

volumes:
  tvarr-data:
```

```bash
docker compose up -d
```

Access the web UI at `http://localhost:8080`.

## Distributed Mode (Coordinator + Workers)

For larger setups or dedicated transcoding hardware:

```yaml title="docker-compose.yml"
services:
  tvarr:
    image: ghcr.io/jmylchreest/tvarr-coordinator:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=UTC
      - TVARR_GRPC_ENABLED=true
    volumes:
      - tvarr-data:/data

  ffmpegd:
    image: ghcr.io/jmylchreest/tvarr-transcoder:latest
    restart: unless-stopped
    depends_on:
      tvarr:
        condition: service_healthy
    environment:
      - TVARR_COORDINATOR_URL=tvarr:9090
      - TVARR_MAX_JOBS=4
    # Intel/AMD GPU
    devices:
      - /dev/dri:/dev/dri

volumes:
  tvarr-data:
```

## NVIDIA GPU Support

Add NVIDIA runtime to your transcoder service:

```yaml
services:
  ffmpegd:
    image: ghcr.io/jmylchreest/tvarr-transcoder:latest
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu, video, compute]
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PUID` | 1000 | User ID for file permissions |
| `PGID` | 1000 | Group ID for file permissions |
| `TZ` | UTC | Timezone |
| `TVARR_SERVER_BASE_URL` | - | External URL for generated playlists |
| `TVARR_LOGGING_LEVEL` | info | Log level (debug, info, warn, error) |
| `TVARR_DATABASE_DSN` | /data/tvarr.db | Database path |

See [Configuration](/docs/next/configuration/) for all options.
