# tvarr Helm Chart

Deploy tvarr to Kubernetes with optional hardware acceleration and distributed transcoding.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.7+ (for OCI support)
- PV provisioner (for persistence)
- (Optional) NVIDIA device plugin for GPU support
- (Optional) Intel device plugin for Intel GPU support

## Deployment Modes

The chart supports two deployment modes controlled by the `mode` value:

| Mode | Coordinator Image | Transcoder Pods | Use Case |
|------|------------------|-----------------|----------|
| **`aio`** (default) | `ghcr.io/jmylchreest/tvarr` | Optional | Single container with local FFmpeg. Can optionally add remote transcoder workers. |
| **`distributed`** | `ghcr.io/jmylchreest/tvarr-coordinator` | Required (1+) | Lightweight coordinator (no FFmpeg) with separate transcoder worker pods. |

## Installation

### From OCI Registry (Recommended)

```bash
# All-in-one mode (default)
helm install tvarr oci://ghcr.io/jmylchreest/charts/tvarr \
  --set env.TVARR_SERVER_BASE_URL=https://tvarr.example.com

# Distributed mode (coordinator + transcoder workers)
helm install tvarr oci://ghcr.io/jmylchreest/charts/tvarr \
  --set mode=distributed \
  --set env.TVARR_SERVER_BASE_URL=https://tvarr.example.com \
  --set transcoder.replicaCount=2

# Install with custom values
helm install tvarr oci://ghcr.io/jmylchreest/charts/tvarr -f my-values.yaml
```

### From Source

```bash
git clone https://github.com/jmylchreest/tvarr.git
cd tvarr
helm install tvarr ./deployment/kubernetes/helm/tvarr \
  --set env.TVARR_SERVER_BASE_URL=https://tvarr.example.com
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

All environment variables are set via the `env` map in values.yaml.

| Variable | Default | Description |
|----------|---------|-------------|
| `PUID` | `1000` | User ID for file permissions |
| `PGID` | `1000` | Group ID for file permissions |
| `TZ` | `UTC` | Timezone (e.g., `America/New_York`) |
| `TVARR_SERVER_PORT` | `8080` | HTTP server port |
| `TVARR_SERVER_BASE_URL` | `` | External base URL for M3U proxy URLs (**required** behind ingress) |
| `TVARR_GRPC_PORT` | `9090` | gRPC server port (for transcoder connections) |
| `TVARR_GRPC_ENABLED` | `true` | Enable gRPC server (required for transcoders) |
| `TVARR_DATABASE_DSN` | `/data/tvarr.db` | Database connection string |
| `TVARR_LOGGING_LEVEL` | `info` | Logging level (debug, info, warn, error) |
| `TVARR_STORAGE_BASE_DIR` | `/data` | Storage base directory for data files |
| `TVARR_FFMPEG_BINARY_PATH` | `/usr/bin/ffmpeg` | FFmpeg binary path (aio mode only) |
| `TVARR_FFMPEG_PROBE_PATH` | `/usr/bin/ffprobe` | FFprobe binary path (aio mode only) |

Additional environment variables can be passed via the `extraEnv` list for secrets or other config not covered above.

### Values

See [values.yaml](values.yaml) for all configurable options.

#### All-in-One Mode (Default)

```yaml
mode: aio  # default

env:
  TVARR_SERVER_BASE_URL: "https://tvarr.example.com"

persistence:
  enabled: true
  size: 10Gi
```

#### Distributed Mode

```yaml
mode: distributed

env:
  TVARR_SERVER_BASE_URL: "https://tvarr.example.com"

persistence:
  enabled: true
  size: 10Gi

# Transcoder workers (automatically enabled in distributed mode)
transcoder:
  replicaCount: 2
  gpu:
    enabled: true
    type: intel
```

#### AIO + Remote Transcoders

You can also use the AIO image (which has local FFmpeg) and add remote transcoders for additional capacity:

```yaml
mode: aio

env:
  TVARR_SERVER_BASE_URL: "https://tvarr.example.com"

transcoder:
  enabled: true
  replicaCount: 3
  gpu:
    enabled: true
    type: nvidia
    nvidia:
      runtimeClassName: nvidia
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

### Transcoder Configuration

Transcoder workers connect to the coordinator via gRPC automatically. Configure worker-specific settings:

```yaml
transcoder:
  replicaCount: 3
  env:
    TVARR_MAX_JOBS: "4"            # Max concurrent transcoding jobs per worker
    TVARR_AUTH_TOKEN: ""            # Must match coordinator TVARR_GRPC_AUTH_TOKEN
  resources:
    limits:
      cpu: 4000m
      memory: 4Gi
      gpu.intel.com/i915: "1"
    requests:
      cpu: 500m
      memory: 512Mi
      gpu.intel.com/i915: "1"

  # Spread across nodes
  topologySpreadConstraints:
    - maxSkew: 1
      topologyKey: "kubernetes.io/hostname"
      whenUnsatisfiable: ScheduleAnyway
      labelSelector:
        matchLabels:
          app.kubernetes.io/component: transcoder

  # Pod disruption budget
  podDisruptionBudget:
    enabled: true
    minAvailable: 1
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

## Upgrading from v0.3.x

The `ffmpegd` values key has been renamed to `transcoder`. The legacy `ffmpegd.enabled` value still works for backwards compatibility but `transcoder` is preferred:

```yaml
# Old (still works)
ffmpegd:
  enabled: true

# New (preferred)
transcoder:
  enabled: true
```

The transcoder image has also been corrected from `tvarr-ffmpegd` to `tvarr-transcoder` (the actual published image name).

## Uninstallation

```bash
helm uninstall tvarr
```

## Troubleshooting

### View Logs

```bash
# Coordinator logs
kubectl logs -f deployment/tvarr

# Transcoder logs
kubectl logs -f deployment/tvarr-transcoder
```

### Check Hardware Acceleration

```bash
# Only available in AIO mode (coordinator has no FFmpeg in distributed mode)
kubectl exec -it deployment/tvarr -- ffmpeg -hide_banner -hwaccels
kubectl exec -it deployment/tvarr -- ffmpeg -hide_banner -encoders | grep -E "(nvenc|vaapi|qsv)"
```
