// Package relay provides streaming relay functionality for tvarr.
package relay

// FlowNodeType represents the type of node in the relay flow graph.
type FlowNodeType string

const (
	// FlowNodeTypeOrigin represents the origin/source node.
	FlowNodeTypeOrigin FlowNodeType = "origin"
	// FlowNodeTypeBuffer represents the shared buffer node.
	FlowNodeTypeBuffer FlowNodeType = "buffer"
	// FlowNodeTypeTranscoder represents an FFmpeg transcoder node.
	FlowNodeTypeTranscoder FlowNodeType = "transcoder"
	// FlowNodeTypeProcessor represents the processing node (HLS/DASH/MPEGTS output).
	FlowNodeTypeProcessor FlowNodeType = "processor"
	// FlowNodeTypeClient represents a connected client node.
	FlowNodeTypeClient FlowNodeType = "client"
	// FlowNodeTypePassthrough represents a passthrough proxy connection.
	FlowNodeTypePassthrough FlowNodeType = "passthrough"
)

// RelayFlowNode represents a node in the relay flow graph.
// This structure is designed to be compatible with React Flow's node format.
type RelayFlowNode struct {
	// ID is a unique identifier for the node.
	ID string `json:"id"`

	// Type determines the node component to render.
	Type FlowNodeType `json:"type"`

	// Position provides layout hints for the node.
	Position FlowPosition `json:"position"`

	// Data contains node-specific information for rendering.
	Data FlowNodeData `json:"data"`

	// ParentID links to the parent node (for hierarchical layout).
	ParentID string `json:"parentId,omitempty"`
}

