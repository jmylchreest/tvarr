package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamProxy_TableName(t *testing.T) {
	proxy := StreamProxy{}
	assert.Equal(t, "stream_proxies", proxy.TableName())
}

func TestStreamProxy_Validate(t *testing.T) {
	tests := []struct {
		name    string
		proxy   StreamProxy
		wantErr error
	}{
		{
			name:    "empty name",
			proxy:   StreamProxy{},
			wantErr: ErrNameRequired,
		},
		{
			name: "valid proxy with defaults",
			proxy: StreamProxy{
				Name: "Test Proxy",
			},
			wantErr: nil,
		},
		{
			name: "valid proxy with all fields",
			proxy: StreamProxy{
				Name:                  "Full Proxy",
				Description:           "A test proxy with all fields",
				ProxyMode:             StreamProxyModeSmart,
				IsActive:              true,
				AutoRegenerate:        true,
				StartingChannelNumber: 100,
				UpstreamTimeout:       60,
				BufferSize:            16384,
				MaxConcurrentStreams:  10,
				CacheChannelLogos:     true,
				CacheProgramLogos:     true,
				OutputPath:            "/output/proxy",
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.proxy.Validate()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStreamProxyStatus(t *testing.T) {
	tests := []struct {
		name   string
		status StreamProxyStatus
		valid  bool
	}{
		{"pending", StreamProxyStatusPending, true},
		{"generating", StreamProxyStatusGenerating, true},
		{"success", StreamProxyStatusSuccess, true},
		{"failed", StreamProxyStatusFailed, true},
		{"invalid", StreamProxyStatus("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the status constants are valid strings
			if tt.valid {
				assert.NotEmpty(t, string(tt.status))
			}
		})
	}
}

func TestStreamProxyMode(t *testing.T) {
	tests := []struct {
		name  string
		mode  StreamProxyMode
		valid bool
	}{
		{"direct", StreamProxyModeDirect, true},
		{"smart", StreamProxyModeSmart, true},
		{"invalid", StreamProxyMode("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.valid {
				assert.NotEmpty(t, string(tt.mode))
			}
		})
	}
}

func TestIsValidProxyMode(t *testing.T) {
	tests := []struct {
		mode  StreamProxyMode
		valid bool
	}{
		{StreamProxyModeDirect, true},
		{StreamProxyModeSmart, true},
		{StreamProxyMode("unknown"), false},
		{StreamProxyMode("redirect"), false},
		{StreamProxyMode("proxy"), false},
		{StreamProxyMode("relay"), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			assert.Equal(t, tt.valid, IsValidProxyMode(tt.mode))
		})
	}
}

func TestStreamProxy_MarkGenerating(t *testing.T) {
	proxy := StreamProxy{
		Name:      "Test",
		Status:    StreamProxyStatusPending,
		LastError: "previous error",
	}

	proxy.MarkGenerating()

	assert.Equal(t, StreamProxyStatusGenerating, proxy.Status)
	assert.Empty(t, proxy.LastError)
}

func TestStreamProxy_MarkSuccess(t *testing.T) {
	proxy := StreamProxy{
		Name:         "Test",
		Status:       StreamProxyStatusGenerating,
		ChannelCount: 0,
		ProgramCount: 0,
	}

	proxy.MarkSuccess(100, 5000)

	assert.Equal(t, StreamProxyStatusSuccess, proxy.Status)
	assert.Equal(t, 100, proxy.ChannelCount)
	assert.Equal(t, 5000, proxy.ProgramCount)
	require.NotNil(t, proxy.LastGeneratedAt)
	assert.Empty(t, proxy.LastError)
}

func TestStreamProxy_MarkFailed(t *testing.T) {
	proxy := StreamProxy{
		Name:   "Test",
		Status: StreamProxyStatusGenerating,
	}

	testErr := assert.AnError
	proxy.MarkFailed(testErr)

	assert.Equal(t, StreamProxyStatusFailed, proxy.Status)
	assert.Equal(t, testErr.Error(), proxy.LastError)
}

func TestStreamProxy_MarkFailed_NilError(t *testing.T) {
	proxy := StreamProxy{
		Name:   "Test",
		Status: StreamProxyStatusGenerating,
	}

	proxy.MarkFailed(nil)

	assert.Equal(t, StreamProxyStatusFailed, proxy.Status)
	assert.Empty(t, proxy.LastError)
}

func TestProxySource_TableName(t *testing.T) {
	ps := ProxySource{}
	assert.Equal(t, "proxy_sources", ps.TableName())
}

func TestProxySource_Validate(t *testing.T) {
	tests := []struct {
		name    string
		ps      ProxySource
		wantErr error
	}{
		{
			name:    "missing proxy ID",
			ps:      ProxySource{SourceID: NewULID()},
			wantErr: ErrProxyIDRequired,
		},
		{
			name:    "missing source ID",
			ps:      ProxySource{ProxyID: NewULID()},
			wantErr: ErrSourceIDRequired,
		},
		{
			name: "valid proxy source",
			ps: ProxySource{
				ProxyID:  NewULID(),
				SourceID: NewULID(),
			},
			wantErr: nil,
		},
		{
			name: "valid with priority",
			ps: ProxySource{
				ProxyID:  NewULID(),
				SourceID: NewULID(),
				Priority: 10,
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ps.Validate()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProxyEpgSource_TableName(t *testing.T) {
	pes := ProxyEpgSource{}
	assert.Equal(t, "proxy_epg_sources", pes.TableName())
}

func TestProxyEpgSource_Validate(t *testing.T) {
	tests := []struct {
		name    string
		pes     ProxyEpgSource
		wantErr error
	}{
		{
			name:    "missing proxy ID",
			pes:     ProxyEpgSource{EpgSourceID: NewULID()},
			wantErr: ErrProxyIDRequired,
		},
		{
			name:    "missing EPG source ID",
			pes:     ProxyEpgSource{ProxyID: NewULID()},
			wantErr: ErrEpgSourceIDRequired,
		},
		{
			name: "valid proxy EPG source",
			pes: ProxyEpgSource{
				ProxyID:     NewULID(),
				EpgSourceID: NewULID(),
			},
			wantErr: nil,
		},
		{
			name: "valid with priority",
			pes: ProxyEpgSource{
				ProxyID:     NewULID(),
				EpgSourceID: NewULID(),
				Priority:    5,
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pes.Validate()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProxyFilter_TableName(t *testing.T) {
	pf := ProxyFilter{}
	assert.Equal(t, "proxy_filters", pf.TableName())
}

func TestProxyFilter_Validate(t *testing.T) {
	tests := []struct {
		name    string
		pf      ProxyFilter
		wantErr error
	}{
		{
			name:    "missing proxy ID",
			pf:      ProxyFilter{FilterID: NewULID()},
			wantErr: ErrProxyIDRequired,
		},
		{
			name:    "missing filter ID",
			pf:      ProxyFilter{ProxyID: NewULID()},
			wantErr: ErrFilterIDRequired,
		},
		{
			name: "valid proxy filter",
			pf: ProxyFilter{
				ProxyID:  NewULID(),
				FilterID: NewULID(),
			},
			wantErr: nil,
		},
		{
			name: "valid with priority",
			pf: ProxyFilter{
				ProxyID:  NewULID(),
				FilterID: NewULID(),
				Priority: 10,
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pf.Validate()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProxyMappingRule_TableName(t *testing.T) {
	pmr := ProxyMappingRule{}
	assert.Equal(t, "proxy_mapping_rules", pmr.TableName())
}

func TestProxyMappingRule_Validate(t *testing.T) {
	tests := []struct {
		name    string
		pmr     ProxyMappingRule
		wantErr error
	}{
		{
			name:    "missing proxy ID",
			pmr:     ProxyMappingRule{MappingRuleID: NewULID()},
			wantErr: ErrProxyIDRequired,
		},
		{
			name:    "missing mapping rule ID",
			pmr:     ProxyMappingRule{ProxyID: NewULID()},
			wantErr: ErrMappingRuleIDRequired,
		},
		{
			name: "valid proxy mapping rule",
			pmr: ProxyMappingRule{
				ProxyID:       NewULID(),
				MappingRuleID: NewULID(),
			},
			wantErr: nil,
		},
		{
			name: "valid with priority",
			pmr: ProxyMappingRule{
				ProxyID:       NewULID(),
				MappingRuleID: NewULID(),
				Priority:      5,
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pmr.Validate()
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStreamProxy_Relationships(t *testing.T) {
	// Test that the relationship structs can be composed correctly
	proxy := StreamProxy{
		Name: "Test Proxy",
	}
	proxy.ID = NewULID()

	sourceID := NewULID()
	epgSourceID := NewULID()
	filterID := NewULID()
	mappingRuleID := NewULID()

	proxySources := []ProxySource{
		{ProxyID: proxy.ID, SourceID: sourceID, Priority: 1},
	}
	proxyEpgSources := []ProxyEpgSource{
		{ProxyID: proxy.ID, EpgSourceID: epgSourceID, Priority: 1},
	}
	proxyFilters := []ProxyFilter{
		{ProxyID: proxy.ID, FilterID: filterID, Priority: 1},
	}
	proxyMappingRules := []ProxyMappingRule{
		{ProxyID: proxy.ID, MappingRuleID: mappingRuleID, Priority: 1},
	}

	// Assign relationships
	proxy.Sources = proxySources
	proxy.EpgSources = proxyEpgSources
	proxy.Filters = proxyFilters
	proxy.MappingRules = proxyMappingRules

	// Verify
	require.Len(t, proxy.Sources, 1)
	require.Len(t, proxy.EpgSources, 1)
	require.Len(t, proxy.Filters, 1)
	require.Len(t, proxy.MappingRules, 1)

	assert.Equal(t, sourceID, proxy.Sources[0].SourceID)
	assert.Equal(t, epgSourceID, proxy.EpgSources[0].EpgSourceID)
	assert.Equal(t, filterID, proxy.Filters[0].FilterID)
	assert.Equal(t, mappingRuleID, proxy.MappingRules[0].MappingRuleID)
}
