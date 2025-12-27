package types

// ESSample represents a single elementary stream sample.
type ESSample struct {
	PTS        int64  `json:"pts"`         // Presentation timestamp (90kHz timescale)
	DTS        int64  `json:"dts"`         // Decode timestamp (90kHz timescale)
	Data       []byte `json:"data"`        // NAL unit or audio frame
	IsKeyframe bool   `json:"is_keyframe"` // True for IDR frames / sync points
	Sequence   uint64 `json:"sequence"`    // Sample sequence number for ordering
}

// ESSampleBatch groups samples for efficient transport.
type ESSampleBatch struct {
	VideoSamples  []ESSample `json:"video_samples,omitempty"`
	AudioSamples  []ESSample `json:"audio_samples,omitempty"`
	IsSource      bool       `json:"is_source"`      // true = source samples from coordinator
	BatchSequence uint64     `json:"batch_sequence"` // Batch sequence for ordering
}

// TotalSamples returns the total number of samples in the batch.
func (b *ESSampleBatch) TotalSamples() int {
	return len(b.VideoSamples) + len(b.AudioSamples)
}

// TotalBytes returns the total data bytes in the batch.
func (b *ESSampleBatch) TotalBytes() int {
	total := 0
	for _, s := range b.VideoSamples {
		total += len(s.Data)
	}
	for _, s := range b.AudioSamples {
		total += len(s.Data)
	}
	return total
}

// HasKeyframe returns true if the batch contains a video keyframe.
func (b *ESSampleBatch) HasKeyframe() bool {
	for _, s := range b.VideoSamples {
		if s.IsKeyframe {
			return true
		}
	}
	return false
}
