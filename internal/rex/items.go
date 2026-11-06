package rex

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/leleasley/rex-xstream/internal/library"
	"github.com/leleasley/rex-xstream/internal/models"
)

type ItemContext struct {
	PlaylistName string
	PublicURL    string
	Guide        map[string][]models.Programme
	UseTMDB      bool
}

func MovieItem(movie models.Movie, ctx ItemContext) (Item, bool) {
	if len(movie.MediaInfo) == 0 {
		return Item{}, false
	}
	ids := map[string]any{}
	if ctx.UseTMDB {
		if id, ok := numericID(movie.TMDBID); ok {
			ids["tmdb"] = id
		} else if movie.IMDBID != "" {
			ids["imdb"] = movie.IMDBID
		}
	}
	year := ""
	if movie.Year > 0 {
		year = fmt.Sprintf("%d", movie.Year)
	}
	item := Item{
		ID:        library.MovieItemID(movie.PlaylistID, movie.ProviderID),
		Path:      library.MoviePath(movie.PlaylistID, movie.ID, movie.Container),
		Kind:      "movie",
		MediaInfo: json.RawMessage(movie.MediaInfo),
		Display: &Display{
			Title:   movie.Title,
			Summary: movie.Plot,
			Date:    year,
			Thumb:   firstNonEmpty(movie.Poster, fallbackPoster(ctx.PublicURL)),
			Art:     movie.Backdrop,
		},
	}
	if len(ids) > 0 {
		item.IDs = ids
	}
	return item, true
}

func EpisodeItem(episode models.Episode, series models.Series, ctx ItemContext) (Item, bool) {
	if len(episode.MediaInfo) == 0 {
		return Item{}, false
	}
	season := episode.SeasonNumber
	number := episode.EpisodeNumber
	ids := map[string]any{}
	if ctx.UseTMDB {
		if id, ok := numericID(series.TMDBID); ok {
			ids["tmdb"] = id
		}
	}
	item := Item{
		ID:      library.EpisodeItemID(series.PlaylistID, int(series.ID), season, number),
		Path:    library.EpisodePath(series.PlaylistID, episode.ID, episode.Container),
		Kind:    "episode",
		Name:    firstNonEmpty(series.Title, "Series"),
		Season:  &season,
		Episode: &number,
		TMDBEpisode: func() int {
			if ctx.UseTMDB {
				return episode.TMDBEpisode
			}
			return 0
		}(),
		MediaInfo: json.RawMessage(episode.MediaInfo),
		Display: &Display{
			Title:   episode.Title,
			Summary: episode.Plot,
			Thumb:   firstNonEmpty(episode.Image, series.Poster),
			Show: &DisplayShow{
				Title:   series.Title,
				Summary: series.Plot,
				Poster:  series.Poster,
				Art:     series.Backdrop,
			},
		},
	}
	if len(ids) > 0 {
		item.IDs = ids
	}
	return item, true
}

func LiveItem(channel models.Channel, ctx ItemContext) (Item, bool) {
	if len(channel.MediaInfo) == 0 {
		return Item{}, false
	}
	number := strings.TrimSpace(channel.Number)
	if number == "" {
		number = fmt.Sprintf("%d", 500000+channel.ID)
	}
	logo := strings.TrimSpace(channel.Logo)
	if logo == "" {
		logo = fallbackPoster(ctx.PublicURL)
	}
	item := Item{
		ID:        library.LiveItemID(channel.PlaylistID, channel.ProviderID),
		Path:      library.LivePath(channel.ID),
		MediaInfo: json.RawMessage(channel.MediaInfo),
		Display: &Display{
			Title: channel.Name,
			Thumb: logo,
		},
		Channel: &Channel{
			Number: number,
			Title:  channel.Name,
			Guide:  guideFor(channel.EPGID, channel.Name, ctx.Guide),
		},
	}
	return item, true
}

func guideFor(epgID, channelName string, guide map[string][]models.Programme) []GuideEntry {
	if epgID == "" {
		return nil
	}
	programmes := guide[epgID]
	if len(programmes) == 0 {
		return nil
	}
	out := make([]GuideEntry, 0, len(programmes))
	for _, programme := range programmes {
		entry := GuideEntry{
			Start:      programme.Start.UTC().Format(time.RFC3339),
			End:        programme.Stop.UTC().Format(time.RFC3339),
			Title:      programme.Title,
			Summary:    programme.Description,
			Thumb:      programme.Icon,
			Categories: categories(programme.Category),
		}
		if programme.Season > 0 && programme.Episode > 0 {
			entry.Type = "episode"
			entry.Show = channelName
			entry.Season = programme.Season
			entry.Episode = programme.Episode
			entry.Title = programme.Title
		}
		out = append(out, entry)
	}
	return out
}

func categories(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func numericID(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	id, err := strconv.Atoi(value)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func fallbackPoster(publicURL string) string {
	if publicURL == "" {
		return ""
	}
	return strings.TrimRight(publicURL, "/") + "/assets/logo-icon.svg"
}
