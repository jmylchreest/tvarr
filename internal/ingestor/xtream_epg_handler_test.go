package ingestor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/pkg/xtream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewXtreamEpgHandler(t *testing.T) {
	handler := NewXtreamEpgHandler()
	assert.NotNil(t, handler)
	assert.Equal(t, defaultDaysToFetch, handler.DaysToFetch)
}

func TestXtreamEpgHandler_Type(t *testing.T) {
	handler := NewXtreamEpgHandler()
	assert.Equal(t, models.EpgSourceTypeXtream, handler.Type())
}

func TestXtreamEpgHandler_Validate(t *testing.T) {
	handler := NewXtreamEpgHandler()

	tests := []struct {
		name    string
		source  *models.EpgSource
		wantErr string
	}{
		{
			name:    "nil source",
			source:  nil,
			wantErr: "source is nil",
		},
		{
			name: "wrong type",
			source: &models.EpgSource{
				Type:     models.EpgSourceTypeXMLTV,
				URL:      "http://example.com",
				Username: "user",
				Password: "pass",
			},
			wantErr: "invalid source type",
		},
		{
			name: "empty URL",
			source: &models.EpgSource{
				Type:     models.EpgSourceTypeXtream,
				URL:      "",
				Username: "user",
				Password: "pass",
			},
			wantErr: "URL is required",
		},
		{
			name: "invalid URL scheme",
			source: &models.EpgSource{
				Type:     models.EpgSourceTypeXtream,
				URL:      "ftp://example.com",
				Username: "user",
				Password: "pass",
			},
			wantErr: "URL must be an HTTP(S) URL",
		},
		{
			name: "missing username",
			source: &models.EpgSource{
				Type:     models.EpgSourceTypeXtream,
				URL:      "http://example.com",
				Username: "",
				Password: "pass",
			},
			wantErr: "username is required",
		},
		{
			name: "missing password",
			source: &models.EpgSource{
				Type:     models.EpgSourceTypeXtream,
				URL:      "http://example.com",
				Username: "user",
				Password: "",
			},
			wantErr: "password is required",
		},
		{
			name: "valid source",
			source: &models.EpgSource{
				Type:     models.EpgSourceTypeXtream,
				URL:      "http://example.com",
				Username: "user",
				Password: "pass",
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.Validate(tt.source)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestXtreamEpgHandler_Ingest(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")

		switch action {
		case "get_live_streams":
			streams := []xtream.Stream{
				{
					StreamID:     xtream.FlexInt(1),
					Name:         "Channel 1",
					EPGChannelID: "ch1.epg",
				},
				{
					StreamID:     xtream.FlexInt(2),
					Name:         "Channel 2",
					EPGChannelID: "ch2.epg",
				},
				{
					StreamID:     xtream.FlexInt(3),
					Name:         "Channel No EPG",
					EPGChannelID: "", // No EPG
				},
			}
			json.NewEncoder(w).Encode(streams)

		case "get_simple_data_table":
			streamID := r.URL.Query().Get("stream_id")
			var response struct {
				EPGListings []xtream.EPGListing `json:"epg_listings"`
			}

			if streamID == "1" {
				response.EPGListings = []xtream.EPGListing{
					{
						ID:             xtream.FlexString("1"),
						Title:          "Morning Show",
						Description:    "Start your day right",
						Lang:           "en",
						StartTimestamp: xtream.FlexInt(1705330800), // 2024-01-15 15:00:00 UTC
						StopTimestamp:  xtream.FlexInt(1705338000), // 2024-01-15 17:00:00 UTC
					},
					{
						ID:             xtream.FlexString("2"),
						Title:          "Evening News",
						Description:    "Daily news update",
						Lang:           "en",
						StartTimestamp: xtream.FlexInt(1705338000),
						StopTimestamp:  xtream.FlexInt(1705341600),
					},
				}
			} else if streamID == "2" {
				response.EPGListings = []xtream.EPGListing{
					{
						ID:             xtream.FlexString("3"),
						Title:          "Movie Time",
						Description:    "Feature film",
						Lang:           "es",
						Start:          "2024-01-15 18:00:00",
						End:            "2024-01-15 20:00:00",
						StartTimestamp: xtream.FlexInt(1705341600),
						StopTimestamp:  xtream.FlexInt(1705348800),
					},
				}
			}

			json.NewEncoder(w).Encode(response)

		default:
			http.Error(w, "Unknown action", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	handler := NewXtreamEpgHandler()
	sourceID := models.NewULID()
	source := &models.EpgSource{
		BaseModel: models.BaseModel{ID: sourceID},
		Type:      models.EpgSourceTypeXtream,
		URL:       server.URL,
		Username:  "testuser",
		Password:  "testpass",
	}

	var programs []*models.EpgProgram
	err := handler.Ingest(context.Background(), source, func(program *models.EpgProgram) error {
		programs = append(programs, program)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, programs, 3) // 2 from channel 1, 1 from channel 2

	// Check first program
	p1 := programs[0]
	assert.Equal(t, sourceID, p1.SourceID)
	assert.Equal(t, "ch1.epg", p1.ChannelID)
	assert.Equal(t, "Morning Show", p1.Title)
	assert.Equal(t, "Start your day right", p1.Description)
	assert.Equal(t, "en", p1.Language)

	// Check timestamp parsing
	expectedStart := time.Unix(1705330800, 0).UTC()
	assert.True(t, p1.Start.Equal(expectedStart), "start time mismatch: got %v, want %v", p1.Start, expectedStart)

	// Check third program (from channel 2 with datetime format)
	p3 := programs[2]
	assert.Equal(t, "ch2.epg", p3.ChannelID)
	assert.Equal(t, "Movie Time", p3.Title)
	assert.Equal(t, "es", p3.Language)
}

func TestXtreamEpgHandler_Ingest_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		if action == "get_live_streams" {
			streams := []xtream.Stream{
				{StreamID: xtream.FlexInt(1), EPGChannelID: "ch1"},
				{StreamID: xtream.FlexInt(2), EPGChannelID: "ch2"},
			}
			json.NewEncoder(w).Encode(streams)
		} else {
			response := struct {
				EPGListings []xtream.EPGListing `json:"epg_listings"`
			}{
				EPGListings: []xtream.EPGListing{
					{ID: xtream.FlexString("1"), Title: "Show", StartTimestamp: xtream.FlexInt(1705330800), StopTimestamp: xtream.FlexInt(1705338000)},
				},
			}
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer server.Close()

	handler := NewXtreamEpgHandler()
	sourceID := models.NewULID()
	source := &models.EpgSource{
		BaseModel: models.BaseModel{ID: sourceID},
		Type:      models.EpgSourceTypeXtream,
		URL:       server.URL,
		Username:  "user",
		Password:  "pass",
	}

	ctx, cancel := context.WithCancel(context.Background())
	count := 0
	err := handler.Ingest(ctx, source, func(program *models.EpgProgram) error {
		count++
		cancel() // Cancel after first program
		return nil
	})

	require.Error(t, err)
	assert.Equal(t, 1, count)
}

func TestXtreamEpgHandler_Ingest_CallbackError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		if action == "get_live_streams" {
			streams := []xtream.Stream{{StreamID: xtream.FlexInt(1), EPGChannelID: "ch1"}}
			json.NewEncoder(w).Encode(streams)
		} else {
			response := struct {
				EPGListings []xtream.EPGListing `json:"epg_listings"`
			}{
				EPGListings: []xtream.EPGListing{
					{ID: xtream.FlexString("1"), Title: "Show", StartTimestamp: xtream.FlexInt(1705330800), StopTimestamp: xtream.FlexInt(1705338000)},
				},
			}
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer server.Close()

	handler := NewXtreamEpgHandler()
	sourceID := models.NewULID()
	source := &models.EpgSource{
		BaseModel: models.BaseModel{ID: sourceID},
		Type:      models.EpgSourceTypeXtream,
		URL:       server.URL,
		Username:  "user",
		Password:  "pass",
	}

	err := handler.Ingest(context.Background(), source, func(program *models.EpgProgram) error {
		return assert.AnError
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "callback error")
}

func TestXtreamEpgHandler_Ingest_FetchError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Error", http.StatusInternalServerError)
	}))
	defer server.Close()

	handler := NewXtreamEpgHandler()
	source := &models.EpgSource{
		Type:     models.EpgSourceTypeXtream,
		URL:      server.URL,
		Username: "user",
		Password: "pass",
	}

	err := handler.Ingest(context.Background(), source, func(program *models.EpgProgram) error {
		return nil
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch live streams")
}

func TestXtreamEpgHandler_Ingest_SkipsInvalidTimeRanges(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		if action == "get_live_streams" {
			streams := []xtream.Stream{{StreamID: xtream.FlexInt(1), EPGChannelID: "ch1"}}
			json.NewEncoder(w).Encode(streams)
		} else {
			response := struct {
				EPGListings []xtream.EPGListing `json:"epg_listings"`
			}{
				EPGListings: []xtream.EPGListing{
					// Valid program
					{ID: xtream.FlexString("1"), Title: "Valid Show", StartTimestamp: xtream.FlexInt(1705330800), StopTimestamp: xtream.FlexInt(1705338000)},
					// Invalid: end time before start time
					{ID: xtream.FlexString("2"), Title: "Invalid Show", StartTimestamp: xtream.FlexInt(1705338000), StopTimestamp: xtream.FlexInt(1705330800)},
					// Invalid: end time equals start time
					{ID: xtream.FlexString("3"), Title: "Equal Times", StartTimestamp: xtream.FlexInt(1705340000), StopTimestamp: xtream.FlexInt(1705340000)},
					// Invalid: zero timestamps
					{ID: xtream.FlexString("4"), Title: "Zero Times", StartTimestamp: xtream.FlexInt(0), StopTimestamp: xtream.FlexInt(0)},
					// Another valid program
					{ID: xtream.FlexString("5"), Title: "Another Valid", StartTimestamp: xtream.FlexInt(1705341000), StopTimestamp: xtream.FlexInt(1705344000)},
				},
			}
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer server.Close()

	handler := NewXtreamEpgHandler()
	sourceID := models.NewULID()
	source := &models.EpgSource{
		BaseModel: models.BaseModel{ID: sourceID},
		Type:      models.EpgSourceTypeXtream,
		URL:       server.URL,
		Username:  "user",
		Password:  "pass",
	}

	var programs []*models.EpgProgram
	err := handler.Ingest(context.Background(), source, func(program *models.EpgProgram) error {
		programs = append(programs, program)
		return nil
	})

	require.NoError(t, err)
	// Should only have 2 valid programs, invalid ones should be skipped
	require.Len(t, programs, 2)
	assert.Equal(t, "Valid Show", programs[0].Title)
	assert.Equal(t, "Another Valid", programs[1].Title)
}

func TestXtreamEpgHandler_Ingest_SkipsChannelsWithoutEpgID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		if action == "get_live_streams" {
			streams := []xtream.Stream{
				{StreamID: xtream.FlexInt(1), EPGChannelID: ""}, // No EPG ID
				{StreamID: xtream.FlexInt(2), EPGChannelID: ""}, // No EPG ID
			}
			json.NewEncoder(w).Encode(streams)
		}
	}))
	defer server.Close()

	handler := NewXtreamEpgHandler()
	source := &models.EpgSource{
		Type:     models.EpgSourceTypeXtream,
		URL:      server.URL,
		Username: "user",
		Password: "pass",
	}

	var programs []*models.EpgProgram
	err := handler.Ingest(context.Background(), source, func(program *models.EpgProgram) error {
		programs = append(programs, program)
		return nil
	})

	require.NoError(t, err)
	assert.Len(t, programs, 0) // No programs because no channels have EPG IDs
}

func TestXtreamEpgHandler_Ingest_ValidationFailure(t *testing.T) {
	handler := NewXtreamEpgHandler()
	source := &models.EpgSource{
		Type: models.EpgSourceTypeXMLTV, // Wrong type
		URL:  "http://example.com",
	}

	err := handler.Ingest(context.Background(), source, func(program *models.EpgProgram) error {
		return nil
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestXtreamEpgHandler_ImplementsInterface(t *testing.T) {
	var _ EpgHandler = (*XtreamEpgHandler)(nil)
}

// T071: Unit test for Xtream API timezone handling in internal/ingestor/xtream_epg_handler_test.go.
// Tests the applyTimeOffset function which handles EpgShift adjustments.
func TestXtreamEpgHandler_TimezoneHandling(t *testing.T) {
	handler := NewXtreamEpgHandler()

	t.Run("applies positive EpgShift", func(t *testing.T) {
		source := &models.EpgSource{
			Type:     models.EpgSourceTypeXtream,
			URL:      "http://example.com",
			Username: "user",
			Password: "pass",
			EpgShift: 2, // +2 hours
		}

		inputTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
		result := handler.applyTimeOffset(inputTime, source)

		// Should add 2 hours
		expected := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
		assert.True(t, result.Equal(expected), "Expected %v, got %v", expected, result)
	})

	t.Run("applies negative EpgShift", func(t *testing.T) {
		source := &models.EpgSource{
			Type:     models.EpgSourceTypeXtream,
			URL:      "http://example.com",
			Username: "user",
			Password: "pass",
			EpgShift: -5, // -5 hours
		}

		inputTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
		result := handler.applyTimeOffset(inputTime, source)

		// Should subtract 5 hours
		expected := time.Date(2024, 1, 15, 5, 0, 0, 0, time.UTC)
		assert.True(t, result.Equal(expected), "Expected %v, got %v", expected, result)
	})

	t.Run("no shift returns UTC time", func(t *testing.T) {
		source := &models.EpgSource{
			Type:     models.EpgSourceTypeXtream,
			URL:      "http://example.com",
			Username: "user",
			Password: "pass",
			EpgShift: 0,
		}

		inputTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
		result := handler.applyTimeOffset(inputTime, source)

		expected := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
		assert.True(t, result.Equal(expected), "Expected %v, got %v", expected, result)
	})

	t.Run("nil source returns original time", func(t *testing.T) {
		inputTime := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
		result := handler.applyTimeOffset(inputTime, nil)

		assert.True(t, result.Equal(inputTime), "Expected %v, got %v", inputTime, result)
	})
}

// TestXtreamEpgHandler_Ingest_WithEpgShift tests that EpgShift is applied during ingestion.
func TestXtreamEpgHandler_Ingest_WithEpgShift(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")

		switch action {
		case "":
			// Auth info request - return server info with timezone
			response := xtream.AuthInfo{
				ServerInfo: xtream.ServerInfo{
					Timezone: "UTC",
				},
			}
			json.NewEncoder(w).Encode(response)

		case "get_live_streams":
			streams := []xtream.Stream{
				{
					StreamID:     xtream.FlexInt(1),
					Name:         "Channel 1",
					EPGChannelID: "ch1.epg",
				},
			}
			json.NewEncoder(w).Encode(streams)

		case "get_simple_data_table":
			response := struct {
				EPGListings []xtream.EPGListing `json:"epg_listings"`
			}{
				EPGListings: []xtream.EPGListing{
					{
						ID:             xtream.FlexString("1"),
						Title:          "Morning Show",
						Description:    "Start your day right",
						Lang:           "en",
						StartTimestamp: xtream.FlexInt(1705330800), // 2024-01-15 15:00:00 UTC
						StopTimestamp:  xtream.FlexInt(1705338000), // 2024-01-15 17:00:00 UTC
					},
				},
			}
			json.NewEncoder(w).Encode(response)

		default:
			http.Error(w, "Unknown action", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	handler := NewXtreamEpgHandler()
	sourceID := models.NewULID()
	source := &models.EpgSource{
		BaseModel:         models.BaseModel{ID: sourceID},
		Type:              models.EpgSourceTypeXtream,
		URL:               server.URL,
		Username:          "testuser",
		Password:          "testpass",
		EpgShift:          2,     // Apply +2 hour shift
		AutoShiftTimezone: "UTC", // Set to match server to prevent auto-shift from overwriting
	}

	var programs []*models.EpgProgram
	err := handler.Ingest(context.Background(), source, func(program *models.EpgProgram) error {
		programs = append(programs, program)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, programs, 1)

	// Original time was 15:00 UTC, with +2 hour shift should be 17:00 UTC
	expectedStart := time.Unix(1705330800, 0).UTC().Add(2 * time.Hour)
	expectedStop := time.Unix(1705338000, 0).UTC().Add(2 * time.Hour)
	assert.True(t, programs[0].Start.Equal(expectedStart), "start time mismatch: got %v, want %v", programs[0].Start, expectedStart)
	assert.True(t, programs[0].Stop.Equal(expectedStop), "stop time mismatch: got %v, want %v", programs[0].Stop, expectedStop)

	// Check that timezone was detected from server info
	assert.Equal(t, "UTC", source.DetectedTimezone, "detected timezone should be UTC")
}
