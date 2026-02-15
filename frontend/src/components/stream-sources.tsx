'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
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
import { ScrollArea } from '@/components/ui/scroll-area';
import {
  Plus,
  Database,
  Trash2,
  RefreshCw,
  Clock,
  Monitor,
  Activity,
  AlertCircle,
  Loader2,
  WifiOff,
  Save,
  CheckCircle,
} from 'lucide-react';
import { RefreshButton } from '@/components/RefreshButton';
import { OperationStatusIndicator } from '@/components/OperationStatusIndicator';
import { AnimatedBadgeGroup } from '@/components/shared/AnimatedBadgeGroup';
import { useConflictHandler } from '@/hooks/useConflictHandler';
import { ConflictNotification } from '@/components/ConflictNotification';
import { useProgressContext } from '@/providers/ProgressProvider';
import {
  StreamSourceResponse,
  CreateStreamSourceRequest,
  UpdateStreamSourceRequest,
  StreamSourceType,
  PaginatedResponse,
} from '@/types/api';
import { apiClient, ApiError } from '@/lib/api-client';
import { API_CONFIG } from '@/lib/config';
import { formatDate, formatRelativeTime } from '@/lib/utils';
import { createFuzzyFilter } from '@/lib/fuzzy-search';
import {
  validateCronExpression,
  COMMON_CRON_TEMPLATES,
} from '@/lib/cron-validation';
import { ManualChannelEditor, ManualChannelInput } from '@/components/manual-channel-editor';
import { ManualM3UImportExport } from '@/components/manual-m3u-import-export';
import {
  MasterDetailLayout,
  DetailPanel,
  DetailEmpty,
  MasterItem,
} from '@/components/shared';
import { StatCard } from '@/components/shared/feedback/StatCard';

