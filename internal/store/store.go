package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/leleasley/rex-xstream/internal/models"
	"github.com/leleasley/rex-xstream/internal/security"
)

// Store owns all persistence. Credentials are encrypted before they touch disk.
type Store struct {
	db     *sql.DB
	cipher *security.Cipher
}

func New(db *sql.DB, cipher *security.Cipher) *Store {
	return &Store{db: db, cipher: cipher}
}

func (s *Store) DB() *sql.DB { return s.db }

func now() int64 { return time.Now().UnixMilli() }

func msToTime(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	t := time.UnixMilli(value.Int64).UTC()
	return &t
}

func jsonStrings(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return nil
	}
	return out
}

func stringsJSON(values []string) string {
	if values == nil {
		return "[]"
	}
	body, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(body)
}

// PlaylistInput is the write shape for create and update.
type PlaylistInput struct {
	Name     string
	BaseURL  string
	Username string
	Password string
	Enabled  *bool
}

type PlaylistAuth struct {
	ID       int64
	Name     string
	BaseURL  string
	Username string
	Password string
	Enabled  bool
}

func (s *Store) CreatePlaylist(ctx context.Context, in PlaylistInput) (models.Playlist, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return models.Playlist{}, fmt.Errorf("name is required")
	}
	if len(name) > 120 {
		return models.Playlist{}, fmt.Errorf("name is too long")
	}
	base, err := NormalizeBaseURL(in.BaseURL)
	if err != nil {
		return models.Playlist{}, err
	}
	if strings.TrimSpace(in.Username) == "" || in.Password == "" {
		return models.Playlist{}, fmt.Errorf("username and password are required")
	}
	encUser, err := s.cipher.Encrypt(in.Username)
	if err != nil {
		return models.Playlist{}, err
	}
	encPass, err := s.cipher.Encrypt(in.Password)
	if err != nil {
		return models.Playlist{}, err
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	stamp := now()
	res, err := s.db.ExecContext(ctx, `INSERT INTO playlists
        (name, base_url, username_enc, password_enc, enabled, status, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, 'new', ?, ?)`,
		name, base, encUser, encPass, boolInt(enabled), stamp, stamp)
	if err != nil {
		return models.Playlist{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return models.Playlist{}, err
	}
	if err := s.EnsureLibraries(ctx, id, name); err != nil {
		return models.Playlist{}, err
	}
	return s.GetPlaylist(ctx, id)
}

func (s *Store) UpdatePlaylist(ctx context.Context, id int64, in PlaylistInput) (models.Playlist, error) {
	current, err := s.GetPlaylist(ctx, id)
	if err != nil {
		return models.Playlist{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = current.Name
	}
	if len(name) > 120 {
		return models.Playlist{}, fmt.Errorf("name is too long")
	}
	if in.Name != "" && name != current.Name {
		if err := s.RenameLibraries(ctx, id, name); err != nil {
			return models.Playlist{}, err
		}
	}
	sets := []string{"name = ?", "updated_at = ?"}
	args := []any{name, now()}
	resetProbes := false
	if in.BaseURL != "" {
		base, err := NormalizeBaseURL(in.BaseURL)
		if err != nil {
			return models.Playlist{}, err
		}
		if base != current.BaseURL {
			resetProbes = true
		}
		sets = append(sets, "base_url = ?")
		args = append(args, base)
	}
	if in.Username != "" {
		enc, err := s.cipher.Encrypt(in.Username)
		if err != nil {
			return models.Playlist{}, err
		}
		sets = append(sets, "username_enc = ?", "status = 'new'")
		args = append(args, enc)
		resetProbes = true
	}
	if in.Password != "" {
		enc, err := s.cipher.Encrypt(in.Password)
		if err != nil {
			return models.Playlist{}, err
		}
		sets = append(sets, "password_enc = ?", "status = 'new'")
		args = append(args, enc)
		resetProbes = true
	}
	if in.Enabled != nil {
		sets = append(sets, "enabled = ?")
		args = append(args, boolInt(*in.Enabled))
	}
	args = append(args, id)
	if _, err := s.db.ExecContext(ctx, `UPDATE playlists SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return models.Playlist{}, err
	}
	if resetProbes {
		if err := s.ResetPlaylistProbes(ctx, id); err != nil {
			return models.Playlist{}, err
		}
	}
	return s.GetPlaylist(ctx, id)
}

// ResetPlaylistProbes forces a fresh probe after the provider URL or
// credentials change. Existing rows stay visible until the new probe lands.
func (s *Store) ResetPlaylistProbes(ctx context.Context, playlistID int64) error {
	queries := []string{
		`UPDATE channels SET probe_status = 'pending', probe_error = '', media_info = '', dirty = 1 WHERE playlist_id = ?`,
		`UPDATE movies SET probe_status = 'pending', probe_error = '', media_info = '', dirty = 1 WHERE playlist_id = ?`,
		`UPDATE episodes SET probe_status = 'pending', probe_error = '', media_info = '', dirty = 1
            WHERE series_id IN (SELECT id FROM series WHERE playlist_id = ?)`,
	}
	for _, query := range queries {
		if _, err := s.db.ExecContext(ctx, query, playlistID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) DeletePlaylist(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM playlists WHERE id = ?`, id); err != nil {
		return err
	}
	return nil
}

func (s *Store) GetPlaylist(ctx context.Context, id int64) (models.Playlist, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, base_url, enabled, status, provider_json,
        subscription_end, max_connections, last_test_at, last_sync_at, last_sync_error, created_at, updated_at
        FROM playlists WHERE id = ?`, id)
	var p models.Playlist
	var enabled int
	var provider string
	var sub, test, sync sql.NullInt64
	var created, updated int64
	if err := row.Scan(&p.ID, &p.Name, &p.BaseURL, &enabled, &p.Status, &provider,
		&sub, &p.MaxConnections, &test, &sync, &p.LastSyncError, &created, &updated); err != nil {
		return models.Playlist{}, err
	}
	p.Enabled = enabled == 1
	p.Provider = parseProvider(provider)
	p.SubscriptionEnd = msToTime(sub)
	p.LastTestAt = msToTime(test)
	p.LastSyncAt = msToTime(sync)
	p.CreatedAt = time.UnixMilli(created).UTC()
	p.UpdatedAt = time.UnixMilli(updated).UTC()
	p.HasCredentials = true
	libs, _ := s.ListLibraries(ctx)
	for _, lib := range libs {
		if lib.PlaylistID == id {
			p.Directories = append(p.Directories, lib)
		}
	}
	return p, nil
}

func (s *Store) ListPlaylists(ctx context.Context) ([]models.Playlist, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM playlists ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	out := make([]models.Playlist, 0, len(ids))
	for _, id := range ids {
		p, err := s.GetPlaylist(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *Store) PlaylistAuth(ctx context.Context, id int64) (PlaylistAuth, error) {
	var out PlaylistAuth
	var encUser, encPass string
	var enabled int
	if err := s.db.QueryRowContext(ctx, `SELECT id, name, base_url, username_enc, password_enc, enabled
        FROM playlists WHERE id = ?`, id).Scan(&out.ID, &out.Name, &out.BaseURL, &encUser, &encPass, &enabled); err != nil {
		return PlaylistAuth{}, err
	}
	user, err := s.cipher.Decrypt(encUser)
	if err != nil {
		return PlaylistAuth{}, err
	}
	pass, err := s.cipher.Decrypt(encPass)
	if err != nil {
		return PlaylistAuth{}, err
	}
	out.Username = user
	out.Password = pass
	out.Enabled = enabled == 1
	return out, nil
}

func (s *Store) EnabledPlaylists(ctx context.Context) ([]PlaylistAuth, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM playlists WHERE enabled = 1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlaylistAuth
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		auth, err := s.PlaylistAuth(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, auth)
	}
	return out, nil
}

func (s *Store) SetPlaylistTest(ctx context.Context, id int64, status, providerJSON string, expiry *time.Time, maxConns int) error {
	var expiryMS any
	if expiry != nil {
		expiryMS = expiry.UnixMilli()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE playlists SET status = ?, provider_json = ?, subscription_end = ?,
        max_connections = ?, last_test_at = ?, updated_at = ? WHERE id = ?`,
		status, providerJSON, expiryMS, maxConns, now(), now(), id)
	return err
}

func (s *Store) SetPlaylistSyncResult(ctx context.Context, id int64, status, errText string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE playlists SET status = ?, last_sync_at = ?, last_sync_error = ?,
        updated_at = ? WHERE id = ?`, status, now(), errText, now(), id)
	return err
}

func (s *Store) SetPlaylistStatus(ctx context.Context, id int64, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE playlists SET status = ?, updated_at = ? WHERE id = ?`, status, now(), id)
	return err
}

