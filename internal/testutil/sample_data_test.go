package testutil

import (
	"strings"
	"testing"

	"github.com/jmylchreest/tvarr/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSampleDataGenerator(t *testing.T) {
	gen := NewSampleDataGenerator()
	require.NotNil(t, gen)
	require.NotNil(t, gen.rng)
}

func TestNewSampleDataGeneratorWithSeed(t *testing.T) {
	gen1 := NewSampleDataGeneratorWithSeed(42)
	gen2 := NewSampleDataGeneratorWithSeed(42)

	// Same seed should produce same results
	assert.Equal(t, gen1.RandomBroadcaster(), gen2.RandomBroadcaster())
}

func TestRandomBroadcaster(t *testing.T) {
	gen := NewSampleDataGenerator()

	for i := 0; i < 10; i++ {
		broadcaster := gen.RandomBroadcaster()
		assert.NotEmpty(t, broadcaster)
		assert.Contains(t, Broadcasters, broadcaster)
	}
}

func TestRandomQuality(t *testing.T) {
	gen := NewSampleDataGenerator()

	for i := 0; i < 10; i++ {
		quality := gen.RandomQuality()
		assert.NotEmpty(t, quality)
		assert.Contains(t, QualityVariants, quality)
	}
}

func TestRandomTimeshift(t *testing.T) {
	gen := NewSampleDataGenerator()

	for i := 0; i < 10; i++ {
		timeshift := gen.RandomTimeshift()
		assert.NotEmpty(t, timeshift)
		assert.Contains(t, TimeshiftVariants, timeshift)
	}
}

func TestGenerateChannelName(t *testing.T) {
	gen := NewSampleDataGenerator()

	tests := []struct {
		category      string
		expectedParts []string
	}{
		{"sports", []string{"Sports", "Racing", "Football"}},
		{"news", []string{"News", "Breaking", "World", "Local"}},
		{"movies", []string{"Movies", "Action", "Classic", "Cinema"}},
	}

	for _, tt := range tests {
		t.Run(tt.category, func(t *testing.T) {
			name := gen.GenerateChannelName(tt.category)
			assert.NotEmpty(t, name)

			// Should contain a broadcaster
			hasBroadcaster := false
			for _, b := range Broadcasters {
				if strings.Contains(name, b) {
					hasBroadcaster = true
					break
				}
			}
			assert.True(t, hasBroadcaster, "Channel name should contain a broadcaster: %s", name)

			// Should contain category-related text
			hasCategory := false
			for _, p := range tt.expectedParts {
				if strings.Contains(name, p) {
					hasCategory = true
					break
				}
			}
			assert.True(t, hasCategory, "Channel name should contain category text: %s", name)
		})
	}
}

func TestGenerateTimeshiftChannelName(t *testing.T) {
	gen := NewSampleDataGenerator()

	name := gen.GenerateTimeshiftChannelName("sports")
	assert.NotEmpty(t, name)

	// Should contain a timeshift variant
	hasTimeshift := false
	for _, ts := range TimeshiftVariants {
		if strings.Contains(name, ts) {
			hasTimeshift = true
			break
		}
	}
	assert.True(t, hasTimeshift, "Channel name should contain timeshift: %s", name)
}

func TestGenerateSampleChannels(t *testing.T) {
	gen := NewSampleDataGenerator()
	opts := DefaultGenerateOptions()
	opts.Category = "sports"
	opts.TimeshiftRatio = 0.0 // No timeshift for predictable testing

	channels := gen.GenerateSampleChannels(10, opts)
	assert.Len(t, channels, 10)

	for i, ch := range channels {
		// Verify TvgID format
		assert.Regexp(t, `^ch\d{3}$`, ch.TvgID)
		assert.Equal(t, 101+i, ch.TvgChNo)
		assert.NotEmpty(t, ch.ChannelName)
		assert.NotEmpty(t, ch.StreamURL)
		assert.Contains(t, ch.StreamURL, "example.com")
		assert.NotEmpty(t, ch.TvgLogo)
		assert.Contains(t, ch.TvgLogo, "example.com")
		assert.Equal(t, "Sports", ch.GroupTitle)
		assert.False(t, ch.IsAdult)
	}
}

func TestGenerateSportsChannels(t *testing.T) {
	gen := NewSampleDataGenerator()
	channels := gen.GenerateSportsChannels(5)

	assert.Len(t, channels, 5)
	for _, ch := range channels {
		assert.Equal(t, "Sports", ch.GroupTitle)
	}
}

func TestGenerateNewsChannels(t *testing.T) {
	gen := NewSampleDataGenerator()
	channels := gen.GenerateNewsChannels(5)

	assert.Len(t, channels, 5)
	for _, ch := range channels {
		assert.Equal(t, "News", ch.GroupTitle)
	}
}

