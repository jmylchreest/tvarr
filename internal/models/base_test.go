package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBoolPtr(t *testing.T) {
	tests := []struct {
		name  string
		input bool
	}{
		{"true", true},
		{"false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ptr := new(tt.input)
			require.NotNil(t, ptr)
			assert.Equal(t, tt.input, *ptr)
		})
	}
}

func TestBoolVal(t *testing.T) {
	tests := []struct {
		name     string
		input    *bool
		expected bool
	}{
		{"nil defaults to true", nil, true},
		{"true pointer", new(true), true},
		{"false pointer", new(false), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, BoolVal(tt.input))
		})
	}
}

func TestNewULID(t *testing.T) {
	id := NewULID()
	assert.False(t, id.IsZero(), "NewULID should generate a non-zero ID")

	// Two ULIDs should be different
	id2 := NewULID()
	assert.NotEqual(t, id, id2, "two NewULID calls should produce different IDs")
}

func TestParseULID(t *testing.T) {
	t.Run("valid ULID string", func(t *testing.T) {
		original := NewULID()
		parsed, err := ParseULID(original.String())
		require.NoError(t, err)
		assert.Equal(t, original, parsed)
	})

	t.Run("invalid ULID string", func(t *testing.T) {
		_, err := ParseULID("not-a-valid-ulid")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid ULID")
	})

	t.Run("empty string", func(t *testing.T) {
		_, err := ParseULID("")
		assert.Error(t, err)
	})
}

func TestULID_String_Roundtrip(t *testing.T) {
	original := NewULID()
	s := original.String()
	assert.Len(t, s, 26, "ULID string should be 26 characters")

	parsed, err := ParseULID(s)
	require.NoError(t, err)
	assert.Equal(t, original, parsed)
}

func TestULID_IsZero(t *testing.T) {
	t.Run("zero ULID", func(t *testing.T) {
		var zero ULID
		assert.True(t, zero.IsZero())
	})

	t.Run("non-zero ULID", func(t *testing.T) {
		id := NewULID()
		assert.False(t, id.IsZero())
	})
}

func TestULID_Value(t *testing.T) {
	t.Run("zero ULID returns nil", func(t *testing.T) {
		var zero ULID
		val, err := zero.Value()
		require.NoError(t, err)
		assert.Nil(t, val)
	})

	t.Run("non-zero ULID returns string", func(t *testing.T) {
		id := NewULID()
		val, err := id.Value()
		require.NoError(t, err)
		assert.Equal(t, id.String(), val)
	})
}

func TestULID_Scan(t *testing.T) {
	validID := NewULID()
	validStr := validID.String()

	tests := []struct {
		name      string
		input     any
		expected  ULID
		expectErr bool
	}{
		{"nil sets zero", nil, ULID{}, false},
		{"valid string", validStr, validID, false},
		{"empty string sets zero", "", ULID{}, false},
		{"valid []byte", []byte(validStr), validID, false},
		{"empty []byte sets zero", []byte{}, ULID{}, false},
		{"invalid string", "bad-ulid", ULID{}, true},
		{"invalid []byte", []byte("bad-ulid"), ULID{}, true},
		{"unsupported type int", 12345, ULID{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var u ULID
			err := u.Scan(tt.input)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, u)
			}
		})
	}
}

func TestULID_MarshalJSON(t *testing.T) {
	t.Run("zero ULID marshals to null", func(t *testing.T) {
		var zero ULID
		data, err := json.Marshal(zero)
		require.NoError(t, err)
		assert.Equal(t, "null", string(data))
	})

	t.Run("non-zero ULID marshals to quoted string", func(t *testing.T) {
		id := NewULID()
		data, err := json.Marshal(id)
		require.NoError(t, err)
		assert.Equal(t, `"`+id.String()+`"`, string(data))
	})
}

func TestULID_UnmarshalJSON(t *testing.T) {
	t.Run("null unmarshals to zero", func(t *testing.T) {
		var u ULID
		err := json.Unmarshal([]byte("null"), &u)
		require.NoError(t, err)
		assert.True(t, u.IsZero())
	})

	t.Run("empty quoted string unmarshals to zero", func(t *testing.T) {
		var u ULID
		err := json.Unmarshal([]byte(`""`), &u)
		require.NoError(t, err)
		assert.True(t, u.IsZero())
	})

	t.Run("valid ULID string unmarshals correctly", func(t *testing.T) {
		id := NewULID()
		data, err := json.Marshal(id)
		require.NoError(t, err)

		var parsed ULID
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)
		assert.Equal(t, id, parsed)
	})

	t.Run("invalid JSON format errors", func(t *testing.T) {
		var u ULID
		err := json.Unmarshal([]byte("12345"), &u)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid ULID JSON")
	})

	t.Run("invalid ULID in valid JSON errors", func(t *testing.T) {
		var u ULID
		err := json.Unmarshal([]byte(`"not-a-ulid"`), &u)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "parsing ULID JSON")
	})
}

func TestULID_JSON_Roundtrip(t *testing.T) {
	type wrapper struct {
		ID ULID `json:"id"`
	}

	t.Run("non-zero roundtrip", func(t *testing.T) {
		original := wrapper{ID: NewULID()}
		data, err := json.Marshal(original)
		require.NoError(t, err)

		var decoded wrapper
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.Equal(t, original.ID, decoded.ID)
	})

	t.Run("zero roundtrip", func(t *testing.T) {
		original := wrapper{}
		data, err := json.Marshal(original)
		require.NoError(t, err)

		var decoded wrapper
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)
		assert.True(t, decoded.ID.IsZero())
	})
}

func TestULID_GormDataType(t *testing.T) {
	var u ULID
	assert.Equal(t, "varchar(26)", u.GormDataType())
}

func TestBaseModel_BeforeCreate(t *testing.T) {
	t.Run("generates ID when zero", func(t *testing.T) {
		m := &BaseModel{}
		assert.True(t, m.ID.IsZero())

		err := m.BeforeCreate(nil)
		require.NoError(t, err)
		assert.False(t, m.ID.IsZero(), "BeforeCreate should set a non-zero ID")
	})

	t.Run("preserves existing ID", func(t *testing.T) {
		existing := NewULID()
		m := &BaseModel{ID: existing}

		err := m.BeforeCreate(nil)
		require.NoError(t, err)
		assert.Equal(t, existing, m.ID, "BeforeCreate should not overwrite existing ID")
	})
}

func TestBaseModel_GetID(t *testing.T) {
	id := NewULID()
	m := &BaseModel{ID: id}
	assert.Equal(t, id, m.GetID())
}
