'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
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
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Label } from '@/components/ui/label';
import {
  Plus,
  AlertCircle,
  Loader2,
  Settings,
  Database,
  Calendar,
  Filter as FilterIcon,
  Tv,
} from 'lucide-react';
import { WizardLayout, WizardStep, WizardStepContent, WizardStepSection, DetailPanel, SortableSelectionList, SelectionItem, SelectedItem } from '@/components/shared';
import { apiClient } from '@/lib/api-client';
import {
  StreamProxy,
  EncodingProfile,
  NumberingMode,
  StreamSourceResponse,
  EpgSourceResponse,
  Filter,
} from '@/types/api';

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

export interface ProxyFormData {
  name: string;
  description?: string;
  proxy_mode: 'direct' | 'smart';
  upstream_timeout: number;
  max_concurrent_streams: number;
  starting_channel_number: number;
  numbering_mode: NumberingMode;
  group_numbering_size: number;
  stream_sources: StreamSourceAssignment[];
  epg_sources: EpgSourceAssignment[];
  filters: FilterAssignment[];
  is_active: boolean;
  auto_regenerate: boolean;
  cache_channel_logos: boolean;
  cache_program_logos: boolean;
  client_detection_enabled: boolean;
  encoding_profile_id?: string;
}


// Channel count preview component
function ChannelCountPreview({
  streamSources,
  allStreamSources,
}: {
  streamSources: StreamSourceAssignment[];
  allStreamSources: StreamSourceResponse[];
}) {
  const totalChannels = useMemo(() => {
    return streamSources.reduce((count, assignment) => {
      const source = allStreamSources.find((s) => s.id === assignment.source_id);
      return count + (source?.channel_count || 0);
    }, 0);
  }, [streamSources, allStreamSources]);

  if (streamSources.length === 0) {
    return null;
  }

  return (
    <div className="flex items-center gap-2 p-3 bg-muted/50 rounded-lg">
      <Tv className="h-5 w-5 text-primary" />
      <div>
        <div className="text-sm font-medium">Estimated Channels: {totalChannels}</div>
        <div className="text-xs text-muted-foreground">
          From {streamSources.length} source{streamSources.length !== 1 ? 's' : ''}
        </div>
      </div>
    </div>
  );
}

// Main wizard component
interface ProxyWizardProps {
  mode: 'create' | 'edit';
  proxy?: StreamProxy | null;
  onSave: (data: ProxyFormData, proxyId?: string) => Promise<void>;
  loading: boolean;
  error: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** When true, renders inline in DetailPanel instead of Dialog */
  inline?: boolean;
  /** Called when cancel is clicked in inline mode */
  onCancel?: () => void;
}

