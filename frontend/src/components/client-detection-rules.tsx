'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  Plus,
  Trash2,
  Loader2,
  Lock,
  Users,
  ArrowUp,
  ArrowDown,
  Play,
  CheckCircle,
  XCircle,
  AlertCircle,
  Copy,
  Check,
} from 'lucide-react';
import {
  ClientDetectionRule,
  ClientDetectionRuleCreateRequest,
  ClientDetectionRuleUpdateRequest,
} from '@/types/api';
import { apiClient, ApiError } from '@/lib/api-client';
import { createFuzzyFilter } from '@/lib/fuzzy-search';
import { ExportDialog, ImportDialog } from '@/components/config-export';
import { ClientDetectionExpressionEditor } from '@/components/client-detection-expression-editor';
import {
  MasterDetailLayout,
  DetailPanel,
  DetailEmpty,
  MasterItem,
} from '@/components/shared';
import { BadgeGroup, BadgeItem } from '@/components/shared';
import { StatCard } from '@/components/shared/feedback/StatCard';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { ChevronDown, ChevronRight } from 'lucide-react';

interface LoadingState {
  rules: boolean;
  create: boolean;
  edit: boolean;
  delete: string | null;
  toggle: string | null;
  reorder: boolean;
  test: boolean;
}

interface ErrorState {
  rules: string | null;
  create: string | null;
  edit: string | null;
  action: string | null;
}

interface RuleFormData {
  name: string;
  description: string;
  expression: string;
  priority: number;
  is_enabled: boolean;
  accepted_video_codecs: string[];
  accepted_audio_codecs: string[];
  preferred_video_codec: string;
  preferred_audio_codec: string;
  supports_fmp4: boolean;
  supports_mpegts: boolean;
  preferred_format: string;
}

const VIDEO_CODECS = ['h264', 'h265', 'vp9', 'av1'];
const AUDIO_CODECS = ['aac', 'opus', 'ac3', 'eac3', 'mp3'];
// Codecs that require fMP4 container (not compatible with MPEG-TS)
const FMP4_ONLY_VIDEO_CODECS = ['vp9', 'av1'];
const FMP4_ONLY_AUDIO_CODECS = ['opus'];
const FORMAT_OPTIONS = [
  { value: 'auto', label: 'Auto' },
  { value: 'hls-fmp4', label: 'HLS (fMP4)' },
  { value: 'hls-ts', label: 'HLS (MPEG-TS segments)' },
  { value: 'mpegts', label: 'MPEG-TS' },
  { value: 'dash', label: 'DASH' },
];

// Check if selected codecs are compatible with the selected format
const isFormatCompatibleWithCodecs = (
  format: string,
  videoCodec: string,
  audioCodec: string
): boolean => {
  // Auto, HLS (fMP4), and DASH support all codecs
  if (['auto', 'hls-fmp4', 'dash'].includes(format)) {
    return true;
  }
  // MPEG-TS formats (hls-ts, mpegts) don't support VP9/AV1/Opus
  if (['hls-ts', 'mpegts'].includes(format)) {
    const hasIncompatibleVideoCodec = FMP4_ONLY_VIDEO_CODECS.includes(videoCodec);
    const hasIncompatibleAudioCodec = FMP4_ONLY_AUDIO_CODECS.includes(audioCodec);
    return !hasIncompatibleVideoCodec && !hasIncompatibleAudioCodec;
  }
  return true;
};

// Helper to display codec value or {dynamic} for SET-based rules
const formatCodec = (codec: string): string => {
  return codec ? codec.toUpperCase() : '{dynamic}';
};

// CopyableExpression component for click-to-copy expressions
function CopyableExpression({
  expression,
  className,
  maxWidth,
}: {
  expression: string;
  className?: string;
  maxWidth?: string;
}) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await navigator.clipboard.writeText(expression);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy expression:', err);
    }
  };

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <code
          className={`cursor-pointer hover:bg-muted px-2 py-1 rounded text-xs font-mono group inline-flex items-center gap-1 ${className || ''}`}
          onClick={handleCopy}
          style={maxWidth ? { maxWidth } : undefined}
        >
          <span className={maxWidth ? 'truncate' : ''}>{expression}</span>
          {copied ? (
            <Check className="h-3 w-3 text-green-500 flex-shrink-0" />
          ) : (
            <Copy className="h-3 w-3 opacity-0 group-hover:opacity-50 flex-shrink-0" />
          )}
        </code>
      </TooltipTrigger>
      <TooltipContent>
        {copied ? 'Copied!' : 'Click to copy'}
      </TooltipContent>
    </Tooltip>
  );
}

