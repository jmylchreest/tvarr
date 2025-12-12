/**
 * Edge builder utility for React Flow relay visualization.
 *
 * This module handles creating edges with the correct handle IDs based on node types.
 * Handle IDs follow the convention: {node}-{connected_node}-{in|out}
 *
 * Node Handle Reference:
 * - Origin:     origin-buffer-out (right edge, source)
 * - Buffer:     buffer-origin-in (left edge, target)
 *               buffer-ffmpeg-out (top right, source) - sends TO ffmpeg
 *               buffer-ffmpeg-in (top left, target) - receives FROM ffmpeg
 *               buffer-processor-out (right edge, source)
 * - FFmpeg:     ffmpeg-buffer-in (bottom right, target) - receives FROM buffer
 *               ffmpeg-buffer-out (bottom left, source) - sends TO buffer
 * - Processor:  processor-buffer-in (left edge, target)
 *               processor-client-out (right edge, source)
 * - Client:     client-processor-in (left edge, target)
 *
 * Flow Connections:
 * - origin-buffer-out → buffer-origin-in
 * - buffer-ffmpeg-out → ffmpeg-buffer-in (source data to transcoder)
 * - ffmpeg-buffer-out → buffer-ffmpeg-in (transcoded data back)
 * - buffer-processor-out → processor-buffer-in
 * - processor-client-out → client-processor-in
 */

import type { Edge, Node } from '@xyflow/react';
import type { FlowEdgeData } from '@/types/relay-flow';

/**
 * Determines the appropriate source and target handles based on node types.
 */
function getHandlesForConnection(
  sourceType: string,
  targetType: string
): { sourceHandle: string; targetHandle: string } | null {
  // Origin → Buffer
  if (sourceType === 'origin' && targetType === 'buffer') {
    return {
      sourceHandle: 'origin-buffer-out',
      targetHandle: 'buffer-origin-in',
    };
  }

  // Buffer → Transcoder (sending source data TO ffmpeg)
  if (sourceType === 'buffer' && targetType === 'transcoder') {
    return {
      sourceHandle: 'buffer-ffmpeg-out',
      targetHandle: 'ffmpeg-buffer-in',
    };
  }

  // Transcoder → Buffer (sending transcoded data back FROM ffmpeg)
  if (sourceType === 'transcoder' && targetType === 'buffer') {
    return {
      sourceHandle: 'ffmpeg-buffer-out',
      targetHandle: 'buffer-ffmpeg-in',
    };
  }

  // Buffer → Processor
  if (sourceType === 'buffer' && targetType === 'processor') {
    return {
      sourceHandle: 'buffer-processor-out',
      targetHandle: 'processor-buffer-in',
    };
  }

  // Processor → Client
  if (sourceType === 'processor' && targetType === 'client') {
    return {
      sourceHandle: 'processor-client-out',
      targetHandle: 'client-processor-in',
    };
  }

  // Unknown connection type
  return null;
}

/**
 * Creates an edge with the correct handle IDs based on source and target node types.
 */
export function createEdge(
  sourceNode: Node,
  targetNode: Node,
  data: FlowEdgeData,
  animated: boolean = true
): Edge {
  const sourceType = sourceNode.type || 'unknown';
  const targetType = targetNode.type || 'unknown';

  const handles = getHandlesForConnection(sourceType, targetType);

  const edgeId = handles
    ? `edge-${sourceNode.id}-${handles.sourceHandle}-${targetNode.id}-${handles.targetHandle}`
    : `edge-${sourceNode.id}-${targetNode.id}`;

  return {
    id: edgeId,
    source: sourceNode.id,
    target: targetNode.id,
    sourceHandle: handles?.sourceHandle,
    targetHandle: handles?.targetHandle,
    type: 'animated',
    animated: animated && data.bandwidthBps > 0,
    data: data as unknown as Record<string, unknown>,
  };
}

/**
 * Creates edges from backend edge data, adding proper handle IDs based on node types.
 * This transforms backend edges (which may not have handle info) into properly connected edges.
 */
export function buildEdgesWithHandles(
  nodes: Node[],
  backendEdges: Array<{
    id: string;
    source: string;
    target: string;
    type?: string;
    animated: boolean;
    data: FlowEdgeData;
    style?: React.CSSProperties;
  }>
): Edge[] {
  // Create a map for quick node lookup
  const nodeMap = new Map<string, Node>();
  for (const node of nodes) {
    nodeMap.set(node.id, node);
  }

  return backendEdges.map((edge) => {
    const sourceNode = nodeMap.get(edge.source);
    const targetNode = nodeMap.get(edge.target);

    if (!sourceNode || !targetNode) {
      // If nodes not found, return edge as-is
      return {
        id: edge.id,
        source: edge.source,
        target: edge.target,
        type: edge.type || 'animated',
        animated: edge.animated,
        data: edge.data as unknown as Record<string, unknown>,
        style: edge.style,
      };
    }

    const sourceType = sourceNode.type || 'unknown';
    const targetType = targetNode.type || 'unknown';
    const handles = getHandlesForConnection(sourceType, targetType);

    // Create new edge ID that includes handle info for uniqueness
    const edgeId = handles
      ? `edge-${edge.source}-${handles.sourceHandle}-${edge.target}-${handles.targetHandle}`
      : edge.id;

    return {
      id: edgeId,
      source: edge.source,
      target: edge.target,
      sourceHandle: handles?.sourceHandle,
      targetHandle: handles?.targetHandle,
      type: edge.type || 'animated',
      animated: edge.animated,
      data: edge.data as unknown as Record<string, unknown>,
      style: edge.style,
    };
  });
}

