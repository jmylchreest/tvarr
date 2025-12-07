'use client';

/**
 * Shared validation badge components for expression editors.
 */

import { Badge } from '@/components/ui/badge';
import { Code } from '@/components/ui/code';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import {
  CheckCircle,
  XCircle,
  AlertCircle,
  Loader2,
  Settings,
  ArrowRight,
} from 'lucide-react';
import { truncateString } from '@/lib/expression-utils';
import type { ExpressionField, ExpressionValidationError } from '@/types/api';
import type { SourceTestResult } from '@/hooks/useExpressionSourceTesting';
import type { Helper } from '@/lib/expression-constants';
import {
  EXPRESSION_OPERATORS,
  EXPRESSION_MODIFIERS,
  VALID_OPERATOR_NAMES,
  VALID_MODIFIER_NAMES,
} from '@/lib/expression-constants';

/**
 * Props for validation state badges.
 */
interface ValidationBadgeProps {
  isValid: boolean | null;
  errors?: ExpressionValidationError[];
  className?: string;
}

/**
 * Base validation badge with consistent styling.
 */
function BaseBadge({
  isValid,
  icon,
  label,
  className,
  children,
}: {
  isValid: boolean | null;
  icon: React.ReactNode;
  label: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge
          className={cn(
            'gap-1 bg-muted',
            isValid === true && 'bg-green-500 hover:bg-green-600 text-white border-transparent',
            isValid === false && 'bg-red-500 hover:bg-red-600 text-white border-transparent',
            className
          )}
        >
          {icon}
          {label}
        </Badge>
      </TooltipTrigger>
      <TooltipContent>{children}</TooltipContent>
    </Tooltip>
  );
}

/**
 * Icon component based on validation state.
 */
function ValidationIcon({
  isValid,
  defaultIcon,
}: {
  isValid: boolean | null;
  defaultIcon?: React.ReactNode;
}) {
  if (isValid === true) return <CheckCircle className="h-3 w-3" />;
  if (isValid === false) return <XCircle className="h-3 w-3" />;
  return defaultIcon || <AlertCircle className="h-3 w-3" />;
}

/**
 * Expression validation status badge.
 */
export function ExpressionBadge({
  isValid,
  errors,
  errorMessage,
  className,
}: ValidationBadgeProps & { errorMessage?: string }) {
  const displayMessage = errorMessage || errors?.[0]?.message;

  return (
    <BaseBadge
      isValid={isValid}
      icon={<ValidationIcon isValid={isValid} />}
      label="Expression"
      className={className}
    >
      <div className="space-y-1">
        <p className="font-medium">Expression Validation</p>
        {isValid === true && <p>Valid expression</p>}
        {isValid === false && displayMessage && <p className="text-destructive">{displayMessage}</p>}
        {isValid === null && <p>Enter expression to validate</p>}
      </div>
    </BaseBadge>
  );
}

/**
 * Syntax validation badge.
 */
