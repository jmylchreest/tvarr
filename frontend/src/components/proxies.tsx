'use client';

import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { TooltipProvider } from '@/components/ui/tooltip';
import {
  Plus,
  Play,
  Activity,
  AlertCircle,
  WifiOff,
  Zap,
  CheckCircle,
} from 'lucide-react';
import { StatCard } from '@/components/shared/feedback/StatCard';
import {
  StreamProxy,
  PaginatedResponse,
  EncodingProfile,
} from '@/types/api';
import { apiClient, ApiError } from '@/lib/api-client';
import { API_CONFIG } from '@/lib/config';
import { createFuzzyFilter } from '@/lib/fuzzy-search';
import { useProgressContext } from '@/providers/ProgressProvider';
import { ProxyWizard, ProxyFormData } from '@/components/ProxyWizard';
import { ProxyDetailPanel } from '@/components/ProxyDetailPanel';
import { MasterDetailLayout, MasterItem, DetailEmpty } from '@/components/shared/layouts/MasterDetailLayout';
import { BadgeItem } from '@/components/shared';
import { AnimatedBadgeGroup } from '@/components/shared/AnimatedBadgeGroup';

interface LoadingState {
  proxies: boolean;
  create: boolean;
  edit: boolean;
  delete: string | null;
}

interface ErrorState {
  proxies: string | null;
  create: string | null;
  edit: string | null;
  action: string | null;
}

interface ProxyMasterItem extends MasterItem {
  proxy: StreamProxy;
}

function getStatusColor(isActive: boolean): string {
  return isActive ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-800';
}

function proxyToMasterItem(proxy: StreamProxy): ProxyMasterItem {
  // Build array of badges with priority-based styling
  const badges: BadgeItem[] = [
    { label: proxy.proxy_mode, priority: 'info' },
  ];

  if (proxy.status === 'failed') {
    badges.push({ label: 'Failed', priority: 'error' });
  }

  if (!proxy.is_active) {
    badges.push({ label: 'Inactive', priority: 'outline' });
  }

  return {
    id: proxy.id,
    title: proxy.name,
    enabled: proxy.is_active,
    badge: <AnimatedBadgeGroup resourceId={proxy.id} badges={badges} size="sm" />,
    proxy,
  };
}

