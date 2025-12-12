'use client';

import { memo } from 'react';
import { Handle, Position } from '@xyflow/react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import type { FlowNodeData } from '@/types/relay-flow';
import { formatBytes } from '@/types/relay-flow';
import { Cog, ArrowRight, Zap } from 'lucide-react';
import { ResourceSparkline } from './resource-sparkline';
import { SpeedDial } from './speed-dial';

interface TranscoderNodeProps {
  data: FlowNodeData;
}

function TranscoderNode({ data }: TranscoderNodeProps) {
  const hasStats = data.transcoderCpu !== undefined || data.transcoderMemMb !== undefined;
  const hasHistory =
    (data.transcoderCpuHistory && data.transcoderCpuHistory.length > 0) ||
    (data.transcoderMemHistory && data.transcoderMemHistory.length > 0);

  // Determine hardware acceleration display
  const getHWAccelInfo = () => {
    if (!data.hwAccelType || data.hwAccelType === 'none' || data.hwAccelType === 'auto') {
      // Check if encoder name suggests hardware acceleration
      const encoder = data.videoEncoder?.toLowerCase() || '';
      if (encoder.includes('nvenc')) return { type: 'NVENC', color: 'bg-green-600' };
      if (encoder.includes('qsv')) return { type: 'QSV', color: 'bg-blue-600' };
      if (encoder.includes('vaapi')) return { type: 'VAAPI', color: 'bg-purple-600' };
      if (encoder.includes('videotoolbox')) return { type: 'VT', color: 'bg-gray-600' };
      if (encoder.includes('amf')) return { type: 'AMF', color: 'bg-red-600' };
      return null; // Software encoding
    }

    const type = data.hwAccelType.toUpperCase();
    const colorMap: Record<string, string> = {
      CUDA: 'bg-green-600',
      NVENC: 'bg-green-600',
      QSV: 'bg-blue-600',
      VAAPI: 'bg-purple-600',
      VIDEOTOOLBOX: 'bg-gray-600',
      AMF: 'bg-red-600',
    };
    return { type, color: colorMap[type] || 'bg-yellow-600' };
  };

  const hwAccel = getHWAccelInfo();

  // Build input codec string
  const inputCodecs = [data.sourceVideoCodec, data.sourceAudioCodec].filter(Boolean).join('/');
  // Build output codec string
  const outputCodecs = [data.targetVideoCodec, data.targetAudioCodec].filter(Boolean).join('/');

  // Check if there's an actual transformation (different codecs)
  const hasTransformation = inputCodecs && outputCodecs && inputCodecs !== outputCodecs;

  return (
    <>
      {/* Input handle from buffer (bottom right) - receives source data from buffer */}
      <Handle
        type="target"
        position={Position.Bottom}
        id="ffmpeg-buffer-in"
        className="w-3 h-3 bg-red-500 border-2 border-background"
        style={{ left: '75%' }}
      />

      <Card className="w-56 shadow-lg border-2 border-red-500/30 bg-card">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium flex items-center gap-2">
            <Cog className="h-4 w-4 text-red-500 animate-[spin_3s_linear_infinite]" />
            <span className="truncate" title={data.label || 'FFmpeg'}>
              {data.label || 'FFmpeg'}
            </span>
            {/* Hardware acceleration badge */}
            {hwAccel && (
              <Badge className={`text-[10px] px-1.5 py-0 ${hwAccel.color} text-white`}>
                <Zap className="h-2.5 w-2.5 mr-0.5" />
                {hwAccel.type}
              </Badge>
            )}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {/* Codec transformation - single line with arrow */}
          {hasTransformation ? (
            <div className="flex items-center gap-1.5 text-xs">
              <span className="font-mono text-muted-foreground">{inputCodecs}</span>
              <ArrowRight className="h-3 w-3 text-red-500 shrink-0" />
              <span className="font-mono text-foreground">{outputCodecs}</span>
            </div>
          ) : outputCodecs ? (
            <div className="text-xs">
              <span className="font-mono text-foreground">{outputCodecs}</span>
            </div>
          ) : null}

          {/* CPU and Memory sparklines */}
          {(hasStats || hasHistory) && (
            <ResourceSparkline
              cpuHistory={data.transcoderCpuHistory}
              memoryHistory={data.transcoderMemHistory}
              currentCpu={data.transcoderCpu}
              currentMemoryMb={data.transcoderMemMb}
            />
          )}

          {/* Encoding speed dial */}
          {data.encodingSpeed !== undefined && (
            <SpeedDial speed={data.encodingSpeed} />
          )}

          {/* Bytes processed */}
          {data.transcoderBytesIn !== undefined && data.transcoderBytesIn > 0 && (
            <div className="text-xs text-red-600 dark:text-red-400">
              {formatBytes(data.transcoderBytesIn)} processed
            </div>
          )}
        </CardContent>
      </Card>

      {/* Output handle back to buffer (bottom left) - sends transcoded data to buffer */}
      <Handle
        type="source"
        position={Position.Bottom}
        id="ffmpeg-buffer-out"
        className="w-3 h-3 bg-red-500 border-2 border-background"
        style={{ left: '25%' }}
      />
    </>
  );
}

export default memo(TranscoderNode);
