#!/bin/bash
# tvarr-ffmpegd Docker Container Entrypoint
#
# Features:
# - PUID/PGID user mapping for GPU device permissions (when running as root)
# - TZ timezone configuration (env var for Go, /etc/localtime when root)
# - Pre-flight diagnostics mode (TVARR_PREFLIGHT=true)
# - Graceful shutdown with coordinator unregistration
# - Supports runAsNonRoot in Kubernetes security contexts
#
# Environment Variables:
# - PUID: User ID (default: 1000, ignored when non-root)
# - PGID: Group ID (default: 1000, ignored when non-root)
# - TZ: Timezone (default: UTC)
# - TVARR_COORDINATOR_URL: Coordinator gRPC address (required)
# - TVARR_AUTH_TOKEN: Authentication token (optional)
# - TVARR_DAEMON_NAME: Human-readable daemon name (optional, defaults to hostname)
# - TVARR_MAX_JOBS: Maximum concurrent transcoding jobs (default: 4)
# - TVARR_PREFLIGHT: Run diagnostics and exit (default: false)

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

# Configure timezone
# Go reads the TZ env var directly, so the symlink is only needed for
# non-Go tools (e.g. date). Safe to skip when non-root.
if [ -n "$TZ" ] && [ -f "/usr/share/zoneinfo/$TZ" ]; then
    run_as_root ln -sf "/usr/share/zoneinfo/$TZ" /etc/localtime
    run_as_root sh -c "echo '$TZ' > /etc/timezone"
fi

# Configure user/group for GPU access
setup_user() {
    if [ "$(id -u)" != "0" ]; then
        log_info "Running as non-root (uid=$(id -u)), skipping user setup"
        return
    fi

    log_info "Setting up user/group: PUID=$PUID PGID=$PGID"

    # Modify tvarr group if PGID is different
    if [ "$PGID" != "1000" ]; then
        groupmod -o -g "$PGID" tvarr 2>/dev/null || true
    fi

    # Modify tvarr user if PUID is different
    if [ "$PUID" != "1000" ]; then
        usermod -o -u "$PUID" tvarr 2>/dev/null || true
    fi

    # Add tvarr user to video and render groups for GPU access
    usermod -aG video tvarr 2>/dev/null || true
    usermod -aG render tvarr 2>/dev/null || true
}

# Pre-flight diagnostics
run_preflight() {
    log_section "tvarr-ffmpegd Pre-flight Diagnostics"

    # Version info
    log_section "Version Information"
    if command -v /app/tvarr-ffmpegd &> /dev/null; then
        echo "tvarr-ffmpegd version: $(/app/tvarr-ffmpegd version 2>/dev/null || echo 'unknown')"
    else
        log_warn "tvarr-ffmpegd binary not found at /app/tvarr-ffmpegd"
    fi

    echo ""
    echo "FFmpeg version:"
    ffmpeg -version 2>/dev/null | head -1 || log_error "FFmpeg not found"

    echo ""
    echo "FFprobe version:"
    ffprobe -version 2>/dev/null | head -1 || log_error "FFprobe not found"

    # Hardware accelerators
    log_section "Hardware Accelerators"
    echo "Available hardware accelerators:"
    ffmpeg -hide_banner -hwaccels 2>/dev/null || log_warn "Could not query hardware accelerators"

    # Video encoders
    log_section "Video Encoders"
    echo "H.264 encoders:"
    ffmpeg -hide_banner -encoders 2>/dev/null | grep -E "264|avc" | head -10 || true

    echo ""
    echo "H.265/HEVC encoders:"
    ffmpeg -hide_banner -encoders 2>/dev/null | grep -E "265|hevc" | head -10 || true

    # GPU/DRI devices
    log_section "GPU Devices"
    echo "/dev/dri contents:"
    if [ -d /dev/dri ]; then
        ls -la /dev/dri/ 2>/dev/null || log_warn "Cannot list /dev/dri"

        # Check permissions
        for device in /dev/dri/render*; do
            if [ -e "$device" ]; then
                if [ -r "$device" ] && [ -w "$device" ]; then
                    log_success "$device is accessible"
                else
                    log_warn "$device exists but may not be accessible"
                fi
            fi
        done
    else
        log_warn "/dev/dri not found - no GPU access"
    fi

    # NVIDIA GPU
    log_section "NVIDIA GPU"
    if command -v nvidia-smi &> /dev/null; then
        nvidia-smi --query-gpu=name,driver_version,memory.total,encoder.stats.sessionCount,encoder.stats.averageFps --format=csv,noheader 2>/dev/null || log_warn "nvidia-smi failed"
    else
        log_info "nvidia-smi not available (NVIDIA runtime not enabled or no GPU)"
    fi

    # VAAPI info
    log_section "VAAPI Information"
    if command -v vainfo &> /dev/null; then
        vainfo 2>/dev/null || log_warn "vainfo failed (may need /dev/dri access)"
    else
        log_info "vainfo not available"
    fi

    # Configuration
    log_section "Configuration"
    echo "PUID: $PUID"
    echo "PGID: $PGID"
    echo "TZ: $TZ"
    echo "TVARR_COORDINATOR_URL: ${TVARR_COORDINATOR_URL:-<not set>}"
    echo "TVARR_AUTH_TOKEN: ${TVARR_AUTH_TOKEN:+<set>}${TVARR_AUTH_TOKEN:-<not set>}"
    echo "TVARR_DAEMON_NAME: ${TVARR_DAEMON_NAME:-<auto>}"
    echo "TVARR_MAX_JOBS: ${TVARR_MAX_JOBS:-4}"

    log_section "Pre-flight Complete"
    log_success "Diagnostics finished. Daemon is ready."
}

# Main execution
main() {
    log_info "tvarr-ffmpegd container starting..."
    log_info "uid=$(id -u), gid=$(id -g), TZ=$TZ"

    # Set up user permissions
    setup_user

    # Check for pre-flight mode
    if [ "${TVARR_PREFLIGHT:-false}" = "true" ]; then
        run_preflight
        exit 0
    fi

    # Validate required configuration
    if [ -z "$TVARR_COORDINATOR_URL" ]; then
        log_error "TVARR_COORDINATOR_URL is required"
        log_info "Set the coordinator URL, e.g.: TVARR_COORDINATOR_URL=tvarr:9090"
        exit 1
    fi

    # Build daemon command
    DAEMON_CMD="/app/tvarr-ffmpegd serve"

    # Add daemon name only if explicitly set (daemon defaults to hostname)
    if [ -n "$TVARR_DAEMON_NAME" ]; then
        DAEMON_CMD="$DAEMON_CMD --name $TVARR_DAEMON_NAME"
    fi

    # Add max jobs if specified
    if [ -n "$TVARR_MAX_JOBS" ]; then
        DAEMON_CMD="$DAEMON_CMD --max-jobs $TVARR_MAX_JOBS"
    fi

    log_info "Connecting to coordinator: $TVARR_COORDINATOR_URL"
    log_info "Daemon name: ${TVARR_DAEMON_NAME:-$(hostname)} (container hostname)"
    log_info "Command: $DAEMON_CMD"

    # When root, drop privileges via gosu; otherwise run directly
    if [ "$(id -u)" = "0" ]; then
        log_info "Dropping privileges to tvarr (uid=$PUID, gid=$PGID)"
        exec gosu tvarr $DAEMON_CMD
    else
        exec $DAEMON_CMD
    fi
}

# Run main function
main "$@"
