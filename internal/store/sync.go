package store

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/leleasley/rex-xstream/internal/models"
)

func (s *Store) StartSyncRun(ctx context.Context, playlistID int64) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO sync_runs(playlist_id, started_at, status, phase) VALUES(?, ?, 'running', 'catalog')`, playlistID, now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) SetSyncPhase(ctx context.Context, runID int64, phase string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sync_runs SET phase = ? WHERE id = ?`, phase, runID)
	return err
}

func (s *Store) FinishSyncRun(ctx context.Context, runID int64, status, phase, errText string, counts map[string]int) error {
	body, _ := json.Marshal(counts)
	_, err := s.db.ExecContext(ctx, `UPDATE sync_runs SET finished_at = ?, status = ?, phase = ?, error = ?, counts_json = ? WHERE id = ?`,
		now(), status, phase, errText, string(body), runID)
	return err
}

// MarkInterruptedRuns closes rows left running by a previous process. The
// next sync starts a fresh run.
func (s *Store) MarkInterruptedRuns(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sync_runs SET status = 'interrupted', phase = 'interrupted',
        finished_at = ?, error = 'rex-xstream restarted while this sync was running' WHERE status = 'running'`, now())
	return err
}

func (s *Store) LatestSyncRun(ctx context.Context, playlistID int64) (*models.SyncStatus, error) {
	row := s.db.QueryRowContext(ctx, `SELECT started_at, finished_at, status, phase, error, counts_json
        FROM sync_runs WHERE playlist_id = ? ORDER BY id DESC LIMIT 1`, playlistID)
	var started int64
	var finished sql.NullInt64
	var status, phase, errText, counts string
	if err := row.Scan(&started, &finished, &status, &phase, &errText, &counts); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	out := &models.SyncStatus{PlaylistID: playlistID, Phase: phase, Error: errText, Running: status == "running"}
	startTime := timeFromMillis(started)
	out.StartedAt = &startTime
	if finished.Valid {
		endTime := timeFromMillis(finished.Int64)
		out.FinishedAt = &endTime
	}
	var values map[string]int
	if err := json.Unmarshal([]byte(counts), &values); err == nil {
		out.Channels = values["channels"]
		out.Movies = values["movies"]
		out.Series = values["series"]
		out.Episodes = values["episodes"]
		out.Programmes = values["programmes"]
		out.Probed = values["probed"]
		out.Pushed = values["pushed"]
	}
	return out, nil
}
