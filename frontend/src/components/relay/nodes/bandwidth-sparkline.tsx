'use client';

import { memo, useMemo } from 'react';
import { formatBps } from '@/types/relay-flow';

interface BandwidthSparklineProps {
  history?: number[];
  currentBps?: number;
  label?: string;
  color?: 'green' | 'blue' | 'orange' | 'teal' | 'purple';
  className?: string;
}

const colorClasses = {
  green: {
    text: 'text-green-600 dark:text-green-400',
    fill: 'fill-green-500/20',
    stroke: 'stroke-green-500',
  },
  blue: {
    text: 'text-blue-600 dark:text-blue-400',
    fill: 'fill-blue-500/20',
    stroke: 'stroke-blue-500',
  },
  orange: {
    text: 'text-orange-600 dark:text-orange-400',
    fill: 'fill-orange-500/20',
    stroke: 'stroke-orange-500',
  },
  teal: {
    text: 'text-teal-600 dark:text-teal-400',
    fill: 'fill-teal-500/20',
    stroke: 'stroke-teal-500',
  },
  purple: {
    text: 'text-purple-600 dark:text-purple-400',
    fill: 'fill-purple-500/20',
    stroke: 'stroke-purple-500',
  },
};

function BandwidthSparkline({
  history,
  currentBps,
  label,
  color = 'green',
  className = '',
}: BandwidthSparklineProps) {
  const colors = colorClasses[color];

  // Generate SVG path for the sparkline
  const { linePath, areaPath } = useMemo(() => {
    if (!history || history.length === 0) {
      return { linePath: '', areaPath: '' };
    }

    // Filter out NaN/undefined/null values
    const validHistory = history.filter((v) => typeof v === 'number' && !isNaN(v) && isFinite(v));
    if (validHistory.length === 0) {
      return { linePath: '', areaPath: '' };
    }

    const width = 100;
    const height = 20;
    const padding = 1;

    // Find max value for scaling (with minimum to avoid flat lines)
    const maxValue = Math.max(...validHistory, 1);

    // Generate points using filtered valid history
    const points = validHistory.map((value, index) => {
      const x = padding + (index / (validHistory.length - 1 || 1)) * (width - 2 * padding);
      const y = height - padding - (value / maxValue) * (height - 2 * padding);
      return { x, y };
    });

    if (points.length === 0) {
      return { linePath: '', areaPath: '' };
    }

    // Create line path
    const linePathParts = points.map((point, index) => {
      return index === 0 ? `M ${point.x} ${point.y}` : `L ${point.x} ${point.y}`;
    });

    // Create area path (line + bottom closure)
    const areaPathParts = [
      ...linePathParts,
      `L ${points[points.length - 1].x} ${height}`,
      `L ${points[0].x} ${height}`,
      'Z',
    ];

    return {
      linePath: linePathParts.join(' '),
      areaPath: areaPathParts.join(' '),
    };
  }, [history]);

  const hasHistory = history && history.length > 0;
  const hasBandwidth = currentBps !== undefined && currentBps > 0;

  if (!hasBandwidth && !hasHistory) {
    return null;
  }

  return (
    <div className={`space-y-0.5 ${className}`}>
      {/* Sparkline graph */}
      {hasHistory && (
        <svg
          viewBox="0 0 100 20"
          className="w-full h-5 rounded"
          preserveAspectRatio="none"
        >
          {/* Area fill */}
          <path d={areaPath} className={colors.fill} />
          {/* Line stroke */}
          <path
            d={linePath}
            fill="none"
            className={colors.stroke}
            strokeWidth="1.5"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      )}

      {/* Bandwidth label */}
      {hasBandwidth && (
        <div className={`text-xs ${colors.text}`}>
          {formatBps(currentBps)}{label ? ` ${label}` : ''}
        </div>
      )}
    </div>
  );
}

export default memo(BandwidthSparkline);
export { BandwidthSparkline };
