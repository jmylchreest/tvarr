'use client';

import React, { useRef, useState, useEffect, useMemo } from 'react';
import { cn } from '@/lib/utils';
import { Badge } from '@/components/ui/badge';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';

export type BadgePriority = 'error' | 'warning' | 'success' | 'info' | 'default' | 'secondary' | 'outline';

export interface BadgeItem {
  label: string;
  /** Priority determines sort order and color. Higher priority items shown first. */
  priority?: BadgePriority;
  /** Direct variant override (auto-mapped from priority if not specified) */
  variant?: 'default' | 'secondary' | 'outline' | 'destructive';
}

export type BadgeAnimation = 'none' | 'sweep' | 'sparkle' | 'pulse' | 'border' | 'bounce';

export interface BadgeGroupProps {
  badges: BadgeItem[];
  /** Maximum width available for badges. If not set, uses container width */
  maxWidth?: number;
  /** Size preset - affects badge padding */
  size?: 'sm' | 'default';
  /** Animation style to apply. Use 'shimmer' for active operations. */
  animate?: BadgeAnimation;
  /** Additional class names */
  className?: string;
}

// Priority order: higher value = higher priority = shown first
const PRIORITY_ORDER: Record<BadgePriority, number> = {
  error: 100,
  warning: 80,
  success: 60,
  info: 40,
  default: 30,
  secondary: 20,
  outline: 10,
};

// Map priority to badge variant
const PRIORITY_TO_VARIANT: Record<BadgePriority, 'default' | 'secondary' | 'outline' | 'destructive'> = {
  error: 'destructive',
  warning: 'secondary', // Will use custom amber color
  success: 'secondary', // Will use custom green color
  info: 'secondary',    // Muted grey
  default: 'secondary', // Muted grey
  secondary: 'secondary', // Muted grey
  outline: 'outline',   // Muted white/subtle
};

// Colors for the collapsed indicator bands
const PRIORITY_COLORS: Record<BadgePriority, string> = {
  error: 'bg-destructive',
  warning: 'bg-amber-500',
  success: 'bg-emerald-500',
  info: 'bg-muted-foreground/60',
  default: 'bg-muted-foreground/60',
  secondary: 'bg-muted-foreground/40',
  outline: 'bg-muted-foreground/30',
};

/**
 * BadgeGroup - A smart badge container that collapses excess badges
 *
 * Shows as many full badges as fit in the available space.
 * Remaining badges collapse into colored bands that expand on hover.
 * Badges are sorted by priority (error > warning > success > info > default).
 */
