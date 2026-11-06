package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/leleasley/rex-xstream/internal/models"
)

const LiveDirectory = "live"

func movieDirectory(playlistID int64) string  { return fmt.Sprintf("p%d-movies", playlistID) }
func seriesDirectory(playlistID int64) string { return fmt.Sprintf("p%d-shows", playlistID) }

// EnsureLibraries creates the stable directories for a playlist. Directory
// names never change; only the titles do, so rex keeps one library per playlist.
func (s *Store) EnsureLibraries(ctx context.Context, playlistID int64, name string) error {
	stamp := now()
	entries := []struct {
		kind  string
		dir   string
		title string
	}{
		{"movies", movieDirectory(playlistID), strings.TrimSpace(name) + " Movies"},
		{"series", seriesDirectory(playlistID), strings.TrimSpace(name) + " Series"},
	}
	for _, entry := range entries {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO libraries(playlist_id, kind, directory, title, updated_at)
            VALUES(?, ?, ?, ?, ?)
            ON CONFLICT(playlist_id, kind) DO UPDATE SET title = excluded.title, updated_at = excluded.updated_at`,
			playlistID, entry.kind, entry.dir, entry.title, stamp); err != nil {
			return err
		}
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO libraries(playlist_id, kind, directory, title, updated_at)
        VALUES(NULL, 'live', ?, 'Live TV', ?)
        ON CONFLICT(directory) DO UPDATE SET title = 'Live TV', updated_at = excluded.updated_at`, LiveDirectory, stamp)
	return err
}

func (s *Store) RenameLibraries(ctx context.Context, playlistID int64, name string) error {
	stamp := now()
	if _, err := s.db.ExecContext(ctx, `UPDATE libraries SET title = ?, updated_at = ? WHERE playlist_id = ? AND kind = 'movies'`,
		strings.TrimSpace(name)+" Movies", stamp, playlistID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE libraries SET title = ?, updated_at = ? WHERE playlist_id = ? AND kind = 'series'`,
		strings.TrimSpace(name)+" Series", stamp, playlistID)
	return err
}

// LibrariesForRex returns the directories of enabled playlists plus Live TV.
func (s *Store) LibrariesForRex(ctx context.Context) ([]models.Library, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT l.playlist_id, l.kind, l.directory, l.title
        FROM libraries l
        LEFT JOIN playlists p ON p.id = l.playlist_id
        WHERE l.kind = 'live' OR p.enabled = 1
        ORDER BY l.kind, l.playlist_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Library
	for rows.Next() {
		var lib models.Library
		var playlist sqlNullInt
		if err := rows.Scan(&playlist, &lib.Kind, &lib.Directory, &lib.Title); err != nil {
			return nil, err
		}
		if playlist.Valid {
			lib.PlaylistID = playlist.Int
		}
		out = append(out, lib)
	}
	return out, nil
}

// sqlNullInt is a small alias so library.go does not import database/sql just
// for one scan type.
type sqlNullInt struct {
	Int   int64
	Valid bool
}

func (n *sqlNullInt) Scan(value any) error {
	if value == nil {
		n.Valid = false
		return nil
	}
	switch v := value.(type) {
	case int64:
		n.Int, n.Valid = v, true
	case int32:
		n.Int, n.Valid = int64(v), true
	default:
		return fmt.Errorf("unsupported nullable integer %T", value)
	}
	return nil
}
