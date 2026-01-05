package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// runProxyTests runs proxy creation and generation tests.
func (r *E2ERunner) runProxyTests(ctx context.Context) {
	// Create Stream Proxy
	r.runTestWithInfo("Create Stream Proxy",
		"POST /api/v1/proxies - create proxy with stream/EPG sources attached",
		func() error {
			var err error
			sourceIDs := []string{}
			epgIDs := []string{}
			if r.StreamSourceID != "" {
				sourceIDs = []string{r.StreamSourceID}
			}
			if r.EpgSourceID != "" {
				epgIDs = []string{r.EpgSourceID}
			}

			opts := CreateStreamProxyOptions{
				StreamSourceIDs:   sourceIDs,
				EpgSourceIDs:      epgIDs,
				CacheChannelLogos: r.cacheChannelLogos,
				CacheProgramLogos: r.cacheProgramLogos,
			}

			if r.testProxyModes {
				// Create direct mode proxy with explicit mode for proxy mode testing
				opts.Name = fmt.Sprintf("E2E Direct Mode Proxy %s", r.runID)
				opts.ProxyMode = "direct"
			} else {
				// Create default proxy (defaults to direct mode)
				opts.Name = fmt.Sprintf("E2E Test Proxy %s", r.runID)
			}

			r.ProxyID, err = r.client.CreateStreamProxy(ctx, opts)
			if err != nil {
				return err
			}
			r.log("  Created proxy: %s (mode=%s, logo caching: channel=%v, program=%v)",
				r.ProxyID, opts.ProxyMode, r.cacheChannelLogos, r.cacheProgramLogos)
			return nil
		})

	// Generate Proxy Output
	r.runTestWithInfo("Generate Proxy Output",
		"POST /api/v1/proxies/{id}/generate + SSE /api/v1/progress/events until completed",
		func() error {
			if r.ProxyID == "" {
				return fmt.Errorf("no proxy to generate")
			}
			return r.client.TriggerProxyGeneration(ctx, r.ProxyID, 5*time.Minute)
		})

	// Verify Proxy Status
	r.runTestWithInfo("Verify Proxy Status",
		"GET /api/v1/proxies/{id} - verify status and channel_count > 0",
		func() error {
			if r.ProxyID == "" {
				return fmt.Errorf("no proxy to verify")
			}
			proxy, err := r.client.GetProxy(ctx, r.ProxyID)
			if err != nil {
				return err
			}
			status, _ := proxy["status"].(string)
			if status == "" {
				return fmt.Errorf("proxy status not found in response")
			}
			r.log("  Proxy status: %s", status)
			channelCount, _ := proxy["channel_count"].(float64)
			r.log("  Channel count: %.0f", channelCount)
			if channelCount == 0 {
				return fmt.Errorf("proxy has no channels after generation")
			}
			return nil
		})
}

// runOutputValidationTests runs M3U/XMLTV output validation tests.
func (r *E2ERunner) runOutputValidationTests(ctx context.Context) {
	// Fetch and Validate M3U Output
	r.runTestWithInfo("Fetch and Validate M3U Output",
		"GET /proxy/{id}/playlist.m3u - validate #EXTM3U header and #EXTINF entries",
		func() error {
			if r.ProxyID == "" {
				return fmt.Errorf("no proxy to fetch M3U from")
			}
			var err error
			r.M3UContent, err = r.client.GetProxyM3U(ctx, r.ProxyID)
			if err != nil {
				return err
			}
			r.log("  M3U size: %d bytes", len(r.M3UContent))

			channelCount, err := ValidateM3U(r.M3UContent)
			if err != nil {
				return err
			}
			r.log("  M3U channels: %d", channelCount)

			// Validate expected channel count if specified
			if r.expectedChannels > 0 && channelCount != r.expectedChannels {
				return fmt.Errorf("channel count mismatch: got %d, expected %d", channelCount, r.expectedChannels)
			}

			// Write M3U to output dir if specified
			if r.outputDir != "" {
				if err := r.writeArtifact(r.ProxyID+".m3u", r.M3UContent); err != nil {
					r.log("  Warning: failed to write M3U: %v", err)
				} else {
					r.log("  Wrote M3U artifact: %s/%s.m3u", r.outputDir, r.ProxyID)
				}
			}

			// Display sample channels if requested
			if r.showSamples {
				r.printSampleChannels(r.M3UContent)
			}

			return nil
		})

	// Fetch and Validate XMLTV Output
	r.runTestWithInfo("Fetch and Validate XMLTV Output",
		"GET /proxy/{id}/epg.xml - validate <tv>, <channel>, <programme> elements",
		func() error {
			if r.ProxyID == "" {
				return fmt.Errorf("no proxy to fetch XMLTV from")
			}
			var err error
			r.XMLTVContent, err = r.client.GetProxyXMLTV(ctx, r.ProxyID)
			if err != nil {
				return err
			}
			r.log("  XMLTV size: %d bytes", len(r.XMLTVContent))

			channelCount, programCount, err := ValidateXMLTV(r.XMLTVContent)
			if err != nil {
				return err
			}
			r.log("  XMLTV channels: %d, programs: %d", channelCount, programCount)

			// Validate expected counts if specified
			if r.expectedChannels > 0 && channelCount != r.expectedChannels {
				return fmt.Errorf("XMLTV channel count mismatch: got %d, expected %d", channelCount, r.expectedChannels)
			}
			if r.expectedPrograms > 0 && programCount != r.expectedPrograms {
				return fmt.Errorf("XMLTV program count mismatch: got %d, expected %d", programCount, r.expectedPrograms)
			}

			// Write XMLTV to output dir if specified
			if r.outputDir != "" {
				if err := r.writeArtifact(r.ProxyID+".xmltv", r.XMLTVContent); err != nil {
					r.log("  Warning: failed to write XMLTV: %v", err)
				} else {
					r.log("  Wrote XMLTV artifact: %s/%s.xmltv", r.outputDir, r.ProxyID)
				}
			}

			// Display sample programs if requested
			if r.showSamples {
				r.printSamplePrograms(r.XMLTVContent)
			}

			return nil
		})
}

