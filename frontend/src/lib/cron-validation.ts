// Cron validation utilities for cron expressions
// Default format: 6-field (sec min hour day-of-month month day-of-week)
// Legacy format: 7-field (sec min hour day-of-month month day-of-week year)
// Example: "0 0 */6 * * *" (every 6 hours)

export interface CronValidationResult {
  isValid: boolean;
  error?: string;
  suggestion?: string;
  normalizedExpression?: string;
}

/**
 * Validates a cron expression (6 or 7 fields)
 * Default is 6-field format: sec min hour day-of-month month day-of-week
 * 7-field format (with year) is accepted for legacy compatibility
 * @param cronExpression The cron expression to validate
 * @returns Validation result with error message and suggestions if invalid
 */
export function validateCronExpression(cronExpression: string): CronValidationResult {
  if (!cronExpression || typeof cronExpression !== 'string') {
    return {
      isValid: false,
      error: 'Cron expression is required',
      suggestion: 'Example: "0 0 */6 * * *" (every 6 hours)',
    };
  }

  const trimmed = cronExpression.trim();
  const fields = trimmed.split(/\s+/);

  // Check if it has 6 or 7 fields
  if (fields.length < 6 || fields.length > 7) {
    let suggestion = '';
    if (fields.length === 5) {
      // Common mistake - Unix cron format (5 fields)
      suggestion =
        'Convert from 5-field Unix format to 6-field format by adding seconds: "0 ' + trimmed + '"';
    } else {
      suggestion = 'Example: "0 0 */6 * * *" (every 6 hours)';
    }

    return {
      isValid: false,
      error: `Cron expression must have 6 fields (or 7 for legacy format). Found ${fields.length}. Format: sec min hour day-of-month month day-of-week`,
      suggestion,
    };
  }

  // Basic field validation
  const fieldNames = [
    'seconds',
    'minutes',
    'hours',
    'day-of-month',
    'month',
    'day-of-week',
    'year',
  ];
  const fieldRanges = [
    [0, 59], // seconds
    [0, 59], // minutes
    [0, 23], // hours
    [1, 31], // day of month
    [1, 12], // month
    [0, 6], // day of week (0 = Sunday)
    [1970, 3000], // year (reasonable range)
  ];

  for (let i = 0; i < fields.length; i++) {
    const field = fields[i];
    const fieldName = fieldNames[i];
    const [min, max] = fieldRanges[i];

    // Allow common cron expressions
    if (
      field === '*' ||
      field === '?' ||
      field.includes('/') ||
      field.includes('-') ||
      field.includes(',')
    ) {
      // These are valid cron field patterns - basic validation passed
      continue;
    }

    // Check if it's a valid number within range
    const num = parseInt(field, 10);
    if (isNaN(num)) {
      return {
        isValid: false,
        error: `Invalid ${fieldName} field: "${field}" is not a valid number or cron expression`,
        suggestion: `${fieldName.charAt(0).toUpperCase() + fieldName.slice(1)} should be a number between ${min}-${max}, or use cron patterns like *, */2, 1-5, etc.`,
      };
    }

    if (num < min || num > max) {
      return {
        isValid: false,
        error: `Invalid ${fieldName} field: ${num} is out of range`,
        suggestion: `${fieldName.charAt(0).toUpperCase() + fieldName.slice(1)} should be between ${min} and ${max}`,
      };
    }
  }

  // Normalize to 6-field format (strip year if present)
  const normalizedExpression = fields.length === 7 ? fields.slice(0, 6).join(' ') : trimmed;

  return { isValid: true, normalizedExpression };
}

/**
 * Normalizes a cron expression to 6-field format
 * Accepts both 6-field and 7-field (legacy) formats
 * @param cronExpression The cron expression to normalize
 * @returns The 6-field normalized expression, or the original if invalid
 */
export function normalizeCronExpression(cronExpression: string): string {
  const result = validateCronExpression(cronExpression);
  return result.normalizedExpression || cronExpression;
}

/**
 * Get human-readable description of cron patterns
 * Handles 6-field format: sec min hour day-of-month month day-of-week
 */
