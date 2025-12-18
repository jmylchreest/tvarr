'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { StatCard } from '@/components/shared/feedback/StatCard';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Checkbox } from '@/components/ui/checkbox';
import { Textarea } from '@/components/ui/textarea';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
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
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {
  Plus,
  Trash2,
  Loader2,
  Copy,
  Star,
  Lock,
  Settings,
  Video,
  Zap,
  Terminal,
  RefreshCw,
} from 'lucide-react';
import { EncodingProfile, EncodingProfilePreview, QualityPreset } from '@/types/api';
import { apiClient, ApiError } from '@/lib/api-client';
import { ExportDialog, ImportDialog } from '@/components/config-export';
import {
  MasterDetailLayout,
  DetailPanel,
  DetailEmpty,
  MasterItem,
} from '@/components/shared';
import { BadgeGroup, BadgeItem } from '@/components/shared';
import { TooltipProvider } from '@/components/ui/tooltip';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { ChevronDown, ChevronRight, AlertCircle } from 'lucide-react';

interface LoadingState {
  profiles: boolean;
  create: boolean;
  edit: boolean;
  delete: string | null;
  setDefault: string | null;
}

interface ErrorState {
  profiles: string | null;
  create: string | null;
  edit: string | null;
  action: string | null;
}

const VIDEO_CODECS = [
  { value: 'h264', label: 'H.264 (AVC)', description: 'Universal compatibility' },
  { value: 'h265', label: 'H.265 (HEVC)', description: 'Better compression, modern devices' },
  { value: 'vp9', label: 'VP9', description: 'Web-optimized, requires fMP4' },
  { value: 'av1', label: 'AV1', description: 'Best compression, requires fMP4' },
];

const AUDIO_CODECS = [
  { value: 'aac', label: 'AAC', description: 'Universal compatibility' },
  { value: 'opus', label: 'Opus', description: 'Best quality/size, requires fMP4' },
  { value: 'ac3', label: 'AC-3 (Dolby Digital)', description: 'Surround sound' },
  { value: 'eac3', label: 'E-AC-3 (Dolby Digital Plus)', description: 'Enhanced surround' },
  { value: 'mp3', label: 'MP3', description: 'Legacy compatibility' },
];

const QUALITY_PRESETS = [
  { value: 'low', label: 'Low', description: 'CRF 28, ~2Mbps max, 128k audio' },
  { value: 'medium', label: 'Medium', description: 'CRF 23, ~5Mbps max, 192k audio' },
  { value: 'high', label: 'High', description: 'CRF 20, ~10Mbps max, 256k audio' },
  { value: 'ultra', label: 'Ultra', description: 'CRF 16, no bitrate cap, 320k audio' },
];

const HW_ACCEL_OPTIONS = [
  { value: 'auto', label: 'Auto', description: 'Detect available hardware' },
  { value: 'none', label: 'None', description: 'Software encoding only' },
  { value: 'cuda', label: 'NVIDIA CUDA', description: 'NVIDIA GPU acceleration' },
  { value: 'vaapi', label: 'VA-API', description: 'Intel/AMD Linux acceleration' },
  { value: 'qsv', label: 'Intel QuickSync', description: 'Intel GPU acceleration' },
  { value: 'videotoolbox', label: 'VideoToolbox', description: 'Apple Silicon acceleration' },
];

function formatRelativeTime(dateString: string): string {
  const now = new Date();
  const date = new Date(dateString);
  const diffMs = now.getTime() - date.getTime();
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
  const diffDays = Math.floor(diffHours / 24);

  if (diffDays > 0) {
    return `${diffDays}d ago`;
  } else if (diffHours > 0) {
    return `${diffHours}h ago`;
  } else {
    return 'Just now';
  }
}

interface ProfileFormData {
  name: string;
  description: string;
  target_video_codec: string;
  target_audio_codec: string;
  quality_preset: QualityPreset;
  hw_accel: string;
  global_flags: string;
  input_flags: string;
  output_flags: string;
  is_default: boolean;
}

const defaultFormData: ProfileFormData = {
  name: '',
  description: '',
  target_video_codec: 'h264',
  target_audio_codec: 'aac',
  quality_preset: 'medium',
  hw_accel: 'auto',
  global_flags: '',
  input_flags: '',
  output_flags: '',
  is_default: false,
};

/**
 * EncodingProfileCreatePanel - Inline panel for creating a new encoding profile
 */
