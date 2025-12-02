package core

import (
	"errors"
	"fmt"
)

// Pipeline errors.
var (
	// ErrNoSources indicates no stream sources were configured.
	ErrNoSources = errors.New("no stream sources configured")

	// ErrNoChannels indicates no channels were loaded.
	ErrNoChannels = errors.New("no channels loaded")

	// ErrPipelineAlreadyRunning indicates a pipeline is already executing for this proxy.
	ErrPipelineAlreadyRunning = errors.New("pipeline already running for this proxy")

	// ErrStageNotFound indicates a requested stage was not found.
	ErrStageNotFound = errors.New("stage not found")

	// ErrInvalidConfiguration indicates invalid pipeline configuration.
	ErrInvalidConfiguration = errors.New("invalid pipeline configuration")
)

// StageError wraps an error with stage context.
type StageError struct {
	StageID   string
	StageName string
	Err       error
}

// Error implements the error interface.
func (e *StageError) Error() string {
	return fmt.Sprintf("stage %s (%s): %v", e.StageName, e.StageID, e.Err)
}

// Unwrap returns the underlying error.
func (e *StageError) Unwrap() error {
	return e.Err
}

// NewStageError creates a new StageError.
func NewStageError(stageID, stageName string, err error) *StageError {
	return &StageError{
		StageID:   stageID,
		StageName: stageName,
		Err:       err,
	}
}

// ConfigurationError represents a configuration problem.
type ConfigurationError struct {
	Field   string
	Message string
}

// Error implements the error interface.
func (e *ConfigurationError) Error() string {
	return fmt.Sprintf("configuration error for %s: %s", e.Field, e.Message)
}

// NewConfigurationError creates a new ConfigurationError.
func NewConfigurationError(field, message string) *ConfigurationError {
	return &ConfigurationError{
		Field:   field,
		Message: message,
	}
}
