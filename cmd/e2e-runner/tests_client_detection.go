//nolint:errcheck,gocognit,gocyclo,nestif,gocritic,godot,wrapcheck,gosec,revive,goprintffuncname,modernize // E2E test runner uses relaxed linting
package main

import (
	"context"
	"fmt"
)

// runClientDetectionTests runs client detection rule tests.
func (r *E2ERunner) runClientDetectionTests(ctx context.Context) {
	// List Mappings
	r.runTestWithInfo("Client Detection: List Mappings",
		"GET /api/v1/client-detection-rules - fetch all rules, verify ordering by priority",
		func() error {
			mappings, err := r.client.GetClientDetectionMappings(ctx)
			if err != nil {
				return err
			}
			if len(mappings) == 0 {
				return fmt.Errorf("expected default client detection mappings, got none")
			}
			r.log("  Found %d client detection mappings", len(mappings))

			// Verify default mappings exist and are ordered by priority
			var lastPriority int = -1
			for _, m := range mappings {
				if m.Priority < lastPriority {
					return fmt.Errorf("mappings not ordered by priority: %s (priority %d) came after priority %d",
						m.Name, m.Priority, lastPriority)
				}
				lastPriority = m.Priority
			}
			r.log("  Mappings are correctly ordered by priority")
			return nil
		})

	// Get Stats
	r.runTestWithInfo("Client Detection: Get Stats",
		"GET /api/v1/client-detection-rules - compute stats from rules list",
		func() error {
			stats, err := r.client.GetClientDetectionStats(ctx)
			if err != nil {
				return err
			}
			if stats.Total == 0 {
				return fmt.Errorf("expected non-zero total mappings")
			}
			if stats.System == 0 {
				return fmt.Errorf("expected non-zero system mappings")
			}
			if stats.Enabled == 0 {
				return fmt.Errorf("expected non-zero enabled mappings")
			}
			r.log("  Stats: total=%d, enabled=%d, system=%d, custom=%d",
				stats.Total, stats.Enabled, stats.System, stats.Custom)
			return nil
		})

	// Test Expression - Chrome Match
	r.runTestWithInfo("Client Detection: Test Expression - Chrome Match",
		`POST /api/v1/client-detection-rules/test with User-Agent: Chrome/120, expr: contains "Chrome"`,
		func() error {
			headers := map[string]string{}
			chromeUA := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
			matches, err := r.client.TestClientDetectionExpressionWithHeaders(ctx,
				`@dynamic(request.headers):user-agent contains "Chrome"`, chromeUA, headers)
			if err != nil {
				return err
			}
			if !matches {
				return fmt.Errorf("expected Chrome expression to match Chrome user agent")
			}
			r.log("  Chrome expression matched correctly")
			return nil
		})

	// Test Expression - Safari Match
	r.runTestWithInfo("Client Detection: Test Expression - Safari Match",
		`POST /api/v1/client-detection-rules/test with User-Agent: Safari/17.0, expr: contains "Safari" AND not_contains "Chrome"`,
		func() error {
			headers := map[string]string{}
			safariUA := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15"
			matches, err := r.client.TestClientDetectionExpressionWithHeaders(ctx,
				`@dynamic(request.headers):user-agent contains "Safari" AND @dynamic(request.headers):user-agent not_contains "Chrome"`,
				safariUA, headers)
			if err != nil {
				return err
			}
			if !matches {
				return fmt.Errorf("expected Safari expression to match Safari user agent")
			}
			r.log("  Safari expression matched correctly")
			return nil
		})

	// Test Expression - Non-Match
	r.runTestWithInfo("Client Detection: Test Expression - Non-Match",
		`POST /api/v1/client-detection-rules/test with User-Agent: Firefox/120, expr: contains "Chrome" (should NOT match)`,
		func() error {
			headers := map[string]string{}
			firefoxUA := "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:120.0) Gecko/20100101 Firefox/120.0"
			matches, err := r.client.TestClientDetectionExpressionWithHeaders(ctx,
				`@dynamic(request.headers):user-agent contains "Chrome"`, firefoxUA, headers)
			if err != nil {
				return err
			}
			if matches {
				return fmt.Errorf("expected Chrome expression to NOT match Firefox user agent")
			}
			r.log("  Chrome expression correctly did not match Firefox")
			return nil
		})

	// Verify Generic Smart TV Rule
	r.runTestWithInfo("Client Detection: Verify Generic Smart TV Rule",
		"GET /api/v1/client-detection-rules - verify system rule 'Generic Smart TV' exists at priority 900",
		func() error {
			mappings, err := r.client.GetClientDetectionMappings(ctx)
			if err != nil {
				return err
			}
			var ruleFound bool
			for _, m := range mappings {
				if m.Name == "Generic Smart TV" && m.Priority == 900 {
					ruleFound = true
					if !m.IsSystem {
						return fmt.Errorf("generic Smart TV rule should be a system rule")
					}
					if !m.IsEnabled {
						return fmt.Errorf("generic Smart TV rule should be enabled")
					}
					r.log("  Found Generic Smart TV rule: %s (priority %d)", m.Name, m.Priority)
					break
				}
			}
			if !ruleFound {
				return fmt.Errorf("expected Generic Smart TV rule with priority 900")
			}
			return nil
		})

	// Explicit Codec Header Tests
	r.runExplicitCodecTests(ctx)
}

