'use client';

import { usePageLoading } from '@/providers/PageLoadingProvider';
import { cn } from '@/lib/utils';

interface PageLoadingSpinnerProps {
  className?: string;
  size?: 'sm' | 'md' | 'lg';
  showText?: boolean;
  text?: string;
}

export function PageLoadingSpinner({
  className,
  size = 'md',
  showText = true,
  text = 'Loading...',
}: PageLoadingSpinnerProps) {
  const { isLoading } = usePageLoading();

  if (!isLoading) {
    return null;
  }

  const sizeClasses = {
    sm: 'h-4 w-4',
    md: 'h-6 w-6',
    lg: 'h-8 w-8',
  };

  return (
    <div
      className={cn(
        'fixed inset-0 z-50 flex items-center justify-center bg-background/80 backdrop-blur-sm',
        className
      )}
    >
      <div className="flex flex-col items-center space-y-3">
        <div
          className={cn('animate-spin rounded-full border-b-2 border-primary', sizeClasses[size])}
        />
        {showText && <p className="text-sm text-muted-foreground">{text}</p>}
      </div>
    </div>
  );
}

// Alternative inline spinner for specific sections
export function InlineLoadingSpinner({
  className,
  size = 'sm',
  showText = false,
  text = 'Loading...',
}: PageLoadingSpinnerProps) {
  const { isLoading } = usePageLoading();

  if (!isLoading) {
    return null;
  }

  const sizeClasses = {
    sm: 'h-4 w-4',
    md: 'h-6 w-6',
    lg: 'h-8 w-8',
  };

  return (
    <div className={cn('flex items-center space-x-2', className)}>
      <div
        className={cn('animate-spin rounded-full border-b-2 border-primary', sizeClasses[size])}
      />
      {showText && <span className="text-sm text-muted-foreground">{text}</span>}
    </div>
  );
}
