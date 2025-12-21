'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Switch } from '@/components/ui/switch';
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
  Lock,
  Settings2,
  ArrowUp,
  ArrowDown,
  AlertCircle,
  Cpu,
  Video,
  Music,
} from 'lucide-react';
import {
  EncoderOverride,
  EncoderOverrideCreateRequest,
  EncoderOverrideUpdateRequest,
  EncoderOverrideCodecType,
} from '@/types/api';
import { apiClient, ApiError } from '@/lib/api-client';
import { createFuzzyFilter } from '@/lib/fuzzy-search';
import {
  MasterDetailLayout,
  DetailPanel,
  DetailEmpty,
  MasterItem,
} from '@/components/shared';
import { StatCard } from '@/components/shared/feedback/StatCard';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';

interface LoadingState {
  overrides: boolean;
  create: boolean;
  edit: boolean;
  delete: string | null;
  toggle: string | null;
  reorder: boolean;
}

interface ErrorState {
  overrides: string | null;
  create: string | null;
  edit: string | null;
  action: string | null;
}

interface OverrideFormData {
  name: string;
  description: string;
  codec_type: EncoderOverrideCodecType;
  source_codec: string;
  target_encoder: string;
  hw_accel_match: string;
  cpu_match: string;
  priority: number;
  is_enabled: boolean;
}

const VIDEO_CODECS = ['h264', 'h265', 'vp9', 'av1'];
const AUDIO_CODECS = ['aac', 'ac3', 'eac3', 'opus', 'mp3'];
// Use "_any" as placeholder since Select.Item can't have empty string value
const HW_ACCEL_ANY = '_any';
const HW_ACCEL_OPTIONS = [
  { value: HW_ACCEL_ANY, label: 'Any (no filter)' },
  { value: 'vaapi', label: 'VAAPI' },
  { value: 'cuda', label: 'CUDA (NVENC)' },
  { value: 'qsv', label: 'QSV (Intel)' },
  { value: 'videotoolbox', label: 'VideoToolbox (macOS)' },
  { value: 'amf', label: 'AMF (AMD Windows)' },
];

// Convert between UI value and API value for hw_accel_match
const hwAccelToUI = (value: string): string => value || HW_ACCEL_ANY;
const hwAccelToAPI = (value: string): string => value === HW_ACCEL_ANY ? '' : value;

const defaultFormData: OverrideFormData = {
  name: '',
  description: '',
  codec_type: 'video',
  source_codec: 'h265',
  target_encoder: '',
  hw_accel_match: HW_ACCEL_ANY,
  cpu_match: '',
  priority: 0, // Auto-assigned by server
  is_enabled: true,
};

// Convert EncoderOverride to MasterItem format
interface EncoderOverrideMasterItem extends MasterItem {
  override: EncoderOverride;
}

function encoderOverrideToMasterItem(override: EncoderOverride): EncoderOverrideMasterItem {
  return {
    id: override.id,
    title: override.name,
    enabled: override.is_enabled,
    override,
  };
}

