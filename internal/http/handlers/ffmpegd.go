package handlers

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/tvarr/internal/service"
	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
)

// FFmpegDHandler handles ffmpegd daemon management API endpoints.
// T091: Create internal/http/handlers/ffmpegd_handler.go with REST handlers
type FFmpegDHandler struct {
	service *service.FFmpegDService
}

// NewFFmpegDHandler creates a new ffmpegd handler.
func NewFFmpegDHandler(service *service.FFmpegDService) *FFmpegDHandler {
	return &FFmpegDHandler{
		service: service,
	}
}

// --- DTO Types ---

// DaemonDTO is the API representation of a daemon.
type DaemonDTO struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	Version            string            `json:"version"`
	Address            string            `json:"address"`
	State              string            `json:"state"`
	ConnectedAt        string            `json:"connected_at"`
	LastHeartbeat      string            `json:"last_heartbeat"`
	HeartbeatsMissed   int               `json:"heartbeats_missed"`
	ActiveJobs         int               `json:"active_jobs"`
	TotalJobsCompleted uint64            `json:"total_jobs_completed"`
	TotalJobsFailed    uint64            `json:"total_jobs_failed"`
	Capabilities       *CapabilitiesDTO  `json:"capabilities,omitempty"`
	SystemStats        *SystemStatsDTO   `json:"system_stats,omitempty"`
}

// CapabilitiesDTO is the API representation of daemon capabilities.
type CapabilitiesDTO struct {
	VideoEncoders     []string       `json:"video_encoders"`
	VideoDecoders     []string       `json:"video_decoders"`
	AudioEncoders     []string       `json:"audio_encoders"`
	AudioDecoders     []string       `json:"audio_decoders"`
	MaxConcurrentJobs int            `json:"max_concurrent_jobs"`
	HWAccels          []HWAccelDTO   `json:"hw_accels,omitempty"`
	GPUs              []GPUInfoDTO   `json:"gpus,omitempty"`
}

// HWAccelDTO is the API representation of hardware acceleration info.
type HWAccelDTO struct {
	Type      string   `json:"type"`
	Device    string   `json:"device"`
	Available bool     `json:"available"`
	Encoders  []string `json:"encoders"`
	Decoders  []string `json:"decoders"`
}

// GPUInfoDTO is the API representation of GPU info.
type GPUInfoDTO struct {
	Index                int    `json:"index"`
	Name                 string `json:"name"`
	Class                string `json:"class"`
	Driver               string `json:"driver,omitempty"`
	MaxEncodeSessions    int    `json:"max_encode_sessions"`
	MaxDecodeSessions    int    `json:"max_decode_sessions"`
	ActiveEncodeSessions int    `json:"active_encode_sessions"`
	ActiveDecodeSessions int    `json:"active_decode_sessions"`
	MemoryTotal          uint64 `json:"memory_total,omitempty"`
}

// SystemStatsDTO is the API representation of system stats.
type SystemStatsDTO struct {
	Hostname        string         `json:"hostname"`
	OS              string         `json:"os,omitempty"`
	Arch            string         `json:"arch,omitempty"`
	CPUCores        int            `json:"cpu_cores"`
	CPUPercent      float64        `json:"cpu_percent"`
	MemoryTotal     uint64         `json:"memory_total"`
	MemoryUsed      uint64         `json:"memory_used"`
	MemoryAvailable uint64         `json:"memory_available"`
	MemoryPercent   float64        `json:"memory_percent"`
	GPUs            []GPUStatsDTO  `json:"gpus,omitempty"`
}

// GPUStatsDTO is the API representation of GPU stats.
type GPUStatsDTO struct {
	Index                int     `json:"index"`
	Name                 string  `json:"name"`
	Utilization          float64 `json:"utilization"`
	MemoryTotal          uint64  `json:"memory_total"`
	MemoryUsed           uint64  `json:"memory_used"`
	MemoryPercent        float64 `json:"memory_percent"`
	Temperature          int     `json:"temperature"`
	EncoderUtilization   float64 `json:"encoder_utilization"`
	DecoderUtilization   float64 `json:"decoder_utilization"`
	ActiveEncodeSessions int     `json:"active_encode_sessions"`
	MaxEncodeSessions    int     `json:"max_encode_sessions"`
}

