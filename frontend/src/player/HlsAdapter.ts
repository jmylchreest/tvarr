/**
 * HlsAdapter.ts
 * -------------
 * PlayerAdapter implementation wrapping hls.js for HLS streams.
 * This adapter is prepared for future use when HLS playback (fMP4/HLS)
 * is needed alongside MPEG-TS playback.
 *
 * Features:
 *  - Dynamic import of hls.js (code-splitting friendly)
 *  - X-Tvarr-Player header injection via xhrSetup for smart container routing
 *  - Unified error + media info callbacks following PlayerAdapter interface
 *  - Clean, idempotent teardown
 *
 * Note: Currently, the application is simplified to expect MPEG-TS streams
 * from the backend. This adapter is provided for future use when the
 * smart container routing feature enables HLS output formats.
 *
 * Usage (future):
 *   const adapter = new HlsAdapter();
 *   await adapter.load({
 *     url: 'http://example.com/stream.m3u8',
 *     videoEl: document.querySelector('video'),
 *     onError: (err) => console.error(err),
 *     onMediaInfo: (info) => console.log(info),
 *   });
 */

import type {
  PlayerAdapter,
  PlayerLoadOptions,
  PlaybackEnv,
  PlayerError,
  PlayerMediaInfo,
  StreamKind,
} from './PlayerAdapter';
import { PLAYER_HEADER_NAME } from '../lib/player-headers';

export interface HlsAdapterConfig {
  /** Enable debug logging in hls.js. Default: false */
  enableDebug?: boolean;
  /** Maximum buffer length in seconds. Default: 30 */
  maxBufferLength?: number;
  /** Maximum maximum buffer length. Default: 600 */
  maxMaxBufferLength?: number;
  /** Additional headers to send with stream requests */
  headers?: Record<string, string>;
  /** Config overrides passed directly to hls.js */
  overrides?: Record<string, unknown>;
}

export class HlsAdapter implements PlayerAdapter {
  public readonly name = 'hls.js';

  private hls: any | null = null;
  private videoEl: HTMLVideoElement | null = null;
  private currentUrl: string | null = null;
  private destroyed = false;

  private readonly cfg: Required<HlsAdapterConfig>;

  constructor(cfg?: HlsAdapterConfig) {
    this.cfg = {
      enableDebug: cfg?.enableDebug ?? false,
      maxBufferLength: cfg?.maxBufferLength ?? 30,
      maxMaxBufferLength: cfg?.maxMaxBufferLength ?? 600,
      headers: cfg?.headers ?? { [PLAYER_HEADER_NAME]: 'hls.js' },
      overrides: cfg?.overrides ?? {},
    };
  }

  /**
   * Suitability: Claim HLS_PLAYLIST streams when MSE is supported.
   * Does not claim RAW_TS (that's for MpegTsAdapter).
   */
  canPlay(kind: StreamKind, _url: string, env: PlaybackEnv): boolean {
    if (kind !== 'HLS_PLAYLIST') return false;
    // hls.js requires MSE support (not needed for Safari which has native HLS)
    return env.mseSupported && !env.isSafari;
  }

