package xc

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// FlexString accepts a JSON string or number. XC panels mix both freely.
type FlexString string

func (f *FlexString) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		*f = ""
		return nil
	}
	if len(data) > 0 && data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*f = FlexString(strings.TrimSpace(value))
		return nil
	}
	if len(data) > 0 && data[0] == '[' {
		var values []any
		if err := json.Unmarshal(data, &values); err != nil {
			*f = ""
			return nil
		}
		for _, value := range values {
			text := strings.TrimSpace(toString(value))
			if text != "" {
				*f = FlexString(text)
				return nil
			}
		}
		*f = ""
		return nil
	}
	if len(data) > 0 && data[0] == '{' {
		*f = ""
		return nil
	}
	*f = FlexString(strings.Trim(string(data), `"`))
	return nil
}

func (f FlexString) String() string { return string(f) }

type FlexInt int

func (f *FlexInt) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		*f = 0
		return nil
	}
	if len(data) > 0 && data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		value = strings.TrimSpace(value)
		if value == "" {
			*f = 0
			return nil
		}
		parsed, err := strconv.Atoi(value)
		if err != nil {
			parsedFloat, ferr := strconv.ParseFloat(value, 64)
			if ferr != nil {
				*f = 0
				return nil
			}
			parsed = int(parsedFloat)
		}
		*f = FlexInt(parsed)
		return nil
	}
	parsed, err := strconv.Atoi(string(data))
	if err != nil {
		parsedFloat, ferr := strconv.ParseFloat(string(data), 64)
		if ferr != nil {
			*f = 0
			return nil
		}
		parsed = int(parsedFloat)
	}
	*f = FlexInt(parsed)
	return nil
}

func (f FlexInt) Int() int { return int(f) }

type Account struct {
	UserInfo   UserInfo   `json:"user_info"`
	ServerInfo ServerInfo `json:"server_info"`
}

type UserInfo struct {
	Username       string     `json:"username"`
	Password       string     `json:"password"`
	Message        string     `json:"message"`
	Auth           FlexInt    `json:"auth"`
	Status         string     `json:"status"`
	ExpDate        FlexString `json:"exp_date"`
	IsTrial        FlexString `json:"is_trial"`
	ActiveCons     FlexString `json:"active_cons"`
	CreatedAt      FlexString `json:"created_at"`
	MaxConnections FlexString `json:"max_connections"`
	AllowedFormats string     `json:"allowed_output_formats"`
}

type ServerInfo struct {
	URL                  string     `json:"url"`
	Port                 FlexString `json:"port"`
	HTTPSPort            FlexString `json:"https_port"`
	ServerProtocol       string     `json:"server_protocol"`
	Timezone             string     `json:"timezone"`
	ServerSoftware       string     `json:"server_software"`
	XUI                  FlexString `json:"xui"`
	AllowedOutputFormats []string   `json:"allowed_output_formats"`
	TimestampNow         FlexString `json:"timestamp_now"`
	TimeNow              string     `json:"time_now"`
}

type Category struct {
	CategoryID   FlexString `json:"category_id"`
	CategoryName string     `json:"category_name"`
	ParentID     FlexString `json:"parent_id"`
}

type LiveStream struct {
	Num           FlexString `json:"num"`
	Name          string     `json:"name"`
	StreamID      FlexString `json:"stream_id"`
	StreamIcon    FlexString `json:"stream_icon"`
	EPGChannelID  string     `json:"epg_channel_id"`
	CategoryID    FlexString `json:"category_id"`
	CategoryIDs   any        `json:"category_ids"`
	TVArchive     FlexInt    `json:"tv_archive"`
	TVArchiveDura FlexInt    `json:"tv_archive_duration"`
	StreamType    string     `json:"stream_type"`
	DirectSource  string     `json:"direct_source"`
}

type VODStream struct {
	Num                FlexString `json:"num"`
	Name               string     `json:"name"`
	StreamID           FlexString `json:"stream_id"`
	StreamIcon         FlexString `json:"stream_icon"`
	CategoryID         FlexString `json:"category_id"`
	CategoryIDs        any        `json:"category_ids"`
	ContainerExtension string     `json:"container_extension"`
	Rating             FlexString `json:"rating"`
	Rating5Based       FlexString `json:"rating_5based"`
	Added              FlexString `json:"added"`
	TMDB               FlexString `json:"tmdb"`
	Trailer            string     `json:"trailer"`
	IsAdult            FlexString `json:"is_adult"`
	DirectSource       string     `json:"direct_source"`
}

type SeriesItem struct {
	Num            FlexString `json:"num"`
	Name           string     `json:"name"`
	SeriesID       FlexString `json:"series_id"`
	Cover          FlexString `json:"cover"`
	BackdropPath   FlexString `json:"backdrop_path"`
	CategoryID     FlexString `json:"category_id"`
	CategoryIDs    any        `json:"category_ids"`
	Plot           FlexString `json:"plot"`
	Genre          FlexString `json:"genre"`
	Rating         FlexString `json:"rating"`
	ReleaseDate    string     `json:"release_date"`
	ReleaseDateAlt string     `json:"releaseDate"`
	TMDB           FlexString `json:"tmdb"`
	LastModified   FlexString `json:"last_modified"`
	EpisodeRunTime FlexString `json:"episode_run_time"`
	Cast           string     `json:"cast"`
	Director       string     `json:"director"`
	YouTubeTrailer string     `json:"youtube_trailer"`
}

