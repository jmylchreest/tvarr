// Package numbering implements the channel numbering pipeline stage.
package numbering

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/jmylchreest/tvarr/internal/pipeline/shared"
)

const (
	// StageID is the unique identifier for this stage.
	StageID = "numbering"
	// StageName is the human-readable name for this stage.
	StageName = "Channel Numbering"
)

// NumberingMode is an alias for models.NumberingMode for backwards compatibility.
type NumberingMode = models.NumberingMode

// Mode constants for backwards compatibility.
const (
	NumberingModeSequential = models.NumberingModeSequential
	NumberingModePreserve   = models.NumberingModePreserve
	NumberingModeGroup      = models.NumberingModeGroup
)

// ConflictResolution represents how a numbering conflict was resolved.
type ConflictResolution struct {
	ChannelName    string
	OriginalNumber int
	AssignedNumber int
}

// Stage assigns channel numbers to channels.
type Stage struct {
	shared.BaseStage
	mode      NumberingMode
	groupSize int // Size of each group range (default 100)
	logger    *slog.Logger
	conflicts []ConflictResolution
}

// New creates a new numbering stage with sequential mode.
func New() *Stage {
	return &Stage{
		BaseStage: shared.NewBaseStage(StageID, StageName),
		mode:      NumberingModeSequential,
		groupSize: 100,
		conflicts: make([]ConflictResolution, 0),
	}
}

// NewConstructor returns a stage constructor for use with the factory.
func NewConstructor() core.StageConstructor {
	return func(deps *core.Dependencies) core.Stage {
		s := New()
		if deps.Logger != nil {
			s.logger = deps.Logger.With("stage", StageID)
		}
		return s
	}
}

// WithMode sets the numbering mode.
func (s *Stage) WithMode(mode NumberingMode) *Stage {
	s.mode = mode
	return s
}

// WithGroupSize sets the group size for group numbering mode.
func (s *Stage) WithGroupSize(size int) *Stage {
	if size > 0 {
		s.groupSize = size
	}
	return s
}

// GetConflicts returns the conflicts resolved during the last execution.
func (s *Stage) GetConflicts() []ConflictResolution {
	return s.conflicts
}

// Execute assigns channel numbers to all channels.
func (s *Stage) Execute(ctx context.Context, state *core.State) (*core.StageResult, error) {
	result := shared.NewResult()

	// Reset conflicts for this execution
	s.conflicts = make([]ConflictResolution, 0)

	if len(state.Channels) == 0 {
		s.log(ctx, slog.LevelInfo, "no channels to number, skipping")
		result.Message = "No channels to number"
		return result, nil
	}

	// T033: Log stage start
	s.log(ctx, slog.LevelInfo, "starting channel numbering",
		slog.Int("channel_count", len(state.Channels)),
		slog.String("mode", string(s.mode)))

	startingNumber := state.Proxy.StartingChannelNumber
	if startingNumber <= 0 {
		startingNumber = 1
	}

	// Determine the numbering mode - use proxy config if set, otherwise use stage default
	mode := s.mode
	if state.Proxy.NumberingMode != "" {
		mode = NumberingMode(state.Proxy.NumberingMode)
	}

	// Determine the group size - use proxy config if set, otherwise use stage default
	groupSize := s.groupSize
	if state.Proxy.GroupNumberingSize > 0 {
		groupSize = state.Proxy.GroupNumberingSize
	}

	var numberedCount int

	switch mode {
	case NumberingModeSequential:
		numberedCount = s.assignSequential(state.Channels, startingNumber)

	case NumberingModePreserve:
		numberedCount = s.assignPreserving(state.Channels, startingNumber)

	case NumberingModeGroup:
		numberedCount = s.assignByGroup(state.Channels, startingNumber, groupSize)

	default:
		numberedCount = s.assignSequential(state.Channels, startingNumber)
	}

	result.RecordsProcessed = len(state.Channels)
	result.RecordsModified = numberedCount

	// Build result message including conflict info
	if len(s.conflicts) > 0 {
		result.Message = fmt.Sprintf("Numbered %d channels starting from %d (%d conflicts resolved)",
			numberedCount, startingNumber, len(s.conflicts))
	} else {
		result.Message = fmt.Sprintf("Numbered %d channels starting from %d", numberedCount, startingNumber)
	}

	// T033: Log stage completion with numbering stats
	s.log(ctx, slog.LevelInfo, "channel numbering complete",
		slog.Int("channels_numbered", numberedCount),
		slog.Int("starting_number", startingNumber),
		slog.String("mode", string(mode)),
		slog.Int("conflicts_resolved", len(s.conflicts)))

	// Create artifact with conflict metadata
	artifact := core.NewArtifact(core.ArtifactTypeChannels, core.ProcessingStageNumbered, StageID).
		WithRecordCount(len(state.Channels)).
		WithMetadata("starting_number", startingNumber).
		WithMetadata("mode", string(s.mode)).
		WithMetadata("conflicts_resolved", len(s.conflicts))
	result.Artifacts = append(result.Artifacts, artifact)

	return result, nil
}

