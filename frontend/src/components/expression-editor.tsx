'use client';

import { useState, useEffect, useCallback, useRef, forwardRef } from 'react';
import { Textarea } from '@/components/ui/textarea';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { getBackendUrl } from '@/lib/config';
import { Debug } from '@/utils/debug';
import {
  ExpressionValidationResponse,
  ExpressionValidationError,
  ExpressionField,
  ApiResponse,
} from '@/types/api';

export interface ExpressionEditorProps {
  value: string;
  onChange: (value: string) => void;
  onValidationChange?: (validation: ExpressionValidationResponse | null) => void;
  onFieldsChange?: (fields: ExpressionField[]) => void;
  onValidationComplete?: () => void;
  validationEndpoint: string;
  fieldsEndpoint: string;
  sourceType: 'stream' | 'epg';
  placeholder?: string;
  className?: string;
  debounceMs?: number;
  disabled?: boolean;
  onKeyDown?: (event: React.KeyboardEvent<HTMLTextAreaElement>) => void;
  onInput?: () => void;
}

interface ErrorHighlight {
  start: number;
  end: number;
  error: ExpressionValidationError;
}

// Find word boundaries from a character position
function findWordBoundaries(text: string, position: number): { start: number; end: number } {
  // Word characters include letters, numbers, underscores
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

// Create error highlights from the new server response format
function createErrorHighlights(
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

// Component for rendering error highlight overlays with tooltips
function ErrorHighlightOverlay({
  text,
  highlights,
  textareaRef,
}: {
  text: string;
  highlights: ErrorHighlight[];
  textareaRef: React.RefObject<HTMLTextAreaElement>;
}) {
  const [measurements, setMeasurements] = useState<
    Array<{
      highlight: ErrorHighlight;
      left: number;
      top: number;
      width: number;
      height: number;
    }>
  >([]);

  useEffect(() => {
    if (!textareaRef.current || highlights.length === 0) {
      setMeasurements([]);
      return;
    }

    const textarea = textareaRef.current;
    const computedStyle = window.getComputedStyle(textarea);

    // Create a hidden div to measure text
    const measurer = document.createElement('div');
    measurer.style.position = 'absolute';
    measurer.style.visibility = 'hidden';
    measurer.style.whiteSpace = 'pre-wrap';
    measurer.style.wordWrap = 'break-word';
    measurer.style.font = computedStyle.font;
    measurer.style.padding = computedStyle.padding;
    measurer.style.border = computedStyle.border;
    measurer.style.width = computedStyle.width;
    measurer.style.lineHeight = computedStyle.lineHeight;
    document.body.appendChild(measurer);

    const newMeasurements = highlights.map((highlight) => {
      // Text before the error
      const beforeText = text.substring(0, highlight.start);
      // The error text itself
      const errorText = text.substring(highlight.start, highlight.end);

      // Measure position of error start
      measurer.textContent = beforeText;
      const beforeRect = measurer.getBoundingClientRect();

      // Measure the error text width
      measurer.textContent = beforeText + errorText;
      const afterRect = measurer.getBoundingClientRect();

      // Calculate relative position
      const textareaRect = textarea.getBoundingClientRect();
      const paddingLeft = parseInt(computedStyle.paddingLeft, 10);
      const paddingTop = parseInt(computedStyle.paddingTop, 10);

      return {
        highlight,
        left: beforeRect.width + paddingLeft,
        top: beforeRect.height - parseInt(computedStyle.lineHeight, 10) + paddingTop,
        width: afterRect.width - beforeRect.width,
        height: parseInt(computedStyle.lineHeight, 10),
      };
    });

    document.body.removeChild(measurer);
    setMeasurements(newMeasurements);
  }, [text, highlights, textareaRef]);

  if (measurements.length === 0) return null;

  return (
    <TooltipProvider>
      <div className="absolute inset-0 pointer-events-none overflow-hidden">
        {(() => {
          // Group overlapping highlights to show multiple errors in one tooltip
          const groupedMeasurements = new Map<
            string,
            (typeof measurements)[0] & { errors: ExpressionValidationError[] }
          >();

          measurements.forEach((measurement) => {
            const key = `${measurement.left}-${measurement.top}-${measurement.width}`;
            if (groupedMeasurements.has(key)) {
              groupedMeasurements.get(key)!.errors.push(measurement.highlight.error);
            } else {
              groupedMeasurements.set(key, {
                ...measurement,
                errors: [measurement.highlight.error],
              });
            }
          });

          return Array.from(groupedMeasurements.values()).map((group, index) => (
            <Tooltip key={`error-group-${index}`}>
              <TooltipTrigger asChild>
                <div
                  className="absolute pointer-events-auto"
                  style={{
                    left: group.left,
                    top: group.top,
                    width: group.width,
                    height: group.height,
                    background:
                      'repeating-linear-gradient(45deg, transparent, transparent 2px, #ef4444 2px, #ef4444 4px)',
                    borderBottom: '2px wavy #ef4444',
                    cursor: 'help',
                  }}
                />
              </TooltipTrigger>
              <TooltipContent side="bottom" className="max-w-sm">
                <div className="space-y-3">
                  {group.errors.map((error, errorIndex) => (
                    <div key={errorIndex} className="space-y-1">
                      <div className="font-medium text-red-700">{error.message}</div>
                      {error.details && <div className="text-sm text-red-600">{error.details}</div>}
                      {error.suggestion && (
                        <div className="text-sm text-blue-600">💡 {error.suggestion}</div>
                      )}
                      {error.context && (
                        <div className="text-xs text-gray-500 font-mono bg-gray-100 p-1 rounded">
                          Context: {error.context}
                        </div>
                      )}
                      {errorIndex < group.errors.length - 1 && <hr className="border-gray-200" />}
                    </div>
                  ))}
                </div>
              </TooltipContent>
            </Tooltip>
          ));
        })()}
      </div>
    </TooltipProvider>
  );
}

export const ExpressionEditor = forwardRef<HTMLTextAreaElement, ExpressionEditorProps>(
  (
    {
      value,
      onChange,
      onValidationChange,
      onFieldsChange,
      onValidationComplete,
      validationEndpoint,
      fieldsEndpoint,
      sourceType,
      placeholder = 'Enter expression...',
      className,
      debounceMs = 500,
      disabled = false,
      onKeyDown,
      onInput,
    },
    ref
  ) => {
    const [validation, setValidation] = useState<ExpressionValidationResponse | null>(null);
    const [allFields, setAllFields] = useState<ExpressionField[]>([]);
    const [isValidating, setIsValidating] = useState(false);
    const [errorHighlights, setErrorHighlights] = useState<ErrorHighlight[]>([]);

    const internalRef = useRef<HTMLTextAreaElement>(null);
    const textareaRef = (ref as React.RefObject<HTMLTextAreaElement>) || internalRef;
    const debounceTimeoutRef = useRef<NodeJS.Timeout | null>(null);

    // Get filtered fields based on current source type
    // For data mapping, the server already filters by source type, so we use all fields
    const fields = fieldsEndpoint.includes('data-mapping')
      ? allFields
      : allFields.filter((field) => field.source_type === sourceType);

    // Fetch all available fields once on mount
    useEffect(() => {
      const fetchAllFields = async () => {
        try {
          const backendUrl = getBackendUrl();
          const response = await fetch(`${backendUrl}/api/v1${fieldsEndpoint}`);
          if (response.ok) {
            const data = await response.json();

            // Handle both direct array and ApiResponse<T> format
            let fieldsArray = Array.isArray(data) ? data : data.data;

            // Handle special case for data-mapping fields API response format
            if (data.fields && Array.isArray(data.fields)) {
              fieldsArray = data.fields;
            }

            if (fieldsArray && Array.isArray(fieldsArray)) {
              // Map API response format to expected ExpressionField format
              const mappedFields = fieldsArray.map((field) => ({
                name: field.name,
                display_name: field.display_name,
                field_type: field.field_type || field.type || 'string', // Handle both formats
                nullable: field.nullable ?? false,
                source_type: field.source_type || sourceType, // Use sourceType if not provided
              }));
              setAllFields(mappedFields);
            }
          }
        } catch (error) {
          Debug.log('Failed to fetch expression fields:', error);
        }
      };

      fetchAllFields();
    }, [fieldsEndpoint, sourceType]);

    // Update filtered fields when sourceType or allFields changes
    useEffect(() => {
      // For data mapping, the server already filters by source type, so we use all fields
      const filteredFields = fieldsEndpoint.includes('data-mapping')
        ? allFields
        : allFields.filter((field) => field.source_type === sourceType);
      onFieldsChange?.(filteredFields);
    }, [allFields, sourceType, onFieldsChange, fieldsEndpoint]);

    // Debounced validation
    const validateExpression = useCallback(
      async (expression: string) => {
        Debug.log('ExpressionEditor: validateExpression called', {
          expression: expression.slice(0, 50) + '...',
          validationEndpoint,
        });
        if (!expression.trim()) {
          Debug.log('ExpressionEditor: Empty expression, clearing validation');
          setValidation(null);
          setErrorHighlights([]);
          onValidationChange?.(null);
          onValidationComplete?.();
          return;
        }

        setIsValidating(true);

        try {
          const backendUrl = getBackendUrl();
          const response = await fetch(`${backendUrl}/api/v1${validationEndpoint}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              expression: expression,
            }),
          });

          if (response.ok) {
            const data = await response.json();

            // Derive a user-friendly error message if the API uses the new shape without a top-level 'error'
            const derivedError =
              data.error ||
              (!data.is_valid &&
                Array.isArray(data.errors) &&
                data.errors.length > 0 &&
                (data.errors[0].message || data.errors[0].details));

            // New API response format
            const validationResult: ExpressionValidationResponse = {
              is_valid: data.is_valid,
              error: derivedError,
              errors: data.errors || [],
              condition_tree: data.condition_tree,
            };

            setValidation(validationResult);
            onValidationChange?.(validationResult);

            // Create error highlights from the errors array
            if (data.errors && Array.isArray(data.errors)) {
              const highlights = createErrorHighlights(data.errors, expression);
              setErrorHighlights(highlights);
            } else {
              setErrorHighlights([]);
            }

            // Notify that validation is complete
            onValidationComplete?.();
          } else {
            // Handle validation errors
            try {
              const errorData = await response.json();

              const derivedError =
                errorData.error ||
                (!errorData.is_valid &&
                  Array.isArray(errorData.errors) &&
                  errorData.errors.length > 0 &&
                  (errorData.errors[0].message || errorData.errors[0].details)) ||
                'Validation failed';

              const validationResult: ExpressionValidationResponse = {
                is_valid: false,
                error: derivedError,
                errors: errorData.errors || [],
                condition_tree: null,
              };

              setValidation(validationResult);
              onValidationChange?.(validationResult);

              // Create error highlights from the errors array
              if (errorData.errors && Array.isArray(errorData.errors)) {
                const highlights = createErrorHighlights(errorData.errors, expression);
                setErrorHighlights(highlights);
              } else {
                setErrorHighlights([]);
              }

              onValidationComplete?.();
            } catch (parseError) {
              Debug.log('Could not parse validation error response');

              const validationResult: ExpressionValidationResponse = {
                is_valid: false,
                error: 'Failed to parse validation response',
                condition_tree: null,
              };

              setValidation(validationResult);
              onValidationChange?.(validationResult);
              setErrorHighlights([]);
              onValidationComplete?.();
            }
          }
        } catch (error) {
          Debug.log('Failed to validate expression:', error);

          const validationResult: ExpressionValidationResponse = {
            is_valid: false,
            error: 'Network error during validation',
            condition_tree: null,
          };

          setValidation(validationResult);
          onValidationChange?.(validationResult);
          setErrorHighlights([]);
        } finally {
          setIsValidating(false);
          onValidationComplete?.();
        }
      },
      [validationEndpoint, sourceType, onValidationChange, onValidationComplete]
    );

    // Handle value changes with debouncing
    useEffect(() => {
      Debug.log('ExpressionEditor: Value changed, setting up debounced validation', {
        value: value.slice(0, 50) + '...',
        debounceMs,
      });
      if (debounceTimeoutRef.current) {
        clearTimeout(debounceTimeoutRef.current);
      }

      debounceTimeoutRef.current = setTimeout(() => {
        Debug.log('ExpressionEditor: Debounce timeout fired, calling validateExpression');
        validateExpression(value);
      }, debounceMs);

      return () => {
        if (debounceTimeoutRef.current) {
          clearTimeout(debounceTimeoutRef.current);
        }
      };
    }, [value, validateExpression, debounceMs]);

    // Handle textarea input
    const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      onChange(e.target.value);
      onInput?.();
    };

    return (
      <div className="relative">
        <div className="relative">
          <Textarea
            ref={textareaRef}
            value={value}
            onChange={handleInputChange}
            onKeyDown={onKeyDown}
            placeholder={placeholder}
            disabled={disabled}
            className={cn(
              'font-mono min-h-[120px] relative',
              errorHighlights.length > 0 && 'border-orange-500',
              validation?.is_valid === false && 'border-red-500',
              validation?.is_valid === true && 'border-green-500',
              className
            )}
          />

          {/* Error highlighting overlay */}
          {errorHighlights.length > 0 && (
            <ErrorHighlightOverlay
              text={value}
              highlights={errorHighlights}
              textareaRef={textareaRef as any}
            />
          )}

          {/* Loading indicator */}
          {isValidating && (
            <div className="absolute top-2 right-2">
              <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-orange-500" />
            </div>
          )}
        </div>
      </div>
    );
  }
);

ExpressionEditor.displayName = 'ExpressionEditor';
