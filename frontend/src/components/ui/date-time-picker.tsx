'use client';

import * as React from 'react';
import { CalendarIcon, ClockIcon } from 'lucide-react';
import { format } from 'date-fns';

import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Calendar } from '@/components/ui/calendar';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';

interface DateTimePickerProps {
  value?: Date;
  onChange?: (date: Date | undefined) => void;
  placeholder?: string;
  className?: string;
  disabled?: boolean;
}

export function DateTimePicker({
  value,
  onChange,
  placeholder = 'Pick a date',
  className,
  disabled = false,
}: DateTimePickerProps) {
  const [open, setOpen] = React.useState(false);
  const [date, setDate] = React.useState<Date | undefined>(value);
  const [time, setTime] = React.useState<string>(value ? format(value, 'HH:mm') : '00:00');

  React.useEffect(() => {
    setDate(value);
    if (value) {
      setTime(format(value, 'HH:mm'));
    }
  }, [value]);

  const handleDateSelect = (selectedDate: Date | undefined) => {
    if (selectedDate) {
      const [hours, minutes] = time.split(':');
      const newDate = new Date(selectedDate);
      newDate.setHours(parseInt(hours, 10), parseInt(minutes, 10), 0, 0);
      setDate(newDate);
      onChange?.(newDate);
    } else {
      setDate(undefined);
      onChange?.(undefined);
    }
  };

  const handleTimeChange = (newTime: string) => {
    setTime(newTime);
    if (date) {
      const [hours, minutes] = newTime.split(':');
      const newDate = new Date(date);
      newDate.setHours(parseInt(hours, 10), parseInt(minutes, 10), 0, 0);
      setDate(newDate);
      onChange?.(newDate);
    }
  };

  const handleHourChange = (hour: string) => {
    const [, minutes] = time.split(':');
    const newTime = `${hour.padStart(2, '0')}:${minutes}`;
    handleTimeChange(newTime);
  };

  const handleMinuteChange = (minute: string) => {
    const [hours] = time.split(':');
    const newTime = `${hours}:${minute.padStart(2, '0')}`;
    handleTimeChange(newTime);
  };

  const hours = Array.from({ length: 24 }, (_, i) => i.toString().padStart(2, '0'));
  const minutes = Array.from({ length: 60 }, (_, i) => i.toString().padStart(2, '0'));

  const [currentHour, currentMinute] = time.split(':');

  return (
    <div className={cn('flex gap-2', className)}>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            className={cn(
              'w-full justify-start text-left font-normal',
              !date && 'text-muted-foreground'
            )}
            disabled={disabled}
          >
            <CalendarIcon className="mr-2 h-4 w-4" />
            {date ? (
              <span className="flex items-center gap-2">
                {format(date, 'MMM dd, yyyy')}
                <ClockIcon className="h-3 w-3 opacity-50" />
                {format(date, 'HH:mm')}
              </span>
            ) : (
              placeholder
            )}
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-auto p-0 z-50" align="start" sideOffset={4}>
          <Calendar
            mode="single"
            selected={date}
            onSelect={handleDateSelect}
            initialFocus
            className="rounded-lg border-0"
          />
          <div className="border-t p-4">
            <div className="space-y-3">
              <Label className="text-sm font-medium flex items-center gap-2">
                <ClockIcon className="h-3 w-3" />
                Time
              </Label>
              <div className="flex items-center gap-2">
                <div className="grid grid-cols-2 gap-2 w-full">
                  <div className="space-y-1">
                    <Label htmlFor="hour-select" className="text-xs text-muted-foreground">
                      Hour
                    </Label>
                    <Select
                      value={currentHour}
                      onValueChange={handleHourChange}
                      disabled={disabled}
                    >
                      <SelectTrigger id="hour-select" className="h-8">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent className="max-h-[200px]">
                        {hours.map((hour) => (
                          <SelectItem key={hour} value={hour}>
                            {hour}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor="minute-select" className="text-xs text-muted-foreground">
                      Minute
                    </Label>
                    <Select
                      value={currentMinute}
                      onValueChange={handleMinuteChange}
                      disabled={disabled}
                    >
                      <SelectTrigger id="minute-select" className="h-8">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent className="max-h-[200px]">
                        {minutes
                          .filter((_, i) => i % 5 === 0)
                          .map((minute) => (
                            <SelectItem key={minute} value={minute}>
                              {minute}
                            </SelectItem>
                          ))}
                      </SelectContent>
                    </Select>
                  </div>
                </div>
              </div>
              <div className="flex justify-between pt-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    const now = new Date();
                    const currentTime = `${now.getHours().toString().padStart(2, '0')}:${now.getMinutes().toString().padStart(2, '0')}`;
                    handleTimeChange(currentTime);
                  }}
                  disabled={disabled}
                  className="text-xs"
                >
                  Now
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => handleTimeChange('00:00')}
                  disabled={disabled}
                  className="text-xs"
                >
                  Reset
                </Button>
              </div>
            </div>
          </div>
        </PopoverContent>
      </Popover>
    </div>
  );
}
