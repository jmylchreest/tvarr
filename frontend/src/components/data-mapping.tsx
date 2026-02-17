'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { StatCard } from '@/components/shared/feedback/StatCard';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { DataMappingExpressionEditor } from '@/components/data-mapping-expression-editor';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import {
  Plus,
  Trash2,
  ArrowUpDown,
  AlertCircle,
  CheckCircle,
  Loader2,
  WifiOff,
  Code,
  Play,
  ArrowUp,
  ArrowDown,
  Lock,
} from 'lucide-react';
import { DataMappingRule, DataMappingSourceType } from '@/types/api';
import { apiClient, ApiError } from '@/lib/api-client';
import { createFuzzyFilter } from '@/lib/fuzzy-search';
import { API_CONFIG } from '@/lib/config';
import { ExportDialog, ImportDialog } from '@/components/config-export';
import {
  MasterDetailLayout,
  DetailPanel,
  DetailEmpty,
  MasterItem,
} from '@/components/shared';

interface LoadingState {
  rules: boolean;
  create: boolean;
  edit: boolean;
  delete: string | null;
  reorder: boolean;
  toggle: string | null;
}

interface ErrorState {
  rules: string | null;
  create: string | null;
  edit: string | null;
  action: string | null;
}

// Convert DataMappingRule to MasterItem format for MasterDetailLayout
interface DataMappingRuleMasterItem extends MasterItem {
  rule: DataMappingRule;
}

function dataMappingRuleToMasterItem(rule: DataMappingRule): DataMappingRuleMasterItem {
  // No badges in master list - keep it clean
  return {
    id: rule.id,
    title: rule.name,
    enabled: rule.is_enabled,
    rule,
  };
}

