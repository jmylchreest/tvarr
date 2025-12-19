package relay

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jmylchreest/tvarr/pkg/ffmpegd/types"
)

func TestStrategyCapabilityMatch(t *testing.T) {
	strategy := NewStrategyCapabilityMatch()

	t.Run("selects_daemon_with_required_encoder", func(t *testing.T) {
		daemons := []*types.Daemon{
			makeDaemon("d1", []string{"libx264"}, 0, 2),
			makeDaemon("d2", []string{"libx264", "h264_nvenc"}, 0, 2),
			makeDaemon("d3", []string{"libx265"}, 0, 2),
		}

		criteria := SelectionCriteria{RequiredEncoder: "h264_nvenc"}
		selected := strategy.Select(daemons, criteria)

		require.NotNil(t, selected)
		assert.Equal(t, types.DaemonID("d2"), selected.ID)
	})

	t.Run("returns_nil_when_no_encoder_match", func(t *testing.T) {
		daemons := []*types.Daemon{
			makeDaemon("d1", []string{"libx264"}, 0, 2),
			makeDaemon("d2", []string{"libx265"}, 0, 2),
		}

		criteria := SelectionCriteria{RequiredEncoder: "h264_nvenc"}
		selected := strategy.Select(daemons, criteria)

		assert.Nil(t, selected)
	})

	t.Run("selects_least_loaded_among_matches", func(t *testing.T) {
		daemons := []*types.Daemon{
			makeDaemon("d1", []string{"libx264"}, 2, 4), // 50% load
			makeDaemon("d2", []string{"libx264"}, 1, 4), // 25% load
			makeDaemon("d3", []string{"libx264"}, 3, 4), // 75% load
		}

		criteria := SelectionCriteria{RequiredEncoder: "libx264"}
		selected := strategy.Select(daemons, criteria)

		require.NotNil(t, selected)
		assert.Equal(t, types.DaemonID("d2"), selected.ID)
	})

	t.Run("filters_by_gpu_requirement", func(t *testing.T) {
		d1 := makeDaemonWithGPU("d1", []string{"h264_nvenc"}, 3, 3) // No available sessions
		d2 := makeDaemonWithGPU("d2", []string{"h264_nvenc"}, 1, 3) // Has available sessions
		daemons := []*types.Daemon{d1, d2}

		criteria := SelectionCriteria{
			RequiredEncoder: "h264_nvenc",
			RequireGPU:      true,
		}
		selected := strategy.Select(daemons, criteria)

		require.NotNil(t, selected)
		assert.Equal(t, types.DaemonID("d2"), selected.ID)
	})

	t.Run("respects_max_cpu_percent", func(t *testing.T) {
		d1 := makeDaemonWithStats("d1", []string{"libx264"}, 90.0, 8000000000) // 90% CPU
		d2 := makeDaemonWithStats("d2", []string{"libx264"}, 40.0, 8000000000) // 40% CPU
		daemons := []*types.Daemon{d1, d2}

		criteria := SelectionCriteria{
			RequiredEncoder: "libx264",
			MaxCPUPercent:   50.0,
		}
		selected := strategy.Select(daemons, criteria)

		require.NotNil(t, selected)
		assert.Equal(t, types.DaemonID("d2"), selected.ID)
	})

	t.Run("respects_min_memory_available", func(t *testing.T) {
		d1 := makeDaemonWithStats("d1", []string{"libx264"}, 50.0, 4000000000)  // 4GB available
		d2 := makeDaemonWithStats("d2", []string{"libx264"}, 50.0, 12000000000) // 12GB available
		daemons := []*types.Daemon{d1, d2}

		criteria := SelectionCriteria{
			RequiredEncoder:    "libx264",
			MinMemoryAvailable: 8000000000, // 8GB required
		}
		selected := strategy.Select(daemons, criteria)

		require.NotNil(t, selected)
		assert.Equal(t, types.DaemonID("d2"), selected.ID)
	})
}

