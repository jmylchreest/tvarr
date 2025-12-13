package service

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockClientDetectionRuleRepo is an in-memory implementation for testing.
type mockClientDetectionRuleRepo struct {
	rules []*models.ClientDetectionRule
}

func (m *mockClientDetectionRuleRepo) Create(_ context.Context, rule *models.ClientDetectionRule) error {
	m.rules = append(m.rules, rule)
	return nil
}

func (m *mockClientDetectionRuleRepo) GetByID(_ context.Context, id models.ULID) (*models.ClientDetectionRule, error) {
	for _, r := range m.rules {
		if r.BaseModel.ID == id {
			return r, nil
		}
	}
	return nil, nil
}

func (m *mockClientDetectionRuleRepo) GetAll(_ context.Context) ([]*models.ClientDetectionRule, error) {
	return m.rules, nil
}

func (m *mockClientDetectionRuleRepo) GetEnabled(_ context.Context) ([]*models.ClientDetectionRule, error) {
	var enabled []*models.ClientDetectionRule
	for _, r := range m.rules {
		if r.IsEnabled {
			enabled = append(enabled, r)
		}
	}
	// Sort by priority (lower first)
	for i := 0; i < len(enabled)-1; i++ {
		for j := i + 1; j < len(enabled); j++ {
			if enabled[j].Priority < enabled[i].Priority {
				enabled[i], enabled[j] = enabled[j], enabled[i]
			}
		}
	}
	return enabled, nil
}

func (m *mockClientDetectionRuleRepo) GetByName(_ context.Context, name string) (*models.ClientDetectionRule, error) {
	for _, r := range m.rules {
		if r.Name == name {
			return r, nil
		}
	}
	return nil, nil
}

func (m *mockClientDetectionRuleRepo) GetSystem(_ context.Context) ([]*models.ClientDetectionRule, error) {
	var system []*models.ClientDetectionRule
	for _, r := range m.rules {
		if r.IsSystem {
			system = append(system, r)
		}
	}
	return system, nil
}

func (m *mockClientDetectionRuleRepo) Update(_ context.Context, rule *models.ClientDetectionRule) error {
	for i, r := range m.rules {
		if r.BaseModel.ID == rule.BaseModel.ID {
			m.rules[i] = rule
			return nil
		}
	}
	return nil
}

