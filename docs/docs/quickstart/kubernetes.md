---
title: Kubernetes
description: Deploy tvarr on Kubernetes with Helm
sidebar_position: 2
---

# Kubernetes

Deploy tvarr using the official Helm chart.

## Prerequisites

- Kubernetes cluster
- Helm 3.x installed
- `kubectl` configured

## Quick Install

### Option 1: OCI Registry (Recommended)

```bash
# Install directly from GitHub Container Registry
helm install tvarr oci://ghcr.io/jmylchreest/charts/tvarr
```

### Option 2: From Source

```bash
git clone https://github.com/jmylchreest/tvarr.git
cd tvarr
helm install tvarr ./deployment/kubernetes/helm/tvarr
```

## Example Values

```yaml title="values.yaml"
# Basic configuration
replicaCount: 1

# Image - defaults to appVersion from chart
# Override tag for snapshots or specific versions:
# image:
#   tag: "0.0.21-dev.9-b37ba0a"  # Snapshot
#   tag: "0.0.20"                 # Specific stable

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

# Environment
env:
  TVARR_SERVER_BASE_URL: "https://tvarr.example.com"
  TVARR_LOGGING_LEVEL: "info"

# Intel GPU support
gpu:
  enabled: true
  type: intel
  intel:
    devicePath: /dev/dri

# Distributed transcoding (optional)
ffmpegd:
  enabled: true
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

## Distributed Transcoding

Enable the ffmpegd workers for distributed transcoding:

```yaml
ffmpegd:
  enabled: true
  replicaCount: 3

  # Scale across nodes with different GPUs
  affinity:
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
        - weight: 100
          podAffinityTerm:
            labelSelector:
              matchLabels:
                app.kubernetes.io/component: ffmpegd
            topologyKey: kubernetes.io/hostname

  # NVIDIA GPU nodes
  gpu:
    enabled: true
    type: nvidia
    nvidia:
      runtimeClassName: nvidia
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
