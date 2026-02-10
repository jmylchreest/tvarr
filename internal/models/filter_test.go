package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilter_TableName(t *testing.T) {
	f := Filter{}
	assert.Equal(t, "filters", f.TableName())
}

func TestFilter_Validate(t *testing.T) {
	tests := []struct {
		name        string
		filter      Filter
		wantErr     bool
		errField    string
		errContains string
	}{
		{
			name: "valid stream include filter",
			filter: Filter{
				Name:       "Test Filter",
				Expression: "channel_name contains 'BBC'",
				SourceType: FilterSourceTypeStream,
				Action:     FilterActionInclude,
			},
			wantErr: false,
		},
		{
			name: "valid epg exclude filter",
			filter: Filter{
				Name:       "EPG Filter",
				Expression: "title matches 'News.*'",
				SourceType: FilterSourceTypeEPG,
				Action:     FilterActionExclude,
			},
			wantErr: false,
		},
		{
			name: "empty action defaults to include",
			filter: Filter{
				Name:       "Default Action",
				Expression: "channel_name contains 'test'",
				SourceType: FilterSourceTypeStream,
				Action:     "", // empty
			},
			wantErr: false,
		},
		{
			name: "missing name",
			filter: Filter{
				Expression: "channel_name contains 'test'",
				SourceType: FilterSourceTypeStream,
				Action:     FilterActionInclude,
			},
			wantErr:     true,
			errField:    "name",
			errContains: "name is required",
		},
		{
			name: "missing expression",
			filter: Filter{
				Name:       "Test",
				SourceType: FilterSourceTypeStream,
				Action:     FilterActionInclude,
			},
			wantErr:     true,
			errField:    "expression",
			errContains: "expression is required",
		},
		{
			name: "missing source_type",
			filter: Filter{
				Name:       "Test",
				Expression: "channel_name contains 'test'",
				Action:     FilterActionInclude,
			},
			wantErr:     true,
			errField:    "source_type",
			errContains: "source_type is required",
		},
		{
			name: "invalid source_type",
			filter: Filter{
				Name:       "Test",
				Expression: "channel_name contains 'test'",
				SourceType: FilterSourceType("invalid"),
				Action:     FilterActionInclude,
			},
			wantErr:     true,
			errField:    "source_type",
			errContains: "must be 'stream' or 'epg'",
		},
		{
			name: "invalid action",
			filter: Filter{
				Name:       "Test",
				Expression: "channel_name contains 'test'",
				SourceType: FilterSourceTypeStream,
				Action:     FilterAction("drop"),
			},
			wantErr:     true,
			errField:    "action",
			errContains: "must be 'include' or 'exclude'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.filter.Validate()
			if tt.wantErr {
				require.Error(t, err)
				var valErr ValidationError
				require.ErrorAs(t, err, &valErr)
				assert.Equal(t, tt.errField, valErr.Field)
				assert.Contains(t, valErr.Message, tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFilter_Validate_EmptyActionDefaultsToInclude(t *testing.T) {
	f := Filter{
		Name:       "Test",
		Expression: "group_title == 'Sports'",
		SourceType: FilterSourceTypeStream,
		Action:     "",
	}

	err := f.Validate()
	require.NoError(t, err)
	assert.Equal(t, FilterActionInclude, f.Action, "empty action should default to include")
}

func TestFilterSourceType_Constants(t *testing.T) {
	assert.Equal(t, FilterSourceType("stream"), FilterSourceTypeStream)
	assert.Equal(t, FilterSourceType("epg"), FilterSourceTypeEPG)
}

func TestFilterAction_Constants(t *testing.T) {
	assert.Equal(t, FilterAction("include"), FilterActionInclude)
	assert.Equal(t, FilterAction("exclude"), FilterActionExclude)
}
