'use client';

import React from 'react';
import { cn } from '@/lib/utils';
import { Checkbox } from '@/components/ui/checkbox';
import { Badge } from '@/components/ui/badge';

export interface SelectableListItemBadge {
  label: string;
  variant?: 'default' | 'secondary' | 'outline' | 'destructive';
}

export interface SelectableListItemProps {
  /** Unique identifier */
  id: string;
  /** Primary text */
  title: string;
  /** Secondary text (shown after title, muted) */
  subtitle?: string;
  /** Badges to display inline */
  badges?: SelectableListItemBadge[];
  /** Whether the item is selected */
  isSelected: boolean;
  /** Called when selection toggled */
  onToggle: () => void;
  /** Whether the item is disabled */
  disabled?: boolean;
  /** Additional class names */
  className?: string;
}

/**
 * SelectableListItem - A compact, single-line selectable item
 *
 * Designed for use in wizard steps, filter lists, etc.
 * Matches the master list styling for consistency.
 */
export function SelectableListItem({
  id,
  title,
  subtitle,
  badges,
  isSelected,
  onToggle,
  disabled = false,
  className,
}: SelectableListItemProps) {
  return (
    <div
      className={cn(
        'flex items-center gap-2 px-2 py-1.5 rounded-md cursor-pointer transition-colors',
        'hover:bg-accent',
        isSelected && 'bg-accent',
        disabled && 'opacity-50 cursor-not-allowed',
        className
      )}
      onClick={disabled ? undefined : onToggle}
      role="option"
      aria-selected={isSelected}
      aria-disabled={disabled}
    >
      <Checkbox
        checked={isSelected}
        onCheckedChange={disabled ? undefined : onToggle}
        onClick={(e) => e.stopPropagation()}
        disabled={disabled}
        className="flex-shrink-0"
      />
      <div className="flex-1 min-w-0 flex items-center gap-2 overflow-hidden">
        <span className={cn('text-sm font-medium truncate', disabled && 'text-muted-foreground')}>
          {title}
        </span>
        {subtitle && (
          <span className="text-xs text-muted-foreground truncate flex-shrink-0">
            {subtitle}
          </span>
        )}
      </div>
      {badges && badges.length > 0 && (
        <div className="flex-shrink-0 flex items-center gap-1">
          {badges.map((badge, index) => (
            <Badge
              key={index}
              variant={badge.variant || 'secondary'}
              className="text-[10px] px-1.5 py-0"
            >
              {badge.label}
            </Badge>
          ))}
        </div>
      )}
    </div>
  );
}

export default SelectableListItem;
