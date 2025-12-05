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
 * Get human-readable description of common cron patterns
 */
export function describeCronExpression(cronExpression: string): string {
  const validation = validateCronExpression(cronExpression);
  if (!validation.isValid) {
    return 'Invalid cron expression';
  }

  const normalized = validation.normalizedExpression || cronExpression;
  const fields = normalized.trim().split(/\s+/);
  const [sec, min, hour, dayOfMonth, month, dayOfWeek] = fields;

  // Common patterns (6-field)
  if (normalized === '0 0 */6 * * *') {
    return 'Every 6 hours';
  }
  if (normalized === '0 0 */2 * * *') {
    return 'Every 2 hours';
  }
  if (normalized === '0 0 */12 * * *') {
    return 'Every 12 hours (twice daily)';
  }
  if (normalized === '0 0 0 * * *') {
    return 'Daily at midnight';
  }
  if (normalized === '0 0 2 * * *') {
    return 'Daily at 2:00 AM';
  }
  if (normalized === '0 0 */4 * * *') {
    return 'Every 4 hours';
  }
  if (normalized === '0 */30 * * * *') {
    return 'Every 30 minutes';
  }

  // Generate description from pattern
  let description = 'Custom schedule: ';

  if (hour.includes('/')) {
    const interval = hour.match(/\*\/(\d+)/);
    if (interval) {
      description += `every ${interval[1]} hour(s)`;
    }
  } else if (hour === '*') {
    description += 'every hour';
  } else {
    description += `at ${hour}:${min === '*' ? '00' : min.padStart(2, '0')}`;
  }

  return description;
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
