'use client';

import { useState, useEffect, useCallback, useMemo, useRef } from 'react';
import Fuse from 'fuse.js';
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
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet';
import { Skeleton } from '@/components/ui/skeleton';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import {
  Plus,
  Search,
  Trash2,
  Edit,
  Upload,
  Image as ImageIcon,
  HardDrive,
  Database,
  Link2,
  AlertCircle,
  Loader2,
  WifiOff,
  Eye,
  Download,
  FileImage,
  RefreshCw,
} from 'lucide-react';
import {
  LogoAsset,
  LogoAssetsResponse,
  LogoStats,
  LogoUploadRequest,
  LogoAssetUpdateRequest,
} from '@/types/api';
import { apiClient, ApiError } from '@/lib/api-client';
import { API_CONFIG } from '@/lib/config';
import { StatCard } from '@/components/shared/feedback/StatCard';
import { formatRelativeTimeShort } from '@/lib/format';

interface LoadingState {
  logos: boolean;
  stats: boolean;
  upload: boolean;
  edit: boolean;
  delete: string | null;
  rescan: boolean;
  clear: boolean;
}

interface ErrorState {
  logos: string | null;
  stats: string | null;
  upload: string | null;
  edit: string | null;
  action: string | null;
  rescan: string | null;
  clear: string | null;
}

