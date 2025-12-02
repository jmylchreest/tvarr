'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Switch } from '@/components/ui/switch';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  Plus,
  Play,
  Edit,
  Trash2,
  RefreshCw,
  Clock,
  Users,
  Activity,
  Grid,
  List,
  Table as TableIcon,
  Search,
  Filter as FilterIcon,
  AlertCircle,
  CheckCircle,
  Loader2,
  WifiOff,
  Settings,
  Zap,
  Copy,
  Check,
  GripVertical,
  ArrowUp,
  ArrowDown,
} from 'lucide-react';
import {
  StreamProxy,
  CreateStreamProxyRequest,
  UpdateStreamProxyRequest,
  PaginatedResponse,
  RelayProfile,
  StreamSourceResponse,
  EpgSourceResponse,
  Filter,
  ProxySourceRequest,
  ProxyEpgSourceRequest,
  ProxyFilterRequest,
} from '@/types/api';
import { apiClient, ApiError } from '@/lib/api-client';
import { DEFAULT_PAGE_SIZE, API_CONFIG } from '@/lib/config';
import { RefreshButton } from '@/components/RefreshButton';
import { useProgressContext } from '@/providers/ProgressProvider';
import { formatDate, formatRelativeTime } from '@/lib/utils';
import { CreateProxyModal, ProxySheet, ProxyFormData } from '@/components/CreateProxyModal';
import { useConflictHandler } from '@/hooks/useConflictHandler';
import { ConflictNotification } from '@/components/ConflictNotification';

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

function getStatusColor(isActive: boolean): string {
  return isActive ? 'bg-green-100 text-green-800' : 'bg-gray-100 text-gray-800';
}