// Create panel for creating a new data mapping rule inline in detail area
function DataMappingRuleCreatePanel({
  onCreate,
  onCancel,
  loading,
  error,
}: {
  onCreate: (rule: Omit<DataMappingRule, 'id' | 'created_at' | 'updated_at'>) => Promise<void>;
  onCancel: () => void;
  loading: boolean;
  error: string | null;
}) {
  const [formData, setFormData] = useState<
    Omit<DataMappingRule, 'id' | 'created_at' | 'updated_at'>
  >({
    name: '',
    source_type: 'stream',
    expression: '',
    description: '',
    is_enabled: true,
    priority: 0,
    stop_on_match: false,
  });
  const [mappingExpression, setMappingExpression] = useState('');

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await onCreate(formData);
    } catch {
      // Error handled by parent
    }
  };

  const isValid = formData.name.trim().length > 0;

  return (
    <DetailPanel
      title="Create Data Mapping Rule"
      actions={
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={onCancel} disabled={loading}>
            Cancel
          </Button>
          <Button size="sm" onClick={handleSubmit} disabled={loading || !isValid}>
            {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Create
          </Button>
        </div>
      }
    >
      <div className="space-y-6">
        {error && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="create-name">Rule Name</Label>
            <Input
              id="create-name"
              value={formData.name}
              onChange={(e) => setFormData({ ...formData, name: e.target.value })}
              placeholder="My Data Mapping Rule"
              required
              disabled={loading}
              autoFocus
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="create-source_type">Source Type</Label>
            <select
              id="create-source_type"
              value={formData.source_type}
              onChange={(e) =>
                setFormData({ ...formData, source_type: e.target.value as DataMappingSourceType })
              }
              className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
              required
              disabled={loading}
            >
              <option value="stream">Stream</option>
              <option value="epg">EPG</option>
            </select>
          </div>

          <div className="space-y-2">
            <Label htmlFor="create-description">Description (Optional)</Label>
            <Textarea
              id="create-description"
              value={formData.description || ''}
              onChange={(e) => setFormData({ ...formData, description: e.target.value })}
              placeholder="Describe what this data mapping rule does..."
              disabled={loading}
              rows={2}
            />
          </div>

          <DataMappingExpressionEditor
            value={mappingExpression}
            onChange={(value) => {
              setMappingExpression(value);
              setFormData({ ...formData, expression: value });
            }}
            sourceType={formData.source_type}
            placeholder='Enter transformation expression (e.g., channel_name = "HD " + channel_name)'
            disabled={loading}
            showTestResults={true}
            autoTest={true}
          />

          <div className="flex items-center space-x-2">
            <input
              id="create-is_enabled"
              type="checkbox"
              checked={formData.is_enabled}
              onChange={(e) => setFormData({ ...formData, is_enabled: e.target.checked })}
              className="rounded border-gray-300"
              disabled={loading}
            />
            <Label htmlFor="create-is_enabled">Active Rule</Label>
          </div>
        </form>
      </div>
    </DetailPanel>
  );
}

// Detail panel for viewing/editing a selected data mapping rule
function DataMappingRuleDetailPanel({
  rule,
  onUpdate,
  onDelete,
  onToggle,
  onMoveUp,
  onMoveDown,
  loading,
  error,
  isOnline,
  isFirst,
  isLast,
}: {
  rule: DataMappingRule;
  onUpdate: (id: string, data: Omit<DataMappingRule, 'id' | 'created_at' | 'updated_at'>, isSystem?: boolean) => Promise<void>;
  onDelete: (id: string) => Promise<void>;
  onToggle: (id: string, enabled: boolean) => Promise<void>;
  onMoveUp: (id: string) => Promise<void>;
  onMoveDown: (id: string) => Promise<void>;
  loading: { edit: boolean; delete: string | null; reorder: boolean; toggle: string | null };
  error: string | null;
  isOnline: boolean;
  isFirst: boolean;
  isLast: boolean;
}) {
  const [formData, setFormData] = useState<Omit<DataMappingRule, 'id' | 'created_at' | 'updated_at'>>({
    name: rule.name,
    source_type: rule.source_type,
    expression: rule.expression || '',
    description: rule.description || '',
    is_enabled: rule.is_enabled,
    priority: rule.priority,
    stop_on_match: rule.stop_on_match,
  });
  const [mappingExpression, setMappingExpression] = useState(rule.expression || '');
  const [hasChanges, setHasChanges] = useState(false);

  // Reset form when rule changes
  useEffect(() => {
    const newFormData = {
      name: rule.name,
      source_type: rule.source_type,
      expression: rule.expression || '',
      description: rule.description || '',
      is_enabled: rule.is_enabled,
      priority: rule.priority,
      stop_on_match: rule.stop_on_match,
    };
    setFormData(newFormData);
    setMappingExpression(rule.expression || '');
    setHasChanges(false);
  }, [rule.id]);

  const handleFieldChange = (field: keyof typeof formData, value: any) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
    setHasChanges(true);
  };

  const handleSave = async () => {
    await onUpdate(rule.id, formData, rule.is_system);
    setHasChanges(false);
  };

  const isSystem = rule.is_system;

  return (
    <DetailPanel
      title={rule.name}
      actions={
        <div className="flex items-center gap-1">
          {/* Enabled/Disabled Toggle - applies immediately */}
          <div className="flex items-center gap-1.5 mr-2 px-2 py-1 rounded-md bg-muted/50">
            <Switch
              id={`toggle-enabled-${rule.id}`}
              checked={rule.is_enabled}
              onCheckedChange={(checked) => onToggle(rule.id, checked)}
              className="h-4 w-7 data-[state=checked]:bg-primary data-[state=unchecked]:bg-input"
              disabled={loading.toggle === rule.id || !isOnline}
            />
            <label htmlFor={`toggle-enabled-${rule.id}`} className="text-xs text-muted-foreground cursor-pointer">
              {rule.is_enabled ? 'Enabled' : 'Disabled'}
            </label>
          </div>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => onMoveUp(rule.id)}
            disabled={isFirst || loading.reorder || !isOnline}
            title="Move up in priority"
          >
            <ArrowUp className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => onMoveDown(rule.id)}
            disabled={isLast || loading.reorder || !isOnline}
            title="Move down in priority"
          >
            <ArrowDown className="h-4 w-4" />
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => onDelete(rule.id)}
            disabled={loading.delete === rule.id || !isOnline || isSystem}
            className="text-destructive hover:text-destructive"
            title={isSystem ? "System rules cannot be deleted" : "Delete rule"}
          >
            {loading.delete === rule.id ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Trash2 className="h-4 w-4" />
            )}
          </Button>
        </div>
      }
    >
      <div className="space-y-6">
        {error && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {isSystem && (
          <Alert>
            <Lock className="h-4 w-4" />
            <AlertTitle>System Rule</AlertTitle>
            <AlertDescription>
              This is a system rule. You can enable/disable it and change its order, but cannot modify or delete it.
            </AlertDescription>
          </Alert>
        )}

        {/* Compact Rule Info */}
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <Badge variant="secondary">{rule.source_type.toUpperCase()}</Badge>
        </div>

        {/* Edit Form */}
        <div className="border-t pt-4 space-y-4">
          <h3 className="text-sm font-medium">Configuration</h3>

          <div className="space-y-2">
            <Label htmlFor="detail-name">Rule Name</Label>
            <Input
              id="detail-name"
              value={formData.name}
              onChange={(e) => handleFieldChange('name', e.target.value)}
              disabled={loading.edit || !isOnline || isSystem}
              autoComplete="off"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="detail-source_type">Source Type</Label>
            <select
              id="detail-source_type"
              value={formData.source_type}
              onChange={(e) => handleFieldChange('source_type', e.target.value as DataMappingSourceType)}
              className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
              disabled={loading.edit || !isOnline || isSystem}
            >
              <option value="stream">Stream</option>
              <option value="epg">EPG</option>
            </select>
          </div>

          <div className="space-y-2">
            <Label htmlFor="detail-description">Description (Optional)</Label>
            <Textarea
              id="detail-description"
              value={formData.description || ''}
              onChange={(e) => handleFieldChange('description', e.target.value)}
              placeholder="Describe what this data mapping rule does..."
              disabled={loading.edit || !isOnline || isSystem}
              rows={2}
            />
          </div>

          <DataMappingExpressionEditor
            key={rule.id}
            value={mappingExpression}
            onChange={(value) => {
              setMappingExpression(value);
              handleFieldChange('expression', value);
            }}
            sourceType={formData.source_type}
            placeholder='Enter transformation expression (e.g., channel_name = "HD " + channel_name)'
            disabled={loading.edit || !isOnline || isSystem}
            showTestResults={true}
            autoTest={true}
          />

          {/* Save Button */}
          {hasChanges && (
            <div className="flex justify-end pt-4 border-t">
              <Button
                onClick={handleSave}
                disabled={loading.edit || !isOnline}
              >
                {loading.edit && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Save Changes
              </Button>
            </div>
          )}
        </div>
      </div>
    </DetailPanel>
  );
}

