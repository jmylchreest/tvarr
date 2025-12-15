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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  Plus,
  Edit,
  Trash2,
  Search,
  Loader2,
  Lock,
  Users,
  GripVertical,
  ArrowUp,
  ArrowDown,
  Grid,
  List,
  Table as TableIcon,
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
import { ExportDialog, ImportDialog } from '@/components/config-export';
import { ClientDetectionExpressionEditor } from '@/components/client-detection-expression-editor';

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
const FORMAT_OPTIONS = [
  { value: 'auto', label: 'Auto' },
  { value: 'hls-fmp4', label: 'HLS (fMP4)' },
  { value: 'hls-ts', label: 'HLS (MPEG-TS)' },
  { value: 'dash', label: 'DASH' },
];

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

function RuleFormSheet({
  rule,
  onSave,
  loading,
  error,
  trigger,
  title,
  description,
  nextPriority,
}: {
  rule?: ClientDetectionRule;
  onSave: (data: RuleFormData) => Promise<void>;
  loading: boolean;
  error: string | null;
  trigger: React.ReactNode;
  title: string;
  description: string;
  nextPriority: number;
}) {
  const [open, setOpen] = useState(false);
  const [formData, setFormData] = useState<RuleFormData>(defaultFormData);
  const [testUserAgent, setTestUserAgent] = useState('');
  const [testResult, setTestResult] = useState<{ matches: boolean; error?: string } | null>(null);
  const [testing, setTesting] = useState(false);

  useEffect(() => {
    if (rule) {
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
    } else {
      setFormData({ ...defaultFormData, priority: nextPriority });
    }
    setTestResult(null);
  }, [rule, open, nextPriority]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    await onSave(formData);
    if (!error) {
      setOpen(false);
    }
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
                placeholder="Chrome Browser"
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
                placeholder="Matches Chrome browser User-Agent strings"
                rows={2}
                disabled={loading}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="expression">Match Expression *</Label>
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

          <SheetFooter>
            <Button type="submit" disabled={loading}>
              {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {rule ? 'Update Rule' : 'Create Rule'}
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  );
}

export function ClientDetectionRules() {
  const [allRules, setAllRules] = useState<ClientDetectionRule[]>([]);
  const [searchTerm, setSearchTerm] = useState('');
  const [filterEnabled, setFilterEnabled] = useState<string>('all');
  const [viewMode, setViewMode] = useState<'grid' | 'list' | 'table'>('table');
  const [draggedItem, setDraggedItem] = useState<ClientDetectionRule | null>(null);
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

  // Local filtering
  const filteredRules = useMemo(() => {
    let filtered = allRules;

    // Filter by enabled status
    if (filterEnabled === 'enabled') {
      filtered = filtered.filter((r) => r.is_enabled);
    } else if (filterEnabled === 'disabled') {
      filtered = filtered.filter((r) => !r.is_enabled);
    }

    // Search term
    if (searchTerm.trim()) {
      const searchLower = searchTerm.toLowerCase();
      filtered = filtered.filter(
        (r) =>
          r.name.toLowerCase().includes(searchLower) ||
          r.description?.toLowerCase().includes(searchLower) ||
          r.expression.toLowerCase().includes(searchLower)
      );
    }

    return filtered;
  }, [allRules, searchTerm, filterEnabled]);

  const nextPriority = useMemo(() => {
    if (allRules.length === 0) return 100;
    return Math.max(...allRules.map((r) => r.priority)) + 10;
  }, [allRules]);

  const handleCreate = async (data: RuleFormData) => {
    setLoading((prev) => ({ ...prev, create: true }));
    setErrors((prev) => ({ ...prev, create: null }));
    try {
      await apiClient.createClientDetectionRule({
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
    const currentIndex = filteredRules.findIndex((r) => r.id === ruleId);
    if (currentIndex === -1) return;

    const targetIndex = direction === 'up' ? currentIndex - 1 : currentIndex + 1;
    if (targetIndex < 0 || targetIndex >= filteredRules.length) return;

    setLoading((prev) => ({ ...prev, reorder: true }));
    setErrors((prev) => ({ ...prev, action: null }));

    try {
      const newOrder = [...filteredRules];
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

  const handleDragStart = (e: React.DragEvent, rule: ClientDetectionRule) => {
    setDraggedItem(rule);
    e.dataTransfer.effectAllowed = 'move';
  };

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
  };

  const handleDrop = async (e: React.DragEvent, targetRule: ClientDetectionRule) => {
    e.preventDefault();
    if (!draggedItem || draggedItem.id === targetRule.id) {
      setDraggedItem(null);
      return;
    }

    setLoading((prev) => ({ ...prev, reorder: true }));
    setErrors((prev) => ({ ...prev, action: null }));

    try {
      const draggedIndex = filteredRules.findIndex((r) => r.id === draggedItem.id);
      const targetIndex = filteredRules.findIndex((r) => r.id === targetRule.id);

      if (draggedIndex === -1 || targetIndex === -1) return;

      const newOrder = [...filteredRules];
      newOrder.splice(draggedIndex, 1);
      newOrder.splice(targetIndex, 0, draggedItem);

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
      setDraggedItem(null);
    }
  };

  // Stats
  const totalRules = allRules.length;
  const enabledRules = allRules.filter((r) => r.is_enabled).length;
  const systemRules = allRules.filter((r) => r.is_system).length;

  return (
    <div className="space-y-6">
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
          <RuleFormSheet
            onSave={handleCreate}
            loading={loading.create}
            error={errors.create}
            title="Create Client Detection Rule"
            description="Create a rule to detect client capabilities and optimize stream delivery"
            nextPriority={nextPriority}
            trigger={
              <Button className="gap-2">
                <Plus className="h-4 w-4" />
                Create Rule
              </Button>
            }
          />
        </div>
      </div>

      {/* Stats */}
      <div className="grid gap-4 md:grid-cols-3">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Total Rules</CardTitle>
            <Users className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{totalRules}</div>
            <p className="text-xs text-muted-foreground">Detection rules configured</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Enabled</CardTitle>
            <CheckCircle className="h-4 w-4 text-green-600" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{enabledRules}</div>
            <p className="text-xs text-muted-foreground">Active rules</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">System</CardTitle>
            <Lock className="h-4 w-4 text-purple-600" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{systemRules}</div>
            <p className="text-xs text-muted-foreground">Built-in rules</p>
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
                  placeholder="Search rules by name, expression..."
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  className="pl-8"
                  disabled={loading.rules}
                />
              </div>
            </div>
            <Select value={filterEnabled} onValueChange={setFilterEnabled} disabled={loading.rules}>
              <SelectTrigger className="w-full sm:w-[180px]">
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Status</SelectItem>
                <SelectItem value="enabled">Enabled</SelectItem>
                <SelectItem value="disabled">Disabled</SelectItem>
              </SelectContent>
            </Select>

            <div className="flex gap-1 border rounded-md p-1">
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant={viewMode === 'table' ? 'secondary' : 'ghost'}
                    size="sm"
                    onClick={() => setViewMode('table')}
                    className="h-8 w-8 p-0"
                  >
                    <TableIcon className="h-4 w-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Table View</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant={viewMode === 'grid' ? 'secondary' : 'ghost'}
                    size="sm"
                    onClick={() => setViewMode('grid')}
                    className="h-8 w-8 p-0"
                  >
                    <Grid className="h-4 w-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Grid View</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant={viewMode === 'list' ? 'secondary' : 'ghost'}
                    size="sm"
                    onClick={() => setViewMode('list')}
                    className="h-8 w-8 p-0"
                  >
                    <List className="h-4 w-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>List View</TooltipContent>
              </Tooltip>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Error Display */}
      {errors.action && (
        <div className="bg-destructive/10 text-destructive px-4 py-3 rounded-md text-sm">
          {errors.action}
        </div>
      )}

      {/* Rules List */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center justify-between">
            <span>
              Client Detection Rules ({filteredRules.length}
              {searchTerm || filterEnabled !== 'all' ? ` of ${totalRules}` : ''})
            </span>
            {(loading.rules || loading.reorder) && <Loader2 className="h-4 w-4 animate-spin" />}
          </CardTitle>
          <CardDescription>
            Drag and drop to reorder rules. Lower priority values are evaluated first.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {loading.rules && filteredRules.length === 0 ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
            </div>
          ) : filteredRules.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">
              {searchTerm || filterEnabled !== 'all' ? 'No rules match your filters' : 'No client detection rules configured'}
            </div>
          ) : viewMode === 'table' ? (
            <div className="space-y-2">
              {/* Column Headers */}
              <div className="flex items-center gap-4 px-4 py-2 text-xs font-medium text-muted-foreground uppercase tracking-wider border-b">
                <div className="w-4"></div>
                <div className="flex-1">Rule</div>
                <div className="w-12 text-center">Status</div>
                <div className="w-36 text-right">Actions</div>
              </div>

              {filteredRules.map((rule, index) => (
                <Card
                  key={rule.id}
                  draggable
                  onDragStart={(e) => handleDragStart(e, rule)}
                  onDragOver={handleDragOver}
                  onDrop={(e) => handleDrop(e, rule)}
                  className={`${draggedItem?.id === rule.id ? 'opacity-50' : ''} ${!rule.is_enabled ? 'opacity-60' : ''}`}
                >
                  <CardContent className="py-4">
                    <div className="flex items-start gap-4">
                      <GripVertical className="h-4 w-4 text-muted-foreground cursor-grab mt-1 flex-shrink-0" />

                      {/* Rule Info */}
                      <div className="flex-1 min-w-0 space-y-1.5">
                        <div className="flex items-center gap-2 flex-wrap">
                          <span className="font-medium">{rule.name}</span>
                          <Badge variant="outline" className="text-xs">{formatCodec(rule.preferred_video_codec)}</Badge>
                          <Badge variant="outline" className="text-xs">{formatCodec(rule.preferred_audio_codec)}</Badge>
                          {rule.supports_fmp4 && <Badge variant="outline" className="text-xs">fMP4</Badge>}
                          {rule.supports_mpegts && <Badge variant="outline" className="text-xs">MPEG-TS</Badge>}
                          {!rule.is_enabled && (
                            <Badge variant="outline" className="text-muted-foreground">
                              Disabled
                            </Badge>
                          )}
                          {rule.is_system && (
                            <Badge variant="outline" className="text-purple-600 border-purple-600">
                              <Lock className="h-3 w-3 mr-1" />
                              System
                            </Badge>
                          )}
                        </div>
                        <CopyableExpression
                          expression={rule.expression}
                          className="text-muted-foreground"
                        />
                      </div>

                      {/* Status */}
                      <div className="w-12 flex justify-center flex-shrink-0">
                        <Switch
                          checked={rule.is_enabled}
                          onCheckedChange={() => handleToggle(rule)}
                          disabled={loading.toggle === rule.id}
                        />
                      </div>

                      {/* Actions */}
                      <div className="w-36 flex items-center justify-end gap-1 flex-shrink-0">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => moveRule(rule.id, 'up')}
                          disabled={index === 0 || loading.reorder}
                          className="h-8 w-8 p-0"
                        >
                          <ArrowUp className="h-4 w-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => moveRule(rule.id, 'down')}
                          disabled={index === filteredRules.length - 1 || loading.reorder}
                          className="h-8 w-8 p-0"
                        >
                          <ArrowDown className="h-4 w-4" />
                        </Button>
                        <RuleFormSheet
                          rule={rule}
                          onSave={(data) => handleUpdate(rule.id, data)}
                          loading={loading.edit}
                          error={errors.edit}
                          title="Edit Client Detection Rule"
                          description="Update the rule configuration"
                          nextPriority={nextPriority}
                          trigger={
                            <Button variant="ghost" size="sm" disabled={rule.is_system} className="h-8 w-8 p-0">
                              <Edit className="h-4 w-4" />
                            </Button>
                          }
                        />
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setDeleteDialog({ open: true, rule })}
                          disabled={rule.is_system || loading.delete === rule.id}
                          className="h-8 w-8 p-0 text-destructive hover:text-destructive"
                        >
                          {loading.delete === rule.id ? (
                            <Loader2 className="h-4 w-4 animate-spin" />
                          ) : (
                            <Trash2 className="h-4 w-4" />
                          )}
                        </Button>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          ) : viewMode === 'grid' ? (
            <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
              {filteredRules.map((rule, index) => (
                <Card
                  key={rule.id}
                  draggable
                  onDragStart={(e) => handleDragStart(e, rule)}
                  onDragOver={handleDragOver}
                  onDrop={(e) => handleDrop(e, rule)}
                  className={`${draggedItem?.id === rule.id ? 'opacity-50' : ''} ${!rule.is_enabled ? 'opacity-60' : ''}`}
                >
                  <CardHeader className="pb-3">
                    <div className="flex items-start justify-between">
                      <div className="flex items-start gap-2">
                        <GripVertical className="h-4 w-4 text-muted-foreground cursor-grab mt-1" />
                        <div>
                          <CardTitle className="text-base">{rule.name}</CardTitle>
                          <div className="flex items-center gap-1 mt-1 flex-wrap">
                            <Badge variant="outline" className="text-xs">{formatCodec(rule.preferred_video_codec)}</Badge>
                            <Badge variant="outline" className="text-xs">{formatCodec(rule.preferred_audio_codec)}</Badge>
                            {rule.supports_fmp4 && <Badge variant="outline" className="text-xs">fMP4</Badge>}
                            {rule.supports_mpegts && <Badge variant="outline" className="text-xs">MPEG-TS</Badge>}
                            {rule.is_system && (
                              <Badge variant="outline" className="text-purple-600 border-purple-600">
                                <Lock className="h-3 w-3 mr-1" />
                                System
                              </Badge>
                            )}
                          </div>
                        </div>
                      </div>
                      <Switch
                        checked={rule.is_enabled}
                        onCheckedChange={() => handleToggle(rule)}
                        disabled={loading.toggle === rule.id}
                      />
                    </div>
                    {rule.description && (
                      <CardDescription className="line-clamp-2">{rule.description}</CardDescription>
                    )}
                  </CardHeader>
                  <CardContent className="space-y-3">
                    <CopyableExpression
                      expression={rule.expression}
                      className="text-muted-foreground"
                    />
                    <div className="flex justify-end gap-1 pt-2 border-t">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => moveRule(rule.id, 'up')}
                        disabled={index === 0 || loading.reorder}
                        className="h-8 w-8 p-0"
                      >
                        <ArrowUp className="h-4 w-4" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => moveRule(rule.id, 'down')}
                        disabled={index === filteredRules.length - 1 || loading.reorder}
                        className="h-8 w-8 p-0"
                      >
                        <ArrowDown className="h-4 w-4" />
                      </Button>
                      <RuleFormSheet
                        rule={rule}
                        onSave={(data) => handleUpdate(rule.id, data)}
                        loading={loading.edit}
                        error={errors.edit}
                        title="Edit Client Detection Rule"
                        description="Update the rule configuration"
                        nextPriority={nextPriority}
                        trigger={
                          <Button variant="ghost" size="sm" disabled={rule.is_system} className="h-8 w-8 p-0">
                            <Edit className="h-4 w-4" />
                          </Button>
                        }
                      />
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => setDeleteDialog({ open: true, rule })}
                        disabled={rule.is_system || loading.delete === rule.id}
                        className="h-8 w-8 p-0 text-destructive hover:text-destructive"
                      >
                        {loading.delete === rule.id ? (
                          <Loader2 className="h-4 w-4 animate-spin" />
                        ) : (
                          <Trash2 className="h-4 w-4" />
                        )}
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
          ) : (
            <div className="space-y-2">
              {filteredRules.map((rule, index) => (
                <Card
                  key={rule.id}
                  draggable
                  onDragStart={(e) => handleDragStart(e, rule)}
                  onDragOver={handleDragOver}
                  onDrop={(e) => handleDrop(e, rule)}
                  className={`${draggedItem?.id === rule.id ? 'opacity-50' : ''} ${!rule.is_enabled ? 'opacity-60' : ''}`}
                >
                  <CardContent className="py-3">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-2 flex-1 min-w-0">
                        <GripVertical className="h-4 w-4 text-muted-foreground cursor-grab flex-shrink-0" />
                        <span className="font-medium">{rule.name}</span>
                        <Badge variant="outline" className="text-xs">{formatCodec(rule.preferred_video_codec)}</Badge>
                        <Badge variant="outline" className="text-xs">{formatCodec(rule.preferred_audio_codec)}</Badge>
                        {rule.supports_fmp4 && <Badge variant="outline" className="text-xs">fMP4</Badge>}
                        {rule.supports_mpegts && <Badge variant="outline" className="text-xs">MPEG-TS</Badge>}
                        {!rule.is_enabled && (
                          <Badge variant="outline" className="text-muted-foreground">Disabled</Badge>
                        )}
                        {rule.is_system && (
                          <Badge variant="outline" className="text-purple-600 border-purple-600">
                            <Lock className="h-3 w-3 mr-1" />
                            System
                          </Badge>
                        )}
                        <span className="hidden md:inline-flex">
                          <CopyableExpression
                            expression={rule.expression}
                            className="text-muted-foreground"
                            maxWidth="200px"
                          />
                        </span>
                      </div>
                      <div className="flex items-center gap-2 flex-shrink-0">
                        <Switch
                          checked={rule.is_enabled}
                          onCheckedChange={() => handleToggle(rule)}
                          disabled={loading.toggle === rule.id}
                        />
                        <div className="flex gap-1">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => moveRule(rule.id, 'up')}
                            disabled={index === 0 || loading.reorder}
                            className="h-8 w-8 p-0"
                          >
                            <ArrowUp className="h-4 w-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => moveRule(rule.id, 'down')}
                            disabled={index === filteredRules.length - 1 || loading.reorder}
                            className="h-8 w-8 p-0"
                          >
                            <ArrowDown className="h-4 w-4" />
                          </Button>
                          <RuleFormSheet
                            rule={rule}
                            onSave={(data) => handleUpdate(rule.id, data)}
                            loading={loading.edit}
                            error={errors.edit}
                            title="Edit Client Detection Rule"
                            description="Update the rule configuration"
                            nextPriority={nextPriority}
                            trigger={
                              <Button variant="ghost" size="sm" disabled={rule.is_system} className="h-8 w-8 p-0">
                                <Edit className="h-4 w-4" />
                              </Button>
                            }
                          />
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => setDeleteDialog({ open: true, rule })}
                            disabled={rule.is_system || loading.delete === rule.id}
                            className="h-8 w-8 p-0 text-destructive hover:text-destructive"
                          >
                            {loading.delete === rule.id ? (
                              <Loader2 className="h-4 w-4 animate-spin" />
                            ) : (
                              <Trash2 className="h-4 w-4" />
                            )}
                          </Button>
                        </div>
                      </div>
                    </div>
                  </CardContent>
                </Card>
              ))}
            </div>
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
