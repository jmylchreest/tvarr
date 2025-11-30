'use client';

import { useState, useEffect, useRef } from 'react';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { AlertCircle, X } from 'lucide-react';
import { Button } from '@/components/ui/button';

interface ConflictNotificationProps {
  children: React.ReactNode;
  show: boolean;
  message?: string;
  onDismiss?: () => void;
  autoClose?: number; // milliseconds, defaults to 4000
}

export function ConflictNotification({
  children,
  show,
  message = 'Operation already in progress. Please wait for it to complete.',
  onDismiss,
  autoClose = 4000,
}: ConflictNotificationProps) {
  const [isOpen, setIsOpen] = useState(false);
  const timeoutRef = useRef<NodeJS.Timeout | null>(null);

  useEffect(() => {
    if (show) {
      setIsOpen(true);

      // Auto-close after specified time
      if (autoClose > 0) {
        timeoutRef.current = setTimeout(() => {
          handleDismiss();
        }, autoClose);
      }
    }

    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
      }
    };
  }, [show, autoClose]);

  const handleDismiss = () => {
    setIsOpen(false);
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
      timeoutRef.current = null;
    }
    onDismiss?.();
  };

  return (
    <Popover open={isOpen} onOpenChange={setIsOpen}>
      <PopoverTrigger asChild>{children}</PopoverTrigger>
      <PopoverContent
        className="w-80 p-0 border-amber-200 bg-amber-50"
        side="bottom"
        align="center"
        sideOffset={8}
      >
        <Alert className="border-0 bg-transparent">
          <AlertCircle className="h-4 w-4 text-amber-600" />
          <AlertDescription className="text-amber-800 pr-8">{message}</AlertDescription>
          <Button
            variant="ghost"
            size="sm"
            onClick={handleDismiss}
            className="absolute right-1 top-1 h-6 w-6 p-0 text-amber-600 hover:text-amber-800 hover:bg-amber-100"
          >
            <X className="h-3 w-3" />
          </Button>
        </Alert>
      </PopoverContent>
    </Popover>
  );
}
