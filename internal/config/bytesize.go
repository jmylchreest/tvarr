package config

import (
	"encoding/json"

	"github.com/jmylchreest/tvarr/pkg/bytesize"
)

// ByteSize is a size value that supports human-readable parsing.
// It extends standard integer sizes with support for units like KB, MB, GB.
//
// Examples:
//   - "5MB" = 5 * 1024 * 1024 bytes
//   - "1.5 GB" = 1.5 * 1024^3 bytes
//   - "500KB" = 500 * 1024 bytes
//   - "5242880" = 5242880 bytes (raw number still works)
//
// This type implements encoding.TextUnmarshaler for Viper/YAML support
// and json.Unmarshaler for JSON configuration files.
type ByteSize int64

// ParseByteSize parses a human-readable byte size string.
func ParseByteSize(s string) (ByteSize, error) {
	size, err := bytesize.Parse(s)
	if err != nil {
		return 0, err
	}
	return ByteSize(size), nil
}

// UnmarshalText implements encoding.TextUnmarshaler for YAML/Viper support.
func (b *ByteSize) UnmarshalText(text []byte) error {
	parsed, err := ParseByteSize(string(text))
	if err != nil {
		return err
	}
	*b = parsed
	return nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (b *ByteSize) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		// Try as a number (bytes) for backwards compatibility
		var bytes int64
		if err := json.Unmarshal(data, &bytes); err != nil {
			return err
		}
		*b = ByteSize(bytes)
		return nil
	}
	return b.UnmarshalText([]byte(s))
}

// MarshalJSON implements json.Marshaler.
// Outputs in the most human-readable format possible.
func (b ByteSize) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.String())
}

// MarshalText implements encoding.TextMarshaler.
func (b ByteSize) MarshalText() ([]byte, error) {
	return []byte(b.String()), nil
}

// Bytes returns the size in bytes as int64.
func (b ByteSize) Bytes() int64 {
	return int64(b)
}

// Int64 returns the size as int64 (alias for Bytes).
func (b ByteSize) Int64() int64 {
	return int64(b)
}

// String returns a human-readable string representation.
func (b ByteSize) String() string {
	return bytesize.Format(bytesize.Size(b))
}
