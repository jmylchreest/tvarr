# tvarr

A self-hosted IPTV stream proxy and management service. Aggregates M3U/Xtream sources, merges EPG data, and generates unified playlists with filtering, transformation, and relay streaming capabilities.

## Quick Start (Docker)

```bash
# Basic - no hardware acceleration
docker run -d \
  -p 8080:8080 \
  -v tvarr-data:/data \
  -e TVARR_SERVER_BASE_URL=http://your-host:8080 \
  ghcr.io/jmylchreest/tvarr:release

# With Intel/AMD GPU acceleration (VAAPI)
docker run -d \
  -p 8080:8080 \
  -v tvarr-data:/data \
  --device /dev/dri:/dev/dri \
  -e TVARR_SERVER_BASE_URL=http://your-host:8080 \
  ghcr.io/jmylchreest/tvarr:release

# With NVIDIA GPU acceleration
docker run -d \
  -p 8080:8080 \
  -v tvarr-data:/data \
  --gpus all \
  -e TVARR_SERVER_BASE_URL=http://your-host:8080 \
  ghcr.io/jmylchreest/tvarr:release
```

Replace `your-host` with the hostname or IP where clients will access tvarr.

The web UI will be available at `http://localhost:8080`.

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `TVARR_SERVER_BASE_URL` | Public URL for generated playlist links | `http://localhost:8080` |
| `TVARR_SERVER_PORT` | HTTP server port | `8080` |
| `TVARR_DATABASE_DSN` | SQLite database path | `tvarr.db` |
| `TVARR_LOGGING_LEVEL` | Log level (debug, info, warn, error) | `info` |
| `PUID` | User ID for file ownership | `1000` |
| `PGID` | Group ID for file ownership | `1000` |
| `TZ` | Timezone | `UTC` |

See [docs/README-CONFIGURABLES.md](docs/README-CONFIGURABLES.md) for the complete list.

## Docker Compose

```yaml
services:
  tvarr:
    image: ghcr.io/jmylchreest/tvarr:release
    ports:
      - "8080:8080"
    volumes:
      - tvarr-data:/data
    environment:
      - TVARR_SERVER_BASE_URL=http://your-host:8080
      - PUID=1000
      - PGID=1000
      - TZ=America/New_York
    devices:
      - /dev/dri:/dev/dri  # For Intel/AMD GPU acceleration
    restart: unless-stopped

volumes:
  tvarr-data:
```

## Helm Chart (Kubernetes)

```bash
helm install tvarr ./deployment/kubernetes/helm/tvarr \
  --set env.TVARR_SERVER_BASE_URL=http://tvarr.example.com
```

See [deployment/kubernetes/helm/tvarr/values.yaml](deployment/kubernetes/helm/tvarr/values.yaml) for configuration options.

## Documentation

- [Detailed Technical Documentation](README-DETAILED.md)
- [Configuration Reference](docs/README-CONFIGURABLES.md)
- [API Changes](API-CHANGES.md)
