package filtering

import (
	"context"
	"fmt"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/pipeline/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func makeChannel(tvgID, name, group string) *models.Channel {
	return &models.Channel{
		TvgID:       tvgID,
		TvgName:     name,
		ChannelName: name,
		GroupTitle:  group,
		StreamURL:   "http://example.com/stream/" + tvgID,
	}
}

func makeProgram(channelID, title, category string) *models.EpgProgram {
	return &models.EpgProgram{
		ChannelID:   channelID,
		Title:       title,
		Description: "Description of " + title,
		Category:    category,
	}
}

// makeState creates a State with a nil proxy so that loadFiltersFromProxy is a
// no-op, allowing filters set via WithExpressionFilters to be used by Execute.
func makeState(channels []*models.Channel, programs []*models.EpgProgram) *core.State {
	t := &testing.T{} // unused, just satisfying helper pattern
	_ = t
	state := &core.State{
		Proxy:      nil,
		Channels:   channels,
		Programs:   programs,
		ChannelMap: make(map[string]*models.Channel),
		Artifacts:  make(map[string][]core.Artifact),
		Metadata:   make(map[string]any),
	}
	for _, ch := range channels {
		if ch.TvgID != "" {
			state.ChannelMap[ch.TvgID] = ch
		}
	}
	return state
}

// makeProxyState creates a State with a real proxy (with optional ProxyFilters).
func makeProxyState(
	t *testing.T,
	channels []*models.Channel,
	programs []*models.EpgProgram,
	proxyFilters []models.ProxyFilter,
) *core.State {
	t.Helper()
	proxy := &models.StreamProxy{
		BaseModel: models.BaseModel{ID: models.NewULID()},
		Name:      "test-proxy",
		Filters:   proxyFilters,
	}
	state := core.NewState(proxy)
	state.Channels = channels
	state.Programs = programs
	for _, ch := range channels {
		if ch.TvgID != "" {
			state.ChannelMap[ch.TvgID] = ch
		}
	}
	return state
}

// makeProxyFilter creates a ProxyFilter with a loaded Filter relationship.
func makeProxyFilter(
	t *testing.T,
	priority int,
	sourceType models.FilterSourceType,
	action models.FilterAction,
	expr string,
	name string,
	isActive *bool,
) models.ProxyFilter {
	t.Helper()
	filterID := models.NewULID()
	return models.ProxyFilter{
		BaseModel: models.BaseModel{ID: models.NewULID()},
		FilterID:  filterID,
		Priority:  priority,
		IsActive:  isActive,
		Filter: &models.Filter{
			BaseModel:  models.BaseModel{ID: filterID},
			Name:       name,
			SourceType: sourceType,
			Action:     action,
			Expression: expr,
		},
	}
}

func boolPtr(b bool) *bool {
	return &b
}

// ---------------------------------------------------------------------------
// Builder method tests
// ---------------------------------------------------------------------------

func TestNew_ReturnsValidStage(t *testing.T) {
	stage := New()
	require.NotNil(t, stage)
	assert.Equal(t, StageID, stage.ID())
	assert.Equal(t, StageName, stage.Name())
	assert.Empty(t, stage.expressionFilters)
}

func TestWithExpressionFilters(t *testing.T) {
	filters := []ExpressionFilter{
		{ID: "f1", Name: "Filter 1", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `channel_name contains "News"`},
		{ID: "f2", Name: "Filter 2", Target: FilterTargetChannel, Action: FilterActionExclude, Expression: `group_title equals "Adult"`},
	}

	stage := New().WithExpressionFilters(filters)
	assert.Len(t, stage.expressionFilters, 2)
	assert.Equal(t, "f1", stage.expressionFilters[0].ID)
	assert.Equal(t, "f2", stage.expressionFilters[1].ID)
}

func TestAddExpressionFilter(t *testing.T) {
	stage := New()
	stage.AddExpressionFilter(ExpressionFilter{ID: "a", Name: "A"})
	stage.AddExpressionFilter(ExpressionFilter{ID: "b", Name: "B"})

	assert.Len(t, stage.expressionFilters, 2)
	assert.Equal(t, "a", stage.expressionFilters[0].ID)
	assert.Equal(t, "b", stage.expressionFilters[1].ID)
}

func TestNewConstructor(t *testing.T) {
	constructor := NewConstructor()
	require.NotNil(t, constructor)

	deps := &core.Dependencies{}
	stage := constructor(deps)
	require.NotNil(t, stage)
	assert.Equal(t, StageID, stage.ID())
	assert.Equal(t, StageName, stage.Name())
}

func TestStage_Cleanup(t *testing.T) {
	stage := New()
	err := stage.Cleanup(context.Background())
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Execute: no filters → passthrough
// ---------------------------------------------------------------------------

func TestExecute_NoFilters_Passthrough(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "Channel One", "Sports"),
		makeChannel("ch2", "Channel Two", "News"),
	}
	programs := []*models.EpgProgram{
		makeProgram("ch1", "Football Match", "Sports"),
		makeProgram("ch2", "Evening News", "News"),
	}
	state := makeState(channels, programs)

	stage := New()
	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Len(t, state.Channels, 2)
	assert.Len(t, state.Programs, 2)
	assert.Equal(t, "No filters assigned to proxy", result.Message)
}

