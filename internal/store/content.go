package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/leleasley/rex-xstream/internal/models"
)

func (s *Store) ListCategories(ctx context.Context, playlistID int64, kind string) ([]models.Category, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT provider_id, name FROM categories WHERE playlist_id = ? AND kind = ? ORDER BY name`, playlistID, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Category
	for rows.Next() {
		item := models.Category{PlaylistID: playlistID, Kind: kind}
		if err := rows.Scan(&item.ProviderID, &item.Name); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) ListChannels(ctx context.Context, playlistID int64, category string, limit, offset int) ([]models.Channel, int, error) {
	where := `playlist_id = ? AND active = 1`
	args := []any{playlistID}
	if category != "" {
		where += ` AND category_ids LIKE ?`
		args = append(args, `%`+category+`%`)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM channels WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, `SELECT id, playlist_id, provider_id, name, number, logo, epg_id,
        category_ids, tv_archive, probe_status, probe_error, active FROM channels WHERE `+where+`
        ORDER BY CASE WHEN number = '' THEN 1 ELSE 0 END, CAST(number AS INTEGER), name LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []models.Channel
	for rows.Next() {
		var item models.Channel
		var categories string
		var archive, active int
		if err := rows.Scan(&item.ID, &item.PlaylistID, &item.ProviderID, &item.Name, &item.Number, &item.Logo,
			&item.EPGID, &categories, &archive, &item.ProbeStatus, &item.ProbeError, &active); err != nil {
			return nil, 0, err
		}
		item.CategoryIDs = decodeCategories(categories)
		item.TVArchive = archive == 1
		item.Active = active == 1
		out = append(out, item)
	}
	return out, total, nil
}