const USER_AGENT_PRESETS = [
  { value: '', label: 'tvarr (Default)' },
  { value: 'VLC/3.0.21', label: 'VLC 3.0.21' },
  { value: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36', label: 'Chrome 133 (Windows)' },
  { value: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36', label: 'Chrome 133 (macOS)' },
  { value: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:135.0) Gecko/20100101 Firefox/135.0', label: 'Firefox 135 (Windows)' },
  { value: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.3 Safari/605.1.15', label: 'Safari 18.3 (macOS)' },
  { value: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36 Edg/133.0.0.0', label: 'Edge 133 (Windows)' },
  { value: 'Lavf/60.16.100', label: 'FFmpeg (Lavf)' },
  { value: 'custom', label: 'Custom...' },
] as const;

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

function getSourceTypeColor(type: StreamSourceType): string {
  switch (type) {
    case 'm3u':
      return 'bg-blue-100 text-blue-800';
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

// Convert StreamSourceResponse to MasterItem format for MasterDetailLayout
interface SourceMasterItem extends MasterItem {
  source: StreamSourceResponse;
}

function sourceToMasterItem(source: StreamSourceResponse): SourceMasterItem {
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

/**
 * SourceCreatePanel - Inline panel for creating a new stream source
 */
function SourceCreatePanel({
  onCreate,
  onCancel,
  loading,
  error,
}: {
  onCreate: (source: CreateStreamSourceRequest) => Promise<void>;
  onCancel: () => void;
  loading: boolean;
  error: string | null;
}) {
  const [formData, setFormData] = useState<CreateStreamSourceRequest>({
    name: '',
    source_type: 'xtream',
    url: '',
    max_concurrent_streams: 0,
    update_cron: '0 0 */6 * * *',
    username: '',
    password: '',
    user_agent: '',
  });
  const [manualChannels, setManualChannels] = useState<ManualChannelInput[]>([]);
  const [manualValid, setManualValid] = useState(false);
  const [cronValidation, setCronValidation] = useState(validateCronExpression('0 0 */6 * * *'));
  const [customUserAgent, setCustomUserAgent] = useState('');
  const [showCustomUserAgent, setShowCustomUserAgent] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const payload: CreateStreamSourceRequest = {
      ...formData,
      ...(formData.source_type === 'manual' ? { manual_channels: manualChannels } : {}),
    };
    await onCreate(payload);
  };

  const isSubmitDisabled =
    loading ||
    !formData.name.trim() ||
    (formData.source_type !== 'manual' && !formData.url?.trim()) ||
    (formData.source_type === 'manual' && (!manualValid || manualChannels.length === 0)) ||
    !cronValidation.isValid;

  return (
    <DetailPanel
      title="Create Stream Source"
      actions={
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={onCancel} disabled={loading}>
            Cancel
          </Button>
          <Button
            size="sm"
            onClick={handleSubmit}
            disabled={isSubmitDisabled}
          >
            {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            <Save className="h-4 w-4 mr-1" />
            Create
          </Button>
        </div>
      }
    >
      {error && (
        <Alert variant="destructive" className="mb-4">
          <AlertCircle className="h-4 w-4" />
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      <form onSubmit={handleSubmit} className="space-y-6" autoComplete="off">
        {/* Basic Info */}
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label htmlFor="create-name">Name</Label>
            <Input
              id="create-name"
              value={formData.name}
              onChange={(e) => setFormData({ ...formData, name: e.target.value })}
              placeholder="Premium Sports Channel"
              required
              disabled={loading}
              autoComplete="off"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="create-source-type">Source Type</Label>
            <Select
              value={formData.source_type}
              onValueChange={(value) =>
                setFormData({ ...formData, source_type: value as StreamSourceType })
              }
              disabled={loading}
            >
              <SelectTrigger id="create-source-type">
                <SelectValue placeholder="Select source type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="m3u">M3U Playlist</SelectItem>
                <SelectItem value="xtream">Xtream Codes</SelectItem>
                <SelectItem value="manual">Manual (Static)</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>

        {/* URL (not for manual) */}
        {formData.source_type !== 'manual' && (
          <div className="space-y-2">
            <Label htmlFor="create-url">URL</Label>
            <Input
              id="create-url"
              value={formData.url}
              onChange={(e) => setFormData({ ...formData, url: e.target.value })}
              placeholder={
                formData.source_type === 'm3u'
                  ? 'https://example.com/playlist.m3u'
                  : 'http://xtream.example.com:8080'
              }
              required
              disabled={loading}
              autoComplete="off"
            />
          </div>
        )}

        {/* Manual source info */}
        {formData.source_type === 'manual' && (
          <div className="rounded-md border p-3 text-sm bg-muted/40">
            Manual source: define static channels below. Refresh re-applies this set.
          </div>
        )}

        {/* Credentials (not for manual) */}
        {formData.source_type !== 'manual' && (
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
        )}

        {/* User-Agent (not for manual) */}
        {formData.source_type !== 'manual' && (
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="create-user-agent">User-Agent</Label>
              <Select
                value={showCustomUserAgent ? 'custom' : (formData.user_agent || 'default')}
                onValueChange={(value) => {
                  if (value === 'custom') {
                    setShowCustomUserAgent(true);
                    setFormData({ ...formData, user_agent: customUserAgent });
                  } else if (value === 'default') {
                    setShowCustomUserAgent(false);
                    setFormData({ ...formData, user_agent: '' });
                    setCustomUserAgent('');
                  } else {
                    setShowCustomUserAgent(false);
                    setFormData({ ...formData, user_agent: value });
                    setCustomUserAgent('');
                  }
                }}
                disabled={loading}
              >
                <SelectTrigger id="create-user-agent">
                  <SelectValue placeholder="Select User-Agent" />
                </SelectTrigger>
                <SelectContent>
                  {USER_AGENT_PRESETS.map((preset) => (
                    <SelectItem key={preset.value || 'default'} value={preset.value || 'default'}>
                      {preset.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">Some providers block unknown User-Agents</p>
            </div>
            {showCustomUserAgent && (
              <div className="space-y-2">
                <Label htmlFor="create-custom-user-agent">Custom User-Agent</Label>
                <Input
                  id="create-custom-user-agent"
                  value={customUserAgent}
                  onChange={(e) => {
                    setCustomUserAgent(e.target.value);
                    setFormData({ ...formData, user_agent: e.target.value });
                  }}
                  placeholder="Mozilla/5.0 ..."
                  disabled={loading}
                  autoComplete="off"
                />
              </div>
            )}
          </div>
        )}

        {/* Settings */}
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label htmlFor="create-max-streams">Max Concurrent Streams</Label>
            <Input
              id="create-max-streams"
              type="number"
              min="0"
              value={formData.max_concurrent_streams}
              onChange={(e) =>
                setFormData({
                  ...formData,
                  max_concurrent_streams: parseInt(e.target.value) || 0,
                })
              }
              autoComplete="off"
              required
              disabled={loading}
            />
            <p className="text-xs text-muted-foreground">0 = unlimited</p>
          </div>
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label htmlFor="create-cron">Update Schedule (Cron)</Label>
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button variant="ghost" size="sm" className="h-6 px-2 text-xs" type="button">
                      ?
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent className="max-w-sm">
                    <div className="space-y-2">
                      <p className="font-medium">6-field cron format:</p>
                      <p className="text-xs">sec min hour day-of-month month day-of-week</p>
                      <div className="space-y-1 text-xs">
                        <p><code>"0 0 */6 * * *"</code> - Every 6 hours</p>
                        <p><code>"0 0 2 * * *"</code> - Daily at 2:00 AM</p>
                        <p><code>"0 */30 * * * *"</code> - Every 30 minutes</p>
                      </div>
                    </div>
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            </div>
            <Input
              id="create-cron"
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
              className={cronValidation.isValid ? '' : 'border-destructive focus-visible:ring-destructive'}
            />
            {!cronValidation.isValid && cronValidation.error && (
              <div className="text-sm text-destructive space-y-1">
                <p>{cronValidation.error}</p>
                {cronValidation.suggestion && (
                  <p className="text-xs text-muted-foreground">{cronValidation.suggestion}</p>
                )}
              </div>
            )}
            <div className="flex flex-wrap gap-1 text-xs">
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
        </div>

        {/* Manual Channels */}
        {formData.source_type === 'manual' && (
          <div className="pt-2 space-y-2">
            <p className="text-xs text-muted-foreground">
              Enter channel details below. Use full Stream URL (e.g.
              http://example.com/stream.m3u8) and Logo URL (e.g. @logo:token or
              https://logo.example/logo.png).
            </p>
            <ManualChannelEditor
              value={manualChannels}
              onChange={setManualChannels}
              onValidityChange={setManualValid}
              disabled={loading}
            />
          </div>
        )}
      </form>
    </DetailPanel>
  );
}

/**
 * SourceDetailPanel - Detail panel for viewing/editing a stream source
 * Used in MasterDetailLayout to replace sheet-based editing
 */
function SourceDetailPanel({
  source,
  onUpdateSource,
  onDeleteSource,
  onRefreshSource,
  loading,
  error,
  isOnline,
}: {
  source: StreamSourceResponse;
  onUpdateSource: (id: string, source: UpdateStreamSourceRequest) => Promise<void>;
  onDeleteSource: (id: string) => Promise<void>;
  onRefreshSource: (id: string) => Promise<void>;
  loading: { edit: boolean; delete: string | null };
  error: string | null;
  isOnline: boolean;
}) {
  const [formData, setFormData] = useState<UpdateStreamSourceRequest>({
    name: '',
    source_type: 'xtream',
    url: '',
    max_concurrent_streams: 0,
    update_cron: '0 0 */6 * * *',
    username: '',
    password: '',
    user_agent: '',
  });
  const [manualChannels, setManualChannels] = useState<ManualChannelInput[]>([]);
  const [manualValid, setManualValid] = useState(false);
  const [cronValidation, setCronValidation] = useState(validateCronExpression('0 0 */6 * * *'));
  const [isDirty, setIsDirty] = useState(false);
  const [customUserAgent, setCustomUserAgent] = useState('');
  const [showCustomUserAgent, setShowCustomUserAgent] = useState(false);

  // Update form data when source changes
  useEffect(() => {
    const defaultCron = '0 0 */6 * * *';
    const sourceUserAgent = source.user_agent || '';
    const isPreset = USER_AGENT_PRESETS.some(p => p.value === sourceUserAgent);
    const newFormData = {
      name: source.name,
      source_type: source.source_type,
      url: source.url,
      max_concurrent_streams: source.max_concurrent_streams,
      update_cron: source.update_cron || defaultCron,
      username: source.username || '',
      password: '',
      user_agent: sourceUserAgent,
    };
    setFormData(newFormData);
    setCronValidation(validateCronExpression(newFormData.update_cron));
    setIsDirty(false);
    // Handle custom user-agent
    if (sourceUserAgent && !isPreset) {
      setShowCustomUserAgent(true);
      setCustomUserAgent(sourceUserAgent);
    } else {
      setShowCustomUserAgent(false);
      setCustomUserAgent('');
    }
  }, [source]);

  // Load existing manual channels when editing a manual source
  useEffect(() => {
    if (source.source_type === 'manual') {
      (async () => {
        try {
          const response = await apiClient.listManualChannels(source.id);
          setManualChannels(response.items);
          setManualValid(response.items.length > 0);
        } catch {
          // Silent fail
        }
      })();
    }
  }, [source.id, source.source_type]);

  const updateFormData = (updates: Partial<UpdateStreamSourceRequest>) => {
    setFormData((prev) => ({ ...prev, ...updates }));
    setIsDirty(true);
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const payload: UpdateStreamSourceRequest = {
      ...formData,
      ...(formData.source_type === 'manual' && manualValid
        ? { manual_channels: manualChannels }
        : {}),
    };

    // Filter out empty password to preserve existing password
    const updateData = { ...formData };
    if (!updateData.password || updateData.password.trim() === '') {
      delete updateData.password;
    }

    await onUpdateSource(
      source.id,
      updateData.source_type === 'manual' ? { ...payload } : updateData
    );
    setIsDirty(false);
  };

  const handleDelete = async () => {
    if (!confirm('Are you sure you want to delete this source? This action cannot be undone.')) {
      return;
    }
    await onDeleteSource(source.id);
  };

  return (
    <DetailPanel
      title={source.name}
      actions={
        <div className="flex items-center gap-2">
          <RefreshButton
            resourceId={source.id}
            onRefresh={() => onRefreshSource(source.id)}
            disabled={!isOnline}
            size="sm"
          />
          <Button
            variant="outline"
            size="sm"
            onClick={handleDelete}
            disabled={loading.delete === source.id || !isOnline}
            className="text-destructive hover:text-destructive"
          >
            {loading.delete === source.id ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Trash2 className="h-4 w-4" />
            )}
          </Button>
          <Button
            size="sm"
            onClick={handleSubmit}
            disabled={
              loading.edit ||
              !isDirty ||
              !isOnline ||
              (formData.source_type === 'manual' && (!manualValid || manualChannels.length === 0))
            }
          >
            {loading.edit && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            <Save className="h-4 w-4 mr-1" />
            Save
          </Button>
        </div>
      }
    >
      {error && (
        <Alert variant="destructive" className="mb-4">
          <AlertCircle className="h-4 w-4" />
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {/* Source Info Banner */}
      <div className="flex items-center gap-4 mb-6 p-4 bg-muted/50 rounded-lg">
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
          <Monitor className="h-4 w-4 inline mr-1" />
          {source.channel_count} channels
        </div>
        {source.last_ingestion_at && (
          <div className="text-sm text-muted-foreground">
            <Clock className="h-4 w-4 inline mr-1" />
            {formatRelativeTime(source.last_ingestion_at)}
          </div>
        )}
      </div>

      <form onSubmit={handleSubmit} className="space-y-6" autoComplete="off">
        {/* Basic Info */}
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label htmlFor="name">Name</Label>
            <Input
              id="name"
              value={formData.name}
              onChange={(e) => updateFormData({ name: e.target.value })}
              placeholder="Source name"
              required
              disabled={loading.edit}
              autoComplete="off"
            />
          </div>
          <div className="space-y-2">
            <Label>Source Type</Label>
            <div className="flex h-9 items-center px-3 py-2 text-sm border border-input bg-muted rounded-md">
              <Badge variant="outline" className="capitalize">
                {formData.source_type === 'm3u'
                  ? 'M3U Playlist'
                  : formData.source_type === 'xtream'
                    ? 'Xtream Codes'
                    : 'Manual (Static)'}
              </Badge>
            </div>
            <p className="text-xs text-muted-foreground">
              Source type cannot be changed
            </p>
          </div>
        </div>

        {/* URL (not for manual) */}
        {formData.source_type !== 'manual' && (
          <div className="space-y-2">
            <Label htmlFor="url">URL</Label>
            <Input
              id="url"
              value={formData.url}
              onChange={(e) => updateFormData({ url: e.target.value })}
              placeholder={
                formData.source_type === 'm3u'
                  ? 'https://example.com/playlist.m3u'
                  : 'http://xtream.example.com:8080'
              }
              required
              disabled={loading.edit}
              autoComplete="off"
            />
          </div>
        )}

        {/* Manual source info */}
        {formData.source_type === 'manual' && (
          <div className="rounded-md border p-3 text-sm bg-muted/40">
            Manual source: define static channels below. Channel list will be replaced on update.
          </div>
        )}

        {/* Credentials (not for manual) */}
        {formData.source_type !== 'manual' && (
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="username">Username</Label>
              <Input
                id="username"
                value={formData.username || ''}
                onChange={(e) => updateFormData({ username: e.target.value })}
                placeholder="Optional"
                disabled={loading.edit}
                autoComplete="off"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                type="password"
                value={formData.password || ''}
                onChange={(e) => updateFormData({ password: e.target.value })}
                placeholder="Leave blank to keep existing"
                disabled={loading.edit}
                autoComplete="off"
              />
            </div>
          </div>
        )}

        {/* User-Agent (not for manual) */}
        {formData.source_type !== 'manual' && (
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="user-agent">User-Agent</Label>
              <Select
                value={showCustomUserAgent ? 'custom' : (formData.user_agent || 'default')}
                onValueChange={(value) => {
                  if (value === 'custom') {
                    setShowCustomUserAgent(true);
                    updateFormData({ user_agent: customUserAgent });
                  } else if (value === 'default') {
                    setShowCustomUserAgent(false);
                    updateFormData({ user_agent: '' });
                    setCustomUserAgent('');
                  } else {
                    setShowCustomUserAgent(false);
                    updateFormData({ user_agent: value });
                    setCustomUserAgent('');
                  }
                }}
                disabled={loading.edit}
              >
                <SelectTrigger id="user-agent">
                  <SelectValue placeholder="Select User-Agent" />
                </SelectTrigger>
                <SelectContent>
                  {USER_AGENT_PRESETS.map((preset) => (
                    <SelectItem key={preset.value || 'default'} value={preset.value || 'default'}>
                      {preset.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">Some providers block unknown User-Agents</p>
            </div>
            {showCustomUserAgent && (
              <div className="space-y-2">
                <Label htmlFor="custom-user-agent">Custom User-Agent</Label>
                <Input
                  id="custom-user-agent"
                  value={customUserAgent}
                  onChange={(e) => {
                    setCustomUserAgent(e.target.value);
                    updateFormData({ user_agent: e.target.value });
                  }}
                  placeholder="Mozilla/5.0 ..."
                  disabled={loading.edit}
                  autoComplete="off"
                />
              </div>
            )}
          </div>
        )}

        {/* Settings */}
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label htmlFor="max_concurrent_streams">Max Concurrent Streams</Label>
            <Input
              id="max_concurrent_streams"
              type="number"
              min="0"
              value={formData.max_concurrent_streams}
              onChange={(e) =>
                updateFormData({ max_concurrent_streams: parseInt(e.target.value) || 0 })
              }
              required
              disabled={loading.edit}
              autoComplete="off"
            />
            <p className="text-xs text-muted-foreground">0 = unlimited</p>
          </div>
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label htmlFor="update_cron">Update Schedule (Cron)</Label>
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button variant="ghost" size="sm" className="h-6 px-2 text-xs" type="button">
                      ?
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent className="max-w-sm">
                    <div className="space-y-2">
                      <p className="font-medium">6-field cron format:</p>
                      <p className="text-xs">sec min hour day-of-month month day-of-week</p>
                      <div className="space-y-1 text-xs">
                        <p><code>"0 0 */6 * * *"</code> - Every 6 hours</p>
                        <p><code>"0 0 2 * * *"</code> - Daily at 2:00 AM</p>
                        <p><code>"0 */30 * * * *"</code> - Every 30 minutes</p>
                      </div>
                    </div>
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            </div>
            <Input
              id="update_cron"
              value={formData.update_cron}
              onChange={(e) => {
                const newValue = e.target.value;
                updateFormData({ update_cron: newValue });
                setCronValidation(validateCronExpression(newValue));
              }}
              placeholder="0 0 */6 * * *"
              required
              disabled={loading.edit}
              autoComplete="off"
              className={cronValidation.isValid ? '' : 'border-destructive focus-visible:ring-destructive'}
            />
            {!cronValidation.isValid && cronValidation.error && (
              <div className="text-sm text-destructive space-y-1">
                <p>{cronValidation.error}</p>
                {cronValidation.suggestion && (
                  <p className="text-xs text-muted-foreground">{cronValidation.suggestion}</p>
                )}
              </div>
            )}
            <div className="flex flex-wrap gap-1 text-xs">
              {COMMON_CRON_TEMPLATES.slice(0, 3).map((template) => (
                <Button
                  key={template.expression}
                  variant="ghost"
                  size="sm"
                  className="h-6 px-2 text-xs"
                  onClick={() => {
                    updateFormData({ update_cron: template.expression });
                    setCronValidation(validateCronExpression(template.expression));
                  }}
                  disabled={loading.edit}
                  type="button"
                >
                  {template.description}
                </Button>
              ))}
            </div>
          </div>
        </div>

        {/* Manual Channels */}
        {formData.source_type === 'manual' && (
          <div className="space-y-4">
            <div className="flex flex-wrap gap-2 items-center">
              <ManualM3UImportExport
                sourceId={source.id}
                existingChannels={manualChannels}
                disabled={loading.edit}
                onPreviewParsed={(chs) => {
                  setManualChannels(chs);
                  setManualValid(chs.length > 0);
                  setIsDirty(true);
                }}
                onApplied={async () => {
                  try {
                    const response = await apiClient.listManualChannels(source.id);
                    setManualChannels(response.items);
                    setManualValid(response.items.length > 0);
                  } catch {
                    /* ignore */
                  }
                }}
              />
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={loading.edit}
                onClick={async () => {
                  try {
                    const response = await apiClient.listManualChannels(source.id);
                    setManualChannels(response.items);
                    setManualValid(response.items.length > 0);
                  } catch {
                    /* ignore */
                  }
                }}
              >
                Reload Channels
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              Provide Stream URL (e.g. http://example.com/live.m3u8) and Logo (e.g. @logo:token or https://logo.example/img.png).
            </p>
            <ManualChannelEditor
              value={manualChannels}
              onChange={(chs) => {
                setManualChannels(chs);
                setIsDirty(true);
              }}
              onValidityChange={setManualValid}
              disabled={loading.edit}
            />
          </div>
        )}
      </form>
    </DetailPanel>
  );
}

export function StreamSources() {
  const progressContext = useProgressContext();
  const [allSources, setAllSources] = useState<StreamSourceResponse[]>([]);
  const [pagination, setPagination] = useState<Omit<
    PaginatedResponse<StreamSourceResponse>,
    'items'
  > | null>(null);

  const [loading, setLoading] = useState<LoadingState>({
    sources: false,
    create: false,
    edit: false,
    delete: null,
  });

  // Integrate with page loading spinner

  const [errors, setErrors] = useState<ErrorState>({
    sources: null,
    create: null,
    edit: null,
    action: null,
  });

  const [selectedSourceId, setSelectedSourceId] = useState<string | null>(null);
  const [refreshingSources, setRefreshingSources] = useState<Set<string>>(new Set());
  const { handleApiError, dismissConflict, getConflictState } = useConflictHandler();

  const [isOnline, setIsOnline] = useState(true);
  const [isCreating, setIsCreating] = useState(false);

  // Sort sources alphabetically by name
  const sortedSources = useMemo(() => {
    return [...allSources].sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true }));
  }, [allSources]);

  // Health check is handled by parent component, no need for redundant calls

  const loadSources = useCallback(async () => {
    if (!isOnline) return;

    setLoading((prev) => ({ ...prev, sources: true }));
    setErrors((prev) => ({ ...prev, sources: null }));

    try {
      // Load all sources without search parameters - filtering happens locally
      const response = await apiClient.getStreamSources();

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
          sources: `Failed to load sources: ${apiError.message}`,
        }));
      }
    } finally {
      setLoading((prev) => ({ ...prev, sources: false }));
    }
  }, []); // Remove isOnline dependency

  // Load sources on mount only
  useEffect(() => {
    loadSources();
  }, []); // Remove loadSources dependency - only run on mount

  // Initialize SSE connection on mount for stream ingestion events
  useEffect(() => {
    // Listen for any stream ingestion events to update refresh states and reload data
    const handleGlobalStreamEvent = (event: any) => {
      console.log('[StreamSources] Received global stream ingestion event:', event);

      // If we see an operation starting (idle or processing state), add it to refreshing set
      if (
        (event.state === 'idle' || event.state === 'processing') &&
        event.id &&
        event.operation_type === 'stream_ingestion'
      ) {
        console.log(`[StreamSources] Adding ${event.id} to refreshing set (state: ${event.state})`);
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
        event.operation_type === 'stream_ingestion'
      ) {
        console.log(
          `[StreamSources] Removing ${event.id} from refreshing set (state: ${event.state})`
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

    const unsubscribe = progressContext.subscribeToType(
      'stream_ingestion',
      handleGlobalStreamEvent
    );

    return () => {
      console.log('[StreamSources] Component unmounting, unsubscribing from stream events');
      unsubscribe();
    };
  }, []);

  const handleCreateSource = async (newSource: CreateStreamSourceRequest) => {
    setLoading((prev) => ({ ...prev, create: true }));
    setErrors((prev) => ({ ...prev, create: null }));

    try {
      const createdSource = await apiClient.createStreamSource(newSource);
      // Optimistic update: add new source to existing list instead of full reload
      setAllSources((prev) => [...prev, createdSource]);
      setPagination((prev) =>
        prev
          ? {
              ...prev,
              total: prev.total + 1,
            }
          : null
      );
      // Exit create mode and select the new source
      setIsCreating(false);
      if (createdSource?.id) {
        setSelectedSourceId(createdSource.id);
      }
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        create: `Failed to create source: ${apiError.message}`,
      }));
      throw error; // Re-throw to prevent dialog from closing
    } finally {
      setLoading((prev) => ({ ...prev, create: false }));
    }
  };

  const handleUpdateSource = async (id: string, updatedSource: UpdateStreamSourceRequest) => {
    setLoading((prev) => ({ ...prev, edit: true }));
    setErrors((prev) => ({ ...prev, edit: null }));

    try {
      const updated = await apiClient.updateStreamSource(id, updatedSource);
      // Optimistic update: update existing source in list instead of full reload
      setAllSources((prev) => prev.map((source) => (source.id === id ? updated : source)));
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        edit: `Failed to update source: ${apiError.message}`,
      }));
      throw error; // Re-throw to prevent dialog from closing
    } finally {
      setLoading((prev) => ({ ...prev, edit: false }));
    }
  };

  const handleRefreshSource = async (sourceId: string) => {
    console.log(`[StreamSources] Starting refresh for source: ${sourceId}`);
    setRefreshingSources((prev) => new Set(prev).add(sourceId));
    setErrors((prev) => ({ ...prev, action: null }));

    try {
      console.log(`[StreamSources] Calling API refresh for source: ${sourceId}`);
      await apiClient.refreshStreamSource(sourceId);
      console.log(`[StreamSources] API refresh call completed for source: ${sourceId}`);

      // Fallback timeout in case SSE events don't work (just clear state, no reload)
      setTimeout(() => {
        console.log(
          `[StreamSources] Fallback timeout - clearing refresh state for source: ${sourceId}`
        );
        setRefreshingSources((prev) => {
          const newSet = new Set(prev);
          newSet.delete(sourceId);
          return newSet;
        });
      }, 30000); // 30 second timeout
    } catch (error) {
      const apiError = error as ApiError;
      console.error(`[StreamSources] Refresh failed for source ${sourceId}:`, apiError);

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
    setLoading((prev) => ({ ...prev, delete: sourceId }));
    setErrors((prev) => ({ ...prev, action: null }));

    try {
      await apiClient.deleteStreamSource(sourceId);
      // Optimistic update: remove source from list instead of full reload
      setAllSources((prev) => prev.filter((source) => source.id !== sourceId));
      setPagination((prev) =>
        prev
          ? {
              ...prev,
              total: prev.total - 1,
            }
          : null
      );
      // Clear selection if the deleted source was selected
      if (selectedSourceId === sourceId) {
        setSelectedSourceId(null);
      }
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        action: `Failed to delete source: ${apiError.message}`,
      }));
    } finally {
      setLoading((prev) => ({ ...prev, delete: null }));
    }
  };

  const totalChannels = allSources?.reduce((sum, source) => sum + source.channel_count, 0) || 0;
  const successfulSources = allSources?.filter((s) => s.status === 'success').length || 0;
  const m3uSources = allSources?.filter((s) => s.source_type === 'm3u').length || 0;
  const xtreamSources = allSources?.filter((s) => s.source_type === 'xtream').length || 0;

  // Convert sources to MasterItem format
  const masterItems = useMemo(
    () => sortedSources.map(sourceToMasterItem),
    [sortedSources]
  );

  // Find the selected source
  const selectedSource = useMemo(
    () => allSources.find((s) => s.id === selectedSourceId) ?? null,
    [allSources, selectedSourceId]
  );

  // Custom filter for MasterDetailLayout - fuzzy search across all fields
  const filterSource = useMemo(
    () => createFuzzyFilter<SourceMasterItem>({
      keys: [
        { name: 'name', weight: 0.4 },
        { name: 'url', weight: 0.2 },
        { name: 'source_type', weight: 0.15 },
        { name: 'status', weight: 0.1 },
        { name: 'channel_count', weight: 0.1 },
        { name: 'enabled', weight: 0.05 },
      ],
      accessor: (item) => ({
        name: item.source.name,
        url: item.source.url || '',
        source_type: item.source.source_type,
        status: item.source.status,
        channel_count: `${item.source.channel_count} channels`,
        enabled: item.source.enabled ? 'enabled' : 'disabled',
      }),
    }),
    []
  );

  // Calculate stats
  const totalSources = pagination?.total || 0;
  const enabledSources = allSources.filter((s) => s.enabled).length;
  const successSources = allSources.filter((s) => s.status === 'success').length;

  return (
    <TooltipProvider>
      <div className="flex flex-col gap-6 h-full">
        {/* Header */}
        <div className="flex items-center justify-between">
          <div>
            <p className="text-muted-foreground">
              Manage stream sources, such as M3U and Xtream Code providers
            </p>
          </div>
          <div className="flex items-center gap-2">
            {!isOnline && <WifiOff className="h-5 w-5 text-destructive" />}
          </div>
        </div>

        {/* Stats */}
        <div className="grid gap-2 md:grid-cols-3">
          <StatCard
            title="Total Sources"
            value={totalSources}
            icon={<Database className="h-4 w-4" />}
          />
          <StatCard
            title="Enabled"
            value={enabledSources}
            icon={<CheckCircle className="h-4 w-4 text-green-600" />}
          />
          <StatCard
            title="Channels"
            value={totalChannels}
            icon={<Activity className="h-4 w-4 text-blue-600" />}
          />
        </div>

        {/* Alerts */}
        {(!isOnline || errors.action || errors.sources) && (
          <div className="space-y-2">
            {!isOnline && (
              <Alert variant="destructive">
                <WifiOff className="h-4 w-4" />
                <AlertTitle>API Service Offline</AlertTitle>
                <AlertDescription>
                  Unable to connect to {API_CONFIG.baseUrl}.
                  <Button variant="outline" size="sm" className="ml-2" onClick={() => window.location.reload()}>
                    Retry
                  </Button>
                </AlertDescription>
              </Alert>
            )}
            {errors.action && (
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertTitle>Error</AlertTitle>
                <AlertDescription>
                  {errors.action}
                  <Button variant="outline" size="sm" className="ml-2" onClick={() => setErrors((prev) => ({ ...prev, action: null }))}>
                    Dismiss
                  </Button>
                </AlertDescription>
              </Alert>
            )}
          </div>
        )}

        {/* MasterDetailLayout */}
        <Card className="flex-1 overflow-hidden min-h-0">
          <CardContent className="p-0 h-full">
            {errors.sources ? (
              <div className="p-6">
                <Alert variant="destructive">
                  <AlertCircle className="h-4 w-4" />
                  <AlertTitle>Failed to Load Sources</AlertTitle>
                  <AlertDescription>
                    {errors.sources}
                    <ConflictNotification
                      show={getConflictState('stream-sources-retry').show}
                      message={getConflictState('stream-sources-retry').message}
                      onDismiss={() => dismissConflict('stream-sources-retry')}
                    >
                      <Button
                        variant="outline"
                        size="sm"
                        className="ml-2"
                        onClick={async () => {
                          try {
                            await loadSources();
                          } catch (error) {
                            handleApiError(error, 'stream-sources-retry', 'Load sources');
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
              </div>
            ) : (
              <MasterDetailLayout
                items={masterItems}
                selectedId={isCreating ? null : selectedSourceId}
                onSelect={(item) => {
                  setIsCreating(false);
                  setSelectedSourceId(item?.id ?? null);
                }}
                isLoading={loading.sources}
                title={`Stream Sources (${sortedSources.length})`}
                searchPlaceholder="Search by name, type, status..."
                storageKey="stream-sources"
                headerAction={
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => {
                      setIsCreating(true);
                      setSelectedSourceId(null);
                      setErrors((prev) => ({ ...prev, create: null }));
                    }}
                    disabled={loading.sources}
                  >
                    <Plus className="h-4 w-4" />
                    <span className="sr-only">Add Source</span>
                  </Button>
                }
                emptyState={{
                  title: 'No stream sources configured',
                  description: 'Get started by creating your first stream source.',
                }}
                filterFn={filterSource}
              >
                {(selectedItem) =>
                  isCreating ? (
                    <SourceCreatePanel
                      onCreate={handleCreateSource}
                      onCancel={() => setIsCreating(false)}
                      loading={loading.create}
                      error={errors.create}
                    />
                  ) : selectedItem && selectedSource ? (
                    <SourceDetailPanel
                      source={selectedSource}
                      onUpdateSource={handleUpdateSource}
                      onDeleteSource={handleDeleteSource}
                      onRefreshSource={handleRefreshSource}
                      loading={{ edit: loading.edit, delete: loading.delete }}
                      error={errors.edit}
                      isOnline={isOnline}
                    />
                  ) : (
                    <DetailEmpty
                      title="Select a source"
                      description="Choose a stream source from the list to view and edit its configuration"
                      icon={<Database className="h-12 w-12 text-muted-foreground" />}
                    />
                  )
                }
              </MasterDetailLayout>
            )}
          </CardContent>
        </Card>
      </div>
    </TooltipProvider>
  );
}
