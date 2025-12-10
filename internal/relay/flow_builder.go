// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"fmt"
	"time"
)

// FlowBuilder builds a flow graph from relay session information.
type FlowBuilder struct {
	// Layout configuration
	originX        float64
	processorX     float64
	clientX        float64
	verticalStart  float64
	verticalSpacing float64
	clientSpacing  float64
}

// NewFlowBuilder creates a new flow builder with default layout settings.
func NewFlowBuilder() *FlowBuilder {
	return &FlowBuilder{
		originX:        50,
		processorX:     350,
		clientX:        650,
		verticalStart:  50,
		verticalSpacing: 200,
		clientSpacing:  80,
	}
}

// BuildFlowGraph builds a complete flow graph from session information.
func (b *FlowBuilder) BuildFlowGraph(sessions []RelaySessionInfo) RelayFlowGraph {
	graph := RelayFlowGraph{
		Nodes: make([]RelayFlowNode, 0),
		Edges: make([]RelayFlowEdge, 0),
		Metadata: FlowGraphMetadata{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}

	var totalIngressBps, totalEgressBps uint64

	for i, session := range sessions {
		yOffset := b.verticalStart + float64(i)*b.verticalSpacing

		// Create origin node
		originNode := b.buildOriginNode(session, yOffset)
		graph.Nodes = append(graph.Nodes, originNode)

		// Create processor node
		processorNode := b.buildProcessorNode(session, yOffset)
		graph.Nodes = append(graph.Nodes, processorNode)

		// Create edge from origin to processor
		originToProcessor := b.buildEdge(
			originNode.ID,
			processorNode.ID,
			session.IngressRateBps,
			session.VideoCodec,
			session.AudioCodec,
			session.SourceFormat,
		)
		graph.Edges = append(graph.Edges, originToProcessor)

		// Create client nodes and edges
		for j, client := range session.Clients {
			clientY := yOffset - float64(len(session.Clients)-1)*b.clientSpacing/2 + float64(j)*b.clientSpacing
			clientNode := b.buildClientNode(session, client, clientY)
			graph.Nodes = append(graph.Nodes, clientNode)

			// Create edge from processor to client
			processorToClient := b.buildEdge(
				processorNode.ID,
				clientNode.ID,
				client.BytesRead, // Use bytes read as proxy for rate (API should provide rate)
				session.VideoCodec,
				session.AudioCodec,
				session.OutputFormat,
			)
			graph.Edges = append(graph.Edges, processorToClient)
		}

		// Update metadata
		graph.Metadata.TotalSessions++
		graph.Metadata.TotalClients += session.ClientCount
		totalIngressBps += session.IngressRateBps
		totalEgressBps += session.EgressRateBps
	}

	graph.Metadata.TotalIngressBps = totalIngressBps
	graph.Metadata.TotalEgressBps = totalEgressBps

	return graph
}

// buildOriginNode creates an origin node for a session.
func (b *FlowBuilder) buildOriginNode(session RelaySessionInfo, yOffset float64) RelayFlowNode {
	// Use stream source name as label if available, otherwise fall back to truncated URL
	label := session.StreamSourceName
	if label == "" {
		label = truncateURL(session.SourceURL, 40)
	}

	return RelayFlowNode{
		ID:   fmt.Sprintf("origin-%s", session.SessionID),
		Type: FlowNodeTypeOrigin,
		Position: FlowPosition{
			X: b.originX,
			Y: yOffset,
		},
		Data: FlowNodeData{
			Label:        label,
			SessionID:    session.SessionID,
			ChannelID:    session.ChannelID,
			ChannelName:  session.ChannelName,
			SourceName:   session.StreamSourceName,
			SourceURL:    session.SourceURL,
			SourceFormat: session.SourceFormat,
			VideoCodec:   session.VideoCodec,
			AudioCodec:   session.AudioCodec,
			IngressBps:   session.IngressRateBps,
		},
	}
}

// buildProcessorNode creates a processor node for a session.
func (b *FlowBuilder) buildProcessorNode(session RelaySessionInfo, yOffset float64) RelayFlowNode {
	data := FlowNodeData{
		Label:            b.getProcessorLabel(session),
		SessionID:        session.SessionID,
		ChannelID:        session.ChannelID,
		ChannelName:      session.ChannelName,
		RouteType:        session.RouteType,
		ProfileName:      session.ProfileName,
		OutputFormat:     session.OutputFormat,
		OutputVideoCodec: session.VideoCodec, // For passthrough/repackage, same as input
		OutputAudioCodec: session.AudioCodec, // For passthrough/repackage, same as input
		ProcessingBps:    session.BytesOut,
		InFallback:       session.InFallback,
		Error:            session.Error,
	}

	// Add FFmpeg stats if available (transcode mode)
	if session.CPUPercent != nil {
		data.CPUPercent = session.CPUPercent
	}
	if session.MemoryBytes != nil {
		memMB := float64(*session.MemoryBytes) / (1024 * 1024)
		data.MemoryMB = &memMB
	}

	return RelayFlowNode{
		ID:   fmt.Sprintf("processor-%s", session.SessionID),
		Type: FlowNodeTypeProcessor,
		Position: FlowPosition{
			X: b.processorX,
			Y: yOffset,
		},
		Data: data,
	}
}

// buildClientNode creates a client node.
func (b *FlowBuilder) buildClientNode(session RelaySessionInfo, client RelayClientInfo, yOffset float64) RelayFlowNode {
	return RelayFlowNode{
		ID:   fmt.Sprintf("client-%s-%s", session.SessionID, client.ClientID),
		Type: FlowNodeTypeClient,
		Position: FlowPosition{
			X: b.clientX,
			Y: yOffset,
		},
		Data: FlowNodeData{
			Label:      b.getClientLabel(client),
			SessionID:  session.SessionID,
			ClientID:   client.ClientID,
			PlayerType: client.PlayerType,
			RemoteAddr: client.RemoteAddr,
			UserAgent:  client.UserAgent,
			BytesRead:  client.BytesRead,
		},
		ParentID: fmt.Sprintf("processor-%s", session.SessionID),
	}
}

// buildEdge creates an edge between two nodes.
func (b *FlowBuilder) buildEdge(sourceID, targetID string, bandwidthBps uint64, videoCodec, audioCodec, format string) RelayFlowEdge {
	return RelayFlowEdge{
		ID:       fmt.Sprintf("edge-%s-%s", sourceID, targetID),
		Source:   sourceID,
		Target:   targetID,
		Type:     "animated",
		Animated: bandwidthBps > 0,
		Data: FlowEdgeData{
			BandwidthBps: bandwidthBps,
			VideoCodec:   videoCodec,
			AudioCodec:   audioCodec,
			Format:       format,
		},
	}
}

// getProcessorLabel returns a descriptive label for the processor node.
func (b *FlowBuilder) getProcessorLabel(session RelaySessionInfo) string {
	switch session.RouteType {
	case RouteTypePassthrough:
		return "Passthrough"
	case RouteTypeRepackage:
		return "Repackage"
	case RouteTypeTranscode:
		if session.ProfileName != "" {
			return fmt.Sprintf("FFmpeg (%s)", session.ProfileName)
		}
		return "FFmpeg"
	default:
		return "Processor"
	}
}

// getClientLabel returns a descriptive label for the client node.
func (b *FlowBuilder) getClientLabel(client RelayClientInfo) string {
	if client.PlayerType != "" {
		return client.PlayerType
	}
	if client.RemoteAddr != "" {
		return truncateString(client.RemoteAddr, 20)
	}
	return "Client"
}

// truncateURL shortens a URL for display.
func truncateURL(url string, maxLen int) string {
	if len(url) <= maxLen {
		return url
	}
	// Try to show the domain and end
	if len(url) > maxLen {
		return url[:maxLen-3] + "..."
	}
	return url
}

// truncateString is defined in fallback.go and reused here.