func (m *mockClientDetectionRuleRepo) Delete(_ context.Context, id models.ULID) error {
	for i, r := range m.rules {
		if r.BaseModel.ID == id {
			m.rules = append(m.rules[:i], m.rules[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockClientDetectionRuleRepo) Count(_ context.Context) (int64, error) {
	return int64(len(m.rules)), nil
}

func (m *mockClientDetectionRuleRepo) CountEnabled(_ context.Context) (int64, error) {
	var count int64
	for _, r := range m.rules {
		if r.IsEnabled {
			count++
		}
	}
	return count, nil
}

func (m *mockClientDetectionRuleRepo) Reorder(_ context.Context, _ []repository.ReorderRequest) error {
	return nil
}

func newMockRepo() *mockClientDetectionRuleRepo {
	return &mockClientDetectionRuleRepo{rules: []*models.ClientDetectionRule{}}
}

// TestClientDetectionService_EvaluateRequest_FirstMatchWins tests that
// the first matching rule is returned and evaluation stops.
func TestClientDetectionService_EvaluateRequest_FirstMatchWins(t *testing.T) {
	repo := newMockRepo()
	svc := NewClientDetectionService(repo)

	// Create rules with different priorities
	rule1 := &models.ClientDetectionRule{
		BaseModel:           models.BaseModel{ID: models.NewULID()},
		Name:                "High Priority Chrome",
		Expression:          `@dynamic(request.headers):user-agent contains "Chrome"`,
		Priority:            10,
		IsEnabled:           true,
		AcceptedVideoCodecs: `["h264","h265"]`,
		AcceptedAudioCodecs: `["aac","opus"]`,
		PreferredVideoCodec: models.VideoCodecH265,
		PreferredAudioCodec: models.AudioCodecAAC,
		SupportsFMP4:        true,
		SupportsMPEGTS:      true,
	}
	rule2 := &models.ClientDetectionRule{
		BaseModel:           models.BaseModel{ID: models.NewULID()},
		Name:                "Low Priority Generic",
		Expression:          `@dynamic(request.headers):user-agent contains "Mozilla"`,
		Priority:            100,
		IsEnabled:           true,
		AcceptedVideoCodecs: `["h264"]`,
		AcceptedAudioCodecs: `["aac"]`,
		PreferredVideoCodec: models.VideoCodecH264,
		PreferredAudioCodec: models.AudioCodecAAC,
		SupportsFMP4:        false,
		SupportsMPEGTS:      true,
	}

	// Add in reverse order to ensure priority sorting works
	repo.rules = append(repo.rules, rule2, rule1)

	// Refresh cache
	err := svc.RefreshCache(context.Background())
	require.NoError(t, err)

	// Create request with Chrome User-Agent (matches both rules)
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/120.0")

	// Evaluate
	result := svc.EvaluateRequest(req)

	// Should match the higher priority rule (rule1, Chrome)
	assert.NotNil(t, result.MatchedRule)
	assert.Equal(t, "High Priority Chrome", result.MatchedRule.Name)
	assert.Equal(t, "h265", result.PreferredVideoCodec)
	assert.True(t, result.SupportsFMP4)
}

// TestClientDetectionService_EvaluateRequest_PriorityOrdering tests that
// rules are evaluated in priority order (lower number = higher priority).
func TestClientDetectionService_EvaluateRequest_PriorityOrdering(t *testing.T) {
	repo := newMockRepo()
	svc := NewClientDetectionService(repo)

	// Create rules that would all match, in various priority orders
	ruleAndroidTV := &models.ClientDetectionRule{
		BaseModel:           models.BaseModel{ID: models.NewULID()},
		Name:                "Android TV",
		Expression:          `@dynamic(request.headers):user-agent contains "Android" AND @dynamic(request.headers):user-agent contains "TV"`,
		Priority:            100,
		IsEnabled:           true,
		AcceptedVideoCodecs: `["h264","h265"]`,
		AcceptedAudioCodecs: `["aac","ac3"]`,
		PreferredVideoCodec: models.VideoCodecH264,
		PreferredAudioCodec: models.AudioCodecAAC,
		SupportsFMP4:        true,
		SupportsMPEGTS:      true,
	}
	ruleAndroidMobile := &models.ClientDetectionRule{
		BaseModel:           models.BaseModel{ID: models.NewULID()},
		Name:                "Android Mobile",
		Expression:          `@dynamic(request.headers):user-agent contains "Android"`,
		Priority:            200, // Lower priority than Android TV
		IsEnabled:           true,
		AcceptedVideoCodecs: `["h264"]`,
		AcceptedAudioCodecs: `["aac"]`,
		PreferredVideoCodec: models.VideoCodecH264,
		PreferredAudioCodec: models.AudioCodecAAC,
		SupportsFMP4:        true,
		SupportsMPEGTS:      true,
	}

	// Add in reverse order
	repo.rules = append(repo.rules, ruleAndroidMobile, ruleAndroidTV)

	err := svc.RefreshCache(context.Background())
	require.NoError(t, err)

	// Test 1: Android TV user agent should match Android TV rule
	t.Run("Android TV matches high priority rule", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 12; SHIELD Android TV) AppleWebKit/537.36")

		result := svc.EvaluateRequest(req)

		assert.NotNil(t, result.MatchedRule)
		assert.Equal(t, "Android TV", result.MatchedRule.Name)
	})

	// Test 2: Regular Android should match the mobile rule
	t.Run("Android Mobile matches lower priority rule", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 12; Pixel 6) AppleWebKit/537.36")

		result := svc.EvaluateRequest(req)

		assert.NotNil(t, result.MatchedRule)
		assert.Equal(t, "Android Mobile", result.MatchedRule.Name)
	})
}

