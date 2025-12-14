'use client';

import * as React from 'react';
import { Download, Loader2 } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { ScrollArea } from '@/components/ui/scroll-area';
import { apiClient } from '@/lib/api-client';
import { ExportType, ExportRequest } from '@/types/api';

interface ExportableItem {
  id: string;
  name: string;
  is_system?: boolean;
}

interface ExportDialogProps {
  exportType: ExportType;
  items: ExportableItem[];
  title: string;
  description?: string;
  trigger?: React.ReactNode;
  onExportComplete?: () => void;
}

const EXPORT_TYPE_LABELS: Record<ExportType, string> = {
  filters: 'Filters',
  data_mapping_rules: 'Data Mapping Rules',
  client_detection_rules: 'Client Detection Rules',
  encoding_profiles: 'Encoding Profiles',
};

export function ExportDialog({
  exportType,
  items,
  title,
  description,
  trigger,
  onExportComplete,
}: ExportDialogProps) {
  const [open, setOpen] = React.useState(false);
  const [selectedIds, setSelectedIds] = React.useState<Set<string>>(new Set());
  const [isExporting, setIsExporting] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);

  // Filter out system items - only user-created items can be exported
  const exportableItems = React.useMemo(
    () => items.filter((item) => !item.is_system),
    [items]
  );

  const allSelected = exportableItems.length > 0 && selectedIds.size === exportableItems.length;
  const someSelected = selectedIds.size > 0 && selectedIds.size < exportableItems.length;

  const handleSelectAll = () => {
    if (allSelected) {
      setSelectedIds(new Set());
    } else {
      setSelectedIds(new Set(exportableItems.map((item) => item.id)));
    }
  };

  const handleToggleItem = (id: string) => {
    const newSelected = new Set(selectedIds);
    if (newSelected.has(id)) {
      newSelected.delete(id);
    } else {
      newSelected.add(id);
    }
    setSelectedIds(newSelected);
  };

  const handleExport = async () => {
    if (selectedIds.size === 0) {
      setError('Please select at least one item to export');
      return;
    }

    setIsExporting(true);
    setError(null);

    try {
      const request: ExportRequest = {
        ids: Array.from(selectedIds),
        all: selectedIds.size === exportableItems.length,
      };

      let exportData;
      switch (exportType) {
        case 'filters':
          exportData = await apiClient.exportFilters(request);
          break;
        case 'data_mapping_rules':
          exportData = await apiClient.exportDataMappingRules(request);
          break;
        case 'client_detection_rules':
          exportData = await apiClient.exportClientDetectionRules(request);
          break;
        case 'encoding_profiles':
          exportData = await apiClient.exportEncodingProfiles(request);
          break;
        default:
          throw new Error(`Unsupported export type: ${exportType}`);
      }

      // Create and download the file
      const blob = new Blob([JSON.stringify(exportData, null, 2)], {
        type: 'application/json',
      });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      const timestamp = new Date().toISOString().split('T')[0];
      link.href = url;
      link.download = `${exportType}-export-${timestamp}.json`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      URL.revokeObjectURL(url);

      setOpen(false);
      setSelectedIds(new Set());
      onExportComplete?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Export failed');
    } finally {
      setIsExporting(false);
    }
  };

  const handleOpenChange = (newOpen: boolean) => {
    setOpen(newOpen);
    if (!newOpen) {
      setError(null);
      setSelectedIds(new Set());
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger asChild>
        {trigger || (
          <Button variant="outline" size="sm">
            <Download className="mr-2 h-4 w-4" />
            Export
          </Button>
        )}
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            {description ||
              `Select the ${EXPORT_TYPE_LABELS[exportType].toLowerCase()} you want to export.`}
          </DialogDescription>
        </DialogHeader>

        {exportableItems.length === 0 ? (
          <div className="py-6 text-center text-muted-foreground">
            No exportable items available. System items cannot be exported.
          </div>
        ) : (
          <>
            <div className="flex items-center space-x-2 border-b pb-2">
              <Checkbox
                id="select-all"
                checked={allSelected}
                onCheckedChange={handleSelectAll}
                ref={(el) => {
                  if (el) {
                    (el as HTMLButtonElement).dataset.state = someSelected ? 'indeterminate' : allSelected ? 'checked' : 'unchecked';
                  }
                }}
              />
              <label
                htmlFor="select-all"
                className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70"
              >
                Select All ({exportableItems.length} items)
              </label>
            </div>

            <ScrollArea className="h-[300px] pr-4">
              <div className="space-y-2">
                {exportableItems.map((item) => (
                  <div key={item.id} className="flex items-center space-x-2">
                    <Checkbox
                      id={item.id}
                      checked={selectedIds.has(item.id)}
                      onCheckedChange={() => handleToggleItem(item.id)}
                    />
                    <label
                      htmlFor={item.id}
                      className="text-sm leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70 truncate"
                    >
                      {item.name}
                    </label>
                  </div>
                ))}
              </div>
            </ScrollArea>
          </>
        )}

        {error && (
          <div className="text-sm text-destructive">{error}</div>
        )}

        <DialogFooter>
          <Button variant="outline" onClick={() => setOpen(false)}>
            Cancel
          </Button>
          <Button
            onClick={handleExport}
            disabled={isExporting || selectedIds.size === 0}
          >
            {isExporting ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Exporting...
              </>
            ) : (
              <>
                <Download className="mr-2 h-4 w-4" />
                Export ({selectedIds.size})
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
