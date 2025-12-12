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
type FlowBuilder struct {
	// Layout configuration
	originX         float64
	processorX      float64
	clientX         float64
	verticalStart   float64
	verticalSpacing float64
	clientSpacing   float64
}

// NewFlowBuilder creates a new flow builder with default layout settings.
func NewFlowBuilder() *FlowBuilder {
	// Node widths (Tailwind): Origin=w-64(256px), Buffer=w-56(224px), Processor=w-64(256px), Client=w-48(192px)
	// With increased gaps for better visual separation:
	// Origin: 50 to 306 (50 + 256)
	// Buffer: 426 to 650 (306 + 120 = 426, 426 + 224 = 650)
	// Processor: 850 to 1106 (650 + 200 = 850, 850 + 256 = 1106) - increased gap from buffer
	// Client: 1206 to 1398 (1106 + 100 = 1206, 1206 + 192 = 1398) - increased gap from processor
	return &FlowBuilder{
		originX:         50,
		processorX:      850,  // Moved further from buffer (was 770)
		clientX:         1206, // Adjusted for new processor position
		verticalStart:   80,
		verticalSpacing: 400,  // Increased for more space between sessions
		clientSpacing:   160,  // Vertical spacing between client nodes
	}
}

// processorSpacing is the minimum vertical gap between processor groups.
const processorSpacing = 200

// clientNodeHeight is the approximate height of a client node for layout calculations.
const clientNodeHeight = 140

// processorNodeHeight is the approximate height of a processor node for layout calculations.
const processorNodeHeight = 160

// bufferX is the X position for buffer nodes (between origin and processor)
// Origin ends at 306 (50 + 256), so buffer starts at 426 (120px gap)
const bufferX = 426

// transcoderYOffset is how far above the main flow line transcoders are placed.
// This should account for the FFmpeg node height (~220px with speed dial) plus a gap (~60px).
// The transcoder's bottom edge should be at least 60px above the buffer's top edge.
const transcoderYOffset = -280

// transcoderSpacing is horizontal spacing between multiple transcoders
const transcoderSpacing = 180

