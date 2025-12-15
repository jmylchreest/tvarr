# tvarr Application Docker Image
# Uses Taskfile for consistent builds between local and container environments
#
# Build: docker build -f deployment/docker/Dockerfile -t ghcr.io/jmylchreest/tvarr:latest .
# Run: docker run -p 8080:8080 -v tvarr-data:/data ghcr.io/jmylchreest/tvarr:latest

ARG FFMPEG_IMAGE=ghcr.io/jmylchreest/tvarr-ffmpeg:latest

# =============================================================================
# Stage 1a: Build gosu from PKGBUILD
# =============================================================================
FROM archlinux:base AS gosu-builder

RUN pacman-key --init && pacman -Syu --noconfirm && \
    pacman -S --noconfirm base-devel go sudo

RUN useradd -m builder && \
    echo "builder ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers

COPY deployment/docker/pkgbuild/gosu /home/builder/pkgbuild-gosu
RUN chown -R builder:builder /home/builder/pkgbuild-gosu

USER builder
WORKDIR /home/builder/pkgbuild-gosu
RUN makepkg -s --noconfirm

# =============================================================================
# Stage 1b: Build diagnostic utilities using alternative root
# =============================================================================
FROM archlinux:base AS utils-builder

# Configure pacman to exclude unnecessary files
RUN echo -e '\n\
NoExtract = usr/share/doc/*\n\
NoExtract = usr/share/man/*\n\
NoExtract = usr/share/info/*\n\
NoExtract = usr/share/locale/*\n\
NoExtract = usr/share/i18n/*\n\
NoExtract = usr/share/help/*\n\
NoExtract = usr/share/gtk-doc/*\n\
NoExtract = usr/share/licenses/*\n\
NoExtract = usr/lib/python*/*' >> /etc/pacman.conf

RUN pacman-key --init && pacman -Syu --noconfirm

# Set up local repo with gosu package
RUN mkdir -p /localrepo
COPY --from=gosu-builder /home/builder/pkgbuild-gosu/*.pkg.tar.zst /localrepo/
RUN repo-add /localrepo/localrepo.db.tar.gz /localrepo/*.pkg.tar.zst && \
    echo -e '\n[localrepo]\nSigLevel = Optional TrustAll\nServer = file:///localrepo' >> /etc/pacman.conf

# Create minimal rootfs with curl, yq, and gosu
# Note: We use go-yq (Go binary) which handles YAML/JSON/XML/TOML
RUN mkdir -p /utilsroot/var/lib/pacman && \
    pacman -Sy --noconfirm --needed -r /utilsroot \
    --dbpath /utilsroot/var/lib/pacman \
    --cachedir /var/cache/pacman/pkg \
    curl go-yq gosu && \
    rm -rf /utilsroot/var/cache/pacman/pkg/* /utilsroot/var/lib/pacman/*

# =============================================================================
# Stage 2: Build tvarr using Taskfile
# =============================================================================
FROM golang:latest AS builder

WORKDIR /src

# Install Node.js LTS, pnpm, Task, and UPX
RUN curl -fsSL https://deb.nodesource.com/setup_lts.x | bash - && \
    apt-get install -y --no-install-recommends nodejs upx && \
    npm install -g pnpm && \
    sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d -b /usr/local/bin && \
    rm -rf /var/lib/apt/lists/*

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy frontend package files for layer caching
COPY frontend/package.json frontend/pnpm-lock.yaml* ./frontend/

# Copy Taskfile early so we can use it
COPY Taskfile.yml ./

# Install frontend dependencies
RUN task frontend:install

# Copy all source code
COPY . .

# Build arguments for version injection
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ARG BRANCH=unknown
ARG TREE_STATE=clean

# Set version info as environment variables for Task
# Use TVARR_BUILD_* prefix to avoid conflicts with Taskfile variable names
ENV TVARR_BUILD_VERSION=${VERSION} \
    TVARR_BUILD_COMMIT=${COMMIT} \
    TVARR_BUILD_DATE=${BUILD_DATE} \
    TVARR_BUILD_BRANCH=${BRANCH} \
    TVARR_BUILD_TREE_STATE=${TREE_STATE}

# Build frontend and embed into assets
# CI=true is required for pnpm to run without TTY
RUN CI=true task frontend:embed

# Build release binary (uses LDFLAGS from Taskfile)
# CGO_ENABLED=1 is required for go-sqlite3
# Debug: show env vars being passed to task
RUN echo "DEBUG: TVARR_BUILD_VERSION=$TVARR_BUILD_VERSION" && \
    GOOS=linux task build && mv dist/debug/tvarr /tvarr

# Compress with UPX
RUN task upx:compress:file FILE=/tvarr

# =============================================================================
# Stage 3: Runtime image based on FFmpeg base
# =============================================================================
FROM ${FFMPEG_IMAGE} AS runtime

LABEL org.opencontainers.image.title="tvarr" \
      org.opencontainers.image.description="IPTV stream management with hardware-accelerated transcoding" \
      org.opencontainers.image.source="https://github.com/jmylchreest/tvarr" \
      org.opencontainers.image.licenses="MIT"

# Copy diagnostic utilities from alternative root build (curl, jq, yq)
# This overlays the binaries and their libraries without package database bloat
# The base FFmpeg image provides core libs (glibc, gcc-libs, etc.)
COPY --from=utils-builder /utilsroot/usr/bin/ /usr/bin/
COPY --from=utils-builder /utilsroot/usr/lib/ /usr/lib/

# Copy compressed tvarr binary (frontend is embedded)
COPY --from=builder /tvarr /app/tvarr

# Copy entrypoint script
COPY deployment/docker/scripts/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Create data directory
RUN mkdir -p /data && chown -R tvarr:tvarr /data

# Environment defaults
ENV PUID=1000 \
    PGID=1000 \
    TZ=UTC \
    TVARR_SERVER_PORT=8080 \
    TVARR_DATABASE_DSN=file:/data/tvarr.db \
    TVARR_LOGGING_LEVEL=info \
    TVARR_STORAGE_BASE_DIR=/data \
    TVARR_FFMPEG_BINARY_PATH=/usr/bin/ffmpeg \
    TVARR_FFMPEG_PROBE_PATH=/usr/bin/ffprobe

# Volume for persistent data
VOLUME ["/data"]

# Expose HTTP port
EXPOSE 8080

# Health check (Docker uses /health, K8s uses /livez and /readyz)
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD curl -sf http://localhost:${TVARR_SERVER_PORT:-8080}/health || exit 1

# Entrypoint handles user setup and signal handling
ENTRYPOINT ["/entrypoint.sh"]
