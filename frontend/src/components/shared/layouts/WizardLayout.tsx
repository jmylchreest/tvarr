'use client';

import React, { useState, useEffect } from 'react';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { ChevronLeft, ChevronRight, Check, Loader2 } from 'lucide-react';

export interface WizardStep {
  id: string;
  title: string;
  description?: string;
  icon?: React.ReactNode;
  isValid?: boolean;
  isOptional?: boolean;
}

export interface WizardLayoutProps {
  steps: WizardStep[];
  currentStep: number;
  onStepChange: (step: number) => void;
  onComplete: () => void;
  children: React.ReactNode;
  isLoading?: boolean;
  canNavigateBack?: boolean;
  canNavigateForward?: boolean;
  completeLabel?: string;
  nextLabel?: string;
  backLabel?: string;
  showStepNumbers?: boolean;
  /** Compact mode - show only step circles without titles for narrow containers */
  compact?: boolean;
  className?: string;
}

/**
 * WizardLayout provides a multi-step wizard interface with:
 * - Step indicator showing progress
 * - Navigation buttons (back/next/complete)
 * - Support for step validation
 * - Optional steps support
 */
export function WizardLayout({
  steps,
  currentStep,
  onStepChange,
  onComplete,
  children,
  isLoading = false,
  canNavigateBack = true,
  canNavigateForward = true,
  completeLabel = 'Complete',
  nextLabel = 'Next',
  backLabel = 'Back',
  showStepNumbers = true,
  compact = false,
  className,
}: WizardLayoutProps) {
  const isFirstStep = currentStep === 0;
  const isLastStep = currentStep === steps.length - 1;
  const currentStepData = steps[currentStep];

  const handleBack = () => {
    if (currentStep > 0 && canNavigateBack) {
      onStepChange(currentStep - 1);
    }
  };

  const handleNext = () => {
    if (currentStep < steps.length - 1 && canNavigateForward) {
      onStepChange(currentStep + 1);
    }
  };

  const handleStepClick = (stepIndex: number) => {
    // Allow clicking on previous steps or if current step is valid
    if (stepIndex < currentStep || (stepIndex <= currentStep + 1 && canNavigateForward)) {
      onStepChange(stepIndex);
    }
  };

  return (
    <div className={cn('flex flex-col h-full', className)}>
      {/* Step Indicator */}
      <div className={cn(
        'border-b bg-muted/30',
        compact ? 'px-4 py-2' : 'px-6 py-4'
      )}>
        <div className={cn(
          'flex items-center justify-between',
          compact ? 'justify-center gap-2' : 'max-w-3xl mx-auto'
        )}>
          {steps.map((step, index) => {
            const isCompleted = index < currentStep;
            const isCurrent = index === currentStep;
            const isClickable = index < currentStep || (index <= currentStep + 1 && canNavigateForward);

            return (
              <React.Fragment key={step.id}>
                {/* Step Circle */}
                <button
                  type="button"
                  onClick={() => handleStepClick(index)}
                  disabled={!isClickable}
                  className={cn(
                    'flex items-center gap-2 transition-colors',
                    isClickable && 'cursor-pointer hover:text-primary',
                    !isClickable && 'cursor-not-allowed opacity-50'
                  )}
                >
                  <div
                    className={cn(
                      'flex items-center justify-center rounded-full border-2 transition-colors',
                      compact ? 'w-7 h-7' : 'w-8 h-8',
                      isCompleted && 'bg-primary border-primary text-primary-foreground',
                      isCurrent && 'border-primary text-primary',
                      !isCompleted && !isCurrent && 'border-muted-foreground/30 text-muted-foreground'
                    )}
                  >
                    {isCompleted ? (
                      <Check className={cn(compact ? 'h-3.5 w-3.5' : 'h-4 w-4')} />
                    ) : showStepNumbers ? (
                      <span className={cn('font-medium', compact ? 'text-xs' : 'text-sm')}>{index + 1}</span>
                    ) : step.icon ? (
                      step.icon
                    ) : (
                      <span className={cn('font-medium', compact ? 'text-xs' : 'text-sm')}>{index + 1}</span>
                    )}
                  </div>
                  {/* Hide titles in compact mode */}
                  {!compact && (
                    <div className="hidden sm:block text-left">
                      <div
                        className={cn(
                          'text-sm font-medium',
                          isCurrent && 'text-foreground',
                          !isCurrent && 'text-muted-foreground'
                        )}
                      >
                        {step.title}
                      </div>
                      {step.isOptional && (
                        <div className="text-xs text-muted-foreground">Optional</div>
                      )}
                    </div>
                  )}
                </button>

                {/* Connector Line */}
                {index < steps.length - 1 && (
                  <div
                    className={cn(
                      'h-0.5',
                      compact ? 'w-6' : 'flex-1 mx-2',
                      index < currentStep ? 'bg-primary' : 'bg-muted-foreground/30'
                    )}
                  />
                )}
              </React.Fragment>
            );
          })}
        </div>
      </div>

      {/* Step Content */}
      <div className={cn('flex-1 overflow-y-auto', compact ? 'p-4' : 'p-6')}>
        <div className={cn(!compact && 'max-w-3xl mx-auto')}>
          {/* Step Header */}
          {currentStepData && (
            <div className={cn(compact ? 'mb-4' : 'mb-6')}>
              <h2 className={cn(compact ? 'text-base font-semibold' : 'text-xl font-semibold')}>
                {currentStepData.title}
              </h2>
              {currentStepData.description && (
                <p className="text-sm text-muted-foreground mt-1">
                  {currentStepData.description}
                </p>
              )}
            </div>
          )}

          {/* Step Content */}
          {children}
        </div>
      </div>

      {/* Navigation Footer */}
      <div className={cn('border-t bg-background', compact ? 'px-4 py-3' : 'px-6 py-4')}>
        <div className={cn('flex items-center justify-between', !compact && 'max-w-3xl mx-auto')}>
          <Button
            type="button"
            variant="outline"
            size={compact ? 'sm' : 'default'}
            onClick={handleBack}
            disabled={isFirstStep || isLoading}
          >
            <ChevronLeft className="h-4 w-4 mr-1" />
            {backLabel}
          </Button>

          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            Step {currentStep + 1} of {steps.length}
          </div>

          {isLastStep ? (
            <Button
              type="button"
              size={compact ? 'sm' : 'default'}
              onClick={onComplete}
              disabled={isLoading || (currentStepData?.isValid === false)}
            >
              {isLoading && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
              {completeLabel}
            </Button>
          ) : (
            <Button
              type="button"
              size={compact ? 'sm' : 'default'}
              onClick={handleNext}
              disabled={!canNavigateForward || isLoading}
            >
              {nextLabel}
              <ChevronRight className="h-4 w-4 ml-1" />
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}

/**
 * WizardStepContent - Wrapper for individual step content
 * Helps with consistent spacing and layout within wizard steps
 */
export function WizardStepContent({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return <div className={cn('space-y-6', className)}>{children}</div>;
}

/**
 * WizardStepSection - Section within a wizard step
 * Use for grouping related form fields
 */
export function WizardStepSection({
  title,
  description,
  children,
  className,
}: {
  title?: string;
  description?: string;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <div className={cn('space-y-4', className)}>
      {(title || description) && (
        <div className="space-y-1">
          {title && <h3 className="text-base font-medium">{title}</h3>}
          {description && <p className="text-sm text-muted-foreground">{description}</p>}
        </div>
      )}
      {children}
    </div>
  );
}
