'use client';

import { memo } from 'react';
import { Handle, Position } from '@xyflow/react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import type { FlowNodeData } from '@/types/relay-flow';
import { formatBytes } from '@/types/relay-flow';
import { Radio, AlertTriangle, Upload } from 'lucide-react';
import { BandwidthSparkline } from './bandwidth-sparkline';

interface ProcessorNodeProps {
  data: FlowNodeData;
}

function ProcessorNode({ data }: ProcessorNodeProps) {
  const getBorderColor = () => {
    if (data.error) return 'border-red-500/50';
    if (data.inFallback) return 'border-yellow-500/50';
    return 'border-orange-500/30';
  };

  return (
    <>
      {/* Input handle from buffer */}
      <Handle
        type="target"
        position={Position.Left}
        id="processor-buffer-in"
        className="w-3 h-3 bg-orange-500 border-2 border-background"
      />

      <Card className={`w-64 shadow-lg border-2 bg-card ${getBorderColor()}`}>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium flex items-center gap-2">
            <Radio className="h-4 w-4 text-orange-500" />
            <span className="truncate">{data.label || 'Output'}</span>
            {data.inFallback && <AlertTriangle className="h-4 w-4 text-yellow-500" />}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {/* Codecs */}
          {(data.outputVideoCodec || data.outputAudioCodec) && (
            <div className="text-xs text-muted-foreground">
              Codecs: {[data.outputVideoCodec, data.outputAudioCodec].filter(Boolean).join(' / ')}
            </div>
          )}

          {/* Processing rate with sparkline */}
          <BandwidthSparkline
            history={data.egressHistory}
            currentBps={data.processingBps}
            label="processing"
            color="blue"
          />

          {/* Total bytes output */}
          {data.totalBytesOut !== undefined && data.totalBytesOut > 0 && (
            <div className="flex items-center gap-1 text-xs text-muted-foreground">
              <Upload className="h-3 w-3" />
              <span>{formatBytes(data.totalBytesOut)} sent</span>
            </div>
          )}

          {/* Error message */}
          {data.error && (
            <div className="text-xs text-red-500 truncate" title={data.error}>
              {data.error}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Output handle to clients */}
      <Handle
        type="source"
        position={Position.Right}
        id="processor-client-out"
        className="w-3 h-3 bg-orange-500 border-2 border-background"
      />
    </>
  );
}

export default memo(ProcessorNode);
