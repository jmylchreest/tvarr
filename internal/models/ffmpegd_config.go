package models

import (
	"time"
)

// FFmpegdConfig stores coordinator configuration for the distributed transcoding system.
// This is a key-value table for storing settings like auth token, gRPC address, etc.
type FFmpegdConfig struct {
	Key       string    `gorm:"primaryKey;size:100" json:"key"`
	Value     string    `gorm:"type:text;not null" json:"value"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name for FFmpegdConfig.
func (FFmpegdConfig) TableName() string {
	return "ffmpegd_config"
}

// FFmpegdConfigKey constants for known configuration keys.
const (
	FFmpegdConfigKeyAuthToken        = "auth_token"
	FFmpegdConfigKeyListenAddress    = "listen_address"
	FFmpegdConfigKeySelectionStrategy = "selection_strategy"
	FFmpegdConfigKeyHeartbeatInterval = "heartbeat_interval"
	FFmpegdConfigKeyUnhealthyThreshold = "unhealthy_threshold"
)

// SelectionStrategy defines how daemons are selected for jobs.
type SelectionStrategy string

const (
	StrategyLeastLoaded     SelectionStrategy = "least-loaded"
	StrategyRoundRobin      SelectionStrategy = "round-robin"
	StrategyCapabilityMatch SelectionStrategy = "capability-match"
	StrategyGPUAware        SelectionStrategy = "gpu-aware"
)

// String implements fmt.Stringer.
func (s SelectionStrategy) String() string {
	return string(s)
}

// IsValid returns true if the strategy is a valid value.
func (s SelectionStrategy) IsValid() bool {
	switch s {
	case StrategyLeastLoaded, StrategyRoundRobin, StrategyCapabilityMatch, StrategyGPUAware:
		return true
	default:
		return false
	}
}
