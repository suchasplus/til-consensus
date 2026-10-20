package storage

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/suchasplus/til-consensus/desktop/services/host/internal/model"
)

func TestDeletedSnapshotsRemainInAllAndOldSnapshotsStayVisible(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	// This recorded pre-deletion JSON deliberately has no deletedAt field.
	legacy := []byte(`{"Snapshot":{"id":"history-legacy","status":"completed","updatedAt":"2026-10-19T01:00:00Z"},"Frozen":{"providerMode":"mock"}}`)
	if _, err := s.db.Exec("INSERT INTO runs(id,updated_at,body) VALUES(?,?,?)", "history-legacy", "2026-10-19T01:00:00Z", legacy); err != nil {
		t.Fatal(err)
	}
	deleted := &model.StoredRun{Snapshot: model.RunSnapshot{RunSummary: model.RunSummary{ID: "history-deleted", Status: "stopped", UpdatedAt: "2026-10-20T01:00:00Z", DeletedAt: "2026-10-20T01:00:00Z"}, Usage: model.Usage{Calls: 12}, LineageRootID: "history-legacy"}}
	if err := s.Save(context.Background(), deleted, nil, "", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	all, err := s.All(context.Background())
	if err != nil || len(all) != 2 {
		t.Fatal("底层All过滤了最近删除项", all, err)
	}
	old, err := s.Load(context.Background(), "history-legacy")
	if err != nil || old.Snapshot.DeletedAt != "" || old.Snapshot.Status != "completed" {
		t.Fatal("旧数据被当作删除项", old, err)
	}
	body, err := json.Marshal(old.Snapshot)
	if err != nil || strings.Contains(string(body), "deletedAt") {
		t.Fatal("可选字段改变了旧数据的可见性语义", string(body), err)
	}
	retained, err := s.Load(context.Background(), "history-deleted")
	if err != nil || retained.Snapshot.DeletedAt != deleted.Snapshot.DeletedAt || retained.Snapshot.Usage.Calls != 12 || retained.Snapshot.LineageRootID != "history-legacy" {
		t.Fatal("删除项的预算或关联信息丢失", retained, err)
	}
}