export function describeCronExpression(cronExpression: string): string {
  const validation = validateCronExpression(cronExpression);
  if (!validation.isValid) {
    return 'Invalid schedule';
  }

  const normalized = validation.normalizedExpression || cronExpression;
  const fields = normalized.trim().split(/\s+/);
  const [sec, min, hour, dayOfMonth, month, dayOfWeek] = fields;

  // Helper to format time
  const formatTime = (h: string, m: string): string => {
    const hourNum = parseInt(h, 10);
    const minNum = parseInt(m, 10);
    const minStr = minNum.toString().padStart(2, '0');

    if (hourNum === 0 && minNum === 0) return 'midnight';
    if (hourNum === 12 && minNum === 0) return 'noon';

    const period = hourNum >= 12 ? 'PM' : 'AM';
    const hour12 = hourNum === 0 ? 12 : hourNum > 12 ? hourNum - 12 : hourNum;

    if (minNum === 0) {
      return `${hour12}${period}`;
    }
    return `${hour12}:${minStr}${period}`;
  };

  // Helper to format hour (for patterns without specific minute)
  const formatHour = (h: string): string => {
    const hourNum = parseInt(h, 10);
    if (hourNum === 0) return '12AM';
    if (hourNum === 12) return '12PM';
    const period = hourNum >= 12 ? 'PM' : 'AM';
    const hour12 = hourNum > 12 ? hourNum - 12 : hourNum;
    return `${hour12}${period}`;
  };

  // Helper to get day name
  const getDayName = (day: string): string => {
    const days = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
    const num = parseInt(day, 10);
    return days[num] || day;
  };

  // Helper to get short day name
  const getShortDayName = (day: string): string => {
    const days = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
    const num = parseInt(day, 10);
    return days[num] || day;
  };

  // Helper to format list of hours
  const formatHourList = (hourField: string): string => {
    const hours = hourField.split(',').map(h => formatHour(h));
    if (hours.length === 2) return `${hours[0]} and ${hours[1]}`;
    if (hours.length > 2) {
      const last = hours.pop();
      return `${hours.join(', ')}, and ${last}`;
    }
    return hours[0];
  };

  // Every minute (sec=0, min=*, hour=*, day=*, month=*, dow=*)
  if (min === '*' && hour === '*' && dayOfMonth === '*' && dayOfWeek === '*') {
    return 'Every minute';
  }

  // Every minute during specific hour(s) (sec=0, min=*, hour=specific)
  if (min === '*' && hour !== '*' && !hour.includes('/')) {
    if (hour.includes(',')) {
      return `Every minute at ${formatHourList(hour)}`;
    }
    if (hour.includes('-')) {
      const [start, end] = hour.split('-');
      return `Every minute from ${formatHour(start)} to ${formatHour(end)}`;
    }
    if (!isNaN(parseInt(hour, 10))) {
      return `Every minute during ${formatHour(hour)} hour`;
    }
  }

  // Helper to extract start and interval from step patterns like */6 or 0/6
  const extractStep = (field: string): { start: number | null; interval: number } | null => {
    const match = field.match(/^(\*|\d+)\/(\d+)$/);
    if (!match) return null;
    const start = match[1] === '*' ? null : parseInt(match[1], 10);
    const interval = parseInt(match[2], 10);
    return { start, interval };
  };

  // Check for second intervals (less common but possible)
  if (sec.includes('/')) {
    const step = extractStep(sec);
    if (step) {
      if (step.start !== null && step.start !== 0) {
        return `Every ${step.interval} seconds from :${step.start.toString().padStart(2, '0')}`;
      }
      return `Every ${step.interval} seconds`;
    }
  }

  // Check for minute intervals
  if (min.includes('/')) {
    const step = extractStep(min);
    if (step) {
      // Check if it's during specific hour(s)
      if (hour !== '*' && !hour.includes('/')) {
        if (hour.includes(',')) {
          return `Every ${step.interval} minutes at ${formatHourList(hour)}`;
        }
        if (!isNaN(parseInt(hour, 10))) {
          return `Every ${step.interval} minutes during ${formatHour(hour)} hour`;
        }
      }
      if (step.start !== null && step.start !== 0) {
        return `Every ${step.interval} minutes from :${step.start.toString().padStart(2, '0')}`;
      }
      return `Every ${step.interval} minutes`;
    }
  }

  // Check for hourly intervals
  if (hour.includes('/')) {
    const step = extractStep(hour);
    if (step) {
      // Format the starting time
      const startHour = step.start ?? 0;
      const minNum = !isNaN(parseInt(min, 10)) ? parseInt(min, 10) : 0;
      const startTimeStr = `${startHour.toString().padStart(2, '0')}:${minNum.toString().padStart(2, '0')}`;

      // Only show "from HH:MM" if not starting at 00:00
      const showFrom = startHour !== 0 || minNum !== 0;

      if (step.interval === 1) {
        return showFrom ? `Every hour from ${startTimeStr}` : 'Every hour';
      }
      if (step.interval === 12) {
        return showFrom ? `Twice daily from ${startTimeStr}` : 'Twice daily';
      }
      return showFrom ? `Every ${step.interval} hours from ${startTimeStr}` : `Every ${step.interval} hours`;
    }
  }

  // Every hour at specific minute (sec=0, min=specific, hour=*)
  if (hour === '*' && !isNaN(parseInt(min, 10))) {
    const minNum = parseInt(min, 10);
    if (minNum === 0) return 'Every hour';
    return `Every hour at :${minNum.toString().padStart(2, '0')}`;
  }

  // Multiple specific hours at specific minute (e.g., "0 0 0,6,12,18 * * *")
  if (hour.includes(',') && !isNaN(parseInt(min, 10))) {
    const minNum = parseInt(min, 10);
    if (minNum === 0) {
      return `Daily at ${formatHourList(hour)}`;
    }
    const minStr = minNum.toString().padStart(2, '0');
    return `Daily at :${minStr} past ${formatHourList(hour)}`;
  }

  // Specific time patterns
  if (!isNaN(parseInt(hour, 10)) && !isNaN(parseInt(min, 10))) {
    const timeStr = formatTime(hour, min);

    // Check for day-of-week patterns
    if (dayOfWeek !== '*' && dayOfMonth === '*') {
      // Specific day(s) of week
      if (dayOfWeek.includes(',')) {
        const days = dayOfWeek.split(',').map(d => getShortDayName(d)).join(', ');
        return `${days} at ${timeStr}`;
      }
      if (dayOfWeek.includes('-')) {
        const [start, end] = dayOfWeek.split('-');
        return `${getShortDayName(start)}-${getShortDayName(end)} at ${timeStr}`;
      }
      // Single day of week
      return `${getDayName(dayOfWeek)}s at ${timeStr}`;
    }

    // Check for day-of-month patterns
    if (dayOfMonth !== '*') {
      if (dayOfMonth.includes('/')) {
        const interval = dayOfMonth.match(/\*\/(\d+)/);
        if (interval) {
          return `Every ${interval[1]} days at ${timeStr}`;
        }
      }
      // Specific day of month
      const dayNum = parseInt(dayOfMonth, 10);
      if (!isNaN(dayNum)) {
        const suffix = dayNum === 1 ? 'st' : dayNum === 2 ? 'nd' : dayNum === 3 ? 'rd' : 'th';
        return `${dayNum}${suffix} of each month at ${timeStr}`;
      }
    }

    // Daily at specific time
    return `Daily at ${timeStr}`;
  }

  // Fallback for complex patterns
  return normalized;
}

/**
 * Common cron expression templates (6-field format)
 */
export const COMMON_CRON_TEMPLATES = [
  {
    expression: '0 0 */2 * * *',
    description: 'Every 2 hours',
    category: 'frequent',
  },
  {
    expression: '0 0 */6 * * *',
    description: 'Every 6 hours',
    category: 'frequent',
  },
  {
    expression: '0 0 */12 * * *',
    description: 'Every 12 hours (twice daily)',
    category: 'frequent',
  },
  {
    expression: '0 0 0 * * *',
    description: 'Daily at midnight',
    category: 'daily',
  },
  {
    expression: '0 0 2 * * *',
    description: 'Daily at 2:00 AM',
    category: 'daily',
  },
  {
    expression: '0 0 */4 * * *',
    description: 'Every 4 hours',
    category: 'frequent',
  },
  {
    expression: '0 */30 * * * *',
    description: 'Every 30 minutes',
    category: 'frequent',
  },
  {
    expression: '0 0 0 */7 * *',
    description: 'Weekly (every 7 days)',
    category: 'weekly',
  },
] as const;
