'use client';

import { memo, useMemo } from 'react';
import { Cpu, MemoryStick } from 'lucide-react';

interface ResourceSparklineProps {
  cpuHistory?: number[];
  memoryHistory?: number[];
  currentCpu?: number;
  currentMemoryMb?: number;
  className?: string;
}

// Format memory in human-readable form
function formatMemory(mb: number): string {
  if (mb < 1024) {
    return `${mb.toFixed(0)} MB`;
  }
  return `${(mb / 1024).toFixed(1)} GB`;
}

// Generate SVG sparkline path
function generateSparklinePath(
  history: number[],
  width: number,
  height: number,
  maxValue?: number
): { linePath: string; areaPath: string } {
  if (!history || history.length === 0) {
    return { linePath: '', areaPath: '' };
  }

  // Filter out NaN/undefined/null values
  const validHistory = history.filter((v) => typeof v === 'number' && !isNaN(v) && isFinite(v));
  if (validHistory.length === 0) {
    return { linePath: '', areaPath: '' };
  }

  const padding = 1;
  // Find max value for scaling (with minimum to avoid flat lines)
  const max = maxValue ?? Math.max(...validHistory, 1);

  // Generate points using filtered valid history
  const points = validHistory.map((value, index) => {
    const x = padding + (index / (validHistory.length - 1 || 1)) * (width - 2 * padding);
    const y = height - padding - (value / max) * (height - 2 * padding);
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
}

function ResourceSparkline({
  cpuHistory,
  memoryHistory,
  currentCpu,
  currentMemoryMb,
  className = '',
}: ResourceSparklineProps) {
  // Generate CPU sparkline paths - use viewBox width of 100 for better resolution
  const cpuPaths = useMemo(() => {
    return generateSparklinePath(cpuHistory || [], 100, 16, 100); // CPU max is 100%
  }, [cpuHistory]);

  // Generate memory sparkline paths
  const memPaths = useMemo(() => {
    // For memory, use dynamic max based on history
    const maxMem =
      memoryHistory && memoryHistory.length > 0
        ? Math.max(...memoryHistory) * 1.2 // Add 20% headroom
        : undefined;
    return generateSparklinePath(memoryHistory || [], 100, 16, maxMem);
  }, [memoryHistory]);

  const hasCpuHistory = cpuHistory && cpuHistory.length > 0;
  const hasMemHistory = memoryHistory && memoryHistory.length > 0;
  const hasCpu = currentCpu !== undefined;
  const hasMem = currentMemoryMb !== undefined;

  if (!hasCpu && !hasMem && !hasCpuHistory && !hasMemHistory) {
    return null;
  }

  // Stacked layout: CPU row on top, Memory row below
  // Each row has: icon | sparkline (fills space) | value label
  return (
    <div className={`space-y-1 ${className}`}>
      {/* CPU Row */}
      {(hasCpuHistory || hasCpu) && (
        <div className="flex items-center gap-1.5">
          <Cpu className="h-3 w-3 text-blue-500 shrink-0" />
          <div className="flex-1 min-w-0">
            {hasCpuHistory ? (
              <svg viewBox="0 0 100 16" className="w-full h-4" preserveAspectRatio="none">
                <path d={cpuPaths.areaPath} className="fill-blue-500/20" />
                <path
                  d={cpuPaths.linePath}
                  fill="none"
                  className="stroke-blue-500"
                  strokeWidth="1.5"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
              </svg>
            ) : (
              <div className="h-4" /> // Placeholder for alignment
            )}
          </div>
          {hasCpu && (
            <span className="text-[10px] text-blue-600 dark:text-blue-400 tabular-nums shrink-0 w-12 text-right">
              {currentCpu.toFixed(0)}%
            </span>
          )}
        </div>
      )}

      {/* Memory Row */}
      {(hasMemHistory || hasMem) && (
        <div className="flex items-center gap-1.5">
          <MemoryStick className="h-3 w-3 text-green-500 shrink-0" />
          <div className="flex-1 min-w-0">
            {hasMemHistory ? (
              <svg viewBox="0 0 100 16" className="w-full h-4" preserveAspectRatio="none">
                <path d={memPaths.areaPath} className="fill-green-500/20" />
                <path
                  d={memPaths.linePath}
                  fill="none"
                  className="stroke-green-500"
                  strokeWidth="1.5"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
              </svg>
            ) : (
              <div className="h-4" /> // Placeholder for alignment
            )}
          </div>
          {hasMem && (
            <span className="text-[10px] text-green-600 dark:text-green-400 tabular-nums shrink-0 w-12 text-right">
              {formatMemory(currentMemoryMb)}
            </span>
          )}
        </div>
      )}
    </div>
  );
}

export default memo(ResourceSparkline);
export { ResourceSparkline };
