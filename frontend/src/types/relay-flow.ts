// Type definitions for relay flow visualization data

export type RouteType = 'passthrough' | 'repackage' | 'transcode';
export type FlowNodeType = 'origin' | 'buffer' | 'transcoder' | 'processor' | 'client';

export interface FlowPosition {
  x: number;
  y: number;
}

// BufferVariantInfo describes a codec variant in the shared buffer
export interface BufferVariantInfo {
  variant: string; // e.g., "h264/aac", "hevc/aac"
  videoCodec: string;
  audioCodec: string;
  videoSamples: number;
  audioSamples: number;
  bytesIngested: number;
  maxBytes: number; // Maximum bytes allowed per variant (e.g., 30MB)
  utilization: number; // 0-100 percentage of max used
  isSource: boolean;
}

export interface FlowNodeData {
  // Common fields
  label: string;
  sessionId?: string;
  channelId?: string;
  channelName?: string;

  // Origin node fields
  sourceName?: string; // Name of the stream source (e.g., "s8k")
  sourceUrl?: string;
  sourceFormat?: string;
  videoCodec?: string;
  audioCodec?: string;
  framerate?: number; // Video framerate (fps)
  videoWidth?: number; // Video width in pixels
  videoHeight?: number; // Video height in pixels
  ingressBps?: number;
  totalBytesIn?: number;
  durationSecs?: number;

  // Bandwidth history for sparkline (last 30 samples, ~1 sample/sec)
  ingressHistory?: number[];

  // Buffer node fields
  bufferVariants?: BufferVariantInfo[];
  bufferMemoryBytes?: number;
  maxBufferBytes?: number; // Maximum buffer size per variant
  videoSampleCount?: number;
  audioSampleCount?: number;
  bufferUtilization?: number; // 0-100 percentage of max buffer used

  // Transcoder node fields (FFmpeg)
  transcoderId?: string;
  sourceVideoCodec?: string; // Source video codec (e.g., "h264")
  sourceAudioCodec?: string; // Source audio codec (e.g., "aac")
  targetVideoCodec?: string; // Target codec name (e.g., "h265", "aac")
  targetAudioCodec?: string;
  videoEncoder?: string; // FFmpeg encoder name (e.g., "libx265", "h264_nvenc")
  audioEncoder?: string; // FFmpeg encoder name (e.g., "aac", "libopus")
  hwAccelType?: string; // Hardware acceleration type (e.g., "cuda", "qsv", "vaapi")
  hwAccelDevice?: string; // Hardware acceleration device (e.g., "/dev/dri/renderD128")
  encodingSpeed?: number;
  transcoderCpu?: number;
  transcoderMemMb?: number;
  transcoderBytesIn?: number;

  // Resource history for sparklines (last 30 samples, ~1 sample/sec)
  transcoderCpuHistory?: number[];
  transcoderMemHistory?: number[];

  // Processor node fields
  routeType?: RouteType;
  profileName?: string;
  outputFormat?: string;
  outputVideoCodec?: string;
  outputAudioCodec?: string;
  cpuPercent?: number;
  memoryMB?: number;
  processingBps?: number;
  totalBytesOut?: number;

  // Bandwidth history for sparkline (last 30 samples, ~1 sample/sec)
  egressHistory?: number[];

  // Client node fields
  clientId?: string;
  playerType?: string;
  clientFormat?: string; // Format this client is using (hls, mpegts, dash)
  remoteAddr?: string;
  userAgent?: string;
  detectionRule?: string;
  bytesRead?: number;
  egressBps?: number;
  connectedSecs?: number;

  // Bandwidth history for sparkline (last 30 samples, ~1 sample/sec)
  clientEgressHistory?: number[];

  // Status fields
  inFallback?: boolean;
  error?: string;
}

export interface RelayFlowNode {
  id: string;
  type: FlowNodeType;
  position: FlowPosition;
  data: FlowNodeData;
  parentId?: string;
}

export interface FlowEdgeData {
  bandwidthBps: number;
  videoCodec?: string;
  audioCodec?: string;
  format?: string;
}

export interface FlowEdgeStyle {
  stroke?: string;
  strokeWidth?: number;
}

export interface RelayFlowEdge {
  id: string;
  source: string;
  target: string;
  type?: string;
  animated: boolean;
  label?: string;
  data: FlowEdgeData;
  style?: FlowEdgeStyle;
}

export interface FlowGraphMetadata {
  totalSessions: number;
  totalClients: number;
  totalIngressBps: number;
  totalEgressBps: number;
  generatedAt: string;
  // System resource usage
  systemCpuPercent?: number;
  systemMemoryPercent?: number;
  systemMemoryUsedMb?: number;
  systemMemoryTotalMb?: number;
}

export interface RelayFlowGraph {
  nodes: RelayFlowNode[];
  edges: RelayFlowEdge[];
  metadata: FlowGraphMetadata;
}

// API response types
export interface RelaySessionInfo {
  session_id: string;
  channel_id: string;
  channel_name: string;
  stream_source_name?: string; // Name of the stream source (e.g., "s8k")
  profile_name: string;
  route_type: RouteType;
  source_url: string;
  source_format: string;
  output_format: string;
  active_processor_formats?: string[];
  video_codec?: string;
  audio_codec?: string;
  started_at: string;
  last_activity: string;
  duration_secs: number;
  client_count: number;
  clients?: RelayClientInfo[];
  bytes_in: number;
  bytes_out: number;
  ingress_rate_bps: number;
  egress_rate_bps: number;
  cpu_percent?: number;
  memory_bytes?: number;
  memory_percent?: number;
  ffmpeg_pid?: number;
  buffer_utilization?: number;
  segment_count?: number;
  in_fallback: boolean;
  error?: string;
}

export interface RelayClientInfo {
  client_id: string;
  user_agent?: string;
  remote_addr?: string;
  player_type?: string;
  detection_rule?: string;
  connected_at: string;
  connected_secs: number;
  bytes_read: number;
}

// Helper function to format bytes per second
export function formatBps(bps: number): string {
  if (bps === 0) return '0 B/s';
  const units = ['B/s', 'KB/s', 'MB/s', 'GB/s'];
  const i = Math.floor(Math.log(bps) / Math.log(1024));
  return `${(bps / Math.pow(1024, i)).toFixed(1)} ${units[Math.min(i, units.length - 1)]}`;
}

// Helper function to format bytes
export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[Math.min(i, units.length - 1)]}`;
}
