'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { Debug } from '@/utils/debug';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Code } from '@/components/ui/code';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { ExpressionEditor } from '@/components/expression-editor';
import { cn } from '@/lib/utils';
import { getBackendUrl } from '@/lib/config';
import {
  ExpressionValidationResponse,
  ExpressionField,
  ExpressionTestResult,
  StreamSourceResponse,
  EpgSourceResponse,
  ApiResponse,
  FilterSourceType,
} from '@/types/api';
import { CheckCircle, XCircle, AlertCircle, RefreshCw, Loader2, Settings } from 'lucide-react';

export interface FilterExpressionEditorProps {
  value: string;
  onChange: (value: string) => void;
  sourceType: FilterSourceType;
  className?: string;
  placeholder?: string;
  disabled?: boolean;
  showTestResults?: boolean;
  autoTest?: boolean;
}

const VALID_OPERATORS = [
  'contains',
  'equals',
  'matches',
  'starts_with',
  'ends_with',
  'greater_than',
  'less_than',
  'greater_than_or_equal',
  'less_than_or_equal',
];

const VALID_MODIFIERS = ['not', 'case_sensitive'];

export function FilterExpressionEditor({
  value,
  onChange,
  sourceType,
  className,
  placeholder,
  disabled = false,
  showTestResults = true,
  autoTest = true,
}: FilterExpressionEditorProps) {
  const [validation, setValidation] = useState<ExpressionValidationResponse | null>(null);
  const [fields, setFields] = useState<ExpressionField[]>([]);
  const [allSources, setAllSources] = useState<(StreamSourceResponse | EpgSourceResponse)[]>([]);
  const [sourceValidations, setSourceValidations] = useState<
    Map<
      string,
      { valid: boolean; matchCount: number; totalCount: number; loading: boolean; error?: string }
    >
  >(new Map());

  // Get filtered sources based on current source type
  const sources = allSources.filter((source) => {
    if ('source_kind' in source) {
      return source.source_kind === sourceType;
    }
    // Fallback for sources without source_kind property
    return sourceType === 'stream' ? 'channel_count' in source : 'program_count' in source;
  });

  // Refs to manage source validation debounce and current state
  const sourceValidationTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const currentValidationRef = useRef<ExpressionValidationResponse | null>(null);
  const currentValueRef = useRef<string>('');
  const currentSourcesRef = useRef<(StreamSourceResponse | EpgSourceResponse)[]>([]);

  // Update refs when state changes
  useEffect(() => {
    currentValidationRef.current = validation;
  }, [validation]);

  useEffect(() => {
    currentValueRef.current = value;
  }, [value]);

  useEffect(() => {
    currentSourcesRef.current = sources;
  }, [sources]);

  // Cleanup timeout on unmount
  useEffect(() => {
    return () => {
      if (sourceValidationTimeoutRef.current) {
        clearTimeout(sourceValidationTimeoutRef.current);
      }
    };
  }, []);

  // Fetch all sources once on mount
  useEffect(() => {
    const fetchAllSources = async () => {
      try {
        const backendUrl = getBackendUrl();
        const response = await fetch(`${backendUrl}/api/v1/sources`);

        if (response.ok) {
          const sourcesData = await response.json();
          if (Array.isArray(sourcesData)) {
            setAllSources(sourcesData.filter((source) => source.is_active));
          }
        }
      } catch (error) {
        console.warn('Failed to fetch sources:', error);
      }
    };

    if (showTestResults) {
      fetchAllSources();
    }
  }, [showTestResults]);

  // Test expression against a single source
  const validateSingleSource = useCallback(
    async (sourceId: string) => {
      Debug.log('🧪 validateSingleSource called for:', sourceId, 'with value:', value);
      if (!value.trim()) {
        Debug.log('❌ No value, returning early');
        return;
      }

      Debug.log('⏳ Setting loading state for source:', sourceId);
      // Set loading state for this source
      setSourceValidations(
        (prev) =>
          new Map(
            prev.set(sourceId, {
              valid: false,
              matchCount: 0,
              totalCount: 0,
              loading: true,
            })
          )
      );

      try {
        const backendUrl = getBackendUrl();
        const testRequest = {
          source_id: sourceId,
          source_type: sourceType,
          filter_expression: value,
          is_inverse: false,
        };
        Debug.log('🚀 Making test request:', testRequest);

        const response = await fetch(`${backendUrl}/api/v1/filters/test`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(testRequest),
        });

        if (response.ok) {
          const data = await response.json();
          setSourceValidations(
            (prev) =>
              new Map(
                prev.set(sourceId, {
                  valid: true, // Test succeeded
                  matchCount: data.matched_count || 0,
                  totalCount: data.total_channels || 0,
                  loading: false,
                  error: undefined,
                })
              )
          );
        } else {
          setSourceValidations(
            (prev) =>
              new Map(
                prev.set(sourceId, {
                  valid: false,
                  matchCount: 0,
                  totalCount: 0,
                  loading: false,
                  error: 'Test failed',
                })
              )
          );
        }
      } catch (error) {
        console.warn(`Failed to test source ${sourceId}:`, error);
        setSourceValidations(
          (prev) =>
            new Map(
              prev.set(sourceId, {
                valid: false,
                matchCount: 0,
                totalCount: 0,
                loading: false,
                error: 'Network error',
              })
            )
        );
      }
    },
    [value, sourceType]
  );

  // Validate expression against all sources
  const validateAllSources = useCallback(async () => {
    if (!value.trim() || sources.length === 0) {
      setSourceValidations(new Map());
      return;
    }

    // Validate each source individually
    await Promise.all(sources.map((source) => validateSingleSource(source.id)));
  }, [sources, validateSingleSource, value]);

  // Get simple validation state for display
  const getValidationState = () => {
    if (!validation) {
      return { isValid: null, error: null };
    }

    return {
      isValid: validation.is_valid,
      error: validation.error,
    };
  };

  // Get validation state for specific category
  const getCategoryValidationState = (category: string) => {
    if (!validation) {
      return { isValid: null, errors: [] };
    }

    const categoryErrors = validation.errors?.filter((err) => err.category === category) || [];
    return {
      isValid: categoryErrors.length === 0,
      errors: categoryErrors,
    };
  };

  // Stable callback functions to prevent infinite re-renders
  const handleValidationChange = useCallback((validation: ExpressionValidationResponse | null) => {
    Debug.log('📝 handleValidationChange called with:', validation);
    setValidation(validation);
  }, []);

  const handleFieldsChange = useCallback((fields: ExpressionField[]) => {
    setFields(fields);
  }, []);

  // Handle validation completion - trigger source testing if validation is valid
  const handleValidationComplete = useCallback(() => {
    // Use a slight delay to ensure validation state is updated
    setTimeout(() => {
      Debug.log('🔍 handleValidationComplete called', {
        autoTest,
        isValid: currentValidationRef.current?.is_valid,
        value: currentValueRef.current?.trim(),
        sourcesLength: currentSourcesRef.current?.length,
      });

      // Clear any existing timeout
      if (sourceValidationTimeoutRef.current) {
        clearTimeout(sourceValidationTimeoutRef.current);
        sourceValidationTimeoutRef.current = null;
      }

      // Always clear source validations first to reset badges to muted state
      setSourceValidations(new Map());

      // Only trigger source testing if auto-test is enabled and expression is valid
      if (
        !autoTest ||
        !currentValidationRef.current?.is_valid ||
        !currentValueRef.current.trim() ||
        currentSourcesRef.current.length === 0
      ) {
        Debug.log('Not triggering source tests - conditions not met', {
          autoTest,
          isValid: currentValidationRef.current?.is_valid,
          hasValue: !!currentValueRef.current?.trim(),
          hasSources: currentSourcesRef.current?.length > 0,
        });
        return;
      }

      Debug.log('Scheduling source tests for', currentSourcesRef.current?.length, 'sources');

      // Debounce source testing to prevent spam
      sourceValidationTimeoutRef.current = setTimeout(async () => {
        const currentSources = currentSourcesRef.current;
        const currentValue = currentValueRef.current;

        Debug.log('Executing source tests', {
          sourcesCount: currentSources?.length,
          value: currentValue,
        });

        if (!currentValue.trim() || currentSources.length === 0) {
          setSourceValidations(new Map());
          sourceValidationTimeoutRef.current = null;
          return;
        }

        // Test each source individually using /filters/test endpoint
        Debug.log(
          'Starting tests for sources:',
          currentSources.map((s) => s.id)
        );
        await Promise.all(
          currentSources.map((source) => {
            Debug.log('Testing source:', source.id);
            return validateSingleSource(source.id);
          })
        );
        sourceValidationTimeoutRef.current = null;
      }, 200); // Short delay to allow for stable validation result
    }, 10); // Very short delay to allow state to update
  }, [autoTest, validateSingleSource]);

  const validationState = getValidationState();

  return (
    <TooltipProvider>
      <div className={cn('space-y-4', className)}>
        {/* Expression Editor */}
        <ExpressionEditor
          value={value}
          onChange={onChange}
          onValidationChange={handleValidationChange}
          onFieldsChange={handleFieldsChange}
          onValidationComplete={handleValidationComplete}
          validationEndpoint={`/expressions/validate?domain=${sourceType === 'epg' ? 'epg_filter' : 'stream_filter'}`}
          fieldsEndpoint={`/filters/fields/${sourceType}`}
          sourceType={sourceType}
          placeholder={placeholder}
          disabled={disabled}
        />

        {/* Status Badges */}
        <div className="flex flex-wrap gap-2">
          {/* Validation Status Badge */}
          <Tooltip>
            <TooltipTrigger asChild>
              <Badge
                className={cn(
                  'gap-1 bg-muted',
                  validationState.isValid === true &&
                    'bg-green-500 hover:bg-green-600 text-white border-transparent',
                  validationState.isValid === false &&
                    'bg-red-500 hover:bg-red-600 text-white border-transparent'
                )}
              >
                {validationState.isValid === true ? (
                  <CheckCircle className="h-3 w-3" />
                ) : validationState.isValid === false ? (
                  <XCircle className="h-3 w-3" />
                ) : (
                  <AlertCircle className="h-3 w-3" />
                )}
                Expression
              </Badge>
            </TooltipTrigger>
            <TooltipContent>
              <div className="space-y-1">
                <p className="font-medium">Expression Validation</p>
                {validationState.isValid === true && <p>✓ Valid expression</p>}
                {validationState.isValid === false && validationState.error && (
                  <p className="text-destructive">✗ {validationState.error}</p>
                )}
                {validationState.isValid === null && <p>Enter expression to validate</p>}
              </div>
            </TooltipContent>
          </Tooltip>

          {/* Syntax Badge */}
          <Tooltip>
            <TooltipTrigger asChild>
              <Badge
                className={cn(
                  'gap-1 bg-muted',
                  getCategoryValidationState('syntax').isValid === true &&
                    'bg-green-500 hover:bg-green-600 text-white border-transparent',
                  getCategoryValidationState('syntax').isValid === false &&
                    'bg-red-500 hover:bg-red-600 text-white border-transparent'
                )}
              >
                {getCategoryValidationState('syntax').isValid === true ? (
                  <CheckCircle className="h-3 w-3" />
                ) : getCategoryValidationState('syntax').isValid === false ? (
                  <XCircle className="h-3 w-3" />
                ) : (
                  <Code className="h-3 w-3" />
                )}
                Syntax
              </Badge>
            </TooltipTrigger>
            <TooltipContent>
              <div className="space-y-2 max-w-xs">
                <p className="font-medium">Expression Syntax</p>
                {getCategoryValidationState('syntax').errors.length > 0 ? (
                  <div className="space-y-1">
                    {getCategoryValidationState('syntax').errors.map((error, idx) => (
                      <div key={idx} className="text-xs">
                        <p className="font-medium text-destructive">{error.message}</p>
                        {error.suggestion && (
                          <p className="text-muted-foreground text-xs">{error.suggestion}</p>
                        )}
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="text-xs text-muted-foreground">
                    {validation && validation.is_valid
                      ? '✓ Syntax is valid'
                      : 'Enter expression to validate syntax'}
                  </p>
                )}
              </div>
            </TooltipContent>
          </Tooltip>

          {/* Fields Reference Badge */}
          <Tooltip>
            <TooltipTrigger asChild>
              <Badge
                className={cn(
                  'gap-1 bg-muted',
                  getCategoryValidationState('field').isValid === true &&
                    'bg-green-500 hover:bg-green-600 text-white border-transparent',
                  getCategoryValidationState('field').isValid === false &&
                    'bg-red-500 hover:bg-red-600 text-white border-transparent'
                )}
              >
                {getCategoryValidationState('field').isValid === true ? (
                  <CheckCircle className="h-3 w-3" />
                ) : getCategoryValidationState('field').isValid === false ? (
                  <XCircle className="h-3 w-3" />
                ) : (
                  <Code className="h-3 w-3" />
                )}
                Fields
              </Badge>
            </TooltipTrigger>
            <TooltipContent>
              <div className="space-y-2 max-w-xs">
                <p className="font-medium">Available {sourceType} Fields</p>
                {getCategoryValidationState('field').errors.length > 0 && (
                  <div className="space-y-1 mb-2">
                    {getCategoryValidationState('field').errors.map((error, idx) => (
                      <div key={idx} className="text-xs">
                        <p className="font-medium text-destructive">{error.message}</p>
                        {error.suggestion && (
                          <p className="text-muted-foreground text-xs">{error.suggestion}</p>
                        )}
                      </div>
                    ))}
                  </div>
                )}
                <div className="grid grid-cols-1 gap-1 text-xs">
                  {fields.map((field) => (
                    <Code key={field.name} variant="muted" size="sm">
                      {field.name} ({field.field_type})
                    </Code>
                  ))}
                </div>
              </div>
            </TooltipContent>
          </Tooltip>

          {/* Operators Reference Badge */}
          <Tooltip>
            <TooltipTrigger asChild>
              <Badge
                className={cn(
                  'gap-1 bg-muted',
                  getCategoryValidationState('operator').isValid === true &&
                    'bg-green-500 hover:bg-green-600 text-white border-transparent',
                  getCategoryValidationState('operator').isValid === false &&
                    'bg-red-500 hover:bg-red-600 text-white border-transparent'
                )}
              >
                {getCategoryValidationState('operator').isValid === true ? (
                  <CheckCircle className="h-3 w-3" />
                ) : getCategoryValidationState('operator').isValid === false ? (
                  <XCircle className="h-3 w-3" />
                ) : (
                  <Settings className="h-3 w-3" />
                )}
                Operators
              </Badge>
            </TooltipTrigger>
            <TooltipContent>
              <div className="space-y-2 max-w-xs">
                <p className="font-medium">Available Operators</p>
                {getCategoryValidationState('operator').errors.length > 0 && (
                  <div className="space-y-1 mb-2">
                    {getCategoryValidationState('operator').errors.map((error, idx) => (
                      <div key={idx} className="text-xs">
                        <p className="font-medium text-destructive">{error.message}</p>
                        {error.suggestion && (
                          <p className="text-muted-foreground text-xs">{error.suggestion}</p>
                        )}
                      </div>
                    ))}
                  </div>
                )}
                <div className="space-y-2">
                  <div>
                    <p className="text-sm font-medium">Comparison:</p>
                    <div className="flex flex-wrap gap-1">
                      {VALID_OPERATORS.map((op) => (
                        <Code key={op} variant="muted" size="sm">
                          {op}
                        </Code>
                      ))}
                    </div>
                  </div>
                  <div>
                    <p className="text-sm font-medium">Modifiers:</p>
                    <div className="flex flex-wrap gap-1">
                      {VALID_MODIFIERS.map((mod) => (
                        <Code key={mod} variant="muted" size="sm">
                          {mod}
                        </Code>
                      ))}
                    </div>
                  </div>
                </div>
              </div>
            </TooltipContent>
          </Tooltip>

          {/* Values Badge */}
          <Tooltip>
            <TooltipTrigger asChild>
              <Badge
                className={cn(
                  'gap-1 bg-muted',
                  getCategoryValidationState('value').isValid === true &&
                    'bg-green-500 hover:bg-green-600 text-white border-transparent',
                  getCategoryValidationState('value').isValid === false &&
                    'bg-red-500 hover:bg-red-600 text-white border-transparent'
                )}
              >
                {getCategoryValidationState('value').isValid === true ? (
                  <CheckCircle className="h-3 w-3" />
                ) : getCategoryValidationState('value').isValid === false ? (
                  <XCircle className="h-3 w-3" />
                ) : (
                  <Code className="h-3 w-3" />
                )}
                Values
              </Badge>
            </TooltipTrigger>
            <TooltipContent>
              <div className="space-y-2 max-w-xs">
                <p className="font-medium">Value Validation</p>
                {getCategoryValidationState('value').errors.length > 0 ? (
                  <div className="space-y-1">
                    {getCategoryValidationState('value').errors.map((error, idx) => (
                      <div key={idx} className="text-xs">
                        <p className="font-medium text-destructive">{error.message}</p>
                        {error.suggestion && (
                          <p className="text-muted-foreground text-xs">{error.suggestion}</p>
                        )}
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="text-xs text-muted-foreground">
                    {validation && validation.is_valid
                      ? '✓ Values are valid'
                      : 'Enter expression to validate values'}
                  </p>
                )}
              </div>
            </TooltipContent>
          </Tooltip>

          {/* Source Test Badges */}
          {showTestResults &&
            sources.map((source) => {
              const testResult = sourceValidations.get(source.id);
              const fullSourceName = source.name || `Source ${source.id}`;
              const sourceName =
                fullSourceName.length > 8 ? fullSourceName.substring(0, 8) + '...' : fullSourceName;

              // Determine badge state
              const isLoading = testResult?.loading;
              const hasError = testResult?.error;
              const isTestedSuccessfully = testResult && !testResult.loading && !testResult.error;
              const isUntested = !testResult;

              return (
                <Tooltip key={source.id}>
                  <TooltipTrigger asChild>
                    <Badge
                      className={cn(
                        'gap-1 bg-muted',
                        isTestedSuccessfully &&
                          'bg-green-500 hover:bg-green-600 text-white border-transparent',
                        hasError && 'bg-red-500 hover:bg-red-600 text-white border-transparent'
                      )}
                    >
                      {isLoading ? (
                        <Loader2 className="h-3 w-3 animate-spin" />
                      ) : hasError ? (
                        <XCircle className="h-3 w-3" />
                      ) : isTestedSuccessfully ? (
                        <CheckCircle className="h-3 w-3" />
                      ) : (
                        <AlertCircle className="h-3 w-3" />
                      )}
                      {sourceName}:{' '}
                      {isLoading
                        ? 'Testing...'
                        : hasError
                          ? 'Error'
                          : isTestedSuccessfully
                            ? `${testResult.matchCount}/${testResult.totalCount}`
                            : '-/-'}
                    </Badge>
                  </TooltipTrigger>
                  <TooltipContent>
                    <div className="space-y-1">
                      <p className="font-medium">{fullSourceName}</p>
                      {isLoading && <p>Testing filter against source...</p>}
                      {hasError ? (
                        <p>Error: {testResult.error}</p>
                      ) : isTestedSuccessfully ? (
                        <div>
                          <p>Matched: {testResult.matchCount}</p>
                          <p>Total: {testResult.totalCount}</p>
                          <p>
                            Percentage:{' '}
                            {testResult.totalCount > 0
                              ? Math.round((testResult.matchCount / testResult.totalCount) * 100)
                              : 0}
                            %
                          </p>
                        </div>
                      ) : (
                        <p>Not yet tested - waiting for valid expression</p>
                      )}
                    </div>
                  </TooltipContent>
                </Tooltip>
              );
            })}
        </div>
      </div>
    </TooltipProvider>
  );
}
