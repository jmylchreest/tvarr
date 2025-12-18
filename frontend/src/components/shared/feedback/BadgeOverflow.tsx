'use client';

import React, { useState, useRef, useEffect, Children, isValidElement, cloneElement } from 'react';
import { cn } from '@/lib/utils';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { Badge } from '@/components/ui/badge';

export interface BadgeOverflowProps {
  /** Badge elements to display */
  children: React.ReactNode;
  /** Maximum number of badges to show before overflow */
  maxVisible?: number;
  /** Additional className for container */
  className?: string;
}

/**
 * BadgeOverflow - Shows badges with overflow indicator
 *
 * When there are more badges than maxVisible, shows the visible badges
 * plus a "+N" indicator. Hovering shows all badges in a tooltip.
 *
 * Usage:
 * ```tsx
 * <BadgeOverflow maxVisible={1}>
 *   <Badge>STREAM</Badge>
 *   <Badge>INCLUDE</Badge>
 *   <Badge>System</Badge>
 * </BadgeOverflow>
 * ```
 */
export const BadgeOverflow: React.FC<BadgeOverflowProps> = ({
  children,
  maxVisible = 1,
  className,
}) => {
  const childArray = Children.toArray(children).filter(isValidElement);
  const totalCount = childArray.length;
  const overflowCount = totalCount - maxVisible;
  const hasOverflow = overflowCount > 0;

  if (totalCount === 0) {
    return null;
  }

  // Get visible badges (up to maxVisible)
  const visibleBadges = childArray.slice(0, maxVisible);
  const allBadges = childArray;

  if (!hasOverflow) {
    return (
      <div className={cn('flex items-center gap-1', className)}>
        {visibleBadges}
      </div>
    );
  }

  return (
    <TooltipProvider delayDuration={200}>
      <Tooltip>
        <TooltipTrigger asChild>
          <div className={cn('flex items-center gap-0.5 cursor-pointer flex-shrink-0', className)}>
            {visibleBadges}
            <Badge
              variant="outline"
              className="text-[10px] px-1 py-0 h-4 min-w-[20px] flex items-center justify-center bg-muted/50 border-dashed"
            >
              +{overflowCount}
            </Badge>
          </div>
        </TooltipTrigger>
        <TooltipContent
          side="bottom"
          align="end"
          className="p-2"
        >
          <div className="flex flex-wrap gap-1 max-w-[200px]">
            {allBadges}
          </div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
};

export default BadgeOverflow;
