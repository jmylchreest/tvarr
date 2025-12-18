'use client';

import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { Button } from '@/components/ui/button';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Card, CardContent } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {
  Database,
  Download,
  Trash2,
  RotateCcw,
  Plus,
  Loader2,
  AlertCircle,
  CheckCircle,
  Upload,
  Settings,
  Shield,
  ShieldOff,
} from 'lucide-react';
import { BackupInfo, BackupListResponse, BackupScheduleInfo, BackupScheduleUpdateRequest, RestoreResult } from '@/types/api';
import { apiClient } from '@/lib/api-client';
import { describeCronExpression, validateCronExpression } from '@/lib/cron-validation';
import { formatFileSize, formatDate, formatRelativeTimeShort } from '@/lib/format';
import {
  MasterDetailLayout,
  DetailPanel,
  DetailEmpty,
  MasterItem,
  BadgeGroup,
  BadgeItem,
} from '@/components/shared';

// Selection can be either 'schedule' or a backup filename
type Selection = { type: 'schedule' } | { type: 'backup'; backup: BackupInfo } | null;

// State for backup directory (from API)
interface ScheduleWithDirectory {
  schedule: BackupScheduleInfo | null;
  backupDirectory: string;
}

// Convert backup to MasterItem format
interface BackupMasterItem extends MasterItem {
  backup?: BackupInfo;
  isSchedule?: boolean;
}

function backupToMasterItem(backup: BackupInfo): BackupMasterItem {
  const badges: BadgeItem[] = [];

  // Shield badge has higher priority (success = 60)
  if (backup.protected) {
    badges.push({
      label: <Shield className="h-3 w-3" />,
      priority: 'success',
      className: 'bg-blue-500/10 text-blue-600 border-blue-500/30',
    });
  }

  // Version badge has lower priority (outline = 10)
  badges.push({
    label: backup.tvarr_version,
    priority: 'outline',
  });

  return {
    id: backup.filename,
    title: backup.filename.replace('tvarr-backup-', '').replace('.tar.gz', '').replace('.db.gz', ''),
    subtitle: formatRelativeTimeShort(backup.created_at),
    badge: <BadgeGroup badges={badges} />,
    icon: <Database className={`h-4 w-4 ${backup.imported || backup.tvarr_version === 'imported' ? 'text-chart-1' : 'opacity-50'}`} />,
    backup,
  };
}

