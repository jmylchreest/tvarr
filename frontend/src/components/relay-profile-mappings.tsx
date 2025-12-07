'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Switch } from '@/components/ui/switch';
import { ExpressionEditor } from '@/components/expression-editor';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
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
  AlertCircle,
  Loader2,
  Lock,
  Users,
  Smartphone,
  Monitor,
  Tv,
  Globe,
  GripVertical,
  Shield,
  Check,
  X,
  ChevronDown,
  ChevronUp,
} from 'lucide-react';
import { RelayProfileMapping, CreateRelayProfileMappingRequest, UpdateRelayProfileMappingRequest, ContainerFormat } from '@/types/api';
import { apiClient, ApiError } from '@/lib/api-client';
import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  DragEndEvent,
} from '@dnd-kit/core';
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';

// Available codecs for selection
const AVAILABLE_VIDEO_CODECS = [
  { value: 'h264', label: 'H.264' },
  { value: 'hevc', label: 'HEVC (H.265)' },
  { value: 'vp9', label: 'VP9' },
  { value: 'av1', label: 'AV1' },
];

const AVAILABLE_AUDIO_CODECS = [
  { value: 'aac', label: 'AAC' },
  { value: 'opus', label: 'Opus' },
  { value: 'mp3', label: 'MP3' },
  { value: 'ac3', label: 'AC3' },
  { value: 'eac3', label: 'E-AC3' },
];

const AVAILABLE_CONTAINERS = [
  { value: 'mpegts', label: 'MPEG-TS' },
  { value: 'fmp4', label: 'fMP4' },
];

// Expression validation hook with debouncing
function useExpressionValidation(expression: string, delay = 500) {
  const [validationState, setValidationState] = useState<{
    isValid: boolean | null;
    error: string | null;
    isValidating: boolean;
  }>({ isValid: null, error: null, isValidating: false });
  const timeoutRef = useRef<NodeJS.Timeout | null>(null);

  useEffect(() => {
    // Clear previous timeout
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
    }

    // Don't validate empty expressions
    if (!expression.trim()) {
      setValidationState({ isValid: null, error: null, isValidating: false });
      return;
    }

    setValidationState((prev) => ({ ...prev, isValidating: true }));

    // Debounce the validation
    timeoutRef.current = setTimeout(async () => {
      try {
        const result = await apiClient.testRelayProfileMappingExpression({
          expression,
          test_data: { user_agent: 'Test/1.0' },
        });

        if (result.error) {
          setValidationState({ isValid: false, error: result.error, isValidating: false });
        } else {
          setValidationState({ isValid: true, error: null, isValidating: false });
        }
      } catch {
        setValidationState({ isValid: false, error: 'Failed to validate expression', isValidating: false });
      }
    }, delay);

    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
      }
    };
  }, [expression, delay]);

  return validationState;
}

interface LoadingState {
  mappings: boolean;
  create: boolean;
  edit: boolean;
  delete: string | null;
  reorder: boolean;
}

interface ErrorState {
  mappings: string | null;
  create: string | null;
  edit: string | null;
  action: string | null;
}

function getMappingIcon(name: string) {
  const nameLower = name.toLowerCase();
  if (nameLower.includes('browser') || nameLower.includes('chrome') || nameLower.includes('firefox') || nameLower.includes('safari') || nameLower.includes('edge')) {
    return <Globe className="h-4 w-4" />;
  }
  if (nameLower.includes('mobile') || nameLower.includes('android') || nameLower.includes('ios') || nameLower.includes('iphone') || nameLower.includes('ipad')) {
    return <Smartphone className="h-4 w-4" />;
  }
  if (nameLower.includes('tv') || nameLower.includes('roku') || nameLower.includes('fire') || nameLower.includes('chromecast') || nameLower.includes('tizen') || nameLower.includes('webos')) {
    return <Tv className="h-4 w-4" />;
  }
  if (nameLower.includes('vlc') || nameLower.includes('mpv') || nameLower.includes('player') || nameLower.includes('ffmpeg') || nameLower.includes('kodi')) {
    return <Monitor className="h-4 w-4" />;
  }
  return <Users className="h-4 w-4" />;
}

