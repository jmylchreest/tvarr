// Type definitions for relay flow visualization data

export type RouteType = 'passthrough' | 'repackage' | 'transcode';
export type FlowNodeType = 'origin' | 'processor' | 'client';

export interface FlowPosition {
  x: number;
  y: number;
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
  ingressBps?: number;

  // Processor node fields
  routeType?: RouteType;
  profileName?: string;
  outputFormat?: string;
  outputVideoCodec?: string;
  outputAudioCodec?: string;
  cpuPercent?: number;
  memoryMB?: number;
  processingBps?: number;

  // Client node fields
  clientId?: string;
  playerType?: string;
  remoteAddr?: string;
  userAgent?: string;
  bytesRead?: number;
  egressBps?: number;

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
  connected_at: string;
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
