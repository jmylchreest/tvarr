// Package m3u provides streaming M3U playlist parsing and writing.
// It supports standard M3U and extended M3U (M3U8) formats with EXTINF metadata.
package m3u

import (
	"bufio"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/ulikunitz/xz"
)

// Entry represents a single channel entry in an M3U playlist.
type Entry struct {
	// Duration is the track duration in seconds (-1 for live streams).
	Duration int

	// TvgID is the EPG channel identifier.
	TvgID string

	// TvgName is the display name from tvg-name attribute.
	TvgName string

	// TvgLogo is the URL to the channel logo.
	TvgLogo string

	// GroupTitle is the category/group from group-title attribute.
	GroupTitle string

	// ChannelNumber is the channel number from tvg-chno attribute.
	ChannelNumber int

	// Title is the display title from EXTINF line.
	Title string

	// URL is the stream URL.
	URL string

	// Extra contains any additional attributes not explicitly parsed.
	Extra map[string]string
}

// Parser provides streaming M3U parsing with callback-based processing.
type Parser struct {
	// OnEntry is called for each parsed entry.
	OnEntry func(entry *Entry) error

	// OnError is called for recoverable parsing errors.
	// If nil, errors are silently ignored.
	OnError func(lineNum int, err error)
}

// Regular expressions for parsing EXTINF attributes.
var (
	// Matches duration and attributes portion: #EXTINF:-1 tvg-id="..." tvg-name="...",Title
	extinfRegex = regexp.MustCompile(`^#EXTINF:\s*(-?\d+)\s*(.*)$`)

	// Matches key="value" or key=value patterns
	attrRegex = regexp.MustCompile(`([a-zA-Z0-9_-]+)=(?:"([^"]*)"|([^\s,]+))`)
)

// Parse parses an M3U playlist from a reader, calling OnEntry for each channel.
// The reader can provide plain text, gzip, bzip2, or xz compressed data.
func (p *Parser) Parse(r io.Reader) error {
	if p.OnEntry == nil {
		return fmt.Errorf("OnEntry callback is required")
	}

	scanner := bufio.NewScanner(r)
	// Increase buffer size for long lines (some M3U files have very long URLs)
	const maxLineSize = 1024 * 1024 // 1MB
	buf := make([]byte, maxLineSize)
	scanner.Buffer(buf, maxLineSize)

	var currentEntry *Entry
	lineNum := 0
	isExtM3U := false

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines
		if line == "" {
			continue
		}

		// Check for M3U header
		if strings.HasPrefix(line, "#EXTM3U") {
			isExtM3U = true
			continue
		}

		// Parse EXTINF line
		if strings.HasPrefix(line, "#EXTINF:") {
			entry, err := p.parseExtinf(line)
			if err != nil {
				p.handleError(lineNum, err)
				continue
			}
			currentEntry = entry
			continue
		}

		// Skip other comment lines
		if strings.HasPrefix(line, "#") {
			continue
		}

		// This should be a URL line
		if currentEntry != nil {
			currentEntry.URL = line
			if err := p.OnEntry(currentEntry); err != nil {
				return fmt.Errorf("callback error at line %d: %w", lineNum, err)
			}
			currentEntry = nil
		} else if isExtM3U {
			// URL without EXTINF - create minimal entry
			entry := &Entry{
				Duration: -1,
				URL:      line,
				Title:    extractTitleFromURL(line),
			}
			if err := p.OnEntry(entry); err != nil {
				return fmt.Errorf("callback error at line %d: %w", lineNum, err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanning M3U: %w", err)
	}

	return nil
}

// ParseCompressed parses a potentially compressed M3U playlist.
// It auto-detects compression based on magic bytes.
func (p *Parser) ParseCompressed(r io.Reader) error {
	// We need to peek at the first bytes to detect compression
	br := bufio.NewReader(r)

	// Peek at first bytes to detect compression
	header, err := br.Peek(6)
	if err != nil && err != io.EOF {
		return fmt.Errorf("peeking header: %w", err)
	}

	var reader io.Reader = br

	// Detect compression format
	switch {
	case len(header) >= 2 && header[0] == 0x1f && header[1] == 0x8b:
		// Gzip
		gzr, err := gzip.NewReader(br)
		if err != nil {
			return fmt.Errorf("creating gzip reader: %w", err)
		}
		defer gzr.Close()
		reader = gzr

	case len(header) >= 3 && header[0] == 'B' && header[1] == 'Z' && header[2] == 'h':
		// Bzip2
		reader = bzip2.NewReader(br)

	case len(header) >= 6 && header[0] == 0xfd && header[1] == '7' && header[2] == 'z' && header[3] == 'X' && header[4] == 'Z' && header[5] == 0x00:
		// XZ
		xzr, err := xz.NewReader(br)
		if err != nil {
			return fmt.Errorf("creating xz reader: %w", err)
		}
		reader = xzr
	}

	return p.Parse(reader)
}

// parseExtinf parses an EXTINF line and extracts metadata.
func (p *Parser) parseExtinf(line string) (*Entry, error) {
	matches := extinfRegex.FindStringSubmatch(line)
	if matches == nil {
		return nil, fmt.Errorf("invalid EXTINF format")
	}

	duration, _ := strconv.Atoi(matches[1])
	remainder := matches[2]

	entry := &Entry{
		Duration: duration,
		Extra:    make(map[string]string),
	}

	// Find the title (everything after the last comma not in quotes)
	titleIdx := findTitleStart(remainder)
	if titleIdx >= 0 {
		entry.Title = strings.TrimSpace(remainder[titleIdx+1:])
		remainder = remainder[:titleIdx]
	}

	// Parse attributes
	attrMatches := attrRegex.FindAllStringSubmatch(remainder, -1)
	for _, match := range attrMatches {
		key := strings.ToLower(match[1])
		value := match[2]
		if value == "" {
			value = match[3]
		}

		switch key {
		case "tvg-id":
			entry.TvgID = value
		case "tvg-name":
			entry.TvgName = value
		case "tvg-logo":
			entry.TvgLogo = value
		case "group-title":
			entry.GroupTitle = value
		case "tvg-chno":
			entry.ChannelNumber, _ = strconv.Atoi(value)
		default:
			entry.Extra[key] = value
		}
	}

	return entry, nil
}

// findTitleStart finds the index of the comma that separates attributes from title.
// It handles commas inside quoted values.
func findTitleStart(s string) int {
	inQuotes := false
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '"' {
			inQuotes = !inQuotes
		}
		if s[i] == ',' && !inQuotes {
			return i
		}
	}
	return -1
}

// extractTitleFromURL extracts a title from a URL when no EXTINF is present.
func extractTitleFromURL(url string) string {
	// Try to extract filename
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		filename := parts[len(parts)-1]
		// Remove query string
		if idx := strings.Index(filename, "?"); idx > 0 {
			filename = filename[:idx]
		}
		// Remove extension
		if idx := strings.LastIndex(filename, "."); idx > 0 {
			filename = filename[:idx]
		}
		if filename != "" {
			return filename
		}
	}
	return "Unknown"
}

// handleError calls the OnError callback if set.
func (p *Parser) handleError(lineNum int, err error) {
	if p.OnError != nil {
		p.OnError(lineNum, err)
	}
}
