'use client';
/**
 * SimpleTsPlayer.tsx
 * -------------------
 * Minimal React component for playing a continuous MPEG-TS live stream using the existing
 * MpegTsAdapter (which wraps mpegts.js). All multi-adapter / HLS detection logic has been
 * removed; the backend is expected to deliver a TS stream (`video/mp2t`) either by raw
 * passthrough or by collapsing a single-variant HLS playlist.
 *
 * Added:
 *  - onVideoReady?: (video: HTMLVideoElement) => void;
 *    Invoked exactly once when the underlying <video> element is created / resolved
 *    and before the adapter load/autoplay attempt finishes (so caller can, e.g.,
 *    attach extra listeners or do metrics).
 * Responsibilities:
 *  - Instantiate and load `MpegTsAdapter` on mount / src change.
 *  - Expose basic lifecycle callbacks (onError, onMediaInfo).
 *  - Attempt autoplay (best effort).
 *  - Provide a minimal optional overlay with channel name + status.
 *
 * Non-Goals:
 *  - No retry / fallback logic here (surface errors upward).
 *  - No adaptive bitrate / HLS support (intentionally removed).
 *  - No advanced buffering telemetry (can be added later if needed).
 *
 * Usage:
 *   <SimpleTsPlayer
 *     src={streamUrl}
 *     channelName="Channel 42"
 *     autoPlay
 *     onError={(e) => console.error(e)}
 *   />
 *
 * Notes:
 *  - If mpegts.js is not supported in the environment, an error callback fires and a simple
 *    inline message is displayed.
 *  - Keep this component *lean*; operational overlays / debug panes can be layered outside.
 */

import React, { useRef, useEffect, useState, useCallback, CSSProperties, memo } from 'react';
import { MpegTsAdapter } from './MpegTsAdapter';
import type { PlayerError, PlayerMediaInfo } from './PlayerAdapter';

export interface SimpleTsPlayerProps {
  /** Live MPEG-TS stream URL (already proxied/hybrid-normalized). */
  src: string;
  /** Attempt autoplay after load (best effort). Default: true. */
  autoPlay?: boolean;
  /** Show a lightweight overlay with channel name + status. Default: true. */
  showInternalOverlay?: boolean; // renamed from showOverlay for clarity
  /** If true, use native browser video controls instead of the custom overlay controls. Default: false. */
  useNativeControls?: boolean;
  /** Optional channel name badge. */
  channelName?: string;
  /** Called on first (or subsequent) adapter media info update. */
  onMediaInfo?: (info: PlayerMediaInfo) => void;
  /** Structured error callback. */
  onError?: (err: PlayerError) => void;
  /** CSS className for <video>. */
  className?: string;
  /** Inline style for <video>. */
  style?: CSSProperties;
  /** Extra overlay className. */
  overlayClassName?: string;
  /** Enable console debug logs. */
  debug?: boolean;
  /** If true, renders a very small live badge (default: true). */
  liveBadge?: boolean;
  /** If true, hides the fallback error message (caller can render its own). */
  suppressInlineError?: boolean;
  /** Server decision token (e.g., raw_ts_direct, hls_collapse_single_ts). */
  decision?: string | null;
  /** Callback fired once when the underlying <video> element is ready (created or resolved). */
  onVideoReady?: (video: HTMLVideoElement) => void;
  /** Playback state change (derived from video play/pause/ended). */
  onPlayStateChange?: (playing: boolean) => void;
}

interface LoadState {
  phase: 'idle' | 'loading' | 'ready' | 'error';
  message?: string;
}