func TestStrategyLeastLoaded(t *testing.T) {
	strategy := NewStrategyLeastLoaded()

	t.Run("selects_daemon_with_lowest_load", func(t *testing.T) {
		daemons := []*types.Daemon{
			makeDaemon("d1", []string{"libx264"}, 3, 4), // 75% load
			makeDaemon("d2", []string{"libx264"}, 1, 4), // 25% load
			makeDaemon("d3", []string{"libx264"}, 2, 4), // 50% load
		}

		criteria := SelectionCriteria{}
		selected := strategy.Select(daemons, criteria)

		require.NotNil(t, selected)
		assert.Equal(t, types.DaemonID("d2"), selected.ID)
	})

	t.Run("still_filters_by_encoder_requirement", func(t *testing.T) {
		daemons := []*types.Daemon{
			makeDaemon("d1", []string{"libx265"}, 0, 4),        // 0% load but wrong encoder
			makeDaemon("d2", []string{"libx264"}, 1, 4),        // 25% load
			makeDaemon("d3", []string{"libx264", "libx265"}, 2, 4), // 50% load
		}

		criteria := SelectionCriteria{RequiredEncoder: "libx264"}
		selected := strategy.Select(daemons, criteria)

		require.NotNil(t, selected)
		assert.Equal(t, types.DaemonID("d2"), selected.ID)
	})
}

func TestStrategyGPUAware(t *testing.T) {
	strategy := NewStrategyGPUAware()

	t.Run("prefers_more_available_gpu_sessions", func(t *testing.T) {
		d1 := makeDaemonWithGPU("d1", []string{"h264_nvenc"}, 2, 3) // 1 available
		d2 := makeDaemonWithGPU("d2", []string{"h264_nvenc"}, 0, 3) // 3 available
		d3 := makeDaemonWithGPU("d3", []string{"h264_nvenc"}, 1, 3) // 2 available
		daemons := []*types.Daemon{d1, d2, d3}

		criteria := SelectionCriteria{RequiredEncoder: "h264_nvenc"}
		selected := strategy.Select(daemons, criteria)

		require.NotNil(t, selected)
		assert.Equal(t, types.DaemonID("d2"), selected.ID)
	})

	t.Run("returns_nil_when_require_gpu_and_none_available", func(t *testing.T) {
		d1 := makeDaemonWithGPU("d1", []string{"h264_nvenc"}, 3, 3) // No available
		d2 := makeDaemonWithGPU("d2", []string{"h264_nvenc"}, 3, 3) // No available
		daemons := []*types.Daemon{d1, d2}

		criteria := SelectionCriteria{
			RequiredEncoder: "h264_nvenc",
			RequireGPU:      true,
		}
		selected := strategy.Select(daemons, criteria)

		assert.Nil(t, selected)
	})

	t.Run("uses_job_load_as_tiebreaker", func(t *testing.T) {
		d1 := makeDaemonWithGPU("d1", []string{"h264_nvenc"}, 1, 3) // 2 available, check load
		d1.ActiveJobs = 2
		d1.Capabilities.MaxConcurrentJobs = 4 // 50% load

		d2 := makeDaemonWithGPU("d2", []string{"h264_nvenc"}, 1, 3) // 2 available, check load
		d2.ActiveJobs = 1
		d2.Capabilities.MaxConcurrentJobs = 4 // 25% load

		daemons := []*types.Daemon{d1, d2}

		criteria := SelectionCriteria{RequiredEncoder: "h264_nvenc"}
		selected := strategy.Select(daemons, criteria)

		require.NotNil(t, selected)
		assert.Equal(t, types.DaemonID("d2"), selected.ID)
	})
}

func TestStrategyRoundRobin(t *testing.T) {
	strategy := NewStrategyRoundRobin()

	t.Run("cycles_through_daemons", func(t *testing.T) {
		daemons := []*types.Daemon{
			makeDaemon("a", []string{"libx264"}, 0, 2),
			makeDaemon("b", []string{"libx264"}, 0, 2),
			makeDaemon("c", []string{"libx264"}, 0, 2),
		}

		criteria := SelectionCriteria{}

		// Should cycle through a, b, c, a, b, c...
		selected1 := strategy.Select(daemons, criteria)
		selected2 := strategy.Select(daemons, criteria)
		selected3 := strategy.Select(daemons, criteria)
		selected4 := strategy.Select(daemons, criteria)

		require.NotNil(t, selected1)
		require.NotNil(t, selected2)
		require.NotNil(t, selected3)
		require.NotNil(t, selected4)

		// Verify cycling (exact order depends on sort)
		assert.NotEqual(t, selected1.ID, selected2.ID)
		assert.NotEqual(t, selected2.ID, selected3.ID)
		assert.Equal(t, selected1.ID, selected4.ID) // Wraps around
	})
}

