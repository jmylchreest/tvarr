package models

import (
	"errors"
	"testing"
	"time"
)

func TestEpgProgram_Validate(t *testing.T) {
	now := time.Now()
	oneHourLater := now.Add(time.Hour)
	sourceID := NewULID()

	tests := []struct {
		name    string
		program *EpgProgram
		wantErr error
	}{
		{
			name:    "missing source ID",
			program: &EpgProgram{ChannelID: "ch1", Start: now, Stop: oneHourLater, Title: "Test"},
			wantErr: ErrSourceIDRequired,
		},
		{
			name:    "missing channel ID",
			program: &EpgProgram{SourceID: sourceID, Start: now, Stop: oneHourLater, Title: "Test"},
			wantErr: ErrChannelIDRequired,
		},
		{
			name:    "missing start time",
			program: &EpgProgram{SourceID: sourceID, ChannelID: "ch1", Stop: oneHourLater, Title: "Test"},
			wantErr: ErrStartTimeRequired,
		},
		{
			name:    "missing end time",
			program: &EpgProgram{SourceID: sourceID, ChannelID: "ch1", Start: now, Title: "Test"},
			wantErr: ErrEndTimeRequired,
		},
		{
			name:    "missing title",
			program: &EpgProgram{SourceID: sourceID, ChannelID: "ch1", Start: now, Stop: oneHourLater},
			wantErr: ErrTitleRequired,
		},
		{
			name:    "end before start",
			program: &EpgProgram{SourceID: sourceID, ChannelID: "ch1", Start: oneHourLater, Stop: now, Title: "Test"},
			wantErr: ErrInvalidTimeRange,
		},
		{
			name:    "valid program",
			program: &EpgProgram{SourceID: sourceID, ChannelID: "ch1", Start: now, Stop: oneHourLater, Title: "Test Program"},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.program.Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error %v, got nil", tt.wantErr)
				} else if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
			}
		})
	}
}

func TestEpgProgram_Duration(t *testing.T) {
	start := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	stop := time.Date(2024, 1, 1, 11, 30, 0, 0, time.UTC)

	program := &EpgProgram{
		Start: start,
		Stop:  stop,
	}

	expected := 90 * time.Minute
	if program.Duration() != expected {
		t.Errorf("expected duration %v, got %v", expected, program.Duration())
	}
}

func TestEpgProgram_IsOnAir(t *testing.T) {
	now := time.Now()

	// Program currently on air
	onAir := &EpgProgram{
		Start: now.Add(-30 * time.Minute),
		Stop:  now.Add(30 * time.Minute),
	}
	if !onAir.IsOnAir() {
		t.Error("expected IsOnAir to return true for current program")
	}

	// Program not started yet
	future := &EpgProgram{
		Start: now.Add(time.Hour),
		Stop:  now.Add(2 * time.Hour),
	}
	if future.IsOnAir() {
		t.Error("expected IsOnAir to return false for future program")
	}

	// Program ended
	past := &EpgProgram{
		Start: now.Add(-2 * time.Hour),
		Stop:  now.Add(-time.Hour),
	}
	if past.IsOnAir() {
		t.Error("expected IsOnAir to return false for past program")
	}
}

func TestEpgProgram_HasEnded(t *testing.T) {
	now := time.Now()

	// Program ended
	past := &EpgProgram{
		Stop: now.Add(-time.Hour),
	}
	if !past.HasEnded() {
		t.Error("expected HasEnded to return true for past program")
	}

	// Program not ended
	future := &EpgProgram{
		Stop: now.Add(time.Hour),
	}
	if future.HasEnded() {
		t.Error("expected HasEnded to return false for future program")
	}
}

func TestEpgProgram_TableName(t *testing.T) {
	program := &EpgProgram{}
	if program.TableName() != "epg_programs" {
		t.Errorf("expected table name 'epg_programs', got %s", program.TableName())
	}
}

func TestEpgProgram_GetID(t *testing.T) {
	id := NewULID()
	program := &EpgProgram{BaseModel: BaseModel{ID: id}}
	if program.GetID() != id {
		t.Errorf("expected ID %s, got %s", id, program.GetID())
	}
}

func TestEpgProgram_GetSourceID(t *testing.T) {
	sourceID := NewULID()
	program := &EpgProgram{SourceID: sourceID}
	if program.GetSourceID() != sourceID {
		t.Errorf("expected SourceID %s, got %s", sourceID, program.GetSourceID())
	}
}