// FlowPosition represents the position of a node in the flow graph.
type FlowPosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// FlowNodeData contains the data payload for a flow node.
type FlowNodeData struct {
	// Common fields
	Label       string `json:"label"`
	SessionID   string `json:"sessionId,omitempty"`
	ChannelID   string `json:"channelId,omitempty"`
	ChannelName string `json:"channelName,omitempty"`

	// Origin node fields
	SourceName   string  `json:"sourceName,omitempty"` // Name of the stream source (e.g., "s8k")
	SourceURL    string  `json:"sourceUrl,omitempty"`
	SourceFormat string  `json:"sourceFormat,omitempty"` // hls, dash, mpegts
	VideoCodec   string  `json:"videoCodec,omitempty"`
	AudioCodec   string  `json:"audioCodec,omitempty"`
	Framerate    float64 `json:"framerate,omitempty"`    // Video framerate (fps)
	VideoWidth   int     `json:"videoWidth,omitempty"`   // Video width in pixels
	VideoHeight  int     `json:"videoHeight,omitempty"`  // Video height in pixels
	IngressBps   uint64  `json:"ingressBps,omitempty"`   // Bytes per second from origin
	TotalBytesIn uint64  `json:"totalBytesIn,omitempty"` // Total bytes received from upstream
	DurationSecs float64 `json:"durationSecs,omitempty"` // Session duration in seconds

	// Bandwidth history for sparkline (last 30 samples, ~1 sample/sec)
	IngressHistory []uint64 `json:"ingressHistory,omitempty"` // Historical ingress bps values

	// Buffer node fields
	BufferVariants    []BufferVariantInfo `json:"bufferVariants,omitempty"`    // Variants in the buffer
	BufferMemoryBytes uint64              `json:"bufferMemoryBytes,omitempty"` // Total resident memory size
	MaxBufferBytes    uint64              `json:"maxBufferBytes,omitempty"`    // Maximum buffer size per variant
	VideoSampleCount  int                 `json:"videoSampleCount,omitempty"`  // Number of video samples in buffer
	AudioSampleCount  int                 `json:"audioSampleCount,omitempty"`  // Number of audio samples in buffer
	BufferUtilization float64             `json:"bufferUtilization,omitempty"` // 0-100 percentage of max buffer used

	// Transcoder node fields (FFmpeg)
	TranscoderID      string   `json:"transcoderId,omitempty"`
	SourceVideoCodec  string   `json:"sourceVideoCodec,omitempty"`  // Source video codec (e.g., "h264")
	SourceAudioCodec  string   `json:"sourceAudioCodec,omitempty"`  // Source audio codec (e.g., "aac")
	TargetVideoCodec  string   `json:"targetVideoCodec,omitempty"`  // Target video codec (e.g., "h265")
	TargetAudioCodec  string   `json:"targetAudioCodec,omitempty"`  // Target audio codec (e.g., "aac")
	VideoEncoder      string   `json:"videoEncoder,omitempty"`      // FFmpeg video encoder (e.g., "libx265", "h264_nvenc")
	AudioEncoder      string   `json:"audioEncoder,omitempty"`      // FFmpeg audio encoder (e.g., "aac", "libopus")
	HWAccelType       string   `json:"hwAccelType,omitempty"`       // Hardware acceleration type (e.g., "cuda", "qsv", "vaapi")
	HWAccelDevice     string   `json:"hwAccelDevice,omitempty"`     // Hardware acceleration device (e.g., "/dev/dri/renderD128")
	EncodingSpeed     *float64 `json:"encodingSpeed,omitempty"`     // Realtime encoding speed (1.0 = realtime)
	TranscoderCPU     *float64 `json:"transcoderCpu,omitempty"`     // CPU usage percentage
	TranscoderMemMB   *float64 `json:"transcoderMemMb,omitempty"`   // Memory usage in MB
	TranscoderBytesIn uint64   `json:"transcoderBytesIn,omitempty"` // Bytes read by transcoder

	// Resource history for sparklines (last 30 samples, ~1 sample/sec)
	TranscoderCPUHistory []float64 `json:"transcoderCpuHistory,omitempty"` // Historical CPU usage percentage
	TranscoderMemHistory []float64 `json:"transcoderMemHistory,omitempty"` // Historical memory usage in MB

	// Processor node fields
	RouteType        RouteType `json:"routeType,omitempty"` // passthrough, repackage, transcode
	ProfileName      string    `json:"profileName,omitempty"`
	OutputFormat     string    `json:"outputFormat,omitempty"`     // hls, dash, mpegts
	OutputVideoCodec string    `json:"outputVideoCodec,omitempty"` // Output video codec (same as input for passthrough/repackage)
	OutputAudioCodec string    `json:"outputAudioCodec,omitempty"` // Output audio codec (same as input for passthrough/repackage)
	CPUPercent       *float64  `json:"cpuPercent,omitempty"`       // Only for FFmpeg
	MemoryMB         *float64  `json:"memoryMB,omitempty"`         // Only for FFmpeg
	ProcessingBps    uint64    `json:"processingBps,omitempty"`
	TotalBytesOut    uint64    `json:"totalBytesOut,omitempty"` // Total bytes sent to clients

	// Bandwidth history for sparkline (last 30 samples, ~1 sample/sec)
	EgressHistory []uint64 `json:"egressHistory,omitempty"` // Historical egress bps values

	// Client node fields
	ClientID      string  `json:"clientId,omitempty"`
	PlayerType    string  `json:"playerType,omitempty"`   // hls.js, mpegts.js, VLC, etc.
	ClientFormat  string  `json:"clientFormat,omitempty"` // Format this client is using (hls, mpegts, dash)
	RemoteAddr    string  `json:"remoteAddr,omitempty"`
	UserAgent     string  `json:"userAgent,omitempty"`     // Full user agent string
	DetectionRule string  `json:"detectionRule,omitempty"` // Client detection rule that matched
	BytesRead     uint64  `json:"bytesRead,omitempty"`
	EgressBps     uint64  `json:"egressBps,omitempty"`     // Bytes per second to client
	ConnectedSecs float64 `json:"connectedSecs,omitempty"` // How long client has been connected

	// Bandwidth history for sparkline (last 30 samples, ~1 sample/sec)
	ClientEgressHistory []uint64 `json:"clientEgressHistory,omitempty"` // Historical client egress bps values

	// Status fields
	InFallback bool   `json:"inFallback,omitempty"`
	Error      string `json:"error,omitempty"`
}

