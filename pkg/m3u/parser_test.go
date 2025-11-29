package m3u

import (
	"bytes"
	"compress/gzip"
	"errors"
	"strings"
	"testing"

	"github.com/dsnet/compress/bzip2"
	"github.com/ulikunitz/xz"
)

func TestParser_BasicParsing(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 tvg-id="channel1" tvg-name="Channel One" tvg-logo="http://example.com/logo.png" group-title="News",Channel 1 HD
http://example.com/stream1.m3u8
#EXTINF:-1 tvg-id="channel2" tvg-name="Channel Two" group-title="Sports",Channel 2
http://example.com/stream2.m3u8
`

	var entries []*Entry
	p := &Parser{
		OnEntry: func(entry *Entry) error {
			entries = append(entries, entry)
			return nil
		},
	}

	err := p.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Verify first entry
	e1 := entries[0]
	if e1.TvgID != "channel1" {
		t.Errorf("expected tvg-id 'channel1', got '%s'", e1.TvgID)
	}
	if e1.TvgName != "Channel One" {
		t.Errorf("expected tvg-name 'Channel One', got '%s'", e1.TvgName)
	}
	if e1.TvgLogo != "http://example.com/logo.png" {
		t.Errorf("expected tvg-logo 'http://example.com/logo.png', got '%s'", e1.TvgLogo)
	}
	if e1.GroupTitle != "News" {
		t.Errorf("expected group-title 'News', got '%s'", e1.GroupTitle)
	}
	if e1.Title != "Channel 1 HD" {
		t.Errorf("expected title 'Channel 1 HD', got '%s'", e1.Title)
	}
	if e1.URL != "http://example.com/stream1.m3u8" {
		t.Errorf("expected URL 'http://example.com/stream1.m3u8', got '%s'", e1.URL)
	}
	if e1.Duration != -1 {
		t.Errorf("expected duration -1, got %d", e1.Duration)
	}

	// Verify second entry
	e2 := entries[1]
	if e2.TvgID != "channel2" {
		t.Errorf("expected tvg-id 'channel2', got '%s'", e2.TvgID)
	}
	if e2.GroupTitle != "Sports" {
		t.Errorf("expected group-title 'Sports', got '%s'", e2.GroupTitle)
	}
}

func TestParser_ChannelNumber(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 tvg-id="ch1" tvg-chno="42",Channel with Number
http://example.com/stream.m3u8
`

	var entries []*Entry
	p := &Parser{
		OnEntry: func(entry *Entry) error {
			entries = append(entries, entry)
			return nil
		},
	}

	err := p.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].ChannelNumber != 42 {
		t.Errorf("expected channel number 42, got %d", entries[0].ChannelNumber)
	}
}

func TestParser_ExtraAttributes(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 tvg-id="ch1" custom-attr="custom-value" another="test",Channel
http://example.com/stream.m3u8
`

	var entries []*Entry
	p := &Parser{
		OnEntry: func(entry *Entry) error {
			entries = append(entries, entry)
			return nil
		},
	}

	err := p.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.Extra["custom-attr"] != "custom-value" {
		t.Errorf("expected custom-attr 'custom-value', got '%s'", e.Extra["custom-attr"])
	}
	if e.Extra["another"] != "test" {
		t.Errorf("expected another 'test', got '%s'", e.Extra["another"])
	}
}

func TestParser_PositiveDuration(t *testing.T) {
	content := `#EXTM3U
#EXTINF:180 tvg-id="song1",Song Title
http://example.com/song.mp3
`

	var entries []*Entry
	p := &Parser{
		OnEntry: func(entry *Entry) error {
			entries = append(entries, entry)
			return nil
		},
	}

	err := p.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].Duration != 180 {
		t.Errorf("expected duration 180, got %d", entries[0].Duration)
	}
}

func TestParser_URLWithoutExtinf(t *testing.T) {
	content := `#EXTM3U
http://example.com/stream.m3u8
`

	var entries []*Entry
	p := &Parser{
		OnEntry: func(entry *Entry) error {
			entries = append(entries, entry)
			return nil
		},
	}

	err := p.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].URL != "http://example.com/stream.m3u8" {
		t.Errorf("expected URL, got '%s'", entries[0].URL)
	}
	if entries[0].Duration != -1 {
		t.Errorf("expected duration -1, got %d", entries[0].Duration)
	}
	if entries[0].Title != "stream" {
		t.Errorf("expected title 'stream', got '%s'", entries[0].Title)
	}
}

func TestParser_CommasInQuotes(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 tvg-id="ch1" tvg-name="Channel, with comma" group-title="News, Sports",Title with Comma Inside
http://example.com/stream.m3u8
`

	var entries []*Entry
	p := &Parser{
		OnEntry: func(entry *Entry) error {
			entries = append(entries, entry)
			return nil
		},
	}

	err := p.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.TvgName != "Channel, with comma" {
		t.Errorf("expected tvg-name 'Channel, with comma', got '%s'", e.TvgName)
	}
	if e.GroupTitle != "News, Sports" {
		t.Errorf("expected group-title 'News, Sports', got '%s'", e.GroupTitle)
	}
	// Title is everything after the last unquoted comma
	if e.Title != "Title with Comma Inside" {
		t.Errorf("expected title 'Title with Comma Inside', got '%s'", e.Title)
	}
}

