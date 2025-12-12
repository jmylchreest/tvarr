/**
 * Layout utilities for ReactFlow relay pipeline visualization.
 *
 * This module handles automatic node positioning based on node types and relationships.
 * Layout is calculated entirely on the frontend to adapt to viewport and node counts.
 *
 * Layout Structure:
 * - Column 0: Origin (source stream)
 * - Column 1: Buffer (shared ES buffer)
 * - Column 1: Transcoder (positioned ABOVE buffer, same X)
 * - Column 2: Processor (output format processors)
 * - Column 3: Client (connected clients)
 *
 * All columns use consistent spacing for visual clarity.
 */

import type { Edge } from '@xyflow/react';

// Generic node type that works with our flow data
interface FlowNode {
  id: string;
  type?: string;
  position: { x: number; y: number };
  data?: Record<string, unknown>;
  parentId?: string;
}

// Layout configuration
interface LayoutConfig {
  /** Base horizontal spacing between columns */
  columnSpacing: number;
  /** Vertical spacing between nodes in the same column */
  rowSpacing: number;
  /** Vertical offset for transcoder above buffer */
  transcoderOffset: number;
  /** Starting X position */
  startX: number;
  /** Starting Y position */
  startY: number;
  /** Node widths by type (for centering calculations) */
  nodeWidths: Record<string, number>;
  /** Node heights by type (for vertical spacing) */
  nodeHeights: Record<string, number>;
}

const DEFAULT_CONFIG: LayoutConfig = {
  columnSpacing: 100, // Base gap between node edges
  rowSpacing: 140, // Space between rows
  transcoderOffset: 180, // How far above buffer the transcoder sits
  startX: 50,
  startY: 200, // Extra top margin for transcoder
  nodeWidths: {
    origin: 256,
    buffer: 224,
    transcoder: 224,
    processor: 256,
    client: 192,
  },
  nodeHeights: {
    origin: 200,
    buffer: 180,
    transcoder: 180,
    processor: 100,
    client: 120,
  },
};

// Edge length constraints for adaptive layout
const EDGE_CONSTRAINTS = {
  minGap: 80, // Minimum gap between node edges
  maxGap: 140, // Maximum gap between node edges
  targetGap: 100, // Ideal gap between node edges
};

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
 * Calculate adaptive gap based on total node count.
 * More nodes = tighter gaps to keep layout compact.
 * Fewer nodes = larger gaps for better readability.
 */
function calculateAdaptiveGap(totalNodes: number): number {
  // Scale factor: fewer nodes get more space
  // 5 nodes = 1.2x, 10 nodes = 1.0x, 20 nodes = 0.8x
  const scaleFactor = Math.max(0.8, Math.min(1.2, 10 / Math.max(totalNodes, 5)));
  const gap = EDGE_CONSTRAINTS.targetGap * scaleFactor;

  // Clamp to min/max
  return Math.max(EDGE_CONSTRAINTS.minGap, Math.min(EDGE_CONSTRAINTS.maxGap, gap));
}

