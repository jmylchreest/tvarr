'use client';

import { memo } from 'react';
import { Handle, Position } from '@xyflow/react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Progress } from '@/components/ui/progress';
import type { FlowNodeData, BufferVariantInfo } from '@/types/relay-flow';
import { formatBytes } from '@/types/relay-flow';
import { Database, HardDrive, DatabaseZap } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';

// Format duration in seconds to a human-readable string (e.g., "1m 30s" or "45s")
function formatDuration(seconds: number): string {
  if (seconds < 1) return '<1s';
  if (seconds < 60) return `${Math.round(seconds)}s`;
  const mins = Math.floor(seconds / 60);
  const secs = Math.round(seconds % 60);
  if (secs === 0) return `${mins}m`;
  return `${mins}m ${secs}s`;
}

// Get the color class for the progress bar based on utilization
function getProgressColorClass(variant: BufferVariantInfo): string {
  // Evicting is normal/expected at steady state - only warn if utilization is very high
  // which could indicate the buffer can't keep up with incoming data
  if (variant.utilization > 98) {
    // Red when buffer is critically full
    return '[&>div]:bg-red-500';
  }
  if (variant.utilization > 95) {
    // Yellow when approaching critical capacity
    return '[&>div]:bg-yellow-500';
  }
  // Default purple for normal operation (including normal eviction)
  return '[&>div]:bg-purple-500';
}

interface BufferNodeProps {
  data: FlowNodeData;
}

function BufferNode({ data }: BufferNodeProps) {
  // Determine if origin is still connected (streaming) - default to true if not specified
  const isOriginConnected = data.originConnected !== false;

  const hasVariants = data.bufferVariants && data.bufferVariants.length > 0;

  return (
    <>
      {/* Input handle from origin (left side) */}
      <Handle
        type="target"
        position={Position.Left}
        id="buffer-origin-in"
        className="w-3 h-3 bg-purple-500 border-2 border-background"
      />

      {/* Top handles for transcoder connection */}
      {/* Input FROM ffmpeg (top left) - receives transcoded data back from ffmpeg */}
      <Handle
        type="target"
        position={Position.Top}
        id="buffer-ffmpeg-in"
        className="w-3 h-3 bg-red-500 border-2 border-background"
        style={{ left: '25%' }}
      />
      {/* Output TO ffmpeg (top right) - sends source data to ffmpeg for transcoding */}
      <Handle
        type="source"
        position={Position.Top}
        id="buffer-ffmpeg-out"
        className="w-3 h-3 bg-red-500 border-2 border-background"
        style={{ left: '75%' }}
      />

      <Card className={`w-72 shadow-lg border-2 bg-card ${
        isOriginConnected ? 'border-purple-500/30' : 'border-gray-500/30 opacity-75'
      }`}>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium flex items-center gap-2">
            {isOriginConnected ? (
              <Database className="h-4 w-4 text-purple-500" />
            ) : (
              <DatabaseZap className="h-4 w-4 text-gray-500" />
            )}
            <span className="truncate" title={data.label || 'Buffer'}>
              {data.label || 'Buffer'}
            </span>
            {!isOriginConnected && (
              <Badge variant="secondary" className="text-[10px] px-1.5 py-0 bg-gray-500/20 text-gray-600 dark:text-gray-400">
                Draining
              </Badge>
            )}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {/* Total memory usage with utilization bar */}
          {data.bufferMemoryBytes !== undefined && data.bufferMemoryBytes > 0 && (
            <div className="space-y-1">
              <div className="flex items-center justify-between gap-1 text-xs">
                <div className="flex items-center gap-1 text-purple-600 dark:text-purple-400">
                  <HardDrive className="h-3 w-3" />
                  <span>{formatBytes(data.bufferMemoryBytes)}</span>
                </div>
                {data.maxBufferBytes !== undefined && (
                  <span className="text-muted-foreground">
                    / {data.maxBufferBytes > 0 ? formatBytes(data.maxBufferBytes) : '∞'}
                  </span>
                )}
              </div>
              {data.bufferUtilization !== undefined && data.maxBufferBytes !== undefined && data.maxBufferBytes > 0 && (
                <Progress value={data.bufferUtilization} className="h-1.5" />
              )}
            </div>
          )}

          {/* Variant details with size and utilization */}
          {hasVariants && data.bufferVariants!.length > 0 && (
            <div className="text-xs text-muted-foreground space-y-2">
              {[...data.bufferVariants!]
                .sort((a, b) => {
                  // Source variant always first
                  if (a.isSource && !b.isSource) return -1;
                  if (!a.isSource && b.isSource) return 1;
                  // Then sort alphabetically by variant name (codec combo)
                  return a.variant.localeCompare(b.variant);
                })
                .map((v) => {
                // Build display name from video/audio codecs (more reliable than variant key)
                const codecDisplay =
                  [v.videoCodec, v.audioCodec].filter(Boolean).join('/') || v.variant;
                const progressColorClass = getProgressColorClass(v);
                return (
                  <div key={v.variant} className="space-y-0.5">
                    <div className="flex justify-between items-center gap-1">
                      <span className="flex items-center gap-1.5">
                        {/* Source/Transcoded badge */}
                        <span
                          className={cn(
                            'text-[10px] px-1.5 py-0.5 rounded font-medium uppercase tracking-wide',
                            v.isSource
                              ? 'bg-blue-500/20 text-blue-400 border border-blue-500/30'
                              : 'bg-red-500/20 text-red-400 border border-red-500/30'
                          )}
                        >
                          {v.isSource ? 'src' : 'enc'}
                        </span>
                        {/* Codec display */}
                        <span className="font-mono">{codecDisplay}</span>
                      </span>
                      {/* Duration and size info */}
                      <span className="flex items-center gap-1 text-muted-foreground/80 whitespace-nowrap">
                        {v.bufferDuration > 0 && (
                          <span title={`Buffer contains ~${formatDuration(v.bufferDuration)} of content`}>
                            ~{formatDuration(v.bufferDuration)}
                          </span>
                        )}
                        <span className="text-muted-foreground/40">|</span>
                        <span>
                          {formatBytes(v.bytesIngested)}
                          <span className="text-muted-foreground/50">
                            /{v.maxBytes > 0 ? formatBytes(v.maxBytes) : '∞'}
                          </span>
                        </span>
                      </span>
                    </div>
                    {v.maxBytes > 0 && v.utilization > 0 && (
                      <Progress value={v.utilization} className={cn('h-1', progressColorClass)} />
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Output handle to processors (right side) */}
      <Handle
        type="source"
        position={Position.Right}
        id="buffer-processor-out"
        className="w-3 h-3 bg-purple-500 border-2 border-background"
      />
    </>
  );
}

export default memo(BufferNode);