// ClusterStatsDTO is the API representation of cluster stats.
type ClusterStatsDTO struct {
	TotalDaemons         int     `json:"total_daemons"`
	ActiveDaemons        int     `json:"active_daemons"`
	UnhealthyDaemons     int     `json:"unhealthy_daemons"`
	DrainingDaemons      int     `json:"draining_daemons"`
	DisconnectedDaemons  int     `json:"disconnected_daemons"`
	TotalActiveJobs      int     `json:"total_active_jobs"`
	TotalGPUs            int     `json:"total_gpus"`
	AvailableGPUSessions int     `json:"available_gpu_sessions"`
	TotalGPUSessions     int     `json:"total_gpu_sessions"`
	AverageCPUPercent    float64 `json:"average_cpu_percent"`
	AverageMemPercent    float64 `json:"average_memory_percent"`
}

// --- Request/Response Types ---

// ListDaemonsInput is the input for listing daemons.
type ListDaemonsInput struct {
	State    string `query:"state" doc:"Filter by daemon state (active, draining, unhealthy, disconnected)"`
	Encoder  string `query:"encoder" doc:"Filter by encoder capability"`
}

// ListDaemonsOutput is the output for listing daemons.
type ListDaemonsOutput struct {
	Body struct {
		Daemons []DaemonDTO `json:"daemons"`
		Total   int         `json:"total"`
	}
}

// GetDaemonInput is the input for getting a daemon.
type GetDaemonInput struct {
	ID string `path:"id" doc:"Daemon ID"`
}

// GetDaemonOutput is the output for getting a daemon.
type GetDaemonOutput struct {
	Body DaemonDTO
}

// GetClusterStatsOutput is the output for cluster stats.
type GetClusterStatsOutput struct {
	Body ClusterStatsDTO
}

// DrainDaemonInput is the input for draining a daemon.
type DrainDaemonInput struct {
	ID string `path:"id" doc:"Daemon ID"`
}

// DrainDaemonOutput is the output for draining a daemon.
type DrainDaemonOutput struct {
	Body struct {
		Success       bool   `json:"success"`
		Message       string `json:"message"`
		RemainingJobs int    `json:"remaining_jobs"`
	}
}

// ActivateDaemonInput is the input for activating a daemon.
type ActivateDaemonInput struct {
	ID string `path:"id" doc:"Daemon ID"`
}

// ActivateDaemonOutput is the output for activating a daemon.
type ActivateDaemonOutput struct {
	Body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
}

// --- Conversion Functions ---

func daemonToDTO(d *types.Daemon) DaemonDTO {
	dto := DaemonDTO{
		ID:                 string(d.ID),
		Name:               d.Name,
		Version:            d.Version,
		Address:            d.Address,
		State:              d.State.String(),
		ConnectedAt:        d.ConnectedAt.Format(time.RFC3339),
		LastHeartbeat:      d.LastHeartbeat.Format(time.RFC3339),
		HeartbeatsMissed:   d.HeartbeatsMissed,
		ActiveJobs:         d.ActiveJobs,
		TotalJobsCompleted: d.TotalJobsCompleted,
		TotalJobsFailed:    d.TotalJobsFailed,
	}

	if d.Capabilities != nil {
		dto.Capabilities = capabilitiesToDTO(d.Capabilities)
	}

	if d.SystemStats != nil {
		dto.SystemStats = systemStatsToDTO(d.SystemStats)
	}

	return dto
}