export function Proxies() {
  const progressContext = useProgressContext();
  const [allProxies, setAllProxies] = useState<StreamProxy[]>([]);
  const [pagination, setPagination] = useState<Omit<
    PaginatedResponse<StreamProxy>,
    'items'
  > | null>(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [filterStatus, setFilterStatus] = useState<'all' | 'active' | 'inactive'>('all');
  const [currentPage, setCurrentPage] = useState(1);
  const [relayProfiles, setRelayProfiles] = useState<RelayProfile[]>([]);
  const { handleApiError, dismissConflict, getConflictState } = useConflictHandler();

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
  const [copiedUrls, setCopiedUrls] = useState<Set<string>>(new Set());
  const [editingProxy, setEditingProxy] = useState<StreamProxy | null>(null);
  const [isEditSheetOpen, setIsEditSheetOpen] = useState(false);
  const [viewMode, setViewMode] = useState<'grid' | 'list' | 'table'>('table');

  // Copy to clipboard functionality
  const copyToClipboard = async (text: string, urlType: string, proxyId: string) => {
    try {
      await navigator.clipboard.writeText(text);
      const urlKey = `${proxyId}-${urlType}`;
      setCopiedUrls((prev) => new Set(prev).add(urlKey));
      setTimeout(() => {
        setCopiedUrls((prev) => {
          const newSet = new Set(prev);
          newSet.delete(urlKey);
          return newSet;
        });
      }, 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

  // Compute filtered results locally
  const filteredProxies = useMemo(() => {
    let filtered = allProxies;

    // Filter by status
    if (filterStatus !== 'all') {
      filtered = filtered.filter((proxy) =>
        filterStatus === 'active' ? proxy.is_active : !proxy.is_active
      );
    }

    // Filter by search term
    if (searchTerm.trim()) {
      const searchLower = searchTerm.toLowerCase();
      filtered = filtered.filter((proxy) => {
        const searchableText = [
          proxy.name.toLowerCase(),
          proxy.proxy_mode.toLowerCase(),
          proxy.description?.toLowerCase() || '',
          proxy.starting_channel_number.toString(),
          proxy.max_concurrent_streams?.toString() || '',
          proxy.upstream_timeout?.toString() || '',
          proxy.relay_profile_id?.toLowerCase() || '',
          // Status labels
          proxy.is_active ? 'active enabled' : 'inactive disabled',
          proxy.auto_regenerate ? 'auto regenerate automatic' : 'manual',
          proxy.cache_channel_logos ? 'cache channel logos' : '',
          proxy.cache_program_logos ? 'cache program logos' : '',
          // Relative time and formatted dates
          formatRelativeTime(proxy.created_at).toLowerCase(),
          formatRelativeTime(proxy.updated_at).toLowerCase(),
          formatDate(proxy.created_at).toLowerCase(),
          formatDate(proxy.updated_at).toLowerCase(),
          // Additional searchable terms
          'stream proxy',
          'proxy server',
          'streaming proxy',
          proxy.created_at.includes('T') ? 'created' : '',
          proxy.updated_at.includes('T') ? 'updated' : '',
          // Mode-based terms
          'proxy mode',
          'channel',
          'buffer',
          'timeout',
          'concurrent streams',
          'relay profile',
        ];

        return searchableText.some((text) => text.toLowerCase().includes(searchLower));
      });
    }

    return filtered;
  }, [allProxies, searchTerm, filterStatus]);

  // Health check is handled by parent component, no need for redundant calls

  // Initialize SSE connection on mount
  useEffect(() => {
    // Listen for any proxy regeneration events
    const handleGlobalProxyEvent = (event: any) => {
      console.log('[Proxies] Received global proxy event:', event);

      // If we see an operation starting, add it to regenerating set
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

      // If we see a completion event, remove from regenerating set
      if (
        (event.state === 'completed' || event.state === 'error') &&
        event.id &&
        event.operation_type === 'proxy_regeneration'
      ) {
        console.log(`[Proxies] Removing ${event.id} from regenerating set (state: ${event.state})`);
        setRegeneratingProxies((prev) => {
          const newSet = new Set(prev);
          newSet.delete(event.id);
          // Reload proxies when regeneration completes
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
      // Load all proxies without search parameters - filtering happens locally
      const response = await apiClient.getProxies();

      setAllProxies(response.items);
      setPagination({
        total: response.total,
        page: response.page,
        per_page: response.per_page,
        total_pages: response.total_pages,
        has_next: response.has_next,
        has_previous: response.has_previous,
      });
      setIsOnline(true);
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

  // Load relay profiles on mount
  useEffect(() => {
    const loadRelayProfilesData = async () => {
      try {
        const profiles = await apiClient.getRelayProfiles();
        setRelayProfiles(profiles);
      } catch (error) {
        console.error('Failed to load relay profiles:', error);
      }
    };
    loadRelayProfilesData();
  }, []);

  // Load proxies on mount only
  useEffect(() => {
    loadProxies();
  }, [loadProxies]);

  const handleCreateProxy = async (formData: ProxyFormData) => {
    setLoading((prev) => ({ ...prev, create: true }));
    setErrors((prev) => ({ ...prev, create: null }));

    try {
      // Convert ProxyFormData to CreateStreamProxyRequest format
      const createRequest: CreateStreamProxyRequest = {
        name: formData.name,
        proxy_mode: formData.proxy_mode,
        starting_channel_number: formData.starting_channel_number,
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
        relay_profile_id: formData.relay_profile_id,
      };

      await apiClient.createProxy(createRequest);
      await loadProxies(); // Reload proxies after creation
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        create: `Failed to create proxy: ${apiError.message}`,
      }));
      throw error; // Re-throw to prevent dialog from closing
    } finally {
      setLoading((prev) => ({ ...prev, create: false }));
    }
  };

  const handleUpdateProxy = async (formData: ProxyFormData, proxyId?: string) => {
    if (!proxyId) return;

    setLoading((prev) => ({ ...prev, edit: true }));
    setErrors((prev) => ({ ...prev, edit: null }));

    try {
      // Convert ProxyFormData to UpdateStreamProxyRequest format
      const updateRequest: UpdateStreamProxyRequest = {
        name: formData.name,
        proxy_mode: formData.proxy_mode,
        starting_channel_number: formData.starting_channel_number,
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
        relay_profile_id: formData.relay_profile_id,
      };

      await apiClient.updateProxy(proxyId, updateRequest);
      await loadProxies(); // Reload proxies after update
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        edit: `Failed to update proxy: ${apiError.message}`,
      }));
      throw error; // Re-throw to prevent dialog from closing
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

      // Fallback timeout in case SSE events don't work
      setTimeout(() => {
        console.log(
          `[Proxies] Fallback timeout - clearing regenerating state for proxy: ${proxyId}`
        );
        setRegeneratingProxies((prev) => {
          const newSet = new Set(prev);
          if (newSet.has(proxyId)) {
            newSet.delete(proxyId);
            // Reload proxies as fallback
            loadProxies();
          }
          return newSet;
        });
      }, 30000); // 30 second timeout
    } catch (error) {
      const apiError = error as ApiError;
      console.error(`[Proxies] Regeneration failed for proxy ${proxyId}:`, apiError);

      // Don't show error alerts for 409 conflicts - let the RefreshButton handle it
      if (apiError.status !== 409) {
        setErrors((prev) => ({
          ...prev,
          action: `Failed to start regeneration: ${apiError.message}`,
        }));
      }

      // Remove from regenerating state on error
      setRegeneratingProxies((prev) => {
        const newSet = new Set(prev);
        newSet.delete(proxyId);
        return newSet;
      });

      // Re-throw so RefreshButton can handle conflicts
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
      await loadProxies(); // Reload proxies after deletion
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

  const activeProxies = allProxies?.filter((p) => p.is_active).length || 0;
  const totalProxies = allProxies?.length || 0;

  // Helper function to get relay profile name by ID
  const getRelayProfileName = (profileId: string) => {
    const profile = relayProfiles.find((p) => p.id === profileId);
    return profile ? profile.name : profileId;
  };

  return (
    <TooltipProvider>
      <div className="space-y-6">
        {/* Header Section */}
        <div className="flex items-center justify-between">
          <div>
            <p className="text-muted-foreground">Manage streaming proxy configurations</p>
          </div>
          <div className="flex items-center gap-2">
            {!isOnline && <WifiOff className="h-5 w-5 text-destructive" />}
            <CreateProxyModal
              onCreateProxy={handleCreateProxy}
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

        {/* Statistics Cards */}
        <div className="grid gap-4 md:grid-cols-3">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">Total Proxies</CardTitle>
              <Play className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{totalProxies}</div>
              <p className="text-xs text-muted-foreground">{activeProxies} active</p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">Active Proxies</CardTitle>
              <Activity className="h-4 w-4 text-green-600" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{activeProxies}</div>
              <p className="text-xs text-muted-foreground">Currently active</p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">Regenerating</CardTitle>
              <Zap className="h-4 w-4 text-blue-600" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{regeneratingProxies.size}</div>
              <p className="text-xs text-muted-foreground">In progress</p>
            </CardContent>
          </Card>
        </div>

        {/* Filters Section */}
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
                    placeholder="Search proxies, URLs, sources, filters..."
                    value={searchTerm}
                    onChange={(e) => setSearchTerm(e.target.value)}
                    className="pl-8"
                    disabled={loading.proxies}
                    autoComplete="off"
                  />
                </div>
              </div>
              <Select
                value={filterStatus}
                onValueChange={(value) => setFilterStatus(value as 'all' | 'active' | 'inactive')}
                disabled={loading.proxies}
              >
                <SelectTrigger className="w-full sm:w-[180px]">
                  <SelectValue placeholder="Filter by status" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Status</SelectItem>
                  <SelectItem value="active">Active Only</SelectItem>
                  <SelectItem value="inactive">Inactive Only</SelectItem>
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

        {/* Proxies Display */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center justify-between">
              <span>
                Stream Proxies ({filteredProxies?.length || 0}
                {searchTerm || filterStatus !== 'all' ? ` of ${allProxies?.length || 0}` : ''})
              </span>
              {loading.proxies && <Loader2 className="h-4 w-4 animate-spin" />}
            </CardTitle>
            <CardDescription>
              Configure and manage your streaming proxy configurations
            </CardDescription>
          </CardHeader>
          <CardContent>
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
                    {loading.proxies && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                    Retry
                  </Button>
                </AlertDescription>
              </Alert>
            ) : (
              <>
                {viewMode === 'table' && (
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Name</TableHead>
                        <TableHead>Mode</TableHead>
                        <TableHead>Status</TableHead>
                        <TableHead>Channel Range</TableHead>
                        <TableHead>Settings</TableHead>
                        <TableHead>Last Updated</TableHead>
                        <TableHead>Actions</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {filteredProxies?.map((proxy) => (
                        <TableRow key={proxy.id}>
                          <TableCell>
                            <div>
                              <div className="font-medium">{proxy.name}</div>
                              {proxy.description && (
                                <div className="text-sm text-muted-foreground truncate max-w-[200px]">
                                  {proxy.description}
                                </div>
                              )}
                            </div>
                          </TableCell>
                          <TableCell>
                            {proxy.proxy_mode === 'relay' && proxy.relay_profile_id ? (
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <Badge variant="outline" className="cursor-help">
                                    {proxy.proxy_mode.toUpperCase()}
                                  </Badge>
                                </TooltipTrigger>
                                <TooltipContent>
                                  <div className="space-y-1">
                                    <p className="font-medium text-sm">Relay Profile</p>
                                    <p className="text-xs text-muted-foreground">
                                      {getRelayProfileName(proxy.relay_profile_id)}
                                    </p>
                                  </div>
                                </TooltipContent>
                              </Tooltip>
                            ) : (
                              <Badge variant="outline">{proxy.proxy_mode.toUpperCase()}</Badge>
                            )}
                          </TableCell>
                          <TableCell>
                            <Badge className={getStatusColor(proxy.is_active)}>
                              {proxy.is_active ? 'Active' : 'Inactive'}
                            </Badge>
                          </TableCell>
                          <TableCell>
                            <div className="text-sm">Ch {proxy.starting_channel_number}+</div>
                          </TableCell>
                          <TableCell>
                            <div className="text-xs space-y-1">
                              {proxy.auto_regenerate && (
                                <div className="flex items-center gap-1">
                                  <RefreshCw className="h-3 w-3" />
                                  Auto-regen
                                </div>
                              )}
                              {proxy.cache_channel_logos && (
                                <div className="flex items-center gap-1">
                                  <Settings className="h-3 w-3" />
                                  Channel logos
                                </div>
                              )}
                              {proxy.cache_program_logos && (
                                <div className="flex items-center gap-1">
                                  <Settings className="h-3 w-3" />
                                  Program logos
                                </div>
                              )}
                            </div>
                          </TableCell>
                          <TableCell>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <div className="text-sm cursor-help">
                                  {formatRelativeTime(proxy.updated_at)}
                                </div>
                              </TooltipTrigger>
                              <TooltipContent>
                                <p className="text-sm">{formatDate(proxy.updated_at)}</p>
                              </TooltipContent>
                            </Tooltip>
                          </TableCell>
                          <TableCell>
                            <div className="flex items-center gap-2">
                              {/* Copy m3u8 URL button */}
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <Button
                                    variant="ghost"
                                    size="sm"
                                    onClick={() => {
                                      if (proxy.m3u8_url) {
                                        copyToClipboard(proxy.m3u8_url, 'm3u8', proxy.id);
                                      }
                                    }}
                                    className="h-8 w-8 p-0"
                                    disabled={!isOnline || !proxy.m3u8_url}
                                  >
                                    {copiedUrls.has(`${proxy.id}-m3u8`) ? (
                                      <Check className="h-4 w-4 text-green-600" />
                                    ) : (
                                      <Copy className="h-4 w-4" />
                                    )}
                                  </Button>
                                </TooltipTrigger>
                                <TooltipContent>
                                  <p className="text-sm">Copy M3U8</p>
                                </TooltipContent>
                              </Tooltip>
                              {/* Copy xmltv URL button */}
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <Button
                                    variant="ghost"
                                    size="sm"
                                    onClick={() => {
                                      if (proxy.xmltv_url) {
                                        copyToClipboard(proxy.xmltv_url, 'xmltv', proxy.id);
                                      }
                                    }}
                                    className="h-8 w-8 p-0"
                                    disabled={!isOnline || !proxy.xmltv_url}
                                  >
                                    {copiedUrls.has(`${proxy.id}-xmltv`) ? (
                                      <Check className="h-4 w-4 text-green-600" />
                                    ) : (
                                      <Copy className="h-4 w-4" />
                                    )}
                                  </Button>
                                </TooltipTrigger>
                                <TooltipContent>
                                  <p className="text-sm">Copy XMLTV</p>
                                </TooltipContent>
                              </Tooltip>
                              <RefreshButton
                                resourceId={proxy.id}
                                onRefresh={() => handleRegenerateProxy(proxy.id)}
                                disabled={!isOnline}
                                size="sm"
                                className="h-8 w-8 p-0"
                                tooltipText="Regenerate"
                              />
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <Button
                                    variant="ghost"
                                    size="sm"
                                    onClick={() => {
                                      setEditingProxy(proxy);
                                      setIsEditSheetOpen(true);
                                    }}
                                    className="h-8 w-8 p-0"
                                    disabled={!proxy}
                                  >
                                    <Edit className="h-4 w-4" />
                                  </Button>
                                </TooltipTrigger>
                                <TooltipContent>
                                  <p className="text-sm">Edit</p>
                                </TooltipContent>
                              </Tooltip>
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <Button
                                    variant="ghost"
                                    size="sm"
                                    onClick={() => handleDeleteProxy(proxy.id)}
                                    className="h-8 w-8 p-0 text-destructive hover:text-destructive"
                                    disabled={loading.delete === proxy.id || !isOnline}
                                  >
                                    {loading.delete === proxy.id ? (
                                      <Loader2 className="h-4 w-4 animate-spin" />
                                    ) : (
                                      <Trash2 className="h-4 w-4" />
                                    )}
                                  </Button>
                                </TooltipTrigger>
                                <TooltipContent>
                                  <p className="text-sm">Delete</p>
                                </TooltipContent>
                              </Tooltip>
                            </div>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                )}

                {viewMode === 'grid' && (
                  <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
                    {filteredProxies?.map((proxy) => (
                      <Card
                        key={proxy.id}
                        className={`transition-all hover:shadow-md ${!proxy.is_active ? 'opacity-60' : ''}`}
                      >
                        <CardHeader>
                          <div className="flex items-start justify-between">
                            <div className="space-y-1 flex-1">
                              <CardTitle className="text-lg flex items-center gap-2">
                                {proxy.name}
                                <Badge className={getStatusColor(proxy.is_active)}>
                                  {proxy.is_active ? 'Active' : 'Inactive'}
                                </Badge>
                              </CardTitle>
                              {proxy.description && (
                                <CardDescription className="line-clamp-2">
                                  {proxy.description}
                                </CardDescription>
                              )}
                            </div>
                          </div>
                        </CardHeader>
                        <CardContent>
                          <div className="space-y-4">
                            <div className="flex flex-wrap gap-2">
                              {proxy.proxy_mode === 'relay' && proxy.relay_profile_id ? (
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <Badge variant="outline" className="cursor-help">
                                      {proxy.proxy_mode.toUpperCase()}
                                    </Badge>
                                  </TooltipTrigger>
                                  <TooltipContent>
                                    <div className="space-y-1">
                                      <p className="font-medium text-sm">Relay Profile</p>
                                      <p className="text-xs text-muted-foreground">
                                        {getRelayProfileName(proxy.relay_profile_id)}
                                      </p>
                                    </div>
                                  </TooltipContent>
                                </Tooltip>
                              ) : (
                                <Badge variant="outline">{proxy.proxy_mode.toUpperCase()}</Badge>
                              )}
                              <Badge variant="secondary">Ch {proxy.starting_channel_number}+</Badge>
                            </div>

                            <div className="text-sm space-y-2">
                              {proxy.auto_regenerate && (
                                <div className="flex items-center gap-2">
                                  <RefreshCw className="h-4 w-4 text-muted-foreground" />
                                  <span>Auto-regenerate enabled</span>
                                </div>
                              )}
                              {proxy.cache_channel_logos && (
                                <div className="flex items-center gap-2">
                                  <Settings className="h-4 w-4 text-muted-foreground" />
                                  <span>Channel logos cached</span>
                                </div>
                              )}
                              {proxy.cache_program_logos && (
                                <div className="flex items-center gap-2">
                                  <Settings className="h-4 w-4 text-muted-foreground" />
                                  <span>Program logos cached</span>
                                </div>
                              )}
                            </div>

                            <div className="text-xs text-muted-foreground">
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <span className="cursor-help">
                                    Updated {formatRelativeTime(proxy.updated_at)}
                                  </span>
                                </TooltipTrigger>
                                <TooltipContent>
                                  <p className="text-sm">{formatDate(proxy.updated_at)}</p>
                                </TooltipContent>
                              </Tooltip>
                            </div>

                            <div className="flex items-center justify-between pt-2 border-t">
                              <div className="flex items-center gap-1">
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <Button
                                      variant="ghost"
                                      size="sm"
                                      onClick={() => {
                                        if (proxy.m3u8_url) {
                                          copyToClipboard(proxy.m3u8_url, 'm3u8', proxy.id);
                                        }
                                      }}
                                      className="h-8 w-8 p-0"
                                      disabled={!isOnline || !proxy.m3u8_url}
                                    >
                                      {copiedUrls.has(`${proxy.id}-m3u8`) ? (
                                        <Check className="h-4 w-4 text-green-600" />
                                      ) : (
                                        <Copy className="h-4 w-4" />
                                      )}
                                    </Button>
                                  </TooltipTrigger>
                                  <TooltipContent>
                                    <p className="text-sm">Copy M3U8</p>
                                  </TooltipContent>
                                </Tooltip>
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <Button
                                      variant="ghost"
                                      size="sm"
                                      onClick={() => {
                                        if (proxy.xmltv_url) {
                                          copyToClipboard(proxy.xmltv_url, 'xmltv', proxy.id);
                                        }
                                      }}
                                      className="h-8 w-8 p-0"
                                      disabled={!isOnline || !proxy.xmltv_url}
                                    >
                                      {copiedUrls.has(`${proxy.id}-xmltv`) ? (
                                        <Check className="h-4 w-4 text-green-600" />
                                      ) : (
                                        <Copy className="h-4 w-4" />
                                      )}
                                    </Button>
                                  </TooltipTrigger>
                                  <TooltipContent>
                                    <p className="text-sm">Copy XMLTV</p>
                                  </TooltipContent>
                                </Tooltip>
                                <RefreshButton
                                  resourceId={proxy.id}
                                  onRefresh={() => handleRegenerateProxy(proxy.id)}
                                  disabled={!isOnline}
                                  size="sm"
                                  className="h-8 w-8 p-0"
                                  tooltipText="Regenerate"
                                />
                              </div>
                              <div className="flex items-center gap-1">
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <Button
                                      variant="ghost"
                                      size="sm"
                                      onClick={() => {
                                        setEditingProxy(proxy);
                                        setIsEditSheetOpen(true);
                                      }}
                                      className="h-8 w-8 p-0"
                                    >
                                      <Edit className="h-4 w-4" />
                                    </Button>
                                  </TooltipTrigger>
                                  <TooltipContent>
                                    <p className="text-sm">Edit</p>
                                  </TooltipContent>
                                </Tooltip>
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <Button
                                      variant="ghost"
                                      size="sm"
                                      onClick={() => handleDeleteProxy(proxy.id)}
                                      className="h-8 w-8 p-0 text-destructive hover:text-destructive"
                                      disabled={loading.delete === proxy.id || !isOnline}
                                    >
                                      {loading.delete === proxy.id ? (
                                        <Loader2 className="h-4 w-4 animate-spin" />
                                      ) : (
                                        <Trash2 className="h-4 w-4" />
                                      )}
                                    </Button>
                                  </TooltipTrigger>
                                  <TooltipContent>
                                    <p className="text-sm">Delete</p>
                                  </TooltipContent>
                                </Tooltip>
                              </div>
                            </div>
                          </div>
                        </CardContent>
                      </Card>
                    ))}
                  </div>
                )}

                {viewMode === 'list' && (
                  <div className="space-y-3">
                    {filteredProxies?.map((proxy) => (
                      <Card
                        key={proxy.id}
                        className={`transition-all hover:shadow-sm ${!proxy.is_active ? 'opacity-60' : ''}`}
                      >
                        <CardContent className="pt-6">
                          <div className="flex items-center justify-between">
                            <div className="flex items-center space-x-4 flex-1">
                              <div className="flex-1 min-w-0">
                                <div className="flex items-center gap-3">
                                  <div>
                                    <p className="font-medium text-sm">{proxy.name}</p>
                                    {proxy.description && (
                                      <p className="text-xs text-muted-foreground truncate max-w-[300px]">
                                        {proxy.description}
                                      </p>
                                    )}
                                  </div>
                                  <div className="flex items-center gap-2">
                                    {proxy.proxy_mode === 'relay' && proxy.relay_profile_id ? (
                                      <Tooltip>
                                        <TooltipTrigger asChild>
                                          <Badge variant="outline" className="cursor-help text-xs">
                                            {proxy.proxy_mode.toUpperCase()}
                                          </Badge>
                                        </TooltipTrigger>
                                        <TooltipContent>
                                          <div className="space-y-1">
                                            <p className="font-medium text-sm">Relay Profile</p>
                                            <p className="text-xs text-muted-foreground">
                                              {getRelayProfileName(proxy.relay_profile_id)}
                                            </p>
                                          </div>
                                        </TooltipContent>
                                      </Tooltip>
                                    ) : (
                                      <Badge variant="outline" className="text-xs">
                                        {proxy.proxy_mode.toUpperCase()}
                                      </Badge>
                                    )}
                                    <Badge className={getStatusColor(proxy.is_active)}>
                                      {proxy.is_active ? 'Active' : 'Inactive'}
                                    </Badge>
                                    <Badge variant="secondary" className="text-xs">
                                      Ch {proxy.starting_channel_number}+
                                    </Badge>
                                  </div>
                                </div>
                              </div>
                            </div>
                            <div className="flex items-center gap-2 ml-4">
                              <div className="flex items-center gap-1">
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <Button
                                      variant="ghost"
                                      size="sm"
                                      onClick={() => {
                                        if (proxy.m3u8_url) {
                                          copyToClipboard(proxy.m3u8_url, 'm3u8', proxy.id);
                                        }
                                      }}
                                      className="h-8 w-8 p-0"
                                      disabled={!isOnline || !proxy.m3u8_url}
                                    >
                                      {copiedUrls.has(`${proxy.id}-m3u8`) ? (
                                        <Check className="h-4 w-4 text-green-600" />
                                      ) : (
                                        <Copy className="h-4 w-4" />
                                      )}
                                    </Button>
                                  </TooltipTrigger>
                                  <TooltipContent>
                                    <p className="text-sm">Copy M3U8</p>
                                  </TooltipContent>
                                </Tooltip>
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <Button
                                      variant="ghost"
                                      size="sm"
                                      onClick={() => {
                                        if (proxy.xmltv_url) {
                                          copyToClipboard(proxy.xmltv_url, 'xmltv', proxy.id);
                                        }
                                      }}
                                      className="h-8 w-8 p-0"
                                      disabled={!isOnline || !proxy.xmltv_url}
                                    >
                                      {copiedUrls.has(`${proxy.id}-xmltv`) ? (
                                        <Check className="h-4 w-4 text-green-600" />
                                      ) : (
                                        <Copy className="h-4 w-4" />
                                      )}
                                    </Button>
                                  </TooltipTrigger>
                                  <TooltipContent>
                                    <p className="text-sm">Copy XMLTV</p>
                                  </TooltipContent>
                                </Tooltip>
                                <RefreshButton
                                  resourceId={proxy.id}
                                  onRefresh={() => handleRegenerateProxy(proxy.id)}
                                  disabled={!isOnline}
                                  size="sm"
                                  className="h-8 w-8 p-0"
                                  tooltipText="Regenerate"
                                />
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <Button
                                      variant="ghost"
                                      size="sm"
                                      onClick={() => {
                                        setEditingProxy(proxy);
                                        setIsEditSheetOpen(true);
                                      }}
                                      className="h-8 w-8 p-0"
                                    >
                                      <Edit className="h-4 w-4" />
                                    </Button>
                                  </TooltipTrigger>
                                  <TooltipContent>
                                    <p className="text-sm">Edit</p>
                                  </TooltipContent>
                                </Tooltip>
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <Button
                                      variant="ghost"
                                      size="sm"
                                      onClick={() => handleDeleteProxy(proxy.id)}
                                      className="h-8 w-8 p-0 text-destructive hover:text-destructive"
                                      disabled={loading.delete === proxy.id || !isOnline}
                                    >
                                      {loading.delete === proxy.id ? (
                                        <Loader2 className="h-4 w-4 animate-spin" />
                                      ) : (
                                        <Trash2 className="h-4 w-4" />
                                      )}
                                    </Button>
                                  </TooltipTrigger>
                                  <TooltipContent>
                                    <p className="text-sm">Delete</p>
                                  </TooltipContent>
                                </Tooltip>
                              </div>
                            </div>
                          </div>
                        </CardContent>
                      </Card>
                    ))}
                  </div>
                )}

                {filteredProxies?.length === 0 && !loading.proxies && (
                  <div className="text-center py-8">
                    <Play className="mx-auto h-12 w-12 text-muted-foreground" />
                    <h3 className="mt-4 text-lg font-semibold">
                      {searchTerm || filterStatus !== 'all'
                        ? 'No matching proxies'
                        : 'No proxies found'}
                    </h3>
                    <p className="text-muted-foreground">
                      {searchTerm || filterStatus !== 'all'
                        ? 'Try adjusting your search or filter criteria.'
                        : 'Get started by creating your first proxy configuration.'}
                    </p>
                  </div>
                )}

                {/* Pagination */}
                {pagination && pagination.total_pages > 1 && (
                  <div className="flex items-center justify-between pt-4">
                    <div className="text-sm text-muted-foreground">
                      Showing {(pagination.page - 1) * pagination.per_page + 1} to{' '}
                      {Math.min(pagination.page * pagination.per_page, pagination.total)} of{' '}
                      {pagination.total} proxies
                    </div>
                    <div className="flex items-center gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setCurrentPage((prev) => Math.max(1, prev - 1))}
                        disabled={!pagination.has_previous || loading.proxies}
                      >
                        Previous
                      </Button>
                      <span className="text-sm">
                        Page {pagination.page} of {pagination.total_pages}
                      </span>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setCurrentPage((prev) => prev + 1)}
                        disabled={!pagination.has_next || loading.proxies}
                      >
                        Next
                      </Button>
                    </div>
                  </div>
                )}
              </>
            )}
          </CardContent>
        </Card>

        {/* Edit Proxy Sheet */}
        {editingProxy && (
          <ProxySheet
            mode="edit"
            proxy={editingProxy}
            onSaveProxy={handleUpdateProxy}
            loading={loading.edit}
            error={errors.edit}
            open={isEditSheetOpen}
            onOpenChange={setIsEditSheetOpen}
          />
        )}
      </div>
    </TooltipProvider>
  );
}
