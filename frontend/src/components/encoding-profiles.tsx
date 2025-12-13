'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
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
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet';
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
  Edit,
  Trash2,
  Search,
  Loader2,
  Copy,
  Star,
  Lock,
  Settings,
  Video,
  Grid,
  List,
  Table as TableIcon,
  Check,
  Zap,
  Terminal,
  RefreshCw,
} from 'lucide-react';
import { EncodingProfile, EncodingProfilePreview, QualityPreset } from '@/types/api';
import { apiClient, ApiError } from '@/lib/api-client';

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

function ProfileFormSheet({
  profile,
  onSave,
  loading,
  error,
  trigger,
  title,
  description,
}: {
  profile?: EncodingProfile;
  onSave: (data: ProfileFormData) => Promise<void>;
  loading: boolean;
  error: string | null;
  trigger: React.ReactNode;
  title: string;
  description: string;
}) {
  const [open, setOpen] = useState(false);
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

  useEffect(() => {
    if (profile) {
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
      // Show advanced if custom flags are set
      setShowAdvanced(!!(profile.global_flags || profile.input_flags || profile.output_flags));
    } else {
      setFormData(defaultFormData);
      setShowAdvanced(false);
    }
    // Reset preview when sheet opens/closes
    setPreview(null);
    setShowPreview(false);
  }, [profile, open]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    await onSave(formData);
    if (!error) {
      setOpen(false);
    }
  };

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>{trigger}</SheetTrigger>
      <SheetContent side="right" className="w-full sm:max-w-2xl overflow-y-auto">
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>{description}</SheetDescription>
        </SheetHeader>

        <form onSubmit={handleSubmit} className="space-y-6 mt-6 px-4">
          {error && (
            <div className="bg-destructive/10 text-destructive px-4 py-3 rounded-md text-sm">
              {error}
            </div>
          )}


          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="name">Name *</Label>
              <Input
                id="name"
                value={formData.name}
                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                placeholder="My Encoding Profile"
                disabled={loading}
                required
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="description">Description</Label>
              <Textarea
                id="description"
                value={formData.description}
                onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                placeholder="Optional description for this profile"
                rows={2}
                disabled={loading}
              />
            </div>

            <div className="flex items-center space-x-2">
              <Checkbox
                id="is_default"
                checked={formData.is_default}
                onCheckedChange={(checked) => setFormData({ ...formData, is_default: checked === true })}
                disabled={loading}
              />
              <Label htmlFor="is_default" className="text-sm font-normal cursor-pointer">
                Set as default encoding profile for proxies
              </Label>
            </div>
          </div>

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

          {(
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
                    <Label htmlFor="global_flags">Global Flags</Label>
                    <Input
                      id="global_flags"
                      value={formData.global_flags}
                      onChange={(e) => setFormData({ ...formData, global_flags: e.target.value })}
                      placeholder={profile?.default_flags?.global_flags || '-hide_banner -stats'}
                    />
                    <p className="text-xs text-muted-foreground">Placed at the start of the command</p>
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="input_flags">Input Flags</Label>
                    <Input
                      id="input_flags"
                      value={formData.input_flags}
                      onChange={(e) => setFormData({ ...formData, input_flags: e.target.value })}
                      placeholder={profile?.default_flags?.input_flags || '# hwaccel auto-detected'}
                    />
                    <p className="text-xs text-muted-foreground">Placed before -i input</p>
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="output_flags">Output Flags</Label>
                    <Textarea
                      id="output_flags"
                      value={formData.output_flags}
                      onChange={(e) => setFormData({ ...formData, output_flags: e.target.value })}
                      placeholder={profile?.default_flags?.output_flags || '-c:v libx264 -preset medium ...'}
                      rows={3}
                    />
                    <p className="text-xs text-muted-foreground">Placed after -i input</p>
                  </div>
                </div>
              )}
            </div>
          )}

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
                    <details className="text-xs">
                      <summary className="cursor-pointer text-muted-foreground hover:text-foreground">
                        Show flag breakdown
                      </summary>
                      <div className="mt-2 space-y-2 pl-2 border-l-2 border-muted">
                        <div>
                          <span className="text-muted-foreground">Global:</span>
                          <code className="ml-2 text-xs bg-muted px-1 rounded">{preview.global_flags}</code>
                        </div>
                        <div>
                          <span className="text-muted-foreground">Input:</span>
                          <code className="ml-2 text-xs bg-muted px-1 rounded">{preview.input_flags}</code>
                        </div>
                        <div>
                          <span className="text-muted-foreground">Output:</span>
                          <code className="ml-2 text-xs bg-muted px-1 rounded break-all">{preview.output_flags}</code>
                        </div>
                      </div>
                    </details>
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

          <SheetFooter>
            <Button type="submit" disabled={loading}>
              {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {profile ? 'Update Profile' : 'Create Profile'}
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  );
}


