package relay

import (
	"testing"
)

func TestPlaceholderSegmentGenerator_New(t *testing.T) {
	tests := []struct {
		variant CodecVariant
		wantErr bool
	}{
		{VariantH264AAC, false},
		{VariantH265AAC, false},
		{VariantVP9Opus, false},
		// AV1 parsing not fully supported
		{NewCodecVariant("h264", "opus"), true}, // Unsupported combination
		{VariantCopy, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.variant), func(t *testing.T) {
			gen, err := NewPlaceholderSegmentGenerator(tt.variant, 4.0, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewPlaceholderSegmentGenerator(%s) error = %v, wantErr %v", tt.variant, err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Check init segment was generated
			initSeg := gen.GetInitSegment()
			if initSeg == nil {
				t.Fatalf("GetInitSegment() returned nil")
			}
			if len(initSeg.Data) == 0 {
				t.Errorf("GetInitSegment() returned empty data")
			}
		})
	}
}

func TestPlaceholderSegmentGenerator_GenerateSegment(t *testing.T) {
	gen, err := NewPlaceholderSegmentGenerator(VariantH264AAC, 4.0, nil)
	if err != nil {
		t.Fatalf("NewPlaceholderSegmentGenerator() error = %v", err)
	}

	// Generate segments with different sequence numbers
	sequences := []uint64{0, 1, 5, 10}
	for _, seq := range sequences {
		t.Run(string(rune('0'+seq)), func(t *testing.T) {
			seg, err := gen.GenerateSegment(seq)
			if err != nil {
				t.Fatalf("GenerateSegment(%d) error = %v", seq, err)
			}

			if seg.Sequence != seq {
				t.Errorf("GenerateSegment(%d).Sequence = %d, want %d", seq, seg.Sequence, seq)
			}
			if seg.Duration < 3.0 || seg.Duration > 5.0 {
				t.Errorf("GenerateSegment(%d).Duration = %f, want ~4.0", seq, seg.Duration)
			}
			if len(seg.Data) == 0 {
				t.Errorf("GenerateSegment(%d).Data is empty", seq)
			}
			if !seg.IsFragmented {
				t.Errorf("GenerateSegment(%d).IsFragmented = false, want true", seq)
			}
		})
	}
}

func TestPlaceholderSegmentGenerator_MultipleVariants(t *testing.T) {
	variants := []CodecVariant{VariantH264AAC, VariantH265AAC, VariantVP9Opus}

	for _, variant := range variants {
		t.Run(string(variant), func(t *testing.T) {
			gen, err := NewPlaceholderSegmentGenerator(variant, 4.0, nil)
			if err != nil {
				t.Fatalf("NewPlaceholderSegmentGenerator(%s) error = %v", variant, err)
			}

			// Generate a few segments
			for seq := uint64(0); seq < 3; seq++ {
				seg, err := gen.GenerateSegment(seq)
				if err != nil {
					t.Fatalf("GenerateSegment(%d) error = %v", seq, err)
				}
				if len(seg.Data) == 0 {
					t.Errorf("Segment %d data is empty", seq)
				}
			}
		})
	}
}