// TestClientDetectionService_EvaluateRequest_ExplicitCodecHeaders tests that
// explicit codec header rules (X-Video-Codec, X-Audio-Codec) work correctly.
func TestClientDetectionService_EvaluateRequest_ExplicitCodecHeaders(t *testing.T) {
	repo := newMockRepo()
	svc := NewClientDetectionService(repo)

	// Create explicit codec header rules (highest priority)
	ruleH265Header := &models.ClientDetectionRule{
		BaseModel:           models.BaseModel{ID: models.NewULID()},
		Name:                "Explicit H.265 Request",
		Expression:          `@dynamic(request.headers):x-video-codec equals "h265"`,
		Priority:            1, // Highest priority
		IsEnabled:           true,
		AcceptedVideoCodecs: `["h265"]`,
		AcceptedAudioCodecs: `["aac","opus"]`,
		PreferredVideoCodec: models.VideoCodecH265,
		PreferredAudioCodec: models.AudioCodecAAC,
		SupportsFMP4:        true,
		SupportsMPEGTS:      true,
	}

	ruleChromeUA := &models.ClientDetectionRule{
		BaseModel:           models.BaseModel{ID: models.NewULID()},
		Name:                "Chrome Browser",
		Expression:          `@dynamic(request.headers):user-agent contains "Chrome"`,
		Priority:            160, // Lower priority than explicit header
		IsEnabled:           true,
		AcceptedVideoCodecs: `["h264","vp9"]`,
		AcceptedAudioCodecs: `["aac","mp3"]`,
		PreferredVideoCodec: models.VideoCodecH264,
		PreferredAudioCodec: models.AudioCodecAAC,
		SupportsFMP4:        true,
		SupportsMPEGTS:      true,
	}

	repo.rules = append(repo.rules, ruleChromeUA, ruleH265Header)

	err := svc.RefreshCache(context.Background())
	require.NoError(t, err)

	t.Run("Explicit H.265 header takes priority over User-Agent", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/120.0")
		req.Header.Set("X-Video-Codec", "h265")

		result := svc.EvaluateRequest(req)

		assert.NotNil(t, result.MatchedRule)
		assert.Equal(t, "Explicit H.265 Request", result.MatchedRule.Name)
		assert.Equal(t, "h265", result.PreferredVideoCodec)
	})

	t.Run("Without explicit header, User-Agent rule matches", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/120.0")

		result := svc.EvaluateRequest(req)

		assert.NotNil(t, result.MatchedRule)
		assert.Equal(t, "Chrome Browser", result.MatchedRule.Name)
		assert.Equal(t, "h264", result.PreferredVideoCodec)
	})
}

// TestClientDetectionService_EvaluateRequest_NoMatch tests default behavior
// when no rules match.
func TestClientDetectionService_EvaluateRequest_NoMatch(t *testing.T) {
	repo := newMockRepo()
	svc := NewClientDetectionService(repo)

	// Add a rule that won't match
	rule := &models.ClientDetectionRule{
		BaseModel:           models.BaseModel{ID: models.NewULID()},
		Name:                "VLC Only",
		Expression:          `@dynamic(request.headers):user-agent contains "VLC"`,
		Priority:            100,
		IsEnabled:           true,
		AcceptedVideoCodecs: `["h264","h265"]`,
		AcceptedAudioCodecs: `["aac"]`,
		PreferredVideoCodec: models.VideoCodecH264,
		PreferredAudioCodec: models.AudioCodecAAC,
		SupportsFMP4:        true,
		SupportsMPEGTS:      true,
	}
	repo.rules = append(repo.rules, rule)

	err := svc.RefreshCache(context.Background())
	require.NoError(t, err)

	// Request with non-matching user agent
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 Firefox/120.0")

	result := svc.EvaluateRequest(req)

	// Should return default result
	assert.Nil(t, result.MatchedRule)
	assert.Equal(t, "default", result.DetectionSource)
	assert.Equal(t, "h264", result.PreferredVideoCodec)
	assert.Equal(t, "aac", result.PreferredAudioCodec)
}