export function EncodingProfiles() {
  const [allProfiles, setAllProfiles] = useState<EncodingProfile[]>([]);
  const [searchTerm, setSearchTerm] = useState('');
  const [filterVideoCodec, setFilterVideoCodec] = useState<string>('all');
  const [filterQuality, setFilterQuality] = useState<string>('all');
  const [viewMode, setViewMode] = useState<'grid' | 'list' | 'table'>('table');
  const [copiedItems, setCopiedItems] = useState<Set<string>>(new Set());
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

  // Local filtering
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

    // Search term
    if (searchTerm.trim()) {
      const searchLower = searchTerm.toLowerCase();
      filtered = filtered.filter(
        (p) =>
          p.name.toLowerCase().includes(searchLower) ||
          p.description?.toLowerCase().includes(searchLower) ||
          p.target_video_codec.toLowerCase().includes(searchLower) ||
          p.target_audio_codec.toLowerCase().includes(searchLower) ||
          p.quality_preset.toLowerCase().includes(searchLower) ||
          p.hw_accel.toLowerCase().includes(searchLower)
      );
    }

    return filtered;
  }, [allProfiles, searchTerm, filterVideoCodec, filterQuality]);

  const handleCreate = async (data: ProfileFormData) => {
    setLoading((prev) => ({ ...prev, create: true }));
    setError((prev) => ({ ...prev, create: null }));
    try {
      await apiClient.createEncodingProfile({
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

  const copyProfileName = async (profileId: string, name: string) => {
    try {
      await navigator.clipboard.writeText(name);
      setCopiedItems((prev) => new Set(prev).add(profileId));
      setTimeout(() => {
        setCopiedItems((prev) => {
          const newSet = new Set(prev);
          newSet.delete(profileId);
          return newSet;
        });
      }, 2000);
    } catch (error) {
      console.error('Failed to copy to clipboard:', error);
    }
  };

  const getCodecLabel = (codec: string, type: 'video' | 'audio') => {
    const codecs = type === 'video' ? VIDEO_CODECS : AUDIO_CODECS;
    return codecs.find((c) => c.value === codec)?.label || codec.toUpperCase();
  };

  const getQualityLabel = (preset: QualityPreset) => {
    return QUALITY_PRESETS.find((p) => p.value === preset)?.label || preset;
  };

  // Statistics
  const totalProfiles = allProfiles.length;
  const systemProfiles = allProfiles.filter((p) => p.is_system).length;
  const hwAccelProfiles = allProfiles.filter((p) => p.hw_accel !== 'none').length;

  return (
    <div className="space-y-6">
      {/* Header Section */}
      <div className="flex items-center justify-between">
        <div>
          <p className="text-muted-foreground">Manage transcoding settings for stream relay output</p>
        </div>
        <ProfileFormSheet
          onSave={handleCreate}
          loading={loading.create}
          error={error.create}
          title="Create Encoding Profile"
          description="Create a new encoding profile for stream transcoding"
          trigger={
            <Button className="gap-2">
              <Plus className="h-4 w-4" />
              Create Profile
            </Button>
          }
        />
      </div>

      {/* Statistics Cards */}
      <div className="grid gap-4 md:grid-cols-3">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Total Profiles</CardTitle>
            <Video className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{totalProfiles}</div>
            <p className="text-xs text-muted-foreground">Encoding configurations</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">System</CardTitle>
            <Lock className="h-4 w-4 text-purple-600" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{systemProfiles}</div>
            <p className="text-xs text-muted-foreground">Built-in profiles</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">HW Accelerated</CardTitle>
            <Zap className="h-4 w-4 text-orange-600" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{hwAccelProfiles}</div>
            <p className="text-xs text-muted-foreground">GPU encoding enabled</p>
          </CardContent>
        </Card>
      </div>

      {/* Search & Filters */}
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
                  placeholder="Search profiles by name, codec, quality..."
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  className="pl-8"
                  disabled={loading.profiles}
                />
              </div>
            </div>
            <Select
              value={filterVideoCodec}
              onValueChange={setFilterVideoCodec}
              disabled={loading.profiles}
            >
              <SelectTrigger className="w-full sm:w-[180px]">
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
              <SelectTrigger className="w-full sm:w-[180px]">
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

            {/* Layout Chooser */}
            <div className="flex rounded-md border">
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    size="sm"
                    variant={viewMode === 'table' ? 'default' : 'ghost'}
                    className="rounded-r-none border-r"
                    onClick={() => setViewMode('table')}
                  >
                    <TableIcon className="w-4 h-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Table View</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    size="sm"
                    variant={viewMode === 'grid' ? 'default' : 'ghost'}
                    className="rounded-none border-r"
                    onClick={() => setViewMode('grid')}
                  >
                    <Grid className="w-4 h-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Grid View</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    size="sm"
                    variant={viewMode === 'list' ? 'default' : 'ghost'}
                    className="rounded-l-none"
                    onClick={() => setViewMode('list')}
                  >
                    <List className="w-4 h-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>List View</TooltipContent>
              </Tooltip>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Error display */}
      {(error.profiles || error.action) && (
        <div className="bg-destructive/10 text-destructive px-4 py-3 rounded-md text-sm">
          {error.profiles || error.action}
        </div>
      )}

      {/* Profiles Display */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center justify-between">
            <span>
              Profiles ({filteredProfiles.length}
              {searchTerm || filterVideoCodec !== 'all' || filterQuality !== 'all'
                ? ` of ${allProfiles.length}`
                : ''})
            </span>
            {loading.profiles && <Loader2 className="h-4 w-4 animate-spin" />}
          </CardTitle>
          <CardDescription>Configure video and audio encoding settings</CardDescription>
        </CardHeader>
        <CardContent>
          {loading.profiles ? (
            <div className="flex items-center justify-center py-12">
              <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
            </div>
          ) : (
            <>
              {/* Table View */}
              {viewMode === 'table' && (
                <div className="space-y-4">
                  {filteredProfiles.map((profile) => (
                    <Card key={profile.id} className="relative">
                      <CardHeader className="pb-3">
                        <div className="flex items-start justify-between">
                          <div className="space-y-2">
                            <div className="flex items-center gap-2">
                              <CardTitle className="text-lg">{profile.name}</CardTitle>
                              {profile.is_system && (
                                <Badge
                                  variant="outline"
                                  className="text-purple-600 border-purple-600"
                                >
                                  <Lock className="h-3 w-3 mr-1" />
                                  System
                                </Badge>
                              )}
                              {profile.is_default && (
                                <Badge variant="default" className="gap-1">
                                  <Star className="h-3 w-3" />
                                  Default
                                </Badge>
                              )}
                            </div>
                            {profile.description && (
                              <CardDescription>{profile.description}</CardDescription>
                            )}
                          </div>
                          <div className="flex items-center gap-2">
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => copyProfileName(profile.id, profile.name)}
                              className="h-8 w-8 p-0"
                              title="Copy name"
                            >
                              {copiedItems.has(profile.id) ? (
                                <Check className="h-4 w-4 text-green-600" />
                              ) : (
                                <Copy className="h-4 w-4" />
                              )}
                            </Button>
                            {!profile.is_system && (
                              <ProfileFormSheet
                                profile={profile}
                                onSave={(data) => handleUpdate(profile.id, data)}
                                loading={loading.edit}
                                error={error.edit}
                                title="Edit Encoding Profile"
                                description="Modify encoding profile settings"
                                trigger={
                                  <Button variant="ghost" size="sm" className="h-8 w-8 p-0" title="Edit profile">
                                    <Edit className="h-4 w-4" />
                                  </Button>
                                }
                              />
                            )}
                            {!profile.is_system && (
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => setDeleteDialog({ open: true, profile })}
                                className="h-8 w-8 p-0 text-destructive hover:text-destructive"
                                title="Delete profile"
                              >
                                <Trash2 className="h-4 w-4" />
                              </Button>
                            )}
                          </div>
                        </div>
                      </CardHeader>
                      <CardContent className="pt-0">
                        <div className="grid grid-cols-4 gap-4 text-sm">
                          <div>
                            <span className="text-muted-foreground">Codecs:</span>{' '}
                            <span className="font-medium">{profile.target_video_codec}/{profile.target_audio_codec}</span>
                          </div>
                          <div>
                            <span className="text-muted-foreground">Quality:</span>{' '}
                            <Badge variant="secondary" className="text-xs ml-1">
                              {getQualityLabel(profile.quality_preset)}
                            </Badge>
                          </div>
                          <div>
                            <span className="text-muted-foreground">HW Accel:</span>{' '}
                            <span className="font-medium">{profile.hw_accel.toUpperCase()}</span>
                          </div>
                          <div>
                            <span className="text-muted-foreground">Created:</span>{' '}
                            <span className="font-medium">{formatRelativeTime(profile.created_at || '')}</span>
                          </div>
                        </div>
                        {!profile.is_default && (
                          <div className="flex items-center gap-2 mt-4 pt-4 border-t">
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => handleSetDefault(profile)}
                              disabled={loading.setDefault === profile.id}
                            >
                              {loading.setDefault === profile.id ? (
                                <Loader2 className="h-4 w-4 animate-spin mr-1" />
                              ) : (
                                <Star className="h-4 w-4 mr-1" />
                              )}
                              Set Default
                            </Button>
                          </div>
                        )}
                      </CardContent>
                    </Card>
                  ))}
                </div>
              )}

              {/* Grid View */}
              {viewMode === 'grid' && (
                <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
                  {filteredProfiles.map((profile) => (
                    <Card
                      key={profile.id}
                      className="relative transition-all hover:shadow-md"
                    >
                      <CardHeader className="pb-2">
                        <div className="flex items-start justify-between">
                          <div className="flex items-center gap-2 flex-wrap">
                            <CardTitle className="text-lg">{profile.name}</CardTitle>
                            {profile.is_system && (
                              <Badge
                                variant="outline"
                                className="text-purple-600 border-purple-600"
                              >
                                <Lock className="h-3 w-3 mr-1" />
                                System
                              </Badge>
                            )}
                            {profile.is_default && (
                              <Badge variant="default" className="gap-1">
                                <Star className="h-3 w-3" />
                                Default
                              </Badge>
                            )}
                          </div>
                        </div>
                        {profile.description && (
                          <CardDescription className="line-clamp-2">{profile.description}</CardDescription>
                        )}
                      </CardHeader>
                      <CardContent className="space-y-4">
                        <div className="text-sm space-y-1">
                          <div>
                            <span className="text-muted-foreground">Codecs:</span>{' '}
                            <span className="font-medium">{profile.target_video_codec}/{profile.target_audio_codec}</span>
                          </div>
                          <div className="flex items-center">
                            <span className="text-muted-foreground">Quality:</span>{' '}
                            <Badge variant="secondary" className="text-xs ml-1">
                              {getQualityLabel(profile.quality_preset)}
                            </Badge>
                          </div>
                          <div>
                            <span className="text-muted-foreground">HW Accel:</span>{' '}
                            <span className="font-medium">{profile.hw_accel.toUpperCase()}</span>
                          </div>
                        </div>

                        <div className="text-xs text-muted-foreground">
                          Created {formatRelativeTime(profile.created_at || '')}
                        </div>

                        <div className="flex items-center gap-2 pt-2 border-t">
                          {!profile.is_default && (
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => handleSetDefault(profile)}
                              disabled={loading.setDefault === profile.id}
                            >
                              {loading.setDefault === profile.id ? (
                                <Loader2 className="h-4 w-4 animate-spin mr-1" />
                              ) : (
                                <Star className="h-4 w-4 mr-1" />
                              )}
                              Default
                            </Button>
                          )}

                          {!profile.is_system && (
                            <ProfileFormSheet
                              profile={profile}
                              onSave={(data) => handleUpdate(profile.id, data)}
                              loading={loading.edit}
                              error={error.edit}
                              title="Edit Encoding Profile"
                              description="Modify encoding profile settings"
                              trigger={
                                <Button variant="outline" size="sm">
                                  <Edit className="h-4 w-4 mr-1" />
                                  Edit
                                </Button>
                              }
                            />
                          )}

                          {!profile.is_system && (
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => setDeleteDialog({ open: true, profile })}
                              className="text-destructive hover:text-destructive"
                            >
                              <Trash2 className="h-4 w-4" />
                            </Button>
                          )}
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              )}

              {/* List View */}
              {viewMode === 'list' && (
                <div className="space-y-2">
                  {filteredProfiles.map((profile) => (
                    <Card key={profile.id} className="transition-all hover:shadow-sm">
                      <CardContent className="pt-4">
                        <div className="flex items-center justify-between">
                          <div className="flex items-center space-x-4 flex-1">
                            <div className="flex-1 min-w-0">
                              <div className="flex items-center gap-3">
                                <div>
                                  <p className="font-medium text-sm">{profile.name}</p>
                                  <p className="text-xs text-muted-foreground">
                                    {profile.description || `${profile.hw_accel.toUpperCase()} acceleration`}
                                  </p>
                                </div>
                                <div className="flex items-center gap-2">
                                  {profile.is_system && (
                                    <Badge
                                      variant="outline"
                                      className="text-xs text-purple-600 border-purple-600"
                                    >
                                      <Lock className="h-3 w-3 mr-1" />
                                      System
                                    </Badge>
                                  )}
                                  {profile.is_default && (
                                    <Badge variant="default" className="text-xs gap-1">
                                      <Star className="h-3 w-3" />
                                      Default
                                    </Badge>
                                  )}
                                  <span className="text-xs text-muted-foreground">
                                    {profile.target_video_codec}/{profile.target_audio_codec}
                                  </span>
                                  <Badge variant="secondary" className="text-xs">
                                    {getQualityLabel(profile.quality_preset)}
                                  </Badge>
                                </div>
                              </div>
                            </div>
                          </div>
                          <div className="flex items-center gap-2 ml-4">
                            <div className="flex items-center gap-1">
                              <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => copyProfileName(profile.id, profile.name)}
                                className="h-8 w-8 p-0"
                                title="Copy name"
                              >
                                {copiedItems.has(profile.id) ? (
                                  <Check className="h-4 w-4 text-green-600" />
                                ) : (
                                  <Copy className="h-4 w-4" />
                                )}
                              </Button>
                              {!profile.is_default && (
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => handleSetDefault(profile)}
                                  disabled={loading.setDefault === profile.id}
                                  className="h-8 w-8 p-0"
                                  title="Set as default"
                                >
                                  {loading.setDefault === profile.id ? (
                                    <Loader2 className="h-4 w-4 animate-spin" />
                                  ) : (
                                    <Star className="h-4 w-4" />
                                  )}
                                </Button>
                              )}
                              {!profile.is_system && (
                                <ProfileFormSheet
                                  profile={profile}
                                  onSave={(data) => handleUpdate(profile.id, data)}
                                  loading={loading.edit}
                                  error={error.edit}
                                  title="Edit Encoding Profile"
                                  description="Modify encoding profile settings"
                                  trigger={
                                    <Button variant="ghost" size="sm" className="h-8 w-8 p-0" title="Edit profile">
                                      <Edit className="h-4 w-4" />
                                    </Button>
                                  }
                                />
                              )}
                              {!profile.is_system && (
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onClick={() => setDeleteDialog({ open: true, profile })}
                                  className="h-8 w-8 p-0 text-destructive hover:text-destructive"
                                  title="Delete profile"
                                >
                                  <Trash2 className="h-4 w-4" />
                                </Button>
                              )}
                            </div>
                          </div>
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              )}

              {/* Empty state */}
              {filteredProfiles.length === 0 && !loading.profiles && (
                <div className="text-center py-12">
                  <Video className="mx-auto h-12 w-12 text-muted-foreground" />
                  <h3 className="mt-4 text-lg font-semibold">
                    {searchTerm || filterVideoCodec !== 'all' || filterQuality !== 'all'
                      ? 'No matching profiles'
                      : 'No encoding profiles yet'}
                  </h3>
                  <p className="text-muted-foreground">
                    {searchTerm || filterVideoCodec !== 'all' || filterQuality !== 'all'
                      ? 'Try adjusting your search or filter criteria.'
                      : 'Get started by creating your first encoding profile.'}
                  </p>
                </div>
              )}
            </>
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
  );
}
