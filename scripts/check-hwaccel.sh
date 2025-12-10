#!/usr/bin/env bash
# Hardware Acceleration Test Script for tvarr/FFmpeg
#
# Tests which hardware-accelerated codecs are actually working by performing
# real encode/decode operations with a test pattern.
#
# Usage:
#   ./scripts/check-hwaccel.sh           # Run all tests
#   ./scripts/check-hwaccel.sh --quick   # Quick check (skip encode tests)
#   ./scripts/check-hwaccel.sh --json    # Output results as JSON
#
# Inside container:
#   /usr/local/bin/check-hwaccel.sh
#
# Exit codes:
#   0 - At least one hardware accelerator is working
#   1 - No hardware acceleration available (software only)
#   2 - FFmpeg not found or other error

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color
BOLD='\033[1m'

# Parse arguments
QUICK_MODE=false
JSON_MODE=false
VERBOSE=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --quick|-q)
            QUICK_MODE=true
            shift
            ;;
        --json|-j)
            JSON_MODE=true
            shift
            ;;
        --verbose|-v)
            VERBOSE=true
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --quick, -q    Quick check (list available, skip encode tests)"
            echo "  --json, -j     Output results as JSON"
            echo "  --verbose, -v  Show detailed FFmpeg output"
            echo "  --help, -h     Show this help"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 2
            ;;
    esac
done

# Temporary directory for test files
TMPDIR="${TMPDIR:-/tmp}"
TEST_DIR=$(mktemp -d "${TMPDIR}/hwaccel-test.XXXXXX")
trap 'rm -rf "$TEST_DIR"' EXIT

# Results storage
declare -A HWACCEL_STATUS
declare -A ENCODER_STATUS
declare -A ENCODER_ERROR
declare -A DECODER_STATUS

# Check if FFmpeg is available
check_ffmpeg() {
    if ! command -v ffmpeg &> /dev/null; then
        if [[ "$JSON_MODE" == "true" ]]; then
            echo '{"error": "FFmpeg not found"}'
        else
            echo -e "${RED}ERROR: FFmpeg not found${NC}"
        fi
        exit 2
    fi
}

# Print section header
print_header() {
    if [[ "$JSON_MODE" != "true" ]]; then
        echo ""
        echo -e "${BOLD}${BLUE}=== $1 ===${NC}"
        echo ""
    fi
}

# Print status
print_status() {
    local name="$1"
    local status="$2"
    local details="${3:-}"

    if [[ "$JSON_MODE" != "true" ]]; then
        if [[ "$status" == "ok" ]]; then
            printf "  %-25s ${GREEN}[OK]${NC}" "$name"
        elif [[ "$status" == "fail" ]]; then
            printf "  %-25s ${RED}[FAIL]${NC}" "$name"
        elif [[ "$status" == "skip" ]]; then
            printf "  %-25s ${YELLOW}[SKIP]${NC}" "$name"
        else
            printf "  %-25s ${CYAN}[INFO]${NC}" "$name"
        fi
        if [[ -n "$details" ]]; then
            echo " $details"
        else
            echo ""
        fi
    fi
}

# Get FFmpeg version and build info
get_ffmpeg_info() {
    print_header "FFmpeg Build Information"

    if [[ "$JSON_MODE" != "true" ]]; then
        ffmpeg -version 2>&1 | head -1
        echo ""
    fi
}

# List available hardware accelerators
list_hwaccels() {
    print_header "Available Hardware Accelerators"

    local hwaccels
    hwaccels=$(ffmpeg -hwaccels 2>/dev/null | tail -n +2 | tr -d ' ' | grep -v '^$' || true)

    if [[ -z "$hwaccels" ]]; then
        print_status "Hardware accelerators" "fail" "None available"
        return
    fi

    while IFS= read -r accel; do
        HWACCEL_STATUS["$accel"]="available"
        print_status "$accel" "info" "available"
    done <<< "$hwaccels"
}

