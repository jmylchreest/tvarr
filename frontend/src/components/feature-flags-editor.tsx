'use client';

import { useState, useEffect } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import { Switch } from '@/components/ui/switch';
// Alert components now handled by parent
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Plus, Trash2, Flag, Settings } from 'lucide-react';
import { Debug } from '@/utils/debug';

interface FeatureFlag {
  key: string;
  enabled: boolean;
  config: Record<string, any>;
}

interface FeatureConfigValue {
  key: string;
  value: string;
  type: 'string' | 'number' | 'boolean';
}

interface FeatureFlagsEditorProps {
  flags: FeatureFlag[];
  setFlags: (flags: FeatureFlag[] | ((prev: FeatureFlag[]) => FeatureFlag[])) => void;
  loading: boolean;
  error: string | null;
  setError: (error: string | null) => void;
  onRefresh: () => Promise<void>;
}

export function FeatureFlagsEditor({
  flags,
  setFlags,
  loading,
  error,
  setError,
  onRefresh,
}: FeatureFlagsEditorProps) {
  const [showAddFlag, setShowAddFlag] = useState(false);
  const [showEditConfig, setShowEditConfig] = useState<string | null>(null);
  const [newFlagKey, setNewFlagKey] = useState('');
  const [configValues, setConfigValues] = useState<FeatureConfigValue[]>([]);

  const debug = Debug.createLogger('FeatureFlagsEditor');

  // Feature flags operations are now handled by parent component

  const toggleFlag = (key: string, enabled: boolean) => {
    setFlags((prev) => prev.map((flag) => (flag.key === key ? { ...flag, enabled } : flag)));
  };

  const deleteFlag = (key: string) => {
    setFlags((prev) => prev.filter((flag) => flag.key !== key));
  };

  const validateFlagKey = (key: string): string | null => {
    if (!key.trim()) return 'Flag key cannot be empty';
    if (!/^[a-z0-9-]+$/.test(key))
      return 'Flag key must contain only lowercase letters, numbers, and hyphens';
    if (key.startsWith('-') || key.endsWith('-'))
      return 'Flag key cannot start or end with a hyphen';
    if (key.includes('--')) return 'Flag key cannot contain consecutive hyphens';
    if (key.length > 50) return 'Flag key must be 50 characters or less';
    return null;
  };

  const addFlag = () => {
    const trimmedKey = newFlagKey.trim();

    const validationError = validateFlagKey(trimmedKey);
    if (validationError) {
      setError(validationError);
      return;
    }

    const exists = flags.find((f) => f.key === trimmedKey);
    if (exists) {
      setError(`Feature flag '${trimmedKey}' already exists`);
      return;
    }

    setFlags((prev) =>
      [
        ...prev,
        {
          key: trimmedKey,
          enabled: false,
          config: {},
        },
      ].sort((a, b) => a.key.localeCompare(b.key))
    );

    setNewFlagKey('');
    setShowAddFlag(false);
    setError(null);
  };

  const openConfigEditor = (key: string) => {
    const flag = flags.find((f) => f.key === key);
    if (!flag) return;

    const values: FeatureConfigValue[] = Object.entries(flag.config).map(([k, v]) => ({
      key: k,
      value: String(v),
      type: typeof v === 'boolean' ? 'boolean' : typeof v === 'number' ? 'number' : 'string',
    }));

    setConfigValues(values);
    setShowEditConfig(key);
  };

  const addConfigValue = () => {
    setConfigValues((prev) => [
      ...prev,
      {
        key: '',
        value: '',
        type: 'string',
      },
    ]);
  };

  const updateConfigValue = (index: number, field: keyof FeatureConfigValue, value: string) => {
    setConfigValues((prev) =>
      prev.map((item, i) => (i === index ? { ...item, [field]: value } : item))
    );
  };

  const removeConfigValue = (index: number) => {
    setConfigValues((prev) => prev.filter((_, i) => i !== index));
  };

  const validateConfigKey = (key: string): string | null => {
    if (!key.trim()) return 'Config key cannot be empty';
    if (!/^[a-zA-Z0-9_-]+$/.test(key))
      return 'Config key must contain only letters, numbers, underscores, and hyphens';
    if (key.length > 50) return 'Config key must be 50 characters or less';
    return null;
  };

  const saveConfig = () => {
    if (!showEditConfig) return;

    // Validate all config keys first
    const errors: string[] = [];
    const duplicateKeys = new Set<string>();
    const seenKeys = new Set<string>();

    configValues.forEach(({ key, value, type }, index) => {
      if (key.trim()) {
        const validationError = validateConfigKey(key);
        if (validationError) {
          errors.push(`Row ${index + 1}: ${validationError}`);
        }

        if (seenKeys.has(key.trim())) {
          duplicateKeys.add(key.trim());
        }
        seenKeys.add(key.trim());

        // Validate boolean values
        if (
          type === 'boolean' &&
          value &&
          value.trim() &&
          !['true', 'false'].includes(value.toLowerCase())
        ) {
          errors.push(`Row ${index + 1}: Boolean value must be 'true' or 'false'`);
        }

        // Validate number values
        if (type === 'number' && value && value.trim() && isNaN(Number(value))) {
          errors.push(`Row ${index + 1}: Invalid number value`);
        }
      }
    });

    if (duplicateKeys.size > 0) {
      errors.push(`Duplicate config keys: ${Array.from(duplicateKeys).join(', ')}`);
    }

    if (errors.length > 0) {
      setError(errors.join('\n'));
      return;
    }

    const config: Record<string, any> = {};
    configValues.forEach(({ key, value, type }) => {
      if (key && key.trim()) {
        if (type === 'boolean') {
          config[key] = value && value.toLowerCase() === 'true';
        } else if (type === 'number') {
          const num = Number(value || 0);
          config[key] = isNaN(num) ? value || '' : num;
        } else {
          config[key] = value || '';
        }
      }
    });

    setFlags((prev) =>
      prev.map((flag) => (flag.key === showEditConfig ? { ...flag, config } : flag))
    );

    setShowEditConfig(null);
    setConfigValues([]);
    setError(null);
  };

  const formatConfigValue = (value: any): string => {
    if (typeof value === 'boolean') return value ? 'true' : 'false';
    if (typeof value === 'number') return String(value);
    return String(value);
  };

  // Initial fetch is handled by parent component

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Flag className="h-5 w-5" />
          Feature Flags Management
        </CardTitle>
        <CardDescription>
          Configure feature flags and their associated configuration values
        </CardDescription>
      </CardHeader>
      <CardContent>
        {/* Controls */}
        <div className="flex justify-start items-center mb-6">
          <Dialog open={showAddFlag} onOpenChange={setShowAddFlag}>
            <DialogTrigger asChild>
              <Button size="sm">
                <Plus className="h-4 w-4 mr-2" />
                Add Flag
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Add Feature Flag</DialogTitle>
                <DialogDescription>
                  Create a new feature flag. Use kebab-case naming (e.g., debug-frontend,
                  advanced-player).
                </DialogDescription>
              </DialogHeader>
              <div className="space-y-4">
                <div>
                  <Label htmlFor="flag-key">Flag Key</Label>
                  <Input
                    id="flag-key"
                    value={newFlagKey}
                    onChange={(e) => {
                      setNewFlagKey(e.target.value);
                      setError(null); // Clear error on input change
                    }}
                    placeholder="e.g., new-feature"
                    onKeyDown={(e) => e.key === 'Enter' && addFlag()}
                    pattern="[a-z0-9-]+"
                  />
                </div>
              </div>
              <DialogFooter>
                <Button variant="outline" onClick={() => setShowAddFlag(false)}>
                  Cancel
                </Button>
                <Button onClick={addFlag} disabled={!newFlagKey.trim()}>
                  Add Flag
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </div>

        {/* Status Messages - now handled by parent component */}

        {/* Feature Flags Table */}
        {flags.length > 0 ? (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Flag Key</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Configuration</TableHead>
                <TableHead>Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {flags.map((flag) => (
                <TableRow key={flag.key}>
                  <TableCell className="font-medium">{flag.key}</TableCell>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      <Switch
                        checked={flag.enabled}
                        onCheckedChange={(enabled) => toggleFlag(flag.key, enabled)}
                      />
                      <Badge variant={flag.enabled ? 'default' : 'secondary'}>
                        {flag.enabled ? 'Enabled' : 'Disabled'}
                      </Badge>
                    </div>
                  </TableCell>
                  <TableCell>
                    {Object.keys(flag.config).length > 0 ? (
                      <div className="space-y-1">
                        {Object.entries(flag.config)
                          .slice(0, 2)
                          .map(([key, value]) => (
                            <div key={key} className="text-xs">
                              <span className="font-mono text-muted-foreground">{key}:</span>{' '}
                              <span className="font-mono">{formatConfigValue(value)}</span>
                            </div>
                          ))}
                        {Object.keys(flag.config).length > 2 && (
                          <div className="text-xs text-muted-foreground">
                            +{Object.keys(flag.config).length - 2} more...
                          </div>
                        )}
                      </div>
                    ) : (
                      <span className="text-muted-foreground text-sm">No configuration</span>
                    )}
                  </TableCell>
                  <TableCell>
                    <div className="flex gap-1">
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => openConfigEditor(flag.key)}
                      >
                        <Settings className="h-3 w-3" />
                      </Button>
                      <Button size="sm" variant="outline" onClick={() => deleteFlag(flag.key)}>
                        <Trash2 className="h-3 w-3" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        ) : (
          <div className="text-center py-8 text-muted-foreground">
            {loading ? 'Loading feature flags...' : 'No feature flags configured'}
          </div>
        )}

        {/* Config Editor Dialog */}
        <Dialog open={!!showEditConfig} onOpenChange={(open) => !open && setShowEditConfig(null)}>
          <DialogContent className="max-w-2xl">
            <DialogHeader>
              <DialogTitle>Configure Feature: {showEditConfig}</DialogTitle>
              <DialogDescription>
                Add configuration key-value pairs for this feature flag
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4 max-h-96 overflow-y-auto">
              {configValues.map((item, index) => (
                <div key={index} className="flex gap-2 items-end">
                  <div className="flex-1">
                    <Label>Key</Label>
                    <Input
                      value={item.key}
                      onChange={(e) => updateConfigValue(index, 'key', e.target.value)}
                      placeholder="config-key"
                    />
                  </div>
                  <div className="flex-1">
                    <Label>Value</Label>
                    <Input
                      value={item.value}
                      onChange={(e) => updateConfigValue(index, 'value', e.target.value)}
                      placeholder="value"
                    />
                  </div>
                  <div className="w-24">
                    <Label>Type</Label>
                    <select
                      className="w-full h-9 px-3 rounded-md border border-input bg-background text-sm"
                      value={item.type}
                      onChange={(e) => updateConfigValue(index, 'type', e.target.value)}
                    >
                      <option value="string">String</option>
                      <option value="number">Number</option>
                      <option value="boolean">Boolean</option>
                    </select>
                  </div>
                  <Button size="sm" variant="outline" onClick={() => removeConfigValue(index)}>
                    <Trash2 className="h-3 w-3" />
                  </Button>
                </div>
              ))}
              <Button variant="outline" size="sm" onClick={addConfigValue} className="w-full">
                <Plus className="h-4 w-4 mr-2" />
                Add Configuration
              </Button>
            </div>
            <DialogFooter>
              <Button variant="outline" onClick={() => setShowEditConfig(null)}>
                Cancel
              </Button>
              <Button onClick={saveConfig}>Save Configuration</Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </CardContent>
    </Card>
  );
}
