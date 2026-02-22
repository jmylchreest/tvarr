package service

import (
	"context"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEncoderOverrideRepo is an in-memory implementation for testing.
type mockEncoderOverrideRepo struct {
	overrides []*models.EncoderOverride
}

func newMockEncoderOverrideRepo() *mockEncoderOverrideRepo {
	return &mockEncoderOverrideRepo{overrides: []*models.EncoderOverride{}}
}

func (m *mockEncoderOverrideRepo) Create(_ context.Context, override *models.EncoderOverride) error {
	if override.ID.IsZero() {
		override.ID = models.NewULID()
	}
	m.overrides = append(m.overrides, override)
	return nil
}

func (m *mockEncoderOverrideRepo) GetByID(_ context.Context, id models.ULID) (*models.EncoderOverride, error) {
	for _, o := range m.overrides {
		if o.ID == id {
			return o, nil
		}
	}
	return nil, nil
}

func (m *mockEncoderOverrideRepo) GetAll(_ context.Context) ([]*models.EncoderOverride, error) {
	return m.overrides, nil
}

func (m *mockEncoderOverrideRepo) GetEnabled(_ context.Context) ([]*models.EncoderOverride, error) {
	var enabled []*models.EncoderOverride
	for _, o := range m.overrides {
		if models.BoolVal(o.IsEnabled) {
			enabled = append(enabled, o)
		}
	}
	// Sort by priority descending (higher first)
	for i := 0; i < len(enabled)-1; i++ {
		for j := i + 1; j < len(enabled); j++ {
			if enabled[j].Priority > enabled[i].Priority {
				enabled[i], enabled[j] = enabled[j], enabled[i]
			}
		}
	}
	return enabled, nil
}

func (m *mockEncoderOverrideRepo) GetByCodecType(_ context.Context, codecType models.EncoderOverrideCodecType) ([]*models.EncoderOverride, error) {
	var result []*models.EncoderOverride
	for _, o := range m.overrides {
		if o.CodecType == codecType {
			result = append(result, o)
		}
	}
	return result, nil
}

func (m *mockEncoderOverrideRepo) GetUserCreated(_ context.Context) ([]*models.EncoderOverride, error) {
	var result []*models.EncoderOverride
	for _, o := range m.overrides {
		if !o.IsSystem {
			result = append(result, o)
		}
	}
	return result, nil
}

func (m *mockEncoderOverrideRepo) GetByName(_ context.Context, name string) (*models.EncoderOverride, error) {
	for _, o := range m.overrides {
		if o.Name == name {
			return o, nil
		}
	}
	return nil, nil
}

func (m *mockEncoderOverrideRepo) GetSystem(_ context.Context) ([]*models.EncoderOverride, error) {
	var result []*models.EncoderOverride
	for _, o := range m.overrides {
		if o.IsSystem {
			result = append(result, o)
		}
	}
	return result, nil
}

func (m *mockEncoderOverrideRepo) Update(_ context.Context, override *models.EncoderOverride) error {
	for i, o := range m.overrides {
		if o.ID == override.ID {
			m.overrides[i] = override
			return nil
		}
	}
	return nil
}

func (m *mockEncoderOverrideRepo) Delete(_ context.Context, id models.ULID) error {
	for i, o := range m.overrides {
		if o.ID == id {
			m.overrides = append(m.overrides[:i], m.overrides[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockEncoderOverrideRepo) Count(_ context.Context) (int64, error) {
	return int64(len(m.overrides)), nil
}

func (m *mockEncoderOverrideRepo) CountEnabled(_ context.Context) (int64, error) {
	var count int64
	for _, o := range m.overrides {
		if models.BoolVal(o.IsEnabled) {
			count++
		}
	}
	return count, nil
}

func (m *mockEncoderOverrideRepo) Reorder(_ context.Context, reorders []repository.ReorderRequest) error {
	for _, r := range reorders {
		for _, o := range m.overrides {
			if o.ID == r.ID {
				o.Priority = r.Priority
				break
			}
		}
	}
	return nil
}

func TestEncoderOverrideService_Create(t *testing.T) {
	repo := newMockEncoderOverrideRepo()
	svc := NewEncoderOverrideService(repo)
	ctx := context.Background()

	override := &models.EncoderOverride{
		Name:          "Test Override",
		CodecType:     models.EncoderOverrideCodecTypeVideo,
		SourceCodec:   "h265",
		TargetEncoder: "libx265",
		IsEnabled:     new(true),
	}

	err := svc.Create(ctx, override)
	require.NoError(t, err)
	assert.False(t, override.ID.IsZero())
	assert.False(t, override.IsSystem, "service should prevent system flag")
	assert.Equal(t, 1, override.Priority, "first override should get priority 1")
}

func TestEncoderOverrideService_Create_AutoAssignsPriority(t *testing.T) {
	repo := newMockEncoderOverrideRepo()
	svc := NewEncoderOverrideService(repo)
	ctx := context.Background()

	// Create first override
	o1 := &models.EncoderOverride{
		Name:          "Override 1",
		CodecType:     models.EncoderOverrideCodecTypeVideo,
		SourceCodec:   "h264",
		TargetEncoder: "libx264",
		IsEnabled:     new(true),
	}
	require.NoError(t, svc.Create(ctx, o1))
	assert.Equal(t, 1, o1.Priority)

	// Create second override
	o2 := &models.EncoderOverride{
		Name:          "Override 2",
		CodecType:     models.EncoderOverrideCodecTypeVideo,
		SourceCodec:   "h265",
		TargetEncoder: "libx265",
		IsEnabled:     new(true),
	}
	require.NoError(t, svc.Create(ctx, o2))
	assert.Equal(t, 2, o2.Priority)
}

func TestEncoderOverrideService_Create_CannotCreateSystem(t *testing.T) {
	repo := newMockEncoderOverrideRepo()
	svc := NewEncoderOverrideService(repo)
	ctx := context.Background()

	override := &models.EncoderOverride{
		Name:          "Fake System",
		CodecType:     models.EncoderOverrideCodecTypeVideo,
		SourceCodec:   "h264",
		TargetEncoder: "libx264",
		IsSystem:      true, // Try to create as system
		IsEnabled:     new(true),
	}

	err := svc.Create(ctx, override)
	require.NoError(t, err)
	assert.False(t, override.IsSystem, "service should prevent system flag")
}

func TestEncoderOverrideService_GetByID(t *testing.T) {
	repo := newMockEncoderOverrideRepo()
	svc := NewEncoderOverrideService(repo)
	ctx := context.Background()

	override := &models.EncoderOverride{
		Name:          "Find Me",
		CodecType:     models.EncoderOverrideCodecTypeVideo,
		SourceCodec:   "h264",
		TargetEncoder: "libx264",
		IsEnabled:     new(true),
	}
	require.NoError(t, svc.Create(ctx, override))

	found, err := svc.GetByID(ctx, override.ID)
	require.NoError(t, err)
	assert.Equal(t, "Find Me", found.Name)
}

func TestEncoderOverrideService_GetByID_NotFound(t *testing.T) {
	repo := newMockEncoderOverrideRepo()
	svc := NewEncoderOverrideService(repo)
	ctx := context.Background()

	_, err := svc.GetByID(ctx, models.NewULID())
	assert.ErrorIs(t, err, ErrEncoderOverrideNotFound)
}

func TestEncoderOverrideService_GetAll(t *testing.T) {
	repo := newMockEncoderOverrideRepo()
	svc := NewEncoderOverrideService(repo)
	ctx := context.Background()

	// Create overrides
	for i := range 3 {
		o := &models.EncoderOverride{
			Name:          "Override " + string(rune('A'+i)),
			CodecType:     models.EncoderOverrideCodecTypeVideo,
			SourceCodec:   "h264",
			TargetEncoder: "libx264",
			IsEnabled:     new(true),
		}
		require.NoError(t, svc.Create(ctx, o))
	}

	all, err := svc.GetAll(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

func TestEncoderOverrideService_GetEnabled(t *testing.T) {
	repo := newMockEncoderOverrideRepo()
	svc := NewEncoderOverrideService(repo)
	ctx := context.Background()

	// Create enabled override
	o1 := &models.EncoderOverride{
		Name:          "Enabled",
		CodecType:     models.EncoderOverrideCodecTypeVideo,
		SourceCodec:   "h264",
		TargetEncoder: "libx264",
		IsEnabled:     new(true),
	}
	require.NoError(t, svc.Create(ctx, o1))

	// Create disabled override
	o2 := &models.EncoderOverride{
		Name:          "Disabled",
		CodecType:     models.EncoderOverrideCodecTypeVideo,
		SourceCodec:   "h265",
		TargetEncoder: "libx265",
		IsEnabled:     new(false),
	}
	require.NoError(t, svc.Create(ctx, o2))

	enabled, err := svc.GetEnabled(ctx)
	require.NoError(t, err)
	assert.Len(t, enabled, 1)
	assert.Equal(t, "Enabled", enabled[0].Name)
}

func TestEncoderOverrideService_GetByCodecType(t *testing.T) {
	repo := newMockEncoderOverrideRepo()
	svc := NewEncoderOverrideService(repo)
	ctx := context.Background()

	// Create video override
	v := &models.EncoderOverride{
		Name:          "Video Override",
		CodecType:     models.EncoderOverrideCodecTypeVideo,
		SourceCodec:   "h264",
		TargetEncoder: "libx264",
		IsEnabled:     new(true),
	}
	require.NoError(t, svc.Create(ctx, v))

	// Create audio override
	a := &models.EncoderOverride{
		Name:          "Audio Override",
		CodecType:     models.EncoderOverrideCodecTypeAudio,
		SourceCodec:   "aac",
		TargetEncoder: "libfdk_aac",
		IsEnabled:     new(true),
	}
	require.NoError(t, svc.Create(ctx, a))

	videoOverrides, err := svc.GetByCodecType(ctx, models.EncoderOverrideCodecTypeVideo)
	require.NoError(t, err)
	assert.Len(t, videoOverrides, 1)
	assert.Equal(t, "Video Override", videoOverrides[0].Name)

	audioOverrides, err := svc.GetByCodecType(ctx, models.EncoderOverrideCodecTypeAudio)
	require.NoError(t, err)
	assert.Len(t, audioOverrides, 1)
	assert.Equal(t, "Audio Override", audioOverrides[0].Name)
}

func TestEncoderOverrideService_Update(t *testing.T) {
	repo := newMockEncoderOverrideRepo()
	svc := NewEncoderOverrideService(repo)
	ctx := context.Background()

	override := &models.EncoderOverride{
		Name:          "Original",
		CodecType:     models.EncoderOverrideCodecTypeVideo,
		SourceCodec:   "h264",
		TargetEncoder: "libx264",
		IsEnabled:     new(true),
	}
	require.NoError(t, svc.Create(ctx, override))

	// Update
	override.Name = "Updated"
	override.TargetEncoder = "h264_nvenc"
	err := svc.Update(ctx, override)
	require.NoError(t, err)

	// Verify
	found, err := svc.GetByID(ctx, override.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", found.Name)
	assert.Equal(t, "h264_nvenc", found.TargetEncoder)
}

func TestEncoderOverrideService_Update_NotFound(t *testing.T) {
	repo := newMockEncoderOverrideRepo()
	svc := NewEncoderOverrideService(repo)
	ctx := context.Background()

	override := &models.EncoderOverride{
		BaseModel: models.BaseModel{ID: models.NewULID()},
		Name:      "Ghost",
	}
	err := svc.Update(ctx, override)
	assert.ErrorIs(t, err, ErrEncoderOverrideNotFound)
}

func TestEncoderOverrideService_Update_SystemOverride_OnlyEnabledToggle(t *testing.T) {
	repo := newMockEncoderOverrideRepo()
	svc := NewEncoderOverrideService(repo)
	ctx := context.Background()

	// Insert system override directly into repo (bypassing service which blocks IsSystem)
	systemOverride := &models.EncoderOverride{
		BaseModel:     models.BaseModel{ID: models.NewULID()},
		Name:          "System Override",
		Description:   "System desc",
		CodecType:     models.EncoderOverrideCodecTypeVideo,
		SourceCodec:   "h265",
		TargetEncoder: "libx265",
		HWAccelMatch:  "vaapi",
		Priority:      100,
		IsEnabled:     new(true),
		IsSystem:      true,
	}
	repo.overrides = append(repo.overrides, systemOverride)

	// Try to update name - should fail
	attempt := &models.EncoderOverride{
		BaseModel:     models.BaseModel{ID: systemOverride.ID},
		Name:          "Renamed System",
		Description:   "System desc",
		CodecType:     models.EncoderOverrideCodecTypeVideo,
		SourceCodec:   "h265",
		TargetEncoder: "libx265",
		HWAccelMatch:  "vaapi",
		Priority:      100,
		IsEnabled:     new(true),
		IsSystem:      true,
	}
	err := svc.Update(ctx, attempt)
	assert.ErrorIs(t, err, ErrEncoderOverrideCannotEditSystem)

	// Toggle enabled - should work
	toggleAttempt := &models.EncoderOverride{
		BaseModel:     models.BaseModel{ID: systemOverride.ID},
		Name:          "System Override",
		Description:   "System desc",
		CodecType:     models.EncoderOverrideCodecTypeVideo,
		SourceCodec:   "h265",
		TargetEncoder: "libx265",
		HWAccelMatch:  "vaapi",
		Priority:      100,
		IsEnabled:     new(false),
		IsSystem:      true,
	}
	err = svc.Update(ctx, toggleAttempt)
	require.NoError(t, err)

	// Verify enabled was toggled
	found, err := svc.GetByID(ctx, systemOverride.ID)
	require.NoError(t, err)
	assert.False(t, models.BoolVal(found.IsEnabled))
}

func TestEncoderOverrideService_Delete(t *testing.T) {
	repo := newMockEncoderOverrideRepo()
	svc := NewEncoderOverrideService(repo)
	ctx := context.Background()

	override := &models.EncoderOverride{
		Name:          "To Delete",
		CodecType:     models.EncoderOverrideCodecTypeVideo,
		SourceCodec:   "h264",
		TargetEncoder: "libx264",
		IsEnabled:     new(true),
	}
	require.NoError(t, svc.Create(ctx, override))

	err := svc.Delete(ctx, override.ID)
	require.NoError(t, err)

	// Verify deleted
	_, err = svc.GetByID(ctx, override.ID)
	assert.ErrorIs(t, err, ErrEncoderOverrideNotFound)
}

func TestEncoderOverrideService_Delete_SystemOverride(t *testing.T) {
	repo := newMockEncoderOverrideRepo()
	svc := NewEncoderOverrideService(repo)
	ctx := context.Background()

	// Insert system override directly
	systemOverride := &models.EncoderOverride{
		BaseModel:     models.BaseModel{ID: models.NewULID()},
		Name:          "System Override",
		CodecType:     models.EncoderOverrideCodecTypeVideo,
		SourceCodec:   "h265",
		TargetEncoder: "libx265",
		IsEnabled:     new(true),
		IsSystem:      true,
	}
	repo.overrides = append(repo.overrides, systemOverride)

	err := svc.Delete(ctx, systemOverride.ID)
	assert.ErrorIs(t, err, ErrEncoderOverrideCannotDeleteSystem)

	// Verify still exists
	found, err := svc.GetByID(ctx, systemOverride.ID)
	require.NoError(t, err)
	assert.Equal(t, "System Override", found.Name)
}

func TestEncoderOverrideService_Delete_NotFound(t *testing.T) {
	repo := newMockEncoderOverrideRepo()
	svc := NewEncoderOverrideService(repo)
	ctx := context.Background()

	err := svc.Delete(ctx, models.NewULID())
	assert.ErrorIs(t, err, ErrEncoderOverrideNotFound)
}

func TestEncoderOverrideService_ToggleEnabled(t *testing.T) {
	repo := newMockEncoderOverrideRepo()
	svc := NewEncoderOverrideService(repo)
	ctx := context.Background()

	override := &models.EncoderOverride{
		Name:          "Toggle Me",
		CodecType:     models.EncoderOverrideCodecTypeVideo,
		SourceCodec:   "h264",
		TargetEncoder: "libx264",
		IsEnabled:     new(true),
	}
	require.NoError(t, svc.Create(ctx, override))

	// Toggle off
	toggled, err := svc.ToggleEnabled(ctx, override.ID)
	require.NoError(t, err)
	assert.False(t, models.BoolVal(toggled.IsEnabled))

	// Toggle on
	toggled, err = svc.ToggleEnabled(ctx, override.ID)
	require.NoError(t, err)
	assert.True(t, models.BoolVal(toggled.IsEnabled))
}

func TestEncoderOverrideService_ToggleEnabled_NotFound(t *testing.T) {
	repo := newMockEncoderOverrideRepo()
	svc := NewEncoderOverrideService(repo)
	ctx := context.Background()

	_, err := svc.ToggleEnabled(ctx, models.NewULID())
	assert.ErrorIs(t, err, ErrEncoderOverrideNotFound)
}

func TestEncoderOverrideService_Reorder(t *testing.T) {
	repo := newMockEncoderOverrideRepo()
	svc := NewEncoderOverrideService(repo)
	ctx := context.Background()

	o1 := &models.EncoderOverride{
		Name:          "Override A",
		CodecType:     models.EncoderOverrideCodecTypeVideo,
		SourceCodec:   "h264",
		TargetEncoder: "libx264",
		IsEnabled:     new(true),
	}
	require.NoError(t, svc.Create(ctx, o1))

	o2 := &models.EncoderOverride{
		Name:          "Override B",
		CodecType:     models.EncoderOverrideCodecTypeVideo,
		SourceCodec:   "h265",
		TargetEncoder: "libx265",
		IsEnabled:     new(true),
	}
	require.NoError(t, svc.Create(ctx, o2))

	// Swap priorities
	err := svc.Reorder(ctx, []repository.ReorderRequest{
		{ID: o1.ID, Priority: 200},
		{ID: o2.ID, Priority: 100},
	})
	require.NoError(t, err)

	// Verify
	found1, err := svc.GetByID(ctx, o1.ID)
	require.NoError(t, err)
	assert.Equal(t, 200, found1.Priority)

	found2, err := svc.GetByID(ctx, o2.ID)
	require.NoError(t, err)
	assert.Equal(t, 100, found2.Priority)
}

func TestEncoderOverrideService_RefreshCache(t *testing.T) {
	repo := newMockEncoderOverrideRepo()
	svc := NewEncoderOverrideService(repo)
	ctx := context.Background()

	// Initially cache should be empty
	protos := svc.GetEnabledProto()
	assert.Nil(t, protos)

	// Add an enabled override (Create auto-assigns priority as maxPriority + 1)
	o := &models.EncoderOverride{
		Name:          "Cached Override",
		CodecType:     models.EncoderOverrideCodecTypeVideo,
		SourceCodec:   "h265",
		TargetEncoder: "libx265",
		HWAccelMatch:  "vaapi",
		CPUMatch:      "AMD",
		IsEnabled:     new(true),
	}
	require.NoError(t, svc.Create(ctx, o))

	// After Create, cache is auto-refreshed
	protos = svc.GetEnabledProto()
	require.Len(t, protos, 1)
	assert.Equal(t, "video", protos[0].CodecType)
	assert.Equal(t, "h265", protos[0].SourceCodec)
	assert.Equal(t, "libx265", protos[0].TargetEncoder)
	assert.Equal(t, "vaapi", protos[0].HwAccelMatch)
	assert.Equal(t, "AMD", protos[0].CpuMatch)
	assert.Equal(t, int32(1), protos[0].Priority)
}

func TestEncoderOverrideService_GetEnabledProto_ExcludesDisabled(t *testing.T) {
	repo := newMockEncoderOverrideRepo()
	svc := NewEncoderOverrideService(repo)
	ctx := context.Background()

	// Create enabled override
	o1 := &models.EncoderOverride{
		Name:          "Enabled",
		CodecType:     models.EncoderOverrideCodecTypeVideo,
		SourceCodec:   "h264",
		TargetEncoder: "libx264",
		IsEnabled:     new(true),
	}
	require.NoError(t, svc.Create(ctx, o1))

	// Create disabled override
	o2 := &models.EncoderOverride{
		Name:          "Disabled",
		CodecType:     models.EncoderOverrideCodecTypeVideo,
		SourceCodec:   "h265",
		TargetEncoder: "libx265",
		IsEnabled:     new(false),
	}
	require.NoError(t, svc.Create(ctx, o2))

	protos := svc.GetEnabledProto()
	assert.Len(t, protos, 1)
	assert.Equal(t, "h264", protos[0].SourceCodec)
}

func TestEncoderOverrideService_GetSystem(t *testing.T) {
	repo := newMockEncoderOverrideRepo()
	svc := NewEncoderOverrideService(repo)
	ctx := context.Background()

	// Insert system override directly
	system := &models.EncoderOverride{
		BaseModel:     models.BaseModel{ID: models.NewULID()},
		Name:          "System",
		CodecType:     models.EncoderOverrideCodecTypeVideo,
		SourceCodec:   "h265",
		TargetEncoder: "libx265",
		IsEnabled:     new(true),
		IsSystem:      true,
	}
	repo.overrides = append(repo.overrides, system)

	// Create user override via service
	user := &models.EncoderOverride{
		Name:          "User",
		CodecType:     models.EncoderOverrideCodecTypeAudio,
		SourceCodec:   "aac",
		TargetEncoder: "libfdk_aac",
		IsEnabled:     new(true),
	}
	require.NoError(t, svc.Create(ctx, user))

	systemOverrides, err := svc.GetSystem(ctx)
	require.NoError(t, err)
	assert.Len(t, systemOverrides, 1)
	assert.Equal(t, "System", systemOverrides[0].Name)
}
