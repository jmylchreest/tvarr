#!/bin/bash
# tvarr Docker Container Entrypoint
#
# Features:
# - PUID/PGID user mapping for volume permissions (when running as root)
# - TZ timezone configuration (env var for Go, /etc/localtime when root)
# - Pre-flight diagnostics mode (TVARR_PREFLIGHT=true)
# - Graceful shutdown signal handling
# - Supports runAsNonRoot in Kubernetes security contexts
#
# Environment Variables:
# - PUID: User ID (default: 1000, ignored when non-root)
# - PGID: Group ID (default: 1000, ignored when non-root)
# - TZ: Timezone (default: UTC)
# - TVARR_PREFLIGHT: Run diagnostics and exit (default: false)
# - TVARR_*: Application-specific variables passed to tvarr

set -e

# Colors for output (disabled if not a terminal)
if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    BLUE='\033[0;34m'
    NC='\033[0m' # No Color
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    NC=''
fi

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[OK]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_section() {
    echo ""
    echo -e "${BLUE}=== $1 ===${NC}"
}

# Run a command only when the container is running as root.
# Silently skips when non-root (e.g. Kubernetes runAsNonRoot).
run_as_root() {
    if [ "$(id -u)" = "0" ]; then
        "$@"
    fi
}

# Default values
PUID=${PUID:-1000}
PGID=${PGID:-1000}
TZ=${TZ:-UTC}
export TZ

# Ensure data directory exists
run_as_root mkdir -p /data

# Configure timezone
# Go reads the TZ env var directly, so the symlink is only needed for
# non-Go tools (e.g. date). Safe to skip when non-root.
if [ -n "$TZ" ] && [ -f "/usr/share/zoneinfo/$TZ" ]; then
    run_as_root ln -sf "/usr/share/zoneinfo/$TZ" /etc/localtime
    run_as_root sh -c "echo '$TZ' > /etc/timezone"
fi

# Configure user/group
setup_user() {
    if [ "$(id -u)" != "0" ]; then
        log_info "Running as non-root (uid=$(id -u)), skipping user setup"
        return
    fi

    log_info "Setting up user/group: PUID=$PUID PGID=$PGID"

    # Create tvarr group if it doesn't exist
    if ! getent group tvarr > /dev/null 2>&1; then
        groupadd -g "$PGID" tvarr 2>/dev/null || true
    elif [ "$PGID" != "1000" ]; then
        groupmod -o -g "$PGID" tvarr 2>/dev/null || true
    fi

    # Create tvarr user if it doesn't exist
    if ! id tvarr > /dev/null 2>&1; then
        useradd -u "$PUID" -g tvarr -s /sbin/nologin tvarr 2>/dev/null || true
    elif [ "$PUID" != "1000" ]; then
        usermod -o -u "$PUID" tvarr 2>/dev/null || true
    fi

    # Fix ownership of data directory
    chown -R tvarr:tvarr /data 2>/dev/null || log_warn "Could not chown /data (may be read-only)"
}

# Pre-flight diagnostics
run_preflight() {
    log_section "tvarr Pre-flight Diagnostics"

    # Version info
    log_section "Version Information"
    if command -v /app/tvarr &> /dev/null; then
        echo "tvarr version: $(/app/tvarr version 2>/dev/null || echo 'unknown')"
    else
        log_warn "tvarr binary not found at /app/tvarr"
    fi

    # FFmpeg info
    log_section "FFmpeg Information"
    if command -v ffmpeg &> /dev/null; then
        ffmpeg -version 2>/dev/null | head -1
    else
        log_warn "ffmpeg not found"
    fi

    # Environment
    log_section "Configuration"
    echo "PUID: $PUID"
    echo "PGID: $PGID"
    echo "TZ: $TZ"
    echo "TVARR_SERVER_PORT: ${TVARR_SERVER_PORT:-8080}"
    echo "TVARR_LOGGING_LEVEL: ${TVARR_LOGGING_LEVEL:-info}"
    echo "TVARR_DATABASE_DSN: ${TVARR_DATABASE_DSN:-/data/tvarr.db}"

    # Data directory
    log_section "Data Directory"
    echo "/data contents:"
    ls -la /data 2>/dev/null || log_warn "Cannot list /data"

    # Network
    log_section "Network"
    echo "Hostname: $(hostname)"

    log_section "Pre-flight Complete"
    log_success "Diagnostics finished. Container is ready."
}

# Main execution
main() {
    log_info "tvarr container starting..."
    log_info "uid=$(id -u), gid=$(id -g), TZ=$TZ"

    # Set up user permissions
    setup_user

    # Check for pre-flight mode
    if [ "${TVARR_PREFLIGHT:-false}" = "true" ]; then
        run_preflight
        exit 0
    fi

    # Build command
    local cmd="/app/tvarr serve"

    # Add port if specified
    if [ -n "$TVARR_PORT" ]; then
        cmd="$cmd --port $TVARR_PORT"
    fi

    # Add log level if specified
    if [ -n "$TVARR_LOG_LEVEL" ]; then
        cmd="$cmd --log-level $TVARR_LOG_LEVEL"
    fi

    # When root, drop privileges via gosu; otherwise run directly
    if [ "$(id -u)" = "0" ]; then
        log_info "Dropping privileges to tvarr (uid=$PUID, gid=$PGID)"
        exec gosu tvarr $cmd
    else
        exec $cmd
    fi
}

# Run main function
main "$@"