func (s *Store) LibraryByDirectory(ctx context.Context, directory string) (models.Library, error) {
	var lib models.Library
	var playlist sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT playlist_id, kind, directory, title FROM libraries WHERE directory = ?`, directory).
		Scan(&playlist, &lib.Kind, &lib.Directory, &lib.Title)
	if err == nil && playlist.Valid {
		lib.PlaylistID = playlist.Int64
	}
	return lib, err
}

func (s *Store) ListLibraries(ctx context.Context) ([]models.Library, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT playlist_id, kind, directory, title FROM libraries ORDER BY kind, playlist_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Library
	for rows.Next() {
		var lib models.Library
		var playlist sql.NullInt64
		if err := rows.Scan(&playlist, &lib.Kind, &lib.Directory, &lib.Title); err != nil {
			return nil, err
		}
		if playlist.Valid {
			lib.PlaylistID = playlist.Int64
		}
		out = append(out, lib)
	}
	return out, nil
}

func (s *Store) encryptURL(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	return s.cipher.Encrypt(value)
}

func (s *Store) decryptURL(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	return s.cipher.Decrypt(value)
}

func parseProvider(raw string) models.Provider {
	var p models.Provider
	if raw == "" {
		return p
	}
	_ = json.Unmarshal([]byte(raw), &p)
	return p
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
