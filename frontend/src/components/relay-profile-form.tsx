'use client';

import { useState, useEffect } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import {
  RelayProfile,
  CreateRelayProfileRequest,
  UpdateRelayProfileRequest,
  VideoCodec,
  AudioCodec,
  RelayOutputFormat,
} from '@/types/api';

interface RelayProfileFormProps {
  profile?: RelayProfile;
  onSubmit: (data: CreateRelayProfileRequest | UpdateRelayProfileRequest) => void;
  onCancel: () => void;
  formId: string;
  loading?: boolean;
}

const VIDEO_CODECS: { value: VideoCodec; label: string }[] = [
  { value: 'H264', label: 'H.264' },
  { value: 'H265', label: 'H.265/HEVC' },
  { value: 'AV1', label: 'AV1' },
  { value: 'MPEG2', label: 'MPEG-2' },
  { value: 'MPEG4', label: 'MPEG-4' },
  { value: 'Copy', label: 'Copy (No transcode)' },
];

const AUDIO_CODECS: { value: AudioCodec; label: string }[] = [
  { value: 'AAC', label: 'AAC' },
  { value: 'MP3', label: 'MP3' },
  { value: 'AC3', label: 'AC3' },
  { value: 'EAC3', label: 'EAC3' },
  { value: 'MPEG2Audio', label: 'MPEG-2 Audio' },
  { value: 'DTS', label: 'DTS' },
  { value: 'Copy', label: 'Copy (No transcode)' },
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

const VIDEO_PROFILES = [
  { value: 'baseline', label: 'Baseline' },
  { value: 'main', label: 'Main' },
  { value: 'high', label: 'High' },
  { value: 'main10', label: 'Main 10' },
];

const HWACCEL_OPTIONS = [
  { value: 'nvenc', label: 'NVIDIA NVENC' },
  { value: 'vaapi', label: 'VA-API' },
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
  const [formData, setFormData] = useState({
    name: profile?.name || '',
    description: profile?.description || '',
    video_codec: profile?.video_codec || ('H264' as VideoCodec),
    audio_codec: profile?.audio_codec || ('AAC' as AudioCodec),
    video_profile: profile?.video_profile || '',
    video_preset: profile?.video_preset || '',
    video_bitrate: profile?.video_bitrate?.toString() || '',
    audio_bitrate: profile?.audio_bitrate?.toString() || '',
    audio_sample_rate: profile?.audio_sample_rate?.toString() || '',
    audio_channels: profile?.audio_channels?.toString() || '',
    enable_hardware_acceleration: profile?.enable_hardware_acceleration || false,
    preferred_hwaccel: profile?.preferred_hwaccel || '',
    manual_args: profile?.manual_args || '',
    input_timeout: profile?.input_timeout?.toString() || '',
    is_active: profile?.is_active ?? true,
  });

  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    // Validate mandatory fields
    if (!formData.name.trim()) {
      alert('Profile name is required');
      return;
    }
    if (!formData.video_codec) {
      alert('Video codec is required');
      return;
    }
    if (!formData.audio_codec) {
      alert('Audio codec is required');
      return;
    }

    setIsSubmitting(true);

    try {
      const data: CreateRelayProfileRequest | UpdateRelayProfileRequest = {
        name: formData.name,
        description: formData.description || undefined,
        video_codec: formData.video_codec,
        audio_codec: formData.audio_codec,
        video_profile: formData.video_profile || undefined,
        video_preset: formData.video_preset || undefined,
        video_bitrate: formData.video_bitrate ? parseInt(formData.video_bitrate) : undefined,
        audio_bitrate: formData.audio_bitrate ? parseInt(formData.audio_bitrate) : undefined,
        audio_sample_rate: formData.audio_sample_rate
          ? parseInt(formData.audio_sample_rate)
          : undefined,
        audio_channels: formData.audio_channels ? parseInt(formData.audio_channels) : undefined,
        enable_hardware_acceleration: formData.enable_hardware_acceleration,
        preferred_hwaccel: formData.preferred_hwaccel || undefined,
        manual_args: formData.manual_args || undefined,
        output_format: 'TransportStream' as RelayOutputFormat,
        input_timeout: formData.input_timeout ? parseInt(formData.input_timeout) : undefined,
        ...(profile ? { is_active: formData.is_active } : { is_system_default: false }),
      };

      await onSubmit(data);
    } finally {
      setIsSubmitting(false);
    }
  };

  const isVideoTranscode = formData.video_codec.toLowerCase() !== 'copy';
  const isAudioTranscode = formData.audio_codec.toLowerCase() !== 'copy';

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

        {profile && (
          <div className="flex items-center space-x-2">
            <Switch
              id="is_active"
              checked={formData.is_active}
              onCheckedChange={(checked) => setFormData({ ...formData, is_active: checked })}
            />
            <Label htmlFor="is_active">Active</Label>
          </div>
        )}
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
              onValueChange={(value: VideoCodec) =>
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
          <div className="grid gap-4 grid-cols-1 md:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="video_profile" className="italic">
                Video Profile
              </Label>
              <Select
                value={formData.video_profile}
                onValueChange={(value) => setFormData({ ...formData, video_profile: value })}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select profile" />
                </SelectTrigger>
                <SelectContent>
                  {VIDEO_PROFILES.map((profile) => (
                    <SelectItem key={profile.value} value={profile.value}>
                      {profile.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

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
          </div>
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
              onValueChange={(value: AudioCodec) =>
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
        <div className="flex items-center space-x-2">
          <Switch
            id="enable_hardware_acceleration"
            checked={formData.enable_hardware_acceleration}
            onCheckedChange={(checked) =>
              setFormData({ ...formData, enable_hardware_acceleration: checked })
            }
          />
          <Label htmlFor="enable_hardware_acceleration">Enable Hardware Acceleration</Label>
        </div>

        {formData.enable_hardware_acceleration && (
          <div className="space-y-2">
            <Label htmlFor="preferred_hwaccel">Preferred Hardware Accelerator</Label>
            <Select
              value={formData.preferred_hwaccel}
              onValueChange={(value) => setFormData({ ...formData, preferred_hwaccel: value })}
            >
              <SelectTrigger>
                <SelectValue placeholder="Auto-detect" />
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
        )}
      </div>

      {/* Timeout Settings */}
      <div className="space-y-2">
        <Label htmlFor="input_timeout" className="italic">
          Input Timeout (seconds)
        </Label>
        <Input
          id="input_timeout"
          type="number"
          value={formData.input_timeout}
          onChange={(e) => setFormData({ ...formData, input_timeout: e.target.value })}
          placeholder="e.g., 30"
        />
      </div>

      {/* Advanced Settings */}
      <div className="space-y-2">
        <Label htmlFor="manual_args" className="italic">
          Manual FFmpeg Arguments
        </Label>
        <Textarea
          id="manual_args"
          value={formData.manual_args}
          onChange={(e) => setFormData({ ...formData, manual_args: e.target.value })}
          placeholder="e.g., -threads 4 -buffer_size 64k"
          rows={3}
        />
        <p className="text-xs text-muted-foreground">
          Optional custom FFmpeg arguments. Use with caution as they override other settings.
        </p>
      </div>
    </form>
  );
}
