/**
 * MpegTsAdapter.ts
 * -----------------
 * PlayerAdapter implementation wrapping mpegts.js for continuous / raw MPEG-TS
 * (video/mp2t) streams. DO NOT feed HLS playlists (.m3u8) to this adapter—
 * those should be handled by native HLS (Safari) or an HLS JS adapter.
 *
 * Features:
 *  - Dynamic import of mpegts.js (code-splitting friendly)
 *  - Sensible live defaults (stash buffering, worker, conservative latency handling)
 *  - Unified error + media info callbacks
 *  - Clean, idempotent teardown
 *
 * Not handled here:
 *  - Automatic retry / fallback to other adapters (controller’s responsibility)
 *  - HEVC / codec support probing (handled elsewhere)
 */

import type {
  PlayerAdapter,
  PlayerLoadOptions,
  PlaybackEnv,
  PlayerError,
  PlayerMediaInfo,
  StreamKind,
} from './PlayerAdapter';

export interface MpegTsAdapterConfig {
  enableWorker?: boolean;
  stashInitialSize?: number;
  stashBufferThreshold?: number;
  appendErrorMaxRetry?: number;
  disableLatencyChasing?: boolean;
  overrides?: Record<string, any>;
}

export class MpegTsAdapter implements PlayerAdapter {
  public readonly name = 'mpegts.js';

  private player: any | null = null;
  private videoEl: HTMLVideoElement | null = null;
  private currentUrl: string | null = null;
  private destroyed = false;

  private readonly cfg: Required<MpegTsAdapterConfig>;

  constructor(cfg?: MpegTsAdapterConfig) {
    this.cfg = {
      enableWorker: cfg?.enableWorker ?? true,
      stashInitialSize: cfg?.stashInitialSize ?? 384 * 1024, // ~384KiB
      stashBufferThreshold: cfg?.stashBufferThreshold ?? 768 * 1024,
      appendErrorMaxRetry: cfg?.appendErrorMaxRetry ?? 6,
      disableLatencyChasing: cfg?.disableLatencyChasing ?? true,
      overrides: cfg?.overrides ?? {},
    };
  }

  /**
   * Suitability: Only claim RAW_TS. The controller may fall back to this for UNKNOWN kinds
   * in the future, but for now we remain strict to avoid mis-feeding.
   */
  canPlay(kind: StreamKind, _url: string, env: PlaybackEnv): boolean {
    if (kind !== 'RAW_TS') return false;
    return env.mseSupported;
  }