// Sortable table row component
function SortableRow({
  mapping,
  onToggleEnabled,
  onDelete,
  loading,
}: {
  mapping: RelayProfileMapping;
  onToggleEnabled: (mapping: RelayProfileMapping) => void;
  onDelete: (id: string) => void;
  loading: LoadingState;
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: mapping.id });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };

  return (
    <TableRow
      ref={setNodeRef}
      style={style}
      className={`${mapping.is_system ? 'bg-muted/50' : ''} ${isDragging ? 'z-10' : ''}`}
    >
      <TableCell>
        <div className="flex items-center gap-2">
          <button
            {...attributes}
            {...listeners}
            className="cursor-grab active:cursor-grabbing p-1 hover:bg-muted rounded touch-none"
            disabled={loading.reorder}
          >
            <GripVertical className="h-4 w-4 text-muted-foreground" />
          </button>
          <span className="text-xs text-muted-foreground font-mono">
            {mapping.priority}
          </span>
        </div>
      </TableCell>
      <TableCell>
        <div className="flex items-center gap-2">
          {getMappingIcon(mapping.name)}
          <div>
            <div className="font-medium flex items-center gap-1">
              {mapping.name}
              {mapping.is_system && (
                <Lock className="h-3 w-3 text-muted-foreground" />
              )}
            </div>
            {mapping.description && (
              <div className="text-xs text-muted-foreground">
                {mapping.description}
              </div>
            )}
          </div>
        </div>
      </TableCell>
      <TableCell>
        <code className="text-xs bg-muted px-2 py-1 rounded">
          {mapping.expression.length > 50
            ? `${mapping.expression.slice(0, 50)}...`
            : mapping.expression}
        </code>
      </TableCell>
      <TableCell>
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger asChild>
              <div className="flex flex-wrap gap-1 cursor-help">
                <Badge variant="outline" className="text-xs">
                  {mapping.preferred_video_codec.toUpperCase()}
                </Badge>
                <Badge variant="outline" className="text-xs">
                  {mapping.preferred_audio_codec.toUpperCase()}
                </Badge>
                <Badge variant="secondary" className="text-xs">
                  {mapping.preferred_container.toUpperCase()}
                </Badge>
              </div>
            </TooltipTrigger>
            <TooltipContent side="bottom" className="max-w-xs">
              <div className="space-y-2 text-xs">
                <p className="font-medium">Accepted Codecs (Passthrough)</p>
                <div className="space-y-1">
                  <p><span className="opacity-70">Video:</span> {mapping.accepted_video_codecs?.map(c => c.toUpperCase()).join(', ') || 'none'}</p>
                  <p><span className="opacity-70">Audio:</span> {mapping.accepted_audio_codecs?.map(c => c.toUpperCase()).join(', ') || 'none'}</p>
                  <p><span className="opacity-70">Container:</span> {mapping.accepted_containers?.map(c => c.toUpperCase()).join(', ') || 'none'}</p>
                </div>
              </div>
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      </TableCell>
      <TableCell>
        <Switch
          checked={mapping.is_enabled}
          onCheckedChange={() => onToggleEnabled(mapping)}
          disabled={loading.reorder}
        />
      </TableCell>
      <TableCell>
        <div className="flex gap-1">
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8"
            disabled={mapping.is_system}
            title={mapping.is_system ? 'Cannot edit system mappings' : 'Edit'}
          >
            <Edit className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8 text-destructive"
            disabled={mapping.is_system || loading.delete === mapping.id}
            onClick={() => onDelete(mapping.id)}
            title={mapping.is_system ? 'Cannot delete system mappings' : 'Delete'}
          >
            {loading.delete === mapping.id ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Trash2 className="h-4 w-4" />
            )}
          </Button>
        </div>
      </TableCell>
    </TableRow>
  );
}

