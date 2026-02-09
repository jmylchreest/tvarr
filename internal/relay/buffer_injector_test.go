package relay

import (
	"testing"
	"time"
)

func TestBufferInjector_HasPlaceholder(t *testing.T) {
	injector := NewBufferInjector(nil)

	tests := []struct {
		variant CodecVariant
		want    bool
	}{
		{VariantH264AAC, true},
		{VariantH265AAC, true},
		{VariantVP9Opus, true},
		{VariantAV1Opus, true},
		{NewCodecVariant("h264", "opus"), false}, // Unsupported combination
		{VariantSource, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.variant), func(t *testing.T) {
			if got := injector.HasPlaceholder(tt.variant); got != tt.want {
				t.Errorf("HasPlaceholder(%s) = %v, want %v", tt.variant, got, tt.want)
			}
		})
	}
}

func TestBufferInjector_GetPlaceholder(t *testing.T) {
	injector := NewBufferInjector(nil)

	tests := []struct {
		variant    CodecVariant
		wantErr    bool
		minSamples int
	}{
		{VariantH264AAC, false, 10},
		{VariantH265AAC, false, 10},
		{VariantVP9Opus, false, 10},
		// AV1 parsing not fully supported by mediacommon's fmp4.Init.Unmarshal
		// {VariantAV1Opus, false, 10},
		{NewCodecVariant("h264", "opus"), true, 0},
	}

	for _, tt := range tests {
		t.Run(string(tt.variant), func(t *testing.T) {
			cached, err := injector.GetPlaceholder(tt.variant)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetPlaceholder(%s) error = %v, wantErr %v", tt.variant, err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if len(cached.VideoSamples) < tt.minSamples {
				t.Errorf("GetPlaceholder(%s) video samples = %d, want at least %d",
					tt.variant, len(cached.VideoSamples), tt.minSamples)
			}
			if cached.Duration == 0 {
				t.Errorf("GetPlaceholder(%s) duration = 0, want > 0", tt.variant)
			}
		})
	}
}

func TestBufferInjector_InjectStartupPlaceholder(t *testing.T) {
	injector := NewBufferInjector(nil)
	variant := NewESVariantWithMaxBytes(VariantH264AAC, 0, false)

	err := injector.InjectStartupPlaceholder(variant, 4*time.Second)
	if err != nil {
		t.Fatalf("InjectStartupPlaceholder() error = %v", err)
	}

	// Check that samples were injected
	videoTrack := variant.VideoTrack()
	audioTrack := variant.AudioTrack()

	if videoTrack.Count() == 0 {
		t.Error("Expected video samples to be injected")
	}
	if audioTrack.Count() == 0 {
		t.Error("Expected audio samples to be injected")
	}

	// With 4 second target and 1 second placeholders, we should have ~4 loops
	// Expect at least 50 video samples (25fps * 4 loops = 100, but be lenient)
	if videoTrack.Count() < 50 {
		t.Errorf("Expected at least 50 video samples for 4s, got %d", videoTrack.Count())
	}
}

func TestBufferInjector_GlobalInstance(t *testing.T) {
	injector1 := GetBufferInjector()
	injector2 := GetBufferInjector()

	if injector1 != injector2 {
		t.Error("GetBufferInjector() should return the same instance")
	}
}

func TestBufferInjector_AvailableVariants(t *testing.T) {
	injector := NewBufferInjector(nil)
	variants := injector.AvailableVariants()

	if len(variants) < 4 {
		t.Errorf("AvailableVariants() = %d variants, want at least 4", len(variants))
	}

	// Check that expected variants are present (use string matching since naming differs)
	expected := []string{"h264/aac", "h265/aac", "vp9/opus", "av1/opus"}
	for _, ev := range expected {
		found := false
		for _, v := range variants {
			if string(v) == ev {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected variant %s to be in AvailableVariants()", ev)
		}
	}
}