// assignSequential assigns sequential channel numbers.
func (s *Stage) assignSequential(channels []*models.Channel, startNum int) int {
	num := startNum
	for _, ch := range channels {
		ch.ChannelNumber = num
		num++
	}
	return len(channels)
}

// assignPreserving keeps existing channel numbers where valid, resolving conflicts.
// When multiple channels have the same number, later channels get incremented.
func (s *Stage) assignPreserving(channels []*models.Channel, startNum int) int {
	// Track which numbers are already claimed and by which channel
	usedNumbers := make(map[int]bool)
	channelsNeedingNumbers := make([]int, 0)       // indices of channels that need assignment
	channelsWithConflicts := make(map[int]int, 0) // index -> original number (for conflict resolution)

	// First pass: identify conflicts and collect existing numbers
	for i, ch := range channels {
		if ch.ChannelNumber > 0 {
			originalNum := ch.ChannelNumber
			if usedNumbers[originalNum] {
				// Conflict detected - this number is already used
				channelsWithConflicts[i] = originalNum
			} else {
				// Claim this number
				usedNumbers[originalNum] = true
			}
		} else {
			// Channel needs a number assigned
			channelsNeedingNumbers = append(channelsNeedingNumbers, i)
		}
	}

	modified := 0

	// Sort conflict indices to ensure deterministic ordering
	conflictIndices := make([]int, 0, len(channelsWithConflicts))
	for idx := range channelsWithConflicts {
		conflictIndices = append(conflictIndices, idx)
	}
	sort.Ints(conflictIndices)

	// Second pass: resolve conflicts by finding next available number
	for _, idx := range conflictIndices {
		originalNum := channelsWithConflicts[idx]
		ch := channels[idx]
		newNum := originalNum

		// Find next available number starting from the original
		for usedNumbers[newNum] {
			newNum++
		}

		// Assign the resolved number
		ch.ChannelNumber = newNum
		usedNumbers[newNum] = true
		modified++

		// Track the conflict resolution
		conflict := ConflictResolution{
			ChannelName:    ch.ChannelName,
			OriginalNumber: originalNum,
			AssignedNumber: newNum,
		}
		s.conflicts = append(s.conflicts, conflict)

		// Log the conflict resolution
		if s.logger != nil {
			s.logger.Warn("channel number conflict resolved",
				"channel", ch.ChannelName,
				"original_number", originalNum,
				"assigned_number", newNum)
		}
	}

	// Third pass: assign numbers to channels without one
	nextNum := startNum
	for _, idx := range channelsNeedingNumbers {
		ch := channels[idx]

		// Find next available number
		for usedNumbers[nextNum] {
			nextNum++
		}

		ch.ChannelNumber = nextNum
		usedNumbers[nextNum] = true
		nextNum++
		modified++
	}

	return modified
}

// assignByGroup assigns channel numbers within group ranges.
func (s *Stage) assignByGroup(channels []*models.Channel, startNum int, groupSize int) int {
	// Group channels by GroupTitle
	groups := make(map[string][]*models.Channel)
	groupOrder := make([]string, 0)

	for _, ch := range channels {
		group := ch.GroupTitle
		if group == "" {
			group = "Uncategorized"
		}

		if _, exists := groups[group]; !exists {
			groupOrder = append(groupOrder, group)
		}
		groups[group] = append(groups[group], ch)
	}

	// Sort groups alphabetically for consistent ordering
	sort.Strings(groupOrder)

	// Assign numbers: each group gets a range based on groupSize
	modified := 0

	for i, groupName := range groupOrder {
		groupStart := startNum + (i * groupSize)
		num := groupStart

		for _, ch := range groups[groupName] {
			ch.ChannelNumber = num
			num++
			modified++
		}
	}

	return modified
}

// log logs a message if the logger is set.
func (s *Stage) log(ctx context.Context, level slog.Level, msg string, attrs ...any) {
	if s.logger != nil {
		s.logger.Log(ctx, level, msg, attrs...)
	}
}

// Ensure Stage implements core.Stage.
var _ core.Stage = (*Stage)(nil)