func capabilitiesToDTO(c *types.Capabilities) *CapabilitiesDTO {
	dto := &CapabilitiesDTO{
		VideoEncoders:     c.VideoEncoders,
		VideoDecoders:     c.VideoDecoders,
		AudioEncoders:     c.AudioEncoders,
		AudioDecoders:     c.AudioDecoders,
		MaxConcurrentJobs: c.MaxConcurrentJobs,
	}

	for _, hw := range c.HWAccels {
		dto.HWAccels = append(dto.HWAccels, HWAccelDTO{
			Type:      string(hw.Type),
			Device:    hw.Device,
			Available: hw.Available,
			Encoders:  hw.Encoders,
			Decoders:  hw.Decoders,
		})
	}

	for _, gpu := range c.GPUs {
		dto.GPUs = append(dto.GPUs, GPUInfoDTO{
			Index:                gpu.Index,
			Name:                 gpu.Name,
			Class:                gpu.Class.String(),
			Driver:               gpu.Driver,
			MaxEncodeSessions:    gpu.MaxEncodeSessions,
			MaxDecodeSessions:    gpu.MaxDecodeSessions,
			ActiveEncodeSessions: gpu.ActiveEncodeSessions,
			ActiveDecodeSessions: gpu.ActiveDecodeSessions,
			MemoryTotal:          gpu.MemoryTotal,
		})
	}

	return dto
}

func systemStatsToDTO(s *types.SystemStats) *SystemStatsDTO {
	dto := &SystemStatsDTO{
		Hostname:        s.Hostname,
		OS:              s.OS,
		Arch:            s.Arch,
		CPUCores:        s.CPUCores,
		CPUPercent:      s.CPUPercent,
		MemoryTotal:     s.MemoryTotal,
		MemoryUsed:      s.MemoryUsed,
		MemoryAvailable: s.MemoryAvailable,
		MemoryPercent:   s.MemoryPercent,
	}

	for _, gpu := range s.GPUs {
		dto.GPUs = append(dto.GPUs, GPUStatsDTO{
			Index:                gpu.Index,
			Name:                 gpu.Name,
			Utilization:          gpu.Utilization,
			MemoryTotal:          gpu.MemoryTotal,
			MemoryUsed:           gpu.MemoryUsed,
			MemoryPercent:        gpu.MemoryPercent,
			Temperature:          gpu.Temperature,
			EncoderUtilization:   gpu.EncoderUtilization,
			DecoderUtilization:   gpu.DecoderUtilization,
			ActiveEncodeSessions: gpu.ActiveEncodeSessions,
			MaxEncodeSessions:    gpu.MaxEncodeSessions,
		})
	}

	return dto
}

func clusterStatsToDTO(s service.ClusterStats) ClusterStatsDTO {
	return ClusterStatsDTO{
		TotalDaemons:         s.TotalDaemons,
		ActiveDaemons:        s.ActiveDaemons,
		UnhealthyDaemons:     s.UnhealthyDaemons,
		DrainingDaemons:      s.DrainingDaemons,
		DisconnectedDaemons:  s.DisconnectedDaemons,
		TotalActiveJobs:      s.TotalActiveJobs,
		TotalGPUs:            s.TotalGPUs,
		AvailableGPUSessions: s.AvailableGPUSessions,
		TotalGPUSessions:     s.TotalGPUSessions,
		AverageCPUPercent:    s.AverageCPUPercent,
		AverageMemPercent:    s.AverageMemPercent,
	}
}

// --- Handler Methods ---

// Register registers the ffmpegd routes with the API.
// T092: Register ffmpegd REST routes
func (h *FFmpegDHandler) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "listDaemons",
		Method:      "GET",
		Path:        "/api/v1/transcoders",
		Summary:     "List transcoders",
		Description: "Returns all registered ffmpegd transcoders/daemons",
		Tags:        []string{"Transcoders"},
	}, h.ListDaemons)

	huma.Register(api, huma.Operation{
		OperationID: "getDaemon",
		Method:      "GET",
		Path:        "/api/v1/transcoders/{id}",
		Summary:     "Get transcoder",
		Description: "Returns a single transcoder by ID",
		Tags:        []string{"Transcoders"},
	}, h.GetDaemon)

	huma.Register(api, huma.Operation{
		OperationID: "getClusterStats",
		Method:      "GET",
		Path:        "/api/v1/transcoders/stats",
		Summary:     "Get cluster statistics",
		Description: "Returns aggregate statistics about the transcoder cluster",
		Tags:        []string{"Transcoders"},
	}, h.GetClusterStats)

	huma.Register(api, huma.Operation{
		OperationID: "drainDaemon",
		Method:      "POST",
		Path:        "/api/v1/transcoders/{id}/drain",
		Summary:     "Drain transcoder",
		Description: "Put transcoder in draining state (no new jobs, finish existing)",
		Tags:        []string{"Transcoders"},
	}, h.DrainDaemon)

	huma.Register(api, huma.Operation{
		OperationID: "activateDaemon",
		Method:      "POST",
		Path:        "/api/v1/transcoders/{id}/activate",
		Summary:     "Activate transcoder",
		Description: "Put transcoder back in active state",
		Tags:        []string{"Transcoders"},
	}, h.ActivateDaemon)
}

