'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { FilterExpressionEditor } from '@/components/filter-expression-editor';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Plus,
  Trash2,
  Filter as FilterIcon,
  AlertCircle,
  Loader2,
  WifiOff,
  Code,
  Play,
  Lock,
} from 'lucide-react';
import { Filter, FilterAction } from '@/types/api';
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
import { StatCard } from '@/components/shared/feedback/StatCard';

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

// Convert Filter to MasterItem format for MasterDetailLayout
interface FilterMasterItem extends MasterItem {
  filter: Filter;
}

function filterToMasterItem(filter: Filter): FilterMasterItem {
  return {
    id: filter.id,
    title: filter.name,
    filter,
  };
}

// Create panel for creating a new filter inline in detail area
function FilterCreatePanel({
  onCreate,
  onCancel,
  loading,
  error,
}: {
  onCreate: (filter: Omit<Filter, 'id' | 'created_at' | 'updated_at'>) => Promise<void>;
  onCancel: () => void;
  loading: boolean;
  error: string | null;
}) {
  const [formData, setFormData] = useState<Omit<Filter, 'id' | 'created_at' | 'updated_at'>>({
    name: '',
    source_type: 'stream',
    action: 'include',
    expression: '',
  });
  const [filterExpression, setFilterExpression] = useState('');

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
      title="Create Filter"
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
            <Label htmlFor="create-name">Name</Label>
            <Input
              id="create-name"
              value={formData.name}
              onChange={(e) => setFormData({ ...formData, name: e.target.value })}
              placeholder="My Filter"
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

          <div className="space-y-2">
            <Label htmlFor="create-action">Action</Label>
            <select
              id="create-action"
              value={formData.action}
              onChange={(e) =>
                setFormData({ ...formData, action: e.target.value as FilterAction })
              }
              className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
              required
              disabled={loading}
            >
              <option value="include">Include</option>
              <option value="exclude">Exclude</option>
            </select>
          </div>
        </form>
      </div>
    </DetailPanel>
  );
}

