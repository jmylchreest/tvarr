import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// Re-export formatting functions from centralized format library
// for backwards compatibility with existing imports
export { formatDate, formatRelativeTime } from './format';
