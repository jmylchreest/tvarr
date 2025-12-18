'use client';

import React from 'react';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { LucideIcon } from 'lucide-react';

export interface EmptyStateProps {
  /** Icon to display - can be a LucideIcon component or a rendered ReactNode */
  icon?: LucideIcon | React.ReactNode;
  /** Main title text */
  title: string;
  /** Description text explaining the empty state */
  description?: string;
  /** Primary action button */
  action?: {
    label: string;
    onClick: () => void;
    variant?: 'default' | 'outline' | 'secondary';
  };
  /** Secondary action link */
  secondaryAction?: {
    label: string;
    onClick: () => void;
  };
  /** Additional className for container */
  className?: string;
  /** Size variant */
  size?: 'sm' | 'md' | 'lg';
}

/**
 * EmptyState - A consistent empty state component for lists and tables
 *
 * Usage:
 * ```tsx
 * <EmptyState
 *   icon={FileText}
 *   title="No channels found"
 *   description="Create your first manual channel to get started"
 *   action={{ label: "Add Channel", onClick: handleAdd }}
 * />
 * ```
 */
export const EmptyState: React.FC<EmptyStateProps> = ({
  icon,
  title,
  description,
  action,
  secondaryAction,
  className,
  size = 'md',
}) => {
  const sizeConfig = {
    sm: {
      container: 'py-6 px-4',
      icon: 'h-8 w-8',
      title: 'text-sm font-medium',
      description: 'text-xs',
      gap: 'gap-2',
    },
    md: {
      container: 'py-12 px-6',
      icon: 'h-12 w-12',
      title: 'text-base font-semibold',
      description: 'text-sm',
      gap: 'gap-3',
    },
    lg: {
      container: 'py-16 px-8',
      icon: 'h-16 w-16',
      title: 'text-lg font-semibold',
      description: 'text-base',
      gap: 'gap-4',
    },
  };

  const config = sizeConfig[size];

  // Check if icon is a LucideIcon component (function) or a ReactNode (already rendered)
  const isLucideIcon = typeof icon === 'function';

  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center text-center',
        config.container,
        config.gap,
        className
      )}
    >
      {icon && (
        <div className="rounded-full bg-muted p-3">
          {isLucideIcon ? (
            React.createElement(icon as LucideIcon, {
              className: cn('text-muted-foreground', config.icon),
            })
          ) : (
            icon
          )}
        </div>
      )}
      <div className={cn('flex flex-col', config.gap)}>
        <h3 className={cn('text-foreground', config.title)}>{title}</h3>
        {description && (
          <p className={cn('text-muted-foreground max-w-sm', config.description)}>
            {description}
          </p>
        )}
      </div>
      {(action || secondaryAction) && (
        <div className="flex items-center gap-2 mt-2">
          {action && (
            <Button
              variant={action.variant ?? 'default'}
              size={size === 'sm' ? 'sm' : 'default'}
              onClick={action.onClick}
            >
              {action.label}
            </Button>
          )}
          {secondaryAction && (
            <Button
              variant="ghost"
              size={size === 'sm' ? 'sm' : 'default'}
              onClick={secondaryAction.onClick}
            >
              {secondaryAction.label}
            </Button>
          )}
        </div>
      )}
    </div>
  );
};

export default EmptyState;
