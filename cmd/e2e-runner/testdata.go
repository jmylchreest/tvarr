package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jmylchreest/tvarr/internal/testutil"
)

// countXMLTVPrograms counts the number of <programme elements in XMLTV content.
func countXMLTVPrograms(xmltv string) int {
	return strings.Count(xmltv, "<programme ")
}

// TestDataConfig configures test data generation.
type TestDataConfig struct {
	ChannelCount        int
	ProgramCount        int
	RandomSeed          int64
	ProgramDurations    []int // in minutes
	IncludePlusOneChans bool
	IncludeHDVariants   bool
	BaseURL             string // URL prefix for stream URLs
	LogoBaseURL         string // URL prefix for logos
	AnchorTime          time.Time
	RequiredChannels    int // Validation: expected channel count
	RequiredPrograms    int // Validation: expected program count
}

// DefaultTestDataConfig returns the default configuration.
func DefaultTestDataConfig() TestDataConfig {
	return TestDataConfig{
		ChannelCount:        50,
		ProgramCount:        5000, // 100 per channel
		RandomSeed:          time.Now().UnixNano(),
		ProgramDurations:    []int{30, 60, 90},
		IncludePlusOneChans: true,
		IncludeHDVariants:   true,
		BaseURL:             "http://teststream.local",
		LogoBaseURL:         "http://testlogos.local",
		AnchorTime:          time.Now().Add(30 * time.Minute),
		RequiredChannels:    0,
		RequiredPrograms:    0,
	}
}

// GeneratedTestData holds the generated M3U and XMLTV content.
type GeneratedTestData struct {
	M3UContent   string
	XMLTVContent string
	ChannelCount int
	ProgramCount int
	M3UPath      string
	XMLTVPath    string
}

// TestDataGenerator generates test data for E2E testing.
type TestDataGenerator struct {
	config    TestDataConfig
	generator *testutil.SampleDataGenerator
	rng       *rand.Rand
}

// NewTestDataGenerator creates a new test data generator.
func NewTestDataGenerator(config TestDataConfig) *TestDataGenerator {
	return &TestDataGenerator{
		config:    config,
		generator: testutil.NewSampleDataGeneratorWithSeed(config.RandomSeed),
		rng:       rand.New(rand.NewSource(config.RandomSeed)),
	}
}

// Generate creates the test data.
func (g *TestDataGenerator) Generate() (*GeneratedTestData, error) {
	channels := g.generateChannels()
	programs := g.generatePrograms(channels)

	// Validate that program slice has expected count
	expectedPrograms := g.config.ProgramCount
	if len(programs) != expectedPrograms {
		return nil, fmt.Errorf("program generation error: generated %d programs but expected %d", len(programs), expectedPrograms)
	}

	m3u := g.generateM3U(channels)
	xmltv := g.generateXMLTV(channels, programs)

	// Validate that the XMLTV content has the expected number of programs
	xmltvProgramCount := countXMLTVPrograms(xmltv)
	if xmltvProgramCount != len(programs) {
		return nil, fmt.Errorf("XMLTV generation error: generated %d programmes but expected %d from slice", xmltvProgramCount, len(programs))
	}

	return &GeneratedTestData{
		M3UContent:   m3u,
		XMLTVContent: xmltv,
		ChannelCount: len(channels),
		ProgramCount: len(programs),
	}, nil
}

