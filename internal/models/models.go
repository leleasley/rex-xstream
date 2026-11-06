package models

import (
	"encoding/json"
	"time"
)

type Playlist struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	BaseURL         string     `json:"base_url"`
	Enabled         bool       `json:"enabled"`
	Status          string     `json:"status"`
	Provider        Provider   `json:"provider"`
	SubscriptionEnd *time.Time `json:"subscription_end,omitempty"`
	MaxConnections  int        `json:"max_connections,omitempty"`
	LastSyncAt      *time.Time `json:"last_sync_at,omitempty"`
	LastSyncError   string     `json:"last_sync_error,omitempty"`
	LastTestAt      *time.Time `json:"last_test_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	HasCredentials  bool       `json:"has_credentials"`
	Directories     []Library  `json:"libraries,omitempty"`
}

// Provider is the subset of XC account information safe to expose.
type Provider struct {
	ServerSoftware string `json:"server_software,omitempty"`
	Timezone       string `json:"timezone,omitempty"`
	Protocol       string `json:"protocol,omitempty"`
	Username       string `json:"username,omitempty"`
	Status         string `json:"status,omitempty"`
	IsTrial        bool   `json:"is_trial"`
	ActiveConns    int    `json:"active_connections,omitempty"`
}

type Library struct {
	PlaylistID int64  `json:"playlist_id"`
	Kind       string `json:"kind"`
	Directory  string `json:"directory"`
	Title      string `json:"title"`
	Items      int    `json:"items"`
}

type Category struct {
	ID         int64  `json:"id"`
	PlaylistID int64  `json:"playlist_id"`
	Kind       string `json:"kind"`
	ProviderID string `json:"provider_id"`
	Name       string `json:"name"`
	Count      int    `json:"count,omitempty"`
}

type Channel struct {
	ID          int64           `json:"id"`
	PlaylistID  int64           `json:"playlist_id"`
	ProviderID  string          `json:"provider_id"`
	Name        string          `json:"name"`
	Number      string          `json:"number,omitempty"`
	Logo        string          `json:"logo,omitempty"`
	EPGID       string          `json:"epg_id,omitempty"`
	CategoryIDs []string        `json:"category_ids,omitempty"`
	TVArchive   bool            `json:"tv_archive"`
	StreamURL   string          `json:"stream_url,omitempty"`
	ProbeStatus string          `json:"probe_status"`
	ProbeError  string          `json:"probe_error,omitempty"`
	MediaInfo   json.RawMessage `json:"media_info,omitempty"`
	Active      bool            `json:"active"`
}

type Movie struct {
	ID          int64           `json:"id"`
	PlaylistID  int64           `json:"playlist_id"`
	ProviderID  string          `json:"provider_id"`
	Title       string          `json:"title"`
	Year        int             `json:"year,omitempty"`
	TMDBID      string          `json:"tmdb_id,omitempty"`
	IMDBID      string          `json:"imdb_id,omitempty"`
	Poster      string          `json:"poster,omitempty"`
	Backdrop    string          `json:"backdrop,omitempty"`
	Plot        string          `json:"plot,omitempty"`
	Genre       string          `json:"genre,omitempty"`
	Rating      string          `json:"rating,omitempty"`
	Duration    string          `json:"duration,omitempty"`
	Container   string          `json:"container,omitempty"`
	CategoryIDs []string        `json:"category_ids,omitempty"`
	StreamURL   string          `json:"stream_url,omitempty"`
	ProbeStatus string          `json:"probe_status"`
	ProbeError  string          `json:"probe_error,omitempty"`
	MediaInfo   json.RawMessage `json:"media_info,omitempty"`
	Active      bool            `json:"active"`
}

type Series struct {
	ID               int64    `json:"id"`
	PlaylistID       int64    `json:"playlist_id"`
	ProviderID       string   `json:"provider_id"`
	Title            string   `json:"title"`
	Year             int      `json:"year,omitempty"`
	TMDBID           string   `json:"tmdb_id,omitempty"`
	Poster           string   `json:"poster,omitempty"`
	Backdrop         string   `json:"backdrop,omitempty"`
	Plot             string   `json:"plot,omitempty"`
	Genre            string   `json:"genre,omitempty"`
	Rating           string   `json:"rating,omitempty"`
	CategoryIDs      []string `json:"category_ids,omitempty"`
	ProviderModified string   `json:"-"`
	Seasons          int      `json:"seasons,omitempty"`
	Episodes         int      `json:"episodes,omitempty"`
	Active           bool     `json:"active"`
}

type Season struct {
	ID           int64  `json:"id"`
	SeriesID     int64  `json:"series_id"`
	Number       int    `json:"number"`
	Title        string `json:"title,omitempty"`
	Poster       string `json:"poster,omitempty"`
	EpisodeCount int    `json:"episode_count,omitempty"`
}

type Episode struct {
	ID            int64           `json:"id"`
	SeriesID      int64           `json:"series_id"`
	SeasonNumber  int             `json:"season_number"`
	EpisodeNumber int             `json:"episode_number"`
	ProviderID    string          `json:"provider_id"`
	Title         string          `json:"title,omitempty"`
	Plot          string          `json:"plot,omitempty"`
	Image         string          `json:"image,omitempty"`
	Container     string          `json:"container,omitempty"`
	TMDBEpisode   int             `json:"tmdb_episode,omitempty"`
	StreamURL     string          `json:"stream_url,omitempty"`
	ProbeStatus   string          `json:"probe_status"`
	ProbeError    string          `json:"probe_error,omitempty"`
	MediaInfo     json.RawMessage `json:"media_info,omitempty"`
	Active        bool            `json:"active"`
}

type Programme struct {
	ChannelEPGID string    `json:"channel_epg_id"`
	Start        time.Time `json:"start"`
	Stop         time.Time `json:"stop"`
	Title        string    `json:"title"`
	Description  string    `json:"description,omitempty"`
	Category     string    `json:"category,omitempty"`
	Icon         string    `json:"icon,omitempty"`
	Season       int       `json:"season,omitempty"`
	Episode      int       `json:"episode,omitempty"`
}

type SyncStatus struct {
	PlaylistID int64      `json:"playlist_id"`
	Running    bool       `json:"running"`
	Phase      string     `json:"phase"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Error      string     `json:"error,omitempty"`
	Channels   int        `json:"channels"`
	Movies     int        `json:"movies"`
	Series     int        `json:"series"`
	Episodes   int        `json:"episodes"`
	Programmes int        `json:"programmes"`
	Probed     int        `json:"probed"`
	Pushed     int        `json:"pushed"`
}
