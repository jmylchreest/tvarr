# tvarr Helm Chart

Deploy tvarr to Kubernetes with optional hardware acceleration support.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.7+ (for OCI support)
- PV provisioner (for persistence)
- (Optional) NVIDIA device plugin for GPU support
- (Optional) Intel device plugin for Intel GPU support

## Installation

### From OCI Registry (Recommended)

```bash
# Install latest stable release
helm install tvarr oci://ghcr.io/jmylchreest/charts/tvarr

# Install with custom values
helm install tvarr oci://ghcr.io/jmylchreest/charts/tvarr -f my-values.yaml

# Install with custom base URL
helm install tvarr oci://ghcr.io/jmylchreest/charts/tvarr \
  --set env.TVARR_SERVER_BASE_URL=https://tvarr.example.com
```

### From Source

```bash
git clone https://github.com/jmylchreest/tvarr.git
cd tvarr
helm install tvarr ./deployment/kubernetes/helm/tvarr
```

### Using a Snapshot Build

To use a development snapshot instead of the stable release:

```bash
helm install tvarr oci://ghcr.io/jmylchreest/charts/tvarr \
  --set image.tag=0.0.21-dev.10-bc64e8e
```

See [GitHub Releases](https://github.com/jmylchreest/tvarr/releases) for available snapshot versions.

## Versioning

The chart uses two versions:
- **Chart version**: Tracks Helm chart changes (incremented on chart updates)
- **appVersion**: Matches the latest stable tvarr release (used as default image tag)

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PUID` | `1000` | User ID for file permissions |
| `PGID` | `1000` | Group ID for file permissions |
| `TZ` | `UTC` | Timezone (e.g., `America/New_York`) |
| `TVARR_SERVER_PORT` | `8080` | HTTP server port |
| `TVARR_SERVER_BASE_URL` | `` | External base URL for M3U proxy URLs (required behind ingress) |
| `TVARR_DATABASE_DSN` | `/data/tvarr.db` | Database connection string |
| `TVARR_LOGGING_LEVEL` | `info` | Logging level (debug, info, warn, error) |
| `TVARR_FFMPEG_BINARY_PATH` | `/usr/bin/ffmpeg` | FFmpeg binary path |
| `TVARR_FFMPEG_PROBE_PATH` | `/usr/bin/ffprobe` | FFprobe binary path |

### Values

See [values.yaml](values.yaml) for all configurable options.

#### Basic Configuration

```yaml
# Override image tag for snapshots
# image:
#   tag: "0.0.21-dev.10-bc64e8e"

service:
  type: ClusterIP
  port: 8080

persistence:
  enabled: true
  size: 10Gi
  storageClass: ""

env:
  TVARR_SERVER_BASE_URL: "https://tvarr.example.com"
```

#### GPU Support

##### Intel GPU (VAAPI/QSV)

```yaml
gpu:
  enabled: true
  type: intel

# Requires intel-gpu-plugin in cluster
resources:
  limits:
    gpu.intel.com/i915: 1
```

##### NVIDIA GPU (NVENC)

```yaml
gpu:
  enabled: true
  type: nvidia

# Requires nvidia-device-plugin in cluster
resources:
  limits:
    nvidia.com/gpu: 1
```

##### AMD GPU (VAAPI)

```yaml
gpu:
  enabled: true
  type: amd
```

### Ingress

```yaml
ingress:
  enabled: true
  className: nginx
  hosts:
    - host: tvarr.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: tvarr-tls
      hosts:
        - tvarr.example.com
```

## Uninstallation

```bash
helm uninstall tvarr
```

## Troubleshooting

### View Logs

```bash
kubectl logs -f deployment/tvarr
```

### Check Hardware Acceleration

```bash
kubectl exec -it deployment/tvarr -- ffmpeg -hide_banner -hwaccels
kubectl exec -it deployment/tvarr -- ffmpeg -hide_banner -encoders | grep -E "(nvenc|vaapi|qsv)"
```
