'use client';

import { useState } from 'react';
import { ChevronDown, ChevronRight } from 'lucide-react';
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
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import {
  RelayProfile,
  CreateRelayProfileRequest,
  UpdateRelayProfileRequest,
} from '@/types/api';

interface RelayProfileFormProps {
  profile?: RelayProfile;
  onSubmit: (data: CreateRelayProfileRequest | UpdateRelayProfileRequest) => void;
  onCancel: () => void;
  formId: string;
  loading?: boolean;
}

// Backend uses FFmpeg codec names
const VIDEO_CODECS = [
  { value: 'libx264', label: 'H.264' },
  { value: 'libx265', label: 'H.265/HEVC' },
  { value: 'libaom-av1', label: 'AV1' },
  { value: 'libvpx-vp9', label: 'VP9' },
  { value: 'copy', label: 'Copy (No transcode)' },
];

const AUDIO_CODECS = [
  { value: 'aac', label: 'AAC' },
  { value: 'libmp3lame', label: 'MP3' },
  { value: 'ac3', label: 'AC3' },
  { value: 'eac3', label: 'EAC3' },
  { value: 'libopus', label: 'Opus' },
  { value: 'copy', label: 'Copy (No transcode)' },
];

const VIDEO_PRESETS = [
  { value: 'ultrafast', label: 'Ultra Fast' },
  { value: 'superfast', label: 'Super Fast' },
  { value: 'veryfast', label: 'Very Fast' },
  { value: 'faster', label: 'Faster' },
  { value: 'fast', label: 'Fast' },
  { value: 'medium', label: 'Medium' },
  { value: 'slow', label: 'Slow' },
  { value: 'slower', label: 'Slower' },
  { value: 'veryslow', label: 'Very Slow' },
];

const OUTPUT_FORMATS = [
  { value: 'mpegts', label: 'MPEG-TS' },
  { value: 'hls', label: 'HLS' },
  { value: 'flv', label: 'FLV' },
  { value: 'matroska', label: 'Matroska (MKV)' },
  { value: 'mp4', label: 'MP4' },
];

const HWACCEL_OPTIONS = [
  { value: 'auto', label: 'Auto (Detect Best)' },
  { value: 'none', label: 'None (Software Only)' },
  { value: 'vaapi', label: 'VA-API (Linux)' },
  { value: 'cuda', label: 'NVIDIA CUDA' },
  { value: 'qsv', label: 'Intel Quick Sync' },
  { value: 'videotoolbox', label: 'VideoToolbox (macOS)' },
];

