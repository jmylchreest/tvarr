'use client';

import { useState } from 'react';
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
  { value: 'none', label: 'None (Software)' },
  { value: 'cuda', label: 'NVIDIA CUDA' },
  { value: 'qsv', label: 'Intel Quick Sync' },
  { value: 'vaapi', label: 'VA-API' },
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
    video_codec: profile?.video_codec || 'libx264',
    audio_codec: profile?.audio_codec || 'aac',
    video_preset: profile?.video_preset || '',
    video_bitrate: profile?.video_bitrate?.toString() || '',
    audio_bitrate: profile?.audio_bitrate?.toString() || '',
    audio_sample_rate: profile?.audio_sample_rate?.toString() || '',
    audio_channels: profile?.audio_channels?.toString() || '',
    hw_accel: profile?.hw_accel || 'none',
    output_format: profile?.output_format || 'mpegts',
  });

  const [isSubmitting, setIsSubmitting] = useState(false);

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
        audio_bitrate: formData.audio_bitrate ? parseInt(formData.audio_bitrate) : undefined,
        audio_sample_rate: formData.audio_sample_rate
          ? parseInt(formData.audio_sample_rate)
          : undefined,
        audio_channels: formData.audio_channels ? parseInt(formData.audio_channels) : undefined,
        hw_accel: formData.hw_accel || undefined,
        output_format: formData.output_format || undefined,
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
    </form>
  );
}
