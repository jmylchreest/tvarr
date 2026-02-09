// Package relay provides streaming relay functionality for tvarr.
package relay

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jmylchreest/tvarr/internal/codec"
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

	for _, session := range sessions {
		// Create origin node
		originNode := b.buildOriginNode(session)
		graph.Nodes = append(graph.Nodes, originNode)

		// Create buffer node
		bufferNode := b.buildBufferNode(session)
		graph.Nodes = append(graph.Nodes, bufferNode)

		// Determine bandwidth for origin -> buffer edge
		// Use real-time tracking if available, otherwise fall back to average rate
		originToBufferBps := session.IngressRateBps
		if session.EdgeBandwidth != nil {
			originToBufferBps = session.EdgeBandwidth.OriginToBuffer.CurrentBps
		}

		// Create edge from origin to buffer
		originToBuffer := b.buildEdge(
			originNode.ID,
			bufferNode.ID,
			originToBufferBps,
			session.VideoCodec,
			session.AudioCodec,
			session.SourceFormat,
		)
		graph.Edges = append(graph.Edges, originToBuffer)

		// Check if there are ES transcoders (for transcode mode with multi-variant support)
		if session.RouteType == RouteTranscode && len(session.ESTranscoders) > 0 {
			// Create a node for each ES transcoder
			for _, esTranscoder := range session.ESTranscoders {
				transcoderNode := b.buildESTranscoderNode(session, esTranscoder)
				graph.Nodes = append(graph.Nodes, transcoderNode)

				// Parse source/target variants for codec info (all normalized for consistent display)
				sourceVideo, sourceAudio := b.resolveVariantCodecs(session, esTranscoder.SourceVariant)
				targetVideo, targetAudio := codec.Normalize(esTranscoder.VideoCodec), codec.Normalize(esTranscoder.AudioCodec)
				if targetVideo == "" {
					targetVideo, _ = b.resolveVariantCodecs(session, esTranscoder.TargetVariant)
				}
				if targetAudio == "" {
					_, targetAudio = b.resolveVariantCodecs(session, esTranscoder.TargetVariant)
				}

				// Calculate bandwidth in bps from total bytes and duration
				var bytesInBps, bytesOutBps uint64
				if !esTranscoder.StartedAt.IsZero() {
					duration := time.Since(esTranscoder.StartedAt).Seconds()
					if duration > 0 {
						bytesInBps = uint64(float64(esTranscoder.BytesIn) / duration)
						bytesOutBps = uint64(float64(esTranscoder.BytesOut) / duration)
					}
				}

				// Create bidirectional edges: buffer <-> transcoder
				bufferToTranscoder := b.buildEdge(
					bufferNode.ID,
					transcoderNode.ID,
					bytesInBps,
					sourceVideo,
					sourceAudio,
					"es",
				)
				graph.Edges = append(graph.Edges, bufferToTranscoder)

				transcoderToBuffer := b.buildEdge(
					transcoderNode.ID,
					bufferNode.ID,
					bytesOutBps,
					targetVideo,
					targetAudio,
					"es",
				)
				graph.Edges = append(graph.Edges, transcoderToBuffer)
			}
		} else if session.RouteType == RouteTranscode && session.FFmpegStats != nil {
			// Fallback to single transcoder node if no ES transcoders
			transcoderNode := b.buildTranscoderNode(session)
			graph.Nodes = append(graph.Nodes, transcoderNode)

			// Determine bandwidth for buffer <-> transcoder edges
			bufferToTranscoderBps := session.IngressRateBps
			transcoderToBufferBps := session.EgressRateBps
			if session.EdgeBandwidth != nil {
				bufferToTranscoderBps = session.EdgeBandwidth.BufferToTranscoder.CurrentBps
				transcoderToBufferBps = session.EdgeBandwidth.TranscoderToBuffer.CurrentBps
			}

			// Create bidirectional edges: buffer <-> transcoder
			bufferToTranscoder := b.buildEdge(
				bufferNode.ID,
				transcoderNode.ID,
				bufferToTranscoderBps,
				session.VideoCodec,
				session.AudioCodec,
				"es",
			)
			graph.Edges = append(graph.Edges, bufferToTranscoder)

			// Transcoder back to buffer (transcoded data)
			targetVideoCodec := session.VideoCodec
			targetAudioCodec := session.AudioCodec
			if session.TargetVideoCodec != "" && session.TargetVideoCodec != "copy" {
				targetVideoCodec = session.TargetVideoCodec
			}
			if session.TargetAudioCodec != "" && session.TargetAudioCodec != "copy" {
				targetAudioCodec = session.TargetAudioCodec
			}

			transcoderToBuffer := b.buildEdge(
				transcoderNode.ID,
				bufferNode.ID,
				transcoderToBufferBps,
				targetVideoCodec,
				targetAudioCodec,
				"es",
			)
			graph.Edges = append(graph.Edges, transcoderToBuffer)
		}

		// Group clients by their format and codec variant
		clientsByKey := b.groupClientsByFormatVariant(session.Clients)

		// Log client variants for debugging
		for _, client := range session.Clients {
			slog.Debug("FlowBuilder: client variant",
				slog.String("session_id", session.SessionID),
				slog.String("client_id", client.ClientID),
				slog.String("format", client.ClientFormat),
				slog.String("variant", client.ClientVariant))
		}

		// Build processor keys from grouped clients
		// This creates one processor per unique format+variant combination
		processorKeys := make([]ProcessorKey, 0, len(clientsByKey))
		for key := range clientsByKey {
			processorKeys = append(processorKeys, key)
			slog.Debug("FlowBuilder: processor key",
				slog.String("session_id", session.SessionID),
				slog.String("format", key.Format),
				slog.String("variant", key.Variant))
		}

		// Fallback: if no clients, use session output format
		if len(processorKeys) == 0 && session.OutputFormat != "" {
			processorKeys = []ProcessorKey{{Format: session.OutputFormat}}
		}

		// Create processor and client nodes for each format+variant combination
		for _, processorKey := range processorKeys {
			processorNode := b.buildProcessorNode(session, processorKey)
			graph.Nodes = append(graph.Nodes, processorNode)

			// Resolve codecs for this processor's variant
			edgeVideoCodec, edgeAudioCodec := b.resolveVariantCodecs(session, processorKey.Variant)
			slog.Debug("FlowBuilder: resolved codecs",
				slog.String("session_id", session.SessionID),
				slog.String("processor_key", processorKey.String()),
				slog.String("variant", processorKey.Variant),
				slog.String("video_codec", edgeVideoCodec),
				slog.String("audio_codec", edgeAudioCodec))

			// Determine bandwidth for buffer -> processor edge
			// Use real-time per-processor tracking if available
			bufferToProcessorBps := session.EgressRateBps / uint64(max(len(processorKeys), 1))
			if session.EdgeBandwidth != nil && session.EdgeBandwidth.BufferToProcessor != nil {
				if processorInfo, ok := session.EdgeBandwidth.BufferToProcessor[processorKey.Format]; ok {
					bufferToProcessorBps = processorInfo.CurrentBps
				}
			}

			bufferToProcessor := b.buildEdge(
				bufferNode.ID,
				processorNode.ID,
				bufferToProcessorBps,
				edgeVideoCodec,
				edgeAudioCodec,
				processorKey.Format,
			)
			graph.Edges = append(graph.Edges, bufferToProcessor)

			// Create client nodes connected to this processor
			clients := clientsByKey[processorKey]
			for _, client := range clients {
				clientNode := b.buildClientNode(session, client, processorKey.Format)
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
					processorKey.Format,
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

// ProcessorKey uniquely identifies a processor by format and codec variant.
type ProcessorKey struct {
	Format  string
	Variant string
}

// String returns a unique string representation for IDs.
func (p ProcessorKey) String() string {
	if p.Variant == "" || p.Variant == "source/source" {
		return p.Format
	}
	return fmt.Sprintf("%s-%s", p.Format, p.Variant)
}

// groupClientsByFormatVariant groups clients by their format and codec variant.
func (b *FlowBuilder) groupClientsByFormatVariant(clients []RelayClientInfo) map[ProcessorKey][]RelayClientInfo {
	result := make(map[ProcessorKey][]RelayClientInfo)
	for _, client := range clients {
		format := client.ClientFormat
		if format == "" {
			continue
		}
		key := ProcessorKey{
			Format:  format,
			Variant: client.ClientVariant,
		}
		result[key] = append(result[key], client)
	}
	return result
}

// resolveVariantCodecs parses a variant string (e.g., "h265/aac") into video and audio codecs.
// Falls back to session source codecs if variant is empty or "source/source".
// All returned codec names are normalized for consistent display.
func (b *FlowBuilder) resolveVariantCodecs(session RelaySessionInfo, variant string) (videoCodec, audioCodec string) {
	// Default to source codecs (already normalized in ToSessionInfo)
	videoCodec = session.VideoCodec
	audioCodec = session.AudioCodec

	// If no variant or it's the copy variant, use source codecs
	if variant == "" || variant == "source/source" {
		return videoCodec, audioCodec
	}

	// Parse variant string "video/audio"
	parts := strings.Split(variant, "/")
	if len(parts) >= 1 && parts[0] != "" && parts[0] != "copy" {
		videoCodec = codec.Normalize(parts[0])
	}
	if len(parts) >= 2 && parts[1] != "" && parts[1] != "copy" {
		audioCodec = codec.Normalize(parts[1])
	}

	return videoCodec, audioCodec
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
			Label:           label,
			SessionID:       session.SessionID,
			ChannelID:       session.ChannelID,
			ChannelName:     session.ChannelName,
			SourceName:      session.StreamSourceName,
			SourceURL:       session.SourceURL,
			SourceFormat:    session.SourceFormat,
			VideoCodec:      session.VideoCodec,
			AudioCodec:      session.AudioCodec,
			Framerate:       session.Framerate,
			VideoWidth:      session.VideoWidth,
			VideoHeight:     session.VideoHeight,
			IngressBps:      session.IngressRateBps,
			TotalBytesIn:    session.BytesIn,
			DurationSecs:    session.DurationSecs,
			OriginConnected: session.OriginConnected,
		},
	}
}

func (b *FlowBuilder) buildBufferNode(session RelaySessionInfo) RelayFlowNode {
	data := FlowNodeData{
		Label:           "Buffer",
		SessionID:       session.SessionID,
		ChannelID:       session.ChannelID,
		ChannelName:     session.ChannelName,
		OriginConnected: session.OriginConnected,
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
		Label:           "FFmpeg",
		SessionID:       session.SessionID,
		ChannelID:       session.ChannelID,
		ChannelName:     session.ChannelName,
		OriginConnected: session.OriginConnected,
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

	// Resolve target codecs - "copy" means use source codec, so display source codec instead
	if session.TargetVideoCodec != "" && session.TargetVideoCodec != "copy" {
		data.TargetVideoCodec = session.TargetVideoCodec
	} else {
		data.TargetVideoCodec = session.VideoCodec
	}
	if session.TargetAudioCodec != "" && session.TargetAudioCodec != "copy" {
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

// buildESTranscoderNode creates a node for a specific ES transcoder instance.
func (b *FlowBuilder) buildESTranscoderNode(session RelaySessionInfo, transcoder ESTranscoderInfo) RelayFlowNode {
	// Parse target variant for codec display (normalize for consistent display)
	targetVideo, targetAudio := codec.Normalize(transcoder.VideoCodec), codec.Normalize(transcoder.AudioCodec)
	if targetVideo == "" || targetAudio == "" {
		parts := strings.Split(transcoder.TargetVariant, "/")
		if len(parts) >= 1 && targetVideo == "" {
			targetVideo = codec.Normalize(parts[0])
		}
		if len(parts) >= 2 && targetAudio == "" {
			targetAudio = codec.Normalize(parts[1])
		}
	}

	// Parse source variant for display (normalize for consistent display)
	// Session codecs are already normalized in ToSessionInfo()
	sourceVideo, sourceAudio := session.VideoCodec, session.AudioCodec
	if transcoder.SourceVariant != "" {
		parts := strings.Split(transcoder.SourceVariant, "/")
		if len(parts) >= 1 {
			sourceVideo = codec.Normalize(parts[0])
		}
		if len(parts) >= 2 {
			sourceAudio = codec.Normalize(parts[1])
		}
	}

	// Create a label that shows the target variant
	label := fmt.Sprintf("FFmpeg (%s)", transcoder.TargetVariant)

	data := FlowNodeData{
		Label:             label,
		SessionID:         session.SessionID,
		ChannelID:         session.ChannelID,
		ChannelName:       session.ChannelName,
		OriginConnected:   session.OriginConnected,
		SourceVideoCodec:  sourceVideo,
		SourceAudioCodec:  sourceAudio,
		TargetVideoCodec:  targetVideo,
		TargetAudioCodec:  targetAudio,
		VideoEncoder:      transcoder.VideoEncoder,
		AudioEncoder:      transcoder.AudioEncoder,
		HWAccelType:       transcoder.HWAccel,
		HWAccelDevice:     transcoder.HWAccelDevice,
		TranscoderBytesIn: transcoder.BytesIn,
	}

	// Add CPU stats if available
	if transcoder.CPUPercent > 0 {
		cpuPercent := transcoder.CPUPercent
		data.TranscoderCPU = &cpuPercent
	}
	if transcoder.MemoryMB > 0 {
		memMB := transcoder.MemoryMB
		data.TranscoderMemMB = &memMB
	}

	// Add resource history for sparkline graphs
	data.TranscoderCPUHistory = transcoder.CPUHistory
	data.TranscoderMemHistory = transcoder.MemoryHistory

	// Generate unique ID using the target variant
	transcoderID := fmt.Sprintf("transcoder-%s-%s", session.SessionID, strings.ReplaceAll(transcoder.TargetVariant, "/", "-"))

	return RelayFlowNode{
		ID:       transcoderID,
		Type:     FlowNodeTypeTranscoder,
		Position: FlowPosition{X: 0, Y: 0}, // Frontend calculates layout
		Data:     data,
	}
}

func (b *FlowBuilder) buildProcessorNode(session RelaySessionInfo, key ProcessorKey) RelayFlowNode {
	// Resolve codecs from the processor's variant
	outputVideoCodec, outputAudioCodec := b.resolveVariantCodecs(session, key.Variant)

	return RelayFlowNode{
		ID:       fmt.Sprintf("processor-%s-%s", session.SessionID, key.String()),
		Type:     FlowNodeTypeProcessor,
		Position: FlowPosition{X: 0, Y: 0}, // Frontend calculates layout
		Data: FlowNodeData{
			Label:            b.getProcessorLabel(key.Format),
			SessionID:        session.SessionID,
			ChannelID:        session.ChannelID,
			ChannelName:      session.ChannelName,
			OriginConnected:  session.OriginConnected,
			RouteType:        session.RouteType,
			ProfileName:      session.ProfileName,
			OutputFormat:     key.Format,
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
		// Note: ParentID intentionally omitted - frontend calculates absolute positions
		// Setting parentId would cause React Flow to add parent's position as offset
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