export function Proxies() {
  const progressContext = useProgressContext();
  const [allProxies, setAllProxies] = useState<StreamProxy[]>([]);
  const [selectedProxy, setSelectedProxy] = useState<StreamProxy | null>(null);
  const [encodingProfiles, setEncodingProfiles] = useState<EncodingProfile[]>([]);

  const [loading, setLoading] = useState<LoadingState>({
    proxies: false,
    create: false,
    edit: false,
    delete: null,
  });

  const [errors, setErrors] = useState<ErrorState>({
    proxies: null,
    create: null,
    edit: null,
    action: null,
  });

  const [regeneratingProxies, setRegeneratingProxies] = useState<Set<string>>(new Set());
  const [isOnline, setIsOnline] = useState(true);
  const [editingProxy, setEditingProxy] = useState<StreamProxy | null>(null);
  const [isCreating, setIsCreating] = useState(false);
  const [isEditing, setIsEditing] = useState(false);

  // Use ref for selected proxy ID to avoid re-triggering loadProxies
  const selectedProxyIdRef = useRef<string | null>(null);
  selectedProxyIdRef.current = selectedProxy?.id ?? null;

  // Convert proxies to master items
  const masterItems = useMemo(
    () => allProxies.map(proxyToMasterItem).sort((a, b) => a.title.localeCompare(b.title, undefined, { numeric: true })),
    [allProxies]
  );

  // Initialize SSE connection on mount
  useEffect(() => {
    const handleGlobalProxyEvent = (event: any) => {
      console.log('[Proxies] Received global proxy event:', event);

      if (
        (event.state === 'idle' || event.state === 'processing') &&
        event.id &&
        event.operation_type === 'proxy_regeneration'
      ) {
        console.log(`[Proxies] Adding ${event.id} to regenerating set (state: ${event.state})`);
        setRegeneratingProxies((prev) => {
          const newSet = new Set(prev);
          newSet.add(event.id);
          return newSet;
        });
      }

      if (
        (event.state === 'completed' || event.state === 'error') &&
        event.id &&
        event.operation_type === 'proxy_regeneration'
      ) {
        console.log(`[Proxies] Removing ${event.id} from regenerating set (state: ${event.state})`);
        setRegeneratingProxies((prev) => {
          const newSet = new Set(prev);
          newSet.delete(event.id);
          if (newSet.has(event.id)) {
            setTimeout(() => loadProxies(), 1000);
          }
          return newSet;
        });
      }
    };

    const unsubscribe = progressContext.subscribeToType(
      'proxy_regeneration',
      handleGlobalProxyEvent
    );

    return () => {
      console.log('[Proxies] Component unmounting, unsubscribing from proxy events');
      unsubscribe();
    };
  }, []);

  const loadProxies = useCallback(async () => {
    if (!isOnline) return;

    setLoading((prev) => ({ ...prev, proxies: true }));
    setErrors((prev) => ({ ...prev, proxies: null }));

    try {
      const response = await apiClient.getProxies();
      setAllProxies(response.items);
      setIsOnline(true);

      // Update selected proxy if it still exists (use ref to avoid dependency)
      const currentSelectedId = selectedProxyIdRef.current;
      if (currentSelectedId) {
        const updated = response.items.find((p) => p.id === currentSelectedId);
        if (updated) {
          setSelectedProxy(updated);
        } else {
          setSelectedProxy(null);
        }
      }
    } catch (error) {
      const apiError = error as ApiError;
      if (apiError.status === 0) {
        setIsOnline(false);
        setErrors((prev) => ({
          ...prev,
          proxies: `Unable to connect to the API service. Please check that the service is running at ${API_CONFIG.baseUrl}.`,
        }));
      } else {
        setErrors((prev) => ({
          ...prev,
          proxies: `Failed to load proxies: ${apiError.message}`,
        }));
      }
    } finally {
      setLoading((prev) => ({ ...prev, proxies: false }));
    }
  }, [isOnline]);

  // Load encoding profiles on mount
  useEffect(() => {
    const loadEncodingProfilesData = async () => {
      try {
        const profiles = await apiClient.getEncodingProfiles();
        setEncodingProfiles(profiles);
      } catch (error) {
        console.error('Failed to load encoding profiles:', error);
      }
    };
    loadEncodingProfilesData();
  }, []);

  // Load proxies on mount only
  useEffect(() => {
    loadProxies();
  }, [loadProxies]);

  const handleCreateProxy = async (formData: ProxyFormData) => {
    setLoading((prev) => ({ ...prev, create: true }));
    setErrors((prev) => ({ ...prev, create: null }));

    try {
      const createRequest = {
        name: formData.name,
        proxy_mode: formData.proxy_mode,
        starting_channel_number: formData.starting_channel_number,
        numbering_mode: formData.numbering_mode,
        group_numbering_size: formData.group_numbering_size,
        stream_sources: formData.stream_sources,
        epg_sources: formData.epg_sources,
        filters: formData.filters,
        is_active: formData.is_active,
        auto_regenerate: formData.auto_regenerate,
        description: formData.description,
        max_concurrent_streams: formData.max_concurrent_streams,
        upstream_timeout: formData.upstream_timeout,
        cache_channel_logos: formData.cache_channel_logos,
        cache_program_logos: formData.cache_program_logos,
        encoding_profile_id: formData.encoding_profile_id,
      };

      const created = await apiClient.createProxy(createRequest);
      await loadProxies();
      // Exit create mode and select the new proxy
      setIsCreating(false);
      const createdId = created?.data?.id;
      if (createdId) {
        // Find and select the newly created proxy
        const response = await apiClient.getProxies();
        const newProxy = response.items.find((p) => p.id === createdId);
        if (newProxy) {
          setSelectedProxy(newProxy);
        }
      }
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        create: `Failed to create proxy: ${apiError.message}`,
      }));
      throw error;
    } finally {
      setLoading((prev) => ({ ...prev, create: false }));
    }
  };

  const handleUpdateProxy = async (formData: ProxyFormData, proxyId?: string) => {
    if (!proxyId) return;

    setLoading((prev) => ({ ...prev, edit: true }));
    setErrors((prev) => ({ ...prev, edit: null }));

    try {
      const updateRequest = {
        name: formData.name,
        proxy_mode: formData.proxy_mode,
        starting_channel_number: formData.starting_channel_number,
        numbering_mode: formData.numbering_mode,
        group_numbering_size: formData.group_numbering_size,
        stream_sources: formData.stream_sources,
        epg_sources: formData.epg_sources,
        filters: formData.filters,
        is_active: formData.is_active,
        auto_regenerate: formData.auto_regenerate,
        description: formData.description,
        max_concurrent_streams: formData.max_concurrent_streams,
        upstream_timeout: formData.upstream_timeout,
        cache_channel_logos: formData.cache_channel_logos,
        cache_program_logos: formData.cache_program_logos,
        encoding_profile_id: formData.encoding_profile_id,
      };

      await apiClient.updateProxy(proxyId, updateRequest);
      await loadProxies();
      // Exit edit mode and keep the proxy selected
      setIsEditing(false);
      setEditingProxy(null);
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        edit: `Failed to update proxy: ${apiError.message}`,
      }));
      throw error;
    } finally {
      setLoading((prev) => ({ ...prev, edit: false }));
    }
  };

  const handleRegenerateProxy = async (proxyId: string) => {
    console.log(`[Proxies] Starting regeneration for proxy: ${proxyId}`);
    setRegeneratingProxies((prev) => new Set(prev).add(proxyId));
    setErrors((prev) => ({ ...prev, action: null }));

    try {
      console.log(`[Proxies] Calling API regenerate for proxy: ${proxyId}`);
      await apiClient.regenerateProxy(proxyId);
      console.log(`[Proxies] API regenerate call completed for proxy: ${proxyId}`);

      setTimeout(() => {
        console.log(
          `[Proxies] Fallback timeout - clearing regenerating state for proxy: ${proxyId}`
        );
        setRegeneratingProxies((prev) => {
          const newSet = new Set(prev);
          if (newSet.has(proxyId)) {
            newSet.delete(proxyId);
            loadProxies();
          }
          return newSet;
        });
      }, 30000);
    } catch (error) {
      const apiError = error as ApiError;
      console.error(`[Proxies] Regeneration failed for proxy ${proxyId}:`, apiError);

      if (apiError.status !== 409) {
        setErrors((prev) => ({
          ...prev,
          action: `Failed to start regeneration: ${apiError.message}`,
        }));
      }

      setRegeneratingProxies((prev) => {
        const newSet = new Set(prev);
        newSet.delete(proxyId);
        return newSet;
      });

      throw error;
    }
  };

  const handleDeleteProxy = async (proxyId: string) => {
    if (!confirm('Are you sure you want to delete this proxy? This action cannot be undone.')) {
      return;
    }

    setLoading((prev) => ({ ...prev, delete: proxyId }));
    setErrors((prev) => ({ ...prev, action: null }));

    try {
      await apiClient.deleteProxy(proxyId);
      if (selectedProxy?.id === proxyId) {
        setSelectedProxy(null);
      }
      await loadProxies();
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        action: `Failed to delete proxy: ${apiError.message}`,
      }));
    } finally {
      setLoading((prev) => ({ ...prev, delete: null }));
    }
  };

  const handleSelectProxy = (item: ProxyMasterItem | null) => {
    setSelectedProxy(item?.proxy ?? null);
  };

  const activeProxies = allProxies?.filter((p) => p.is_active).length || 0;
  const totalProxies = allProxies?.length || 0;

  return (
    <TooltipProvider>
      <div className="flex flex-col gap-6 h-full">
        {/* Header */}
        <div className="flex items-center justify-between">
          <p className="text-muted-foreground">
            Manage streaming proxy configurations
          </p>
          {!isOnline && <WifiOff className="h-5 w-5 text-destructive" />}
        </div>

        {/* Connection Status Alert */}
        {!isOnline && (
          <Alert variant="destructive">
            <WifiOff className="h-4 w-4" />
            <AlertTitle>API Service Offline</AlertTitle>
            <AlertDescription>
              Unable to connect to the API service at {API_CONFIG.baseUrl}. Please ensure the
              service is running and try again.
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

        {/* Stats */}
        <div className="grid gap-2 md:grid-cols-3">
          <StatCard
            title="Total Proxies"
            value={totalProxies}
            icon={<Play className="h-4 w-4" />}
          />
          <StatCard
            title="Active"
            value={activeProxies}
            icon={<CheckCircle className="h-4 w-4" />}
          />
          <StatCard
            title="Regenerating"
            value={regeneratingProxies.size}
            icon={<Zap className="h-4 w-4" />}
          />
        </div>

        {/* Proxy List Error */}
        {errors.proxies ? (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Failed to Load Proxies</AlertTitle>
            <AlertDescription>
              {errors.proxies}
              <Button
                variant="outline"
                size="sm"
                className="ml-2"
                onClick={loadProxies}
                disabled={loading.proxies}
              >
                Retry
              </Button>
            </AlertDescription>
          </Alert>
        ) : (
          /* Master-Detail Layout in Card container */
          <Card className="flex-1 overflow-hidden min-h-0">
            <CardContent className="p-0 h-full">
              <MasterDetailLayout
                items={masterItems}
                selectedId={isCreating ? null : selectedProxy?.id}
                onSelect={(item) => {
                  setIsCreating(false);
                  handleSelectProxy(item);
                }}
                isLoading={loading.proxies}
                title={`Proxies (${allProxies.length})`}
                searchPlaceholder="Search by name, mode, status..."
                storageKey="proxies"
                headerAction={
                  <Button
                    size="sm"
                    variant="ghost"
                    className="h-8 w-8 p-0"
                    onClick={() => {
                      setIsCreating(true);
                      setSelectedProxy(null);
                      setErrors((prev) => ({ ...prev, create: null }));
                    }}
                    disabled={loading.proxies}
                  >
                    <Plus className="h-4 w-4" />
                  </Button>
                }
                emptyState={{
                  title: 'No proxies configured',
                  description: 'Get started by creating your first proxy configuration.',
                }}
                filterFn={createFuzzyFilter<ProxyMasterItem>({
                  keys: [
                    { name: 'name', weight: 0.4 },
                    { name: 'description', weight: 0.2 },
                    { name: 'proxy_mode', weight: 0.15 },
                    { name: 'status', weight: 0.1 },
                    { name: 'channel_count', weight: 0.1 },
                    { name: 'active', weight: 0.05 },
                  ],
                  accessor: (item) => ({
                    name: item.proxy.name,
                    description: item.proxy.description || '',
                    proxy_mode: item.proxy.proxy_mode,
                    status: item.proxy.status || '',
                    channel_count: `${item.proxy.channel_count} channels`,
                    active: item.proxy.is_active ? 'active' : 'inactive',
                  }),
                })}
              >
                {(item) =>
                  isCreating ? (
                    <ProxyWizard
                      mode="create"
                      onSave={handleCreateProxy}
                      loading={loading.create}
                      error={errors.create}
                      open={isCreating}
                      onOpenChange={setIsCreating}
                      inline={true}
                      onCancel={() => setIsCreating(false)}
                    />
                  ) : isEditing && editingProxy ? (
                    <ProxyWizard
                      mode="edit"
                      proxy={editingProxy}
                      onSave={handleUpdateProxy}
                      loading={loading.edit}
                      error={errors.edit}
                      open={isEditing}
                      onOpenChange={(open) => {
                        setIsEditing(open);
                        if (!open) setEditingProxy(null);
                      }}
                      inline={true}
                      onCancel={() => {
                        setIsEditing(false);
                        setEditingProxy(null);
                      }}
                    />
                  ) : item ? (
                    <ProxyDetailPanel
                      proxy={item.proxy}
                      onEdit={() => {
                        setEditingProxy(item.proxy);
                        setIsEditing(true);
                      }}
                      onDelete={() => handleDeleteProxy(item.proxy.id)}
                      onRegenerate={() => handleRegenerateProxy(item.proxy.id)}
                      isRegenerating={regeneratingProxies.has(item.proxy.id)}
                      isOnline={isOnline}
                      isDeleting={loading.delete === item.proxy.id}
                      encodingProfiles={encodingProfiles}
                    />
                  ) : (
                    <DetailEmpty
                      title="Select a proxy"
                      description="Choose a proxy from the list to view details and manage it"
                      icon={<Play className="h-12 w-12 text-muted-foreground" />}
                    />
                  )
                }
              </MasterDetailLayout>
            </CardContent>
          </Card>
        )}
      </div>
    </TooltipProvider>
  );
}
