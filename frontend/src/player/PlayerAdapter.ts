/**
 * PlayerAdapter.ts (Minimal TS-only API)
 * --------------------------------------
 * The application has been simplified to always expect a continuous MPEG-TS (video/mp2t)
 * stream from the backend. All multi-adapter / HLS logic has been removed.
 *
 * Only the MPEG-TS player (mpegts.js) remains. We preserve a subset of the previous
 * types so that the existing `MpegTsAdapter` (which still imports these symbols)
 * continues to compile with minimal changes. Non-TS kinds are intentionally pruned.
 *
 * Deprecated / Removed:
 *  - Multiple StreamKind variants (now only 'RAW_TS' is meaningful)
 *  - Priority ordering / adapter selection helpers
 *  - Generic multi-adapter interface usage (kept nominally for backward compatibility
 *    but should not be extended)
 *
 * If further simplification is desired, the `MpegTsAdapter` can inline these types and
 * this file can be deleted entirely.
 */

/* ------------------ Core Types (Slimmed) ------------------ */

/**
 * Only RAW_TS is supported now. Other former kinds are retained in the union
 * for backward source compatibility but should not be produced by new code.
 */
export type StreamKind = 'RAW_TS' | 'HLS_PLAYLIST' | 'PROGRESSIVE' | 'UNKNOWN';

/**
 * Structured player error (unchanged).
 */
export interface PlayerError {
  message: string;
  fatal?: boolean;
  cause?: unknown;
  code?: string;
  category?: 'network' | 'media' | 'decode' | 'timing' | 'unknown';
  meta?: Record<string, unknown>;
}

/**
 * Media info snapshot reported by the MPEG-TS player.
 */
export interface PlayerMediaInfo {
  videoCodec?: string;
  audioCodec?: string;
  width?: number;
  height?: number;
  fps?: number;
  live?: boolean;
  [key: string]: unknown;
}

/**
 * Environment hints (still referenced by the adapter).
 */
export interface PlaybackEnv {
  isSafari: boolean;
  isMobile: boolean;
  mseSupported: boolean;
  ua?: string;
}

/**
 * Load options consumed by the (single) MPEG-TS adapter.
 */
export interface PlayerLoadOptions {
  url: string;
  videoEl: HTMLVideoElement;
  onError: (err: PlayerError) => void;
  onMediaInfo?: (info: PlayerMediaInfo) => void;
  log?: {
    debug: (...a: any[]) => void;
    info: (...a: any[]) => void;
    warn: (...a: any[]) => void;
    error: (...a: any[]) => void;
  };
  flags?: Record<string, unknown>;
}

/**
 * Minimal adapter contract (retained so existing MpegTsAdapter continues to type-check).
 * Do not add new implementations; future refactors can inline this directly into the
 * adapter file if desired.
 */
export interface PlayerAdapter {
  readonly name: string;
  canPlay(kind: StreamKind, url: string, env: PlaybackEnv): boolean;
  load(opts: PlayerLoadOptions): Promise<void>;
  destroy(): void;
}

/* ------------------ Helpers ------------------ */

/**
 * Convenience error factory (kept for compatibility).
 */
export function makeError(
  message: string,
  partial?: Partial<PlayerError>,
  cause?: unknown
): PlayerError {
  return {
    message,
    fatal: partial?.fatal,
    cause: cause ?? partial?.cause,
    code: partial?.code,
    category: partial?.category ?? 'unknown',
    meta: partial?.meta,
  };
}

/* ------------------ Deprecated Exports ------------------
 * These are left as no-ops or placeholders to avoid breaking imports
 * elsewhere that may still reference them.
 */

/**
 * Deprecated: Multi-adapter priority map (no longer used).
 */
export const DEFAULT_ADAPTER_PRIORITY: Record<string, number> = {
  'mpegts.js': 1,
};

/**
 * Deprecated: Sorting helper (returns array unchanged).
 */
export function sortAdaptersByPriority<T extends PlayerAdapter>(adapters: T[]): T[] {
  return adapters;
}
