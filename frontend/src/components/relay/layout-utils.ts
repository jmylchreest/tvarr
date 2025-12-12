/**
 * Layout utilities for ReactFlow relay pipeline visualization.
 *
 * This module handles automatic node positioning based on node types and relationships.
 * Layout is calculated entirely on the frontend using MEASURED node dimensions from React Flow.
 *
 * Layout Structure:
 * - Column 0: Origin (source stream)
 * - Column 1: Buffer (shared ES buffer)
 * - Column 1: Transcoder (positioned ABOVE buffer, same X)
 * - Column 2: Processor (output format processors)
 * - Column 3: Client (connected clients)
 *
 * The layout uses actual measured node dimensions when available, falling back to
 * estimates only for unmeasured nodes.
 */

import type { Edge } from '@xyflow/react';

// Generic node type that works with our flow data
// This is compatible with React Flow's Node type
interface FlowNode {
  id: string;
  type?: string;
  position: { x: number; y: number };
  data: Record<string, unknown>; // Required to match React Flow's Node type
  parentId?: string;
  // React Flow adds these after measurement (dimensions may be undefined)
  measured?: { width?: number; height?: number };
  width?: number;
  height?: number;
}

// Layout configuration
interface LayoutConfig {
  /** Minimum horizontal gap between node edges */
  columnGap: number;
  /** Minimum vertical gap between node edges */
  rowGap: number;
  /** Vertical gap between client nodes (smaller for compact layout) */
  clientGap: number;
  /** Additional vertical gap above buffer for transcoder */
  transcoderGap: number;
  /** Starting X position */
  startX: number;
  /** Starting Y position (must leave room for transcoder above) */
  startY: number;
  /** Fallback widths when nodes aren't measured yet */
  fallbackWidths: Record<string, number>;
  /** Fallback heights when nodes aren't measured yet */
  fallbackHeights: Record<string, number>;
}

const DEFAULT_CONFIG: LayoutConfig = {
  columnGap: 80, // Horizontal gap between node edges
  rowGap: 40, // Vertical gap between node edges
  clientGap: 15, // Smaller gap between clients for compact layout
  transcoderGap: 60, // Gap between transcoder bottom and buffer top
  startX: 50,
  startY: 350, // Leave room for transcoder above
  fallbackWidths: {
    origin: 256,
    buffer: 288,
    transcoder: 224,
    processor: 256,
    client: 192,
  },
  fallbackHeights: {
    origin: 220,
    buffer: 200,
    transcoder: 240,
    processor: 140,
    client: 100, // Smaller client nodes
  },
};

/**
 * Gets the width of a node, preferring measured dimensions.
 */
function getNodeWidth(node: FlowNode, config: LayoutConfig): number {
  // React Flow v12+ uses node.measured.width
  if (node.measured?.width) return node.measured.width;
  // Older versions or direct setting
  if (node.width) return node.width;
  // Fallback to estimates
  return config.fallbackWidths[node.type || 'origin'] || 200;
}

/**
 * Gets the height of a node, preferring measured dimensions.
 */
function getNodeHeight(node: FlowNode, config: LayoutConfig): number {
  // React Flow v12+ uses node.measured.height
  if (node.measured?.height) return node.measured.height;
  // Older versions or direct setting
  if (node.height) return node.height;
  // Fallback to estimates
  return config.fallbackHeights[node.type || 'origin'] || 150;
}

/**
 * Groups nodes by their session ID to handle multi-session layouts.
 */
function groupNodesBySession(nodes: FlowNode[]): Map<string, FlowNode[]> {
  const sessions = new Map<string, FlowNode[]>();

  for (const node of nodes) {
    const data = node.data as Record<string, unknown> | undefined;
    const sessionId = (data?.sessionId as string) || (data?.SessionID as string) || 'default';
    if (!sessions.has(sessionId)) {
      sessions.set(sessionId, []);
    }
    sessions.get(sessionId)!.push(node);
  }

  return sessions;
}

