package relay

import (
	"fmt"
	"log/slog"
	"sort"

	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
)

// SelectionCriteria specifies requirements for daemon selection.
type SelectionCriteria struct {
	// Required encoder (e.g., "h264_nvenc", "libx264")
	RequiredEncoder string

	// Required decoder (e.g., "h264_cuvid", "hevc")
	RequiredDecoder string

	// Source codec for HW transcode preference (e.g., "h264", "hevc")
	// Used to check if daemon has HW decoder for source
	SourceCodec string

	// Target codec for HW transcode preference (e.g., "h264", "hevc")
	// Used to check if daemon has HW encoder for target
	TargetCodec string

	// Required HW accel type (e.g., "nvenc", "vaapi", "qsv")
	RequiredHWAccel types.HWAccelType

	// Whether GPU encode sessions are required
	RequireGPU bool

	// Minimum available memory (bytes)
	MinMemoryAvailable uint64

	// Maximum acceptable CPU load (0-100)
	MaxCPUPercent float64

	// Preferred daemon IDs (for affinity)
	PreferredDaemons []types.DaemonID
}

// SelectionStrategy defines an interface for daemon selection algorithms.
type SelectionStrategy interface {
	// Select chooses the best daemon from available daemons based on criteria.
	// Returns nil if no suitable daemon is found.
	Select(daemons []*types.Daemon, criteria SelectionCriteria) *types.Daemon

	// Name returns the strategy name for logging/debugging.
	Name() string
}

// DaemonSelector provides high-level daemon selection using strategies.
type DaemonSelector struct {
	registry *DaemonRegistry
	strategy SelectionStrategy
	logger   *slog.Logger
}

// NewDaemonSelector creates a new daemon selector with the given strategy.
func NewDaemonSelector(registry *DaemonRegistry, strategy SelectionStrategy, logger *slog.Logger) *DaemonSelector {
	return &DaemonSelector{
		registry: registry,
		strategy: strategy,
		logger:   logger,
	}
}

// SelectDaemon selects the best daemon for the given criteria.
func (s *DaemonSelector) SelectDaemon(criteria SelectionCriteria) (*types.Daemon, error) {
	// Get all daemons that can accept jobs
	available := s.registry.GetAvailable()

	if len(available) == 0 {
		return nil, fmt.Errorf("no available daemons")
	}

	// Apply strategy
	daemon := s.strategy.Select(available, criteria)
	if daemon == nil {
		s.logger.Debug("no daemon matched criteria",
			slog.String("strategy", s.strategy.Name()),
			slog.String("required_encoder", criteria.RequiredEncoder),
			slog.String("required_decoder", criteria.RequiredDecoder),
			slog.Bool("require_gpu", criteria.RequireGPU),
		)
		return nil, fmt.Errorf("no daemon matches criteria")
	}

	s.logger.Debug("daemon selected",
		slog.String("strategy", s.strategy.Name()),
		slog.String("daemon_id", string(daemon.ID)),
		slog.String("daemon_name", daemon.Name),
	)

	return daemon, nil
}

// SetStrategy changes the selection strategy.
func (s *DaemonSelector) SetStrategy(strategy SelectionStrategy) {
	s.strategy = strategy
}

// -----------------------------------------------------------------------------
// Strategy Implementations
// -----------------------------------------------------------------------------

// StrategyCapabilityMatch selects daemons that have required capabilities.
// Among matching daemons, it picks the least loaded one.
type StrategyCapabilityMatch struct{}

func NewStrategyCapabilityMatch() *StrategyCapabilityMatch {
	return &StrategyCapabilityMatch{}
}

func (s *StrategyCapabilityMatch) Name() string {
	return "capability-match"
}

func (s *StrategyCapabilityMatch) Select(daemons []*types.Daemon, criteria SelectionCriteria) *types.Daemon {
	var candidates []*types.Daemon

	for _, d := range daemons {
		if !s.matchesCriteria(d, criteria) {
			continue
		}
		candidates = append(candidates, d)
	}

	if len(candidates) == 0 {
		return nil
	}

	// Select least loaded among candidates
	return selectLeastLoaded(candidates)
}

