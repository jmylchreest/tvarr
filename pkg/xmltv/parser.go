// Package xmltv provides streaming XMLTV parsing and writing.
// It supports standard XMLTV format for electronic program guide data.
package xmltv

import (
	"bufio"
	"compress/bzip2"
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ulikunitz/xz"
)

// Programme represents a single program entry in an XMLTV file.
type Programme struct {
	Start          time.Time
	Stop           time.Time
	Channel        string
	Title          string
	SubTitle       string
	Description    string
	Category       string
	Icon           string
	EpisodeNum     string
	Rating         string
	Language       string
	IsNew          bool
	IsPremiere     bool
	Credits        *Credits
	TimezoneOffset string // The timezone offset from the start time (e.g., "+0000", "-0500")
}

// Credits holds cast and crew information.
type Credits struct {
	Directors  []string
	Actors     []string
	Writers    []string
	Producers  []string
	Presenters []string
}

// Channel represents a channel definition in an XMLTV file.
type Channel struct {
	ID          string
	DisplayName string
	Icon        string
	URL         string
}

// Parser provides streaming XMLTV parsing with callback-based processing.
type Parser struct {
	// OnChannel is called for each channel definition.
	OnChannel func(channel *Channel) error

	// OnProgramme is called for each parsed programme.
	OnProgramme func(programme *Programme) error

	// OnError is called for recoverable parsing errors.
	OnError func(err error)
}

// ParsedTime contains a parsed time value and the timezone offset string if present.
type ParsedTime struct {
	Time           time.Time
	TimezoneOffset string // e.g., "+0000", "-0500", "" if not present
}

// parseXMLTVTime parses XMLTV time format: "20240101120000 +0000"
// Returns the parsed time and the timezone offset string if present.
func parseXMLTVTime(s string) (ParsedTime, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return ParsedTime{}, fmt.Errorf("empty time string")
	}

	// Extract timezone offset if present (e.g., "+0000", "-0500")
	var tzOffset string
	parts := strings.SplitN(s, " ", 2)
	if len(parts) == 2 {
		tzOffset = strings.TrimSpace(parts[1])
	}

	// Try different formats
	formats := []string{
		"20060102150405 -0700",
		"20060102150405 +0000",
		"20060102150405",
		"200601021504",
	}

	for _, format := range formats {
		t, err := time.Parse(format, s)
		if err == nil {
			return ParsedTime{Time: t, TimezoneOffset: tzOffset}, nil
		}
	}

	return ParsedTime{}, fmt.Errorf("unable to parse time: %s", s)
}

// Parse parses an XMLTV file from a reader.
func (p *Parser) Parse(r io.Reader) error {
	decoder := xml.NewDecoder(r)
	decoder.Strict = false
	decoder.AutoClose = xml.HTMLAutoClose
	decoder.Entity = xml.HTMLEntity

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading XML token: %w", err)
		}

		switch elem := token.(type) {
		case xml.StartElement:
			switch elem.Name.Local {
			case "channel":
				if p.OnChannel != nil {
					channel, err := p.parseChannel(decoder, elem)
					if err != nil {
						p.handleError(err)
						continue
					}
					if err := p.OnChannel(channel); err != nil {
						return fmt.Errorf("channel callback: %w", err)
					}
				} else {
					_ = decoder.Skip()
				}

			case "programme":
				if p.OnProgramme != nil {
					programme, err := p.parseProgramme(decoder, elem)
					if err != nil {
						p.handleError(err)
						continue
					}
					if err := p.OnProgramme(programme); err != nil {
						return fmt.Errorf("programme callback: %w", err)
					}
				} else {
					_ = decoder.Skip()
				}
			}
		}
	}

	return nil
}

// ParseCompressed parses a potentially compressed XMLTV file.
// It auto-detects compression based on magic bytes.
func (p *Parser) ParseCompressed(r io.Reader) error {
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

// parseChannel parses a channel element.
func (p *Parser) parseChannel(decoder *xml.Decoder, start xml.StartElement) (*Channel, error) {
	channel := &Channel{}

	// Get ID from attributes
	for _, attr := range start.Attr {
		if attr.Name.Local == "id" {
			channel.ID = attr.Value
		}
	}

	// Parse child elements
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}

		switch elem := token.(type) {
		case xml.StartElement:
			switch elem.Name.Local {
			case "display-name":
				var name string
				if err := decoder.DecodeElement(&name, &elem); err == nil && channel.DisplayName == "" {
					channel.DisplayName = name
				}
			case "icon":
				for _, attr := range elem.Attr {
					if attr.Name.Local == "src" {
						channel.Icon = attr.Value
					}
				}
				_ = decoder.Skip()
			case "url":
				var url string
				if err := decoder.DecodeElement(&url, &elem); err == nil {
					channel.URL = url
				}
			default:
				_ = decoder.Skip()
			}
		case xml.EndElement:
			if elem.Name.Local == "channel" {
				return channel, nil
			}
		}
	}
}

