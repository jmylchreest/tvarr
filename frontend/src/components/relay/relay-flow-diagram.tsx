'use client';

import { useCallback, useMemo, useEffect, useRef } from 'react';
import {
  ReactFlow,
  Controls,
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

interface RelayFlowDiagramProps {
  pollingInterval?: number;
  className?: string;
}

function RelayFlowDiagramInner({ pollingInterval = 2000, className = '' }: RelayFlowDiagramProps) {
  const { data, isLoading, error, refetch } = useRelayFlowData({
    pollingInterval,
    enabled: true,
  });

  const { fitView, getNodes } = useReactFlow();

  // useNodesInitialized returns true when React Flow has measured all node dimensions
  const nodesInitialized = useNodesInitialized();

  // Track if this is the initial load (for fitView and layout)
  const isInitialLoad = useRef(true);
  // Track if we've applied initial layout (force clean positions on mount)
  const hasAppliedInitialLayout = useRef(false);
  // Track if we've done the measured layout pass
  const hasDoneMeasuredLayout = useRef(false);
  // Track known node IDs to detect new nodes
  const knownNodeIds = useRef<Set<string>>(new Set());
  // Track manually moved nodes (don't auto-position these)
  const manuallyMovedNodes = useRef<Set<string>>(new Set());

  // Convert flow graph data to React Flow format
  const { nodes: rawNodes, edges } = useMemo(() => {
    if (!data?.nodes || !data?.edges) {
      return { nodes: [] as Node[], edges: [] as Edge[] };
    }

    const nodes: Node[] = data.nodes.map((node) => ({
      id: node.id,
      type: node.type,
      position: { x: 0, y: 0 }, // Will be calculated by layout
      data: node.data as unknown as Record<string, unknown>,
      parentId: node.parentId,
    }));

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

  // Calculate layout for new nodes only
  const layoutedNodes = useMemo((): Node[] => {
    if (rawNodes.length === 0) return [];
    return calculateLayout(rawNodes, edges);
  }, [rawNodes, edges]);

  const [nodesState, setNodes, onNodesChange] = useNodesState(layoutedNodes);
  const [edgesState, setEdges, onEdgesChange] = useEdgesState(edges);

  // Custom node change handler that tracks manual moves
  const handleNodesChange = useCallback(
    (changes: NodeChange[]) => {
      for (const change of changes) {
        // Track nodes that the user has manually dragged
        if (change.type === 'position' && change.dragging === false && change.id) {
          manuallyMovedNodes.current.add(change.id);
        }
      }
      onNodesChange(changes);
    },
    [onNodesChange]
  );

  // Update nodes and edges when data changes
  // On initial load: use layout positions for all nodes (clean slate)
  // On subsequent updates: only preserve positions for manually-dragged nodes
  useEffect(() => {
    if (layoutedNodes.length === 0) return;

    // On first mount, force clean layout and clear any stale tracking
    if (!hasAppliedInitialLayout.current) {
      hasAppliedInitialLayout.current = true;
      hasDoneMeasuredLayout.current = false; // Reset for measured pass
      knownNodeIds.current.clear();
      manuallyMovedNodes.current.clear();

      // Add all current nodes to known set
      for (const node of layoutedNodes) {
        knownNodeIds.current.add(node.id);
      }

      // Use layout positions directly - no position preservation on initial load
      setNodes(layoutedNodes);
      setEdges(edges);

      // Fit view after initial layout
      setTimeout(() => {
        fitView({ padding: 0.1, duration: 200 });
      }, 100);
      return;
    }

    setNodes((currentNodes) => {
      // Build a map of current node positions
      const currentPositions = new Map<string, { x: number; y: number }>();
      for (const node of currentNodes) {
        currentPositions.set(node.id, node.position);
      }

      // Identify new nodes (not seen before)
      const newNodeIds = new Set<string>();
      for (const node of layoutedNodes) {
        if (!knownNodeIds.current.has(node.id)) {
          newNodeIds.add(node.id);
          knownNodeIds.current.add(node.id);
          // New nodes need measurement
          hasDoneMeasuredLayout.current = false;
        }
      }

      // Clean up known nodes that no longer exist
      const currentNodeIds = new Set(layoutedNodes.map((n) => n.id));
      for (const id of knownNodeIds.current) {
        if (!currentNodeIds.has(id)) {
          knownNodeIds.current.delete(id);
          manuallyMovedNodes.current.delete(id);
        }
      }

      // Merge: only preserve positions for nodes the user manually dragged
      // All other nodes use the calculated layout positions
      return layoutedNodes.map((layoutedNode) => {
        const existingPosition = currentPositions.get(layoutedNode.id);
        const wasManuallyMoved = manuallyMovedNodes.current.has(layoutedNode.id);

        // Only preserve position if user manually dragged this specific node
        const shouldUseExistingPosition = wasManuallyMoved && existingPosition;

        return {
          ...layoutedNode,
          position: shouldUseExistingPosition ? existingPosition : layoutedNode.position,
        };
      });
    });

    setEdges(edges);
  }, [layoutedNodes, edges, setNodes, setEdges, fitView]);

  // Re-layout with measured dimensions after React Flow measures the nodes
  // This is the second pass that uses actual DOM dimensions for accurate spacing
  useEffect(() => {
    if (!nodesInitialized || hasDoneMeasuredLayout.current) return;
    if (nodesState.length === 0) return;

    // Get nodes with their measured dimensions from React Flow
    const measuredNodes = getNodes();
    if (measuredNodes.length === 0) return;

    // Check if any nodes have been measured
    const hasMeasuredNodes = measuredNodes.some((n) => n.measured?.width || n.measured?.height);
    if (!hasMeasuredNodes) return;

    hasDoneMeasuredLayout.current = true;

    // Re-calculate layout using measured dimensions
    const reLayoutedNodes = calculateLayout(measuredNodes, edges);

    // Apply the new layout, respecting manually moved nodes
    setNodes((currentNodes) => {
      return reLayoutedNodes.map((layoutedNode) => {
        const wasManuallyMoved = manuallyMovedNodes.current.has(layoutedNode.id);
        const currentNode = currentNodes.find((n) => n.id === layoutedNode.id);

        if (wasManuallyMoved && currentNode) {
          return { ...layoutedNode, position: currentNode.position };
        }
        return layoutedNode;
      });
    });

    // Fit view after measured layout
    setTimeout(() => {
      fitView({ padding: 0.1, duration: 200 });
    }, 50);
  }, [nodesInitialized, nodesState.length, edges, getNodes, setNodes, fitView]);

  const onInit = useCallback(() => {
    // Fit view on initial load
    if (isInitialLoad.current) {
      setTimeout(() => {
        fitView({ padding: 0.1 });
      }, 100);
    }
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
            <p className="text-sm">No active relay sessions</p>
            <p className="text-xs mt-1">Sessions will appear here when channels are streaming</p>
          </div>
        ) : (
          <div className="h-[400px] w-full border rounded-lg overflow-hidden bg-background">
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
              <Controls showInteractive={false} />
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