func (s *StrategyCapabilityMatch) matchesCriteria(d *types.Daemon, c SelectionCriteria) bool {
	if d.Capabilities == nil {
		return false
	}

	// Check required encoder
	if c.RequiredEncoder != "" && !d.Capabilities.HasEncoder(c.RequiredEncoder) {
		return false
	}

	// Check required decoder
	if c.RequiredDecoder != "" && !d.Capabilities.HasDecoder(c.RequiredDecoder) {
		return false
	}

	// Check required HW accel
	if c.RequiredHWAccel != "" && !d.Capabilities.HasHWAccel(c.RequiredHWAccel) {
		return false
	}

	// Check GPU requirement
	if c.RequireGPU && !d.HasAvailableGPUSessions() {
		return false
	}

	// Check memory requirement
	if c.MinMemoryAvailable > 0 && d.SystemStats != nil {
		if d.SystemStats.MemoryAvailable < c.MinMemoryAvailable {
			return false
		}
	}

	// Check CPU load
	if c.MaxCPUPercent > 0 && d.SystemStats != nil {
		if d.SystemStats.CPUPercent > c.MaxCPUPercent {
			return false
		}
	}

	return true
}

// StrategyLeastLoaded selects the daemon with the lowest job load.
type StrategyLeastLoaded struct{}

func NewStrategyLeastLoaded() *StrategyLeastLoaded {
	return &StrategyLeastLoaded{}
}

func (s *StrategyLeastLoaded) Name() string {
	return "least-loaded"
}

func (s *StrategyLeastLoaded) Select(daemons []*types.Daemon, criteria SelectionCriteria) *types.Daemon {
	// Filter by basic capability requirements first
	var filtered []*types.Daemon
	for _, d := range daemons {
		if d.Capabilities == nil {
			continue
		}
		if criteria.RequiredEncoder != "" && !d.Capabilities.HasEncoder(criteria.RequiredEncoder) {
			continue
		}
		filtered = append(filtered, d)
	}

	if len(filtered) == 0 {
		return nil
	}

	return selectLeastLoaded(filtered)
}

// StrategyGPUAware selects daemons with available GPU sessions.
// Prioritizes daemons with the most available GPU sessions.
type StrategyGPUAware struct{}

func NewStrategyGPUAware() *StrategyGPUAware {
	return &StrategyGPUAware{}
}

func (s *StrategyGPUAware) Name() string {
	return "gpu-aware"
}

func (s *StrategyGPUAware) Select(daemons []*types.Daemon, criteria SelectionCriteria) *types.Daemon {
	type scored struct {
		daemon         *types.Daemon
		availSessions  int
		totalSessions  int
		jobLoadPercent float64
	}

	var candidates []scored

	for _, d := range daemons {
		if d.Capabilities == nil {
			continue
		}

		// Check required encoder
		if criteria.RequiredEncoder != "" && !d.Capabilities.HasEncoder(criteria.RequiredEncoder) {
			continue
		}

		// Check required decoder
		if criteria.RequiredDecoder != "" && !d.Capabilities.HasDecoder(criteria.RequiredDecoder) {
			continue
		}

		// Calculate GPU session availability
		avail, total := d.GPUSessionAvailability()
		if criteria.RequireGPU && avail == 0 {
			continue
		}

		jobLoad := float64(0)
		if d.Capabilities.MaxConcurrentJobs > 0 {
			jobLoad = float64(d.ActiveJobs) / float64(d.Capabilities.MaxConcurrentJobs)
		}

		candidates = append(candidates, scored{
			daemon:         d,
			availSessions:  avail,
			totalSessions:  total,
			jobLoadPercent: jobLoad,
		})
	}

	if len(candidates) == 0 {
		return nil
	}

	// Sort by: most available sessions, then least loaded
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].availSessions != candidates[j].availSessions {
			return candidates[i].availSessions > candidates[j].availSessions
		}
		return candidates[i].jobLoadPercent < candidates[j].jobLoadPercent
	})

	return candidates[0].daemon
}

// StrategyFullHWTranscode prefers daemons that can do full hardware transcoding
// (HW decode of source + HW encode of target). This is optimal as frames stay on GPU.
// Falls back to HW encode only, then any capable daemon.
type StrategyFullHWTranscode struct{}

func NewStrategyFullHWTranscode() *StrategyFullHWTranscode {
	return &StrategyFullHWTranscode{}
}

func (s *StrategyFullHWTranscode) Name() string {
	return "full-hw-transcode"
}

