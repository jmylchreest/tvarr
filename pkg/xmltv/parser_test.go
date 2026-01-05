package xmltv

import (
	"bytes"
	"compress/gzip"
	"errors"
	"strings"
	"testing"
	"time"
)

const sampleXMLTV = `<?xml version="1.0" encoding="UTF-8"?>
<tv generator-info-name="test">
  <channel id="channel1.tv">
    <display-name>Channel One</display-name>
    <icon src="http://example.com/logo1.png"/>
    <url>http://example.com/channel1</url>
  </channel>
  <channel id="channel2.tv">
    <display-name>Channel Two</display-name>
  </channel>
  <programme start="20240115180000 +0000" stop="20240115190000 +0000" channel="channel1.tv">
    <title>News at Six</title>
    <sub-title>Evening Edition</sub-title>
    <desc>The latest news and weather.</desc>
    <category>News</category>
    <icon src="http://example.com/news.png"/>
    <episode-num system="onscreen">S01E05</episode-num>
    <rating>
      <value>TV-PG</value>
    </rating>
    <language>en</language>
    <new/>
    <credits>
      <presenter>John Smith</presenter>
      <presenter>Jane Doe</presenter>
    </credits>
  </programme>
  <programme start="20240115190000 +0000" stop="20240115200000 +0000" channel="channel1.tv">
    <title>Evening Drama</title>
    <desc>A dramatic story unfolds.</desc>
    <category>Drama</category>
    <premiere/>
  </programme>
</tv>`

func TestParser_ParseChannels(t *testing.T) {
	var channels []*Channel
	p := &Parser{
		OnChannel: func(ch *Channel) error {
			channels = append(channels, ch)
			return nil
		},
	}

	err := p.Parse(strings.NewReader(sampleXMLTV))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(channels))
	}

	// Verify first channel
	ch1 := channels[0]
	if ch1.ID != "channel1.tv" {
		t.Errorf("expected ID 'channel1.tv', got %q", ch1.ID)
	}
	if ch1.DisplayName != "Channel One" {
		t.Errorf("expected DisplayName 'Channel One', got %q", ch1.DisplayName)
	}
	if ch1.Icon != "http://example.com/logo1.png" {
		t.Errorf("expected Icon URL, got %q", ch1.Icon)
	}
	if ch1.URL != "http://example.com/channel1" {
		t.Errorf("expected URL, got %q", ch1.URL)
	}

	// Verify second channel
	ch2 := channels[1]
	if ch2.ID != "channel2.tv" {
		t.Errorf("expected ID 'channel2.tv', got %q", ch2.ID)
	}
}

func TestParser_ParseProgrammes(t *testing.T) {
	var programmes []*Programme
	p := &Parser{
		OnProgramme: func(prog *Programme) error {
			programmes = append(programmes, prog)
			return nil
		},
	}

	err := p.Parse(strings.NewReader(sampleXMLTV))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(programmes) != 2 {
		t.Fatalf("expected 2 programmes, got %d", len(programmes))
	}

	// Verify first programme
	prog1 := programmes[0]
	if prog1.Channel != "channel1.tv" {
		t.Errorf("expected channel 'channel1.tv', got %q", prog1.Channel)
	}
	if prog1.Title != "News at Six" {
		t.Errorf("expected title 'News at Six', got %q", prog1.Title)
	}
	if prog1.SubTitle != "Evening Edition" {
		t.Errorf("expected subtitle 'Evening Edition', got %q", prog1.SubTitle)
	}
	if prog1.Description != "The latest news and weather." {
		t.Errorf("expected description, got %q", prog1.Description)
	}
	if prog1.Category != "News" {
		t.Errorf("expected category 'News', got %q", prog1.Category)
	}
	if prog1.Icon != "http://example.com/news.png" {
		t.Errorf("expected icon, got %q", prog1.Icon)
	}
	if prog1.EpisodeNum != "S01E05" {
		t.Errorf("expected episode num 'S01E05', got %q", prog1.EpisodeNum)
	}
	if prog1.Rating != "TV-PG" {
		t.Errorf("expected rating 'TV-PG', got %q", prog1.Rating)
	}
	if prog1.Language != "en" {
		t.Errorf("expected language 'en', got %q", prog1.Language)
	}
	if !prog1.IsNew {
		t.Error("expected IsNew to be true")
	}
	if prog1.IsPremiere {
		t.Error("expected IsPremiere to be false")
	}

	// Check credits
	if prog1.Credits == nil {
		t.Fatal("expected credits to be set")
	}
	if len(prog1.Credits.Presenters) != 2 {
		t.Errorf("expected 2 presenters, got %d", len(prog1.Credits.Presenters))
	}

	// Verify times
	expectedStart := time.Date(2024, 1, 15, 18, 0, 0, 0, time.UTC)
	if !prog1.Start.Equal(expectedStart) {
		t.Errorf("expected start time %v, got %v", expectedStart, prog1.Start)
	}
	expectedStop := time.Date(2024, 1, 15, 19, 0, 0, 0, time.UTC)
	if !prog1.Stop.Equal(expectedStop) {
		t.Errorf("expected stop time %v, got %v", expectedStop, prog1.Stop)
	}

	// Verify second programme
	prog2 := programmes[1]
	if prog2.Title != "Evening Drama" {
		t.Errorf("expected title 'Evening Drama', got %q", prog2.Title)
	}
	if prog2.IsNew {
		t.Error("expected IsNew to be false")
	}
	if !prog2.IsPremiere {
		t.Error("expected IsPremiere to be true")
	}
}

func TestParser_CallbackError(t *testing.T) {
	expectedErr := errors.New("callback failed")
	p := &Parser{
		OnProgramme: func(prog *Programme) error {
			return expectedErr
		},
	}

	err := p.Parse(strings.NewReader(sampleXMLTV))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "callback") {
		t.Errorf("expected callback error, got: %v", err)
	}
}