const defaultFormData: RuleFormData = {
  name: '',
  description: '',
  expression: '',
  priority: 0,
  is_enabled: true,
  accepted_video_codecs: ['h264'],
  accepted_audio_codecs: ['aac'],
  preferred_video_codec: 'h264',
  preferred_audio_codec: 'aac',
  supports_fmp4: true,
  supports_mpegts: true,
  preferred_format: 'auto',
};

/**
 * ClientDetectionRuleCreatePanel - Inline panel for creating a new client detection rule
 */
function ClientDetectionRuleCreatePanel({
  onCreate,
  onCancel,
  loading,
  error,
  nextPriority,
}: {
  onCreate: (data: RuleFormData) => Promise<void>;
  onCancel: () => void;
  loading: boolean;
  error: string | null;
  nextPriority: number;
}) {
  const [formData, setFormData] = useState<RuleFormData>({ ...defaultFormData, priority: nextPriority });
  const [testUserAgent, setTestUserAgent] = useState('');
  const [testResult, setTestResult] = useState<{ matches: boolean; error?: string } | null>(null);
  const [testing, setTesting] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    await onCreate(formData);
  };

  const handleTestExpression = async () => {
    if (!formData.expression.trim() || !testUserAgent.trim()) return;

    setTesting(true);
    setTestResult(null);
    try {
      const result = await apiClient.testClientDetectionExpression(
        formData.expression,
        testUserAgent
      );
      setTestResult(result);
    } catch (err) {
      setTestResult({
        matches: false,
        error: err instanceof Error ? err.message : 'Test failed',
      });
    } finally {
      setTesting(false);
    }
  };

  const toggleVideoCodec = (codec: string) => {
    setFormData((prev) => ({
      ...prev,
      accepted_video_codecs: prev.accepted_video_codecs.includes(codec)
        ? prev.accepted_video_codecs.filter((c) => c !== codec)
        : [...prev.accepted_video_codecs, codec],
    }));
  };

  const toggleAudioCodec = (codec: string) => {
    setFormData((prev) => ({
      ...prev,
      accepted_audio_codecs: prev.accepted_audio_codecs.includes(codec)
        ? prev.accepted_audio_codecs.filter((c) => c !== codec)
        : [...prev.accepted_audio_codecs, codec],
    }));
  };

  return (
    <DetailPanel
      title="Create Client Detection Rule"
      actions={
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={onCancel} disabled={loading}>
            Cancel
          </Button>
          <Button size="sm" onClick={handleSubmit} disabled={loading || !formData.name.trim() || !formData.expression.trim()}>
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
              placeholder="Chrome Browser"
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
              placeholder="Matches Chrome browser User-Agent strings"
              rows={2}
              disabled={loading}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="create-expression">Match Expression *</Label>
            <ClientDetectionExpressionEditor
              value={formData.expression}
              onChange={(value) => setFormData({ ...formData, expression: value })}
              placeholder='user_agent contains "Chrome" AND NOT user_agent contains "Edge"'
              disabled={loading}
              showValidationBadges={true}
            />
          </div>

          {/* Test Expression */}
          <div className="space-y-2 p-3 border rounded-lg bg-muted/50">
            <Label className="text-sm font-medium">Test Expression</Label>
            <div className="flex gap-2">
              <Input
                value={testUserAgent}
                onChange={(e) => setTestUserAgent(e.target.value)}
                placeholder="Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0..."
                className="flex-1 text-sm"
              />
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={handleTestExpression}
                disabled={testing || !formData.expression.trim() || !testUserAgent.trim()}
              >
                {testing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
              </Button>
            </div>
            {testResult && (
              <div className={`flex items-center gap-2 text-sm ${testResult.error ? 'text-destructive' : testResult.matches ? 'text-green-600' : 'text-orange-600'}`}>
                {testResult.error ? (
                  <><AlertCircle className="h-4 w-4" /> {testResult.error}</>
                ) : testResult.matches ? (
                  <><CheckCircle className="h-4 w-4" /> Expression matches</>
                ) : (
                  <><XCircle className="h-4 w-4" /> Expression does not match</>
                )}
              </div>
            )}
          </div>
        </div>

        {/* Priority and Status */}
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label>Priority</Label>
            <Input
              type="number"
              value={formData.priority}
              onChange={(e) => setFormData({ ...formData, priority: parseInt(e.target.value) || 0 })}
              disabled={loading}
            />
            <p className="text-xs text-muted-foreground">Lower values = higher priority</p>
          </div>

          <div className="space-y-2">
            <Label>Status</Label>
            <div className="flex items-center gap-2 pt-2">
              <Switch
                checked={formData.is_enabled}
                onCheckedChange={(checked) => setFormData({ ...formData, is_enabled: checked })}
                disabled={loading}
              />
              <span className="text-sm">{formData.is_enabled ? 'Enabled' : 'Disabled'}</span>
            </div>
          </div>
        </div>

        {/* Client Capabilities */}
        <div className="space-y-4">
          <h3 className="text-sm font-medium">Client Capabilities</h3>

          <div className="space-y-2">
            <Label className="text-sm">Accepted Video Codecs</Label>
            <div className="flex flex-wrap gap-2">
              {VIDEO_CODECS.map((codec) => (
                <Badge
                  key={codec}
                  variant={formData.accepted_video_codecs.includes(codec) ? 'default' : 'outline'}
                  className="cursor-pointer"
                  onClick={() => toggleVideoCodec(codec)}
                >
                  {codec.toUpperCase()}
                </Badge>
              ))}
            </div>
          </div>

          <div className="space-y-2">
            <Label className="text-sm">Accepted Audio Codecs</Label>
            <div className="flex flex-wrap gap-2">
              {AUDIO_CODECS.map((codec) => (
                <Badge
                  key={codec}
                  variant={formData.accepted_audio_codecs.includes(codec) ? 'default' : 'outline'}
                  className="cursor-pointer"
                  onClick={() => toggleAudioCodec(codec)}
                >
                  {codec.toUpperCase()}
                </Badge>
              ))}
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div className="flex items-center justify-between p-3 border rounded-lg">
              <Label className="text-sm">Supports fMP4</Label>
              <Switch
                checked={formData.supports_fmp4}
                onCheckedChange={(checked) => setFormData({ ...formData, supports_fmp4: checked })}
                disabled={loading}
              />
            </div>
            <div className="flex items-center justify-between p-3 border rounded-lg">
              <Label className="text-sm">Supports MPEG-TS</Label>
              <Switch
                checked={formData.supports_mpegts}
                onCheckedChange={(checked) => setFormData({ ...formData, supports_mpegts: checked })}
                disabled={loading}
              />
            </div>
          </div>
        </div>

        {/* Preferred Codecs */}
        <div className="space-y-4">
          <h3 className="text-sm font-medium">Transcoding Preferences (when source not compatible)</h3>

          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label>Preferred Video Codec</Label>
              <Select
                value={formData.preferred_video_codec}
                onValueChange={(value) => setFormData({ ...formData, preferred_video_codec: value })}
                disabled={loading}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {VIDEO_CODECS.map((codec) => (
                    <SelectItem key={codec} value={codec}>{codec.toUpperCase()}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label>Preferred Audio Codec</Label>
              <Select
                value={formData.preferred_audio_codec}
                onValueChange={(value) => setFormData({ ...formData, preferred_audio_codec: value })}
                disabled={loading}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {AUDIO_CODECS.map((codec) => (
                    <SelectItem key={codec} value={codec}>{codec.toUpperCase()}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="space-y-2">
            <Label>Preferred Output Format</Label>
            <Select
              value={formData.preferred_format}
              onValueChange={(value) => setFormData({ ...formData, preferred_format: value })}
              disabled={loading}
            >
              <SelectTrigger>
                <SelectValue placeholder="Auto" />
              </SelectTrigger>
              <SelectContent>
                {FORMAT_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
      </form>
    </DetailPanel>
  );
}

// Convert ClientDetectionRule to MasterItem format for MasterDetailLayout
interface ClientDetectionRuleMasterItem extends MasterItem {
  rule: ClientDetectionRule;
}

function clientDetectionRuleToMasterItem(rule: ClientDetectionRule): ClientDetectionRuleMasterItem {
  return {
    id: rule.id,
    title: rule.name,
    enabled: rule.is_enabled,
    rule,
  };
}

// Collapsible section component for organizing rule settings
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

// Detail panel for viewing/editing a selected client detection rule
function ClientDetectionRuleDetailPanel({
  rule,
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
  rule: ClientDetectionRule;
  onUpdate: (id: string, data: RuleFormData) => Promise<void>;
  onDelete: (rule: ClientDetectionRule) => Promise<void>;
  onToggle: (rule: ClientDetectionRule) => Promise<void>;
  onMoveUp: (id: string) => void;
  onMoveDown: (id: string) => void;
  loading: { edit: boolean; delete: string | null; toggle: string | null; reorder: boolean };
  error: string | null;
  isFirst: boolean;
  isLast: boolean;
}) {
  const [formData, setFormData] = useState<RuleFormData>({
    name: rule.name,
    description: rule.description || '',
    expression: rule.expression,
    priority: rule.priority,
    is_enabled: rule.is_enabled,
    accepted_video_codecs: rule.accepted_video_codecs,
    accepted_audio_codecs: rule.accepted_audio_codecs,
    preferred_video_codec: rule.preferred_video_codec,
    preferred_audio_codec: rule.preferred_audio_codec,
    supports_fmp4: rule.supports_fmp4,
    supports_mpegts: rule.supports_mpegts,
    preferred_format: rule.preferred_format || 'auto',
  });
  const [hasChanges, setHasChanges] = useState(false);

  // Reset form when rule changes
  useEffect(() => {
    setFormData({
      name: rule.name,
      description: rule.description || '',
      expression: rule.expression,
      priority: rule.priority,
      is_enabled: rule.is_enabled,
      accepted_video_codecs: rule.accepted_video_codecs,
      accepted_audio_codecs: rule.accepted_audio_codecs,
      preferred_video_codec: rule.preferred_video_codec,
      preferred_audio_codec: rule.preferred_audio_codec,
      supports_fmp4: rule.supports_fmp4,
      supports_mpegts: rule.supports_mpegts,
      preferred_format: rule.preferred_format || 'auto',
    });
    setHasChanges(false);
  }, [rule.id]);

  const handleFieldChange = (field: keyof RuleFormData, value: any) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
    setHasChanges(true);
  };

  const handleSave = async () => {
    await onUpdate(rule.id, formData);
    setHasChanges(false);
  };

  const toggleVideoCodec = (codec: string) => {
    const updated = formData.accepted_video_codecs.includes(codec)
      ? formData.accepted_video_codecs.filter((c) => c !== codec)
      : [...formData.accepted_video_codecs, codec];
    handleFieldChange('accepted_video_codecs', updated);
  };

  const toggleAudioCodec = (codec: string) => {
    const updated = formData.accepted_audio_codecs.includes(codec)
      ? formData.accepted_audio_codecs.filter((c) => c !== codec)
      : [...formData.accepted_audio_codecs, codec];
    handleFieldChange('accepted_audio_codecs', updated);
  };

  const isSystem = rule.is_system;

  return (
    <DetailPanel
      title={rule.name}
      actions={
        <div className="flex items-center gap-1">
          {/* Enabled/Disabled Toggle */}
          <div className="flex items-center gap-1.5 mr-2 px-2 py-1 rounded-md bg-muted/50">
            <Switch
              id={`toggle-enabled-${rule.id}`}
              checked={rule.is_enabled}
              onCheckedChange={() => onToggle(rule)}
              disabled={loading.toggle === rule.id}
              className="h-4 w-7 data-[state=checked]:bg-primary data-[state=unchecked]:bg-input"
            />
            <label htmlFor={`toggle-enabled-${rule.id}`} className="text-xs text-muted-foreground cursor-pointer">
              {rule.is_enabled ? 'Enabled' : 'Disabled'}
            </label>
          </div>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => onMoveUp(rule.id)}
            disabled={isFirst || loading.reorder}
            title="Move up (higher priority)"
          >
            <ArrowUp className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => onMoveDown(rule.id)}
            disabled={isLast || loading.reorder}
            title="Move down (lower priority)"
          >
            <ArrowDown className="h-4 w-4" />
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => onDelete(rule)}
            disabled={loading.delete === rule.id || isSystem}
            className="text-destructive hover:text-destructive"
            title={isSystem ? "System rules cannot be deleted" : "Delete rule"}
          >
            {loading.delete === rule.id ? (
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
            <AlertTitle>System Rule</AlertTitle>
            <AlertDescription>
              This is a system rule. You can enable/disable it and change its order, but cannot modify or delete it.
            </AlertDescription>
          </Alert>
        )}

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

        {/* Match Expression */}
        <CollapsibleSection title="Match Expression" defaultOpen={true}>
          <div className="space-y-4 pt-3">
            <ClientDetectionExpressionEditor
              value={formData.expression}
              onChange={(value) => handleFieldChange('expression', value)}
              placeholder='user_agent contains "Chrome" AND NOT user_agent contains "Edge"'
              disabled={loading.edit || isSystem}
              showValidationBadges={true}
            />
          </div>
        </CollapsibleSection>

        {/* Client Capabilities */}
        <CollapsibleSection title="Client Capabilities">
          <div className="space-y-4 pt-3">
            <div className="space-y-2">
              <Label className="text-sm">Accepted Video Codecs</Label>
              <div className="flex flex-wrap gap-2">
                {VIDEO_CODECS.map((codec) => (
                  <Badge
                    key={codec}
                    variant={formData.accepted_video_codecs.includes(codec) ? 'default' : 'outline'}
                    className={`cursor-pointer ${isSystem ? 'cursor-not-allowed opacity-60' : ''}`}
                    onClick={() => !isSystem && toggleVideoCodec(codec)}
                  >
                    {codec.toUpperCase()}
                  </Badge>
                ))}
              </div>
            </div>

            <div className="space-y-2">
              <Label className="text-sm">Accepted Audio Codecs</Label>
              <div className="flex flex-wrap gap-2">
                {AUDIO_CODECS.map((codec) => (
                  <Badge
                    key={codec}
                    variant={formData.accepted_audio_codecs.includes(codec) ? 'default' : 'outline'}
                    className={`cursor-pointer ${isSystem ? 'cursor-not-allowed opacity-60' : ''}`}
                    onClick={() => !isSystem && toggleAudioCodec(codec)}
                  >
                    {codec.toUpperCase()}
                  </Badge>
                ))}
              </div>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="flex items-center justify-between p-3 border rounded-lg">
                <Label className="text-sm">Supports fMP4</Label>
                <Switch
                  checked={formData.supports_fmp4}
                  onCheckedChange={(checked) => handleFieldChange('supports_fmp4', checked)}
                  disabled={loading.edit || isSystem}
                />
              </div>
              <div className="flex items-center justify-between p-3 border rounded-lg">
                <Label className="text-sm">Supports MPEG-TS</Label>
                <Switch
                  checked={formData.supports_mpegts}
                  onCheckedChange={(checked) => handleFieldChange('supports_mpegts', checked)}
                  disabled={loading.edit || isSystem}
                />
              </div>
            </div>
          </div>
        </CollapsibleSection>

        {/* Transcoding Preferences */}
        <CollapsibleSection title="Transcoding Preferences">
          <div className="space-y-4 pt-3">
            <p className="text-sm text-muted-foreground">
              Preferred codecs when source is not compatible with client capabilities
            </p>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label>Preferred Video Codec</Label>
                <Select
                  value={formData.preferred_video_codec}
                  onValueChange={(value) => handleFieldChange('preferred_video_codec', value)}
                  disabled={loading.edit || isSystem}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {VIDEO_CODECS.map((codec) => (
                      <SelectItem key={codec} value={codec}>{codec.toUpperCase()}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-2">
                <Label>Preferred Audio Codec</Label>
                <Select
                  value={formData.preferred_audio_codec}
                  onValueChange={(value) => handleFieldChange('preferred_audio_codec', value)}
                  disabled={loading.edit || isSystem}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {AUDIO_CODECS.map((codec) => (
                      <SelectItem key={codec} value={codec}>{codec.toUpperCase()}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            <div className="space-y-2">
              <Label>Preferred Output Format</Label>
              <Select
                value={formData.preferred_format}
                onValueChange={(value) => handleFieldChange('preferred_format', value)}
                disabled={loading.edit || isSystem}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Auto" />
                </SelectTrigger>
                <SelectContent>
                  {FORMAT_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
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

export function ClientDetectionRules() {
  const [allRules, setAllRules] = useState<ClientDetectionRule[]>([]);
  const [selectedRule, setSelectedRule] = useState<ClientDetectionRuleMasterItem | null>(null);
  const [loading, setLoading] = useState<LoadingState>({
    rules: true,
    create: false,
    edit: false,
    delete: null,
    toggle: null,
    reorder: false,
    test: false,
  });
  const [errors, setErrors] = useState<ErrorState>({
    rules: null,
    create: null,
    edit: null,
    action: null,
  });
  const [deleteDialog, setDeleteDialog] = useState<{ open: boolean; rule: ClientDetectionRule | null }>({
    open: false,
    rule: null,
  });
  const [isCreating, setIsCreating] = useState(false);

  const loadRules = useCallback(async () => {
    setLoading((prev) => ({ ...prev, rules: true }));
    setErrors((prev) => ({ ...prev, rules: null }));
    try {
      const data = await apiClient.getClientDetectionRules();
      setAllRules(data);
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to load client detection rules';
      setErrors((prev) => ({ ...prev, rules: message }));
    } finally {
      setLoading((prev) => ({ ...prev, rules: false }));
    }
  }, []);

  useEffect(() => {
    loadRules();
  }, [loadRules]);

  // Sort rules by priority
  const sortedRules = useMemo(() => {
    return [...allRules].sort((a, b) => a.priority - b.priority);
  }, [allRules]);

  // Convert rules to master items for MasterDetailLayout
  const masterItems = useMemo(
    () => sortedRules.map(clientDetectionRuleToMasterItem),
    [sortedRules]
  );

  const nextPriority = useMemo(() => {
    if (allRules.length === 0) return 100;
    return Math.max(...allRules.map((r) => r.priority)) + 10;
  }, [allRules]);

  const handleCreate = async (data: RuleFormData) => {
    setLoading((prev) => ({ ...prev, create: true }));
    setErrors((prev) => ({ ...prev, create: null }));
    try {
      const created = await apiClient.createClientDetectionRule({
        name: data.name,
        description: data.description || undefined,
        expression: data.expression,
        priority: data.priority,
        is_enabled: data.is_enabled,
        accepted_video_codecs: data.accepted_video_codecs,
        accepted_audio_codecs: data.accepted_audio_codecs,
        preferred_video_codec: data.preferred_video_codec,
        preferred_audio_codec: data.preferred_audio_codec,
        supports_fmp4: data.supports_fmp4,
        supports_mpegts: data.supports_mpegts,
        preferred_format: data.preferred_format || undefined,
      });
      await loadRules();
      // Exit create mode and select the new rule
      setIsCreating(false);
      if (created?.id) {
        // Find the created rule in the updated list and select it
        const updatedRules = await apiClient.getClientDetectionRules();
        const newRule = updatedRules.find((r) => r.id === created.id);
        if (newRule) {
          setSelectedRule(clientDetectionRuleToMasterItem(newRule));
        }
      }
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to create rule';
      setErrors((prev) => ({ ...prev, create: message }));
      throw err;
    } finally {
      setLoading((prev) => ({ ...prev, create: false }));
    }
  };

  const handleUpdate = async (id: string, data: RuleFormData) => {
    setLoading((prev) => ({ ...prev, edit: true }));
    setErrors((prev) => ({ ...prev, edit: null }));
    try {
      await apiClient.updateClientDetectionRule(id, {
        name: data.name,
        description: data.description || undefined,
        expression: data.expression,
        priority: data.priority,
        is_enabled: data.is_enabled,
        accepted_video_codecs: data.accepted_video_codecs,
        accepted_audio_codecs: data.accepted_audio_codecs,
        preferred_video_codec: data.preferred_video_codec,
        preferred_audio_codec: data.preferred_audio_codec,
        supports_fmp4: data.supports_fmp4,
        supports_mpegts: data.supports_mpegts,
        preferred_format: data.preferred_format || undefined,
      });
      await loadRules();
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to update rule';
      setErrors((prev) => ({ ...prev, edit: message }));
      throw err;
    } finally {
      setLoading((prev) => ({ ...prev, edit: false }));
    }
  };

  const handleDelete = async (rule: ClientDetectionRule) => {
    setLoading((prev) => ({ ...prev, delete: rule.id }));
    setErrors((prev) => ({ ...prev, action: null }));
    try {
      await apiClient.deleteClientDetectionRule(rule.id);
      await loadRules();
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to delete rule';
      setErrors((prev) => ({ ...prev, action: message }));
    } finally {
      setLoading((prev) => ({ ...prev, delete: null }));
      setDeleteDialog({ open: false, rule: null });
    }
  };

  const handleToggle = async (rule: ClientDetectionRule) => {
    setLoading((prev) => ({ ...prev, toggle: rule.id }));
    setErrors((prev) => ({ ...prev, action: null }));
    try {
      await apiClient.toggleClientDetectionRule(rule.id);
      await loadRules();
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to toggle rule';
      setErrors((prev) => ({ ...prev, action: message }));
    } finally {
      setLoading((prev) => ({ ...prev, toggle: null }));
    }
  };

  const moveRule = async (ruleId: string, direction: 'up' | 'down') => {
    const currentIndex = sortedRules.findIndex((r) => r.id === ruleId);
    if (currentIndex === -1) return;

    const targetIndex = direction === 'up' ? currentIndex - 1 : currentIndex + 1;
    if (targetIndex < 0 || targetIndex >= sortedRules.length) return;

    setLoading((prev) => ({ ...prev, reorder: true }));
    setErrors((prev) => ({ ...prev, action: null }));

    try {
      const newOrder = [...sortedRules];
      [newOrder[currentIndex], newOrder[targetIndex]] = [
        newOrder[targetIndex],
        newOrder[currentIndex],
      ];

      const reorderRequest = newOrder.map((rule, index) => ({
        id: rule.id,
        priority: (index + 1) * 10,
      }));

      await apiClient.reorderClientDetectionRules(reorderRequest);
      await loadRules();
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to reorder rules';
      setErrors((prev) => ({ ...prev, action: message }));
    } finally {
      setLoading((prev) => ({ ...prev, reorder: false }));
    }
  };

  // Handle drag/drop reordering
  const handleDragReorder = async (reorderedIds: string[]) => {
    setLoading((prev) => ({ ...prev, reorder: true }));
    setErrors((prev) => ({ ...prev, action: null }));

    try {
      const reorderRequest = reorderedIds.map((id, index) => ({
        id,
        priority: (index + 1) * 10,
      }));

      await apiClient.reorderClientDetectionRules(reorderRequest);
      await loadRules();
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to reorder rules';
      setErrors((prev) => ({ ...prev, action: message }));
    } finally {
      setLoading((prev) => ({ ...prev, reorder: false }));
    }
  };

  // Stats
  const totalRules = allRules.length;
  const enabledRules = allRules.filter((r) => r.is_enabled).length;
  const systemRules = allRules.filter((r) => r.is_system).length;

  return (
    <div className="flex flex-col gap-6 h-full">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <p className="text-muted-foreground">
            Configure client detection rules to optimize stream delivery based on device capabilities
          </p>
        </div>
        <div className="flex items-center gap-2">
          <ImportDialog
            importType="client_detection_rules"
            title="Import Client Detection Rules"
            onImportComplete={loadRules}
          />
          <ExportDialog
            exportType="client_detection_rules"
            items={allRules.map((r) => ({ id: r.id, name: r.name, is_system: r.is_system }))}
            title="Export Client Detection Rules"
          />
        </div>
      </div>

      {/* Stats */}
      <div className="grid gap-2 md:grid-cols-3">
        <StatCard
          title="Total Rules"
          value={totalRules}
          icon={<Users className="h-4 w-4" />}
        />
        <StatCard
          title="Enabled"
          value={enabledRules}
          icon={<CheckCircle className="h-4 w-4 text-green-600" />}
        />
        <StatCard
          title="System"
          value={systemRules}
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
          {errors.rules ? (
            <div className="p-6">
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertTitle>Failed to Load Rules</AlertTitle>
                <AlertDescription>
                  {errors.rules}
                  <Button
                    variant="outline"
                    size="sm"
                    className="ml-2"
                    onClick={loadRules}
                    disabled={loading.rules}
                  >
                    {loading.rules && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                    Retry
                  </Button>
                </AlertDescription>
              </Alert>
            </div>
          ) : (
            <MasterDetailLayout
              items={masterItems}
              selectedId={isCreating ? null : selectedRule?.id}
              onSelect={(item) => {
                setIsCreating(false);
                setSelectedRule(item);
              }}
              isLoading={loading.rules}
              title={`Client Detection Rules (${sortedRules.length})`}
              searchPlaceholder="Search by name, expression..."
              sortable={true}
              onReorder={handleDragReorder}
              headerAction={
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    setIsCreating(true);
                    setSelectedRule(null);
                    setErrors((prev) => ({ ...prev, create: null }));
                  }}
                  disabled={loading.rules}
                >
                  <Plus className="h-4 w-4" />
                  <span className="sr-only">Create Rule</span>
                </Button>
              }
              emptyState={{
                title: 'No client detection rules configured',
                description: 'Get started by creating your first client detection rule.',
              }}
              filterFn={createFuzzyFilter<ClientDetectionRuleMasterItem>({
                keys: [
                  { name: 'name', weight: 0.4 },
                  { name: 'description', weight: 0.25 },
                  { name: 'expression', weight: 0.2 },
                  { name: 'enabled', weight: 0.1 },
                  { name: 'system', weight: 0.05 },
                ],
                accessor: (item) => ({
                  name: item.rule.name,
                  description: item.rule.description || '',
                  expression: item.rule.expression,
                  enabled: item.rule.is_enabled ? 'enabled' : 'disabled',
                  system: item.rule.is_system ? 'system' : '',
                }),
              })}
            >
              {(selected) =>
                isCreating ? (
                  <ClientDetectionRuleCreatePanel
                    onCreate={handleCreate}
                    onCancel={() => setIsCreating(false)}
                    loading={loading.create}
                    error={errors.create}
                    nextPriority={nextPriority}
                  />
                ) : selected ? (
                  <ClientDetectionRuleDetailPanel
                    rule={selected.rule}
                    onUpdate={handleUpdate}
                    onDelete={async (rule) => setDeleteDialog({ open: true, rule })}
                    onToggle={handleToggle}
                    onMoveUp={(id) => moveRule(id, 'up')}
                    onMoveDown={(id) => moveRule(id, 'down')}
                    loading={{ edit: loading.edit, delete: loading.delete, toggle: loading.toggle, reorder: loading.reorder }}
                    error={errors.edit}
                    isFirst={sortedRules.findIndex((r) => r.id === selected.rule.id) === 0}
                    isLast={sortedRules.findIndex((r) => r.id === selected.rule.id) === sortedRules.length - 1}
                  />
                ) : (
                  <DetailEmpty
                    icon={<Users className="h-12 w-12" />}
                    title="Select a Client Detection Rule"
                    description="Choose a rule from the list to view and edit its configuration."
                  />
                )
              }
            </MasterDetailLayout>
          )}
        </CardContent>
      </Card>

      {/* Delete Confirmation Dialog */}
      <Dialog open={deleteDialog.open} onOpenChange={(open) => setDeleteDialog({ open, rule: null })}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Client Detection Rule</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete &quot;{deleteDialog.rule?.name}&quot;? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteDialog({ open: false, rule: null })}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => deleteDialog.rule && handleDelete(deleteDialog.rule)}
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
