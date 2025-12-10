# tvarr Helm Chart

Deploy tvarr to Kubernetes with optional hardware acceleration support.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- PV provisioner (for persistence)
- (Optional) NVIDIA device plugin for GPU support
- (Optional) Intel device plugin for Intel GPU support

## Installation

```bash
# Add the Helm repository (if published)
helm repo add tvarr https://jmylchreest.github.io/tvarr
helm repo update

# Install with default values
helm install tvarr tvarr/tvarr

# Install from local chart
helm install tvarr ./deployment/kubernetes/helm/tvarr

# Install with custom values
helm install tvarr tvarr/tvarr -f my-values.yaml
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PUID` | `1000` | User ID for file permissions |
| `PGID` | `1000` | Group ID for file permissions |
| `TZ` | `UTC` | Timezone (e.g., `America/New_York`) |
| `TVARR_PORT` | `8080` | HTTP server port |
| `TVARR_DATABASE_DSN` | `file:/data/tvarr.db` | Database connection string |
| `TVARR_LOG_LEVEL` | `info` | Logging level (debug, info, warn, error) |
| `TVARR_CONFIG_DIR` | `/data/config` | Configuration directory |
| `TVARR_FFMPEG_PATH` | `/usr/bin/ffmpeg` | FFmpeg binary path |
| `TVARR_FFPROBE_PATH` | `/usr/bin/ffprobe` | FFprobe binary path |
| `TVARR_PREFLIGHT` | `false` | Run pre-flight diagnostics and exit |

### Values

See [values.yaml](values.yaml) for all configurable options.

#### Basic Configuration

```yaml
image:
  repository: ghcr.io/jmylchreest/tvarr
  tag: latest
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 8080

persistence:
  enabled: true
  size: 10Gi
  storageClass: ""
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

### Pre-flight Check

Run the pre-flight check to verify hardware acceleration:

```bash
kubectl exec -it deployment/tvarr -- /bin/sh -c "TVARR_PREFLIGHT=true /entrypoint.sh"
```

### View Logs

```bash
kubectl logs -f deployment/tvarr
```

### Check Hardware Acceleration

```bash
kubectl exec -it deployment/tvarr -- ffmpeg -hide_banner -hwaccels
kubectl exec -it deployment/tvarr -- ffmpeg -hide_banner -encoders | grep -E "(nvenc|vaapi|qsv)"
```