function EncodingProfileCreatePanel({
  onCreate,
  onCancel,
  loading,
  error,
}: {
  onCreate: (data: ProfileFormData) => Promise<void>;
  onCancel: () => void;
  loading: boolean;
  error: string | null;
}) {
  const [formData, setFormData] = useState<ProfileFormData>(defaultFormData);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [showPreview, setShowPreview] = useState(false);
  const [preview, setPreview] = useState<EncodingProfilePreview | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [previewError, setPreviewError] = useState<string | null>(null);

  const loadPreview = useCallback(async () => {
    setPreviewLoading(true);
    setPreviewError(null);
    try {
      const result = await apiClient.previewEncodingProfileCommand({
        target_video_codec: formData.target_video_codec,
        target_audio_codec: formData.target_audio_codec,
        quality_preset: formData.quality_preset,
        hw_accel: formData.hw_accel,
        global_flags: formData.global_flags || undefined,
        input_flags: formData.input_flags || undefined,
        output_flags: formData.output_flags || undefined,
      });
      setPreview(result);
    } catch (err) {
      setPreviewError(err instanceof Error ? err.message : 'Failed to load preview');
    } finally {
      setPreviewLoading(false);
    }
  }, [formData]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    await onCreate(formData);
  };

  return (
    <DetailPanel
      title="Create Encoding Profile"
      actions={
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={onCancel} disabled={loading}>
            Cancel
          </Button>
          <Button size="sm" onClick={handleSubmit} disabled={loading || !formData.name.trim()}>
            {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
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

      <form onSubmit={handleSubmit} className="space-y-4">
        {/* Basic Info */}
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="create-name">Name *</Label>
            <Input
              id="create-name"
              value={formData.name}
              onChange={(e) => setFormData({ ...formData, name: e.target.value })}
              placeholder="My Encoding Profile"
              disabled={loading}
              required
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="create-description">Description</Label>
            <Textarea
              id="create-description"
              value={formData.description}
              onChange={(e) => setFormData({ ...formData, description: e.target.value })}
              placeholder="Optional description for this profile"
              rows={2}
              disabled={loading}
            />
          </div>

          <div className="flex items-center space-x-2">
            <Checkbox
              id="create-is_default"
              checked={formData.is_default}
              onCheckedChange={(checked) => setFormData({ ...formData, is_default: checked === true })}
              disabled={loading}
            />
            <Label htmlFor="create-is_default" className="text-sm font-normal cursor-pointer">
              Set as default encoding profile for proxies
            </Label>
          </div>
        </div>

        {/* Codec Settings */}
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label>Video Codec</Label>
            <Select
              value={formData.target_video_codec}
              onValueChange={(value) => setFormData({ ...formData, target_video_codec: value })}
              disabled={loading}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {VIDEO_CODECS.map((codec) => (
                  <SelectItem key={codec.value} value={codec.value}>
                    <div className="flex flex-col">
                      <span>{codec.label}</span>
                      <span className="text-xs text-muted-foreground">{codec.description}</span>
                    </div>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2">
            <Label>Audio Codec</Label>
            <Select
              value={formData.target_audio_codec}
              onValueChange={(value) => setFormData({ ...formData, target_audio_codec: value })}
              disabled={loading}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {AUDIO_CODECS.map((codec) => (
                  <SelectItem key={codec.value} value={codec.value}>
                    <div className="flex flex-col">
                      <span>{codec.label}</span>
                      <span className="text-xs text-muted-foreground">{codec.description}</span>
                    </div>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>

        {/* Quality Settings */}
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label>Quality Preset</Label>
            <Select
              value={formData.quality_preset}
              onValueChange={(value) => setFormData({ ...formData, quality_preset: value as QualityPreset })}
              disabled={loading}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {QUALITY_PRESETS.map((preset) => (
                  <SelectItem key={preset.value} value={preset.value}>
                    <div className="flex flex-col">
                      <span>{preset.label}</span>
                      <span className="text-xs text-muted-foreground">{preset.description}</span>
                    </div>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2">
            <Label>Hardware Acceleration</Label>
            <Select
              value={formData.hw_accel}
              onValueChange={(value) => setFormData({ ...formData, hw_accel: value })}
              disabled={loading}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {HW_ACCEL_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    <div className="flex flex-col">
                      <span>{option.label}</span>
                      <span className="text-xs text-muted-foreground">{option.description}</span>
                    </div>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>

        {/* Advanced FFmpeg Flags */}
        <div className="space-y-4">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setShowAdvanced(!showAdvanced)}
            className="w-full"
          >
            <Settings className="h-4 w-4 mr-2" />
            {showAdvanced ? 'Hide' : 'Show'} Advanced FFmpeg Flags
          </Button>

          {showAdvanced && (
            <div className="space-y-4 p-4 border rounded-lg bg-muted/50">
              <p className="text-sm text-muted-foreground">
                Custom flags override auto-generated flags. Leave empty to use defaults.
              </p>

              <div className="space-y-2">
                <Label htmlFor="create-global_flags">Global Flags</Label>
                <Input
                  id="create-global_flags"
                  value={formData.global_flags}
                  onChange={(e) => setFormData({ ...formData, global_flags: e.target.value })}
                  placeholder="-hide_banner -stats"
                  disabled={loading}
                />
                <p className="text-xs text-muted-foreground">Placed at the start of the command</p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="create-input_flags">Input Flags</Label>
                <Input
                  id="create-input_flags"
                  value={formData.input_flags}
                  onChange={(e) => setFormData({ ...formData, input_flags: e.target.value })}
                  placeholder="# hwaccel auto-detected"
                  disabled={loading}
                />
                <p className="text-xs text-muted-foreground">Placed before -i input</p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="create-output_flags">Output Flags</Label>
                <Textarea
                  id="create-output_flags"
                  value={formData.output_flags}
                  onChange={(e) => setFormData({ ...formData, output_flags: e.target.value })}
                  placeholder="-c:v libx264 -preset medium ..."
                  rows={3}
                  disabled={loading}
                />
                <p className="text-xs text-muted-foreground">Placed after -i input</p>
              </div>
            </div>
          )}
        </div>

        {/* FFmpeg Command Preview */}
        <div className="space-y-4">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => {
              setShowPreview(!showPreview);
              if (!showPreview && !preview) {
                loadPreview();
              }
            }}
            className="w-full"
          >
            <Terminal className="h-4 w-4 mr-2" />
            {showPreview ? 'Hide' : 'Show'} FFmpeg Command Preview
          </Button>

          {showPreview && (
            <div className="space-y-4 p-4 border rounded-lg bg-muted/50">
              <div className="flex items-center justify-between">
                <p className="text-sm font-medium">Generated FFmpeg Command</p>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={loadPreview}
                  disabled={previewLoading}
                >
                  {previewLoading ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <RefreshCw className="h-4 w-4" />
                  )}
                </Button>
              </div>

              {previewError && (
                <div className="bg-destructive/10 text-destructive px-3 py-2 rounded-md text-sm">
                  {previewError}
                </div>
              )}

              {preview && !previewError && (
                <div className="space-y-3">
                  <div className="flex items-center gap-2 text-sm">
                    <span className="text-muted-foreground">Encoders:</span>
                    <Badge variant="outline">{preview.video_encoder}</Badge>
                    <Badge variant="outline">{preview.audio_encoder}</Badge>
                    {preview.using_custom && (
                      <Badge variant="secondary">Custom flags</Badge>
                    )}
                  </div>
                  <div className="relative">
                    <pre className="text-xs bg-black/90 text-green-400 p-3 rounded-md overflow-x-auto whitespace-pre-wrap font-mono">
                      {preview.command}
                    </pre>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="absolute top-2 right-2 h-6 w-6 p-0"
                      onClick={() => navigator.clipboard.writeText(preview.command)}
                    >
                      <Copy className="h-3 w-3" />
                    </Button>
                  </div>
                </div>
              )}

              {previewLoading && !preview && (
                <div className="flex items-center justify-center py-4">
                  <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
                </div>
              )}
            </div>
          )}
        </div>
      </form>
    </DetailPanel>
  );
}

// Convert EncodingProfile to MasterItem format for MasterDetailLayout
interface EncodingProfileMasterItem extends MasterItem {
  profile: EncodingProfile;
}

function encodingProfileToMasterItem(profile: EncodingProfile): EncodingProfileMasterItem {
  // Build array of badges with priority-based styling
  const badges: BadgeItem[] = [];

  if (profile.is_default) {
    badges.push({ label: 'Default', priority: 'success' });
  }
  if (profile.is_system) {
    badges.push({ label: 'System', priority: 'secondary' });
  }
  if (profile.hw_accel !== 'none') {
    badges.push({ label: profile.hw_accel, priority: 'info' });
  }

  return {
    id: profile.id,
    title: profile.name,
    badge: badges.length > 0 ? <BadgeGroup badges={badges} size="sm" /> : null,
    profile,
  };
}

// Collapsible section component for organizing profile settings
function CollapsibleSection({
  title,
  defaultOpen = false,
  children,
}: {
  title: string;
  defaultOpen?: boolean;
  children: React.ReactNode;
}) {
  const [isOpen, setIsOpen] = useState(defaultOpen);

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen} className="border rounded-lg">
      <CollapsibleTrigger className="flex items-center justify-between w-full p-3 hover:bg-muted/50 transition-colors">
        <span className="font-medium text-sm">{title}</span>
        {isOpen ? (
          <ChevronDown className="h-4 w-4 text-muted-foreground" />
        ) : (
          <ChevronRight className="h-4 w-4 text-muted-foreground" />
        )}
      </CollapsibleTrigger>
      <CollapsibleContent className="p-3 pt-0 border-t">{children}</CollapsibleContent>
    </Collapsible>
  );
}

// Detail panel for viewing/editing a selected encoding profile
function EncodingProfileDetailPanel({
  profile,
  onUpdate,
  onDelete,
  onSetDefault,
  loading,
  error,
}: {
  profile: EncodingProfile;
  onUpdate: (id: string, data: ProfileFormData) => Promise<void>;
  onDelete: (profile: EncodingProfile) => Promise<void>;
  onSetDefault: (profile: EncodingProfile) => Promise<void>;
  loading: { edit: boolean; delete: string | null; setDefault: string | null };
  error: string | null;
}) {
  const [formData, setFormData] = useState<ProfileFormData>({
    name: profile.name,
    description: profile.description || '',
    target_video_codec: profile.target_video_codec,
    target_audio_codec: profile.target_audio_codec,
    quality_preset: profile.quality_preset,
    hw_accel: profile.hw_accel,
    global_flags: profile.global_flags || '',
    input_flags: profile.input_flags || '',
    output_flags: profile.output_flags || '',
    is_default: profile.is_default,
  });
  const [hasChanges, setHasChanges] = useState(false);
  const [preview, setPreview] = useState<EncodingProfilePreview | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [previewError, setPreviewError] = useState<string | null>(null);

  // Reset form when profile changes
  useEffect(() => {
    setFormData({
      name: profile.name,
      description: profile.description || '',
      target_video_codec: profile.target_video_codec,
      target_audio_codec: profile.target_audio_codec,
      quality_preset: profile.quality_preset,
      hw_accel: profile.hw_accel,
      global_flags: profile.global_flags || '',
      input_flags: profile.input_flags || '',
      output_flags: profile.output_flags || '',
      is_default: profile.is_default,
    });
    setHasChanges(false);
    setPreview(null);
    setPreviewError(null);
  }, [profile.id]);

  const handleFieldChange = (field: keyof ProfileFormData, value: any) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
    setHasChanges(true);
    setPreview(null); // Clear preview when settings change
  };

  const handleSave = async () => {
    await onUpdate(profile.id, formData);
    setHasChanges(false);
  };

  const loadPreview = async () => {
    setPreviewLoading(true);
    setPreviewError(null);
    try {
      const result = await apiClient.previewEncodingProfileCommand({
        target_video_codec: formData.target_video_codec,
        target_audio_codec: formData.target_audio_codec,
        quality_preset: formData.quality_preset,
        hw_accel: formData.hw_accel,
        global_flags: formData.global_flags || undefined,
        input_flags: formData.input_flags || undefined,
        output_flags: formData.output_flags || undefined,
      });
      setPreview(result);
    } catch (err) {
      setPreviewError(err instanceof Error ? err.message : 'Failed to load preview');
    } finally {
      setPreviewLoading(false);
    }
  };

  const isSystem = profile.is_system;

  return (
    <DetailPanel
      title={profile.name}
      actions={
        <div className="flex items-center gap-1">
          {!profile.is_default && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onSetDefault(profile)}
              disabled={loading.setDefault === profile.id}
              title="Set as default profile"
            >
              {loading.setDefault === profile.id ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Star className="h-4 w-4" />
              )}
            </Button>
          )}
          <Button
            variant="outline"
            size="sm"
            onClick={() => onDelete(profile)}
            disabled={loading.delete === profile.id || isSystem}
            className="text-destructive hover:text-destructive"
            title={isSystem ? "System profiles cannot be deleted" : "Delete profile"}
          >
            {loading.delete === profile.id ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Trash2 className="h-4 w-4" />
            )}
          </Button>
        </div>
      }
    >
      <div className="space-y-4">
        {error && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        {isSystem && (
          <Alert>
            <Lock className="h-4 w-4" />
            <AlertTitle>System Profile</AlertTitle>
            <AlertDescription>
              This is a system profile and cannot be modified or deleted.
            </AlertDescription>
          </Alert>
        )}

        {/* Profile Info */}
        <div className="grid grid-cols-2 gap-4">
          <div>
            <Label className="text-xs text-muted-foreground">Status</Label>
            <div className="mt-1 flex items-center gap-2">
              {profile.is_default && (
                <Badge variant="default">
                  <Star className="h-3 w-3 mr-1" />
                  Default
                </Badge>
              )}
              {profile.is_system && (
                <Badge variant="outline" className="text-purple-600 border-purple-600">
                  <Lock className="h-3 w-3 mr-1" />
                  System
                </Badge>
              )}
              {!profile.is_default && !profile.is_system && (
                <Badge variant="outline">Custom</Badge>
              )}
            </div>
          </div>
          <div>
            <Label className="text-xs text-muted-foreground">Created</Label>
            <div className="mt-1 text-sm">
              {profile.created_at ? formatRelativeTime(profile.created_at) : 'Unknown'}
            </div>
          </div>
        </div>

        {/* Basic Settings */}
        <CollapsibleSection title="Basic Settings" defaultOpen={true}>
          <div className="space-y-4 pt-3">
            <div className="space-y-2">
              <Label htmlFor="detail-name">Name</Label>
              <Input
                id="detail-name"
                value={formData.name}
                onChange={(e) => handleFieldChange('name', e.target.value)}
                disabled={loading.edit || isSystem}
                autoComplete="off"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="detail-description">Description</Label>
              <Textarea
                id="detail-description"
                value={formData.description}
                onChange={(e) => handleFieldChange('description', e.target.value)}
                placeholder="Optional description"
                disabled={loading.edit || isSystem}
                rows={2}
              />
            </div>
          </div>
        </CollapsibleSection>

        {/* Codec Settings */}
        <CollapsibleSection title="Codec Settings" defaultOpen={true}>
          <div className="space-y-4 pt-3">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label>Video Codec</Label>
                <Select
                  value={formData.target_video_codec}
                  onValueChange={(value) => handleFieldChange('target_video_codec', value)}
                  disabled={loading.edit || isSystem}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {VIDEO_CODECS.map((codec) => (
                      <SelectItem key={codec.value} value={codec.value}>
                        {codec.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>Audio Codec</Label>
                <Select
                  value={formData.target_audio_codec}
                  onValueChange={(value) => handleFieldChange('target_audio_codec', value)}
                  disabled={loading.edit || isSystem}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {AUDIO_CODECS.map((codec) => (
                      <SelectItem key={codec.value} value={codec.value}>
                        {codec.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
          </div>
        </CollapsibleSection>

        {/* Quality Settings */}
        <CollapsibleSection title="Quality Settings" defaultOpen={true}>
          <div className="space-y-4 pt-3">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label>Quality Preset</Label>
                <Select
                  value={formData.quality_preset}
                  onValueChange={(value) => handleFieldChange('quality_preset', value as QualityPreset)}
                  disabled={loading.edit || isSystem}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {QUALITY_PRESETS.map((preset) => (
                      <SelectItem key={preset.value} value={preset.value}>
                        {preset.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>Hardware Acceleration</Label>
                <Select
                  value={formData.hw_accel}
                  onValueChange={(value) => handleFieldChange('hw_accel', value)}
                  disabled={loading.edit || isSystem}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {HW_ACCEL_OPTIONS.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
          </div>
        </CollapsibleSection>

        {/* Advanced FFmpeg Flags */}
        <CollapsibleSection title="Advanced FFmpeg Flags">
          <div className="space-y-4 pt-3">
            <p className="text-sm text-muted-foreground">
              Custom flags override auto-generated flags. Leave empty to use defaults.
            </p>
            <div className="space-y-2">
              <Label htmlFor="detail-global_flags">Global Flags</Label>
              <Input
                id="detail-global_flags"
                value={formData.global_flags}
                onChange={(e) => handleFieldChange('global_flags', e.target.value)}
                placeholder={profile.default_flags?.global_flags || '-hide_banner -stats'}
                disabled={loading.edit || isSystem}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="detail-input_flags">Input Flags</Label>
              <Input
                id="detail-input_flags"
                value={formData.input_flags}
                onChange={(e) => handleFieldChange('input_flags', e.target.value)}
                placeholder={profile.default_flags?.input_flags || '# hwaccel auto-detected'}
                disabled={loading.edit || isSystem}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="detail-output_flags">Output Flags</Label>
              <Textarea
                id="detail-output_flags"
                value={formData.output_flags}
                onChange={(e) => handleFieldChange('output_flags', e.target.value)}
                placeholder={profile.default_flags?.output_flags || '-c:v libx264 -preset medium ...'}
                disabled={loading.edit || isSystem}
                rows={3}
              />
            </div>
          </div>
        </CollapsibleSection>

        {/* FFmpeg Command Preview */}
        <CollapsibleSection title="FFmpeg Command Preview">
          <div className="space-y-4 pt-3">
            <div className="flex items-center justify-between">
              <p className="text-sm text-muted-foreground">Preview the generated FFmpeg command</p>
              <Button
                variant="outline"
                size="sm"
                onClick={loadPreview}
                disabled={previewLoading}
              >
                {previewLoading ? (
                  <Loader2 className="h-4 w-4 animate-spin mr-1" />
                ) : (
                  <RefreshCw className="h-4 w-4 mr-1" />
                )}
                Generate
              </Button>
            </div>

            {previewError && (
              <div className="bg-destructive/10 text-destructive px-3 py-2 rounded-md text-sm">
                {previewError}
              </div>
            )}

            {preview && (
              <div className="space-y-3">
                <div className="flex items-center gap-2 text-sm">
                  <span className="text-muted-foreground">Encoders:</span>
                  <Badge variant="outline">{preview.video_encoder}</Badge>
                  <Badge variant="outline">{preview.audio_encoder}</Badge>
                  {preview.using_custom && <Badge variant="secondary">Custom flags</Badge>}
                </div>
                <div className="relative">
                  <pre className="text-xs bg-black/90 text-green-400 p-3 rounded-md overflow-x-auto whitespace-pre-wrap font-mono">
                    {preview.command}
                  </pre>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="absolute top-2 right-2 h-6 w-6 p-0"
                    onClick={() => navigator.clipboard.writeText(preview.command)}
                  >
                    <Copy className="h-3 w-3" />
                  </Button>
                </div>
              </div>
            )}
          </div>
        </CollapsibleSection>

        {/* Save Button */}
        {hasChanges && !isSystem && (
          <div className="flex justify-end pt-4 border-t">
            <Button onClick={handleSave} disabled={loading.edit}>
              {loading.edit && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Save Changes
            </Button>
          </div>
        )}
      </div>
    </DetailPanel>
  );
}

export function EncodingProfiles() {
  const [allProfiles, setAllProfiles] = useState<EncodingProfile[]>([]);
  const [filterVideoCodec, setFilterVideoCodec] = useState<string>('all');
  const [filterQuality, setFilterQuality] = useState<string>('all');
  const [selectedProfile, setSelectedProfile] = useState<EncodingProfileMasterItem | null>(null);
  const [loading, setLoading] = useState<LoadingState>({
    profiles: true,
    create: false,
    edit: false,
    delete: null,
    setDefault: null,
  });
  const [error, setError] = useState<ErrorState>({
    profiles: null,
    create: null,
    edit: null,
    action: null,
  });
  const [deleteDialog, setDeleteDialog] = useState<{ open: boolean; profile: EncodingProfile | null }>({
    open: false,
    profile: null,
  });
  const [isCreating, setIsCreating] = useState(false);

  const loadProfiles = useCallback(async () => {
    setLoading((prev) => ({ ...prev, profiles: true }));
    setError((prev) => ({ ...prev, profiles: null }));
    try {
      const data = await apiClient.getEncodingProfiles();
      setAllProfiles(data);
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to load encoding profiles';
      setError((prev) => ({ ...prev, profiles: message }));
    } finally {
      setLoading((prev) => ({ ...prev, profiles: false }));
    }
  }, []);

  useEffect(() => {
    loadProfiles();
  }, [loadProfiles]);

  // Local filtering (search is handled by MasterDetailLayout)
  const filteredProfiles = useMemo(() => {
    let filtered = allProfiles;

    // Filter by video codec
    if (filterVideoCodec !== 'all') {
      filtered = filtered.filter((p) => p.target_video_codec === filterVideoCodec);
    }

    // Filter by quality preset
    if (filterQuality !== 'all') {
      filtered = filtered.filter((p) => p.quality_preset === filterQuality);
    }

    return filtered;
  }, [allProfiles, filterVideoCodec, filterQuality]);

  // Convert filtered profiles to master items for MasterDetailLayout
  const masterItems = useMemo(
    () => filteredProfiles.map(encodingProfileToMasterItem),
    [filteredProfiles]
  );

  const handleCreate = async (data: ProfileFormData) => {
    setLoading((prev) => ({ ...prev, create: true }));
    setError((prev) => ({ ...prev, create: null }));
    try {
      const created = await apiClient.createEncodingProfile({
        name: data.name,
        description: data.description || undefined,
        target_video_codec: data.target_video_codec,
        target_audio_codec: data.target_audio_codec,
        quality_preset: data.quality_preset,
        hw_accel: data.hw_accel,
        global_flags: data.global_flags || undefined,
        input_flags: data.input_flags || undefined,
        output_flags: data.output_flags || undefined,
        is_default: data.is_default,
      });
      await loadProfiles();
      // Exit create mode and select the new profile
      setIsCreating(false);
      if (created?.id) {
        // Find the created profile in the updated list and select it
        const updatedProfiles = await apiClient.getEncodingProfiles();
        const newProfile = updatedProfiles.find((p) => p.id === created.id);
        if (newProfile) {
          setSelectedProfile(encodingProfileToMasterItem(newProfile));
        }
      }
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to create profile';
      setError((prev) => ({ ...prev, create: message }));
      throw err;
    } finally {
      setLoading((prev) => ({ ...prev, create: false }));
    }
  };

  const handleUpdate = async (id: string, data: ProfileFormData) => {
    setLoading((prev) => ({ ...prev, edit: true }));
    setError((prev) => ({ ...prev, edit: null }));
    try {
      await apiClient.updateEncodingProfile(id, {
        name: data.name,
        description: data.description || undefined,
        target_video_codec: data.target_video_codec,
        target_audio_codec: data.target_audio_codec,
        quality_preset: data.quality_preset,
        hw_accel: data.hw_accel,
        global_flags: data.global_flags || undefined,
        input_flags: data.input_flags || undefined,
        output_flags: data.output_flags || undefined,
        enabled: true,
      });
      await loadProfiles();
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to update profile';
      setError((prev) => ({ ...prev, edit: message }));
      throw err;
    } finally {
      setLoading((prev) => ({ ...prev, edit: false }));
    }
  };

  const handleDelete = async (profile: EncodingProfile) => {
    setLoading((prev) => ({ ...prev, delete: profile.id }));
    setError((prev) => ({ ...prev, action: null }));
    try {
      await apiClient.deleteEncodingProfile(profile.id);
      await loadProfiles();
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to delete profile';
      setError((prev) => ({ ...prev, action: message }));
    } finally {
      setLoading((prev) => ({ ...prev, delete: null }));
      setDeleteDialog({ open: false, profile: null });
    }
  };

  const handleSetDefault = async (profile: EncodingProfile) => {
    setLoading((prev) => ({ ...prev, setDefault: profile.id }));
    setError((prev) => ({ ...prev, action: null }));
    try {
      await apiClient.setDefaultEncodingProfile(profile.id);
      await loadProfiles();
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to set default profile';
      setError((prev) => ({ ...prev, action: message }));
    } finally {
      setLoading((prev) => ({ ...prev, setDefault: null }));
    }
  };

  // Statistics
  const totalProfiles = allProfiles.length;
  const systemProfiles = allProfiles.filter((p) => p.is_system).length;
  const hwAccelProfiles = allProfiles.filter((p) => p.hw_accel !== 'none').length;

  return (
    <TooltipProvider>
      <div className="space-y-6">
        {/* Header Section */}
        <div className="flex items-center justify-between">
          <p className="text-muted-foreground">Manage transcoding settings for stream relay output</p>
          <div className="flex items-center gap-2">
            <ImportDialog
              importType="encoding_profiles"
              title="Import Encoding Profiles"
              onImportComplete={loadProfiles}
            />
            <ExportDialog
              exportType="encoding_profiles"
              items={allProfiles.map((p) => ({ id: p.id, name: p.name, is_system: p.is_system }))}
              title="Export Encoding Profiles"
            />
          </div>
        </div>

      {/* Statistics Cards */}
      <div className="grid gap-2 md:grid-cols-3">
        <StatCard title="Total Profiles" value={totalProfiles} icon={<Video className="h-4 w-4" />} />
        <StatCard title="System" value={systemProfiles} icon={<Lock className="h-4 w-4 text-purple-600" />} />
        <StatCard title="HW Accelerated" value={hwAccelProfiles} icon={<Zap className="h-4 w-4 text-orange-600" />} />
      </div>

      {/* Filter Dropdowns */}
      <Card className="p-4">
        <div className="flex items-center gap-4 flex-wrap">
          <Label className="text-sm font-medium">Filters:</Label>
          <Select
            value={filterVideoCodec}
            onValueChange={setFilterVideoCodec}
            disabled={loading.profiles}
          >
            <SelectTrigger className="w-[180px]">
              <SelectValue placeholder="Video Codec" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Codecs</SelectItem>
              {VIDEO_CODECS.map((codec) => (
                <SelectItem key={codec.value} value={codec.value}>
                  {codec.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Select
            value={filterQuality}
            onValueChange={setFilterQuality}
            disabled={loading.profiles}
          >
            <SelectTrigger className="w-[180px]">
              <SelectValue placeholder="Quality" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Quality</SelectItem>
              {QUALITY_PRESETS.map((preset) => (
                <SelectItem key={preset.value} value={preset.value}>
                  {preset.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </Card>

      {/* Error display */}
      {error.action && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>{error.action}</AlertDescription>
        </Alert>
      )}

      {/* MasterDetailLayout */}
      <Card className="flex-1">
        <CardContent className="p-0 min-h-[500px] h-[calc(100vh-320px)]">
          {error.profiles ? (
            <div className="p-6">
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertTitle>Failed to Load Profiles</AlertTitle>
                <AlertDescription>
                  {error.profiles}
                  <Button
                    variant="outline"
                    size="sm"
                    className="ml-2"
                    onClick={loadProfiles}
                    disabled={loading.profiles}
                  >
                    {loading.profiles && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                    Retry
                  </Button>
                </AlertDescription>
              </Alert>
            </div>
          ) : (
            <MasterDetailLayout
              items={masterItems}
              selectedId={isCreating ? null : selectedProfile?.id}
              onSelect={(item) => {
                setIsCreating(false);
                setSelectedProfile(item);
              }}
              isLoading={loading.profiles}
              title={`Encoding Profiles (${filteredProfiles.length}${filterVideoCodec !== 'all' || filterQuality !== 'all' ? ` of ${allProfiles.length}` : ''})`}
              searchPlaceholder="Search profiles by name, codec, quality..."
              headerAction={
                <Button
                  size="sm"
                  variant="ghost"
                  className="h-8 w-8 p-0"
                  onClick={() => {
                    setIsCreating(true);
                    setSelectedProfile(null);
                    setError((prev) => ({ ...prev, create: null }));
                  }}
                  disabled={loading.profiles}
                >
                  <Plus className="h-4 w-4" />
                </Button>
              }
              emptyState={{
                title: filterVideoCodec !== 'all' || filterQuality !== 'all' ? 'No matching profiles' : 'No encoding profiles yet',
                description: filterVideoCodec !== 'all' || filterQuality !== 'all' ? 'Try adjusting your filter criteria.' : 'Get started by creating your first encoding profile.',
              }}
              filterFn={(item, term) => {
                const profile = item.profile;
                const lower = term.toLowerCase();
                return (
                  profile.name.toLowerCase().includes(lower) ||
                  (profile.description || '').toLowerCase().includes(lower) ||
                  profile.target_video_codec.toLowerCase().includes(lower) ||
                  profile.target_audio_codec.toLowerCase().includes(lower) ||
                  profile.quality_preset.toLowerCase().includes(lower) ||
                  profile.hw_accel.toLowerCase().includes(lower)
                );
              }}
            >
              {(selected) =>
                isCreating ? (
                  <EncodingProfileCreatePanel
                    onCreate={handleCreate}
                    onCancel={() => setIsCreating(false)}
                    loading={loading.create}
                    error={error.create}
                  />
                ) : selected ? (
                  <EncodingProfileDetailPanel
                    profile={selected.profile}
                    onUpdate={handleUpdate}
                    onDelete={async (profile) => setDeleteDialog({ open: true, profile })}
                    onSetDefault={handleSetDefault}
                    loading={{ edit: loading.edit, delete: loading.delete, setDefault: loading.setDefault }}
                    error={error.edit}
                  />
                ) : (
                  <DetailEmpty
                    icon={<Video className="h-12 w-12" />}
                    title="Select an Encoding Profile"
                    description="Choose a profile from the list to view and edit its configuration."
                  />
                )
              }
            </MasterDetailLayout>
          )}
        </CardContent>
      </Card>

      {/* Delete confirmation dialog */}
      <Dialog open={deleteDialog.open} onOpenChange={(open) => setDeleteDialog({ ...deleteDialog, open })}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Encoding Profile</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete &quot;{deleteDialog.profile?.name}&quot;? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteDialog({ open: false, profile: null })}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => deleteDialog.profile && handleDelete(deleteDialog.profile)}
            >
              {loading.delete ? <Loader2 className="h-4 w-4 animate-spin mr-1" /> : null}
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      </div>
    </TooltipProvider>
  );
}