/**
 * Builds edges directly from nodes based on their relationships.
 * This can be used when backend only sends nodes without edge definitions.
 */
export function buildEdgesFromNodes(nodes: Node[]): Edge[] {
  const edges: Edge[] = [];

  // Group nodes by session
  const sessionNodes = new Map<string, Map<string, Node[]>>();

  for (const node of nodes) {
    const sessionId = (node.data as Record<string, unknown>)?.sessionId as string;
    if (!sessionId) continue;

    if (!sessionNodes.has(sessionId)) {
      sessionNodes.set(sessionId, new Map());
    }

    const typeMap = sessionNodes.get(sessionId)!;
    const nodeType = node.type || 'unknown';

    if (!typeMap.has(nodeType)) {
      typeMap.set(nodeType, []);
    }
    typeMap.get(nodeType)!.push(node);
  }

  // For each session, create edges based on the flow
  for (const [, typeMap] of sessionNodes) {
    const origins = typeMap.get('origin') || [];
    const buffers = typeMap.get('buffer') || [];
    const transcoders = typeMap.get('transcoder') || [];
    const processors = typeMap.get('processor') || [];
    const clients = typeMap.get('client') || [];

    // Origin → Buffer
    for (const origin of origins) {
      for (const buffer of buffers) {
        edges.push(
          createEdge(origin, buffer, {
            bandwidthBps: (origin.data as Record<string, unknown>)?.ingressBps as number || 0,
            videoCodec: (origin.data as Record<string, unknown>)?.videoCodec as string,
            audioCodec: (origin.data as Record<string, unknown>)?.audioCodec as string,
            format: (origin.data as Record<string, unknown>)?.sourceFormat as string,
          })
        );
      }
    }

    // Buffer ↔ Transcoder (bidirectional)
    for (const buffer of buffers) {
      for (const transcoder of transcoders) {
        // Buffer → Transcoder (source data)
        edges.push(
          createEdge(buffer, transcoder, {
            bandwidthBps: (buffer.data as Record<string, unknown>)?.ingressBps as number || 0,
            videoCodec: (buffer.data as Record<string, unknown>)?.videoCodec as string,
            audioCodec: (buffer.data as Record<string, unknown>)?.audioCodec as string,
            format: 'es',
          })
        );

        // Transcoder → Buffer (transcoded data)
        edges.push(
          createEdge(transcoder, buffer, {
            bandwidthBps: (transcoder.data as Record<string, unknown>)?.processingBps as number || 0,
            videoCodec: (transcoder.data as Record<string, unknown>)?.targetVideoCodec as string,
            audioCodec: (transcoder.data as Record<string, unknown>)?.targetAudioCodec as string,
            format: 'es',
          })
        );
      }
    }

    // Buffer → Processors
    for (const buffer of buffers) {
      for (const processor of processors) {
        edges.push(
          createEdge(buffer, processor, {
            bandwidthBps: (processor.data as Record<string, unknown>)?.processingBps as number || 0,
            videoCodec: (processor.data as Record<string, unknown>)?.outputVideoCodec as string,
            audioCodec: (processor.data as Record<string, unknown>)?.outputAudioCodec as string,
            format: (processor.data as Record<string, unknown>)?.outputFormat as string,
          })
        );
      }
    }

    // Processor → Clients (need to match by format or parentId)
    for (const client of clients) {
      const clientData = client.data as Record<string, unknown>;
      const clientFormat = clientData?.clientFormat as string;
      const parentId = client.parentId;

      // Find matching processor by parentId or format
      let matchedProcessor: Node | undefined;

      if (parentId) {
        matchedProcessor = processors.find((p) => p.id === parentId);
      }

      if (!matchedProcessor && clientFormat) {
        matchedProcessor = processors.find((p) => {
          const processorData = p.data as Record<string, unknown>;
          return processorData?.outputFormat === clientFormat;
        });
      }

      if (!matchedProcessor && processors.length > 0) {
        // Default to first processor if no match found
        matchedProcessor = processors[0];
      }

      if (matchedProcessor) {
        edges.push(
          createEdge(matchedProcessor, client, {
            bandwidthBps: clientData?.egressBps as number || 0,
            videoCodec: (matchedProcessor.data as Record<string, unknown>)?.outputVideoCodec as string,
            audioCodec: (matchedProcessor.data as Record<string, unknown>)?.outputAudioCodec as string,
            format: clientFormat,
          })
        );
      }
    }
  }

  return edges;
}