// generateChannels creates the channel list using testutil.
func (g *TestDataGenerator) generateChannels() []testutil.SampleChannel {
	channels := make([]testutil.SampleChannel, 0, g.config.ChannelCount)

	// Categories to cycle through
	categories := []string{"news", "sports", "movies", "entertainment", "music", "kids"}

	// Calculate distribution
	channelNum := 101

	for i := 0; len(channels) < g.config.ChannelCount; i++ {
		category := categories[i%len(categories)]

		opts := testutil.GenerateOptions{
			Category:        category,
			TimeshiftRatio:  0,
			StartChannelNum: channelNum,
			StreamURLBase:   g.config.BaseURL + "/live/",
			LogoURLBase:     g.config.LogoBaseURL + "/logos/",
		}

		// Generate base channel
		baseChannels := g.generator.GenerateSampleChannels(1, opts)
		if len(baseChannels) == 0 {
			continue
		}

		baseChannel := baseChannels[0]
		baseChannel.TvgID = fmt.Sprintf("ch%03d", len(channels)+1)
		baseChannel.StreamURL = fmt.Sprintf("%s/live/%s/stream.m3u8", g.config.BaseURL, baseChannel.TvgID)
		channels = append(channels, baseChannel)
		channelNum++

		// HD variant (if enabled and within limit)
		if g.config.IncludeHDVariants && len(channels) < g.config.ChannelCount && g.rng.Float32() > 0.3 {
			hdChannel := baseChannel
			hdChannel.TvgID = fmt.Sprintf("ch%03d", len(channels)+1)
			hdChannel.TvgName = baseChannel.TvgName + " HD"
			hdChannel.ChannelName = baseChannel.ChannelName + " HD"
			hdChannel.TvgChNo = channelNum
			hdChannel.StreamURL = fmt.Sprintf("%s/live/%s/stream.m3u8", g.config.BaseURL, hdChannel.TvgID)
			channels = append(channels, hdChannel)
			channelNum++
		}

		// +1 variant (if enabled and within limit)
		if g.config.IncludePlusOneChans && len(channels) < g.config.ChannelCount && g.rng.Float32() > 0.5 {
			plusOneChannel := baseChannel
			plusOneChannel.TvgID = fmt.Sprintf("ch%03d", len(channels)+1)
			plusOneChannel.TvgName = baseChannel.TvgName + " +1"
			plusOneChannel.ChannelName = baseChannel.ChannelName + " +1"
			plusOneChannel.TvgChNo = channelNum
			plusOneChannel.StreamURL = fmt.Sprintf("%s/live/%s/stream.m3u8", g.config.BaseURL, plusOneChannel.TvgID)
			channels = append(channels, plusOneChannel)
			channelNum++
		}
	}

	return channels
}

// generatePrograms creates the program list using testutil.
func (g *TestDataGenerator) generatePrograms(channels []testutil.SampleChannel) []testutil.SampleProgram {
	opts := testutil.DefaultProgramGenerateOptions()
	opts.Durations = g.config.ProgramDurations
	opts.AnchorTime = g.config.AnchorTime
	opts.IconURLBase = g.config.LogoBaseURL + "/programs"

	return g.generator.GenerateProgramsForChannels(channels, g.config.ProgramCount, opts)
}

// generateM3U creates the M3U playlist content.
func (g *TestDataGenerator) generateM3U(channels []testutil.SampleChannel) string {
	var sb strings.Builder
	sb.WriteString("#EXTM3U\n")

	for _, ch := range channels {
		// Build EXTINF line with attributes
		sb.WriteString(fmt.Sprintf(`#EXTINF:-1 tvg-id="%s" tvg-name="%s" tvg-logo="%s" group-title="%s" tvg-chno="%d"`,
			ch.TvgID, ch.TvgName, ch.TvgLogo, ch.GroupTitle, ch.TvgChNo))

		// Add tvg-shift for timeshift channels
		if strings.Contains(ch.ChannelName, "+1") {
			sb.WriteString(` tvg-shift="1"`)
		} else if strings.Contains(ch.ChannelName, "+2") {
			sb.WriteString(` tvg-shift="2"`)
		} else if strings.Contains(ch.ChannelName, "+24") {
			sb.WriteString(` tvg-shift="24"`)
		}

		sb.WriteString(fmt.Sprintf(",%s\n", ch.ChannelName))

		// Stream URL
		sb.WriteString(ch.StreamURL + "\n")
	}

	return sb.String()
}

