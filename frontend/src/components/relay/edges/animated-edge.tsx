'use client';

import { memo } from 'react';
import { BaseEdge, EdgeLabelRenderer, getBezierPath, type Position } from '@xyflow/react';
import type { FlowEdgeData } from '@/types/relay-flow';
import { formatBps, formatBytes } from '@/types/relay-flow';

interface AnimatedEdgeProps {
  id: string;
  sourceX: number;
  sourceY: number;
  targetX: number;
  targetY: number;
  sourcePosition: Position;
  targetPosition: Position;
  data?: FlowEdgeData;
  style?: React.CSSProperties;
  markerEnd?: string;
}

function AnimatedEdge({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  data,
  style = {},
  markerEnd,
}: AnimatedEdgeProps) {
  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
  });

  // Calculate stroke width based on bandwidth (optional visual scaling)
  const getStrokeWidth = () => {
    if (!data?.bandwidthBps) return 2;
    if (data.bandwidthBps > 10_000_000) return 4; // > 10 Mbps
    if (data.bandwidthBps > 1_000_000) return 3; // > 1 Mbps
    return 2;
  };

  // Calculate stroke color based on bandwidth
  const getStrokeColor = () => {
    if (!data?.bandwidthBps || data.bandwidthBps === 0) return '#94a3b8'; // gray
    return '#22c55e'; // green when active
  };

  const strokeWidth = getStrokeWidth();
  const strokeColor = getStrokeColor();
  const isActive = data?.bandwidthBps && data.bandwidthBps > 0;

  return (
    <>
      <BaseEdge
        id={id}
        path={edgePath}
        markerEnd={markerEnd}
        style={{
          ...style,
          strokeWidth,
          stroke: strokeColor,
          strokeDasharray: isActive ? undefined : '5,5',
        }}
      />

      {/* Animated flow indicator */}
      {isActive && (
        <circle r="4" fill={strokeColor}>
          <animateMotion dur="2s" repeatCount="indefinite" path={edgePath} />
        </circle>
      )}

      {/* Label showing bandwidth - always show for active connections */}
      <EdgeLabelRenderer>
        <div
          style={{
            position: 'absolute',
            transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
            pointerEvents: 'all',
          }}
          className="nodrag nopan"
        >
          <div
            className={`backdrop-blur-sm border rounded px-1.5 py-0.5 text-xs font-medium ${
              isActive
                ? 'bg-background/90 text-green-600 dark:text-green-400 border-green-500/30'
                : 'bg-background/60 text-muted-foreground/70'
            }`}
          >
            {data?.bandwidthBps !== undefined && data.bandwidthBps > 0
              ? formatBps(data.bandwidthBps)
              : 'idle'}
          </div>
        </div>
      </EdgeLabelRenderer>
    </>
  );
}

export default memo(AnimatedEdge);
