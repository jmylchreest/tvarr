'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
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
import { DataMappingExpressionEditor } from '@/components/data-mapping-expression-editor';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet';
import {
  Plus,
  Edit,
  Trash2,
  ArrowRight as TransformIcon,
  Search,
  AlertCircle,
  CheckCircle,
  Loader2,
  WifiOff,
  Code,
  Play,
  Settings,
  Hash,
  ChevronDown,
  ChevronUp,
  GripVertical,
  ArrowUp,
  ArrowDown,
  Grid,
  List,
  Table as TableIcon,
} from 'lucide-react';
import { DataMappingRule, DataMappingSourceType, PaginatedResponse } from '@/types/api';
import { apiClient, ApiError } from '@/lib/api-client';
import { DEFAULT_PAGE_SIZE, API_CONFIG } from '@/lib/config';

interface LoadingState {
  rules: boolean;
  create: boolean;
  edit: boolean;
  delete: string | null;
  reorder: boolean;
}

interface ErrorState {
  rules: string | null;
  create: string | null;
  edit: string | null;
  action: string | null;
}

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleString();
}

function formatRelativeTime(dateString: string): string {
  const now = new Date();
  const date = new Date(dateString);
  const diffMs = now.getTime() - date.getTime();
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
  const diffDays = Math.floor(diffHours / 24);

  if (diffDays > 0) {
    return `${diffDays}d ago`;
  } else if (diffHours > 0) {
    return `${diffHours}h ago`;
  } else {
    return 'Just now';
  }
}

function getSourceTypeColor(sourceType: string): string {
  switch (sourceType.toLowerCase()) {
    case 'stream':
      return 'bg-blue-100 text-blue-800';
    case 'epg':
      return 'bg-green-100 text-green-800';
    default:
      return 'bg-gray-100 text-gray-800';
  }
}