/**
 * Calculates optimal layout for relay flow nodes.
 *
 * Layout strategy:
 * 1. Group nodes by session
 * 2. Within each session, arrange nodes in columns by type
 * 3. Position transcoder above buffer with sufficient clearance
 * 4. Position clients vertically aligned with their connected processor
 * 5. Use adaptive spacing based on total node count
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

    // Get nodes by column (cast back to T for proper typing)
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

    // Find max clients per processor to calculate spacing
    let maxClientsPerProcessor = 0;
    for (const clients of clientsByProcessor.values()) {
      maxClientsPerProcessor = Math.max(maxClientsPerProcessor, clients.length);
    }

    // Calculate the maximum height needed for this session
    const maxNodesInColumn = Math.max(
      originNodes.length,
      bufferNodes.length,
      processorNodes.length,
      maxClientsPerProcessor,
      1 // At least 1 row
    );

    // Calculate center Y for this session's main row
    const sessionCenterY = sessionYOffset + (maxNodesInColumn * cfg.rowSpacing) / 2;

    // Calculate adaptive gap based on total node count in this session
    const totalNodes =
      originNodes.length +
      bufferNodes.length +
      transcoderNodes.length +
      processorNodes.length +
      clientNodes.length;

    const gap = calculateAdaptiveGap(totalNodes);

    // Column X positions - using adaptive gaps between node edges
    // Each column is: previous column X + previous node width + gap
    const originX = cfg.startX;
    const bufferX = originX + cfg.nodeWidths.origin + gap;
    const processorX = bufferX + cfg.nodeWidths.buffer + gap;
    const clientX = processorX + cfg.nodeWidths.processor + gap;

    // Position origin nodes (column 0)
    for (let i = 0; i < originNodes.length; i++) {
      const y = sessionCenterY - ((originNodes.length - 1) / 2 - i) * cfg.rowSpacing;
      layoutedNodes.push({
        ...originNodes[i],
        position: { x: originX, y },
      });
    }

    // Position buffer nodes (column 1)
    for (let i = 0; i < bufferNodes.length; i++) {
      const y = sessionCenterY - ((bufferNodes.length - 1) / 2 - i) * cfg.rowSpacing;
      layoutedNodes.push({
        ...bufferNodes[i],
        position: { x: bufferX, y },
      });
    }

    // Position transcoder nodes (above buffer, same X, centered horizontally)
    if (transcoderNodes.length > 0) {
      // Center the transcoder above the buffer
      const bufferWidth = cfg.nodeWidths.buffer || 224;
      const transcoderWidth = cfg.nodeWidths.transcoder || 224;
      const transcoderX = bufferX + (bufferWidth - transcoderWidth) / 2;
      const transcoderY = sessionCenterY - cfg.transcoderOffset;

      for (let i = 0; i < transcoderNodes.length; i++) {
        layoutedNodes.push({
          ...transcoderNodes[i],
          position: {
            x: transcoderX + i * (transcoderWidth + 20),
            y: transcoderY,
          },
        });
      }
    }

    // Position processor nodes (column 2)
    const processorPositions: Map<string, { x: number; y: number }> = new Map();

    // Adaptive row spacing for processors when there are many
    const processorRowSpacing =
      processorNodes.length > 3
        ? Math.max(cfg.nodeHeights.processor + 30, cfg.rowSpacing * 0.7)
        : cfg.rowSpacing;

    if (processorNodes.length === 1) {
      // Single processor - center it
      processorPositions.set(processorNodes[0].id, {
        x: processorX,
        y: sessionCenterY,
      });
      layoutedNodes.push({
        ...processorNodes[0],
        position: { x: processorX, y: sessionCenterY },
      });
    } else if (processorNodes.length > 1) {
      // Multiple processors - spread vertically with adaptive spacing
      const totalProcessorSpan = (processorNodes.length - 1) * processorRowSpacing;
      const processorStartY = sessionCenterY - totalProcessorSpan / 2;

      for (let i = 0; i < processorNodes.length; i++) {
        const y = processorStartY + i * processorRowSpacing;
        processorPositions.set(processorNodes[i].id, { x: processorX, y });
        layoutedNodes.push({
          ...processorNodes[i],
          position: { x: processorX, y },
        });
      }
    }

    // Position client nodes - grouped by processor
    // Adaptive spacing based on client count
    const clientVerticalSpacing =
      clientNodes.length > 5
        ? Math.max(cfg.nodeHeights.client + 10, 100)
        : cfg.nodeHeights.client + 20;

    for (const [processorId, clients] of clientsByProcessor) {
      const processorPos = processorPositions.get(processorId);
      const baseCenterY = processorPos?.y ?? sessionCenterY;

      // Center clients around their processor's Y position
      const totalClientSpan = (clients.length - 1) * clientVerticalSpacing;
      const clientStartY = baseCenterY - totalClientSpan / 2;

      for (let i = 0; i < clients.length; i++) {
        const y = clientStartY + i * clientVerticalSpacing;
        layoutedNodes.push({
          ...clients[i],
          position: { x: clientX, y },
        });
      }
    }

    // Handle clients not connected to any processor
    const unconnectedClients = clientNodes.filter((c) => !clientToProcessor.has(c.id));
    if (unconnectedClients.length > 0) {
      const lastProcessorY = Math.max(
        ...Array.from(processorPositions.values()).map((p) => p.y),
        sessionCenterY
      );

      for (let i = 0; i < unconnectedClients.length; i++) {
        const y = lastProcessorY + (i + 1) * clientVerticalSpacing;
        layoutedNodes.push({
          ...unconnectedClients[i],
          position: { x: clientX, y },
        });
      }
    }

    // Update session Y offset for next session
    const sessionHeight = Math.max(maxNodesInColumn * cfg.rowSpacing, cfg.rowSpacing * 2);
    // Add extra space if there's a transcoder
    const transcoderSpace = transcoderNodes.length > 0 ? cfg.transcoderOffset : 0;
    sessionYOffset += sessionHeight + transcoderSpace + cfg.rowSpacing;
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
    const width = DEFAULT_CONFIG.nodeWidths[node.type || 'origin'] || 200;
    const height = DEFAULT_CONFIG.nodeHeights[node.type || 'origin'] || 100;

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