  async load(options: PlayerLoadOptions): Promise<void> {
    const { url, videoEl, onError, onMediaInfo, log } = options;

    // Clean any prior instance BEFORE assigning the new video element.
    // Previously the order caused internalDestroy(false) to null out the
    // freshly assigned this.videoEl, triggering a false "Video element missing".
    this.internalDestroy(false);

    this.videoEl = videoEl;
    this.currentUrl = url;
    this.destroyed = false;

    if (!this.videoEl) {
      onError(this.err('Video element missing', { fatal: true, category: 'media' }));
      return;
    }

    // Dynamic import for tree-shaking / smaller initial bundle
    let mpegtsMod: any;
    try {
      mpegtsMod = (await import('mpegts.js')).default;
    } catch (e) {
      onError(
        this.err('Failed to load mpegts.js library', {
          fatal: true,
          category: 'media',
          cause: e,
        })
      );
      return;
    }

    if (!mpegtsMod || !mpegtsMod.isSupported()) {
      onError(
        this.err('mpegts.js not supported in this environment', {
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
      (this.videoEl as any).disablePictureInPicture = true;
      this.videoEl.controls = false;
      this.videoEl.removeAttribute('controls');
    } catch (e) {
      log?.warn?.('[MpegTsAdapter] Failed to reset video element', e);
    }

    const baseConfig = {
      enableWorker: this.cfg.enableWorker,
      enableStashBuffer: true,
      stashInitialSize: this.cfg.stashInitialSize,
      stashBufferThreshold: this.cfg.stashBufferThreshold,
      isLive: true,
      liveBufferLatencyChasing: !this.cfg.disableLatencyChasing,
      enableStatisticsInfo: true,
      autoCleanupSourceBuffer: true,
      autoCleanupMaxBackwardDuration: 40,
      autoCleanupMinBackwardDuration: 15,
      fixAudioTimestampGap: true,
      reuseRedirectedURL: true,
      appendErrorMaxRetry: this.cfg.appendErrorMaxRetry,
      ...this.cfg.overrides,
    };

    const mediaDataSource = {
      type: 'mpegts',
      isLive: true,
      hasAudio: true,
      hasVideo: true,
      url,
    };

    // Create player
    try {
      this.player = mpegtsMod.createPlayer(mediaDataSource, baseConfig);
    } catch (e) {
      onError(
        this.err('Failed to create mpegts.js player', {
          fatal: true,
          category: 'media',
          cause: e,
        })
      );
      return;
    }

    // Attach <video>
    try {
      this.player.attachMediaElement(this.videoEl);
    } catch (e) {
      onError(
        this.err('Failed to attach media element', {
          fatal: true,
          category: 'media',
          cause: e,
        })
      );
      return;
    }

    // MEDIA_INFO event
    this.player.on(mpegtsMod.Events.MEDIA_INFO, (info: any) => {
      if (this.destroyed) return;
      const mediaInfo: PlayerMediaInfo = {
        videoCodec:
          info?.video?.codec ||
          info?.video?.codecName ||
          info?.videoTracks?.[0]?.codecName ||
          undefined,
        audioCodec:
          info?.audio?.codec ||
          info?.audio?.codecName ||
          info?.audioTracks?.[0]?.codecName ||
          undefined,
        width: info?.video?.presentWidth || info?.video?.width || undefined,
        height: info?.video?.presentHeight || info?.video?.height || undefined,
        fps: info?.video?.fps || info?.framerate || undefined,
        live: true,
        raw: info,
      };
      onMediaInfo?.(mediaInfo);
      log?.debug?.('[MpegTsAdapter] MEDIA_INFO', mediaInfo);
    });

    // LOADING_COMPLETE
    this.player.on(mpegtsMod.Events.LOADING_COMPLETE, () => {
      if (this.destroyed) return;
      log?.info?.('[MpegTsAdapter] LOADING_COMPLETE');
      // Attempt autoplay (non-fatal if blocked)
      this.videoEl
        ?.play()
        .catch((e: any) =>
          log?.warn?.('[MpegTsAdapter] Autoplay blocked / awaiting user interaction', e)
        );
    });

    // ERROR
    this.player.on(
      mpegtsMod.Events.ERROR,
      (type: string, details: string, data: unknown | undefined) => {
        if (this.destroyed) return;
        log?.error?.('[MpegTsAdapter] ERROR', { type, details, data });

        const fatal = this.isFatal(type, details);
        onError(
          this.err(`mpegts.js error: ${type}${details ? ` (${details})` : ''}`, {
            fatal,
            category: this.mapCategory(type, details),
            code: type,
            cause: data,
            meta: { details },
          })
        );
      }
    );

    // Start streaming
    try {
      this.player.load();
      // Second attempt to play (some browsers require it after attach+load)
      this.videoEl
        ?.play()
        .catch((e: any) => log?.debug?.('[MpegTsAdapter] Deferred play attempt failed', e));
    } catch (e) {
      onError(
        this.err('Failed to start MPEG-TS playback', {
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

    if (this.player) {
      try {
        this.player.unload?.();
      } catch {
        /* ignore */
      }
      try {
        this.player.detachMediaElement?.();
      } catch {
        /* ignore */
      }
      try {
        this.player.destroy?.();
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

    this.player = null;
    this.videoEl = null;
    this.currentUrl = null;
  }

  private isFatal(type: string, details: string): boolean {
    const s = `${type}:${details}`.toLowerCase();
    if (s.includes('unsupported')) return true;
    if (s.includes('demux') || s.includes('exception')) return true;
    // Append / network issues can be transient (retry logic belongs higher up)
    return false;
  }

  private mapCategory(type: string, details: string): PlayerError['category'] {
    const s = `${type}:${details}`.toLowerCase();
    if (s.includes('network')) return 'network';
    if (s.includes('media') || s.includes('append')) return 'media';
    if (s.includes('demux') || s.includes('codec')) return 'decode';
    if (s.includes('time') || s.includes('pts') || s.includes('dts')) return 'timing';
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

/**
 * Builder for PlaybackEnv (optional usage).
 */
export function buildPlaybackEnv(): PlaybackEnv {
  if (typeof navigator === 'undefined') {
    return { isSafari: false, isMobile: false, mseSupported: false };
  }
  const ua = navigator.userAgent;
  const isSafari =
    /Safari/i.test(ua) && !/Chrome|CriOS|Chromium|Edg/i.test(ua) && !/Android/i.test(ua);
  const isMobile = /Mobi|Android|iPhone|iPad|iPod/i.test(ua);
  const mseSupported = typeof (window as any).MediaSource !== 'undefined';
  return { isSafari, isMobile, mseSupported, ua };
}
