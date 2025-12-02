'use client';

import React, { useMemo } from 'react';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from '@/components/ui/sheet';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { Badge } from '@/components/ui/badge';
import { Copy, Check, Info } from 'lucide-react';
import { cn } from '@/lib/utils';

type Primitive = string | number | boolean | null | undefined;

export interface ChannelLike {
  id?: string;
  name?: string;
  logo_url?: string;
  group?: string;
  stream_url?: string;
  proxy_id?: string;
  source_type?: string;
  source_name?: string;
  tvg_id?: string;
  tvg_name?: string;
  tvg_chno?: string;
  tvg_shift?: string;
  video_codec?: string;
  audio_codec?: string;
  resolution?: string;
  last_probed_at?: string;
  probe_method?: string;
  container_format?: string;
  video_width?: number;
  video_height?: number;
  framerate?: string;
  bitrate?: number | null;
  video_bitrate?: number | null;
  audio_bitrate?: number | null;
  audio_channels?: number | null;
  audio_sample_rate?: number | null;
  probe_source?: string;
  // Allow arbitrary extra fields
  [key: string]: any;
}

interface ChannelDetailsSheetProps {
  channel: ChannelLike | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  additionalFields?: Record<string, Primitive>;
  className?: string;
}

/* ---------------- Formatting Helpers ---------------- */

function humanizeKey(key: string): string {
  if (!key) return key;
  const explicit: Record<string, string> = {
    id: 'ID',
    tvg_id: 'TVG ID',
    tvg_name: 'TVG Name',
    tvg_chno: 'TVG Channel #',
    tvg_shift: 'TVG Shift',
    stream_url: 'Stream URL',
    original_stream_url: 'Original Stream URL',
    proxy_id: 'Proxy ID',
    video_codec: 'Video Codec',
    audio_codec: 'Audio Codec',
    container_format: 'Container',
    video_width: 'Video Width',
    video_height: 'Video Height',
    framerate: 'Framerate',
    bitrate: 'Bitrate (Total)',
    video_bitrate: 'Bitrate (Video)',
    audio_bitrate: 'Bitrate (Audio)',
    audio_channels: 'Audio Channels',
    audio_sample_rate: 'Audio Sample Rate',
    last_probed_at: 'Last Probed',
    probe_method: 'Probe Method',
    probe_source: 'Probe Source',
    source_type: 'Source Type',
    source_name: 'Source Name',
    logo_url: 'Logo URL',
  };
  if (explicit[key]) return explicit[key];
  return key
    .replace(/([a-z0-9])([A-Z])/g, '$1_$2')
    .split(/[_\s]+/)
    .filter(Boolean)
    .map((p) => p.charAt(0).toUpperCase() + p.slice(1).toLowerCase())
    .join(' ');
}

function formatValue(key: string, value: any): string {
  if (value === null || value === undefined || value === '') {
    return key === 'name' ? 'empty' : '';
  }
  if (typeof value === 'object') {
    try {
      return JSON.stringify(value);
    } catch {
      return String(value);
    }
  }
  if (key === 'audio_sample_rate' && typeof value === 'number') {
    return `${value} Hz`;
  }
  if (
    (key === 'bitrate' || key === 'video_bitrate' || key === 'audio_bitrate') &&
    typeof value === 'number'
  ) {
    return `${Math.round(value / 1000)} kbps`;
  }
  return String(value);
}

/* ---------------- Row Component ---------------- */

interface FieldRowProps {
  index: number;
  label: string;
  rawKey: string;
  value: string;
  muted?: boolean;
  onCopy: (text: string) => void;
  copied: boolean;
}

