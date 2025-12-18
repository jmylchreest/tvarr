'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { StatCard } from '@/components/shared/feedback/StatCard';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
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
  Plus,
  Server,
  Trash2,
  Archive,
  Search,
  AlertCircle,
  Loader2,
  WifiOff,
  Clock,
} from 'lucide-react';
import {
  EpgSourceResponse,
  CreateEpgSourceRequest,
  EpgSourceType,
  XtreamApiMethod,
  PaginatedResponse,
} from '@/types/api';
import { apiClient, ApiError } from '@/lib/api-client';
import { DEFAULT_PAGE_SIZE, API_CONFIG } from '@/lib/config';
import { RefreshButton } from '@/components/RefreshButton';
import { useConflictHandler } from '@/hooks/useConflictHandler';
import { ConflictNotification } from '@/components/ConflictNotification';
import { useProgressContext } from '@/providers/ProgressProvider';
import { formatDate, formatRelativeTime } from '@/lib/utils';
import {
  validateCronExpression,
  describeCronExpression,
  COMMON_CRON_TEMPLATES,
} from '@/lib/cron-validation';
import {
  MasterDetailLayout,
  DetailPanel,
  DetailEmpty,
  MasterItem,
} from '@/components/shared';
import { ScrollArea } from '@/components/ui/scroll-area';
import { OperationStatusIndicator } from '@/components/OperationStatusIndicator';
import { AnimatedBadgeGroup } from '@/components/shared/AnimatedBadgeGroup';

// Helper to get UTC offset for a timezone string
function getTimezoneOffset(timezone: string): string | null {
  if (!timezone) return null;

  // If it's already an offset like "+01:00" or "-05:00", return as-is
  if (/^[+-]\d{2}:\d{2}$/.test(timezone)) {
    return timezone;
  }

  // Try to get the offset for a named timezone (e.g., "Europe/Amsterdam")
  try {
    const now = new Date();
    const formatter = new Intl.DateTimeFormat('en-US', {
      timeZone: timezone,
      timeZoneName: 'shortOffset',
    });
    const parts = formatter.formatToParts(now);
    const offsetPart = parts.find((p) => p.type === 'timeZoneName');
    if (offsetPart) {
      // Convert "GMT+1" to "+01:00" format
      const match = offsetPart.value.match(/GMT([+-]?)(\d+)?(?::(\d+))?/);
      if (match) {
        const sign = match[1] || '+';
        const hours = match[2] ? match[2].padStart(2, '0') : '00';
        const minutes = match[3] ? match[3].padStart(2, '0') : '00';
        return `${sign}${hours}:${minutes}`;
      }
      return offsetPart.value;
    }
  } catch {
    // Invalid timezone, return null
  }
  return null;
}

// Format detected timezone with offset for display
function formatDetectedTimezone(timezone: string | undefined): { display: string; tooltip: string } {
  if (!timezone) {
    return { display: 'Not detected yet', tooltip: '' };
  }

  const offset = getTimezoneOffset(timezone);

  // If the timezone is already an offset format, just display it
  if (/^[+-]\d{2}:\d{2}$/.test(timezone)) {
    return { display: `UTC${timezone}`, tooltip: '' };
  }

  // For named timezones, show name with offset
  if (offset) {
    return {
      display: `${timezone} (UTC${offset})`,
      tooltip: `UTC offset: ${offset}`,
    };
  }

  return { display: timezone, tooltip: '' };
}

interface LoadingState {
  sources: boolean;
  create: boolean;
  edit: boolean;
  delete: string | null;
}

interface ErrorState {
  sources: string | null;
  create: string | null;
  edit: string | null;
  action: string | null;
}

function getSourceTypeColor(type: EpgSourceType): string {
  switch (type) {
    case 'xmltv':
      return 'bg-purple-100 text-purple-800';
    case 'xtream':
      return 'bg-green-100 text-green-800';
    default:
      return 'bg-gray-100 text-gray-800';
  }
}

function getStatusColor(status: string): string {
  switch (status) {
    case 'success':
      return 'bg-green-100 text-green-800';
    case 'ingesting':
      return 'bg-blue-100 text-blue-800';
    case 'pending':
      return 'bg-yellow-100 text-yellow-800';
    case 'failed':
      return 'bg-red-100 text-red-800';
    default:
      return 'bg-gray-100 text-gray-800';
  }
}

