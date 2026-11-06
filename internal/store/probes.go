package store

import (
	"context"
	"database/sql"
	"strings"

	"github.com/leleasley/rex-xstream/internal/models"
)

type ProbeItem struct {
	Kind      string
	ID        int64
	StreamURL string
	Live      bool
}

func (s *Store) PendingProbes(ctx context.Context, playlistID int64, limit int) ([]ProbeItem, error) {
	if limit <= 0 {
		limit = 50
	}
	var out []ProbeItem
	rows, err := s.db.QueryContext(ctx, `SELECT id, stream_url FROM channels
        WHERE playlist_id = ? AND active = 1 AND probe_status = 'pending' AND stream_url <> '' LIMIT ?`, playlistID, limit)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item ProbeItem
		item.Kind = "channel"
		item.Live = true
		if err := rows.Scan(&item.ID, &item.StreamURL); err != nil {
			rows.Close()
			return nil, err
		}
		item.StreamURL, err = s.decryptURL(item.StreamURL)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, item)
	}
	rows.Close()
	rows, err = s.db.QueryContext(ctx, `SELECT id, stream_url FROM movies
        WHERE playlist_id = ? AND active = 1 AND probe_status = 'pending' AND stream_url <> '' LIMIT ?`, playlistID, limit)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item ProbeItem
		item.Kind = "movie"
		if err := rows.Scan(&item.ID, &item.StreamURL); err != nil {
			rows.Close()
			return nil, err
		}
		item.StreamURL, err = s.decryptURL(item.StreamURL)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, item)
	}
	rows.Close()
	rows, err = s.db.QueryContext(ctx, `SELECT e.id, e.stream_url FROM episodes e
        JOIN series s ON s.id = e.series_id
        WHERE s.playlist_id = ? AND e.active = 1 AND e.probe_status = 'pending' AND e.stream_url <> '' LIMIT ?`,
		playlistID, limit)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item ProbeItem
		item.Kind = "episode"
		if err := rows.Scan(&item.ID, &item.StreamURL); err != nil {
			rows.Close()
			return nil, err
		}
		item.StreamURL, err = s.decryptURL(item.StreamURL)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, item)
	}
	rows.Close()
	return out, nil
}

// StreamFor returns the decrypted provider URL for one stored item.
func (s *Store) StreamFor(ctx context.Context, kind string, playlistID, id int64) (string, int64, error) {
	var encrypted string
	var owner int64
	var err error
	switch kind {
	case "movie":
		err = s.db.QueryRowContext(ctx, `SELECT stream_url, playlist_id FROM movies WHERE id = ? AND playlist_id = ?`, id, playlistID).Scan(&encrypted, &owner)
	case "episode":
		err = s.db.QueryRowContext(ctx, `SELECT e.stream_url, s.playlist_id FROM episodes e JOIN series s ON s.id = e.series_id WHERE e.id = ? AND s.playlist_id = ?`, id, playlistID).Scan(&encrypted, &owner)
	case "channel":
		err = s.db.QueryRowContext(ctx, `SELECT stream_url, playlist_id FROM channels WHERE id = ?`, id).Scan(&encrypted, &owner)
	default:
		return "", 0, sql.ErrNoRows
	}
	if err != nil {
		return "", 0, err
	}
	value, err := s.decryptURL(encrypted)
	return value, owner, err
}

func (s *Store) SaveProbe(ctx context.Context, kind string, id int64, mediaInfo, probeError string) error {
	status := "done"
	if probeError != "" {
		status = "failed"
	}
	table := map[string]string{"channel": "channels", "movie": "movies", "episode": "episodes"}[kind]
	if table == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `UPDATE `+table+` SET probe_status = ?, probe_error = ?, media_info = ?,
        dirty = CASE WHEN ? = 'done' THEN 1 ELSE dirty END, updated_at = ? WHERE id = ?`,
		status, probeError, mediaInfo, status, now(), id)
	return err
}

