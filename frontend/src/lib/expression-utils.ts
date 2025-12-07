/**
 * Shared utilities for expression editors.
 */

import type { ExpressionValidationResponse, ExpressionValidationError } from '@/types/api';

/**
 * Calculate cursor position within a textarea for popup positioning.
 * Uses a hidden mirror div with a marker span to measure exact cursor location.
 */
export function getCursorPosition(textarea: HTMLTextAreaElement): { x: number; y: number } {
  const { selectionStart } = textarea;
  const rect = textarea.getBoundingClientRect();
  const style = window.getComputedStyle(textarea);

  // Create a mirror div that matches the textarea exactly
  const div = document.createElement('div');
  div.style.position = 'absolute';
  div.style.visibility = 'hidden';
  div.style.whiteSpace = 'pre-wrap';
  div.style.wordWrap = 'break-word';
  div.style.font = style.font;
  div.style.padding = style.padding;
  div.style.border = style.border;
  div.style.width = style.width;
  div.style.lineHeight = style.lineHeight;
  div.style.overflow = 'hidden';

  // Insert text before cursor, then a marker span
  const textBeforeCursor = textarea.value.substring(0, selectionStart);
  const marker = document.createElement('span');
  marker.textContent = '|';

  div.textContent = textBeforeCursor;
  div.appendChild(marker);

  document.body.appendChild(div);
  const markerRect = marker.getBoundingClientRect();
  const divRect = div.getBoundingClientRect();
  document.body.removeChild(div);

  // Calculate position relative to textarea, accounting for scroll
  const x = rect.left + (markerRect.left - divRect.left);
  const y = rect.top + (markerRect.top - divRect.top) - textarea.scrollTop;

  return { x, y };
}

/**
 * Get the current word being typed at cursor position.
 * Returns the word and its starting position.
 */
export function getCurrentWord(text: string, cursorPos: number): { word: string; start: number } {
  let start = cursorPos;
  while (start > 0 && /[a-zA-Z0-9_]/.test(text[start - 1])) {
    start--;
  }
  return { word: text.substring(start, cursorPos), start };
}

/**
 * Find word boundaries from a character position.
 */
export function findWordBoundaries(text: string, position: number): { start: number; end: number } {
  const isWordChar = (char: string) => /[a-zA-Z0-9_]/.test(char);

  let start = position;
  let end = position;

  // Find start of word
  while (start > 0 && isWordChar(text[start - 1])) {
    start--;
  }

  // Find end of word
  while (end < text.length && isWordChar(text[end])) {
    end++;
  }

  return { start, end };
}

/**
 * Validation state result for simple display.
 */
export interface ValidationState {
  isValid: boolean | null;
  error: string | null;
}

/**
 * Extract simple validation state from validation response.
 */
export function getValidationState(validation: ExpressionValidationResponse | null): ValidationState {
  if (!validation) {
    return { isValid: null, error: null };
  }

  return {
    isValid: validation.is_valid,
    error: validation.error || null,
  };
}

/**
 * Category validation state with errors.
 */
export interface CategoryValidationState {
  isValid: boolean | null;
  errors: ExpressionValidationError[];
}

/**
 * Get validation state for a specific error category.
 */
export function getCategoryValidationState(
  validation: ExpressionValidationResponse | null,
  category: string
): CategoryValidationState {
  if (!validation) {
    return { isValid: null, errors: [] };
  }

  const categoryErrors = validation.errors?.filter((err) => err.category === category) || [];
  return {
    isValid: categoryErrors.length === 0,
    errors: categoryErrors,
  };
}

/**
 * Error highlight for inline error display.
 */
export interface ErrorHighlight {
  start: number;
  end: number;
  error: ExpressionValidationError;
}

/**
 * Create error highlights from validation errors.
 */
export function createErrorHighlights(
  errors: ExpressionValidationError[],
  expression: string
): ErrorHighlight[] {
  const highlights: ErrorHighlight[] = [];

  errors.forEach((error) => {
    if (error.position !== undefined && error.position >= 0 && error.position < expression.length) {
      const { start, end } = findWordBoundaries(expression, error.position);
      highlights.push({
        start,
        end,
        error,
      });
    }
  });

  return highlights;
}

/**
 * Truncate a string for display, adding ellipsis if needed.
 */
export function truncateString(str: string, maxLength: number): string {
  if (str.length <= maxLength) return str;
  return str.substring(0, maxLength) + '...';
}