/**
 * Groups nodes by their type within a session.
 */
function groupNodesByType(nodes: FlowNode[]): Map<string, FlowNode[]> {
  const groups = new Map<string, FlowNode[]>();

  for (const node of nodes) {
    const type = node.type || 'unknown';
    if (!groups.has(type)) {
      groups.set(type, []);
    }
    groups.get(type)!.push(node);
  }

  return groups;
}

/**
 * Finds which processor a client is connected to based on edges.
 */
function getClientProcessorMap(nodes: FlowNode[], edges: Edge[]): Map<string, string> {
  const clientToProcessor = new Map<string, string>();

  for (const edge of edges) {
    const sourceNode = nodes.find((n) => n.id === edge.source);
    const targetNode = nodes.find((n) => n.id === edge.target);

    if (sourceNode?.type === 'processor' && targetNode?.type === 'client') {
      clientToProcessor.set(edge.target, edge.source);
    }
  }

  return clientToProcessor;
}

/**
 * Calculates optimal layout for relay flow nodes using measured dimensions.
 *
 * Layout strategy:
 * 1. Group nodes by session
 * 2. Sessions stack vertically
 * 3. Within each session: Origin, Buffer, Processors, Clients are TOP-ALIGNED
 * 4. Transcoder positioned above buffer with guaranteed gap
 * 5. Node spacing uses actual measured heights + gap to prevent overlap
 */
