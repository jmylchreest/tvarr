// Package testutil provides test utilities including sample data generation.
package testutil

import (
	"fmt"
	"math/rand"

	"github.com/jmylchreest/tvarr/internal/models"
)

// Standard fictional broadcasters for test data.
// NEVER use real brand names like BBC, ESPN, HBO, Sky, etc.
var (
	Broadcasters = []string{
		"StreamCast",
		"ViewMedia",
		"AeroVision",
		"GlobalStream",
		"NationalNet",
		"SportsCentral",
		"CinemaMax",
		"MusicMax",
		"NewsFirst",
		"PrimeTV",
	}

	ChannelVariants = []string{
		"One",
		"Two",
		"Three",
		"Prime",
		"Plus",
		"Max",
		"Gold",
		"Extra",
	}

	QualityVariants = []string{
		"HD",
		"SD",
		"4K",
		"UHD",
	}

	TimeshiftVariants = []string{
		"+1",
		"+2",
		"+24",
		"+1h",
	}

	// Categories with their associated channel name suffixes
	Categories = map[string][]string{
		"news": {
			"News",
			"News HD",
			"Breaking News",
			"World News",
			"Local News",
		},
		"sports": {
			"Sports",
			"Sports HD",
			"Racing HD",
			"Football HD",
			"Sports Extra",
		},
		"movies": {
			"Movies",
			"Movies HD",
			"Action Movies HD",
			"Classic Movies",
			"Cinema",
		},
		"entertainment": {
			"Entertainment",
			"Entertainment HD",
			"Lifestyle",
			"Comedy",
			"Drama",
		},
		"adult": {
			"Adult Channel",
			"Adult Movies",
			"Adult Entertainment",
			"Late Night",
		},
		"music": {
			"Music",
			"Music HD",
			"Hits",
			"Classic Hits",
			"Dance",
		},
		"kids": {
			"Kids",
			"Kids HD",
			"Cartoons",
			"Junior",
			"Family",
		},
	}
)

// SampleChannel represents a generated sample channel for testing.
type SampleChannel struct {
	TvgID       string
	TvgName     string
	TvgChNo     int
	ChannelName string
	TvgLogo     string
	GroupTitle  string
	StreamURL   string
	StreamType  string
	Language    string
	Country     string
	IsAdult     bool
}

// ToChannel converts a SampleChannel to a models.Channel.
func (s *SampleChannel) ToChannel(sourceID models.ULID) *models.Channel {
	return &models.Channel{
		SourceID:      sourceID,
		ExtID:         s.TvgID,
		TvgID:         s.TvgID,
		TvgName:       s.TvgName,
		TvgLogo:       s.TvgLogo,
		GroupTitle:    s.GroupTitle,
		ChannelName:   s.ChannelName,
		ChannelNumber: s.TvgChNo,
		StreamURL:     s.StreamURL,
		StreamType:    s.StreamType,
		Language:      s.Language,
		Country:       s.Country,
		IsAdult:       s.IsAdult,
	}
}

// SampleDataGenerator generates realistic but fictional channel data for testing.
type SampleDataGenerator struct {
	rng *rand.Rand
}

// NewSampleDataGenerator creates a new sample data generator with a random seed.
func NewSampleDataGenerator() *SampleDataGenerator {
	return &SampleDataGenerator{
		rng: rand.New(rand.NewSource(rand.Int63())),
	}
}

// NewSampleDataGeneratorWithSeed creates a new generator with a fixed seed for reproducibility.
func NewSampleDataGeneratorWithSeed(seed int64) *SampleDataGenerator {
	return &SampleDataGenerator{
		rng: rand.New(rand.NewSource(seed)),
	}
}

// RandomBroadcaster returns a random broadcaster name.
func (g *SampleDataGenerator) RandomBroadcaster() string {
	return Broadcasters[g.rng.Intn(len(Broadcasters))]
}

// RandomQuality returns a random quality variant (HD, SD, 4K, UHD).
func (g *SampleDataGenerator) RandomQuality() string {
	return QualityVariants[g.rng.Intn(len(QualityVariants))]
}

// RandomTimeshift returns a random timeshift variant (+1, +2, etc).
func (g *SampleDataGenerator) RandomTimeshift() string {
	return TimeshiftVariants[g.rng.Intn(len(TimeshiftVariants))]
}

// RandomChannelFromCategory returns a random channel suffix for the given category.
func (g *SampleDataGenerator) RandomChannelFromCategory(category string) string {
	channels, ok := Categories[category]
	if !ok {
		channels = Categories["entertainment"]
	}
	return channels[g.rng.Intn(len(channels))]
}

// GenerateChannelName generates a full channel name with broadcaster.
func (g *SampleDataGenerator) GenerateChannelName(category string) string {
	broadcaster := g.RandomBroadcaster()
	suffix := g.RandomChannelFromCategory(category)
	return fmt.Sprintf("%s %s", broadcaster, suffix)
}

