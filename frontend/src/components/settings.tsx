'use client';

import { useState, useEffect } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Alert, AlertDescription } from '@/components/ui/alert';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  RefreshCw,
  Save,
  Settings as SettingsIcon,
  CheckCircle,
  AlertCircle,
  XCircle,
  Shield,
  Activity,
} from 'lucide-react';
import { RuntimeSettings, UpdateSettingsRequest, SettingsResponse } from '@/types/api';
import { apiClient } from '@/lib/api-client';
import { FeatureFlagsEditor } from '@/components/feature-flags-editor';
import { useFeatureFlags, invalidateFeatureFlagsCache } from '@/hooks/useFeatureFlags';
import { getBackendUrl } from '@/lib/config';

// Feature flag interface (should match the one in FeatureFlagsEditor)
interface FeatureFlag {
  key: string;
  enabled: boolean;
  config: Record<string, any>;
}

// Standard Rust tracing log levels
const LOG_LEVELS = [
  { value: 'TRACE', label: 'TRACE', description: 'Most verbose, includes all details' },
  { value: 'DEBUG', label: 'DEBUG', description: 'Debugging information' },
  { value: 'INFO', label: 'INFO', description: 'General information (default)' },
  { value: 'WARN', label: 'WARN', description: 'Warning messages' },
  { value: 'ERROR', label: 'ERROR', description: 'Error messages only' },
] as const;

function getStatusIcon(success: boolean) {
  return success ? (
    <CheckCircle className="h-4 w-4 text-green-500" />
  ) : (
    <XCircle className="h-4 w-4 text-destructive" />
  );
}