// ---------------------------------------------------------------------------
// Execute: include channel filter
// ---------------------------------------------------------------------------

func TestExecute_IncludeChannelFilter(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "Sports HD", "Sports"),
		makeChannel("ch2", "News 24", "News"),
		makeChannel("ch3", "Movie Channel", "Movies"),
	}
	programs := []*models.EpgProgram{
		makeProgram("ch1", "Football", "Sports"),
		makeProgram("ch2", "Headlines", "News"),
		makeProgram("ch3", "Thriller", "Movies"),
	}
	state := makeState(channels, programs)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{
			ID:         "inc-sports",
			Name:       "Include Sports",
			Target:     FilterTargetChannel,
			Action:     FilterActionInclude,
			Expression: `group_title equals "Sports"`,
		},
	})

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Len(t, state.Channels, 1)
	assert.Equal(t, "ch1", state.Channels[0].TvgID)

	require.Len(t, state.Programs, 1)
	assert.Equal(t, "Football", state.Programs[0].Title)
}

// ---------------------------------------------------------------------------
// Execute: exclude channel filter
// ---------------------------------------------------------------------------

func TestExecute_ExcludeChannelFilter(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "Sports HD", "Sports"),
		makeChannel("ch2", "News 24", "News"),
		makeChannel("ch3", "Movie Channel", "Movies"),
	}
	programs := []*models.EpgProgram{
		makeProgram("ch1", "Football", "Sports"),
		makeProgram("ch2", "Headlines", "News"),
		makeProgram("ch3", "Thriller", "Movies"),
	}
	state := makeState(channels, programs)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{
			ID:         "inc-all",
			Name:       "Include All",
			Target:     FilterTargetChannel,
			Action:     FilterActionInclude,
			Expression: `channel_name contains ""`,
		},
		{
			ID:         "exc-movies",
			Name:       "Exclude Movies",
			Target:     FilterTargetChannel,
			Action:     FilterActionExclude,
			Expression: `group_title equals "Movies"`,
		},
	})

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)
	require.NotNil(t, result)

	require.Len(t, state.Channels, 2)
	tvgIDs := []string{state.Channels[0].TvgID, state.Channels[1].TvgID}
	assert.Contains(t, tvgIDs, "ch1")
	assert.Contains(t, tvgIDs, "ch2")
	assert.NotContains(t, tvgIDs, "ch3")

	for _, p := range state.Programs {
		assert.NotEqual(t, "ch3", p.ChannelID)
	}
}

// ---------------------------------------------------------------------------
// Execute: include then exclude (sequential)
// ---------------------------------------------------------------------------

func TestExecute_IncludeThenExclude(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "Sports HD", "Sports"),
		makeChannel("ch2", "Sports News", "Sports"),
		makeChannel("ch3", "News 24", "News"),
	}
	state := makeState(channels, nil)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{
			ID:         "inc-sports",
			Name:       "Include Sports",
			Target:     FilterTargetChannel,
			Action:     FilterActionInclude,
			Expression: `group_title equals "Sports"`,
		},
		{
			ID:         "exc-sports-news",
			Name:       "Exclude Sports News",
			Target:     FilterTargetChannel,
			Action:     FilterActionExclude,
			Expression: `channel_name contains "News"`,
		},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	require.Len(t, state.Channels, 1)
	assert.Equal(t, "ch1", state.Channels[0].TvgID)
}

