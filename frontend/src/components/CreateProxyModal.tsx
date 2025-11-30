'use client';

import { useState, useEffect } from 'react';
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Button } from '@/components/ui/button';
import { Switch } from '@/components/ui/switch';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Label } from '@/components/ui/label';
import { Plus, GripVertical, Trash2, AlertCircle, Loader2, ArrowUp, ArrowDown } from 'lucide-react';
import { getBackendUrl } from '@/lib/config';
import { apiClient } from '@/lib/api-client';
import { StreamProxy } from '@/types/api';

// Types based on your API specification
interface StreamSourceResponse {
  id: string;
  name: string;
  source_type: string;
  url: string;
  is_active: boolean;
}

interface EpgSourceResponse {
  id: string;
  name: string;
  url: string;
  is_active: boolean;
}

interface FilterResponse {
  id: string;
  name: string;
  source_type: string;
  is_inverse: boolean;
  is_system_default: boolean;
  is_active?: boolean;
}

interface RelayProfileResponse {
  id: string;
  name: string;
  description?: string;
}

interface StreamSourceAssignment {
  source_id: string;
  priority_order: number;
}

interface EpgSourceAssignment {
  epg_source_id: string;
  priority_order: number;
}

interface FilterAssignment {
  filter_id: string;
  priority_order: number;
  is_active: boolean;
}

interface ProxyFormData {
  name: string;
  description?: string;
  proxy_mode: 'redirect' | 'proxy' | 'relay';
  upstream_timeout: number;
  max_concurrent_streams: number;
  starting_channel_number: number;
  stream_sources: StreamSourceAssignment[];
  epg_sources: EpgSourceAssignment[];
  filters: FilterAssignment[];
  is_active: boolean;
  auto_regenerate: boolean;
  cache_channel_logos: boolean;
  cache_program_logos: boolean;
  relay_profile_id?: string;
}