func TestStrategyAffinity(t *testing.T) {
	strategy := NewStrategyAffinity(NewStrategyCapabilityMatch())

	t.Run("prefers_specified_daemon", func(t *testing.T) {
		daemons := []*types.Daemon{
			makeDaemon("d1", []string{"libx264"}, 0, 4), // Less loaded
			makeDaemon("d2", []string{"libx264"}, 2, 4), // More loaded but preferred
		}

		criteria := SelectionCriteria{
			RequiredEncoder:  "libx264",
			PreferredDaemons: []types.DaemonID{"d2"},
		}
		selected := strategy.Select(daemons, criteria)

		require.NotNil(t, selected)
		assert.Equal(t, types.DaemonID("d2"), selected.ID)
	})

	t.Run("falls_back_when_preferred_unavailable", func(t *testing.T) {
		d1 := makeDaemon("d1", []string{"libx264"}, 0, 4)
		d2 := makeDaemon("d2", []string{"libx264"}, 0, 4)
		d2.State = types.DaemonStateUnhealthy // Can't accept jobs

		daemons := []*types.Daemon{d1, d2}

		criteria := SelectionCriteria{
			RequiredEncoder:  "libx264",
			PreferredDaemons: []types.DaemonID{"d2"},
		}
		selected := strategy.Select(daemons, criteria)

		require.NotNil(t, selected)
		assert.Equal(t, types.DaemonID("d1"), selected.ID)
	})

	t.Run("falls_back_when_preferred_lacks_capability", func(t *testing.T) {
		daemons := []*types.Daemon{
			makeDaemon("d1", []string{"h264_nvenc"}, 0, 4),
			makeDaemon("d2", []string{"libx264"}, 0, 4), // Preferred but wrong encoder
		}

		criteria := SelectionCriteria{
			RequiredEncoder:  "h264_nvenc",
			PreferredDaemons: []types.DaemonID{"d2"},
		}
		selected := strategy.Select(daemons, criteria)

		require.NotNil(t, selected)
		assert.Equal(t, types.DaemonID("d1"), selected.ID)
	})
}

func TestStrategyChain(t *testing.T) {
	t.Run("tries_strategies_in_order", func(t *testing.T) {
		// First strategy returns nil
		// Second strategy returns a daemon
		daemons := []*types.Daemon{
			makeDaemon("d1", []string{"libx264"}, 0, 2),
		}

		chain := NewStrategyChain(
			&mockStrategy{returns: nil},
			&mockStrategy{returns: daemons[0]},
		)

		selected := chain.Select(daemons, SelectionCriteria{})
		require.NotNil(t, selected)
		assert.Equal(t, types.DaemonID("d1"), selected.ID)
	})

	t.Run("returns_nil_when_all_fail", func(t *testing.T) {
		daemons := []*types.Daemon{
			makeDaemon("d1", []string{"libx264"}, 0, 2),
		}

		chain := NewStrategyChain(
			&mockStrategy{returns: nil},
			&mockStrategy{returns: nil},
		)

		selected := chain.Select(daemons, SelectionCriteria{})
		assert.Nil(t, selected)
	})
}

func TestDefaultSelectionStrategy(t *testing.T) {
	strategy := DefaultSelectionStrategy()
	assert.NotNil(t, strategy)
	assert.Equal(t, "chain", strategy.Name())
}

// -----------------------------------------------------------------------------
// Helper Functions
// -----------------------------------------------------------------------------

func makeDaemon(id string, encoders []string, activeJobs, maxJobs int) *types.Daemon {
	return &types.Daemon{
		ID:         types.DaemonID(id),
		Name:       "Daemon " + id,
		State:      types.DaemonStateConnected,
		ActiveJobs: activeJobs,
		Capabilities: &types.Capabilities{
			VideoEncoders:     encoders,
			MaxConcurrentJobs: maxJobs,
		},
	}
}

func makeDaemonWithGPU(id string, encoders []string, activeSessions, maxSessions int) *types.Daemon {
	d := makeDaemon(id, encoders, 0, 4)
	d.Capabilities.GPUs = []types.GPUInfo{
		{
			Index:                0,
			Name:                 "Test GPU",
			MaxEncodeSessions:    maxSessions,
			ActiveEncodeSessions: activeSessions,
		},
	}
	return d
}

func makeDaemonWithStats(id string, encoders []string, cpuPercent float64, memAvailable uint64) *types.Daemon {
	d := makeDaemon(id, encoders, 0, 4)
	d.SystemStats = &types.SystemStats{
		CPUPercent:      cpuPercent,
		MemoryAvailable: memAvailable,
	}
	return d
}

type mockStrategy struct {
	returns *types.Daemon
}

func (m *mockStrategy) Name() string {
	return "mock"
}

func (m *mockStrategy) Select(daemons []*types.Daemon, criteria SelectionCriteria) *types.Daemon {
	return m.returns
}
