#!/bin/bash
# Update all external dependencies to their latest versions
# Usage: ./scripts/update-dependencies.sh [--check|--update] [--component=NAME]
#   --check     : Check for updates only (default, no changes made)
#   --update    : Update PKGBUILDs to latest versions
#   --component : Only check/update specific component (ffmpeg, gosu)
#
# Exit codes:
#   0 - Success (up to date or updated)
#   1 - Error (failed to fetch versions, update failed)
#   2 - Updates available (when using --check)

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Script directory (for relative paths)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Configuration
FFMPEG_PKGBUILD="$ROOT_DIR/deployment/docker/pkgbuild/ffmpeg/PKGBUILD"
GOSU_PKGBUILD="$ROOT_DIR/deployment/docker/pkgbuild/gosu/PKGBUILD"
FFMPEG_REPO="https://git.ffmpeg.org/ffmpeg.git"
GOSU_API="https://api.github.com/repos/tianon/gosu/releases/latest"

# State
UPDATE=false
COMPONENT=""
UPDATES_AVAILABLE=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --update)
            UPDATE=true
            shift
            ;;
        --check)
            UPDATE=false
            shift
            ;;
        --component=*)
            COMPONENT="${1#*=}"
            shift
            ;;
        -h|--help)
            echo "Usage: $0 [--check|--update] [--component=NAME]"
            echo ""
            echo "Options:"
            echo "  --check         Check for updates only (default)"
            echo "  --update        Update PKGBUILDs to latest versions"
            echo "  --component=X   Only check/update specific component (ffmpeg, gosu)"
            echo ""
            echo "Exit codes:"
            echo "  0 - Success (up to date or updated)"
            echo "  1 - Error"
            echo "  2 - Updates available (when using --check)"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

# Utility functions
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

# Compare versions (returns 0 if $1 > $2)
version_gt() {
    test "$(printf '%s\n' "$1" "$2" | sort -V | tail -1)" = "$1" && test "$1" != "$2"
}

# Get version from PKGBUILD
get_pkgbuild_version() {
    local pkgbuild="$1"
    grep -E "^pkgver=" "$pkgbuild" | cut -d'=' -f2
}

# Get sha256sum from PKGBUILD
get_pkgbuild_sha256() {
    local pkgbuild="$1"
    grep -E "^sha256sums=" "$pkgbuild" | sed "s/sha256sums=('\([^']*\)')/\1/"
}

# Update version in PKGBUILD
update_pkgbuild_version() {
    local pkgbuild="$1"
    local new_version="$2"
    sed -i "s/^pkgver=.*/pkgver=${new_version}/" "$pkgbuild"
}

# Update sha256sum in PKGBUILD
update_pkgbuild_sha256() {
    local pkgbuild="$1"
    local new_sha256="$2"
    sed -i "s/^sha256sums=('.*')/sha256sums=('${new_sha256}')/" "$pkgbuild"
}

# ============================================================================
# FFmpeg Update Functions
# ============================================================================

get_ffmpeg_latest() {
    git ls-remote --tags --refs "$FFMPEG_REPO" 2>/dev/null \
        | grep -E 'refs/tags/n[0-9]+\.[0-9]+(\.[0-9]+)?$' \
        | sed 's|.*refs/tags/n||' \
        | sort -V \
        | tail -1
}

check_ffmpeg() {
    log_section "FFmpeg"

    if [[ ! -f "$FFMPEG_PKGBUILD" ]]; then
        log_error "PKGBUILD not found: $FFMPEG_PKGBUILD"
        return 1
    fi

    local current_version latest_version
    current_version=$(get_pkgbuild_version "$FFMPEG_PKGBUILD")
    echo "Current version: $current_version"

    latest_version=$(get_ffmpeg_latest)
    if [[ -z "$latest_version" ]]; then
        log_error "Could not fetch latest FFmpeg version"
        return 1
    fi
    echo "Latest version:  $latest_version"

    if [[ "$current_version" == "$latest_version" ]]; then
        log_success "FFmpeg is up to date"
        return 0
    fi

    if ! version_gt "$latest_version" "$current_version"; then
        log_warn "Latest version ($latest_version) is not newer than current ($current_version)"
        return 0
    fi

    echo -e "${GREEN}Update available: ${current_version} -> ${latest_version}${NC}"
    UPDATES_AVAILABLE=true

    if [[ "$UPDATE" == true ]]; then
        log_info "Updating FFmpeg PKGBUILD..."
        update_pkgbuild_version "$FFMPEG_PKGBUILD" "$latest_version"

        # Verify the update
        local new_version
        new_version=$(get_pkgbuild_version "$FFMPEG_PKGBUILD")
        if [[ "$new_version" == "$latest_version" ]]; then
            log_success "FFmpeg PKGBUILD updated to $latest_version"
        else
            log_error "Failed to update FFmpeg PKGBUILD"
            return 1
        fi
    fi

    return 0
}

