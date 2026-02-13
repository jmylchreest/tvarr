package m3u

import (
	"fmt"
	"io"
	"strings"
)

// Writer provides streaming M3U playlist writing.
type Writer struct {
	w             io.Writer
	headerWritten bool
}

// NewWriter creates a new M3U writer.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// WriteHeader writes the M3U header.
// This is automatically called by WriteEntry if not already written.
func (w *Writer) WriteHeader() error {
	if w.headerWritten {
		return nil
	}
	_, err := fmt.Fprintln(w.w, "#EXTM3U")
	if err != nil {
		return fmt.Errorf("writing M3U header: %w", err)
	}
	w.headerWritten = true
	return nil
}

// WriteEntry writes a single channel entry to the M3U playlist.
func (w *Writer) WriteEntry(entry *Entry) error {
	if err := w.WriteHeader(); err != nil {
		return err
	}

	// Build EXTINF line with attributes
	var attrs []string

	if entry.TvgID != "" {
		attrs = append(attrs, fmt.Sprintf(`tvg-id="%s"`, escapeQuotes(entry.TvgID)))
	}
	if entry.TvgName != "" {
		attrs = append(attrs, fmt.Sprintf(`tvg-name="%s"`, escapeQuotes(entry.TvgName)))
	}
	if entry.TvgLogo != "" {
		attrs = append(attrs, fmt.Sprintf(`tvg-logo="%s"`, escapeQuotes(entry.TvgLogo)))
	}
	if entry.GroupTitle != "" {
		attrs = append(attrs, fmt.Sprintf(`group-title="%s"`, escapeQuotes(entry.GroupTitle)))
	}
	if entry.ChannelNumber > 0 {
		attrs = append(attrs, fmt.Sprintf(`tvg-chno="%d"`, entry.ChannelNumber))
	}

	// Add any extra attributes
	for k, v := range entry.Extra {
		attrs = append(attrs, fmt.Sprintf(`%s="%s"`, k, escapeQuotes(v)))
	}

	// Build the EXTINF line
	duration := entry.Duration
	if duration == 0 {
		duration = -1 // Default to -1 for live streams
	}

	var extinf string
	if len(attrs) > 0 {
		extinf = fmt.Sprintf("#EXTINF:%d %s,%s", duration, strings.Join(attrs, " "), entry.Title)
	} else {
		extinf = fmt.Sprintf("#EXTINF:%d,%s", duration, entry.Title)
	}

	// Write EXTINF line
	if _, err := fmt.Fprintln(w.w, extinf); err != nil {
		return fmt.Errorf("writing EXTINF: %w", err)
	}

	// Write URL line
	if _, err := fmt.Fprintln(w.w, entry.URL); err != nil {
		return fmt.Errorf("writing URL: %w", err)
	}

	return nil
}

// escapeQuotes escapes double quotes in attribute values.
func escapeQuotes(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}