function getStatusLabel(status: string): string {
  switch (status) {
    case 'success':
      return 'Success';
    case 'ingesting':
      return 'Ingesting';
    case 'pending':
      return 'Pending';
    case 'failed':
      return 'Failed';
    default:
      return status;
  }
}

// Convert EpgSourceResponse to MasterItem format for MasterDetailLayout
interface EpgSourceMasterItem extends MasterItem {
  source: EpgSourceResponse;
}

function epgSourceToMasterItem(source: EpgSourceResponse): EpgSourceMasterItem {
  // Map status to badge priority
  const getStatusPriority = (status: string) => {
    switch (status) {
      case 'failed':
        return 'error' as const;
      case 'pending':
      case 'ingesting':
        return 'outline' as const;
      default:
        return 'secondary' as const;
    }
  };

  return {
    id: source.id,
    title: source.name,
    badge: (
      <div className="flex items-center gap-1">
        <AnimatedBadgeGroup
          resourceId={source.id}
          badges={[
            { label: source.source_type, priority: 'info' },
            { label: getStatusLabel(source.status), priority: getStatusPriority(source.status) },
          ]}
        />
        <OperationStatusIndicator resourceId={source.id} />
      </div>
    ),
    source,
  };
}


// Create panel for creating a new EPG source inline in detail area
function EpgSourceCreatePanel({
  onCreate,
  onCancel,
  loading,
  error,
}: {
  onCreate: (source: CreateEpgSourceRequest) => Promise<void>;
  onCancel: () => void;
  loading: boolean;
  error: string | null;
}) {
  const [formData, setFormData] = useState<CreateEpgSourceRequest>({
    name: '',
    source_type: 'xtream',
    url: '',
    update_cron: '0 0 */6 * * *',
    epg_shift: 0,
    username: '',
    password: '',
    api_method: 'stream_id',
  });
  const [cronValidation, setCronValidation] = useState(validateCronExpression('0 0 */6 * * *'));

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await onCreate(formData);
    } catch {
      // Error handled by parent
    }
  };

  const isValid = formData.name.trim().length > 0 && formData.url.trim().length > 0 && cronValidation.isValid;

  return (
    <DetailPanel
      title="Add EPG Source"
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
              placeholder="Premium EPG Data"
              required
              disabled={loading}
              autoFocus
              autoComplete="off"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="create-source_type">Source Type</Label>
            <Select
              value={formData.source_type}
              onValueChange={(value) =>
                setFormData({ ...formData, source_type: value as EpgSourceType })
              }
              disabled={loading}
            >
              <SelectTrigger>
                <SelectValue placeholder="Select source type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="xmltv">XMLTV</SelectItem>
                <SelectItem value="xtream">Xtream Codes</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2">
            <Label htmlFor="create-url">URL</Label>
            <Input
              id="create-url"
              value={formData.url}
              onChange={(e) => setFormData({ ...formData, url: e.target.value })}
              placeholder={
                formData.source_type === 'xmltv'
                  ? 'https://example.com/epg.xml'
                  : 'http://xtream.example.com:8080'
              }
              required
              disabled={loading}
              autoComplete="off"
            />
          </div>

          {formData.source_type === 'xtream' && (
            <div className="space-y-2">
              <Label htmlFor="create-api_method">API Method</Label>
              <Select
                value={formData.api_method || 'stream_id'}
                onValueChange={(value) =>
                  setFormData({ ...formData, api_method: value as XtreamApiMethod })
                }
                disabled={loading}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select API method" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="stream_id">Xtream StreamID (richer)</SelectItem>
                  <SelectItem value="bulk_xmltv">Bulk XMLTV (faster)</SelectItem>
                </SelectContent>
              </Select>
            </div>
          )}

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="create-username">Username</Label>
              <Input
                id="create-username"
                value={formData.username || ''}
                onChange={(e) => setFormData({ ...formData, username: e.target.value })}
                placeholder="Optional"
                disabled={loading}
                autoComplete="off"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="create-password">Password</Label>
              <Input
                id="create-password"
                type="password"
                value={formData.password || ''}
                onChange={(e) => setFormData({ ...formData, password: e.target.value })}
                placeholder="Optional"
                disabled={loading}
                autoComplete="off"
              />
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="create-epg_shift">EPG Time Shift (hours)</Label>
            <Input
              id="create-epg_shift"
              type="number"
              min={-12}
              max={12}
              value={formData.epg_shift ?? 0}
              onChange={(e) => {
                const value = parseInt(e.target.value) || 0;
                setFormData({ ...formData, epg_shift: Math.max(-12, Math.min(12, value)) });
              }}
              placeholder="0"
              disabled={loading}
              autoComplete="off"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="create-update_cron">Update Schedule (Cron)</Label>
            <Input
              id="create-update_cron"
              value={formData.update_cron}
              onChange={(e) => {
                const newValue = e.target.value;
                setFormData({ ...formData, update_cron: newValue });
                setCronValidation(validateCronExpression(newValue));
              }}
              placeholder="0 0 */6 * * *"
              required
              disabled={loading}
              autoComplete="off"
              className={cronValidation.isValid ? '' : 'border-destructive'}
            />
            {!cronValidation.isValid && cronValidation.error && (
              <p className="text-sm text-destructive">{cronValidation.error}</p>
            )}
            <div className="flex flex-wrap gap-1">
              {COMMON_CRON_TEMPLATES.slice(0, 3).map((template) => (
                <Button
                  key={template.expression}
                  variant="ghost"
                  size="sm"
                  className="h-6 px-2 text-xs"
                  onClick={() => {
                    setFormData({ ...formData, update_cron: template.expression });
                    setCronValidation(validateCronExpression(template.expression));
                  }}
                  disabled={loading}
                  type="button"
                >
                  {template.description}
                </Button>
              ))}
            </div>
          </div>
        </form>
      </div>
    </DetailPanel>
  );
}

