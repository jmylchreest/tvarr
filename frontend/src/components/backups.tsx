'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
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
  Clock,
  HardDrive,
  FolderOpen,
  Calendar,
  Upload,
} from 'lucide-react';
import { BackupInfo, BackupListResponse, BackupScheduleInfo, RestoreResult } from '@/types/api';
import { apiClient } from '@/lib/api-client';
import { StatCard } from '@/components/shared/feedback/StatCard';

function formatFileSize(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
}

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleString();
}

function formatRelativeTime(dateString: string): string {
  const now = new Date();
  const date = new Date(dateString);
  const diffMs = now.getTime() - date.getTime();
  const diffMins = Math.floor(diffMs / (1000 * 60));
  const diffHours = Math.floor(diffMins / 60);
  const diffDays = Math.floor(diffHours / 24);

  if (diffDays > 0) {
    return `${diffDays}d ago`;
  } else if (diffHours > 0) {
    return `${diffHours}h ago`;
  } else if (diffMins > 0) {
    return `${diffMins}m ago`;
  }
  return 'Just now';
}

function parseCronExpression(cron: string): string {
  // Very simplified cron description
  const parts = cron.split(' ');
  if (parts.length >= 6) {
    const [second, minute, hour] = parts;
    if (second === '0' && minute === '0') {
      return `Daily at ${hour}:00`;
    }
    return `${hour}:${minute}`;
  }
  return cron;
}

