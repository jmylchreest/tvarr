'use client';

import { useCallback, useMemo, useEffect, useRef } from 'react';
import {
  ReactFlow,
  Background,
  BackgroundVariant,
  useNodesState,
  useEdgesState,
  useReactFlow,
  useNodesInitialized,
  ReactFlowProvider,
  type Node,
  type Edge,
  type NodeChange,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Loader2, AlertCircle, Activity } from 'lucide-react';
import { useRelayFlowData } from '@/hooks/use-relay-flow-data';
import { formatBps, type FlowEdgeData } from '@/types/relay-flow';
import { OriginNode, BufferNode, TranscoderNode, ProcessorNode, ClientNode } from './nodes';
import { AnimatedEdge } from './edges';
import { calculateLayout } from './layout-utils';
import { buildEdgesWithHandles } from './edge-builder';

// Define custom node types
const nodeTypes = {
  origin: OriginNode,
  buffer: BufferNode,
  transcoder: TranscoderNode,
  processor: ProcessorNode,
  client: ClientNode,
};

// Define custom edge types
const edgeTypes = {
  animated: AnimatedEdge,
};

export interface FlowMetadata {
  totalSessions: number;
  totalClients: number;
  totalIngressBps: number;
  totalEgressBps: number;
}

interface RelayFlowDiagramProps {
  pollingInterval?: number;
  /** Whether polling is enabled. When false, no automatic fetching occurs. */
  enabled?: boolean;
  className?: string;
  onMetadataUpdate?: (metadata: FlowMetadata) => void;
}

