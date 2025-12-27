package handlers

import (
	"context"
	"strings"
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

// ActiveJobDTO is the API representation of an active transcode job.
type ActiveJobDTO struct {
	ID            string  `json:"id"`
	SessionID     string  `json:"session_id"`
	ChannelID     string  `json:"channel_id"`
	ChannelName   string  `json:"channel_name"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryMB      float64 `json:"memory_mb"`
	EncodingSpeed float64 `json:"encoding_speed"`
	SamplesIn     uint64  `json:"samples_in"`
	SamplesOut    uint64  `json:"samples_out"`
	BytesIn       uint64  `json:"bytes_in"`
	BytesOut      uint64  `json:"bytes_out"`
	RunningTimeMs int64   `json:"running_time_ms"`
	HWAccel       string  `json:"hw_accel,omitempty"`
	HWDevice      string  `json:"hw_device,omitempty"`
	FFmpegCommand string  `json:"ffmpeg_command,omitempty"`
}

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
	ActiveCPUJobs      int               `json:"active_cpu_jobs"`      // Software encoding jobs
	ActiveGPUJobs      int               `json:"active_gpu_jobs"`      // Hardware encoding jobs
	ActiveJobDetails   []ActiveJobDTO    `json:"active_job_details,omitempty"`
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
	MaxConcurrentJobs int            `json:"max_concurrent_jobs"`          // Overall guard limit
	MaxCPUJobs        int            `json:"max_cpu_jobs,omitempty"`       // Max CPU (software) jobs (0 = use guard)
	MaxGPUJobs        int            `json:"max_gpu_jobs,omitempty"`       // Max GPU (hardware) jobs (0 = no GPU)
	MaxProbeJobs      int            `json:"max_probe_jobs,omitempty"`     // Max probe operations
	HWAccels          []HWAccelDTO   `json:"hw_accels,omitempty"`
	GPUs              []GPUInfoDTO   `json:"gpus,omitempty"`
}

// FilteredEncoderDTO represents an encoder that was filtered out.
type FilteredEncoderDTO struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// HWAccelDTO is the API representation of hardware acceleration info.
type HWAccelDTO struct {
	Type             string               `json:"type"`
	Device           string               `json:"device"`
	Available        bool                 `json:"available"`
	Encoders         []string             `json:"hw_encoders"`
	Decoders         []string             `json:"hw_decoders"`
	FilteredEncoders []FilteredEncoderDTO `json:"filtered_encoders,omitempty"`
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
	TotalCPUJobs         int     `json:"total_cpu_jobs"`         // Current CPU jobs across cluster
	TotalGPUJobs         int     `json:"total_gpu_jobs"`         // Current GPU jobs across cluster
	MaxConcurrentJobs    int     `json:"max_concurrent_jobs"`    // Sum of all daemon guard limits
	MaxCPUJobs           int     `json:"max_cpu_jobs"`           // Sum of all daemon CPU job limits
	MaxGPUJobs           int     `json:"max_gpu_jobs"`           // Sum of all daemon GPU job limits
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

func daemonToDTOWithJobs(d *types.Daemon, jobs []service.ActiveJobInfo) DaemonDTO {
	// Use actual job count from job manager as source of truth
	// (heartbeat-reported d.ActiveJobs may be stale or not updated)
	actualJobCount := len(jobs)

	// Compute effective state: show "transcoding" when connected with active jobs
	effectiveState := d.State.String()
	if d.State == types.DaemonStateConnected && actualJobCount > 0 {
		effectiveState = "transcoding"
	}

	// Count GPU sessions by device and CPU vs GPU jobs
	gpuSessions := make(map[string]int) // device -> session count
	cpuJobCount := 0
	gpuJobCount := 0
	for _, job := range jobs {
		if job.HWDevice != "" {
			gpuSessions[job.HWDevice]++
			gpuJobCount++
		} else {
			cpuJobCount++
		}
	}

	dto := DaemonDTO{
		ID:                 string(d.ID),
		Name:               d.Name,
		Version:            d.Version,
		Address:            d.Address,
		State:              effectiveState,
		ConnectedAt:        d.ConnectedAt.Format(time.RFC3339),
		LastHeartbeat:      d.LastHeartbeat.Format(time.RFC3339),
		HeartbeatsMissed:   d.HeartbeatsMissed,
		ActiveJobs:         actualJobCount, // Use actual count, not heartbeat-reported
		ActiveCPUJobs:      cpuJobCount,
		ActiveGPUJobs:      gpuJobCount,
		TotalJobsCompleted: d.TotalJobsCompleted,
		TotalJobsFailed:    d.TotalJobsFailed,
	}

	if d.Capabilities != nil {
		dto.Capabilities = capabilitiesToDTOWithSessions(d.Capabilities, gpuSessions)
	}

	if d.SystemStats != nil {
		dto.SystemStats = systemStatsToDTO(d.SystemStats)
	}

	if actualJobCount > 0 {
		dto.ActiveJobDetails = make([]ActiveJobDTO, 0, actualJobCount)
		for _, job := range jobs {
			dto.ActiveJobDetails = append(dto.ActiveJobDetails, ActiveJobDTO{
				ID:            job.ID,
				SessionID:     job.SessionID,
				ChannelID:     job.ChannelID,
				ChannelName:   job.ChannelName,
				CPUPercent:    job.CPUPercent,
				MemoryMB:      job.MemoryMB,
				EncodingSpeed: job.EncodingSpeed,
				SamplesIn:     job.SamplesIn,
				SamplesOut:    job.SamplesOut,
				BytesIn:       job.BytesIn,
				BytesOut:      job.BytesOut,
				RunningTimeMs: job.RunningTimeMs,
				HWAccel:       job.HWAccel,
				HWDevice:      job.HWDevice,
				FFmpegCommand: job.FFmpegCommand,
			})
		}
	}

	return dto
}

// capabilitiesToDTOWithSessions converts capabilities to DTO, updating GPU session counts
// from the active jobs' hw_device usage.
func capabilitiesToDTOWithSessions(c *types.Capabilities, gpuSessions map[string]int) *CapabilitiesDTO {
	dto := &CapabilitiesDTO{
		VideoEncoders:     c.VideoEncoders,
		VideoDecoders:     c.VideoDecoders,
		AudioEncoders:     c.AudioEncoders,
		AudioDecoders:     c.AudioDecoders,
		MaxConcurrentJobs: c.MaxConcurrentJobs,
		MaxCPUJobs:        c.MaxCPUJobs,
		MaxGPUJobs:        c.MaxGPUJobs,
		MaxProbeJobs:      c.MaxProbeJobs,
	}

	// Build device-to-GPU-index mapping from hw_accels
	// e.g., "/dev/dri/renderD128" -> 0
	deviceToGPUIndex := make(map[string]int)
	for _, hw := range c.HWAccels {
		// Convert filtered encoders
		var filteredEncoders []FilteredEncoderDTO
		for _, fe := range hw.FilteredEncoders {
			filteredEncoders = append(filteredEncoders, FilteredEncoderDTO{
				Name:   fe.Name,
				Reason: fe.Reason,
			})
		}

		dto.HWAccels = append(dto.HWAccels, HWAccelDTO{
			Type:             string(hw.Type),
			Device:           hw.Device,
			Available:        hw.Available,
			Encoders:         hw.Encoders,
			Decoders:         hw.Decoders,
			FilteredEncoders: filteredEncoders,
		})

		// Map device path to GPU index (assume GPUs are ordered by index)
		if hw.Device != "" && hw.Available {
			// Find matching GPU by checking if name contains device
			for _, gpu := range c.GPUs {
				if strings.Contains(gpu.Name, hw.Device) {
					deviceToGPUIndex[hw.Device] = gpu.Index
					break
				}
			}
			// Fallback: if only one GPU, map to it
			if _, found := deviceToGPUIndex[hw.Device]; !found && len(c.GPUs) == 1 {
				deviceToGPUIndex[hw.Device] = c.GPUs[0].Index
			}
		}
	}

	// Compute session counts per GPU index from gpuSessions map
	gpuIndexSessions := make(map[int]int)
	for device, count := range gpuSessions {
		if idx, found := deviceToGPUIndex[device]; found {
			gpuIndexSessions[idx] += count
		}
	}

	for _, gpu := range c.GPUs {
		activeSessions := gpu.ActiveEncodeSessions
		// Override with computed count if available
		if count, found := gpuIndexSessions[gpu.Index]; found {
			activeSessions = count
		}

		dto.GPUs = append(dto.GPUs, GPUInfoDTO{
			Index:                gpu.Index,
			Name:                 gpu.Name,
			Class:                gpu.Class.String(),
			Driver:               gpu.Driver,
			MaxEncodeSessions:    gpu.MaxEncodeSessions,
			MaxDecodeSessions:    gpu.MaxDecodeSessions,
			ActiveEncodeSessions: activeSessions,
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
		TotalCPUJobs:         s.TotalCPUJobs,
		TotalGPUJobs:         s.TotalGPUJobs,
		MaxConcurrentJobs:    s.MaxConcurrentJobs,
		MaxCPUJobs:           s.MaxCPUJobs,
		MaxGPUJobs:           s.MaxGPUJobs,
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
		// Get active job details for each daemon
		jobs, _ := h.service.GetDaemonActiveJobs(d.ID)
		output.Body.Daemons = append(output.Body.Daemons, daemonToDTOWithJobs(d, jobs))
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

	// Get active job details
	jobs, _ := h.service.GetDaemonActiveJobs(daemon.ID)

	return &GetDaemonOutput{
		Body: daemonToDTOWithJobs(daemon, jobs),
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
