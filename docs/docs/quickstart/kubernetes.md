---
title: Kubernetes
description: Deploy tvarr on Kubernetes with Helm
sidebar_position: 2
---

# Kubernetes

Deploy tvarr using the official Helm chart.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.7+ (for OCI registry support)
- `kubectl` configured
- PV provisioner (for persistence)
- (Optional) NVIDIA device plugin for GPU support
- (Optional) Intel device plugin for Intel GPU support

## Quick Install

### Option 1: OCI Registry (Recommended)

```bash
# Install directly from GitHub Container Registry
helm install tvarr oci://ghcr.io/jmylchreest/charts/tvarr \
  --set env.TVARR_SERVER_BASE_URL=https://tvarr.example.com

# Install with custom values file
helm install tvarr oci://ghcr.io/jmylchreest/charts/tvarr -f my-values.yaml
```

### Option 2: From Source

```bash
git clone https://github.com/jmylchreest/tvarr.git
cd tvarr
helm install tvarr ./deployment/kubernetes/helm/tvarr \
  --set env.TVARR_SERVER_BASE_URL=https://tvarr.example.com
```

:::warning Base URL Required Behind Ingress
When deploying behind an ingress controller, you **must** set `TVARR_SERVER_BASE_URL` to your external URL. Without this, generated M3U playlist URLs will point to the internal container address and won't work for clients outside the cluster.
:::

## Environment Variables

All environment variables are set via the `env` map in values.yaml:

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

## Deployment Modes

The chart supports two deployment modes:

| Mode | Coordinator Image | Transcoder Pods | Use Case |
|------|------------------|-----------------|----------|
| **`aio`** (default) | `ghcr.io/jmylchreest/tvarr` | Optional | Single container with local FFmpeg. Can optionally add remote transcoder workers. |
| **`distributed`** | `ghcr.io/jmylchreest/tvarr-coordinator` | Required (1+) | Lightweight coordinator (no FFmpeg) with separate transcoder worker pods. |

## Example Values

### All-in-One Mode (Default)

```yaml title="values-aio.yaml"
# mode: aio  # default, can be omitted

# Persistence (required for SQLite)
persistence:
  enabled: true
  size: 10Gi

# Ingress
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

# Environment - set your external URL
env:
  TVARR_SERVER_BASE_URL: "https://tvarr.example.com"
  TVARR_LOGGING_LEVEL: "info"

# Intel GPU support (for local transcoding)
gpu:
  enabled: true
  type: intel
  intel:
    devicePath: /dev/dri
```

### Distributed Mode

```yaml title="values-distributed.yaml"
mode: distributed

persistence:
  enabled: true
  size: 10Gi

ingress:
  enabled: true
  className: nginx
  hosts:
    - host: tvarr.example.com
      paths:
        - path: /
          pathType: Prefix

env:
  TVARR_SERVER_BASE_URL: "https://tvarr.example.com"

# Transcoder workers (automatically enabled in distributed mode)
transcoder:
  replicaCount: 2
  gpu:
    enabled: true
    type: intel
```

```bash
helm install tvarr oci://ghcr.io/jmylchreest/charts/tvarr -f values.yaml
```

## Versioning

The chart uses two versions:
- **Chart version**: Tracks Helm chart changes (incremented on chart updates)
- **appVersion**: Matches the latest stable tvarr release (used as default image tag)

### Using Snapshot Builds

To use a development snapshot instead of the stable release:

```yaml
image:
  tag: "0.0.21-dev.9-b37ba0a"  # Check GitHub Releases for available snapshots
```

## GPU Support

### Intel GPU (VAAPI/QSV)

```yaml
gpu:
  enabled: true
  type: intel

# Requires intel-gpu-plugin in cluster
resources:
  limits:
    gpu.intel.com/i915: 1
```

### NVIDIA GPU (NVENC)

```yaml
gpu:
  enabled: true
  type: nvidia

# Requires nvidia-device-plugin in cluster
resources:
  limits:
    nvidia.com/gpu: 1
```

### AMD GPU (VAAPI)

```yaml
gpu:
  enabled: true
  type: amd
```

## Distributed Transcoding

Enable transcoder workers for distributed transcoding. The coordinator must have gRPC enabled (it is by default via `TVARR_GRPC_ENABLED: "true"`).

In `distributed` mode, transcoders are always deployed. In `aio` mode, set `transcoder.enabled: true` to add them alongside the all-in-one coordinator.

```yaml
transcoder:
  enabled: true  # not needed in distributed mode
  replicaCount: 3

  # Spread across nodes
  topologySpreadConstraints:
    - maxSkew: 1
      topologyKey: "kubernetes.io/hostname"
      whenUnsatisfiable: ScheduleAnyway
      labelSelector:
        matchLabels:
          app.kubernetes.io/component: transcoder

  # NVIDIA GPU nodes
  gpu:
    enabled: true
    type: nvidia
    nvidia:
      runtimeClassName: nvidia
```

Transcoder workers automatically connect to the coordinator via the gRPC service. You can configure worker-specific settings:

```yaml
transcoder:
  env:
    TVARR_MAX_JOBS: "4"          # Max concurrent transcoding jobs per worker
    TVARR_AUTH_TOKEN: ""          # Authentication token (must match coordinator TVARR_GRPC_AUTH_TOKEN)
```

## Raw Manifests

If you prefer raw Kubernetes manifests, they're available at:

```bash
kubectl apply -f https://raw.githubusercontent.com/jmylchreest/tvarr/main/deployment/kubernetes/manifests.yaml
```

Or clone and customize:

```bash
git clone https://github.com/jmylchreest/tvarr.git
cd tvarr/deployment/kubernetes
kubectl apply -f manifests.yaml
```

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

# Previous container (if crashed)
kubectl logs deploy/tvarr --previous
```

### Check Hardware Acceleration

```bash
# Only available in AIO mode (coordinator has no FFmpeg in distributed mode)
kubectl exec -it deployment/tvarr -- ffmpeg -hide_banner -hwaccels
kubectl exec -it deployment/tvarr -- ffmpeg -hide_banner -encoders | grep -E "(nvenc|vaapi|qsv)"
```