// ---------------------------------------------------------------------------
// Execute: multiple include filters → union
// ---------------------------------------------------------------------------

func TestExecute_MultipleIncludeFilters_Union(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "Sports HD", "Sports"),
		makeChannel("ch2", "News 24", "News"),
		makeChannel("ch3", "Movie Channel", "Movies"),
		makeChannel("ch4", "Kids TV", "Kids"),
	}
	state := makeState(channels, nil)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{
			ID:         "inc-sports",
			Name:       "Include Sports",
			Target:     FilterTargetChannel,
			Action:     FilterActionInclude,
			Expression: `group_title equals "Sports"`,
		},
		{
			ID:         "inc-news",
			Name:       "Include News",
			Target:     FilterTargetChannel,
			Action:     FilterActionInclude,
			Expression: `group_title equals "News"`,
		},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	require.Len(t, state.Channels, 2)
	tvgIDs := []string{state.Channels[0].TvgID, state.Channels[1].TvgID}
	assert.Contains(t, tvgIDs, "ch1")
	assert.Contains(t, tvgIDs, "ch2")
}

// ---------------------------------------------------------------------------
// Execute: program filtering (include / exclude by category and title)
// ---------------------------------------------------------------------------

func TestExecute_ProgramFiltering_Include(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "Channel One", "General"),
	}
	programs := []*models.EpgProgram{
		makeProgram("ch1", "Morning News", "News"),
		makeProgram("ch1", "Cooking Show", "Entertainment"),
		makeProgram("ch1", "Evening News", "News"),
		makeProgram("ch1", "Late Night Movie", "Movies"),
	}
	state := makeState(channels, programs)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "inc-ch", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `channel_name contains ""`},
		{ID: "inc-news-prog", Target: FilterTargetProgram, Action: FilterActionInclude, Expression: `programme_category equals "News"`},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	assert.Len(t, state.Channels, 1)
	require.Len(t, state.Programs, 2)
	for _, p := range state.Programs {
		assert.Equal(t, "News", p.Category)
	}
}

func TestExecute_ProgramFiltering_Exclude(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "Channel One", "General"),
	}
	programs := []*models.EpgProgram{
		makeProgram("ch1", "Morning News", "News"),
		makeProgram("ch1", "Cooking Show", "Entertainment"),
		makeProgram("ch1", "Evening News", "News"),
	}
	state := makeState(channels, programs)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "inc-ch", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `channel_name contains ""`},
		{ID: "inc-all-prog", Target: FilterTargetProgram, Action: FilterActionInclude, Expression: `programme_title contains ""`},
		{ID: "exc-news-prog", Target: FilterTargetProgram, Action: FilterActionExclude, Expression: `programme_category equals "News"`},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	require.Len(t, state.Programs, 1)
	assert.Equal(t, "Cooking Show", state.Programs[0].Title)
}

func TestExecute_ProgramFilterByTitle(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "General", "General"),
	}
	programs := []*models.EpgProgram{
		makeProgram("ch1", "Morning News", "News"),
		makeProgram("ch1", "Cooking Show", "Entertainment"),
		makeProgram("ch1", "Evening News", "News"),
	}
	state := makeState(channels, programs)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "inc-ch", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `channel_name contains ""`},
		{ID: "inc-prog", Target: FilterTargetProgram, Action: FilterActionInclude, Expression: `programme_title contains "News"`},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	require.Len(t, state.Programs, 2)
	for _, p := range state.Programs {
		assert.Contains(t, p.Title, "News")
	}
}

func TestExecute_ProgramDescription(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "Channel 1", "General"),
	}
	programs := []*models.EpgProgram{
		makeProgram("ch1", "Show A", "Drama"),
		makeProgram("ch1", "Show B", "Comedy"),
	}
	programs[0].Description = "An exciting thriller with action scenes"
	programs[1].Description = "A light-hearted comedy about friendship"

	state := makeState(channels, programs)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "inc-ch", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `channel_name contains ""`},
		{ID: "inc-prog", Target: FilterTargetProgram, Action: FilterActionInclude, Expression: `programme_description contains "thriller"`},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	require.Len(t, state.Programs, 1)
	assert.Equal(t, "Show A", state.Programs[0].Title)
}

