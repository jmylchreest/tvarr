import * as React from 'react';
import { cn } from '@/lib/utils';

export interface CodeProps extends React.HTMLAttributes<HTMLElement> {
  variant?: 'default' | 'muted' | 'outline';
  size?: 'sm' | 'default';
}

const Code = React.forwardRef<HTMLElement, CodeProps>(
  ({ className, variant = 'default', size = 'default', ...props }, ref) => {
    return (
      <code
        ref={ref}
        className={cn(
          // Base styles
          'relative rounded font-mono font-medium',
          // Variant styles
          {
            'bg-muted text-muted-foreground': variant === 'muted',
            'border bg-background text-foreground': variant === 'outline',
            'bg-primary/10 text-primary': variant === 'default',
          },
          // Size styles
          {
            'px-1 py-0.5 text-xs': size === 'sm',
            'px-2 py-1 text-sm': size === 'default',
          },
          className
        )}
        {...props}
      />
    );
  }
);
Code.displayName = 'Code';

export { Code };
