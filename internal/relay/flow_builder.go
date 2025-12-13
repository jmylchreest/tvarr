// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"fmt"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

// FlowBuilder builds a flow graph from relay session information.
// Note: Node positioning is handled entirely by the frontend using React Flow's
// measured dimensions. The backend only provides graph structure (nodes, edges, data).
type FlowBuilder struct{}

// NewFlowBuilder creates a new flow builder.
func NewFlowBuilder() *FlowBuilder {
	return &FlowBuilder{}
}

// BuildFlowGraph builds a complete flow graph from session information.
// Positions are set to zero - the frontend calculates actual layout using
// measured node dimensions.
func (b *FlowBuilder) BuildFlowGraph(sessions []RelaySessionInfo) RelayFlowGraph {
	return b.BuildFlowGraphWithPassthrough(sessions, nil)
}

// BuildFlowGraphWithPassthrough builds a complete flow graph from session and passthrough information.
// Positions are set to zero - the frontend calculates actual layout using
// measured node dimensions.
func (b *FlowBuilder) BuildFlowGraphWithPassthrough(sessions []RelaySessionInfo, passthroughConns []*PassthroughConnection) RelayFlowGraph {
	graph := RelayFlowGraph{
		Nodes: make([]RelayFlowNode, 0),
		Edges: make([]RelayFlowEdge, 0),
		Metadata: FlowGraphMetadata{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}

	// Collect system stats
	b.collectSystemStats(&graph.Metadata)

	var totalIngressBps, totalEgressBps uint64

	// Build passthrough connection nodes first
	for _, conn := range passthroughConns {
		passthroughNode := b.buildPassthroughNode(conn)
		graph.Nodes = append(graph.Nodes, passthroughNode)

		// Passthrough counts as both a session and a client
		graph.Metadata.TotalSessions++
		graph.Metadata.TotalClients++
		totalIngressBps += conn.IngressBps()
		totalEgressBps += conn.EgressBps()
	}

	for _, session := range sessions {
		// Create origin node
		originNode := b.buildOriginNode(session)
		graph.Nodes = append(graph.Nodes, originNode)

		// Create buffer node
		bufferNode := b.buildBufferNode(session)
		graph.Nodes = append(graph.Nodes, bufferNode)

		// Create edge from origin to buffer
		originToBuffer := b.buildEdge(
			originNode.ID,
			bufferNode.ID,
			session.IngressRateBps,
			session.VideoCodec,
			session.AudioCodec,
			session.SourceFormat,
		)
		graph.Edges = append(graph.Edges, originToBuffer)

		// Check if there's a transcoder (for transcode mode)
		if session.RouteType == RouteTypeTranscode && session.FFmpegStats != nil {
			transcoderNode := b.buildTranscoderNode(session)
			graph.Nodes = append(graph.Nodes, transcoderNode)

			// Create bidirectional edges: buffer <-> transcoder
			bufferToTranscoder := b.buildEdge(
				bufferNode.ID,
				transcoderNode.ID,
				session.IngressRateBps,
				session.VideoCodec,
				session.AudioCodec,
				"es",
			)
			graph.Edges = append(graph.Edges, bufferToTranscoder)

			// Transcoder back to buffer (transcoded data)
			targetVideoCodec := session.VideoCodec
			targetAudioCodec := session.AudioCodec
			if session.TargetVideoCodec != "" {
				targetVideoCodec = session.TargetVideoCodec
			}
			if session.TargetAudioCodec != "" {
				targetAudioCodec = session.TargetAudioCodec
			}

			transcoderToBuffer := b.buildEdge(
				transcoderNode.ID,
				bufferNode.ID,
				session.EgressRateBps,
				targetVideoCodec,
				targetAudioCodec,
				"es",
			)
			graph.Edges = append(graph.Edges, transcoderToBuffer)
		}

		// Group clients by their format for connecting to the right processor
		clientsByFormat := b.groupClientsByFormat(session.Clients)

		// Determine which processors to show
		activeFormats := session.ActiveProcessorFormats
		if len(activeFormats) == 0 {
			for format := range clientsByFormat {
				activeFormats = append(activeFormats, format)
			}
			if len(activeFormats) == 0 && session.OutputFormat != "" {
				activeFormats = []string{session.OutputFormat}
			}
		}

		// Create processor and client nodes
		for _, format := range activeFormats {
			processorNode := b.buildProcessorNode(session, format)
			graph.Nodes = append(graph.Nodes, processorNode)

			// Create edge from buffer to processor
			edgeVideoCodec := session.VideoCodec
			edgeAudioCodec := session.AudioCodec
			if session.RouteType == RouteTypeTranscode {
				if session.TargetVideoCodec != "" {
					edgeVideoCodec = session.TargetVideoCodec
				}
				if session.TargetAudioCodec != "" {
					edgeAudioCodec = session.TargetAudioCodec
				}
			}

			bufferToProcessor := b.buildEdge(
				bufferNode.ID,
				processorNode.ID,
				session.EgressRateBps/uint64(max(len(activeFormats), 1)),
				edgeVideoCodec,
				edgeAudioCodec,
				format,
			)
			graph.Edges = append(graph.Edges, bufferToProcessor)

			// Create client nodes connected to this processor
			clients := clientsByFormat[format]
			for _, client := range clients {
				clientNode := b.buildClientNode(session, client, format)
				graph.Nodes = append(graph.Nodes, clientNode)

				// Calculate per-client egress rate
				var clientEgressBps uint64
				if client.ConnectedSecs > 0 && client.BytesRead > 0 {
					clientEgressBps = uint64(float64(client.BytesRead) / client.ConnectedSecs)
				} else if session.EgressRateBps > 0 && len(session.Clients) > 0 {
					clientEgressBps = session.EgressRateBps / uint64(len(session.Clients))
				}

				processorToClient := b.buildEdge(
					processorNode.ID,
					clientNode.ID,
					clientEgressBps,
					edgeVideoCodec,
					edgeAudioCodec,
					format,
				)
				graph.Edges = append(graph.Edges, processorToClient)
			}
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

// collectSystemStats gathers system CPU and memory information.
func (b *FlowBuilder) collectSystemStats(metadata *FlowGraphMetadata) {
	if cpuPercent, err := cpu.Percent(0, false); err == nil && len(cpuPercent) > 0 {
		metadata.SystemCPUPercent = cpuPercent[0]
	}

	if memStats, err := mem.VirtualMemory(); err == nil {
		metadata.SystemMemoryPercent = memStats.UsedPercent
		metadata.SystemMemoryUsedMB = memStats.Used / (1024 * 1024)
		metadata.SystemMemoryTotalMB = memStats.Total / (1024 * 1024)
	}
}

// groupClientsByFormat groups clients by their output format.
func (b *FlowBuilder) groupClientsByFormat(clients []RelayClientInfo) map[string][]RelayClientInfo {
	result := make(map[string][]RelayClientInfo)
	for _, client := range clients {
		format := client.ClientFormat
		if format == "" {
			continue
		}
		result[format] = append(result[format], client)
	}
	return result
}

func (b *FlowBuilder) buildOriginNode(session RelaySessionInfo) RelayFlowNode {
	label := session.StreamSourceName
	if label == "" {
		label = truncateURL(session.SourceURL, 40)
	}

	return RelayFlowNode{
		ID:       fmt.Sprintf("origin-%s", session.SessionID),
		Type:     FlowNodeTypeOrigin,
		Position: FlowPosition{X: 0, Y: 0}, // Frontend calculates layout
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
			Framerate:    session.Framerate,
			VideoWidth:   session.VideoWidth,
			VideoHeight:  session.VideoHeight,
			IngressBps:   session.IngressRateBps,
			TotalBytesIn: session.BytesIn,
			DurationSecs: session.DurationSecs,
		},
	}
}

func (b *FlowBuilder) buildBufferNode(session RelaySessionInfo) RelayFlowNode {
	data := FlowNodeData{
		Label:       "Buffer",
		SessionID:   session.SessionID,
		ChannelID:   session.ChannelID,
		ChannelName: session.ChannelName,
	}

	if len(session.BufferVariants) > 0 {
		data.BufferVariants = session.BufferVariants
		var totalMemory, totalMaxBytes uint64
		var totalVideoSamples, totalAudioSamples int
		for _, v := range session.BufferVariants {
			totalMemory += v.BytesIngested
			totalMaxBytes += v.MaxBytes
			totalVideoSamples += v.VideoSamples
			totalAudioSamples += v.AudioSamples
		}
		data.BufferMemoryBytes = totalMemory
		data.MaxBufferBytes = totalMaxBytes
		data.VideoSampleCount = totalVideoSamples
		data.AudioSampleCount = totalAudioSamples
		if totalMaxBytes > 0 {
			data.BufferUtilization = float64(totalMemory) / float64(totalMaxBytes) * 100
		}
	}

	return RelayFlowNode{
		ID:       fmt.Sprintf("buffer-%s", session.SessionID),
		Type:     FlowNodeTypeBuffer,
		Position: FlowPosition{X: 0, Y: 0}, // Frontend calculates layout
		Data:     data,
	}
}

func (b *FlowBuilder) buildTranscoderNode(session RelaySessionInfo) RelayFlowNode {
	data := FlowNodeData{
		Label:       "FFmpeg",
		SessionID:   session.SessionID,
		ChannelID:   session.ChannelID,
		ChannelName: session.ChannelName,
	}

	if session.FFmpegStats != nil {
		data.TranscoderCPU = session.CPUPercent
		if session.MemoryBytes != nil {
			memMB := float64(*session.MemoryBytes) / (1024 * 1024)
			data.TranscoderMemMB = &memMB
		}
		data.TranscoderBytesIn = session.FFmpegStats.BytesWritten
		data.TranscoderCPUHistory = session.CPUHistory
		data.TranscoderMemHistory = session.MemoryHistory

		if session.FFmpegStats.EncodingSpeed > 0 {
			data.EncodingSpeed = &session.FFmpegStats.EncodingSpeed
		} else if session.EncodingSpeed != nil && *session.EncodingSpeed > 0 {
			data.EncodingSpeed = session.EncodingSpeed
		}
	}

	data.SourceVideoCodec = session.VideoCodec
	data.SourceAudioCodec = session.AudioCodec

	if session.TargetVideoCodec != "" {
		data.TargetVideoCodec = session.TargetVideoCodec
	} else {
		data.TargetVideoCodec = session.VideoCodec
	}
	if session.TargetAudioCodec != "" {
		data.TargetAudioCodec = session.TargetAudioCodec
	} else {
		data.TargetAudioCodec = session.AudioCodec
	}

	data.VideoEncoder = session.VideoEncoder
	data.AudioEncoder = session.AudioEncoder
	data.HWAccelType = session.HWAccelType
	data.HWAccelDevice = session.HWAccelDevice

	return RelayFlowNode{
		ID:       fmt.Sprintf("transcoder-%s", session.SessionID),
		Type:     FlowNodeTypeTranscoder,
		Position: FlowPosition{X: 0, Y: 0}, // Frontend calculates layout
		Data:     data,
	}
}

func (b *FlowBuilder) buildProcessorNode(session RelaySessionInfo, format string) RelayFlowNode {
	outputVideoCodec := session.VideoCodec
	outputAudioCodec := session.AudioCodec
	if session.RouteType == RouteTypeTranscode {
		if session.TargetVideoCodec != "" {
			outputVideoCodec = session.TargetVideoCodec
		}
		if session.TargetAudioCodec != "" {
			outputAudioCodec = session.TargetAudioCodec
		}
	}

	return RelayFlowNode{
		ID:       fmt.Sprintf("processor-%s-%s", session.SessionID, format),
		Type:     FlowNodeTypeProcessor,
		Position: FlowPosition{X: 0, Y: 0}, // Frontend calculates layout
		Data: FlowNodeData{
			Label:            b.getProcessorLabel(format),
			SessionID:        session.SessionID,
			ChannelID:        session.ChannelID,
			ChannelName:      session.ChannelName,
			RouteType:        session.RouteType,
			ProfileName:      session.ProfileName,
			OutputFormat:     format,
			OutputVideoCodec: outputVideoCodec,
			OutputAudioCodec: outputAudioCodec,
			ProcessingBps:    session.EgressRateBps,
			TotalBytesOut:    session.BytesOut,
			InFallback:       session.InFallback,
			Error:            session.Error,
		},
	}
}

func (b *FlowBuilder) buildClientNode(session RelaySessionInfo, client RelayClientInfo, format string) RelayFlowNode {
	var egressBps uint64
	if client.ConnectedSecs > 0 {
		egressBps = uint64(float64(client.BytesRead) / client.ConnectedSecs)
	}

	clientFormat := client.ClientFormat
	if clientFormat == "" {
		clientFormat = session.OutputFormat
	}

	return RelayFlowNode{
		ID:       fmt.Sprintf("client-%s-%s", session.SessionID, client.ClientID),
		Type:     FlowNodeTypeClient,
		Position: FlowPosition{X: 0, Y: 0}, // Frontend calculates layout
		Data: FlowNodeData{
			Label:         b.getClientLabel(client),
			SessionID:     session.SessionID,
			ClientID:      client.ClientID,
			PlayerType:    client.PlayerType,
			ClientFormat:  client.ClientFormat,
			RemoteAddr:    client.RemoteAddr,
			UserAgent:     client.UserAgent,
			DetectionRule: client.DetectionRule,
			BytesRead:     client.BytesRead,
			EgressBps:     egressBps,
			ConnectedSecs: client.ConnectedSecs,
		},
		ParentID: fmt.Sprintf("processor-%s-%s", session.SessionID, clientFormat),
	}
}

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

func (b *FlowBuilder) getProcessorLabel(format string) string {
	switch format {
	case "hls":
		return "HLS"
	case "dash":
		return "DASH"
	case "mpegts":
		return "MPEG-TS"
	case "fmp4":
		return "fMP4"
	default:
		if format != "" {
			return strings.ToUpper(format)
		}
		return "Output"
	}
}

func (b *FlowBuilder) getClientLabel(client RelayClientInfo) string {
	if client.PlayerType != "" {
		return client.PlayerType
	}
	if client.RemoteAddr != "" {
		return truncateString(client.RemoteAddr, 20)
	}
	return "Client"
}

func truncateURL(url string, maxLen int) string {
	if len(url) <= maxLen {
		return url
	}
	return url[:maxLen-3] + "..."
}

// buildPassthroughNode creates a node for a passthrough connection.
// Passthrough connections are direct HTTP proxies without transcoding/buffering.
func (b *FlowBuilder) buildPassthroughNode(conn *PassthroughConnection) RelayFlowNode {
	// Extract a readable label from the channel name or remote addr
	label := conn.ChannelName
	if label == "" {
		label = truncateString(conn.RemoteAddr, 20)
	}

	return RelayFlowNode{
		ID:       fmt.Sprintf("passthrough-%s", conn.ID),
		Type:     FlowNodeTypePassthrough,
		Position: FlowPosition{X: 0, Y: 0}, // Frontend calculates layout
		Data: FlowNodeData{
			Label:          label,
			ChannelID:      conn.ChannelID.String(),
			ChannelName:    conn.ChannelName,
			SourceURL:      truncateURL(conn.StreamURL, 60),
			SourceFormat:   conn.SourceFormat,
			VideoCodec:     conn.VideoCodec,
			AudioCodec:     conn.AudioCodec,
			RemoteAddr:     conn.RemoteAddr,
			UserAgent:      conn.UserAgent,
			IngressBps:     conn.IngressBps(),
			EgressBps:      conn.EgressBps(),
			TotalBytesIn:   conn.BytesIn(),
			TotalBytesOut:  conn.BytesOut(),
			DurationSecs:   conn.DurationSecs(),
			IngressHistory: conn.IngressHistory(),
			EgressHistory:  conn.EgressHistory(),
			RouteType:      RouteTypePassthrough,
			OutputFormat:   conn.SourceFormat,
		},
	}
}