// Multi-select modal component
interface MultiSelectModalProps {
  title: string;
  type: 'stream' | 'epg' | 'filter';
  selectedIds: string[];
  onConfirm: (selectedIds: string[]) => void;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

function MultiSelectModal({
  title,
  type,
  selectedIds,
  onConfirm,
  open,
  onOpenChange,
}: MultiSelectModalProps) {
  const [tempSelected, setTempSelected] = useState<string[]>([]);
  const [sources, setSources] = useState<any[]>([]);
  const [loading, setLoading] = useState(false);

  // Load sources when modal opens
  useEffect(() => {
    if (open) {
      setTempSelected([]);
      loadSources();
    }
  }, [open, type]);

  const loadSources = async () => {
    setLoading(true);
    try {
      let data: any[] = [];

      if (type === 'stream') {
        const response = await apiClient.getStreamSources();
        data = response.items || [];
      } else if (type === 'epg') {
        const response = await apiClient.getEpgSources();
        data = response.items || [];
      } else if (type === 'filter') {
        const response = await apiClient.getFilters();
        // Handle the nested filter structure from API
        data = Array.isArray(response)
          ? response.map((item) => ({
              id: item.filter.id,
              name: item.filter.name,
              source_type: item.filter.source_type,
              is_inverse: item.filter.is_inverse,
              is_system_default: item.filter.is_system_default,
              is_active: true,
            }))
          : [];
      }

      setSources(data);
    } catch (error) {
      console.error(`Failed to load ${type} sources:`, error);
      setSources([]);
    } finally {
      setLoading(false);
    }
  };

  const availableSources = sources.filter((source) => !selectedIds.includes(source.id));

  const handleToggleSelection = (sourceId: string) => {
    setTempSelected((prev) =>
      prev.includes(sourceId) ? prev.filter((id) => id !== sourceId) : [...prev, sourceId]
    );
  };

  const handleConfirm = () => {
    onConfirm(tempSelected);
    onOpenChange(false);
  };

  const getSourceTypeLabel = (source: any) => {
    if (type === 'filter') {
      const labels = [];
      if (source.source_type) {
        labels.push(source.source_type.toUpperCase());
      }
      if (source.is_inverse) {
        labels.push('Inverse');
      }
      if (source.is_system_default) {
        labels.push('System Default');
      }
      return labels;
    } else {
      return [source.source_type?.toUpperCase() || 'Unknown'];
    }
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-full sm:max-w-lg overflow-y-auto">
        <SheetHeader>
          <SheetTitle>Add {title}</SheetTitle>
          <SheetDescription>
            Select multiple {title.toLowerCase()} to add to your proxy
          </SheetDescription>
        </SheetHeader>

        <div className="space-y-4">
          {loading ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin" />
              <span className="ml-2 text-sm text-muted-foreground">
                Loading {title.toLowerCase()}...
              </span>
            </div>
          ) : availableSources.length === 0 ? (
            <Card className="border-dashed border-2">
              <CardContent className="flex flex-col items-center justify-center py-8">
                <p className="text-sm text-muted-foreground text-center">
                  No available {title.toLowerCase()} to add.
                  {selectedIds.length > 0 && ` All available items are already selected.`}
                </p>
              </CardContent>
            </Card>
          ) : (
            <div className="space-y-2">
              <div className="text-sm text-muted-foreground mb-3">
                {tempSelected.length} of {availableSources.length} selected
              </div>
              {availableSources.map((source) => {
                const isSelected = tempSelected.includes(source.id);
                const labels = getSourceTypeLabel(source);
                return (
                  <Card
                    key={source.id}
                    className={`cursor-pointer transition-colors ${
                      isSelected ? 'ring-2 ring-primary bg-primary/5' : 'hover:bg-muted/50'
                    }`}
                    onClick={() => handleToggleSelection(source.id)}
                  >
                    <CardContent className="p-3">
                      <div className="flex items-center gap-3">
                        <input
                          type="checkbox"
                          checked={isSelected}
                          onChange={() => handleToggleSelection(source.id)}
                          className="rounded border-gray-300"
                          onClick={(e) => e.stopPropagation()}
                        />
                        <div className="flex-1">
                          <div className="text-sm font-medium">{source.name}</div>
                          <div className="flex gap-1 mt-1">
                            {labels.map((label, index) => (
                              <Badge
                                key={index}
                                variant={
                                  label === 'Inverse' || label === 'System Default'
                                    ? 'outline'
                                    : 'secondary'
                                }
                                className="text-xs"
                              >
                                {label}
                              </Badge>
                            ))}
                          </div>
                        </div>
                        {source.is_active !== undefined && (
                          <Badge
                            variant={source.is_active ? 'default' : 'secondary'}
                            className="text-xs"
                          >
                            {source.is_active ? 'Active' : 'Inactive'}
                          </Badge>
                        )}
                      </div>
                    </CardContent>
                  </Card>
                );
              })}
            </div>
          )}
        </div>

        <SheetFooter className="gap-2">
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={handleConfirm} disabled={tempSelected.length === 0 || loading}>
            Add {tempSelected.length} {title}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}

// Assigned items list with reordering
interface AssignedItemsListProps {
  title: string;
  items: Array<{ priority_order: number; is_active?: boolean }>;
  onRemove: (index: number) => void;
  onReorder: (index: number, direction: 'up' | 'down') => void;
  onToggleActive?: (index: number) => void;
  onAddItems: () => void;
  getSourceId: (item: any) => string;
  type: 'stream' | 'epg' | 'filter';
}

function AssignedItemsList({
  title,
  items,
  onRemove,
  onReorder,
  onToggleActive,
  onAddItems,
  getSourceId,
  type,
}: AssignedItemsListProps) {
  const [sourceNames, setSourceNames] = useState<Record<string, string>>({});
  const itemsArray = Array.isArray(items) ? items : [];

  // Load source names for display
  useEffect(() => {
    const loadSourceNames = async () => {
      if (itemsArray.length === 0) return;

      try {
        let sources: any[] = [];

        if (type === 'stream') {
          const response = await apiClient.getStreamSources();
          sources = response.items || [];
        } else if (type === 'epg') {
          const response = await apiClient.getEpgSources();
          sources = response.items || [];
        } else if (type === 'filter') {
          const response = await apiClient.getFilters();
          sources = Array.isArray(response)
            ? response.map((item) => ({
                id: item.filter.id,
                name: item.filter.name,
              }))
            : [];
        }

        const nameMap: Record<string, string> = {};
        sources.forEach((source) => {
          nameMap[source.id] = source.name;
        });
        setSourceNames(nameMap);
      } catch (error) {
        console.error(`Failed to load ${type} source names:`, error);
      }
    };

    loadSourceNames();
  }, [itemsArray.length, type]);

  const getSourceName = (id: string) => {
    return sourceNames[id] || `Loading... (${id.slice(0, 8)})`;
  };

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <Label className="text-sm font-medium">
          {title} ({itemsArray.length})
        </Label>
        <Button type="button" variant="outline" size="sm" onClick={onAddItems} className="gap-2">
          <Plus className="h-4 w-4" />
          Add {title}
        </Button>
      </div>

      {itemsArray.length === 0 ? (
        <Card className="border-dashed border-2">
          <CardContent className="flex flex-col items-center justify-center py-6">
            <p className="text-xs text-muted-foreground mb-2">No {title.toLowerCase()} added</p>
            <Button type="button" variant="ghost" size="sm" onClick={onAddItems} className="gap-2">
              <Plus className="h-4 w-4" />
              Add {title}
            </Button>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-2">
          {itemsArray.map((item, index) => (
            <Card key={`${getSourceId(item)}-${index}`}>
              <CardContent className="flex items-center gap-3 p-3">
                <div className="flex flex-col gap-1">
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => onReorder(index, 'up')}
                    disabled={index === 0}
                    className="h-5 w-5 p-0"
                  >
                    <ArrowUp className="h-3 w-3" />
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => onReorder(index, 'down')}
                    disabled={index === itemsArray.length - 1}
                    className="h-5 w-5 p-0"
                  >
                    <ArrowDown className="h-3 w-3" />
                  </Button>
                </div>
                <div className="flex-1">
                  <div className="text-sm font-medium">{getSourceName(getSourceId(item))}</div>
                  <div className="text-xs text-muted-foreground">
                    Priority: {item.priority_order}
                  </div>
                </div>
                {onToggleActive && item.hasOwnProperty('is_active') && (
                  <Switch checked={item.is_active} onCheckedChange={() => onToggleActive(index)} />
                )}
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => onRemove(index)}
                  className="h-8 w-8 p-0 text-destructive hover:text-destructive"
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}

// Main component - now handles both create and edit
interface ProxySheetProps {
  mode: 'create' | 'edit';
  proxy?: StreamProxy | null;
  onSaveProxy: (proxy: ProxyFormData, proxyId?: string) => Promise<void>;
  loading: boolean;
  error: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function ProxySheet({
  mode,
  proxy,
  onSaveProxy,
  loading,
  error,
  open,
  onOpenChange,
}: ProxySheetProps) {
  const [relayProfiles, setRelayProfiles] = useState<RelayProfileResponse[]>([]);

  // Modal states
  const [streamSourceModalOpen, setStreamSourceModalOpen] = useState(false);
  const [epgSourceModalOpen, setEpgSourceModalOpen] = useState(false);
  const [filterModalOpen, setFilterModalOpen] = useState(false);

  // Form state
  const [formData, setFormData] = useState<ProxyFormData>({
    name: '',
    description: '',
    proxy_mode: 'proxy',
    upstream_timeout: 30,
    max_concurrent_streams: 10,
    starting_channel_number: 1,
    stream_sources: [],
    epg_sources: [],
    filters: [],
    is_active: true,
    auto_regenerate: true,
    cache_channel_logos: true,
    cache_program_logos: false,
  });

  // Load relay profiles and proxy data when modal opens
  useEffect(() => {
    if (open) {
      const loadData = async () => {
        try {
          // Load relay profiles
          const relayProfiles = await apiClient.getRelayProfiles();
          console.log('Relay profiles data:', relayProfiles);
          setRelayProfiles(Array.isArray(relayProfiles) ? relayProfiles : []);

          // If editing, load existing proxy data
          if (mode === 'edit' && proxy) {
            // Get detailed proxy information with relationships
            let streamSources: any[] = [];
            let epgSources: any[] = [];
            let filters: any[] = [];
            let detailedProxy: any = null;

            try {
              const proxyResponse = await apiClient.getProxy(proxy.id);
              detailedProxy = proxyResponse.data || proxyResponse;

              if (detailedProxy && typeof detailedProxy === 'object') {
                const proxyData = detailedProxy as any;
                console.log('Detailed proxy data:', proxyData);

                // Extract relationships from proxy response
                if (proxyData.stream_sources && Array.isArray(proxyData.stream_sources)) {
                  streamSources = proxyData.stream_sources.map((source: any) => ({
                    source_id: source.source_id,
                    priority_order: source.priority_order,
                  }));
                }
                if (proxyData.epg_sources && Array.isArray(proxyData.epg_sources)) {
                  epgSources = proxyData.epg_sources.map((source: any) => ({
                    epg_source_id: source.epg_source_id,
                    priority_order: source.priority_order,
                  }));
                }
                if (proxyData.filters && Array.isArray(proxyData.filters)) {
                  filters = proxyData.filters.map((filter: any) => ({
                    filter_id: filter.filter_id,
                    priority_order: filter.priority_order,
                    is_active: filter.is_active,
                  }));
                }
              }
            } catch (proxyError) {
              console.error('Failed to load detailed proxy data:', proxyError);
            }

            console.log('Loaded proxy associations:', {
              streamSources,
              epgSources,
              filters,
            });

            // Update form data with proxy values and loaded associations
            // Use the detailed proxy data from API response if available, otherwise fall back to input proxy
            const sourceProxyData = detailedProxy || proxy;

            setFormData({
              name: sourceProxyData.name,
              description: sourceProxyData.description || '',
              proxy_mode: sourceProxyData.proxy_mode as 'redirect' | 'proxy' | 'relay',
              upstream_timeout: sourceProxyData.upstream_timeout || 30,
              max_concurrent_streams: sourceProxyData.max_concurrent_streams || 10,
              starting_channel_number: sourceProxyData.starting_channel_number,
              stream_sources: streamSources,
              epg_sources: epgSources,
              filters: filters,
              is_active: sourceProxyData.is_active,
              auto_regenerate: sourceProxyData.auto_regenerate,
              cache_channel_logos: sourceProxyData.cache_channel_logos,
              cache_program_logos: sourceProxyData.cache_program_logos,
              relay_profile_id: sourceProxyData.relay_profile_id || '',
            });
          } else {
            // Reset form for create mode
            setFormData({
              name: '',
              description: '',
              proxy_mode: 'proxy',
              upstream_timeout: 30,
              max_concurrent_streams: 10,
              starting_channel_number: 1,
              stream_sources: [],
              epg_sources: [],
              filters: [],
              is_active: true,
              auto_regenerate: true,
              cache_channel_logos: true,
              cache_program_logos: false,
            });
          }
        } catch (error) {
          console.error('Failed to load proxy data:', error);
          setRelayProfiles([]);
        }
      };

      loadData();
    }
  }, [open, mode, proxy]);

  const addStreamSources = (sourceIds: string[]) => {
    const orders = formData.stream_sources.map((s) => s.priority_order);
    const maxOrder = orders.length > 0 ? Math.max(...orders) : 0;

    const newSources = sourceIds.map((sourceId, index) => ({
      source_id: sourceId,
      priority_order: maxOrder + index + 1,
    }));

    setFormData((prev) => ({
      ...prev,
      stream_sources: [...prev.stream_sources, ...newSources],
    }));
  };

  const addEpgSources = (sourceIds: string[]) => {
    const orders = formData.epg_sources.map((s) => s.priority_order);
    const maxOrder = orders.length > 0 ? Math.max(...orders) : 0;

    const newSources = sourceIds.map((sourceId, index) => ({
      epg_source_id: sourceId,
      priority_order: maxOrder + index + 1,
    }));

    setFormData((prev) => ({
      ...prev,
      epg_sources: [...prev.epg_sources, ...newSources],
    }));
  };

  const addFilters = (filterIds: string[]) => {
    const orders = formData.filters.map((f) => f.priority_order);
    const maxOrder = orders.length > 0 ? Math.max(...orders) : 0;

    const newFilters = filterIds.map((filterId, index) => ({
      filter_id: filterId,
      priority_order: maxOrder + index + 1,
      is_active: true,
    }));

    setFormData((prev) => ({
      ...prev,
      filters: [...prev.filters, ...newFilters],
    }));
  };

  const reorderItems = <T extends { priority_order: number }>(
    items: T[],
    index: number,
    direction: 'up' | 'down'
  ): T[] => {
    const newItems = [...items];
    const targetIndex = direction === 'up' ? index - 1 : index + 1;

    if (targetIndex < 0 || targetIndex >= items.length) return newItems;

    // Swap items
    const temp = newItems[index];
    newItems[index] = newItems[targetIndex];
    newItems[targetIndex] = temp;

    // Update priority orders
    return newItems.map((item, i) => ({
      ...item,
      priority_order: i + 1,
    }));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!formData.name.trim()) {
      return;
    }

    try {
      await onSaveProxy(formData, mode === 'edit' ? proxy?.id : undefined);
      if (!error) {
        onOpenChange(false);
        // Reset form only for create mode
        if (mode === 'create') {
          setFormData({
            name: '',
            description: '',
            proxy_mode: 'proxy',
            upstream_timeout: 30,
            max_concurrent_streams: 10,
            starting_channel_number: 1,
            stream_sources: [],
            epg_sources: [],
            filters: [],
            is_active: true,
            auto_regenerate: true,
            cache_channel_logos: true,
            cache_program_logos: false,
          });
        }
      }
    } catch (err) {
      // Error handled by parent
    }
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-full sm:max-w-2xl overflow-y-auto">
        <SheetHeader>
          <SheetTitle>{mode === 'create' ? 'Create Stream Proxy' : 'Edit Stream Proxy'}</SheetTitle>
          <SheetDescription>
            {mode === 'create'
              ? 'Configure a new stream proxy with sources, EPG, and filters'
              : 'Update the stream proxy configuration'}
          </SheetDescription>
        </SheetHeader>

        {error && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <form onSubmit={handleSubmit} className="space-y-6 px-4">
          {/* Basic Settings */}
          <div className="space-y-4">
            <div className="grid grid-cols-1 gap-4">
              <div className="space-y-2">
                <Label htmlFor="name">Name</Label>
                <Input
                  id="name"
                  value={formData.name}
                  onChange={(e) => setFormData((prev) => ({ ...prev, name: e.target.value }))}
                  placeholder="My IPTV Proxy"
                  required
                />
              </div>

              <div className="space-y-4">
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label htmlFor="proxy_mode">Proxy Mode</Label>
                    <Select
                      value={formData.proxy_mode}
                      onValueChange={(value: 'redirect' | 'proxy' | 'relay') => {
                        setFormData((prev) => ({
                          ...prev,
                          proxy_mode: value,
                          relay_profile_id: value !== 'relay' ? undefined : prev.relay_profile_id,
                        }));
                      }}
                    >
                      <SelectTrigger>
                        <SelectValue placeholder="Select proxy mode" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="redirect">Redirect</SelectItem>
                        <SelectItem value="proxy">Proxy</SelectItem>
                        <SelectItem value="relay">Relay</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>

                {/* Relay Profile Selection - only show when proxy mode is relay */}
                {formData.proxy_mode === 'relay' && (
                  <div className="space-y-2">
                    <Label htmlFor="relay_profile">Relay Profile</Label>
                    <Select
                      value={formData.relay_profile_id || ''}
                      onValueChange={(value) =>
                        setFormData((prev) => ({ ...prev, relay_profile_id: value }))
                      }
                      required
                    >
                      <SelectTrigger>
                        <SelectValue placeholder="Select relay profile" />
                      </SelectTrigger>
                      <SelectContent>
                        {relayProfiles.map((profile) => (
                          <SelectItem key={profile.id} value={profile.id}>
                            {profile.name}
                            {profile.description && (
                              <span className="text-xs text-muted-foreground ml-2">
                                - {profile.description}
                              </span>
                            )}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    {relayProfiles.length === 0 && (
                      <p className="text-xs text-muted-foreground">
                        No relay profiles available. Please create a relay profile first.
                      </p>
                    )}
                  </div>
                )}
              </div>
            </div>

            <div className="space-y-2">
              <Label htmlFor="description">Description</Label>
              <Textarea
                id="description"
                value={formData.description || ''}
                onChange={(e) => setFormData((prev) => ({ ...prev, description: e.target.value }))}
                placeholder="Optional description"
                className="resize-none"
                rows={2}
              />
            </div>
          </div>

          {/* Stream Sources */}
          <div className="space-y-4">
            <h3 className="text-lg font-medium">Stream Sources</h3>
            <AssignedItemsList
              title="Stream Sources"
              items={formData.stream_sources}
              type="stream"
              onAddItems={() => setStreamSourceModalOpen(true)}
              onRemove={(index) => {
                setFormData((prev) => ({
                  ...prev,
                  stream_sources: prev.stream_sources.filter((_, i) => i !== index),
                }));
              }}
              onReorder={(index, direction) => {
                setFormData((prev) => ({
                  ...prev,
                  stream_sources: reorderItems(prev.stream_sources, index, direction),
                }));
              }}
              getSourceId={(item) => item.source_id}
            />
          </div>

          {/* EPG Sources */}
          <div className="space-y-4">
            <h3 className="text-lg font-medium">EPG Sources</h3>
            <AssignedItemsList
              title="EPG Sources"
              items={formData.epg_sources}
              type="epg"
              onAddItems={() => setEpgSourceModalOpen(true)}
              onRemove={(index) => {
                setFormData((prev) => ({
                  ...prev,
                  epg_sources: prev.epg_sources.filter((_, i) => i !== index),
                }));
              }}
              onReorder={(index, direction) => {
                setFormData((prev) => ({
                  ...prev,
                  epg_sources: reorderItems(prev.epg_sources, index, direction),
                }));
              }}
              getSourceId={(item) => item.epg_source_id}
            />
          </div>

          {/* Filters */}
          <div className="space-y-4">
            <h3 className="text-lg font-medium">Filters</h3>
            <AssignedItemsList
              title="Filters"
              items={formData.filters}
              type="filter"
              onAddItems={() => setFilterModalOpen(true)}
              onRemove={(index) => {
                setFormData((prev) => ({
                  ...prev,
                  filters: prev.filters.filter((_, i) => i !== index),
                }));
              }}
              onReorder={(index, direction) => {
                setFormData((prev) => ({
                  ...prev,
                  filters: reorderItems(prev.filters, index, direction),
                }));
              }}
              onToggleActive={(index) => {
                setFormData((prev) => ({
                  ...prev,
                  filters: prev.filters.map((filter, i) =>
                    i === index ? { ...filter, is_active: !filter.is_active } : filter
                  ),
                }));
              }}
              getSourceId={(item) => item.filter_id}
            />
          </div>

          {/* Advanced Settings */}
          <div className="space-y-4">
            <h3 className="text-lg font-medium">Advanced Settings</h3>

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="starting_channel_number">Starting Channel Number</Label>
                <Input
                  id="starting_channel_number"
                  type="text"
                  inputMode="numeric"
                  pattern="[0-9]*"
                  value={formData.starting_channel_number.toString()}
                  onChange={(e) => {
                    const value = e.target.value.replace(/[^0-9]/g, '');
                    if (value === '' || parseInt(value) >= 1) {
                      setFormData((prev) => ({
                        ...prev,
                        starting_channel_number: value === '' ? 1 : parseInt(value),
                      }));
                    }
                  }}
                  onFocus={(e) => e.target.select()}
                  placeholder="1"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="upstream_timeout">Upstream Timeout (seconds)</Label>
                <Input
                  id="upstream_timeout"
                  type="text"
                  inputMode="numeric"
                  pattern="[0-9]*"
                  value={formData.upstream_timeout.toString()}
                  onChange={(e) => {
                    const value = e.target.value.replace(/[^0-9]/g, '');
                    if (value === '' || (parseInt(value) >= 1 && parseInt(value) <= 300)) {
                      setFormData((prev) => ({
                        ...prev,
                        upstream_timeout: value === '' ? 30 : parseInt(value),
                      }));
                    }
                  }}
                  onFocus={(e) => e.target.select()}
                  placeholder="30"
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="max_concurrent_streams">Max Concurrent Streams</Label>
                <Input
                  id="max_concurrent_streams"
                  type="text"
                  inputMode="numeric"
                  pattern="[0-9]*"
                  value={formData.max_concurrent_streams.toString()}
                  onChange={(e) => {
                    const value = e.target.value.replace(/[^0-9]/g, '');
                    if (value === '' || (parseInt(value) >= 1 && parseInt(value) <= 1000)) {
                      setFormData((prev) => ({
                        ...prev,
                        max_concurrent_streams: value === '' ? 10 : parseInt(value),
                      }));
                    }
                  }}
                  onFocus={(e) => e.target.select()}
                  placeholder="10"
                />
              </div>
            </div>
          </div>

          {/* Boolean Settings */}
          <div className="space-y-4">
            <div className="grid grid-cols-1 gap-4">
              <div className="flex items-center justify-between rounded-lg border p-3">
                <div>
                  <Label>Active</Label>
                  <p className="text-sm text-muted-foreground">Whether the proxy is active</p>
                </div>
                <Switch
                  checked={formData.is_active}
                  onCheckedChange={(checked) =>
                    setFormData((prev) => ({ ...prev, is_active: checked }))
                  }
                />
              </div>

              <div className="flex items-center justify-between rounded-lg border p-3">
                <div>
                  <Label>Auto Regenerate</Label>
                  <p className="text-sm text-muted-foreground">Regenerate when sources change</p>
                </div>
                <Switch
                  checked={formData.auto_regenerate}
                  onCheckedChange={(checked) =>
                    setFormData((prev) => ({ ...prev, auto_regenerate: checked }))
                  }
                />
              </div>

              <div className="flex items-center justify-between rounded-lg border p-3">
                <div>
                  <Label>Cache Channel Logos</Label>
                  <p className="text-sm text-muted-foreground">Cache channel logo images</p>
                </div>
                <Switch
                  checked={formData.cache_channel_logos}
                  onCheckedChange={(checked) =>
                    setFormData((prev) => ({ ...prev, cache_channel_logos: checked }))
                  }
                />
              </div>

              <div className="flex items-center justify-between rounded-lg border p-3">
                <div>
                  <Label>Cache Program Logos</Label>
                  <p className="text-sm text-muted-foreground">Cache program logo images</p>
                </div>
                <Switch
                  checked={formData.cache_program_logos}
                  onCheckedChange={(checked) =>
                    setFormData((prev) => ({ ...prev, cache_program_logos: checked }))
                  }
                />
              </div>
            </div>
          </div>
        </form>

        <SheetFooter className="gap-2">
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={
              loading ||
              !formData.name.trim() ||
              (formData.proxy_mode === 'relay' && !formData.relay_profile_id)
            }
          >
            {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            {mode === 'create' ? 'Create Proxy' : 'Update Proxy'}
          </Button>
        </SheetFooter>
      </SheetContent>

      {/* Multi-select modals */}
      <MultiSelectModal
        title="Stream Sources"
        type="stream"
        selectedIds={formData.stream_sources.map((s) => s.source_id)}
        onConfirm={addStreamSources}
        open={streamSourceModalOpen}
        onOpenChange={setStreamSourceModalOpen}
      />

      <MultiSelectModal
        title="EPG Sources"
        type="epg"
        selectedIds={formData.epg_sources.map((s) => s.epg_source_id)}
        onConfirm={addEpgSources}
        open={epgSourceModalOpen}
        onOpenChange={setEpgSourceModalOpen}
      />

      <MultiSelectModal
        title="Filters"
        type="filter"
        selectedIds={formData.filters.map((f) => f.filter_id)}
        onConfirm={addFilters}
        open={filterModalOpen}
        onOpenChange={setFilterModalOpen}
      />
    </Sheet>
  );
}

// Convenience wrapper for create mode with trigger button
export function CreateProxyModal({
  onCreateProxy,
  loading,
  error,
}: {
  onCreateProxy: (proxy: ProxyFormData) => Promise<void>;
  loading: boolean;
  error: string | null;
}) {
  const [open, setOpen] = useState(false);

  return (
    <>
      <Button className="gap-2" onClick={() => setOpen(true)}>
        <Plus className="h-4 w-4" />
        Create Proxy
      </Button>
      <ProxySheet
        mode="create"
        onSaveProxy={onCreateProxy}
        loading={loading}
        error={error}
        open={open}
        onOpenChange={setOpen}
      />
    </>
  );
}

// Export types for external use
export type { ProxyFormData };