// ListDaemons returns all registered daemons.
func (h *FFmpegDHandler) ListDaemons(ctx context.Context, input *ListDaemonsInput) (*ListDaemonsOutput, error) {
	var daemons []*types.Daemon

	// Apply filters
	if input.State != "" {
		state := parseState(input.State)
		daemons = h.service.GetDaemonsByState(state)
	} else if input.Encoder != "" {
		daemons = h.service.GetDaemonsByCapability(input.Encoder)
	} else {
		daemons = h.service.ListDaemons()
	}

	output := &ListDaemonsOutput{}
	output.Body.Daemons = make([]DaemonDTO, 0, len(daemons))
	for _, d := range daemons {
		output.Body.Daemons = append(output.Body.Daemons, daemonToDTO(d))
	}
	output.Body.Total = len(output.Body.Daemons)

	return output, nil
}

// GetDaemon returns a single daemon by ID.
func (h *FFmpegDHandler) GetDaemon(ctx context.Context, input *GetDaemonInput) (*GetDaemonOutput, error) {
	daemon, err := h.service.GetDaemon(types.DaemonID(input.ID))
	if err != nil {
		return nil, huma.Error404NotFound("daemon not found")
	}

	return &GetDaemonOutput{
		Body: daemonToDTO(daemon),
	}, nil
}

// GetClusterStats returns cluster statistics.
func (h *FFmpegDHandler) GetClusterStats(ctx context.Context, input *struct{}) (*GetClusterStatsOutput, error) {
	stats := h.service.GetClusterStats()
	return &GetClusterStatsOutput{
		Body: clusterStatsToDTO(stats),
	}, nil
}

// DrainDaemon puts a daemon into draining state.
func (h *FFmpegDHandler) DrainDaemon(ctx context.Context, input *DrainDaemonInput) (*DrainDaemonOutput, error) {
	daemon, err := h.service.GetDaemon(types.DaemonID(input.ID))
	if err != nil {
		return nil, huma.Error404NotFound("daemon not found")
	}

	err = h.service.DrainDaemon(types.DaemonID(input.ID))
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	return &DrainDaemonOutput{
		Body: struct {
			Success       bool   `json:"success"`
			Message       string `json:"message"`
			RemainingJobs int    `json:"remaining_jobs"`
		}{
			Success:       true,
			Message:       "Daemon is now draining",
			RemainingJobs: daemon.ActiveJobs,
		},
	}, nil
}

// ActivateDaemon puts a daemon back into active state.
func (h *FFmpegDHandler) ActivateDaemon(ctx context.Context, input *ActivateDaemonInput) (*ActivateDaemonOutput, error) {
	err := h.service.ActivateDaemon(types.DaemonID(input.ID))
	if err != nil {
		if err.Error() == "daemon not found: "+input.ID {
			return nil, huma.Error404NotFound("daemon not found")
		}
		return nil, huma.Error400BadRequest(err.Error())
	}

	return &ActivateDaemonOutput{
		Body: struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}{
			Success: true,
			Message: "Daemon is now active",
		},
	}, nil
}

// parseState parses a state string into DaemonState.
func parseState(s string) types.DaemonState {
	switch s {
	case "connected":
		return types.DaemonStateConnected
	case "draining":
		return types.DaemonStateDraining
	case "unhealthy":
		return types.DaemonStateUnhealthy
	case "disconnected":
		return types.DaemonStateDisconnected
	case "connecting":
		return types.DaemonStateConnecting
	default:
		return types.DaemonStateConnected
	}
}
