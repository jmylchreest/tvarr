'use client';

import { useState, useCallback } from 'react';
import { TooltipProvider } from '@/components/ui/tooltip';
import { ExpressionEditor } from '@/components/expression-editor';
import { ValidationBadges } from '@/components/expression-validation-badges';
import { useExpressionSourceTesting } from '@/hooks/useExpressionSourceTesting';
import { getValidationState, getCategoryValidationState } from '@/lib/expression-utils';
import { cn } from '@/lib/utils';
import type {
  ExpressionValidationResponse,
  ExpressionField,
  FilterSourceType,
} from '@/types/api';

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

  // Use shared source testing hook
  const {
    sources,
    sourceResults,
    handleValidationComplete: handleSourceValidationComplete,
  } = useExpressionSourceTesting({
    expression: value,
    sourceType,
    enabled: showTestResults,
    autoTest,
    testEndpoint: 'filter',
    validation,
  });

  // Stable callback functions to prevent infinite re-renders
  const handleValidationChange = useCallback((newValidation: ExpressionValidationResponse | null) => {
    setValidation(newValidation);
  }, []);

  const handleFieldsChange = useCallback((newFields: ExpressionField[]) => {
    setFields(newFields);
  }, []);

  // Handle validation completion - trigger source testing
  const handleValidationComplete = useCallback(() => {
    handleSourceValidationComplete();
  }, [handleSourceValidationComplete]);

  // Get validation states for display
  const validationState = getValidationState(validation);
  const categoryStates = {
    syntax: getCategoryValidationState(validation, 'syntax'),
    field: getCategoryValidationState(validation, 'field'),
    operator: getCategoryValidationState(validation, 'operator'),
    value: getCategoryValidationState(validation, 'value'),
  };

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
        <ValidationBadges
          validation={validationState}
          categoryStates={categoryStates}
          fields={fields}
          sourceType={sourceType}
          sources={sources.map(s => ({ id: s.id, name: s.name || `Source ${s.id}` }))}
          sourceResults={sourceResults}
          testVariant="filter"
        />
      </div>
    </TooltipProvider>
  );
}