export const SimpleTsPlayer: React.FC<SimpleTsPlayerProps> = memo(
  ({
    src,
    autoPlay = true,
    showInternalOverlay = true,
    useNativeControls = false,
    channelName,
    onMediaInfo,
    onError,
    className,
    style,
    overlayClassName,
    debug = false,
    liveBadge = true,
    suppressInlineError = false,
    decision,
    onVideoReady,
    onPlayStateChange,
  }) => {
    const videoRef = useRef<HTMLVideoElement | null>(null);
    const adapterRef = useRef<MpegTsAdapter | null>(null);
    const mountRef = useRef<HTMLDivElement | null>(null);
    const retryTimerRef = useRef<number | null>(null);
    const [state, setState] = useState<LoadState>({ phase: 'idle' });
    const [firstMediaInfo, setFirstMediaInfo] = useState<PlayerMediaInfo | null>(null);
    const [lastError, setLastError] = useState<PlayerError | null>(null);

    const log = useCallback(
      (...a: unknown[]) => {
        if (debug) {
          // eslint-disable-next-line no-console
          console.debug('[SimpleTsPlayer]', ...a);
        }
      },
      [debug]
    );

    const fail = useCallback(
      (err: PlayerError) => {
        setState({ phase: 'error', message: err.message });
        setLastError(err);
        onError?.(err);
        log('Error', err);
      },
      [onError, log]
    );

    // Attempt to resolve a <video> element if not yet assigned (SSR / hydration guard)
    const ensureVideoElement = useCallback((): HTMLVideoElement | null => {
      if (videoRef.current) return videoRef.current;
      if (!mountRef.current) return null;
      const maybe = mountRef.current.querySelector('video');
      if (maybe instanceof HTMLVideoElement) {
        videoRef.current = maybe;
        return maybe;
      }
      return null;
    }, []);

    // Load / reload effect
    useEffect(() => {
      if (!src) {
        setState({ phase: 'error', message: 'No stream source provided' });
        return;
      }

      let canceled = false;

      async function load() {
        setState({ phase: 'loading' });
        setLastError(null);
        setFirstMediaInfo(null);

        // Tear down any existing adapter first
        if (adapterRef.current) {
          try {
            adapterRef.current.destroy();
          } catch {
            /* ignore */
          }
          adapterRef.current = null;
        }

        let videoEl = ensureVideoElement();
        if (!videoEl) {
          // Create video element if it does not exist (defensive SSR/hydration fallback)
          if (mountRef.current) {
            const v = document.createElement('video');
            v.playsInline = true;
            v.muted = false;
            // Always disable native controls to avoid overlay conflicts
            v.removeAttribute('controls');
            v.controls = false;
            v.preload = 'auto';
            v.style.width = '100%';
            v.style.height = '100%';
            mountRef.current.appendChild(v);
            videoRef.current = v;
            videoEl = v;
            // Fire onVideoReady as soon as we physically create the element
            try {
              onVideoReady?.(v);
            } catch {
              /* swallow */
            }
          }
        }
        if (!videoEl) {
          // Schedule a short retry (browser may not have hydrated yet)
          if (!retryTimerRef.current) {
            retryTimerRef.current = window.setTimeout(() => {
              retryTimerRef.current = null;
              load(); // retry
            }, 100);
          }
          return;
        }

        // Basic attribute resets
        try {
          videoEl.pause();
          videoEl.removeAttribute('src');
          videoEl.load();
          // Always disable native controls to avoid overlay conflicts
          videoEl.removeAttribute('controls');
          videoEl.controls = false;
          videoEl.playsInline = true;
          videoEl.preload = 'auto';
          // Ensure callback fires if video element pre-existed (SSR/HMR path)
          try {
            onVideoReady?.(videoEl);
          } catch {
            /* ignore */
          }
        } catch {
          /* non-fatal */
        }

        const adapter = new MpegTsAdapter({ enableWorker: false }); // disable worker to avoid CSP worker creation violation
        adapterRef.current = adapter;

        try {
          await adapter.load({
            url: src,
            videoEl,
            onError: (err) => {
              // Adapter's error callback may fire multiple times; treat first fatal as terminal
              if (err.fatal) {
                if (!canceled) fail(err);
              } else {
                onError?.(err);
              }
            },
            onMediaInfo: (info) => {
              if (!firstMediaInfo) {
                setFirstMediaInfo(info);
              }
              onMediaInfo?.(info);
            },
            log: {
              debug: (...a) => log(...a),
              info: (...a) => log(...a),
              warn: (...a) => log(...a),
              error: (...a) => log(...a),
            },
          });

          if (canceled) return;

          // Try autoplay (non-fatal)
          if (autoPlay) {
            videoEl
              .play()
              .then(() => {
                try {
                  onPlayStateChange?.(true);
                } catch {
                  /* ignore */
                }
              })
              .catch((e) => log('Autoplay blocked; awaiting user gesture', e));
          }

          setState({ phase: 'ready' });
        } catch (e: any) {
          fail({
            message: e?.message || 'Failed to start MPEG-TS playback',
            fatal: true,
            cause: e,
            category: 'media',
          });
        }
      }

      load();

      return () => {
        canceled = true;
        if (retryTimerRef.current) {
          window.clearTimeout(retryTimerRef.current);
          retryTimerRef.current = null;
        }
        if (adapterRef.current) {
          try {
            adapterRef.current.destroy();
          } catch {
            /* ignore */
          }
          adapterRef.current = null;
        }
      };
      // Intentionally re-run ONLY when src changes
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [src, useNativeControls]);

    // Keep native controls state in sync if parent toggles prop during runtime
    useEffect(() => {
      const v = videoRef.current;
      if (!v) return;
      // Always disable native controls to avoid overlay conflicts
      v.removeAttribute('controls');
      v.controls = false;
    }, [useNativeControls]);

    // Emit play state changes upward
    useEffect(() => {
      const v = videoRef.current;
      if (!v || !onPlayStateChange) return;
      const emit = () => {
        try {
          onPlayStateChange(!v.paused && !v.ended);
        } catch {
          /* ignore */
        }
      };
      v.addEventListener('play', emit);
      v.addEventListener('playing', emit);
      v.addEventListener('pause', emit);
      v.addEventListener('ended', emit);
      // Initial state emission
      emit();
      return () => {
        v.removeEventListener('play', emit);
        v.removeEventListener('playing', emit);
        v.removeEventListener('pause', emit);
        v.removeEventListener('ended', emit);
      };
    }, [onPlayStateChange]);

    const overlay = showInternalOverlay ? (
      <div
        className={`sp-overlay ${overlayClassName || ''}`}
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          //          right: 0, // removed to avoid covering top-right exit/close button
          pointerEvents: 'none', // allow clicks to pass to underlying video
          display: 'flex',
          flexDirection: 'row',
          gap: '8px',
          padding: '6px 8px',
          fontFamily: 'system-ui, sans-serif',
          fontSize: 12,
          fontWeight: 500,
          color: '#fff',
          textShadow: '0 1px 2px rgba(0,0,0,0.65)',
        }}
      >
        {channelName && <Badge label={channelName} tone="primary" />}
        {decision && decisionBadge(decision)}
        {liveBadge && <Badge label="LIVE" tone="danger" />}
        <Badge
          label={
            state.phase === 'ready'
              ? firstMediaInfo
                ? formatResolution(firstMediaInfo) || 'TS'
                : 'TS'
              : state.phase.toUpperCase()
          }
          tone={state.phase === 'error' ? 'danger' : 'neutral'}
        />
        {lastError && <Badge label={trimError(lastError.message)} tone="danger" />}
      </div>
    ) : null;

    return (
      <div
        className="simple-ts-player"
        ref={mountRef}
        style={{
          position: 'relative',
          width: '100%',
          height: '100%',
          background: '#000',
          overflow: 'hidden',
        }}
      >
        {/* Always render a stable video element for mounting / hydration */}
        <video
          ref={videoRef}
          className={className}
          style={{
            width: '100%',
            height: '100%',
            objectFit: 'contain',
            pointerEvents: 'auto',
            background: '#000',
            ...style,
          }}
          playsInline
          muted={false}
          autoPlay={false}
          controls={false}
        />
        {overlay}
        {state.phase === 'error' && !suppressInlineError && (
          <div
            style={{
              position: 'absolute',
              inset: 0,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              background: 'linear-gradient(to bottom right, rgba(0,0,0,0.65), rgba(0,0,0,0.85))',
              color: '#fff',
              fontFamily: 'system-ui, sans-serif',
              fontSize: 14,
              padding: 16,
              textAlign: 'center',
            }}
          >
            <div>
              <strong>Stream Error</strong>
              <div style={{ marginTop: 8 }}>{state.message || 'Playback failed'}</div>
            </div>
          </div>
        )}
        {/* Hidden description element to satisfy dialog accessibility expectations */}
        <span
          style={{
            position: 'absolute',
            width: 1,
            height: 1,
            padding: 0,
            margin: -1,
            overflow: 'hidden',
            clip: 'rect(0 0 0 0)',
            whiteSpace: 'nowrap',
            border: 0,
          }}
          data-radix-dialog-description=""
        >
          Live MPEG-TS video stream player
        </span>
      </div>
    );
  }
);