export function Settings() {
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saveSuccess, setSaveSuccess] = useState<string | null>(null);
  const [settings, setSettings] = useState<RuntimeSettings | null>(null);
  const [editedSettings, setEditedSettings] = useState<Partial<RuntimeSettings>>({});

  // Feature flags state
  const [flags, setFlags] = useState<FeatureFlag[]>([]);
  const [flagsLoaded, setFlagsLoaded] = useState(false);
  const { refetch } = useFeatureFlags();

  // Circuit breaker state
  const [circuitBreakerConfig, setCircuitBreakerConfig] = useState<any>(null);
  const [editedCbConfig, setEditedCbConfig] = useState<any>({});
  const [cbLoading, setCbLoading] = useState(false);
  const [cbSaving, setCbSaving] = useState(false);

  const fetchSettings = async () => {
    setLoading(true);
    setError(null);
    setSaveSuccess(null);

    try {
      const response: SettingsResponse = await apiClient.getSettings();
      if (response.success && response.settings) {
        setSettings(response.settings);
        setEditedSettings({});
      } else {
        setError('No settings data received');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch settings');
    } finally {
      setLoading(false);
    }
  };

  const fetchFeatureFlags = async () => {
    try {
      const response = await apiClient.getFeatures();
      const featureFlags: FeatureFlag[] = [];

      // Convert flags and config into unified structure
      Object.entries(response.flags || {}).forEach(([key, enabled]) => {
        featureFlags.push({
          key,
          enabled: Boolean(enabled),
          config: response.config?.[key] || {},
        });
      });

      // Add any config-only features (features with config but no flag)
      Object.keys(response.config || {}).forEach((key) => {
        if (!featureFlags.find((f) => f.key === key)) {
          featureFlags.push({
            key,
            enabled: false,
            config: response.config[key] || {},
          });
        }
      });

      setFlags(featureFlags.sort((a, b) => a.key.localeCompare(b.key)));
      setFlagsLoaded(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch feature flags');
    }
  };

  const fetchCircuitBreakerData = async () => {
    setCbLoading(true);
    try {
      const backendUrl = getBackendUrl();
      const configResponse = await fetch(`${backendUrl}/api/v1/circuit-breakers/config`);

      if (configResponse.ok) {
        const configData = await configResponse.json();
        const config = configData.data?.config || null;
        setCircuitBreakerConfig(config);
        setEditedCbConfig({}); // Reset edited changes when fetching fresh data
      }
    } catch (err) {
      console.warn('Circuit breaker config endpoint not available:', err);
      // Don't set error since circuit breakers might not be configured
    } finally {
      setCbLoading(false);
    }
  };

  const fetchAll = async () => {
    setLoading(true);
    setError(null);
    setSaveSuccess(null);

    try {
      await Promise.all([fetchSettings(), fetchFeatureFlags(), fetchCircuitBreakerData()]);
    } catch (err) {
      // Error handling is done in individual functions
    } finally {
      setLoading(false);
    }
  };

  const saveSettings = async (): Promise<string> => {
    if (!settings || Object.keys(editedSettings).length === 0) {
      return 'No settings changes to save';
    }

    try {
      const updateRequest: UpdateSettingsRequest = editedSettings;
      const response: SettingsResponse = await apiClient.updateSettings(updateRequest);

      if (response.success) {
        setSettings(response.settings);
        setEditedSettings({});
        return (
          response.message +
          (response.applied_changes.length > 0
            ? ` Applied changes: ${response.applied_changes.join(', ')}`
            : '')
        );
      } else {
        throw new Error('Failed to save settings');
      }
    } catch (err) {
      throw new Error(err instanceof Error ? err.message : 'Failed to save settings');
    }
  };

  const saveFeatureFlags = async (): Promise<string> => {
    try {
      const flagsData = flags.reduce(
        (acc, flag) => {
          acc[flag.key] = flag.enabled;
          return acc;
        },
        {} as Record<string, boolean>
      );

      const configData = flags.reduce(
        (acc, flag) => {
          if (Object.keys(flag.config).length > 0) {
            acc[flag.key] = flag.config;
          }
          return acc;
        },
        {} as Record<string, Record<string, any>>
      );

      await apiClient.updateFeatures({
        flags: flagsData,
        config: configData,
      });

      // Invalidate cache and refresh the feature flags context
      invalidateFeatureFlagsCache();
      await refetch();

      return 'Feature flags updated successfully';
    } catch (err) {
      throw new Error(err instanceof Error ? err.message : 'Failed to save feature flags');
    }
  };

  const saveCircuitBreakerConfig = async (): Promise<string> => {
    if (!circuitBreakerConfig || Object.keys(editedCbConfig).length === 0) {
      return 'No circuit breaker changes to save';
    }

    try {
      // Build the updated config by merging original with edited changes
      const updatedConfig = {
        global: {
          ...circuitBreakerConfig.global,
          ...editedCbConfig.global,
        },
        profiles: {
          ...circuitBreakerConfig.profiles,
          ...editedCbConfig.profiles,
        },
      };

      const backendUrl = getBackendUrl();
      const response = await fetch(`${backendUrl}/api/v1/circuit-breakers/config`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ config: updatedConfig }),
      });

      if (!response.ok) {
        const errorData = await response.json();
        throw new Error(errorData.error || 'Failed to update circuit breaker configuration');
      }

      const result = await response.json();

      // Update local state with the new config
      setCircuitBreakerConfig(updatedConfig);
      setEditedCbConfig({});

      return `Circuit breaker configuration updated successfully. Updated ${result.data.updated_count} services.`;
    } catch (err) {
      throw new Error(
        err instanceof Error ? err.message : 'Failed to save circuit breaker configuration'
      );
    }
  };

  const saveAll = async () => {
    setSaving(true);
    setError(null);
    setSaveSuccess(null);

    const settingsHasChanges = settings && Object.keys(editedSettings).length > 0;
    const featureFlagsChanged = flagsLoaded; // Assume flags might have changed if loaded
    const circuitBreakerChanged = circuitBreakerConfig && Object.keys(editedCbConfig).length > 0;

    if (!settingsHasChanges && !featureFlagsChanged && !circuitBreakerChanged) {
      return;
    }

    try {
      const results: string[] = [];

      if (settingsHasChanges) {
        const settingsResult = await saveSettings();
        results.push(settingsResult);
      }

      if (featureFlagsChanged) {
        const flagsResult = await saveFeatureFlags();
        results.push(flagsResult);
      }

      if (circuitBreakerChanged) {
        const cbResult = await saveCircuitBreakerConfig();
        results.push(cbResult);
      }

      setSaveSuccess(results.join('. '));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save changes');
    } finally {
      setSaving(false);
    }
  };

  const handleInputChange = (key: keyof RuntimeSettings, value: any) => {
    if (settings && value === settings[key]) {
      // Value is back to original, remove from edited settings
      setEditedSettings((prev) => {
        const newSettings = { ...prev };
        delete newSettings[key];
        return newSettings;
      });
    } else {
      // Value is different from original, add to edited settings
      setEditedSettings((prev) => ({
        ...prev,
        [key]: value,
      }));
    }
  };

  const getCurrentValue = (key: keyof RuntimeSettings) => {
    return editedSettings.hasOwnProperty(key) ? editedSettings[key] : settings?.[key];
  };

  const isModified = (key: keyof RuntimeSettings) => {
    return editedSettings.hasOwnProperty(key) && settings && editedSettings[key] !== settings[key];
  };

  // Circuit breaker change helpers
  const handleCbGlobalChange = (key: string, value: any) => {
    setEditedCbConfig((prev: any) => ({
      ...prev,
      global: {
        ...prev.global,
        [key]: value,
      },
    }));
  };

  const handleCbProfileChange = (serviceName: string, key: string, value: any) => {
    setEditedCbConfig((prev: any) => ({
      ...prev,
      profiles: {
        ...prev.profiles,
        [serviceName]: {
          ...circuitBreakerConfig?.profiles?.[serviceName],
          ...prev.profiles?.[serviceName],
          [key]: value,
        },
      },
    }));
  };

  const getCbGlobalValue = (key: string) => {
    return editedCbConfig.global?.[key] ?? circuitBreakerConfig?.global?.[key];
  };

  const getCbProfileValue = (serviceName: string, key: string) => {
    return (
      editedCbConfig.profiles?.[serviceName]?.[key] ??
      circuitBreakerConfig?.profiles?.[serviceName]?.[key]
    );
  };

  const isCbGlobalModified = (key: string) => {
    return editedCbConfig.global?.[key] !== undefined;
  };

  const isCbProfileModified = (serviceName: string, key: string) => {
    return editedCbConfig.profiles?.[serviceName]?.[key] !== undefined;
  };

  const hasSettingsChanges = Object.keys(editedSettings).length > 0;
  const hasCbChanges = Object.keys(editedCbConfig).length > 0;
  const hasAnyChanges = hasSettingsChanges || flagsLoaded || hasCbChanges; // Simplified - assume flags might have changes if loaded

  useEffect(() => {
    fetchAll();
  }, []);

  return (
    <div className="space-y-4">
      {/* Header with controls */}
      <div className="flex justify-between items-center">
        <div>
          <p className="text-sm text-muted-foreground">
            Runtime application settings that can be changed without restart
          </p>
        </div>
        <div className="flex gap-2">
          <Button onClick={fetchAll} disabled={loading} size="sm" variant="outline">
            <RefreshCw className={`h-4 w-4 mr-2 ${loading ? 'animate-spin' : ''}`} />
            Refresh All
          </Button>
          <Button onClick={saveAll} disabled={saving || !hasAnyChanges} size="sm">
            <Save className={`h-4 w-4 mr-2 ${saving ? 'animate-spin' : ''}`} />
            Save All Changes
          </Button>
        </div>
      </div>

      {/* Status Messages */}
      {error && (
        <Card className="border-destructive">
          <CardContent className="pt-6">
            <div className="flex items-center gap-2 text-destructive">
              <XCircle className="h-4 w-4" />
              <span className="font-medium">Error:</span>
              <span>{error}</span>
            </div>
          </CardContent>
        </Card>
      )}

      {saveSuccess && (
        <Alert variant="success">
          <CheckCircle className="h-4 w-4" />
          <AlertDescription>
            <span className="font-medium">Success:</span> {saveSuccess}
          </AlertDescription>
        </Alert>
      )}

      {/* Feature Flags Management */}
      <FeatureFlagsEditor
        flags={flags}
        setFlags={setFlags}
        loading={loading}
        error={error}
        setError={setError}
        onRefresh={fetchFeatureFlags}
      />

      {/* Circuit Breaker Configuration */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Shield className="h-5 w-5" />
            Circuit Breaker Configuration
          </CardTitle>
          <CardDescription>
            Runtime circuit breaker settings that can be modified without restart
          </CardDescription>
        </CardHeader>
        <CardContent>
          {cbLoading ? (
            <div className="flex items-center gap-2">
              <RefreshCw className="h-4 w-4 animate-spin" />
              <span>Loading circuit breaker configuration...</span>
            </div>
          ) : circuitBreakerConfig ? (
            <div className="space-y-3">
              {/* Circuit Breaker Cards */}
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
                {/* Global Configuration Card */}
                {circuitBreakerConfig?.global && (
                  <Card className="h-fit">
                    <CardHeader className="pb-3">
                      <CardTitle className="text-sm flex items-center gap-2">
                        <SettingsIcon className="h-4 w-4" />
                        Global Defaults
                      </CardTitle>
                    </CardHeader>
                    <CardContent className="space-y-3">
                      <div className="text-xs text-muted-foreground">
                        Implementation:{' '}
                        <span className="font-medium capitalize">
                          {getCbGlobalValue('implementation_type')}
                        </span>
                      </div>

                      <div className="grid grid-cols-2 gap-2">
                        <div className="space-y-1">
                          <Label className="text-xs text-muted-foreground flex items-center gap-1">
                            Failures
                            {isCbGlobalModified('failure_threshold') && (
                              <Badge variant="secondary" className="text-xs">
                                *
                              </Badge>
                            )}
                          </Label>
                          <Input
                            type="number"
                            min="1"
                            max="100"
                            value={getCbGlobalValue('failure_threshold') || ''}
                            onChange={(e) =>
                              handleCbGlobalChange('failure_threshold', parseInt(e.target.value))
                            }
                            className="h-7 text-xs"
                          />
                        </div>

                        <div className="space-y-1">
                          <Label className="text-xs text-muted-foreground flex items-center gap-1">
                            Success
                            {isCbGlobalModified('success_threshold') && (
                              <Badge variant="secondary" className="text-xs">
                                *
                              </Badge>
                            )}
                          </Label>
                          <Input
                            type="number"
                            min="1"
                            max="100"
                            value={getCbGlobalValue('success_threshold') || ''}
                            onChange={(e) =>
                              handleCbGlobalChange('success_threshold', parseInt(e.target.value))
                            }
                            className="h-7 text-xs"
                          />
                        </div>

                        <div className="space-y-1">
                          <Label className="text-xs text-muted-foreground flex items-center gap-1">
                            Op Timeout
                            {isCbGlobalModified('operation_timeout') && (
                              <Badge variant="secondary" className="text-xs">
                                *
                              </Badge>
                            )}
                          </Label>
                          <Input
                            type="text"
                            value={getCbGlobalValue('operation_timeout') || ''}
                            onChange={(e) =>
                              handleCbGlobalChange('operation_timeout', e.target.value)
                            }
                            placeholder="5s"
                            className="h-7 text-xs"
                          />
                        </div>

                        <div className="space-y-1">
                          <Label className="text-xs text-muted-foreground flex items-center gap-1">
                            Reset
                            {isCbGlobalModified('reset_timeout') && (
                              <Badge variant="secondary" className="text-xs">
                                *
                              </Badge>
                            )}
                          </Label>
                          <Input
                            type="text"
                            value={getCbGlobalValue('reset_timeout') || ''}
                            onChange={(e) => handleCbGlobalChange('reset_timeout', e.target.value)}
                            placeholder="30s"
                            className="h-7 text-xs"
                          />
                        </div>
                      </div>
                    </CardContent>
                  </Card>
                )}

                {/* Service-Specific Profile Cards */}
                {circuitBreakerConfig?.profiles &&
                  Object.entries(circuitBreakerConfig.profiles).map(
                    ([serviceName, profile]: [string, any]) => (
                      <Card key={serviceName} className="h-fit">
                        <CardHeader className="pb-3">
                          <CardTitle className="text-sm flex items-center gap-2">
                            <Activity className="h-4 w-4" />
                            {serviceName}
                          </CardTitle>
                        </CardHeader>
                        <CardContent className="space-y-3">
                          <div className="text-xs text-muted-foreground">
                            Implementation:{' '}
                            <span className="font-medium capitalize">
                              {getCbProfileValue(serviceName, 'implementation_type')}
                            </span>
                          </div>

                          <div className="grid grid-cols-2 gap-2">
                            <div className="space-y-1">
                              <Label className="text-xs text-muted-foreground flex items-center gap-1">
                                Failures
                                {isCbProfileModified(serviceName, 'failure_threshold') && (
                                  <Badge variant="secondary" className="text-xs">
                                    *
                                  </Badge>
                                )}
                              </Label>
                              <Input
                                type="number"
                                min="1"
                                max="100"
                                value={getCbProfileValue(serviceName, 'failure_threshold') || ''}
                                onChange={(e) =>
                                  handleCbProfileChange(
                                    serviceName,
                                    'failure_threshold',
                                    parseInt(e.target.value)
                                  )
                                }
                                className="h-7 text-xs"
                              />
                            </div>

                            <div className="space-y-1">
                              <Label className="text-xs text-muted-foreground flex items-center gap-1">
                                Success
                                {isCbProfileModified(serviceName, 'success_threshold') && (
                                  <Badge variant="secondary" className="text-xs">
                                    *
                                  </Badge>
                                )}
                              </Label>
                              <Input
                                type="number"
                                min="1"
                                max="100"
                                value={getCbProfileValue(serviceName, 'success_threshold') || ''}
                                onChange={(e) =>
                                  handleCbProfileChange(
                                    serviceName,
                                    'success_threshold',
                                    parseInt(e.target.value)
                                  )
                                }
                                className="h-7 text-xs"
                              />
                            </div>

                            <div className="space-y-1">
                              <Label className="text-xs text-muted-foreground flex items-center gap-1">
                                Op Timeout
                                {isCbProfileModified(serviceName, 'operation_timeout') && (
                                  <Badge variant="secondary" className="text-xs">
                                    *
                                  </Badge>
                                )}
                              </Label>
                              <Input
                                type="text"
                                value={getCbProfileValue(serviceName, 'operation_timeout') || ''}
                                onChange={(e) =>
                                  handleCbProfileChange(
                                    serviceName,
                                    'operation_timeout',
                                    e.target.value
                                  )
                                }
                                placeholder="5s"
                                className="h-7 text-xs"
                              />
                            </div>

                            <div className="space-y-1">
                              <Label className="text-xs text-muted-foreground flex items-center gap-1">
                                Reset
                                {isCbProfileModified(serviceName, 'reset_timeout') && (
                                  <Badge variant="secondary" className="text-xs">
                                    *
                                  </Badge>
                                )}
                              </Label>
                              <Input
                                type="text"
                                value={getCbProfileValue(serviceName, 'reset_timeout') || ''}
                                onChange={(e) =>
                                  handleCbProfileChange(
                                    serviceName,
                                    'reset_timeout',
                                    e.target.value
                                  )
                                }
                                placeholder="30s"
                                className="h-7 text-xs"
                              />
                            </div>
                          </div>
                        </CardContent>
                      </Card>
                    )
                  )}
              </div>

              <div className="text-xs text-muted-foreground p-2 bg-muted/50 rounded text-center">
                Changes apply immediately and persist to config. Check debug page for statistics.
              </div>
            </div>
          ) : (
            <div className="text-center py-8 text-muted-foreground">
              <Shield className="h-12 w-12 mx-auto mb-3 opacity-50" />
              <p>Circuit breaker functionality is not configured</p>
              <p className="text-sm mt-1">
                Configure circuit breakers in your application config to see them here
              </p>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Settings Table */}
      {settings && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <SettingsIcon className="h-5 w-5" />
              Runtime Settings
            </CardTitle>
            <CardDescription>
              Modify application settings that take effect immediately
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {/* Log Level */}
              <div className="space-y-2">
                <Label className="text-sm font-medium flex items-center gap-2">
                  Log Level
                  {isModified('log_level') && (
                    <Badge variant="secondary" className="text-xs">
                      *
                    </Badge>
                  )}
                </Label>
                <Select
                  value={String(getCurrentValue('log_level') || 'INFO')}
                  onValueChange={(value) => handleInputChange('log_level', value)}
                >
                  <SelectTrigger className="h-8 text-sm">
                    <SelectValue placeholder="Select level" />
                  </SelectTrigger>
                  <SelectContent>
                    {LOG_LEVELS.map((level) => (
                      <SelectItem key={level.value} value={level.value}>
                        <div className="flex flex-col text-left">
                          <span className="font-medium text-sm">{level.label}</span>
                          <span className="text-xs text-muted-foreground">{level.description}</span>
                        </div>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              {/* Request Logging */}
              <div className="space-y-2">
                <Label className="text-sm font-medium flex items-center gap-2">
                  Request Logging
                  {isModified('enable_request_logging') && (
                    <Badge variant="secondary" className="text-xs">
                      *
                    </Badge>
                  )}
                </Label>
                <div className="flex items-center gap-2 h-8">
                  <input
                    id="enable_request_logging"
                    type="checkbox"
                    checked={Boolean(getCurrentValue('enable_request_logging'))}
                    onChange={(e) => handleInputChange('enable_request_logging', e.target.checked)}
                    className="rounded border-gray-300"
                  />
                  <Label htmlFor="enable_request_logging" className="text-sm text-muted-foreground">
                    Enable HTTP request logs
                  </Label>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Raw Settings Data (for debugging) */}
      {settings && (
        <Card>
          <CardHeader>
            <CardTitle>Raw Settings Data</CardTitle>
            <CardDescription>Current settings as returned by the API</CardDescription>
          </CardHeader>
          <CardContent>
            <pre className="bg-muted p-3 rounded text-xs overflow-auto">
              {JSON.stringify(settings, null, 2)}
            </pre>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
