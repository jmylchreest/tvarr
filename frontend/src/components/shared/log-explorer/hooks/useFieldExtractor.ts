import { useMemo } from 'react';
import { LogField } from '../types';

/**
 * Formats a field path into a human-readable display name.
 * Converts snake_case and camelCase to Title Case.
 *
 * @example
 * formatFieldName("fields.channel_name") => "Channel Name"
 * formatFieldName("context.errorMessage") => "Error Message"
 */
export function formatFieldName(path: string): string {
  // Get the last segment of the path
  const lastPart = path.split('.').pop() || path;

  return lastPart
    // Replace underscores with spaces
    .replace(/_/g, ' ')
    // Insert space before capital letters (camelCase)
    .replace(/([a-z])([A-Z])/g, '$1 $2')
    // Capitalize first letter of each word
    .replace(/\b\w/g, (c) => c.toUpperCase());
}

/**
 * Determines the type of a value for display purposes.
 */
function getValueType(value: unknown): LogField['type'] {
  if (value === null || value === undefined) {
    return 'string';
  }
  if (Array.isArray(value)) {
    return 'array';
  }
  if (value instanceof Date) {
    return 'datetime';
  }
  if (typeof value === 'object') {
    return 'object';
  }
  if (typeof value === 'number') {
    return 'number';
  }
  if (typeof value === 'boolean') {
    return 'boolean';
  }
  // Check if string looks like a datetime
  if (typeof value === 'string') {
    const datePattern = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}/;
    if (datePattern.test(value)) {
      return 'datetime';
    }
  }
  return 'string';
}

/**
 * Recursively extracts fields from an object, flattening nested structures.
 */
function extractFieldsFromObject(
  obj: Record<string, unknown>,
  prefix: string,
  fieldMap: Map<string, { count: number; type: LogField['type']; samples: Set<unknown> }>,
  excludeFields: Set<string>,
  maxDepth: number = 3,
  currentDepth: number = 0
): void {
  if (currentDepth >= maxDepth) return;

  for (const [key, value] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${key}` : key;

    // Skip excluded fields
    if (excludeFields.has(path)) continue;
    // Skip null/undefined at top level extraction
    if (value === null || value === undefined) continue;

    // Check if this is a plain object that should be recursed into
    const isPlainObject =
      typeof value === 'object' &&
      value !== null &&
      !Array.isArray(value) &&
      !(value instanceof Date);

    if (isPlainObject && Object.keys(value as object).length > 0) {
      // Recurse into nested objects
      extractFieldsFromObject(
        value as Record<string, unknown>,
        path,
        fieldMap,
        excludeFields,
        maxDepth,
        currentDepth + 1
      );
    } else {
      // Leaf value - add to field map
      const existing = fieldMap.get(path) || {
        count: 0,
        type: getValueType(value),
        samples: new Set<unknown>(),
      };
      existing.count++;

      // Collect sample values (up to 10)
      if (existing.samples.size < 10) {
        const sampleValue =
          typeof value === 'object' ? JSON.stringify(value) : value;
        existing.samples.add(sampleValue);
      }

      fieldMap.set(path, existing);
    }
  }
}

/**
 * Extracts all fields from a collection of records, flattening nested objects.
 *
 * @param records - Array of records to extract fields from
 * @param excludeFields - Field paths to exclude from extraction
 * @returns Array of LogField objects sorted by frequency
 *
 * @example
 * Input: [{ message: "test", fields: { channel: "abc", count: 5 } }]
 * Output fields: ["message", "fields.channel", "fields.count"]
 */
export function extractFields<T extends Record<string, unknown>>(
  records: T[],
  excludeFields: string[] = []
): LogField[] {
  const fieldMap = new Map<
    string,
    { count: number; type: LogField['type']; samples: Set<unknown> }
  >();
  const excludeSet = new Set(excludeFields);

  for (const record of records) {
    extractFieldsFromObject(record, '', fieldMap, excludeSet);
  }

  return Array.from(fieldMap.entries())
    .map(([path, info]) => ({
      path,
      displayName: formatFieldName(path),
      type: info.type,
      count: info.count,
      sampleValues: Array.from(info.samples).slice(0, 5) as (
        | string
        | number
        | boolean
      )[],
    }))
    .sort((a, b) => {
      // Sort by frequency first, then alphabetically
      if (b.count !== a.count) {
        return b.count - a.count;
      }
      return a.path.localeCompare(b.path);
    });
}

/**
 * Gets a nested value from an object using a dot-notation path.
 *
 * @param obj - The object to get the value from
 * @param path - Dot-notation path (e.g., "fields.channel_name")
 * @returns The value at the path, or undefined if not found
 */
export function getNestedValue(obj: Record<string, unknown>, path: string): unknown {
  const parts = path.split('.');
  let current: unknown = obj;

  for (const part of parts) {
    if (current === null || current === undefined) {
      return undefined;
    }
    if (typeof current !== 'object') {
      return undefined;
    }
    current = (current as Record<string, unknown>)[part];
  }

  return current;
}

/**
 * Formats a value for display in the table.
 */
export function formatValue(value: unknown): string {
  if (value === null || value === undefined) {
    return '-';
  }
  if (typeof value === 'boolean') {
    return value ? 'true' : 'false';
  }
  if (typeof value === 'number') {
    return value.toString();
  }
  if (typeof value === 'object') {
    return JSON.stringify(value);
  }
  return String(value);
}

/**
 * Hook to extract and memoize available fields from log data.
 *
 * @param data - Array of log entries
 * @param excludeFields - Fields to exclude from extraction
 * @returns Array of available fields
 */
export function useFieldExtractor<T extends Record<string, unknown>>(
  data: T[],
  excludeFields: string[] = []
): LogField[] {
  return useMemo(() => {
    if (data.length === 0) return [];
    // Sample up to 500 records for field extraction to avoid performance issues
    const sampleSize = Math.min(data.length, 500);
    const sample = data.slice(0, sampleSize);
    return extractFields(sample, excludeFields);
  }, [data, excludeFields]);
}
