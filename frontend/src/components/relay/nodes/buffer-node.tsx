'use client';

import { memo } from 'react';
import { Handle, Position } from '@xyflow/react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Progress } from '@/components/ui/progress';
import type { FlowNodeData } from '@/types/relay-flow';
import { formatBytes } from '@/types/relay-flow';
import { Database, HardDrive } from 'lucide-react';

interface BufferNodeProps {
  data: FlowNodeData;
}

function BufferNode({ data }: BufferNodeProps) {
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

      <Card className="w-56 shadow-lg border-2 border-purple-500/30 bg-card">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium flex items-center gap-2">
            <Database className="h-4 w-4 text-purple-500" />
            <span className="truncate" title={data.label || 'Buffer'}>
              {data.label || 'Buffer'}
            </span>
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
                {data.maxBufferBytes !== undefined && data.maxBufferBytes > 0 && (
                  <span className="text-muted-foreground">
                    / {formatBytes(data.maxBufferBytes)}
                  </span>
                )}
              </div>
              {data.bufferUtilization !== undefined && (
                <Progress value={data.bufferUtilization} className="h-1.5" />
              )}
            </div>
          )}

          {/* Variant details with size and utilization */}
          {hasVariants && data.bufferVariants!.length > 0 && (
            <div className="text-xs text-muted-foreground space-y-1.5">
              {data.bufferVariants!.map((v) => {
                // Build display name from video/audio codecs (more reliable than variant key)
                const codecDisplay =
                  [v.videoCodec, v.audioCodec].filter(Boolean).join('/') || v.variant;
                return (
                  <div key={v.variant} className="space-y-0.5">
                    <div className="flex justify-between items-center gap-2">
                      <span className={`font-mono ${v.isSource ? 'font-bold' : ''}`}>
                        {codecDisplay}
                      </span>
                      <span>
                        {formatBytes(v.bytesIngested)}
                        {v.maxBytes > 0 && (
                          <span className="text-muted-foreground/60">
                            {' '}
                            / {formatBytes(v.maxBytes)}
                          </span>
                        )}
                      </span>
                    </div>
                    {v.utilization > 0 && <Progress value={v.utilization} className="h-1" />}
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
