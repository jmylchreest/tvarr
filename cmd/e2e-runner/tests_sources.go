//nolint:errcheck,gocognit,gocyclo,nestif,gocritic,godot,wrapcheck,gosec,revive,goprintffuncname,modernize // E2E test runner uses relaxed linting
package main

import (
	"context"
	"fmt"
	"time"
)

// runLogoUploadTests runs logo upload and data mapping rule tests.
func (r *E2ERunner) runLogoUploadTests(ctx context.Context) {
	// Upload Channel Logo Placeholder
	r.runTestWithInfo("Upload Channel Logo Placeholder",
		"POST /api/v1/logos/upload - upload channel-placeholder.webp (64x64 WebP)",
		func() error {
			channelLogoData, err := testdataFS.ReadFile("testdata/channel.webp")
			if err != nil {
				return fmt.Errorf("read channel.webp from embedded testdata: %w", err)
			}
			result, err := r.client.UploadLogo(ctx, "channel-placeholder.webp", channelLogoData)
			if err != nil {
				return err
			}
			r.channelLogoID = result.ID
			r.channelLogoURL = result.URL
			r.log("  Uploaded channel logo: ID=%s URL=%s", r.channelLogoID, r.channelLogoURL)
			return nil
		})

	// Upload Program Logo Placeholder
	r.runTestWithInfo("Upload Program Logo Placeholder",
		"POST /api/v1/logos/upload - upload program-placeholder.webp (64x64 WebP)",
		func() error {
			programLogoData, err := testdataFS.ReadFile("testdata/program.webp")
			if err != nil {
				return fmt.Errorf("read program.webp from embedded testdata: %w", err)
			}
			result, err := r.client.UploadLogo(ctx, "program-placeholder.webp", programLogoData)
			if err != nil {
				return err
			}
			r.programLogoID = result.ID
			r.programLogoURL = result.URL
			r.log("  Uploaded program logo: ID=%s URL=%s", r.programLogoID, r.programLogoURL)
			return nil
		})

	// Create Channel Logo Mapping Rule
	r.runTestWithInfo("Create Channel Logo Mapping Rule",
		"POST /api/v1/filters - create stream filter: tvg_logo starts_with http SET @logo:ULID",
		func() error {
			if r.channelLogoID == "" {
				return fmt.Errorf("no channel logo ID available")
			}
			expression := fmt.Sprintf(`tvg_logo starts_with "http" SET tvg_logo = "@logo:%s"`, r.channelLogoID)
			ruleID, err := r.client.CreateDataMappingRule(ctx,
				fmt.Sprintf("E2E Channel Logo Placeholder %s", r.runID),
				"stream",
				expression,
				100, // High priority
			)
			if err != nil {
				return err
			}
			r.log("  Created channel logo mapping rule: %s (sets @logo:%s)", ruleID, r.channelLogoID)
			return nil
		})

	// Create Program Icon Mapping Rule
	r.runTestWithInfo("Create Program Icon Mapping Rule",
		"POST /api/v1/filters - create EPG filter: programme_icon starts_with http SET @logo:ULID",
		func() error {
			if r.programLogoID == "" {
				return fmt.Errorf("no program logo ID available")
			}
			expression := fmt.Sprintf(`programme_icon starts_with "http" SET programme_icon = "@logo:%s"`, r.programLogoID)
			ruleID, err := r.client.CreateDataMappingRule(ctx,
				fmt.Sprintf("E2E Program Icon Placeholder %s", r.runID),
				"epg",
				expression,
				100, // High priority
			)
			if err != nil {
				return err
			}
			r.log("  Created program icon mapping rule: %s (sets @logo:%s)", ruleID, r.programLogoID)
			return nil
		})
}

// runSourceTests runs source creation and ingestion tests.
func (r *E2ERunner) runSourceTests(ctx context.Context) {
	// Create Stream Source
	r.runTestWithInfo("Create Stream Source",
		"POST /api/v1/sources/stream - create M3U source from testdata URL",
		func() error {
			var err error
			r.StreamSourceID, err = r.client.CreateStreamSource(ctx, fmt.Sprintf("E2E Test M3U %s", r.runID), r.m3uURL)
			if err != nil {
				return err
			}
			r.log("  Created stream source: %s", r.StreamSourceID)
			return nil
		})

	// Ingest Stream Source
	r.runTestWithInfo("Ingest Stream Source",
		"POST /api/v1/sources/stream/{id}/ingest + SSE /api/v1/progress/events until completed",
		func() error {
			if r.StreamSourceID == "" {
				return fmt.Errorf("no stream source to ingest")
			}
			return r.client.TriggerIngestion(ctx, "stream", r.StreamSourceID, 3*time.Minute)
		})

	// Verify Channel Count
	r.runTestWithInfo("Verify Channel Count",
		"GET /api/v1/channels?limit=1 - verify channels exist after ingestion",
		func() error {
			count, err := r.client.GetChannelCount(ctx)
			if err != nil {
				return err
			}
			r.log("  Channel count: %d", count)
			if count == 0 {
				return fmt.Errorf("no channels found after ingestion")
			}
			return nil
		})

	// Create EPG Source
	r.runTestWithInfo("Create EPG Source",
		"POST /api/v1/sources/epg - create XMLTV source from testdata URL",
		func() error {
			var err error
			r.EpgSourceID, err = r.client.CreateEPGSource(ctx, fmt.Sprintf("E2E Test EPG %s", r.runID), r.epgURL)
			if err != nil {
				return err
			}
			r.log("  Created EPG source: %s", r.EpgSourceID)
			return nil
		})

	// Ingest EPG Source
	r.runTestWithInfo("Ingest EPG Source",
		"POST /api/v1/sources/epg/{id}/ingest + SSE /api/v1/progress/events until completed",
		func() error {
			if r.EpgSourceID == "" {
				return fmt.Errorf("no EPG source to ingest")
			}
			return r.client.TriggerIngestion(ctx, "epg", r.EpgSourceID, 5*time.Minute)
		})
}
