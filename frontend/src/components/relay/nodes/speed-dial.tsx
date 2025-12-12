'use client';

import { memo } from 'react';

interface SpeedDialProps {
  speed: number; // Encoding speed multiplier (1.0 = realtime)
  size?: 'sm' | 'md' | 'lg';
}

/**
 * SpeedDial displays encoding speed as a visual dial/gauge.
 * - Green when >= 1.0x (encoding faster than realtime)
 * - Red when < 1.0x (encoding slower than realtime - will cause playback issues)
 */
function SpeedDialComponent({ speed, size = 'sm' }: SpeedDialProps) {
  // Clamp speed for dial display (0.0 to 2.0 range for visualization)
  const clampedSpeed = Math.min(Math.max(speed, 0), 2);

  // Calculate the dial angle (0 = 0.0x, 180 = 2.0x, 90 = 1.0x realtime)
  const angle = (clampedSpeed / 2) * 180;

  // Color based on speed threshold
  const isUnderrun = speed < 1.0;
  const dialColor = isUnderrun ? '#ef4444' : '#22c55e'; // red-500 or green-500
  const textColor = isUnderrun
    ? 'text-red-500 dark:text-red-400'
    : 'text-green-600 dark:text-green-400';
  const bgColor = isUnderrun
    ? 'bg-red-500/10'
    : 'bg-green-500/10';

  // Size variants
  const sizeConfig = {
    sm: { width: 64, height: 36, radius: 24, strokeWidth: 4, fontSize: 'text-xs' },
    md: { width: 80, height: 44, radius: 30, strokeWidth: 5, fontSize: 'text-sm' },
    lg: { width: 96, height: 52, radius: 36, strokeWidth: 6, fontSize: 'text-base' },
  };

  const config = sizeConfig[size];
  const centerX = config.width / 2;
  const centerY = config.height - 4;

  // SVG arc path for the gauge background and fill
  const createArc = (startAngle: number, endAngle: number) => {
    const startRad = (startAngle - 180) * (Math.PI / 180);
    const endRad = (endAngle - 180) * (Math.PI / 180);

    const startX = centerX + config.radius * Math.cos(startRad);
    const startY = centerY + config.radius * Math.sin(startRad);
    const endX = centerX + config.radius * Math.cos(endRad);
    const endY = centerY + config.radius * Math.sin(endRad);

    const largeArc = endAngle - startAngle > 180 ? 1 : 0;

    return `M ${startX} ${startY} A ${config.radius} ${config.radius} 0 ${largeArc} 1 ${endX} ${endY}`;
  };

  // Needle position
  const needleAngle = (angle - 180) * (Math.PI / 180);
  const needleLength = config.radius - 4;
  const needleX = centerX + needleLength * Math.cos(needleAngle);
  const needleY = centerY + needleLength * Math.sin(needleAngle);

  return (
    <div className={`flex items-center gap-2 px-2 py-1.5 rounded-md ${bgColor}`}>
      {/* Dial SVG */}
      <svg width={config.width} height={config.height} className="shrink-0">
        {/* Background arc (gray) */}
        <path
          d={createArc(0, 180)}
          fill="none"
          stroke="currentColor"
          strokeWidth={config.strokeWidth}
          className="text-muted-foreground/20"
          strokeLinecap="round"
        />

        {/* 1.0x marker line */}
        <line
          x1={centerX}
          y1={centerY - config.radius + config.strokeWidth}
          x2={centerX}
          y2={centerY - config.radius - 2}
          stroke="currentColor"
          strokeWidth={1.5}
          className="text-muted-foreground/50"
        />

        {/* Filled arc showing current speed */}
        <path
          d={createArc(0, angle)}
          fill="none"
          stroke={dialColor}
          strokeWidth={config.strokeWidth}
          strokeLinecap="round"
        />

        {/* Needle */}
        <line
          x1={centerX}
          y1={centerY}
          x2={needleX}
          y2={needleY}
          stroke={dialColor}
          strokeWidth={2}
          strokeLinecap="round"
        />

        {/* Center dot */}
        <circle
          cx={centerX}
          cy={centerY}
          r={3}
          fill={dialColor}
        />
      </svg>

      {/* Speed value */}
      <div className="flex flex-col items-start">
        <span className={`font-mono font-semibold ${config.fontSize} ${textColor}`}>
          {speed.toFixed(2)}x
        </span>
        <span className="text-[9px] text-muted-foreground leading-tight">
          {isUnderrun ? 'underrun' : 'realtime'}
        </span>
      </div>
    </div>
  );
}

export const SpeedDial = memo(SpeedDialComponent);
