package shared

import (
	"context"

	"github.com/jmylchreest/tvarr/internal/pipeline/core"
)

// BaseStage provides common functionality for pipeline stages.
// Embed this in stage implementations to get default behaviors.
type BaseStage struct {
	id   string
	name string
}

// NewBaseStage creates a new BaseStage.
func NewBaseStage(id, name string) BaseStage {
	return BaseStage{
		id:   id,
		name: name,
	}
}

// ID returns the stage identifier.
func (b *BaseStage) ID() string {
	return b.id
}

// Name returns the human-readable stage name.
func (b *BaseStage) Name() string {
	return b.name
}

// Cleanup provides a default no-op cleanup implementation.
func (b *BaseStage) Cleanup(ctx context.Context) error {
	return nil
}

// NewResult creates a new StageResult.
func NewResult() *core.StageResult {
	return &core.StageResult{
		Artifacts: make([]core.Artifact, 0),
	}
}
