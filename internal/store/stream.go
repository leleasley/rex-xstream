package store

import (
	"context"
	"database/sql"

	"github.com/leleasley/rex-xstream/internal/library"
)

// StreamPath is the addon-relative path rex asks for. It is derived from the
// stored row, never from client input.
func (s *Store) StreamPath(ctx context.Context, kind string, playlistID, id int64) (string, error) {
	switch kind {
	case "movie":
		var container string
		err := s.db.QueryRowContext(ctx, `SELECT container FROM movies WHERE id = ? AND playlist_id = ?`, id, playlistID).Scan(&container)
		if err != nil {
			return "", err
		}
		return library.MoviePath(playlistID, id, container), nil
	case "episode":
		var container string
		err := s.db.QueryRowContext(ctx, `SELECT e.container FROM episodes e JOIN series s ON s.id = e.series_id WHERE e.id = ? AND s.playlist_id = ?`, id, playlistID).Scan(&container)
		if err != nil {
			return "", err
		}
		return library.EpisodePath(playlistID, id, container), nil
	case "channel", "live":
		var exists int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM channels WHERE id = ?`, id).Scan(&exists); err != nil {
			return "", err
		}
		if exists == 0 {
			return "", sql.ErrNoRows
		}
		return library.LivePath(id), nil
	default:
		return "", sql.ErrNoRows
	}
}
