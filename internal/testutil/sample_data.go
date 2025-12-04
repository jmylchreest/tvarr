// Package testutil provides test utilities including sample data generation.
package testutil

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/jmylchreest/tvarr/internal/models"
)

// E2E Test Data URLs - publicly accessible, compatible M3U and EPG data
// These URLs are used for E2E validation testing. The sources are:
// - Free and publicly accessible (no authentication required)
// - Compatible: EPG channel IDs match M3U channel IDs
// - Reasonably sized for testing (not too large, not too small)
const (
	// E2ETestM3UURL is the default M3U stream source for E2E testing.
	// m3upt.com provides free European channels with matching EPG.
	E2ETestM3UURL = "https://m3upt.com/iptv"

	// E2ETestEPGURL is the default EPG source for E2E testing.
	// m3upt.com EPG contains channel IDs matching the IPTV source.
	E2ETestEPGURL = "https://m3upt.com/epg"

	// AlternativeM3UURL is an alternative M3U source (US channels, larger dataset).
	// Note: This source has no matching EPG, use only for M3U-only testing.
	AlternativeM3UURL = "https://iptv-org.github.io/iptv/countries/us.m3u"
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

	// ProgramTemplates provides fictional program titles and descriptions.
	// NEVER use real show names, movie titles, or trademarked content.
	ProgramTemplates = []ProgramTemplate{
		// News programs
		{Title: "Morning Report", Description: "Start your day with comprehensive news coverage and weather updates.", Category: "News"},
		{Title: "Midday Bulletin", Description: "Midday news roundup with the latest headlines.", Category: "News"},
		{Title: "Evening Edition", Description: "In-depth coverage of the day's major stories.", Category: "News"},
		{Title: "World Tonight", Description: "International news and global affairs.", Category: "News"},
		{Title: "Business Update", Description: "Financial markets and business news analysis.", Category: "News"},
		{Title: "Weather Watch", Description: "Detailed weather forecasts and climate updates.", Category: "News"},

		// Entertainment programs
		{Title: "Morning Show Live", Description: "Wake up with interviews, music, and lifestyle features.", Category: "Entertainment"},
		{Title: "Talk of the Town", Description: "Celebrity interviews and entertainment news.", Category: "Entertainment"},
		{Title: "Quiz Masters", Description: "Test your knowledge in this exciting game show.", Category: "Entertainment"},
		{Title: "Talent Search", Description: "Discover the next big star in this competition series.", Category: "Entertainment"},
		{Title: "Cooking Challenge", Description: "Chefs compete to create the ultimate dish.", Category: "Entertainment"},
		{Title: "Home Renovation", Description: "Transform living spaces with expert designers.", Category: "Lifestyle"},
		{Title: "Garden Time", Description: "Tips and ideas for your outdoor spaces.", Category: "Lifestyle"},
		{Title: "Travel Journeys", Description: "Explore destinations around the world.", Category: "Lifestyle"},

		// Drama programs
		{Title: "City Hospital", Description: "Drama unfolds in a busy metropolitan medical center.", Category: "Drama"},
		{Title: "Legal Eagles", Description: "Lawyers fight for justice in complex cases.", Category: "Drama"},
		{Title: "Family Matters", Description: "A family navigates the challenges of modern life.", Category: "Drama"},
		{Title: "Crime Division", Description: "Detectives solve mysterious cases in the city.", Category: "Drama"},
		{Title: "Historical Tales", Description: "Period drama set in a bygone era.", Category: "Drama"},

		// Comedy programs
		{Title: "Laugh Track", Description: "Stand-up comedy from emerging talents.", Category: "Comedy"},
		{Title: "Sitcom Central", Description: "Hilarious adventures of quirky characters.", Category: "Comedy"},
		{Title: "Comedy Hour", Description: "The best in sketch comedy and improvisation.", Category: "Comedy"},

		// Sports programs
		{Title: "Sports Central", Description: "All the latest sports news and highlights.", Category: "Sports"},
		{Title: "Match Day", Description: "Live coverage of today's big game.", Category: "Sports"},
		{Title: "Sports Analysis", Description: "Expert commentary and game breakdowns.", Category: "Sports"},
		{Title: "Fitness Focus", Description: "Workout tips and health advice.", Category: "Sports"},
		{Title: "Extreme Sports", Description: "Adrenaline-pumping action sports coverage.", Category: "Sports"},

		// Movies (generic descriptions, no real titles)
		{Title: "Action Feature", Description: "High-octane thrills and explosive excitement.", Category: "Movies"},
		{Title: "Drama Feature", Description: "A compelling story of human triumph.", Category: "Movies"},
		{Title: "Comedy Feature", Description: "Laugh-out-loud entertainment for the whole family.", Category: "Movies"},
		{Title: "Thriller Feature", Description: "Edge-of-your-seat suspense and mystery.", Category: "Movies"},
		{Title: "Romance Feature", Description: "A heartwarming tale of love and connection.", Category: "Movies"},
		{Title: "Sci-Fi Feature", Description: "Journey to new worlds and distant futures.", Category: "Movies"},
		{Title: "Classic Cinema", Description: "Timeless storytelling from the golden age.", Category: "Movies"},

		// Documentary programs
		{Title: "Nature World", Description: "Stunning wildlife and natural wonders.", Category: "Documentary"},
		{Title: "History Uncovered", Description: "Revealing secrets from the past.", Category: "Documentary"},
		{Title: "Science Today", Description: "The latest discoveries and innovations.", Category: "Documentary"},
		{Title: "True Stories", Description: "Real-life accounts of extraordinary events.", Category: "Documentary"},
		{Title: "Ocean Explorer", Description: "Dive into the mysteries of the deep sea.", Category: "Documentary"},

		// Kids programs
		{Title: "Cartoon Time", Description: "Fun animated adventures for young viewers.", Category: "Kids"},
		{Title: "Learning Fun", Description: "Educational entertainment for children.", Category: "Kids"},
		{Title: "Story Corner", Description: "Classic tales brought to life.", Category: "Kids"},
		{Title: "Art Studio", Description: "Creative activities and crafts for kids.", Category: "Kids"},
		{Title: "Animal Friends", Description: "Meet amazing animals from around the world.", Category: "Kids"},

		// Music programs
		{Title: "Music Mix", Description: "The hottest tracks and artist interviews.", Category: "Music"},
		{Title: "Classic Sounds", Description: "Timeless music from legendary artists.", Category: "Music"},
		{Title: "Live Sessions", Description: "Exclusive live performances.", Category: "Music"},
		{Title: "Chart Show", Description: "This week's top music countdown.", Category: "Music"},
	}

	// ProgramDurations contains common program lengths in minutes.
	ProgramDurations = []int{10, 15, 30, 60, 90, 120}
)