# ============================================================================
# gosu Update Functions
# ============================================================================

get_gosu_latest() {
    # Fetch latest release from GitHub API
    local release_info
    release_info=$(curl -fsSL "$GOSU_API" 2>/dev/null)
    if [[ -z "$release_info" ]]; then
        return 1
    fi

    # Extract version (remove 'v' prefix if present)
    echo "$release_info" | grep -oP '"tag_name":\s*"\K[^"]+' | sed 's/^v//'
}

get_gosu_sha256() {
    local version="$1"
    local tarball_url="https://github.com/tianon/gosu/archive/refs/tags/${version}.tar.gz"

    # Download and compute sha256
    curl -fsSL "$tarball_url" 2>/dev/null | sha256sum | cut -d' ' -f1
}

check_gosu() {
    log_section "gosu"

    if [[ ! -f "$GOSU_PKGBUILD" ]]; then
        log_error "PKGBUILD not found: $GOSU_PKGBUILD"
        return 1
    fi

    local current_version latest_version
    current_version=$(get_pkgbuild_version "$GOSU_PKGBUILD")
    echo "Current version: $current_version"

    latest_version=$(get_gosu_latest)
    if [[ -z "$latest_version" ]]; then
        log_error "Could not fetch latest gosu version"
        return 1
    fi
    echo "Latest version:  $latest_version"

    if [[ "$current_version" == "$latest_version" ]]; then
        log_success "gosu is up to date"
        return 0
    fi

    if ! version_gt "$latest_version" "$current_version"; then
        log_warn "Latest version ($latest_version) is not newer than current ($current_version)"
        return 0
    fi

    echo -e "${GREEN}Update available: ${current_version} -> ${latest_version}${NC}"
    UPDATES_AVAILABLE=true

    if [[ "$UPDATE" == true ]]; then
        log_info "Updating gosu PKGBUILD..."

        # Get new sha256sum
        log_info "Computing sha256sum for gosu $latest_version..."
        local new_sha256
        new_sha256=$(get_gosu_sha256 "$latest_version")
        if [[ -z "$new_sha256" ]]; then
            log_error "Could not compute sha256sum for gosu $latest_version"
            return 1
        fi
        echo "New sha256sum: $new_sha256"

        # Update version and sha256sum
        update_pkgbuild_version "$GOSU_PKGBUILD" "$latest_version"
        update_pkgbuild_sha256 "$GOSU_PKGBUILD" "$new_sha256"

        # Verify the update
        local new_version new_stored_sha256
        new_version=$(get_pkgbuild_version "$GOSU_PKGBUILD")
        new_stored_sha256=$(get_pkgbuild_sha256 "$GOSU_PKGBUILD")

        if [[ "$new_version" == "$latest_version" ]] && [[ "$new_stored_sha256" == "$new_sha256" ]]; then
            log_success "gosu PKGBUILD updated to $latest_version"
        else
            log_error "Failed to update gosu PKGBUILD"
            return 1
        fi
    fi

    return 0
}

# ============================================================================
# Main
# ============================================================================

main() {
    echo -e "${BLUE}Checking external dependencies for updates...${NC}"

    local exit_code=0

    # Check/update each component
    if [[ -z "$COMPONENT" ]] || [[ "$COMPONENT" == "ffmpeg" ]]; then
        check_ffmpeg || exit_code=1
    fi

    if [[ -z "$COMPONENT" ]] || [[ "$COMPONENT" == "gosu" ]]; then
        check_gosu || exit_code=1
    fi

    # Summary
    log_section "Summary"

    if [[ "$exit_code" -ne 0 ]]; then
        log_error "Some checks failed"
        exit 1
    fi

    if [[ "$UPDATES_AVAILABLE" == true ]]; then
        if [[ "$UPDATE" == true ]]; then
            log_success "All updates applied successfully"
            echo ""
            echo "Next steps:"
            echo "  1. Review changes: git diff deployment/docker/pkgbuild/*/PKGBUILD"
            echo "  2. Test builds: task container:build:ffmpeg && task container:build:tvarr"
            echo "  3. Commit: git add deployment/docker/pkgbuild/*/PKGBUILD && git commit -m 'chore: update dependencies'"
        else
            log_warn "Updates available. Run with --update to apply."
            exit 2
        fi
    else
        log_success "All dependencies are up to date"
    fi

    exit 0
}

main "$@"
