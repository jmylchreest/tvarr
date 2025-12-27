/**
 * Layout utilities for ReactFlow relay pipeline visualization using dagre.
 *
 * This module uses the dagre library for automatic graph layout, which handles
 * node positioning based on the graph structure and edges.
 *
 * Layout Structure (left to right):
 * - Origin (source stream)
 * - Buffer (shared ES buffer) + Transcoder (above buffer)
 * - Processor (output format processors)
 * - Client (connected clients)
 */

import Dagre from '@dagrejs/dagre';
import type { Edge } from '@xyflow/react';

// Generic node type that works with our flow data
interface FlowNode {
  id: string;
  type?: string;
  position: { x: number; y: number };
  data: Record<string, unknown>;
  parentId?: string;
  measured?: { width?: number; height?: number };
  width?: number;
  height?: number;
}

// Fallback dimensions for unmeasured nodes
const FALLBACK_DIMENSIONS: Record<string, { width: number; height: number }> = {
  origin: { width: 256, height: 220 },
  buffer: { width: 288, height: 200 },
  transcoder: { width: 224, height: 240 },
  processor: { width: 256, height: 140 },
  client: { width: 192, height: 160 },
};

// Layout configuration
const LAYOUT_CONFIG = {
  nodesep: 80, // Vertical spacing between nodes (increased from 50)
  ranksep: 120, // Horizontal spacing between columns (increased from 100)
  marginx: 50,
  marginy: 50,
  transcoderGap: 40, // Gap between transcoders
  transcoderBufferGap: 100, // Gap between transcoders and buffer
};

/**
 * Gets the width of a node, preferring measured dimensions.
 */
function getNodeWidth(node: FlowNode): number {
  if (node.measured?.width) return node.measured.width;
  if (node.width) return node.width;
  return FALLBACK_DIMENSIONS[node.type || 'origin']?.width || 200;
}

/**
 * Gets the height of a node, preferring measured dimensions.
 */
function getNodeHeight(node: FlowNode): number {
  if (node.measured?.height) return node.measured.height;
  if (node.height) return node.height;
  return FALLBACK_DIMENSIONS[node.type || 'origin']?.height || 150;
}

/**
 * Assigns a rank (column) to each node type for left-to-right layout.
 */
function getNodeRank(node: FlowNode): number {
  switch (node.type) {
    case 'origin':
      return 0;
    case 'transcoder':
      return 1;
    case 'buffer':
      return 1;
    case 'processor':
      return 2;
    case 'client':
      return 3;
    default:
      return 0;
  }
}

/**
 * Calculates layout using dagre for automatic node positioning.
 * After dagre layout, transcoder nodes are repositioned above their buffer.
 * The layout is then shifted to ensure transcoders don't overlap with other nodes.
 */
export function calculateLayout<T extends FlowNode>(nodes: T[], edges: Edge[]): T[] {
  if (nodes.length === 0) return [];

  // Separate transcoder nodes - we'll position them manually after dagre
  const transcoderNodes = nodes.filter((n) => n.type === 'transcoder');
  const nonTranscoderNodes = nodes.filter((n) => n.type !== 'transcoder');

  // Calculate transcoder space needed upfront
  let transcoderSpaceNeeded = 0;
  if (transcoderNodes.length > 0) {
    const maxTranscoderHeight = Math.max(
      ...transcoderNodes.map((t) => getNodeHeight(t))
    );
    transcoderSpaceNeeded = maxTranscoderHeight + LAYOUT_CONFIG.transcoderBufferGap;
  }

  // Create a new dagre graph
  const g = new Dagre.graphlib.Graph().setDefaultEdgeLabel(() => ({}));

  // Configure the graph for left-to-right layout
  // Add extra top margin to account for transcoders
  g.setGraph({
    rankdir: 'LR', // Left to right
    nodesep: LAYOUT_CONFIG.nodesep,
    ranksep: LAYOUT_CONFIG.ranksep,
    marginx: LAYOUT_CONFIG.marginx,
    marginy: LAYOUT_CONFIG.marginy + transcoderSpaceNeeded,
  });

  // Add non-transcoder nodes to the graph
  for (const node of nonTranscoderNodes) {
    const width = getNodeWidth(node);
    const height = getNodeHeight(node);

    g.setNode(node.id, {
      width,
      height,
    });
  }

  // Add edges (excluding transcoder edges for now)
  for (const edge of edges) {
    const isTranscoderEdge =
      transcoderNodes.some((t) => t.id === edge.source || t.id === edge.target);

    if (!isTranscoderEdge && g.hasNode(edge.source) && g.hasNode(edge.target)) {
      g.setEdge(edge.source, edge.target);
    }
  }

  // Run the dagre layout algorithm
  Dagre.layout(g);

  // Apply the calculated positions back to non-transcoder nodes
  const layoutedNodes: T[] = nonTranscoderNodes.map((node) => {
    const nodeWithPosition = g.node(node.id);

    // Dagre returns center position, convert to top-left for React Flow
    const width = getNodeWidth(node);
    const height = getNodeHeight(node);

    return {
      ...node,
      position: {
        x: nodeWithPosition.x - width / 2,
        y: nodeWithPosition.y - height / 2,
      },
    };
  });

  // Now position transcoder nodes above the buffer node
  // Find buffer node(s) to align transcoders with
  const bufferNode = layoutedNodes.find((n) => n.type === 'buffer');

  if (bufferNode && transcoderNodes.length > 0) {
    const bufferWidth = getNodeWidth(bufferNode);
    const bufferX = bufferNode.position.x;
    const bufferY = bufferNode.position.y;

    // Get dimensions for all transcoders
    const transcoderDimensions = transcoderNodes.map((t) => ({
      node: t,
      width: getNodeWidth(t),
      height: getNodeHeight(t),
    }));

    const totalTranscoderWidth = transcoderDimensions.reduce(
      (sum, t) => sum + t.width,
      0
    ) + (transcoderNodes.length - 1) * LAYOUT_CONFIG.transcoderGap;

    // Find the tallest transcoder for positioning
    const maxTranscoderHeight = Math.max(...transcoderDimensions.map((t) => t.height));

    // Center the transcoders horizontally above the buffer
    const bufferCenterX = bufferX + bufferWidth / 2;
    let currentX = bufferCenterX - totalTranscoderWidth / 2;
    const transcoderY = bufferY - maxTranscoderHeight - LAYOUT_CONFIG.transcoderBufferGap;

    // Position each transcoder side by side
    for (const { node, width, height } of transcoderDimensions) {
      layoutedNodes.push({
        ...node,
        position: {
          x: currentX,
          // Align bottoms of transcoders (so they're level with each other)
          y: transcoderY + (maxTranscoderHeight - height),
        },
      });
      currentX += width + LAYOUT_CONFIG.transcoderGap;
    }
  } else {
    // No buffer node, just add transcoders with default position
    for (const transcoder of transcoderNodes) {
      layoutedNodes.push({
        ...transcoder,
        position: { x: 200, y: 50 },
      });
    }
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
    const width = getNodeWidth(node);
    const height = getNodeHeight(node);

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