// ProgramTemplate represents a template for generating program data.
type ProgramTemplate struct {
	Title       string
	Description string
	Category    string
}

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

// SampleProgram represents a generated sample program for testing.
type SampleProgram struct {
	ChannelID   string
	Title       string
	Description string
	Category    string
	Start       time.Time
	Stop        time.Time
	EpisodeNum  string
	Icon        string
	Rating      string
}

// ToEpgProgram converts a SampleProgram to a models.EpgProgram.
func (s *SampleProgram) ToEpgProgram(sourceID models.ULID) *models.EpgProgram {
	return &models.EpgProgram{
		SourceID:    sourceID,
		ChannelID:   s.ChannelID,
		Title:       s.Title,
		Description: s.Description,
		Category:    s.Category,
		Start:       s.Start,
		Stop:        s.Stop,
		EpisodeNum:  s.EpisodeNum,
		Icon:        s.Icon,
		Rating:      s.Rating,
	}
}

// ProgramGenerateOptions configures program generation.
type ProgramGenerateOptions struct {
	Durations     []int     // Available durations in minutes
	AnchorTime    time.Time // Starting time for programs (will be truncated to hour)
	IconURLBase   string    // Base URL for program icons
	IncludeRating bool      // Whether to include ratings
}

// DefaultProgramGenerateOptions returns default program generation options.
func DefaultProgramGenerateOptions() ProgramGenerateOptions {
	return ProgramGenerateOptions{
		Durations:     ProgramDurations,
		AnchorTime:    time.Now().Add(-1 * time.Hour).Truncate(time.Hour),
		IconURLBase:   "https://icons.example.com/program",
		IncludeRating: true,
	}
}