// GenerateTimeshiftChannelName generates a channel name with timeshift suffix.
func (g *SampleDataGenerator) GenerateTimeshiftChannelName(category string) string {
	base := g.GenerateChannelName(category)
	timeshift := g.RandomTimeshift()
	return fmt.Sprintf("%s %s", base, timeshift)
}

// GenerateOptions configures channel generation.
type GenerateOptions struct {
	Category        string  // Category filter (news, sports, movies, entertainment, adult, music, kids)
	TimeshiftRatio  float64 // Ratio of timeshift channels (0.0-1.0)
	StartChannelNum int     // Starting channel number
	StreamURLBase   string  // Base URL for streams (defaults to example.com)
	LogoURLBase     string  // Base URL for logos (defaults to logos.example.com)
}

// DefaultGenerateOptions returns default generation options.
func DefaultGenerateOptions() GenerateOptions {
	return GenerateOptions{
		Category:        "entertainment",
		TimeshiftRatio:  0.2,
		StartChannelNum: 101,
		StreamURLBase:   "https://stream.example.com/channel",
		LogoURLBase:     "https://logos.example.com/channel",
	}
}

// GenerateSampleChannels generates multiple sample channels for testing.
func (g *SampleDataGenerator) GenerateSampleChannels(count int, opts GenerateOptions) []SampleChannel {
	channels := make([]SampleChannel, count)

	for i := 0; i < count; i++ {
		var channelName string
		if g.rng.Float64() < opts.TimeshiftRatio {
			channelName = g.GenerateTimeshiftChannelName(opts.Category)
		} else {
			channelName = g.GenerateChannelName(opts.Category)
		}

		groupTitle := opts.Category
		if groupTitle == "" {
			groupTitle = "Entertainment"
		}
		// Capitalize first letter
		if len(groupTitle) > 0 {
			groupTitle = string(groupTitle[0]-32) + groupTitle[1:]
		}

		channels[i] = SampleChannel{
			TvgID:       fmt.Sprintf("ch%03d", i+1),
			TvgName:     channelName,
			TvgChNo:     opts.StartChannelNum + i,
			ChannelName: channelName,
			TvgLogo:     fmt.Sprintf("%s%d.png", opts.LogoURLBase, i+1),
			GroupTitle:  groupTitle,
			StreamURL:   fmt.Sprintf("%s%d", opts.StreamURLBase, i+1),
			StreamType:  "live",
			IsAdult:     opts.Category == "adult",
		}
	}

	return channels
}

// GenerateSportsChannels generates sports channels.
func (g *SampleDataGenerator) GenerateSportsChannels(count int) []SampleChannel {
	opts := DefaultGenerateOptions()
	opts.Category = "sports"
	return g.GenerateSampleChannels(count, opts)
}

// GenerateNewsChannels generates news channels.
func (g *SampleDataGenerator) GenerateNewsChannels(count int) []SampleChannel {
	opts := DefaultGenerateOptions()
	opts.Category = "news"
	return g.GenerateSampleChannels(count, opts)
}

// GenerateMovieChannels generates movie channels.
func (g *SampleDataGenerator) GenerateMovieChannels(count int) []SampleChannel {
	opts := DefaultGenerateOptions()
	opts.Category = "movies"
	return g.GenerateSampleChannels(count, opts)
}

// GenerateAdultChannels generates adult channels (for filter testing).
func (g *SampleDataGenerator) GenerateAdultChannels(count int) []SampleChannel {
	opts := DefaultGenerateOptions()
	opts.Category = "adult"
	return g.GenerateSampleChannels(count, opts)
}

// GenerateTimeshiftChannels generates channels with timeshift suffixes.
func (g *SampleDataGenerator) GenerateTimeshiftChannels(count int, category string) []SampleChannel {
	opts := DefaultGenerateOptions()
	opts.Category = category
	opts.TimeshiftRatio = 1.0 // All channels will be timeshift
	return g.GenerateSampleChannels(count, opts)
}

// GenerateStandardChannels generates channels without timeshift suffixes.
func (g *SampleDataGenerator) GenerateStandardChannels(count int, category string) []SampleChannel {
	opts := DefaultGenerateOptions()
	opts.Category = category
	opts.TimeshiftRatio = 0.0 // No timeshift channels
	return g.GenerateSampleChannels(count, opts)
}

// GenerateMixedChannels generates a mix of channels from different categories.
func (g *SampleDataGenerator) GenerateMixedChannels(count int) []SampleChannel {
	categories := []string{"news", "sports", "movies", "entertainment", "music", "kids"}
	channels := make([]SampleChannel, count)

	for i := 0; i < count; i++ {
		category := categories[g.rng.Intn(len(categories))]
		opts := DefaultGenerateOptions()
		opts.Category = category
		opts.StartChannelNum = 101 + i

		generated := g.GenerateSampleChannels(1, opts)
		if len(generated) > 0 {
			channels[i] = generated[0]
			channels[i].TvgID = fmt.Sprintf("ch%03d", i+1)
			channels[i].TvgChNo = 101 + i
		}
	}

	return channels
}
