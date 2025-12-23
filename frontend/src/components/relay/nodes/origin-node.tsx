'use client';

import { memo } from 'react';
import { Handle, Position } from '@xyflow/react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import type { FlowNodeData } from '@/types/relay-flow';
import { formatBps, formatBytes } from '@/types/relay-flow';
import { Globe, Clock, Download, Film, Monitor, CloudOff, Check } from 'lucide-react';
import { BandwidthSparkline } from './bandwidth-sparkline';

interface OriginNodeProps {
  data: FlowNodeData;
}

function OriginNode({ data }: OriginNodeProps) {
  // Determine connection state - default to connected if not specified
  const isConnected = data.originConnected !== false;

  return (
    <>
      <Card
        className={`w-64 shadow-lg border-2 bg-card ${
          isConnected
            ? 'border-blue-500/30'
            : 'border-amber-500/50 opacity-80'
        }`}
      >
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium flex items-center gap-2">
            {isConnected ? (
              <Globe className="h-4 w-4 text-blue-500" />
            ) : (
              <CloudOff className="h-4 w-4 text-amber-500" />
            )}
            <span className="truncate flex-1" title={data.label || 'Origin'}>
              {data.label || 'Origin'}
            </span>
            {!isConnected && (
              <Badge variant="secondary" className="text-xs bg-amber-500/20 text-amber-600 dark:text-amber-400 border-0">
                <Check className="h-3 w-3 mr-1" />
                EOF
              </Badge>
            )}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {/* Channel name */}
          {data.channelName && (
            <div className="text-sm font-medium truncate" title={data.channelName}>
              {data.channelName}
            </div>
          )}

          {/* Source URL */}
          <div className="text-xs text-muted-foreground truncate" title={data.sourceUrl}>
            {data.sourceUrl}
          </div>

          {/* Format and codecs */}
          <div className="text-xs text-muted-foreground space-y-0.5">
            {data.sourceFormat && <div>Container: {data.sourceFormat.toUpperCase()}</div>}
            {(data.videoCodec || data.audioCodec) && (
              <div>Codecs: {[data.videoCodec, data.audioCodec].filter(Boolean).join(' / ')}</div>
            )}
            {data.videoWidth !== undefined &&
              data.videoWidth > 0 &&
              data.videoHeight !== undefined &&
              data.videoHeight > 0 && (
                <div className="flex items-center gap-1">
                  <Monitor className="h-3 w-3" />
                  <span>
                    {data.videoWidth}x{data.videoHeight}
                  </span>
                </div>
              )}
            {data.framerate !== undefined && data.framerate > 0 && (
              <div className="flex items-center gap-1">
                <Film className="h-3 w-3" />
                <span>{data.framerate.toFixed(2)} fps</span>
              </div>
            )}
          </div>

          {/* Bandwidth sparkline with ingress rate */}
          <BandwidthSparkline
            history={data.ingressHistory}
            currentBps={data.ingressBps}
            label="ingress"
            color="green"
          />

          {/* Total bytes received */}
          {data.totalBytesIn !== undefined && data.totalBytesIn > 0 && (
            <div className="flex items-center gap-1 text-xs text-muted-foreground">
              <Download className="h-3 w-3" />
              <span>{formatBytes(data.totalBytesIn)} received</span>
            </div>
          )}

          {/* Session duration */}
          {data.durationSecs !== undefined && data.durationSecs > 0 && (
            <div className="flex items-center gap-1 text-xs text-muted-foreground">
              <Clock className="h-3 w-3" />
              <span>{formatDuration(data.durationSecs)}</span>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Output handle to buffer */}
      <Handle
        type="source"
        position={Position.Right}
        id="origin-buffer-out"
        className="w-3 h-3 bg-blue-500 border-2 border-background"
      />
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

export default memo(OriginNode);