const FieldRow: React.FC<FieldRowProps> = ({
  index,
  label,
  rawKey,
  value,
  muted,
  onCopy,
  copied,
}) => {
  const isEmptyName = rawKey === 'name' && value === 'empty';
  const display = value === '' ? (rawKey === 'name' ? 'empty' : '—') : value;

  return (
    <div
      className={cn(
        'group grid grid-cols-[200px_minmax(0,1fr)] items-stretch text-xs md:text-[13px] leading-relaxed',
        'border-b border-border/60 last:border-b-0 relative',
        index % 2 === 0 ? 'bg-muted/40' : 'bg-background'
      )}
    >
      <div
        className={cn(
          'px-3 py-1.5 font-mono text-[11px] md:text-[11px] tracking-tight text-muted-foreground/90 truncate',
          'flex items-center'
        )}
        title={rawKey}
      >
        {rawKey}
      </div>
      <div
        className={cn(
          'px-3 pr-10 py-1.5 font-medium break-all whitespace-pre-wrap min-h-[36px] flex items-center relative',
          (muted || isEmptyName || value === '') && 'text-muted-foreground font-normal italic'
        )}
        title={display}
      >
        {display}
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              aria-label={`Copy ${label}`}
              onClick={() => onCopy(value === '' ? '' : value)}
              className={cn(
                'absolute inset-y-0 right-0 flex items-center justify-center w-8 text-muted-foreground transition',
                'opacity-0 group-hover:opacity-100 focus:opacity-100 hover:text-primary',
                'focus-visible:ring-2 focus-visible:ring-ring outline-none'
              )}
            >
              {copied ? (
                <Check className="h-4 w-4 text-green-600 dark:text-green-400" />
              ) : (
                <Copy className="h-4 w-4" />
              )}
            </button>
          </TooltipTrigger>
          <TooltipContent side="left" className="text-xs">
            {copied ? 'Copied!' : 'Copy value'}
          </TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
};

/* ---------------- Component ---------------- */

export const ChannelDetailsSheet: React.FC<ChannelDetailsSheetProps> = ({
  channel,
  open,
  onOpenChange,
  additionalFields,
  className,
}) => {
  const rows = useMemo(() => {
    if (!channel) return [];
    const orderedKeys: string[] = [
      // Core identifiers & metadata (top priority)
      'id',
      'name',
      'stream_url',
      'original_stream_url',
      'source_name',
      'source_type',
      'group',
      'tvg_id',
      'tvg_name',
      'tvg_chno',
      'tvg_shift',
      'probe_source',
      'proxy_id',
      'last_probed_at',
      'probe_method',
      // Video-related
      'video_codec',
      'resolution',
      'container_format',
      'video_width',
      'video_height',
      'framerate',
      'bitrate',
      'video_bitrate',
      // Audio-related
      'audio_codec',
      'audio_bitrate',
      'audio_channels',
      'audio_sample_rate',
      // Misc / remaining
      'logo_url',
    ];

    const seen = new Set(orderedKeys);

    const baseEntries = orderedKeys.map((k) => {
      const raw = (channel as any)[k];
      const value = formatValue(k, raw);
      const muted = value === '' || (k === 'name' && value === 'empty');
      return {
        rawKey: k,
        label: humanizeKey(k),
        value,
        muted,
      };
    });

    const extraFromProp =
      additionalFields &&
      Object.entries(additionalFields)
        .filter(([k]) => !seen.has(k))
        .map(([k, v]) => ({
          rawKey: k,
          label: humanizeKey(k),
          value: formatValue(k, v),
          muted: v === null || v === undefined || v === '',
        }));

    const extraFromChannel = Object.entries(channel)
      .filter(([k]) => !seen.has(k) && k !== 'logo_url')
      .map(([k, v]) => ({
        rawKey: k,
        label: humanizeKey(k),
        value: formatValue(k, v),
        muted: v === null || v === undefined || v === '',
      }));

    return [
      ...baseEntries,
      ...(extraFromProp ?? []),
      ...extraFromChannel.filter((e) => !extraFromProp?.some((p) => p.rawKey === e.rawKey)),
    ];
  }, [channel, additionalFields]);

  const [copiedMap, setCopiedMap] = React.useState<Record<string, number>>({});
  React.useEffect(() => {
    if (!open) setCopiedMap({});
  }, [open]);

  const handleCopy = (key: string, text: string) => {
    navigator.clipboard
      .writeText(text)
      .then(() => {
        setCopiedMap((prev) => ({ ...prev, [key]: Date.now() }));
        setTimeout(() => {
          setCopiedMap((prev) => {
            const next = { ...prev };
            if (next[key] && Date.now() - next[key] >= 1800) {
              delete next[key];
            }
            return next;
          });
        }, 2000);
      })
      .catch(() => {
        /* ignore */
      });
  };

  const title = channel?.name && channel.name.trim() !== '' ? channel.name : 'Channel Details';

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className={cn(
          // Wider sheet for lengthy values; responsive max width
          'sm:max-w-4xl w-[97%] sm:w-[960px] p-0 flex flex-col min-h-0',
          className
        )}
      >
        <SheetHeader className="border-b bg-muted/40 px-6 py-4">
          <SheetTitle className="flex items-center gap-2 text-base">
            {channel?.name && channel.name.trim() !== '' ? (
              <span className="truncate">{channel.name}</span>
            ) : (
              <span className="text-muted-foreground italic">empty</span>
            )}
            {channel?.video_codec && (
              <Badge variant="outline" className="text-[10px] font-normal">
                {channel.video_codec}
              </Badge>
            )}
            {channel?.audio_codec && (
              <Badge variant="outline" className="text-[10px] font-normal">
                {channel.audio_codec}
              </Badge>
            )}
          </SheetTitle>
          {/* Removed descriptive subtitle to conserve vertical space */}
        </SheetHeader>

        {channel?.logo_url && (
          <div className="border-b px-5 py-3 flex items-center justify-center bg-background">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src={channel.logo_url}
              alt={title}
              className="max-h-28 object-contain"
              onError={(e) => {
                const img = e.currentTarget;
                img.style.display = 'none';
              }}
            />
          </div>
        )}

        <TooltipProvider delayDuration={150}>
          <ScrollArea className="flex-1 min-h-0">
            {/* Header row for clarity */}
            <div className="sticky top-0 z-10 grid grid-cols-[200px_minmax(0,1fr)] text-[11px] md:text-xs uppercase tracking-wide bg-background/95 backdrop-blur border-b border-border/70">
              <div className="px-3 py-2 font-semibold text-muted-foreground">Key</div>
              <div className="px-3 py-2 font-semibold text-muted-foreground">Value</div>
            </div>
            <div>
              {rows.map((r, i) => (
                <FieldRow
                  key={r.rawKey}
                  index={i}
                  label={r.label}
                  rawKey={r.rawKey}
                  value={r.value}
                  muted={r.muted}
                  copied={!!copiedMap[r.rawKey]}
                  onCopy={(text) => handleCopy(r.rawKey, text)}
                />
              ))}
            </div>
          </ScrollArea>
        </TooltipProvider>
      </SheetContent>
    </Sheet>
  );
};

export default ChannelDetailsSheet;
