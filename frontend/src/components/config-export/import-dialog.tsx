'use client';

import * as React from 'react';
import { Upload, Loader2, AlertCircle, CheckCircle2, XCircle, AlertTriangle } from 'lucide-react';
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
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { ScrollArea } from '@/components/ui/scroll-area';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Badge } from '@/components/ui/badge';
import { Separator } from '@/components/ui/separator';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { apiClient } from '@/lib/api-client';
import {
  ExportType,
  ImportPreview,
  ImportResult,
  ConflictItem,
  ConflictResolution,
} from '@/types/api';

interface ImportDialogProps {
  importType: ExportType;
  title: string;
  description?: string;
  trigger?: React.ReactNode;
  onImportComplete?: () => void;
}

type ImportStep = 'select' | 'preview' | 'result';

const IMPORT_TYPE_LABELS: Record<ExportType, string> = {
  filters: 'Filters',
  data_mapping_rules: 'Data Mapping Rules',
  client_detection_rules: 'Client Detection Rules',
  encoding_profiles: 'Encoding Profiles',
};

export function ImportDialog({
  importType,
  title,
  description,
  trigger,
  onImportComplete,
}: ImportDialogProps) {
  const [open, setOpen] = React.useState(false);
  const [step, setStep] = React.useState<ImportStep>('select');
  const [file, setFile] = React.useState<File | null>(null);
  const [isLoading, setIsLoading] = React.useState(false);
  const [error, setError] = React.useState<string | null>(null);
  const [preview, setPreview] = React.useState<ImportPreview | null>(null);
  const [result, setResult] = React.useState<ImportResult | null>(null);
  const [conflicts, setConflicts] = React.useState<Record<string, ConflictResolution>>({});
  const [bulkResolution, setBulkResolution] = React.useState<ConflictResolution | null>(null);

  const fileInputRef = React.useRef<HTMLInputElement>(null);

  const handleFileSelect = (event: React.ChangeEvent<HTMLInputElement>) => {
    const selectedFile = event.target.files?.[0];
    if (selectedFile) {
      if (!selectedFile.name.endsWith('.json')) {
        setError('Please select a JSON file');
        return;
      }
      setFile(selectedFile);
      setError(null);
    }
  };

  const handlePreview = async () => {
    if (!file) {
      setError('Please select a file');
      return;
    }

    setIsLoading(true);
    setError(null);

    try {
      let previewResult: ImportPreview;
      switch (importType) {
        case 'filters':
          previewResult = await apiClient.importFiltersPreview(file);
          break;
        case 'data_mapping_rules':
          previewResult = await apiClient.importDataMappingRulesPreview(file);
          break;
        case 'client_detection_rules':
          previewResult = await apiClient.importClientDetectionRulesPreview(file);
          break;
        case 'encoding_profiles':
          previewResult = await apiClient.importEncodingProfilesPreview(file);
          break;
        default:
          throw new Error(`Unsupported import type: ${importType}`);
      }

      setPreview(previewResult);

      // Initialize conflict resolutions to 'skip' by default
      const initialConflicts: Record<string, ConflictResolution> = {};
      previewResult.conflicts.forEach((conflict) => {
        initialConflicts[conflict.import_name] = conflict.resolution || 'skip';
      });
      setConflicts(initialConflicts);

      setStep('preview');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to preview import');
    } finally {
      setIsLoading(false);
    }
  };

  const handleImport = async () => {
    if (!file) return;

    setIsLoading(true);
    setError(null);

    try {
      let importResult: ImportResult;
      switch (importType) {
        case 'filters':
          importResult = await apiClient.importFilters(file, conflicts, bulkResolution || undefined);
          break;
        case 'data_mapping_rules':
          importResult = await apiClient.importDataMappingRules(file, conflicts, bulkResolution || undefined);
          break;
        case 'client_detection_rules':
          importResult = await apiClient.importClientDetectionRules(file, conflicts, bulkResolution || undefined);
          break;
        case 'encoding_profiles':
          importResult = await apiClient.importEncodingProfiles(file, conflicts, bulkResolution || undefined);
          break;
        default:
          throw new Error(`Unsupported import type: ${importType}`);
      }

      setResult(importResult);
      setStep('result');
      onImportComplete?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Import failed');
    } finally {
      setIsLoading(false);
    }
  };

  const handleConflictResolution = (name: string, resolution: ConflictResolution) => {
    setConflicts((prev) => ({ ...prev, [name]: resolution }));
    setBulkResolution(null); // Clear bulk resolution when individual is changed
  };

  const handleBulkResolution = (resolution: ConflictResolution) => {
    setBulkResolution(resolution);
    if (preview) {
      const bulkConflicts: Record<string, ConflictResolution> = {};
      preview.conflicts.forEach((conflict) => {
        bulkConflicts[conflict.import_name] = resolution;
      });
      setConflicts(bulkConflicts);
    }
  };

  const handleOpenChange = (newOpen: boolean) => {
    setOpen(newOpen);
    if (!newOpen) {
      // Reset state when dialog closes
      setStep('select');
      setFile(null);
      setError(null);
      setPreview(null);
      setResult(null);
      setConflicts({});
      setBulkResolution(null);
      if (fileInputRef.current) {
        fileInputRef.current.value = '';
      }
    }
  };

  const renderSelectStep = () => (
    <>
      <div className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="import-file">Select Export File</Label>
          <Input
            id="import-file"
            ref={fileInputRef}
            type="file"
            accept=".json"
            onChange={handleFileSelect}
          />
          {file && (
            <p className="text-sm text-muted-foreground">
              Selected: {file.name} ({(file.size / 1024).toFixed(1)} KB)
            </p>
          )}
        </div>
      </div>

      <DialogFooter>
        <Button variant="outline" onClick={() => setOpen(false)}>
          Cancel
        </Button>
        <Button onClick={handlePreview} disabled={!file || isLoading}>
          {isLoading ? (
            <>
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              Loading...
            </>
          ) : (
            'Preview Import'
          )}
        </Button>
      </DialogFooter>
    </>
  );

  const renderPreviewStep = () => {
    if (!preview) return null;

    const hasConflicts = preview.conflicts.length > 0;
    const hasErrors = preview.errors.length > 0;

    return (
      <>
        <div className="space-y-4">
          {/* Summary */}
          <div className="flex gap-4 text-sm">
            <div className="flex items-center gap-1">
              <CheckCircle2 className="h-4 w-4 text-green-500" />
              <span>{preview.new_items.length} new</span>
            </div>
            <div className="flex items-center gap-1">
              <AlertCircle className="h-4 w-4 text-yellow-500" />
              <span>{preview.conflicts.length} conflicts</span>
            </div>
            {hasErrors && (
              <div className="flex items-center gap-1">
                <XCircle className="h-4 w-4 text-red-500" />
                <span>{preview.errors.length} errors</span>
              </div>
            )}
          </div>

          {/* Version Warning */}
          {preview.version_warning && (
            <Alert variant="destructive" className="border-yellow-500 bg-yellow-50 dark:bg-yellow-900/20">
              <AlertTriangle className="h-4 w-4 text-yellow-600" />
              <AlertTitle className="text-yellow-800 dark:text-yellow-200">Version Mismatch</AlertTitle>
              <AlertDescription className="text-yellow-700 dark:text-yellow-300 text-sm">
                {preview.version_warning}
              </AlertDescription>
            </Alert>
          )}

          <ScrollArea className="h-[300px] pr-4">
            {/* New Items */}
            {preview.new_items.length > 0 && (
              <div className="mb-4">
                <h4 className="text-sm font-medium mb-2">New Items</h4>
                <div className="space-y-1">
                  {preview.new_items.map((item, index) => (
                    <div key={index} className="flex items-center gap-2 text-sm">
                      <CheckCircle2 className="h-3 w-3 text-green-500" />
                      <span>{item.name}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Conflicts */}
            {hasConflicts && (
              <div className="mb-4">
                <div className="flex items-center justify-between mb-2">
                  <h4 className="text-sm font-medium">Conflicts</h4>
                  <div className="flex gap-1">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleBulkResolution('skip')}
                      className="h-6 text-xs"
                    >
                      Skip All
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleBulkResolution('rename')}
                      className="h-6 text-xs"
                    >
                      Rename All
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleBulkResolution('overwrite')}
                      className="h-6 text-xs"
                    >
                      Overwrite All
                    </Button>
                  </div>
                </div>
                <div className="space-y-2">
                  {preview.conflicts.map((conflict) => (
                    <ConflictRow
                      key={conflict.import_name}
                      conflict={conflict}
                      resolution={conflicts[conflict.import_name] || 'skip'}
                      onResolutionChange={(resolution) =>
                        handleConflictResolution(conflict.import_name, resolution)
                      }
                    />
                  ))}
                </div>
              </div>
            )}

            {/* Errors */}
            {hasErrors && (
              <div>
                <h4 className="text-sm font-medium mb-2 text-destructive">Errors</h4>
                <div className="space-y-1">
                  {preview.errors.map((err, index) => (
                    <div key={index} className="flex items-start gap-2 text-sm">
                      <XCircle className="h-3 w-3 text-destructive mt-0.5" />
                      <div>
                        <span className="font-medium">{err.item_name}:</span>{' '}
                        <span className="text-muted-foreground">{err.error}</span>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </ScrollArea>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => setStep('select')}>
            Back
          </Button>
          <Button
            onClick={handleImport}
            disabled={isLoading || (preview.new_items.length === 0 && Object.values(conflicts).every((r) => r === 'skip'))}
          >
            {isLoading ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Importing...
              </>
            ) : (
              <>
                <Upload className="mr-2 h-4 w-4" />
                Import
              </>
            )}
          </Button>
        </DialogFooter>
      </>
    );
  };

  const renderResultStep = () => {
    if (!result) return null;

    return (
      <>
        <div className="space-y-4">
          <div className="flex flex-wrap gap-2">
            <Badge variant="default">{result.imported} imported</Badge>
            <Badge variant="secondary">{result.skipped} skipped</Badge>
            <Badge variant="outline">{result.overwritten} overwritten</Badge>
            <Badge variant="outline">{result.renamed} renamed</Badge>
            {result.errors > 0 && (
              <Badge variant="destructive">{result.errors} errors</Badge>
            )}
          </div>

          <ScrollArea className="h-[250px] pr-4">
            {result.imported_items && result.imported_items.length > 0 && (
              <div className="mb-4">
                <h4 className="text-sm font-medium mb-2">Imported Items</h4>
                <div className="space-y-1">
                  {result.imported_items.map((item, index) => (
                    <div key={index} className="flex items-center gap-2 text-sm">
                      <CheckCircle2 className="h-3 w-3 text-green-500" />
                      <span>
                        {item.original_name}
                        {item.original_name !== item.final_name && (
                          <span className="text-muted-foreground">
                            {' '}-&gt; {item.final_name}
                          </span>
                        )}
                      </span>
                      <Badge variant="outline" className="text-xs">
                        {item.action}
                      </Badge>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {result.error_details && result.error_details.length > 0 && (
              <div>
                <h4 className="text-sm font-medium mb-2 text-destructive">Errors</h4>
                <div className="space-y-1">
                  {result.error_details.map((err, index) => (
                    <div key={index} className="flex items-start gap-2 text-sm">
                      <XCircle className="h-3 w-3 text-destructive mt-0.5" />
                      <div>
                        <span className="font-medium">{err.item_name}:</span>{' '}
                        <span className="text-muted-foreground">{err.error}</span>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </ScrollArea>
        </div>

        <DialogFooter>
          <Button onClick={() => setOpen(false)}>Done</Button>
        </DialogFooter>
      </>
    );
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger asChild>
        {trigger || (
          <Button variant="outline" size="sm">
            <Upload className="mr-2 h-4 w-4" />
            Import
          </Button>
        )}
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {step === 'select' && title}
            {step === 'preview' && `Preview Import - ${IMPORT_TYPE_LABELS[importType]}`}
            {step === 'result' && 'Import Complete'}
          </DialogTitle>
          <DialogDescription>
            {step === 'select' &&
              (description ||
                `Import ${IMPORT_TYPE_LABELS[importType].toLowerCase()} from a previously exported JSON file.`)}
            {step === 'preview' && 'Review the items to be imported and resolve any conflicts.'}
            {step === 'result' && 'The import has been completed.'}
          </DialogDescription>
        </DialogHeader>

        {error && (
          <div className="flex items-center gap-2 text-sm text-destructive">
            <AlertCircle className="h-4 w-4" />
            {error}
          </div>
        )}

        {step === 'select' && renderSelectStep()}
        {step === 'preview' && renderPreviewStep()}
        {step === 'result' && renderResultStep()}
      </DialogContent>
    </Dialog>
  );
}

interface ConflictRowProps {
  conflict: ConflictItem;
  resolution: ConflictResolution;
  onResolutionChange: (resolution: ConflictResolution) => void;
}

function ConflictRow({ conflict, resolution, onResolutionChange }: ConflictRowProps) {
  return (
    <div className="flex items-center justify-between gap-2 p-2 rounded-md border bg-muted/50">
      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium truncate">{conflict.import_name}</p>
        <p className="text-xs text-muted-foreground">
          Conflicts with existing: {conflict.existing_name}
        </p>
      </div>
      <Select value={resolution} onValueChange={(v) => onResolutionChange(v as ConflictResolution)}>
        <SelectTrigger className="w-[120px]">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="skip">Skip</SelectItem>
          <SelectItem value="rename">Rename</SelectItem>
          <SelectItem value="overwrite">Overwrite</SelectItem>
        </SelectContent>
      </Select>
    </div>
  );
}
