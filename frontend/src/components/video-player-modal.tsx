'use client';
// NOTE: Unable to safely produce minimal surgical edits without a fresh authoritative
// copy of the file including line numbers. Please provide the current file content
// (or at least the top ~900 lines with line numbers) so I can generate correct
// <old_text>/<new_text> replacements. The diff format requires exact matching text.
// Once supplied, I will:
// 1. Remove reloadCounter and any remaining references.
// 2. Replace complex sync + mutation observer with:
//    - onVideoReady immediate state prime
//    - play / pause listeners only
//    - one short rAF loop (up to 1s) to catch autoplay
//    - a final 300ms timeout forced sync
// 3. Add console.debug instrumentation for: onVideoReady, play, pause, autoplay attempts.
// 4. Ensure aria-describedby points to existing element (resolve warning).
// 5. Remove unused imports (RefreshCw) and related JSX.
// Please re‑send the file so I can emit a valid edit block.

/**
 * VideoPlayerModal
 * ----------------
 * Modal container providing UI (title, metadata, controls) around a live MPEG-TS stream
 * rendered by the simplified SimpleTsPlayer (MPEG-TS only).
 *
 * Key Features:
 *  - Channel + Program display (title, subtitle, description).
 *  - Copy stream URL / external player playlist export.
 *  - Codec & HEVC indication (heuristic).
 *  - Error overlay + retry.
 *  - Basic play/pause, mute, fullscreen, reload, URL export.
 *
 * Notes:
 *  - All previous multi-adapter / PlayerController logic removed.
 *  - Client-side HLS / stream kind detection eliminated; server normalizes or passthroughs.
 *  - Buffer health/UI hooks can be extended later if needed.
 */

import React, { useCallback, useEffect, useMemo, useRef, useState, forwardRef } from 'react';

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { ScrollArea } from '@/components/ui/scroll-area';
import {
  X,
  Copy,
  ExternalLink,
  Check,
  RefreshCw,
  Play,
  Pause,
  Volume2,
  VolumeX,
  Maximize2,
  Minimize2,
  Tv,
  AlertTriangle,
} from 'lucide-react';

import { SimpleTsPlayer } from '../player/SimpleTsPlayer';
import type { PlayerMediaInfo } from '../player/PlayerAdapter';
import { Debug } from '@/utils/debug';

/* ---------- Types ---------- */

interface Channel {
  id: string;
  name: string;
  logo_url?: string;
  group?: string;
  stream_url: string;
  /**
   * Original upstream (unproxied) URL captured before replacing with the proxy
   * endpoint, used for heuristic stream-kind detection (extension based).
   */
  original_stream_url?: string;
  source_type: string;
  source_name?: string;
}

interface EpgProgram {
  id: string;
  channel_id: string;
  channel_name: string;
  title: string;
  description?: string;
  start_time: string;
  end_time: string;
  category?: string;
  stream_url?: string;
}

interface VideoPlayerModalProps {
  isOpen: boolean;
  onClose: () => void;
  channel?: Channel;
  program?: EpgProgram;
}

/* ---------- Component ---------- */