// TestClientDetectionService_EvaluateRequest_DisabledRule tests that
// disabled rules are skipped.
func TestClientDetectionService_EvaluateRequest_DisabledRule(t *testing.T) {
	repo := newMockRepo()
	svc := NewClientDetectionService(repo)

	// Disabled rule (would match)
	disabledRule := &models.ClientDetectionRule{
		BaseModel:           models.BaseModel{ID: models.NewULID()},
		Name:                "Disabled Chrome Rule",
		Expression:          `@dynamic(request.headers):user-agent contains "Chrome"`,
		Priority:            10,
		IsEnabled:           false, // Disabled
		AcceptedVideoCodecs: `["h265"]`,
		AcceptedAudioCodecs: `["opus"]`,
		PreferredVideoCodec: models.VideoCodecH265,
		PreferredAudioCodec: models.AudioCodecOpus,
		SupportsFMP4:        true,
		SupportsMPEGTS:      false,
	}

	// Enabled rule (lower priority)
	enabledRule := &models.ClientDetectionRule{
		BaseModel:           models.BaseModel{ID: models.NewULID()},
		Name:                "Enabled Generic Rule",
		Expression:          `@dynamic(request.headers):user-agent contains "Mozilla"`,
		Priority:            100,
		IsEnabled:           true,
		AcceptedVideoCodecs: `["h264"]`,
		AcceptedAudioCodecs: `["aac"]`,
		PreferredVideoCodec: models.VideoCodecH264,
		PreferredAudioCodec: models.AudioCodecAAC,
		SupportsFMP4:        true,
		SupportsMPEGTS:      true,
	}

	repo.rules = append(repo.rules, enabledRule, disabledRule)

	err := svc.RefreshCache(context.Background())
	require.NoError(t, err)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/120.0")

	result := svc.EvaluateRequest(req)

	// Should match the enabled rule, not the disabled one
	assert.NotNil(t, result.MatchedRule)
	assert.Equal(t, "Enabled Generic Rule", result.MatchedRule.Name)
}

