'use client';

import { useState, useCallback, useRef } from 'react';
import { TooltipProvider } from '@/components/ui/tooltip';
import { ExpressionEditor } from '@/components/expression-editor';
import { AutocompletePopup } from '@/components/autocomplete-popup';
import { ValidationBadges } from '@/components/expression-validation-badges';
import { useHelperAutocomplete } from '@/hooks/useHelperAutocomplete';
import { getValidationState, getCategoryValidationState } from '@/lib/expression-utils';
import { cn } from '@/lib/utils';
import { CLIENT_DETECTION_HELPERS } from '@/lib/expression-constants';
import type {
  ExpressionValidationResponse,
  ExpressionField,
} from '@/types/api';

export interface ClientDetectionExpressionEditorProps {
  value: string;
  onChange: (value: string) => void;
  className?: string;
  placeholder?: string;
  disabled?: boolean;
  showValidationBadges?: boolean;
}

export function ClientDetectionExpressionEditor({
  value,
  onChange,
  className,
  placeholder,
  disabled = false,
  showValidationBadges = true,
}: ClientDetectionExpressionEditorProps) {
  const [validation, setValidation] = useState<ExpressionValidationResponse | null>(null);
  const [fields, setFields] = useState<ExpressionField[]>([]);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // Initialize helper autocomplete hook with client detection helpers
  const { state: autocompleteState, handlers: autocompleteHandlers } = useHelperAutocomplete(
    textareaRef as React.RefObject<HTMLTextAreaElement>,
    CLIENT_DETECTION_HELPERS,
    onChange
  );

  // Stable callback functions
  const handleValidationChange = useCallback((newValidation: ExpressionValidationResponse | null) => {
    setValidation(newValidation);
  }, []);

  const handleFieldsChange = useCallback((newFields: ExpressionField[]) => {
    setFields(newFields);
  }, []);

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
            validationEndpoint="/expressions/validate?domain=client_detection"
            fieldsEndpoint="/client-detection/fields"
            sourceType="client_detection"
            placeholder={
              placeholder ||
              'Enter client detection expression (e.g., user_agent contains "Chrome")'
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
        {showValidationBadges && (
          <ValidationBadges
            validation={validationState}
            categoryStates={categoryStates}
            fields={fields}
            sourceType="client_detection"
            helpers={CLIENT_DETECTION_HELPERS}
          />
        )}
      </div>
    </TooltipProvider>
  );
}
