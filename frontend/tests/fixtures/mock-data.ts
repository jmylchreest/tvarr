/**
 * Realistic mock data for Playwright API mocking.
 *
 * These match the actual API response shapes from the tvarr backend.
 * Used by the mock-api fixture to intercept fetch requests.
 */

// ─── Stream Proxies ────────────────────────────────────

export const mockProxies = [
  {
    id: 'proxy-1',
    name: 'UK Freeview',
    proxy_mode: 'direct' as const,
    starting_channel_number: 1,
    numbering_mode: 'sequential' as const,
    group_numbering_size: 100,
    is_active: true,
    auto_regenerate: true,
    description: 'UK terrestrial channels',
    max_concurrent_streams: 5,
    upstream_timeout: 30,
    cache_channel_logos: true,
    cache_program_logos: false,
    encoding_profile_id: undefined,
    m3u8_url: '/api/v1/proxies/proxy-1/playlist.m3u8',
    xmltv_url: '/api/v1/proxies/proxy-1/epg.xml',
    status: 'ready' as const,
    channel_count: 142,
    program_count: 5280,
    last_error: undefined,
    created_at: '2025-01-15T10:00:00Z',
    updated_at: '2025-02-10T14:30:00Z',
    last_generated_at: '2025-02-10T14:30:00Z',
  },
  {
    id: 'proxy-2',
    name: 'Sports Package',
    proxy_mode: 'smart' as const,
    starting_channel_number: 500,
    numbering_mode: 'group' as const,
    group_numbering_size: 50,
    is_active: true,
    auto_regenerate: false,
    description: 'Premium sports channels with transcoding',
    max_concurrent_streams: 3,
    upstream_timeout: 60,
    cache_channel_logos: true,
    cache_program_logos: true,
    encoding_profile_id: 'profile-1',
    m3u8_url: '/api/v1/proxies/proxy-2/playlist.m3u8',
    xmltv_url: '/api/v1/proxies/proxy-2/epg.xml',
    status: 'ready' as const,
    channel_count: 48,
    program_count: 1200,
    last_error: undefined,
    created_at: '2025-01-20T12:00:00Z',
    updated_at: '2025-02-08T09:15:00Z',
    last_generated_at: '2025-02-08T09:15:00Z',
  },
  {
    id: 'proxy-3',
    name: 'Movies',
    proxy_mode: 'direct' as const,
    starting_channel_number: 1000,
    numbering_mode: 'preserve' as const,
    group_numbering_size: 100,
    is_active: false,
    auto_regenerate: true,
    description: 'Movie channels - currently disabled',
    max_concurrent_streams: 2,
    upstream_timeout: 30,
    cache_channel_logos: false,
    cache_program_logos: false,
    encoding_profile_id: undefined,
    status: 'failed' as const,
    channel_count: 0,
    program_count: 0,
    last_error: 'No matching channels found after filtering',
    created_at: '2025-02-01T08:00:00Z',
    updated_at: '2025-02-05T16:45:00Z',
    last_generated_at: undefined,
  },
];

// ─── Stream Sources ────────────────────────────────────

export const mockStreamSources = [
  {
    id: 'source-1',
    name: 'Primary IPTV',
    source_type: 'm3u' as const,
    url: 'http://provider.example.com/playlist.m3u',
    max_concurrent_streams: 10,
    update_cron: '0 */6 * * *',
    ignore_channel_numbers: false,
    created_at: '2025-01-10T08:00:00Z',
    updated_at: '2025-02-10T12:00:00Z',
    enabled: true,
    status: 'success' as const,
    channel_count: 3500,
    last_ingestion_at: '2025-02-10T12:00:00Z',
    next_scheduled_update: '2025-02-10T18:00:00Z',
  },
  {
    id: 'source-2',
    name: 'Backup Provider',
    source_type: 'xtream' as const,
    url: 'http://backup.example.com',
    max_concurrent_streams: 5,
    update_cron: '0 0 * * *',
    ignore_channel_numbers: true,
    created_at: '2025-01-12T10:00:00Z',
    updated_at: '2025-02-09T00:00:00Z',
    enabled: true,
    status: 'success' as const,
    channel_count: 2100,
    username: 'user123',
    last_ingestion_at: '2025-02-09T00:00:00Z',
    next_scheduled_update: '2025-02-10T00:00:00Z',
  },
];

// ─── EPG Sources ───────────────────────────────────────

export const mockEpgSources = [
  {
    id: 'epg-1',
    name: 'Main EPG',
    source_type: 'xmltv' as const,
    url: 'http://epg.example.com/guide.xml',
    update_cron: '0 */4 * * *',
    epg_shift: 0,
    created_at: '2025-01-10T08:00:00Z',
    updated_at: '2025-02-10T08:00:00Z',
    enabled: true,
    status: 'success' as const,
    channel_count: 1200,
    program_count: 48000,
    last_ingestion_at: '2025-02-10T08:00:00Z',
    next_scheduled_update: '2025-02-10T12:00:00Z',
  },
];