export function SyntaxBadge({
  isValid,
  errors,
  validMessage = 'Syntax is valid',
  className,
}: ValidationBadgeProps & { validMessage?: string }) {
  return (
    <BaseBadge
      isValid={isValid}
      icon={<ValidationIcon isValid={isValid} defaultIcon={<Code className="h-3 w-3" />} />}
      label="Syntax"
      className={className}
    >
      <div className="space-y-2 max-w-xs">
        <p className="font-medium">Expression Syntax</p>
        {errors && errors.length > 0 ? (
          <div className="space-y-1">
            {errors.map((error, idx) => (
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
            {isValid === true ? validMessage : 'Enter expression to validate syntax'}
          </p>
        )}
      </div>
    </BaseBadge>
  );
}

/**
 * Fields validation badge with field reference.
 */
export function FieldsBadge({
  isValid,
  errors,
  fields,
  sourceType,
  className,
}: ValidationBadgeProps & {
  fields: ExpressionField[];
  sourceType: string;
}) {
  return (
    <BaseBadge
      isValid={isValid}
      icon={<ValidationIcon isValid={isValid} defaultIcon={<Code className="h-3 w-3" />} />}
      label="Fields"
      className={className}
    >
      <div className="space-y-2 max-w-xs">
        <p className="font-medium">Available {sourceType} Fields</p>
        {errors && errors.length > 0 && (
          <div className="space-y-1 mb-2">
            {errors.map((error, idx) => (
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
    </BaseBadge>
  );
}

/**
 * Operators validation badge with reference.
 */
export function OperatorsBadge({ isValid, errors, className }: ValidationBadgeProps) {
  return (
    <BaseBadge
      isValid={isValid}
      icon={<ValidationIcon isValid={isValid} defaultIcon={<Settings className="h-3 w-3" />} />}
      label="Operators"
      className={className}
    >
      <div className="space-y-2 max-w-xs">
        <p className="font-medium">Available Operators</p>
        {errors && errors.length > 0 && (
          <div className="space-y-1 mb-2">
            {errors.map((error, idx) => (
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
              {VALID_OPERATOR_NAMES.map((op) => (
                <Code key={op} variant="muted" size="sm">
                  {op}
                </Code>
              ))}
            </div>
          </div>
          <div>
            <p className="text-sm font-medium">Modifiers:</p>
            <div className="flex flex-wrap gap-1">
              {VALID_MODIFIER_NAMES.map((mod) => (
                <Code key={mod} variant="muted" size="sm">
                  {mod}
                </Code>
              ))}
            </div>
          </div>
        </div>
      </div>
    </BaseBadge>
  );
}

/**
 * Values validation badge.
 */
export function ValuesBadge({
  isValid,
  errors,
  helpers,
  className,
}: ValidationBadgeProps & { helpers?: Helper[] }) {
  return (
    <BaseBadge
      isValid={isValid}
      icon={<ValidationIcon isValid={isValid} defaultIcon={<Code className="h-3 w-3" />} />}
      label="Values"
      className={className}
    >
      <div className="space-y-2 max-w-xs">
        <p className="font-medium">{helpers ? 'Available Helpers' : 'Value Validation'}</p>
        {errors && errors.length > 0 ? (
          <div className="space-y-1">
            {errors.map((error, idx) => (
              <div key={idx} className="text-xs">
                <p className="font-medium text-destructive">{error.message}</p>
                {error.suggestion && (
                  <p className="text-muted-foreground text-xs">{error.suggestion}</p>
                )}
              </div>
            ))}
          </div>
        ) : helpers ? (
          <div className="flex flex-wrap gap-1">
            {helpers.map((helper) => (
              <Code key={helper.name} variant="muted" size="sm">
                {helper.prefix}
              </Code>
            ))}
            {helpers.length === 0 && <p className="text-xs">Loading helpers...</p>}
          </div>
        ) : (
          <p className="text-xs text-muted-foreground">
            {isValid === true ? 'Values are valid' : 'Enter expression to validate values'}
          </p>
        )}
      </div>
    </BaseBadge>
  );
}

/**
 * Source test badge props.
 */
interface SourceTestBadgeProps {
  sourceId: string;
  sourceName: string;
  result?: SourceTestResult;
  variant: 'filter' | 'data-mapping';
  className?: string;
}

/**
 * Source test result badge.
 */
export function SourceTestBadge({
  sourceId,
  sourceName,
  result,
  variant,
  className,
}: SourceTestBadgeProps) {
  const displayName = truncateString(sourceName, 8);
  const isLoading = result?.loading;
  const hasError = result?.error;
  const isTestedSuccessfully = result && !result.loading && !result.error;
  const isUntested = !result;

  // Determine display text
  let displayText = '-/-';
  if (isLoading) {
    displayText = variant === 'filter' ? 'Testing...' : '...';
  } else if (hasError) {
    displayText = 'Error';
  } else if (isTestedSuccessfully) {
    if (variant === 'filter') {
      displayText = `${result.matchCount || 0}/${result.totalCount || 0}`;
    } else {
      displayText = `${result.preview?.affected_channels || 0}/${result.preview?.total_channels || 0}`;
    }
  }

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge
          className={cn(
            'gap-1 bg-muted',
            isLoading && variant === 'data-mapping' && 'bg-yellow-500 hover:bg-yellow-600 text-white border-transparent',
            isTestedSuccessfully && variant === 'filter' && 'bg-green-500 hover:bg-green-600 text-white border-transparent',
            isTestedSuccessfully && variant === 'data-mapping' && 'bg-blue-500 hover:bg-blue-600 text-white border-transparent',
            hasError && 'bg-red-500 hover:bg-red-600 text-white border-transparent',
            className
          )}
        >
          {isLoading ? (
            <Loader2 className="h-3 w-3 animate-spin" />
          ) : hasError ? (
            <XCircle className="h-3 w-3" />
          ) : isTestedSuccessfully ? (
            variant === 'filter' ? <CheckCircle className="h-3 w-3" /> : <ArrowRight className="h-3 w-3" />
          ) : (
            <AlertCircle className="h-3 w-3" />
          )}
          {displayName}: {displayText}
        </Badge>
      </TooltipTrigger>
      <TooltipContent>
        <div className="space-y-1 max-w-sm">
          <p className="font-medium">{sourceName}</p>
          {isLoading && (
            <p>{variant === 'filter' ? 'Testing filter against source...' : 'Generating preview...'}</p>
          )}
          {hasError ? (
            <p>Error: {result.error}</p>
          ) : isTestedSuccessfully ? (
            <div>
              {variant === 'filter' ? (
                <>
                  <p>Matched: {result.matchCount}</p>
                  <p>Total: {result.totalCount}</p>
                  <p>
                    Percentage:{' '}
                    {result.totalCount && result.totalCount > 0
                      ? Math.round((result.matchCount! / result.totalCount) * 100)
                      : 0}
                    %
                  </p>
                </>
              ) : (
                <>
                  {result.preview?.success ? (
                    <p>
                      Success:{' '}
                      {result.preview?.message ||
                        `Applied to ${result.preview.total_channels || 0} channels`}
                    </p>
                  ) : (
                    <p>Error: {result.preview?.message || 'Unknown error'}</p>
                  )}
                </>
              )}
            </div>
          ) : (
            <p>Not yet tested - waiting for valid expression</p>
          )}
        </div>
      </TooltipContent>
    </Tooltip>
  );
}

/**
 * Props for the validation badges bar.
 */
export interface ValidationBadgesProps {
  validation: {
    isValid: boolean | null;
    error?: string | null;
  };
  categoryStates: {
    syntax: { isValid: boolean | null; errors: ExpressionValidationError[] };
    field: { isValid: boolean | null; errors: ExpressionValidationError[] };
    operator: { isValid: boolean | null; errors: ExpressionValidationError[] };
    value: { isValid: boolean | null; errors: ExpressionValidationError[] };
  };
  fields: ExpressionField[];
  sourceType: string;
  helpers?: Helper[];
  sources?: Array<{ id: string; name: string }>;
  sourceResults?: Map<string, SourceTestResult>;
  testVariant?: 'filter' | 'data-mapping';
  className?: string;
}

/**
 * Complete validation badges bar component.
 */
export function ValidationBadges({
  validation,
  categoryStates,
  fields,
  sourceType,
  helpers,
  sources,
  sourceResults,
  testVariant = 'filter',
  className,
}: ValidationBadgesProps) {
  return (
    <div className={cn('flex flex-wrap gap-2', className)}>
      <ExpressionBadge
        isValid={validation.isValid}
        errorMessage={validation.error || undefined}
      />

      <SyntaxBadge
        isValid={categoryStates.syntax.isValid}
        errors={categoryStates.syntax.errors}
      />

      <FieldsBadge
        isValid={categoryStates.field.isValid}
        errors={categoryStates.field.errors}
        fields={fields}
        sourceType={sourceType}
      />

      <OperatorsBadge
        isValid={categoryStates.operator.isValid}
        errors={categoryStates.operator.errors}
      />

      <ValuesBadge
        isValid={categoryStates.value.isValid}
        errors={categoryStates.value.errors}
        helpers={helpers}
      />

      {sources?.map((source) => (
        <SourceTestBadge
          key={source.id}
          sourceId={source.id}
          sourceName={source.name || `Source ${source.id}`}
          result={sourceResults?.get(source.id)}
          variant={testVariant}
        />
      ))}
    </div>
  );
}