func (s *Store) DirtyMovies(ctx context.Context, playlistID int64, limit int) ([]models.Movie, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, playlist_id, provider_id, title, year, tmdb_id, imdb_id,
        poster, backdrop, plot, genre, rating, duration, container, category_ids, probe_status, probe_error, media_info
        FROM movies WHERE playlist_id = ? AND active = 1 AND probe_status = 'done' AND dirty = 1 ORDER BY id LIMIT ?`,
		playlistID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Movie
	for rows.Next() {
		var item models.Movie
		var categories, media string
		if err := rows.Scan(&item.ID, &item.PlaylistID, &item.ProviderID, &item.Title, &item.Year, &item.TMDBID,
			&item.IMDBID, &item.Poster, &item.Backdrop, &item.Plot, &item.Genre, &item.Rating, &item.Duration,
			&item.Container, &categories, &item.ProbeStatus, &item.ProbeError, &media); err != nil {
			return nil, err
		}
		item.CategoryIDs = decodeCategories(categories)
		item.MediaInfo = []byte(media)
		item.Active = true
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) DirtyEpisodes(ctx context.Context, playlistID int64, limit int) ([]models.Episode, []models.Series, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT e.id, e.series_id, e.season_number, e.episode_number, e.provider_id,
        e.title, e.plot, e.image, e.container, e.tmdb_episode, e.probe_status, e.probe_error, e.media_info,
        s.playlist_id, s.title, s.tmdb_id, s.poster, s.backdrop, s.plot
        FROM episodes e JOIN series s ON s.id = e.series_id
        WHERE s.playlist_id = ? AND e.active = 1 AND e.probe_status = 'done' AND e.dirty = 1
        ORDER BY e.series_id, e.season_number, e.episode_number LIMIT ?`, playlistID, limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var episodes []models.Episode
	var series []models.Series
	seen := map[int64]bool{}
	for rows.Next() {
		var episode models.Episode
		var show models.Series
		var media string
		if err := rows.Scan(&episode.ID, &episode.SeriesID, &episode.SeasonNumber, &episode.EpisodeNumber,
			&episode.ProviderID, &episode.Title, &episode.Plot, &episode.Image, &episode.Container,
			&episode.TMDBEpisode, &episode.ProbeStatus, &episode.ProbeError, &media,
			&show.PlaylistID, &show.Title, &show.TMDBID, &show.Poster, &show.Backdrop, &show.Plot); err != nil {
			return nil, nil, err
		}
		episode.MediaInfo = []byte(media)
		episode.Active = true
		episodes = append(episodes, episode)
		show.ID = episode.SeriesID
		if !seen[show.ID] {
			seen[show.ID] = true
			series = append(series, show)
		}
	}
	return episodes, series, nil
}

func (s *Store) DirtyChannels(ctx context.Context, playlistID int64, limit int) ([]models.Channel, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, playlist_id, provider_id, name, number, logo, epg_id,
        category_ids, tv_archive, probe_status, probe_error, media_info, active
        FROM channels WHERE playlist_id = ? AND active = 1 AND probe_status = 'done' AND dirty = 1 ORDER BY id LIMIT ?`,
		playlistID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Channel
	for rows.Next() {
		var item models.Channel
		var categories, media string
		var archive, active int
		if err := rows.Scan(&item.ID, &item.PlaylistID, &item.ProviderID, &item.Name, &item.Number, &item.Logo,
			&item.EPGID, &categories, &archive, &item.ProbeStatus, &item.ProbeError, &media, &active); err != nil {
			return nil, err
		}
		item.CategoryIDs = decodeCategories(categories)
		item.TVArchive = archive == 1
		item.Active = active == 1
		item.MediaInfo = []byte(media)
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) MarkPushed(ctx context.Context, kind string, ids []int64, rexIDs []string) error {
	table := map[string]string{"channel": "channels", "movie": "movies", "episode": "episodes"}[kind]
	if table == "" || len(ids) == 0 {
		return nil
	}
	for index, id := range ids {
		rexID := ""
		if index < len(rexIDs) {
			rexID = rexIDs[index]
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE `+table+` SET dirty = 0, pushed_at = ?, rex_id = CASE WHEN ? <> '' THEN ? ELSE rex_id END WHERE id = ?`,
			now(), rexID, rexID, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) InactiveRexIDs(ctx context.Context, playlistID int64) ([]string, error) {
	var out []string
	queries := []string{
		`SELECT rex_id FROM movies WHERE playlist_id = ? AND active = 0 AND rex_id <> ''`,
		`SELECT rex_id FROM channels WHERE playlist_id = ? AND active = 0 AND rex_id <> ''`,
		`SELECT e.rex_id FROM episodes e JOIN series s ON s.id = e.series_id WHERE s.playlist_id = ? AND e.active = 0 AND e.rex_id <> ''`,
	}
	for _, query := range queries {
		rows, err := s.db.QueryContext(ctx, query, playlistID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, value)
		}
		rows.Close()
	}
	return out, nil
}

func (s *Store) ClearRexIDs(ctx context.Context, playlistID int64) error {
	queries := []string{
		`UPDATE movies SET rex_id = '', pushed_at = NULL WHERE playlist_id = ? AND active = 0`,
		`UPDATE channels SET rex_id = '', pushed_at = NULL WHERE playlist_id = ? AND active = 0`,
		`UPDATE episodes SET rex_id = '', pushed_at = NULL WHERE series_id IN (SELECT id FROM series WHERE playlist_id = ?) AND active = 0`,
	}
	for _, query := range queries {
		if _, err := s.db.ExecContext(ctx, query, playlistID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) AllRexIDs(ctx context.Context, playlistID int64) ([]string, error) {
	var out []string
	queries := []string{
		`SELECT rex_id FROM movies WHERE playlist_id = ? AND rex_id <> ''`,
		`SELECT rex_id FROM channels WHERE playlist_id = ? AND rex_id <> ''`,
		`SELECT e.rex_id FROM episodes e JOIN series s ON s.id = e.series_id WHERE s.playlist_id = ? AND e.rex_id <> ''`,
	}
	for _, query := range queries {
		rows, err := s.db.QueryContext(ctx, query, playlistID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, value)
		}
		rows.Close()
	}
	return out, nil
}

func scanNullTime(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}

func (s *Store) ActiveEpgIDs(ctx context.Context, playlistID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT epg_id FROM channels WHERE playlist_id = ? AND active = 1 AND epg_id <> ''`, playlistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		out = append(out, strings.TrimSpace(value))
	}
	return out, nil
}