export function RelayProfileForm({
  profile,
  onSubmit,
  onCancel,
  formId,
  loading,
}: RelayProfileFormProps) {
  // Default values should match GORM defaults in the backend model
  const [formData, setFormData] = useState({
    name: profile?.name || '',
    description: profile?.description || '',
    video_codec: profile?.video_codec || 'copy',        // GORM default: 'copy'
    audio_codec: profile?.audio_codec || 'copy',        // GORM default: 'copy'
    video_preset: profile?.video_preset || '',
    video_bitrate: profile?.video_bitrate?.toString() || '',
    video_maxrate: profile?.video_maxrate?.toString() || '',
    video_width: profile?.video_width?.toString() || '',
    video_height: profile?.video_height?.toString() || '',
    audio_bitrate: profile?.audio_bitrate?.toString() || '',
    audio_sample_rate: profile?.audio_sample_rate?.toString() || '',
    audio_channels: profile?.audio_channels?.toString() || '',
    hw_accel: profile?.hw_accel || 'auto',              // GORM default: 'auto'
    hw_accel_device: profile?.hw_accel_device || '',
    hw_accel_output_format: profile?.hw_accel_output_format || '',
    hw_accel_decoder_codec: profile?.hw_accel_decoder_codec || '',
    hw_accel_extra_options: profile?.hw_accel_extra_options || '',
    gpu_index: profile?.gpu_index?.toString() || '',
    input_options: profile?.input_options || '',
    output_options: profile?.output_options || '',
    filter_complex: profile?.filter_complex || '',
    output_format: profile?.output_format || 'mpegts',  // GORM default: 'mpegts'
    fallback_enabled: profile?.fallback_enabled ?? true,  // GORM default: true
    fallback_error_threshold: profile?.fallback_error_threshold?.toString() || '3',
    fallback_recovery_interval: profile?.fallback_recovery_interval?.toString() || '30',  // GORM default: 30
  });

  const [isSubmitting, setIsSubmitting] = useState(false);
  const [showAdvancedVideo, setShowAdvancedVideo] = useState(
    !!(profile?.video_maxrate || profile?.video_width || profile?.video_height)
  );
  const [showHwAccelAdvanced, setShowHwAccelAdvanced] = useState(
    !!(profile?.hw_accel_device || profile?.hw_accel_output_format || profile?.hw_accel_decoder_codec)
  );
  const [showCustomFlags, setShowCustomFlags] = useState(
    !!(profile?.input_options || profile?.output_options || profile?.filter_complex)
  );
  const [showFallback, setShowFallback] = useState(profile?.fallback_enabled ?? true);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    // Validate mandatory fields
    if (!formData.name.trim()) {
      alert('Profile name is required');
      return;
    }

    setIsSubmitting(true);

    try {
      const data: CreateRelayProfileRequest | UpdateRelayProfileRequest = {
        name: formData.name,
        description: formData.description || undefined,
        video_codec: formData.video_codec || undefined,
        audio_codec: formData.audio_codec || undefined,
        video_preset: formData.video_preset || undefined,
        video_bitrate: formData.video_bitrate ? parseInt(formData.video_bitrate) : undefined,
        video_maxrate: formData.video_maxrate ? parseInt(formData.video_maxrate) : undefined,
        video_width: formData.video_width ? parseInt(formData.video_width) : undefined,
        video_height: formData.video_height ? parseInt(formData.video_height) : undefined,
        audio_bitrate: formData.audio_bitrate ? parseInt(formData.audio_bitrate) : undefined,
        audio_sample_rate: formData.audio_sample_rate
          ? parseInt(formData.audio_sample_rate)
          : undefined,
        audio_channels: formData.audio_channels ? parseInt(formData.audio_channels) : undefined,
        hw_accel: formData.hw_accel || undefined,
        hw_accel_device: formData.hw_accel_device || undefined,
        hw_accel_output_format: formData.hw_accel_output_format || undefined,
        hw_accel_decoder_codec: formData.hw_accel_decoder_codec || undefined,
        hw_accel_extra_options: formData.hw_accel_extra_options || undefined,
        gpu_index: formData.gpu_index ? parseInt(formData.gpu_index) : undefined,
        input_options: formData.input_options || undefined,
        output_options: formData.output_options || undefined,
        filter_complex: formData.filter_complex || undefined,
        output_format: formData.output_format || undefined,
        fallback_enabled: formData.fallback_enabled,
        fallback_error_threshold: formData.fallback_error_threshold
          ? parseInt(formData.fallback_error_threshold)
          : undefined,
        fallback_recovery_interval: formData.fallback_recovery_interval
          ? parseInt(formData.fallback_recovery_interval)
          : undefined,
      };

      await onSubmit(data);
    } finally {
      setIsSubmitting(false);
    }
  };

  const isVideoTranscode = formData.video_codec !== 'copy';
  const isAudioTranscode = formData.audio_codec !== 'copy';

  return (
    <form id={formId} onSubmit={handleSubmit} className="space-y-4 px-4">
      {/* Basic Information */}
      <div className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="name" className="font-bold">
            Profile Name
          </Label>
          <Input
            id="name"
            value={formData.name}
            onChange={(e) => setFormData({ ...formData, name: e.target.value })}
            placeholder="e.g., HD Transcode Profile"
            required
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="description">Description</Label>
          <Textarea
            id="description"
            value={formData.description}
            onChange={(e) => setFormData({ ...formData, description: e.target.value })}
            placeholder="Optional description of what this profile is used for"
            rows={2}
          />
        </div>
      </div>

      {/* Video Settings */}
      <div className="space-y-4">
        <div className="grid gap-4 grid-cols-1 md:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="video_codec" className="font-bold">
              Video Codec
            </Label>
            <Select
              value={formData.video_codec}
              onValueChange={(value) =>
                setFormData({ ...formData, video_codec: value })
              }
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

          {isVideoTranscode && (
            <div className="space-y-2">
              <Label htmlFor="video_bitrate" className="italic">
                Video Bitrate (kbps)
              </Label>
              <Input
                id="video_bitrate"
                type="number"
                value={formData.video_bitrate}
                onChange={(e) => setFormData({ ...formData, video_bitrate: e.target.value })}
                placeholder="e.g., 2000"
              />
            </div>
          )}
        </div>

        {isVideoTranscode && (
          <div className="space-y-2">
            <Label htmlFor="video_preset" className="italic">
              Video Preset
            </Label>
            <Select
              value={formData.video_preset}
              onValueChange={(value) => setFormData({ ...formData, video_preset: value })}
            >
              <SelectTrigger>
                <SelectValue placeholder="Select preset" />
              </SelectTrigger>
              <SelectContent>
                {VIDEO_PRESETS.map((preset) => (
                  <SelectItem key={preset.value} value={preset.value}>
                    {preset.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}

        {/* Advanced Video Settings */}
        {isVideoTranscode && (
          <Collapsible open={showAdvancedVideo} onOpenChange={setShowAdvancedVideo}>
            <CollapsibleTrigger className="flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground transition-colors">
              {showAdvancedVideo ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
              Advanced Video Settings
            </CollapsibleTrigger>
            <CollapsibleContent className="space-y-4 pt-4">
              <div className="grid gap-4 grid-cols-1 md:grid-cols-3">
                <div className="space-y-2">
                  <Label htmlFor="video_maxrate" className="italic">
                    Max Bitrate (kbps)
                  </Label>
                  <Input
                    id="video_maxrate"
                    type="number"
                    value={formData.video_maxrate}
                    onChange={(e) => setFormData({ ...formData, video_maxrate: e.target.value })}
                    placeholder="e.g., 4000"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="video_width" className="italic">
                    Width (pixels)
                  </Label>
                  <Input
                    id="video_width"
                    type="number"
                    value={formData.video_width}
                    onChange={(e) => setFormData({ ...formData, video_width: e.target.value })}
                    placeholder="e.g., 1920"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="video_height" className="italic">
                    Height (pixels)
                  </Label>
                  <Input
                    id="video_height"
                    type="number"
                    value={formData.video_height}
                    onChange={(e) => setFormData({ ...formData, video_height: e.target.value })}
                    placeholder="e.g., 1080"
                  />
                </div>
              </div>
            </CollapsibleContent>
          </Collapsible>
        )}
      </div>

      {/* Audio Settings */}
      <div className="space-y-4">
        <div className="grid gap-4 grid-cols-1 md:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="audio_codec" className="font-bold">
              Audio Codec
            </Label>
            <Select
              value={formData.audio_codec}
              onValueChange={(value) =>
                setFormData({ ...formData, audio_codec: value })
              }
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

          {isAudioTranscode && (
            <div className="space-y-2">
              <Label htmlFor="audio_bitrate" className="italic">
                Audio Bitrate (kbps)
              </Label>
              <Input
                id="audio_bitrate"
                type="number"
                value={formData.audio_bitrate}
                onChange={(e) => setFormData({ ...formData, audio_bitrate: e.target.value })}
                placeholder="e.g., 128"
              />
            </div>
          )}
        </div>

        {isAudioTranscode && (
          <div className="grid gap-4 grid-cols-1 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="audio_sample_rate" className="italic">
                Sample Rate (Hz)
              </Label>
              <Input
                id="audio_sample_rate"
                type="number"
                value={formData.audio_sample_rate}
                onChange={(e) => setFormData({ ...formData, audio_sample_rate: e.target.value })}
                placeholder="e.g., 48000"
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="audio_channels" className="italic">
                Audio Channels
              </Label>
              <Input
                id="audio_channels"
                type="number"
                value={formData.audio_channels}
                onChange={(e) => setFormData({ ...formData, audio_channels: e.target.value })}
                placeholder="e.g., 2"
              />
            </div>
          </div>
        )}
      </div>

      {/* Hardware Acceleration */}
      <div className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="hw_accel">Hardware Acceleration</Label>
          <Select
            value={formData.hw_accel}
            onValueChange={(value) => setFormData({ ...formData, hw_accel: value })}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {HWACCEL_OPTIONS.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {/* Advanced Hardware Acceleration Settings */}
        {formData.hw_accel !== 'none' && (
          <Collapsible open={showHwAccelAdvanced} onOpenChange={setShowHwAccelAdvanced}>
            <CollapsibleTrigger className="flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground transition-colors">
              {showHwAccelAdvanced ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
              Advanced Hardware Acceleration Settings
            </CollapsibleTrigger>
            <CollapsibleContent className="space-y-4 pt-4">
              <div className="grid gap-4 grid-cols-1 md:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="hw_accel_device" className="italic">
                    Device Path
                  </Label>
                  <Input
                    id="hw_accel_device"
                    value={formData.hw_accel_device}
                    onChange={(e) => setFormData({ ...formData, hw_accel_device: e.target.value })}
                    placeholder="e.g., /dev/dri/renderD128"
                  />
                  <p className="text-xs text-muted-foreground">
                    Device path for hardware acceleration (optional)
                  </p>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="gpu_index" className="italic">
                    GPU Index
                  </Label>
                  <Input
                    id="gpu_index"
                    type="number"
                    value={formData.gpu_index}
                    onChange={(e) => setFormData({ ...formData, gpu_index: e.target.value })}
                    placeholder="e.g., 0"
                  />
                  <p className="text-xs text-muted-foreground">
                    GPU index for multi-GPU systems (optional)
                  </p>
                </div>
              </div>
              <div className="grid gap-4 grid-cols-1 md:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="hw_accel_output_format" className="italic">
                    Output Format
                  </Label>
                  <Input
                    id="hw_accel_output_format"
                    value={formData.hw_accel_output_format}
                    onChange={(e) => setFormData({ ...formData, hw_accel_output_format: e.target.value })}
                    placeholder="e.g., nv12, cuda"
                  />
                  <p className="text-xs text-muted-foreground">
                    Hardware frame output format (optional)
                  </p>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="hw_accel_decoder_codec" className="italic">
                    Decoder Codec
                  </Label>
                  <Input
                    id="hw_accel_decoder_codec"
                    value={formData.hw_accel_decoder_codec}
                    onChange={(e) => setFormData({ ...formData, hw_accel_decoder_codec: e.target.value })}
                    placeholder="e.g., h264_cuvid"
                  />
                  <p className="text-xs text-muted-foreground">
                    Hardware decoder to use (optional)
                  </p>
                </div>
              </div>
              <div className="space-y-2">
                <Label htmlFor="hw_accel_extra_options" className="italic">
                  Extra Options
                </Label>
                <Input
                  id="hw_accel_extra_options"
                  value={formData.hw_accel_extra_options}
                  onChange={(e) => setFormData({ ...formData, hw_accel_extra_options: e.target.value })}
                  placeholder="e.g., -extra_hw_frames 4"
                />
                <p className="text-xs text-muted-foreground">
                  Additional hardware acceleration options (advanced)
                </p>
              </div>
            </CollapsibleContent>
          </Collapsible>
        )}
      </div>

      {/* Output Format */}
      <div className="space-y-2">
        <Label htmlFor="output_format">Output Format</Label>
        <Select
          value={formData.output_format}
          onValueChange={(value) => setFormData({ ...formData, output_format: value })}
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {OUTPUT_FORMATS.map((format) => (
              <SelectItem key={format.value} value={format.value}>
                {format.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {/* Custom FFmpeg Flags */}
      <Collapsible open={showCustomFlags} onOpenChange={setShowCustomFlags}>
        <CollapsibleTrigger className="flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground transition-colors">
          {showCustomFlags ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
          Custom FFmpeg Flags
        </CollapsibleTrigger>
        <CollapsibleContent className="space-y-4 pt-4">
          <p className="text-xs text-muted-foreground mb-2">
            Advanced: Add custom FFmpeg flags for fine-grained control. These are validated for security.
          </p>
          <div className="space-y-2">
            <Label htmlFor="input_options" className="italic">
              Input Options
            </Label>
            <Textarea
              id="input_options"
              value={formData.input_options}
              onChange={(e) => setFormData({ ...formData, input_options: e.target.value })}
              placeholder="e.g., -fflags +igndts"
              rows={2}
              className="font-mono text-sm"
            />
            <p className="text-xs text-muted-foreground">
              Options applied before the input (affects decoding)
            </p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="output_options" className="italic">
              Output Options
            </Label>
            <Textarea
              id="output_options"
              value={formData.output_options}
              onChange={(e) => setFormData({ ...formData, output_options: e.target.value })}
              placeholder="e.g., -map 0:v -map 0:a"
              rows={2}
              className="font-mono text-sm"
            />
            <p className="text-xs text-muted-foreground">
              Options applied after codecs (affects output)
            </p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="filter_complex" className="italic">
              Filter Complex
            </Label>
            <Textarea
              id="filter_complex"
              value={formData.filter_complex}
              onChange={(e) => setFormData({ ...formData, filter_complex: e.target.value })}
              placeholder="e.g., [0:v]scale=1920:1080[v]"
              rows={3}
              className="font-mono text-sm"
            />
            <p className="text-xs text-muted-foreground">
              Complex filtergraph for advanced video/audio processing
            </p>
          </div>
        </CollapsibleContent>
      </Collapsible>

      {/* Fallback Settings */}
      <Collapsible open={showFallback} onOpenChange={setShowFallback}>
        <CollapsibleTrigger className="flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground transition-colors">
          {showFallback ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
          Fallback Settings
        </CollapsibleTrigger>
        <CollapsibleContent className="space-y-4 pt-4">
          <div className="flex items-center justify-between">
            <div>
              <Label htmlFor="fallback_enabled">Enable Fallback to Copy Mode</Label>
              <p className="text-xs text-muted-foreground">
                Automatically fallback to copy mode if transcoding fails repeatedly
              </p>
            </div>
            <Switch
              id="fallback_enabled"
              checked={formData.fallback_enabled}
              onCheckedChange={(checked) => setFormData({ ...formData, fallback_enabled: checked })}
            />
          </div>
          {formData.fallback_enabled && (
            <div className="grid gap-4 grid-cols-1 md:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="fallback_error_threshold" className="italic">
                  Error Threshold
                </Label>
                <Input
                  id="fallback_error_threshold"
                  type="number"
                  value={formData.fallback_error_threshold}
                  onChange={(e) => setFormData({ ...formData, fallback_error_threshold: e.target.value })}
                  placeholder="e.g., 3"
                />
                <p className="text-xs text-muted-foreground">
                  Number of errors before falling back
                </p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="fallback_recovery_interval" className="italic">
                  Recovery Interval (seconds)
                </Label>
                <Input
                  id="fallback_recovery_interval"
                  type="number"
                  value={formData.fallback_recovery_interval}
                  onChange={(e) => setFormData({ ...formData, fallback_recovery_interval: e.target.value })}
                  placeholder="e.g., 300"
                />
                <p className="text-xs text-muted-foreground">
                  Seconds to wait before retrying transcoding
                </p>
              </div>
            </div>
          )}
        </CollapsibleContent>
      </Collapsible>
    </form>
  );
}