# Check VAAPI support
check_vaapi() {
    print_header "VAAPI (Intel/AMD GPU)"

    # Check for DRI devices
    if [[ ! -d /dev/dri ]]; then
        print_status "DRI devices" "fail" "/dev/dri not found"
        HWACCEL_STATUS["vaapi"]="no_device"
        return
    fi

    local render_nodes
    render_nodes=$(ls /dev/dri/renderD* 2>/dev/null || true)
    if [[ -z "$render_nodes" ]]; then
        print_status "Render nodes" "fail" "No render nodes found"
        HWACCEL_STATUS["vaapi"]="no_render_node"
        return
    fi

    print_status "Render nodes" "ok" "$render_nodes"

    # Check vainfo if available
    if command -v vainfo &> /dev/null; then
        local vainfo_out
        if vainfo_out=$(vainfo 2>&1); then
            local driver
            driver=$(echo "$vainfo_out" | grep -o "Driver version:.*" | head -1 || echo "unknown")
            print_status "VAAPI driver" "ok" "$driver"
            HWACCEL_STATUS["vaapi"]="ok"
        else
            print_status "VAAPI driver" "fail" "vainfo failed"
            HWACCEL_STATUS["vaapi"]="driver_fail"
        fi
    else
        print_status "vainfo" "skip" "not installed"
    fi
}

# Check NVIDIA support
check_nvidia() {
    print_header "NVIDIA (NVENC/NVDEC)"

    # Check for NVIDIA devices
    if [[ ! -e /dev/nvidia0 ]] && [[ ! -e /dev/nvidiactl ]]; then
        print_status "NVIDIA devices" "fail" "Not found (need --gpus or --device)"
        HWACCEL_STATUS["cuda"]="no_device"
        return
    fi

    print_status "NVIDIA devices" "ok" "Found"

    # Check nvidia-smi if available
    if command -v nvidia-smi &> /dev/null; then
        local gpu_info
        if gpu_info=$(nvidia-smi --query-gpu=name,driver_version --format=csv,noheader 2>/dev/null | head -1); then
            print_status "NVIDIA GPU" "ok" "$gpu_info"
            HWACCEL_STATUS["cuda"]="ok"
        else
            print_status "NVIDIA GPU" "fail" "nvidia-smi failed"
            HWACCEL_STATUS["cuda"]="driver_fail"
        fi
    else
        # nvidia-smi not available but devices exist
        HWACCEL_STATUS["cuda"]="available"
        print_status "NVIDIA driver" "info" "devices present (nvidia-smi not available)"
    fi
}

# Check Vulkan support
check_vulkan() {
    print_header "Vulkan"

    # Check if Vulkan is in hwaccels
    if [[ "${HWACCEL_STATUS[vulkan]:-}" == "available" ]]; then
        print_status "Vulkan support" "ok" "Available in FFmpeg"
    else
        print_status "Vulkan support" "skip" "Not in FFmpeg hwaccels"
    fi

    # Check for Vulkan ICD
    if [[ -d /etc/vulkan/icd.d ]] || [[ -d /usr/share/vulkan/icd.d ]]; then
        print_status "Vulkan ICD" "ok" "Found"
    else
        print_status "Vulkan ICD" "info" "No ICD directory found"
    fi
}

# Generate a short test video
generate_test_video() {
    local output="$1"
    local duration="${2:-2}"

    ffmpeg -y -f lavfi -i "testsrc=duration=${duration}:size=1280x720:rate=30" \
        -f lavfi -i "sine=frequency=1000:duration=${duration}" \
        -c:v libx264 -preset ultrafast -c:a aac \
        "$output" 2>/dev/null
}