// ---------------------------------------------------------------------------
// Execute: programs filtered by channel exclusion
// ---------------------------------------------------------------------------

func TestExecute_ProgramsFilteredByChannel(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "Sports HD", "Sports"),
		makeChannel("ch2", "News 24", "News"),
	}
	programs := []*models.EpgProgram{
		makeProgram("ch1", "Football", "Sports"),
		makeProgram("ch1", "Tennis", "Sports"),
		makeProgram("ch2", "Headlines", "News"),
		makeProgram("ch2", "Weather", "News"),
	}
	state := makeState(channels, programs)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "inc-sports", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `group_title equals "Sports"`},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	require.Len(t, state.Channels, 1)
	assert.Equal(t, "ch1", state.Channels[0].TvgID)

	require.Len(t, state.Programs, 2)
	for _, p := range state.Programs {
		assert.Equal(t, "ch1", p.ChannelID)
	}
}

// ---------------------------------------------------------------------------
// Execute: no program filters with channel filters → all programs pass
// ---------------------------------------------------------------------------

func TestExecute_NoProgramFiltersWithChannelFilters(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "Sports HD", "Sports"),
		makeChannel("ch2", "News 24", "News"),
	}
	programs := []*models.EpgProgram{
		makeProgram("ch1", "Football", "Sports"),
		makeProgram("ch2", "Headlines", "News"),
	}
	state := makeState(channels, programs)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "inc-all", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `channel_name contains ""`},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	assert.Len(t, state.Channels, 2)
	assert.Len(t, state.Programs, 2)
}

// ---------------------------------------------------------------------------
// Execute: empty expression → skipped
// ---------------------------------------------------------------------------

func TestExecute_EmptyExpression_Skipped(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "Channel One", "General"),
	}
	state := makeState(channels, nil)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "empty", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: ""},
		{ID: "whitespace", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: "   "},
		{ID: "valid", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `channel_name contains "Channel"`},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	require.Len(t, state.Channels, 1)
	assert.Equal(t, "ch1", state.Channels[0].TvgID)
}

// ---------------------------------------------------------------------------
// Execute: invalid expression → error
// ---------------------------------------------------------------------------

func TestExecute_InvalidExpression_Error(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "Channel One", "General"),
	}
	state := makeState(channels, nil)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{
			ID:         "bad",
			Name:       "Bad Expression",
			Target:     FilterTargetChannel,
			Action:     FilterActionInclude,
			Expression: `channel_name unknown_op "test"`,
		},
	})

	_, err := stage.Execute(context.Background(), state)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compiling expression filters")
}

// ---------------------------------------------------------------------------
// Execute: context cancellation
// ---------------------------------------------------------------------------

func TestExecute_ContextCancellation_ChannelFilters(t *testing.T) {
	channels := make([]*models.Channel, 0, 100)
	for i := range 100 {
		channels = append(channels, makeChannel(fmt.Sprintf("ch%d", i), "Channel", "Group"))
	}
	state := makeState(channels, nil)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "inc", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `channel_name contains "Channel"`},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := stage.Execute(ctx, state)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestExecute_ContextCancellation_ProgramFilters(t *testing.T) {
	channels := []*models.Channel{makeChannel("ch1", "Test", "General")}
	programs := make([]*models.EpgProgram, 0, 50)
	for range 50 {
		programs = append(programs, makeProgram("ch1", "Program", "Cat"))
	}
	state := makeState(channels, programs)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "inc-ch", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `channel_name contains ""`},
		{ID: "inc-prog", Target: FilterTargetProgram, Action: FilterActionInclude, Expression: `programme_title contains "Program"`},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := stage.Execute(ctx, state)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

// ---------------------------------------------------------------------------
// Execute: case insensitive matching (default)
// ---------------------------------------------------------------------------

func TestExecute_CaseInsensitiveMatching(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "SPORTS HD", "Sports"),
		makeChannel("ch2", "sports live", "sports"),
		makeChannel("ch3", "News 24", "News"),
	}
	state := makeState(channels, nil)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "inc", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `group_title equals "sports"`},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	require.Len(t, state.Channels, 2)
	tvgIDs := []string{state.Channels[0].TvgID, state.Channels[1].TvgID}
	assert.Contains(t, tvgIDs, "ch1")
	assert.Contains(t, tvgIDs, "ch2")
}

