package httpclient

import (
	"fmt"
	"strconv"
	"strings"
)

// StatusCodeRange represents a range of HTTP status codes (inclusive).
type StatusCodeRange struct {
	Min int
	Max int
}

// Contains returns true if the code falls within this range.
func (r StatusCodeRange) Contains(code int) bool {
	return code >= r.Min && code <= r.Max
}

// StatusCodeSet represents a set of acceptable HTTP status codes.
// It supports both individual codes and ranges for efficient storage.
//
// Example formats:
//   - "200" - single code
//   - "200,404" - multiple codes
//   - "200-299" - range (inclusive)
//   - "200-299,404,500-599" - mixed ranges and codes
type StatusCodeSet struct {
	codes  map[int]struct{}  // Individual codes for O(1) lookup
	ranges []StatusCodeRange // Ranges for efficient storage
}

// NewStatusCodeSet creates an empty StatusCodeSet.
func NewStatusCodeSet() *StatusCodeSet {
	return &StatusCodeSet{
		codes:  make(map[int]struct{}),
		ranges: nil,
	}
}

// ParseStatusCodes parses a string like "200-299,404,500-599" into a StatusCodeSet.
// Returns nil if the input is empty.
func ParseStatusCodes(s string) (*StatusCodeSet, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}

	set := NewStatusCodeSet()

	parts := strings.Split(s, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if strings.Contains(part, "-") {
			// Parse range like "200-299"
			rangeParts := strings.SplitN(part, "-", 2)
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("invalid range format: %q", part)
			}

			min, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid range start %q: %w", rangeParts[0], err)
			}

			max, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid range end %q: %w", rangeParts[1], err)
			}

			if min > max {
				return nil, fmt.Errorf("invalid range %d-%d: min > max", min, max)
			}

			if min < 100 || max > 599 {
				return nil, fmt.Errorf("invalid HTTP status code range %d-%d: must be 100-599", min, max)
			}

			set.ranges = append(set.ranges, StatusCodeRange{Min: min, Max: max})
		} else {
			// Parse individual code
			code, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid status code %q: %w", part, err)
			}

			if code < 100 || code > 599 {
				return nil, fmt.Errorf("invalid HTTP status code %d: must be 100-599", code)
			}

			set.codes[code] = struct{}{}
		}
	}

	if len(set.codes) == 0 && len(set.ranges) == 0 {
		return nil, nil
	}

	return set, nil
}

// MustParseStatusCodes is like ParseStatusCodes but panics on error.
// Use only for compile-time constants.
func MustParseStatusCodes(s string) *StatusCodeSet {
	set, err := ParseStatusCodes(s)
	if err != nil {
		panic(err)
	}
	return set
}

// StatusCodesFromSlice creates a StatusCodeSet from a slice of individual codes.
// This is useful for programmatic construction.
func StatusCodesFromSlice(codes []int) *StatusCodeSet {
	if len(codes) == 0 {
		return nil
	}

	set := NewStatusCodeSet()
	for _, code := range codes {
		set.codes[code] = struct{}{}
	}
	return set
}

// Add adds an individual status code to the set.
func (s *StatusCodeSet) Add(code int) {
	if s.codes == nil {
		s.codes = make(map[int]struct{})
	}
	s.codes[code] = struct{}{}
}

// AddRange adds a range of status codes to the set.
func (s *StatusCodeSet) AddRange(min, max int) {
	s.ranges = append(s.ranges, StatusCodeRange{Min: min, Max: max})
}

// Contains returns true if the status code is in the set.
func (s *StatusCodeSet) Contains(code int) bool {
	if s == nil {
		return false
	}

	// Check individual codes first (O(1))
	if _, ok := s.codes[code]; ok {
		return true
	}

	// Check ranges
	for _, r := range s.ranges {
		if r.Contains(code) {
			return true
		}
	}

	return false
}

// IsEmpty returns true if the set has no codes or ranges.
func (s *StatusCodeSet) IsEmpty() bool {
	if s == nil {
		return true
	}
	return len(s.codes) == 0 && len(s.ranges) == 0
}

// String returns a string representation of the set.
func (s *StatusCodeSet) String() string {
	if s == nil || s.IsEmpty() {
		return ""
	}

	var parts []string

	// Add ranges
	for _, r := range s.ranges {
		if r.Min == r.Max {
			parts = append(parts, strconv.Itoa(r.Min))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", r.Min, r.Max))
		}
	}

	// Add individual codes
	for code := range s.codes {
		parts = append(parts, strconv.Itoa(code))
	}

	return strings.Join(parts, ",")
}

// Default2xxStatusCodes returns a StatusCodeSet containing all 2xx status codes.
func Default2xxStatusCodes() *StatusCodeSet {
	set := NewStatusCodeSet()
	set.AddRange(200, 299)
	return set
}

// Clone returns a deep copy of the StatusCodeSet.
func (s *StatusCodeSet) Clone() *StatusCodeSet {
	if s == nil {
		return nil
	}

	clone := NewStatusCodeSet()

	// Copy individual codes
	for code := range s.codes {
		clone.codes[code] = struct{}{}
	}

	// Copy ranges
	if len(s.ranges) > 0 {
		clone.ranges = make([]StatusCodeRange, len(s.ranges))
		copy(clone.ranges, s.ranges)
	}

	return clone
}