# Test a specific encoder and capture detailed error info
test_encoder() {
    local encoder="$1"
    local hwaccel="${2:-}"
    local extra_args="${3:-}"

    local input="${TEST_DIR}/test_input.mp4"
    local output="${TEST_DIR}/test_${encoder}.mp4"
    local errlog="${TEST_DIR}/err_${encoder}.log"

    # Generate input if needed
    if [[ ! -f "$input" ]]; then
        generate_test_video "$input" 1
    fi

    local cmd="ffmpeg -y"
    if [[ -n "$hwaccel" ]]; then
        cmd="$cmd -hwaccel $hwaccel"
    fi
    cmd="$cmd -i $input $extra_args -c:v $encoder -t 1 $output"

    if [[ "$VERBOSE" == "true" ]]; then
        echo "  Testing: $cmd"
    fi

    # Run and capture stderr for error analysis
    if eval "$cmd" 2>"$errlog"; then
        ENCODER_STATUS["$encoder"]="ok"
        ENCODER_ERROR["$encoder"]=""
        return 0
    else
        # Analyze the error
        local error_msg=""
        if grep -qi "cannot open shared object file\|libcuda\|libnvidia\|libvdpau_nvidia" "$errlog" 2>/dev/null; then
            error_msg="missing library"
            ENCODER_STATUS["$encoder"]="missing_lib"
        elif grep -qi "no device\|device not found\|cannot open\|failed to initialise" "$errlog" 2>/dev/null; then
            error_msg="no device"
            ENCODER_STATUS["$encoder"]="no_device"
        elif grep -qi "driver\|permission denied" "$errlog" 2>/dev/null; then
            error_msg="driver/permission"
            ENCODER_STATUS["$encoder"]="driver_fail"
        else
            error_msg="encode failed"
            ENCODER_STATUS["$encoder"]="fail"
        fi
        ENCODER_ERROR["$encoder"]="$error_msg"

        if [[ "$VERBOSE" == "true" ]]; then
            echo "  Error log:"
            cat "$errlog" | head -10 | sed 's/^/    /'
        fi
        return 1
    fi
}

# Test hardware encoders
# Tests ALL available hw encoders to verify they work (or identify why they fail)
test_encoders() {
    if [[ "$QUICK_MODE" == "true" ]]; then
        return
    fi

    print_header "Hardware Encoder Tests"

    echo -e "  ${CYAN}Testing all hardware encoders built into FFmpeg...${NC}"
    echo ""

    # Get all hardware encoders
    local hw_encoders
    hw_encoders=$(ffmpeg -encoders 2>/dev/null | grep -E '(vaapi|nvenc|qsv|v4l2m2m|amf)' | awk '{print $2}' || true)

    if [[ -z "$hw_encoders" ]]; then
        print_status "No HW encoders" "info" "FFmpeg built without hardware encoders"
        return
    fi

    # Test each encoder
    while IFS= read -r enc; do
        local hwaccel=""
        local extra_args=""

        # Determine hwaccel and args based on encoder type
        case "$enc" in
            *_vaapi)
                hwaccel="vaapi"
                # Find first available render device
                local render_dev="/dev/dri/renderD128"
                if [[ -e /dev/dri/renderD129 ]] && [[ ! -e /dev/dri/renderD128 ]]; then
                    render_dev="/dev/dri/renderD129"
                fi
                extra_args="-vaapi_device $render_dev -vf 'format=nv12,hwupload'"
                ;;
            *_nvenc)
                hwaccel="cuda"
                ;;
            *_qsv)
                hwaccel="qsv"
                ;;
            *_v4l2m2m)
                hwaccel=""
                ;;
            *_amf)
                hwaccel="amf"
                ;;
        esac

        if test_encoder "$enc" "$hwaccel" "$extra_args"; then
            print_status "$enc" "ok" "Working"
        else
            local reason="${ENCODER_ERROR[$enc]:-unknown}"
            case "${ENCODER_STATUS[$enc]}" in
                missing_lib)
                    # For NVIDIA, libcuda.so is provided by host driver via --gpus
                    if [[ "$enc" == *_nvenc ]]; then
                        print_status "$enc" "skip" "No NVIDIA driver (need --gpus all)"
                    else
                        print_status "$enc" "skip" "Missing library"
                    fi
                    ;;
                no_device)
                    print_status "$enc" "skip" "No device (GPU not present/accessible)"
                    ;;
                driver_fail)
                    print_status "$enc" "fail" "Driver/permission issue"
                    ;;
                *)
                    print_status "$enc" "fail" "$reason"
                    ;;
            esac
        fi
    done <<< "$hw_encoders"
}

