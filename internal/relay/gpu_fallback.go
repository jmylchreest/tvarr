package relay

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
)

// GPUExhaustedPolicy defines how to handle GPU session exhaustion.
type GPUExhaustedPolicy string

const (
	// GPUPolicyFallback falls back to software encoding when GPU sessions are exhausted.
	GPUPolicyFallback GPUExhaustedPolicy = "fallback"

	// GPUPolicyQueue waits for a GPU session to become available.
	GPUPolicyQueue GPUExhaustedPolicy = "queue"

	// GPUPolicyReject immediately fails the request when GPU sessions are exhausted.
	GPUPolicyReject GPUExhaustedPolicy = "reject"
)

// String returns the string representation of the policy.
func (p GPUExhaustedPolicy) String() string {
	return string(p)
}

// ParseGPUExhaustedPolicy parses a string into a GPUExhaustedPolicy.
func ParseGPUExhaustedPolicy(s string) (GPUExhaustedPolicy, error) {
	switch s {
	case string(GPUPolicyFallback), "":
		return GPUPolicyFallback, nil // Default
	case string(GPUPolicyQueue):
		return GPUPolicyQueue, nil
	case string(GPUPolicyReject):
		return GPUPolicyReject, nil
	default:
		return GPUPolicyFallback, fmt.Errorf("unknown GPU exhausted policy: %s", s)
	}
}

// GPUFallbackHandler handles GPU session exhaustion according to the configured policy.
type GPUFallbackHandler struct {
	policy   GPUExhaustedPolicy
	registry *DaemonRegistry
	logger   *slog.Logger

	// Queue management (for GPUPolicyQueue)
	mu            sync.Mutex
	waitQueue     []chan *types.Daemon
	queueTimeout  time.Duration
	maxQueueSize  int
	notifyChannel chan struct{}
}

// GPUFallbackConfig configures the GPU fallback handler.
type GPUFallbackConfig struct {
	Policy       GPUExhaustedPolicy
	QueueTimeout time.Duration
	MaxQueueSize int
}

// DefaultGPUFallbackConfig returns the default configuration.
func DefaultGPUFallbackConfig() GPUFallbackConfig {
	return GPUFallbackConfig{
		Policy:       GPUPolicyFallback,
		QueueTimeout: 30 * time.Second,
		MaxQueueSize: 100,
	}
}

// NewGPUFallbackHandler creates a new GPU fallback handler.
func NewGPUFallbackHandler(registry *DaemonRegistry, config GPUFallbackConfig, logger *slog.Logger) *GPUFallbackHandler {
	return &GPUFallbackHandler{
		policy:        config.Policy,
		registry:      registry,
		logger:        logger,
		queueTimeout:  config.QueueTimeout,
		maxQueueSize:  config.MaxQueueSize,
		notifyChannel: make(chan struct{}, 100),
	}
}

// FallbackResult contains the result of a fallback decision.
type FallbackResult struct {
	// Daemon selected for the job (may be nil if rejected/timeout)
	Daemon *types.Daemon

	// UsedFallback indicates if software encoding fallback was used
	UsedFallback bool

	// WaitedForGPU indicates if the request waited in queue
	WaitedForGPU bool

	// Error if the request could not be fulfilled
	Error error
}

// HandleGPUExhausted handles a request when GPU sessions are exhausted.
// It applies the configured policy and returns the appropriate result.
func (h *GPUFallbackHandler) HandleGPUExhausted(
	ctx context.Context,
	criteria SelectionCriteria,
	strategy SelectionStrategy,
) FallbackResult {
	switch h.policy {
	case GPUPolicyFallback:
		return h.handleFallback(criteria, strategy)
	case GPUPolicyQueue:
		return h.handleQueue(ctx, criteria, strategy)
	case GPUPolicyReject:
		return h.handleReject()
	default:
		return h.handleFallback(criteria, strategy)
	}
}