func (s *StrategyFullHWTranscode) Select(daemons []*types.Daemon, criteria SelectionCriteria) *types.Daemon {
	// Need source and target codecs to evaluate HW transcode capability
	if criteria.SourceCodec == "" || criteria.TargetCodec == "" {
		return nil // Can't evaluate without codec info
	}

	type scored struct {
		daemon         *types.Daemon
		hasFullHW      bool // Has both HW decoder and HW encoder
		hasHWEncode    bool // Has HW encoder only
		availSessions  int
		jobLoadPercent float64
	}

	var candidates []scored

	for _, d := range daemons {
		if d.Capabilities == nil || !d.CanAcceptJobs() {
			continue
		}

		// Check required encoder if specified
		if criteria.RequiredEncoder != "" && !d.Capabilities.HasEncoder(criteria.RequiredEncoder) {
			continue
		}

		// Check required decoder if specified
		if criteria.RequiredDecoder != "" && !d.Capabilities.HasDecoder(criteria.RequiredDecoder) {
			continue
		}

		// Calculate GPU session availability
		avail, _ := d.GPUSessionAvailability()
		if criteria.RequireGPU && avail == 0 {
			continue
		}

		// Check HW capabilities using the helper functions
		hasFullHW := CanDoFullHWTranscode(criteria.SourceCodec, criteria.TargetCodec, d)
		hasHWEncode := HasHWEncoder(criteria.TargetCodec, d)

		jobLoad := float64(0)
		if d.Capabilities.MaxConcurrentJobs > 0 {
			jobLoad = float64(d.ActiveJobs) / float64(d.Capabilities.MaxConcurrentJobs)
		}

		candidates = append(candidates, scored{
			daemon:         d,
			hasFullHW:      hasFullHW,
			hasHWEncode:    hasHWEncode,
			availSessions:  avail,
			jobLoadPercent: jobLoad,
		})
	}

	if len(candidates) == 0 {
		return nil
	}

	// Sort by: full HW transcode > HW encode only > most sessions > least loaded
	sort.Slice(candidates, func(i, j int) bool {
		// Prefer full HW transcode
		if candidates[i].hasFullHW != candidates[j].hasFullHW {
			return candidates[i].hasFullHW
		}
		// Then prefer HW encode
		if candidates[i].hasHWEncode != candidates[j].hasHWEncode {
			return candidates[i].hasHWEncode
		}
		// Then prefer more available sessions
		if candidates[i].availSessions != candidates[j].availSessions {
			return candidates[i].availSessions > candidates[j].availSessions
		}
		// Finally, prefer least loaded
		return candidates[i].jobLoadPercent < candidates[j].jobLoadPercent
	})

	return candidates[0].daemon
}

// StrategyRoundRobin cycles through daemons in order.
// Maintains state across calls to distribute load evenly.
type StrategyRoundRobin struct {
	lastIndex int
}

func NewStrategyRoundRobin() *StrategyRoundRobin {
	return &StrategyRoundRobin{lastIndex: -1}
}

func (s *StrategyRoundRobin) Name() string {
	return "round-robin"
}

func (s *StrategyRoundRobin) Select(daemons []*types.Daemon, criteria SelectionCriteria) *types.Daemon {
	if len(daemons) == 0 {
		return nil
	}

	// Filter by capability requirements
	var filtered []*types.Daemon
	for _, d := range daemons {
		if d.Capabilities == nil {
			continue
		}
		if criteria.RequiredEncoder != "" && !d.Capabilities.HasEncoder(criteria.RequiredEncoder) {
			continue
		}
		if criteria.RequireGPU && !d.HasAvailableGPUSessions() {
			continue
		}
		filtered = append(filtered, d)
	}

	if len(filtered) == 0 {
		return nil
	}

	// Sort for consistent ordering
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].ID < filtered[j].ID
	})

	// Round robin
	s.lastIndex = (s.lastIndex + 1) % len(filtered)
	return filtered[s.lastIndex]
}

// StrategyAffinity prefers specific daemons but falls back to capability match.
type StrategyAffinity struct {
	fallback SelectionStrategy
}

func NewStrategyAffinity(fallback SelectionStrategy) *StrategyAffinity {
	if fallback == nil {
		fallback = NewStrategyCapabilityMatch()
	}
	return &StrategyAffinity{fallback: fallback}
}

func (s *StrategyAffinity) Name() string {
	return "affinity"
}