type SeriesInfo struct {
	Info     SeriesItem           `json:"info"`
	Seasons  []SeriesSeason       `json:"seasons"`
	Episodes map[string][]Episode `json:"episodes"`
}

type SeriesSeason struct {
	SeasonNumber FlexInt    `json:"season_number"`
	Name         string     `json:"name"`
	Cover        FlexString `json:"cover"`
	CoverBig     FlexString `json:"cover_big"`
	EpisodeCount FlexInt    `json:"episode_count"`
	Overview     string     `json:"overview"`
}

type Episode struct {
	ID                 FlexString  `json:"id"`
	EpisodeNum         FlexString  `json:"episode_num"`
	Title              string      `json:"title"`
	ContainerExtension string      `json:"container_extension"`
	Season             FlexString  `json:"season"`
	Info               EpisodeInfo `json:"info"`
	Added              FlexString  `json:"added"`
	DirectSource       string      `json:"direct_source"`
}

type EpisodeInfo struct {
	TMDBID       FlexString  `json:"tmdb_id"`
	Plot         FlexString  `json:"plot"`
	Duration     string      `json:"duration"`
	DurationSecs FlexString  `json:"duration_secs"`
	Releasedate  string      `json:"releasedate"`
	MovieImage   FlexString  `json:"movie_image"`
	Rating       FlexString  `json:"rating"`
	Season       FlexString  `json:"season"`
	Video        EpisodeTech `json:"video"`
	Audio        EpisodeTech `json:"audio"`
	Bitrate      FlexString  `json:"bitrate"`
}

type EpisodeTech struct {
	Index         FlexInt    `json:"index"`
	CodecName     string     `json:"codec_name"`
	CodecLong     string     `json:"codec_long_name"`
	Profile       string     `json:"profile"`
	CodecType     string     `json:"codec_type"`
	CodecTag      string     `json:"codec_tag_string"`
	Width         FlexInt    `json:"width"`
	Height        FlexInt    `json:"height"`
	Channels      FlexInt    `json:"channels"`
	SampleRate    FlexString `json:"sample_rate"`
	DisplayAspect string     `json:"display_aspect_ratio"`
}

type EPGListing struct {
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	Start          string     `json:"start"`
	End            string     `json:"end"`
	StartTimestamp FlexString `json:"start_timestamp"`
	StopTimestamp  FlexString `json:"stop_timestamp"`
	ChannelID      string     `json:"channel_id"`
	HasArchive     FlexInt    `json:"has_archive"`
	NowPlaying     FlexInt    `json:"now_playing"`
	CategoryID     FlexString `json:"category_id"`
}

type SimpleDataTable struct {
	Listings []EPGListing `json:"epg_listings"`
}

type VODInfo struct {
	Info      VODInfoDetails `json:"info"`
	MovieData VODMovieData   `json:"movie_data"`
}

type VODInfoDetails struct {
	Name         string      `json:"name"`
	Oname        string      `json:"o_name"`
	MovieImage   FlexString  `json:"movie_image"`
	CoverBig     FlexString  `json:"cover_big"`
	BackdropPath FlexString  `json:"backdrop_path"`
	Plot         FlexString  `json:"plot"`
	Description  string      `json:"description"`
	Genre        string      `json:"genre"`
	Releasedate  string      `json:"releasedate"`
	Rating       FlexString  `json:"rating"`
	Duration     string      `json:"duration"`
	DurationSecs FlexString  `json:"duration_secs"`
	TMDBID       FlexString  `json:"tmdb_id"`
	YouTube      string      `json:"youtube_trailer"`
	Actors       string      `json:"actors"`
	Director     string      `json:"director"`
	Country      string      `json:"country"`
	Status       string      `json:"status"`
	Bitrate      FlexString  `json:"bitrate"`
	Video        EpisodeTech `json:"video"`
	Audio        EpisodeTech `json:"audio"`
}

type VODMovieData struct {
	StreamID           FlexString `json:"stream_id"`
	Name               string     `json:"name"`
	Added              FlexString `json:"added"`
	CategoryID         FlexString `json:"category_id"`
	ContainerExtension string     `json:"container_extension"`
	CustomSID          string     `json:"custom_sid"`
	DirectSource       string     `json:"direct_source"`
}

func CategoryIDs(value any) []string {
	switch v := value.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, strings.TrimSpace(toString(item)))
		}
		return out
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		var out []string
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
		return out
	default:
		return nil
	}
}

func toString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case json.Number:
		return v.String()
	case nil:
		return ""
	default:
		return ""
	}
}