// TestClientDetectionService_ExplicitHeaderPriorityOverUserAgent tests that
// explicit codec header rules (priority 1-10) take precedence over User-Agent based rules.
// This is T046 - User Story 5: Explicit Codec Headers.
func TestClientDetectionService_ExplicitHeaderPriorityOverUserAgent(t *testing.T) {
	repo := newMockRepo()
	svc := NewClientDetectionService(repo)

	// Create explicit codec header rules (highest priority, 1-10)
	ruleExplicitH265 := &models.ClientDetectionRule{
		BaseModel:           models.BaseModel{ID: models.NewULID()},
		Name:                "Explicit H.265 Video Request",
		Expression:          `@dynamic(request.headers):x-video-codec equals "h265" OR @dynamic(request.headers):x-video-codec equals "hevc"`,
		Priority:            1, // Highest priority
		IsEnabled:           true,
		AcceptedVideoCodecs: `["h265"]`,
		AcceptedAudioCodecs: `["aac","opus","ac3","eac3"]`,
		PreferredVideoCodec: models.VideoCodecH265,
		PreferredAudioCodec: models.AudioCodecAAC,
		SupportsFMP4:        true,
		SupportsMPEGTS:      true,
	}

	ruleExplicitH264 := &models.ClientDetectionRule{
		BaseModel:           models.BaseModel{ID: models.NewULID()},
		Name:                "Explicit H.264 Video Request",
		Expression:          `@dynamic(request.headers):x-video-codec equals "h264" OR @dynamic(request.headers):x-video-codec equals "avc"`,
		Priority:            2,
		IsEnabled:           true,
		AcceptedVideoCodecs: `["h264"]`,
		AcceptedAudioCodecs: `["aac","mp3","ac3"]`,
		PreferredVideoCodec: models.VideoCodecH264,
		PreferredAudioCodec: models.AudioCodecAAC,
		SupportsFMP4:        true,
		SupportsMPEGTS:      true,
	}

	ruleExplicitVP9 := &models.ClientDetectionRule{
		BaseModel:           models.BaseModel{ID: models.NewULID()},
		Name:                "Explicit VP9 Video Request",
		Expression:          `@dynamic(request.headers):x-video-codec equals "vp9"`,
		Priority:            3,
		IsEnabled:           true,
		AcceptedVideoCodecs: `["vp9"]`,
		AcceptedAudioCodecs: `["opus","aac"]`,
		PreferredVideoCodec: models.VideoCodecVP9,
		PreferredAudioCodec: models.AudioCodecOpus,
		SupportsFMP4:        true,
		SupportsMPEGTS:      false, // VP9 requires fMP4
	}

	ruleExplicitAV1 := &models.ClientDetectionRule{
		BaseModel:           models.BaseModel{ID: models.NewULID()},
		Name:                "Explicit AV1 Video Request",
		Expression:          `@dynamic(request.headers):x-video-codec equals "av1"`,
		Priority:            4,
		IsEnabled:           true,
		AcceptedVideoCodecs: `["av1"]`,
		AcceptedAudioCodecs: `["opus","aac"]`,
		PreferredVideoCodec: models.VideoCodecAV1,
		PreferredAudioCodec: models.AudioCodecOpus,
		SupportsFMP4:        true,
		SupportsMPEGTS:      false, // AV1 requires fMP4
	}

	// User-Agent based rules (lower priority, 100+)
	ruleChrome := &models.ClientDetectionRule{
		BaseModel:           models.BaseModel{ID: models.NewULID()},
		Name:                "Chrome Browser",
		Expression:          `@dynamic(request.headers):user-agent contains "Chrome" AND NOT @dynamic(request.headers):user-agent contains "Edge"`,
		Priority:            160,
		IsEnabled:           true,
		AcceptedVideoCodecs: `["h264","vp9","av1"]`,
		AcceptedAudioCodecs: `["aac","mp3","opus"]`,
		PreferredVideoCodec: models.VideoCodecH264,
		PreferredAudioCodec: models.AudioCodecAAC,
		SupportsFMP4:        true,
		SupportsMPEGTS:      true,
	}

	ruleSafari := &models.ClientDetectionRule{
		BaseModel:           models.BaseModel{ID: models.NewULID()},
		Name:                "Safari Browser",
		Expression:          `@dynamic(request.headers):user-agent contains "Safari" AND @dynamic(request.headers):user-agent contains "Macintosh"`,
		Priority:            180,
		IsEnabled:           true,
		AcceptedVideoCodecs: `["h264","h265"]`,
		AcceptedAudioCodecs: `["aac","mp3"]`,
		PreferredVideoCodec: models.VideoCodecH264,
		PreferredAudioCodec: models.AudioCodecAAC,
		SupportsFMP4:        true,
		SupportsMPEGTS:      false, // Safari prefers fMP4
	}

	// Add rules in random order to test priority sorting
	repo.rules = append(repo.rules, ruleChrome, ruleExplicitVP9, ruleSafari, ruleExplicitH265, ruleExplicitH264, ruleExplicitAV1)

	err := svc.RefreshCache(context.Background())
	require.NoError(t, err)

	tests := []struct {
		name                 string
		userAgent            string
		videoCodecHeader     string
		expectedRuleName     string
		expectedVideoCodec   string
		expectedSupportsFMP4 bool
	}{
		{
			name:                 "Chrome with X-Video-Codec h265 gets explicit rule",
			userAgent:            "Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 Chrome/120.0",
			videoCodecHeader:     "h265",
			expectedRuleName:     "Explicit H.265 Video Request",
			expectedVideoCodec:   "h265",
			expectedSupportsFMP4: true,
		},
		{
			name:                 "Chrome with X-Video-Codec hevc alias gets explicit rule",
			userAgent:            "Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 Chrome/120.0",
			videoCodecHeader:     "hevc",
			expectedRuleName:     "Explicit H.265 Video Request",
			expectedVideoCodec:   "h265",
			expectedSupportsFMP4: true,
		},
		{
			name:                 "Safari with X-Video-Codec vp9 gets explicit VP9 rule",
			userAgent:            "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Safari/605.1.15",
			videoCodecHeader:     "vp9",
			expectedRuleName:     "Explicit VP9 Video Request",
			expectedVideoCodec:   "vp9",
			expectedSupportsFMP4: true,
		},
		{
			name:                 "Safari with X-Video-Codec av1 gets explicit AV1 rule",
			userAgent:            "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Safari/605.1.15",
			videoCodecHeader:     "av1",
			expectedRuleName:     "Explicit AV1 Video Request",
			expectedVideoCodec:   "av1",
			expectedSupportsFMP4: true,
		},
		{
			name:                 "Chrome without codec header uses User-Agent rule",
			userAgent:            "Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 Chrome/120.0",
			videoCodecHeader:     "",
			expectedRuleName:     "Chrome Browser",
			expectedVideoCodec:   "h264",
			expectedSupportsFMP4: true,
		},
		{
			name:                 "Safari without codec header uses User-Agent rule",
			userAgent:            "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Safari/605.1.15",
			videoCodecHeader:     "",
			expectedRuleName:     "Safari Browser",
			expectedVideoCodec:   "h264",
			expectedSupportsFMP4: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/stream", nil)
			req.Header.Set("User-Agent", tt.userAgent)
			if tt.videoCodecHeader != "" {
				req.Header.Set("X-Video-Codec", tt.videoCodecHeader)
			}

			result := svc.EvaluateRequest(req)

			require.NotNil(t, result.MatchedRule, "Expected a rule to match")
			assert.Equal(t, tt.expectedRuleName, result.MatchedRule.Name)
			assert.Equal(t, tt.expectedVideoCodec, result.PreferredVideoCodec)
			assert.Equal(t, tt.expectedSupportsFMP4, result.SupportsFMP4)
		})
	}
}

