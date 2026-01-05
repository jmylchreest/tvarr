package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

// runStreamTests runs proxy mode stream tests (direct, smart, relay).
func (r *E2ERunner) runStreamTests(ctx context.Context) {
	// Create Smart Mode Proxy
	r.runTestWithInfo("Create Proxy (Smart Mode)",
		"POST /api/v1/proxies with proxy_mode=smart (HLS passthrough/repackage)",
		func() error {
			var err error
			r.SmartModeProxyID, err = r.client.CreateStreamProxy(ctx, CreateStreamProxyOptions{
				Name:            fmt.Sprintf("E2E Smart Mode Proxy %s", r.runID),
				StreamSourceIDs: []string{r.StreamSourceID},
				ProxyMode:       "smart",
			})
			if err != nil {
				return err
			}
			r.log("  Created smart mode proxy: %s", r.SmartModeProxyID)
			return nil
		})

	// Create Smart Mode + Relay Profile Proxy (only if ffmpeg is available)
	if r.ffmpegAvailable {
		r.runTestWithInfo("Create Proxy (Smart Mode with Relay Profile)",
			"POST /api/v1/proxies with proxy_mode=smart + encoding_profile_id (enables transcoding)",
			func() error {
				relayProfileID, err := r.client.GetFirstRelayProfileID(ctx)
				if err != nil {
					return fmt.Errorf("failed to get relay profile: %w", err)
				}
				if relayProfileID == "" {
					return fmt.Errorf("no relay profiles available")
				}
				r.log("  Using relay profile: %s", relayProfileID)

				r.RelayProfileProxyID, err = r.client.CreateStreamProxy(ctx, CreateStreamProxyOptions{
					Name:            fmt.Sprintf("E2E Smart+Relay Proxy %s", r.runID),
					StreamSourceIDs: []string{r.StreamSourceID},
					ProxyMode:       "smart",
					RelayProfileID:  relayProfileID,
				})
				if err != nil {
					return err
				}
				r.log("  Created smart mode proxy with relay profile: %s", r.RelayProfileProxyID)
				return nil
			})
	} else {
		r.runTestWithInfo("Skip Relay Profile Tests (No FFmpeg)",
			"FFmpeg not in PATH - skipping transcoding tests",
			func() error {
				r.log("  SKIPPED: Relay profile tests require ffmpeg")
				fmt.Println("WARNING: Skipping relay profile proxy test - ffmpeg not available")
				fmt.Fprintln(os.Stderr, "WARNING: Skipping relay profile proxy test - ffmpeg not available")
				return nil
			})
	}

	// Test Direct Mode Stream
	r.runTestWithInfo("Test Stream (Direct Mode)",
		"GET /proxy/{id}/{channelId} - verify direct mode returns redirect to source URL",
		func() error {
			if r.ProxyID == "" {
				return fmt.Errorf("direct mode proxy was not created")
			}

			channelID, err := r.client.GetFirstChannelID(ctx, r.StreamSourceID)
			if err != nil {
				return fmt.Errorf("get channel ID: %w", err)
			}

			streamURL := r.client.GetProxyStreamURL(r.ProxyID, channelID)
			r.log("  Testing stream URL: %s", streamURL)

			result, err := r.client.TestStreamRequest(ctx, streamURL)
			if err != nil {
				return fmt.Errorf("stream request: %w", err)
			}

			if result.StatusCode != http.StatusFound {
				return fmt.Errorf("expected HTTP 302, got %d", result.StatusCode)
			}
			if result.Location == "" {
				return fmt.Errorf("expected Location header in redirect response")
			}
			r.log("  Direct mode: HTTP %d -> %s", result.StatusCode, result.Location)
			return nil
		})

	// Test Smart Mode Stream
	r.runTestWithInfo("Test Stream (Smart Mode)",
		"GET /proxy/{id}/{channelId} with X-Container: mpegts - verify smart mode returns HTTP 200 with CORS + TS bytes",
		func() error {
			if r.SmartModeProxyID == "" {
				return fmt.Errorf("smart mode proxy was not created")
			}

			// Trigger proxy generation
			if err := r.client.TriggerProxyGeneration(ctx, r.SmartModeProxyID, time.Minute); err != nil {
				return fmt.Errorf("trigger proxy generation: %w", err)
			}

			channelID, err := r.client.GetFirstChannelID(ctx, r.StreamSourceID)
			if err != nil {
				return fmt.Errorf("get channel ID: %w", err)
			}

			// Use X-Container header to request MPEG-TS format via client detection rules.
			// Smart mode now serves content based on source format (HLS sources serve HLS playlists by default).
			// The X-Container header triggers the dynamic client detection rule that extracts format preference.
			streamURL := r.client.GetProxyStreamURL(r.SmartModeProxyID, channelID)
			r.log("  Testing stream URL: %s (with X-Container: mpegts)", streamURL)

			result, err := r.client.TestStreamRequestWithHeaders(ctx, streamURL, map[string]string{
				"X-Container": "mpegts",
			})
			if err != nil {
				return fmt.Errorf("stream request: %w", err)
			}

			if result.StatusCode != http.StatusOK {
				return fmt.Errorf("expected HTTP 200, got %d", result.StatusCode)
			}
			if !result.HasCORSHeaders {
				return fmt.Errorf("expected CORS headers in response")
			}
			if result.BytesReceived == 0 {
				return fmt.Errorf("expected to receive bytes from smart mode stream")
			}
			if !result.TSSyncByteValid {
				return fmt.Errorf("expected first byte to be TS sync byte (0x47)")
			}
			r.log("  Smart mode: HTTP %d, CORS=%v, Bytes=%d, TS sync valid",
				result.StatusCode, result.HasCORSHeaders, result.BytesReceived)
			return nil
		})

	// Test Relay Profile Stream (only if ffmpeg is available)
	if r.ffmpegAvailable && r.RelayProfileProxyID != "" {
		r.runTestWithInfo("Test Stream (Smart Mode with Relay Profile)",
			"GET /proxy/{id}/{channelId} with X-Container: mpegts - verify relay transcoding works",
			func() error {
				// Trigger proxy generation
				if err := r.client.TriggerProxyGeneration(ctx, r.RelayProfileProxyID, time.Minute); err != nil {
					return fmt.Errorf("trigger proxy generation: %w", err)
				}

				channelID, err := r.client.GetFirstChannelID(ctx, r.StreamSourceID)
				if err != nil {
					return fmt.Errorf("get channel ID: %w", err)
				}

				// Use X-Container header to request MPEG-TS format via client detection rules
				streamURL := r.client.GetProxyStreamURL(r.RelayProfileProxyID, channelID)
				r.log("  Testing stream URL: %s (with X-Container: mpegts)", streamURL)

				result, err := r.client.TestStreamRequestWithHeaders(ctx, streamURL, map[string]string{
					"X-Container": "mpegts",
				})
				if err != nil {
					return fmt.Errorf("stream request: %w", err)
				}

				if result.StatusCode != http.StatusOK {
					return fmt.Errorf("expected HTTP 200, got %d", result.StatusCode)
				}
				if !result.HasCORSHeaders {
					return fmt.Errorf("expected CORS headers in response")
				}
				if result.BytesReceived == 0 {
					return fmt.Errorf("expected to receive bytes from relay stream")
				}
				r.log("  Relay mode: HTTP %d, CORS=%v, Bytes=%d",
					result.StatusCode, result.HasCORSHeaders, result.BytesReceived)
				return nil
			})
	}
}