function CreateMappingSheet({
  onCreateMapping,
  loading,
  error,
}: {
  onCreateMapping: (mapping: CreateRelayProfileMappingRequest) => Promise<void>;
  loading: boolean;
  error: string | null;
}) {
  const [open, setOpen] = useState(false);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [showTestSection, setShowTestSection] = useState(false);
  const [testData, setTestData] = useState<Record<string, string>>({
    user_agent: typeof navigator !== 'undefined' ? navigator.userAgent : '',
    client_ip: '127.0.0.1',
    request_path: '/proxy/channel/12345',
    request_url: '',
    query_params: '',
    x_forwarded_for: '',
    x_real_ip: '',
    accept: '*/*',
    accept_language: typeof navigator !== 'undefined' ? navigator.language : 'en-US',
    host: typeof window !== 'undefined' ? window.location.host : 'localhost:8080',
    referer: '',
  });
  const [testResult, setTestResult] = useState<{ matches: boolean; error?: string } | null>(null);
  const [testLoading, setTestLoading] = useState(false);
  const [formData, setFormData] = useState<CreateRelayProfileMappingRequest>({
    name: '',
    expression: '',
    description: '',
    is_enabled: true,
    accepted_video_codecs: ['h264', 'hevc', 'vp9', 'av1'],
    accepted_audio_codecs: ['aac', 'opus', 'mp3'],
    accepted_containers: ['mpegts', 'fmp4'],
    preferred_video_codec: 'h264',
    preferred_audio_codec: 'aac',
    preferred_container: 'mpegts',
  });

  // Expression validation
  const expressionValidation = useExpressionValidation(formData.expression);

  // Test expression with sample data
  const handleTestExpression = async () => {
    if (!formData.expression.trim()) return;

    setTestLoading(true);
    setTestResult(null);

    try {
      const result = await apiClient.testRelayProfileMappingExpression({
        expression: formData.expression,
        test_data: testData,
      });

      if (result.error) {
        setTestResult({ matches: false, error: result.error });
      } else {
        setTestResult({ matches: result.matches });
      }
    } catch {
      setTestResult({ matches: false, error: 'Failed to test expression' });
    } finally {
      setTestLoading(false);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    await onCreateMapping(formData);
    if (!error) {
      setOpen(false);
      setShowAdvanced(false);
      setFormData({
        name: '',
        expression: '',
        description: '',
        is_enabled: true,
        accepted_video_codecs: ['h264', 'hevc', 'vp9', 'av1'],
        accepted_audio_codecs: ['aac', 'opus', 'mp3'],
        accepted_containers: ['mpegts', 'fmp4'],
        preferred_video_codec: 'h264',
        preferred_audio_codec: 'aac',
        preferred_container: 'mpegts',
      });
    }
  };

  const toggleCodec = (list: string[] | undefined, codec: string, type: 'video' | 'audio' | 'container') => {
    const currentList = list || [];
    const newList = currentList.includes(codec)
      ? currentList.filter((c) => c !== codec)
      : [...currentList, codec];

    if (type === 'video') {
      setFormData({ ...formData, accepted_video_codecs: newList });
    } else if (type === 'audio') {
      setFormData({ ...formData, accepted_audio_codecs: newList });
    } else {
      setFormData({ ...formData, accepted_containers: newList });
    }
  };

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <Button className="gap-2">
          <Plus className="h-4 w-4" />
          Create Mapping
        </Button>
      </SheetTrigger>
      <SheetContent side="right" className="w-full sm:max-w-2xl overflow-y-auto">
        <SheetHeader>
          <SheetTitle>Create Client Detection Mapping</SheetTitle>
          <SheetDescription>
            Create a new rule to match clients based on request properties
          </SheetDescription>
        </SheetHeader>

        {error && (
          <Alert variant="destructive" className="mt-4">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>Error</AlertTitle>
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}

        <form id="create-mapping-form" onSubmit={handleSubmit} className="space-y-4 p-4">
          <div className="space-y-2">
            <Label htmlFor="name">Name</Label>
            <Input
              id="name"
              value={formData.name}
              onChange={(e) => setFormData({ ...formData, name: e.target.value })}
              placeholder="e.g., Chrome Browser"
              required
              disabled={loading}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="expression">Expression</Label>
            <div className="relative">
              <ExpressionEditor
                value={formData.expression}
                onChange={(value) => setFormData({ ...formData, expression: value })}
                fieldsEndpoint="/client-detection/fields"
                sourceType="client"
                enableValidation={false}
                enableAutocomplete={true}
                placeholder='e.g., user_agent contains "Chrome"'
                disabled={loading}
                rows={3}
                className={`text-sm pr-10 min-h-0 ${
                  expressionValidation.isValid === false
                    ? 'border-destructive focus-visible:ring-destructive'
                    : expressionValidation.isValid === true
                    ? 'border-green-500 focus-visible:ring-green-500'
                    : ''
                }`}
              />
              {formData.expression && (
                <div className="absolute right-2 top-2 z-10">
                  {expressionValidation.isValidating ? (
                    <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
                  ) : expressionValidation.isValid === true ? (
                    <Check className="h-4 w-4 text-green-500" />
                  ) : expressionValidation.isValid === false ? (
                    <X className="h-4 w-4 text-destructive" />
                  ) : null}
                </div>
              )}
            </div>
            {expressionValidation.error && (
              <p className="text-xs text-destructive">{expressionValidation.error}</p>
            )}
            <p className="text-xs text-muted-foreground">
              Start typing a field name to see autocomplete suggestions. Use Tab or Enter to select.
            </p>

            {/* Collapsible Test Section */}
            <div className="border rounded-md mt-2">
              <button
                type="button"
                onClick={() => setShowTestSection(!showTestSection)}
                className="flex w-full items-center justify-between p-3 text-sm font-medium hover:bg-muted/50"
              >
                <span>Test Expression with Sample Data</span>
                {showTestSection ? (
                  <ChevronUp className="h-4 w-4" />
                ) : (
                  <ChevronDown className="h-4 w-4" />
                )}
              </button>
              {showTestSection && (
                <div className="p-4 pt-0 space-y-4 border-t">
                  <p className="text-xs text-muted-foreground">
                    Test your expression against sample request data. User-Agent is pre-populated from your browser.
                  </p>

                  <div className="grid grid-cols-2 gap-3">
                    <div className="space-y-1">
                      <Label htmlFor="test_user_agent" className="text-xs">user_agent</Label>
                      <Input
                        id="test_user_agent"
                        value={testData.user_agent}
                        onChange={(e) => setTestData({ ...testData, user_agent: e.target.value })}
                        className="text-xs h-8"
                        placeholder="Browser User-Agent"
                      />
                    </div>
                    <div className="space-y-1">
                      <Label htmlFor="test_client_ip" className="text-xs">client_ip</Label>
                      <Input
                        id="test_client_ip"
                        value={testData.client_ip}
                        onChange={(e) => setTestData({ ...testData, client_ip: e.target.value })}
                        className="text-xs h-8"
                        placeholder="127.0.0.1"
                      />
                    </div>
                    <div className="space-y-1">
                      <Label htmlFor="test_request_path" className="text-xs">request_path</Label>
                      <Input
                        id="test_request_path"
                        value={testData.request_path}
                        onChange={(e) => setTestData({ ...testData, request_path: e.target.value })}
                        className="text-xs h-8"
                        placeholder="/proxy/channel/123"
                      />
                    </div>
                    <div className="space-y-1">
                      <Label htmlFor="test_host" className="text-xs">host</Label>
                      <Input
                        id="test_host"
                        value={testData.host}
                        onChange={(e) => setTestData({ ...testData, host: e.target.value })}
                        className="text-xs h-8"
                        placeholder="localhost:8080"
                      />
                    </div>
                    <div className="space-y-1">
                      <Label htmlFor="test_accept" className="text-xs">accept</Label>
                      <Input
                        id="test_accept"
                        value={testData.accept}
                        onChange={(e) => setTestData({ ...testData, accept: e.target.value })}
                        className="text-xs h-8"
                        placeholder="*/*"
                      />
                    </div>
                    <div className="space-y-1">
                      <Label htmlFor="test_accept_language" className="text-xs">accept_language</Label>
                      <Input
                        id="test_accept_language"
                        value={testData.accept_language}
                        onChange={(e) => setTestData({ ...testData, accept_language: e.target.value })}
                        className="text-xs h-8"
                        placeholder="en-US"
                      />
                    </div>
                    <div className="space-y-1">
                      <Label htmlFor="test_x_forwarded_for" className="text-xs">x_forwarded_for</Label>
                      <Input
                        id="test_x_forwarded_for"
                        value={testData.x_forwarded_for}
                        onChange={(e) => setTestData({ ...testData, x_forwarded_for: e.target.value })}
                        className="text-xs h-8"
                        placeholder="Optional"
                      />
                    </div>
                    <div className="space-y-1">
                      <Label htmlFor="test_referer" className="text-xs">referer</Label>
                      <Input
                        id="test_referer"
                        value={testData.referer}
                        onChange={(e) => setTestData({ ...testData, referer: e.target.value })}
                        className="text-xs h-8"
                        placeholder="Optional"
                      />
                    </div>
                  </div>

                  <div className="flex items-center gap-3">
                    <Button
                      type="button"
                      variant="secondary"
                      size="sm"
                      onClick={handleTestExpression}
                      disabled={testLoading || !formData.expression.trim()}
                      className="gap-2"
                    >
                      {testLoading && <Loader2 className="h-3 w-3 animate-spin" />}
                      Test Expression
                    </Button>

                    {testResult && (
                      <div className={`flex items-center gap-2 text-sm ${testResult.matches ? 'text-green-600' : 'text-destructive'}`}>
                        {testResult.error ? (
                          <>
                            <X className="h-4 w-4" />
                            <span>{testResult.error}</span>
                          </>
                        ) : testResult.matches ? (
                          <>
                            <Check className="h-4 w-4" />
                            <span>Expression matches!</span>
                          </>
                        ) : (
                          <>
                            <X className="h-4 w-4" />
                            <span>Expression does not match</span>
                          </>
                        )}
                      </div>
                    )}
                  </div>
                </div>
              )}
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="description">Description (Optional)</Label>
            <Textarea
              id="description"
              value={formData.description || ''}
              onChange={(e) => setFormData({ ...formData, description: e.target.value })}
              placeholder="Describe what this mapping does..."
              disabled={loading}
              rows={2}
            />
          </div>

          <div className="flex items-center space-x-2">
            <Switch
              id="is_enabled"
              checked={formData.is_enabled}
              onCheckedChange={(checked) => setFormData({ ...formData, is_enabled: checked })}
              disabled={loading}
            />
            <Label htmlFor="is_enabled">Enabled</Label>
          </div>

          <div className="space-y-2">
            <Label className="text-sm font-medium">Preferred Codecs (Transcode Target)</Label>
            <p className="text-xs text-muted-foreground">When source cannot be copied, transcode to these codecs.</p>
            <div className="grid grid-cols-3 gap-4">
              <div className="space-y-2">
                <Label htmlFor="preferred_video_codec" className="text-xs text-muted-foreground">Video</Label>
                <select
                  id="preferred_video_codec"
                  value={formData.preferred_video_codec || 'h264'}
                  onChange={(e) => setFormData({ ...formData, preferred_video_codec: e.target.value })}
                  className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm"
                  disabled={loading}
                >
                  <option value="copy">Copy</option>
                  <option value="h264">H.264</option>
                  <option value="hevc">HEVC</option>
                  <option value="vp9">VP9</option>
                  <option value="av1">AV1</option>
                </select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="preferred_audio_codec" className="text-xs text-muted-foreground">Audio</Label>
                <select
                  id="preferred_audio_codec"
                  value={formData.preferred_audio_codec || 'aac'}
                  onChange={(e) => setFormData({ ...formData, preferred_audio_codec: e.target.value })}
                  className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm"
                  disabled={loading}
                >
                  <option value="copy">Copy</option>
                  <option value="aac">AAC</option>
                  <option value="opus">Opus</option>
                  <option value="mp3">MP3</option>
                </select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="preferred_container" className="text-xs text-muted-foreground">Container</Label>
                <select
                  id="preferred_container"
                  value={formData.preferred_container || 'mpegts'}
                  onChange={(e) => setFormData({ ...formData, preferred_container: e.target.value as ContainerFormat })}
                  className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm"
                  disabled={loading}
                >
                  <option value="mpegts">MPEG-TS</option>
                  <option value="fmp4">fMP4</option>
                </select>
              </div>
            </div>
          </div>

          {/* Advanced: Accepted Codecs */}
          <div className="border rounded-md">
            <button
              type="button"
              onClick={() => setShowAdvanced(!showAdvanced)}
              className="flex w-full items-center justify-between p-3 text-sm font-medium hover:bg-muted/50"
            >
              <span>Accepted Codecs (Passthrough)</span>
              {showAdvanced ? (
                <ChevronUp className="h-4 w-4" />
              ) : (
                <ChevronDown className="h-4 w-4" />
              )}
            </button>
            {showAdvanced && (
              <div className="p-4 pt-0 space-y-4 border-t">
                <p className="text-xs text-muted-foreground">
                  Codecs the client can accept without transcoding. If the source matches these, it will be copied directly.
                </p>

                <div className="space-y-2">
                  <Label className="text-xs text-muted-foreground">Accepted Video Codecs</Label>
                  <div className="flex flex-wrap gap-3">
                    {AVAILABLE_VIDEO_CODECS.map((codec) => (
                      <div key={codec.value} className="flex items-center space-x-2">
                        <Checkbox
                          id={`video-${codec.value}`}
                          checked={formData.accepted_video_codecs?.includes(codec.value) ?? false}
                          onCheckedChange={() =>
                            toggleCodec(formData.accepted_video_codecs, codec.value, 'video')
                          }
                          disabled={loading}
                        />
                        <label
                          htmlFor={`video-${codec.value}`}
                          className="text-sm cursor-pointer"
                        >
                          {codec.label}
                        </label>
                      </div>
                    ))}
                  </div>
                </div>

                <div className="space-y-2">
                  <Label className="text-xs text-muted-foreground">Accepted Audio Codecs</Label>
                  <div className="flex flex-wrap gap-3">
                    {AVAILABLE_AUDIO_CODECS.map((codec) => (
                      <div key={codec.value} className="flex items-center space-x-2">
                        <Checkbox
                          id={`audio-${codec.value}`}
                          checked={formData.accepted_audio_codecs?.includes(codec.value) ?? false}
                          onCheckedChange={() =>
                            toggleCodec(formData.accepted_audio_codecs, codec.value, 'audio')
                          }
                          disabled={loading}
                        />
                        <label
                          htmlFor={`audio-${codec.value}`}
                          className="text-sm cursor-pointer"
                        >
                          {codec.label}
                        </label>
                      </div>
                    ))}
                  </div>
                </div>

                <div className="space-y-2">
                  <Label className="text-xs text-muted-foreground">Accepted Containers</Label>
                  <div className="flex flex-wrap gap-3">
                    {AVAILABLE_CONTAINERS.map((container) => (
                      <div key={container.value} className="flex items-center space-x-2">
                        <Checkbox
                          id={`container-${container.value}`}
                          checked={formData.accepted_containers?.includes(container.value) ?? false}
                          onCheckedChange={() =>
                            toggleCodec(formData.accepted_containers, container.value, 'container')
                          }
                          disabled={loading}
                        />
                        <label
                          htmlFor={`container-${container.value}`}
                          className="text-sm cursor-pointer"
                        >
                          {container.label}
                        </label>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            )}
          </div>
        </form>

        <SheetFooter className="mt-4">
          <Button
            type="submit"
            form="create-mapping-form"
            disabled={loading}
            className="gap-2"
          >
            {loading && <Loader2 className="h-4 w-4 animate-spin" />}
            Create Mapping
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}

// Fallback rule card component
function FallbackRuleCard({
  fallbackRule,
  onUpdateFallback,
  loading,
}: {
  fallbackRule: RelayProfileMapping | null;
  onUpdateFallback: (updates: UpdateRelayProfileMappingRequest) => void;
  loading: boolean;
}) {
  if (!fallbackRule) return null;

  return (
    <Card className="border-dashed border-2">
      <CardHeader className="pb-3">
        <div className="flex items-center gap-2">
          <Shield className="h-5 w-5 text-muted-foreground" />
          <CardTitle className="text-base">Default Fallback Rule</CardTitle>
        </div>
        <CardDescription>
          This rule matches all unmatched clients and provides maximum compatibility settings.
          It always runs last (priority 999).
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="grid grid-cols-3 gap-4">
          <div className="space-y-2">
            <Label className="text-xs text-muted-foreground">Video Codec</Label>
            <select
              value={fallbackRule.preferred_video_codec || 'h264'}
              onChange={(e) => onUpdateFallback({ preferred_video_codec: e.target.value })}
              className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm"
              disabled={loading}
            >
              <option value="h264">H.264 (Most Compatible)</option>
              <option value="hevc">HEVC (H.265)</option>
            </select>
          </div>
          <div className="space-y-2">
            <Label className="text-xs text-muted-foreground">Audio Codec</Label>
            <select
              value={fallbackRule.preferred_audio_codec || 'aac'}
              onChange={(e) => onUpdateFallback({ preferred_audio_codec: e.target.value })}
              className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm"
              disabled={loading}
            >
              <option value="aac">AAC (Most Compatible)</option>
              <option value="mp3">MP3</option>
            </select>
          </div>
          <div className="space-y-2">
            <Label className="text-xs text-muted-foreground">Container</Label>
            <select
              value={fallbackRule.preferred_container || 'mpegts'}
              onChange={(e) => onUpdateFallback({ preferred_container: e.target.value as ContainerFormat })}
              className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-sm"
              disabled={loading}
            >
              <option value="mpegts">MPEG-TS (Most Compatible)</option>
              <option value="fmp4">fMP4</option>
            </select>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

export function RelayProfileMappings() {
  const [mappings, setMappings] = useState<RelayProfileMapping[]>([]);
  const [searchQuery, setSearchQuery] = useState('');
  const [loading, setLoading] = useState<LoadingState>({
    mappings: true,
    create: false,
    edit: false,
    delete: null,
    reorder: false,
  });
  const [error, setError] = useState<ErrorState>({
    mappings: null,
    create: null,
    edit: null,
    action: null,
  });

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    })
  );

  const fetchMappings = useCallback(async () => {
    setLoading((prev) => ({ ...prev, mappings: true }));
    setError((prev) => ({ ...prev, mappings: null }));

    try {
      const data = await apiClient.getRelayProfileMappings();
      setMappings(data);
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to fetch mappings';
      setError((prev) => ({ ...prev, mappings: message }));
    } finally {
      setLoading((prev) => ({ ...prev, mappings: false }));
    }
  }, []);

  useEffect(() => {
    fetchMappings();
  }, [fetchMappings]);

  const handleCreateMapping = async (data: CreateRelayProfileMappingRequest) => {
    setLoading((prev) => ({ ...prev, create: true }));
    setError((prev) => ({ ...prev, create: null }));

    try {
      await apiClient.createRelayProfileMapping(data);
      await fetchMappings();
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to create mapping';
      setError((prev) => ({ ...prev, create: message }));
      throw err;
    } finally {
      setLoading((prev) => ({ ...prev, create: false }));
    }
  };

  const handleDeleteMapping = async (id: string) => {
    if (!confirm('Are you sure you want to delete this mapping?')) return;

    setLoading((prev) => ({ ...prev, delete: id }));
    setError((prev) => ({ ...prev, action: null }));

    try {
      await apiClient.deleteRelayProfileMapping(id);
      await fetchMappings();
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to delete mapping';
      setError((prev) => ({ ...prev, action: message }));
    } finally {
      setLoading((prev) => ({ ...prev, delete: null }));
    }
  };

  const handleToggleEnabled = async (mapping: RelayProfileMapping) => {
    try {
      // Send all mapping fields to satisfy backend validation
      await apiClient.updateRelayProfileMapping(mapping.id, {
        name: mapping.name,
        description: mapping.description,
        expression: mapping.expression,
        priority: mapping.priority,
        is_enabled: !mapping.is_enabled,
        accepted_video_codecs: mapping.accepted_video_codecs,
        accepted_audio_codecs: mapping.accepted_audio_codecs,
        accepted_containers: mapping.accepted_containers,
        preferred_video_codec: mapping.preferred_video_codec,
        preferred_audio_codec: mapping.preferred_audio_codec,
        preferred_container: mapping.preferred_container,
      });
      await fetchMappings();
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to update mapping';
      setError((prev) => ({ ...prev, action: message }));
    }
  };

  const handleDragEnd = async (event: DragEndEvent) => {
    const { active, over } = event;

    if (!over || active.id === over.id) return;

    setLoading((prev) => ({ ...prev, reorder: true }));

    // Optimistically update UI
    const oldIndex = regularMappings.findIndex((m) => m.id === active.id);
    const newIndex = regularMappings.findIndex((m) => m.id === over.id);

    const newOrder = arrayMove(regularMappings, oldIndex, newIndex);

    // Calculate new priorities
    const reorderRequests = newOrder.map((mapping, index) => ({
      id: mapping.id,
      priority: index + 1,
    }));

    try {
      await apiClient.reorderRelayProfileMappings(reorderRequests);
      await fetchMappings();
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to reorder';
      setError((prev) => ({ ...prev, action: message }));
    } finally {
      setLoading((prev) => ({ ...prev, reorder: false }));
    }
  };

  const handleUpdateFallback = async (updates: UpdateRelayProfileMappingRequest) => {
    if (!fallbackRule) return;

    try {
      // Send all mapping fields with the specific update applied
      await apiClient.updateRelayProfileMapping(fallbackRule.id, {
        name: fallbackRule.name,
        description: fallbackRule.description,
        expression: fallbackRule.expression,
        priority: fallbackRule.priority,
        is_enabled: fallbackRule.is_enabled,
        accepted_video_codecs: fallbackRule.accepted_video_codecs,
        accepted_audio_codecs: fallbackRule.accepted_audio_codecs,
        accepted_containers: fallbackRule.accepted_containers,
        preferred_video_codec: fallbackRule.preferred_video_codec,
        preferred_audio_codec: fallbackRule.preferred_audio_codec,
        preferred_container: fallbackRule.preferred_container,
        ...updates, // Override with the specific update
      });
      await fetchMappings();
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed to update fallback rule';
      setError((prev) => ({ ...prev, action: message }));
    }
  };

  // Separate fallback rule (priority 999) from regular mappings
  const fallbackRule = mappings.find((m) => m.priority === 999);
  const regularMappings = mappings.filter((m) => m.priority !== 999);

  const filteredMappings = regularMappings.filter((m) =>
    m.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
    m.expression.toLowerCase().includes(searchQuery.toLowerCase())
  );

  return (
    <div className="container mx-auto p-4 space-y-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Client Detection</h1>
          <p className="text-muted-foreground">
            Configure automatic codec selection based on client properties
          </p>
        </div>
        <CreateMappingSheet
          onCreateMapping={handleCreateMapping}
          loading={loading.create}
          error={error.create}
        />
      </div>

      {error.action && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertTitle>Error</AlertTitle>
          <AlertDescription>{error.action}</AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Users className="h-5 w-5" />
            Detection Rules
          </CardTitle>
          <CardDescription>
            Drag rules to reorder. Rules are evaluated in priority order - first match wins.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="mb-4">
            <div className="relative">
              <Search className="absolute left-2 top-2.5 h-4 w-4 text-muted-foreground" />
              <Input
                placeholder="Search mappings..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="pl-8"
              />
            </div>
          </div>

          {loading.mappings ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin" />
              <span className="ml-2">Loading mappings...</span>
            </div>
          ) : error.mappings ? (
            <Alert variant="destructive">
              <AlertCircle className="h-4 w-4" />
              <AlertTitle>Error</AlertTitle>
              <AlertDescription>{error.mappings}</AlertDescription>
            </Alert>
          ) : filteredMappings.length === 0 ? (
            <div className="text-center py-8 text-muted-foreground">
              No client detection mappings found.
            </div>
          ) : (
            <DndContext
              sensors={sensors}
              collisionDetection={closestCenter}
              onDragEnd={handleDragEnd}
            >
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-20">Priority</TableHead>
                    <TableHead>Name</TableHead>
                    <TableHead>Expression</TableHead>
                    <TableHead>Preferred Codecs</TableHead>
                    <TableHead className="w-24">Status</TableHead>
                    <TableHead className="w-32">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <SortableContext
                    items={filteredMappings.map((m) => m.id)}
                    strategy={verticalListSortingStrategy}
                  >
                    {filteredMappings.map((mapping) => (
                      <SortableRow
                        key={mapping.id}
                        mapping={mapping}
                        onToggleEnabled={handleToggleEnabled}
                        onDelete={handleDeleteMapping}
                        loading={loading}
                      />
                    ))}
                  </SortableContext>
                </TableBody>
              </Table>
            </DndContext>
          )}
        </CardContent>
      </Card>

      {/* Fallback Rule - Always at bottom */}
      <FallbackRuleCard
        fallbackRule={fallbackRule || null}
        onUpdateFallback={handleUpdateFallback}
        loading={loading.reorder}
      />

      <Card>
        <CardHeader>
          <CardTitle>How It Works</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">
            Client detection rules match incoming stream requests based on HTTP headers and
            other request properties. When a client connects:
          </p>
          <ul className="list-disc list-inside space-y-1 text-sm text-muted-foreground ml-2">
            <li>Rules are evaluated in priority order (lowest number first)</li>
            <li>The first matching rule determines which codecs the client accepts</li>
            <li>If the source stream matches accepted codecs, it is passed through (copy mode)</li>
            <li>Otherwise, the stream is transcoded to the preferred codec</li>
            <li>If no rule matches, the fallback rule provides maximum compatibility settings</li>
          </ul>

          <div className="pt-2">
            <p className="text-sm font-medium mb-2">Available Expression Fields:</p>
            <div className="flex flex-wrap gap-2">
              {[
                { name: 'user_agent', desc: 'HTTP User-Agent header' },
                { name: 'client_ip', desc: 'Client IP address' },
                { name: 'request_path', desc: 'Request URL path' },
                { name: 'request_url', desc: 'Full request URL' },
                { name: 'query_params', desc: 'Raw query string' },
                { name: 'x_forwarded_for', desc: 'X-Forwarded-For header' },
                { name: 'x_real_ip', desc: 'X-Real-IP header' },
                { name: 'accept', desc: 'Accept header' },
                { name: 'accept_language', desc: 'Accept-Language header' },
                { name: 'host', desc: 'Host header' },
                { name: 'referer', desc: 'Referer header' },
              ].map((field) => (
                <code
                  key={field.name}
                  className="text-xs bg-muted px-2 py-1 rounded"
                  title={field.desc}
                >
                  {field.name}
                </code>
              ))}
            </div>
          </div>

          <div className="pt-2">
            <p className="text-sm font-medium mb-2">Expression Operators:</p>
            <div className="flex flex-wrap gap-2 text-xs">
              <code className="bg-muted px-2 py-1 rounded">equals</code>
              <code className="bg-muted px-2 py-1 rounded">not_equals</code>
              <code className="bg-muted px-2 py-1 rounded">contains</code>
              <code className="bg-muted px-2 py-1 rounded">not_contains</code>
              <code className="bg-muted px-2 py-1 rounded">starts_with</code>
              <code className="bg-muted px-2 py-1 rounded">ends_with</code>
              <code className="bg-muted px-2 py-1 rounded">matches</code>
              <code className="bg-muted px-2 py-1 rounded">AND</code>
              <code className="bg-muted px-2 py-1 rounded">OR</code>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
