'use client';

import { memo } from 'react';
import { Handle, Position } from '@xyflow/react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import type { FlowNodeData } from '@/types/relay-flow';
import { formatBytes } from '@/types/relay-flow';
import { Monitor, Download, Clock } from 'lucide-react';
import { BandwidthSparkline } from './bandwidth-sparkline';

interface ClientNodeProps {
  data: FlowNodeData;
}

/**
 * Parse a user agent string to extract the most significant part.
 * Common formats:
 * - "Mozilla/5.0 ... Chrome/120.0.0.0 Safari/537.36" -> "Chrome/120"
 * - "Lavf/60.16.100" -> "Lavf/60.16"
 * - "mpv 0.37.0" -> "mpv/0.37"
 * - "VLC/3.0.20 LibVLC/3.0.20" -> "VLC/3.0"
 * - "ExoPlayer/2.19.1" -> "ExoPlayer/2.19"
 */
function parseUserAgent(userAgent: string | undefined): string {
  if (!userAgent) return 'Unknown';

  // Try to match common patterns
  const patterns = [
    // mpv style: "mpv 0.37.0" or "mpv/0.37.0"
    /\b(mpv)[\/\s](\d+\.\d+)/i,
    // Lavf (libavformat, used by ffmpeg/mpv): "Lavf/60.16.100"
    /\b(Lavf)[\/](\d+\.\d+)/i,
    // VLC: "VLC/3.0.20"
    /\b(VLC)[\/](\d+\.\d+)/i,
    // ExoPlayer: "ExoPlayer/2.19.1"
    /\b(ExoPlayer)[\/](\d+\.\d+)/i,
    // hls.js: "hls.js/1.4.12"
    /\b(hls\.js)[\/](\d+\.\d+)/i,
    // mpegts.js
    /\b(mpegts\.js)[\/](\d+\.\d+)/i,
    // FFmpeg
    /\b(FFmpeg)[\/\s](\d+\.\d+)/i,
    // Chrome: "Chrome/120.0.0.0"
    /\b(Chrome)[\/](\d+)/i,
    // Firefox: "Firefox/121.0"
    /\b(Firefox)[\/](\d+)/i,
    // Safari (but not Chrome's Safari token): need to check it's actually Safari
    /\b(Safari)[\/](\d+)/i,
    // Edge: "Edg/120.0.0.0"
    /\b(Edg)[\/](\d+)/i,
  ];

  for (const pattern of patterns) {
    const match = userAgent.match(pattern);
    if (match) {
      const name = match[1];
      const version = match[2];
      // Normalize some names
      const normalizedName = name === 'Edg' ? 'Edge' : name === 'Lavf' ? 'libav' : name;
      return `${normalizedName}/${version}`;
    }
  }

  // If no pattern matched, try to get the first product/version token
  const productMatch = userAgent.match(/^([^\s\/]+)(?:[\/\s](\d+(?:\.\d+)?))?/);
  if (productMatch) {
    const name = productMatch[1];
    const version = productMatch[2];
    if (version) {
      return `${name}/${version}`;
    }
    return name.slice(0, 20); // Truncate if too long
  }

  // Last resort: truncate the user agent
  return userAgent.slice(0, 20) + (userAgent.length > 20 ? '…' : '');
}

function ClientNode({ data }: ClientNodeProps) {
  const getPlayerIcon = () => {
    return <Monitor className="h-4 w-4 text-teal-500" />;
  };

  const parsedUA = parseUserAgent(data.userAgent);

  // Build tooltip content with all available client info
  const tooltipLines: string[] = [];
  if (data.userAgent) {
    tooltipLines.push(`User-Agent: ${data.userAgent}`);
  }
  if (data.remoteAddr) {
    tooltipLines.push(`Address: ${data.remoteAddr}`);
  }
  if (data.clientId) {
    tooltipLines.push(`Client ID: ${data.clientId}`);
  }
  if (data.detectionRule) {
    tooltipLines.push(`Detection Rule: ${data.detectionRule}`);
  }
  if (data.clientFormat) {
    tooltipLines.push(`Format: ${data.clientFormat.toUpperCase()}`);
  }

  return (
    <>
      {/* Input handle from processor */}
      <Handle
        type="target"
        position={Position.Left}
        id="client-processor-in"
        className="w-3 h-3 bg-teal-500 border-2 border-background"
      />

      <Card className="w-48 shadow-lg border-2 border-teal-500/30 bg-card">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium flex items-center gap-2">
            {getPlayerIcon()}
            <span className="truncate font-mono" title={data.remoteAddr || 'Client'}>
              {data.remoteAddr || 'Client'}
            </span>
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-1.5">
          {/* User agent (parsed) with tooltip for full details */}
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <div className="flex flex-wrap gap-1 cursor-help">
                  <Badge variant="secondary" className="text-xs">
                    {parsedUA}
                  </Badge>
                  {data.clientFormat && (
                    <Badge variant="outline" className="text-xs">
                      {data.clientFormat.toUpperCase()}
                    </Badge>
                  )}
                </div>
              </TooltipTrigger>
              <TooltipContent side="bottom" className="max-w-xs">
                <div className="text-xs space-y-1 font-mono">
                  {tooltipLines.map((line, i) => (
                    <div key={i} className="break-all">
                      {line}
                    </div>
                  ))}
                </div>
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>

          {/* Bytes received - prominent */}
          {data.bytesRead !== undefined && data.bytesRead > 0 && (
            <div className="flex items-center gap-1 text-xs font-medium text-teal-600 dark:text-teal-400">
              <Download className="h-3 w-3" />
              <span>{formatBytes(data.bytesRead)}</span>
            </div>
          )}

          {/* Egress rate with sparkline */}
          <BandwidthSparkline
            history={data.clientEgressHistory}
            currentBps={data.egressBps}
            color="teal"
          />

          {/* Connection duration */}
          {data.connectedSecs !== undefined && data.connectedSecs > 0 && (
            <div className="flex items-center gap-1 text-xs text-muted-foreground">
              <Clock className="h-3 w-3" />
              <span>{formatDuration(data.connectedSecs)}</span>
            </div>
          )}
        </CardContent>
      </Card>
    </>
  );
}

// Helper to format duration in human-readable format
function formatDuration(seconds: number): string {
  if (seconds < 60) {
    return `${Math.floor(seconds)}s`;
  }
  if (seconds < 3600) {
    const mins = Math.floor(seconds / 60);
    const secs = Math.floor(seconds % 60);
    return `${mins}m ${secs}s`;
  }
  const hours = Math.floor(seconds / 3600);
  const mins = Math.floor((seconds % 3600) / 60);
  return `${hours}h ${mins}m`;
}

export default memo(ClientNode);
