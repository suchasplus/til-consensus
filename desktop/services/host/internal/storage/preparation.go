package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
)

func (s *Store) GetPreparation(ctx context.Context, id string) (model.PreparationRecord, error) {
	var body []byte
	var used string
	if err := s.db.QueryRowContext(ctx, "SELECT body,used_by_run_id FROM preparations WHERE id=?", id).Scan(&body, &used); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.PreparationRecord{}, fmt.Errorf("准备记录不存在: %s", id)
		}
		return model.PreparationRecord{}, err
	}
	var record model.PreparationRecord
	err := json.Unmarshal(body, &record)
	record.UsedByRunID = used
	return record, err
}
func (s *Store) PreparationExists(ctx context.Context, id string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM preparations WHERE id=?", id).Scan(&count)
	return count > 0, err
}
func (s *Store) SavePreparation(ctx context.Context, record *model.PreparationRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var used string
	err = tx.QueryRowContext(ctx, "SELECT used_by_run_id FROM preparations WHERE id=?", record.ID).Scan(&used)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if used != "" {
		record.UsedByRunID = used
	}
	body, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO preparations(id,request_hash,status,used_by_run_id,updated_at,body) VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET request_hash=excluded.request_hash,status=excluded.status,used_by_run_id=excluded.used_by_run_id,updated_at=excluded.updated_at,body=excluded.body`, record.ID, record.RequestHash, record.Status, record.UsedByRunID, record.UpdatedAt, body)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Preparations(ctx context.Context) ([]model.PreparationRecord, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT body,used_by_run_id FROM preparations ORDER BY updated_at,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.PreparationRecord{}
	for rows.Next() {
		var body []byte
		var used string
		if err := rows.Scan(&body, &used); err != nil {
			return nil, err
		}
		var record model.PreparationRecord
		if err := json.Unmarshal(body, &record); err != nil {
			return nil, err
		}
		record.UsedByRunID = used
		result = append(result, record)
	}
	return result, rows.Err()
}
func consumePreparation(ctx context.Context, tx *sql.Tx, id string, run *model.StoredRun) error {
	var body []byte
	var status, used string
	if err := tx.QueryRowContext(ctx, "SELECT body,status,used_by_run_id FROM preparations WHERE id=?", id).Scan(&body, &status, &used); err != nil {
		return err
	}
	if used != "" {
		return fmt.Errorf("准备结果已被讨论 %s 采用，不能重复批准", used)
	}
	if status != "completed" {
		return fmt.Errorf("准备结果未完成，不能采用")
	}
	var record model.PreparationRecord
	if err := json.Unmarshal(body, &record); err != nil {
		return err
	}
	if run.Snapshot.Preparation == nil || record.InputHash != run.Snapshot.Preparation.InputHash || record.RequestHash != run.Snapshot.Preparation.RequestHash {
		return fmt.Errorf("准备结果在创建讨论前发生变化")
	}
	record.UsedByRunID = run.Snapshot.ID
	updated, err := json.Marshal(record)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, "UPDATE preparations SET used_by_run_id=?,body=? WHERE id=? AND used_by_run_id='' AND status='completed' AND request_hash=?", run.Snapshot.ID, updated, id, record.RequestHash)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("准备结果已被其他讨论采用")
	}
	return nil
}
