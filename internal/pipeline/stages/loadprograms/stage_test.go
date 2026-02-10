package loadprograms

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock repository ---

type mockProgramRepo struct {
	programs map[models.ULID][]*models.EpgProgram
	err      error // error to return from GetBySourceID / CountBySourceID
}

func (m *mockProgramRepo) GetBySourceID(ctx context.Context, sourceID models.ULID, callback func(*models.EpgProgram) error) error {
	if m.err != nil {
		return m.err
	}
	for _, prog := range m.programs[sourceID] {
		if err := callback(prog); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockProgramRepo) CountBySourceID(ctx context.Context, sourceID models.ULID) (int64, error) {
	if m.err != nil {
		return 0, m.err
	}
	return int64(len(m.programs[sourceID])), nil
}

// Unused interface methods — return nil/zero.

func (m *mockProgramRepo) Create(ctx context.Context, program *models.EpgProgram) error {
	return nil
}

func (m *mockProgramRepo) CreateBatch(ctx context.Context, programs []*models.EpgProgram) error {
	return nil
}

func (m *mockProgramRepo) GetByID(ctx context.Context, id models.ULID) (*models.EpgProgram, error) {
	return nil, nil
}

func (m *mockProgramRepo) GetByChannelID(ctx context.Context, channelID string, start, end time.Time) ([]*models.EpgProgram, error) {
	return nil, nil
}

func (m *mockProgramRepo) GetByChannelIDWithLimit(ctx context.Context, channelID string, limit int) ([]*models.EpgProgram, error) {
	return nil, nil
}

func (m *mockProgramRepo) GetCurrentByChannelID(ctx context.Context, channelID string) (*models.EpgProgram, error) {
	return nil, nil
}

func (m *mockProgramRepo) Delete(ctx context.Context, id models.ULID) error {
	return nil
}

func (m *mockProgramRepo) DeleteBySourceID(ctx context.Context, sourceID models.ULID) error {
	return nil
}

func (m *mockProgramRepo) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	return 0, nil
}

func (m *mockProgramRepo) DeleteOld(ctx context.Context) (int64, error) {
	return 0, nil
}

// --- Helpers ---

func boolPtr(b bool) *bool {
	return &b
}

func makeEpgSource(name string, enabled bool) *models.EpgSource {
	return &models.EpgSource{
		BaseModel: models.BaseModel{ID: models.NewULID()},
		Name:      name,
		Enabled:   boolPtr(enabled),
	}
}

func makeProgram(channelID, title string, start, stop time.Time) *models.EpgProgram {
	return &models.EpgProgram{
		BaseModel: models.BaseModel{ID: models.NewULID()},
		ChannelID: channelID,
		Title:     title,
		Start:     start,
		Stop:      stop,
	}
}

func newTestState(t *testing.T) *core.State {
	t.Helper()
	proxy := &models.StreamProxy{
		BaseModel: models.BaseModel{ID: models.NewULID()},
		Name:      "Test Proxy",
	}
	return core.NewState(proxy)
}

// --- Tests ---

func TestStage_Interface(t *testing.T) {
	stage := New(nil)
	assert.Equal(t, StageID, stage.ID())
	assert.Equal(t, StageName, stage.Name())
}

func TestNewConstructor(t *testing.T) {
	repo := &mockProgramRepo{}
	constructor := NewConstructor()
	stage := constructor(&core.Dependencies{EpgProgramRepo: repo})
	require.NotNil(t, stage)
	assert.Equal(t, StageID, stage.ID())
}

