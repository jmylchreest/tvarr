package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataMappingRule_TableName(t *testing.T) {
	r := DataMappingRule{}
	assert.Equal(t, "data_mapping_rules", r.TableName())
}

func TestDataMappingRule_Validate(t *testing.T) {
	tests := []struct {
		name        string
		rule        DataMappingRule
		wantErr     bool
		errField    string
		errContains string
	}{
		{
			name: "valid stream rule",
			rule: DataMappingRule{
				Name:       "Group Mapping",
				Expression: "channel_name contains 'BBC' SET group_title = 'UK Channels'",
				SourceType: DataMappingRuleSourceTypeStream,
			},
			wantErr: false,
		},
		{
			name: "valid epg rule",
			rule: DataMappingRule{
				Name:       "EPG Mapping",
				Expression: "title matches 'News.*' SET category = 'News'",
				SourceType: DataMappingRuleSourceTypeEPG,
			},
			wantErr: false,
		},
		{
			name: "valid rule with all optional fields",
			rule: DataMappingRule{
				Name:        "Full Rule",
				Description: "A complete rule with all fields",
				Expression:  "channel_name contains 'test' SET group_title = 'Test'",
				SourceType:  DataMappingRuleSourceTypeStream,
				Priority:    10,
				StopOnMatch: true,
				IsEnabled:   new(true),
			},
			wantErr: false,
		},
		{
			name: "missing name",
			rule: DataMappingRule{
				Expression: "some expression",
				SourceType: DataMappingRuleSourceTypeStream,
			},
			wantErr:     true,
			errField:    "name",
			errContains: "name is required",
		},
		{
			name: "missing expression",
			rule: DataMappingRule{
				Name:       "Test",
				SourceType: DataMappingRuleSourceTypeStream,
			},
			wantErr:     true,
			errField:    "expression",
			errContains: "expression is required",
		},
		{
			name: "missing source_type",
			rule: DataMappingRule{
				Name:       "Test",
				Expression: "some expression",
			},
			wantErr:     true,
			errField:    "source_type",
			errContains: "source_type is required",
		},
		{
			name: "invalid source_type",
			rule: DataMappingRule{
				Name:       "Test",
				Expression: "some expression",
				SourceType: DataMappingRuleSourceType("invalid"),
			},
			wantErr:     true,
			errField:    "source_type",
			errContains: "must be 'stream' or 'epg'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.Validate()
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

func TestDataMappingRuleSourceType_Constants(t *testing.T) {
	assert.Equal(t, DataMappingRuleSourceType("stream"), DataMappingRuleSourceTypeStream)
	assert.Equal(t, DataMappingRuleSourceType("epg"), DataMappingRuleSourceTypeEPG)
}

func TestDataMappingRule_IsEnabled_DefaultBehavior(t *testing.T) {
	t.Run("nil IsEnabled treated as true via BoolVal", func(t *testing.T) {
		r := DataMappingRule{
			Name:       "Test",
			Expression: "expr",
			SourceType: DataMappingRuleSourceTypeStream,
			IsEnabled:  nil,
		}
		assert.True(t, BoolVal(r.IsEnabled), "nil IsEnabled should default to true")
	})

	t.Run("explicitly enabled", func(t *testing.T) {
		r := DataMappingRule{
			IsEnabled: new(true),
		}
		assert.True(t, BoolVal(r.IsEnabled))
	})

	t.Run("explicitly disabled", func(t *testing.T) {
		r := DataMappingRule{
			IsEnabled: new(false),
		}
		assert.False(t, BoolVal(r.IsEnabled))
	})
}