// BufferVariantInfo describes a codec variant in the shared buffer.
type BufferVariantInfo struct {
	Variant        string  `json:"variant"` // e.g., "h264/aac", "hevc/aac"
	VideoCodec     string  `json:"videoCodec"`
	AudioCodec     string  `json:"audioCodec"`
	VideoSamples   int     `json:"videoSamples"`
	AudioSamples   int     `json:"audioSamples"`
	BytesIngested  uint64  `json:"bytesIngested"`
	MaxBytes       uint64  `json:"maxBytes"`       // Maximum bytes for this variant (0 = unlimited)
	Utilization    float64 `json:"utilization"`    // 0-100 percentage of max used
	IsSource       bool    `json:"isSource"`       // True if this is the original source variant
	IsEvicting     bool    `json:"isEvicting"`     // True if buffer is at capacity and evicting
	BufferDuration float64 `json:"bufferDuration"` // Duration of content in buffer (seconds)
	EvictedSamples uint64  `json:"evictedSamples"` // Total samples evicted since start
	EvictedBytes   uint64  `json:"evictedBytes"`   // Total bytes evicted since start
}

// RelayFlowEdge represents an edge (connection) in the relay flow graph.
// This structure is designed to be compatible with React Flow's edge format.
type RelayFlowEdge struct {
	// ID is a unique identifier for the edge.
	ID string `json:"id"`

	// Source is the ID of the source node.
	Source string `json:"source"`

	// Target is the ID of the target node.
	Target string `json:"target"`

	// SourceHandle specifies which handle on the source node to connect from.
	SourceHandle string `json:"sourceHandle,omitempty"`

	// TargetHandle specifies which handle on the target node to connect to.
	TargetHandle string `json:"targetHandle,omitempty"`

	// Type determines the edge component to render.
	Type string `json:"type,omitempty"` // "animated" for data flow animation

	// Animated enables animation on the edge.
	Animated bool `json:"animated"`

	// Label displays information on the edge.
	Label string `json:"label,omitempty"`

	// Data contains edge-specific information.
	Data FlowEdgeData `json:"data"`

	// Style customization
	Style *FlowEdgeStyle `json:"style,omitempty"`
}

// FlowEdgeData contains the data payload for a flow edge.
type FlowEdgeData struct {
	// Bandwidth is the current throughput in bytes per second.
	BandwidthBps uint64 `json:"bandwidthBps"`

	// Codec information for the stream on this edge.
	VideoCodec string `json:"videoCodec,omitempty"`
	AudioCodec string `json:"audioCodec,omitempty"`

	// Format of the stream on this edge.
	Format string `json:"format,omitempty"` // mpegts, fmp4, hls, dash
}

// FlowEdgeStyle contains styling information for an edge.
type FlowEdgeStyle struct {
	Stroke      string `json:"stroke,omitempty"`
	StrokeWidth int    `json:"strokeWidth,omitempty"`
}

// RelayFlowGraph represents the complete flow graph for visualization.
type RelayFlowGraph struct {
	// Nodes in the flow graph.
	Nodes []RelayFlowNode `json:"nodes"`

	// Edges connecting the nodes.
	Edges []RelayFlowEdge `json:"edges"`

	// Metadata about the graph.
	Metadata FlowGraphMetadata `json:"metadata"`
}

// FlowGraphMetadata contains metadata about the flow graph.
type FlowGraphMetadata struct {
	// TotalSessions is the number of active relay sessions.
	TotalSessions int `json:"totalSessions"`

	// TotalClients is the number of connected clients across all sessions.
	TotalClients int `json:"totalClients"`

	// TotalIngressBps is the combined ingress bandwidth.
	TotalIngressBps uint64 `json:"totalIngressBps"`

	// TotalEgressBps is the combined egress bandwidth.
	TotalEgressBps uint64 `json:"totalEgressBps"`

	// GeneratedAt is the timestamp when this graph was generated.
	GeneratedAt string `json:"generatedAt"`

	// System resource usage
	SystemCPUPercent    float64 `json:"systemCpuPercent,omitempty"`
	SystemMemoryPercent float64 `json:"systemMemoryPercent,omitempty"`
	SystemMemoryUsedMB  uint64  `json:"systemMemoryUsedMb,omitempty"`
	SystemMemoryTotalMB uint64  `json:"systemMemoryTotalMb,omitempty"`
}
