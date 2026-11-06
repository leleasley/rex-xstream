package sync

import (
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/leleasley/rex-xstream/internal/models"
	"github.com/leleasley/rex-xstream/internal/xc"
)

var yearPattern = regexp.MustCompile(`\((19|20)\d{2}\)`)

func mapChannels(client *xc.Client, streams []xc.LiveStream, limit int) []models.Channel {
	if limit > 0 && len(streams) > limit {
		streams = streams[:limit]
	}
	out := make([]models.Channel, 0, len(streams))
	for _, stream := range streams {
		id := strings.TrimSpace(stream.StreamID.String())
		if id == "" {
			continue
		}
		categories := xc.CategoryIDs(stream.CategoryIDs)
		if len(categories) == 0 && stream.CategoryID.String() != "" {
			categories = []string{stream.CategoryID.String()}
		}
		out = append(out, models.Channel{
			ProviderID:  id,
			Name:        strings.TrimSpace(stream.Name),
			Number:      strings.TrimSpace(stream.Num.String()),
			Logo:        safeImage(stream.StreamIcon.String()),
			EPGID:       strings.TrimSpace(stream.EPGChannelID),
			CategoryIDs: categories,
			TVArchive:   stream.TVArchive.Int() != 0,
			StreamURL:   client.LiveStreamURL(id),
			Active:      true,
		})
	}
	return out
}

func mapMovies(client *xc.Client, streams []xc.VODStream, limit int) []models.Movie {
	if limit > 0 && len(streams) > limit {
		streams = streams[:limit]
	}
	out := make([]models.Movie, 0, len(streams))
	for _, stream := range streams {
		id := strings.TrimSpace(stream.StreamID.String())
		if id == "" {
			continue
		}
		categories := xc.CategoryIDs(stream.CategoryIDs)
		if len(categories) == 0 && stream.CategoryID.String() != "" {
			categories = []string{stream.CategoryID.String()}
		}
		title, year := splitYear(stream.Name)
		out = append(out, models.Movie{
			ProviderID:  id,
			Title:       title,
			Year:        year,
			TMDBID:      strings.TrimSpace(stream.TMDB.String()),
			Poster:      safeImage(stream.StreamIcon.String()),
			Rating:      strings.TrimSpace(stream.Rating.String()),
			Container:   strings.TrimSpace(stream.ContainerExtension),
			CategoryIDs: categories,
			StreamURL:   client.MovieStreamURL(id, stream.ContainerExtension),
			Active:      true,
		})
	}
	return out
}

func mapSeries(items []xc.SeriesItem, limit int) []models.Series {
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	out := make([]models.Series, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item.SeriesID.String())
		if id == "" {
			continue
		}
		categories := xc.CategoryIDs(item.CategoryIDs)
		if len(categories) == 0 && item.CategoryID.String() != "" {
			categories = []string{item.CategoryID.String()}
		}
		title, year := splitYear(item.Name)
		out = append(out, models.Series{
			ProviderID:       id,
			Title:            title,
			Year:             year,
			TMDBID:           strings.TrimSpace(item.TMDB.String()),
			Poster:           safeImage(item.Cover.String()),
			Backdrop:         safeImage(item.BackdropPath.String()),
			Plot:             strings.TrimSpace(item.Plot.String()),
			Genre:            strings.TrimSpace(item.Genre.String()),
			Rating:           strings.TrimSpace(item.Rating.String()),
			CategoryIDs:      categories,
			ProviderModified: strings.TrimSpace(item.LastModified.String()),
			Active:           true,
		})
	}
	return out
}

