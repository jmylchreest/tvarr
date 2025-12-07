'use client';

import { useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Badge } from '@/components/ui/badge';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import {
  Play,
  Loader2,
  CheckCircle,
  XCircle,
  AlertTriangle,
  ChevronDown,
  ChevronRight,
  Lightbulb,
  Terminal,
  Cpu,
} from 'lucide-react';
import { RelayProfile, ProfileTestResult } from '@/types/api';
import { getBackendUrl } from '@/lib/config';

interface ProfileTestDialogProps {
  profile: RelayProfile;
  trigger?: React.ReactNode;
}

export function ProfileTestDialog({ profile, trigger }: ProfileTestDialogProps) {
  const [open, setOpen] = useState(false);
  const [streamUrl, setStreamUrl] = useState('');
  const [testing, setTesting] = useState(false);
  const [result, setResult] = useState<ProfileTestResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [showOutput, setShowOutput] = useState(false);
  const [showCommand, setShowCommand] = useState(false);

  const handleTest = async () => {
    if (!streamUrl.trim()) {
      setError('Please enter a stream URL to test');
      return;
    }

    setTesting(true);
    setError(null);
    setResult(null);

    try {
      const backendUrl = getBackendUrl();
      const response = await fetch(
        `${backendUrl}/api/v1/relay/profiles/${profile.id}/test`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ stream_url: streamUrl }),
        }
      );

      if (!response.ok) {
        const errorData = await response.json();
        throw new Error(errorData.error || 'Failed to test profile');
      }

      const data = await response.json();
      setResult(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Test failed');
    } finally {
      setTesting(false);
    }
  };

  const handleClose = () => {
    setOpen(false);
    // Reset state after dialog closes
    setTimeout(() => {
      setResult(null);
      setError(null);
      setShowOutput(false);
      setShowCommand(false);
    }, 200);
  };

  return (
    <Dialog open={open} onOpenChange={(o) => (o ? setOpen(true) : handleClose())}>
      <DialogTrigger asChild>
        {trigger || (
          <Button variant="ghost" size="sm" title="Test profile">
            <Play className="h-4 w-4" />
          </Button>
        )}
      </DialogTrigger>
      <DialogContent className="max-w-2xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Test Profile: {profile.name}</DialogTitle>
          <DialogDescription>
            Test this relay profile against a live stream to verify configuration
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {/* Stream URL Input */}
          <div className="space-y-2">
            <Label htmlFor="stream_url">Stream URL</Label>
            <div className="flex gap-2">
              <Input
                id="stream_url"
                value={streamUrl}
                onChange={(e) => setStreamUrl(e.target.value)}
                placeholder="http://example.com/stream.m3u8"
                disabled={testing}
                className="flex-1"
              />
              <Button onClick={handleTest} disabled={testing || !streamUrl.trim()}>
                {testing ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Testing...
                  </>
                ) : (
                  <>
                    <Play className="mr-2 h-4 w-4" />
                    Test
                  </>
                )}
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              Enter a stream URL to test this profile against. The test runs for up to 30 seconds.
            </p>
          </div>

          {/* Error Display */}
          {error && (
            <div className="p-4 rounded-lg bg-destructive/10 border border-destructive/20">
              <div className="flex items-center gap-2 text-destructive">
                <XCircle className="h-5 w-5" />
                <span className="font-medium">Test Failed</span>
              </div>
              <p className="mt-2 text-sm text-destructive">{error}</p>
            </div>
          )}

          {/* Test Results */}
          {result && (
            <div className="space-y-4">
              {/* Status Banner */}
              <div
                className={`p-4 rounded-lg ${
                  result.success
                    ? 'bg-green-500/10 border border-green-500/20'
                    : 'bg-destructive/10 border border-destructive/20'
                }`}
              >
                <div className="flex items-center gap-2">
                  {result.success ? (
                    <CheckCircle className="h-5 w-5 text-green-500" />
                  ) : (
                    <XCircle className="h-5 w-5 text-destructive" />
                  )}
                  <span
                    className={`font-medium ${
                      result.success ? 'text-green-600' : 'text-destructive'
                    }`}
                  >
                    {result.success ? 'Test Passed' : 'Test Failed'}
                  </span>
                  <span className="text-sm text-muted-foreground ml-auto">
                    {(result.duration_ms / 1000).toFixed(1)}s
                  </span>
                </div>
              </div>

              {/* Metrics Grid */}
              <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                <div className="p-3 rounded-lg bg-muted/50">
                  <p className="text-xs text-muted-foreground">Frames</p>
                  <p className="text-lg font-semibold">{result.frames_processed}</p>
                </div>
                <div className="p-3 rounded-lg bg-muted/50">
                  <p className="text-xs text-muted-foreground">FPS</p>
                  <p className="text-lg font-semibold">{result.fps.toFixed(1)}</p>
                </div>
                <div className="p-3 rounded-lg bg-muted/50">
                  <p className="text-xs text-muted-foreground">Bitrate</p>
                  <p className="text-lg font-semibold">
                    {result.bitrate_kbps ? `${result.bitrate_kbps} kbps` : 'N/A'}
                  </p>
                </div>
                <div className="p-3 rounded-lg bg-muted/50">
                  <p className="text-xs text-muted-foreground">Resolution</p>
                  <p className="text-lg font-semibold">{result.resolution || 'N/A'}</p>
                </div>
              </div>

              {/* Hardware Acceleration */}
              <div className="flex items-center gap-2">
                <Cpu className="h-4 w-4 text-muted-foreground" />
                <span className="text-sm">Hardware Acceleration:</span>
                {result.hw_accel_active ? (
                  <Badge variant="default" className="bg-green-500">
                    Active ({result.hw_accel_method})
                  </Badge>
                ) : (
                  <Badge variant="outline">Not Active</Badge>
                )}
              </div>

              {/* Codec Info */}
              <div className="grid grid-cols-2 gap-4 text-sm">
                <div>
                  <span className="text-muted-foreground">Video In:</span>{' '}
                  <span>{result.video_codec_in || 'Unknown'}</span>
                </div>
                <div>
                  <span className="text-muted-foreground">Video Out:</span>{' '}
                  <span>{result.video_codec_out || 'Unknown'}</span>
                </div>
                <div>
                  <span className="text-muted-foreground">Audio In:</span>{' '}
                  <span>{result.audio_codec_in || 'Unknown'}</span>
                </div>
                <div>
                  <span className="text-muted-foreground">Audio Out:</span>{' '}
                  <span>{result.audio_codec_out || 'Unknown'}</span>
                </div>
              </div>

              {/* Errors */}
              {result.errors && result.errors.length > 0 && (
                <div className="p-3 rounded-lg bg-destructive/10 border border-destructive/20">
                  <div className="flex items-center gap-2 text-destructive mb-2">
                    <XCircle className="h-4 w-4" />
                    <span className="font-medium">Errors</span>
                  </div>
                  <ul className="space-y-1 text-sm text-destructive">
                    {result.errors.map((err, i) => (
                      <li key={i}>{err}</li>
                    ))}
                  </ul>
                </div>
              )}

              {/* Warnings */}
              {result.warnings && result.warnings.length > 0 && (
                <div className="p-3 rounded-lg bg-yellow-500/10 border border-yellow-500/20">
                  <div className="flex items-center gap-2 text-yellow-600 mb-2">
                    <AlertTriangle className="h-4 w-4" />
                    <span className="font-medium">Warnings</span>
                  </div>
                  <ul className="space-y-1 text-sm text-yellow-600">
                    {result.warnings.map((warn, i) => (
                      <li key={i}>{warn}</li>
                    ))}
                  </ul>
                </div>
              )}

              {/* Suggestions */}
              {result.suggestions && result.suggestions.length > 0 && (
                <div className="p-3 rounded-lg bg-blue-500/10 border border-blue-500/20">
                  <div className="flex items-center gap-2 text-blue-600 mb-2">
                    <Lightbulb className="h-4 w-4" />
                    <span className="font-medium">Suggestions</span>
                  </div>
                  <ul className="space-y-1 text-sm text-blue-600">
                    {result.suggestions.map((sug, i) => (
                      <li key={i}>{sug}</li>
                    ))}
                  </ul>
                </div>
              )}

              {/* FFmpeg Command */}
              {result.ffmpeg_command && (
                <Collapsible open={showCommand} onOpenChange={setShowCommand}>
                  <CollapsibleTrigger className="flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground">
                    {showCommand ? (
                      <ChevronDown className="h-4 w-4" />
                    ) : (
                      <ChevronRight className="h-4 w-4" />
                    )}
                    <Terminal className="h-4 w-4" />
                    FFmpeg Command
                  </CollapsibleTrigger>
                  <CollapsibleContent className="pt-2">
                    <pre className="p-3 rounded-lg bg-muted text-xs overflow-x-auto whitespace-pre-wrap break-all">
                      {result.ffmpeg_command}
                    </pre>
                  </CollapsibleContent>
                </Collapsible>
              )}

              {/* FFmpeg Output */}
              {result.ffmpeg_output && (
                <Collapsible open={showOutput} onOpenChange={setShowOutput}>
                  <CollapsibleTrigger className="flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground">
                    {showOutput ? (
                      <ChevronDown className="h-4 w-4" />
                    ) : (
                      <ChevronRight className="h-4 w-4" />
                    )}
                    <Terminal className="h-4 w-4" />
                    FFmpeg Output
                  </CollapsibleTrigger>
                  <CollapsibleContent className="pt-2">
                    <pre className="p-3 rounded-lg bg-muted text-xs overflow-x-auto max-h-64 overflow-y-auto whitespace-pre-wrap">
                      {result.ffmpeg_output}
                    </pre>
                  </CollapsibleContent>
                </Collapsible>
              )}
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
