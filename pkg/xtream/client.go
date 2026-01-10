package xtream

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jmylchreest/tvarr/internal/version"
)

// Default configuration values.
const (
	DefaultTimeout = 2 * time.Minute

	// API endpoint paths.
	pathPlayerAPI = "/player_api.php"
	pathXMLTV     = "/xmltv.php"
	pathGetM3U    = "/get.php"
	pathLive      = "/live"
	pathMovie     = "/movie"
	pathSeries    = "/series"
	pathTimeshift = "/timeshift"

	// API actions.
	actionGetLiveCategories   = "get_live_categories"
	actionGetVODCategories    = "get_vod_categories"
	actionGetSeriesCategories = "get_series_categories"
	actionGetLiveStreams      = "get_live_streams"
	actionGetVODStreams       = "get_vod_streams"
	actionGetVODInfo          = "get_vod_info"
	actionGetSeries           = "get_series"
	actionGetSeriesInfo       = "get_series_info"
	actionGetShortEPG         = "get_short_epg"
	actionGetSimpleDataTable  = "get_simple_data_table"

	// Query parameter names.
	paramUsername   = "username"
	paramPassword   = "password"
	paramAction     = "action"
	paramCategoryID = "category_id"
	paramVODID      = "vod_id"
	paramSeriesID   = "series_id"
	paramStreamID   = "stream_id"
	paramLimit      = "limit"
	paramType       = "type"
	paramOutput     = "output"

	// Default values.
	defaultExtensionTS   = "ts"
	defaultExtensionMP4  = "mp4"
	defaultExtensionMKV  = "mkv"
	defaultPlaylistType  = "m3u_plus"
	defaultOutputFormat  = "ts"
	maxErrorBodyReadSize = 1024
)

// HTTP header constants.
const (
	headerUserAgent = "User-Agent"
)

// Client is an Xtream Codes API client.
type Client struct {
	// BaseURL is the server base URL (e.g., "http://example.com:8080").
	BaseURL string

	// Username is the API username.
	Username string

	// Password is the API password.
	Password string

	// HTTPClient is the standard HTTP client used for requests.
	// If nil, a default client with DefaultTimeout is used.
	HTTPClient *http.Client

	// UserAgent is the User-Agent header sent with requests.
	UserAgent string
}

// ClientOption is a function that configures a Client.
type ClientOption func(*Client)

// NewClient creates a new Xtream Codes API client.
// It accepts the standard *http.Client, allowing injection of any HTTP client
// implementation (standard, with middleware, resilient wrapper, etc.).
func NewClient(baseURL, username, password string, opts ...ClientOption) *Client {
	c := &Client{
		BaseURL:  strings.TrimSuffix(baseURL, "/"),
		Username: username,
		Password: password,
		HTTPClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		UserAgent: version.UserAgent(),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// WithHTTPClient sets a custom standard library HTTP client.
// This allows injection of any *http.Client, including ones wrapped
// with retry logic, circuit breakers, or other middleware.
func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *Client) {
		c.HTTPClient = client
	}
}

// WithUserAgent sets a custom User-Agent header.
func WithUserAgent(ua string) ClientOption {
	return func(c *Client) {
		c.UserAgent = ua
	}
}

// WithTimeout sets the HTTP client timeout.
// This creates a new HTTP client with the specified timeout.
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.HTTPClient = &http.Client{
			Timeout: timeout,
		}
	}
}

// apiURL builds the player_api.php URL with the given action and parameters.
func (c *Client) apiURL(action string, params map[string]string) string {
	var u strings.Builder
	u.WriteString(fmt.Sprintf("%s%s?%s=%s&%s=%s",
		c.BaseURL,
		pathPlayerAPI,
		paramUsername, url.QueryEscape(c.Username),
		paramPassword, url.QueryEscape(c.Password)))

	if action != "" {
		u.WriteString("&" + paramAction + "=" + url.QueryEscape(action))
	}

	for k, v := range params {
		u.WriteString("&" + url.QueryEscape(k) + "=" + url.QueryEscape(v))
	}

	return u.String()
}