func TestStage_Execute(t *testing.T) {
	now := time.Now()
	future := now.Add(2 * time.Hour)
	past := now.Add(-2 * time.Hour)
	farFuture := now.Add(4 * time.Hour)

	t.Run("no EPG sources returns skip message", func(t *testing.T) {
		state := newTestState(t)
		state.EpgSources = nil
		state.ChannelMap = map[string]*models.Channel{
			"ch1": {TvgID: "ch1"},
		}

		stage := New(&mockProgramRepo{})
		result, err := stage.Execute(context.Background(), state)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Contains(t, result.Message, "No EPG sources or no channels")
		assert.Equal(t, 0, result.RecordsProcessed)
		assert.Empty(t, state.Programs)
	})

	t.Run("no channels in ChannelMap returns skip message", func(t *testing.T) {
		state := newTestState(t)
		src := makeEpgSource("src1", true)
		state.EpgSources = []*models.EpgSource{src}
		state.ChannelMap = map[string]*models.Channel{} // empty

		stage := New(&mockProgramRepo{})
		result, err := stage.Execute(context.Background(), state)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Contains(t, result.Message, "No EPG sources or no channels")
		assert.Equal(t, 0, result.RecordsProcessed)
	})

	t.Run("basic load matches programs to channel TvgIDs", func(t *testing.T) {
		src := makeEpgSource("src1", true)
		repo := &mockProgramRepo{
			programs: map[models.ULID][]*models.EpgProgram{
				src.ID: {
					makeProgram("ch1", "Show A", now, future),
					makeProgram("ch2", "Show B", now, future),
				},
			},
		}

		state := newTestState(t)
		state.EpgSources = []*models.EpgSource{src}
		state.ChannelMap = map[string]*models.Channel{
			"ch1": {TvgID: "ch1"},
			"ch2": {TvgID: "ch2"},
		}

		stage := New(repo)
		result, err := stage.Execute(context.Background(), state)

		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, 2, result.RecordsProcessed)
		assert.Len(t, state.Programs, 2)
	})

	t.Run("expired programs are skipped", func(t *testing.T) {
		src := makeEpgSource("src1", true)
		repo := &mockProgramRepo{
			programs: map[models.ULID][]*models.EpgProgram{
				src.ID: {
					makeProgram("ch1", "Expired Show", past, now.Add(-1*time.Minute)),
					makeProgram("ch1", "Future Show", now, future),
				},
			},
		}

		state := newTestState(t)
		state.EpgSources = []*models.EpgSource{src}
		state.ChannelMap = map[string]*models.Channel{
			"ch1": {TvgID: "ch1"},
		}

		stage := New(repo)
		result, err := stage.Execute(context.Background(), state)

		require.NoError(t, err)
		assert.Equal(t, 1, result.RecordsProcessed)
		require.Len(t, state.Programs, 1)
		assert.Equal(t, "Future Show", state.Programs[0].Title)
	})

	t.Run("programs for non-matching channels are skipped", func(t *testing.T) {
		src := makeEpgSource("src1", true)
		repo := &mockProgramRepo{
			programs: map[models.ULID][]*models.EpgProgram{
				src.ID: {
					makeProgram("ch1", "Matching", now, future),
					makeProgram("ch_unknown", "Not Matching", now, future),
				},
			},
		}

		state := newTestState(t)
		state.EpgSources = []*models.EpgSource{src}
		state.ChannelMap = map[string]*models.Channel{
			"ch1": {TvgID: "ch1"},
		}

		stage := New(repo)
		result, err := stage.Execute(context.Background(), state)

		require.NoError(t, err)
		assert.Equal(t, 1, result.RecordsProcessed)
		require.Len(t, state.Programs, 1)
		assert.Equal(t, "Matching", state.Programs[0].Title)
	})

	t.Run("multiple EPG sources are all loaded", func(t *testing.T) {
		src1 := makeEpgSource("src1", true)
		src2 := makeEpgSource("src2", true)
		repo := &mockProgramRepo{
			programs: map[models.ULID][]*models.EpgProgram{
				src1.ID: {
					makeProgram("ch1", "Show From Src1", now, future),
				},
				src2.ID: {
					makeProgram("ch2", "Show From Src2", now, farFuture),
				},
			},
		}

		state := newTestState(t)
		state.EpgSources = []*models.EpgSource{src1, src2}
		state.ChannelMap = map[string]*models.Channel{
			"ch1": {TvgID: "ch1"},
			"ch2": {TvgID: "ch2"},
		}

		stage := New(repo)
		result, err := stage.Execute(context.Background(), state)

		require.NoError(t, err)
		assert.Equal(t, 2, result.RecordsProcessed)
		assert.Len(t, state.Programs, 2)
	})

	t.Run("disabled EPG source is skipped", func(t *testing.T) {
		enabledSrc := makeEpgSource("enabled", true)
		disabledSrc := makeEpgSource("disabled", false)

		repo := &mockProgramRepo{
			programs: map[models.ULID][]*models.EpgProgram{
				enabledSrc.ID: {
					makeProgram("ch1", "From Enabled", now, future),
				},
				disabledSrc.ID: {
					makeProgram("ch1", "From Disabled", now, future),
				},
			},
		}

		state := newTestState(t)
		state.EpgSources = []*models.EpgSource{enabledSrc, disabledSrc}
		state.ChannelMap = map[string]*models.Channel{
			"ch1": {TvgID: "ch1"},
		}

		stage := New(repo)
		result, err := stage.Execute(context.Background(), state)

		require.NoError(t, err)
		// Only the enabled source's program should be loaded
		assert.Equal(t, 1, result.RecordsProcessed)
		require.Len(t, state.Programs, 1)
		assert.Equal(t, "From Enabled", state.Programs[0].Title)
	})

	t.Run("repo error is non-fatal and recorded in state", func(t *testing.T) {
		goodSrc := makeEpgSource("good", true)
		badSrc := makeEpgSource("bad", true)

		// We need a repo that fails only for one source. Build a custom one.
		failingRepo := &perSourceMockRepo{
			programs: map[models.ULID][]*models.EpgProgram{
				goodSrc.ID: {
					makeProgram("ch1", "Good Program", now, future),
				},
			},
			errSources: map[models.ULID]error{
				badSrc.ID: errors.New("database connection lost"),
			},
		}

		state := newTestState(t)
		state.EpgSources = []*models.EpgSource{goodSrc, badSrc}
		state.ChannelMap = map[string]*models.Channel{
			"ch1": {TvgID: "ch1"},
		}

		stage := New(failingRepo)
		result, err := stage.Execute(context.Background(), state)

		require.NoError(t, err) // non-fatal: no top-level error
		assert.Equal(t, 1, result.RecordsProcessed)
		require.Len(t, state.Programs, 1)
		assert.True(t, state.HasErrors(), "state should record the non-fatal error")
		assert.Len(t, state.Errors, 1)
		assert.Contains(t, state.Errors[0].Error(), "database connection lost")
	})

	t.Run("context cancellation returns ctx error", func(t *testing.T) {
		src := makeEpgSource("src1", true)
		repo := &mockProgramRepo{
			programs: map[models.ULID][]*models.EpgProgram{
				src.ID: {
					makeProgram("ch1", "Show A", now, future),
				},
			},
		}

		state := newTestState(t)
		state.EpgSources = []*models.EpgSource{src}
		state.ChannelMap = map[string]*models.Channel{
			"ch1": {TvgID: "ch1"},
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		stage := New(repo)
		// The cancelled context error propagates from GetBySourceID callback
		// and is recorded as a non-fatal error on that source, or the stage
		// may return programs before checking context. Either way the error
		// surfaces.
		result, err := stage.Execute(ctx, state)

		// The error from the callback (ctx.Err()) is returned by GetBySourceID,
		// which the stage treats as a per-source error added to state.Errors.
		// The function itself doesn't return an error — it continues to the next source.
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.True(t, state.HasErrors())
	})

	t.Run("result counts match loaded programs", func(t *testing.T) {
		src := makeEpgSource("src1", true)
		repo := &mockProgramRepo{
			programs: map[models.ULID][]*models.EpgProgram{
				src.ID: {
					makeProgram("ch1", "P1", now, future),
					makeProgram("ch1", "P2", future, farFuture),
					makeProgram("ch2", "P3", now, future),
					// one expired, should not count
					makeProgram("ch1", "Expired", past, now.Add(-1*time.Second)),
					// one for unknown channel, should not count
					makeProgram("unknown", "Unknown", now, future),
				},
			},
		}

		state := newTestState(t)
		state.EpgSources = []*models.EpgSource{src}
		state.ChannelMap = map[string]*models.Channel{
			"ch1": {TvgID: "ch1"},
			"ch2": {TvgID: "ch2"},
		}

		stage := New(repo)
		result, err := stage.Execute(context.Background(), state)

		require.NoError(t, err)
		assert.Equal(t, 3, result.RecordsProcessed)
		assert.Len(t, state.Programs, 3)
		// Verify artifact was created
		require.Len(t, result.Artifacts, 1)
		assert.Equal(t, core.ArtifactTypePrograms, result.Artifacts[0].Type)
		assert.Equal(t, 3, result.Artifacts[0].RecordCount)
	})

	t.Run("nil Enabled treated as enabled (BoolVal default)", func(t *testing.T) {
		src := &models.EpgSource{
			BaseModel: models.BaseModel{ID: models.NewULID()},
			Name:      "nil-enabled",
			Enabled:   nil, // BoolVal(nil) returns true
		}
		repo := &mockProgramRepo{
			programs: map[models.ULID][]*models.EpgProgram{
				src.ID: {
					makeProgram("ch1", "Should Load", now, future),
				},
			},
		}

		state := newTestState(t)
		state.EpgSources = []*models.EpgSource{src}
		state.ChannelMap = map[string]*models.Channel{
			"ch1": {TvgID: "ch1"},
		}

		stage := New(repo)
		result, err := stage.Execute(context.Background(), state)

		require.NoError(t, err)
		assert.Equal(t, 1, result.RecordsProcessed)
		require.Len(t, state.Programs, 1)
	})
}

