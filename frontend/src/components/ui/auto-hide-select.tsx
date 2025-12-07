'use client';

import * as React from 'react';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';

export interface SelectOption {
  value: string;
  label: string;
}

interface AutoHideSelectProps {
  options: SelectOption[];
  value: string;
  onValueChange: (value: string) => void;
  placeholder?: string;
  className?: string;
  disabled?: boolean;
}

/**
 * A Select component that automatically hides when there's only one option.
 * When hidden, it still maintains the value in state but doesn't render anything.
 * This keeps forms clean by not showing dropdowns with no meaningful choices.
 */
export function AutoHideSelect({
  options,
  value,
  onValueChange,
  placeholder,
  className,
  disabled,
}: AutoHideSelectProps) {
  // If there's 1 or fewer options, don't render the select
  // The value is already set via the form's default state
  if (options.length <= 1) {
    return null;
  }

  return (
    <Select value={value} onValueChange={onValueChange} disabled={disabled}>
      <SelectTrigger className={className}>
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option.value} value={option.value}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

interface AutoHideSelectFieldProps extends AutoHideSelectProps {
  label: string;
  labelClassName?: string;
  id?: string;
  description?: string;
}

/**
 * A complete form field with label that auto-hides when there's only one option.
 * Useful for forms where you want the entire field (label + select) to disappear.
 */
export function AutoHideSelectField({
  label,
  labelClassName,
  id,
  options,
  description,
  ...selectProps
}: AutoHideSelectFieldProps) {
  // If there's 1 or fewer options, don't render the field at all
  if (options.length <= 1) {
    return null;
  }

  return (
    <div className="space-y-2">
      <label htmlFor={id} className={labelClassName}>
        {label}
      </label>
      <AutoHideSelect options={options} {...selectProps} />
      {description && (
        <p className="text-xs text-muted-foreground">{description}</p>
      )}
    </div>
  );
}