export function DataMapping() {
  const [allRules, setAllRules] = useState<DataMappingRule[]>([]);

  const [loading, setLoading] = useState<LoadingState>({
    rules: false,
    create: false,
    edit: false,
    delete: null,
    reorder: false,
    toggle: null,
  });

  const [errors, setErrors] = useState<ErrorState>({
    rules: null,
    create: null,
    edit: null,
    action: null,
  });

  const [selectedRule, setSelectedRule] = useState<DataMappingRuleMasterItem | null>(null);
  const [isOnline, setIsOnline] = useState(true);
  const [isCreating, setIsCreating] = useState(false);

  // Sort rules by priority
  const sortedRules = useMemo(() => {
    return [...allRules].sort((a, b) => a.priority - b.priority);
  }, [allRules]);

  // Convert rules to master items for MasterDetailLayout
  const masterItems = useMemo(
    () => sortedRules.map(dataMappingRuleToMasterItem),
    [sortedRules]
  );

  const loadRules = useCallback(async () => {
    if (!isOnline) return;

    setLoading((prev) => ({ ...prev, rules: true }));
    setErrors((prev) => ({ ...prev, rules: null }));

    try {
      const response = await apiClient.getDataMappingRules();

      // Filter out any malformed rule objects
      const validRules = response.filter((rule) => {
        if (!rule) {
          console.warn('Invalid rule object - missing rule:', rule);
          return false;
        }

        if (!rule.id || typeof rule.id !== 'string') {
          console.warn('Invalid rule object - missing or invalid ID:', rule);
          return false;
        }

        return true;
      });

      if (validRules.length !== response.length) {
        console.warn(`Filtered out ${response.length - validRules.length} invalid rule(s)`);
      }

      setAllRules(validRules);
      setIsOnline(true);
    } catch (error) {
      const apiError = error as ApiError;
      if (apiError.status === 0) {
        setIsOnline(false);
        setErrors((prev) => ({
          ...prev,
          rules: `Unable to connect to the API service. Please check that the service is running at ${API_CONFIG.baseUrl}.`,
        }));
      } else {
        setErrors((prev) => ({
          ...prev,
          rules: `Failed to load data mapping rules: ${apiError.message}`,
        }));
      }
    } finally {
      setLoading((prev) => ({ ...prev, rules: false }));
    }
  }, [isOnline]);

  // Load rules on mount only
  useEffect(() => {
    loadRules();
  }, [loadRules]);

  const handleCreateRule = async (
    newRule: Omit<DataMappingRule, 'id' | 'created_at' | 'updated_at'>
  ) => {
    setLoading((prev) => ({ ...prev, create: true }));
    setErrors((prev) => ({ ...prev, create: null }));

    try {
      // Set priority to be the highest + 1
      const maxPriority = allRules.length > 0 ? Math.max(...allRules.map((r) => r.priority)) : 0;
      const ruleWithPriority = { ...newRule, priority: maxPriority + 1 };

      const response = await apiClient.createDataMappingRule(ruleWithPriority);
      await loadRules(); // Reload rules after creation
      setIsCreating(false);
      // Select the newly created rule
      if (response?.data?.id) {
        const newMasterItem = dataMappingRuleToMasterItem(response.data);
        setSelectedRule(newMasterItem);
      }
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        create: `Failed to create data mapping rule: ${apiError.message}`,
      }));
      throw error; // Re-throw to prevent dialog from closing
    } finally {
      setLoading((prev) => ({ ...prev, create: false }));
    }
  };

  const handleUpdateRule = async (
    id: string,
    ruleData: Omit<DataMappingRule, 'id' | 'created_at' | 'updated_at'>,
    isSystem?: boolean
  ) => {
    setLoading((prev) => ({ ...prev, edit: true }));
    setErrors((prev) => ({ ...prev, edit: null }));

    try {
      // System rules can only be toggled via PATCH
      if (isSystem) {
        await apiClient.patchDataMappingRule(id, ruleData.is_enabled);
      } else {
        await apiClient.updateDataMappingRule(id, ruleData);
      }
      await loadRules(); // Reload rules after update
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        edit: `Failed to update data mapping rule: ${apiError.message}`,
      }));
      throw error; // Re-throw to prevent dialog from closing
    } finally {
      setLoading((prev) => ({ ...prev, edit: false }));
    }
  };

  const handleDeleteRule = async (ruleId: string) => {
    if (
      !confirm(
        'Are you sure you want to delete this data mapping rule? This action cannot be undone.'
      )
    ) {
      return;
    }

    setLoading((prev) => ({ ...prev, delete: ruleId }));
    setErrors((prev) => ({ ...prev, action: null }));

    try {
      await apiClient.deleteDataMappingRule(ruleId);
      await loadRules(); // Reload rules after deletion
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        action: `Failed to delete data mapping rule: ${apiError.message}`,
      }));
    } finally {
      setLoading((prev) => ({ ...prev, delete: null }));
    }
  };

  const handleToggleRule = async (ruleId: string, enabled: boolean) => {
    setLoading((prev) => ({ ...prev, toggle: ruleId }));
    setErrors((prev) => ({ ...prev, action: null }));

    try {
      await apiClient.patchDataMappingRule(ruleId, enabled);
      await loadRules(); // Reload rules after toggle
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        action: `Failed to toggle data mapping rule: ${apiError.message}`,
      }));
    } finally {
      setLoading((prev) => ({ ...prev, toggle: null }));
    }
  };

  const moveRule = async (ruleId: string, direction: 'up' | 'down') => {
    const currentIndex = sortedRules.findIndex((r) => r.id === ruleId);
    if (currentIndex === -1) return;

    const targetIndex = direction === 'up' ? currentIndex - 1 : currentIndex + 1;
    if (targetIndex < 0 || targetIndex >= sortedRules.length) return;

    setLoading((prev) => ({ ...prev, reorder: true }));
    setErrors((prev) => ({ ...prev, action: null }));

    try {
      // Create new array with swapped items
      const newOrder = [...sortedRules];
      [newOrder[currentIndex], newOrder[targetIndex]] = [
        newOrder[targetIndex],
        newOrder[currentIndex],
      ];

      // Update priorities
      const reorderRequest = newOrder.map((rule, index) => ({
        id: rule.id,
        priority: index + 1,
      }));

      await apiClient.reorderDataMappingRules(reorderRequest);
      await loadRules(); // Reload to get updated order
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        action: `Failed to reorder rules: ${apiError.message}`,
      }));
    } finally {
      setLoading((prev) => ({ ...prev, reorder: false }));
    }
  };

  // Handle drag/drop reordering
  const handleDragReorder = async (reorderedIds: string[]) => {
    setLoading((prev) => ({ ...prev, reorder: true }));
    setErrors((prev) => ({ ...prev, action: null }));

    try {
      const reorderRequest = reorderedIds.map((id, index) => ({
        id,
        priority: index + 1,
      }));

      await apiClient.reorderDataMappingRules(reorderRequest);
      await loadRules();
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        action: `Failed to reorder rules: ${apiError.message}`,
      }));
    } finally {
      setLoading((prev) => ({ ...prev, reorder: false }));
    }
  };

  const streamRules = allRules?.filter((r) => r.source_type.toLowerCase() === 'stream').length || 0;
  const epgRules = allRules?.filter((r) => r.source_type.toLowerCase() === 'epg').length || 0;
  const activeRules = allRules?.filter((r) => r.is_enabled).length || 0;
  const totalRules = allRules?.length || 0;

  return (
    <div className="flex flex-col gap-6 h-full">
      {/* Header Section */}
      <div className="flex items-center justify-between">
        <div>
          <p className="text-muted-foreground">Manage data transformation rules</p>
        </div>
        <div className="flex items-center gap-2">
          {!isOnline && <WifiOff className="h-5 w-5 text-destructive" />}
          <ImportDialog
            importType="data_mapping_rules"
            title="Import Data Mapping Rules"
            onImportComplete={loadRules}
          />
          <ExportDialog
            exportType="data_mapping_rules"
            items={allRules.map((r) => ({ id: r.id, name: r.name, is_system: r.is_system }))}
            title="Export Data Mapping Rules"
          />
        </div>
      </div>

      {/* Connection Status Alert */}
      {!isOnline && (
        <Alert variant="destructive">
          <WifiOff className="h-4 w-4" />
          <AlertTitle>API Service Offline</AlertTitle>
          <AlertDescription>
            Unable to connect to the API service at {API_CONFIG.baseUrl}. Please ensure the service
            is running and try again.
            <Button
              variant="outline"
              size="sm"
              className="ml-2"
              onClick={() => window.location.reload()}
            >
              Retry
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {/* Action Error Alert */}
      {errors.action && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>
            {errors.action}
            <Button
              variant="outline"
              size="sm"
              className="ml-2"
              onClick={() => setErrors((prev) => ({ ...prev, action: null }))}
            >
              Dismiss
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {/* Statistics Cards */}
      <div className="grid gap-2 md:grid-cols-4">
        <StatCard title="Total Rules" value={totalRules} icon={<ArrowUpDown className="h-4 w-4" />} />
        <StatCard title="Stream Rules" value={streamRules} icon={<Play className="h-4 w-4 text-blue-600" />} />
        <StatCard title="EPG Rules" value={epgRules} icon={<Code className="h-4 w-4 text-green-600" />} />
        <StatCard title="Active Rules" value={activeRules} icon={<CheckCircle className="h-4 w-4 text-orange-600" />} />
      </div>

      {/* MasterDetailLayout */}
      <Card className="flex-1 overflow-hidden min-h-0">
        <CardContent className="p-0 h-full">
          {errors.rules ? (
            <div className="p-6">
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertTitle>Failed to Load Rules</AlertTitle>
                <AlertDescription>
                  {errors.rules}
                  <Button
                    variant="outline"
                    size="sm"
                    className="ml-2"
                    onClick={loadRules}
                    disabled={loading.rules}
                  >
                    {loading.rules && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                    Retry
                  </Button>
                </AlertDescription>
              </Alert>
            </div>
          ) : (
            <MasterDetailLayout
              items={masterItems}
              selectedId={selectedRule?.id}
              showDetailPanel={isCreating}
              onSelect={(item) => {
                setSelectedRule(item);
                if (item) setIsCreating(false);
              }}
              isLoading={loading.rules}
              title={`Data Mapping Rules (${sortedRules.length})`}
              searchPlaceholder="Search by name, type, expression..."
              storageKey="data-mapping"
              sortable={true}
              onReorder={handleDragReorder}
              headerAction={
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    setIsCreating(true);
                    setSelectedRule(null);
                    setErrors((prev) => ({ ...prev, create: null }));
                  }}
                  disabled={isCreating}
                >
                  <Plus className="h-4 w-4" />
                  <span className="sr-only">Create Rule</span>
                </Button>
              }
              emptyState={{
                title: 'No data mapping rules configured',
                description: 'Get started by creating your first data transformation rule.',
              }}
              filterFn={createFuzzyFilter<DataMappingRuleMasterItem>({
                keys: [
                  { name: 'name', weight: 0.35 },
                  { name: 'expression', weight: 0.2 },
                  { name: 'description', weight: 0.2 },
                  { name: 'source_type', weight: 0.1 },
                  { name: 'enabled', weight: 0.1 },
                  { name: 'system', weight: 0.05 },
                ],
                accessor: (item) => ({
                  name: item.rule.name,
                  expression: item.rule.expression || '',
                  description: item.rule.description || '',
                  source_type: item.rule.source_type,
                  enabled: item.rule.is_enabled ? 'enabled' : 'disabled',
                  system: item.rule.is_system ? 'system' : '',
                }),
              })}
            >
              {(selected) =>
                isCreating ? (
                  <DataMappingRuleCreatePanel
                    onCreate={handleCreateRule}
                    onCancel={() => setIsCreating(false)}
                    loading={loading.create}
                    error={errors.create}
                  />
                ) : selected ? (
                  <DataMappingRuleDetailPanel
                    rule={selected.rule}
                    onUpdate={handleUpdateRule}
                    onDelete={handleDeleteRule}
                    onToggle={handleToggleRule}
                    onMoveUp={(id) => moveRule(id, 'up')}
                    onMoveDown={(id) => moveRule(id, 'down')}
                    loading={{ edit: loading.edit, delete: loading.delete, reorder: loading.reorder, toggle: loading.toggle }}
                    error={errors.edit}
                    isOnline={isOnline}
                    isFirst={sortedRules.findIndex((r) => r.id === selected.rule.id) === 0}
                    isLast={sortedRules.findIndex((r) => r.id === selected.rule.id) === sortedRules.length - 1}
                  />
                ) : (
                  <DetailEmpty
                    icon={<ArrowUpDown className="h-12 w-12" />}
                    title="Select a Data Mapping Rule"
                    description="Choose a rule from the list to view and edit its configuration."
                  />
                )
              }
            </MasterDetailLayout>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
