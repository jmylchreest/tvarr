'use client';

import { memo } from 'react';
import { Handle, Position } from '@xyflow/react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import type { FlowNodeData } from '@/types/relay-flow';
import { formatBytes, formatBps } from '@/types/relay-flow';
import { Monitor, Wifi, Download } from 'lucide-react';

interface ClientNodeProps {
  data: FlowNodeData;
}

function ClientNode({ data }: ClientNodeProps) {
  const getPlayerIcon = () => {
    // Could extend this to show different icons based on player type
    return <Monitor className="h-4 w-4 text-teal-500" />;
  };

  return (
    <>
      {/* Input handle */}
      <Handle
        type="target"
        position={Position.Left}
        className="w-3 h-3 bg-teal-500 border-2 border-background"
      />

      <Card className="w-56 shadow-lg border-2 border-teal-500/30 bg-card">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium flex items-center gap-2">
            {getPlayerIcon()}
            <span className="truncate" title={data.remoteAddr || 'Client'}>
              {data.remoteAddr || 'Client'}
            </span>
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {/* Player type */}
          {data.playerType && (
            <Badge variant="secondary" className="text-xs">
              {data.playerType}
            </Badge>
          )}

          {/* User agent */}
          {data.userAgent && (
            <div
              className="flex items-center gap-1 text-xs text-muted-foreground"
              title={data.userAgent}
            >
              <Wifi className="h-3 w-3" />
              <span className="truncate max-w-[160px]">
                {data.userAgent.length > 25 ? `${data.userAgent.substring(0, 25)}...` : data.userAgent}
              </span>
            </div>
          )}

          {/* Bytes received */}
          {data.bytesRead !== undefined && data.bytesRead > 0 && (
            <div className="flex items-center gap-1 text-xs text-teal-600 dark:text-teal-400">
              <Download className="h-3 w-3" />
              <span>{formatBytes(data.bytesRead)} received</span>
            </div>
          )}

          {/* Egress rate if available */}
          {data.egressBps !== undefined && data.egressBps > 0 && (
            <div className="text-xs text-blue-600 dark:text-blue-400">
              {formatBps(data.egressBps)} egress
            </div>
          )}
        </CardContent>
      </Card>
    </>
  );
}

export default memo(ClientNode);