  async load(options: PlayerLoadOptions): Promise<void> {
    const { url, videoEl, onError, onMediaInfo, log } = options;

    // Clean any prior instance
    this.internalDestroy(false);

    this.videoEl = videoEl;
    this.currentUrl = url;
    this.destroyed = false;

    if (!this.videoEl) {
      onError(this.err('Video element missing', { fatal: true, category: 'media' }));
      return;
    }

    // Dynamic import for tree-shaking / smaller initial bundle
    let HlsModule: any;
    try {
      HlsModule = (await import('hls.js')).default;
    } catch (e) {
      onError(
        this.err('Failed to load hls.js library', {
          fatal: true,
          category: 'media',
          cause: e,
        })
      );
      return;
    }

    if (!HlsModule || !HlsModule.isSupported()) {
      onError(
        this.err('hls.js not supported in this environment', {
          fatal: true,
          category: 'media',
        })
      );
      return;
    }

    // Sanitize the video element
    try {
      this.videoEl.pause();
      this.videoEl.removeAttribute('src');
      this.videoEl.load();
      this.videoEl.preload = 'auto';
      this.videoEl.playsInline = true;
      this.videoEl.controls = false;
    } catch (e) {
      log?.warn?.('[HlsAdapter] Failed to reset video element', e);
    }

    // Build hls.js config with X-Tvarr-Player header injection
    const headers = this.cfg.headers;
    const hlsConfig = {
      debug: this.cfg.enableDebug,
      maxBufferLength: this.cfg.maxBufferLength,
      maxMaxBufferLength: this.cfg.maxMaxBufferLength,
      enableWorker: true,
      lowLatencyMode: false,
      // Inject X-Tvarr-Player header on all XHR requests
      xhrSetup: (xhr: XMLHttpRequest, _url: string) => {
        for (const [key, value] of Object.entries(headers)) {
          xhr.setRequestHeader(key, value);
        }
      },
      ...this.cfg.overrides,
    };

    // Create hls.js instance
    try {
      this.hls = new HlsModule(hlsConfig);
    } catch (e) {
      onError(
        this.err('Failed to create hls.js instance', {
          fatal: true,
          category: 'media',
          cause: e,
        })
      );
      return;
    }

    // MANIFEST_PARSED event - stream is ready to play
    this.hls.on(HlsModule.Events.MANIFEST_PARSED, (_event: string, data: any) => {
      if (this.destroyed) return;
      log?.debug?.('[HlsAdapter] MANIFEST_PARSED', data);

      // Extract media info from levels
      const levels = data?.levels || [];
      const level = levels[0];
      if (level) {
        const mediaInfo: PlayerMediaInfo = {
          videoCodec: level.videoCodec,
          audioCodec: level.audioCodec,
          width: level.width,
          height: level.height,
          fps: level.frameRate,
          live: data?.live,
          raw: data,
        };
        onMediaInfo?.(mediaInfo);
      }

      // Attempt autoplay
      this.videoEl
        ?.play()
        .catch((e: any) =>
          log?.warn?.('[HlsAdapter] Autoplay blocked / awaiting user interaction', e)
        );
    });

    // LEVEL_LOADED - additional media info updates
    this.hls.on(HlsModule.Events.LEVEL_LOADED, (_event: string, data: any) => {
      if (this.destroyed) return;
      log?.debug?.('[HlsAdapter] LEVEL_LOADED', data);

      const details = data?.details;
      if (details) {
        const mediaInfo: PlayerMediaInfo = {
          live: details.live,
          raw: data,
        };
        onMediaInfo?.(mediaInfo);
      }
    });

    // ERROR event
    this.hls.on(HlsModule.Events.ERROR, (_event: string, data: any) => {
      if (this.destroyed) return;

      const { type, details, fatal } = data;
      log?.error?.('[HlsAdapter] ERROR', { type, details, fatal });

      onError(
        this.err(`hls.js error: ${type}${details ? ` (${details})` : ''}`, {
          fatal,
          category: this.mapCategory(type, details),
          code: details,
          cause: data,
          meta: { type, details },
        })
      );

      // If fatal, try to recover
      if (fatal) {
        switch (type) {
          case HlsModule.ErrorTypes.NETWORK_ERROR:
            log?.warn?.('[HlsAdapter] Attempting to recover from network error');
            this.hls?.startLoad();
            break;
          case HlsModule.ErrorTypes.MEDIA_ERROR:
            log?.warn?.('[HlsAdapter] Attempting to recover from media error');
            this.hls?.recoverMediaError();
            break;
          default:
            // Cannot recover, surface error
            break;
        }
      }
    });

    // Attach to video element and load source
    try {
      this.hls.attachMedia(this.videoEl);
      this.hls.loadSource(url);
    } catch (e) {
      onError(
        this.err('Failed to start HLS playback', {
          fatal: true,
          category: 'media',
          cause: e,
        })
      );
    }
  }

  destroy(): void {
    this.internalDestroy(true);
  }

  /* ---------------- Internal Helpers ---------------- */

  private internalDestroy(markDestroyed: boolean) {
    if (markDestroyed) this.destroyed = true;

    if (this.hls) {
      try {
        this.hls.stopLoad?.();
      } catch {
        /* ignore */
      }
      try {
        this.hls.detachMedia?.();
      } catch {
        /* ignore */
      }
      try {
        this.hls.destroy?.();
      } catch {
        /* ignore */
      }
    }

    if (this.videoEl) {
      try {
        this.videoEl.pause();
        this.videoEl.removeAttribute('src');
        this.videoEl.load();
      } catch {
        /* ignore */
      }
    }

    this.hls = null;
    this.videoEl = null;
    this.currentUrl = null;
  }

  private mapCategory(type: string, details: string): PlayerError['category'] {
    const s = `${type}:${details}`.toLowerCase();
    if (s.includes('network')) return 'network';
    if (s.includes('media') || s.includes('mux')) return 'media';
    if (s.includes('frag') || s.includes('buffer')) return 'decode';
    if (s.includes('level')) return 'media';
    return 'unknown';
  }

  private err(message: string, partial?: Partial<PlayerError>): PlayerError {
    return {
      message,
      fatal: partial?.fatal,
      cause: partial?.cause,
      code: partial?.code,
      category: partial?.category ?? 'unknown',
      meta: partial?.meta,
    };
  }
}
