// Package models defines GORM database models for tvarr entities.
package models

import (
	"crypto/rand"
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
)

// BoolPtr returns a pointer to a bool value.
// Useful for setting *bool fields in structs.
func BoolPtr(b bool) *bool {
	return &b
}

// BoolVal returns the value of a bool pointer, defaulting to true if nil.
// This matches GORM's default:true behavior for optional bool fields.
func BoolVal(b *bool) bool {
	return b == nil || *b
}

// BoolValDefault returns the value of a bool pointer with a custom default.
func BoolValDefault(b *bool, defaultVal bool) bool {
	if b == nil {
		return defaultVal
	}
	return *b
}

// ULID is a wrapper around ulid.ULID for database storage as primary key.
type ULID ulid.ULID

// NewULID generates a new ULID.
func NewULID() ULID {
	return ULID(ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader))
}

// ParseULID parses a ULID string.
func ParseULID(s string) (ULID, error) {
	id, err := ulid.Parse(s)
	if err != nil {
		return ULID{}, fmt.Errorf("invalid ULID: %w", err)
	}
	return ULID(id), nil
}

// MustParseULID parses a ULID string and panics on error.
func MustParseULID(s string) ULID {
	id, err := ParseULID(s)
	if err != nil {
		panic(err)
	}
	return id
}

// String returns the string representation of the ULID.
func (u ULID) String() string {
	return ulid.ULID(u).String()
}

// IsZero returns true if the ULID is zero/empty.
func (u ULID) IsZero() bool {
	return ulid.ULID(u).Compare(ulid.ULID{}) == 0
}

// Value implements driver.Valuer for database storage.
func (u ULID) Value() (driver.Value, error) {
	if u.IsZero() {
		return nil, nil
	}
	return ulid.ULID(u).String(), nil
}

// Scan implements sql.Scanner for database retrieval.
func (u *ULID) Scan(value any) error {
	if value == nil {
		*u = ULID{}
		return nil
	}

	switch v := value.(type) {
	case string:
		if v == "" {
			*u = ULID{}
			return nil
		}
		id, err := ulid.Parse(v)
		if err != nil {
			return fmt.Errorf("scanning ULID: %w", err)
		}
		*u = ULID(id)
	case []byte:
		if len(v) == 0 {
			*u = ULID{}
			return nil
		}
		id, err := ulid.Parse(string(v))
		if err != nil {
			return fmt.Errorf("scanning ULID: %w", err)
		}
		*u = ULID(id)
	default:
		return fmt.Errorf("unsupported type for ULID: %T", value)
	}
	return nil
}

// MarshalJSON implements json.Marshaler.
func (u ULID) MarshalJSON() ([]byte, error) {
	if u.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + u.String() + `"`), nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (u *ULID) UnmarshalJSON(data []byte) error {
	// Handle null
	if string(data) == "null" {
		*u = ULID{}
		return nil
	}
	// Remove quotes
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid ULID JSON: %s", string(data))
	}
	s := string(data[1 : len(data)-1])
	if s == "" {
		*u = ULID{}
		return nil
	}
	id, err := ulid.Parse(s)
	if err != nil {
		return fmt.Errorf("parsing ULID JSON: %w", err)
	}
	*u = ULID(id)
	return nil
}

// GormDataType returns the GORM data type for ULID.
func (ULID) GormDataType() string {
	return "varchar(26)"
}

// BaseModel provides common fields for all models with ULID as primary key.
type BaseModel struct {
	ID        ULID           `gorm:"primarykey;type:varchar(26)" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

// BeforeCreate generates a ULID if not already set.
func (b *BaseModel) BeforeCreate(tx *gorm.DB) error {
	if b.ID.IsZero() {
		b.ID = NewULID()
	}
	return nil
}

// GetID returns the ULID identifier.
func (b *BaseModel) GetID() ULID {
	return b.ID
}

// Time is an alias for time.Time used in models.
type Time = time.Time

// Now returns the current time.
func Now() Time {
	return time.Now()
}