// --- Per-source failing mock ---

// perSourceMockRepo lets you configure errors per source ID while
// returning programs normally for other sources.
type perSourceMockRepo struct {
	programs   map[models.ULID][]*models.EpgProgram
	errSources map[models.ULID]error
}

func (m *perSourceMockRepo) GetBySourceID(ctx context.Context, sourceID models.ULID, callback func(*models.EpgProgram) error) error {
	if err, ok := m.errSources[sourceID]; ok {
		return err
	}
	for _, prog := range m.programs[sourceID] {
		if err := callback(prog); err != nil {
			return err
		}
	}
	return nil
}

func (m *perSourceMockRepo) CountBySourceID(ctx context.Context, sourceID models.ULID) (int64, error) {
	if err, ok := m.errSources[sourceID]; ok {
		return 0, err
	}
	return int64(len(m.programs[sourceID])), nil
}

// Unused interface methods.

func (m *perSourceMockRepo) Create(ctx context.Context, program *models.EpgProgram) error {
	return nil
}

func (m *perSourceMockRepo) CreateBatch(ctx context.Context, programs []*models.EpgProgram) error {
	return nil
}

func (m *perSourceMockRepo) GetByID(ctx context.Context, id models.ULID) (*models.EpgProgram, error) {
	return nil, nil
}

func (m *perSourceMockRepo) GetByChannelID(ctx context.Context, channelID string, start, end time.Time) ([]*models.EpgProgram, error) {
	return nil, nil
}

func (m *perSourceMockRepo) GetByChannelIDWithLimit(ctx context.Context, channelID string, limit int) ([]*models.EpgProgram, error) {
	return nil, nil
}

func (m *perSourceMockRepo) GetCurrentByChannelID(ctx context.Context, channelID string) (*models.EpgProgram, error) {
	return nil, nil
}

func (m *perSourceMockRepo) Delete(ctx context.Context, id models.ULID) error {
	return nil
}

func (m *perSourceMockRepo) DeleteBySourceID(ctx context.Context, sourceID models.ULID) error {
	return nil
}

func (m *perSourceMockRepo) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	return 0, nil
}

func (m *perSourceMockRepo) DeleteOld(ctx context.Context) (int64, error) {
	return 0, nil
}
