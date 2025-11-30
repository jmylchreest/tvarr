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
import { FilterExpressionEditor } from '@/components/filter-expression-editor';
import { ConditionTreeView } from '@/components/condition-tree-view';
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
  Filter as FilterIcon,
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
  Grid,
  List,
  Table as TableIcon,
  Copy,
  Check,
} from 'lucide-react';
import { Filter, FilterWithMeta, PaginatedResponse } from '@/types/api';
import { apiClient, ApiError } from '@/lib/api-client';
import { DEFAULT_PAGE_SIZE, API_CONFIG } from '@/lib/config';

interface LoadingState {
  filters: boolean;
  create: boolean;
  edit: boolean;
  delete: string | null;
}

interface ErrorState {
  filters: string | null;
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

function CreateFilterSheet({
  onCreateFilter,
  loading,
  error,
}: {
  onCreateFilter: (filter: Omit<Filter, 'id' | 'created_at' | 'updated_at'>) => Promise<void>;
  loading: boolean;
  error: string | null;
}) {
  const [open, setOpen] = useState(false);
  const [formData, setFormData] = useState<
    Omit<
      Filter,
      'id' | 'created_at' | 'updated_at' | 'condition_tree' | 'usage_count' | 'is_system_default'
    >
  >({
    name: '',
    source_type: 'stream',
    is_inverse: false,
    expression: '',
  });
  const [filterExpression, setFilterExpression] = useState('');

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    await onCreateFilter({
      ...formData,
      condition_tree: {},
      usage_count: 0,
      is_system_default: false,
    });
    if (!error) {
      setOpen(false);
      setFormData({
        name: '',
        source_type: 'stream',
        is_inverse: false,
        expression: '',
      });
      setFilterExpression('');
    }
  };

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <Button className="gap-2">
          <Plus className="h-4 w-4" />
          Create Filter
        </Button>
      </SheetTrigger>
      <SheetContent side="right" className="w-full sm:max-w-2xl overflow-y-auto">
        <SheetHeader>
          <SheetTitle>Create Filter</SheetTitle>
          <SheetDescription>Create a new filter to process streaming or EPG data</SheetDescription>
        </SheetHeader>

