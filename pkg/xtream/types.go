package xtream

import (
	"encoding/json"
	"strconv"
	"time"
)

// AuthInfo contains the combined server and user information returned by the API.
type AuthInfo struct {
	UserInfo   UserInfo   `json:"user_info"`
	ServerInfo ServerInfo `json:"server_info"`
}

// UserInfo contains user account information.
type UserInfo struct {
	Username             string   `json:"username"`
	Password             string   `json:"password"`
	Message              string   `json:"message"`
	Auth                 FlexInt  `json:"auth"`
	Status               string   `json:"status"`
	ExpDate              FlexInt  `json:"exp_date"`
	IsTrial              FlexInt  `json:"is_trial"`
	ActiveConnections    FlexInt  `json:"active_cons"`
	CreatedAt            FlexInt  `json:"created_at"`
	MaxConnections       FlexInt  `json:"max_connections"`
	AllowedOutputFormats []string `json:"allowed_output_formats"`
}

// IsAuthenticated returns true if the user is authenticated.
func (u *UserInfo) IsAuthenticated() bool {
	return u.Auth.Int() == 1 && u.Status == "Active"
}

// ExpirationTime returns the account expiration time.
func (u *UserInfo) ExpirationTime() time.Time {
	if u.ExpDate.Int() == 0 {
		return time.Time{}
	}
	return time.Unix(u.ExpDate.Int(), 0)
}

// IsExpired returns true if the account has expired.
func (u *UserInfo) IsExpired() bool {
	exp := u.ExpirationTime()
	if exp.IsZero() {
		return false
	}
	return time.Now().After(exp)
}

// ServerInfo contains server configuration information.
type ServerInfo struct {
	URL            string  `json:"url"`
	Port           FlexInt `json:"port"`
	HTTPSPort      FlexInt `json:"https_port"`
	ServerProtocol string  `json:"server_protocol"`
	RTMPPort       FlexInt `json:"rtmp_port"`
	Timezone       string  `json:"timezone"`
	TimestampNow   FlexInt `json:"timestamp_now"`
	TimeNow        string  `json:"time_now"`
	Process        bool    `json:"process"`
}

// Category represents a content category.
type Category struct {
	CategoryID   FlexString `json:"category_id"`
	CategoryName string     `json:"category_name"`
	ParentID     FlexInt    `json:"parent_id"`
}

// Stream represents a live stream.
type Stream struct {
	Num                FlexInt    `json:"num"`
	Name               string     `json:"name"`
	StreamType         string     `json:"stream_type"`
	StreamID           FlexInt    `json:"stream_id"`
	StreamIcon         string     `json:"stream_icon"`
	EPGChannelID       string     `json:"epg_channel_id"`
	Added              FlexInt    `json:"added"`
	IsAdult            FlexInt    `json:"is_adult"`
	CategoryID         FlexString `json:"category_id"`
	CategoryIDs        []FlexInt  `json:"category_ids"`
	CustomSID          string     `json:"custom_sid"`
	TVArchive          FlexInt    `json:"tv_archive"`
	DirectSource       string     `json:"direct_source"`
	TVArchiveDays      FlexInt    `json:"tv_archive_duration"`
	ContainerExtension string     `json:"container_extension,omitempty"`
}

// AddedTime returns the time the stream was added.
func (s *Stream) AddedTime() time.Time {
	if s.Added.Int() == 0 {
		return time.Time{}
	}
	return time.Unix(s.Added.Int(), 0)
}

// VODStream represents a video on demand item.
type VODStream struct {
	Num                FlexInt    `json:"num"`
	Name               string     `json:"name"`
	StreamType         string     `json:"stream_type"`
	StreamID           FlexInt    `json:"stream_id"`
	StreamIcon         string     `json:"stream_icon"`
	Rating             FlexFloat  `json:"rating"`
	RatingVotes        FlexInt    `json:"rating_5based"`
	Added              FlexInt    `json:"added"`
	IsAdult            FlexInt    `json:"is_adult"`
	CategoryID         FlexString `json:"category_id"`
	CategoryIDs        []FlexInt  `json:"category_ids"`
	ContainerExtension string     `json:"container_extension"`
	CustomSID          string     `json:"custom_sid"`
	DirectSource       string     `json:"direct_source"`
}

