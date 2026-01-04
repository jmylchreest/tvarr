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

```bash
# Add the tvarr Helm repository
helm repo add tvarr https://jmylchreest.github.io/tvarr/charts
helm repo update

# Install with default values
helm install tvarr tvarr/tvarr
```

## Example Values

```yaml title="values.yaml"
# Basic configuration
replicaCount: 1

image:
  repository: ghcr.io/jmylchreest/tvarr
  tag: latest

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
helm install tvarr tvarr/tvarr -f values.yaml
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