// parseProgramme parses a programme element.
func (p *Parser) parseProgramme(decoder *xml.Decoder, start xml.StartElement) (*Programme, error) {
	prog := &Programme{}

	// Get attributes
	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "start":
			parsed, err := parseXMLTVTime(attr.Value)
			if err == nil {
				prog.Start = parsed.Time
				prog.TimezoneOffset = parsed.TimezoneOffset // Capture timezone from start time
			}
		case "stop":
			parsed, err := parseXMLTVTime(attr.Value)
			if err == nil {
				prog.Stop = parsed.Time
			}
		case "channel":
			prog.Channel = attr.Value
		}
	}

	// Parse child elements
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}

		switch elem := token.(type) {
		case xml.StartElement:
			switch elem.Name.Local {
			case "title":
				var title string
				if err := decoder.DecodeElement(&title, &elem); err == nil && prog.Title == "" {
					prog.Title = strings.TrimSpace(title)
				}
			case "sub-title":
				var subtitle string
				if err := decoder.DecodeElement(&subtitle, &elem); err == nil {
					prog.SubTitle = strings.TrimSpace(subtitle)
				}
			case "desc":
				var desc string
				if err := decoder.DecodeElement(&desc, &elem); err == nil {
					prog.Description = strings.TrimSpace(desc)
				}
			case "category":
				var cat string
				if err := decoder.DecodeElement(&cat, &elem); err == nil && prog.Category == "" {
					prog.Category = strings.TrimSpace(cat)
				}
			case "icon":
				for _, attr := range elem.Attr {
					if attr.Name.Local == "src" {
						prog.Icon = attr.Value
					}
				}
				_ = decoder.Skip()
			case "episode-num":
				var epNum string
				if err := decoder.DecodeElement(&epNum, &elem); err == nil {
					prog.EpisodeNum = strings.TrimSpace(epNum)
				}
			case "rating":
				p.parseRating(decoder, &elem, prog)
			case "language":
				var lang string
				if err := decoder.DecodeElement(&lang, &elem); err == nil {
					prog.Language = strings.TrimSpace(lang)
				}
			case "new":
				prog.IsNew = true
				_ = decoder.Skip()
			case "premiere":
				prog.IsPremiere = true
				_ = decoder.Skip()
			case "credits":
				prog.Credits = p.parseCredits(decoder)
			default:
				_ = decoder.Skip()
			}
		case xml.EndElement:
			if elem.Name.Local == "programme" {
				return prog, nil
			}
		}
	}
}

// parseRating parses the rating element.
func (p *Parser) parseRating(decoder *xml.Decoder, start *xml.StartElement, prog *Programme) {
	for {
		token, err := decoder.Token()
		if err != nil {
			return
		}

		switch elem := token.(type) {
		case xml.StartElement:
			if elem.Name.Local == "value" {
				var value string
				if err := decoder.DecodeElement(&value, &elem); err == nil {
					prog.Rating = strings.TrimSpace(value)
				}
			} else {
				_ = decoder.Skip()
			}
		case xml.EndElement:
			if elem.Name.Local == "rating" {
				return
			}
		}
	}
}

// parseCredits parses the credits element.
func (p *Parser) parseCredits(decoder *xml.Decoder) *Credits {
	credits := &Credits{}

	for {
		token, err := decoder.Token()
		if err != nil {
			return credits
		}

		switch elem := token.(type) {
		case xml.StartElement:
			var value string
			if err := decoder.DecodeElement(&value, &elem); err == nil {
				value = strings.TrimSpace(value)
				switch elem.Name.Local {
				case "director":
					credits.Directors = append(credits.Directors, value)
				case "actor":
					credits.Actors = append(credits.Actors, value)
				case "writer":
					credits.Writers = append(credits.Writers, value)
				case "producer":
					credits.Producers = append(credits.Producers, value)
				case "presenter":
					credits.Presenters = append(credits.Presenters, value)
				}
			}
		case xml.EndElement:
			if elem.Name.Local == "credits" {
				return credits
			}
		}
	}
}

// handleError calls the OnError callback if set.
func (p *Parser) handleError(err error) {
	if p.OnError != nil {
		p.OnError(err)
	}
}

// ParseAll parses an entire XMLTV file and returns all programmes.
// Note: This loads all programmes into memory - use Parse with callbacks for large files.
func ParseAll(r io.Reader) ([]*Programme, error) {
	var programmes []*Programme
	p := &Parser{
		OnProgramme: func(prog *Programme) error {
			programmes = append(programmes, prog)
			return nil
		},
	}
	if err := p.Parse(r); err != nil {
		return nil, err
	}
	return programmes, nil
}

// ParseString parses an XMLTV string and calls the callback for each programme.
func ParseString(content string, onProgramme func(*Programme) error) error {
	p := &Parser{OnProgramme: onProgramme}
	return p.Parse(strings.NewReader(content))
}