func TestParser_ChannelCallbackError(t *testing.T) {
	expectedErr := errors.New("channel callback failed")
	p := &Parser{
		OnChannel: func(ch *Channel) error {
			return expectedErr
		},
	}

	err := p.Parse(strings.NewReader(sampleXMLTV))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestParser_ParseCompressed_Gzip(t *testing.T) {
	// Compress the sample
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write([]byte(sampleXMLTV))
	_ = gw.Close()

	var programmes []*Programme
	p := &Parser{
		OnProgramme: func(prog *Programme) error {
			programmes = append(programmes, prog)
			return nil
		},
	}

	err := p.ParseCompressed(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(programmes) != 2 {
		t.Errorf("expected 2 programmes, got %d", len(programmes))
	}
}

func TestParser_ParseCompressed_Uncompressed(t *testing.T) {
	var programmes []*Programme
	p := &Parser{
		OnProgramme: func(prog *Programme) error {
			programmes = append(programmes, prog)
			return nil
		},
	}

	err := p.ParseCompressed(strings.NewReader(sampleXMLTV))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(programmes) != 2 {
		t.Errorf("expected 2 programmes, got %d", len(programmes))
	}
}

func TestParseXMLTVTime(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
		wantErr  bool
	}{
		{
			input:    "20240115180000 +0000",
			expected: time.Date(2024, 1, 15, 18, 0, 0, 0, time.UTC),
			wantErr:  false,
		},
		{
			input:    "20240115180000 -0500",
			expected: time.Date(2024, 1, 15, 18, 0, 0, 0, time.FixedZone("", -5*3600)),
			wantErr:  false,
		},
		{
			input:    "20240115180000",
			expected: time.Date(2024, 1, 15, 18, 0, 0, 0, time.UTC),
			wantErr:  false,
		},
		{
			input:   "",
			wantErr: true,
		},
		{
			input:   "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := parseXMLTVTime(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if !result.Time.Equal(tt.expected) {
					t.Errorf("expected %v, got %v", tt.expected, result.Time)
				}
			}
		})
	}
}

func TestParseAll(t *testing.T) {
	programmes, err := ParseAll(strings.NewReader(sampleXMLTV))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(programmes) != 2 {
		t.Errorf("expected 2 programmes, got %d", len(programmes))
	}
}

func TestParseString(t *testing.T) {
	count := 0
	err := ParseString(sampleXMLTV, func(prog *Programme) error {
		count++
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 programmes, got %d", count)
	}
}

func TestParser_LargeFile(t *testing.T) {
	// Build a larger XMLTV file
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="UTF-8"?><tv>`)

	numProgrammes := 10000
	for range numProgrammes {
		builder.WriteString(`<programme start="20240115180000 +0000" stop="20240115190000 +0000" channel="ch1">`)
		builder.WriteString(`<title>Programme Title</title>`)
		builder.WriteString(`<desc>Programme description goes here.</desc>`)
		builder.WriteString(`</programme>`)
	}
	builder.WriteString(`</tv>`)

	count := 0
	p := &Parser{
		OnProgramme: func(prog *Programme) error {
			count++
			return nil
		},
	}

	err := p.Parse(strings.NewReader(builder.String()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if count != numProgrammes {
		t.Errorf("expected %d programmes, got %d", numProgrammes, count)
	}
}

func TestParser_Credits(t *testing.T) {
	xmltv := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <programme start="20240115180000 +0000" stop="20240115190000 +0000" channel="ch1">
    <title>Movie</title>
    <credits>
      <director>Director One</director>
      <director>Director Two</director>
      <actor>Actor One</actor>
      <actor>Actor Two</actor>
      <actor>Actor Three</actor>
      <writer>Writer One</writer>
      <producer>Producer One</producer>
      <presenter>Host One</presenter>
    </credits>
  </programme>
</tv>`

	var prog *Programme
	p := &Parser{
		OnProgramme: func(pr *Programme) error {
			prog = pr
			return nil
		},
	}

	err := p.Parse(strings.NewReader(xmltv))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if prog == nil {
		t.Fatal("expected programme")
	}

	if prog.Credits == nil {
		t.Fatal("expected credits")
	}

	if len(prog.Credits.Directors) != 2 {
		t.Errorf("expected 2 directors, got %d", len(prog.Credits.Directors))
	}
	if len(prog.Credits.Actors) != 3 {
		t.Errorf("expected 3 actors, got %d", len(prog.Credits.Actors))
	}
	if len(prog.Credits.Writers) != 1 {
		t.Errorf("expected 1 writer, got %d", len(prog.Credits.Writers))
	}
	if len(prog.Credits.Producers) != 1 {
		t.Errorf("expected 1 producer, got %d", len(prog.Credits.Producers))
	}
	if len(prog.Credits.Presenters) != 1 {
		t.Errorf("expected 1 presenter, got %d", len(prog.Credits.Presenters))
	}
}

func BenchmarkParser_Parse(b *testing.B) {
	// Build sample content
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="UTF-8"?><tv>`)
	for range 1000 {
		builder.WriteString(`<programme start="20240115180000 +0000" stop="20240115190000 +0000" channel="ch1">`)
		builder.WriteString(`<title>Programme Title</title><desc>Description</desc><category>Category</category>`)
		builder.WriteString(`</programme>`)
	}
	builder.WriteString(`</tv>`)
	content := builder.String()

	for b.Loop() {
		p := &Parser{
			OnProgramme: func(prog *Programme) error {
				return nil
			},
		}
		_ = p.Parse(strings.NewReader(content))
	}
}