// doRequest performs an HTTP GET request and decodes the JSON response.
func (c *Client) doRequest(ctx context.Context, requestURL string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	if c.UserAgent != "" {
		req.Header.Set(headerUserAgent, c.UserAgent)
	}

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyReadSize))
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	return nil
}

// GetAuthInfo retrieves authentication and server information.
// This is typically the first call to verify credentials.
func (c *Client) GetAuthInfo(ctx context.Context) (*AuthInfo, error) {
	var info AuthInfo
	if err := c.doRequest(ctx, c.apiURL("", nil), &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// GetLiveCategories retrieves all live stream categories.
func (c *Client) GetLiveCategories(ctx context.Context) ([]Category, error) {
	var categories []Category
	if err := c.doRequest(ctx, c.apiURL(actionGetLiveCategories, nil), &categories); err != nil {
		return nil, err
	}
	return categories, nil
}

// GetVODCategories retrieves all video on demand categories.
func (c *Client) GetVODCategories(ctx context.Context) ([]Category, error) {
	var categories []Category
	if err := c.doRequest(ctx, c.apiURL(actionGetVODCategories, nil), &categories); err != nil {
		return nil, err
	}
	return categories, nil
}

// GetSeriesCategories retrieves all series categories.
func (c *Client) GetSeriesCategories(ctx context.Context) ([]Category, error) {
	var categories []Category
	if err := c.doRequest(ctx, c.apiURL(actionGetSeriesCategories, nil), &categories); err != nil {
		return nil, err
	}
	return categories, nil
}

// StreamsOptions contains options for listing streams.
type StreamsOptions struct {
	// CategoryID filters streams by category. Empty means all categories.
	CategoryID string
}

// GetLiveStreams retrieves live streams, optionally filtered by category.
func (c *Client) GetLiveStreams(ctx context.Context, opts *StreamsOptions) ([]Stream, error) {
	params := make(map[string]string)
	if opts != nil && opts.CategoryID != "" {
		params[paramCategoryID] = opts.CategoryID
	}

	var streams []Stream
	if err := c.doRequest(ctx, c.apiURL(actionGetLiveStreams, params), &streams); err != nil {
		return nil, err
	}
	return streams, nil
}

// GetVODStreams retrieves VOD content, optionally filtered by category.
func (c *Client) GetVODStreams(ctx context.Context, opts *StreamsOptions) ([]VODStream, error) {
	params := make(map[string]string)
	if opts != nil && opts.CategoryID != "" {
		params[paramCategoryID] = opts.CategoryID
	}

	var streams []VODStream
	if err := c.doRequest(ctx, c.apiURL(actionGetVODStreams, params), &streams); err != nil {
		return nil, err
	}
	return streams, nil
}

// GetVODInfo retrieves detailed information about a VOD item.
func (c *Client) GetVODInfo(ctx context.Context, vodID int) (*VODInfo, error) {
	params := map[string]string{paramVODID: fmt.Sprintf("%d", vodID)}

	var info VODInfo
	if err := c.doRequest(ctx, c.apiURL(actionGetVODInfo, params), &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// GetSeries retrieves all series, optionally filtered by category.
func (c *Client) GetSeries(ctx context.Context, opts *StreamsOptions) ([]Series, error) {
	params := make(map[string]string)
	if opts != nil && opts.CategoryID != "" {
		params[paramCategoryID] = opts.CategoryID
	}

	var series []Series
	if err := c.doRequest(ctx, c.apiURL(actionGetSeries, params), &series); err != nil {
		return nil, err
	}
	return series, nil
}

// GetSeriesInfo retrieves detailed information about a series including episodes.
func (c *Client) GetSeriesInfo(ctx context.Context, seriesID int) (*SeriesInfo, error) {
	params := map[string]string{paramSeriesID: fmt.Sprintf("%d", seriesID)}

	var info SeriesInfo
	if err := c.doRequest(ctx, c.apiURL(actionGetSeriesInfo, params), &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// GetShortEPG retrieves a short EPG listing for a stream.
// The limit parameter controls how many entries to return (0 = server default).
func (c *Client) GetShortEPG(ctx context.Context, streamID int, limit int) ([]EPGListing, error) {
	params := map[string]string{paramStreamID: fmt.Sprintf("%d", streamID)}
	if limit > 0 {
		params[paramLimit] = fmt.Sprintf("%d", limit)
	}

	var response EPGResponse
	if err := c.doRequest(ctx, c.apiURL(actionGetShortEPG, params), &response); err != nil {
		return nil, err
	}
	return response.EPGListings, nil
}

// GetFullEPG retrieves the full EPG data for a stream.
func (c *Client) GetFullEPG(ctx context.Context, streamID int) ([]EPGListing, error) {
	params := map[string]string{paramStreamID: fmt.Sprintf("%d", streamID)}

	var response EPGResponse
	if err := c.doRequest(ctx, c.apiURL(actionGetSimpleDataTable, params), &response); err != nil {
		return nil, err
	}
	return response.EPGListings, nil
}

// GetXMLTVURL returns the URL for the full XMLTV EPG file.
func (c *Client) GetXMLTVURL() string {
	return fmt.Sprintf("%s%s?%s=%s&%s=%s",
		c.BaseURL,
		pathXMLTV,
		paramUsername, url.QueryEscape(c.Username),
		paramPassword, url.QueryEscape(c.Password))
}

// GetXMLTV retrieves the full XMLTV EPG data as raw bytes.
// Note: This can be a very large file.
func (c *Client) GetXMLTV(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.GetXMLTVURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if c.UserAgent != "" {
		req.Header.Set(headerUserAgent, c.UserAgent)
	}

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// GetXMLTVReader retrieves the full XMLTV EPG data as a streaming reader.
// The caller is responsible for closing the returned ReadCloser.
// Note: This can be a very large file and should be processed in streaming fashion.
func (c *Client) GetXMLTVReader(ctx context.Context) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.GetXMLTVURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if c.UserAgent != "" {
		req.Header.Set(headerUserAgent, c.UserAgent)
	}

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// GetLiveStreamURL returns the URL for a live stream.
// Common extensions: ts, m3u8
func (c *Client) GetLiveStreamURL(streamID int, extension string) string {
	if extension == "" {
		extension = defaultExtensionTS
	}
	return fmt.Sprintf("%s%s/%s/%s/%d.%s",
		c.BaseURL, pathLive, c.Username, c.Password, streamID, extension)
}

// GetVODStreamURL returns the URL for a VOD stream.
// The extension should match the container_extension from the VOD info.
func (c *Client) GetVODStreamURL(vodID int, extension string) string {
	if extension == "" {
		extension = defaultExtensionMP4
	}
	return fmt.Sprintf("%s%s/%s/%s/%d.%s",
		c.BaseURL, pathMovie, c.Username, c.Password, vodID, extension)
}

// GetSeriesStreamURL returns the URL for a series episode stream.
// The extension should match the container_extension from the episode info.
func (c *Client) GetSeriesStreamURL(episodeID int, extension string) string {
	if extension == "" {
		extension = defaultExtensionMKV
	}
	return fmt.Sprintf("%s%s/%s/%s/%d.%s",
		c.BaseURL, pathSeries, c.Username, c.Password, episodeID, extension)
}

// GetM3UPlaylistURL returns the URL for a full M3U playlist.
// Type can be "m3u" or "m3u_plus" for extended format.
func (c *Client) GetM3UPlaylistURL(playlistType string) string {
	if playlistType == "" {
		playlistType = defaultPlaylistType
	}
	return fmt.Sprintf("%s%s?%s=%s&%s=%s&%s=%s&%s=%s",
		c.BaseURL,
		pathGetM3U,
		paramUsername, url.QueryEscape(c.Username),
		paramPassword, url.QueryEscape(c.Password),
		paramType, url.QueryEscape(playlistType),
		paramOutput, defaultOutputFormat)
}

// GetTimeshiftURL returns the URL for timeshift/catchup content.
// startTime should be in "YYYY-MM-DD:HH-MM" format.
// duration is in minutes.
func (c *Client) GetTimeshiftURL(streamID int, startTime string, duration int) string {
	return fmt.Sprintf("%s%s/%s/%s/%d/%s/%d.%s",
		c.BaseURL, pathTimeshift, c.Username, c.Password, duration, startTime, streamID, defaultExtensionTS)
}