// BuildFlowGraph builds a complete flow graph from session information.
func (b *FlowBuilder) BuildFlowGraph(sessions []RelaySessionInfo) RelayFlowGraph {
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

	for i, session := range sessions {
		yOffset := b.verticalStart + float64(i)*b.verticalSpacing

		// Create origin node
		originNode := b.buildOriginNode(session, yOffset)
		graph.Nodes = append(graph.Nodes, originNode)

		// Create buffer node (between origin and processor)
		bufferNode := b.buildBufferNode(session, yOffset)
		graph.Nodes = append(graph.Nodes, bufferNode)

		// Create edge from origin to buffer
		// Frontend determines handle IDs based on node types
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
		// Transcoders appear above the buffer node
		if session.RouteType == RouteTypeTranscode && session.FFmpegStats != nil {
			// Position transcoder above the buffer
			transcoderY := yOffset + transcoderYOffset
			transcoderNode := b.buildTranscoderNode(session, transcoderY)
			graph.Nodes = append(graph.Nodes, transcoderNode)

			// Create bidirectional edges: buffer <-> transcoder
			// Frontend determines handle IDs based on node types
			bufferToTranscoder := b.buildEdge(
				bufferNode.ID,
				transcoderNode.ID,
				session.IngressRateBps,
				session.VideoCodec,
				session.AudioCodec,
				"es", // Elementary stream from buffer
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

		// Determine which processors to show:
		// 1. Use ActiveProcessorFormats if available (actual running processors)
		// 2. Fall back to client formats if no active processors reported
		// 3. Fall back to session's output format as last resort
		activeFormats := session.ActiveProcessorFormats
		if len(activeFormats) == 0 {
			// No active processors reported - infer from clients or output format
			for format := range clientsByFormat {
				activeFormats = append(activeFormats, format)
			}
			if len(activeFormats) == 0 && session.OutputFormat != "" {
				activeFormats = []string{session.OutputFormat}
			}
		}

		// Calculate vertical space needed for each processor and its clients.
		// Each processor-client group needs space for:
		// - The processor node itself
		// - All clients connected to that processor (spread vertically around the processor)
		// We need to ensure groups don't overlap.

		type processorGroup struct {
			format     string
			clients    []RelayClientInfo
			height     float64 // Total vertical space needed
			processorY float64 // Calculated Y position for processor
		}

		groups := make([]processorGroup, 0, len(activeFormats))
		for _, format := range activeFormats {
			clients := clientsByFormat[format]
			numClients := len(clients)

			// Calculate the height needed for this group
			// If multiple clients, they spread around the processor
			var groupHeight float64
			if numClients <= 1 {
				// Single client or no clients: just need processor height
				groupHeight = processorNodeHeight
			} else {
				// Multiple clients spread vertically
				// Total span = (numClients - 1) * clientSpacing
				// But we also need to account for the top and bottom client heights
				groupHeight = float64(numClients-1)*b.clientSpacing + clientNodeHeight
			}

			groups = append(groups, processorGroup{
				format:  format,
				clients: clients,
				height:  groupHeight,
			})
		}

		// Calculate total height needed and starting Y position
		totalHeight := 0.0
		for i, g := range groups {
			totalHeight += g.height
			if i < len(groups)-1 {
				totalHeight += processorSpacing // Gap between groups
			}
		}

		// Center the entire processor/client layout around yOffset
		currentY := yOffset - totalHeight/2

		// Position each group
		for i := range groups {
			// The processor Y is at the center of this group's client spread
			numClients := len(groups[i].clients)
			if numClients <= 1 {
				groups[i].processorY = currentY + processorNodeHeight/2
			} else {
				// Processor at center of client spread
				groups[i].processorY = currentY + groups[i].height/2
			}
			currentY += groups[i].height
			if i < len(groups)-1 {
				currentY += processorSpacing
			}
		}

		// Now create nodes and edges for each group
		for _, group := range groups {
			format := group.format
			processorY := group.processorY

			processorNode := b.buildProcessorNodeForFormat(session, processorY, format)
			graph.Nodes = append(graph.Nodes, processorNode)

			// Create edge from buffer to this processor
			// For transcode sessions, the data going to processors uses target codecs
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
				session.EgressRateBps/uint64(max(len(groups), 1)), // Divide egress among formats
				edgeVideoCodec,
				edgeAudioCodec,
				format,
			)
			graph.Edges = append(graph.Edges, bufferToProcessor)

			// Create client nodes connected to this processor
			numClients := len(group.clients)
			for j, client := range group.clients {
				// Position clients centered around their processor
				clientY := processorY
				if numClients > 1 {
					clientY = processorY - float64(numClients-1)*b.clientSpacing/2 + float64(j)*b.clientSpacing
				}
				clientNode := b.buildClientNode(session, client, clientY)
				graph.Nodes = append(graph.Nodes, clientNode)

				// Calculate per-client egress rate
				var clientEgressBps uint64
				if client.ConnectedSecs > 0 && client.BytesRead > 0 {
					clientEgressBps = uint64(float64(client.BytesRead) / client.ConnectedSecs)
				} else if session.EgressRateBps > 0 && numClients > 0 {
					clientEgressBps = session.EgressRateBps / uint64(len(session.Clients))
				}

				// Create edge from processor to client with correct format and codecs
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
	// Get CPU usage (non-blocking, returns immediately with last sample)
	if cpuPercent, err := cpu.Percent(0, false); err == nil && len(cpuPercent) > 0 {
		metadata.SystemCPUPercent = cpuPercent[0]
	}

	// Get memory usage
	if memStats, err := mem.VirtualMemory(); err == nil {
		metadata.SystemMemoryPercent = memStats.UsedPercent
		metadata.SystemMemoryUsedMB = memStats.Used / (1024 * 1024)
		metadata.SystemMemoryTotalMB = memStats.Total / (1024 * 1024)
	}
}

// buildOriginNode creates an origin node for a session.
// groupClientsByFormat groups clients by their output format.
// Returns a map of format -> clients. If client has no format set, uses session's OutputFormat.
func (b *FlowBuilder) groupClientsByFormat(clients []RelayClientInfo) map[string][]RelayClientInfo {
	result := make(map[string][]RelayClientInfo)
	for _, client := range clients {
		format := client.ClientFormat
		if format == "" {
			// Skip clients without format - they'll be handled by the default processor
			continue
		}
		result[format] = append(result[format], client)
	}
	return result
}

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
			Framerate:    session.Framerate,
			VideoWidth:   session.VideoWidth,
			VideoHeight:  session.VideoHeight,
			IngressBps:   session.IngressRateBps,
			TotalBytesIn: session.BytesIn,
			DurationSecs: session.DurationSecs,
		},
	}
}

// buildBufferNode creates a shared buffer node for a session.
func (b *FlowBuilder) buildBufferNode(session RelaySessionInfo, yOffset float64) RelayFlowNode {
	data := FlowNodeData{
		Label:       "Buffer",
		SessionID:   session.SessionID,
		ChannelID:   session.ChannelID,
		ChannelName: session.ChannelName,
	}

	// Add buffer variant information if available
	if len(session.BufferVariants) > 0 {
		data.BufferVariants = session.BufferVariants
		// Calculate total memory and max from all variants
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
		// Calculate utilization percentage
		if totalMaxBytes > 0 {
			data.BufferUtilization = float64(totalMemory) / float64(totalMaxBytes) * 100
		}
	}

	return RelayFlowNode{
		ID:   fmt.Sprintf("buffer-%s", session.SessionID),
		Type: FlowNodeTypeBuffer,
		Position: FlowPosition{
			X: bufferX,
			Y: yOffset,
		},
		Data: data,
	}
}

// buildTranscoderNode creates an FFmpeg transcoder node for a session.
func (b *FlowBuilder) buildTranscoderNode(session RelaySessionInfo, yOffset float64) RelayFlowNode {
	data := FlowNodeData{
		Label:       "FFmpeg",
		SessionID:   session.SessionID,
		ChannelID:   session.ChannelID,
		ChannelName: session.ChannelName,
	}

	// Add transcoder stats if available
	if session.FFmpegStats != nil {
		data.TranscoderCPU = session.CPUPercent
		if session.MemoryBytes != nil {
			memMB := float64(*session.MemoryBytes) / (1024 * 1024)
			data.TranscoderMemMB = &memMB
		}
		// Add bytes processed
		data.TranscoderBytesIn = session.FFmpegStats.BytesWritten

		// Add resource history for sparklines
		data.TranscoderCPUHistory = session.CPUHistory
		data.TranscoderMemHistory = session.MemoryHistory

		// Add encoding speed
		if session.FFmpegStats.EncodingSpeed > 0 {
			data.EncodingSpeed = &session.FFmpegStats.EncodingSpeed
		} else if session.EncodingSpeed != nil && *session.EncodingSpeed > 0 {
			data.EncodingSpeed = session.EncodingSpeed
		}
	}

	// Source codecs - the original input codec (e.g., "h264", "aac")
	data.SourceVideoCodec = session.VideoCodec
	data.SourceAudioCodec = session.AudioCodec

	// Target codecs - the codec names (e.g., "h265", "aac")
	// These are what the stream IS after transcoding
	if session.TargetVideoCodec != "" {
		data.TargetVideoCodec = session.TargetVideoCodec
	} else {
		data.TargetVideoCodec = session.VideoCodec // Fallback to source if not set
	}
	if session.TargetAudioCodec != "" {
		data.TargetAudioCodec = session.TargetAudioCodec
	} else {
		data.TargetAudioCodec = session.AudioCodec // Fallback to source if not set
	}

	// Encoder names - the FFmpeg encoders used (e.g., "libx265", "h264_nvenc")
	// These show what FFmpeg uses to produce the codec
	data.VideoEncoder = session.VideoEncoder
	data.AudioEncoder = session.AudioEncoder

	// Hardware acceleration info
	data.HWAccelType = session.HWAccelType
	data.HWAccelDevice = session.HWAccelDevice

	return RelayFlowNode{
		ID:   fmt.Sprintf("transcoder-%s", session.SessionID),
		Type: FlowNodeTypeTranscoder,
		Position: FlowPosition{
			X: bufferX, // Position above buffer (same X)
			Y: yOffset,
		},
		Data: data,
	}
}

// buildProcessorNode creates a processor node for a session with the session's default output format.
func (b *FlowBuilder) buildProcessorNode(session RelaySessionInfo, yOffset float64, format string) RelayFlowNode {
	return b.buildProcessorNodeForFormat(session, yOffset, format)
}

// buildProcessorNodeForFormat creates a processor node for a specific output format.
func (b *FlowBuilder) buildProcessorNodeForFormat(session RelaySessionInfo, yOffset float64, format string) RelayFlowNode {
	// For transcode sessions, use target codecs; otherwise use source codecs
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

	data := FlowNodeData{
		Label:            b.getProcessorLabelForFormat(format),
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
	}

	return RelayFlowNode{
		ID:   fmt.Sprintf("processor-%s-%s", session.SessionID, format),
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
	// Calculate egress rate from bytes and connection time
	var egressBps uint64
	if client.ConnectedSecs > 0 {
		egressBps = uint64(float64(client.BytesRead) / client.ConnectedSecs)
	}

	// Determine the processor ID this client is connected to
	clientFormat := client.ClientFormat
	if clientFormat == "" {
		clientFormat = session.OutputFormat
	}

	return RelayFlowNode{
		ID:   fmt.Sprintf("client-%s-%s", session.SessionID, client.ClientID),
		Type: FlowNodeTypeClient,
		Position: FlowPosition{
			X: b.clientX,
			Y: yOffset,
		},
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

// buildEdge creates an edge between two nodes.
func (b *FlowBuilder) buildEdge(sourceID, targetID string, bandwidthBps uint64, videoCodec, audioCodec, format string) RelayFlowEdge {
	return b.buildEdgeWithHandles(sourceID, targetID, "", "", bandwidthBps, videoCodec, audioCodec, format)
}

// buildEdgeWithHandles creates an edge between two nodes with specific handle IDs.
func (b *FlowBuilder) buildEdgeWithHandles(sourceID, targetID, sourceHandle, targetHandle string, bandwidthBps uint64, videoCodec, audioCodec, format string) RelayFlowEdge {
	edgeID := fmt.Sprintf("edge-%s-%s", sourceID, targetID)
	if sourceHandle != "" || targetHandle != "" {
		edgeID = fmt.Sprintf("edge-%s-%s-%s-%s", sourceID, sourceHandle, targetID, targetHandle)
	}
	return RelayFlowEdge{
		ID:           edgeID,
		Source:       sourceID,
		Target:       targetID,
		SourceHandle: sourceHandle,
		TargetHandle: targetHandle,
		Type:         "animated",
		Animated:     bandwidthBps > 0,
		Data: FlowEdgeData{
			BandwidthBps: bandwidthBps,
			VideoCodec:   videoCodec,
			AudioCodec:   audioCodec,
			Format:       format,
		},
	}
}

// getProcessorLabel returns a descriptive label for the processor node.
// Shows the output format type (HLS, DASH, MPEG-TS, etc.)
func (b *FlowBuilder) getProcessorLabel(session RelaySessionInfo) string {
	format := session.OutputFormat
	if format == "" {
		format = session.SourceFormat
	}
	return b.getProcessorLabelForFormat(format)
}

// getProcessorLabelForFormat returns a descriptive label for a specific format.
func (b *FlowBuilder) getProcessorLabelForFormat(format string) string {
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