        {error && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <form id="create-filter-form" onSubmit={handleSubmit} className="space-y-4 px-4">
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="name">Name</Label>
              <Input
                id="name"
                value={formData.name}
                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                placeholder="My Filter"
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
                  setFormData({ ...formData, source_type: e.target.value as 'stream' | 'epg' })
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

          <FilterExpressionEditor
            value={filterExpression}
            onChange={(value) => {
              setFilterExpression(value);
              setFormData({ ...formData, expression: value });
            }}
            sourceType={formData.source_type}
            placeholder='Enter filter expression (e.g., channel_name contains "sport")'
            disabled={loading}
            showTestResults={true}
            autoTest={true}
          />

          <div className="flex items-center space-x-2">
            <input
              id="is_inverse"
              type="checkbox"
              checked={formData.is_inverse}
              onChange={(e) => setFormData({ ...formData, is_inverse: e.target.checked })}
              className="rounded border-gray-300"
              disabled={loading}
            />
            <Label htmlFor="is_inverse">Inverse Filter</Label>
          </div>
        </form>

        <SheetFooter className="gap-2">
          <Button type="button" variant="outline" onClick={() => setOpen(false)} disabled={loading}>
            Cancel
          </Button>
          <Button form="create-filter-form" type="submit" disabled={loading}>
            {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Create Filter
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}

function EditFilterSheet({
  filter,
  onUpdateFilter,
  loading,
  error,
  open,
  onOpenChange,
}: {
  filter: Filter;
  onUpdateFilter: (
    id: string,
    filterData: Omit<Filter, 'id' | 'created_at' | 'updated_at'>
  ) => Promise<void>;
  loading: boolean;
  error: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [formData, setFormData] = useState<Omit<Filter, 'id' | 'created_at' | 'updated_at'>>({
    name: filter.name,
    source_type: filter.source_type,
    is_inverse: filter.is_inverse,
    expression: filter.expression,
    condition_tree: filter.condition_tree,
    usage_count: filter.usage_count,
    is_system_default: filter.is_system_default,
  });
  const [filterExpression, setFilterExpression] = useState(() => {
    try {
      return filter.expression || '';
    } catch {
      return filter.expression || '';
    }
  });

  // Reset form data when filter changes
  useEffect(() => {
    setFormData({
      name: filter.name,
      source_type: filter.source_type,
      is_inverse: filter.is_inverse,
      expression: filter.expression,
      condition_tree: filter.condition_tree,
      usage_count: filter.usage_count,
      is_system_default: filter.is_system_default,
    });
    try {
      setFilterExpression(filter.expression || '');
    } catch {
      setFilterExpression(filter.expression || '');
    }
  }, [filter]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    await onUpdateFilter(filter.id, {
      ...formData,
      condition_tree: filter.condition_tree,
      usage_count: filter.usage_count,
      is_system_default: filter.is_system_default,
    });
    if (!error) {
      onOpenChange(false);
    }
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-full sm:max-w-2xl overflow-y-auto">
        <SheetHeader>
          <SheetTitle>Edit Filter</SheetTitle>
          <SheetDescription>Modify the filter configuration</SheetDescription>
        </SheetHeader>

        {error && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <form id="edit-filter-form" onSubmit={handleSubmit} className="space-y-4 px-4">
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="edit-name">Name</Label>
              <Input
                id="edit-name"
                value={formData.name}
                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                placeholder="My Filter"
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
                  setFormData({ ...formData, source_type: e.target.value as 'stream' | 'epg' })
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

          <FilterExpressionEditor
            key={filter.id}
            value={filterExpression}
            onChange={(value) => {
              setFilterExpression(value);
              setFormData({ ...formData, expression: value });
            }}
            sourceType={formData.source_type}
            placeholder='Enter filter expression (e.g., channel_name contains "sport")'
            disabled={loading}
            showTestResults={true}
            autoTest={true}
          />

          <div className="flex items-center space-x-6">
            <div className="flex items-center space-x-2">
              <input
                id="edit-is_inverse"
                type="checkbox"
                checked={formData.is_inverse}
                onChange={(e) => setFormData({ ...formData, is_inverse: e.target.checked })}
                className="rounded border-gray-300"
                disabled={loading}
              />
              <Label htmlFor="edit-is_inverse">Inverse Filter</Label>
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
          <Button form="edit-filter-form" type="submit" disabled={loading}>
            {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Update Filter
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}

export function Filters() {
  const [allFilters, setAllFilters] = useState<FilterWithMeta[]>([]);
  const [pagination, setPagination] = useState<{ total: number } | null>(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [filterSourceType, setFilterSourceType] = useState<'all' | 'stream' | 'epg'>('all');
  const [currentPage, setCurrentPage] = useState(1);

  const [loading, setLoading] = useState<LoadingState>({
    filters: false,
    create: false,
    edit: false,
    delete: null,
  });

  const [errors, setErrors] = useState<ErrorState>({
    filters: null,
    create: null,
    edit: null,
    action: null,
  });

  const [editingFilter, setEditingFilter] = useState<Filter | null>(null);
  const [isEditSheetOpen, setIsEditSheetOpen] = useState(false);
  const [expandedFilters, setExpandedFilters] = useState<Set<string>>(new Set());
  const [isOnline, setIsOnline] = useState(true);
  const [viewMode, setViewMode] = useState<'grid' | 'list' | 'table'>('table');
  const [copiedItems, setCopiedItems] = useState<Set<string>>(new Set());

  // Helper function to recursively extract searchable text from expression tree
  const extractTreeText = useCallback((tree: any): string[] => {
    const texts: string[] = [];

    if (!tree) return texts;

    if (tree.type === 'condition') {
      // Add field name
      if (tree.field) texts.push(tree.field);

      // Add operator (both raw and formatted)
      if (tree.operator) {
        texts.push(tree.operator);
        // Add formatted operator names that users see
        const formattedOp = tree.operator.toLowerCase();
        const isNegated = formattedOp.startsWith('not_') || tree.negate === true;

        switch (formattedOp) {
          case 'contains':
          case 'not_contains':
            texts.push(isNegated ? 'not contains' : 'contains');
            if (isNegated) texts.push('notcontains');
            break;
          case 'equals':
          case 'not_equals':
            texts.push(isNegated ? 'not equals' : 'equals');
            if (isNegated) texts.push('notequals');
            break;
          case 'matches':
          case 'not_matches':
            texts.push(isNegated ? 'not matches' : 'matches');
            if (isNegated) texts.push('notmatches');
            break;
          case 'starts_with':
          case 'startswith':
          case 'not_starts_with':
          case 'not_startswith':
            texts.push(isNegated ? 'not starts with' : 'starts with');
            if (isNegated) texts.push('not startswith', 'notstartswith');
            else texts.push('startswith');
            break;
          case 'ends_with':
          case 'endswith':
          case 'not_ends_with':
          case 'not_endswith':
            texts.push(isNegated ? 'not ends with' : 'ends with');
            if (isNegated) texts.push('not endswith', 'notendswith');
            else texts.push('endswith');
            break;
        }
      }

      // Add value
      if (tree.value) texts.push(tree.value);

      // Add modifiers
      if (tree.case_sensitive) texts.push('case sensitive', 'case-sensitive');
      if (tree.negate) texts.push('not', 'negate', 'negated');
    }

    if (tree.type === 'group') {
      // Add group operator
      if (tree.operator) {
        texts.push(tree.operator.toLowerCase());
        if (tree.operator.toLowerCase() === 'and') texts.push('and');
        if (tree.operator.toLowerCase() === 'or') texts.push('or');
      }

      // Recursively process children
      if (tree.children && Array.isArray(tree.children)) {
        tree.children.forEach((child: any) => {
          texts.push(...extractTreeText(child));
        });
      }
    }

    return texts;
  }, []);

  // Toggle filter expansion
  const toggleFilterExpansion = (filterId: string) => {
    setExpandedFilters((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(filterId)) {
        newSet.delete(filterId);
      } else {
        newSet.add(filterId);
      }
      return newSet;
    });
  };

  // Copy filter expression to clipboard
  const copyFilterExpression = async (filterId: string, expression: string) => {
    try {
      await navigator.clipboard.writeText(expression);
      setCopiedItems((prev) => new Set(prev).add(filterId));

      // Reset copied state after 2 seconds
      setTimeout(() => {
        setCopiedItems((prev) => {
          const newSet = new Set(prev);
          newSet.delete(filterId);
          return newSet;
        });
      }, 2000);
    } catch (error) {
      console.error('Failed to copy to clipboard:', error);
    }
  };

  // Compute filtered results locally
  const filteredFilters = useMemo(() => {
    let filtered = allFilters;

    // Filter by source type
    if (filterSourceType !== 'all') {
      filtered = filtered.filter(
        (f) => f.filter.source_type.toLowerCase() === filterSourceType.toLowerCase()
      );
    }

    // Filter by search term
    if (searchTerm.trim()) {
      const searchLower = searchTerm.toLowerCase();
      filtered = filtered.filter((f) => {
        const filter = f.filter;

        // Search in basic filter properties
        const basicMatches = [
          filter.name.toLowerCase(),
          filter.source_type.toLowerCase(),
          filter.expression || '',
        ];

        // Search in filter options/labels
        const optionMatches = [];
        if (filter.is_inverse) optionMatches.push('inverse', 'inverted');
        if (filter.is_system_default) optionMatches.push('system default', 'default', 'system');

        // Search in condition tree content
        const treeMatches: string[] = [];
        if (filter.condition_tree) {
          try {
            let tree: any;
            if (typeof filter.condition_tree === 'string') {
              tree = JSON.parse(filter.condition_tree);
            } else {
              tree = filter.condition_tree;
            }
            if (tree.root) {
              treeMatches.push(...extractTreeText(tree.root));
            }
          } catch (error) {
            // Ignore parsing errors for search
          }
        }

        // Combine all searchable text
        const allSearchableText = [...basicMatches, ...optionMatches, ...treeMatches];

        // Check if search term matches any of the searchable text
        return allSearchableText.some((text) => text.toLowerCase().includes(searchLower));
      });
    }

    return filtered;
  }, [allFilters, searchTerm, filterSourceType, extractTreeText]);

  // Health check is handled by parent component, no need for redundant calls

  const loadFilters = useCallback(async () => {
    if (!isOnline) return;

    setLoading((prev) => ({ ...prev, filters: true }));
    setErrors((prev) => ({ ...prev, filters: null }));

    try {
      // Load all filters without search parameters - filtering happens locally
      const response = await apiClient.getFilters();

      // Filter out any malformed filter objects that might cause React key issues
      const validFilters = response.filter((filterWithMeta) => {
        if (!filterWithMeta?.filter) {
          console.warn('Invalid filter object - missing filter property:', filterWithMeta);
          return false;
        }

        if (!filterWithMeta.filter.id || typeof filterWithMeta.filter.id !== 'string') {
          console.warn('Invalid filter object - missing or invalid ID:', filterWithMeta.filter);
          return false;
        }

        return true;
      });

      if (validFilters.length !== response.length) {
        console.warn(`Filtered out ${response.length - validFilters.length} invalid filter(s)`);
      }

      setAllFilters(validFilters);
      setPagination({
        total: validFilters.length,
      });
      setIsOnline(true);
    } catch (error) {
      const apiError = error as ApiError;
      if (apiError.status === 0) {
        setIsOnline(false);
        setErrors((prev) => ({
          ...prev,
          filters: `Unable to connect to the API service. Please check that the service is running at ${API_CONFIG.baseUrl}.`,
        }));
      } else {
        setErrors((prev) => ({
          ...prev,
          filters: `Failed to load filters: ${apiError.message}`,
        }));
      }
    } finally {
      setLoading((prev) => ({ ...prev, filters: false }));
    }
  }, [isOnline]);

  // Load filters on mount only
  useEffect(() => {
    loadFilters();
  }, [loadFilters]);

  const handleCreateFilter = async (
    newFilter: Omit<Filter, 'id' | 'created_at' | 'updated_at'>
  ) => {
    setLoading((prev) => ({ ...prev, create: true }));
    setErrors((prev) => ({ ...prev, create: null }));

    try {
      await apiClient.createFilter(newFilter);
      await loadFilters(); // Reload filters after creation
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        create: `Failed to create filter: ${apiError.message}`,
      }));
      throw error; // Re-throw to prevent dialog from closing
    } finally {
      setLoading((prev) => ({ ...prev, create: false }));
    }
  };

  const handleUpdateFilter = async (
    id: string,
    filterData: Omit<Filter, 'id' | 'created_at' | 'updated_at'>
  ) => {
    setLoading((prev) => ({ ...prev, edit: true }));
    setErrors((prev) => ({ ...prev, edit: null }));

    try {
      await apiClient.updateFilter(id, filterData);
      await loadFilters(); // Reload filters after update
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        edit: `Failed to update filter: ${apiError.message}`,
      }));
      throw error; // Re-throw to prevent dialog from closing
    } finally {
      setLoading((prev) => ({ ...prev, edit: false }));
    }
  };

