'use client';

import { useState, useEffect, useCallback } from 'react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
  DialogFooter,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import {
  Eye,
  Loader2,
  Terminal,
  Copy,
  Check,
  Info,
  RefreshCw,
} from 'lucide-react';
import { RelayProfile, CommandPreview } from '@/types/api';
import { getBackendUrl } from '@/lib/config';

interface CommandPreviewModalProps {
  profile: RelayProfile;
  trigger?: React.ReactNode;
}

export function CommandPreviewModal({ profile, trigger }: CommandPreviewModalProps) {
  const [open, setOpen] = useState(false);
  const [inputUrl, setInputUrl] = useState('http://example.com/stream.m3u8');
  const [loading, setLoading] = useState(false);
  const [preview, setPreview] = useState<CommandPreview | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const fetchPreview = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      const backendUrl = getBackendUrl();
      const response = await fetch(
        `${backendUrl}/api/v1/relay/profiles/${profile.id}/preview`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            input_url: inputUrl,
            output_url: 'pipe:1', // Always use pipe:1 for relay output
          }),
        }
      );

      if (!response.ok) {
        const errorData = await response.json();
        throw new Error(errorData.error || 'Failed to generate preview');
      }

      const data = await response.json();
      setPreview(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Preview failed');
    } finally {
      setLoading(false);
    }
  }, [profile.id, inputUrl]);

  // Fetch preview when dialog opens
  useEffect(() => {
    if (open) {
      fetchPreview();
    }
  }, [open, fetchPreview]);

  const handleCopy = async () => {
    if (preview?.command) {
      await navigator.clipboard.writeText(preview.command);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  const handleClose = () => {
    setOpen(false);
    // Reset state after dialog closes
    setTimeout(() => {
      setPreview(null);
      setError(null);
    }, 200);
  };

  return (
    <Dialog open={open} onOpenChange={(o) => (o ? setOpen(true) : handleClose())}>
      <DialogTrigger asChild>
        {trigger || (
          <Button variant="ghost" size="sm" title="Preview command">
            <Eye className="h-4 w-4" />
          </Button>
        )}
      </DialogTrigger>
      <DialogContent className="max-w-3xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Command Preview: {profile.name}</DialogTitle>
          <DialogDescription>
            Preview the FFmpeg command that will be generated for this profile
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {/* Input URL */}
          <div className="flex gap-4 items-end">
            <div className="flex-1 space-y-2">
              <Label htmlFor="input_url">Sample Input URL</Label>
              <Input
                id="input_url"
                value={inputUrl}
                onChange={(e) => setInputUrl(e.target.value)}
                placeholder="http://example.com/stream.m3u8"
              />
            </div>
            <Button variant="outline" onClick={fetchPreview} disabled={loading}>
              {loading ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <RefreshCw className="mr-2 h-4 w-4" />
              )}
              Refresh
            </Button>
          </div>

          {/* Error Display */}
          {error && (
            <div className="p-4 rounded-lg bg-destructive/10 border border-destructive/20">
              <p className="text-sm text-destructive">{error}</p>
            </div>
          )}

          {/* Loading State */}
          {loading && !preview && (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
            </div>
          )}

          {/* Preview Content */}
          {preview && (
            <div className="space-y-4">
              {/* Configuration Summary */}
              <div className="grid grid-cols-3 gap-4">
                <div className="p-3 rounded-lg bg-muted/50">
                  <p className="text-xs text-muted-foreground">Video Codec</p>
                  <p className="text-sm font-semibold">
                    {preview.video_codec || 'Default'}
                  </p>
                </div>
                <div className="p-3 rounded-lg bg-muted/50">
                  <p className="text-xs text-muted-foreground">Audio Codec</p>
                  <p className="text-sm font-semibold">
                    {preview.audio_codec || 'Default'}
                  </p>
                </div>
                <div className="p-3 rounded-lg bg-muted/50">
                  <p className="text-xs text-muted-foreground">HW Accel</p>
                  <p className="text-sm font-semibold">
                    {preview.hw_accel || 'None'}
                  </p>
                </div>
              </div>

              {/* Notes */}
              {preview.notes && preview.notes.length > 0 && (
                <div className="p-3 rounded-lg bg-blue-500/10 border border-blue-500/20">
                  <div className="flex items-center gap-2 text-blue-600 mb-2">
                    <Info className="h-4 w-4" />
                    <span className="font-medium">Configuration Notes</span>
                  </div>
                  <ul className="space-y-1 text-sm text-blue-600">
                    {preview.notes.map((note, i) => (
                      <li key={i}>{note}</li>
                    ))}
                  </ul>
                </div>
              )}

              {/* Full Command */}
              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Terminal className="h-4 w-4 text-muted-foreground" />
                    <span className="text-sm font-medium">Full Command</span>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleCopy}
                    disabled={!preview.command}
                  >
                    {copied ? (
                      <>
                        <Check className="mr-2 h-3 w-3" />
                        Copied
                      </>
                    ) : (
                      <>
                        <Copy className="mr-2 h-3 w-3" />
                        Copy
                      </>
                    )}
                  </Button>
                </div>
                <pre className="p-4 rounded-lg bg-muted text-xs overflow-x-auto whitespace-pre-wrap break-all font-mono">
                  {preview.command}
                </pre>
              </div>

              {/* Arguments List */}
              <div className="space-y-2">
                <span className="text-sm font-medium">Arguments</span>
                <div className="flex flex-wrap gap-1">
                  {preview.args.map((arg, i) => (
                    <Badge
                      key={i}
                      variant="outline"
                      className="font-mono text-xs"
                    >
                      {arg}
                    </Badge>
                  ))}
                </div>
              </div>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={handleClose}>
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