function RelayFlowDiagramInner({ pollingInterval = 2000, enabled = true, className = '', onMetadataUpdate }: RelayFlowDiagramProps) {
  const { data, isLoading, error, refetch } = useRelayFlowData({
    pollingInterval,
    enabled,
  });

  // Notify parent of metadata updates
  useEffect(() => {
    if (data?.metadata && onMetadataUpdate) {
      onMetadataUpdate({
        totalSessions: data.metadata.totalSessions ?? 0,
        totalClients: data.metadata.totalClients ?? 0,
        totalIngressBps: data.metadata.totalIngressBps ?? 0,
        totalEgressBps: data.metadata.totalEgressBps ?? 0,
      });
    }
  }, [data?.metadata, onMetadataUpdate]);

  const { fitView, getNodes } = useReactFlow();
  const nodesInitialized = useNodesInitialized();

  // Track if we've done the measured layout pass
  const hasDoneMeasuredLayout = useRef(false);
  // Debounce timer for re-layout
  const layoutTimer = useRef<NodeJS.Timeout | null>(null);
  // Track valid node IDs from current API data to filter out stale nodes
  const validNodeIds = useRef<Set<string>>(new Set());

  // Convert flow graph data to React Flow format
  const { nodes: rawNodes, edges } = useMemo(() => {
    if (!data?.nodes || !data?.edges) {
      validNodeIds.current = new Set();
      return { nodes: [] as Node[], edges: [] as Edge[] };
    }

    const nodes: Node[] = data.nodes.map((node) => ({
      id: node.id,
      type: node.type,
      position: { x: 0, y: 0 }, // Will be calculated by layout
      data: node.data as unknown as Record<string, unknown>,
    }));

    // Track valid node IDs from current API data
    validNodeIds.current = new Set(nodes.map(n => n.id));

    const backendEdges = data.edges.map((edge) => ({
      id: edge.id,
      source: edge.source,
      target: edge.target,
      type: edge.type || 'animated',
      animated: edge.animated,
      data: edge.data as FlowEdgeData,
      style: edge.style as React.CSSProperties | undefined,
    }));

    const edges: Edge[] = buildEdgesWithHandles(nodes, backendEdges);

    return { nodes, edges };
  }, [data?.nodes, data?.edges]);

  // Calculate initial layout
  const layoutedNodes = useMemo((): Node[] => {
    if (rawNodes.length === 0) return [];
    return calculateLayout(rawNodes, edges);
  }, [rawNodes, edges]);

  const [nodesState, setNodes, onNodesChange] = useNodesState(layoutedNodes);
  const [edgesState, setEdges, onEdgesChange] = useEdgesState(edges);

  // Handle node changes - detect dimension changes and re-layout
  const handleNodesChange = useCallback(
    (changes: NodeChange[]) => {
      // Check if any dimension changes occurred
      const hasDimensionChange = changes.some(
        (change) => change.type === 'dimensions' && change.dimensions
      );

      onNodesChange(changes);

      // Debounce re-layout when dimensions change
      if (hasDimensionChange) {
        if (layoutTimer.current) {
          clearTimeout(layoutTimer.current);
        }
        layoutTimer.current = setTimeout(() => {
          const measuredNodes = getNodes();
          if (measuredNodes.length === 0) return;

          // Filter out stale nodes that no longer exist in API data
          const currentValidIds = validNodeIds.current;
          const validMeasuredNodes = measuredNodes.filter(n => currentValidIds.has(n.id));
          if (validMeasuredNodes.length === 0) return;

          const reLayoutedNodes = calculateLayout(validMeasuredNodes, edges);
          setNodes(reLayoutedNodes);

          // Fit view after re-layout
          setTimeout(() => {
            fitView({ padding: 0.1, duration: 200 });
          }, 50);
        }, 150);
      }
    },
    [onNodesChange, edges, getNodes, setNodes, fitView]
  );

  // Update nodes when data changes
  useEffect(() => {
    // Reset measured layout flag when nodes change
    hasDoneMeasuredLayout.current = false;

    // Always update nodes/edges to ensure stale nodes are removed
    setNodes(layoutedNodes);
    setEdges(edges);

    // Fit view after layout (only if we have nodes)
    if (layoutedNodes.length > 0) {
      setTimeout(() => {
        fitView({ padding: 0.1, duration: 200 });
      }, 100);
    }
  }, [layoutedNodes, edges, setNodes, setEdges, fitView]);

  // Re-layout with measured dimensions after React Flow measures nodes
  useEffect(() => {
    if (!nodesInitialized || hasDoneMeasuredLayout.current) return;
    if (nodesState.length === 0) return;

    const measuredNodes = getNodes();
    if (measuredNodes.length === 0) return;

    // Check if any nodes have been measured
    const hasMeasuredNodes = measuredNodes.some((n) => n.measured?.width || n.measured?.height);
    if (!hasMeasuredNodes) return;

    hasDoneMeasuredLayout.current = true;

    // Filter out stale nodes that no longer exist in API data
    const currentValidIds = validNodeIds.current;
    const validMeasuredNodes = measuredNodes.filter(n => currentValidIds.has(n.id));
    if (validMeasuredNodes.length === 0) return;

    // Re-calculate layout using measured dimensions
    const reLayoutedNodes = calculateLayout(validMeasuredNodes, edges);
    setNodes(reLayoutedNodes);

    // Fit view after measured layout
    setTimeout(() => {
      fitView({ padding: 0.1, duration: 200 });
    }, 50);
  }, [nodesInitialized, nodesState.length, edges, getNodes, setNodes, fitView]);

  const onInit = useCallback(() => {
    setTimeout(() => {
      fitView({ padding: 0.1 });
    }, 100);
  }, [fitView]);

  if (isLoading && !data) {
    return (
      <Card className={className}>
        <CardContent className="flex items-center justify-center h-64">
          <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
        </CardContent>
      </Card>
    );
  }

  if (error) {
    return (
      <Card className={className}>
        <CardContent className="flex flex-col items-center justify-center h-64 gap-4">
          <AlertCircle className="h-8 w-8 text-destructive" />
          <p className="text-sm text-muted-foreground">{error.message}</p>
          <button onClick={refetch} className="text-sm text-primary hover:underline">
            Retry
          </button>
        </CardContent>
      </Card>
    );
  }

  const hasActiveSessions = data?.metadata?.totalSessions && data.metadata.totalSessions > 0;

  return (
    <Card className={className}>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <CardTitle className="text-lg font-medium flex items-center gap-2">
            <Activity className="h-5 w-5 text-primary" />
            Relay Sessions
          </CardTitle>

          {/* Summary badges */}
          {data?.metadata && (
            <div className="flex items-center gap-2 flex-wrap">
              <Badge variant="outline">
                {data.metadata.totalSessions} session{data.metadata.totalSessions !== 1 ? 's' : ''}
              </Badge>
              <Badge variant="outline">
                {data.metadata.totalClients} client{data.metadata.totalClients !== 1 ? 's' : ''}
              </Badge>
              {data.metadata.totalIngressBps > 0 && (
                <Badge variant="secondary" className="text-green-600 dark:text-green-400">
                  {formatBps(data.metadata.totalIngressBps)} in
                </Badge>
              )}
              {data.metadata.totalEgressBps > 0 && (
                <Badge variant="secondary" className="text-blue-600 dark:text-blue-400">
                  {formatBps(data.metadata.totalEgressBps)} out
                </Badge>
              )}
            </div>
          )}
        </div>
      </CardHeader>
      <CardContent>
        {!hasActiveSessions ? (
          <div className="flex flex-col items-center justify-center h-64 text-muted-foreground">
            <Activity className="h-12 w-12 mb-4 opacity-50" />
            <p className="text-sm">No active sessions</p>
            <p className="text-xs mt-1">Sessions will appear here when channels are streaming</p>
          </div>
        ) : (
          <div className="min-h-[400px] h-[calc(100vh-300px)] w-full border rounded-lg overflow-hidden bg-background">
            <ReactFlow
              nodes={nodesState}
              edges={edgesState}
              onNodesChange={handleNodesChange}
              onEdgesChange={onEdgesChange}
              onInit={onInit}
              nodeTypes={nodeTypes}
              edgeTypes={edgeTypes}
              fitView
              fitViewOptions={{
                padding: 0.1,
                includeHiddenNodes: false,
              }}
              minZoom={0.3}
              maxZoom={1.5}
              defaultViewport={{ x: 0, y: 0, zoom: 0.8 }}
              proOptions={{ hideAttribution: true }}
              nodesDraggable={true}
              nodesConnectable={false}
              elementsSelectable={true}
            >
              <Background variant={BackgroundVariant.Dots} gap={12} size={1} />
            </ReactFlow>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

// Wrap with ReactFlowProvider for useReactFlow hook
export function RelayFlowDiagram(props: RelayFlowDiagramProps) {
  return (
    <ReactFlowProvider>
      <RelayFlowDiagramInner {...props} />
    </ReactFlowProvider>
  );
}

export default RelayFlowDiagram;
