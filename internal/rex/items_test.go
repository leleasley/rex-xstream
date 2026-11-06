package rex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/leleasley/rex-xstream/internal/models"
)

func media() json.RawMessage {
	return json.RawMessage(`{"format":{"format_name":"matroska"},"streams":[{"codec_type":"video","codec_name":"h264"}]}`)
}

func TestRenameLibrariesUsesPlexAPI(t *testing.T) {
	var renamed string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/library/sections":
			fmt.Fprint(w, `{"MediaContainer":{"Directory":[{"key":"4","title":"Old Movies","type":"movie"},{"key":"5","title":"Old Series","type":"show"}]}}`)
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/library/sections/"):
			renamed = r.URL.Query().Get("name")
			fmt.Fprint(w, `{}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := New(server.URL, "test", "u", "p", 10)
	client.SetToken("token")
	if err := client.RenameLibraries(context.Background(), []Rename{{OldTitle: "Old Movies", NewTitle: "New Movies", Type: "movie"}}); err != nil {
		t.Fatal(err)
	}
	if renamed != "New Movies" {
		t.Fatalf("renamed to %q", renamed)
	}
}

func TestMovieItemUsesTMDBAndDisplay(t *testing.T) {
	movie := models.Movie{ID: 5, PlaylistID: 2, ProviderID: "10", Title: "Film", Year: 2024, TMDBID: "123", Poster: "https://img/p.jpg", MediaInfo: media()}
	item, ok := MovieItem(movie, ItemContext{PublicURL: "http://addon", UseTMDB: true})
	if !ok {
		t.Fatal("movie was dropped")
	}
	if item.Kind != "movie" || item.Path != "p2-movies/5.mp4" {
		t.Fatalf("item: %+v", item)
	}
	if item.IDs["tmdb"] != 123 {
		t.Fatalf("tmdb id: %+v", item.IDs)
	}
	if item.Display.Thumb != "https://img/p.jpg" {
		t.Fatalf("poster: %s", item.Display.Thumb)
	}
}

func TestTMDBIDsCanBeDisabled(t *testing.T) {
	movie := models.Movie{ID: 5, PlaylistID: 2, Title: "Film", TMDBID: "123", MediaInfo: media()}
	item, _ := MovieItem(movie, ItemContext{})
	if len(item.IDs) != 0 {
		t.Fatalf("ids should be omitted: %+v", item.IDs)
	}
}

func TestMovieWithoutProbeIsDropped(t *testing.T) {
	if _, ok := MovieItem(models.Movie{ID: 1}, ItemContext{}); ok {
		t.Fatal("unprobed movie should be dropped")
	}
}

func TestEpisodeItemKeepsShowAndNumbers(t *testing.T) {
	episode := models.Episode{ID: 9, SeriesID: 4, SeasonNumber: 2, EpisodeNumber: 3, Title: "Pilot", TMDBEpisode: 777, MediaInfo: media()}
	series := models.Series{ID: 4, PlaylistID: 2, Title: "Show", TMDBID: "456", Poster: "https://img/show.jpg"}
	item, ok := EpisodeItem(episode, series, ItemContext{UseTMDB: true})
	if !ok {
		t.Fatal("episode was dropped")
	}
	if item.Kind != "episode" || item.Name != "Show" || *item.Season != 2 || *item.Episode != 3 {
		t.Fatalf("episode: %+v", item)
	}
	if item.TMDBEpisode != 777 || item.IDs["tmdb"] != 456 {
		t.Fatalf("ids: %+v %d", item.IDs, item.TMDBEpisode)
	}
	if item.Display.Show == nil || item.Display.Show.Title != "Show" {
		t.Fatalf("show display: %+v", item.Display)
	}
}

func TestLiveItemHasChannelAndGuide(t *testing.T) {
	start := time.Date(2026, 11, 6, 18, 0, 0, 0, time.UTC)
	channel := models.Channel{ID: 7, PlaylistID: 2, ProviderID: "50", Name: "News", Number: "101", EPGID: "news", MediaInfo: media(), Logo: "https://img/logo.png"}
	guide := map[string][]models.Programme{
		"news": {{ChannelEPGID: "news", Start: start, Stop: start.Add(time.Hour), Title: "Bulletin", Category: "News", Season: 0}},
	}
	item, ok := LiveItem(channel, ItemContext{PublicURL: "http://addon", Guide: guide})
	if !ok {
		t.Fatal("channel was dropped")
	}
	if item.Path != "live/7.m3u8" || item.Channel == nil || item.Channel.Number != "101" {
		t.Fatalf("live item: %+v", item)
	}
	if len(item.Channel.Guide) != 1 || item.Channel.Guide[0].Title != "Bulletin" {
		t.Fatalf("guide: %+v", item.Channel.Guide)
	}
	if item.Display.Thumb != "https://img/logo.png" {
		t.Fatalf("logo: %s", item.Display.Thumb)
	}
}

func TestLiveItemFallbackLogoAndNumber(t *testing.T) {
	channel := models.Channel{ID: 7, PlaylistID: 2, Name: "No Logo", MediaInfo: media()}
	item, _ := LiveItem(channel, ItemContext{PublicURL: "http://addon"})
	if item.Channel.Number == "" {
		t.Fatal("a channel number must exist")
	}
	if item.Display.Thumb != "http://addon/assets/logo-icon.svg" {
		t.Fatalf("fallback: %s", item.Display.Thumb)
	}
}