// ---------------------------------------------------------------------------
// Execute: various operators
// ---------------------------------------------------------------------------

func TestExecute_ContainsOperator(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "BBC News HD", "News"),
		makeChannel("ch2", "CNN International", "News"),
		makeChannel("ch3", "Sports HD", "Sports"),
	}
	state := makeState(channels, nil)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "inc", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `channel_name contains "HD"`},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	require.Len(t, state.Channels, 2)
	tvgIDs := []string{state.Channels[0].TvgID, state.Channels[1].TvgID}
	assert.Contains(t, tvgIDs, "ch1")
	assert.Contains(t, tvgIDs, "ch3")
}

func TestExecute_StartsWithOperator(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "BBC One", "UK"),
		makeChannel("ch2", "BBC Two", "UK"),
		makeChannel("ch3", "ITV One", "UK"),
	}
	state := makeState(channels, nil)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "inc", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `channel_name starts_with "BBC"`},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	require.Len(t, state.Channels, 2)
	for _, ch := range state.Channels {
		assert.Contains(t, ch.ChannelName, "BBC")
	}
}

func TestExecute_EndsWithOperator(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "Sports HD", "Sports"),
		makeChannel("ch2", "News HD", "News"),
		makeChannel("ch3", "Movie SD", "Movies"),
	}
	state := makeState(channels, nil)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "inc", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `channel_name ends_with "HD"`},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	require.Len(t, state.Channels, 2)
	tvgIDs := []string{state.Channels[0].TvgID, state.Channels[1].TvgID}
	assert.Contains(t, tvgIDs, "ch1")
	assert.Contains(t, tvgIDs, "ch2")
}

func TestExecute_ANDCombinator(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "Sports HD", "Sports"),
		makeChannel("ch2", "Sports SD", "Sports"),
		makeChannel("ch3", "News HD", "News"),
	}
	state := makeState(channels, nil)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "inc", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `group_title equals "Sports" AND channel_name contains "HD"`},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	require.Len(t, state.Channels, 1)
	assert.Equal(t, "ch1", state.Channels[0].TvgID)
}

func TestExecute_ORCombinator(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "Sports HD", "Sports"),
		makeChannel("ch2", "News 24", "News"),
		makeChannel("ch3", "Movie Channel", "Movies"),
	}
	state := makeState(channels, nil)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "inc", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `group_title equals "Sports" OR group_title equals "News"`},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	require.Len(t, state.Channels, 2)
	tvgIDs := []string{state.Channels[0].TvgID, state.Channels[1].TvgID}
	assert.Contains(t, tvgIDs, "ch1")
	assert.Contains(t, tvgIDs, "ch2")
}

// ---------------------------------------------------------------------------
// Execute: edge cases
// ---------------------------------------------------------------------------

func TestExecute_NilProxy_Passthrough(t *testing.T) {
	state := &core.State{
		Proxy:      nil,
		Channels:   []*models.Channel{makeChannel("ch1", "Test", "General")},
		Programs:   []*models.EpgProgram{},
		ChannelMap: map[string]*models.Channel{},
		Artifacts:  make(map[string][]core.Artifact),
		Metadata:   make(map[string]any),
	}

	stage := New()
	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)
	assert.Equal(t, "No filters assigned to proxy", result.Message)
}

func TestExecute_ChannelMapUpdated(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "Sports HD", "Sports"),
		makeChannel("ch2", "News 24", "News"),
	}
	state := makeState(channels, nil)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "inc", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `group_title equals "Sports"`},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	assert.Len(t, state.ChannelMap, 1)
	_, ok := state.ChannelMap["ch1"]
	assert.True(t, ok)
	_, ok = state.ChannelMap["ch2"]
	assert.False(t, ok)
}

