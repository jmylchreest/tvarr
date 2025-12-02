package models

import (
	"errors"
	"testing"
)

func TestEpgSource_Validate(t *testing.T) {
	tests := []struct {
		name    string
		source  *EpgSource
		wantErr error
	}{
		{
			name:    "empty name",
			source:  &EpgSource{Name: "", Type: EpgSourceTypeXMLTV, URL: "http://example.com"},
			wantErr: ErrNameRequired,
		},
		{
			name:    "empty URL",
			source:  &EpgSource{Name: "Test", Type: EpgSourceTypeXMLTV, URL: ""},
			wantErr: ErrURLRequired,
		},
		{
			name:    "invalid type",
			source:  &EpgSource{Name: "Test", Type: "invalid", URL: "http://example.com"},
			wantErr: ErrInvalidEpgSourceType,
		},
		{
			name:    "xtream without username",
			source:  &EpgSource{Name: "Test", Type: EpgSourceTypeXtream, URL: "http://example.com", Password: "pass"},
			wantErr: ErrXtreamCredentialsRequired,
		},
		{
			name:    "xtream without password",
			source:  &EpgSource{Name: "Test", Type: EpgSourceTypeXtream, URL: "http://example.com", Username: "user"},
			wantErr: ErrXtreamCredentialsRequired,
		},
		{
			name:    "valid xmltv source",
			source:  &EpgSource{Name: "Test", Type: EpgSourceTypeXMLTV, URL: "http://example.com/epg.xml"},
			wantErr: nil,
		},
		{
			name:    "valid xtream source",
			source:  &EpgSource{Name: "Test", Type: EpgSourceTypeXtream, URL: "http://example.com", Username: "user", Password: "pass"},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.source.Validate()
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

func TestEpgSource_IsXMLTV(t *testing.T) {
	xmltvSource := &EpgSource{Type: EpgSourceTypeXMLTV}
	xtreamSource := &EpgSource{Type: EpgSourceTypeXtream}

	if !xmltvSource.IsXMLTV() {
		t.Error("expected IsXMLTV to return true for XMLTV source")
	}
	if xmltvSource.IsXtream() {
		t.Error("expected IsXtream to return false for XMLTV source")
	}
	if xtreamSource.IsXMLTV() {
		t.Error("expected IsXMLTV to return false for Xtream source")
	}
	if !xtreamSource.IsXtream() {
		t.Error("expected IsXtream to return true for Xtream source")
	}
}

func TestEpgSource_MarkIngesting(t *testing.T) {
	source := &EpgSource{
		Status:    EpgSourceStatusPending,
		LastError: "previous error",
	}

	source.MarkIngesting()

	if source.Status != EpgSourceStatusIngesting {
		t.Errorf("expected status %s, got %s", EpgSourceStatusIngesting, source.Status)
	}
	if source.LastError != "" {
		t.Errorf("expected empty LastError, got %s", source.LastError)
	}
}

func TestEpgSource_MarkSuccess(t *testing.T) {
	source := &EpgSource{
		Status:    EpgSourceStatusIngesting,
		LastError: "previous error",
	}

	source.MarkSuccess(5000)

	if source.Status != EpgSourceStatusSuccess {
		t.Errorf("expected status %s, got %s", EpgSourceStatusSuccess, source.Status)
	}
	if source.ProgramCount != 5000 {
		t.Errorf("expected program count 5000, got %d", source.ProgramCount)
	}
	if source.LastIngestionAt == nil {
		t.Error("expected LastIngestionAt to be set")
	}
	if source.LastError != "" {
		t.Errorf("expected empty LastError, got %s", source.LastError)
	}
}

func TestEpgSource_MarkFailed(t *testing.T) {
	source := &EpgSource{
		Status: EpgSourceStatusIngesting,
	}

	testErr := errors.New("ingestion failed")
	source.MarkFailed(testErr)

	if source.Status != EpgSourceStatusFailed {
		t.Errorf("expected status %s, got %s", EpgSourceStatusFailed, source.Status)
	}
	if source.LastError != "ingestion failed" {
		t.Errorf("expected LastError 'ingestion failed', got %s", source.LastError)
	}
}

func TestEpgSource_MarkFailed_NilError(t *testing.T) {
	source := &EpgSource{
		Status: EpgSourceStatusIngesting,
	}

	source.MarkFailed(nil)

	if source.Status != EpgSourceStatusFailed {
		t.Errorf("expected status %s, got %s", EpgSourceStatusFailed, source.Status)
	}
	if source.LastError != "" {
		t.Errorf("expected empty LastError, got %s", source.LastError)
	}
}

func TestEpgSource_TableName(t *testing.T) {
	source := &EpgSource{}
	if source.TableName() != "epg_sources" {
		t.Errorf("expected table name 'epg_sources', got %s", source.TableName())
	}
}

func TestEpgSource_GetID(t *testing.T) {
	id := NewULID()
	source := &EpgSource{BaseModel: BaseModel{ID: id}}
	if source.GetID() != id {
		t.Errorf("expected ID %s, got %s", id, source.GetID())
	}
}
