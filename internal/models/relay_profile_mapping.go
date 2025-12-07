package models

// RelayProfileMapping defines client detection rules for automatic codec selection.
// When a RelayProfile uses VideoCodecAuto or AudioCodecAuto, these mappings
// are evaluated in priority order to determine the actual codecs to use.
type RelayProfileMapping struct {
	BaseModel

	// Identity and description
	Name        string `json:"name" gorm:"uniqueIndex;not null;size:100"`
	Description string `json:"description,omitempty" gorm:"size:500"`

	// Priority for rule evaluation (lower = higher priority, first match wins)
	Priority int `json:"priority" gorm:"default:0;index"`

	// Expression for matching request context (User-Agent, IP, headers, etc.)
	// Uses the same expression engine as filters
	Expression string `json:"expression" gorm:"type:text;not null"`

	// Rule state
	IsEnabled bool `json:"is_enabled" gorm:"default:true"`
	IsSystem  bool `json:"is_system" gorm:"default:false"` // System rules cannot be deleted

	// Codec arrays - source codecs that can be passed through (copied) without transcoding
	// If the source stream's codec is in this list, it will be copied; otherwise transcoded
	AcceptedVideoCodecs PqStringArray `json:"accepted_video_codecs" gorm:"type:text;serializer:json"`
	AcceptedAudioCodecs PqStringArray `json:"accepted_audio_codecs" gorm:"type:text;serializer:json"`

	// Container formats that can be passed through
	AcceptedContainers PqStringArray `json:"accepted_containers" gorm:"type:text;serializer:json"`

	// Preferred transcode targets (used when source codec is not in accepted list)
	PreferredVideoCodec  VideoCodec      `json:"preferred_video_codec" gorm:"default:'h265'"`
	PreferredAudioCodec  AudioCodec      `json:"preferred_audio_codec" gorm:"default:'aac'"`
	PreferredContainer   ContainerFormat `json:"preferred_container" gorm:"default:'fmp4'"`
}

// PqStringArray is a helper type for storing string arrays in the database
type PqStringArray []string

// TableName returns the table name for GORM
func (RelayProfileMapping) TableName() string {
	return "relay_profile_mappings"
}

// AcceptsVideoCodec returns true if the given codec is in the accepted list
func (m *RelayProfileMapping) AcceptsVideoCodec(codec VideoCodec) bool {
	for _, accepted := range m.AcceptedVideoCodecs {
		if VideoCodec(accepted) == codec {
			return true
		}
	}
	return false
}

// AcceptsAudioCodec returns true if the given codec is in the accepted list
func (m *RelayProfileMapping) AcceptsAudioCodec(codec AudioCodec) bool {
	for _, accepted := range m.AcceptedAudioCodecs {
		if AudioCodec(accepted) == codec {
			return true
		}
	}
	return false
}

// AcceptsContainer returns true if the given container is in the accepted list
func (m *RelayProfileMapping) AcceptsContainer(container ContainerFormat) bool {
	for _, accepted := range m.AcceptedContainers {
		if ContainerFormat(accepted) == container {
			return true
		}
	}
	return false
}

// GetVideoCodecFor returns the video codec to use based on source codec.
// If source is accepted, returns "copy"; otherwise returns preferred codec.
func (m *RelayProfileMapping) GetVideoCodecFor(sourceCodec VideoCodec) VideoCodec {
	if m.AcceptsVideoCodec(sourceCodec) {
		return VideoCodecCopy
	}
	return m.PreferredVideoCodec
}

// GetAudioCodecFor returns the audio codec to use based on source codec.
// If source is accepted, returns "copy"; otherwise returns preferred codec.
func (m *RelayProfileMapping) GetAudioCodecFor(sourceCodec AudioCodec) AudioCodec {
	if m.AcceptsAudioCodec(sourceCodec) {
		return AudioCodecCopy
	}
	return m.PreferredAudioCodec
}

// GetContainerFor returns the container format to use based on source container.
// If source is accepted, returns "auto"; otherwise returns preferred container.
func (m *RelayProfileMapping) GetContainerFor(sourceContainer ContainerFormat) ContainerFormat {
	if m.AcceptsContainer(sourceContainer) {
		return ContainerFormatAuto
	}
	return m.PreferredContainer
}

// VideoCodecStrings returns accepted video codecs as VideoCodec slice
func (m *RelayProfileMapping) VideoCodecStrings() []VideoCodec {
	result := make([]VideoCodec, len(m.AcceptedVideoCodecs))
	for i, s := range m.AcceptedVideoCodecs {
		result[i] = VideoCodec(s)
	}
	return result
}

// AudioCodecStrings returns accepted audio codecs as AudioCodec slice
func (m *RelayProfileMapping) AudioCodecStrings() []AudioCodec {
	result := make([]AudioCodec, len(m.AcceptedAudioCodecs))
	for i, s := range m.AcceptedAudioCodecs {
		result[i] = AudioCodec(s)
	}
	return result
}

// SetAcceptedVideoCodecs sets accepted video codecs from VideoCodec slice
func (m *RelayProfileMapping) SetAcceptedVideoCodecs(codecs []VideoCodec) {
	m.AcceptedVideoCodecs = make([]string, len(codecs))
	for i, c := range codecs {
		m.AcceptedVideoCodecs[i] = string(c)
	}
}

// SetAcceptedAudioCodecs sets accepted audio codecs from AudioCodec slice
func (m *RelayProfileMapping) SetAcceptedAudioCodecs(codecs []AudioCodec) {
	m.AcceptedAudioCodecs = make([]string, len(codecs))
	for i, c := range codecs {
		m.AcceptedAudioCodecs[i] = string(c)
	}
}

// SetAcceptedContainers sets accepted containers from ContainerFormat slice
func (m *RelayProfileMapping) SetAcceptedContainers(containers []ContainerFormat) {
	m.AcceptedContainers = make([]string, len(containers))
	for i, c := range containers {
		m.AcceptedContainers[i] = string(c)
	}
}