export function VideoPlayerModal({ isOpen, onClose, channel, program }: VideoPlayerModalProps) {
  const videoElRef = useRef<HTMLVideoElement | null>(null); // retained for future direct video access if needed
  const debug = useMemo(() => Debug.createLogger('VideoPlayerModal'), []);

  // UI state
  const [copySuccess, setCopySuccess] = useState(false);
  const [isMuted, setIsMuted] = useState(false);
  const [isPlaying, setIsPlaying] = useState(false);
  const isPlayingRef = useRef(false); // mirror state for sync
  const [volume, setVolume] = useState(1); // controlled volume state

  // Keep ref in sync with state (prevents stale ref logic)
  useEffect(() => {
    isPlayingRef.current = isPlaying;
  }, [isPlaying]);
  const [lastError, setLastError] = useState<string | null>(null);
  const [mediaInfo, setMediaInfo] = useState<PlayerMediaInfo | null>(null);
  const [showControls, setShowControls] = useState(true);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [isHevc, setIsHevc] = useState(false);
  const [bufferSeconds, setBufferSeconds] = useState<number | null>(null); // retained (legacy - not updated after simplification)
  const [reloadCounter, setReloadCounter] = useState(0);

  // Hover / inactivity timer for fading controls
  const hideTimerRef = useRef<number | null>(null);

  const streamUrl = channel?.stream_url || program?.stream_url || '';

  // Detected stream kind from server headers (X-Stream-Origin-Kind)
  const [streamKind, setStreamKind] = useState<'RAW_TS' | 'HLS_PLAYLIST' | 'UNKNOWN' | null>(null);
  const [streamDecision, setStreamDecision] = useState<string | null>(null);

  // Fetch stream classification headers (HEAD) when stream changes / modal opens / reloads
  useEffect(() => {
    if (!isOpen || !streamUrl) {
      setStreamKind(null);
      setStreamDecision(null);
      return;
    }
    let canceled = false;
    setStreamKind(null);
    setStreamDecision(null);

    (async () => {
      try {
        const res = await fetch(streamUrl, { method: 'HEAD', cache: 'no-cache' });
        if (canceled) return;
        const kind = (res.headers.get('X-Stream-Origin-Kind') || '').toUpperCase();
        const decision = res.headers.get('X-Stream-Decision');
        if (kind === 'HLS_PLAYLIST') {
          setStreamKind('HLS_PLAYLIST');
        } else if (kind === 'RAW_TS') {
          setStreamKind('RAW_TS');
        } else {
          setStreamKind('UNKNOWN');
        }
        if (decision) setStreamDecision(decision);
      } catch {
        if (!canceled) {
          // Fall back – assume TS if URL ends in .ts
          if (/\.ts(\?|$)/i.test(streamUrl)) {
            setStreamKind('RAW_TS');
          } else if (/\.m3u8(\?|$)/i.test(streamUrl)) {
            setStreamKind('HLS_PLAYLIST');
          } else {
            setStreamKind('UNKNOWN');
          }
        }
      }
    })();

    return () => {
      canceled = true;
    };
  }, [streamUrl, isOpen, reloadCounter]);

  const displayTitle = program?.title || channel?.name || 'Live Stream';
  const displaySubtitle = program
    ? `${program.channel_name} • ${new Date(program.start_time).toLocaleTimeString([], {
        hour: '2-digit',
        minute: '2-digit',
      })}`
    : channel?.group || undefined;

  /* ---------- Helpers ---------- */

  const clearHideTimer = () => {
    if (hideTimerRef.current) {
      window.clearTimeout(hideTimerRef.current);
      hideTimerRef.current = null;
    }
  };

  const scheduleHideControls = () => {
    clearHideTimer();
    hideTimerRef.current = window.setTimeout(() => {
      setShowControls(false);
    }, 5000); // 5s inactivity timeout
  };

  const bumpControls = useCallback(() => {
    setShowControls(true);
    scheduleHideControls();
  }, []);

  const handleMouseMove = useCallback(() => {
    bumpControls();
  }, [bumpControls]);

  const handleClose = useCallback(() => {
    if (videoElRef.current) {
      try {
        videoElRef.current.pause();
      } catch {
        /* ignore */
      }
    }
    onClose();
  }, [onClose]);

  const handleCopyUrl = useCallback(async () => {
    if (!streamUrl) return;
    try {
      await navigator.clipboard.writeText(makeAbsolute(streamUrl));
      setCopySuccess(true);
      setTimeout(() => setCopySuccess(false), 1500);
    } catch (e) {
      debug.error('Clipboard copy failed', e);
    }
  }, [streamUrl, debug]);

  const handleExternalPlayer = useCallback(async () => {
    if (!streamUrl) return;
    try {
      const absoluteUrl = makeAbsolute(streamUrl);
      const playlist = `#EXTM3U
#EXT-X-VERSION:3
#EXTINF:0,
${absoluteUrl}
`;
      const blob = new Blob([playlist], {
        type: 'application/vnd.apple.mpegurl',
      });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `${(displayTitle || 'stream').replace(/[^a-zA-Z0-9_-]/g, '_')}.m3u8`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      setCopySuccess(true);
      setTimeout(() => setCopySuccess(false), 2000);
      URL.revokeObjectURL(url);
    } catch (e) {
      debug.error('External playlist export failed', e);
    }
  }, [streamUrl, displayTitle, debug]);

  const toggleMute = useCallback(() => {
    let vid = videoElRef.current;
    if (!vid && playerContainerRef.current) {
      const found = playerContainerRef.current.querySelector('video') as HTMLVideoElement | null;
      if (found) {
        videoElRef.current = found;
        vid = found;
      }
    }
    if (!vid) {
      debug.warn('toggleMute: video element not yet available');
      return;
    }
    vid.muted = !vid.muted;
    setIsMuted(vid.muted);
  }, [debug]);

  const togglePlay = useCallback(() => {
    let vid = videoElRef.current;
    if (!vid && playerContainerRef.current) {
      const found = playerContainerRef.current.querySelector('video') as HTMLVideoElement | null;
      if (found) {
        videoElRef.current = found;
        vid = found;
      }
    }
    if (!vid) {
      debug.warn('togglePlay: video element not yet available');
      return;
    }
    if (vid.paused) {
      vid
        .play()
        .then(() => setIsPlaying(true))
        .catch((e) => {
          debug.warn('Play() blocked (autoplay policy or user gesture required)', e);
          // Fallback: attempt a mute-then-play strategy (some browsers allow this)
          if (!vid.muted) {
            vid.muted = true;
            vid
              .play()
              .then(() => {
                setIsMuted(true);
                setIsPlaying(true);
              })
              .catch((err2) => debug.warn('Muted fallback play() also blocked', err2));
          }
        });
    } else {
      vid.pause();
      setIsPlaying(false);
    }
  }, [debug]);

  // Reload logic removed (no longer needed; stream plays continuously)

  // Fullscreen now targets the player container so overlays remain visible and we avoid
  // browser-native control interference that can appear when fullscreening the <video> itself.
  const toggleFullscreen = useCallback(() => {
    const container = playerContainerRef.current;
    if (!container) return;
    if (document.fullscreenElement) {
      document.exitFullscreen().catch(() => {});
    } else {
      container.requestFullscreen().catch(() => {});
    }
  }, []);

  // Removed PlayerController status handler – no longer needed after simplification

  const handleMediaInfo = useCallback((info: PlayerMediaInfo) => {
    setMediaInfo(info);
    const vc = (info.videoCodec || '').toLowerCase();
    if (/hevc|hvc1|hev1|h265|265/.test(vc)) {
      setIsHevc(true);
    }
    // Additional sync: if media info arrives and video element exists, refresh play state
    const vid = videoElRef.current;
    if (vid) {
      const playing = !vid.paused && !vid.ended;
      if (playing !== isPlayingRef.current) {
        setIsPlaying(playing);
      }
      setIsMuted(vid.muted);
      if (typeof vid.volume === 'number') {
        setVolume(vid.volume);
      }
    }
  }, []);

  // Container ref for locating the underlying <video> element rendered by SimpleTsPlayer
  const playerContainerRef = useRef<HTMLDivElement | null>(null);

  // Attach to underlying <video> only for mute/volume sync (play state comes via onPlayStateChange)
  useEffect(() => {
    if (!isOpen) return;
    const container = playerContainerRef.current;
    if (!container) return;
    const vid = container.querySelector('video') as HTMLVideoElement | null;
    if (!vid) return;

    videoElRef.current = vid;

    try {
      vid.controls = false;
      vid.removeAttribute('controls');
    } catch {
      /* ignore */
    }

    setIsMuted(vid.muted);
    if (typeof vid.volume === 'number') setVolume(vid.volume);

    const handleVolume = () => {
      setIsMuted(vid.muted);
      setVolume(vid.volume);
    };

    vid.addEventListener('volumechange', handleVolume);
    bumpControls();

    return () => {
      vid.removeEventListener('volumechange', handleVolume);
      clearHideTimer();
    };
  }, [isOpen, bumpControls]);

  // When modal opens/closes reset state
  useEffect(() => {
    if (!isOpen) {
      setLastError(null);
      setIsPlaying(false);
      setIsMuted(false);
      setIsHevc(false);
      setBufferSeconds(null);
      setVolume(1);
    }
  }, [isOpen]);

  // Removed mutation observer + polling: play state now driven by SimpleTsPlayer.onPlayStateChange

  // Removed autoplay polling – simplified event-driven state tracking now handles play state.

  // Debug: log detection context (heuristic vs proxied)
  useEffect(() => {
    if (!channel) return;
    debug.log('Detection context', {
      channelId: channel.id,
      originalStreamUrl: channel.original_stream_url,
      proxiedStreamUrl: streamUrl,
      extension: channel.original_stream_url
        ? channel.original_stream_url.split('?')[0].split('.').pop()
        : undefined,
    });
  }, [channel, streamUrl, debug]);

  /* ---------- Derived Badges ---------- */

  // Simplified badge derivations (no adapter / phase / retries after TS-only refactor)
  const resolution =
    mediaInfo?.width && mediaInfo?.height ? `${mediaInfo.width}x${mediaInfo.height}` : undefined;

  const videoCodec = mediaInfo?.videoCodec;
  const audioCodec = mediaInfo?.audioCodec;

  const bufferClass =
    bufferSeconds == null
      ? 'bg-muted text-muted-foreground'
      : bufferSeconds < 2
        ? 'bg-red-900/50 text-red-300 ring-1 ring-red-500/40'
        : bufferSeconds < 5
          ? 'bg-yellow-900/40 text-yellow-200 ring-1 ring-yellow-500/40'
          : 'bg-green-900/40 text-green-200 ring-1 ring-green-500/40';

  // Phase styling removed (no phase state now)

  /* ---------- Render Helpers ---------- */

  const renderBadges = () => (
    <div className="flex flex-wrap items-center gap-1 mt-2 text-xs">
      {program?.category && (
        <Badge variant="secondary" className="text-[10px]">
          {program.category}
        </Badge>
      )}
      {channel?.source_name && (
        <Badge variant="outline" className="text-[10px]">
          {channel.source_name}
        </Badge>
      )}
      {resolution && (
        <Badge variant="outline" className="text-[10px]">
          {resolution}
        </Badge>
      )}
      {videoCodec && (
        <Badge variant="outline" className="text-[10px]">
          {videoCodec}
        </Badge>
      )}
      {audioCodec && (
        <Badge variant="outline" className="text-[10px]">
          {audioCodec}
        </Badge>
      )}
      {isHevc && (
        <span
          className="px-2 py-0.5 rounded-md text-[10px] leading-none font-semibold bg-purple-900/50 text-purple-200"
          title="Detected HEVC/H.265"
        >
          HEVC
        </span>
      )}
      {/* Decision / transformation badges */}
      {streamDecision &&
        /collapse|collapsed-ts|hls-collapse/.test(streamDecision.toLowerCase()) && (
          <Badge variant="secondary" className="text-[10px]">
            COLLAPSED
          </Badge>
        )}
      {streamDecision && /raw|passthrough-raw/.test(streamDecision.toLowerCase()) && (
        <Badge variant="outline" className="text-[10px]">
          RAW
        </Badge>
      )}
      {/* Always show LIVE for active playback context */}
      <Badge variant="destructive" className="text-[10px]">
        LIVE
      </Badge>
      {lastError && (
        <span
          className="px-2 py-0.5 rounded-md text-[10px] leading-none font-semibold bg-red-900/60 text-red-200 flex items-center gap-1"
          title={lastError}
        >
          <AlertTriangle className="w-3 h-3" />
          err
        </span>
      )}
    </div>
  );

  const renderErrorPanel = () => {
    if (!lastError) return null;
    return (
      <div className="absolute inset-0 flex items-center justify-center bg-black/70 z-20">
        <div className="max-w-md mx-auto p-6 rounded-lg border border-red-700 bg-red-950/60 text-red-200 shadow-lg space-y-4">
          <div className="flex items-center gap-2">
            <AlertTriangle className="w-5 h-5 text-red-400" />
            <h3 className="text-lg font-semibold">Playback Error</h3>
          </div>
          <p className="text-sm leading-relaxed break-words">{lastError}</p>
          {isHevc && (
            <p className="text-xs text-purple-200">
              This appears to be an HEVC stream. Some browsers require additional support or flags.
            </p>
          )}
          <div className="flex gap-2 justify-end flex-wrap">
            <Button
              variant="outline"
              size="sm"
              aria-label={copySuccess ? 'Stream URL copied' : 'Copy stream URL'}
              onClick={handleCopyUrl}
            >
              {copySuccess ? <Check className="w-4 h-4 mr-1" /> : <Copy className="w-4 h-4 mr-1" />}
              {copySuccess ? 'Copied' : 'Copy URL'}
            </Button>
            <Button
              variant="outline"
              size="sm"
              aria-label="Retry playback"
              onClick={() => {
                setLastError(null);
                // reload removed - no action
              }}
            >
              <RefreshCw className="w-4 h-4 mr-1" />
              Retry
            </Button>
            <Button variant="outline" size="sm" aria-label="Close player" onClick={handleClose}>
              <X className="w-4 h-4 mr-1" />
              Close
            </Button>
          </div>
        </div>
      </div>
    );
  };

  /* ---------- Main Render ---------- */

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent
        id="video-player-dialog-content"
        className="max-w-6xl w-[94vw] p-0 bg-black/95 border border-slate-800 overflow-hidden"
        aria-describedby="player-description"
        onMouseMove={handleMouseMove}
        onClick={handleMouseMove}
      >
        {/* Hidden description to satisfy Radix/Dialog accessibility without forcing visible text */}
        <DialogDescription id="player-description" className="sr-only">
          Live video player for {displayTitle}. Use keyboard or on-screen controls to interact.
        </DialogDescription>
        <div className="relative aspect-video bg-black">
          {/* UNIFIED PLAYER OVERLAY */}
          <div
            data-player-overlay
            className={`absolute inset-0 z-10 transition-all duration-300 ${
              showControls ? 'opacity-100 pointer-events-auto' : 'opacity-0 pointer-events-none'
            }`}
          >
            {/* Top section (header + badges) */}
            <div className="absolute top-0 left-0 right-0 bg-gradient-to-b from-black/80 to-transparent px-4 pt-3 pb-4 pointer-events-none">
              <DialogHeader className="p-0 space-y-1">
                <DialogTitle className="text-base md:text-lg font-semibold text-white truncate">
                  {displayTitle}
                </DialogTitle>
                {displaySubtitle && (
                  <DialogDescription className="text-xs md:text-sm text-gray-300">
                    {displaySubtitle}
                  </DialogDescription>
                )}
              </DialogHeader>
              {renderBadges()}
            </div>

            {/* Bottom controls */}
            <div
              data-player-controls
              className="absolute bottom-0 left-0 right-0 bg-gradient-to-t from-black/80 to-transparent px-3 pb-3 pt-14 md:pt-24 flex flex-col justify-end pointer-events-auto"
            >
              <div className="flex items-center gap-2 justify-between">
                <div className="flex items-center gap-2">
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 text-white hover:bg-white/10"
                    aria-label={isPlaying ? 'Pause' : 'Play'}
                    onClick={togglePlay}
                  >
                    {isPlaying ? <Pause className="w-4 h-4" /> : <Play className="w-4 h-4" />}
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 text-white hover:bg-white/10"
                    aria-label={isMuted ? 'Unmute' : 'Mute'}
                    onClick={toggleMute}
                  >
                    {isMuted ? <VolumeX className="w-4 h-4" /> : <Volume2 className="w-4 h-4" />}
                  </Button>
                  <input
                    type="range"
                    min={0}
                    max={1}
                    step={0.05}
                    value={volume}
                    aria-label="Volume"
                    className="h-8 w-24 md:w-32 accent-white"
                    onChange={(e) => {
                      const v = parseFloat(e.target.value);
                      if (Number.isNaN(v)) return;
                      setVolume(v);
                      let vid = videoElRef.current;
                      if (!vid && playerContainerRef.current) {
                        const found = playerContainerRef.current.querySelector(
                          'video'
                        ) as HTMLVideoElement | null;
                        if (found) {
                          videoElRef.current = found;
                          vid = found;
                        }
                      }
                      if (vid) {
                        vid.volume = v;
                        if (v === 0 && !vid.muted) {
                          vid.muted = true;
                          setIsMuted(true);
                        } else if (v > 0 && vid.muted) {
                          vid.muted = false;
                          setIsMuted(false);
                        }
                      }
                    }}
                  />
                  {/* Reload button removed */}
                </div>

                <div className="flex items-center gap-2">
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 text-white hover:bg-white/10"
                    aria-label={copySuccess ? 'Stream URL copied' : 'Copy stream URL'}
                    onClick={handleCopyUrl}
                    title="Copy stream URL"
                  >
                    {copySuccess ? <Check className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 text-white hover:bg-white/10"
                    aria-label="Download playlist (M3U)"
                    onClick={handleExternalPlayer}
                    title="Download playlist for external player"
                  >
                    <ExternalLink className="w-4 h-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 text-white hover:bg-white/10"
                    aria-label={isFullscreen ? 'Exit fullscreen' : 'Enter fullscreen'}
                    onClick={toggleFullscreen}
                    title="Toggle fullscreen"
                  >
                    {isFullscreen ? (
                      <Minimize2 className="w-4 h-4" />
                    ) : (
                      <Maximize2 className="w-4 h-4" />
                    )}
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 text-white hover:bg-white/10"
                    aria-label="Close player"
                    onClick={handleClose}
                    title="Close"
                  >
                    <X className="w-4 h-4" />
                  </Button>
                </div>
              </div>
            </div>
          </div>

          {/* PLAYER */}
          <div className="absolute inset-0 pointer-events-auto" ref={playerContainerRef}>
            {!streamUrl && (
              <div className="flex items-center justify-center h-full text-gray-400 text-sm">
                No stream URL
              </div>
            )}

            {streamUrl && streamKind === null && (
              <div className="flex items-center justify-center h-full text-gray-400 text-xs animate-pulse">
                Probing stream…
              </div>
            )}

            {streamUrl &&
              streamKind === 'HLS_PLAYLIST' &&
              streamDecision !== 'hls_collapse_single_ts' && (
                <div className="flex h-full w-full flex-col items-center justify-center gap-4 text-center text-slate-300 px-6">
                  <div className="space-y-2 max-w-xl">
                    <p className="text-sm font-medium">
                      This channel is an HLS playlist which isn&apos;t yet supported in the embedded
                      web player.
                    </p>
                    <p className="text-xs text-slate-400">
                      You can open it in an external player (VLC, MPV, IINA, etc.). Copy the URL or
                      download a tiny M3U file below.
                    </p>
                    {streamDecision && (
                      <p className="text-[11px] text-slate-500">
                        Decision: <code className="font-mono">{streamDecision}</code>
                      </p>
                    )}
                  </div>
                  <div className="w-full max-w-xl break-all rounded bg-slate-800/60 px-3 py-2 text-[11px] font-mono text-slate-200 border border-slate-700/70">
                    {makeAbsolute(streamUrl)}
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button variant="outline" size="sm" onClick={handleCopyUrl}>
                      {copySuccess ? (
                        <Check className="w-4 h-4 mr-1" />
                      ) : (
                        <Copy className="w-4 h-4 mr-1" />
                      )}
                      {copySuccess ? 'Copied' : 'Copy URL'}
                    </Button>
                    <Button variant="outline" size="sm" onClick={handleExternalPlayer}>
                      <ExternalLink className="w-4 h-4 mr-1" />
                      Download M3U
                    </Button>
                    {/* Re-check button removed (reload functionality deprecated) */}
                  </div>
                </div>
              )}

            {streamUrl &&
              (streamKind === 'RAW_TS' ||
                streamKind === 'UNKNOWN' ||
                (streamKind === 'HLS_PLAYLIST' && streamDecision === 'hls_collapse_single_ts')) && (
                <SimpleTsPlayer
                  key={streamUrl}
                  src={streamUrl}
                  channelName={channel?.name}
                  autoPlay
                  showInternalOverlay={false}
                  suppressInlineError={true}
                  decision={streamDecision}
                  onMediaInfo={handleMediaInfo}
                  onPlayStateChange={(playing) => {
                    setIsPlaying(playing);
                  }}
                  onVideoReady={(video) => {
                    // Prime mute / volume state; play state arrives via onPlayStateChange
                    setIsMuted(video.muted);
                    if (typeof video.volume === 'number') {
                      setVolume(video.volume);
                    }
                  }}
                  onError={(err) => {
                    debug.error('TS player error', err);
                    setLastError(err.message);
                  }}
                />
              )}
          </div>

          {/* ERROR OVERLAY */}
          {renderErrorPanel()}

          {/* HEVC INFO (lightweight) */}
          {isHevc && !lastError && (
            <div
              className={`absolute bottom-3 right-3 z-20 transition-opacity duration-500 ${
                showControls ? 'opacity-100' : 'opacity-0'
              }`}
            >
              <div className="bg-purple-950/60 border border-purple-600/50 text-purple-200 px-3 py-2 rounded-md text-xs flex items-center gap-2 backdrop-blur-sm">
                <Tv className="w-4 h-4" />
                <span>HEVC stream detected</span>
              </div>
            </div>
          )}
        </div>

        {/* DESCRIPTION (optional) */}
        {program?.description && (
          <div className="border-t border-slate-800 bg-slate-900/70">
            <ScrollArea className="max-h-40">
              <div className="p-4 space-y-2">
                <h4 className="text-sm font-semibold text-slate-200">Description</h4>
                <p className="text-xs text-slate-300 leading-relaxed whitespace-pre-wrap">
                  {program.description}
                </p>
              </div>
            </ScrollArea>
          </div>
        )}

        {/* (Replaced by hidden DialogDescription placed near DialogContent opening) */}
      </DialogContent>
    </Dialog>
  );
}

/* ---------- Utilities ---------- */

function makeAbsolute(url: string): string {
  if (!url) return url;
  if (/^https?:\/\//i.test(url)) return url;
  if (typeof window === 'undefined') return url;
  return `${window.location.origin}${url.startsWith('/') ? url : `/${url}`}`;
}
