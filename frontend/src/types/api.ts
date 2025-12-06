// API Types generated from OpenAPI specification

// Core API Response Types
export interface ApiResponse<T> {
  success: boolean;
  timestamp: string;
  data?: T;
  error?: string;
  details?: Record<string, string>;
}

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
  has_next: boolean;
  has_previous: boolean;
}

// Stream Source Types
// Added 'manual' to align with backend enum (m3u | xtream | manual)
export type StreamSourceType = 'm3u' | 'xtream' | 'manual';

// Manual channel definition (only used when source_type === 'manual')
export interface ManualChannelInput {
  channel_number?: number;
  name: string;
  stream_url: string;
  group_title?: string;
  tvg_id?: string;
  tvg_name?: string;
  tvg_logo?: string; // @logo:token or URL
  epg_id?: string;
  // All rows are treated as active in current UX, but keep flag for possible future use
  is_active?: boolean;
}

export interface StreamSource {
  id: string;
  name: string;
  source_type: StreamSourceType;
  // For manual sources the backend may omit URL; make optional
  url?: string;
  max_concurrent_streams: number;
  update_cron: string;
  ignore_channel_numbers: boolean;
  created_at: string;
  updated_at: string;
  is_active: boolean;
  field_map?: string;
  last_ingested_at?: string;
  username?: string;
  password?: string;
  // (Optional future extension) manual_channels?: ManualChannelInput[];  // not included in current backend list response
}

export interface StreamSourceResponse extends StreamSource {
  source_kind?: 'stream'; // Added for unified /sources endpoint
  channel_count: number;
  next_scheduled_update?: string;
}

// EPG Source Types
export type EpgSourceType = 'xmltv' | 'xtream';
export type XtreamApiMethod = 'stream_id' | 'bulk_xmltv';

export interface EpgSource {
  id: string;
  name: string;
  source_type: EpgSourceType;
  url: string;
  update_cron: string;
  time_offset: string;
  created_at: string;
  updated_at: string;
  is_active: boolean;
  last_ingested_at?: string;
  original_timezone?: string;
  username?: string;
  password?: string;
  api_method?: XtreamApiMethod; // Only for Xtream sources
}

export interface EpgSourceResponse extends EpgSource {
  source_kind?: 'epg'; // Added for unified /sources endpoint
  channel_count: number;
  program_count: number;
  next_scheduled_update?: string;
}

// Proxy Types
export interface StreamProxy {
  id: string;
  name: string;
  proxy_mode: string;
  starting_channel_number: number;
  is_active: boolean;
  auto_regenerate: boolean;
  description?: string;
  max_concurrent_streams?: number;
  upstream_timeout?: number;
  cache_channel_logos: boolean;
  cache_program_logos: boolean;
  relay_profile_id?: string;
  m3u8_url?: string;
  xmltv_url?: string;
  created_at: string;
  updated_at: string;
  last_generated_at?: string;
}

export interface ProxySourceRequest {
  source_id: string;
  priority_order: number;
}

export interface ProxyEpgSourceRequest {
  epg_source_id: string;
  priority_order: number;
}

export interface ProxyFilterRequest {
  filter_id: string;
  priority_order: number;
  is_active: boolean;
}

// Filter Types
export type FilterSourceType = 'stream' | 'epg';
export type FilterAction = 'include' | 'exclude';

export interface Filter {
  id: string;
  name: string;
  description?: string;
  source_type: FilterSourceType;
  action: FilterAction;
  expression: string;
  priority: number;
  is_enabled: boolean;
  is_system?: boolean;  // Optional - set by backend, not API
  source_id?: string;
  created_at: string;
  updated_at: string;
}

// Response wrapper for filter list endpoint
export interface FilterListResponse {
  filters: Filter[];
  count: number;
}

export interface FilterExpressionTree {
  case_sensitive?: boolean;
  field?: string;
  negate?: boolean;
  operator?: string;
  type: 'condition' | 'group';
  value?: string;
  children?: FilterExpressionTree[];
}

// Deprecated - keeping for backward compatibility during migration
export interface FilterWithMeta {
  filter: Filter;
}