export function ProxyWizard({
  mode,
  proxy,
  onSave,
  loading,
  error,
  open,
  onOpenChange,
  inline = false,
  onCancel,
}: ProxyWizardProps) {
  const [currentStep, setCurrentStep] = useState(0);
  const [isDataLoaded, setIsDataLoaded] = useState(false);

  // Available sources data
  const [streamSources, setStreamSources] = useState<StreamSourceResponse[]>([]);
  const [epgSources, setEpgSources] = useState<EpgSourceResponse[]>([]);
  const [filters, setFilters] = useState<Filter[]>([]);
  const [encodingProfiles, setEncodingProfiles] = useState<EncodingProfile[]>([]);

  // Form state
  const [formData, setFormData] = useState<ProxyFormData>({
    name: '',
    description: '',
    proxy_mode: 'smart',
    upstream_timeout: 30,
    max_concurrent_streams: 0,
    starting_channel_number: 1,
    numbering_mode: 'preserve',
    group_numbering_size: 100,
    stream_sources: [],
    epg_sources: [],
    filters: [],
    is_active: true,
    auto_regenerate: true,
    cache_channel_logos: true,
    cache_program_logos: false,
    client_detection_enabled: true,
  });

  // Wizard steps configuration
  const steps: WizardStep[] = [
    {
      id: 'basic',
      title: 'Basic Info',
      description: 'Name and proxy mode',
      icon: <Settings className="h-4 w-4" />,
      isValid: formData.name.trim().length > 0,
    },
    {
      id: 'sources',
      title: 'Stream Sources',
      description: 'Select channel sources',
      icon: <Database className="h-4 w-4" />,
      isOptional: false,
    },
    {
      id: 'epg',
      title: 'EPG Sources',
      description: 'Program guide data',
      icon: <Calendar className="h-4 w-4" />,
      isOptional: true,
    },
    {
      id: 'filters',
      title: 'Filters',
      description: 'Channel filtering rules',
      icon: <FilterIcon className="h-4 w-4" />,
      isOptional: true,
    },
    {
      id: 'settings',
      title: 'Settings',
      description: 'Advanced configuration',
      icon: <Settings className="h-4 w-4" />,
    },
  ];

  // Reset state when modal/panel closes
  useEffect(() => {
    if (!open) {
      setCurrentStep(0);
      setIsDataLoaded(false);
    }
  }, [open]);

  // Load data when modal/panel opens
  useEffect(() => {
    if (open && !isDataLoaded) {
      loadData();
    }
  }, [open, isDataLoaded]);

  const loadData = async () => {
    try {
      // Load all available sources in parallel
      const [streamRes, epgRes, filterRes, profileRes] = await Promise.all([
        apiClient.getStreamSources(),
        apiClient.getEpgSources(),
        apiClient.getFilters(),
        apiClient.getEncodingProfiles(),
      ]);

      setStreamSources(streamRes.items || []);
      setEpgSources(epgRes.items || []);
      setFilters(Array.isArray(filterRes) ? filterRes : []);
      setEncodingProfiles(Array.isArray(profileRes) ? profileRes : []);

      // Set up form data
      if (mode === 'edit' && proxy) {
        // Load existing proxy data
        const proxyResponse = await apiClient.getProxy(proxy.id);
        // The API returns associations but TypeScript types don't capture them
        const detailedProxy = (proxyResponse.data || proxyResponse) as any;

        setFormData({
          name: detailedProxy.name,
          description: detailedProxy.description || '',
          proxy_mode: detailedProxy.proxy_mode as 'direct' | 'smart',
          upstream_timeout: detailedProxy.upstream_timeout || 30,
          max_concurrent_streams: detailedProxy.max_concurrent_streams ?? 0,
          starting_channel_number: detailedProxy.starting_channel_number,
          numbering_mode: detailedProxy.numbering_mode || 'preserve',
          group_numbering_size: detailedProxy.group_numbering_size || 100,
          stream_sources: (detailedProxy.stream_sources || []).map((s: any) => ({
            source_id: s.source_id,
            priority_order: s.priority_order,
          })),
          epg_sources: (detailedProxy.epg_sources || []).map((s: any) => ({
            epg_source_id: s.epg_source_id,
            priority_order: s.priority_order,
          })),
          filters: (detailedProxy.filters || []).map((f: any) => ({
            filter_id: f.filter_id,
            priority_order: f.priority_order,
            is_active: f.is_active,
          })),
          is_active: detailedProxy.is_active,
          auto_regenerate: detailedProxy.auto_regenerate,
          cache_channel_logos: detailedProxy.cache_channel_logos,
          cache_program_logos: detailedProxy.cache_program_logos,
          client_detection_enabled: detailedProxy.client_detection_enabled ?? true,
          encoding_profile_id: detailedProxy.encoding_profile_id || '',
        });
      } else {
        // Default for create mode - pre-select all sources and system filters
        const defaultProfile = (Array.isArray(profileRes) ? profileRes : []).find((p: any) => p.is_default);
        const systemFilters = (Array.isArray(filterRes) ? filterRes : []).filter(
          (f: any) => f.is_system && f.is_enabled
        );

        setFormData({
          name: '',
          description: '',
          proxy_mode: 'smart',
          upstream_timeout: 30,
          max_concurrent_streams: 0,
          starting_channel_number: 1,
          numbering_mode: 'preserve',
          group_numbering_size: 100,
          stream_sources: (streamRes.items || []).map((s: any, i: number) => ({
            source_id: s.id,
            priority_order: i,
          })),
          epg_sources: (epgRes.items || []).map((s: any, i: number) => ({
            epg_source_id: s.id,
            priority_order: i,
          })),
          filters: systemFilters.map((f: any, i: number) => ({
            filter_id: f.id,
            priority_order: i,
            is_active: true,
          })),
          is_active: true,
          auto_regenerate: true,
          cache_channel_logos: true,
          cache_program_logos: false,
          client_detection_enabled: true,
          encoding_profile_id: defaultProfile?.id || '',
        });
      }

      setIsDataLoaded(true);
    } catch (err) {
      console.error('Failed to load proxy data:', err);
    }
  };

  // Convert data to SortableSelectionList format
  const streamSourceItems: SelectionItem[] = useMemo(() =>
    streamSources.map((source) => ({
      id: source.id,
      title: source.name,
      subtitle: `${source.channel_count || 0} ch`,
      badges: [
        { label: source.source_type.toUpperCase(), priority: 'info' as const },
        ...(source.enabled ? [] : [{ label: 'Inactive', priority: 'warning' as const }]),
      ],
    })), [streamSources]);

  const epgSourceItems: SelectionItem[] = useMemo(() =>
    epgSources.map((source) => ({
      id: source.id,
      title: source.name,
      subtitle: `${source.program_count || 0} prog`,
      badges: source.enabled ? [] : [{ label: 'Inactive', priority: 'warning' as const }],
    })), [epgSources]);

  const filterItems: SelectionItem[] = useMemo(() =>
    filters.map((filter) => ({
      id: filter.id,
      title: filter.name,
      badges: [
        { label: filter.source_type.toUpperCase(), priority: 'info' as const },
        { label: filter.action.toUpperCase(), priority: filter.action === 'exclude' ? 'warning' as const : 'success' as const },
        ...(filter.is_system ? [{ label: 'System', priority: 'secondary' as const }] : []),
      ],
    })), [filters]);

  // Selection handlers for SortableSelectionList
  const handleStreamSourcesChange = useCallback((items: SelectedItem[]) => {
    setFormData((prev) => ({
      ...prev,
      stream_sources: items.map((item, i) => ({
        source_id: item.id,
        priority_order: i,
      })),
    }));
  }, []);

  const handleEpgSourcesChange = useCallback((items: SelectedItem[]) => {
    setFormData((prev) => ({
      ...prev,
      epg_sources: items.map((item, i) => ({
        epg_source_id: item.id,
        priority_order: i,
      })),
    }));
  }, []);

  const handleFiltersChange = useCallback((items: SelectedItem[]) => {
    setFormData((prev) => ({
      ...prev,
      filters: items.map((item, i) => ({
        filter_id: item.id,
        priority_order: i,
        is_active: item.isActive ?? true,
      })),
    }));
  }, []);

  const handleComplete = async () => {
    try {
      await onSave(formData, mode === 'edit' ? proxy?.id : undefined);
      if (inline) {
        onCancel?.();
      } else {
        onOpenChange(false);
      }
    } catch (err) {
      // Error handled by parent
    }
  };

  const canNavigateForward = useMemo(() => {
    if (currentStep === 0) {
      return formData.name.trim().length > 0;
    }
    return true;
  }, [currentStep, formData.name]);

  // Render step content
  const renderStepContent = () => {
    switch (steps[currentStep].id) {
      case 'basic':
        return (
          <WizardStepContent>
            <WizardStepSection title="Proxy Details">
              <div className="space-y-4">
                <div className="space-y-2">
                  <Label htmlFor="name">Name *</Label>
                  <Input
                    id="name"
                    value={formData.name}
                    onChange={(e) => setFormData((prev) => ({ ...prev, name: e.target.value }))}
                    placeholder="My IPTV Proxy"
                  />
                </div>

                <div className="space-y-2">
                  <Label htmlFor="description">Description</Label>
                  <Textarea
                    id="description"
                    value={formData.description || ''}
                    onChange={(e) => setFormData((prev) => ({ ...prev, description: e.target.value }))}
                    placeholder="Optional description"
                    rows={2}
                  />
                </div>

                <div className="space-y-2">
                  <Label>Proxy Mode</Label>
                  <Select
                    value={formData.proxy_mode}
                    onValueChange={(value: 'direct' | 'smart') =>
                      setFormData((prev) => ({ ...prev, proxy_mode: value }))
                    }
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="direct">Direct (302 Redirect)</SelectItem>
                      <SelectItem value="smart">Smart (Auto-optimize)</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                {formData.proxy_mode === 'smart' && (
                  <>
                    <div className="flex items-center justify-between rounded-lg border p-3">
                      <div>
                        <Label>Client Detection</Label>
                        <p className="text-sm text-muted-foreground">
                          Automatically detect client capabilities
                        </p>
                      </div>
                      <Switch
                        checked={formData.client_detection_enabled}
                        onCheckedChange={(checked) =>
                          setFormData((prev) => ({ ...prev, client_detection_enabled: checked }))
                        }
                      />
                    </div>

                    {encodingProfiles.length > 0 && (
                      <div className="space-y-2">
                        <Label>Default Encoding Profile</Label>
                        <Select
                          value={formData.encoding_profile_id || ''}
                          onValueChange={(value) =>
                            setFormData((prev) => ({ ...prev, encoding_profile_id: value }))
                          }
                        >
                          <SelectTrigger>
                            <SelectValue placeholder="Select encoding profile" />
                          </SelectTrigger>
                          <SelectContent>
                            {encodingProfiles.map((profile) => (
                              <SelectItem key={profile.id} value={profile.id}>
                                {profile.name}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                    )}
                  </>
                )}
              </div>
            </WizardStepSection>
          </WizardStepContent>
        );

      case 'sources':
        return (
          <WizardStepContent>
            <ChannelCountPreview
              streamSources={formData.stream_sources}
              allStreamSources={streamSources}
            />

            <WizardStepSection
              title="Stream Sources"
              description="Select and order sources to include in this proxy. Drag to reorder."
            >
              <SortableSelectionList
                items={streamSourceItems}
                selectedItems={formData.stream_sources.map((s) => ({ id: s.source_id }))}
                onSelectionChange={handleStreamSourcesChange}
                maxHeight="300px"
                emptyMessage="No stream sources available"
                selectedLabel="Selected Sources"
                availableLabel="Available Sources"
              />
            </WizardStepSection>
          </WizardStepContent>
        );

      case 'epg':
        return (
          <WizardStepContent>
            <WizardStepSection
              title="EPG Sources"
              description="Select program guide sources for channel metadata. Drag to reorder."
            >
              <SortableSelectionList
                items={epgSourceItems}
                selectedItems={formData.epg_sources.map((s) => ({ id: s.epg_source_id }))}
                onSelectionChange={handleEpgSourcesChange}
                maxHeight="300px"
                emptyMessage="No EPG sources available"
                selectedLabel="Selected EPG Sources"
                availableLabel="Available EPG Sources"
              />
            </WizardStepSection>
          </WizardStepContent>
        );

      case 'filters':
        return (
          <WizardStepContent>
            <WizardStepSection
              title="Filters"
              description="Select and order filters to apply to channels. Drag to reorder, toggle to enable/disable."
            >
              <SortableSelectionList
                items={filterItems}
                selectedItems={formData.filters.map((f) => ({
                  id: f.filter_id,
                  isActive: f.is_active,
                }))}
                onSelectionChange={handleFiltersChange}
                showActiveToggle={true}
                maxHeight="300px"
                emptyMessage="No filters available"
                selectedLabel="Selected Filters"
                availableLabel="Available Filters"
              />
            </WizardStepSection>
          </WizardStepContent>
        );

      case 'settings':
        return (
          <WizardStepContent>
            <WizardStepSection title="Channel Numbering">
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>Starting Channel Number</Label>
                  <Input
                    type="number"
                    min={1}
                    value={formData.starting_channel_number}
                    onChange={(e) =>
                      setFormData((prev) => ({
                        ...prev,
                        starting_channel_number: parseInt(e.target.value) || 1,
                      }))
                    }
                  />
                </div>

                <div className="space-y-2">
                  <Label>Numbering Mode</Label>
                  <Select
                    value={formData.numbering_mode}
                    onValueChange={(value: NumberingMode) =>
                      setFormData((prev) => ({ ...prev, numbering_mode: value }))
                    }
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="preserve">Preserve</SelectItem>
                      <SelectItem value="sequential">Sequential</SelectItem>
                      <SelectItem value="group">Group by Category</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                {formData.numbering_mode === 'group' && (
                  <div className="space-y-2">
                    <Label>Group Size</Label>
                    <Input
                      type="number"
                      min={1}
                      max={10000}
                      value={formData.group_numbering_size}
                      onChange={(e) =>
                        setFormData((prev) => ({
                          ...prev,
                          group_numbering_size: parseInt(e.target.value) || 100,
                        }))
                      }
                    />
                  </div>
                )}
              </div>
            </WizardStepSection>

            <WizardStepSection title="Performance">
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>Upstream Timeout (seconds)</Label>
                  <Input
                    type="number"
                    min={1}
                    max={300}
                    value={formData.upstream_timeout}
                    onChange={(e) =>
                      setFormData((prev) => ({
                        ...prev,
                        upstream_timeout: parseInt(e.target.value) || 30,
                      }))
                    }
                  />
                </div>

                <div className="space-y-2">
                  <Label>Max Concurrent Streams</Label>
                  <Input
                    type="number"
                    min={0}
                    value={formData.max_concurrent_streams}
                    onChange={(e) =>
                      setFormData((prev) => ({
                        ...prev,
                        max_concurrent_streams: parseInt(e.target.value) || 0,
                      }))
                    }
                  />
                  <p className="text-xs text-muted-foreground">0 = unlimited</p>
                </div>
              </div>
            </WizardStepSection>

            <WizardStepSection title="Options">
              <div className="space-y-3">
                <div className="flex items-center justify-between rounded-lg border p-3">
                  <div>
                    <Label>Active</Label>
                    <p className="text-sm text-muted-foreground">Enable this proxy</p>
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
            </WizardStepSection>
          </WizardStepContent>
        );

      default:
        return null;
    }
  };

  // Inline mode - render in DetailPanel
  if (inline) {
    return (
      <DetailPanel
        title={mode === 'create' ? 'Create Stream Proxy' : 'Edit Stream Proxy'}
        actions={
          <Button
            variant="outline"
            size="sm"
            onClick={onCancel}
            disabled={loading}
          >
            Cancel
          </Button>
        }
      >
        {error && (
          <Alert variant="destructive" className="mb-4">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {!isDataLoaded ? (
          <div className="flex items-center justify-center py-12">
            <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
          </div>
        ) : (
          <WizardLayout
            steps={steps}
            currentStep={currentStep}
            onStepChange={setCurrentStep}
            onComplete={handleComplete}
            isLoading={loading}
            canNavigateForward={canNavigateForward}
            completeLabel={mode === 'create' ? 'Create Proxy' : 'Update Proxy'}
            compact={true}
            className="min-h-0"
          >
            {renderStepContent()}
          </WizardLayout>
        )}
      </DetailPanel>
    );
  }

  // Dialog mode - existing behavior
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl h-[80vh] p-0 flex flex-col">
        <DialogHeader className="px-6 pt-6 pb-0">
          <DialogTitle>
            {mode === 'create' ? 'Create Stream Proxy' : 'Edit Stream Proxy'}
          </DialogTitle>
          <DialogDescription>
            {mode === 'create'
              ? 'Configure your stream proxy step by step'
              : 'Update the stream proxy configuration'}
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div className="px-6">
            <Alert variant="destructive">
              <AlertCircle className="h-4 w-4" />
              <AlertTitle>Error</AlertTitle>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          </div>
        )}

        {!isDataLoaded ? (
          <div className="flex-1 flex items-center justify-center">
            <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
          </div>
        ) : (
          <WizardLayout
            steps={steps}
            currentStep={currentStep}
            onStepChange={setCurrentStep}
            onComplete={handleComplete}
            isLoading={loading}
            canNavigateForward={canNavigateForward}
            completeLabel={mode === 'create' ? 'Create Proxy' : 'Update Proxy'}
            className="flex-1 min-h-0"
          >
            {renderStepContent()}
          </WizardLayout>
        )}
      </DialogContent>
    </Dialog>
  );
}

// Convenience wrapper for create mode with trigger button
export function CreateProxyButton({
  onCreateProxy,
  loading,
  error,
  triggerButton,
}: {
  onCreateProxy: (proxy: ProxyFormData) => Promise<void>;
  loading: boolean;
  error: string | null;
  triggerButton?: React.ReactNode;
}) {
  const [open, setOpen] = useState(false);

  return (
    <>
      {triggerButton ? (
        <div onClick={() => setOpen(true)}>{triggerButton}</div>
      ) : (
        <Button className="gap-2" onClick={() => setOpen(true)}>
          <Plus className="h-4 w-4" />
          Create Proxy
        </Button>
      )}
      <ProxyWizard
        mode="create"
        onSave={onCreateProxy}
        loading={loading}
        error={error}
        open={open}
        onOpenChange={setOpen}
      />
    </>
  );
}