export function calculateLayout<T extends FlowNode>(
  nodes: T[],
  edges: Edge[],
  config: Partial<LayoutConfig> = {}
): T[] {
  const cfg = { ...DEFAULT_CONFIG, ...config };

  if (nodes.length === 0) return [];

  // Map of client ID to processor ID
  const clientToProcessor = getClientProcessorMap(nodes as FlowNode[], edges);

  // Group by session
  const sessions = groupNodesBySession(nodes as FlowNode[]);
  const layoutedNodes: T[] = [];

  let sessionYOffset = cfg.startY;

  for (const [_sessionId, sessionNodes] of sessions) {
    // Group nodes by type within this session
    const typeGroups = groupNodesByType(sessionNodes);

    // Get nodes by column
    const originNodes = (typeGroups.get('origin') || []) as T[];
    const bufferNodes = (typeGroups.get('buffer') || []) as T[];
    const transcoderNodes = (typeGroups.get('transcoder') || []) as T[];
    const processorNodes = (typeGroups.get('processor') || []) as T[];
    const clientNodes = (typeGroups.get('client') || []) as T[];

    // Group clients by their processor
    const clientsByProcessor = new Map<string, T[]>();
    for (const client of clientNodes) {
      const processorId = clientToProcessor.get(client.id) || 'default';
      if (!clientsByProcessor.has(processorId)) {
        clientsByProcessor.set(processorId, []);
      }
      clientsByProcessor.get(processorId)!.push(client);
    }

    // Calculate column X positions based on actual node widths
    // Each column starts where the previous one ends + gap
    const originWidth = originNodes.length > 0 ? Math.max(...originNodes.map((n) => getNodeWidth(n, cfg))) : 0;
    const bufferWidth = bufferNodes.length > 0 ? Math.max(...bufferNodes.map((n) => getNodeWidth(n, cfg))) : 0;
    const processorWidth =
      processorNodes.length > 0 ? Math.max(...processorNodes.map((n) => getNodeWidth(n, cfg))) : 0;

    const originX = cfg.startX;
    const bufferX = originX + originWidth + cfg.columnGap;
    const processorX = bufferX + bufferWidth + cfg.columnGap;
    const clientX = processorX + processorWidth + cfg.columnGap;

    // The main row Y is where the top of origin/buffer/first-processor align
    const mainRowY = sessionYOffset;

    // Position origin nodes - stack vertically with measured heights
    let originY = mainRowY;
    for (const node of originNodes) {
      layoutedNodes.push({
        ...node,
        position: { x: originX, y: originY },
      });
      originY += getNodeHeight(node, cfg) + cfg.rowGap;
    }

    // Position buffer nodes - stack vertically with measured heights
    let bufferY = mainRowY;
    for (const node of bufferNodes) {
      layoutedNodes.push({
        ...node,
        position: { x: bufferX, y: bufferY },
      });
      bufferY += getNodeHeight(node, cfg) + cfg.rowGap;
    }

    // Position transcoder nodes ABOVE buffer
    // Calculate position so transcoder bottom is transcoderGap above buffer top
    if (transcoderNodes.length > 0) {
      const transcoderWidth = getNodeWidth(transcoderNodes[0], cfg);
      const transcoderX = bufferX + (bufferWidth - transcoderWidth) / 2;

      let transcoderY = mainRowY;
      for (let i = transcoderNodes.length - 1; i >= 0; i--) {
        const node = transcoderNodes[i];
        const height = getNodeHeight(node, cfg);
        // Position above the main row
        transcoderY -= height + cfg.transcoderGap;
        layoutedNodes.push({
          ...node,
          position: { x: transcoderX, y: transcoderY },
        });
      }
    }

    // Position processor nodes - stack vertically with measured heights
    // Track Y position for each processor so clients can align with them
    const processorPositions: Map<string, { x: number; y: number; height: number }> = new Map();
    let processorY = mainRowY;

    for (const node of processorNodes) {
      const height = getNodeHeight(node, cfg);
      processorPositions.set(node.id, { x: processorX, y: processorY, height });
      layoutedNodes.push({
        ...node,
        position: { x: processorX, y: processorY },
      });
      processorY += height + cfg.rowGap;
    }

    // Position client nodes - stack all clients compactly from mainRowY
    // This prevents clients from spreading out vertically when there are multiple processors
    let clientY = mainRowY;

    // Collect all clients in order (by processor, then unconnected)
    const allClients: T[] = [];
    for (const [, clients] of clientsByProcessor) {
      allClients.push(...clients);
    }
    // Add unconnected clients
    const unconnectedClients = clientNodes.filter((c) => !clientToProcessor.has(c.id));
    allClients.push(...unconnectedClients);

    // Position all clients compactly with smaller gap
    for (const node of allClients) {
      const height = getNodeHeight(node, cfg);
      layoutedNodes.push({
        ...node,
        position: { x: clientX, y: clientY },
      });
      clientY += height + cfg.clientGap;
    }

    // Calculate session height for next session offset
    // Find the maximum Y extent of all nodes in this session
    const sessionNodeIds = new Set(sessionNodes.map((n) => n.id));
    let maxY = mainRowY;
    for (const node of layoutedNodes) {
      if (sessionNodeIds.has(node.id)) {
        const bottom = node.position.y + getNodeHeight(node as FlowNode, cfg);
        maxY = Math.max(maxY, bottom);
      }
    }

    // Next session starts below this one with extra padding
    sessionYOffset = maxY + cfg.rowGap * 2;
  }

  return layoutedNodes;
}

/**
 * Calculates the bounding box of all nodes for viewport fitting.
 */
export function getNodesBounds(nodes: FlowNode[]): {
  minX: number;
  minY: number;
  maxX: number;
  maxY: number;
  width: number;
  height: number;
} {
  if (nodes.length === 0) {
    return { minX: 0, minY: 0, maxX: 0, maxY: 0, width: 0, height: 0 };
  }

  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;

  for (const node of nodes) {
    const { x, y } = node.position;
    const width = getNodeWidth(node, DEFAULT_CONFIG);
    const height = getNodeHeight(node, DEFAULT_CONFIG);

    minX = Math.min(minX, x);
    minY = Math.min(minY, y);
    maxX = Math.max(maxX, x + width);
    maxY = Math.max(maxY, y + height);
  }

  return {
    minX,
    minY,
    maxX,
    maxY,
    width: maxX - minX,
    height: maxY - minY,
  };
}