func TestExecute_ResultCounts(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "Sports HD", "Sports"),
		makeChannel("ch2", "News 24", "News"),
		makeChannel("ch3", "Movie Channel", "Movies"),
	}
	programs := []*models.EpgProgram{
		makeProgram("ch1", "Football", "Sports"),
		makeProgram("ch2", "Headlines", "News"),
		makeProgram("ch3", "Thriller", "Movies"),
	}
	state := makeState(channels, programs)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "inc-sports", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `group_title equals "Sports"`},
	})

	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	assert.Equal(t, 6, result.RecordsProcessed)
	assert.Equal(t, 4, result.RecordsModified)
	assert.Contains(t, result.Message, "2/3 channels")
	assert.Contains(t, result.Message, "2/3 programs")

	require.Len(t, result.Artifacts, 1)
	assert.Equal(t, core.ArtifactTypeChannels, result.Artifacts[0].Type)
	assert.Equal(t, 1, result.Artifacts[0].RecordCount)
}

func TestExecute_PreservesOriginalOrder(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch-alpha", "Alpha", "Group"),
		makeChannel("ch-beta", "Beta", "Group"),
		makeChannel("ch-gamma", "Gamma", "Group"),
		makeChannel("ch-delta", "Delta", "Group"),
	}
	state := makeState(channels, nil)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "inc", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `channel_name contains ""`},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	require.Len(t, state.Channels, 4)
	assert.Equal(t, "ch-alpha", state.Channels[0].TvgID)
	assert.Equal(t, "ch-beta", state.Channels[1].TvgID)
	assert.Equal(t, "ch-gamma", state.Channels[2].TvgID)
	assert.Equal(t, "ch-delta", state.Channels[3].TvgID)
}

func TestExecute_OnlyExcludeFilters_EmptyOutput(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "Sports", "Sports"),
	}
	state := makeState(channels, nil)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "exc", Target: FilterTargetChannel, Action: FilterActionExclude, Expression: `group_title equals "Sports"`},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	assert.Len(t, state.Channels, 0)
}

func TestExecute_StreamURLField(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "Channel 1", "General"),
		makeChannel("ch2", "Channel 2", "General"),
	}
	channels[0].StreamURL = "http://provider-a.com/live/ch1.ts"
	channels[1].StreamURL = "http://provider-b.com/live/ch2.ts"

	state := makeState(channels, nil)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "inc", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `stream_url contains "provider-a"`},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	require.Len(t, state.Channels, 1)
	assert.Equal(t, "ch1", state.Channels[0].TvgID)
}

func TestExecute_TvgIDField(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("bbc.one.uk", "BBC One", "UK"),
		makeChannel("itv.one.uk", "ITV One", "UK"),
		makeChannel("cnn.us", "CNN", "US"),
	}
	state := makeState(channels, nil)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "inc", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `tvg_id ends_with ".uk"`},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	require.Len(t, state.Channels, 2)
	for _, ch := range state.Channels {
		assert.Contains(t, ch.TvgID, ".uk")
	}
}

func TestExecute_TvgLogoField(t *testing.T) {
	channels := []*models.Channel{
		{TvgID: "ch1", ChannelName: "A", TvgLogo: "http://logo.com/sports.png", StreamURL: "http://ex.com/1"},
		{TvgID: "ch2", ChannelName: "B", TvgLogo: "http://logo.com/news.png", StreamURL: "http://ex.com/2"},
	}
	state := makeState(channels, nil)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "inc", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: `tvg_logo contains "sports"`},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	require.Len(t, state.Channels, 1)
	assert.Equal(t, "ch1", state.Channels[0].TvgID)
}

func TestExecute_AllFiltersEmptyExpression(t *testing.T) {
	channels := []*models.Channel{makeChannel("ch1", "Test Channel", "General")}
	programs := []*models.EpgProgram{makeProgram("ch1", "Test Show", "Drama")}
	state := makeState(channels, programs)

	stage := New().WithExpressionFilters([]ExpressionFilter{
		{ID: "e1", Target: FilterTargetChannel, Action: FilterActionInclude, Expression: ""},
		{ID: "e2", Target: FilterTargetProgram, Action: FilterActionInclude, Expression: "  "},
	})

	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	// All expressions skipped during compile. With no compiled channel filters,
	// output starts empty → 0 channels, 0 programs.
	assert.Len(t, state.Channels, 0)
}

// ---------------------------------------------------------------------------
// Tests with proxy.Filters (loadFiltersFromProxy)
// ---------------------------------------------------------------------------

