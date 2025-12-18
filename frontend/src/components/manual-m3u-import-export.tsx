'use client';

/**
 * ManualM3UImportExport
 *
 * UI component providing M3U import / export tooling for Manual Stream Sources.
 *
 * Features:
 *  - Import dialog:
 *      * Paste raw M3U
 *      * Preview parse (server-side validation)
 *      * Show parsed channel count & diff vs current in-memory editor state
 *      * Apply (replace + materialize) with progress feedback
 *  - Export dialog:
 *      * Fetch current manual definitions from backend as M3U
 *      * Download file or copy to clipboard
 *
 * Assumptions:
 *  - Back-end endpoints implemented:
 *      GET  /api/v1/sources/stream/{id}/manual-channels
 *      PUT  /api/v1/sources/stream/{id}/manual-channels
 *      POST /api/v1/sources/stream/{id}/manual-channels/import-m3u[?apply=true]
 *      GET  /api/v1/sources/stream/{id}/manual-channels/export.m3u
 *
 *  - Existing apiClient exposes:
 *      listManualChannels, replaceManualChannels,
 *      importManualChannelsM3U, exportManualChannelsM3U
 *
 *  - All manual channel rows are treated active by product decision.
 *
 * Integration example (inside Manual source edit sheet):
 *
 *  <ManualM3UImportExport
 *     sourceId={source.id}
 *     existingChannels={manualChannels}
 *     onPreview={(parsed) => setManualChannels(parsed)}
 *     onApplied={() => { reloadChannels(); }}
 *  />
 */

import React, { useCallback, useMemo, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger, DialogFooter, DialogDescription } from '@/components/ui/dialog';
import { Textarea } from '@/components/ui/textarea';
import { Input } from '@/components/ui/input';
import { Badge } from '@/components/ui/badge';
import { Separator } from '@/components/ui/separator';
import {
  Upload,
  Download,
  RefreshCw,
  Check,
  AlertCircle,
  FileUp,
  FileDown,
  X,
  Clipboard,
} from 'lucide-react';
import { apiClient } from '@/lib/api-client';
import { ManualChannelInput } from '@/types/api';
import { cn } from '@/lib/utils';

interface ManualM3UImportExportProps {
  sourceId: string;
  existingChannels?: ManualChannelInput[]; // current unsaved editor contents (optional)
  disabled?: boolean;
  onPreviewParsed?: (channels: ManualChannelInput[]) => void;
  onApplied?: (summary: any) => void;
  onExportFetched?: (m3u: string) => void;
  className?: string;
}

type ImportPhase = 'idle' | 'parsing' | 'parsed' | 'applying';

