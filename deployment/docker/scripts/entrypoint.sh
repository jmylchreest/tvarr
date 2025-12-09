#!/bin/bash
# tvarr Docker Container Entrypoint
#
# Features:
# - PUID/PGID user mapping for volume permissions
# - TZ timezone configuration
# - Pre-flight diagnostics mode (TVARR_PREFLIGHT=true)
# - Graceful shutdown signal handling
#
# Environment Variables:
# - PUID: User ID (default: 1000)
# - PGID: Group ID (default: 1000)
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

# Default values
PUID=${PUID:-1000}
PGID=${PGID:-1000}
TZ=${TZ:-UTC}

# Ensure data directory exists
mkdir -p /data

# Configure timezone
if [ -n "$TZ" ] && [ -f "/usr/share/zoneinfo/$TZ" ]; then
    ln -sf "/usr/share/zoneinfo/$TZ" /etc/localtime
    echo "$TZ" > /etc/timezone
fi

# Configure user/group
setup_user() {
    log_info "Setting up user/group: PUID=$PUID PGID=$PGID"

    # Modify tvarr group if PGID is different
    if [ "$PGID" != "1000" ]; then
        groupmod -o -g "$PGID" tvarr 2>/dev/null || true
    fi

    # Modify tvarr user if PUID is different
    if [ "$PUID" != "1000" ]; then
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

    echo ""
    echo "VP9 encoders:"
    ffmpeg -hide_banner -encoders 2>/dev/null | grep -i vp9 | head -10 || true

    echo ""
    echo "AV1 encoders:"
    ffmpeg -hide_banner -encoders 2>/dev/null | grep -i av1 | head -10 || true

    # Audio encoders
    log_section "Audio Encoders"
    echo "Audio encoders (aac, mp3, opus, ac3, eac3):"
    ffmpeg -hide_banner -encoders 2>/dev/null | grep -E "aac|mp3|opus|ac3|eac3" | head -10 || true

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

    # V4L2 devices (Raspberry Pi)
    log_section "V4L2 Devices (Raspberry Pi)"
    if ls /dev/video* 1> /dev/null 2>&1; then
        echo "V4L2 devices found:"
        ls -la /dev/video* 2>/dev/null || true
    else
        log_info "No /dev/video* devices found (normal if not on Raspberry Pi)"
    fi

    # NVIDIA GPU
    log_section "NVIDIA GPU"
    if command -v nvidia-smi &> /dev/null; then
        nvidia-smi --query-gpu=name,driver_version,memory.total --format=csv,noheader 2>/dev/null || log_warn "nvidia-smi failed"
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

    # Environment
    log_section "Environment"
    echo "PUID: $PUID"
    echo "PGID: $PGID"
    echo "TZ: $TZ"
    echo "TVARR_PORT: ${TVARR_PORT:-8080}"
    echo "TVARR_LOG_LEVEL: ${TVARR_LOG_LEVEL:-info}"
    echo "TVARR_DATABASE_DSN: ${TVARR_DATABASE_DSN:-file:/data/tvarr.db}"

    # Data directory
    log_section "Data Directory"
    echo "/data contents:"
    ls -la /data 2>/dev/null || log_warn "Cannot list /data"

    log_section "Pre-flight Complete"
    log_success "Diagnostics finished. Container is ready."
}

# Signal handler for graceful shutdown
shutdown_handler() {
    log_info "Received shutdown signal, stopping tvarr..."
    if [ -n "$TVARR_PID" ]; then
        kill -TERM "$TVARR_PID" 2>/dev/null
        wait "$TVARR_PID" 2>/dev/null
    fi
    exit 0
}

# Set up signal handlers
trap shutdown_handler SIGTERM SIGINT SIGQUIT

# Main execution
main() {
    log_info "tvarr container starting..."
    log_info "PUID: $PUID, PGID: $PGID, TZ: $TZ"

    # Set up user permissions
    setup_user

    # Check for pre-flight mode
    if [ "${TVARR_PREFLIGHT:-false}" = "true" ]; then
        run_preflight
        exit 0
    fi

    # Build tvarr command with environment variables
    TVARR_CMD="/app/tvarr serve"

    # Add port if specified
    if [ -n "$TVARR_PORT" ]; then
        TVARR_CMD="$TVARR_CMD --port $TVARR_PORT"
    fi

    # Add log level if specified
    if [ -n "$TVARR_LOG_LEVEL" ]; then
        TVARR_CMD="$TVARR_CMD --log-level $TVARR_LOG_LEVEL"
    fi

    log_info "Starting tvarr as user tvarr (uid=$PUID, gid=$PGID)"
    log_info "Command: $TVARR_CMD"

    # Execute tvarr as the configured user using gosu
    # gosu is designed for containers - simpler than su/sudo, proper signal handling
    # Use exec to replace shell process for proper signal handling
    exec gosu tvarr $TVARR_CMD
}

# Run main function
main "$@"