// Schedule detail panel
function ScheduleDetailPanel({
  schedule,
  backupDirectory,
  onSave,
  saving,
  error,
}: {
  schedule: BackupScheduleInfo | null;
  backupDirectory: string;
  onSave: (settings: BackupScheduleUpdateRequest) => Promise<void>;
  saving: boolean;
  error: string | null;
}) {
  const [formData, setFormData] = useState({
    enabled: false,
    cron: '',
    retention: 7,
  });
  const [cronError, setCronError] = useState<string | null>(null);

  // Sync form data when schedule changes
  useEffect(() => {
    if (schedule) {
      setFormData({
        enabled: schedule.enabled,
        cron: schedule.cron,
        retention: schedule.retention,
      });
    }
  }, [schedule]);

  const handleCronChange = (value: string) => {
    setFormData(prev => ({ ...prev, cron: value }));
    if (value) {
      const result = validateCronExpression(value);
      setCronError(result.isValid ? null : result.error || 'Invalid cron expression');
    } else {
      setCronError(null);
    }
  };

  const handleSave = async () => {
    if (cronError) return;
    await onSave({
      enabled: formData.enabled,
      cron: formData.cron,
      retention: formData.retention,
    });
  };

  return (
    <DetailPanel
      title="Backup Schedule"
      actions={
        <Button onClick={handleSave} disabled={saving || !!cronError}>
          {saving ? (
            <>
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              Saving...
            </>
          ) : (
            'Save'
          )}
        </Button>
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

        <p className="text-sm text-muted-foreground">
          Configure automatic database backups. Changes take effect immediately.
        </p>

        {/* Enabled toggle */}
        <div className="flex items-center justify-between rounded-lg border p-4">
          <div className="space-y-0.5">
            <Label htmlFor="backup-enabled" className="text-base">Automatic Backups</Label>
            <p className="text-sm text-muted-foreground">
              Enable scheduled database backups
            </p>
          </div>
          <Switch
            id="backup-enabled"
            checked={formData.enabled}
            onCheckedChange={(checked) => setFormData(prev => ({ ...prev, enabled: checked }))}
          />
        </div>

        {/* Cron schedule */}
        <div className="space-y-2">
          <Label htmlFor="backup-cron">Schedule (Cron)</Label>
          <Input
            id="backup-cron"
            value={formData.cron}
            onChange={(e) => handleCronChange(e.target.value)}
            placeholder="0 0 2 * * *"
            className={cronError ? 'border-destructive' : ''}
          />
          {cronError ? (
            <p className="text-sm text-destructive">{cronError}</p>
          ) : formData.cron ? (
            <p className="text-sm text-muted-foreground">
              {describeCronExpression(formData.cron)}
            </p>
          ) : (
            <p className="text-sm text-muted-foreground">
              6-field cron: seconds minutes hours day-of-month month day-of-week
            </p>
          )}
        </div>

        {/* Retention */}
        <div className="space-y-2">
          <Label htmlFor="backup-retention">Retention (backups to keep)</Label>
          <Input
            id="backup-retention"
            type="number"
            min={0}
            value={formData.retention}
            onChange={(e) => setFormData(prev => ({ ...prev, retention: parseInt(e.target.value) || 0 }))}
          />
          <p className="text-sm text-muted-foreground">
            {formData.retention === 0
              ? 'Keep all backups (no automatic cleanup)'
              : `Keep the ${formData.retention} most recent backups`}
          </p>
        </div>

        {/* Current status */}
        {schedule && (
          <div className="rounded-lg border p-4 bg-muted/50">
            <h4 className="font-medium mb-2">Current Status</h4>
            <div className="grid grid-cols-2 gap-2 text-sm">
              <span className="text-muted-foreground">Status:</span>
              <span>{schedule.enabled ? 'Enabled' : 'Disabled'}</span>
              <span className="text-muted-foreground">Schedule:</span>
              <span>{schedule.cron ? describeCronExpression(schedule.cron) : 'Not set'}</span>
              <span className="text-muted-foreground">Retention:</span>
              <span>{schedule.retention === 0 ? 'Unlimited' : `${schedule.retention} backups`}</span>
            </div>
          </div>
        )}

        {/* Backup location */}
        {backupDirectory && (
          <div className="rounded-lg border p-4">
            <h4 className="font-medium mb-2">Storage Location</h4>
            <p className="text-sm font-mono text-muted-foreground break-all">{backupDirectory}</p>
            <p className="text-xs text-muted-foreground mt-2">
              Configure via <code className="bg-muted px-1 rounded">backup.directory</code> in config or{' '}
              <code className="bg-muted px-1 rounded">TVARR_BACKUP_DIRECTORY</code> environment variable.
            </p>
          </div>
        )}
      </div>
    </DetailPanel>
  );
}

// Backup detail panel
function BackupDetailPanel({
  backup,
  onRestore,
  onDelete,
  onDownload,
  onToggleProtection,
  restoring,
  deleting,
  togglingProtection,
}: {
  backup: BackupInfo;
  onRestore: (backup: BackupInfo) => void;
  onDelete: (filename: string) => void;
  onDownload: (filename: string) => void;
  onToggleProtection: (filename: string, protected_: boolean) => void;
  restoring: boolean;
  deleting: boolean;
  togglingProtection: boolean;
}) {
  return (
    <DetailPanel
      title={backup.filename}
      actions={
        <div className="flex items-center gap-2">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant={backup.protected ? 'default' : 'outline'}
                size="sm"
                onClick={() => onToggleProtection(backup.filename, !backup.protected)}
                disabled={togglingProtection}
                className={backup.protected ? 'bg-blue-500 hover:bg-blue-600' : ''}
              >
                {togglingProtection ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : backup.protected ? (
                  <Shield className="h-4 w-4" />
                ) : (
                  <ShieldOff className="h-4 w-4" />
                )}
              </Button>
            </TooltipTrigger>
            <TooltipContent>{backup.protected ? 'Remove protection' : 'Protect from cleanup'}</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="outline" size="sm" onClick={() => onDownload(backup.filename)}>
                <Download className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Download</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="outline" size="sm" onClick={() => onRestore(backup)} disabled={restoring}>
                {restoring ? <Loader2 className="h-4 w-4 animate-spin" /> : <RotateCcw className="h-4 w-4" />}
              </Button>
            </TooltipTrigger>
            <TooltipContent>Restore</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="outline" size="sm" onClick={() => onDelete(backup.filename)} disabled={deleting}>
                {deleting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4 text-destructive" />}
              </Button>
            </TooltipTrigger>
            <TooltipContent>Delete</TooltipContent>
          </Tooltip>
        </div>
      }
    >
      <div className="space-y-6">
        {/* Basic info */}
        <div className="grid grid-cols-2 gap-4">
          <div>
            <p className="text-sm text-muted-foreground">Created</p>
            <p className="font-medium">{formatDate(backup.created_at)}</p>
            <p className="text-sm text-muted-foreground">{formatRelativeTimeShort(backup.created_at)}</p>
          </div>
          <div>
            <p className="text-sm text-muted-foreground">Version</p>
            <p className="font-medium">{backup.tvarr_version}</p>
          </div>
        </div>

        {/* Size info */}
        <div className="rounded-lg border p-4">
          <h4 className="font-medium mb-3">Size</h4>
          <div className="grid grid-cols-2 gap-2 text-sm">
            <span className="text-muted-foreground">Compressed:</span>
            <span>{formatFileSize(backup.file_size)}</span>
            <span className="text-muted-foreground">Original:</span>
            <span>{formatFileSize(backup.database_size)}</span>
            <span className="text-muted-foreground">Compression:</span>
            <span>{backup.database_size > 0 ? `${Math.round((1 - backup.file_size / backup.database_size) * 100)}%` : 'N/A'}</span>
          </div>
        </div>

        {/* Table counts */}
        <div className="rounded-lg border p-4">
          <h4 className="font-medium mb-3">Contents</h4>
          <div className="grid grid-cols-2 gap-2 text-sm">
            <span className="text-muted-foreground">Stream Sources:</span>
            <span>{backup.table_counts.stream_sources}</span>
            <span className="text-muted-foreground">EPG Sources:</span>
            <span>{backup.table_counts.epg_sources}</span>
            <span className="text-muted-foreground">Channels:</span>
            <span>{backup.table_counts.channels}</span>
            <span className="text-muted-foreground">EPG Programs:</span>
            <span>{backup.table_counts.epg_programs.toLocaleString()}</span>
            <span className="text-muted-foreground">Proxies:</span>
            <span>{backup.table_counts.stream_proxies}</span>
            <span className="text-muted-foreground">Filters:</span>
            <span>{backup.table_counts.filters}</span>
            <span className="text-muted-foreground">Data Mapping Rules:</span>
            <span>{backup.table_counts.data_mapping_rules}</span>
            <span className="text-muted-foreground">Client Detection Rules:</span>
            <span>{backup.table_counts.client_detection_rules}</span>
            <span className="text-muted-foreground">Encoding Profiles:</span>
            <span>{backup.table_counts.encoding_profiles}</span>
          </div>
        </div>

        {/* Protection status */}
        <div className="rounded-lg border p-4">
          <div className="flex items-center gap-2 mb-2">
            {backup.protected ? (
              <>
                <Shield className="h-4 w-4 text-blue-500" />
                <h4 className="font-medium">Protected</h4>
              </>
            ) : (
              <>
                <ShieldOff className="h-4 w-4 text-muted-foreground" />
                <h4 className="font-medium">Not Protected</h4>
              </>
            )}
          </div>
          <p className="text-sm text-muted-foreground">
            {backup.protected
              ? 'This backup is protected from automatic retention cleanup.'
              : 'This backup may be deleted by automatic retention cleanup when older backups exceed the retention limit.'}
          </p>
        </div>

        {/* Checksum */}
        <div>
          <p className="text-sm text-muted-foreground">Checksum (SHA256)</p>
          <p className="font-mono text-xs break-all">{backup.checksum}</p>
        </div>
      </div>
    </DetailPanel>
  );
}

export function Backups() {
  const [backups, setBackups] = useState<BackupInfo[]>([]);
  const [schedule, setSchedule] = useState<BackupScheduleInfo | null>(null);
  const [backupDirectory, setBackupDirectory] = useState<string>('');
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [restoring, setRestoring] = useState<string | null>(null);
  const [togglingProtection, setTogglingProtection] = useState<string | null>(null);
  const [savingSchedule, setSavingSchedule] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [scheduleError, setScheduleError] = useState<string | null>(null);

  // Selection state
  const [selection, setSelection] = useState<Selection>({ type: 'schedule' });

  // Hidden file input ref for upload
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Restore confirmation dialog
  const [restoreDialog, setRestoreDialog] = useState<BackupInfo | null>(null);

  const loadBackups = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      const response: BackupListResponse = await apiClient.listBackups();
      // Sort backups by created_at descending (most recent first)
      const sorted = [...(response.backups || [])].sort(
        (a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
      );
      setBackups(sorted);
      setSchedule(response.schedule || null);
      setBackupDirectory(response.backup_directory || '');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load backups');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadBackups();
  }, [loadBackups]);

  const handleCreateBackup = async () => {
    setCreating(true);
    setError(null);
    setSuccess(null);

    try {
      const backup = await apiClient.createBackup();
      setSuccess(`Backup created: ${backup.filename}`);
      await loadBackups();
      // Select the new backup
      setSelection({ type: 'backup', backup });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create backup');
    } finally {
      setCreating(false);
    }
  };

  const handleDeleteBackup = async (filename: string) => {
    setDeleting(filename);
    setError(null);
    setSuccess(null);

    try {
      await apiClient.deleteBackup(filename);
      setSuccess(`Backup deleted: ${filename}`);
      // Clear selection if deleted backup was selected
      if (selection?.type === 'backup' && selection.backup.filename === filename) {
        setSelection({ type: 'schedule' });
      }
      await loadBackups();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete backup');
    } finally {
      setDeleting(null);
    }
  };

  const handleRestoreBackup = async (backup: BackupInfo) => {
    setRestoreDialog(null);
    setRestoring(backup.filename);
    setError(null);
    setSuccess(null);

    try {
      const result: RestoreResult = await apiClient.restoreBackup(backup.filename);
      if (result.success) {
        setSuccess(
          `Database restored from ${backup.filename}. ${result.pre_restore_backup ? `Pre-restore backup: ${result.pre_restore_backup}` : ''}`
        );
        // Reload page after short delay to reconnect to restored database
        setTimeout(() => {
          window.location.reload();
        }, 3000);
      } else {
        setError(result.message || 'Restore failed');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to restore backup');
    } finally {
      setRestoring(null);
    }
  };

  const handleDownloadBackup = (filename: string) => {
    const url = apiClient.getBackupDownloadUrl(filename);
    const link = document.createElement('a');
    link.href = url;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
  };

  const handleUploadClick = () => {
    fileInputRef.current?.click();
  };

  const handleFileSelect = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    // Reset the input so the same file can be selected again
    event.target.value = '';

    // Validate file extension (support both new .tar.gz and legacy .db.gz)
    if (!file.name.endsWith('.tar.gz') && !file.name.endsWith('.db.gz')) {
      setError('Invalid file type. Please select a tvarr backup file (.tar.gz or .db.gz)');
      return;
    }

    setUploading(true);
    setError(null);
    setSuccess(null);

    try {
      const backup = await apiClient.uploadBackup(file);
      setSuccess(`Backup uploaded: ${backup.filename}`);
      await loadBackups();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to upload backup');
    } finally {
      setUploading(false);
    }
  };

  const handleSaveSchedule = async (settings: BackupScheduleUpdateRequest) => {
    setSavingSchedule(true);
    setScheduleError(null);

    try {
      const updatedSchedule = await apiClient.updateBackupSchedule(settings);
      setSchedule(updatedSchedule);
      setSuccess('Backup schedule updated');
    } catch (err) {
      setScheduleError(err instanceof Error ? err.message : 'Failed to update schedule');
    } finally {
      setSavingSchedule(false);
    }
  };

  const handleToggleProtection = async (filename: string, protected_: boolean) => {
    setTogglingProtection(filename);
    setError(null);

    try {
      const updated = await apiClient.setBackupProtection(filename, protected_);
      // Update the backup in state
      setBackups(prev => prev.map(b => b.filename === filename ? updated : b));
      // Update selection if this backup is selected
      if (selection?.type === 'backup' && selection.backup.filename === filename) {
        setSelection({ type: 'backup', backup: updated });
      }
      setSuccess(protected_ ? `${filename} is now protected` : `Protection removed from ${filename}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update protection');
    } finally {
      setTogglingProtection(null);
    }
  };

  // Convert backups to master items
  const masterItems = useMemo(() => {
    const items: BackupMasterItem[] = [];

    // Add schedule as first item
    const scheduleBadges: BadgeItem[] = [{
      label: schedule?.enabled ? 'On' : 'Off',
      priority: schedule?.enabled ? 'success' : 'outline',
      className: schedule?.enabled ? 'bg-green-500/10 text-green-600 border-green-500/30' : undefined,
    }];
    items.push({
      id: '__schedule__',
      title: 'Schedule',
      subtitle: schedule?.enabled && schedule?.cron ? describeCronExpression(schedule.cron) : 'Disabled',
      icon: <Settings className="h-4 w-4" />,
      badge: <BadgeGroup badges={scheduleBadges} />,
      isSchedule: true,
    });

    // Add backups
    backups.forEach(backup => {
      items.push(backupToMasterItem(backup));
    });

    return items;
  }, [backups, schedule]);

  // Get selected ID for MasterDetailLayout
  const selectedId = selection?.type === 'schedule'
    ? '__schedule__'
    : selection?.type === 'backup'
    ? selection.backup.filename
    : null;

  // Handle selection
  const handleSelect = (item: BackupMasterItem | null) => {
    if (!item) {
      setSelection(null);
      return;
    }
    if (item.isSchedule) {
      setSelection({ type: 'schedule' });
    } else if (item.backup) {
      setSelection({ type: 'backup', backup: item.backup });
    }
  };

  // Determine what to show in detail panel
  const selectedBackup = selection?.type === 'backup' ? selection.backup : null;
  const showSchedule = selection?.type === 'schedule';

  return (
    <TooltipProvider>
      <div className="flex flex-col gap-6 h-full">
        {/* Hidden file input for upload */}
        <input
          type="file"
          ref={fileInputRef}
          onChange={handleFileSelect}
          accept=".tar.gz,.db.gz"
          className="hidden"
        />

        {/* Alerts */}
        {(error || success) && (
          <div>
            {error && (
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertTitle>Error</AlertTitle>
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            {success && (
              <Alert>
                <CheckCircle className="h-4 w-4 text-green-500" />
                <AlertTitle>Success</AlertTitle>
                <AlertDescription>{success}</AlertDescription>
              </Alert>
            )}
          </div>
        )}

        <Card className="flex-1 overflow-hidden min-h-0">
          <CardContent className="p-0 h-full">
            <MasterDetailLayout
              items={masterItems}
              selectedId={selectedId}
              onSelect={handleSelect}
              isLoading={loading}
              title="Backups"
              searchPlaceholder="Search backups..."
              headerAction={
                <div className="flex items-center gap-1">
                  <Button variant="ghost" size="sm" onClick={handleUploadClick} disabled={uploading || loading}>
                    {uploading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Upload className="h-4 w-4" />}
                  </Button>
                  <Button variant="ghost" size="sm" onClick={handleCreateBackup} disabled={creating || loading}>
                    {creating ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
                  </Button>
                </div>
              }
              emptyState={{
                title: 'No backups',
                description: 'Create a backup to get started',
                actionLabel: 'Create Backup',
              }}
              filterFn={(item, term) => {
                if (item.isSchedule) return true; // Always show schedule
                return item.title.toLowerCase().includes(term.toLowerCase()) ||
                       (item.subtitle?.toLowerCase().includes(term.toLowerCase()) ?? false);
              }}
              renderItem={(item, isSelected) => (
                <div
                  className={`flex items-center gap-2 px-2 py-1.5 rounded-md cursor-pointer transition-colors overflow-hidden hover:bg-accent ${
                    isSelected ? 'bg-accent' : ''
                  } ${item.isSchedule ? 'border-b mb-1 pb-2' : ''}`}
                >
                  {item.icon && (
                    <div className="flex-shrink-0 text-muted-foreground">
                      {item.icon}
                    </div>
                  )}
                  <div className="flex-1 min-w-0 overflow-hidden">
                    <div className="text-sm font-medium truncate">{item.title}</div>
                    {item.subtitle && (
                      <div className="text-[11px] text-muted-foreground truncate">
                        {item.subtitle}
                      </div>
                    )}
                  </div>
                  {item.badge && <div className="flex-shrink-0 ml-auto">{item.badge}</div>}
                </div>
              )}
            >
              {() => (
                <>
                  {showSchedule && (
                    <ScheduleDetailPanel
                      schedule={schedule}
                      backupDirectory={backupDirectory}
                      onSave={handleSaveSchedule}
                      saving={savingSchedule}
                      error={scheduleError}
                    />
                  )}
                  {selectedBackup && (
                    <BackupDetailPanel
                      backup={selectedBackup}
                      onRestore={setRestoreDialog}
                      onDelete={handleDeleteBackup}
                      onDownload={handleDownloadBackup}
                      onToggleProtection={handleToggleProtection}
                      restoring={restoring === selectedBackup.filename}
                      deleting={deleting === selectedBackup.filename}
                      togglingProtection={togglingProtection === selectedBackup.filename}
                    />
                  )}
                  {!showSchedule && !selectedBackup && (
                    <DetailEmpty
                      title="Select a backup"
                      description="Choose a backup from the list to view details, or select Schedule to configure automatic backups"
                      icon={<Database className="h-12 w-12" />}
                    />
                  )}
                </>
              )}
            </MasterDetailLayout>
          </CardContent>
        </Card>

        {/* Restore Confirmation Dialog */}
        <Dialog open={restoreDialog !== null} onOpenChange={() => setRestoreDialog(null)}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Restore Database</DialogTitle>
              <DialogDescription>
                This will replace all current data with the backup contents. A pre-restore
                backup will be created automatically.
              </DialogDescription>
            </DialogHeader>

            {restoreDialog && (
              <div className="space-y-4">
                <Alert variant="destructive">
                  <AlertCircle className="h-4 w-4" />
                  <AlertTitle>Warning</AlertTitle>
                  <AlertDescription>
                    Active streams will be interrupted. This action cannot be undone without
                    restoring from another backup.
                  </AlertDescription>
                </Alert>

                <div className="bg-muted p-4 rounded-lg space-y-2">
                  <div className="flex justify-between text-sm">
                    <span className="text-muted-foreground">Backup:</span>
                    <span className="font-mono">{restoreDialog.filename}</span>
                  </div>
                  <div className="flex justify-between text-sm">
                    <span className="text-muted-foreground">Created:</span>
                    <span>{formatDate(restoreDialog.created_at)}</span>
                  </div>
                  <div className="flex justify-between text-sm">
                    <span className="text-muted-foreground">Version:</span>
                    <span>{restoreDialog.tvarr_version}</span>
                  </div>
                </div>
              </div>
            )}

            <DialogFooter>
              <Button variant="outline" onClick={() => setRestoreDialog(null)}>
                Cancel
              </Button>
              <Button
                variant="destructive"
                onClick={() => restoreDialog && handleRestoreBackup(restoreDialog)}
              >
                <RotateCcw className="mr-2 h-4 w-4" />
                Restore Database
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </TooltipProvider>
  );
}