// ─── Filters ───────────────────────────────────────────

export const mockFilters = [
  {
    id: 'filter-1',
    name: 'UK Only',
    description: 'Include only UK channels',
    source_type: 'stream' as const,
    action: 'include' as const,
    expression: 'group_title matches "UK|United Kingdom"',
    is_system: false,
    created_at: '2025-01-15T10:00:00Z',
    updated_at: '2025-01-15T10:00:00Z',
  },
  {
    id: 'filter-2',
    name: 'No Adult',
    description: 'Exclude adult content',
    source_type: 'stream' as const,
    action: 'exclude' as const,
    expression: 'is_adult == true',
    is_system: false,
    created_at: '2025-01-15T10:30:00Z',
    updated_at: '2025-01-15T10:30:00Z',
  },
];

// ─── Encoding Profiles ─────────────────────────────────

export const mockEncodingProfiles = [
  {
    id: 'profile-1',
    name: 'H.264/AAC (Universal)',
    description: 'Maximum device compatibility - works with all players and browsers',
    target_video_codec: 'h264',
    target_audio_codec: 'aac',
    quality_preset: 'medium' as const,
    hw_accel: 'auto',
    default_flags: {
      global_flags: '-hide_banner -stats',
      input_flags: '-f mpegts -analyzeduration 500000 -probesize 500000',
      output_flags: '-map 0:v:0 -map 0:a:0? -c:v libx264 -preset medium -maxrate 5M -bufsize 10M -c:a aac -b:a 192k -f mpegts',
    },
    is_default: true,
    is_system: true,
    enabled: true,
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
  },
  {
    id: 'profile-2',
    name: 'H.265 High Quality',
    description: 'High quality H.265 encoding for home theater setups',
    target_video_codec: 'h265',
    target_audio_codec: 'aac',
    quality_preset: 'high' as const,
    hw_accel: 'auto',
    default_flags: {
      global_flags: '-hide_banner -stats',
      input_flags: '-f mpegts -analyzeduration 500000 -probesize 500000',
      output_flags: '-map 0:v:0 -map 0:a:0? -c:v libx265 -preset medium -maxrate 10M -bufsize 20M -c:a aac -b:a 256k -f mpegts',
    },
    is_default: false,
    is_system: true,
    enabled: true,
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
  },
];

// ─── Data Mapping Rules ────────────────────────────────

export const mockDataMappingRules = [
  {
    id: 'dmr-1',
    name: 'Prefix HD channels',
    description: 'Adds HD prefix to channel names',
    source_type: 'stream' as const,
    expression: 'channel_name = "HD " + channel_name',
    priority: 1,
    stop_on_match: false,
    is_enabled: true,
    is_system: false,
    source_id: null,
    created_at: '2025-01-15T10:30:00Z',
    updated_at: '2025-01-15T10:30:00Z',
  },
  {
    id: 'dmr-2',
    name: 'Normalize group names',
    description: 'Standardize group_title casing',
    source_type: 'stream' as const,
    expression: 'group_title = upper(group_title)',
    priority: 2,
    stop_on_match: false,
    is_enabled: true,
    is_system: false,
    source_id: null,
    created_at: '2025-01-16T09:00:00Z',
    updated_at: '2025-01-16T09:00:00Z',
  },
];

// ─── Client Detection Rules ────────────────────────────

export const mockClientDetectionRules = [
  {
    id: 'cdr-1',
    name: 'Chrome Browser',
    description: 'Matches Chrome User-Agent strings',
    expression: 'user_agent contains "Chrome" AND NOT user_agent contains "Edge"',
    priority: 10,
    is_enabled: true,
    is_system: false,
    accepted_video_codecs: ['h264', 'h265', 'vp9', 'av1'],
    accepted_audio_codecs: ['aac', 'opus'],
    preferred_video_codec: 'h264',
    preferred_audio_codec: 'aac',
    supports_fmp4: true,
    supports_mpegts: true,
    preferred_format: 'auto',
    encoding_profile_id: null,
    created_at: '2025-01-15T10:30:00Z',
    updated_at: '2025-01-15T10:30:00Z',
  },
];

// ─── Encoder Overrides ─────────────────────────────────

export const mockEncoderOverrides = [
  {
    id: 'eo-1',
    name: 'Force software H.265 encoder',
    description: 'Use libx265 when VAAPI h265 is broken',
    codec_type: 'video',
    source_codec: 'h265',
    target_encoder: 'libx265',
    hw_accel_match: 'vaapi',
    cpu_match: '',
    priority: 100,
    is_enabled: true,
    is_system: false,
    created_at: '2025-01-15T10:30:00Z',
    updated_at: '2025-01-15T10:30:00Z',
  },
];

// ─── Transcoders / Daemons ─────────────────────────────

