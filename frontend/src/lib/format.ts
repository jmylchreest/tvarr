/**
 * Centralized formatting utilities for human-readable display.
 * Use these functions throughout the app for consistent formatting.
 */

import { format, formatDistanceToNow } from 'date-fns';

// =============================================================================
// FILE SIZE FORMATTING
// =============================================================================

/**
 * Format bytes into human readable format.
 * @example formatBytes(1536) => "1.5 KB"
 */
export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
}

/**
 * Alias for formatBytes for semantic clarity.
 */
export const formatFileSize = formatBytes;

/**
 * Format memory size in MB to human readable format.
 * @example formatMemorySize(2048) => "2.0 GB"
 */
export function formatMemorySize(mb: number): string {
  if (mb >= 1024) {
    return `${(mb / 1024).toFixed(1)} GB`;
  }
  return `${mb.toFixed(0)} MB`;
}

// =============================================================================
// BANDWIDTH / BITRATE FORMATTING
// =============================================================================

/**
 * Format bandwidth with /s suffix.
 * @example formatBandwidth(1536000) => "1.5 MB/s"
 */
export function formatBandwidth(bytesPerSecond: number): string {
  if (bytesPerSecond === 0) return '0 B/s';
  const k = 1024;
  const sizes = ['B/s', 'KB/s', 'MB/s', 'GB/s', 'TB/s'];
  const i = Math.floor(Math.log(bytesPerSecond) / Math.log(k));
  return `${parseFloat((bytesPerSecond / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`;
}

/**
 * Format bitrate in Mbps.
 * @example formatBitrate(0.5) => "500 Kbps"
 * @example formatBitrate(5.5) => "5.5 Mbps"
 */
export function formatBitrate(mbps: number): string {
  if (mbps < 1) {
    return `${(mbps * 1000).toFixed(0)} Kbps`;
  }
  return `${mbps.toFixed(1)} Mbps`;
}

// =============================================================================
// DURATION FORMATTING
// =============================================================================

/**
 * Format duration from seconds to human readable format.
 * Shows days/hours/minutes/seconds, hiding leading zero units.
 * @example formatDuration(3661) => "1h 1m 1s"
 * @example formatDuration(90061) => "1d 1h 1m 1s"
 */
export function formatDuration(seconds: number): string {
  if (seconds < 0) seconds = 0;

  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = Math.floor(seconds % 60);

  if (days > 0) {
    return `${days}d ${hours}h ${minutes}m ${secs}s`;
  } else if (hours > 0) {
    return `${hours}h ${minutes}m ${secs}s`;
  } else if (minutes > 0) {
    return `${minutes}m ${secs}s`;
  } else {
    return `${secs}s`;
  }
}

/**
 * Format duration from milliseconds to human readable format.
 * @example formatDurationMs(3661000) => "1h 1m 1s"
 */
export function formatDurationMs(ms: number): string {
  return formatDuration(Math.floor(ms / 1000));
}

/**
 * Format duration in a short format (largest unit only).
 * @example formatDurationShort(3661) => "1h"
 * @example formatDurationShort(90061) => "1d"
 */
export function formatDurationShort(seconds: number): string {
  if (seconds < 0) seconds = 0;

  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);

  if (days > 0) {
    return `${days}d`;
  } else if (hours > 0) {
    return `${hours}h`;
  } else if (minutes > 0) {
    return `${minutes}m`;
  } else {
    return `${Math.floor(seconds)}s`;
  }
}

/**
 * Alias for formatDuration for semantic clarity.
 */
export const formatUptime = formatDuration;

/**
 * Alias for formatDuration for backwards compatibility.
 */
export const formatUptimeFromSeconds = formatDuration;

// =============================================================================
// DATE / TIME FORMATTING
// =============================================================================

/**
 * Format a date to a full human-readable format.
 * @example formatDate("2025-01-15T10:30:00Z") => "Jan 15, 2025 at 10:30 AM"
 */
export function formatDate(date: string | Date): string {
  try {
    return format(new Date(date), "MMM d, yyyy 'at' h:mm a");
  } catch {
    return 'Invalid date';
  }
}

/**
 * Format a date to just the date portion.
 * @example formatDateShort("2025-01-15T10:30:00Z") => "Jan 15, 2025"
 */
export function formatDateShort(date: string | Date): string {
  try {
    return format(new Date(date), 'MMM d, yyyy');
  } catch {
    return 'Invalid date';
  }
}

/**
 * Format a date to just the time portion.
 * @example formatTime("2025-01-15T10:30:00Z") => "10:30 AM"
 */
export function formatTime(date: string | Date): string {
  try {
    return format(new Date(date), 'h:mm a');
  } catch {
    return 'Invalid time';
  }
}

/**
 * Format a date as relative time using date-fns.
 * @example formatRelativeTime("2025-01-15T10:30:00Z") => "5 minutes ago"
 */
export function formatRelativeTime(date: string | Date): string {
  try {
    return formatDistanceToNow(new Date(date), { addSuffix: true });
  } catch {
    return 'Unknown';
  }
}

/**
 * Format a date as short relative time (compact format).
 * @example formatRelativeTimeShort("2025-01-15T10:30:00Z") => "5m ago"
 */
export function formatRelativeTimeShort(date: string | Date): string {
  try {
    const now = new Date();
    const d = new Date(date);
    const diffMs = now.getTime() - d.getTime();
    const diffSecs = Math.floor(diffMs / 1000);
    const diffMins = Math.floor(diffSecs / 60);
    const diffHours = Math.floor(diffMins / 60);
    const diffDays = Math.floor(diffHours / 24);

    if (diffDays > 0) {
      return `${diffDays}d ago`;
    } else if (diffHours > 0) {
      return `${diffHours}h ago`;
    } else if (diffMins > 0) {
      return `${diffMins}m ago`;
    } else {
      return 'Just now';
    }
  } catch {
    return 'Unknown';
  }
}

// =============================================================================
// NUMBER FORMATTING
// =============================================================================

/**
 * Format a number with thousand separators.
 * @example formatNumber(1234567) => "1,234,567"
 */
export function formatNumber(n: number): string {
  return n.toLocaleString();
}

/**
 * Format a number in compact notation.
 * @example formatNumberCompact(1234567) => "1.2M"
 */
export function formatNumberCompact(n: number): string {
  if (n >= 1_000_000_000) {
    return `${(n / 1_000_000_000).toFixed(1)}B`;
  } else if (n >= 1_000_000) {
    return `${(n / 1_000_000).toFixed(1)}M`;
  } else if (n >= 1_000) {
    return `${(n / 1_000).toFixed(1)}K`;
  }
  return n.toString();
}

/**
 * Format a percentage.
 * @example formatPercentage(45.678) => "45.7%"
 * @example formatPercentage(45.678, 0) => "46%"
 */
export function formatPercentage(value: number, decimals: number = 1): string {
  return `${value.toFixed(decimals)}%`;
}

// =============================================================================
// UTILITY
// =============================================================================

/**
 * Parse a string number from API (they sometimes come as strings).
 */
export function parseStringNumber(str: string): number {
  return parseFloat(str) || 0;
}

/**
 * Format time duration from a connected-at timestamp.
 */
export function formatTimeConnected(connectedAt: string): string {
  try {
    return formatDistanceToNow(new Date(connectedAt), { addSuffix: false });
  } catch {
    return 'Unknown';
  }
}
