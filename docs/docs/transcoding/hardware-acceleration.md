---
title: Hardware Acceleration
description: GPU-accelerated encoding with VAAPI, NVENC, QSV, and AMF
sidebar_position: 3
---

# Hardware Acceleration

Use your GPU for faster, more efficient transcoding.

## Supported Hardware

| Type | GPUs | Requires |
|------|------|----------|
| **VAAPI** | Intel, AMD | `/dev/dri` device access |
| **NVENC** | NVIDIA | NVIDIA driver + runtime |
| **QSV** | Intel | Intel Media SDK |
| **AMF** | AMD | AMD drivers |

## Docker Setup

### Intel / AMD (VAAPI)

Pass the DRI device to your container:

```yaml
services:
  tvarr:
    image: ghcr.io/jmylchreest/tvarr:latest
    devices:
      - /dev/dri:/dev/dri
```

Verify access:

```bash
docker exec tvarr ls -la /dev/dri
# Should show renderD128, card0, etc.
```

### NVIDIA (NVENC)

Use the NVIDIA container runtime:

```yaml
services:
  tvarr:
    image: ghcr.io/jmylchreest/tvarr:latest
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu, video, compute]
```

Or with the legacy runtime flag:

```yaml
services:
  tvarr:
    image: ghcr.io/jmylchreest/tvarr:latest
    runtime: nvidia
    environment:
      - NVIDIA_VISIBLE_DEVICES=all
      - NVIDIA_DRIVER_CAPABILITIES=video,compute
```

## Kubernetes Setup

### Intel GPU

```yaml
spec:
  containers:
    - name: tvarr
      resources:
        limits:
          gpu.intel.com/i915: 1
      volumeMounts:
        - name: dri
          mountPath: /dev/dri
  volumes:
    - name: dri
      hostPath:
        path: /dev/dri
```

### NVIDIA GPU

```yaml
spec:
  runtimeClassName: nvidia
  containers:
    - name: tvarr
      resources:
        limits:
          nvidia.com/gpu: 1
```

## Verifying Hardware

Check detected hardware in the logs:

```bash
docker logs tvarr 2>&1 | grep -i "gpu\|vaapi\|nvenc\|qsv"
```

Or visit the **Transcoders** page to see detected capabilities.

## Encoder Priority

When multiple encoders are available, tvarr prefers:

1. GPU hardware encoder (fastest, lowest CPU)
2. CPU software encoder (universal fallback)

You can override this per-profile with encoder overrides.

## Session Limits

GPUs have encoding session limits:

| GPU | Max Sessions |
|-----|--------------|
| NVIDIA Consumer | 3-5 |
| NVIDIA Quadro/Pro | Unlimited |
| Intel (recent) | ~20 |
| AMD | Varies |

tvarr auto-detects these limits and won't exceed them.

## Troubleshooting

### VAAPI Not Working

Check render device permissions:

```bash
# Inside container
ls -la /dev/dri/renderD128
# Should be readable/writable

# Check user group
id
# Should include 'video' or 'render' group
```

### NVENC "Too Many Sessions"

You've hit the session limit. Options:

1. Use fewer concurrent streams
2. Use a Quadro/professional GPU
3. Apply the [NVENC patch](https://github.com/keylase/nvidia-patch)

### "No Hardware Encoder Found"

Ensure:

1. Device is passed to container correctly
2. Drivers are installed on host
3. Container user has access to device