// handleFallback attempts to fall back to software encoding.
func (h *GPUFallbackHandler) handleFallback(criteria SelectionCriteria, strategy SelectionStrategy) FallbackResult {
	// Modify criteria to not require GPU and use software encoder
	fallbackCriteria := criteria
	fallbackCriteria.RequireGPU = false

	// Map hardware encoder to software fallback
	if criteria.RequiredEncoder != "" {
		fallbackEncoder := h.getSoftwareFallbackEncoder(criteria.RequiredEncoder)
		if fallbackEncoder != "" {
			fallbackCriteria.RequiredEncoder = fallbackEncoder
		}
	}

	daemons := h.registry.GetAvailable()
	daemon := strategy.Select(daemons, fallbackCriteria)

	if daemon == nil {
		h.logger.Warn("GPU fallback failed - no software encoder available",
			slog.String("original_encoder", criteria.RequiredEncoder),
			slog.String("fallback_encoder", fallbackCriteria.RequiredEncoder),
		)
		return FallbackResult{
			Error: fmt.Errorf("no software encoder fallback available"),
		}
	}

	h.logger.Info("GPU exhausted - falling back to software encoding",
		slog.String("daemon_id", string(daemon.ID)),
		slog.String("original_encoder", criteria.RequiredEncoder),
		slog.String("fallback_encoder", fallbackCriteria.RequiredEncoder),
	)

	return FallbackResult{
		Daemon:       daemon,
		UsedFallback: true,
	}
}

// handleQueue waits for a GPU session to become available.
func (h *GPUFallbackHandler) handleQueue(
	ctx context.Context,
	criteria SelectionCriteria,
	strategy SelectionStrategy,
) FallbackResult {
	h.mu.Lock()
	if len(h.waitQueue) >= h.maxQueueSize {
		h.mu.Unlock()
		h.logger.Warn("GPU queue full - rejecting request",
			slog.Int("queue_size", h.maxQueueSize),
		)
		return FallbackResult{
			Error: fmt.Errorf("GPU session queue full (max %d)", h.maxQueueSize),
		}
	}

	// Create a channel to receive notification when a daemon becomes available
	waitChan := make(chan *types.Daemon, 1)
	h.waitQueue = append(h.waitQueue, waitChan)
	queuePosition := len(h.waitQueue)
	h.mu.Unlock()

	h.logger.Debug("request queued for GPU session",
		slog.Int("queue_position", queuePosition),
	)

	// Wait for either:
	// 1. Context cancellation
	// 2. Queue timeout
	// 3. GPU session becoming available

	timer := time.NewTimer(h.queueTimeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			h.removeFromQueue(waitChan)
			return FallbackResult{
				Error: ctx.Err(),
			}

		case <-timer.C:
			h.removeFromQueue(waitChan)
			h.logger.Warn("GPU queue timeout",
				slog.Duration("timeout", h.queueTimeout),
			)
			return FallbackResult{
				Error: fmt.Errorf("GPU session wait timeout after %s", h.queueTimeout),
			}

		case daemon := <-waitChan:
			if daemon != nil {
				h.logger.Info("GPU session became available from queue",
					slog.String("daemon_id", string(daemon.ID)),
				)
				return FallbackResult{
					Daemon:       daemon,
					WaitedForGPU: true,
				}
			}

		case <-h.notifyChannel:
			// Check if GPU is now available
			daemons := h.registry.GetWithAvailableGPU()
			daemon := strategy.Select(daemons, criteria)
			if daemon != nil {
				h.removeFromQueue(waitChan)
				h.logger.Info("GPU session became available",
					slog.String("daemon_id", string(daemon.ID)),
				)
				return FallbackResult{
					Daemon:       daemon,
					WaitedForGPU: true,
				}
			}
		}
	}
}