// TestClientDetectionService_InvalidCodecHeaderFallthrough tests that invalid or
// unrecognized codec header values fall through to User-Agent based rules.
// This is T047 - User Story 5: Explicit Codec Headers.
func TestClientDetectionService_InvalidCodecHeaderFallthrough(t *testing.T) {
	repo := newMockRepo()
	svc := NewClientDetectionService(repo)

	// Explicit codec header rules only match valid codec values
	ruleExplicitH265 := &models.ClientDetectionRule{
		BaseModel:           models.BaseModel{ID: models.NewULID()},
		Name:                "Explicit H.265 Video Request",
		Expression:          `@dynamic(request.headers):x-video-codec equals "h265" OR @dynamic(request.headers):x-video-codec equals "hevc"`,
		Priority:            1,
		IsEnabled:           true,
		AcceptedVideoCodecs: `["h265"]`,
		AcceptedAudioCodecs: `["aac"]`,
		PreferredVideoCodec: models.VideoCodecH265,
		PreferredAudioCodec: models.AudioCodecAAC,
		SupportsFMP4:        true,
		SupportsMPEGTS:      true,
	}

	ruleExplicitH264 := &models.ClientDetectionRule{
		BaseModel:           models.BaseModel{ID: models.NewULID()},
		Name:                "Explicit H.264 Video Request",
		Expression:          `@dynamic(request.headers):x-video-codec equals "h264" OR @dynamic(request.headers):x-video-codec equals "avc"`,
		Priority:            2,
		IsEnabled:           true,
		AcceptedVideoCodecs: `["h264"]`,
		AcceptedAudioCodecs: `["aac"]`,
		PreferredVideoCodec: models.VideoCodecH264,
		PreferredAudioCodec: models.AudioCodecAAC,
		SupportsFMP4:        true,
		SupportsMPEGTS:      true,
	}

	// Fallback User-Agent rule
	ruleChrome := &models.ClientDetectionRule{
		BaseModel:           models.BaseModel{ID: models.NewULID()},
		Name:                "Chrome Browser",
		Expression:          `@dynamic(request.headers):user-agent contains "Chrome"`,
		Priority:            160,
		IsEnabled:           true,
		AcceptedVideoCodecs: `["h264","vp9"]`,
		AcceptedAudioCodecs: `["aac","mp3"]`,
		PreferredVideoCodec: models.VideoCodecH264,
		PreferredAudioCodec: models.AudioCodecAAC,
		SupportsFMP4:        true,
		SupportsMPEGTS:      true,
	}

	repo.rules = append(repo.rules, ruleExplicitH265, ruleExplicitH264, ruleChrome)

	err := svc.RefreshCache(context.Background())
	require.NoError(t, err)

	tests := []struct {
		name               string
		videoCodecHeader   string
		expectedRuleName   string
		expectedVideoCodec string
		description        string
	}{
		{
			name:               "Invalid codec value falls through",
			videoCodecHeader:   "invalid_codec",
			expectedRuleName:   "Chrome Browser",
			expectedVideoCodec: "h264",
			description:        "Invalid codec value should not match explicit rules",
		},
		{
			name:               "Typo in codec name falls through",
			videoCodecHeader:   "h265x",
			expectedRuleName:   "Chrome Browser",
			expectedVideoCodec: "h264",
			description:        "Typo in codec name should not match",
		},
		{
			name:               "Case mismatch falls through",
			videoCodecHeader:   "H265",
			expectedRuleName:   "Chrome Browser",
			expectedVideoCodec: "h264",
			description:        "Uppercase codec value should not match lowercase rule",
		},
		{
			name:               "Empty string falls through",
			videoCodecHeader:   "",
			expectedRuleName:   "Chrome Browser",
			expectedVideoCodec: "h264",
			description:        "Empty codec header should fall through to User-Agent",
		},
		{
			name:               "Whitespace codec falls through",
			videoCodecHeader:   "  h265  ",
			expectedRuleName:   "Chrome Browser",
			expectedVideoCodec: "h264",
			description:        "Whitespace-padded codec should not match",
		},
		{
			name:               "Valid h265 codec matches explicit rule",
			videoCodecHeader:   "h265",
			expectedRuleName:   "Explicit H.265 Video Request",
			expectedVideoCodec: "h265",
			description:        "Valid codec should match explicit rule",
		},
		{
			name:               "Valid h264 codec matches explicit rule",
			videoCodecHeader:   "h264",
			expectedRuleName:   "Explicit H.264 Video Request",
			expectedVideoCodec: "h264",
			description:        "Valid codec should match explicit rule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/stream", nil)
			req.Header.Set("User-Agent", "Mozilla/5.0 Chrome/120.0")
			if tt.videoCodecHeader != "" {
				req.Header.Set("X-Video-Codec", tt.videoCodecHeader)
			}

			result := svc.EvaluateRequest(req)

			require.NotNil(t, result.MatchedRule, "Expected a rule to match for: %s", tt.description)
			assert.Equal(t, tt.expectedRuleName, result.MatchedRule.Name, tt.description)
			assert.Equal(t, tt.expectedVideoCodec, result.PreferredVideoCodec, tt.description)
		})
	}
}

