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
  Database,
  Edit,
  Trash2,
  RefreshCw,
  Clock,
  Monitor,
  Activity,
  Search,
  Filter,
  AlertCircle,
  CheckCircle,
  Loader2,
  WifiOff,
  Grid,
  List,
  Table as TableIcon,
} from 'lucide-react';
import { RefreshButton } from '@/components/RefreshButton';
import { OperationStatusIndicator } from '@/components/OperationStatusIndicator';
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
import { DEFAULT_PAGE_SIZE, API_CONFIG } from '@/lib/config';
import { formatDate, formatRelativeTime } from '@/lib/utils';
import {
  validateCronExpression,
  describeCronExpression,
  COMMON_CRON_TEMPLATES,
} from '@/lib/cron-validation';
import { ManualChannelEditor, ManualChannelInput } from '@/components/manual-channel-editor';
import { ManualM3UImportExport } from '@/components/manual-m3u-import-export';

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

function getStatusColor(isActive: boolean): string {
  return isActive ? 'bg-green-100 text-green-800' : 'bg-red-100 text-red-800';
}

function CreateSourceSheet({
  onCreateSource,
  loading,
  error,
}: {
  onCreateSource: (source: CreateStreamSourceRequest) => Promise<void>;
  loading: boolean;
  error: string | null;
}) {
  const [open, setOpen] = useState(false);
  const [formData, setFormData] = useState<CreateStreamSourceRequest>({
    name: '',
    source_type: 'xtream',
    url: '',
    max_concurrent_streams: 0,
    update_cron: '0 0 */6 * * * *',
    username: '',
    password: '',
  });
  // Manual channels state (only used when source_type === 'manual')
  const [manualChannels, setManualChannels] = useState<ManualChannelInput[]>([]);
  const [manualValid, setManualValid] = useState(false);
  const [cronValidation, setCronValidation] = useState(validateCronExpression('0 0 */6 * * * *'));

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const payload: CreateStreamSourceRequest = {
      ...formData,
      ...(formData.source_type === 'manual' ? { manual_channels: manualChannels } : {}),
    };
    await onCreateSource(payload);
    if (!error) {
      setOpen(false);
      setFormData({
        name: '',
        source_type: 'xtream',
        url: '',
        max_concurrent_streams: 0,
        update_cron: '0 0 */6 * * * *',
        username: '',
        password: '',
      });
      setManualChannels([]);
      setManualValid(false);
      setCronValidation(validateCronExpression('0 0 */6 * * * *'));
    }
  };

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <Button className="gap-2">
          <Plus className="h-4 w-4" />
          Add Source
        </Button>
      </SheetTrigger>
      <SheetContent side="right" className="w-full sm:max-w-2xl overflow-y-auto">
        <SheetHeader>
          <SheetTitle>Add Stream Source</SheetTitle>
          <SheetDescription>
            Create a new stream source from M3U playlist or Xtream Codes API
          </SheetDescription>
        </SheetHeader>

        {error && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <form
          id="create-source-form"
          onSubmit={handleSubmit}
          className="space-y-4 px-4"
          autoComplete="off"
        >
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="name">Name</Label>
              <Input
                id="name"
                value={formData.name}
                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                placeholder="Premium Sports Channel"
                required
                disabled={loading}
                autoComplete="off"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="source_type">Source Type</Label>
              <Select
                value={formData.source_type}
                onValueChange={(value) =>
                  setFormData({ ...formData, source_type: value as StreamSourceType })
                }
                disabled={loading}
              >
                <SelectTrigger>
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

          {(formData.source_type as string) !== 'manual' && (
            <div className="space-y-2">
              <Label htmlFor="url">URL</Label>
              <Input
                id="url"
                value={formData.url}
                onChange={(e) => setFormData({ ...formData, url: e.target.value })}
                placeholder={
                  formData.source_type === 'm3u'
                    ? 'https://example.com/playlist.m3u'
                    : 'http://xtream.example.com:8080'
                }
                required={formData.source_type !== 'manual'}
                disabled={loading}
                autoComplete="off"
              />
            </div>
          )}
          {formData.source_type === 'manual' && (
            <div className="rounded-md border p-3 text-sm bg-muted/40">
              Manual source: define static channels below. Refresh re-applies this set.
            </div>
          )}

          {(formData.source_type as string) !== 'manual' && (
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="username">Username</Label>
                <Input
                  id="username"
                  value={formData.username || ''}
                  onChange={(e) => setFormData({ ...formData, username: e.target.value })}
                  placeholder="Optional"
                  disabled={loading}
                  autoComplete="off"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="password">Password</Label>
                <Input
                  id="password"
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

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="max_concurrent_streams">Max Concurrent Streams</Label>
              <Input
                id="max_concurrent_streams"
                type="number"
                min="0"
                value={formData.max_concurrent_streams}
                onChange={(e) =>
                  setFormData({ ...formData, max_concurrent_streams: parseInt(e.target.value) || 0 })
                }
                autoComplete="off"
                required
                disabled={loading}
              />
              <p className="text-xs text-muted-foreground">0 = unlimited</p>
            </div>
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label htmlFor="update_cron">Update Schedule (Cron)</Label>
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button variant="ghost" size="sm" className="h-6 px-2 text-xs">
                        ?
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent className="max-w-sm">
                      <div className="space-y-2">
                        <p className="font-medium">7-field cron format:</p>
                        <p className="text-xs">sec min hour day-of-month month day-of-week year</p>
                        <div className="space-y-1 text-xs">
                          <p>
                            <code>"0 0 */6 * * * *"</code> - Every 6 hours
                          </p>
                          <p>
                            <code>"0 0 2 * * * *"</code> - Daily at 2:00 AM
                          </p>
                          <p>
                            <code>"0 */30 * * * * *"</code> - Every 30 minutes
                          </p>
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
                  setFormData({ ...formData, update_cron: newValue });
                  setCronValidation(validateCronExpression(newValue));
                }}
                placeholder="0 0 */6 * * * *"
                required
                disabled={loading}
                autoComplete="off"
                className={
                  cronValidation.isValid ? '' : 'border-destructive focus:border-destructive'
                }
              />
              {!cronValidation.isValid && cronValidation.error && (
                <div className="text-sm text-destructive space-y-1">
                  <p>{cronValidation.error}</p>
                  {cronValidation.suggestion && (
                    <p className="text-xs text-muted-foreground">💡 {cronValidation.suggestion}</p>
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

        <SheetFooter className="gap-2">
          <Button type="button" variant="outline" onClick={() => setOpen(false)} disabled={loading}>
            Cancel
          </Button>
          <Button
            form="create-source-form"
            type="submit"
            disabled={
              loading ||
              (formData.source_type === 'manual' && (!manualValid || manualChannels.length === 0))
            }
          >
            {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Create Source
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}

function EditSourceSheet({
  source,
  onUpdateSource,
  loading,
  error,
  open,
  onOpenChange,
}: {
  source: StreamSourceResponse | null;
  onUpdateSource: (id: string, source: UpdateStreamSourceRequest) => Promise<void>;
  loading: boolean;
  error: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [formData, setFormData] = useState<UpdateStreamSourceRequest>({
    name: '',
    source_type: 'xtream',
    url: '',
    max_concurrent_streams: 0,
    update_cron: '0 0 */6 * * * *',
    username: '',
    password: '',
  });
  const [manualChannels, setManualChannels] = useState<ManualChannelInput[]>([]);
  const [manualValid, setManualValid] = useState(false);
  const [cronValidation, setCronValidation] = useState(validateCronExpression('0 0 */6 * * * *'));

  // Update form data when source changes
  useEffect(() => {
    if (source) {
      const defaultCron = '0 0 */6 * * * *'; // Every 6 hours
      const newFormData = {
        name: source.name,
        source_type: source.source_type,
        url: source.url,
        max_concurrent_streams: source.max_concurrent_streams,
        update_cron: source.update_cron || defaultCron,
        username: source.username || '',
        password: source.password || '',
      };
      setFormData(newFormData);
      setCronValidation(validateCronExpression(newFormData.update_cron));
    }
  }, [source]);

  // Load existing manual channels when editing a manual source
  useEffect(() => {
    if (source && source.source_type === 'manual') {
      (async () => {
        try {
          const chs = await apiClient.listManualChannels(source.id, true);
          setManualChannels(chs);
          setManualValid(chs.length > 0);
        } catch {
          // Silent fail; could add toast / alert if desired
        }
      })();
    }
  }, [source]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!source) return;
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

    // For manual sources include manual channel payload (already added in payload above)
    await onUpdateSource(
      source.id,
      updateData.source_type === 'manual' ? { ...payload } : updateData
    );
    if (!error) {
      onOpenChange(false);
    }
  };

  if (!source) return null;

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" className="w-full sm:max-w-2xl overflow-y-auto">
        <SheetHeader>
          <SheetTitle>Edit Stream Source</SheetTitle>
          <SheetDescription>Update the stream source configuration</SheetDescription>
        </SheetHeader>

        {error && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <form
          id="edit-source-form"
          onSubmit={handleSubmit}
          className="space-y-4 px-4"
          autoComplete="off"
        >
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="edit-name">Name</Label>
              <Input
                id="edit-name"
                value={formData.name}
                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                placeholder="Premium Sports Channel"
                required
                disabled={loading}
                autoComplete="off"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="edit-source_type">Source Type</Label>
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
                Source type cannot be changed after creation
              </p>
            </div>
          </div>

          {(formData.source_type as string) !== 'manual' && (
            <div className="space-y-2">
              <Label htmlFor="edit-url">URL</Label>
              <Input
                id="edit-url"
                value={formData.url}
                onChange={(e) => setFormData({ ...formData, url: e.target.value })}
                placeholder={
                  formData.source_type === 'm3u'
                    ? 'https://example.com/playlist.m3u'
                    : 'http://xtream.example.com:8080'
                }
                required={formData.source_type !== 'manual'}
                disabled={loading}
                autoComplete="off"
              />
            </div>
          )}
          {formData.source_type === 'manual' && (
            <div className="rounded-md border p-3 text-sm bg-muted/40">
              Manual source: define static channels below. Existing channel list will be replaced on
              update.
            </div>
          )}

          {(formData.source_type as string) !== 'manual' && (
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="edit-username">Username</Label>
                <Input
                  id="edit-username"
                  value={formData.username || ''}
                  onChange={(e) => setFormData({ ...formData, username: e.target.value })}
                  placeholder="Optional"
                  disabled={loading}
                  autoComplete="off"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="edit-password">Password</Label>
                <Input
                  id="edit-password"
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

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="edit-max_concurrent_streams">Max Concurrent Streams</Label>
              <Input
                id="edit-max_concurrent_streams"
                type="number"
                min="0"
                value={formData.max_concurrent_streams}
                onChange={(e) =>
                  setFormData({ ...formData, max_concurrent_streams: parseInt(e.target.value) || 0 })
                }
                required
                disabled={loading}
                autoComplete="off"
              />
              <p className="text-xs text-muted-foreground">0 = unlimited</p>
            </div>
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label htmlFor="edit-update_cron">Update Schedule (Cron)</Label>
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button variant="ghost" size="sm" className="h-6 px-2 text-xs">
                        ?
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent className="max-w-sm">
                      <div className="space-y-2">
                        <p className="font-medium">7-field cron format:</p>
                        <p className="text-xs">sec min hour day-of-month month day-of-week year</p>
                        <div className="space-y-1 text-xs">
                          <p>
                            <code>"0 0 */6 * * * *"</code> - Every 6 hours
                          </p>
                          <p>
                            <code>"0 0 2 * * * *"</code> - Daily at 2:00 AM
                          </p>
                          <p>
                            <code>"0 */30 * * * * *"</code> - Every 30 minutes
                          </p>
                        </div>
                      </div>
                    </TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              </div>
              <Input
                id="edit-update_cron"
                value={formData.update_cron}
                onChange={(e) => {
                  const newValue = e.target.value;
                  setFormData({ ...formData, update_cron: newValue });
                  setCronValidation(validateCronExpression(newValue));
                }}
                placeholder="0 0 */6 * * * *"
                required
                disabled={loading}
                autoComplete="off"
                className={
                  cronValidation.isValid ? '' : 'border-destructive focus:border-destructive'
                }
              />
              {!cronValidation.isValid && cronValidation.error && (
                <div className="text-sm text-destructive space-y-1">
                  <p>{cronValidation.error}</p>
                  {cronValidation.suggestion && (
                    <p className="text-xs text-muted-foreground">💡 {cronValidation.suggestion}</p>
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
          {formData.source_type === 'manual' && (
            <div className="pt-2 space-y-4">
              <div className="flex flex-wrap gap-2 items-center">
                <ManualM3UImportExport
                  sourceId={source.id}
                  existingChannels={manualChannels}
                  disabled={loading}
                  onPreviewParsed={(chs) => {
                    setManualChannels(chs);
                    setManualValid(chs.length > 0);
                  }}
                  onApplied={async () => {
                    try {
                      const chs = await apiClient.listManualChannels(source.id, true);
                      setManualChannels(chs);
                      setManualValid(chs.length > 0);
                    } catch {
                      /* ignore */
                    }
                  }}
                />
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={loading}
                  onClick={async () => {
                    try {
                      const chs = await apiClient.listManualChannels(source.id, true);
                      setManualChannels(chs);
                      setManualValid(chs.length > 0);
                    } catch {
                      /* ignore */
                    }
                  }}
                >
                  Reload Channels
                </Button>
              </div>
              <p className="text-xs text-muted-foreground">
                Provide Stream URL (e.g. http://example.com/live.m3u8) and Logo (e.g. @logo:token or
                https://logo.example/img.png).
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

        <SheetFooter className="gap-2">
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={loading}
          >
            Cancel
          </Button>
          <Button
            form="edit-source-form"
            type="submit"
            disabled={
              loading ||
              (formData.source_type === 'manual' && (!manualValid || manualChannels.length === 0))
            }
          >
            {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Update Source
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}

export function StreamSources() {
  const progressContext = useProgressContext();
  const [allSources, setAllSources] = useState<StreamSourceResponse[]>([]);
  const [pagination, setPagination] = useState<Omit<
    PaginatedResponse<StreamSourceResponse>,
    'items'
  > | null>(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [filterType, setFilterType] = useState<StreamSourceType | 'all'>('all');
  const [filterStatus, setFilterStatus] = useState<'all' | 'active' | 'inactive'>('all');
  const [currentPage, setCurrentPage] = useState(1);

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

  const [editingSource, setEditingSource] = useState<StreamSourceResponse | null>(null);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [refreshingSources, setRefreshingSources] = useState<Set<string>>(new Set());
  const [viewMode, setViewMode] = useState<'grid' | 'list' | 'table'>('table');
  const { handleApiError, dismissConflict, getConflictState } = useConflictHandler();

  const [isOnline, setIsOnline] = useState(true);

  // Compute filtered results locally
  const filteredSources = useMemo(() => {
    let filtered = allSources;

    // Filter by source type
    if (filterType !== 'all') {
      filtered = filtered.filter((source) => source.source_type === filterType);
    }

    // Filter by status
    if (filterStatus !== 'all') {
      filtered = filtered.filter((source) =>
        filterStatus === 'active' ? source.is_active : !source.is_active
      );
    }

    // Filter by search term
    if (searchTerm.trim()) {
      const searchLower = searchTerm.toLowerCase();
      filtered = filtered.filter((source) => {
        const searchableText = [
          source.name.toLowerCase(),
          (source.url || '').toLowerCase(),
          source.source_type.toLowerCase(),
          source.username || '',
          source.password || '',
          source.update_cron.toLowerCase(),
          source.max_concurrent_streams.toString(),
          // Status labels
          source.is_active ? 'active enabled' : 'inactive disabled',
          // Relative time and formatted dates
          formatRelativeTime(source.created_at).toLowerCase(),
          formatRelativeTime(source.updated_at).toLowerCase(),
          formatDate(source.created_at).toLowerCase(),
          formatDate(source.updated_at).toLowerCase(),
          // Type labels
          source.source_type === 'm3u' ? 'm3u playlist' : 'xtream codes api',
          // Additional searchable terms
          'stream source',
          source.created_at.includes('T') ? 'created' : '',
          source.updated_at.includes('T') ? 'updated' : '',
        ];

        return searchableText.some((text) => text.toLowerCase().includes(searchLower));
      });
    }

    return filtered;
  }, [allSources, searchTerm, filterType, filterStatus]);

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

  const handleEditSource = (source: StreamSourceResponse) => {
    setEditingSource(source);
    setEditDialogOpen(true);
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
    if (!confirm('Are you sure you want to delete this source? This action cannot be undone.')) {
      return;
    }

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
  const activeSources = allSources?.filter((s) => s.is_active).length || 0;
  const m3uSources = allSources?.filter((s) => s.source_type === 'm3u').length || 0;
  const xtreamSources = allSources?.filter((s) => s.source_type === 'xtream').length || 0;

  return (
    <TooltipProvider>
      <div className="space-y-6">
        {/* Header Section */}
        <div className="flex items-center justify-between">
          <div>
            <p className="text-muted-foreground">
              Manage stream sources, such as M3U and Xtream Code providers
            </p>
          </div>
          <div className="flex items-center gap-2">
            {!isOnline && <WifiOff className="h-5 w-5 text-destructive" />}
            <CreateSourceSheet
              onCreateSource={handleCreateSource}
              loading={loading.create}
              error={errors.create}
            />
          </div>
        </div>

        {/* Edit Sheet */}
        <EditSourceSheet
          source={editingSource}
          onUpdateSource={handleUpdateSource}
          loading={loading.edit}
          error={errors.edit}
          open={editDialogOpen}
          onOpenChange={setEditDialogOpen}
        />

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
        <div className="grid gap-4 md:grid-cols-4">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">Total Sources</CardTitle>
              <Database className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{pagination?.total || 0}</div>
              <p className="text-xs text-muted-foreground">{activeSources} active</p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">Total Channels</CardTitle>
              <Activity className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{totalChannels}</div>
              <p className="text-xs text-muted-foreground">Across all sources</p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">M3U Sources</CardTitle>
              <Database className="h-4 w-4 text-blue-600" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{m3uSources}</div>
              <p className="text-xs text-muted-foreground">M3U playlists</p>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">Xtream Sources</CardTitle>
              <Database className="h-4 w-4 text-green-600" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{xtreamSources}</div>
              <p className="text-xs text-muted-foreground">Xtream Codes APIs</p>
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
                    placeholder="Search sources, types, URLs, credentials..."
                    value={searchTerm}
                    onChange={(e) => setSearchTerm(e.target.value)}
                    className="pl-8"
                    disabled={loading.sources}
                    autoComplete="off"
                  />
                </div>
              </div>
              <Select
                value={filterType}
                onValueChange={(value) => setFilterType(value as StreamSourceType | 'all')}
              >
                <SelectTrigger className="w-full sm:w-[180px]">
                  <SelectValue placeholder="Filter by type" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Types</SelectItem>
                  <SelectItem value="m3u">M3U Playlist</SelectItem>
                  <SelectItem value="xtream">Xtream Codes</SelectItem>
                </SelectContent>
              </Select>
              <Select
                value={filterStatus}
                onValueChange={(value) => setFilterStatus(value as 'all' | 'active' | 'inactive')}
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

        {/* Sources Table */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center justify-between">
              <span>
                Stream Sources ({filteredSources?.length || 0}
                {searchTerm || filterType !== 'all' || filterStatus !== 'all'
                  ? ` of ${allSources?.length || 0}`
                  : ''}
                )
              </span>
              {loading.sources && <Loader2 className="h-4 w-4 animate-spin" />}
            </CardTitle>
            <CardDescription>Configure and manage your stream sources</CardDescription>
          </CardHeader>
          <CardContent>
            {errors.sources ? (
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
            ) : (
              <>
                {viewMode === 'table' ? (
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Name</TableHead>
                        <TableHead>Type</TableHead>
                        <TableHead>Status</TableHead>
                        <TableHead>Channels</TableHead>
                        <TableHead>Last Updated</TableHead>
                        <TableHead>Next Update</TableHead>
                        <TableHead>Actions</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {filteredSources?.map((source) => (
                        <TableRow key={source.id}>
                          <TableCell>
                            <div className="space-y-2">
                              <div>
                                <div className="font-medium">{source.name}</div>
                                <div className="text-sm text-muted-foreground truncate max-w-xs sm:max-w-sm md:max-w-md lg:max-w-lg">
                                  {source.url}
                                </div>
                              </div>
                            </div>
                          </TableCell>
                          <TableCell>
                            <Badge className={getSourceTypeColor(source.source_type)}>
                              {source.source_type.toUpperCase()}
                            </Badge>
                          </TableCell>
                          <TableCell>
                            <div className="flex items-center gap-2">
                              <Badge className={getStatusColor(source.is_active)}>
                                {source.is_active ? 'Active' : 'Inactive'}
                              </Badge>
                              {/* T049: Operation error/warning indicator */}
                              <OperationStatusIndicator resourceId={source.id} />
                            </div>
                          </TableCell>
                          <TableCell>
                            <div className="flex items-center gap-1">
                              <Monitor className="h-4 w-4 text-muted-foreground" />
                              {source.channel_count}
                            </div>
                          </TableCell>
                          <TableCell>
                            {source.last_ingested_at ? (
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <div className="text-sm cursor-help">
                                    {formatRelativeTime(source.last_ingested_at)}
                                  </div>
                                </TooltipTrigger>
                                <TooltipContent>
                                  <p className="text-sm">{formatDate(source.last_ingested_at)}</p>
                                </TooltipContent>
                              </Tooltip>
                            ) : (
                              <span className="text-muted-foreground text-sm">Never</span>
                            )}
                          </TableCell>
                          <TableCell>
                            {source.next_scheduled_update ? (
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <div className="text-sm cursor-help flex items-center gap-1">
                                    <Clock className="h-3 w-3 text-muted-foreground" />
                                    {formatRelativeTime(source.next_scheduled_update)}
                                  </div>
                                </TooltipTrigger>
                                <TooltipContent>
                                  <p className="text-sm">
                                    {formatDate(source.next_scheduled_update)}
                                  </p>
                                </TooltipContent>
                              </Tooltip>
                            ) : (
                              <span className="text-muted-foreground text-sm">-</span>
                            )}
                          </TableCell>
                          <TableCell>
                            <div className="flex items-center gap-2">
                              <RefreshButton
                                resourceId={source.id}
                                onRefresh={() => {
                                  console.log(
                                    `[StreamSources] RefreshButton clicked for source ID: ${source.id}`
                                  );
                                  handleRefreshSource(source.id);
                                }}
                                onComplete={() => loadSources()}
                                disabled={!isOnline}
                                size="sm"
                                className="h-8 w-8 p-0"
                              />
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <Button
                                    variant="ghost"
                                    size="sm"
                                    onClick={() => handleEditSource(source)}
                                    className="h-8 w-8 p-0"
                                    disabled={!isOnline}
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
                                    onClick={() => handleDeleteSource(source.id)}
                                    className="h-8 w-8 p-0 text-destructive hover:text-destructive"
                                    disabled={loading.delete === source.id || !isOnline}
                                  >
                                    {loading.delete === source.id ? (
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
                ) : viewMode === 'grid' ? (
                  <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
                    {filteredSources?.map((source) => (
                      <Card key={source.id}>
                        <CardHeader className="pb-3">
                          <div className="flex items-start justify-between">
                            <div className="space-y-1">
                              <CardTitle className="text-base">{source.name}</CardTitle>
                              <Badge className={getSourceTypeColor(source.source_type)}>
                                {source.source_type.toUpperCase()}
                              </Badge>
                            </div>
                            <div className="flex items-center gap-2">
                              <Badge className={getStatusColor(source.is_active)}>
                                {source.is_active ? 'Active' : 'Inactive'}
                              </Badge>
                              {/* T049: Operation error/warning indicator */}
                              <OperationStatusIndicator resourceId={source.id} />
                            </div>
                          </div>
                        </CardHeader>
                        <CardContent className="pt-0">
                          <div className="space-y-2 text-sm">
                            <p className="text-muted-foreground truncate">{source.url}</p>
                            <div className="flex justify-between">
                              <span>Channels:</span>
                              <span>{source.channel_count || 0}</span>
                            </div>
                            <div className="flex justify-between">
                              <span>Last Updated:</span>
                              <span>
                                {source.last_ingested_at
                                  ? new Date(source.last_ingested_at).toLocaleDateString()
                                  : 'Never'}
                              </span>
                            </div>
                          </div>
                          <div className="flex justify-end gap-2 mt-3 pt-3 border-t">
                            <RefreshButton
                              resourceId={source.id}
                              onRefresh={() => handleRefreshSource(source.id)}
                              onComplete={() => loadSources()}
                              disabled={!isOnline}
                              size="sm"
                              className="h-8 w-8 p-0"
                            />
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => handleEditSource(source)}
                                  className="h-8 w-8 p-0"
                                  disabled={!isOnline}
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
                                  onClick={() => handleDeleteSource(source.id)}
                                  className="h-8 w-8 p-0 text-destructive hover:text-destructive"
                                  disabled={loading.delete === source.id || !isOnline}
                                >
                                  {loading.delete === source.id ? (
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
                        </CardContent>
                      </Card>
                    ))}
                  </div>
                ) : (
                  <div className="space-y-3">
                    {filteredSources?.map((source) => (
                      <Card key={source.id}>
                        <CardContent className="p-4">
                          <div className="flex items-center justify-between">
                            <div className="flex-1 space-y-1">
                              <div className="flex items-center gap-2">
                                <h3 className="font-medium">{source.name}</h3>
                                <Badge className={getSourceTypeColor(source.source_type)}>
                                  {source.source_type.toUpperCase()}
                                </Badge>
                                <Badge className={getStatusColor(source.is_active)}>
                                  {source.is_active ? 'Active' : 'Inactive'}
                                </Badge>
                                {/* T049: Operation error/warning indicator */}
                                <OperationStatusIndicator resourceId={source.id} />
                              </div>
                              <p className="text-sm text-muted-foreground truncate">{source.url}</p>
                              <div className="flex gap-4 text-xs text-muted-foreground">
                                <span>Channels: {source.channel_count || 0}</span>
                                <span>
                                  Last Updated:{' '}
                                  {source.last_ingested_at
                                    ? new Date(source.last_ingested_at).toLocaleDateString()
                                    : 'Never'}
                                </span>
                              </div>
                            </div>
                            <div className="flex items-center gap-2 ml-4">
                              <RefreshButton
                                resourceId={source.id}
                                onRefresh={() => handleRefreshSource(source.id)}
                                onComplete={() => loadSources()}
                                disabled={!isOnline}
                                size="sm"
                                className="h-8 w-8 p-0"
                              />
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <Button
                                    variant="ghost"
                                    size="sm"
                                    onClick={() => handleEditSource(source)}
                                    className="h-8 w-8 p-0"
                                    disabled={!isOnline}
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
                                    onClick={() => handleDeleteSource(source.id)}
                                    className="h-8 w-8 p-0 text-destructive hover:text-destructive"
                                    disabled={loading.delete === source.id || !isOnline}
                                  >
                                    {loading.delete === source.id ? (
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
                        </CardContent>
                      </Card>
                    ))}
                  </div>
                )}

                {filteredSources?.length === 0 && !loading.sources && (
                  <div className="text-center py-8">
                    <Database className="mx-auto h-12 w-12 text-muted-foreground" />
                    <h3 className="mt-4 text-lg font-semibold">
                      {searchTerm || filterType !== 'all' || filterStatus !== 'all'
                        ? 'No matching sources'
                        : 'No sources found'}
                    </h3>
                    <p className="text-muted-foreground">
                      {searchTerm || filterType !== 'all' || filterStatus !== 'all'
                        ? 'Try adjusting your search or filter criteria.'
                        : 'Get started by adding your first stream source.'}
                    </p>
                  </div>
                )}

                {/* Pagination */}
                {pagination && pagination.total_pages > 1 && (
                  <div className="flex items-center justify-between pt-4">
                    <div className="text-sm text-muted-foreground">
                      Showing {(pagination.page - 1) * pagination.per_page + 1} to{' '}
                      {Math.min(pagination.page * pagination.per_page, pagination.total)} of{' '}
                      {pagination.total} sources
                    </div>
                    <div className="flex items-center gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setCurrentPage((prev) => Math.max(1, prev - 1))}
                        disabled={!pagination.has_previous || loading.sources}
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
                        disabled={!pagination.has_next || loading.sources}
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
      </div>
    </TooltipProvider>
  );
}