func TestExecute_LoadFiltersFromProxy(t *testing.T) {
	channels := []*models.Channel{
		makeChannel("ch1", "Sports HD", "Sports"),
		makeChannel("ch2", "News 24", "News"),
	}
	programs := []*models.EpgProgram{
		makeProgram("ch1", "Football", "Sports"),
		makeProgram("ch1", "Evening News", "News"),
	}

	pf1 := makeProxyFilter(t, 1, models.FilterSourceTypeStream, models.FilterActionInclude,
		`group_title equals "Sports"`, "Include Sports", boolPtr(true))
	pf2 := makeProxyFilter(t, 2, models.FilterSourceTypeStream, models.FilterActionExclude,
		`channel_name contains ""`, "Exclude All", boolPtr(false)) // Inactive
	pf3 := makeProxyFilter(t, 3, models.FilterSourceTypeEPG, models.FilterActionInclude,
		`programme_category equals "News"`, "Include News Programs", nil) // nil = active

	state := makeProxyState(t, channels, programs, []models.ProxyFilter{pf1, pf2, pf3})

	stage := New()
	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	require.Len(t, state.Channels, 1)
	assert.Equal(t, "ch1", state.Channels[0].TvgID)

	require.Len(t, state.Programs, 1)
	assert.Equal(t, "Evening News", state.Programs[0].Title)
}

func TestExecute_LoadFiltersFromProxy_NilFilterRelationship(t *testing.T) {
	pf := models.ProxyFilter{
		BaseModel: models.BaseModel{ID: models.NewULID()},
		FilterID:  models.NewULID(),
		Priority:  1,
		Filter:    nil,
	}

	state := makeProxyState(t,
		[]*models.Channel{makeChannel("ch1", "Test", "General")},
		nil,
		[]models.ProxyFilter{pf},
	)

	stage := New()
	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)
	assert.Equal(t, "No filters assigned to proxy", result.Message)
}

func TestExecute_LoadFiltersFromProxy_UnknownSourceType(t *testing.T) {
	filterID := models.NewULID()
	pf := models.ProxyFilter{
		BaseModel: models.BaseModel{ID: models.NewULID()},
		FilterID:  filterID,
		Priority:  1,
		Filter: &models.Filter{
			BaseModel:  models.BaseModel{ID: filterID},
			Name:       "Unknown Type",
			SourceType: "unknown",
			Action:     models.FilterActionInclude,
			Expression: `channel_name contains "test"`,
		},
	}

	state := makeProxyState(t,
		[]*models.Channel{makeChannel("ch1", "Test", "General")},
		nil,
		[]models.ProxyFilter{pf},
	)

	stage := New()
	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)
	assert.Equal(t, "No filters assigned to proxy", result.Message)
}

func TestExecute_LoadFiltersFromProxy_UnknownAction(t *testing.T) {
	filterID := models.NewULID()
	pf := models.ProxyFilter{
		BaseModel: models.BaseModel{ID: models.NewULID()},
		FilterID:  filterID,
		Priority:  1,
		Filter: &models.Filter{
			BaseModel:  models.BaseModel{ID: filterID},
			Name:       "Unknown Action",
			SourceType: models.FilterSourceTypeStream,
			Action:     "unknown_action",
			Expression: `channel_name contains "test"`,
		},
	}

	state := makeProxyState(t,
		[]*models.Channel{makeChannel("ch1", "Test", "General")},
		nil,
		[]models.ProxyFilter{pf},
	)

	stage := New()
	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)
	assert.Equal(t, "No filters assigned to proxy", result.Message)
}

func TestExecute_LoadFiltersFromProxy_PrioritySorting(t *testing.T) {
	// Provide in reverse order to ensure sorting works
	pfExclude := makeProxyFilter(t, 10, models.FilterSourceTypeStream, models.FilterActionExclude,
		`group_title equals "News"`, "Exclude News", nil)
	pfInclude := makeProxyFilter(t, 1, models.FilterSourceTypeStream, models.FilterActionInclude,
		`channel_name contains ""`, "Include All", nil)

	channels := []*models.Channel{
		makeChannel("ch1", "Sports HD", "Sports"),
		makeChannel("ch2", "News 24", "News"),
		makeChannel("ch3", "Movie Channel", "Movies"),
	}

	state := makeProxyState(t, channels, nil, []models.ProxyFilter{pfExclude, pfInclude})

	stage := New()
	_, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)

	require.Len(t, state.Channels, 2)
	tvgIDs := []string{state.Channels[0].TvgID, state.Channels[1].TvgID}
	assert.Contains(t, tvgIDs, "ch1")
	assert.Contains(t, tvgIDs, "ch3")
	assert.NotContains(t, tvgIDs, "ch2")
}