export function Backups() {
  const [backups, setBackups] = useState<BackupInfo[]>([]);
  const [backupDir, setBackupDir] = useState<string>('');
  const [schedule, setSchedule] = useState<BackupScheduleInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [restoring, setRestoring] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  // Hidden file input ref for upload
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Restore confirmation dialog
  const [restoreDialog, setRestoreDialog] = useState<BackupInfo | null>(null);

  const loadBackups = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      const response: BackupListResponse = await apiClient.listBackups();
      setBackups(response.backups || []);
      setBackupDir(response.backup_directory || '');
      setSchedule(response.schedule || null);
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

    // Validate file extension
    if (!file.name.endsWith('.db.gz')) {
      setError('Invalid file type. Please select a tvarr backup file (.db.gz)');
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

  const totalBackupSize = backups.reduce((sum, b) => sum + b.file_size, 0);

  return (
    <TooltipProvider>
      <div className="space-y-6">
        {/* Hidden file input for upload */}
        <input
          type="file"
          ref={fileInputRef}
          onChange={handleFileSelect}
          accept=".db.gz"
          className="hidden"
        />

        {/* Header */}
        <div className="flex items-center justify-between">
          <p className="text-muted-foreground">
            Manage database backups and restore points
          </p>
          <div className="flex items-center gap-2">
            <Button variant="outline" onClick={handleUploadClick} disabled={uploading || loading}>
              {uploading ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Uploading...
                </>
              ) : (
                <>
                  <Upload className="mr-2 h-4 w-4" />
                  Upload
                </>
              )}
            </Button>
            <Button onClick={handleCreateBackup} disabled={creating || loading}>
              {creating ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Creating...
                </>
              ) : (
                <>
                  <Plus className="mr-2 h-4 w-4" />
                  Create
                </>
              )}
            </Button>
          </div>
        </div>

        {/* Alerts */}
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

        {/* Statistics */}
        <div className="grid gap-2 md:grid-cols-5">
          <StatCard
            title="Total Backups"
            value={backups.length}
            icon={<Database className="h-4 w-4" />}
          />
          <StatCard
            title="Storage Used"
            value={formatFileSize(totalBackupSize)}
            icon={<HardDrive className="h-4 w-4" />}
          />
          <StatCard
            title="Schedule"
            value={schedule?.enabled ? 'Active' : 'Disabled'}
            icon={<Clock className="h-4 w-4" />}
            className={schedule?.enabled ? 'border-green-500/30' : ''}
          />
          <StatCard
            title="Retention"
            value={schedule?.retention || 0}
            icon={<Calendar className="h-4 w-4" />}
          />
          <Tooltip>
            <TooltipTrigger asChild>
              <div>
                <StatCard
                  title="Location"
                  value={backupDir ? '...' + backupDir.slice(-15) : 'Not set'}
                  icon={<FolderOpen className="h-4 w-4" />}
                />
              </div>
            </TooltipTrigger>
            <TooltipContent>
              <code className="text-xs">{backupDir || 'Not configured'}</code>
            </TooltipContent>
          </Tooltip>
        </div>

      {/* Backups Table */}
      <Card>
        <CardHeader>
          <CardTitle>Backups</CardTitle>
          <CardDescription>
            Available backup files for restore. Most recent backups are shown first.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {loading ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
            </div>
          ) : backups.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">
              <Database className="h-12 w-12 mx-auto mb-4 opacity-50" />
              <p>No backups found</p>
              <p className="text-sm">Create a backup to get started</p>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Filename</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead>Size</TableHead>
                  <TableHead>Version</TableHead>
                  <TableHead>Contents</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {backups.map((backup) => (
                  <TableRow key={backup.filename}>
                    <TableCell className="font-mono text-sm">
                      {backup.filename}
                    </TableCell>
                    <TableCell>
                      <Tooltip>
                        <TooltipTrigger className="cursor-default">
                          {formatRelativeTime(backup.created_at)}
                        </TooltipTrigger>
                        <TooltipContent>
                          {formatDate(backup.created_at)}
                        </TooltipContent>
                      </Tooltip>
                    </TableCell>
                    <TableCell>
                      <Tooltip>
                        <TooltipTrigger className="cursor-default">
                          {formatFileSize(backup.file_size)}
                        </TooltipTrigger>
                        <TooltipContent>
                          Compressed: {formatFileSize(backup.file_size)}
                          <br />
                          Original: {formatFileSize(backup.database_size)}
                        </TooltipContent>
                      </Tooltip>
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline">{backup.tvarr_version}</Badge>
                    </TableCell>
                    <TableCell>
                      <Tooltip>
                        <TooltipTrigger className="cursor-default">
                          <span className="text-sm text-muted-foreground">
                            {backup.table_counts.channels} channels, {backup.table_counts.filters} filters
                          </span>
                        </TooltipTrigger>
                        <TooltipContent className="text-left">
                          <div className="space-y-1">
                            <div>Stream Sources: {backup.table_counts.stream_sources}</div>
                            <div>EPG Sources: {backup.table_counts.epg_sources}</div>
                            <div>Channels: {backup.table_counts.channels}</div>
                            <div>Proxies: {backup.table_counts.stream_proxies}</div>
                            <div>Filters: {backup.table_counts.filters}</div>
                            <div>Data Mapping Rules: {backup.table_counts.data_mapping_rules}</div>
                            <div>Client Detection Rules: {backup.table_counts.client_detection_rules}</div>
                            <div>Encoding Profiles: {backup.table_counts.encoding_profiles}</div>
                          </div>
                        </TooltipContent>
                      </Tooltip>
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex items-center justify-end gap-1">
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon"
                              onClick={() => handleDownloadBackup(backup.filename)}
                            >
                              <Download className="h-4 w-4" />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>Download</TooltipContent>
                        </Tooltip>

                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon"
                              onClick={() => setRestoreDialog(backup)}
                              disabled={restoring === backup.filename}
                            >
                              {restoring === backup.filename ? (
                                <Loader2 className="h-4 w-4 animate-spin" />
                              ) : (
                                <RotateCcw className="h-4 w-4" />
                              )}
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>Restore</TooltipContent>
                        </Tooltip>

                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon"
                              onClick={() => handleDeleteBackup(backup.filename)}
                              disabled={deleting === backup.filename}
                            >
                              {deleting === backup.filename ? (
                                <Loader2 className="h-4 w-4 animate-spin" />
                              ) : (
                                <Trash2 className="h-4 w-4 text-destructive" />
                              )}
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>Delete</TooltipContent>
                        </Tooltip>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
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