  const handleDeleteFilter = async (filterId: string) => {
    if (!confirm('Are you sure you want to delete this filter? This action cannot be undone.')) {
      return;
    }

    setLoading((prev) => ({ ...prev, delete: filterId }));
    setErrors((prev) => ({ ...prev, action: null }));

    try {
      await apiClient.deleteFilter(filterId);
      await loadFilters(); // Reload filters after deletion
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        action: `Failed to delete filter: ${apiError.message}`,
      }));
    } finally {
      setLoading((prev) => ({ ...prev, delete: null }));
    }
  };

  const streamFilters =
    allFilters?.filter((f) => f.filter.source_type.toLowerCase() === 'stream').length || 0;
  const epgFilters =
    allFilters?.filter((f) => f.filter.source_type.toLowerCase() === 'epg').length || 0;
  const systemDefaults = allFilters?.filter((f) => f.filter.is_system_default).length || 0;
  const totalFilters = allFilters?.length || 0;

  return (
    <div className="space-y-6">
      {/* Header Section */}
      <div className="flex items-center justify-between">
        <div>
          <p className="text-muted-foreground">Manage filtering rules</p>
        </div>
        <div className="flex items-center gap-2">
          {!isOnline && <WifiOff className="h-5 w-5 text-destructive" />}
          <CreateFilterSheet
            onCreateFilter={handleCreateFilter}
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
            <CardTitle className="text-sm font-medium">Total Filters</CardTitle>
            <FilterIcon className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{totalFilters}</div>
            <p className="text-xs text-muted-foreground">Active filtering rules</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Stream Filters</CardTitle>
            <Play className="h-4 w-4 text-blue-600" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{streamFilters}</div>
            <p className="text-xs text-muted-foreground">Stream processing</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">EPG Filters</CardTitle>
            <Code className="h-4 w-4 text-green-600" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{epgFilters}</div>
            <p className="text-xs text-muted-foreground">EPG processing</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">System Defaults</CardTitle>
            <Settings className="h-4 w-4 text-orange-600" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{systemDefaults}</div>
            <p className="text-xs text-muted-foreground">Default rules</p>
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
                  placeholder="Search filters, operators, fields, values..."
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  className="pl-8"
                  disabled={loading.filters}
                />
              </div>
            </div>
            <Select
              value={filterSourceType}
              onValueChange={(value) => setFilterSourceType(value as 'all' | 'stream' | 'epg')}
              disabled={loading.filters}
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

      {/* Filters Display */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center justify-between">
            <span>
              Filters ({filteredFilters?.length || 0}
              {searchTerm || filterSourceType !== 'all' ? ` of ${allFilters?.length || 0}` : ''})
            </span>
            {loading.filters && <Loader2 className="h-4 w-4 animate-spin" />}
          </CardTitle>
          <CardDescription>Manage filtering rules for streams and EPG data</CardDescription>
        </CardHeader>
        <CardContent>
          {errors.filters ? (
            <Alert variant="destructive">
              <AlertCircle className="h-4 w-4" />
              <AlertTitle>Failed to Load Filters</AlertTitle>
              <AlertDescription>
                {errors.filters}
                <Button
                  variant="outline"
                  size="sm"
                  className="ml-2"
                  onClick={loadFilters}
                  disabled={loading.filters}
                >
                  {loading.filters && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                  Retry
                </Button>
              </AlertDescription>
            </Alert>
          ) : (
            <>
              {viewMode === 'table' && (
                <div className="space-y-4">
                  {filteredFilters?.map((filterWithMeta, index) => {
                    const filter = filterWithMeta.filter;
                    const safeKey = filter?.id
                      ? String(filter.id)
                      : `filter-${index}-${filter?.name || 'unnamed'}`;

                    return (
                      <Card key={safeKey} className="relative">
                        <CardHeader className="pb-3">
                          <div className="flex items-start justify-between">
                            <div className="space-y-2">
                              <div className="flex items-center gap-2">
                                <CardTitle className="text-lg">{filter.name}</CardTitle>
                                <Badge className={getSourceTypeColor(filter.source_type)}>
                                  {filter.source_type.toUpperCase()}
                                </Badge>
                                {filter.is_system_default && (
                                  <Badge variant="outline">System Default</Badge>
                                )}
                                {filter.is_inverse && (
                                  <Badge variant="outline">
                                    <Code className="h-3 w-3 mr-1" />
                                    Inverse
                                  </Badge>
                                )}
                              </div>
                              {filterWithMeta.filter.usage_count > 0 && (
                                <div className="flex items-center gap-1 text-sm text-muted-foreground">
                                  <Hash className="h-3 w-3" />
                                  Used {filterWithMeta.filter.usage_count}x
                                </div>
                              )}
                            </div>
                            <div className="flex items-center gap-2">
                              {filter.expression && (
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() =>
                                    copyFilterExpression(filter.id, filter.expression || '')
                                  }
                                  className="h-8 w-8 p-0"
                                  title="Copy expression to clipboard"
                                >
                                  {copiedItems.has(filter.id) ? (
                                    <Check className="h-4 w-4 text-green-600" />
                                  ) : (
                                    <Copy className="h-4 w-4" />
                                  )}
                                </Button>
                              )}
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => filter?.id && toggleFilterExpansion(filter.id)}
                                className="h-8 w-8 p-0"
                                title={
                                  filter?.id && expandedFilters.has(filter.id)
                                    ? 'Collapse expression tree'
                                    : 'Expand expression tree'
                                }
                                disabled={!filter?.id}
                              >
                                {filter?.id && expandedFilters.has(filter.id) ? (
                                  <ChevronUp className="h-4 w-4" />
                                ) : (
                                  <ChevronDown className="h-4 w-4" />
                                )}
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => {
                                  setEditingFilter(filter);
                                  setIsEditSheetOpen(true);
                                }}
                                className="h-8 w-8 p-0"
                                disabled={!isOnline || filter.is_system_default}
                                title={
                                  filter.is_system_default
                                    ? 'Cannot edit system default filter'
                                    : 'Edit filter'
                                }
                              >
                                <Edit className="h-4 w-4" />
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => handleDeleteFilter(filter.id)}
                                className="h-8 w-8 p-0 text-destructive hover:text-destructive"
                                disabled={
                                  loading.delete === filter.id ||
                                  !isOnline ||
                                  filter.is_system_default
                                }
                                title={
                                  filter.is_system_default
                                    ? 'Cannot delete system default filter'
                                    : 'Delete filter'
                                }
                              >
                                {loading.delete === filter.id ? (
                                  <Loader2 className="h-4 w-4 animate-spin" />
                                ) : (
                                  <Trash2 className="h-4 w-4" />
                                )}
                              </Button>
                            </div>
                          </div>
                        </CardHeader>
                        {filter?.id && expandedFilters.has(filter.id) && (
                          <CardContent className="pt-0">
                            {filter.condition_tree ? (
                              <div className="bg-muted/30 rounded-lg p-4">
                                <ConditionTreeView
                                  conditionTreeJson={filter.condition_tree}
                                  compact={true}
                                  className="text-sm"
                                />
                              </div>
                            ) : (
                              <div className="bg-muted/30 rounded-lg p-4 text-center text-muted-foreground">
                                <FilterIcon className="h-8 w-8 mx-auto mb-2 opacity-50" />
                                <p className="text-sm">No condition tree available</p>
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
                  {filteredFilters?.map((filterWithMeta, index) => {
                    const filter = filterWithMeta.filter;
                    const safeKey = filter?.id
                      ? String(filter.id)
                      : `filter-${index}-${filter?.name || 'unnamed'}`;

                    return (
                      <Card key={safeKey} className="transition-all hover:shadow-md">
                        <CardHeader>
                          <div className="flex items-start justify-between">
                            <div className="space-y-1 flex-1">
                              <CardTitle className="text-lg">{filter.name}</CardTitle>
                              <div className="flex flex-wrap gap-1">
                                <Badge className={getSourceTypeColor(filter.source_type)}>
                                  {filter.source_type.toUpperCase()}
                                </Badge>
                                {filter.is_system_default && (
                                  <Badge variant="outline">System Default</Badge>
                                )}
                                {filter.is_inverse && (
                                  <Badge variant="outline">
                                    <Code className="h-3 w-3 mr-1" />
                                    Inverse
                                  </Badge>
                                )}
                              </div>
                            </div>
                          </div>
                        </CardHeader>
                        <CardContent>
                          <div className="space-y-4">
                            {filterWithMeta.filter.usage_count > 0 && (
                              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                                <Hash className="h-4 w-4" />
                                <span>Used {filterWithMeta.filter.usage_count} times</span>
                              </div>
                            )}

                            <div className="text-xs text-muted-foreground">
                              Created {formatRelativeTime(filter.created_at || '')}
                            </div>

                            <div className="flex items-center justify-between pt-2 border-t">
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => filter?.id && toggleFilterExpansion(filter.id)}
                                className="h-8 px-2 text-xs"
                                disabled={!filter?.id}
                              >
                                {filter?.id && expandedFilters.has(filter.id) ? (
                                  <>
                                    <ChevronUp className="h-3 w-3 mr-1" />
                                    Hide Tree
                                  </>
                                ) : (
                                  <>
                                    <ChevronDown className="h-3 w-3 mr-1" />
                                    Show Tree
                                  </>
                                )}
                              </Button>
                              <div className="flex items-center gap-1">
                                {filter.expression && (
                                  <Button
                                    variant="ghost"
                                    size="sm"
                                    onClick={() =>
                                      copyFilterExpression(filter.id, filter.expression || '')
                                    }
                                    className="h-8 w-8 p-0"
                                    title="Copy expression to clipboard"
                                  >
                                    {copiedItems.has(filter.id) ? (
                                      <Check className="h-4 w-4 text-green-600" />
                                    ) : (
                                      <Copy className="h-4 w-4" />
                                    )}
                                  </Button>
                                )}
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => {
                                    setEditingFilter(filter);
                                    setIsEditSheetOpen(true);
                                  }}
                                  className="h-8 w-8 p-0"
                                  disabled={!isOnline || filter.is_system_default}
                                >
                                  <Edit className="h-4 w-4" />
                                </Button>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => handleDeleteFilter(filter.id)}
                                  className="h-8 w-8 p-0 text-destructive hover:text-destructive"
                                  disabled={
                                    loading.delete === filter.id ||
                                    !isOnline ||
                                    filter.is_system_default
                                  }
                                >
                                  {loading.delete === filter.id ? (
                                    <Loader2 className="h-4 w-4 animate-spin" />
                                  ) : (
                                    <Trash2 className="h-4 w-4" />
                                  )}
                                </Button>
                              </div>
                            </div>
                          </div>
                        </CardContent>
                        {filter?.id && expandedFilters.has(filter.id) && (
                          <CardContent className="pt-0 border-t">
                            {filter.condition_tree ? (
                              <div className="bg-muted/30 rounded-lg p-3">
                                <ConditionTreeView
                                  conditionTreeJson={filter.condition_tree}
                                  compact={true}
                                  className="text-sm"
                                />
                              </div>
                            ) : (
                              <div className="bg-muted/30 rounded-lg p-3 text-center text-muted-foreground">
                                <FilterIcon className="h-6 w-6 mx-auto mb-1 opacity-50" />
                                <p className="text-xs">No condition tree</p>
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
                  {filteredFilters?.map((filterWithMeta, index) => {
                    const filter = filterWithMeta.filter;
                    const safeKey = filter?.id
                      ? String(filter.id)
                      : `filter-${index}-${filter?.name || 'unnamed'}`;

                    return (
                      <Card key={safeKey} className="transition-all hover:shadow-sm">
                        <CardContent className="pt-4">
                          <div className="flex items-center justify-between">
                            <div className="flex items-center space-x-4 flex-1">
                              <div className="flex-1 min-w-0">
                                <div className="flex items-center gap-3">
                                  <div>
                                    <p className="font-medium text-sm">{filter.name}</p>
                                    <p className="text-xs text-muted-foreground">
                                      {filter.expression && filter.expression.length > 50
                                        ? `${filter.expression.substring(0, 50)}...`
                                        : filter.expression || 'No expression'}
                                    </p>
                                  </div>
                                  <div className="flex items-center gap-2">
                                    <Badge className={getSourceTypeColor(filter.source_type)}>
                                      {filter.source_type.toUpperCase()}
                                    </Badge>
                                    {filter.is_system_default && (
                                      <Badge variant="outline" className="text-xs">
                                        Default
                                      </Badge>
                                    )}
                                    {filter.is_inverse && (
                                      <Badge variant="outline" className="text-xs">
                                        <Code className="h-3 w-3 mr-1" />
                                        Inverse
                                      </Badge>
                                    )}
                                    {filterWithMeta.filter.usage_count > 0 && (
                                      <Badge variant="secondary" className="text-xs">
                                        <Hash className="h-3 w-3 mr-1" />
                                        {filterWithMeta.filter.usage_count}x
                                      </Badge>
                                    )}
                                  </div>
                                </div>
                              </div>
                            </div>
                            <div className="flex items-center gap-2 ml-4">
                              <div className="flex items-center gap-1">
                                {filter.expression && (
                                  <Button
                                    variant="ghost"
                                    size="sm"
                                    onClick={() =>
                                      copyFilterExpression(filter.id, filter.expression || '')
                                    }
                                    className="h-8 w-8 p-0"
                                    title="Copy expression to clipboard"
                                  >
                                    {copiedItems.has(filter.id) ? (
                                      <Check className="h-4 w-4 text-green-600" />
                                    ) : (
                                      <Copy className="h-4 w-4" />
                                    )}
                                  </Button>
                                )}
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => filter?.id && toggleFilterExpansion(filter.id)}
                                  className="h-8 w-8 p-0"
                                  disabled={!filter?.id}
                                >
                                  {filter?.id && expandedFilters.has(filter.id) ? (
                                    <ChevronUp className="h-4 w-4" />
                                  ) : (
                                    <ChevronDown className="h-4 w-4" />
                                  )}
                                </Button>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => {
                                    setEditingFilter(filter);
                                    setIsEditSheetOpen(true);
                                  }}
                                  className="h-8 w-8 p-0"
                                  disabled={!isOnline || filter.is_system_default}
                                >
                                  <Edit className="h-4 w-4" />
                                </Button>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => handleDeleteFilter(filter.id)}
                                  className="h-8 w-8 p-0 text-destructive hover:text-destructive"
                                  disabled={
                                    loading.delete === filter.id ||
                                    !isOnline ||
                                    filter.is_system_default
                                  }
                                >
                                  {loading.delete === filter.id ? (
                                    <Loader2 className="h-4 w-4 animate-spin" />
                                  ) : (
                                    <Trash2 className="h-4 w-4" />
                                  )}
                                </Button>
                              </div>
                            </div>
                          </div>
                          {filter?.id && expandedFilters.has(filter.id) && (
                            <div className="mt-4 pt-4 border-t">
                              {filter.condition_tree ? (
                                <div className="bg-muted/30 rounded-lg p-3">
                                  <ConditionTreeView
                                    conditionTreeJson={filter.condition_tree}
                                    compact={true}
                                    className="text-sm"
                                  />
                                </div>
                              ) : (
                                <div className="bg-muted/30 rounded-lg p-3 text-center text-muted-foreground">
                                  <FilterIcon className="h-6 w-6 mx-auto mb-1 opacity-50" />
                                  <p className="text-xs">No condition tree available</p>
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

              {filteredFilters?.length === 0 && !loading.filters && (
                <div className="text-center py-8">
                  <FilterIcon className="mx-auto h-12 w-12 text-muted-foreground" />
                  <h3 className="mt-4 text-lg font-semibold">
                    {searchTerm || filterSourceType !== 'all'
                      ? 'No matching filters'
                      : 'No filters found'}
                  </h3>
                  <p className="text-muted-foreground">
                    {searchTerm || filterSourceType !== 'all'
                      ? 'Try adjusting your search or filter criteria.'
                      : 'Get started by creating your first filter rule.'}
                  </p>
                </div>
              )}
            </>
          )}
        </CardContent>
      </Card>

      {/* Edit Filter Sheet */}
      {editingFilter && (
        <EditFilterSheet
          filter={editingFilter}
          onUpdateFilter={handleUpdateFilter}
          loading={loading.edit}
          error={errors.edit}
          open={isEditSheetOpen}
          onOpenChange={setIsEditSheetOpen}
        />
      )}
    </div>
  );
}