// Ratings for program content.
var programRatings = []string{"TV-G", "TV-PG", "TV-14", "TV-MA", "G", "PG", "PG-13", "R", ""}

// GenerateProgramsForChannel generates programs for a single channel.
func (g *SampleDataGenerator) GenerateProgramsForChannel(channelID string, count int, opts ProgramGenerateOptions) []SampleProgram {
	programs := make([]SampleProgram, count)
	currentTime := opts.AnchorTime.Truncate(time.Hour)

	for i := 0; i < count; i++ {
		// Pick a random duration
		duration := opts.Durations[g.rng.Intn(len(opts.Durations))]

		// Pick a random program template
		template := ProgramTemplates[g.rng.Intn(len(ProgramTemplates))]

		start := currentTime
		stop := currentTime.Add(time.Duration(duration) * time.Minute)

		// Generate episode number sometimes (50% chance)
		var episodeNum string
		if g.rng.Float32() > 0.5 {
			season := g.rng.Intn(10) + 1
			episode := g.rng.Intn(20) + 1
			episodeNum = fmt.Sprintf("%d.%d.", season-1, episode-1)
		}

		// Generate rating if enabled
		var rating string
		if opts.IncludeRating {
			rating = programRatings[g.rng.Intn(len(programRatings))]
		}

		// Generate icon sometimes (70% chance)
		var icon string
		if g.rng.Float32() > 0.3 {
			icon = fmt.Sprintf("%s/%s_%d.jpg", opts.IconURLBase, channelID, i)
		}

		programs[i] = SampleProgram{
			ChannelID:   channelID,
			Title:       template.Title,
			Description: template.Description,
			Category:    template.Category,
			Start:       start,
			Stop:        stop,
			EpisodeNum:  episodeNum,
			Icon:        icon,
			Rating:      rating,
		}

		currentTime = stop
	}

	return programs
}

// GenerateProgramsForChannels generates programs for multiple channels.
func (g *SampleDataGenerator) GenerateProgramsForChannels(channels []SampleChannel, totalPrograms int, opts ProgramGenerateOptions) []SampleProgram {
	if len(channels) == 0 {
		return nil
	}

	programs := make([]SampleProgram, 0, totalPrograms)
	programsPerChannel := totalPrograms / len(channels)
	extraPrograms := totalPrograms % len(channels)

	for i, ch := range channels {
		count := programsPerChannel
		if i < extraPrograms {
			count++
		}

		// Adjust anchor time for timeshift channels
		channelOpts := opts
		// Check if channel name contains timeshift indicators
		if containsTimeshift(ch.ChannelName) {
			channelOpts.AnchorTime = opts.AnchorTime.Add(-1 * time.Hour)
		}

		channelPrograms := g.GenerateProgramsForChannel(ch.TvgID, count, channelOpts)
		programs = append(programs, channelPrograms...)
	}

	return programs
}

// containsTimeshift checks if a channel name indicates a timeshift channel.
func containsTimeshift(name string) bool {
	timeshiftIndicators := []string{"+1", "+2", "+24", "+1h", "+2h"}
	for _, indicator := range timeshiftIndicators {
		if len(name) >= len(indicator) {
			// Check if the name ends with or contains the indicator
			for i := 0; i <= len(name)-len(indicator); i++ {
				if name[i:i+len(indicator)] == indicator {
					return true
				}
			}
		}
	}
	return false
}