// handleReject immediately rejects the request.
func (h *GPUFallbackHandler) handleReject() FallbackResult {
	h.logger.Warn("GPU sessions exhausted - rejecting request (policy: reject)")
	return FallbackResult{
		Error: fmt.Errorf("GPU sessions exhausted and reject policy is configured"),
	}
}

// NotifyGPUAvailable notifies waiting requests that GPU sessions may be available.
// Call this when a GPU session is released.
func (h *GPUFallbackHandler) NotifyGPUAvailable() {
	select {
	case h.notifyChannel <- struct{}{}:
	default:
		// Channel full, notification will be processed
	}
}

// removeFromQueue removes a wait channel from the queue.
func (h *GPUFallbackHandler) removeFromQueue(waitChan chan *types.Daemon) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for i, ch := range h.waitQueue {
		if ch == waitChan {
			h.waitQueue = append(h.waitQueue[:i], h.waitQueue[i+1:]...)
			close(waitChan)
			return
		}
	}
}

// QueueSize returns the current number of requests waiting for GPU sessions.
func (h *GPUFallbackHandler) QueueSize() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.waitQueue)
}

// Policy returns the configured policy.
func (h *GPUFallbackHandler) Policy() GPUExhaustedPolicy {
	return h.policy
}

// getSoftwareFallbackEncoder returns the software encoder to use as fallback
// for a given hardware encoder.
func (h *GPUFallbackHandler) getSoftwareFallbackEncoder(hwEncoder string) string {
	// Map hardware encoders to their software equivalents
	fallbacks := map[string]string{
		// NVIDIA NVENC
		"h264_nvenc": "libx264",
		"hevc_nvenc": "libx265",
		"av1_nvenc":  "libaom-av1",

		// Intel QuickSync (QSV)
		"h264_qsv": "libx264",
		"hevc_qsv": "libx265",
		"av1_qsv":  "libaom-av1",
		"vp9_qsv":  "libvpx-vp9",

		// Intel VA-API
		"h264_vaapi": "libx264",
		"hevc_vaapi": "libx265",
		"vp9_vaapi":  "libvpx-vp9",
		"av1_vaapi":  "libaom-av1",

		// AMD AMF
		"h264_amf": "libx264",
		"hevc_amf": "libx265",

		// Apple VideoToolbox
		"h264_videotoolbox": "libx264",
		"hevc_videotoolbox": "libx265",

		// NVIDIA CUVID (decode to encode mapping)
		"h264_cuvid": "libx264",
		"hevc_cuvid": "libx265",
	}

	if fallback, ok := fallbacks[hwEncoder]; ok {
		return fallback
	}

	return "" // No known fallback
}

// SelectWithFallback selects a daemon, handling GPU exhaustion according to policy.
// This is a convenience method that combines daemon selection with fallback handling.
func (h *GPUFallbackHandler) SelectWithFallback(
	ctx context.Context,
	criteria SelectionCriteria,
	strategy SelectionStrategy,
) FallbackResult {
	// First, try to select a daemon normally
	daemons := h.registry.GetAvailable()
	daemon := strategy.Select(daemons, criteria)

	if daemon != nil {
		return FallbackResult{
			Daemon: daemon,
		}
	}

	// If GPU was required and we didn't find a daemon, check if it's due to GPU exhaustion
	if criteria.RequireGPU {
		// Check if there are daemons with the encoder but exhausted GPU sessions
		for _, d := range daemons {
			if d.Capabilities != nil && d.Capabilities.HasEncoder(criteria.RequiredEncoder) {
				// Found a daemon with the encoder but GPU exhausted
				h.logger.Info("GPU sessions exhausted, applying fallback policy",
					slog.String("policy", string(h.policy)),
					slog.String("encoder", criteria.RequiredEncoder),
				)
				return h.HandleGPUExhausted(ctx, criteria, strategy)
			}
		}
	}

	// No suitable daemon found at all
	return FallbackResult{
		Error: fmt.Errorf("no daemon available for encoder: %s", criteria.RequiredEncoder),
	}
}
