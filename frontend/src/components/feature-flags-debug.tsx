'use client';

import React from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { useFeatureFlags } from '@/hooks/useFeatureFlags';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { RefreshCw, Flag, Check, X } from 'lucide-react';

/**
 * Feature flags debug component that shows all current feature flags and their values
 */
export function FeatureFlagsDebug() {
  const { featureFlags, featureConfigs, isLoading, error } = useFeatureFlags();

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Flag className="w-5 h-5" />
            Feature Flags
          </CardTitle>
          <CardDescription>Current feature flag configuration</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-center py-8">
            <RefreshCw className="w-6 h-6 animate-spin mr-2" />
            <span>Loading feature flags...</span>
          </div>
        </CardContent>
      </Card>
    );
  }

  if (error) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Flag className="w-5 h-5" />
            Feature Flags
          </CardTitle>
          <CardDescription>Current feature flag configuration</CardDescription>
        </CardHeader>
        <CardContent>
          <Alert variant="destructive">
            <AlertDescription>Failed to load feature flags: {error}</AlertDescription>
          </Alert>
        </CardContent>
      </Card>
    );
  }

  const flagEntries = Object.entries(featureFlags);
  const enabledFlags = flagEntries.filter(([_, enabled]) => enabled);
  const disabledFlags = flagEntries.filter(([_, enabled]) => !enabled);
  const configEntries = Object.entries(featureConfigs);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Flag className="h-5 w-5" />
          Feature Flags
        </CardTitle>
        <CardDescription>Runtime feature toggles and configuration</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {flagEntries.length === 0 && configEntries.length === 0 ? (
          <div className="text-center py-6 text-muted-foreground">
            <Flag className="h-8 w-8 mx-auto mb-2 opacity-50" />
            <p className="text-sm">No feature flags found</p>
          </div>
        ) : (
          <div className="space-y-3">
            {/* Feature flags as horizontal bars */}
            {flagEntries.map(([key, enabled]) => {
              const config = configEntries.find(([configKey]) => configKey === key)?.[1];
              return (
                <div key={key} className="bg-muted/50 rounded p-2">
                  <div className="flex justify-between items-center mb-1">
                    <div className="font-medium text-sm font-mono truncate pr-2">{key}</div>
                    <Badge variant={enabled ? 'default' : 'secondary'} className="text-xs">
                      {enabled ? (
                        <>
                          <Check className="w-3 h-3 mr-1" />
                          enabled
                        </>
                      ) : (
                        <>
                          <X className="w-3 h-3 mr-1" />
                          disabled
                        </>
                      )}
                    </Badge>
                  </div>

                  {/* Configuration details if present */}
                  {config && Object.keys(config).length > 0 && (
                    <div className="mt-2 pt-1 border-t border-border/50">
                      <div className="grid gap-1 text-xs">
                        {Object.entries(config).map(([configKey, configValue]) => (
                          <div key={configKey} className="flex justify-between items-center">
                            <span className="text-muted-foreground font-mono">{configKey}:</span>
                            <span className="font-medium font-mono">
                              {typeof configValue === 'string'
                                ? `"${configValue}"`
                                : JSON.stringify(configValue)}
                            </span>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