func TestGenerateAdultChannels(t *testing.T) {
	gen := NewSampleDataGenerator()
	channels := gen.GenerateAdultChannels(3)

	assert.Len(t, channels, 3)
	for _, ch := range channels {
		assert.Equal(t, "Adult", ch.GroupTitle)
		assert.True(t, ch.IsAdult)
	}
}

func TestGenerateTimeshiftChannels(t *testing.T) {
	gen := NewSampleDataGenerator()
	channels := gen.GenerateTimeshiftChannels(5, "news")

	assert.Len(t, channels, 5)
	for _, ch := range channels {
		hasTimeshift := false
		for _, ts := range TimeshiftVariants {
			if strings.Contains(ch.ChannelName, ts) {
				hasTimeshift = true
				break
			}
		}
		assert.True(t, hasTimeshift, "Channel should have timeshift: %s", ch.ChannelName)
	}
}

func TestGenerateStandardChannels(t *testing.T) {
	gen := NewSampleDataGenerator()
	channels := gen.GenerateStandardChannels(5, "movies")

	assert.Len(t, channels, 5)
	for _, ch := range channels {
		hasTimeshift := false
		for _, ts := range TimeshiftVariants {
			if strings.Contains(ch.ChannelName, ts) {
				hasTimeshift = true
				break
			}
		}
		assert.False(t, hasTimeshift, "Channel should not have timeshift: %s", ch.ChannelName)
	}
}

func TestGenerateMixedChannels(t *testing.T) {
	gen := NewSampleDataGenerator()
	channels := gen.GenerateMixedChannels(20)

	assert.Len(t, channels, 20)

	groupTitles := make(map[string]int)
	for _, ch := range channels {
		groupTitles[ch.GroupTitle]++
	}

	// Should have some variety (at least 2 different categories in 20 channels)
	assert.GreaterOrEqual(t, len(groupTitles), 2, "Should have variety in categories")
}

func TestSampleChannelToChannel(t *testing.T) {
	sample := SampleChannel{
		TvgID:       "ch001",
		TvgName:     "StreamCast Sports HD",
		TvgChNo:     101,
		ChannelName: "StreamCast Sports HD",
		TvgLogo:     "https://logos.example.com/channel1.png",
		GroupTitle:  "Sports",
		StreamURL:   "https://stream.example.com/channel1",
		StreamType:  "live",
		Language:    "en",
		Country:     "US",
		IsAdult:     false,
	}

	sourceID := models.NewULID()
	channel := sample.ToChannel(sourceID)

	assert.Equal(t, sourceID, channel.SourceID)
	assert.Equal(t, "ch001", channel.ExtID)
	assert.Equal(t, "ch001", channel.TvgID)
	assert.Equal(t, "StreamCast Sports HD", channel.TvgName)
	assert.Equal(t, "StreamCast Sports HD", channel.ChannelName)
	assert.Equal(t, 101, channel.ChannelNumber)
	assert.Equal(t, "https://logos.example.com/channel1.png", channel.TvgLogo)
	assert.Equal(t, "Sports", channel.GroupTitle)
	assert.Equal(t, "https://stream.example.com/channel1", channel.StreamURL)
	assert.Equal(t, "live", channel.StreamType)
	assert.Equal(t, "en", channel.Language)
	assert.Equal(t, "US", channel.Country)
	assert.False(t, channel.IsAdult)
}

func TestNoRealBrandNames(t *testing.T) {
	// This test ensures we never accidentally include real brand names
	// Only check brands that are likely to appear as whole words in channel names
	realBrands := []string{
		"BBC", "CNN", "ESPN", "HBO", "Sky", "Fox", "NBC", "CBS",
		"Netflix", "Disney", "Paramount", "Discovery", "MTV",
	}

	// Check that broadcasters don't exactly match real brands
	for _, brand := range Broadcasters {
		for _, real := range realBrands {
			assert.NotEqual(t, strings.ToUpper(brand), strings.ToUpper(real),
				"Broadcaster should not be a real brand: %s", real)
		}
	}

	// Check generated channel names don't start with real brand names
	gen := NewSampleDataGenerator()
	for i := 0; i < 100; i++ {
		name := gen.GenerateChannelName("entertainment")
		words := strings.Fields(name)
		if len(words) > 0 {
			firstWord := strings.ToUpper(words[0])
			for _, real := range realBrands {
				assert.NotEqual(t, firstWord, strings.ToUpper(real),
					"Generated channel name should not start with real brand: %s in %s", real, name)
			}
		}
	}
}