func TestExecute_LoadFiltersFromProxy_EmptyFilters_Passthrough(t *testing.T) {
	state := makeProxyState(t,
		[]*models.Channel{makeChannel("ch1", "Test", "General")},
		[]*models.EpgProgram{makeProgram("ch1", "Show", "Drama")},
		nil,
	)

	stage := New()
	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)
	assert.Equal(t, "No filters assigned to proxy", result.Message)
	assert.Len(t, state.Channels, 1)
	assert.Len(t, state.Programs, 1)
}

func TestExecute_LoadFiltersFromProxy_InactiveAll(t *testing.T) {
	pf := makeProxyFilter(t, 1, models.FilterSourceTypeStream, models.FilterActionInclude,
		`channel_name contains "Test"`, "Inactive Include", boolPtr(false))

	state := makeProxyState(t,
		[]*models.Channel{makeChannel("ch1", "Test", "General")},
		nil,
		[]models.ProxyFilter{pf},
	)

	stage := New()
	result, err := stage.Execute(context.Background(), state)
	require.NoError(t, err)
	assert.Equal(t, "No filters assigned to proxy", result.Message)
}

// ---------------------------------------------------------------------------
// Table-driven tests
// ---------------------------------------------------------------------------

func TestExecute_TableDriven(t *testing.T) {
	tests := []struct {
		name             string
		channels         []*models.Channel
		programs         []*models.EpgProgram
		filters          []ExpressionFilter
		wantChannelCount int
		wantProgramCount int
		wantErr          bool
	}{
		{
			name: "include by tvg_name",
			channels: []*models.Channel{
				makeChannel("ch1", "Alpha", "A"),
				makeChannel("ch2", "Beta", "B"),
			},
			filters: []ExpressionFilter{
				{ID: "1", Target: FilterTargetChannel, Action: FilterActionInclude,
					Expression: `tvg_name equals "Alpha"`},
			},
			wantChannelCount: 1,
		},
		{
			name: "exclude programs by category then no programs left",
			channels: []*models.Channel{
				makeChannel("ch1", "General", "General"),
			},
			programs: []*models.EpgProgram{
				makeProgram("ch1", "Show", "Drama"),
			},
			filters: []ExpressionFilter{
				{ID: "1", Target: FilterTargetChannel, Action: FilterActionInclude,
					Expression: `channel_name contains ""`},
				{ID: "2", Target: FilterTargetProgram, Action: FilterActionInclude,
					Expression: `programme_category equals "Comedy"`},
			},
			wantChannelCount: 1,
			wantProgramCount: 0,
		},
		{
			name: "programs for excluded channels are removed",
			channels: []*models.Channel{
				makeChannel("ch1", "Keep", "A"),
				makeChannel("ch2", "Remove", "B"),
			},
			programs: []*models.EpgProgram{
				makeProgram("ch1", "Show1", "Cat"),
				makeProgram("ch2", "Show2", "Cat"),
			},
			filters: []ExpressionFilter{
				{ID: "1", Target: FilterTargetChannel, Action: FilterActionInclude,
					Expression: `channel_name equals "Keep"`},
			},
			wantChannelCount: 1,
			wantProgramCount: 1,
		},
		{
			name: "invalid expression returns error",
			channels: []*models.Channel{
				makeChannel("ch1", "Test", "A"),
			},
			filters: []ExpressionFilter{
				{ID: "1", Target: FilterTargetChannel, Action: FilterActionInclude,
					Expression: `channel_name unknown_op "test"`},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := makeState(tt.channels, tt.programs)
			stage := New().WithExpressionFilters(tt.filters)

			_, err := stage.Execute(context.Background(), state)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, state.Channels, tt.wantChannelCount)
			assert.Len(t, state.Programs, tt.wantProgramCount)
		})
	}
}
