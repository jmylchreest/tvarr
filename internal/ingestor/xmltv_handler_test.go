package ingestor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewXMLTVHandler(t *testing.T) {
	handler := NewXMLTVHandler()
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.fetcher)
}

func TestXMLTVHandler_Type(t *testing.T) {
	handler := NewXMLTVHandler()
	assert.Equal(t, models.EpgSourceTypeXMLTV, handler.Type())
}

func TestXMLTVHandler_Validate(t *testing.T) {
	handler := NewXMLTVHandler()

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
				Type: models.EpgSourceTypeXtream,
				URL:  "http://example.com/epg.xml",
			},
			wantErr: "invalid source type",
		},
		{
			name: "empty URL",
			source: &models.EpgSource{
				Type: models.EpgSourceTypeXMLTV,
				URL:  "",
			},
			wantErr: "URL is required",
		},
		{
			name: "invalid URL scheme",
			source: &models.EpgSource{
				Type: models.EpgSourceTypeXMLTV,
				URL:  "ftp://example.com/epg.xml",
			},
			wantErr: "URL must be HTTP, HTTPS, or file://",
		},
		{
			name: "valid HTTP URL",
			source: &models.EpgSource{
				Type: models.EpgSourceTypeXMLTV,
				URL:  "http://example.com/epg.xml",
			},
			wantErr: "",
		},
		{
			name: "valid HTTPS URL",
			source: &models.EpgSource{
				Type: models.EpgSourceTypeXMLTV,
				URL:  "https://example.com/epg.xml",
			},
			wantErr: "",
		},
		{
			name: "valid file URL",
			source: &models.EpgSource{
				Type: models.EpgSourceTypeXMLTV,
				URL:  "file:///path/to/epg.xml",
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

func TestXMLTVHandler_Ingest(t *testing.T) {
	xmltvData := `<?xml version="1.0" encoding="UTF-8"?>
<tv generator-info-name="test">
  <channel id="channel1">
    <display-name>Channel One</display-name>
  </channel>
  <channel id="channel2">
    <display-name>Channel Two</display-name>
  </channel>
  <programme start="20240115180000 +0000" stop="20240115190000 +0000" channel="channel1">
    <title>Test Show</title>
    <sub-title>Episode 1</sub-title>
    <desc>A test description</desc>
    <category>Drama</category>
    <episode-num system="xmltv_ns">0.0.0</episode-num>
    <icon src="http://example.com/icon.png"/>
    <credits>
      <director>John Director</director>
      <actor>Jane Actress</actor>
    </credits>
    <rating>
      <value>PG-13</value>
    </rating>
  </programme>
  <programme start="20240115190000 +0000" stop="20240115200000 +0000" channel="channel2">
    <title>Another Show</title>
    <desc>Another description</desc>
  </programme>
</tv>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(xmltvData))
	}))
	defer server.Close()

	handler := NewXMLTVHandler()
	sourceID := models.NewULID()
	source := &models.EpgSource{
		BaseModel: models.BaseModel{ID: sourceID},
		Type:      models.EpgSourceTypeXMLTV,
		URL:       server.URL + "/epg.xml",
	}

	var programs []*models.EpgProgram
	err := handler.Ingest(context.Background(), source, func(program *models.EpgProgram) error {
		programs = append(programs, program)
		return nil
	})

	require.NoError(t, err)
	require.Len(t, programs, 2)

	// Check first program
	p1 := programs[0]
	assert.Equal(t, sourceID, p1.SourceID)
	assert.Equal(t, "channel1", p1.ChannelID)
	assert.Equal(t, "Test Show", p1.Title)
	assert.Equal(t, "Episode 1", p1.SubTitle)
	assert.Equal(t, "A test description", p1.Description)
	assert.Equal(t, "Drama", p1.Category)
	assert.Equal(t, "0.0.0", p1.EpisodeNum)
	assert.Equal(t, "http://example.com/icon.png", p1.Icon)
	assert.Equal(t, "PG-13", p1.Rating)

	// Check credits JSON
	require.NotEmpty(t, p1.Credits)
	var credits map[string][]string
	err = json.Unmarshal([]byte(p1.Credits), &credits)
	require.NoError(t, err)
	assert.Equal(t, []string{"John Director"}, credits["directors"])
	assert.Equal(t, []string{"Jane Actress"}, credits["actors"])

	// Check times
	expectedStart := time.Date(2024, 1, 15, 18, 0, 0, 0, time.UTC)
	expectedStop := time.Date(2024, 1, 15, 19, 0, 0, 0, time.UTC)
	assert.True(t, p1.Start.Equal(expectedStart), "start time mismatch: got %v, want %v", p1.Start, expectedStart)
	assert.True(t, p1.Stop.Equal(expectedStop), "end time mismatch: got %v, want %v", p1.Stop, expectedStop)

	// Check second program
	p2 := programs[1]
	assert.Equal(t, "channel2", p2.ChannelID)
	assert.Equal(t, "Another Show", p2.Title)
	assert.Equal(t, "Another description", p2.Description)
}

func TestXMLTVHandler_Ingest_CallbackError(t *testing.T) {
	xmltvData := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <programme start="20240115180000 +0000" stop="20240115190000 +0000" channel="ch1">
    <title>Test</title>
  </programme>
</tv>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(xmltvData))
	}))
	defer server.Close()

	handler := NewXMLTVHandler()
	sourceID := models.NewULID()
	source := &models.EpgSource{
		BaseModel: models.BaseModel{ID: sourceID},
		Type:      models.EpgSourceTypeXMLTV,
		URL:       server.URL,
	}

	err := handler.Ingest(context.Background(), source, func(program *models.EpgProgram) error {
		return assert.AnError
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse XMLTV")
}

func TestXMLTVHandler_Ingest_ContextCancellation(t *testing.T) {
	xmltvData := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <programme start="20240115180000 +0000" stop="20240115190000 +0000" channel="ch1">
    <title>Test 1</title>
  </programme>
  <programme start="20240115190000 +0000" stop="20240115200000 +0000" channel="ch1">
    <title>Test 2</title>
  </programme>
</tv>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(xmltvData))
	}))
	defer server.Close()

	handler := NewXMLTVHandler()
	sourceID := models.NewULID()
	source := &models.EpgSource{
		BaseModel: models.BaseModel{ID: sourceID},
		Type:      models.EpgSourceTypeXMLTV,
		URL:       server.URL,
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

func TestXMLTVHandler_Ingest_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	handler := NewXMLTVHandler()
	source := &models.EpgSource{
		Type: models.EpgSourceTypeXMLTV,
		URL:  server.URL,
	}

	err := handler.Ingest(context.Background(), source, func(program *models.EpgProgram) error {
		return nil
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status code: 404")
}

func TestXMLTVHandler_Ingest_InvalidURL(t *testing.T) {
	handler := NewXMLTVHandler()
	source := &models.EpgSource{
		Type: models.EpgSourceTypeXMLTV,
		URL:  "http://invalid.localhost.test:99999/epg.xml",
	}

	err := handler.Ingest(context.Background(), source, func(program *models.EpgProgram) error {
		return nil
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to fetch XMLTV")
}

func TestXMLTVHandler_Ingest_ValidationFailure(t *testing.T) {
	handler := NewXMLTVHandler()
	source := &models.EpgSource{
		Type: models.EpgSourceTypeXtream, // Wrong type
		URL:  "http://example.com/epg.xml",
	}

	err := handler.Ingest(context.Background(), source, func(program *models.EpgProgram) error {
		return nil
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestXMLTVHandler_Ingest_SkipsInvalidTimeRanges(t *testing.T) {
	xmltvData := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <programme start="20240115180000 +0000" stop="20240115190000 +0000" channel="ch1">
    <title>Valid Show 1</title>
  </programme>
  <programme start="20240115200000 +0000" stop="20240115190000 +0000" channel="ch1">
    <title>Invalid: End Before Start</title>
  </programme>
  <programme start="20240115190000 +0000" stop="20240115190000 +0000" channel="ch1">
    <title>Invalid: Equal Times</title>
  </programme>
  <programme start="20240115200000 +0000" stop="20240115210000 +0000" channel="ch1">
    <title>Valid Show 2</title>
  </programme>
</tv>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(xmltvData))
	}))
	defer server.Close()

	handler := NewXMLTVHandler()
	sourceID := models.NewULID()
	source := &models.EpgSource{
		BaseModel: models.BaseModel{ID: sourceID},
		Type:      models.EpgSourceTypeXMLTV,
		URL:       server.URL,
	}

	var programs []*models.EpgProgram
	err := handler.Ingest(context.Background(), source, func(program *models.EpgProgram) error {
		programs = append(programs, program)
		return nil
	})

	require.NoError(t, err)
	// Should only have 2 valid programs, invalid ones should be skipped
	require.Len(t, programs, 2)
	assert.Equal(t, "Valid Show 1", programs[0].Title)
	assert.Equal(t, "Valid Show 2", programs[1].Title)
}

func TestXMLTVHandler_ImplementsInterface(t *testing.T) {
	var _ EpgHandler = (*XMLTVHandler)(nil)
}