// TestClientDetectionService_TestExpression tests the expression testing functionality.
func TestClientDetectionService_TestExpression(t *testing.T) {
	repo := newMockRepo()
	svc := NewClientDetectionService(repo)

	tests := []struct {
		name       string
		expression string
		userAgent  string
		expected   bool
		wantErr    bool
	}{
		{
			name:       "Simple contains match",
			expression: `@dynamic(request.headers):user-agent contains "Chrome"`,
			userAgent:  "Mozilla/5.0 Chrome/120.0",
			expected:   true,
		},
		{
			name:       "Simple contains no match",
			expression: `@dynamic(request.headers):user-agent contains "Firefox"`,
			userAgent:  "Mozilla/5.0 Chrome/120.0",
			expected:   false,
		},
		{
			name:       "AND expression both true",
			expression: `@dynamic(request.headers):user-agent contains "Android" AND @dynamic(request.headers):user-agent contains "TV"`,
			userAgent:  "Mozilla/5.0 (Linux; Android 12; SHIELD Android TV)",
			expected:   true,
		},
		{
			name:       "AND expression one false",
			expression: `@dynamic(request.headers):user-agent contains "Android" AND @dynamic(request.headers):user-agent contains "TV"`,
			userAgent:  "Mozilla/5.0 (Linux; Android 12; Pixel 6)",
			expected:   false,
		},
		{
			name:       "OR expression one true",
			expression: `@dynamic(request.headers):user-agent contains "iPhone" OR @dynamic(request.headers):user-agent contains "iPad"`,
			userAgent:  "Mozilla/5.0 (iPad; CPU OS 16_0 like Mac OS X)",
			expected:   true,
		},
		{
			name:       "NOT expression",
			expression: `@dynamic(request.headers):user-agent contains "Android" AND NOT @dynamic(request.headers):user-agent contains "TV"`,
			userAgent:  "Mozilla/5.0 (Linux; Android 12; Pixel 6)",
			expected:   true,
		},
		{
			name:       "Invalid expression",
			expression: `invalid syntax [[[`,
			userAgent:  "any",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("User-Agent", tt.userAgent)

			result, err := svc.TestExpression(tt.expression, req)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestClientDetectionService_DynamicCodecHeaders(t *testing.T) {
	ctx := context.Background()

	// Create rules with dynamic header extraction using SET actions
	// The SET action extracts the header value and assigns it to preferred_video_codec/preferred_audio_codec
	repo := &mockClientDetectionRuleRepo{
		rules: []*models.ClientDetectionRule{
			{
				BaseModel:           models.BaseModel{ID: models.NewULID()},
				Name:                "Dynamic Video Codec",
				Expression:          `@dynamic(request.headers):x-video-codec not_equals "" SET preferred_video_codec = @dynamic(request.headers):x-video-codec`,
				Priority:            10,
				IsEnabled:           true,
				PreferredVideoCodec: "h264", // Fallback if SET value is invalid
				AcceptedVideoCodecs: `["h264"]`,
				AcceptedAudioCodecs: `["aac"]`,
				SupportsFMP4:        true,
				SupportsMPEGTS:      true,
			},
			{
				BaseModel:           models.BaseModel{ID: models.NewULID()},
				Name:                "Dynamic Audio Codec",
				Expression:          `@dynamic(request.headers):x-audio-codec not_equals "" SET preferred_audio_codec = @dynamic(request.headers):x-audio-codec`,
				Priority:            20,
				IsEnabled:           true,
				PreferredAudioCodec: "aac", // Fallback if SET value is invalid
				AcceptedVideoCodecs: `["h264"]`,
				AcceptedAudioCodecs: `["aac"]`,
				SupportsFMP4:        true,
				SupportsMPEGTS:      true,
			},
			{
				BaseModel:           models.BaseModel{ID: models.NewULID()},
				Name:                "Fallback Rule",
				Expression:          `@dynamic(request.headers):user-agent not_equals ""`, // Always matches if there's a user agent
				Priority:            100,
				IsEnabled:           true,
				PreferredVideoCodec: "h264",
				PreferredAudioCodec: "aac",
				AcceptedVideoCodecs: `["h264"]`,
				AcceptedAudioCodecs: `["aac"]`,
				SupportsFMP4:        true,
				SupportsMPEGTS:      true,
			},
		},
	}

	svc := NewClientDetectionService(repo)
	require.NoError(t, svc.RefreshCache(ctx))

	tests := []struct {
		name               string
		videoCodecHeader   string
		audioCodecHeader   string
		expectedVideoCodec string
		expectedAudioCodec string
	}{
		{
			name:               "Valid H.265 video codec from header",
			videoCodecHeader:   "h265",
			audioCodecHeader:   "",
			expectedVideoCodec: "h265",
			expectedAudioCodec: "aac", // Falls back to static value
		},
		{
			name:               "Valid HEVC alias from header",
			videoCodecHeader:   "hevc",
			audioCodecHeader:   "",
			expectedVideoCodec: "h265", // Normalized from alias
			expectedAudioCodec: "aac",
		},
		{
			name:               "Valid VP9 video codec from header",
			videoCodecHeader:   "vp9",
			audioCodecHeader:   "",
			expectedVideoCodec: "vp9",
			expectedAudioCodec: "aac",
		},
		{
			name:               "Valid audio codec from header",
			videoCodecHeader:   "",
			audioCodecHeader:   "opus",
			expectedVideoCodec: "h264", // Falls back to static value
			expectedAudioCodec: "opus",
		},
		{
			name:               "Both video and audio from headers",
			videoCodecHeader:   "av1",
			audioCodecHeader:   "eac3",
			expectedVideoCodec: "av1",
			expectedAudioCodec: "eac3",
		},
		{
			name:               "Invalid video codec falls back to static",
			videoCodecHeader:   "invalid_codec",
			audioCodecHeader:   "",
			expectedVideoCodec: "h264", // Fallback
			expectedAudioCodec: "aac",
		},
		{
			name:               "Case insensitive video codec",
			videoCodecHeader:   "H265",
			audioCodecHeader:   "",
			expectedVideoCodec: "h265",
			expectedAudioCodec: "aac",
		},
		{
			name:               "Whitespace trimmed from codec value",
			videoCodecHeader:   "  h265  ",
			audioCodecHeader:   "",
			expectedVideoCodec: "h265",
			expectedAudioCodec: "aac",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/stream", nil)
			if tt.videoCodecHeader != "" {
				req.Header.Set("X-Video-Codec", tt.videoCodecHeader)
			}
			if tt.audioCodecHeader != "" {
				req.Header.Set("X-Audio-Codec", tt.audioCodecHeader)
			}

			result := svc.EvaluateRequest(req)

			assert.Equal(t, tt.expectedVideoCodec, result.PreferredVideoCodec,
				"expected video codec %s, got %s", tt.expectedVideoCodec, result.PreferredVideoCodec)
			assert.Equal(t, tt.expectedAudioCodec, result.PreferredAudioCodec,
				"expected audio codec %s, got %s", tt.expectedAudioCodec, result.PreferredAudioCodec)
		})
	}
}