func TestParser_EmptyLines(t *testing.T) {
	content := `#EXTM3U

#EXTINF:-1 tvg-id="ch1",Channel 1
http://example.com/stream1.m3u8

#EXTINF:-1 tvg-id="ch2",Channel 2

http://example.com/stream2.m3u8
`

	var entries []*Entry
	p := &Parser{
		OnEntry: func(entry *Entry) error {
			entries = append(entries, entry)
			return nil
		},
	}

	err := p.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestParser_SkipsOtherComments(t *testing.T) {
	content := `#EXTM3U
#EXTVLCOPT:network-caching=1000
#EXTINF:-1 tvg-id="ch1",Channel 1
http://example.com/stream.m3u8
#Some other comment
`

	var entries []*Entry
	p := &Parser{
		OnEntry: func(entry *Entry) error {
			entries = append(entries, entry)
			return nil
		},
	}

	err := p.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestParser_CallbackError(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 tvg-id="ch1",Channel 1
http://example.com/stream.m3u8
`

	expectedErr := errors.New("callback failed")
	p := &Parser{
		OnEntry: func(entry *Entry) error {
			return expectedErr
		},
	}

	err := p.Parse(strings.NewReader(content))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "callback error") {
		t.Errorf("expected callback error, got: %v", err)
	}
}

func TestParser_NilOnEntry(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1,Channel
http://example.com/stream.m3u8
`

	p := &Parser{}
	err := p.Parse(strings.NewReader(content))
	if err == nil {
		t.Fatal("expected error for nil OnEntry")
	}
	if !strings.Contains(err.Error(), "OnEntry callback is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParser_InvalidExtinfFormat(t *testing.T) {
	content := `#EXTM3U
#EXTINF:invalid format
http://example.com/stream1.m3u8
#EXTINF:-1,Valid Channel
http://example.com/stream2.m3u8
`

	var entries []*Entry
	var parseErrors []string
	p := &Parser{
		OnEntry: func(entry *Entry) error {
			entries = append(entries, entry)
			return nil
		},
		OnError: func(lineNum int, err error) {
			parseErrors = append(parseErrors, err.Error())
		},
	}

	err := p.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should parse both entries - invalid EXTINF is skipped but URL still creates minimal entry
	// because we're in EXTM3U mode. Total: 2 entries (one minimal from orphan URL, one valid)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Should have recorded the parse error for invalid EXTINF
	if len(parseErrors) != 1 {
		t.Fatalf("expected 1 parse error, got %d", len(parseErrors))
	}

	// Second entry should be the valid one
	if entries[1].Title != "Valid Channel" {
		t.Errorf("expected second entry title 'Valid Channel', got '%s'", entries[1].Title)
	}
}

func TestParser_LargeFile(t *testing.T) {
	// Build a large M3U content
	var builder strings.Builder
	builder.WriteString("#EXTM3U\n")

	numChannels := 10000
	for i := 0; i < numChannels; i++ {
		builder.WriteString("#EXTINF:-1 tvg-id=\"ch")
		builder.WriteString(strings.Repeat("x", 100)) // Long ID
		builder.WriteString("\" tvg-name=\"Channel with a very long name that goes on and on\",Title\n")
		builder.WriteString("http://example.com/stream/path/that/is/also/quite/long/stream.m3u8\n")
	}

	content := builder.String()
	count := 0
	p := &Parser{
		OnEntry: func(entry *Entry) error {
			count++
			return nil
		},
	}

	err := p.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if count != numChannels {
		t.Errorf("expected %d entries, got %d", numChannels, count)
	}
}

func TestParser_ParseString(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 tvg-id="ch1",Channel 1
http://example.com/stream.m3u8
`

	var entries []*Entry
	err := ParseString(content, func(entry *Entry) error {
		entries = append(entries, entry)
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestParser_ParseAll(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 tvg-id="ch1",Channel 1
http://example.com/stream1.m3u8
#EXTINF:-1 tvg-id="ch2",Channel 2
http://example.com/stream2.m3u8
`

	entries, err := ParseAll(strings.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestParser_ParseCompressed_Gzip(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 tvg-id="ch1",Channel 1
http://example.com/stream.m3u8
`

	// Compress with gzip
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, err := gw.Write([]byte(content))
	if err != nil {
		t.Fatalf("failed to write gzip: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("failed to close gzip: %v", err)
	}

	var entries []*Entry
	p := &Parser{
		OnEntry: func(entry *Entry) error {
			entries = append(entries, entry)
			return nil
		},
	}

	err = p.ParseCompressed(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].TvgID != "ch1" {
		t.Errorf("expected tvg-id 'ch1', got '%s'", entries[0].TvgID)
	}
}

func TestParser_ParseCompressed_Bzip2(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 tvg-id="ch1",Channel 1
http://example.com/stream.m3u8
`

	// Compress with bzip2
	var buf bytes.Buffer
	bw, err := bzip2.NewWriter(&buf, nil)
	if err != nil {
		t.Fatalf("failed to create bzip2 writer: %v", err)
	}
	_, err = bw.Write([]byte(content))
	if err != nil {
		t.Fatalf("failed to write bzip2: %v", err)
	}
	if err := bw.Close(); err != nil {
		t.Fatalf("failed to close bzip2: %v", err)
	}

	var entries []*Entry
	p := &Parser{
		OnEntry: func(entry *Entry) error {
			entries = append(entries, entry)
			return nil
		},
	}

	err = p.ParseCompressed(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].TvgID != "ch1" {
		t.Errorf("expected tvg-id 'ch1', got '%s'", entries[0].TvgID)
	}
}

func TestParser_ParseCompressed_XZ(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 tvg-id="ch1",Channel 1
http://example.com/stream.m3u8
`

	// Compress with xz
	var buf bytes.Buffer
	xw, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatalf("failed to create xz writer: %v", err)
	}
	_, err = xw.Write([]byte(content))
	if err != nil {
		t.Fatalf("failed to write xz: %v", err)
	}
	if err := xw.Close(); err != nil {
		t.Fatalf("failed to close xz: %v", err)
	}

	var entries []*Entry
	p := &Parser{
		OnEntry: func(entry *Entry) error {
			entries = append(entries, entry)
			return nil
		},
	}

	err = p.ParseCompressed(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].TvgID != "ch1" {
		t.Errorf("expected tvg-id 'ch1', got '%s'", entries[0].TvgID)
	}
}

func TestParser_ParseCompressed_Uncompressed(t *testing.T) {
	content := `#EXTM3U
#EXTINF:-1 tvg-id="ch1",Channel 1
http://example.com/stream.m3u8
`

	var entries []*Entry
	p := &Parser{
		OnEntry: func(entry *Entry) error {
			entries = append(entries, entry)
			return nil
		},
	}

	// ParseCompressed should handle uncompressed data too
	err := p.ParseCompressed(strings.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestExtractTitleFromURL(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"http://example.com/channel.m3u8", "channel"},
		{"http://example.com/path/to/stream.ts", "stream"},
		{"http://example.com/live?token=abc", "live"},
		{"http://example.com/", "Unknown"},
		{"http://example.com", "example"}, // Last path segment is the domain
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := extractTitleFromURL(tt.url)
			if result != tt.expected {
				t.Errorf("extractTitleFromURL(%s) = %s, want %s", tt.url, result, tt.expected)
			}
		})
	}
}

func TestFindTitleStart(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{`tvg-id="ch1",Title`, 12},
		{`tvg-name="Name, with comma",Title`, 27},
		{`no comma here`, -1},
		{`"quoted,comma",Title`, 14},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := findTitleStart(tt.input)
			if result != tt.expected {
				t.Errorf("findTitleStart(%s) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

// Benchmark tests for performance validation
func BenchmarkParser_Parse(b *testing.B) {
	// Build a sample M3U with 1000 channels
	var builder strings.Builder
	builder.WriteString("#EXTM3U\n")
	for i := 0; i < 1000; i++ {
		builder.WriteString("#EXTINF:-1 tvg-id=\"ch1\" tvg-name=\"Channel Name\" tvg-logo=\"http://logo.com/logo.png\" group-title=\"Category\",Channel Title\n")
		builder.WriteString("http://example.com/stream.m3u8\n")
	}
	content := builder.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := &Parser{
			OnEntry: func(entry *Entry) error {
				return nil
			},
		}
		_ = p.Parse(strings.NewReader(content))
	}
}

func BenchmarkParser_ParseCompressed_Gzip(b *testing.B) {
	// Build and compress content
	var builder strings.Builder
	builder.WriteString("#EXTM3U\n")
	for i := 0; i < 1000; i++ {
		builder.WriteString("#EXTINF:-1 tvg-id=\"ch1\" tvg-name=\"Channel Name\" group-title=\"Category\",Channel Title\n")
		builder.WriteString("http://example.com/stream.m3u8\n")
	}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write([]byte(builder.String()))
	_ = gw.Close()
	compressed := buf.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := &Parser{
			OnEntry: func(entry *Entry) error {
				return nil
			},
		}
		_ = p.ParseCompressed(bytes.NewReader(compressed))
	}
}
