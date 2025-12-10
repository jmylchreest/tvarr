// Package relay provides streaming relay functionality for tvarr.
package relay

// FlowNodeType represents the type of node in the relay flow graph.
type FlowNodeType string

const (
	// FlowNodeTypeOrigin represents the origin/source node.
	FlowNodeTypeOrigin FlowNodeType = "origin"
	// FlowNodeTypeProcessor represents the processing node (FFmpeg, gohlslib, etc).
	FlowNodeTypeProcessor FlowNodeType = "processor"
	// FlowNodeTypeClient represents a connected client node.
	FlowNodeTypeClient FlowNodeType = "client"
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
	SourceName   string `json:"sourceName,omitempty"`   // Name of the stream source (e.g., "s8k")
	SourceURL    string `json:"sourceUrl,omitempty"`
	SourceFormat string `json:"sourceFormat,omitempty"` // hls, dash, mpegts
	VideoCodec   string `json:"videoCodec,omitempty"`
	AudioCodec   string `json:"audioCodec,omitempty"`
	IngressBps   uint64 `json:"ingressBps,omitempty"` // Bytes per second from origin

	// Processor node fields
	RouteType        RouteType `json:"routeType,omitempty"` // passthrough, repackage, transcode
	ProfileName      string    `json:"profileName,omitempty"`
	OutputFormat     string    `json:"outputFormat,omitempty"`     // hls, dash, mpegts
	OutputVideoCodec string    `json:"outputVideoCodec,omitempty"` // Output video codec (same as input for passthrough/repackage)
	OutputAudioCodec string    `json:"outputAudioCodec,omitempty"` // Output audio codec (same as input for passthrough/repackage)
	CPUPercent       *float64  `json:"cpuPercent,omitempty"`       // Only for FFmpeg
	MemoryMB         *float64  `json:"memoryMB,omitempty"`         // Only for FFmpeg
	ProcessingBps    uint64    `json:"processingBps,omitempty"`

	// Client node fields
	ClientID   string `json:"clientId,omitempty"`
	PlayerType string `json:"playerType,omitempty"` // hls.js, mpegts.js, VLC, etc.
	RemoteAddr string `json:"remoteAddr,omitempty"`
	UserAgent  string `json:"userAgent,omitempty"` // Full user agent string
	BytesRead  uint64 `json:"bytesRead,omitempty"`
	EgressBps  uint64 `json:"egressBps,omitempty"` // Bytes per second to client

	// Status fields
	InFallback bool   `json:"inFallback,omitempty"`
	Error      string `json:"error,omitempty"`
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
}