export const ManualM3UImportExport: React.FC<ManualM3UImportExportProps> = ({
  sourceId,
  existingChannels,
  disabled,
  onPreviewParsed,
  onApplied,
  onExportFetched,
  className,
}) => {
  // Import dialog state
  const [importOpen, setImportOpen] = useState(false);
  const [exportOpen, setExportOpen] = useState(false);
  const [m3uText, setM3uText] = useState('');
  const [importPhase, setImportPhase] = useState<ImportPhase>('idle');
  const [parsedChannels, setParsedChannels] = useState<ManualChannelInput[] | null>(null);
  const [importError, setImportError] = useState<string | null>(null);
  const [applySummary, setApplySummary] = useState<any | null>(null);

  // Export state
  const [exportLoading, setExportLoading] = useState(false);
  const [exportError, setExportError] = useState<string | null>(null);
  const [exportedM3U, setExportedM3U] = useState<string>('');
  const [downloadFileName, setDownloadFileName] = useState('manual_channels.m3u');

  const resetImportState = useCallback(() => {
    setM3uText('');
    setParsedChannels(null);
    setImportPhase('idle');
    setImportError(null);
    setApplySummary(null);
  }, []);

  const openImport = () => {
    resetImportState();
    setImportOpen(true);
  };

  const channelDiff = useMemo(() => {
    if (!parsedChannels || !existingChannels) return null;
    // We calculate simple high-level differences:
    // - count difference
    // - overlap by name + stream_url
    const key = (c: ManualChannelInput) => `${c.channel_name.trim()}||${c.stream_url.trim()}`;
    const existingSet = new Set(existingChannels.map(key));
    const parsedSet = new Set(parsedChannels.map(key));
    let overlap = 0;
    parsedSet.forEach((k) => {
      if (existingSet.has(k)) overlap += 1;
    });
    return {
      existing: existingChannels.length,
      parsed: parsedChannels.length,
      overlap,
      added: parsedChannels.length - overlap,
      removedPotential: existingChannels.length - overlap,
    };
  }, [parsedChannels, existingChannels]);

  const handlePreview = async () => {
    if (!m3uText.trim()) {
      setImportError('Please paste M3U content first.');
      return;
    }
    setImportError(null);
    setImportPhase('parsing');
    setParsedChannels(null);
    setApplySummary(null);
    try {
      const result = await apiClient.importManualChannelsM3U(sourceId, m3uText, false);
      // result should be an array of parsed channel definitions
      if (!Array.isArray(result)) {
        throw new Error('Unexpected parse response format (expected array).');
      }
      setParsedChannels(result);
      onPreviewParsed?.(result);
      setImportPhase('parsed');
    } catch (e: any) {
      setImportPhase('idle');
      setImportError(e?.message || 'Failed to parse M3U');
    }
  };

  const handleApply = async () => {
    if (!parsedChannels || parsedChannels.length === 0) {
      setImportError('No parsed channels to apply.');
      return;
    }
    setImportError(null);
    setImportPhase('applying');
    setApplySummary(null);
    try {
      const summary = await apiClient.importManualChannelsM3U(sourceId, m3uText, true);
      setApplySummary(summary);
      onApplied?.(summary);
      // Keep dialog open to show summary, but allow user to close manually.
      setImportPhase('parsed'); // revert phase for further context
    } catch (e: any) {
      setImportPhase('parsed');
      setImportError(e?.message || 'Failed to apply imported channels.');
    }
  };

  const handleExport = async () => {
    setExportError(null);
    setExportLoading(true);
    try {
      const text = await apiClient.exportManualChannelsM3U(sourceId);
      setExportedM3U(text);
      onExportFetched?.(text);
      // Derive a filename with timestamp
      const ts = new Date().toISOString().replace(/[:.]/g, '-');
      setDownloadFileName(`manual_channels_${ts}.m3u`);
    } catch (e: any) {
      setExportError(e?.message || 'Failed to export manual channels.');
    } finally {
      setExportLoading(false);
    }
  };

  const handleDownload = () => {
    const blob = new Blob([exportedM3U], { type: 'audio/x-mpegurl;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = downloadFileName || 'manual_channels.m3u';
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  };

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(exportedM3U);
    } catch {
      // swallow
    }
  };

  return (
    <div className={cn('flex items-center gap-2 flex-wrap', className)}>
      {/* Import Button */}
      <Dialog open={importOpen} onOpenChange={setImportOpen}>
        <DialogTrigger asChild>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={disabled}
            onClick={openImport}
            className="gap-1"
          >
            <FileUp className="h-4 w-4" />
            Import M3U
          </Button>
        </DialogTrigger>
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>Import Manual Channels (M3U)</DialogTitle>
            <DialogDescription>
              Paste an extended M3U playlist. Preview first, then apply to replace existing manual
              channels.
            </DialogDescription>
          </DialogHeader>

            <div className="space-y-4 max-h-[72vh] overflow-y-auto pr-1">
              {/* Textarea */}
              <div className="space-y-2">
                <label className="text-sm font-medium">M3U Playlist</label>
                <Textarea
                  value={m3uText}
                  onChange={(e) => setM3uText(e.target.value)}
                  rows={12}
                  placeholder="#EXTM3U&#10;#EXTINF:-1 tvg-id=&quot;id&quot; tvg-name=&quot;Name&quot; group-title=&quot;Group&quot; tvg-logo=&quot;http://logo&quot; tvg-chno=&quot;1&quot;,Channel Name&#10;http://example.com/stream1"
                  disabled={importPhase === 'parsing' || importPhase === 'applying'}
                  className="font-mono text-xs leading-snug"
                />
              </div>

              {/* Actions Row */}
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  type="button"
                  size="sm"
                  variant="secondary"
                  disabled={
                    !m3uText.trim() ||
                    importPhase === 'parsing' ||
                    importPhase === 'applying'
                  }
                  onClick={handlePreview}
                >
                  {importPhase === 'parsing' && (
                    <RefreshCw className="h-4 w-4 animate-spin mr-1" />
                  )}
                  Preview
                </Button>
                <Button
                  type="button"
                  size="sm"
                  disabled={
                    !parsedChannels ||
                    parsedChannels.length === 0 ||
                    importPhase === 'parsing' ||
                    importPhase === 'applying'
                  }
                  onClick={handleApply}
                >
                  {importPhase === 'applying' && (
                    <RefreshCw className="h-4 w-4 animate-spin mr-1" />
                  )}
                  Apply
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => setImportOpen(false)}
                  disabled={importPhase === 'parsing' || importPhase === 'applying'}
                >
                  Close
                </Button>

                {parsedChannels && (
                  <Badge variant="outline" className="ml-auto">
                    Parsed: {parsedChannels.length}
                  </Badge>
                )}
                {channelDiff && (
                  <Badge variant="secondary">
                    Overlap {channelDiff.overlap} / New {channelDiff.added} / Potential Removed{' '}
                    {channelDiff.removedPotential}
                  </Badge>
                )}
              </div>

              {/* Status / Errors */}
              {importError && (
                <div className="flex items-start gap-2 rounded-md border border-destructive/50 bg-destructive/10 p-2 text-sm text-destructive">
                  <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
                  <span>{importError}</span>
                </div>
              )}

              {/* Preview Table (compact) */}
              {parsedChannels && (
                <div className="space-y-2">
                  <Separator />
                  <div className="text-sm font-medium">
                    Preview ({parsedChannels.length} channel
                    {parsedChannels.length === 1 ? '' : 's'})
                  </div>
                  <div className="border rounded-md max-h-60 overflow-auto text-xs">
                    <table className="w-full border-collapse">
                      <thead className="bg-muted sticky top-0">
                        <tr className="text-left">
                          <th className="px-2 py-1 font-medium">#</th>
                          <th className="px-2 py-1 font-medium">Name</th>
                          <th className="px-2 py-1 font-medium">URL</th>
                          <th className="px-2 py-1 font-medium">Group</th>
                          <th className="px-2 py-1 font-medium">tvg-id</th>
                          <th className="px-2 py-1 font-medium">Logo</th>
                        </tr>
                      </thead>
                      <tbody>
                        {parsedChannels.map((c, i) => (
                          <tr
                            key={i}
                            className={cn(
                              'border-t hover:bg-accent/40',
                              existingChannels &&
                                existingChannels.find(
                                  (e) =>
                                    e.channel_name.trim() === c.channel_name.trim() &&
                                    e.stream_url.trim() === c.stream_url.trim()
                                ) &&
                                'bg-amber-50 dark:bg-amber-900/20'
                            )}
                          >
                            <td className="px-2 py-1 whitespace-nowrap">
                              {c.channel_number ?? ''}
                            </td>
                            <td className="px-2 py-1 whitespace-nowrap">{c.channel_name}</td>
                            <td className="px-2 py-1 max-w-[220px] truncate">{c.stream_url}</td>
                            <td className="px-2 py-1 whitespace-nowrap">
                              {c.group_title || ''}
                            </td>
                            <td className="px-2 py-1 whitespace-nowrap">{c.tvg_id || ''}</td>
                            <td className="px-2 py-1 max-w-[140px] truncate">
                              {c.tvg_logo || ''}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                  {applySummary && (
                    <div className="mt-3 text-xs rounded-md border bg-muted/50 p-3 space-y-1">
                      <div className="flex items-center gap-1 font-medium">
                        <Check className="h-4 w-4 text-green-600" />
                        Applied Successfully
                      </div>
                      <div className="grid grid-cols-2 md:grid-cols-4 gap-1">
                        <div>
                          <span className="font-semibold">Added:</span> {applySummary.delta?.added}
                        </div>
                        <div>
                          <span className="font-semibold">Updated:</span>{' '}
                          {applySummary.delta?.updated}
                        </div>
                        <div>
                          <span className="font-semibold">Removed:</span>{' '}
                          {applySummary.delta?.removed}
                        </div>
                        <div>
                          <span className="font-semibold">Total:</span>{' '}
                          {applySummary.delta?.total_after}
                        </div>
                      </div>
                    </div>
                  )}
                </div>
              )}
            </div>
          <DialogFooter />
        </DialogContent>
      </Dialog>

      {/* Export Button */}
      <Dialog open={exportOpen} onOpenChange={setExportOpen}>
        <DialogTrigger asChild>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={disabled}
            onClick={() => {
              setExportOpen(true);
              setExportError(null);
              setExportedM3U('');
              void handleExport();
            }}
            className="gap-1"
          >
            <FileDown className="h-4 w-4" />
            Export M3U
          </Button>
        </DialogTrigger>
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>Export Manual Channels (M3U)</DialogTitle>
            <DialogDescription>
              Download or copy the current manual channel definitions as an extended M3U playlist.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4 max-h-[72vh] overflow-y-auto pr-1">
            <div className="flex flex-col gap-2">
              <label className="text-sm font-medium">File Name</label>
              <Input
                value={downloadFileName}
                onChange={(e) => setDownloadFileName(e.target.value)}
                disabled={exportLoading}
              />
            </div>

            {/* Status / Errors */}
            {exportError && (
              <div className="flex items-start gap-2 rounded-md border border-destructive/50 bg-destructive/10 p-2 text-sm text-destructive">
                <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
                <span>{exportError}</span>
              </div>
            )}

            <div className="flex flex-wrap gap-2 items-center">
              <Button
                type="button"
                size="sm"
                variant="secondary"
                disabled={exportLoading}
                onClick={() => void handleExport()}
              >
                {exportLoading && <RefreshCw className="h-4 w-4 animate-spin mr-1" />}
                Refresh
              </Button>
              <Button
                type="button"
                size="sm"
                disabled={exportLoading || !exportedM3U}
                onClick={handleDownload}
              >
                <Download className="h-4 w-4 mr-1" />
                Download
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={exportLoading || !exportedM3U}
                onClick={handleCopy}
              >
                <Clipboard className="h-4 w-4 mr-1" />
                Copy
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setExportOpen(false)}
                disabled={exportLoading}
              >
                Close
              </Button>
              {exportedM3U && (
                <Badge variant="outline" className="ml-auto">
                  {exportedM3U.split('\n').filter(Boolean).length} lines
                </Badge>
              )}
            </div>

            <div className="space-y-2">
              <label className="text-sm font-medium">M3U Output</label>
              <Textarea
                className="font-mono text-xs leading-snug h-64"
                value={exportedM3U || (exportLoading ? 'Loading...' : '')}
                readOnly
              />
            </div>
          </div>
          <DialogFooter />
        </DialogContent>
      </Dialog>
    </div>
  );
};

export default ManualM3UImportExport;