function CreateDataMappingSheet({
  onCreateRule,
  loading,
  error,
}: {
  onCreateRule: (rule: Omit<DataMappingRule, 'id' | 'created_at' | 'updated_at'>) => Promise<void>;
  loading: boolean;
  error: string | null;
}) {
  const [open, setOpen] = useState(false);
  const [formData, setFormData] = useState<
    Omit<DataMappingRule, 'id' | 'created_at' | 'updated_at'>
  >({
    name: '',
    source_type: 'stream',
    expression: '',
    description: '',
    is_active: true,
    sort_order: 0,
  });
  const [mappingExpression, setMappingExpression] = useState('');

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    await onCreateRule(formData);
    if (!error) {
      setOpen(false);
      setFormData({
        name: '',
        source_type: 'stream',
        expression: '',
        description: '',
        is_active: true,
        sort_order: 0,
      });
      setMappingExpression('');
    }
  };

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <Button className="gap-2">
          <Plus className="h-4 w-4" />
          Create Data Mapping Rule
        </Button>
      </SheetTrigger>
      <SheetContent side="right" className="w-full sm:max-w-2xl overflow-y-auto">
        <SheetHeader>
          <SheetTitle>Create Data Mapping Rule</SheetTitle>
          <SheetDescription>
            Create a new rule to transform streaming or EPG data fields
          </SheetDescription>
        </SheetHeader>

        {error && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <form id="create-mapping-form" onSubmit={handleSubmit} className="space-y-4 px-4">
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="name">Rule Name</Label>
              <Input
                id="name"
                value={formData.name}
                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                placeholder="My Data Mapping Rule"
                required
                disabled={loading}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="source_type">Source Type</Label>
              <select
                id="source_type"
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
          </div>

          <div className="space-y-2">
            <Label htmlFor="description">Description (Optional)</Label>
            <Textarea
              id="description"
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
              id="is_active"
              type="checkbox"
              checked={formData.is_active}
              onChange={(e) => setFormData({ ...formData, is_active: e.target.checked })}
              className="rounded border-gray-300"
              disabled={loading}
            />
            <Label htmlFor="is_active">Active Rule</Label>
          </div>
        </form>

        <SheetFooter className="gap-2">
          <Button type="button" variant="outline" onClick={() => setOpen(false)} disabled={loading}>
            Cancel
          </Button>
          <Button form="create-mapping-form" type="submit" disabled={loading}>
            {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Create Rule
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}

function EditDataMappingSheet({
  rule,
  onUpdateRule,
  loading,
  error,
  open,
  onOpenChange,
}: {
  rule: DataMappingRule;
  onUpdateRule: (
    id: string,
    ruleData: Omit<DataMappingRule, 'id' | 'created_at' | 'updated_at'>
  ) => Promise<void>;
  loading: boolean;
  error: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [formData, setFormData] = useState<
    Omit<DataMappingRule, 'id' | 'created_at' | 'updated_at'>
  >({
    name: rule.name,
    source_type: rule.source_type,
    expression: rule.expression || '',
    description: rule.description || '',
    is_active: rule.is_active,
    sort_order: rule.sort_order,
  });
  const [mappingExpression, setMappingExpression] = useState(() => {
    try {
      return rule.expression || '';
    } catch {
      return rule.expression || '';
    }
  });

  // Reset form data when rule changes
  useEffect(() => {
    setFormData({
      name: rule.name,
      source_type: rule.source_type,
      expression: rule.expression || '',
      description: rule.description || '',
      is_active: rule.is_active,
      sort_order: rule.sort_order,
    });
    try {
      setMappingExpression(rule.expression || '');
    } catch {
      setMappingExpression(rule.expression || '');
    }
  }, [rule]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    await onUpdateRule(rule.id, formData);
    if (!error) {
      onOpenChange(false);
    }
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-full sm:max-w-2xl overflow-y-auto">
        <SheetHeader>
          <SheetTitle>Edit Data Mapping Rule</SheetTitle>
          <SheetDescription>Modify the data mapping rule configuration</SheetDescription>
        </SheetHeader>

        {error && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <form id="edit-mapping-form" onSubmit={handleSubmit} className="space-y-4 px-4">
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="edit-name">Rule Name</Label>
              <Input
                id="edit-name"
                value={formData.name}
                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                placeholder="My Data Mapping Rule"
                required
                disabled={loading}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-source_type">Source Type</Label>
              <select
                id="edit-source_type"
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
          </div>

          <div className="space-y-2">
            <Label htmlFor="edit-description">Description (Optional)</Label>
            <Textarea
              id="edit-description"
              value={formData.description || ''}
              onChange={(e) => setFormData({ ...formData, description: e.target.value })}
              placeholder="Describe what this data mapping rule does..."
              disabled={loading}
              rows={2}
            />
          </div>

          <DataMappingExpressionEditor
            key={rule.id}
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

          <div className="flex items-center space-x-6">
            <div className="flex items-center space-x-2">
              <input
                id="edit-is_active"
                type="checkbox"
                checked={formData.is_active}
                onChange={(e) => setFormData({ ...formData, is_active: e.target.checked })}
                className="rounded border-gray-300"
                disabled={loading}
              />
              <Label htmlFor="edit-is_active">Active Rule</Label>
            </div>
          </div>
        </form>

        <SheetFooter className="gap-2">
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={loading}
          >
            Cancel
          </Button>
          <Button form="edit-mapping-form" type="submit" disabled={loading}>
            {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Update Rule
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}

export function DataMapping() {
  const [allRules, setAllRules] = useState<DataMappingRule[]>([]);
  const [searchTerm, setSearchTerm] = useState('');
  const [filterSourceType, setFilterSourceType] = useState<'all' | 'stream' | 'epg'>('all');

  const [loading, setLoading] = useState<LoadingState>({
    rules: false,
    create: false,
    edit: false,
    delete: null,
    reorder: false,
  });

  const [errors, setErrors] = useState<ErrorState>({
    rules: null,
    create: null,
    edit: null,
    action: null,
  });

  const [editingRule, setEditingRule] = useState<DataMappingRule | null>(null);
  const [isEditSheetOpen, setIsEditSheetOpen] = useState(false);
  const [expandedRules, setExpandedRules] = useState<Set<string>>(new Set());
  const [isOnline, setIsOnline] = useState(true);

  // Drag and drop state
  const [draggedItem, setDraggedItem] = useState<DataMappingRule | null>(null);
  const [dragOverItem, setDragOverItem] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<'grid' | 'list' | 'table'>('table');

  // Toggle rule expansion
  const toggleRuleExpansion = (ruleId: string) => {
    setExpandedRules((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(ruleId)) {
        newSet.delete(ruleId);
      } else {
        newSet.add(ruleId);
      }
      return newSet;
    });
  };

  // Compute filtered results locally
  const filteredRules = useMemo(() => {
    let filtered = allRules;

    // Filter by source type
    if (filterSourceType !== 'all') {
      filtered = filtered.filter(
        (r) => r.source_type.toLowerCase() === filterSourceType.toLowerCase()
      );
    }

    // Filter by search term
    if (searchTerm.trim()) {
      const searchLower = searchTerm.toLowerCase();
      filtered = filtered.filter((r) => {
        // Search in basic rule properties
        const basicMatches = [
          r.name.toLowerCase(),
          r.source_type.toLowerCase(),
          r.expression || '',
          r.description || '',
        ];

        // Search in rule options/labels
        const optionMatches = [];
        if (r.is_active) optionMatches.push('active', 'enabled');
        else optionMatches.push('inactive', 'disabled');

        // Combine all searchable text
        const allSearchableText = [...basicMatches, ...optionMatches];

        // Check if search term matches any of the searchable text
        return allSearchableText.some((text) => text.toLowerCase().includes(searchLower));
      });
    }

    // Sort by sort order
    return filtered.sort((a, b) => a.sort_order - b.sort_order);
  }, [allRules, searchTerm, filterSourceType]);

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
      // Set sort order to be the highest + 1
      const maxSortOrder = allRules.length > 0 ? Math.max(...allRules.map((r) => r.sort_order)) : 0;
      const ruleWithSortOrder = { ...newRule, sort_order: maxSortOrder + 1 };

      await apiClient.createDataMappingRule(ruleWithSortOrder);
      await loadRules(); // Reload rules after creation
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
    ruleData: Omit<DataMappingRule, 'id' | 'created_at' | 'updated_at'>
  ) => {
    setLoading((prev) => ({ ...prev, edit: true }));
    setErrors((prev) => ({ ...prev, edit: null }));

    try {
      await apiClient.updateDataMappingRule(id, ruleData);
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

  const moveRule = async (ruleId: string, direction: 'up' | 'down') => {
    const currentIndex = filteredRules.findIndex((r) => r.id === ruleId);
    if (currentIndex === -1) return;

    const targetIndex = direction === 'up' ? currentIndex - 1 : currentIndex + 1;
    if (targetIndex < 0 || targetIndex >= filteredRules.length) return;

    setLoading((prev) => ({ ...prev, reorder: true }));
    setErrors((prev) => ({ ...prev, action: null }));

    try {
      // Create new array with swapped items
      const newOrder = [...filteredRules];
      [newOrder[currentIndex], newOrder[targetIndex]] = [
        newOrder[targetIndex],
        newOrder[currentIndex],
      ];

      // Update sort orders
      const reorderRequest = newOrder.map((rule, index) => ({
        id: rule.id,
        sort_order: index + 1,
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

  // Drag and drop handlers using HTML5 API
  const handleDragStart = (e: React.DragEvent, rule: DataMappingRule) => {
    setDraggedItem(rule);
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/html', rule.id);
  };

  const handleDragOver = (e: React.DragEvent, ruleId: string) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    setDragOverItem(ruleId);
  };

  const handleDragLeave = () => {
    setDragOverItem(null);
  };

  const handleDrop = async (e: React.DragEvent, targetRule: DataMappingRule) => {
    e.preventDefault();
    setDragOverItem(null);

    if (!draggedItem || draggedItem.id === targetRule.id) {
      setDraggedItem(null);
      return;
    }

    setLoading((prev) => ({ ...prev, reorder: true }));
    setErrors((prev) => ({ ...prev, action: null }));

    try {
      const draggedIndex = filteredRules.findIndex((r) => r.id === draggedItem.id);
      const targetIndex = filteredRules.findIndex((r) => r.id === targetRule.id);

      if (draggedIndex === -1 || targetIndex === -1) return;

      // Create new array with reordered items
      const newOrder = [...filteredRules];
      newOrder.splice(draggedIndex, 1);
      newOrder.splice(targetIndex, 0, draggedItem);

      // Update sort orders
      const reorderRequest = newOrder.map((rule, index) => ({
        id: rule.id,
        sort_order: index + 1,
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
      setDraggedItem(null);
    }
  };

  const streamRules = allRules?.filter((r) => r.source_type.toLowerCase() === 'stream').length || 0;
  const epgRules = allRules?.filter((r) => r.source_type.toLowerCase() === 'epg').length || 0;
  const activeRules = allRules?.filter((r) => r.is_active).length || 0;
  const totalRules = allRules?.length || 0;

  return (
    <div className="space-y-6">
      {/* Header Section */}
      <div className="flex items-center justify-between">
        <div>
          <p className="text-muted-foreground">Manage data transformation rules</p>
        </div>
        <div className="flex items-center gap-2">
          {!isOnline && <WifiOff className="h-5 w-5 text-destructive" />}
          <CreateDataMappingSheet
            onCreateRule={handleCreateRule}
            loading={loading.create}
            error={errors.create}
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
      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Total Rules</CardTitle>
            <TransformIcon className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{totalRules}</div>
            <p className="text-xs text-muted-foreground">Data transformation rules</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Stream Rules</CardTitle>
            <Play className="h-4 w-4 text-blue-600" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{streamRules}</div>
            <p className="text-xs text-muted-foreground">Stream data mapping</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">EPG Rules</CardTitle>
            <Code className="h-4 w-4 text-green-600" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{epgRules}</div>
            <p className="text-xs text-muted-foreground">EPG data mapping</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Active Rules</CardTitle>
            <CheckCircle className="h-4 w-4 text-orange-600" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{activeRules}</div>
            <p className="text-xs text-muted-foreground">Currently enabled</p>
          </CardContent>
        </Card>
      </div>

      {/* Search & Filters */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Search className="h-5 w-5" />
            Search & Filters
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col sm:flex-row gap-4">
            <div className="flex-1">
              <div className="relative">
                <Search className="absolute left-2 top-2.5 h-4 w-4 text-muted-foreground" />
                <Input
                  placeholder="Search rules, expressions, descriptions..."
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  className="pl-8"
                  disabled={loading.rules}
                />
              </div>
            </div>
            <Select
              value={filterSourceType}
              onValueChange={(value) => setFilterSourceType(value as 'all' | 'stream' | 'epg')}
              disabled={loading.rules}
            >
              <SelectTrigger className="w-full sm:w-[180px]">
                <SelectValue placeholder="Filter by type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Types</SelectItem>
                <SelectItem value="stream">Stream Only</SelectItem>
                <SelectItem value="epg">EPG Only</SelectItem>
              </SelectContent>
            </Select>

            {/* Layout Chooser */}
            <div className="flex rounded-md border">
              <Button
                size="sm"
                variant={viewMode === 'table' ? 'default' : 'ghost'}
                className="rounded-r-none border-r"
                onClick={() => setViewMode('table')}
              >
                <TableIcon className="w-4 h-4" />
              </Button>
              <Button
                size="sm"
                variant={viewMode === 'grid' ? 'default' : 'ghost'}
                className="rounded-none border-r"
                onClick={() => setViewMode('grid')}
              >
                <Grid className="w-4 h-4" />
              </Button>
              <Button
                size="sm"
                variant={viewMode === 'list' ? 'default' : 'ghost'}
                className="rounded-l-none"
                onClick={() => setViewMode('list')}
              >
                <List className="w-4 h-4" />
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Data Mapping Rules Display */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center justify-between">
            <span>
              Data Mapping Rules ({filteredRules?.length || 0}
              {searchTerm || filterSourceType !== 'all' ? ` of ${allRules?.length || 0}` : ''})
            </span>
            {(loading.rules || loading.reorder) && <Loader2 className="h-4 w-4 animate-spin" />}
          </CardTitle>
          <CardDescription>
            Manage data transformation rules with drag-and-drop ordering
          </CardDescription>
        </CardHeader>
        <CardContent>
          {errors.rules ? (
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
          ) : (
            <>
              {viewMode === 'table' && (
                <div className="space-y-4">
                  {filteredRules?.map((rule, index) => {
                    const safeKey = rule?.id
                      ? String(rule.id)
                      : `rule-${index}-${rule?.name || 'unnamed'}`;

                    return (
                      <Card
                        key={safeKey}
                        className={`relative transition-all ${
                          dragOverItem === rule.id ? 'border-blue-500 bg-blue-50' : ''
                        } ${draggedItem?.id === rule.id ? 'opacity-50' : ''}`}
                        draggable
                        onDragStart={(e) => handleDragStart(e, rule)}
                        onDragOver={(e) => handleDragOver(e, rule.id)}
                        onDragLeave={handleDragLeave}
                        onDrop={(e) => handleDrop(e, rule)}
                      >
                        <CardHeader className="pb-3">
                          <div className="flex items-start justify-between">
                            <div className="flex items-start gap-3 flex-1">
                              <div className="flex items-center pt-1">
                                <GripVertical className="h-4 w-4 text-muted-foreground cursor-grab" />
                              </div>
                              <div className="space-y-2 flex-1">
                                <div className="flex items-center gap-2">
                                  <Badge
                                    variant="secondary"
                                    className="text-xs font-mono bg-muted text-muted-foreground"
                                  >
                                    {rule.sort_order}
                                  </Badge>
                                  <CardTitle className="text-lg">{rule.name}</CardTitle>
                                  <Badge className={getSourceTypeColor(rule.source_type)}>
                                    {rule.source_type.toUpperCase()}
                                  </Badge>
                                  {rule.is_active ? (
                                    <Badge
                                      variant="outline"
                                      className="text-green-600 border-green-600"
                                    >
                                      <CheckCircle className="h-3 w-3 mr-1" />
                                      Active
                                    </Badge>
                                  ) : (
                                    <Badge
                                      variant="outline"
                                      className="text-gray-500 border-gray-500"
                                    >
                                      Inactive
                                    </Badge>
                                  )}
                                </div>
                                {rule.description && (
                                  <p className="text-sm text-muted-foreground">
                                    {rule.description}
                                  </p>
                                )}
                                <div className="text-xs text-muted-foreground">
                                  Created {formatRelativeTime(rule.created_at)}
                                </div>
                              </div>
                            </div>
                            <div className="flex items-center gap-1">
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => moveRule(rule.id, 'up')}
                                className="h-8 w-8 p-0"
                                disabled={index === 0 || loading.reorder}
                                title="Move up"
                              >
                                <ArrowUp className="h-4 w-4" />
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => moveRule(rule.id, 'down')}
                                className="h-8 w-8 p-0"
                                disabled={index === filteredRules.length - 1 || loading.reorder}
                                title="Move down"
                              >
                                <ArrowDown className="h-4 w-4" />
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => rule?.id && toggleRuleExpansion(rule.id)}
                                className="h-8 w-8 p-0"
                                title={
                                  rule?.id && expandedRules.has(rule.id)
                                    ? 'Collapse expression'
                                    : 'Expand expression'
                                }
                                disabled={!rule?.id}
                              >
                                {rule?.id && expandedRules.has(rule.id) ? (
                                  <ChevronUp className="h-4 w-4" />
                                ) : (
                                  <ChevronDown className="h-4 w-4" />
                                )}
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => {
                                  setEditingRule(rule);
                                  setIsEditSheetOpen(true);
                                }}
                                className="h-8 w-8 p-0"
                                disabled={!isOnline}
                                title="Edit rule"
                              >
                                <Edit className="h-4 w-4" />
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => handleDeleteRule(rule.id)}
                                className="h-8 w-8 p-0 text-destructive hover:text-destructive"
                                disabled={loading.delete === rule.id || !isOnline}
                                title="Delete rule"
                              >
                                {loading.delete === rule.id ? (
                                  <Loader2 className="h-4 w-4 animate-spin" />
                                ) : (
                                  <Trash2 className="h-4 w-4" />
                                )}
                              </Button>
                            </div>
                          </div>
                        </CardHeader>
                        {rule?.id && expandedRules.has(rule.id) && (
                          <CardContent className="pt-0">
                            {rule.expression ? (
                              <div className="bg-muted/30 rounded-lg p-4">
                                <div className="space-y-2">
                                  <p className="text-sm font-medium text-muted-foreground">
                                    Transformation Expression:
                                  </p>
                                  <code className="text-sm bg-background p-2 rounded block overflow-x-auto">
                                    {rule.expression}
                                  </code>
                                </div>
                              </div>
                            ) : (
                              <div className="bg-muted/30 rounded-lg p-4 text-center text-muted-foreground">
                                <TransformIcon className="h-8 w-8 mx-auto mb-2 opacity-50" />
                                <p className="text-sm">No transformation expression available</p>
                              </div>
                            )}
                          </CardContent>
                        )}
                      </Card>
                    );
                  })}
                </div>
              )}

              {viewMode === 'grid' && (
                <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
                  {filteredRules?.map((rule, index) => {
                    const safeKey = rule?.id
                      ? String(rule.id)
                      : `rule-${index}-${rule?.name || 'unnamed'}`;

                    return (
                      <Card
                        key={safeKey}
                        className={`transition-all hover:shadow-md ${
                          dragOverItem === rule.id ? 'border-blue-500 bg-blue-50' : ''
                        } ${draggedItem?.id === rule.id ? 'opacity-50' : ''}`}
                        draggable
                        onDragStart={(e) => handleDragStart(e, rule)}
                        onDragOver={(e) => handleDragOver(e, rule.id)}
                        onDragLeave={handleDragLeave}
                        onDrop={(e) => handleDrop(e, rule)}
                      >
                        <CardHeader>
                          <div className="flex items-start justify-between">
                            <div className="space-y-1 flex-1">
                              <div className="flex items-center gap-2">
                                <GripVertical className="h-3 w-3 text-muted-foreground cursor-grab" />
                                <Badge variant="secondary" className="text-xs font-mono">
                                  #{rule.sort_order}
                                </Badge>
                                <CardTitle className="text-base">{rule.name}</CardTitle>
                              </div>
                              <div className="flex flex-wrap gap-1">
                                <Badge className={getSourceTypeColor(rule.source_type)}>
                                  {rule.source_type.toUpperCase()}
                                </Badge>
                                {rule.is_active ? (
                                  <Badge
                                    variant="outline"
                                    className="text-green-600 border-green-600"
                                  >
                                    <CheckCircle className="h-3 w-3 mr-1" />
                                    Active
                                  </Badge>
                                ) : (
                                  <Badge
                                    variant="outline"
                                    className="text-gray-500 border-gray-500"
                                  >
                                    Inactive
                                  </Badge>
                                )}
                              </div>
                            </div>
                          </div>
                        </CardHeader>
                        <CardContent>
                          <div className="space-y-4">
                            {rule.description && (
                              <p className="text-sm text-muted-foreground">{rule.description}</p>
                            )}

                            <div className="text-xs text-muted-foreground">
                              Created {formatRelativeTime(rule.created_at)}
                            </div>

                            <div className="flex items-center justify-between pt-2 border-t">
                              <div className="flex items-center gap-1">
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => moveRule(rule.id, 'up')}
                                  className="h-8 w-8 p-0"
                                  disabled={index === 0 || loading.reorder}
                                  title="Move up"
                                >
                                  <ArrowUp className="h-3 w-3" />
                                </Button>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => moveRule(rule.id, 'down')}
                                  className="h-8 w-8 p-0"
                                  disabled={index === filteredRules.length - 1 || loading.reorder}
                                  title="Move down"
                                >
                                  <ArrowDown className="h-3 w-3" />
                                </Button>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => rule?.id && toggleRuleExpansion(rule.id)}
                                  className="h-8 px-2 text-xs"
                                  disabled={!rule?.id}
                                >
                                  {rule?.id && expandedRules.has(rule.id) ? (
                                    <>
                                      <ChevronUp className="h-3 w-3 mr-1" />
                                      Hide
                                    </>
                                  ) : (
                                    <>
                                      <ChevronDown className="h-3 w-3 mr-1" />
                                      Show
                                    </>
                                  )}
                                </Button>
                              </div>
                              <div className="flex items-center gap-1">
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => {
                                    setEditingRule(rule);
                                    setIsEditSheetOpen(true);
                                  }}
                                  className="h-8 w-8 p-0"
                                  disabled={!isOnline}
                                >
                                  <Edit className="h-4 w-4" />
                                </Button>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => handleDeleteRule(rule.id)}
                                  className="h-8 w-8 p-0 text-destructive hover:text-destructive"
                                  disabled={loading.delete === rule.id || !isOnline}
                                >
                                  {loading.delete === rule.id ? (
                                    <Loader2 className="h-4 w-4 animate-spin" />
                                  ) : (
                                    <Trash2 className="h-4 w-4" />
                                  )}
                                </Button>
                              </div>
                            </div>
                          </div>
                        </CardContent>
                        {rule?.id && expandedRules.has(rule.id) && (
                          <CardContent className="pt-0 border-t">
                            {rule.expression ? (
                              <div className="bg-muted/30 rounded-lg p-3">
                                <div className="space-y-1">
                                  <p className="text-xs font-medium text-muted-foreground">
                                    Expression:
                                  </p>
                                  <code className="text-xs bg-background p-2 rounded block overflow-x-auto">
                                    {rule.expression}
                                  </code>
                                </div>
                              </div>
                            ) : (
                              <div className="bg-muted/30 rounded-lg p-3 text-center text-muted-foreground">
                                <TransformIcon className="h-6 w-6 mx-auto mb-1 opacity-50" />
                                <p className="text-xs">No expression</p>
                              </div>
                            )}
                          </CardContent>
                        )}
                      </Card>
                    );
                  })}
                </div>
              )}

              {viewMode === 'list' && (
                <div className="space-y-2">
                  {filteredRules?.map((rule, index) => {
                    const safeKey = rule?.id
                      ? String(rule.id)
                      : `rule-${index}-${rule?.name || 'unnamed'}`;

                    return (
                      <Card
                        key={safeKey}
                        className={`transition-all hover:shadow-sm ${
                          dragOverItem === rule.id ? 'border-blue-500 bg-blue-50' : ''
                        } ${draggedItem?.id === rule.id ? 'opacity-50' : ''}`}
                        draggable
                        onDragStart={(e) => handleDragStart(e, rule)}
                        onDragOver={(e) => handleDragOver(e, rule.id)}
                        onDragLeave={handleDragLeave}
                        onDrop={(e) => handleDrop(e, rule)}
                      >
                        <CardContent className="pt-4">
                          <div className="flex items-center justify-between">
                            <div className="flex items-center space-x-4 flex-1">
                              <div className="flex items-center">
                                <GripVertical className="h-4 w-4 text-muted-foreground cursor-grab mr-2" />
                                <Badge variant="secondary" className="text-xs font-mono">
                                  #{rule.sort_order}
                                </Badge>
                              </div>
                              <div className="flex-1 min-w-0">
                                <div className="flex items-center gap-3">
                                  <div>
                                    <p className="font-medium text-sm">{rule.name}</p>
                                    <p className="text-xs text-muted-foreground">
                                      {rule.description && rule.description.length > 50
                                        ? `${rule.description.substring(0, 50)}...`
                                        : rule.description || 'No description'}
                                    </p>
                                  </div>
                                  <div className="flex items-center gap-2">
                                    <Badge className={getSourceTypeColor(rule.source_type)}>
                                      {rule.source_type.toUpperCase()}
                                    </Badge>
                                    {rule.is_active ? (
                                      <Badge
                                        variant="outline"
                                        className="text-green-600 border-green-600 text-xs"
                                      >
                                        <CheckCircle className="h-3 w-3 mr-1" />
                                        Active
                                      </Badge>
                                    ) : (
                                      <Badge
                                        variant="outline"
                                        className="text-gray-500 border-gray-500 text-xs"
                                      >
                                        Inactive
                                      </Badge>
                                    )}
                                  </div>
                                </div>
                              </div>
                            </div>
                            <div className="flex items-center gap-2 ml-4">
                              <div className="flex items-center gap-1">
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => moveRule(rule.id, 'up')}
                                  className="h-8 w-8 p-0"
                                  disabled={index === 0 || loading.reorder}
                                  title="Move up"
                                >
                                  <ArrowUp className="h-4 w-4" />
                                </Button>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => moveRule(rule.id, 'down')}
                                  className="h-8 w-8 p-0"
                                  disabled={index === filteredRules.length - 1 || loading.reorder}
                                  title="Move down"
                                >
                                  <ArrowDown className="h-4 w-4" />
                                </Button>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => rule?.id && toggleRuleExpansion(rule.id)}
                                  className="h-8 w-8 p-0"
                                  disabled={!rule?.id}
                                >
                                  {rule?.id && expandedRules.has(rule.id) ? (
                                    <ChevronUp className="h-4 w-4" />
                                  ) : (
                                    <ChevronDown className="h-4 w-4" />
                                  )}
                                </Button>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => {
                                    setEditingRule(rule);
                                    setIsEditSheetOpen(true);
                                  }}
                                  className="h-8 w-8 p-0"
                                  disabled={!isOnline}
                                >
                                  <Edit className="h-4 w-4" />
                                </Button>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => handleDeleteRule(rule.id)}
                                  className="h-8 w-8 p-0 text-destructive hover:text-destructive"
                                  disabled={loading.delete === rule.id || !isOnline}
                                >
                                  {loading.delete === rule.id ? (
                                    <Loader2 className="h-4 w-4 animate-spin" />
                                  ) : (
                                    <Trash2 className="h-4 w-4" />
                                  )}
                                </Button>
                              </div>
                            </div>
                          </div>
                          {rule?.id && expandedRules.has(rule.id) && (
                            <div className="mt-4 pt-4 border-t">
                              {rule.expression ? (
                                <div className="bg-muted/30 rounded-lg p-3">
                                  <div className="space-y-1">
                                    <p className="text-xs font-medium text-muted-foreground">
                                      Transformation Expression:
                                    </p>
                                    <code className="text-xs bg-background p-2 rounded block overflow-x-auto">
                                      {rule.expression}
                                    </code>
                                  </div>
                                </div>
                              ) : (
                                <div className="bg-muted/30 rounded-lg p-3 text-center text-muted-foreground">
                                  <TransformIcon className="h-6 w-6 mx-auto mb-1 opacity-50" />
                                  <p className="text-xs">No transformation expression available</p>
                                </div>
                              )}
                            </div>
                          )}
                        </CardContent>
                      </Card>
                    );
                  })}
                </div>
              )}

              {filteredRules?.length === 0 && !loading.rules && (
                <div className="text-center py-8">
                  <TransformIcon className="mx-auto h-12 w-12 text-muted-foreground" />
                  <h3 className="mt-4 text-lg font-semibold">
                    {searchTerm || filterSourceType !== 'all'
                      ? 'No matching rules'
                      : 'No data mapping rules found'}
                  </h3>
                  <p className="text-muted-foreground">
                    {searchTerm || filterSourceType !== 'all'
                      ? 'Try adjusting your search or filter criteria.'
                      : 'Get started by creating your first data transformation rule.'}
                  </p>
                </div>
              )}
            </>
          )}
        </CardContent>
      </Card>

      {/* Edit Rule Sheet */}
      {editingRule && (
        <EditDataMappingSheet
          rule={editingRule}
          onUpdateRule={handleUpdateRule}
          loading={loading.edit}
          error={errors.edit}
          open={isEditSheetOpen}
          onOpenChange={setIsEditSheetOpen}
        />
      )}
    </div>
  );
}