# List available hardware encoders/decoders
list_hw_codecs() {
    print_header "Available Hardware Codecs"

    echo -e "  ${BOLD}Encoders:${NC}"
    local hw_encoders
    hw_encoders=$(ffmpeg -encoders 2>/dev/null | grep -E '(vaapi|nvenc|qsv|v4l2m2m|videotoolbox|amf)' | awk '{print $2}' || true)
    if [[ -n "$hw_encoders" ]]; then
        while IFS= read -r enc; do
            print_status "  $enc" "info"
        done <<< "$hw_encoders"
    else
        print_status "  (none found)" "info"
    fi

    echo ""
    echo -e "  ${BOLD}Decoders:${NC}"
    local hw_decoders
    hw_decoders=$(ffmpeg -decoders 2>/dev/null | grep -E '(cuvid|vaapi|qsv|v4l2m2m|videotoolbox)' | awk '{print $2}' || true)
    if [[ -n "$hw_decoders" ]]; then
        while IFS= read -r dec; do
            print_status "  $dec" "info"
        done <<< "$hw_decoders"
    else
        print_status "  (none found)" "info"
    fi
}

# Print summary
print_summary() {
    print_header "Summary"

    local working_hwaccels=0
    local working_encoders=0

    for status in "${HWACCEL_STATUS[@]}"; do
        if [[ "$status" == "ok" ]]; then
            ((working_hwaccels++)) || true
        fi
    done

    for status in "${ENCODER_STATUS[@]}"; do
        if [[ "$status" == "ok" ]]; then
            ((working_encoders++)) || true
        fi
    done

    if [[ "$JSON_MODE" == "true" ]]; then
        # Output JSON
        echo "{"
        echo '  "hwaccels": {'
        local first=true
        for key in "${!HWACCEL_STATUS[@]}"; do
            if [[ "$first" != "true" ]]; then echo ","; fi
            first=false
            printf '    "%s": "%s"' "$key" "${HWACCEL_STATUS[$key]}"
        done
        echo ""
        echo "  },"
        echo '  "encoders": {'
        first=true
        for key in "${!ENCODER_STATUS[@]}"; do
            if [[ "$first" != "true" ]]; then echo ","; fi
            first=false
            printf '    "%s": "%s"' "$key" "${ENCODER_STATUS[$key]}"
        done
        echo ""
        echo "  },"
        printf '  "working_hwaccels": %d,\n' "$working_hwaccels"
        printf '  "working_encoders": %d\n' "$working_encoders"
        echo "}"
    else
        if [[ $working_hwaccels -gt 0 ]]; then
            echo -e "  ${GREEN}Hardware acceleration: AVAILABLE${NC}"
            echo -e "  Working accelerators: $working_hwaccels"
            if [[ "$QUICK_MODE" != "true" ]]; then
                echo -e "  Working encoders: $working_encoders"
            fi
        else
            echo -e "  ${YELLOW}Hardware acceleration: NOT AVAILABLE${NC}"
            echo -e "  FFmpeg will use software encoding/decoding"
        fi
        echo ""
    fi

    # Return appropriate exit code
    if [[ $working_hwaccels -gt 0 ]]; then
        return 0
    else
        return 1
    fi
}

# Main
main() {
    check_ffmpeg
    get_ffmpeg_info
    list_hwaccels
    check_vaapi
    check_nvidia
    check_vulkan
    list_hw_codecs
    test_encoders
    print_summary
}

main