// generateXMLTV creates the XMLTV content.
func (g *TestDataGenerator) generateXMLTV(channels []testutil.SampleChannel, programs []testutil.SampleProgram) string {
	var sb strings.Builder

	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString("\n")
	sb.WriteString(`<tv generator-info-name="tvarr-testdata" generator-info-url="https://github.com/jmylchreest/tvarr">`)
	sb.WriteString("\n")

	// Write channels
	for _, ch := range channels {
		sb.WriteString(fmt.Sprintf(`  <channel id="%s">`, escapeXML(ch.TvgID)))
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf(`    <display-name>%s</display-name>`, escapeXML(ch.ChannelName)))
		sb.WriteString("\n")
		if ch.TvgLogo != "" {
			sb.WriteString(fmt.Sprintf(`    <icon src="%s"/>`, escapeXML(ch.TvgLogo)))
			sb.WriteString("\n")
		}
		sb.WriteString(`  </channel>`)
		sb.WriteString("\n")
	}

	// Write programs
	for _, p := range programs {
		startStr := generateXMLTVTimestamp(p.Start)
		stopStr := generateXMLTVTimestamp(p.Stop)

		sb.WriteString(fmt.Sprintf(`  <programme start="%s" stop="%s" channel="%s">`,
			startStr, stopStr, escapeXML(p.ChannelID)))
		sb.WriteString("\n")

		sb.WriteString(fmt.Sprintf(`    <title lang="en">%s</title>`, escapeXML(p.Title)))
		sb.WriteString("\n")

		if p.Description != "" {
			sb.WriteString(fmt.Sprintf(`    <desc lang="en">%s</desc>`, escapeXML(p.Description)))
			sb.WriteString("\n")
		}

		if p.Category != "" {
			sb.WriteString(fmt.Sprintf(`    <category lang="en">%s</category>`, escapeXML(p.Category)))
			sb.WriteString("\n")
		}

		if p.EpisodeNum != "" {
			sb.WriteString(fmt.Sprintf(`    <episode-num system="xmltv_ns">%s</episode-num>`, escapeXML(p.EpisodeNum)))
			sb.WriteString("\n")
		}

		if p.Icon != "" {
			sb.WriteString(fmt.Sprintf(`    <icon src="%s"/>`, escapeXML(p.Icon)))
			sb.WriteString("\n")
		}

		if p.Rating != "" {
			sb.WriteString(`    <rating>`)
			sb.WriteString("\n")
			sb.WriteString(fmt.Sprintf(`      <value>%s</value>`, escapeXML(p.Rating)))
			sb.WriteString("\n")
			sb.WriteString(`    </rating>`)
			sb.WriteString("\n")
		}

		sb.WriteString(`  </programme>`)
		sb.WriteString("\n")
	}

	sb.WriteString(`</tv>`)
	sb.WriteString("\n")

	return sb.String()
}

// generateXMLTVTimestamp formats a time for XMLTV (YYYYMMDDHHmmss +0000).
func generateXMLTVTimestamp(t time.Time) string {
	// XMLTV uses format: YYYYMMDDHHmmss +HHMM (with space, not T)
	return t.UTC().Format("20060102150405") + " +0000"
}

// escapeXML escapes special characters for XML content.
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// WriteToFiles writes the generated test data to files.
func (data *GeneratedTestData) WriteToFiles(dir string) error {
	// Ensure directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Write M3U file
	data.M3UPath = filepath.Join(dir, "test.m3u")
	if err := os.WriteFile(data.M3UPath, []byte(data.M3UContent), 0644); err != nil {
		return fmt.Errorf("failed to write M3U file: %w", err)
	}

	// Write XMLTV file
	data.XMLTVPath = filepath.Join(dir, "test.xml")
	if err := os.WriteFile(data.XMLTVPath, []byte(data.XMLTVContent), 0644); err != nil {
		return fmt.Errorf("failed to write XMLTV file: %w", err)
	}

	return nil
}

// Validate checks if the generated data meets requirements.
func (data *GeneratedTestData) Validate(requiredChannels, requiredPrograms int) error {
	if requiredChannels > 0 && data.ChannelCount != requiredChannels {
		return fmt.Errorf("channel count mismatch: generated %d, required %d", data.ChannelCount, requiredChannels)
	}
	if requiredPrograms > 0 && data.ProgramCount != requiredPrograms {
		return fmt.Errorf("program count mismatch: generated %d, required %d", data.ProgramCount, requiredPrograms)
	}
	return nil
}

// M3UURL returns a file:// URL to the M3U file.
func (data *GeneratedTestData) M3UURL() string {
	return "file://" + data.M3UPath
}

// XMLTVURL returns a file:// URL to the XMLTV file.
func (data *GeneratedTestData) XMLTVURL() string {
	return "file://" + data.XMLTVPath
}

// GenerateStaticTestData generates the static test data files for embedding.
// This function is used to generate test.m3u and test.xml files that can be
// committed to the repository.
func GenerateStaticTestData() (*GeneratedTestData, error) {
	config := TestDataConfig{
		ChannelCount:        50,
		ProgramCount:        5000, // 100 per channel
		RandomSeed:          42,   // Fixed seed for reproducibility
		ProgramDurations:    []int{10, 15, 30, 60, 90},
		IncludePlusOneChans: true,
		IncludeHDVariants:   true,
		BaseURL:             "http://teststream.tvarr.local",
		LogoBaseURL:         "http://testlogos.tvarr.local",
		AnchorTime:          time.Now().Add(-1 * time.Hour).Truncate(time.Hour),
	}

	generator := NewTestDataGenerator(config)
	return generator.Generate()
}