SimpleTsPlayer.displayName = 'SimpleTsPlayer';

/* ----------------- Small Helpers ----------------- */

function formatResolution(info: PlayerMediaInfo): string | null {
  if (info?.width && info?.height) {
    return `${info.width}x${info.height}`;
  }
  return null;
}

function trimError(msg: string, max = 48): string {
  if (!msg) return 'error';
  return msg.length > max ? msg.slice(0, max - 1) + '…' : msg;
}

/* ----------------- Badge Component ----------------- */

function decisionBadge(decision: string) {
  const d = decision.toLowerCase();
  if (d.includes('hls-to-ts')) {
    return <Badge label="hls-to-ts" tone="primary" />;
  }
  if (d.includes('raw')) {
    return <Badge label="RAW" tone="neutral" />;
  }
  return null;
}

const Badge: React.FC<{ label: string; tone?: 'primary' | 'danger' | 'neutral' }> = ({
  label,
  tone = 'neutral',
}) => {
  const colors: Record<string, { bg: string; fg: string }> = {
    primary: { bg: 'rgba(0,120,255,0.75)', fg: '#fff' },
    danger: { bg: 'rgba(220,50,47,0.85)', fg: '#fff' },
    neutral: { bg: 'rgba(0,0,0,0.55)', fg: '#fff' },
  };
  const c = colors[tone] || colors.neutral;
  return (
    <span
      style={{
        display: 'inline-block',
        padding: '2px 6px',
        borderRadius: 4,
        background: c.bg,
        color: c.fg,
        lineHeight: 1.2,
        letterSpacing: 0.5,
        whiteSpace: 'nowrap',
        maxWidth: 160,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
      }}
      title={label}
    >
      {label}
    </span>
  );
};

export default SimpleTsPlayer;
