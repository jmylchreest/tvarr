'use client';

import { useState, useCallback, useRef, useEffect } from 'react';
import { TooltipProvider } from '@/components/ui/tooltip';
import { ExpressionEditor } from '@/components/expression-editor';
import { AutocompletePopup } from '@/components/autocomplete-popup';
import { ValidationBadges } from '@/components/expression-validation-badges';
import { useExpressionSourceTesting } from '@/hooks/useExpressionSourceTesting';
import { useHelperAutocomplete, type Helper } from '@/hooks/useHelperAutocomplete';
import { getValidationState, getCategoryValidationState } from '@/lib/expression-utils';
import { getBackendUrl } from '@/lib/config';
import { Debug } from '@/utils/debug';
import { cn } from '@/lib/utils';
import type {
  ExpressionValidationResponse,
  ExpressionField,
  DataMappingSourceType,
} from '@/types/api';

export interface DataMappingExpressionEditorProps {
  value: string;
  onChange: (value: string) => void;
  sourceType: DataMappingSourceType;
  className?: string;
  placeholder?: string;
  disabled?: boolean;
  showTestResults?: boolean;
  autoTest?: boolean;
}

export function DataMappingExpressionEditor({
  value,
  onChange,
  sourceType,
  className,
  placeholder,
  disabled = false,
  showTestResults = true,
  autoTest = true,
}: DataMappingExpressionEditorProps) {
  const [validation, setValidation] = useState<ExpressionValidationResponse | null>(null);
  const [fields, setFields] = useState<ExpressionField[]>([]);
  const [helpers, setHelpers] = useState<Helper[]>([]);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // Initialize helper autocomplete hook
  const { state: autocompleteState, handlers: autocompleteHandlers } = useHelperAutocomplete(
    textareaRef as React.RefObject<HTMLTextAreaElement>,
    helpers,
    onChange
  );

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
    testEndpoint: 'data-mapping',
    validation,
  });

  // Fetch helpers once on mount
  useEffect(() => {
    const fetchHelpers = async () => {
      try {
        const backendUrl = getBackendUrl();
        const response = await fetch(`${backendUrl}/api/v1/data-mapping/helpers`);

        if (response.ok) {
          const data = await response.json();
          Debug.log('DataMapping: Helpers API response', data);
          if (data.helpers && Array.isArray(data.helpers)) {
            setHelpers(data.helpers);
          }
        }
      } catch (error) {
        console.warn('Failed to fetch data mapping helpers:', error);
      }
    };

    fetchHelpers();
  }, []);

  // Stable callback functions
  const handleValidationChange = useCallback((newValidation: ExpressionValidationResponse | null) => {
    setValidation(newValidation);
  }, []);

  const handleFieldsChange = useCallback((newFields: ExpressionField[]) => {
    setFields(newFields);
  }, []);

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
        {/* Expression Editor with Helper Autocomplete */}
        <div className="relative">
          <ExpressionEditor
            ref={textareaRef}
            value={value}
            onChange={onChange}
            onValidationChange={handleValidationChange}
            onFieldsChange={handleFieldsChange}
            onValidationComplete={handleValidationComplete}
            validationEndpoint={`/expressions/validate?domain=${sourceType === 'epg' ? 'epg_mapping' : 'stream_mapping'}`}
            fieldsEndpoint={`/data-mapping/fields/${sourceType}`}
            sourceType={sourceType}
            placeholder={
              placeholder ||
              'Enter data transformation expression (e.g., channel_name = "New " + channel_name)'
            }
            disabled={disabled}
            onKeyDown={autocompleteHandlers.onKeyDown}
            onInput={autocompleteHandlers.onInputChange}
          />

          {/* Helper Autocomplete Popup */}
          <AutocompletePopup
            state={autocompleteState}
            onSuggestionClick={autocompleteHandlers.onSuggestionClick}
          />
        </div>

        {/* Status Badges */}
        <ValidationBadges
          validation={validationState}
          categoryStates={categoryStates}
          fields={fields}
          sourceType={sourceType}
          helpers={helpers}
          sources={sources.map(s => ({ id: s.id, name: s.name || `Source ${s.id}` }))}
          sourceResults={sourceResults}
          testVariant="data-mapping"
        />
      </div>
    </TooltipProvider>
  );
}
