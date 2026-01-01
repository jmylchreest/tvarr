#!/bin/bash
# Generate 1-second placeholder fMP4 segments for stream startup
# These are embedded in the binary and looped to fill segment duration.
# Using 1s keeps binary size small while allowing flexible segment durations.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="${SCRIPT_DIR}/../internal/relay/placeholders"

# Configuration - 1 second base unit (will be looped by BufferInjector)
DURATION=1
WIDTH=1920
HEIGHT=1080
FRAMERATE=25
AUDIO_RATE=48000
AUDIO_CHANNELS=2

# Text to display
MESSAGE="Stream Starting..."
FONT_SIZE=72
FONT_COLOR="white"
BG_COLOR="0x1a1a2e"  # Dark blue-gray

mkdir -p "${OUTPUT_DIR}"

echo "Generating 1-second placeholder content (looped at runtime to fill segments)..."
echo "Output directory: ${OUTPUT_DIR}"

# Generate lavfi filter for video with centered text
VIDEO_FILTER="color=c=${BG_COLOR}:s=${WIDTH}x${HEIGHT}:d=${DURATION}:r=${FRAMERATE},format=yuv420p,drawtext=text='${MESSAGE}':fontsize=${FONT_SIZE}:fontcolor=${FONT_COLOR}:x=(w-text_w)/2:y=(h-text_h)/2"

# Silent audio source
AUDIO_FILTER="anullsrc=r=${AUDIO_RATE}:cl=stereo:d=${DURATION}"

generate_variant() {
    local name=$1
    local vcodec=$2
    local acodec=$3
    local vcodec_opts=$4
    local acodec_opts=$5

    local output_file="${OUTPUT_DIR}/placeholder_${name}_${DURATION}s.mp4"

    echo "Generating ${name}..."

    ffmpeg -y \
        -f lavfi -i "${VIDEO_FILTER}" \
        -f lavfi -i "${AUDIO_FILTER}" \
        -c:v ${vcodec} ${vcodec_opts} \
        -c:a ${acodec} ${acodec_opts} \
        -movflags +frag_keyframe+empty_moov+default_base_moof \
        -frag_duration 1000000 \
        -t ${DURATION} \
        "${output_file}" 2>/dev/null

    local size=$(stat -f%z "${output_file}" 2>/dev/null || stat -c%s "${output_file}" 2>/dev/null)
    echo "  Created: ${output_file} (${size} bytes)"
}

# H.264/AAC - Most compatible
generate_variant "h264_aac" "libx264" "aac" \
    "-preset fast -profile:v high -level 4.1 -pix_fmt yuv420p -g ${FRAMERATE} -b:v 300k" \
    "-b:a 48k -ar ${AUDIO_RATE} -ac ${AUDIO_CHANNELS}"

# H.265/AAC - Better compression
generate_variant "h265_aac" "libx265" "aac" \
    "-preset fast -profile:v main -pix_fmt yuv420p -g ${FRAMERATE} -b:v 200k -tag:v hvc1" \
    "-b:a 48k -ar ${AUDIO_RATE} -ac ${AUDIO_CHANNELS}"

# VP9/Opus - WebM compatible
generate_variant "vp9_opus" "libvpx-vp9" "libopus" \
    "-deadline realtime -cpu-used 4 -row-mt 1 -g ${FRAMERATE} -b:v 200k" \
    "-b:a 48k -ar ${AUDIO_RATE} -ac ${AUDIO_CHANNELS}"

# AV1/Opus - Next-gen
generate_variant "av1_opus" "libaom-av1" "libopus" \
    "-cpu-used 8 -row-mt 1 -tiles 2x1 -g ${FRAMERATE} -b:v 150k -strict experimental" \
    "-b:a 48k -ar ${AUDIO_RATE} -ac ${AUDIO_CHANNELS}"

echo ""
echo "All placeholders generated successfully!"
echo "These 1s clips will be looped by BufferInjector to fill segment duration."
echo ""
echo "Files:"
ls -lh "${OUTPUT_DIR}"/*.mp4 2>/dev/null || echo "No files generated"