export function BadgeGroup({
  badges,
  maxWidth,
  size = 'sm',
  animate = 'none',
  className,
}: BadgeGroupProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [containerWidth, setContainerWidth] = useState(maxWidth || 200);
  const [visibleCount, setVisibleCount] = useState(badges.length);
  const measureRef = useRef<HTMLDivElement>(null);

  // Sort badges by priority
  const sortedBadges = useMemo(() => {
    return [...badges].sort((a, b) => {
      const priorityA = PRIORITY_ORDER[a.priority || 'default'];
      const priorityB = PRIORITY_ORDER[b.priority || 'default'];
      return priorityB - priorityA;
    });
  }, [badges]);

  // Measure container width
  useEffect(() => {
    if (!containerRef.current || maxWidth) return;

    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setContainerWidth(entry.contentRect.width);
      }
    });

    observer.observe(containerRef.current);
    return () => observer.disconnect();
  }, [maxWidth]);

  // Calculate how many badges fit
  useEffect(() => {
    if (!measureRef.current || sortedBadges.length === 0) {
      setVisibleCount(sortedBadges.length);
      return;
    }

    const badgeElements = measureRef.current.querySelectorAll('[data-badge]');
    let totalWidth = 0;
    let count = 0;
    const gap = 4; // gap-1 = 4px
    const collapsedWidth = 24; // Width reserved for collapsed indicator
    const availableWidth = (maxWidth || containerWidth) - collapsedWidth;

    for (let i = 0; i < badgeElements.length; i++) {
      const badgeWidth = badgeElements[i].getBoundingClientRect().width;
      const newTotal = totalWidth + badgeWidth + (count > 0 ? gap : 0);

      if (newTotal <= availableWidth || count === 0) {
        totalWidth = newTotal;
        count++;
      } else {
        break;
      }
    }

    // If all badges fit without the collapsed indicator, show them all
    const fullWidth = Array.from(badgeElements).reduce((sum, el, i) => {
      return sum + el.getBoundingClientRect().width + (i > 0 ? gap : 0);
    }, 0);

    if (fullWidth <= (maxWidth || containerWidth)) {
      setVisibleCount(sortedBadges.length);
    } else {
      setVisibleCount(Math.max(1, count));
    }
  }, [sortedBadges, containerWidth, maxWidth]);

  const visibleBadges = sortedBadges.slice(0, visibleCount);
  const collapsedBadges = sortedBadges.slice(visibleCount);

  const getVariant = (badge: BadgeItem) => {
    if (badge.variant) return badge.variant;
    return PRIORITY_TO_VARIANT[badge.priority || 'default'];
  };

  const getBadgeClassName = (badge: BadgeItem) => {
    const baseClass = size === 'sm' ? 'text-[10px] px-1.5 py-0' : '';

    // Custom colors for warning and success
    if (badge.priority === 'warning' && !badge.variant) {
      return cn(baseClass, 'bg-amber-500 text-white border-transparent');
    }
    if (badge.priority === 'success' && !badge.variant) {
      return cn(baseClass, 'bg-emerald-500 text-white border-transparent');
    }

    return baseClass;
  };

  if (badges.length === 0) return null;

  // Only show tooltip if there are collapsed badges
  const hasCollapsed = collapsedBadges.length > 0;

  // Animation class based on animate prop
  const getAnimationClass = () => {
    switch (animate) {
      case 'sweep':
        return 'badge-sweep rounded-md';
      case 'sparkle':
        return 'badge-sparkle rounded-md';
      case 'pulse':
        return 'badge-pulse';
      case 'border':
        return 'badge-border rounded-md';
      case 'bounce':
        return 'badge-bounce';
      default:
        return '';
    }
  };
  const animationClass = getAnimationClass();

  const badgeContent = (
    <div
      ref={containerRef}
      className={cn('flex items-center gap-1 flex-shrink-0', animationClass, hasCollapsed && 'cursor-help', className)}
    >
      {/* Hidden measurement container */}
      <div
        ref={measureRef}
        className="absolute invisible flex items-center gap-1 pointer-events-none"
        aria-hidden="true"
      >
        {sortedBadges.map((badge, index) => (
          <Badge
            key={index}
            data-badge
            variant={getVariant(badge)}
            className={getBadgeClassName(badge)}
          >
            {badge.label.toUpperCase()}
          </Badge>
        ))}
      </div>

      {/* Visible badges */}
      {visibleBadges.map((badge, index) => (
        <Badge
          key={index}
          variant={getVariant(badge)}
          className={getBadgeClassName(badge)}
        >
          {badge.label.toUpperCase()}
        </Badge>
      ))}

      {/* Collapsed indicator bands */}
      {hasCollapsed && (
        <div className="flex items-center gap-0.5 h-4">
          {collapsedBadges.map((badge, index) => (
            <div
              key={index}
              className={cn(
                'w-1 h-full rounded-sm',
                PRIORITY_COLORS[badge.priority || 'default']
              )}
            />
          ))}
        </div>
      )}
    </div>
  );

  // Wrap entire group in tooltip if there are collapsed badges
  if (hasCollapsed) {
    return (
      <TooltipProvider delayDuration={200}>
        <Tooltip>
          <TooltipTrigger asChild>
            {badgeContent}
          </TooltipTrigger>
          <TooltipContent side="top" className="p-2">
            <div className="flex flex-wrap gap-1 max-w-[200px]">
              {sortedBadges.map((badge, index) => (
                <Badge
                  key={index}
                  variant={getVariant(badge)}
                  className={getBadgeClassName(badge)}
                >
                  {badge.label.toUpperCase()}
                </Badge>
              ))}
            </div>
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
    );
  }

  return badgeContent;
}

export default BadgeGroup;
