package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "desktop.sqlite"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS runs (id TEXT PRIMARY KEY, updated_at TEXT NOT NULL, body BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS events (id INTEGER PRIMARY KEY AUTOINCREMENT, run_id TEXT NOT NULL, body BLOB NOT NULL);
CREATE INDEX IF NOT EXISTS events_run ON events(run_id,id);
CREATE TABLE IF NOT EXISTS idempotency (key TEXT PRIMARY KEY, action TEXT NOT NULL, fingerprint TEXT NOT NULL, run_id TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS provider_settings (id TEXT PRIMARY KEY, body BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS preparations (id TEXT PRIMARY KEY, request_hash TEXT NOT NULL, status TEXT NOT NULL, used_by_run_id TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL, body BLOB NOT NULL);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		db.Close()
		return nil, err
	}
	if version > model.SchemaVersion {
		db.Close()
		return nil, fmt.Errorf("数据版本 %d 高于当前支持版本 %d", version, model.SchemaVersion)
	}
	if version < model.SchemaVersion {
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version=%d", model.SchemaVersion)); err != nil {
			db.Close()
			return nil, err
		}
	}
	// SQLite may create these files using the process umask; settings stay private.
	for _, name := range []string{"desktop.sqlite", "desktop.sqlite-wal", "desktop.sqlite-shm"} {
		if err := os.Chmod(filepath.Join(dir, name), 0600); err != nil && !errors.Is(err, os.ErrNotExist) {
			db.Close()
			return nil, err
		}
	}
	return &Store{db}, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Load(ctx context.Context, id string) (*model.StoredRun, error) {
	var b []byte
	if err := s.db.QueryRowContext(ctx, "SELECT body FROM runs WHERE id=?", id).Scan(&b); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("讨论不存在: %s", id)
		}
		return nil, err
	}
	var r model.StoredRun
	err := json.Unmarshal(b, &r)
	normalize(&r)
	return &r, err
}
func (s *Store) All(ctx context.Context) ([]model.StoredRun, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT body FROM runs ORDER BY updated_at DESC,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.StoredRun{}
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		var r model.StoredRun
		if err = json.Unmarshal(b, &r); err != nil {
			return nil, err
		}
		normalize(&r)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Save atomically persists the snapshot and its durable reconnect cursor.
func (s *Store) Save(ctx context.Context, r *model.StoredRun, e *model.DesktopEvent, key, action, fingerprint string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if r.ConsumePreparationID != "" {
		if err := consumePreparation(ctx, tx, r.ConsumePreparationID, r); err != nil {
			return err
		}
	}
	if key != "" {
		if _, err = tx.ExecContext(ctx, "INSERT INTO idempotency(key,action,fingerprint,run_id) VALUES(?,?,?,?)", key, action, fingerprint, r.Snapshot.ID); err != nil {
			return err
		}
	}
	if e != nil {
		result, err := tx.ExecContext(ctx, "INSERT INTO events(run_id,body) VALUES(?,?)", r.Snapshot.ID, []byte("{}"))
		if err != nil {
			return err
		}
		e.EventID, err = result.LastInsertId()
		if err != nil {
			return err
		}
		r.Snapshot.LastEventID = e.EventID
		b, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE events SET body=? WHERE id=?", b, e.EventID); err != nil {
			return err
		}
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO runs(id,updated_at,body) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET updated_at=excluded.updated_at,body=excluded.body", r.Snapshot.ID, r.Snapshot.UpdatedAt, b); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	r.ConsumePreparationID = ""
	return nil
}
func (s *Store) LookupKey(ctx context.Context, key, action, fingerprint string) (string, error) {
	var a, f, id string
	err := s.db.QueryRowContext(ctx, "SELECT action,fingerprint,run_id FROM idempotency WHERE key=?", key).Scan(&a, &f, &id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if a != action || f != fingerprint {
		return "", fmt.Errorf("idempotencyKey 已用于不同请求")
	}
	return id, nil
}
func (s *Store) Events(ctx context.Context, runID string, after int64) ([]model.DesktopEvent, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT body FROM events WHERE run_id=? AND id>? ORDER BY id", runID, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.DesktopEvent{}
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		var e model.DesktopEvent
		if err = json.Unmarshal(b, &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// JSON snapshots migrate additively; provenance and the original config hash are
// intentionally preserved, including historical Mock runs.
func normalize(r *model.StoredRun) {
	s := &r.Snapshot
	if s.Materials == nil {
		s.Materials = []model.Material{}
	}
	if s.ProviderMode == "" {
		s.ProviderMode = r.Frozen.ProviderMode
	}
	if s.ProviderMode == "" {
		s.ProviderMode = "mock"
	}
	if s.Messages == nil {
		s.Messages = []model.ChatMessage{}
	}
	if s.Reviews == nil {
		s.Reviews = []model.ReviewIssue{}
	}
	if s.Claims == nil {
		s.Claims = []model.ClaimView{}
	}
	if s.Providers == nil {
		s.Providers = []model.ProviderConfig{}
	}
	if s.ProposalHistory == nil {
		s.ProposalHistory = []model.Proposal{}
		if s.Proposal != nil {
			s.ProposalHistory = append(s.ProposalHistory, *s.Proposal)
		}
	}
	if s.Ballots == nil {
		s.Ballots = []model.BallotBinding{}
	}
	if s.Evidence == nil {
		s.Evidence = []model.Evidence{}
	}
	if s.Decision != nil {
		if s.Decision.Votes == nil {
			s.Decision.Votes = []model.Vote{}
		}
		if s.Decision.Assumptions == nil {
			s.Decision.Assumptions = []string{}
		}
		if s.Decision.MinorityOpinions == nil {
			s.Decision.MinorityOpinions = []string{}
		}
	}
}
