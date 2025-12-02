package httpclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStatusCodes(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantErr     bool
		errContains string
		checkCodes  []int // codes that should be in the set
		checkNot    []int // codes that should NOT be in the set
	}{
		{
			name:       "empty string returns nil",
			input:      "",
			wantErr:    false,
			checkCodes: nil,
		},
		{
			name:       "whitespace only returns nil",
			input:      "   ",
			wantErr:    false,
			checkCodes: nil,
		},
		{
			name:       "single code",
			input:      "200",
			checkCodes: []int{200},
			checkNot:   []int{199, 201, 404},
		},
		{
			name:       "multiple codes",
			input:      "200,404,500",
			checkCodes: []int{200, 404, 500},
			checkNot:   []int{201, 403, 501},
		},
		{
			name:       "single range",
			input:      "200-299",
			checkCodes: []int{200, 250, 299},
			checkNot:   []int{199, 300, 404},
		},
		{
			name:       "mixed ranges and codes",
			input:      "200-299,404,500-599",
			checkCodes: []int{200, 250, 299, 404, 500, 550, 599},
			checkNot:   []int{199, 300, 403, 405, 499},
		},
		{
			name:       "with whitespace",
			input:      " 200 , 404 , 500-599 ",
			checkCodes: []int{200, 404, 500, 599},
			checkNot:   []int{201, 403, 499},
		},
		{
			name:       "range with same min and max",
			input:      "200-200",
			checkCodes: []int{200},
			checkNot:   []int{199, 201},
		},
		{
			name:        "invalid code format",
			input:       "abc",
			wantErr:     true,
			errContains: "invalid status code",
		},
		{
			name:        "invalid range format - missing end",
			input:       "200-",
			wantErr:     true,
			errContains: "invalid range end",
		},
		{
			name:        "invalid range format - missing start",
			input:       "-299",
			wantErr:     true,
			errContains: "invalid range start",
		},
		{
			name:        "invalid range - min > max",
			input:       "299-200",
			wantErr:     true,
			errContains: "min > max",
		},
		{
			name:        "code too low",
			input:       "99",
			wantErr:     true,
			errContains: "must be 100-599",
		},
		{
			name:        "code too high",
			input:       "600",
			wantErr:     true,
			errContains: "must be 100-599",
		},
		{
			name:        "range too low",
			input:       "50-150",
			wantErr:     true,
			errContains: "must be 100-599",
		},
		{
			name:        "range too high",
			input:       "500-700",
			wantErr:     true,
			errContains: "must be 100-599",
		},
		{
			name:       "boundary codes",
			input:      "100,599",
			checkCodes: []int{100, 599},
			checkNot:   []int{99, 600},
		},
		{
			name:       "full valid range",
			input:      "100-599",
			checkCodes: []int{100, 200, 300, 400, 500, 599},
			checkNot:   []int{99, 600},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set, err := ParseStatusCodes(tt.input)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)

			if tt.checkCodes == nil {
				assert.Nil(t, set)
				return
			}

			require.NotNil(t, set)

			for _, code := range tt.checkCodes {
				assert.True(t, set.Contains(code), "expected set to contain %d", code)
			}

			for _, code := range tt.checkNot {
				assert.False(t, set.Contains(code), "expected set NOT to contain %d", code)
			}
		})
	}
}

func TestStatusCodeSet_Contains_NilSet(t *testing.T) {
	var set *StatusCodeSet
	assert.False(t, set.Contains(200))
}

func TestStatusCodeSet_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		set      *StatusCodeSet
		expected bool
	}{
		{
			name:     "nil set",
			set:      nil,
			expected: true,
		},
		{
			name:     "empty set",
			set:      NewStatusCodeSet(),
			expected: true,
		},
		{
			name: "set with code",
			set: func() *StatusCodeSet {
				s := NewStatusCodeSet()
				s.Add(200)
				return s
			}(),
			expected: false,
		},
		{
			name: "set with range",
			set: func() *StatusCodeSet {
				s := NewStatusCodeSet()
				s.AddRange(200, 299)
				return s
			}(),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.set.IsEmpty())
		})
	}
}

func TestStatusCodesFromSlice(t *testing.T) {
	tests := []struct {
		name       string
		codes      []int
		checkCodes []int
		checkNot   []int
		expectNil  bool
	}{
		{
			name:      "nil slice",
			codes:     nil,
			expectNil: true,
		},
		{
			name:      "empty slice",
			codes:     []int{},
			expectNil: true,
		},
		{
			name:       "single code",
			codes:      []int{200},
			checkCodes: []int{200},
			checkNot:   []int{201},
		},
		{
			name:       "multiple codes",
			codes:      []int{200, 404, 500},
			checkCodes: []int{200, 404, 500},
			checkNot:   []int{201, 403, 501},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set := StatusCodesFromSlice(tt.codes)

			if tt.expectNil {
				assert.Nil(t, set)
				return
			}

			require.NotNil(t, set)

			for _, code := range tt.checkCodes {
				assert.True(t, set.Contains(code), "expected set to contain %d", code)
			}

			for _, code := range tt.checkNot {
				assert.False(t, set.Contains(code), "expected set NOT to contain %d", code)
			}
		})
	}
}

func TestMustParseStatusCodes(t *testing.T) {
	t.Run("valid input", func(t *testing.T) {
		set := MustParseStatusCodes("200-299,404")
		require.NotNil(t, set)
		assert.True(t, set.Contains(200))
		assert.True(t, set.Contains(404))
	})

	t.Run("panics on invalid input", func(t *testing.T) {
		assert.Panics(t, func() {
			MustParseStatusCodes("invalid")
		})
	})
}

func TestStatusCodeSet_String(t *testing.T) {
	tests := []struct {
		name     string
		set      *StatusCodeSet
		contains []string // parts that should be in the output
	}{
		{
			name:     "nil set",
			set:      nil,
			contains: nil,
		},
		{
			name:     "empty set",
			set:      NewStatusCodeSet(),
			contains: nil,
		},
		{
			name: "single code",
			set: func() *StatusCodeSet {
				s := NewStatusCodeSet()
				s.Add(200)
				return s
			}(),
			contains: []string{"200"},
		},
		{
			name: "range",
			set: func() *StatusCodeSet {
				s := NewStatusCodeSet()
				s.AddRange(200, 299)
				return s
			}(),
			contains: []string{"200-299"},
		},
		{
			name: "single-value range",
			set: func() *StatusCodeSet {
				s := NewStatusCodeSet()
				s.AddRange(200, 200)
				return s
			}(),
			contains: []string{"200"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.set.String()

			if tt.contains == nil {
				assert.Empty(t, result)
				return
			}

			for _, part := range tt.contains {
				assert.Contains(t, result, part)
			}
		})
	}
}

func TestDefault2xxStatusCodes(t *testing.T) {
	set := Default2xxStatusCodes()
	require.NotNil(t, set)

	// Should contain all 2xx codes
	for code := 200; code <= 299; code++ {
		assert.True(t, set.Contains(code), "expected set to contain %d", code)
	}

	// Should not contain non-2xx codes
	assert.False(t, set.Contains(199))
	assert.False(t, set.Contains(300))
	assert.False(t, set.Contains(404))
	assert.False(t, set.Contains(500))
}
