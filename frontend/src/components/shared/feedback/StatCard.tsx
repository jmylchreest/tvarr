'use client';

import React from 'react';
import { cn } from '@/lib/utils';
import { Card, CardContent } from '@/components/ui/card';

export interface StatCardProps {
  /** The title/label for the stat */
  title: string;
  /** The main value to display */
  value: string | number;
  /** Optional icon to display */
  icon?: React.ReactNode;
  /** Optional subtitle (hidden by default for compact view) */
  subtitle?: string;
  /** Whether to show subtitle */
  showSubtitle?: boolean;
  /** Additional className */
  className?: string;
}

/**
 * StatCard - A compact statistics card for displaying metrics
 *
 * Usage:
 * ```tsx
 * <StatCard title="Total Rules" value={22} icon={<Filter />} />
 * ```
 */
export const StatCard: React.FC<StatCardProps> = ({
  title,
  value,
  icon,
  subtitle,
  showSubtitle = false,
  className,
}) => {
  return (
    <Card className={cn('', className)}>
      <CardContent className="p-3">
        <div className="flex items-center justify-between">
          <div className="flex-1 min-w-0">
            <p className="text-xs font-medium text-muted-foreground truncate">{title}</p>
            <p className="text-lg font-semibold mt-0.5">{value}</p>
            {showSubtitle && subtitle && (
              <p className="text-[10px] text-muted-foreground truncate">{subtitle}</p>
            )}
          </div>
          {icon && (
            <div className="flex-shrink-0 ml-2 text-muted-foreground">
              {icon}
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  );
};

export default StatCard;