// Relay Types
// Backend uses FFmpeg codec names like 'libx264', 'libx265', 'copy', 'aac', etc.
export type VideoCodec = string;
export type AudioCodec = string;
export type RelayOutputFormat = string;
export type HWAccelType = string;

export interface RelayProfile {
  id: string;
  name: string;
  description?: string;
  video_codec: VideoCodec;
  audio_codec: AudioCodec;
  video_bitrate?: number;
  audio_bitrate?: number;
  video_maxrate?: number;
  video_preset?: string;
  video_width?: number;
  video_height?: number;
  audio_sample_rate?: number;
  audio_channels?: number;
  hw_accel: HWAccelType;
  output_format: RelayOutputFormat;
  is_default: boolean;
  is_system?: boolean;  // Optional - set by backend, not API
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateRelayProfileRequest {
  name: string;
  description?: string;
  video_codec?: VideoCodec;
  audio_codec?: AudioCodec;
  video_preset?: string;
  video_bitrate?: number;
  audio_bitrate?: number;
  video_maxrate?: number;
  video_width?: number;
  video_height?: number;
  audio_sample_rate?: number;
  audio_channels?: number;
  hw_accel?: HWAccelType;
  output_format?: RelayOutputFormat;
  is_default?: boolean;
}

export interface UpdateRelayProfileRequest {
  name?: string;
  description?: string;
  video_codec?: VideoCodec;
  audio_codec?: AudioCodec;
  video_preset?: string;
  video_bitrate?: number;
  audio_bitrate?: number;
  video_maxrate?: number;
  video_width?: number;
  video_height?: number;
  audio_sample_rate?: number;
  audio_channels?: number;
  hw_accel?: HWAccelType;
  output_format?: RelayOutputFormat;
}

export interface ConnectedClient {
  id: string;
  ip: string;
  user_agent?: string;
  connected_at: string;
  bytes_served: number;
  last_activity: string;
}

export interface RelayProcessHealth {
  config_id: string;
  profile_id: string;
  profile_name: string;
  proxy_id?: string;
  source_url: string;
  status: 'healthy' | 'unhealthy' | 'starting' | 'stopping' | 'failed';
  pid?: number;
  uptime_seconds: number;
  memory_usage_mb: number;
  cpu_usage_percent: number;
  bytes_received_upstream: number;
  bytes_delivered_downstream: number;
  connected_clients: ConnectedClient[];
  last_heartbeat: string;
}

export interface RelayHealthResponse {
  total_processes: number;
  healthy_processes: number;
  unhealthy_processes: number;
  processes: RelayProcessHealth[];
  last_check: string;
}

// Real relay health response structure from API
export interface RelayHealthApiResponse {
  healthy_processes: string;
  unhealthy_processes: string;
  total_processes: string;
  last_check: string;
  processes: RelayProcess[];
}

export interface RelayProcess {
  config_id: string;
  profile_id: string;
  profile_name: string;
  proxy_id?: string;
  channel_name?: string;
  source_url: string;
  status: 'healthy' | 'unhealthy' | 'starting' | 'stopping' | 'failed';
  pid?: string;
  uptime_seconds: string;
  memory_usage_mb: string;
  cpu_usage_percent: string;
  bytes_received_upstream: string;
  bytes_delivered_downstream: string;
  connected_clients: RelayConnectedClient[];
  last_heartbeat: string;
}

export interface RelayConnectedClient {
  id: string;
  ip: string;
  user_agent?: string;
  connected_at: string;
  bytes_served: string;
  last_activity: string;
}

// Logo Types
export interface LinkedAsset {
  type: string;
  path: string;
  content_type: string;
  size: number;
  url: string;
}

export interface LogoAsset {
  id: string;
  name: string;
  description?: string;
  file_name: string;
  file_path: string;
  file_size: number;
  mime_type: string;
  original_mime_type?: string;
  original_file_size?: number;
  asset_type: 'uploaded' | 'cached';
  source_url?: string;
  width: number | null;
  height: number | null;
  parent_asset_id?: string | null;
  format_type: string;
  created_at: string;
  updated_at: string;
  url: string;
  linked_assets?: LinkedAsset[];
  linked_assets_count: number;
  total_linked_size: number;
}

export interface LogoAssetsResponse {
  assets: LogoAsset[];
  total_count: number;
  page: number;
  limit: number;
  total_pages: number;
}

export interface LogoStats {
  total_cached_logos: number;
  total_uploaded_logos: number;
  total_storage_used: number;
  total_linked_assets: number;
  cache_hit_rate: number | null;
  filesystem_cached_logos: number;
  filesystem_cached_storage: number;
}

export interface LogoAssetUpdateRequest {
  name?: string;
  description?: string;
}

export interface LogoUploadRequest {
  name: string;
  description?: string;
  file: File;
}

// Data Mapping Types
export type DataMappingSourceType = 'stream' | 'epg';

export interface DataMappingRule {
  id: string;
  name: string;
  description?: string;
  source_type: DataMappingSourceType;
  expression: string;
  priority: number;
  stop_on_match: boolean;
  is_enabled: boolean;
  is_system?: boolean;  // Optional - set by backend, not API
  source_id?: string;
  created_at: string;
  updated_at: string;
}

// Response wrapper for data mapping rule list endpoint
export interface DataMappingRuleListResponse {
  rules: DataMappingRule[];
  count: number;
}

// Dashboard Metrics Types
export interface DashboardMetrics {
  active_clients: number;
  active_relays: number;
  total_channels: number;
  total_bandwidth: number;
  system_health: 'healthy' | 'warning' | 'critical';
  uptime: string;
}

export interface ClientMetrics {
  id: string;
  ip_address: string;
  user_agent: string;
  channel_name: string;
  channel_id: string;
  proxy_name: string;
  connected_at: string;
  data_transferred: number;
  current_bitrate: number;
  status: 'connected' | 'buffering' | 'disconnected';
}

export interface RelayMetrics {
  config_id: string;
  channel_name: string;
  connected_clients: number;
  upstream_bitrate: number;
  downstream_bitrate: number;
  cpu_usage: number;
  memory_usage: number;
  status: 'running' | 'starting' | 'stopping' | 'error';
  uptime: string;
}

// Health Types
export interface HealthData {
  status: string;
  timestamp: string;
  version: string;
  uptime_seconds: number;
  system_load: number;
  cpu_info: {
    cores: number;
    load_1min: number;
    load_5min: number;
    load_15min: number;
    load_percentage_1min: number;
  };
  memory: {
    total_memory_mb: number;
    used_memory_mb: number;
    free_memory_mb: number;
    available_memory_mb: number;
    swap_used_mb: number;
    swap_total_mb: number;
    process_memory: {
      main_process_mb: number;
      child_processes_mb: number;
      total_process_tree_mb: number;
      percentage_of_system: number;
      child_process_count: number;
    };
  };
  components: {
    database: {
      status: string;
      connection_pool_size: number;
      active_connections: number;
      idle_connections: number;
      pool_utilization_percent: number;
      response_time_ms: number;
      response_time_status: string;
      tables_accessible: boolean;
      write_capability: boolean;
      no_blocking_locks: boolean;
    };
    scheduler: {
      status: string;
      sources_scheduled: {
        stream_sources: number;
        epg_sources: number;
      };
      next_scheduled_times: Array<{
        source_id: string;
        source_name: string;
        source_type: string;
        next_run: string;
        cron_expression: string;
      }>;
      last_cache_refresh: string;
      active_ingestions: number;
    };
    sandbox_manager: {
      status: string;
      last_cleanup_run: string;
      cleanup_status: string;
      temp_files_cleaned: number;
      disk_space_freed_mb: number;
      managed_directories: Array<{
        name: string;
        retention_duration: string;
        cleanup_interval: string;
      }>;
    };
    relay_system: {
      status: string;
      total_processes: number;
      healthy_processes: number;
      unhealthy_processes: number;
      ffmpeg_available: boolean;
      ffmpeg_version: string | null;
      ffprobe_available: boolean;
      ffprobe_version: string | null;
      hwaccel_available: boolean;
      hwaccel_capabilities: {
        accelerators: string[];
        codecs: string[];
        support_matrix: {
          [accelerator: string]: {
            h264: boolean;
            hevc: boolean;
            av1: boolean;
          };
        };
      };
    };
    circuit_breakers?: {
      [serviceName: string]: {
        total_calls: number;
        successful_calls: number;
        failed_calls: number;
        state: 'Closed' | 'Open' | 'HalfOpen';
        failure_rate: number;
      };
    };
  };
}

// K8s-aligned probe responses matching backend LivezResponse and ReadyzResponse
export interface LivezProbeResponse {
  status: string; // "ok" when healthy
}

export interface ReadyzProbeResponse {
  status: string; // "ok" when ready, "not_ready" otherwise
  components?: Record<string, string>; // e.g., { database: "ok", scheduler: "ok" }
}

// Legacy alias for backward compatibility
export interface KubernetesProbeResponse {
  success: boolean;
  timestamp: string;
}

// Request Types
export interface CreateStreamSourceRequest {
  name: string;
  source_type: StreamSourceType;
  // Optional because manual sources do not require a URL
  url?: string;
  max_concurrent_streams: number;
  update_cron: string;
  field_map?: string;
  ignore_channel_numbers?: boolean;
  username?: string;
  password?: string;
  // Only include when source_type === 'manual' and length >= 1
  manual_channels?: ManualChannelInput[];
}

export interface UpdateStreamSourceRequest {
  name: string;
  source_type: StreamSourceType;
  // Optional for manual sources
  url?: string;
  max_concurrent_streams: number;
  update_cron: string;
  field_map?: string;
  ignore_channel_numbers?: boolean;
  username?: string;
  password?: string;
  // When provided for a manual source triggers replace+materialize
  manual_channels?: ManualChannelInput[];
}

export interface CreateEpgSourceRequest {
  name: string;
  source_type: EpgSourceType;
  url: string;
  update_cron: string;
  original_timezone?: string;
  time_offset?: string;
  username?: string;
  password?: string;
  api_method?: XtreamApiMethod; // Only for Xtream sources
}

export interface CreateStreamProxyRequest {
  name: string;
  proxy_mode: string;
  starting_channel_number: number;
  stream_sources: ProxySourceRequest[];
  epg_sources: ProxyEpgSourceRequest[];
  filters: ProxyFilterRequest[];
  is_active: boolean;
  auto_regenerate: boolean;
  description?: string;
  max_concurrent_streams?: number;
  upstream_timeout?: number;
  cache_channel_logos: boolean;
  cache_program_logos: boolean;
  relay_profile_id?: string;
}

export interface UpdateStreamProxyRequest {
  name: string;
  proxy_mode: string;
  starting_channel_number: number;
  stream_sources: ProxySourceRequest[];
  epg_sources: ProxyEpgSourceRequest[];
  filters: ProxyFilterRequest[];
  is_active: boolean;
  auto_regenerate?: boolean;
  description?: string;
  max_concurrent_streams?: number;
  upstream_timeout?: number;
  cache_channel_logos?: boolean;
  cache_program_logos?: boolean;
  relay_profile_id?: string;
}

export interface FilterTestRequest {
  source_id: string;
  source_type: FilterSourceType;
  filter_expression: string;
  is_inverse: boolean;
}

// Event Types for SSE
export interface ServiceEvent {
  id: string;
  timestamp: string;
  level: 'debug' | 'info' | 'warn' | 'error';
  message: string;
  source: string;
  context?: Record<string, any>;
}

// Progress Stage Types
export interface ProgressStage {
  id: string;
  name: string;
  percentage: number;
  state:
    | 'idle'
    | 'preparing'
    | 'connecting'
    | 'downloading'
    | 'processing'
    | 'saving'
    | 'cleanup'
    | 'completed'
    | 'error'
    | 'cancelled';
  stage_step: string | null;
}

// T043: ErrorDetail interface for structured error information
export interface ErrorDetail {
  stage: string;
  message: string;
  technical?: string;
  suggestion?: string;
}

// Updated Progress Event Types for SSE
export interface ProgressEvent {
  id: string;
  operation_name: string;
  operation_type: 'epg_ingestion' | 'stream_ingestion' | 'proxy_regeneration';
  owner_type: 'epg_source' | 'stream_source' | 'proxy';
  owner_id: string;
  state:
    | 'idle'
    | 'preparing'
    | 'connecting'
    | 'downloading'
    | 'processing'
    | 'saving'
    | 'cleanup'
    | 'completed'
    | 'error'
    | 'cancelled';
  current_stage: string;
  overall_percentage: number;
  stages: ProgressStage[];
  started_at: string;
  last_update: string;
  completed_at: string | null;
  error: string | null;
  // T044: Structured error and warning fields for UI feedback
  error_detail?: ErrorDetail;
  warning_count?: number;
  warnings?: string[];
}

export type EventHandler = (event: ServiceEvent | ProgressEvent) => void;

// Log Types for SSE
export interface LogEntry {
  id: string;
  timestamp: string;
  level: 'trace' | 'debug' | 'info' | 'warn' | 'error';
  target: string;
  message: string;
  fields: Record<string, any>;
  span?: any;
  module?: string;
  file?: string;
  line?: number;
  context?: Record<string, any>;
}

export interface LogStats {
  total_logs: number;
  logs_by_level: {
    trace: number;
    debug: number;
    info: number;
    warn: number;
    error: number;
  };
  logs_by_module: Record<string, number>;
  recent_errors: LogEntry[];
  log_rate_per_minute: number;
  oldest_log_timestamp?: string;
  newest_log_timestamp?: string;
}

export type LogHandler = (log: LogEntry) => void;

// Settings Types
export interface RuntimeSettings {
  log_level: string;
  enable_request_logging: boolean;
}

export interface UpdateSettingsRequest {
  log_level?: string;
  enable_request_logging?: boolean;
}

export interface SettingsResponse {
  success: boolean;
  message: string;
  settings: RuntimeSettings;
  applied_changes: string[];
}

// Startup Config Types (read-only)
export interface StartupConfigSetting {
  key: string;
  value: string | number | boolean;
  type: string;
  description: string;
}

export interface StartupConfigSection {
  name: string;
  description: string;
  settings: StartupConfigSetting[];
}

export interface StartupConfigResponse {
  success: boolean;
  message: string;
  sections: StartupConfigSection[];
  timestamp: string;
}

// Config Meta and Persist Types
export interface ConfigMeta {
  config_path?: string;
  can_persist: boolean;
  last_modified?: string;
  source: 'file' | 'env' | 'defaults';
}

export interface ConfigPersistResponse {
  success: boolean;
  message: string;
  path?: string;
  sections?: string[];
}

// Expression Editor Types
export interface ExpressionField {
  name: string;
  display_name: string;
  field_type: 'string' | 'number' | 'boolean' | 'datetime';
  nullable: boolean;
  source_type: 'stream' | 'epg';
}

export interface ExpressionValidationError {
  category: 'field' | 'syntax' | 'operator' | 'value';
  error_type: string;
  message: string;
  details?: string;
  position: number;
  context?: string;
  suggestion?: string;
}

export interface ExpressionValidationResponse {
  is_valid: boolean;
  error: string | null;
  errors?: ExpressionValidationError[];
  condition_tree: string | null;
}

export interface ExpressionTestResult {
  source_id: string;
  source_name: string;
  matched_count: number;
  total_count: number;
  error?: string;
}

export interface ExpressionTestRequest {
  source_id: string;
  source_type: FilterSourceType;
  filter_expression: string;
  is_inverse: boolean;
}

export interface ExpressionEditorConfig {
  validationEndpoint: string;
  fieldsEndpoint: string;
  testEndpoint?: string;
  sourcesEndpoint?: string;
  sourceType: 'stream' | 'epg';
  debounceMs?: number;
  showTestResults?: boolean;
}
