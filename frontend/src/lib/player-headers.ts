/**
 * player-headers.ts
 * -----------------
 * Constants and utilities for player identification headers.
 * Used by frontend players (mpegts.js, hls.js) to identify themselves
 * to the backend for smart container routing decisions.
 *
 * The X-Tvarr-Player header allows the backend to detect client capabilities
 * more accurately than User-Agent parsing, enabling optimal format selection.
 *
 * Format: "player" or "player:format"
 * Examples:
 *   - "mpegts.js" - mpegts.js player (prefers MPEG-TS)
 *   - "hls.js" - hls.js player (prefers HLS with fMP4)
 *   - "hls.js:ts" - hls.js player with explicit TS preference
 */

/**
 * Header name for explicit player identification.
 * Must match the backend constant in internal/relay/default_client_detector.go
 */
export const PLAYER_HEADER_NAME = 'X-Tvarr-Player';

/**
 * Known player identifiers.
 * These must match the xTvarrPlayerFormats map in default_client_detector.go
 */
export const PlayerNames = {
  MPEGTS_JS: 'mpegts.js',
  HLS_JS: 'hls.js',
  VIDEO_JS: 'video.js',
  EXOPLAYER: 'exoplayer',
  AVPLAYER: 'avplayer',
  VLC: 'vlc',
  KODI: 'kodi',
  SHAKA: 'shaka',
  DASH_JS: 'dash.js',
} as const;

export type PlayerName = (typeof PlayerNames)[keyof typeof PlayerNames];

/**
 * Format override values.
 * These must match the format values in default_client_detector.go
 */
export const FormatValues = {
  HLS: 'hls',
  HLS_FMP4: 'hls-fmp4',
  HLS_TS: 'hls-ts',
  MPEGTS: 'mpegts',
  DASH: 'dash',
} as const;

export type FormatValue = (typeof FormatValues)[keyof typeof FormatValues];

/**
 * Build the X-Tvarr-Player header value.
 *
 * @param playerName - The player identifier (e.g., "mpegts.js", "hls.js")
 * @param formatOverride - Optional format preference (e.g., "ts", "fmp4")
 * @returns Header value string (e.g., "mpegts.js" or "hls.js:fmp4")
 */
export function buildPlayerHeader(playerName: PlayerName, formatOverride?: FormatValue): string {
  if (formatOverride) {
    return `${playerName}:${formatOverride}`;
  }
  return playerName;
}

/**
 * Build headers object for use with fetch or player libraries.
 *
 * @param playerName - The player identifier
 * @param formatOverride - Optional format preference
 * @returns Headers object with X-Tvarr-Player header
 */
export function buildPlayerHeaders(
  playerName: PlayerName,
  formatOverride?: FormatValue
): Record<string, string> {
  return {
    [PLAYER_HEADER_NAME]: buildPlayerHeader(playerName, formatOverride),
  };
}
