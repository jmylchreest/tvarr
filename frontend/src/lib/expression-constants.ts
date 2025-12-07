/**
 * Shared constants for expression editors.
 * These are used across filter, data-mapping, and client detection expression editors.
 */

/**
 * Valid comparison operators for expressions.
 */
export const EXPRESSION_OPERATORS = [
  { name: 'contains', description: 'Field contains value (case-insensitive)', category: 'comparison' },
  { name: 'equals', description: 'Field equals value exactly', category: 'comparison' },
  { name: 'matches', description: 'Field matches regex pattern', category: 'comparison' },
  { name: 'starts_with', description: 'Field starts with value', category: 'comparison' },
  { name: 'ends_with', description: 'Field ends with value', category: 'comparison' },
  { name: 'greater_than', description: 'Field is greater than value', category: 'comparison' },
  { name: 'less_than', description: 'Field is less than value', category: 'comparison' },
  { name: 'greater_than_or_equal', description: 'Field is greater than or equal to value', category: 'comparison' },
  { name: 'less_than_or_equal', description: 'Field is less than or equal to value', category: 'comparison' },
] as const;

/**
 * Valid logical operators for combining conditions.
 */
export const LOGICAL_OPERATORS = [
  { name: 'AND', description: 'Logical AND between conditions', category: 'logical' },
  { name: 'OR', description: 'Logical OR between conditions', category: 'logical' },
  { name: 'NOT', description: 'Negate the following condition', category: 'logical' },
] as const;

/**
 * All operators combined for autocomplete.
 */
export const ALL_OPERATORS = [...EXPRESSION_OPERATORS, ...LOGICAL_OPERATORS];

/**
 * Valid modifiers that can be applied to operators.
 */
export const EXPRESSION_MODIFIERS = [
  { name: 'not', description: 'Negate the comparison result' },
  { name: 'case_sensitive', description: 'Make comparison case-sensitive' },
] as const;

/**
 * Operator names as a simple array for validation.
 */
export const VALID_OPERATOR_NAMES = EXPRESSION_OPERATORS.map(op => op.name);

/**
 * Modifier names as a simple array for validation.
 */
export const VALID_MODIFIER_NAMES = EXPRESSION_MODIFIERS.map(mod => mod.name);

/**
 * Type definitions for autocomplete suggestions.
 */
export type AutocompleteSuggestionType = 'field' | 'operator' | 'helper' | 'completion';

export interface AutocompleteSuggestion {
  label: string;
  value: string;
  description?: string;
  preview?: string;
  type: AutocompleteSuggestionType;
}

export interface AutocompleteState {
  isOpen: boolean;
  suggestions: AutocompleteSuggestion[];
  selectedIndex: number;
  position: { x: number; y: number };
  loading: boolean;
  context?: AutocompleteContext;
}

export interface AutocompleteContext {
  type: 'field' | 'operator' | 'helper' | 'completion';
  query: string;
  startPos: number;
  endPos: number;
  helper?: Helper;
}

export interface Helper {
  name: string;
  prefix: string;
  description: string;
  example: string;
  completion?: HelperCompletion;
}

export interface HelperCompletion {
  type: 'search' | 'static' | 'function';
  endpoint?: string;
  query_param?: string;
  display_field?: string;
  value_field?: string;
  preview_field?: string;
  min_chars?: number;
  debounce_ms?: number;
  max_results?: number;
  placeholder?: string;
  empty_message?: string;
  options?: Array<{
    label: string;
    value: string;
    description?: string;
  }>;
  context_fields?: string[];
}