// runLogoValidationTests validates that @logo:ULID helpers were resolved correctly.
func (r *E2ERunner) runLogoValidationTests(ctx context.Context) {
	// Validate Channel Logo URLs in M3U
	r.runTestWithInfo("Validate Channel Logo URLs in M3U",
		"Verify M3U tvg-logo attributes contain /api/v1/logos/{uploadedLogoID}",
		func() error {
			if r.channelLogoID == "" {
				r.log("  Skipping: no channel logo ID available")
				return nil
			}
			if r.M3UContent == "" {
				return fmt.Errorf("no M3U content available for validation")
			}

			expectedLogoPath := fmt.Sprintf("/api/v1/logos/%s", r.channelLogoID)

			if !strings.Contains(r.M3UContent, expectedLogoPath) {
				if strings.Contains(r.M3UContent, "tvg-logo=") {
					sampleLogo := extractAttribute(r.M3UContent, "tvg-logo")
					return fmt.Errorf("M3U contains tvg-logo but not with expected logo ID.\n  Expected path containing: %s\n  Sample logo found: %s\n  This indicates the @logo: helper was not resolved correctly", expectedLogoPath, sampleLogo)
				}
				r.log("  Warning: No tvg-logo attributes found in M3U (may be expected if source had no logos)")
				return nil
			}

			logoCount := strings.Count(r.M3UContent, expectedLogoPath)
			r.log("  Validated: %d channel logos use uploaded placeholder (ID: %s)", logoCount, r.channelLogoID)
			return nil
		})

	// Validate Program Icon URLs in XMLTV
	r.runTestWithInfo("Validate Program Icon URLs in XMLTV",
		"Verify XMLTV <icon> elements contain /api/v1/logos/{uploadedLogoID}",
		func() error {
			if r.programLogoID == "" {
				r.log("  Skipping: no program logo ID available")
				return nil
			}
			if r.XMLTVContent == "" {
				return fmt.Errorf("no XMLTV content available for validation")
			}

			expectedIconPath := fmt.Sprintf("/api/v1/logos/%s", r.programLogoID)

			if !strings.Contains(r.XMLTVContent, expectedIconPath) {
				if strings.Contains(r.XMLTVContent, "<icon src=") {
					sampleIcon := extractXMLElement(r.XMLTVContent, "icon")
					return fmt.Errorf("XMLTV contains icons but not with expected logo ID.\n  Expected path containing: %s\n  Sample icon found: %s\n  This indicates the @logo: helper was not resolved correctly", expectedIconPath, sampleIcon)
				}
				r.log("  Warning: No <icon> elements found in XMLTV (may be expected if source had no icons)")
				return nil
			}

			iconCount := strings.Count(r.XMLTVContent, expectedIconPath)
			r.log("  Validated: %d program icons use uploaded placeholder (ID: %s)", iconCount, r.programLogoID)
			return nil
		})
}