func mapSeriesDetail(client *xc.Client, info xc.SeriesInfo) ([]models.Season, []models.Episode) {
	var seasons []models.Season
	for _, season := range info.Seasons {
		number := season.SeasonNumber.Int()
		seasons = append(seasons, models.Season{
			Number: number,
			Title:  strings.TrimSpace(season.Name),
			Poster: strings.TrimSpace(firstNonEmpty(season.Cover.String(), season.CoverBig.String())),
		})
	}
	var episodes []models.Episode
	for key, list := range info.Episodes {
		keySeason, _ := strconv.Atoi(strings.TrimSpace(key))
		for _, episode := range list {
			seasonNumber, _ := strconv.Atoi(strings.TrimSpace(episode.Season.String()))
			if seasonNumber == 0 {
				seasonNumber = keySeason
			}
			episodeNumber, _ := strconv.Atoi(strings.TrimSpace(episode.EpisodeNum.String()))
			if seasonNumber <= 0 || episodeNumber <= 0 {
				continue
			}
			title := strings.TrimSpace(episode.Title)
			tmdbEpisode, _ := strconv.Atoi(strings.TrimSpace(episode.Info.TMDBID.String()))
			episodes = append(episodes, models.Episode{
				SeasonNumber:  seasonNumber,
				EpisodeNumber: episodeNumber,
				ProviderID:    strings.TrimSpace(episode.ID.String()),
				Title:         title,
				Plot:          strings.TrimSpace(episode.Info.Plot.String()),
				Image:         safeImage(episode.Info.MovieImage.String()),
				Container:     strings.TrimSpace(episode.ContainerExtension),
				TMDBEpisode:   tmdbEpisode,
				StreamURL:     client.EpisodeStreamURL(episode.ID.String(), episode.ContainerExtension),
				Active:        true,
			})
		}
	}
	return seasons, episodes
}

func splitYear(name string) (string, int) {
	original := strings.TrimSpace(name)
	name = original
	year := 0
	if match := yearPattern.FindString(name); match != "" {
		digits := strings.Trim(match, "()")
		year, _ = strconv.Atoi(digits)
		name = strings.TrimSpace(yearPattern.ReplaceAllString(name, ""))
	}
	if year == 0 {
		fields := strings.Fields(name)
		if len(fields) > 0 {
			if parsed, err := strconv.Atoi(strings.Trim(fields[len(fields)-1], "-()")); err == nil && parsed >= 1900 && parsed <= 2100 {
				year = parsed
				name = strings.TrimSpace(strings.TrimSuffix(name, fields[len(fields)-1]))
			}
		}
	}
	name = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(name), "-:."))
	if strings.TrimSpace(name) == "" {
		name = original
	}
	return name, year
}

// dedupeMovies removes provider entries that name the same film. XC panels
// often list one film many times, once per quality or source stream. Keeping
// one row per TMDB id prevents a library full of identical tiles.
func dedupeMovies(items []models.Movie) []models.Movie {
	index := map[string]int{}
	out := make([]models.Movie, 0, len(items))
	for _, item := range items {
		key := "tmdb:" + strings.TrimSpace(item.TMDBID)
		if strings.TrimSpace(item.TMDBID) == "" {
			key = "title:" + strings.ToLower(strings.TrimSpace(item.Title)) + "|" + strconv.Itoa(item.Year)
		}
		if position, exists := index[key]; exists {
			if ratingValue(item.Rating) > ratingValue(out[position].Rating) {
				out[position] = item
			}
			continue
		}
		index[key] = len(out)
		out = append(out, item)
	}
	return out
}

func dedupeSeries(items []models.Series) []models.Series {
	index := map[string]int{}
	out := make([]models.Series, 0, len(items))
	for _, item := range items {
		key := "tmdb:" + strings.TrimSpace(item.TMDBID)
		if strings.TrimSpace(item.TMDBID) == "" {
			key = "title:" + strings.ToLower(strings.TrimSpace(item.Title)) + "|" + strconv.Itoa(item.Year)
		}
		if _, exists := index[key]; exists {
			continue
		}
		index[key] = len(out)
		out = append(out, item)
	}
	return out
}

func ratingValue(value string) float64 {
	parsed, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return parsed
}

// safeImage keeps public HTTP artwork and rejects private or malformed URLs.
// REX fetches display artwork itself, so an unchecked URL would be an SSRF path.
func safeImage(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ""
	}
	host := strings.Trim(parsed.Hostname(), "[]")
	if strings.EqualFold(host, "localhost") {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return ""
		}
	}
	return raw
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
