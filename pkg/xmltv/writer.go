package xmltv

import (
	"encoding/xml"
	"fmt"
	"io"
	"time"
)

// Writer provides streaming XMLTV file writing.
type Writer struct {
	w             io.Writer
	headerWritten bool
	channelsDone  bool
}

// NewWriter creates a new XMLTV writer.
func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// WriteHeader writes the XML declaration and opens the tv element.
func (w *Writer) WriteHeader() error {
	if w.headerWritten {
		return nil
	}
	_, err := fmt.Fprintln(w.w, `<?xml version="1.0" encoding="UTF-8"?>`)
	if err != nil {
		return fmt.Errorf("writing XML declaration: %w", err)
	}
	_, err = fmt.Fprintln(w.w, `<tv generator-info-name="tvarr" generator-info-url="https://github.com/jmylchreest/tvarr">`)
	if err != nil {
		return fmt.Errorf("writing tv element: %w", err)
	}
	w.headerWritten = true
	return nil
}

// WriteChannel writes a channel definition.
// All channels must be written before any programmes.
func (w *Writer) WriteChannel(ch *Channel) error {
	if err := w.WriteHeader(); err != nil {
		return err
	}
	if w.channelsDone {
		return fmt.Errorf("channels must be written before programmes")
	}

	_, err := fmt.Fprintf(w.w, `  <channel id="%s">`, xmlEscape(ch.ID))
	if err != nil {
		return fmt.Errorf("writing channel start: %w", err)
	}
	_, err = fmt.Fprintln(w.w)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(w.w, `    <display-name>%s</display-name>`, xmlEscape(ch.DisplayName))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w.w)
	if err != nil {
		return err
	}

	if ch.Icon != "" {
		_, err = fmt.Fprintf(w.w, `    <icon src="%s"/>`, xmlEscape(ch.Icon))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w.w)
		if err != nil {
			return err
		}
	}

	if ch.URL != "" {
		_, err = fmt.Fprintf(w.w, `    <url>%s</url>`, xmlEscape(ch.URL))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w.w)
		if err != nil {
			return err
		}
	}

	_, err = fmt.Fprintln(w.w, `  </channel>`)
	return err
}

// WriteProgramme writes a programme entry.
func (w *Writer) WriteProgramme(prog *Programme) error {
	if err := w.WriteHeader(); err != nil {
		return err
	}
	w.channelsDone = true

	startStr := formatXMLTVTime(prog.Start)
	stopStr := formatXMLTVTime(prog.Stop)

	_, err := fmt.Fprintf(w.w, `  <programme start="%s" stop="%s" channel="%s">`,
		startStr, stopStr, xmlEscape(prog.Channel))
	if err != nil {
		return fmt.Errorf("writing programme start: %w", err)
	}
	_, err = fmt.Fprintln(w.w)
	if err != nil {
		return err
	}

	// Title (required)
	lang := prog.Language
	if lang == "" {
		lang = "en"
	}
	_, err = fmt.Fprintf(w.w, `    <title lang="%s">%s</title>`, lang, xmlEscape(prog.Title))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w.w)
	if err != nil {
		return err
	}

	// Sub-title
	if prog.SubTitle != "" {
		_, err = fmt.Fprintf(w.w, `    <sub-title lang="%s">%s</sub-title>`, lang, xmlEscape(prog.SubTitle))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w.w)
		if err != nil {
			return err
		}
	}

	// Description
	if prog.Description != "" {
		_, err = fmt.Fprintf(w.w, `    <desc lang="%s">%s</desc>`, lang, xmlEscape(prog.Description))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w.w)
		if err != nil {
			return err
		}
	}

	// Category
	if prog.Category != "" {
		_, err = fmt.Fprintf(w.w, `    <category lang="%s">%s</category>`, lang, xmlEscape(prog.Category))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w.w)
		if err != nil {
			return err
		}
	}

	// Icon
	if prog.Icon != "" {
		_, err = fmt.Fprintf(w.w, `    <icon src="%s"/>`, xmlEscape(prog.Icon))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w.w)
		if err != nil {
			return err
		}
	}

	// Episode number
	if prog.EpisodeNum != "" {
		_, err = fmt.Fprintf(w.w, `    <episode-num system="onscreen">%s</episode-num>`, xmlEscape(prog.EpisodeNum))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w.w)
		if err != nil {
			return err
		}
	}

	// Rating
	if prog.Rating != "" {
		_, err = fmt.Fprintf(w.w, `    <rating><value>%s</value></rating>`, xmlEscape(prog.Rating))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w.w)
		if err != nil {
			return err
		}
	}

	// Flags
	if prog.IsNew {
		_, err = fmt.Fprintln(w.w, `    <new/>`)
		if err != nil {
			return err
		}
	}
	if prog.IsPremiere {
		_, err = fmt.Fprintln(w.w, `    <premiere/>`)
		if err != nil {
			return err
		}
	}

	_, err = fmt.Fprintln(w.w, `  </programme>`)
	return err
}

// WriteFooter closes the tv element.
func (w *Writer) WriteFooter() error {
	_, err := fmt.Fprintln(w.w, `</tv>`)
	return err
}

// formatXMLTVTime formats a time in XMLTV format.
func formatXMLTVTime(t time.Time) string {
	return t.UTC().Format("20060102150405 +0000")
}

// xmlEscape escapes special XML characters.
func xmlEscape(s string) string {
	var buf []byte
	xml.EscapeText((*xmlEscapeWriter)(&buf), []byte(s))
	return string(buf)
}

// xmlEscapeWriter is a helper for xml.EscapeText.
type xmlEscapeWriter []byte

func (w *xmlEscapeWriter) Write(p []byte) (int, error) {
	*w = append(*w, p...)
	return len(p), nil
}