// VODInfo contains detailed information about a VOD item.
type VODInfo struct {
	Info      VODInfoDetails `json:"info"`
	MovieData VODStream      `json:"movie_data"`
}

// VODInfoDetails contains the detailed metadata for a VOD item.
type VODInfoDetails struct {
	MovieImage     string    `json:"movie_image"`
	TMDBId         FlexInt   `json:"tmdb_id"`
	Backdrop       string    `json:"backdrop_path"`
	YoutubeTrailer string    `json:"youtube_trailer"`
	Genre          string    `json:"genre"`
	Plot           string    `json:"plot"`
	Cast           string    `json:"cast"`
	Rating         FlexFloat `json:"rating"`
	Director       string    `json:"director"`
	ReleaseDate    string    `json:"releasedate"`
	BackdropPath   []string  `json:"backdrop_path_list"`
	Duration       string    `json:"duration"`
	DurationSecs   FlexInt   `json:"duration_secs"`
	Bitrate        FlexInt   `json:"bitrate"`
	Video          VideoInfo `json:"video"`
	Audio          AudioInfo `json:"audio"`
}

// VideoInfo contains video codec information.
type VideoInfo struct {
	Index     int    `json:"index"`
	CodecName string `json:"codec_name"`
	CodecType string `json:"codec_type"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

// AudioInfo contains audio codec information.
type AudioInfo struct {
	Index      int    `json:"index"`
	CodecName  string `json:"codec_name"`
	CodecType  string `json:"codec_type"`
	Channels   int    `json:"channels"`
	SampleRate string `json:"sample_rate"`
}

// Series represents a TV series.
type Series struct {
	Num            FlexInt    `json:"num"`
	Name           string     `json:"name"`
	SeriesID       FlexInt    `json:"series_id"`
	Cover          string     `json:"cover"`
	Plot           string     `json:"plot"`
	Cast           string     `json:"cast"`
	Director       string     `json:"director"`
	Genre          string     `json:"genre"`
	ReleaseDate    string     `json:"releaseDate"`
	LastModified   FlexInt    `json:"last_modified"`
	Rating         FlexFloat  `json:"rating"`
	RatingVotes    FlexInt    `json:"rating_5based"`
	BackdropPath   []string   `json:"backdrop_path"`
	YoutubeTrailer string     `json:"youtube_trailer"`
	TMDBId         FlexInt    `json:"tmdb_id"`
	CategoryID     FlexString `json:"category_id"`
	CategoryIDs    []FlexInt  `json:"category_ids"`
}

// SeriesInfo contains detailed information about a series including episodes.
type SeriesInfo struct {
	Seasons  []SeasonInfo         `json:"seasons"`
	Info     SeriesInfoDetails    `json:"info"`
	Episodes map[string][]Episode `json:"episodes"`
}

// SeasonInfo contains information about a season.
type SeasonInfo struct {
	AirDate      string `json:"air_date"`
	EpisodeCount int    `json:"episode_count"`
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Overview     string `json:"overview"`
	SeasonNumber int    `json:"season_number"`
	Cover        string `json:"cover"`
	CoverBig     string `json:"cover_big"`
}

// SeriesInfoDetails contains the series metadata.
type SeriesInfoDetails struct {
	Name           string     `json:"name"`
	Cover          string     `json:"cover"`
	Plot           string     `json:"plot"`
	Cast           string     `json:"cast"`
	Director       string     `json:"director"`
	Genre          string     `json:"genre"`
	ReleaseDate    string     `json:"releaseDate"`
	LastModified   FlexInt    `json:"last_modified"`
	Rating         FlexFloat  `json:"rating"`
	RatingVotes    FlexInt    `json:"rating_5based"`
	BackdropPath   []string   `json:"backdrop_path"`
	YoutubeTrailer string     `json:"youtube_trailer"`
	TMDBId         FlexInt    `json:"tmdb_id"`
	EpisodeRunTime string     `json:"episode_run_time"`
	CategoryID     FlexString `json:"category_id"`
	CategoryIDs    []FlexInt  `json:"category_ids"`
}

// Episode represents a single episode in a series.
type Episode struct {
	ID                 FlexInt     `json:"id"`
	EpisodeNum         FlexInt     `json:"episode_num"`
	Title              string      `json:"title"`
	ContainerExtension string      `json:"container_extension"`
	Info               EpisodeInfo `json:"info"`
	CustomSID          string      `json:"custom_sid"`
	Added              FlexInt     `json:"added"`
	Season             FlexInt     `json:"season"`
	DirectSource       string      `json:"direct_source"`
}

// EpisodeInfo contains episode metadata.
type EpisodeInfo struct {
	MovieImage   string    `json:"movie_image"`
	Plot         string    `json:"plot"`
	ReleaseDate  string    `json:"releasedate"`
	Rating       FlexFloat `json:"rating"`
	Duration     string    `json:"duration"`
	DurationSecs FlexInt   `json:"duration_secs"`
	Bitrate      FlexInt   `json:"bitrate"`
	Video        VideoInfo `json:"video"`
	Audio        AudioInfo `json:"audio"`
}

// EPGListing represents a single EPG entry.
type EPGListing struct {
	ID             FlexString `json:"id"`
	EPGId          FlexString `json:"epg_id"`
	Title          string     `json:"title"`
	Lang           string     `json:"lang"`
	Start          string     `json:"start"`
	End            string     `json:"end"`
	Description    string     `json:"description"`
	ChannelID      string     `json:"channel_id"`
	StartTimestamp FlexInt    `json:"start_timestamp"`
	StopTimestamp  FlexInt    `json:"stop_timestamp"`
	NowPlaying     FlexInt    `json:"now_playing"`
	HasArchive     FlexInt    `json:"has_archive"`
}

// StartTime returns the program start time.
func (e *EPGListing) StartTime() time.Time {
	if e.StartTimestamp.Int() > 0 {
		return time.Unix(e.StartTimestamp.Int(), 0)
	}
	// Try parsing the start string
	if t, err := time.Parse("2006-01-02 15:04:05", e.Start); err == nil {
		return t
	}
	return time.Time{}
}

// EndTime returns the program end time.
func (e *EPGListing) EndTime() time.Time {
	if e.StopTimestamp.Int() > 0 {
		return time.Unix(e.StopTimestamp.Int(), 0)
	}
	// Try parsing the end string
	if t, err := time.Parse("2006-01-02 15:04:05", e.End); err == nil {
		return t
	}
	return time.Time{}
}

// EPGResponse wraps the EPG listings response.
type EPGResponse struct {
	EPGListings []EPGListing `json:"epg_listings"`
}

// FlexInt handles JSON numbers that may be strings or integers.
type FlexInt int64

// Int returns the integer value.
func (f FlexInt) Int() int64 {
	return int64(f)
}

// UnmarshalJSON handles both string and number JSON values.
func (f *FlexInt) UnmarshalJSON(data []byte) error {
	// Try as number first
	var n int64
	if err := json.Unmarshal(data, &n); err == nil {
		*f = FlexInt(n)
		return nil
	}

	// Try as string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s == "" {
			*f = 0
			return nil
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			*f = 0
			return nil
		}
		*f = FlexInt(n)
		return nil
	}

	*f = 0
	return nil
}

// FlexFloat handles JSON numbers that may be strings or floats.
type FlexFloat float64

// Float returns the float value.
func (f FlexFloat) Float() float64 {
	return float64(f)
}

// UnmarshalJSON handles both string and number JSON values.
func (f *FlexFloat) UnmarshalJSON(data []byte) error {
	// Try as number first
	var n float64
	if err := json.Unmarshal(data, &n); err == nil {
		*f = FlexFloat(n)
		return nil
	}

	// Try as string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		if s == "" {
			*f = 0
			return nil
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			*f = 0
			return nil
		}
		*f = FlexFloat(n)
		return nil
	}

	*f = 0
	return nil
}

// FlexString handles JSON values that may be strings or numbers.
type FlexString string

// String returns the string value.
func (f FlexString) String() string {
	return string(f)
}

// UnmarshalJSON handles both string and number JSON values.
func (f *FlexString) UnmarshalJSON(data []byte) error {
	// Try as string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = FlexString(s)
		return nil
	}

	// Try as number
	var n json.Number
	if err := json.Unmarshal(data, &n); err == nil {
		*f = FlexString(n.String())
		return nil
	}

	*f = ""
	return nil
}