func (s *Store) ListMovies(ctx context.Context, playlistID int64, category string, limit, offset int) ([]models.Movie, int, error) {
	where := `playlist_id = ? AND active = 1`
	args := []any{playlistID}
	if category != "" {
		where += ` AND category_ids LIKE ?`
		args = append(args, `%`+category+`%`)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM movies WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, `SELECT id, playlist_id, provider_id, title, year, tmdb_id, imdb_id,
        poster, backdrop, plot, genre, rating, duration, container, category_ids, probe_status, probe_error, active
        FROM movies WHERE `+where+` ORDER BY title, year LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []models.Movie
	for rows.Next() {
		var item models.Movie
		var categories string
		var active int
		if err := rows.Scan(&item.ID, &item.PlaylistID, &item.ProviderID, &item.Title, &item.Year, &item.TMDBID,
			&item.IMDBID, &item.Poster, &item.Backdrop, &item.Plot, &item.Genre, &item.Rating, &item.Duration,
			&item.Container, &categories, &item.ProbeStatus, &item.ProbeError, &active); err != nil {
			return nil, 0, err
		}
		item.CategoryIDs = decodeCategories(categories)
		item.Active = active == 1
		out = append(out, item)
	}
	return out, total, nil
}

func (s *Store) ListSeries(ctx context.Context, playlistID int64, category string, limit, offset int) ([]models.Series, int, error) {
	where := `s.playlist_id = ? AND s.active = 1`
	args := []any{playlistID}
	if category != "" {
		where += ` AND s.category_ids LIKE ?`
		args = append(args, `%`+category+`%`)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM series s WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, `SELECT s.id, s.playlist_id, s.provider_id, s.title, s.year, s.tmdb_id,
        s.poster, s.backdrop, s.plot, s.genre, s.rating, s.category_ids, s.active,
        (SELECT COUNT(1) FROM seasons se WHERE se.series_id = s.id),
        (SELECT COUNT(1) FROM episodes e WHERE e.series_id = s.id AND e.active = 1)
        FROM series s WHERE `+where+` ORDER BY s.title LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []models.Series
	for rows.Next() {
		var item models.Series
		var categories string
		var active int
		if err := rows.Scan(&item.ID, &item.PlaylistID, &item.ProviderID, &item.Title, &item.Year, &item.TMDBID,
			&item.Poster, &item.Backdrop, &item.Plot, &item.Genre, &item.Rating, &categories, &active,
			&item.Seasons, &item.Episodes); err != nil {
			return nil, 0, err
		}
		item.CategoryIDs = decodeCategories(categories)
		item.Active = active == 1
		out = append(out, item)
	}
	return out, total, nil
}

func (s *Store) GetSeries(ctx context.Context, playlistID, seriesID int64) (models.Series, error) {
	var item models.Series
	var categories string
	var active int
	err := s.db.QueryRowContext(ctx, `SELECT id, playlist_id, provider_id, title, year, tmdb_id, poster, backdrop,
        plot, genre, rating, category_ids, active FROM series WHERE id = ? AND playlist_id = ?`, seriesID, playlistID).
		Scan(&item.ID, &item.PlaylistID, &item.ProviderID, &item.Title, &item.Year, &item.TMDBID, &item.Poster,
			&item.Backdrop, &item.Plot, &item.Genre, &item.Rating, &categories, &active)
	if err != nil {
		return models.Series{}, err
	}
	item.CategoryIDs = decodeCategories(categories)
	item.Active = active == 1
	return item, nil
}

func (s *Store) Seasons(ctx context.Context, seriesID int64) ([]models.Season, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, series_id, number, title, poster,
        (SELECT COUNT(1) FROM episodes e WHERE e.series_id = seasons.series_id AND e.season_number = seasons.number AND e.active = 1)
        FROM seasons WHERE series_id = ? ORDER BY number`, seriesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Season
	for rows.Next() {
		var item models.Season
		if err := rows.Scan(&item.ID, &item.SeriesID, &item.Number, &item.Title, &item.Poster, &item.EpisodeCount); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) Episodes(ctx context.Context, seriesID int64, season int) ([]models.Episode, error) {
	query := `SELECT id, series_id, season_number, episode_number, provider_id, title, plot, image, container,
        tmdb_episode, probe_status, probe_error, active FROM episodes WHERE series_id = ? AND active = 1`
	args := []any{seriesID}
	if season >= 0 {
		query += ` AND season_number = ?`
		args = append(args, season)
	}
	query += ` ORDER BY season_number, episode_number`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Episode
	for rows.Next() {
		var item models.Episode
		var active int
		if err := rows.Scan(&item.ID, &item.SeriesID, &item.SeasonNumber, &item.EpisodeNumber, &item.ProviderID,
			&item.Title, &item.Plot, &item.Image, &item.Container, &item.TMDBEpisode,
			&item.ProbeStatus, &item.ProbeError, &active); err != nil {
			return nil, err
		}
		item.Active = active == 1
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) Programmes(ctx context.Context, playlistID int64, epgID string, limit int) ([]models.Programme, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT channel_epg_id, start_utc, stop_utc, title, description, category, icon, season, episode
        FROM programmes WHERE playlist_id = ? AND channel_epg_id = ? ORDER BY start_utc LIMIT ?`, playlistID, epgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Programme
	for rows.Next() {
		var item models.Programme
		var start, stop int64
		if err := rows.Scan(&item.ChannelEPGID, &start, &stop, &item.Title, &item.Description,
			&item.Category, &item.Icon, &item.Season, &item.Episode); err != nil {
			return nil, err
		}
		item.Start = timeFromMillis(start)
		item.Stop = timeFromMillis(stop)
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) LibraryItems(ctx context.Context, directory string) (map[string]any, error) {
	if strings.HasPrefix(directory, "p") && strings.HasSuffix(directory, "-movies") {
		var playlistID int64
		if _, err := fmtSscan(directory, "p%d-movies", &playlistID); err != nil {
			return nil, err
		}
		movies, total, err := s.ListMovies(ctx, playlistID, "", 500, 0)
		if err != nil {
			return nil, err
		}
		return map[string]any{"kind": "movies", "playlist_id": playlistID, "total": total, "items": movies}, nil
	}
	if strings.HasPrefix(directory, "p") && strings.HasSuffix(directory, "-shows") {
		var playlistID int64
		if _, err := fmtSscan(directory, "p%d-shows", &playlistID); err != nil {
			return nil, err
		}
		series, total, err := s.ListSeries(ctx, playlistID, "", 500, 0)
		if err != nil {
			return nil, err
		}
		return map[string]any{"kind": "series", "playlist_id": playlistID, "total": total, "items": series}, nil
	}
	if directory == LiveDirectory {
		var total int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM channels WHERE active = 1`).Scan(&total); err != nil {
			return nil, err
		}
		rows, err := s.db.QueryContext(ctx, `SELECT id, playlist_id, provider_id, name, number, logo, epg_id,
            category_ids, tv_archive, probe_status, probe_error, active
            FROM channels WHERE active = 1 ORDER BY playlist_id, name LIMIT 500`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var channels []models.Channel
		for rows.Next() {
			var item models.Channel
			var categories string
			var archive, active int
			if err := rows.Scan(&item.ID, &item.PlaylistID, &item.ProviderID, &item.Name, &item.Number, &item.Logo,
				&item.EPGID, &categories, &archive, &item.ProbeStatus, &item.ProbeError, &active); err != nil {
				return nil, err
			}
			item.CategoryIDs = decodeCategories(categories)
			item.TVArchive = archive == 1
			item.Active = active == 1
			channels = append(channels, item)
		}
		return map[string]any{"kind": "live", "total": total, "items": channels}, nil
	}
	return nil, sql.ErrNoRows
}

func fmtSscan(value, format string, args ...any) (int, error) {
	return fmt.Sscanf(value, format, args...)
}