export const mockDaemons = [
  {
    id: 'daemon-01',
    name: 'transcoder-1',
    version: '0.5.0',
    address: '10.0.0.5:9090',
    state: 'connected',
    connected_at: '2025-01-15T10:30:00Z',
    last_heartbeat: '2025-02-17T12:00:00Z',
    heartbeats_missed: 0,
    active_jobs: 1,
    active_cpu_jobs: 0,
    active_gpu_jobs: 1,
    active_job_details: [],
    total_jobs_completed: 150,
    total_jobs_failed: 3,
    capabilities: {
      video_encoders: ['libx264', 'h264_vaapi'],
      video_decoders: ['h264', 'hevc'],
      audio_encoders: ['aac'],
      audio_decoders: ['aac', 'ac3'],
      max_concurrent_jobs: 8,
      max_cpu_jobs: 4,
      max_gpu_jobs: 4,
      max_probe_jobs: 4,
      hw_accels: [],
      gpus: [],
    },
    system_stats: {
      hostname: 'transcoder-host-1',
      os: 'linux',
      arch: 'amd64',
      cpu_cores: 8,
      cpu_percent: 35.5,
      memory_total: 16384,
      memory_used: 8192,
      memory_available: 8192,
      memory_percent: 50.0,
      gpus: [],
    },
  },
];

export const mockClusterStats = {
  total_daemons: 1,
  active_daemons: 1,
  unhealthy_daemons: 0,
  draining_daemons: 0,
  disconnected_daemons: 0,
  total_active_jobs: 1,
  total_cpu_jobs: 0,
  total_gpu_jobs: 1,
  max_concurrent_jobs: 8,
  max_cpu_jobs: 4,
  max_gpu_jobs: 4,
  total_gpus: 0,
  available_gpu_sessions: 0,
  total_gpu_sessions: 0,
  average_cpu_percent: 35.5,
  average_memory_percent: 50.0,
};

// ─── Backups ───────────────────────────────────────────

export const mockBackups = [
  {
    filename: 'tvarr-backup-2025-02-15T020000Z.tar.gz',
    file_path: '/data/backups/tvarr-backup-2025-02-15T020000Z.tar.gz',
    created_at: '2025-02-15T02:00:00Z',
    file_size: 524288,
    database_size: 2097152,
    tvarr_version: '0.5.0',
    checksum: 'a1b2c3d4e5f6789012345678abcdef01',
    table_counts: {
      filters: 5,
      data_mapping_rules: 3,
      client_detection_rules: 4,
      encoding_profiles: 2,
      stream_sources: 3,
      epg_sources: 2,
      stream_proxies: 1,
      channels: 500,
      epg_programs: 25000,
    },
    protected: false,
    imported: false,
  },
];

export const mockBackupSchedule = {
  enabled: true,
  cron: '0 0 2 * * *',
  retention: 7,
  next_run: '2025-02-18T02:00:00Z',
};

// ─── API Response Wrappers ─────────────────────────────

/** Backend returns { proxies: [...] } — matches ListStreamProxiesOutputBody */
export function proxiesApiResponse(proxies = mockProxies) {
  return { proxies };
}

/** Backend returns { sources: [...] } — matches ListStreamSourcesOutputBody */
export function streamSourcesApiResponse(sources = mockStreamSources) {
  return { sources };
}

/** Backend returns { sources: [...] } — matches ListEpgSourcesOutputBody */
export function epgSourcesApiResponse(sources = mockEpgSources) {
  return { sources };
}

/** Backend returns { filters: [...], count } */
export function filtersApiResponse(filters = mockFilters) {
  return {
    filters,
    count: filters.length,
  };
}

/** Backend returns { profiles: [...] } */
export function encodingProfilesApiResponse(profiles = mockEncodingProfiles) {
  return {
    profiles,
  };
}

/** Backend returns { rules: [...], count } for data mapping */
export function dataMappingApiResponse(rules = mockDataMappingRules) {
  return {
    rules,
    count: rules.length,
  };
}

/** Backend returns { rules: [...], count } for client detection rules */
export function clientDetectionRulesApiResponse(rules = mockClientDetectionRules) {
  return {
    rules,
    count: rules.length,
  };
}

/** Backend returns { overrides: [...], count } for encoder overrides */
export function encoderOverridesApiResponse(overrides = mockEncoderOverrides) {
  return {
    overrides,
    count: overrides.length,
  };
}

/** Backend returns { daemons: [...], total } for transcoders */
export function transcodersApiResponse(daemons = mockDaemons) {
  return {
    daemons,
    total: daemons.length,
  };
}

/** Backend returns { backups: [...], backup_directory, schedule } */
export function backupsApiResponse(
  backups = mockBackups,
  schedule = mockBackupSchedule,
) {
  return {
    backups,
    backup_directory: '/data/backups',
    schedule,
  };
}
