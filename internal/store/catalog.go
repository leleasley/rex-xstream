package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/leleasley/rex-xstream/internal/models"
)

func (s *Store) SyncCategories(ctx context.Context, playlistID int64, kind string, categories []models.Category) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO categories(playlist_id, kind, provider_id, name)
        VALUES(?, ?, ?, ?)
        ON CONFLICT(playlist_id, kind, provider_id) DO UPDATE SET name = excluded.name`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, category := range categories {
		if strings.TrimSpace(category.ProviderID) == "" {
			continue
		}
		if _, err := stmt.ExecContext(ctx, playlistID, kind, category.ProviderID, category.Name); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SyncChannels(ctx context.Context, playlistID int64, channels []models.Channel) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE channels SET active = 0 WHERE playlist_id = ? AND active = 1`, playlistID); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO channels
        (playlist_id, provider_id, name, number, logo, epg_id, category_ids, tv_archive, stream_url,
         probe_status, probe_error, media_info, dirty, active, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', '', '', 1, 1, ?)
        ON CONFLICT(playlist_id, provider_id) DO UPDATE SET
            name = excluded.name, number = excluded.number, logo = excluded.logo,
            epg_id = excluded.epg_id, category_ids = excluded.category_ids,
            tv_archive = excluded.tv_archive, active = 1, updated_at = excluded.updated_at,
            probe_status = channels.probe_status,
            dirty = CASE WHEN channels.name <> excluded.name
                OR channels.logo <> excluded.logo OR channels.epg_id <> excluded.epg_id THEN 1 ELSE channels.dirty END,
            stream_url = excluded.stream_url`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	stamp := now()
	for _, channel := range channels {
		if channel.ProviderID == "" {
			continue
		}
		encrypted, err := s.encryptURL(channel.StreamURL)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx, playlistID, channel.ProviderID, channel.Name, channel.Number,
			channel.Logo, channel.EPGID, stringsJSON(channel.CategoryIDs), boolInt(channel.TVArchive),
			encrypted, stamp); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SyncMovies(ctx context.Context, playlistID int64, movies []models.Movie) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE movies SET active = 0 WHERE playlist_id = ? AND active = 1`, playlistID); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO movies
        (playlist_id, provider_id, title, year, tmdb_id, imdb_id, poster, backdrop, plot, genre, rating,
         duration, container, category_ids, stream_url, probe_status, probe_error, media_info, dirty, active, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', '', '', 1, 1, ?)
        ON CONFLICT(playlist_id, provider_id) DO UPDATE SET
            title = excluded.title, year = excluded.year, tmdb_id = excluded.tmdb_id, imdb_id = excluded.imdb_id,
            poster = excluded.poster, backdrop = excluded.backdrop, plot = excluded.plot, genre = excluded.genre,
            rating = excluded.rating, duration = excluded.duration, container = excluded.container,
            category_ids = excluded.category_ids, active = 1, updated_at = excluded.updated_at,
            probe_status = CASE WHEN movies.container <> excluded.container THEN 'pending' ELSE movies.probe_status END,
            media_info = CASE WHEN movies.container <> excluded.container THEN '' ELSE movies.media_info END,
            probe_error = CASE WHEN movies.container <> excluded.container THEN '' ELSE movies.probe_error END,
            dirty = CASE WHEN movies.container <> excluded.container OR movies.title <> excluded.title
                OR movies.tmdb_id <> excluded.tmdb_id OR movies.poster <> excluded.poster THEN 1 ELSE movies.dirty END,
            stream_url = excluded.stream_url`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	stamp := now()
	for _, movie := range movies {
		if movie.ProviderID == "" {
			continue
		}
		encrypted, err := s.encryptURL(movie.StreamURL)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx, playlistID, movie.ProviderID, movie.Title, movie.Year, movie.TMDBID,
			movie.IMDBID, movie.Poster, movie.Backdrop, movie.Plot, movie.Genre, movie.Rating, movie.Duration,
			movie.Container, stringsJSON(movie.CategoryIDs), encrypted, stamp); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SyncSeries(ctx context.Context, playlistID int64, series []models.Series) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE series SET active = 0 WHERE playlist_id = ? AND active = 1`, playlistID); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO series
        (playlist_id, provider_id, title, year, tmdb_id, poster, backdrop, plot, genre, rating,
         category_ids, provider_modified, active, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?)
        ON CONFLICT(playlist_id, provider_id) DO UPDATE SET
            title = excluded.title, year = excluded.year, tmdb_id = excluded.tmdb_id,
            poster = excluded.poster, backdrop = excluded.backdrop, plot = excluded.plot,
            genre = excluded.genre, rating = excluded.rating, category_ids = excluded.category_ids,
            provider_modified = excluded.provider_modified, active = 1, updated_at = excluded.updated_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	stamp := now()
	for _, item := range series {
		if item.ProviderID == "" {
			continue
		}
		if _, err := stmt.ExecContext(ctx, playlistID, item.ProviderID, item.Title, item.Year, item.TMDBID,
			item.Poster, item.Backdrop, item.Plot, item.Genre, item.Rating, stringsJSON(item.CategoryIDs),
			item.ProviderModified, stamp); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SyncSeriesDetail replaces one series' seasons and episodes.
func (s *Store) SyncSeriesDetail(ctx context.Context, seriesID int64, seasons []models.Season, episodes []models.Episode) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE episodes SET active = 0 WHERE series_id = ? AND active = 1`, seriesID); err != nil {
		return err
	}
	seasonStmt, err := tx.PrepareContext(ctx, `INSERT INTO seasons(series_id, number, title, poster)
        VALUES(?, ?, ?, ?) ON CONFLICT(series_id, number) DO UPDATE SET title = excluded.title, poster = excluded.poster`)
	if err != nil {
		return err
	}
	defer seasonStmt.Close()
	for _, season := range seasons {
		if _, err := seasonStmt.ExecContext(ctx, seriesID, season.Number, season.Title, season.Poster); err != nil {
			return err
		}
	}
	episodeStmt, err := tx.PrepareContext(ctx, `INSERT INTO episodes
        (series_id, season_number, episode_number, provider_id, title, plot, image, container, tmdb_episode,
         stream_url, probe_status, probe_error, media_info, dirty, active, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', '', '', 1, 1, ?)
        ON CONFLICT(series_id, season_number, episode_number) DO UPDATE SET
            provider_id = excluded.provider_id, title = excluded.title, plot = excluded.plot,
            image = excluded.image, container = excluded.container, tmdb_episode = excluded.tmdb_episode,
            active = 1, updated_at = excluded.updated_at,
            probe_status = CASE WHEN episodes.container <> excluded.container THEN 'pending' ELSE episodes.probe_status END,
            media_info = CASE WHEN episodes.container <> excluded.container THEN '' ELSE episodes.media_info END,
            probe_error = CASE WHEN episodes.container <> excluded.container THEN '' ELSE episodes.probe_error END,
            dirty = CASE WHEN episodes.container <> excluded.container OR episodes.title <> excluded.title
                OR episodes.tmdb_episode <> excluded.tmdb_episode THEN 1 ELSE episodes.dirty END,
            stream_url = excluded.stream_url`)
	if err != nil {
		return err
	}
	defer episodeStmt.Close()
	stamp := now()
	for _, episode := range episodes {
		encrypted, err := s.encryptURL(episode.StreamURL)
		if err != nil {
			return err
		}
		if _, err := episodeStmt.ExecContext(ctx, seriesID, episode.SeasonNumber, episode.EpisodeNumber,
			episode.ProviderID, episode.Title, episode.Plot, episode.Image, episode.Container,
			episode.TMDBEpisode, encrypted, stamp); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE series SET last_detail_fetch = ? WHERE id = ?`, stamp, seriesID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SeriesNeedingDetail(ctx context.Context, playlistID int64) ([]models.Series, error) {
	cutoff := time.Now().Add(-24 * time.Hour).UnixMilli()
	rows, err := s.db.QueryContext(ctx, `SELECT s.id, s.provider_id, s.title, s.tmdb_id, s.provider_modified,
        s.last_detail_fetch, (SELECT COUNT(1) FROM episodes e WHERE e.series_id = s.id AND e.active = 1)
        FROM series s WHERE s.playlist_id = ? AND s.active = 1
        AND (s.last_detail_fetch IS NULL OR s.last_detail_fetch < ?
             OR (SELECT COUNT(1) FROM episodes e WHERE e.series_id = s.id AND e.active = 1) = 0)
        ORDER BY s.id`, playlistID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Series
	for rows.Next() {
		var item models.Series
		var modified string
		var last sql.NullInt64
		var episodes int
		if err := rows.Scan(&item.ID, &item.ProviderID, &item.Title, &item.TMDBID, &modified, &last, &episodes); err != nil {
			return nil, err
		}
		item.PlaylistID = playlistID
		item.ProviderModified = modified
		_ = episodes
		_ = last
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) ActiveSeries(ctx context.Context, playlistID int64) ([]models.Series, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, provider_id, title, year, tmdb_id, poster, backdrop, plot, genre, rating, provider_modified
        FROM series WHERE playlist_id = ? AND active = 1 ORDER BY title`, playlistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Series
	for rows.Next() {
		var item models.Series
		if err := rows.Scan(&item.ID, &item.ProviderID, &item.Title, &item.Year, &item.TMDBID,
			&item.Poster, &item.Backdrop, &item.Plot, &item.Genre, &item.Rating, &item.ProviderModified); err != nil {
			return nil, err
		}
		item.PlaylistID = playlistID
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) CategoryCount(ctx context.Context, playlistID int64, kind string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM categories WHERE playlist_id = ? AND kind = ?`, playlistID, kind).Scan(&count)
	return count, err
}

func (s *Store) ReplaceProgrammes(ctx context.Context, playlistID int64, programmes []models.Programme) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM programmes WHERE playlist_id = ?`, playlistID); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO programmes
        (playlist_id, channel_epg_id, start_utc, stop_utc, title, description, category, icon, season, episode)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(playlist_id, channel_epg_id, start_utc) DO UPDATE SET
            stop_utc = excluded.stop_utc, title = excluded.title, description = excluded.description,
            category = excluded.category, icon = excluded.icon, season = excluded.season, episode = excluded.episode`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, programme := range programmes {
		if programme.ChannelEPGID == "" || programme.Title == "" {
			continue
		}
		if _, err := stmt.ExecContext(ctx, playlistID, programme.ChannelEPGID,
			programme.Start.UnixMilli(), programme.Stop.UnixMilli(), programme.Title,
			programme.Description, programme.Category, programme.Icon, programme.Season, programme.Episode); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ProgrammeCount(ctx context.Context, playlistID int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM programmes WHERE playlist_id = ?`, playlistID).Scan(&count)
	return count, err
}

func (s *Store) Guide(ctx context.Context, playlistID int64, epgIDs []string, from, to int64) (map[string][]models.Programme, error) {
	out := map[string][]models.Programme{}
	if len(epgIDs) == 0 {
		return out, nil
	}
	for start := 0; start < len(epgIDs); start += 400 {
		end := start + 400
		if end > len(epgIDs) {
			end = len(epgIDs)
		}
		chunk := epgIDs[start:end]
		placeholders := make([]string, len(chunk))
		args := []any{playlistID, from, to}
		for i, id := range chunk {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query := `SELECT channel_epg_id, start_utc, stop_utc, title, description, category, icon, season, episode
            FROM programmes WHERE playlist_id = ? AND start_utc >= ? AND start_utc <= ? AND channel_epg_id IN (` +
			strings.Join(placeholders, ",") + `) ORDER BY channel_epg_id, start_utc`
		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var programme models.Programme
			var startMS, stopMS int64
			if err := rows.Scan(&programme.ChannelEPGID, &startMS, &stopMS, &programme.Title,
				&programme.Description, &programme.Category, &programme.Icon, &programme.Season, &programme.Episode); err != nil {
				rows.Close()
				return nil, err
			}
			programme.Start = timeFromMillis(startMS)
			programme.Stop = timeFromMillis(stopMS)
			out[programme.ChannelEPGID] = append(out[programme.ChannelEPGID], programme)
		}
		rows.Close()
	}
	return out, nil
}

func timeFromMillis(value int64) (t time.Time) {
	return time.UnixMilli(value).UTC()
}

func (s *Store) Stats(ctx context.Context) (map[string]int, error) {
	out := map[string]int{}
	queries := map[string]string{
		"playlists":  `SELECT COUNT(1) FROM playlists`,
		"channels":   `SELECT COUNT(1) FROM channels WHERE active = 1`,
		"movies":     `SELECT COUNT(1) FROM movies WHERE active = 1`,
		"series":     `SELECT COUNT(1) FROM series WHERE active = 1`,
		"episodes":   `SELECT COUNT(1) FROM episodes WHERE active = 1`,
		"programmes": `SELECT COUNT(1) FROM programmes`,
	}
	for key, query := range queries {
		var count int
		if err := s.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
			return nil, err
		}
		out[key] = count
	}
	return out, nil
}

func decodeCategories(raw string) []string {
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func (s *Store) PlaylistCounts(ctx context.Context, playlistID int64) (map[string]int, error) {
	out := map[string]int{}
	queries := map[string]string{
		"channels": `SELECT COUNT(1) FROM channels WHERE playlist_id = ? AND active = 1`,
		"movies":   `SELECT COUNT(1) FROM movies WHERE playlist_id = ? AND active = 1`,
		"series":   `SELECT COUNT(1) FROM series WHERE playlist_id = ? AND active = 1`,
		"episodes": `SELECT COUNT(1) FROM episodes e JOIN series s ON s.id = e.series_id WHERE s.playlist_id = ? AND e.active = 1`,
	}
	for key, query := range queries {
		var count int
		if err := s.db.QueryRowContext(ctx, query, playlistID).Scan(&count); err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		out[key] = count
	}
	return out, nil
}
