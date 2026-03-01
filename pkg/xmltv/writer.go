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

	// Display-names (emit all if available, otherwise just DisplayName)
	if len(ch.DisplayNames) > 0 {
		for _, name := range ch.DisplayNames {
			_, err = fmt.Fprintf(w.w, `    <display-name>%s</display-name>`, xmlEscape(name))
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(w.w)
			if err != nil {
				return err
			}
		}
	} else if ch.DisplayName != "" {
		_, err = fmt.Fprintf(w.w, `    <display-name>%s</display-name>`, xmlEscape(ch.DisplayName))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w.w)
		if err != nil {
			return err
		}
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
// Element ordering follows the XMLTV DTD specification:
//
//	title, sub-title, desc, credits, date, category, keyword, language,
//	orig-language, length, icon, url, country, episode-num, video, audio,
//	previously-shown, premiere, last-chance, new, subtitles, rating,
//	star-rating, review, image
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

	// Determine language attribute: use program's language if set, omit if unknown.
	// Jellyfin filters elements by lang matching the user's preferred language;
	// omitting lang causes it to be treated as a universal match.
	lang := prog.Language

	// --- DTD order: title (required) ---
	if lang != "" {
		_, err = fmt.Fprintf(w.w, `    <title lang="%s">%s</title>`, lang, xmlEscape(prog.Title))
	} else {
		_, err = fmt.Fprintf(w.w, `    <title>%s</title>`, xmlEscape(prog.Title))
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w.w)
	if err != nil {
		return err
	}

	// --- DTD order: sub-title ---
	if prog.SubTitle != "" {
		if lang != "" {
			_, err = fmt.Fprintf(w.w, `    <sub-title lang="%s">%s</sub-title>`, lang, xmlEscape(prog.SubTitle))
		} else {
			_, err = fmt.Fprintf(w.w, `    <sub-title>%s</sub-title>`, xmlEscape(prog.SubTitle))
		}
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w.w)
		if err != nil {
			return err
		}
	}

	// --- DTD order: desc ---
	if prog.Description != "" {
		if lang != "" {
			_, err = fmt.Fprintf(w.w, `    <desc lang="%s">%s</desc>`, lang, xmlEscape(prog.Description))
		} else {
			_, err = fmt.Fprintf(w.w, `    <desc>%s</desc>`, xmlEscape(prog.Description))
		}
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w.w)
		if err != nil {
			return err
		}
	}

	// --- DTD order: credits ---
	if prog.Credits != nil {
		if err := w.writeCredits(prog.Credits); err != nil {
			return err
		}
	}

	// --- DTD order: date ---
	if prog.Date != "" {
		_, err = fmt.Fprintf(w.w, `    <date>%s</date>`, xmlEscape(prog.Date))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w.w)
		if err != nil {
			return err
		}
	}

	// --- DTD order: category ---
	categories := prog.Categories
	if len(categories) == 0 && prog.Category != "" {
		categories = []string{prog.Category}
	}
	for _, category := range categories {
		if lang != "" {
			_, err = fmt.Fprintf(w.w, `    <category lang="%s">%s</category>`, lang, xmlEscape(category))
		} else {
			_, err = fmt.Fprintf(w.w, `    <category>%s</category>`, xmlEscape(category))
		}
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w.w)
		if err != nil {
			return err
		}
	}

	// --- DTD order: icon ---
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

	// --- DTD order: episode-num ---
	// xmltv_ns format: values are 1-based in model (0 = unknown), xmltv_ns uses 0-based
	if prog.SeasonNumber > 0 || prog.EpisodeNumber > 0 {
		season := prog.SeasonNumber - 1
		if season < 0 {
			season = 0
		}
		episode := prog.EpisodeNumber - 1
		if episode < 0 {
			episode = 0
		}
		_, err = fmt.Fprintf(w.w, `    <episode-num system="xmltv_ns">%d.%d.</episode-num>`, season, episode)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w.w)
		if err != nil {
			return err
		}

		// SxxExx format for broader compatibility (Plex onscreen support)
		if prog.SeasonNumber > 0 && prog.EpisodeNumber > 0 {
			_, err = fmt.Fprintf(w.w, `    <episode-num system="onscreen">S%02dE%02d</episode-num>`,
				prog.SeasonNumber, prog.EpisodeNumber)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(w.w)
			if err != nil {
				return err
			}
		}
	}

	// dd_progid format
	if prog.ProgramID != "" {
		_, err = fmt.Fprintf(w.w, `    <episode-num system="dd_progid">%s</episode-num>`, xmlEscape(prog.ProgramID))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w.w)
		if err != nil {
			return err
		}
	}

	// --- DTD order: previously-shown ---
	if prog.PreviouslyShown {
		_, err = fmt.Fprintln(w.w, `    <previously-shown/>`)
		if err != nil {
			return err
		}
	}

	// --- DTD order: premiere ---
	if prog.IsPremiere {
		_, err = fmt.Fprintln(w.w, `    <premiere/>`)
		if err != nil {
			return err
		}
	}

	// --- DTD order: new ---
	if prog.IsNew {
		_, err = fmt.Fprintln(w.w, `    <new/>`)
		if err != nil {
			return err
		}
	}

	// --- DTD order: rating ---
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

	// --- DTD order: star-rating ---
	if prog.StarRating != "" {
		_, err = fmt.Fprintf(w.w, `    <star-rating><value>%s</value></star-rating>`, xmlEscape(prog.StarRating))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w.w)
		if err != nil {
			return err
		}
	}

	// --- Non-DTD extension: live (Jellyfin-specific) ---
	if prog.IsLive {
		_, err = fmt.Fprintln(w.w, `    <live/>`)
		if err != nil {
			return err
		}
	}

	_, err = fmt.Fprintln(w.w, `  </programme>`)
	return err
}

// writeCredits writes the credits element with proper DTD-ordered sub-elements.
// DTD order: director, actor, writer, adapter, producer, composer, editor,
// presenter, commentator, guest.
func (w *Writer) writeCredits(credits *Credits) error {
	_, err := fmt.Fprintln(w.w, `    <credits>`)
	if err != nil {
		return err
	}
	for _, director := range credits.Directors {
		if _, err = fmt.Fprintf(w.w, "      <director>%s</director>\n", xmlEscape(director)); err != nil {
			return err
		}
	}
	for _, actor := range credits.Actors {
		if _, err = fmt.Fprintf(w.w, "      <actor>%s</actor>\n", xmlEscape(actor)); err != nil {
			return err
		}
	}
	for _, writer := range credits.Writers {
		if _, err = fmt.Fprintf(w.w, "      <writer>%s</writer>\n", xmlEscape(writer)); err != nil {
			return err
		}
	}
	for _, producer := range credits.Producers {
		if _, err = fmt.Fprintf(w.w, "      <producer>%s</producer>\n", xmlEscape(producer)); err != nil {
			return err
		}
	}
	for _, presenter := range credits.Presenters {
		if _, err = fmt.Fprintf(w.w, "      <presenter>%s</presenter>\n", xmlEscape(presenter)); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(w.w, `    </credits>`)
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
	_ = xml.EscapeText((*xmlEscapeWriter)(&buf), []byte(s))
	return string(buf)
}

// xmlEscapeWriter is a helper for xml.EscapeText.
type xmlEscapeWriter []byte

func (w *xmlEscapeWriter) Write(p []byte) (int, error) {
	*w = append(*w, p...)
	return len(p), nil
}