function formatFileSize(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

function formatDate(dateString: string): string {
  return new Date(dateString).toLocaleString();
}

function getAssetTypeColor(assetType: string): string {
  switch (assetType) {
    case 'uploaded':
      return 'bg-blue-100 text-blue-800';
    case 'cached':
      return 'bg-green-100 text-green-800';
    default:
      return 'bg-gray-100 text-gray-800';
  }
}

function getFormatFromMimeType(mimeType: string): string {
  const formats = {
    'image/png': 'PNG',
    'image/jpeg': 'JPG',
    'image/jpg': 'JPG',
    'image/gif': 'GIF',
    'image/svg+xml': 'SVG',
    'image/webp': 'WEBP',
  };
  return (
    formats[mimeType as keyof typeof formats] || mimeType.split('/')[1]?.toUpperCase() || 'IMG'
  );
}

function UploadLogoSheet({
  onUploadLogo,
  loading,
  error,
  open,
  onOpenChange,
}: {
  onUploadLogo: (data: LogoUploadRequest) => Promise<void>;
  loading: boolean;
  error: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [formData, setFormData] = useState<{
    name: string;
    description: string;
    file: File | null;
  }>({
    name: '',
    description: '',
    file: null,
  });

  const [isDragOver, setIsDragOver] = useState(false);
  const [showFileError, setShowFileError] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Reset drag state and form when sheet opens/closes
  useEffect(() => {
    if (!open) {
      setIsDragOver(false);
      setShowFileError(false);
      setFormData({
        name: '',
        description: '',
        file: null,
      });
    } else {
      setIsDragOver(false); // Also reset when opening
      setShowFileError(false); // Reset error state when opening
    }
  }, [open]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!formData.file) {
      setShowFileError(true);
      return;
    }
    setShowFileError(false);

    await onUploadLogo({
      name: formData.name,
      description: formData.description || undefined,
      file: formData.file,
    });

    if (!error) {
      onOpenChange(false);
      setFormData({
        name: '',
        description: '',
        file: null,
      });
    }
  };

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      setFormData((prev) => ({
        ...prev,
        file,
        name: prev.name || file.name.replace(/\.[^/.]+$/, ''), // Remove extension for name
      }));
      setShowFileError(false); // Clear error when file is selected
    }
  };

  const isValidImageFile = (file: File) => {
    return file.type.startsWith('image/');
  };

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (!isDragOver) {
      setIsDragOver(true);
    }
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    // Only set to false if we're leaving the drop zone entirely
    const relatedTarget = e.relatedTarget as Node;
    if (!relatedTarget || !e.currentTarget.contains(relatedTarget)) {
      setIsDragOver(false);
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragOver(false);

    const files = Array.from(e.dataTransfer.files);
    const imageFile = files.find(isValidImageFile);

    if (imageFile) {
      setFormData((prev) => ({
        ...prev,
        file: imageFile,
        name: prev.name || imageFile.name.replace(/\.[^/.]+$/, ''), // Remove extension for name
      }));
      setShowFileError(false); // Clear error when file is dropped
    }
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="w-full sm:max-w-lg overflow-y-auto"
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
      >
        <SheetHeader>
          <SheetTitle>Upload Logo</SheetTitle>
          <SheetDescription>
            Upload a new logo asset to the system. You can drag and drop image files directly onto
            this dialog.
          </SheetDescription>
        </SheetHeader>

        {error && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {isDragOver && (
          <div className="fixed inset-0 bg-primary/10 flex items-center justify-center z-[60] pointer-events-none">
            <div className="bg-background border border-border rounded-lg p-6 shadow-lg">
              <div className="text-center">
                <Upload className="h-12 w-12 mx-auto mb-2 text-primary" />
                <p className="text-lg font-semibold text-primary">Drop image file here</p>
                <p className="text-sm text-muted-foreground">
                  Supports JPG, PNG, WebP, and other image formats
                </p>
              </div>
            </div>
          </div>
        )}

        <form id="upload-logo-form" onSubmit={handleSubmit} className="space-y-4 px-4">
          <div className="space-y-2">
            <Label>Logo File</Label>
            <div
              className={`relative border-2 border-dashed rounded-lg p-4 text-center transition-colors cursor-pointer ${
                showFileError
                  ? 'border-destructive bg-destructive/5'
                  : 'border-muted-foreground/30 hover:border-muted-foreground/50'
              }`}
            >
              <input
                ref={fileInputRef}
                type="file"
                accept="image/*"
                onChange={handleFileChange}
                disabled={loading}
                className="absolute inset-0 w-full h-full opacity-0 cursor-pointer z-10"
              />
              <Upload className="h-8 w-8 mx-auto mb-2 text-muted-foreground pointer-events-none" />
              <p className="text-sm font-medium pointer-events-none">Click to browse files</p>
              <p className="text-xs text-muted-foreground mt-1 pointer-events-none">
                Or drag and drop an image anywhere on this dialog
              </p>
            </div>
            {showFileError && (
              <p className="text-sm text-destructive">Please select an image file to upload.</p>
            )}
            {formData.file && (
              <div className="text-sm text-muted-foreground bg-muted p-2 rounded flex items-center gap-2">
                <FileImage className="h-4 w-4" />
                <span>
                  {formData.file.name} ({formatFileSize(formData.file.size)})
                </span>
              </div>
            )}
          </div>

          <div className="space-y-2">
            <Label htmlFor="name">Name</Label>
            <Input
              id="name"
              value={formData.name}
              onChange={(e) => setFormData({ ...formData, name: e.target.value })}
              placeholder="Logo name"
              required
              disabled={loading}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="description">Description (Optional)</Label>
            <Input
              id="description"
              value={formData.description}
              onChange={(e) => setFormData({ ...formData, description: e.target.value })}
              placeholder="Logo description"
              disabled={loading}
            />
          </div>
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
          <Button form="upload-logo-form" type="submit" disabled={loading || !formData.file}>
            {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Upload Logo
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}

function EditLogoSheet({
  logo,
  onUpdateLogo,
  onReplaceLogo,
  loading,
  error,
  open,
  onOpenChange,
}: {
  logo: LogoAsset | null;
  onUpdateLogo: (id: string, data: LogoAssetUpdateRequest) => Promise<void>;
  onReplaceLogo: (id: string, file: File, name: string, description?: string) => Promise<void>;
  loading: boolean;
  error: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [formData, setFormData] = useState<{
    name: string;
    description: string;
  }>({
    name: '',
    description: '',
  });
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [isDragOver, setIsDragOver] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Reset form data when logo changes (not when previewUrl changes)
  useEffect(() => {
    if (logo) {
      setFormData({
        name: logo.name,
        description: logo.description || '',
      });
      // Only reset file state when switching to a different logo
      setSelectedFile(null);
      setPreviewUrl((prev) => {
        if (prev) URL.revokeObjectURL(prev);
        return null;
      });
      setIsDragOver(false);
      // Reset file input
      if (fileInputRef.current) {
        fileInputRef.current.value = '';
      }
    }
  }, [logo?.id]); // Only depend on logo.id, not the entire logo object or previewUrl

  // Cleanup preview URL when component unmounts
  useEffect(() => {
    return () => {
      if (previewUrl) {
        URL.revokeObjectURL(previewUrl);
      }
    };
  }, []); // Empty dependency - only cleanup on unmount

  // Handle file selection
  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      // Clean up previous preview URL
      if (previewUrl) {
        URL.revokeObjectURL(previewUrl);
      }
      setSelectedFile(file);
      const url = URL.createObjectURL(file);
      setPreviewUrl(url);
    }
  };

  const isValidImageFile = (file: File) => {
    return file.type.startsWith('image/');
  };

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (!isDragOver) {
      setIsDragOver(true);
    }
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    // Only set to false if we're leaving the drop zone entirely
    const relatedTarget = e.relatedTarget as Node;
    if (!relatedTarget || !e.currentTarget.contains(relatedTarget)) {
      setIsDragOver(false);
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsDragOver(false);

    const files = Array.from(e.dataTransfer.files);
    const imageFile = files.find(isValidImageFile);

    if (imageFile) {
      // Clean up previous preview URL
      if (previewUrl) {
        URL.revokeObjectURL(previewUrl);
      }
      setSelectedFile(imageFile);
      const url = URL.createObjectURL(imageFile);
      setPreviewUrl(url);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!logo) return;

    try {
      if (selectedFile) {
        // Replace image and update metadata via parent callback
        await onReplaceLogo(
          logo.id,
          selectedFile,
          formData.name,
          formData.description || undefined
        );
      } else {
        // Just update metadata
        await onUpdateLogo(logo.id, {
          name: formData.name,
          description: formData.description || undefined,
        });
      }

      // Close sheet on success (error state will prevent this in the parent)
      onOpenChange(false);
    } catch (err) {
      console.error('Failed to update logo:', err);
    }
  };

  if (!logo) return null;

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="w-full sm:max-w-lg overflow-y-auto"
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
      >
        <SheetHeader>
          <SheetTitle>Edit Logo</SheetTitle>
          <SheetDescription>
            Update the logo information and metadata. You can drag and drop image files directly
            onto this dialog to replace the image.
          </SheetDescription>
        </SheetHeader>

        {error && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {isDragOver && (
          <div className="fixed inset-0 bg-primary/10 flex items-center justify-center z-[60] pointer-events-none">
            <div className="bg-background border border-border rounded-lg p-6 shadow-lg">
              <div className="text-center">
                <Upload className="h-12 w-12 mx-auto mb-2 text-primary" />
                <p className="text-lg font-semibold text-primary">Drop image file to replace</p>
                <p className="text-sm text-muted-foreground">
                  Supports JPG, PNG, WebP, and other image formats
                </p>
              </div>
            </div>
          </div>
        )}

        <form id="edit-logo-form" onSubmit={handleSubmit} className="space-y-4 px-4">
          {/* Logo preview at top */}
          <div className="space-y-2">
            <Label>Preview</Label>
            <div className="flex items-start gap-4">
              <div className="w-24 h-24 bg-muted rounded-md flex items-center justify-center overflow-hidden flex-shrink-0">
                <img
                  src={
                    previewUrl ||
                    (logo.url.startsWith('http') ? logo.url : `${API_CONFIG.baseUrl}${logo.url}`)
                  }
                  alt={logo.name}
                  className="max-w-full max-h-full object-contain"
                  onError={(e) => {
                    e.currentTarget.style.display = 'none';
                    const nextElement = e.currentTarget.nextElementSibling as HTMLElement;
                    if (nextElement) {
                      nextElement.style.display = 'flex';
                    }
                  }}
                />
                <div
                  className="w-full h-full flex items-center justify-center text-muted-foreground"
                  style={{ display: 'none' }}
                >
                  <FileImage className="h-8 w-8" />
                </div>
              </div>
              <div className="flex-1 text-sm space-y-1">
                {selectedFile ? (
                  <>
                    <p className="font-medium text-primary">New file selected:</p>
                    <p>{selectedFile.name}</p>
                    <p className="text-muted-foreground">{formatFileSize(selectedFile.size)}</p>
                  </>
                ) : (
                  <>
                    <p className="font-medium">{logo.file_name}</p>
                    <div className="flex gap-2">
                      <Badge variant="outline">{getFormatFromMimeType(logo.mime_type)}</Badge>
                      <Badge variant="secondary">{formatFileSize(logo.file_size)}</Badge>
                    </div>
                    {logo.width && logo.height && (
                      <p className="text-muted-foreground">
                        {logo.width} x {logo.height} px
                      </p>
                    )}
                  </>
                )}
              </div>
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="edit-name">Name</Label>
            <Input
              id="edit-name"
              value={formData.name}
              onChange={(e) => setFormData({ ...formData, name: e.target.value })}
              placeholder="Logo name"
              required
              disabled={loading}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="edit-description">Description (optional)</Label>
            <Input
              id="edit-description"
              value={formData.description}
              onChange={(e) => setFormData({ ...formData, description: e.target.value })}
              placeholder="Logo description"
              disabled={loading}
            />
          </div>

          {/* File replacement */}
          <div className="space-y-2">
            <Label>Replace Image (optional)</Label>
            <div className="relative border-2 border-dashed rounded-lg p-4 text-center transition-colors cursor-pointer border-muted-foreground/30 hover:border-muted-foreground/50">
              <input
                ref={fileInputRef}
                type="file"
                accept="image/*"
                onChange={handleFileChange}
                disabled={loading}
                className="absolute inset-0 w-full h-full opacity-0 cursor-pointer z-10"
              />
              <Upload className="h-8 w-8 mx-auto mb-2 text-muted-foreground pointer-events-none" />
              <p className="text-sm font-medium pointer-events-none">Click to browse files</p>
              <p className="text-xs text-muted-foreground mt-1 pointer-events-none">
                Or drag and drop an image anywhere on this dialog
              </p>
            </div>
            {selectedFile && (
              <div className="flex items-center justify-between gap-2 bg-muted p-2 rounded">
                <div className="flex items-center gap-2">
                  <FileImage className="h-4 w-4" />
                  <span className="text-sm truncate">{selectedFile.name}</span>
                  <Badge variant="default" className="bg-primary">
                    {getFormatFromMimeType(selectedFile.type)}
                  </Badge>
                  <span className="text-sm text-muted-foreground">
                    {formatFileSize(selectedFile.size)}
                  </span>
                </div>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-6 px-2 text-xs"
                  onClick={() => {
                    setSelectedFile(null);
                    if (previewUrl) URL.revokeObjectURL(previewUrl);
                    setPreviewUrl(null);
                    if (fileInputRef.current) fileInputRef.current.value = '';
                  }}
                >
                  Clear
                </Button>
              </div>
            )}
          </div>

          {/* Metadata section */}
          <div className="space-y-2">
            <Label>Metadata</Label>
            <div className="bg-muted p-3 rounded-md space-y-2 text-sm">
              <div className="flex justify-between">
                <span className="text-muted-foreground">ID:</span>
                <code className="text-xs bg-background px-1 rounded">{logo.id}</code>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Type:</span>
                <Badge variant="secondary">
                  {logo.asset_type === 'cached' ? 'Cached' : 'Uploaded'}
                </Badge>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Format:</span>
                <span>{getFormatFromMimeType(logo.mime_type)}</span>
              </div>
              {logo.original_mime_type && (
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Original Format:</span>
                  <span>{getFormatFromMimeType(logo.original_mime_type)}</span>
                </div>
              )}
              {logo.width && logo.height && (
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Dimensions:</span>
                  <span>
                    {logo.width} x {logo.height} px
                  </span>
                </div>
              )}
              <div className="flex justify-between">
                <span className="text-muted-foreground">File Size:</span>
                <span>{formatFileSize(logo.file_size)}</span>
              </div>
              {logo.original_file_size && (
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Original Size:</span>
                  <span>{formatFileSize(logo.original_file_size)}</span>
                </div>
              )}
              <div className="flex justify-between">
                <span className="text-muted-foreground">Created:</span>
                <span>{formatDate(logo.created_at)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Updated:</span>
                <span>{formatDate(logo.updated_at)}</span>
              </div>
              {logo.source_url && (
                <div className="pt-2 border-t">
                  <span className="text-muted-foreground block mb-1">Source URL:</span>
                  <code className="text-xs bg-background px-1 rounded break-all block">
                    {logo.source_url}
                  </code>
                </div>
              )}
            </div>
          </div>

          {/* Linked Assets Info */}
          {logo.linked_assets && logo.linked_assets.length > 0 && (
            <div className="space-y-2">
              <Label>Linked Assets ({logo.linked_assets.length})</Label>
              <div className="bg-muted p-3 rounded-md space-y-2">
                {logo.linked_assets.map((asset, idx) => (
                  <div key={idx} className="flex items-center justify-between text-sm">
                    <div className="flex items-center gap-2">
                      <Badge variant="outline" className="text-xs capitalize">
                        {asset.type}
                      </Badge>
                      <Badge variant="secondary" className="text-xs">
                        {getFormatFromMimeType(asset.content_type)}
                      </Badge>
                    </div>
                    <span className="text-muted-foreground">{formatFileSize(asset.size)}</span>
                  </div>
                ))}
                <div className="flex justify-between text-sm font-medium border-t pt-2 mt-2">
                  <span>Total Storage:</span>
                  <span>{formatFileSize(logo.total_linked_size || 0)}</span>
                </div>
              </div>
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
          <Button form="edit-logo-form" type="submit" disabled={loading}>
            {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            {selectedFile ? 'Replace Image' : 'Update Logo'}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}

export function Logos() {
  const [allLogos, setAllLogos] = useState<LogoAsset[]>([]);
  const [stats, setStats] = useState<LogoStats | null>(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [logoFilter, setLogoFilter] = useState<'all' | 'uploaded' | 'cached'>('all');
  const [currentPage, setCurrentPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [totalCount, setTotalCount] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [isOnline, setIsOnline] = useState(true);
  const [isUploadSheetOpen, setIsUploadSheetOpen] = useState(false);
  const [selectedLogo, setSelectedLogo] = useState<LogoAsset | null>(null);
  const [isInitialLoad, setIsInitialLoad] = useState(true); // Track initial load

  // Ref for infinite scroll trigger
  const loadMoreRef = useRef<HTMLDivElement>(null);

  // Ref for search input to maintain focus during debounced searches
  const searchInputRef = useRef<HTMLInputElement>(null);

  const [loading, setLoading] = useState<LoadingState>({
    logos: false,
    stats: false,
    upload: false,
    edit: false,
    delete: null,
    rescan: false,
    clear: false,
  });

  const [errors, setErrors] = useState<ErrorState>({
    logos: null,
    stats: null,
    upload: null,
    edit: null,
    action: null,
    rescan: null,
    clear: null,
  });

  // Debounced search term for API calls
  const [debouncedSearchTerm, setDebouncedSearchTerm] = useState('');
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearchTerm(searchTerm);
    }, 300);
    return () => clearTimeout(timer);
  }, [searchTerm]);

  // Track if user is actively typing to maintain focus
  const [isTyping, setIsTyping] = useState(false);

  // Track typing state
  useEffect(() => {
    if (searchTerm !== debouncedSearchTerm) {
      setIsTyping(true);
    } else {
      setIsTyping(false);
    }
  }, [searchTerm, debouncedSearchTerm]);

  // Maintain focus on search input when loading completes during typing
  useEffect(() => {
    if (
      !loading.logos &&
      isTyping &&
      searchInputRef.current &&
      document.activeElement !== searchInputRef.current
    ) {
      // Restore focus and cursor position after API call completes
      const cursorPosition = searchInputRef.current.selectionStart;
      searchInputRef.current.focus();
      searchInputRef.current.setSelectionRange(cursorPosition || 0, cursorPosition || 0);
    }
  }, [loading.logos, isTyping]);

  // Create Fuse.js instance for fuzzy search
  const logoFuse = useMemo(() => {
    if (allLogos.length === 0) return null;
    return new Fuse(allLogos, {
      keys: [
        { name: 'name', weight: 0.4 },
        { name: 'description', weight: 0.2 },
        { name: 'file_name', weight: 0.15 },
        { name: 'asset_type', weight: 0.1 },
        { name: 'source_url', weight: 0.15 },
      ],
      threshold: 0.4,
      distance: 100,
      includeScore: true,
      ignoreLocation: true,
      minMatchCharLength: 2,
    });
  }, [allLogos]);

  // Client-side filtering with Fuse.js fuzzy search
  const filteredLogos = useMemo(() => {
    let filtered = allLogos;

    // Filter by logo type first
    if (logoFilter === 'uploaded') {
      filtered = filtered.filter((logo) => logo.asset_type !== 'cached');
    } else if (logoFilter === 'cached') {
      filtered = filtered.filter((logo) => logo.asset_type === 'cached');
    }
    // logoFilter === 'all' shows everything (no additional filtering)

    // Apply fuzzy search if search term is provided
    if (searchTerm.trim()) {
      if (searchTerm.length >= 2 && logoFuse) {
        // Use Fuse.js for fuzzy matching (typo tolerant)
        const results = logoFuse.search(searchTerm);
        const matchedIds = new Set(results.map((r) => r.item.id));
        // Filter the already-filtered list to maintain type filter
        filtered = filtered.filter((logo) => matchedIds.has(logo.id));
      } else {
        // Fall back to simple includes for very short queries
        const searchLower = searchTerm.toLowerCase();
        filtered = filtered.filter((logo) => {
          const searchableText = [
            logo.name.toLowerCase(),
            logo.description?.toLowerCase() || '',
            logo.file_name.toLowerCase(),
            logo.asset_type.toLowerCase(),
            getFormatFromMimeType(logo.mime_type).toLowerCase(),
            logo.source_url?.toLowerCase() || '',
            formatFileSize(logo.file_size).toLowerCase(),
          ];
          return searchableText.some((text) => text.includes(searchLower));
        });
      }
    }

    // Sort: uploaded logos first, then cached, each sorted alphabetically by name
    // Spread to create a new array to avoid mutating state
    return [...filtered].sort((a, b) => {
      // Uploaded logos come first (asset_type === 'uploaded')
      const aIsUploaded = a.asset_type === 'uploaded';
      const bIsUploaded = b.asset_type === 'uploaded';
      if (aIsUploaded && !bIsUploaded) return -1;
      if (!aIsUploaded && bIsUploaded) return 1;
      // Within same type, sort alphabetically by name
      return a.name.localeCompare(b.name, undefined, { numeric: true });
    });
  }, [allLogos, searchTerm, logoFilter, logoFuse]);

  const loadStats = useCallback(async () => {
    if (!isOnline) return;

    setLoading((prev) => ({ ...prev, stats: true }));
    setErrors((prev) => ({ ...prev, stats: null }));

    try {
      const response = await apiClient.getLogoStats();
      setStats(response);
      setIsOnline(true);
    } catch (error) {
      const apiError = error as ApiError;
      if (apiError.status === 0) {
        setIsOnline(false);
        setErrors((prev) => ({
          ...prev,
          stats: `Unable to connect to the API service. Please check that the service is running at ${API_CONFIG.baseUrl}.`,
        }));
      } else {
        setErrors((prev) => ({
          ...prev,
          stats: `Failed to load logo stats: ${apiError.message}`,
        }));
      }
    } finally {
      setLoading((prev) => ({ ...prev, stats: false }));
    }
  }, [isOnline]);

  const loadLogos = useCallback(
    async (page: number = 1, append: boolean = false, searchTerm?: string, filter?: string) => {
      if (!isOnline) return;

      setLoading((prev) => ({ ...prev, logos: true }));
      setErrors((prev) => ({ ...prev, logos: null }));

      try {
        const response = await apiClient.getLogos({
          page,
          limit: 50, // Load more items per page for better UX
          include_cached: (filter || logoFilter) !== 'uploaded', // Include cached unless filtering for uploaded only
          search: searchTerm || undefined,
        });

        if (append) {
          setAllLogos((prev) => {
            // Deduplicate by ID
            const existing = new Set(prev.map((logo) => logo.id));
            const newLogos = response.assets.filter((logo) => !existing.has(logo.id));
            return [...prev, ...newLogos];
          });
        } else {
          setAllLogos(response.assets);
        }

        setCurrentPage(response.page);
        setTotalPages(response.total_pages);
        setTotalCount(response.total_count);
        setHasMore(response.page < response.total_pages);
        setIsOnline(true);

        // Mark initial load as complete after first successful load
        if (isInitialLoad) {
          setIsInitialLoad(false);
        }
      } catch (error) {
        const apiError = error as ApiError;
        if (apiError.status === 0) {
          setIsOnline(false);
          setErrors((prev) => ({
            ...prev,
            logos: `Unable to connect to the API service. Please check that the service is running at ${API_CONFIG.baseUrl}.`,
          }));
        } else {
          setErrors((prev) => ({
            ...prev,
            logos: `Failed to load logos: ${apiError.message}`,
          }));
        }
        // Mark initial load as complete even on error
        if (isInitialLoad) {
          setIsInitialLoad(false);
        }
      } finally {
        setLoading((prev) => ({ ...prev, logos: false }));
      }
    },
    [isOnline, logoFilter, isInitialLoad]
  );

  // Load initial data
  useEffect(() => {
    loadStats();
  }, [loadStats]);

  // Handle initial load
  useEffect(() => {
    if (isInitialLoad) {
      loadLogos(1, false);
      setCurrentPage(1);
    }
  }, [loadLogos, isInitialLoad]);

  // Handle search and filter changes
  useEffect(() => {
    if (!isInitialLoad) {
      loadLogos(1, false, debouncedSearchTerm, logoFilter);
      setCurrentPage(1);
    }
  }, [debouncedSearchTerm, logoFilter]);

  const handleLoadMore = useCallback(() => {
    if (hasMore && !loading.logos) {
      loadLogos(currentPage + 1, true, debouncedSearchTerm, logoFilter);
    }
  }, [hasMore, loading.logos, currentPage, loadLogos, debouncedSearchTerm, logoFilter]);

  // Infinite scroll effect
  useEffect(() => {
    const loadMoreElement = loadMoreRef.current;
    if (!loadMoreElement) return;

    const observer = new IntersectionObserver(
      (entries) => {
        const [entry] = entries;
        // Trigger load more when the element comes into view and we have more data
        if (entry.isIntersecting && hasMore && !loading.logos && !searchTerm) {
          console.log('[Logos] Loading more items via infinite scroll');
          handleLoadMore();
        }
      },
      {
        // Trigger when the element is 200px away from being visible
        rootMargin: '200px',
        threshold: 0.1,
      }
    );

    observer.observe(loadMoreElement);

    return () => {
      observer.unobserve(loadMoreElement);
    };
  }, [hasMore, loading.logos, searchTerm, handleLoadMore]);

  const handleUploadLogo = async (data: LogoUploadRequest) => {
    setLoading((prev) => ({ ...prev, upload: true }));
    setErrors((prev) => ({ ...prev, upload: null }));

    try {
      await apiClient.uploadLogo(data);
      await loadLogos(1, false); // Reload first page
      await loadStats(); // Update stats
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        upload: `Failed to upload logo: ${apiError.message}`,
      }));
      throw error; // Re-throw to prevent dialog from closing
    } finally {
      setLoading((prev) => ({ ...prev, upload: false }));
    }
  };

  const handleUpdateLogo = async (id: string, data: LogoAssetUpdateRequest) => {
    setLoading((prev) => ({ ...prev, edit: true }));
    setErrors((prev) => ({ ...prev, edit: null }));

    try {
      await apiClient.updateLogo(id, data);
      await loadLogos(1, false); // Reload first page
      await loadStats(); // Update stats
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        edit: `Failed to update logo: ${apiError.message}`,
      }));
      throw error; // Re-throw to prevent dialog from closing
    } finally {
      setLoading((prev) => ({ ...prev, edit: false }));
    }
  };

  const handleReplaceLogo = async (
    id: string,
    file: File,
    name: string,
    description?: string
  ) => {
    setLoading((prev) => ({ ...prev, edit: true }));
    setErrors((prev) => ({ ...prev, edit: null }));

    try {
      await apiClient.replaceLogoImage(id, file, name, description);
      await loadLogos(1, false); // Reload first page
      await loadStats(); // Update stats
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        edit: `Failed to replace logo: ${apiError.message}`,
      }));
      throw error; // Re-throw to prevent dialog from closing
    } finally {
      setLoading((prev) => ({ ...prev, edit: false }));
    }
  };

  const handleDeleteLogo = async (logoId: string) => {
    if (!confirm('Are you sure you want to delete this logo? This action cannot be undone.')) {
      return;
    }

    setLoading((prev) => ({ ...prev, delete: logoId }));
    setErrors((prev) => ({ ...prev, action: null }));

    try {
      await apiClient.deleteLogo(logoId);
      await loadLogos(1, false); // Reload first page
      await loadStats(); // Update stats
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        action: `Failed to delete logo: ${apiError.message}`,
      }));
    } finally {
      setLoading((prev) => ({ ...prev, delete: null }));
    }
  };

  const handleRescanCache = async () => {
    setLoading((prev) => ({ ...prev, rescan: true }));
    setErrors((prev) => ({ ...prev, rescan: null }));

    try {
      const response = await apiClient.rescanLogoCache();
      console.log('Logo cache rescan completed:', response);

      // Reload logos and stats after successful rescan
      await loadLogos(1, false); // Reload first page
      await loadStats(); // Update stats

      // Show success message or handle response as needed
      if (response.success) {
        console.log('Cache rescan successful:', response.message);
      }
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        rescan: `Failed to rescan cache: ${apiError.message}`,
      }));
      console.error('Rescan failed:', apiError);
    } finally {
      setLoading((prev) => ({ ...prev, rescan: false }));
    }
  };

  const handleClearCache = async () => {
    if (
      !confirm('Are you sure you want to clear all cached logos? This action cannot be undone.')
    ) {
      return;
    }

    setLoading((prev) => ({ ...prev, clear: true }));
    setErrors((prev) => ({ ...prev, clear: null }));

    try {
      const response = await apiClient.clearLogoCache();
      console.log('Logo cache cleared:', response);

      // Reload logos and stats after successful clear
      await loadLogos(1, false); // Reload first page
      await loadStats(); // Update stats

      // Show success message or handle response as needed
      if (response.success) {
        console.log('Cache clear successful:', response.message);
      }
    } catch (error) {
      const apiError = error as ApiError;
      setErrors((prev) => ({
        ...prev,
        clear: `Failed to clear cache: ${apiError.message}`,
      }));
      console.error('Clear cache failed:', apiError);
    } finally {
      setLoading((prev) => ({ ...prev, clear: false }));
    }
  };

  // Calculate total storage including filesystem cached
  const totalStorageUsed =
    (stats?.total_storage_used || 0) + (stats?.filesystem_cached_storage || 0);
  const totalCachedLogos = (stats?.total_cached_logos || 0) + (stats?.filesystem_cached_logos || 0);

  return (
    <TooltipProvider>
      <div className="space-y-6">
        {/* Header Section */}
        <div className="flex items-center justify-between">
          <div>
            <p className="text-muted-foreground">Manage uploaded and cached logo assets</p>
          </div>
          <div className="flex items-center gap-2">
            {!isOnline && <WifiOff className="h-5 w-5 text-destructive" />}
            <Button onClick={() => setIsUploadSheetOpen(true)} className="gap-2">
              <Plus className="h-4 w-4" />
              Upload Logo
            </Button>
            <Button
              onClick={handleRescanCache}
              disabled={loading.rescan}
              variant="outline"
              className="gap-2"
            >
              {loading.rescan ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <RefreshCw className="h-4 w-4" />
              )}
              Rescan Cache
            </Button>
            <Button
              onClick={handleClearCache}
              disabled={loading.clear}
              variant="outline"
              className="gap-2 text-destructive hover:text-destructive"
            >
              {loading.clear ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Trash2 className="h-4 w-4" />
              )}
              Clear Cache
            </Button>
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

        {/* Rescan Error Alert */}
        {errors.rescan && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Cache Rescan Error</AlertTitle>
            <AlertDescription>
              {errors.rescan}
              <Button
                variant="outline"
                size="sm"
                className="ml-2"
                onClick={() => setErrors((prev) => ({ ...prev, rescan: null }))}
              >
                Dismiss
              </Button>
            </AlertDescription>
          </Alert>
        )}

        {/* Clear Cache Error Alert */}
        {errors.clear && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Clear Cache Error</AlertTitle>
            <AlertDescription>
              {errors.clear}
              <Button
                variant="outline"
                size="sm"
                className="ml-2"
                onClick={() => setErrors((prev) => ({ ...prev, clear: null }))}
              >
                Dismiss
              </Button>
            </AlertDescription>
          </Alert>
        )}

        {/* Statistics Cards */}
        <div className="grid gap-2 md:grid-cols-4">
          <StatCard
            title="Total Logos"
            value={(stats?.total_uploaded_logos || 0) + totalCachedLogos}
            icon={<ImageIcon className="h-4 w-4" />}
          />
          <StatCard
            title="Storage Used"
            value={formatFileSize(totalStorageUsed)}
            icon={<HardDrive className="h-4 w-4" />}
          />
          <StatCard
            title="Uploaded"
            value={stats?.total_uploaded_logos || 0}
            icon={<Upload className="h-4 w-4" />}
          />
          <StatCard
            title="Linked Assets"
            value={stats?.total_linked_assets || 0}
            icon={<Link2 className="h-4 w-4" />}
          />
        </div>

        {/* Logo Display */}
        <Card>
          <CardHeader className="pb-4">
            <div className="flex flex-col sm:flex-row sm:items-center gap-4">
              <div className="flex-1">
                <CardTitle className="flex items-center gap-2">
                  <span>Logos ({filteredLogos.length})</span>
                  {loading.logos && <Loader2 className="h-4 w-4 animate-spin" />}
                </CardTitle>
              </div>
              <div className="flex items-center gap-2">
                <div className="relative">
                  <Search className="absolute left-2 top-2.5 h-4 w-4 text-muted-foreground" />
                  <Input
                    ref={searchInputRef}
                    placeholder="Search logos (fuzzy)..."
                    value={searchTerm}
                    onChange={(e) => setSearchTerm(e.target.value)}
                    className="pl-8 w-[200px] h-9"
                  />
                </div>
                <Select
                  value={logoFilter}
                  onValueChange={(value) => setLogoFilter(value as 'all' | 'uploaded' | 'cached')}
                  disabled={loading.logos}
                >
                  <SelectTrigger className="w-[140px] h-9">
                    <SelectValue placeholder="Logo types" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">All Types</SelectItem>
                    <SelectItem value="uploaded">Uploaded</SelectItem>
                    <SelectItem value="cached">Cached</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            {loading.logos && isInitialLoad ? (
              <div className="grid gap-4 grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 2xl:grid-cols-6">
                {Array.from({ length: 12 }).map((_, i) => (
                  <Card key={i} className="animate-pulse">
                    <CardHeader className="pb-2">
                      <div className="flex items-center gap-2">
                        <Skeleton className="h-5 w-16" />
                        <Skeleton className="h-5 w-12" />
                      </div>
                    </CardHeader>
                    <CardContent className="space-y-3">
                      <Skeleton className="aspect-square w-full" />
                      <div className="space-y-2">
                        <Skeleton className="h-4 w-3/4" />
                        <Skeleton className="h-3 w-1/2" />
                      </div>
                      <div className="space-y-1">
                        <div className="flex justify-between">
                          <Skeleton className="h-3 w-8" />
                          <Skeleton className="h-3 w-12" />
                        </div>
                        <div className="flex justify-between">
                          <Skeleton className="h-3 w-8" />
                          <Skeleton className="h-3 w-16" />
                        </div>
                      </div>
                    </CardContent>
                  </Card>
                ))}
              </div>
            ) : errors.logos ? (
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertTitle>Failed to Load Logos</AlertTitle>
                <AlertDescription>
                  {errors.logos}
                  <Button
                    variant="outline"
                    size="sm"
                    className="ml-2"
                    onClick={() => loadLogos(1, false)}
                    disabled={loading.logos}
                  >
                    {loading.logos && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                    Retry
                  </Button>
                </AlertDescription>
              </Alert>
            ) : (
              <>
                  <div className="grid gap-4 grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 2xl:grid-cols-6">
                    {filteredLogos.map((logo) => (
                      <Card
                        key={logo.id}
                        className={`relative group transition-all hover:shadow-md cursor-pointer ${
                          selectedLogo?.id === logo.id ? 'ring-2 ring-primary' : ''
                        }`}
                        onClick={() => setSelectedLogo(logo)}
                      >
                        <CardHeader className="pb-2">
                          <div className="flex items-start justify-between">
                            <div className="flex flex-wrap items-center gap-1">
                              <Badge variant="secondary">
                                {logo.asset_type === 'cached' ? 'Cached' : 'Uploaded'}
                              </Badge>
                              {/* Show format badges for linked assets */}
                              {logo.linked_assets && logo.linked_assets.length > 0 ? (
                                logo.linked_assets.map((asset, idx) => (
                                  <Tooltip key={idx}>
                                    <TooltipTrigger asChild>
                                      <Badge
                                        variant={asset.type === 'display' ? 'default' : 'outline'}
                                        className="text-xs cursor-help"
                                      >
                                        {getFormatFromMimeType(asset.content_type)}
                                      </Badge>
                                    </TooltipTrigger>
                                    <TooltipContent side="top" className="text-xs">
                                      <p className="capitalize">{asset.type}</p>
                                      <p className="text-muted-foreground">{formatFileSize(asset.size)}</p>
                                    </TooltipContent>
                                  </Tooltip>
                                ))
                              ) : (
                                <Badge variant="outline">
                                  {getFormatFromMimeType(logo.mime_type)}
                                </Badge>
                              )}
                            </div>
                            <div
                              className="absolute top-2 right-2 flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity bg-popover border border-border rounded-md p-1 shadow-md"
                              onClick={(e) => e.stopPropagation()}
                            >
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => handleDeleteLogo(logo.id)}
                                className="h-6 w-6 p-0 text-destructive hover:text-destructive hover:bg-destructive/10"
                                disabled={loading.delete === logo.id}
                                title="Delete logo"
                              >
                                {loading.delete === logo.id ? (
                                  <Loader2 className="h-3 w-3 animate-spin" />
                                ) : (
                                  <Trash2 className="h-3 w-3" />
                                )}
                              </Button>
                            </div>
                          </div>
                        </CardHeader>

                        <CardContent className="space-y-3">
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <div className="aspect-square bg-muted rounded-md flex items-center justify-center overflow-hidden cursor-help">
                                <img
                                  src={
                                    logo.url.startsWith('http')
                                      ? logo.url
                                      : `${API_CONFIG.baseUrl}${logo.url}`
                                  }
                                  alt={logo.name}
                                  className="max-w-full max-h-full object-contain"
                                  onError={(e) => {
                                    e.currentTarget.style.display = 'none';
                                    e.currentTarget.nextElementSibling?.classList.remove('hidden');
                                  }}
                                />
                                <div className="hidden flex-col items-center gap-2 text-muted-foreground">
                                  <FileImage className="h-8 w-8" />
                                  <span className="text-xs">Preview unavailable</span>
                                </div>
                              </div>
                            </TooltipTrigger>
                            <TooltipContent side="right" className="max-w-xs">
                              <div className="space-y-2 text-xs">
                                <p className="font-semibold">{logo.name}</p>
                                <div className="grid grid-cols-2 gap-x-3 gap-y-1">
                                  <span className="text-muted-foreground">Format:</span>
                                  <span>{getFormatFromMimeType(logo.mime_type)}</span>
                                  {logo.original_mime_type && (
                                    <>
                                      <span className="text-muted-foreground">Original:</span>
                                      <span>{getFormatFromMimeType(logo.original_mime_type)}</span>
                                    </>
                                  )}
                                  <span className="text-muted-foreground">Display Size:</span>
                                  <span>{formatFileSize(logo.file_size)}</span>
                                  {logo.original_file_size && (
                                    <>
                                      <span className="text-muted-foreground">Original Size:</span>
                                      <span>{formatFileSize(logo.original_file_size)}</span>
                                    </>
                                  )}
                                  {logo.width && logo.height && (
                                    <>
                                      <span className="text-muted-foreground">Dimensions:</span>
                                      <span>
                                        {logo.width}x{logo.height}
                                      </span>
                                    </>
                                  )}
                                  <span className="text-muted-foreground">Linked Assets:</span>
                                  <span>{logo.linked_assets_count || 0}</span>
                                  {(logo.total_linked_size || 0) > 0 && (
                                    <>
                                      <span className="text-muted-foreground">Total Size:</span>
                                      <span>{formatFileSize(logo.total_linked_size || 0)}</span>
                                    </>
                                  )}
                                </div>
                                {logo.linked_assets && logo.linked_assets.length > 0 && (
                                  <div className="border-t pt-1 mt-1">
                                    <p className="font-medium mb-1">Assets:</p>
                                    {logo.linked_assets.map((asset, idx) => (
                                      <div key={idx} className="flex justify-between">
                                        <span className="text-muted-foreground capitalize">
                                          {asset.type}:
                                        </span>
                                        <span>
                                          {getFormatFromMimeType(asset.content_type)} (
                                          {formatFileSize(asset.size)})
                                        </span>
                                      </div>
                                    ))}
                                  </div>
                                )}
                              </div>
                            </TooltipContent>
                          </Tooltip>

                          <div className="space-y-2">
                            <div>
                              <p className="font-medium text-sm truncate" title={logo.name}>
                                {logo.name}
                              </p>
                              {logo.description && (
                                <p
                                  className="text-xs text-muted-foreground truncate"
                                  title={logo.description}
                                >
                                  {logo.description}
                                </p>
                              )}
                            </div>

                            <div className="space-y-1">
                              <div className="flex items-center justify-between text-xs">
                                <span className="text-muted-foreground">ID:</span>
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <code className="text-xs bg-muted px-1 rounded truncate max-w-[60px] cursor-help">
                                      {logo.id.split('-')[0]}...
                                    </code>
                                  </TooltipTrigger>
                                  <TooltipContent>
                                    <p className="font-mono text-xs">{logo.id}</p>
                                  </TooltipContent>
                                </Tooltip>
                              </div>

                              <div className="flex items-center justify-between text-xs">
                                <span className="text-muted-foreground">Size:</span>
                                <span>{formatFileSize(logo.file_size)}</span>
                              </div>

                              {logo.width && logo.height && (
                                <div className="flex items-center justify-between text-xs">
                                  <span className="text-muted-foreground">Dimensions:</span>
                                  <span>
                                    {logo.width}×{logo.height}
                                  </span>
                                </div>
                              )}

                              <div className="flex items-center justify-between text-xs">
                                <span className="text-muted-foreground">Created:</span>
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <span className="cursor-help">
                                      {formatRelativeTimeShort(logo.created_at)}
                                    </span>
                                  </TooltipTrigger>
                                  <TooltipContent>
                                    <p className="text-xs">{formatDate(logo.created_at)}</p>
                                  </TooltipContent>
                                </Tooltip>
                              </div>
                            </div>
                          </div>
                        </CardContent>
                      </Card>
                    ))}
                  </div>

                {/* Infinite Scroll Trigger */}
                {hasMore && !searchTerm && (
                  <div ref={loadMoreRef} className="h-20 flex items-center justify-center">
                    {loading.logos ? (
                      <div className="flex items-center gap-2 text-muted-foreground">
                        <Loader2 className="h-4 w-4 animate-spin" />
                        <span className="text-sm">Loading more logos...</span>
                      </div>
                    ) : (
                      <div className="text-center">
                        <p className="text-sm text-muted-foreground mb-2">
                          {totalPages - currentPage} pages remaining
                        </p>
                        <Button
                          variant="outline"
                          onClick={handleLoadMore}
                          size="sm"
                          className="gap-2"
                        >
                          <Download className="h-4 w-4" />
                          Load More
                        </Button>
                      </div>
                    )}
                  </div>
                )}

                {/* Empty State */}
                {filteredLogos.length === 0 && !loading.logos && (
                  <div className="text-center py-8">
                    <ImageIcon className="mx-auto h-12 w-12 text-muted-foreground" />
                    <h3 className="mt-4 text-lg font-semibold">
                      {searchTerm || logoFilter !== 'all' ? 'No matching logos' : 'No logos found'}
                    </h3>
                    <p className="text-muted-foreground">
                      {searchTerm || logoFilter !== 'all'
                        ? 'Try adjusting your search or filter criteria.'
                        : 'Get started by uploading your first logo asset.'}
                    </p>
                  </div>
                )}
              </>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Upload Logo Sheet */}
      <UploadLogoSheet
        onUploadLogo={handleUploadLogo}
        loading={loading.upload}
        error={errors.upload}
        open={isUploadSheetOpen}
        onOpenChange={setIsUploadSheetOpen}
      />

      <EditLogoSheet
        logo={selectedLogo}
        onUpdateLogo={handleUpdateLogo}
        onReplaceLogo={handleReplaceLogo}
        loading={loading.edit}
        error={errors.edit}
        open={selectedLogo !== null}
        onOpenChange={(open) => !open && setSelectedLogo(null)}
      />
    </TooltipProvider>
  );
}
