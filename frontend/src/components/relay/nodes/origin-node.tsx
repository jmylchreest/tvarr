'use client';

import { memo } from 'react';
import { Handle, Position } from '@xyflow/react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import type { FlowNodeData } from '@/types/relay-flow';
import { formatBps } from '@/types/relay-flow';
import { Globe } from 'lucide-react';

interface OriginNodeProps {
  data: FlowNodeData;
}

function OriginNode({ data }: OriginNodeProps) {
  return (
    <>
      <Card className="w-64 shadow-lg border-2 border-blue-500/30 bg-card">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium flex items-center gap-2">
            <Globe className="h-4 w-4 text-blue-500" />
            <span className="truncate" title={data.label || 'Origin'}>
              {data.label || 'Origin'}
            </span>
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
          <div
            className="text-xs text-muted-foreground truncate"
            title={data.sourceUrl}
          >
            {data.sourceUrl}
          </div>

          {/* Format and codecs - matching processor node style */}
          {(data.sourceFormat || data.videoCodec || data.audioCodec) && (
            <div className="text-xs text-muted-foreground space-y-0.5">
              {data.sourceFormat && (
                <div>Format: {data.sourceFormat.toUpperCase()}</div>
              )}
              {(data.videoCodec || data.audioCodec) && (
                <div>
                  Codecs: {[data.videoCodec, data.audioCodec].filter(Boolean).join(' / ')}
                </div>
              )}
            </div>
          )}

          {/* Ingress rate */}
          {data.ingressBps !== undefined && data.ingressBps > 0 && (
            <div className="text-xs text-green-600 dark:text-green-400">
              {formatBps(data.ingressBps)} ingress
            </div>
          )}
        </CardContent>
      </Card>

      {/* Output handle */}
      <Handle
        type="source"
        position={Position.Right}
        className="w-3 h-3 bg-blue-500 border-2 border-background"
      />
    </>
  );
}

export default memo(OriginNode);
