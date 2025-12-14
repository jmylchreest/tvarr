//nolint:errcheck,gocognit,gocyclo,nestif,gocritic,godot,wrapcheck,gosec,revive,goprintffuncname,modernize // E2E test runner uses relaxed linting
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// APIClient wraps HTTP calls to the tvarr API.
type APIClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewAPIClient creates a new API client.
func NewAPIClient(baseURL string) *APIClient {
	return &APIClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // Long timeout for proxy generation with logo caching
		},
	}
}

// HealthCheck verifies the API is accessible.
func (c *APIClient) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}
	return nil
}

// CreateStreamSource creates a new M3U stream source.
func (c *APIClient) CreateStreamSource(ctx context.Context, name, url string) (string, error) {
	body := map[string]string{
		"name": name,
		"type": "m3u",
		"url":  url,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/sources/stream", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create stream source failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	id, ok := result["id"].(string)
	if !ok {
		return "", fmt.Errorf("response missing id field")
	}
	return id, nil
}

// CreateEPGSource creates a new XMLTV EPG source.
func (c *APIClient) CreateEPGSource(ctx context.Context, name, url string) (string, error) {
	body := map[string]string{
		"name": name,
		"type": "xmltv",
		"url":  url,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/sources/epg", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create EPG source failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	id, ok := result["id"].(string)
	if !ok {
		return "", fmt.Errorf("response missing id field")
	}
	return id, nil
}

// TriggerIngestion triggers ingestion for a source and waits for completion.
func (c *APIClient) TriggerIngestion(ctx context.Context, sourceType, sourceID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build ingestion URL
	var ingestURL string
	if sourceType == "stream" {
		ingestURL = fmt.Sprintf("%s/api/v1/sources/stream/%s/ingest", c.baseURL, sourceID)
	} else {
		ingestURL = fmt.Sprintf("%s/api/v1/sources/epg/%s/ingest", c.baseURL, sourceID)
	}

	// Start SSE listener BEFORE triggering ingestion to avoid race condition
	// where the ingestion completes before the listener connects
	sseReady := make(chan struct{})
	progressDone := make(chan error, 1)
	go func() {
		progressDone <- c.waitForSSECompletionWithReady(ctx, sourceID, sseReady)
	}()

	// Wait for SSE listener to be ready
	select {
	case <-sseReady:
		// SSE listener is connected and ready
	case <-ctx.Done():
		return fmt.Errorf("SSE listener setup timed out: %w", ctx.Err())
	case err := <-progressDone:
		// SSE listener failed before becoming ready
		return fmt.Errorf("SSE listener failed during setup: %w", err)
	}

	// Now trigger the ingestion
	req, err := http.NewRequestWithContext(ctx, "POST", ingestURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("trigger ingestion failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("trigger ingestion failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// Wait for completion via SSE
	select {
	case err := <-progressDone:
		return err
	case <-ctx.Done():
		return fmt.Errorf("ingestion timed out after %v", timeout)
	}
}

// waitForSSECompletionWithReady monitors SSE events for completion.
// If ready is non-nil, it signals when the SSE connection is established and ready to receive events.
func (c *APIClient) waitForSSECompletionWithReady(ctx context.Context, ownerID string, ready chan<- struct{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/progress/events", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Signal that SSE connection is ready before reading events
	if ready != nil {
		close(ready)
	}

	// Use a channel-based approach to make scanner.Scan() interruptible by context
	type scanResult struct {
		line string
		ok   bool
		err  error
	}
	lineCh := make(chan scanResult, 1)

	scanner := bufio.NewScanner(resp.Body)

	// Read lines in a goroutine so we can select on context cancellation
	go func() {
		for {
			ok := scanner.Scan()
			if !ok {
				lineCh <- scanResult{err: scanner.Err()}
				return
			}
			select {
			case lineCh <- scanResult{line: scanner.Text(), ok: true}:
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case result := <-lineCh:
			if !result.ok {
				return result.err
			}
			line := result.line
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				var event map[string]interface{}
				if err := json.Unmarshal([]byte(data), &event); err != nil {
					continue
				}

				// Check if this event is for our source
				eventOwnerID, _ := event["owner_id"].(string)
				if eventOwnerID != ownerID {
					continue
				}

				// Check for completion or error
				state, _ := event["state"].(string)
				switch state {
				case "completed":
					return nil
				case "error":
					errMsg, _ := event["error"].(string)
					return fmt.Errorf("ingestion failed: %s", errMsg)
				}
			}
		}
	}
}

// GetChannelCount returns the number of channels in the system.
func (c *APIClient) GetChannelCount(ctx context.Context) (int, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/channels?limit=1", nil)
	if err != nil {
		return 0, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	total, ok := result["total"].(float64)
	if !ok {
		// Try to count items array
		items, ok := result["items"].([]interface{})
		if ok {
			return len(items), nil
		}
		channels, ok := result["channels"].([]interface{})
		if ok {
			return len(channels), nil
		}
		return 0, nil
	}
	return int(total), nil
}

// CreateStreamProxy creates a new stream proxy.
func (c *APIClient) CreateStreamProxy(ctx context.Context, opts CreateStreamProxyOptions) (string, error) {
	body := map[string]interface{}{
		"name":                opts.Name,
		"source_ids":          opts.StreamSourceIDs,
		"epg_source_ids":      opts.EpgSourceIDs,
		"cache_channel_logos": opts.CacheChannelLogos,
		"cache_program_logos": opts.CacheProgramLogos,
	}
	// Add proxy_mode if specified (valid values: "direct", "smart")
	if opts.ProxyMode != "" {
		body["proxy_mode"] = opts.ProxyMode
	}
	// Add encoding_profile_id if specified (enables transcoding with "smart" mode)
	if opts.RelayProfileID != "" {
		body["encoding_profile_id"] = opts.RelayProfileID
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/proxies", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create proxy failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	id, ok := result["id"].(string)
	if !ok {
		return "", fmt.Errorf("response missing id field")
	}
	return id, nil
}

// TriggerProxyGeneration triggers proxy generation and waits for completion.
// Note: The generate endpoint is async - it starts generation in a goroutine.
// We poll the proxy status until generation completes (success/failed).
func (c *APIClient) TriggerProxyGeneration(ctx context.Context, proxyID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Trigger regeneration - this is async
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/proxies/"+proxyID+"/regenerate", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("trigger generation failed: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("trigger generation failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	// Poll for completion - generation runs asynchronously
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("generation timeout: %w", ctx.Err())
		case <-ticker.C:
			proxy, err := c.GetProxy(ctx, proxyID)
			if err != nil {
				return fmt.Errorf("failed to check generation status: %w", err)
			}
			status, _ := proxy["status"].(string)
			switch status {
			case "success":
				return nil
			case "failed":
				lastError, _ := proxy["last_error"].(string)
				return fmt.Errorf("generation failed: %s", lastError)
			case "generating", "pending":
				// Still running, continue polling
				continue
			default:
				return fmt.Errorf("unknown proxy status: %s", status)
			}
		}
	}
}

// GetProxy fetches a proxy by ID.
func (c *APIClient) GetProxy(ctx context.Context, proxyID string) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/proxies/"+proxyID, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get proxy failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetProxyM3U fetches the M3U output for a proxy.
func (c *APIClient) GetProxyM3U(ctx context.Context, proxyID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/proxy/"+proxyID+".m3u", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch M3U failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("fetch M3U failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read M3U body failed: %w", err)
	}

	return string(data), nil
}

// GetProxyXMLTV fetches the XMLTV output for a proxy.
func (c *APIClient) GetProxyXMLTV(ctx context.Context, proxyID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/proxy/"+proxyID+".xmltv", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch XMLTV failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("fetch XMLTV failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read XMLTV body failed: %w", err)
	}

	return string(data), nil
}

// GetFirstChannelID gets the first channel ID for a given source ID.
func (c *APIClient) GetFirstChannelID(ctx context.Context, sourceID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		c.baseURL+"/api/v1/channels?source_id="+sourceID+"&limit=1", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("get channels failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("get channels failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode channels response: %w", err)
	}

	if len(result.Items) == 0 {
		return "", fmt.Errorf("no channels found for source %s", sourceID)
	}

	return result.Items[0].ID, nil
}

// GetProxyStreamURL constructs the URL to stream a channel through a proxy.
func (c *APIClient) GetProxyStreamURL(proxyID, channelID string) string {
	return fmt.Sprintf("%s/proxy/%s/%s", c.baseURL, proxyID, channelID)
}

// TestStreamRequest tests a stream URL and returns information about the response.
// It does not follow redirects automatically.
func (c *APIClient) TestStreamRequest(ctx context.Context, streamURL string) (*StreamTestResult, error) {
	// Create client that doesn't follow redirects
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", streamURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stream request failed: %w", err)
	}
	defer resp.Body.Close()

	result := &StreamTestResult{
		StatusCode:  resp.StatusCode,
		Headers:     resp.Header,
		ContentType: resp.Header.Get("Content-Type"),
	}

	// Check for redirect location
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently {
		result.Location = resp.Header.Get("Location")
	}

	// Check for CORS headers
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		result.HasCORSHeaders = true
	}

	// For 200 responses, read a bit of content to verify it's valid
	if resp.StatusCode == http.StatusOK {
		buf := make([]byte, 188*2) // Read enough for 2 TS packets
		n, _ := io.ReadAtLeast(resp.Body, buf, 1)
		result.BytesReceived = n

		// Check TS sync byte
		if n > 0 && buf[0] == 0x47 {
			result.TSSyncByteValid = true
		}
	}

	return result, nil
}

// UploadLogo uploads a logo file and returns the ID and URL to access it.
func (c *APIClient) UploadLogo(ctx context.Context, name string, fileData []byte) (*UploadLogoResult, error) {
	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add the file field
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		return nil, fmt.Errorf("create form file failed: %w", err)
	}
	if _, err := part.Write(fileData); err != nil {
		return nil, fmt.Errorf("write file data failed: %w", err)
	}

	// Add the name field
	if err := writer.WriteField("name", name); err != nil {
		return nil, fmt.Errorf("write name field failed: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close writer failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/logos/upload", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload logo failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upload logo failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	id, ok := result["id"].(string)
	if !ok {
		return nil, fmt.Errorf("response missing id field")
	}
	url, ok := result["url"].(string)
	if !ok {
		return nil, fmt.Errorf("response missing url field")
	}
	return &UploadLogoResult{ID: id, URL: url}, nil
}

// CreateDataMappingRule creates a new data mapping rule.
func (c *APIClient) CreateDataMappingRule(ctx context.Context, name, sourceType, expression string, priority int) (string, error) {
	isEnabled := true
	body := map[string]interface{}{
		"name":        name,
		"source_type": sourceType,
		"expression":  expression,
		"priority":    priority,
		"is_enabled":  isEnabled,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/data-mapping", bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("create data mapping rule failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create data mapping rule failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response failed: %w", err)
	}

	id, ok := result["id"].(string)
	if !ok {
		return "", fmt.Errorf("response missing id field")
	}
	return id, nil
}

// GetEncodingProfiles fetches all encoding profiles (formerly relay profiles).
func (c *APIClient) GetEncodingProfiles(ctx context.Context) ([]map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/encoding-profiles", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch encoding profiles failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch encoding profiles failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	profiles, ok := result["profiles"].([]interface{})
	if !ok {
		return nil, nil // No profiles
	}

	var profileMaps []map[string]interface{}
	for _, p := range profiles {
		if pm, ok := p.(map[string]interface{}); ok {
			profileMaps = append(profileMaps, pm)
		}
	}
	return profileMaps, nil
}

// GetRelayProfiles is deprecated - use GetEncodingProfiles instead.
func (c *APIClient) GetRelayProfiles(ctx context.Context) ([]map[string]interface{}, error) {
	return c.GetEncodingProfiles(ctx)
}

// GetFirstRelayProfileID returns the ID of the first relay profile, or empty string if none.
func (c *APIClient) GetFirstRelayProfileID(ctx context.Context) (string, error) {
	profiles, err := c.GetRelayProfiles(ctx)
	if err != nil {
		return "", err
	}
	if len(profiles) == 0 {
		return "", nil
	}
	id, ok := profiles[0]["id"].(string)
	if !ok {
		return "", nil
	}
	return id, nil
}

// GetClientDetectionMappings fetches all client detection rules.
func (c *APIClient) GetClientDetectionMappings(ctx context.Context) ([]ClientDetectionMapping, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/client-detection-rules", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch client detection rules failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch client detection rules failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Rules []ClientDetectionMapping `json:"rules"`
		Count int                      `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	return result.Rules, nil
}

// GetClientDetectionStats computes client detection statistics from the rules list.
func (c *APIClient) GetClientDetectionStats(ctx context.Context) (*ClientDetectionStats, error) {
	rules, err := c.GetClientDetectionMappings(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch client detection rules for stats failed: %w", err)
	}

	stats := &ClientDetectionStats{
		Total: len(rules),
	}

	for _, rule := range rules {
		if rule.IsEnabled {
			stats.Enabled++
		}
		if rule.IsSystem {
			stats.System++
		} else {
			stats.Custom++
		}
	}

	return stats, nil
}

// TestClientDetectionExpression tests an expression against sample data (legacy API).
func (c *APIClient) TestClientDetectionExpression(ctx context.Context, expression string, testData map[string]string) (bool, error) {
	// Extract user_agent from testData for the new API
	userAgent := testData["user_agent"]
	return c.TestClientDetectionExpressionWithHeaders(ctx, expression, userAgent, nil)
}

// TestClientDetectionExpressionWithHeaders tests an expression against a User-Agent and optional headers.
func (c *APIClient) TestClientDetectionExpressionWithHeaders(ctx context.Context, expression, userAgent string, headers map[string]string) (bool, error) {
	body := map[string]interface{}{
		"expression": expression,
		"user_agent": userAgent,
	}
	if len(headers) > 0 {
		body["headers"] = headers
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/client-detection-rules/test", bytes.NewReader(jsonBody))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("test expression failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("test expression failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Matches bool   `json:"matches"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("decode response failed: %w", err)
	}

	if result.Error != "" {
		return false, fmt.Errorf("expression error: %s", result.Error)
	}

	return result.Matches, nil
}

// GetThemes fetches all available themes.
func (c *APIClient) GetThemes(ctx context.Context) (*ThemeListResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/themes", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch themes failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch themes failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result ThemeListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	return &result, nil
}

// GetThemeCSS fetches the CSS content for a theme.
func (c *APIClient) GetThemeCSS(ctx context.Context, themeID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/themes/"+themeID+".css", nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch theme CSS failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("fetch theme CSS failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read theme CSS failed: %w", err)
	}

	return string(content), nil
}
