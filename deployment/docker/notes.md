# tvarr Docker Container Build

This directory contains Docker build configuration for tvarr and its dependencies.

## Directory Structure

```
deployment/docker/
├── Dockerfile           # tvarr application image (uses tvarr-ffmpeg as base)
├── Dockerfile.ffmpeg    # FFmpeg base image with hardware acceleration
├── pkgbuild/
│   ├── ffmpeg/         # FFmpeg PKGBUILD for custom minimal build
│   │   └── PKGBUILD
│   └── gosu/           # gosu PKGBUILD for privilege dropping
│       └── PKGBUILD
├── scripts/
│   └── entrypoint.sh   # Container entrypoint with PUID/PGID support
└── notes.md            # This file
```

## tvarr-ffmpeg Image

Minimal FFmpeg container with hardware acceleration support for AMD, Intel, and NVIDIA GPUs.

### Build Approach

Three-stage Docker build using Arch Linux (rolling release):

1. **Builder stage**: Compiles custom `tvarr-ffmpeg` package via PKGBUILD
2. **Rootfs-builder stage**: Creates minimal rootfs with NoExtract rules
3. **Runtime stage**: FROM scratch with just the rootfs

### Custom PKGBUILD

The `pkgbuild/ffmpeg/PKGBUILD` builds a minimal FFmpeg with only required features:
- Video codecs: x264, x265, VP8/VP9, Opus, Vorbis
- Hardware acceleration: VAAPI, Vulkan, NVENC/NVDEC
- Network: gnutls for HTTPS
- Dependencies include Mesa, Intel, and AMD drivers

### NoExtract Optimization

`pacman.conf` NoExtract rules prevent installation of:
- Documentation (man, info, doc)
- Locales and i18n
- Python bindings
- Static libraries
- Development headers

### Image Size

~720 MB due to:
- LLVM (~150MB) - Required by Mesa for AMD radeonsi shader compilation
- FFmpeg + minimal codecs (~50MB)
- Mesa + VAAPI drivers (~150MB)
- Intel drivers (~100MB)
- Vulkan drivers (~50MB)
- Core system libs (~200MB)

## Hardware Acceleration

### AMD GPUs (VAAPI)
```bash
podman run --device /dev/dri ghcr.io/jmylchreest/tvarr-ffmpeg:latest vainfo
```
Encoders: `h264_vaapi`, `hevc_vaapi`, `av1_vaapi` (RDNA 3+), `vp9_vaapi`

### Intel GPUs (VAAPI/QSV)
```bash
podman run --device /dev/dri ghcr.io/jmylchreest/tvarr-ffmpeg:latest vainfo
```
Encoders: `h264_vaapi`, `hevc_vaapi`, `h264_qsv`, `hevc_qsv`, `av1_qsv` (Arc)

### NVIDIA GPUs (NVENC/NVDEC)
```bash
podman run --gpus all ghcr.io/jmylchreest/tvarr-ffmpeg:latest ffmpeg -hwaccels
```
Encoders: `h264_nvenc`, `hevc_nvenc`, `av1_nvenc` (RTX 40+)

NVIDIA requires host drivers and `nvidia-container-toolkit` configured.

## Building

```bash
# Build minimal runtime image
task container:build:ffmpeg

# Build with full base system (for debugging)
podman build -f deployment/docker/Dockerfile.ffmpeg --target full-build -t tvarr-ffmpeg .

# Force rebuild without cache
task container:build:ffmpeg NO_CACHE=true
```

## tvarr Application Image

The main tvarr image is built on top of tvarr-ffmpeg and includes:
- tvarr Go application binary (UPX compressed)
- gosu for privilege dropping (PUID/PGID support)
- curl and yq for diagnostics

### Build Stages

1. **gosu-builder**: Compiles gosu from PKGBUILD
2. **utils-builder**: Creates minimal rootfs with curl, yq, gosu
3. **builder**: Compiles tvarr using Taskfile (Go + embedded frontend)
4. **runtime**: Based on tvarr-ffmpeg with tvarr binary and utilities

## Updating Dependencies

Dependencies are automatically checked weekly via GitHub Actions (Monday 06:00 UTC).
Manual updates can be done using:

```bash
# Check for updates
./scripts/update-dependencies.sh --check

# Apply updates
./scripts/update-dependencies.sh --update

# Update specific component
./scripts/update-dependencies.sh --update --component=ffmpeg
./scripts/update-dependencies.sh --update --component=gosu
```

The script updates:
- `pkgbuild/ffmpeg/PKGBUILD`: FFmpeg version from git.ffmpeg.org tags
- `pkgbuild/gosu/PKGBUILD`: gosu version and sha256sum from GitHub releases

## GitHub Actions Workflows

- **docker-ffmpeg.yml**: Builds tvarr-ffmpeg (Sunday midnight UTC, or on pkgbuild changes)
- **docker-tvarr.yml**: Builds tvarr (on push to main or tags)
- **dependency-updates.yml**: Checks for updates and creates PRs (Monday 06:00 UTC)

## Image Tags

### tvarr-ffmpeg
- `ghcr.io/jmylchreest/tvarr-ffmpeg:YYYY.MM.DD-N` - Specific build
- `ghcr.io/jmylchreest/tvarr-ffmpeg:latest` - Latest build
- `ghcr.io/jmylchreest/tvarr-ffmpeg:amd64` / `:arm64` - Architecture-specific

### tvarr
- `ghcr.io/jmylchreest/tvarr:X.Y.Z` - Semantic version (releases)
- `ghcr.io/jmylchreest/tvarr:latest` - Latest build from main
- `ghcr.io/jmylchreest/tvarr:amd64` / `:arm64` - Architecture-specific
