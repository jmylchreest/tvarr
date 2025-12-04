// Package numbering implements the channel numbering pipeline stage.
//
// The numbering algorithm follows a two-pass approach:
//  1. First pass: Collect all channels with explicit ChannelNumber values (set via data mapping rules).
//     If multiple channels have the same number, resolve conflicts by incrementing to the next available.
//  2. Second pass: Assign sequential numbers from StartingChannelNumber to remaining unnumbered channels.
//
// This ensures channels with explicit numbers keep them (or get the nearest available),
// while unnumbered channels fill in gaps starting from the configured starting number.
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

// New creates a new numbering stage with preserve mode (default).
func New() *Stage {
	return &Stage{
		BaseStage: shared.NewBaseStage(StageID, StageName),
		mode:      NumberingModePreserve, // Default to preserve mode
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
// This follows the m3u-proxy algorithm:
//  1. First pass: Claim all explicit ChannelNumber > 0 values. If conflict, increment to next available.
//  2. Build available number pool from StartingChannelNumber.
//  3. Second pass: Assign sequential numbers from pool to channels with ChannelNumber == 0.
//
// The key difference from simple sequential: channels with explicit numbers get priority and keep
// their numbers (or nearest available), while unnumbered channels fill in from StartingChannelNumber.
func (s *Stage) assignPreserving(channels []*models.Channel, startNum int) int {
	// Track which numbers are already claimed
	usedNumbers := make(map[int]bool)

	// Track channels that need assignment and their resolved numbers
	// If resolvedNum is nil, channel needs sequential assignment from pool
	// If resolvedNum is set, channel had a conflict and was already resolved
	type channelAssignment struct {
		index       int
		resolvedNum *int // nil means needs sequential assignment, non-nil means conflict was resolved
	}
	channelsNeedingNumbers := make([]channelAssignment, 0)

	channelsWithExplicit := 0
	conflictsResolved := 0

	// First pass: collect existing ChannelNumber values and handle conflicts
	// This mirrors m3u-proxy's first pass (lines 292-339)
	for i, ch := range channels {
		if ch.ChannelNumber > 0 {
			channelsWithExplicit++
			desiredNum := ch.ChannelNumber
			originalNum := desiredNum

			// Try to use the desired number, or increment until we find an available one
			for usedNumbers[desiredNum] {
				desiredNum++
				conflictsResolved++
			}

			// Claim the resolved number
			usedNumbers[desiredNum] = true

			// If number was changed due to conflict, track it for later assignment
			if desiredNum != originalNum {
				if s.logger != nil {
					s.logger.Warn("channel number conflict resolved",
						"channel", ch.ChannelName,
						"original_number", originalNum,
						"assigned_number", desiredNum)
				}

				// Track the conflict resolution
				s.conflicts = append(s.conflicts, ConflictResolution{
					ChannelName:    ch.ChannelName,
					OriginalNumber: originalNum,
					AssignedNumber: desiredNum,
				})

				// Mark for later assignment (conflict resolved)
				resolvedNum := desiredNum
				channelsNeedingNumbers = append(channelsNeedingNumbers, channelAssignment{
					index:       i,
					resolvedNum: &resolvedNum,
				})
			}
			// If number didn't change, channel already has correct number, no action needed
		} else {
			// Channel needs a number assigned - mark for sequential assignment
			channelsNeedingNumbers = append(channelsNeedingNumbers, channelAssignment{
				index:       i,
				resolvedNum: nil, // needs sequential assignment
			})
		}
	}

	// Build available number pool from StartingChannelNumber
	// Only count channels that need sequential assignment (not conflict-resolved ones)
	sequentialNeeded := 0
	for _, ca := range channelsNeedingNumbers {
		if ca.resolvedNum == nil {
			sequentialNeeded++
		}
	}

	// Create pool of available numbers starting from startNum
	// We need at least sequentialNeeded numbers, but some might be taken by explicit numbers
	availableNumbers := make([]int, 0, sequentialNeeded)
	num := startNum
	for len(availableNumbers) < sequentialNeeded {
		if !usedNumbers[num] {
			availableNumbers = append(availableNumbers, num)
		}
		num++
	}

	// Second pass: assign numbers to channels that need them
	modified := 0
	availableIdx := 0

	for _, ca := range channelsNeedingNumbers {
		ch := channels[ca.index]

		if ca.resolvedNum != nil {
			// Conflict was resolved - assign the pre-resolved number
			ch.ChannelNumber = *ca.resolvedNum
			modified++
		} else {
			// Needs sequential assignment from pool
			if availableIdx < len(availableNumbers) {
				ch.ChannelNumber = availableNumbers[availableIdx]
				usedNumbers[ch.ChannelNumber] = true
				availableIdx++
				modified++
			}
		}
	}

	// Log summary
	if s.logger != nil {
		s.logger.Debug("numbering analysis",
			"channels_with_explicit", channelsWithExplicit,
			"conflicts_resolved", conflictsResolved,
			"sequential_assigned", availableIdx,
			"total_modified", modified)
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
