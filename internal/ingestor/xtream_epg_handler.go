// Package ingestor provides source ingestion handlers for stream and EPG sources.
package ingestor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/jmylchreest/tvarr/pkg/httpclient"
	"github.com/jmylchreest/tvarr/pkg/xmltv"
	"github.com/jmylchreest/tvarr/pkg/xtream"
)

// EPG handler configuration defaults.
const (
	defaultEpgTimeout  = 5 * time.Minute
	defaultDaysToFetch = 7
	httpSchemePrefix   = "http://"
	httpsSchemePrefix  = "https://"
)

// XtreamEpgHandler handles EPG ingestion from Xtream Codes API sources.
type XtreamEpgHandler struct {
	httpClient     *httpclient.Client
	DaysToFetch    int
	logger         *slog.Logger
	serverLocation *time.Location
}

// NewXtreamEpgHandler creates a new Xtream EPG handler with default settings.
func NewXtreamEpgHandler() *XtreamEpgHandler {
	cfg := httpclient.DefaultConfig()
	cfg.Timeout = defaultEpgTimeout
	breaker := httpclient.DefaultManager.GetOrCreate("xtream-epg")

	return &XtreamEpgHandler{
		httpClient:  httpclient.NewWithBreaker(cfg, breaker),
		DaysToFetch: defaultDaysToFetch,
	}
}

// WithHTTPClientConfig sets a custom HTTP client configuration.
func (h *XtreamEpgHandler) WithHTTPClientConfig(cfg httpclient.Config) *XtreamEpgHandler {
	breaker := httpclient.DefaultManager.GetOrCreate("xtream-epg")
	h.httpClient = httpclient.NewWithBreaker(cfg, breaker)
	return h
}

// WithDaysToFetch sets the number of days of EPG data to fetch.
func (h *XtreamEpgHandler) WithDaysToFetch(days int) *XtreamEpgHandler {
	h.DaysToFetch = days
	return h
}

// WithLogger sets a structured logger for the handler.
func (h *XtreamEpgHandler) WithLogger(logger *slog.Logger) *XtreamEpgHandler {
	h.logger = logger
	return h
}

// Type returns the EPG source type this handler supports.
func (h *XtreamEpgHandler) Type() models.EpgSourceType {
	return models.EpgSourceTypeXtream
}

// Validate checks if the EPG source configuration is valid for Xtream.
func (h *XtreamEpgHandler) Validate(source *models.EpgSource) error {
	if source == nil {
		return fmt.Errorf("source is nil")
	}
	if source.Type != models.EpgSourceTypeXtream {
		return fmt.Errorf("invalid source type: expected %s, got %s", models.EpgSourceTypeXtream, source.Type)
	}
	if source.URL == "" {
		return fmt.Errorf("URL is required for Xtream EPG sources")
	}
	if !strings.HasPrefix(source.URL, httpSchemePrefix) && !strings.HasPrefix(source.URL, httpsSchemePrefix) {
		return fmt.Errorf("URL must be an HTTP(S) URL")
	}
	if source.Username == "" {
		return fmt.Errorf("username is required for Xtream EPG sources")
	}
	if source.Password == "" {
		return fmt.Errorf("password is required for Xtream EPG sources")
	}
	return nil
}

// xtreamEpgBreakerNameFromURL extracts a circuit breaker name from the source URL.
func xtreamEpgBreakerNameFromURL(sourceURL string) string {
	parsed, err := url.Parse(sourceURL)
	if err != nil || parsed.Host == "" {
		return "xtream-epg"
	}
	return "epg-" + parsed.Host
}

// Ingest fetches EPG data from the Xtream API and yields programs via the callback.
func (h *XtreamEpgHandler) Ingest(ctx context.Context, source *models.EpgSource, callback ProgramCallback) error {
	if err := h.Validate(source); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	breakerName := xtreamEpgBreakerNameFromURL(source.URL)
	breaker := httpclient.DefaultManager.GetOrCreate(breakerName)

	cfg := httpclient.DefaultConfig()
	cfg.Timeout = defaultEpgTimeout
	httpClient := httpclient.NewWithBreaker(cfg, breaker)

	client := xtream.NewClient(
		source.URL,
		source.Username,
		source.Password,
		xtream.WithHTTPClient(httpClient.StandardClient()),
	)

	h.serverLocation = nil

	if authInfo, err := client.GetAuthInfo(ctx); err == nil {
		if authInfo.ServerInfo.Timezone != "" {
			detectedTz := authInfo.ServerInfo.Timezone
			source.DetectedTimezone = FormatTimezoneOffset(detectedTz)

			if loc, err := time.LoadLocation(detectedTz); err == nil {
				h.serverLocation = loc
				if h.logger != nil {
					h.logger.Debug("loaded server timezone location",
						slog.String("timezone", detectedTz),
						slog.String("source_name", source.Name),
					)
				}
			} else if h.logger != nil {
				h.logger.Debug("could not load server timezone as location, will use UTC for string parsing",
					slog.String("timezone", detectedTz),
					slog.String("error", err.Error()),
				)
			}

			if source.AutoShiftTimezone != detectedTz {
				newShift := CalculateAutoShift(detectedTz)
				if h.logger != nil {
					h.logger.Info("auto-adjusting EPG timeshift based on detected timezone",
						slog.String("source_name", source.Name),
						slog.String("detected_timezone", detectedTz),
						slog.String("previous_auto_shift_timezone", source.AutoShiftTimezone),
						slog.Int("previous_epg_shift", source.EpgShift),
						slog.Int("new_epg_shift", newShift),
					)
				}
				source.EpgShift = newShift
				source.AutoShiftTimezone = detectedTz
			}

			LogTimezoneDetection(h.logger, source.Name, source.ID.String(), source.DetectedTimezone, source.ProgramCount)
		}
	}

	if source.EpgShift != 0 {
		LogTimezoneNormalization(h.logger, source.Name, source.DetectedTimezone, source.EpgShift)
	}

	apiMethod := source.ApiMethod
	if apiMethod == "" {
		apiMethod = models.XtreamApiMethodStreamID
	}

	switch apiMethod {
	case models.XtreamApiMethodBulkXMLTV:
		return h.ingestBulkXMLTV(ctx, client, source, callback)
	case models.XtreamApiMethodStreamID:
		fallthrough
	default:
		return h.ingestPerStream(ctx, client, source, callback)
	}
}