// Create panel for new override
function EncoderOverrideCreatePanel({
  onCreate,
  onCancel,
  loading,
  error,
}: {
  onCreate: (data: OverrideFormData) => Promise<void>;
  onCancel: () => void;
  loading: boolean;
  error: string | null;
}) {
  const [formData, setFormData] = useState<OverrideFormData>({ ...defaultFormData });

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    await onCreate(formData);
  };

  const codecOptions = formData.codec_type === 'video' ? VIDEO_CODECS : AUDIO_CODECS;

  return (
    <DetailPanel
      title="Create Encoder Override"
      actions={
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={onCancel} disabled={loading}>
            Cancel
          </Button>
          <Button size="sm" onClick={handleSubmit} disabled={loading || !formData.name.trim() || !formData.target_encoder.trim()}>
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
        <div className="space-y-2">
          <Label htmlFor="create-name">Name *</Label>
          <Input
            id="create-name"
            value={formData.name}
            onChange={(e) => setFormData({ ...formData, name: e.target.value })}
            placeholder="e.g., Force software H.265 encoder"
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
            placeholder="Optional: explain why this override is needed"
            rows={2}
            disabled={loading}
          />
        </div>

        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label>Codec Type</Label>
            <Select
              value={formData.codec_type}
              onValueChange={(value: EncoderOverrideCodecType) => setFormData({ ...formData, codec_type: value, source_codec: value === 'video' ? 'h265' : 'aac' })}
              disabled={loading}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="video">Video</SelectItem>
                <SelectItem value="audio">Audio</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2">
            <Label>Target Codec</Label>
            <Select
              value={formData.source_codec}
              onValueChange={(value) => setFormData({ ...formData, source_codec: value })}
              disabled={loading}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {codecOptions.map((codec) => (
                  <SelectItem key={codec} value={codec}>{codec.toUpperCase()}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">Codec to match when encoding</p>
          </div>
        </div>

        <div className="space-y-2">
          <Label htmlFor="create-target">Override Encoder *</Label>
          <Input
            id="create-target"
            value={formData.target_encoder}
            onChange={(e) => setFormData({ ...formData, target_encoder: e.target.value })}
            placeholder="e.g., libx265, h264_nvenc, libopus"
            disabled={loading}
            required
          />
          <p className="text-xs text-muted-foreground">FFmpeg encoder to use instead of the auto-selected one</p>
        </div>

        <div className="space-y-2">
          <Label>HW Accel Match (optional, video only)</Label>
          <Select
            value={formData.hw_accel_match}
            onValueChange={(value) => setFormData({ ...formData, hw_accel_match: value })}
            disabled={loading || formData.codec_type !== 'video'}
          >
            <SelectTrigger>
              <SelectValue placeholder="Any (no filter)" />
            </SelectTrigger>
            <SelectContent>
              {HW_ACCEL_OPTIONS.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>{opt.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-2">
          <Label htmlFor="create-cpu">CPU Match (optional regex)</Label>
          <Input
            id="create-cpu"
            value={formData.cpu_match}
            onChange={(e) => setFormData({ ...formData, cpu_match: e.target.value })}
            placeholder="AMD|Advanced Micro Devices"
            disabled={loading}
          />
          <p className="text-xs text-muted-foreground">Regex pattern to match against CPU vendor/model (empty matches all)</p>
        </div>

        <div className="flex items-center gap-3 pt-2">
          <Switch
            id="create-enabled"
            checked={formData.is_enabled}
            onCheckedChange={(checked) => setFormData({ ...formData, is_enabled: checked })}
            disabled={loading}
          />
          <Label htmlFor="create-enabled" className="cursor-pointer">
            {formData.is_enabled ? 'Enabled' : 'Disabled'}
          </Label>
        </div>
      </form>
    </DetailPanel>
  );
}

// Detail panel for viewing/editing an override
function EncoderOverrideDetailPanel({
  override,
  onUpdate,
  onDelete,
  onToggle,
  onMoveUp,
  onMoveDown,
  loading,
  error,
  isFirst,
  isLast,
}: {
  override: EncoderOverride;
  onUpdate: (id: string, data: OverrideFormData) => Promise<void>;
  onDelete: (override: EncoderOverride) => Promise<void>;
  onToggle: (override: EncoderOverride) => Promise<void>;
  onMoveUp: (id: string) => void;
  onMoveDown: (id: string) => void;
  loading: { edit: boolean; delete: string | null; toggle: string | null; reorder: boolean };
  error: string | null;
  isFirst: boolean;
  isLast: boolean;
}) {
  const [formData, setFormData] = useState<OverrideFormData>({
    name: override.name,
    description: override.description || '',
    codec_type: override.codec_type,
    source_codec: override.source_codec,
    target_encoder: override.target_encoder,
    hw_accel_match: hwAccelToUI(override.hw_accel_match || ''),
    cpu_match: override.cpu_match || '',
    priority: override.priority,
    is_enabled: override.is_enabled,
  });
  const [hasChanges, setHasChanges] = useState(false);

  // Reset form when override changes
  useEffect(() => {
    setFormData({
      name: override.name,
      description: override.description || '',
      codec_type: override.codec_type,
      source_codec: override.source_codec,
      target_encoder: override.target_encoder,
      hw_accel_match: hwAccelToUI(override.hw_accel_match || ''),
      cpu_match: override.cpu_match || '',
      priority: override.priority,
      is_enabled: override.is_enabled,
    });
    setHasChanges(false);
  }, [override.id]);

  const handleFieldChange = (field: keyof OverrideFormData, value: any) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
    setHasChanges(true);
  };

  const handleSave = async () => {
    await onUpdate(override.id, formData);
    setHasChanges(false);
  };

  const codecOptions = formData.codec_type === 'video' ? VIDEO_CODECS : AUDIO_CODECS;
  const isSystem = override.is_system;

  return (
    <DetailPanel
      title={override.name}
      actions={
        <div className="flex items-center gap-1">
          <div className="flex items-center gap-1.5 mr-2 px-2 py-1 rounded-md bg-muted/50">
            <Switch
              id={`toggle-enabled-${override.id}`}
              checked={override.is_enabled}
              onCheckedChange={() => onToggle(override)}
              disabled={loading.toggle === override.id}
              className="h-4 w-7 data-[state=checked]:bg-primary data-[state=unchecked]:bg-input"
            />
            <label htmlFor={`toggle-enabled-${override.id}`} className="text-xs text-muted-foreground cursor-pointer">
              {override.is_enabled ? 'Enabled' : 'Disabled'}
            </label>
          </div>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => onMoveUp(override.id)}
            disabled={isFirst || loading.reorder}
            title="Move up (higher priority)"
          >
            <ArrowUp className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => onMoveDown(override.id)}
            disabled={isLast || loading.reorder}
            title="Move down (lower priority)"
          >
            <ArrowDown className="h-4 w-4" />
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => onDelete(override)}
            disabled={loading.delete === override.id || isSystem}
            className="text-destructive hover:text-destructive"
            title={isSystem ? "System overrides cannot be deleted" : "Delete override"}
          >
            {loading.delete === override.id ? (
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
            <AlertTitle>System Override</AlertTitle>
            <AlertDescription>
              This is a system override. You can enable/disable it and change its order, but cannot modify or delete it.
            </AlertDescription>
          </Alert>
        )}

        {/* Compact Status Display */}
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <Badge variant="secondary">
            {override.codec_type === 'video' ? 'Video' : 'Audio'}
          </Badge>
          <Badge variant="outline">
            {override.source_codec.toUpperCase()} → {override.target_encoder}
          </Badge>
        </div>

        <div className="space-y-2">
          <Label htmlFor="detail-name">Name</Label>
          <Input
            id="detail-name"
            value={formData.name}
            onChange={(e) => handleFieldChange('name', e.target.value)}
            disabled={loading.edit || isSystem}
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

        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label>Codec Type</Label>
            <Select
              value={formData.codec_type}
              onValueChange={(value: EncoderOverrideCodecType) => {
                handleFieldChange('codec_type', value);
                handleFieldChange('source_codec', value === 'video' ? 'h265' : 'aac');
              }}
              disabled={loading.edit || isSystem}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="video">Video</SelectItem>
                <SelectItem value="audio">Audio</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2">
            <Label>Target Codec</Label>
            <Select
              value={formData.source_codec}
              onValueChange={(value) => handleFieldChange('source_codec', value)}
              disabled={loading.edit || isSystem}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {codecOptions.map((codec) => (
                  <SelectItem key={codec} value={codec}>{codec.toUpperCase()}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>

        <div className="space-y-2">
          <Label htmlFor="detail-target">Override Encoder</Label>
          <Input
            id="detail-target"
            value={formData.target_encoder}
            onChange={(e) => handleFieldChange('target_encoder', e.target.value)}
            disabled={loading.edit || isSystem}
          />
        </div>

        <div className="space-y-2">
          <Label>HW Accel Match</Label>
          <Select
            value={formData.hw_accel_match}
            onValueChange={(value) => handleFieldChange('hw_accel_match', value)}
            disabled={loading.edit || isSystem || formData.codec_type !== 'video'}
          >
            <SelectTrigger>
              <SelectValue placeholder="Any (no filter)" />
            </SelectTrigger>
            <SelectContent>
              {HW_ACCEL_OPTIONS.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>{opt.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-2">
          <Label htmlFor="detail-cpu">CPU Match (regex)</Label>
          <Input
            id="detail-cpu"
            value={formData.cpu_match}
            onChange={(e) => handleFieldChange('cpu_match', e.target.value)}
            placeholder="AMD|Advanced Micro Devices"
            disabled={loading.edit || isSystem}
          />
        </div>

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

export function EncoderOverrides() {
  const [allOverrides, setAllOverrides] = useState<EncoderOverride[]>([]);
  const [selectedOverride, setSelectedOverride] = useState<EncoderOverrideMasterItem | null>(null);
  const [loading, setLoading] = useState<LoadingState>({
    overrides: true,
    create: false,
    edit: false,
    delete: null,
    toggle: null,
    reorder: false,
  });
  const [errors, setErrors] = useState<ErrorState>({
    overrides: null,
    create: null,
    edit: null,
    action: null,
  });
  const [deleteDialog, setDeleteDialog] = useState<{ open: boolean; override: EncoderOverride | null }>({
    open: false,
    override: null,
  });
  const [isCreating, setIsCreating] = useState(false);

  const loadOverrides = useCallback(async () => {
    setLoading((prev) => ({ ...prev, overrides: true }));
    setErrors((prev) => ({ ...prev, overrides: null }));
    try {
      const data = await apiClient.getEncoderOverrides();
      setAllOverrides(data);
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to load encoder overrides';
      setErrors((prev) => ({ ...prev, overrides: message }));
    } finally {
      setLoading((prev) => ({ ...prev, overrides: false }));
    }
  }, []);

  useEffect(() => {
    loadOverrides();
  }, [loadOverrides]);

  // Sort by priority descending (higher = first)
  const sortedOverrides = useMemo(() => {
    return [...allOverrides].sort((a, b) => b.priority - a.priority);
  }, [allOverrides]);

  const masterItems = useMemo(
    () => sortedOverrides.map(encoderOverrideToMasterItem),
    [sortedOverrides]
  );

  const handleCreate = async (data: OverrideFormData) => {
    setLoading((prev) => ({ ...prev, create: true }));
    setErrors((prev) => ({ ...prev, create: null }));
    try {
      await apiClient.createEncoderOverride({
        name: data.name,
        description: data.description || undefined,
        codec_type: data.codec_type,
        source_codec: data.source_codec,
        target_encoder: data.target_encoder,
        hw_accel_match: hwAccelToAPI(data.hw_accel_match) || undefined,
        cpu_match: data.cpu_match || undefined,
        priority: data.priority,
        is_enabled: data.is_enabled,
      });
      await loadOverrides();
      setIsCreating(false);
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to create override';
      setErrors((prev) => ({ ...prev, create: message }));
      throw err;
    } finally {
      setLoading((prev) => ({ ...prev, create: false }));
    }
  };

  const handleUpdate = async (id: string, data: OverrideFormData) => {
    setLoading((prev) => ({ ...prev, edit: true }));
    setErrors((prev) => ({ ...prev, edit: null }));
    try {
      await apiClient.updateEncoderOverride(id, {
        name: data.name,
        description: data.description || undefined,
        codec_type: data.codec_type,
        source_codec: data.source_codec,
        target_encoder: data.target_encoder,
        hw_accel_match: hwAccelToAPI(data.hw_accel_match) || undefined,
        cpu_match: data.cpu_match || undefined,
        priority: data.priority,
        is_enabled: data.is_enabled,
      });
      await loadOverrides();
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to update override';
      setErrors((prev) => ({ ...prev, edit: message }));
      throw err;
    } finally {
      setLoading((prev) => ({ ...prev, edit: false }));
    }
  };

  const handleDelete = async (override: EncoderOverride) => {
    setLoading((prev) => ({ ...prev, delete: override.id }));
    setErrors((prev) => ({ ...prev, action: null }));
    try {
      await apiClient.deleteEncoderOverride(override.id);
      await loadOverrides();
      if (selectedOverride?.id === override.id) {
        setSelectedOverride(null);
      }
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to delete override';
      setErrors((prev) => ({ ...prev, action: message }));
    } finally {
      setLoading((prev) => ({ ...prev, delete: null }));
      setDeleteDialog({ open: false, override: null });
    }
  };

  const handleToggle = async (override: EncoderOverride) => {
    setLoading((prev) => ({ ...prev, toggle: override.id }));
    setErrors((prev) => ({ ...prev, action: null }));
    try {
      await apiClient.toggleEncoderOverride(override.id);
      await loadOverrides();
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to toggle override';
      setErrors((prev) => ({ ...prev, action: message }));
    } finally {
      setLoading((prev) => ({ ...prev, toggle: null }));
    }
  };

  const moveOverride = async (id: string, direction: 'up' | 'down') => {
    const currentIndex = sortedOverrides.findIndex((o) => o.id === id);
    if (currentIndex === -1) return;

    const targetIndex = direction === 'up' ? currentIndex - 1 : currentIndex + 1;
    if (targetIndex < 0 || targetIndex >= sortedOverrides.length) return;

    setLoading((prev) => ({ ...prev, reorder: true }));
    setErrors((prev) => ({ ...prev, action: null }));

    try {
      const newOrder = [...sortedOverrides];
      [newOrder[currentIndex], newOrder[targetIndex]] = [newOrder[targetIndex], newOrder[currentIndex]];

      // Assign descending priorities (higher = first)
      const reorderRequest = newOrder.map((o, index) => ({
        id: o.id,
        priority: (newOrder.length - index) * 10,
      }));

      await apiClient.reorderEncoderOverrides(reorderRequest);
      await loadOverrides();
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to reorder overrides';
      setErrors((prev) => ({ ...prev, action: message }));
    } finally {
      setLoading((prev) => ({ ...prev, reorder: false }));
    }
  };

  const handleDragReorder = async (reorderedIds: string[]) => {
    setLoading((prev) => ({ ...prev, reorder: true }));
    setErrors((prev) => ({ ...prev, action: null }));

    try {
      const reorderRequest = reorderedIds.map((id, index) => ({
        id,
        priority: (reorderedIds.length - index) * 10,
      }));

      await apiClient.reorderEncoderOverrides(reorderRequest);
      await loadOverrides();
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to reorder overrides';
      setErrors((prev) => ({ ...prev, action: message }));
    } finally {
      setLoading((prev) => ({ ...prev, reorder: false }));
    }
  };

  // Stats
  const totalOverrides = allOverrides.length;
  const enabledOverrides = allOverrides.filter((o) => o.is_enabled).length;
  const systemOverrides = allOverrides.filter((o) => o.is_system).length;
  const videoOverrides = allOverrides.filter((o) => o.codec_type === 'video').length;

  return (
    <div className="flex flex-col gap-6 h-full">
      {/* Header */}
      <div>
        <p className="text-muted-foreground">
          Configure encoder override rules to force specific encoders when hardware encoders are broken or unsupported
        </p>
      </div>

      {/* Stats */}
      <div className="grid gap-2 md:grid-cols-4">
        <StatCard
          title="Total Overrides"
          value={totalOverrides}
          icon={<Settings2 className="h-4 w-4" />}
        />
        <StatCard
          title="Enabled"
          value={enabledOverrides}
          icon={<Cpu className="h-4 w-4 text-green-600" />}
        />
        <StatCard
          title="Video"
          value={videoOverrides}
          icon={<Video className="h-4 w-4 text-blue-600" />}
        />
        <StatCard
          title="System"
          value={systemOverrides}
          icon={<Lock className="h-4 w-4 text-purple-600" />}
        />
      </div>

      {/* Error Display */}
      {errors.action && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>{errors.action}</AlertDescription>
        </Alert>
      )}

      {/* MasterDetailLayout */}
      <Card className="flex-1 overflow-hidden min-h-0">
        <CardContent className="p-0 h-full">
          {errors.overrides ? (
            <div className="p-6">
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertTitle>Failed to Load Overrides</AlertTitle>
                <AlertDescription>
                  {errors.overrides}
                  <Button
                    variant="outline"
                    size="sm"
                    className="ml-2"
                    onClick={loadOverrides}
                    disabled={loading.overrides}
                  >
                    {loading.overrides && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                    Retry
                  </Button>
                </AlertDescription>
              </Alert>
            </div>
          ) : (
            <MasterDetailLayout
              items={masterItems}
              selectedId={isCreating ? null : selectedOverride?.id}
              onSelect={(item) => {
                setIsCreating(false);
                setSelectedOverride(item);
              }}
              isLoading={loading.overrides}
              title={`Encoder Overrides (${sortedOverrides.length})`}
              searchPlaceholder="Search by name, codec..."
              storageKey="encoder-overrides"
              sortable={true}
              onReorder={handleDragReorder}
              headerAction={
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    setIsCreating(true);
                    setSelectedOverride(null);
                    setErrors((prev) => ({ ...prev, create: null }));
                  }}
                  disabled={loading.overrides}
                >
                  <Plus className="h-4 w-4" />
                  <span className="sr-only">Create Override</span>
                </Button>
              }
              emptyState={{
                title: 'No encoder overrides configured',
                description: 'Create an override to force specific encoders when conditions match.',
              }}
              filterFn={createFuzzyFilter<EncoderOverrideMasterItem>({
                keys: [
                  { name: 'name', weight: 0.4 },
                  { name: 'description', weight: 0.2 },
                  { name: 'source_codec', weight: 0.15 },
                  { name: 'target_encoder', weight: 0.15 },
                  { name: 'codec_type', weight: 0.1 },
                ],
                accessor: (item) => ({
                  name: item.override.name,
                  description: item.override.description || '',
                  source_codec: item.override.source_codec,
                  target_encoder: item.override.target_encoder,
                  codec_type: item.override.codec_type,
                }),
              })}
            >
              {(selected) =>
                isCreating ? (
                  <EncoderOverrideCreatePanel
                    onCreate={handleCreate}
                    onCancel={() => setIsCreating(false)}
                    loading={loading.create}
                    error={errors.create}
                  />
                ) : selected ? (
                  <EncoderOverrideDetailPanel
                    override={selected.override}
                    onUpdate={handleUpdate}
                    onDelete={async (o) => setDeleteDialog({ open: true, override: o })}
                    onToggle={handleToggle}
                    onMoveUp={(id) => moveOverride(id, 'up')}
                    onMoveDown={(id) => moveOverride(id, 'down')}
                    loading={{ edit: loading.edit, delete: loading.delete, toggle: loading.toggle, reorder: loading.reorder }}
                    error={errors.edit}
                    isFirst={sortedOverrides.findIndex((o) => o.id === selected.override.id) === 0}
                    isLast={sortedOverrides.findIndex((o) => o.id === selected.override.id) === sortedOverrides.length - 1}
                  />
                ) : (
                  <DetailEmpty
                    icon={<Settings2 className="h-12 w-12" />}
                    title="Select an Encoder Override"
                    description="Choose an override from the list to view and edit its configuration."
                  />
                )
              }
            </MasterDetailLayout>
          )}
        </CardContent>
      </Card>

      {/* Delete Confirmation Dialog */}
      <Dialog open={deleteDialog.open} onOpenChange={(open) => setDeleteDialog({ open, override: null })}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Encoder Override</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete &quot;{deleteDialog.override?.name}&quot;? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteDialog({ open: false, override: null })}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => deleteDialog.override && handleDelete(deleteDialog.override)}
              disabled={loading.delete !== null}
            >
              {loading.delete && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
