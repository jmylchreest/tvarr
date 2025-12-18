'use client';

import { useState } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import {
  Database,
  Calendar,
  Play,
  ArrowRight,
  CheckCircle2,
  Circle,
  Sparkles,
} from 'lucide-react';
import Link from 'next/link';

interface SetupStep {
  id: string;
  title: string;
  description: string;
  icon: React.ReactNode;
  href: string;
  isComplete: boolean;
}

interface SetupWizardProps {
  hasStreamSources: boolean;
  hasEpgSources: boolean;
  hasProxies: boolean;
  onDismiss?: () => void;
}

export function SetupWizard({
  hasStreamSources,
  hasEpgSources,
  hasProxies,
  onDismiss,
}: SetupWizardProps) {
  const [isDismissed, setIsDismissed] = useState(false);

  const steps: SetupStep[] = [
    {
      id: 'stream-sources',
      title: 'Add Stream Source',
      description: 'Import your M3U playlist or connect to an IPTV provider',
      icon: <Database className="h-5 w-5" />,
      href: '/sources/stream',
      isComplete: hasStreamSources,
    },
    {
      id: 'epg-sources',
      title: 'Add EPG Source',
      description: 'Import XMLTV program guide for channel metadata',
      icon: <Calendar className="h-5 w-5" />,
      href: '/sources/epg',
      isComplete: hasEpgSources,
    },
    {
      id: 'proxies',
      title: 'Create Proxy',
      description: 'Combine sources and filters into a streaming proxy',
      icon: <Play className="h-5 w-5" />,
      href: '/proxies',
      isComplete: hasProxies,
    },
  ];

  const completedSteps = steps.filter((s) => s.isComplete).length;
  const allComplete = completedSteps === steps.length;
  const currentStepIndex = steps.findIndex((s) => !s.isComplete);
  const currentStep = currentStepIndex >= 0 ? steps[currentStepIndex] : null;

  // Don't show if all steps are complete or dismissed
  if (allComplete || isDismissed) {
    return null;
  }

  const handleDismiss = () => {
    setIsDismissed(true);
    onDismiss?.();
  };

  return (
    <Card className="border-primary/20 bg-gradient-to-br from-primary/5 to-transparent">
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-2">
            <Sparkles className="h-5 w-5 text-primary" />
            <CardTitle className="text-lg">Welcome to tvarr</CardTitle>
          </div>
          <Button variant="ghost" size="sm" onClick={handleDismiss}>
            Dismiss
          </Button>
        </div>
        <CardDescription>
          Complete the setup steps below to start streaming
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Progress indicator */}
        <div className="flex items-center gap-2 mb-4">
          <div className="flex-1 h-2 bg-muted rounded-full overflow-hidden">
            <div
              className="h-full bg-primary transition-all duration-500"
              style={{ width: `${(completedSteps / steps.length) * 100}%` }}
            />
          </div>
          <Badge variant="secondary" className="text-xs">
            {completedSteps}/{steps.length}
          </Badge>
        </div>

        {/* Steps */}
        <div className="space-y-3">
          {steps.map((step, index) => {
            const isActive = index === currentStepIndex;
            const isPast = step.isComplete;
            const isFuture = !step.isComplete && index > currentStepIndex;

            return (
              <div
                key={step.id}
                className={`flex items-start gap-3 p-3 rounded-lg transition-colors ${
                  isActive
                    ? 'bg-primary/10 border border-primary/20'
                    : isPast
                      ? 'bg-muted/50'
                      : 'opacity-50'
                }`}
              >
                {/* Step indicator */}
                <div
                  className={`flex-shrink-0 mt-0.5 ${
                    isPast
                      ? 'text-green-500'
                      : isActive
                        ? 'text-primary'
                        : 'text-muted-foreground'
                  }`}
                >
                  {isPast ? (
                    <CheckCircle2 className="h-5 w-5" />
                  ) : (
                    <Circle className="h-5 w-5" />
                  )}
                </div>

                {/* Step content */}
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span
                      className={`font-medium ${isPast ? 'text-muted-foreground line-through' : ''}`}
                    >
                      {step.title}
                    </span>
                    {isPast && (
                      <Badge variant="outline" className="text-xs text-green-600 border-green-600">
                        Done
                      </Badge>
                    )}
                  </div>
                  <p className="text-sm text-muted-foreground">{step.description}</p>
                </div>

                {/* Action button */}
                {isActive && (
                  <Link href={step.href}>
                    <Button size="sm" className="flex-shrink-0">
                      {step.icon}
                      <span className="ml-1">Start</span>
                      <ArrowRight className="h-4 w-4 ml-1" />
                    </Button>
                  </Link>
                )}
                {isPast && (
                  <Link href={step.href}>
                    <Button size="sm" variant="ghost" className="flex-shrink-0">
                      View
                    </Button>
                  </Link>
                )}
              </div>
            );
          })}
        </div>

        {/* Quick tip */}
        {currentStep && (
          <div className="mt-4 p-3 bg-muted/50 rounded-lg">
            <p className="text-sm text-muted-foreground">
              <strong className="text-foreground">Next up:</strong> {currentStep.description}
            </p>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