// ingestPerStream fetches EPG data using the per-stream JSON API.
func (h *XtreamEpgHandler) ingestPerStream(ctx context.Context, client *xtream.Client, source *models.EpgSource, callback ProgramCallback) error {
	streams, err := client.GetLiveStreams(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to fetch live streams: %w", err)
	}

	for _, stream := range streams {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if stream.EPGChannelID == "" {
			continue
		}

		epgListings, err := client.GetFullEPG(ctx, int(stream.StreamID.Int()))
		if err != nil {
			continue
		}

		for _, listing := range epgListings {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			program := h.convertListing(listing, source, stream.EPGChannelID)

			if err := program.Validate(); err != nil {
				continue
			}

			if err := callback(program); err != nil {
				return fmt.Errorf("callback error: %w", err)
			}
		}
	}

	return nil
}

// ingestBulkXMLTV fetches EPG data using the bulk /xmltv.php endpoint.
func (h *XtreamEpgHandler) ingestBulkXMLTV(ctx context.Context, client *xtream.Client, source *models.EpgSource, callback ProgramCallback) error {
	reader, err := client.GetXMLTVReader(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch XMLTV: %w", err)
	}
	defer reader.Close()

	parser := &xmltv.Parser{
		OnChannel: func(channel *xmltv.Channel) error {
			return nil
		},
		OnProgramme: func(programme *xmltv.Programme) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			program := h.convertXMLTVProgramme(programme, source)

			if err := program.Validate(); err != nil {
				return nil
			}

			return callback(program)
		},
		OnError: func(err error) {
		},
	}

	if err := parser.ParseCompressed(reader); err != nil {
		return fmt.Errorf("failed to parse XMLTV: %w", err)
	}

	return nil
}

// convertListing converts an Xtream EPG listing to an EpgProgram model.
func (h *XtreamEpgHandler) convertListing(listing xtream.EPGListing, source *models.EpgSource, channelID string) *models.EpgProgram {
	start := h.applyTimeOffset(listing.StartTimeInLocation(h.serverLocation), source)
	stop := h.applyTimeOffset(listing.EndTimeInLocation(h.serverLocation), source)

	program := &models.EpgProgram{
		SourceID:    source.ID,
		ChannelID:   channelID,
		Title:       decodeBase64OrOriginal(listing.Title),
		Description: decodeBase64OrOriginal(listing.Description),
		Language:    listing.Lang,
		Start:       start,
		Stop:        stop,
	}

	return program
}

// decodeBase64OrOriginal attempts to decode a base64 string.
func decodeBase64OrOriginal(s string) string {
	if s == "" {
		return s
	}
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return s
	}
	return string(decoded)
}

// convertXMLTVProgramme converts an XMLTV Programme to an EpgProgram model.
func (h *XtreamEpgHandler) convertXMLTVProgramme(p *xmltv.Programme, source *models.EpgSource) *models.EpgProgram {
	start := h.applyTimeOffset(p.Start, source)
	stop := h.applyTimeOffset(p.Stop, source)

	program := &models.EpgProgram{
		SourceID:    source.ID,
		ChannelID:   p.Channel,
		Start:       start,
		Stop:        stop,
		Title:       p.Title,
		SubTitle:    p.SubTitle,
		Description: p.Description,
		Category:    p.Category,
		Icon:        p.Icon,
		EpisodeNum:  p.EpisodeNum,
		Rating:      p.Rating,
		Language:    p.Language,
		IsNew:       p.IsNew,
		IsPremiere:  p.IsPremiere,
	}

	if p.Credits != nil {
		credits := make(map[string][]string)
		if len(p.Credits.Directors) > 0 {
			credits["directors"] = p.Credits.Directors
		}
		if len(p.Credits.Actors) > 0 {
			credits["actors"] = p.Credits.Actors
		}
		if len(p.Credits.Writers) > 0 {
			credits["writers"] = p.Credits.Writers
		}
		if len(p.Credits.Producers) > 0 {
			credits["producers"] = p.Credits.Producers
		}
		if len(p.Credits.Presenters) > 0 {
			credits["presenters"] = p.Credits.Presenters
		}
		if len(credits) > 0 {
			if data, err := json.Marshal(credits); err == nil {
				program.Credits = string(data)
			}
		}
	}

	return program
}

// applyTimeOffset applies timezone normalization and the source's EpgShift to a time value.
func (h *XtreamEpgHandler) applyTimeOffset(t time.Time, source *models.EpgSource) time.Time {
	if source == nil {
		return t
	}

	var detectedOffset time.Duration
	if source.DetectedTimezone != "" {
		var err error
		detectedOffset, err = ParseTimezoneOffset(source.DetectedTimezone)
		if err != nil && h.logger != nil {
			h.logger.Debug("failed to parse detected timezone offset",
				slog.String("offset", source.DetectedTimezone),
				slog.String("error", err.Error()),
			)
		}
	}

	return NormalizeProgramTime(t, detectedOffset, source.EpgShift)
}

// Ensure XtreamEpgHandler implements EpgHandler.
var _ EpgHandler = (*XtreamEpgHandler)(nil)
