'use client';

import { memo } from 'react';
import { Handle, Position } from '@xyflow/react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import type { FlowNodeData, RouteType } from '@/types/relay-flow';
import { formatBps } from '@/types/relay-flow';
import { Cpu, MemoryStick, Settings, AlertTriangle } from 'lucide-react';

interface ProcessorNodeProps {
  data: FlowNodeData;
}

function ProcessorNode({ data }: ProcessorNodeProps) {
  const getRouteTypeColor = (routeType?: RouteType) => {
    switch (routeType) {
      case 'passthrough':
        return 'bg-green-500/20 text-green-700 dark:text-green-400 border-green-500/30';
      case 'repackage':
        return 'bg-yellow-500/20 text-yellow-700 dark:text-yellow-400 border-yellow-500/30';
      case 'transcode':
        return 'bg-purple-500/20 text-purple-700 dark:text-purple-400 border-purple-500/30';
      default:
        return 'bg-muted text-muted-foreground';
    }
  };

  const getBorderColor = () => {
    if (data.error) return 'border-red-500/50';
    if (data.inFallback) return 'border-yellow-500/50';
    return 'border-orange-500/30';
  };

  return (
    <>
      {/* Input handle */}
      <Handle
        type="target"
        position={Position.Left}
        className="w-3 h-3 bg-orange-500 border-2 border-background"
      />

      <Card className={`w-64 shadow-lg border-2 bg-card ${getBorderColor()}`}>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium flex items-center gap-2">
            <Settings className="h-4 w-4 text-orange-500" />
            {data.label || 'Processor'}
            {data.inFallback && (
              <AlertTriangle className="h-4 w-4 text-yellow-500" />
            )}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {/* Route type badge */}
          {data.routeType && (
            <Badge variant="outline" className={`text-xs ${getRouteTypeColor(data.routeType)}`}>
              {data.routeType.toUpperCase()}
            </Badge>
          )}

          {/* Profile name */}
          {data.profileName && (
            <div className="text-xs text-muted-foreground">
              Profile: {data.profileName}
            </div>
          )}

          {/* Output format and codecs */}
          {(data.outputFormat || data.outputVideoCodec || data.outputAudioCodec) && (
            <div className="text-xs text-muted-foreground space-y-0.5">
              {data.outputFormat && (
                <div>Format: {data.outputFormat.toUpperCase()}</div>
              )}
              {(data.outputVideoCodec || data.outputAudioCodec) && (
                <div>
                  Codecs: {[data.outputVideoCodec, data.outputAudioCodec].filter(Boolean).join(' / ')}
                </div>
              )}
            </div>
          )}

          {/* CPU and Memory stats (only for transcode) */}
          {(data.cpuPercent !== undefined || data.memoryMB !== undefined) && (
            <div className="flex gap-3 text-xs">
              {data.cpuPercent !== undefined && (
                <div className="flex items-center gap-1">
                  <Cpu className="h-3 w-3 text-blue-500" />
                  <span>{data.cpuPercent.toFixed(1)}%</span>
                </div>
              )}
              {data.memoryMB !== undefined && (
                <div className="flex items-center gap-1">
                  <MemoryStick className="h-3 w-3 text-green-500" />
                  <span>{data.memoryMB.toFixed(0)} MB</span>
                </div>
              )}
            </div>
          )}

          {/* Processing rate */}
          {data.processingBps !== undefined && data.processingBps > 0 && (
            <div className="text-xs text-blue-600 dark:text-blue-400">
              {formatBps(data.processingBps)} processing
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

      {/* Output handle */}
      <Handle
        type="source"
        position={Position.Right}
        className="w-3 h-3 bg-orange-500 border-2 border-background"
      />
    </>
  );
}

export default memo(ProcessorNode);