// runExplicitCodecTests runs tests for explicit X-Video-Codec and X-Audio-Codec headers.
func (r *E2ERunner) runExplicitCodecTests(ctx context.Context) {
	// X-Video-Codec H.265
	r.runTestWithInfo("Client Detection: Explicit X-Video-Codec H.265 Match",
		`POST /api/v1/client-detection-rules/test with X-Video-Codec: h265`,
		func() error {
			headers := map[string]string{
				"X-Video-Codec": "h265",
			}
			matches, err := r.client.TestClientDetectionExpressionWithHeaders(ctx,
				`@dynamic(request.headers):x-video-codec equals "h265"`, "", headers)
			if err != nil {
				return err
			}
			if !matches {
				return fmt.Errorf("expected X-Video-Codec h265 expression to match")
			}
			r.log("  X-Video-Codec h265 expression matched correctly")
			return nil
		})

	// X-Video-Codec H.264
	r.runTestWithInfo("Client Detection: Explicit X-Video-Codec H.264 Match",
		`POST /api/v1/client-detection-rules/test with X-Video-Codec: h264`,
		func() error {
			headers := map[string]string{
				"X-Video-Codec": "h264",
			}
			matches, err := r.client.TestClientDetectionExpressionWithHeaders(ctx,
				`@dynamic(request.headers):x-video-codec equals "h264"`, "", headers)
			if err != nil {
				return err
			}
			if !matches {
				return fmt.Errorf("expected X-Video-Codec h264 expression to match")
			}
			r.log("  X-Video-Codec h264 expression matched correctly")
			return nil
		})

	// X-Video-Codec VP9
	r.runTestWithInfo("Client Detection: Explicit X-Video-Codec VP9 Match",
		`POST /api/v1/client-detection-rules/test with X-Video-Codec: vp9`,
		func() error {
			headers := map[string]string{
				"X-Video-Codec": "vp9",
			}
			matches, err := r.client.TestClientDetectionExpressionWithHeaders(ctx,
				`@dynamic(request.headers):x-video-codec equals "vp9"`, "", headers)
			if err != nil {
				return err
			}
			if !matches {
				return fmt.Errorf("expected X-Video-Codec vp9 expression to match")
			}
			r.log("  X-Video-Codec vp9 expression matched correctly")
			return nil
		})

	// X-Video-Codec AV1
	r.runTestWithInfo("Client Detection: Explicit X-Video-Codec AV1 Match",
		`POST /api/v1/client-detection-rules/test with X-Video-Codec: av1`,
		func() error {
			headers := map[string]string{
				"X-Video-Codec": "av1",
			}
			matches, err := r.client.TestClientDetectionExpressionWithHeaders(ctx,
				`@dynamic(request.headers):x-video-codec equals "av1"`, "", headers)
			if err != nil {
				return err
			}
			if !matches {
				return fmt.Errorf("expected X-Video-Codec av1 expression to match")
			}
			r.log("  X-Video-Codec av1 expression matched correctly")
			return nil
		})

	// X-Audio-Codec AAC
	r.runTestWithInfo("Client Detection: Explicit X-Audio-Codec AAC Match",
		`POST /api/v1/client-detection-rules/test with X-Audio-Codec: aac`,
		func() error {
			headers := map[string]string{
				"X-Audio-Codec": "aac",
			}
			matches, err := r.client.TestClientDetectionExpressionWithHeaders(ctx,
				`@dynamic(request.headers):x-audio-codec equals "aac"`, "", headers)
			if err != nil {
				return err
			}
			if !matches {
				return fmt.Errorf("expected X-Audio-Codec aac expression to match")
			}
			r.log("  X-Audio-Codec aac expression matched correctly")
			return nil
		})

	// X-Audio-Codec Opus
	r.runTestWithInfo("Client Detection: Explicit X-Audio-Codec Opus Match",
		`POST /api/v1/client-detection-rules/test with X-Audio-Codec: opus`,
		func() error {
			headers := map[string]string{
				"X-Audio-Codec": "opus",
			}
			matches, err := r.client.TestClientDetectionExpressionWithHeaders(ctx,
				`@dynamic(request.headers):x-audio-codec equals "opus"`, "", headers)
			if err != nil {
				return err
			}
			if !matches {
				return fmt.Errorf("expected X-Audio-Codec opus expression to match")
			}
			r.log("  X-Audio-Codec opus expression matched correctly")
			return nil
		})

	// Combined Video and Audio Codec Headers
	r.runTestWithInfo("Client Detection: Combined Video and Audio Codec Headers",
		`POST /api/v1/client-detection-rules/test with X-Video-Codec: h265, X-Audio-Codec: aac`,
		func() error {
			headers := map[string]string{
				"X-Video-Codec": "h265",
				"X-Audio-Codec": "aac",
			}
			matches, err := r.client.TestClientDetectionExpressionWithHeaders(ctx,
				`@dynamic(request.headers):x-video-codec equals "h265" AND @dynamic(request.headers):x-audio-codec equals "aac"`,
				"", headers)
			if err != nil {
				return err
			}
			if !matches {
				return fmt.Errorf("expected combined codec expression to match")
			}
			r.log("  Combined codec expression matched correctly")
			return nil
		})

	// Codec Header Priority Over User-Agent
	r.runTestWithInfo("Client Detection: Codec Header Priority Over User-Agent",
		`POST /api/v1/client-detection-rules/test with X-Video-Codec: h265 + User-Agent: Chrome`,
		func() error {
			headers := map[string]string{
				"X-Video-Codec": "h265",
			}
			chromeUA := "Mozilla/5.0 Chrome/120.0"

			matches, err := r.client.TestClientDetectionExpressionWithHeaders(ctx,
				`@dynamic(request.headers):x-video-codec equals "h265"`, chromeUA, headers)
			if err != nil {
				return err
			}
			if !matches {
				return fmt.Errorf("expected explicit codec header to match with User-Agent present")
			}
			r.log("  Explicit codec header matched with User-Agent present")
			return nil
		})

	// Invalid Codec Header Fallthrough
	r.runTestWithInfo("Client Detection: Invalid Codec Header Fallthrough",
		`POST /api/v1/client-detection-rules/test with X-Video-Codec: invalid_codec (should NOT match h265)`,
		func() error {
			headers := map[string]string{
				"X-Video-Codec": "invalid_codec",
			}
			matches, err := r.client.TestClientDetectionExpressionWithHeaders(ctx,
				`@dynamic(request.headers):x-video-codec equals "h265"`, "", headers)
			if err != nil {
				return err
			}
			if matches {
				return fmt.Errorf("expected invalid codec to NOT match h265 expression")
			}
			r.log("  Invalid codec correctly did not match")
			return nil
		})

	// Case Sensitive Codec Matching
	r.runTestWithInfo("Client Detection: Case Sensitive Codec Matching",
		`POST /api/v1/client-detection-rules/test with X-Video-Codec: H265 vs expr: equals "h265"`,
		func() error {
			headers := map[string]string{
				"X-Video-Codec": "H265", // uppercase
			}
			matches, err := r.client.TestClientDetectionExpressionWithHeaders(ctx,
				`@dynamic(request.headers):x-video-codec equals "h265"`, "", headers)
			if err != nil {
				return err
			}
			if matches {
				return fmt.Errorf("expected uppercase codec to NOT match lowercase expression")
			}
			r.log("  Case sensitivity verified - H265 != h265")
			return nil
		})
}
