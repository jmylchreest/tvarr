---
title: Storage
description: File storage configuration
sidebar_position: 3
---

# Storage

tvarr stores data in several directories.

## Directory Structure

```
/data/
├── tvarr.db          # SQLite database (if using SQLite)
├── logos/            # Cached channel logos
├── output/           # Generated M3U/XMLTV files
├── temp/             # Temporary files
└── themes/           # Custom UI themes
```

## Configuration

```bash
# Base directory (all others relative to this)
TVARR_STORAGE_BASE_DIR=/data

# Override individual directories
TVARR_STORAGE_LOGO_DIR=/data/logos
TVARR_STORAGE_OUTPUT_DIR=/data/output
TVARR_STORAGE_TEMP_DIR=/data/temp
```

## Logo Cache

Channel logos are downloaded and cached locally:

```bash
# How long to keep cached logos
TVARR_STORAGE_LOGO_RETENTION=720h  # 30 days

# Maximum logo file size
TVARR_STORAGE_MAX_LOGO_SIZE=5242880  # 5MB
```

Logos are stored with SHA256-based names and include HTTP cache headers.

## Docker Volumes

When using Docker, mount the data directory:

```yaml
volumes:
  - tvarr-data:/data
```

Or bind mount to host:

```yaml
volumes:
  - /path/on/host:/data
```

## Permissions

Ensure the container user can read/write the data directory:

```bash
# Set ownership
chown -R 1000:1000 /path/to/data

# Or set PUID/PGID in container
environment:
  - PUID=1000
  - PGID=1000
```
