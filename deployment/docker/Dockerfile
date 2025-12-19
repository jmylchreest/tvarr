# tvarr: Full-Featured IPTV Stream Management
# Based on tvarr-ffmpeg image with local transcoding support
#
# Build: docker build -f deployment/docker/Dockerfile -t ghcr.io/jmylchreest/tvarr:latest .
# Run: docker run -p 8080:8080 -v tvarr-data:/data ghcr.io/jmylchreest/tvarr:latest
#
# This is the default tvarr image with:
# - Full FFmpeg with hardware acceleration (VA-API, NVENC, QSV)
# - Local transcoding capability via internal FFmpeg wrapper
#
# For a lightweight coordinator-only image (no local FFmpeg), use:
#   ghcr.io/jmylchreest/tvarr-coordinator
#
# For a standalone transcoding daemon image (distributed transcoding), use:
#   ghcr.io/jmylchreest/tvarr-ffmpegd

ARG FFMPEG_IMAGE=ghcr.io/jmylchreest/tvarr-ffmpeg:latest

# =============================================================================
# Stage 1: Build tvarr using Taskfile
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

# Build tvarr binary
# CGO_ENABLED=1 is required for go-sqlite3
RUN GOOS=linux task build && mv dist/debug/tvarr /tvarr

# Compress binary with UPX
RUN task upx:compress:file FILE=/tvarr

# =============================================================================
# Stage 2: Runtime image based on FFmpeg base
# =============================================================================
FROM ${FFMPEG_IMAGE} AS runtime

LABEL org.opencontainers.image.title="tvarr" \
      org.opencontainers.image.description="IPTV stream management with transcoding support" \
      org.opencontainers.image.source="https://github.com/jmylchreest/tvarr" \
      org.opencontainers.image.licenses="MIT"

# Copy compressed binary (frontend is embedded in tvarr)
COPY --from=builder /tvarr /app/tvarr

# Copy entrypoint script
COPY deployment/docker/scripts/entrypoint-tvarr.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Environment defaults
ENV PUID=1000 \
    PGID=1000 \
    TZ=UTC \
    TVARR_SERVER_PORT=8080 \
    TVARR_DATABASE_DSN=file:/data/tvarr.db \
    TVARR_LOGGING_LEVEL=info \
    TVARR_STORAGE_BASE_DIR=/data

# Volume for persistent data
VOLUME ["/data"]

# Expose HTTP port
EXPOSE 8080

# Health check (Docker uses /health, K8s uses /livez and /readyz)
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD curl -sf http://localhost:${TVARR_SERVER_PORT:-8080}/health || exit 1

# Entrypoint handles user setup and signal handling
ENTRYPOINT ["/entrypoint.sh"]
