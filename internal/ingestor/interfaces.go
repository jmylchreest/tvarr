// Package ingestor provides source ingestion handlers for stream and EPG sources.
package ingestor

import (
	"context"
	"io"

	"github.com/jmylchreest/tvarr/internal/models"
)

// SourceHandler defines the interface for processing different source types.
type SourceHandler interface {
	// Type returns the source type this handler supports (e.g., "m3u", "xtream").
	Type() models.SourceType

	// Ingest processes a source and yields channels via the callback.
	// The callback is called for each parsed channel, allowing streaming processing.
	// If the callback returns an error, ingestion stops and the error is returned.
	Ingest(ctx context.Context, source *models.StreamSource, callback ChannelCallback) error

	// Validate checks if the source configuration is valid for this handler.
	Validate(source *models.StreamSource) error
}

// ChannelCallback is called for each channel during ingestion.
// Returning an error stops the ingestion process.
type ChannelCallback func(channel *models.Channel) error

// EpgHandler defines the interface for processing EPG sources.
type EpgHandler interface {
	// Type returns the EPG source type this handler supports (e.g., "xmltv", "xtream").
	Type() models.EpgSourceType

	// Ingest processes an EPG source and yields programs via the callback.
	// The callback is called for each parsed program, allowing streaming processing.
	// If the callback returns an error, ingestion stops and the error is returned.
	Ingest(ctx context.Context, source *models.EpgSource, callback ProgramCallback) error

	// Validate checks if the EPG source configuration is valid for this handler.
	Validate(source *models.EpgSource) error
}

// ProgramCallback is called for each program during EPG ingestion.
// Returning an error stops the ingestion process.
type ProgramCallback func(program *models.EpgProgram) error

// Fetcher defines how to retrieve source content.
type Fetcher interface {
	// Fetch retrieves content from a URL and returns a reader.
	// The caller is responsible for closing the reader.
	Fetch(ctx context.Context, url string) (io.ReadCloser, error)
}

// HTTPFetcher implements Fetcher using HTTP requests.
type HTTPFetcher struct {
	// UserAgent is the User-Agent header to send with requests.
	UserAgent string

	// Timeout is the request timeout. Zero means no timeout.
	Timeout int
}

// IngestStats contains statistics from an ingestion operation.
type IngestStats struct {
	// TotalProcessed is the number of entries processed.
	TotalProcessed int

	// SuccessCount is the number successfully ingested.
	SuccessCount int

	// SkippedCount is the number skipped (e.g., duplicates).
	SkippedCount int

	// ErrorCount is the number that failed.
	ErrorCount int

	// Errors contains up to the first N errors encountered.
	Errors []error
}

// IngestResult represents the outcome of an ingestion operation.
type IngestResult struct {
	// Stats contains ingestion statistics.
	Stats IngestStats

	// Error is set if the ingestion failed fatally.
	Error error
}
