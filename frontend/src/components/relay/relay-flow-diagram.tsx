'use client';

import { useCallback, useMemo } from 'react';
import {
  ReactFlow,
  Controls,
  Background,
  BackgroundVariant,
  useNodesState,
  useEdgesState,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Loader2, AlertCircle, Activity } from 'lucide-react';
import { useRelayFlowData } from '@/hooks/use-relay-flow-data';
import { formatBps } from '@/types/relay-flow';
import { OriginNode, ProcessorNode, ClientNode } from './nodes';
import { AnimatedEdge } from './edges';

// Define custom node types
const nodeTypes = {
  origin: OriginNode,
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

export function RelayFlowDiagram({
  pollingInterval = 2000,
  className = '',
}: RelayFlowDiagramProps) {
  const { data, isLoading, error, refetch } = useRelayFlowData({
    pollingInterval,
    enabled: true,
  });

  // Convert flow graph data to React Flow format
  // Using 'any' to avoid strict type conflicts with @xyflow/react's Record<string, unknown> constraint
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const nodes = useMemo((): any[] => {
    if (!data?.nodes) return [];
    return data.nodes.map((node) => ({
      id: node.id,
      type: node.type,
      position: node.position,
      data: node.data,
      parentId: node.parentId,
    }));
  }, [data?.nodes]);

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const edges = useMemo((): any[] => {
    if (!data?.edges) return [];
    return data.edges.map((edge) => ({
      id: edge.id,
      source: edge.source,
      target: edge.target,
      type: edge.type || 'animated',
      animated: edge.animated,
      data: edge.data,
      style: edge.style,
    }));
  }, [data?.edges]);

  const [nodesState, setNodes, onNodesChange] = useNodesState(nodes);
  const [edgesState, setEdges, onEdgesChange] = useEdgesState(edges);

  // Update nodes and edges when data changes
  useMemo(() => {
    setNodes(nodes);
    setEdges(edges);
  }, [nodes, edges, setNodes, setEdges]);

  const onInit = useCallback(() => {
    // React Flow initialized
  }, []);

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
          <button
            onClick={refetch}
            className="text-sm text-primary hover:underline"
          >
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
            <div className="flex items-center gap-2">
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
              onNodesChange={onNodesChange}
              onEdgesChange={onEdgesChange}
              onInit={onInit}
              nodeTypes={nodeTypes}
              edgeTypes={edgeTypes}
              fitView
              fitViewOptions={{
                padding: 0.2,
                includeHiddenNodes: false,
              }}
              minZoom={0.5}
              maxZoom={1.5}
              defaultViewport={{ x: 0, y: 0, zoom: 1 }}
              proOptions={{ hideAttribution: true }}
            >
              <Controls />
              <Background variant={BackgroundVariant.Dots} gap={12} size={1} />
            </ReactFlow>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

export default RelayFlowDiagram;