// Detail panel for viewing/editing a selected filter
function FilterDetailPanel({
  filter,
  onUpdate,
  onDelete,
  loading,
  error,
  isOnline,
}: {
  filter: Filter;
  onUpdate: (id: string, data: Omit<Filter, 'id' | 'created_at' | 'updated_at'>) => Promise<void>;
  onDelete: (filter: Filter) => void;
  loading: { edit: boolean; delete: string | null };
  error: string | null;
  isOnline: boolean;
}) {
  const [formData, setFormData] = useState<Omit<Filter, 'id' | 'created_at' | 'updated_at'>>({
    name: filter.name,
    description: filter.description,
    source_type: filter.source_type,
    action: filter.action,
    expression: filter.expression,
  });
  const [filterExpression, setFilterExpression] = useState(filter.expression || '');
  const [hasChanges, setHasChanges] = useState(false);

  // Reset form when filter changes
  useEffect(() => {
    const newFormData = {
      name: filter.name,
      description: filter.description,
      source_type: filter.source_type,
      action: filter.action,
      expression: filter.expression,
    };
    setFormData(newFormData);
    setFilterExpression(filter.expression || '');
    setHasChanges(false);
  }, [filter.id]);

  const handleFieldChange = (field: keyof typeof formData, value: string) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
    setHasChanges(true);
  };

  const handleSave = async () => {
    await onUpdate(filter.id, formData);
    setHasChanges(false);
  };

  const isSystem = filter.is_system;

  return (
    <DetailPanel
      title={filter.name}
      actions={
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => onDelete(filter)}
            disabled={loading.delete === filter.id || !isOnline || isSystem}
            className="text-destructive hover:text-destructive"
            title={isSystem ? "System filters cannot be deleted" : "Delete filter"}
          >
            {loading.delete === filter.id ? (
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
            <AlertTitle>System Filter</AlertTitle>
            <AlertDescription>
              This is a system filter and cannot be modified or deleted.
            </AlertDescription>
          </Alert>
        )}

        {/* Compact Filter Info */}
        <div className="flex flex-wrap gap-2 text-sm">
          <Badge variant="secondary">{filter.source_type.toUpperCase()}</Badge>
          <Badge variant={filter.action === 'exclude' ? 'destructive' : 'default'}>
            {filter.action.toUpperCase()}
          </Badge>
          <span className="text-muted-foreground">
            Created {filter.created_at ? formatRelativeTime(filter.created_at) : 'Unknown'}
          </span>
        </div>

        {/* Edit Form */}
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="detail-name">Name</Label>
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
              onChange={(e) => handleFieldChange('source_type', e.target.value as 'stream' | 'epg')}
              className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
              disabled={loading.edit || !isOnline || isSystem}
            >
              <option value="stream">Stream</option>
              <option value="epg">EPG</option>
            </select>
          </div>

          <FilterExpressionEditor
            key={filter.id}
            value={filterExpression}
            onChange={(value) => {
              setFilterExpression(value);
              handleFieldChange('expression', value);
            }}
            sourceType={formData.source_type}
            placeholder='Enter filter expression (e.g., channel_name contains "sport")'
            disabled={loading.edit || !isOnline || isSystem}
            showTestResults={true}
            autoTest={true}
          />

          <div className="space-y-2">
            <Label htmlFor="detail-action">Action</Label>
            <select
              id="detail-action"
              value={formData.action}
              onChange={(e) => handleFieldChange('action', e.target.value as FilterAction)}
              className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
              disabled={loading.edit || !isOnline || isSystem}
            >
              <option value="include">Include</option>
              <option value="exclude">Exclude</option>
            </select>
          </div>

          {/* Save Button */}
          {hasChanges && !isSystem && (
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

export function Filters() {
  const [allFilters, setAllFilters] = useState<Filter[]>([]);
  const [pagination, setPagination] = useState<{ total: number } | null>(null);

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

  const [deleteDialog, setDeleteDialog] = useState<{ open: boolean; filter: Filter | null }>({
    open: false,
    filter: null,
  });

  const [selectedFilter, setSelectedFilter] = useState<FilterMasterItem | null>(null);
  const [isOnline, setIsOnline] = useState(true);
  const [isCreating, setIsCreating] = useState(false);

  // Sort filters alphabetically by name
  const sortedFilters = useMemo(() => {
    return [...allFilters].sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true }));
  }, [allFilters]);

  // Convert filters to master items for the layout
  const masterItems = useMemo(
    () => sortedFilters.map(filterToMasterItem),
    [sortedFilters]
  );

  const loadFilters = useCallback(async () => {
    if (!isOnline) return;

    setLoading((prev) => ({ ...prev, filters: true }));
    setErrors((prev) => ({ ...prev, filters: null }));

    try {
      const filters = await apiClient.getFilters();

      // Filter out any malformed filter objects that might cause React key issues
      const validFilters = filters.filter((filter) => {
        if (!filter?.id || typeof filter.id !== 'string') {
          console.warn('Invalid filter object - missing or invalid ID:', filter);
          return false;
        }
        return true;
      });

      if (validFilters.length !== filters.length) {
        console.warn(`Filtered out ${filters.length - validFilters.length} invalid filter(s)`);
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
      const response = await apiClient.createFilter(newFilter);
      await loadFilters(); // Reload filters after creation
      setIsCreating(false);
      // Select the newly created filter
      if (response?.data?.id) {
        const newMasterItem = filterToMasterItem(response.data);
        setSelectedFilter(newMasterItem);
      }
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
      throw error;
    } finally {
      setLoading((prev) => ({ ...prev, edit: false }));
    }
  };

  const handleDeleteFilter = async (filter: Filter) => {
    setLoading((prev) => ({ ...prev, delete: filter.id }));
    setErrors((prev) => ({ ...prev, action: null }));

    try {
      await apiClient.deleteFilter(filter.id);
      await loadFilters(); // Reload filters after deletion
      // Clear selection if the deleted filter was selected
      if (selectedFilter?.filter.id === filter.id) {
        setSelectedFilter(null);
      }
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        action: `Failed to delete filter: ${apiError.message}`,
      }));
    } finally {
      setLoading((prev) => ({ ...prev, delete: null }));
      setDeleteDialog({ open: false, filter: null });
    }
  };

  const streamFilters =
    allFilters?.filter((f) => f.source_type.toLowerCase() === 'stream').length || 0;
  const epgFilters =
    allFilters?.filter((f) => f.source_type.toLowerCase() === 'epg').length || 0;
  const systemFilters = allFilters?.filter((f) => f.is_system).length || 0;
  const totalFilters = allFilters?.length || 0;

  return (
    <div className="flex flex-col gap-6 h-full">
      {/* Header Section */}
      <div className="flex items-center justify-between">
        <div>
          <p className="text-muted-foreground">Manage filtering rules</p>
        </div>
        <div className="flex items-center gap-2">
          {!isOnline && <WifiOff className="h-5 w-5 text-destructive" />}
          <ImportDialog
            importType="filters"
            title="Import Filters"
            onImportComplete={loadFilters}
          />
          <ExportDialog
            exportType="filters"
            items={allFilters.map((f) => ({ id: f.id, name: f.name, is_system: f.is_system }))}
            title="Export Filters"
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
        <StatCard title="Total Filters" value={totalFilters} icon={<FilterIcon className="h-4 w-4" />} />
        <StatCard title="Stream Filters" value={streamFilters} icon={<Play className="h-4 w-4 text-blue-600" />} />
        <StatCard title="EPG Filters" value={epgFilters} icon={<Code className="h-4 w-4 text-green-600" />} />
        <StatCard title="System" value={systemFilters} icon={<Lock className="h-4 w-4 text-orange-600" />} />
      </div>

      {/* Error Loading Filters */}
      {errors.filters && (
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
      )}

      {/* Master-Detail Layout */}
      <Card className="flex-1 overflow-hidden min-h-0">
        <CardContent className="p-0 h-full">
          <MasterDetailLayout
            items={masterItems}
            selectedId={selectedFilter?.id}
            onSelect={(item) => {
              setSelectedFilter(item);
              if (item) setIsCreating(false);
            }}
            isLoading={loading.filters}
            title={`Filters (${sortedFilters.length})`}
            searchPlaceholder="Search by name, type, action..."
            headerAction={
              <Button
                size="sm"
                variant="ghost"
                onClick={() => {
                  setIsCreating(true);
                  setSelectedFilter(null);
                  setErrors((prev) => ({ ...prev, create: null }));
                }}
                disabled={isCreating}
              >
                <Plus className="h-4 w-4" />
                <span className="sr-only">Create Filter</span>
              </Button>
            }
            emptyState={{
              title: 'No filters configured',
              description: 'Get started by creating your first filter rule.',
            }}
            filterFn={createFuzzyFilter<FilterMasterItem>({
              keys: [
                { name: 'name', weight: 0.35 },
                { name: 'expression', weight: 0.2 },
                { name: 'description', weight: 0.2 },
                { name: 'source_type', weight: 0.1 },
                { name: 'action', weight: 0.1 },
                { name: 'system', weight: 0.05 },
              ],
              accessor: (item) => ({
                name: item.filter.name,
                expression: item.filter.expression || '',
                description: item.filter.description || '',
                source_type: item.filter.source_type,
                action: item.filter.action,
                system: item.filter.is_system ? 'system' : '',
              }),
            })}
          >
            {(selected) =>
              isCreating ? (
                <FilterCreatePanel
                  onCreate={handleCreateFilter}
                  onCancel={() => setIsCreating(false)}
                  loading={loading.create}
                  error={errors.create}
                />
              ) : selected ? (
                <FilterDetailPanel
                  filter={selected.filter}
                  onUpdate={handleUpdateFilter}
                  onDelete={(filter) => setDeleteDialog({ open: true, filter })}
                  loading={{ edit: loading.edit, delete: loading.delete }}
                  error={errors.edit}
                  isOnline={isOnline}
                />
              ) : (
                <DetailEmpty
                  title="Select a filter"
                  description="Choose a filter from the list to view details and edit configuration"
                  icon={<FilterIcon className="h-12 w-12 text-muted-foreground" />}
                />
              )
            }
          </MasterDetailLayout>
        </CardContent>
      </Card>

      {/* Delete confirmation dialog */}
      <Dialog open={deleteDialog.open} onOpenChange={(open) => setDeleteDialog({ open, filter: null })}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Filter</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete &quot;{deleteDialog.filter?.name}&quot;? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteDialog({ open: false, filter: null })}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => deleteDialog.filter && handleDeleteFilter(deleteDialog.filter)}
              disabled={loading.delete !== null}
            >
              {loading.delete ? <Loader2 className="h-4 w-4 animate-spin mr-1" /> : null}
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