func (s *StrategyAffinity) Select(daemons []*types.Daemon, criteria SelectionCriteria) *types.Daemon {
	// Try preferred daemons first
	if len(criteria.PreferredDaemons) > 0 {
		for _, prefID := range criteria.PreferredDaemons {
			for _, d := range daemons {
				if d.ID == prefID && d.CanAcceptJobs() {
					// Verify capabilities match
					if criteria.RequiredEncoder == "" || (d.Capabilities != nil && d.Capabilities.HasEncoder(criteria.RequiredEncoder)) {
						return d
					}
				}
			}
		}
	}

	// Fall back to default strategy
	return s.fallback.Select(daemons, criteria)
}

// StrategyChain tries multiple strategies in order until one succeeds.
type StrategyChain struct {
	strategies []SelectionStrategy
}

func NewStrategyChain(strategies ...SelectionStrategy) *StrategyChain {
	return &StrategyChain{strategies: strategies}
}

func (s *StrategyChain) Name() string {
	return "chain"
}

func (s *StrategyChain) Select(daemons []*types.Daemon, criteria SelectionCriteria) *types.Daemon {
	for _, strategy := range s.strategies {
		if d := strategy.Select(daemons, criteria); d != nil {
			return d
		}
	}
	return nil
}

// -----------------------------------------------------------------------------
// Helper Functions
// -----------------------------------------------------------------------------

// selectLeastLoaded returns the daemon with the lowest job load.
func selectLeastLoaded(daemons []*types.Daemon) *types.Daemon {
	if len(daemons) == 0 {
		return nil
	}

	var selected *types.Daemon
	lowestLoad := float64(1.1) // Higher than any possible load

	for _, d := range daemons {
		if d.Capabilities == nil || d.Capabilities.MaxConcurrentJobs == 0 {
			continue
		}

		load := float64(d.ActiveJobs) / float64(d.Capabilities.MaxConcurrentJobs)
		if load < lowestLoad {
			lowestLoad = load
			selected = d
		}
	}

	return selected
}

// DefaultSelectionStrategy returns the recommended default strategy.
// Uses capability matching with GPU awareness for hardware encoders.
func DefaultSelectionStrategy() SelectionStrategy {
	return NewStrategyChain(
		NewStrategyFullHWTranscode(), // Prefer full HW transcode (decode+encode on GPU)
		NewStrategyGPUAware(),        // Then prefer GPU encode
		NewStrategyCapabilityMatch(), // Then capability match
		NewStrategyLeastLoaded(),     // Finally least loaded
	)
}

// TranscodingSelectionStrategy returns a strategy optimized for transcoding.
// Prefers full hardware transcode path (HW decode + HW encode) when available.
func TranscodingSelectionStrategy() SelectionStrategy {
	return NewStrategyChain(
		NewStrategyFullHWTranscode(),
		NewStrategyGPUAware(),
		NewStrategyCapabilityMatch(),
	)
}

// HWEncoderSelectionStrategy returns a strategy optimized for hardware encoding.
// Prioritizes GPU session availability to avoid session exhaustion.
func HWEncoderSelectionStrategy() SelectionStrategy {
	return NewStrategyGPUAware()
}

// SoftwareEncoderSelectionStrategy returns a strategy for software encoding.
// Prioritizes CPU load distribution.
func SoftwareEncoderSelectionStrategy() SelectionStrategy {
	return NewStrategyCapabilityMatch()
}

// ProbeSelectionStrategy returns the default strategy for probe operations.
// Probing is lightweight so we just pick the least loaded daemon.
func ProbeSelectionStrategy() SelectionStrategy {
	return NewStrategyLeastLoaded()
}

// ProbeStrategyProvider provides the selection strategy for probe operations.
// This abstraction allows for future configuration-based strategy selection.
type ProbeStrategyProvider interface {
	// GetProbeStrategy returns the strategy to use for daemon selection during probing.
	GetProbeStrategy() SelectionStrategy
}

// DefaultProbeStrategyProvider returns the default probe strategy (LeastLoaded).
// This can be replaced with a configurable provider in the future.
type DefaultProbeStrategyProvider struct{}

// NewDefaultProbeStrategyProvider creates a new default probe strategy provider.
func NewDefaultProbeStrategyProvider() *DefaultProbeStrategyProvider {
	return &DefaultProbeStrategyProvider{}
}

// GetProbeStrategy returns the LeastLoaded strategy for probing.
func (p *DefaultProbeStrategyProvider) GetProbeStrategy() SelectionStrategy {
	return ProbeSelectionStrategy()
}