// Detail panel for viewing/editing a selected EPG source
function EpgSourceDetailPanel({
  source,
  onUpdate,
  onRefresh,
  onDelete,
  loading,
  error,
  isOnline,
}: {
  source: EpgSourceResponse;
  onUpdate: (id: string, data: CreateEpgSourceRequest) => Promise<void>;
  onRefresh: (id: string) => Promise<void>;
  onDelete: (id: string) => Promise<void>;
  loading: { edit: boolean; delete: string | null };
  error: string | null;
  isOnline: boolean;
}) {
  const [formData, setFormData] = useState<CreateEpgSourceRequest>({
    name: source.name,
    source_type: source.source_type,
    url: source.url,
    update_cron: source.update_cron || '0 0 */6 * * *',
    epg_shift: source.epg_shift ?? 0,
    username: source.username || '',
    password: '',
    api_method: source.api_method || 'stream_id',
  });
  const [cronValidation, setCronValidation] = useState(
    validateCronExpression(source.update_cron || '0 0 */6 * * *')
  );
  const [hasChanges, setHasChanges] = useState(false);

  // Reset form when source changes
  useEffect(() => {
    const newFormData = {
      name: source.name,
      source_type: source.source_type,
      url: source.url,
      update_cron: source.update_cron || '0 0 */6 * * *',
      epg_shift: source.epg_shift ?? 0,
      username: source.username || '',
      password: '',
      api_method: source.api_method || 'stream_id',
    };
    setFormData(newFormData);
    setCronValidation(validateCronExpression(newFormData.update_cron));
    setHasChanges(false);
  }, [source.id]);

  const handleFieldChange = (field: keyof CreateEpgSourceRequest, value: any) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
    setHasChanges(true);
    if (field === 'update_cron') {
      setCronValidation(validateCronExpression(value));
    }
  };

  const handleSave = async () => {
    const updateData = { ...formData };
    if (!updateData.password || updateData.password.trim() === '') {
      delete updateData.password;
    }
    await onUpdate(source.id, updateData);
    setHasChanges(false);
  };

  return (
    <DetailPanel
      title={source.name}
      actions={
        <div className="flex items-center gap-2">
          <RefreshButton
            resourceId={source.id}
            onRefresh={() => onRefresh(source.id)}
            disabled={!isOnline}
            size="sm"
          />
          <Button
            variant="outline"
            size="sm"
            onClick={() => onDelete(source.id)}
            disabled={loading.delete === source.id || !isOnline}
            className="text-destructive hover:text-destructive"
          >
            {loading.delete === source.id ? (
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

        {/* Source Info Banner */}
        <div className="flex items-center gap-4 p-4 bg-muted/50 rounded-lg">
          <div className="flex items-center gap-2">
            <Badge variant="secondary">
              {source.source_type.toUpperCase()}
            </Badge>
            <Badge variant={source.status === 'success' ? 'secondary' : source.status === 'failed' ? 'destructive' : 'outline'}>
              {getStatusLabel(source.status)}
            </Badge>
            <OperationStatusIndicator resourceId={source.id} />
          </div>
          <div className="flex-1" />
          <div className="text-sm text-muted-foreground">
            <Archive className="h-4 w-4 inline mr-1" />
            {source.program_count} programs
          </div>
          {source.last_ingestion_at && (
            <div className="text-sm text-muted-foreground">
              <Clock className="h-4 w-4 inline mr-1" />
              {formatRelativeTime(source.last_ingestion_at)}
            </div>
          )}
          {source.detected_timezone && (
            <div className="text-sm text-muted-foreground">
              TZ: {formatDetectedTimezone(source.detected_timezone).display}
            </div>
          )}
        </div>

        {/* Edit Form */}
        <div className="border-t pt-4 space-y-4">
          <h3 className="text-sm font-medium">Configuration</h3>

          <div className="space-y-2">
            <Label htmlFor="detail-name">Name</Label>
            <Input
              id="detail-name"
              value={formData.name}
              onChange={(e) => handleFieldChange('name', e.target.value)}
              disabled={loading.edit || !isOnline}
              autoComplete="off"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="detail-url">URL</Label>
            <Input
              id="detail-url"
              value={formData.url}
              onChange={(e) => handleFieldChange('url', e.target.value)}
              disabled={loading.edit || !isOnline}
              autoComplete="off"
            />
          </div>

          {source.source_type === 'xtream' && (
            <div className="space-y-2">
              <Label htmlFor="detail-api_method">API Method</Label>
              <Select
                value={formData.api_method || 'stream_id'}
                onValueChange={(value) => handleFieldChange('api_method', value as XtreamApiMethod)}
                disabled={loading.edit || !isOnline}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select API method" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="stream_id">Xtream StreamID (richer)</SelectItem>
                  <SelectItem value="bulk_xmltv">Bulk XMLTV (faster)</SelectItem>
                </SelectContent>
              </Select>
            </div>
          )}

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="detail-username">Username</Label>
              <Input
                id="detail-username"
                value={formData.username || ''}
                onChange={(e) => handleFieldChange('username', e.target.value)}
                placeholder="Optional"
                disabled={loading.edit || !isOnline}
                autoComplete="off"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="detail-password">Password</Label>
              <Input
                id="detail-password"
                type="password"
                value={formData.password || ''}
                onChange={(e) => handleFieldChange('password', e.target.value)}
                placeholder="Leave empty to keep current"
                disabled={loading.edit || !isOnline}
                autoComplete="off"
              />
            </div>
          </div>

          <div className="space-y-2">
            <div className="flex items-center gap-1">
              <Label htmlFor="detail-epg_shift">EPG Time Shift (hours)</Label>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button variant="ghost" size="sm" className="h-5 w-5 p-0 text-muted-foreground">
                    ?
                  </Button>
                </TooltipTrigger>
                <TooltipContent className="max-w-sm">
                  <p className="text-sm">
                    Adjust normalised UTC times (-12 to +12 hours). Auto-set based on detected timezone.
                  </p>
                </TooltipContent>
              </Tooltip>
            </div>
            <Input
              id="detail-epg_shift"
              type="number"
              min={-12}
              max={12}
              value={formData.epg_shift ?? 0}
              onChange={(e) => {
                const value = parseInt(e.target.value) || 0;
                handleFieldChange('epg_shift', Math.max(-12, Math.min(12, value)));
              }}
              disabled={loading.edit || !isOnline}
              autoComplete="off"
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="detail-update_cron">Update Schedule (Cron)</Label>
            <Input
              id="detail-update_cron"
              value={formData.update_cron}
              onChange={(e) => handleFieldChange('update_cron', e.target.value)}
              disabled={loading.edit || !isOnline}
              autoComplete="off"
              className={cronValidation.isValid ? '' : 'border-destructive'}
            />
            {!cronValidation.isValid && cronValidation.error && (
              <p className="text-sm text-destructive">{cronValidation.error}</p>
            )}
            <div className="flex flex-wrap gap-1">
              {COMMON_CRON_TEMPLATES.slice(0, 3).map((template) => (
                <Button
                  key={template.expression}
                  variant="ghost"
                  size="sm"
                  className="h-6 px-2 text-xs"
                  onClick={() => handleFieldChange('update_cron', template.expression)}
                  disabled={loading.edit || !isOnline}
                  type="button"
                >
                  {template.description}
                </Button>
              ))}
            </div>
          </div>

          {/* Save Button */}
          {hasChanges && (
            <div className="flex justify-end pt-4 border-t">
              <Button
                onClick={handleSave}
                disabled={loading.edit || !isOnline || !cronValidation.isValid}
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

export function EpgSources() {
  const progressContext = useProgressContext();
  const [allSources, setAllSources] = useState<EpgSourceResponse[]>([]);
  const [pagination, setPagination] = useState<Omit<
    PaginatedResponse<EpgSourceResponse>,
    'items'
  > | null>(null);

  const [loading, setLoading] = useState<LoadingState>({
    sources: false,
    create: false,
    edit: false,
    delete: null,
  });

  const [errors, setErrors] = useState<ErrorState>({
    sources: null,
    create: null,
    edit: null,
    action: null,
  });

  const [selectedSource, setSelectedSource] = useState<EpgSourceMasterItem | null>(null);
  const { handleApiError, dismissConflict, getConflictState } = useConflictHandler();
  const [refreshingSources, setRefreshingSources] = useState<Set<string>>(new Set());
  const [isOnline, setIsOnline] = useState(true);
  const [isCreating, setIsCreating] = useState(false);

  // Sort sources alphabetically by name
  const sortedSources = useMemo(() => {
    return [...allSources].sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true }));
  }, [allSources]);

  // Convert sources to master items for the layout
  const masterItems = useMemo(
    () => sortedSources.map(epgSourceToMasterItem),
    [sortedSources]
  );

  // Health check is handled by parent component, no need for redundant calls

  // Initialize SSE connection on mount for EPG ingestion events
  useEffect(() => {
    // Listen for any EPG ingestion events to update refresh states
    const handleGlobalEpgEvent = (event: any) => {
      console.log('[EpgSources] Received global EPG ingestion event:', event);

      // If we see an operation starting (idle or processing state), add it to refreshing set
      if (
        (event.state === 'idle' || event.state === 'processing') &&
        event.id &&
        event.operation_type === 'epg_ingestion'
      ) {
        console.log(`[EpgSources] Adding ${event.id} to refreshing set (state: ${event.state})`);
        setRefreshingSources((prev) => {
          const newSet = new Set(prev);
          newSet.add(event.id);
          return newSet;
        });
      }

      // If we see a completion event, remove from refreshing set and reload sources
      if (
        (event.state === 'completed' || event.state === 'error') &&
        event.id &&
        event.operation_type === 'epg_ingestion'
      ) {
        console.log(
          `[EpgSources] Removing ${event.id} from refreshing set (state: ${event.state})`
        );
        setRefreshingSources((prev) => {
          const newSet = new Set(prev);
          const wasRefreshing = newSet.has(event.id);
          newSet.delete(event.id);
          // Reload sources when refresh completes
          if (wasRefreshing) {
            setTimeout(() => loadSources(), 1000);
          }
          return newSet;
        });
      }
    };

    const unsubscribe = progressContext.subscribeToType('epg_ingestion', handleGlobalEpgEvent);

    return () => {
      console.log('[EpgSources] Component unmounting, unsubscribing from EPG events');
      unsubscribe();
    };
  }, []);

  const loadSources = useCallback(async () => {
    if (!isOnline) return;

    setLoading((prev) => ({ ...prev, sources: true }));
    setErrors((prev) => ({ ...prev, sources: null }));

    try {
      // Load all sources without search parameters - filtering happens locally
      const response = await apiClient.getEpgSources();

      setAllSources(response.items);
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
          sources: `Unable to connect to the API service. Please check that the service is running at ${API_CONFIG.baseUrl}.`,
        }));
      } else {
        setErrors((prev) => ({
          ...prev,
          sources: `Failed to load EPG sources: ${apiError.message}`,
        }));
      }
    } finally {
      setLoading((prev) => ({ ...prev, sources: false }));
    }
  }, [isOnline]);

  // Load sources on mount only
  useEffect(() => {
    loadSources();
  }, [loadSources]);

  const handleCreateSource = async (newSource: CreateEpgSourceRequest) => {
    setLoading((prev) => ({ ...prev, create: true }));
    setErrors((prev) => ({ ...prev, create: null }));

    try {
      const created = await apiClient.createEpgSource(newSource);
      await loadSources(); // Reload sources after creation
      setIsCreating(false);
      // Select the newly created source
      if (created?.id) {
        const newMasterItem = epgSourceToMasterItem(created);
        setSelectedSource(newMasterItem);
      }
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        create: `Failed to create EPG source: ${apiError.message}`,
      }));
      throw error; // Re-throw to prevent dialog from closing
    } finally {
      setLoading((prev) => ({ ...prev, create: false }));
    }
  };

  const handleUpdateSource = async (id: string, updatedSource: CreateEpgSourceRequest) => {
    setLoading((prev) => ({ ...prev, edit: true }));
    setErrors((prev) => ({ ...prev, edit: null }));

    try {
      await apiClient.updateEpgSource(id, updatedSource);
      await loadSources(); // Reload sources after update
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        edit: `Failed to update EPG source: ${apiError.message}`,
      }));
      throw error; // Re-throw to prevent dialog from closing
    } finally {
      setLoading((prev) => ({ ...prev, edit: false }));
    }
  };

  const handleRefreshSource = async (sourceId: string) => {
    console.log(`[EpgSources] Starting refresh for source: ${sourceId}`);
    setRefreshingSources((prev) => new Set(prev).add(sourceId));
    setErrors((prev) => ({ ...prev, action: null }));

    try {
      console.log(`[EpgSources] Calling API refresh for source: ${sourceId}`);
      await apiClient.refreshEpgSource(sourceId);
      console.log(`[EpgSources] API refresh call completed for source: ${sourceId}`);

      // Fallback timeout in case SSE events don't work
      setTimeout(() => {
        console.log(
          `[EpgSources] Fallback timeout - clearing refresh state for source: ${sourceId}`
        );
        setRefreshingSources((prev) => {
          const newSet = new Set(prev);
          if (newSet.has(sourceId)) {
            newSet.delete(sourceId);
            // Reload sources as fallback
            loadSources();
          }
          return newSet;
        });
      }, 30000); // 30 second timeout
    } catch (error) {
      const apiError = error as ApiError;
      console.error(`[EpgSources] Refresh failed for source ${sourceId}:`, apiError);

      // Don't show error alerts for 409 conflicts - let the RefreshButton handle it
      if (apiError.status !== 409) {
        setErrors((prev) => ({
          ...prev,
          action: `Failed to start refresh: ${apiError.message}`,
        }));
      }

      // Remove from refreshing state on error
      setRefreshingSources((prev) => {
        const newSet = new Set(prev);
        newSet.delete(sourceId);
        return newSet;
      });

      // Re-throw so RefreshButton can handle conflicts
      throw error;
    }
  };

  const handleDeleteSource = async (sourceId: string) => {
    if (
      !confirm('Are you sure you want to delete this EPG source? This action cannot be undone.')
    ) {
      return;
    }

    setLoading((prev) => ({ ...prev, delete: sourceId }));
    setErrors((prev) => ({ ...prev, action: null }));

    try {
      await apiClient.deleteEpgSource(sourceId);
      await loadSources(); // Reload sources after deletion
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        action: `Failed to delete EPG source: ${apiError.message}`,
      }));
    } finally {
      setLoading((prev) => ({ ...prev, delete: null }));
    }
  };

  const totalPrograms = allSources?.reduce((sum, source) => sum + source.program_count, 0) || 0;
  const successfulSources = allSources?.filter((s) => s.status === 'success').length || 0;
  const xmltvSources = allSources?.filter((s) => s.source_type === 'xmltv').length || 0;
  const xtreamSources = allSources?.filter((s) => s.source_type === 'xtream').length || 0;

  return (
    <TooltipProvider>
      <div className="space-y-6">
        {/* Header Section */}
        <div className="flex items-center justify-between">
          <p className="text-muted-foreground">
            Manage EPG sources, such as XMLTV and Xtream Code providers
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

        {/* Statistics Cards */}
        <div className="grid gap-2 md:grid-cols-4">
          <StatCard title="Total Sources" value={pagination?.total || 0} icon={<Server className="h-4 w-4" />} />
          <StatCard title="Total Programs" value={totalPrograms} icon={<Archive className="h-4 w-4" />} />
          <StatCard title="XMLTV Sources" value={xmltvSources} icon={<Server className="h-4 w-4 text-purple-600" />} />
          <StatCard title="Xtream Sources" value={xtreamSources} icon={<Server className="h-4 w-4 text-green-600" />} />
        </div>

        {/* Error Loading Sources */}
        {errors.sources && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Failed to Load EPG Sources</AlertTitle>
            <AlertDescription>
              {errors.sources}
              <ConflictNotification
                show={getConflictState('epg-sources-retry').show}
                message={getConflictState('epg-sources-retry').message}
                onDismiss={() => dismissConflict('epg-sources-retry')}
              >
                <Button
                  variant="outline"
                  size="sm"
                  className="ml-2"
                  onClick={async () => {
                    try {
                      await loadSources();
                    } catch (error) {
                      handleApiError(error, 'epg-sources-retry', 'Load EPG sources');
                    }
                  }}
                  disabled={loading.sources}
                >
                  {loading.sources && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                  Retry
                </Button>
              </ConflictNotification>
            </AlertDescription>
          </Alert>
        )}

        {/* Master-Detail Layout */}
        <Card className="flex-1 overflow-hidden">
          <CardContent className="p-0 min-h-[500px] h-[calc(100vh-320px)]">
            <MasterDetailLayout
              items={masterItems}
              selectedId={selectedSource?.id}
              onSelect={(item) => {
                setSelectedSource(item);
                if (item) setIsCreating(false);
              }}
              isLoading={loading.sources}
              title={`EPG Sources (${sortedSources.length})`}
              searchPlaceholder="Search by name, type, status..."
              headerAction={
                <Button
                  size="sm"
                  variant="ghost"
                  className="h-8 w-8 p-0"
                  onClick={() => {
                    setIsCreating(true);
                    setSelectedSource(null);
                    setErrors((prev) => ({ ...prev, create: null }));
                  }}
                  disabled={isCreating}
                >
                  <Plus className="h-4 w-4" />
                </Button>
              }
              emptyState={{
                title: 'No EPG sources configured',
                description: 'Get started by creating your first EPG source.',
              }}
              filterFn={(item, term) => {
                const source = item.source;
                const lower = term.toLowerCase();
                // Search across name, url, source_type, status, program count
                const searchableFields = [
                  source.name,
                  source.url || '',
                  source.source_type,
                  source.status,
                  `${source.program_count} programs`,
                  source.enabled ? 'enabled' : 'disabled',
                ];
                return searchableFields.some(field => field.toLowerCase().includes(lower));
              }}
            >
              {(selected) =>
                isCreating ? (
                  <EpgSourceCreatePanel
                    onCreate={handleCreateSource}
                    onCancel={() => setIsCreating(false)}
                    loading={loading.create}
                    error={errors.create}
                  />
                ) : selected ? (
                  <EpgSourceDetailPanel
                    source={selected.source}
                    onUpdate={handleUpdateSource}
                    onRefresh={handleRefreshSource}
                    onDelete={handleDeleteSource}
                    loading={{ edit: loading.edit, delete: loading.delete }}
                    error={errors.edit}
                    isOnline={isOnline}
                  />
                ) : (
                  <DetailEmpty
                    title="Select an EPG source"
                    description="Choose an EPG source from the list to view details and edit configuration"
                    icon={<Server className="h-12 w-12 text-muted-foreground" />}
                  />
                )
              }
            </MasterDetailLayout>
          </CardContent>
        </Card>
      </div>
    </TooltipProvider>
  );
}
